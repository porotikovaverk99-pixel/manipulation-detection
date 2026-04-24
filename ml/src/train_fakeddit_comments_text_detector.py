import argparse
import csv
import io
import json
import os
import pickle
import sys
import zipfile
from collections import defaultdict
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Optional, Tuple

import numpy as np
import pandas as pd
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import (
    accuracy_score,
    average_precision_score,
    confusion_matrix,
    f1_score,
    precision_score,
    recall_score,
    roc_auc_score,
)
from sklearn.pipeline import FeatureUnion, Pipeline


REQUIRED_POST_COLUMNS = {
    "id",
    "clean_title",
    "title",
    "subreddit",
    "score",
    "num_comments",
    "upvote_ratio",
    "hasImage",
    "2_way_label",
}
REQUIRED_COMMENT_COLUMNS = {"submission_id", "body", "ups"}


@dataclass
class TrainConfig:
    data_dir: Path
    comments_zip: Path
    output_dir: Path
    model_version: str
    positive_label: int
    max_train_rows: int
    max_eval_rows: int
    top_comments: int
    max_comment_chars: int
    comment_chunksize: int
    max_features: int
    min_df: int
    random_seed: int


def main() -> None:
    args = parse_args()
    config = TrainConfig(
        data_dir=args.data_dir,
        comments_zip=args.comments_zip,
        output_dir=args.output_dir,
        model_version=args.model_version,
        positive_label=args.positive_label,
        max_train_rows=args.max_train_rows,
        max_eval_rows=args.max_eval_rows,
        top_comments=args.top_comments,
        max_comment_chars=args.max_comment_chars,
        comment_chunksize=args.comment_chunksize,
        max_features=args.max_features,
        min_df=args.min_df,
        random_seed=args.random_seed,
    )
    config.output_dir.mkdir(parents=True, exist_ok=True)

    train_df = sample_frame(load_split(config.data_dir / "all_train.tsv", config.positive_label), config.max_train_rows, config.random_seed, balanced=True)
    valid_df = sample_frame(load_split(config.data_dir / "all_validate.tsv", config.positive_label), config.max_eval_rows, config.random_seed, balanced=False)
    test_df = sample_frame(load_split(config.data_dir / "all_test_public.tsv", config.positive_label), config.max_eval_rows, config.random_seed, balanced=False)

    all_ids = set(train_df["id"]).union(valid_df["id"]).union(test_df["id"])
    comments_by_id = load_top_comments(config.comments_zip, all_ids, config.top_comments, config.max_comment_chars, config.comment_chunksize)

    train_df = attach_comments(train_df, comments_by_id)
    valid_df = attach_comments(valid_df, comments_by_id)
    test_df = attach_comments(test_df, comments_by_id)

    pipeline = make_pipeline(config)
    pipeline.fit(train_df["text_with_comments"], train_df["label"])

    metrics = {
        "model_version": config.model_version,
        "model_kind": "tfidf_word_char_logreg_title_comments",
        "task": "Fakeddit 2-way fake/suspicious title+comments detection",
        "positive_label": config.positive_label,
        "config": serialize_config(config),
        "dataset_summary": {
            "train": summarize_frame(train_df),
            "validate": summarize_frame(valid_df),
            "test_public": summarize_frame(test_df),
        },
        "splits": {
            "validate": evaluate_split(pipeline, valid_df),
            "test_public": evaluate_split(pipeline, test_df),
        },
        "generated_at": datetime.now(timezone.utc).isoformat(),
    }

    model_path = config.output_dir / "fakeddit_comments_text_detector_model.pkl"
    metrics_path = config.output_dir / "fakeddit_comments_text_detector_metrics.json"
    validate_predictions_path = config.output_dir / "fakeddit_comments_validate_predictions.csv"
    test_predictions_path = config.output_dir / "fakeddit_comments_test_public_predictions.csv"
    coverage_path = config.output_dir / "fakeddit_comments_coverage.json"

    with model_path.open("wb") as fh:
        pickle.dump(
            {
                "pipeline": pipeline,
                "model_version": config.model_version,
                "positive_label": config.positive_label,
                "metrics": metrics,
                "trained_at": datetime.now(timezone.utc).isoformat(),
            },
            fh,
        )
    metrics_path.write_text(json.dumps(metrics, indent=2, ensure_ascii=False), encoding="utf-8")
    write_predictions(pipeline, valid_df, validate_predictions_path)
    write_predictions(pipeline, test_df, test_predictions_path)
    coverage_path.write_text(
        json.dumps(
            {
                "target_submission_ids": len(all_ids),
                "submissions_with_comments": len(comments_by_id),
                "coverage_rate": len(comments_by_id) / max(1, len(all_ids)),
                "top_comments": config.top_comments,
                "max_comment_chars": config.max_comment_chars,
            },
            indent=2,
            ensure_ascii=False,
        ),
        encoding="utf-8",
    )

    print(
        json.dumps(
            {
                "model_path": str(model_path),
                "metrics_path": str(metrics_path),
                "coverage_path": str(coverage_path),
                "validate": metrics["splits"]["validate"],
                "test_public": metrics["splits"]["test_public"],
                "coverage": json.loads(coverage_path.read_text(encoding="utf-8")),
            },
            indent=2,
            ensure_ascii=False,
        )
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Train a Fakeddit title+comments text baseline detector")
    parser.add_argument("--data-dir", type=Path, default=Path(os.getenv("FAKEDDIT_DATA_DIR", default_fakeddit_data_dir())))
    parser.add_argument("--comments-zip", type=Path, default=Path(os.getenv("FAKEDDIT_COMMENTS_ZIP", default_comments_zip())))
    parser.add_argument("--output-dir", type=Path, default=Path(os.getenv("FAKEDDIT_COMMENTS_OUTPUT_DIR", default_output_dir())))
    parser.add_argument("--model-version", default=os.getenv("FAKEDDIT_COMMENTS_MODEL_VERSION", "fakeddit-comments-tfidf-logreg-v1"))
    parser.add_argument("--positive-label", type=int, default=int(os.getenv("FAKEDDIT_POSITIVE_LABEL", "0")))
    parser.add_argument("--max-train-rows", type=int, default=int(os.getenv("FAKEDDIT_MAX_TRAIN_ROWS", "100000")))
    parser.add_argument("--max-eval-rows", type=int, default=int(os.getenv("FAKEDDIT_MAX_EVAL_ROWS", "50000")))
    parser.add_argument("--top-comments", type=int, default=int(os.getenv("FAKEDDIT_TOP_COMMENTS", "3")))
    parser.add_argument("--max-comment-chars", type=int, default=int(os.getenv("FAKEDDIT_MAX_COMMENT_CHARS", "600")))
    parser.add_argument("--comment-chunksize", type=int, default=int(os.getenv("FAKEDDIT_COMMENT_CHUNKSIZE", "200000")))
    parser.add_argument("--max-features", type=int, default=int(os.getenv("FAKEDDIT_MAX_FEATURES", "160000")))
    parser.add_argument("--min-df", type=int, default=int(os.getenv("FAKEDDIT_MIN_DF", "2")))
    parser.add_argument("--random-seed", type=int, default=int(os.getenv("FAKEDDIT_RANDOM_SEED", "42")))
    return parser.parse_args()


def default_fakeddit_data_dir() -> str:
    workspace_root = Path(__file__).resolve().parents[3]
    return str(workspace_root / "datasets/raw/fakeddit/text_metadata/all_samples (also includes non multimodal)")


def default_comments_zip() -> str:
    workspace_root = Path(__file__).resolve().parents[3]
    return str(workspace_root / "datasets/raw/fakeddit/comments/all_comments.tsv.zip")


def default_output_dir() -> str:
    workspace_root = Path(__file__).resolve().parents[3]
    return str(workspace_root / "evaluation_outputs/fakeddit_comments_text_detector")


def load_split(path: Path, positive_label: int) -> pd.DataFrame:
    if not path.exists():
        raise FileNotFoundError(f"Fakeddit split not found: {path}")
    df = pd.read_csv(
        path,
        sep="\t",
        usecols=lambda column: column in REQUIRED_POST_COLUMNS,
        dtype="string",
        on_bad_lines="skip",
    )
    missing = REQUIRED_POST_COLUMNS.difference(df.columns)
    if missing:
        raise RuntimeError(f"{path} is missing required columns: {sorted(missing)}")

    df["title_text"] = df["clean_title"].fillna("").str.strip()
    empty_text = df["title_text"] == ""
    df.loc[empty_text, "title_text"] = df.loc[empty_text, "title"].fillna("").str.strip()
    df["label_raw"] = pd.to_numeric(df["2_way_label"], errors="coerce")
    df = df[df["label_raw"].notna()]
    df["label_raw"] = df["label_raw"].astype(int)
    df = df[df["title_text"].str.len() > 0]
    df["label"] = (df["label_raw"] == positive_label).astype(int)
    return df[["id", "title_text", "label", "label_raw", "subreddit", "score", "num_comments", "upvote_ratio", "hasImage"]].reset_index(drop=True)


def sample_frame(df: pd.DataFrame, max_rows: int, random_seed: int, balanced: bool) -> pd.DataFrame:
    if max_rows <= 0 or len(df) <= max_rows:
        return df.reset_index(drop=True)
    if not balanced:
        return df.sample(n=max_rows, random_state=random_seed).reset_index(drop=True)

    per_class = max_rows // 2
    parts = []
    for label in [0, 1]:
        label_df = df[df["label"] == label]
        n = min(per_class, len(label_df))
        if n > 0:
            parts.append(label_df.sample(n=n, random_state=random_seed))
    sampled = pd.concat(parts)
    remainder = max_rows - len(sampled)
    if remainder > 0:
        rest = df.drop(sampled.index, errors="ignore")
        if not rest.empty:
            sampled = pd.concat([sampled, rest.sample(n=min(remainder, len(rest)), random_state=random_seed)])
    return sampled.sample(frac=1.0, random_state=random_seed).reset_index(drop=True)


def load_top_comments(
    comments_zip: Path,
    target_ids: set[str],
    top_comments: int,
    max_comment_chars: int,
    chunksize: int,
) -> Dict[str, List[str]]:
    if not comments_zip.exists():
        raise FileNotFoundError(f"Fakeddit comments archive not found: {comments_zip}")
    set_csv_field_size_limit()
    raw: Dict[str, List[Tuple[float, str]]] = defaultdict(list)
    with zipfile.ZipFile(comments_zip) as archive:
        with archive.open("all_comments.tsv") as raw_handle:
            text_handle = io.TextIOWrapper(raw_handle, encoding="utf-8", errors="replace", newline="")
            reader = csv.DictReader(text_handle, delimiter="\t")
            missing = REQUIRED_COMMENT_COLUMNS.difference(reader.fieldnames or [])
            if missing:
                raise RuntimeError(f"{comments_zip} is missing required columns: {sorted(missing)}")
            for row_index, row in enumerate(reader, start=1):
                submission_id = str(row.get("submission_id") or "").strip()
                if submission_id not in target_ids:
                    if row_index % chunksize == 0:
                        print(
                            json.dumps({"event": "comments_progress", "rows_read": row_index, "covered": len(raw)}),
                            flush=True,
                        )
                    continue
                body = " ".join(str(row.get("body") or "").split())
                if not body:
                    continue
                ups = parse_float(row.get("ups"))
                raw[submission_id].append((ups, body[:max_comment_chars]))
                if len(raw[submission_id]) > top_comments * 4:
                    raw[submission_id] = sorted(raw[submission_id], key=lambda item: item[0], reverse=True)[:top_comments]
                if row_index % chunksize == 0:
                    print(
                        json.dumps({"event": "comments_progress", "rows_read": row_index, "covered": len(raw)}),
                        flush=True,
                    )

    return {
        submission_id: [body for _, body in sorted(items, key=lambda item: item[0], reverse=True)[:top_comments]]
        for submission_id, items in raw.items()
    }


def set_csv_field_size_limit() -> None:
    limit = sys.maxsize
    while True:
        try:
            csv.field_size_limit(limit)
            return
        except OverflowError:
            limit = int(limit / 10)


def parse_float(value: object) -> float:
    try:
        return float(value)
    except (TypeError, ValueError):
        return 0.0


def attach_comments(df: pd.DataFrame, comments_by_id: Dict[str, List[str]]) -> pd.DataFrame:
    df = df.copy()
    df["comments_text"] = df["id"].map(lambda item: " ".join(comments_by_id.get(str(item), [])))
    df["comment_count_used"] = df["id"].map(lambda item: len(comments_by_id.get(str(item), []))).astype(int)
    has_comments = df["comments_text"].str.len() > 0
    df["text_with_comments"] = df["title_text"]
    df.loc[has_comments, "text_with_comments"] = (
        df.loc[has_comments, "title_text"] + " [COMMENTS] " + df.loc[has_comments, "comments_text"]
    )
    return df


def make_pipeline(config: TrainConfig) -> Pipeline:
    features = FeatureUnion(
        transformer_list=[
            (
                "word_tfidf",
                TfidfVectorizer(
                    lowercase=True,
                    strip_accents="unicode",
                    ngram_range=(1, 2),
                    min_df=config.min_df,
                    max_features=config.max_features,
                    sublinear_tf=True,
                ),
            ),
            (
                "char_tfidf",
                TfidfVectorizer(
                    analyzer="char_wb",
                    lowercase=True,
                    strip_accents="unicode",
                    ngram_range=(3, 5),
                    min_df=config.min_df,
                    max_features=max(10000, config.max_features // 3),
                    sublinear_tf=True,
                ),
            ),
        ]
    )
    return Pipeline(
        steps=[
            ("features", features),
            (
                "model",
                LogisticRegression(
                    C=2.0,
                    max_iter=2000,
                    class_weight="balanced",
                    random_state=config.random_seed,
                    solver="saga",
                ),
            ),
        ]
    )


def evaluate_split(pipeline: Pipeline, df: pd.DataFrame) -> Dict[str, object]:
    if df.empty:
        return {"rows": 0}
    y_true = df["label"].to_numpy()
    probabilities = pipeline.predict_proba(df["text_with_comments"])[:, 1]
    predictions = (probabilities >= 0.5).astype(int)
    tn, fp, fn, tp = confusion_matrix(y_true, predictions, labels=[0, 1]).ravel()
    return {
        "rows": int(len(df)),
        "positive_rows": int(y_true.sum()),
        "positive_rate": float(y_true.mean()),
        "comment_coverage_rate": float((df["comment_count_used"] > 0).mean()),
        "mean_comment_count_used": float(df["comment_count_used"].mean()),
        "accuracy": float(accuracy_score(y_true, predictions)),
        "precision": float(precision_score(y_true, predictions, zero_division=0)),
        "recall": float(recall_score(y_true, predictions, zero_division=0)),
        "f1": float(f1_score(y_true, predictions, zero_division=0)),
        "roc_auc": safe_metric(roc_auc_score, y_true, probabilities),
        "pr_auc": safe_metric(average_precision_score, y_true, probabilities),
        "precision_at_100": precision_at_k(y_true, probabilities, 100),
        "precision_at_500": precision_at_k(y_true, probabilities, 500),
        "confusion_matrix": {"tn": int(tn), "fp": int(fp), "fn": int(fn), "tp": int(tp)},
    }


def write_predictions(pipeline: Pipeline, df: pd.DataFrame, path: Path) -> None:
    output = df[["id", "label", "label_raw", "subreddit", "title_text", "comment_count_used"]].copy()
    output["probability"] = pipeline.predict_proba(df["text_with_comments"])[:, 1]
    output["predicted_label"] = (output["probability"] >= 0.5).astype(int)
    output.sort_values("probability", ascending=False).to_csv(path, index=False)


def summarize_frame(df: pd.DataFrame) -> Dict[str, object]:
    if df.empty:
        return {"rows": 0}
    return {
        "rows": int(len(df)),
        "positive_rows": int(df["label"].sum()),
        "positive_rate": float(df["label"].mean()),
        "raw_label_counts": {str(k): int(v) for k, v in df["label_raw"].value_counts().sort_index().items()},
        "subreddit_count": int(df["subreddit"].fillna("").nunique()),
        "comment_coverage_rate": float((df["comment_count_used"] > 0).mean()),
        "mean_comment_count_used": float(df["comment_count_used"].mean()),
    }


def precision_at_k(y_true: np.ndarray, probabilities: np.ndarray, k: int) -> float:
    if len(y_true) == 0 or k <= 0:
        return 0.0
    order = np.argsort(-probabilities)[: min(k, len(y_true))]
    return float(y_true[order].mean())


def safe_metric(metric_fn, y_true: np.ndarray, y_score: np.ndarray) -> Optional[float]:
    try:
        return float(metric_fn(y_true, y_score))
    except ValueError:
        return None


def serialize_config(config: TrainConfig) -> Dict[str, object]:
    payload = asdict(config)
    payload["data_dir"] = str(config.data_dir)
    payload["comments_zip"] = str(config.comments_zip)
    payload["output_dir"] = str(config.output_dir)
    return payload


if __name__ == "__main__":
    main()

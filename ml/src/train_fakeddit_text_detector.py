import argparse
import hashlib
import json
import os
import pickle
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, Iterable, List, Optional, Tuple

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


REQUIRED_COLUMNS = {
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


@dataclass
class TrainConfig:
    data_dir: Path
    output_dir: Path
    model_version: str
    positive_label: int
    max_train_rows: int
    max_eval_rows: int
    max_features: int
    min_df: int
    random_seed: int


def main() -> None:
    args = parse_args()
    config = TrainConfig(
        data_dir=args.data_dir,
        output_dir=args.output_dir,
        model_version=args.model_version,
        positive_label=args.positive_label,
        max_train_rows=args.max_train_rows,
        max_eval_rows=args.max_eval_rows,
        max_features=args.max_features,
        min_df=args.min_df,
        random_seed=args.random_seed,
    )
    config.output_dir.mkdir(parents=True, exist_ok=True)

    train_df = load_split(config.data_dir / "all_train.tsv", config.positive_label)
    valid_df = load_split(config.data_dir / "all_validate.tsv", config.positive_label)
    test_df = load_split(config.data_dir / "all_test_public.tsv", config.positive_label)

    train_df = sample_frame(train_df, config.max_train_rows, config.random_seed, balanced=True)
    valid_df = sample_frame(valid_df, config.max_eval_rows, config.random_seed, balanced=False)
    test_df = sample_frame(test_df, config.max_eval_rows, config.random_seed, balanced=False)

    if train_df.empty:
        raise RuntimeError("No train rows available after filtering")

    pipeline = make_pipeline(config)
    pipeline.fit(train_df["text"], train_df["label"])

    metrics = {
        "model_version": config.model_version,
        "model_kind": "tfidf_word_char_logreg",
        "task": "Fakeddit 2-way fake/suspicious content detection",
        "positive_label": config.positive_label,
        "positive_label_meaning": "suspicious/fake according to Fakeddit 2_way_label mapping",
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
    metrics["model_hash"] = compute_model_hash(metrics)

    model_path = config.output_dir / "fakeddit_text_detector_model.pkl"
    metrics_path = config.output_dir / "fakeddit_text_detector_metrics.json"
    validate_predictions_path = config.output_dir / "fakeddit_validate_predictions.csv"
    test_predictions_path = config.output_dir / "fakeddit_test_public_predictions.csv"
    top_validate_path = config.output_dir / "fakeddit_top_validate_cases.json"

    bundle = {
        "pipeline": pipeline,
        "model_version": config.model_version,
        "model_hash": metrics["model_hash"],
        "positive_label": config.positive_label,
        "metrics": metrics,
        "trained_at": datetime.now(timezone.utc).isoformat(),
    }
    with model_path.open("wb") as fh:
        pickle.dump(bundle, fh)
    metrics_path.write_text(json.dumps(metrics, indent=2, ensure_ascii=False), encoding="utf-8")

    write_predictions(pipeline, valid_df, validate_predictions_path)
    write_predictions(pipeline, test_df, test_predictions_path)
    top_validate_path.write_text(
        json.dumps(top_cases(pipeline, valid_df, 50), indent=2, ensure_ascii=False),
        encoding="utf-8",
    )

    print(
        json.dumps(
            {
                "model_path": str(model_path),
                "metrics_path": str(metrics_path),
                "validate_predictions_path": str(validate_predictions_path),
                "test_predictions_path": str(test_predictions_path),
                "top_validate_path": str(top_validate_path),
                "validate": metrics["splits"]["validate"],
                "test_public": metrics["splits"]["test_public"],
            },
            indent=2,
            ensure_ascii=False,
        )
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Train a Fakeddit title-text baseline detector")
    parser.add_argument(
        "--data-dir",
        type=Path,
        default=Path(os.getenv("FAKEDDIT_DATA_DIR", default_fakeddit_data_dir())),
        help="Directory with all_train.tsv/all_validate.tsv/all_test_public.tsv",
    )
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=Path(os.getenv("FAKEDDIT_MODEL_OUTPUT_DIR", default_output_dir())),
        help="Directory for model, metrics, and predictions",
    )
    parser.add_argument(
        "--model-version",
        default=os.getenv("FAKEDDIT_MODEL_VERSION", "fakeddit-tfidf-logreg-v1"),
    )
    parser.add_argument(
        "--positive-label",
        type=int,
        default=int(os.getenv("FAKEDDIT_POSITIVE_LABEL", "0")),
        help="Fakeddit 2_way_label value treated as suspicious/fake. Default: 0.",
    )
    parser.add_argument(
        "--max-train-rows",
        type=int,
        default=int(os.getenv("FAKEDDIT_MAX_TRAIN_ROWS", "100000")),
        help="Balanced training sample size. Use 0 for full train split.",
    )
    parser.add_argument(
        "--max-eval-rows",
        type=int,
        default=int(os.getenv("FAKEDDIT_MAX_EVAL_ROWS", "50000")),
        help="Evaluation sample size per split. Use 0 for full validation/test split.",
    )
    parser.add_argument(
        "--max-features",
        type=int,
        default=int(os.getenv("FAKEDDIT_MAX_FEATURES", "120000")),
    )
    parser.add_argument(
        "--min-df",
        type=int,
        default=int(os.getenv("FAKEDDIT_MIN_DF", "2")),
    )
    parser.add_argument(
        "--random-seed",
        type=int,
        default=int(os.getenv("FAKEDDIT_RANDOM_SEED", "42")),
    )
    return parser.parse_args()


def default_fakeddit_data_dir() -> str:
    workspace_root = Path(__file__).resolve().parents[3]
    return str(workspace_root / "datasets/raw/fakeddit/text_metadata/all_samples (also includes non multimodal)")


def default_output_dir() -> str:
    workspace_root = Path(__file__).resolve().parents[3]
    return str(workspace_root / "evaluation_outputs/fakeddit_text_detector")


def load_split(path: Path, positive_label: int) -> pd.DataFrame:
    if not path.exists():
        raise FileNotFoundError(f"Fakeddit split not found: {path}")

    df = pd.read_csv(
        path,
        sep="\t",
        usecols=lambda column: column in REQUIRED_COLUMNS,
        dtype="string",
        on_bad_lines="skip",
    )
    missing = REQUIRED_COLUMNS.difference(df.columns)
    if missing:
        raise RuntimeError(f"{path} is missing required columns: {sorted(missing)}")

    df["text"] = df["clean_title"].fillna("").str.strip()
    empty_text = df["text"] == ""
    df.loc[empty_text, "text"] = df.loc[empty_text, "title"].fillna("").str.strip()
    df["label_raw"] = pd.to_numeric(df["2_way_label"], errors="coerce")
    df = df[df["label_raw"].notna()]
    df["label_raw"] = df["label_raw"].astype(int)
    df = df[df["text"].str.len() > 0]
    df["label"] = (df["label_raw"] == positive_label).astype(int)

    return df[
        [
            "id",
            "text",
            "label",
            "label_raw",
            "subreddit",
            "score",
            "num_comments",
            "upvote_ratio",
            "hasImage",
        ]
    ].reset_index(drop=True)


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
            sampled = pd.concat(
                [sampled, rest.sample(n=min(remainder, len(rest)), random_state=random_seed)]
            )
    return sampled.sample(frac=1.0, random_state=random_seed).reset_index(drop=True)


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
    probabilities = pipeline.predict_proba(df["text"])[:, 1]
    predictions = (probabilities >= 0.5).astype(int)
    tn, fp, fn, tp = confusion_matrix(y_true, predictions, labels=[0, 1]).ravel()
    return {
        "rows": int(len(df)),
        "positive_rows": int(y_true.sum()),
        "positive_rate": float(y_true.mean()),
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
    if df.empty:
        pd.DataFrame().to_csv(path, index=False)
        return
    output = df[["id", "label", "label_raw", "subreddit", "text"]].copy()
    output["probability"] = pipeline.predict_proba(df["text"])[:, 1]
    output["predicted_label"] = (output["probability"] >= 0.5).astype(int)
    output.sort_values("probability", ascending=False).to_csv(path, index=False)


def top_cases(pipeline: Pipeline, df: pd.DataFrame, limit: int) -> List[Dict[str, object]]:
    if df.empty:
        return []
    probabilities = pipeline.predict_proba(df["text"])[:, 1]
    preview = df[["id", "label", "label_raw", "subreddit", "text"]].copy()
    preview["probability"] = probabilities
    preview = preview.sort_values("probability", ascending=False).head(limit)
    records = []
    for row in preview.to_dict(orient="records"):
        text = str(row.get("text") or "")
        row["text"] = text[:240]
        row["probability"] = float(row["probability"])
        row["label"] = int(row["label"])
        row["label_raw"] = int(row["label_raw"])
        records.append(row)
    return records


def summarize_frame(df: pd.DataFrame) -> Dict[str, object]:
    if df.empty:
        return {"rows": 0}
    return {
        "rows": int(len(df)),
        "positive_rows": int(df["label"].sum()),
        "positive_rate": float(df["label"].mean()),
        "raw_label_counts": {str(k): int(v) for k, v in df["label_raw"].value_counts().sort_index().items()},
        "subreddit_count": int(df["subreddit"].fillna("").nunique()),
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
    payload["output_dir"] = str(config.output_dir)
    return payload


def compute_model_hash(metrics: Dict[str, object]) -> str:
    payload = json.dumps(
        {
            "model_version": metrics["model_version"],
            "model_kind": metrics["model_kind"],
            "config": metrics["config"],
            "dataset_summary": metrics["dataset_summary"],
        },
        sort_keys=True,
        ensure_ascii=False,
    )
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


if __name__ == "__main__":
    main()

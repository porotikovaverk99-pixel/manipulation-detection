import argparse
import csv
import io
import json
import os
import sys
import time
import zipfile
from collections import defaultdict
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Optional, Tuple

# The host can expose both the discrete RX 7800 XT and an integrated AMD GPU.
# Keep Hugging Face Trainer on the first visible ROCm device by default.
if os.getenv("FAKEDDIT_FORCE_SINGLE_GPU", "1") == "1":
    os.environ.setdefault("HIP_VISIBLE_DEVICES", "0")
    os.environ.setdefault("CUDA_VISIBLE_DEVICES", "0")
    os.environ.setdefault("ROCR_VISIBLE_DEVICES", "0")

import numpy as np
import pandas as pd
import torch
from sklearn.metrics import (
    accuracy_score,
    average_precision_score,
    confusion_matrix,
    f1_score,
    precision_score,
    recall_score,
    roc_auc_score,
)
from transformers import (
    AutoModelForSequenceClassification,
    AutoTokenizer,
    DataCollatorWithPadding,
    Trainer,
    TrainingArguments,
    set_seed,
)


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
    model_name: str
    model_version: str
    positive_label: int
    max_train_rows: int
    max_eval_rows: int
    top_comments: int
    max_comment_chars: int
    comment_chunksize: int
    max_length: int
    epochs: float
    batch_size: int
    eval_batch_size: int
    learning_rate: float
    weight_decay: float
    warmup_ratio: float
    logging_steps: int
    save_checkpoints: bool
    save_steps: int
    save_total_limit: int
    random_seed: int
    fp16: bool
    gradient_checkpointing: bool
    save_final_model: bool


class FakedditTitleCommentsDataset(torch.utils.data.Dataset):
    def __init__(self, frame: pd.DataFrame, tokenizer: AutoTokenizer, max_length: int) -> None:
        self.ids = frame["id"].astype(str).tolist()
        self.texts = frame["text_with_comments"].astype(str).tolist()
        self.labels = frame["label"].astype(int).tolist()
        self.label_raw = frame["label_raw"].astype(int).tolist()
        self.tokenizer = tokenizer
        self.max_length = max_length

    def __len__(self) -> int:
        return len(self.labels)

    def __getitem__(self, index: int) -> Dict[str, object]:
        encoded = self.tokenizer(
            self.texts[index],
            truncation=True,
            max_length=self.max_length,
        )
        encoded["labels"] = self.labels[index]
        return encoded


def main() -> None:
    args = parse_args()
    config = TrainConfig(
        data_dir=args.data_dir,
        comments_zip=args.comments_zip,
        output_dir=args.output_dir,
        model_name=args.model_name,
        model_version=args.model_version,
        positive_label=args.positive_label,
        max_train_rows=args.max_train_rows,
        max_eval_rows=args.max_eval_rows,
        top_comments=args.top_comments,
        max_comment_chars=args.max_comment_chars,
        comment_chunksize=args.comment_chunksize,
        max_length=args.max_length,
        epochs=args.epochs,
        batch_size=args.batch_size,
        eval_batch_size=args.eval_batch_size,
        learning_rate=args.learning_rate,
        weight_decay=args.weight_decay,
        warmup_ratio=args.warmup_ratio,
        logging_steps=args.logging_steps,
        save_checkpoints=args.save_checkpoints,
        save_steps=args.save_steps,
        save_total_limit=args.save_total_limit,
        random_seed=args.random_seed,
        fp16=args.fp16,
        gradient_checkpointing=args.gradient_checkpointing,
        save_final_model=args.save_final_model,
    )
    config.output_dir.mkdir(parents=True, exist_ok=True)
    set_seed(config.random_seed)

    device_info = device_summary()
    print(json.dumps({"event": "device", **device_info}, ensure_ascii=False), flush=True)

    train_df = sample_frame(
        load_split(config.data_dir / "all_train.tsv", config.positive_label),
        config.max_train_rows,
        config.random_seed,
        balanced=True,
    )
    valid_df = sample_frame(
        load_split(config.data_dir / "all_validate.tsv", config.positive_label),
        config.max_eval_rows,
        config.random_seed,
        balanced=False,
    )
    test_df = sample_frame(
        load_split(config.data_dir / "all_test_public.tsv", config.positive_label),
        config.max_eval_rows,
        config.random_seed,
        balanced=False,
    )

    all_ids = set(train_df["id"]).union(valid_df["id"]).union(test_df["id"])
    comments_by_id = load_top_comments(
        config.comments_zip,
        all_ids,
        config.top_comments,
        config.max_comment_chars,
        config.comment_chunksize,
    )

    train_df = attach_comments(train_df, comments_by_id)
    valid_df = attach_comments(valid_df, comments_by_id)
    test_df = attach_comments(test_df, comments_by_id)

    tokenizer = AutoTokenizer.from_pretrained(config.model_name, use_fast=True)
    model = AutoModelForSequenceClassification.from_pretrained(config.model_name, num_labels=2)
    if config.gradient_checkpointing:
        model.gradient_checkpointing_enable()
        model.config.use_cache = False

    train_dataset = FakedditTitleCommentsDataset(train_df, tokenizer, config.max_length)
    valid_dataset = FakedditTitleCommentsDataset(valid_df, tokenizer, config.max_length)
    test_dataset = FakedditTitleCommentsDataset(test_df, tokenizer, config.max_length)

    trainer = Trainer(
        model=model,
        args=training_args(config),
        train_dataset=train_dataset,
        eval_dataset=valid_dataset,
        tokenizer=tokenizer,
        data_collator=DataCollatorWithPadding(tokenizer=tokenizer),
        compute_metrics=compute_metrics,
    )

    started = time.perf_counter()
    train_result = trainer.train()
    training_seconds = time.perf_counter() - started

    validate_metrics = prefixed_to_plain(trainer.evaluate(valid_dataset, metric_key_prefix="validate"))
    test_metrics = prefixed_to_plain(trainer.evaluate(test_dataset, metric_key_prefix="test_public"))

    model_dir: Optional[Path] = None
    if config.save_final_model:
        model_dir = config.output_dir / "model"
        trainer.save_model(str(model_dir))
        tokenizer.save_pretrained(str(model_dir))

    metrics = {
        "model_version": config.model_version,
        "model_kind": "transformer_sequence_classifier_title_comments",
        "base_model": config.model_name,
        "task": "Fakeddit 2-way fake/suspicious title+comments detection",
        "positive_label": config.positive_label,
        "config": serialize_config(config),
        "device": device_info,
        "dataset_summary": {
            "train": summarize_frame(train_df),
            "validate": summarize_frame(valid_df),
            "test_public": summarize_frame(test_df),
        },
        "training_seconds": training_seconds,
        "train_result": train_result.metrics,
        "splits": {
            "validate": validate_metrics,
            "test_public": test_metrics,
        },
        "model_dir": str(model_dir) if model_dir else None,
        "generated_at": datetime.now(timezone.utc).isoformat(),
    }

    metrics_path = config.output_dir / "fakeddit_comments_transformer_metrics.json"
    validate_predictions_path = config.output_dir / "fakeddit_comments_transformer_validate_predictions.csv"
    test_predictions_path = config.output_dir / "fakeddit_comments_transformer_test_public_predictions.csv"
    text_preview_path = config.output_dir / "fakeddit_comments_transformer_text_preview.csv"

    metrics_path.write_text(json.dumps(metrics, indent=2, ensure_ascii=False), encoding="utf-8")
    write_predictions(trainer, valid_dataset, valid_df, validate_predictions_path)
    write_predictions(trainer, test_dataset, test_df, test_predictions_path)
    pd.concat([train_df, valid_df, test_df]).head(100)[
        ["id", "label", "label_raw", "title_text", "comment_count_used", "text_with_comments"]
    ].to_csv(text_preview_path, index=False)

    print(
        json.dumps(
            {
                "metrics_path": str(metrics_path),
                "validate_predictions_path": str(validate_predictions_path),
                "test_predictions_path": str(test_predictions_path),
                "text_preview_path": str(text_preview_path),
                "model_dir": str(model_dir) if model_dir else None,
                "training_seconds": training_seconds,
                "device": device_info,
                "validate": validate_metrics,
                "test_public": test_metrics,
            },
            indent=2,
            ensure_ascii=False,
        ),
        flush=True,
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Fine-tune a transformer on Fakeddit title+comments text")
    parser.add_argument("--data-dir", type=Path, default=Path(os.getenv("FAKEDDIT_DATA_DIR", default_fakeddit_data_dir())))
    parser.add_argument("--comments-zip", type=Path, default=Path(os.getenv("FAKEDDIT_COMMENTS_ZIP", default_comments_zip())))
    parser.add_argument("--output-dir", type=Path, default=Path(os.getenv("FAKEDDIT_COMMENTS_TRANSFORMER_OUTPUT_DIR", default_output_dir())))
    parser.add_argument("--model-name", default=os.getenv("FAKEDDIT_COMMENTS_TRANSFORMER_MODEL_NAME", "microsoft/deberta-v3-base"))
    parser.add_argument("--model-version", default=os.getenv("FAKEDDIT_COMMENTS_TRANSFORMER_MODEL_VERSION", "fakeddit-deberta-v3-base-title-comments-v1"))
    parser.add_argument("--positive-label", type=int, default=int(os.getenv("FAKEDDIT_POSITIVE_LABEL", "0")))
    parser.add_argument("--max-train-rows", type=int, default=int(os.getenv("FAKEDDIT_MAX_TRAIN_ROWS", "50000")))
    parser.add_argument("--max-eval-rows", type=int, default=int(os.getenv("FAKEDDIT_MAX_EVAL_ROWS", "10000")))
    parser.add_argument("--top-comments", type=int, default=int(os.getenv("FAKEDDIT_TOP_COMMENTS", "3")))
    parser.add_argument("--max-comment-chars", type=int, default=int(os.getenv("FAKEDDIT_MAX_COMMENT_CHARS", "500")))
    parser.add_argument("--comment-chunksize", type=int, default=int(os.getenv("FAKEDDIT_COMMENT_CHUNKSIZE", "200000")))
    parser.add_argument("--max-length", type=int, default=int(os.getenv("FAKEDDIT_MAX_LENGTH", "256")))
    parser.add_argument("--epochs", type=float, default=float(os.getenv("FAKEDDIT_EPOCHS", "3")))
    parser.add_argument("--batch-size", type=int, default=int(os.getenv("FAKEDDIT_BATCH_SIZE", "8")))
    parser.add_argument("--eval-batch-size", type=int, default=int(os.getenv("FAKEDDIT_EVAL_BATCH_SIZE", "16")))
    parser.add_argument("--learning-rate", type=float, default=float(os.getenv("FAKEDDIT_LEARNING_RATE", "2e-5")))
    parser.add_argument("--weight-decay", type=float, default=float(os.getenv("FAKEDDIT_WEIGHT_DECAY", "0.01")))
    parser.add_argument("--warmup-ratio", type=float, default=float(os.getenv("FAKEDDIT_WARMUP_RATIO", "0.06")))
    parser.add_argument("--logging-steps", type=int, default=int(os.getenv("FAKEDDIT_LOGGING_STEPS", "500")))
    parser.add_argument(
        "--save-checkpoints",
        action="store_true",
        default=os.getenv("FAKEDDIT_SAVE_CHECKPOINTS", "0") == "1",
        help="Save periodic Hugging Face checkpoints during long training runs.",
    )
    parser.add_argument("--save-steps", type=int, default=int(os.getenv("FAKEDDIT_SAVE_STEPS", "1000")))
    parser.add_argument("--save-total-limit", type=int, default=int(os.getenv("FAKEDDIT_SAVE_TOTAL_LIMIT", "2")))
    parser.add_argument("--random-seed", type=int, default=int(os.getenv("FAKEDDIT_RANDOM_SEED", "42")))
    parser.add_argument("--fp16", action="store_true", default=os.getenv("FAKEDDIT_FP16", "0") == "1")
    parser.add_argument(
        "--gradient-checkpointing",
        action=argparse.BooleanOptionalAction,
        default=os.getenv("FAKEDDIT_GRADIENT_CHECKPOINTING", "0") == "1",
        help="Enable transformer gradient checkpointing. Disabled by default because it can fail on some ROCm builds.",
    )
    parser.add_argument("--save-final-model", action="store_true", default=os.getenv("FAKEDDIT_SAVE_FINAL_MODEL", "0") == "1")
    return parser.parse_args()


def default_fakeddit_data_dir() -> str:
    workspace_root = Path(__file__).resolve().parents[3]
    return str(workspace_root / "datasets/raw/fakeddit/text_metadata/all_samples (also includes non multimodal)")


def default_comments_zip() -> str:
    workspace_root = Path(__file__).resolve().parents[3]
    return str(workspace_root / "datasets/raw/fakeddit/comments/all_comments.tsv.zip")


def default_output_dir() -> str:
    workspace_root = Path(__file__).resolve().parents[3]
    return str(workspace_root / "evaluation_outputs/fakeddit_comments_transformer_detector")


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
    return df[
        [
            "id",
            "title_text",
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


def training_args(config: TrainConfig) -> TrainingArguments:
    return TrainingArguments(
        output_dir=str(config.output_dir / "hf_checkpoints"),
        overwrite_output_dir=True,
        num_train_epochs=config.epochs,
        per_device_train_batch_size=config.batch_size,
        per_device_eval_batch_size=config.eval_batch_size,
        learning_rate=config.learning_rate,
        weight_decay=config.weight_decay,
        warmup_ratio=config.warmup_ratio,
        eval_strategy="epoch",
        save_strategy="steps" if config.save_checkpoints else "no",
        save_steps=config.save_steps,
        save_total_limit=config.save_total_limit,
        logging_strategy="steps",
        logging_steps=config.logging_steps,
        report_to=[],
        seed=config.random_seed,
        fp16=config.fp16,
        dataloader_num_workers=0,
        disable_tqdm=True,
    )


def compute_metrics(eval_prediction) -> Dict[str, float]:
    logits, labels = eval_prediction
    probabilities = softmax(logits)[:, 1]
    predictions = (probabilities >= 0.5).astype(int)
    return {
        "accuracy": float(accuracy_score(labels, predictions)),
        "precision": float(precision_score(labels, predictions, zero_division=0)),
        "recall": float(recall_score(labels, predictions, zero_division=0)),
        "f1": float(f1_score(labels, predictions, zero_division=0)),
        "roc_auc": safe_metric(roc_auc_score, labels, probabilities),
        "pr_auc": safe_metric(average_precision_score, labels, probabilities),
        "precision_at_100": precision_at_k(labels, probabilities, 100),
        "precision_at_500": precision_at_k(labels, probabilities, 500),
    }


def write_predictions(
    trainer: Trainer,
    dataset: FakedditTitleCommentsDataset,
    frame: pd.DataFrame,
    path: Path,
) -> None:
    prediction = trainer.predict(dataset)
    probabilities = softmax(prediction.predictions)[:, 1]
    output = frame[["id", "label", "label_raw", "subreddit", "title_text", "comment_count_used"]].copy()
    output["probability"] = probabilities
    output["predicted_label"] = (output["probability"] >= 0.5).astype(int)
    output.sort_values("probability", ascending=False).to_csv(path, index=False)


def softmax(logits: np.ndarray) -> np.ndarray:
    shifted = logits - np.max(logits, axis=1, keepdims=True)
    exp = np.exp(shifted)
    return exp / np.sum(exp, axis=1, keepdims=True)


def precision_at_k(y_true: np.ndarray, probabilities: np.ndarray, k: int) -> float:
    if len(y_true) == 0 or k <= 0:
        return 0.0
    order = np.argsort(-probabilities)[: min(k, len(y_true))]
    return float(np.asarray(y_true)[order].mean())


def safe_metric(metric_fn, y_true: np.ndarray, y_score: np.ndarray) -> Optional[float]:
    try:
        return float(metric_fn(y_true, y_score))
    except ValueError:
        return None


def prefixed_to_plain(metrics: Dict[str, float]) -> Dict[str, object]:
    plain: Dict[str, object] = {}
    for key, value in metrics.items():
        name = key
        for prefix in ["validate_", "test_public_", "eval_", "test_"]:
            if key.startswith(prefix):
                name = key[len(prefix) :]
                break
        plain[name] = float(value) if isinstance(value, (int, float, np.number)) else value
    return plain


def summarize_frame(df: pd.DataFrame) -> Dict[str, object]:
    if df.empty:
        return {"rows": 0}
    y_true = df["label"].to_numpy()
    comment_coverage_rate = float((df["comment_count_used"] > 0).mean())
    return {
        "rows": int(len(df)),
        "positive_rows": int(df["label"].sum()),
        "positive_rate": float(df["label"].mean()),
        "raw_label_counts": {str(k): int(v) for k, v in df["label_raw"].value_counts().sort_index().items()},
        "subreddit_count": int(df["subreddit"].fillna("").nunique()),
        "comment_coverage_rate": comment_coverage_rate,
        "mean_comment_count_used": float(df["comment_count_used"].mean()),
        "class_balance": {"negative": int((y_true == 0).sum()), "positive": int((y_true == 1).sum())},
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


def device_summary() -> Dict[str, object]:
    summary: Dict[str, object] = {
        "torch_version": torch.__version__,
        "hip_version": getattr(torch.version, "hip", None),
        "cuda_available": bool(torch.cuda.is_available()),
        "device_count": int(torch.cuda.device_count()),
    }
    if torch.cuda.is_available():
        summary["device_name"] = torch.cuda.get_device_name(0)
    return summary


def serialize_config(config: TrainConfig) -> Dict[str, object]:
    payload = asdict(config)
    payload["data_dir"] = str(config.data_dir)
    payload["comments_zip"] = str(config.comments_zip)
    payload["output_dir"] = str(config.output_dir)
    return payload


if __name__ == "__main__":
    main()

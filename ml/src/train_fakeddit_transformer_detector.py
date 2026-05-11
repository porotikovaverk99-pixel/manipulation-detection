import argparse
import json
import os
import time
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, Optional

# The host exposes both the RX 7800 XT and an integrated AMD device through ROCm.
# Hugging Face Trainer wraps the model in DataParallel when it sees 2 devices,
# which fails on this mixed setup. Default to the discrete GPU unless the caller
# explicitly provides visibility settings.
if os.getenv("FAKEDDIT_FORCE_SINGLE_GPU", "1") == "1":
    os.environ.setdefault("HIP_VISIBLE_DEVICES", "0")
    os.environ.setdefault("CUDA_VISIBLE_DEVICES", "0")
    os.environ.setdefault("ROCR_VISIBLE_DEVICES", "0")

import numpy as np
import pandas as pd
import torch
from sklearn.metrics import accuracy_score, average_precision_score, f1_score, precision_score, recall_score, roc_auc_score
from transformers import (
    AutoModelForSequenceClassification,
    AutoTokenizer,
    DataCollatorWithPadding,
    Trainer,
    TrainingArguments,
    set_seed,
)


REQUIRED_COLUMNS = {"id", "clean_title", "title", "subreddit", "2_way_label"}


@dataclass
class TrainConfig:
    data_dir: Path
    output_dir: Path
    model_name: str
    model_version: str
    positive_label: int
    max_train_rows: int
    max_eval_rows: int
    max_length: int
    epochs: float
    batch_size: int
    learning_rate: float
    weight_decay: float
    warmup_ratio: float
    random_seed: int
    fp16: bool


class FakedditTitleDataset(torch.utils.data.Dataset):
    def __init__(self, frame: pd.DataFrame, tokenizer: AutoTokenizer, max_length: int) -> None:
        self.ids = frame["id"].astype(str).tolist()
        self.texts = frame["text"].astype(str).tolist()
        self.labels = frame["label"].astype(int).tolist()
        self.label_raw = frame["label_raw"].astype(int).tolist()
        self.tokenizer = tokenizer
        self.max_length = max_length

    def __len__(self) -> int:
        return len(self.labels)

    def __getitem__(self, index: int) -> Dict[str, torch.Tensor | int | str]:
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
        output_dir=args.output_dir,
        model_name=args.model_name,
        model_version=args.model_version,
        positive_label=args.positive_label,
        max_train_rows=args.max_train_rows,
        max_eval_rows=args.max_eval_rows,
        max_length=args.max_length,
        epochs=args.epochs,
        batch_size=args.batch_size,
        learning_rate=args.learning_rate,
        weight_decay=args.weight_decay,
        warmup_ratio=args.warmup_ratio,
        random_seed=args.random_seed,
        fp16=args.fp16,
    )
    config.output_dir.mkdir(parents=True, exist_ok=True)
    set_seed(config.random_seed)

    device_info = device_summary()
    print(json.dumps({"event": "device", **device_info}, ensure_ascii=False), flush=True)

    train_df = sample_frame(load_split(config.data_dir / "all_train.tsv", config.positive_label), config.max_train_rows, config.random_seed, balanced=True)
    valid_df = sample_frame(load_split(config.data_dir / "all_validate.tsv", config.positive_label), config.max_eval_rows, config.random_seed, balanced=False)
    test_df = sample_frame(load_split(config.data_dir / "all_test_public.tsv", config.positive_label), config.max_eval_rows, config.random_seed, balanced=False)

    tokenizer = AutoTokenizer.from_pretrained(config.model_name, use_fast=True)
    model = AutoModelForSequenceClassification.from_pretrained(config.model_name, num_labels=2)

    train_dataset = FakedditTitleDataset(train_df, tokenizer, config.max_length)
    valid_dataset = FakedditTitleDataset(valid_df, tokenizer, config.max_length)
    test_dataset = FakedditTitleDataset(test_df, tokenizer, config.max_length)

    training_args = TrainingArguments(
        output_dir=str(config.output_dir / "hf_checkpoints"),
        overwrite_output_dir=True,
        num_train_epochs=config.epochs,
        per_device_train_batch_size=config.batch_size,
        per_device_eval_batch_size=config.batch_size * 2,
        learning_rate=config.learning_rate,
        weight_decay=config.weight_decay,
        warmup_ratio=config.warmup_ratio,
        eval_strategy="epoch",
        save_strategy="no",
        logging_strategy="epoch",
        report_to=[],
        seed=config.random_seed,
        fp16=config.fp16,
        dataloader_num_workers=0,
    )

    trainer = Trainer(
        model=model,
        args=training_args,
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

    model_dir = config.output_dir / "model"
    trainer.save_model(str(model_dir))
    tokenizer.save_pretrained(str(model_dir))

    metrics = {
        "model_version": config.model_version,
        "model_kind": "transformer_sequence_classifier",
        "base_model": config.model_name,
        "task": "Fakeddit 2-way fake/suspicious title detection",
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
        "generated_at": datetime.now(timezone.utc).isoformat(),
    }

    metrics_path = config.output_dir / "fakeddit_transformer_metrics.json"
    metrics_path.write_text(json.dumps(metrics, indent=2, ensure_ascii=False), encoding="utf-8")

    print(
        json.dumps(
            {
                "model_dir": str(model_dir),
                "metrics_path": str(metrics_path),
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
    parser = argparse.ArgumentParser(description="Fine-tune a transformer on Fakeddit title text")
    parser.add_argument("--data-dir", type=Path, default=Path(os.getenv("FAKEDDIT_DATA_DIR", default_fakeddit_data_dir())))
    parser.add_argument("--output-dir", type=Path, default=Path(os.getenv("FAKEDDIT_TRANSFORMER_OUTPUT_DIR", default_output_dir())))
    parser.add_argument("--model-name", default=os.getenv("FAKEDDIT_TRANSFORMER_MODEL_NAME", "distilroberta-base"))
    parser.add_argument("--model-version", default=os.getenv("FAKEDDIT_TRANSFORMER_MODEL_VERSION", "fakeddit-transformer-v1"))
    parser.add_argument("--positive-label", type=int, default=int(os.getenv("FAKEDDIT_POSITIVE_LABEL", "0")))
    parser.add_argument("--max-train-rows", type=int, default=int(os.getenv("FAKEDDIT_MAX_TRAIN_ROWS", "20000")))
    parser.add_argument("--max-eval-rows", type=int, default=int(os.getenv("FAKEDDIT_MAX_EVAL_ROWS", "5000")))
    parser.add_argument("--max-length", type=int, default=int(os.getenv("FAKEDDIT_MAX_LENGTH", "96")))
    parser.add_argument("--epochs", type=float, default=float(os.getenv("FAKEDDIT_EPOCHS", "2")))
    parser.add_argument("--batch-size", type=int, default=int(os.getenv("FAKEDDIT_BATCH_SIZE", "32")))
    parser.add_argument("--learning-rate", type=float, default=float(os.getenv("FAKEDDIT_LEARNING_RATE", "2e-5")))
    parser.add_argument("--weight-decay", type=float, default=float(os.getenv("FAKEDDIT_WEIGHT_DECAY", "0.01")))
    parser.add_argument("--warmup-ratio", type=float, default=float(os.getenv("FAKEDDIT_WARMUP_RATIO", "0.06")))
    parser.add_argument("--random-seed", type=int, default=int(os.getenv("FAKEDDIT_RANDOM_SEED", "42")))
    parser.add_argument("--fp16", action="store_true", default=os.getenv("FAKEDDIT_FP16", "0") == "1")
    return parser.parse_args()


def default_fakeddit_data_dir() -> str:
    workspace_root = Path(__file__).resolve().parents[3]
    return str(workspace_root / "datasets/raw/fakeddit/text_metadata/all_samples (also includes non multimodal)")


def default_output_dir() -> str:
    workspace_root = Path(__file__).resolve().parents[3]
    return str(workspace_root / "evaluation_outputs/fakeddit_transformer_detector")


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
    return df[["id", "text", "label", "label_raw", "subreddit"]].reset_index(drop=True)


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


def prefixed_to_plain(metrics: Dict[str, float]) -> Dict[str, float]:
    plain = {}
    for key, value in metrics.items():
        name = key
        for prefix in ["validate_", "test_public_", "eval_", "test_"]:
            if key.startswith(prefix):
                name = key[len(prefix) :]
                break
        plain[name] = float(value) if isinstance(value, (int, float, np.number)) else value
    return plain


def summarize_frame(df: pd.DataFrame) -> Dict[str, object]:
    return {
        "rows": int(len(df)),
        "positive_rows": int(df["label"].sum()),
        "positive_rate": float(df["label"].mean()) if len(df) else 0.0,
        "raw_label_counts": {str(k): int(v) for k, v in df["label_raw"].value_counts().sort_index().items()},
    }


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
    payload["output_dir"] = str(config.output_dir)
    return payload


if __name__ == "__main__":
    main()

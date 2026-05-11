import argparse
import json
import os
import time
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Optional, Tuple

if os.getenv("PHEME_FORCE_SINGLE_GPU", "1") == "1":
    os.environ.setdefault("HIP_VISIBLE_DEVICES", "0")
    os.environ.setdefault("CUDA_VISIBLE_DEVICES", "0")
    os.environ.setdefault("ROCR_VISIBLE_DEVICES", "0")

import numpy as np
import pandas as pd
import psycopg2
import torch
from psycopg2.extras import RealDictCursor
from sklearn.metrics import accuracy_score, average_precision_score, f1_score, precision_score, recall_score, roc_auc_score
from sklearn.model_selection import LeaveOneGroupOut
from transformers import (
    AutoModelForSequenceClassification,
    AutoTokenizer,
    DataCollatorWithPadding,
    Trainer,
    TrainingArguments,
    set_seed,
)

from train_case_detector import find_best_threshold, normalize_label, precision_at_k, safe_metric


@dataclass
class TrainConfig:
    database_url: str
    output_dir: Path
    model_name: str
    model_version: str
    dataset_name: str
    dataset_splits: Tuple[str, ...]
    source_name: str
    positive_labels: Tuple[str, ...]
    text_mode: str
    max_reactions: int
    max_reaction_chars: int
    max_length: int
    epochs: float
    batch_size: int
    learning_rate: float
    weight_decay: float
    warmup_ratio: float
    random_seed: int
    fp16: bool
    only_event: str
    fold_limit: int
    save_final_model: bool


class PhemeCaseTextDataset(torch.utils.data.Dataset):
    def __init__(self, frame: pd.DataFrame, tokenizer: AutoTokenizer, max_length: int) -> None:
        self.case_ids = frame["case_id"].astype(int).tolist()
        self.texts = frame["text"].astype(str).tolist()
        self.labels = frame["label"].astype(int).tolist()
        self.tokenizer = tokenizer
        self.max_length = max_length

    def __len__(self) -> int:
        return len(self.labels)

    def __getitem__(self, index: int) -> Dict[str, torch.Tensor | int]:
        encoded = self.tokenizer(self.texts[index], truncation=True, max_length=self.max_length)
        encoded["labels"] = self.labels[index]
        return encoded


def main() -> None:
    args = parse_args()
    config = TrainConfig(
        database_url=args.database_url,
        output_dir=args.output_dir,
        model_name=args.model_name,
        model_version=args.model_version,
        dataset_name=args.dataset_name,
        dataset_splits=tuple(item.strip() for item in args.dataset_splits.split(",") if item.strip()),
        source_name=args.source_name,
        positive_labels=tuple(normalize_label(item) for item in args.positive_labels.split(",") if item.strip()),
        text_mode=args.text_mode,
        max_reactions=args.max_reactions,
        max_reaction_chars=args.max_reaction_chars,
        max_length=args.max_length,
        epochs=args.epochs,
        batch_size=args.batch_size,
        learning_rate=args.learning_rate,
        weight_decay=args.weight_decay,
        warmup_ratio=args.warmup_ratio,
        random_seed=args.random_seed,
        fp16=args.fp16,
        only_event=args.only_event,
        fold_limit=args.fold_limit,
        save_final_model=args.save_final_model,
    )
    if not config.database_url:
        raise RuntimeError("--database-url or DATABASE_URL is required")
    config.output_dir.mkdir(parents=True, exist_ok=True)
    set_seed(config.random_seed)

    device_info = device_summary()
    print(json.dumps({"event": "device", **device_info}, ensure_ascii=False), flush=True)

    frame = load_case_text_frame(config)
    if frame.empty:
        raise RuntimeError("no PHEME case text rows available")
    if frame["event_name"].nunique() < 2:
        raise RuntimeError("need at least 2 distinct event_name values for event-based evaluation")

    tokenizer = AutoTokenizer.from_pretrained(config.model_name, use_fast=True)
    groups = frame["event_name"]
    labels = frame["label"].to_numpy()
    oof_prob = np.full(len(frame), np.nan, dtype=float)
    fold_metrics = []
    folds_run = 0
    started = time.perf_counter()

    for fold_index, (train_idx, test_idx) in enumerate(LeaveOneGroupOut().split(frame, labels, groups), start=1):
        test_event = str(groups.iloc[test_idx].iloc[0])
        if config.only_event and test_event != config.only_event:
            continue
        if config.fold_limit > 0 and folds_run >= config.fold_limit:
            break
        folds_run += 1

        print(json.dumps({"event": "fold_start", "fold": fold_index, "test_event": test_event}), flush=True)
        model = AutoModelForSequenceClassification.from_pretrained(config.model_name, num_labels=2)
        train_dataset = PhemeCaseTextDataset(frame.iloc[train_idx], tokenizer, config.max_length)
        test_dataset = PhemeCaseTextDataset(frame.iloc[test_idx], tokenizer, config.max_length)

        trainer = Trainer(
            model=model,
            args=training_args(config, fold_index),
            train_dataset=train_dataset,
            tokenizer=tokenizer,
            data_collator=DataCollatorWithPadding(tokenizer=tokenizer),
        )
        train_result = trainer.train()
        prediction = trainer.predict(test_dataset)
        probabilities = softmax(prediction.predictions)[:, 1]
        predictions = (probabilities >= 0.5).astype(int)
        oof_prob[test_idx] = probabilities

        y_test = labels[test_idx]
        fold_metrics.append(
            {
                "fold": fold_index,
                "test_event": test_event,
                "rows": int(len(test_idx)),
                "positive_rows": int(y_test.sum()),
                "precision": float(precision_score(y_test, predictions, zero_division=0)),
                "recall": float(recall_score(y_test, predictions, zero_division=0)),
                "f1": float(f1_score(y_test, predictions, zero_division=0)),
                "roc_auc": safe_metric(roc_auc_score, y_test, probabilities),
                "pr_auc": safe_metric(average_precision_score, y_test, probabilities),
                "train_runtime": float(train_result.metrics.get("train_runtime", 0.0)),
            }
        )
        del trainer, model
        if torch.cuda.is_available():
            torch.cuda.empty_cache()

    valid_mask = ~np.isnan(oof_prob)
    if not valid_mask.any():
        raise RuntimeError("no folds were evaluated")
    y_valid = labels[valid_mask]
    prob_valid = oof_prob[valid_mask]
    best_threshold = find_best_threshold(y_valid, prob_valid)
    pred_valid = (prob_valid >= best_threshold).astype(int)
    evaluated_frame = frame.loc[valid_mask].reset_index(drop=True).copy()
    evaluated_frame["oof_probability"] = prob_valid
    evaluated_frame["predicted_label"] = pred_valid

    metrics = {
        "model_version": config.model_version,
        "model_kind": "pheme_case_text_transformer",
        "base_model": config.model_name,
        "dataset_name": config.dataset_name,
        "dataset_splits": list(config.dataset_splits),
        "source_name": config.source_name,
        "text_mode": config.text_mode,
        "rows": int(len(frame)),
        "evaluated_rows": int(valid_mask.sum()),
        "event_count": int(frame["event_name"].nunique()),
        "evaluated_events": sorted(evaluated_frame["event_name"].unique().tolist()),
        "positive_labels": list(config.positive_labels),
        "recommended_threshold": float(best_threshold),
        "precision_at_10": precision_at_k(evaluated_frame, prob_valid, 10),
        "precision_at_20": precision_at_k(evaluated_frame, prob_valid, 20),
        "precision": float(precision_score(y_valid, pred_valid, zero_division=0)),
        "recall": float(recall_score(y_valid, pred_valid, zero_division=0)),
        "f1": float(f1_score(y_valid, pred_valid, zero_division=0)),
        "roc_auc": safe_metric(roc_auc_score, y_valid, prob_valid),
        "pr_auc": safe_metric(average_precision_score, y_valid, prob_valid),
        "fold_metrics": fold_metrics,
        "dataset_summary": summarize_frame(frame),
        "device": device_info,
        "config": serialize_config(config),
        "training_seconds": time.perf_counter() - started,
        "generated_at": datetime.now(timezone.utc).isoformat(),
    }

    if config.save_final_model:
        model = AutoModelForSequenceClassification.from_pretrained(config.model_name, num_labels=2)
        final_trainer = Trainer(
            model=model,
            args=training_args(config, "final"),
            train_dataset=PhemeCaseTextDataset(frame, tokenizer, config.max_length),
            tokenizer=tokenizer,
            data_collator=DataCollatorWithPadding(tokenizer=tokenizer),
        )
        final_trainer.train()
        model_dir = config.output_dir / "model"
        final_trainer.save_model(str(model_dir))
        tokenizer.save_pretrained(str(model_dir))
        metrics["model_dir"] = str(model_dir)

    metrics_path = config.output_dir / "pheme_transformer_metrics.json"
    oof_path = config.output_dir / "pheme_transformer_oof_predictions.csv"
    text_preview_path = config.output_dir / "pheme_transformer_text_preview.csv"
    metrics_path.write_text(json.dumps(metrics, indent=2, ensure_ascii=False), encoding="utf-8")
    evaluated_frame[["case_id", "event_name", "label_name", "oof_probability", "predicted_label"]].to_csv(oof_path, index=False)
    frame[["case_id", "event_name", "label_name", "root_text", "reaction_count", "text"]].head(100).to_csv(
        text_preview_path,
        index=False,
    )

    print(
        json.dumps(
            {
                "metrics_path": str(metrics_path),
                "oof_path": str(oof_path),
                "text_preview_path": str(text_preview_path),
                "rows": metrics["rows"],
                "evaluated_rows": metrics["evaluated_rows"],
                "evaluated_events": metrics["evaluated_events"],
                "precision_at_10": metrics["precision_at_10"],
                "precision_at_20": metrics["precision_at_20"],
                "f1": metrics["f1"],
                "roc_auc": metrics["roc_auc"],
                "pr_auc": metrics["pr_auc"],
                "training_seconds": metrics["training_seconds"],
            },
            indent=2,
            ensure_ascii=False,
        ),
        flush=True,
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Fine-tune a transformer on PHEME case text")
    parser.add_argument("--database-url", default=os.getenv("DATABASE_URL") or os.getenv("DATABASE_URI") or "")
    parser.add_argument("--output-dir", type=Path, default=Path(os.getenv("PHEME_TRANSFORMER_OUTPUT_DIR", default_output_dir())))
    parser.add_argument("--model-name", default=os.getenv("PHEME_TRANSFORMER_MODEL_NAME", "distilroberta-base"))
    parser.add_argument("--model-version", default=os.getenv("PHEME_TRANSFORMER_MODEL_VERSION", "pheme-distilroberta-case-text-v1"))
    parser.add_argument("--dataset-name", default=os.getenv("PHEME_DATASET_NAME", "pheme"))
    parser.add_argument("--dataset-splits", default=os.getenv("PHEME_DATASET_SPLITS", "eventcv_large"))
    parser.add_argument("--source-name", default=os.getenv("PHEME_SOURCE_NAME", "pheme_large"))
    parser.add_argument("--positive-labels", default=os.getenv("PHEME_POSITIVE_LABELS", "rumour,rumor"))
    parser.add_argument("--text-mode", choices=["root_only", "root_reactions"], default=os.getenv("PHEME_TEXT_MODE", "root_reactions"))
    parser.add_argument("--max-reactions", type=int, default=int(os.getenv("PHEME_MAX_REACTIONS", "8")))
    parser.add_argument("--max-reaction-chars", type=int, default=int(os.getenv("PHEME_MAX_REACTION_CHARS", "220")))
    parser.add_argument("--max-length", type=int, default=int(os.getenv("PHEME_MAX_LENGTH", "192")))
    parser.add_argument("--epochs", type=float, default=float(os.getenv("PHEME_EPOCHS", "3")))
    parser.add_argument("--batch-size", type=int, default=int(os.getenv("PHEME_BATCH_SIZE", "16")))
    parser.add_argument("--learning-rate", type=float, default=float(os.getenv("PHEME_LEARNING_RATE", "2e-5")))
    parser.add_argument("--weight-decay", type=float, default=float(os.getenv("PHEME_WEIGHT_DECAY", "0.01")))
    parser.add_argument("--warmup-ratio", type=float, default=float(os.getenv("PHEME_WARMUP_RATIO", "0.06")))
    parser.add_argument("--random-seed", type=int, default=int(os.getenv("PHEME_RANDOM_SEED", "42")))
    parser.add_argument("--fp16", action="store_true", default=os.getenv("PHEME_FP16", "0") == "1")
    parser.add_argument("--only-event", default=os.getenv("PHEME_ONLY_EVENT", ""))
    parser.add_argument("--fold-limit", type=int, default=int(os.getenv("PHEME_FOLD_LIMIT", "0")))
    parser.add_argument("--save-final-model", action="store_true", default=os.getenv("PHEME_SAVE_FINAL_MODEL", "0") == "1")
    return parser.parse_args()


def default_output_dir() -> str:
    workspace_root = Path(__file__).resolve().parents[3]
    return str(workspace_root / "evaluation_outputs/pheme_transformer_detector")


def load_case_text_frame(config: TrainConfig) -> pd.DataFrame:
    cases_query = """
        SELECT id, event_name, label, title, external_case_id
        FROM cases
        WHERE dataset_name = %s
          AND source_name = %s
          AND COALESCE(label, '') <> ''
    """
    params: List[object] = [config.dataset_name, config.source_name]
    if config.dataset_splits:
        cases_query += " AND dataset_split = ANY(%s)"
        params.append(list(config.dataset_splits))
    cases_query += " ORDER BY event_name, id"

    with psycopg2.connect(config.database_url) as conn:
        with conn.cursor(cursor_factory=RealDictCursor) as cur:
            cur.execute(cases_query, params)
            cases = cur.fetchall()
            case_ids = [int(row["id"]) for row in cases]
            if not case_ids:
                return pd.DataFrame()
            cur.execute(
                """
                SELECT case_id, content, is_case_root, published_at, id
                FROM posts
                WHERE case_id = ANY(%s)
                  AND COALESCE(content, '') <> ''
                ORDER BY case_id, is_case_root DESC, published_at NULLS LAST, id
                """,
                (case_ids,),
            )
            posts = cur.fetchall()

    posts_by_case: Dict[int, List[Dict[str, object]]] = {}
    for post in posts:
        posts_by_case.setdefault(int(post["case_id"]), []).append(dict(post))

    records = []
    for row in cases:
        label_name = normalize_label(row.get("label") or "")
        if not label_name:
            continue
        case_id = int(row["id"])
        case_posts = posts_by_case.get(case_id, [])
        root_text = select_root_text(row, case_posts)
        if not root_text:
            continue
        reactions = select_reactions(case_posts, root_text, config)
        text = build_case_text(root_text, reactions, config.text_mode)
        records.append(
            {
                "case_id": case_id,
                "external_case_id": row.get("external_case_id") or "",
                "event_name": str(row.get("event_name") or ""),
                "label_name": label_name,
                "label": 1 if label_name in config.positive_labels else 0,
                "root_text": root_text,
                "reaction_count": len(reactions),
                "text": text,
            }
        )
    return pd.DataFrame(records).reset_index(drop=True)


def select_root_text(case_row: Dict[str, object], posts: List[Dict[str, object]]) -> str:
    for post in posts:
        if bool(post.get("is_case_root")):
            return normalize_text(post.get("content"))
    title = normalize_text(case_row.get("title"))
    if title:
        return title
    if posts:
        return normalize_text(posts[0].get("content"))
    return ""


def select_reactions(posts: List[Dict[str, object]], root_text: str, config: TrainConfig) -> List[str]:
    reactions = []
    for post in posts:
        text = normalize_text(post.get("content"))
        if not text or text == root_text:
            continue
        reactions.append(text[: config.max_reaction_chars])
        if len(reactions) >= config.max_reactions:
            break
    return reactions


def build_case_text(root_text: str, reactions: List[str], text_mode: str) -> str:
    if text_mode == "root_only" or not reactions:
        return f"Root tweet: {root_text}"
    return " ".join(["Root tweet:", root_text, "Reactions:"] + reactions)


def normalize_text(value: object) -> str:
    return " ".join(str(value or "").split())


def training_args(config: TrainConfig, fold_id: int | str) -> TrainingArguments:
    return TrainingArguments(
        output_dir=str(config.output_dir / "hf_checkpoints" / f"fold_{fold_id}"),
        overwrite_output_dir=True,
        num_train_epochs=config.epochs,
        per_device_train_batch_size=config.batch_size,
        per_device_eval_batch_size=config.batch_size * 2,
        learning_rate=config.learning_rate,
        weight_decay=config.weight_decay,
        warmup_ratio=config.warmup_ratio,
        eval_strategy="no",
        save_strategy="no",
        logging_strategy="epoch",
        report_to=[],
        seed=config.random_seed,
        fp16=config.fp16,
        dataloader_num_workers=0,
    )


def softmax(logits: np.ndarray) -> np.ndarray:
    shifted = logits - np.max(logits, axis=1, keepdims=True)
    exp = np.exp(shifted)
    return exp / np.sum(exp, axis=1, keepdims=True)


def summarize_frame(df: pd.DataFrame) -> Dict[str, object]:
    return {
        "rows": int(len(df)),
        "positive_rows": int(df["label"].sum()),
        "positive_rate": float(df["label"].mean()) if len(df) else 0.0,
        "events": {str(k): int(v) for k, v in df["event_name"].value_counts().sort_index().items()},
        "mean_reaction_count": float(df["reaction_count"].mean()) if len(df) else 0.0,
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
    payload["output_dir"] = str(config.output_dir)
    payload["database_url"] = "<redacted>"
    return payload


if __name__ == "__main__":
    main()

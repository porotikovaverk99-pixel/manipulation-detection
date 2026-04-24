import argparse
import json
import os
import re
import time
from collections import Counter
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, Iterable, List, Sequence, Tuple

import numpy as np
import pandas as pd
import torch
from torch import nn
from torch.utils.data import DataLoader, Dataset


TOKEN_RE = re.compile(r"[a-z0-9_]+")
REQUIRED_COLUMNS = {"id", "clean_title", "title", "subreddit", "2_way_label"}


@dataclass
class TrainConfig:
    data_dir: Path
    output_dir: Path
    model_version: str
    positive_label: int
    max_train_rows: int
    max_eval_rows: int
    vocab_size: int
    min_freq: int
    max_tokens: int
    embedding_dim: int
    hidden_dim: int
    batch_size: int
    epochs: int
    learning_rate: float
    random_seed: int
    device: str


class TextDataset(Dataset):
    def __init__(self, rows: Sequence[Tuple[str, int]], vocab: Dict[str, int], max_tokens: int) -> None:
        self.rows = rows
        self.vocab = vocab
        self.max_tokens = max_tokens

    def __len__(self) -> int:
        return len(self.rows)

    def __getitem__(self, index: int) -> Tuple[List[int], int]:
        text, label = self.rows[index]
        ids = [self.vocab.get(token, 1) for token in tokenize(text)[: self.max_tokens]]
        if not ids:
            ids = [1]
        return ids, label


class EmbeddingBagClassifier(nn.Module):
    def __init__(self, vocab_size: int, embedding_dim: int, hidden_dim: int) -> None:
        super().__init__()
        self.embedding = nn.EmbeddingBag(vocab_size, embedding_dim, mode="mean")
        self.classifier = nn.Sequential(
            nn.LayerNorm(embedding_dim),
            nn.Linear(embedding_dim, hidden_dim),
            nn.ReLU(),
            nn.Dropout(0.2),
            nn.Linear(hidden_dim, 1),
        )

    def forward(self, tokens: torch.Tensor, offsets: torch.Tensor) -> torch.Tensor:
        embedded = self.embedding(tokens, offsets)
        return self.classifier(embedded).squeeze(1)


def main() -> None:
    args = parse_args()
    config = TrainConfig(
        data_dir=args.data_dir,
        output_dir=args.output_dir,
        model_version=args.model_version,
        positive_label=args.positive_label,
        max_train_rows=args.max_train_rows,
        max_eval_rows=args.max_eval_rows,
        vocab_size=args.vocab_size,
        min_freq=args.min_freq,
        max_tokens=args.max_tokens,
        embedding_dim=args.embedding_dim,
        hidden_dim=args.hidden_dim,
        batch_size=args.batch_size,
        epochs=args.epochs,
        learning_rate=args.learning_rate,
        random_seed=args.random_seed,
        device=args.device,
    )
    config.output_dir.mkdir(parents=True, exist_ok=True)
    set_seed(config.random_seed)

    device = resolve_device(config.device)
    print(json.dumps({"event": "device", **device_summary(device)}, ensure_ascii=False), flush=True)

    train_df = sample_frame(load_split(config.data_dir / "all_train.tsv", config.positive_label), config.max_train_rows, config.random_seed, balanced=True)
    valid_df = sample_frame(load_split(config.data_dir / "all_validate.tsv", config.positive_label), config.max_eval_rows, config.random_seed, balanced=False)
    test_df = sample_frame(load_split(config.data_dir / "all_test_public.tsv", config.positive_label), config.max_eval_rows, config.random_seed, balanced=False)

    vocab = build_vocab(train_df["text"], config.vocab_size, config.min_freq)
    train_rows = list(zip(train_df["text"].tolist(), train_df["label"].astype(int).tolist()))
    valid_rows = list(zip(valid_df["text"].tolist(), valid_df["label"].astype(int).tolist()))
    test_rows = list(zip(test_df["text"].tolist(), test_df["label"].astype(int).tolist()))

    train_loader = make_loader(train_rows, vocab, config, shuffle=True)
    valid_loader = make_loader(valid_rows, vocab, config, shuffle=False)
    test_loader = make_loader(test_rows, vocab, config, shuffle=False)

    model = EmbeddingBagClassifier(len(vocab), config.embedding_dim, config.hidden_dim).to(device)
    optimizer = torch.optim.AdamW(model.parameters(), lr=config.learning_rate, weight_decay=1e-4)
    loss_fn = nn.BCEWithLogitsLoss()

    started = time.perf_counter()
    epoch_metrics = []
    best_valid_f1 = -1.0
    best_state = None

    for epoch in range(1, config.epochs + 1):
        train_loss = train_one_epoch(model, train_loader, optimizer, loss_fn, device)
        valid_metrics = evaluate(model, valid_loader, device)
        epoch_record = {"epoch": epoch, "train_loss": train_loss, "validate": valid_metrics}
        epoch_metrics.append(epoch_record)
        print(json.dumps({"event": "epoch", **epoch_record}, ensure_ascii=False), flush=True)
        if valid_metrics["f1"] > best_valid_f1:
            best_valid_f1 = valid_metrics["f1"]
            best_state = {key: value.detach().cpu() for key, value in model.state_dict().items()}

    if best_state is not None:
        model.load_state_dict(best_state)

    training_seconds = time.perf_counter() - started
    validate_metrics = evaluate(model, valid_loader, device)
    test_metrics = evaluate(model, test_loader, device)

    metrics = {
        "model_version": config.model_version,
        "model_kind": "torch_embeddingbag_classifier",
        "task": "Fakeddit 2-way fake/suspicious title detection",
        "positive_label": config.positive_label,
        "config": serialize_config(config),
        "device": device_summary(device),
        "vocab_size": len(vocab),
        "dataset_summary": {
            "train": summarize_frame(train_df),
            "validate": summarize_frame(valid_df),
            "test_public": summarize_frame(test_df),
        },
        "training_seconds": training_seconds,
        "epoch_metrics": epoch_metrics,
        "splits": {
            "validate": validate_metrics,
            "test_public": test_metrics,
        },
        "generated_at": datetime.now(timezone.utc).isoformat(),
    }

    model_path = config.output_dir / "fakeddit_torch_text_detector.pt"
    vocab_path = config.output_dir / "fakeddit_torch_vocab.json"
    metrics_path = config.output_dir / "fakeddit_torch_text_detector_metrics.json"

    torch.save(
        {
            "model_state_dict": model.state_dict(),
            "model_version": config.model_version,
            "model_kind": "torch_embeddingbag_classifier",
            "config": serialize_config(config),
            "vocab_size": len(vocab),
            "metrics": metrics,
        },
        model_path,
    )
    vocab_path.write_text(json.dumps(vocab, ensure_ascii=False), encoding="utf-8")
    metrics_path.write_text(json.dumps(metrics, indent=2, ensure_ascii=False), encoding="utf-8")

    print(
        json.dumps(
            {
                "model_path": str(model_path),
                "vocab_path": str(vocab_path),
                "metrics_path": str(metrics_path),
                "training_seconds": training_seconds,
                "device": metrics["device"],
                "validate": validate_metrics,
                "test_public": test_metrics,
            },
            indent=2,
            ensure_ascii=False,
        ),
        flush=True,
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Train a Fakeddit title detector with PyTorch on CPU/GPU")
    parser.add_argument(
        "--data-dir",
        type=Path,
        default=Path(os.getenv("FAKEDDIT_DATA_DIR", default_fakeddit_data_dir())),
    )
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=Path(os.getenv("FAKEDDIT_TORCH_OUTPUT_DIR", default_output_dir())),
    )
    parser.add_argument("--model-version", default=os.getenv("FAKEDDIT_TORCH_MODEL_VERSION", "fakeddit-torch-embeddingbag-v1"))
    parser.add_argument("--positive-label", type=int, default=int(os.getenv("FAKEDDIT_POSITIVE_LABEL", "0")))
    parser.add_argument("--max-train-rows", type=int, default=int(os.getenv("FAKEDDIT_MAX_TRAIN_ROWS", "100000")))
    parser.add_argument("--max-eval-rows", type=int, default=int(os.getenv("FAKEDDIT_MAX_EVAL_ROWS", "50000")))
    parser.add_argument("--vocab-size", type=int, default=int(os.getenv("FAKEDDIT_VOCAB_SIZE", "50000")))
    parser.add_argument("--min-freq", type=int, default=int(os.getenv("FAKEDDIT_MIN_FREQ", "2")))
    parser.add_argument("--max-tokens", type=int, default=int(os.getenv("FAKEDDIT_MAX_TOKENS", "64")))
    parser.add_argument("--embedding-dim", type=int, default=int(os.getenv("FAKEDDIT_EMBEDDING_DIM", "128")))
    parser.add_argument("--hidden-dim", type=int, default=int(os.getenv("FAKEDDIT_HIDDEN_DIM", "128")))
    parser.add_argument("--batch-size", type=int, default=int(os.getenv("FAKEDDIT_BATCH_SIZE", "1024")))
    parser.add_argument("--epochs", type=int, default=int(os.getenv("FAKEDDIT_EPOCHS", "4")))
    parser.add_argument("--learning-rate", type=float, default=float(os.getenv("FAKEDDIT_LEARNING_RATE", "0.001")))
    parser.add_argument("--random-seed", type=int, default=int(os.getenv("FAKEDDIT_RANDOM_SEED", "42")))
    parser.add_argument("--device", default=os.getenv("FAKEDDIT_TORCH_DEVICE", "auto"))
    return parser.parse_args()


def default_fakeddit_data_dir() -> str:
    workspace_root = Path(__file__).resolve().parents[3]
    return str(workspace_root / "datasets/raw/fakeddit/text_metadata/all_samples (also includes non multimodal)")


def default_output_dir() -> str:
    workspace_root = Path(__file__).resolve().parents[3]
    return str(workspace_root / "evaluation_outputs/fakeddit_torch_text_detector")


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


def tokenize(text: str) -> List[str]:
    return TOKEN_RE.findall(str(text).lower())


def build_vocab(texts: Iterable[str], max_size: int, min_freq: int) -> Dict[str, int]:
    counter: Counter[str] = Counter()
    for text in texts:
        counter.update(tokenize(text))

    vocab = {"<pad>": 0, "<unk>": 1}
    for token, count in counter.most_common(max(0, max_size - len(vocab))):
        if count < min_freq:
            break
        vocab[token] = len(vocab)
    return vocab


def make_loader(rows: Sequence[Tuple[str, int]], vocab: Dict[str, int], config: TrainConfig, shuffle: bool) -> DataLoader:
    dataset = TextDataset(rows, vocab, config.max_tokens)
    return DataLoader(
        dataset,
        batch_size=config.batch_size,
        shuffle=shuffle,
        collate_fn=collate_batch,
        num_workers=0,
        pin_memory=torch.cuda.is_available(),
    )


def collate_batch(batch: Sequence[Tuple[List[int], int]]) -> Tuple[torch.Tensor, torch.Tensor, torch.Tensor]:
    tokens: List[int] = []
    offsets = [0]
    labels = []
    for token_ids, label in batch:
        tokens.extend(token_ids)
        offsets.append(len(tokens))
        labels.append(label)
    return (
        torch.tensor(tokens, dtype=torch.long),
        torch.tensor(offsets[:-1], dtype=torch.long),
        torch.tensor(labels, dtype=torch.float32),
    )


def train_one_epoch(
    model: nn.Module,
    loader: DataLoader,
    optimizer: torch.optim.Optimizer,
    loss_fn: nn.Module,
    device: torch.device,
) -> float:
    model.train()
    total_loss = 0.0
    total_rows = 0
    for tokens, offsets, labels in loader:
        tokens = tokens.to(device, non_blocking=True)
        offsets = offsets.to(device, non_blocking=True)
        labels = labels.to(device, non_blocking=True)

        optimizer.zero_grad(set_to_none=True)
        logits = model(tokens, offsets)
        loss = loss_fn(logits, labels)
        loss.backward()
        optimizer.step()

        batch_size = labels.numel()
        total_loss += float(loss.detach().cpu()) * batch_size
        total_rows += batch_size
    return total_loss / max(1, total_rows)


@torch.no_grad()
def evaluate(model: nn.Module, loader: DataLoader, device: torch.device) -> Dict[str, float | int]:
    model.eval()
    probabilities = []
    labels = []
    for tokens, offsets, batch_labels in loader:
        tokens = tokens.to(device, non_blocking=True)
        offsets = offsets.to(device, non_blocking=True)
        logits = model(tokens, offsets)
        probabilities.extend(torch.sigmoid(logits).detach().cpu().numpy().tolist())
        labels.extend(batch_labels.numpy().astype(int).tolist())

    y_true = np.asarray(labels, dtype=np.int64)
    y_prob = np.asarray(probabilities, dtype=np.float64)
    y_pred = (y_prob >= 0.5).astype(np.int64)
    tp = int(((y_true == 1) & (y_pred == 1)).sum())
    tn = int(((y_true == 0) & (y_pred == 0)).sum())
    fp = int(((y_true == 0) & (y_pred == 1)).sum())
    fn = int(((y_true == 1) & (y_pred == 0)).sum())
    precision = tp / max(1, tp + fp)
    recall = tp / max(1, tp + fn)
    f1 = 2 * precision * recall / max(1e-12, precision + recall)
    return {
        "rows": int(len(y_true)),
        "positive_rows": int(y_true.sum()),
        "positive_rate": float(y_true.mean()) if len(y_true) else 0.0,
        "accuracy": float((y_true == y_pred).mean()) if len(y_true) else 0.0,
        "precision": float(precision),
        "recall": float(recall),
        "f1": float(f1),
        "roc_auc": roc_auc(y_true, y_prob),
        "pr_auc": average_precision(y_true, y_prob),
        "precision_at_100": precision_at_k(y_true, y_prob, 100),
        "precision_at_500": precision_at_k(y_true, y_prob, 500),
        "tn": tn,
        "fp": fp,
        "fn": fn,
        "tp": tp,
    }


def precision_at_k(y_true: np.ndarray, y_prob: np.ndarray, k: int) -> float:
    if len(y_true) == 0 or k <= 0:
        return 0.0
    order = np.argsort(-y_prob)[: min(k, len(y_true))]
    return float(y_true[order].mean())


def roc_auc(y_true: np.ndarray, y_prob: np.ndarray) -> float | None:
    positives = int(y_true.sum())
    negatives = int(len(y_true) - positives)
    if positives == 0 or negatives == 0:
        return None
    order = np.argsort(-y_prob)
    sorted_true = y_true[order]
    tps = np.cumsum(sorted_true == 1)
    fps = np.cumsum(sorted_true == 0)
    tpr = np.concatenate([[0.0], tps / positives, [1.0]])
    fpr = np.concatenate([[0.0], fps / negatives, [1.0]])
    return float(np.trapezoid(tpr, fpr))


def average_precision(y_true: np.ndarray, y_prob: np.ndarray) -> float | None:
    positives = int(y_true.sum())
    if positives == 0:
        return None
    order = np.argsort(-y_prob)
    sorted_true = y_true[order]
    precision = np.cumsum(sorted_true == 1) / (np.arange(len(sorted_true)) + 1)
    return float((precision * (sorted_true == 1)).sum() / positives)


def summarize_frame(df: pd.DataFrame) -> Dict[str, object]:
    return {
        "rows": int(len(df)),
        "positive_rows": int(df["label"].sum()),
        "positive_rate": float(df["label"].mean()) if len(df) else 0.0,
        "raw_label_counts": {str(k): int(v) for k, v in df["label_raw"].value_counts().sort_index().items()},
    }


def resolve_device(raw: str) -> torch.device:
    if raw == "auto":
        return torch.device("cuda:0" if torch.cuda.is_available() else "cpu")
    return torch.device(raw)


def device_summary(device: torch.device) -> Dict[str, object]:
    summary: Dict[str, object] = {
        "torch_version": torch.__version__,
        "hip_version": getattr(torch.version, "hip", None),
        "requested_device": str(device),
        "cuda_available": bool(torch.cuda.is_available()),
        "device_count": int(torch.cuda.device_count()),
    }
    if device.type == "cuda" and torch.cuda.is_available():
        summary["device_name"] = torch.cuda.get_device_name(device.index or 0)
    return summary


def serialize_config(config: TrainConfig) -> Dict[str, object]:
    payload = asdict(config)
    payload["data_dir"] = str(config.data_dir)
    payload["output_dir"] = str(config.output_dir)
    return payload


def set_seed(seed: int) -> None:
    np.random.seed(seed)
    torch.manual_seed(seed)
    if torch.cuda.is_available():
        torch.cuda.manual_seed_all(seed)


if __name__ == "__main__":
    main()

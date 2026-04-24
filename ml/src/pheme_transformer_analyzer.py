import hashlib
import json
import os
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Tuple


class PhemeTransformerAnalyzer:
    """Runtime wrapper for the PHEME root-tweet + reactions transformer."""

    def __init__(
        self,
        model_path: str,
        threshold: float | None = None,
        text_mode: str | None = None,
        max_reactions: int | None = None,
        max_reaction_chars: int | None = None,
        max_length: int | None = None,
        device: str | None = None,
    ) -> None:
        model_dir = Path(model_path)
        if not model_dir.exists():
            raise FileNotFoundError(f"PHEME transformer model not found: {model_path}")

        try:
            import torch
            from transformers import AutoModelForSequenceClassification, AutoTokenizer
        except ImportError as exc:
            raise RuntimeError(
                "PHEME transformer runtime requires torch and transformers. "
                "Use the ROCm Docker image with ml/requirements-rocm-training.txt "
                "or install equivalent dependencies in the ML environment."
            ) from exc

        self.torch = torch
        self.model_dir = model_dir
        self.metrics = self._load_metrics(model_dir)
        config = self.metrics.get("config", {})

        env_threshold = os.getenv("PHEME_TRANSFORMER_THRESHOLD", "").strip()
        self.threshold = float(
            threshold
            if threshold is not None
            else env_threshold
            or self.metrics.get("recommended_threshold")
            or 0.5
        )
        self.high_threshold = float(os.getenv("PHEME_TRANSFORMER_HIGH_THRESHOLD", "0.70"))
        self.text_mode = (
            text_mode
            or os.getenv("PHEME_TRANSFORMER_TEXT_MODE", "").strip()
            or self.metrics.get("text_mode")
            or config.get("text_mode")
            or "root_reactions"
        )
        self.max_reactions = int(
            max_reactions
            if max_reactions is not None
            else os.getenv("PHEME_TRANSFORMER_MAX_REACTIONS", "")
            or config.get("max_reactions")
            or 8
        )
        self.max_reaction_chars = int(
            max_reaction_chars
            if max_reaction_chars is not None
            else os.getenv("PHEME_TRANSFORMER_MAX_REACTION_CHARS", "")
            or config.get("max_reaction_chars")
            or 220
        )
        self.max_length = int(
            max_length
            if max_length is not None
            else os.getenv("PHEME_TRANSFORMER_MAX_LENGTH", "")
            or config.get("max_length")
            or 192
        )
        self.model_version = (
            os.getenv("PHEME_TRANSFORMER_MODEL_VERSION", "").strip()
            or self.metrics.get("model_version")
            or "pheme-transformer-runtime-v1"
        )
        self.pipeline_hash = self._pipeline_hash()

        selected_device = device or os.getenv("PHEME_TRANSFORMER_DEVICE", "").strip()
        if not selected_device:
            selected_device = "cuda" if torch.cuda.is_available() else "cpu"
        self.device = selected_device

        self.tokenizer = AutoTokenizer.from_pretrained(str(model_dir), use_fast=True)
        self.model = AutoModelForSequenceClassification.from_pretrained(str(model_dir))
        self.model.to(self.device)
        self.model.eval()

    def analyze_case(self, case_payload: Dict[str, Any]) -> Dict[str, Any]:
        root_text, reactions = self.build_case_text_parts(case_payload)
        text = self.build_case_text(root_text, reactions, self.text_mode)
        probability = self.predict_probability(text)

        risk_level = "low"
        if probability >= self.high_threshold:
            risk_level = "high"
        elif probability >= self.threshold:
            risk_level = "medium"

        confidence_score = min(0.99, 0.55 + abs(probability - 0.5))
        root_chars = len(root_text)
        reaction_chars = sum(len(item) for item in reactions)
        evidence = [
            f"text_model_score={probability:.3f}",
            f"text_model_threshold={self.threshold:.3f}",
            f"root_reactions_used={len(reactions)}",
        ]
        if root_chars > 0:
            evidence.append(f"root_chars={root_chars}")
        if reaction_chars > 0:
            evidence.append(f"reaction_chars={reaction_chars}")

        return {
            "model_version": self.model_version,
            "model_path": str(self.model_dir),
            "pipeline_hash": self.pipeline_hash,
            "risk_score": round(probability, 6),
            "risk_level": risk_level,
            "confidence_score": round(confidence_score, 6),
            "text_mode": self.text_mode,
            "max_length": self.max_length,
            "reaction_count_used": len(reactions),
            "feature_payload": {
                "case_id": case_payload.get("case_id"),
                "external_case_id": case_payload.get("external_case_id"),
                "source_name": case_payload.get("source_name"),
                "dataset_name": case_payload.get("dataset_name"),
                "dataset_split": case_payload.get("dataset_split"),
                "event_name": case_payload.get("event_name"),
                "label": case_payload.get("label"),
                "root_char_count": root_chars,
                "reaction_count_used": len(reactions),
                "reaction_char_count": reaction_chars,
                "text_model_threshold": self.threshold,
                "text_model_high_threshold": self.high_threshold,
            },
            "evidence": evidence,
            "model_info": {
                "type": "pheme_case_text_transformer",
                "trained": True,
                "model_version": self.model_version,
                "model_path": str(self.model_dir),
                "device": self.device,
                "threshold": self.threshold,
                "high_threshold": self.high_threshold,
                "base_model": self.metrics.get("base_model"),
                "training_metrics": {
                    "precision_at_10": self.metrics.get("precision_at_10"),
                    "precision_at_20": self.metrics.get("precision_at_20"),
                    "f1": self.metrics.get("f1"),
                    "roc_auc": self.metrics.get("roc_auc"),
                    "pr_auc": self.metrics.get("pr_auc"),
                },
            },
        }

    def predict_probability(self, text: str) -> float:
        encoded = self.tokenizer(
            text,
            truncation=True,
            max_length=self.max_length,
            return_tensors="pt",
        )
        encoded = {key: value.to(self.device) for key, value in encoded.items()}
        with self.torch.no_grad():
            logits = self.model(**encoded).logits
            probabilities = self.torch.softmax(logits, dim=-1)
        if probabilities.shape[-1] < 2:
            return float(probabilities[0, 0].detach().cpu().item())
        return float(probabilities[0, 1].detach().cpu().item())

    def build_case_text_parts(self, case_payload: Dict[str, Any]) -> Tuple[str, List[str]]:
        posts = sorted(
            case_payload.get("posts", []) or [],
            key=lambda item: (
                self._parse_time(item.get("published_at")).timestamp(),
                int(item.get("post_id", 0) or 0),
            ),
        )
        root_post = next((post for post in posts if post.get("is_case_root")), posts[0] if posts else {})
        root_post_id = root_post.get("post_id")
        root_text = self._normalize_text(root_post.get("content") or case_payload.get("title"))
        if not root_text and posts:
            root_text = self._normalize_text(posts[0].get("content"))
            root_post_id = posts[0].get("post_id")
        if not root_text:
            raise ValueError("case has no root text for transformer analysis")

        reactions: List[str] = []
        for post in posts:
            if root_post_id is not None and post.get("post_id") == root_post_id:
                continue
            if post.get("is_case_root"):
                continue
            text = self._normalize_text(post.get("content"))
            if not text or text == root_text:
                continue
            reactions.append(text[: self.max_reaction_chars])
            if len(reactions) >= self.max_reactions:
                break
        return root_text, reactions

    def build_case_text(self, root_text: str, reactions: List[str], text_mode: str) -> str:
        if text_mode == "root_only" or not reactions:
            return f"Root tweet: {root_text}"
        return " ".join(["Root tweet:", root_text, "Reactions:"] + reactions)

    def _normalize_text(self, value: Any) -> str:
        return " ".join(str(value or "").split())

    def _parse_time(self, raw_value: Any) -> datetime:
        if isinstance(raw_value, datetime):
            dt = raw_value
        else:
            raw = str(raw_value or "").strip()
            if not raw:
                return datetime.now(timezone.utc)
            dt = datetime.fromisoformat(raw.replace("Z", "+00:00"))
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        return dt.astimezone(timezone.utc)

    def _load_metrics(self, model_dir: Path) -> Dict[str, Any]:
        metrics_path = model_dir.parent / "pheme_transformer_metrics.json"
        if not metrics_path.exists():
            return {}
        return json.loads(metrics_path.read_text(encoding="utf-8"))

    def _pipeline_hash(self) -> str:
        payload = json.dumps(
            {
                "model_version": self.model_version,
                "model_file_bytes": self._model_file_bytes(),
                "text_mode": self.text_mode,
                "max_reactions": self.max_reactions,
                "max_reaction_chars": self.max_reaction_chars,
                "max_length": self.max_length,
                "threshold": self.threshold,
                "high_threshold": self.high_threshold,
            },
            sort_keys=True,
        )
        return hashlib.sha256(payload.encode("utf-8")).hexdigest()

    def _model_file_bytes(self) -> int:
        for name in ("model.safetensors", "pytorch_model.bin"):
            model_file = self.model_dir / name
            if model_file.exists():
                return model_file.stat().st_size
        return 0

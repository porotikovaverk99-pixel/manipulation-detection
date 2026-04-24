import argparse
import hashlib
import json
import os
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Tuple

import pandas as pd
import psycopg2
from psycopg2.extras import Json, execute_values


def main() -> None:
    args = parse_args()
    database_url = args.database_url or os.getenv("DATABASE_URL") or os.getenv("DATABASE_URI") or ""
    if not database_url:
        raise RuntimeError("--database-url or DATABASE_URL is required")

    metrics = load_metrics(args.metrics_json)
    model_version = args.model_version or str(metrics.get("model_version") or args.scorer_key)
    threshold = args.threshold
    if threshold is None:
        threshold = float(metrics.get("recommended_threshold", 0.5))

    frame = pd.read_csv(args.predictions_csv)
    required = {"case_id", args.probability_column}
    missing_columns = sorted(required - set(frame.columns))
    if missing_columns:
        raise RuntimeError(f"prediction CSV is missing required columns: {missing_columns}")

    frame = frame.dropna(subset=["case_id", args.probability_column]).copy()
    frame["case_id"] = frame["case_id"].astype(int)
    frame[args.probability_column] = frame[args.probability_column].astype(float).clip(0.0, 1.0)
    frame = frame.drop_duplicates(subset=["case_id"], keep="last")

    pipeline_hash = args.pipeline_hash or compute_pipeline_hash(
        {
            "scorer_key": args.scorer_key,
            "model_version": model_version,
            "metrics": metrics,
            "predictions_csv": str(args.predictions_csv),
        }
    )

    with psycopg2.connect(database_url) as conn:
        existing_case_ids = load_existing_case_ids(conn, frame["case_id"].tolist())
        missing_case_ids = sorted(set(frame["case_id"].tolist()) - existing_case_ids)
        if missing_case_ids and args.strict:
            raise RuntimeError(f"{len(missing_case_ids)} case ids do not exist, first={missing_case_ids[:10]}")
        if missing_case_ids:
            frame = frame[frame["case_id"].isin(existing_case_ids)].copy()

        records = build_records(
            frame=frame,
            args=args,
            model_version=model_version,
            metrics=metrics,
            threshold=threshold,
            pipeline_hash=pipeline_hash,
        )
        if not args.dry_run and records:
            upsert_case_model_scores(conn, records)

    summary = {
        "scorer_key": args.scorer_key,
        "model_version": model_version,
        "predictions_csv": str(args.predictions_csv),
        "metrics_json": str(args.metrics_json) if args.metrics_json else "",
        "rows_in_csv": int(len(frame) + len(missing_case_ids)),
        "rows_imported": int(len(records)),
        "missing_case_ids": int(len(missing_case_ids)),
        "threshold": threshold,
        "pipeline_hash": pipeline_hash,
        "dry_run": args.dry_run,
        "generated_at": datetime.now(timezone.utc).isoformat(),
    }
    print(json.dumps(summary, indent=2))


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Import offline case model predictions into case_model_scores")
    parser.add_argument("--database-url", default="")
    parser.add_argument("--predictions-csv", type=Path, required=True)
    parser.add_argument("--metrics-json", type=Path)
    parser.add_argument("--scorer-key", required=True)
    parser.add_argument("--model-version", default="")
    parser.add_argument("--probability-column", default="oof_probability")
    parser.add_argument("--threshold", type=float)
    parser.add_argument("--pipeline-hash", default="")
    parser.add_argument("--source-endpoint", default="offline_oof_import")
    parser.add_argument("--strict", action="store_true")
    parser.add_argument("--dry-run", action="store_true")
    return parser.parse_args()


def load_metrics(path: Path | None) -> Dict[str, Any]:
    if not path:
        return {}
    if not path.exists():
        raise FileNotFoundError(path)
    return json.loads(path.read_text(encoding="utf-8"))


def load_existing_case_ids(conn, case_ids: List[int]) -> set[int]:
    if not case_ids:
        return set()
    with conn.cursor() as cur:
        cur.execute("SELECT id FROM cases WHERE id = ANY(%s)", (case_ids,))
        return {int(row[0]) for row in cur.fetchall()}


def build_records(
    frame: pd.DataFrame,
    args: argparse.Namespace,
    model_version: str,
    metrics: Dict[str, Any],
    threshold: float,
    pipeline_hash: str,
) -> List[Tuple[Any, ...]]:
    model_info = {
        "model_kind": metrics.get("model_kind") or metrics.get("model_type") or "",
        "base_model": metrics.get("base_model") or "",
        "text_mode": metrics.get("text_mode") or "",
        "recommended_threshold": threshold,
        "offline_metrics": compact_metrics(metrics),
    }
    records = []
    for row in frame.to_dict("records"):
        risk_score = clamp01(float(row[args.probability_column]))
        confidence = max(risk_score, 1.0 - risk_score)
        label_name = str(row.get("label_name", "") or "")
        event_name = str(row.get("event_name", "") or "")
        predicted_label = row.get("predicted_label", None)
        payload = {
            "import_source": "offline_oof_predictions",
            "probability_column": args.probability_column,
            "recommended_threshold": threshold,
            "label_name": label_name,
            "event_name": event_name,
            "predicted_label": safe_int(predicted_label),
        }
        evidence = [
            f"Offline OOF probability imported for {model_version}.",
            f"Score={risk_score:.4f}; threshold={threshold:.4f}.",
        ]
        records.append(
            (
                int(row["case_id"]),
                args.scorer_key,
                model_version,
                risk_score,
                risk_level(risk_score),
                confidence,
                None,
                None,
                None,
                Json(evidence),
                Json(payload),
                Json(model_info),
                pipeline_hash,
                args.source_endpoint,
            )
        )
    return records


def upsert_case_model_scores(conn, records: List[Tuple[Any, ...]]) -> None:
    sql = """
        INSERT INTO case_model_scores (
            case_id, scorer_key, model_version, risk_score, risk_level,
            confidence_score, temporal_score, coordination_score, content_score,
            evidence, feature_payload, model_info, pipeline_hash, source_endpoint
        )
        VALUES %s
        ON CONFLICT (case_id, scorer_key) DO UPDATE SET
            model_version = EXCLUDED.model_version,
            risk_score = EXCLUDED.risk_score,
            risk_level = EXCLUDED.risk_level,
            confidence_score = EXCLUDED.confidence_score,
            temporal_score = EXCLUDED.temporal_score,
            coordination_score = EXCLUDED.coordination_score,
            content_score = EXCLUDED.content_score,
            evidence = EXCLUDED.evidence,
            feature_payload = EXCLUDED.feature_payload,
            model_info = EXCLUDED.model_info,
            pipeline_hash = EXCLUDED.pipeline_hash,
            source_endpoint = EXCLUDED.source_endpoint,
            computed_at = NOW()
    """
    with conn.cursor() as cur:
        execute_values(cur, sql, records, page_size=500)


def compact_metrics(metrics: Dict[str, Any]) -> Dict[str, Any]:
    keys = [
        "precision_at_10",
        "precision_at_20",
        "precision",
        "recall",
        "f1",
        "roc_auc",
        "pr_auc",
        "training_seconds",
        "rows",
        "event_count",
    ]
    return {key: metrics[key] for key in keys if key in metrics}


def compute_pipeline_hash(payload: Dict[str, Any]) -> str:
    raw = json.dumps(payload, sort_keys=True, default=str).encode("utf-8")
    return hashlib.sha256(raw).hexdigest()


def risk_level(score: float) -> str:
    if score >= 0.7:
        return "high"
    if score >= 0.4:
        return "medium"
    return "low"


def clamp01(value: float) -> float:
    return max(0.0, min(1.0, value))


def safe_int(value: Any) -> int | None:
    try:
        if pd.isna(value):
            return None
        return int(value)
    except (TypeError, ValueError):
        return None


if __name__ == "__main__":
    main()

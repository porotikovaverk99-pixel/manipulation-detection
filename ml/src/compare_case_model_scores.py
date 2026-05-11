import argparse
import hashlib
import json
import os
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Tuple

import numpy as np
import pandas as pd
import psycopg2
from psycopg2.extras import Json, RealDictCursor, execute_values
from sklearn.metrics import average_precision_score, f1_score, precision_score, recall_score, roc_auc_score


def main() -> None:
    args = parse_args()
    database_url = args.database_url or os.getenv("DATABASE_URL") or os.getenv("DATABASE_URI") or ""
    if not database_url:
        raise RuntimeError("--database-url or DATABASE_URL is required")

    scorer_keys = parse_csv(args.scorer_keys)
    if len(scorer_keys) < 2:
        raise RuntimeError("at least two --scorer-keys are required")

    positive_labels = {normalize_label(item) for item in parse_csv(args.positive_labels)}
    weights = parse_weights(args.weights, scorer_keys)

    with psycopg2.connect(database_url) as conn:
        frame = load_score_frame(conn, args, scorer_keys)
        if frame.empty:
            raise RuntimeError("no case_model_scores rows found for selected scorer keys")

        wide = build_wide_frame(frame, scorer_keys, require_complete=not args.allow_partial)
        if wide.empty:
            raise RuntimeError("no cases have the required model scores")

        summary = build_summary(wide, scorer_keys, positive_labels, weights, args)
        if args.write_ensemble:
            records = build_ensemble_records(wide, scorer_keys, weights, summary, args)
            upsert_case_model_scores(conn, records)
            summary["ensemble_written"] = len(records)

    args.output_dir.mkdir(parents=True, exist_ok=True)
    summary_path = args.output_dir / "case_model_score_comparison.json"
    wide_path = args.output_dir / "case_model_score_comparison_cases.csv"
    summary_path.write_text(json.dumps(summary, indent=2, ensure_ascii=False), encoding="utf-8")
    wide.to_csv(wide_path, index=False)

    print(
        json.dumps(
            {
                "summary_path": str(summary_path),
                "cases_path": str(wide_path),
                "cases": int(len(wide)),
                "scorers": scorer_keys,
                "ensemble_key": args.ensemble_key if args.write_ensemble else "",
                "ensemble_metrics": summary.get("ensemble", {}).get("metrics", {}),
            },
            indent=2,
        )
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Compare case_model_scores and optionally persist a weighted ensemble")
    parser.add_argument("--database-url", default="")
    parser.add_argument("--source-name", default="pheme_large")
    parser.add_argument("--dataset-name", default="pheme")
    parser.add_argument("--dataset-split", default="eventcv_large")
    parser.add_argument(
        "--scorer-keys",
        default="case_logreg_oof,case_lightgbm_oof,pheme_transformer_text_oof",
    )
    parser.add_argument(
        "--weights",
        default="case_logreg_oof=0,case_lightgbm_oof=0.35,pheme_transformer_text_oof=0.65",
    )
    parser.add_argument("--positive-labels", default="rumour,rumor")
    parser.add_argument("--output-dir", type=Path, default=Path("/tmp/case_model_score_comparison"))
    parser.add_argument("--ensemble-key", default="case_ensemble_v1")
    parser.add_argument("--ensemble-version", default="case-ensemble-v1")
    parser.add_argument("--allow-partial", action="store_true")
    parser.add_argument("--write-ensemble", action="store_true")
    return parser.parse_args()


def load_score_frame(conn, args: argparse.Namespace, scorer_keys: List[str]) -> pd.DataFrame:
    conditions = ["cms.scorer_key = ANY(%s)", "COALESCE(c.label, '') <> ''"]
    params: List[Any] = [scorer_keys]
    if args.source_name:
        params.append(args.source_name)
        conditions.append(f"c.source_name = ${len(params)}")
    if args.dataset_name:
        params.append(args.dataset_name)
        conditions.append(f"c.dataset_name = ${len(params)}")
    if args.dataset_split:
        params.append(args.dataset_split)
        conditions.append(f"c.dataset_split = ${len(params)}")

    query = f"""
        SELECT
            c.id AS case_id,
            c.external_case_id,
            c.event_name,
            c.label,
            cms.scorer_key,
            cms.model_version,
            cms.risk_score,
            cms.risk_level,
            cms.pipeline_hash,
            cms.computed_at
        FROM cases c
        JOIN case_model_scores cms ON cms.case_id = c.id
        WHERE {' AND '.join(conditions)}
        ORDER BY c.event_name, c.id, cms.scorer_key
    """
    query = convert_dollar_placeholders(query)
    with conn.cursor(cursor_factory=RealDictCursor) as cur:
        cur.execute(query, params)
        rows = cur.fetchall()
    return pd.DataFrame([dict(row) for row in rows])


def build_wide_frame(frame: pd.DataFrame, scorer_keys: List[str], require_complete: bool) -> pd.DataFrame:
    base_columns = ["case_id", "external_case_id", "event_name", "label"]
    first_values = frame.sort_values(["case_id", "scorer_key"]).drop_duplicates("case_id")[base_columns]
    scores = frame.pivot_table(index="case_id", columns="scorer_key", values="risk_score", aggfunc="last")
    versions = frame.pivot_table(index="case_id", columns="scorer_key", values="model_version", aggfunc="last")
    scores = scores.reset_index()
    versions = versions.reset_index()
    wide = first_values.merge(scores, on="case_id", how="inner")
    for key in scorer_keys:
        if key not in wide.columns:
            wide[key] = np.nan
        version_col = f"{key}__model_version"
        if key in versions.columns:
            wide = wide.merge(versions[["case_id", key]].rename(columns={key: version_col}), on="case_id", how="left")
        else:
            wide[version_col] = ""
    if require_complete:
        wide = wide.dropna(subset=scorer_keys)
    return wide.sort_values(["event_name", "case_id"]).reset_index(drop=True)


def build_summary(
    wide: pd.DataFrame,
    scorer_keys: List[str],
    positive_labels: set[str],
    weights: Dict[str, float],
    args: argparse.Namespace,
) -> Dict[str, Any]:
    y = wide["label"].map(lambda value: 1 if normalize_label(value) in positive_labels else 0).to_numpy()
    model_metrics: Dict[str, Any] = {}
    for key in scorer_keys:
        if key in wide.columns:
            model_metrics[key] = metrics_for_scores(y, wide[key].fillna(0.0).to_numpy())

    ensemble_scores = np.zeros(len(wide), dtype=float)
    for key, weight in weights.items():
        ensemble_scores += wide[key].fillna(0.0).to_numpy() * weight
    ensemble_scores = np.clip(ensemble_scores, 0.0, 1.0)
    wide["ensemble_score"] = ensemble_scores
    ensemble_metrics = metrics_for_scores(y, ensemble_scores)

    top_cases = []
    for row in wide.sort_values(["ensemble_score", "case_id"], ascending=[False, True]).head(20).to_dict("records"):
        top_cases.append(
            {
                "case_id": int(row["case_id"]),
                "event_name": row.get("event_name", ""),
                "label": row.get("label", ""),
                "ensemble_score": float(row["ensemble_score"]),
                "component_scores": {key: float(row[key]) for key in scorer_keys},
            }
        )

    return {
        "dataset": {
            "source_name": args.source_name,
            "dataset_name": args.dataset_name,
            "dataset_split": args.dataset_split,
        },
        "positive_labels": sorted(positive_labels),
        "case_count": int(len(wide)),
        "positive_cases": int(y.sum()),
        "negative_cases": int(len(y) - y.sum()),
        "scorers": scorer_keys,
        "weights": weights,
        "models": model_metrics,
        "ensemble": {
            "scorer_key": args.ensemble_key,
            "model_version": args.ensemble_version,
            "metrics": ensemble_metrics,
        },
        "top_cases": top_cases,
        "generated_at": datetime.now(timezone.utc).isoformat(),
    }


def metrics_for_scores(y: np.ndarray, scores: np.ndarray) -> Dict[str, Any]:
    threshold = best_threshold(y, scores)
    pred = (scores >= threshold).astype(int)
    return {
        "best_threshold": float(threshold),
        "precision_at_10": precision_at_k(y, scores, 10),
        "precision_at_20": precision_at_k(y, scores, 20),
        "precision": safe_metric(lambda: precision_score(y, pred, zero_division=0)),
        "recall": safe_metric(lambda: recall_score(y, pred, zero_division=0)),
        "f1": safe_metric(lambda: f1_score(y, pred, zero_division=0)),
        "roc_auc": safe_metric(lambda: roc_auc_score(y, scores)),
        "pr_auc": safe_metric(lambda: average_precision_score(y, scores)),
    }


def best_threshold(y: np.ndarray, scores: np.ndarray) -> float:
    best = 0.5
    best_f1 = -1.0
    for threshold in np.linspace(0.0, 1.0, 101):
        pred = (scores >= threshold).astype(int)
        value = f1_score(y, pred, zero_division=0)
        if value > best_f1:
            best_f1 = value
            best = float(threshold)
    return best


def precision_at_k(y: np.ndarray, scores: np.ndarray, k: int) -> float | None:
    if len(y) == 0 or k <= 0:
        return None
    top_k = min(k, len(y))
    order = np.lexsort((np.arange(len(scores)), -scores))
    return float(y[order[:top_k]].sum() / top_k)


def build_ensemble_records(
    wide: pd.DataFrame,
    scorer_keys: List[str],
    weights: Dict[str, float],
    summary: Dict[str, Any],
    args: argparse.Namespace,
) -> List[Tuple[Any, ...]]:
    pipeline_hash = compute_pipeline_hash(
        {
            "ensemble_key": args.ensemble_key,
            "ensemble_version": args.ensemble_version,
            "weights": weights,
            "scorers": scorer_keys,
            "dataset": summary["dataset"],
        }
    )
    threshold = float(summary["ensemble"]["metrics"]["best_threshold"])
    model_info = {
        "ensemble_method": "weighted_average",
        "weights": weights,
        "input_scorers": scorer_keys,
        "offline_metrics": summary["ensemble"]["metrics"],
        "recommended_threshold": threshold,
    }
    records = []
    for row in wide.to_dict("records"):
        score = clamp01(float(row["ensemble_score"]))
        component_scores = {key: float(row[key]) for key in scorer_keys}
        payload = {
            "component_scores": component_scores,
            "weights": weights,
            "recommended_threshold": threshold,
            "event_name": row.get("event_name", ""),
            "label_name": row.get("label", ""),
        }
        evidence = [
            f"Weighted ensemble score={score:.4f}.",
            "Components: "
            + ", ".join(f"{key}={component_scores[key]:.4f}*{weights[key]:.2f}" for key in scorer_keys),
        ]
        records.append(
            (
                int(row["case_id"]),
                args.ensemble_key,
                args.ensemble_version,
                score,
                risk_level(score),
                max(score, 1.0 - score),
                None,
                None,
                None,
                Json(evidence),
                Json(payload),
                Json(model_info),
                pipeline_hash,
                "offline_weighted_ensemble",
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


def parse_weights(raw: str, scorer_keys: List[str]) -> Dict[str, float]:
    values: Dict[str, float] = {}
    if raw.strip():
        for item in parse_csv(raw):
            if "=" not in item:
                raise RuntimeError(f"invalid weight item {item!r}; expected scorer=weight")
            key, value = item.split("=", 1)
            values[key.strip()] = float(value)
    if not values:
        values = {key: 1.0 for key in scorer_keys}
    missing = sorted(set(scorer_keys) - set(values))
    extra = sorted(set(values) - set(scorer_keys))
    if missing or extra:
        raise RuntimeError(f"weights/scorers mismatch: missing={missing}, extra={extra}")
    total = sum(values.values())
    if total <= 0:
        raise RuntimeError("weights must sum to a positive value")
    return {key: value / total for key, value in values.items()}


def parse_csv(raw: str) -> List[str]:
    return [item.strip() for item in raw.split(",") if item.strip()]


def normalize_label(raw: Any) -> str:
    return str(raw or "").strip().lower().replace("-", "_").replace(" ", "_")


def convert_dollar_placeholders(query: str) -> str:
    # psycopg2 uses %s placeholders. This helper keeps SQL assembly readable.
    for index in range(1, 20):
        query = query.replace(f"${index}", "%s")
    return query


def safe_metric(fn) -> float | None:
    try:
        value = float(fn())
        if np.isnan(value) or np.isinf(value):
            return None
        return value
    except ValueError:
        return None


def risk_level(score: float) -> str:
    if score >= 0.7:
        return "high"
    if score >= 0.4:
        return "medium"
    return "low"


def clamp01(value: float) -> float:
    return max(0.0, min(1.0, value))


def compute_pipeline_hash(payload: Dict[str, Any]) -> str:
    raw = json.dumps(payload, sort_keys=True, default=str).encode("utf-8")
    return hashlib.sha256(raw).hexdigest()


if __name__ == "__main__":
    main()

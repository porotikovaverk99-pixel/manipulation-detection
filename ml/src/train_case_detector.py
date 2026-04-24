import json
import os
import pickle
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Tuple

import numpy as np
import pandas as pd
import psycopg2
from psycopg2.extras import RealDictCursor
from sklearn.impute import SimpleImputer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import average_precision_score, f1_score, precision_score, recall_score, roc_auc_score
from sklearn.model_selection import LeaveOneGroupOut
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

from case_feature_utils import build_case_feature_row


@dataclass
class TrainConfig:
    database_url: str
    output_dir: Path
    model_version: str
    dataset_name: str = "pheme"
    dataset_splits: Tuple[str, ...] = ()
    source_name: str = "pheme"
    random_seed: int = 42
    positive_labels: Tuple[str, ...] = ("rumour", "rumor")


def main() -> None:
    config = TrainConfig(
        database_url=getenv("DATABASE_URL", getenv("DATABASE_URI", "")),
        output_dir=Path(
            getenv(
                "CASE_MODEL_OUTPUT_DIR",
                "/home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/case_detector",
            )
        ),
        model_version=getenv("CASE_MODEL_VERSION", "case-logreg-v1"),
        dataset_name=getenv("CASE_DATASET_NAME", "pheme"),
        dataset_splits=tuple(
            item.strip() for item in getenv("CASE_DATASET_SPLITS", "evalmix").split(",") if item.strip()
        ),
        source_name=getenv("CASE_SOURCE_NAME", "pheme"),
        random_seed=getenv_int("CASE_RANDOM_SEED", 42),
        positive_labels=tuple(
            normalize_label(item)
            for item in getenv("CASE_POSITIVE_LABELS", "rumour,rumor").split(",")
            if item.strip()
        ),
    )
    if not config.database_url:
        raise RuntimeError("DATABASE_URL or DATABASE_URI is required")
    config.output_dir.mkdir(parents=True, exist_ok=True)

    df, feature_columns, dataset_summary = load_training_frame(config)
    if df.empty:
        raise RuntimeError("no case rows available for training")

    event_count = df["event_name"].nunique()
    if event_count < 2:
        raise RuntimeError("need at least 2 distinct event_name values for event-based evaluation")

    X = df[feature_columns]
    y = df["label"]
    groups = df["event_name"]

    logo = LeaveOneGroupOut()
    oof_prob = np.zeros(len(df), dtype=float)
    fold_metrics = []

    for fold_index, (train_idx, test_idx) in enumerate(logo.split(X, y, groups), start=1):
        X_train = X.iloc[train_idx]
        y_train = y.iloc[train_idx]
        X_test = X.iloc[test_idx]
        y_test = y.iloc[test_idx]
        test_event = str(groups.iloc[test_idx].iloc[0])

        pipeline = make_pipeline(config.random_seed)
        pipeline.fit(X_train, y_train)
        probabilities = pipeline.predict_proba(X_test)[:, 1]
        predictions = (probabilities >= 0.5).astype(int)
        oof_prob[test_idx] = probabilities

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
            }
        )

    best_threshold = find_best_threshold(y.to_numpy(), oof_prob)
    overall_pred = (oof_prob >= best_threshold).astype(int)

    metrics = {
        "model_version": config.model_version,
        "dataset_name": config.dataset_name,
        "dataset_splits": list(config.dataset_splits),
        "source_name": config.source_name,
        "rows": int(len(df)),
        "event_count": int(event_count),
        "events": sorted(df["event_name"].unique().tolist()),
        "feature_count": len(feature_columns),
        "feature_columns": feature_columns,
        "positive_labels": list(config.positive_labels),
        "recommended_threshold": float(best_threshold),
        "precision_at_10": precision_at_k(df, oof_prob, 10),
        "precision_at_20": precision_at_k(df, oof_prob, 20),
        "precision": float(precision_score(y, overall_pred, zero_division=0)),
        "recall": float(recall_score(y, overall_pred, zero_division=0)),
        "f1": float(f1_score(y, overall_pred, zero_division=0)),
        "roc_auc": safe_metric(roc_auc_score, y, oof_prob),
        "pr_auc": safe_metric(average_precision_score, y, oof_prob),
        "fold_metrics": fold_metrics,
        "dataset_summary": dataset_summary,
        "generated_at": datetime.now(timezone.utc).isoformat(),
    }

    final_pipeline = make_pipeline(config.random_seed)
    final_pipeline.fit(X, y)
    model_hash = compute_model_hash(feature_columns, metrics)

    bundle = {
        "pipeline": final_pipeline,
        "feature_columns": feature_columns,
        "model_version": config.model_version,
        "recommended_threshold": best_threshold,
        "metrics": metrics,
        "model_hash": model_hash,
        "dataset_summary": dataset_summary,
        "trained_at": datetime.now(timezone.utc).isoformat(),
    }

    model_path = config.output_dir / "case_detector_model.pkl"
    metrics_path = config.output_dir / "case_detector_metrics.json"
    preview_path = config.output_dir / "case_training_dataset_sample.csv"
    oof_path = config.output_dir / "case_detector_oof_predictions.csv"

    with model_path.open("wb") as fh:
        pickle.dump(bundle, fh)
    metrics_path.write_text(json.dumps(metrics, indent=2), encoding="utf-8")

    preview = df[["case_id", "event_name", "label_name"] + feature_columns].head(500)
    preview.to_csv(preview_path, index=False)

    oof_df = df[["case_id", "event_name", "label_name"]].copy()
    oof_df["oof_probability"] = oof_prob
    oof_df["predicted_label"] = overall_pred
    oof_df.to_csv(oof_path, index=False)

    print(
        json.dumps(
            {
                "model_path": str(model_path),
                "metrics_path": str(metrics_path),
                "preview_path": str(preview_path),
                "oof_path": str(oof_path),
                "rows": int(len(df)),
                "events": metrics["events"],
                "recommended_threshold": best_threshold,
                "precision_at_10": metrics["precision_at_10"],
                "f1": metrics["f1"],
                "roc_auc": metrics["roc_auc"],
                "pr_auc": metrics["pr_auc"],
            },
            indent=2,
        )
    )


def load_training_frame(config: TrainConfig) -> Tuple[pd.DataFrame, List[str], Dict[str, int]]:
    query = """
        SELECT
            c.id AS case_id,
            c.event_name,
            c.label,
            cf.event_count,
            cf.unique_account_count,
            cf.unique_url_count,
            cf.unique_hashtag_count,
            cf.temporal_features,
            cf.coordination_features,
            cf.content_features,
            cf.feature_payload,
            cs.temporal_score,
            cs.coordination_score,
            cs.content_score
        FROM cases c
        JOIN case_features cf ON cf.case_id = c.id
        JOIN case_scores cs ON cs.case_id = c.id
        WHERE c.dataset_name = %s
          AND c.source_name = %s
          AND COALESCE(c.label, '') <> ''
    """
    params: List[object] = [config.dataset_name, config.source_name]
    if config.dataset_splits:
        query += " AND c.dataset_split = ANY(%s)"
        params.append(list(config.dataset_splits))
    query += " ORDER BY c.event_name, c.id"

    with psycopg2.connect(config.database_url) as conn:
        with conn.cursor(cursor_factory=RealDictCursor) as cur:
            cur.execute(query, params)
            rows = cur.fetchall()

    dataset_summary: Dict[str, int] = {}
    records: List[Dict[str, object]] = []
    all_feature_names = set()

    for row in rows:
        label_name = normalize_label(row.get("label", ""))
        if label_name == "":
            continue
        label = 1 if label_name in config.positive_labels else 0
        event_name = (row.get("event_name") or "").strip()
        if not event_name:
            continue

        record_payload = {
            "event_count": row.get("event_count"),
            "unique_account_count": row.get("unique_account_count"),
            "unique_url_count": row.get("unique_url_count"),
            "unique_hashtag_count": row.get("unique_hashtag_count"),
            "temporal_score": row.get("temporal_score"),
            "coordination_score": row.get("coordination_score"),
            "content_score": row.get("content_score"),
            "temporal_features": ensure_dict(row.get("temporal_features")),
            "coordination_features": ensure_dict(row.get("coordination_features")),
            "content_features": ensure_dict(row.get("content_features")),
            "feature_payload": ensure_dict(row.get("feature_payload")),
        }
        feature_row = build_case_feature_row(record_payload)
        if not feature_row:
            continue

        all_feature_names.update(feature_row.keys())
        dataset_summary[event_name] = dataset_summary.get(event_name, 0) + 1
        feature_row["case_id"] = int(row["case_id"])
        feature_row["event_name"] = event_name
        feature_row["label"] = label
        feature_row["label_name"] = label_name
        records.append(feature_row)

    if not records:
        return pd.DataFrame(), [], dataset_summary

    feature_columns = sorted(all_feature_names)
    df = pd.DataFrame(records)
    for column in feature_columns:
        if column not in df.columns:
            df[column] = np.nan
    return df, feature_columns, dataset_summary


def ensure_dict(value: object) -> Dict[str, object]:
    if isinstance(value, dict):
        return value
    if value is None:
        return {}
    if isinstance(value, str):
        text = value.strip()
        if not text:
            return {}
        try:
            parsed = json.loads(text)
            return parsed if isinstance(parsed, dict) else {}
        except json.JSONDecodeError:
            return {}
    return {}


def make_pipeline(random_seed: int) -> Pipeline:
    return Pipeline(
        steps=[
            ("imputer", SimpleImputer(strategy="median")),
            ("scaler", StandardScaler()),
            (
                "model",
                LogisticRegression(
                    max_iter=2000,
                    class_weight="balanced",
                    random_state=random_seed,
                ),
            ),
        ]
    )


def precision_at_k(df: pd.DataFrame, probabilities: np.ndarray, k: int) -> float:
    if len(df) == 0 or k <= 0:
        return 0.0
    ranking = df[["label"]].copy()
    ranking["probability"] = probabilities
    ranking = ranking.sort_values(["probability"], ascending=False).head(min(k, len(ranking)))
    return float(ranking["label"].mean())


def find_best_threshold(y_true: np.ndarray, probabilities: np.ndarray) -> float:
    best_threshold = 0.5
    best_f1 = -1.0
    for threshold in np.linspace(0.05, 0.95, 91):
        predicted = (probabilities >= threshold).astype(int)
        score = f1_score(y_true, predicted, zero_division=0)
        if score > best_f1:
            best_f1 = score
            best_threshold = float(threshold)
    return best_threshold


def safe_metric(metric_fn, y_true, y_score) -> float | None:
    try:
        return float(metric_fn(y_true, y_score))
    except ValueError:
        return None


def normalize_label(raw: str) -> str:
    return raw.strip().lower().replace("-", "_").replace(" ", "_")


def compute_model_hash(feature_columns: List[str], metrics: Dict[str, object]) -> str:
    payload = json.dumps(
        {
            "feature_columns": feature_columns,
            "rows": metrics.get("rows"),
            "events": metrics.get("events"),
            "recommended_threshold": metrics.get("recommended_threshold"),
        },
        sort_keys=True,
    )
    import hashlib

    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def getenv(key: str, fallback: str) -> str:
    value = os.getenv(key)
    if value is None or value.strip() == "":
        return fallback
    return value.strip()


def getenv_int(key: str, fallback: int) -> int:
    value = os.getenv(key)
    if value is None or value.strip() == "":
        return fallback
    try:
        parsed = int(value.strip())
    except ValueError:
        return fallback
    return parsed if parsed > 0 else fallback


if __name__ == "__main__":
    main()

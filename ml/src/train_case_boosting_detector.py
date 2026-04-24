import json
import os
import pickle
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Tuple

import numpy as np
import pandas as pd
from sklearn.ensemble import HistGradientBoostingClassifier, RandomForestClassifier
from sklearn.impute import SimpleImputer
from sklearn.metrics import average_precision_score, f1_score, precision_score, recall_score, roc_auc_score
from sklearn.model_selection import LeaveOneGroupOut
from sklearn.pipeline import Pipeline

from train_case_detector import (
    TrainConfig,
    compute_model_hash,
    find_best_threshold,
    getenv,
    getenv_int,
    load_training_frame,
    normalize_label,
    precision_at_k,
    safe_metric,
)


@dataclass
class BoostingConfig:
    base: TrainConfig
    model_kind: str


def main() -> None:
    config = BoostingConfig(
        base=TrainConfig(
            database_url=getenv("DATABASE_URL", getenv("DATABASE_URI", "")),
            output_dir=Path(
                getenv(
                    "CASE_MODEL_OUTPUT_DIR",
                    "/home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/case_boosting_detector",
                )
            ),
            model_version=getenv("CASE_MODEL_VERSION", "case-xgboost-v1"),
            dataset_name=getenv("CASE_DATASET_NAME", "pheme"),
            dataset_splits=tuple(
                item.strip() for item in getenv("CASE_DATASET_SPLITS", "eventcv_large").split(",") if item.strip()
            ),
            source_name=getenv("CASE_SOURCE_NAME", "pheme_large"),
            random_seed=getenv_int("CASE_RANDOM_SEED", 42),
            positive_labels=tuple(
                normalize_label(item)
                for item in getenv("CASE_POSITIVE_LABELS", "rumour,rumor").split(",")
                if item.strip()
            ),
        ),
        model_kind=getenv("CASE_BOOSTING_MODEL_KIND", "lightgbm").strip().lower(),
    )
    if not config.base.database_url:
        raise RuntimeError("DATABASE_URL or DATABASE_URI is required")
    config.base.output_dir.mkdir(parents=True, exist_ok=True)

    df, feature_columns, dataset_summary = load_training_frame(config.base)
    if df.empty:
        raise RuntimeError("no case rows available for training")
    if df["event_name"].nunique() < 2:
        raise RuntimeError("need at least 2 distinct event_name values for event-based evaluation")

    X = df[feature_columns]
    y = df["label"]
    groups = df["event_name"]

    logo = LeaveOneGroupOut()
    oof_prob = np.zeros(len(df), dtype=float)
    fold_metrics = []

    for fold_index, (train_idx, test_idx) in enumerate(logo.split(X, y, groups), start=1):
        pipeline = make_pipeline(config)
        pipeline.fit(X.iloc[train_idx], y.iloc[train_idx])
        probabilities = pipeline.predict_proba(X.iloc[test_idx])[:, 1]
        predictions = (probabilities >= 0.5).astype(int)
        oof_prob[test_idx] = probabilities

        y_test = y.iloc[test_idx]
        fold_metrics.append(
            {
                "fold": fold_index,
                "test_event": str(groups.iloc[test_idx].iloc[0]),
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
        "model_version": config.base.model_version,
        "model_kind": config.model_kind,
        "dataset_name": config.base.dataset_name,
        "dataset_splits": list(config.base.dataset_splits),
        "source_name": config.base.source_name,
        "rows": int(len(df)),
        "event_count": int(df["event_name"].nunique()),
        "events": sorted(df["event_name"].unique().tolist()),
        "feature_count": len(feature_columns),
        "feature_columns": feature_columns,
        "positive_labels": list(config.base.positive_labels),
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

    final_pipeline = make_pipeline(config)
    final_pipeline.fit(X, y)
    importances = feature_importance(final_pipeline, feature_columns)
    metrics["top_feature_importance"] = importances[:30]
    model_hash = compute_model_hash(feature_columns, metrics)

    bundle = {
        "pipeline": final_pipeline,
        "feature_columns": feature_columns,
        "model_version": config.base.model_version,
        "model_kind": config.model_kind,
        "recommended_threshold": best_threshold,
        "metrics": metrics,
        "model_hash": model_hash,
        "dataset_summary": dataset_summary,
        "trained_at": datetime.now(timezone.utc).isoformat(),
    }

    model_path = config.base.output_dir / "case_boosting_detector_model.pkl"
    metrics_path = config.base.output_dir / "case_boosting_detector_metrics.json"
    full_dataset_path = config.base.output_dir / "case_training_dataset.csv"
    oof_path = config.base.output_dir / "case_boosting_oof_predictions.csv"
    importance_path = config.base.output_dir / "case_boosting_feature_importance.json"

    with model_path.open("wb") as fh:
        pickle.dump(bundle, fh)
    metrics_path.write_text(json.dumps(metrics, indent=2), encoding="utf-8")
    importance_path.write_text(json.dumps(importances, indent=2), encoding="utf-8")
    df[["case_id", "event_name", "label_name"] + feature_columns].to_csv(full_dataset_path, index=False)

    oof_df = df[["case_id", "event_name", "label_name"]].copy()
    oof_df["oof_probability"] = oof_prob
    oof_df["predicted_label"] = overall_pred
    oof_df.to_csv(oof_path, index=False)

    print(
        json.dumps(
            {
                "model_path": str(model_path),
                "metrics_path": str(metrics_path),
                "importance_path": str(importance_path),
                "oof_path": str(oof_path),
                "rows": int(len(df)),
                "events": metrics["events"],
                "recommended_threshold": best_threshold,
                "precision_at_10": metrics["precision_at_10"],
                "precision_at_20": metrics["precision_at_20"],
                "f1": metrics["f1"],
                "roc_auc": metrics["roc_auc"],
                "pr_auc": metrics["pr_auc"],
            },
            indent=2,
        )
    )


def make_pipeline(config: BoostingConfig) -> Pipeline:
    if config.model_kind == "xgboost":
        try:
            from xgboost import XGBClassifier
        except ImportError as exc:
            raise RuntimeError(
                "xgboost is not installed. Install optional dependency: pip install 'xgboost>=2,<4'"
            ) from exc
        model = XGBClassifier(
            n_estimators=getenv_int("CASE_XGB_N_ESTIMATORS", 300),
            max_depth=getenv_int("CASE_XGB_MAX_DEPTH", 2),
            learning_rate=float(getenv("CASE_XGB_LEARNING_RATE", "0.03")),
            subsample=float(getenv("CASE_XGB_SUBSAMPLE", "0.90")),
            colsample_bytree=float(getenv("CASE_XGB_COLSAMPLE_BYTREE", "0.90")),
            min_child_weight=float(getenv("CASE_XGB_MIN_CHILD_WEIGHT", "2.0")),
            reg_lambda=float(getenv("CASE_XGB_REG_LAMBDA", "2.0")),
            reg_alpha=float(getenv("CASE_XGB_REG_ALPHA", "0.1")),
            objective="binary:logistic",
            eval_metric="logloss",
            tree_method="hist",
            n_jobs=-1,
            random_state=config.base.random_seed,
        )
        return Pipeline([("imputer", SimpleImputer(strategy="median")), ("model", model)])

    if config.model_kind == "lightgbm":
        try:
            from lightgbm import LGBMClassifier
        except ImportError as exc:
            raise RuntimeError(
                "lightgbm is not installed. Install optional dependency: pip install 'lightgbm>=4,<5'"
            ) from exc
        model = LGBMClassifier(
            n_estimators=getenv_int("CASE_LGBM_N_ESTIMATORS", 250),
            max_depth=getenv_int("CASE_LGBM_MAX_DEPTH", 3),
            num_leaves=getenv_int("CASE_LGBM_NUM_LEAVES", 7),
            learning_rate=float(getenv("CASE_LGBM_LEARNING_RATE", "0.03")),
            subsample=float(getenv("CASE_LGBM_SUBSAMPLE", "0.90")),
            colsample_bytree=float(getenv("CASE_LGBM_COLSAMPLE_BYTREE", "0.90")),
            reg_lambda=float(getenv("CASE_LGBM_REG_LAMBDA", "2.0")),
            reg_alpha=float(getenv("CASE_LGBM_REG_ALPHA", "0.1")),
            min_child_samples=getenv_int("CASE_LGBM_MIN_CHILD_SAMPLES", 20),
            class_weight="balanced",
            objective="binary",
            random_state=config.base.random_seed,
            n_jobs=-1,
            verbose=-1,
        )
        return Pipeline([("imputer", SimpleImputer(strategy="median")), ("model", model)])

    if config.model_kind == "hist_gradient_boosting":
        model = HistGradientBoostingClassifier(
            max_iter=getenv_int("CASE_HGB_MAX_ITER", 220),
            learning_rate=float(getenv("CASE_HGB_LEARNING_RATE", "0.04")),
            max_leaf_nodes=getenv_int("CASE_HGB_MAX_LEAF_NODES", 15),
            l2_regularization=float(getenv("CASE_HGB_L2", "0.1")),
            random_state=config.base.random_seed,
        )
        return Pipeline([("imputer", SimpleImputer(strategy="median")), ("model", model)])

    if config.model_kind == "random_forest":
        model = RandomForestClassifier(
            n_estimators=getenv_int("CASE_RF_N_ESTIMATORS", 500),
            max_depth=getenv_int("CASE_RF_MAX_DEPTH", 5),
            min_samples_leaf=getenv_int("CASE_RF_MIN_SAMPLES_LEAF", 5),
            class_weight="balanced",
            n_jobs=-1,
            random_state=config.base.random_seed,
        )
        return Pipeline([("imputer", SimpleImputer(strategy="median")), ("model", model)])

    raise RuntimeError(f"unsupported CASE_BOOSTING_MODEL_KIND={config.model_kind!r}")


def feature_importance(pipeline: Pipeline, feature_columns: List[str]) -> List[Dict[str, float | str]]:
    model = pipeline.named_steps["model"]
    values = getattr(model, "feature_importances_", None)
    if values is None:
        return []
    rows = [
        {"feature": feature, "importance": float(value)}
        for feature, value in zip(feature_columns, values)
    ]
    rows.sort(key=lambda item: item["importance"], reverse=True)
    return rows


if __name__ == "__main__":
    main()

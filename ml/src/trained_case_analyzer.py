import hashlib
import json
import math
import pickle
from pathlib import Path
from typing import Any, Dict, List

import numpy as np
import pandas as pd

from case_analyzer import CaseManipulationAnalyzer
from case_feature_utils import build_case_feature_row


class TrainedCaseAnalyzer:
    def __init__(self, model_path: str, bot_analyzer: Any = None) -> None:
        bundle_path = Path(model_path)
        if not bundle_path.exists():
            raise FileNotFoundError(f"case model not found: {model_path}")

        with bundle_path.open("rb") as fh:
            bundle = pickle.load(fh)

        self.pipeline = bundle["pipeline"]
        self.feature_columns = bundle["feature_columns"]
        self.model_version = bundle.get("model_version", "case-logreg-v1")
        self.recommended_threshold = float(bundle.get("recommended_threshold", 0.5))
        self.training_metrics = bundle.get("metrics", {})
        self.model_path = str(bundle_path)
        self.model_hash = bundle.get("model_hash", self._compute_model_hash(bundle))
        self.base_analyzer = CaseManipulationAnalyzer(bot_analyzer=bot_analyzer)

    def analyze_case(self, case_payload: Dict[str, Any]) -> Dict[str, Any]:
        heuristic_result = self.base_analyzer.analyze_case(case_payload)
        feature_row = build_case_feature_row(heuristic_result)
        frame = pd.DataFrame(
            [{column: feature_row.get(column, np.nan) for column in self.feature_columns}]
        )

        probability = float(self.pipeline.predict_proba(frame)[0, 1])
        risk_level = "low"
        if probability >= 0.70:
            risk_level = "high"
        elif probability >= 0.40:
            risk_level = "medium"

        confidence_score = min(0.99, 0.55 + abs(probability - 0.5))
        top_contributors = self._top_contributors(frame)

        evidence = []
        for item in top_contributors[:3]:
            evidence.append(f"model:{item['feature']}={item['contribution']:+.3f}")
        evidence.extend(heuristic_result.get("evidence", [])[:2])

        heuristic_result["score_version"] = self.model_version
        heuristic_result["pipeline_hash"] = self.model_hash
        heuristic_result["risk_score"] = round(probability, 6)
        heuristic_result["risk_level"] = risk_level
        heuristic_result["confidence_score"] = round(confidence_score, 6)
        heuristic_result["evidence"] = evidence
        heuristic_result["feature_payload"] = {
            **heuristic_result.get("feature_payload", {}),
            "trained_case_model": True,
            "case_model_version": self.model_version,
            "case_model_threshold": self.recommended_threshold,
        }
        heuristic_result["model_info"] = {
            "type": "trained_case_logistic_regression",
            "trained": True,
            "model_version": self.model_version,
            "model_path": self.model_path,
            "recommended_threshold": self.recommended_threshold,
            "top_contributors": top_contributors,
            "training_metrics": self.training_metrics,
        }
        return heuristic_result

    def _top_contributors(self, frame: pd.DataFrame) -> List[Dict[str, float]]:
        imputer = self.pipeline.named_steps["imputer"]
        scaler = self.pipeline.named_steps["scaler"]
        model = self.pipeline.named_steps["model"]

        transformed = imputer.transform(frame)
        transformed = scaler.transform(transformed)
        contributions = transformed[0] * model.coef_[0]
        indices = np.argsort(np.abs(contributions))[::-1][:5]

        result = []
        for idx in indices:
            raw_value = frame.iloc[0, idx]
            value = 0.0
            try:
                value = float(raw_value)
                if math.isnan(value) or math.isinf(value):
                    value = 0.0
            except (TypeError, ValueError):
                value = 0.0
            result.append(
                {
                    "feature": self.feature_columns[idx],
                    "contribution": round(float(contributions[idx]), 6),
                    "value": round(value, 6),
                }
            )
        return result

    def _compute_model_hash(self, bundle: Dict[str, Any]) -> str:
        payload = json.dumps(
            {
                "model_version": bundle.get("model_version"),
                "feature_columns": bundle.get("feature_columns"),
                "recommended_threshold": bundle.get("recommended_threshold"),
            },
            sort_keys=True,
        )
        return hashlib.sha256(payload.encode("utf-8")).hexdigest()

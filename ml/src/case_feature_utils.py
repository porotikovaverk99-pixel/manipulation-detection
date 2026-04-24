import math
from typing import Any, Dict


NON_NUMERIC_FEATURE_KEYS = {
    "case_id",
    "root_post_id",
    "external_case_id",
    "root_username",
    "ml_service_url",
    "scoring_source",
}


def build_case_feature_row(payload: Dict[str, Any]) -> Dict[str, float]:
    row: Dict[str, float] = {}

    for key in (
        "event_count",
        "unique_account_count",
        "unique_url_count",
        "unique_hashtag_count",
        "temporal_score",
        "coordination_score",
        "content_score",
    ):
        if key in payload:
            numeric = _safe_float(payload.get(key))
            if numeric is not None:
                row[key] = numeric

    for section_name in (
        "temporal_features",
        "coordination_features",
        "content_features",
        "feature_payload",
    ):
        section = payload.get(section_name, {}) or {}
        if not isinstance(section, dict):
            continue
        for key, value in section.items():
            if key in NON_NUMERIC_FEATURE_KEYS:
                continue
            numeric = _safe_float(value)
            if numeric is None:
                continue
            row[key] = numeric

    return row


def _safe_float(value: Any) -> float | None:
    try:
        numeric = float(value)
    except (TypeError, ValueError):
        return None
    if math.isnan(numeric) or math.isinf(numeric):
        return None
    return numeric

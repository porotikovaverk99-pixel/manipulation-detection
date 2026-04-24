import argparse
import csv
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Optional


def main() -> None:
    args = parse_args()
    top_cases = json.loads(args.top_cases_json.read_text(encoding="utf-8"))
    features_by_case = load_features(args.features_csv)

    cards = []
    for item in top_cases[: args.limit]:
        case_id = str(item["case_id"])
        features = features_by_case.get(case_id)
        cards.append(build_card(item, features))

    payload = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "top_cases_json": str(args.top_cases_json),
        "features_csv": str(args.features_csv),
        "limit": args.limit,
        "count": len(cards),
        "cards": cards,
    }

    args.output_json.parent.mkdir(parents=True, exist_ok=True)
    args.output_json.write_text(json.dumps(payload, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    if args.output_md:
        args.output_md.parent.mkdir(parents=True, exist_ok=True)
        args.output_md.write_text(render_markdown(payload), encoding="utf-8")

    print(
        json.dumps(
            {
                "output_json": str(args.output_json),
                "output_md": str(args.output_md) if args.output_md else None,
                "count": len(cards),
            },
            indent=2,
            ensure_ascii=False,
        )
    )


def parse_args() -> argparse.Namespace:
    workspace_root = Path(__file__).resolve().parents[3]
    default_run_dir = workspace_root / "evaluation_outputs/case_detector_run6_richer_large"
    parser = argparse.ArgumentParser(description="Export PHEME case-level evidence cards from evaluation artifacts")
    parser.add_argument("--top-cases-json", type=Path, default=default_run_dir / "top_oof_cases.json")
    parser.add_argument("--features-csv", type=Path, default=default_run_dir / "case_training_dataset.csv")
    parser.add_argument("--output-json", type=Path, default=default_run_dir / "evidence_cards_v1.json")
    parser.add_argument("--output-md", type=Path, default=default_run_dir / "evidence_cards_v1.md")
    parser.add_argument("--limit", type=int, default=10)
    return parser.parse_args()


def load_features(path: Path) -> Dict[str, Dict[str, str]]:
    if not path.exists():
        return {}
    with path.open(newline="", encoding="utf-8") as handle:
        return {str(row["case_id"]): row for row in csv.DictReader(handle)}


def build_card(item: Dict[str, object], features: Optional[Dict[str, str]]) -> Dict[str, object]:
    score = float(item.get("oof_probability", 0.0))
    return {
        "rank": item.get("rank"),
        "case_id": item.get("case_id"),
        "external_case_id": item.get("external_case_id"),
        "event_name": item.get("event_name"),
        "label_name": item.get("label_name"),
        "risk_score": round(score, 6),
        "risk_level": risk_level(score),
        "post_count": item.get("post_count"),
        "title": item.get("title"),
        "source_trace": {
            "first_event_at": item.get("first_event_at"),
            "last_event_at": item.get("last_event_at"),
        },
        "feature_snapshot_status": "available" if features else "unavailable",
        "main_signals": main_signals(features),
        "counter_signals": counter_signals(item, features),
    }


def risk_level(score: float) -> str:
    if score >= 0.8:
        return "high"
    if score >= 0.5:
        return "medium"
    return "low"


def main_signals(features: Optional[Dict[str, str]]) -> List[str]:
    if not features:
        return ["feature snapshot unavailable in exported sample"]
    signals: List[str] = []
    add_threshold_signal(signals, features, "first_10m_share", 0.50, "early burst in first 10 minutes")
    add_threshold_signal(signals, features, "first_30m_share", 0.70, "early burst in first 30 minutes")
    add_threshold_signal(signals, features, "peak_10m_share", 0.50, "high peak activity in a 10 minute window")
    add_threshold_signal(signals, features, "author_concentration", 0.30, "activity concentrated among few authors")
    add_threshold_signal(signals, features, "unique_domain_count", 3.0, "multiple linked domains")
    add_threshold_signal(signals, features, "root_jaccard_mean", 0.20, "reaction text overlaps with root claim")
    add_threshold_signal(signals, features, "reply_tree_available", 0.50, "reply tree is available")
    add_threshold_signal(signals, features, "root_uncertainty_flag", 0.50, "root contains uncertainty marker")
    add_threshold_signal(signals, features, "root_claim_flag", 0.50, "root contains claim marker")
    add_threshold_signal(signals, features, "root_denial_flag", 0.50, "root contains denial/debunk marker")
    add_threshold_signal(signals, features, "urgency_marker_ratio", 0.20, "urgency markers are frequent")
    add_threshold_signal(signals, features, "denial_marker_ratio", 0.20, "denial/debunk markers are frequent")
    add_threshold_signal(signals, features, "question_marker_ratio", 0.20, "question markers are frequent")
    return signals[:8] if signals else ["no single strong threshold signal; ranked by combined model weights"]


def counter_signals(item: Dict[str, object], features: Optional[Dict[str, str]]) -> List[str]:
    signals: List[str] = []
    post_count = int(item.get("post_count") or 0)
    if post_count < 5:
        signals.append(f"small case size: post_count={post_count}")
    if not features:
        signals.append("feature values are not present in the sampled feature CSV")
        return signals
    lexical_diversity = parse_float(features.get("lexical_diversity"))
    if lexical_diversity is not None and lexical_diversity >= 0.75:
        signals.append(f"high lexical diversity: lexical_diversity={lexical_diversity:.3f}")
    root_overlap_mean = parse_float(features.get("root_overlap_mean"))
    if root_overlap_mean is not None and root_overlap_mean < 0.10:
        signals.append(f"low direct root overlap: root_overlap_mean={root_overlap_mean:.3f}")
    return signals or ["no strong counter-signal in exported feature snapshot"]


def add_threshold_signal(
    signals: List[str],
    features: Dict[str, str],
    feature: str,
    threshold: float,
    label: str,
) -> None:
    value = parse_float(features.get(feature))
    if value is not None and value >= threshold:
        signals.append(f"{label}: {feature}={value:.3f}")


def parse_float(value: object) -> Optional[float]:
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def render_markdown(payload: Dict[str, object]) -> str:
    lines = [
        "# PHEME Evidence Cards v1",
        "",
        f"Generated at: `{payload['generated_at']}`",
        "",
        "These cards are generated from out-of-fold top-case artifacts. They are analyst-triage explanations, not causal proof of manipulation intent.",
        "",
        "| Rank | Case | Event | Label | Risk | Posts | Feature Snapshot |",
        "|---:|---:|---|---|---:|---:|---|",
    ]
    for card in payload["cards"]:
        lines.append(
            "| {rank} | `{case_id}` | `{event}` | `{label}` | `{risk:.4f}` | `{posts}` | `{status}` |".format(
                rank=card["rank"],
                case_id=card["case_id"],
                event=card["event_name"],
                label=card["label_name"],
                risk=card["risk_score"],
                posts=card["post_count"],
                status=card["feature_snapshot_status"],
            )
        )
    lines.append("")
    for card in payload["cards"]:
        lines.extend(render_card(card))
    return "\n".join(lines) + "\n"


def render_card(card: Dict[str, object]) -> List[str]:
    title = str(card.get("title") or "").replace("\n", " ").replace("|", "\\|")
    lines = [
        f"## Case `{card['case_id']}`",
        "",
        f"- Rank: `{card['rank']}`",
        f"- Event: `{card['event_name']}`",
        f"- Label: `{card['label_name']}`",
        f"- Risk score: `{card['risk_score']:.4f}`",
        f"- Risk level: `{card['risk_level']}`",
        f"- Posts: `{card['post_count']}`",
        f"- External case id: `{card['external_case_id']}`",
        f"- Title: {title}",
        "",
        "Main signals:",
    ]
    lines.extend(f"- {signal}" for signal in card["main_signals"])
    lines.append("")
    lines.append("Counter signals / limitations:")
    lines.extend(f"- {signal}" for signal in card["counter_signals"])
    lines.append("")
    return lines


if __name__ == "__main__":
    main()

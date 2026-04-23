import hashlib
import re
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Set
from urllib.parse import urlparse


class CaseManipulationAnalyzer:
    def __init__(self) -> None:
        self.feature_version = "case-features-ml-v2"
        self.score_version = "case-score-ml-v2"
        self.urgency_markers = [
            "breaking", "urgent", "alert", "now", "immediately",
            "warning", "must", "watch",
        ]
        self.claim_markers = [
            "reportedly", "rumour", "rumor", "claim", "claims",
            "according to", "unconfirmed", "reports",
        ]
        self.denial_markers = [
            "false", "fake", "hoax", "debunk", "not true",
            "isn't true", "is not true", "no evidence",
        ]
        self.question_markers = [
            "?", "why", "what", "how", "really", "are you sure",
            "is it true", "source", "proof", "confirm",
        ]
        self.uncertainty_markers = [
            "reportedly", "rumour", "rumor", "unconfirmed", "alleged",
            "apparently", "maybe", "possibly", "seems", "unclear",
            "are you sure", "is it true", "source", "proof",
            "confirm", "confirmed?", "not sure",
        ]
        self.retweet_prefixes = (
            "rt ", "rt@", "mt ", "mt@", "via ", "via@", "\"@", "“@", "‘@",
        )

    def analyze_case(self, case_payload: Dict[str, Any]) -> Dict[str, Any]:
        posts = sorted(
            case_payload.get("posts", []),
            key=lambda item: self._parse_time(item.get("published_at")).timestamp(),
        )
        if not posts:
            raise ValueError("case has no posts")

        root_post = next((post for post in posts if post.get("is_case_root")), posts[0])
        root_post_id = int(root_post.get("post_id", 0) or 0)
        root_text = root_post.get("content", "") or ""
        root_normalized = self._normalize_text(root_text)
        root_tokens = self._tokenize(root_normalized)

        first_event = self._parse_time(posts[0].get("published_at"))
        last_event = self._parse_time(posts[-1].get("published_at"))

        account_counts: Dict[int, int] = {}
        url_counts: Dict[str, int] = {}
        domain_counts: Dict[str, int] = {}
        unique_tags = set()
        text_counts: Dict[str, int] = {}
        token_posts: List[Set[str]] = []

        reply_count = 0
        direct_reply_to_root_count = 0
        deep_reply_count = 0
        urgency_posts = 0
        claim_posts = 0
        denial_posts = 0
        question_posts = 0
        uncertainty_posts = 0
        retweet_style_posts = 0
        root_copy_posts = 0
        total_url_refs = 0
        gap_sum = 0.0

        for idx, post in enumerate(posts):
            account_id = int(post.get("account_id", 0) or 0)
            account_counts[account_id] = account_counts.get(account_id, 0) + 1

            published_at = self._parse_time(post.get("published_at"))
            if published_at < first_event:
                first_event = published_at
            if published_at > last_event:
                last_event = published_at

            reply_to_post_id = post.get("reply_to_post_id")
            if reply_to_post_id is not None:
                reply_count += 1
                reply_target = int(reply_to_post_id)
                if root_post_id and reply_target == root_post_id:
                    direct_reply_to_root_count += 1
                elif root_post_id:
                    deep_reply_count += 1

            content = post.get("content", "") or ""
            normalized = self._normalize_text(content)
            tokens = self._tokenize(normalized)
            token_posts.append(tokens)
            if normalized:
                text_counts[normalized] = text_counts.get(normalized, 0) + 1

            lowered = content.lower()
            if self._contains_any(lowered, self.urgency_markers):
                urgency_posts += 1
            if self._contains_any(lowered, self.claim_markers):
                claim_posts += 1
            if self._contains_any(lowered, self.denial_markers):
                denial_posts += 1
            if self._contains_any(lowered, self.question_markers) or "?" in content:
                question_posts += 1
            if self._contains_any(lowered, self.uncertainty_markers):
                uncertainty_posts += 1

            stripped = lowered.strip()
            if stripped.startswith(self.retweet_prefixes):
                retweet_style_posts += 1

            if not post.get("is_case_root") and root_tokens:
                similarity = self._overlap_coefficient(tokens, root_tokens)
                if similarity >= 0.60:
                    root_copy_posts += 1

            for tag in post.get("tags", []) or []:
                clean_tag = (tag or "").strip().lower()
                if clean_tag:
                    unique_tags.add(clean_tag)

            for link in post.get("links", []) or []:
                clean_link = (link or "").strip().lower()
                if not clean_link:
                    continue
                url_counts[clean_link] = url_counts.get(clean_link, 0) + 1
                total_url_refs += 1
                domain = self._extract_domain(clean_link)
                if domain:
                    domain_counts[domain] = domain_counts.get(domain, 0) + 1

            if idx > 0:
                prev_time = self._parse_time(posts[idx - 1].get("published_at"))
                gap = (published_at - prev_time).total_seconds()
                if gap > 0:
                    gap_sum += gap

        event_count = len(posts)
        non_root_count = max(event_count - 1, 1)
        unique_account_count = len(account_counts)
        unique_url_count = len(url_counts)
        unique_domain_count = len(domain_counts)
        unique_hashtag_count = len(unique_tags)

        first_window_end = first_event + timedelta(hours=1)
        first_window_count = sum(
            1 for post in posts if self._parse_time(post.get("published_at")) <= first_window_end
        )

        span_seconds = max((last_event - first_event).total_seconds(), 0.0)
        span_hours = max(span_seconds / 3600.0, 1.0 / 60.0)
        avg_gap_seconds = 3600.0
        if event_count > 1:
            avg_gap_seconds = gap_sum / float(event_count - 1)
            if avg_gap_seconds <= 0:
                avg_gap_seconds = 3600.0

        max_author_posts = max(account_counts.values()) if account_counts else 0
        duplicate_posts = sum(count - 1 for count in text_counts.values() if count > 1)
        repeated_url_refs = sum(count - 1 for count in url_counts.values() if count > 1)
        repeated_domain_refs = sum(count - 1 for count in domain_counts.values() if count > 1)

        lexical_similarity_posts = 0
        for tokens in token_posts[1:]:
            if root_tokens and self._jaccard_similarity(tokens, root_tokens) >= 0.40:
                lexical_similarity_posts += 1

        first_window_share = self._safe_ratio(first_window_count, event_count)
        activity_density = self._clamp01(event_count / (span_hours * 10.0))
        gap_intensity = self._clamp01(1800.0 / max(avg_gap_seconds, 60.0))

        author_concentration = self._safe_ratio(max_author_posts, event_count)
        reply_ratio = self._safe_ratio(reply_count, non_root_count)
        direct_reply_ratio = self._safe_ratio(direct_reply_to_root_count, non_root_count)
        deep_reply_ratio = self._safe_ratio(deep_reply_count, non_root_count)
        duplicate_text_ratio = self._safe_ratio(duplicate_posts, event_count)
        lexical_similarity_ratio = self._safe_ratio(lexical_similarity_posts, non_root_count)
        root_copy_ratio = self._safe_ratio(root_copy_posts, non_root_count)
        retweet_style_ratio = self._safe_ratio(retweet_style_posts, non_root_count)

        repeated_url_ratio = 0.0
        if total_url_refs > 0:
            repeated_url_ratio = self._safe_ratio(repeated_url_refs, total_url_refs)
        repeated_domain_ratio = 0.0
        if total_url_refs > 0:
            repeated_domain_ratio = self._safe_ratio(repeated_domain_refs, total_url_refs)

        urgency_ratio = self._safe_ratio(urgency_posts, event_count)
        claim_ratio = self._safe_ratio(claim_posts, event_count)
        denial_ratio = self._safe_ratio(denial_posts, event_count)
        question_ratio = self._safe_ratio(question_posts, event_count)
        uncertainty_ratio = self._safe_ratio(uncertainty_posts, event_count)
        lexical_repetition_ratio = duplicate_text_ratio

        temporal_score = self._clamp01(
            0.35 * first_window_share
            + 0.30 * activity_density
            + 0.20 * gap_intensity
            + 0.15 * reply_ratio
        )
        coordination_score = self._clamp01(
            0.30 * author_concentration
            + 0.20 * duplicate_text_ratio
            + 0.10 * repeated_url_ratio
            + 0.10 * repeated_domain_ratio
            + 0.15 * lexical_similarity_ratio
            + 0.15 * reply_ratio
        )
        content_score = self._clamp01(
            0.18 * urgency_ratio
            + 0.17 * claim_ratio
            + 0.10 * denial_ratio
            + 0.15 * lexical_repetition_ratio
            + 0.20 * question_ratio
            + 0.20 * uncertainty_ratio
        )
        discussion_score = self._clamp01(
            0.35 * question_ratio
            + 0.35 * uncertainty_ratio
            + 0.20 * direct_reply_ratio
            + 0.10 * deep_reply_ratio
        )
        broadcast_relief = self._clamp01(
            0.55 * root_copy_ratio
            + 0.25 * retweet_style_ratio
            + 0.20 * repeated_domain_ratio
        )

        risk_score = self._clamp01(
            0.18 * temporal_score
            + 0.22 * coordination_score
            + 0.33 * content_score
            + 0.27 * discussion_score
            - 0.22 * broadcast_relief
        )

        risk_level = "low"
        if risk_score >= 0.70:
            risk_level = "high"
        elif risk_score >= 0.40:
            risk_level = "medium"

        confidence_score = min(
            0.95,
            0.50
            + 0.18 * self._clamp01(event_count / 12.0)
            + 0.27 * max(content_score, discussion_score, coordination_score),
        )

        temporal_features = {
            "first_window_share": first_window_share,
            "activity_density": activity_density,
            "gap_intensity": gap_intensity,
            "reply_ratio": reply_ratio,
            "time_span_seconds": span_seconds,
        }
        coordination_features = {
            "author_concentration": author_concentration,
            "direct_reply_ratio": direct_reply_ratio,
            "deep_reply_ratio": deep_reply_ratio,
            "duplicate_text_ratio": duplicate_text_ratio,
            "lexical_similarity_ratio": lexical_similarity_ratio,
            "repeated_url_ratio": repeated_url_ratio,
            "repeated_domain_ratio": repeated_domain_ratio,
            "root_copy_ratio": root_copy_ratio,
            "retweet_style_ratio": retweet_style_ratio,
            "broadcast_relief": broadcast_relief,
        }
        content_features = {
            "urgency_marker_ratio": urgency_ratio,
            "claim_marker_ratio": claim_ratio,
            "denial_marker_ratio": denial_ratio,
            "question_marker_ratio": question_ratio,
            "uncertainty_marker_ratio": uncertainty_ratio,
            "lexical_repetition_ratio": lexical_repetition_ratio,
            "discussion_score": discussion_score,
        }
        feature_payload = {
            "case_id": case_payload.get("case_id"),
            "external_case_id": case_payload.get("external_case_id"),
            "event_count": event_count,
            "unique_account_count": unique_account_count,
            "unique_url_count": unique_url_count,
            "unique_domain_count": unique_domain_count,
            "unique_hashtag_count": unique_hashtag_count,
            "time_span_seconds": span_seconds,
            "root_username": root_post.get("username", ""),
            "root_post_id": root_post_id,
            "broadcast_relief": broadcast_relief,
            "discussion_score": discussion_score,
        }

        evidence = self._build_evidence(
            {
                "temporal_score": temporal_score,
                "coordination_score": coordination_score,
                "content_score": content_score,
                "discussion_score": discussion_score,
                "question_marker_ratio": question_ratio,
                "uncertainty_marker_ratio": uncertainty_ratio,
                "root_copy_ratio": root_copy_ratio,
                "retweet_style_ratio": retweet_style_ratio,
            },
            broadcast_relief=broadcast_relief,
        )

        pipeline_hash = self._compute_pipeline_hash(self.feature_version, self.score_version)

        return {
            "feature_version": self.feature_version,
            "score_version": self.score_version,
            "pipeline_hash": pipeline_hash,
            "risk_score": round(risk_score, 6),
            "risk_level": risk_level,
            "confidence_score": round(confidence_score, 6),
            "temporal_score": round(temporal_score, 6),
            "coordination_score": round(coordination_score, 6),
            "content_score": round(content_score, 6),
            "event_count": event_count,
            "unique_account_count": unique_account_count,
            "unique_url_count": unique_url_count,
            "unique_hashtag_count": unique_hashtag_count,
            "temporal_features": temporal_features,
            "coordination_features": coordination_features,
            "content_features": content_features,
            "feature_payload": feature_payload,
            "evidence": evidence,
            "model_info": {
                "type": "heuristic_case_baseline",
                "trained": False,
            },
        }

    def _normalize_text(self, text: str) -> str:
        text = text.lower()
        text = re.sub(r"https?://\S+", " ", text)
        text = re.sub(r"[^\w\s@]", " ", text, flags=re.UNICODE)
        text = re.sub(r"\s+", " ", text).strip()
        return text

    def _tokenize(self, normalized_text: str) -> Set[str]:
        if not normalized_text:
            return set()
        return {
            token
            for token in normalized_text.split()
            if len(token) > 2 and not token.startswith("@")
        }

    def _contains_any(self, text: str, markers: List[str]) -> bool:
        return any(marker in text for marker in markers)

    def _safe_ratio(self, numerator: float, denominator: float) -> float:
        if denominator <= 0:
            return 0.0
        return self._clamp01(float(numerator) / float(denominator))

    def _clamp01(self, value: float) -> float:
        return max(0.0, min(1.0, float(value)))

    def _parse_time(self, raw_value: Any) -> datetime:
        if isinstance(raw_value, datetime):
            dt = raw_value
        else:
            raw = (raw_value or "").strip()
            if not raw:
                return datetime.now(timezone.utc)
            dt = datetime.fromisoformat(raw.replace("Z", "+00:00"))
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        return dt.astimezone(timezone.utc)

    def _extract_domain(self, url: str) -> str:
        try:
            parsed = urlparse(url)
        except Exception:
            return ""
        return (parsed.netloc or "").lower()

    def _jaccard_similarity(self, tokens_a: Set[str], tokens_b: Set[str]) -> float:
        if not tokens_a or not tokens_b:
            return 0.0
        union = tokens_a | tokens_b
        if not union:
            return 0.0
        return float(len(tokens_a & tokens_b)) / float(len(union))

    def _overlap_coefficient(self, tokens_a: Set[str], tokens_b: Set[str]) -> float:
        if not tokens_a or not tokens_b:
            return 0.0
        return float(len(tokens_a & tokens_b)) / float(min(len(tokens_a), len(tokens_b)))

    def _build_evidence(self, feature_scores: Dict[str, float], broadcast_relief: float) -> List[str]:
        items = sorted(feature_scores.items(), key=lambda item: item[1], reverse=True)
        evidence: List[str] = []
        for name, value in items[:4]:
            if value < 0.18:
                continue
            evidence.append(f"{name}={value:.3f}")
        if broadcast_relief >= 0.30:
            evidence.append(f"broadcast_relief={broadcast_relief:.3f}")
        return evidence

    def _compute_pipeline_hash(self, feature_version: str, score_version: str) -> str:
        payload = (
            f"{feature_version}|{score_version}|"
            "risk=0.18*temp+0.22*coord+0.33*content+0.27*discussion-0.22*broadcast|"
            "temp=0.35,0.30,0.20,0.15|coord=0.30,0.20,0.10,0.10,0.15,0.15|"
            "content=0.18,0.17,0.10,0.15,0.20,0.20|discussion=0.35,0.35,0.20,0.10|"
            "broadcast=0.55,0.25,0.20"
        )
        return hashlib.sha256(payload.encode("utf-8")).hexdigest()

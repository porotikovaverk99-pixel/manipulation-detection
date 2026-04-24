import hashlib
import re
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Set
from urllib.parse import urlparse


class CaseManipulationAnalyzer:
    def __init__(self, bot_analyzer: Any = None) -> None:
        self.feature_version = "case-features-ml-v4"
        self.score_version = "case-score-ml-v4"
        self.bot_analyzer = bot_analyzer
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

    def set_bot_analyzer(self, bot_analyzer: Any) -> None:
        self.bot_analyzer = bot_analyzer

    def analyze_case(self, case_payload: Dict[str, Any]) -> Dict[str, Any]:
        posts = sorted(
            case_payload.get("posts", []),
            key=lambda item: self._parse_time(item.get("published_at")).timestamp(),
        )
        if not posts:
            raise ValueError("case has no posts")

        root_post = next((post for post in posts if post.get("is_case_root")), posts[0])
        root_post_id = int(root_post.get("post_id", 0) or 0)
        root_account_id = int(root_post.get("account_id", 0) or 0)
        root_text = root_post.get("content", "") or ""
        root_normalized = self._normalize_text(root_text)
        root_tokens = self._tokenize(root_normalized)
        root_lowered = root_text.lower()

        first_event = self._parse_time(posts[0].get("published_at"))
        last_event = self._parse_time(posts[-1].get("published_at"))
        root_time = self._parse_time(root_post.get("published_at"))

        account_counts: Dict[int, int] = {}
        url_counts: Dict[str, int] = {}
        domain_counts: Dict[str, int] = {}
        reply_parent_counts: Dict[int, int] = {}
        unique_tags = set()
        text_counts: Dict[str, int] = {}
        token_posts: List[Set[str]] = []
        token_lengths: List[int] = []
        all_tokens: List[str] = []
        event_times: List[datetime] = []
        reaction_delays: List[float] = []
        root_jaccard_values: List[float] = []
        root_overlap_values: List[float] = []
        early_30m_accounts: Set[int] = set()

        reply_count = 0
        direct_reply_to_root_count = 0
        deep_reply_count = 0
        root_author_reaction_count = 0
        urgency_posts = 0
        claim_posts = 0
        denial_posts = 0
        question_posts = 0
        uncertainty_posts = 0
        reaction_question_posts = 0
        reaction_uncertainty_posts = 0
        reaction_denial_posts = 0
        retweet_style_posts = 0
        root_copy_posts = 0
        total_url_refs = 0
        gap_sum = 0.0

        for idx, post in enumerate(posts):
            account_id = int(post.get("account_id", 0) or 0)
            account_counts[account_id] = account_counts.get(account_id, 0) + 1

            published_at = self._parse_time(post.get("published_at"))
            event_times.append(published_at)
            if published_at < first_event:
                first_event = published_at
            if published_at > last_event:
                last_event = published_at
            if account_id and published_at <= root_time + timedelta(minutes=30):
                early_30m_accounts.add(account_id)

            reply_to_post_id = post.get("reply_to_post_id")
            if reply_to_post_id is not None:
                reply_count += 1
                reply_target = int(reply_to_post_id)
                reply_parent_counts[reply_target] = reply_parent_counts.get(reply_target, 0) + 1
                if root_post_id and reply_target == root_post_id:
                    direct_reply_to_root_count += 1
                elif root_post_id:
                    deep_reply_count += 1

            content = post.get("content", "") or ""
            normalized = self._normalize_text(content)
            tokens = self._tokenize(normalized)
            token_posts.append(tokens)
            token_lengths.append(len(tokens))
            all_tokens.extend(tokens)
            if normalized:
                text_counts[normalized] = text_counts.get(normalized, 0) + 1

            lowered = content.lower()
            is_root = bool(post.get("is_case_root"))
            if self._contains_any(lowered, self.urgency_markers):
                urgency_posts += 1
            if self._contains_any(lowered, self.claim_markers):
                claim_posts += 1
            if self._contains_any(lowered, self.denial_markers):
                denial_posts += 1
                if not is_root:
                    reaction_denial_posts += 1
            if self._contains_any(lowered, self.question_markers) or "?" in content:
                question_posts += 1
                if not is_root:
                    reaction_question_posts += 1
            if self._contains_any(lowered, self.uncertainty_markers):
                uncertainty_posts += 1
                if not is_root:
                    reaction_uncertainty_posts += 1

            stripped = lowered.strip()
            if stripped.startswith(self.retweet_prefixes):
                retweet_style_posts += 1

            if not is_root:
                if account_id and root_account_id and account_id == root_account_id:
                    root_author_reaction_count += 1
                reaction_delays.append(max((published_at - root_time).total_seconds(), 0.0))
                if root_tokens:
                    jaccard = self._jaccard_similarity(tokens, root_tokens)
                    overlap = self._overlap_coefficient(tokens, root_tokens)
                    root_jaccard_values.append(jaccard)
                    root_overlap_values.append(overlap)
                else:
                    overlap = 0.0
                if root_tokens and overlap >= 0.60:
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
        unique_reply_parent_count = len(reply_parent_counts)

        first_window_end = first_event + timedelta(hours=1)
        first_window_count = sum(
            1 for post in posts if self._parse_time(post.get("published_at")) <= first_window_end
        )
        first_10m_count = sum(1 for item in event_times if item <= first_event + timedelta(minutes=10))
        first_30m_count = sum(1 for item in event_times if item <= first_event + timedelta(minutes=30))

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
        max_reply_parent_posts = max(reply_parent_counts.values()) if reply_parent_counts else 0
        singleton_author_count = sum(1 for count in account_counts.values() if count == 1)
        repeated_author_count = sum(1 for count in account_counts.values() if count > 1)

        lexical_similarity_posts = 0
        for tokens in token_posts[1:]:
            if root_tokens and self._jaccard_similarity(tokens, root_tokens) >= 0.40:
                lexical_similarity_posts += 1

        first_window_share = self._safe_ratio(first_window_count, event_count)
        first_10m_share = self._safe_ratio(first_10m_count, event_count)
        first_30m_share = self._safe_ratio(first_30m_count, event_count)
        peak_10m_share = self._peak_window_share(event_times, 600.0)
        peak_30m_share = self._peak_window_share(event_times, 1800.0)
        activity_density = self._clamp01(event_count / (span_hours * 10.0))
        gap_intensity = self._clamp01(1800.0 / max(avg_gap_seconds, 60.0))
        median_reaction_delay_seconds = self._median(reaction_delays)
        median_reaction_delay_score = self._clamp01(3600.0 / max(median_reaction_delay_seconds, 60.0))
        early_actor_share = self._safe_ratio(len(early_30m_accounts), unique_account_count)

        author_concentration = self._safe_ratio(max_author_posts, event_count)
        author_singleton_share = self._safe_ratio(singleton_author_count, unique_account_count)
        repeated_author_share = self._safe_ratio(repeated_author_count, unique_account_count)
        reply_ratio = self._safe_ratio(reply_count, non_root_count)
        direct_reply_ratio = self._safe_ratio(direct_reply_to_root_count, non_root_count)
        deep_reply_ratio = self._safe_ratio(deep_reply_count, non_root_count)
        root_reply_share = self._safe_ratio(direct_reply_to_root_count, reply_count)
        reply_parent_concentration = self._safe_ratio(max_reply_parent_posts, reply_count)
        thread_branching_factor = self._safe_ratio(unique_reply_parent_count, non_root_count)
        root_author_reaction_share = self._safe_ratio(root_author_reaction_count, non_root_count)
        duplicate_text_ratio = self._safe_ratio(duplicate_posts, event_count)
        lexical_similarity_ratio = self._safe_ratio(lexical_similarity_posts, non_root_count)
        root_jaccard_mean = self._mean(root_jaccard_values)
        root_jaccard_max = max(root_jaccard_values) if root_jaccard_values else 0.0
        root_overlap_mean = self._mean(root_overlap_values)
        root_overlap_max = max(root_overlap_values) if root_overlap_values else 0.0
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
        reaction_question_ratio = self._safe_ratio(reaction_question_posts, non_root_count)
        reaction_uncertainty_ratio = self._safe_ratio(reaction_uncertainty_posts, non_root_count)
        reaction_denial_ratio = self._safe_ratio(reaction_denial_posts, non_root_count)
        lexical_repetition_ratio = duplicate_text_ratio
        avg_token_count = self._mean([float(value) for value in token_lengths])
        root_token_count = float(len(root_tokens))
        lexical_diversity = self._safe_ratio(len(set(all_tokens)), len(all_tokens))
        root_question_flag = 1.0 if self._contains_any(root_lowered, self.question_markers) or "?" in root_text else 0.0
        root_uncertainty_flag = 1.0 if self._contains_any(root_lowered, self.uncertainty_markers) else 0.0
        root_claim_flag = 1.0 if self._contains_any(root_lowered, self.claim_markers) else 0.0
        root_denial_flag = 1.0 if self._contains_any(root_lowered, self.denial_markers) else 0.0

        temporal_score = self._clamp01(
            0.20 * first_window_share
            + 0.15 * first_30m_share
            + 0.20 * peak_30m_share
            + 0.20 * activity_density
            + 0.15 * gap_intensity
            + 0.10 * median_reaction_delay_score
        )
        coordination_score = self._clamp01(
            0.18 * author_concentration
            + 0.12 * repeated_author_share
            + 0.18 * duplicate_text_ratio
            + 0.10 * repeated_url_ratio
            + 0.10 * repeated_domain_ratio
            + 0.12 * lexical_similarity_ratio
            + 0.12 * reply_parent_concentration
            + 0.10 * root_reply_share
            + 0.08 * early_actor_share
        )
        content_score = self._clamp01(
            0.13 * urgency_ratio
            + 0.15 * claim_ratio
            + 0.10 * denial_ratio
            + 0.12 * lexical_repetition_ratio
            + 0.17 * question_ratio
            + 0.17 * uncertainty_ratio
            + 0.08 * root_uncertainty_flag
            + 0.08 * root_claim_flag
        )
        discussion_score = self._clamp01(
            0.25 * reaction_question_ratio
            + 0.25 * reaction_uncertainty_ratio
            + 0.12 * reaction_denial_ratio
            + 0.16 * direct_reply_ratio
            + 0.10 * deep_reply_ratio
            + 0.12 * thread_branching_factor
        )
        broadcast_relief = self._clamp01(
            0.35 * root_copy_ratio
            + 0.20 * retweet_style_ratio
            + 0.15 * repeated_domain_ratio
            + 0.15 * root_jaccard_mean
            + 0.15 * root_author_reaction_share
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
            "first_10m_share": first_10m_share,
            "first_30m_share": first_30m_share,
            "peak_10m_share": peak_10m_share,
            "peak_30m_share": peak_30m_share,
            "activity_density": activity_density,
            "gap_intensity": gap_intensity,
            "median_reaction_delay_seconds": median_reaction_delay_seconds,
            "median_reaction_delay_score": median_reaction_delay_score,
            "early_actor_share": early_actor_share,
            "reply_ratio": reply_ratio,
            "time_span_seconds": span_seconds,
        }
        coordination_features = {
            "author_concentration": author_concentration,
            "author_singleton_share": author_singleton_share,
            "repeated_author_share": repeated_author_share,
            "direct_reply_ratio": direct_reply_ratio,
            "deep_reply_ratio": deep_reply_ratio,
            "root_reply_share": root_reply_share,
            "reply_parent_concentration": reply_parent_concentration,
            "thread_branching_factor": thread_branching_factor,
            "root_author_reaction_share": root_author_reaction_share,
            "duplicate_text_ratio": duplicate_text_ratio,
            "lexical_similarity_ratio": lexical_similarity_ratio,
            "root_jaccard_mean": root_jaccard_mean,
            "root_jaccard_max": root_jaccard_max,
            "root_overlap_mean": root_overlap_mean,
            "root_overlap_max": root_overlap_max,
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
            "reaction_question_ratio": reaction_question_ratio,
            "reaction_uncertainty_ratio": reaction_uncertainty_ratio,
            "reaction_denial_ratio": reaction_denial_ratio,
            "lexical_repetition_ratio": lexical_repetition_ratio,
            "avg_token_count": avg_token_count,
            "root_token_count": root_token_count,
            "lexical_diversity": lexical_diversity,
            "root_question_flag": root_question_flag,
            "root_uncertainty_flag": root_uncertainty_flag,
            "root_claim_flag": root_claim_flag,
            "root_denial_flag": root_denial_flag,
            "discussion_score": discussion_score,
        }
        bot_features = self._compute_bot_features(posts, root_post_id)
        feature_payload = {
            "case_id": case_payload.get("case_id"),
            "external_case_id": case_payload.get("external_case_id"),
            "event_count": event_count,
            "unique_account_count": unique_account_count,
            "unique_url_count": unique_url_count,
            "unique_domain_count": unique_domain_count,
            "unique_hashtag_count": unique_hashtag_count,
            "unique_reply_parent_count": unique_reply_parent_count,
            "time_span_seconds": span_seconds,
            "root_username": root_post.get("username", ""),
            "root_post_id": root_post_id,
            "broadcast_relief": broadcast_relief,
            "discussion_score": discussion_score,
            "reply_tree_available": 1.0 if unique_reply_parent_count > 0 else 0.0,
        }
        feature_payload.update(bot_features)

        evidence = self._build_evidence(
            {
                "temporal_score": temporal_score,
                "coordination_score": coordination_score,
                "content_score": content_score,
                "discussion_score": discussion_score,
                "question_marker_ratio": question_ratio,
                "uncertainty_marker_ratio": uncertainty_ratio,
                "reaction_question_ratio": reaction_question_ratio,
                "reaction_uncertainty_ratio": reaction_uncertainty_ratio,
                "peak_30m_share": peak_30m_share,
                "reply_parent_concentration": reply_parent_concentration,
                "root_copy_ratio": root_copy_ratio,
                "retweet_style_ratio": retweet_style_ratio,
            },
            broadcast_relief=broadcast_relief,
            bot_features=bot_features,
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
                "bot_model_enabled": self.bot_analyzer is not None,
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

    def _build_evidence(self, feature_scores: Dict[str, float], broadcast_relief: float, bot_features: Dict[str, Any]) -> List[str]:
        items = sorted(feature_scores.items(), key=lambda item: item[1], reverse=True)
        evidence: List[str] = []
        for name, value in items[:4]:
            if value < 0.18:
                continue
            evidence.append(f"{name}={value:.3f}")
        bot_max_score = float(bot_features.get("bot_score_max", 0.0) or 0.0)
        bot_high_share = float(bot_features.get("bot_high_share", 0.0) or 0.0)
        if bot_max_score >= 0.60:
            evidence.append(f"bot_score_max={bot_max_score:.3f}")
        if bot_high_share >= 0.30:
            evidence.append(f"bot_high_share={bot_high_share:.3f}")
        if broadcast_relief >= 0.30:
            evidence.append(f"broadcast_relief={broadcast_relief:.3f}")
        return evidence

    def _compute_bot_features(self, posts: List[Dict[str, Any]], root_post_id: int) -> Dict[str, Any]:
        base = {
            "bot_account_count": 0.0,
            "bot_score_mean": 0.0,
            "bot_score_max": 0.0,
            "bot_high_share": 0.0,
            "root_author_bot_score": 0.0,
            "bot_top_score_1": 0.0,
            "bot_top_score_2": 0.0,
            "bot_top_score_3": 0.0,
        }
        if self.bot_analyzer is None:
            return base

        accounts: Dict[int, Dict[str, Any]] = {}
        root_account_id = 0
        for post in posts:
            account_id = int(post.get("account_id", 0) or 0)
            if account_id == 0:
                continue
            if post.get("is_case_root"):
                root_account_id = account_id
            account = accounts.get(account_id)
            if account is None:
                account = {
                    "account_id": str(account_id),
                    "username": post.get("username"),
                    "statuses_count": float(post.get("posts_count", 0) or 0),
                    "followers_count": float(post.get("followers_count", 0) or 0),
                    "friends_count": float(post.get("following_count", 0) or 0),
                    "verified": bool(post.get("is_verified", False)),
                    "created_at": post.get("account_created_at"),
                    "url": post.get("account_url"),
                    "tweets": [],
                }
                accounts[account_id] = account

            content = str(post.get("content", "") or "")
            normalized = content.lower().strip()
            account["tweets"].append(
                {
                    "text": content,
                    "in_reply_to_status_id": str(post["reply_to_post_id"]) if post.get("reply_to_post_id") is not None else None,
                    "retweeted_status_id": str(root_post_id) if normalized.startswith(self.retweet_prefixes) and root_post_id else None,
                    "retweet_count": float(post.get("reposts_count", 0) or 0),
                    "reply_count": float(post.get("replies_count", 0) or 0),
                    "favorite_count": float(post.get("likes_count", 0) or 0),
                    "num_hashtags": float(len(post.get("tags", []) or [])),
                    "num_urls": float(len(post.get("links", []) or [])),
                    "num_mentions": float(len(re.findall(r"@[A-Za-z0-9_]+", content))),
                }
            )

        if not accounts:
            return base

        scores: List[float] = []
        high_count = 0
        root_author_bot_score = 0.0
        ranked_scores: List[float] = []

        for account_id, payload in accounts.items():
            tweet_count = len(payload["tweets"])
            if payload["statuses_count"] <= 0:
                payload["statuses_count"] = float(tweet_count)
            result = self.bot_analyzer.analyze_account(payload)
            bot_score = float(result.get("bot_score", 0.0) or 0.0)
            scores.append(bot_score)
            ranked_scores.append(bot_score)
            if bot_score >= 0.5:
                high_count += 1
            if account_id == root_account_id:
                root_author_bot_score = bot_score

        ranked_scores.sort(reverse=True)
        account_count = float(len(scores))
        return {
            "bot_account_count": account_count,
            "bot_score_mean": round(sum(scores) / account_count, 6),
            "bot_score_max": round(max(scores), 6),
            "bot_high_share": round(float(high_count) / account_count, 6),
            "root_author_bot_score": round(root_author_bot_score, 6),
            "bot_top_score_1": round(ranked_scores[0], 6) if len(ranked_scores) >= 1 else 0.0,
            "bot_top_score_2": round(ranked_scores[1], 6) if len(ranked_scores) >= 2 else 0.0,
            "bot_top_score_3": round(ranked_scores[2], 6) if len(ranked_scores) >= 3 else 0.0,
        }

    def _compute_pipeline_hash(self, feature_version: str, score_version: str) -> str:
        payload = (
            f"{feature_version}|{score_version}|"
            "risk=0.18*temp+0.22*coord+0.33*content+0.27*discussion-0.22*broadcast|"
            "temp=v4:window,peak,density,gap,delay|coord=v4:author,text,url,tree,early|"
            "content=v4:markers,root_flags|discussion=v4:reaction_markers,tree|"
            "broadcast=v4:copy,retweet,domain,root_similarity,root_author|bot_features=summary_only"
        )
        return hashlib.sha256(payload.encode("utf-8")).hexdigest()

    def _mean(self, values: List[float]) -> float:
        clean = [float(value) for value in values if value is not None]
        if not clean:
            return 0.0
        return sum(clean) / float(len(clean))

    def _median(self, values: List[float]) -> float:
        clean = sorted(float(value) for value in values if value is not None)
        if not clean:
            return 0.0
        mid = len(clean) // 2
        if len(clean) % 2 == 1:
            return clean[mid]
        return (clean[mid - 1] + clean[mid]) / 2.0

    def _peak_window_share(self, event_times: List[datetime], window_seconds: float) -> float:
        if not event_times:
            return 0.0
        times = sorted(event.timestamp() for event in event_times)
        best = 0
        left = 0
        for right, current in enumerate(times):
            while current - times[left] > window_seconds:
                left += 1
            best = max(best, right - left + 1)
        return self._safe_ratio(best, len(times))

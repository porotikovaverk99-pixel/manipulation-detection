import math
import pickle
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List

import numpy as np
import pandas as pd


class BotAccountAnalyzer:
    def __init__(self, model_path: str) -> None:
        bundle_path = Path(model_path)
        if not bundle_path.exists():
            raise FileNotFoundError(f"bot model not found: {model_path}")

        with bundle_path.open("rb") as fh:
            bundle = pickle.load(fh)

        self.pipeline = bundle["pipeline"]
        self.feature_columns = bundle["feature_columns"]
        self.dataset_summary = bundle.get("dataset_summary", [])
        self.model_version = "bot-logreg-v1"
        self.model_path = str(bundle_path)

    def analyze_account(self, payload: Dict[str, Any]) -> Dict[str, Any]:
        features = self._build_feature_row(payload)
        for column in self.feature_columns:
            if column not in features:
                features[column] = np.nan

        frame = pd.DataFrame([{column: features.get(column, np.nan) for column in self.feature_columns}])
        bot_score = float(self.pipeline.predict_proba(frame)[0, 1])
        predicted_label = "bot" if bot_score >= 0.5 else "human"

        top_contributors = self._top_contributors(frame)

        return {
            "model_version": self.model_version,
            "model_path": self.model_path,
            "bot_score": round(bot_score, 6),
            "predicted_label": predicted_label,
            "feature_payload": {key: self._json_value(value) for key, value in features.items()},
            "top_contributors": top_contributors,
        }

    def _build_feature_row(self, payload: Dict[str, Any]) -> Dict[str, float]:
        created_at = self._parse_time(payload.get("created_at"))
        updated_at = self._parse_time(payload.get("updated_at")) or datetime.now(timezone.utc)
        if created_at is None:
            created_at = updated_at
        account_age_days = max((updated_at - created_at).total_seconds() / 86400.0, 0.0)

        followers_count = self._float(payload.get("followers_count"))
        friends_count = self._float(payload.get("friends_count"))
        features: Dict[str, float] = {
            "statuses_count": math.log1p(max(self._float(payload.get("statuses_count")), 0.0)),
            "followers_count": math.log1p(max(followers_count, 0.0)),
            "friends_count": math.log1p(max(friends_count, 0.0)),
            "favourites_count": math.log1p(max(self._float(payload.get("favourites_count")), 0.0)),
            "listed_count": math.log1p(max(self._float(payload.get("listed_count")), 0.0)),
            "followers_friends_ratio": followers_count / (friends_count + 1.0),
            "default_profile": self._bool_float(payload.get("default_profile")),
            "default_profile_image": self._bool_float(payload.get("default_profile_image")),
            "geo_enabled": self._bool_float(payload.get("geo_enabled")),
            "verified": self._bool_float(payload.get("verified")),
            "protected": self._bool_float(payload.get("protected")),
            "has_description": self._text_present(payload.get("description")),
            "has_url": self._text_present(payload.get("url")),
            "has_location": self._text_present(payload.get("location")),
            "account_age_days": math.log1p(max(account_age_days, 0.0)),
        }

        tweets = payload.get("tweets", []) or []
        tweet_count = len(tweets)
        if tweet_count == 0:
            zero_defaults = {
                "tweet_count": 0.0,
                "avg_num_hashtags": 0.0,
                "avg_num_urls": 0.0,
                "avg_num_mentions": 0.0,
                "avg_retweet_count": 0.0,
                "avg_reply_count": 0.0,
                "avg_favorite_count": 0.0,
                "retweet_post_ratio": 0.0,
                "reply_post_ratio": 0.0,
                "hashtag_tweet_ratio": 0.0,
                "url_tweet_ratio": 0.0,
                "mention_tweet_ratio": 0.0,
                "avg_text_length": 0.0,
                "source_diversity": 0.0,
                "text_duplication_ratio": 0.0,
            }
            features.update(zero_defaults)
            return features

        text_norms = []
        sources = set()
        num_hashtags = []
        num_urls = []
        num_mentions = []
        retweet_counts = []
        reply_counts = []
        favorite_counts = []
        text_lengths = []
        retweet_posts = 0
        reply_posts = 0
        hashtag_posts = 0
        url_posts = 0
        mention_posts = 0

        for tweet in tweets:
            text = str(tweet.get("text", "") or "")
            normalized = " ".join(text.lower().split())
            text_norms.append(normalized)
            text_lengths.append(len(text))

            source = str(tweet.get("source", "") or "").strip()
            if source:
                sources.add(source)

            hashtags = max(self._float(tweet.get("num_hashtags")), 0.0)
            urls = max(self._float(tweet.get("num_urls")), 0.0)
            mentions = max(self._float(tweet.get("num_mentions")), 0.0)
            retweets = max(self._float(tweet.get("retweet_count")), 0.0)
            replies = max(self._float(tweet.get("reply_count")), 0.0)
            favorites = max(self._float(tweet.get("favorite_count")), 0.0)

            num_hashtags.append(hashtags)
            num_urls.append(urls)
            num_mentions.append(mentions)
            retweet_counts.append(retweets)
            reply_counts.append(replies)
            favorite_counts.append(favorites)

            if self._is_present(tweet.get("retweeted_status_id")):
                retweet_posts += 1
            if self._is_present(tweet.get("in_reply_to_status_id")):
                reply_posts += 1
            if hashtags > 0:
                hashtag_posts += 1
            if urls > 0:
                url_posts += 1
            if mentions > 0:
                mention_posts += 1

        unique_text_count = len(set(text_norms)) if text_norms else 0

        features.update(
            {
                "tweet_count": math.log1p(float(tweet_count)),
                "avg_num_hashtags": float(np.mean(num_hashtags)) if num_hashtags else 0.0,
                "avg_num_urls": float(np.mean(num_urls)) if num_urls else 0.0,
                "avg_num_mentions": float(np.mean(num_mentions)) if num_mentions else 0.0,
                "avg_retweet_count": math.log1p(float(np.mean(retweet_counts))) if retweet_counts else 0.0,
                "avg_reply_count": math.log1p(float(np.mean(reply_counts))) if reply_counts else 0.0,
                "avg_favorite_count": math.log1p(float(np.mean(favorite_counts))) if favorite_counts else 0.0,
                "retweet_post_ratio": float(retweet_posts) / float(tweet_count),
                "reply_post_ratio": float(reply_posts) / float(tweet_count),
                "hashtag_tweet_ratio": float(hashtag_posts) / float(tweet_count),
                "url_tweet_ratio": float(url_posts) / float(tweet_count),
                "mention_tweet_ratio": float(mention_posts) / float(tweet_count),
                "avg_text_length": math.log1p(float(np.mean(text_lengths))) if text_lengths else 0.0,
                "source_diversity": float(len(sources)) / float(tweet_count),
                "text_duplication_ratio": 1.0 - (float(unique_text_count) / float(tweet_count)),
            }
        )
        return features

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
            result.append(
                {
                    "feature": self.feature_columns[idx],
                    "contribution": round(float(contributions[idx]), 6),
                    "value": round(self._json_value(frame.iloc[0, idx]), 6),
                }
            )
        return result

    def _parse_time(self, raw: Any) -> datetime | None:
        text = str(raw or "").strip()
        if not text:
            return None
        try:
            dt = datetime.fromisoformat(text.replace("Z", "+00:00"))
        except ValueError:
            return None
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        return dt.astimezone(timezone.utc)

    def _bool_float(self, value: Any) -> float:
        return 1.0 if str(value).strip().lower() in {"1", "true", "t", "yes"} else 0.0

    def _text_present(self, value: Any) -> float:
        return 1.0 if str(value or "").strip() else 0.0

    def _float(self, value: Any) -> float:
        try:
            return float(value)
        except (TypeError, ValueError):
            return 0.0

    def _is_present(self, value: Any) -> bool:
        return str(value or "").strip().lower() not in {"", "0", "null", "nan"}

    def _json_value(self, value: Any) -> float:
        try:
            numeric = float(value)
        except (TypeError, ValueError):
            return 0.0
        if math.isnan(numeric) or math.isinf(numeric):
            return 0.0
        return numeric

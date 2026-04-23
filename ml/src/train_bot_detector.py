import io
import json
import math
import pickle
import zipfile
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, List, Tuple

import numpy as np
import pandas as pd
from sklearn.impute import SimpleImputer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import classification_report, f1_score, precision_score, recall_score, roc_auc_score
from sklearn.model_selection import train_test_split
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler


OUTER_SUBSETS = [
    ("genuine_accounts", 0),
    ("fake_followers", 1),
    ("social_spambots_1", 1),
    ("social_spambots_2", 1),
    ("social_spambots_3", 1),
    ("traditional_spambots_1", 1),
    ("traditional_spambots_2", 1),
    ("traditional_spambots_3", 1),
    ("traditional_spambots_4", 1),
]

USER_COLUMNS = [
    "id",
    "statuses_count",
    "followers_count",
    "friends_count",
    "favourites_count",
    "listed_count",
    "default_profile",
    "default_profile_image",
    "geo_enabled",
    "verified",
    "protected",
    "description",
    "url",
    "location",
    "created_at",
    "updated",
]

TWEET_COLUMNS = [
    "user_id",
    "text",
    "source",
    "in_reply_to_status_id",
    "retweeted_status_id",
    "retweet_count",
    "reply_count",
    "favorite_count",
    "num_hashtags",
    "num_urls",
    "num_mentions",
]


@dataclass
class BuildConfig:
    outer_zip_path: Path
    output_dir: Path
    max_users_per_subset: int = 1000
    chunk_size: int = 50000
    random_seed: int = 42


def main() -> None:
    config = BuildConfig(
        outer_zip_path=Path(
            getenv(
                "CRESCI_ZIP_PATH",
                "/home/richt/Documents/coding/cursor_fun/dplm_tst/datasets/raw/bots/cresci-2017.csv.zip",
            )
        ),
        output_dir=Path(
            getenv(
                "BOT_OUTPUT_DIR",
                "/home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/bot_detector",
            )
        ),
        max_users_per_subset=getenv_int("MAX_USERS_PER_SUBSET", 1000),
        chunk_size=getenv_int("TWEET_CHUNK_SIZE", 50000),
        random_seed=getenv_int("BOT_RANDOM_SEED", 42),
    )

    config.output_dir.mkdir(parents=True, exist_ok=True)
    df, feature_columns, dataset_summary = build_training_frame(config)
    if df.empty:
        raise RuntimeError("training dataset is empty")

    X = df[feature_columns]
    y = df["label"]
    X_train, X_test, y_train, y_test = train_test_split(
        X,
        y,
        test_size=0.25,
        random_state=config.random_seed,
        stratify=y,
    )

    pipeline = Pipeline(
        steps=[
            ("imputer", SimpleImputer(strategy="median")),
            ("scaler", StandardScaler()),
            ("model", LogisticRegression(max_iter=2000, class_weight="balanced")),
        ]
    )
    pipeline.fit(X_train, y_train)

    y_pred = pipeline.predict(X_test)
    y_score = pipeline.predict_proba(X_test)[:, 1]

    metrics = {
        "train_rows": int(len(X_train)),
        "test_rows": int(len(X_test)),
        "feature_count": len(feature_columns),
        "feature_columns": feature_columns,
        "precision": float(precision_score(y_test, y_pred, zero_division=0)),
        "recall": float(recall_score(y_test, y_pred, zero_division=0)),
        "f1": float(f1_score(y_test, y_pred, zero_division=0)),
        "roc_auc": float(roc_auc_score(y_test, y_score)),
        "classification_report": classification_report(y_test, y_pred, output_dict=True, zero_division=0),
        "dataset_summary": dataset_summary,
    }

    metrics_path = config.output_dir / "bot_detector_metrics.json"
    model_path = config.output_dir / "bot_detector_model.pkl"
    dataset_path = config.output_dir / "bot_training_dataset_sample.csv"

    metrics_path.write_text(json.dumps(metrics, indent=2), encoding="utf-8")
    dataset_preview = df[["user_id", "subset_name", "label"] + feature_columns].head(500)
    dataset_preview.to_csv(dataset_path, index=False)
    with model_path.open("wb") as fh:
        pickle.dump(
            {
                "pipeline": pipeline,
                "feature_columns": feature_columns,
                "dataset_summary": dataset_summary,
            },
            fh,
        )

    print(json.dumps(
        {
            "metrics_path": str(metrics_path),
            "model_path": str(model_path),
            "dataset_path": str(dataset_path),
            "precision": metrics["precision"],
            "recall": metrics["recall"],
            "f1": metrics["f1"],
            "roc_auc": metrics["roc_auc"],
            "rows": len(df),
        },
        indent=2,
    ))


def build_training_frame(config: BuildConfig) -> Tuple[pd.DataFrame, List[str], List[Dict[str, object]]]:
    frames: List[pd.DataFrame] = []
    dataset_summary: List[Dict[str, object]] = []

    with zipfile.ZipFile(config.outer_zip_path) as outer_zip:
        for subset_name, label in OUTER_SUBSETS:
            outer_member = f"datasets_full.csv/{subset_name}.csv.zip"
            nested = load_nested_zip(outer_zip, outer_member)
            users_df = load_users_frame(nested, subset_name)
            if users_df.empty:
                continue

            sample_size = min(len(users_df), config.max_users_per_subset)
            if sample_size < len(users_df):
                users_df = users_df.sample(sample_size, random_state=config.random_seed)
            user_ids = set(users_df["id"].astype(str))

            tweets_df = load_tweets_frame(
                nested=nested,
                subset_name=subset_name,
                selected_user_ids=user_ids,
                chunk_size=config.chunk_size,
            )
            feature_df = build_account_features(users_df, tweets_df)
            feature_df["label"] = label
            feature_df["subset_name"] = subset_name

            frames.append(feature_df)
            dataset_summary.append(
                {
                    "subset_name": subset_name,
                    "label": label,
                    "users_selected": int(len(users_df)),
                    "tweets_selected": int(len(tweets_df)),
                }
            )

    if not frames:
        return pd.DataFrame(), [], dataset_summary

    full_df = pd.concat(frames, ignore_index=True)
    feature_columns = [
        column
        for column in full_df.columns
        if column not in {"user_id", "label", "subset_name"}
    ]
    full_df[feature_columns] = full_df[feature_columns].replace([np.inf, -np.inf], np.nan)
    return full_df, feature_columns, dataset_summary


def load_nested_zip(outer_zip: zipfile.ZipFile, outer_member: str) -> zipfile.ZipFile:
    payload = outer_zip.read(outer_member)
    return zipfile.ZipFile(io.BytesIO(payload))


def load_users_frame(nested: zipfile.ZipFile, subset_name: str) -> pd.DataFrame:
    inner_member = f"{subset_name}.csv/users.csv"
    with nested.open(inner_member) as fh:
        users_df = pd.read_csv(
            fh,
            usecols=USER_COLUMNS,
            dtype={
                "id": str,
                "statuses_count": "float64",
                "followers_count": "float64",
                "friends_count": "float64",
                "favourites_count": "float64",
                "listed_count": "float64",
                "default_profile": str,
                "default_profile_image": str,
                "geo_enabled": str,
                "verified": str,
                "protected": str,
                "description": str,
                "url": str,
                "location": str,
                "created_at": str,
                "updated": str,
            },
            low_memory=False,
            encoding="latin-1",
        )
    return users_df.drop_duplicates(subset=["id"]).reset_index(drop=True)


def load_tweets_frame(
    nested: zipfile.ZipFile,
    subset_name: str,
    selected_user_ids: set,
    chunk_size: int,
) -> pd.DataFrame:
    inner_member = f"{subset_name}.csv/tweets.csv"
    chunks: List[pd.DataFrame] = []
    if inner_member not in nested.namelist():
        return pd.DataFrame(columns=TWEET_COLUMNS)
    with nested.open(inner_member) as fh:
        reader = pd.read_csv(
            fh,
            usecols=TWEET_COLUMNS,
            dtype={
                "user_id": str,
                "text": str,
                "source": str,
                "in_reply_to_status_id": str,
                "retweeted_status_id": str,
                "retweet_count": "float64",
                "reply_count": "float64",
                "favorite_count": "float64",
                "num_hashtags": "float64",
                "num_urls": "float64",
                "num_mentions": "float64",
            },
            chunksize=chunk_size,
            low_memory=False,
            encoding="latin-1",
        )
        for chunk in reader:
            filtered = chunk[chunk["user_id"].isin(selected_user_ids)]
            if not filtered.empty:
                chunks.append(filtered)
    if not chunks:
        return pd.DataFrame(columns=TWEET_COLUMNS)
    return pd.concat(chunks, ignore_index=True)


def build_account_features(users_df: pd.DataFrame, tweets_df: pd.DataFrame) -> pd.DataFrame:
    base = users_df.copy()
    base["user_id"] = base["id"].astype(str)

    base["followers_friends_ratio"] = base["followers_count"] / (base["friends_count"] + 1.0)
    base["has_description"] = base["description"].fillna("").str.strip().ne("").astype(float)
    base["has_url"] = base["url"].fillna("").str.strip().ne("").astype(float)
    base["has_location"] = base["location"].fillna("").str.strip().ne("").astype(float)

    for col in ["default_profile", "default_profile_image", "geo_enabled", "verified", "protected"]:
        base[col] = base[col].apply(to_bool_float)

    created_at = pd.to_datetime(base["created_at"], errors="coerce", utc=True)
    updated_at = pd.to_datetime(base["updated"], errors="coerce", utc=True)
    age_days = (updated_at - created_at).dt.total_seconds() / 86400.0
    age_median = age_days.median()
    if pd.isna(age_median):
        age_median = 0.0
    base["account_age_days"] = age_days.fillna(age_median)

    numeric_user_cols = [
        "statuses_count",
        "followers_count",
        "friends_count",
        "favourites_count",
        "listed_count",
        "followers_friends_ratio",
        "default_profile",
        "default_profile_image",
        "geo_enabled",
        "verified",
        "protected",
        "has_description",
        "has_url",
        "has_location",
        "account_age_days",
    ]
    for col in numeric_user_cols:
        base[col] = pd.to_numeric(base[col], errors="coerce")

    if tweets_df.empty:
        aggregates = pd.DataFrame({"user_id": base["user_id"]})
    else:
        tweets = tweets_df.copy()
        tweets["text"] = tweets["text"].fillna("")
        tweets["text_norm"] = tweets["text"].str.lower().str.replace(r"\s+", " ", regex=True).str.strip()
        tweets["text_length"] = tweets["text"].str.len()
        tweets["is_retweet_post"] = tweets["retweeted_status_id"].apply(is_present).astype(float)
        tweets["is_reply_post"] = tweets["in_reply_to_status_id"].apply(is_present).astype(float)
        tweets["has_hashtags"] = pd.to_numeric(tweets["num_hashtags"], errors="coerce").fillna(0).gt(0).astype(float)
        tweets["has_urls"] = pd.to_numeric(tweets["num_urls"], errors="coerce").fillna(0).gt(0).astype(float)
        tweets["has_mentions"] = pd.to_numeric(tweets["num_mentions"], errors="coerce").fillna(0).gt(0).astype(float)

        grouped = tweets.groupby("user_id", dropna=False)
        aggregates = grouped.agg(
            tweet_count=("user_id", "size"),
            avg_num_hashtags=("num_hashtags", "mean"),
            avg_num_urls=("num_urls", "mean"),
            avg_num_mentions=("num_mentions", "mean"),
            avg_retweet_count=("retweet_count", "mean"),
            avg_reply_count=("reply_count", "mean"),
            avg_favorite_count=("favorite_count", "mean"),
            retweet_post_ratio=("is_retweet_post", "mean"),
            reply_post_ratio=("is_reply_post", "mean"),
            hashtag_tweet_ratio=("has_hashtags", "mean"),
            url_tweet_ratio=("has_urls", "mean"),
            mention_tweet_ratio=("has_mentions", "mean"),
            avg_text_length=("text_length", "mean"),
            unique_source_count=("source", pd.Series.nunique),
            unique_text_count=("text_norm", pd.Series.nunique),
        ).reset_index()
        aggregates["source_diversity"] = aggregates["unique_source_count"] / aggregates["tweet_count"].clip(lower=1)
        aggregates["text_duplication_ratio"] = 1.0 - (
            aggregates["unique_text_count"] / aggregates["tweet_count"].clip(lower=1)
        )
        aggregates = aggregates.drop(columns=["unique_source_count", "unique_text_count"])

    full = base.merge(aggregates, on="user_id", how="left")

    for col in full.columns:
        if col in {"id", "user_id", "description", "url", "location", "created_at", "updated"}:
            continue
        full[col] = pd.to_numeric(full[col], errors="coerce")

    full = full.drop(columns=["id", "description", "url", "location", "created_at", "updated"])
    count_columns = [
        "statuses_count",
        "followers_count",
        "friends_count",
        "favourites_count",
        "listed_count",
        "tweet_count",
        "avg_retweet_count",
        "avg_reply_count",
        "avg_favorite_count",
        "avg_text_length",
        "account_age_days",
    ]
    for col in count_columns:
        if col in full.columns:
            full[col] = np.log1p(full[col].clip(lower=0))
    return full


def to_bool_float(value: object) -> float:
    if value is None or (isinstance(value, float) and math.isnan(value)):
        return 0.0
    return 1.0 if str(value).strip().lower() in {"1", "true", "t", "yes"} else 0.0


def is_present(value: object) -> bool:
    if value is None:
        return False
    text = str(value).strip().lower()
    return text not in {"", "0", "null", "nan"}


def getenv(key: str, fallback: str) -> str:
    import os

    value = os.getenv(key)
    if value is None or value.strip() == "":
        return fallback
    return value.strip()


def getenv_int(key: str, fallback: int) -> int:
    import os

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

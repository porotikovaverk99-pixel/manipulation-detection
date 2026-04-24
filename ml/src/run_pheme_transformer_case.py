import argparse
import json
import os
from datetime import date, datetime
from pathlib import Path
from typing import Any, Dict

import psycopg2
from psycopg2.extras import RealDictCursor

from pheme_transformer_analyzer import PhemeTransformerAnalyzer


def main() -> None:
    args = parse_args()
    if not args.model_path:
        raise RuntimeError("--model-path or PHEME_TRANSFORMER_MODEL_PATH is required")

    if args.payload_path:
        payload = json.loads(Path(args.payload_path).read_text(encoding="utf-8"))
    else:
        if not args.database_url:
            raise RuntimeError("--database-url or DATABASE_URL is required when --payload-path is not used")
        if args.case_id <= 0:
            raise RuntimeError("--case-id is required when --payload-path is not used")
        payload = load_case_payload(args.database_url, args.case_id)

    analyzer = PhemeTransformerAnalyzer(args.model_path)
    result = analyzer.analyze_case(payload)
    print(json.dumps(result, indent=2 if args.pretty else None, ensure_ascii=False))


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run the trained PHEME transformer on one case payload")
    parser.add_argument("--model-path", default=os.getenv("PHEME_TRANSFORMER_MODEL_PATH", ""))
    parser.add_argument("--database-url", default=os.getenv("DATABASE_URL") or os.getenv("DATABASE_URI") or "")
    parser.add_argument("--case-id", type=int, default=int(os.getenv("PHEME_CASE_ID", "0")))
    parser.add_argument("--payload-path", default="")
    parser.add_argument("--pretty", action="store_true")
    return parser.parse_args()


def load_case_payload(database_url: str, case_id: int) -> Dict[str, Any]:
    with psycopg2.connect(database_url) as conn:
        with conn.cursor(cursor_factory=RealDictCursor) as cur:
            cur.execute(
                """
                SELECT
                    id,
                    external_case_id,
                    source_name,
                    dataset_name,
                    dataset_split,
                    case_type,
                    label,
                    title,
                    event_name,
                    first_event_at,
                    last_event_at
                FROM cases
                WHERE id = %s
                """,
                (case_id,),
            )
            case_row = cur.fetchone()
            if not case_row:
                raise RuntimeError(f"case not found: {case_id}")

            cur.execute(
                """
                SELECT
                    p.id AS post_id,
                    p.external_id,
                    p.account_id,
                    a.username,
                    p.published_at,
                    p.content,
                    p.is_case_root,
                    p.reply_to_post_id,
                    COALESCE(p.likes_count, 0) AS likes_count,
                    COALESCE(p.reposts_count, 0) AS reposts_count,
                    COALESCE(p.replies_count, 0) AS replies_count,
                    COALESCE(a.followers_count, 0) AS followers_count,
                    COALESCE(a.following_count, 0) AS following_count,
                    COALESCE(a.posts_count, 0) AS posts_count,
                    COALESCE(a.is_verified, FALSE) AS is_verified,
                    a.created_at AS account_created_at,
                    COALESCE(a.account_url, '') AS account_url,
                    ARRAY(
                        SELECT pt.tag_name
                        FROM post_tags pt
                        WHERE pt.post_id = p.id
                        ORDER BY pt.tag_name
                    ) AS tags,
                    ARRAY(
                        SELECT pl.url
                        FROM post_links pl
                        WHERE pl.post_id = p.id
                        ORDER BY pl.id
                    ) AS links
                FROM posts p
                JOIN accounts a ON a.id = p.account_id
                WHERE p.case_id = %s
                ORDER BY p.published_at ASC, p.id ASC
                """,
                (case_id,),
            )
            posts = cur.fetchall()

    return {
        "case_id": int(case_row["id"]),
        "external_case_id": case_row.get("external_case_id") or "",
        "source_name": case_row.get("source_name") or "",
        "dataset_name": case_row.get("dataset_name") or "",
        "dataset_split": case_row.get("dataset_split") or "",
        "case_type": case_row.get("case_type") or "thread",
        "label": case_row.get("label") or "",
        "title": case_row.get("title") or "",
        "event_name": case_row.get("event_name") or "",
        "first_event_at": to_json_value(case_row.get("first_event_at")),
        "last_event_at": to_json_value(case_row.get("last_event_at")),
        "posts": [normalize_post(row) for row in posts],
    }


def normalize_post(row: Dict[str, Any]) -> Dict[str, Any]:
    return {
        "post_id": int(row["post_id"]),
        "external_id": row.get("external_id") or "",
        "account_id": int(row["account_id"]),
        "username": row.get("username") or "",
        "published_at": to_json_value(row.get("published_at")),
        "content": row.get("content") or "",
        "is_case_root": bool(row.get("is_case_root")),
        "reply_to_post_id": int(row["reply_to_post_id"]) if row.get("reply_to_post_id") is not None else None,
        "likes_count": int(row.get("likes_count") or 0),
        "reposts_count": int(row.get("reposts_count") or 0),
        "replies_count": int(row.get("replies_count") or 0),
        "followers_count": int(row.get("followers_count") or 0),
        "following_count": int(row.get("following_count") or 0),
        "posts_count": int(row.get("posts_count") or 0),
        "is_verified": bool(row.get("is_verified")),
        "account_created_at": to_json_value(row.get("account_created_at")),
        "account_url": row.get("account_url") or "",
        "tags": list(row.get("tags") or []),
        "links": list(row.get("links") or []),
    }


def to_json_value(value: Any) -> str | None:
    if value is None:
        return None
    if isinstance(value, (datetime, date)):
        return value.isoformat()
    return str(value)


if __name__ == "__main__":
    main()

# Frontend DB snapshot

This document explains how to pass a prepared PostgreSQL state to a frontend developer without sharing raw dataset folders or requiring local ML/model runs.

## Why this exists

The frontend needs real `cases`, `posts`, `case_features`, `case_scores` and `case_model_scores` to build the case-level dashboard.

Running the full pipeline requires:

- source datasets;
- PostgreSQL;
- Go loaders/scorers;
- ML service;
- trained/offline model artifacts.

For frontend work this is unnecessary. A prepared DB snapshot is enough.

## Important note

The snapshot contains dataset-derived records already loaded into PostgreSQL, including case texts and replies. It is not the original dataset folder, but it still contains dataset content.

Use it only inside the private project workflow. If the GitHub repository is public, do not push this snapshot until the project owner explicitly accepts that publication risk or changes the repository visibility to private.

The root `.gitignore` ignores `*.dump` and `*.pgdump` by default, but explicitly allows the prepared frontend snapshot under `backend/db_snapshots/`.

## Current local snapshot

Local path:

```text
backend/db_snapshots/manipulation_frontend_snapshot_2026-04-25.pgdump
```

Size:

```text
3.7M
```

SHA256:

```text
352b7693bb1b455cea86bc334a004846dc5a840dcd82001f96ff96bfa8e77231
```

This snapshot was tested by restoring it into a temporary database.

Restored counts:

```text
cases: 1221

case_model_scores:
case_ensemble_v1:           1000
case_feature:               1221
case_lightgbm_oof:          1000
case_logreg_oof:            1000
pheme_transformer_text:        1
pheme_transformer_text_oof: 1000
```

## How to create a fresh snapshot

From the repository root, while Docker Postgres is running:

```bash
mkdir -p ../db_exports

docker compose exec -T postgres pg_dump \
  -U postgres \
  -d manipulation_detection \
  --format=custom \
  --no-owner \
  --no-acl \
  > ../db_exports/manipulation_frontend_snapshot_$(date +%F).pgdump
```

By default, new snapshots should stay outside the repository or be shared as a private release/cloud artifact. Commit a snapshot into the repository only when the team intentionally wants that exact database state to travel with the code.

## How another developer restores it

From the repository root on the target machine:

```bash
docker compose up -d --wait postgres
```

Reset the target database:

```bash
docker compose exec -T postgres dropdb -U postgres --if-exists manipulation_detection
docker compose exec -T postgres createdb -U postgres manipulation_detection
```

Restore the snapshot:

```bash
cat backend/db_snapshots/manipulation_frontend_snapshot_2026-04-25.pgdump | \
  docker compose exec -T postgres pg_restore \
    -U postgres \
    -d manipulation_detection \
    --clean \
    --if-exists \
    --no-owner \
    --no-acl
```

Then run the backend API:

```bash
cd backend

DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
go run ./cmd/api
```

Useful checks:

```bash
curl 'http://localhost:8080/api/cases?source_name=pheme_large&dataset_name=pheme&dataset_split=eventcv_large&scorer_key=case_ensemble_v1&limit=5'

curl 'http://localhost:8080/api/cases/236?scorer_key=case_ensemble_v1'

curl 'http://localhost:8080/api/cases/236/scores'

curl 'http://localhost:8080/api/model-comparison?source_name=pheme_large&dataset_name=pheme&dataset_split=eventcv_large&top_k=5'
```

## What frontend developer does not need

For UI work with this snapshot, the developer does not need:

- raw datasets;
- trained transformer artifacts;
- GPU;
- ML service;
- dataset loaders;
- case scorers.

They only need:

- Docker/Postgres;
- Go backend API;
- frontend app.

## If dataset sharing becomes a concern

If even dataset-derived texts must not be shared, create a smaller synthetic/anonymized snapshot instead:

- keep the same table structure;
- keep realistic `risk_score`, `risk_level`, model metrics and timestamps;
- replace post content/usernames/external ids with generated demo values;
- keep 20-50 cases only.

That version is weaker for research validation, but enough for frontend layout and API integration.

# Local development

## 1. Postgres

From the repository root:

```bash
cp .env.example .env
make db-up
make db-migrate
```

This starts Postgres on `localhost:5432` and applies Goose migrations from `backend/migrations`.

Useful commands:

```bash
make db-logs
make db-shell
make db-migrate
make db-status
make db-version
make db-down-migration
make db-reset
```

Notes:

- `make db-migrate` runs `goose up`.
- `make db-status` shows applied/pending migrations.
- `make db-version` prints the current Goose schema version.
- `make db-down-migration` rolls back one migration.
- `make db-reset` removes the Postgres volume, recreates the database, and applies Goose migrations.

New migrations must use Goose sections in a single `.sql` file:

```sql
-- +goose Up
CREATE TABLE example_table (
    id BIGSERIAL PRIMARY KEY
);

-- +goose Down
DROP TABLE IF EXISTS example_table;
```

For PostgreSQL `DO $$ ... $$;` blocks, wrap the block:

```sql
-- +goose StatementBegin
DO $$
BEGIN
    -- statements
END $$;
-- +goose StatementEnd
```

## 2. Backend tools

From `backend`:

```bash
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
DATASET_PATH=./testdata/sample_dataset.jsonl \
DATASET_NAME=sample \
DATASET_SPLIT=dev \
go run ./cmd/dataset_loader
```

## 3. ML service

From `ml`:

```bash
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt

DATABASE_URL='postgresql://postgres:password@localhost:5432/manipulation_detection' \
ENABLE_LIVE_COMMENT_FETCH=false \
ML_SAVE_ANALYSIS_RESULTS=false \
PYTHONPATH=src \
.venv/bin/uvicorn main:app --host 0.0.0.0 --port 8000
```

## 4. Dataset analyzer

From `backend`, while the ML service is running:

```bash
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
ML_SERVICE_URL=http://localhost:8000 \
SOURCE_TYPE=dataset \
DATASET_NAME=sample \
DATASET_SPLIT=dev \
BATCH_LIMIT=100 \
go run ./cmd/dataset_analyzer
```

## 5. API checks

Run the backend API from `backend`:

```bash
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
go run ./cmd/api
```

Then:

```bash
curl 'http://localhost:8080/health'
curl 'http://localhost:8080/api/ingestion/runs?limit=10'
curl 'http://localhost:8080/api/analysis/summary?source_type=dataset&dataset_name=sample&dataset_split=dev'
```

Case-level frontend API checks:

```bash
curl 'http://localhost:8080/api/cases?source_name=pheme_large&dataset_name=pheme&dataset_split=eventcv_large&scorer_key=case_ensemble_v1&limit=5'
curl 'http://localhost:8080/api/cases/236?scorer_key=case_ensemble_v1'
curl 'http://localhost:8080/api/cases/236/scores'
curl 'http://localhost:8080/api/model-comparison?source_name=pheme_large&dataset_name=pheme&dataset_split=eventcv_large&top_k=5'
```

Endpoint purpose:

- `GET /api/cases` returns the case queue. Optional `scorer_key` switches the displayed score from active `case_scores` to a selected `case_model_scores` row such as `case_ensemble_v1`.
- `GET /api/cases/{id}` returns case metadata, root post, posts/replies, accounts, artifacts and feature snapshot.
- `GET /api/cases/{id}/scores` returns all model-specific scores for one case.
- `GET /api/model-comparison` computes model metrics from `case_model_scores` for the selected dataset slice.
- `GET /health` checks backend readiness, PostgreSQL connectivity and ML service `/health`.

## 6. Persistent audit events

The API writes persistent audit events to `audit_events`.

Each HTTP request records:

- `request_id`;
- endpoint path;
- HTTP method and status;
- response size;
- duration;
- remote address and user agent;
- normalized audit status: `succeeded`, `skipped` or `failed`.

This is separate from `collection_logs`. `collection_logs` is source-collection-specific; `audit_events` is a general operational trace for API and pipeline actions.

CLI commands also write audit events:

- `cmd/api` records API process lifecycle as `cli.api`.
- `cmd/db_migrate` records migration command result as `cli.db_migrate`.
- `cmd/dataset_loader` records generic JSONL ingestion as `cli.dataset_loader`.
- `cmd/pheme_loader` records PHEME ingestion as `cli.pheme_loader`.
- `cmd/dataset_analyzer` records post-level ML analysis runs as `cli.dataset_analyzer`.
- `cmd/case_scorer` records feature-based case scoring as `cli.case_scorer`.
- `cmd/case_ml_scorer` records ML-backed case scoring as `cli.case_ml_scorer`.
- `cmd/evaluate_cases` records evaluation runs as `cli.evaluate_cases`.
- `cmd/export_top_cases` records JSON export runs as `cli.export_top_cases`.
- `cmd/collector` records live collector lifecycle as `cli.collector` and each collection cycle as `cli.collector_collect_all`.

## 7. Runtime file logs

Runtime file logs are optional. They are separate from `audit_events`.

Use:

```bash
LOG_TO_FILE=true \
LOG_DIR=./logs \
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
go run ./cmd/api
```

By default each component writes to its own file:

- `logs/api.log`
- `logs/db_migrate.log`
- `logs/dataset_loader.log`
- `logs/pheme_loader.log`
- `logs/dataset_analyzer.log`
- `logs/case_scorer.log`
- `logs/case_ml_scorer.log`
- `logs/evaluate_cases.log`
- `logs/export_top_cases.log`
- `logs/collector.log`

Set `LOG_FILE=custom.log` if one process should write to a specific file.
Logs are still written to stderr, so Docker Desktop and terminal output continue to work.
The `logs/` directories and `*.log` files are ignored by Git.

## 8. Backend tests

From `backend`:

```bash
go test ./...
```

From the repository root:

```bash
make backend-test
```

Optional PostgreSQL integration smoke test:

```bash
make db-up
make db-migrate
make backend-integration-test
```

This test is opt-in. It uses `RUN_DB_INTEGRATION_TESTS=1`, writes temporary `ingestion_runs` and `audit_events` rows, verifies repository round trips, and removes the rows.

Current fast test coverage includes:

- case-level handler contract tests without a real database;
- query parameter validation for case list and model comparison endpoints;
- ingestion and analysis handler contract tests;
- health check handler tests for PostgreSQL and ML service status mapping;
- persistent audit middleware tests;
- structured HTTP logger middleware tests;
- HTTP status mapping for invalid input and repository errors;
- model metric unit tests for `Precision@K`, threshold metrics, `ROC-AUC` and `PR-AUC`.

## 9. Frontend DB snapshot

For frontend work on a weak laptop, use a prepared PostgreSQL snapshot instead of running dataset loaders and ML scorers.

See:

```text
backend/docs/frontend_db_snapshot.md
```

## 10. Case Model Scores

The project keeps `case_scores` as the active score used by existing API/UI paths. Multiple model outputs are stored separately in `case_model_scores`.

Run the ML case scorer for the transformer text model:

```bash
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
ML_SERVICE_URL='http://localhost:8000' \
CASE_SCORERS=text \
SOURCE_NAME=pheme_large \
DATASET_NAME=pheme \
DATASET_SPLIT=eventcv_large \
CASE_LIMIT=10 \
ONLY_UNSCORED=true \
go run ./cmd/case_ml_scorer
```

Use `CASE_SCORERS=feature,text` when both the explainable case model and the transformer text model should be computed in one run.

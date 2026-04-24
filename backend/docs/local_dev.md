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
curl 'http://localhost:8080/api/ingestion/runs?limit=10'
curl 'http://localhost:8080/api/analysis/summary?source_type=dataset&dataset_name=sample&dataset_split=dev'
```

## 6. Case Model Scores

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

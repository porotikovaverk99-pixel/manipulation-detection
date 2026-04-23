COMPOSE ?= docker compose
POSTGRES_SERVICE ?= postgres
POSTGRES_DB ?= manipulation_detection
POSTGRES_USER ?= postgres
DATABASE_URL ?= postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable

.PHONY: db-up db-down db-reset db-logs db-shell db-migrate db-status db-version db-down-migration backend-test ml-venv ml-install py-check

db-up:
	$(COMPOSE) up -d --wait $(POSTGRES_SERVICE)

db-down:
	$(COMPOSE) down

db-reset:
	$(COMPOSE) down -v
	$(COMPOSE) up -d --wait $(POSTGRES_SERVICE)
	cd backend && DATABASE_URL='$(DATABASE_URL)' go run ./cmd/db_migrate -dir ./migrations up

db-logs:
	$(COMPOSE) logs -f $(POSTGRES_SERVICE)

db-shell:
	$(COMPOSE) exec $(POSTGRES_SERVICE) psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)

db-migrate:
	cd backend && DATABASE_URL='$(DATABASE_URL)' go run ./cmd/db_migrate -dir ./migrations up

db-status:
	cd backend && DATABASE_URL='$(DATABASE_URL)' go run ./cmd/db_migrate -dir ./migrations status

db-version:
	cd backend && DATABASE_URL='$(DATABASE_URL)' go run ./cmd/db_migrate -dir ./migrations version

db-down-migration:
	cd backend && DATABASE_URL='$(DATABASE_URL)' go run ./cmd/db_migrate -dir ./migrations down

backend-test:
	cd backend && go test ./...

ml-venv:
	cd ml && python3 -m venv .venv

ml-install: ml-venv
	cd ml && .venv/bin/pip install -r requirements.txt

py-check:
	cd ml && .venv/bin/python -m py_compile src/main.py src/db_integration.py src/analyzer.py src/advanced_analyzer.py

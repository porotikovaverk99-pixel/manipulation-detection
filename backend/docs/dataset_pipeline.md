# Dataset-first pipeline

Этот документ фиксирует текущий минимальный воспроизводимый путь для backend MVP:

`dataset -> canonical posts -> ML analysis -> analysis_results/evidence_cards`

и ближайший переход к:

`dataset -> cases + posts -> case-level analysis`.

## 1. Что уже поддерживается

- `cmd/dataset_loader` загружает JSONL в `accounts` и `posts`.
- `ingestion_runs` хранит run-level метаданные загрузки.
- `cmd/dataset_analyzer` выбирает dataset-посты, вызывает ML `/analyze/advanced` и сохраняет результаты в Postgres.
- `GET /api/ingestion/runs` показывает последние загрузки.
- `GET /api/analysis/summary` показывает агрегаты по risk levels.

## 2. Минимальная JSONL-схема

Каждая строка - один JSON-объект. Loader понимает несколько вариантов названий полей:

- `id`, `external_id` или `post_id`
- `username`, `author` или `user`
- `content`, `text` или `body`
- `published_at`, `timestamp` или `created_at`
- `url` или `post_url`
- `lang` или `language`
- `likes_count`, `reposts_count`, `replies_count`

Пример лежит в `backend/testdata/sample_dataset.jsonl`.

## 3. Загрузка тестового слайса

Сначала подними локальный Postgres из корня репозитория:

```bash
make db-up
make db-migrate
```

Из директории `backend`:

```bash
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
DATASET_PATH=./testdata/sample_dataset.jsonl \
DATASET_NAME=sample \
DATASET_SPLIT=dev \
go run ./cmd/dataset_loader
```

После загрузки:

```bash
curl 'http://localhost:8080/api/ingestion/runs?limit=10'
```

## 4. Batch-анализ dataset-постов

Перед запуском должен работать ML-сервис на `ML_SERVICE_URL`:

```bash
cd ../ml
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt

DATABASE_URL='postgresql://postgres:password@localhost:5432/manipulation_detection' \
ENABLE_LIVE_COMMENT_FETCH=false \
ML_SAVE_ANALYSIS_RESULTS=false \
PYTHONPATH=src \
.venv/bin/uvicorn main:app --host 0.0.0.0 --port 8000
```

Затем из директории `backend`:

```bash
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
ML_SERVICE_URL=http://localhost:8000 \
SOURCE_TYPE=dataset \
DATASET_NAME=sample \
DATASET_SPLIT=dev \
BATCH_LIMIT=100 \
go run ./cmd/dataset_analyzer
```

Проверка агрегатов:

```bash
curl 'http://localhost:8080/api/analysis/summary?source_type=dataset&dataset_name=sample&dataset_split=dev'
```

## 5. Загрузка PHEME threads

Новая команда:

```bash
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
PHEME_ARCHIVE_PATH='/absolute/path/to/datasets/raw/pheme/pheme-rnr-dataset.tar.bz2' \
DATASET_NAME=pheme \
DATASET_SPLIT=dev \
SOURCE_NAME=pheme \
PHEME_LIMIT_CASES=10 \
go run ./cmd/pheme_loader
```

Что делает команда:

- читает PHEME archive;
- создает `cases`;
- сохраняет source tweet и reactions в `posts`;
- ставит `posts.case_id`;
- создает `ingestion_run`.

## 6. Case-level scoring

Сейчас поддерживаются два режима:

- `cmd/case_scorer` - локальный Go baseline без внешнего ML сервиса;
- `cmd/case_ml_scorer` - case-level scoring через Python ML endpoint `/analyze/case`.

Для дипломного основного контура предпочтителен второй путь, потому что он ближе к будущей обучаемой ML-части и уже держит единый контракт `backend -> ml service -> postgres`.

### 6.1 Go baseline scorer

После загрузки кейсов:

```bash
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
DATASET_NAME=pheme \
DATASET_SPLIT=dev \
SOURCE_NAME=pheme \
CASE_LIMIT=50 \
ONLY_UNSCORED=true \
go run ./cmd/case_scorer
```

Что делает команда:

- выбирает `cases`;
- загружает связанные `posts`;
- считает baseline case features;
- сохраняет `case_features`;
- сохраняет `case_scores`.

### 6.2 ML case scorer

Перед запуском должен работать ML-сервис:

```bash
cd ../ml

DATABASE_URL='postgresql://postgres:password@localhost:5432/manipulation_detection' \
ML_SAVE_ANALYSIS_RESULTS=false \
PYTHONPATH=src \
.venv/bin/uvicorn main:app --host 127.0.0.1 --port 8000
```

Затем из директории `backend`:

```bash
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
ML_SERVICE_URL='http://127.0.0.1:8000' \
DATASET_NAME=pheme \
DATASET_SPLIT=dev \
SOURCE_NAME=pheme \
CASE_LIMIT=50 \
ONLY_UNSCORED=true \
go run ./cmd/case_ml_scorer
```

Что делает команда:

- выбирает `cases`;
- загружает связанные `posts`;
- отправляет case в `POST /analyze/case`;
- сохраняет `case_features`;
- сохраняет `case_scores`;
- пишет в `feature_payload.scoring_source = ml_service`.

Текущая реализация `/analyze/case` пока использует прозрачный heuristic baseline, но уже живёт в Python ML service. Это даёт стабильную точку для следующего шага: заменить или откалибровать scorer без переписывания backend pipeline.

## 7. API inspection

Новый endpoint:

```bash
curl 'http://localhost:8080/api/cases?dataset_name=pheme&dataset_split=dev&limit=20'
```

Он возвращает:

- case metadata;
- post count;
- risk score;
- component scores (`temporal`, `coordination`, `content`).

## 8. Evaluation

После case scoring можно посчитать и сохранить агрегированные метрики:

```bash
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
DATASET_NAME=pheme \
DATASET_SPLIT=dev \
SOURCE_NAME=pheme \
POSITIVE_LABELS='rumour' \
RISK_THRESHOLD='0.40' \
OUTPUT_PATH='/absolute/path/to/evaluation_outputs/pheme_metrics.json' \
go run ./cmd/evaluate_cases
```

Что делает команда:

- берет scored cases из `cases + case_scores`;
- сортирует их по `risk_score`;
- считает `Precision@10`, `Precision@20`, threshold `Precision/Recall/F1`, `ROC-AUC`, `PR-AUC`;
- сохраняет результат в `evaluation_runs`;
- пишет JSON summary в `OUTPUT_PATH` или stdout.

## 9. Export top cases

Для ручного разбора suspicious threads:

```bash
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
DATASET_NAME=pheme \
DATASET_SPLIT=dev \
SOURCE_NAME=pheme \
EXPORT_LIMIT=25 \
MIN_RISK_SCORE='0.30' \
OUTPUT_PATH='/absolute/path/to/evaluation_outputs/pheme_top_cases.json' \
go run ./cmd/export_top_cases
```

Что делает команда:

- выбирает scored cases;
- сортирует их по `risk_score DESC`;
- сохраняет case metadata, component scores, evidence и labels в JSON.

## 10. Как это переносится на PHEME/SNAP

Для `PHEME` и `SNAP` нужен только адаптер подготовки входа к текущему контракту. Downstream не меняется:

- loader пишет те же `posts`;
- post-level analyzer вызывает тот же ML endpoint;
- case-level analyzer вызывает `/analyze/case`;
- результаты лежат в `case_features` и `case_scores`;
- live-track позже подключается через тот же контракт, но с `source_type=live`.

Для `PHEME` уже работает текущий путь:

- `PHEME thread = case`;
- `source-tweet + reactions` грузятся как `posts`, привязанные к `case_id`;
- затем case scoring идёт через Go baseline или через Python ML service.

## 11. Почему пока без RabbitMQ/Kafka

Для дипломного MVP pipeline batch/replay проще и воспроизводимее. Брокер понадобится только если появятся несколько live-источников, независимые воркеры, backpressure и reprocessing в near-real-time режиме.

## 12. Bot side-track

Дополнительно к case-level pipeline теперь есть account-level bot baseline на `Cresci-2017`.

Тренировка модели:

```bash
cd ../ml

CRESCI_ZIP_PATH='/absolute/path/to/datasets/raw/bots/cresci-2017.csv.zip' \
BOT_OUTPUT_DIR='/absolute/path/to/evaluation_outputs/bot_detector_run1' \
MAX_USERS_PER_SUBSET=300 \
.venv/bin/python src/train_bot_detector.py
```

Артефакты:

- `bot_detector_metrics.json`
- `bot_detector_model.pkl`
- `bot_training_dataset_sample.csv`

Runtime endpoint:

```bash
BOT_MODEL_PATH='/absolute/path/to/bot_detector_model.pkl' \
PYTHONPATH=src \
.venv/bin/uvicorn main:app --host 127.0.0.1 --port 8000
```

И затем:

```bash
curl -X POST http://127.0.0.1:8000/analyze/bot-account \
  -H 'Content-Type: application/json' \
  --data '{...}'
```

Этот трек пока живёт отдельно от case scorer. Следующий шаг - добавить `bot_score` как дополнительный signal в case-level features.

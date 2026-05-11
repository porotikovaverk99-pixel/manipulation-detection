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
- сохраняет активный score в `case_scores`;
- сохраняет тот же model-specific score в `case_model_scores` с ключом `case_feature`.

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
- сохраняет активный explainable score в `case_scores`;
- сохраняет model-specific score в `case_model_scores`;
- пишет в `feature_payload.scoring_source = ml_service`.

По умолчанию `cmd/case_ml_scorer` запускает только explainable feature scorer:

```bash
CASE_SCORERS=feature
```

Для transformer text score нужен ML service с `PHEME_TRANSFORMER_MODEL_PATH`, затем:

```bash
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
ML_SERVICE_URL='http://127.0.0.1:8000' \
CASE_SCORERS=text \
DATASET_NAME=pheme \
DATASET_SPLIT=eventcv_large \
SOURCE_NAME=pheme_large \
CASE_LIMIT=50 \
ONLY_UNSCORED=true \
go run ./cmd/case_ml_scorer
```

Поддерживаемые значения:

- `CASE_SCORERS=feature` - вызывает `POST /analyze/case`, пишет `case_feature`;
- `CASE_SCORERS=text` - вызывает `POST /analyze/case-text`, пишет `pheme_transformer_text`;
- `CASE_SCORERS=feature,text` или `CASE_SCORERS=all` - запускает оба scorer-а.

`ONLY_UNSCORED=true` теперь учитывает выбранные `case_model_scores`: например, для `CASE_SCORERS=text` будут выбраны cases, где ещё нет `pheme_transformer_text`.

`/analyze/case` теперь работает в двух режимах:

- по умолчанию - прозрачный heuristic baseline;
- если задан `CASE_MODEL_PATH` - обученная case-level модель (`case-logreg-v1`).

Это позволяет не переписывать backend pipeline при переходе от baseline к supervised scoring.

Текущий основной feature extractor:

- `case-features-ml-v4`;
- добавляет thread/case-level признаки по временным окнам, peak activity, reply-tree shape, root/reaction similarity, reaction markers и lexical diversity.

Текущий предпочтительный trained artifact для основного PHEME-трека:

- `evaluation_outputs/case_detector_run4_richer/case_detector_model.pkl`

Почему именно он:

- лучше `case-logreg-v1` по `ROC-AUC`, `PR-AUC` и `Precision@20`;
- лучше, чем вариант `richer + portable account-risk`, если account-risk не считать главным треком.

Тренировка первой case-level модели:

```bash
cd ../ml

DATABASE_URL='postgresql://postgres:password@localhost:5432/manipulation_detection' \
CASE_MODEL_OUTPUT_DIR='/absolute/path/to/evaluation_outputs/case_detector_run1' \
CASE_DATASET_NAME='pheme' \
CASE_DATASET_SPLITS='eventcv' \
CASE_SOURCE_NAME='pheme' \
.venv/bin/python src/train_case_detector.py
```

Артефакты:

- `case_detector_model.pkl`
- `case_detector_metrics.json`
- `case_training_dataset_sample.csv`
- `case_detector_oof_predictions.csv`

Runtime c обученной моделью:

```bash
CASE_MODEL_PATH='/absolute/path/to/case_detector_model.pkl' \
PYTHONPATH=src \
.venv/bin/uvicorn main:app --host 127.0.0.1 --port 8000
```

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

Обновление:

- `bot_score` уже протянут в case-level feature extraction;
- `case_ml_scorer` теперь передает расширенные account-level поля по авторам кейса;
- `/analyze/case` при наличии `BOT_MODEL_PATH` добавляет в `feature_payload` агрегаты `bot_score_mean`, `bot_score_max`, `bot_high_share`, `root_author_bot_score`.

Дальше можно переобучить case-level модель уже на bot-aware признаках:

```bash
cd ../ml

DATABASE_URL='postgresql://postgres:password@localhost:5432/manipulation_detection' \
CASE_MODEL_OUTPUT_DIR='/absolute/path/to/evaluation_outputs/case_detector_run2_bot' \
CASE_MODEL_VERSION='case-logreg-v2-bot' \
CASE_DATASET_NAME='pheme' \
CASE_DATASET_SPLITS='eventcv' \
CASE_SOURCE_NAME='pheme' \
.venv/bin/python src/train_case_detector.py
```

Практическое ограничение текущего шага:

- `Cresci-2017` bot model на `PHEME` даёт почти saturated bot scores;
- это означает domain shift и требует отдельной калибровки/перепроверки, прежде чем использовать bot signal как сильный научный аргумент.

# PHEME Advanced Training

This document records the two stronger PHEME model tracks:

- tabular gradient boosting on engineered case features;
- transformer fine-tuning on case text (`root tweet + reactions`).

The baseline remains `case-logreg-v6-richer-large`. These models are evaluated with the same event-level protocol: leave one PHEME event out.

## Baseline Reference

`case-logreg-v6-richer-large`:

- rows: `1000`;
- features: `68`;
- evaluation: leave-one-event-out;
- Precision@10: `0.800`;
- Precision@20: `0.750`;
- F1: `0.663`;
- ROC-AUC: `0.633`;
- PR-AUC: `0.641`.

## LightGBM On Case Features

Script:

```bash
ml/src/train_case_boosting_detector.py
```

Install optional dependency:

```bash
ml/.venv/bin/python -m pip install -r ml/requirements-boosting.txt
```

Run:

```bash
DATABASE_URL='postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable' \
CASE_MODEL_OUTPUT_DIR='/home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/case_boosting_lightgbm_run1' \
CASE_MODEL_VERSION='case-lightgbm-v1' \
CASE_BOOSTING_MODEL_KIND='lightgbm' \
CASE_DATASET_NAME='pheme' \
CASE_SOURCE_NAME='pheme_large' \
CASE_DATASET_SPLITS='eventcv_large' \
ml/.venv/bin/python ml/src/train_case_boosting_detector.py
```

Observed result:

- Precision@10: `0.900`;
- Precision@20: `0.900`;
- F1: `0.667`;
- ROC-AUC: `0.629`;
- PR-AUC: `0.647`.

Interpretation:

- LightGBM improves top-K triage ranking;
- global separation stays close to logistic regression;
- event shift remains visible, especially on `ferguson`.

## DistilRoBERTa On PHEME Case Text

Script:

```bash
ml/src/train_pheme_transformer_detector.py
```

Input construction:

- one row = one PHEME case/thread;
- text = root tweet + up to `8` early reactions;
- validation = leave-one-event-out;
- positive class = `rumour`.

Run on ROCm Docker:

```bash
docker run --rm \
  --network host \
  --device=/dev/kfd \
  --device=/dev/dri \
  --group-add 44 \
  --group-add 110 \
  --ipc=host \
  --shm-size 8G \
  -e HIP_VISIBLE_DEVICES=0 \
  -e CUDA_VISIBLE_DEVICES=0 \
  -e ROCR_VISIBLE_DEVICES=0 \
  -e HF_HOME=/workspace/.cache/huggingface \
  -e PIP_CACHE_DIR=/tmp/pip-cache \
  -v /home/richt/Documents/coding/cursor_fun/dplm_tst:/workspace \
  -w /workspace/manipulation-detection \
  rocm/pytorch:latest \
  bash -lc 'python3 -m pip install -q -r ml/requirements-rocm-training.txt && python3 ml/src/train_pheme_transformer_detector.py \
    --database-url "postgresql://postgres:password@localhost:5432/manipulation_detection" \
    --model-name distilroberta-base \
    --model-version pheme-distilroberta-root-reactions-v1 \
    --output-dir /workspace/evaluation_outputs/pheme_transformer_root_reactions_run1 \
    --epochs 3 \
    --batch-size 16 \
    --max-length 192 \
    --max-reactions 8 \
    --save-final-model'
```

Observed result:

- device: `AMD Radeon RX 7800 XT`;
- rows: `1000`;
- evaluated rows: `1000`;
- text mode: `root_reactions`;
- epochs: `3`;
- Precision@10: `0.900`;
- Precision@20: `0.850`;
- Precision: `0.712`;
- Recall: `0.826`;
- F1: `0.765`;
- ROC-AUC: `0.821`;
- PR-AUC: `0.787`;
- training seconds inside script: `378.3`;
- full Docker wall time: `504s`.

Event-level result:

| Held-out event | F1 | ROC-AUC | PR-AUC |
|---|---:|---:|---:|
| `charliehebdo` | `0.860` | `0.861` | `0.832` |
| `ferguson` | `0.644` | `0.820` | `0.770` |
| `germanwings-crash` | `0.730` | `0.757` | `0.728` |
| `ottawashooting` | `0.761` | `0.848` | `0.867` |
| `sydneysiege` | `0.788` | `0.853` | `0.790` |

Interpretation:

- This is the strongest PHEME model so far by ROC-AUC, PR-AUC, and F1.
- It directly uses textual evidence from source tweet and reactions.
- It is less explainable than logistic regression and LightGBM, so the diploma should present it as a stronger model, while keeping the feature-based model for interpretable evidence cards.

## Current Decision

Use three tiers in the diploma:

- interpretable baseline: `case-logreg-v6-richer-large`;
- stronger tabular ranking model: `case-lightgbm-v1`;
- strongest text model: `pheme-distilroberta-root-reactions-v1`.

The recommended main scientific result can now be framed as:

> A unified case-level pipeline where interpretable engineered features provide evidence cards, and a transformer text model improves event-level predictive performance.

## Runtime Usage

The saved PHEME transformer can now be used outside the training script.

Runtime wrapper:

```bash
ml/src/pheme_transformer_analyzer.py
```

CLI smoke runner:

```bash
ml/src/run_pheme_transformer_case.py
```

FastAPI endpoint:

```text
POST /analyze/case-text
```

It accepts the same payload shape as `POST /analyze/case` and returns:

- `risk_score`;
- `risk_level`;
- `confidence_score`;
- transformer model metadata;
- compact evidence such as `text_model_score`, threshold, and number of reactions used.

The current `/analyze/case` endpoint is intentionally unchanged. It remains the interpretable feature/evidence analyzer used by the backend case scorer. The transformer endpoint is separate so we can compare and later combine both signals without silently changing stored case scores.

Run one DB-backed case through the saved model in ROCm Docker:

```bash
docker run --rm \
  --network host \
  --device=/dev/kfd \
  --device=/dev/dri \
  --group-add 44 \
  --group-add 110 \
  --ipc=host \
  --shm-size 8G \
  -e HIP_VISIBLE_DEVICES=0 \
  -e CUDA_VISIBLE_DEVICES=0 \
  -e ROCR_VISIBLE_DEVICES=0 \
  -e HF_HOME=/workspace/.cache/huggingface \
  -e PIP_CACHE_DIR=/tmp/pip-cache \
  -v /home/richt/Documents/coding/cursor_fun/dplm_tst:/workspace \
  -w /workspace/manipulation-detection \
  rocm/pytorch:latest \
  bash -lc 'python3 -m pip install -q -r ml/requirements-rocm-training.txt && python3 ml/src/run_pheme_transformer_case.py \
    --database-url "postgresql://postgres:password@localhost:5432/manipulation_detection" \
    --model-path /workspace/evaluation_outputs/pheme_transformer_root_reactions_run1/model \
    --case-id 1 \
    --pretty'
```

Run the ML API with the transformer endpoint enabled:

```bash
PHEME_TRANSFORMER_MODEL_PATH='/home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/pheme_transformer_root_reactions_run1/model' \
ml/.venv/bin/uvicorn main:app --host 0.0.0.0 --port 8000 --app-dir ml/src
```

For GPU runtime, start the service from the ROCm Docker image instead of the local CPU virtualenv.

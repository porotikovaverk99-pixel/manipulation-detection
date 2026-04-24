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

## Longer DistilRoBERTa Check

We also tested whether the same `root_reactions` setup improves with more epochs:

```text
model_version = pheme-distilroberta-root-reactions-v2-5epochs
epochs = 5
learning_rate = 1e-5
max_reactions = 8
max_length = 192
output = /workspace/evaluation_outputs/pheme_transformer_root_reactions_5epochs_run1
```

Observed result:

- Precision@10: `0.900`;
- Precision@20: `0.800`;
- F1: `0.741`;
- ROC-AUC: `0.791`;
- PR-AUC: `0.764`;
- training seconds: `622.1`.

This is worse than the 3-epoch `pheme-distilroberta-root-reactions-v1` run:

- Precision@20 dropped from `0.850` to `0.800`;
- F1 dropped from `0.765` to `0.741`;
- ROC-AUC dropped from `0.821` to `0.791`;
- PR-AUC dropped from `0.787` to `0.764`.

Conclusion:

- simple longer fine-tuning is not the right improvement path for PHEME;
- keep `pheme-distilroberta-root-reactions-v1` as the current best text model;
- next useful neural checks are `root_only` control, longer context (`max_length=256/384`), or a stronger encoder such as `roberta-base` / `deberta-v3-small`.

## RoBERTa-base Check

We also tested a stronger encoder with the same text construction and validation protocol:

```text
model_version = pheme-roberta-base-root-reactions-v1
base_model = roberta-base
epochs = 3
learning_rate = 2e-5
max_reactions = 8
max_length = 192
batch_size = 16
output = /workspace/evaluation_outputs/pheme_roberta_base_root_reactions_run1
```

Observed result:

- device: `AMD Radeon RX 7800 XT` through ROCm Docker;
- rows: `1000`;
- evaluated rows: `1000`;
- Precision@10: `0.800`;
- Precision@20: `0.750`;
- Precision: `0.726`;
- Recall: `0.794`;
- F1: `0.758`;
- ROC-AUC: `0.812`;
- PR-AUC: `0.775`;
- training seconds: `781.1`.

Comparison with the current best DistilRoBERTa run:

| Model | Precision@10 | Precision@20 | Precision | Recall | F1 | ROC-AUC | PR-AUC | Training seconds |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| `pheme-distilroberta-root-reactions-v1` | `0.900` | `0.850` | `0.712` | `0.826` | `0.765` | `0.821` | `0.787` | `378.3` |
| `pheme-roberta-base-root-reactions-v1` | `0.800` | `0.750` | `0.726` | `0.794` | `0.758` | `0.812` | `0.775` | `781.1` |

Event-level result:

| Held-out event | F1 | ROC-AUC | PR-AUC |
|---|---:|---:|---:|
| `charliehebdo` | `0.857` | `0.869` | `0.838` |
| `ferguson` | `0.490` | `0.821` | `0.756` |
| `germanwings-crash` | `0.734` | `0.765` | `0.747` |
| `ottawashooting` | `0.821` | `0.873` | `0.878` |
| `sydneysiege` | `0.779` | `0.852` | `0.795` |

Conclusion:

- RoBERTa-base trained successfully on the RX 7800 XT through ROCm;
- it did not beat the lighter DistilRoBERTa model on the main quality metrics;
- the result supports keeping `pheme-distilroberta-root-reactions-v1` as the default runtime text model;
- for the next neural step, prefer richer input/context checks over simply increasing model size.

## Text Input Ablations

We tested whether the current `root tweet + reactions` input is justified, and whether a longer token budget helps.

### Root-only Control

```text
model_version = pheme-distilroberta-root-only-v1
base_model = distilroberta-base
text_mode = root_only
epochs = 3
learning_rate = 2e-5
max_length = 192
max_reactions = 0
output = /workspace/evaluation_outputs/pheme_transformer_root_only_run1
```

Observed result:

- Precision@10: `0.800`;
- Precision@20: `0.850`;
- Precision: `0.734`;
- Recall: `0.780`;
- F1: `0.757`;
- ROC-AUC: `0.814`;
- PR-AUC: `0.796`;
- training seconds: `91.8`.

Comparison:

| Model | Input | Precision@10 | Precision@20 | Precision | Recall | F1 | ROC-AUC | PR-AUC | Training seconds |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| `pheme-distilroberta-root-only-v1` | root only | `0.800` | `0.850` | `0.734` | `0.780` | `0.757` | `0.814` | `0.796` | `91.8` |
| `pheme-distilroberta-root-reactions-v1` | root + 8 reactions | `0.900` | `0.850` | `0.712` | `0.826` | `0.765` | `0.821` | `0.787` | `378.3` |

Interpretation:

- root-only is a strong and much faster baseline;
- reactions improve top-10 triage, recall, F1, and ROC-AUC, but the gain is modest on PHEME;
- the project should keep both modes conceptually:
  - root-only as a fast fallback when a case has no reactions yet;
  - root + reactions as the fuller case-level detector.

### Longer Context For Reactions

```text
model_version = pheme-distilroberta-root-reactions-len256-v1
base_model = distilroberta-base
text_mode = root_reactions
epochs = 3
learning_rate = 2e-5
max_length = 256
max_reactions = 8
output = /workspace/evaluation_outputs/pheme_transformer_root_reactions_len256_run1
```

Observed result:

- Precision@10: `1.000`;
- Precision@20: `0.800`;
- Precision: `0.695`;
- Recall: `0.844`;
- F1: `0.762`;
- ROC-AUC: `0.814`;
- PR-AUC: `0.782`;
- training seconds: `527.8`.

Comparison with the current default:

| Model | Max length | Precision@10 | Precision@20 | Precision | Recall | F1 | ROC-AUC | PR-AUC | Training seconds |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| `pheme-distilroberta-root-reactions-v1` | `192` | `0.900` | `0.850` | `0.712` | `0.826` | `0.765` | `0.821` | `0.787` | `378.3` |
| `pheme-distilroberta-root-reactions-len256-v1` | `256` | `1.000` | `0.800` | `0.695` | `0.844` | `0.762` | `0.814` | `0.782` | `527.8` |

Interpretation:

- a longer context improves the very top of the ranking;
- it does not improve the general detector quality and is slower;
- keep `max_length=192` as the default runtime setting;
- mention `max_length=256` as an optional aggressive top-10 triage variant, not as the main model.

## Updated Neural Decision

Current recommended transformer setup:

- default runtime text model: `pheme-distilroberta-root-reactions-v1`;
- default input: `root tweet + up to 8 reactions`;
- default `max_length`: `192`;
- fast fallback: `pheme-distilroberta-root-only-v1`;
- rejected as default:
  - `pheme-distilroberta-root-reactions-v2-5epochs`;
  - `pheme-roberta-base-root-reactions-v1`;
  - `pheme-distilroberta-root-reactions-len256-v1`.

The next engineering step is no longer single-model tuning. It is model comparison and ensembling through `case_model_scores`:

- feature/logistic score;
- LightGBM case-feature score;
- transformer text score;
- optional ensemble score.

## Case Model Scores And Ensemble

Two utility scripts now connect offline evaluation artifacts with the shared `case_model_scores` table.

Import OOF predictions:

```bash
DATABASE_URL='postgresql://postgres:password@localhost:5432/manipulation_detection' \
ml/.venv/bin/python ml/src/import_case_model_scores.py \
  --predictions-csv /home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/case_detector_run6_richer_large/case_detector_oof_predictions.csv \
  --metrics-json /home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/case_detector_run6_richer_large/case_detector_metrics.json \
  --scorer-key case_logreg_oof \
  --source-endpoint offline_oof_import/logreg

DATABASE_URL='postgresql://postgres:password@localhost:5432/manipulation_detection' \
ml/.venv/bin/python ml/src/import_case_model_scores.py \
  --predictions-csv /home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/case_boosting_lightgbm_run1/case_boosting_oof_predictions.csv \
  --metrics-json /home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/case_boosting_lightgbm_run1/case_boosting_detector_metrics.json \
  --scorer-key case_lightgbm_oof \
  --source-endpoint offline_oof_import/lightgbm

DATABASE_URL='postgresql://postgres:password@localhost:5432/manipulation_detection' \
ml/.venv/bin/python ml/src/import_case_model_scores.py \
  --predictions-csv /home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/pheme_transformer_root_reactions_run1/pheme_transformer_oof_predictions.csv \
  --metrics-json /home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/pheme_transformer_root_reactions_run1/pheme_transformer_metrics.json \
  --scorer-key pheme_transformer_text_oof \
  --source-endpoint offline_oof_import/pheme_transformer
```

Compare scores and write a weighted ensemble:

```bash
DATABASE_URL='postgresql://postgres:password@localhost:5432/manipulation_detection' \
ml/.venv/bin/python ml/src/compare_case_model_scores.py \
  --weights 'case_logreg_oof=0,case_lightgbm_oof=0.35,pheme_transformer_text_oof=0.65' \
  --output-dir /home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/case_model_score_comparison_run1 \
  --write-ensemble
```

Rows imported into `case_model_scores`:

| scorer_key | model_version | rows |
|---|---|---:|
| `case_logreg_oof` | `case-logreg-v6-richer-large` | `1000` |
| `case_lightgbm_oof` | `case-lightgbm-v1` | `1000` |
| `pheme_transformer_text_oof` | `pheme-distilroberta-root-reactions-v1` | `1000` |
| `case_ensemble_v1` | `case-ensemble-v1` | `1000` |

Comparison on `PHEME / pheme_large / eventcv_large`:

| scorer_key | Precision@10 | Precision@20 | F1 | ROC-AUC | PR-AUC |
|---|---:|---:|---:|---:|---:|
| `case_logreg_oof` | `0.800` | `0.750` | `0.667` | `0.633` | `0.641` |
| `case_lightgbm_oof` | `0.900` | `0.900` | `0.667` | `0.629` | `0.647` |
| `pheme_transformer_text_oof` | `0.900` | `0.850` | `0.765` | `0.821` | `0.787` |
| `case_ensemble_v1` | `0.900` | `0.950` | `0.766` | `0.814` | `0.805` |

Interpretation:

- the transformer remains the strongest standalone detector;
- LightGBM is useful for top-K ranking despite weak global separation;
- logistic regression remains the interpretable baseline, but its weight is `0` in `case_ensemble_v1` because it dilutes ranking quality;
- `case_ensemble_v1` is best framed as a triage/ranking score: it improves `Precision@20` and `PR-AUC`, while ROC-AUC stays slightly below the transformer alone.

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

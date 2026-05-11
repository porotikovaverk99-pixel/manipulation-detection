# Fakeddit Training

This track is secondary to the main PHEME case-level detector. Its purpose is to provide a stronger content/discussion benchmark for suspicious or fake content detection.

## Current Baseline

`ml/src/train_fakeddit_text_detector.py` trains a reproducible text baseline:

- input: `all_train.tsv`, `all_validate.tsv`, `all_test_public.tsv`;
- text field: `clean_title`, fallback to `title`;
- label: `2_way_label`;
- default positive class: `0`, treated as suspicious/fake;
- model: word + character TF-IDF with logistic regression;
- outputs: model bundle, metrics JSON, validation/test predictions, top suspicious validation cases.

The baseline is intentionally CPU-friendly. It gives us a comparison point before training heavier transformer models on ROCm.

## Smoke Test

From the workspace root:

```bash
manipulation-detection/ml/.venv/bin/python manipulation-detection/ml/src/train_fakeddit_text_detector.py \
  --max-train-rows 2000 \
  --max-eval-rows 1000 \
  --max-features 20000 \
  --output-dir /home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/fakeddit_text_detector_smoke \
  --model-version fakeddit-tfidf-logreg-smoke
```

## Larger Baseline

```bash
manipulation-detection/ml/.venv/bin/python manipulation-detection/ml/src/train_fakeddit_text_detector.py \
  --max-train-rows 100000 \
  --max-eval-rows 50000 \
  --max-features 120000 \
  --output-dir /home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/fakeddit_text_detector_run1 \
  --model-version fakeddit-tfidf-logreg-v1
```

Use `--max-train-rows 0 --max-eval-rows 0` only when we intentionally want a full split run.

Observed `fakeddit-tfidf-logreg-v1` metrics on `100k train / 50k validation / 50k test_public`:

- validation: `F1 = 0.813`, `ROC-AUC = 0.890`, `PR-AUC = 0.894`, `Precision@500 = 1.00`;
- test_public: `F1 = 0.811`, `ROC-AUC = 0.892`, `PR-AUC = 0.894`, `Precision@500 = 1.00`.

## ROCm PyTorch Check

The host has been verified with:

```bash
docker run --rm \
  --device=/dev/kfd \
  --device=/dev/dri \
  --group-add 44 \
  --group-add 110 \
  --ipc=host \
  --shm-size 8G \
  rocm/pytorch:latest \
  python3 -c "import torch; print(torch.cuda.is_available()); print(torch.cuda.get_device_name(0))"
```

Expected GPU device:

```text
AMD Radeon RX 7800 XT
```

## GPU Baseline

`ml/src/train_fakeddit_torch_text_detector.py` trains a real PyTorch model on CPU/GPU:

- model: `EmbeddingBag + MLP`;
- input: tokenized `clean_title`, fallback to `title`;
- label: `2_way_label`;
- default positive class: `0`;
- intended device: `cuda:0` in `rocm/pytorch:latest`.

Run from the host:

```bash
docker run --rm \
  --device=/dev/kfd \
  --device=/dev/dri \
  --group-add 44 \
  --group-add 110 \
  --ipc=host \
  --shm-size 8G \
  -v /home/richt/Documents/coding/cursor_fun/dplm_tst:/workspace \
  -w /workspace \
  rocm/pytorch:latest \
  bash -lc 'python3 manipulation-detection/ml/src/train_fakeddit_torch_text_detector.py \
    --max-train-rows 100000 \
    --max-eval-rows 20000 \
    --epochs 4 \
    --batch-size 2048 \
    --vocab-size 50000 \
    --embedding-dim 128 \
    --hidden-dim 128 \
    --output-dir /workspace/evaluation_outputs/fakeddit_torch_text_detector_gpu_run1 \
    --model-version fakeddit-torch-embeddingbag-gpu-v1'
```

Observed `fakeddit-torch-embeddingbag-gpu-v1` run:

- device: `AMD Radeon RX 7800 XT`;
- `torch.cuda.is_available() = True`;
- train sample: `100000`;
- validation sample: `20000`;
- test_public sample: `20000`;
- epochs: `4`;
- training loop time: `3.8s`;
- full Docker command wall time: about `10s`.

Metrics:

- validation: `F1 = 0.757`, `ROC-AUC = 0.838`, `PR-AUC = 0.842`, `Precision@500 = 1.00`;
- test_public: `F1 = 0.757`, `ROC-AUC = 0.837`, `PR-AUC = 0.837`, `Precision@500 = 1.00`.

This confirms that real model training works on the AMD GPU. The TF-IDF baseline is still stronger, so the next GPU model should be a transformer rather than a small `EmbeddingBag` classifier.

## Next Transformer Step

For the heavier model, use `rocm/pytorch:latest` and install only the missing training libraries:

```bash
pip install -r ml/requirements-rocm-training.txt
```

Then implement a transformer trainer using a compact model first:

- `distilroberta-base`;
- or `microsoft/deberta-v3-small`;
- input v1: `clean_title`;
- input v2: `clean_title + selected top comments`;
- metrics: F1, macro-F1, ROC-AUC, PR-AUC, Precision@K.

This should stay separate from the main PHEME pipeline. PHEME answers the case-level manipulation question; Fakeddit answers whether stronger text evidence can improve content-level suspiciousness signals.

## Transformer GPU Fine-Tuning

`ml/src/train_fakeddit_transformer_detector.py` fine-tunes a Hugging Face sequence classifier on the same Fakeddit title task.

Important ROCm detail: the host exposes both the discrete RX 7800 XT and an integrated AMD device. The command must force visibility to the discrete GPU only:

```bash
-e HIP_VISIBLE_DEVICES=0 \
-e CUDA_VISIBLE_DEVICES=0 \
-e ROCR_VISIBLE_DEVICES=0
```

Smoke run:

```bash
docker run --rm \
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
  -w /workspace \
  rocm/pytorch:latest \
  bash -lc 'python3 -m pip install -q "transformers>=4.40,<5" "accelerate>=0.30,<2" "scikit-learn>=1.4,<2" && python3 manipulation-detection/ml/src/train_fakeddit_transformer_detector.py \
    --model-name distilroberta-base \
    --max-train-rows 2000 \
    --max-eval-rows 1000 \
    --epochs 1 \
    --batch-size 16 \
    --max-length 96 \
    --output-dir /workspace/evaluation_outputs/fakeddit_transformer_smoke_gpu \
    --model-version fakeddit-distilroberta-smoke-gpu'
```

Observed smoke result:

- train sample: `2000`;
- validation/test sample: `1000`;
- epochs: `1`;
- training time: `10.5s`;
- full command wall time: `39s`;
- validation: `F1 = 0.704`, `ROC-AUC = 0.826`;
- test_public: `F1 = 0.718`, `ROC-AUC = 0.860`.

Larger transformer run:

```bash
docker run --rm \
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
  -w /workspace \
  rocm/pytorch:latest \
  bash -lc 'python3 -m pip install -q "transformers>=4.40,<5" "accelerate>=0.30,<2" "scikit-learn>=1.4,<2" && python3 manipulation-detection/ml/src/train_fakeddit_transformer_detector.py \
    --model-name distilroberta-base \
    --max-train-rows 20000 \
    --max-eval-rows 5000 \
    --epochs 3 \
    --batch-size 32 \
    --max-length 96 \
    --output-dir /workspace/evaluation_outputs/fakeddit_transformer_gpu_run1 \
    --model-version fakeddit-distilroberta-gpu-v1'
```

Observed `fakeddit-distilroberta-gpu-v1` result:

- device: `AMD Radeon RX 7800 XT`;
- train sample: `20000`;
- validation/test sample: `5000`;
- epochs: `3`;
- training time: `272.9s`;
- full command wall time: `311s`;
- validation: `F1 = 0.823`, `ROC-AUC = 0.907`, `PR-AUC = 0.908`, `Precision@500 = 0.974`;
- test_public: `F1 = 0.831`, `ROC-AUC = 0.910`, `PR-AUC = 0.902`, `Precision@500 = 0.972`.

Comparable 50k evaluation run:

```bash
docker run --rm \
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
  -w /workspace \
  rocm/pytorch:latest \
  bash -lc 'python3 -m pip install -q "transformers>=4.40,<5" "accelerate>=0.30,<2" "scikit-learn>=1.4,<2" && python3 manipulation-detection/ml/src/train_fakeddit_transformer_detector.py \
    --model-name distilroberta-base \
    --max-train-rows 20000 \
    --max-eval-rows 50000 \
    --epochs 3 \
    --batch-size 32 \
    --max-length 96 \
    --output-dir /workspace/evaluation_outputs/fakeddit_transformer_gpu_run2_eval50k \
    --model-version fakeddit-distilroberta-gpu-v2-eval50k'
```

Observed `fakeddit-distilroberta-gpu-v2-eval50k` result:

- device: `AMD Radeon RX 7800 XT`;
- train sample: `20000`;
- validation/test sample: `50000`;
- epochs: `3`;
- training time: `464.5s`;
- full command wall time: `631s`;
- validation: `F1 = 0.827`, `ROC-AUC = 0.910`, `PR-AUC = 0.910`, `Precision@500 = 1.000`;
- test_public: `F1 = 0.826`, `ROC-AUC = 0.910`, `PR-AUC = 0.908`, `Precision@500 = 0.998`.

## Comments Baseline

`ml/src/train_fakeddit_comments_text_detector.py` extends the TF-IDF baseline with top comments:

- input: title text plus up to `N` comments per submission;
- comment source: `comments/all_comments.tsv.zip`;
- matching key: `submission_id -> id`;
- ranking inside a submission: highest `ups`;
- default: `top_comments = 3`, `max_comment_chars = 600`;
- model: word + character TF-IDF with logistic regression.

The comments archive contains long and imperfect TSV rows, so the loader uses a defensive streaming `csv.DictReader` instead of pandas C-parser for the comments file.

Smoke run:

```bash
manipulation-detection/ml/.venv/bin/python manipulation-detection/ml/src/train_fakeddit_comments_text_detector.py \
  --max-train-rows 20000 \
  --max-eval-rows 5000 \
  --top-comments 3 \
  --max-features 80000 \
  --output-dir /home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/fakeddit_comments_text_detector_smoke \
  --model-version fakeddit-comments-tfidf-smoke
```

Observed smoke result:

- train sample: `20000`;
- validation/test sample: `5000`;
- comments coverage: about `0.595`;
- full command wall time: `40s`;
- validation: `F1 = 0.851`, `ROC-AUC = 0.934`, `PR-AUC = 0.935`;
- test_public: `F1 = 0.852`, `ROC-AUC = 0.933`, `PR-AUC = 0.933`.

Medium run:

```bash
manipulation-detection/ml/.venv/bin/python manipulation-detection/ml/src/train_fakeddit_comments_text_detector.py \
  --max-train-rows 50000 \
  --max-eval-rows 20000 \
  --top-comments 3 \
  --max-features 120000 \
  --output-dir /home/richt/Documents/coding/cursor_fun/dplm_tst/evaluation_outputs/fakeddit_comments_text_detector_run1 \
  --model-version fakeddit-comments-tfidf-v1
```

Observed `fakeddit-comments-tfidf-v1` result:

- train sample: `50000`;
- validation/test sample: `20000`;
- comments coverage: about `0.601`;
- full command wall time: `62s`;
- validation: `F1 = 0.865`, `ROC-AUC = 0.943`, `PR-AUC = 0.944`, `Precision@500 = 1.000`;
- test_public: `F1 = 0.864`, `ROC-AUC = 0.941`, `PR-AUC = 0.941`, `Precision@500 = 1.000`.

Current interpretation:

- comments give the largest content-track improvement so far;
- title-only transformer is useful, but comments add more signal in this benchmark;
- this track should feed an optional future `content_suspiciousness_score`, not replace the main PHEME case-level detector.

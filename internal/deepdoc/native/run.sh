#!/usr/bin/env bash
# Run the local Go DeepDoc prototype (DLA / TSR / OCR-rec / DET) and emit each
# task's JSON output. The Python reference scripts that this helper formerly
# compared against have been removed — the Go ports are now validated by the
# in-repo golden/equivalence tests (go test ./internal/deepdoc/native/...),
# so there is no external Python reference to compare with.
#
#   bash run.sh            # uses default MODEL_DIR below
#   MODEL_DIR=... bash run.sh
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# ONNX Runtime is statically linked (libonnxruntime.a) and resolved via
# dlopen(NULL); no ORT_LIB is needed.
#
# Default MODEL_DIR to the canonical repo model dir (rag/res/deepdoc) so running
# `download_go_deps.py` is enough — no MODEL_DIR export is required for local
# Go DeepDoc runs. Override with `MODEL_DIR=... bash run.sh` if needed.
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
MODEL_DIR="${MODEL_DIR:-$REPO_ROOT/rag/res/deepdoc}"
export MODEL_DIR

run_task() {
  # $1 = task name, $2 = image file
  echo "================ $1 ($2) ================"
  go run . -task "$1" -image "testdata/$2"
}

run_task dla     page0.png
run_task tsr     table0.png
run_task ocr-rec line0.png
run_task det     page0.png

echo "================ DONE ================"

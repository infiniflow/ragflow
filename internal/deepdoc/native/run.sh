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
cd "$(dirname "$0")"

# ONNX Runtime is statically linked (libonnxruntime.a) and resolved via
# dlopen(NULL); no ORT_LIB is needed.
MODEL_DIR="${MODEL_DIR:?Set MODEL_DIR to the deepdoc model directory (e.g. .../rag/res/deepdoc)}"
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

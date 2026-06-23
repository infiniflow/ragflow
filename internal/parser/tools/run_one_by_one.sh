#!/bin/bash
# Process PDFs one at a time with Go + Python OCR, compare immediately.
# Already-processed PDFs are skipped (cached).
#
# Usage:
#   cd /home/shenyushi/cc-workspace/ragflow/internal/parser
#   bash tools/run_one_by_one.sh

set -euo pipefail

PARSER_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PROJECT_DIR="/home/shenyushi/cc-workspace/ragflow"
SCRIPT_DIR="$PARSER_DIR/testdata"
PDF_DIR="$SCRIPT_DIR/real_pdfs"
GO_TEXT_DIR="$PARSER_DIR/testdata/output/go/ocr/text"
PY_TEXT_DIR="$PARSER_DIR/testdata/output/py/ocr/text"

CGO_CFLAGS="-I$HOME/pdf_oxide/v0.3.63/include"
CGO_LDFLAGS="$HOME/pdf_oxide/v0.3.63/lib/linux_amd64/libpdf_oxide.a -lm -lpthread -ldl -lrt -lgcc_s -lutil -lc"

KEEP_GOING="${BATCH_KEEP_GOING:-0}"

mkdir -p "$GO_TEXT_DIR" "$PY_TEXT_DIR"
cd "$PARSER_DIR"

echo "=== Collecting PDFs (sorted by size, smallest first) ==="
mapfile -t pdfs < <(ls -S "$PDF_DIR"/*.pdf 2>/dev/null | while read f; do basename "$f"; done | tac)

TOTAL="${#pdfs[@]}"
SUCCESS=0
FAILED=0
COMPARED=0

for i in "${!pdfs[@]}"; do
    name="${pdfs[$i]}"
    short="$(echo "$name" | cut -c1-45)"
    label="[$((i+1))/$TOTAL] $short"
    echo ""
    echo "============================================================"
    echo "$label"
    echo "============================================================"

    go_txt="$GO_TEXT_DIR/$name.txt"
    py_txt="$PY_TEXT_DIR/$name.txt"

    # ── Go ────────────────────────────────────────────────────
    if [ -f "$go_txt" ]; then
        echo "  Go:   SKIP (cached)"
    else
        echo "  Go:   processing..."
        BATCH_SINGLE="$name" \
        CGO_CFLAGS="$CGO_CFLAGS" CGO_LDFLAGS="$CGO_LDFLAGS" \
        go test -tags "cgo,manual" -run TestBatchResults -count=1 -timeout 20m > /tmp/go_one.log 2>&1 || true
        if [ -f "$go_txt" ]; then
            echo "  Go:   OK"
            SUCCESS=$((SUCCESS+1))
        else
            echo "  Go:   FAILED"
            tail -3 /tmp/go_one.log
            FAILED=$((FAILED+1))
            [ "$KEEP_GOING" = "1" ] || exit 1
            continue
        fi
    fi

    # ── Python ─────────────────────────────────────────────────
    if [ -f "$py_txt" ]; then
        echo "  Py:   SKIP (cached)"
    else
        echo "  Py:   processing..."
        cd "$PROJECT_DIR"
        PYTHONPATH="$PROJECT_DIR" python3 "internal/parser/testdata/dump_py_results.py" --single "$name" > /tmp/py_one.log 2>&1 || true
        cd "$PARSER_DIR"
        if [ -f "$py_txt" ]; then
            echo "  Py:   OK"
        else
            echo "  Py:   FAILED"
            tail -5 /tmp/py_one.log
            FAILED=$((FAILED+1))
            [ "$KEEP_GOING" = "1" ] || exit 1
            continue
        fi
    fi

    # ── Compare ────────────────────────────────────────────────
    if [ -f "$go_txt" ] && [ -f "$py_txt" ]; then
        echo "  Compare:"
        PY_OCR_SUFFIX=py \
        CGO_CFLAGS="$CGO_CFLAGS" CGO_LDFLAGS="$CGO_LDFLAGS" \
        go test -tags "cgo,manual" -run TestBatchCompareWithPython -count=1 2>&1 | grep -E "→.*→|CharSim|Summary|Med=" | sed 's/^/    /' || true
        COMPARED=$((COMPARED+1))
    fi
done

echo ""
echo "============================================================"
echo "Done. $TOTAL PDFs, $SUCCESS Go processed, $COMPARED compared, $FAILED failed"
echo "============================================================"

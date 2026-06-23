"""
Generate Python reference results using the PRODUCTION pipeline
(_parse_loaded_window_into_bboxes), capturing per-stage box counts
for comparison with Go's Parse() output.

Pipeline (matching Go Parse):
  __images__  →  charsToBoxes
  _layouts_rec  →  (stub, no-op)
  _table_transformer_job  →  (stub, no-op)
  _text_merge  →  AssignColumn + TextMerge
  _concat_downward  →  sortByPageThenY
  _naive_vertical_merge  →  NaiveVerticalMerge
  [_extract_table_figure  →  skipped in comparison (OCR-dependent)]
  boxes → text_len (without _line_tag)

Usage:
    cd /home/shenyushi/cc-workspace/ragflow
    PYTHONPATH=. uv run python3 internal/parser/tools/dump_aligned_results.py
"""

import json
import os
import sys
import time
import traceback
from pathlib import Path

# Ensure project root is on path
PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
sys.path.insert(0, PROJECT_ROOT)

SCRIPT_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "testdata")
REAL_PDFS_DIR = os.path.join(SCRIPT_DIR, "real_pdfs")
SNAPSHOT_PDFS_DIR = os.path.join(SCRIPT_DIR, "pdfs")

OUT_REAL = os.path.join(SCRIPT_DIR, "real_pdf_results_py_aligned.json")
OUT_SNAPSHOTS_DIR = os.path.join(SCRIPT_DIR, "snapshots_aligned")
OUT_SNAPSHOT_INDEX = os.path.join(SCRIPT_DIR, "snapshots_aligned", "index.json")


def count_boxes(parser):
    """Count boxes — parser.boxes may be list-of-lists or flat list."""
    if not parser.boxes:
        return 0
    if isinstance(parser.boxes[0], list):
        return sum(len(bxs) for bxs in parser.boxes)
    return len(parser.boxes)


def count_chars(parser):
    if hasattr(parser, "page_chars") and parser.page_chars:
        return sum(len(ch) for ch in parser.page_chars)
    return 0


def run_production_pipeline(pdf_path):
    """Run the production pipeline and return per-stage stats."""
    from deepdoc.parser.pdf_parser import RAGFlowPdfParser

    parser = RAGFlowPdfParser()
    t0 = time.time()

    # Stage 1: __images__
    parser.__images__(pdf_path, zoomin=3)
    chars_total = count_chars(parser)
    boxes_initial = count_boxes(parser)

    # Stage 2-3: OCR stubs (no-op when OCR disabled)
    parser._layouts_rec(3)
    parser._table_transformer_job(3, auto_rotate=False)

    # Stage 4: _text_merge (includes column assignment)
    boxes_before_tm = count_boxes(parser)
    parser._text_merge(zoomin=3)
    boxes_after_tm = count_boxes(parser)

    # Stage 5: _concat_downward (Y-sort)
    parser._concat_downward()
    boxes_after_sort = count_boxes(parser)

    # Stage 6: _naive_vertical_merge
    boxes_before_vm = count_boxes(parser)
    parser._naive_vertical_merge(zoomin=3)
    boxes_after_vm = count_boxes(parser)

    # Stage 7: _extract_table_figure → skip (OCR-dependent, empty without OCR)
    # Stage 8: insert_table_figures → no-op when no tables detected
    # Stage 9: add position_tag/image/positions → doesn't change text

    boxes_final = count_boxes(parser)

    # Compute text length from final boxes (without _line_tag appended)
    flat_boxes = parser.boxes if not (parser.boxes and isinstance(parser.boxes[0], list)) else \
        [b for bxs in parser.boxes for b in bxs]
    text_len = sum(len(b.get("text", "")) for b in flat_boxes)

    elapsed = time.time() - t0

    return {
        "pages": parser.total_page if hasattr(parser, "total_page") else len(parser.page_images),
        "chars": chars_total,
        "boxes_initial": boxes_initial,
        "boxes_before_text_merge": boxes_before_tm,
        "boxes_after_text_merge": boxes_after_tm,
        "boxes_after_sort": boxes_after_sort,
        "boxes_before_vertical_merge": boxes_before_vm,
        "boxes_after_vertical_merge": boxes_after_vm,
        "boxes_final": boxes_final,
        "text_len": text_len,
        "is_english": parser.is_english if hasattr(parser, "is_english") else None,
        "time_s": round(elapsed, 2),
    }


def main_real():
    """Run production pipeline on all real PDFs and save results."""
    pdf_files = sorted(Path(REAL_PDFS_DIR).glob("*.pdf"))
    if not pdf_files:
        print(f"No PDFs found in {REAL_PDFS_DIR}")
        return

    results = []
    for i, pdf_path in enumerate(pdf_files):
        name = pdf_path.name
        print(f"[{i+1}/{len(pdf_files)}] {name} ...", end=" ", flush=True)
        try:
            stats = run_production_pipeline(str(pdf_path))
            stats["file"] = name
            results.append(stats)
            print(f"pages={stats['pages']} chars={stats['chars']} "
                  f"boxes: {stats['boxes_initial']}→{stats['boxes_after_text_merge']}→"
                  f"{stats['boxes_after_vertical_merge']}→{stats['boxes_final']} "
                  f"text={stats['text_len']}")
        except Exception as e:
            print(f"FAIL: {e}")
            traceback.print_exc()
            results.append({"file": name, "error": str(e)})

    with open(OUT_REAL, "w", encoding="utf-8") as f:
        json.dump(results, f, ensure_ascii=False, indent=2)
    print(f"\nSaved {len(results)} results to {OUT_REAL}")


def main_snapshots():
    """Run production pipeline on snapshot PDFs and save per-stage results."""
    pdf_files = sorted(Path(SNAPSHOT_PDFS_DIR).glob("*.pdf"))
    if not pdf_files:
        print(f"No PDFs found in {SNAPSHOT_PDFS_DIR}")
        return

    os.makedirs(OUT_SNAPSHOTS_DIR, exist_ok=True)
    index = []

    for i, pdf_path in enumerate(pdf_files):
        name = pdf_path.stem
        print(f"[{i+1}/{len(pdf_files)}] {name} ...", end=" ", flush=True)
        try:
            stats = run_production_pipeline(str(pdf_path))
            stats["file"] = name
            out_path = os.path.join(OUT_SNAPSHOTS_DIR, f"{name}.json")
            with open(out_path, "w", encoding="utf-8") as f:
                json.dump(stats, f, ensure_ascii=False, indent=2)
            index.append({"file": name, "path": f"snapshots_aligned/{name}.json"})
            print(f"ok (boxes: {stats['boxes_initial']}→{stats['boxes_final']}, text={stats['text_len']})")
        except Exception as e:
            print(f"FAIL: {e}")
            traceback.print_exc()
            index.append({"file": name, "error": str(e)})

    with open(OUT_SNAPSHOT_INDEX, "w", encoding="utf-8") as f:
        json.dump(index, f, ensure_ascii=False, indent=2)
    print(f"\nSaved {len(index)} snapshots to {OUT_SNAPSHOTS_DIR}")


if __name__ == "__main__":
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument("--real", action="store_true", help="Process real_pdfs/")
    ap.add_argument("--snapshots", action="store_true", help="Process pdfs/ (snapshot set)")
    args = ap.parse_args()

    if not args.real and not args.snapshots:
        # Default: both
        args.real = True
        args.snapshots = True

    if args.real:
        print("=== Real PDFs ===\n")
        main_real()
        print()

    if args.snapshots:
        print("=== Snapshot PDFs ===\n")
        main_snapshots()

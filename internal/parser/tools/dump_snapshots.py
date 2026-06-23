"""
Generate golden snapshots from pdf_parser.py for each test PDF.

Each snapshot captures the parser's intermediate state at the model-free
pipeline stages that match Go's Parse():

  __images__          → Go: charsToBoxes
  _text_merge         → Go: AssignColumn + TextMerge
  _concat_downward    → Go: sortByPageThenY
  _naive_vertical_merge → Go: NaiveVerticalMerge

DLA/TSR/OCR stages are skipped so that layoutno/layout_type are empty,
matching Go's behaviour when DeepDoc is nil.

Usage:
    uv run python3 internal/parser/tools/dump_snapshots.py
"""

import json
import os
import sys
import traceback
import numpy as np
from pathlib import Path

# Ensure project root is on path
PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
sys.path.insert(0, PROJECT_ROOT)

TESTDATA_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "testdata")
PDF_DIR = os.path.join(TESTDATA_DIR, "pdfs")
SNAPSHOT_DIR = os.path.join(TESTDATA_DIR, "snapshots")


def _serialize(obj):
    """Recursively convert numpy types, sets, and non-serializable objects to JSON-safe types."""
    if isinstance(obj, (np.integer,)):
        return int(obj)
    if isinstance(obj, (np.floating,)):
        return float(obj)
    if isinstance(obj, np.ndarray):
        return obj.tolist()
    if isinstance(obj, set):
        return list(obj)
    if isinstance(obj, dict):
        return {str(k): _serialize(v) for k, v in obj.items()}
    if isinstance(obj, (list, tuple)):
        return [_serialize(v) for v in obj]
    return obj


class _NoOpOCR:
    """Stub OCR that returns empty results — matching Go when DeepDoc is nil."""
    def __init__(self, *args, **kwargs): pass
    def detect(self, img, device_id=None): return []
    def recognize_batch(self, imgs, device_id=None): return []
    def get_rotate_crop_image(self, img, box): return None
    def __call__(self, img): return []

class _NoOpLayouter:
    """Stub layouter that flattens boxes — matching Go when DeepDoc is nil."""
    def __init__(self, *args, **kwargs): pass
    def __call__(self, page_images, boxes, ZM, drop=True):
        # Flatten: __images__ produces [[page0 boxes], [page1 boxes], ...]
        flat = [b for bxs in boxes for b in bxs]
        return flat, [[] for _ in range(len(boxes))]

class _NoOpTSR:
    """Stub TSR that returns no tables — matching Go when DeepDoc is nil."""
    def __init__(self, *args, **kwargs): pass
    def __call__(self, imgs): return []
    def construct_table(self, bxs, html=False, is_english=False): return ""


class SnapshotDumper:
    """
    Runs the model-free PDF parsing pipeline and captures intermediate state
    at each stage.  Matches Go's Parse() when DeepDoc is nil.
    """

    def __init__(self, pdf_path, output_path):
        self.pdf_path = pdf_path
        self.output_path = output_path
        self.snapshot = {
            "pdf_file": os.path.basename(pdf_path),
            "stages": {},
        }

    def run(self):
        # Patch model imports BEFORE constructing RAGFlowPdfParser so __init__
        # doesn't try to download HuggingFace models.
        import deepdoc.parser.pdf_parser as pp_mod
        pp_mod.OCR = _NoOpOCR
        pp_mod.LayoutRecognizer = _NoOpLayouter
        pp_mod.TableStructureRecognizer = _NoOpTSR

        # Also patch AscendLayoutRecognizer if it exists
        if hasattr(pp_mod, 'AscendLayoutRecognizer'):
            pp_mod.AscendLayoutRecognizer = _NoOpLayouter

        from deepdoc.parser.pdf_parser import RAGFlowPdfParser
        parser = RAGFlowPdfParser()

        # ── Stage 1: __images__ ─────────────────────────────────────────
        # Extracts characters from PDF pages, builds initial boxes, detects
        # English.  This is the only stage that touches the PDF binary.
        parser.__images__(self.pdf_path, zoomin=3)
        self.snapshot["stages"]["__images__"] = {
            "total_pages": parser.total_page,
            "page_count": len(parser.page_images),
            "mean_height": _serialize(parser.mean_height),
            "mean_width": _serialize(parser.mean_width),
            "is_english": parser.is_english,
            "page_cum_height": _serialize(parser.page_cum_height),
            "boxes_per_page": [len(bxs) for bxs in parser.boxes],
            "page_chars_count": [len(ch) for ch in parser.page_chars],
            "lefted_chars_count": len(parser.lefted_chars),
            "sample_boxes_page0": _serialize(parser.boxes[0][:5] if parser.boxes else []),
            # Export full page_chars for Go pipeline comparison tests
            "page_chars": [[_serialize(c) for c in page_chars] for page_chars in parser.page_chars],
            "page_images_size": [{"width": img.size[0], "height": img.size[1]} for img in parser.page_images],
        }
        print(f"  Stage 1 __images__: {len(parser.boxes)} page(s) with boxes, is_english={parser.is_english}")

        # ── SKIP _layouts_rec (DLA) — no model in Go ────────────────────
        # ── SKIP _table_transformer_job (TSR) — no model in Go ──────────
        # _layouts_rec normally flattens self.boxes from [[page0], [page1]] to [box, box, ...].
        # Do it manually since we skipped it.
        parser.boxes = [b for bxs in parser.boxes for b in bxs]

        # ── Stage 2: _text_merge ───────────────────────────────────────
        # Includes _assign_column (KMeans x0 clustering) + horizontal merge.
        boxes_before_merge = len([b for bxs in parser.boxes for b in bxs])
        parser._text_merge(zoomin=3)
        boxes_after_merge = len(parser.boxes)
        self.snapshot["stages"]["_text_merge"] = {
            "boxes_before": boxes_before_merge,
            "boxes_after": boxes_after_merge,
            "sample_boxes": _serialize(parser.boxes[:10]),
        }
        print(f"  Stage 2 _text_merge: {boxes_before_merge} -> {boxes_after_merge} boxes")

        # ── Stage 3: _concat_downward ──────────────────────────────────
        boxes_before = len(parser.boxes)
        parser._concat_downward(concat_between_pages=True)
        boxes_after = len(parser.boxes)
        self.snapshot["stages"]["_concat_downward"] = {
            "boxes_before": boxes_before,
            "boxes_after": boxes_after,
            "sample_boxes": _serialize(parser.boxes[:10]),
        }
        print(f"  Stage 3 _concat_downward: {boxes_before} -> {boxes_after} boxes")

        # ── Stage 4: _naive_vertical_merge ─────────────────────────────
        boxes_before = len(parser.boxes)
        parser._naive_vertical_merge(zoomin=3)
        boxes_after = len(parser.boxes)
        self.snapshot["stages"]["_naive_vertical_merge"] = {
            "boxes_before": boxes_before,
            "boxes_after": boxes_after,
            "sample_boxes": _serialize(parser.boxes[:10]),
        }
        print(f"  Stage 4 _naive_vertical_merge: {boxes_before} -> {boxes_after} boxes")

        # ── Write snapshot ─────────────────────────────────────────────
        os.makedirs(os.path.dirname(self.output_path), exist_ok=True)
        with open(self.output_path, "w", encoding="utf-8") as f:
            json.dump(_serialize(self.snapshot), f, ensure_ascii=False, indent=2)
        print(f"  → Snapshot saved: {self.output_path}")


def main():
    pdf_files = sorted(Path(PDF_DIR).glob("*.pdf"))
    if not pdf_files:
        print(f"No PDFs found in {PDF_DIR}")
        return

    os.makedirs(SNAPSHOT_DIR, exist_ok=True)

    for pdf_path in pdf_files:
        name = pdf_path.stem
        snapshot_path = os.path.join(SNAPSHOT_DIR, f"{name}.json")
        # Always regenerate to overwrite old model-dependent snapshots
        if os.path.exists(snapshot_path):
            os.remove(snapshot_path)

        print(f"\n{'='*60}")
        print(f"Processing: {name}")
        print(f"{'='*60}")
        try:
            dumper = SnapshotDumper(str(pdf_path), snapshot_path)
            dumper.run()
        except Exception as e:
            print(f"FAIL: {name}")
            traceback.print_exc()

    print(f"\nDone. Snapshots in: {SNAPSHOT_DIR}")


if __name__ == "__main__":
    main()

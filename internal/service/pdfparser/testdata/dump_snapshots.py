"""
Generate golden snapshots from pdf_parser.py for each test PDF.

Each snapshot captures the parser's intermediate state at every pipeline stage
so the Go implementation can be verified stage-by-stage.

Usage:
    uv run python3 test/testdata/dump_snapshots.py
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

PDF_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "pdfs")
SNAPSHOT_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "snapshots")


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


class SnapshotDumper:
    """
    Monkey-patches RAGFlowPdfParser to capture intermediate state at each stage,
    then runs the full pipeline and saves snapshots.
    """

    def __init__(self, pdf_path, output_path):
        self.pdf_path = pdf_path
        self.output_path = output_path
        self.snapshot = {
            "pdf_file": os.path.basename(pdf_path),
            "stages": {},
        }

    def run(self):
        from deepdoc.parser.pdf_parser import RAGFlowPdfParser

        parser = RAGFlowPdfParser()

        # ── Stage 1: __images__ ─────────────────────────────────────────
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
            # Sample first few boxes from page 0
            "sample_boxes_page0": _serialize(parser.boxes[0][:5] if parser.boxes else []),
            # Export full page_chars for Go pipeline comparison tests
            "page_chars": [[_serialize(c) for c in page_chars] for page_chars in parser.page_chars],
            # Export page image sizes (for Go crop/geometry references)
            "page_images_size": [{"width": img.size[0], "height": img.size[1]} for img in parser.page_images],
        }
        print(f"  Stage 1 __images__: {len(parser.boxes)} page(s) with boxes, is_english={parser.is_english}")

        # ── Stage 2: _layouts_rec ──────────────────────────────────────
        parser._layouts_rec(3)
        all_boxes = [b for bxs in parser.boxes for b in bxs if isinstance(b, dict)]
        layout_types = {}
        for b in all_boxes:
            lt = b.get("layout_type", "unknown")
            layout_types[lt] = layout_types.get(lt, 0) + 1

        self.snapshot["stages"]["_layouts_rec"] = {
            "total_boxes": len(all_boxes),
            "layout_type_counts": _serialize(layout_types),
            "page_layout_counts": [len(pl) for pl in parser.page_layout],
            "sample_boxes": _serialize(all_boxes[:10]),
        }
        print(f"  Stage 2 _layouts_rec: {len(all_boxes)} boxes, layout types: {layout_types}")

        # ── Stage 3: _table_transformer_job ────────────────────────────
        parser._table_transformer_job(3, auto_rotate=False)
        tb_cpns_count = len(parser.tb_cpns)
        self.snapshot["stages"]["_table_transformer_job"] = {
            "tb_cpns_count": tb_cpns_count,
            "sample_tb_cpns": _serialize(parser.tb_cpns[:5]),
        }
        print(f"  Stage 3 _table_transformer_job: {tb_cpns_count} table components")

        # ── Stage 4: _text_merge ───────────────────────────────────────
        boxes_before_merge = len([b for bxs in parser.boxes for b in bxs])
        parser._text_merge(zoomin=3)
        boxes_after_merge = len(parser.boxes)
        self.snapshot["stages"]["_text_merge"] = {
            "boxes_before": boxes_before_merge,
            "boxes_after": boxes_after_merge,
            "sample_boxes": _serialize(parser.boxes[:10]),
        }
        print(f"  Stage 4 _text_merge: {boxes_before_merge} -> {boxes_after_merge} boxes")

        # ── Stage 5: _concat_downward ──────────────────────────────────
        boxes_before = len(parser.boxes)
        parser._concat_downward(concat_between_pages=True)
        boxes_after = len(parser.boxes)
        self.snapshot["stages"]["_concat_downward"] = {
            "boxes_before": boxes_before,
            "boxes_after": boxes_after,
            "sample_boxes": _serialize(parser.boxes[:10]),
        }
        print(f"  Stage 5 _concat_downward: {boxes_before} -> {boxes_after} boxes")

        # ── Stage 6: _naive_vertical_merge ─────────────────────────────
        boxes_before = len(parser.boxes)
        parser._naive_vertical_merge(zoomin=3)
        boxes_after = len(parser.boxes)
        self.snapshot["stages"]["_naive_vertical_merge"] = {
            "boxes_before": boxes_before,
            "boxes_after": boxes_after,
            "sample_boxes": _serialize(parser.boxes[:10]),
        }
        print(f"  Stage 6 _naive_vertical_merge: {boxes_before} -> {boxes_after} boxes")

        # ── Stage 7: _filter_forpages ──────────────────────────────────
        boxes_before = len(parser.boxes)
        parser._filter_forpages()
        boxes_after = len(parser.boxes)
        self.snapshot["stages"]["_filter_forpages"] = {
            "boxes_before": boxes_before,
            "boxes_after": boxes_after,
        }
        print(f"  Stage 7 _filter_forpages: {boxes_before} -> {boxes_after} boxes")

        # ── Stage 8: _extract_table_figure ─────────────────────────────
        from copy import deepcopy
        tbls = parser._extract_table_figure(
            need_image=True, ZM=3, return_html=True,
            need_position=False, separate_tables_figures=False,
        )
        self.snapshot["stages"]["_extract_table_figure"] = {
            "table_count": len(tbls) if tbls else 0,
            "remaining_boxes": len(parser.boxes),
        }
        print(f"  Stage 8 _extract_table_figure: {len(tbls) if tbls else 0} tables extracted")

        # ── Stage 9: __filterout_scraps (final output) ─────────────────
        boxes_copy = deepcopy(parser.boxes)
        final_text = parser._RAGFlowPdfParser__filterout_scraps(boxes_copy, 3)
        self.snapshot["stages"]["__filterout_scraps"] = {
            "text_length": len(final_text),
            "text_preview": final_text[:2000],
        }
        print(f"  Stage 9 __filterout_scraps: final text length = {len(final_text)}")

        # ── Also run the full __call__ for end-to-end comparison ───────
        parser2 = RAGFlowPdfParser()
        final_text2, final_tbls = parser2(
            self.pdf_path, need_image=True, zoomin=3,
            return_html=True, auto_rotate_tables=False,
        )
        self.snapshot["stages"]["__call__"] = {
            "text_length": len(final_text2),
            "text_preview": final_text2[:2000],
            "table_count": len(final_tbls) if final_tbls else 0,
        }
        print(f"  Full __call__: text={len(final_text2)} chars, tables={len(final_tbls) if final_tbls else 0}")

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
        if os.path.exists(snapshot_path):
            print(f"SKIP {name} (snapshot exists)")
            continue

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

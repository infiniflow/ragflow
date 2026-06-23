"""Generate golden test data for Go pdfparser tests.

Uses the REAL pdf_parser.py algorithmic methods (not hand-written simplifications)
to produce chars.json and per-stage py_boxes_*.json golden data.

Produces:
  - chars.json: real pdfplumber dedupe_chars() output
  - py_boxes_initial.json: chars grouped into boxes (Go charsToBoxes equivalent)
  - py_boxes_sorted.json: after sort_X_by_page
  - py_boxes_text_merge.json: after _text_merge
  - py_boxes_vertical_merge.json: after _naive_vertical_merge
  - py_boxes_reading_order.json: after _final_reading_order_merge
  - py_sections.json: final sections with position tags
  - mean_heights.json, mean_widths.json, page_geoms.json: metadata
"""

import json, os, sys

PROJ = os.path.join(os.path.dirname(os.path.abspath(__file__)), "../../../")
sys.path.insert(0, PROJ)

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'testdata', 'pipeline')
PDF = os.path.join(OUT, "../../../test/benchmark/test_docs/Doc1.pdf")
ZOOM = 3


def main():
    import pdfplumber
    from deepdoc.parser.pdf_parser import RAGFlowPdfParser
    import threading

    LOCK_KEY = "global_shared_lock_pdfplumber"
    if LOCK_KEY not in sys.modules:
        sys.modules[LOCK_KEY] = threading.Lock()

    # ── Step 1: Extract real chars via pdfplumber ─────────────────────────
    with sys.modules[LOCK_KEY]:
        with pdfplumber.open(PDF) as pdf:
            pages = list(pdf.pages)
            page_chars = []
            page_geoms = {}
            for i, p in enumerate(pages):
                try:
                    ch = p.dedupe_chars().chars
                except Exception:
                    ch = []
                page_chars.append(ch)
                page_geoms[i] = {
                    "width": float(p.width or 612),
                    "height": float(p.height or 792),
                    "zoom": float(ZOOM),
                }

    print(f"PDF: {PDF} -> {len(pages)} pages")

    # Serialize chars
    all_chars = []
    for pg, ch_list in enumerate(page_chars):
        for c in ch_list:
            all_chars.append({
                "x0": c.get("x0", 0), "x1": c.get("x1", 0),
                "top": c.get("top", 0), "bottom": c.get("bottom", 0),
                "text": c.get("text", ""), "fontname": c.get("fontname", ""),
                "page_number": pg,
            })

    with open(os.path.join(OUT, "chars.json"), "w") as f:
        json.dump(all_chars, f, indent=2, ensure_ascii=False)
    print(f"  chars: {len(all_chars)} -> chars.json")

    # ── Step 2: chars → boxes (Go charsToBoxes equivalent) ────────────────
    boxes = chars_to_boxes(all_chars)
    dump_boxes(boxes, "py_boxes_initial.json")
    print(f"  boxes (initial): {len(boxes)} -> py_boxes_initial.json")

    mean_h, mean_w = compute_mean_dims(all_chars)
    with open(os.path.join(OUT, "mean_heights.json"), "w") as f:
        json.dump({str(k): v for k, v in mean_h.items()}, f, indent=2)
    with open(os.path.join(OUT, "mean_widths.json"), "w") as f:
        json.dump({str(k): v for k, v in mean_w.items()}, f, indent=2)
    with open(os.path.join(OUT, "page_geoms.json"), "w") as f:
        json.dump(page_geoms, f, indent=2)

    # ── Step 3: Run REAL pdf_parser.py algorithmic stages ─────────────────
    parser = RAGFlowPdfParser()
    parser.boxes = boxes
    parser.mean_height = [mean_h.get(pg, 10) for pg in sorted(mean_h)]
    parser.mean_width = [mean_w.get(pg, 5) for pg in sorted(mean_w)]
    parser.is_english = False

    # sort_X_by_page (static method)
    boxes = RAGFlowPdfParser.sort_X_by_page(boxes, 3)
    dump_boxes(boxes, "py_boxes_sorted.json")
    print(f"  boxes (sorted): {len(boxes)} -> py_boxes_sorted.json")

    # assign_column
    parser.boxes = boxes
    parser._assign_column(parser.boxes, ZOOM)
    boxes = parser.boxes

    # text_merge
    parser._text_merge(ZOOM)
    boxes = parser.boxes
    dump_boxes(boxes, "py_boxes_text_merge.json")
    print(f"  boxes (text_merge): {len(boxes)} -> py_boxes_text_merge.json")

    # naive_vertical_merge
    parser._naive_vertical_merge(ZOOM)
    boxes = parser.boxes
    dump_boxes(boxes, "py_boxes_vertical_merge.json")
    print(f"  boxes (vertical_merge): {len(boxes)} -> py_boxes_vertical_merge.json")

    # final_reading_order_merge
    parser._final_reading_order_merge(ZOOM)
    boxes = parser.boxes
    dump_boxes(boxes, "py_boxes_reading_order.json")
    print(f"  boxes (reading_order): {len(boxes)} -> py_boxes_reading_order.json")

    # ── Step 4: boxes → sections (position tags) ──────────────────────────
    page_cum_height = [0.0]
    for pg in sorted(page_geoms):
        page_cum_height.append(page_cum_height[-1] + page_geoms[pg]["height"])
    parser.page_cum_height = page_cum_height
    parser.page_from = 0

    from unittest.mock import MagicMock
    parser.page_images = []
    for pg, geom in page_geoms.items():
        mock_img = MagicMock()
        mock_img.size = (geom["width"] * ZOOM, geom["height"] * ZOOM)
        parser.page_images.append(mock_img)

    sections = []
    for b in boxes:
        try:
            tag = parser._line_tag(b, ZOOM)
        except Exception:
            tag = "@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##".format(
                b.get("page_number", 0), b.get("x0", 0), b.get("x1", 0),
                b.get("top", 0), b.get("bottom", 0),
            )
        text_with_tag = b.get("text", "") + tag
        sections.append([text_with_tag, tag])

    with open(os.path.join(OUT, "py_sections.json"), "w") as f:
        json.dump(sections, f, indent=2, ensure_ascii=False)
    print(f"  sections: {len(sections)} -> py_sections.json")
    print("\nDone!")


# ── Helper functions ──────────────────────────────────────────────────────

def chars_to_boxes(chars):
    """Group chars into line-level boxes, matching Go charsToBoxes()."""
    if not chars:
        return []
    chars = sorted(chars, key=lambda c: (c.get("top", 0), c.get("x0", 0)))
    lines, cur = [], []
    for c in chars:
        if not cur:
            cur.append(c); continue
        h0 = cur[-1].get("bottom", 0) - cur[-1].get("top", 0)
        h1 = c.get("bottom", 0) - c.get("top", 0)
        if abs(c.get("top", 0) - cur[-1].get("top", 0)) < max(h0, h1) * 0.5:
            cur.append(c)
        else:
            if cur: lines.append(cur)
            cur = [c]
    if cur: lines.append(cur)

    boxes = []
    for line in lines:
        boxes.append({
            "x0": min(c.get("x0", 0) for c in line),
            "x1": max(c.get("x1", 0) for c in line),
            "top": min(c.get("top", 0) for c in line),
            "bottom": max(c.get("bottom", 0) for c in line),
            "text": "".join(c.get("text", "") for c in line),
            "page_number": line[0].get("page_number", 0),
            "layout_type": "text", "layoutno": "1", "col_id": 0, "R": "",
        })
    return boxes


def compute_mean_dims(chars):
    """Per-page mean char height and width."""
    by_page = {}
    for c in chars:
        pg = c.get("page_number", 0)
        by_page.setdefault(pg, []).append(c)
    mean_h, mean_w = {}, {}
    for pg, cl in by_page.items():
        mean_h[pg] = sum(c.get("bottom", 0) - c.get("top", 0) for c in cl) / max(len(cl), 1)
        mean_w[pg] = sum(c.get("x1", 0) - c.get("x0", 0) for c in cl) / max(len(cl), 1)
    return mean_h, mean_w


def dump_boxes(boxes, name):
    with open(os.path.join(OUT, name), "w") as f:
        json.dump(boxes, f, indent=2, ensure_ascii=False)


if __name__ == "__main__":
    main()

"""
Batch TSR: pdfplumber render → DeepDoc TSR, per-PDF output.

Usage:
    uv run python3 internal/parser/tools/batch_compare_tsr.py [count]

    count: process first N PDFs (sorted by name). Default: all.
"""
import io, json, os, sys, time
from pathlib import Path

import pdfplumber, requests
from PIL import Image

TSR_URL = os.environ.get("TSR_URL", "http://localhost:8000/predict/tsr")
DPI = 216
SCRIPT_DIR = Path(os.path.dirname(os.path.abspath(__file__))).parent / "testdata"
PDF_DIR = SCRIPT_DIR / "real_pdfs"
OUT_DIR = SCRIPT_DIR / "output" / "tsr_py"
OUT_DIR.mkdir(exist_ok=True)

count = int(sys.argv[1]) if len(sys.argv) > 1 else None


def process_pdf(pdf_path):
    name = pdf_path.name
    results = []
    t0 = time.time()
    with pdfplumber.open(str(pdf_path)) as pdf:
        for pg, page in enumerate(pdf.pages):
            tables = page.find_tables()
            if not tables:
                continue
            try:
                page_img = page.to_image(resolution=DPI, antialias=True).annotated
            except Exception as e:
                print(f"  page {pg}: render failed — {e}")
                continue
            for ti, t in enumerate(tables):
                bbox = list(t.bbox)
                bbox_px = tuple(int(v * DPI / 72) for v in bbox)
                try:
                    cropped = page_img.crop(bbox_px)
                except Exception:
                    continue
                buf = io.BytesIO()
                cropped.save(buf, format="PNG")
                t1 = time.time()
                try:
                    resp = requests.post(TSR_URL, files={"request": ("t.png", buf.getvalue(), "image/png")}, timeout=120)
                    if resp.status_code == 200:
                        cells = len(resp.json().get("bboxes", []))
                    else:
                        cells = -1
                except Exception:
                    cells = -1
                results.append({
                    "page": pg, "table_idx": ti,
                    "bbox_pts": [round(v, 1) for v in bbox],
                    "cells": cells,
                    "tsr_ms": round((time.time() - t1) * 1000),
                })
    return {"pdf": name, "tables": len(results), "results": results,
            "time_s": round(time.time() - t0, 1)}


def main():
    pdfs = sorted(PDF_DIR.glob("*.pdf"))
    if count:
        pdfs = pdfs[:count]

    total = 0
    for i, p in enumerate(pdfs):
        name = p.name
        # Skip if already processed
        out_path = OUT_DIR / f"{name}.json"
        if out_path.exists():
            with open(out_path) as f:
                prev = json.load(f)
            n = prev.get("tables", 0)
            total += n
            print(f"[{i+1}/{len(pdfs)}] {name} — SKIP (already processed, {n} tables)")
            continue

        r = process_pdf(p)
        with open(out_path, "w") as f:
            json.dump(r, f, ensure_ascii=False, indent=2)
        total += r["tables"]
        print(f"[{i+1}/{len(pdfs)}] {name} — {r['tables']} tables ({r['time_s']}s)")

    print(f"\nDone. {total} tables. Output: {OUT_DIR}/")


if __name__ == "__main__":
    main()

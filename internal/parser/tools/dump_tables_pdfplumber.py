"""
Extract tables from real PDFs using pdfplumber.find_tables() (no OCR).
Produces reference data for comparing with pdf_oxide.extract_tables().

Usage:
    cd /home/shenyushi/cc-workspace/ragflow
    PYTHONPATH=. uv run python3 internal/parser/tools/dump_tables_pdfplumber.py
"""

import json
import os
import sys
import time
import traceback
from pathlib import Path

import pdfplumber

SCRIPT_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "testdata")
REAL_PDFS = os.path.join(SCRIPT_DIR, "real_pdfs")
OUT = os.path.join(SCRIPT_DIR, "real_pdf_tables_pdfplumber.json")


def main():
    pdf_files = sorted(Path(REAL_PDFS).glob("*.pdf"))
    results = []
    total_tables = 0
    files_with_tables = 0

    t0 = time.time()
    for i, pdf_path in enumerate(pdf_files):
        name = pdf_path.name
        tables_info = []
        try:
            with pdfplumber.open(str(pdf_path)) as pdf:
                for pg, page in enumerate(pdf.pages):
                    try:
                        found = page.find_tables()
                    except Exception:
                        continue
                    for t in (found or []):
                        rows = len(t.rows)
                        cols = len(t.rows[0].cells) if t.rows else 0
                        tables_info.append({
                            "page": pg,
                            "bbox": [round(v, 1) for v in list(t.bbox)],  # x0, top, x1, bottom
                            "rows": rows,
                            "cols": cols,
                        })
        except Exception as e:
            print(f"[{i+1}/{len(pdf_files)}] {name} — FAIL: {e}")
            traceback.print_exc()
            tables_info = []  # record as no tables on error

        n = len(tables_info)
        if n > 0:
            files_with_tables += 1
            total_tables += n
            print(f"[{i+1}/{len(pdf_files)}] {name} — {n} table(s)")
        else:
            print(f"[{i+1}/{len(pdf_files)}] {name} — 0 tables")

        results.append({
            "file": name,
            "tables": tables_info,
            "table_count": n,
        })

    elapsed = time.time() - t0
    print(f"\nDone. {files_with_tables}/{len(pdf_files)} PDFs have tables, {total_tables} total tables. ({elapsed:.1f}s)")

    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(results, f, ensure_ascii=False, indent=2)
    print(f"Saved to {OUT}")


if __name__ == "__main__":
    main()

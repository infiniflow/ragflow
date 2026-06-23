#!/usr/bin/env python3
"""Render page 0 of each test PDF at 216 DPI using poppler (pdftoppm).

Outputs to testdata/output/render_compare/py/{pdf}_p0.png for comparison
with Go's pdfium renders.

Usage:
    cd /home/shenyushi/cc-workspace/ragflow/internal/parser
    python3 tools/render_compare.py
"""
import os
import subprocess
import sys
from pathlib import Path

SCRIPT_DIR = Path(__file__).parent
PARSER_DIR = SCRIPT_DIR / ".."
PDF_DIR = PARSER_DIR / "testdata" / "pdfs"
OUT_DIR = PARSER_DIR / "testdata" / "output" / "render_compare" / "py"
DPI = 216

OUT_DIR.mkdir(parents=True, exist_ok=True)

pdfs = sorted(PDF_DIR.glob("*.pdf"))
print(f"Rendering {len(pdfs)} PDFs at {DPI} DPI → {OUT_DIR}/")

for pdf in pdfs:
    out_path = OUT_DIR / f"{pdf.name}_p0.png"
    if out_path.exists():
        print(f"  SKIP {pdf.name} (cached)")
        continue
    try:
        prefix = str(out_path).replace(".png", "")
        subprocess.run(
            ["pdftoppm", "-png", "-r", str(DPI), "-f", "1", "-l", "1", str(pdf), prefix],
            check=True, capture_output=True, timeout=30,
        )
        # pdftoppm outputs {prefix}-1.png or {prefix}-01.png
        for suffix in ["-1.png", "-01.png"]:
            candidate = prefix + suffix
            if os.path.exists(candidate):
                os.rename(candidate, str(out_path))
                break
        if out_path.exists():
            print(f"  OK   {pdf.name}")
        else:
            print(f"  FAIL {pdf.name} (no output)")
    except Exception as e:
        print(f"  FAIL {pdf.name}: {e}")

print("Done.")

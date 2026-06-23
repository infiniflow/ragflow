"""
Extract table cell text from a specific PDF page using pdfplumber.
Used to compare cell content against pdf_oxide's extract_tables output.

Usage:
    uv run python3 internal/parser/tools/dump_table_cells.py [pdf_name] [page_num]

Default: RAGFlow 产品白皮书(1).pdf, page 10
"""

import sys, json
import pdfplumber

PDF = sys.argv[1] if len(sys.argv) > 1 else "RAGFlow 产品白皮书(1).pdf"
PAGE = int(sys.argv[2]) if len(sys.argv) > 2 else 10

import os
SCRIPT_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "testdata")
PDF_PATH = os.path.join(SCRIPT_DIR, "real_pdfs", PDF)

with pdfplumber.open(PDF_PATH) as pdf:
    if PAGE >= len(pdf.pages):
        print(f"Page {PAGE} out of range (max {len(pdf.pages)-1})")
        sys.exit(1)

    page = pdf.pages[PAGE]
    tables = page.find_tables()

    print(f"File: {PDF}")
    print(f"Page: {PAGE}")
    print(f"Tables found: {len(tables)}")
    print()

    for ti, t in enumerate(tables):
        bbox = [round(v, 1) for v in list(t.bbox)]
        rows = len(t.rows)
        cols = len(t.rows[0].cells) if t.rows else 0
        print(f"=== Table {ti}: {rows}r x {cols}c  bbox={bbox} ===")

        for ri, row in enumerate(t.rows):
            cells = []
            for cell_bbox in row.cells:
                text = page.within_bbox(cell_bbox).extract_text() or ""
                cells.append(text.strip().replace("\n", " "))
            print(f"  row{ri}: {cells}")
        print()

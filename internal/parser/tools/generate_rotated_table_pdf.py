"""Generate a test PDF with normal and rotated tables for rotation detection testing.

Usage:
    PYTHONPATH=. uv run python3 internal/parser/tools/generate_rotated_table_pdf.py
"""
import os, sys
from pathlib import Path

SCRIPT_DIR = Path(os.path.dirname(os.path.abspath(__file__)))
OUT_DIR = SCRIPT_DIR.parent / "testdata" / "pdfs"
OUT_DIR.mkdir(exist_ok=True)

from reportlab.lib.pagesizes import A4
from reportlab.lib.units import mm
from reportlab.lib import colors
from reportlab.platypus import (
    SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle,
)
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.lib.enums import TA_CENTER
from reportlab.pdfgen import canvas as pdf_canvas
from reportlab.pdfbase import pdfmetrics

PAGE_W, PAGE_H = A4  # 595.27 x 841.89 points

def make_table_data(rows=3, cols=3):
    """Generate simple table data with recognizable text."""
    data = [["Header A", "Header B", "Header C"]]
    for r in range(1, rows + 1):
        row = [f"Cell {r}{chr(65+c)}" for c in range(cols)]  # Cell 1A, Cell 1B, ...
        data.append(row)
    return data

def make_normal_page():
    """Page 1: normal horizontal table at top."""
    buf = []
    styles = getSampleStyleSheet()
    buf.append(Paragraph("Page 1: Normal Table (0°)", styles["Title"]))
    buf.append(Spacer(1, 10*mm))

    data = make_table_data(4, 3)
    table = Table(data, colWidths=[100, 100, 100])
    table.setStyle(TableStyle([
        ('BACKGROUND', (0, 0), (-1, 0), colors.grey),
        ('TEXTCOLOR', (0, 0), (-1, 0), colors.whitesmoke),
        ('ALIGN', (0, 0), (-1, -1), 'CENTER'),
        ('FONTNAME', (0, 0), (-1, 0), 'Helvetica-Bold'),
        ('FONTSIZE', (0, 0), (-1, 0), 12),
        ('BOTTOMPADDING', (0, 0), (-1, 0), 8),
        ('GRID', (0, 0), (-1, -1), 1, colors.black),
        ('FONTNAME', (0, 1), (-1, -1), 'Helvetica'),
        ('FONTSIZE', (0, 1), (-1, -1), 10),
        ('TOPPADDING', (0, 1), (-1, -1), 6),
        ('BOTTOMPADDING', (0, 1), (-1, -1), 6),
    ]))
    buf.append(table)
    return buf

def make_rotated_page():
    """Page 2: 90° rotated table (clockwise)."""
    buf = []
    styles = getSampleStyleSheet()
    buf.append(Paragraph("Page 2: Rotated Table (90° CW)", styles["Title"]))
    buf.append(Spacer(1, 10*mm))

    data = make_table_data(4, 3)
    table = Table(data, colWidths=[100, 100, 100])
    table.setStyle(TableStyle([
        ('BACKGROUND', (0, 0), (-1, 0), colors.HexColor('#4472C4')),
        ('TEXTCOLOR', (0, 0), (-1, 0), colors.whitesmoke),
        ('ALIGN', (0, 0), (-1, -1), 'CENTER'),
        ('FONTNAME', (0, 0), (-1, 0), 'Helvetica-Bold'),
        ('FONTSIZE', (0, 0), (-1, 0), 12),
        ('BOTTOMPADDING', (0, 0), (-1, 0), 8),
        ('GRID', (0, 0), (-1, -1), 1, colors.black),
        ('FONTNAME', (0, 1), (-1, -1), 'Helvetica'),
        ('FONTSIZE', (0, 1), (-1, -1), 10),
        ('TOPPADDING', (0, 1), (-1, -1), 6),
        ('BOTTOMPADDING', (0, 1), (-1, -1), 6),
    ]))

    # Rotate the table 90° clockwise.
    # Reportlab uses a transformation: we wrap the table and rotate the
    # coordinate system so the table appears rotated.

    # Place the table rotated.  The canvas is transformed so that x'=y, y'=w-x.
    # We place it at (table_x, table_y) in pre-rotation coords.
    buf.append(Spacer(1, 20*mm))
    buf.append(table)  # normal table for reference
    buf.append(Spacer(1, 15*mm))

    # Rotated table: draw directly on canvas using post-page render hook.
    # SimpleDocTemplate doesn't support rotated flowables well, so we use
    # a canvas-based approach: add the rotated table as an annotation.
    from reportlab.platypus.flowables import Flowable

    class RotatedTable(Flowable):
        def __init__(self, tbl, angle=90):
            super().__init__()
            self._tbl = tbl
            self._angle = angle
            tw, th = tbl.wrap(0, 0)
            self._tw, self._th = tw, th
            # After 90° CW: width=original_height, height=original_width
            self.width = th
            self.height = tw

        def draw(self):
            c = self.canv
            c.saveState()
            c.translate(0, self._tw)
            c.rotate(-90)
            self._tbl.wrap(self._th, self._tw)
            self._tbl.drawOn(c, 0, 0)
            c.restoreState()

    buf.append(Paragraph("Rotated table (90deg CW):", styles["Normal"]))
    buf.append(Spacer(1, 5*mm))
    buf.append(RotatedTable(table, angle=90))
    return buf

def build():
    out_path = OUT_DIR / "table_rotation_test.pdf"
    doc = SimpleDocTemplate(
        str(out_path),
        pagesize=A4,
        leftMargin=50, rightMargin=50,
        topMargin=30, bottomMargin=30,
    )
    doc.build(make_normal_page() + make_rotated_page())
    print(f"Generated: {out_path}")
    return str(out_path)

if __name__ == "__main__":
    sys.exit(0 if build() else 1)

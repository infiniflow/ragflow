"""
Generate test PDFs for pdf_parser validation.

Each PDF exercises specific features of RAGFlowPdfParser:
- Text extraction and character positioning
- Garbled text detection (subset fonts)
- Column detection (KMeans clustering)
- Projection/heading matching (proj_match)
- Table and figure layout detection
- Multi-page text merging
- Vertical concatenation (_concat_downward / _naive_vertical_merge)
- Position tagging and cropping
"""

import os
from reportlab.lib.pagesizes import A4
from reportlab.lib.units import mm, cm
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.lib.enums import TA_LEFT, TA_CENTER
from reportlab.platypus import (
    SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle,
    PageBreak, Frame, PageTemplate, BaseDocTemplate
)
from reportlab.lib import colors
from reportlab.pdfgen import canvas
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.cidfonts import UnicodeCIDFont

OUTPUT_DIR = os.path.dirname(os.path.abspath(__file__)) + "/../testdata/pdfs"

# Register CJK font — use reportlab built-in CID font (no external file needed)
_CJK_FONT = "Helvetica"
try:
    pdfmetrics.registerFont(UnicodeCIDFont("STSong-Light"))
    _CJK_FONT = "STSong-Light"
except Exception:
    pass


def _make_doc(filename):
    """Create a SimpleDocTemplate with standard margins."""
    return SimpleDocTemplate(
        os.path.join(OUTPUT_DIR, filename),
        pagesize=A4,
        leftMargin=2 * cm,
        rightMargin=2 * cm,
        topMargin=2 * cm,
        bottomMargin=2 * cm,
    )


# ── PDF 1: Simple English single page ──────────────────────────────────────
def generate_english_simple():
    """Single-page English document — tests basic text extraction and box merging."""
    doc = _make_doc("01_english_simple.pdf")
    styles = getSampleStyleSheet()
    story = []

    story.append(Paragraph("Introduction to RAG Systems", styles["Title"]))
    story.append(Spacer(1, 0.5 * cm))
    story.append(Paragraph(
        "Retrieval-Augmented Generation (RAG) is a technique that combines "
        "information retrieval with large language models. When a user asks a "
        "question, the system first retrieves relevant documents from a knowledge "
        "base, then feeds them into an LLM to generate an accurate, grounded response. "
        "This approach significantly reduces hallucination and keeps answers up-to-date "
        "without requiring model fine-tuning.",
        styles["Normal"],
    ))
    story.append(Spacer(1, 0.3 * cm))
    story.append(Paragraph(
        "RAG systems typically consist of three main components: a document ingestion "
        "pipeline that parses and chunks documents, a vector database that stores "
        "embeddings, and a retrieval-and-generation loop that finds relevant chunks "
        "and synthesizes answers.",
        styles["Normal"],
    ))
    story.append(Spacer(1, 1 * cm))
    story.append(Paragraph("Key Benefits", styles["Heading2"]))
    story.append(Paragraph(
        "• Reduced hallucination through grounded generation<br/>"
        "• Always up-to-date knowledge without retraining<br/>"
        "• Transparent attribution to source documents<br/>"
        "• Cost-effective compared to fine-tuning large models",
        styles["Normal"],
    ))
    story.append(Spacer(1, 1 * cm))
    story.append(Paragraph("Conclusion", styles["Heading2"]))
    story.append(Paragraph(
        "RAG represents a practical middle ground between pure retrieval systems "
        "and pure generative models. By grounding generation in real documents, "
        "it achieves both accuracy and flexibility. Future work will focus on "
        "improving retrieval quality and reducing latency.",
        styles["Normal"],
    ))

    doc.build(story)


# ── PDF 2: Single-page with Chinese text ───────────────────────────────────
def generate_chinese_simple():
    """Single-page Chinese document — tests CJK text extraction and language detection."""
    if _CJK_FONT == "Helvetica":
        print("WARN: skipping 02_chinese_simple.pdf — no CJK font available")
        return
    doc = _make_doc("02_chinese_simple.pdf")
    styles = getSampleStyleSheet()
    styles["Title"].fontName = _CJK_FONT
    styles["Normal"].fontName = _CJK_FONT
    story = []

    story.append(Paragraph("RAG系统概述", styles["Title"]))
    story.append(Spacer(1, 0.5 * cm))
    story.append(Paragraph(
        "检索增强生成（Retrieval-Augmented Generation，简称RAG）是一种将信息检索"
        "与大型语言模型相结合的技术。当用户提出问题时，系统首先从知识库中检索相关"
        "文档，然后将这些文档输入到大语言模型中，生成准确且有依据的回答。这种方法"
        "能够显著减少模型的幻觉现象，并在不需要重新训练模型的情况下保持答案的时效性。",
        styles["Normal"],
    ))
    story.append(Spacer(1, 0.3 * cm))
    story.append(Paragraph(
        "RAG系统通常包含三个主要组件：文档摄入管道，负责解析和切分文档；向量数据库，"
        "用于存储嵌入向量；以及检索与生成循环，负责找到相关的文档块并合成答案。"
        "此外，系统还需要支持多种文档格式，包括PDF、Word、Excel等常见办公文件格式。",
        styles["Normal"],
    ))

    doc.build(story)


# ── PDF 3: Multi-page document ─────────────────────────────────────────────
def generate_multipage():
    """Multi-page document — tests page_cum_height, cross-page text merging,
    and pagination handling."""
    doc = _make_doc("03_multipage.pdf")
    styles = getSampleStyleSheet()
    story = []

    for chapter in range(1, 4):
        story.append(Paragraph(f"Chapter {chapter}: Topic Overview", styles["Title"]))
        story.append(Spacer(1, 0.3 * cm))
        for para_idx in range(1, 5):
            story.append(Paragraph(
                f"This is paragraph {para_idx} of chapter {chapter}. "
                f"The quick brown fox jumps over the lazy dog. "
                f"Machine learning systems require careful evaluation and testing "
                f"to ensure they perform reliably in production environments. "
                f"Data quality is just as important as model architecture when "
                f"building practical AI applications that serve real users.",
                styles["Normal"],
            ))
            story.append(Spacer(1, 0.2 * cm))
        story.append(PageBreak())

    doc.build(story)


# ── PDF 4: Multi-column layout ─────────────────────────────────────────────
def generate_multicolumn():
    """Multi-column document — tests _assign_column (KMeans column detection)."""
    filepath = os.path.join(OUTPUT_DIR, "04_multicolumn.pdf")
    c = canvas.Canvas(filepath, pagesize=A4)
    width, height = A4
    margin = 2 * cm
    col_width = (width - 2 * margin - 1 * cm) / 2

    # Title spanning both columns
    c.setFont("Helvetica-Bold", 16)
    c.drawString(margin, height - 2 * cm, "Two-Column Layout Test Document")
    c.setFont("Helvetica", 10)

    left_x = margin
    right_x = margin + col_width + 1 * cm
    y_start = height - 3.5 * cm

    def draw_column(x, y, text_lines):
        cy = y
        for line in text_lines:
            c.drawString(x, cy, line[:60])  # truncate to fit
            cy -= 14
            if cy < margin:
                break
        return cy

    left_lines = [
        "Left column paragraph one. This text should be",
        "detected as belonging to the left column by the",
        "KMeans clustering algorithm that analyzes x0",
        "coordinates of bounding boxes.",
        "",
        "Left column paragraph two. The column detection",
        "works by clustering the x-coordinates of text",
        "boxes and selecting the optimal number of",
        "clusters using silhouette scoring.",
    ]
    right_lines = [
        "Right column paragraph one. This text occupies",
        "the right-hand column and should be assigned",
        "a different column ID than the left column.",
        "",
        "Right column paragraph two. Multi-column PDFs",
        "are common in academic papers, newspapers, and",
        "technical documentation. Correct column detection",
        "is essential for maintaining reading order.",
        "",
        "Right column paragraph three. After column",
        "detection, text within each column is merged",
        "vertically before interleaving columns.",
    ]

    draw_column(left_x, y_start, left_lines)
    draw_column(right_x, y_start, right_lines)

    c.save()


# ── PDF 5: Bullet points and numbered lists ────────────────────────────────
def generate_bullets_and_headers():
    """Document with headings, numbered lists, bullet points —
    tests proj_match() and _match_proj() for recognizing structural patterns."""
    doc = _make_doc("05_bullets_and_headers.pdf")
    styles = getSampleStyleSheet()
    story = []

    story.append(Paragraph("第一章 项目背景", styles["Title"]))
    story.append(Spacer(1, 0.5 * cm))
    story.append(Paragraph("第一条 项目目标", styles["Heading2"]))
    story.append(Paragraph(
        "本项目旨在构建一个高效的文档解析系统，支持以下功能：",
        styles["Normal"],
    ))
    story.append(Paragraph(
        "1. PDF文档的文字提取与版面分析<br/>"
        "2. 表格结构识别与内容提取<br/>"
        "3. 多栏排版的阅读顺序恢复<br/>"
        "4. 图片与图表的自动识别",
        styles["Normal"],
    ))
    story.append(Spacer(1, 0.5 * cm))
    story.append(Paragraph("第二条 技术选型", styles["Heading2"]))
    story.append(Paragraph(
        "经过调研，我们选择以下技术方案：",
        styles["Normal"],
    ))
    story.append(Paragraph(
        "（一）OCR引擎选用PaddleOCR<br/>"
        "（二）版面分析使用ONNX模型<br/>"
        "（三）表格识别使用自研模型<br/>"
        "（四）文本拼接使用XGBoost分类器",
        styles["Normal"],
    ))
    story.append(Spacer(1, 0.5 * cm))
    story.append(Paragraph("第二章 实施方案", styles["Title"]))
    story.append(Paragraph(
        "1.1 第一阶段：核心功能开发<br/>"
        "1.2 第二阶段：性能优化<br/>"
        "1.3 第三阶段：生产部署<br/>"
        "2.1 质量保证：单元测试与集成测试<br/>"
        "2.2 监控告警：日志与指标采集",
        styles["Normal"],
    ))
    story.append(Spacer(1, 0.5 * cm))
    story.append(Paragraph("结论与展望", styles["Heading2"]))
    story.append(Paragraph(
        "⚫ 文档解析准确率达到95%以上<br/>"
        "➢ 单页处理时间控制在1秒以内<br/>"
        "✓ 支持中英文混合文档",
        styles["Normal"],
    ))

    doc.build(story)


# ── PDF 6: Table-like content ──────────────────────────────────────────────
def generate_table_content():
    """Document with tables — tests table detection and extraction."""
    doc = _make_doc("06_table_content.pdf")
    styles = getSampleStyleSheet()
    story = []

    story.append(Paragraph("Quarterly Sales Report", styles["Title"]))
    story.append(Spacer(1, 0.5 * cm))
    story.append(Paragraph(
        "The following table summarizes the quarterly sales performance "
        "across different product categories.",
        styles["Normal"],
    ))
    story.append(Spacer(1, 0.3 * cm))

    table_data = [
        ["Category", "Q1", "Q2", "Q3", "Q4", "Total"],
        ["Electronics", "$12,000", "$15,000", "$18,000", "$22,000", "$67,000"],
        ["Software", "$8,000", "$9,500", "$11,000", "$14,000", "$42,500"],
        ["Services", "$5,000", "$6,000", "$7,500", "$9,000", "$27,500"],
        ["Hardware", "$10,000", "$11,000", "$13,000", "$16,000", "$50,000"],
        ["Total", "$35,000", "$41,500", "$49,500", "$61,000", "$187,000"],
    ]

    tbl = Table(table_data, colWidths=[3.5 * cm] + [2.5 * cm] * 5)
    tbl.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), colors.grey),
        ("TEXTCOLOR", (0, 0), (-1, 0), colors.whitesmoke),
        ("BACKGROUND", (0, -1), (-1, -1), colors.lightgrey),
        ("GRID", (0, 0), (-1, -1), 0.5, colors.black),
        ("FONTNAME", (0, 0), (-1, 0), "Helvetica-Bold"),
        ("FONTNAME", (0, -1), (-1, -1), "Helvetica-Bold"),
        ("ALIGN", (1, 0), (-1, -1), "RIGHT"),
        ("VALIGN", (0, 0), (-1, -1), "MIDDLE"),
        ("FONTSIZE", (0, 0), (-1, -1), 9),
    ]))
    story.append(tbl)
    story.append(Spacer(1, 0.5 * cm))
    story.append(Paragraph(
        "Table 1: Quarterly sales by product category (in USD)",
        styles["Normal"],
    ))
    story.append(Spacer(1, 1 * cm))
    story.append(Paragraph(
        "Note: All figures are preliminary and subject to audit. "
        "The Q4 numbers reflect the holiday season boost across all categories.",
        styles["Normal"],
    ))

    doc.build(story)


# ── PDF 17: Garbage layout (header/footer/reference) ────────────────────────
def generate_garbage_layout():
    """Document with header, footer, and reference — tests garbage layout pop."""
    doc = _make_doc("17_garbage_layout.pdf")
    styles = getSampleStyleSheet()
    story = []

    story.append(Paragraph("Chapter 1: Introduction", styles["Title"]))
    story.append(Spacer(1, 0.5*cm))
    story.append(Paragraph("Section 1.1", styles["Heading2"]))
    story.append(Paragraph(
        "This is the main content of the document. The header above and "
        "footer below should be detected by DLA and popped from the output "
        "if they are at the correct page-edge positions.",
        styles["Normal"],
    ))
    story.append(Spacer(1, 0.5*cm))
    story.append(Paragraph(
        "Additional paragraph to provide enough content for layout analysis. "
        "The parser should correctly identify the document structure and "
        "remove decorative elements like page numbers and running headers.",
        styles["Normal"],
    ))

    doc.build(story)


# ── PDF 18: Table with caption + figure with caption ────────────────────────
def generate_table_with_caption():
    """Table + caption and figure + caption — tests TSR backfill and caption merge."""
    doc = _make_doc("18_table_caption.pdf")
    styles = getSampleStyleSheet()
    story = []

    story.append(Paragraph("Product Comparison Report", styles["Title"]))
    story.append(Spacer(1, 0.5*cm))
    story.append(Paragraph("Product Specifications", styles["Heading2"]))
    story.append(Spacer(1, 0.3*cm))

    # Table with distinct cell content
    table_data = [
        ["Model", "Weight", "Price", "Rating"],
        ["Alpha-X1", "1.2 kg", "$899", "4.5/5"],
        ["Beta-Y2", "1.5 kg", "$1,199", "4.2/5"],
        ["Gamma-Z3", "0.9 kg", "$749", "4.8/5"],
    ]
    tbl = Table(table_data, colWidths=[3.5*cm]*4)
    tbl.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), colors.HexColor("#4472C4")),
        ("TEXTCOLOR", (0, 0), (-1, 0), colors.white),
        ("GRID", (0, 0), (-1, -1), 0.5, colors.black),
        ("FONTNAME", (0, 0), (-1, 0), "Helvetica-Bold"),
        ("ALIGN", (1, 0), (-1, -1), "CENTER"),
        ("VALIGN", (0, 0), (-1, -1), "MIDDLE"),
        ("FONTSIZE", (0, 0), (-1, -1), 9),
    ]))
    story.append(tbl)
    story.append(Spacer(1, 0.2*cm))
    story.append(Paragraph(
        "Table 1: Product specification comparison (2024 Q2)",
        styles["Normal"],
    ))
    story.append(Spacer(1, 1*cm))
    story.append(Paragraph("Market Share Overview", styles["Heading2"]))
    story.append(Paragraph(
        "Figure 1 below illustrates the market share distribution among "
        "the three product lines in the current quarter.",
        styles["Normal"],
    ))

    doc.build(story)


# ── PDF 19: Multi-page for chunk testing ────────────────────────────────────
def generate_multipage_chunk():
    """52-page document — tests multi-chunk processing (chunkSize=50)."""
    doc = _make_doc("19_multipage_chunk.pdf")
    styles = getSampleStyleSheet()
    story = []
    for i in range(52):
        story.append(Paragraph(f"Page {i+1}: Test Content for Chunk Processing", styles["Title"]))
        story.append(Spacer(1, 0.3*cm))
        story.append(Paragraph(
            f"This is page {i+1} of the multi-page chunk test document. "
            f"It contains enough pages (52) to trigger the chunked processing "
            f"path which splits the document into batches of 50 pages.",
            styles["Normal"],
        ))
        story.append(PageBreak())
    doc.build(story)


# ── PDF 7: Mixed content with images ───────────────────────────────────────
def generate_mixed_content():
    """Document with text and embedded images — tests figure detection."""
    filepath = os.path.join(OUTPUT_DIR, "07_mixed_content.pdf")
    c = canvas.Canvas(filepath, pagesize=A4)
    width, height = A4

    c.setFont("Helvetica-Bold", 16)
    c.drawString(2 * cm, height - 2 * cm, "Document with Embedded Graphics")

    c.setFont("Helvetica", 10)
    c.drawString(2 * cm, height - 3 * cm,
                 "This document contains both text content and graphic elements.")

    # Draw a simple rectangle as a "figure"
    c.setFillColor(colors.lightblue)
    c.rect(4 * cm, height - 10 * cm, 6 * cm, 4 * cm, fill=True, stroke=True)
    c.setFillColor(colors.black)
    c.setFont("Helvetica", 8)
    c.drawString(4.5 * cm, height - 7.5 * cm, "Figure 1: System Architecture Diagram")

    c.setFont("Helvetica", 10)
    c.drawString(2 * cm, height - 11 * cm,
                 "The figure above illustrates the high-level system architecture. "
                 "Data flows from left to right through the ingestion, processing, "
                 "and serving layers.")

    # Second figure
    c.setFillColor(colors.lightgreen)
    c.rect(12 * cm, height - 10 * cm, 5 * cm, 4 * cm, fill=True, stroke=True)
    c.setFillColor(colors.black)
    c.setFont("Helvetica", 8)
    c.drawString(12.5 * cm, height - 7.5 * cm, "Figure 2: Data Pipeline Flow")

    c.save()


# ── PDF 8: Edge cases — special characters, font variations ────────────────
def generate_edge_cases():
    """Document with special characters, font variations, and edge cases —
    tests garbled text detection and font handling."""
    doc = _make_doc("08_edge_cases.pdf")
    styles = getSampleStyleSheet()
    story = []

    story.append(Paragraph("Edge Case Test Document", styles["Title"]))
    story.append(Spacer(1, 0.5 * cm))
    story.append(Paragraph("Section 1: Special Characters", styles["Heading2"]))
    story.append(Paragraph(
        "This section contains various special characters: "
        "© 2024, ® Registered, ™ Trademark, "
        "α β γ δ ε — Greek letters, "
        "≥ ≤ ≠ ≈ — Mathematical symbols, "
        "→ ← ↑ ↓ — Arrows, "
        "★ ☆ ♥ ♦ — Unicode symbols.",
        styles["Normal"],
    ))
    story.append(Spacer(1, 0.3 * cm))
    story.append(Paragraph("Section 2: Mixed CJK and Latin", styles["Heading2"]))
    story.append(Paragraph(
        "This section mixes CJK characters (日本語のテキスト) with Latin text. "
        "한국어 텍스트도 포함되어 있습니다. 混合中文、English、日本語、한국어。"
        "The parser should correctly handle all these scripts without "
        "flagging legitimate CJK as garbled text. ただし、文字化け検出は重要です。",
        styles["Normal"],
    ))
    story.append(Spacer(1, 0.3 * cm))
    story.append(Paragraph("Section 3: Numeric Content", styles["Heading2"]))
    story.append(Paragraph(
        "Financial data: $1,234,567.89 | €999.99 | ¥10,000<br/>"
        "Dates: 2024-01-15 | 01/15/2024 | 2024年1月15日<br/>"
        "Phone: +86-138-0000-0000 | (555) 123-4567<br/>"
        "URLs: https://example.com/path?q=search<br/>"
        "Email: user@example.com",
        styles["Normal"],
    ))
    story.append(Spacer(1, 0.3 * cm))
    story.append(Paragraph("Section 4: Long Continuous Text", styles["Heading2"]))
    story.append(Paragraph(
        "This is a very long paragraph that should test the text merging "
        "capabilities of the parser. It contains multiple sentences that "
        "should be concatenated correctly. The parser should recognize that "
        "these sentences belong together in the same paragraph. It should not "
        "split them just because there is a period. The vertical merging "
        "algorithm should look at punctuation, spacing, and layout features "
        "to make the correct decision about which text boxes to join together.",
        styles["Normal"],
    ))

    doc.build(story)


# ── Main ───────────────────────────────────────────────────────────────────
def main():
    os.makedirs(OUTPUT_DIR, exist_ok=True)

    generators = [
        ("01_english_simple.pdf", generate_english_simple),
        ("02_chinese_simple.pdf", generate_chinese_simple),
        ("03_multipage.pdf", generate_multipage),
        ("04_multicolumn.pdf", generate_multicolumn),
        ("05_bullets_and_headers.pdf", generate_bullets_and_headers),
        ("06_table_content.pdf", generate_table_content),
        ("07_mixed_content.pdf", generate_mixed_content),
        ("08_edge_cases.pdf", generate_edge_cases),
        ("17_garbage_layout.pdf", generate_garbage_layout),
        ("18_table_caption.pdf", generate_table_with_caption),
        ("19_multipage_chunk.pdf", generate_multipage_chunk),
    ]

    for filename, gen_fn in generators:
        filepath = os.path.join(OUTPUT_DIR, filename)
        if os.path.exists(filepath):
            print(f"SKIP (already exists): {filename}")
            continue
        try:
            gen_fn()
            print(f"OK  : {filename}")
        except Exception as e:
            print(f"FAIL: {filename} — {e}")

    print(f"\nAll test PDFs generated in: {OUTPUT_DIR}")


if __name__ == "__main__":
    main()

"""
Additional test PDFs covering edge cases missed by batch 1.

New scenarios:
- Cross-page paragraph continuation (naive_vertical_merge L979 cross-page logic)
- Multi-level numbering patterns (proj_match patterns 5-12)
- 3-column layout (KMeans k=3)
- Mixed single/multi-column on same page
- Cross-page table (table merge logic L1242-1261)
- Text + table interleaved on same page
- Sparse / empty pages
- Dense CJK long paragraphs (vertical merge stress)
"""

import os
from reportlab.lib.pagesizes import A4
from reportlab.lib.units import cm
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.platypus import (
    SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle, PageBreak
)
from reportlab.lib import colors
from reportlab.pdfgen import canvas

OUTPUT_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "testdata", "pdfs")


def _make_doc(filename):
    return SimpleDocTemplate(
        os.path.join(OUTPUT_DIR, filename),
        pagesize=A4, leftMargin=2 * cm, rightMargin=2 * cm,
        topMargin=2 * cm, bottomMargin=2 * cm,
    )


# ── PDF 09: Cross-page paragraph continuation ─────────────────────────────
def generate_crosspage_paragraph():
    """Tests _naive_vertical_merge cross-page case, page_cum_height, multi-page _line_tag."""
    doc = _make_doc("09_crosspage_paragraph.pdf")
    styles = getSampleStyleSheet()
    story = []
    story.append(Paragraph("Cross-Page Paragraph Continuation Test", styles["Title"]))
    story.append(Spacer(1, 0.5 * cm))

    sentences = [
        "This document tests cross-page paragraph continuation in the PDF parser. ",
        "When a paragraph spans multiple pages the parser must correctly identify ",
        "that boxes on different pages belong to the same logical paragraph. ",
        "This requires accurate page_cum_height computation and proper handling of ",
        "vertical distances that cross page boundaries. ",
    ]
    story.append(Paragraph(" ".join(sentences * 60), styles["Normal"]))

    story.append(Spacer(1, 8 * cm))
    story.append(Paragraph("Near-Bottom Paragraph Start", styles["Heading2"]))
    story.append(Paragraph(
        "This paragraph starts near the bottom of a page and continues across "
        "the page boundary onto the next page. " * 50,
        styles["Normal"],
    ))
    doc.build(story)


# ── PDF 10: Numbering patterns ─────────────────────────────────────────────
def generate_numbering_patterns():
    """Tests proj_match patterns 5-12: 1.1, 1.1.1, 1), （1）, short line with colon, etc."""
    doc = _make_doc("10_numbering_patterns.pdf")
    styles = getSampleStyleSheet()
    story = []
    story.append(Paragraph("项目实施方案细则", styles["Title"]))
    story.append(Spacer(1, 0.5 * cm))
    story.append(Paragraph("一、项目背景", styles["Heading2"]))
    story.append(Paragraph("本项目来源于公司数字化转型战略。", styles["Normal"]))
    story.append(Spacer(1, 0.3 * cm))
    story.append(Paragraph("二、技术方案", styles["Heading2"]))
    story.append(Paragraph(
        "1. 第一阶段：基础设施建设<br/>"
        "1.1 数据库选型与部署<br/>"
        "1.1.1 关系型数据库方案<br/>"
        "1.1.2 向量数据库方案<br/>"
        "1.2 中间件与服务治理<br/>"
        "1.2.1 消息队列选型<br/>"
        "1.2.2 服务注册与发现<br/>"
        "2. 第二阶段：核心功能开发<br/>"
        "2.1 文档解析引擎<br/>"
        "2.1.1 PDF解析模块<br/>"
        "2.1.2 Word解析模块<br/>"
        "2.2 检索增强生成",
        styles["Normal"],
    ))
    story.append(Spacer(1, 0.3 * cm))
    story.append(Paragraph("三、实施步骤", styles["Heading2"]))
    story.append(Paragraph(
        "（1）需求分析与方案设计<br/>"
        "（2）技术选型与环境搭建<br/>"
        "（3）核心模块开发<br/>"
        "（4）集成测试与上线",
        styles["Normal"],
    ))
    story.append(Spacer(1, 0.3 * cm))
    story.append(Paragraph("四、关键决策点", styles["Heading2"]))
    story.append(Paragraph(
        "1）是否采用云端部署方案<br/>"
        "2）是否引入第三方OCR服务<br/>"
        "3）数据安全合规方案选择<br/>"
        "4）运维监控体系设计",
        styles["Normal"],
    ))
    story.append(Spacer(1, 0.3 * cm))
    story.append(Paragraph("五、短标题行测试", styles["Heading2"]))
    for topic in ["性能优化方向：", "安全加固策略：", "成本控制方案："]:
        story.append(Paragraph(topic, styles["Normal"]))
        story.append(Paragraph(f"针对{topic.strip('：')}的详细实施计划。", styles["Normal"]))
    doc.build(story)


# ── PDF 11: Three-column layout ───────────────────────────────────────────
def generate_three_column():
    """Tests _assign_column KMeans with k=3."""
    fp = os.path.join(OUTPUT_DIR, "11_three_column.pdf")
    c = canvas.Canvas(fp, pagesize=A4)
    w, h = A4
    m = 1.5 * cm; gap = 0.5 * cm; cw = (w - 2 * m - 2 * gap) / 3
    c.setFont("Helvetica-Bold", 14)
    c.drawString(m, h - 2 * cm, "Three-Column Newsletter Layout")
    c.setFont("Helvetica", 8)
    cols = [
        ["COLUMN ONE HEADING", "KMeans should detect three", "x0 clusters. First column", "boxes get col_id=0.", "", "Text is merged vertically", "within each column before", "reading-order interleave."],
        ["COLUMN TWO HEADING", "The middle column should", "receive col_id=1 from", "the KMeans clustering.", "", "Three-column layouts are", "common in newsletters and", "some academic journals."],
        ["COLUMN THREE HEADING", "The right column receives", "col_id=2. All columns are", "then processed in order", "to produce the final text", "with correct reading order", "for downstream RAG use."],
    ]
    for ci, lines in enumerate(cols):
        x = m + ci * (cw + gap); y = h - 3.5 * cm
        for line in lines:
            c.drawString(x, y, line[:45]); y -= 11
    c.showPage()
    c.setFont("Helvetica-Bold", 14)
    c.drawString(m, h - 2 * cm, "Three-Column — Page 2")
    c.setFont("Helvetica", 8)
    for ci, lines in enumerate([
        ["Page 2 column one continued.", "Cross-page column assignments", "should remain consistent."],
        ["Page 2 column two continued.", "Testing multi-page column", "detection stability."],
        ["Page 2 column three continued.", "Persists across page breaks."],
    ]):
        x = m + ci * (cw + gap); y = h - 3.5 * cm
        for line in lines:
            c.drawString(x, y, line[:45]); y -= 11
    c.save()


# ── PDF 12: Mixed column layout ───────────────────────────────────────────
def generate_mixed_columns():
    """Full-width abstract + 2-column body on same page."""
    fp = os.path.join(OUTPUT_DIR, "12_mixed_columns.pdf")
    c = canvas.Canvas(fp, pagesize=A4)
    w, h = A4; m = 2 * cm; cw = (w - 2 * m - 1 * cm) / 2
    c.setFont("Helvetica-Bold", 14)
    c.drawString(m, h - 2 * cm, "Mixed Column Layout Paper")
    c.setFont("Helvetica", 9)
    abstract = (
        "Abstract: This paper presents an approach to document layout analysis "
        "that handles mixed single-column and multi-column layouts within the "
        "same page. The key insight is that column detection must operate at "
        "the page level and handle heterogeneous x0 distributions effectively."
    )
    # simple word wrap
    y = h - 3.2 * cm; words = abstract.split(); line = ""
    for wd in words:
        if c.stringWidth(line + wd, "Helvetica", 9) < w - 2 * m:
            line += wd + " "
        else:
            c.drawString(m, y, line.strip()); y -= 12; line = wd + " "
    if line.strip():
        c.drawString(m, y, line.strip()); y -= 20
    left_x = m; right_x = m + cw + 1 * cm; yc = y - 10
    c.setFont("Helvetica-Bold", 11)
    c.drawString(left_x, yc, "1. Introduction"); c.drawString(right_x, yc, "2. Related Work")
    c.setFont("Helvetica", 9)
    for i, (ll, rl) in enumerate(zip(
        ["Document layout analysis is a", "fundamental problem in document", "understanding. Modern approaches", "use deep learning for detection."],
        ["Previous work includes projection-", "profile and clustering methods.", "The KMeans-based method used here", "is simple yet effective."],
    )):
        cy = yc - 15 - i * 13
        c.drawString(left_x, cy, ll); c.drawString(right_x, cy, rl)
    c.save()


# ── PDF 13: Cross-page table ──────────────────────────────────────────────
def generate_crosspage_table():
    """Tests table merge logic across pages (L1242-1261)."""
    doc = _make_doc("13_crosspage_table.pdf")
    styles = getSampleStyleSheet()
    story = [Paragraph("Extended Financial Report", styles["Title"]), Spacer(1, 0.5 * cm)]
    hdr = ["Month", "Revenue", "Costs", "Profit", "Margin %"]
    data = [hdr]
    for i in range(1, 81):
        rev = 10000 + i * 500; cost = 6000 + i * 300; profit = rev - cost
        data.append([f"2024-{i:02d}", f"${rev:,}", f"${cost:,}", f"${profit:,}", f"{round(profit/rev*100,1)}%"])
    tbl = Table(data, colWidths=[3 * cm, 3 * cm, 3 * cm, 3 * cm, 3 * cm])
    tbl.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), colors.grey),
        ("TEXTCOLOR", (0, 0), (-1, 0), colors.whitesmoke),
        ("GRID", (0, 0), (-1, -1), 0.3, colors.grey),
        ("FONTNAME", (0, 0), (-1, 0), "Helvetica-Bold"),
        ("FONTSIZE", (0, 0), (-1, -1), 7),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, colors.lavender]),
    ]))
    story.append(tbl)
    story.append(Paragraph("Table: Monthly financial summary FY2024", styles["Normal"]))
    doc.build(story)


# ── PDF 14: Text + Table interleaved ──────────────────────────────────────
def generate_text_table_interleaved():
    """Mixed: paragraph → table → paragraph → table on same page."""
    doc = _make_doc("14_text_table_interleaved.pdf")
    styles = getSampleStyleSheet()
    story = [Paragraph("Product Analysis Report", styles["Title"]), Spacer(1, 0.3 * cm)]
    story.append(Paragraph(
        "The following analysis compares product performance across different "
        "market segments. Table 1 shows revenue by category.",
        styles["Normal"],
    ))
    for label, rows in [("Table 1: Revenue", [
        ["Category", "H1 Revenue", "Growth"],
        ["Software", "$500K", "+12%"], ["Hardware", "$350K", "+8%"],
        ["Services", "$300K", "+15%"], ["Other", "$270K", "+5%"],
    ]), ("Table 2: Satisfaction", [
        ["Category", "NPS", "Retention"],
        ["Software", "72", "92%"], ["Hardware", "65", "88%"],
        ["Services", "78", "95%"], ["Other", "60", "85%"],
    ])]:
        story.append(Spacer(1, 0.2 * cm))
        tbl = Table(rows, colWidths=[4 * cm, 3 * cm, 3 * cm])
        tbl.setStyle(TableStyle([
            ("BACKGROUND", (0, 0), (-1, 0), colors.HexColor("#4472C4")),
            ("TEXTCOLOR", (0, 0), (-1, 0), colors.white),
            ("GRID", (0, 0), (-1, -1), 0.3, colors.grey),
            ("FONTNAME", (0, 0), (-1, 0), "Helvetica-Bold"),
            ("FONTSIZE", (0, 0), (-1, -1), 9),
            ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, colors.aliceblue]),
        ]))
        story.append(tbl)
        story.append(Paragraph(label, styles["Normal"]))
        story.append(Paragraph("Analysis continues with additional metrics.", styles["Normal"]))
    doc.build(story)


# ── PDF 15: Sparse / empty pages ──────────────────────────────────────────
def generate_sparse_content():
    """Pages with little text, large whitespace gaps, empty pages."""
    fp = os.path.join(OUTPUT_DIR, "15_sparse_content.pdf")
    c = canvas.Canvas(fp, pagesize=A4)
    w, h = A4; m = 2 * cm
    c.setFont("Helvetica-Bold", 14)
    c.drawString(m, h - 2 * cm, "Sparse Content — Page 1")
    c.setFont("Helvetica", 10)
    c.drawString(m, h - 3 * cm, "Content only at top and one line at bottom.")
    c.drawString(m, m + 0.5 * cm, "Isolated footer line — large gap above.")
    c.showPage()  # Page 2: almost empty
    c.setFont("Helvetica", 10)
    c.drawString(m, h - 2 * cm, "Page 2: Only this single line.")
    c.showPage()  # Page 3: truly empty
    c.showPage()  # Page 4: resume
    c.setFont("Helvetica-Bold", 14)
    c.drawString(m, h - 2 * cm, "Sparse Content — Page 4")
    c.setFont("Helvetica", 10)
    for i, line in enumerate([
        "After an empty page, content resumes normally.",
        "The parser should handle empty pages gracefully,",
        "without crashing. page_cum_height still accumulates.",
    ]):
        c.drawString(m, h - 3.5 * cm - i * 14, line)
    c.save()


# ── PDF 16: Dense CJK long paragraphs ─────────────────────────────────────
def generate_dense_cjk():
    """Stress-tests vertical merge with many CJK boxes and mixed content."""
    doc = _make_doc("16_dense_cjk.pdf")
    styles = getSampleStyleSheet()
    try:
        from reportlab.pdfbase import pdfmetrics
        from reportlab.pdfbase.ttfonts import TTFont
        for path in [
            "/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
            "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
        ]:
            if os.path.exists(path):
                pdfmetrics.registerFont(TTFont("NotoCJK", path))
                styles.add(ParagraphStyle("CJKBody", fontName="NotoCJK", fontSize=10, leading=14))
                break
    except Exception:
        styles.add(ParagraphStyle("CJKBody", fontName="Helvetica", fontSize=10, leading=14))
    story = []
    story.append(Paragraph("深度学习在文档理解中的应用", styles["Title"]))
    para = (
        "文档理解是人工智能领域的重要研究方向之一，其目标是让计算机能够像人类一样"
        "理解和分析各种格式的文档内容。随着深度学习技术的快速发展，基于神经网络的"
        "文档理解方法取得了显著进展。这些方法通常包括文字检测、文字识别、版面分析、"
        "表格识别等多个子任务，每个子任务都有其独特的技术挑战。文字检测需要在复杂"
        "背景下准确定位文字区域，文字识别则需要处理各种字体、大小和排版的文字。"
        "版面分析要求系统能够理解文档的逻辑结构，区分标题、正文、表格、图片等不同"
        "的版面元素。表格识别则更加复杂，需要同时理解表格的物理结构和逻辑结构。"
        "近年来，基于Transformer架构的端到端文档理解模型逐渐成为研究热点，这些"
        "模型能够在一个统一的框架内完成多个子任务，大大简化了文档理解系统的设计。"
    ) * 12
    story.append(Paragraph(para, styles.get("CJKBody", styles["Normal"])))
    story.append(Spacer(1, 0.5 * cm))
    para2 = (
        "在RAG系统中，文档解析质量直接影响答案准确性。一个典型的RAG系统包括"
        "文档摄入管道、向量数据库（Milvus、Pinecone等）、检索引擎（BM25+vector"
        " similarity+reranking）以及大语言模型（GPT-4、Claude等）。根据2024年"
        "行业报告，超过78.5%的企业级AI应用采用了RAG架构。优秀文档解析器应达到："
        "文本准确率≥95%、表格F1≥90%、速度≥50 pages/min。"
    ) * 8
    story.append(Paragraph(para2, styles.get("CJKBody", styles["Normal"])))
    doc.build(story)


# ── Main ───────────────────────────────────────────────────────────────────
def main():
    os.makedirs(OUTPUT_DIR, exist_ok=True)
    generators = [
        ("09_crosspage_paragraph.pdf", generate_crosspage_paragraph),
        ("10_numbering_patterns.pdf", generate_numbering_patterns),
        ("11_three_column.pdf", generate_three_column),
        ("12_mixed_columns.pdf", generate_mixed_columns),
        ("13_crosspage_table.pdf", generate_crosspage_table),
        ("14_text_table_interleaved.pdf", generate_text_table_interleaved),
        ("15_sparse_content.pdf", generate_sparse_content),
        ("16_dense_cjk.pdf", generate_dense_cjk),
    ]
    for filename, gen_fn in generators:
        fp = os.path.join(OUTPUT_DIR, filename)
        if os.path.exists(fp):
            print(f"SKIP: {filename}")
            continue
        try:
            gen_fn()
            print(f"OK  : {filename}")
        except Exception as e:
            print(f"FAIL: {filename} — {e}")
    print(f"\nDone. PDFs in: {OUTPUT_DIR}")


if __name__ == "__main__":
    main()

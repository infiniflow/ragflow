"""Parser-layer regression guard for the Book builtin DSL highlight path.

The Book builtin DSL parses PDF with ``parse_method=DeepDOC`` and
``output_format=json``. The parser (rag/flow/parser/parser.py:781) runs
``normalize_pdf_items_metadata(bboxes)`` on the deepdoc output, which must
keep each box's PDF coordinates so the downstream TitleChunker can build
``position_int`` and the parsing-result view can highlight the text.

This test drives the REAL gate function with deepdoc-style boxes (carrying
both ``positions`` and ``position_tag``, exactly as
deepdoc/parser/pdf_parser.py:1900-1902 emits) and asserts the coordinates
survive into the internal ``_pdf_positions`` field without stripping the
original ``positions`` field. It is the parser-layer counterpart of
rag/flow/tests/test_title_chunker_position_int.py (which proves the
chunker layer keeps the coordinates). Together they pin down the full
parser -> chunker -> position_int bridge for infiniflow/ragflow#18148.
"""

from rag.flow.parser.pdf_chunk_metadata import (
    PDF_POSITIONS_KEY,
    extract_pdf_positions,
    normalize_pdf_items_metadata,
)


def _deepdoc_style_bboxes():
    # Shape mirrors deepdoc RAGFlowPdfParser.parse_into_bboxes output:
    # one entry with both position_tag + positions, two with positions only,
    # spanning page 1 and page 2.
    return [
        {
            "text": "Introduction paragraph on page one.",
            "layout_type": "text",
            "position_tag": "@@1\tIntroduction paragraph on page one.",
            "positions": [[1, 10, 200, 50, 80]],
        },
        {
            "text": "Body text continues on page one.",
            "layout_type": "text",
            "positions": [[1, 12, 205, 90, 120]],
        },
        {
            "text": "Second chapter starts on page two.",
            "layout_type": "text",
            "positions": [[2, 15, 210, 40, 75]],
        },
    ]


def test_parser_gate_preserves_bbox_coordinates():
    bboxes = _deepdoc_style_bboxes()
    # This is exactly what parser.py:781 calls for output_format == "json".
    normalize_pdf_items_metadata(bboxes)

    for box in bboxes:
        # Coordinate bridge: the chunker reads PDF_POSITIONS_KEY.
        assert box.get(PDF_POSITIONS_KEY), f"missing {PDF_POSITIONS_KEY}: {box}"
        # The original field must NOT be stripped by normalization.
        assert "positions" in box, f"positions stripped from box: {box}"

    # Pages referenced by the downstream chunker must cover every source page.
    pages = {int(p[0]) for box in bboxes for p in extract_pdf_positions(box)}
    assert pages == {1, 2}, f"expected pages {{1,2}}, got {pages}"


def test_parser_gate_produces_exact_coordinates():
    bboxes = _deepdoc_style_bboxes()
    normalize_pdf_items_metadata(bboxes)

    # Each normalized box keeps the source (page, left, right, top, bottom).
    assert extract_pdf_positions(bboxes[0]) == [[1, 10, 200, 50, 80]]
    assert extract_pdf_positions(bboxes[2]) == [[2, 15, 210, 40, 75]]

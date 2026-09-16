#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
from pathlib import Path
from unittest.mock import Mock

import pytest

from rag.app.pdf_naive_vision import (
    _apply_document_vertical_coords,
    merge_vlm_enhanced_bboxes_into_naive_pdf,
)

@pytest.mark.p1
def test_merge_updates_matching_section_text():
    pdf_parser = Mock()
    pdf_parser._line_tag = Mock(return_value="@@1\t0\t0\t0\t0##")
    sections = [("caption", "@@1\t0\t0\t0\t0##")]
    tables = []
    bboxes = [
        {
            "text": "caption\nVLM body",
            "image": object(),
            "position_tag": "@@1\t0\t0\t0\t0##",
            "positions": [[1, 0, 10, 0, 10]],
        }
    ]
    sections, tables = merge_vlm_enhanced_bboxes_into_naive_pdf(sections, tables, bboxes, pdf_parser)
    assert "VLM body" in sections[0][0]
    assert tables == []


@pytest.mark.p1
def test_merge_adds_table_when_no_section_match():
    pdf_parser = Mock()
    pdf_parser._line_tag = Mock(return_value="@@1\t0\t0\t0\t0##")
    bboxes = [
        {
            "text": "only vlm",
            "image": object(),
            "positions": [[1, 0, 10, 0, 10]],
        }
    ]
    sections, tables = merge_vlm_enhanced_bboxes_into_naive_pdf([], [], bboxes, pdf_parser)
    assert len(tables) == 1
    assert tables[0][0][1] == ["only vlm"]


@pytest.mark.p1
def test_apply_document_vertical_coords_for_supplemented_box():
    box = {
        "page_number": 2,
        "top": 10.0,
        "bottom": 20.0,
        "_embedded_supplement": True,
    }
    page_cum_height = [0, 100, 250]
    out = _apply_document_vertical_coords(box, page_cum_height)
    assert out["top"] == 110.0
    assert out["bottom"] == 120.0
    assert "_embedded_supplement" not in out


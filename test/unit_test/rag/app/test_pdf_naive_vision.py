#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
from unittest.mock import Mock

import pytest

from rag.app.pdf_naive_vision import merge_vlm_enhanced_bboxes_into_naive_pdf
from rag.flow.parser.pdf_chunk_metadata import apply_document_vertical_coords


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
def test_merge_puts_image_only_vlm_into_sections_not_tables():
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
    assert tables == []
    assert len(sections) == 1
    assert sections[0][0] == "only vlm"


@pytest.mark.p1
def test_merge_adds_table_when_orphan_image_with_existing_sections():
    pdf_parser = Mock()
    pdf_parser._line_tag = Mock(return_value="@@9\t0\t0\t0\t0##")
    sections = [("intro", "@@1\t0\t0\t0\t0##")]
    bboxes = [
        {
            "text": "orphan figure",
            "image": object(),
            "positions": [[2, 0, 10, 0, 10]],
        }
    ]
    sections, tables = merge_vlm_enhanced_bboxes_into_naive_pdf(sections, [], bboxes, pdf_parser)
    assert len(tables) == 1
    assert tables[0][0][1] == ["orphan figure"]
    assert sections[0][0] == "intro"


@pytest.mark.p1
def test_apply_document_vertical_coords_for_supplemented_box():
    box = {
        "page_number": 2,
        "top": 10.0,
        "bottom": 20.0,
        "_embedded_supplement": True,
    }
    page_cum_height = [0, 100, 250]
    out = apply_document_vertical_coords(box, page_cum_height)
    assert out["top"] == 110.0
    assert out["bottom"] == 120.0
    assert "_embedded_supplement" not in out


@pytest.mark.p1
def test_apply_document_vertical_coords_updates_positions():
    box = {
        "page_number": 2,
        "top": 10.0,
        "bottom": 20.0,
        "positions": [[2, 0, 100, 10, 20]],
        "_embedded_supplement": True,
    }
    page_cum_height = [0, 100, 250]
    out = apply_document_vertical_coords(box, page_cum_height)
    assert out["positions"][0][3:] == [110, 120]


@pytest.mark.p1
def test_merge_updates_existing_table_figure_by_position():
    from PIL import Image

    pdf_parser = Mock()
    pdf_parser._line_tag = Mock(return_value="@@9\t0\t0\t0\t0##")
    img = Image.new("RGB", (10, 10))
    tables = [((img, ["caption"]), [(1, 0.0, 10.0, 0.0, 10.0)])]
    bboxes = [
        {
            "text": "VLM caption",
            "image": img,
            "positions": [[2, 0, 10, 0, 10]],
        }
    ]
    _sections, tables = merge_vlm_enhanced_bboxes_into_naive_pdf([("body", "@@1\t0\t0\t0\t0##")], tables, bboxes, pdf_parser)
    assert len(tables) == 1
    assert tables[0][0][1] == ["VLM caption"]

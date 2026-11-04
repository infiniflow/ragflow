"""Focused regressions for RAGFlowPdfParser.get_position and _line_tag.

Internal boxes use a 1-based page_number inside the loaded window. top/bottom
are cumulative, pre-zoom y. page_from only shifts exported tags/positions and
must not change local get_position or _line_tag results. #19952 readers still
accept negative coordinates; the writer must emit an in-range page span.
"""

import copy
import logging
import math

import pytest
from PIL import Image

from test_pdf_parser_crop import _close, _load_pdf_parser, _page, _parser


@pytest.fixture
def parser_module(monkeypatch):
    return _load_pdf_parser(monkeypatch)


def _cum_height(pages, zoom):
    heights = [0.0]
    for page in pages:
        heights.append(heights[-1] + page.size[1] / zoom)
    return heights


def _box(page_number, x0, x1, top, bottom):
    return {"page_number": page_number, "x0": x0, "x1": x1, "top": top, "bottom": bottom}


def _ready(module, pages, zoom, page_from=0):
    parser = _parser(module, pages, page_from=page_from)
    parser.page_cum_height = _cum_height(pages, zoom)
    return parser


def _expand(poss, pages, zoom):
    """Expand extract_positions (0-based page lists, local endpoints) to per-page spans."""
    expanded = []
    for pns, left, right, top, bottom in poss:
        if not pns or any(b != a + 1 for a, b in zip(pns, pns[1:])):
            continue
        start = top
        end = bottom + sum(pages[pn].size[1] / zoom for pn in pns[:-1])
        if end <= start:
            continue
        offset = 0.0
        left, right = sorted((left, right))
        for pn in pns:
            height = pages[pn].size[1] / zoom
            width = pages[pn].size[0] / zoom
            y0 = max(0.0, min(start - offset, height))
            y1 = max(0.0, min(end - offset, height))
            x0 = max(0.0, min(left, width))
            x1 = max(0.0, min(right, width))
            offset += height
            if x1 > x0 and y1 > y0:
                expanded.append((pn + 1, x0, x1, y0, y1))
    return expanded


def test_single_page_zm_keeps_local_span(parser_module):
    pages = [_page(90, 120, (1, 2, 3))]
    bx = _box(1, 10.2, 40.8, 15.5, 48.25)
    parser = _ready(parser_module, pages, 1)
    _close(parser.get_position(bx, 1), [(1, 10.2, 40.8, 15.5, 48.25)])
    # ZM=3 intersects zoomed pixels, then converts back to pre-zoom units.
    zoomed = [_page(90 * 3, 120 * 3, (1, 2, 3))]
    parser = _ready(parser_module, zoomed, 3)
    _close(parser.get_position(bx, 3), [(1, 10.2, 40.8, 15.5, 48.25)])


def test_heterogeneous_pages_use_cumulative_prezoom_y(parser_module):
    pages = [
        _page(90, 40, (10, 0, 0)),
        _page(120, 50, (0, 10, 0)),
        _page(80, 30, (0, 0, 10)),
    ]
    for zoom in (1, 3):
        scaled = [_page(page.size[0] * zoom, page.size[1] * zoom, page.getpixel((0, 0))) for page in pages]
        parser = _ready(parser_module, scaled, zoom)
        # Local y on page 2 is 10; cumulative top is 40+10. Span crosses into page 3.
        bx = _box(2, 70, 100, 50, 40 + 50 + 12)
        _close(
            parser.get_position(bx, zoom),
            [
                (2, 70, 100, 10, 50),
                (3, 70, 80, 0, 12),
            ],
        )


def test_inverted_axes_are_sorted_before_clamp(parser_module):
    pages = [_page(100, 80, (255, 255, 255)), _page(60, 40, (1, 1, 1))]
    parser = _ready(parser_module, pages, 1)
    bx = _box(1, 80, 20, 70, 10)
    _close(parser.get_position(bx, 1), [(1, 20, 80, 10, 70)])

    # Inverted cumulative y still covers page 1 bottom through page 2.
    cross = _box(1, 90, 10, 100, 70)
    _close(
        parser.get_position(cross, 1),
        [
            (1, 10, 90, 70, 80),
            (2, 10, 60, 0, 20),
        ],
    )


def test_negative_top_and_bottom_are_clamped_per_page(parser_module, caplog):
    pages = [_page(100, 80, (255, 255, 255)), _page(100, 40, (0, 0, 1))]
    parser = _ready(parser_module, pages, 2)
    bx = _box(1, -20, 140, -15, 10)
    with caplog.at_level(logging.WARNING):
        _close(parser.get_position(bx, 2), [(1, 0, 50, 0, 10)])
    assert "list index out of range" not in caplog.text.lower()


def test_out_of_window_and_bad_page_numbers_return_empty(parser_module, caplog):
    pages = [_page(40, 30, (1, 1, 1)), _page(40, 30, (2, 2, 2))]
    parser = _ready(parser_module, pages, 1)
    cases = [
        _box(0, 1, 2, 0, 5),
        _box(-1, 1, 2, 0, 5),
        _box(3, 1, 2, 0, 5),
        _box("2", 1, 2, 0, 5),
        _box(float("nan"), 1, 2, 0, 5),
        _box(1, 1, 2, float("inf"), 5),
        _box(1, 1, 2, 0, float("nan")),
    ]
    with caplog.at_level(logging.WARNING):
        for bx in cases:
            assert parser.get_position(bx, 1) == []
    assert caplog.records
    assert "list index out of range" not in caplog.text.lower()


def test_missing_images_and_past_last_page_are_bounded(parser_module, caplog):
    empty = _parser(parser_module, [], page_from=0)
    empty.page_cum_height = [0]
    with caplog.at_level(logging.WARNING):
        assert empty.get_position(_box(1, 1, 2, 0, 4), 1) == []
    assert "page images" in caplog.text.lower() or "missing page geometry" in caplog.text.lower()

    pages = [_page(50, 20, (3, 3, 3)), _page(50, 20, (4, 4, 4))]
    parser = _ready(parser_module, pages, 1)
    # Starts on the last page and extends past it: keep the in-range slice only.
    overrun = _box(2, 5, 40, 25, 55)
    got = parser.get_position(overrun, 1)
    _close(got, [(2, 5, 40, 5, 20)])
    assert all(1 <= pn <= len(pages) for pn, *_rest in got)


def test_page_from_does_not_change_local_position_or_tag(parser_module):
    pages = [_page(90 * 3, 40 * 3, (9, 0, 0)), _page(80 * 3, 30 * 3, (0, 9, 0))]
    bx = _box(1, 10, 70, 30, 40 + 12)
    tags = {}
    for page_from in (0, 7):
        parser = _ready(parser_module, pages, 3, page_from=page_from)
        got = parser.get_position(bx, 3)
        _close(got, [(1, 10, 70, 30, 40), (2, 10, 70, 0, 12)])
        tags[page_from] = parser._line_tag(bx, 3)
    assert tags[0] == tags[7]
    assert tags[0].startswith("@@1-2\t")


def test_same_x_pages_share_one_tag_and_width_change_splits(parser_module):
    pages = [
        _page(100, 40, (1, 0, 0)),
        _page(100, 50, (0, 1, 0)),
        _page(60, 30, (0, 0, 1)),
    ]
    parser = _ready(parser_module, pages, 1)
    bx = _box(1, 10, 90, 10, 40 + 50 + 12)
    tag = parser._line_tag(bx, 1)
    # Last page of a same-x group keeps its own local bottom, not the first page height.
    assert tag == "@@1-2\t10.0\t90.0\t10.0\t50.0##@@3\t10.0\t60.0\t0.0\t12.0##"
    assert parser.extract_positions(tag)


def test_tag_extract_expansion_matches_get_position(parser_module):
    pages = [
        _page(90, 40, (10, 0, 0)),
        _page(120, 50, (0, 10, 0)),
        _page(80, 30, (0, 0, 10)),
    ]
    for zoom in (1, 3):
        scaled = [_page(p.size[0] * zoom, p.size[1] * zoom, p.getpixel((0, 0))) for p in pages]
        parser = _ready(parser_module, scaled, zoom, page_from=7)
        bx = _box(1, 70, 20, 35, 40 + 50 + 8)
        positions = parser.get_position(bx, zoom)
        tag = parser._line_tag(bx, zoom)
        expanded = _expand(parser_module.RAGFlowPdfParser.extract_positions(tag), scaled, zoom)
        _close(expanded, positions)
        # Writer stays in the local window; page_from is not baked into the tag.
        assert "8" not in tag.split("\t", 1)[0]


def test_invalid_box_tag_is_empty_and_does_not_emit_negative_page(parser_module, caplog):
    pages = [_page(40, 20, (1, 1, 1))]
    parser = _ready(parser_module, pages, 1)
    with caplog.at_level(logging.WARNING):
        for bx in (_box(0, 1, 2, 0, 5), _box(-1, 1, 2, 0, 5), _box(2, 1, 2, 0, 5)):
            assert parser._line_tag(bx, 1) == ""
            assert "-1\t" not in parser._line_tag(bx, 1)
    # #19952: a leading minus is a coordinate, so "-1" is not a readable page token.
    # The writer must still never emit that token as a page span.
    assert parser_module.RAGFlowPdfParser.extract_positions("@@-1\t1.0\t2.0\t3.0\t4.0##") == []
    assert parser_module.RAGFlowPdfParser.extract_positions("@@1\t-1.0\t2.0\t3.0\t4.0##") == [([0], -1.0, 2.0, 3.0, 4.0)]


def test_get_position_does_not_mutate_input_box(parser_module):
    pages = [_page(80, 40, (1, 1, 1)), _page(60, 20, (2, 2, 2))]
    parser = _ready(parser_module, pages, 1)
    bx = _box(1, 50, 10, 50, 5)
    snapshot = copy.deepcopy(bx)
    parser.get_position(bx, 1)
    parser._line_tag(bx, 1)
    assert bx == snapshot


def test_writer_extract_crop_uses_real_pil_chain(parser_module):
    pages = [
        _page(90, 40, (255, 0, 0)),
        _page(70, 30, (0, 0, 255)),
    ]
    parser = _ready(parser_module, pages, 1, page_from=4)
    bx = _box(1, 10, 80, 10, 40 + 12)
    tag = parser._line_tag(bx, 1)
    image, positions = parser.crop(tag, ZM=1, need_position=True)
    expected = parser.get_position(bx, 1)
    # crop returns the 0-based page index plus page_from; get_position stays 1-based local.
    _close([(pn - parser.page_from + 1, *rest) for pn, *rest in positions], expected)
    assert [pn for pn, *_rest in positions] == [local_pn - 1 + parser.page_from for local_pn, *_rest in expected]
    assert isinstance(image, Image.Image)
    # Head context is dimmed. Page-1 body starts after that 4px context plus the gap.
    assert image.getpixel((0, 4 + 6)) == (255, 0, 0)
    # Page-1 body is 30px, then a 6px gap, then page 2.
    assert image.getpixel((0, 4 + 6 + 30 + 6)) == (0, 0, 255)
    assert all(math.isfinite(v) for pos in positions for v in pos[1:])


def test_numpy_cumulative_heights_match_runtime_geometry(parser_module):
    import numpy as np

    pages = [_page(300, 300, "white"), _page(300, 450, "white")]
    parser = _ready(parser_module, pages, 3)
    parser.page_cum_height = np.cumsum([0, 100, 150])
    bx = _box(np.int64(2), 10, 30, 110, 140)
    _close(parser.get_position(bx, 3), [(2, 10, 30, 10, 40)])
    assert parser._line_tag(bx, 3) == "@@2\t10.0\t30.0\t10.0\t40.0##"


@pytest.mark.parametrize("zoom", [0, -1, float("inf"), float("nan"), "invalid"])
def test_invalid_zoom_does_not_generate_positions(parser_module, zoom):
    parser = _ready(parser_module, [_page(100, 100, "white")], 1)
    assert parser.get_position(_box(1, 10, 30, 10, 20), zoom) == []
    assert parser._line_tag(_box(1, 10, 30, 10, 20), zoom) == ""

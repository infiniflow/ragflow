#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""Reading-order merge of the PDF media and text chunk streams.

``chunk_order_int`` numbers chunks by list position, so concatenating the two
streams put every image and table ahead of the text.
"""

import pytest

from rag import nlp


def _chunk(name, page=None, top=None, left=50, right=None):
    """Build a tokenized chunk; ``page=None`` models one without positions.

    ``left``/``right`` default to a left-column box; pass a right-column range
    for a two-column page.
    """
    ck = {"content_with_weight": name}
    if page is not None:
        top = top if top is not None else 0
        right = right if right is not None else left + 250
        ck["page_num_int"] = [page]
        ck["top_int"] = [top]
        ck["position_int"] = [(page, left, right, top, top + 10)]
    return ck


@pytest.mark.p2
def test_media_chunks_interleave_between_text_chunks():
    # t1 < img-p2 < t2, and t3 starts above tbl-p3 on page 3.
    text = [_chunk("t1", 1, 10), _chunk("t2", 2, 900), _chunk("t3", 3, 20)]
    media = [_chunk("img-p2", 2, 40), _chunk("tbl-p3", 3, 500)]

    merged = nlp.merge_chunks_by_reading_order(media, text)

    assert [ck["content_with_weight"] for ck in merged] == ["t1", "img-p2", "t2", "t3", "tbl-p3"]


@pytest.mark.p2
def test_media_ahead_of_every_text_chunk_stays_ahead():
    text = [_chunk("t1", 4, 10), _chunk("t2", 5, 10)]
    media = [_chunk("img-p1", 1, 30), _chunk("img-p2", 2, 30)]

    merged = nlp.merge_chunks_by_reading_order(media, text)

    assert [ck["content_with_weight"] for ck in merged] == ["img-p1", "img-p2", "t1", "t2"]


@pytest.mark.p2
def test_media_after_every_text_chunk_stays_behind():
    text = [_chunk("t1", 1, 10), _chunk("t2", 2, 10)]
    media = [_chunk("img-p9", 9, 30)]

    merged = nlp.merge_chunks_by_reading_order(media, text)

    assert [ck["content_with_weight"] for ck in merged] == ["t1", "t2", "img-p9"]


@pytest.mark.p2
def test_unsorted_media_is_sorted_by_position():
    # ``tokenize_table`` emits every figure before every table.
    text = [_chunk("t1", 1, 10), _chunk("t2", 2, 900)]
    media = [_chunk("fig-p4", 4, 50), _chunk("tbl-p1", 1, 400)]

    merged = nlp.merge_chunks_by_reading_order(media, text)

    assert [ck["content_with_weight"] for ck in merged] == ["t1", "tbl-p1", "t2", "fig-p4"]


@pytest.mark.p2
def test_two_column_text_order_is_preserved():
    # Column-major, so the text tops go 100 -> 700 -> 120 -> 400: sorting the
    # text by position would scramble it, only the media chunk moves.
    text = [
        _chunk("left-top", 1, 100),
        _chunk("left-bottom", 1, 700),
        _chunk("right-top", 1, 120, left=308, right=546),
        _chunk("right-body", 1, 400, left=308, right=546),
    ]
    media = [_chunk("fig-right", 1, 130, left=308, right=546)]

    merged = nlp.merge_chunks_by_reading_order(media, text)

    assert [ck["content_with_weight"] for ck in merged] == [
        "left-top",
        "left-bottom",
        "right-top",
        "fig-right",
        "right-body",
    ]


@pytest.mark.p2
def test_right_column_table_stays_behind_left_column_figure():
    # first_order-7_merged.pdf p1: Table 3 in the right column (431) sits higher
    # than Figure 5 in the left column (487), but the left column is read first.
    text = [
        _chunk("left-text", 1, 520),
        _chunk("right-text", 1, 480, left=308, right=546),
    ]
    media = [
        _chunk("Table 3", 1, 431, left=308, right=546),
        _chunk("Figure 5", 1, 487),
    ]

    merged = nlp.merge_chunks_by_reading_order(media, text)

    assert [ck["content_with_weight"] for ck in merged] == [
        "Figure 5",
        "left-text",
        "Table 3",
        "right-text",
    ]


@pytest.mark.p2
def test_media_heads_a_single_multicolumn_text_chunk():
    # first_order-7_merged.pdf p1 as stored: one text chunk covers the page and
    # starts in the right column (444), with four media blocks above it. The
    # media order must follow the geometry, not the order they were handed in.
    text = [_chunk("page-text", 1, 444, left=308, right=546)]
    text[0]["position_int"].append((1, 50, 286, 515, 524))
    media = [
        _chunk("Table 3", 1, 378, left=309, right=543),
        _chunk("Figure 5", 1, 380, left=61, right=271),
        _chunk("wide-table", 1, 274, left=49, right=545),
        _chunk("wide-figure", 1, 68, left=48, right=546),
    ]

    merged = nlp.merge_chunks_by_reading_order(media, text)

    assert [ck["content_with_weight"] for ck in merged] == [
        "wide-figure",
        "wide-table",
        "Figure 5",
        "Table 3",
        "page-text",
    ]


@pytest.mark.p2
def test_media_order_does_not_depend_on_input_order():
    # A full-width block between two columns makes pairwise comparison
    # inconsistent, so the result must not depend on the input order.
    text = [
        _chunk("left-text", 1, 900, left=50, right=288),
        _chunk("right-text", 1, 30, left=308, right=546),
    ]
    left_bottom = _chunk("left-bottom", 1, 700, left=50, right=288)
    right_top = _chunk("right-top", 1, 100, left=308, right=546)
    wide_middle = _chunk("wide-middle", 1, 500, left=50, right=546)

    orders = (
        [left_bottom, right_top, wide_middle],
        [right_top, wide_middle, left_bottom],
        [wide_middle, left_bottom, right_top],
    )
    results = {tuple(ck["content_with_weight"] for ck in nlp.merge_chunks_by_reading_order(list(order), text)) for order in orders}

    assert len(results) == 1


@pytest.mark.p2
def test_unpositioned_media_keeps_the_historical_order():
    text = [_chunk("t1", 1, 10), _chunk("t2", 2, 10)]
    media = [_chunk("img")]

    merged = nlp.merge_chunks_by_reading_order(media, text)

    assert [ck["content_with_weight"] for ck in merged] == ["img", "t1", "t2"]


@pytest.mark.p2
def test_unpositioned_text_keeps_the_historical_order():
    text = [_chunk("t1"), _chunk("t2")]
    media = [_chunk("img-p2", 2, 30)]

    merged = nlp.merge_chunks_by_reading_order(media, text)

    assert [ck["content_with_weight"] for ck in merged] == ["img-p2", "t1", "t2"]


@pytest.mark.p2
def test_equal_position_places_media_ahead_of_text():
    text = [_chunk("t1", 2, 40)]
    media = [_chunk("img", 2, 40)]

    merged = nlp.merge_chunks_by_reading_order(media, text)

    assert [ck["content_with_weight"] for ck in merged] == ["img", "t1"]


@pytest.mark.p2
def test_empty_stream_passes_through():
    text = [_chunk("t1", 1, 10)]

    assert nlp.merge_chunks_by_reading_order(None, text) == text
    assert nlp.merge_chunks_by_reading_order([], text) == text

    media = [_chunk("img", 1, 30)]
    assert nlp.merge_chunks_by_reading_order(media, None) == media
    assert nlp.merge_chunks_by_reading_order(media, []) == media


@pytest.mark.p2
def test_reading_order_key_reads_the_first_position():
    assert nlp.reading_order_key(_chunk("t", 3, 42)) == (3, 50, 300, 42)
    assert nlp.reading_order_key(_chunk("t")) is None
    assert nlp.reading_order_key(_chunk("t", 3, None)) == (3, 50, 300, 0)

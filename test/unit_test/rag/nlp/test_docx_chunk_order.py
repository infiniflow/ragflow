#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

"""Pin the chunk order produced by ``_build_cks`` when a table/image follows text.

Reported in #19520: plain text is buffered in ``seg`` until a delimiter
matches, whereas table/image chunks are appended immediately. A caption
paragraph that preceded a table (or image) was therefore emitted *after* it,
which both broke the document order and inverted ``context_above`` /
``context_below`` in ``_add_context`` (it decides "above" / "below" purely by
array position).

The fix flushes the pending text before a table/image chunk is appended.
These tests pin:

* a caption paragraph that buffered before a table/image stays before it;
* ``context_above`` / ``context_below`` follow the document order;
* the two control cases — a table with no preceding text, and an empty
  ``delimiter`` field — keep their pre-existing, already-correct behaviour.
"""

from rag.nlp import _add_context, _build_cks

TABLE_HTML = "<table><tr><td>x</td></tr></table>"
DELIMITER = "\n"
CONTEXT_SIZE = 128


def _build(sections, delimiter=DELIMITER):
    """Return ``(cks, tables, images)`` for ``sections`` (drops ``has_custom``)."""
    cks, tables, images, _ = _build_cks(sections, delimiter)
    return cks, tables, images


def test_caption_paragraph_precedes_table_chunk_when_buffered_before_it():
    cks, tables, _ = _build(
        [
            ("table caption", None, None),
            ("", None, TABLE_HTML),
            ("following paragraph", None, None),
        ]
    )
    assert [ck["ck_type"] for ck in cks] == ["text", "table", "text"]
    assert cks[tables[0] - 1]["text"] == "table caption"
    assert cks[tables[0] + 1]["text"] == "following paragraph"


def test_table_context_above_and_below_follow_document_order():
    cks, tables, _ = _build(
        [
            ("table caption", None, None),
            ("", None, TABLE_HTML),
            ("following paragraph", None, None),
        ]
    )
    _add_context(cks, tables[0], CONTEXT_SIZE)
    assert cks[tables[0]]["context_above"] == "table caption"
    assert cks[tables[0]]["context_below"] == "following paragraph"


def test_caption_paragraph_precedes_image_chunk_when_buffered_before_it():
    cks, _, images = _build(
        [
            ("figure caption", None, None),
            ("", "IMG", None),
            ("following paragraph", None, None),
        ]
    )
    assert [ck["ck_type"] for ck in cks] == ["text", "image", "text"]
    assert cks[images[0] - 1]["text"] == "figure caption"
    assert cks[images[0] + 1]["text"] == "following paragraph"


def test_image_context_above_and_below_follow_document_order():
    cks, _, images = _build(
        [
            ("figure caption", None, None),
            ("", "IMG", None),
            ("following paragraph", None, None),
        ]
    )
    _add_context(cks, images[0], CONTEXT_SIZE)
    assert cks[images[0]]["context_above"] == "figure caption"
    assert cks[images[0]]["context_below"] == "following paragraph"


def test_table_without_preceding_text_keeps_its_order():
    """Control: nothing is buffered, so the table already led on main."""
    cks, tables, _ = _build(
        [
            ("", None, TABLE_HTML),
            ("following paragraph", None, None),
        ]
    )
    assert [ck["ck_type"] for ck in cks] == ["table", "text"]
    assert tables == [0]


def test_empty_delimiter_keeps_its_order():
    """Control: with no delimiter, text is appended directly and never buffered."""
    cks, tables, _ = _build(
        [
            ("table caption", None, None),
            ("", None, TABLE_HTML),
            ("following paragraph", None, None),
        ],
        delimiter="",
    )
    assert [ck["ck_type"] for ck in cks] == ["text", "table", "text"]
    assert cks[0]["text"] == "table caption"
    assert cks[tables[0] + 1]["text"] == "following paragraph"

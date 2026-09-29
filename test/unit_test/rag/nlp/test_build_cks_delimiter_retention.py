#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""Regression tests for delimiter retention in ``_build_cks`` / ``naive_merge_docx``.

``_build_cks`` is the docx/text-with-tables path used by the ``naive`` builtin
parser. Like the ``naive_merge`` default path, it used to *drop* the matched
bare delimiter and thereby lose sentence punctuation (``。；！？`` / ``.`` /
``!``) from text chunks — the same defect the Go ``TokenChunker`` fixed in
#20276. These tests pin the corrected contract:

* A *bare* delimiter is retained by attaching it to the text segment that
  precedes it, so concatenating the emitted text chunks reproduces the source
  exactly (no dropped characters).
* A *custom* (backtick-wrapped) delimiter is still dropped.
* Whitespace between consecutive delimiters is folded into the buffer (no empty
  text chunk, source spacing preserved).
"""

from __future__ import annotations

from rag.nlp import _build_cks, naive_merge_docx


def _texts(cks):
    return [ck["text"] for ck in cks if ck["ck_type"] == "text"]


def _joined_texts(cks):
    return "".join(_texts(cks))


# --------------------------------------------------------------------------- #
# _build_cks — bare delimiters retained
# --------------------------------------------------------------------------- #


def test_bare_delimiter_retained_in_text_chunk():
    # A single text paragraph split by a bare sentence delimiter keeps the
    # punctuation inside the produced text chunk.
    cks, _, _, has_custom = _build_cks([("first part。second part", None, None)], "。")
    assert has_custom is False
    joined = _joined_texts(cks)
    assert "。" in joined
    assert joined == "first part。second part", joined


def test_lossless_reproduces_source_across_sections():
    # Two text sections with sentence punctuation; each section survives with
    # its delimiter attached. (Consecutive text sections are buffered into one
    # chunk by _build_cks, so the inter-section newline is preserved — the
    # delimiters themselves are never dropped.)
    cks, _, _, _ = _build_cks(
        [("A。B", None, None), ("C！D", None, None)],
        "。！",
    )
    joined = _joined_texts(cks)
    assert "A。B" in joined, joined
    assert "C！D" in joined, joined


def test_whitespace_between_delimiters_folded_no_empty_chunk():
    # "a。 b" split on "。" — the whitespace between the delimiter and the next
    # word is folded into the buffer, so no empty text chunk is emitted.
    cks, _, _, _ = _build_cks([("a。 b", None, None)], "。")
    texts = _texts(cks)
    assert all(t.strip() for t in texts), texts
    assert _joined_texts(cks) == "a。 b", _joined_texts(cks)


def test_delimiter_before_table_is_retained_in_caption():
    # A caption paragraph ending with a delimiter, followed by a table chunk.
    # The caption text keeps its trailing punctuation (#19520 flush order +
    # #20276 retention).
    cks, tables, _, _ = _build_cks(
        [("caption。", None, None), ("", None, "<table></table>")],
        "。",
    )
    caption = cks[tables[0] - 1]["text"]
    assert caption.endswith("。"), caption


# --------------------------------------------------------------------------- #
# _build_cks — custom (backtick-wrapped) delimiters still dropped
# --------------------------------------------------------------------------- #


def test_custom_backtick_delimiter_dropped():
    cks, _, _, has_custom = _build_cks([("aa;bb;cc", None, None)], "`;`")
    assert has_custom is True
    joined = _joined_texts(cks)
    assert ";" not in joined, joined
    assert all(p in joined for p in ["aa", "bb", "cc"]), joined


# --------------------------------------------------------------------------- #
# naive_merge_docx end-to-end (wraps _build_cks + _merge_cks)
# --------------------------------------------------------------------------- #


def test_naive_merge_docx_retains_bare_delimiter():
    chunks, _ = naive_merge_docx(
        [("first part。second part", None, None)],
        chunk_token_num=512,
        delimiter="。",
    )
    joined = "".join(c["text"] for c in chunks if c["ck_type"] == "text")
    assert "。" in joined
    assert joined == "first part。second part", joined

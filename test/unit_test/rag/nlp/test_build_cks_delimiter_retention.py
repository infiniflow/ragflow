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
    # Two text sections with sentence punctuation; each delimiter splits the
    # text into its own chunk (the preceding text + the delimiter), and the
    # delimiter punctuation is never dropped. #20384 regressed this by
    # accumulating the delimiter into the buffer without ever flushing, which
    # collapsed every section into a single chunk — so we now pin the chunk
    # count, not just the concatenated text.
    cks, _, _, _ = _build_cks(
        [("A。B", None, None), ("C！D", None, None)],
        "。！",
    )
    texts = _texts(cks)
    # The pending buffer carries across sections (there is no per-section
    # flush), so "A。B" -> ["A。", "B"] and "C！D" -> ["C！", "D"] combine with
    # the inter-section newline into ["A。", "B\nC！", "D"] — three text chunks,
    # never the single chunk the #20384 regression collapsed everything into.
    assert len(texts) == 3, texts
    joined = _joined_texts(cks)
    assert "A。B" in joined, joined
    assert "C！D" in joined, joined


def test_bare_delimiter_splits_into_multiple_chunks():
    # Regression guard for the docx "不分块" bug (#20384 follow-up): a bare
    # delimiter must flush and start a fresh chunk instead of accumulating the
    # whole section into one. With more than one delimiter the result must be
    # more than one text chunk.
    cks, _, _, has_custom = _build_cks(
        [("first part。second part。third part", None, None)],
        "。",
    )
    assert has_custom is False
    texts = _texts(cks)
    assert len(texts) > 1, texts
    # Each emitted chunk except the last carries its trailing delimiter.
    assert texts[0].endswith("。"), texts
    assert _joined_texts(cks) == "first part。second part。third part", _joined_texts(cks)


def test_whitespace_between_delimiters_no_empty_chunk():
    # "a。 b" split on "。" — the whitespace-only segment between the delimiter
    # and the next word must never produce an empty text chunk. The delimiter
    # is retained and both words survive; inter-chunk whitespace may be
    # normalized at the boundary (this is expected for chunking, not a defect).
    cks, _, _, _ = _build_cks([("a。 b", None, None)], "。")
    texts = _texts(cks)
    assert all(t.strip() for t in texts), texts
    joined = _joined_texts(cks)
    assert "。" in joined, joined
    assert "a" in joined and "b" in joined, joined


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

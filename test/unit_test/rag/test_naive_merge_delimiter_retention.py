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

"""Regression tests for lossless delimiter handling in ``naive_merge`` /
``naive_merge_with_images`` (Python parity with the Go TokenChunker fix #20276).

Before #20276 the default path dropped the matched *bare* delimiter and
re-inserted a synthetic ``"\\n"`` between merged pieces, so sentence
punctuation (``。；！？`` / ``.`` / ``!``) was lost from chunk text and
spurious newlines were injected. These tests pin the corrected contract:

* A *bare* delimiter is retained by attaching it to the paragraph that
  precedes it, so concatenating the emitted chunks reproduces the source
  exactly (no dropped characters, no synthesized newline).
* A *custom* (backtick-wrapped) delimiter is still dropped.
* The ``"\\n"`` that separates original sections is preserved; blank lines
  and whitespace-only segments between consecutive delimiters are folded into
  an adjacent paragraph (no empty chunk).
"""

from __future__ import annotations

import pytest

pytestmark = pytest.mark.usefixtures("pdf_parser_stub")

from rag import nlp
from rag.nlp import naive_merge, naive_merge_with_images


@pytest.fixture(autouse=True)
def _mock_token_counter(monkeypatch):
    """Return 1 token per string so every input stays in a single chunk and we
    can assert the split/merge output purely from delimiter behavior."""

    def fake(_s):
        return 1

    monkeypatch.setattr(nlp, "num_tokens_from_string", fake)


def _joined(sections, delimiter="。；！？", **kw):
    return "".join(naive_merge(list(sections), chunk_token_num=512, delimiter=delimiter, **kw))


# --------------------------------------------------------------------------- #
# Bare delimiters are retained (lossless)
# --------------------------------------------------------------------------- #


def test_bare_sentence_delimiter_retained():
    joined = _joined(["句子一。句子二"], delimiter="。")
    # The delimiter is kept on the chunk it ends.
    assert joined == "\n句子一。句子二", joined
    assert "。" in joined


def test_lossless_reproduces_source_across_sections():
    # Two sections (e.g. two lines of a book) with sentence punctuation.
    joined = _joined(["A。B", "C!D"], delimiter="。！")
    # Source with the inter-section newline is reproduced exactly.
    assert joined == "\nA。B\nC!D", joined
    assert "。" in joined and "!" in joined


def test_no_synthetic_newline_within_section():
    # Within a single section the retained delimiter separates the pieces; no
    # extra "\\n" is injected between them.
    joined = _joined(["ab。cd"], delimiter="。")
    assert joined == "\nab。cd", joined


def test_empty_delimiter_keeps_inter_section_newline():
    # Regression guard: with an empty delimiter (size-only mode) every section
    # keeps its leading "\n" separator, so multi-section input is NOT
    # concatenated into one blob. (#20384 review — naive_merge empty-delim path)
    chunks = naive_merge([("sec1", ""), ("sec2", "")], chunk_token_num=512, delimiter="")
    assert "".join(chunks) == "\nsec1\nsec2", chunks


def test_empty_delimiter_with_images_keeps_inter_section_newline():
    # Same contract for the images path; the two splitters must stay consistent.
    chunks, _images = naive_merge_with_images(
        ["sec1", "sec2"],
        [None, None],
        chunk_token_num=512,
        delimiter="",
    )
    assert "".join(chunks) == "\nsec1\nsec2", chunks


def test_book_txt_flow_retains_punctuation_and_line_breaks():
    # Mirrors rag/app/book.py's txt path: split on "\n", then naive_merge with
    # the default Chinese delimiter. Sentence punctuation must survive and the
    # line break between the two lines must be preserved.
    txt = "第一章。这是内容\n第二章。更多内容"
    sections = [(line, "") for line in txt.split("\n") if line]
    joined = _joined(sections, delimiter="\n。；！？")
    assert joined.lstrip("\n") == "第一章。这是内容\n第二章。更多内容", joined


# --------------------------------------------------------------------------- #
# Custom (backtick-wrapped) delimiters are still dropped
# --------------------------------------------------------------------------- #


def test_custom_backtick_delimiter_dropped():
    joined = _joined(["aa;bb;cc"], delimiter="`;``?`")
    assert ";" not in joined and "?" not in joined, joined
    assert all(p in joined for p in ["aa", "bb", "cc"]), joined


# --------------------------------------------------------------------------- #
# Whitespace / blank-line folding
# --------------------------------------------------------------------------- #


def test_blank_line_between_delimiters_folded_no_empty_chunk():
    # "a\n\nb" split on "\n": the two newlines are both retained (no empty
    # chunk emitted).
    joined = _joined(["a\n\nb"], delimiter="\n")
    assert joined.lstrip("\n") == "a\n\nb", joined
    chunks = naive_merge(["a\n\nb"], chunk_token_num=512, delimiter="\n")
    # No whitespace-only chunk.
    assert all(c.strip() for c in chunks), chunks


# --------------------------------------------------------------------------- #
# naive_merge_with_images mirrors the same contract
# --------------------------------------------------------------------------- #


def test_with_images_retains_bare_delimiter():
    chunks, _images = naive_merge_with_images(
        ["句子一。句子二"],
        [None],
        chunk_token_num=512,
        delimiter="。",
    )
    joined = "".join(chunks)
    assert joined == "\n句子一。句子二", joined

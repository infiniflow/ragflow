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

"""Tests for the paper chunk policy's size bound (rag/app/paper.py).

``_merge_sections_by_pivot`` concatenates sections sharing a title pivot but
bounds the accumulation at ``chunk_token_num`` so a long section is not emitted
as one oversized chunk.
"""

import pytest

import rag.app.paper as paper
from rag.app.paper import _merge_sections_by_pivot


@pytest.fixture(autouse=True)
def word_count_tokens(monkeypatch):
    monkeypatch.setattr(paper, "num_tokens_from_string", lambda s: len((s or "").split()))


def _sections(n, words_each=4):
    return [(" ".join(["word"] * words_each), "") for _ in range(n)]


@pytest.mark.p2
def test_long_pivot_is_split_into_bounded_chunks():
    # One pivot, 10 sections x 4 tokens = 40 tokens.
    chunks = _merge_sections_by_pivot(_sections(10), [0] * 10, chunk_token_num=12)
    assert len(chunks) > 1
    # A section is never split mid-way, so allow one section of slack over the budget.
    assert all(len(c.split()) <= 12 + 4 for c in chunks)
    assert sum(c.count("word") for c in chunks) == 40


@pytest.mark.p2
def test_nonpositive_budget_keeps_one_chunk_per_pivot():
    chunks = _merge_sections_by_pivot(_sections(10), [0] * 10, chunk_token_num=0)
    assert len(chunks) == 1
    assert len(chunks[0].split()) == 40


@pytest.mark.p2
def test_distinct_pivots_are_never_merged():
    sections = [("alpha", ""), ("beta", ""), ("gamma", "")]
    assert _merge_sections_by_pivot(sections, [0, 1, 2], chunk_token_num=1000) == ["alpha", "beta", "gamma"]


@pytest.mark.p2
def test_position_tags_do_not_count_toward_budget():
    # Each section is 4 body tokens plus a position tag. The @@...## tag is
    # cropping metadata, not content, so it must not consume the budget: 3 x 4 = 12
    # body tokens fit chunk_token_num=12 and stay one chunk. Were the tag counted
    # (it splits into several whitespace-separated tokens) the pivot would break up.
    tag = "@@1\t72.0\t523.0\t88.0\t101.0##"
    sections = [("word word word word" + tag, "") for _ in range(3)]
    chunks = _merge_sections_by_pivot(sections, [0, 0, 0], chunk_token_num=12)
    assert len(chunks) == 1
    # The tags stay in the chunk so pdf_parser.crop() can still map page + bbox.
    assert chunks[0].count("@@") == 3


@pytest.mark.p2
def test_chunk_bounds_oversized_pivot(monkeypatch):
    # End-to-end guard through paper.chunk(): a single title pivot spanning many
    # sections must NOT collapse into one oversized chunk. Drives the real merge
    # path; red against the pre-fix unbounded loop (one 24-token chunk), green
    # once chunk_token_num caps it. Everything around the merge is stubbed so the
    # test needs no PDF / model.
    sections = _sections(6)  # 6 sections x 4 tokens, one pivot => 24 tokens

    class FakePdf:
        def __call__(self, *a, **k):
            return {"title": "t", "authors": " ", "abstract": "", "sections": sections, "tables": []}

        def remove_tag(self, s):
            return s

    captured = {}

    def fake_tokenize_chunks(chunks, *a, **k):
        captured["chunks"] = chunks
        return []

    class FakeTok:
        def tokenize(self, s):
            return ""

        def fine_grained_tokenize(self, s):
            return ""

    monkeypatch.setattr(paper, "normalize_layout_recognizer", lambda x: ("DeepDOC", None))
    monkeypatch.setattr(paper, "Pdf", FakePdf)
    monkeypatch.setattr(paper, "vision_figure_parser_pdf_wrapper", lambda tbls, **k: tbls)
    monkeypatch.setattr(paper, "bullets_category", lambda *_: 1)
    # One pivot: every section shares a level that never advances the pivot id.
    monkeypatch.setattr(paper, "title_frequency", lambda bull, secs: (1, [2] * len(secs)))
    monkeypatch.setattr(paper, "tokenize_table", lambda *a, **k: [])
    monkeypatch.setattr(paper, "tokenize_chunks", fake_tokenize_chunks)
    monkeypatch.setattr(paper, "rag_tokenizer", FakeTok())

    paper.chunk("doc.pdf", binary=b"x", callback=lambda *a, **k: None, parser_config={"chunk_token_num": 12, "layout_recognize": "DeepDOC"})

    chunks = captured["chunks"]
    assert len(chunks) > 1, f"oversized pivot collapsed into {len(chunks)} chunk(s)"
    # A section is never split mid-way, so allow one section of slack over budget.
    assert all(len(c.split()) <= 12 + 4 for c in chunks)
    assert sum(c.count("word") for c in chunks) == 24  # nothing lost

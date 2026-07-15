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

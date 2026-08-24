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

"""Tests for the laws chunk policy's size bound (rag/app/laws.py -> tree_merge).

A dataset with ``chunk_method="laws"`` ingesting one heading followed by a long,
sub-heading-free body used to come out of ``tree_merge`` as a single unbounded
chunk: over-depth lines accumulate onto the current heading's ``Node`` with no
token counting (``Node.build_tree``), and ``Node._dfs`` joined the whole
accumulation into one chunk with no cap. This mirrors the fix #16959 made for
``hierarchical_merge`` (book/paper), applied to ``tree_merge`` (laws), which
#16959 does not touch.
"""

import re

import pytest

import rag.nlp as nlp
from rag.nlp import tree_merge

# bull=3 is the BULLET_PATTERN entry for "PART/Chapter/Section/Article" headings,
# the exact style reproduced against upstream (a single "Article 1" heading with
# a long body and no sub-headings).
BULL = 3


@pytest.fixture(autouse=True)
def word_count_tokens(monkeypatch):
    """Count tokens as whitespace-delimited words, ignoring the ``@@..##`` tag,
    same as tree_merge's own tag-stripping so tests don't depend on tiktoken."""

    def fake_num_tokens(s):
        s = re.sub(r"@@[0-9]+.*", "", s or "")
        return len(s.split())

    monkeypatch.setattr(nlp, "num_tokens_from_string", fake_num_tokens)


def _body(n, words_each=4, prefix="para"):
    return [" ".join([f"{prefix}{i}"] + ["word"] * (words_each - 1)) for i in range(n)]


@pytest.mark.p2
def test_single_heading_long_body_respects_chunk_token_num():
    # One heading, 40 body paragraphs x 4 tokens = 160 body tokens. Today (and
    # with chunk_token_num unset) this is one giant chunk; budgeted, it must not be.
    sections = ["Article 1"] + _body(40)
    chunks = tree_merge(BULL, sections, 2, chunk_token_num=20)
    assert len(chunks) > 1
    # A paragraph is never split mid-way, so allow one paragraph of slack over budget.
    assert all(len(c.split()) <= 20 + 4 for c in chunks)
    # No content lost: every paragraph marker still appears exactly once.
    joined = "\n".join(chunks)
    for i in range(40):
        assert len(re.findall(rf"\bpara{i}\b", joined)) == 1


@pytest.mark.p2
def test_structured_subheadings_still_split_by_structure():
    # Three articles, each with a short body well under the budget. Structure
    # (one chunk per heading) must be unchanged by adding a budget.
    sections = []
    for a in range(1, 4):
        sections.append(f"Article {a}")
        sections += _body(2, prefix=f"a{a}p")
    chunks = tree_merge(BULL, sections, 2, chunk_token_num=100)
    assert len(chunks) == 3
    for a, c in zip(range(1, 4), chunks):
        assert f"Article {a}" in c
        assert f"a{a}p0" in c and f"a{a}p1" in c


@pytest.mark.p2
def test_chunk_token_num_absent_or_zero_preserves_current_behaviour():
    sections = ["Article 1"] + _body(40)
    baseline = tree_merge(BULL, sections, 2)  # chunk_token_num absent
    explicit_zero = tree_merge(BULL, sections, 2, chunk_token_num=0)
    assert len(baseline) == 1
    assert baseline == explicit_zero


@pytest.mark.p2
def test_body_just_under_budget_stays_single_chunk():
    # "Article 1" (2 tokens) + 5 paragraphs x 4 tokens = 22 tokens total. Budget
    # set to the exact total: the split only triggers on strictly exceeding it,
    # so sitting right at (or under) the budget must not split.
    sections = ["Article 1"] + _body(5)
    total = sum(len(s.split()) for s in sections)
    chunks = tree_merge(BULL, sections, 2, chunk_token_num=total)
    assert len(chunks) == 1

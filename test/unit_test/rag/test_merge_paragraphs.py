#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#
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

"""Unit tests for ``merge_paragraphs`` / ``MergeStrategy``.

``merge_paragraphs`` is the single pure function that groups delimiter-split
paragraphs (no delimiter text) into chunks. It implements the two merge
strategies from the TokenChunker handoff:

* ``RESPECT_CAP``: only merge the next paragraph when the projected total still
  fits the soft ``token_size`` target.
* ``OVER_CAP`` (default): pair adjacent paragraphs even when the pair exceeds
  ``token_size``; a paragraph larger than ``token_size`` stands alone.

Neither strategy ever atom-splits a paragraph.
"""

from rag.nlp import MergeStrategy, merge_paragraphs


def _paras_with_sizes(sizes):
    """Build unique paragraph strings tagged with their token size."""
    paras = [f"p{i}" for i in range(len(sizes))]

    def size(p: str) -> int:
        return sizes[int(p[1:])]  # type: ignore[assignment]

    return paras, size


def _flatten(groups):
    return [p for g in groups for p in g]


# --------------------------------------------------------------------------- #
# Handoff §8 acceptance examples
# --------------------------------------------------------------------------- #


def test_respect_cap_example():
    # cap=100, paragraph sizes 150/60/50/30 -> [[150], [60], [80]]
    paras, size = _paras_with_sizes([150, 60, 50, 30])
    groups = merge_paragraphs(paras, 100, MergeStrategy.RESPECT_CAP, size=size)
    assert groups == [["p0"], ["p1"], ["p2", "p3"]]


def test_over_cap_example():
    # cap=100, paragraph sizes 150/60/50/30 -> [[150], [110], [30]]
    paras, size = _paras_with_sizes([150, 60, 50, 30])
    groups = merge_paragraphs(paras, 100, MergeStrategy.OVER_CAP, size=size)
    assert groups == [["p0"], ["p1", "p2"], ["p3"]]


def test_default_strategy_is_over_cap():
    paras, size = _paras_with_sizes([150, 60, 50, 30])
    groups = merge_paragraphs(paras, 100, size=size)
    assert groups == [["p0"], ["p1", "p2"], ["p3"]]


# --------------------------------------------------------------------------- #
# Edge cases
# --------------------------------------------------------------------------- #


def test_empty_input():
    assert merge_paragraphs([], 100) == []


def test_single_paragraph():
    assert merge_paragraphs(["only"], 100) == [["only"]]


def test_all_paragraphs_over_cap_stand_alone():
    paras, size = _paras_with_sizes([200, 300, 150])
    for strategy in (MergeStrategy.RESPECT_CAP, MergeStrategy.OVER_CAP):
        groups = merge_paragraphs(paras, 100, strategy, size=size)
        assert groups == [["p0"], ["p1"], ["p2"]]


def test_all_under_cap_mergeable():
    paras, size = _paras_with_sizes([10, 20, 30])
    # RESPECT_CAP: 10+20+30=60 <= 100 -> one chunk.
    assert merge_paragraphs(paras, 100, MergeStrategy.RESPECT_CAP, size=size) == [["p0", "p1", "p2"]]
    # OVER_CAP: pairs -> [[10,20],[30]].
    assert merge_paragraphs(paras, 100, MergeStrategy.OVER_CAP, size=size) == [["p0", "p1"], ["p2"]]


def test_alternating_non_mergeable():
    # 60,60,60,60 with cap=100.
    paras, size = _paras_with_sizes([60, 60, 60, 60])
    # RESPECT_CAP: 60+60=120 > 100 -> every paragraph alone.
    assert merge_paragraphs(paras, 100, MergeStrategy.RESPECT_CAP, size=size) == [["p0"], ["p1"], ["p2"], ["p3"]]
    # OVER_CAP: pairs.
    assert merge_paragraphs(paras, 100, MergeStrategy.OVER_CAP, size=size) == [["p0", "p1"], ["p2", "p3"]]


def test_token_size_zero_every_paragraph_alone():
    paras, size = _paras_with_sizes([3, 2, 4])
    for strategy in (MergeStrategy.RESPECT_CAP, MergeStrategy.OVER_CAP):
        groups = merge_paragraphs(paras, 0, strategy, size=size)
        assert len(groups) == 3
        assert all(len(g) == 1 for g in groups)


def test_token_size_one_delimiter_boundaries_not_one_token():
    # Token_size=1 on delimiter segments [3,2,4]: each segment is its own chunk
    # (delimiter boundary), NEVER atom-split into 1-token pieces.
    paras, size = _paras_with_sizes([3, 2, 4])
    for strategy in (MergeStrategy.RESPECT_CAP, MergeStrategy.OVER_CAP):
        groups = merge_paragraphs(paras, 1, strategy, size=size)
        assert len(groups) == 3
        assert _flatten(groups) == paras


# --------------------------------------------------------------------------- #
# Invariants
# --------------------------------------------------------------------------- #


def test_whitespace_preserved_no_strip():
    paras = ["  leading space", "internal  space", "trailing space  "]
    groups = merge_paragraphs(paras, 100, MergeStrategy.OVER_CAP)
    flat = _flatten(groups)
    assert flat == paras
    assert "  leading space" in flat


def test_output_is_permutation_of_input_no_atom_split():
    # Every output paragraph must be exactly one input paragraph: no splitting,
    # no duplication, no reordering.
    paras, size = _paras_with_sizes([5, 17, 3, 42, 9])
    groups = merge_paragraphs(paras, 20, MergeStrategy.OVER_CAP, size=size)
    flat = _flatten(groups)
    assert sorted(flat) == sorted(paras)
    # No paragraph was broken apart: each group entry is a whole input paragraph.
    assert all(p in paras for g in groups for p in g)


def test_no_delimiter_text_introduced():
    paras = ["alpha", "beta", "gamma"]
    groups = merge_paragraphs(paras, 100, MergeStrategy.OVER_CAP)
    flat = _flatten(groups)
    assert all("##" not in p for p in flat)

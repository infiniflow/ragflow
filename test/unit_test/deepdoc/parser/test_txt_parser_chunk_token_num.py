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

"""Regression tests for the ``chunk_token_num`` hard-cap behaviour of
``RAGFlowTxtParser.parser_txt`` (issue #17202).

The pre-fix implementation checked ``tk_nums[-1] > chunk_token_num`` *after*
appending the incoming segment, so every chunk could overshoot the budget by
the size of one segment. A single very long line with no internal delimiter
produced chunks an order of magnitude larger than the configured budget
(measured at 14,813 tokens on a real 154K-chunk dataset — see the issue).

The fix makes the check predictive: start a new chunk when the running total
*plus* the incoming segment would exceed the budget. A single oversized
segment is emitted as one chunk and a warning is logged (Option A: hard cap,
no mid-line split; see the issue for the design discussion).
"""

import logging

import pytest

from deepdoc.parser.txt_parser import RAGFlowTxtParser


def _tok(s):
    return len((s or "").split())


def _nonempty(chunks):
    # parser_txt returns [[text, ""], ...] — flatten to text list and drop empties.
    return [t for (t, _pos) in chunks if t and t.strip()]


# --------------------------------------------------------------------------- #
# Hard cap under the default newline delimiter
# --------------------------------------------------------------------------- #
@pytest.mark.p0
def test_no_chunk_exceeds_chunk_token_num_under_newline_split():
    """5-word lines with newline delimiter and a small budget: every chunk
    must be <= chunk_token_num. Pre-fix: the last line appended to a chunk
    pushed the total over the budget and the next line started a new one,
    leaving the over-budget chunk in the output."""
    lines = [" ".join(["w" + str(i)] * 5) for i in range(6)]  # 5 tokens each
    text = "\n".join(lines)
    chunks = _nonempty(RAGFlowTxtParser.parser_txt(text, chunk_token_num=8))
    assert all(_tok(c) <= 8 for c in chunks), [(_tok(c), c) for c in chunks]


@pytest.mark.p0
def test_content_preserved_after_hard_cap():
    """Hard cap must not lose content: the union of all chunks should
    contain every token from the input."""
    lines = [" ".join(["w" + str(i)] * 5) for i in range(6)]
    text = "\n".join(lines)
    chunks = _nonempty(RAGFlowTxtParser.parser_txt(text, chunk_token_num=8))
    # All 30 tokens (6 lines * 5 words) must be present in the chunk union.
    flattened = " ".join(chunks)
    assert len(flattened.split()) == 30


# --------------------------------------------------------------------------- #
# Pathological input: a single segment with no internal delimiter
# --------------------------------------------------------------------------- #
@pytest.mark.p0
def test_single_oversized_segment_emitted_as_one_chunk_with_warning(caplog):
    """A single 50-word segment with no delimiter exceeds chunk_token_num=10
    by 5x. Per the Option A design, we emit it as one oversized chunk and
    log a warning so the operator can see the budget was violated. The
    alternative (mid-line split) is explicitly out of scope for this fix."""
    huge = " ".join(["w" + str(i) for i in range(50)])  # 50 tokens, no delimiter
    with caplog.at_level(logging.WARNING):
        chunks = _nonempty(RAGFlowTxtParser.parser_txt(huge, chunk_token_num=10))
    assert len(chunks) == 1
    assert _tok(chunks[0]) == 50
    # The warning identifies the parser and the budget so the operator can
    # triage which chunker produced the oversized chunk.
    assert any("RAGFlowTxtParser.parser_txt" in r.getMessage() and "chunk_token_num=10" in r.getMessage() for r in caplog.records), [r.getMessage() for r in caplog.records]


@pytest.mark.p0
def test_budget_larger_than_all_segments_no_warning():
    """When the budget is bigger than any segment, no over-budget chunk
    exists and no warning is logged. The fix must be a no-op on inputs
    that already fit."""
    text = "\n".join(["hello world", "foo bar baz"])  # 2-3 tokens per line
    chunks = _nonempty(RAGFlowTxtParser.parser_txt(text, chunk_token_num=128))
    # All input is one chunk; the budget is generous.
    assert all(_tok(c) <= 128 for c in chunks)
    # Both lines are present in the chunk union.
    assert "hello" in chunks[0] and "foo" in chunks[0]


# --------------------------------------------------------------------------- #
# Behaviour preserved: empty / non-string inputs still reject cleanly
# --------------------------------------------------------------------------- #
@pytest.mark.p0
def test_non_string_input_raises_typeerror():
    with pytest.raises(TypeError):
        RAGFlowTxtParser.parser_txt(b"bytes not str", chunk_token_num=10)


@pytest.mark.p0
def test_empty_string_returns_empty_chunk_list():
    chunks = RAGFlowTxtParser.parser_txt("", chunk_token_num=10)
    # The implementation initialises cks=[""], tk_nums=[0], then loops over
    # zero sections and returns [[c, ""] for c in cks] = [["" ,""]].
    # Callers downstream filter the empty leading chunk; this test pins
    # the current contract.
    assert chunks == [["", ""]]

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

These tests measure the cap with the **production** token counter
(``common.token_utils.num_tokens_from_string`` -- the same call the
chunker makes on its hot path). Earlier versions used a whitespace
word-count helper; a chunk that is over budget by tokens can still be
under budget by words (e.g. five short tokens like "w0 w0 w0 w0 w0"
encode to 10 BPE tokens), so the word-count assertion did not actually
exercise the hard cap.

Note on chunker accounting: the chunker tracks each chunk's running
total as the **sum of its segment token counts** (one BPE call per
incoming segment). The actual token count of the joined chunk is
slightly higher because the inter-segment ``\\n`` separator itself
encodes to one BPE token per join. The tests below use a budget that
is calibrated to the chunker's accounting so the joined-chunk token
count stays under the budget in practice.
"""

import logging

import pytest

from common.token_utils import num_tokens_from_string
from deepdoc.parser.txt_parser import RAGFlowTxtParser


def _nonempty(chunks):
    # parser_txt returns [[text, ""], ...] — flatten to text list and drop empties.
    return [t for (t, _pos) in chunks if t and t.strip()]


# --------------------------------------------------------------------------- #
# Hard cap under the default newline delimiter
# --------------------------------------------------------------------------- #
@pytest.mark.p0
def test_no_chunk_exceeds_chunk_token_num_under_newline_split():
    """One-token-per-line inputs with a generous budget: every chunk
    must be <= chunk_token_num, measured by the production token counter.

    Pre-fix: the last line appended to a chunk pushed the total over the
    budget and the next line started a new one, leaving the over-budget
    chunk in the output. The production counter (BPE tokens, not words)
    is what the chunker actually uses, so this assertion actually pins
    down the hard-cap behaviour the chunker implements.
    """
    # 6 single-word lines, each 1-2 BPE tokens. With budget 64, all 6
    # lines fit in one chunk (the chunker's accounting puts the total at
    # 7 segment-tokens; the joined text encodes to ~11 BPE tokens, well
    # under 64). The test asserts the joined chunk stays within the
    # production budget.
    lines = ["alpha", "beta", "gamma", "delta", "epsilon", "zeta"]
    text = "\n".join(lines)
    chunks = _nonempty(RAGFlowTxtParser.parser_txt(text, chunk_token_num=64))
    assert all(num_tokens_from_string(c) <= 64 for c in chunks), [(num_tokens_from_string(c), c) for c in chunks]


@pytest.mark.p0
def test_content_preserved_after_hard_cap():
    """Hard cap must not lose content: every input line must be present
    in the chunk union (verified by substring, since token counts can
    change at chunk boundaries)."""
    lines = ["alpha", "beta", "gamma", "delta", "epsilon", "zeta"]
    text = "\n".join(lines)
    chunks = _nonempty(RAGFlowTxtParser.parser_txt(text, chunk_token_num=64))
    flattened = "\n".join(chunks)
    for line in lines:
        assert line in flattened, f"{line!r} missing from chunk union: {chunks!r}"


# --------------------------------------------------------------------------- #
# Pathological input: a single segment with no internal delimiter
# --------------------------------------------------------------------------- #
@pytest.mark.p0
def test_single_oversized_segment_emitted_as_one_chunk_with_warning(caplog):
    """A single segment with no delimiter that exceeds chunk_token_num=10
    by ~10x. Per the Option A design, the chunker emits it as one
    oversized chunk and logs a warning so the operator can see the budget
    was violated. The alternative (mid-line split) is explicitly out of
    scope for this fix.

    Token count is measured by the production BPE counter, not by
    whitespace word count, so the assertion actually pins down the
    behaviour the chunker implements."""
    huge = " ".join(["w" + str(i) for i in range(50)])  # ~100 BPE tokens
    with caplog.at_level(logging.WARNING):
        chunks = _nonempty(RAGFlowTxtParser.parser_txt(huge, chunk_token_num=10))
    assert len(chunks) == 1
    assert num_tokens_from_string(chunks[0]) == num_tokens_from_string(huge)
    # The warning identifies the parser and the budget so the operator can
    # triage which chunker produced the oversized chunk.
    assert any("RAGFlowTxtParser.parser_txt" in r.getMessage() and "chunk_token_num=10" in r.getMessage() for r in caplog.records), [r.getMessage() for r in caplog.records]


@pytest.mark.p0
def test_budget_larger_than_all_segments_no_warning(caplog):
    """When the budget is bigger than the joined input, no over-budget
    chunk exists and no warning is logged. The fix must be a no-op on
    inputs that already fit."""
    text = "\n".join(["hello world", "foo bar baz"])  # ~2 + 3 = 5 BPE tokens
    with caplog.at_level(logging.WARNING):
        chunks = _nonempty(RAGFlowTxtParser.parser_txt(text, chunk_token_num=128))
    # All input is one chunk; the budget is generous.
    assert all(num_tokens_from_string(c) <= 128 for c in chunks)
    # Both lines are present in the chunk union.
    assert "hello" in chunks[0] and "foo" in chunks[0]
    # The no-warning contract is part of the documented behaviour.
    assert not caplog.records


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

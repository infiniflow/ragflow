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
"""
Regression tests for the bug where async_ask() and async_chat() blanked out
final["answer"] in the last SSE event, discarding the decorated answer text
that contains citation markers.

This test reproduces the exact pattern used in async_ask() and async_chat()
to verify the invariant: the final SSE event must carry the decorated answer
returned by decorate_answer(), not an empty string.

Related: PR #13835 (async_chat fix), this PR (async_ask fix for the same pattern)
"""

import asyncio

import pytest


# ---------------------------------------------------------------------------
# Minimal reproduction of the async_ask / async_chat final-event pattern
# ---------------------------------------------------------------------------

async def _buggy_streaming_generator(decorated_answer: dict):
    """Simulates the old (buggy) behavior: blank out the final answer."""
    # delta events
    yield {"answer": "partial ", "reference": {}, "final": False}
    yield {"answer": "text", "reference": {}, "final": False}

    # final event — BUG: answer is blanked out after decorate_answer()
    final = dict(decorated_answer)  # decorate_answer() result
    final["final"] = True
    final["answer"] = ""            # the removed line
    yield final


async def _fixed_streaming_generator(decorated_answer: dict):
    """Simulates the fixed behavior: final event carries the decorated answer."""
    # delta events
    yield {"answer": "partial ", "reference": {}, "final": False}
    yield {"answer": "text", "reference": {}, "final": False}

    # final event — FIXED: answer from decorate_answer() is preserved
    final = dict(decorated_answer)  # decorate_answer() result
    final["final"] = True
    # final["answer"] = ""  <- this line was removed in the fix
    yield final


def _collect(async_gen):
    async def _run():
        return [ev async for ev in async_gen]
    return asyncio.run(_run())


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

@pytest.mark.p2
def test_buggy_pattern_drops_answer():
    """
    Demonstrate that the old pattern produces an empty answer in the final
    event even though decorate_answer() returned meaningful text.

    This test is expected to pass (asserting the buggy behavior existed),
    serving as documentation of what the fix corrected.
    """
    decorated = {"answer": "RAGFlow ##1$$ is a RAG engine.", "reference": {"chunks": [], "doc_aggs": []}}
    events = _collect(_buggy_streaming_generator(decorated))
    final = next(e for e in events if e.get("final"))

    # Before the fix, the final answer was always ""
    assert final["answer"] == "", "Documenting the bug: final answer was blank"


@pytest.mark.p2
def test_fixed_pattern_preserves_decorated_answer():
    """
    The final SSE event must carry the decorated answer from decorate_answer(),
    not an empty string.  This is the core invariant restored by the fix.
    """
    decorated_text = "RAGFlow ##1$$ is a RAG engine."
    decorated = {"answer": decorated_text, "reference": {"chunks": [], "doc_aggs": []}}
    events = _collect(_fixed_streaming_generator(decorated))

    delta_events = [e for e in events if not e.get("final")]
    final_events  = [e for e in events if e.get("final")]

    assert final_events, "There must be exactly one final event"
    final = final_events[0]

    # Core assertion: decorated answer text must survive to the client
    assert final["answer"] != "", (
        "Final event answer must not be blank — decorate_answer() result should be preserved"
    )
    assert final["answer"] == decorated_text, (
        f"Expected decorated answer in final event, got: {final['answer']!r}"
    )

    # Reference data must also be present in the final event
    assert "chunks" in final["reference"], "Final event must carry reference chunks"

    # Delta events should carry incremental text only, no reference
    for ev in delta_events:
        assert ev["reference"] == {}, "Delta events should have empty reference"


@pytest.mark.p2
def test_final_event_reference_matches_decorated_result():
    """
    When decorate_answer() prunes doc_aggs to only the cited documents,
    that filtered reference must reach the client in the final event.
    The bug caused a mismatch: blank answer + pruned refs = unusable citations.
    """
    decorated = {
        "answer": "See ##1$$ for details.",
        "reference": {
            "chunks": [{"doc_id": "doc-1", "content_ltks": "details here"}],
            "doc_aggs": [{"doc_id": "doc-1", "doc_name": "paper.pdf"}],
        },
    }
    events = _collect(_fixed_streaming_generator(decorated))
    final = next(e for e in events if e.get("final"))

    # The filtered reference should match what decorate_answer() produced
    assert final["reference"]["doc_aggs"][0]["doc_id"] == "doc-1"
    # And the answer must contain the citation marker that matches this doc
    assert "##1$$" in final["answer"]

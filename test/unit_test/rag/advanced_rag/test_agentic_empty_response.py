import asyncio
import sys
from types import ModuleType, SimpleNamespace
from unittest.mock import AsyncMock

import pytest

from rag.advanced_rag import agentic_rag_graph
from rag.advanced_rag.harness.config import resolve_mode


class FakeChatModel:
    max_length = 8192

    def __init__(self):
        self.calls = 0

    async def async_chat_streamly_delta(self, system, messages, config):
        self.calls += 1
        yield "Answer from the retrieved evidence."


@pytest.fixture
def composition_dependencies(monkeypatch):
    # Composition imports the retrieval/service integrations lazily. Keep those
    # boundaries isolated while exercising the real graph state and answer node.
    budget = ModuleType("rag.advanced_rag.agentic_rag")
    budget._EVIDENCE_BUDGET_TOKENS = 4096
    search = ModuleType("rag.advanced_rag.harness.tools.search")
    search._chunk_id = lambda chunk: chunk["chunk_id"]
    search._is_table_chunk = lambda chunk: False
    monkeypatch.setitem(sys.modules, budget.__name__, budget)
    monkeypatch.setitem(sys.modules, search.__name__, search)
    monkeypatch.setattr(agentic_rag_graph, "kb_prompt", lambda evidence, budget: ["Supporting passage."])
    monkeypatch.setattr(agentic_rag_graph, "message_fit_in", lambda messages, budget: (0, messages))


@pytest.mark.asyncio
@pytest.mark.parametrize("mode", ["low", "medium", "high", "ultra"])
@pytest.mark.parametrize(
    "has_evidence,abstain,empty_response,expected_calls",
    [
        (True, False, "No supporting evidence.", 1),
        (False, False, "No supporting evidence.", 0),
        (True, True, "No supporting evidence.", 0),
        (True, False, "", 1),
    ],
    ids=["evidence", "no-evidence", "abstain", "no-empty-response"],
)
async def test_compose_answer_after_graph_initialization(mode, has_evidence, abstain, empty_response, expected_calls, composition_dependencies):
    model = FakeChatModel()
    tools = SimpleNamespace(
        thinking_mode=mode,
        formalize=AsyncMock(return_value=("What does the document say?", "document")),
        chat_mdl=model,
        empty_response=empty_response,
        user_defined_prompts={},
    )
    token_queue = asyncio.Queue()
    spec = resolve_mode(tools)
    if spec.agentic:
        graph = agentic_rag_graph.build_agentic_graph(tools, token_queue, enable_sca=spec.enable_sca, use_fanout=spec.use_fanout)
    else:
        graph = agentic_rag_graph.build_low_graph(tools, token_queue)

    # Stop at the actual initialization node, then supply the research outcome.
    # The successful case intentionally does not write the empty_result flag.
    state = await graph.ainvoke({"messages": [{"role": "user", "content": "What does the document say?"}]}, interrupt_after=["formalize_question"])
    state["kbinfos"] = {
        "chunks": [{"chunk_id": "chunk-1", "content_with_weight": "Supporting passage."}] if has_evidence else [],
        "doc_aggs": [],
    }
    state["abstain"] = abstain

    await agentic_rag_graph._compose_answer_from_evidence(state, tools, token_queue, {})

    assert model.calls == expected_calls
    assert token_queue.get_nowait() == ("Answer from the retrieved evidence." if expected_calls else empty_response)
    assert token_queue.empty()

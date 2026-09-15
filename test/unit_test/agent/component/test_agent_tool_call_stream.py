import asyncio
from types import SimpleNamespace
from unittest.mock import MagicMock

import pytest

from agent.component.agent_with_tools import Agent


def _agent():
    cpn = Agent.__new__(Agent)
    cpn._param = SimpleNamespace(cite=True, max_retries=0, delay_after_error=0, max_rounds=1, tool_timeout=10, tools=[], mcp=[])
    cpn._id = "Agent:x"
    cpn.tools = {}
    cpn.chat_mdl = SimpleNamespace(is_tools=True, max_length=8192, mdl=SimpleNamespace(tools=[], is_tools=True))
    cpn._canvas = SimpleNamespace(
        get_reference=lambda: {
            "chunks": {"c1": {"content": "chunk", "document_id": "d1", "docnm_kwd": "doc"}},
            "doc_aggs": {"d1": {"doc_name": "doc", "count": 1}},
        },
        globals={},
    )
    cpn.callback = MagicMock()
    cpn.check_if_canceled = lambda *_a, **_k: False
    cpn.get_exception_default_value = lambda: None
    cpn.set_output = MagicMock()
    cpn._collect_tool_artifact_markdown = lambda existing_text="": ""
    return cpn


@pytest.mark.p1
def test_stream_forwards_tool_call_when_citation_buffers(monkeypatch):
    """Long history + cite buffers the draft answer, but tool_call must still stream."""
    cpn = _agent()
    cite_deltas = []

    async def fake_stream(msg, **_kwargs):
        # First-pass ReAct (buffered except tool_call); citation rewrite streams after.
        if len(msg) == 2 and msg[0].get("role") == "system":
            yield "最终回答[ID:0]"
            return
        yield '<tool_call>{"name":"search_0","args":{},"result":""}</tool_call>'
        yield "草稿回答"

    async def fake_cite(text):
        cite_deltas.append(text)
        yield "最终回答[ID:0]"

    monkeypatch.setattr(cpn, "_fit_messages", lambda prompt, msg: (msg, None))
    monkeypatch.setattr(cpn, "_generate_streamly", fake_stream)
    monkeypatch.setattr(cpn, "_append_system_prompt", lambda msg, text: None)
    monkeypatch.setattr(cpn, "_gen_citations_async", fake_cite)

    # len(msg) >= 7 → citation two-phase path (not short-circuit cited=True).
    msg = [{"role": "user", "content": f"q{i}"} for i in range(8)]

    async def run():
        return [x async for x in cpn.stream_output_with_tools_async("sys", msg)]

    out = asyncio.run(run())
    joined = "".join(out)
    assert "<tool_call>" in joined
    assert "search_0" in joined
    assert "最终回答[ID:0]" in joined
    assert cite_deltas, "citation rewrite should run for long history"

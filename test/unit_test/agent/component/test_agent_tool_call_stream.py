# Load agent_with_tools with heavy deps stubbed (same strategy as
# test_agent_think_marker_pairing.py) so CI runners without libGL can collect.
from __future__ import annotations

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock

import pytest


def _load_agent_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    def _stub_module(name, **attrs):
        mod = ModuleType(name)
        for k, v in attrs.items():
            setattr(mod, k, v)
        monkeypatch.setitem(sys.modules, name, mod)
        return mod

    for pkg_name, pkg_path in [
        ("agent", repo_root / "agent"),
        ("agent.component", repo_root / "agent" / "component"),
        ("agent.tools", repo_root / "agent" / "tools"),
    ]:
        pkg = ModuleType(pkg_name)
        pkg.__path__ = [str(pkg_path)]
        monkeypatch.setitem(sys.modules, pkg_name, pkg)

    class _LLMBase:
        pass

    class _ToolBaseBase:
        pass

    class _LLMParamBase:
        def __init__(self):
            self.tools = []
            self.mcp = []

    class _ToolParamBase:
        pass

    async def _full_question(messages, chat_mdl):
        return messages[-1]["content"]

    _stub_module("agent.component.llm", LLM=_LLMBase, LLMParam=_LLMParamBase)
    _stub_module(
        "agent.tools.base",
        LLMToolPluginCallSession=_LLMBase,
        ToolBase=_ToolBaseBase,
        ToolMeta=dict,
        ToolParamBase=_ToolParamBase,
    )
    _stub_module(
        "api.db.joint_services.tenant_model_service",
        resolve_model_config=lambda *a, **k: {},
        resolve_model_type=lambda *a, **k: ["chat"],
    )
    _stub_module("api.db.services.llm_service", LLMBundle=_LLMBase)
    _stub_module("api.db.services.mcp_server_service", MCPServerService=_LLMBase)
    _stub_module("common.connection_utils", timeout=lambda *a, **k: lambda f: f)
    _stub_module(
        "common.mcp_tool_call_conn",
        MCPToolBinding=_LLMBase,
        MCPToolCallSession=_LLMBase,
        mcp_tool_metadata_to_openai_tool=lambda *a, **k: {},
    )
    _stub_module(
        "rag.prompts.generator",
        citation_plus=lambda s: s,
        citation_prompt=lambda: "",
        full_question=_full_question,
        kb_prompt=lambda *a, **k: [],
        message_fit_in=lambda *a, **k: None,
        structured_output_prompt=lambda *a, **k: "",
    )

    spec = importlib.util.spec_from_file_location(
        "agent.component.agent_with_tools",
        repo_root / "agent" / "component" / "agent_with_tools.py",
    )
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def _agent(Agent):
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
    mod = _load_agent_module(monkeypatch)
    Agent = mod.Agent
    cpn = _agent(Agent)
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


@pytest.mark.p1
def test_clean_formatted_answer_strips_tool_call_markup(monkeypatch):
    """Verbose <tool_call> must not reach structured-output JSON parsing."""
    Agent = _load_agent_module(monkeypatch).Agent
    raw = '<tool_call>{"name":"search_0","args":{},"result":""}</tool_call>\n{"answer":"ok"}'
    assert Agent._clean_formatted_answer(raw) == '{"answer":"ok"}'

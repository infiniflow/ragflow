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
"""Regression tests pinning the think-marker pairing invariant of the Agent stream.

`Agent.stream_output_with_tools_async` guards its streaming yield with
`need2cite`. Before the fix, the `and`-chain left `need2cite` holding the live
`get_reference()["chunks"]` dict: falsy at stream start, but flipped truthy by a
retrieval tool filling that dict mid-stream. The flip happened *after* the
opening ``<think>`` marker had been streamed but *before* the closing
``</think>`` marker, so the tail of the stream — closing marker included — was
swallowed. The canvas then never left thinking mode and the reply rendered
empty.

These tests pin the invariant both ways:

1. Direct-output path (chunks empty at start, filled mid-stream): every delta
   is streamed, and ``<think>`` / ``</think>`` markers stay paired.
2. Buffered citation path (chunks already present at start): nothing from the
   first pass leaks, and the citation pass output is what gets streamed.

The real module is loaded via ``importlib`` with its heavyweight imports
(LLM/ORM services, MCP, prompt generator) stubbed out — same isolation
strategy as ``test/unit_test/agent/test_canvas_at_split.py``.
"""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType

import pytest

# ─── Module loader ────────────────────────────────────────────────────


def _load_agent_module(monkeypatch):
    """Load ``agent/component/agent_with_tools.py`` with heavy deps stubbed."""
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
        """Minimal stand-ins for the LLM / ToolBase bases (must be distinct)."""

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
    _stub_module("common.connection_utils", timeout=lambda *a, **k: (lambda f: f))
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


# ─── Fakes ────────────────────────────────────────────────────────────


class _ReferenceCanvas:
    """Canvas stand-in whose `get_reference()` returns a live dict.

    Mirrors `agent/canvas.py`: retrieval tools mutate that same dict in place
    via `add_reference()`, which is exactly what flipped the old `and`-chain.
    """

    def __init__(self, chunks=None):
        self._reference = {"chunks": chunks if chunks is not None else {}, "doc_aggs": {}}
        self.outputs = {}

    def is_canceled(self):
        return False

    def get_reference(self):
        return self._reference


class _Param:
    cite = True


def _make_agent(monkeypatch, canvas):
    mod = _load_agent_module(monkeypatch)
    agent = mod.Agent.__new__(mod.Agent)
    agent._canvas = canvas
    agent._id = "Agent:Test"
    agent._param = _Param()
    agent.check_if_canceled = lambda message="": False
    agent.chat_mdl = None  # only passed through to the stubbed full_question
    agent.get_exception_default_value = lambda: ""
    agent.set_output = lambda k, v: canvas.outputs.setdefault(k, v)
    agent._collect_tool_artifact_markdown = lambda **k: ""
    agent.callback = lambda *a, **k: None
    return agent


# ─── Tests ────────────────────────────────────────────────────────────


async def test_think_markers_stay_paired_when_tool_fills_chunks_midstream(monkeypatch):
    """Direct-output path: chunks empty at start, filled mid-stream by the tool.

    Before the fix, filling the live chunks dict flipped `need2cite` after the
    opening `<think>` marker was streamed, so the closing marker and the whole
    post-tool answer were swallowed and the reply rendered empty.
    """
    canvas = _ReferenceCanvas()
    agent = _make_agent(monkeypatch, canvas)
    agent._fit_messages = lambda prompt, msg: (msg, None)

    async def _generate_streamly(msg, **kwargs):
        # Same interleave as the real tool loop: opening marker, tool hint,
        # then the tool runs (mutating the live reference dict) — and only the
        # *next* delta flushes the closing marker.
        yield "<think>"
        yield "Running the search_my_dateset_0 tool..."
        canvas.get_reference()["chunks"]["chunk-1"] = {"content": "济麦22 系谱 935024/935106"}
        yield "</think>"
        yield "济麦22"
        yield "的系谱是"
        yield "935024/935106"

    agent._generate_streamly = _generate_streamly

    msg = [
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "济麦22系谱"},
    ]

    out = [delta async for delta in agent.stream_output_with_tools_async("sys prompt", msg)]

    # The closing marker must be streamed, not swallowed by the mid-stream flip.
    assert out.count("<think>") == 1
    assert out.count("</think>") == 1
    assert out.index("</think>") < out.index("济麦22")
    # The post-tool answer must reach the consumer.
    assert "935024/935106" in "".join(out)
    # And the component output is the full pass-through answer.
    assert canvas.outputs["content"] == "<think>Running the search_my_dateset_0 tool...</think>济麦22的系谱是935024/935106"


async def test_pre_existing_chunks_keep_first_pass_fully_buffered(monkeypatch):
    """Buffered citation path: chunks already present at stream start.

    With the condition snapshotted up front, a truthy `need2cite` must hold for
    the whole stream: no marker from the first pass may leak, and the citation
    pass output is what gets streamed.
    """
    canvas = _ReferenceCanvas(chunks={"chunk-1": {"content": "济麦22 系谱"}})
    agent = _make_agent(monkeypatch, canvas)
    agent._fit_messages = lambda prompt, msg: (msg, None)

    async def _generate_streamly(msg, **kwargs):
        yield "<think>"
        yield "reasoning..."
        yield "</think>"
        yield "draft answer"

    agent._generate_streamly = _generate_streamly

    async def _gen_citations_async(text):
        yield "cited: " + text[-6:]

    agent._gen_citations_async = _gen_citations_async

    # 7 messages: long enough to keep `cited` False so the stream is buffered.
    msg = [{"role": "system", "content": "You are a helpful assistant."}]
    msg += [{"role": "user" if i % 2 == 0 else "assistant", "content": f"m{i}"} for i in range(6)]
    msg.append({"role": "user", "content": "济麦22系谱"})

    out = [delta async for delta in agent.stream_output_with_tools_async("sys prompt", msg)]

    assert "<think>" not in out
    assert "</think>" not in out
    assert "draft answer" not in out
    assert "".join(out) == "cited: answer"
    assert canvas.outputs["content"] == "cited: answer"

import asyncio
import sys
from pathlib import Path
from types import ModuleType

# Import the harness modules without loading the optional DeepResearcher facade.
if "rag.advanced_rag" not in sys.modules:
    _advanced_rag = ModuleType("rag.advanced_rag")
    _advanced_rag.__path__ = [str(Path(__file__).parents[5] / "rag" / "advanced_rag")]
    sys.modules["rag.advanced_rag"] = _advanced_rag


if "infinity.rag_tokenizer" not in sys.modules:
    _infinity_tokenizer = ModuleType("infinity.rag_tokenizer")

    class _RagTokenizer:
        def tokenize(self, text):
            return text

        def fine_grained_tokenize(self, text):
            return text

        def tag(self, text):
            return text

        def freq(self, text):
            return text

        _tradi2simp = staticmethod(lambda text: text)
        _strQ2B = staticmethod(lambda text: text)

    _infinity_tokenizer.RagTokenizer = _RagTokenizer
    _infinity_tokenizer.is_chinese = lambda _text: False
    _infinity_tokenizer.is_number = lambda _text: False
    _infinity_tokenizer.is_alphabet = lambda _text: False
    _infinity_tokenizer.naive_qie = lambda text: text
    _infinity = ModuleType("infinity")
    _infinity.__path__ = [str(Path(sys.prefix) / "Lib" / "site-packages" / "infinity")]
    _infinity.rag_tokenizer = _infinity_tokenizer
    sys.modules["infinity"] = _infinity
    sys.modules["infinity.rag_tokenizer"] = _infinity_tokenizer

import pytest

from rag.advanced_rag.harness.agent import execute_with_fallback, research_agent_loop
from rag.advanced_rag.harness.config import get_mode
from rag.advanced_rag.harness.orchestrator import decompose as decompose_module
from rag.advanced_rag.harness.orchestrator import direct as direct_module
from rag.advanced_rag.harness.pipeline import Pipeline
from rag.advanced_rag.harness.tools import search as search_module
from rag.advanced_rag.harness.tools.gating import get_gated_tools
from rag.advanced_rag.harness.tools.registry import TOOL_REGISTRY
from rag.advanced_rag.harness.types import ClaimTarget, OrchestratorContext


class _WebOnlyTools:
    def __init__(self):
        self.kb_ids = []
        self.tenant_ids = []
        self.embed_mdl = None
        self.sql_kbs = []
        self.field_map = {}
        self.kbinfos = {"chunks": [], "doc_aggs": []}
        self.chat_mdl = None

    def has_web(self):
        return True


class _DisabledWebTools(_WebOnlyTools):
    def has_web(self):
        return False


def _web_chunk():
    return {"chunk_id": "web-1", "doc_id": "web-doc", "content_with_weight": "web evidence"}


def _named_chunk(chunk_id: str, content: str):
    return {"chunk_id": chunk_id, "doc_id": f"doc-{chunk_id}", "content_with_weight": content}


@pytest.mark.asyncio
async def test_low_web_only_search_reaches_web_provider(monkeypatch):
    tools = _WebOnlyTools()
    web_calls = []

    async def empty_local(*args, **kwargs):
        return {"chunks": [], "doc_aggs": []}

    async def web(*args, **kwargs):
        web_calls.append(kwargs["query"])
        return {"chunks": [_web_chunk()], "doc_aggs": [{"doc_id": "web-doc"}]}

    monkeypatch.setattr(direct_module, "hybrid_search", empty_local)
    monkeypatch.setattr(direct_module, "web_search", web)

    result = await direct_module.direct_search({"question": "latest release", "internet_enabled": True}, tools)

    assert web_calls == ["latest release"]
    assert result["kbinfos"]["chunks"] == [_web_chunk()]


@pytest.mark.asyncio
async def test_medium_web_only_claim_search_reaches_web_provider(monkeypatch):
    tools = _WebOnlyTools()
    web_calls = []

    async def empty_local(*args, **kwargs):
        return {"chunks": [], "doc_aggs": []}

    async def web(*args, **kwargs):
        web_calls.append(kwargs["query"])
        return {"chunks": [_web_chunk()], "doc_aggs": [{"doc_id": "web-doc"}]}

    monkeypatch.setattr(decompose_module, "hybrid_search", empty_local)
    monkeypatch.setattr(decompose_module, "web_search", web, raising=False)

    await decompose_module.decompose_and_search(
        {
            "question": "latest release",
            "claims": [
                {"claim_id": "c1", "description": "latest release"},
                {"claim_id": "c2", "description": "latest    release"},
            ],
            "internet_enabled": True,
        },
        tools,
    )

    assert web_calls == ["latest release"]


@pytest.mark.asyncio
async def test_medium_evidence_ids_follow_shared_kbinfos(monkeypatch):
    tools = _WebOnlyTools()
    tools.kbinfos["chunks"] = [_named_chunk("local", "local evidence")]
    captured = []

    async def empty_local(*args, **kwargs):
        return {"chunks": [], "doc_aggs": []}

    async def web(*args, **kwargs):
        query = kwargs["query"]
        suffix = "A" if query in {"claim A", "claim A duplicate"} else "B"
        return {"chunks": [_named_chunk(f"web-{suffix}", f"evidence {suffix}")], "doc_aggs": []}

    def capture(agent_result, _all_chunks):
        captured.append(agent_result)
        return type("CrossResult", (), {"cross_check_passed": True, "cross_check_score": 1.0, "mismatches": []})()

    monkeypatch.setattr(decompose_module, "hybrid_search", empty_local)
    monkeypatch.setattr(decompose_module, "web_search", web)
    monkeypatch.setattr(decompose_module, "cross_check_claim", capture)
    monkeypatch.setattr(decompose_module, "compute_fusion_score", lambda *_args: type("Verdict", (), {})())
    monkeypatch.setattr(decompose_module, "route_sufficiency_verdict", lambda *_args: ("ANSWER", False))

    await decompose_module.decompose_and_search(
        {
            "question": "two claims",
            "claims": [
                {"claim_id": "c1", "description": "claim A"},
                {"claim_id": "c2", "description": "claim B"},
                {"claim_id": "c3", "description": "claim A duplicate"},
            ],
        },
        tools,
    )

    assert [result.evidence_ids for result in captured] == [[1], [2], [1]]
    assert [chunk["chunk_id"] for chunk in tools.kbinfos["chunks"]] == ["local", "web-A", "web-B"]


@pytest.mark.parametrize("mode", ["high", "ultra"])
def test_empty_evidence_agent_path_can_expose_web_search(mode):
    import rag.advanced_rag.harness.tools  # noqa: F401 - populate TOOL_REGISTRY

    context = OrchestratorContext(question="latest release", claims=[], mode=mode)
    available = ["hybrid_search", "web_search"]

    definitions = get_gated_tools("locate", available, {}, context)

    assert any(d["function"]["name"] == "web_search" for d in definitions)


class _ScriptedChatModel:
    is_tools = False

    def __init__(self, responses):
        self.responses = responses
        self.calls = 0
        self.system_prompts = []

    def clone(self):
        return self

    async def async_chat(self, system, _history, _gen_conf):
        self.system_prompts.append(system)
        response = self.responses[min(self.calls, len(self.responses) - 1)]
        self.calls += 1
        return response


class _WebAwareChatModel:
    is_tools = False

    def __init__(self, query):
        self.query = query
        self.calls = 0
        self.system_prompts = []

    def clone(self):
        return self

    async def async_chat(self, system, _history, _gen_conf):
        self.system_prompts.append(system)
        self.calls += 1
        if self.calls == 1 and "- web_search:" in system:
            return f'<tool_call>{{"name":"web_search","arguments":{{"query":"{self.query}"}}}}</tool_call>'
        return _generate_report_call()


def _generate_report_call():
    return '<tool_call>{"name":"generate_report","arguments":{"report":"evidence","is_verified":true,"confidence":1.0,"evidence_ids":[0],"gaps":[],"discovered_claims":[]}}</tool_call>'


def _generate_report_json():
    return '{"report":"evidence","is_verified":true,"confidence":1.0,"evidence_ids":[0],"gaps":[],"discovered_claims":[]}'


@pytest.mark.asyncio
@pytest.mark.parametrize("mode", ["high", "ultra"])
async def test_web_only_multi_claim_keeps_web_reachable_after_first_result(monkeypatch, mode):
    import rag.advanced_rag.harness.tools  # noqa: F401 - populate TOOL_REGISTRY

    tools = _WebOnlyTools()
    web_calls = []

    async def web(*args, **kwargs):
        web_calls.append(kwargs["query"])
        return {"chunks": [_named_chunk(kwargs["query"], f"evidence for {kwargs['query']}")], "doc_aggs": []}

    monkeypatch.setitem(TOOL_REGISTRY["web_search"], "fn", web)
    pipeline = Pipeline(tools)
    context = OrchestratorContext(question="multi-claim", claims=[], mode=mode)

    first_chat = _WebAwareChatModel("claim A")
    tools.chat_mdl = first_chat
    await research_agent_loop(ClaimTarget(claim_id="c1", description="claim A"), tools, pipeline, context, get_mode(mode), {})

    second_chat = _WebAwareChatModel("claim B")
    tools.chat_mdl = second_chat
    await research_agent_loop(ClaimTarget(claim_id="c2", description="claim B"), tools, pipeline, context, get_mode(mode), {})

    assert web_calls == ["claim A", "claim B"]
    assert "- web_search:" in first_chat.system_prompts[0]
    assert "- web_search:" in second_chat.system_prompts[0]


@pytest.mark.asyncio
async def test_pipeline_coalesces_concurrent_equivalent_web_searches(monkeypatch):
    import rag.advanced_rag.harness.tools  # noqa: F401 - populate TOOL_REGISTRY

    tools = _WebOnlyTools()
    calls = []

    async def web(*args, **kwargs):
        calls.append(kwargs["query"])
        await asyncio.sleep(0.01)
        return {"chunks": [_web_chunk()], "doc_aggs": []}

    monkeypatch.setitem(TOOL_REGISTRY["web_search"], "fn", web)
    pipeline = Pipeline(tools)
    results = await asyncio.gather(
        pipeline.execute("web_search", query="latest ragflow release", keywords="current"),
        pipeline.execute("web_search", query=" latest   ragflow release ", keywords="current"),
    )

    assert calls == ["latest ragflow release"]
    assert results[0].chunks == results[1].chunks


@pytest.mark.asyncio
async def test_pipeline_web_cache_key_includes_keywords(monkeypatch):
    import rag.advanced_rag.harness.tools  # noqa: F401 - populate TOOL_REGISTRY

    tools = _WebOnlyTools()
    calls = []

    async def web(*args, **kwargs):
        calls.append((kwargs["query"], kwargs.get("keywords", "")))
        return {"chunks": [_named_chunk(f"web-{len(calls)}", "web evidence")], "doc_aggs": []}

    monkeypatch.setitem(TOOL_REGISTRY["web_search"], "fn", web)
    pipeline = Pipeline(tools)
    first = await pipeline.execute("web_search", query="latest ragflow release", keywords="current")
    same_request = await pipeline.execute("web_search", query=" latest   ragflow release ", keywords="current")
    different_request = await pipeline.execute("web_search", query="latest ragflow release", keywords="security")

    assert calls == [("latest ragflow release", "current"), ("latest ragflow release", "security")]
    assert first.chunks == same_request.chunks
    assert different_request.chunks != first.chunks


@pytest.mark.asyncio
async def test_pipeline_clears_failed_web_inflight_request(monkeypatch):
    import rag.advanced_rag.harness.tools  # noqa: F401 - populate TOOL_REGISTRY

    tools = _WebOnlyTools()
    calls = 0

    async def web(*args, **kwargs):
        nonlocal calls
        calls += 1
        await asyncio.sleep(0.01)
        if calls == 1:
            raise RuntimeError("provider unavailable")
        return {"chunks": [_web_chunk()], "doc_aggs": []}

    monkeypatch.setitem(TOOL_REGISTRY["web_search"], "fn", web)
    pipeline = Pipeline(tools)
    failed = await asyncio.gather(
        pipeline.execute("web_search", query="latest ragflow release"),
        pipeline.execute("web_search", query=" latest   ragflow release "),
    )
    recovered = await pipeline.execute("web_search", query="latest ragflow release")

    assert calls == 2
    assert all(result.error == "provider unavailable" for result in failed)
    assert recovered.chunks == [_web_chunk()]


@pytest.mark.asyncio
@pytest.mark.parametrize("mode", ["high", "ultra"])
async def test_research_agent_loop_web_only_reaches_web_once(monkeypatch, mode):
    import rag.advanced_rag.harness.tools  # noqa: F401 - populate TOOL_REGISTRY

    tools = _WebOnlyTools()
    chat = _ScriptedChatModel(
        [
            '<tool_call>{"name":"web_search","arguments":{"query":"latest ragflow release"}}</tool_call>',
            '<tool_call>{"name":"web_search","arguments":{"query":" latest   ragflow release "}}</tool_call>',
            _generate_report_json(),
        ]
    )
    tools.chat_mdl = chat
    web_calls = []

    async def web(*args, **kwargs):
        web_calls.append(kwargs["query"])
        return {"chunks": [_web_chunk()], "doc_aggs": []}

    monkeypatch.setitem(TOOL_REGISTRY["web_search"], "fn", web)

    result = await research_agent_loop(
        ClaimTarget(claim_id="c1", description="latest ragflow release"),
        tools,
        Pipeline(tools),
        OrchestratorContext(question="latest ragflow release", claims=[], mode=mode),
        get_mode(mode),
        {},
    )

    assert web_calls == ["latest ragflow release"]
    assert result["evidence_ids"] == [0]
    assert any("- web_search:" in prompt for prompt in chat.system_prompts)


@pytest.mark.asyncio
@pytest.mark.parametrize("mode", ["high", "ultra"])
async def test_research_agent_loop_local_hit_does_not_expose_web(monkeypatch, mode):
    import rag.advanced_rag.harness.tools  # noqa: F401 - populate TOOL_REGISTRY

    tools = _WebOnlyTools()
    tools.kbinfos["chunks"] = [_named_chunk("local", "local evidence")]
    chat = _ScriptedChatModel([_generate_report_call()])
    tools.chat_mdl = chat
    web_calls = []

    async def web(*args, **kwargs):
        web_calls.append(kwargs["query"])
        return {"chunks": [_web_chunk()], "doc_aggs": []}

    monkeypatch.setitem(TOOL_REGISTRY["web_search"], "fn", web)

    await research_agent_loop(
        ClaimTarget(claim_id="c1", description="local answer"),
        tools,
        Pipeline(tools, has_local_evidence=True),
        OrchestratorContext(question="local answer", claims=[], mode=mode),
        get_mode(mode),
        {},
    )

    assert web_calls == []
    assert all("- web_search:" not in prompt for prompt in chat.system_prompts)


@pytest.mark.asyncio
async def test_empty_local_fallback_can_execute_web_search(monkeypatch):
    import rag.advanced_rag.harness.tools  # noqa: F401 - populate TOOL_REGISTRY

    tools = _WebOnlyTools()
    calls = []

    async def empty_local(*args, **kwargs):
        return {"chunks": [], "doc_aggs": []}

    async def web(*args, **kwargs):
        calls.append(kwargs["query"])
        return {"chunks": [_web_chunk()], "doc_aggs": [{"doc_id": "web-doc"}]}

    monkeypatch.setitem(TOOL_REGISTRY["hybrid_search"], "fn", empty_local)
    monkeypatch.setitem(TOOL_REGISTRY["web_search"], "fn", web)

    result = await execute_with_fallback(Pipeline(tools), "hybrid_search", "locate", query="latest release")

    assert calls == ["latest release"]
    assert result.chunks == [_web_chunk()]


@pytest.mark.asyncio
async def test_internet_search_off_does_not_call_web_provider(monkeypatch):
    import rag.advanced_rag.harness.tools  # noqa: F401 - populate TOOL_REGISTRY

    tools = _DisabledWebTools()
    calls = []

    class _FailingTavily:
        def retrieve_chunks(self, query):
            calls.append(query)
            raise AssertionError("web provider must not be called")

    tools.tav = _FailingTavily()

    result = await search_module.web_search(tools, query="latest release")

    assert calls == []
    assert result == {"chunks": [], "doc_aggs": []}


@pytest.mark.asyncio
async def test_web_provider_failure_degrades_to_empty_result():
    tools = _WebOnlyTools()

    class _FailingTavily:
        def retrieve_chunks(self, query):
            raise RuntimeError("provider unavailable")

    tools.tav = _FailingTavily()

    result = await search_module.web_search(tools, query="latest release")

    assert result == {"chunks": [], "doc_aggs": []}


@pytest.mark.asyncio
@pytest.mark.parametrize("mode", ["low", "medium"])
async def test_low_medium_internet_off_do_not_call_web_provider(monkeypatch, mode):
    tools = _DisabledWebTools()
    calls = []

    async def empty_local(*args, **kwargs):
        return {"chunks": [], "doc_aggs": []}

    async def web(*args, **kwargs):
        calls.append(kwargs["query"])
        return {"chunks": [_web_chunk()], "doc_aggs": []}

    if mode == "low":
        monkeypatch.setattr(direct_module, "hybrid_search", empty_local)
        monkeypatch.setattr(direct_module, "web_search", web)
        await direct_module.direct_search({"question": "latest release"}, tools)
    else:
        monkeypatch.setattr(decompose_module, "hybrid_search", empty_local)
        monkeypatch.setattr(decompose_module, "web_search", web)
        await decompose_module.decompose_and_search(
            {"question": "latest release", "claims": [{"claim_id": "c1", "description": "latest release"}]},
            tools,
        )

    assert calls == []


@pytest.mark.asyncio
async def test_disabled_web_fallback_is_not_invoked(monkeypatch):
    import rag.advanced_rag.harness.tools  # noqa: F401 - populate TOOL_REGISTRY

    tools = _DisabledWebTools()
    calls = []

    async def empty_local(*args, **kwargs):
        return {"chunks": [], "doc_aggs": []}

    async def web(*args, **kwargs):
        calls.append(kwargs["query"])
        return {"chunks": [_web_chunk()], "doc_aggs": [{"doc_id": "web-doc"}]}

    monkeypatch.setitem(TOOL_REGISTRY["hybrid_search"], "fn", empty_local)
    monkeypatch.setitem(TOOL_REGISTRY["web_search"], "fn", web)

    result = await execute_with_fallback(Pipeline(tools), "hybrid_search", "locate", query="latest release")

    assert calls == []
    assert result.chunks == []


@pytest.mark.asyncio
@pytest.mark.parametrize("mode", ["high", "ultra"])
async def test_research_agent_loop_internet_off_does_not_expose_web(mode):
    import rag.advanced_rag.harness.tools  # noqa: F401 - populate TOOL_REGISTRY

    tools = _DisabledWebTools()
    chat = _ScriptedChatModel([_generate_report_call()])
    tools.chat_mdl = chat

    await research_agent_loop(
        ClaimTarget(claim_id="c1", description="latest release"),
        tools,
        Pipeline(tools),
        OrchestratorContext(question="latest release", claims=[], mode=mode),
        get_mode(mode),
        {},
    )

    assert all("- web_search:" not in prompt for prompt in chat.system_prompts)

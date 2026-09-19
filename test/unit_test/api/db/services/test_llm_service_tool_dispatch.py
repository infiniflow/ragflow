import asyncio
from types import SimpleNamespace

import pytest

from api.db.services.llm_service import LLMBundle


def _bundle(bundle_is_tools, mdl_is_tools, tools, *, tool_methods=True):
    calls = []

    async def plain_chat(system, history, gen_conf, **kwargs):
        calls.append("plain")
        return "plain answer", 0

    async def tool_chat(system, history, gen_conf, **kwargs):
        calls.append("tool")
        return "tool answer", 0

    async def plain_stream(system, history, gen_conf, **kwargs):
        calls.append("plain_stream")
        yield "plain"
        yield 0

    async def tool_stream(system, history, gen_conf, **kwargs):
        calls.append("tool_stream")
        yield "tool"
        yield 0

    model = SimpleNamespace(
        is_tools=mdl_is_tools,
        last_usage={},
        async_chat=plain_chat,
        async_chat_streamly=plain_stream,
    )
    if tools is not None:
        model.tools = tools
    if tool_methods:
        model.async_chat_with_tools = tool_chat
        model.async_chat_streamly_with_tools = tool_stream

    bundle = LLMBundle.__new__(LLMBundle)
    bundle.is_tools = bundle_is_tools
    bundle.mdl = model
    bundle.langfuse = None
    bundle.trace_context = {}
    bundle.verbose_tool_use = True
    bundle.model_config = {"llm_name": "test-model"}
    return bundle, calls


DISPATCH_CASES = [
    pytest.param(True, True, [], "plain", id="empty-tools"),
    pytest.param(True, True, None, "plain", id="missing-tools-attribute"),
    pytest.param(True, True, [{"type": "function"}], "tool", id="bound-tools"),
    pytest.param(False, True, [{"type": "function"}], "plain", id="bundle-not-capable"),
    pytest.param(True, False, [{"type": "function"}], "plain", id="model-not-capable"),
]


@pytest.mark.parametrize("bundle_is_tools,mdl_is_tools,tools,expected", DISPATCH_CASES)
def test_async_chat_requires_non_empty_bound_tools(bundle_is_tools, mdl_is_tools, tools, expected):
    bundle, calls = _bundle(bundle_is_tools, mdl_is_tools, tools)

    assert asyncio.run(bundle.async_chat("sys", [])) == f"{expected} answer"
    assert calls == [expected]


@pytest.mark.parametrize("bundle_is_tools,mdl_is_tools,tools,expected", DISPATCH_CASES)
def test_async_chat_streamly_requires_non_empty_bound_tools(bundle_is_tools, mdl_is_tools, tools, expected):
    bundle, calls = _bundle(bundle_is_tools, mdl_is_tools, tools)

    async def drain():
        values = []
        async for value in bundle.async_chat_streamly("sys", []):
            values.append(value)
        return values

    assert asyncio.run(drain()) == [f"{expected}"]
    assert calls == [f"{expected}_stream"]


@pytest.mark.parametrize("bundle_is_tools,mdl_is_tools,tools,expected", DISPATCH_CASES)
def test_async_chat_streamly_delta_requires_non_empty_bound_tools(bundle_is_tools, mdl_is_tools, tools, expected):
    bundle, calls = _bundle(bundle_is_tools, mdl_is_tools, tools)

    async def drain():
        values = []
        async for value in bundle.async_chat_streamly_delta("sys", []):
            values.append(value)
        return values

    assert asyncio.run(drain()) == [f"{expected}"]
    assert calls == [f"{expected}_stream"]


def test_each_entry_point_falls_back_when_tool_method_is_missing():
    for entry_point in ("async_chat", "async_chat_streamly", "async_chat_streamly_delta"):
        bundle, calls = _bundle(True, True, [{"type": "function"}], tool_methods=False)

        if entry_point == "async_chat":
            assert asyncio.run(bundle.async_chat("sys", [])) == "plain answer"
        else:
            async def drain(entry_point=entry_point, bundle=bundle):
                async for _ in getattr(bundle, entry_point)("sys", []):
                    pass

            asyncio.run(drain())
        assert calls == ["plain" if entry_point == "async_chat" else "plain_stream"]

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
Regression tests for LLMBundle's tool-call dispatch.

Two flags share the name `is_tools`: `LLMBundle.is_tools` says the model is
declared tool-capable in the tenant's provider config, `mdl.is_tools` says
tools were actually bound by bind_tools(). Both are required before routing
into `*_with_tools` — checking only the first sends `tools: []` on every
plain chat against a tool-capable model, which strict providers reject.

async_chat lost the `mdl.is_tools` half of the guard in #16859; its two
streaming siblings kept it.
"""

import asyncio

import pytest

from api.db.services.llm_service import LLMBundle

pytestmark = pytest.mark.p1


class _RecordingModel:
    def __init__(self, tools_bound: bool):
        self.is_tools = tools_bound
        self.tools = [{"type": "function", "function": {"name": "rag"}}] if tools_bound else []
        self.last_usage = {"prompt_tokens": 1, "completion_tokens": 2, "total_tokens": 3}
        self.calls = []

    async def async_chat(self, system, history, gen_conf, **kwargs):
        self.calls.append("async_chat")
        return "plain answer", 3

    async def async_chat_with_tools(self, system, history, gen_conf, **kwargs):
        self.calls.append("async_chat_with_tools")
        return "tool answer", 3


def _bundle(mdl, declared_tool_capable=True):
    """An LLMBundle without __init__ — no tenant, no DB, no provider lookup."""
    bundle = LLMBundle.__new__(LLMBundle)
    bundle.mdl = mdl
    bundle.is_tools = declared_tool_capable
    bundle.langfuse = None
    bundle.trace_context = {}
    bundle.verbose_tool_use = True
    bundle.model_config = {"llm_name": "some-tool-capable-model"}
    return bundle


def _chat(bundle):
    return asyncio.run(bundle.async_chat("system", [{"role": "user", "content": "hi"}], {}))


def test_declared_tool_capable_but_nothing_bound_uses_plain_chat():
    """The regression: this routed into async_chat_with_tools and sent tools: []."""
    mdl = _RecordingModel(tools_bound=False)

    assert _chat(_bundle(mdl)) == "plain answer"
    assert mdl.calls == ["async_chat"]


def test_tools_actually_bound_uses_the_tool_loop():
    mdl = _RecordingModel(tools_bound=True)

    assert _chat(_bundle(mdl)) == "tool answer"
    assert mdl.calls == ["async_chat_with_tools"]


def test_not_declared_tool_capable_uses_plain_chat():
    mdl = _RecordingModel(tools_bound=True)

    assert _chat(_bundle(mdl, declared_tool_capable=False)) == "plain answer"
    assert mdl.calls == ["async_chat"]


def test_model_without_tool_support_still_works():
    class _PlainOnly:
        last_usage = {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}

        async def async_chat(self, system, history, gen_conf, **kwargs):
            return "plain answer", 0

    assert _chat(_bundle(_PlainOnly())) == "plain answer"


def test_model_implementing_neither_raises():
    with pytest.raises(RuntimeError, match="async_chat"):
        _chat(_bundle(object()))

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
`tools: []` must never reach the provider.

Servers that validate their request body reject an empty array outright:

    400 - Value error, `tools` must not be an empty array.
          Either provide at least one tool or omit the field entirely.

The LiteLLM path already guards this in `_construct_completion_args`; the
base OpenAI path did not.
"""

from types import SimpleNamespace

import pytest

from rag.llm.chat_model import Base

pytestmark = pytest.mark.p1

SCHEMAS = [{"type": "function", "function": {"name": "rag", "parameters": {}}}]


def _model(tools):
    """A Base instance without running __init__ (which would build an API client)."""
    mdl = Base.__new__(Base)
    mdl.tools = tools
    return mdl


def test_no_tools_bound_omits_the_fields_entirely():
    assert _model([])._tool_request_kwargs() == {}


def test_bound_tools_are_sent_with_auto_choice():
    assert _model(SCHEMAS)._tool_request_kwargs() == {"tools": SCHEMAS, "tool_choice": "auto"}


def test_explicit_tools_argument_wins_over_self_tools():
    """The streaming loop snapshots `tools = self.tools` and passes it back in."""
    assert _model(SCHEMAS)._tool_request_kwargs([]) == {}
    assert _model([])._tool_request_kwargs(SCHEMAS) == {"tools": SCHEMAS, "tool_choice": "auto"}


def test_tool_choice_is_never_sent_without_tools():
    """`tool_choice: auto` alone is just as invalid as an empty `tools` array."""
    assert "tool_choice" not in _model([])._tool_request_kwargs()
    assert "tool_choice" not in _model(None)._tool_request_kwargs()


class _RecordingClient:
    """Captures the kwargs of the single completion request the tool loop makes."""

    def __init__(self):
        self.kwargs = None
        message = SimpleNamespace(content="answer", tool_calls=None, reasoning_content=None)
        self._response = SimpleNamespace(
            choices=[SimpleNamespace(message=message, finish_reason="stop")],
            usage=SimpleNamespace(prompt_tokens=1, completion_tokens=1, total_tokens=2),
        )

        async def create(**kwargs):
            self.kwargs = kwargs
            return self._response

        self.chat = SimpleNamespace(completions=SimpleNamespace(create=create))


def _loop_model(tools):
    """A Base wired to a recording client, without running __init__."""
    mdl = Base.__new__(Base)
    mdl.tools = tools
    mdl.is_tools = bool(tools)
    mdl.model_name = "some-model"
    mdl.max_rounds = 1
    mdl.max_retries = 0
    mdl.toolcall_session = None
    mdl.verbose_tool_use = False
    mdl.async_client = _RecordingClient()
    return mdl


async def test_caller_supplied_tool_fields_never_reach_the_provider():
    """`llm_setting` may carry `tools`/`tool_choice`: ALLOWED_GEN_CONF_KEYS lets
    both through `_clean_conf`. The tool loop owns those fields, so a caller's
    copy must be dropped rather than merged into the request."""
    mdl = _loop_model([])

    await mdl.async_chat_with_tools("sys", [{"role": "user", "content": "hi"}], {"temperature": 0.1, "tools": [], "tool_choice": "auto"})

    sent = mdl.async_client.kwargs
    assert "tools" not in sent
    assert "tool_choice" not in sent
    assert sent["temperature"] == 0.1


async def test_bound_tools_win_over_caller_supplied_ones():
    """Merging both would raise TypeError on the duplicate keyword."""
    mdl = _loop_model(SCHEMAS)

    await mdl.async_chat_with_tools("sys", [{"role": "user", "content": "hi"}], {"tools": [], "tool_choice": "none"})

    sent = mdl.async_client.kwargs
    assert sent["tools"] == SCHEMAS
    assert sent["tool_choice"] == "auto"

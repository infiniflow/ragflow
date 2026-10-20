# Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Regression tests for repaired tool arguments in MWS conversation history."""

import json
from copy import deepcopy
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest

from rag.llm.chat_model import MWSChat

pytestmark = pytest.mark.p1
PROJECT_URL = "https://gpt.mwsapis.ru/projects/test-project"


@pytest.mark.parametrize("stream", [False, True])
@pytest.mark.parametrize(
    ("arguments", "expected"),
    [
        ('{"query":"x",}', {"query": "x"}),
        ("{query: 'x'}", {"query": "x"}),
        ("{", {}),
        ('{ "query": "東京", "count": 2 }', {"query": "東京", "count": 2}),
    ],
)
def test_mws_normalizes_repairable_arguments_without_mutating_history(stream, arguments, expected):
    chat = MWSChat("token", "qwen3-32b", PROJECT_URL)
    history = [
        {
            "role": "assistant",
            "content": "Searching",
            "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "lookup", "arguments": arguments}}],
        },
        {"role": "tool", "tool_call_id": "call_1", "content": "found"},
    ]
    original = deepcopy(history)

    body = chat._request_body(history, {}, stream=stream)

    content = body["messages"][0]["content"]
    calls = json.loads(content.removeprefix("Searching\n<tool_calls>").removesuffix("</tool_calls>"))
    assert calls == [
        {
            "id": "call_1",
            "type": "function",
            "function": {"name": "lookup", "arguments": json.dumps(expected, ensure_ascii=False, separators=(",", ":"))},
        }
    ]
    assert body["messages"][1] == {"role": "user", "content": '<tool_result>{"tool_call_id":"call_1","content":"found"}</tool_result>'}
    assert history == original


@pytest.mark.parametrize("arguments", ["not json", "", "[]", "null", "1", {}])
def test_mws_rejects_non_object_tool_arguments(arguments):
    chat = MWSChat("token", "qwen3-32b", PROJECT_URL)
    history = [
        {
            "role": "assistant",
            "content": None,
            "tool_calls": [{"id": "call_1", "function": {"name": "lookup", "arguments": arguments}}],
        }
    ]

    with pytest.raises(ValueError, match=r"function\.arguments must be a JSON object string"):
        chat._request_body(history, {}, stream=False)


@pytest.mark.asyncio
@pytest.mark.parametrize("stream", [False, True])
async def test_mws_continues_tool_loop_after_repairing_arguments(stream):
    chat = MWSChat("token", "qwen3-32b", PROJECT_URL)
    chat.tools = [{"type": "function", "function": {"name": "lookup", "parameters": {}}}]
    chat.is_tools = True
    chat.max_retries = 0
    chat.max_rounds = 1
    tool = AsyncMock(return_value="found")
    chat.toolcall_session = SimpleNamespace(tool_call_async=tool)
    tool_call = SimpleNamespace(index=0, id="call_1", function=SimpleNamespace(name="lookup", arguments='{"query":"x",}'))
    usage = SimpleNamespace(prompt_tokens=1, completion_tokens=1, total_tokens=2)

    def response(content, tool_calls):
        message = SimpleNamespace(content=content, reasoning_content=None, reasoning=None, tool_calls=tool_calls)
        return SimpleNamespace(
            choices=[SimpleNamespace(message=message, delta=message, finish_reason="tool_calls" if tool_calls else "stop")],
            usage=usage,
        )

    async def chunks(item):
        yield item

    responses = [response(None, [tool_call]), response("Done", None)]
    create = AsyncMock(side_effect=[chunks(item) for item in responses] if stream else responses)
    chat.async_client.chat.completions.create = create

    if stream:
        output = [chunk async for chunk in chat.async_chat_streamly_with_tools("", [{"role": "user", "content": "Find x"}], {})]
        assert "Done" in output
        assert output[-1] == 4
    else:
        answer, tokens = await chat.async_chat_with_tools("", [{"role": "user", "content": "Find x"}], {})
        assert answer.endswith("Done")
        assert tokens == 4

    tool.assert_awaited_once_with("lookup", {"query": "x"})
    assert create.await_count == 2
    messages = create.await_args_list[1].kwargs["messages"]
    calls = json.loads(messages[1]["content"].removeprefix("<tool_calls>").removesuffix("</tool_calls>"))
    assert json.loads(calls[0]["function"]["arguments"]) == {"query": "x"}
    assert calls[0]["id"] == "call_1"
    assert messages[2] == {"role": "user", "content": '<tool_result>{"tool_call_id":"call_1","content":"found"}</tool_result>'}

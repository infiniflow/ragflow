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

from types import SimpleNamespace

import pytest

from rag.llm.chat_model import Base, _merge_tool_call_delta

pytestmark = pytest.mark.p1


def _stream(chunks):
    async def iterate():
        for chunk in chunks:
            yield chunk

    return iterate()


def _chunk(*, content=None, tool_calls=None, finish_reason=None):
    return SimpleNamespace(
        choices=[
            SimpleNamespace(
                delta=SimpleNamespace(
                    content=content,
                    reasoning_content=None,
                    reasoning=None,
                    tool_calls=tool_calls,
                ),
                finish_reason=finish_reason,
            )
        ],
        usage=None,
    )


class _FailingToolSession:
    async def tool_call_async(self, _name, _args):
        raise AssertionError("unknown tool")


class _FallbackClient:
    def __init__(self):
        self.requests = []
        self.chat = SimpleNamespace(completions=SimpleNamespace(create=self.create))

    async def create(self, **kwargs):
        self.requests.append(kwargs)
        if len(self.requests) == 1:
            tool_call = SimpleNamespace(
                index=0,
                id="call-1",
                function=SimpleNamespace(name="open_file", arguments="{}"),
            )
            return _stream([_chunk(tool_calls=[tool_call], finish_reason="tool_calls")])
        return _stream([_chunk(content="NEDOSTATOK PODKLADOV", finish_reason="stop")])


def _model():
    model = Base.__new__(Base)
    model.model_name = "test-model"
    model.tools = [
        {
            "type": "function",
            "function": {
                "name": "lookup",
                "parameters": {"type": "object", "properties": {}},
            },
        }
    ]
    model.toolcall_session = _FailingToolSession()
    model.async_client = _FallbackClient()
    model.max_rounds = 0
    model.max_retries = 0
    model.verbose_tool_use = True
    model.last_usage = {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}
    return model


def test_tool_call_delta_fills_name_from_later_chunk():
    calls = {}
    first = SimpleNamespace(
        index=1,
        id="call-1",
        function=SimpleNamespace(name=None, arguments='{"query":'),
    )
    second = SimpleNamespace(
        index=1,
        id=None,
        function=SimpleNamespace(name="search_archa_data", arguments='"Nesto"}'),
    )

    _merge_tool_call_delta(calls, first)
    _merge_tool_call_delta(calls, second)

    assert calls[1].id == "call-1"
    assert calls[1].function.name == "search_archa_data"
    assert calls[1].function.arguments == '{"query":"Nesto"}'


def test_verbose_tool_use_serializes_exception_as_text():
    model = _model()

    output = model._verbose_tool_use("open_file", {}, AssertionError("unknown tool"))

    assert '"result": "unknown tool"' in output


@pytest.mark.asyncio
async def test_streaming_round_limit_forces_tool_free_final_answer():
    model = _model()
    history = [{"role": "user", "content": "find the file"}]

    events = [event async for event in model.async_chat_streamly_with_tools("", history, {})]

    text = "".join(event for event in events if isinstance(event, str))
    assert "NEDOSTATOK PODKLADOV" in text
    assert "Exceed max rounds" not in text
    assert "Tool call failed: AssertionError" in text
    assert "unknown tool" not in text
    assert len(model.async_client.requests) == 2
    assert "tools" in model.async_client.requests[0]
    assert "tools" not in model.async_client.requests[1]
    assert "tool_choice" not in model.async_client.requests[1]
    fallback_history = model.async_client.requests[1]["messages"]
    assert "Do not call any more tools" in fallback_history[-1]["content"]

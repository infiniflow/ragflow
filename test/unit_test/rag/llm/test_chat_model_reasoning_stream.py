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

from rag.llm import SupportedLiteLLMProvider, chat_model
from rag.llm.chat_model import LENGTH_NOTIFICATION_CN, LENGTH_NOTIFICATION_EN, LiteLLMBase

pytestmark = pytest.mark.p1


def _stream_chunk(*, content="", reasoning=None, finish_reason=None):
    return SimpleNamespace(
        choices=[
            SimpleNamespace(
                delta=SimpleNamespace(content=content, reasoning_content=reasoning, reasoning=None, tool_calls=None),
                finish_reason=finish_reason,
            )
        ],
        usage=None,
    )


def _stream(chunks):
    async def _iterate():
        for chunk in chunks:
            yield chunk

    return _iterate()


def _make_model():
    model = LiteLLMBase.__new__(LiteLLMBase)
    model.model_name = "test-model"
    model.provider = SupportedLiteLLMProvider.OpenAI
    model.api_key = "test-key"
    model.base_url = "https://example.test/v1"
    model.max_retries = 0
    model.max_rounds = 0
    model.timeout = 1
    model.tools = [
        {
            "type": "function",
            "function": {
                "name": "lookup",
                "parameters": {"type": "object", "properties": {}},
            },
        }
    ]
    model.last_usage = {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}
    return model


async def _collect(monkeypatch, model, *, chunks, with_tools=False, with_reasoning=True):
    async def fake_acompletion(**_kwargs):
        return _stream(chunks)

    monkeypatch.setattr(chat_model.litellm, "acompletion", fake_acompletion)
    history = [{"role": "user", "content": "hello"}]
    if with_tools:
        return [event async for event in model.async_chat_streamly_with_tools("", history, {})]
    return [event async for event in model.async_chat_streamly("", history, {}, with_reasoning=with_reasoning)]


@pytest.mark.asyncio
@pytest.mark.parametrize("with_tools", [False, True])
async def test_reasoning_stream_emits_one_marker_pair(monkeypatch, with_tools):
    events = await _collect(
        monkeypatch,
        _make_model(),
        with_tools=with_tools,
        chunks=[
            _stream_chunk(reasoning="reasoning one"),
            _stream_chunk(),
            _stream_chunk(reasoning=" and two"),
            _stream_chunk(content="answer", finish_reason="stop"),
        ],
    )

    text_events = [event for event in events if isinstance(event, str) and event]
    assert text_events == ["<think>", "reasoning one", " and two", "</think>", "answer"]
    assert isinstance(events[-1], int)


@pytest.mark.asyncio
@pytest.mark.parametrize("with_tools", [False, True])
async def test_reasoning_and_answer_in_same_delta_are_both_emitted(monkeypatch, with_tools):
    events = await _collect(
        monkeypatch,
        _make_model(),
        with_tools=with_tools,
        chunks=[_stream_chunk(reasoning="reasoning", content="answer", finish_reason="stop")],
    )

    text_events = [event for event in events if isinstance(event, str) and event]
    assert text_events == ["<think>", "reasoning", "</think>", "answer"]


@pytest.mark.asyncio
async def test_reasoning_stream_can_hide_reasoning(monkeypatch):
    events = await _collect(
        monkeypatch,
        _make_model(),
        with_reasoning=False,
        chunks=[
            _stream_chunk(reasoning="hidden"),
            _stream_chunk(content="answer", finish_reason="stop"),
        ],
    )

    assert [event for event in events if isinstance(event, str) and event] == ["answer"]
    assert isinstance(events[-1], int)


@pytest.mark.asyncio
@pytest.mark.parametrize("finish_reason", [None, "length"])
async def test_reasoning_only_stream_is_closed(monkeypatch, finish_reason):
    events = await _collect(monkeypatch, _make_model(), chunks=[_stream_chunk(reasoning="思考", finish_reason=finish_reason)])

    assert events[:3] == ["<think>", "思考", "</think>"]
    assert events.count("<think>") == 1
    assert events.count("</think>") == 1
    if finish_reason == "length":
        assert LENGTH_NOTIFICATION_CN in events
    assert isinstance(events[-1], int)


@pytest.mark.asyncio
@pytest.mark.parametrize("with_tools", [False, True])
async def test_length_notification_uses_answer_language(monkeypatch, with_tools):
    events = await _collect(
        monkeypatch,
        _make_model(),
        with_tools=with_tools,
        chunks=[_stream_chunk(content="这是答案", finish_reason="length")],
    )

    assert LENGTH_NOTIFICATION_CN in events
    assert LENGTH_NOTIFICATION_EN not in events

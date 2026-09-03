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

import json
from types import SimpleNamespace

import httpx
import pytest
from openai import AsyncOpenAI

from rag.llm import SupportedLiteLLMProvider, chat_model
from rag.llm.chat_model import Base, LiteLLMBase

pytestmark = pytest.mark.p1


def _completion_response():
    """Build the minimal non-stream LiteLLM response used by call-path tests."""
    return SimpleNamespace(
        choices=[
            SimpleNamespace(
                message=SimpleNamespace(content="ok", tool_calls=[]),
                finish_reason="stop",
            )
        ],
        usage=SimpleNamespace(prompt_tokens=1, completion_tokens=1, total_tokens=2),
    )


def _stream_chunk():
    """Build the minimal streaming LiteLLM response chunk."""
    return SimpleNamespace(
        choices=[
            SimpleNamespace(
                delta=SimpleNamespace(content="ok", reasoning_content=None, reasoning=None, tool_calls=None),
                finish_reason="stop",
            )
        ],
        usage=None,
    )


async def _stream_response():
    """Yield one response chunk through LiteLLM's async-stream contract."""
    yield _stream_chunk()


def _make_litellm_qwen(provider=SupportedLiteLLMProvider.OpenAI):
    """Create a lightweight Qwen model without initializing external clients."""
    model = LiteLLMBase.__new__(LiteLLMBase)
    model.model_name = "qwen3-8b"
    model.provider = provider
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


@pytest.mark.asyncio
async def test_qwen3_openai_sdk_sends_nested_transport_json():
    """The OpenAI SDK must flatten extra_body into vLLM's final JSON shape."""
    sent_body = {}

    def handle_request(request):
        """Capture the exact JSON produced by the OpenAI SDK transport."""
        sent_body.update(json.loads(request.content))
        return httpx.Response(
            200,
            json={
                "id": "chatcmpl-test",
                "object": "chat.completion",
                "created": 0,
                "model": "qwen3-8b",
                "choices": [
                    {
                        "index": 0,
                        "message": {"role": "assistant", "content": "ok"},
                        "finish_reason": "stop",
                    }
                ],
                "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
            },
        )

    http_client = httpx.AsyncClient(transport=httpx.MockTransport(handle_request))
    model = Base.__new__(Base)
    model.model_name = "qwen3-8b"
    model.async_client = AsyncOpenAI(
        api_key="test-key",
        base_url="https://example.test/v1",
        http_client=http_client,
        max_retries=0,
    )
    model.last_usage = {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}

    try:
        answer, _ = await model._async_chat(
            [{"role": "user", "content": "hello"}],
            {"thinking": "disabled"},
            extra_body={
                "seed": 1,
                "chat_template_kwargs": {"custom_template_flag": "keep"},
            },
        )
    finally:
        await model.async_client.close()

    assert answer == "ok"
    assert sent_body["seed"] == 1
    assert sent_body["chat_template_kwargs"] == {
        "custom_template_flag": "keep",
        "enable_thinking": False,
    }
    assert "enable_thinking" not in sent_body
    assert "thinking" not in sent_body
    assert "extra_body" not in sent_body


@pytest.mark.asyncio
@pytest.mark.parametrize("path", ["chat", "stream", "tools", "stream_tools"])
async def test_litellm_qwen3_payload_reaches_every_chat_path(monkeypatch, path):
    """Every LiteLLM chat entry point must retain the normalized Qwen payload."""
    calls = []

    async def fake_acompletion(**kwargs):
        """Capture the final arguments passed through each LiteLLM entry point."""
        calls.append(kwargs)
        if kwargs.get("stream"):
            return _stream_response()
        return _completion_response()

    monkeypatch.setattr(chat_model.litellm, "acompletion", fake_acompletion)
    model = _make_litellm_qwen()
    history = [{"role": "user", "content": "hello"}]
    gen_conf = {
        "thinking": "enabled",
        "extra_body": {
            "seed": 1,
            "chat_template_kwargs": {"custom_template_flag": "keep"},
        },
    }

    if path == "chat":
        await model.async_chat("", history, gen_conf)
    elif path == "stream":
        _ = [item async for item in model.async_chat_streamly("", history, gen_conf)]
    elif path == "tools":
        await model.async_chat_with_tools("", history, gen_conf)
    else:
        _ = [item async for item in model.async_chat_streamly_with_tools("", history, gen_conf)]

    assert len(calls) == 1
    completion_args = calls[0]
    assert completion_args["extra_body"] == {
        "seed": 1,
        "chat_template_kwargs": {
            "custom_template_flag": "keep",
            "enable_thinking": True,
        },
    }
    assert "thinking" not in completion_args
    assert "enable_thinking" not in completion_args
    assert gen_conf == {
        "thinking": "enabled",
        "extra_body": {
            "seed": 1,
            "chat_template_kwargs": {"custom_template_flag": "keep"},
        },
    }


def test_litellm_dashscope_qwen3_keeps_provider_native_payload():
    """DashScope must keep its provider-native root body field."""
    model = _make_litellm_qwen(SupportedLiteLLMProvider.Dashscope)

    completion_args = model._construct_completion_args(
        history=[],
        stream=False,
        tools=False,
        thinking="disabled",
    )

    assert completion_args["extra_body"] == {"enable_thinking": False}
    assert "enable_thinking" not in completion_args
    assert "chat_template_kwargs" not in completion_args["extra_body"]


def test_litellm_openrouter_qwen3_preserves_routing_and_thinking_payload():
    """OpenRouter routing must not replace Qwen chat-template settings."""
    model = _make_litellm_qwen(SupportedLiteLLMProvider.OpenRouter)
    model.provider_order = "provider-a, provider-b"

    completion_args = model._construct_completion_args(
        history=[],
        stream=False,
        tools=False,
        thinking="disabled",
        extra_body={
            "seed": 1,
            "chat_template_kwargs": {"custom_template_flag": "keep"},
        },
    )

    assert completion_args["api_base"] == model.base_url
    assert completion_args["extra_body"] == {
        "seed": 1,
        "chat_template_kwargs": {
            "custom_template_flag": "keep",
            "enable_thinking": False,
        },
        "provider": {
            "order": ["provider-a", "provider-b"],
            "allow_fallbacks": False,
        },
    }
    assert "thinking" not in completion_args
    assert "enable_thinking" not in completion_args

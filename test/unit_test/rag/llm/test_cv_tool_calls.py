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

import asyncio
import json
import sys
import types
from types import SimpleNamespace

if "json_repair" not in sys.modules:
    json_repair_stub = types.ModuleType("json_repair")
    json_repair_stub.loads = json.loads
    sys.modules["json_repair"] = json_repair_stub

if "openai" not in sys.modules:
    openai_stub = types.ModuleType("openai")
    openai_lib_stub = types.ModuleType("openai.lib")
    openai_azure_stub = types.ModuleType("openai.lib.azure")

    class _OpenAIClient:
        def __init__(self, *_args, **_kwargs):
            self.chat = SimpleNamespace(completions=SimpleNamespace(create=None))

    openai_stub.OpenAI = _OpenAIClient
    openai_stub.AsyncOpenAI = _OpenAIClient
    openai_azure_stub.AzureOpenAI = _OpenAIClient
    openai_azure_stub.AsyncAzureOpenAI = _OpenAIClient
    sys.modules["openai"] = openai_stub
    sys.modules["openai.lib"] = openai_lib_stub
    sys.modules["openai.lib.azure"] = openai_azure_stub

token_utils_stub = types.ModuleType("common.token_utils")
token_utils_stub.num_tokens_from_string = lambda text: len(str(text or ""))
token_utils_stub.total_token_count_from_response = lambda response: getattr(getattr(response, "usage", None), "total_tokens", 0) or 0
sys.modules.setdefault("common.token_utils", token_utils_stub)

misc_utils_stub = types.ModuleType("common.misc_utils")


async def _thread_pool_exec(fn, *args, **kwargs):
    return fn(*args, **kwargs)


misc_utils_stub.thread_pool_exec = _thread_pool_exec
sys.modules.setdefault("common.misc_utils", misc_utils_stub)

rag_nlp_stub = types.ModuleType("rag.nlp")
rag_nlp_stub.is_english = lambda _text: True
sys.modules.setdefault("rag.nlp", rag_nlp_stub)

rag_prompts_stub = types.ModuleType("rag.prompts.generator")
rag_prompts_stub.vision_llm_describe_prompt = lambda: "Describe this image."
sys.modules.setdefault("rag.prompts.generator", rag_prompts_stub)

from rag.llm.cv_model import OpenAI_APICV


class _FakeCompletions:
    def __init__(self, responses):
        self.responses = list(responses)
        self.calls = []

    async def create(self, **kwargs):
        self.calls.append(kwargs)
        return self.responses.pop(0)


class _FakeToolSession:
    def __init__(self):
        self.calls = []

    async def tool_call_async(self, name, args):
        self.calls.append((name, args))
        return {"ok": True}


def _tool_call(name="lookup_0", arguments='{"query": "ragflow"}'):
    return SimpleNamespace(
        index=0,
        id="call_1",
        function=SimpleNamespace(name=name, arguments=arguments),
    )


def _message(content=None, tool_calls=None):
    return SimpleNamespace(content=content, tool_calls=tool_calls)


def _response(message, finish_reason="stop"):
    return SimpleNamespace(
        choices=[SimpleNamespace(message=message, finish_reason=finish_reason)],
        usage=SimpleNamespace(total_tokens=0),
    )


def test_openai_compatible_cv_models_bind_and_run_tools():
    model = OpenAI_APICV("sk-test", "qwen-vl___OpenAI-API", base_url="http://localhost")
    completions = _FakeCompletions(
        [
            _response(_message(tool_calls=[_tool_call()])),
            _response(_message(content="final answer", tool_calls=[])),
        ]
    )
    model.async_client = SimpleNamespace(chat=SimpleNamespace(completions=completions))

    tool_session = _FakeToolSession()
    tools = [
        {
            "type": "function",
            "function": {
                "name": "lookup_0",
                "description": "Lookup facts.",
                "parameters": {"type": "object", "properties": {}},
            },
        }
    ]
    model.bind_tools(tool_session, tools)

    answer, token_count = asyncio.run(
        model.async_chat_with_tools(
            "",
            [{"role": "user", "content": "use the tool"}],
            {},
            images=["abc"],
        )
    )

    assert model.is_tools is True
    assert answer.endswith("final answer")
    assert token_count == 0
    assert tool_session.calls == [("lookup_0", {"query": "ragflow"})]

    first_request = completions.calls[0]
    assert first_request["model"] == "qwen-vl"
    assert first_request["tools"] == tools
    assert first_request["tool_choice"] == "auto"
    assert first_request["messages"][0]["content"][1]["type"] == "image_url"

    second_request = completions.calls[1]
    assert any(message["role"] == "tool" for message in second_request["messages"])

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

import pytest

from common.constants import LLMType
from rag.llm.chat_model import AnonRouterChat
from rag.llm.cv_model import AnonRouterCV
from rag.llm.model_meta import AnonRouter

pytestmark = pytest.mark.p2

BASE_URL = "https://api.anonrouter.ai/v1"


def test_anonrouter_chat_pins_the_gateway_endpoint():
    # A tenant-supplied base_url must not redirect the tenant's key elsewhere.
    chat = AnonRouterChat("sk-test", "anthropic/claude-sonnet-5", base_url="https://attacker.example/v1")

    assert chat.base_url == BASE_URL
    assert str(chat.client.base_url).rstrip("/") == BASE_URL
    assert str(chat.async_client.base_url).rstrip("/") == BASE_URL


def test_anonrouter_chat_defaults_to_the_gateway_endpoint():
    chat = AnonRouterChat("sk-test", "openai/gpt-6-astra")

    assert chat.base_url == BASE_URL
    assert chat.model_name == "openai/gpt-6-astra"


def test_anonrouter_cv_pins_the_gateway_endpoint():
    cv = AnonRouterCV(
        "sk-test",
        "google/gemini-2.5-flash",
        base_url="https://attacker.example/v1",
    )

    assert cv.base_url == BASE_URL
    assert str(cv.client.base_url).rstrip("/") == BASE_URL
    assert str(cv.async_client.base_url).rstrip("/") == BASE_URL


def test_anonrouter_model_list_url_is_pinned():
    assert AnonRouter(api_key="sk-test", base_url=None)._get_model_list_url() == f"{BASE_URL}/models"
    assert AnonRouter(api_key="sk-test", base_url="https://attacker.example/v1")._get_model_list_url() == f"{BASE_URL}/models"


def test_anonrouter_formats_openai_format_catalogue():
    provider = AnonRouter(api_key="sk-test", base_url=None)

    models = provider._format_model_list(
        {
            "object": "list",
            "data": [
                {"id": "anthropic/claude-sonnet-5", "object": "model"},
                {"id": "deepseek/deepseek-v4-pro", "object": "model"},
                {"object": "model"},
            ],
        }
    )

    assert models == [
        {"name": "anthropic/claude-sonnet-5", "model_types": [LLMType.CHAT.value], "features": [], "max_tokens": 8192},
        {"name": "deepseek/deepseek-v4-pro", "model_types": [LLMType.CHAT.value], "features": [], "max_tokens": 8192},
    ]


def test_anonrouter_classes_share_the_factory_name():
    assert AnonRouterChat._FACTORY_NAME == "AnonRouter"
    assert AnonRouterCV._FACTORY_NAME == "AnonRouter"
    assert AnonRouter._FACTORY_NAME == "AnonRouter"

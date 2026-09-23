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

import pytest
from litellm.llms.bedrock.common_utils import BedrockModelInfo

from rag.llm import SupportedLiteLLMProvider
from rag.llm.chat_model import LiteLLMBase
from rag.llm.cv_model import BedrockCV

pytestmark = pytest.mark.p1

_KEY = json.dumps({"auth_mode": "bedrock_api_key", "bedrock_region": "us-east-1", "bedrock_api_key": "bedrock-test-key"})


@pytest.mark.parametrize(
    "model_name, expected",
    [
        ("us.openai.gpt-6-sol", "bedrock/converse/us.openai.gpt-6-sol"),
        ("global.openai.gpt-6-luna", "bedrock/converse/global.openai.gpt-6-luna"),
        ("us.openai.gpt-6-astra", "bedrock/converse/us.openai.gpt-6-astra"),
        ("openai.gpt-oss-120b-1:0", "bedrock/openai.gpt-oss-120b-1:0"),
        ("us.anthropic.claude-sonnet-4-6", "bedrock/us.anthropic.claude-sonnet-4-6"),
        ("converse/us.openai.gpt-6-sol", "bedrock/converse/us.openai.gpt-6-sol"),
        ("invoke/us.openai.gpt-6-sol", "bedrock/invoke/us.openai.gpt-6-sol"),
    ],
)
def test_bedrock_chat_and_vision_model_route(model_name, expected, monkeypatch):
    # What LiteLLM 1.84.0's bundled map does for OpenAI GPT-5.x / GPT-6: no entry, so InvokeModel.
    monkeypatch.setattr(BedrockModelInfo, "get_bedrock_route", staticmethod(lambda model: "invoke"))

    assert LiteLLMBase(_KEY, model_name, provider=SupportedLiteLLMProvider.Bedrock).model_name == expected
    assert BedrockCV(_KEY, model_name).model_name == expected


def test_bedrock_openai_gpt_keeps_litellm_route_once_litellm_knows_the_model(monkeypatch):
    monkeypatch.setattr(BedrockModelInfo, "get_bedrock_route", staticmethod(lambda model: "converse"))

    assert LiteLLMBase(_KEY, "us.openai.gpt-6-sol", provider=SupportedLiteLLMProvider.Bedrock).model_name == "bedrock/us.openai.gpt-6-sol"

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
from unittest.mock import MagicMock, patch

import pytest
from botocore import UNSIGNED

from rag.llm import SupportedLiteLLMProvider
from rag.llm.chat_model import LiteLLMBase
from rag.llm.cv_model import BedrockCV
from rag.llm.embedding_model import BedrockEmbed


class _BedrockChat(LiteLLMBase):
    pass


def _api_key_config(**overrides: str) -> str:
    config = {
        "auth_mode": "bedrock_api_key",
        "bedrock_region": "us-east-1",
        "bedrock_api_key": "bedrock-test-key",
        **overrides,
    }
    return json.dumps(config)


def _chat_model(api_key: str) -> _BedrockChat:
    return _BedrockChat(
        api_key,
        "amazon.nova-lite-v1:0",
        provider=SupportedLiteLLMProvider.Bedrock,
        max_retries=5,
    )


def test_bedrock_chat_uses_request_scoped_bearer_token():
    model = _chat_model(_api_key_config())
    args = model._construct_completion_args(
        [{"role": "user", "content": "Hi"}],
        stream=False,
        tools=False,
    )

    assert model.model_name == "bedrock/amazon.nova-lite-v1:0"
    assert args["aws_region_name"] == "us-east-1"
    assert args["api_key"] == "bedrock-test-key"
    assert "aws_access_key_id" not in args
    assert "aws_secret_access_key" not in args


def test_bedrock_chat_requires_api_key_value():
    with pytest.raises(ValueError, match="Bedrock API key must be provided"):
        _chat_model(_api_key_config(bedrock_api_key=""))._construct_completion_args(
            [{"role": "user", "content": "Hi"}],
            stream=False,
            tools=False,
        )


def test_bedrock_embedding_uses_unsigned_client_with_bearer_token():
    client = MagicMock()
    with patch("boto3.client", return_value=client) as client_factory:
        BedrockEmbed(_api_key_config(), "amazon.titan-embed-text-v2:0")

    kwargs = client_factory.call_args.kwargs
    assert kwargs["service_name"] == "bedrock-runtime"
    assert kwargs["region_name"] == "us-east-1"
    assert kwargs["config"].signature_version == UNSIGNED

    event_name, add_bearer_token = client.meta.events.register.call_args.args
    request = SimpleNamespace(headers={})
    add_bearer_token(request)
    assert event_name == "before-sign.bedrock-runtime.*"
    assert request.headers["Authorization"] == "Bearer bedrock-test-key"


def test_bedrock_vision_uses_request_scoped_bearer_token():
    args = BedrockCV(_api_key_config(), "amazon.nova-lite-v1:0")._get_aws_creds()

    assert args["aws_region_name"] == "us-east-1"
    assert args["api_key"] == "bedrock-test-key"
    assert "aws_access_key_id" not in args
    assert "aws_secret_access_key" not in args

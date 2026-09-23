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
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

import pytest
from botocore import UNSIGNED
from botocore.exceptions import ClientError

from common.constants import LLMType
from rag.llm.model_meta import Bedrock

pytestmark = pytest.mark.p2


def test_bedrock_lists_supported_on_demand_models_with_bearer_token():
    client = MagicMock()
    client.list_foundation_models.return_value = {
        "modelSummaries": [
            {
                "modelId": "amazon.nova-lite-v1:0",
                "inputModalities": ["TEXT", "IMAGE"],
                "outputModalities": ["TEXT"],
                "inferenceTypesSupported": ["ON_DEMAND"],
                "modelLifecycle": {"status": "ACTIVE"},
            },
            {
                "modelId": "amazon.titan-embed-text-v2:0",
                "inputModalities": ["TEXT"],
                "outputModalities": ["EMBEDDING"],
                "inferenceTypesSupported": ["ON_DEMAND"],
            },
            {
                "modelId": "cohere.rerank-v3-5:0",
                "inputModalities": ["TEXT"],
                "outputModalities": ["TEXT"],
            },
        ]
    }
    config = {
        "auth_mode": "bedrock_api_key",
        "bedrock_api_key": "bedrock-test-key",
        "bedrock_region": "us-east-1",
    }

    with patch("boto3.client", return_value=client) as client_factory:
        models = asyncio.run(Bedrock(config).get_model_list())

    kwargs = client_factory.call_args.kwargs
    assert kwargs["service_name"] == "bedrock"
    assert kwargs["region_name"] == "us-east-1"
    assert kwargs["config"].signature_version == UNSIGNED
    event_name, add_bearer_token = client.meta.events.register.call_args.args
    request = SimpleNamespace(headers={})
    add_bearer_token(request)
    assert event_name == "before-sign.bedrock.*"
    assert request.headers["Authorization"] == "Bearer bedrock-test-key"
    assert models == [
        {
            "name": "amazon.nova-lite-v1:0",
            "model_types": [LLMType.CHAT.value, LLMType.VISION.value],
            "max_tokens": 8192,
            "features": [],
        },
        {
            "name": "amazon.titan-embed-text-v2:0",
            "model_types": [LLMType.EMBEDDING.value],
            "max_tokens": 8192,
            "features": [],
        },
    ]


def test_bedrock_requires_api_key_for_model_discovery():
    config = {
        "auth_mode": "bedrock_api_key",
        "bedrock_api_key": "",
        "bedrock_region": "us-east-1",
    }

    with pytest.raises(ValueError, match="Bedrock API key must be provided"):
        asyncio.run(Bedrock(config).get_model_list())


def test_bedrock_model_discovery_error_includes_region():
    client = MagicMock()
    client.list_foundation_models.side_effect = ClientError(
        {
            "Error": {"Code": "AccessDeniedException", "Message": "denied"},
            "ResponseMetadata": {"HTTPStatusCode": 403},
        },
        "ListFoundationModels",
    )
    config = {
        "auth_mode": "bedrock_api_key",
        "bedrock_api_key": "bedrock-test-key",
        "bedrock_region": "us-east-1",
    }

    with patch("boto3.client", return_value=client), pytest.raises(ValueError, match="region 'us-east-1'"):
        asyncio.run(Bedrock(config).get_model_list())


def _profile_only_client():
    summaries = [
        {
            "modelId": "openai.gpt-6-sol",
            "inputModalities": ["TEXT", "IMAGE"],
            "outputModalities": ["TEXT"],
            "inferenceTypesSupported": ["INFERENCE_PROFILE"],
            "modelLifecycle": {"status": "ACTIVE"},
        },
        {
            "modelId": "openai.gpt-oss-120b-1:0",
            "inputModalities": ["TEXT"],
            "outputModalities": ["TEXT"],
            "inferenceTypesSupported": ["ON_DEMAND"],
        },
        {
            "modelId": "amazon.titan-embed-text-v2:0:8k",
            "inputModalities": ["TEXT"],
            "outputModalities": ["EMBEDDING"],
            "inferenceTypesSupported": [],
        },
        {
            "modelId": "cohere.embed-v4:0",
            "inputModalities": ["TEXT", "IMAGE"],
            "outputModalities": ["EMBEDDING"],
            "inferenceTypesSupported": ["INFERENCE_PROFILE"],
        },
    ]
    client = MagicMock()
    # Like the API: byInferenceType narrows the list server-side.
    client.list_foundation_models.side_effect = lambda byInferenceType=None: {"modelSummaries": [summary for summary in summaries if byInferenceType in (None, *summary["inferenceTypesSupported"])]}
    arn = "arn:aws:bedrock:{}::foundation-model/openai.gpt-6-sol"
    client.get_paginator.return_value.paginate.return_value = [
        {
            "inferenceProfileSummaries": [
                {"inferenceProfileId": "us.openai.gpt-6-sol", "models": [{"modelArn": arn.format(region)} for region in ("us-east-1", "us-east-2", "us-west-2")]},
                {"inferenceProfileId": "global.openai.gpt-6-sol", "models": [{"modelArn": arn.format("")}]},
                {"inferenceProfileId": "us.cohere.embed-v4:0", "models": [{"modelArn": "arn:aws:bedrock:us-west-2::foundation-model/cohere.embed-v4:0"}]},
            ]
        }
    ]
    return client


def test_bedrock_lists_inference_profile_ids_for_models_without_on_demand():
    client = _profile_only_client()
    config = {
        "auth_mode": "bedrock_api_key",
        "bedrock_api_key": "bedrock-test-key",
        "bedrock_region": "us-east-1",
    }

    with patch("boto3.client", return_value=client):
        models = asyncio.run(Bedrock(config).get_model_list())

    # us.cohere.embed-v4:0 is not offered: BedrockEmbed needs the bare ``cohere.`` id.
    assert [(model["name"], model["model_types"]) for model in models] == [
        ("us.openai.gpt-6-sol", [LLMType.CHAT.value, LLMType.VISION.value]),
        ("global.openai.gpt-6-sol", [LLMType.CHAT.value, LLMType.VISION.value]),
        ("openai.gpt-oss-120b-1:0", [LLMType.CHAT.value]),
    ]
    client.get_paginator.assert_called_once_with("list_inference_profiles")


def test_bedrock_model_discovery_falls_back_to_on_demand_models_when_profiles_are_denied():
    client = _profile_only_client()
    client.get_paginator.return_value.paginate.side_effect = ClientError(
        {
            "Error": {"Code": "AccessDeniedException", "Message": "denied"},
            "ResponseMetadata": {"HTTPStatusCode": 403},
        },
        "ListInferenceProfiles",
    )
    config = {
        "auth_mode": "bedrock_api_key",
        "bedrock_api_key": "bedrock-test-key",
        "bedrock_region": "us-east-1",
    }

    with patch("boto3.client", return_value=client):
        models = asyncio.run(Bedrock(config).get_model_list())

    assert [model["name"] for model in models] == ["openai.gpt-oss-120b-1:0"]

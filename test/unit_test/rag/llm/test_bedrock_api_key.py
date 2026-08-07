#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#

import asyncio
import json
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

import pytest

from api.apps.services.provider_api_service import _bedrock_model_list_api_key, list_provider_models, verify_api_key
from rag.llm.bedrock_model_discovery import BEDROCK_DISCOVERY_TIMEOUT_SECONDS, BedrockModelDiscoveryError, create_bedrock_bearer_client, discover_bedrock_models
from rag.llm.chat_model import LiteLLMBase, SupportedLiteLLMProvider
from rag.llm.cv_model import BedrockCV
from rag.llm.embedding_model import BedrockEmbed
from rag.llm.model_meta import Bedrock as BedrockModelMeta
from rag.llm.rerank_model import BedrockRerank
from rag.utils.bedrock_endpoint import normalize_bedrock_endpoint, resolve_bedrock_endpoint, validate_bedrock_endpoint_target, validate_bedrock_region


def test_bedrock_model_list_api_key_maps_extensions():
    result = _bedrock_model_list_api_key(
        "token",
        "ap-northeast-1",
        {
            "auth_mode": "bedrock_api_key",
            "endpoint_type": "runtime",
            "discovery_endpoint_url": "https://bedrock.ap-northeast-1.amazonaws.com",
        },
    )

    assert result is not None
    assert json.loads(result) == {
        "auth_mode": "bedrock_api_key",
        "bedrock_api_key": "token",
        "bedrock_region": "ap-northeast-1",
        "bedrock_endpoint_type": "runtime",
        "bedrock_discovery_endpoint_url": "https://bedrock.ap-northeast-1.amazonaws.com",
    }


def test_bedrock_model_list_api_key_keeps_legacy_string_without_extensions():
    assert _bedrock_model_list_api_key("legacy", "ap-northeast-1", None) == "legacy"


def test_list_bedrock_models_without_api_key_keeps_static_catalog():
    class UnexpectedDiscovery:
        def __init__(self, *_args):
            raise AssertionError("Bedrock discovery must not run without API-key credentials")

    factory_info = [
        {
            "name": "Bedrock",
            "url": "",
            "llm": [{"name": "static-model", "model_type": "chat", "max_tokens": 1024}],
        }
    ]
    with (
        patch("api.apps.services.provider_api_service.TenantModelProviderService.get_by_id", return_value=(False, None)),
        patch("api.apps.services.provider_api_service.FACTORY_LLM_INFOS", factory_info),
        patch.dict("api.apps.services.provider_api_service.ModelMeta", {"Bedrock": UnexpectedDiscovery}),
    ):
        success, models = asyncio.run(list_provider_models("Bedrock", None, None))

    assert success is True
    assert models == [{"name": "static-model", "max_tokens": 1024, "model_types": ["chat"], "features": []}]


def test_list_bedrock_models_times_out_remote_discovery(monkeypatch):
    monkeypatch.delenv("LLM_TIMEOUT_SECONDS", raising=False)

    class SlowDiscovery:
        def __init__(self, *_args):
            pass

        async def get_model_list(self):
            return []

    async def raise_timeout(awaitable, **kwargs):
        assert kwargs["timeout"] == 10
        awaitable.close()
        raise TimeoutError

    factory_info = [{"name": "Bedrock", "url": "", "llm": []}]
    api_key = json.dumps(
        {
            "auth_mode": "bedrock_api_key",
            "bedrock_api_key": "token",
            "bedrock_region": "ap-northeast-1",
        }
    )
    with (
        patch("api.apps.services.provider_api_service.TenantModelProviderService.get_by_id", return_value=(False, None)),
        patch("api.apps.services.provider_api_service.FACTORY_LLM_INFOS", factory_info),
        patch.dict("api.apps.services.provider_api_service.ModelMeta", {"Bedrock": SlowDiscovery}),
        patch("api.apps.services.provider_api_service.asyncio.wait_for", side_effect=raise_timeout),
    ):
        success, message = asyncio.run(list_provider_models("Bedrock", api_key, None))

    assert success is False
    assert message == "Timed out while listing models from Bedrock"


@patch("boto3.client")
def test_runtime_discovery_client_has_bounded_network_timeouts(mock_client):
    create_bedrock_bearer_client("bedrock", "token", "ap-northeast-1")

    config = mock_client.call_args.kwargs["config"]
    assert config.connect_timeout == BEDROCK_DISCOVERY_TIMEOUT_SECONDS
    assert config.read_timeout == BEDROCK_DISCOVERY_TIMEOUT_SECONDS
    assert config.retries == {"max_attempts": 0}


def test_verify_bedrock_api_key_without_models_uses_discovery():
    class SuccessfulBedrockDiscovery:
        def __init__(self, api_key, base_url):
            assert json.loads(api_key)["bedrock_api_key"] == "token"
            assert base_url == ""

        async def get_model_list(self):
            return [{"name": "anthropic.claude", "model_types": ["chat"]}]

    api_key = json.dumps(
        {
            "auth_mode": "bedrock_api_key",
            "bedrock_api_key": "token",
            "bedrock_region": "ap-northeast-1",
        }
    )
    with (
        patch("api.apps.services.provider_api_service.TenantModelProviderService.get_by_id", return_value=(False, None)),
        patch.dict("api.apps.services.provider_api_service.ModelMeta", {"Bedrock": SuccessfulBedrockDiscovery}),
    ):
        success, message, model_results = asyncio.run(verify_api_key("Bedrock", api_key, "", "ap-northeast-1", []))

    assert success is True
    assert message == "success"
    assert model_results == {}


def test_verify_bedrock_api_key_without_models_uses_default_for_invalid_timeout(monkeypatch):
    monkeypatch.setenv("LLM_TIMEOUT_SECONDS", "invalid")

    class SuccessfulBedrockDiscovery:
        def __init__(self, _api_key, _base_url):
            pass

        async def get_model_list(self):
            return [{"name": "anthropic.claude", "model_types": ["chat"]}]

    api_key = json.dumps(
        {
            "auth_mode": "bedrock_api_key",
            "bedrock_api_key": "token",
            "bedrock_region": "ap-northeast-1",
        }
    )
    with (
        patch("api.apps.services.provider_api_service.TenantModelProviderService.get_by_id", return_value=(False, None)),
        patch.dict("api.apps.services.provider_api_service.ModelMeta", {"Bedrock": SuccessfulBedrockDiscovery}),
    ):
        success, message, model_results = asyncio.run(verify_api_key("Bedrock", api_key, "", "ap-northeast-1", []))

    assert success is True
    assert message == "success"
    assert model_results == {}


def test_verify_selected_bedrock_model_uses_default_for_invalid_timeout(monkeypatch):
    monkeypatch.setenv("LLM_TIMEOUT_SECONDS", "invalid")

    class SuccessfulBedrockChat:
        def __init__(self, *_args, **_kwargs):
            pass

        async def async_chat_streamly(self, *_args, **_kwargs):
            yield "ok"

    async def run_verification(_label, awaitable, timeout_seconds):
        assert timeout_seconds == 10
        awaitable.close()
        return True, True

    api_key = json.dumps(
        {
            "auth_mode": "bedrock_api_key",
            "bedrock_api_key": "token",
            "bedrock_region": "ap-northeast-1",
        }
    )
    model_info = [{"model_name": "anthropic.claude", "model_type": ["chat"]}]
    with (
        patch("api.apps.services.provider_api_service.TenantModelProviderService.get_by_id", return_value=(False, None)),
        patch.dict("api.apps.services.provider_api_service.ChatModel", {"Bedrock": SuccessfulBedrockChat}),
        patch("api.apps.services.provider_api_service._run_verification", side_effect=run_verification),
    ):
        success, message, model_results = asyncio.run(verify_api_key("Bedrock", api_key, "", "ap-northeast-1", model_info))

    assert success is True
    assert message == "success"
    assert model_results == {"anthropic.claude": "success"}


def test_verify_bedrock_api_key_without_models_reports_discovery_error():
    class FailedBedrockDiscovery:
        def __init__(self, _api_key, _base_url):
            pass

        async def get_model_list(self):
            raise BedrockModelDiscoveryError("Failed to list models from Amazon Bedrock")

    api_key = json.dumps(
        {
            "auth_mode": "bedrock_api_key",
            "bedrock_api_key": "invalid",
            "bedrock_region": "ap-northeast-1",
        }
    )
    with (
        patch("api.apps.services.provider_api_service.TenantModelProviderService.get_by_id", return_value=(False, None)),
        patch.dict("api.apps.services.provider_api_service.ModelMeta", {"Bedrock": FailedBedrockDiscovery}),
    ):
        success, message, model_results = asyncio.run(verify_api_key("Bedrock", api_key, "", "ap-northeast-1", []))

    assert success is False
    assert message == "\nFail to access Bedrock model discovery.Failed to list models from Amazon Bedrock"
    assert model_results == {}


def test_verify_bedrock_api_key_without_models_rejects_empty_discovery():
    class EmptyBedrockDiscovery:
        def __init__(self, _api_key, _base_url):
            pass

        async def get_model_list(self):
            return []

    api_key = json.dumps(
        {
            "auth_mode": "bedrock_api_key",
            "bedrock_api_key": "token",
            "bedrock_region": "ap-northeast-1",
        }
    )
    with (
        patch("api.apps.services.provider_api_service.TenantModelProviderService.get_by_id", return_value=(False, None)),
        patch.dict("api.apps.services.provider_api_service.ModelMeta", {"Bedrock": EmptyBedrockDiscovery}),
    ):
        success, message, model_results = asyncio.run(verify_api_key("Bedrock", api_key, "", "ap-northeast-1", []))

    assert success is False
    assert message == "No compatible Bedrock models were discovered"
    assert model_results == {}


def test_list_bedrock_models_reports_discovery_error():
    class FailedBedrockDiscovery:
        def __init__(self, _api_key, _base_url):
            pass

        async def get_model_list(self):
            raise BedrockModelDiscoveryError("Failed to list models from Amazon Bedrock")

    api_key = json.dumps(
        {
            "auth_mode": "bedrock_api_key",
            "bedrock_api_key": "invalid",
            "bedrock_region": "ap-northeast-1",
        }
    )
    factory_info = [{"name": "Bedrock", "url": "", "llm": []}]
    with (
        patch("api.apps.services.provider_api_service.TenantModelProviderService.get_by_id", return_value=(False, None)),
        patch("api.apps.services.provider_api_service.FACTORY_LLM_INFOS", factory_info),
        patch.dict("api.apps.services.provider_api_service.ModelMeta", {"Bedrock": FailedBedrockDiscovery}),
    ):
        success, message = asyncio.run(list_provider_models("Bedrock", api_key, None))

    assert success is False
    assert message == "Failed to list models from Amazon Bedrock"


def test_bedrock_model_meta_rejects_non_object_api_key():
    with pytest.raises(ValueError, match="must be a JSON object"):
        asyncio.run(BedrockModelMeta("12345").get_model_list())


def test_normalize_mantle_endpoint_from_models_url():
    endpoint = "https://bedrock-mantle.ap-northeast-1.api.aws/v1/models"
    assert normalize_bedrock_endpoint("mantle_openai", endpoint) == "https://bedrock-mantle.ap-northeast-1.api.aws/v1"
    assert normalize_bedrock_endpoint("mantle_anthropic", endpoint) == "https://bedrock-mantle.ap-northeast-1.api.aws/anthropic"


def test_rejects_untrusted_mantle_endpoint():
    with pytest.raises(ValueError, match="hostname is not allowed"):
        resolve_bedrock_endpoint("bedrock_api_key", "mantle_openai", "https://example.com/v1")


def test_rejects_non_default_bedrock_endpoint_port():
    with pytest.raises(ValueError, match="non-default port"):
        validate_bedrock_endpoint_target("https://bedrock-runtime.us-east-1.amazonaws.com:8443")


def test_allows_explicit_https_bedrock_endpoint_port():
    validate_bedrock_endpoint_target("https://bedrock-runtime.us-east-1.amazonaws.com:443")


@pytest.mark.parametrize("region", ["attacker.example?ignored=", "ap_northeast_1", "-us-east-1", "us-east-1-"])
def test_rejects_region_values_that_can_escape_the_aws_hostname(region):
    with pytest.raises(ValueError, match="valid AWS region identifier"):
        validate_bedrock_region(region)


def test_runtime_chat_rejects_malicious_region_before_litellm_request():
    model = LiteLLMBase(
        '{"auth_mode":"bedrock_api_key","bedrock_region":"attacker.example?ignored=","bedrock_api_key":"token"}',
        "anthropic.claude-sonnet-current",
        provider=SupportedLiteLLMProvider.Bedrock,
    )

    with pytest.raises(ValueError, match="valid AWS region identifier"):
        model._construct_completion_args([{"role": "user", "content": "hi"}], False, False)


def test_runtime_chat_uses_bearer_header_without_ambient_aws_credentials():
    model = LiteLLMBase(
        '{"auth_mode":"bedrock_api_key","bedrock_region":"ap-northeast-1","bedrock_api_key":"token"}',
        "anthropic.claude-sonnet-current",
        provider=SupportedLiteLLMProvider.Bedrock,
    )

    args = model._construct_completion_args([{"role": "user", "content": "hi"}], False, False)

    assert args["extra_headers"]["Authorization"] == "Bearer token"
    assert args["aws_access_key_id"] == "bedrock-api-key"
    assert args["aws_secret_access_key"] == "bedrock-api-key"
    assert "api_key" not in args


def test_runtime_chat_trims_bedrock_api_key():
    model = LiteLLMBase(
        '{"auth_mode":"bedrock_api_key","bedrock_region":"ap-northeast-1","bedrock_api_key":" token "}',
        "anthropic.claude-sonnet-current",
        provider=SupportedLiteLLMProvider.Bedrock,
    )

    args = model._construct_completion_args([{"role": "user", "content": "hi"}], False, False)

    assert args["extra_headers"]["Authorization"] == "Bearer token"


def test_runtime_vision_uses_bearer_header_without_ambient_aws_credentials():
    model = BedrockCV(
        '{"auth_mode":"bedrock_api_key","bedrock_region":"ap-northeast-1","bedrock_api_key":"token"}',
        "anthropic.claude-sonnet-current",
    )

    args = model._get_aws_creds()

    assert args["extra_headers"]["Authorization"] == "Bearer token"
    assert args["aws_access_key_id"] == "bedrock-api-key"
    assert args["aws_secret_access_key"] == "bedrock-api-key"
    assert "api_key" not in args


def test_runtime_vision_trims_bedrock_api_key():
    model = BedrockCV(
        '{"auth_mode":"bedrock_api_key","bedrock_region":"ap-northeast-1","bedrock_api_key":" token "}',
        "anthropic.claude-sonnet-current",
    )

    args = model._get_aws_creds()

    assert args["extra_headers"]["Authorization"] == "Bearer token"


@patch("rag.llm.bedrock_model_discovery.create_bedrock_bearer_client")
def test_runtime_discovery_uses_remote_modalities(mock_create_client):
    mock_create_client.return_value.list_foundation_models.return_value = {
        "modelSummaries": [
            {
                "modelId": "anthropic.claude-sonnet-current",
                "inputModalities": ["TEXT", "IMAGE"],
                "outputModalities": ["TEXT"],
                "inferenceTypesSupported": ["ON_DEMAND"],
                "modelLifecycle": {"status": "ACTIVE"},
            },
            {
                "modelId": "amazon.titan-embed-text-v2:0",
                "inputModalities": ["TEXT"],
                "outputModalities": ["EMBEDDING"],
                "inferenceTypesSupported": ["ON_DEMAND", "PROVISIONED"],
                "modelLifecycle": {"status": "ACTIVE"},
            },
            {
                "modelId": "amazon.rerank-v1:0",
                "inputModalities": ["TEXT"],
                "outputModalities": ["TEXT"],
                "inferenceTypesSupported": ["ON_DEMAND"],
                "modelLifecycle": {"status": "ACTIVE"},
            },
            {
                "modelId": "anthropic.provisioned-only",
                "inputModalities": ["TEXT"],
                "outputModalities": ["TEXT"],
                "inferenceTypesSupported": ["PROVISIONED"],
                "modelLifecycle": {"status": "ACTIVE"},
            },
            {
                "modelId": "anthropic.legacy-model",
                "inputModalities": ["TEXT"],
                "outputModalities": ["TEXT"],
                "inferenceTypesSupported": ["ON_DEMAND"],
                "modelLifecycle": {"status": "LEGACY"},
            },
        ]
    }

    models = discover_bedrock_models(
        {
            "bedrock_api_key": "token",
            "bedrock_region": "ap-northeast-1",
            "bedrock_endpoint_type": "runtime",
        }
    )

    assert models == [
        {
            "name": "anthropic.claude-sonnet-current",
            "model_types": ["chat", "vision"],
            "max_tokens": 8192,
            "features": [],
        },
        {
            "name": "amazon.titan-embed-text-v2:0",
            "model_types": ["embedding"],
            "max_tokens": 8192,
            "features": [],
        },
    ]
    mock_create_client.return_value.list_foundation_models.assert_called_once_with(byInferenceType="ON_DEMAND")


@patch("openai.OpenAI")
def test_mantle_anthropic_discovery_filters_catalog(mock_openai):
    mock_openai.return_value.models.list.return_value = SimpleNamespace(
        data=[
            SimpleNamespace(id="Anthropic.claude-sonnet-current"),
            SimpleNamespace(id="amazon.nova-pro-v1:0"),
            SimpleNamespace(id="amazon.titan-embed-text-v2:0"),
        ]
    )

    models = discover_bedrock_models(
        {
            "bedrock_api_key": "token",
            "bedrock_region": "ap-northeast-1",
            "bedrock_endpoint_type": "mantle_anthropic",
            "bedrock_endpoint_url": "https://bedrock-mantle.ap-northeast-1.api.aws/v1/models",
        }
    )

    assert [model["name"] for model in models] == ["Anthropic.claude-sonnet-current"]
    mock_openai.assert_called_once_with(
        api_key="token",
        base_url="https://bedrock-mantle.ap-northeast-1.api.aws/v1",
        timeout=BEDROCK_DISCOVERY_TIMEOUT_SECONDS,
        max_retries=0,
    )


def test_bedrock_bearer_client_registers_request_scoped_header():
    events = MagicMock()
    client = SimpleNamespace(meta=SimpleNamespace(events=events))
    with patch("boto3.client", return_value=client):
        from rag.llm.bedrock_model_discovery import create_bedrock_bearer_client

        assert create_bedrock_bearer_client("bedrock-runtime", " token ", "ap-northeast-1") is client
    event_name, callback = events.register.call_args.args
    request = SimpleNamespace(headers={})
    callback(request)
    assert event_name == "before-sign.bedrock-runtime.*"
    assert request.headers["Authorization"] == "Bearer token"


def test_bedrock_bearer_client_rejects_control_characters():
    with pytest.raises(ValueError, match="control characters"):
        create_bedrock_bearer_client("bedrock-runtime", "token\nInjected: value", "ap-northeast-1")


@patch("rag.llm.bedrock_model_discovery.create_bedrock_bearer_client")
def test_embedding_uses_request_scoped_bearer_client(mock_create_client):
    client = MagicMock()
    mock_create_client.return_value = client
    model = BedrockEmbed(
        '{"auth_mode":"bedrock_api_key","bedrock_region":"ap-northeast-1","bedrock_api_key":"token"}',
        "amazon.titan-embed-text-v2:0",
    )
    assert model.client is client
    mock_create_client.assert_called_once_with("bedrock-runtime", "token", "ap-northeast-1", "")


def test_rerank_rejects_bedrock_api_key():
    with pytest.raises(ValueError, match=r"do not support.*rerank"):
        BedrockRerank(
            '{"auth_mode":"bedrock_api_key","bedrock_region":"ap-northeast-1","bedrock_api_key":"token"}',
            "amazon.rerank-v1:0",
        )


def test_mantle_anthropic_chat_uses_api_key_header_credentials():
    model = LiteLLMBase(
        '{"auth_mode":"bedrock_api_key","bedrock_region":"ap-northeast-1","bedrock_api_key":"token","bedrock_endpoint_type":"mantle_anthropic","bedrock_endpoint_url":"https://bedrock-mantle.ap-northeast-1.api.aws"}',
        "anthropic.claude-sonnet-current",
        provider=SupportedLiteLLMProvider.Bedrock,
    )

    args = model._construct_completion_args([{"role": "user", "content": "hi"}], False, False)

    assert args["model"] == "anthropic/anthropic.claude-sonnet-current"
    assert args["api_key"] == "token"
    assert "extra_headers" not in args


def test_mantle_anthropic_vision_uses_api_key_header_credentials():
    model = BedrockCV(
        '{"auth_mode":"bedrock_api_key","bedrock_region":"ap-northeast-1","bedrock_api_key":"token","bedrock_endpoint_type":"mantle_anthropic","bedrock_endpoint_url":"https://bedrock-mantle.ap-northeast-1.api.aws"}',
        "anthropic.claude-sonnet-current",
    )

    args = model._get_aws_creds()

    assert model.litellm_model_name == "anthropic/anthropic.claude-sonnet-current"
    assert args["api_key"] == "token"
    assert "extra_headers" not in args


def test_bedrock_vision_requires_region():
    with pytest.raises(ValueError, match="region must be provided"):
        BedrockCV(
            '{"auth_mode":"bedrock_api_key","bedrock_api_key":"token"}',
            "anthropic.claude-sonnet-current",
        )

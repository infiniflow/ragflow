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
import threading
import time
from datetime import datetime, timedelta, timezone
from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from api.apps.services.provider_api_service import _bedrock_model_list_api_key, _normalize_model_info, create_provider_instance, list_provider_models, update_provider_instance, verify_api_key
from rag.llm.bedrock_model_discovery import BEDROCK_DISCOVERY_TIMEOUT_SECONDS, BedrockModelDiscoveryError, create_bedrock_bearer_client, discover_bedrock_models
from rag.llm.chat_model import LiteLLMBase, SupportedLiteLLMProvider
from rag.llm.cv_model import BedrockCV
from rag.llm.embedding_model import BedrockEmbed
from rag.llm.model_meta import Bedrock as BedrockModelMeta
from rag.llm.rerank_model import BedrockRerank
from rag.utils.bedrock_endpoint import normalize_bedrock_endpoint, resolve_bedrock_endpoint, validate_bedrock_api_key, validate_bedrock_endpoint_target, validate_bedrock_region


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


def test_model_info_validation_rejects_mixed_unclassified_models() -> None:
    normalized, error = _normalize_model_info(
        [
            {"model_name": "valid-model", "model_type": ["chat"]},
            {"model_name": "future-model", "model_type": []},
        ]
    )

    assert normalized is None
    assert error == "Model 'future-model': At least one supported model_type is required"


def test_model_info_validation_canonicalizes_legacy_type_aliases() -> None:
    normalized, error = _normalize_model_info([{"model_name": " model ", "model_type": ["image2text", "speech2text", "doc_parse"]}])

    assert error is None
    assert normalized == [{"model_name": "model", "model_type": ["vision", "asr", "doc_parse"]}]


@pytest.mark.parametrize(
    ("model_info", "expected_error"),
    [
        (
            [
                {"model_name": " duplicate ", "model_type": ["chat"]},
                {"model_name": "duplicate", "model_type": ["embedding"]},
            ],
            "Duplicate model_name 'duplicate'",
        ),
        (
            [{"model_name": "model", "model_type": ["chat"], "extra": "invalid"}],
            "Model 'model': extra must be an object",
        ),
    ],
)
def test_model_info_validation_rejects_non_atomic_inputs(model_info: list[dict], expected_error: str) -> None:
    normalized, error = _normalize_model_info(model_info)

    assert normalized is None
    assert error == expected_error


class _RecordingAtomic:
    def __init__(self):
        self.error_type = None

    def __enter__(self):
        return self

    def __exit__(self, error_type, _error, _traceback):
        self.error_type = error_type
        return False


def test_create_provider_instance_rolls_back_when_a_model_write_fails() -> None:
    atomic = _RecordingAtomic()
    provider = SimpleNamespace(id="provider-id", provider_name="Bedrock")
    models = [
        {"model_name": "model-a", "model_type": ["chat"]},
        {"model_name": "model-b", "model_type": ["chat"]},
    ]
    with (
        patch("api.apps.services.provider_api_service.DB.atomic", return_value=atomic),
        patch("api.apps.services.provider_api_service.FACTORY_LLM_INFOS", [{"name": "Bedrock", "llm": []}]),
        patch("api.apps.services.provider_api_service.TenantModelProviderService.get_by_tenant_id_and_provider_id", return_value=provider),
        patch("api.apps.services.provider_api_service.TenantModelInstanceService.create_instance"),
        patch("api.apps.services.provider_api_service.verify_api_key", new=AsyncMock(return_value=(True, "", {}))),
        patch("api.apps.services.provider_api_service.add_model_to_instance", side_effect=[(True, ""), (False, "second model failed")]),
    ):
        success, message = asyncio.run(create_provider_instance("tenant", "Bedrock", "instance", "token", "", "default", models))

    assert success is False
    assert message == "second model failed"
    assert atomic.error_type.__name__ == "_ModelPersistenceError"


def test_update_provider_instance_rolls_back_when_a_model_write_fails() -> None:
    atomic = _RecordingAtomic()
    provider = SimpleNamespace(id="provider-id", provider_name="Bedrock")
    instance = SimpleNamespace(id="instance-id", provider_id="provider-id", instance_name="instance", extra="{}")
    models = [
        {"model_name": "model-a", "model_type": ["chat"]},
        {"model_name": "model-b", "model_type": ["chat"]},
    ]
    with (
        patch("api.apps.services.provider_api_service.DB.atomic", return_value=atomic),
        patch("api.apps.services.provider_api_service.TenantModelProviderService.get_by_tenant_id_and_provider_id", return_value=provider),
        patch("api.apps.services.provider_api_service.TenantModelInstanceService.get_by_id", return_value=(True, instance)),
        patch("api.apps.services.provider_api_service.TenantModelInstanceService.update_by_id"),
        patch("api.apps.services.provider_api_service.TenantModelService.get_models_by_instance_id", return_value=[]),
        patch("api.apps.services.provider_api_service.add_model_to_instance", side_effect=[(True, ""), (False, "second model failed")]),
    ):
        success, message = asyncio.run(update_provider_instance("tenant", "Bedrock", "instance-id", "instance", "token", "", "default", models, verify=False))

    assert success is False
    assert message == "second model failed"
    assert atomic.error_type.__name__ == "_ModelPersistenceError"


def test_update_provider_instance_clears_empty_endpoint_overrides() -> None:
    atomic = _RecordingAtomic()
    provider = SimpleNamespace(id="provider-id", provider_name="Bedrock")
    instance = SimpleNamespace(
        id="instance-id",
        provider_id="provider-id",
        instance_name="instance",
        extra=json.dumps({"base_url": "https://old.example.com", "region": "us-east-1", "preserved": True}),
    )
    update_by_id = MagicMock()
    with (
        patch("api.apps.services.provider_api_service.DB.atomic", return_value=atomic),
        patch("api.apps.services.provider_api_service.TenantModelProviderService.get_by_tenant_id_and_provider_id", return_value=provider),
        patch("api.apps.services.provider_api_service.TenantModelInstanceService.get_by_id", return_value=(True, instance)),
        patch("api.apps.services.provider_api_service.TenantModelInstanceService.update_by_id", update_by_id),
        patch("api.apps.services.provider_api_service.TenantModelService.get_models_by_instance_id", return_value=[]),
    ):
        success, message = asyncio.run(update_provider_instance("tenant", "Bedrock", "instance-id", "instance", "token", "", None, [], verify=False))

    assert success is True
    assert message == "success"
    update_extra = json.loads(update_by_id.call_args.args[1]["extra"])
    assert update_extra == {"preserved": True}


def test_verify_rejects_mixed_unclassified_models_before_driver_calls() -> None:
    success, message, model_results = asyncio.run(
        verify_api_key(
            "Bedrock",
            "token",
            model_info=[
                {"model_name": "valid-model", "model_type": ["chat"]},
                {"model_name": "future-model", "model_type": []},
            ],
        )
    )

    assert success is False
    assert message == "Model 'future-model': At least one supported model_type is required"
    assert model_results == {}


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
    assert config.retries == {"mode": "standard", "max_attempts": 0}


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

    async def run_verification(_label, awaitable, timeout_seconds):
        assert timeout_seconds == 10
        awaitable.close()
        return True, [{"name": "anthropic.claude", "model_types": ["chat"]}]

    with (
        patch("api.apps.services.provider_api_service.TenantModelProviderService.get_by_id", return_value=(False, None)),
        patch.dict("api.apps.services.provider_api_service.ModelMeta", {"Bedrock": SuccessfulBedrockDiscovery}),
        patch("api.apps.services.provider_api_service._run_verification", side_effect=run_verification),
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
    assert message == "No Bedrock models were discovered"
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


@pytest.mark.parametrize("discovery_endpoint_url", [1, []])
def test_list_bedrock_models_reports_non_string_discovery_endpoint(discovery_endpoint_url: object) -> None:
    factory_info = [{"name": "Bedrock", "url": "", "llm": []}]
    extensions = {
        "auth_mode": "bedrock_api_key",
        "endpoint_type": "runtime",
        "discovery_endpoint_url": discovery_endpoint_url,
    }
    with (
        patch("api.apps.services.provider_api_service.TenantModelProviderService.get_by_id", return_value=(False, None)),
        patch("api.apps.services.provider_api_service.FACTORY_LLM_INFOS", factory_info),
    ):
        success, message = asyncio.run(list_provider_models("Bedrock", "token", None, "ap-northeast-1", extensions))

    assert success is False
    assert message == "Bedrock discovery endpoint URL must be a string"


def test_bedrock_model_meta_rejects_non_object_api_key():
    with pytest.raises(ValueError, match="must be a JSON object"):
        asyncio.run(BedrockModelMeta("12345").get_model_list())


def test_bedrock_model_meta_rejects_malformed_api_key() -> None:
    with pytest.raises(ValueError, match="must be a JSON object"):
        asyncio.run(BedrockModelMeta("{").get_model_list())


def test_normalize_mantle_endpoint_from_models_url():
    endpoint = "https://bedrock-mantle.ap-northeast-1.api.aws/v1/models"
    assert normalize_bedrock_endpoint("mantle_openai", endpoint) == "https://bedrock-mantle.ap-northeast-1.api.aws/v1"
    assert normalize_bedrock_endpoint("mantle_anthropic", endpoint) == "https://bedrock-mantle.ap-northeast-1.api.aws/anthropic"


def test_normalize_mantle_endpoint_trims_surrounding_whitespace() -> None:
    endpoint = "  https://bedrock-mantle.ap-northeast-1.api.aws/v1/models/  "
    assert normalize_bedrock_endpoint("mantle_openai", endpoint) == "https://bedrock-mantle.ap-northeast-1.api.aws/v1"


def test_rejects_untrusted_mantle_endpoint():
    with pytest.raises(ValueError, match="hostname is not allowed"):
        resolve_bedrock_endpoint("bedrock_api_key", "mantle_openai", "https://example.com/v1")


@pytest.mark.parametrize("api_key", [1, ["token"], {"token": "value"}])
def test_rejects_non_string_bedrock_api_key(api_key):
    with pytest.raises(ValueError, match="API key must be a string"):
        validate_bedrock_api_key(api_key)


@pytest.mark.parametrize(
    ("auth_mode", "endpoint_type", "endpoint_url", "message"),
    [
        (["bedrock_api_key"], "runtime", "", "auth_mode must be a string"),
        ("bedrock_api_key", ["runtime"], "", "endpoint type must be a string"),
        ("bedrock_api_key", "runtime", 1, "endpoint URL must be a string"),
    ],
)
def test_rejects_non_string_bedrock_endpoint_fields(auth_mode, endpoint_type, endpoint_url, message):
    with pytest.raises(ValueError, match=message):
        resolve_bedrock_endpoint(auth_mode, endpoint_type, endpoint_url)


def test_rejects_non_default_bedrock_endpoint_port():
    with pytest.raises(ValueError, match="non-default port"):
        validate_bedrock_endpoint_target("https://bedrock-runtime.us-east-1.amazonaws.com:8443")


def test_allows_explicit_https_bedrock_endpoint_port():
    validate_bedrock_endpoint_target("https://bedrock-runtime.us-east-1.amazonaws.com:443")


def test_allows_explicit_allowlisted_proxy_port(monkeypatch):
    monkeypatch.setenv("BEDROCK_ENDPOINT_HOST_ALLOWLIST", "bedrock-proxy.example.com:8443")
    validate_bedrock_endpoint_target("https://bedrock-proxy.example.com:8443")


def test_requires_exact_allowlisted_proxy_port(monkeypatch):
    monkeypatch.setenv("BEDROCK_ENDPOINT_HOST_ALLOWLIST", "bedrock-proxy.example.com")
    with pytest.raises(ValueError, match="exact host:port"):
        validate_bedrock_endpoint_target("https://bedrock-proxy.example.com:8443")


@pytest.mark.parametrize("region", ["attacker.example?ignored=", "ap_northeast_1", "-us-east-1", "us-east-1-"])
def test_rejects_region_values_that_can_escape_the_aws_hostname(region):
    with pytest.raises(ValueError, match="valid AWS region identifier"):
        validate_bedrock_region(region)


@pytest.mark.parametrize("region", [None, 123, []])
def test_rejects_non_string_bedrock_region(region: object) -> None:
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


@pytest.mark.parametrize("credential", ["{", "[]"])
@pytest.mark.parametrize("driver", ["chat", "vision", "embedding", "rerank"])
def test_bedrock_drivers_reject_non_object_credentials(credential: str, driver: str) -> None:
    with pytest.raises(ValueError, match="must be a JSON object"):
        if driver == "chat":
            model = LiteLLMBase(credential, "anthropic.claude-sonnet-current", provider=SupportedLiteLLMProvider.Bedrock)
            model._construct_completion_args([{"role": "user", "content": "hi"}], False, False)
        elif driver == "vision":
            BedrockCV(credential, "anthropic.claude-sonnet-current")
        elif driver == "embedding":
            BedrockEmbed(credential, "amazon.titan-embed-text-v2:0")
        else:
            BedrockRerank(credential, "amazon.rerank-v1:0")


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


@pytest.mark.parametrize(
    ("auth_mode", "credential_fields"),
    [
        ("access_key_secret", {"bedrock_ak": "access", "bedrock_sk": "secret"}),
        ("iam_role", {"aws_role_arn": "arn:aws:iam::123456789012:role/Bedrock"}),
        ("assume_role", {}),
    ],
)
def test_embedding_sigv4_modes_use_resolved_runtime_endpoint(auth_mode: str, credential_fields: dict[str, str]) -> None:
    endpoint_url = "https://bedrock-runtime.ap-northeast-1.amazonaws.com"
    credential = json.dumps(
        {
            "auth_mode": auth_mode,
            "bedrock_region": "ap-northeast-1",
            "bedrock_endpoint_url": endpoint_url,
            **credential_fields,
        }
    )
    sts_client = MagicMock()
    sts_client.assume_role.return_value = {
        "Credentials": {
            "AccessKeyId": "assumed-access",
            "SecretAccessKey": "assumed-secret",
            "SessionToken": "assumed-session",
        }
    }
    runtime_client = MagicMock()

    def create_client(*args, **kwargs):
        service_name = kwargs.get("service_name") or args[0]
        return sts_client if service_name == "sts" else runtime_client

    with patch("boto3.client", side_effect=create_client) as mock_client:
        model = BedrockEmbed(credential, "amazon.titan-embed-text-v2:0")

    assert model.client is runtime_client
    runtime_call = next(call for call in mock_client.call_args_list if call.kwargs.get("service_name") == "bedrock-runtime")
    assert runtime_call.kwargs["region_name"] == "ap-northeast-1"
    assert runtime_call.kwargs["endpoint_url"] == endpoint_url
    if auth_mode == "iam_role":
        assert runtime_call.kwargs["aws_access_key_id"] == "assumed-access"
        sts_call = next(call for call in mock_client.call_args_list if call.args == ("sts",))
        assert sts_call.kwargs == {"region_name": "ap-northeast-1"}


@pytest.mark.parametrize(
    ("auth_mode", "credential_fields"),
    [
        ("access_key_secret", {"bedrock_ak": "access", "bedrock_sk": "secret"}),
        ("iam_role", {"aws_role_arn": "arn:aws:iam::123456789012:role/Bedrock"}),
        ("assume_role", {}),
    ],
)
def test_vision_sigv4_modes_use_resolved_runtime_endpoint(auth_mode: str, credential_fields: dict[str, str]) -> None:
    endpoint_url = "https://bedrock-runtime.ap-northeast-1.amazonaws.com"
    credential = json.dumps(
        {
            "auth_mode": auth_mode,
            "bedrock_region": "ap-northeast-1",
            "bedrock_endpoint_url": endpoint_url,
            **credential_fields,
        }
    )
    sts_client = MagicMock()
    sts_client.assume_role.return_value = {
        "Credentials": {
            "AccessKeyId": "assumed-access",
            "SecretAccessKey": "assumed-secret",
            "SessionToken": "assumed-session",
        }
    }

    with patch("boto3.client", return_value=sts_client):
        args = BedrockCV(credential, "anthropic.claude-sonnet-current")._get_aws_creds()

    assert args["aws_region_name"] == "ap-northeast-1"
    assert args["aws_bedrock_runtime_endpoint"] == endpoint_url
    if auth_mode == "iam_role":
        assert args["aws_access_key_id"] == "assumed-access"


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


@patch("rag.llm.bedrock_model_discovery.create_bedrock_bearer_client")
def test_runtime_discovery_normalizes_catalog_endpoint(mock_create_client) -> None:
    mock_create_client.return_value.list_foundation_models.return_value = {"modelSummaries": []}

    with pytest.raises(ValueError, match="No Bedrock models"):
        discover_bedrock_models(
            {
                "bedrock_api_key": "token",
                "bedrock_region": "ap-northeast-1",
                "bedrock_endpoint_type": "runtime",
                "bedrock_discovery_endpoint_url": "  https://bedrock.ap-northeast-1.amazonaws.com/  ",
            }
        )

    mock_create_client.assert_called_once_with("bedrock", "token", "ap-northeast-1", "https://bedrock.ap-northeast-1.amazonaws.com")


@pytest.mark.parametrize("discovery_endpoint_url", [1, []])
def test_runtime_discovery_rejects_non_string_catalog_endpoint(discovery_endpoint_url: object) -> None:
    with pytest.raises(ValueError, match="discovery endpoint URL must be a string"):
        discover_bedrock_models(
            {
                "bedrock_api_key": "token",
                "bedrock_region": "ap-northeast-1",
                "bedrock_endpoint_type": "runtime",
                "bedrock_discovery_endpoint_url": discovery_endpoint_url,
            }
        )


@patch("openai.OpenAI")
def test_mantle_discovery_keeps_candidates_without_inferring_capabilities(mock_openai):
    mock_openai.return_value.models.list.return_value = SimpleNamespace(
        data=[
            SimpleNamespace(id="Anthropic.claude-sonnet-current"),
            SimpleNamespace(id="amazon.nova-pro-v1:0"),
            SimpleNamespace(id="amazon.titan-embed-text-v2:0"),
            SimpleNamespace(id="  openai.future-model  "),
            SimpleNamespace(id="   "),
            SimpleNamespace(id=""),
            SimpleNamespace(id=123),
        ]
    )

    models = discover_bedrock_models(
        {
            "bedrock_api_key": "token",
            "bedrock_region": "ap-northeast-1",
            "bedrock_endpoint_type": "mantle_anthropic",
            "bedrock_endpoint_url": "https://bedrock-mantle.ap-northeast-1.api.aws/v1/models",
            "bedrock_discovery_endpoint_url": "https://attacker.example.com",
        }
    )

    assert [model["name"] for model in models] == [
        "Anthropic.claude-sonnet-current",
        "amazon.nova-pro-v1:0",
        "amazon.titan-embed-text-v2:0",
        "openai.future-model",
    ]
    assert all(model["model_types"] == [] for model in models)
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
def test_embedding_uses_request_scoped_bearer_client(mock_create_client, monkeypatch):
    monkeypatch.delenv("LLM_TIMEOUT_SECONDS", raising=False)
    monkeypatch.delenv("LLM_MAX_RETRIES", raising=False)
    client = MagicMock()
    mock_create_client.return_value = client
    model = BedrockEmbed(
        '{"auth_mode":"bedrock_api_key","bedrock_region":"ap-northeast-1","bedrock_api_key":"token"}',
        "amazon.titan-embed-text-v2:0",
    )
    assert model.client is client
    mock_create_client.assert_called_once_with(
        "bedrock-runtime",
        "token",
        "ap-northeast-1",
        "",
        timeout_seconds=600,
        max_attempts=5,
    )


def _bedrock_iam_role_model() -> LiteLLMBase:
    return LiteLLMBase(
        '{"auth_mode":"iam_role","bedrock_region":"ap-northeast-1","aws_role_arn":"arn:aws:iam::123456789012:role/Bedrock"}',
        "anthropic.claude-sonnet-current",
        provider=SupportedLiteLLMProvider.Bedrock,
    )


def _assumed_role_response(suffix: str, expiration: datetime) -> dict[str, object]:
    return {
        "Credentials": {
            "AccessKeyId": f"access-{suffix}",
            "SecretAccessKey": f"secret-{suffix}",
            "SessionToken": f"session-{suffix}",
            "Expiration": expiration,
        }
    }


@patch("boto3.client")
def test_chat_iam_role_reuses_unexpired_credentials_off_event_loop(mock_client):
    assume_role_threads: list[int] = []

    def assume_role(**_kwargs):
        assume_role_threads.append(threading.get_ident())
        return _assumed_role_response("one", datetime.now(timezone.utc) + timedelta(hours=1))

    mock_client.return_value.assume_role.side_effect = assume_role
    model = _bedrock_iam_role_model()

    async def construct_twice():
        event_loop_thread = threading.get_ident()
        first = await model._construct_completion_args_async([], False, False)
        second = await model._construct_completion_args_async([], False, False)
        return event_loop_thread, first, second

    event_loop_thread, first, second = asyncio.run(construct_twice())

    assert mock_client.return_value.assume_role.call_count == 1
    assert len(assume_role_threads) == 1
    assert assume_role_threads[0] != event_loop_thread
    assert first["aws_access_key_id"] == second["aws_access_key_id"] == "access-one"


@patch("boto3.client")
def test_chat_iam_role_refreshes_credentials_inside_expiry_window(mock_client):
    mock_client.return_value.assume_role.side_effect = [
        _assumed_role_response("one", datetime.fromtimestamp(2_000, timezone.utc)),
        _assumed_role_response("two", datetime.fromtimestamp(4_000, timezone.utc)),
    ]
    model = _bedrock_iam_role_model()

    async def construct_twice():
        with patch("rag.llm.chat_model.time.time", side_effect=[1_000, 1_950]):
            first = await model._construct_completion_args_async([], False, False)
            second = await model._construct_completion_args_async([], False, False)
        return first, second

    first, second = asyncio.run(construct_twice())

    assert mock_client.return_value.assume_role.call_count == 2
    assert first["aws_access_key_id"] == "access-one"
    assert second["aws_access_key_id"] == "access-two"


@patch("boto3.client")
def test_chat_iam_role_refresh_is_single_flight(mock_client):
    def assume_role(**_kwargs):
        time.sleep(0.05)
        return _assumed_role_response("one", datetime.now(timezone.utc) + timedelta(hours=1))

    mock_client.return_value.assume_role.side_effect = assume_role
    model = _bedrock_iam_role_model()

    async def construct_concurrently():
        return await asyncio.gather(*(model._construct_completion_args_async([], False, False) for _ in range(3)))

    results = asyncio.run(construct_concurrently())

    assert mock_client.return_value.assume_role.call_count == 1
    assert {result["aws_access_key_id"] for result in results} == {"access-one"}


@patch("boto3.client")
def test_chat_iam_role_failure_is_not_cached(mock_client):
    from botocore.exceptions import ClientError

    mock_client.return_value.assume_role.side_effect = ClientError(
        {"Error": {"Code": "AccessDenied", "Message": "denied"}},
        "AssumeRole",
    )
    model = _bedrock_iam_role_model()

    for _ in range(2):
        with pytest.raises(ValueError, match="Failed to assume Bedrock AWS role"):
            asyncio.run(model._construct_completion_args_async([], False, False))

    assert mock_client.return_value.assume_role.call_count == 2


@patch("rag.llm.bedrock_model_discovery.create_bedrock_bearer_client")
def test_embedding_bearer_client_uses_runtime_transport_settings(mock_create_client, monkeypatch):
    monkeypatch.setenv("LLM_TIMEOUT_SECONDS", "120")
    monkeypatch.setenv("LLM_MAX_RETRIES", "3")

    BedrockEmbed(
        '{"auth_mode":"bedrock_api_key","bedrock_region":"ap-northeast-1","bedrock_api_key":"token"}',
        "amazon.titan-embed-text-v2:0",
    )

    mock_create_client.assert_called_once_with(
        "bedrock-runtime",
        "token",
        "ap-northeast-1",
        "",
        timeout_seconds=120,
        max_attempts=3,
    )


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

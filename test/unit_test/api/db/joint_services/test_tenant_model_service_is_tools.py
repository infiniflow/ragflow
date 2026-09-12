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

from common.constants import ActiveStatusEnum, LLMType
from api.db.joint_services import tenant_model_service as tms


def _factory_infos(is_tools=True):
    return [
        {
            "name": "OpenAI-API-Compatible",
            "llm": [
                {
                    "llm_name": "qwen-test",
                    "model_type": "chat",
                    "is_tools": is_tools,
                    "max_tokens": 32768,
                }
            ],
        }
    ]


def _patch_name_lookup(monkeypatch, provider, instance, model, factory_infos):
    monkeypatch.setattr(
        tms.TenantModelProviderService,
        "get_by_tenant_id_and_provider_name",
        lambda tenant_id, provider_name: provider,
    )
    monkeypatch.setattr(
        tms.TenantModelInstanceService,
        "get_by_provider_id_and_instance_name",
        lambda provider_id, instance_name: instance,
    )
    monkeypatch.setattr(
        tms.TenantModelService,
        "get_by_provider_id_and_instance_id_and_model_type_and_model_name",
        lambda provider_id, instance_id, model_type, model_name: model,
    )
    monkeypatch.setattr(tms.settings, "FACTORY_LLM_INFOS", factory_infos)


@pytest.mark.p1
def test_is_tools_falls_back_to_factory_when_extra_and_api_key_are_plain(monkeypatch):
    provider = SimpleNamespace(id="provider-1", provider_name="OpenAI-API-Compatible")
    instance = SimpleNamespace(id="instance-1", api_key="sk-test", extra='{"base_url": "http://llm"}')
    model = SimpleNamespace(
        model_name="qwen-test",
        model_type="chat",
        status=ActiveStatusEnum.ACTIVE.value,
        extra="{}",
    )
    _patch_name_lookup(monkeypatch, provider, instance, model, _factory_infos(True))

    config = tms.get_model_config_from_provider_instance(
        "tenant-1", "chat", "qwen-test@litellm@OpenAI-API-Compatible"
    )

    assert config["is_tools"] is True


@pytest.mark.p1
def test_is_tools_prefers_model_extra_over_factory(monkeypatch):
    provider = SimpleNamespace(id="provider-1", provider_name="OpenAI-API-Compatible")
    instance = SimpleNamespace(id="instance-1", api_key="sk-test", extra="{}")
    model = SimpleNamespace(
        model_name="qwen-test",
        model_type="chat",
        status=ActiveStatusEnum.ACTIVE.value,
        extra='{"is_tools": false}',
    )
    _patch_name_lookup(monkeypatch, provider, instance, model, _factory_infos(True))

    config = tms.get_model_config_from_provider_instance(
        "tenant-1", "chat", "qwen-test@litellm@OpenAI-API-Compatible"
    )

    assert config["is_tools"] is False


@pytest.mark.p1
def test_is_tools_prefers_api_key_json_over_factory_when_extra_empty(monkeypatch):
    provider = SimpleNamespace(id="provider-1", provider_name="OpenAI-API-Compatible")
    instance = SimpleNamespace(
        id="instance-1",
        api_key='{"api_key": "sk-test", "is_tools": false}',
        extra="{}",
    )
    model = SimpleNamespace(
        model_name="qwen-test",
        model_type="chat",
        status=ActiveStatusEnum.ACTIVE.value,
        extra="{}",
    )
    _patch_name_lookup(monkeypatch, provider, instance, model, _factory_infos(True))

    config = tms.get_model_config_from_provider_instance(
        "tenant-1", "chat", "qwen-test@litellm@OpenAI-API-Compatible"
    )

    assert config["is_tools"] is False


@pytest.mark.p1
def test_get_model_config_by_id_falls_back_to_factory_is_tools(monkeypatch):
    tenant_id = "tenant-1"
    model_id = "model-1"
    model = SimpleNamespace(
        id=model_id,
        provider_id="provider-1",
        instance_id="instance-1",
        model_name="qwen-test",
        model_type=tms.calculate_model_type(LLMType.CHAT.value),
        status=ActiveStatusEnum.ACTIVE.value,
        extra="{}",
    )
    provider = SimpleNamespace(id="provider-1", tenant_id=tenant_id, provider_name="OpenAI-API-Compatible")
    instance = SimpleNamespace(id="instance-1", api_key="sk-test", extra="{}")

    monkeypatch.setattr(tms.TenantModelService, "get_by_id", lambda _id: (True, model))
    monkeypatch.setattr(tms.TenantModelProviderService, "get_by_id", lambda _id: (True, provider))
    monkeypatch.setattr(tms.TenantModelInstanceService, "get_by_id", lambda _id: (True, instance))
    monkeypatch.setattr(tms.settings, "FACTORY_LLM_INFOS", _factory_infos(True))

    config = tms.get_model_config_by_id(tenant_id, LLMType.CHAT, model_id)

    assert config["is_tools"] is True

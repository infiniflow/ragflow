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

from common.constants import ActiveStatusEnum
from api.db.joint_services import tenant_model_service as tms


@pytest.mark.p1
def test_max_tokens_falls_back_to_factory_when_model_extra_empty(monkeypatch):
    provider = SimpleNamespace(id="provider-1", provider_name="OpenAI")
    instance = SimpleNamespace(id="instance-1", api_key="sk-test", extra='{"base_url": "https://api.example"}')
    model = SimpleNamespace(
        model_name="gpt-test",
        model_type="chat",
        status=ActiveStatusEnum.ACTIVE.value,
        extra="{}",
    )

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
    monkeypatch.setattr(
        tms.settings,
        "FACTORY_LLM_INFOS",
        [
            {
                "name": "OpenAI",
                "llm": [
                    {
                        "llm_name": "gpt-test",
                        "model_type": "chat",
                        "max_tokens": 128000,
                    }
                ],
            }
        ],
    )

    config = tms.get_model_config_from_provider_instance("tenant-1", "chat", "gpt-test@default@OpenAI")

    assert config["max_tokens"] == 128000


@pytest.mark.p1
def test_max_tokens_prefers_model_extra_over_factory(monkeypatch):
    provider = SimpleNamespace(id="provider-1", provider_name="OpenAI")
    instance = SimpleNamespace(id="instance-1", api_key="sk-test", extra="{}")
    model = SimpleNamespace(
        model_name="gpt-test",
        model_type="chat",
        status=ActiveStatusEnum.ACTIVE.value,
        extra='{"max_tokens": 32000}',
    )

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
    monkeypatch.setattr(
        tms.settings,
        "FACTORY_LLM_INFOS",
        [
            {
                "name": "OpenAI",
                "llm": [
                    {
                        "llm_name": "gpt-test",
                        "model_type": "chat",
                        "max_tokens": 128000,
                    }
                ],
            }
        ],
    )

    config = tms.get_model_config_from_provider_instance("tenant-1", "chat", "gpt-test@default@OpenAI")

    assert config["max_tokens"] == 32000

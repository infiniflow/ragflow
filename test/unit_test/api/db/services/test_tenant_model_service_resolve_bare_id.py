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
"""Regression test for https://github.com/infiniflow/ragflow/issues/18398.

``knowledgebase.embd_id`` (and other ``tenant_*_id`` refs) can be stored as a
bare ``tenant_model.id`` UUID with no ``@`` in it. ``resolve_model_config``
must resolve that directly via ``get_model_config_by_id`` and must not fall
through to ``get_model_config_from_provider_instance``, which parses
``model_name@instance@provider`` via ``split_model_name`` and raises
``LookupError: Provider  not found for model <uuid>`` for a bare id.
"""

from types import SimpleNamespace

from common.constants import ActiveStatusEnum, LLMType
from api.db.joint_services import tenant_model_service as module

BARE_MODEL_ID = "ab87a19896db11f189344b8174659a17"
TENANT_ID = "tenant-1"


def test_resolve_model_config_resolves_bare_tenant_model_id_without_name_fallback(monkeypatch):
    model = SimpleNamespace(
        id=BARE_MODEL_ID,
        provider_id="provider-1",
        instance_id="instance-1",
        model_name="text-embedding-3-large",
        model_type=module.calculate_model_type(LLMType.EMBEDDING.value),
        status=ActiveStatusEnum.ACTIVE.value,
        extra="",
    )
    provider = SimpleNamespace(id="provider-1", tenant_id=TENANT_ID, provider_name="OpenAI")
    instance = SimpleNamespace(id="instance-1", api_key="sk-test", extra="")

    def get_model_by_id(model_id):
        assert model_id == BARE_MODEL_ID
        return True, model

    def get_provider_by_id(provider_id):
        assert provider_id == "provider-1"
        return True, provider

    def get_instance_by_id(instance_id):
        assert instance_id == "instance-1"
        return True, instance

    monkeypatch.setattr(module.TenantModelService, "get_by_id", get_model_by_id)
    monkeypatch.setattr(module.TenantModelProviderService, "get_by_id", get_provider_by_id)
    monkeypatch.setattr(module.TenantModelInstanceService, "get_by_id", get_instance_by_id)

    def _fail_if_called(*args, **kwargs):
        raise AssertionError("must not fall back to split_model_name/provider-name resolution for a bare id")

    monkeypatch.setattr(module.TenantModelProviderService, "get_by_tenant_id_and_provider_name", _fail_if_called)

    config = module.resolve_model_config(TENANT_ID, LLMType.EMBEDDING, BARE_MODEL_ID)

    assert config["llm_factory"] == "OpenAI"
    assert config["llm_name"] == "text-embedding-3-large"

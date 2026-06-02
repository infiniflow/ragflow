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
import importlib.util
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _StrEnum(str):
    @property
    def value(self):
        return str(self)


class _TenantModelProviderService:
    @staticmethod
    def get_by_tenant_id_and_provider_name(_tenant_id, _provider_name):
        return None


class _TenantModelInstanceService:
    @staticmethod
    def get_by_provider_id_and_api_key(_provider_id, _api_key):
        return None

    @staticmethod
    def create_instance(**_kwargs):
        return None


class _TenantModelService:
    pass


def _run(coro):
    return asyncio.run(coro)


def _load_provider_api_service(monkeypatch):
    repo_root = Path(__file__).resolve().parents[3]

    common_mod = ModuleType("common")
    common_mod.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_mod)

    constants_mod = ModuleType("common.constants")
    constants_mod.LLMType = SimpleNamespace(
        CHAT=_StrEnum("chat"),
        EMBEDDING=_StrEnum("embedding"),
        RERANK=_StrEnum("rerank"),
    )
    constants_mod.ActiveStatusEnum = SimpleNamespace(
        ACTIVE=SimpleNamespace(value="active"),
        INACTIVE=SimpleNamespace(value="inactive"),
    )
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    misc_utils_mod = ModuleType("common.misc_utils")
    misc_utils_mod.get_uuid = lambda: "generated-id"
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    settings_mod = ModuleType("common.settings")
    settings_mod.FACTORY_LLM_INFOS = []
    monkeypatch.setitem(sys.modules, "common.settings", settings_mod)

    api_mod = ModuleType("api")
    api_mod.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_mod)

    db_mod = ModuleType("api.db")
    db_mod.__path__ = [str(repo_root / "api" / "db")]
    monkeypatch.setitem(sys.modules, "api.db", db_mod)

    joint_services_mod = ModuleType("api.db.joint_services")
    joint_services_mod.__path__ = [str(repo_root / "api" / "db" / "joint_services")]
    monkeypatch.setitem(sys.modules, "api.db.joint_services", joint_services_mod)

    tenant_model_joint_mod = ModuleType("api.db.joint_services.tenant_model_service")
    tenant_model_joint_mod.get_model_config_from_provider_instance = lambda *_args, **_kwargs: None
    tenant_model_joint_mod.delete_models_by_instance_ids = lambda *_args, **_kwargs: None
    tenant_model_joint_mod.delete_instances_by_provider_ids = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "api.db.joint_services.tenant_model_service", tenant_model_joint_mod)

    services_mod = ModuleType("api.db.services")
    services_mod.__path__ = [str(repo_root / "api" / "db" / "services")]
    monkeypatch.setitem(sys.modules, "api.db.services", services_mod)

    provider_service_mod = ModuleType("api.db.services.tenant_model_provider_service")
    provider_service_mod.TenantModelProviderService = _TenantModelProviderService
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_model_provider_service", provider_service_mod)

    instance_service_mod = ModuleType("api.db.services.tenant_model_instance_service")
    instance_service_mod.TenantModelInstanceService = _TenantModelInstanceService
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_model_instance_service", instance_service_mod)

    model_service_mod = ModuleType("api.db.services.tenant_model_service")
    model_service_mod.TenantModelService = _TenantModelService
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_model_service", model_service_mod)

    rag_mod = ModuleType("rag")
    rag_mod.__path__ = [str(repo_root / "rag")]
    monkeypatch.setitem(sys.modules, "rag", rag_mod)

    rag_llm_mod = ModuleType("rag.llm")
    rag_llm_mod.EmbeddingModel = {}
    rag_llm_mod.ChatModel = {}
    rag_llm_mod.RerankModel = {}
    monkeypatch.setitem(sys.modules, "rag.llm", rag_llm_mod)

    module_path = repo_root / "api" / "apps" / "services" / "provider_api_service.py"
    spec = importlib.util.spec_from_file_location("test_provider_api_service_unit_module", module_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_create_provider_instance_skips_verification_for_empty_factory(monkeypatch):
    module = _load_provider_api_service(monkeypatch)
    module.FACTORY_LLM_INFOS = [{"name": "OpenAI-API-Compatible", "llm": []}]
    provider = SimpleNamespace(id="provider-1")

    monkeypatch.setattr(module.TenantModelProviderService, "get_by_tenant_id_and_provider_name", lambda *_args: provider)
    monkeypatch.setattr(module.TenantModelInstanceService, "get_by_provider_id_and_api_key", lambda *_args: None)

    created = {}
    monkeypatch.setattr(module.TenantModelInstanceService, "create_instance", lambda **kwargs: created.update(kwargs))

    async def _verify_should_not_run(*_args, **_kwargs):
        raise AssertionError("empty-model providers should not verify during instance creation")

    monkeypatch.setattr(module, "verify_api_key", _verify_should_not_run)

    success, msg = _run(
        module.create_provider_instance(
            "tenant-1",
            "OpenAI-API-Compatible",
            "openai-compatible",
            "sk-test",
            "https://api.example.com/v1",
            "default",
        )
    )

    assert success is True
    assert msg == "success"
    assert created["provider_id"] == "provider-1"
    assert created["instance_name"] == "openai-compatible"
    assert created["api_key"] == "sk-test"
    assert json.loads(created["extra"]) == {"base_url": "https://api.example.com/v1", "region": "default"}


@pytest.mark.p2
def test_create_provider_instance_verifies_factory_with_models(monkeypatch):
    module = _load_provider_api_service(monkeypatch)
    module.FACTORY_LLM_INFOS = [{"name": "OpenAI", "llm": [{"llm_name": "gpt-4o", "model_type": "chat"}]}]
    provider = SimpleNamespace(id="provider-1")

    monkeypatch.setattr(module.TenantModelProviderService, "get_by_tenant_id_and_provider_name", lambda *_args: provider)
    monkeypatch.setattr(module.TenantModelInstanceService, "get_by_provider_id_and_api_key", lambda *_args: None)

    created = {}
    monkeypatch.setattr(module.TenantModelInstanceService, "create_instance", lambda **kwargs: created.update(kwargs))

    verified = []

    async def _verify_api_key(*args):
        verified.append(args)
        return True, "success"

    monkeypatch.setattr(module, "verify_api_key", _verify_api_key)

    success, msg = _run(
        module.create_provider_instance(
            "tenant-1",
            "OpenAI",
            "openai-main",
            "sk-test",
            "https://api.openai.com/v1",
            "",
        )
    )

    assert success is True
    assert msg == "success"
    assert verified == [("OpenAI", "sk-test", "https://api.openai.com/v1", "")]
    assert created["instance_name"] == "openai-main"


@pytest.mark.p2
def test_verify_api_key_awaits_embedding_timeout_wrapper(monkeypatch):
    module = _load_provider_api_service(monkeypatch)
    module.FACTORY_LLM_INFOS = [
        {
            "name": "EmbeddingProvider",
            "llm": [{"llm_name": "embed-small", "model_type": "embedding"}],
        }
    ]

    class _EmbeddingModel:
        def __init__(self, api_key, llm_name, base_url=None):
            self.api_key = api_key
            self.llm_name = llm_name
            self.base_url = base_url

        def encode(self, texts):
            assert texts == ["Test if the api key is available"]
            return [[0.1, 0.2]], 2

    module.EmbeddingModel["EmbeddingProvider"] = _EmbeddingModel

    success, msg = _run(module.verify_api_key("EmbeddingProvider", "sk-test", "https://api.example.com/v1"))

    assert success is True
    assert msg == "success"

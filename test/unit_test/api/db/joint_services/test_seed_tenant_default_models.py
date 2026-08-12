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
import sys
import types
import warnings
from types import SimpleNamespace

import pytest

# xgboost imports pkg_resources and emits a deprecation warning that is promoted
# to error in our pytest configuration; ignore it for this unit test module.
warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)


def _install_cv2_stub_if_unavailable():
    try:
        import cv2  # noqa: F401

        return
    except Exception:
        pass

    stub = types.ModuleType("cv2")

    def _missing(*_args, **_kwargs):
        raise RuntimeError("cv2 runtime call is unavailable in this test environment")

    def _module_getattr(name):
        if name.isupper():
            return 0
        return _missing

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub


_install_cv2_stub_if_unavailable()

from api.db.joint_services import tenant_model_service as tms  # noqa: E402


# ---------------------------------------------------------------------------
# In-memory, call-recording stand-ins for the model-catalog services
# ---------------------------------------------------------------------------


class _ProviderStore:
    def __init__(self):
        self.by_key = {}
        self.inserts = []

    def get_by_tenant_id_and_provider_name(self, tenant_id, name):
        return self.by_key.get((tenant_id, name))

    def insert(self, **kwargs):
        self.inserts.append(kwargs)
        obj = SimpleNamespace(id=f"prov-{len(self.inserts)}", **kwargs)
        self.by_key[(kwargs["tenant_id"], kwargs["provider_name"])] = obj
        return obj


class _InstanceStore:
    def __init__(self):
        self.by_key = {}
        self.inserts = []

    def get_by_provider_id_and_instance_name(self, provider_id, name):
        return self.by_key.get((provider_id, name))

    def create_instance(self, **kwargs):
        self.inserts.append(kwargs)
        obj = SimpleNamespace(id=f"inst-{len(self.inserts)}", **kwargs)
        self.by_key[(kwargs["provider_id"], kwargs["instance_name"])] = obj
        return obj


class _ModelStore:
    def __init__(self):
        self.by_key = {}
        self.inserts = []
        self.updates = []

    def get_by_provider_id_and_instance_id_and_model_name(self, provider_id, instance_id, name):
        return self.by_key.get((provider_id, instance_id, name))

    def insert(self, **kwargs):
        self.inserts.append(kwargs)
        obj = SimpleNamespace(id=f"mdl-{len(self.inserts) + 1}", model_type=kwargs["model_type"])
        self.by_key[(kwargs["provider_id"], kwargs["instance_id"], kwargs["model_name"])] = obj
        return obj

    def update_model(self, model_id, update_dict):
        self.updates.append((model_id, update_dict))
        for obj in self.by_key.values():
            if obj.id == model_id and "model_type" in update_dict:
                obj.model_type = update_dict["model_type"]


def _model_type_bits(type_names):
    bits = {"chat": 1, "embedding": 2, "asr": 4, "vision": 8, "rerank": 16, "tts": 32, "ocr": 64}
    result = 0
    for name in type_names:
        result |= bits.get(name, 0)
    return result


def _cfg(name, factory="Tongyi-Qianwen", api_key="sk-test", base_url="https://example/v1"):
    if not name:
        return {"model": "", "factory": "", "api_key": "", "base_url": ""}
    return {"model": f"{name}@{factory}", "factory": factory, "api_key": api_key, "base_url": base_url}


@pytest.fixture()
def catalog(monkeypatch):
    provider_store = _ProviderStore()
    instance_store = _InstanceStore()
    model_store = _ModelStore()
    monkeypatch.setattr(tms, "TenantModelProviderService", provider_store)
    monkeypatch.setattr(tms, "TenantModelInstanceService", instance_store)
    monkeypatch.setattr(tms, "TenantModelService", model_store)
    monkeypatch.setattr(tms, "calculate_model_type", _model_type_bits)
    return SimpleNamespace(provider=provider_store, instance=instance_store, model=model_store)


@pytest.fixture()
def default_settings(monkeypatch):
    monkeypatch.setattr(tms.settings, "CHAT_CFG", _cfg("qwen3.7-plus"))
    monkeypatch.setattr(tms.settings, "EMBEDDING_CFG", _cfg("text-embedding-v4"))
    monkeypatch.setattr(tms.settings, "RERANK_CFG", _cfg("qwen3-rerank"))
    monkeypatch.setattr(tms.settings, "ASR_CFG", _cfg(""))
    monkeypatch.setattr(tms.settings, "VISION_CFG", _cfg(""))


@pytest.mark.p2
class TestSeedTenantDefaultModels:
    def test_seeds_provider_instance_and_models(self, catalog, default_settings):
        tms.seed_tenant_default_models("tenant-1")

        # one provider row for the configured factory
        assert len(catalog.provider.inserts) == 1
        assert catalog.provider.inserts[0] == {"tenant_id": "tenant-1", "provider_name": "Tongyi-Qianwen"}

        # one "default" instance carrying the api key and base_url
        assert len(catalog.instance.inserts) == 1
        instance = catalog.instance.inserts[0]
        assert instance["provider_id"] == catalog.provider.by_key[("tenant-1", "Tongyi-Qianwen")].id
        assert instance["instance_name"] == "default"
        assert instance["api_key"] == "sk-test"
        assert "https://example/v1" in instance["extra"]

        # one model row per configured default model, with the right type bit
        inserted = {m["model_name"]: m["model_type"] for m in catalog.model.inserts}
        assert inserted == {"qwen3.7-plus": 1, "text-embedding-v4": 2, "qwen3-rerank": 16}

    def test_is_idempotent(self, catalog, default_settings):
        tms.seed_tenant_default_models("tenant-1")
        tms.seed_tenant_default_models("tenant-1")

        assert len(catalog.provider.inserts) == 1
        assert len(catalog.instance.inserts) == 1
        assert len(catalog.model.inserts) == 3
        assert catalog.model.updates == []

    def test_skips_unconfigured_model_types(self, catalog, default_settings):
        tms.seed_tenant_default_models("tenant-1")

        # ASR / VISION are not configured -> never materialized
        names = {m["model_name"] for m in catalog.model.inserts}
        assert names == {"qwen3.7-plus", "text-embedding-v4", "qwen3-rerank"}

    def test_merges_type_bits_for_same_model_name(self, catalog, monkeypatch):
        # Two configured default model types sharing one model name: the second
        # pass must OR its type bit onto the existing row instead of duplicating.
        monkeypatch.setattr(tms.settings, "CHAT_CFG", _cfg("shared-model"))
        monkeypatch.setattr(tms.settings, "EMBEDDING_CFG", _cfg("shared-model"))
        monkeypatch.setattr(tms.settings, "RERANK_CFG", _cfg(""))
        monkeypatch.setattr(tms.settings, "ASR_CFG", _cfg(""))
        monkeypatch.setattr(tms.settings, "VISION_CFG", _cfg(""))

        tms.seed_tenant_default_models("tenant-1")

        assert len(catalog.model.inserts) == 1
        assert catalog.model.inserts[0]["model_name"] == "shared-model"
        assert catalog.model.inserts[0]["model_type"] == 1  # chat inserted first
        assert len(catalog.model.updates) == 1
        _model_id, update = catalog.model.updates[0]
        assert update["model_type"] == 1 | 2  # embedding bit merged in

    def test_seed_failure_does_not_raise(self, catalog, monkeypatch):
        # A broken service must not propagate; registration has to stay intact.
        monkeypatch.setattr(tms.settings, "CHAT_CFG", _cfg("qwen3.7-plus"))
        monkeypatch.setattr(tms.settings, "EMBEDDING_CFG", _cfg(""))
        monkeypatch.setattr(tms.settings, "RERANK_CFG", _cfg(""))
        monkeypatch.setattr(tms.settings, "ASR_CFG", _cfg(""))
        monkeypatch.setattr(tms.settings, "VISION_CFG", _cfg(""))

        def _boom(**_kwargs):
            raise RuntimeError("db down")

        monkeypatch.setattr(catalog.provider, "insert", _boom)

        # should swallow, not raise
        tms.seed_tenant_default_models("tenant-1")

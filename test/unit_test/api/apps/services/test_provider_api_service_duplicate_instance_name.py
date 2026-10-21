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
"""Regression tests for duplicate detection on provider instance names.

Instance names identify a model instance inside `model@instance@provider`, so a
name that differs from an existing one only in case is the same name as far as
anything downstream is concerned. Creating or renaming into it used to be
allowed (the storage layer silently rewrote created instances to `name(1)`),
which produced two instances that look different but resolve to the same
identifier.
"""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest

pytestmark = pytest.mark.p2

PROVIDER = "LM-Studio"
TENANT_ID = "tenant-1"
PROVIDER_ID = "provider-1"
MODEL_INFO = [{"model_name": "m1", "model_type": ["chat"], "max_tokens": 4096}]


class _Instance:
    def __init__(self, instance_id, instance_name):
        self.id = instance_id
        self.instance_name = instance_name
        self.provider_id = PROVIDER_ID
        self.extra = "{}"


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    if "." in name:
        parent_name, _, child_name = name.rpartition(".")
        parent_mod = sys.modules.get(parent_name)
        if parent_mod is not None:
            monkeypatch.setattr(parent_mod, child_name, mod, raising=False)
    return mod


def _load_service(monkeypatch, existing_instance_names):
    """Load provider_api_service with a provider owning `existing_instance_names`.

    Returns the module plus two recorders: instance names handed to
    `create_instance`, and `(id, update_dict)` pairs handed to `update_by_id`, so
    a test can assert nothing was written on rejection.
    """
    created = []
    updated = []
    instances = [_Instance(f"instance-{i}", name) for i, name in enumerate(existing_instance_names)]
    provider_obj = SimpleNamespace(id=PROVIDER_ID, provider_name=PROVIDER)

    def _get_by_id(instance_id_or_name):
        for instance_obj in instances:
            if instance_obj.id == instance_id_or_name or instance_obj.instance_name == instance_id_or_name:
                return True, instance_obj
        return False, None

    _stub(
        monkeypatch,
        "common.constants",
        LLMType=SimpleNamespace(),
        ActiveStatusEnum=SimpleNamespace(ACTIVE=SimpleNamespace(value="active")),
        ModelVerifyStatusEnum=SimpleNamespace(UNKNOWN=SimpleNamespace(value="unknown")),
    )
    _stub(monkeypatch, "common.settings", FACTORY_LLM_INFOS=[{"name": PROVIDER, "llm": [], "url": ""}])
    _stub(monkeypatch, "api.db.db_models", DB=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        resolve_model_config=lambda *_a, **_k: {},
        delete_models_by_instance_ids=lambda *_a, **_k: None,
        delete_instances_by_provider_ids=lambda *_a, **_k: None,
    )
    _stub(
        monkeypatch,
        "api.db.services.tenant_model_provider_service",
        TenantModelProviderService=SimpleNamespace(
            get_by_id=lambda _id: (False, None),
            get_by_tenant_id_and_provider_id=lambda *_a, **_k: None,
            get_by_tenant_id_and_provider_name=lambda *_a, **_k: provider_obj,
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.tenant_model_instance_service",
        TenantModelInstanceService=SimpleNamespace(
            get_all_by_provider_id=lambda _provider_id: list(instances),
            create_instance=lambda provider_id, instance_name, api_key, extra: created.append(instance_name),
            get_by_id=_get_by_id,
            get_by_provider_id_and_instance_name=lambda _provider_id, instance_name: next((obj for obj in instances if obj.instance_name == instance_name), None),
            update_by_id=lambda instance_id, update_dict: updated.append((instance_id, update_dict)),
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.tenant_model_service",
        TenantModelService=SimpleNamespace(
            get_models_by_instance_id=lambda _instance_id: [],
            delete_by_ids=lambda _ids: None,
            update_model=lambda *_a, **_k: None,
        ),
    )
    _stub(
        monkeypatch,
        "api.utils.model_utils",
        get_model_type_human=lambda *_a, **_k: "",
        calculate_model_type=lambda *_a, **_k: 0,
        normalize_model_types=lambda types: types,
    )
    _stub(
        monkeypatch,
        "rag.llm",
        ChatModel={},
        CvModel={},
        EmbeddingModel={},
        ModelMeta={},
        OcrModel={},
        RerankModel={},
        Seq2txtModel={},
        TTSModel={},
    )

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "services" / "provider_api_service.py"
    spec = importlib.util.spec_from_file_location("test_provider_api_service_duplicate_instance_name_mod", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, spec.name, module)
    spec.loader.exec_module(module)

    async def _verify_api_key(*_args, **_kwargs):
        return True, "success", {}

    monkeypatch.setattr(module, "verify_api_key", _verify_api_key)
    monkeypatch.setattr(module, "add_model_to_instance", lambda *_a, **_k: (True, ""))
    return module, created, updated


async def test_instance_name_differing_only_in_case_is_rejected(monkeypatch):
    """`myinstance` next to `MyInstance` is a duplicate: creation must not happen."""
    module, created, _ = _load_service(monkeypatch, ["MyInstance"])

    ok, msg = await module.create_provider_instance(TENANT_ID, PROVIDER, "myinstance", "sk-test", "", "", MODEL_INFO)

    assert ok is False
    assert "myinstance" in msg
    assert "MyInstance" in msg
    assert created == []


async def test_identical_instance_name_is_rejected(monkeypatch):
    """The exact same name says nothing about case, so the short message applies."""
    module, created, _ = _load_service(monkeypatch, ["MyInstance"])

    ok, msg = await module.create_provider_instance(TENANT_ID, PROVIDER, "MyInstance", "sk-test", "", "", MODEL_INFO)

    assert ok is False
    assert msg == "Instance MyInstance already exists"
    assert created == []


async def test_distinct_instance_name_is_created(monkeypatch):
    """A name that collides with nothing still goes through untouched."""
    module, created, _ = _load_service(monkeypatch, ["MyInstance"])

    ok, _msg = await module.create_provider_instance(TENANT_ID, PROVIDER, "Other", "sk-test", "", "", MODEL_INFO)

    assert ok is True
    assert created == ["Other"]


async def test_name_only_creation_rejects_case_insensitive_duplicate(monkeypatch):
    """The name-only path is the one the UI uses, and it needs the same guard."""
    module, created, _ = _load_service(monkeypatch, ["MyInstance"])

    ok, msg = await module.create_name_only_provider_instance(TENANT_ID, PROVIDER, "MYINSTANCE")

    assert ok is False
    assert "MyInstance" in msg
    assert created == []


async def test_name_only_creation_accepts_free_name(monkeypatch):
    module, created, _ = _load_service(monkeypatch, ["MyInstance"])

    ok, _msg = await module.create_name_only_provider_instance(TENANT_ID, PROVIDER, "Other")

    assert ok is True
    assert created == ["Other"]


async def test_rename_onto_another_instance_name_is_rejected(monkeypatch):
    """Renaming onto a sibling instance's name, in any case, leaves the row alone."""
    module, _, updated = _load_service(monkeypatch, ["MyInstance", "OtherInstance"])

    ok, msg = await module.update_provider_instance(TENANT_ID, PROVIDER, "MyInstance", "otherinstance", "sk-test", "", "", MODEL_INFO)

    assert ok is False
    assert "OtherInstance" in msg
    assert updated == []


async def test_rename_changing_only_own_case_is_allowed(monkeypatch):
    """An instance may re-case its own name; it is not a collision with itself."""
    module, _, updated = _load_service(monkeypatch, ["MyInstance"])

    ok, _msg = await module.update_provider_instance(TENANT_ID, PROVIDER, "MyInstance", "MYINSTANCE", "sk-test", "", "", MODEL_INFO)

    assert ok is True
    assert updated[0][1]["instance_name"] == "MYINSTANCE"


async def test_rename_to_free_name_is_allowed(monkeypatch):
    module, _, updated = _load_service(monkeypatch, ["MyInstance"])

    ok, _msg = await module.update_provider_instance(TENANT_ID, PROVIDER, "MyInstance", "Other", "sk-test", "", "", MODEL_INFO)

    assert ok is True
    assert updated[0][1]["instance_name"] == "Other"

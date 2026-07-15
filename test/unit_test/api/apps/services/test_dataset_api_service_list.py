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

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest

pytestmark = pytest.mark.p2


_STATUS_FIELDS = ["unstart_count", "running_count", "cancel_count", "done_count", "fail_count"]


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


def _load_dataset_api_service(monkeypatch, *, status_by_kb):
    repo_root = Path(__file__).resolve().parents[5]

    calls = []

    class _DocumentService:
        @staticmethod
        def get_parsing_status_by_kb_ids(kb_ids):
            calls.append(list(kb_ids))
            return status_by_kb

    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=_DocumentService,
        queue_raptor_o_graphrag_tasks=lambda **_kwargs: "task-queued",
    )
    _stub(monkeypatch, "api.db.services.file2document_service", File2DocumentService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.file_service", FileService=SimpleNamespace())

    class _KnowledgebaseService:
        @staticmethod
        def get_kb_by_id(_kb_id, _tenant_id):
            return []

        @staticmethod
        def get_kb_by_name(_name, _tenant_id):
            return []

        @staticmethod
        def get_list(*_args, **_kwargs):
            return [
                {"id": "kb-1", "tenant_id": "tenant-1", "name": "alpha"},
                {"id": "kb-2", "tenant_id": "tenant-1", "name": "beta"},
            ], 2

    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=_KnowledgebaseService,
        validate_dataset_embedding_models=lambda _kbs: None,
    )
    _stub(monkeypatch, "api.db.services.connector_service", Connector2KbService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.task_service", GRAPH_RAPTOR_FAKE_DOC_ID="fake-doc", TaskService=SimpleNamespace())

    class _TenantService:
        @staticmethod
        def get_joined_tenants_by_user_id(_tenant_id):
            return [{"tenant_id": "tenant-1"}]

    class _UserService:
        @staticmethod
        def get_by_ids(_ids):
            return []

    _stub(
        monkeypatch,
        "api.db.services.user_service",
        TenantService=_TenantService,
        UserService=_UserService,
        UserTenantService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        resolve_model_config=lambda *_args, **_kwargs: None,
        resolve_model_id=lambda *_args, **_kwargs: None,
    )
    _stub(
        monkeypatch,
        "api.db.services.tenant_model_service",
        TenantModelService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.db_models",
        File=SimpleNamespace(source_type="source_type", id="id", type="type", name="name"),
    )
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        deep_merge=lambda base, updates: {**base, **updates},
        get_parser_config=lambda *_args, **_kwargs: {},
        remap_dictionary_keys=lambda data: data,
        verify_embedding_availability=lambda *_args, **_kwargs: (True, None),
    )
    _stub(
        monkeypatch,
        "common.constants",
        PAGERANK_FLD="pagerank",
        LLMType=SimpleNamespace(EMBEDDING="embedding"),
        FileSource=SimpleNamespace(KNOWLEDGEBASE="knowledgebase"),
        StatusEnum=SimpleNamespace(VALID=SimpleNamespace(value="1")),
    )
    _stub(monkeypatch, "common.settings", docStoreConn=SimpleNamespace())

    module_path = repo_root / "api" / "apps" / "services" / "dataset_api_service.py"
    spec = importlib.util.spec_from_file_location("test_dataset_api_service_list_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_dataset_api_service_list_module", module)
    spec.loader.exec_module(module)
    return module, calls


def test_list_datasets_merges_parsing_status_when_requested(monkeypatch):
    module, calls = _load_dataset_api_service(
        monkeypatch,
        status_by_kb={
            "kb-1": {
                "unstart_count": 1,
                "running_count": 2,
                "cancel_count": 3,
                "done_count": 4,
                "fail_count": 5,
            }
        },
    )

    ok, result = module.list_datasets(
        "tenant-1",
        {
            "name": "",
            "page": 1,
            "page_size": 30,
            "orderby": "create_time",
            "desc": True,
            "include_parsing_status": True,
            "ext": {},
        },
    )

    assert ok is True
    assert calls == [["kb-1", "kb-2"]]
    first, second = result["data"]
    assert {field: first[field] for field in _STATUS_FIELDS} == {
        "unstart_count": 1,
        "running_count": 2,
        "cancel_count": 3,
        "done_count": 4,
        "fail_count": 5,
    }
    assert {field: second[field] for field in _STATUS_FIELDS} == {field: 0 for field in _STATUS_FIELDS}

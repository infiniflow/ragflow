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
"""Regression tests for dataset navigation tree service helpers (#17301)."""

import importlib.util
import json
import sys
from enum import IntEnum
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import pytest


pytestmark = pytest.mark.p2


class _StubModelTypeBinary(IntEnum):
    CHAT = 1
    EMBEDDING = 2


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


def _load_nav_module(monkeypatch, *, accessible=True, index_pack=("idx-1", None), total=0, field_map=None):
    doc_store = MagicMock()
    doc_store.search = MagicMock(return_value={})
    doc_store.get_fields = MagicMock(return_value=field_map or {})
    doc_store.get_total = MagicMock(return_value=total)
    doc_store.delete = MagicMock(return_value=0)

    kb = SimpleNamespace(tenant_id="tenant-1", id="kb-1")
    knowledgebase_service = SimpleNamespace(
        accessible=MagicMock(return_value=accessible),
        get_by_id=MagicMock(return_value=(True, kb)),
    )

    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_composite_model_name_by_ids=MagicMock(),
        resolve_model_config=MagicMock(),
        resolve_model_id=MagicMock(),
    )
    _stub(
        monkeypatch,
        "common.constants",
        PAGERANK_FLD="pagerank",
        LLMType=SimpleNamespace(),
        FileSource=SimpleNamespace(KNOWLEDGEBASE="knowledgebase"),
        RetCode=SimpleNamespace(),
        StatusEnum=SimpleNamespace(),
        TaskStatus=SimpleNamespace(),
        ModelTypeBinary=_StubModelTypeBinary,
    )
    _stub(monkeypatch, "common.settings", docStoreConn=doc_store, DOC_ENGINE="infinity")
    _stub(
        monkeypatch,
        "api.db.db_models",
        Connector2Kb=SimpleNamespace(),
        Document=SimpleNamespace(),
        File=SimpleNamespace(),
        SyncLogs=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(get_parsing_status_by_kb_ids=MagicMock()),
        queue_raptor_o_graphrag_tasks=MagicMock(),
    )
    _stub(monkeypatch, "api.db.services.file2document_service", File2DocumentService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.file_service", FileService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=knowledgebase_service,
        validate_dataset_embedding_models=lambda kbs: None,
    )
    _stub(
        monkeypatch,
        "api.db.services.connector_service",
        Connector2KbService=SimpleNamespace(),
        SyncLogsService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.services.task_service",
        GRAPH_RAPTOR_FAKE_DOC_ID="fake-doc",
        TaskService=SimpleNamespace(),
    )
    _stub(monkeypatch, "api.db.services.tenant_model_service", TenantModelService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.user_service",
        TenantService=SimpleNamespace(get_joined_tenants_by_user_id=lambda user_id: [{"tenant_id": "tenant-1"}]),
        UserService=SimpleNamespace(get_by_ids=lambda ids: []),
        UserTenantService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        deep_merge=MagicMock(),
        get_parser_config=MagicMock(),
        remap_dictionary_keys=lambda source_data, key_aliases=None: dict(source_data),
        verify_embedding_availability=MagicMock(),
    )
    _stub(
        monkeypatch,
        "common.misc_utils",
        thread_pool_exec=AsyncMock(side_effect=lambda fn, *args, **kwargs: fn(*args, **kwargs)),
        thread_pool_exec_long_time=AsyncMock(side_effect=lambda fn, *args, **kwargs: fn(*args, **kwargs)),
    )
    _stub(monkeypatch, "rag.advanced_rag.knowlege_compile.wiki", WIKI_PAGE_COMPILE_KWD="wiki")
    _stub(monkeypatch, "common.doc_store.doc_store_base", OrderByExpr=MagicMock)

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "services" / "dataset_api_service.py"
    spec = importlib.util.spec_from_file_location("test_dataset_api_service_nav_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_dataset_api_service_nav_module", module)
    spec.loader.exec_module(module)
    monkeypatch.setattr(module, "_compiled_index_or_none", lambda _tenant_id, _kb_id: index_pack)
    return module, knowledgebase_service, doc_store


def test_nav_item_shapes_cluster_row(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    row = {
        "name": "cluster-a",
        "type_kwd": "nav_cluster",
        "content_with_weight": json.dumps({"description": "Top cluster"}),
        "doc_count_int": 3,
    }
    item = module._nav_item(row)
    assert item == {
        "name": "cluster-a",
        "description": "Top cluster",
        "keywords": [],
        "entities": [],
        "graph_content": "",
        "doc_count": 3,
        "type": "cluster",
        "doc_id": None,
        "has_children": True,
    }


def test_nav_item_shapes_doc_leaf_row(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    row = {
        "name": "doc-leaf",
        "type_kwd": "nav_doc",
        "content_with_weight": "{}",
        "doc_id": "doc-123",
    }
    item = module._nav_item(row)
    assert item["type"] == "doc"
    assert item["doc_id"] == "doc-123"
    assert item["doc_count"] == 1
    assert item["has_children"] is False


@pytest.mark.asyncio
async def test_list_nav_clusters_returns_empty_when_index_missing(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch, index_pack=None)
    ok, payload = await module.list_nav_clusters("kb-1", "tenant-1")
    assert ok is True
    assert payload == {"total": 0, "items": []}


@pytest.mark.asyncio
async def test_list_nav_clusters_denies_inaccessible_dataset(monkeypatch):
    module, knowledgebase_service, doc_store = _load_nav_module(monkeypatch, accessible=False)
    ok, payload = await module.list_nav_clusters("kb-1", "tenant-1")
    assert ok is False
    assert payload == "no authorization"
    doc_store.search.assert_not_called()
    knowledgebase_service.get_by_id.assert_not_called()


@pytest.mark.asyncio
async def test_list_nav_children_uses_parent_name_filter(monkeypatch):
    module, _, doc_store = _load_nav_module(
        monkeypatch,
        total=1,
        field_map={
            "row-1": {
                "name": "child-a",
                "type_kwd": "nav_doc",
                "content_with_weight": "{}",
                "doc_id": "doc-1",
            }
        },
    )

    ok, payload = await module.list_nav_children("kb-1", "tenant-1", "cluster-a")
    assert ok is True
    assert payload["total"] == 1
    assert payload["items"][0]["name"] == "child-a"
    call_kwargs = doc_store.search.call_args.kwargs
    condition = call_kwargs.get("condition") or doc_store.search.call_args.args[2]
    assert condition["parent_kwd"] == ["cluster-a"]


@pytest.mark.asyncio
async def test_delete_nav_returns_zero_when_index_missing(monkeypatch):
    module, _, doc_store = _load_nav_module(monkeypatch, index_pack=None)
    ok, payload = await module.delete_nav("kb-1", "tenant-1")
    assert ok is True
    assert payload == {"deleted": 0}
    doc_store.delete.assert_not_called()

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
import inspect
import json
import sys
from copy import deepcopy
from datetime import datetime
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _AwaitableValue:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _co():
            return self._value

        return _co().__await__()


class _DummyArgs(dict):
    def getlist(self, key):
        value = self.get(key)
        if value is None:
            return []
        if isinstance(value, list):
            return value
        return [value]


class _DummyKB:
    def __init__(self, *, kb_id="kb-1", name="old_kb", tenant_id="tenant-1", pagerank=0):
        self.id = kb_id
        self.name = name
        self.tenant_id = tenant_id
        self.pagerank = pagerank
        self.parser_config = {}

    def to_dict(self):
        return {
            "id": self.id,
            "name": self.name,
            "tenant_id": self.tenant_id,
            "pagerank": self.pagerank,
            "parser_config": deepcopy(self.parser_config),
        }


class _DummyTask:
    def __init__(self, task_id, progress):
        self.id = task_id
        self.progress = progress

    def to_dict(self):
        return {"id": self.id, "progress": self.progress}


def _run(coro):
    return asyncio.run(coro)


def _unwrap_route(func):
    route_func = inspect.unwrap(func)
    visited = set()
    while getattr(route_func, "__closure__", None) and route_func not in visited:
        visited.add(route_func)
        nested = None
        for cell in route_func.__closure__:
            candidate = cell.cell_contents
            if inspect.isfunction(candidate) and candidate is not route_func:
                nested = inspect.unwrap(candidate)
                break
        if nested is None:
            break
        route_func = nested
    return route_func


def _load_kb_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    deepdoc_pkg = ModuleType("deepdoc")
    deepdoc_parser_pkg = ModuleType("deepdoc.parser")
    deepdoc_parser_pkg.__path__ = []

    class _StubPdfParser:
        pass

    class _StubExcelParser:
        pass

    class _StubDocxParser:
        pass

    deepdoc_parser_pkg.PdfParser = _StubPdfParser
    deepdoc_parser_pkg.ExcelParser = _StubExcelParser
    deepdoc_parser_pkg.DocxParser = _StubDocxParser
    deepdoc_pkg.parser = deepdoc_parser_pkg
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_pkg)
    monkeypatch.setitem(sys.modules, "deepdoc.parser", deepdoc_parser_pkg)

    deepdoc_excel_module = ModuleType("deepdoc.parser.excel_parser")
    deepdoc_excel_module.RAGFlowExcelParser = _StubExcelParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.excel_parser", deepdoc_excel_module)

    deepdoc_parser_utils = ModuleType("deepdoc.parser.utils")
    deepdoc_parser_utils.get_text = lambda *_args, **_kwargs: ""
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", deepdoc_parser_utils)
    monkeypatch.setitem(sys.modules, "xgboost", ModuleType("xgboost"))

    apps_mod = ModuleType("api.apps")
    apps_mod.current_user = SimpleNamespace(id="user-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    module_name = "test_kb_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "kb_app.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _set_request_json(monkeypatch, module, payload):
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(deepcopy(payload)))


def _set_request_args(monkeypatch, module, args):
    monkeypatch.setattr(module, "request", SimpleNamespace(args=_DummyArgs(args)))


def _base_update_payload(**kwargs):
    payload = {"kb_id": "kb-1", "name": "new_kb", "description": "", "parser_id": "naive"}
    payload.update(kwargs)
    return payload


@pytest.mark.p2
def test_create_branches(monkeypatch):
    module = _load_kb_module(monkeypatch)

    _set_request_json(monkeypatch, module, {"name": "early"})
    monkeypatch.setattr(module.KnowledgebaseService, "create_with_name", lambda **_kwargs: (False, {"code": 777, "message": "early"}))
    res = _run(inspect.unwrap(module.create)())
    assert res["code"] == 777, res

    _set_request_json(monkeypatch, module, {"name": "save-fail"})
    monkeypatch.setattr(module.KnowledgebaseService, "create_with_name", lambda **_kwargs: (True, {"id": "kb-1"}))
    monkeypatch.setattr(module.KnowledgebaseService, "save", lambda **_kwargs: False)
    res = _run(inspect.unwrap(module.create)())
    assert res["code"] == module.RetCode.DATA_ERROR, res

    _set_request_json(monkeypatch, module, {"name": "save-ok"})
    monkeypatch.setattr(module.KnowledgebaseService, "save", lambda **_kwargs: True)
    res = _run(inspect.unwrap(module.create)())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["kb_id"] == "kb-1", res

    _set_request_json(monkeypatch, module, {"name": "save-ex"})
    def _raise_save(**_kwargs):
        raise RuntimeError("save boom")
    monkeypatch.setattr(module.KnowledgebaseService, "save", _raise_save)
    res = _run(inspect.unwrap(module.create)())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res
    assert "save boom" in res["message"], res


@pytest.mark.p2
def test_update_branches(monkeypatch):
    module = _load_kb_module(monkeypatch)
    update_route = _unwrap_route(module.update)

    _set_request_json(monkeypatch, module, _base_update_payload(name=1))
    res = _run(update_route())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "must be string" in res["message"], res

    _set_request_json(monkeypatch, module, _base_update_payload(name=" "))
    res = _run(update_route())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "can't be empty" in res["message"], res

    _set_request_json(monkeypatch, module, _base_update_payload(name="a" * 129))
    res = _run(update_route())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "large than" in res["message"], res

    monkeypatch.setattr(module.settings, "DOC_ENGINE_INFINITY", True)
    _set_request_json(monkeypatch, module, _base_update_payload(parser_id="tag"))
    res = _run(update_route())
    assert res["code"] == module.RetCode.OPERATING_ERROR, res

    _set_request_json(monkeypatch, module, _base_update_payload(pagerank=50))
    res = _run(update_route())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "pagerank" in res["message"], res

    monkeypatch.setattr(module.settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr(module.KnowledgebaseService, "accessible4deletion", lambda *_args, **_kwargs: False)
    _set_request_json(monkeypatch, module, _base_update_payload())
    res = _run(update_route())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible4deletion", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [])
    _set_request_json(monkeypatch, module, _base_update_payload())
    res = _run(update_route())
    assert res["code"] == module.RetCode.OPERATING_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **kwargs: [SimpleNamespace(id="kb-1")] if kwargs.get("created_by") else [])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
    _set_request_json(monkeypatch, module, _base_update_payload())
    res = _run(update_route())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Can't find this dataset" in res["message"], res

    kb = _DummyKB(kb_id="kb-1", name="old_name", pagerank=0)
    def _query_duplicate(**kwargs):
        if kwargs.get("created_by"):
            return [SimpleNamespace(id="kb-1")]
        if kwargs.get("name"):
            return [SimpleNamespace(id="dup")]
        return []
    monkeypatch.setattr(module.KnowledgebaseService, "query", _query_duplicate)
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, kb))
    monkeypatch.setattr(module.FileService, "filter_update", lambda *_args, **_kwargs: None)
    _set_request_json(monkeypatch, module, _base_update_payload(name="new_name"))
    res = _run(update_route())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Duplicated dataset name" in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **kwargs: [SimpleNamespace(id="kb-1")] if kwargs.get("created_by") else [])
    monkeypatch.setattr(module.KnowledgebaseService, "update_by_id", lambda *_args, **_kwargs: False)
    _set_request_json(monkeypatch, module, _base_update_payload(name="new_name", connectors=["c1"]))
    res = _run(update_route())
    assert res["code"] == module.RetCode.DATA_ERROR, res

    async def _thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_exec)
    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(update=lambda *_args, **_kwargs: True))
    monkeypatch.setattr(module.search, "index_name", lambda _tenant: "idx")
    monkeypatch.setattr(module.KnowledgebaseService, "update_by_id", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.Connector2KbService, "link_connectors", lambda *_args, **_kwargs: ["warn"])
    monkeypatch.setattr(module.logging, "error", lambda *_args, **_kwargs: None)

    kb_first = _DummyKB(kb_id="kb-1", name="old_name", pagerank=0)
    kb_second = _DummyKB(kb_id="kb-1", name="new_kb", pagerank=50)
    get_by_id_results = [(True, kb_first), (True, kb_second)]
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: get_by_id_results.pop(0))
    _set_request_json(monkeypatch, module, _base_update_payload(name="new_kb", pagerank=50, connectors=["conn-1"]))
    res = _run(update_route())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["connectors"] == ["conn-1"], res

    kb_first = _DummyKB(kb_id="kb-1", name="old_name", pagerank=50)
    kb_second = _DummyKB(kb_id="kb-1", name="new_kb", pagerank=0)
    get_by_id_results = [(True, kb_first), (True, kb_second)]
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: get_by_id_results.pop(0))
    monkeypatch.setattr(module.Connector2KbService, "link_connectors", lambda *_args, **_kwargs: [])
    _set_request_json(monkeypatch, module, _base_update_payload(name="new_kb", pagerank=0))
    res = _run(update_route())
    assert res["code"] == module.RetCode.SUCCESS, res

    kb_first = _DummyKB(kb_id="kb-1", name="old_name", pagerank=0)
    get_by_id_results = [(True, kb_first), (False, None)]
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: get_by_id_results.pop(0))
    _set_request_json(monkeypatch, module, _base_update_payload(name="new_kb"))
    res = _run(update_route())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Database error" in res["message"], res

    def _raise_query(**_kwargs):
        raise RuntimeError("update boom")
    monkeypatch.setattr(module.KnowledgebaseService, "query", _raise_query)
    _set_request_json(monkeypatch, module, _base_update_payload())
    res = _run(update_route())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res
    assert "update boom" in res["message"], res


@pytest.mark.p2
def test_update_metadata_setting_not_found(monkeypatch):
    module = _load_kb_module(monkeypatch)
    _set_request_json(monkeypatch, module, {"kb_id": "missing-kb", "metadata": {}})
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
    res = _run(inspect.unwrap(module.update_metadata_setting)())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Database error" in res["message"], res


@pytest.mark.p2
def test_detail_branches(monkeypatch):
    module = _load_kb_module(monkeypatch)

    _set_request_args(monkeypatch, module, {"kb_id": "kb-1"})
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [])
    res = inspect.unwrap(module.detail)()
    assert res["code"] == module.RetCode.OPERATING_ERROR, res

    _set_request_args(monkeypatch, module, {"kb_id": "kb-1"})
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [SimpleNamespace(id="kb-1")])
    monkeypatch.setattr(module.KnowledgebaseService, "get_detail", lambda _kb_id: None)
    res = inspect.unwrap(module.detail)()
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Can't find this dataset" in res["message"], res

    finish_at = datetime(2025, 1, 1, 12, 30, 0)
    kb_detail = {
        "id": "kb-1",
        "parser_config": {"metadata": {"x": "y"}},
        "graphrag_task_finish_at": finish_at,
        "raptor_task_finish_at": finish_at,
        "mindmap_task_finish_at": finish_at,
    }
    monkeypatch.setattr(module.KnowledgebaseService, "get_detail", lambda _kb_id: deepcopy(kb_detail))
    monkeypatch.setattr(module.DocumentService, "get_total_size_by_kb_id", lambda **_kwargs: 1024)
    monkeypatch.setattr(module.Connector2KbService, "list_connectors", lambda _kb_id: ["conn-1"])
    monkeypatch.setattr(module, "turn2jsonschema", lambda metadata: {"type": "object", "properties": metadata})
    res = inspect.unwrap(module.detail)()
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["size"] == 1024, res
    assert res["data"]["connectors"] == ["conn-1"], res
    assert isinstance(res["data"]["parser_config"]["metadata"], dict), res
    assert res["data"]["graphrag_task_finish_at"] == "2025-01-01 12:30:00", res

    def _raise_tenants(**_kwargs):
        raise RuntimeError("detail boom")
    monkeypatch.setattr(module.UserTenantService, "query", _raise_tenants)
    res = inspect.unwrap(module.detail)()
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res
    assert "detail boom" in res["message"], res


@pytest.mark.p2
def test_list_kbs_owner_ids_and_desc(monkeypatch):
    module = _load_kb_module(monkeypatch)

    _set_request_args(monkeypatch, module, {"keywords": "", "page": "1", "page_size": "2", "parser_id": "naive", "orderby": "create_time", "desc": "false"})
    _set_request_json(monkeypatch, module, {})
    monkeypatch.setattr(module.TenantService, "get_joined_tenants_by_user_id", lambda _uid: [{"tenant_id": "tenant-1"}])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_tenant_ids", lambda *_args, **_kwargs: ([{"id": "kb-1", "tenant_id": "tenant-1"}], 1))
    res = _run(inspect.unwrap(module.list_kbs)())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["total"] == 1, res

    _set_request_json(monkeypatch, module, {"owner_ids": ["tenant-1"]})
    monkeypatch.setattr(
        module.KnowledgebaseService,
        "get_by_tenant_ids",
        lambda *_args, **_kwargs: (
            [{"id": "kb-1", "tenant_id": "tenant-1"}, {"id": "kb-2", "tenant_id": "tenant-2"}],
            2,
        ),
    )
    res = _run(inspect.unwrap(module.list_kbs)())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["total"] == 1, res
    assert all(kb["tenant_id"] == "tenant-1" for kb in res["data"]["kbs"]), res

    def _raise_kb_list(*_args, **_kwargs):
        raise RuntimeError("list boom")
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_tenant_ids", _raise_kb_list)
    res = _run(inspect.unwrap(module.list_kbs)())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res
    assert "list boom" in res["message"], res


@pytest.mark.p2
def test_rm_and_rm_sync_branches(monkeypatch):
    module = _load_kb_module(monkeypatch)

    _set_request_json(monkeypatch, module, {"kb_id": "kb-1"})
    monkeypatch.setattr(module.KnowledgebaseService, "accessible4deletion", lambda *_args, **_kwargs: False)
    res = _run(inspect.unwrap(module.rm)())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible4deletion", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [])
    res = _run(inspect.unwrap(module.rm)())
    assert res["code"] == module.RetCode.OPERATING_ERROR, res

    async def _thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)
    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_exec)

    kbs = [SimpleNamespace(id="kb-1", tenant_id="tenant-1", name="kb-1")]
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: kbs)
    monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [SimpleNamespace(id="doc-1")])
    monkeypatch.setattr(module.DocumentService, "remove_document", lambda *_args, **_kwargs: False)
    res = _run(inspect.unwrap(module.rm)())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Document removal" in res["message"], res

    monkeypatch.setattr(module.DocumentService, "remove_document", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.File2DocumentService, "get_by_document_id", lambda _doc_id: [SimpleNamespace(file_id="file-1")])
    monkeypatch.setattr(module.FileService, "filter_delete", lambda *_args, **_kwargs: None)
    monkeypatch.setattr(module.File2DocumentService, "delete_by_document_id", lambda _doc_id: None)

    class _DocStore:
        def delete(self, *_args, **_kwargs):
            raise RuntimeError("drop failed")

        def delete_idx(self, *_args, **_kwargs):
            return True

    monkeypatch.setattr(module.settings, "docStoreConn", _DocStore())
    monkeypatch.setattr(module.search, "index_name", lambda _tenant_id: "idx")
    monkeypatch.setattr(module.KnowledgebaseService, "delete_by_id", lambda _kb_id: False)
    res = _run(inspect.unwrap(module.rm)())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Knowledgebase removal" in res["message"], res

    class _Storage:
        def __init__(self):
            self.removed = []

        def remove_bucket(self, kb_id):
            self.removed.append(kb_id)

    storage = _Storage()
    monkeypatch.setattr(module.settings, "STORAGE_IMPL", storage)

    class _GoodDocStore:
        def delete(self, *_args, **_kwargs):
            return True

        def delete_idx(self, *_args, **_kwargs):
            return True

    monkeypatch.setattr(module.settings, "docStoreConn", _GoodDocStore())
    monkeypatch.setattr(module.KnowledgebaseService, "delete_by_id", lambda _kb_id: True)
    res = _run(inspect.unwrap(module.rm)())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"] is True, res
    assert storage.removed == ["kb-1"], storage.removed

    def _raise_rm(**_kwargs):
        raise RuntimeError("rm boom")
    monkeypatch.setattr(module.KnowledgebaseService, "query", _raise_rm)
    res = _run(inspect.unwrap(module.rm)())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res
    assert "rm boom" in res["message"], res


@pytest.mark.p2
def test_tags_and_meta_branches(monkeypatch):
    module = _load_kb_module(monkeypatch)

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: False)
    res = inspect.unwrap(module.list_tags)("kb-1")
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.UserTenantService, "get_tenants_by_user_id", lambda _uid: [{"tenant_id": "tenant-1"}, {"tenant_id": "tenant-2"}])
    monkeypatch.setattr(module.settings, "retriever", SimpleNamespace(all_tags=lambda tenant_id, kb_ids: [f"{tenant_id}:{kb_ids[0]}"]))
    res = inspect.unwrap(module.list_tags)("kb-1")
    assert res["code"] == module.RetCode.SUCCESS, res
    assert len(res["data"]) == 2, res

    _set_request_args(monkeypatch, module, {"kb_ids": "kb-1,kb-2"})
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda kb_id, _uid: kb_id == "kb-1")
    res = inspect.unwrap(module.list_tags_from_kbs)()
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    res = inspect.unwrap(module.list_tags_from_kbs)()
    assert res["code"] == module.RetCode.SUCCESS, res
    assert isinstance(res["data"], list), res

    _set_request_json(monkeypatch, module, {"tags": ["a", "b"]})
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: False)
    res = _run(inspect.unwrap(module.rm_tags)("kb-1"))
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _DummyKB(tenant_id="tenant-1")))
    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(update=lambda *_args, **_kwargs: True))
    monkeypatch.setattr(module.search, "index_name", lambda _tenant_id: "idx")
    res = _run(inspect.unwrap(module.rm_tags)("kb-1"))
    assert res["code"] == module.RetCode.SUCCESS, res

    _set_request_json(monkeypatch, module, {"from_tag": "a", "to_tag": "b"})
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: False)
    res = _run(inspect.unwrap(module.rename_tags)("kb-1"))
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    res = _run(inspect.unwrap(module.rename_tags)("kb-1"))
    assert res["code"] == module.RetCode.SUCCESS, res

    _set_request_args(monkeypatch, module, {"kb_ids": "kb-1,kb-2"})
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda kb_id, _uid: kb_id == "kb-1")
    res = inspect.unwrap(module.get_meta)()
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda _kb_ids: {"source": ["a"]})
    res = inspect.unwrap(module.get_meta)()
    assert res["code"] == module.RetCode.SUCCESS, res
    assert "source" in res["data"], res

    _set_request_args(monkeypatch, module, {"kb_id": "kb-1"})
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: False)
    res = inspect.unwrap(module.get_basic_info)()
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.DocumentService, "knowledgebase_basic_info", lambda _kb_id: {"finished": 1})
    res = inspect.unwrap(module.get_basic_info)()
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["finished"] == 1, res


@pytest.mark.p2
def test_knowledge_graph_branches(monkeypatch):
    module = _load_kb_module(monkeypatch)

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: False)
    res = _run(inspect.unwrap(module.knowledge_graph)("kb-1"))
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _DummyKB(tenant_id="tenant-1")))
    monkeypatch.setattr(module.search, "index_name", lambda _tenant_id: "idx")
    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(index_exist=lambda *_args, **_kwargs: False))
    res = _run(inspect.unwrap(module.knowledge_graph)("kb-1"))
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"] == {"graph": {}, "mind_map": {}}, res

    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(index_exist=lambda *_args, **_kwargs: True))

    class _EmptyRetriever:
        async def search(self, *_args, **_kwargs):
            return SimpleNamespace(ids=[], field={})

    monkeypatch.setattr(module.settings, "retriever", _EmptyRetriever())
    res = _run(inspect.unwrap(module.knowledge_graph)("kb-1"))
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"] == {"graph": {}, "mind_map": {}}, res

    graph_payload = {
        "nodes": [{"id": "n2", "pagerank": 2}, {"id": "n1", "pagerank": 3}],
        "edges": [
            {"source": "n1", "target": "n2", "weight": 2},
            {"source": "n1", "target": "n1", "weight": 3},
            {"source": "n1", "target": "n3", "weight": 4},
        ],
    }

    class _GraphRetriever:
        async def search(self, *_args, **_kwargs):
            return SimpleNamespace(
                ids=["bad"],
                field={
                    "bad": {"knowledge_graph_kwd": "graph", "content_with_weight": "{bad json"},
                },
            )

    monkeypatch.setattr(module.settings, "retriever", _GraphRetriever())
    res = _run(inspect.unwrap(module.knowledge_graph)("kb-1"))
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["graph"] == {}, res

    class _GraphRetrieverSuccess:
        async def search(self, *_args, **_kwargs):
            return SimpleNamespace(
                ids=["good"],
                field={
                    "good": {"knowledge_graph_kwd": "graph", "content_with_weight": json.dumps(graph_payload)},
                },
            )

    monkeypatch.setattr(module.settings, "retriever", _GraphRetrieverSuccess())
    res = _run(inspect.unwrap(module.knowledge_graph)("kb-1"))
    assert res["code"] == module.RetCode.SUCCESS, res
    assert len(res["data"]["graph"]["nodes"]) == 2, res
    assert len(res["data"]["graph"]["edges"]) == 1, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: False)
    res = inspect.unwrap(module.delete_knowledge_graph)("kb-1")
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(delete=lambda *_args, **_kwargs: True))
    res = inspect.unwrap(module.delete_knowledge_graph)("kb-1")
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"] is True, res


@pytest.mark.p2
def test_list_pipeline_logs_validation_branches(monkeypatch):
    module = _load_kb_module(monkeypatch)

    _set_request_args(monkeypatch, module, {})
    _set_request_json(monkeypatch, module, {})
    res = _run(inspect.unwrap(module.list_pipeline_logs)())
    assert res["code"] == module.RetCode.ARGUMENT_ERROR, res
    assert "KB ID" in res["message"], res

    _set_request_args(
        monkeypatch,
        module,
        {
            "kb_id": "kb-1",
            "keywords": "k",
            "page": "1",
            "page_size": "10",
            "orderby": "create_time",
            "desc": "false",
            "create_date_from": "2025-02-01",
            "create_date_to": "2025-01-01",
        },
    )
    _set_request_json(monkeypatch, module, {})
    monkeypatch.setattr(module.PipelineOperationLogService, "get_file_logs_by_kb_id", lambda *_args, **_kwargs: ([], 0))
    res = _run(inspect.unwrap(module.list_pipeline_logs)())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["total"] == 0, res

    _set_request_args(
        monkeypatch,
        module,
        {
            "kb_id": "kb-1",
            "create_date_from": "2025-01-01",
            "create_date_to": "2025-02-01",
        },
    )
    _set_request_json(monkeypatch, module, {})
    res = _run(inspect.unwrap(module.list_pipeline_logs)())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Create data filter is abnormal." in res["message"], res


@pytest.mark.p2
def test_list_pipeline_logs_filter_and_exception_branches(monkeypatch):
    module = _load_kb_module(monkeypatch)

    _set_request_args(
        monkeypatch,
        module,
        {
            "kb_id": "kb-1",
            "page": "1",
            "page_size": "10",
            "desc": "false",
            "create_date_from": "2025-02-01",
            "create_date_to": "2025-01-01",
        },
    )

    _set_request_json(monkeypatch, module, {"operation_status": ["BAD_STATUS"]})
    res = _run(inspect.unwrap(module.list_pipeline_logs)())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "operation_status" in res["message"], res

    _set_request_json(monkeypatch, module, {"types": ["bad_type"]})
    res = _run(inspect.unwrap(module.list_pipeline_logs)())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Invalid filter conditions" in res["message"], res

    def _raise_file_logs(*_args, **_kwargs):
        raise RuntimeError("logs boom")

    _set_request_json(monkeypatch, module, {"suffix": [".txt"]})
    monkeypatch.setattr(module.PipelineOperationLogService, "get_file_logs_by_kb_id", _raise_file_logs)
    res = _run(inspect.unwrap(module.list_pipeline_logs)())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res
    assert "logs boom" in res["message"], res


@pytest.mark.p2
def test_list_pipeline_dataset_logs_branches(monkeypatch):
    module = _load_kb_module(monkeypatch)

    _set_request_args(monkeypatch, module, {})
    _set_request_json(monkeypatch, module, {})
    res = _run(inspect.unwrap(module.list_pipeline_dataset_logs)())
    assert res["code"] == module.RetCode.ARGUMENT_ERROR, res
    assert "KB ID" in res["message"], res

    _set_request_args(
        monkeypatch,
        module,
        {
            "kb_id": "kb-1",
            "desc": "false",
            "create_date_from": "2025-01-01",
            "create_date_to": "2025-02-01",
        },
    )
    _set_request_json(monkeypatch, module, {})
    res = _run(inspect.unwrap(module.list_pipeline_dataset_logs)())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Create data filter is abnormal." in res["message"], res

    _set_request_args(
        monkeypatch,
        module,
        {
            "kb_id": "kb-1",
            "page": "1",
            "page_size": "10",
            "desc": "false",
            "create_date_from": "2025-02-01",
            "create_date_to": "2025-01-01",
        },
    )
    _set_request_json(monkeypatch, module, {"operation_status": ["NOT_A_STATUS"]})
    res = _run(inspect.unwrap(module.list_pipeline_dataset_logs)())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "operation_status" in res["message"], res

    _set_request_args(
        monkeypatch,
        module,
        {
            "kb_id": "kb-1",
            "page": "1",
            "page_size": "10",
            "desc": "true",
            "create_date_from": "2025-02-01",
            "create_date_to": "2025-01-01",
        },
    )
    _set_request_json(monkeypatch, module, {"operation_status": []})
    monkeypatch.setattr(
        module.PipelineOperationLogService,
        "get_dataset_logs_by_kb_id",
        lambda *_args, **_kwargs: ([{"id": "l1"}], 1),
    )
    res = _run(inspect.unwrap(module.list_pipeline_dataset_logs)())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["total"] == 1, res
    assert res["data"]["logs"][0]["id"] == "l1", res

    def _raise_dataset_logs(*_args, **_kwargs):
        raise RuntimeError("dataset logs boom")

    monkeypatch.setattr(module.PipelineOperationLogService, "get_dataset_logs_by_kb_id", _raise_dataset_logs)
    res = _run(inspect.unwrap(module.list_pipeline_dataset_logs)())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res
    assert "dataset logs boom" in res["message"], res


@pytest.mark.p2
def test_pipeline_log_detail_and_delete_routes_branches(monkeypatch):
    module = _load_kb_module(monkeypatch)

    _set_request_args(monkeypatch, module, {})
    _set_request_json(monkeypatch, module, {})
    res = _run(inspect.unwrap(module.delete_pipeline_logs)())
    assert res["code"] == module.RetCode.ARGUMENT_ERROR, res
    assert "KB ID" in res["message"], res

    deleted_ids = []

    def _delete_by_ids(log_ids):
        deleted_ids.extend(log_ids)

    monkeypatch.setattr(module.PipelineOperationLogService, "delete_by_ids", _delete_by_ids)
    _set_request_args(monkeypatch, module, {"kb_id": "kb-1"})
    _set_request_json(monkeypatch, module, {})
    res = _run(inspect.unwrap(module.delete_pipeline_logs)())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"] is True, res
    assert deleted_ids == [], deleted_ids

    _set_request_json(monkeypatch, module, {"log_ids": ["l1", "l2"]})
    res = _run(inspect.unwrap(module.delete_pipeline_logs)())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert deleted_ids == ["l1", "l2"], deleted_ids

    _set_request_args(monkeypatch, module, {})
    res = inspect.unwrap(module.pipeline_log_detail)()
    assert res["code"] == module.RetCode.ARGUMENT_ERROR, res
    assert "Pipeline log ID" in res["message"], res

    _set_request_args(monkeypatch, module, {"log_id": "missing"})
    monkeypatch.setattr(module.PipelineOperationLogService, "get_by_id", lambda _log_id: (False, None))
    res = inspect.unwrap(module.pipeline_log_detail)()
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Invalid pipeline log ID" in res["message"], res

    class _Log:
        def to_dict(self):
            return {"id": "log-1", "status": "ok"}

    monkeypatch.setattr(module.PipelineOperationLogService, "get_by_id", lambda _log_id: (True, _Log()))
    res = inspect.unwrap(module.pipeline_log_detail)()
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["id"] == "log-1", res


@pytest.mark.p2
@pytest.mark.parametrize(
    "route_name,task_attr,response_key,task_type",
    [
        ("run_graphrag", "graphrag_task_id", "graphrag_task_id", "graphrag"),
        ("run_raptor", "raptor_task_id", "raptor_task_id", "raptor"),
        ("run_mindmap", "mindmap_task_id", "mindmap_task_id", "mindmap"),
    ],
)
def test_run_pipeline_task_routes_branch_matrix(monkeypatch, route_name, task_attr, response_key, task_type):
    module = _load_kb_module(monkeypatch)
    route = inspect.unwrap(getattr(module, route_name))

    def _make_kb(task_id):
        payload = {
            "id": "kb-1",
            "tenant_id": "tenant-1",
            "graphrag_task_id": "",
            "raptor_task_id": "",
            "mindmap_task_id": "",
        }
        payload[task_attr] = task_id
        return SimpleNamespace(**payload)

    warnings = []
    monkeypatch.setattr(module.logging, "warning", lambda msg, *_args, **_kwargs: warnings.append(msg))

    _set_request_json(monkeypatch, module, {"kb_id": ""})
    res = _run(route())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "KB ID" in res["message"], res

    _set_request_json(monkeypatch, module, {"kb_id": "kb-1"})
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
    res = _run(route())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Invalid Knowledgebase ID" in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _make_kb("task-running")))
    monkeypatch.setattr(module.TaskService, "get_by_id", lambda _task_id: (True, SimpleNamespace(progress=0)))
    res = _run(route())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "already running" in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _make_kb("task-stale")))
    monkeypatch.setattr(module.TaskService, "get_by_id", lambda _task_id: (False, None))
    monkeypatch.setattr(module.DocumentService, "get_by_kb_id", lambda **_kwargs: ([], 0))
    res = _run(route())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "No documents in Knowledgebase kb-1" in res["message"], res
    assert warnings, "Expected warning for stale task id"

    queue_calls = {}

    def _queue_stub(**kwargs):
        queue_calls.update(kwargs)
        return "queued-task-id"

    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _make_kb("")))
    monkeypatch.setattr(
        module.DocumentService,
        "get_by_kb_id",
        lambda **_kwargs: ([{"id": "doc-1"}, {"id": "doc-2"}], 2),
    )
    monkeypatch.setattr(module, "queue_raptor_o_graphrag_tasks", _queue_stub)
    monkeypatch.setattr(module.KnowledgebaseService, "update_by_id", lambda *_args, **_kwargs: False)
    res = _run(route())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"][response_key] == "queued-task-id", res
    assert queue_calls["ty"] == task_type, queue_calls
    assert queue_calls["doc_ids"] == ["doc-1", "doc-2"], queue_calls


@pytest.mark.p2
@pytest.mark.parametrize(
    "route_name,task_attr,empty_on_missing_task,error_text",
    [
        ("trace_graphrag", "graphrag_task_id", True, ""),
        ("trace_raptor", "raptor_task_id", False, "RAPTOR Task Not Found or Error Occurred"),
        ("trace_mindmap", "mindmap_task_id", False, "Mindmap Task Not Found or Error Occurred"),
    ],
)
def test_trace_pipeline_task_routes_branch_matrix(monkeypatch, route_name, task_attr, empty_on_missing_task, error_text):
    module = _load_kb_module(monkeypatch)
    route = inspect.unwrap(getattr(module, route_name))

    def _make_kb(task_id):
        payload = {
            "id": "kb-1",
            "tenant_id": "tenant-1",
            "graphrag_task_id": "",
            "raptor_task_id": "",
            "mindmap_task_id": "",
        }
        payload[task_attr] = task_id
        return SimpleNamespace(**payload)

    _set_request_args(monkeypatch, module, {"kb_id": ""})
    res = route()
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "KB ID" in res["message"], res

    _set_request_args(monkeypatch, module, {"kb_id": "kb-1"})
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
    res = route()
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Invalid Knowledgebase ID" in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _make_kb("")))
    res = route()
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"] == {}, res

    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _make_kb("task-1")))
    monkeypatch.setattr(module.TaskService, "get_by_id", lambda _task_id: (False, None))
    res = route()
    if empty_on_missing_task:
        assert res["code"] == module.RetCode.SUCCESS, res
        assert res["data"] == {}, res
    else:
        assert res["code"] == module.RetCode.DATA_ERROR, res
        assert error_text in res["message"], res

    monkeypatch.setattr(module.TaskService, "get_by_id", lambda _task_id: (True, _DummyTask("task-1", 1)))
    res = route()
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["id"] == "task-1", res


@pytest.mark.p2
def test_unbind_task_branch_matrix(monkeypatch):
    module = _load_kb_module(monkeypatch)
    route = inspect.unwrap(module.delete_kb_task)

    _set_request_args(monkeypatch, module, {"kb_id": ""})
    res = route()
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "KB ID" in res["message"], res

    _set_request_args(monkeypatch, module, {"kb_id": "missing", "pipeline_task_type": module.PipelineTaskType.GRAPH_RAG})
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
    res = route()
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"] is True, res

    kb = SimpleNamespace(
        id="kb-1",
        tenant_id="tenant-1",
        graphrag_task_id="graph-task",
        raptor_task_id="raptor-task",
        mindmap_task_id="mindmap-task",
    )
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, kb))
    _set_request_args(monkeypatch, module, {"kb_id": "kb-1", "pipeline_task_type": "unknown"})
    res = route()
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Invalid task type" in res["message"], res

    cancelled = []
    deleted = []
    update_payloads = []
    monkeypatch.setattr(module.REDIS_CONN, "set", lambda key, value: cancelled.append((key, value)))
    monkeypatch.setattr(module.search, "index_name", lambda _tenant_id: "idx")
    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(delete=lambda *args, **_kwargs: deleted.append(args)))

    def _record_update(_kb_id, payload):
        update_payloads.append((_kb_id, payload))
        return True

    monkeypatch.setattr(module.KnowledgebaseService, "update_by_id", _record_update)

    _set_request_args(monkeypatch, module, {"kb_id": "kb-1", "pipeline_task_type": module.PipelineTaskType.GRAPH_RAG})
    res = route()
    assert res["code"] == module.RetCode.SUCCESS, res

    _set_request_args(monkeypatch, module, {"kb_id": "kb-1", "pipeline_task_type": module.PipelineTaskType.RAPTOR})
    res = route()
    assert res["code"] == module.RetCode.SUCCESS, res

    _set_request_args(monkeypatch, module, {"kb_id": "kb-1", "pipeline_task_type": module.PipelineTaskType.MINDMAP})
    res = route()
    assert res["code"] == module.RetCode.SUCCESS, res

    assert ("graph-task-cancel", "x") in cancelled, cancelled
    assert ("raptor-task-cancel", "x") in cancelled, cancelled
    assert ("mindmap-task-cancel", "x") in cancelled, cancelled
    assert len(deleted) == 2, deleted
    assert any(payload.get("graphrag_task_id") == "" for _, payload in update_payloads), update_payloads
    assert any(payload.get("raptor_task_id") == "" for _, payload in update_payloads), update_payloads
    assert any(payload.get("mindmap_task_id") == "" for _, payload in update_payloads), update_payloads

    class _FlakyPipelineType:
        def __init__(self, target):
            self.target = target
            self.calls = 0

        def __eq__(self, other):
            self.calls += 1
            if self.calls == 1:
                return other == self.target
            return False

    _set_request_args(
        monkeypatch,
        module,
        {"kb_id": "kb-1", "pipeline_task_type": _FlakyPipelineType(module.PipelineTaskType.GRAPH_RAG)},
    )
    res = route()
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Internal Error: Invalid task type" in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "update_by_id", lambda *_args, **_kwargs: False)
    monkeypatch.setattr(module, "server_error_response", lambda e: module.get_json_result(code=module.RetCode.EXCEPTION_ERROR, message=str(e)))
    _set_request_args(monkeypatch, module, {"kb_id": "kb-1", "pipeline_task_type": module.PipelineTaskType.GRAPH_RAG})
    res = route()
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res
    assert "cannot delete task" in res["message"], res

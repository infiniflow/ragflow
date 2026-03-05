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
import functools
import importlib.util
import inspect
import json
import os
import sys
from copy import deepcopy
from enum import Enum
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
    def get(self, key, default=None, type=None):
        value = super().get(key, default)
        if value is None or type is None:
            return value
        try:
            return type(value)
        except (TypeError, ValueError):
            return default


class _Field:
    def __init__(self, name):
        self.name = name

    def __eq__(self, other):
        return (self.name, "==", other)


class _KB:
    def __init__(
        self,
        *,
        kb_id="kb-1",
        name="old",
        tenant_id="tenant-1",
        parser_id="naive",
        parser_config=None,
        embd_id="embd-1",
        chunk_num=0,
        pagerank=0,
        graphrag_task_id="",
        raptor_task_id="",
    ):
        self.id = kb_id
        self.name = name
        self.tenant_id = tenant_id
        self.parser_id = parser_id
        self.parser_config = parser_config or {}
        self.embd_id = embd_id
        self.chunk_num = chunk_num
        self.pagerank = pagerank
        self.graphrag_task_id = graphrag_task_id
        self.raptor_task_id = raptor_task_id

    def to_dict(self):
        return {
            "id": self.id,
            "name": self.name,
            "tenant_id": self.tenant_id,
            "parser_id": self.parser_id,
            "parser_config": deepcopy(self.parser_config),
            "embd_id": self.embd_id,
            "pagerank": self.pagerank,
        }


def _run(coro):
    return asyncio.run(coro)


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


def _set_request_args(monkeypatch, module, args):
    monkeypatch.setattr(module, "request", SimpleNamespace(args=_DummyArgs(args)))


def _patch_json_parser(monkeypatch, module, payload_state, err_state=None):
    async def _parse_json(*_args, **_kwargs):
        return deepcopy(payload_state), err_state

    monkeypatch.setattr(module, "validate_and_parse_json_request", _parse_json)


def _load_dataset_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    quart_mod = ModuleType("quart")
    quart_mod.Request = type("Request", (), {})
    quart_mod.request = SimpleNamespace(args=_DummyArgs())
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    utils_pkg = ModuleType("api.utils")
    utils_pkg.__path__ = [str(repo_root / "api" / "utils")]
    monkeypatch.setitem(sys.modules, "api.utils", utils_pkg)
    api_pkg.utils = utils_pkg

    apps_pkg = ModuleType("api.apps")
    apps_pkg.__path__ = [str(repo_root / "api" / "apps")]
    monkeypatch.setitem(sys.modules, "api.apps", apps_pkg)
    api_pkg.apps = apps_pkg

    sdk_pkg = ModuleType("api.apps.sdk")
    sdk_pkg.__path__ = [str(repo_root / "api" / "apps" / "sdk")]
    monkeypatch.setitem(sys.modules, "api.apps.sdk", sdk_pkg)
    apps_pkg.sdk = sdk_pkg

    db_pkg = ModuleType("api.db")
    db_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db", db_pkg)
    api_pkg.db = db_pkg

    db_models_mod = ModuleType("api.db.db_models")
    db_models_mod.File = SimpleNamespace(
        source_type=_Field("source_type"),
        id=_Field("id"),
        type=_Field("type"),
        name=_Field("name"),
    )
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    document_service_mod = ModuleType("api.db.services.document_service")

    class _StubDocumentService:
        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def remove_document(*_args, **_kwargs):
            return True

        @staticmethod
        def get_by_kb_id(**_kwargs):
            return [], 0

    document_service_mod.DocumentService = _StubDocumentService
    document_service_mod.queue_raptor_o_graphrag_tasks = lambda **_kwargs: "task-queued"
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", document_service_mod)
    services_pkg.document_service = document_service_mod

    file2document_service_mod = ModuleType("api.db.services.file2document_service")

    class _StubFile2DocumentService:
        @staticmethod
        def get_by_document_id(_doc_id):
            return [SimpleNamespace(file_id="file-1")]

        @staticmethod
        def delete_by_document_id(_doc_id):
            return None

    file2document_service_mod.File2DocumentService = _StubFile2DocumentService
    monkeypatch.setitem(sys.modules, "api.db.services.file2document_service", file2document_service_mod)
    services_pkg.file2document_service = file2document_service_mod

    file_service_mod = ModuleType("api.db.services.file_service")

    class _StubFileService:
        @staticmethod
        def filter_delete(_filters):
            return None

    file_service_mod.FileService = _StubFileService
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service_mod)
    services_pkg.file_service = file_service_mod

    knowledgebase_service_mod = ModuleType("api.db.services.knowledgebase_service")

    class _StubKnowledgebaseService:
        @staticmethod
        def create_with_name(**_kwargs):
            return True, {"id": "kb-1"}

        @staticmethod
        def save(**_kwargs):
            return True

        @staticmethod
        def get_by_id(_kb_id):
            return True, _KB()

        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def get_or_none(**_kwargs):
            return _KB()

        @staticmethod
        def delete_by_id(_kb_id):
            return True

        @staticmethod
        def update_by_id(_kb_id, _payload):
            return True

        @staticmethod
        def get_kb_by_id(_kb_id, _tenant_id):
            return [SimpleNamespace(id=_kb_id)]

        @staticmethod
        def get_kb_by_name(_name, _tenant_id):
            return [SimpleNamespace(name=_name)]

        @staticmethod
        def get_list(*_args, **_kwargs):
            return [], 0

        @staticmethod
        def accessible(_dataset_id, _tenant_id):
            return True

    knowledgebase_service_mod.KnowledgebaseService = _StubKnowledgebaseService
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", knowledgebase_service_mod)
    services_pkg.knowledgebase_service = knowledgebase_service_mod

    task_service_mod = ModuleType("api.db.services.task_service")

    class _StubTaskService:
        @staticmethod
        def get_by_id(_task_id):
            return False, None

    task_service_mod.GRAPH_RAPTOR_FAKE_DOC_ID = "fake-doc"
    task_service_mod.TaskService = _StubTaskService
    monkeypatch.setitem(sys.modules, "api.db.services.task_service", task_service_mod)
    services_pkg.task_service = task_service_mod

    user_service_mod = ModuleType("api.db.services.user_service")

    class _StubTenantService:
        @staticmethod
        def get_by_id(_tenant_id):
            return True, SimpleNamespace(embd_id="embd-default")

        @staticmethod
        def get_joined_tenants_by_user_id(_tenant_id):
            return [{"tenant_id": "tenant-1"}]

    user_service_mod.TenantService = _StubTenantService
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)
    services_pkg.user_service = user_service_mod

    constants_mod = ModuleType("common.constants")

    class _RetCode:
        SUCCESS = 0
        ARGUMENT_ERROR = 101
        DATA_ERROR = 102
        AUTHENTICATION_ERROR = 108

    class _FileSource:
        KNOWLEDGEBASE = "knowledgebase"

    class _StatusEnum(Enum):
        VALID = "valid"

    constants_mod.RetCode = _RetCode
    constants_mod.FileSource = _FileSource
    constants_mod.StatusEnum = _StatusEnum
    constants_mod.PAGERANK_FLD = "pagerank"
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    common_pkg.settings = SimpleNamespace(
        docStoreConn=SimpleNamespace(
            delete_idx=lambda *_args, **_kwargs: None,
            delete=lambda *_args, **_kwargs: None,
            update=lambda *_args, **_kwargs: None,
            index_exist=lambda *_args, **_kwargs: False,
        ),
        retriever=SimpleNamespace(search=lambda *_args, **_kwargs: _AwaitableValue(SimpleNamespace(ids=[], field={}))),
        STORAGE_IMPL=SimpleNamespace(),
    )
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    api_utils_mod = ModuleType("api.utils.api_utils")

    def _deep_merge(base, updates):
        merged = deepcopy(base)
        for key, value in updates.items():
            if isinstance(value, dict) and isinstance(merged.get(key), dict):
                merged[key] = _deep_merge(merged[key], value)
            else:
                merged[key] = value
        return merged

    def _get_result(*, data=None, message="", code=_RetCode.SUCCESS, total=None):
        payload = {"code": code, "data": data, "message": message}
        if total is not None:
            payload["total"] = total
        return payload

    def _get_error_argument_result(message=""):
        return _get_result(code=_RetCode.ARGUMENT_ERROR, message=message)

    def _get_error_data_result(message=""):
        return _get_result(code=_RetCode.DATA_ERROR, message=message)

    def _get_error_permission_result(message=""):
        return _get_result(code=_RetCode.AUTHENTICATION_ERROR, message=message)

    def _token_required(func):
        @functools.wraps(func)
        async def _async_wrapper(*args, **kwargs):
            return await func(*args, **kwargs)

        @functools.wraps(func)
        def _sync_wrapper(*args, **kwargs):
            return func(*args, **kwargs)

        return _async_wrapper if asyncio.iscoroutinefunction(func) else _sync_wrapper

    api_utils_mod.deep_merge = _deep_merge
    api_utils_mod.get_error_argument_result = _get_error_argument_result
    api_utils_mod.get_error_data_result = _get_error_data_result
    api_utils_mod.get_error_permission_result = _get_error_permission_result
    api_utils_mod.get_parser_config = lambda _chunk_method, _unused: {"auto": True}
    api_utils_mod.get_result = _get_result
    api_utils_mod.remap_dictionary_keys = lambda data: data
    api_utils_mod.token_required = _token_required
    api_utils_mod.verify_embedding_availability = lambda _embd_id, _tenant_id: (True, None)
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    async def _parse_json(*_args, **_kwargs):
        return {}, None

    def _parse_args(*_args, **_kwargs):
        return {"name": "", "page": 1, "page_size": 30, "orderby": "create_time", "desc": True}, None

    validation_spec = importlib.util.spec_from_file_location(
        "api.utils.validation_utils", repo_root / "api" / "utils" / "validation_utils.py"
    )
    validation_mod = importlib.util.module_from_spec(validation_spec)
    monkeypatch.setitem(sys.modules, "api.utils.validation_utils", validation_mod)
    validation_spec.loader.exec_module(validation_mod)
    validation_mod.validate_and_parse_json_request = _parse_json
    validation_mod.validate_and_parse_request_args = _parse_args

    rag_pkg = ModuleType("rag")
    rag_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag", rag_pkg)

    rag_nlp_pkg = ModuleType("rag.nlp")
    rag_nlp_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag.nlp", rag_nlp_pkg)

    search_mod = ModuleType("rag.nlp.search")
    search_mod.index_name = lambda _tenant_id: "idx"
    monkeypatch.setitem(sys.modules, "rag.nlp.search", search_mod)
    rag_nlp_pkg.search = search_mod

    module_name = "test_dataset_sdk_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "sdk" / "dataset.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_create_route_error_matrix_unit(monkeypatch):
    module = _load_dataset_module(monkeypatch)
    req_state = {"name": "kb"}
    _patch_json_parser(monkeypatch, module, req_state)

    monkeypatch.setattr(module.KnowledgebaseService, "create_with_name", lambda **_kwargs: (False, {"code": 777, "message": "early"}))
    res = _run(inspect.unwrap(module.create)("tenant-1"))
    assert res["code"] == 777, res

    monkeypatch.setattr(module.KnowledgebaseService, "create_with_name", lambda **_kwargs: (True, {"id": "kb-1"}))
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tenant_id: (False, None))
    res = _run(inspect.unwrap(module.create)("tenant-1"))
    assert res["message"] == "Tenant not found", res

    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tenant_id: (True, SimpleNamespace(embd_id="embd-1")))
    monkeypatch.setattr(module.KnowledgebaseService, "save", lambda **_kwargs: False)
    res = _run(inspect.unwrap(module.create)("tenant-1"))
    assert res["code"] == module.RetCode.DATA_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "save", lambda **_kwargs: True)
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
    res = _run(inspect.unwrap(module.create)("tenant-1"))
    assert "Dataset created failed" in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "save", lambda **_kwargs: (_ for _ in ()).throw(RuntimeError("save boom")))
    res = _run(inspect.unwrap(module.create)("tenant-1"))
    assert res["message"] == "Database operation failed", res


@pytest.mark.p2
def test_delete_route_error_summary_matrix_unit(monkeypatch):
    module = _load_dataset_module(monkeypatch)
    req_state = {"ids": ["kb-1"]}
    _patch_json_parser(monkeypatch, module, req_state)

    kb = _KB(kb_id="kb-1", name="kb-1", tenant_id="tenant-1")
    monkeypatch.setattr(module.KnowledgebaseService, "get_or_none", lambda **_kwargs: kb)
    monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [SimpleNamespace(id="doc-1")])
    monkeypatch.setattr(module.DocumentService, "remove_document", lambda *_args, **_kwargs: False)
    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(delete_idx=lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("drop failed"))))
    monkeypatch.setattr(module.KnowledgebaseService, "delete_by_id", lambda _kb_id: False)
    res = _run(inspect.unwrap(module.delete)("tenant-1"))
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Successfully deleted 0 datasets" in res["message"], res

    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(delete_idx=lambda *_args, **_kwargs: None))
    monkeypatch.setattr(module.KnowledgebaseService, "delete_by_id", lambda _kb_id: True)
    res = _run(inspect.unwrap(module.delete)("tenant-1"))
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["success_count"] == 1, res
    assert res["data"]["errors"], res

    req_state["ids"] = None
    monkeypatch.setattr(
        module.KnowledgebaseService,
        "query",
        lambda **_kwargs: (_ for _ in ()).throw(module.OperationalError("db down")),
    )
    res = _run(inspect.unwrap(module.delete)("tenant-1"))
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert res["message"] == "Database operation failed", res


@pytest.mark.p2
def test_update_route_branch_matrix_unit(monkeypatch):
    module = _load_dataset_module(monkeypatch)
    req_state = {"name": "new"}
    _patch_json_parser(monkeypatch, module, req_state)

    monkeypatch.setattr(module.KnowledgebaseService, "get_or_none", lambda **_kwargs: None)
    res = _run(inspect.unwrap(module.update)("tenant-1", "kb-1"))
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    kb = _KB(kb_id="kb-1", name="old", chunk_num=0)

    def _get_or_none_duplicate(**kwargs):
        if kwargs.get("id"):
            return kb
        if kwargs.get("name"):
            return SimpleNamespace(id="dup")
        return None

    monkeypatch.setattr(module.KnowledgebaseService, "get_or_none", _get_or_none_duplicate)
    req_state.clear()
    req_state.update({"name": "new"})
    res = _run(inspect.unwrap(module.update)("tenant-1", "kb-1"))
    assert "already exists" in res["message"], res

    kb_chunked = _KB(kb_id="kb-1", name="old", chunk_num=2, embd_id="embd-1")
    monkeypatch.setattr(module.KnowledgebaseService, "get_or_none", lambda **kwargs: kb_chunked if kwargs.get("id") else None)
    req_state.clear()
    req_state.update({"embd_id": "embd-2"})
    res = _run(inspect.unwrap(module.update)("tenant-1", "kb-1"))
    assert "chunk_num" in res["message"], res

    kb_rank = _KB(kb_id="kb-1", name="old", pagerank=0)
    monkeypatch.setattr(module.KnowledgebaseService, "get_or_none", lambda **kwargs: kb_rank if kwargs.get("id") else None)
    req_state.clear()
    req_state.update({"pagerank": 3})
    os.environ["DOC_ENGINE"] = "infinity"
    res = _run(inspect.unwrap(module.update)("tenant-1", "kb-1"))
    assert "doc_engine" in res["message"], res
    os.environ.pop("DOC_ENGINE", None)

    update_calls = []
    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(update=lambda *args, **_kwargs: update_calls.append(args)))
    monkeypatch.setattr(module.KnowledgebaseService, "update_by_id", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _KB(kb_id="kb-1", pagerank=3)))

    req_state.clear()
    req_state.update({"pagerank": 3})
    res = _run(inspect.unwrap(module.update)("tenant-1", "kb-1"))
    assert res["code"] == module.RetCode.SUCCESS, res
    assert update_calls and update_calls[-1][0] == {"kb_id": "kb-1"}, update_calls

    update_calls.clear()
    monkeypatch.setattr(module.KnowledgebaseService, "get_or_none", lambda **kwargs: _KB(kb_id="kb-1", pagerank=3) if kwargs.get("id") else None)
    req_state.clear()
    req_state.update({"pagerank": 0})
    res = _run(inspect.unwrap(module.update)("tenant-1", "kb-1"))
    assert res["code"] == module.RetCode.SUCCESS, res
    assert update_calls and update_calls[-1][0] == {"exists": module.PAGERANK_FLD}, update_calls

    monkeypatch.setattr(module.KnowledgebaseService, "update_by_id", lambda *_args, **_kwargs: False)
    req_state.clear()
    req_state.update({"description": "changed"})
    res = _run(inspect.unwrap(module.update)("tenant-1", "kb-1"))
    assert "Update dataset error" in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "update_by_id", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
    res = _run(inspect.unwrap(module.update)("tenant-1", "kb-1"))
    assert "Dataset created failed" in res["message"], res

    monkeypatch.setattr(
        module.KnowledgebaseService,
        "get_or_none",
        lambda **_kwargs: (_ for _ in ()).throw(module.OperationalError("update down")),
    )
    res = _run(inspect.unwrap(module.update)("tenant-1", "kb-1"))
    assert res["message"] == "Database operation failed", res


@pytest.mark.p2
def test_list_knowledge_graph_delete_kg_matrix_unit(monkeypatch):
    module = _load_dataset_module(monkeypatch)

    _set_request_args(monkeypatch, module, {"id": "", "name": "", "page": 1, "page_size": 30, "orderby": "create_time", "desc": True})
    monkeypatch.setattr(
        module,
        "validate_and_parse_request_args",
        lambda *_args, **_kwargs: ({"name": "", "page": 1, "page_size": 30, "orderby": "create_time", "desc": True}, None),
    )
    monkeypatch.setattr(
        module.KnowledgebaseService,
        "get_list",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(module.OperationalError("list down")),
    )
    res = module.list_datasets("tenant-1")
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert res["message"] == "Database operation failed", res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: False)
    res = _run(inspect.unwrap(module.knowledge_graph)("tenant-1", "kb-1"))
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _KB(tenant_id="tenant-1")))
    monkeypatch.setattr(module.search, "index_name", lambda _tenant_id: "idx")
    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(index_exist=lambda *_args, **_kwargs: False))
    res = _run(inspect.unwrap(module.knowledge_graph)("tenant-1", "kb-1"))
    assert res["data"] == {"graph": {}, "mind_map": {}}, res

    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(index_exist=lambda *_args, **_kwargs: True))

    class _EmptyRetriever:
        async def search(self, *_args, **_kwargs):
            return SimpleNamespace(ids=[], field={})

    monkeypatch.setattr(module.settings, "retriever", _EmptyRetriever())
    res = _run(inspect.unwrap(module.knowledge_graph)("tenant-1", "kb-1"))
    assert res["data"] == {"graph": {}, "mind_map": {}}, res

    class _BadRetriever:
        async def search(self, *_args, **_kwargs):
            return SimpleNamespace(ids=["bad"], field={"bad": {"knowledge_graph_kwd": "graph", "content_with_weight": "{bad"}})

    monkeypatch.setattr(module.settings, "retriever", _BadRetriever())
    res = _run(inspect.unwrap(module.knowledge_graph)("tenant-1", "kb-1"))
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["graph"] == {}, res

    payload = {
        "nodes": [{"id": "n2", "pagerank": 2}, {"id": "n1", "pagerank": 5}],
        "edges": [
            {"source": "n1", "target": "n2", "weight": 2},
            {"source": "n1", "target": "n1", "weight": 10},
            {"source": "n1", "target": "n3", "weight": 9},
        ],
    }

    class _GoodRetriever:
        async def search(self, *_args, **_kwargs):
            return SimpleNamespace(ids=["good"], field={"good": {"knowledge_graph_kwd": "graph", "content_with_weight": json.dumps(payload)}})

    monkeypatch.setattr(module.settings, "retriever", _GoodRetriever())
    res = _run(inspect.unwrap(module.knowledge_graph)("tenant-1", "kb-1"))
    assert res["code"] == module.RetCode.SUCCESS, res
    assert len(res["data"]["graph"]["nodes"]) == 2, res
    assert len(res["data"]["graph"]["edges"]) == 1, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: False)
    res = inspect.unwrap(module.delete_knowledge_graph)("tenant-1", "kb-1")
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res


@pytest.mark.p2
def test_run_trace_graphrag_matrix_unit(monkeypatch):
    module = _load_dataset_module(monkeypatch)

    warnings = []
    monkeypatch.setattr(module.logging, "warning", lambda msg, *_args, **_kwargs: warnings.append(msg))

    res = inspect.unwrap(module.run_graphrag)("tenant-1", "")
    assert 'Dataset ID' in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: False)
    res = inspect.unwrap(module.run_graphrag)("tenant-1", "kb-1")
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
    res = inspect.unwrap(module.run_graphrag)("tenant-1", "kb-1")
    assert "Invalid Dataset ID" in res["message"], res

    stale_kb = _KB(kb_id="kb-1", graphrag_task_id="task-old")
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, stale_kb))
    monkeypatch.setattr(module.TaskService, "get_by_id", lambda _task_id: (False, None))
    monkeypatch.setattr(module.DocumentService, "get_by_kb_id", lambda **_kwargs: ([{"id": "doc-1"}], 1))
    monkeypatch.setattr(module, "queue_raptor_o_graphrag_tasks", lambda **_kwargs: "task-new")
    monkeypatch.setattr(module.KnowledgebaseService, "update_by_id", lambda *_args, **_kwargs: True)
    res = inspect.unwrap(module.run_graphrag)("tenant-1", "kb-1")
    assert res["code"] == module.RetCode.SUCCESS, res
    assert any("GraphRAG" in msg for msg in warnings), warnings

    monkeypatch.setattr(module.TaskService, "get_by_id", lambda _task_id: (True, SimpleNamespace(progress=0)))
    res = inspect.unwrap(module.run_graphrag)("tenant-1", "kb-1")
    assert "already running" in res["message"], res

    warnings.clear()
    queue_calls = {}
    no_task_kb = _KB(kb_id="kb-1", graphrag_task_id="")
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, no_task_kb))
    monkeypatch.setattr(module.TaskService, "get_by_id", lambda _task_id: (False, None))
    monkeypatch.setattr(module.DocumentService, "get_by_kb_id", lambda **_kwargs: ([{"id": "doc-1"}, {"id": "doc-2"}], 2))

    def _queue(**kwargs):
        queue_calls.update(kwargs)
        return "queued-id"

    monkeypatch.setattr(module, "queue_raptor_o_graphrag_tasks", _queue)
    monkeypatch.setattr(module.KnowledgebaseService, "update_by_id", lambda *_args, **_kwargs: False)
    res = inspect.unwrap(module.run_graphrag)("tenant-1", "kb-1")
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["graphrag_task_id"] == "queued-id", res
    assert queue_calls["doc_ids"] == ["doc-1", "doc-2"], queue_calls
    assert any("Cannot save graphrag_task_id" in msg for msg in warnings), warnings

    res = inspect.unwrap(module.trace_graphrag)("tenant-1", "")
    assert 'Dataset ID' in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: False)
    res = inspect.unwrap(module.trace_graphrag)("tenant-1", "kb-1")
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
    res = inspect.unwrap(module.trace_graphrag)("tenant-1", "kb-1")
    assert "Invalid Dataset ID" in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _KB(kb_id="kb-1", graphrag_task_id="task-1")))
    monkeypatch.setattr(module.TaskService, "get_by_id", lambda _task_id: (False, None))
    res = inspect.unwrap(module.trace_graphrag)("tenant-1", "kb-1")
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"] == {}, res

    monkeypatch.setattr(module.TaskService, "get_by_id", lambda _task_id: (True, SimpleNamespace(to_dict=lambda: {"id": _task_id, "progress": 1})))
    res = inspect.unwrap(module.trace_graphrag)("tenant-1", "kb-1")
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["id"] == "task-1", res


@pytest.mark.p2
def test_run_trace_raptor_matrix_unit(monkeypatch):
    module = _load_dataset_module(monkeypatch)

    warnings = []
    monkeypatch.setattr(module.logging, "warning", lambda msg, *_args, **_kwargs: warnings.append(msg))

    res = inspect.unwrap(module.run_raptor)("tenant-1", "")
    assert 'Dataset ID' in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: False)
    res = inspect.unwrap(module.run_raptor)("tenant-1", "kb-1")
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
    res = inspect.unwrap(module.run_raptor)("tenant-1", "kb-1")
    assert "Invalid Dataset ID" in res["message"], res

    stale_kb = _KB(kb_id="kb-1", raptor_task_id="task-old")
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, stale_kb))
    monkeypatch.setattr(module.TaskService, "get_by_id", lambda _task_id: (False, None))
    monkeypatch.setattr(module.DocumentService, "get_by_kb_id", lambda **_kwargs: ([{"id": "doc-1"}], 1))
    monkeypatch.setattr(module, "queue_raptor_o_graphrag_tasks", lambda **_kwargs: "task-new")
    monkeypatch.setattr(module.KnowledgebaseService, "update_by_id", lambda *_args, **_kwargs: True)
    res = inspect.unwrap(module.run_raptor)("tenant-1", "kb-1")
    assert res["code"] == module.RetCode.SUCCESS, res
    assert any("RAPTOR" in msg for msg in warnings), warnings

    monkeypatch.setattr(module.TaskService, "get_by_id", lambda _task_id: (True, SimpleNamespace(progress=0)))
    res = inspect.unwrap(module.run_raptor)("tenant-1", "kb-1")
    assert "already running" in res["message"], res

    warnings.clear()
    no_task_kb = _KB(kb_id="kb-1", raptor_task_id="")
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, no_task_kb))
    monkeypatch.setattr(module.DocumentService, "get_by_kb_id", lambda **_kwargs: ([{"id": "doc-1"}], 1))
    monkeypatch.setattr(module, "queue_raptor_o_graphrag_tasks", lambda **_kwargs: "queued-raptor")
    monkeypatch.setattr(module.KnowledgebaseService, "update_by_id", lambda *_args, **_kwargs: False)
    res = inspect.unwrap(module.run_raptor)("tenant-1", "kb-1")
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["raptor_task_id"] == "queued-raptor", res
    assert any("Cannot save raptor_task_id" in msg for msg in warnings), warnings

    res = inspect.unwrap(module.trace_raptor)("tenant-1", "")
    assert 'Dataset ID' in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: False)
    res = inspect.unwrap(module.trace_raptor)("tenant-1", "kb-1")
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
    res = inspect.unwrap(module.trace_raptor)("tenant-1", "kb-1")
    assert "Invalid Dataset ID" in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _KB(kb_id="kb-1", raptor_task_id="task-1")))
    monkeypatch.setattr(module.TaskService, "get_by_id", lambda _task_id: (False, None))
    res = inspect.unwrap(module.trace_raptor)("tenant-1", "kb-1")
    assert "RAPTOR Task Not Found" in res["message"], res

    monkeypatch.setattr(module.TaskService, "get_by_id", lambda _task_id: (True, SimpleNamespace(to_dict=lambda: {"id": _task_id, "progress": -1})))
    res = inspect.unwrap(module.trace_raptor)("tenant-1", "kb-1")
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"]["id"] == "task-1", res

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


class _DummyFile:
    def __init__(self, file_id, file_type, *, name="file.txt", location="loc", size=1):
        self.id = file_id
        self.type = file_type
        self.name = name
        self.location = location
        self.size = size


class _FalsyFile(_DummyFile):
    def __bool__(self):
        return False


def _run(coro):
    return asyncio.run(coro)


def _set_request_json(monkeypatch, module, payload_state):
    async def _req_json():
        return deepcopy(payload_state)

    monkeypatch.setattr(module, "get_request_json", _req_json)


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


def _load_file2document_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="user-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)
    api_pkg.apps = apps_mod

    db_pkg = ModuleType("api.db")
    db_pkg.__path__ = []

    class _FileType(Enum):
        FOLDER = "folder"
        DOC = "doc"

    db_pkg.FileType = _FileType
    monkeypatch.setitem(sys.modules, "api.db", db_pkg)
    api_pkg.db = db_pkg

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    file2document_mod = ModuleType("api.db.services.file2document_service")

    class _StubFile2DocumentService:
        @staticmethod
        def get_by_file_id(_file_id):
            return []

        @staticmethod
        def delete_by_file_id(*_args, **_kwargs):
            return None

        @staticmethod
        def insert(_payload):
            return SimpleNamespace(to_json=lambda: {})

    file2document_mod.File2DocumentService = _StubFile2DocumentService
    monkeypatch.setitem(sys.modules, "api.db.services.file2document_service", file2document_mod)
    services_pkg.file2document_service = file2document_mod

    file_service_mod = ModuleType("api.db.services.file_service")

    class _StubFileService:
        @staticmethod
        def get_by_ids(_file_ids):
            return []

        @staticmethod
        def get_all_innermost_file_ids(_file_id, _acc):
            return []

        @staticmethod
        def get_by_id(_file_id):
            return True, _DummyFile(_file_id, _FileType.DOC.value)

    file_service_mod.FileService = _StubFileService
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service_mod)
    services_pkg.file_service = file_service_mod

    kb_service_mod = ModuleType("api.db.services.knowledgebase_service")

    class _StubKnowledgebaseService:
        @staticmethod
        def get_by_id(_kb_id):
            return False, None

    kb_service_mod.KnowledgebaseService = _StubKnowledgebaseService
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", kb_service_mod)
    services_pkg.knowledgebase_service = kb_service_mod

    document_service_mod = ModuleType("api.db.services.document_service")

    class _StubDocumentService:
        @staticmethod
        def get_by_id(doc_id):
            return True, SimpleNamespace(id=doc_id)

        @staticmethod
        def get_tenant_id(_doc_id):
            return "tenant-1"

        @staticmethod
        def remove_document(*_args, **_kwargs):
            return True

        @staticmethod
        def insert(_payload):
            return SimpleNamespace(id="doc-1")

    document_service_mod.DocumentService = _StubDocumentService
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", document_service_mod)
    services_pkg.document_service = document_service_mod

    api_utils_mod = ModuleType("api.utils.api_utils")

    def get_json_result(data=None, message="", code=0):
        return {"code": code, "data": data, "message": message}

    def get_data_error_result(message=""):
        return {"code": 102, "data": None, "message": message}

    async def get_request_json():
        return {}

    def server_error_response(err):
        return {"code": 500, "data": None, "message": str(err)}

    def validate_request(*_keys):
        def _decorator(func):
            @functools.wraps(func)
            async def _wrapper(*args, **kwargs):
                return await func(*args, **kwargs)

            return _wrapper

        return _decorator

    api_utils_mod.get_json_result = get_json_result
    api_utils_mod.get_data_error_result = get_data_error_result
    api_utils_mod.get_request_json = get_request_json
    api_utils_mod.server_error_response = server_error_response
    api_utils_mod.validate_request = validate_request
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    misc_utils_mod = ModuleType("common.misc_utils")
    misc_utils_mod.get_uuid = lambda: "uuid"
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    constants_mod = ModuleType("common.constants")

    class _RetCode:
        ARGUMENT_ERROR = 101

    constants_mod.RetCode = _RetCode
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    module_name = "test_file2document_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "file2document_app.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_convert_branch_matrix_unit(monkeypatch):
    module = _load_file2document_module(monkeypatch)
    req_state = {"kb_ids": ["kb-1"], "file_ids": ["f1"]}
    _set_request_json(monkeypatch, module, req_state)

    events = {"deleted": []}

    monkeypatch.setattr(module.FileService, "get_by_ids", lambda _ids: [_FalsyFile("f1", module.FileType.DOC.value)])
    res = _run(module.convert())
    assert res["message"] == "File not found!"

    monkeypatch.setattr(module.FileService, "get_by_ids", lambda _ids: [_DummyFile("f1", module.FileType.DOC.value)])
    monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _file_id: [SimpleNamespace(document_id="doc-1")])
    monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (False, None))
    res = _run(module.convert())
    assert res["message"] == "Document not found!"

    monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (True, SimpleNamespace(id=_doc_id)))
    monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _doc_id: None)
    res = _run(module.convert())
    assert res["message"] == "Tenant not found!"

    monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _doc_id: "tenant-1")
    monkeypatch.setattr(module.DocumentService, "remove_document", lambda *_args, **_kwargs: False)
    res = _run(module.convert())
    assert "Document removal" in res["message"]

    monkeypatch.setattr(module.DocumentService, "remove_document", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _file_id: [])
    monkeypatch.setattr(module.File2DocumentService, "delete_by_file_id", lambda file_id: events["deleted"].append(file_id))
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
    res = _run(module.convert())
    assert res["message"] == "Can't find this dataset!"
    assert events["deleted"] == ["f1"]

    kb = SimpleNamespace(id="kb-1", parser_id="naive", pipeline_id="p1", parser_config={})
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, kb))
    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
    res = _run(module.convert())
    assert res["message"] == "Can't find this file!"

    req_state["file_ids"] = ["folder-1"]
    monkeypatch.setattr(module.FileService, "get_by_ids", lambda _ids: [_DummyFile("folder-1", module.FileType.FOLDER.value, name="folder")])
    monkeypatch.setattr(module.FileService, "get_all_innermost_file_ids", lambda _file_id, _acc: ["inner-1"])
    monkeypatch.setattr(
        module.FileService,
        "get_by_id",
        lambda _file_id: (True, _DummyFile("inner-1", module.FileType.DOC.value, name="inner.txt", location="inner.loc", size=2)),
    )
    monkeypatch.setattr(module.DocumentService, "insert", lambda _payload: SimpleNamespace(id="doc-new"))
    monkeypatch.setattr(
        module.File2DocumentService,
        "insert",
        lambda _payload: SimpleNamespace(to_json=lambda: {"file_id": "inner-1", "document_id": "doc-new"}),
    )
    res = _run(module.convert())
    assert res["code"] == 0
    assert res["data"] == [{"file_id": "inner-1", "document_id": "doc-new"}]

    req_state["file_ids"] = ["f1"]
    monkeypatch.setattr(
        module.FileService,
        "get_by_ids",
        lambda _ids: (_ for _ in ()).throw(RuntimeError("convert boom")),
    )
    res = _run(module.convert())
    assert res["code"] == 500
    assert "convert boom" in res["message"]


@pytest.mark.p2
def test_rm_branch_matrix_unit(monkeypatch):
    module = _load_file2document_module(monkeypatch)
    req_state = {"file_ids": []}
    _set_request_json(monkeypatch, module, req_state)

    deleted = []

    res = _run(module.rm())
    assert res["code"] == module.RetCode.ARGUMENT_ERROR
    assert 'Lack of "Files ID"' in res["message"]

    req_state["file_ids"] = ["f1"]
    monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _file_id: [])
    res = _run(module.rm())
    assert res["message"] == "Inform not found!"

    monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _file_id: [None])
    res = _run(module.rm())
    assert res["message"] == "Inform not found!"

    monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _file_id: [SimpleNamespace(document_id="doc-1")])
    monkeypatch.setattr(module.File2DocumentService, "delete_by_file_id", lambda file_id: deleted.append(file_id))
    monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (False, None))
    res = _run(module.rm())
    assert res["message"] == "Document not found!"
    assert deleted == ["f1"]

    monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (True, SimpleNamespace(id=_doc_id)))
    monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _doc_id: None)
    res = _run(module.rm())
    assert res["message"] == "Tenant not found!"

    monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _doc_id: "tenant-1")
    monkeypatch.setattr(module.DocumentService, "remove_document", lambda *_args, **_kwargs: False)
    res = _run(module.rm())
    assert "Document removal" in res["message"]

    req_state["file_ids"] = ["f1", "f2"]
    monkeypatch.setattr(
        module.File2DocumentService,
        "get_by_file_id",
        lambda file_id: [SimpleNamespace(document_id=f"doc-{file_id}")],
    )
    monkeypatch.setattr(module.DocumentService, "get_by_id", lambda doc_id: (True, SimpleNamespace(id=doc_id)))
    monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _doc_id: "tenant-1")
    monkeypatch.setattr(module.DocumentService, "remove_document", lambda *_args, **_kwargs: True)
    res = _run(module.rm())
    assert res["code"] == 0
    assert res["data"] is True

    monkeypatch.setattr(
        module.File2DocumentService,
        "get_by_file_id",
        lambda _file_id: (_ for _ in ()).throw(RuntimeError("rm boom")),
    )
    req_state["file_ids"] = ["boom"]
    res = _run(module.rm())
    assert res["code"] == 500
    assert "rm boom" in res["message"]

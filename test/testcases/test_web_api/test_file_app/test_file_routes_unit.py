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


class _Args(dict):
    def get(self, key, default=None, type=None):
        value = super().get(key, default)
        if value is None or type is None:
            return value
        try:
            return type(value)
        except (TypeError, ValueError):
            return default


class _DummyUploadFile:
    def __init__(self, filename, blob=b"blob"):
        self.filename = filename
        self._blob = blob

    def read(self):
        return self._blob


class _DummyFiles(dict):
    def __init__(self, file_objs=None):
        super().__init__()
        self._file_objs = list(file_objs or [])
        if file_objs is not None:
            self["file"] = self._file_objs

    def getlist(self, key):
        if key == "file":
            return list(self._file_objs)
        return []


class _DummyRequest:
    def __init__(
        self,
        *,
        args=None,
        form=None,
        files=None,
        json_data=None,
        headers=None,
        method="POST",
        content_length=0,
    ):
        self.args = _Args(args or {})
        self.form = _AwaitableValue(form or {})
        self.files = _AwaitableValue(files if files is not None else _DummyFiles())
        self.json = _AwaitableValue(json_data or {})
        self.headers = headers or {}
        self.method = method
        self.content_length = content_length


class _DummyResponse:
    def __init__(self, data):
        self.data = data
        self.headers = {}


class _DummyFile:
    def __init__(
        self,
        file_id,
        file_type,
        *,
        tenant_id="tenant1",
        parent_id="pf1",
        location="file.bin",
        name="file.txt",
        source_type="user",
    ):
        self.id = file_id
        self.type = file_type
        self.tenant_id = tenant_id
        self.parent_id = parent_id
        self.location = location
        self.name = name
        self.source_type = source_type

    def to_json(self):
        return {"id": self.id, "name": self.name, "type": self.type}


def _run(coro):
    return asyncio.run(coro)


def _set_request(
    monkeypatch,
    module,
    *,
    args=None,
    form=None,
    files=None,
    json_data=None,
    headers=None,
    method="POST",
    content_length=0,
):
    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(
            args=args,
            form=form,
            files=files,
            json_data=json_data,
            headers=headers,
            method=method,
            content_length=content_length,
        ),
    )


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


def _load_file_app_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    quart_mod = ModuleType("quart")
    quart_mod.request = _DummyRequest()

    async def _make_response(data):
        return _DummyResponse(data)

    quart_mod.make_response = _make_response
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="tenant1", tenant_id="tenant1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)
    api_pkg.apps = apps_mod

    api_common_pkg = ModuleType("api.common")
    api_common_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.common", api_common_pkg)

    permission_mod = ModuleType("api.common.check_team_permission")
    permission_mod.check_file_team_permission = lambda *_args, **_kwargs: True
    monkeypatch.setitem(sys.modules, "api.common.check_team_permission", permission_mod)
    api_common_pkg.check_team_permission = permission_mod

    db_pkg = ModuleType("api.db")
    db_pkg.__path__ = []

    class _FileType(Enum):
        FOLDER = "folder"
        VIRTUAL = "virtual"
        DOC = "doc"
        VISUAL = "visual"

    db_pkg.FileType = _FileType
    monkeypatch.setitem(sys.modules, "api.db", db_pkg)
    api_pkg.db = db_pkg

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    services_pkg.duplicate_name = lambda _query, **kwargs: kwargs.get("name", "")
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    document_service_mod = ModuleType("api.db.services.document_service")

    class _StubDocumentService:
        @staticmethod
        def get_doc_count(_uid):
            return 0

        @staticmethod
        def get_by_id(doc_id):
            return True, SimpleNamespace(id=doc_id)

        @staticmethod
        def get_tenant_id(_doc_id):
            return "tenant1"

        @staticmethod
        def remove_document(*_args, **_kwargs):
            return True

        @staticmethod
        def update_by_id(*_args, **_kwargs):
            return True

    document_service_mod.DocumentService = _StubDocumentService
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", document_service_mod)
    services_pkg.document_service = document_service_mod

    file2doc_mod = ModuleType("api.db.services.file2document_service")

    class _StubFile2DocumentService:
        @staticmethod
        def get_by_file_id(_file_id):
            return []

        @staticmethod
        def delete_by_file_id(*_args, **_kwargs):
            return None

        @staticmethod
        def get_storage_address(**_kwargs):
            return "bucket2", "location2"

    file2doc_mod.File2DocumentService = _StubFile2DocumentService
    monkeypatch.setitem(sys.modules, "api.db.services.file2document_service", file2doc_mod)
    services_pkg.file2document_service = file2doc_mod

    file_service_mod = ModuleType("api.db.services.file_service")

    class _StubFileService:
        @staticmethod
        def get_root_folder(_tenant_id):
            return {"id": "root"}

        @staticmethod
        def get_by_id(file_id):
            return True, _DummyFile(file_id, _FileType.DOC.value, name="file.txt")

        @staticmethod
        def get_id_list_by_id(_pf_id, _names, _index, ids):
            return ids

        @staticmethod
        def create_folder(_file, parent_id, _names, _len_id):
            return SimpleNamespace(id=parent_id, name=str(parent_id))

        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def insert(data):
            return SimpleNamespace(to_json=lambda: data)

        @staticmethod
        def is_parent_folder_exist(_pf_id):
            return True

        @staticmethod
        def get_by_pf_id(*_args, **_kwargs):
            return [], 0

        @staticmethod
        def get_parent_folder(_file_id):
            return SimpleNamespace(to_json=lambda: {"id": "root"})

        @staticmethod
        def get_all_parent_folders(_file_id):
            return []

        @staticmethod
        def init_knowledgebase_docs(*_args, **_kwargs):
            return None

        @staticmethod
        def list_all_files_by_parent_id(_parent_id):
            return []

        @staticmethod
        def delete(_file):
            return True

        @staticmethod
        def update_by_id(*_args, **_kwargs):
            return True

        @staticmethod
        def get_by_ids(_file_ids):
            return []

        @staticmethod
        def delete_by_id(_file_id):
            return True

    file_service_mod.FileService = _StubFileService
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service_mod)
    services_pkg.file_service = file_service_mod

    api_utils_mod = ModuleType("api.utils.api_utils")

    class _RetCode:
        SUCCESS = 0
        ARGUMENT_ERROR = 101
        AUTHENTICATION_ERROR = 401
        OPERATING_ERROR = 103

    def get_json_result(data=None, message="", code=_RetCode.SUCCESS):
        return {"code": code, "data": data, "message": message}

    async def get_request_json():
        return {}

    def get_data_error_result(message=""):
        return {"code": _RetCode.OPERATING_ERROR, "data": None, "message": message}

    def server_error_response(err):
        return {"code": 500, "data": None, "message": str(err)}

    def validate_request(*_required):
        def _decorator(func):
            return func

        return _decorator

    api_utils_mod.get_json_result = get_json_result
    api_utils_mod.get_request_json = get_request_json
    api_utils_mod.get_data_error_result = get_data_error_result
    api_utils_mod.server_error_response = server_error_response
    api_utils_mod.validate_request = validate_request
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    file_utils_mod = ModuleType("api.utils.file_utils")
    file_utils_mod.filename_type = lambda _name: _FileType.DOC.value
    monkeypatch.setitem(sys.modules, "api.utils.file_utils", file_utils_mod)

    web_utils_mod = ModuleType("api.utils.web_utils")
    web_utils_mod.CONTENT_TYPE_MAP = {"txt": "text/plain", "json": "application/json"}
    web_utils_mod.apply_safe_file_response_headers = (
        lambda response, content_type, ext: response.headers.update({"content_type": content_type, "extension": ext})
    )
    monkeypatch.setitem(sys.modules, "api.utils.web_utils", web_utils_mod)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    settings_mod = ModuleType("common.settings")
    settings_mod.STORAGE_IMPL = SimpleNamespace(
        obj_exist=lambda *_args, **_kwargs: False,
        put=lambda *_args, **_kwargs: None,
        rm=lambda *_args, **_kwargs: None,
        get=lambda *_args, **_kwargs: b"",
    )
    common_pkg.settings = settings_mod
    monkeypatch.setitem(sys.modules, "common.settings", settings_mod)

    constants_mod = ModuleType("common.constants")

    class _FileSource:
        KNOWLEDGEBASE = "knowledgebase"

    constants_mod.RetCode = _RetCode
    constants_mod.FileSource = _FileSource
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    misc_utils_mod = ModuleType("common.misc_utils")
    misc_utils_mod.get_uuid = lambda: "uuid-1"

    async def thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    misc_utils_mod.thread_pool_exec = thread_pool_exec
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    module_name = "test_file_app_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "file_app.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_upload_branch_matrix_unit(monkeypatch):
    module = _load_file_app_module(monkeypatch)
    monkeypatch.setattr(module.FileService, "get_root_folder", lambda _uid: {"id": "root"})

    _set_request(monkeypatch, module, form={}, files=_DummyFiles())
    res = _run(module.upload())
    assert res["code"] == module.RetCode.ARGUMENT_ERROR
    assert res["message"] == "No file part!"

    _set_request(
        monkeypatch,
        module,
        form={"parent_id": "pf1"},
        files=_DummyFiles([_DummyUploadFile("")]),
    )
    res = _run(module.upload())
    assert res["code"] == module.RetCode.ARGUMENT_ERROR
    assert res["message"] == "No file selected!"

    _set_request(
        monkeypatch,
        module,
        form={"parent_id": "pf1"},
        files=_DummyFiles([_DummyUploadFile("a.txt")]),
    )
    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
    res = _run(module.upload())
    assert res["code"] == module.RetCode.OPERATING_ERROR
    assert res["message"] == "Can't find this folder!"

    monkeypatch.setenv("MAX_FILE_NUM_PER_USER", "1")
    _set_request(
        monkeypatch,
        module,
        form={"parent_id": "pf1"},
        files=_DummyFiles([_DummyUploadFile("cap.txt")]),
    )
    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (True, SimpleNamespace(id="pf1", name="pf1")))
    monkeypatch.setattr(module.DocumentService, "get_doc_count", lambda _uid: 1)
    res = _run(module.upload())
    assert res["code"] == module.RetCode.SUCCESS
    assert "Exceed the maximum file number of a free user!" in res["data"][0]["message"]
    monkeypatch.delenv("MAX_FILE_NUM_PER_USER", raising=False)

    class _StorageNoCollision:
        def __init__(self):
            self.put_calls = []

        def obj_exist(self, _bucket, _location):
            return False

        def put(self, bucket, location, blob):
            self.put_calls.append((bucket, location, blob))

    storage_no_collision = _StorageNoCollision()
    monkeypatch.setattr(module.settings, "STORAGE_IMPL", storage_no_collision)
    _set_request(
        monkeypatch,
        module,
        form={"parent_id": "pf1"},
        files=_DummyFiles([_DummyUploadFile(None, b"none-blob")]),
    )
    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (True, SimpleNamespace(id="pf1", name="pf1")))
    monkeypatch.setattr(module.FileService, "get_id_list_by_id", lambda *_args, **_kwargs: ["pf1"])
    monkeypatch.setattr(
        module.FileService,
        "create_folder",
        lambda _file, parent_id, _names, _len_id: SimpleNamespace(id=f"{parent_id}-folder"),
    )
    monkeypatch.setattr(module, "filename_type", lambda _name: module.FileType.DOC.value)
    monkeypatch.setattr(module, "duplicate_name", lambda _query, **kwargs: kwargs.get("name"))
    monkeypatch.setattr(module, "get_uuid", lambda: "uuid-none")
    monkeypatch.setattr(module.FileService, "insert", lambda data: SimpleNamespace(to_json=lambda: {"id": data["id"]}))
    res = _run(module.upload())
    assert res["code"] == module.RetCode.SUCCESS
    assert len(res["data"]) == 1
    assert storage_no_collision.put_calls == [("pf1-folder", None, b"none-blob")]

    _set_request(
        monkeypatch,
        module,
        form={"parent_id": "pf1"},
        files=_DummyFiles([_DummyUploadFile("dir/a.txt")]),
    )
    monkeypatch.setattr(module.FileService, "get_id_list_by_id", lambda *_args, **_kwargs: ["pf1", "missing-child"])

    def _get_by_id_missing_child(file_id):
        if file_id == "missing-child":
            return False, None
        return True, SimpleNamespace(id=file_id, name=file_id)

    monkeypatch.setattr(module.FileService, "get_by_id", _get_by_id_missing_child)
    res = _run(module.upload())
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"][0]["message"] == "Folder not found!"

    _set_request(
        monkeypatch,
        module,
        form={"parent_id": "pf1"},
        files=_DummyFiles([_DummyUploadFile("b.txt")]),
    )
    monkeypatch.setattr(module.FileService, "get_id_list_by_id", lambda *_args, **_kwargs: ["pf1", "leaf"])
    pf1_calls = {"count": 0}

    def _get_by_id_missing_parent_else(file_id):
        if file_id == "pf1":
            pf1_calls["count"] += 1
            if pf1_calls["count"] == 1:
                return True, SimpleNamespace(id="pf1", name="pf1")
            return False, None
        return True, SimpleNamespace(id=file_id, name=file_id)

    monkeypatch.setattr(module.FileService, "get_by_id", _get_by_id_missing_parent_else)
    res = _run(module.upload())
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"][0]["message"] == "Folder not found!"

    class _StorageCollision:
        def __init__(self):
            self.obj_calls = 0
            self.put_calls = []

        def obj_exist(self, _bucket, _location):
            self.obj_calls += 1
            return self.obj_calls == 1

        def put(self, bucket, location, blob):
            self.put_calls.append((bucket, location, blob))

    storage_collision = _StorageCollision()
    monkeypatch.setattr(module.settings, "STORAGE_IMPL", storage_collision)
    _set_request(
        monkeypatch,
        module,
        form={"parent_id": "pf1"},
        files=_DummyFiles([_DummyUploadFile("dir/a.txt", b"a"), _DummyUploadFile("b.txt", b"b")]),
    )

    def _get_by_id_ok(file_id):
        return True, SimpleNamespace(id=file_id, name=file_id)

    def _get_id_list(_pf_id, file_obj_names, _idx, _ids):
        if file_obj_names[-1] == "a.txt":
            return ["pf1", "mid-id"]
        return ["pf1", "leaf-id"]

    def _create_folder(_file, parent_id, _names, _len_id):
        return SimpleNamespace(id=f"{parent_id}-folder")

    inserted_payloads = []
    monkeypatch.setattr(module.FileService, "get_by_id", _get_by_id_ok)
    monkeypatch.setattr(module.FileService, "get_id_list_by_id", _get_id_list)
    monkeypatch.setattr(module.FileService, "create_folder", _create_folder)
    monkeypatch.setattr(module, "filename_type", lambda _name: module.FileType.DOC.value)
    monkeypatch.setattr(module, "duplicate_name", lambda _query, **kwargs: kwargs["name"])
    monkeypatch.setattr(module, "get_uuid", lambda: "file-id")
    monkeypatch.setattr(module.FileService, "query", lambda **_kwargs: [])

    def _insert(data):
        inserted_payloads.append(data)
        return SimpleNamespace(to_json=lambda: {"id": data["id"], "location": data["location"]})

    monkeypatch.setattr(module.FileService, "insert", _insert)
    res = _run(module.upload())
    assert res["code"] == module.RetCode.SUCCESS
    assert len(res["data"]) == 2
    assert len(storage_collision.put_calls) == 2
    assert any(location.endswith("_") for _, location, _ in storage_collision.put_calls)
    assert len(inserted_payloads) == 2

    _set_request(
        monkeypatch,
        module,
        form={"parent_id": "pf1"},
        files=_DummyFiles([_DummyUploadFile("boom.txt")]),
    )
    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (_ for _ in ()).throw(RuntimeError("upload boom")))
    monkeypatch.setattr(module, "server_error_response", lambda err: {"code": 500, "message": str(err)})
    res = _run(module.upload())
    assert res["code"] == 500
    assert "upload boom" in res["message"]


@pytest.mark.p2
def test_create_and_list_branch_matrix_unit(monkeypatch):
    module = _load_file_app_module(monkeypatch)
    req_state = {"name": "file1"}
    _set_request_json(monkeypatch, module, req_state)
    monkeypatch.setattr(module.FileService, "get_root_folder", lambda _uid: {"id": "root"})

    monkeypatch.setattr(module.FileService, "is_parent_folder_exist", lambda _pf_id: False)
    res = _run(module.create())
    assert res["code"] == module.RetCode.OPERATING_ERROR
    assert "Parent Folder Doesn't Exist!" in res["message"]

    req_state.update({"name": "dup", "parent_id": "pf1"})
    monkeypatch.setattr(module.FileService, "is_parent_folder_exist", lambda _pf_id: True)
    monkeypatch.setattr(module.FileService, "query", lambda **_kwargs: [object()])
    res = _run(module.create())
    assert "Duplicated folder name" in res["message"]

    inserted = {}

    def _insert(data):
        inserted["payload"] = data
        return SimpleNamespace(to_json=lambda: data)

    monkeypatch.setattr(module.FileService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module, "get_uuid", lambda: "uuid-folder")
    monkeypatch.setattr(module.FileService, "insert", _insert)

    req_state.update({"name": "folder", "parent_id": "pf1", "type": module.FileType.FOLDER.value})
    res = _run(module.create())
    assert res["code"] == module.RetCode.SUCCESS
    assert inserted["payload"]["type"] == module.FileType.FOLDER.value

    req_state.update({"name": "virtual", "parent_id": "pf1", "type": "UNKNOWN"})
    res = _run(module.create())
    assert res["code"] == module.RetCode.SUCCESS
    assert inserted["payload"]["type"] == module.FileType.VIRTUAL.value

    monkeypatch.setattr(
        module.FileService,
        "is_parent_folder_exist",
        lambda _pf_id: (_ for _ in ()).throw(RuntimeError("create boom")),
    )
    monkeypatch.setattr(module, "server_error_response", lambda err: {"code": 500, "message": str(err)})
    res = _run(module.create())
    assert res["code"] == 500
    assert "create boom" in res["message"]

    list_calls = {"init": 0}
    monkeypatch.setattr(module.FileService, "get_root_folder", lambda _uid: {"id": "root"})
    monkeypatch.setattr(
        module.FileService,
        "init_knowledgebase_docs",
        lambda _pf_id, _uid: list_calls.__setitem__("init", list_calls["init"] + 1),
    )
    _set_request(monkeypatch, module, args={})
    monkeypatch.setattr(module.FileService, "get_by_id", lambda _pf_id: (False, None))
    res = module.list_files()
    assert res["message"] == "Folder not found!"
    assert list_calls["init"] == 1

    _set_request(
        monkeypatch,
        module,
        args={
            "parent_id": "p1",
            "keywords": "k",
            "page": "2",
            "page_size": "10",
            "orderby": "name",
            "desc": "False",
        },
    )
    monkeypatch.setattr(module.FileService, "get_by_id", lambda _pf_id: (True, SimpleNamespace(id="p1")))
    monkeypatch.setattr(module.FileService, "get_by_pf_id", lambda *_args, **_kwargs: ([{"id": "f1"}], 1))
    monkeypatch.setattr(module.FileService, "get_parent_folder", lambda _pf_id: None)
    res = module.list_files()
    assert res["message"] == "File not found!"

    monkeypatch.setattr(module.FileService, "get_parent_folder", lambda _pf_id: SimpleNamespace(to_json=lambda: {"id": "p0"}))
    res = module.list_files()
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["total"] == 1
    assert res["data"]["parent_folder"]["id"] == "p0"

    monkeypatch.setattr(module.FileService, "get_by_id", lambda _pf_id: (_ for _ in ()).throw(RuntimeError("list boom")))
    monkeypatch.setattr(module, "server_error_response", lambda err: {"code": 500, "message": str(err)})
    res = module.list_files()
    assert res["code"] == 500
    assert "list boom" in res["message"]


@pytest.mark.p2
def test_folder_lookup_routes_branch_matrix_unit(monkeypatch):
    module = _load_file_app_module(monkeypatch)

    monkeypatch.setattr(module.FileService, "get_root_folder", lambda _uid: {"id": "root"})
    res = module.get_root_folder()
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["root_folder"]["id"] == "root"

    monkeypatch.setattr(module.FileService, "get_root_folder", lambda _uid: (_ for _ in ()).throw(RuntimeError("root boom")))
    monkeypatch.setattr(module, "server_error_response", lambda err: {"code": 500, "message": str(err)})
    res = module.get_root_folder()
    assert res["code"] == 500
    assert "root boom" in res["message"]

    _set_request(monkeypatch, module, args={"file_id": "missing"})
    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
    res = module.get_parent_folder()
    assert res["message"] == "Folder not found!"

    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (True, SimpleNamespace(id="f1")))
    monkeypatch.setattr(module.FileService, "get_parent_folder", lambda _file_id: SimpleNamespace(to_json=lambda: {"id": "p1"}))
    res = module.get_parent_folder()
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["parent_folder"]["id"] == "p1"

    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (_ for _ in ()).throw(RuntimeError("parent boom")))
    monkeypatch.setattr(module, "server_error_response", lambda err: {"code": 500, "message": str(err)})
    res = module.get_parent_folder()
    assert res["code"] == 500
    assert "parent boom" in res["message"]

    _set_request(monkeypatch, module, args={"file_id": "missing"})
    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
    res = module.get_all_parent_folders()
    assert res["message"] == "Folder not found!"

    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (True, SimpleNamespace(id="f1")))
    monkeypatch.setattr(
        module.FileService,
        "get_all_parent_folders",
        lambda _file_id: [SimpleNamespace(to_json=lambda: {"id": "p1"}), SimpleNamespace(to_json=lambda: {"id": "p2"})],
    )
    res = module.get_all_parent_folders()
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["parent_folders"] == [{"id": "p1"}, {"id": "p2"}]

    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (_ for _ in ()).throw(RuntimeError("all-parent boom")))
    monkeypatch.setattr(module, "server_error_response", lambda err: {"code": 500, "message": str(err)})
    res = module.get_all_parent_folders()
    assert res["code"] == 500
    assert "all-parent boom" in res["message"]


@pytest.mark.p2
def test_rm_branch_matrix_unit(monkeypatch):
    module = _load_file_app_module(monkeypatch)
    req_state = {"file_ids": ["missing"]}
    _set_request_json(monkeypatch, module, req_state)

    allow = {"value": True}
    monkeypatch.setattr(module, "check_file_team_permission", lambda _file, _uid: allow["value"])

    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
    res = _run(module.rm())
    assert res["message"] == "File or Folder not found!"

    req_state["file_ids"] = ["tenant-missing"]
    monkeypatch.setattr(
        module.FileService,
        "get_by_id",
        lambda _file_id: (True, _DummyFile(_file_id, module.FileType.DOC.value, tenant_id=None)),
    )
    res = _run(module.rm())
    assert res["message"] == "Tenant not found!"

    req_state["file_ids"] = ["deny"]
    allow["value"] = False
    monkeypatch.setattr(
        module.FileService,
        "get_by_id",
        lambda _file_id: (True, _DummyFile(_file_id, module.FileType.DOC.value)),
    )
    res = _run(module.rm())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert res["message"] == "No authorization."
    allow["value"] = True

    req_state["file_ids"] = ["kb"]
    monkeypatch.setattr(
        module.FileService,
        "get_by_id",
        lambda _file_id: (
            True,
            _DummyFile(
                _file_id,
                module.FileType.DOC.value,
                source_type=module.FileSource.KNOWLEDGEBASE,
            ),
        ),
    )
    res = _run(module.rm())
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"] is True

    events = {
        "rm_calls": [],
        "deleted_files": [],
        "deleted_links": [],
        "removed_docs": [],
    }

    class _Storage:
        def rm(self, bucket, location):
            events["rm_calls"].append((bucket, location))
            raise RuntimeError("storage rm boom")

    monkeypatch.setattr(module.settings, "STORAGE_IMPL", _Storage())
    monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda file_id: [SimpleNamespace(document_id=f"doc-{file_id}")])
    monkeypatch.setattr(module.DocumentService, "get_by_id", lambda doc_id: (True, SimpleNamespace(id=doc_id)))
    monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _doc_id: "tenant1")
    monkeypatch.setattr(
        module.DocumentService,
        "remove_document",
        lambda doc, tenant: events["removed_docs"].append((doc.id, tenant)),
    )
    monkeypatch.setattr(
        module.File2DocumentService,
        "delete_by_file_id",
        lambda file_id: events["deleted_links"].append(file_id),
    )
    monkeypatch.setattr(module.FileService, "delete", lambda file: events["deleted_files"].append(file.id))

    req_state["file_ids"] = ["doc-top"]
    monkeypatch.setattr(
        module.FileService,
        "get_by_id",
        lambda _file_id: (True, _DummyFile("doc-top", module.FileType.DOC.value, location="top.bin")),
    )
    res = _run(module.rm())
    assert res["code"] == module.RetCode.SUCCESS

    req_state["file_ids"] = ["folder1"]
    folder1 = _DummyFile("folder1", module.FileType.FOLDER.value, location="")
    nested_folder = _DummyFile("nested-folder", module.FileType.FOLDER.value, parent_id="folder1", location="")
    doc1 = _DummyFile("doc1", module.FileType.DOC.value, parent_id="folder1", location="doc1.bin")
    doc2 = _DummyFile("doc2", module.FileType.DOC.value, parent_id="nested-folder", location="doc2.bin")

    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (True, folder1))

    def _list_all(parent_id):
        if parent_id == "folder1":
            return [nested_folder, doc1]
        if parent_id == "nested-folder":
            return [doc2]
        return []

    monkeypatch.setattr(module.FileService, "list_all_files_by_parent_id", _list_all)
    res = _run(module.rm())
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"] is True
    assert ("pf1", "top.bin") in events["rm_calls"]
    assert ("folder1", "doc1.bin") in events["rm_calls"]
    assert ("nested-folder", "doc2.bin") in events["rm_calls"]
    assert {"doc-top", "doc1", "doc2", "nested-folder", "folder1"}.issubset(set(events["deleted_files"]))
    assert {"doc-top", "doc1", "doc2"}.issubset(set(events["deleted_links"]))
    assert len(events["removed_docs"]) >= 3

    async def _thread_pool_boom(_func, *_args, **_kwargs):
        raise RuntimeError("rm route boom")

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_boom)
    req_state["file_ids"] = ["boom"]
    res = _run(module.rm())
    assert res["code"] == 500
    assert "rm route boom" in res["message"]


@pytest.mark.p2
def test_rename_branch_matrix_unit(monkeypatch):
    module = _load_file_app_module(monkeypatch)
    req_state = {"file_id": "f1", "name": "new.txt"}
    _set_request_json(monkeypatch, module, req_state)

    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
    res = _run(module.rename())
    assert res["message"] == "File not found!"

    monkeypatch.setattr(
        module.FileService,
        "get_by_id",
        lambda _file_id: (True, _DummyFile("f1", module.FileType.DOC.value, name="origin.txt")),
    )
    monkeypatch.setattr(module, "check_file_team_permission", lambda _file, _uid: False)
    res = _run(module.rename())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert res["message"] == "No authorization."

    monkeypatch.setattr(module, "check_file_team_permission", lambda _file, _uid: True)
    req_state["name"] = "new.pdf"
    res = _run(module.rename())
    assert res["code"] == module.RetCode.ARGUMENT_ERROR
    assert "extension of file can't be changed" in res["message"]

    req_state["name"] = "new.txt"
    monkeypatch.setattr(module.FileService, "query", lambda **_kwargs: [SimpleNamespace(name="new.txt")])
    res = _run(module.rename())
    assert "Duplicated file name in the same folder." in res["message"]

    monkeypatch.setattr(module.FileService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module.FileService, "update_by_id", lambda *_args, **_kwargs: False)
    res = _run(module.rename())
    assert "Database error (File rename)!" in res["message"]

    monkeypatch.setattr(module.FileService, "update_by_id", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _file_id: [SimpleNamespace(document_id="doc1")])
    monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: False)
    res = _run(module.rename())
    assert "Database error (Document rename)!" in res["message"]

    monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _file_id: [])
    res = _run(module.rename())
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"] is True

    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (_ for _ in ()).throw(RuntimeError("rename boom")))
    monkeypatch.setattr(module, "server_error_response", lambda err: {"code": 500, "message": str(err)})
    res = _run(module.rename())
    assert res["code"] == 500
    assert "rename boom" in res["message"]


@pytest.mark.p2
def test_get_file_branch_matrix_unit(monkeypatch):
    module = _load_file_app_module(monkeypatch)

    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
    res = _run(module.get("missing"))
    assert res["message"] == "Document not found!"

    monkeypatch.setattr(
        module.FileService,
        "get_by_id",
        lambda _file_id: (True, _DummyFile("f1", module.FileType.DOC.value, name="a.txt")),
    )
    monkeypatch.setattr(module, "check_file_team_permission", lambda _file, _uid: False)
    res = _run(module.get("f1"))
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert res["message"] == "No authorization."

    class _Storage:
        def __init__(self):
            self.calls = []

        def get(self, bucket, location):
            self.calls.append((bucket, location))
            if len(self.calls) == 1:
                return None
            return b"blob-data"

    storage = _Storage()
    monkeypatch.setattr(module.settings, "STORAGE_IMPL", storage)
    monkeypatch.setattr(
        module.FileService,
        "get_by_id",
        lambda _file_id: (
            True,
            _DummyFile(
                "f1",
                module.FileType.VISUAL.value,
                parent_id="pf1",
                location="loc1",
                name="image.abc",
            ),
        ),
    )
    monkeypatch.setattr(module, "check_file_team_permission", lambda _file, _uid: True)
    monkeypatch.setattr(module.File2DocumentService, "get_storage_address", lambda **_kwargs: ("pf2", "loc2"))

    async def _make_response(data):
        return _DummyResponse(data)

    monkeypatch.setattr(module, "make_response", _make_response)
    monkeypatch.setattr(
        module,
        "apply_safe_file_response_headers",
        lambda response, content_type, ext: response.headers.update(
            {"content_type": content_type, "extension": ext}
        ),
    )
    res = _run(module.get("f1"))
    assert isinstance(res, _DummyResponse)
    assert res.data == b"blob-data"
    assert storage.calls == [("pf1", "loc1"), ("pf2", "loc2")]
    assert res.headers["extension"] == "abc"
    assert res.headers["content_type"] == "image/abc"


@pytest.mark.p2
def test_get_file_content_type_and_error_paths_unit(monkeypatch):
    module = _load_file_app_module(monkeypatch)
    monkeypatch.setattr(module, "check_file_team_permission", lambda _file, _uid: True)

    class _Storage:
        @staticmethod
        def get(_bucket, _location):
            return b"blob-data"

    monkeypatch.setattr(module.settings, "STORAGE_IMPL", _Storage())
    monkeypatch.setattr(module.File2DocumentService, "get_storage_address", lambda **_kwargs: ("pf2", "loc2"))

    async def _make_response(data):
        return _DummyResponse(data)

    headers_calls = []

    def _apply_headers(response, content_type, ext):
        headers_calls.append((content_type, ext))
        response.headers["content_type"] = content_type
        response.headers["extension"] = ext

    monkeypatch.setattr(module, "make_response", _make_response)
    monkeypatch.setattr(module, "apply_safe_file_response_headers", _apply_headers)

    monkeypatch.setattr(
        module.FileService,
        "get_by_id",
        lambda _file_id: (
            True,
            _DummyFile("img", module.FileType.VISUAL.value, parent_id="pf1", location="loc1", name="image.abc"),
        ),
    )
    res = _run(module.get("img"))
    assert isinstance(res, _DummyResponse)
    assert res.headers["content_type"] == "image/abc"
    assert res.headers["extension"] == "abc"

    monkeypatch.setattr(
        module.FileService,
        "get_by_id",
        lambda _file_id: (
            True,
            _DummyFile("noext", module.FileType.DOC.value, parent_id="pf1", location="loc1", name="README"),
        ),
    )
    res = _run(module.get("noext"))
    assert isinstance(res, _DummyResponse)
    assert res.headers["content_type"] is None
    assert res.headers["extension"] is None
    assert headers_calls == [("image/abc", "abc"), (None, None)]

    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (_ for _ in ()).throw(RuntimeError("get crash")))
    monkeypatch.setattr(module, "server_error_response", lambda err: {"code": 500, "message": str(err)})
    res = _run(module.get("boom"))
    assert res["code"] == 500
    assert "get crash" in res["message"]


@pytest.mark.p2
def test_move_recursive_branch_matrix_unit(monkeypatch):
    module = _load_file_app_module(monkeypatch)
    req_state = {"src_file_ids": ["f1"], "dest_file_id": "dest"}
    _set_request_json(monkeypatch, module, req_state)

    async def _thread_pool_exec(fn, *args, **kwargs):
        return fn(*args, **kwargs)

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_exec)
    monkeypatch.setattr(module, "check_file_team_permission", lambda _file, _uid: True)

    dest_folder = SimpleNamespace(id="dest")
    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
    res = _run(module.move())
    assert res["message"] == "Parent folder not found!"

    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (True, dest_folder))
    monkeypatch.setattr(module.FileService, "get_by_ids", lambda _file_ids: [])
    res = _run(module.move())
    assert res["message"] == "Source files not found!"

    req_state["src_file_ids"] = ["f1", "f2"]
    monkeypatch.setattr(module.FileService, "get_by_ids", lambda _file_ids: [_DummyFile("f1", module.FileType.DOC.value)])
    res = _run(module.move())
    assert res["message"] == "File or folder not found!"

    req_state["src_file_ids"] = ["tenant-missing"]
    monkeypatch.setattr(
        module.FileService,
        "get_by_ids",
        lambda _file_ids: [_DummyFile("tenant-missing", module.FileType.DOC.value, tenant_id=None)],
    )
    res = _run(module.move())
    assert res["message"] == "Tenant not found!"

    req_state["src_file_ids"] = ["deny"]
    monkeypatch.setattr(module.FileService, "get_by_ids", lambda _file_ids: [_DummyFile("deny", module.FileType.DOC.value)])
    monkeypatch.setattr(module, "check_file_team_permission", lambda _file, _uid: False)
    res = _run(module.move())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert res["message"] == "No authorization."

    monkeypatch.setattr(module, "check_file_team_permission", lambda _file, _uid: True)

    req_state["src_file_ids"] = ["folder_existing", "folder_new", "doc_main"]
    folder_existing = _DummyFile(
        "folder_existing",
        module.FileType.FOLDER.value,
        tenant_id="tenant1",
        parent_id="old_bucket",
        location="",
        name="existing-folder",
    )
    folder_new = _DummyFile(
        "folder_new",
        module.FileType.FOLDER.value,
        tenant_id="tenant1",
        parent_id="old_bucket",
        location="",
        name="new-folder",
    )
    doc_main = _DummyFile(
        "doc_main",
        module.FileType.DOC.value,
        tenant_id="tenant1",
        parent_id="old_bucket",
        location="doc.bin",
        name="doc.bin",
    )
    sub_doc = _DummyFile(
        "sub_doc",
        module.FileType.DOC.value,
        tenant_id="tenant1",
        parent_id="folder_existing",
        location="sub.txt",
        name="sub.txt",
    )

    monkeypatch.setattr(module.FileService, "get_by_ids", lambda _file_ids: [folder_existing, folder_new, doc_main])

    inserted = []
    deleted = []
    updated = []
    existing_dest = SimpleNamespace(id="dest-existing")
    new_dest = SimpleNamespace(id="dest-new")

    def _query(**kwargs):
        if kwargs.get("name") == "existing-folder":
            return [existing_dest]
        if kwargs.get("name") == "new-folder":
            return []
        return []

    def _insert(payload):
        inserted.append(payload)
        return new_dest

    def _list_subfiles(parent_id):
        if parent_id == "folder_existing":
            return [sub_doc]
        if parent_id == "folder_new":
            return []
        return []

    class _Storage:
        def __init__(self):
            self.move_calls = []
            self._collision = 0

        def obj_exist(self, _bucket, location):
            if location == "doc.bin" and self._collision == 0:
                self._collision += 1
                return True
            return False

        def move(self, old_parent, old_location, new_parent, new_location):
            self.move_calls.append((old_parent, old_location, new_parent, new_location))

    storage = _Storage()
    monkeypatch.setattr(module.settings, "STORAGE_IMPL", storage)
    monkeypatch.setattr(module.FileService, "query", _query)
    monkeypatch.setattr(module.FileService, "insert", _insert)
    monkeypatch.setattr(module.FileService, "list_all_files_by_parent_id", _list_subfiles)
    monkeypatch.setattr(module.FileService, "delete_by_id", lambda file_id: deleted.append(file_id))
    monkeypatch.setattr(module.FileService, "update_by_id", lambda file_id, payload: updated.append((file_id, payload)) or True)

    res = _run(module.move())
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"] is True
    assert inserted and inserted[0]["name"] == "new-folder"
    assert set(deleted) == {"folder_existing", "folder_new"}
    assert ("old_bucket", "doc.bin", "dest", "doc.bin_") in storage.move_calls
    assert ("folder_existing", "sub.txt", "dest-existing", "sub.txt") in storage.move_calls
    assert ("doc_main", {"parent_id": "dest", "location": "doc.bin_"}) in updated
    assert ("sub_doc", {"parent_id": "dest-existing", "location": "sub.txt"}) in updated

    req_state["src_file_ids"] = ["boom_doc"]
    monkeypatch.setattr(
        module.FileService,
        "get_by_ids",
        lambda _file_ids: [
            _DummyFile("boom_doc", module.FileType.DOC.value, tenant_id="tenant1", parent_id="old_bucket", location="boom", name="boom")
        ],
    )

    class _StorageBoom:
        @staticmethod
        def obj_exist(_bucket, _location):
            return False

        @staticmethod
        def move(*_args, **_kwargs):
            raise RuntimeError("storage down")

    monkeypatch.setattr(module.settings, "STORAGE_IMPL", _StorageBoom())
    monkeypatch.setattr(module, "server_error_response", lambda err: {"code": 500, "message": str(err)})
    res = _run(module.move())
    assert res["code"] == 500
    assert "Move file failed at storage layer: storage down" in res["message"]

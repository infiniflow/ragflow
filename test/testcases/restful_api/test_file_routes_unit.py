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


class _DummyUploadFile:
    def __init__(self, filename, blob=b"blob"):
        self.filename = filename
        self._blob = blob

    def read(self):
        return self._blob


class _DummyRequest:
    def __init__(self, *, content_type="", form=None, files=None, args=None):
        self.content_type = content_type
        self.form = _AwaitableValue(form or {})
        self.files = _AwaitableValue(files if files is not None else _DummyFiles())
        self.args = args or {}


class _DummyResponse:
    def __init__(self, data):
        self.data = data
        self.headers = {}


def _run(coro):
    return asyncio.run(coro)


def _load_file_api_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[3]

    quart_mod = ModuleType("quart")
    quart_mod.request = _DummyRequest()

    async def _make_response(data):
        return _DummyResponse(data)

    quart_mod.make_response = _make_response
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_pkg = ModuleType("api.apps")
    apps_pkg.__path__ = [str(repo_root / "api" / "apps")]
    apps_pkg.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_pkg)
    api_pkg.apps = apps_pkg

    services_pkg = ModuleType("api.apps.services")
    services_pkg.__path__ = [str(repo_root / "api" / "apps" / "services")]
    monkeypatch.setitem(sys.modules, "api.apps.services", services_pkg)
    apps_pkg.services = services_pkg

    file_api_service_mod = ModuleType("api.apps.services.file_api_service")

    async def _upload_file(_tenant_id, _pf_id, _file_objs):
        return True, [{"id": "f1"}]

    async def _create_folder(_tenant_id, _name, _parent_id=None, _file_type=None):
        return True, {"id": "folder1"}

    async def _delete_files(_tenant_id, _ids):
        return True, True

    async def _move_files(_tenant_id, _src_file_ids, _dest_file_id=None, _new_name=None):
        return True, True

    file_api_service_mod.upload_file = _upload_file
    file_api_service_mod.create_folder = _create_folder
    file_api_service_mod.list_files = lambda _tenant_id, _args: (True, {"files": [], "total": 0})
    file_api_service_mod.delete_files = _delete_files
    file_api_service_mod.move_files = _move_files
    file_api_service_mod.get_file_content = lambda _tenant_id, _file_id: (
        True,
        SimpleNamespace(parent_id="bucket1", location="path1", name="doc.txt", type="doc"),
    )
    file_api_service_mod.get_parent_folder = lambda _file_id, user_id=None: (True, {"parent_folder": {"id": "parent1"}})
    file_api_service_mod.get_all_parent_folders = lambda _file_id, user_id=None: (True, {"parent_folders": [{"id": "root"}]})
    monkeypatch.setitem(sys.modules, "api.apps.services.file_api_service", file_api_service_mod)
    services_pkg.file_api_service = file_api_service_mod

    db_pkg = ModuleType("api.db")
    db_pkg.__path__ = []

    class _FileType(Enum):
        DOC = "doc"
        VISUAL = "visual"

    db_pkg.FileType = _FileType
    monkeypatch.setitem(sys.modules, "api.db", db_pkg)
    api_pkg.db = db_pkg

    file2doc_mod = ModuleType("api.db.services.file2document_service")
    file2doc_mod.File2DocumentService = SimpleNamespace(get_storage_address=lambda **_kwargs: ("bucket2", "path2"))
    monkeypatch.setitem(sys.modules, "api.db.services.file2document_service", file2doc_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")
    api_utils_mod.add_tenant_id_to_kwargs = lambda func: func
    api_utils_mod.get_error_argument_result = lambda message: {"code": 400, "data": None, "message": message}
    api_utils_mod.get_error_data_result = lambda message: {"code": 500, "data": None, "message": message}
    api_utils_mod.get_result = lambda data=None: {"code": 0, "data": data, "message": ""}
    api_utils_mod.get_json_result = lambda code=0, message="success", data=None: {"code": code, "data": data, "message": message}
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    validation_mod = ModuleType("api.utils.validation_utils")
    validation_mod.CreateFolderReq = object
    validation_mod.DeleteFileReq = object
    validation_mod.ListFileReq = object
    validation_mod.MoveFileReq = object

    async def _validate_json_request(_request, _schema):
        return {}, None

    validation_mod.validate_and_parse_json_request = _validate_json_request
    validation_mod.validate_and_parse_request_args = lambda _request, _schema: ({}, None)
    monkeypatch.setitem(sys.modules, "api.utils.validation_utils", validation_mod)

    web_utils_mod = ModuleType("api.utils.web_utils")
    web_utils_mod.CONTENT_TYPE_MAP = {"txt": "text/plain"}
    web_utils_mod.apply_safe_file_response_headers = lambda response, content_type, ext: response.headers.update({"content_type": content_type, "ext": ext})
    monkeypatch.setitem(sys.modules, "api.utils.web_utils", web_utils_mod)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    common_pkg.settings = SimpleNamespace(
        STORAGE_IMPL=SimpleNamespace(
            get=lambda *_args, **_kwargs: b"blob",
        )
    )
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    misc_utils_mod = ModuleType("common.misc_utils")

    async def thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    misc_utils_mod.thread_pool_exec = thread_pool_exec
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    module_path = repo_root / "api" / "apps" / "restful_apis" / "file_api.py"
    spec = importlib.util.spec_from_file_location("api.apps.restful_apis.file_api", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, "api.apps.restful_apis.file_api", module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_create_or_upload_multipart_requires_file(monkeypatch):
    module = _load_file_api_module(monkeypatch)
    monkeypatch.setattr(module, "request", _DummyRequest(content_type="multipart/form-data", form={}, files=_DummyFiles()))

    res = _run(module.create_or_upload("tenant1"))
    assert res["code"] == 400
    assert res["message"] == "No file part!"


@pytest.mark.p2
def test_create_or_upload_uploads_via_new_service(monkeypatch):
    module = _load_file_api_module(monkeypatch)
    files = _DummyFiles([_DummyUploadFile("a.txt")])
    monkeypatch.setattr(module, "request", _DummyRequest(content_type="multipart/form-data", form={"parent_id": "pf1"}, files=files))

    seen = {}

    async def _upload_file(tenant_id, pf_id, file_objs):
        seen["args"] = (tenant_id, pf_id, [f.filename for f in file_objs])
        return True, [{"id": "f1"}]

    monkeypatch.setattr(module.file_api_service, "upload_file", _upload_file)
    res = _run(module.create_or_upload("tenant1"))

    assert seen["args"] == ("tenant1", "pf1", ["a.txt"])
    assert res["code"] == 0
    assert res["data"] == [{"id": "f1"}]


@pytest.mark.p2
def test_create_or_upload_creates_folder_from_json(monkeypatch):
    module = _load_file_api_module(monkeypatch)
    monkeypatch.setattr(module, "request", _DummyRequest(content_type="application/json"))

    async def _validate(_request, _schema):
        return {"name": "folder-a", "parent_id": "pf1", "type": "folder"}, None

    async def _create_folder(tenant_id, name, parent_id=None, file_type=None):
        return True, {"tenant_id": tenant_id, "name": name, "parent_id": parent_id, "type": file_type}

    monkeypatch.setattr(module, "validate_and_parse_json_request", _validate)
    monkeypatch.setattr(module.file_api_service, "create_folder", _create_folder)

    res = _run(module.create_or_upload("tenant1"))
    assert res["code"] == 0
    assert res["data"]["tenant_id"] == "tenant1"
    assert res["data"]["name"] == "folder-a"


@pytest.mark.p2
def test_list_files_validation_error(monkeypatch):
    module = _load_file_api_module(monkeypatch)
    monkeypatch.setattr(module, "validate_and_parse_request_args", lambda _request, _schema: (None, "bad args"))

    res = _run(module.list_files("tenant1"))
    assert res["code"] == 400
    assert res["message"] == "bad args"


@pytest.mark.p2
def test_move_uses_new_payload_shape(monkeypatch):
    module = _load_file_api_module(monkeypatch)

    async def _validate(_request, _schema):
        return {"src_file_ids": ["f1"], "dest_file_id": "pf2"}, None

    seen = {}

    async def _move_files(tenant_id, src_file_ids, dest_file_id=None, new_name=None):
        seen["args"] = (tenant_id, src_file_ids, dest_file_id, new_name)
        return True, True

    monkeypatch.setattr(module, "validate_and_parse_json_request", _validate)
    monkeypatch.setattr(module.file_api_service, "move_files", _move_files)

    res = _run(module.move("tenant1"))
    assert seen["args"] == ("tenant1", ["f1"], "pf2", None)
    assert res["code"] == 0
    assert res["data"] is True


@pytest.mark.p2
def test_rename_via_move_route(monkeypatch):
    module = _load_file_api_module(monkeypatch)

    async def _validate(_request, _schema):
        return {"src_file_ids": ["file1"], "new_name": "renamed.txt"}, None

    seen = {}

    async def _move_files(tenant_id, src_file_ids, dest_file_id=None, new_name=None):
        seen["args"] = (tenant_id, src_file_ids, dest_file_id, new_name)
        return True, True

    monkeypatch.setattr(module, "validate_and_parse_json_request", _validate)
    monkeypatch.setattr(module.file_api_service, "move_files", _move_files)

    res = _run(module.move("tenant1"))
    assert seen["args"] == ("tenant1", ["file1"], None, "renamed.txt")
    assert res["code"] == 0
    assert res["data"] is True


@pytest.mark.p2
def test_download_falls_back_to_document_storage(monkeypatch):
    module = _load_file_api_module(monkeypatch)
    storage_calls = []

    def _get(bucket, location):
        storage_calls.append((bucket, location))
        return b"" if len(storage_calls) == 1 else b"fallback-blob"

    monkeypatch.setattr(module.settings, "STORAGE_IMPL", SimpleNamespace(get=_get))
    res = _run(module.download("tenant1", "file1"))

    assert storage_calls == [("bucket1", "path1"), ("bucket2", "path2")]
    assert res.data == b"fallback-blob"
    assert res.headers["content_type"] == "text/plain"
    assert res.headers["ext"] == "txt"


@pytest.mark.p2
def test_parent_and_ancestors_use_new_routes(monkeypatch):
    module = _load_file_api_module(monkeypatch)

    parent_res = _run(module.parent_folder("tenant1", "file1"))
    ancestors_res = _run(module.ancestors("tenant1", "file1"))

    assert parent_res["code"] == 0
    assert parent_res["data"]["parent_folder"]["id"] == "parent1"
    assert ancestors_res["code"] == 0
    assert ancestors_res["data"]["parent_folders"][0]["id"] == "root"

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

import functools
from copy import deepcopy

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


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
    repo_root = Path(__file__).resolve().parents[3]

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

    common_pkg = ModuleType("api.common")
    common_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.common", common_pkg)

    permission_mod = ModuleType("api.common.check_team_permission")
    permission_mod.check_file_team_permission = lambda *_args, **_kwargs: True
    permission_mod.check_kb_team_permission = lambda *_args, **_kwargs: True
    monkeypatch.setitem(sys.modules, "api.common.check_team_permission", permission_mod)
    common_pkg.check_team_permission = permission_mod

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

        @staticmethod
        def get_parser(_file_type, _file_name, parser_id):
            return parser_id

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
    module_path = repo_root / "api" / "apps" / "restful_apis" / "file2document_api.py"
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

    # Falsy file returns "File not found!" during synchronous validation.
    monkeypatch.setattr(module.FileService, "get_by_ids", lambda _ids: [_FalsyFile("f1", module.FileType.DOC.value)])
    res = _run(module.convert())
    assert res["code"] == 102
    assert res["message"] == "File not found!"

    # Valid file but invalid kb returns "Can't find this dataset!" during synchronous validation.
    monkeypatch.setattr(module.FileService, "get_by_ids", lambda _ids: [_DummyFile("f1", module.FileType.DOC.value)])
    res = _run(module.convert())
    assert res["code"] == 102
    assert res["message"] == "Can't find this dataset!"

    kb = SimpleNamespace(id="kb-1", parser_id="naive", pipeline_id="p1", parser_config={})
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, kb))

    # Unauthorized file access is rejected before scheduling background work.
    monkeypatch.setattr(module, "check_file_team_permission", lambda *_args, **_kwargs: False)
    res = _run(module.convert())
    assert res["code"] == 102
    assert res["message"] == "No authorization."

    # Unauthorized dataset access is rejected before scheduling background work.
    monkeypatch.setattr(module, "check_file_team_permission", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module, "check_kb_team_permission", lambda *_args, **_kwargs: False)
    res = _run(module.convert())
    assert res["code"] == 102
    assert res["message"] == "No authorization."

    # Valid file and kb schedule background work and return data=True immediately.
    monkeypatch.setattr(module, "check_kb_team_permission", lambda *_args, **_kwargs: True)
    res = _run(module.convert())
    assert res["code"] == 0
    assert res["data"] is True

    # Folder expansion schedules background work and returns data=True immediately.
    req_state["file_ids"] = ["folder-1"]
    monkeypatch.setattr(module.FileService, "get_by_ids", lambda _ids: [_DummyFile("folder-1", module.FileType.FOLDER.value, name="folder")])
    monkeypatch.setattr(module.FileService, "get_all_innermost_file_ids", lambda _file_id, _acc: ["inner-1"])
    res = _run(module.convert())
    assert res["code"] == 0
    assert res["data"] is True

    # Exception in file lookup returns 500.
    req_state["file_ids"] = ["f1"]
    monkeypatch.setattr(
        module.FileService,
        "get_by_ids",
        lambda _ids: (_ for _ in ()).throw(RuntimeError("convert boom")),
    )
    res = _run(module.convert())
    assert res["code"] == 500
    assert "convert boom" in res["message"]

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

    async def _move_files(_tenant_id, _src_file_ids, _dest_file_id):
        return True, True

    async def _rename_file(_tenant_id, _file_id, _name):
        return True, True

    file_api_service_mod.upload_file = _upload_file
    file_api_service_mod.create_folder = _create_folder
    file_api_service_mod.list_files = lambda _tenant_id, _args: (True, {"files": [], "total": 0})
    file_api_service_mod.delete_files = _delete_files
    file_api_service_mod.get_root_folder = lambda _tenant_id: (True, {"root_folder": {"id": "root"}})
    file_api_service_mod.move_files = _move_files
    file_api_service_mod.get_file_content = lambda _tenant_id, _file_id: (
        True,
        SimpleNamespace(parent_id="bucket1", location="path1", name="doc.txt", type="doc"),
    )
    file_api_service_mod.get_parent_folder = lambda _file_id: (True, {"parent_folder": {"id": "parent1"}})
    file_api_service_mod.get_all_parent_folders = lambda _file_id: (True, {"parent_folders": [{"id": "root"}]})
    file_api_service_mod.rename_file = _rename_file
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
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    validation_mod = ModuleType("api.utils.validation_utils")
    validation_mod.CreateFolderReq = object
    validation_mod.DeleteFileReq = object
    validation_mod.ListFileReq = object
    validation_mod.MoveFileReq = object
    validation_mod.RenameFileReq = object

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

    res = module.list_files("tenant1")
    assert res["code"] == 400
    assert res["message"] == "bad args"


@pytest.mark.p2
def test_move_uses_new_payload_shape(monkeypatch):
    module = _load_file_api_module(monkeypatch)

    async def _validate(_request, _schema):
        return {"src_file_ids": ["f1"], "dest_file_id": "pf2"}, None

    seen = {}

    async def _move_files(tenant_id, src_file_ids, dest_file_id):
        seen["args"] = (tenant_id, src_file_ids, dest_file_id)
        return True, True

    monkeypatch.setattr(module, "validate_and_parse_json_request", _validate)
    monkeypatch.setattr(module.file_api_service, "move_files", _move_files)

    res = _run(module.move("tenant1"))
    assert seen["args"] == ("tenant1", ["f1"], "pf2")
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

    parent_res = module.parent_folder("tenant1", "file1")
    ancestors_res = module.ancestors("tenant1", "file1")

    assert parent_res["code"] == 0
    assert parent_res["data"]["parent_folder"]["id"] == "parent1"
    assert ancestors_res["code"] == 0
    assert ancestors_res["data"]["parent_folders"][0]["id"] == "root"


@pytest.mark.p2
def test_rename_uses_path_id(monkeypatch):
    module = _load_file_api_module(monkeypatch)

    async def _validate(_request, _schema):
        return {"name": "renamed.txt"}, None

    seen = {}

    async def _rename_file(tenant_id, file_id, name):
        seen["args"] = (tenant_id, file_id, name)
        return True, True

    monkeypatch.setattr(module, "validate_and_parse_json_request", _validate)
    monkeypatch.setattr(module.file_api_service, "rename_file", _rename_file)

    res = _run(module.rename("tenant1", "file1"))
    assert seen["args"] == ("tenant1", "file1", "renamed.txt")
    assert res["code"] == 0
    assert res["data"] is True

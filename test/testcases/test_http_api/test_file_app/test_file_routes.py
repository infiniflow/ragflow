#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
from enum import Enum
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func
        return decorator


def _load_files_app(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

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
        def get_by_id(_doc_id):
            return True, SimpleNamespace(id=_doc_id)

        @staticmethod
        def get_tenant_id(_doc_id):
            return "tenant1"

        @staticmethod
        def remove_document(*_args, **_kwargs):
            return True

        @staticmethod
        def update_by_id(*_args, **_kwargs):
            return True

        @staticmethod
        def insert(_doc):
            return SimpleNamespace(id="doc1")

    document_service_mod.DocumentService = _StubDocumentService
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", document_service_mod)
    services_pkg.document_service = document_service_mod

    file2document_service_mod = ModuleType("api.db.services.file2document_service")

    class _StubFile2DocumentService:
        @staticmethod
        def get_by_file_id(_file_id):
            return []

        @staticmethod
        def delete_by_file_id(*_args, **_kwargs):
            return None

        @staticmethod
        def get_storage_address(**_kwargs):
            return "bucket", "location"

        @staticmethod
        def insert(_data):
            return SimpleNamespace(to_json=lambda: {})

    file2document_service_mod.File2DocumentService = _StubFile2DocumentService
    monkeypatch.setitem(sys.modules, "api.db.services.file2document_service", file2document_service_mod)
    services_pkg.file2document_service = file2document_service_mod

    knowledgebase_service_mod = ModuleType("api.db.services.knowledgebase_service")

    class _StubKnowledgebaseService:
        @staticmethod
        def get_by_id(_kb_id):
            return False, None

    knowledgebase_service_mod.KnowledgebaseService = _StubKnowledgebaseService
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", knowledgebase_service_mod)
    services_pkg.knowledgebase_service = knowledgebase_service_mod

    file_service_mod = ModuleType("api.db.services.file_service")

    class _StubFileService:
        @staticmethod
        def get_root_folder(_tenant_id):
            return {"id": "root"}

        @staticmethod
        def get_by_id(_file_id):
            return True, SimpleNamespace(id=_file_id, parent_id="root", location="file", tenant_id="tenant1")

        @staticmethod
        def get_id_list_by_id(_pf_id, _file_obj_names, _idx, ids):
            return ids

        @staticmethod
        def create_folder(_file, parent_id, _file_obj_names, _len_id_list):
            return SimpleNamespace(id=parent_id)

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
        def get_all_innermost_file_ids(_file_id, _acc):
            return []

        @staticmethod
        def delete_folder_by_pf_id(*_args, **_kwargs):
            return None

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
        def move_file(*_args, **_kwargs):
            return None

        @staticmethod
        def init_knowledgebase_docs(*_args, **_kwargs):
            return None

        @staticmethod
        def get_parser(_file_type, _file_name, parser_id):
            return parser_id

    file_service_mod.FileService = _StubFileService
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service_mod)
    services_pkg.file_service = file_service_mod

    api_utils_mod = ModuleType("api.utils.api_utils")

    def get_json_result(data=None, message="", code=0):
        return {"code": code, "data": data, "message": message}

    async def get_request_json():
        return {}

    def server_error_response(err):
        return {"code": 100, "data": None, "message": str(err)}

    def token_required(func):
        @functools.wraps(func)
        async def _wrapper(*args, **kwargs):
            return await func(*args, **kwargs)

        return _wrapper

    api_utils_mod.get_json_result = get_json_result
    api_utils_mod.get_request_json = get_request_json
    api_utils_mod.server_error_response = server_error_response
    api_utils_mod.token_required = token_required
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    file_utils_mod = ModuleType("api.utils.file_utils")
    file_utils_mod.filename_type = lambda _filename: _FileType.DOC.value
    monkeypatch.setitem(sys.modules, "api.utils.file_utils", file_utils_mod)

    web_utils_mod = ModuleType("api.utils.web_utils")
    web_utils_mod.CONTENT_TYPE_MAP = {"txt": "text/plain", "json": "application/json"}
    web_utils_mod.apply_safe_file_response_headers = lambda response, *_args, **_kwargs: response
    monkeypatch.setitem(sys.modules, "api.utils.web_utils", web_utils_mod)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    common_pkg.settings = SimpleNamespace(
        STORAGE_IMPL=SimpleNamespace(
            obj_exist=lambda *_args, **_kwargs: False,
            put=lambda *_args, **_kwargs: None,
            get=lambda *_args, **_kwargs: b"",
            rm=lambda *_args, **_kwargs: None,
        )
    )
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    misc_utils_mod = ModuleType("common.misc_utils")
    misc_utils_mod.get_uuid = lambda: "uuid"

    async def thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    misc_utils_mod.thread_pool_exec = thread_pool_exec
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    constants_mod = ModuleType("common.constants")

    class _RetCode:
        SUCCESS = 0
        BAD_REQUEST = 400
        NOT_FOUND = 404
        CONFLICT = 409
        SERVER_ERROR = 500

    constants_mod.RetCode = _RetCode
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    module_path = repo_root / "api" / "apps" / "sdk" / "files.py"
    spec = importlib.util.spec_from_file_location("api.apps.sdk.files", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, "api.apps.sdk.files", module)
    spec.loader.exec_module(module)
    return module


def _run(coro):
    return asyncio.run(coro)


class _DummyFile:
    def __init__(self, file_id, file_type, name="doc.txt", tenant_id="tenant1", parent_id="parent1", location=None):
        self.id = file_id
        self.type = file_type
        self.name = name
        self.location = location or name
        self.size = 1
        self.tenant_id = tenant_id
        self.parent_id = parent_id

    def to_json(self):
        return {"id": self.id, "name": self.name, "type": self.type}


class _FalsyFile(_DummyFile):
    def __bool__(self):
        return False


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


class _DummyRequest:
    def __init__(self, *, args=None, form=None, files=None):
        self.args = _Args(args or {})
        self.form = _AwaitableValue(form or {})
        self.files = _AwaitableValue(files if files is not None else _DummyFiles())


class _DummyUploadFile:
    def __init__(self, filename, blob=b"file-bytes"):
        self.filename = filename
        self._blob = blob

    def read(self):
        return self._blob


class _DummyFiles(dict):
    def __init__(self, file_objs=None):
        super().__init__()
        self._file_objs = file_objs or []
        if file_objs is not None:
            self["file"] = self._file_objs

    def getlist(self, key):
        if key == "file":
            return list(self._file_objs)
        return []


class _DummyResponse:
    def __init__(self, data):
        self.data = data
        self.headers = {}


@pytest.mark.p2
class TestFileMoveUnit:
    def test_move_success_and_invalid_parent(self, monkeypatch):
        module = _load_files_app(monkeypatch)
        file_id = "file1"
        parent_id = "parent1"

        async def fake_request_json():
            return {"src_file_ids": [file_id], "dest_file_id": parent_id}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        monkeypatch.setattr(module.FileService, "get_by_ids", lambda _ids: [_DummyFile(file_id, module.FileType.DOC.value)])
        monkeypatch.setattr(module.FileService, "get_by_id", lambda _pid: (True, _DummyFile(parent_id, module.FileType.FOLDER.value)))
        monkeypatch.setattr(module.FileService, "move_file", lambda *_args, **_kwargs: None)

        res = _run(module.move.__wrapped__("tenant1"))
        assert res["code"] == 0
        assert res["data"] is True

        monkeypatch.setattr(module.FileService, "get_by_id", lambda _pid: (False, None))
        res = _run(module.move.__wrapped__("tenant1"))
        assert res["code"] == 404
        assert res["message"] == "Parent Folder not found!"

    def test_move_missing_payload(self, monkeypatch):
        module = _load_files_app(monkeypatch)

        async def fake_request_json():
            return {}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.move.__wrapped__("tenant1"))
        assert res["code"] == 100

    def test_move_missing_source_branch(self, monkeypatch):
        module = _load_files_app(monkeypatch)

        async def fake_request_json():
            return {"src_file_ids": ["file1"], "dest_file_id": "parent1"}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        monkeypatch.setattr(module.FileService, "get_by_ids", lambda _ids: [_FalsyFile("file1", module.FileType.DOC.value)])
        res = _run(module.move.__wrapped__("tenant1"))
        assert res["code"] == 404
        assert res["message"] == "File or Folder not found!"


@pytest.mark.p2
class TestFileConvertUnit:
    def test_convert_success_and_delete(self, monkeypatch):
        module = _load_files_app(monkeypatch)
        file_id = "file1"
        kb_id = "kb1"

        async def fake_request_json():
            return {"kb_ids": [kb_id], "file_ids": [file_id]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        monkeypatch.setattr(module.FileService, "get_by_ids", lambda _ids: [_DummyFile(file_id, module.FileType.DOC.value)])

        class _Inform:
            document_id = "doc1"

        monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _id: [_Inform()])
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (True, _DummyFile("doc1", module.FileType.DOC.value)))
        monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _doc_id: "tenant1")
        monkeypatch.setattr(module.DocumentService, "remove_document", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.File2DocumentService, "delete_by_file_id", lambda *_args, **_kwargs: None)

        class _Kb:
            id = kb_id
            parser_id = "parser"
            parser_config = {}

        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _Kb()))
        monkeypatch.setattr(module.FileService, "get_by_id", lambda _id: (True, _DummyFile(file_id, module.FileType.DOC.value)))

        class _Doc:
            def __init__(self, doc_id):
                self.id = doc_id

        monkeypatch.setattr(module.DocumentService, "insert", lambda _doc: _Doc("newdoc"))

        class _File2Doc:
            def to_json(self):
                return {"file_id": file_id, "document_id": "newdoc"}

        monkeypatch.setattr(module.File2DocumentService, "insert", lambda _data: _File2Doc())

        res = _run(module.convert.__wrapped__("tenant1"))
        assert res["code"] == 0
        assert res["data"]

    def test_convert_folder(self, monkeypatch):
        module = _load_files_app(monkeypatch)
        kb_id = "kb1"

        async def fake_request_json():
            return {"kb_ids": [kb_id], "file_ids": ["folder1"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        monkeypatch.setattr(module.FileService, "get_by_ids", lambda _ids: [_DummyFile("folder1", module.FileType.FOLDER.value, name="folder")])
        monkeypatch.setattr(module.FileService, "get_all_innermost_file_ids", lambda *_args, **_kwargs: ["inner1"])
        monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _id: [])
        monkeypatch.setattr(module.File2DocumentService, "delete_by_file_id", lambda *_args, **_kwargs: None)

        class _Kb:
            id = kb_id
            parser_id = "parser"
            parser_config = {}

        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _Kb()))
        monkeypatch.setattr(module.FileService, "get_by_id", lambda _id: (True, _DummyFile("inner1", module.FileType.DOC.value)))
        monkeypatch.setattr(module.DocumentService, "insert", lambda _doc: _DummyFile("doc1", module.FileType.DOC.value))
        monkeypatch.setattr(module.File2DocumentService, "insert", lambda _data: SimpleNamespace(to_json=lambda: {"file_id": "inner1"}))

        res = _run(module.convert.__wrapped__("tenant1"))
        assert res["code"] == 0
        assert res["data"]

    def test_convert_invalid_file_id(self, monkeypatch):
        module = _load_files_app(monkeypatch)

        async def fake_request_json():
            return {"kb_ids": ["kb1"], "file_ids": ["missing"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        monkeypatch.setattr(module.FileService, "get_by_ids", lambda _ids: [_FalsyFile("missing", module.FileType.DOC.value)])
        res = _run(module.convert.__wrapped__("tenant1"))
        assert res["code"] == 404
        assert res["message"] == "File not found!"

    def test_convert_invalid_kb_id(self, monkeypatch):
        module = _load_files_app(monkeypatch)

        async def fake_request_json():
            return {"kb_ids": ["missing"], "file_ids": ["file1"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        monkeypatch.setattr(module.FileService, "get_by_ids", lambda _ids: [_DummyFile("file1", module.FileType.DOC.value)])
        monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _id: [])
        monkeypatch.setattr(module.File2DocumentService, "delete_by_file_id", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
        res = _run(module.convert.__wrapped__("tenant1"))
        assert res["code"] == 404
        assert res["message"] == "Can't find this dataset!"

    def test_convert_file_missing_second_lookup(self, monkeypatch):
        module = _load_files_app(monkeypatch)

        async def fake_request_json():
            return {"kb_ids": ["kb1"], "file_ids": ["file1"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        monkeypatch.setattr(module.FileService, "get_by_ids", lambda _ids: [_DummyFile("file1", module.FileType.DOC.value)])
        monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _id: [])
        monkeypatch.setattr(module.File2DocumentService, "delete_by_file_id", lambda *_args, **_kwargs: None)

        class _Kb:
            id = "kb1"
            parser_id = "parser"
            parser_config = {}

        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _Kb()))
        monkeypatch.setattr(module.FileService, "get_by_id", lambda _id: (False, None))
        res = _run(module.convert.__wrapped__("tenant1"))
        assert res["code"] == 404
        assert res["message"] == "Can't find this file!"

    def test_convert_missing_payload(self, monkeypatch):
        module = _load_files_app(monkeypatch)

        async def fake_request_json():
            return {}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        with pytest.raises(KeyError):
            _run(module.convert.__wrapped__("tenant1"))


@pytest.mark.p2
class TestFileRouteBranchUnit:
    def test_upload_branch_matrix(self, monkeypatch):
        module = _load_files_app(monkeypatch)
        monkeypatch.setattr(module.FileService, "get_root_folder", lambda _tenant_id: {"id": "root"})

        # Missing file part.
        monkeypatch.setattr(module, "request", _DummyRequest(form={}, files=_DummyFiles()))
        res = _run(module.upload.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.BAD_REQUEST
        assert res["message"] == "No file part!"

        # Empty filename.
        monkeypatch.setattr(
            module,
            "request",
            _DummyRequest(form={"parent_id": "pf1"}, files=_DummyFiles([_DummyUploadFile("")])),
        )
        res = _run(module.upload.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.BAD_REQUEST
        assert res["message"] == "No selected file!"

        # Parent folder missing.
        monkeypatch.setattr(
            module,
            "request",
            _DummyRequest(form={"parent_id": "pf1"}, files=_DummyFiles([_DummyUploadFile("a.txt")])),
        )
        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
        res = _run(module.upload.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "Can't find this folder!"

        # Missing folder in branch: file_len != len_id_list.
        monkeypatch.setattr(
            module,
            "request",
            _DummyRequest(form={"parent_id": "pf1"}, files=_DummyFiles([_DummyUploadFile("dir/a.txt")])),
        )
        monkeypatch.setattr(module.FileService, "get_id_list_by_id", lambda *_args, **_kwargs: ["pf1", "missing-child"])

        def get_by_id_missing_child(file_id):
            if file_id == "missing-child":
                return False, None
            return True, SimpleNamespace(id="pf1")

        monkeypatch.setattr(module.FileService, "get_by_id", get_by_id_missing_child)
        res = _run(module.upload.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "Folder not found!"

        # Missing folder in branch: file_len == len_id_list.
        monkeypatch.setattr(
            module,
            "request",
            _DummyRequest(form={"parent_id": "pf1"}, files=_DummyFiles([_DummyUploadFile("b.txt")])),
        )
        monkeypatch.setattr(module.FileService, "get_id_list_by_id", lambda *_args, **_kwargs: ["pf1", "leaf"])
        pf1_calls = {"count": 0}

        def get_by_id_missing_parent_in_else(file_id):
            if file_id == "pf1":
                pf1_calls["count"] += 1
                if pf1_calls["count"] == 1:
                    return True, SimpleNamespace(id="pf1")
                return False, None
            return True, SimpleNamespace(id=file_id)

        monkeypatch.setattr(module.FileService, "get_by_id", get_by_id_missing_parent_in_else)
        res = _run(module.upload.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "Folder not found!"

        class _Storage:
            def __init__(self):
                self.obj_calls = 0
                self.put_calls = []

            def obj_exist(self, _bucket, _location):
                self.obj_calls += 1
                return self.obj_calls == 1

            def put(self, bucket, location, blob):
                self.put_calls.append((bucket, location, blob))

        storage = _Storage()
        monkeypatch.setattr(module.settings, "STORAGE_IMPL", storage)
        monkeypatch.setattr(
            module,
            "request",
            _DummyRequest(
                form={"parent_id": "pf1"},
                files=_DummyFiles([_DummyUploadFile("dir/a.txt", b"a"), _DummyUploadFile("b.txt", b"b")]),
            ),
        )

        def fake_get_by_id(file_id):
            if file_id == "mid-id":
                return True, SimpleNamespace(id="mid-id")
            return True, SimpleNamespace(id="pf1")

        def fake_get_id_list_by_id(_pf_id, file_obj_names, _idx, _ids):
            if file_obj_names[-1] == "a.txt":
                return ["pf1", "mid-id"]
            return ["pf1", "leaf-id"]

        def fake_create_folder(_file, parent_id, _file_obj_names, _len_id_list):
            return SimpleNamespace(id=f"{parent_id}-folder")

        monkeypatch.setattr(module.FileService, "get_by_id", fake_get_by_id)
        monkeypatch.setattr(module.FileService, "get_id_list_by_id", fake_get_id_list_by_id)
        monkeypatch.setattr(module.FileService, "create_folder", fake_create_folder)
        monkeypatch.setattr(module, "filename_type", lambda _name: module.FileType.DOC.value)
        monkeypatch.setattr(module, "duplicate_name", lambda _query, **kwargs: kwargs["name"])
        monkeypatch.setattr(module, "get_uuid", lambda: "file-id")
        monkeypatch.setattr(module.FileService, "query", lambda **_kwargs: [])
        monkeypatch.setattr(module.FileService, "insert", lambda data: SimpleNamespace(to_json=lambda: {"id": data["id"]}))
        res = _run(module.upload.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.SUCCESS
        assert len(res["data"]) == 2
        assert storage.put_calls

        # Exception path.
        monkeypatch.setattr(
            module,
            "request",
            _DummyRequest(form={"parent_id": "pf1"}, files=_DummyFiles([_DummyUploadFile("boom.txt")])),
        )
        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (_ for _ in ()).throw(RuntimeError("upload boom")))
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = _run(module.upload.__wrapped__("tenant1"))
        assert res["code"] == 500
        assert "upload boom" in res["message"]

    def test_create_branch_matrix(self, monkeypatch):
        module = _load_files_app(monkeypatch)
        state = {"req": {"name": "file1"}}

        async def fake_request_json():
            return state["req"]

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        monkeypatch.setattr(module.FileService, "get_root_folder", lambda _tenant_id: {"id": "root"})
        monkeypatch.setattr(module.FileService, "is_parent_folder_exist", lambda _pf_id: False)
        res = _run(module.create.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.BAD_REQUEST
        assert "Parent Folder Doesn't Exist!" in res["message"]

        state["req"] = {"name": "dup", "parent_id": "pf1"}
        monkeypatch.setattr(module.FileService, "is_parent_folder_exist", lambda _pf_id: True)
        monkeypatch.setattr(module.FileService, "query", lambda **_kwargs: [object()])
        res = _run(module.create.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.CONFLICT
        assert "Duplicated folder name" in res["message"]

        inserted = {}

        def fake_insert(data):
            inserted["payload"] = data
            return SimpleNamespace(to_json=lambda: data)

        monkeypatch.setattr(module.FileService, "query", lambda **_kwargs: [])
        monkeypatch.setattr(module, "get_uuid", lambda: "uuid-folder")
        monkeypatch.setattr(module.FileService, "insert", fake_insert)

        state["req"] = {"name": "folder", "parent_id": "pf1", "type": module.FileType.FOLDER.value}
        res = _run(module.create.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.SUCCESS
        assert inserted["payload"]["type"] == module.FileType.FOLDER.value

        state["req"] = {"name": "virtual", "parent_id": "pf1", "type": "UNKNOWN"}
        res = _run(module.create.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.SUCCESS
        assert inserted["payload"]["type"] == module.FileType.VIRTUAL.value

        monkeypatch.setattr(module.FileService, "is_parent_folder_exist", lambda _pf_id: (_ for _ in ()).throw(RuntimeError("create boom")))
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = _run(module.create.__wrapped__("tenant1"))
        assert res["code"] == 500
        assert "create boom" in res["message"]

    def test_list_files_branch_matrix(self, monkeypatch):
        module = _load_files_app(monkeypatch)
        calls = {"init": 0}

        monkeypatch.setattr(module.FileService, "get_root_folder", lambda _tenant_id: {"id": "root"})
        monkeypatch.setattr(
            module.FileService,
            "init_knowledgebase_docs",
            lambda _pf_id, _tenant_id: calls.__setitem__("init", calls["init"] + 1),
        )
        monkeypatch.setattr(module, "request", _DummyRequest(args={}))
        monkeypatch.setattr(module.FileService, "get_by_id", lambda _pf_id: (False, None))
        res = _run(module.list_files.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "Folder not found!"
        assert calls["init"] == 1

        monkeypatch.setattr(
            module,
            "request",
            _DummyRequest(args={"parent_id": "p1", "keywords": "k", "page": "2", "page_size": "10", "orderby": "name", "desc": "False"}),
        )
        monkeypatch.setattr(module.FileService, "get_by_id", lambda _pf_id: (True, SimpleNamespace(id="p1")))
        monkeypatch.setattr(module.FileService, "get_by_pf_id", lambda *_args, **_kwargs: ([{"id": "f1"}], 1))
        monkeypatch.setattr(module.FileService, "get_parent_folder", lambda _pf_id: None)
        res = _run(module.list_files.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "File not found!"

        monkeypatch.setattr(module.FileService, "get_parent_folder", lambda _pf_id: SimpleNamespace(to_json=lambda: {"id": "p0"}))
        res = _run(module.list_files.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.SUCCESS
        assert res["data"]["total"] == 1
        assert res["data"]["parent_folder"]["id"] == "p0"

        monkeypatch.setattr(module.FileService, "get_by_id", lambda _pf_id: (_ for _ in ()).throw(RuntimeError("list boom")))
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = _run(module.list_files.__wrapped__("tenant1"))
        assert res["code"] == 500
        assert "list boom" in res["message"]

    def test_get_root_folder_branch_matrix(self, monkeypatch):
        module = _load_files_app(monkeypatch)
        monkeypatch.setattr(module.FileService, "get_root_folder", lambda _tenant_id: {"id": "root"})
        res = _run(module.get_root_folder.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.SUCCESS
        assert res["data"]["root_folder"]["id"] == "root"

        monkeypatch.setattr(module.FileService, "get_root_folder", lambda _tenant_id: (_ for _ in ()).throw(RuntimeError("root boom")))
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = _run(module.get_root_folder.__wrapped__("tenant1"))
        assert res["code"] == 500
        assert "root boom" in res["message"]

    def test_get_parent_folder_branch_matrix(self, monkeypatch):
        module = _load_files_app(monkeypatch)
        monkeypatch.setattr(module, "request", _DummyRequest(args={"file_id": "missing"}))
        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
        res = _run(module.get_parent_folder.__wrapped__())
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "Folder not found!"

        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (True, SimpleNamespace(id="f1")))
        monkeypatch.setattr(module.FileService, "get_parent_folder", lambda _file_id: SimpleNamespace(to_json=lambda: {"id": "p1"}))
        res = _run(module.get_parent_folder.__wrapped__())
        assert res["code"] == module.RetCode.SUCCESS
        assert res["data"]["parent_folder"]["id"] == "p1"

        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (_ for _ in ()).throw(RuntimeError("parent boom")))
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = _run(module.get_parent_folder.__wrapped__())
        assert res["code"] == 500
        assert "parent boom" in res["message"]

    def test_get_all_parent_folders_branch_matrix(self, monkeypatch):
        module = _load_files_app(monkeypatch)
        monkeypatch.setattr(module, "request", _DummyRequest(args={"file_id": "missing"}))
        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
        res = _run(module.get_all_parent_folders.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "Folder not found!"

        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (True, SimpleNamespace(id="f1")))
        monkeypatch.setattr(
            module.FileService,
            "get_all_parent_folders",
            lambda _file_id: [SimpleNamespace(to_json=lambda: {"id": "p1"})],
        )
        res = _run(module.get_all_parent_folders.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.SUCCESS
        assert res["data"]["parent_folders"] == [{"id": "p1"}]

        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (_ for _ in ()).throw(RuntimeError("all parent boom")))
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = _run(module.get_all_parent_folders.__wrapped__("tenant1"))
        assert res["code"] == 500
        assert "all parent boom" in res["message"]

    def test_rm_branch_matrix(self, monkeypatch):
        module = _load_files_app(monkeypatch)
        req_state = {"file_ids": ["f1"]}

        async def fake_request_json():
            return req_state

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        monkeypatch.setattr(module.settings, "STORAGE_IMPL", SimpleNamespace(rm=lambda *_args, **_kwargs: None))

        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
        res = _run(module.rm.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "File or Folder not found!"

        monkeypatch.setattr(
            module.FileService,
            "get_by_id",
            lambda _file_id: (True, _DummyFile(_file_id, module.FileType.DOC.value, tenant_id=None)),
        )
        res = _run(module.rm.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "Tenant not found!"

        req_state["file_ids"] = ["folder1"]

        def folder_missing_inner(file_id):
            if file_id == "folder1":
                return True, _DummyFile("folder1", module.FileType.FOLDER.value, parent_id="pf1")
            if file_id == "inner1":
                return False, None
            return False, None

        monkeypatch.setattr(module.FileService, "get_by_id", folder_missing_inner)
        monkeypatch.setattr(module.FileService, "get_all_innermost_file_ids", lambda _file_id, _acc: ["inner1"])
        res = _run(module.rm.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "File not found!"

        req_state["file_ids"] = ["doc1"]
        monkeypatch.setattr(
            module.FileService,
            "get_by_id",
            lambda _file_id: (True, _DummyFile("doc1", module.FileType.DOC.value, parent_id="pf1")),
        )
        monkeypatch.setattr(module.FileService, "delete", lambda _file: False)
        res = _run(module.rm.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.SERVER_ERROR
        assert "Database error (File removal)!" in res["message"]

        class _Inform:
            document_id = "doc1"

        monkeypatch.setattr(module.FileService, "delete", lambda _file: True)
        monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _file_id: [_Inform()])
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (False, None))
        res = _run(module.rm.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "Document not found!"

        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (True, SimpleNamespace(id=_doc_id)))
        monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _doc_id: None)
        res = _run(module.rm.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "Tenant not found!"

        monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _doc_id: "tenant1")
        monkeypatch.setattr(module.DocumentService, "remove_document", lambda *_args, **_kwargs: False)
        res = _run(module.rm.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.SERVER_ERROR
        assert "Database error (Document removal)!" in res["message"]

        req_state["file_ids"] = ["folder-ok"]
        deleted = {"folder": 0, "link": 0}

        def folder_success(file_id):
            if file_id == "folder-ok":
                return True, _DummyFile("folder-ok", module.FileType.FOLDER.value, parent_id="pf1")
            if file_id == "inner-ok":
                return True, _DummyFile("inner-ok", module.FileType.DOC.value, parent_id="pf1", location="inner.bin")
            return False, None

        monkeypatch.setattr(module.FileService, "get_by_id", folder_success)
        monkeypatch.setattr(module.FileService, "get_all_innermost_file_ids", lambda _file_id, _acc: ["inner-ok"])
        monkeypatch.setattr(
            module.FileService,
            "delete_folder_by_pf_id",
            lambda _tenant_id, _file_id: deleted.__setitem__("folder", deleted["folder"] + 1),
        )
        monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _file_id: [])
        monkeypatch.setattr(
            module.File2DocumentService,
            "delete_by_file_id",
            lambda _file_id: deleted.__setitem__("link", deleted["link"] + 1),
        )
        res = _run(module.rm.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.SUCCESS
        assert res["data"] is True
        assert deleted == {"folder": 1, "link": 1}

        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (_ for _ in ()).throw(RuntimeError("rm boom")))
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        req_state["file_ids"] = ["boom"]
        res = _run(module.rm.__wrapped__("tenant1"))
        assert res["code"] == 500
        assert "rm boom" in res["message"]

    def test_rename_branch_matrix(self, monkeypatch):
        module = _load_files_app(monkeypatch)
        req_state = {"file_id": "f1", "name": "new.txt"}

        async def fake_request_json():
            return req_state

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
        res = _run(module.rename.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "File not found!"

        monkeypatch.setattr(
            module.FileService,
            "get_by_id",
            lambda _file_id: (True, _DummyFile("f1", module.FileType.DOC.value, name="origin.txt")),
        )
        req_state["name"] = "new.pdf"
        res = _run(module.rename.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.BAD_REQUEST
        assert "extension of file can't be changed" in res["message"]

        req_state["name"] = "new.txt"
        monkeypatch.setattr(module.FileService, "query", lambda **_kwargs: [SimpleNamespace(name="new.txt")])
        res = _run(module.rename.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.CONFLICT
        assert "Duplicated file name in the same folder." in res["message"]

        monkeypatch.setattr(module.FileService, "query", lambda **_kwargs: [])
        monkeypatch.setattr(module.FileService, "update_by_id", lambda *_args, **_kwargs: False)
        res = _run(module.rename.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.SERVER_ERROR
        assert "Database error (File rename)!" in res["message"]

        monkeypatch.setattr(module.FileService, "update_by_id", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _file_id: [SimpleNamespace(document_id="doc1")])
        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: False)
        res = _run(module.rename.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.SERVER_ERROR
        assert "Database error (Document rename)!" in res["message"]

        monkeypatch.setattr(module.File2DocumentService, "get_by_file_id", lambda _file_id: [])
        res = _run(module.rename.__wrapped__("tenant1"))
        assert res["code"] == module.RetCode.SUCCESS
        assert res["data"] is True

        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (_ for _ in ()).throw(RuntimeError("rename boom")))
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = _run(module.rename.__wrapped__("tenant1"))
        assert res["code"] == 500
        assert "rename boom" in res["message"]

    def test_get_file_branch_matrix(self, monkeypatch):
        module = _load_files_app(monkeypatch)
        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))
        res = _run(module.get.__wrapped__("tenant1", "missing"))
        assert res["code"] == module.RetCode.NOT_FOUND
        assert res["message"] == "Document not found!"

        class _Storage:
            def __init__(self):
                self.calls = 0

            def get(self, _bucket, _location):
                self.calls += 1
                if self.calls == 1:
                    return None
                return b"blob-data"

        storage = _Storage()
        monkeypatch.setattr(module.settings, "STORAGE_IMPL", storage)
        monkeypatch.setattr(
            module.FileService,
            "get_by_id",
            lambda _file_id: (True, _DummyFile("f1", module.FileType.VISUAL.value, name="image.abc", parent_id="pf1", location="loc1")),
        )
        monkeypatch.setattr(module.File2DocumentService, "get_storage_address", lambda **_kwargs: ("pf2", "loc2"))

        async def fake_make_response(data):
            return _DummyResponse(data)

        monkeypatch.setattr(module, "make_response", fake_make_response)
        monkeypatch.setattr(
            module,
            "apply_safe_file_response_headers",
            lambda response, content_type, extension: response.headers.update(
                {"content_type": content_type, "extension": extension}
            ),
        )
        res = _run(module.get.__wrapped__("tenant1", "f1"))
        assert isinstance(res, _DummyResponse)
        assert res.data == b"blob-data"
        assert res.headers["extension"] == "abc"
        assert res.headers["content_type"] == "image/abc"

        monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (_ for _ in ()).throw(RuntimeError("get boom")))
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = _run(module.get.__wrapped__("tenant1", "f1"))
        assert res["code"] == 500
        assert "get boom" in res["message"]

    def test_download_attachment_branch_matrix(self, monkeypatch):
        module = _load_files_app(monkeypatch)
        monkeypatch.setattr(module, "request", _DummyRequest(args={"ext": "abc"}))

        async def fake_thread_pool_exec(_fn, _tenant_id, _attachment_id):
            return b"attachment"

        async def fake_make_response(data):
            return _DummyResponse(data)

        monkeypatch.setattr(module, "thread_pool_exec", fake_thread_pool_exec)
        monkeypatch.setattr(module, "make_response", fake_make_response)
        monkeypatch.setattr(
            module,
            "apply_safe_file_response_headers",
            lambda response, content_type, extension: response.headers.update(
                {"content_type": content_type, "extension": extension}
            ),
        )
        res = _run(module.download_attachment.__wrapped__("tenant1", "att1"))
        assert isinstance(res, _DummyResponse)
        assert res.data == b"attachment"
        assert res.headers["extension"] == "abc"
        assert res.headers["content_type"] == "application/abc"

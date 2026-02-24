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
import importlib.util
import sys
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
    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    module_path = repo_root / "api" / "apps" / "sdk" / "files.py"
    spec = importlib.util.spec_from_file_location("test_files_app_unit", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


def _run(coro):
    return asyncio.run(coro)


class _DummyFile:
    def __init__(self, file_id, file_type, name="doc.txt", tenant_id="tenant1"):
        self.id = file_id
        self.type = file_type
        self.name = name
        self.location = name
        self.size = 1
        self.tenant_id = tenant_id


class _FalsyFile(_DummyFile):
    def __bool__(self):
        return False


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
        res = _run(module.convert.__wrapped__("tenant1"))
        assert res["code"] == 100

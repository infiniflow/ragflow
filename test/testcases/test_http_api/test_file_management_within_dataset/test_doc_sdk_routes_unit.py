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
from pathlib import Path
from types import ModuleType, SimpleNamespace

import numpy as np
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
    def getlist(self, key):
        return self.get(key, [])


class _DummyArgs(dict):
    def getlist(self, key):
        v = self.get(key, [])
        if v is None:
            return []
        if isinstance(v, list):
            return v
        return [v]


class _DummyDoc:
    def __init__(
        self,
        *,
        doc_id="doc-1",
        kb_id="kb-1",
        name="doc.txt",
        chunk_num=1,
        token_num=2,
        progress=0,
        process_duration=0,
        parser_id="naive",
        doc_type=1,
        status=True,
        run=0,
    ):
        self.id = doc_id
        self.kb_id = kb_id
        self.name = name
        self.chunk_num = chunk_num
        self.token_num = token_num
        self.progress = progress
        self.process_duration = process_duration
        self.parser_id = parser_id
        self.type = doc_type
        self.status = status
        self.run = run

    def to_dict(self):
        return {
            "id": self.id,
            "kb_id": self.kb_id,
            "name": self.name,
            "chunk_num": self.chunk_num,
            "token_num": self.token_num,
            "progress": self.progress,
            "process_duration": self.process_duration,
            "parser_id": self.parser_id,
            "run": self.run,
            "status": self.status,
        }


class _ToggleBoolDocList:
    def __init__(self, value):
        self._calls = 0
        self._value = value

    def __getitem__(self, item):
        return self._value

    def __bool__(self):
        self._calls += 1
        return self._calls == 1


def _run(coro):
    return asyncio.run(coro)


def _load_doc_module(monkeypatch):
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

    module_path = repo_root / "api" / "apps" / "sdk" / "doc.py"
    spec = importlib.util.spec_from_file_location("test_doc_sdk_routes_unit", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


def _patch_send_file(monkeypatch, module):
    async def _fake_send_file(file_obj, **kwargs):
        return {"file": file_obj, "filename": kwargs.get("attachment_filename")}

    monkeypatch.setattr(module, "send_file", _fake_send_file)


def _patch_storage(monkeypatch, module, *, file_stream=b"abc"):
    storage = SimpleNamespace(get=lambda *_args, **_kwargs: file_stream, rm=lambda *_args, **_kwargs: None)
    monkeypatch.setattr(module.settings, "STORAGE_IMPL", storage)


def _patch_docstore(monkeypatch, module, **kwargs):
    defaults = {
        "delete": lambda *_args, **_kwargs: 0,
        "update": lambda *_args, **_kwargs: None,
        "get": lambda *_args, **_kwargs: {},
        "insert": lambda *_args, **_kwargs: None,
        "index_exist": lambda *_args, **_kwargs: False,
    }
    defaults.update(kwargs)
    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(**defaults))


@pytest.mark.p2
class TestDocRoutesUnit:
    def test_chunk_positions_validation_error(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        with pytest.raises(ValueError) as exc_info:
            module.Chunk(positions=[[1, 2, 3, 4]])
        assert "length of 5" in str(exc_info.value)

    def test_upload_validation_and_upload_error(self, monkeypatch):
        module = _load_doc_module(monkeypatch)

        class _FileObj:
            def __init__(self, name):
                self.filename = name

        monkeypatch.setattr(module, "request", SimpleNamespace(form=_AwaitableValue({}), files=_AwaitableValue(_DummyFiles({"file": [_FileObj("")]}))))
        res = _run(module.upload.__wrapped__("ds-1", "tenant-1"))
        assert res["code"] == module.RetCode.ARGUMENT_ERROR
        assert res["message"] == "No file selected!"

        long_name = "a" * (module.FILE_NAME_LEN_LIMIT + 1)
        monkeypatch.setattr(module, "request", SimpleNamespace(form=_AwaitableValue({}), files=_AwaitableValue(_DummyFiles({"file": [_FileObj(long_name)]}))))
        res = _run(module.upload.__wrapped__("ds-1", "tenant-1"))
        assert res["code"] == module.RetCode.ARGUMENT_ERROR
        assert "bytes or less" in res["message"]

        monkeypatch.setattr(module, "request", SimpleNamespace(form=_AwaitableValue({}), files=_AwaitableValue(_DummyFiles({"file": [_FileObj("ok.txt")]}))))
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, SimpleNamespace()))
        monkeypatch.setattr(module.FileService, "upload_document", lambda *_args, **_kwargs: (["upload failed"], []))
        res = _run(module.upload.__wrapped__("ds-1", "tenant-1"))
        assert res["code"] == module.RetCode.SERVER_ERROR
        assert res["message"] == "upload failed"

    def test_update_doc_guards_and_error_paths(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        doc = _DummyDoc()
        monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [])
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({}))
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["message"] == "You don't own the dataset."

        monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [1])
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (False, None))
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["message"] == "Can't find this dataset!"

        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, SimpleNamespace(tenant_id="tenant-1")))
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "doesn't own the document" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [doc])
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"chunk_count": 100}))
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "chunk_count" in res["message"]

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"token_count": 100}))
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "token_count" in res["message"]

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"progress": 100}))
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "progress" in res["message"]

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"meta_fields": []}))
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["message"] == "meta_fields must be a dictionary"

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"meta_fields": {"k": "v"}}))
        monkeypatch.setattr(module.DocMetadataService, "update_document_metadata", lambda *_args, **_kwargs: False)
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["message"] == "Failed to update metadata"

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"name": "a" * (module.FILE_NAME_LEN_LIMIT + 1)}))
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["code"] == module.RetCode.ARGUMENT_ERROR
        assert "bytes or less" in res["message"]

        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: False)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"name": "new.txt"}))
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "Document rename" in res["message"]

    def test_update_doc_chunk_method_enabled_and_db_error(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        visual_doc = _DummyDoc(parser_id="naive", doc_type=module.FileType.VISUAL)
        kb = SimpleNamespace(tenant_id="tenant-1")
        monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [1])
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, kb))
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [visual_doc])
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"chunk_method": "naive"}))
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["message"] == "Not supported yet!"

        doc = _DummyDoc(token_num=2, chunk_num=1, parser_id="naive")
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [doc])
        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: False)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"chunk_method": "manual"}))
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["message"] == "Document not found!"

        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "update_parser_config", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module.DocumentService, "increment_chunk_num", lambda *_args, **_kwargs: False)
        _patch_docstore(monkeypatch, module, delete=lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"chunk_method": "manual"}))
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["message"] == "Document not found!"

        monkeypatch.setattr(module.DocumentService, "increment_chunk_num", lambda *_args, **_kwargs: True)
        doc_for_enabled = _DummyDoc(status=False)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [doc_for_enabled])
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _id: (True, doc_for_enabled))
        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: False)
        _patch_docstore(monkeypatch, module, update=lambda *_args, **_kwargs: None, delete=lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"enabled": True}))
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "Document update" in res["message"]

        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: True)
        _patch_docstore(monkeypatch, module, update=lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("boom")), delete=lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["code"] == 500
        assert "boom" in res["message"]

        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _id: (False, None))
        _patch_docstore(monkeypatch, module, update=lambda *_args, **_kwargs: None, delete=lambda *_args, **_kwargs: None)
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["message"] == "Dataset created failed"

        # cover token reset + docStore deletion branch
        doc_reset = _DummyDoc(token_num=3, chunk_num=2, parser_id="naive", run=0)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [doc_reset])
        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "increment_chunk_num", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _id: (True, doc_reset))
        _patch_docstore(monkeypatch, module, delete=lambda *_args, **_kwargs: None, update=lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"chunk_method": "manual"}))
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["code"] == 0

        def _raise_operational_error(_id):
            raise module.OperationalError("db down")

        monkeypatch.setattr(module.DocumentService, "get_by_id", _raise_operational_error)
        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["message"] == "Database operation failed"

    def test_download_and_download_doc_errors(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        _patch_send_file(monkeypatch, module)
        _patch_storage(monkeypatch, module, file_stream=b"")
        res = _run(module.download.__wrapped__("tenant-1", "ds-1", ""))
        assert res["message"] == "Specify document_id please."
        monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [])
        res = _run(module.download.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "do not own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [1])
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
        res = _run(module.download.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "not own the document" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc()])
        monkeypatch.setattr(module.File2DocumentService, "get_storage_address", lambda **_kwargs: ("b", "n"))
        res = _run(module.download.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["message"] == "This file is empty."

        monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer"}))
        res = _run(module.download_doc("doc-1"))
        assert "Authorization is not valid" in res["message"]

        monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer token"}))
        monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [])
        res = _run(module.download_doc("doc-1"))
        assert "API key is invalid" in res["message"]

        monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace()])
        res = _run(module.download_doc(""))
        assert res["message"] == "Specify document_id please."

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
        res = _run(module.download_doc("doc-1"))
        assert "not own the document" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc()])
        monkeypatch.setattr(module.File2DocumentService, "get_storage_address", lambda **_kwargs: ("b", "n"))
        _patch_storage(monkeypatch, module, file_stream=b"")
        res = _run(module.download_doc("doc-1"))
        assert res["message"] == "This file is empty."

        _patch_storage(monkeypatch, module, file_stream=b"abc")
        res = _run(module.download_doc("doc-1"))
        assert res["filename"] == "doc.txt"

    def test_list_docs_metadata_filters(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        monkeypatch.setattr(module, "request", SimpleNamespace(args=_DummyArgs()))
        res = module.list_docs.__wrapped__("ds-1", "tenant-1")
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(
            module,
            "request",
            SimpleNamespace(
                args=_DummyArgs(
                    {
                        "metadata_condition": "{bad json",
                    }
                )
            ),
        )
        res = module.list_docs.__wrapped__("ds-1", "tenant-1")
        assert res["message"] == "metadata_condition must be valid JSON."

        monkeypatch.setattr(module, "request", SimpleNamespace(args=_DummyArgs({"metadata_condition": "[1]"})))
        res = module.list_docs.__wrapped__("ds-1", "tenant-1")
        assert res["message"] == "metadata_condition must be an object."

        monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda _kbs: [{"doc_id": "x"}])
        monkeypatch.setattr(module, "meta_filter", lambda *_args, **_kwargs: [])
        monkeypatch.setattr(module, "convert_conditions", lambda cond: cond)
        monkeypatch.setattr(
            module,
            "request",
            SimpleNamespace(args=_DummyArgs({"metadata_condition": '{"conditions":[{"field":"x","op":"eq","value":"y"}]}'})),
        )
        res = module.list_docs.__wrapped__("ds-1", "tenant-1")
        assert res["code"] == module.RetCode.SUCCESS
        assert res["data"]["total"] == 0

        monkeypatch.setattr(
            module.DocumentService,
            "get_list",
            lambda *_args, **_kwargs: ([{"id": "doc-1", "create_time": 100, "run": "0"}], 1),
        )
        monkeypatch.setattr(
            module,
            "request",
            SimpleNamespace(
                args=_DummyArgs(
                    {
                        "create_time_from": "101",
                        "create_time_to": "200",
                    }
                )
            ),
        )
        res = module.list_docs.__wrapped__("ds-1", "tenant-1")
        assert res["code"] == 0
        assert res["data"]["docs"] == []

    def test_metadata_summary_and_batch_update(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        monkeypatch.setattr(module, "convert_conditions", lambda cond: cond)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"selector": {}}))
        res = _run(module.metadata_batch_update.__wrapped__("ds-1", "tenant-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(module.metadata_summary.__wrapped__("ds-1", "tenant-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"doc_ids": ["d1"]}))
        monkeypatch.setattr(module.DocMetadataService, "get_metadata_summary", lambda *_args, **_kwargs: {"k": 1})
        res = _run(module.metadata_summary.__wrapped__("ds-1", "tenant-1"))
        assert res["code"] == 0
        assert res["data"]["summary"] == {"k": 1}

        monkeypatch.setattr(module.DocMetadataService, "get_metadata_summary", lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("x")))
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = _run(module.metadata_summary.__wrapped__("ds-1", "tenant-1"))
        assert res["code"] == 500

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"selector": [1]}))
        res = _run(module.metadata_batch_update.__wrapped__("ds-1", "tenant-1"))
        assert res["message"] == "selector must be an object."

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"selector": {}, "updates": {"k": "v"}, "deletes": []}))
        res = _run(module.metadata_batch_update.__wrapped__("ds-1", "tenant-1"))
        assert res["message"] == "updates and deletes must be lists."

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"selector": {"metadata_condition": [1]}, "updates": [], "deletes": []}),
        )
        res = _run(module.metadata_batch_update.__wrapped__("ds-1", "tenant-1"))
        assert res["message"] == "metadata_condition must be an object."

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"selector": {"document_ids": "doc-1"}, "updates": [], "deletes": []}),
        )
        res = _run(module.metadata_batch_update.__wrapped__("ds-1", "tenant-1"))
        assert res["message"] == "document_ids must be a list."

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"selector": {}, "updates": [{"key": ""}], "deletes": []}),
        )
        res = _run(module.metadata_batch_update.__wrapped__("ds-1", "tenant-1"))
        assert "Each update requires key and value." in res["message"]

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"selector": {}, "updates": [], "deletes": [{"x": "y"}]}),
        )
        res = _run(module.metadata_batch_update.__wrapped__("ds-1", "tenant-1"))
        assert "Each delete requires key." in res["message"]

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue(
                {
                    "selector": {"document_ids": ["bad"], "metadata_condition": {"conditions": []}},
                    "updates": [{"key": "k", "value": "v"}],
                    "deletes": [],
                }
            ),
        )
        monkeypatch.setattr(module.KnowledgebaseService, "list_documents_by_ids", lambda _ids: ["doc-1"])
        res = _run(module.metadata_batch_update.__wrapped__("ds-1", "tenant-1"))
        assert "do not belong to dataset" in res["message"]

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue(
                {
                    "selector": {"document_ids": ["doc-1"], "metadata_condition": {"conditions": [{"f": "x"}]}},
                    "updates": [{"key": "k", "value": "v"}],
                    "deletes": [],
                }
            ),
        )
        monkeypatch.setattr(module, "meta_filter", lambda *_args, **_kwargs: [])
        monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda _kbs: [])
        res = _run(module.metadata_batch_update.__wrapped__("ds-1", "tenant-1"))
        assert res["code"] == 0
        assert res["data"]["updated"] == 0
        assert res["data"]["matched_docs"] == 0

        monkeypatch.setattr(module, "meta_filter", lambda *_args, **_kwargs: ["doc-1"])
        monkeypatch.setattr(module.DocMetadataService, "batch_update_metadata", lambda *_args, **_kwargs: 1)
        res = _run(module.metadata_batch_update.__wrapped__("ds-1", "tenant-1"))
        assert res["code"] == 0
        assert res["data"]["updated"] == 1
        assert res["data"]["matched_docs"] == 1

    def test_delete_branches(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(module.delete.__wrapped__("tenant-1", "ds-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"ids": ["doc-1"]}))
        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
        monkeypatch.setattr(module.FileService, "get_root_folder", lambda _tenant: {"id": "pf-1"})
        monkeypatch.setattr(module.FileService, "init_knowledgebase_docs", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _id: (True, _DummyDoc()))
        monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _id: None)
        res = _run(module.delete.__wrapped__("tenant-1", "ds-1"))
        assert res["message"] == "Tenant not found!"

        monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _id: "tenant-1")
        monkeypatch.setattr(module.File2DocumentService, "get_storage_address", lambda **_kwargs: ("b", "n"))
        monkeypatch.setattr(module.DocumentService, "remove_document", lambda *_args, **_kwargs: False)
        res = _run(module.delete.__wrapped__("tenant-1", "ds-1"))
        assert "Document removal" in res["message"]

        def _raise_get_by_id(_id):
            raise RuntimeError("boom")

        monkeypatch.setattr(module.DocumentService, "get_by_id", _raise_get_by_id)
        res = _run(module.delete.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == module.RetCode.SERVER_ERROR
        assert "boom" in res["message"]

        monkeypatch.setattr(module, "check_duplicate_ids", lambda _ids, _kind: ([], ["Duplicate document ids: doc-1"]))
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _id: (False, None))
        res = _run(module.delete.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "Duplicate document ids" in res["message"]

    def test_parse_branches(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(module.parse.__wrapped__("tenant-1", "ds-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"document_ids": ["doc-1"]}))
        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
        toggle_doc = _ToggleBoolDocList(_DummyDoc(progress=0))
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: toggle_doc)
        res = _run(module.parse.__wrapped__("tenant-1", "ds-1"))
        assert "don't own the document" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc(run=module.TaskStatus.RUNNING.value)])
        monkeypatch.setattr(
            module.DocumentService,
            "update_by_id",
            lambda *_args, **_kwargs: (_ for _ in ()).throw(AssertionError("update_by_id must not be called for running docs")),
        )
        res = _run(module.parse.__wrapped__("tenant-1", "ds-1"))
        assert "currently being processed" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc(progress=0)])
        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _id: (True, _DummyDoc()))
        monkeypatch.setattr(module.File2DocumentService, "get_storage_address", lambda **_kwargs: ("b", "n"))
        _patch_docstore(monkeypatch, module, delete=lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module.TaskService, "filter_delete", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "queue_tasks", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, ["Duplicate document ids: doc-1"]))
        res = _run(module.parse.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == 0
        assert res["data"]["success_count"] == 1
        assert "Duplicate document ids" in res["data"]["errors"][0]

        monkeypatch.setattr(module, "check_duplicate_ids", lambda _ids, _kind: ([], ["Duplicate document ids: doc-1"]))
        res = _run(module.parse.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "Duplicate document ids" in res["message"]

    def test_stop_parsing_branches(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({}))
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert "`document_ids` is required" in res["message"]

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"document_ids": ["doc-1"]}))
        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert "don't own the document" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc(run=module.TaskStatus.DONE.value)])
        monkeypatch.setattr(
            module,
            "cancel_all_task_of",
            lambda *_args, **_kwargs: (_ for _ in ()).throw(AssertionError("cancel_all_task_of must not be called for non-running docs")),
        )
        monkeypatch.setattr(
            module.DocumentService,
            "update_by_id",
            lambda *_args, **_kwargs: (_ for _ in ()).throw(AssertionError("update_by_id must not be called for non-running docs")),
        )
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == module.RetCode.DATA_ERROR
        assert res["data"]["error_code"] == module.DOC_STOP_PARSING_INVALID_STATE_ERROR_CODE
        assert res["message"] == module.DOC_STOP_PARSING_INVALID_STATE_MESSAGE

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc(run=module.TaskStatus.RUNNING.value)])
        monkeypatch.setattr(module, "cancel_all_task_of", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: True)
        _patch_docstore(monkeypatch, module, delete=lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, ["Duplicate document ids: doc-1"]))
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == 0
        assert res["data"]["success_count"] == 1
        assert "Duplicate document ids" in res["data"]["errors"][0]

        monkeypatch.setattr(module, "check_duplicate_ids", lambda _ids, _kind: ([], ["Duplicate document ids: doc-1"]))
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "Duplicate document ids" in res["message"]

        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc(run=module.TaskStatus.RUNNING.value)])
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == 0

    def test_list_chunks_branches(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(module.list_chunks.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
        res = _run(module.list_chunks.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "don't own the document" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc()])
        monkeypatch.setattr(module, "request", SimpleNamespace(args=_DummyArgs({"id": "chunk-1"})))
        _patch_docstore(monkeypatch, module, get=lambda *_args, **_kwargs: None)
        res = _run(module.list_chunks.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "Chunk not found" in res["message"]

        _patch_docstore(monkeypatch, module, get=lambda *_args, **_kwargs: {"id_vec": [1], "content_with_weight_vec": [2]})
        res = _run(module.list_chunks.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "Chunk `chunk-1` not found." in res["message"]

        _patch_docstore(
            monkeypatch,
            module,
            get=lambda *_args, **_kwargs: {
                "chunk_id": "chunk-1",
                "content_with_weight": "x",
                "doc_id": "doc-1",
                "docnm_kwd": "doc",
                "position_int": [[1, 2, 3, 4, 5]],
            },
        )
        res = _run(module.list_chunks.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["code"] == 0
        assert res["data"]["total"] == 1
        assert res["data"]["chunks"][0]["id"] == "chunk-1"

    def test_add_chunk_access_guard(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(module.add_chunk.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "don't own the dataset" in res["message"]

    def test_rm_chunk_branches(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(module.rm_chunk.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "get_by_ids", lambda _ids: [])
        with pytest.raises(LookupError):
            _run(module.rm_chunk.__wrapped__("tenant-1", "ds-1", "doc-1"))

        monkeypatch.setattr(module.DocumentService, "get_by_ids", lambda _ids: [_DummyDoc()])
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({}))
        _patch_docstore(monkeypatch, module, delete=lambda *_args, **_kwargs: 2)
        monkeypatch.setattr(module.DocumentService, "decrement_chunk_num", lambda *_args, **_kwargs: None)
        res = _run(module.rm_chunk.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["code"] == 0

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"chunk_ids": ["c1", "c1"]}))
        monkeypatch.setattr(module, "check_duplicate_ids", lambda _ids, _kind: (["c1"], ["Duplicate chunk ids: c1"]))
        _patch_docstore(monkeypatch, module, delete=lambda *_args, **_kwargs: 1)
        res = _run(module.rm_chunk.__wrapped__("tenant-1", "ds-1", "doc-1"))
        assert res["code"] == 0
        assert res["data"]["errors"] == ["Duplicate chunk ids: c1"]

    def test_update_chunk_branches(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        _patch_docstore(monkeypatch, module, get=lambda *_args, **_kwargs: None)
        res = _run(module.update_chunk.__wrapped__("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert "Can't find this chunk" in res["message"]

        _patch_docstore(monkeypatch, module, get=lambda *_args, **_kwargs: {"content_with_weight": "q\na"})
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(module.update_chunk.__wrapped__("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
        res = _run(module.update_chunk.__wrapped__("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert "don't own the document" in res["message"]

        doc = _DummyDoc(parser_id="naive")
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [doc])
        monkeypatch.setattr(module.rag_tokenizer, "tokenize", lambda text: text or "")
        monkeypatch.setattr(module.rag_tokenizer, "fine_grained_tokenize", lambda text: text or "")
        monkeypatch.setattr(module.rag_tokenizer, "is_chinese", lambda _text: False)
        monkeypatch.setattr(module.DocumentService, "get_embd_id", lambda _doc_id: "embd")

        class _EmbedModel:
            def encode(self, _texts):
                return [np.array([0.2, 0.8]), np.array([0.3, 0.7])], 1

        monkeypatch.setattr(module.TenantLLMService, "model_instance", lambda *_args, **_kwargs: _EmbedModel())
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"positions": "bad"}))
        res = _run(module.update_chunk.__wrapped__("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert "`positions` should be a list" in res["message"]

        _patch_docstore(monkeypatch, module, get=lambda *_args, **_kwargs: {"content_with_weight": "x"}, update=lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"positions": [[1, 2, 3, 4, 5]]}))
        res = _run(module.update_chunk.__wrapped__("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert res["code"] == 0

        qa_doc = _DummyDoc(parser_id=module.ParserType.QA)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [qa_doc])
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"content": "no-separator"}))
        res = _run(module.update_chunk.__wrapped__("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert "Q&A must be separated" in res["message"]

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"content": "Q?\nA!"}))
        _patch_docstore(monkeypatch, module, get=lambda *_args, **_kwargs: {"content_with_weight": "Q?\nA!"}, update=lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "beAdoc", lambda d, *_args, **_kwargs: d)
        res = _run(module.update_chunk.__wrapped__("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert res["code"] == 0

    def test_retrieval_validation_matrix(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"dataset_ids": "bad"}))
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "`dataset_ids` should be a list" in res["message"]

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"dataset_ids": ["ds-1"]}))
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_ids", lambda _ids: [SimpleNamespace(embd_id="m1"), SimpleNamespace(embd_id="m2")])
        monkeypatch.setattr(module.TenantLLMService, "split_model_name_and_factory", lambda embd_id: (embd_id, "f"))
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "different embedding models" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "get_by_ids", lambda _ids: [SimpleNamespace(embd_id="m1", tenant_id="tenant-1")])
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "`question` is required." in res["message"]

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "   "}),
        )
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert res["code"] == 0
        assert res["data"]["chunks"] == []

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "document_ids": "bad"}),
        )
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "`documents` should be a list" in res["message"]

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "document_ids": ["not-owned"]}),
        )
        monkeypatch.setattr(module.KnowledgebaseService, "list_documents_by_ids", lambda _ids: ["doc-1"])
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "don't own the document" in res["message"]

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "metadata_condition": {"logic": "and"}}),
        )
        monkeypatch.setattr(module.DocMetadataService, "get_meta_by_kbs", lambda _ids: [])
        monkeypatch.setattr(module, "meta_filter", lambda *_args, **_kwargs: [])
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "code" in res

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "highlight": "True"}),
        )
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_ids", lambda _ids: [SimpleNamespace(embd_id="m1", tenant_id="tenant-1")])
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, SimpleNamespace(tenant_id="tenant-1", embd_id="m1")))

        class _Retriever:
            async def retrieval(self, *_args, **_kwargs):
                return {"chunks": [], "total": 0}

            def retrieval_by_children(self, chunks, *_args, **_kwargs):
                return chunks

        monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: SimpleNamespace())
        monkeypatch.setattr(module, "label_question", lambda *_args, **_kwargs: {})
        monkeypatch.setattr(module.settings, "retriever", _Retriever())
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert res["code"] == 0
        assert res["data"]["chunks"] == []

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "highlight": True}),
        )
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert res["code"] == 0

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "highlight": "yes"}),
        )
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "`highlight` should be a boolean" in res["message"]

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "highlight": 1}),
        )
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "`highlight` should be a boolean" in res["message"]

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q"}),
        )
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (False, None))
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "Dataset not found!" in res["message"]

        feature_calls = {"cross": None, "keyword": None, "retrieval_question": None}

        async def _cross_languages(_tenant_id, _dialog, question, langs):
            feature_calls["cross"] = tuple(langs)
            return f"{question}-xl"

        async def _keyword_extraction(_chat_mdl, question):
            feature_calls["keyword"] = question
            return "-kw"

        class _FeatureRetriever:
            async def retrieval(self, question, *_args, **_kwargs):
                feature_calls["retrieval_question"] = question
                return {
                    "chunks": [
                        {
                            "chunk_id": "c1",
                            "content_with_weight": "content",
                            "doc_id": "doc-1",
                            "kb_id": "ds-1",
                            "vector": [1, 2],
                        }
                    ],
                    "total": 1,
                }

            async def retrieval_by_toc(self, question, chunks, tenant_ids, _chat_mdl, size):
                assert question == "q-xl-kw"
                assert chunks and tenant_ids
                assert size == 30
                return [
                    {
                        "chunk_id": "toc-1",
                        "content_with_weight": "toc content",
                        "doc_id": "doc-toc",
                        "kb_id": "ds-1",
                    }
                ]

            def retrieval_by_children(self, chunks, _tenant_ids):
                return chunks + [
                    {
                        "chunk_id": "child-1",
                        "content_with_weight": "child content",
                        "doc_id": "doc-child",
                        "kb_id": "ds-1",
                    }
                ]

        class _FeatureKgRetriever:
            async def retrieval(self, *_args, **_kwargs):
                return {
                    "chunk_id": "kg-1",
                    "content_with_weight": "kg content",
                    "doc_id": "doc-kg",
                    "kb_id": "ds-1",
                }

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue(
                {
                    "dataset_ids": ["ds-1"],
                    "question": "q",
                    "rerank_id": "rerank-1",
                    "cross_languages": ["fr"],
                    "keyword": True,
                    "toc_enhance": True,
                    "use_kg": True,
                }
            ),
        )
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, SimpleNamespace(tenant_id="tenant-1", embd_id="m1")))
        monkeypatch.setattr(module, "cross_languages", _cross_languages)
        monkeypatch.setattr(module, "keyword_extraction", _keyword_extraction)
        monkeypatch.setattr(module.settings, "retriever", _FeatureRetriever())
        monkeypatch.setattr(module.settings, "kg_retriever", _FeatureKgRetriever())
        monkeypatch.setattr(module, "label_question", lambda *_args, **_kwargs: {})
        monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: SimpleNamespace())
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert res["code"] == 0
        assert feature_calls["cross"] == ("fr",)
        assert feature_calls["keyword"] == "q-xl"
        assert feature_calls["retrieval_question"] == "q-xl-kw"
        assert res["data"]["chunks"][0]["id"] == "kg-1"
        assert res["data"]["chunks"][0]["content"] == "kg content"
        assert any(chunk["id"] == "toc-1" for chunk in res["data"]["chunks"])
        assert any(chunk["id"] == "child-1" for chunk in res["data"]["chunks"])

        class _NotFoundRetriever:
            async def retrieval(self, *_args, **_kwargs):
                raise Exception("boom not_found boom")

            def retrieval_by_children(self, chunks, *_args, **_kwargs):
                return chunks

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q"}),
        )
        monkeypatch.setattr(module.settings, "retriever", _NotFoundRetriever())
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "No chunk found! Check the chunk status please!" in res["message"]

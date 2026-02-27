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
import sys
import string
from types import ModuleType, SimpleNamespace
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from common import list_kbs, upload_documents
from configs import DOCUMENT_NAME_LIMIT, INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth
from utils.file_utils import create_txt_file
from api.constants import FILE_NAME_LEN_LIMIT


@pytest.mark.p1
@pytest.mark.usefixtures("clear_datasets")
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = upload_documents(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestDocumentsUpload:
    @pytest.mark.p1
    def test_valid_single_upload(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp])
        assert res["code"] == 0, res
        assert res["data"][0]["kb_id"] == kb_id, res
        assert res["data"][0]["name"] == fp.name, res

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "generate_test_files",
        [
            "docx",
            "excel",
            "ppt",
            "image",
            "pdf",
            "txt",
            "md",
            "json",
            "eml",
            "html",
        ],
        indirect=True,
    )
    def test_file_type_validation(self, WebApiAuth, add_dataset_func, generate_test_files, request):
        kb_id = add_dataset_func
        fp = generate_test_files[request.node.callspec.params["generate_test_files"]]
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp])
        assert res["code"] == 0, res
        assert res["data"][0]["kb_id"] == kb_id, res
        assert res["data"][0]["name"] == fp.name, res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "file_type",
        ["exe", "unknown"],
    )
    def test_unsupported_file_type(self, WebApiAuth, add_dataset_func, tmp_path, file_type):
        kb_id = add_dataset_func
        fp = tmp_path / f"ragflow_test.{file_type}"
        fp.touch()
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp])
        assert res["code"] == 500, res
        assert res["message"] == f"ragflow_test.{file_type}: This type of file has not been supported yet!", res

    @pytest.mark.p2
    def test_missing_file(self, WebApiAuth, add_dataset_func):
        kb_id = add_dataset_func
        res = upload_documents(WebApiAuth, {"kb_id": kb_id})
        assert res["code"] == 101, res
        assert res["message"] == "No file part!", res

    @pytest.mark.p3
    def test_empty_file(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        fp = tmp_path / "empty.txt"
        fp.touch()

        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp])
        assert res["code"] == 0, res
        assert res["data"][0]["size"] == 0, res

    @pytest.mark.p3
    def test_filename_empty(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func

        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp], filename_override="")
        assert res["code"] == 101, res
        assert res["message"] == "No file selected!", res

    @pytest.mark.p3
    def test_filename_exceeds_max_length(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        fp = create_txt_file(tmp_path / f"{'a' * (DOCUMENT_NAME_LIMIT - 4)}.txt")
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp])
        assert res["code"] == 0, res
        assert res["data"][0]["name"] == fp.name, res

    @pytest.mark.p2
    def test_invalid_kb_id(self, WebApiAuth, tmp_path):
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documents(WebApiAuth, {"kb_id": "invalid_kb_id"}, [fp])
        assert res["code"] == 100, res
        assert res["message"] == """LookupError("Can't find this dataset!")""", res

    @pytest.mark.p2
    def test_duplicate_files(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp, fp])
        assert res["code"] == 0, res
        assert len(res["data"]) == 2, res
        for i in range(len(res["data"])):
            assert res["data"][i]["kb_id"] == kb_id, res
            expected_name = fp.name
            if i != 0:
                expected_name = f"{fp.stem}({i}){fp.suffix}"
            assert res["data"][i]["name"] == expected_name, res

    @pytest.mark.p3
    def test_filename_special_characters(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        illegal_chars = '<>:"/\\|?*'
        translation_table = str.maketrans({char: "_" for char in illegal_chars})
        safe_filename = string.punctuation.translate(translation_table)
        fp = tmp_path / f"{safe_filename}.txt"
        fp.write_text("Sample text content")

        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp])
        assert res["code"] == 0, res
        assert len(res["data"]) == 1, res
        assert res["data"][0]["kb_id"] == kb_id, res
        assert res["data"][0]["name"] == fp.name, res

    @pytest.mark.p1
    def test_multiple_files(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        expected_document_count = 20
        fps = []
        for i in range(expected_document_count):
            fp = create_txt_file(tmp_path / f"ragflow_test_{i}.txt")
            fps.append(fp)
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, fps)
        assert res["code"] == 0, res

        res = list_kbs(WebApiAuth)
        assert res["data"]["kbs"][0]["doc_num"] == expected_document_count, res

    @pytest.mark.p3
    def test_concurrent_upload(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func

        count = 20
        fps = []
        for i in range(count):
            fp = create_txt_file(tmp_path / f"ragflow_test_{i}.txt")
            fps.append(fp)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(upload_documents, WebApiAuth, {"kb_id": kb_id}, fps[i : i + 1]) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures), responses

        res = list_kbs(WebApiAuth)
        assert res["data"]["kbs"][0]["doc_num"] == count, res


class _AwaitableValue:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _coro():
            return self._value

        return _coro().__await__()


class _DummyFiles(dict):
    def getlist(self, key):
        value = self.get(key, [])
        if isinstance(value, list):
            return value
        return [value]


class _DummyFile:
    def __init__(self, filename):
        self.filename = filename
        self.closed = False
        self.stream = self

    def close(self):
        self.closed = True


class _DummyRequest:
    def __init__(self, form=None, files=None):
        self._form = form or {}
        self._files = files or _DummyFiles()

    @property
    def form(self):
        return _AwaitableValue(self._form)

    @property
    def files(self):
        return _AwaitableValue(self._files)


def _run(coro):
    return asyncio.run(coro)


@pytest.mark.p2
class TestDocumentsUploadUnit:
    def test_missing_kb_id(self, document_app_module, monkeypatch):
        module = document_app_module
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": ""}, files=_DummyFiles()))
        res = _run(module.upload.__wrapped__())
        assert res["code"] == 101
        assert res["message"] == 'Lack of "KB ID"'

    def test_missing_file_part(self, document_app_module, monkeypatch):
        module = document_app_module
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "kb1"}, files=_DummyFiles()))
        res = _run(module.upload.__wrapped__())
        assert res["code"] == 101
        assert res["message"] == "No file part!"

    def test_empty_filename_closes_files(self, document_app_module, monkeypatch):
        module = document_app_module
        file_obj = _DummyFile("")
        files = _DummyFiles({"file": [file_obj]})
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "kb1"}, files=files))
        res = _run(module.upload.__wrapped__())
        assert res["code"] == 101
        assert res["message"] == "No file selected!"
        assert file_obj.closed is True

    def test_filename_too_long(self, document_app_module, monkeypatch):
        module = document_app_module
        long_name = "a" * (FILE_NAME_LEN_LIMIT + 1)
        file_obj = _DummyFile(long_name)
        files = _DummyFiles({"file": [file_obj]})
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "kb1"}, files=files))
        res = _run(module.upload.__wrapped__())
        assert res["code"] == 101
        assert res["message"] == f"File name must be {FILE_NAME_LEN_LIMIT} bytes or less."

    def test_invalid_kb_id_raises(self, document_app_module, monkeypatch):
        module = document_app_module
        file_obj = _DummyFile("ragflow_test.txt")
        files = _DummyFiles({"file": [file_obj]})
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "missing"}, files=files))
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
        with pytest.raises(LookupError):
            _run(module.upload.__wrapped__())

    def test_no_permission(self, document_app_module, monkeypatch):
        module = document_app_module
        kb = SimpleNamespace(id="kb1", tenant_id="tenant1", name="kb", parser_id="parser", parser_config={})
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, kb))
        monkeypatch.setattr(module, "check_kb_team_permission", lambda *_args, **_kwargs: False)
        file_obj = _DummyFile("ragflow_test.txt")
        files = _DummyFiles({"file": [file_obj]})
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "kb1"}, files=files))
        res = _run(module.upload.__wrapped__())
        assert res["code"] == 109
        assert res["message"] == "No authorization."

    def test_thread_pool_errors(self, document_app_module, monkeypatch):
        module = document_app_module
        kb = SimpleNamespace(id="kb1", tenant_id="tenant1", name="kb", parser_id="parser", parser_config={})
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, kb))
        monkeypatch.setattr(module, "check_kb_team_permission", lambda *_args, **_kwargs: True)

        async def fake_thread_pool_exec(*_args, **_kwargs):
            return (["unsupported type"], [("file1", "blob")])

        monkeypatch.setattr(module, "thread_pool_exec", fake_thread_pool_exec)
        file_obj = _DummyFile("ragflow_test.txt")
        files = _DummyFiles({"file": [file_obj]})
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "kb1"}, files=files))
        res = _run(module.upload.__wrapped__())
        assert res["code"] == 500
        assert "unsupported type" in res["message"]
        assert res["data"] == ["file1"]

    def test_empty_upload_result(self, document_app_module, monkeypatch):
        module = document_app_module
        kb = SimpleNamespace(id="kb1", tenant_id="tenant1", name="kb", parser_id="parser", parser_config={})
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, kb))
        monkeypatch.setattr(module, "check_kb_team_permission", lambda *_args, **_kwargs: True)

        async def fake_thread_pool_exec(*_args, **_kwargs):
            return (None, [])

        monkeypatch.setattr(module, "thread_pool_exec", fake_thread_pool_exec)
        file_obj = _DummyFile("ragflow_test.txt")
        files = _DummyFiles({"file": [file_obj]})
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "kb1"}, files=files))
        res = _run(module.upload.__wrapped__())
        assert res["code"] == 102
        assert "file format" in res["message"]

    def test_upload_and_parse_matrix_unit(self, document_app_module, monkeypatch):
        module = document_app_module
        monkeypatch.setattr(module, "request", _DummyRequest(form={"conversation_id": "conv-1"}, files=_DummyFiles({"file": [_DummyFile("")]})))
        res = _run(module.upload_and_parse.__wrapped__())
        assert res["code"] == module.RetCode.ARGUMENT_ERROR
        assert res["message"] == "No file selected!"

        files = _DummyFiles({"file": [_DummyFile("note.txt")]})
        monkeypatch.setattr(module, "request", _DummyRequest(form={"conversation_id": "conv-1"}, files=files))
        monkeypatch.setattr(module, "doc_upload_and_parse", lambda _conv_id, _files, _uid: ["doc-1"])
        res = _run(module.upload_and_parse.__wrapped__())
        assert res["code"] == 0
        assert res["data"] == ["doc-1"]

    def test_parse_url_and_multipart_matrix_unit(self, document_app_module, monkeypatch, tmp_path):
        module = document_app_module

        async def req_invalid_url():
            return {"url": "not-a-url"}

        monkeypatch.setattr(module, "get_request_json", req_invalid_url)
        monkeypatch.setattr(module, "is_valid_url", lambda _url: False)
        res = _run(module.parse())
        assert res["code"] == module.RetCode.ARGUMENT_ERROR
        assert res["message"] == "The URL format is invalid"

        webdriver_mod = ModuleType("seleniumwire.webdriver")

        class _FakeChromeOptions:
            def __init__(self):
                self.args = []
                self.experimental = {}

            def add_argument(self, arg):
                self.args.append(arg)

            def add_experimental_option(self, key, value):
                self.experimental[key] = value

        class _Req:
            def __init__(self, headers):
                self.response = SimpleNamespace(headers=headers)

        class _FakeDriver:
            def __init__(self, requests, page_source):
                self.requests = requests
                self.page_source = page_source
                self.quit_called = False
                self.visited = []
                self.options = None

            def get(self, url):
                self.visited.append(url)

            def quit(self):
                self.quit_called = True

        queue = []
        created = []

        def _fake_chrome(options=None):
            driver = queue.pop(0)
            driver.options = options
            created.append(driver)
            return driver

        webdriver_mod.Chrome = _fake_chrome
        webdriver_mod.ChromeOptions = _FakeChromeOptions

        seleniumwire_mod = ModuleType("seleniumwire")
        seleniumwire_mod.webdriver = webdriver_mod
        monkeypatch.setitem(sys.modules, "seleniumwire", seleniumwire_mod)
        monkeypatch.setitem(sys.modules, "seleniumwire.webdriver", webdriver_mod)
        monkeypatch.setattr(module, "get_project_base_directory", lambda: str(tmp_path))
        monkeypatch.setattr(module, "is_valid_url", lambda _url: True)

        class _Parser:
            def parser_txt(self, page_source):
                assert "page" in page_source
                return ["section1", "section2"]

        monkeypatch.setattr(module, "RAGFlowHtmlParser", lambda: _Parser())
        queue.append(_FakeDriver([_Req({"x": "1"}), _Req({"y": "2"})], "<html>page</html>"))

        async def req_url_html():
            return {"url": "http://example.com/html"}

        monkeypatch.setattr(module, "get_request_json", req_url_html)
        res = _run(module.parse())
        assert res["code"] == 0
        assert res["data"] == "section1\nsection2"
        assert created[-1].quit_called is True

        (tmp_path / "logs" / "downloads").mkdir(parents=True, exist_ok=True)
        (tmp_path / "logs" / "downloads" / "doc.txt").write_bytes(b"downloaded-bytes")
        queue.append(_FakeDriver([_Req({"content-disposition": 'attachment; filename="doc.txt"'})], "<html>file</html>"))
        captured = {}

        def parse_docs_read(files, _uid):
            captured["filename"] = files[0].filename
            captured["content"] = files[0].read()
            return "parsed-download"

        monkeypatch.setattr(module.FileService, "parse_docs", parse_docs_read)

        async def req_url_file():
            return {"url": "http://example.com/file"}

        monkeypatch.setattr(module, "get_request_json", req_url_file)
        res = _run(module.parse())
        assert res["code"] == 0
        assert res["data"] == "parsed-download"
        assert captured["filename"] == "doc.txt"
        assert captured["content"] == b"downloaded-bytes"

        async def req_no_url():
            return {}

        monkeypatch.setattr(module, "get_request_json", req_no_url)
        monkeypatch.setattr(module, "request", _DummyRequest(files=_DummyFiles()))
        res = _run(module.parse())
        assert res["code"] == module.RetCode.ARGUMENT_ERROR
        assert res["message"] == "No file part!"

        monkeypatch.setattr(module, "request", _DummyRequest(files=_DummyFiles({"file": [_DummyFile("f1.txt")]})))
        monkeypatch.setattr(module.FileService, "parse_docs", lambda _files, _uid: "parsed-upload")
        res = _run(module.parse())
        assert res["code"] == 0
        assert res["data"] == "parsed-upload"


@pytest.mark.p2
class TestWebCrawlUnit:
    def test_missing_kb_id(self, document_app_module, monkeypatch):
        module = document_app_module
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "", "name": "doc", "url": "http://example.com"}))
        res = _run(module.web_crawl.__wrapped__())
        assert res["code"] == 101
        assert res["message"] == 'Lack of "KB ID"'

    def test_invalid_url(self, document_app_module, monkeypatch):
        module = document_app_module
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "kb1", "name": "doc", "url": "not-a-url"}))
        res = _run(module.web_crawl.__wrapped__())
        assert res["code"] == 101
        assert res["message"] == "The URL format is invalid"

    def test_invalid_kb_id_raises(self, document_app_module, monkeypatch):
        module = document_app_module
        monkeypatch.setattr(module, "is_valid_url", lambda _url: True)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "missing", "name": "doc", "url": "http://example.com"}))
        with pytest.raises(LookupError):
            _run(module.web_crawl.__wrapped__())

    def test_no_permission(self, document_app_module, monkeypatch):
        module = document_app_module
        kb = SimpleNamespace(id="kb1", tenant_id="tenant1", name="kb", parser_id="parser", parser_config={})
        monkeypatch.setattr(module, "is_valid_url", lambda _url: True)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, kb))
        monkeypatch.setattr(module, "check_kb_team_permission", lambda *_args, **_kwargs: False)
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "kb1", "name": "doc", "url": "http://example.com"}))
        res = _run(module.web_crawl.__wrapped__())
        assert res["code"] == 109
        assert res["message"] == "No authorization."

    def test_download_failure(self, document_app_module, monkeypatch):
        module = document_app_module
        kb = SimpleNamespace(id="kb1", tenant_id="tenant1", name="kb", parser_id="parser", parser_config={})
        monkeypatch.setattr(module, "is_valid_url", lambda _url: True)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, kb))
        monkeypatch.setattr(module, "check_kb_team_permission", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module, "html2pdf", lambda _url: None)
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "kb1", "name": "doc", "url": "http://example.com"}))
        res = _run(module.web_crawl.__wrapped__())
        assert res["code"] == 100
        assert "Download failure" in res["message"]

    def test_unsupported_type(self, document_app_module, monkeypatch):
        module = document_app_module
        kb = SimpleNamespace(id="kb1", tenant_id="tenant1", name="kb", parser_id="parser", parser_config={})
        monkeypatch.setattr(module, "is_valid_url", lambda _url: True)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, kb))
        monkeypatch.setattr(module, "check_kb_team_permission", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module, "html2pdf", lambda _url: b"%PDF-1.4")
        monkeypatch.setattr(module.FileService, "get_root_folder", lambda _uid: {"id": "root"})
        monkeypatch.setattr(module.FileService, "init_knowledgebase_docs", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module.FileService, "get_kb_folder", lambda *_args, **_kwargs: {"id": "kb_root"})
        monkeypatch.setattr(module.FileService, "new_a_file_from_kb", lambda *_args, **_kwargs: {"id": "kb_folder"})
        monkeypatch.setattr(module, "duplicate_name", lambda *_args, **_kwargs: "bad.exe")
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "kb1", "name": "doc", "url": "http://example.com"}))
        res = _run(module.web_crawl.__wrapped__())
        assert res["code"] == 100
        assert "supported yet" in res["message"]

    @pytest.mark.parametrize(
        "filename,filetype,expected_parser",
        [
            ("image.png", "visual", "picture"),
            ("sound.mp3", "aural", "audio"),
            ("deck.pptx", "doc", "presentation"),
            ("mail.eml", "doc", "email"),
        ],
    )
    def test_success_parser_overrides(self, document_app_module, monkeypatch, filename, filetype, expected_parser):
        module = document_app_module
        kb = SimpleNamespace(id="kb1", tenant_id="tenant1", name="kb", parser_id="parser", parser_config={})
        captured = {}

        class _Storage:
            def obj_exist(self, *_args, **_kwargs):
                return False

            def put(self, *_args, **_kwargs):
                captured["put"] = True

        def insert_doc(doc):
            captured["doc"] = doc

        monkeypatch.setattr(module, "is_valid_url", lambda _url: True)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, kb))
        monkeypatch.setattr(module, "check_kb_team_permission", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module, "html2pdf", lambda _url: b"%PDF-1.4")
        monkeypatch.setattr(module.FileService, "get_root_folder", lambda _uid: {"id": "root"})
        monkeypatch.setattr(module.FileService, "init_knowledgebase_docs", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module.FileService, "get_kb_folder", lambda *_args, **_kwargs: {"id": "kb_root"})
        monkeypatch.setattr(module.FileService, "new_a_file_from_kb", lambda *_args, **_kwargs: {"id": "kb_folder"})
        monkeypatch.setattr(module, "duplicate_name", lambda *_args, **_kwargs: filename)
        monkeypatch.setattr(module, "filename_type", lambda _name: filetype)
        monkeypatch.setattr(module, "thumbnail", lambda *_args, **_kwargs: "")
        monkeypatch.setattr(module, "get_uuid", lambda: "doc-1")
        monkeypatch.setattr(module.settings, "STORAGE_IMPL", _Storage())
        monkeypatch.setattr(module.DocumentService, "insert", insert_doc)
        monkeypatch.setattr(module.FileService, "add_file_from_kb", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "kb1", "name": "doc", "url": "http://example.com"}))

        res = _run(module.web_crawl.__wrapped__())
        assert res["code"] == 0
        assert captured["doc"]["parser_id"] == expected_parser
        assert captured["put"] is True

    def test_exception_path(self, document_app_module, monkeypatch):
        module = document_app_module
        kb = SimpleNamespace(id="kb1", tenant_id="tenant1", name="kb", parser_id="parser", parser_config={})

        class _Storage:
            def obj_exist(self, *_args, **_kwargs):
                return False

            def put(self, *_args, **_kwargs):
                return None

        def insert_doc(_doc):
            raise RuntimeError("boom")

        monkeypatch.setattr(module, "is_valid_url", lambda _url: True)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, kb))
        monkeypatch.setattr(module, "check_kb_team_permission", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module, "html2pdf", lambda _url: b"%PDF-1.4")
        monkeypatch.setattr(module.FileService, "get_root_folder", lambda _uid: {"id": "root"})
        monkeypatch.setattr(module.FileService, "init_knowledgebase_docs", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module.FileService, "get_kb_folder", lambda *_args, **_kwargs: {"id": "kb_root"})
        monkeypatch.setattr(module.FileService, "new_a_file_from_kb", lambda *_args, **_kwargs: {"id": "kb_folder"})
        monkeypatch.setattr(module, "duplicate_name", lambda *_args, **_kwargs: "doc.pdf")
        monkeypatch.setattr(module, "filename_type", lambda _name: "pdf")
        monkeypatch.setattr(module, "thumbnail", lambda *_args, **_kwargs: "")
        monkeypatch.setattr(module, "get_uuid", lambda: "doc-1")
        monkeypatch.setattr(module.settings, "STORAGE_IMPL", _Storage())
        monkeypatch.setattr(module.DocumentService, "insert", insert_doc)
        monkeypatch.setattr(module.FileService, "add_file_from_kb", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "request", _DummyRequest(form={"kb_id": "kb1", "name": "doc", "url": "http://example.com"}))

        res = _run(module.web_crawl.__wrapped__())
        assert res["code"] == 100

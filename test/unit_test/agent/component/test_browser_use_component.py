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

import sys
import types
from pathlib import Path
from types import SimpleNamespace

from api.db import FileType


def _install_cv2_stub_if_unavailable():
    try:
        import cv2  # noqa: F401
        return
    except Exception:
        pass
    stub = types.ModuleType("cv2")
    stub.INTER_LINEAR = 1
    stub.INTER_CUBIC = 2
    stub.BORDER_CONSTANT = 0
    stub.BORDER_REPLICATE = 1

    def _module_getattr(name):
        if name.isupper():
            return 0
        raise RuntimeError("cv2 runtime call is unavailable in this test environment")

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub


_install_cv2_stub_if_unavailable()

from agent.component import browser as browser_use_module  # noqa: E402


class _FakeCanvas:
    def __init__(self, refs=None):
        self._refs = refs or {}

    def is_reff(self, token):
        key = token.strip("{} ")
        return key in self._refs or token in self._refs

    def get_variable_value(self, token):
        key = token.strip("{} ")
        if key in self._refs:
            return self._refs[key]
        return self._refs[token]

    def get_tenant_id(self):
        return "tenant-1"


def _build_component():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas()
    component._param = SimpleNamespace(upload_sources="")
    return component


def test_prepare_input_values_records_variable_inputs():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas(refs={"sys.query": "open example.com"})
    component._param = browser_use_module.BrowserParam()
    component._param.prompts = "{sys.query}"
    component._param.inputs = {}

    component._prepare_input_values()

    assert component.get_input_value("sys.query") == "open example.com"
    assert component.get_input_values()["sys.query"] == "open example.com"


def test_extract_ids_supports_mixed_literals_and_variables():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas(
        refs={
            "{begin@file_ids}": ["f2", "f3,f4"],
            "{begin@extra_file}": "f5",
        }
    )

    file_ids = component._extract_ids("f1,{begin@file_ids},{begin@extra_file},f1")

    assert file_ids == ["f1", "f2", "f3", "f4", "f5"]


def test_extract_ids_supports_json_array_and_csv_format():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas()

    json_ids = component._extract_ids('["1","2"]')
    csv_ids = component._extract_ids("1,2")

    assert json_ids == ["1", "2"]
    assert csv_ids == ["1", "2"]


def test_extract_ids_supports_variable_values_from_input_or_globals():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas(
        refs={
            "{begin@upload_ids}": '["10","20"]',
            "{sys@upload_id}": 30,
            "{begin@file_obj}": {"id": "40", "name": "demo.pdf"},
        }
    )

    file_ids = component._extract_ids("{begin@upload_ids},{sys@upload_id},{begin@file_obj}")

    assert file_ids == ["10", "20", "30", "40"]


def test_extract_ids_supports_url_key_in_variable_object():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas(
        refs={
            "{begin@upload_url_obj}": {"url": "https://example.com/demo.pdf"},
        }
    )

    refs = component._extract_ids("{begin@upload_url_obj}")

    assert refs == ["https://example.com/demo.pdf"]


def test_extract_ids_does_not_split_http_url_by_comma():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas()

    refs = component._extract_ids("https://example.com/download?name=a,b.txt")

    assert refs == ["https://example.com/download?name=a,b.txt"]


def test_prepare_upload_files_supports_http_url(monkeypatch, tmp_path):
    component = _build_component()
    component._param.upload_sources = "https://example.com/files/demo.txt"

    class _FakeResponse:
        def __init__(self):
            self.headers = {"Content-Disposition": 'attachment; filename="remote_demo.txt"'}
            self._data = b"hello from url"
            self._pos = 0

        def read(self, size=-1):
            if size <= 0:
                chunk = self._data[self._pos :]
                self._pos = len(self._data)
                return chunk
            chunk = self._data[self._pos : self._pos + size]
            self._pos += len(chunk)
            return chunk

        def __enter__(self):
            return self

        def __exit__(self, exc_type, exc_val, exc_tb):
            return False

    monkeypatch.setattr(browser_use_module, "urlopen", lambda *_args, **_kwargs: _FakeResponse())

    prepared = component._prepare_upload_files(str(tmp_path))

    assert len(prepared) == 1
    assert prepared[0]["file_id"] == ""
    assert prepared[0]["name"] == "remote_demo.txt"
    assert prepared[0]["source_url"] == "https://example.com/files/demo.txt"
    assert Path(prepared[0]["local_path"]).exists()
    assert Path(prepared[0]["local_path"]).read_bytes() == b"hello from url"


def test_save_downloads_persists_file_records(monkeypatch, tmp_path):
    component = _build_component()
    component._canvas = _FakeCanvas()

    download_file = tmp_path / "report.txt"
    download_file.write_text("ok", encoding="utf-8")

    monkeypatch.setattr(
        browser_use_module.FileService,
        "get_by_id",
        lambda _folder_id: (True, SimpleNamespace(type=FileType.FOLDER.value)),
    )
    monkeypatch.setattr(browser_use_module, "duplicate_name", lambda *_args, **_kwargs: "report.txt")

    stored = {}

    def _put(parent_id, location, blob):
        stored["parent_id"] = parent_id
        stored["location"] = location
        stored["blob"] = blob

    monkeypatch.setattr(browser_use_module.settings, "STORAGE_IMPL", SimpleNamespace(put=_put))
    monkeypatch.setattr(
        browser_use_module.FileService,
        "insert",
        lambda data: SimpleNamespace(
            id="file-1",
            name=data["name"],
            size=data["size"],
            parent_id=data["parent_id"],
        ),
    )

    result = component._save_downloads(str(tmp_path), "dir-1")

    assert len(result) == 1
    assert result[0]["file_id"] == "file-1"
    assert result[0]["parent_id"] == "dir-1"
    assert stored["parent_id"] == "dir-1"
    assert stored["location"] == "report.txt"
    assert stored["blob"] == b"ok"
    assert Path(download_file).exists()

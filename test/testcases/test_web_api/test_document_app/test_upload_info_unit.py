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
from pathlib import Path
import importlib.util
import sys
from types import ModuleType

import pytest


class _AwaitableValue:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _co():
            return self._value

        return _co().__await__()


class _DummyFiles(dict):
    def getlist(self, key):
        value = self.get(key, [])
        if isinstance(value, list):
            return value
        return [value]


class _DummyFile:
    def __init__(self, filename):
        self.filename = filename


class _DummyRequest:
    def __init__(self, *, files=None, args=None):
        self._files = files or _DummyFiles()
        self.args = args or {}

    @property
    def files(self):
        return _AwaitableValue(self._files)


def _run(coro):
    return asyncio.run(coro)


def _load_document_app_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]
    common_mod = ModuleType("common")
    common_mod.bulk_upload_documents = lambda *_args, **_kwargs: []
    common_mod.delete_document = lambda *_args, **_kwargs: None
    common_mod.list_documents = lambda *_args, **_kwargs: {"data": {"docs": []}}
    monkeypatch.setitem(sys.modules, "common", common_mod)
    module_path = repo_root / "test" / "testcases" / "test_web_api" / "test_document_app" / "conftest.py"
    spec = importlib.util.spec_from_file_location("test_document_app_unit_conftest", module_path)
    module = importlib.util.module_from_spec(spec)
    sys.modules["test_document_app_unit_conftest"] = module
    spec.loader.exec_module(module)
    return module.document_app_module.__wrapped__(monkeypatch)


@pytest.mark.p2
def test_upload_info_rejects_mixed_inputs(monkeypatch):
    module = _load_document_app_module(monkeypatch)
    monkeypatch.setattr(module, "assert_url_is_safe", lambda url: ("example.com", "93.184.216.34"))
    files = _DummyFiles({"file": [_DummyFile("a.txt")]})
    monkeypatch.setattr(module, "request", _DummyRequest(files=files, args={"url": "https://example.com/a.txt"}))

    res = _run(module.upload_info())
    assert res["code"] == module.RetCode.BAD_REQUEST
    assert "not both" in res["message"]


@pytest.mark.p2
def test_upload_info_requires_file_or_url(monkeypatch):
    module = _load_document_app_module(monkeypatch)
    monkeypatch.setattr(module, "request", _DummyRequest(files=_DummyFiles()))

    res = _run(module.upload_info())
    assert res["code"] == module.RetCode.BAD_REQUEST
    assert "Missing input" in res["message"]


@pytest.mark.p2
def test_upload_info_supports_url_single_and_multiple_files(monkeypatch):
    module = _load_document_app_module(monkeypatch)
    monkeypatch.setattr(module, "assert_url_is_safe", lambda url: ("example.com", "93.184.216.34"))
    captured = []

    def fake_upload_info(user_id, file_obj, url=None):
        captured.append((user_id, getattr(file_obj, "filename", None), url))
        if url is not None:
            return {"kind": "url", "value": url}
        return {"kind": "file", "value": file_obj.filename}

    monkeypatch.setattr(module.FileService, "upload_info", fake_upload_info)

    monkeypatch.setattr(module, "request", _DummyRequest(files=_DummyFiles(), args={"url": "https://example.com/a.txt"}))
    res = _run(module.upload_info())
    assert res["code"] == 0
    assert res["data"] == {"kind": "url", "value": "https://example.com/a.txt"}

    monkeypatch.setattr(module, "request", _DummyRequest(files=_DummyFiles({"file": _DummyFile("single.txt")})))
    res = _run(module.upload_info())
    assert res["code"] == 0
    assert res["data"] == {"kind": "file", "value": "single.txt"}

    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(files=_DummyFiles({"file": [_DummyFile("a.txt"), _DummyFile("b.txt")]})),
    )
    res = _run(module.upload_info())
    assert res["code"] == 0
    assert res["data"] == [
        {"kind": "file", "value": "a.txt"},
        {"kind": "file", "value": "b.txt"},
    ]
    assert captured == [
        ("user-1", None, "https://example.com/a.txt"),
        ("user-1", "single.txt", None),
        ("user-1", "a.txt", None),
        ("user-1", "b.txt", None),
    ]

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
import importlib.util
import logging
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from enum import IntFlag, auto
from pathlib import Path
from types import ModuleType

import pytest


@dataclass
class _Document:
    id: str
    blob: bytes
    source: str
    semantic_identifier: str
    extension: str
    doc_updated_at: datetime
    size_bytes: int


@dataclass
class _SlimDocument:
    id: str


class _DocumentSource:
    WEBDAV = "webdav"


class _OnyxExtensionType(IntFlag):
    Plain = auto()
    Document = auto()
    Multimedia = auto()


class _LoadConnector:
    pass


class _PollConnector:
    pass


class _SlimConnectorWithPermSync:
    pass


def _install_dependency_stubs():
    config_module = ModuleType("common.data_source.config")
    config_module.DocumentSource = _DocumentSource
    config_module.INDEX_BATCH_SIZE = 10
    config_module.BLOB_STORAGE_SIZE_THRESHOLD = 20

    exceptions_module = ModuleType("common.data_source.exceptions")
    for name in (
        "ConnectorMissingCredentialError",
        "ConnectorValidationError",
        "CredentialExpiredError",
        "InsufficientPermissionsError",
    ):
        setattr(exceptions_module, name, type(name, (Exception,), {}))

    interfaces_module = ModuleType("common.data_source.interfaces")
    interfaces_module.LoadConnector = _LoadConnector
    interfaces_module.PollConnector = _PollConnector
    interfaces_module.SlimConnectorWithPermSync = _SlimConnectorWithPermSync
    interfaces_module.OnyxExtensionType = _OnyxExtensionType
    interfaces_module.GenerateDocumentsOutput = object
    interfaces_module.GenerateSlimDocumentOutput = object
    interfaces_module.SecondsSinceUnixEpoch = int

    models_module = ModuleType("common.data_source.models")
    models_module.Document = _Document
    models_module.SlimDocument = _SlimDocument
    models_module.GenerateDocumentsOutput = object
    models_module.GenerateSlimDocumentOutput = object
    models_module.SecondsSinceUnixEpoch = int

    utils_module = ModuleType("common.data_source.utils")
    utils_module.get_file_ext = lambda file_name: Path(file_name).suffix.lstrip(".").lower()
    utils_module.is_accepted_file_ext = lambda file_ext, extension_type: bool(file_ext)

    webdav4_module = ModuleType("webdav4")
    webdav4_client_module = ModuleType("webdav4.client")
    webdav4_client_module.Client = object

    sys.modules["common.data_source.config"] = config_module
    sys.modules["common.data_source.exceptions"] = exceptions_module
    sys.modules["common.data_source.interfaces"] = interfaces_module
    sys.modules["common.data_source.models"] = models_module
    sys.modules["common.data_source.utils"] = utils_module
    sys.modules["webdav4"] = webdav4_module
    sys.modules["webdav4.client"] = webdav4_client_module


def _load_webdav_connector_module():
    """Load webdav_connector.py in isolation (avoid the package __init__)."""
    repo_root = Path(__file__).resolve().parents[3]
    package_name = "common.data_source"
    saved_modules = {name: module for name, module in sys.modules.items() if name == package_name or name.startswith(f"{package_name}.")}
    saved_webdav_modules = {name: sys.modules[name] for name in ("webdav4", "webdav4.client") if name in sys.modules}
    package_stub = ModuleType(package_name)
    package_stub.__path__ = [str(repo_root / "common" / "data_source")]
    sys.modules[package_name] = package_stub
    _install_dependency_stubs()

    try:
        spec = importlib.util.spec_from_file_location(
            "_webdav_connector_under_test",
            repo_root / "common" / "data_source" / "webdav_connector.py",
        )
        module = importlib.util.module_from_spec(spec)
        assert spec.loader is not None
        spec.loader.exec_module(module)
        return module
    finally:
        for name in list(sys.modules):
            if name == package_name or name.startswith(f"{package_name}."):
                if name in saved_modules:
                    sys.modules[name] = saved_modules[name]
                else:
                    sys.modules.pop(name, None)
        for name in ("webdav4", "webdav4.client"):
            if name in saved_webdav_modules:
                sys.modules[name] = saved_webdav_modules[name]
            else:
                sys.modules.pop(name, None)


webdav_connector = _load_webdav_connector_module()
WebDAVConnector = webdav_connector.WebDAVConnector


class _FakeClient:
    def __init__(self):
        self.downloaded_paths = []

    def download_fileobj(self, path, buffer):
        self.downloaded_paths.append(path)
        buffer.write(b"hello webdav")


def _stub_files(files):
    def _list_files_recursive(*args, **kwargs):
        return files

    return _list_files_recursive


@pytest.mark.p2
def test_get_size_bytes_accepts_integer_strings():
    assert WebDAVConnector._get_size_bytes({"size": 128}) == 128
    assert WebDAVConnector._get_size_bytes({"size": "128"}) == 128
    assert WebDAVConnector._get_size_bytes({"size": " 128 "}) == 128
    assert WebDAVConnector._get_size_bytes({"size": 0}) == 0


@pytest.mark.p2
def test_get_size_bytes_ignores_unknown_values():
    assert WebDAVConnector._get_size_bytes({}) is None
    assert WebDAVConnector._get_size_bytes({"size": ""}) is None
    assert WebDAVConnector._get_size_bytes({"size": "unknown"}) is None
    assert WebDAVConnector._get_size_bytes({"size": "-1"}) is None
    assert WebDAVConnector._get_size_bytes({"size": -1}) is None
    assert WebDAVConnector._get_size_bytes({"size": True}) is None
    assert WebDAVConnector._get_size_bytes({"size": "1" * 21}) is None


@pytest.mark.p2
def test_get_size_bytes_reads_webdav4_content_length():
    # webdav4's Client.ls(detail=True) exposes size as "content_length", not "size".
    assert WebDAVConnector._get_size_bytes({"content_length": 128}) == 128
    assert WebDAVConnector._get_size_bytes({"content_length": "128"}) == 128
    assert WebDAVConnector._get_size_bytes({"content_length": 0}) == 0
    assert WebDAVConnector._get_size_bytes({"getcontentlength": "128"}) == 128


@pytest.mark.p2
def test_get_size_bytes_falls_back_across_keys():
    # When the preferred key is absent/invalid, fall back to the next valid key.
    assert WebDAVConnector._get_size_bytes({"content_length": "256"}) == 256
    assert WebDAVConnector._get_size_bytes({"size": None, "content_length": 256}) == 256
    assert WebDAVConnector._get_size_bytes({"size": "bad", "content_length": 256}) == 256
    assert WebDAVConnector._get_size_bytes({"content_length": None, "getcontentlength": "256"}) == 256


@pytest.mark.p1
def test_yield_webdav_documents_skips_numeric_string_sizes_over_threshold(caplog):
    connector = WebDAVConnector("https://webdav.example", batch_size=10)
    connector.client = _FakeClient()
    connector.size_threshold = 20
    modified = datetime(2026, 1, 1, tzinfo=timezone.utc)
    caplog.set_level(logging.WARNING)
    connector._list_files_recursive = _stub_files(
        [
            ("/large.txt", {"size": "21", "modified": modified}),
            ("/small.txt", {"size": "20", "modified": modified}),
        ]
    )

    batches = list(
        connector._yield_webdav_documents(
            datetime(2025, 1, 1, tzinfo=timezone.utc),
            datetime(2026, 2, 1, tzinfo=timezone.utc),
        )
    )

    assert [doc.id for batch in batches for doc in batch] == [
        "webdav:https://webdav.example:/small.txt",
    ]
    assert connector.client.downloaded_paths == ["/small.txt"]
    assert batches[0][0].size_bytes == 20
    assert "large.txt exceeds size threshold of 20 (size_bytes=21). Skipping." in caplog.text


@pytest.mark.p1
def test_yield_webdav_documents_skips_missing_size_metadata(caplog):
    connector = WebDAVConnector("https://webdav.example", batch_size=10)
    connector.client = _FakeClient()
    connector.size_threshold = 20
    modified = datetime(2026, 1, 1, tzinfo=timezone.utc)
    caplog.set_level(logging.WARNING)
    connector._list_files_recursive = _stub_files(
        [
            ("/missing.txt", {"modified": modified}),
            ("/small.txt", {"size": "20", "modified": modified}),
        ]
    )

    batches = list(
        connector._yield_webdav_documents(
            datetime(2025, 1, 1, tzinfo=timezone.utc),
            datetime(2026, 2, 1, tzinfo=timezone.utc),
        )
    )

    assert [doc.id for batch in batches for doc in batch] == [
        "webdav:https://webdav.example:/small.txt",
    ]
    assert connector.client.downloaded_paths == ["/small.txt"]
    assert ("missing.txt: size metadata missing from WebDAV server response, skipping to avoid processing potentially large files.") in caplog.text


@pytest.mark.p1
def test_retrieve_all_slim_docs_skips_numeric_string_sizes_over_threshold(caplog):
    connector = WebDAVConnector("https://webdav.example", batch_size=10)
    connector.client = object()
    connector.size_threshold = 20
    caplog.set_level(logging.WARNING)
    connector._list_files_recursive = _stub_files(
        [
            ("/large.txt", {"size": "21"}),
            ("/small.txt", {"size": "20"}),
        ]
    )

    batches = list(connector.retrieve_all_slim_docs_perm_sync())

    assert [doc.id for batch in batches for doc in batch] == [
        "webdav:https://webdav.example:/small.txt",
    ]
    assert "large.txt exceeds size threshold of 20 (size_bytes=21). Skipping." in caplog.text


@pytest.mark.p1
def test_retrieve_all_slim_docs_skips_missing_size_metadata(caplog):
    connector = WebDAVConnector("https://webdav.example", batch_size=10)
    connector.client = object()
    connector.size_threshold = 20
    caplog.set_level(logging.WARNING)
    connector._list_files_recursive = _stub_files(
        [
            ("/missing.txt", {}),
            ("/small.txt", {"size": "20"}),
        ]
    )

    batches = list(connector.retrieve_all_slim_docs_perm_sync())

    assert [doc.id for batch in batches for doc in batch] == [
        "webdav:https://webdav.example:/small.txt",
    ]
    assert ("missing.txt: size metadata missing from WebDAV server response, skipping to avoid processing potentially large files.") in caplog.text

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

from contextlib import contextmanager
from datetime import datetime, timezone
from unittest.mock import patch

import pytest
import requests

from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
)
from common.data_source.yandex_disk_connector import YandexDiskConnector

API_BASE = "https://cloud-api.yandex.net/v1/disk"
_DEFAULT_PAGE_SIZE = 100


class FakeResponse:
    def __init__(self, payload=None, content=b"", status_code=200):
        self._payload = payload
        self.content = content
        self.status_code = status_code

    def json(self):
        return self._payload

    def raise_for_status(self):
        if self.status_code >= 400:
            raise requests.HTTPError()


def _make_file_item(name, path, modified="2024-01-01T00:00:00Z", size=100, md5="abc"):
    return {
        "type": "file",
        "name": name,
        "path": path,
        "modified": modified,
        "size": size,
        "md5": md5,
    }


def _make_dir_item(name, path):
    return {"type": "dir", "name": name, "path": path}


def _make_dir_page(items):
    return {"_embedded": {"items": items}}


def _patch_requests(router):
    """Route all connector HTTP through `router(url, **kwargs)`."""

    def fake_request(method, url, **kwargs):
        return router(url, **kwargs)

    return patch.object(requests.Session, "request", side_effect=fake_request)


@contextmanager
def _connector(router, **kwargs):
    connector = YandexDiskConnector(**kwargs)
    with _patch_requests(router):
        connector.load_credentials({"oauth_token": "test-token"})
        yield connector


def test_missing_token_raises():
    connector = YandexDiskConnector()
    with pytest.raises(ConnectorMissingCredentialError):
        connector.load_credentials({})


def test_load_from_state_yields_documents():
    def router(url, **kwargs):
        if url == f"{API_BASE}/resources":
            params = kwargs.get("params", {})
            if params["path"] == "/":
                return FakeResponse(
                    _make_dir_page(
                        [
                            _make_file_item("a.pdf", "disk:/docs/a.pdf"),
                            _make_dir_item("sub", "disk:/sub"),
                        ]
                    )
                )
            if params["path"] == "disk:/sub":
                return FakeResponse(_make_dir_page([_make_file_item("b.txt", "disk:/sub/b.txt")]))
        if url == f"{API_BASE}/resources/download":
            return FakeResponse({"href": "https://download.example.com/file"})
        if url == "https://download.example.com/file":
            return FakeResponse(content=b"hello world")
        raise AssertionError(f"unexpected request: {url}")

    with _connector(router) as connector:
        batches = list(connector.load_from_state())
    docs = [doc for batch in batches for doc in batch]

    assert len(docs) == 2
    a, b = docs
    assert a.id == "yandex_disk:disk:/docs/a.pdf"
    assert a.semantic_identifier == "a.pdf"
    assert a.extension == ".pdf"
    assert a.blob == b"hello world"
    assert a.source == "yandex_disk"
    assert a.doc_updated_at == datetime(2024, 1, 1, tzinfo=timezone.utc)
    assert a.size_bytes == 100
    assert b.id == "yandex_disk:disk:/sub/b.txt"


def test_poll_source_filters_by_modified_window():
    def router(url, **kwargs):
        if url == f"{API_BASE}/resources":
            return FakeResponse(
                _make_dir_page(
                    [
                        _make_file_item("old.txt", "disk:/old.txt", modified="2020-01-01T00:00:00Z"),
                        _make_file_item("new.txt", "disk:/new.txt", modified="2024-06-01T00:00:00Z"),
                    ]
                )
            )
        if url == f"{API_BASE}/resources/download":
            return FakeResponse({"href": "https://download.example.com/file"})
        if url == "https://download.example.com/file":
            return FakeResponse(content=b"data")
        raise AssertionError(f"unexpected request: {url}")

    with _connector(router) as connector:
        start = datetime(2024, 1, 1, tzinfo=timezone.utc).timestamp()
        end = datetime(2024, 12, 31, tzinfo=timezone.utc).timestamp()
        docs = [doc for batch in connector.poll_source(start, end) for doc in batch]
    assert [d.semantic_identifier for d in docs] == ["new.txt"]


def test_size_threshold_skips_large_files():
    def router(url, **kwargs):
        if url == f"{API_BASE}/resources":
            return FakeResponse(
                _make_dir_page([_make_file_item("huge.pdf", "disk:/huge.pdf", size=99)])
            )
        if url == f"{API_BASE}/resources/download":
            return FakeResponse({"href": "https://download.example.com/file"})
        if url == "https://download.example.com/file":
            return FakeResponse(content=b"data")
        raise AssertionError(f"unexpected request: {url}")

    with _connector(router, size_threshold=10) as connector:
        docs = [doc for batch in connector.load_from_state() for doc in batch]
    assert docs == []


def test_list_keys_and_get_value_fingerprint_flow():
    def router(url, **kwargs):
        if url == f"{API_BASE}/resources":
            return FakeResponse(
                _make_dir_page([_make_file_item("a.pdf", "disk:/a.pdf", md5="m1")])
            )
        if url == f"{API_BASE}/resources/download":
            return FakeResponse({"href": "https://download.example.com/file"})
        if url == "https://download.example.com/file":
            return FakeResponse(content=b"data")
        raise AssertionError(f"unexpected request: {url}")

    with _connector(router) as connector:
        key_records = list(connector.list_keys())
        assert len(key_records) == 1
        assert key_records[0].key == "yandex_disk:disk:/a.pdf"
        assert key_records[0].fingerprint is not None

        doc = connector.get_value(key_records[0].key)
        assert doc.semantic_identifier == "a.pdf"
        assert doc.blob == b"data"


def test_retrieve_all_slim_docs_perm_sync():
    def router(url, **kwargs):
        if url == f"{API_BASE}/resources":
            path = kwargs.get("params", {}).get("path")
            if path == "disk:/sub":
                return FakeResponse(_make_dir_page([_make_file_item("b.txt", "disk:/sub/b.txt")]))
            return FakeResponse(
                _make_dir_page(
                    [
                        _make_file_item("a.pdf", "disk:/a.pdf"),
                        _make_dir_item("sub", "disk:/sub"),
                    ]
                )
            )
        raise AssertionError(f"unexpected request: {url}")

    with _connector(router) as connector:
        slim_docs = [d for batch in connector.retrieve_all_slim_docs_perm_sync() for d in batch]
    ids = {d.id for d in slim_docs}
    assert ids == {"yandex_disk:disk:/a.pdf", "yandex_disk:disk:/sub/b.txt"}


def test_validate_connector_settings_ok():
    def router(url, **kwargs):
        if url == API_BASE:
            return FakeResponse({"total_space": 100})
        if url == f"{API_BASE}/resources":
            return FakeResponse(_make_dir_page([]))
        raise AssertionError(f"unexpected request: {url}")

    with _connector(router) as connector:
        connector.validate_connector_settings()


def test_validate_connector_settings_401_raises():
    def router(url, **kwargs):
        return FakeResponse(status_code=401)

    with _connector(router) as connector, pytest.raises(CredentialExpiredError):
        connector.validate_connector_settings()


def test_validate_connector_settings_404_raises():
    def router(url, **kwargs):
        if url == API_BASE:
            return FakeResponse({"total_space": 100})
        if url == f"{API_BASE}/resources":
            return FakeResponse(status_code=404)
        raise AssertionError(f"unexpected request: {url}")

    with _connector(router) as connector, pytest.raises(ConnectorValidationError):
        connector.validate_connector_settings()

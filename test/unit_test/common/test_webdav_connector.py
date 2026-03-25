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

from datetime import datetime, timezone

from common.data_source.webdav_connector import WebDAVConnector


class FakeWebDAVClient:
    def __init__(self, payloads: dict[str, bytes] | None = None, listings: dict[str, list[dict]] | None = None):
        self.payloads = payloads or {}
        self.listings = listings or {}
        self.downloaded_paths: list[str] = []

    def ls(self, path, detail=True):
        return self.listings.get(path, [])

    def download_fileobj(self, file_path, buffer):
        self.downloaded_paths.append(file_path)
        buffer.write(self.payloads.get(file_path, f"blob:{file_path}".encode("utf-8")))


def _build_connector(files, *, allow_images=False, size_threshold=None):
    connector = WebDAVConnector(base_url="https://dav.example.com", remote_path="/")
    connector.client = FakeWebDAVClient()
    connector.set_allow_images(allow_images)
    connector.size_threshold = size_threshold
    connector._list_files_recursive = lambda path, start, end: files
    return connector


def _generate_documents(connector: WebDAVConnector):
    start = datetime(2024, 1, 1, tzinfo=timezone.utc)
    end = datetime(2024, 1, 2, tzinfo=timezone.utc)
    batches = list(connector._yield_webdav_documents(start, end))
    return [doc for batch in batches for doc in batch]


def test_webdav_connector_skips_unsupported_extensions_before_download():
    modified = datetime(2024, 1, 1, 12, 0, tzinfo=timezone.utc)
    connector = _build_connector([
        ("/docs/notes.txt", {"size": 12, "modified": modified}),
        ("/docs/archive.exe", {"size": 12, "modified": modified}),
    ])

    docs = _generate_documents(connector)

    assert [doc.semantic_identifier for doc in docs] == ["notes.txt"]
    assert [doc.extension for doc in docs] == [".txt"]
    assert connector.client.downloaded_paths == ["/docs/notes.txt"]


def test_webdav_connector_filters_unsupported_extensions_during_listing():
    modified = datetime(2024, 1, 1, 12, 0, tzinfo=timezone.utc)
    connector = WebDAVConnector(base_url="https://dav.example.com", remote_path="/")
    connector.client = FakeWebDAVClient(listings={
        "/": [
            {"name": "/docs/notes.txt", "type": "file", "size": 12, "modified": modified},
            {"name": "/docs/archive.exe", "type": "file", "size": 12, "modified": modified},
        ]
    })

    files = connector._list_files_recursive(
        "/",
        datetime(2024, 1, 1, tzinfo=timezone.utc),
        datetime(2024, 1, 2, tzinfo=timezone.utc),
    )

    assert files == [("/docs/notes.txt", {"name": "/docs/notes.txt", "type": "file", "size": 12, "modified": modified})]


def test_webdav_connector_skips_images_when_allow_images_is_false():
    modified = datetime(2024, 1, 1, 12, 0, tzinfo=timezone.utc)
    connector = _build_connector([
        ("/docs/diagram.png", {"size": 12, "modified": modified}),
    ], allow_images=False)

    docs = _generate_documents(connector)

    assert docs == []
    assert connector.client.downloaded_paths == []


def test_webdav_connector_keeps_images_when_allow_images_is_true():
    modified = datetime(2024, 1, 1, 12, 0, tzinfo=timezone.utc)
    connector = _build_connector([
        ("/docs/diagram.png", {"size": 12, "modified": modified}),
    ], allow_images=True)

    docs = _generate_documents(connector)

    assert [doc.semantic_identifier for doc in docs] == ["diagram.png"]
    assert [doc.extension for doc in docs] == [".png"]
    assert connector.client.downloaded_paths == ["/docs/diagram.png"]


def test_webdav_connector_still_skips_large_supported_files():
    modified = datetime(2024, 1, 1, 12, 0, tzinfo=timezone.utc)
    connector = _build_connector([
        ("/docs/large.txt", {"size": 100, "modified": modified}),
    ], size_threshold=10)

    docs = _generate_documents(connector)

    assert docs == []
    assert connector.client.downloaded_paths == []


def test_webdav_connector_normalizes_uppercase_extensions():
    modified = datetime(2024, 1, 1, 12, 0, tzinfo=timezone.utc)
    connector = _build_connector([
        ("/docs/Report.PDF", {"size": 12, "modified": modified}),
    ])

    docs = _generate_documents(connector)

    assert [doc.semantic_identifier for doc in docs] == ["Report.PDF"]
    assert [doc.extension for doc in docs] == [".pdf"]
    assert connector.client.downloaded_paths == ["/docs/Report.PDF"]

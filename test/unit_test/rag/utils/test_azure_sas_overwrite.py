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

"""Exercise Azure SAS writes through the real SDK and a local HTTP peer."""

import importlib.util
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from threading import Thread
from types import ModuleType
from unittest.mock import Mock
from urllib.parse import urlsplit

import pytest
from azure.storage.blob import ContainerClient

import common

pytestmark = pytest.mark.p2


@pytest.fixture
def blob_peer():
    """Implement only block-blob PUT/GET and the create-only HTTP condition."""
    objects = {}
    writes = []

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def reply(self, status, body=b"", **headers):
            self.send_response(status)
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Content-Type", "application/octet-stream")
            self.send_header("ETag", '"test-etag"')
            self.send_header("Last-Modified", "Tue, 01 Sep 2026 00:00:00 GMT")
            self.send_header("x-ms-blob-type", "BlockBlob")
            for key, value in headers.items():
                self.send_header(key, value)
            self.end_headers()
            self.wfile.write(body)

        def do_PUT(self):
            path = urlsplit(self.path).path
            body = self.rfile.read(int(self.headers["Content-Length"]))
            condition = self.headers.get("If-None-Match")
            writes.append((path, condition))
            if path in objects and condition == "*":
                self.reply(409, b"<Error><Code>BlobAlreadyExists</Code><Message>Exists</Message></Error>", **{"x-ms-error-code": "BlobAlreadyExists"})
                return
            objects[path] = body
            self.reply(201)

        def do_GET(self):
            path = urlsplit(self.path).path
            if path not in objects:
                self.reply(404, **{"x-ms-error-code": "BlobNotFound"})
                return
            body = objects[path]
            if body:
                self.reply(206, body, **{"Content-Range": f"bytes 0-{len(body) - 1}/{len(body)}"})
            elif self.headers.get("x-ms-range") or self.headers.get("Range"):
                self.reply(416, **{"x-ms-error-code": "InvalidRange"})
            else:
                self.reply(200)

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}", objects, writes
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


@pytest.fixture
def storage(blob_peer, monkeypatch):
    """Use the original adapter methods with an isolated, real Azure client."""
    settings = ModuleType("common.settings")
    monkeypatch.setitem(sys.modules, "common.settings", settings)
    monkeypatch.setattr(common, "settings", settings, raising=False)
    path = Path(__file__).resolve().parents[4] / "rag/utils/azure_sas_conn.py"
    spec = importlib.util.spec_from_file_location("test_azure_sas_adapter", path)
    module = importlib.util.module_from_spec(spec)
    with monkeypatch.context() as load_patch:
        load_patch.setattr("common.decorator.singleton", lambda cls: cls)
        spec.loader.exec_module(module)
    adapter = module.RAGFlowAzureSasBlob.__new__(module.RAGFlowAzureSasBlob)
    adapter.conn = ContainerClient(blob_peer[0], "container", credential=None, retry_total=0, connection_timeout=2, read_timeout=2)
    # Failed baseline writes must keep using the same peer without retry delays.
    monkeypatch.setattr(adapter, "__open__", Mock())
    monkeypatch.setattr(module.time, "sleep", Mock())
    try:
        yield adapter
    finally:
        adapter.conn.close()


@pytest.mark.parametrize("replacement", [b"replacement content", b""])
def test_put_replaces_existing_object(storage, blob_peer, replacement):
    assert storage.put("kb", "file.txt", b"original") is not None
    assert storage.get("kb", "file.txt") == b"original"
    assert storage.put("kb", "file.txt", replacement) is not None
    assert storage.get("kb", "file.txt") == replacement
    assert blob_peer[1]["/container/kb/file.txt"] == replacement
    assert len(blob_peer[2]) == 2
    storage.__open__.assert_not_called()


def test_put_preserves_other_bucket_objects(storage):
    storage.put("first-kb", "file.txt", b"first")
    storage.put("second-kb", "file.txt", b"second")
    storage.put("first-kb", "file.txt", b"updated")
    assert storage.get("first-kb", "file.txt") == b"updated"
    assert storage.get("second-kb", "file.txt") == b"second"


def test_health_can_run_repeatedly(storage, blob_peer):
    for _ in range(3):
        assert storage.health() is not None
    assert blob_peer[1] == {"/container/txtxtxtxt1/txtxtxtxt1": b"_t@@@1"}
    assert len(blob_peer[2]) == 3

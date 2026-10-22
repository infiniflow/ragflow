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

"""Exercise SPN existence checks with the real Data Lake SDK and a local peer."""

import importlib.util
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from threading import Thread
from types import ModuleType
from urllib.parse import unquote, urlsplit

import pytest
from azure.storage.filedatalake import FileSystemClient

import common

pytestmark = pytest.mark.p2


@pytest.fixture
def file_peer():
    """Serve only HEAD requests, including missing and inaccessible objects."""
    paths = {"/container/kb/file.txt", "/container/kb/folder/space name.txt"}
    requests = []

    class Handler(BaseHTTPRequestHandler):
        """Return deterministic Data Lake existence responses for the SDK."""

        def log_message(self, *args):
            """Suppress the HTTP server's default stderr logging."""

        def do_HEAD(self):
            """Record the decoded file path and return its configured HTTP status."""
            path = unquote(urlsplit(self.path).path)
            requests.append(path)
            status = 200 if path in paths else 404
            error = "PathNotFound"
            if path == "/container/kb/denied.txt":
                status, error = 403, "AuthorizationPermissionMismatch"
            self.send_response(status)
            self.send_header("Content-Length", "0")
            self.send_header("x-ms-request-id", "local-test")
            if status != 200:
                self.send_header("x-ms-error-code", error)
            self.end_headers()

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}", requests
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


@pytest.fixture
def storage(file_peer, monkeypatch):
    """Isolate service config and singleton setup; retain the real SDK methods."""
    settings = ModuleType("common.settings")
    monkeypatch.setitem(sys.modules, "common.settings", settings)
    monkeypatch.setattr(common, "settings", settings, raising=False)
    path = Path(__file__).resolve().parents[4] / "rag/utils/azure_spn_conn.py"
    spec = importlib.util.spec_from_file_location("test_azure_spn_exists_adapter", path)
    module = importlib.util.module_from_spec(spec)
    with monkeypatch.context() as load_patch:
        load_patch.setattr("common.decorator.singleton", lambda cls: cls)
        spec.loader.exec_module(module)
    adapter = module.RAGFlowAzureSpnBlob.__new__(module.RAGFlowAzureSpnBlob)
    adapter.conn = FileSystemClient(file_peer[0], "container", credential=None, retry_total=0, connection_timeout=2, read_timeout=2)
    try:
        yield adapter
    finally:
        adapter.conn.close()


@pytest.mark.parametrize("tenant_id", [None, "tenant-a"])
@pytest.mark.parametrize(
    "bucket,filename,exists",
    [("kb", "file.txt", True), ("kb", "folder/space name.txt", True), ("kb", "missing.txt", False), ("other-kb", "file.txt", False)],
)
def test_obj_exist_checks_the_requested_file(storage, file_peer, bucket, filename, exists, tenant_id):
    """Existing, absent and same-name files in another bucket retain their status."""
    assert storage.obj_exist(bucket, filename, tenant_id) is exists
    assert file_peer[1] == [f"/container/{bucket}/{filename}"]


def test_obj_exist_logs_service_errors(storage, file_peer, caplog):
    """The adapter still logs a failed request and returns False."""
    assert storage.obj_exist("kb", "denied.txt") is False
    assert file_peer[1] == ["/container/kb/denied.txt"]
    assert "Fail obj_exist kb/denied.txt" in caplog.text

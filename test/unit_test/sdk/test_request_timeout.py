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

"""Verify SDK timeout configuration through Requests and local HTTP sockets."""

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Event, Thread

import pytest
import requests
from ragflow_sdk import RAGFlow

pytestmark = pytest.mark.p2


@pytest.fixture
def transport_api(monkeypatch):
    """Record actual HTTP requests and optionally stall headers or stream data."""
    release = Event()
    state = {"requests": [], "timeouts": []}
    original_send = requests.adapters.HTTPAdapter.send

    def send(adapter, request, **kwargs):
        """Observe the timeout passed to Requests without mocking the response."""
        state["timeouts"].append(kwargs.get("timeout"))
        return original_send(adapter, request, **kwargs)

    monkeypatch.setattr(requests.adapters.HTTPAdapter, "send", send)

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            """Suppress local server logging."""

        def respond(self):
            """Serve a short body, stalled response headers, or stalled stream."""
            body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
            state["requests"].append((self.command, self.path, self.headers, body))
            if self.path.endswith("/slow-headers"):
                release.wait(3)
            self.send_response(200)
            self.send_header("Content-Length", "2")
            self.end_headers()
            try:
                if self.path.endswith("/slow-body"):
                    self.wfile.write(b"a")
                    self.wfile.flush()
                    release.wait(3)
                    self.wfile.write(b"b")
                else:
                    self.wfile.write(b"ok")
            except (BrokenPipeError, ConnectionResetError):
                pass  # The client has already timed out and closed the socket.

        do_GET = respond
        do_POST = respond
        do_PUT = respond
        do_PATCH = respond
        do_DELETE = respond

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}", state
    finally:
        release.set()
        server.shutdown()
        server.server_close()
        thread.join()


@pytest.mark.parametrize("method", ["get", "post", "put", "patch", "delete"])
@pytest.mark.parametrize("config", [{}, {"timeout": None}, {"timeout": 2}, {"timeout": (1, 2.5)}])
def test_timeout_applies_to_every_http_method(transport_api, method, config):
    """Send real requests with the configured timeout and preserve JSON/auth."""
    url, state = transport_api
    rag = RAGFlow("test-key", url, "v1", **config)
    response = getattr(rag, method)("/echo", json={"key": "value"})
    assert response.content == b"ok"
    assert state["timeouts"] == [config.get("timeout")]
    assert len(state["requests"]) == 1
    verb, path, headers, body = state["requests"][0]
    assert (verb, path) == (method.upper(), "/api/v1/echo")
    assert headers["Authorization"] == "Bearer test-key"
    assert body == b'{"key": "value"}'


def test_timeout_preserves_multipart_uploads(transport_api):
    """Upload file bytes while using the same connection/read configuration."""
    url, state = transport_api
    rag = RAGFlow("test-key", url, timeout=(1, 2))
    assert rag.post("/upload", files={"file": ("sample.txt", b"sample content")}).content == b"ok"
    assert state["timeouts"] == [(1, 2)]
    assert state["requests"][0][2]["Content-Type"].startswith("multipart/form-data;")
    assert b"sample content" in state["requests"][0][3]


@pytest.mark.parametrize("stream", [False, True])
def test_timeout_bounds_wait_for_response_headers(transport_api, stream):
    """Raise a real ReadTimeout while the local server withholds headers."""
    url, state = transport_api
    rag = RAGFlow("test-key", url, timeout=(1, 0.1))
    with pytest.raises(requests.exceptions.ReadTimeout):
        rag.post("/slow-headers", stream=stream)
    assert len(state["requests"]) == 1


def test_timeout_bounds_idle_stream_reads(transport_api):
    """Preserve Requests' ConnectionError when a started stream stops sending."""
    url, state = transport_api
    rag = RAGFlow("test-key", url, timeout=(1, 0.1))
    with rag.post("/slow-body", stream=True) as response:
        parts = response.iter_content(chunk_size=1)
        assert next(parts) == b"a"
        with pytest.raises(requests.exceptions.ConnectionError, match="Read timed out"):
            next(parts)
    assert len(state["requests"]) == 1


def test_timeout_preserves_connect_timeout(transport_api, monkeypatch):
    """Pass the connect value to urllib3 and preserve its timeout exception."""
    url, state = transport_api
    observed = []

    def fail_connect(address, timeout, **kwargs):
        """Simulate a connection attempt that exhausts its supplied timeout."""
        observed.append(timeout)
        raise TimeoutError("connection timed out")

    monkeypatch.setattr("urllib3.connection.connection.create_connection", fail_connect)
    with pytest.raises(requests.exceptions.ConnectTimeout):
        RAGFlow("test-key", url, timeout=(0.2, 1)).get("/echo")
    assert observed == [0.2]
    assert state["requests"] == []


@pytest.mark.parametrize("timeout", [0, -1, True, False, float("nan"), float("inf"), "1", [], [1, 2], (), (1,), (1, 2, 3), (0, 1), (1, -1), (1, None), (None, 1), (1, True), (1, float("inf"))])
def test_timeout_rejects_invalid_configuration(timeout):
    """Reject unsupported configurations before any request can start."""
    with pytest.raises(ValueError, match="timeout must be"):
        RAGFlow("test-key", "http://unused.invalid", timeout=timeout)


@pytest.mark.parametrize("sign", [1, -1], ids=["positive", "negative"])
@pytest.mark.parametrize("position", ["scalar", "connect", "read"])
def test_timeout_rejects_oversized_integers_consistently(sign, position):
    """Reject integers outside float range with the documented validation error."""
    value = sign * 10**1000
    timeout = {"scalar": value, "connect": (value, 1), "read": (1, value)}[position]
    with pytest.raises(ValueError, match="timeout must be"):
        RAGFlow("test-key", "http://unused.invalid", timeout=timeout)

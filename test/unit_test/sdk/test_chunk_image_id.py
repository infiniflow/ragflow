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

"""Keep image references when constructing chunks from public SDK endpoints."""

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Thread

import pytest
from ragflow_sdk import Chunk, Document, RAGFlow

pytestmark = pytest.mark.p2


@pytest.fixture
def chunk_api():
    """Return the chunk envelopes used by add, list and retrieval endpoints."""
    state = {"chunk": {"id": "chunk-1", "content": "caption", "document_id": "doc", "dataset_id": "kb"}, "requests": []}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def respond(self):
            state["requests"].append((self.command, self.path))
            self.rfile.read(int(self.headers.get("Content-Length", "0")))
            if self.command == "POST" and self.path.endswith("/documents/doc/chunks"):
                data = {"chunk": state["chunk"]}
            else:
                data = {"chunks": [state["chunk"]], "total": 1}
            body = json.dumps({"code": 0, "data": data}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        do_GET = respond
        do_POST = respond

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield RAGFlow("test-key", f"http://127.0.0.1:{server.server_port}"), state
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


@pytest.mark.parametrize("endpoint", ["add", "list", "retrieve"])
@pytest.mark.parametrize("image_id", ["kb-image-1", ""])
def test_chunk_endpoints_preserve_image_id(chunk_api, endpoint, image_id):
    rag, state = chunk_api
    state["chunk"]["image_id"] = image_id
    doc = Document(rag, {"id": "doc", "dataset_id": "kb"})
    if endpoint == "add":
        chunk = doc.add_chunk("caption")
    elif endpoint == "list":
        chunk = doc.list_chunks()[0]
    else:
        chunk = rag.retrieve(["kb"], question="caption")[0]
    assert chunk.image_id == image_id
    assert chunk.to_json()["image_id"] == image_id
    assert chunk.content == "caption"
    assert len(state["requests"]) == 1


def test_chunk_without_an_image_field_defaults_to_empty_string():
    assert Chunk(None, {"id": "text-chunk"}).image_id == ""


def test_unknown_chunk_fields_are_still_filtered():
    chunk = Chunk(None, {"id": "chunk", "internal_field": "not public"})
    assert "internal_field" not in chunk.to_json()

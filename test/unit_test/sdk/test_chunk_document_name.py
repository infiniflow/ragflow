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

"""Expose the document name returned by the chunk listing API."""

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Thread
from urllib.parse import parse_qs, urlsplit

import pytest
from ragflow_sdk import Chunk, Document, RAGFlow

pytestmark = pytest.mark.p2


@pytest.fixture
def chunk_list_api():
    state = {"document_name": "report.txt", "queries": []}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_GET(self):
            url = urlsplit(self.path)
            if url.path != "/api/v1/datasets/kb/documents/doc/chunks":
                self.send_error(404)
                return
            state["queries"].append(parse_qs(url.query))
            chunk = {
                "id": "chunk-1",
                "content": "Report content",
                "docnm_kwd": state["document_name"],
                "document_id": "doc",
                "dataset_id": "kb",
                "important_keywords": [],
                "questions": [],
                "tag_kwd": [],
                "image_id": "",
                "doc_type_kwd": "text",
                "available": True,
                "positions": [],
            }
            body = json.dumps({"code": 0, "data": {"chunks": [chunk], "total": 1}}, ensure_ascii=False).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        rag = RAGFlow("test-key", f"http://127.0.0.1:{server.server_port}")
        yield Document(rag, {"id": "doc", "dataset_id": "kb"}), state
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


@pytest.mark.parametrize("document_name", ["report.txt", "季度报告.txt"])
@pytest.mark.parametrize("chunk_id", ["", "chunk-1"])
def test_list_chunks_preserves_document_name(chunk_list_api, document_name, chunk_id):
    document, state = chunk_list_api
    state["document_name"] = document_name

    chunks = document.list_chunks(id=chunk_id)

    assert len(chunks) == 1
    chunk = chunks[0]
    assert chunk.document_name == document_name
    assert chunk.to_json()["document_name"] == document_name
    assert "docnm_kwd" not in chunk.to_json()
    assert chunk.content == "Report content"
    assert chunk.document_id == "doc"
    assert chunk.dataset_id == "kb"
    assert len(state["queries"]) == 1
    assert state["queries"][0].get("id", [""]) == [chunk_id]


@pytest.mark.parametrize(
    "fields,expected",
    [
        ({"document_name": "retrieved.txt"}, "retrieved.txt"),
        ({"document_keyword": "keyword.txt"}, "keyword.txt"),
        ({"document_name": "retrieved.txt", "docnm_kwd": "listed.txt"}, "retrieved.txt"),
        ({"document_name": "", "document_keyword": "keyword.txt"}, "keyword.txt"),
        ({"docnm_kwd": ""}, ""),
        ({}, ""),
    ],
)
def test_chunk_document_name_preserves_existing_fields_and_defaults(fields, expected):
    chunk = Chunk(None, {"id": "chunk-1", **fields})

    assert chunk.document_name == expected
    assert chunk.to_json()["document_name"] == expected

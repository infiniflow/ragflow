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

"""Exercise document filters through the real SDK and Requests query encoder."""

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Thread
from urllib.parse import parse_qs, urlsplit

import pytest
from ragflow_sdk import DataSet, Document, RAGFlow

pytestmark = pytest.mark.p2


@pytest.fixture
def document_api():
    """Capture document-list queries and return scripted API envelopes."""
    state = {"queries": [], "response": {"code": 0, "data": {"docs": [{"id": "doc", "name": "report.pdf", "run": "FAIL"}]}}}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            """Suppress test-server request logging."""

        def do_GET(self):
            """Record decoded repeated keys without reimplementing filtering."""
            assert urlsplit(self.path).path == "/api/v1/datasets/kb/documents"
            state["queries"].append(parse_qs(urlsplit(self.path).query))
            body = json.dumps(state["response"]).encode()
            self.send_response(200)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield DataSet(RAGFlow("test-key", f"http://127.0.0.1:{server.server_port}"), {"id": "kb"}), state
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


@pytest.mark.parametrize(
    "filters",
    [
        {"run": ["FAIL"]},
        {"suffix": ["pdf"]},
        {"run": ["FAIL", "RUNNING"], "suffix": ["pdf", "docx"]},
        {"run": ["4", "1"]},
        {"run": ["FAILED", "COMPLETED"]},
        {"run": [" fail "], "suffix": ["PDF", "报告"]},
    ],
)
def test_list_documents_encodes_filters_as_repeated_query_keys(document_api, filters):
    """Forward server-specific values unchanged and deserialize returned documents."""
    dataset, state = document_api
    docs = dataset.list_documents(**filters)
    assert isinstance(docs[0], Document)
    assert (docs[0].id, docs[0].name, docs[0].run) == ("doc", "report.pdf", "FAIL")
    assert len(state["queries"]) == 1
    query = state["queries"][0]
    for key in ("run", "suffix"):
        assert query.get(key) == filters.get(key)


@pytest.mark.parametrize("filters", [{}, {"run": None, "suffix": None}, {"run": [], "suffix": []}])
def test_list_documents_omits_unset_filters(document_api, filters):
    """Keep the default wire query unchanged when filters are not supplied."""
    dataset, state = document_api
    dataset.list_documents(**filters)
    assert "run" not in state["queries"][0]
    assert "suffix" not in state["queries"][0]


def test_list_documents_combines_filters_with_existing_arguments(document_api):
    """Keep all existing positional parameters and repeated document IDs intact."""
    dataset, state = document_api
    dataset.list_documents(None, ["doc", "other"], "report.pdf", "report & review", 2, 7, "name", False, 10, 20, run=["FAIL"], suffix=["pdf"])
    assert state["queries"] == [
        {
            "ids": ["doc", "other"],
            "name": ["report.pdf"],
            "keywords": ["report & review"],
            "page": ["2"],
            "page_size": ["7"],
            "orderby": ["name"],
            "desc": ["False"],
            "create_time_from": ["10"],
            "create_time_to": ["20"],
            "run": ["FAIL"],
            "suffix": ["pdf"],
        }
    ]


def test_list_documents_preserves_server_validation_errors(document_api):
    """Let the server validate status names and surface its error unchanged."""
    dataset, state = document_api
    state["response"] = {"code": 102, "message": "Invalid filter run status conditions: invalid"}
    with pytest.raises(Exception, match="Invalid filter run status conditions: invalid"):
        dataset.list_documents(run=["invalid"], suffix=["pdf"])
    assert state["queries"][0]["run"] == ["invalid"]


def test_list_documents_preserves_empty_results(document_api):
    """Return an empty document list when the server finds no matches."""
    dataset, state = document_api
    state["response"] = {"code": 0, "data": {"docs": []}}
    assert dataset.list_documents(run=["FAIL"], suffix=["pdf"]) == []


def test_list_documents_keeps_id_conflict_validation(document_api):
    """Reject conflicting ID selectors before making a filtered request."""
    dataset, state = document_api
    with pytest.raises(ValueError, match="Cannot use both"):
        dataset.list_documents(id="doc", ids=["doc"], run=["FAIL"])
    assert state["queries"] == []

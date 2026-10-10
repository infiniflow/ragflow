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

"""Synchronous parsing must stop when a status lookup can no longer succeed."""

import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Thread
from urllib.parse import parse_qs, urlsplit

import pytest
import requests
from ragflow_sdk import DataSet, RAGFlow

pytestmark = pytest.mark.p2


@pytest.fixture
def parsing_api(monkeypatch):
    state = {"responses": {}, "requests": [], "sleeps": 0}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def reply(self, payload):
            body = json.dumps(payload).encode() if isinstance(payload, dict) else payload
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_POST(self):
            self.rfile.read(int(self.headers.get("Content-Length", "0")))
            state["requests"].append(("POST", self.path))
            self.reply({"code": 0})

        def do_GET(self):
            doc_id = parse_qs(urlsplit(self.path).query)["id"][0]
            state["requests"].append(("GET", doc_id))
            replies = state["responses"][doc_id]
            self.reply(replies.pop(0) if len(replies) > 1 else replies[0])

    def bounded_sleep(_seconds):
        state["sleeps"] += 1
        if state["sleeps"] > 2:
            pytest.fail("status polling did not stop after a lookup failure")

    monkeypatch.setattr(time, "sleep", bounded_sleep)
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        rag = RAGFlow("test-key", f"http://127.0.0.1:{server.server_port}")
        yield DataSet(rag, {"id": "kb"}), state
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


def document_status(doc_id="doc", run="DONE", progress=1.0):
    return {"code": 0, "data": {"docs": [{"id": doc_id, "run": run, "progress": progress, "chunk_count": 2, "token_count": 10}]}}


@pytest.mark.parametrize("message", ["Permission denied", "Dataset not found"])
def test_parse_propagates_status_api_error(parsing_api, message):
    dataset, state = parsing_api
    state["responses"]["doc"] = [{"code": 102, "message": message}]
    with pytest.raises(Exception, match=message):
        dataset.parse_documents(["doc"])
    assert state["requests"] == [("POST", "/api/v1/datasets/kb/chunks"), ("GET", "doc")]
    assert state["sleeps"] == 0


def test_parse_stops_if_document_disappears(parsing_api):
    dataset, state = parsing_api
    state["responses"]["doc"] = [{"code": 0, "data": {"docs": []}}]
    with pytest.raises(RuntimeError, match="doc.*not found"):
        dataset.parse_documents(["doc"])
    assert state["sleeps"] == 0


def test_parse_propagates_invalid_status_response(parsing_api):
    dataset, state = parsing_api
    state["responses"]["doc"] = [b"invalid json"]
    with pytest.raises(requests.exceptions.JSONDecodeError):
        dataset.parse_documents(["doc"])
    assert state["sleeps"] == 0


def test_parse_preserves_transport_error(parsing_api, monkeypatch):
    dataset, state = parsing_api
    error = requests.ConnectionError("connection lost")

    def fail_lookup(**_kwargs):
        raise error

    monkeypatch.setattr(dataset, "list_documents", fail_lookup)
    with pytest.raises(requests.ConnectionError) as exc:
        dataset.parse_documents(["doc"])
    assert exc.value is error
    assert state["sleeps"] == 0


@pytest.mark.parametrize("run,progress", [("DONE", 1.0), ("FAIL", -1.0), ("CANCEL", 0.3), ("RUNNING", 1.0)])
def test_parse_returns_first_completed_status_without_polling_again(parsing_api, run, progress):
    dataset, state = parsing_api
    # Once terminal status is observed, a later missing document must not erase
    # the result or restart the wait.
    state["responses"]["doc"] = [document_status(run=run, progress=progress), {"code": 0, "data": {"docs": []}}]
    expected_run = "DONE" if run == "RUNNING" else run
    assert dataset.parse_documents(["doc"]) == [("doc", expected_run, 2, 10)]
    assert state["requests"].count(("GET", "doc")) == 1


def test_parse_still_polls_running_documents(parsing_api):
    dataset, state = parsing_api
    state["responses"]["doc"] = [document_status(run="RUNNING", progress=0.4), document_status()]
    assert dataset.parse_documents(["doc"]) == [("doc", "DONE", 2, 10)]
    assert state["sleeps"] == 1
    assert state["requests"].count(("GET", "doc")) == 2


def test_keyboard_interrupt_still_cancels_and_collects_status(monkeypatch):
    dataset = DataSet(None, {"id": "kb"})
    calls = []
    monkeypatch.setattr(dataset, "async_parse_documents", lambda ids: calls.append(("start", ids)))
    monkeypatch.setattr(dataset, "async_cancel_parse_documents", lambda ids: calls.append(("cancel", ids)))

    def status(ids):
        calls.append(("status", ids))
        if len(calls) == 2:
            raise KeyboardInterrupt
        return [("doc", "CANCEL", 0, 0)]

    monkeypatch.setattr(dataset, "_get_documents_status", status)
    assert dataset.parse_documents(["doc"]) == [("doc", "CANCEL", 0, 0)]
    assert calls == [("start", ["doc"]), ("status", ["doc"]), ("cancel", ["doc"]), ("status", ["doc"])]


def test_parse_does_not_requery_finished_documents_in_a_batch(parsing_api):
    dataset, state = parsing_api
    state["responses"]["finished"] = [document_status("finished")]
    state["responses"]["running"] = [document_status("running", "RUNNING", 0.2), document_status("running")]
    assert sorted(dataset.parse_documents(["finished", "running"])) == [("finished", "DONE", 2, 10), ("running", "DONE", 2, 10)]
    assert state["requests"].count(("GET", "finished")) == 1
    assert state["requests"].count(("GET", "running")) == 2
    assert state["sleeps"] == 1


def test_parse_start_error_is_not_replaced_by_polling(monkeypatch):
    dataset = DataSet(None, {"id": "kb"})
    error = RuntimeError("parse request rejected")

    def reject_start(_ids):
        raise error

    monkeypatch.setattr(dataset, "async_parse_documents", reject_start)
    monkeypatch.setattr(dataset, "_get_documents_status", lambda _ids: pytest.fail("must not poll after a rejected start"))
    with pytest.raises(RuntimeError) as exc:
        dataset.parse_documents(["doc"])
    assert exc.value is error

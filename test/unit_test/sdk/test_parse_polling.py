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

"""Synchronous parsing reports progress and stops when a lookup cannot succeed."""

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

        def do_DELETE(self):
            self.rfile.read(int(self.headers.get("Content-Length", "0")))
            state["requests"].append(("DELETE", self.path))
            self.reply({"code": 0})

        def do_GET(self):
            doc_id = parse_qs(urlsplit(self.path).query)["id"][0]
            state["requests"].append(("GET", doc_id))
            replies = state["responses"][doc_id]
            self.reply(replies.pop(0) if len(replies) > 1 else replies[0])

    def bounded_sleep(_seconds):
        state["sleeps"] += 1
        if state["sleeps"] > state.get("max_sleeps", 2):
            pytest.fail("status polling exceeded the test limit")

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


def document_status(doc_id="doc", run="DONE", progress=1.0, progress_msg=""):
    return {"code": 0, "data": {"docs": [{"id": doc_id, "run": run, "progress": progress, "progress_msg": progress_msg, "chunk_count": 2, "token_count": 10}]}}


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

    def status(ids, *, on_progress=None):
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


def test_progress_reports_first_changed_and_terminal_snapshots(parsing_api):
    dataset, state = parsing_api
    state["max_sleeps"] = 5
    state["responses"]["doc"] = [
        document_status(run="RUNNING", progress=0.2, progress_msg="Reading"),
        document_status(run="RUNNING", progress=0.2, progress_msg="Reading"),
        document_status(run="RUNNING", progress=0.2, progress_msg="Embedding"),
        document_status(run="RUNNING", progress=0.6, progress_msg="Embedding"),
        document_status(run="RUNNING", progress=0.6, progress_msg="Embedding"),
        document_status(progress_msg="Complete"),
    ]
    updates = []
    assert dataset.parse_documents(["doc"], on_progress=updates.append) == [("doc", "DONE", 2, 10)]
    assert [(doc.id, doc.run, doc.progress, doc.progress_msg) for doc in updates] == [
        ("doc", "RUNNING", 0.2, "Reading"),
        ("doc", "RUNNING", 0.2, "Embedding"),
        ("doc", "RUNNING", 0.6, "Embedding"),
        ("doc", "DONE", 1.0, "Complete"),
    ]
    assert state["requests"].count(("GET", "doc")) == 6


def test_progress_is_tracked_per_document_in_a_batch(parsing_api):
    dataset, state = parsing_api
    for doc_id in ("first", "second"):
        state["responses"][doc_id] = [document_status(doc_id, "RUNNING", 0.2), document_status(doc_id)]
    state["responses"]["second"].insert(1, document_status("second", "RUNNING", 0.2))
    updates = []
    assert sorted(dataset.parse_documents(["first", "second"], on_progress=updates.append)) == [("first", "DONE", 2, 10), ("second", "DONE", 2, 10)]
    for doc_id in ("first", "second"):
        assert [(doc.run, doc.progress) for doc in updates if doc.id == doc_id] == [("RUNNING", 0.2), ("DONE", 1.0)]
    assert state["requests"].count(("GET", "first")) == 2
    assert state["requests"].count(("GET", "second")) == 3


@pytest.mark.parametrize("run,progress", [("DONE", 1.0), ("FAIL", -1.0), ("CANCEL", 0.3), ("RUNNING", 1.0)])
def test_progress_reports_a_document_that_is_already_terminal(parsing_api, run, progress):
    dataset, state = parsing_api
    state["responses"]["doc"] = [document_status(run=run, progress=progress)]
    updates = []
    expected_run = "DONE" if run == "RUNNING" else run
    assert dataset.parse_documents(["doc"], on_progress=updates.append) == [("doc", expected_run, 2, 10)]
    # The callback receives the server snapshot, even when the result infers DONE.
    assert [(doc.run, doc.progress) for doc in updates] == [(run, progress)]
    assert state["sleeps"] == 0


@pytest.mark.parametrize(
    "response,error,match",
    [
        ({"code": 102, "message": "Permission denied"}, Exception, "Permission denied"),
        ({"code": 0, "data": {"docs": []}}, RuntimeError, "not found"),
        (b"invalid json", requests.exceptions.JSONDecodeError, "Expecting value"),
    ],
)
def test_progress_preserves_lookup_errors_without_synthesizing_updates(parsing_api, response, error, match):
    dataset, state = parsing_api
    state["responses"]["doc"] = [document_status(run="RUNNING", progress=0.2), response]
    updates = []
    with pytest.raises(error, match=match):
        dataset.parse_documents(["doc"], on_progress=updates.append)
    assert [(doc.run, doc.progress) for doc in updates] == [("RUNNING", 0.2)]
    assert not any(method == "DELETE" for method, _ in state["requests"])


def test_progress_callback_error_propagates_without_cancelling(parsing_api):
    dataset, state = parsing_api
    state["responses"]["doc"] = [document_status(run="RUNNING", progress=0.2)]
    error = RuntimeError("progress consumer failed")

    def report(_doc):
        raise error

    with pytest.raises(RuntimeError) as exc:
        dataset.parse_documents(["doc"], on_progress=report)
    assert exc.value is error
    assert state["requests"] == [("POST", "/api/v1/datasets/kb/chunks"), ("GET", "doc")]
    assert state["sleeps"] == 0


def test_progress_callback_cannot_change_polling_results(parsing_api):
    dataset, state = parsing_api
    state["responses"]["doc"] = [document_status(run="RUNNING", progress=0.2), document_status()]

    def report(doc):
        doc.run = "FAIL"
        doc.progress = -1.0
        doc.chunk_count = 999

    assert dataset.parse_documents(["doc"], on_progress=report) == [("doc", "DONE", 2, 10)]
    assert state["requests"].count(("GET", "doc")) == 2


@pytest.mark.parametrize("interrupt_from_callback", [False, True])
def test_progress_continues_after_keyboard_interrupt_requests_cancellation(parsing_api, monkeypatch, interrupt_from_callback):
    dataset, state = parsing_api
    state["responses"]["doc"] = [document_status(run="RUNNING", progress=0.2), document_status(run="CANCEL", progress=0.2)]
    updates = []

    def interrupt(_seconds):
        raise KeyboardInterrupt

    def report(doc):
        updates.append(doc)
        if interrupt_from_callback and doc.run == "RUNNING":
            raise KeyboardInterrupt

    if not interrupt_from_callback:
        monkeypatch.setattr(time, "sleep", interrupt)
    assert dataset.parse_documents(["doc"], on_progress=report) == [("doc", "CANCEL", 2, 10)]
    assert [(doc.run, doc.progress) for doc in updates] == [("RUNNING", 0.2), ("CANCEL", 0.2)]
    assert state["requests"] == [
        ("POST", "/api/v1/datasets/kb/chunks"),
        ("GET", "doc"),
        ("DELETE", "/api/v1/datasets/kb/chunks"),
        ("GET", "doc"),
    ]


def test_progress_accepts_a_falsey_callable(parsing_api):
    dataset, state = parsing_api
    state["responses"]["doc"] = [document_status()]
    updates = []

    class Reporter:
        def __bool__(self):
            return False

        def __call__(self, doc):
            updates.append(doc.id)

    dataset.parse_documents(["doc"], on_progress=Reporter())
    assert updates == ["doc"]


def test_progress_is_not_called_for_an_empty_batch(parsing_api):
    dataset, state = parsing_api
    updates = []
    assert dataset.parse_documents([], on_progress=updates.append) == []
    assert updates == []
    assert state["requests"] == [("POST", "/api/v1/datasets/kb/chunks")]

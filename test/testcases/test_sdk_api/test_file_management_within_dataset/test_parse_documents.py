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

import time
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from ragflow_sdk.modules.dataset import DataSet
from ragflow_sdk.modules.document import Document


def _document_rows(dataset, document_ids):
    rows = []
    max_pages = len(document_ids) // 100 + 1
    for page in range(1, max_pages + 1):
        response = dataset.rag.get(
            f"/datasets/{dataset.id}/documents",
            params={"ids": document_ids, "page": page, "page_size": 100},
        ).json()
        if response.get("code") != 0:
            raise AssertionError(response.get("message"))
        batch = response["data"]["docs"]
        rows.extend(batch)
        if len(batch) < 100:
            return rows
    raise AssertionError(f"Document ID filter exceeded the expected {max_pages} pages")


def _wait_for_terminal_statuses(dataset, document_ids, timeout, previous_event_ids=None):
    deadline = time.monotonic() + timeout
    target_ids = set(document_ids)
    terminal = {"COMPLETED", "STOPPED", "FAILED"}
    rows = {}
    while time.monotonic() < deadline:
        rows = {row["id"]: row for row in _document_rows(dataset, document_ids)}
        terminal_rows = rows.keys() == target_ids and all(row.get("ingestion_status") in terminal for row in rows.values())
        new_events = previous_event_ids is None or all(
            isinstance(row.get("latest_ingestion_event"), dict) and row["latest_ingestion_event"].get("id") != previous_event_ids[doc_id] for doc_id, row in rows.items()
        )
        if terminal_rows and new_events:
            return rows
        time.sleep(min(1, max(0, deadline - time.monotonic())))
    statuses = {doc_id: row.get("ingestion_status") for doc_id, row in rows.items()}
    raise AssertionError(f"Parsing did not reach a terminal state within {timeout}s: {statuses}")


def _assert_completed(rows, document_ids):
    assert set(rows) == set(document_ids)
    for row in rows.values():
        assert row["ingestion_status"] == "COMPLETED"
        assert row["process_begin_at"]
        assert row["process_duration"] > 0
        assert row["progress"] >= 1
        assert row["chunk_count"] >= 0
        assert row["token_count"] >= 0


@pytest.mark.integration
class TestDocumentsParse:
    @pytest.mark.parametrize(
        "document_ids, expected_message",
        [
            (None, "`document_ids` is required"),
            ([], "`document_ids` is required"),
            (["invalid_id"], "documents not found: [invalid_id]"),
            (["\n!?。；！？\"'"], "documents not found: ["),
            ("not json", "Invalid request body"),
        ],
    )
    def test_invalid_document_ids(self, add_documents_func, document_ids, expected_message):
        dataset, _documents = add_documents_func
        with pytest.raises(Exception) as exc_info:
            dataset.async_parse_documents(document_ids=document_ids)
        assert expected_message in str(exc_info.value)

    @pytest.mark.parametrize("selection", ["one", "all"])
    def test_parse_valid_documents(self, add_documents_func, integration_settings, selection):
        dataset, documents = add_documents_func
        document_ids = [document.id for document in documents]
        if selection == "one":
            document_ids = document_ids[:1]

        assert dataset.async_parse_documents(document_ids=document_ids) is None
        rows = _wait_for_terminal_statuses(dataset, document_ids, integration_settings["parse_timeout"])
        _assert_completed(rows, document_ids)

    @pytest.mark.parametrize(
        "invalid_position",
        ["first", "middle", "last"],
    )
    def test_invalid_id_rejects_entire_parse_request(self, add_documents_func, invalid_position):
        dataset, documents = add_documents_func
        document_ids = [document.id for document in documents]
        ids = list(document_ids)
        index = {"first": 0, "middle": len(ids) // 2, "last": len(ids)}[invalid_position]
        ids.insert(index, "invalid_id")
        before = {row["id"]: row["ingestion_status"] for row in _document_rows(dataset, document_ids)}

        with pytest.raises(Exception, match="documents not found:"):
            dataset.async_parse_documents(document_ids=ids)

        after = {row["id"]: row["ingestion_status"] for row in _document_rows(dataset, document_ids)}
        assert {doc_id: after[doc_id] for doc_id in document_ids} == {doc_id: before[doc_id] for doc_id in document_ids}

    def test_repeated_parse_reprocesses_completed_documents(self, add_documents_func, integration_settings):
        dataset, documents = add_documents_func
        document_ids = [document.id for document in documents]

        dataset.async_parse_documents(document_ids=document_ids)
        first_rows = _wait_for_terminal_statuses(dataset, document_ids, integration_settings["parse_timeout"])
        _assert_completed(first_rows, document_ids)
        first_event_ids = {doc_id: row["latest_ingestion_event"]["id"] for doc_id, row in first_rows.items()}

        dataset.async_parse_documents(document_ids=document_ids)
        second_rows = _wait_for_terminal_statuses(
            dataset,
            document_ids,
            integration_settings["parse_timeout"],
            previous_event_ids=first_event_ids,
        )
        _assert_completed(second_rows, document_ids)

    def test_duplicate_ids_are_parsed_once(self, add_documents_func, integration_settings):
        dataset, documents = add_documents_func
        document_ids = [document.id for document in documents]

        dataset.async_parse_documents(document_ids=document_ids + document_ids)
        rows = _wait_for_terminal_statuses(dataset, document_ids, integration_settings["parse_timeout"])
        _assert_completed(rows, document_ids)

    @pytest.mark.p3
    def test_parse_100_files(self, add_dataset_func, upload_documents, integration_settings):
        dataset = add_dataset_func
        documents = upload_documents(dataset, 100)
        document_ids = [document.id for document in documents]

        dataset.async_parse_documents(document_ids=document_ids)
        rows = _wait_for_terminal_statuses(dataset, document_ids, integration_settings["parse_timeout"])
        _assert_completed(rows, document_ids)

    @pytest.mark.p3
    def test_concurrent_parse(self, add_dataset_func, upload_documents, integration_settings):
        dataset = add_dataset_func
        documents = upload_documents(dataset, 100)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(dataset.async_parse_documents, [document.id]) for document in documents]
            for future in as_completed(futures):
                assert future.result() is None

        document_ids = [document.id for document in documents]
        rows = _wait_for_terminal_statuses(dataset, document_ids, integration_settings["parse_timeout"])
        _assert_completed(rows, document_ids)


def test_get_documents_status_handles_terminal_and_progress_paths(monkeypatch):
    dataset = DataSet(None, {"id": "dataset-1"})
    states = {
        "doc-done": ("DONE", 0.0),
        "doc-fail": ("FAIL", -1.0),
        "doc-cancel": ("CANCEL", 0.3),
        "doc-progress": ("RUNNING", 1.0),
    }
    calls = []

    def _list_documents(id=None, **_kwargs):
        calls.append(id)
        run, progress = states[id]
        return [Document(None, {"id": id, "run": run, "progress": progress, "chunk_count": 2, "token_count": 4})]

    monkeypatch.setattr(dataset, "list_documents", _list_documents)
    monkeypatch.setattr("time.sleep", lambda *_args: pytest.fail("terminal documents must not be polled again"))

    finished = dataset._get_documents_status(list(states))
    assert sorted(finished) == sorted((doc_id, "DONE" if run == "RUNNING" else run, 2, 4) for doc_id, (run, _) in states.items())
    assert sorted(calls) == sorted(states)


def test_get_documents_status_propagates_lookup_error(monkeypatch):
    dataset = DataSet(None, {"id": "dataset-1"})
    error = RuntimeError("temporary list failure")
    calls = []

    def _list_documents(id=None, **_kwargs):
        calls.append(id)
        raise error

    monkeypatch.setattr(dataset, "list_documents", _list_documents)
    monkeypatch.setattr("time.sleep", lambda *_args: pytest.fail("lookup failures must not be retried"))

    with pytest.raises(RuntimeError, match="temporary list failure") as exc_info:
        dataset._get_documents_status(["doc-1"])
    assert exc_info.value is error
    assert calls == ["doc-1"]


def test_get_documents_status_raises_for_missing_document(monkeypatch):
    dataset = DataSet(None, {"id": "dataset-1"})
    calls = []

    def _list_documents(id=None, **_kwargs):
        calls.append(id)
        return []

    monkeypatch.setattr(dataset, "list_documents", _list_documents)
    monkeypatch.setattr("time.sleep", lambda *_args: pytest.fail("missing documents must not be retried"))

    with pytest.raises(RuntimeError, match="Document doc-1 not found"):
        dataset._get_documents_status(["doc-1"])
    assert calls == ["doc-1"]


def test_parse_documents_keyboard_interrupt_triggers_cancel_then_returns_status(monkeypatch):
    dataset = DataSet(None, {"id": "dataset-1"})
    state = {"cancel_calls": 0, "status_calls": 0}
    expected_status = [("doc-1", "CANCEL", 1, 2)]

    def _raise_keyboard_interrupt(_document_ids):
        raise KeyboardInterrupt

    def _cancel(document_ids):
        state["cancel_calls"] += 1
        assert document_ids == ["doc-1"]

    def _status(document_ids):
        state["status_calls"] += 1
        assert document_ids == ["doc-1"]
        return expected_status

    monkeypatch.setattr(dataset, "async_parse_documents", _raise_keyboard_interrupt)
    monkeypatch.setattr(dataset, "async_cancel_parse_documents", _cancel)
    monkeypatch.setattr(dataset, "_get_documents_status", _status)

    assert dataset.parse_documents(["doc-1"]) == expected_status
    assert state == {"cancel_calls": 1, "status_calls": 1}


def test_parse_documents_returns_first_completed_status(monkeypatch):
    dataset = DataSet(None, {"id": "dataset-1"})
    state = {"status_calls": 0}

    def _noop_parse(_document_ids):
        return None

    def _status(document_ids):
        state["status_calls"] += 1
        assert document_ids == ["doc-1"]
        return [("doc-1", f"DONE-{state['status_calls']}", 1, 2)]

    monkeypatch.setattr(dataset, "async_parse_documents", _noop_parse)
    monkeypatch.setattr(dataset, "_get_documents_status", _status)

    assert dataset.parse_documents(["doc-1"]) == [("doc-1", "DONE-1", 1, 2)]
    assert state["status_calls"] == 1


def test_async_parse_documents_sends_document_ids(monkeypatch):
    dataset = DataSet(None, {"id": "dataset-1"})
    calls = []

    class _Response:
        @staticmethod
        def json():
            return {"code": 0}

    def _post(path, body):
        calls.append((path, body))
        return _Response()

    monkeypatch.setattr(dataset, "post", _post)
    assert dataset.async_parse_documents(["doc-1", "doc-2"]) is None
    assert calls == [("/datasets/dataset-1/chunks", {"document_ids": ["doc-1", "doc-2"]})]


def test_terminal_status_from_previous_parse_run_is_not_reused(monkeypatch):
    dataset = DataSet(None, {"id": "dataset-1"})
    row = {
        "id": "doc-1",
        "ingestion_status": "COMPLETED",
        "latest_ingestion_event": {"id": 42},
    }
    calls = []

    class _Response:
        @staticmethod
        def json():
            return {"code": 0, "data": {"docs": [row]}}

    class _Rag:
        @staticmethod
        def get(_path, params):
            calls.append(params)
            return _Response()

    dataset.rag = _Rag()

    with pytest.raises(AssertionError, match="did not reach a terminal state"):
        _wait_for_terminal_statuses(
            dataset,
            ["doc-1"],
            timeout=0.02,
            previous_event_ids={"doc-1": 42},
        )
    assert calls
    assert all(call == {"ids": ["doc-1"], "page": 1, "page_size": 100} for call in calls)


@pytest.mark.parametrize(
    "ingestion_status, progress, expected_status",
    [
        ("COMPLETED", 1.0, "DONE"),
        ("FAILED", -1.0, "FAIL"),
        ("STOPPED", 0.0, "CANCEL"),
    ],
)
def test_parse_documents_returns_documented_terminal_status(monkeypatch, ingestion_status, progress, expected_status):
    dataset = DataSet(None, {"id": "dataset-1"})

    def _list_documents(**_kwargs):
        return [
            Document(
                None,
                {
                    "id": "doc-1",
                    "ingestion_status": ingestion_status,
                    "progress": progress,
                    "chunk_count": 2,
                    "token_count": 4,
                },
            )
        ]

    monkeypatch.setattr(dataset, "list_documents", _list_documents)
    monkeypatch.setattr("time.sleep", lambda *_args: pytest.fail("terminal documents must not be polled again"))
    assert dataset._get_documents_status(["doc-1"]) == [("doc-1", expected_status, 2, 4)]

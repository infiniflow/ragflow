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
from concurrent.futures import ThreadPoolExecutor, as_completed
import pytest
from common import bulk_upload_documents
from ragflow_sdk import DataSet
from ragflow_sdk.modules.document import Document
from utils import wait_for


@wait_for(30, 1, "Document parsing timeout")
def condition(_dataset: DataSet, _document_ids: list[str] = None):
    documents = _dataset.list_documents(page_size=1000)

    if _document_ids is None:
        for document in documents:
            if document.run != "DONE":
                return False
        return True

    target_ids = set(_document_ids)
    for document in documents:
        if document.id in target_ids:
            if document.run != "DONE":
                return False
    return True


def validate_document_details(dataset, document_ids):
    documents = dataset.list_documents(page_size=1000)
    for document in documents:
        if document.id in document_ids:
            assert document.run == "DONE"
            assert len(document.process_begin_at) > 0
            assert document.process_duration > 0
            assert document.progress > 0
            assert "Task done" in document.progress_msg


class TestDocumentsParse:
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            pytest.param(None, "AttributeError", marks=pytest.mark.skip),
            pytest.param({"document_ids": []}, "`document_ids` is required", marks=pytest.mark.p1),
            pytest.param({"document_ids": ["invalid_id"]}, "Documents not found: ['invalid_id']", marks=pytest.mark.p3),
            pytest.param({"document_ids": ["\n!?。；！？\"'"]}, "Documents not found: ['\\n!?。；！？\"\\'']", marks=pytest.mark.p3),
            pytest.param("not json", "AttributeError", marks=pytest.mark.skip),
            pytest.param(lambda r: {"document_ids": r[:1]}, "", marks=pytest.mark.p1),
            pytest.param(lambda r: {"document_ids": r}, "", marks=pytest.mark.p1),
        ],
    )
    def test_basic_scenarios(self, add_documents_func, payload, expected_message):
        dataset, documents = add_documents_func
        if callable(payload):
            payload = payload([doc.id for doc in documents])

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                dataset.async_parse_documents(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            dataset.async_parse_documents(**payload)
            condition(dataset, payload["document_ids"])
            validate_document_details(dataset, payload["document_ids"])

    @pytest.mark.parametrize(
        "payload",
        [
            pytest.param(lambda r: {"document_ids": ["invalid_id"] + r}, marks=pytest.mark.p3),
            pytest.param(lambda r: {"document_ids": r[:1] + ["invalid_id"] + r[1:3]}, marks=pytest.mark.p1),
            pytest.param(lambda r: {"document_ids": r + ["invalid_id"]}, marks=pytest.mark.p3),
        ],
    )
    def test_parse_partial_invalid_document_id(self, add_documents_func, payload):
        dataset, documents = add_documents_func
        document_ids = [doc.id for doc in documents]
        payload = payload(document_ids)

        with pytest.raises(Exception) as exception_info:
            dataset.async_parse_documents(**payload)
        assert "Documents not found: ['invalid_id']" in str(exception_info.value), str(exception_info.value)

        condition(dataset, document_ids)
        validate_document_details(dataset, document_ids)

    @pytest.mark.p3
    def test_repeated_parse(self, add_documents_func):
        dataset, documents = add_documents_func
        document_ids = [doc.id for doc in documents]
        dataset.async_parse_documents(document_ids=document_ids)
        condition(dataset, document_ids)
        dataset.async_parse_documents(document_ids=document_ids)

    @pytest.mark.p3
    def test_duplicate_parse(self, add_documents_func):
        dataset, documents = add_documents_func
        document_ids = [doc.id for doc in documents]
        dataset.async_parse_documents(document_ids=document_ids + document_ids)
        condition(dataset, document_ids)
        validate_document_details(dataset, document_ids)


@pytest.mark.p2
def test_get_documents_status_handles_retry_terminal_and_progress_paths(add_dataset_func, monkeypatch):
    dataset = add_dataset_func
    call_counts = {"doc-retry": 0, "doc-progress": 0, "doc-exception": 0}

    def _doc(doc_id, run, chunk_count, token_count, progress):
        return Document(
            dataset.rag,
            {
                "id": doc_id,
                "dataset_id": dataset.id,
                "run": run,
                "chunk_count": chunk_count,
                "token_count": token_count,
                "progress": progress,
            },
        )

    def _list_documents(id=None, **_kwargs):
        if id == "doc-retry":
            call_counts["doc-retry"] += 1
            if call_counts["doc-retry"] == 1:
                return []
            return [_doc("doc-retry", "DONE", 3, 5, 0.0)]
        if id == "doc-progress":
            call_counts["doc-progress"] += 1
            return [_doc("doc-progress", "RUNNING", 2, 4, 1.0)]
        if id == "doc-exception":
            call_counts["doc-exception"] += 1
            if call_counts["doc-exception"] == 1:
                raise Exception("temporary list failure")
            return [_doc("doc-exception", "DONE", 7, 11, 0.0)]
        return []

    monkeypatch.setattr(dataset, "list_documents", _list_documents)
    monkeypatch.setattr("time.sleep", lambda *_args, **_kwargs: None)

    finished = dataset._get_documents_status(["doc-retry", "doc-progress", "doc-exception"])
    assert {item[0] for item in finished} == {"doc-retry", "doc-progress", "doc-exception"}
    finished_map = {item[0]: item for item in finished}
    assert finished_map["doc-retry"][1] == "DONE"
    assert finished_map["doc-progress"][1] == "DONE"
    assert finished_map["doc-exception"][1] == "DONE"


@pytest.mark.p2
def test_parse_documents_keyboard_interrupt_triggers_cancel_then_returns_status(add_dataset_func, monkeypatch):
    dataset = add_dataset_func
    state = {"cancel_calls": 0, "status_calls": 0}
    expected_status = [("doc-1", "DONE", 1, 2)]

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

    status = dataset.parse_documents(["doc-1"])
    assert status == expected_status
    assert state["cancel_calls"] == 1
    assert state["status_calls"] == 1


@pytest.mark.p2
def test_parse_documents_happy_path_runs_initial_wait_then_returns_status(add_dataset_func, monkeypatch):
    dataset = add_dataset_func
    state = {"status_calls": 0}

    def _noop_parse(_document_ids):
        return None

    def _status(document_ids):
        state["status_calls"] += 1
        assert document_ids == ["doc-1"]
        return [("doc-1", f"DONE-{state['status_calls']}", 1, 2)]

    monkeypatch.setattr(dataset, "async_parse_documents", _noop_parse)
    monkeypatch.setattr(dataset, "_get_documents_status", _status)

    status = dataset.parse_documents(["doc-1"])
    assert state["status_calls"] == 2
    assert status == [("doc-1", "DONE-2", 1, 2)]


@pytest.mark.p2
def test_async_cancel_parse_documents_raises_on_nonzero_code(add_dataset_func, monkeypatch):
    dataset = add_dataset_func

    class _Resp:
        @staticmethod
        def json():
            return {"code": 102, "message": "cancel failed"}

    monkeypatch.setattr(dataset, "rm", lambda *_args, **_kwargs: _Resp())

    with pytest.raises(Exception) as exc_info:
        dataset.async_cancel_parse_documents(["doc-1"])
    assert "cancel failed" in str(exc_info.value), str(exc_info.value)


@pytest.mark.p3
def test_parse_100_files(add_dataset_func, tmp_path):
    @wait_for(200, 1, "Document parsing timeout")
    def condition_inner(_dataset: DataSet, _count: int):
        docs = _dataset.list_documents(page_size=_count * 2)
        for document in docs:
            if document.run != "DONE":
                return False
        return True

    count = 100
    dataset = add_dataset_func
    documents = bulk_upload_documents(dataset, count, tmp_path)
    document_ids = [doc.id for doc in documents]

    dataset.async_parse_documents(document_ids=document_ids)
    condition_inner(dataset, count)
    validate_document_details(dataset, document_ids)


@pytest.mark.p3
def test_concurrent_parse(add_dataset_func, tmp_path):
    @wait_for(200, 1, "Document parsing timeout")
    def condition_inner(_dataset: DataSet, _count: int):
        docs = _dataset.list_documents(page_size=_count * 2)
        for document in docs:
            if document.run != "DONE":
                return False
        return True

    count = 100
    dataset = add_dataset_func
    documents = bulk_upload_documents(dataset, count, tmp_path)
    document_ids = [doc.id for doc in documents]

    def parse_doc(doc_id):
        dataset.async_parse_documents(document_ids=[doc_id])

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [executor.submit(parse_doc, doc.id) for doc in documents]

    responses = list(as_completed(futures))
    assert len(responses) == count, responses

    condition_inner(dataset, count)
    validate_document_details(dataset, document_ids)

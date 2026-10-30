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

import pytest
from ragflow_sdk.modules.dataset import DataSet


def test_async_cancel_parse_documents_sends_document_ids(monkeypatch):
    dataset = DataSet(None, {"id": "dataset-1"})
    calls = []

    class _Response:
        @staticmethod
        def json():
            return {"code": 0}

    def _rm(path, body):
        calls.append((path, body))
        return _Response()

    monkeypatch.setattr(dataset, "rm", _rm)
    assert dataset.async_cancel_parse_documents(["doc-1", "doc-2"]) is None
    assert calls == [("/datasets/dataset-1/chunks", {"document_ids": ["doc-1", "doc-2"]})]


def test_async_cancel_parse_documents_surfaces_api_error(monkeypatch):
    dataset = DataSet(None, {"id": "dataset-1"})

    class _Response:
        @staticmethod
        def json():
            return {"code": 102, "message": "cancel failed"}

    monkeypatch.setattr(dataset, "rm", lambda *_args, **_kwargs: _Response())
    with pytest.raises(Exception, match="cancel failed"):
        dataset.async_cancel_parse_documents(["doc-1"])


@pytest.mark.integration
def test_async_cancel_stops_an_active_parse(add_dataset_func, document_factory, integration_settings):
    dataset = add_dataset_func
    document = document_factory(
        dataset,
        blob=(b"RAGFlow SDK cancellation test. " * 131072),
    )
    dataset.async_parse_documents([document.id])

    def read_status():
        response = dataset.rag.get(
            f"/datasets/{dataset.id}/documents",
            params={"id": document.id, "page": 1, "page_size": 1},
        ).json()
        assert response.get("code") == 0, response.get("message")
        rows = response["data"]["docs"]
        assert len(rows) == 1
        return rows[0]["ingestion_status"]

    def wait_for_status(statuses, *, terminal=None):
        deadline = time.monotonic() + integration_settings["parse_timeout"]
        status = None
        while time.monotonic() < deadline:
            status = read_status()
            if status in statuses:
                return status
            if terminal and status in terminal:
                pytest.fail(f"parse reached {status!r} before expected status {statuses}")
            time.sleep(min(0.5, max(0, deadline - time.monotonic())))
        pytest.fail(f"document did not reach {statuses}; latest status was {status!r}")

    wait_for_status({"CREATED", "SCHEDULED", "RUNNING", "STOPPING"})
    dataset.async_cancel_parse_documents([document.id])
    status = wait_for_status(
        {"STOPPED", "COMPLETED", "FAILED"},
        terminal={"COMPLETED", "FAILED"},
    )
    assert status == "STOPPED"

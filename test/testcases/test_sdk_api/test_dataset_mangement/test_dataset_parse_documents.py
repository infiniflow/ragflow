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
import pytest

from ragflow_sdk.modules.dataset import DataSet
from ragflow_sdk.modules.document import Document


class _DummyRag:
    pass


@pytest.mark.p2
class TestDataSetParseDocuments:
    def test_get_documents_status_terminal_and_progress(self, monkeypatch):
        dataset = DataSet(_DummyRag(), {"id": "ds"})
        docs = {
            "doc_done": Document(_DummyRag(), {"id": "doc_done", "run": "DONE", "progress": 0.5, "chunk_count": 2, "token_count": 3}),
            "doc_progress": Document(_DummyRag(), {"id": "doc_progress", "run": "RUNNING", "progress": 1.0, "chunk_count": 5, "token_count": 8}),
        }

        def _list_documents(id=None, **_kwargs):
            return [docs[id]] if id in docs else []

        monkeypatch.setattr(dataset, "list_documents", _list_documents)
        monkeypatch.setattr("time.sleep", lambda *_args, **_kwargs: None)

        result = dataset._get_documents_status(list(docs.keys()))
        result_map = {doc_id: status for doc_id, status, *_ in result}
        assert result_map["doc_done"] == "DONE"
        assert result_map["doc_progress"] == "DONE"

    def test_parse_documents_keyboard_interrupt_triggers_cancel(self, monkeypatch):
        dataset = DataSet(_DummyRag(), {"id": "ds"})
        called = {"cancel": None}
        sentinel = [("doc1", "DONE", 0, 0)]

        def _async_parse_documents(_ids):
            raise KeyboardInterrupt()

        def _async_cancel_parse_documents(_ids):
            called["cancel"] = _ids

        monkeypatch.setattr(dataset, "async_parse_documents", _async_parse_documents)
        monkeypatch.setattr(dataset, "async_cancel_parse_documents", _async_cancel_parse_documents)
        monkeypatch.setattr(dataset, "_get_documents_status", lambda _ids: sentinel)

        doc_ids = ["doc1"]
        result = dataset.parse_documents(doc_ids)
        assert called["cancel"] == doc_ids
        assert result == sentinel

    def test_async_cancel_parse_documents_error_raises(self, monkeypatch):
        dataset = DataSet(_DummyRag(), {"id": "ds"})

        class _Resp:
            def json(self):
                return {"code": 1, "message": "cancel failed"}

        monkeypatch.setattr(dataset, "rm", lambda *_args, **_kwargs: _Resp())

        with pytest.raises(Exception) as excinfo:
            dataset.async_cancel_parse_documents(["doc1"])
        assert "cancel failed" in str(excinfo.value)

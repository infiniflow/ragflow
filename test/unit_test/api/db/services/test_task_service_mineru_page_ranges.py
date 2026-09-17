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
import pytest

from api.db.services import task_service as ts


class _Storage:
    @staticmethod
    def get(_bucket, _name):
        return b"%PDF-1.4"


def _queued_page_spans(monkeypatch, layout_model_name, pages, total_pages=30):
    """Run queue_tasks on a PDF and return the (from_page, to_page) of each task."""
    captured = []
    parser_config = {"layout_recognize": "a" * 32}
    if pages is not None:
        parser_config["pages"] = pages
    doc = {
        "id": "doc1",
        "name": "manual.pdf",
        "type": ts.FileType.PDF.value,
        "parser_id": "naive",
        "parser_config": parser_config,
    }

    monkeypatch.setattr(ts.settings, "STORAGE_IMPL", _Storage, raising=False)
    monkeypatch.setattr(ts.PdfParser, "total_page_number", staticmethod(lambda *_a, **_k: total_pages))
    monkeypatch.setattr(ts, "get_composite_model_name_by_id", lambda _id: layout_model_name)
    monkeypatch.setattr(ts.DocumentService, "get_chunking_config", staticmethod(lambda _id: {"tenant_id": "t", "kb_id": "k", "parser_config": {}}))
    monkeypatch.setattr(ts.TaskService, "get_tasks", staticmethod(lambda _id: []))
    monkeypatch.setattr(ts.DocumentService, "update_by_id", staticmethod(lambda *_a, **_k: None))
    monkeypatch.setattr(ts.DocumentService, "begin2parse", staticmethod(lambda *_a, **_k: None))
    monkeypatch.setattr(ts, "bulk_insert_into_db", lambda _model, rows, _replace: captured.extend(rows))
    monkeypatch.setattr(ts, "seed_doc_chunking_counter", lambda *_a, **_k: True)
    monkeypatch.setattr(ts.REDIS_CONN, "queue_product", lambda *_a, **_k: True)

    ts.queue_tasks(doc, "bucket", "manual.pdf", 0)
    return [(task["from_page"], task["to_page"]) for task in captured]


@pytest.mark.p2
class TestQueueTasksMinerUPageRanges:
    def test_four_ranges_become_one_task(self, monkeypatch):
        # Every task uploads the whole PDF to the MinerU API server, so four
        # configured ranges must still produce a single request.
        assert _queued_page_spans(monkeypatch, "vlm@MinerU", [(1, 5), (8, 12), (15, 20), (25, 30)]) == [(0, 29)]

    def test_two_ranges_span_the_gap(self, monkeypatch):
        assert _queued_page_spans(monkeypatch, "vlm@MinerU", [(1, 10), (20, 30)]) == [(0, 29)]

    def test_one_range_is_unchanged(self, monkeypatch):
        assert _queued_page_spans(monkeypatch, "vlm@MinerU", [(1, 10)]) == [(0, 9)]

    def test_no_range_is_unchanged(self, monkeypatch):
        assert _queued_page_spans(monkeypatch, "vlm@MinerU", None) == [(0, 30)]

    def test_other_parsers_keep_one_task_per_range(self, monkeypatch):
        # The collapse is MinerU-only. DeepDOC reads the file once per task, so
        # it keeps a task per range and splits each range by task_page_size.
        assert _queued_page_spans(monkeypatch, "vlm@DeepDOC", [(1, 10), (20, 30)]) == [(0, 9), (19, 29)]

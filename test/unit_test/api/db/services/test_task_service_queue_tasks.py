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
from types import SimpleNamespace

import pytest

from api.db import FileType
from api.db.services import task_service


def _queue_tasks(monkeypatch, parser_id, parser_config):
    queued_tasks = []
    monkeypatch.setattr(task_service.settings, "STORAGE_IMPL", SimpleNamespace(get=lambda *_args: b"pdf"))
    monkeypatch.setattr(task_service.PdfParser, "total_page_number", lambda *_args: 20)
    monkeypatch.setattr(task_service.DocumentService, "get_chunking_config", lambda *_args: {})
    monkeypatch.setattr(task_service.TaskService, "get_tasks", lambda *_args: [])
    monkeypatch.setattr(task_service.DocumentService, "update_by_id", lambda *_args: None)
    monkeypatch.setattr(task_service, "bulk_insert_into_db", lambda _model, tasks, _replace: queued_tasks.extend(tasks))
    monkeypatch.setattr(task_service.DocumentService, "begin2parse", lambda *_args: None)
    monkeypatch.setattr(task_service, "seed_doc_chunking_counter", lambda *_args: True)
    monkeypatch.setattr(task_service.REDIS_CONN, "queue_product", lambda *_args, **_kwargs: True)

    task_service.queue_tasks(
        {
            "id": "doc-id",
            "name": "resume.pdf",
            "type": FileType.PDF.value,
            "parser_id": parser_id,
            "parser_config": parser_config,
        },
        "bucket",
        "resume.pdf",
        0,
    )
    return queued_tasks


@pytest.mark.p2
def test_queue_tasks_collapses_multiple_page_ranges_for_resume(monkeypatch):
    queued_tasks = _queue_tasks(monkeypatch, "resume", {"pages": [[1, 5], [10, 15]]})

    assert len(queued_tasks) == 1
    assert queued_tasks[0]["from_page"] == 0
    assert queued_tasks[0]["to_page"] == 20


@pytest.mark.p2
def test_queue_tasks_keeps_multiple_page_ranges_for_naive(monkeypatch):
    queued_tasks = _queue_tasks(monkeypatch, "naive", {"pages": [[1, 5], [10, 15]], "task_page_size": 12})

    assert len(queued_tasks) > 1

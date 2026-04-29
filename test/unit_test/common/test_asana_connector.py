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
from datetime import datetime, timezone

import pytest

from common import settings as _settings  # noqa: F401
import common.data_source.asana_connector as asana_connector
from common.data_source.asana_connector import AsanaConnector, AsanaTask


def _task(task_id: str, last_modified: datetime | None = None) -> AsanaTask:
    return AsanaTask(
        id=task_id,
        title=f"Task {task_id}",
        text="",
        link=f"https://app.asana.com/0/project/{task_id}",
        last_modified=last_modified or datetime(2026, 1, 1, tzinfo=timezone.utc),
        project_gid="project-1",
        project_name="Project",
    )


class _FakeAsanaClient:
    def __init__(
        self,
        attachments_by_task: dict[str, list[dict]],
        fail_on_task: str | None = None,
        task_modified_at: dict[str, datetime] | None = None,
    ) -> None:
        self.api_error_count = 0
        self.attachments_by_task = attachments_by_task
        self.fail_on_task = fail_on_task
        self.task_modified_at = task_modified_at or {}
        self.project_ids = None
        self.start_time = None

    def get_tasks(self, project_ids, start_time):
        self.project_ids = project_ids
        self.start_time = start_time
        for task_id in self.attachments_by_task:
            yield _task(task_id, self.task_modified_at.get(task_id))

    def get_attachments(self, task_id: str) -> list[dict]:
        if task_id == self.fail_on_task:
            self.api_error_count += 1
            return []
        return self.attachments_by_task[task_id]


def test_retrieve_all_slim_docs_perm_sync_batches_matching_attachment_ids(monkeypatch):
    connector = AsanaConnector("workspace-1", "project-1", batch_size=2)
    fake_client = _FakeAsanaClient(
        {
            "task-1": [{"gid": "att-1"}, {"gid": "att-2"}],
            "task-2": [{"gid": "att-3"}],
        }
    )
    connector.asana_client = fake_client

    def _unexpected_download(*_args, **_kwargs):
        raise AssertionError("slim document snapshot must not download attachment blobs")

    monkeypatch.setattr(asana_connector.requests, "get", _unexpected_download)

    batches = list(connector.retrieve_all_slim_docs_perm_sync())

    assert [[doc.id for doc in batch] for batch in batches] == [
        ["asana:task-1:att-1", "asana:task-1:att-2"],
        ["asana:task-2:att-3"],
    ]
    assert fake_client.project_ids == ["project-1"]
    assert fake_client.start_time == datetime.fromtimestamp(0).isoformat()


def test_retrieve_all_slim_docs_perm_sync_aborts_on_snapshot_api_errors():
    connector = AsanaConnector("workspace-1", "project-1")
    connector.asana_client = _FakeAsanaClient(
        {
            "task-1": [{"gid": "att-1"}],
            "task-2": [{"gid": "att-2"}],
        },
        fail_on_task="task-2",
    )

    with pytest.raises(RuntimeError, match="Asana slim document snapshot failed"):
        list(connector.retrieve_all_slim_docs_perm_sync())


class _FakeResponse:
    content = b"attachment"

    def raise_for_status(self):
        return None


def test_poll_source_respects_end_boundary(monkeypatch):
    connector = AsanaConnector("workspace-1", "project-1", batch_size=1)
    connector.workspace_users_email = {"owner@example.com"}
    end_time = datetime(2026, 1, 2, tzinfo=timezone.utc)
    connector.asana_client = _FakeAsanaClient(
        {
            "task-1": [{"gid": "att-1", "download_url": "https://example.test/att-1.pdf", "name": "att-1.pdf", "size": 10}],
            "task-2": [{"gid": "att-2", "download_url": "https://example.test/att-2.pdf", "name": "att-2.pdf", "size": 10}],
        },
        task_modified_at={
            "task-1": datetime(2026, 1, 1, 12, tzinfo=timezone.utc),
            "task-2": end_time,
        },
    )
    monkeypatch.setattr(asana_connector.requests, "get", lambda *_args, **_kwargs: _FakeResponse())

    batches = list(
        connector.poll_source(
            datetime(2026, 1, 1, tzinfo=timezone.utc).timestamp(),
            end_time.timestamp(),
        )
    )

    assert [[doc.id for doc in batch] for batch in batches] == [["asana:task-1:att-1"]]

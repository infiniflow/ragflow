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
"""Sync task list orders SCHEDULE first, RUNNING second, then the rest."""

from datetime import UTC, datetime

from peewee import SqliteDatabase

from api.db.db_models import DB, Connector, Connector2Kb, Knowledgebase, SyncLogs
from api.db.services.connector_service import SyncLogsService
from common.constants import TaskStatus

_MODELS = [SyncLogs, Connector, Connector2Kb, Knowledgebase]


def test_list_sync_tasks_status_priority():
    sqlite_db = SqliteDatabase(":memory:")
    sqlite_db.bind(_MODELS, bind_refs=False, bind_backrefs=False)
    sqlite_db.connect()
    sqlite_db.create_tables(_MODELS)
    try:
        Connector.create(
            id="conn-1",
            tenant_id="tenant-1",
            name="conn-1",
            source="rss",
            input_type="poll",
            status=TaskStatus.SCHEDULE,
            refresh_freq=5,
            prune_freq=5,
            timeout_secs=60,
        )
        Knowledgebase.create(id="kb-1", tenant_id="tenant-1", name="kb-1", embd_id="", created_by="tenant-1")
        Connector2Kb.create(id="c2k-1", connector_id="conn-1", kb_id="kb-1", auto_parse="1")

        rows = [
            ("task-done-1", TaskStatus.DONE, 100),
            ("task-done-2", TaskStatus.DONE, 200),
            ("task-running-1", TaskStatus.RUNNING, 300),
            ("task-schedule-1", TaskStatus.SCHEDULE, 400),
            ("task-fail-1", TaskStatus.FAIL, 500),
            ("task-schedule-2", TaskStatus.SCHEDULE, 600),
        ]
        for task_id, status, ts in rows:
            SyncLogs.create(id=task_id, connector_id="conn-1", kb_id="kb-1", status=status)
            sqlite_db.execute_sql(
                "UPDATE sync_logs SET update_time = ?, update_date = ? WHERE id = ?",
                (ts, datetime.fromtimestamp(ts / 1000, tz=UTC), task_id),
            )

        logs, total = SyncLogsService.list_sync_tasks("conn-1", 1, 100)
        assert total == len(rows)
        assert [log["id"] for log in logs] == [
            "task-schedule-2",
            "task-schedule-1",
            "task-running-1",
            "task-fail-1",
            "task-done-2",
            "task-done-1",
        ]
        assert all("status_rank" not in log for log in logs)
    finally:
        sqlite_db.drop_tables(_MODELS)
        sqlite_db.close()
        DB.bind(_MODELS, bind_refs=False, bind_backrefs=False)

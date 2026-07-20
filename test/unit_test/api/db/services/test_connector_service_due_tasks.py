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

from api.db.db_models import Connector, SyncLogs
from api.db.services.connector_service import SyncLogsService
from common.constants import ConnectorTaskType


@pytest.mark.p2
def test_list_due_sync_tasks_excludes_non_positive_refresh_frequencies_unless_reindexing(monkeypatch):
    tasks = [
        {"id": "negative", "refresh_freq": -1, "reindex": "0"},
        {"id": "zero", "refresh_freq": 0, "reindex": "0"},
        {"id": "unset", "refresh_freq": None, "reindex": "0"},
        {"id": "positive", "refresh_freq": 5, "reindex": "0"},
        {"id": "reindex", "refresh_freq": 0, "reindex": "1"},
    ]
    calls = []

    def _list_due_tasks(cls, task_type, freq_field):
        calls.append((task_type, freq_field))
        return tasks

    monkeypatch.setattr(SyncLogsService, "_list_due_tasks_for_freq", classmethod(_list_due_tasks))

    due_tasks = SyncLogsService.list_due_sync_tasks()

    assert due_tasks == [
        {"id": "positive", "refresh_freq": 5, "reindex": "0"},
        {"id": "reindex", "refresh_freq": 0, "reindex": "1"},
    ]
    assert calls == [(ConnectorTaskType.SYNC, "refresh_freq")]


@pytest.mark.p2
def test_due_sync_query_filters_disabled_connectors_before_materialization(monkeypatch):
    where_calls = []

    class Query:
        def join(self, *_args, **_kwargs):
            return self

        def where(self, *conditions):
            where_calls.append(conditions)
            return self

        def distinct(self):
            return self

        def order_by(self, *_args):
            return self

        def dicts(self):
            return []

    monkeypatch.setattr(SyncLogs, "select", lambda *_fields: Query())

    assert SyncLogsService._list_due_tasks_for_freq(ConnectorTaskType.SYNC, "refresh_freq") == []

    frequency_filter = next(condition for conditions in where_calls for condition in conditions if getattr(condition, "op", None) == "OR")
    assert (frequency_filter.lhs.lhs, frequency_filter.lhs.op, frequency_filter.lhs.rhs) == (Connector.refresh_freq, ">", 0)
    assert (frequency_filter.rhs.lhs, frequency_filter.rhs.op, frequency_filter.rhs.rhs) == (SyncLogs.from_beginning, "=", "1")

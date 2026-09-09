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
"""
``API4ConversationService.append_message`` must persist the conversation
snapshot and increment ``round`` / ``tokens`` / ``duration`` in a single
UPDATE, using server-side increments rather than the (possibly stale)
values carried by the snapshot.
"""

import pytest
from peewee import SqliteDatabase

from api.db.db_models import API4Conversation
from api.db.services.api_service import API4ConversationService

# Bypass @DB.connection_context() (bound to the real pooled database) and
# exercise the service body against an in-memory SQLite table.
_append_message = API4ConversationService.append_message.__wrapped__


@pytest.fixture
def sqlite_conv():
    db = SqliteDatabase(":memory:")
    with db.bind_ctx([API4Conversation]):
        db.create_tables([API4Conversation])
        API4Conversation.create(
            id="conv-1",
            dialog_id="dialog-1",
            user_id="user-1",
            message=[{"role": "assistant", "content": "Hi"}],
            reference=[],
            round=2,
            tokens=100,
            duration=50.0,
        )
        yield API4Conversation.get_by_id("conv-1")
    db.close()


@pytest.mark.p2
def test_append_message_persists_snapshot_and_increments_counters(sqlite_conv):
    executed = []
    original_execute_sql = API4Conversation._meta.database.execute_sql

    def spy(sql, *args, **kwargs):
        executed.append(sql)
        return original_execute_sql(sql, *args, **kwargs)

    API4Conversation._meta.database.execute_sql = spy

    sqlite_conv.message.append({"role": "user", "content": "Hello?"})
    sqlite_conv.message.append({"role": "assistant", "content": "Hello!"})
    snapshot = dict(sqlite_conv.to_dict())
    # Simulate a stale snapshot: counters read before another request's increment.
    snapshot["round"] = 0
    snapshot["tokens"] = 0
    snapshot["duration"] = 0.0

    assert _append_message(API4ConversationService, "conv-1", snapshot, tokens=42, duration=12.5) == 1

    assert len([sql for sql in executed if sql.lstrip().upper().startswith("UPDATE")]) == 1

    row = API4Conversation.get_by_id("conv-1")
    assert [m["content"] for m in row.message] == ["Hi", "Hello?", "Hello!"]
    assert row.round == 3
    assert row.tokens == 142
    assert row.duration == 62.5
    assert row.update_time is not None


@pytest.mark.p2
def test_append_message_defaults_leave_tokens_and_duration_unchanged(sqlite_conv):
    _append_message(API4ConversationService, "conv-1", dict(sqlite_conv.to_dict()))

    row = API4Conversation.get_by_id("conv-1")
    assert row.round == 3
    assert row.tokens == 100
    assert row.duration == 50.0

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

"""Exercise maintenance through real connectors and pandas, with local SQL I/O."""

import json
import logging
import sqlite3
from types import SimpleNamespace
from unittest.mock import MagicMock

import pandas as pd
import pytest
from infinity.common import SortType

import common.settings as settings  # Initialize the connector circular imports first.
from memory.services.messages import MessageService
from memory.utils import infinity_conn

pytestmark = pytest.mark.p2


class MemoryTable:
    """Use SQLite to enforce scalar predicates, ordering and projected columns.

    This replaces the remote client, not the connector's result conversion.
    It does not emulate Infinity's vector or full-text search.
    """

    schema = {
        "id": ("Varchar", ""),
        "memory_id": ("Varchar", ""),
        "message_id": ("Integer", 0),
        "content": ("Varchar", ""),
        "q_2_vec": ("Embedding(float,2)", None),
        "forget_at_flt": ("Float", 0.0),
        "valid_at_flt": ("Float", 0.0),
        "status_int": ("Integer", 1),
    }

    def __init__(self):
        """Create an isolated SQL table with the memory fields under test."""
        self.db = sqlite3.connect(":memory:")
        self.db.execute(
            "CREATE TABLE messages (id TEXT, memory_id TEXT, message_id INTEGER, content TEXT, q_2_vec TEXT, forget_at_flt REAL DEFAULT 0, valid_at_flt REAL DEFAULT 0, status_int INTEGER DEFAULT 1)"
        )

    def show_columns(self):
        """Expose schema types and defaults in the Infinity client's format."""
        return SimpleNamespace(rows=lambda: [(name, ty, default, "") for name, (ty, default) in self.schema.items()])

    def output(self, fields):
        """Begin a query with the requested projection and reset query state."""
        self.fields = fields
        self.predicate = "1=1"
        self.order = []
        self.start = 0
        self.count = 512
        return self

    def filter(self, predicate):
        """Set the scalar predicate that SQLite will evaluate."""
        self.predicate = predicate
        return self

    def sort(self, order):
        """Set the field and direction pairs for result ordering."""
        self.order = order
        return self

    def offset(self, start):
        """Set the number of matching rows to skip."""
        self.start = start
        return self

    def limit(self, count):
        """Bound the number of rows returned by the query."""
        self.count = count
        return self

    def option(self, _options):
        """Accept client options; this fixture always returns hit metadata."""
        return self

    def to_df(self):
        """Execute the scalar query and return a DataFrame plus hit metadata."""
        order = ", ".join(f"{field} {'ASC' if direction == SortType.Asc else 'DESC'}" for field, direction in self.order)
        sql = f"SELECT {', '.join(self.fields)} FROM messages WHERE {self.predicate}"
        if order:
            sql += f" ORDER BY {order}"
        cursor = self.db.execute(sql + " LIMIT ? OFFSET ?", (self.count, self.start))
        frame = pd.DataFrame(cursor.fetchall(), columns=[column[0] for column in cursor.description])
        if "q_2_vec" in frame:
            frame["q_2_vec"] = frame["q_2_vec"].apply(json.loads)
        return frame, {"total_hits_count": len(frame)}

    def insert(self, documents):
        """Insert messages while serializing embedding arrays for SQLite."""
        for document in documents:
            values = [json.dumps(value) if key == "q_2_vec" else value for key, value in document.items()]
            self.db.execute(f"INSERT INTO messages ({', '.join(document)}) VALUES ({', '.join('?' for _ in document)})", values)

    def delete(self, predicate):
        """Delete matching rows and expose the client-style deletion count."""
        cursor = self.db.execute(f"DELETE FROM messages WHERE {predicate}")
        return SimpleNamespace(deleted_rows=cursor.rowcount)


def message(number, forgotten=0.0, memory_id="mem-1"):
    """Build a deterministic message with stable text and embedding sizes."""
    return {
        "id": f"{memory_id}_{number}",
        "memory_id": memory_id,
        "message_id": number,
        "content": "remember this",
        "q_2_vec": [0.1, 0.2],
        "forget_at_flt": forgotten,
        "valid_at_flt": float(number),
    }


@pytest.fixture
def store(monkeypatch):
    """Use production connector methods with a local table and no network I/O."""
    # The singleton decorator replaces the class with a closure. Avoid __init__,
    # which opens network connections, while retaining all production methods.
    cls = next(cell.cell_contents for cell in infinity_conn.InfinityConnection.__closure__ if isinstance(cell.cell_contents, type))
    conn = cls.__new__(cls)
    conn.dbName = "test_memory"
    conn.logger = logging.getLogger(__name__)
    conn.connPool = MagicMock()
    table = MemoryTable()
    conn.connPool.get_conn.return_value.get_database.return_value.get_table.return_value = table
    monkeypatch.setattr(settings, "msgStoreConn", conn)
    yield conn, table
    table.db.close()


@pytest.mark.parametrize("method", ["get_forgotten_messages", "get_missing_field_message"])
@pytest.mark.parametrize("empty", [False, True])
def test_maintenance_results_preserve_ids_fields_order_scope_and_limit(store, method, empty):
    """Verify maintenance projections, filtering and pagination end to end."""
    conn, table = store
    if not empty:
        table.insert([message(3, 20), message(2, 10), message(1), message(99, 1, "other-memory")])
    select_fields = ["message_id", "content", "content_embed"]
    kwargs = {"field_name": "forget_at_flt"} if method == "get_missing_field_message" else {}
    result = getattr(conn, method)(select_fields, "memory_tenant-1", "mem-1", limit=1, **kwargs)
    conn.logger.debug("Maintenance query method=%s empty=%s result=%r", method, empty, result)

    assert isinstance(result, pd.DataFrame)
    docs = conn.get_fields(result, select_fields)
    expected_id = 1 if method == "get_missing_field_message" else 2
    assert docs == ({} if empty else {f"mem-1_{expected_id}": {"message_id": expected_id, "content": "remember this", "content_embed": [0.1, 0.2]}})
    assert select_fields == ["message_id", "content", "content_embed"]
    conn.connPool.release_conn.assert_called_once_with(conn.connPool.get_conn.return_value)


@pytest.mark.parametrize("method", ["get_forgotten_messages", "get_missing_field_message"])
def test_maintenance_releases_connection_when_query_fails(store, monkeypatch, method):
    """Ensure query errors propagate after the pooled connection is released."""
    conn, table = store
    monkeypatch.setattr(table, "to_df", MagicMock(side_effect=RuntimeError("query failed")))
    kwargs = {"field_name": "content"} if method == "get_missing_field_message" else {}
    with pytest.raises(RuntimeError, match="query failed"):
        getattr(conn, method)(["message_id"], "memory_tenant-1", "mem-1", **kwargs)
    conn.connPool.release_conn.assert_called_once_with(conn.connPool.get_conn.return_value)


@pytest.mark.parametrize(
    "rows,needed,expected_ids",
    [
        ([message(3, 20), message(2, 10), message(1)], 1, [2]),
        ([message(3, 20), message(2, 10), message(1)], 3, [2, 3, 1]),
        ([message(2), message(1)], 1, [1]),
        ([], 1, []),
    ],
)
def test_fifo_prefers_forgotten_then_oldest_active_messages(store, rows, needed, expected_ids):
    """Verify eviction priority and byte accounting, including empty results."""
    conn, table = store
    table.insert(rows)
    size = MessageService.calculate_message_size({"content": "remember this", "content_embed": [0.1, 0.2]})
    ids, removed_size = MessageService.pick_messages_to_delete_by_fifo("mem-1", "tenant-1", needed * size)
    conn.logger.debug("FIFO requested_bytes=%s selected_ids=%s removed_bytes=%s", needed * size, ids, removed_size)
    assert ids == expected_ids
    assert removed_size == len(expected_ids) * size


@pytest.mark.parametrize("empty", [False, True])
def test_missing_field_service_accepts_dataframe_results(store, empty):
    """Normalize populated and empty maintenance DataFrames to message lists."""
    conn, table = store
    if not empty:
        table.insert([message(3, 20), message(2), message(1)])
    result = MessageService.get_missing_field_messages("mem-1", "tenant-1", "forget_at_flt")
    conn.logger.debug("Missing-field query empty=%s result=%r", empty, result)
    assert result == ([] if empty else [{"message_id": 1, "content": "remember this"}, {"message_id": 2, "content": "remember this"}])


def test_missing_indexes_still_skip_result_conversion(monkeypatch):
    """Keep absent-index results out of the connector's field converter."""
    conn = MagicMock()
    conn.get_forgotten_messages.return_value = None
    conn.get_missing_field_message.return_value = None
    conn.search.return_value = ({}, 0)
    conn.get_fields.return_value = {}
    monkeypatch.setattr(settings, "msgStoreConn", conn)

    assert MessageService.get_missing_field_messages("mem-1", "tenant-1", "content") == []
    conn.get_fields.assert_not_called()
    assert MessageService.pick_messages_to_delete_by_fifo("mem-1", "tenant-1", 1) == ([], 0)
    conn.get_fields.assert_called_once_with({}, ["message_id", "content", "content_embed"])


async def test_capacity_overflow_evicts_old_messages_and_saves_new_message(store, monkeypatch):
    """Exercise FIFO deletion, embedding persistence and capacity accounting."""
    from api.db.joint_services import memory_message_service

    conn, table = store
    table.insert([message(3, 20), message(2, 10), message(1)])
    one_message_size = MessageService.calculate_message_size({"content": "remember this", "content_embed": [0.1, 0.2]})
    memory = SimpleNamespace(
        id="mem-1",
        tenant_id="tenant-1",
        tenant_embd_id=None,
        embd_id="embedding",
        memory_size=2 * one_message_size,
        forgetting_policy="FIFO",
    )
    bundle = MagicMock()
    bundle.__enter__.return_value.encode.return_value = ([[0.1, 0.2]], 0)
    monkeypatch.setattr(memory_message_service, "LLMBundle", lambda *_args: bundle)
    monkeypatch.setattr(memory_message_service, "resolve_model_config", lambda *_args: {})
    monkeypatch.setattr(memory_message_service, "get_memory_size_cache", lambda *_args: 3 * one_message_size)
    decrease, increase = MagicMock(), MagicMock()
    monkeypatch.setattr(memory_message_service, "decrease_memory_size_cache", decrease)
    monkeypatch.setattr(memory_message_service, "increase_memory_size_cache", increase)
    new_message = {"message_id": 4, "memory_id": "mem-1", "content": "remember this", "status": True}

    conn.logger.debug("Capacity before save cached_bytes=%s limit_bytes=%s", 3 * one_message_size, memory.memory_size)
    result = await memory_message_service.embed_and_save(memory, [new_message])
    conn.logger.debug("Capacity save result=%r cache_decreases=%s cache_increases=%s", result, decrease.call_args_list, increase.call_args_list)

    assert result == (True, "Message saved successfully.")
    assert table.db.execute("SELECT message_id FROM messages ORDER BY message_id").fetchall() == [(1,), (4,)]
    assert json.loads(table.db.execute("SELECT q_2_vec FROM messages WHERE message_id = 4").fetchone()[0]) == [0.1, 0.2]
    decrease.assert_called_once_with("mem-1", 2 * one_message_size)
    increase.assert_called_once_with("mem-1", one_message_size)

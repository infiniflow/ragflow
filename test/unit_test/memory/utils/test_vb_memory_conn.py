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

"""
Unit tests for the Vastbase memory-store connection (memory/utils/vastbase_conn.py).
"""

import logging
import threading

from memory.utils.vastbase_conn import VBConnection


def _vb_memory_connection_class():
    return next(cell.cell_contents for cell in VBConnection.__closure__ if isinstance(cell.cell_contents, type))


class RecordingCursor:
    def __init__(self, conn):
        self._conn = conn

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def execute(self, query, params=None):
        self._conn.executed.append((query, params))

    def fetchone(self):
        return self._conn.fetchone_results.pop(0)


class RecordingConnection:
    def __init__(self, fetchone_results=None):
        self.executed = []
        self.commits = 0
        self.fetchone_results = list(fetchone_results or [])

    def cursor(self):
        return RecordingCursor(self)

    def commit(self):
        self.commits += 1

    def rollback(self):
        pass


def _make_connection(conn):
    connection = object.__new__(_vb_memory_connection_class())
    connection.logger = logging.getLogger("ragflow.memory_vastbase_conn")
    connection._table_exists = lambda index_name: True
    connection._get_filters = lambda condition: ["memory_id = 'm1'"]
    connection._get_conn = lambda: conn
    connection._put_conn = lambda conn_: None
    return connection


class TestUpdate:
    def test_status_update_binds_integer_not_boolean(self):
        """update_dict keys are converted (status -> status_int), so the special
        status branch must match the converted key and inline 1/0 — a bound
        Python bool fails against the INTEGER status_int column."""
        conn = RecordingConnection()
        connection = _make_connection(conn)
        assert connection.update({"id": "x1"}, {"status": False}, "memory_t1", "m1") is True
        sql, params = conn.executed[0]
        assert "status_int = 0" in sql
        assert params == {}

    def test_status_true_inlines_one(self):
        conn = RecordingConnection()
        connection = _make_connection(conn)
        assert connection.update({"id": "x1"}, {"status": True}, "memory_t1", "m1") is True
        assert "status_int = 1" in conn.executed[0][0]


class TestVectorDdl:
    def _connection(self, conn):
        connection = object.__new__(_vb_memory_connection_class())
        connection.logger = logging.getLogger("ragflow.memory_vastbase_conn")
        connection._get_conn = lambda: conn
        connection._put_conn = lambda conn_: None
        connection._table_exists_cache = set()
        connection._table_exists_cache_lock = threading.RLock()
        return connection

    def test_create_table_declares_floatvector_column(self):
        conn = RecordingConnection()
        self._connection(conn)._create_table("memory_t1", 3)
        create_sql, _ = conn.executed[0]
        assert isinstance(create_sql, str)
        assert "q_3_vec floatvector(3)" in create_sql

    def test_ensure_vector_column_alters_to_floatvector(self):
        conn = RecordingConnection(fetchone_results=[None])
        self._connection(conn)._ensure_vector_column("memory_t1", 5)
        alter_sql, _ = conn.executed[-1]
        assert isinstance(alter_sql, str)
        assert "ADD COLUMN IF NOT EXISTS q_5_vec floatvector(5)" in alter_sql

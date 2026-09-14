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
import json
import math
import sys
import threading
import types
from unittest.mock import Mock

import pytest
import psycopg2
from psycopg2 import errorcodes
from sqlglot import exp, parse_one

from common.doc_store.doc_store_base import FusionExpr, MatchDenseExpr, MatchTextExpr, OrderByExpr
from common.doc_store.gaussdb_conn_base import (
    GaussDBConnectionBase,
    GaussDBDDLBuilder,
    InvalidGaussDBObjectName,
    UnsafeGaussDBSQL,
)
import rag.utils.gaussdb_conn as gaussdb_conn_module
from rag.utils.gaussdb_conn import (
    GaussDBConnection,
    GaussDBError,
    SearchResult,
    decode_column_value,
    ordered_columns,
    parse_json_dict,
    parse_fusion_vector_weight,
    vector_literal,
    zero_vector_literal,
)


class RecordingCursor:
    def __init__(self):
        self.executed = []
        self.rowcount = 0
        self.description = None
        self.rows = []
        self.closed = False

    def execute(self, sql, params=None):
        self.executed.append((sql, params or []))

    def executemany(self, sql, params):
        materialized = [list(row) for row in params]
        self.executed.append((sql, materialized))
        self.rowcount = len(materialized)

    def fetchone(self):
        return self.rows[0] if self.rows else None

    def fetchall(self):
        return self.rows

    def close(self):
        self.closed = True


class UndefinedTableError(Exception):
    pgcode = errorcodes.UNDEFINED_TABLE


class MissingTableOnceCursor(RecordingCursor):
    def __init__(self):
        super().__init__()
        self.insert_attempts = 0

    def executemany(self, sql, params):
        materialized = [list(row) for row in params]
        self.executed.append((sql, materialized))
        self.insert_attempts += 1
        if self.insert_attempts == 1:
            raise UndefinedTableError("chunk table does not exist")
        self.rowcount = len(materialized)


class SequencedCursor(RecordingCursor):
    def __init__(self, results):
        super().__init__()
        self.results = list(results)

    def execute(self, sql, params=None):
        super().execute(sql, params)
        rows, description = self.results.pop(0)
        self.rows = rows
        self.description = description


class RecordingConnection:
    def __init__(self, cursor):
        self.cursor_obj = cursor
        self.commits = 0
        self.rollbacks = 0

    def cursor(self):
        return self.cursor_obj

    def commit(self):
        self.commits += 1

    def rollback(self):
        self.rollbacks += 1


class RecordingPool:
    def __init__(self, cursor):
        self.cursor = cursor
        self.conn = RecordingConnection(cursor)
        self.get_calls = 0
        self.put_back = []

    def get_conn(self):
        self.get_calls += 1
        return self.conn

    def put_conn(self, conn):
        self.put_back.append(conn)


class HealthPool:
    masked_uri = "u@h:5432/d?schema=public"
    resolved_schema = "public"

    def __init__(self, rows):
        self.rows = list(rows)
        self.checked = False
        self.sql = []

    def check_schema_access(self):
        self.checked = True

    def fetch_one(self, sql, _params=None):
        self.sql.append(sql)
        row = self.rows.pop(0)
        if isinstance(row, Exception):
            raise row
        return row


def make_conn(cursor, vector_dimensions=None):
    conn = GaussDBConnection.__new__(GaussDBConnection)
    conn.schema = "public"
    conn.resolved_schema = "public"
    conn.pool = RecordingPool(cursor)
    conn.ddl = GaussDBDDLBuilder(schema="public")
    conn.logger = type("Logger", (), {"debug": lambda *_args, **_kwargs: None, "error": lambda *_args, **_kwargs: None})()
    if vector_dimensions is not None:
        conn.get_vector_dimensions = lambda _table: vector_dimensions
    return conn


CANONICAL_CREATE_IDX_1024_EXECUTIONS = [
    ("SELECT pg_advisory_xact_lock(hashtext(%s))", ["create_idx:public:ragflow_wrt_012"]),
    (
        """CREATE TABLE IF NOT EXISTS "public"."ragflow_wrt_012" (
  id VARCHAR(256) NOT NULL,
  kb_id VARCHAR(256) NOT NULL,
  doc_id VARCHAR(256),
  docnm_kwd VARCHAR(256),
  doc_type_kwd VARCHAR(256),
  title_tks VARCHAR(256),
  title_sm_tks VARCHAR(256),
  content_with_weight TEXT,
  content_ltks TEXT,
  content_sm_ltks TEXT,
  important_kwd JSONB,
  important_tks TEXT,
  question_kwd JSONB,
  question_tks TEXT,
  tag_kwd JSONB,
  tag_feas JSONB,
  available_int INTEGER DEFAULT 1 NOT NULL,
  pagerank_fea INTEGER,
  create_time VARCHAR(19),
  create_timestamp_flt DOUBLE PRECISION,
  img_id VARCHAR(128),
  position_int JSONB,
  page_num_int JSONB,
  top_int JSONB,
  metadata JSONB,
  chunk_data JSONB,
  extra JSONB,
  _order_id INTEGER,
  group_id VARCHAR(256),
  mom_id VARCHAR(256),
  knowledge_graph_kwd VARCHAR(256),
  source_id JSONB,
  entity_kwd VARCHAR(256),
  entity_type_kwd VARCHAR(256),
  from_entity_kwd VARCHAR(256),
  to_entity_kwd VARCHAR(256),
  weight_int INTEGER,
  weight_flt DOUBLE PRECISION,
  entities_kwd JSONB,
  rank_flt DOUBLE PRECISION,
  n_hop_with_weight TEXT,
  removed_kwd VARCHAR(256) DEFAULT 'N',
  raptor_kwd VARCHAR(256),
  raptor_layer_int INTEGER,
  PRIMARY KEY (kb_id, id)
) WITH (storage_type=USTORE)""",
        [],
    ),
    (
        'CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_wrt_012_doc_id" ON "public"."ragflow_wrt_012" (doc_id)',
        [],
    ),
    (
        'CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_wrt_012_available_int" ON "public"."ragflow_wrt_012" (available_int)',
        [],
    ),
    (
        'CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_wrt_012_knowledge_graph_kwd" ON "public"."ragflow_wrt_012" (knowledge_graph_kwd)',
        [],
    ),
    (
        'CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_wrt_012_entity_type_kwd" ON "public"."ragflow_wrt_012" (entity_type_kwd)',
        [],
    ),
    (
        'CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_wrt_012_removed_kwd" ON "public"."ragflow_wrt_012" (removed_kwd)',
        [],
    ),
    (
        """CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_wrt_012_fts_all"
  ON "public"."ragflow_wrt_012"
  USING ugin(to_tsvector('simple', """
        "coalesce(title_tks, ' ') || ' ' || coalesce(title_sm_tks, ' ') || ' ' || "
        "coalesce(important_tks, ' ') || ' ' || coalesce(question_tks, ' ') || ' ' || "
        "coalesce(content_ltks, ' ') || ' ' || coalesce(content_sm_ltks, ' ')))",
        [],
    ),
    (
        """CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_wrt_012_fts_all_ngram"
  ON "public"."ragflow_wrt_012"
  USING ugin(to_tsvector('ngram', """
        "coalesce(title_tks, ' ') || ' ' || coalesce(title_sm_tks, ' ') || ' ' || "
        "coalesce(important_tks, ' ') || ' ' || coalesce(question_tks, ' ') || ' ' || "
        "coalesce(content_ltks, ' ') || ' ' || coalesce(content_sm_ltks, ' ')))",
        [],
    ),
    (
        'ALTER TABLE "public"."ragflow_wrt_012" ADD COLUMN IF NOT EXISTS q_1024_vec floatvector(1024) DEFAULT (array_fill(0, ARRAY[1024])::text::floatvector(1024))',
        [],
    ),
    (
        'ALTER TABLE "public"."ragflow_wrt_012" ADD COLUMN IF NOT EXISTS q_1024_vec_valid BOOLEAN DEFAULT FALSE NOT NULL',
        [],
    ),
    (
        "SELECT pg_advisory_xact_lock(hashtext(%s))",
        ["create_vector_idx:public:ragflow_wrt_012:1024"],
    ),
    ("SET LOCAL maintenance_work_mem = '1GB'", []),
    (
        'CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_wrt_012_q_1024_vec_diskann" ON "public"."ragflow_wrt_012" USING gsdiskann (q_1024_vec COSINE) WITH (subgraph_count=1)',
        [],
    ),
]


CANONICAL_CREATE_DOC_META_EXECUTIONS = [
    (
        "SELECT pg_advisory_xact_lock(hashtext(%s))",
        ["create_doc_meta_idx:public:ragflow_doc_meta_tenant"],
    ),
    (
        """CREATE TABLE IF NOT EXISTS "public"."ragflow_doc_meta_tenant" (
  id VARCHAR(256) NOT NULL,
  kb_id VARCHAR(256) NOT NULL,
  meta_fields JSONB,
  PRIMARY KEY (id)
) WITH (storage_type=USTORE)""",
        [],
    ),
    (
        'CREATE INDEX IF NOT EXISTS "idx_gdb_ragflow_doc_meta_tenant_kb_id" ON "public"."ragflow_doc_meta_tenant" (kb_id)',
        [],
    ),
]


def test_tc_sql_501_sql_executes_scoped_docengine_select_with_runtime_guard():
    cursor = RecordingCursor()
    cursor.description = [("doc_id",), ("amount",)]
    cursor.rows = [("doc1", "120")]
    conn = make_conn(cursor)

    result = conn.sql(
        "SELECT doc_id, chunk_data #>> '{amount}' AS amount FROM ragflow_tenant WHERE kb_id = 'kb1'",
        fetch_size=20,
    )

    sql, params = cursor.executed[-1]
    assert cursor.executed[0] == ("SET LOCAL statement_timeout = 30000", [])
    assert 'FROM "public".ragflow_tenant' in sql
    assert "kb_id = 'kb1'" in sql
    assert "LIMIT 20" in sql
    assert params == []
    assert result == {
        "columns": [{"name": "doc_id", "type": "text"}, {"name": "amount", "type": "text"}],
        "rows": [["doc1", "120"]],
    }


def test_tc_sql_306_sql_passes_fetch_size_to_readonly_guard(monkeypatch):
    cursor = RecordingCursor()
    cursor.description = [("doc_id",)]
    cursor.rows = [("doc1",)]
    conn = make_conn(cursor)
    source_sql = "SELECT doc_id FROM ragflow_t1 WHERE kb_id = 'kb-1'"
    guarded_sql = f"{source_sql} LIMIT 64"
    validator = Mock()
    validator.validate_and_patch.return_value = types.SimpleNamespace(sql=guarded_sql)
    readonly_guard = Mock(return_value=validator)
    gaussdb_module = sys.modules[GaussDBConnection.__module__]
    monkeypatch.setattr(gaussdb_module.GaussDBSQLValidator, "readonly_guard", readonly_guard)

    result = conn.sql(source_sql, fetch_size=64)

    readonly_guard.assert_called_once_with(default_limit=64, execution_schema="public")
    validator.validate_and_patch.assert_called_once_with(source_sql)
    assert cursor.executed == [
        ("SET LOCAL statement_timeout = 30000", []),
        (guarded_sql, []),
    ]
    assert result == {
        "columns": [{"name": "doc_id", "type": "text"}],
        "rows": [["doc1"]],
    }


def test_tc_cfg_312_db_type_reports_gaussdb():
    assert make_conn(RecordingCursor()).db_type() == "gaussdb"
    assert GaussDBConnectionBase(pool=HealthPool([])).db_type() == "gaussdb"


def test_tc_sql_511_sql_returns_connection_after_query_failure():
    class FailingCursor(RecordingCursor):
        def execute(self, sql, params=None):
            super().execute(sql, params)
            if not str(sql).startswith("SET LOCAL"):
                raise RuntimeError("query timeout")

    cursor = FailingCursor()
    conn = make_conn(cursor)

    with pytest.raises(RuntimeError, match="query timeout"):
        conn.sql("SELECT doc_id FROM ragflow_tenant WHERE kb_id = 'kb1'")

    assert cursor.executed[0] == ("SET LOCAL statement_timeout = 30000", [])
    assert cursor.executed[1][0] == ("SELECT doc_id FROM \"public\".ragflow_tenant WHERE kb_id = 'kb1' LIMIT 128")
    assert cursor.closed is True
    assert conn.pool.put_back == [conn.pool.conn]


@pytest.mark.parametrize(
    "sql",
    [
        "SELECT doc_id FROM pg_class WHERE kb_id = 'kb1'",
        "SELECT doc_id FROM public.ragflow_tenant WHERE kb_id = 'kb1'",
        "SELECT doc_id FROM ragflow_tenant",
        "SELECT doc_id FROM ragflow_tenant WHERE kb_id = 'kb1' OR 1 = 1",
        "SELECT content_with_weight FROM ragflow_tenant WHERE kb_id = 'kb1'",
        "SELECT chunk_data ->> 'amount' FROM ragflow_tenant WHERE kb_id = 'kb1'",
        "SELECT current_database() FROM ragflow_tenant WHERE kb_id = 'kb1'",
    ],
)
def test_tc_sql_510_sql_runtime_guard_rejects_unscoped_or_unsafe_sql(sql):
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    with pytest.raises(UnsafeGaussDBSQL):
        conn.sql(sql)

    assert cursor.executed == []


def test_tc_wrt_102_insert_chunk_uses_parameterized_upsert_and_valid_vector_flag():
    cursor = RecordingCursor()
    conn = make_conn(cursor, vector_dimensions=[1024])
    kb_id = "kb-wrt-102"
    vector = [0.1] * 1024
    vector_param = "[" + ",".join(["0.1"] * 1024) + "]"

    errors = conn.insert(
        [{"id": "c1", "kb_id": kb_id, "docnm_kwd": None, "q_1024_vec": vector, "content_with_weight": "x"}],
        "ragflow_wrt_102",
        kb_id,
    )

    expected_sql = (
        'INSERT INTO "public"."ragflow_wrt_102" '
        "(id, kb_id, docnm_kwd, content_with_weight, available_int, removed_kwd, q_1024_vec, q_1024_vec_valid) "
        "VALUES (%s, %s, %s, %s, %s, %s, %s::floatvector(1024), %s) "
        "ON DUPLICATE KEY UPDATE docnm_kwd = VALUES(docnm_kwd), content_with_weight = VALUES(content_with_weight), "
        "available_int = VALUES(available_int), removed_kwd = VALUES(removed_kwd), "
        "q_1024_vec = VALUES(q_1024_vec), q_1024_vec_valid = VALUES(q_1024_vec_valid)"
    )
    expected_params = [["c1", kb_id, None, "x", 1, "N", vector_param, True]]
    assert errors == []
    assert cursor.executed == [(expected_sql, expected_params)]
    assert "c1" not in expected_sql
    assert kb_id not in expected_sql
    assert vector_param not in expected_sql
    assert conn.pool.conn.commits == 1
    assert conn.pool.conn.rollbacks == 0
    assert cursor.closed is True
    assert conn.pool.put_back == [conn.pool.conn]


def test_tc_wrt_115_insert_chunk_creates_missing_gaussdb_table_and_retries_once():
    cursor = MissingTableOnceCursor()
    conn = make_conn(cursor, vector_dimensions=[4])
    create_calls = []
    conn.index_exist = lambda *_args: False
    conn.create_idx = lambda index_name, dataset_id, vector_size: create_calls.append((index_name, dataset_id, vector_size))

    errors = conn.insert(
        [
            {
                "id": "c1",
                "kb_id": "kb1",
                "doc_id": "d1",
                "content_with_weight": "hello",
                "q_4_vec": [0.1, 0.2, 0.3, 0.4],
            }
        ],
        "ragflow_tenant",
        "kb1",
    )

    assert errors == []
    assert create_calls == [("ragflow_tenant", "kb1", 4)]
    assert cursor.insert_attempts == 2
    assert conn.pool.conn.rollbacks == 1
    assert conn.pool.conn.commits == 1


def test_tc_wrt_115_insert_chunk_does_not_create_table_for_unrelated_write_failure():
    class InvalidWriteCursor(RecordingCursor):
        def executemany(self, sql, params):
            super().executemany(sql, params)
            raise ValueError("invalid chunk value")

    cursor = InvalidWriteCursor()
    conn = make_conn(cursor, vector_dimensions=[4])
    create_calls = []
    conn.create_idx = lambda *args: create_calls.append(args)

    errors = conn.insert(
        [{"id": "c1", "kb_id": "kb1", "doc_id": "d1", "q_4_vec": [0.1, 0.2, 0.3, 0.4]}],
        "ragflow_tenant",
        "kb1",
    )

    assert errors == ["c1"]
    assert create_calls == []
    assert conn.pool.conn.rollbacks == 1


def test_tc_wrt_115_insert_chunk_does_not_create_table_when_target_still_exists():
    cursor = MissingTableOnceCursor()
    conn = make_conn(cursor, vector_dimensions=[4])
    conn.index_exist = lambda *_args: True
    create_calls = []
    conn.create_idx = lambda *args: create_calls.append(args)

    errors = conn.insert(
        [{"id": "c1", "kb_id": "kb1", "doc_id": "d1", "q_4_vec": [0.1, 0.2, 0.3, 0.4]}],
        "ragflow_tenant",
        "kb1",
    )

    assert errors == ["c1"]
    assert create_calls == []
    assert cursor.insert_attempts == 1


def test_tc_wrt_108_insert_normalizes_kb_id_list_to_first_value():
    cursor = RecordingCursor()
    conn = make_conn(cursor, vector_dimensions=[4])
    vector = [0.1, 0.2, 0.3, 0.4]

    errors = conn.insert(
        [{"id": "c1", "kb_id": ["kb-wrt-108", "kb-other"], "q_4_vec": vector, "content_with_weight": "test"}],
        "ragflow_wrt_108",
        "kb-wrt-108",
    )

    expected_sql = (
        'INSERT INTO "public"."ragflow_wrt_108" '
        "(id, kb_id, content_with_weight, available_int, removed_kwd, q_4_vec, q_4_vec_valid) "
        "VALUES (%s, %s, %s, %s, %s, %s::floatvector(4), %s) "
        "ON DUPLICATE KEY UPDATE content_with_weight = VALUES(content_with_weight), "
        "available_int = VALUES(available_int), removed_kwd = VALUES(removed_kwd), "
        "q_4_vec = VALUES(q_4_vec), q_4_vec_valid = VALUES(q_4_vec_valid)"
    )
    expected_params = [["c1", "kb-wrt-108", "test", 1, "N", "[0.1,0.2,0.3,0.4]", True]]
    assert errors == []
    assert cursor.executed == [(expected_sql, expected_params)]
    assert "kb-wrt-108" not in expected_sql
    assert "kb-other" not in expected_sql


def test_tc_wrt_201_insert_missing_vector_writes_invalid_placeholder_vector():
    cursor = RecordingCursor()
    conn = make_conn(cursor, vector_dimensions=[1024])
    zero_vector = "[" + ",".join(["0"] * 1024) + "]"

    errors = conn.insert(
        [{"id": "c2", "kb_id": "k1", "content_with_weight": "mother"}],
        "ragflow_wrt_201",
        "k1",
    )

    expected_sql = (
        'INSERT INTO "public"."ragflow_wrt_201" '
        "(id, kb_id, content_with_weight, available_int, removed_kwd, q_1024_vec, q_1024_vec_valid) "
        "VALUES (%s, %s, %s, %s, %s, %s::floatvector(1024), %s) "
        "ON DUPLICATE KEY UPDATE content_with_weight = VALUES(content_with_weight), "
        "available_int = VALUES(available_int), removed_kwd = VALUES(removed_kwd), "
        "q_1024_vec = VALUES(q_1024_vec), q_1024_vec_valid = VALUES(q_1024_vec_valid)"
    )
    expected_params = [["c2", "k1", "mother", 1, "N", zero_vector, False]]
    assert errors == []
    assert cursor.executed == [(expected_sql, expected_params)]
    assert "c2" not in expected_sql
    assert "k1" not in expected_sql
    assert zero_vector not in expected_sql
    assert conn.pool.conn.commits == 1
    assert conn.pool.conn.rollbacks == 0


def test_tc_wrt_202_insert_missing_vector_rejects_when_existing_dimension_is_unknown():
    cursor = RecordingCursor()
    conn = make_conn(cursor, vector_dimensions=[])
    kb_id = "kb-wrt-202"
    document = {"id": "c2", "kb_id": kb_id, "content_with_weight": "mother"}

    errors = conn.insert([document], "ragflow_wrt_202", kb_id)

    assert errors == ["c2"]
    with pytest.raises(ValueError) as exc_info:
        conn._normalize_chunk_row("ragflow_wrt_202", document, kb_id, [])
    assert exc_info.type is ValueError
    assert exc_info.value.args == ("cannot infer GaussDB vector dimension",)
    assert str(exc_info.value) == "cannot infer GaussDB vector dimension"
    assert cursor.executed == []
    assert conn.pool.put_back == []


def test_tc_wrt_113_insert_normalization_failure_returns_all_batch_ids_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor, vector_dimensions=[4])
    vector = [0.1, 0.2, 0.3, 0.4]

    errors = conn.insert(
        [
            {"id": "c1", "kb_id": "kb1", "doc_id": "d1", "q_4_vec": vector},
            {"id": "c2", "kb_id": "kb2", "doc_id": "d2", "q_4_vec": vector},
        ],
        "ragflow_tenant",
        "kb1",
    )

    assert errors == ["c1", "c2"]
    assert cursor.executed == []
    assert conn.pool.put_back == []


def test_tc_wrt_106_insert_missing_vector_infers_dimension_from_same_batch_real_vector():
    cursor = RecordingCursor()
    conn = make_conn(cursor, vector_dimensions=[])

    errors = conn.insert(
        [
            {"id": "c1", "kb_id": "kb1", "doc_id": "d1", "q_4_vec": [0.1, 0.2, 0.3, 0.4]},
            {"id": "c2", "kb_id": "kb1", "doc_id": "d2"},
        ],
        "ragflow_tenant",
        "kb1",
    )

    expected_sql = (
        'INSERT INTO "public"."ragflow_tenant" '
        "(id, kb_id, doc_id, available_int, group_id, removed_kwd, q_4_vec, q_4_vec_valid) "
        "VALUES (%s, %s, %s, %s, %s, %s, %s::floatvector(4), %s) "
        "ON DUPLICATE KEY UPDATE doc_id = VALUES(doc_id), available_int = VALUES(available_int), "
        "group_id = VALUES(group_id), removed_kwd = VALUES(removed_kwd), q_4_vec = VALUES(q_4_vec), "
        "q_4_vec_valid = VALUES(q_4_vec_valid)"
    )
    expected_params = [
        ["c1", "kb1", "d1", 1, "d1", "N", "[0.1,0.2,0.3,0.4]", True],
        ["c2", "kb1", "d2", 1, "d2", "N", "[0,0,0,0]", False],
    ]
    assert errors == []
    assert cursor.executed == [(expected_sql, expected_params)]
    sentinels = ("c1", "c2", "kb1", "d1", "d2", "[0.1,0.2,0.3,0.4]", "[0,0,0,0]")
    assert all(value not in expected_sql for value in sentinels)
    assert conn.pool.conn.commits == 1
    assert conn.pool.conn.rollbacks == 0


def test_tc_wrt_106_insert_preserves_metadata_and_derives_group_and_title_fields():
    cursor = RecordingCursor()
    conn = make_conn(cursor, vector_dimensions=[1024])
    kb_id = "kb-wrt-106"
    vector_param = "[" + ",".join(["0.1"] * 1024) + "]"
    fallback_vector_param = "[" + ",".join(["0.2"] * 1024) + "]"
    metadata = {"_group_id": "g1", "_title": "DocName", "other": 1}
    metadata_without_group = {"status": "open"}
    metadata_param = json.dumps(metadata, ensure_ascii=False)
    metadata_without_group_param = json.dumps(metadata_without_group, ensure_ascii=False)

    errors = conn.insert(
        [
            {
                "id": "c1",
                "kb_id": kb_id,
                "doc_id": "doc1",
                "q_1024_vec": [0.1] * 1024,
                "metadata": metadata,
            },
            {
                "id": "c2",
                "kb_id": kb_id,
                "doc_id": "doc2",
                "q_1024_vec": [0.2] * 1024,
                "metadata": metadata_without_group,
            },
        ],
        "ragflow_wrt_106",
        kb_id,
    )

    expected_sql = (
        'INSERT INTO "public"."ragflow_wrt_106" '
        "(id, kb_id, doc_id, docnm_kwd, available_int, metadata, group_id, removed_kwd, "
        "q_1024_vec, q_1024_vec_valid) "
        "VALUES (%s, %s, %s, %s, %s, %s::jsonb, %s, %s, %s::floatvector(1024), %s) "
        "ON DUPLICATE KEY UPDATE doc_id = VALUES(doc_id), docnm_kwd = VALUES(docnm_kwd), "
        "available_int = VALUES(available_int), metadata = VALUES(metadata), group_id = VALUES(group_id), "
        "removed_kwd = VALUES(removed_kwd), q_1024_vec = VALUES(q_1024_vec), "
        "q_1024_vec_valid = VALUES(q_1024_vec_valid)"
    )
    expected_params = [
        ["c1", kb_id, "doc1", "DocName", 1, metadata_param, "g1", "N", vector_param, True],
        ["c2", kb_id, "doc2", None, 1, metadata_without_group_param, "doc2", "N", fallback_vector_param, True],
    ]
    assert errors == []
    assert cursor.executed == [(expected_sql, expected_params)]
    assert "c1" not in expected_sql
    assert kb_id not in expected_sql
    assert "DocName" not in expected_sql
    assert metadata_param not in expected_sql
    assert metadata_without_group_param not in expected_sql
    assert vector_param not in expected_sql
    assert fallback_vector_param not in expected_sql
    assert conn.pool.conn.commits == 1
    assert conn.pool.conn.rollbacks == 0


def test_tc_wrt_107_insert_merges_unknown_fields_with_existing_extra_values():
    cursor = RecordingCursor()
    conn = make_conn(cursor, vector_dimensions=[4])
    kb_id = "kb-wrt-107"
    vector = [0.1, 0.2, 0.3, 0.4]
    vector_param = "[0.1,0.2,0.3,0.4]"

    errors = conn.insert(
        [
            {
                "id": "c1",
                "kb_id": kb_id,
                "q_4_vec": vector,
                "extra": '{"existing": 1}',
                "custom_field1": "value1",
                "custom_field2": 2,
            },
            {
                "id": "c2",
                "kb_id": kb_id,
                "q_4_vec": vector,
                "extra": "{malformed",
                "custom_field3": 3,
            },
            {
                "id": "c3",
                "kb_id": kb_id,
                "q_4_vec": vector,
                "extra": 1,
                "custom_field4": 4,
            },
        ],
        "ragflow_wrt_107",
        kb_id,
    )

    expected_sql = (
        'INSERT INTO "public"."ragflow_wrt_107" '
        "(id, kb_id, available_int, extra, removed_kwd, q_4_vec, q_4_vec_valid) "
        "VALUES (%s, %s, %s, %s::jsonb, %s, %s::floatvector(4), %s) "
        "ON DUPLICATE KEY UPDATE available_int = VALUES(available_int), extra = VALUES(extra), "
        "removed_kwd = VALUES(removed_kwd), q_4_vec = VALUES(q_4_vec), "
        "q_4_vec_valid = VALUES(q_4_vec_valid)"
    )
    expected_params = [
        ["c1", kb_id, 1, '{"existing": 1, "custom_field1": "value1", "custom_field2": 2}', "N", vector_param, True],
        ["c2", kb_id, 1, '{"custom_field3": 3}', "N", vector_param, True],
        ["c3", kb_id, 1, '{"custom_field4": 4}', "N", vector_param, True],
    ]
    assert errors == []
    assert cursor.executed == [(expected_sql, expected_params)]
    sentinels = ("c1", "c2", "c3", kb_id, "value1", vector_param)
    assert all(value not in expected_sql for value in sentinels)
    assert conn.pool.conn.commits == 1
    assert conn.pool.conn.rollbacks == 0


def test_tc_wrt_301_insert_doc_meta_uses_parameterized_upsert():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    errors = conn.insert(
        [{"id": "d1", "kb_id": "kb-wrt-301", "meta_fields": {"status": "active"}}],
        "ragflow_doc_meta_unit",
        "kb-wrt-301",
    )

    expected_sql = 'INSERT INTO "public"."ragflow_doc_meta_unit" (id, kb_id, meta_fields) VALUES (%s, %s, %s::jsonb) ON DUPLICATE KEY UPDATE meta_fields = VALUES(meta_fields)'
    expected_params = [["d1", "kb-wrt-301", '{"status": "active"}']]
    assert errors == []
    assert cursor.executed == [(expected_sql, expected_params)]
    assert "d1" not in expected_sql
    assert "kb-wrt-301" not in expected_sql
    assert "active" not in expected_sql
    assert conn.pool.conn.commits == 1
    assert conn.pool.conn.rollbacks == 0


def test_tc_wrt_101_insert_empty_input_returns_empty_without_database_call():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    assert conn.insert([], "ragflow_tenant", "kb1") == []
    assert cursor.executed == []
    with pytest.raises(ValueError, match="rows are required"):
        conn._build_chunk_upsert("ragflow_tenant", [])


def test_tc_wrt_113_insert_returns_all_ids_and_original_error_when_write_fails(monkeypatch):
    class FailingManyCursor(RecordingCursor):
        def __init__(self, failure):
            super().__init__()
            self.failure = failure

        def executemany(self, sql, params):
            super().executemany(sql, params)
            raise self.failure

    failure = psycopg2.Error("Batch insert failed")
    cursor = FailingManyCursor(failure)
    conn = make_conn(cursor, vector_dimensions=[4])
    logger = Mock()
    gaussdb_module = sys.modules[GaussDBConnection.__module__]
    monkeypatch.setattr(gaussdb_module, "logger", logger)
    documents = [{"id": f"c{i}", "kb_id": "k1"} for i in range(1, 4)]

    errors = conn.insert(documents, "ragflow_tenant", "k1")

    expected_sql = (
        'INSERT INTO "public"."ragflow_tenant" '
        "(id, kb_id, available_int, removed_kwd, q_4_vec, q_4_vec_valid) "
        "VALUES (%s, %s, %s, %s, %s::floatvector(4), %s) "
        "ON DUPLICATE KEY UPDATE available_int = VALUES(available_int), removed_kwd = VALUES(removed_kwd), "
        "q_4_vec = VALUES(q_4_vec), q_4_vec_valid = VALUES(q_4_vec_valid)"
    )
    expected_params = [[f"c{i}", "k1", 1, "N", "[0,0,0,0]", False] for i in range(1, 4)]
    assert errors == ["c1", "c2", "c3"]
    assert len(errors) == 3
    assert cursor.executed == [(expected_sql, expected_params)]
    assert all(doc_id not in expected_sql for doc_id in errors)
    assert "k1" not in expected_sql
    assert type(failure) is psycopg2.Error
    assert failure.args == ("Batch insert failed",)
    assert str(failure) == "Batch insert failed"
    assert conn.pool.conn.rollbacks == 1
    assert conn.pool.conn.commits == 0
    logger.error.assert_called_once_with(
        "GaussDB insert failed for table=%s ids=%s error=%s",
        "ragflow_tenant",
        ["c1", "c2", "c3"],
        failure,
    )
    assert logger.error.call_args.args[-1] is failure
    assert cursor.closed is True
    assert conn.pool.put_back == [conn.pool.conn]


def test_tc_wrt_109_insert_rejects_missing_chunk_id_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    result = conn.insert([{"kb_id": "k1", "q_4_vec": [0.1, 0.2, 0.3, 0.4]}], "ragflow_tenant", "k1")

    assert result == ["chunk id is required"]
    assert "chunk id is required" in str(result[0])
    assert cursor.executed == []


def test_tc_wrt_110_insert_rejects_missing_kb_id_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    result = conn.insert([{"id": "c1", "q_4_vec": [0.1, 0.2, 0.3, 0.4]}], "ragflow_tenant", None)

    assert result == ["c1"]
    assert len(result) == 1
    assert cursor.executed == []


def test_tc_wrt_111_insert_rejects_kb_id_mismatch_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    result = conn.insert([{"id": "c1", "kb_id": "k1", "q_4_vec": [0.1, 0.2, 0.3, 0.4]}], "ragflow_tenant", "k2")

    assert result == ["c1"]
    assert len(result) == 1
    assert cursor.executed == []


def test_tc_wrt_304_insert_doc_meta_rejects_missing_doc_id_or_kb_id_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    missing_doc_id = conn.insert([{"kb_id": "kb1", "meta_fields": {}}], "ragflow_doc_meta_tenant", "kb1")
    missing_kb_id = conn.insert([{"id": "doc1", "meta_fields": {}}], "ragflow_doc_meta_tenant", None)

    assert missing_doc_id == ["doc metadata id and kb_id are required"]
    assert missing_kb_id == ["doc1"]
    with pytest.raises(ValueError, match="^doc metadata id and kb_id are required$"):
        conn._build_doc_meta_upsert("ragflow_doc_meta_tenant", [{"kb_id": "kb1"}], "kb1")
    with pytest.raises(ValueError, match="^doc metadata id and kb_id are required$"):
        conn._build_doc_meta_upsert("ragflow_doc_meta_tenant", [{"id": "doc1"}], None)
    assert cursor.executed == []


def test_tc_wrt_305_insert_doc_meta_rejects_kb_id_mismatch_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    result = conn.insert([{"id": "doc1", "kb_id": "kb2", "meta_fields": {}}], "ragflow_doc_meta_tenant", "kb1")

    assert result == ["doc1"]
    with pytest.raises(ValueError, match="^kb_id kb2 does not match dataset_id kb1$"):
        conn._build_doc_meta_upsert(
            "ragflow_doc_meta_tenant",
            [{"id": "doc1", "kb_id": "kb2", "meta_fields": {}}],
            "kb1",
        )
    assert cursor.executed == []


@pytest.mark.parametrize(
    ("condition", "predicate", "expected_params"),
    [
        ({"id": "c1"}, "id = %s", ["c1", "kb1"]),
        ({"id": ["c1", "c2"]}, "id IN (%s, %s)", ["c1", "c2", "kb1"]),
    ],
)
def test_tc_wrt_602_delete_chunk_scopes_by_id_and_kb_id(condition, predicate, expected_params):
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)

    deleted = conn.delete(condition, "ragflow_tenant", "kb1")

    sql, params = cursor.executed[-1]
    assert deleted == 1
    assert 'DELETE FROM "public"."ragflow_tenant"' in sql
    assert predicate in sql
    assert "AND kb_id = %s" in sql
    assert params == expected_params


def test_tc_wrt_309_delete_doc_meta_scopes_by_id_and_kb_id():
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)

    deleted = conn.delete({"id": "doc1"}, "ragflow_doc_meta_tenant", "kb1")

    assert deleted == 1
    assert cursor.executed == [('DELETE FROM "public"."ragflow_doc_meta_tenant" WHERE id = %s AND kb_id = %s', ["doc1", "kb1"])]
    sql = cursor.executed[0][0]
    assert "doc1" not in sql
    assert "kb1" not in sql
    assert "DROP TABLE" not in sql
    assert conn.pool.conn.commits == 1
    assert conn.pool.conn.rollbacks == 0


def test_tc_wrt_601_delete_rejects_unscoped_condition():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    deleted = conn.delete({}, "ragflow_tenant", "kb1")

    assert deleted == 0
    assert cursor.executed == []


def test_tc_wrt_613_delete_without_kb_scope_returns_zero_without_database_call():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    assert conn.delete({"id": "c1"}, "ragflow_tenant", None) == 0
    assert cursor.executed == []


def test_tc_wrt_614_delete_allows_kb_scope_from_condition_without_dataset_id():
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)

    deleted = conn.delete({"id": "c1", "kb_id": "kb1"}, "ragflow_tenant", None)

    assert deleted == 1
    assert cursor.executed == [('DELETE FROM "public"."ragflow_tenant" WHERE id = %s AND kb_id = %s', ["c1", "kb1"])]
    sql = cursor.executed[0][0]
    assert "c1" not in sql
    assert "kb1" not in sql
    assert "DROP TABLE" not in sql


def test_tc_wrt_615_repeated_missing_chunk_delete_emits_same_scoped_sql():
    cursor = RecordingCursor()
    cursor.rowcount = 0
    conn = make_conn(cursor)

    first = conn.delete({"id": "c1"}, "ragflow_tenant", "kb1")
    second = conn.delete({"id": "c1"}, "ragflow_tenant", "kb1")

    assert first == second == 0
    assert len(cursor.executed) == 2
    assert cursor.executed[0] == cursor.executed[1]


def test_tc_wrt_503_update_builds_parameterized_set_clause_and_scopes_kb_id():
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)

    updated = conn.update(
        {"id": "c1", "kb_id": "k1"},
        {"available_int": 0},
        "ragflow_tenant",
        "k1",
    )

    assert updated is True
    assert cursor.executed == [('UPDATE "public"."ragflow_tenant" SET available_int = %s WHERE id = %s AND kb_id = %s', [0, "c1", "k1"])]
    sql = cursor.executed[0][0]
    assert "c1" not in sql
    assert "k1" not in sql
    assert all(keyword not in sql for keyword in ("DELETE ", "DROP ", "CREATE ", "ALTER "))
    assert conn.pool.conn.commits == 1
    assert conn.pool.conn.rollbacks == 0


def test_tc_wrt_502_update_without_kb_scope_returns_false_without_database_call():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    updated = conn.update({"id": "c1"}, {"pagerank_fea": 9}, "ragflow_tenant", None)

    assert updated is False
    assert cursor.executed == []


@pytest.mark.parametrize(
    ("new_value", "message"),
    [
        ({"id": "new-value"}, "key column cannot be updated: id"),
        ({"kb_id": "new-value"}, "key column cannot be updated: kb_id"),
        ({"remove": "id"}, "key column cannot be updated: id"),
        ({"remove": "kb_id"}, "key column cannot be updated: kb_id"),
    ],
)
def test_tc_wrt_508_update_rejects_key_column_changes_without_database_call(new_value, message):
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    updated = conn.update({"id": "c1"}, new_value, "ragflow_tenant", "kb1")

    assert updated is False
    with pytest.raises(ValueError, match=f"^{message}$"):
        conn._build_set_clause(new_value, is_meta=False, condition={"id": "c1"})
    assert cursor.executed == []


def test_tc_wrt_508_update_ignores_redundant_matching_id():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    updated = conn.update(
        {"id": "c1"},
        {"id": "c1", "content_with_weight": "updated", "q_3_vec": [0.1, 0.2, 0.3]},
        "ragflow_tenant",
        "kb1",
    )

    assert updated is True
    assert cursor.executed == [
        (
            'UPDATE "public"."ragflow_tenant" SET content_with_weight = %s, q_3_vec = %s::floatvector(3), q_3_vec_valid = TRUE WHERE id = %s AND kb_id = %s',
            ["updated", "[0.1,0.2,0.3]", "c1", "kb1"],
        )
    ]


def test_tc_wrt_509_update_merges_dynamic_column_into_extra_jsonb():
    cursor = RecordingCursor()
    cursor.rows = [("c1", "k1", '{"existing": 1}')]
    conn = make_conn(cursor)

    result = conn.update({"id": "c1", "kb_id": "k1"}, {"unknown_col": 1}, "ragflow_tenant", "k1")

    assert result is True
    select_sql, select_params = cursor.executed[-2]
    update_sql, update_params = cursor.executed[-1]
    assert "SELECT id, kb_id, extra" in select_sql
    assert "FOR UPDATE" in select_sql
    assert select_params == ["c1", "k1"]
    assert "extra = %s::jsonb" in update_sql
    assert json.loads(update_params[0]) == {"existing": 1, "unknown_col": 1}
    assert update_params[1:] == ["k1", "c1"]


def test_tc_wrt_509_update_rejects_unsafe_dynamic_column_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    result = conn.update(
        {"id": "c1", "kb_id": "k1"},
        {"bad'; DROP TABLE user; --": 1},
        "ragflow_tenant",
        "k1",
    )

    assert result is False
    assert cursor.executed == []


def test_tc_wrt_507_update_adds_jsonb_multivalue_with_scoped_parameters():
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)

    updated = conn.update({"id": "c1", "kb_id": "k1"}, {"add": {"tag_kwd": "t3"}}, "ragflow_tenant", "k1")

    assert updated is True
    assert cursor.executed == [
        (
            'UPDATE "public"."ragflow_tenant" '
            "SET tag_kwd = (SELECT CASE WHEN candidate @> %s::jsonb THEN candidate "
            "ELSE json_array_append(candidate, '$', %s::jsonb)::jsonb END "
            "FROM (SELECT COALESCE(tag_kwd, '[]'::jsonb) AS candidate) AS jsonb_update) "
            "WHERE id = %s AND kb_id = %s",
            ['["t3"]', '"t3"', "c1", "k1"],
        )
    ]
    sql = cursor.executed[0][0]
    assert "t3" not in sql
    assert "c1" not in sql
    assert "k1" not in sql


def test_tc_wrt_506_update_removes_jsonb_value_with_scoped_parameters():
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)

    updated = conn.update({"id": "c1", "kb_id": "k1"}, {"remove": {"tag_kwd": "t1"}}, "ragflow_tenant", "k1")

    assert updated is True
    assert cursor.executed == [
        (
            'UPDATE "public"."ragflow_tenant" '
            "SET tag_kwd = COALESCE(json_remove(tag_kwd, json_unquote(json_search(tag_kwd, 'one', "
            "jsonb_array_element_text(%s::jsonb, 0))))::jsonb, tag_kwd) WHERE id = %s AND kb_id = %s",
            ['["t1"]', "c1", "k1"],
        )
    ]
    sql = cursor.executed[0][0]
    assert "t1" not in sql
    assert "c1" not in sql
    assert "k1" not in sql


def test_tc_wrt_506_json_search_treats_wildcards_as_literals():
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)

    updated = conn.update(
        {"id": "c1", "kb_id": "k1"},
        {"remove": {"tag_kwd": "a%_\\b"}},
        "ragflow_tenant",
        "k1",
    )

    assert updated is True
    assert json.loads(cursor.executed[0][1][0]) == ["a\\%\\_\\\\b"]
    assert cursor.executed[0][1][1:] == ["c1", "k1"]


def test_tc_wrt_507_same_column_remove_then_add_uses_one_assignment():
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)

    updated = conn.update(
        {"id": "c1", "kb_id": "k1"},
        {"add": {"tag_kwd": "new"}, "remove": {"tag_kwd": "old"}},
        "ragflow_tenant",
        "k1",
    )

    assert updated is True
    assert cursor.executed == [
        (
            'UPDATE "public"."ragflow_tenant" '
            "SET tag_kwd = (SELECT CASE WHEN candidate @> %s::jsonb THEN candidate "
            "ELSE json_array_append(candidate, '$', %s::jsonb)::jsonb END "
            "FROM (SELECT COALESCE(json_remove(tag_kwd, json_unquote(json_search(tag_kwd, 'one', "
            "jsonb_array_element_text(%s::jsonb, 0))))::jsonb, tag_kwd, '[]'::jsonb) AS candidate) AS jsonb_update) "
            "WHERE id = %s AND kb_id = %s",
            ['["new"]', '"new"', '["old"]', "c1", "k1"],
        )
    ]
    assert cursor.executed[0][0].count("SET tag_kwd =") == 1


def test_tc_wrt_505_update_remove_column_sets_null_with_scoped_parameters():
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)

    updated = conn.update({"id": "c1", "kb_id": "k1"}, {"remove": "important_kwd"}, "ragflow_tenant", "k1")

    assert updated is True
    assert cursor.executed == [('UPDATE "public"."ragflow_tenant" SET important_kwd = NULL WHERE id = %s AND kb_id = %s', ["c1", "k1"])]
    sql = cursor.executed[0][0]
    assert "c1" not in sql
    assert "k1" not in sql


@pytest.mark.parametrize(
    ("new_value", "expected_error"),
    [
        ({"remove": {"chunk_data": "x"}}, "unsupported JSONB remove target: chunk_data"),
        ({"add": {"chunk_data": "x"}}, "unsupported JSONB add target: chunk_data"),
        ({"remove": 1}, "unsupported remove target: 1"),
        ({"remove": "bad_column"}, "unsupported remove target: bad_column"),
        ({"add": "bad"}, "unsupported add target: bad"),
    ],
)
def test_tc_wrt_510_update_rejects_invalid_remove_add_operations_without_sql(new_value, expected_error):
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    with pytest.raises(ValueError) as exc_info:
        conn._build_set_clause(new_value, is_meta=False, condition={"id": "c1", "kb_id": "k1"})

    assert str(exc_info.value) == expected_error
    result = conn.update({"id": "c1", "kb_id": "k1"}, new_value, "ragflow_tenant", "k1")

    assert result is False
    assert cursor.executed == []


def test_tc_wrt_611_delete_jsonb_multivalue_conditions_use_contains_predicates():
    cursor = RecordingCursor()
    cursor.rowcount = 2
    conn = make_conn(cursor)

    deleted = conn.delete({"tag_kwd": ["t1"]}, "ragflow_tenant", "k1")

    assert deleted == 2
    assert cursor.executed == [('DELETE FROM "public"."ragflow_tenant" WHERE (tag_kwd @> %s::jsonb) AND kb_id = %s', ['["t1"]', "k1"])]
    sql = cursor.executed[0][0]
    assert "t1" not in sql
    assert "k1" not in sql
    assert "DROP TABLE" not in sql


def test_tc_wrt_607_delete_must_not_exists_uses_is_null_with_kb_scope():
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)

    deleted = conn.delete({"must_not": {"exists": "tag_kwd"}}, "ragflow_tenant", "k1")

    assert deleted == 1
    assert cursor.executed == [('DELETE FROM "public"."ragflow_tenant" WHERE tag_kwd IS NULL AND kb_id = %s', ["k1"])]


def test_tc_wrt_616_delete_compile_kwd_uses_extra_jsonb_mapping():
    cursor = RecordingCursor()
    cursor.rowcount = 2
    conn = make_conn(cursor)

    deleted = conn.delete(
        {"doc_id": "d1", "id": ["c1", "c2"], "must_not": {"exists": "compile_kwd"}},
        "ragflow_tenant",
        "k1",
    )

    assert deleted == 2
    assert cursor.executed == [
        (
            'DELETE FROM "public"."ragflow_tenant" WHERE doc_id = %s AND id IN (%s, %s) AND (extra #>> \'{compile_kwd}\') IS NULL AND kb_id = %s',
            ["d1", "c1", "c2", "k1"],
        )
    ]


@pytest.mark.parametrize(
    ("condition", "index_name", "is_meta", "expected_error"),
    [
        ({"id": []}, "ragflow_tenant", False, "empty list condition for id"),
        ({"tag_kwd": []}, "ragflow_tenant", False, "empty list condition for tag_kwd"),
        ({"unknown": "x"}, "ragflow_doc_meta_tenant", True, "unsupported metadata filter column: unknown"),
    ],
    ids=["empty-scalar-list", "empty-jsonb-list", "metadata-column"],
)
def test_tc_wrt_609_delete_rejects_empty_or_unsupported_conditions_without_sql(
    condition,
    index_name,
    is_meta,
    expected_error,
):
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    result = conn.delete(condition, index_name, "k1")

    assert result == 0
    with pytest.raises(ValueError) as exc_info:
        conn._build_where_clause(condition, "k1", is_meta=is_meta)
    assert str(exc_info.value) == expected_error
    assert cursor.executed == []


def test_tc_wrt_616_delete_filters_dynamic_fields_through_extra_jsonb():
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)

    deleted = conn.delete(
        {"compile_type": ["artifact_page"]},
        "ragflow_tenant",
        "kb1",
    )

    assert deleted == 1
    sql, params = cursor.executed[-1]
    assert "(extra -> 'compile_type') = %s::jsonb" in sql
    assert params == ['"artifact_page"', '["artifact_page"]', "kb1"]


def test_tc_wrt_610_delete_rejects_kb_id_mismatch_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    result = conn.delete({"kb_id": "k2"}, "ragflow_tenant", "k1")

    assert result == 0
    with pytest.raises(ValueError, match="condition kb_id k2 does not match dataset_id k1"):
        conn._build_where_clause({"kb_id": "k2"}, "k1", is_meta=False)
    assert cursor.executed == []


def test_tc_ret_511_point_get_scopes_by_id_and_kb_id_and_decodes_jsonb():
    cursor = RecordingCursor()
    cursor.description = [("id",), ("kb_id",), ("position_int",), ("q_4_vec",), ("q_4_vec_valid",)]
    cursor.rows = [("c1", "kb1", "[[1, 2, 3, 4]]", "[0,0,0,0]", False)]
    conn = make_conn(cursor)

    row = conn.get("c1", "ragflow_tenant", ["kb1"])

    sql, params = cursor.executed[-1]
    assert sql == 'SELECT * FROM "public"."ragflow_tenant" WHERE id = %s AND kb_id IN (%s) LIMIT 1'
    assert params == ["c1", "kb1"]
    assert row == {"id": "c1", "kb_id": "kb1", "position_int": [[1, 2, 3, 4]], "q_4_vec_valid": False}

    cursor.rows = [("c1", "kb1", "[[1, 2, 3, 4]]", "[0,0,0,0]", False), ("c2", "kb2", "[]", "[0,0,0,0]", False)]
    rows = conn.get("c1", "ragflow_tenant", ["kb1", "kb2"])
    assert rows["id"] == "c1"
    assert "ORDER BY kb_id ASC LIMIT 2" in cursor.executed[-1][0]


def test_tc_ret_513_get_promotes_dynamic_fields_from_extra_jsonb():
    cursor = RecordingCursor()
    cursor.description = [("id",), ("kb_id",), ("extra",)]
    cursor.rows = [("c1", "kb1", '{"compile_type": "artifact_page"}')]
    conn = make_conn(cursor)

    row = conn.get("c1", "ragflow_tenant", ["kb1"])

    assert row["compile_type"] == "artifact_page"
    assert row["extra"] == {"compile_type": "artifact_page"}


def test_tc_wrt_410_get_doc_meta_allows_empty_kb_scope_and_decodes_jsonb():
    cursor = RecordingCursor()
    cursor.description = [("id",), ("kb_id",), ("meta_fields",)]
    cursor.rows = [("doc1", "kb1", '{"author": "Alice"}')]
    conn = make_conn(cursor)

    row = conn.get("doc1", "ragflow_doc_meta_tenant", [""])

    sql, params = cursor.executed[-1]
    assert "WHERE id = %s" in sql
    assert "kb_id IN" not in sql
    assert params == ["doc1"]
    assert row == {"id": "doc1", "kb_id": "kb1", "meta_fields": {"author": "Alice"}}


def test_tc_wrt_401_get_empty_chunk_id_returns_none_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    assert conn.get("", "ragflow_tenant", ["kb1"]) is None
    assert cursor.executed == []


def test_tc_wrt_409_get_missing_chunk_returns_none_after_scoped_lookup():
    cursor = SequencedCursor(
        [
            ([(1,)], [("?column?",)]),
            ([], [("id",), ("kb_id",)]),
        ]
    )
    conn = make_conn(cursor)

    result = conn.get("c1", "ragflow_tenant", ["kb1"])

    assert result is None
    assert "information_schema.tables" in cursor.executed[0][0]
    assert cursor.executed[0][1] == ["public", "ragflow_tenant"]
    assert "WHERE id = %s AND kb_id IN (%s) LIMIT 1" in cursor.executed[1][0]
    assert cursor.executed[1][1] == ["c1", "kb1"]


def test_tc_wrt_402_get_non_meta_without_kb_ids_returns_none():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    assert conn.get("c1", "ragflow_tenant", []) is None
    assert cursor.executed == []


def test_tc_wrt_403_get_returns_none_when_table_does_not_exist():
    cursor = SequencedCursor(
        [
            ([], None),
        ]
    )
    conn = make_conn(cursor)

    assert conn.get("c1", "ragflow_tenant", ["kb1"]) is None
    assert len(cursor.executed) == 1
    assert "information_schema.tables" in cursor.executed[0][0]


def test_tc_ret_003_search_doc_meta_table_returns_search_result():
    cursor = RecordingCursor()
    cursor.description = [("id",), ("kb_id",), ("meta_fields",), ("__total",)]
    cursor.rows = [("doc1", "kb1", '{"author": "Alice"}', 1001)]
    conn = make_conn(cursor)

    result = conn.search(
        select_fields=["id", "kb_id", "meta_fields", "id"],
        highlight_fields=[],
        condition={"id": "doc1"},
        match_expressions=[],
        order_by=OrderByExpr().desc("id"),
        offset=5,
        limit=10,
        index_names="ragflow_doc_meta_tenant",
        knowledgebase_ids=["kb1", "kb2", "kb1"],
    )

    sql, params = cursor.executed[-1]
    assert result == SearchResult(total=1001, chunks=[{"id": "doc1", "kb_id": "kb1", "meta_fields": {"author": "Alice"}}])
    assert 'SELECT id, kb_id, meta_fields, COUNT(*) OVER() AS __total FROM "public"."ragflow_doc_meta_tenant"' in sql
    assert "id = %s" in sql
    assert "kb_id IN (%s, %s)" in sql
    assert "ORDER BY id DESC" in sql
    assert "LIMIT %s OFFSET %s" in sql
    assert "to_tsvector" not in sql.lower()
    assert "plainto_tsquery" not in sql.lower()
    assert params == ["doc1", "kb1", "kb2", 10, 5]


def test_tc_ret_003_search_doc_meta_empty_page_falls_back_to_count():
    cursor = RecordingCursor()
    conn = make_conn(cursor)
    conn._fetch_all_with_description = lambda _sql, _params: ([], [("id",), ("kb_id",), ("meta_fields",), ("__total",)])
    count_queries = []

    def fake_fetch_one(sql, params):
        count_queries.append((sql, params))
        return (1001,)

    conn._fetch_one = fake_fetch_one

    result = conn.search(
        select_fields=["*"],
        highlight_fields=[],
        condition={"id": "doc-missing"},
        match_expressions=[],
        order_by=OrderByExpr(),
        offset=2000,
        limit=1000,
        index_names="ragflow_doc_meta_tenant",
        knowledgebase_ids=["kb1"],
    )

    sql, params = count_queries[0]
    assert result == SearchResult(total=1001, chunks=[])
    assert 'SELECT COUNT(*) FROM "public"."ragflow_doc_meta_tenant"' in sql
    assert "id = %s" in sql
    assert "kb_id IN (%s)" in sql
    assert "to_tsvector" not in sql.lower()
    assert "plainto_tsquery" not in sql.lower()
    assert params == ["doc-missing", "kb1"]


def test_tc_mgf_713_fetch_metadata_doc_ids_builds_scoped_jsonb_query():
    cursor = RecordingCursor()
    cursor.rows = [("doc2",), ("doc1",)]
    conn = make_conn(cursor)

    doc_ids = conn.fetch_metadata_doc_ids(
        "ragflow_doc_meta_tenant",
        ["kb1", "kb2"],
        "lower(meta_fields #>> '{author}') = %s",
        ["alice"],
        25,
    )

    sql, params = cursor.executed[-1]
    assert doc_ids == ["doc2", "doc1"]
    assert 'SELECT id FROM "public"."ragflow_doc_meta_tenant"' in sql
    assert "kb_id IN (%s, %s)" in sql
    assert "(lower(meta_fields #>> '{author}') = %s)" in sql
    assert "ORDER BY id" in sql
    assert "LIMIT %s" in sql
    assert params == ["kb1", "kb2", "alice", 25]
    assert cursor.closed is True
    assert conn.pool.put_back == [conn.pool.conn]
    assert conn.pool.conn.commits == 0
    assert conn.pool.conn.rollbacks == 0


def test_tc_wrt_202_get_vector_dimensions_reads_q_vector_columns():
    cursor = RecordingCursor()
    cursor.rows = [("q_3072_vec",), ("q_1024_vec",), ("q_3072_vec_valid",)]
    conn = make_conn(cursor)

    dimensions = conn.get_vector_dimensions("ragflow_tenant")

    sql, params = cursor.executed[0]
    query = parse_one(sql, read="postgres")
    table = query.find(exp.Table)
    predicates = list(query.find_all(exp.EQ))
    assert dimensions == [1024, 3072]
    assert isinstance(query, exp.Select)
    assert [expression.name for expression in query.expressions] == ["column_name"]
    assert (table.db, table.name) == ("information_schema", "columns")
    assert {predicate.this.name for predicate in predicates} == {"table_schema", "table_name"}
    assert all(isinstance(predicate.expression, exp.Placeholder) for predicate in predicates)
    assert params == ["public", "ragflow_tenant"]


def test_tc_wrt_705_index_exist_checks_information_schema_tables():
    cursor = RecordingCursor()
    cursor.rows = [(1,)]
    conn = make_conn(cursor)

    result = conn.index_exist("ragflow_tenant")

    sql, params = cursor.executed[0]
    query = parse_one(sql, read="postgres")
    table = query.find(exp.Table)
    predicates = list(query.find_all(exp.EQ))
    assert result is True
    assert isinstance(query, exp.Select)
    assert query.expressions[0].this == "1"
    assert (table.db, table.name) == ("information_schema", "tables")
    assert {predicate.this.name for predicate in predicates} == {"table_schema", "table_name"}
    assert all(isinstance(predicate.expression, exp.Placeholder) for predicate in predicates)
    assert query.args["limit"].expression.this == "1"
    assert params == ["public", "ragflow_tenant"]
    assert conn.pool.conn.commits == 0
    assert conn.pool.conn.rollbacks == 0
    assert cursor.closed is True
    assert conn.pool.put_back == [conn.pool.conn]


def test_tc_wrt_708_index_exist_rejects_invalid_index_name_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    with pytest.raises(InvalidGaussDBObjectName, match="^1abc$"):
        conn.index_exist("1abc")

    assert cursor.executed == []


def test_tc_wrt_701_delete_idx_with_dataset_id_deletes_only_that_kb():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    result = conn.delete_idx("ragflow_tenant", "k1")

    assert result is None
    assert cursor.executed == [('DELETE FROM "public"."ragflow_tenant" WHERE kb_id = %s', ["k1"])]
    sql = cursor.executed[0][0]
    assert "k1" not in sql
    assert "DROP TABLE" not in sql
    assert conn.pool.conn.commits == 1
    assert conn.pool.conn.rollbacks == 0
    assert cursor.closed is True
    assert conn.pool.put_back == [conn.pool.conn]


def test_tc_wrt_702_delete_idx_without_dataset_id_drops_table():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    result = conn.delete_idx("ragflow_tenant", None)

    assert result is None
    assert cursor.executed == [('DROP TABLE IF EXISTS "public"."ragflow_tenant"', [])]
    assert "DELETE FROM" not in cursor.executed[0][0]
    assert conn.pool.conn.commits == 1
    assert conn.pool.conn.rollbacks == 0
    assert cursor.closed is True
    assert conn.pool.put_back == [conn.pool.conn]


def test_tc_wrt_709_delete_idx_missing_table_only_ignores_undefined_table():
    class CodedDatabaseError(psycopg2.Error):
        def __init__(self, code):
            super().__init__("database error")
            self._code = code

        @property
        def pgcode(self):
            return self._code

    class FailingCursor(RecordingCursor):
        def __init__(self, failure):
            super().__init__()
            self.failure = failure

        def execute(self, sql, params=None):
            super().execute(sql, params)
            raise self.failure

    missing_cursor = FailingCursor(CodedDatabaseError(errorcodes.UNDEFINED_TABLE))
    missing_conn = make_conn(missing_cursor)

    assert missing_conn.delete_idx("ragflow_missing", "k1") is None
    assert missing_cursor.executed == [('DELETE FROM "public"."ragflow_missing" WHERE kb_id = %s', ["k1"])]
    assert missing_conn.pool.conn.rollbacks == 1
    assert missing_conn.pool.conn.commits == 0
    assert missing_cursor.closed is True
    assert missing_conn.pool.put_back == [missing_conn.pool.conn]

    permission_cursor = FailingCursor(CodedDatabaseError(errorcodes.INSUFFICIENT_PRIVILEGE))
    permission_conn = make_conn(permission_cursor)
    with pytest.raises(CodedDatabaseError) as permission_error:
        permission_conn.delete_idx("ragflow_forbidden", "k1")
    assert permission_error.value.pgcode == errorcodes.INSUFFICIENT_PRIVILEGE
    assert permission_conn.pool.conn.rollbacks == 1
    assert permission_conn.pool.conn.commits == 0
    assert permission_cursor.closed is True
    assert permission_conn.pool.put_back == [permission_conn.pool.conn]

    runtime_cursor = FailingCursor(RuntimeError("connection failure"))
    runtime_conn = make_conn(runtime_cursor)
    with pytest.raises(RuntimeError, match="connection failure"):
        runtime_conn.delete_idx("ragflow_unavailable", "k1")
    assert runtime_conn.pool.conn.rollbacks == 1
    assert runtime_conn.pool.conn.commits == 0
    assert runtime_cursor.closed is True
    assert runtime_conn.pool.put_back == [runtime_conn.pool.conn]


def test_tc_wrt_303_create_doc_meta_idx_rejects_non_metadata_table_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    with pytest.raises(ValueError, match="^invalid GaussDB document metadata table name: ragflow_tenant$"):
        conn.create_doc_meta_idx("ragflow_tenant")

    assert cursor.executed == []


def test_tc_wrt_912_create_doc_meta_idx_executes_table_and_kb_index_ddls():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    result = conn.create_doc_meta_idx("ragflow_doc_meta_tenant")

    assert result is True
    assert cursor.executed == CANONICAL_CREATE_DOC_META_EXECUTIONS
    assert conn.pool.conn.commits == 1
    assert conn.pool.conn.rollbacks == 0
    assert cursor.closed is True
    assert conn.pool.put_back == [conn.pool.conn]


def test_tc_wrt_011_create_idx_uses_schema_lock_before_ddl():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    conn.create_idx("ragflow_tenant", "kb1", 4)

    statements = [sql for sql, _params in cursor.executed]
    assert statements[0] == "SELECT pg_advisory_xact_lock(hashtext(%s))"
    assert cursor.executed[0][1] == ["create_idx:public:ragflow_tenant"]
    work_mem_index = statements.index("SET LOCAL maintenance_work_mem = '1GB'")
    diskann_index = next(i for i, sql in enumerate(statements) if "USING gsdiskann" in sql)
    assert work_mem_index < diskann_index


def test_tc_wrt_010_create_idx_keeps_functional_table_when_diskann_ddl_fails():
    class FailingDiskANNCursor(RecordingCursor):
        def execute(self, sql, params=None):
            super().execute(sql, params)
            if "USING gsdiskann" in sql:
                raise psycopg2.OperationalError("DiskANN index creation failed")

    cursor = FailingDiskANNCursor()
    conn = make_conn(cursor)

    with pytest.raises(psycopg2.OperationalError) as exc_info:
        conn.create_idx("ragflow_wrt_012", "kb-diskann-fail", 1024)

    assert exc_info.type is psycopg2.OperationalError
    assert exc_info.value.args == ("DiskANN index creation failed",)
    assert cursor.executed == CANONICAL_CREATE_IDX_1024_EXECUTIONS
    assert conn.pool.conn.rollbacks == 1
    assert conn.pool.conn.commits == 1
    assert cursor.closed is True
    assert conn.pool.put_back == [conn.pool.conn, conn.pool.conn]


def test_tc_wrt_012_create_idx_executes_complete_ddl_sequence():
    cursor = RecordingCursor()
    conn = make_conn(cursor)
    table = "ragflow_wrt_012"

    result = conn.create_idx(table, "kb-wrt-012", 1024)

    assert result is True
    assert len(CANONICAL_CREATE_IDX_1024_EXECUTIONS) == 14
    assert cursor.executed == CANONICAL_CREATE_IDX_1024_EXECUTIONS
    assert all("kb-wrt-012" not in sql for sql, _params in cursor.executed)
    assert conn.pool.conn.commits == 2
    assert conn.pool.conn.rollbacks == 0
    assert cursor.closed is True
    assert conn.pool.put_back == [conn.pool.conn, conn.pool.conn]


@pytest.mark.parametrize("adapter", ["get_fields", "get_highlight", "get_scores", "get"])
def test_tc_ret_508_chunk_adapters_reject_cross_kb_duplicate_ids(adapter):
    duplicate_chunks = [
        {"id": "c1", "kb_id": "kb1", "doc_id": "d1", "_highlight": "old", "_score": 0.1},
        {"id": "c1", "kb_id": "kb2", "doc_id": "d2", "_highlight": "new", "_score": 0.9},
    ]
    if adapter == "get":
        cursor = RecordingCursor()
        cursor.description = [("id",), ("kb_id",), ("doc_id",)]
        cursor.rows = [("c1", "kb1", "d1"), ("c1", "kb2", "d2")]
        conn = make_conn(cursor)

        def call():
            return conn.get("c1", "ragflow_tenant", ["kb1", "kb2"])
    else:
        conn = GaussDBConnection.__new__(GaussDBConnection)
        result = SearchResult(total=2, chunks=duplicate_chunks)
        calls = {
            "get_fields": lambda: conn.get_fields(result, ["doc_id"]),
            "get_highlight": lambda: conn.get_highlight(result, [], "content_with_weight"),
            "get_scores": lambda: conn.get_scores(result),
        }
        call = calls[adapter]

    with pytest.raises(GaussDBError, match="^cross-KB duplicate chunk id: c1$"):
        call()


def test_tc_ret_509_search_hides_invalid_placeholder_vector_when_vector_field_is_requested():
    cursor = RecordingCursor()
    cursor.description = [
        ("id",),
        ("kb_id",),
        ("q_4_vec",),
        ("q_4_vec_valid",),
        ("_score",),
        ("__total",),
    ]
    cursor.rows = [("c1", "kb1", "[0,0,0,0]", False, 0.0, 1)]
    conn = make_conn(cursor)

    result = conn.search(["q_4_vec"], [], {}, [], OrderByExpr(), 0, 10, "ragflow_tenant", ["kb1"])

    sql, _params = cursor.executed[-1]
    assert "q_4_vec_valid" in sql
    assert conn.get_fields(result, ["q_4_vec"]) == {"c1": {}}


def test_tc_ret_009_search_empty_deep_page_falls_back_to_total_count():
    description = [("id",), ("kb_id",), ("_score",), ("__total",)]
    cursor = SequencedCursor(
        [
            ([], description),
            ([("c1", "kb1", 0.0, 15)], description),
        ]
    )
    conn = make_conn(cursor)

    result = conn.search(["id"], [], {}, [], OrderByExpr(), 20, 10, "ragflow_tenant", ["kb1"])

    assert result.total == 15
    assert result.chunks == []
    assert len(cursor.executed) == 2
    first_sql, first_params = cursor.executed[0]
    fallback_sql, fallback_params = cursor.executed[1]
    assert all('FROM "public"."ragflow_tenant"' in sql for sql in (first_sql, fallback_sql))
    assert all("WHERE kb_id IN (%s)" in sql for sql in (first_sql, fallback_sql))
    assert "COUNT(*) OVER() AS __total" in fallback_sql
    assert first_params[-2:] == [10, 20]
    assert fallback_params[-2:] == [1, 0]


def test_tc_ret_002_multi_table_search_applies_global_pagination_and_ordering():
    description = [("id",), ("kb_id",), ("_score",), ("__total",)]
    cursor = SequencedCursor(
        [
            ([("c1", "kb1", 0.9, 2), ("c3", "kb1", 0.3, 2)], description),
            ([("c2", "kb1", 0.8, 1)], description),
        ]
    )
    conn = make_conn(cursor)
    match = MatchTextExpr(["content_ltks"], "risk", 10, {"original_query": "risk"})

    result = conn.search(["id"], [], {}, [match], OrderByExpr(), 1, 1, ["ragflow_tenant_a", "ragflow_tenant_b"], ["kb1"])

    assert result.total == 3
    assert [chunk["id"] for chunk in result.chunks] == ["c2"]
    assert len(cursor.executed) == 2
    assert all("LIMIT %s OFFSET %s" in sql for sql, _params in cursor.executed)
    assert [params[-2:] for _sql, params in cursor.executed] == [[2, 0], [2, 0]]


def test_tc_ret_002_multi_table_search_breaks_score_ties_by_kb_then_id():
    description = [("id",), ("kb_id",), ("_score",), ("__total",)]
    cursor = SequencedCursor(
        [
            ([("c4", "kb2", 0.8, 2), ("c2", "kb1", 0.8, 2)], description),
            ([("c3", "kb2", 0.8, 2), ("c1", "kb1", 0.8, 2)], description),
        ]
    )
    conn = make_conn(cursor)
    match = MatchTextExpr(["content_ltks"], "risk", 10, {"original_query": "risk"})

    result = conn.search(
        ["id"],
        [],
        {},
        [match],
        OrderByExpr(),
        0,
        10,
        ["ragflow_tenant_a", "ragflow_tenant_b"],
        ["kb1", "kb2"],
    )

    assert result.total == 4
    assert [(chunk["kb_id"], chunk["id"]) for chunk in result.chunks] == [
        ("kb1", "c1"),
        ("kb1", "c2"),
        ("kb2", "c3"),
        ("kb2", "c4"),
    ]


def test_tc_ret_002_multi_table_without_match_or_order_sorts_by_kb_then_id():
    description = [("id",), ("kb_id",), ("_score",), ("__total",)]
    cursor = SequencedCursor(
        [
            ([("c4", "kb2", 0.0, 2), ("c2", "kb1", 0.0, 2)], description),
            ([("c3", "kb2", 0.0, 2), ("c1", "kb1", 0.0, 2)], description),
        ]
    )
    conn = make_conn(cursor)

    result = conn.search(
        ["id"],
        [],
        {},
        [],
        OrderByExpr(),
        0,
        10,
        ["ragflow_tenant_a", "ragflow_tenant_b"],
        ["kb1", "kb2"],
    )

    assert result.total == 4
    assert [(chunk["kb_id"], chunk["id"]) for chunk in result.chunks] == [
        ("kb1", "c1"),
        ("kb1", "c2"),
        ("kb2", "c3"),
        ("kb2", "c4"),
    ]


def test_tc_ret_606_multi_table_aggregation_merges_duplicate_buckets():
    description = [("value",), ("count",)]
    cursor = SequencedCursor(
        [
            ([("doc-a", 2), ("doc-b", 1)], description),
            ([("doc-a", 3)], description),
        ]
    )
    conn = make_conn(cursor)

    result = conn.search([], [], {}, [], OrderByExpr(), 0, 0, ["ragflow_tenant_a", "ragflow_tenant_b"], ["kb1"], ["docnm_kwd"])

    assert result.chunks == [{"value": "doc-a", "count": 5}, {"value": "doc-b", "count": 1}]
    assert result.total == 2


def test_tc_ret_603_get_aggregation_counts_local_jsonb_array_values():
    conn = GaussDBConnection.__new__(GaussDBConnection)
    result = SearchResult(
        total=4,
        chunks=[{"tag_kwd": ["a", "b", ""]}, {"tag_kwd": ["a"]}, {"tag_kwd": "c"}, {"tag_kwd": None}],
    )

    assert sorted(conn.get_aggregation(result, "tag_kwd")) == [("a", 2), ("b", 1), ("c", 1)]


def test_tc_ret_604_get_aggregation_uses_value_count_rows_without_recounting():
    conn = GaussDBConnection.__new__(GaussDBConnection)
    result = SearchResult(total=2, chunks=[{"value": "active", "count": 2}, {"value": "inactive", "count": 1}])

    assert conn.get_aggregation(result, "doc_type_kwd") == [("active", 2), ("inactive", 1)]


def test_tc_cfg_313_base_connection_uses_shared_gaussdb_conn_by_default(monkeypatch):
    class BasePool:
        masked_uri = "user@host:19995/postgres?schema=public"
        resolved_schema = "public"

        def __init__(self):
            self.checked = False

        def check_schema_access(self):
            self.checked = True

    pool = BasePool()
    monkeypatch.setitem(sys.modules, "common.doc_store.gaussdb_conn_pool", types.SimpleNamespace(GAUSSDB_CONN=pool))

    base = GaussDBConnectionBase()

    assert base.pool is pool
    assert pool.checked is True


@pytest.mark.parametrize(
    ("method_name", "args"),
    [
        ("create_idx", ("ragflow_tenant", "kb1", 4)),
        ("delete_idx", ("ragflow_tenant", "kb1")),
        ("index_exist", ("ragflow_tenant", "kb1")),
        (
            "search",
            ([], [], {}, [], OrderByExpr(), 0, 10, "ragflow_tenant", ["kb1"]),
        ),
        ("get", ("c1", "ragflow_tenant", ["kb1"])),
        ("insert", ([], "ragflow_tenant", "kb1")),
        ("update", ({"id": "c1"}, {"pagerank_fea": 1}, "ragflow_tenant", "kb1")),
        ("delete", ({"id": "c1"}, "ragflow_tenant", "kb1")),
        ("get_total", (SearchResult(total=0, chunks=[]),)),
        ("get_doc_ids", (SearchResult(total=0, chunks=[]),)),
        (
            "get_fields",
            (SearchResult(total=0, chunks=[]), ["id"]),
        ),
        (
            "get_highlight",
            (SearchResult(total=0, chunks=[]), [], "content_with_weight"),
        ),
        (
            "get_aggregation",
            (SearchResult(total=0, chunks=[]), "doc_type_kwd"),
        ),
        ("sql", ("SELECT 1", 1, "json")),
    ],
)
def test_tc_cfg_319_base_unimplemented_interfaces_fail_fast(method_name, args):
    base = GaussDBConnectionBase(pool=HealthPool([]))

    with pytest.raises(NotImplementedError):
        getattr(base, method_name)(*args)


def test_tc_cfg_615_base_connection_health_reports_healthy_for_a_compatible_gaussdb():
    pool = HealthPool([("GaussDB 8",), ("A",), ("UTF8",), ("UTF8",)])
    health = GaussDBConnectionBase(pool=pool).health()

    assert health["status"] == "healthy"
    assert health["version_comment"] == "GaussDB 8"
    assert health["sql_compatibility"] == "A"
    assert health["server_encoding"] == "UTF8"
    assert health["client_encoding"] == "UTF8"
    assert health["uri"] == pool.masked_uri
    assert pool.checked is True
    assert pool.sql == [
        "SELECT version()",
        "SHOW sql_compatibility",
        "SHOW server_encoding",
        "SHOW client_encoding",
    ]


def test_tc_cfg_602_base_connection_health_reports_healthy_for_ora_compatible_gaussdb():
    pool = HealthPool([("GaussDB 8",), ("ORA",), ("SQL_ASCII",), ("UTF-8",)])
    pool.check_schema_access = Mock(wraps=pool.check_schema_access)
    health = GaussDBConnectionBase(pool=pool).health()

    assert health["status"] == "healthy"
    assert health["version_comment"] == "GaussDB 8"
    assert health["sql_compatibility"] == "ORA"
    assert health["server_encoding"] == "SQL_ASCII"
    assert health["client_encoding"] == "UTF-8"
    assert "RAGFlow clients are forced to UTF8" in health["warning"]
    assert health["uri"] == "u@h:5432/d?schema=public"
    pool.check_schema_access.assert_called_once_with()
    assert pool.checked is True
    assert pool.sql == [
        "SELECT version()",
        "SHOW sql_compatibility",
        "SHOW server_encoding",
        "SHOW client_encoding",
    ]
    assert "password" not in str(health).lower()


def test_tc_cfg_621_base_connection_health_rejects_non_utf8_client_encoding():
    pool = HealthPool([("GaussDB 8",), ("A",), ("UTF8",), ("LATIN1",)])

    health = GaussDBConnectionBase(pool=pool).health()

    assert health["status"] == "unhealthy"
    assert health["client_encoding"] == "LATIN1"
    assert health["error"] == ("unsupported GaussDB client encoding, expected UTF8: client_encoding=LATIN1")


def test_tc_cfg_603_base_connection_health_reports_unhealthy_for_b_compatibility():
    pool = HealthPool([("GaussDB 8",), ("B",)])
    health = GaussDBConnectionBase(pool=pool).health()

    assert health["status"] == "unhealthy"
    assert health["sql_compatibility"] == "B"
    assert health["error"] == "unsupported GaussDB compatibility, expected A/ORA: sql_compatibility=B"
    assert health["uri"] == pool.masked_uri
    assert "postgresql://user:secret@host:19995/postgres" not in str(health)
    assert "password" not in str(health).lower()
    assert "token" not in str(health).lower()


def test_tc_cfg_604_base_connection_health_reports_unhealthy_for_m_compatibility():
    pool = HealthPool([("GaussDB 3.1.0",), ("M",)])
    health = GaussDBConnectionBase(pool=pool).health()

    assert health["status"] == "unhealthy"
    assert health["sql_compatibility"] == "M"
    assert "unsupported GaussDB compatibility" in health["error"]
    assert "sql_compatibility=M" in health["error"]
    assert health["uri"] == pool.masked_uri
    assert "password" not in health["error"].lower()
    assert "token" not in health["error"].lower()
    assert "postgresql://user:secret@host:19995/postgres" not in str(health)


def test_tc_cfg_605_base_connection_health_reports_unhealthy_when_version_is_missing():
    pool = HealthPool([None])
    health = GaussDBConnectionBase(pool=pool).health()

    assert health["status"] == "unhealthy"
    assert health["version_comment"] == "unknown"
    assert health["error"] == "GaussDB version query returned no rows"
    assert health["uri"] == pool.masked_uri
    assert "password" not in str(health).lower()
    assert "postgresql://user:secret@host:19995/postgres" not in str(health)


def test_tc_cfg_606_base_connection_health_redacts_secrets_from_query_failures():
    password = "cfg606-password"
    dsn = f"postgresql://admin:{password}@db.internal:5432/ragflow"
    token = "cfg606-token"
    error = ConnectionError(f"connection failed password={password} dsn={dsn} token={token}")

    pool = HealthPool([error])

    health = GaussDBConnectionBase(pool=pool).health()

    assert health == {
        "status": "unhealthy",
        "uri": "u@h:5432/d?schema=public",
        "version_comment": "unknown",
        "schema": "public",
        "server_encoding": "unknown",
        "client_encoding": "unknown",
        "error": "connection failed password=*** dsn=*** token=***",
    }
    assert pool.checked is True
    assert pool.sql == ["SELECT version()"]
    serialized = json.dumps(health)
    assert password not in serialized
    assert dsn not in serialized
    assert token not in serialized


def test_tc_cfg_607_base_connection_performance_metrics_reports_connected():
    pool = HealthPool([(1,)])

    metrics = GaussDBConnectionBase(pool=pool).get_performance_metrics()

    assert metrics["connection"] == "connected"
    assert isinstance(metrics["latency_ms"], float)
    assert math.isfinite(metrics["latency_ms"])
    assert metrics["latency_ms"] >= 0
    assert metrics["schema"] == "public"
    assert "error" not in metrics
    assert pool.sql == ["SELECT 1"]
    serialized = json.dumps(metrics).lower()
    for forbidden in ("password", "dsn", "api_key", "access_token", "postgresql://"):
        assert forbidden not in serialized


def test_tc_cfg_608_base_connection_performance_metrics_reports_disconnected():
    password = "cfg608-password"
    dsn = f"postgresql://admin:{password}@db.internal:5432/ragflow"
    token = "cfg608-token"
    pool = HealthPool([RuntimeError(f"connection failed password={password} dsn={dsn} token={token}")])
    metrics = GaussDBConnectionBase(pool=pool).get_performance_metrics()

    assert metrics["connection"] == "disconnected"
    assert isinstance(metrics["latency_ms"], float)
    assert math.isfinite(metrics["latency_ms"])
    assert metrics["latency_ms"] >= 0
    assert metrics["error"] == "connection failed password=*** dsn=*** token=***"
    serialized = json.dumps(metrics)
    assert password not in serialized
    assert dsn not in serialized
    assert token not in serialized
    assert pool.sql == ["SELECT 1"]


def test_tc_cfg_609_base_connection_health_does_not_start_background_polling(monkeypatch):
    def fail_thread_creation(*_args, **_kwargs):
        raise AssertionError("background polling must not start")

    monkeypatch.setattr(threading, "Thread", fail_thread_creation)
    monkeypatch.setattr(threading, "Timer", fail_thread_creation)
    pool = HealthPool([("GaussDB 8",), ("A",), ("UTF8",), ("UTF8",)])
    conn = GaussDBConnectionBase(pool=pool)
    before_count = threading.active_count()

    health = conn.health()

    assert health["status"] == "healthy"
    assert threading.active_count() == before_count
    assert pool.sql == [
        "SELECT version()",
        "SHOW sql_compatibility",
        "SHOW server_encoding",
        "SHOW client_encoding",
    ]


def test_tc_cfg_610_base_connection_health_does_not_create_business_tables_or_indexes():
    class TestableGaussDBConnection(GaussDBConnection):
        def create_idx(self, *_args, **_kwargs):
            raise AssertionError("DDL must not run")

        def create_doc_meta_idx(self, *_args, **_kwargs):
            raise AssertionError("DDL must not run")

    pool = HealthPool([("GaussDB 8",), ("A",), ("UTF8",), ("UTF8",)])
    health = TestableGaussDBConnection(pool=pool).health()

    assert health["status"] == "healthy"
    assert pool.sql == [
        "SELECT version()",
        "SHOW sql_compatibility",
        "SHOW server_encoding",
        "SHOW client_encoding",
    ]


def test_tc_ret_008_search_empty_index_names_returns_empty_without_database_call():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    res = conn.search(["id"], [], {}, [], OrderByExpr(), 0, 10, [], ["k1"])

    assert type(res).__name__ == "SearchResult"
    assert res.total == 0
    assert type(res.total) is int
    assert res.chunks == []
    assert conn.search([], [], {}, [], OrderByExpr(), 0, 10, "", None, dataset_ids=["kb1"]) == SearchResult(total=0, chunks=[])
    assert cursor.executed == []


def test_tc_ret_004_search_rejects_mixed_doc_meta_and_chunk_tables_without_database_call():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    with pytest.raises(ValueError, match="^document metadata tables cannot be mixed with chunk search$"):
        conn.search(
            ["id"],
            [],
            {"kb_id": "kb1", "doc_id": ["d1"]},
            [],
            OrderByExpr(),
            0,
            10,
            ["ragflow_doc_meta_tenant", "ragflow_tenant"],
            ["kb1"],
        )

    assert cursor.executed == []


def test_tc_ret_006_search_rejects_missing_kb_boundary_without_database_call():
    cursor = RecordingCursor()
    conn = make_conn(cursor)
    fetch_all = Mock(side_effect=AssertionError("unscoped search must stop before SQL"))
    conn._fetch_all_with_description = fetch_all

    with pytest.raises(ValueError, match="^GaussDB chunk search requires a kb_id boundary$"):
        conn.search(["id"], [], {}, [], OrderByExpr(), 0, 10, "ragflow_t1", [])

    assert cursor.executed == []
    fetch_all.assert_not_called()
    assert conn._scoped_search_condition({"doc_ids": ["d1"]}, ["kb1"]) == {
        "doc_id": ["d1"],
        "kb_id": ["kb1"],
    }


def test_tc_ret_006_empty_doc_ids_keep_kb_scope():
    conn = make_conn(RecordingCursor())

    assert conn._scoped_search_condition({"doc_ids": []}, ["kb1"]) == {"kb_id": ["kb1"]}


def test_tc_ret_007_search_rejects_condition_kb_id_outside_authorized_kbs_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    for index_name in ("ragflow_t1", "ragflow_doc_meta_t1"):
        with pytest.raises(ValueError, match="^condition kb_id must stay within knowledgebase_ids$"):
            conn.search(["id"], [], {"kb_id": "k_other"}, [], None, 0, 10, index_name, ["k1"])

    assert cursor.executed == []


def test_tc_ret_601_search_executes_single_field_aggregation():
    cursor = SequencedCursor(
        [
            (
                [("active", 2), ("inactive", 1)],
                [("value",), ("count",)],
            )
        ]
    )
    conn = make_conn(cursor)

    result = conn.search(
        [],
        [],
        {},
        [],
        OrderByExpr(),
        0,
        10,
        "ragflow_tenant",
        ["kb1"],
        ["doc_type_kwd"],
    )

    assert result == SearchResult(
        total=2,
        chunks=[{"value": "active", "count": 2}, {"value": "inactive", "count": 1}],
    )
    assert conn.get_aggregation(result, "doc_type_kwd") == [("active", 2), ("inactive", 1)]
    assert cursor.executed == [
        (
            'SELECT doc_type_kwd AS value, COUNT(1) AS count FROM "public"."ragflow_tenant" WHERE kb_id IN (%s) AND doc_type_kwd IS NOT NULL GROUP BY value ORDER BY count DESC, value ASC LIMIT %s',
            ["kb1", 1000],
        )
    ]


def test_tc_ret_608_search_rejects_multiple_aggregation_fields_without_partial_query():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    with pytest.raises(ValueError, match="^GaussDB search supports one aggregation field per request$"):
        conn.search(
            [],
            [],
            {},
            [],
            OrderByExpr(),
            0,
            10,
            "ragflow_tenant",
            ["kb1"],
            ["docnm_kwd", "tag_kwd"],
        )

    assert cursor.executed == []


def test_tc_ret_209_search_rejects_non_float_vector_type_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)
    bad = MatchDenseExpr("q_1024_vec", [0.1] * 1024, "byte", "cosine", 10, {"similarity": 0.0})

    with pytest.raises(ValueError, match="unsupported GaussDB vector data type|byte"):
        conn.search(["id"], [], {"kb_id": "k1"}, [bad], OrderByExpr(), 0, 10, "ragflow_t1", ["k1"])

    assert cursor.executed == []


def test_tc_ret_207_search_propagates_ann_unavailable_error_without_fallback():
    conn = make_conn(RecordingCursor())
    calls = []

    def fail_fetch_all_with_description(sql, params):
        calls.append((sql, params))
        raise GaussDBError("ANN gsdiskann index unavailable")

    conn._fetch_all_with_description = fail_fetch_all_with_description
    dense = MatchDenseExpr("q_1024_vec", [0.1] * 1024, "float", "cosine", 10, {"similarity": 0.0})

    with pytest.raises(GaussDBError, match="ANN|gsdiskann|index"):
        conn.search(["id"], [], {"kb_id": "k1"}, [dense], OrderByExpr(), 0, 10, "ragflow_t1", ["k1"])

    assert len(calls) == 1
    assert "q_1024_vec" in calls[0][0]


def test_tc_ret_208_search_propagates_missing_vector_column_error():
    conn = make_conn(RecordingCursor())
    calls = []

    def fail_fetch_all_with_description(sql, params):
        calls.append((sql, params))
        raise psycopg2.errors.UndefinedColumn("column q_512_vec does not exist")

    conn._fetch_all_with_description = fail_fetch_all_with_description
    dense = MatchDenseExpr("q_512_vec", [0.1] * 512, "float", "cosine", 10, {"similarity": 0.0})

    with pytest.raises(psycopg2.errors.UndefinedColumn, match="q_512_vec"):
        conn.search(["id"], [], {"kb_id": "k1"}, [dense], OrderByExpr(), 0, 10, "ragflow_t1", ["k1"])

    assert len(calls) == 1
    assert "q_512_vec" in calls[0][0]


def test_tc_ret_010_multi_table_search_limit_zero_uses_default_collection_limit():
    conn = make_conn(RecordingCursor())
    calls = []
    table_chunks = {
        "ragflow_a": [{"id": f"a-{index:05d}"} for index in range(5001)],
        "ragflow_b": [{"id": f"b-{index:05d}"} for index in range(5000)],
    }

    def fake_search_chunk_table(**kwargs):
        calls.append((kwargs["table"], kwargs["limit"]))
        chunks = table_chunks[kwargs["table"]]
        return SearchResult(total=len(chunks), chunks=chunks)

    conn._search_chunk_table = fake_search_chunk_table

    result = conn.search(["id"], [], {"kb_id": "kb1"}, [], OrderByExpr().asc("id"), 0, 0, ["ragflow_a", "ragflow_b"], ["kb1"])

    assert result.total == 10001
    assert len(result.chunks) == 10000
    assert result.chunks[0]["id"] == "a-00000"
    assert result.chunks[-1]["id"] == "b-04998"
    assert all(chunk["id"] != "b-04999" for chunk in result.chunks)
    assert calls == [("ragflow_a", 10000), ("ragflow_b", 10000)]

    calls.clear()
    offset_result = conn.search(["id"], [], {"kb_id": "kb1"}, [], OrderByExpr().asc("id"), 100, 0, ["ragflow_a", "ragflow_b"], ["kb1"])

    assert offset_result.total == 10001
    assert len(offset_result.chunks) == 9901
    assert offset_result.chunks[0]["id"] == "a-00100"
    assert offset_result.chunks[-1]["id"] == "b-04999"
    assert calls == [("ragflow_a", 10100), ("ragflow_b", 10100)]


def test_tc_wrt_511_update_failure_rolls_back_once_without_commit():
    class FailingCursor(RecordingCursor):
        def execute(self, sql, params=None):
            super().execute(sql, params)
            raise psycopg2.Error("Update failed")

    cursor = FailingCursor()
    conn = make_conn(cursor)

    assert conn.update({"id": "c1"}, {"pagerank_fea": 1}, "ragflow_tenant", "kb1") is False
    assert len(cursor.executed) == 1
    assert conn.pool.conn.rollbacks == 1
    assert conn.pool.conn.commits == 0


def test_tc_wrt_501_update_empty_condition_or_value_returns_false_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    assert conn.update({}, {"pagerank_fea": 1}, "ragflow_tenant", "kb1") is False
    assert conn.update({"id": "c1"}, {}, "ragflow_tenant", "kb1") is False
    assert cursor.executed == []


def test_tc_sql_508_sql_markdown_serializes_bytes_and_json_values():
    cursor = RecordingCursor()
    cursor.description = [("doc_id",), ("payload",), ("raw",)]
    cursor.rows = [("doc1", {"amount": 120}, b"ok")]
    conn = make_conn(cursor)

    result = conn.sql("SELECT doc_id FROM ragflow_tenant WHERE kb_id = 'kb1'", format="markdown")

    assert result == {
        "columns": [
            {"name": "doc_id", "type": "text"},
            {"name": "payload", "type": "text"},
            {"name": "raw", "type": "text"},
        ],
        "rows": [["doc1", '{"amount": 120}', "ok"]],
        "markdown": '|doc_id|payload|raw|\n|---|---|---|\n|doc1|{"amount": 120}|ok|',
    }


def test_tc_sql_502_fetch_all_with_description_sets_statement_timeout_before_business_sql():
    cursor = RecordingCursor()
    cursor.rows = [("doc1",)]
    cursor.description = [("doc_id",)]
    conn = make_conn(cursor)

    rows, description = conn._fetch_all_with_description("SELECT doc_id FROM ragflow_tenant WHERE kb_id = 'kb1'", [], 30000)

    assert rows == [("doc1",)]
    assert description == [("doc_id",)]
    assert cursor.executed[0] == ("SET LOCAL statement_timeout = 30000", [])
    assert cursor.executed[1] == ("SELECT doc_id FROM ragflow_tenant WHERE kb_id = 'kb1'", [])
    assert cursor.closed is True
    assert conn.pool.put_back == [conn.pool.conn]


def test_tc_sql_503_sql_propagates_timeout_from_fetch_all_with_statement_timeout():
    conn = make_conn(RecordingCursor())
    calls = []

    def fail_fetch(sql, params, statement_timeout_ms=None):
        calls.append((sql, params, statement_timeout_ms))
        raise TimeoutError("statement timeout")

    conn._fetch_all_with_description = fail_fetch

    with pytest.raises(TimeoutError, match="statement timeout"):
        conn.sql("SELECT doc_id FROM ragflow_tenant WHERE kb_id = 'kb1'")

    assert calls == [
        (
            "SELECT doc_id FROM \"public\".ragflow_tenant WHERE kb_id = 'kb1' LIMIT 128",
            [],
            30000,
        )
    ]


def test_tc_sql_510_validator_rejection_prevents_database_fetch():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    def fail_fetch(*_args, **_kwargs):
        raise AssertionError("database fetch must not be called")

    conn._fetch_all_with_description = fail_fetch

    with pytest.raises(UnsafeGaussDBSQL, match="SELECT \\* is not allowed"):
        conn.sql("SELECT * FROM ragflow_tenant WHERE kb_id = 'kb1'")

    assert cursor.executed == []
    assert conn.pool.put_back == []


def test_tc_ret_605_get_highlight_uses_alias_and_ignores_empty_rows():
    conn = GaussDBConnection.__new__(GaussDBConnection)
    result = SearchResult(
        total=4,
        chunks=[
            {"id": "c1", "_highlight": "<em>risk</em>"},
            {"id": "c2", "highlight": "audit"},
            {"id": "c3"},
            {"highlight": "missing id"},
        ],
    )

    assert conn.get_highlight(result, ["risk"], "content_with_weight") == {"c1": "<em>risk</em>", "c2": "audit"}
    assert (
        conn.get_highlight(
            SearchResult(total=2, chunks=[{"id": "c3"}, {"id": "c4", "_highlight": None}]),
            ["risk"],
            "content_with_weight",
        )
        == {}
    )


def test_tc_ret_506_get_fields_filters_none_values_per_chunk():
    conn = GaussDBConnection.__new__(GaussDBConnection)
    res = SearchResult(
        total=3,
        chunks=[
            {"id": "c1", "field1": "value1", "field2": None},
            {"id": "c2", "field1": None, "field2": "value2"},
            {"field1": "missing id", "field2": "ignored"},
        ],
    )

    assert conn.get_fields(res, ["field1", "field2"]) == {
        "c1": {"field1": "value1"},
        "c2": {"field2": "value2"},
    }


def test_tc_ret_507_get_scores_defaults_missing_score_to_zero():
    conn = GaussDBConnection.__new__(GaussDBConnection)
    res = SearchResult(
        total=4,
        chunks=[{"id": "c1", "_score": 0.8}, {"id": "c2", "_score": None}, {"id": "c3"}, {"_score": 1.0}],
    )

    assert conn.get_scores(res) == {"c1": 0.8, "c2": 0.0, "c3": 0.0}


def test_tc_ret_509_invalid_vector_is_removed_before_get_fields_returns_values():
    conn = GaussDBConnection.__new__(GaussDBConnection)
    chunk = conn._row_to_chunk(
        ("c1", "k1", "[0.0,0.0]", False, "hello"),
        [("id",), ("kb_id",), ("q_2_vec",), ("q_2_vec_valid",), ("content_with_weight",)],
    )
    fields = conn.get_fields(SearchResult(total=1, chunks=[chunk]), ["q_2_vec", "content_with_weight"])

    assert chunk == {"id": "c1", "kb_id": "k1", "q_2_vec_valid": False, "content_with_weight": "hello"}
    assert fields == {"c1": {"content_with_weight": "hello"}}


def test_tc_ret_505_get_doc_ids_filters_missing_ids_in_stable_order():
    conn = GaussDBConnection.__new__(GaussDBConnection)
    result = SearchResult(
        total=3,
        chunks=[
            {"id": "c2", "kb_id": "kb1"},
            {"kb_id": "kb1"},
            {"id": "c1", "kb_id": "kb1"},
        ],
    )

    assert conn.get_doc_ids(result) == ["c2", "c1"]


def test_tc_ret_510_search_returns_search_result_with_total_and_chunks_shape():
    cursor = RecordingCursor()
    cursor.description = [("id",), ("kb_id",), ("_score",), ("__total",)]
    cursor.rows = [("c1", "k1", 0.7, 2), ("c2", "k1", 0.5, 2)]
    conn = make_conn(cursor)

    res = conn.search(["id", "kb_id"], [], {"kb_id": "k1"}, [], OrderByExpr(), 0, 10, "ragflow_t1", ["k1"])

    assert type(res).__name__ == "SearchResult"
    assert res.total == 2
    assert isinstance(res.total, int)
    assert res.chunks == [{"id": "c1", "kb_id": "k1", "_score": 0.7}, {"id": "c2", "kb_id": "k1", "_score": 0.5}]
    assert conn.get_total(SearchResult(total="3", chunks=[])) == 3


def test_tc_mgf_713_fetch_metadata_doc_ids_returns_empty_for_empty_scope_or_filter():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    assert conn.fetch_metadata_doc_ids("ragflow_doc_meta_tenant", [], "meta_fields ? 'a'", [], 10) == []
    assert conn.fetch_metadata_doc_ids("ragflow_doc_meta_tenant", ["kb1"], "", [], 10) == []
    assert cursor.executed == []
    assert conn.pool.get_calls == 0
    assert conn.pool.put_back == []
    assert conn.pool.conn.commits == 0


def test_tc_mgf_811_fetch_metadata_doc_ids_preserves_probe_limit_and_row_shapes():
    cursor = RecordingCursor()
    cursor.rows = [{"id": "doc1"}, {"id": None}, ("doc2",), []]
    conn = make_conn(cursor)

    result = conn.fetch_metadata_doc_ids("ragflow_doc_meta_tenant", ["kb1"], "meta_fields ? 'a'", [], 4)

    assert result == ["doc1", "doc2"]
    assert cursor.executed == [
        (
            'SELECT id FROM "public"."ragflow_doc_meta_tenant" WHERE kb_id IN (%s) AND (meta_fields ? \'a\') ORDER BY id LIMIT %s',
            ["kb1", 4],
        )
    ]
    assert cursor.closed is True
    assert conn.pool.put_back == [conn.pool.conn]
    assert conn.pool.conn.commits == 0
    assert conn.pool.conn.rollbacks == 0


def test_tc_ret_003_search_doc_meta_without_kb_scope_fails_before_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    with pytest.raises(ValueError, match="^GaussDB document metadata search requires a kb_id boundary$"):
        conn.search(
            ["id", "kb_id"],
            [],
            {},
            [],
            OrderByExpr(),
            0,
            10,
            "ragflow_doc_meta_tenant",
            [],
        )

    assert cursor.executed == []
    assert conn.pool.get_calls == 0
    assert conn.pool.put_back == []
    assert conn.pool.conn.commits == 0


def test_tc_ret_604_search_chunk_aggregation_ignores_null_values_and_missing_counts():
    cursor = SequencedCursor([([(None, 2), ("risk", None)], [("value",), ("count",)])])
    conn = make_conn(cursor)

    result = conn.search([], [], {}, [], OrderByExpr(), 0, 10, "ragflow_tenant", ["kb1"], ["docnm_kwd"])

    assert result == SearchResult(total=1, chunks=[{"value": "risk", "count": 0}])


def test_tc_ret_607_search_chunk_aggregation_returns_value_counts_for_jsonb_array_fields():
    cursor = SequencedCursor([([("tag1", 2), ("tag2", 1)], [("value",), ("count",)])])
    conn = make_conn(cursor)

    result = conn.search([], [], {}, [], OrderByExpr(), 0, 10, "ragflow_tenant", ["kb1"], ["tag_kwd"])

    assert result == SearchResult(total=2, chunks=[{"value": "tag1", "count": 2}, {"value": "tag2", "count": 1}])
    assert len(cursor.executed) == 1
    sql, params = cursor.executed[0]
    assert "jsonb_array_elements_text(COALESCE(tag_kwd, '[]'::jsonb))" in sql
    assert "LATERAL" not in sql.upper()
    assert params == ["kb1", 1000]


def test_tc_ret_607_search_chunk_aggregation_reraises_non_jsonb_array_builder_errors():
    class FailingAggregationBuilder:
        def build_aggregation_sql(self, **_kwargs):
            raise ValueError("unsupported aggregation field")

    cursor = RecordingCursor()
    conn = make_conn(cursor)
    conn._search_builder = lambda: FailingAggregationBuilder()

    with pytest.raises(ValueError, match="unsupported aggregation field"):
        conn.search([], [], {}, [], OrderByExpr(), 0, 10, "ragflow_tenant", ["kb1"], ["docnm_kwd"])

    assert cursor.executed == []


def test_tc_mgf_713_fetch_metadata_doc_ids_rejects_chunk_table():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    with pytest.raises(ValueError, match="document metadata"):
        conn.fetch_metadata_doc_ids("ragflow_tenant", ["kb1"], "TRUE", [], 10)

    assert cursor.executed == []
    assert conn.pool.get_calls == 0
    assert conn.pool.put_back == []
    assert conn.pool.conn.commits == 0


@pytest.mark.parametrize(
    ("delta", "min_w", "max_w"),
    [(5, 0, 100), (-5, 0, 100), (200, -10, 120), (-200, -10, 120)],
)
def test_tc_wrt_801_adjust_chunk_pagerank_repeated_calls_use_atomic_sql(delta, min_w, max_w):
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)

    first = conn.adjust_chunk_pagerank_fea("c1", "ragflow_tenant", "kb1", delta, min_w=min_w, max_w=max_w)
    second = conn.adjust_chunk_pagerank_fea("c1", "ragflow_tenant", "kb1", delta, min_w=min_w, max_w=max_w)

    expected = (
        'UPDATE "public"."ragflow_tenant" SET pagerank_fea = GREATEST(%s, LEAST(%s, COALESCE(pagerank_fea, 0) + %s)) WHERE kb_id = %s AND id = %s',
        [min_w, max_w, delta, "kb1", "c1"],
    )
    assert first is second is True
    assert cursor.executed == [expected, expected]
    assert conn.pool.conn.commits == 2


def test_tc_wrt_804_adjust_chunk_pagerank_ignores_extra_identity_parameters():
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)

    result = conn.adjust_chunk_pagerank_fea(
        "c1",
        "ragflow_tenant",
        "kb1",
        5,
        row_id="row-ignored",
        extra_param="ignored",
    )

    sql, params = cursor.executed[-1]
    assert result is True
    assert "WHERE kb_id = %s AND id = %s" in sql
    assert "row_id" not in sql
    assert "extra_param" not in sql
    assert params == [0, 100, 5, "kb1", "c1"]


def test_tc_wrt_805_adjust_chunk_pagerank_requires_chunk_and_dataset_scope_without_sql():
    cursor = RecordingCursor()
    conn = make_conn(cursor)

    assert conn.adjust_chunk_pagerank_fea("", "ragflow_tenant", "kb1", 1) is False
    assert conn.adjust_chunk_pagerank_fea("c1", "ragflow_tenant", "", 1) is False
    assert cursor.executed == []


@pytest.mark.parametrize(
    ("field", "first_value", "second_value", "expected_ids", "expected_order"),
    [
        (
            "page_num_int",
            "[2]",
            "[1]",
            ["c-second", "c-first"],
            "COALESCE((page_num_int #>> '{0}')::int, 100000000) ASC",
        ),
        (
            "position_int",
            "{}",
            "[[0, 0, 0, 9]]",
            ["c-second", "c-first"],
            "COALESCE((position_int #>> '{0,3}')::int, 100000000) ASC",
        ),
        (
            "top_int",
            None,
            "[2]",
            ["c-second", "c-first"],
            "COALESCE((top_int #>> '{0}')::int, 100000000) ASC",
        ),
        ("pagerank_fea", 2, 1, ["c-second", "c-first"], "ORDER BY pagerank_fea ASC"),
        ("doc_id", None, "a", ["c-first", "c-second"], "ORDER BY doc_id ASC"),
    ],
)
def test_tc_ret_512_multi_table_search_applies_safe_compatibility_sorting(
    field,
    first_value,
    second_value,
    expected_ids,
    expected_order,
):
    description = [("id",), ("kb_id",), (field,), ("_score",), ("__total",)]
    cursor = SequencedCursor(
        [
            ([("c-first", "kb1", first_value, 0.0, 1)], description),
            ([("c-second", "kb1", second_value, 0.0, 1)], description),
        ]
    )
    conn = make_conn(cursor)
    order_by = type("Order", (), {"fields": [(field, False)]})()

    result = conn.search(
        ["id", field],
        [],
        {},
        [],
        order_by,
        0,
        10,
        ["ragflow_tenant_a", "ragflow_tenant_b"],
        ["kb1"],
    )

    assert result.total == 2
    assert [chunk["id"] for chunk in result.chunks] == expected_ids
    assert all(expected_order in sql for sql, _params in cursor.executed)


def test_tc_wrt_504_update_overwrites_metadata_and_derives_title_with_scoped_parameters():
    cursor = RecordingCursor()
    cursor.rowcount = 1
    conn = make_conn(cursor)
    metadata = {"_title": "New", "x": 1}

    updated = conn.update(
        {"id": "c1", "kb_id": "k1"},
        {"metadata": metadata},
        "ragflow_tenant",
        "k1",
    )

    assert updated is True
    assert cursor.executed == [
        (
            'UPDATE "public"."ragflow_tenant" SET metadata = %s::jsonb, docnm_kwd = %s WHERE id = %s AND kb_id = %s',
            ['{"_title": "New", "x": 1}', "New", "c1", "k1"],
        )
    ]
    sql = cursor.executed[0][0]
    assert all(value not in sql for value in ("New", "c1", "k1"))
    assert json.dumps(metadata, ensure_ascii=False) not in sql
    assert all(keyword not in sql for keyword in ("DELETE ", "DROP ", "CREATE ", "ALTER "))

    group_sql, group_params = conn._build_set_clause(
        {"metadata": {"_group_id": "g-new"}},
        is_meta=False,
        condition={"id": "c1", "kb_id": "k1"},
    )
    meta_sql, meta_params = conn._build_set_clause(
        {"meta_fields": {"status": "open"}},
        is_meta=True,
        condition={"id": "doc1", "kb_id": "k1"},
    )
    assert group_sql == "metadata = %s::jsonb, group_id = %s"
    assert group_params == ['{"_group_id": "g-new"}', "g-new"]
    assert meta_sql == "meta_fields = %s::jsonb"
    assert meta_params == ['{"status": "open"}']


def test_tc_wrt_608_delete_exists_stays_within_kb_scope():
    cursor = RecordingCursor()
    cursor.rowcount = 2
    conn = make_conn(cursor)

    deleted = conn.delete({"exists": "doc_id"}, "ragflow_tenant", "kb1")

    assert deleted == 2
    assert cursor.executed == [('DELETE FROM "public"."ragflow_tenant" WHERE doc_id IS NOT NULL AND kb_id = %s', ["kb1"])]
    assert all(" WHERE " in sql and "kb_id = %s" in sql for sql, _params in cursor.executed)


def test_tc_ret_804_parse_fusion_vector_weight_uses_last_weight_value():
    assert parse_fusion_vector_weight(FusionExpr("weighted_sum", 10, {"weights": "0.7,0.3"})) == 0.3


def test_tc_ret_805_parse_fusion_vector_weight_returns_none_for_missing_or_bad_weights():
    assert parse_fusion_vector_weight(FusionExpr("weighted_sum", 10, None)) is None
    assert parse_fusion_vector_weight(FusionExpr("weighted_sum", 10, {})) is None
    assert parse_fusion_vector_weight(FusionExpr("weighted_sum", 10, {"weights": ""})) is None
    assert parse_fusion_vector_weight(FusionExpr("weighted_sum", 10, {"weights": "abc"})) is None
    assert parse_fusion_vector_weight(FusionExpr("weighted_sum", 10, {"weights": "0.7,bad"})) is None


def test_tc_ret_806_parse_match_expressions_applies_gaussdb_vector_weight_defaults(monkeypatch):
    monkeypatch.setattr(gaussdb_conn_module, "_tokenize_query_terms", lambda query: str(query).split())
    conn = make_conn(RecordingCursor())
    text = MatchTextExpr(["content_with_weight"], "hello", 10, {"original_query": "hello", "minimum_should_match": 0.3})
    dense = MatchDenseExpr("q_4_vec", [0.1, 0.2, 0.3, 0.4], "float", "cosine", 10, {"similarity": 0.0})
    fusion = FusionExpr("weighted_sum", 6, {"weights": "0.7,0.3"})

    text_only = conn._parse_match_expressions([text], rank_feature=None)
    vector_only = conn._parse_match_expressions([dense], rank_feature=None)
    hybrid_default = conn._parse_match_expressions([text, dense], rank_feature=None)
    hybrid_explicit = conn._parse_match_expressions([text, dense, fusion], rank_feature={"pagerank_fea": 7})

    assert text_only["vector_weight"] == 0.0
    assert text_only["minimum_should_match"] == 0.3
    assert vector_only["vector_weight"] == 1.0
    assert vector_only["minimum_should_match"] is None
    assert hybrid_default["vector_weight"] == 0.5
    assert {text_only["pagerank_weight"], vector_only["pagerank_weight"], hybrid_default["pagerank_weight"]} == {10.0}
    assert hybrid_explicit["vector_weight"] == 0.3
    assert hybrid_explicit["topn"] == 6
    assert hybrid_explicit["pagerank_weight"] == 7.0


def test_tc_ret_110_parse_match_tokenizes_original_query(monkeypatch):
    queries = []

    def tokenize(query):
        queries.append(query)
        return ["高斯", "数据库", "端", "到", "端", "检索"]

    monkeypatch.setattr(gaussdb_conn_module, "_tokenize_query_terms", tokenize)
    conn = make_conn(RecordingCursor())
    text = MatchTextExpr(["content_ltks"], "backend-specific syntax", 10, {"original_query": "高斯数据库端到端检索"})

    parsed = conn._parse_match_expressions([text], rank_feature=None)

    assert queries == ["高斯数据库端到端检索"]
    assert parsed["keywords"] == ["高斯", "数据库", "端", "到", "端", "检索"]


def test_tc_wrt_905_vector_literal_rejects_dimension_mismatch():
    with pytest.raises(ValueError, match="vector dimension mismatch: expected 3, got 2"):
        vector_literal([0.1, 0.2], 3)


def test_tc_wrt_905_vector_literal_accepts_native_and_json_vectors():
    assert vector_literal([0.1, 0.2, 0.3], 3) == "[0.1,0.2,0.3]"
    assert vector_literal("[0.1, 0.2, 0.3]", 3) == "[0.1,0.2,0.3]"


def test_tc_wrt_906_zero_vector_literal_returns_all_zero_vector_string():
    result = zero_vector_literal(3)

    assert result == "[0,0,0]"
    assert isinstance(result, str)


def test_tc_wrt_907_ordered_columns_unifies_heterogeneous_rows():
    rows = [
        {"q_1024_vec": "[0,0]", "id": "c1"},
        {"important_kwd": '["risk"]', "id": "c2"},
    ]

    assert ordered_columns(rows) == ["id", "important_kwd", "q_1024_vec"]


def test_tc_wrt_908_decode_column_value_deserializes_jsonb_and_vector_strings():
    assert decode_column_value("chunk_data", '{"amount":120,"active":true}') == {"amount": 120, "active": True}
    assert decode_column_value("chunk_data", {"amount": 120}) == {"amount": 120}
    assert decode_column_value("q_3_vec", "[0.1, 0.2, 0.3]") == [0.1, 0.2, 0.3]
    assert decode_column_value("q_3_vec", [0.1, 0.2, 0.3]) == [0.1, 0.2, 0.3]
    assert parse_json_dict('{"amount":120}') == {"amount": 120}
    assert parse_json_dict("not-json") == {}
    assert parse_json_dict("[1,2]") == {}
    assert parse_json_dict(None) == {}


def test_tc_wrt_908_decode_column_value_rejects_malformed_jsonb_readback():
    with pytest.raises(ValueError, match="^invalid JSONB value for chunk_data$") as exc_info:
        decode_column_value("chunk_data", "not-json")

    assert exc_info.type is ValueError


def test_tc_wrt_908_decode_column_value_rejects_malformed_vector_readback():
    with pytest.raises(ValueError, match="^invalid vector value for q_3_vec$") as exc_info:
        decode_column_value("q_3_vec", "not-vector")

    assert exc_info.type is ValueError


def test_tc_wrt_908_vector_literal_rejects_invalid_string():
    with pytest.raises(ValueError, match="^invalid vector literal$") as exc_info:
        vector_literal("not-json", 2)

    assert exc_info.type is ValueError

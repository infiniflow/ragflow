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
from datetime import datetime
from decimal import Decimal

import numpy as np
import pytest

from common.doc_store.doc_store_base import FusionExpr, MatchDenseExpr, MatchTextExpr, OrderByExpr
from common.doc_store.gaussdb_conn_base import InvalidGaussDBObjectName
from memory.utils import gaussdb_conn as gaussdb_mem
from memory.utils.gaussdb_conn import GaussDBMemoryConnection, GaussDBMemoryDDLBuilder, SearchResult


class FakePool:
    masked_uri = "unit@example:19995/db?schema=public"
    resolved_schema = "public"

    def check_schema_access(self):
        return None


def make_conn() -> GaussDBMemoryConnection:
    return GaussDBMemoryConnection(pool=FakePool())


def test_physical_table_name_depends_only_on_index_name():
    builder = GaussDBMemoryDDLBuilder(schema="public")

    table = builder.physical_table_name("memory_tenant_a")

    assert table == builder.physical_table_name("memory_tenant_a")
    assert table != builder.physical_table_name("memory_tenant_b")
    assert table.startswith("ragflow_mem_")
    assert "memory_id" not in table
    assert len(table) <= 63


def test_memory_table_ddl_uses_oracle_compatible_columns_and_ustore():
    builder = GaussDBMemoryDDLBuilder(schema="public")
    ddl = builder.build_memory_table_ddl("ragflow_mem_abc")

    assert 'CREATE TABLE IF NOT EXISTS "public"."ragflow_mem_abc"' in ddl
    assert "message_id NUMBER(19) NOT NULL" in ddl
    assert "message_type_kwd VARCHAR2(64)" in ddl
    assert "valid_at TIMESTAMP" in ddl
    assert "content_ltks TEXT" in ddl
    assert "WITH (storage_type=USTORE)" in ddl


def test_memory_indexes_use_ugin_and_gsdiskann_with_empty_marker():
    builder = GaussDBMemoryDDLBuilder(schema="public")

    fulltext = builder.build_fulltext_ugin_ddl("ragflow_mem_abc")
    vector_columns = "\n".join(builder.build_vector_column_ddls("ragflow_mem_abc", 3))
    empty_index = builder.build_vector_empty_index_ddl("ragflow_mem_abc", 3)
    diskann = builder.build_diskann_index_ddl("ragflow_mem_abc", 3)

    assert "USING ugin (to_tsvector('simple', tokenized_content_ltks))" in fulltext
    assert "q_3_vec floatvector(3)" in vector_columns
    assert "DEFAULT (array_fill(0, ARRAY[3])::text::floatvector(3)) NOT NULL" in vector_columns
    assert "q_3_vec_empty BOOLEAN DEFAULT TRUE NOT NULL" in vector_columns
    assert "(memory_id, q_3_vec_empty)" in empty_index
    assert "USING gsdiskann (q_3_vec cosine)" in diskann


def test_merge_sql_uses_dual_and_resets_other_vector_dimensions():
    conn = make_conn()
    row, dim = conn._message_to_row(
        {
            "id": "mem1_1",
            "message_id": 1,
            "message_type": "raw",
            "source_id": 0,
            "memory_id": "mem1",
            "agent_id": "agent1",
            "session_id": "session1",
            "content": "hello",
            "valid_at": "2026-06-29 12:00:00",
            "status": True,
            "content_embed": [0.1, 0.2, 0.3],
        },
        "mem1",
    )

    sql, params = conn._build_merge_sql("ragflow_mem_abc", dim, [3, 5], [row])

    assert sql.startswith('MERGE INTO "public"."ragflow_mem_abc"')
    assert "FROM dual" in sql
    assert "ON CONFLICT" not in sql
    assert "q_3_vec = s.q_3_vec" in sql
    assert "q_3_vec_empty = FALSE" in sql
    assert "q_5_vec = '[0,0,0,0,0]'::floatvector" in sql
    assert "q_5_vec_empty = TRUE" in sql
    assert params[0][0] == "mem1_1"
    assert params[0][-1] == "[0.1,0.2,0.3]"


def test_where_clause_skips_empty_string_filters_and_adds_memory_boundary():
    conn = make_conn()

    where_sql, params = conn._build_where_clause(
        {"agent_id": "", "session_id": "s1", "status": 1},
        memory_ids=["mem1", "mem2"],
        hide_forgotten=True,
    )

    assert "memory_id IN (%s, %s)" in where_sql
    assert "forget_at IS NULL" in where_sql
    assert "session_id = %s" in where_sql
    assert "status_int = %s" in where_sql
    assert "agent_id" not in where_sql
    assert params == ["mem1", "mem2", "s1", 1]


def test_delete_empty_message_id_list_is_noop():
    conn = make_conn()

    assert conn.delete({"message_id": []}, "memory_tenant", "mem1") == 0


def test_get_sql_does_not_add_fetch_first_one(monkeypatch):
    conn = make_conn()
    captured = {}

    monkeypatch.setattr(conn, "_table_exists", lambda _table: True)
    monkeypatch.setattr(conn, "_vector_columns_for_select", lambda _table: [])

    def fake_fetch_one_with_description(sql, params):
        captured["sql"] = sql
        captured["params"] = params
        return None, []

    monkeypatch.setattr(conn, "_fetch_one_with_description", fake_fetch_one_with_description)

    assert conn.get("mem1_1", "memory_tenant", ["mem1"]) is None
    assert "FETCH FIRST 1 ROWS ONLY" not in captured["sql"]
    assert "WHERE id = %s" in captured["sql"]
    assert captured["params"] == ["mem1_1"]


def test_catalog_existence_checks_do_not_add_fetch_first_one(monkeypatch):
    conn = make_conn()
    captured = []

    def fake_fetch_one(sql, params):
        captured.append((sql, params))
        return (1,)

    monkeypatch.setattr(conn, "_fetch_one", fake_fetch_one)

    assert conn._table_exists("ragflow_mem_abc")
    assert conn._column_exists("ragflow_mem_abc", "q_3_vec")
    assert all("FETCH FIRST 1 ROWS ONLY" not in sql for sql, _params in captured)


def test_sql_returns_none_without_executing_external_sql():
    conn = make_conn()

    assert conn.sql("SELECT 1") is None


def test_update_content_set_refreshes_tokenized_content():
    conn = make_conn()

    set_sql, params = conn._build_update_set("ragflow_mem_abc", "memory_tenant", "mem1", {"content": "hello world"})

    assert "content_ltks = %s" in set_sql
    assert "tokenized_content_ltks = %s" in set_sql
    assert params[0] == "hello world"
    assert params[1]


def test_remove_content_clears_original_and_tokenized_content():
    conn = make_conn()

    set_sql, params = conn._build_update_set("ragflow_mem_abc", "memory_tenant", "mem1", {"remove": "content"})

    assert set_sql == "content_ltks = NULL, tokenized_content_ltks = NULL"
    assert params == []


def test_vector_search_sql_filters_empty_vectors_and_uses_threshold():
    conn = make_conn()
    parsed = conn._parse_match_expressions(
        [
            MatchDenseExpr(
                "q_3_vec",
                [0.1, 0.2, 0.3],
                "float",
                "cosine",
                topn=5,
                extra_options={"similarity": 0.62},
            )
        ]
    )

    sql, params = conn._build_vector_search_sql(
        table="ragflow_mem_abc",
        select_fields=["message_id", "content"],
        condition={"status": 1},
        parsed=parsed,
        offset=0,
        limit=10,
        memory_ids=["mem1"],
        hide_forgotten=True,
    )

    assert "1 - (q_3_vec <+> %s::floatvector(3)) AS _score" in sql
    assert "q_3_vec_empty = FALSE" in sql
    assert ">= %s" in sql
    assert "ORDER BY q_3_vec <+> %s::floatvector(3) ASC" in sql
    assert "OFFSET %s LIMIT %s" in sql
    assert "FETCH NEXT" not in sql
    assert params == ["[0.1,0.2,0.3]", "mem1", 1, "[0.1,0.2,0.3]", 0.62, "[0.1,0.2,0.3]", 0, 5]


def test_fulltext_search_sql_uses_tokenized_original_query_and_plain_tsquery():
    conn = make_conn()
    parsed = conn._parse_match_expressions([MatchTextExpr(["content_ltks"], "ignored", 8, extra_options={"original_query": "contract risk"})])

    sql, params = conn._build_fulltext_search_sql(
        table="ragflow_mem_abc",
        select_fields=["message_id", "content"],
        highlight_fields=[],
        condition={"status": 1},
        parsed=parsed,
        offset=0,
        limit=10,
        memory_ids=["mem1"],
        hide_forgotten=True,
    )

    assert "to_tsvector('simple', tokenized_content_ltks)" in sql
    assert "plainto_tsquery('simple', %s)" in sql
    assert "ts_rank" in sql
    assert "OFFSET %s LIMIT %s" in sql
    assert "FETCH NEXT" not in sql
    assert params[0] == "contract risk"
    assert params[-2:] == [0, 8]


def test_parse_match_expression_tokenizes_fulltext_query_like_stored_content():
    conn = make_conn()

    parsed = conn._parse_match_expressions([MatchTextExpr(["content_ltks"], "ignored", 8, extra_options={"original_query": "coriander"})])

    assert parsed["text_query"] == "coriand"


def test_fusion_sql_uses_fulltext_candidates_then_vector_filter():
    conn = make_conn()
    parsed = conn._parse_match_expressions(
        [
            MatchTextExpr(["content_ltks"], "ignored", 20, extra_options={"original_query": "risk"}),
            MatchDenseExpr("q_3_vec", [0.1, 0.2, 0.3], "float", "cosine", topn=10, extra_options={"similarity": 0.5}),
            FusionExpr("weighted_sum", 10, {"weights": "0.3,0.7"}),
        ]
    )

    sql, params = conn._build_fusion_search_sql(
        table="ragflow_mem_abc",
        select_fields=["message_id", "content"],
        condition={"status": 1},
        parsed=parsed,
        offset=0,
        limit=10,
        memory_ids=["mem1"],
        hide_forgotten=True,
    )

    assert "WITH fulltext_results AS" in sql
    assert "ORDER BY relevance DESC LIMIT %s" in sql
    assert "q_3_vec_empty = FALSE" in sql
    assert "relevance * %s + (1 - (q_3_vec <+> %s::floatvector(3))) * %s AS _score" in sql
    assert "OFFSET %s LIMIT %s" in sql
    assert "FETCH FIRST" not in sql
    assert "FETCH NEXT" not in sql
    assert params[0] == "risk"
    assert 0.7 in params
    assert 0.30000000000000004 in params


def test_get_fields_restores_content_embed_by_empty_marker():
    conn = make_conn()
    result = SearchResult(
        total=1,
        messages=[
            {
                "id": "mem1_1",
                "message_id": 1,
                "message_type_kwd": "raw",
                "source_id": 0,
                "memory_id": "mem1",
                "agent_id": "agent1",
                "session_id": "session1",
                "valid_at": "2026-06-29 12:00:00",
                "status_int": 1,
                "content_ltks": "hello",
                "q_3_vec": "[0,0,0]",
                "q_3_vec_empty": True,
                "q_5_vec": "[1,2,3,4,5]",
                "q_5_vec_empty": False,
            }
        ],
    )

    fields = conn.get_fields(result, ["message_id", "message_type", "content", "content_embed"])

    assert fields["mem1_1"]["message_id"] == 1
    assert fields["mem1_1"]["message_type"] == "raw"
    assert fields["mem1_1"]["content"] == "hello"
    assert fields["mem1_1"]["content_embed"] == [1.0, 2.0, 3.0, 4.0, 5.0]


def test_filter_search_uses_order_by_and_offset_limit_syntax():
    conn = make_conn()
    order_by = OrderByExpr().desc("valid_at")

    sql, params = conn._build_filter_search_sql(
        table="ragflow_mem_abc",
        select_fields=["message_id", "content"],
        condition={},
        order_by=order_by,
        offset=20,
        limit=10,
        memory_ids=["mem1"],
        hide_forgotten=False,
    )

    assert "ORDER BY valid_at DESC" in sql
    assert "OFFSET %s LIMIT %s" in sql
    assert "FETCH NEXT" not in sql
    assert params == ["mem1", 20, 10]


def test_sort_messages_uses_numeric_keys_for_message_id():
    conn = make_conn()
    order_by = OrderByExpr().desc("message_id")

    messages = [
        {"id": "mem_b_99", "message_id": 99},
        {"id": "mem_a_100", "message_id": 100},
    ]

    sorted_messages = conn._sort_messages(messages, order_by, has_match=False)

    assert [message["message_id"] for message in sorted_messages] == [100, 99]


def test_health_reuses_gaussdb_base_health(monkeypatch):
    conn = make_conn()
    monkeypatch.setattr(conn, "_query_version", lambda: "GaussDB unit")
    monkeypatch.setattr(conn, "_query_sql_compatibility", lambda: "PG")

    health = conn.health()

    assert "health" not in GaussDBMemoryConnection.__dict__
    assert health["status"] == "unhealthy"
    assert health["sql_compatibility"] == "PG"
    assert "A/ORA" in health["error"]


def make_base_health_conn(monkeypatch, *, server_encoding, client_encoding):
    from common.doc_store.gaussdb_conn_base import GaussDBConnectionBase

    conn = object.__new__(GaussDBConnectionBase)
    conn.masked_uri = "gaussdb://user@db.example:19995/postgres?schema=ragflow_doc"
    conn.resolved_schema = "ragflow_doc"
    monkeypatch.setattr(conn, "_query_version", lambda: "GaussDB unit")
    monkeypatch.setattr(conn, "_query_sql_compatibility", lambda: "A")
    monkeypatch.setattr(conn, "_query_server_encoding", lambda: server_encoding)
    monkeypatch.setattr(conn, "_query_client_encoding", lambda: client_encoding)
    return conn


def test_health_allows_sql_ascii_server_with_utf8_client_warning(monkeypatch):
    conn = make_base_health_conn(monkeypatch, server_encoding="SQL_ASCII", client_encoding="UTF8")

    result = conn.health()

    assert result["status"] == "healthy"
    assert result["server_encoding"] == "SQL_ASCII"
    assert result["client_encoding"] == "UTF8"
    assert "does not validate" in result["warning"]


def test_health_rejects_non_utf8_client(monkeypatch):
    conn = make_base_health_conn(monkeypatch, server_encoding="UTF8", client_encoding="SQL_ASCII")

    result = conn.health()

    assert result["status"] == "unhealthy"
    assert "client_encoding" in result["error"]


def test_health_accepts_utf8_server_and_client_without_warning(monkeypatch):
    conn = make_base_health_conn(monkeypatch, server_encoding="UTF8", client_encoding="UTF8")

    result = conn.health()

    assert result["status"] == "healthy"
    assert "warning" not in result


def test_short_circuit_branches_and_tuple_result_helpers(monkeypatch):
    conn = make_conn()
    result = SearchResult(total=2, messages=[{"id": "doc-1", "content_ltks": "contract risk"}, {"id": ""}])

    assert conn.insert([], "memory_tenant", "mem1") == []
    assert conn.update({}, {"content": "x"}, "memory_tenant", "mem1") is False
    assert conn.update({"id": "doc-1"}, {}, "memory_tenant", "mem1") is False
    assert conn.delete({}, "memory_tenant", "mem1") == 0
    assert conn.delete({"id": ["", None]}, "memory_tenant", "mem1") == 0
    assert conn.get("", "memory_tenant", ["mem1"]) is None
    assert conn.get_total((result, 2)) == 2
    assert conn.get_doc_ids((result, 2)) == ["doc-1"]
    assert conn.get_fields((result, 2), []) == {}
    assert "<em>contract</em>" in conn.get_highlight((result, 2), ["contract"], "content_ltks")["doc-1"]
    assert conn.get_aggregation((result, 2), "content_ltks") == [("contract risk", 1)]

    monkeypatch.setattr(conn, "_table_exists", lambda _table: False)
    assert conn.update({"id": "doc-1"}, {"content": "x"}, "missing", "mem1") is True
    assert conn.delete({"id": ["doc-1"]}, "missing", "mem1") == 0
    assert conn.get("doc-1", "missing", ["mem1"]) is None
    assert conn.get_forgotten_messages(["message_id"], "missing", "mem1") is None
    assert conn.get_missing_field_message(["message_id"], "missing", "mem1", "content") is None

    empty_res, empty_total = conn.search(["message_id"], [], {}, [], OrderByExpr(), 0, 10, "", ["mem1"])
    assert empty_total == 0
    assert empty_res.messages == []
    empty_res, empty_total = conn.search(["message_id"], [], {}, [], OrderByExpr(), 0, 10, "idx", [])
    assert empty_total == 0
    empty_res, empty_total = conn.search(["message_id"], [], {}, [], OrderByExpr(), 0, 0, "idx", ["mem1"])
    assert empty_total == 0
    assert empty_res.messages == []


def test_index_exist_false_when_catalog_is_incomplete(monkeypatch):
    conn = make_conn()
    monkeypatch.setattr(conn, "_table_exists", lambda _table: True)
    monkeypatch.setattr(conn, "_column_names", lambda _table: [])
    assert conn.index_exist("memory_tenant", "mem1") is False

    monkeypatch.setattr(conn, "_column_names", lambda _table: list(gaussdb_mem.BASE_COLUMNS))
    monkeypatch.setattr(conn, "_index_names", lambda _table: [])
    assert conn.index_exist("memory_tenant", "mem1") is False


def test_message_normalization_vector_selection_and_invalid_inputs(caplog):
    conn = make_conn()

    with pytest.raises(InvalidGaussDBObjectName):
        conn.physical_table("")
    assert conn.convert_field_name("content", use_tokenized_content=True) == "tokenized_content_ltks"
    with pytest.raises(ValueError, match="content_embed"):
        conn._message_to_row({"message_id": 1, "content_embed": []}, "mem1")
    with pytest.raises(ValueError, match="memory_id"):
        conn._message_to_row({"message_id": 1, "content_embed": [0.1, 0.2, 0.3]}, None)

    row = {
        "id": "doc-1",
        "q_2_vec": np.array([1, 2]),
        "q_2_vec_empty": False,
        "q_3_vec": "[3,4,5]",
        "q_3_vec_empty": False,
    }
    assert conn._content_embed_from_row(row) == [3.0, 4.0, 5.0]
    assert "multiple non-empty vector dimensions" in caplog.text


def test_insert_normalization_failure_returns_all_batch_ids_without_write(monkeypatch):
    conn = make_conn()
    monkeypatch.setattr(conn, "_table_exists", lambda _table: pytest.fail("database access must not occur"))

    errors = conn.insert(
        [
            {
                "id": "mem1_1",
                "message_id": 1,
                "status": True,
                "content": "first",
                "content_embed": [0.1, 0.2, 0.3],
            },
            {
                "id": "mem1_2",
                "message_id": 2,
                "status": True,
                "content": "second",
                "content_embed": [0.1, 0.2],
            },
        ],
        "memory_tenant",
        "mem1",
    )

    assert errors == ["mem1_1", "mem1_2"]


def test_update_set_covers_remove_vector_empty_embed_numeric_and_time_branches(monkeypatch):
    conn = make_conn()
    monkeypatch.setattr(conn, "_ensure_vector_column_exists", lambda _table, _dim: None)
    monkeypatch.setattr(conn, "_vector_dimensions", lambda _table: [3, 5])

    set_sql, params = conn._build_update_set(
        "ragflow_mem_abc",
        "memory_tenant",
        "mem1",
        {
            "remove": ["q_3_vec", "agent_id"],
            "content_embed": [],
            "message_id": Decimal("7"),
            "agent_id": "",
            "invalid_at": "-",
            "status": True,
        },
    )

    assert "q_3_vec = %s::floatvector(3)" in set_sql
    assert "q_3_vec_empty = TRUE" in set_sql
    assert "agent_id = NULL" in set_sql
    assert "message_id = %s" in set_sql
    assert "invalid_at = %s::timestamp" in set_sql
    assert "status_int = %s" in set_sql
    assert params == ["[0,0,0]", 7, None, None, 1]


def test_column_where_order_and_parse_helpers_cover_edge_cases(monkeypatch):
    conn = make_conn()
    monkeypatch.setattr(conn, "_vector_dimensions", lambda _table: [3])

    parsed = conn._parse_match_expressions(
        [
            MatchDenseExpr("content_embed", np.array([0.1, 0.2]), "float", "cosine", 8, {}),
            FusionExpr("weighted_sum", 6, {"weights": "0.2"}),
        ]
    )
    assert parsed["vector_dim"] == 2
    assert parsed["vector"] == "[0.1,0.2]"
    assert parsed["vector_weight"] == 0.5
    assert parsed["topn"] == 6

    assert conn._select_columns("ragflow_mem_abc", ["id", "_score", "content", "content_embed", "content"]) == [
        "id",
        "content_ltks",
        "q_3_vec",
        "q_3_vec_empty",
    ]
    with pytest.raises(InvalidGaussDBObjectName):
        conn._select_columns("ragflow_mem_abc", ["bad;column"])
    assert conn._build_where_clause({}, memory_ids=[], force_memory_filter=True) == ("1=0", [])
    assert conn._build_where_clause({"message_id": ["", None]}, memory_ids=["mem1"]) == ("memory_id IN (%s)", ["mem1"])
    assert conn._build_order_by(None) == ""

    assert gaussdb_mem.normalize_index_names(" a, ,b ") == ["a", "b"]
    assert gaussdb_mem.normalize_index_names([" a ", "", None]) == ["a", "None"]
    assert gaussdb_mem.clean_list_values(None) == []
    assert gaussdb_mem.clean_list_values("mem1") == ["mem1"]
    assert gaussdb_mem.effective_limit(0, None) == 10000
    assert gaussdb_mem.parse_vector_dim("q_12_vec") == 12
    assert gaussdb_mem.parse_vector_dim("content_embed") is None
    assert gaussdb_mem.vector_literal("[1,2]", 2) == "[1.0,2.0]"
    assert gaussdb_mem.vector_literal("raw", 3) == "raw"
    assert gaussdb_mem.vector_literal(np.array([1, 2]), 2) == "[1.0,2.0]"
    with pytest.raises(ValueError, match="dimension mismatch"):
        gaussdb_mem.vector_literal([1], 2)
    assert gaussdb_mem.parse_vector_value(None) == []
    assert gaussdb_mem.parse_vector_value(np.array([1, 2])) == [1.0, 2.0]
    assert gaussdb_mem.parse_vector_value((1, 2)) == [1.0, 2.0]
    assert gaussdb_mem.parse_vector_value("") == []
    assert gaussdb_mem.parse_vector_value("[1, 2]") == [1.0, 2.0]
    assert gaussdb_mem.parse_vector_value("[]") == []
    assert gaussdb_mem.parse_vector_value("not-json") == []
    assert gaussdb_mem.parse_vector_value('{"x": 1}') == []
    assert gaussdb_mem.zero_vector_literal(3) == "[0,0,0]"
    assert gaussdb_mem.normalize_timestamp("-") is None
    assert gaussdb_mem.normalize_timestamp(datetime(2026, 6, 29, 12, 0, 0)) == "2026-06-29 12:00:00"
    assert gaussdb_mem.format_timestamp(datetime(2026, 6, 29, 12, 0, 0)) == "2026-06-29 12:00:00"
    assert gaussdb_mem.none_if_empty("") is None
    assert gaussdb_mem.to_int_or_none(False) == 0
    assert gaussdb_mem.to_int_or_none(np.int64(3)) == 3
    assert gaussdb_mem.to_int_or_original(None) is None
    assert gaussdb_mem.to_int_or_original(True) == 1
    assert gaussdb_mem.to_int_or_original(np.int64(4)) == 4
    assert gaussdb_mem.to_int_or_original(Decimal("5")) == 5
    assert gaussdb_mem.to_int_or_original("abc") == "abc"
    assert gaussdb_mem.row_value({"x": 1}, "x", 0) == 1
    assert gaussdb_mem.unique_preserve_order(["a", "b", "a"]) == ["a", "b"]
    assert gaussdb_mem.sortable_value(datetime(2026, 6, 29, 12, 0, 0)) == "2026-06-29T12:00:00"
    assert gaussdb_mem.is_maintenance_work_mem_error(RuntimeError("maintenance_work_mem below required")) is True
    assert gaussdb_mem.is_maintenance_work_mem_error(RuntimeError("other error")) is False
    gaussdb_mem.close_cursor(None)


def test_diskann_retry_handles_maintenance_work_mem_errors(monkeypatch):
    conn = make_conn()
    calls = []

    def fail_once(_statements):
        calls.append(_statements)
        if len(calls) == 1:
            raise RuntimeError("maintenance_work_mem below required")

    monkeypatch.setattr(conn, "_execute_statements", fail_once)
    conn._create_diskann_index_with_retry("ragflow_mem_abc", 3)
    assert len(calls) == 2
    assert calls[0][1] == "SET LOCAL maintenance_work_mem = '1GB'"
    assert calls[1][1] == "SET LOCAL maintenance_work_mem = '2GB'"

    def fail_non_maintenance(_statements):
        raise RuntimeError("syntax error")

    monkeypatch.setattr(conn, "_execute_statements", fail_non_maintenance)
    with pytest.raises(RuntimeError, match="syntax error"):
        conn._create_diskann_index_with_retry("ragflow_mem_abc", 3)


def test_diskann_unavailable_propagates_missing_prerequisite(monkeypatch):
    conn = make_conn()

    class UnsupportedVectorIndexError(Exception):
        pgcode = "0A000"

    def unsupported(_statements):
        raise UnsupportedVectorIndexError("The vectordb indexes are supported only by high-order features")

    monkeypatch.setattr(conn, "_execute_statements", unsupported)

    with pytest.raises(UnsupportedVectorIndexError, match="supported only by high-order features"):
        conn._create_diskann_index_with_retry("ragflow_mem_abc", 3)

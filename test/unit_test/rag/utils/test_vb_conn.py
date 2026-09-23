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
Unit tests for Vastbase connection utility functions.
"""

import contextlib
import json
from unittest.mock import MagicMock

import pandas as pd
import pytest
from psycopg2 import sql

import rag.utils.vastbase_conn as vastbase_conn_module
from common.doc_store.doc_store_base import MatchDenseExpr, OrderByExpr
from rag.utils.vastbase_conn import (
    VBConnection,
    _parse_floatvector,
    _text_column,
    concat_dataframes,
    ensure_table_columns,
    equivalent_condition_to_str,
    field_keyword,
    quote_ident,
    select_identifier,
)


def _vb_connection_class():
    return next(cell.cell_contents for cell in VBConnection.__closure__ if isinstance(cell.cell_contents, type))


def _render(composable) -> str:
    """Render psycopg2 composed SQL deterministically without a live connection.

    Composable.as_string() requires a real connection/cursor (the C extension
    type-checks it), so tests walk the composable tree instead.
    """
    if isinstance(composable, sql.Composed):
        return "".join(_render(part) for part in composable.seq)
    if isinstance(composable, sql.SQL):
        return composable.string
    if isinstance(composable, sql.Identifier):
        return '"' + composable.string.replace('"', '""') + '"'
    if isinstance(composable, sql.Literal):
        return repr(composable._wrapped)
    return repr(composable)


class RecordingCursor:
    """Cursor stub that records executed SQL; fetch results come from the owning connection."""

    def __init__(self, conn):
        self._conn = conn
        self.description = conn.description

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def execute(self, query, params=None):
        self._conn.executed.append((query, params))
        error = self._conn.error_on_execute
        if callable(error):
            error = error(query)
        if error is not None:
            raise error

    def fetchone(self):
        return self._conn.fetchone_results.pop(0)

    def fetchall(self):
        return self._conn.fetchall_results.pop(0)


class RecordingConnection:
    """psycopg2 connection stub: records executed SQL plus canned fetch results."""

    def __init__(self, fetchone_results=None, fetchall_results=None, error_on_execute=None, description=None):
        self.executed = []
        self.commits = 0
        self.rollbacks = 0
        self.fetchone_results = list(fetchone_results or [])
        self.fetchall_results = list(fetchall_results or [])
        self.error_on_execute = error_on_execute
        self.description = description

    def cursor(self):
        return RecordingCursor(self)

    def commit(self):
        self.commits += 1

    def rollback(self):
        self.rollbacks += 1


def _chunk_table_instance():
    """Column metadata shaped like get_table_instance() output (name, data_type, column_default, is_nullable)."""
    return [
        ("id", "character varying(256)", "", "NO"),
        ("doc_id", "character varying(256)", "''", "YES"),
        ("kb_id", "character varying(256)", "''", "YES"),
        ("docnm_kwd", "text", "''", "YES"),
        ("available_int", "integer", "1", "YES"),
        ("position_int", "integer[]", None, "YES"),
        ("page_num_int", "integer[]", None, "YES"),
        ("important_kwd", "text", "''", "YES"),
        ("removed_kwd", "character varying(256)", "'N'", "YES"),
        ("metadata", "text", "''", "YES"),
        ("q_3_vec", "floatvector(3)", None, "YES"),
    ]


def _make_connection(table_instance=None):
    """A VBConnection instance that never touches a real Vastbase.

    Module-level get_table_instance() is patched on the module by the caller
    (monkeypatch), and get_conn() yields a RecordingConnection.
    """
    connection = object.__new__(_vb_connection_class())
    connection.db_compatibility = "PG"
    conn = RecordingConnection()
    connection.get_conn = contextlib.contextmanager(lambda: (yield conn))
    return connection, conn


class TestVBVectorTypeCaster:
    def test_parses_vector_text_to_float_list(self):
        assert _parse_floatvector("[0.1,0.25,0.5]", None) == [0.1, 0.25, 0.5]

    def test_tolerates_spaces_between_components(self):
        assert _parse_floatvector("[0.1, 0.25, 0.5]", None) == [0.1, 0.25, 0.5]

    def test_decodes_bytes(self):
        assert _parse_floatvector(b"[1.0,2.0]", None) == [1.0, 2.0]

    def test_none_and_empty_become_none(self):
        assert _parse_floatvector(None, None) is None
        assert _parse_floatvector("", None) is None


class TestFieldKeyword:
    def test_source_id_is_keyword(self):
        assert field_keyword("source_id") is True

    def test_kwd_suffixed_field_is_keyword(self):
        assert field_keyword("important_kwd") is True
        assert field_keyword("raptor_kwd") is True

    def test_docnm_kwd_is_not_keyword(self):
        """docnm_kwd is always a plain string, never a ###-joined list."""
        assert field_keyword("docnm_kwd") is False

    def test_knowledge_graph_kwd_is_not_keyword(self):
        assert field_keyword("knowledge_graph_kwd") is False

    def test_plain_field_is_not_keyword(self):
        assert field_keyword("title_tks") is False
        assert field_keyword("content_with_weight") is False


class TestQuoteIdent:
    def test_simple_identifier_is_double_quoted(self):
        assert quote_ident("id") == '"id"'

    def test_embedded_double_quote_is_doubled(self):
        assert quote_ident('a"b') == '"a""b"'

    def test_injection_attempt_stays_one_quoted_token(self):
        """Whatever the input, the result is a single quoted identifier — no statement can be injected."""
        malicious = "id; DROP TABLE users;--"
        rendered = quote_ident(malicious)
        assert rendered == '"' + malicious + '"'
        assert rendered.count('"') == 2


class TestTextColumn:
    def test_textish_types(self):
        for ty in ("character varying(256)", "varchar(32)", "text", "character(1)"):
            assert _text_column(ty) is True

    def test_non_text_types(self):
        for ty in ("integer", "double precision", "integer[]", "floatvector(1024)"):
            assert _text_column(ty) is False

    def test_none_type(self):
        assert _text_column(None) is False


class TestEquivalentConditionToStr:
    """ES-parity filter semantics: '' counts as field-absent on text columns, unmapped fields match no row."""

    def test_empty_condition_matches_all(self):
        assert equivalent_condition_to_str({}, _chunk_table_instance()) == "1=1"

    def test_kb_id_and_falsy_values_are_skipped(self):
        assert equivalent_condition_to_str({"kb_id": "kb1", "available_int": 0, "doc_id": ""}, _chunk_table_instance()) == "1=1"

    def test_string_equality_on_known_column(self):
        assert equivalent_condition_to_str({"doc_id": "abc"}, _chunk_table_instance()) == "\"doc_id\"='abc'"

    def test_numeric_equality_on_known_column(self):
        assert equivalent_condition_to_str({"available_int": 3}, _chunk_table_instance()) == '"available_int"=3'

    def test_unknown_column_matches_no_row(self):
        """Filtering a column the table lacks is like an unmapped ES field: no row matches."""
        assert equivalent_condition_to_str({"unknown_flt": 1.5}, _chunk_table_instance()) == "1=0"

    def test_list_condition_builds_in_clause(self):
        result = equivalent_condition_to_str({"doc_id": ["a", "b"]}, _chunk_table_instance())
        assert result == "\"doc_id\" IN ('a', 'b')"

    def test_list_items_with_quotes_are_escaped(self):
        result = equivalent_condition_to_str({"doc_id": ["b'c"]}, _chunk_table_instance())
        assert result == "\"doc_id\" IN ('b''c')"

    def test_scalar_string_with_quotes_is_escaped(self):
        result = equivalent_condition_to_str({"docnm_kwd": "it's"}, _chunk_table_instance())
        assert result == "\"docnm_kwd\"='it''s'"

    def test_exists_on_text_column_excludes_empty_string(self):
        """Unset text columns hold DEFAULT '' (not NULL), so '' plays the role of ES's field absent."""
        result = equivalent_condition_to_str({"exists": "docnm_kwd"}, _chunk_table_instance())
        assert result == '("docnm_kwd" IS NOT NULL AND "docnm_kwd" <> \'\')'

    def test_exists_on_non_text_column_checks_null_only(self):
        result = equivalent_condition_to_str({"exists": "available_int"}, _chunk_table_instance())
        assert result == '"available_int" IS NOT NULL'

    def test_exists_on_unknown_column_matches_no_row(self):
        assert equivalent_condition_to_str({"exists": "never_stored_fld"}, _chunk_table_instance()) == "1=0"

    def test_must_not_exists_on_text_column_treats_empty_as_absent(self):
        result = equivalent_condition_to_str({"must_not": {"exists": "docnm_kwd"}}, _chunk_table_instance())
        assert result == '("docnm_kwd" IS NULL OR "docnm_kwd" = \'\')'

    def test_must_not_exists_on_non_text_column(self):
        result = equivalent_condition_to_str({"must_not": {"exists": "available_int"}}, _chunk_table_instance())
        assert result == '"available_int" IS NULL'

    def test_must_not_exists_on_unknown_column_holds_for_all_rows(self):
        """A column the table lacks is NULL on every row, so must_not exists holds everywhere."""
        assert equivalent_condition_to_str({"must_not": {"exists": "never_stored_fld"}}, _chunk_table_instance()) == "1=1"

    def test_multiple_conditions_joined_with_and(self):
        result = equivalent_condition_to_str({"doc_id": "abc", "available_int": 3}, _chunk_table_instance())
        assert "\"doc_id\"='abc'" in result
        assert '"available_int"=3' in result
        assert " AND " in result

    def test_without_table_instance_unknown_columns_are_allowed(self):
        assert equivalent_condition_to_str({"doc_id": "abc"}, None) == "\"doc_id\"='abc'"

    def test_id_in_condition_is_rejected(self):
        with pytest.raises(AssertionError):
            equivalent_condition_to_str({"_id": "c1"}, _chunk_table_instance())

    def test_keyword_field_condition_is_currently_dropped(self):
        """KNOWN GAP (found while writing these tests): conditions on field_keyword()
        columns (source_id, *_kwd except docnm_kwd/knowledge_graph_kwd) fall into the
        `pass` branch and generate NO predicate, so e.g. {"raptor_kwd": ["raptor"]}
        or {"source_id": [...]} silently becomes match-all. Infinity, which this
        code mirrors, emits filter_fulltext() predicates for these columns instead.
        If this starts failing, the gap was fixed — update this test to assert the
        real predicate (###-array overlap)."""
        assert equivalent_condition_to_str({"raptor_kwd": ["raptor"]}, _chunk_table_instance()) == "1=1"
        assert equivalent_condition_to_str({"source_id": ["d1"]}, _chunk_table_instance()) == "1=1"


class TestSelectIdentifier:
    def test_asterisk_stays_literal(self):
        rendered = select_identifier("*")
        assert isinstance(rendered, sql.SQL)
        assert rendered.string == "*"

    def test_field_is_quoted_identifier(self):
        rendered = select_identifier("doc_id")
        assert isinstance(rendered, sql.Identifier)
        assert rendered.string == "doc_id"


class TestConcatDataframes:
    def test_all_empty_returns_schema_with_mapped_score_columns(self):
        df = concat_dataframes([], ["id", "score()", "similarity()"])
        assert df.empty
        assert list(df.columns) == ["id", "SCORE", "SIMILARITY"]

    def test_plain_fields_keep_their_names(self):
        df = concat_dataframes([], ["id", "docnm_kwd"])
        assert list(df.columns) == ["id", "docnm_kwd"]

    def test_non_empty_frames_are_concatenated(self):
        df1 = pd.DataFrame({"id": ["c1"], "v": [1]})
        df2 = pd.DataFrame({"id": ["c2"], "v": [2]})
        df = concat_dataframes([pd.DataFrame(columns=["id", "v"]), df1, df2], ["id"])
        assert list(df["id"]) == ["c1", "c2"]
        assert list(df.index) == [0, 1]


class TestEnsureTableColumns:
    """Static schema + auto ALTER for new upstream fields (idempotent)."""

    def test_no_missing_columns_returns_none_without_sql(self):
        conn = RecordingConnection()
        assert ensure_table_columns(conn, "t", _chunk_table_instance(), ["id", "doc_id"]) is None
        assert conn.executed == []

    def test_missing_known_field_gets_declared_type(self, monkeypatch):
        monkeypatch.setattr(vastbase_conn_module, "_MAPPING_SCHEMA_CACHE", {"toc_kwd": {"type": "varchar(32)", "default": ""}})
        table_instance = [row for row in _chunk_table_instance() if row[0] != "toc_kwd"]
        refreshed = _chunk_table_instance()
        conn = RecordingConnection(fetchone_results=[[True]], fetchall_results=[refreshed])
        result = ensure_table_columns(conn, "t", table_instance, ["toc_kwd"])
        assert result == refreshed
        alter = _render(conn.executed[0][0])
        assert alter == 'ALTER TABLE "t" ADD COLUMN "toc_kwd" varchar(32) DEFAULT \'\''
        assert conn.commits == 1

    def test_missing_vector_field_becomes_floatvector(self, monkeypatch):
        monkeypatch.setattr(vastbase_conn_module, "_MAPPING_SCHEMA_CACHE", {})
        conn = RecordingConnection(fetchone_results=[[True]], fetchall_results=[_chunk_table_instance()])
        ensure_table_columns(conn, "t", _chunk_table_instance(), ["q_512_vec"])
        assert 'ADD COLUMN "q_512_vec" floatvector(512)' in _render(conn.executed[0][0])

    def test_unknown_field_falls_back_to_varchar(self, monkeypatch):
        monkeypatch.setattr(vastbase_conn_module, "_MAPPING_SCHEMA_CACHE", {})
        conn = RecordingConnection(fetchone_results=[[True]], fetchall_results=[_chunk_table_instance()])
        ensure_table_columns(conn, "t", _chunk_table_instance(), ["brand_new_field"])
        assert 'ADD COLUMN "brand_new_field" varchar(256)' in _render(conn.executed[0][0])

    def test_uses_the_shipped_conf_mapping_types(self):
        conn = RecordingConnection(fetchone_results=[[True]], fetchall_results=[_chunk_table_instance()])
        ensure_table_columns(conn, "t", _chunk_table_instance(), ["toc_kwd"])
        assert 'ADD COLUMN "toc_kwd" varchar(32)' in _render(conn.executed[0][0])

    def test_concurrent_already_exists_is_tolerated(self, monkeypatch):
        monkeypatch.setattr(vastbase_conn_module, "_MAPPING_SCHEMA_CACHE", {})
        conn = RecordingConnection(
            fetchone_results=[[True]],
            fetchall_results=[_chunk_table_instance()],
            # Only the ALTER trips; the trailing get_table_instance() SELECTs must succeed.
            error_on_execute=lambda q: Exception('column "toc_kwd" of relation "t" already exists') if "ALTER TABLE" in _render(q) else None,
        )
        result = ensure_table_columns(conn, "t", _chunk_table_instance(), ["toc_kwd"])
        assert result == _chunk_table_instance()
        assert conn.rollbacks == 1

    def test_other_alter_errors_are_raised(self):
        conn = RecordingConnection(error_on_execute=RuntimeError("connection reset"))
        with pytest.raises(RuntimeError):
            ensure_table_columns(conn, "t", _chunk_table_instance(), ["toc_kwd"])
        assert conn.rollbacks == 1


class TestVBConnectionHelpers:
    def test_db_type(self):
        connection = object.__new__(_vb_connection_class())
        assert connection.db_type() == "vastbase"

    def test_vector_distance_op_depends_on_compatibility_mode(self):
        connection = object.__new__(_vb_connection_class())
        connection.db_compatibility = "PG"
        assert connection._vector_distance_op() == "<=>"
        connection.db_compatibility = "B"
        assert connection._vector_distance_op() == "<+>"

    def test_get_total_unwraps_tuple(self):
        connection = object.__new__(_vb_connection_class())
        assert connection.get_total((pd.DataFrame({"id": []}), 7)) == 7
        assert connection.get_total(pd.DataFrame({"id": [1, 2, 3]})) == 3

    def test_get_scores_reads_similarity_column(self):
        connection = object.__new__(_vb_connection_class())
        df = pd.DataFrame({"id": ["c1", "c2", "c3"], "SIMILARITY": [0.9, None, 0.1]})
        assert connection.get_scores((df, 3)) == {"c1": 0.9, "c2": 0.0, "c3": 0.1}

    def test_get_scores_defaults_to_zero_without_similarity_column(self):
        connection = object.__new__(_vb_connection_class())
        df = pd.DataFrame({"id": ["c1", "c2"], "docnm_kwd": ["a", "b"]})
        assert connection.get_scores(df) == {"c1": 0.0, "c2": 0.0}

    def test_get_scores_empty_result(self):
        connection = object.__new__(_vb_connection_class())
        assert connection.get_scores(pd.DataFrame()) == {}

    def test_get_doc_ids(self):
        connection = object.__new__(_vb_connection_class())
        assert connection.get_doc_ids((pd.DataFrame({"id": ["c1", "c2"]}), 2)) == ["c1", "c2"]

    def test_health_green(self):
        connection = object.__new__(_vb_connection_class())
        conn = MagicMock()
        connection.get_conn = contextlib.contextmanager(lambda: (yield conn))
        res = connection.health()
        assert res == {"type": "vastbase", "status": "green", "error": ""}
        conn.cursor.return_value.__enter__.return_value.execute.assert_called_once_with("SELECT vb_version()")

    def test_health_red_on_error(self):
        connection = object.__new__(_vb_connection_class())
        conn = MagicMock()
        conn.cursor.return_value.__enter__.return_value.execute.side_effect = ValueError("boom")
        connection.get_conn = contextlib.contextmanager(lambda: (yield conn))
        res = connection.health()
        assert res["status"] == "red"
        assert "boom" in res["error"]

    def test_dispose_is_safe_without_engine(self):
        connection = object.__new__(_vb_connection_class())
        connection.dispose()  # must not raise

    def test_dispose_disposes_engine(self):
        connection = object.__new__(_vb_connection_class())
        connection.engine = MagicMock()
        connection.dispose()
        connection.engine.dispose.assert_called_once()


class TestGetFields:
    def _connection(self):
        return object.__new__(_vb_connection_class())

    def test_keyword_columns_are_split_on_separator(self):
        df = pd.DataFrame([{"id": "c1", "important_kwd": "a###b"}, {"id": "c2", "important_kwd": None}])
        result = self._connection().get_fields(df, ["important_kwd"])
        assert result["c1"]["important_kwd"] == ["a", "b"]
        assert result["c2"]["important_kwd"] == []

    def test_position_int_is_regrouped_into_fives(self):
        df = pd.DataFrame([{"id": "c1", "position_int": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]}])
        result = self._connection().get_fields(df, ["position_int"])
        assert result["c1"]["position_int"] == [[1, 2, 3, 4, 5], [6, 7, 8, 9, 10]]

    def test_page_num_and_top_int_stay_lists(self):
        df = pd.DataFrame([{"id": "c1", "page_num_int": [1, 2], "top_int": None}])
        result = self._connection().get_fields(df, ["page_num_int", "top_int"])
        assert result["c1"]["page_num_int"] == [1, 2]
        assert result["c1"]["top_int"] == []

    def test_metadata_json_is_parsed(self):
        df = pd.DataFrame([{"id": "c1", "metadata": '{"a": 1}'}])
        result = self._connection().get_fields(df, ["metadata"])
        assert result["c1"]["metadata"] == {"a": 1}

    def test_non_json_text_stays(self):
        df = pd.DataFrame([{"id": "c1", "metadata": "plain"}])
        result = self._connection().get_fields(df, ["metadata"])
        assert result["c1"]["metadata"] == "plain"

    def test_score_alias_maps_to_similarity_column(self):
        df = pd.DataFrame([{"id": "c1", "similarity": 0.75}])
        result = self._connection().get_fields(df, ["_score"])
        assert result["c1"]["_score"] == 0.75

    def test_missing_columns_and_nan_values_are_stripped(self):
        df = pd.DataFrame([{"id": "c1", "doc_id": "d1", "weight_flt": float("nan")}])
        result = self._connection().get_fields(df, ["doc_id", "weight_flt", "never_selected_fld"])
        assert result["c1"] == {"doc_id": "d1"}

    def test_rows_are_deduplicated_by_id(self):
        df = pd.DataFrame([{"id": "c1", "doc_id": "d1"}, {"id": "c1", "doc_id": "d2"}])
        result = self._connection().get_fields(df, ["doc_id"])
        assert list(result.keys()) == ["c1"]
        assert result["c1"]["doc_id"] == "d1"

    def test_empty_fields_request(self):
        df = pd.DataFrame([{"id": "c1"}])
        assert self._connection().get_fields(df, []) == {}


class TestGetHighlight:
    def test_keywords_are_emphasized_per_sentence(self):
        connection = object.__new__(_vb_connection_class())
        df = pd.DataFrame([{"id": "c1", "content_with_weight": "Hello world. Nothing here. Hello again."}])
        ans = connection.get_highlight(df, ["Hello"], "content_with_weight")
        assert ans == {"c1": "<em>Hello</em> world... <em>Hello</em> again"}

    def test_missing_field_returns_empty(self):
        connection = object.__new__(_vb_connection_class())
        df = pd.DataFrame([{"id": "c1", "content_with_weight": "Hello"}])
        assert connection.get_highlight(df, ["Hello"], "title_tks") == {}


class TestUpdate:
    """VBConnection.update() builds SQL through psycopg2 composition, so dynamic identifiers are quoted, not rejected."""

    def test_legitimate_update_succeeds(self, monkeypatch):
        connection, conn = _make_connection()
        monkeypatch.setattr(vastbase_conn_module, "get_table_instance", lambda c, t: _chunk_table_instance())
        assert connection.update({"id": "c1"}, {"docnm_kwd": "foo"}, "test_table", "kb1") is True
        assert len(conn.executed) == 1
        assert _render(conn.executed[0][0]) == 'UPDATE "test_table_kb1" SET "docnm_kwd"=\'foo\' WHERE "id"=\'c1\''
        assert conn.commits == 1

    def test_keyword_list_values_are_hash_joined(self, monkeypatch):
        connection, conn = _make_connection()
        monkeypatch.setattr(vastbase_conn_module, "get_table_instance", lambda c, t: _chunk_table_instance())
        assert connection.update({"id": "c1"}, {"important_kwd": ["a", "b"]}, "test_table", "kb1") is True
        assert "SET \"important_kwd\"='a###b'" in _render(conn.executed[0][0])

    def test_remove_resets_column_to_its_default(self, monkeypatch):
        connection, conn = _make_connection()
        monkeypatch.setattr(vastbase_conn_module, "get_table_instance", lambda c, t: _chunk_table_instance())
        assert connection.update({"id": "c1"}, {"remove": "removed_kwd"}, "test_table", "kb1") is True
        # The literal "remove" key must NOT survive into the SET clause; the
        # column is reset to its DDL default via the SQL DEFAULT keyword.
        assert _render(conn.executed[0][0]) == 'UPDATE "test_table_kb1" SET "removed_kwd"=DEFAULT WHERE "id"=\'c1\''

    def test_remove_unknown_column_is_rejected_without_sql(self, monkeypatch):
        connection, conn = _make_connection()
        monkeypatch.setattr(vastbase_conn_module, "get_table_instance", lambda c, t: _chunk_table_instance())
        with pytest.raises(AssertionError):
            connection.update({"id": "c1"}, {"remove": "id; DROP TABLE users;--"}, "test_table", "kb1")
        assert conn.executed == []

    def test_malicious_identifier_is_quoted_not_injected(self, monkeypatch):
        """Unlike the OB allowlist, VBConnection passes identifiers through sql.Identifier, which quotes them."""
        connection, conn = _make_connection()
        monkeypatch.setattr(vastbase_conn_module, "get_table_instance", lambda c, t: _chunk_table_instance())
        assert connection.update({"id": "c1"}, {"id; DROP TABLE users;--": "foo"}, "test_table", "kb1") is True
        rendered = _render(conn.executed[0][0])
        assert "SET \"id; DROP TABLE users;--\"='foo'" in rendered
        assert "DROP TABLE users" not in rendered.replace('"id; DROP TABLE users;--"', "")

    def test_id_in_condition_is_rejected(self, monkeypatch):
        connection, _ = _make_connection()
        monkeypatch.setattr(vastbase_conn_module, "get_table_instance", lambda c, t: _chunk_table_instance())
        with pytest.raises(AssertionError):
            connection.update({"_id": "c1"}, {"docnm_kwd": "foo"}, "test_table", "kb1")


class TestInsert:
    def test_cannot_infer_vector_size_when_table_missing(self, monkeypatch):
        connection, conn = _make_connection()
        monkeypatch.setattr(vastbase_conn_module, "get_table_instance", lambda c, t: None)
        with pytest.raises(ValueError, match="Cannot infer vector size"):
            connection.insert([{"id": "c1", "docnm_kwd": "x"}], "vb", "kb1")
        assert conn.executed == []

    def test_missing_id_is_rejected(self, monkeypatch):
        connection, _ = _make_connection()
        monkeypatch.setattr(vastbase_conn_module, "get_table_instance", lambda c, t: _chunk_table_instance())
        with pytest.raises(AssertionError):
            connection.insert([{"doc_id": "d1"}], "vb", "kb1")

    def test_documents_are_normalized_before_insert(self, monkeypatch):
        connection, conn = _make_connection()
        monkeypatch.setattr(vastbase_conn_module, "get_table_instance", lambda c, t: _chunk_table_instance())
        captured = {}

        def fake_execute_values(cur, insert_sql, values):
            captured["sql"] = _render(insert_sql)
            captured["values"] = list(values)

        monkeypatch.setattr(vastbase_conn_module, "execute_values", fake_execute_values)

        documents = [
            {
                "id": "c1",
                "doc_id": "d1",
                "kb_id": ["k1", "k2"],
                "important_kwd": ["a", "b"],
                "position_int": [[1, 2, 3, 4, 5], [6, 7, 8, 9, 10]],
                "metadata": {"a": 1},
                "q_3_vec": [0.1, 0.2, 0.3],
            }
        ]
        assert connection.insert(documents, "vb", "kb1") == []

        delete_sql, delete_params = conn.executed[0]
        assert _render(delete_sql) == 'DELETE FROM "vb_kb1" WHERE id IN %s'
        assert delete_params == (("c1",),)

        assert captured["sql"].startswith('INSERT INTO "vb_kb1" ("id"')
        assert len(captured["values"]) == 1
        by_column = dict(zip(documents[0].keys(), captured["values"][0]))
        assert by_column["kb_id"] == "k1"
        assert by_column["important_kwd"] == "a###b"
        assert by_column["position_int"] == [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
        assert by_column["metadata"] == json.dumps({"a": 1})
        assert by_column["q_3_vec"] == [0.1, 0.2, 0.3]
        assert conn.commits == 1

        # insert() deep-copies its input, so the caller's documents are untouched.
        assert documents[0]["important_kwd"] == ["a", "b"]
        assert documents[0]["position_int"] == [[1, 2, 3, 4, 5], [6, 7, 8, 9, 10]]
        assert documents[0]["metadata"] == {"a": 1}

    def test_missing_vector_columns_are_backfilled_with_zeroes(self, monkeypatch):
        """Docs lacking the table's vector column must be backfilled with zero
        vectors — and the backfill must not run inside the per-field loop it
        used to share (mutating the dict during items() iteration)."""
        connection, _ = _make_connection()
        monkeypatch.setattr(vastbase_conn_module, "get_table_instance", lambda c, t: _chunk_table_instance())
        captured = {}
        monkeypatch.setattr(vastbase_conn_module, "execute_values", lambda cur, insert_sql, values: captured.update(values=list(values)))

        documents = [{"id": "c1", "doc_id": "d1"}]
        connection.insert(documents, "vb", "kb1")

        assert len(captured["values"]) == 1
        # column order follows docs[0].keys() after backfill: id, doc_id, q_3_vec
        assert captured["values"][0] == ("c1", "d1", [0, 0, 0])


@pytest.fixture
def render_as_string(monkeypatch):
    """create_idx() renders SQL for debug logging via as_string(conn), which the
    psycopg2 C extension restricts to real connections — substitute the test
    renderer (each Composable subclass overrides as_string, so patch them all)."""
    for composable_cls in (sql.Composable, sql.Composed, sql.SQL, sql.Identifier, sql.Literal):
        monkeypatch.setattr(composable_cls, "as_string", lambda self, context: _render(self))


class TestTableOperations:
    def _connection(self, conn, db_compatibility="PG"):
        connection = object.__new__(_vb_connection_class())
        connection.db_compatibility = db_compatibility
        connection.get_conn = contextlib.contextmanager(lambda: (yield conn))
        return connection

    def test_create_idx_pg_mode_builds_table_graph_and_gin_indexes(self, render_as_string):
        conn = RecordingConnection()
        connection = self._connection(conn, "PG")
        connection.create_idx("vb", "kb1", 1024)
        rendered = [_render(q) for q, _ in conn.executed]
        assert any('CREATE TABLE IF NOT EXISTS "vb_kb1"' in r and "floatvector(1024)" in r for r in rendered)
        assert any('USING graph_index ("q_1024_vec" floatvector_cosine_ops)' in r for r in rendered)
        assert any("USING gin(" in r and "to_tsvector('cn_tokenizer', \"title_tks\")" in r for r in rendered)
        assert conn.commits == 2

    def test_create_idx_b_mode_builds_fulltext_indexes(self, render_as_string):
        conn = RecordingConnection()
        connection = self._connection(conn, "B")
        connection.create_idx("vb", "kb1", 3)
        rendered = [_render(q) for q, _ in conn.executed]
        assert any("ADD INDEX" in r and 'USING "fulltext"' in r for r in rendered)
        # One fulltext index per text field.
        assert sum('USING "fulltext"' in r for r in rendered) == 7

    def test_delete_idx_drops_the_table(self):
        conn = RecordingConnection()
        connection = self._connection(conn)
        connection.delete_idx("vb", "kb1")
        assert _render(conn.executed[0][0]) == 'DROP TABLE IF EXISTS "vb_kb1"'
        assert conn.commits == 1

    def test_index_exist_uses_table_lookup(self):
        conn = RecordingConnection(fetchone_results=[[True]])
        assert self._connection(conn).index_exist("vb", "kb1") is True

    def test_index_exist_doc_meta_table_name_is_used_as_is(self):
        conn = RecordingConnection(fetchone_results=[[True]])
        assert self._connection(conn).index_exist("ragflow_doc_meta_t1", "kb1") is True
        # The doc-meta table name carries the tenant id itself — no kb suffix is appended.
        # get_table_exists() passes the name as a bind parameter.
        assert conn.executed[0][1] == ("ragflow_doc_meta_t1",)


class TestSearchDenseSql:
    @pytest.mark.parametrize("compatibility,distance_op", [("PG", "<=>"), ("B", "<+>")])
    def test_similarity_threshold_wraps_the_knn_scan(self, monkeypatch, render_as_string, compatibility, distance_op):
        """The dense-search threshold must live in an outer WHERE over the KNN
        subquery: a distance predicate in the inner WHERE makes the planner
        fall back to a Seq Scan and bypass the graph_index (HNSW)."""
        conn = RecordingConnection(fetchall_results=[[]], description=[("id",)])
        connection = object.__new__(_vb_connection_class())
        connection.db_compatibility = compatibility
        connection.get_conn = contextlib.contextmanager(lambda: (yield conn))
        monkeypatch.setattr(vastbase_conn_module, "get_table_exists", lambda c, t: True)
        dense = MatchDenseExpr("q_3_vec", [0.1, 0.2, 0.3], "float", "cosine", 10, {"similarity": 0.17})
        connection.search(["id"], [], {}, [dense], OrderByExpr(), 0, 10, "vb", ["kb1"])

        rendered = _render(conn.executed[0][0])
        inner, _, outer = rendered.partition(") AS vector_candidates")
        # Pure KNN scan: ORDER BY + LIMIT, no WHERE — the graph index stays eligible.
        assert "WHERE" not in inner
        assert f'ORDER BY "q_3_vec" {distance_op} ' in inner
        assert "LIMIT 10" in inner
        # The threshold filters the SIMILARITY alias after the scan, in the outer query.
        assert 'WHERE "SIMILARITY" >= 0.17' in outer

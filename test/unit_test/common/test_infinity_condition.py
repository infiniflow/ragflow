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
"""Unit tests for the Infinity ``equivalent_condition_to_str`` branch that
handles the migrated JSON-list columns
(``source_doc_ids``/``source_chunk_ids``/``compilation_template_ids``/
``doc_ids_kwd``/``entity_names_kwd``/``outlinks_kwd``/
``related_kb_pages_kwd``/``rechunked_from_chunk_ids``).

These columns were migrated from ``varchar`` (``whitespace-#`` analyzer,
``###``-joined encoding) to ``json`` in #17288, and then exposed through a
``json_contains`` filter in ``InfinityConnectionBase.equivalent_condition_to_str``.

The pre-#17288 chunk tables still in the wild have these columns as
``varchar``. ``json_contains`` against a Varchar column returns
``3030 json_contains(Varchar, Varchar) not found``, so the translator must
fall back to a ``filter_fulltext`` query that matches the legacy encoding.
This module pins that behavior down (#17685).

Run with: python -m pytest test/unit_test/common/test_infinity_condition.py -v
"""

from unittest.mock import MagicMock, patch

import pytest

pytestmark = pytest.mark.p2


# ``common.doc_store.infinity_conn_base`` is loaded via ``common.settings``,
# which in turn imports the rag- and memory-side Infinity connectors. We
# pre-load ``common.settings`` first so the partial-module circular import
# in the rag/memory side is already resolved by the time we reach the base
# class.
import common.settings  # noqa: F401
from rag.utils import infinity_conn as rag_infinity_conn


# ``InfinityConnection`` is wrapped by ``@common.decorator.singleton``,
# which replaces the class object with a factory function. Reach into the
# closure to recover the undecorated class so we can call its methods
# without dialing Infinity.
def _resolve_infinity_class():
    factory = rag_infinity_conn.InfinityConnection
    closure_vars = factory.__closure__
    assert closure_vars, "singleton factory has no closure"
    for cell in closure_vars:
        cls = cell.cell_contents
        if isinstance(cls, type):
            return cls
    raise RuntimeError("could not recover InfinityConnection from singleton closure")


_InfinityConnection = _resolve_infinity_class()


# ---------------------------------------------------------------------------
# Lightweight stand-in for ``infinity.remote_thrift.table.RemoteTable``.
# Captures the column metadata that ``equivalent_condition_to_str`` reads via
# ``table_instance.show_columns().rows()``.
# ---------------------------------------------------------------------------


class _FakeInfinityTable:
    """Minimal mock of an Infinity table: returns a fixed column list."""

    def __init__(self, columns):
        # ``columns`` is a dict of name -> (type_string, default).
        self._columns = dict(columns)

    def show_columns(self):
        class _Resp:
            def __init__(self, rows):
                self._rows = rows

            def rows(self):
                return self._rows

        return _Resp([(n, ty, de, "") for n, (ty, de) in self._columns.items()])


# ---------------------------------------------------------------------------
# The translator is a static-ish helper that does not actually touch Infinity
# at runtime; we instantiate the base class only for ``convert_matching_field``
# and the column-typing helpers.
# ---------------------------------------------------------------------------


def _translate(condition, columns):
    """Run ``equivalent_condition_to_str`` against the supplied schema."""
    # ``equivalent_condition_to_str`` does not touch the connection, so we
    # can skip ``__init__`` (which would otherwise try to dial Infinity via
    # the singleton decorator).
    return _InfinityConnection.equivalent_condition_to_str(
        _InfinityConnection.__new__(_InfinityConnection),
        dict(condition),
        table_instance=_FakeInfinityTable(columns),
    )


_JSON_COLS = {
    # New schema (since #17288)
    "source_doc_ids": ("Json", "[]"),
    "source_chunk_ids": ("Json", "[]"),
    "compilation_template_ids": ("Json", "[]"),
    "doc_ids_kwd": ("Json", "[]"),
    "entity_names_kwd": ("Json", "[]"),
    "outlinks_kwd": ("Json", "[]"),
    "related_kb_pages_kwd": ("Json", "[]"),
    "rechunked_from_chunk_ids": ("Json", "[]"),
}

_VARCHAR_COLS = {
    # Legacy schema (pre-#17288) — Varchar with a ``###``-joined encoding
    "source_doc_ids": ("Varchar", ""),
    "source_chunk_ids": ("Varchar", ""),
    "compilation_template_ids": ("Varchar", ""),
    "doc_ids_kwd": ("Varchar", ""),
    "entity_names_kwd": ("Varchar", ""),
    "outlinks_kwd": ("Varchar", ""),
    "related_kb_pages_kwd": ("Varchar", ""),
    "rechunked_from_chunk_ids": ("Varchar", ""),
}


# ---------------------------------------------------------------------------
# JSON (post-#17288) columns
# ---------------------------------------------------------------------------


class TestJsonColumnsUseJsonContains:
    """New tables (post-#17288) have these columns as Json and must use
    ``json_contains`` with a JSON-serialized literal."""

    @pytest.mark.parametrize(
        "col",
        [
            "source_doc_ids",
            "source_chunk_ids",
            "compilation_template_ids",
            "doc_ids_kwd",
            "entity_names_kwd",
            "outlinks_kwd",
            "related_kb_pages_kwd",
            "rechunked_from_chunk_ids",
        ],
    )
    def test_string_value_uses_json_contains(self, col):
        result = _translate({col: ["doc-1"]}, {col: ("Json", "[]")})
        assert result is not None
        # The JSON literal for a string is the quoted form.
        assert f"json_contains({col}, '\"doc-1\"')" in result

    def test_list_of_strings_joined_with_or(self):
        result = _translate(
            {"source_doc_ids": ["doc-1", "doc-2"]},
            {"source_doc_ids": ("Json", "[]")},
        )
        assert result is not None
        assert "json_contains(source_doc_ids, '\"doc-1\"')" in result
        assert "json_contains(source_doc_ids, '\"doc-2\"')" in result
        assert " or " in result

    def test_numeric_value_uses_unquoted_literal(self):
        result = _translate(
            {"doc_ids_kwd": [42, 99]},
            {"doc_ids_kwd": ("Json", "[]")},
        )
        assert result is not None
        # ``json.dumps(42) == '42'`` (no surrounding quotes).
        assert "json_contains(doc_ids_kwd, '42')" in result
        assert "json_contains(doc_ids_kwd, '99')" in result

    def test_apostrophe_in_value_is_escaped(self):
        result = _translate(
            {"source_doc_ids": ["o'brien"]},
            {"source_doc_ids": ("Json", "[]")},
        )
        assert result is not None
        # ``json.dumps("o'brien")`` -> ``"o'brien"``; the single quote inside
        # is doubled to keep the surrounding SQL literal valid.
        assert "json_contains(source_doc_ids, '\"o''brien\"')" in result


# ---------------------------------------------------------------------------
# Legacy Varchar (pre-#17288) columns
# ---------------------------------------------------------------------------


class TestLegacyVarcharColumnsUseFilterFulltext:
    """Pre-#17288 chunk tables store these columns as Varchar with a
    ``###``-joined encoding. ``json_contains`` returns
    ``3030 json_contains(Varchar, Varchar) not found`` on them, so the
    translator must fall back to ``filter_fulltext`` with the bare item
    value (the ``whitespace-#`` analyzer tokenizes the ``###``-joined
    string into the individual values)."""

    @pytest.mark.parametrize(
        "col",
        [
            "source_doc_ids",
            "source_chunk_ids",
            "compilation_template_ids",
            "doc_ids_kwd",
            "entity_names_kwd",
            "outlinks_kwd",
            "related_kb_pages_kwd",
            "rechunked_from_chunk_ids",
        ],
    )
    def test_uses_filter_fulltext_with_bare_value(self, col):
        result = _translate({col: ["doc-1"]}, {col: ("Varchar", "")})
        assert result is not None
        # Bare value, NOT the JSON-serialized literal. ``filter_fulltext``
        # takes a quoted column name and a quoted value.
        assert f"filter_fulltext('{col}', 'doc-1')" in result
        # The buggy ``json_contains`` form must NOT be emitted.
        assert "json_contains" not in result

    def test_list_of_strings_joined_with_or(self):
        result = _translate(
            {"source_doc_ids": ["doc-1", "doc-2"]},
            {"source_doc_ids": ("Varchar", "")},
        )
        assert result is not None
        assert "filter_fulltext('source_doc_ids', 'doc-1')" in result
        assert "filter_fulltext('source_doc_ids', 'doc-2')" in result
        assert " or " in result

    def test_apostrophe_in_value_is_escaped(self):
        result = _translate(
            {"source_doc_ids": ["o'brien"]},
            {"source_doc_ids": ("Varchar", "")},
        )
        assert result is not None
        assert "filter_fulltext('source_doc_ids', 'o''brien')" in result

    def test_numeric_value_is_skipped_on_legacy_varchar(self):
        """Pre-#17288 the ``###``-joined encoding could not represent a
        numeric value in a searchable way — emitting a query would just
        return nothing, so we skip non-string items rather than emit a
        query that lies to the caller."""
        result = _translate(
            {"doc_ids_kwd": [42]},
            {"doc_ids_kwd": ("Varchar", "")},
        )
        # No predicate should be emitted, so the empty condition yields
        # the ``1=1`` default.
        assert result == "1=1"


# ---------------------------------------------------------------------------
# Unknown / missing columns
# ---------------------------------------------------------------------------


class TestUnknownColumnsAreSkipped:
    """If the condition references one of the JSON-list columns but the
    table doesn't carry it (or carries it under an unknown type), the
    translator must skip the predicate rather than emit a query that
    Infinity would reject. The remaining conditions (or ``1=1``) keep the
    request valid."""

    def test_json_list_column_missing_from_schema_is_skipped(self):
        result = _translate(
            {
                "source_doc_ids": ["doc-1"],
                # ``source_chunk_ids`` is in the JSON-list set but not in the
                # supplied table schema — we cannot tell its type, so we
                # skip the predicate rather than risk a
                # ``json_contains(Varchar, Varchar) not found`` (#17685).
                "source_chunk_ids": ["chunk-x"],
            },
            {"source_doc_ids": ("Json", "[]")},
        )
        assert result is not None
        assert "json_contains(source_doc_ids, '\"doc-1\"')" in result
        # The unknown-type column contributes nothing.
        assert "source_chunk_ids" not in result

    def test_only_unknown_type_column_yields_one_equals_one(self):
        result = _translate(
            {"source_doc_ids": ["doc-1"]},
            # No columns at all — we cannot tell the type, so skip.
            {},
        )
        assert result == "1=1"

    def test_no_table_metadata_skips_json_predicate(self):
        """``table_instance=None`` means we have no column metadata. We must
        not fabricate column types — the predicate is skipped to avoid a
        query that Infinity would reject."""

        result = _InfinityConnection.equivalent_condition_to_str(
            _InfinityConnection.__new__(_InfinityConnection),
            {"source_doc_ids": ["doc-1"]},
            table_instance=None,
        )
        # The Json predicate is gated on having seen the column as Json. With
        # no metadata we skip the predicate rather than risk the legacy
        # Varchar ``json_contains`` failure.
        assert result == "1=1"


# ---------------------------------------------------------------------------
# Other behavior (smoke)
# ---------------------------------------------------------------------------


class TestOtherConditionBranches:
    """Confirm we didn't accidentally regress the non-JSON-list branches."""

    def test_available_int(self):
        result = _translate({"available_int": 1}, {})
        assert result == "available_int=1"

    def test_compile_kwd_string(self):
        result = _translate({"compile_kwd": ["entity"]}, {})
        assert result == "(compile_kwd='entity')"

    def test_compile_kwd_multi(self):
        result = _translate(
            {"compile_kwd": ["entity", "relation"]},
            {},
        )
        assert "compile_kwd='entity'" in result
        assert "compile_kwd='relation'" in result
        assert " or " in result

    def test_kb_id_varchar(self):
        result = _translate({"kb_id": "kb-1"}, {"kb_id": ("Varchar", "")})
        assert result == "kb_id='kb-1'"


class TestDeleteSafety:
    """``delete()`` must abort and raise ValueError if a non-empty condition generates
    an unconstrained filter ('1=1') or unmapped predicate to prevent accidental table truncation."""

    def test_delete_raises_when_condition_yields_unconstrained_filter(self):
        inst = _InfinityConnection.__new__(_InfinityConnection)
        inst.dbName = "default_db"
        inst.logger = MagicMock()
        inst.connPool = MagicMock()

        inf_conn = MagicMock()
        db = MagicMock()
        table = MagicMock()
        # Empty schema -> equivalent_condition_to_str yields "1=1"
        table.show_columns.return_value.rows.return_value = []
        db.get_table.return_value = table
        inf_conn.get_database.return_value = db

        with patch.object(inst.connPool, "get_conn", return_value=inf_conn), patch.object(inst.connPool, "release_conn"):
            with pytest.raises(ValueError, match="Cannot build delete predicate|unconstrained filter"):
                inst.delete({"source_doc_ids": ["doc-1"]}, "ragflow_tenant", "kb-1")

        # Must NOT call table.delete()
        table.delete.assert_not_called()

    def test_delete_raises_value_error_for_unmapped_delete_predicate(self):
        inst = _InfinityConnection.__new__(_InfinityConnection)
        inst.dbName = "default_db"
        inst.logger = MagicMock()
        inst.connPool = MagicMock()

        inf_conn = MagicMock()
        db = MagicMock()
        table = MagicMock()
        table.show_columns.return_value.rows.return_value = [("other_col", "Varchar", "", "")]
        db.get_table.return_value = table
        inf_conn.get_database.return_value = db

        with patch.object(inst.connPool, "get_conn", return_value=inf_conn), patch.object(inst.connPool, "release_conn"):
            with pytest.raises(ValueError, match="Cannot build delete predicate"):
                inst.delete({"source_doc_ids": ["doc-1"]}, "ragflow_tenant", "kb-1")

        table.delete.assert_not_called()

    def test_legacy_varchar_chunk_table_delete_by_source_doc_ids(self):
        inst = _InfinityConnection.__new__(_InfinityConnection)
        inst.dbName = "default_db"
        inst.logger = MagicMock()
        inst.connPool = MagicMock()

        inf_conn = MagicMock()
        db = MagicMock()
        table = MagicMock()
        # Schema with legacy Varchar column
        table.show_columns.return_value.rows.return_value = [("source_doc_ids", "Varchar", "", "")]
        table.delete.return_value = MagicMock(deleted_rows=5)
        db.get_table.return_value = table
        inf_conn.get_database.return_value = db

        with patch.object(inst.connPool, "get_conn", return_value=inf_conn), patch.object(inst.connPool, "release_conn"):
            deleted = inst.delete({"source_doc_ids": ["doc-123"]}, "ragflow_tenant", "kb-1")

        assert deleted == 5
        table.delete.assert_called_once_with("(filter_fulltext('source_doc_ids', 'doc-123'))")


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

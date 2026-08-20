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
"""Unit tests for the two ``InfinityConnection`` helpers added in #17685:

- ``get`` special-cases the ``ragflow_doc_meta_`` index family so callers
  can pass an empty ``knowledgebase_ids`` (the meta table is per-tenant and
  has no ``_kb_id`` suffix).
- ``ensure_columns`` upgrades pre-existing chunk tables in place to add
  columns that were introduced after the table was created (e.g.
  ``deleted_doc_id``).

The methods need a live Infinity connection, so we patch the connection
pool and exercise the routing logic. Run with::

    python -m pytest test/unit_test/rag/utils/test_infinity_conn_helpers.py -v
"""

import logging
from unittest.mock import MagicMock, patch

import pytest

pytestmark = pytest.mark.p2


import common.settings  # noqa: F401  -- see test_infinity_condition for the why
from rag.utils import infinity_conn as rag_infinity_conn


def _resolve_infinity_class():
    """The class is wrapped by ``@common.decorator.singleton``; recover the
    underlying class object from the wrapper's closure (same trick as in
    test_infinity_condition)."""
    factory = rag_infinity_conn.InfinityConnection
    for cell in factory.__closure__ or ():
        cls = cell.cell_contents
        if isinstance(cls, type):
            return cls
    raise RuntimeError("could not recover InfinityConnection from singleton closure")


_Inf = _resolve_infinity_class()


# ---------------------------------------------------------------------------
# ``InfinityConnection.get`` — doc-meta special case
# ---------------------------------------------------------------------------


class TestGetHandlesDocMetaIndex:
    """``get`` must treat ``ragflow_doc_meta_`` indexes as per-tenant, not
    per-kb. The pre-#17685 code built a ``<tenant>_<kb>`` table name and
    either logged a "blank knowledgebase_ids" warning (when ``[""]`` was
    passed) or a "Table not found" warning (when a real ``kb_id`` was
    passed), without ever actually querying the table."""

    def _new_conn(self):
        """Build a bare ``InfinityConnection`` that does not touch the real
        connection pool. ``__new__`` skips ``__init__``; we hand-roll the
        attributes the methods under test read."""
        inst = _Inf.__new__(_Inf)
        inst.dbName = "default_db"
        inst.logger = logging.getLogger("test.infinity_conn_helpers")
        inst.connPool = MagicMock()
        return inst

    def test_meta_index_with_empty_kb_ids_uses_index_name_as_table(self):
        conn = self._new_conn()
        inf_conn = MagicMock()
        db = MagicMock()
        table = MagicMock()
        empty_df = MagicMock()
        empty_df.empty = True
        empty_df.columns.tolist.return_value = ["id"]
        table.output.return_value.filter.return_value.to_df.return_value = (empty_df, 0)
        db.get_table.return_value = table
        inf_conn.get_database.return_value = db

        with patch.object(conn.connPool, "get_conn", return_value=inf_conn), patch.object(conn.connPool, "release_conn") as release:
            # ``[""]`` (or any list of blanks) used to trigger the
            # "blank knowledgebase_ids" warning. The fixed code treats
            # meta tables by index name and silently returns ``None`` for
            # the (now nonexistent) row.
            result = conn.get(
                "doc-1",
                "ragflow_doc_meta_tenant-A",
                [""],
            )

        assert result is None
        # The table is queried by index name, not by ``index_name + "_" + kb_id``.
        db.get_table.assert_called_once_with("ragflow_doc_meta_tenant-A")
        release.assert_called_once_with(inf_conn)

    def test_meta_index_with_real_kb_id_still_uses_index_name(self):
        """Existing callers that pass ``[kb_id]`` for a meta index used to
        build the non-existent ``<tenant>_<kb>`` table and log a warning.
        The fix routes them to the meta table directly."""
        conn = self._new_conn()
        inf_conn = MagicMock()
        db = MagicMock()
        table = MagicMock()
        empty_df = MagicMock()
        empty_df.empty = True
        empty_df.columns.tolist.return_value = ["id"]
        table.output.return_value.filter.return_value.to_df.return_value = (empty_df, 0)
        db.get_table.return_value = table
        inf_conn.get_database.return_value = db

        with patch.object(conn.connPool, "get_conn", return_value=inf_conn):
            conn.get("doc-1", "ragflow_doc_meta_tenant-A", ["kb-1"])

        db.get_table.assert_called_once_with("ragflow_doc_meta_tenant-A")

    def test_non_meta_index_with_empty_kb_ids_still_returns_none(self):
        """The pre-existing behavior for chunk indexes is unchanged: an
        empty ``knowledgebase_ids`` is a programmer error and the call
        short-circuits to ``None``."""
        conn = self._new_conn()
        inf_conn = MagicMock()
        with patch.object(conn.connPool, "get_conn", return_value=inf_conn):
            result = conn.get("chunk-1", "ragflow_tenant-A", [])

        assert result is None
        inf_conn.get_database.assert_not_called()

    def test_non_meta_index_with_blank_kb_ids_still_returns_none(self):
        conn = self._new_conn()
        inf_conn = MagicMock()
        with patch.object(conn.connPool, "get_conn", return_value=inf_conn):
            result = conn.get("chunk-1", "ragflow_tenant-A", ["", ""])

        assert result is None
        inf_conn.get_database.assert_not_called()


# ---------------------------------------------------------------------------
# ``InfinityConnection.ensure_columns``
# ---------------------------------------------------------------------------


class TestEnsureColumns:
    """``ensure_columns`` upgrades chunk tables in place with columns that
    are not yet present (e.g. ``deleted_doc_id`` from #17685). The method
    is idempotent and silent on already-present columns."""

    def _new_conn(self):
        inst = _Inf.__new__(_Inf)
        inst.dbName = "default_db"
        inst.logger = logging.getLogger("test.infinity_conn_helpers")
        inst.connPool = MagicMock()
        return inst

    def test_adds_missing_column_via_add_columns(self):
        conn = self._new_conn()
        inf_conn = MagicMock()
        db = MagicMock()
        table = MagicMock()
        # ``deleted_doc_id`` is missing; ``kb_id`` is present.
        table.show_columns.return_value.rows.return_value = [
            ("id", "Varchar", "", ""),
            ("kb_id", "Varchar", "", ""),
        ]
        db.get_table.return_value = table
        inf_conn.get_database.return_value = db

        with patch.object(conn.connPool, "get_conn", return_value=inf_conn), patch.object(conn.connPool, "release_conn") as release:
            conn.ensure_columns(
                "ragflow_tenant-A",
                "kb-1",
                {"deleted_doc_id": {"type": "varchar", "default": ""}},
            )

        # Only the missing column is passed to ``add_columns``.
        table.add_columns.assert_called_once_with({"deleted_doc_id": {"type": "varchar", "default": ""}})
        release.assert_called_once_with(inf_conn)

    def test_skips_when_all_columns_present(self):
        conn = self._new_conn()
        inf_conn = MagicMock()
        db = MagicMock()
        table = MagicMock()
        table.show_columns.return_value.rows.return_value = [
            ("id", "Varchar", "", ""),
            ("deleted_doc_id", "Varchar", "", ""),
        ]
        db.get_table.return_value = table
        inf_conn.get_database.return_value = db

        with patch.object(conn.connPool, "get_conn", return_value=inf_conn), patch.object(conn.connPool, "release_conn") as release:
            conn.ensure_columns(
                "ragflow_tenant-A",
                "kb-1",
                {"deleted_doc_id": {"type": "varchar", "default": ""}},
            )

        table.add_columns.assert_not_called()
        release.assert_called_once_with(inf_conn)

    def test_silent_when_table_missing(self):
        """A missing table means the next ``insert()`` will create it with
        the current schema; ``ensure_columns`` must not interfere."""
        from infinity.common import InfinityException

        conn = self._new_conn()
        inf_conn = MagicMock()
        db = MagicMock()
        db.get_table.side_effect = InfinityException(3022, "table missing")
        inf_conn.get_database.return_value = db

        with patch.object(conn.connPool, "get_conn", return_value=inf_conn), patch.object(conn.connPool, "release_conn") as release:
            conn.ensure_columns(
                "ragflow_tenant-A",
                "kb-1",
                {"deleted_doc_id": {"type": "varchar", "default": ""}},
            )

        release.assert_called_once_with(inf_conn)

    def test_logs_exception_when_other_infinity_error(self):
        """Non-TABLE_NOT_EXIST Infinity exceptions are re-raised internally
        and caught/logged by the outer exception handler."""
        from infinity.common import InfinityException

        conn = self._new_conn()
        inf_conn = MagicMock()
        db = MagicMock()
        db.get_table.side_effect = InfinityException(3000, "catalog corrupted")
        inf_conn.get_database.return_value = db

        with patch.object(conn.connPool, "get_conn", return_value=inf_conn), patch.object(conn.connPool, "release_conn") as release:
            conn.ensure_columns(
                "ragflow_tenant-A",
                "kb-1",
                {"deleted_doc_id": {"type": "varchar", "default": ""}},
            )

        release.assert_called_once_with(inf_conn)

    def test_meta_table_uses_index_name_directly(self):
        conn = self._new_conn()
        inf_conn = MagicMock()
        db = MagicMock()
        table = MagicMock()
        table.show_columns.return_value.rows.return_value = [
            ("id", "Varchar", "", ""),
            ("kb_id", "Varchar", "", ""),
            ("meta_fields", "Json", "{}", ""),
        ]
        db.get_table.return_value = table
        inf_conn.get_database.return_value = db

        with patch.object(conn.connPool, "get_conn", return_value=inf_conn):
            # Doc-meta tables pass an empty ``knowledgebase_id``; the helper
            # should still resolve the table by index name only.
            conn.ensure_columns(
                "ragflow_doc_meta_tenant-A",
                "",
                {"new_col": {"type": "varchar", "default": ""}},
            )

        db.get_table.assert_called_once_with("ragflow_doc_meta_tenant-A")
        table.add_columns.assert_called_once_with({"new_col": {"type": "varchar", "default": ""}})

    def test_swallows_add_columns_failure(self):
        """``add_columns`` is best-effort; a failure must not propagate so
        the caller's own write path can still proceed (and log a
        structured warning via the connector)."""
        conn = self._new_conn()
        inf_conn = MagicMock()
        db = MagicMock()
        table = MagicMock()
        table.show_columns.return_value.rows.return_value = [
            ("id", "Varchar", "", ""),
        ]
        table.add_columns.side_effect = RuntimeError("boom")
        db.get_table.return_value = table
        inf_conn.get_database.return_value = db

        with patch.object(conn.connPool, "get_conn", return_value=inf_conn), patch.object(conn.connPool, "release_conn") as release:
            # Must not raise.
            conn.ensure_columns(
                "ragflow_tenant-A",
                "kb-1",
                {"deleted_doc_id": {"type": "varchar", "default": ""}},
            )

        release.assert_called_once_with(inf_conn)


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

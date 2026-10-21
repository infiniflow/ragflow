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
"""Unit tests for ``dataset_structure_merger.py`` (deletion markers + row field normalization)."""

from unittest.mock import MagicMock, patch

import pytest

pytestmark = pytest.mark.p2

from common.doc_store.doc_store_base import field_scalar
from rag.svr.task_executor_refactor.dataset_structure_merger import _UPGRADED_TABLES, _is_relation_row, record_doc_deletion


class TestFieldScalar:
    """``*_kwd`` columns are one-item lists on Infinity and scalars on ES."""

    def test_unwraps_infinity_list(self):
        assert field_scalar(["tree"]) == "tree"

    def test_passes_through_es_scalar(self):
        assert field_scalar("tree") == "tree"

    def test_none_and_empty_are_empty(self):
        assert field_scalar(None) == ""
        assert field_scalar([]) == ""
        assert field_scalar("  ") == ""

    def test_default_applies_to_missing(self):
        assert field_scalar(None, default="skill") == "skill"
        assert field_scalar([], default="skill") == "skill"
        assert field_scalar(["x"], default="skill") == "x"


class TestIsRelationRow:
    def test_entity_row_is_not_a_relation_even_when_infinity_fills_the_column(self):
        # Infinity fills absent columns, so ``from_entity_kwd`` exists (as [])
        # on an entity row; a ``key in row`` test would misclassify it.
        entity_row = {"name_kwd": ["apple"], "from_entity_kwd": [], "to_entity_kwd": []}
        assert not _is_relation_row(entity_row)

    def test_relation_row_is_detected_from_knowledge_graph_kwd(self):
        assert _is_relation_row({"knowledge_graph_kwd": ["relation"]})

    def test_relation_row_is_detected_from_entity_columns(self):
        assert _is_relation_row({"from_entity_kwd": ["apple"], "to_entity_kwd": ["iphone"]})

    def test_relation_row_without_marker_is_not_a_relation(self):
        assert not _is_relation_row({"name_kwd": ["apple"]})


class TestRecordDocDeletion:
    def setup_method(self):
        _UPGRADED_TABLES.clear()

    def test_record_doc_deletion_invokes_ensure_columns_and_insert(self):
        tenant_id = "tenant_123"
        kb_id = "kb_456"
        doc_id = "doc_789"

        mock_conn = MagicMock()
        mock_conn.index_exist.return_value = True
        mock_conn.ensure_columns = MagicMock()

        with patch("rag.svr.task_executor_refactor.dataset_structure_merger.settings") as mock_settings, patch("rag.svr.task_executor_refactor.dataset_structure_merger.search") as mock_search:
            mock_settings.docStoreConn = mock_conn
            mock_search.index_name.return_value = "ragflow_tenant_123"

            record_doc_deletion(tenant_id, kb_id, doc_id)

            mock_conn.index_exist.assert_called_once_with("ragflow_tenant_123", kb_id)
            mock_conn.ensure_columns.assert_called_once_with(
                "ragflow_tenant_123",
                kb_id,
                {"deleted_doc_id": {"type": "varchar", "default": "", "analyzer": "whitespace-#"}},
            )
            mock_conn.insert.assert_called_once()
            inserted_rows, index, kb = mock_conn.insert.call_args[0]
            assert index == "ragflow_tenant_123"
            assert kb == kb_id
            assert len(inserted_rows) == 1
            assert inserted_rows[0]["deleted_doc_id"] == doc_id
            assert inserted_rows[0]["kb_id"] == kb_id

    def test_record_doc_deletion_caches_upgraded_tables_per_kb(self):
        tenant_id = "tenant_123"
        kb_id1 = "kb_456"
        kb_id2 = "kb_789"

        mock_conn = MagicMock()
        mock_conn.index_exist.return_value = True
        mock_conn.ensure_columns = MagicMock()

        with patch("rag.svr.task_executor_refactor.dataset_structure_merger.settings") as mock_settings, patch("rag.svr.task_executor_refactor.dataset_structure_merger.search") as mock_search:
            mock_settings.docStoreConn = mock_conn
            mock_search.index_name.return_value = "ragflow_tenant_123"

            record_doc_deletion(tenant_id, kb_id1, "doc_1")
            record_doc_deletion(tenant_id, kb_id1, "doc_2")
            record_doc_deletion(tenant_id, kb_id2, "doc_3")

            # ensure_columns is called twice: once for kb_456, once for kb_789
            assert mock_conn.ensure_columns.call_count == 2
            assert mock_conn.insert.call_count == 3
            inserted_doc_ids = [call.args[0][0]["deleted_doc_id"] for call in mock_conn.insert.call_args_list]
            assert inserted_doc_ids == ["doc_1", "doc_2", "doc_3"]

    def test_record_doc_deletion_retries_ensure_columns_on_insert_failure(self):
        tenant_id = "tenant_123"
        kb_id = "kb_456"

        mock_conn = MagicMock()
        mock_conn.index_exist.return_value = True
        mock_conn.ensure_columns = MagicMock()
        # First insert fails, second succeeds
        mock_conn.insert.side_effect = [RuntimeError("insert failed"), None]

        with patch("rag.svr.task_executor_refactor.dataset_structure_merger.settings") as mock_settings, patch("rag.svr.task_executor_refactor.dataset_structure_merger.search") as mock_search:
            mock_settings.docStoreConn = mock_conn
            mock_search.index_name.return_value = "ragflow_tenant_123"

            with pytest.raises(RuntimeError, match="insert failed"):
                record_doc_deletion(tenant_id, kb_id, "doc_1")
            assert (("ragflow_tenant_123", kb_id)) not in _UPGRADED_TABLES

            record_doc_deletion(tenant_id, kb_id, "doc_2")
            assert (("ragflow_tenant_123", kb_id)) in _UPGRADED_TABLES

            # ensure_columns was retried on the second attempt
            assert mock_conn.ensure_columns.call_count == 2
            assert mock_conn.insert.call_count == 2

    def test_record_doc_deletion_when_index_not_exist(self):
        mock_conn = MagicMock()
        mock_conn.index_exist.return_value = False
        mock_conn.ensure_columns = MagicMock()

        with patch("rag.svr.task_executor_refactor.dataset_structure_merger.settings") as mock_settings, patch("rag.svr.task_executor_refactor.dataset_structure_merger.search") as mock_search:
            mock_settings.docStoreConn = mock_conn
            mock_search.index_name.return_value = "ragflow_tenant_123"

            record_doc_deletion("tenant_123", "kb_456", "doc_789")

            mock_conn.ensure_columns.assert_not_called()
            mock_conn.insert.assert_not_called()

    def test_record_doc_deletion_when_ensure_columns_missing(self):
        mock_conn = MagicMock(spec=["index_exist", "insert"])
        mock_conn.index_exist.return_value = True

        with patch("rag.svr.task_executor_refactor.dataset_structure_merger.settings") as mock_settings, patch("rag.svr.task_executor_refactor.dataset_structure_merger.search") as mock_search:
            mock_settings.docStoreConn = mock_conn
            mock_search.index_name.return_value = "ragflow_tenant_123"

            record_doc_deletion("tenant_123", "kb_456", "doc_789")

            mock_conn.insert.assert_called_once()

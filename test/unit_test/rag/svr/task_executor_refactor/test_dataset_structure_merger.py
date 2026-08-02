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
"""Unit tests for ``record_doc_deletion`` in ``dataset_structure_merger.py``."""

from unittest.mock import MagicMock, patch

import pytest

pytestmark = pytest.mark.p2

from rag.svr.task_executor_refactor.dataset_structure_merger import record_doc_deletion


class TestRecordDocDeletion:
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
                {"deleted_doc_id": {"type": "varchar", "default": ""}},
            )
            mock_conn.insert.assert_called_once()
            inserted_rows, index, kb = mock_conn.insert.call_args[0]
            assert index == "ragflow_tenant_123"
            assert kb == kb_id
            assert len(inserted_rows) == 1
            assert inserted_rows[0]["deleted_doc_id"] == doc_id
            assert inserted_rows[0]["kb_id"] == kb_id

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

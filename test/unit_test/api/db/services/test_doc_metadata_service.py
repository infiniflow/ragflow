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
"""Unit tests for ``DocMetadataService`` metadata lookup and deletion methods."""

from unittest.mock import MagicMock, patch

import pytest

pytestmark = pytest.mark.p2

from api.db.services.doc_metadata_service import DocMetadataService


class TestDocMetadataServiceConnectorGetCalls:
    """Verify that ``DocMetadataService`` passes ``[]`` as knowledgebase_ids
    when interacting with ``docStoreConn.get`` for per-tenant metadata indexes,
    avoiding the creation of non-existent ``<tenant>_<kb>`` table names."""

    def test_delete_document_metadata_passes_empty_kb_list_to_get(self):
        mock_doc_store = MagicMock()
        mock_doc_store.index_exist.return_value = True
        mock_doc_store.get.return_value = None  # No metadata found

        with (
            patch("api.db.services.doc_metadata_service.settings") as mock_settings,
            patch("api.db.db_models.DB.connect"),
            patch("api.db.db_models.DB.connection_context"),
        ):
            mock_settings.docStoreConn = mock_doc_store

            res = DocMetadataService.delete_document_metadata("doc_789", "kb_123", tenant_id="tenant_456")

            assert res is True
            mock_doc_store.get.assert_called_once_with(
                "doc_789",
                "ragflow_doc_meta_tenant_456",
                [],
            )

    def test_get_document_metadata_passes_empty_kb_list_to_get(self):
        mock_doc_store = MagicMock()
        mock_doc_store.get.return_value = {"id": "doc_789", "meta_fields": {"author": "alice"}}

        mock_doc = MagicMock()
        mock_doc.knowledgebase.tenant_id = "tenant_456"
        mock_doc.kb_id = "kb_123"

        with (
            patch("api.db.services.doc_metadata_service.settings") as mock_settings,
            patch("api.db.services.doc_metadata_service.Document") as mock_document_model,
            patch("api.db.db_models.DB.connect"),
            patch("api.db.db_models.DB.connection_context"),
        ):
            mock_settings.docStoreConn = mock_doc_store
            mock_document_model.select.return_value.join.return_value.where.return_value.first.return_value = mock_doc

            result = DocMetadataService.get_document_metadata("doc_789")

            mock_doc_store.get.assert_called_once_with(
                "doc_789",
                "ragflow_doc_meta_tenant_456",
                [],
            )
            assert result == {"author": "alice"}

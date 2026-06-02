#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

"""
Unit tests for rag/svr/task_executor_refactor/raptor_utils.py module.
"""

import pytest
from unittest.mock import MagicMock, patch
from rag.svr.task_executor_refactor.raptor_utils import (
    get_raptor_chunk_field_map,
    delete_raptor_chunks,
)


class TestGetRaptorChunkFieldMap:
    """Tests for get_raptor_chunk_field_map function."""

    @pytest.mark.asyncio
    async def test_returns_primary_result_when_raptor_chunks_exist(self):
        """Test that primary result is returned when RAPTOR chunks exist."""
        from common import settings
        original_retriever = settings.docStoreConn
        
        mock_doc_store = MagicMock()
        mock_doc_store.search.return_value = {"chunk_1": {"raptor_kwd": "raptor", "extra": {"raptor_method": "raptor"}}}
        mock_doc_store.get_fields.return_value = {"chunk_1": {"raptor_kwd": "raptor", "extra": {"raptor_method": "raptor"}}}
        settings.docStoreConn = mock_doc_store

        try:
            with patch("rag.svr.task_executor_refactor.raptor_utils.thread_pool_exec") as mock_thread:
                async def mock_exec(*args, **kwargs):
                    return {"chunk_1": {"raptor_kwd": "raptor", "extra": {"raptor_method": "raptor"}}}
                mock_thread.side_effect = mock_exec

                with patch("rag.svr.task_executor_refactor.raptor_utils.collect_raptor_chunk_ids") as mock_collect:
                    mock_collect.return_value = {"chunk_1"}

                    result = await get_raptor_chunk_field_map("doc_1", "tenant_1", "kb_1")

                    assert "chunk_1" in result
        finally:
            settings.docStoreConn = original_retriever

    @pytest.mark.asyncio
    async def test_falls_back_to_secondary_search_when_no_raptor_chunks(self):
        """Test that fallback search is used when no RAPTOR chunks found."""
        from common import settings
        original_retriever = settings.docStoreConn
        
        mock_doc_store = MagicMock()
        settings.docStoreConn = mock_doc_store

        try:
            call_count = 0
            async def mock_exec(*args, **kwargs):
                nonlocal call_count
                call_count += 1
                if call_count == 1:
                    return {}  # Primary returns empty
                else:
                    return {"chunk_1": {"raptor_kwd": "raptor"}}  # Fallback

            with patch("rag.svr.task_executor_refactor.raptor_utils.thread_pool_exec") as mock_thread:
                mock_thread.side_effect = mock_exec

                with patch("rag.svr.task_executor_refactor.raptor_utils.collect_raptor_chunk_ids") as mock_collect:
                    mock_collect.return_value = set()  # Primary has no RAPTOR chunks

                    _ = await get_raptor_chunk_field_map("doc_1", "tenant_1", "kb_1")

                    # Should have called thread_pool_exec twice (primary + fallback)
                    assert mock_thread.call_count == 2
        finally:
            settings.docStoreConn = original_retriever

    @pytest.mark.asyncio
    async def test_handles_fallback_search_exception(self):
        """Test that exception in fallback search is handled gracefully."""
        from common import settings
        original_retriever = settings.docStoreConn
        
        mock_doc_store = MagicMock()
        mock_doc_store.get_fields.return_value = {}
        settings.docStoreConn = mock_doc_store

        try:
            call_count = 0
            async def mock_exec(*args, **kwargs):
                nonlocal call_count
                call_count += 1
                if call_count == 1:
                    return {}  # Primary returns empty
                else:
                    raise Exception("Fallback search failed")  # Fallback will raise exception

            with patch("rag.svr.task_executor_refactor.raptor_utils.thread_pool_exec") as mock_thread:
                mock_thread.side_effect = mock_exec

                with patch("rag.svr.task_executor_refactor.raptor_utils.collect_raptor_chunk_ids") as mock_collect:
                    mock_collect.return_value = set()  # Primary has no RAPTOR chunks

                    # Fallback will raise exception, but it should be caught
                    result = await get_raptor_chunk_field_map("doc_1", "tenant_1", "kb_1")

                    # Should return primary result (empty)
                    assert result == {}
        finally:
            settings.docStoreConn = original_retriever


class TestDeleteRaptorChunks:
    """Tests for delete_raptor_chunks function."""

    @pytest.mark.asyncio
    async def test_deletes_all_chunks_when_keep_method_is_none(self):
        """Test that all RAPTOR chunks are deleted when keep_method is None."""
        from common import settings
        original_retriever = settings.docStoreConn
        
        mock_doc_store = MagicMock()
        settings.docStoreConn = mock_doc_store

        try:
            with patch("rag.svr.task_executor_refactor.raptor_utils.thread_pool_exec") as mock_thread:
                mock_thread.return_value = 0

                _ = await delete_raptor_chunks("doc_1", "tenant_1", "kb_1", keep_method=None)

                mock_thread.assert_called_once()
                # Verify delete was called with correct condition
                call_args = mock_thread.call_args
                assert call_args[0][0] == settings.docStoreConn.delete
        finally:
            settings.docStoreConn = original_retriever

    @pytest.mark.asyncio
    async def test_returns_0_when_no_stale_chunks(self):
        """Test that 0 is returned when no stale chunks to delete."""
        with patch("rag.svr.task_executor_refactor.raptor_utils.get_raptor_chunk_field_map") as mock_get_map:
            mock_get_map.return_value = {}

            with patch("rag.svr.task_executor_refactor.raptor_utils.collect_raptor_chunk_ids") as mock_collect:
                mock_collect.return_value = set()  # No stale chunks

                result = await delete_raptor_chunks("doc_1", "tenant_1", "kb_1", keep_method="raptor")

                assert result == 0
                mock_collect.assert_called_once()

    @pytest.mark.asyncio
    async def test_deletes_stale_chunks_when_keep_method_specified(self):
        """Test that stale chunks are deleted when keep_method is specified."""
        from common import settings
        original_retriever = settings.docStoreConn
        
        mock_doc_store = MagicMock()
        settings.docStoreConn = mock_doc_store

        try:
            with patch("rag.svr.task_executor_refactor.raptor_utils.get_raptor_chunk_field_map") as mock_get_map:
                mock_get_map.return_value = {
                    "chunk_1": {"raptor_kwd": "raptor", "extra": {"raptor_method": "psi"}},
                    "chunk_2": {"raptor_kwd": "raptor", "extra": {"raptor_method": "raptor"}}
                }

                with patch("rag.svr.task_executor_refactor.raptor_utils.collect_raptor_chunk_ids") as mock_collect:
                    mock_collect.return_value = {"chunk_1"}  # Only chunk_1 is stale (psi, not raptor)

                    with patch("rag.svr.task_executor_refactor.raptor_utils.thread_pool_exec") as mock_thread:
                        mock_thread.return_value = 0

                        _ = await delete_raptor_chunks("doc_1", "tenant_1", "kb_1", keep_method="raptor")

                        # Should have called delete for stale chunks
                        mock_thread.assert_called_once()
        finally:
            settings.docStoreConn = original_retriever

    @pytest.mark.asyncio
    async def test_logs_info_when_removing_stale_chunks(self):
        """Test that info is logged when removing stale chunks."""
        from common import settings
        original_retriever = settings.docStoreConn
        
        mock_doc_store = MagicMock()
        settings.docStoreConn = mock_doc_store

        try:
            with patch("rag.svr.task_executor_refactor.raptor_utils.get_raptor_chunk_field_map") as mock_get_map:
                mock_get_map.return_value = {
                    "chunk_1": {"raptor_kwd": "raptor", "extra": {"raptor_method": "psi"}}
                }

                with patch("rag.svr.task_executor_refactor.raptor_utils.collect_raptor_chunk_ids") as mock_collect:
                    mock_collect.return_value = {"chunk_1"}

                    with patch("rag.svr.task_executor_refactor.raptor_utils.thread_pool_exec") as mock_thread:
                        mock_thread.return_value = 0

                        with patch("rag.svr.task_executor_refactor.raptor_utils.logging.info") as mock_log:
                            await delete_raptor_chunks("doc_1", "tenant_1", "kb_1", keep_method="raptor")

                            # Should have logged the removal
                            mock_log.assert_called()
        finally:
            settings.docStoreConn = original_retriever

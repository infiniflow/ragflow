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
Unit tests for PostProcessor module.
"""

import pytest
from unittest.mock import MagicMock, patch, AsyncMock
from rag.svr.task_executor_refactor.post_processor import PostProcessor


class TestPostProcessorInit:
    """Tests for PostProcessor initialization."""

    def test_init_stores_task_context(self):
        """Test that task context is stored."""
        ctx = MagicMock()
        service = PostProcessor(ctx=ctx)
        assert service._task_context is ctx


class TestPostProcessorProcessTableParserMetadata:
    """Tests for process_table_parser_metadata method."""

    @pytest.mark.asyncio
    async def test_skips_non_table_parser(self):
        """Test that processing is skipped for non-table parser."""
        ctx = MagicMock()
        ctx.parser_id = "naive"
        service = PostProcessor(ctx=ctx)

        await service.process_table_parser_metadata("doc_1", [])

        # Should return early without any further processing

    @pytest.mark.asyncio
    async def test_skips_when_not_manual_column_mode(self):
        """Test that processing is skipped when not in manual column mode."""
        ctx = MagicMock()
        ctx.parser_id = "table"
        ctx.raw_task = {}
        service = PostProcessor(ctx=ctx)

        with patch("rag.svr.task_executor_refactor.post_processor.merge_table_parser_config_from_kb") as mock_merge:
            mock_merge.return_value = {"table_column_mode": "auto"}
            await service.process_table_parser_metadata("doc_1", [])

            mock_merge.assert_called_once()

    @pytest.mark.asyncio
    async def test_processes_table_parser_manual_mode(self):
        """Test that table parser in manual mode aggregates and persists metadata."""
        ctx = MagicMock()
        ctx.parser_id = "table"
        ctx.raw_task = {"parser_config": {}}
        ctx.write_interceptor = None
        chunks = [{"col_key": "val"}]

        with (
            patch("rag.svr.task_executor_refactor.post_processor.merge_table_parser_config_from_kb") as mock_merge,
            patch("rag.svr.task_executor_refactor.post_processor.aggregate_table_doc_metadata") as mock_agg,
            patch("rag.svr.task_executor_refactor.post_processor.table_parser_strip_doc_metadata_keys") as mock_strip,
            patch("rag.svr.task_executor_refactor.post_processor.update_metadata_to") as mock_update,
            patch("rag.svr.task_executor_refactor.post_processor.DocMetadataService") as mock_meta,
        ):
            mock_merge.return_value = {"table_column_mode": "manual"}
            mock_agg.return_value = {"col_key": ["val1", "val2"]}
            mock_strip.return_value = set()
            mock_meta.get_document_metadata.return_value = {}
            mock_update.return_value = {"col_key": ["val1", "val2"]}

            service = PostProcessor(ctx=ctx)
            await service.process_table_parser_metadata("doc_1", chunks)

            mock_agg.assert_called_once_with(chunks, ctx.raw_task)
            mock_meta.update_document_metadata.assert_called_once()

    @pytest.mark.asyncio
    async def test_processes_table_parser_with_write_interceptor(self):
        """Test table parser with write interceptor bypasses DB."""
        ctx = MagicMock()
        ctx.parser_id = "table"
        ctx.raw_task = {}
        ctx.write_interceptor = MagicMock()

        with (
            patch("rag.svr.task_executor_refactor.post_processor.merge_table_parser_config_from_kb") as mock_merge,
            patch("rag.svr.task_executor_refactor.post_processor.aggregate_table_doc_metadata") as mock_agg,
            patch("rag.svr.task_executor_refactor.post_processor.table_parser_strip_doc_metadata_keys") as mock_strip,
            patch("rag.svr.task_executor_refactor.post_processor.DocMetadataService") as mock_meta,
        ):
            mock_merge.return_value = {"table_column_mode": "manual"}
            mock_agg.return_value = {"key": ["v"]}
            mock_strip.return_value = set()
            mock_meta.get_document_metadata.return_value = {}

            service = PostProcessor(ctx=ctx)
            await service.process_table_parser_metadata("doc_1", [])

            ctx.write_interceptor.intercept.assert_called_once_with("DocMetadataService.update_document_metadata")


class TestPostProcessorInsertTocChunk:
    """Tests for insert_toc_chunk method."""

    @pytest.mark.asyncio
    async def test_returns_false_for_none_chunk(self):
        """Test that method returns False when chunk is None."""
        ctx = MagicMock()
        service = PostProcessor(ctx=ctx)
        chunk_service = MagicMock()

        result = await service.insert_toc_chunk(None, chunk_service)

        assert result is False
        chunk_service.insert_chunks.assert_not_called()

    @pytest.mark.asyncio
    async def test_checks_cancellation(self):
        """Test that cancellation is checked."""
        ctx = MagicMock()
        ctx.id = "task_1"
        ctx.has_canceled_func = MagicMock(return_value=True)
        ctx.progress_cb = MagicMock()
        service = PostProcessor(ctx=ctx)
        chunk_service = MagicMock()
        toc_chunk = {"id": "toc_1"}

        result = await service.insert_toc_chunk(toc_chunk, chunk_service)

        assert result is False
        ctx.progress_cb.assert_called_with(-1, msg="Task has been canceled.")

    @pytest.mark.asyncio
    async def test_inserts_toc_chunk_successfully(self):
        """Test successful TOC chunk insertion."""
        ctx = MagicMock()
        ctx.id = "task_1"
        ctx.tenant_id = "tenant_1"
        ctx.kb_id = "kb_1"
        ctx.has_canceled_func = MagicMock(return_value=False)
        service = PostProcessor(ctx=ctx)
        chunk_service = AsyncMock()
        chunk_service.insert_chunks = AsyncMock(return_value=True)
        toc_chunk = {"id": "toc_1"}

        result = await service.insert_toc_chunk(toc_chunk, chunk_service)

        assert result is True
        chunk_service.insert_chunks.assert_called_once_with("task_1", "tenant_1", "kb_1", [toc_chunk])

    @pytest.mark.asyncio
    async def test_handles_insert_failure(self):
        """Test handling of insert failure."""
        ctx = MagicMock()
        ctx.id = "task_1"
        ctx.tenant_id = "tenant_1"
        ctx.kb_id = "kb_1"
        ctx.has_canceled_func = MagicMock(return_value=False)
        service = PostProcessor(ctx=ctx)
        chunk_service = AsyncMock()
        chunk_service.insert_chunks = AsyncMock(return_value=False)
        toc_chunk = {"id": "toc_1"}

        result = await service.insert_toc_chunk(toc_chunk, chunk_service)

        assert result is False

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
Unit tests for ChunkBuilder module.
"""

import pytest
from unittest.mock import MagicMock, patch, AsyncMock
from rag.svr.task_executor_refactor.chunk_builder import (
    get_parser,
    run_chunking,
    extract_outline,
)


class TestGetParser:
    """Tests for get_parser function."""

    @pytest.mark.parametrize("parser_id", [
        "naive", "general", "table", "paper", "book",
        "picture", "audio", "email", "presentation", "manual",
        "laws", "qa", "resume", "one", "tag",
    ])
    def test_get_parser_returns_non_none(self, parser_id):
        """Test that get_parser returns non-None for all parser types."""
        parser = get_parser(parser_id)
        assert parser is not None

    def test_get_parser_kg(self):
        """Test getting kg parser (maps to naive)."""
        from common.constants import ParserType
        parser = get_parser(ParserType.KG.value)
        assert parser is not None


class TestRunChunking:
    """Tests for run_chunking function."""

    def _create_mock_context(self):
        """Helper to create a mock TaskContext."""
        ctx = MagicMock()
        ctx.name = "test.pdf"
        ctx.location = "/path/to/test.pdf"
        ctx.from_page = 0
        ctx.to_page = -1
        ctx.language = "en"
        ctx.kb_id = "kb_1"
        ctx.parser_config = {}
        ctx.tenant_id = "tenant_1"
        ctx.progress_cb = MagicMock()
        ctx.raw_task = {}
        ctx.chunk_limiter = MagicMock()
        ctx.chunk_limiter.__aenter__ = AsyncMock()
        ctx.chunk_limiter.__aexit__ = AsyncMock()
        return ctx

    @pytest.mark.asyncio
    async def test_run_chunking_success(self):
        """Test successful chunking."""
        ctx = self._create_mock_context()

        mock_chunker = MagicMock()
        mock_chunker.chunk = MagicMock(return_value=[{"content_with_weight": "chunk1"}])

        with patch("rag.svr.task_executor_refactor.chunk_builder.thread_pool_exec") as mock_thread:
            # thread_pool_exec returns an awaitable that returns the list
            mock_thread.return_value = [{"content_with_weight": "chunk1"}]

            result = await run_chunking(mock_chunker, b"binary", ctx)

            assert result is not None
            assert len(result) == 1

    @pytest.mark.asyncio
    async def test_run_chunking_with_parser_config(self):
        """Test chunking merges table parser config."""
        ctx = self._create_mock_context()
        ctx.raw_task = {"parser_config": {"chunk_token_num": 128}}

        mock_chunker = MagicMock()
        mock_chunker.chunk = MagicMock(return_value=[])

        with patch("rag.svr.task_executor_refactor.chunk_builder.thread_pool_exec") as mock_thread:
            mock_thread.return_value = []

            with patch("rag.svr.task_executor_refactor.chunk_builder.merge_table_parser_config_from_kb") as mock_merge:
                mock_merge.return_value = {"chunk_token_num": 128}

                await run_chunking(mock_chunker, b"binary", ctx)

                mock_merge.assert_called_once_with(ctx.raw_task)

    @pytest.mark.asyncio
    async def test_run_chunking_exception(self):
        """Test chunking handles exception."""
        ctx = self._create_mock_context()

        mock_chunker = MagicMock()
        mock_chunker.chunk = MagicMock(side_effect=Exception("Test error"))

        with patch("rag.svr.task_executor_refactor.chunk_builder.thread_pool_exec") as mock_thread:
            mock_thread.side_effect = Exception("Test error")

            with pytest.raises(Exception):
                await run_chunking(mock_chunker, b"binary", ctx)

            # Verify progress_cb was called with error message
            ctx.progress_cb.assert_called()


class TestExtractOutline:
    """Tests for extract_outline function."""

    def _create_mock_context(self):
        """Helper to create a mock TaskContext."""
        ctx = MagicMock()
        ctx.doc_id = "doc_1"
        ctx.write_interceptor = None
        ctx.progress_cb = MagicMock()
        return ctx

    @pytest.mark.asyncio
    async def test_extract_outline_with_data(self):
        """Test outline extraction when outline data is present."""
        ctx = self._create_mock_context()

        outline_data = [{"title": "Chapter 1", "page": 1}]
        cks = [{"__outline__": outline_data}]

        mock_rec_ctx = MagicMock()
        ctx.recording_context = mock_rec_ctx

        with patch("rag.svr.task_executor_refactor.chunk_builder.DocMetadataService") as mock_meta:
            mock_meta.get_document_metadata.return_value = {}
            mock_meta.update_document_metadata = MagicMock()

            await extract_outline(cks, ctx)

            mock_rec_ctx.record.assert_called_with("outline_data", outline_data)
            # Outline should be popped from first chunk
            assert "__outline__" not in cks[0]
            mock_meta.update_document_metadata.assert_called_once()

    @pytest.mark.asyncio
    async def test_extract_outline_without_data(self):
        """Test outline extraction when no outline data."""
        ctx = self._create_mock_context()

        cks = [{"content_with_weight": "test"}]

        mock_rec_ctx = MagicMock()
        ctx.recording_context = mock_rec_ctx

        await extract_outline(cks, ctx)

        mock_rec_ctx.record.assert_called_with("outline_data", None)

    @pytest.mark.asyncio
    async def test_extract_outline_empty_chunks(self):
        """Test outline extraction with empty chunks list."""
        ctx = self._create_mock_context()

        mock_rec_ctx = MagicMock()
        ctx.recording_context = mock_rec_ctx

        await extract_outline([], ctx)

        mock_rec_ctx.record.assert_called_with("outline_data", None)

    @pytest.mark.asyncio
    async def test_extract_outline_with_write_interceptor(self):
        """Test outline extraction with write interceptor."""
        ctx = self._create_mock_context()
        ctx.write_interceptor = MagicMock()

        outline_data = [{"title": "Chapter 1", "page": 1}]
        cks = [{"__outline__": outline_data}]

        mock_rec_ctx = MagicMock()
        ctx.recording_context = mock_rec_ctx

        await extract_outline(cks, ctx)

        ctx.write_interceptor.intercept.assert_called_once_with(
            "DocMetadataService.update_document_metadata"
        )

    @pytest.mark.asyncio
    async def test_extract_outline_persistence_exception(self):
        """Test outline extraction handles persistence exception."""
        ctx = self._create_mock_context()

        outline_data = [{"title": "Chapter 1", "page": 1}]
        cks = [{"__outline__": outline_data}]

        mock_rec_ctx = MagicMock()
        ctx.recording_context = mock_rec_ctx

        with patch("rag.svr.task_executor_refactor.chunk_builder.DocMetadataService") as mock_meta:
            mock_meta.get_document_metadata.return_value = {}
            mock_meta.update_document_metadata.side_effect = Exception("DB error")

            # Should not raise exception, just log warning
            await extract_outline(cks, ctx)

            mock_rec_ctx.record.assert_called_with("outline_data", outline_data)

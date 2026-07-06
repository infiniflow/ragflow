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
from unittest.mock import MagicMock, patch
from rag.svr.task_executor_refactor.chunk_builder import (
    get_parser,
    run_chunking,
    extract_outline,
)
from test.unit_test.rag.svr.task_executor_refactor.conftest import make_task_context


class TestGetParser:
    """Tests for get_parser function."""

    @pytest.mark.parametrize(
        "parser_id",
        [
            "naive",
            "general",
            "table",
            "paper",
            "book",
            "picture",
            "audio",
            "email",
            "presentation",
            "manual",
            "laws",
            "qa",
            "resume",
            "one",
            "tag",
        ],
    )
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

    @pytest.mark.asyncio
    async def test_run_chunking_success(self):
        """Test successful chunking."""
        ctx = make_task_context()

        mock_chunker = MagicMock()
        mock_chunker.chunk = MagicMock(return_value=[{"content_with_weight": "chunk1"}])

        with patch("rag.svr.task_executor_refactor.chunk_builder.thread_pool_exec") as mock_thread:
            mock_thread.return_value = [{"content_with_weight": "chunk1"}]
            result = await run_chunking(mock_chunker, b"binary", ctx)
            assert result is not None
            assert len(result) == 1

    @pytest.mark.asyncio
    async def test_run_chunking_with_parser_config(self):
        """Test chunking merges table parser config."""
        ctx = make_task_context()
        ctx.raw_task = {"parser_config": {"chunk_token_num": 128}}

        mock_chunker = MagicMock()
        mock_chunker.chunk = MagicMock(return_value=[])

        with (
            patch("rag.svr.task_executor_refactor.chunk_builder.thread_pool_exec") as mock_thread,
            patch("rag.svr.task_executor_refactor.chunk_builder.merge_table_parser_config_from_kb") as mock_merge,
        ):
            mock_thread.return_value = []
            mock_merge.return_value = {"chunk_token_num": 128}
            await run_chunking(mock_chunker, b"binary", ctx)
            mock_merge.assert_called_once_with(ctx.raw_task)

    @pytest.mark.asyncio
    async def test_run_chunking_exception(self):
        """Test chunking handles exception."""
        ctx = make_task_context()

        mock_chunker = MagicMock()
        mock_chunker.chunk = MagicMock(side_effect=Exception("Test error"))

        with patch("rag.svr.task_executor_refactor.chunk_builder.thread_pool_exec") as mock_thread:
            mock_thread.side_effect = Exception("Test error")
            with pytest.raises(Exception):
                await run_chunking(mock_chunker, b"binary", ctx)
            ctx.progress_cb.assert_called()


class TestExtractOutline:
    """Tests for extract_outline function."""

    @staticmethod
    def _ctx(recording_ctx=None, **overrides):
        ctx = make_task_context(**overrides)
        ctx.recording_context = recording_ctx or MagicMock()
        return ctx

    @pytest.mark.asyncio
    async def test_extract_outline_with_data(self):
        """Test outline extraction when outline data is present."""
        ctx = self._ctx()

        outline_data = [{"title": "Chapter 1", "page": 1}]
        cks = [{"__outline__": outline_data}]

        with patch("rag.svr.task_executor_refactor.chunk_builder.DocMetadataService") as mock_meta:
            mock_meta.get_document_metadata.return_value = {}
            mock_meta.update_document_metadata = MagicMock()
            await extract_outline(cks, ctx)
            ctx.recording_context.record.assert_called_with("outline_data", outline_data)
            assert "__outline__" not in cks[0]
            mock_meta.update_document_metadata.assert_called_once()

    @pytest.mark.asyncio
    async def test_extract_outline_without_data(self):
        """Test outline extraction when no outline data."""
        ctx = self._ctx()
        cks = [{"content_with_weight": "test"}]
        await extract_outline(cks, ctx)
        ctx.recording_context.record.assert_called_with("outline_data", None)

    @pytest.mark.asyncio
    async def test_extract_outline_empty_chunks(self):
        """Test outline extraction with empty chunks list."""
        ctx = self._ctx()
        await extract_outline([], ctx)
        ctx.recording_context.record.assert_called_with("outline_data", None)

    @pytest.mark.asyncio
    async def test_extract_outline_with_write_interceptor(self):
        """Test outline extraction with write interceptor."""
        ctx = self._ctx(write_interceptor=MagicMock())

        outline_data = [{"title": "Chapter 1", "page": 1}]
        cks = [{"__outline__": outline_data}]
        await extract_outline(cks, ctx)
        ctx.write_interceptor.intercept.assert_called_once_with("DocMetadataService.update_document_metadata")

    @pytest.mark.asyncio
    async def test_extract_outline_persistence_exception(self):
        """Test outline extraction handles persistence exception."""
        ctx = self._ctx()

        outline_data = [{"title": "Chapter 1", "page": 1}]
        cks = [{"__outline__": outline_data}]

        with patch("rag.svr.task_executor_refactor.chunk_builder.DocMetadataService") as mock_meta:
            mock_meta.get_document_metadata.return_value = {}
            mock_meta.update_document_metadata.side_effect = Exception("DB error")
            await extract_outline(cks, ctx)
            ctx.recording_context.record.assert_called_with("outline_data", outline_data)

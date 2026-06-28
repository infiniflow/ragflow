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
Unit tests for ChunkService module.

Note: After refactoring, some functionality has been moved to:
- chunk_builder.py: Parser factory, run_chunking, extract_outline
- chunk_post_processor.py: Keyword extraction, question generation, metadata, tagging

This test file now focuses on ChunkService-specific functionality:
- build_chunks orchestration
- _prepare_docs_and_upload
- insert_chunks and related methods
"""

import pytest
from unittest.mock import MagicMock, patch, AsyncMock
from rag.svr.task_executor_refactor.chunk_service import ChunkService
from test.unit_test.rag.svr.task_executor_refactor.conftest import make_task_context


class TestChunkServiceInit:
    """Tests for ChunkService initialization."""

    def test_init_stores_task_context(self):
        """Test that task context is stored."""
        ctx = MagicMock()
        service = ChunkService(ctx=ctx)
        assert service._task_context is ctx


class TestChunkServiceBuildChunks:
    """Tests for build_chunks method."""


    @pytest.mark.asyncio
    async def test_build_chunks_file_size_exceeded(self):
        """Test build_chunks returns empty list when file size exceeds limit."""
        ctx = make_task_context(size=1000000000)  # Very large size

        service = ChunkService(ctx=ctx)

        with patch("rag.svr.task_executor_refactor.chunk_service.settings") as mock_settings:
            mock_settings.DOC_MAXIMUM_SIZE = 1000  # Small limit

            mock_rec_ctx = MagicMock()
            ctx.recording_context = mock_rec_ctx

            result = await service.build_chunks(b"test binary")

            assert result == []
            mock_rec_ctx.record.assert_any_call("file_size_exceeded", True)

    @pytest.mark.asyncio
    async def test_build_chunks_file_size_ok(self):
        """Test build_chunks proceeds when file size is within limit."""
        ctx = make_task_context(size=1000)

        service = ChunkService(ctx=ctx)

        with patch("rag.svr.task_executor_refactor.chunk_service.settings") as mock_settings:
            mock_settings.DOC_MAXIMUM_SIZE = 10000000  # Large limit

            mock_rec_ctx = MagicMock()
            ctx.recording_context = mock_rec_ctx

            with patch("rag.svr.task_executor_refactor.chunk_service.get_parser") as mock_get_parser:
                mock_parser = MagicMock()
                mock_get_parser.return_value = mock_parser

                with patch("rag.svr.task_executor_refactor.chunk_service.run_chunking", new_callable=AsyncMock) as mock_run_chunking:
                    mock_run_chunking.return_value = [{"content_with_weight": "test"}]

                    with patch("rag.svr.task_executor_refactor.chunk_service.extract_outline", new_callable=AsyncMock):
                        with patch.object(service, '_prepare_docs_and_upload', new_callable=AsyncMock) as mock_prepare:
                            mock_prepare.return_value = [{"id": "chunk_1", "content_with_weight": "test"}]

                            await service.build_chunks(b"test binary")

                            mock_rec_ctx.record.assert_any_call("file_size_exceeded", False)
                            mock_rec_ctx.record.assert_any_call("parser_id", "naive")
                            mock_get_parser.assert_called_once_with("naive")

    @pytest.mark.asyncio
    @pytest.mark.parametrize("task_kwargs,func_path,func_name", [
        ({"parser_config": {"auto_keywords": 5}}, "extract_keywords", "extract_keywords"),
        ({"parser_config": {"auto_questions": 3}}, "generate_questions", "generate_questions"),
        ({"kb_parser_config": {"tag_kb_ids": ["kb_1"]}}, "apply_tags", "apply_tags"),
        ({"parser_config": {"enable_metadata": True, "metadata": [{"name": "category", "type": "string"}]}},
         "generate_metadata", "generate_metadata"),
    ])
    async def test_build_chunks_with_post_processing(self, task_kwargs, func_path, func_name):
        """Test build_chunks triggers post-processing when configured."""
        ctx = make_task_context(**task_kwargs)
        service = ChunkService(ctx=ctx)

        mock_rec_ctx = MagicMock()
        ctx.recording_context = mock_rec_ctx

        with patch("rag.svr.task_executor_refactor.chunk_service.settings") as mock_settings, \
             patch("rag.svr.task_executor_refactor.chunk_service.get_parser") as mock_get_parser, \
             patch("rag.svr.task_executor_refactor.chunk_service.run_chunking", new_callable=AsyncMock) as mock_run_chunking, \
             patch("rag.svr.task_executor_refactor.chunk_service.extract_outline", new_callable=AsyncMock), \
             patch.object(service, '_prepare_docs_and_upload', new_callable=AsyncMock) as mock_prepare, \
             patch(f"rag.svr.task_executor_refactor.chunk_service.{func_path}", new_callable=AsyncMock) as mock_fn:
            mock_settings.DOC_MAXIMUM_SIZE = 10000000
            mock_get_parser.return_value = MagicMock()
            mock_run_chunking.return_value = []
            mock_prepare.return_value = [{"id": "chunk_1", "content_with_weight": "test"}]
            await service.build_chunks(b"test binary")
            mock_fn.assert_called_once()


class TestChunkServicePrepareDocsAndUpload:
    """Tests for _prepare_docs_and_upload method."""


    @pytest.mark.asyncio
    async def test_prepare_docs_and_upload_basic(self):
        """Test basic document preparation."""
        ctx = make_task_context()
        service = ChunkService(ctx=ctx)

        cks = [{"content_with_weight": "test chunk"}]

        with patch("rag.svr.task_executor_refactor.chunk_service.settings") as mock_settings:
            mock_settings.STORAGE_IMPL = MagicMock()
            mock_settings.STORAGE_IMPL.put = MagicMock()

            with patch("rag.svr.task_executor_refactor.chunk_service.image2id", new_callable=AsyncMock):

                docs = await service._prepare_docs_and_upload(cks)

                assert len(docs) == 1
                assert docs[0]["doc_id"] == "doc_1"
                assert docs[0]["kb_id"] == "kb_1"

    @pytest.mark.asyncio
    async def test_prepare_docs_and_upload_with_pagerank(self):
        """Test document preparation with pagerank."""
        ctx = make_task_context()
        ctx.pagerank = 5
        service = ChunkService(ctx=ctx)

        cks = [{"content_with_weight": "test chunk"}]

        with patch("rag.svr.task_executor_refactor.chunk_service.settings") as mock_settings:
            mock_settings.STORAGE_IMPL = MagicMock()

            with patch("rag.svr.task_executor_refactor.chunk_service.image2id", new_callable=AsyncMock):

                docs = await service._prepare_docs_and_upload(cks)

                assert docs[0].get("pagerank_fea") == 5


class TestChunkServiceInsertChunks:
    """Tests for insert_chunks method."""


    @pytest.mark.asyncio
    async def test_insert_chunks_success(self):
        """Test successful chunk insertion."""
        ctx = make_task_context()
        service = ChunkService(ctx=ctx)

        chunks = [
            {"id": "chunk_1", "content_with_weight": "test1"},
            {"id": "chunk_2", "content_with_weight": "test2"},
        ]

        with patch("rag.svr.task_executor_refactor.chunk_service.settings") as mock_settings:
            mock_settings.DOC_BULK_SIZE = 100
            mock_settings.docStoreConn = MagicMock()
            mock_settings.docStoreConn.insert = MagicMock(return_value=None)

            with patch("rag.svr.task_executor_refactor.chunk_service.search.index_name") as mock_index:
                mock_index.return_value = "test_index"

                with patch("rag.svr.task_executor_refactor.chunk_service.thread_pool_exec") as mock_thread:
                    mock_thread.return_value = None

                    with patch("rag.svr.task_executor_refactor.chunk_service.TaskService") as mock_task:
                        mock_task.update_chunk_ids = MagicMock()

                        result = await service.insert_chunks("task_1", "tenant_1", "kb_1", chunks)

                        assert result is True

    @pytest.mark.asyncio
    async def test_insert_chunks_canceled(self):
        """Test chunk insertion when task is canceled."""
        ctx = make_task_context()
        ctx.has_canceled_func = MagicMock(return_value=True)
        service = ChunkService(ctx=ctx)

        chunks = [{"id": "chunk_1", "content_with_weight": "test1"}]

        with patch("rag.svr.task_executor_refactor.chunk_service.settings") as mock_settings:
            mock_settings.DOC_BULK_SIZE = 100
            mock_settings.docStoreConn = MagicMock()
            mock_settings.docStoreConn.insert = MagicMock(return_value=None)

            with patch("rag.svr.task_executor_refactor.chunk_service.search.index_name") as mock_index:
                mock_index.return_value = "test_index"

                with patch("rag.svr.task_executor_refactor.chunk_service.thread_pool_exec") as mock_thread:
                    mock_thread.return_value = None

                    result = await service.insert_chunks("task_1", "tenant_1", "kb_1", chunks)

                    assert result is False
                    ctx.progress_cb.assert_called_with(-1, msg="Task has been canceled.")

    @pytest.mark.asyncio
    async def test_insert_chunks_doc_store_error(self):
        """Test chunk insertion when doc store returns error."""
        ctx = make_task_context()
        service = ChunkService(ctx=ctx)

        chunks = [{"id": "chunk_1", "content_with_weight": "test1"}]

        with patch("rag.svr.task_executor_refactor.chunk_service.settings") as mock_settings:
            mock_settings.DOC_BULK_SIZE = 100
            mock_settings.docStoreConn = MagicMock()
            mock_settings.docStoreConn.insert = MagicMock(return_value="Error message")

            with patch("rag.svr.task_executor_refactor.chunk_service.search.index_name") as mock_index:
                mock_index.return_value = "test_index"

                with patch("rag.svr.task_executor_refactor.chunk_service.thread_pool_exec") as mock_thread:
                    mock_thread.return_value = "Error"

                    with pytest.raises(Exception, match="Insert chunk error"):
                        await service.insert_chunks("task_1", "tenant_1", "kb_1", chunks)


class TestChunkServiceCreateMotherChunks:
    """Tests for _create_mother_chunks class method."""

    def test_create_mother_chunks_with_mom_field(self):
        """Test creating mother chunks from mom field."""
        chunks = [
            {"id": "chunk_1", "mom": "Summary text 1", "content_with_weight": "test1"},
        ]

        mothers = ChunkService._create_mother_chunks(chunks)

        assert len(mothers) == 1
        assert mothers[0]["content_with_weight"] == "Summary text 1"
        assert mothers[0]["available_int"] == 0

    def test_create_mother_chunks_with_mom_with_weight_field(self):
        """Test creating mother chunks from mom_with_weight field."""
        chunks = [
            {"id": "chunk_1", "mom_with_weight": "Summary text 2", "content_with_weight": "test1"},
        ]

        mothers = ChunkService._create_mother_chunks(chunks)

        assert len(mothers) == 1
        assert mothers[0]["content_with_weight"] == "Summary text 2"

    def test_create_mother_chunks_no_mom_field(self):
        """Test creating mother chunks when no mom field present."""
        chunks = [
            {"id": "chunk_1", "content_with_weight": "test1"},
        ]

        mothers = ChunkService._create_mother_chunks(chunks)

        assert len(mothers) == 0

    def test_create_mother_chunks_empty_mom(self):
        """Test creating mother chunks with empty mom field."""
        chunks = [
            {"id": "chunk_1", "mom": "", "content_with_weight": "test1"},
        ]

        mothers = ChunkService._create_mother_chunks(chunks)

        assert len(mothers) == 0

    def test_create_mother_chunks_deduplicates_ids(self):
        """Test that mother chunks deduplicate by ID."""
        chunks = [
            {"id": "chunk_1", "mom": "Same summary", "content_with_weight": "test1"},
            {"id": "chunk_2", "mom": "Same summary", "content_with_weight": "test2"},
        ]

        mothers = ChunkService._create_mother_chunks(chunks)

        assert len(mothers) == 1

    def test_create_mother_chunks_filters_fields(self):
        """Test that mother chunks only keep allowed fields."""
        chunks = [
            {"id": "chunk_1", "mom": "Summary", "extra_field": "should be removed", "content_with_weight": "test1"},
        ]

        mothers = ChunkService._create_mother_chunks(chunks)

        assert "extra_field" not in mothers[0]
        assert "id" in mothers[0]
        assert "content_with_weight" in mothers[0]

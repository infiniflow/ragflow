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


class TestChunkServiceInit:
    """Tests for ChunkService initialization."""

    def test_init_stores_task_context(self):
        """Test that task context is stored."""
        ctx = MagicMock()
        service = ChunkService(ctx=ctx)
        assert service._task_context is ctx


class TestChunkServiceBuildChunks:
    """Tests for build_chunks method."""

    def _create_mock_context(self, parser_id="naive", size=1000, parser_config=None, kb_parser_config=None):
        """Helper to create a mock TaskContext."""
        ctx = MagicMock()
        ctx.parser_id = parser_id
        ctx.name = "test.pdf"
        ctx.size = size
        ctx.from_page = 0
        ctx.to_page = -1
        ctx.parser_config = parser_config or {}
        ctx.kb_parser_config = kb_parser_config or {}
        ctx.language = "en"
        ctx.id = "task_1"
        ctx.tenant_id = "tenant_1"
        ctx.kb_id = "kb_1"
        ctx.doc_id = "doc_1"
        ctx.progress_cb = MagicMock()
        ctx.has_canceled_func = MagicMock(return_value=False)
        ctx.write_interceptor = None
        ctx.raw_task = {}
        ctx.llm_id = "llm_1"
        ctx.pagerank = 0
        ctx.location = "/path/to/test.pdf"
        ctx.chunk_limiter = MagicMock()
        ctx.chunk_limiter.__aenter__ = AsyncMock()
        ctx.chunk_limiter.__aexit__ = AsyncMock()
        ctx.chat_limiter = MagicMock()
        ctx.chat_limiter.__aenter__ = AsyncMock()
        ctx.chat_limiter.__aexit__ = AsyncMock()
        return ctx

    @pytest.mark.asyncio
    async def test_build_chunks_file_size_exceeded(self):
        """Test build_chunks returns empty list when file size exceeds limit."""
        ctx = self._create_mock_context(size=1000000000)  # Very large size

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
        ctx = self._create_mock_context(size=1000)

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
    async def test_build_chunks_with_auto_keywords(self):
        """Test build_chunks triggers keyword extraction when configured."""
        ctx = self._create_mock_context(parser_config={"auto_keywords": 5})

        service = ChunkService(ctx=ctx)

        with patch("rag.svr.task_executor_refactor.chunk_service.settings") as mock_settings:
            mock_settings.DOC_MAXIMUM_SIZE = 10000000

            mock_rec_ctx = MagicMock()
            ctx.recording_context = mock_rec_ctx

            with patch("rag.svr.task_executor_refactor.chunk_service.get_parser") as mock_get_parser:
                mock_get_parser.return_value = MagicMock()

                with patch("rag.svr.task_executor_refactor.chunk_service.run_chunking", new_callable=AsyncMock) as mock_run_chunking:
                    mock_run_chunking.return_value = []

                    with patch("rag.svr.task_executor_refactor.chunk_service.extract_outline", new_callable=AsyncMock):
                        with patch.object(service, '_prepare_docs_and_upload', new_callable=AsyncMock) as mock_prepare:
                            mock_prepare.return_value = [{"id": "chunk_1", "content_with_weight": "test"}]

                            with patch("rag.svr.task_executor_refactor.chunk_service.extract_keywords", new_callable=AsyncMock) as mock_extract:
                                await service.build_chunks(b"test binary")
                                mock_extract.assert_called_once()

    @pytest.mark.asyncio
    async def test_build_chunks_with_auto_questions(self):
        """Test build_chunks triggers question generation when configured."""
        ctx = self._create_mock_context(parser_config={"auto_questions": 3})

        service = ChunkService(ctx=ctx)

        with patch("rag.svr.task_executor_refactor.chunk_service.settings") as mock_settings:
            mock_settings.DOC_MAXIMUM_SIZE = 10000000

            mock_rec_ctx = MagicMock()
            ctx.recording_context = mock_rec_ctx

            with patch("rag.svr.task_executor_refactor.chunk_service.get_parser") as mock_get_parser:
                mock_get_parser.return_value = MagicMock()

                with patch("rag.svr.task_executor_refactor.chunk_service.run_chunking", new_callable=AsyncMock) as mock_run_chunking:
                    mock_run_chunking.return_value = []

                    with patch("rag.svr.task_executor_refactor.chunk_service.extract_outline", new_callable=AsyncMock):
                        with patch.object(service, '_prepare_docs_and_upload', new_callable=AsyncMock) as mock_prepare:
                            mock_prepare.return_value = [{"id": "chunk_1", "content_with_weight": "test"}]

                            with patch("rag.svr.task_executor_refactor.chunk_service.generate_questions", new_callable=AsyncMock) as mock_gen:
                                await service.build_chunks(b"test binary")
                                mock_gen.assert_called_once()

    @pytest.mark.asyncio
    async def test_build_chunks_with_tag_kb_ids(self):
        """Test build_chunks triggers tag application when tag_kb_ids configured."""
        ctx = self._create_mock_context(kb_parser_config={"tag_kb_ids": ["kb_1"]})

        service = ChunkService(ctx=ctx)

        with patch("rag.svr.task_executor_refactor.chunk_service.settings") as mock_settings:
            mock_settings.DOC_MAXIMUM_SIZE = 10000000

            mock_rec_ctx = MagicMock()
            ctx.recording_context = mock_rec_ctx

            with patch("rag.svr.task_executor_refactor.chunk_service.get_parser") as mock_get_parser:
                mock_get_parser.return_value = MagicMock()

                with patch("rag.svr.task_executor_refactor.chunk_service.run_chunking", new_callable=AsyncMock) as mock_run_chunking:
                    mock_run_chunking.return_value = []

                    with patch("rag.svr.task_executor_refactor.chunk_service.extract_outline", new_callable=AsyncMock):
                        with patch.object(service, '_prepare_docs_and_upload', new_callable=AsyncMock) as mock_prepare:
                            mock_prepare.return_value = [{"id": "chunk_1", "content_with_weight": "test"}]

                            with patch("rag.svr.task_executor_refactor.chunk_service.apply_tags", new_callable=AsyncMock) as mock_apply:
                                await service.build_chunks(b"test binary")
                                mock_apply.assert_called_once()

    @pytest.mark.asyncio
    async def test_build_chunks_with_metadata(self):
        """Test build_chunks triggers metadata generation when configured."""
        ctx = self._create_mock_context(
            parser_config={
                "enable_metadata": True,
                "metadata": [{"name": "category", "type": "string"}]
            }
        )

        service = ChunkService(ctx=ctx)

        with patch("rag.svr.task_executor_refactor.chunk_service.settings") as mock_settings:
            mock_settings.DOC_MAXIMUM_SIZE = 10000000

            mock_rec_ctx = MagicMock()
            ctx.recording_context = mock_rec_ctx

            with patch("rag.svr.task_executor_refactor.chunk_service.get_parser") as mock_get_parser:
                mock_get_parser.return_value = MagicMock()

                with patch("rag.svr.task_executor_refactor.chunk_service.run_chunking", new_callable=AsyncMock) as mock_run_chunking:
                    mock_run_chunking.return_value = []

                    with patch("rag.svr.task_executor_refactor.chunk_service.extract_outline", new_callable=AsyncMock):
                        with patch.object(service, '_prepare_docs_and_upload', new_callable=AsyncMock) as mock_prepare:
                            mock_prepare.return_value = [{"id": "chunk_1", "content_with_weight": "test"}]

                            with patch("rag.svr.task_executor_refactor.chunk_service.generate_metadata", new_callable=AsyncMock) as mock_meta:
                                await service.build_chunks(b"test binary")
                                mock_meta.assert_called_once()


class TestChunkServicePrepareDocsAndUpload:
    """Tests for _prepare_docs_and_upload method."""

    def _create_mock_context(self):
        """Helper to create a mock TaskContext."""
        ctx = MagicMock()
        ctx.doc_id = "doc_1"
        ctx.kb_id = "kb_1"
        ctx.tenant_id = "tenant_1"
        ctx.name = "test.pdf"
        ctx.location = "/path/to/test.pdf"
        ctx.pagerank = 0
        ctx.progress_cb = MagicMock()
        return ctx

    @pytest.mark.asyncio
    async def test_prepare_docs_and_upload_basic(self):
        """Test basic document preparation."""
        ctx = self._create_mock_context()
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
        ctx = self._create_mock_context()
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

    def _create_mock_context(self):
        """Helper to create a mock TaskContext."""
        ctx = MagicMock()
        ctx.id = "task_1"
        ctx.tenant_id = "tenant_1"
        ctx.kb_id = "kb_1"
        ctx.doc_id = "doc_1"
        ctx.parser_id = "naive"
        ctx.progress_cb = MagicMock()
        ctx.has_canceled_func = MagicMock(return_value=False)
        ctx.write_interceptor = None
        return ctx

    @pytest.mark.asyncio
    async def test_insert_chunks_success(self):
        """Test successful chunk insertion."""
        ctx = self._create_mock_context()
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
        ctx = self._create_mock_context()
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
        ctx = self._create_mock_context()
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

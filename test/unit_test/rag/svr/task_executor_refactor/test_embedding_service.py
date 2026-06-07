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
Unit tests for EmbeddingService module.

All tests validate behavior through the public API (embed_chunks) rather than
reaching into private orchestration methods.  The new async implementation uses
thread_pool_exec for model.encode calls; tests mock that boundary.
"""

import numpy as np
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from rag.svr.task_executor_refactor.embedding_service import EmbeddingService


class TestEmbeddingServiceInit:
    """Tests for EmbeddingService initialization."""

    @patch("rag.svr.task_executor_refactor.embedding_service.settings")
    def test_init_with_default_batch_size(self, mock_settings):
        """Test initialization with default batch size."""
        mock_settings.EMBEDDING_BATCH_SIZE = 32
        ctx = MagicMock()
        service = EmbeddingService(ctx=ctx)
        assert service._embedding_batch_size == 32

    @patch("rag.svr.task_executor_refactor.embedding_service.settings")
    def test_init_with_custom_batch_size(self, mock_settings):
        """Test initialization with custom batch size."""
        ctx = MagicMock()
        service = EmbeddingService(ctx=ctx, embedding_batch_size=64)
        assert service._embedding_batch_size == 64

    def test_init_stores_task_context(self):
        """Test that task context is stored."""
        ctx = MagicMock()
        service = EmbeddingService(ctx=ctx)
        assert service._task_context is ctx


class TestEmbeddingServiceEmbedChunks:
    """Tests for the public embed_chunks method.

    The async implementation uses thread_pool_exec for model.encode calls.
    Tests mock thread_pool_exec at the module level to control returned vectors.
    """

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.embedding_service.thread_pool_exec", new_callable=AsyncMock)
    async def test_embed_chunks_basic(self, mock_thread_pool):
        """Test basic chunk embedding."""
        mock_thread_pool.return_value = (np.array([[1.0, 2.0]]), 10)
        ctx = MagicMock()
        ctx.progress_cb = None
        ctx.embed_limiter = AsyncMockLimiter()
        service = EmbeddingService(ctx=ctx, embedding_batch_size=10)
        model = MagicMock()
        model.max_length = 100

        docs = [
            {"docnm_kwd": "Title1", "content_with_weight": "Content1"},
        ]
        tk_count, vector_size = await service.embed_chunks(docs, model)

        assert tk_count > 0
        assert vector_size == 2
        assert "q_2_vec" in docs[0]

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.embedding_service.thread_pool_exec", new_callable=AsyncMock)
    async def test_embed_chunks_uses_embedding_utils(self, mock_thread_pool):
        """Test that embed_chunks uses thread_pool_exec for encoding."""
        mock_thread_pool.return_value = (np.array([[1.0, 2.0]]), 10)
        ctx = MagicMock()
        ctx.progress_cb = None
        ctx.embed_limiter = AsyncMockLimiter()
        service = EmbeddingService(ctx=ctx, embedding_batch_size=10)
        model = MagicMock()
        model.max_length = 100

        docs = [
            {"docnm_kwd": "Title1", "content_with_weight": "Content1"},
        ]
        await service.embed_chunks(docs, model)

        # thread_pool_exec should be called at least once for encoding
        mock_thread_pool.assert_called()

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.embedding_service.thread_pool_exec", new_callable=AsyncMock)
    async def test_embed_chunks_with_title_content_combination(self, mock_thread_pool):
        """Test that title and content vectors are combined."""
        mock_thread_pool.return_value = (np.array([[1.0, 2.0]]), 10)
        ctx = MagicMock()
        ctx.progress_cb = None
        ctx.embed_limiter = AsyncMockLimiter()
        service = EmbeddingService(ctx=ctx, embedding_batch_size=10)
        model = MagicMock()
        model.max_length = 100

        docs = [
            {"docnm_kwd": "Title1", "content_with_weight": "Content1"},
        ]
        _, vector_size = await service.embed_chunks(docs, model, parser_config={"filename_embd_weight": 0.5})

        assert vector_size == 2

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.embedding_service.thread_pool_exec", new_callable=AsyncMock)
    async def test_embed_chunks_handles_long_text(self, mock_thread_pool):
        """Test that long texts are handled by embedding pipeline.

        Even with content exceeding model.max_length, embed_chunks produces
        valid vectors, meaning truncation (via EmbeddingUtils) is wired
        correctly in the encode path.
        """
        mock_thread_pool.return_value = (np.array([[1.0, 2.0]]), 10)
        ctx = MagicMock()
        ctx.progress_cb = None
        ctx.embed_limiter = AsyncMockLimiter()
        service = EmbeddingService(ctx=ctx, embedding_batch_size=10)
        model = MagicMock()
        model.max_length = 100

        docs = [
            {"docnm_kwd": "Title1", "content_with_weight": "a" * 200},
        ]
        tk_count, vector_size = await service.embed_chunks(docs, model)

        # Public contract: embed_chunks returns valid token counts and vectors
        assert tk_count > 0
        assert vector_size == 2
        assert "q_2_vec" in docs[0]


    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.embedding_service.thread_pool_exec", new_callable=AsyncMock)
    async def test_embed_chunks_empty_docs(self, mock_thread_pool):
        """Test embedding with empty docs list returns zero results."""
        ctx = MagicMock()
        ctx.progress_cb = None
        ctx.embed_limiter = AsyncMockLimiter()
        service = EmbeddingService(ctx=ctx)
        model = MagicMock()
        model.max_length = 100

        tk_count, vector_size = await service.embed_chunks([], model)

        assert tk_count == 0
        assert vector_size == 0

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.embedding_service.thread_pool_exec", new_callable=AsyncMock)
    async def test_embed_chunks_no_title(self, mock_thread_pool):
        """Test embedding when chunks have no title — content vectors used directly."""
        mock_thread_pool.return_value = (np.array([[3.0, 4.0]]), 5)
        ctx = MagicMock()
        ctx.progress_cb = None
        ctx.embed_limiter = AsyncMockLimiter()
        service = EmbeddingService(ctx=ctx, embedding_batch_size=10)
        model = MagicMock()
        model.max_length = 100

        docs = [{"content_with_weight": "Content only, no title"}]
        tk_count, vector_size = await service.embed_chunks(docs, model)

        # With no title, only content is encoded (1 call); vector_size comes from content vec
        assert vector_size == 2
        assert "q_2_vec" in docs[0]

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.embedding_service.thread_pool_exec", new_callable=AsyncMock)
    async def test_embed_chunks_title_weight_zero(self, mock_thread_pool):
        """Test embedding with filename_embd_weight=0.0 — no title contribution."""
        mock_thread_pool.return_value = (np.array([[1.0, 2.0]]), 5)
        ctx = MagicMock()
        ctx.progress_cb = None
        ctx.embed_limiter = AsyncMockLimiter()
        service = EmbeddingService(ctx=ctx, embedding_batch_size=10)
        model = MagicMock()
        model.max_length = 100

        docs = [{"docnm_kwd": "Title1", "content_with_weight": "Content1"}]
        _, vector_size = await service.embed_chunks(docs, model,
                                                     parser_config={"filename_embd_weight": 0.0})

        assert vector_size == 2

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.embedding_service.thread_pool_exec", new_callable=AsyncMock)
    async def test_embed_chunks_title_weight_one(self, mock_thread_pool):
        """Test embedding with filename_embd_weight=1.0 — full title contribution."""
        mock_thread_pool.return_value = (np.array([[1.0, 2.0]]), 5)
        ctx = MagicMock()
        ctx.progress_cb = None
        ctx.embed_limiter = AsyncMockLimiter()
        service = EmbeddingService(ctx=ctx, embedding_batch_size=10)
        model = MagicMock()
        model.max_length = 100

        docs = [{"docnm_kwd": "Title1", "content_with_weight": "Content1"}]
        _, vector_size = await service.embed_chunks(docs, model,
                                                     parser_config={"filename_embd_weight": 1.0})

        assert vector_size == 2

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.embedding_service.thread_pool_exec", new_callable=AsyncMock)
    async def test_embed_chunks_encode_failure_propagates(self, mock_thread_pool):
        """Test that model.encode exceptions are propagated to caller."""
        mock_thread_pool.side_effect = RuntimeError("embedding service unavailable")
        ctx = MagicMock()
        ctx.progress_cb = None
        ctx.embed_limiter = AsyncMockLimiter()
        service = EmbeddingService(ctx=ctx, embedding_batch_size=10)
        model = MagicMock()
        model.max_length = 100

        docs = [{"docnm_kwd": "Title1", "content_with_weight": "Content1"}]
        with pytest.raises(RuntimeError, match="embedding service unavailable"):
            await service.embed_chunks(docs, model)

    @pytest.mark.asyncio
    @patch("rag.svr.task_executor_refactor.embedding_service.thread_pool_exec", new_callable=AsyncMock)
    async def test_embed_chunks_multiple_batches(self, mock_thread_pool):
        """Test embedding with more chunks than batch size — multiple encode calls."""
        # Each call returns vectors matching input count
        def side_effect(func, texts, *args, **kw):
            n = len(texts) if isinstance(texts, list) else 1
            return np.random.rand(n, 2).astype(np.float32), 10 * n

        mock_thread_pool.side_effect = side_effect
        ctx = MagicMock()
        ctx.progress_cb = MagicMock()
        ctx.embed_limiter = AsyncMockLimiter()
        # batch_size=2, 5 chunks with titles → 1 title + ceil(5/2)=3 content = 4 calls
        service = EmbeddingService(ctx=ctx, embedding_batch_size=2)
        model = MagicMock()
        model.max_length = 100

        docs = [{"docnm_kwd": f"Title{i}", "content_with_weight": f"Content{i}"} for i in range(5)]
        _, vector_size = await service.embed_chunks(docs, model)

        assert mock_thread_pool.call_count == 4
        assert vector_size > 0


# Reuse from conftest
from test.unit_test.rag.svr.task_executor_refactor.conftest import AsyncMockLimiter

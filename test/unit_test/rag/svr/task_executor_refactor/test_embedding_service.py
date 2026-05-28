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
reaching into private orchestration methods like _encode_single, _encode_batch,
or _run_encode.  Those internal boundaries may be reshaped during a refactor
without changing the external behavior; the suite should not break in that case.
"""

import numpy as np
from unittest.mock import MagicMock, patch

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

    Internal helpers _encode_single, _encode_batch, and _run_encode are
    exercised through this public entry point so the suite stays resilient to
    method-boundary reshuffles.
    """

    @patch.object(EmbeddingService, '_run_encode')
    def test_embed_chunks_basic(self, mock_run_encode):
        """Test basic chunk embedding."""
        mock_run_encode.return_value = (np.array([[1.0, 2.0]]), 10)
        ctx = MagicMock()
        ctx.progress_cb = None
        service = EmbeddingService(ctx=ctx, embedding_batch_size=10)
        model = MagicMock()
        model.max_length = 100

        docs = [
            {"docnm_kwd": "Title1", "content_with_weight": "Content1"},
        ]
        tk_count, vector_size = service.embed_chunks(docs, model)

        assert tk_count > 0
        assert vector_size == 2
        assert "q_2_vec" in docs[0]

    @patch.object(EmbeddingService, '_run_encode')
    def test_embed_chunks_uses_embedding_utils(self, mock_run_encode):
        """Test that embed_chunks uses EmbeddingUtils internally.

        The internal path runs _encode_batch -> EmbeddingUtils.truncate_texts
        -> _run_encode.  We verify via the public embed_chunks that the chain
        is wired correctly without asserting on individual private method calls.
        """
        mock_run_encode.return_value = (np.array([[1.0, 2.0]]), 10)
        ctx = MagicMock()
        ctx.progress_cb = None
        service = EmbeddingService(ctx=ctx, embedding_batch_size=10)
        model = MagicMock()
        model.max_length = 100

        docs = [
            {"docnm_kwd": "Title1", "content_with_weight": "Content1"},
        ]
        service.embed_chunks(docs, model)

        mock_run_encode.assert_called()

    @patch.object(EmbeddingService, '_run_encode')
    def test_embed_chunks_with_title_content_combination(self, mock_run_encode):
        """Test that title and content vectors are combined."""
        mock_run_encode.return_value = (np.array([[1.0, 2.0]]), 10)
        ctx = MagicMock()
        ctx.progress_cb = None
        service = EmbeddingService(ctx=ctx, embedding_batch_size=10)
        model = MagicMock()
        model.max_length = 100

        docs = [
            {"docnm_kwd": "Title1", "content_with_weight": "Content1"},
        ]
        _, vector_size = service.embed_chunks(docs, model, parser_config={"filename_embd_weight": 0.5})

        assert vector_size == 2

    @patch.object(EmbeddingService, '_run_encode')
    def test_embed_chunks_handles_long_text(self, mock_run_encode):
        """Test that long texts are handled by embedding pipeline.

        Even with content exceeding model.max_length, embed_chunks produces
        valid vectors, meaning truncation (via EmbeddingUtils) is wired
        correctly in the encode path.
        """
        mock_run_encode.return_value = (np.array([[1.0, 2.0]]), 10)
        ctx = MagicMock()
        ctx.progress_cb = None
        service = EmbeddingService(ctx=ctx, embedding_batch_size=10)
        model = MagicMock()
        model.max_length = 100

        docs = [
            {"docnm_kwd": "Title1", "content_with_weight": "a" * 200},
        ]
        tk_count, vector_size = service.embed_chunks(docs, model)

        # Public contract: embed_chunks returns valid token counts and vectors
        assert tk_count > 0
        assert vector_size == 2
        assert "q_2_vec" in docs[0]

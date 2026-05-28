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
Embedding Service Module.

Provides [`EmbeddingService`](rag/svr/task_executor_refactor/embedding_service.py:42) for vector embedding operations.
"""

import asyncio
from typing import Any, Dict, List, Tuple

import numpy as np
from common import settings
from rag.svr.task_executor_refactor.embedding_utils import EmbeddingUtils
from rag.svr.task_executor_refactor.task_context import TaskContext


class EmbeddingService:
    """Service for vector embedding operations.

    This service handles:
    - Batch encoding of text chunks
    - Title + content vector combination
    - Embedding model rate limiting

    All intermediate results are recorded via RecordingContext for comparison.
    """

    def __init__(
        self,
        ctx: TaskContext,
        embedding_batch_size: int = None,
    ):
        """Initialize EmbeddingService.

        Args:
            ctx: TaskContext containing task configuration and execution resources.
            embedding_batch_size: Batch size for embedding operations.
        """
        self._task_context = ctx

        self._embedding_batch_size = embedding_batch_size or settings.EMBEDDING_BATCH_SIZE

    def embed_chunks(
        self,
        docs: List[Dict[str, Any]],
        embedding_model,
        parser_config: Dict = None,
    ) -> Tuple[int, int]:
        """Embed a list of chunks.

        Args:
            docs: List of chunk dictionaries to embed.
            embedding_model: The embedding model bundle (LLMBundle).
            parser_config: Parser configuration for filename embedding weight.

        Returns:
            Tuple of (token_count, vector_size).
        """
        if parser_config is None:
            parser_config = {}

        # Prepare text for embedding using EmbeddingUtils
        titles, contents = EmbeddingUtils.prepare_texts_for_embedding(docs)

        # Encode titles using EmbeddingUtils for truncation
        tk_count = 0
        if len(titles) > 0 and len(titles) == len(contents):
            vts, c = self._encode_single([titles[0]], embedding_model)
            tts = np.tile(vts[0], (len(contents), 1))
            tk_count += c
        else:
            tts = None

        # Batch encode contents using EmbeddingUtils
        vects_batches = []
        for i in range(0, len(contents), self._embedding_batch_size):
            batch = contents[i: i + self._embedding_batch_size]
            vts, c = self._encode_batch(batch, embedding_model)
            vects_batches.append(vts)
            tk_count += c
            if self._task_context.progress_cb:
                self._task_context.progress_cb(prog=0.7 + 0.2 * (i + 1) / len(contents), msg="")

        # Stack vectors using EmbeddingUtils
        cnts = EmbeddingUtils.stack_vectors(vects_batches)

        # Combine title and content vectors using EmbeddingUtils
        title_weight = parser_config.get("filename_embd_weight", EmbeddingUtils.DEFAULT_TITLE_WEIGHT)
        vects = EmbeddingUtils.combine_title_content_vectors(tts, cnts, title_weight)

        assert len(vects) == len(docs)

        # Attach vectors to docs using EmbeddingUtils
        vector_size = EmbeddingUtils.attach_vectors(docs, vects)

        return tk_count, vector_size

    def _encode_single(self, texts: List[str], model) -> Tuple[np.ndarray, int]:
        """Encode a single batch of texts."""
        return self._run_encode(texts, model)

    def _encode_batch(self, texts: List[str], model) -> Tuple[np.ndarray, int]:
        """Encode a batch of texts with rate limiting and truncation."""
        # Use EmbeddingUtils for truncation
        truncated = EmbeddingUtils.truncate_texts(texts, model.max_length)
        return self._run_encode(truncated, model)

    def _run_encode(self, texts: List[str], model) -> Tuple[np.ndarray, int]:
        """Run encoding with rate limiting."""
        async def _encode():
            async with self._task_context.embed_limiter:
                return model.encode(texts)
        return asyncio.get_event_loop().run_until_complete(_encode())

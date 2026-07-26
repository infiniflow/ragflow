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
Embedding Utils Module.

Provides utility functions for vector embedding operations to avoid code duplication
across different services (e.g., [`EmbeddingService`](rag/svr/task_executor_refactor/embedding_service.py),
[`DataflowService`](rag/svr/task_executor_refactor/dataflow_service.py)).

This module centralizes:
- Batch encoding of texts with truncation
- Vector stacking from multiple batches
- Vector attachment to chunk dictionaries
- Title and content vector combination with configurable weights
"""

import re
from typing import Any, Dict, List, Optional, Tuple

import numpy as np

from common.token_utils import truncate


class EmbeddingUtils:
    """Utility class for common embedding operations.

    This class provides static methods for:
    - Preparing texts for embedding (title/content extraction, HTML normalization)
    - Batch encoding with truncation
    - Stacking vector batches
    - Attaching vectors to chunk dictionaries
    - Combining title and content vectors with weights
    """

    DEFAULT_TITLE_WEIGHT = 0.1
    DEFAULT_TITLE_PLACEHOLDER = "Title"
    CONTENT_PLACEHOLDER_FOR_WHITESPACE = "None"

    @classmethod
    def prepare_texts_for_embedding(
        cls,
        docs: List[Dict[str, Any]],
        use_question_kwd: bool = True,
    ) -> Tuple[List[str], List[str]]:
        """Prepare title and content texts for embedding.

        Extracts titles from 'docnm_kwd' field and contents from 'question_kwd'
        (if available and use_question_kwd is True) or 'content_with_weight'.
        Table HTML tags are normalized to spaces.

        Args:
            docs: List of chunk dictionaries.
            use_question_kwd: Whether to use 'question_kwd' as content if available.

        Returns:
            Tuple of (titles, contents) lists.
        """
        titles = []
        contents = []
        for d in docs:
            title = d.get("docnm_kwd", cls.DEFAULT_TITLE_PLACEHOLDER)
            titles.append(title)

            content = cls._extract_content(d, use_question_kwd=use_question_kwd)
            content = cls._normalize_table_html(content)
            content = cls._handle_whitespace(content)

            contents.append(content)
        return titles, contents

    @classmethod
    def prepare_texts_for_dataflow_embedding(
        cls,
        chunks: List[Dict[str, Any]],
    ) -> List[str]:
        """Prepare texts for dataflow embedding.

        Extracts content from 'questions', 'summary', or 'text' fields
        (in priority order).

        Args:
            chunks: List of chunk dictionaries from dataflow output.

        Returns:
            List of text strings for embedding.
        """
        texts = []
        for chunk in chunks:
            text = chunk.get("questions", chunk.get("summary", chunk.get("text", "")))
            texts.append(text)
        return texts

    @classmethod
    def truncate_texts(cls, texts: List[str], max_length: int) -> List[str]:
        """Truncate texts to the specified maximum length.

        Args:
            texts: List of text strings to truncate.
            max_length: Maximum length for each text (will subtract 10 for safety margin).

        Returns:
            List of truncated text strings.
        """
        safe_max_length = max_length - 10
        return [truncate(text, safe_max_length) for text in texts]

    @classmethod
    def stack_vectors(cls, vects_batches: List[np.ndarray]) -> np.ndarray:
        """Stack a list of vector batches into a single array.

        Args:
            vects_batches: List of numpy arrays from batch encoding.

        Returns:
            Stacked numpy array, or empty array if no batches provided.
        """
        return np.vstack(vects_batches) if vects_batches else np.array([])

    @classmethod
    def attach_vectors(
        cls,
        docs: List[Dict[str, Any]],
        vectors: np.ndarray,
        vector_key_template: str = "q_%d_vec",
    ) -> int:
        """Attach vectors to chunk dictionaries.

        Args:
            docs: List of chunk dictionaries to modify in-place.
            vectors: Numpy array of vectors to attach.
            vector_key_template: Format string for the vector key (default: "q_%d_vec").

        Returns:
            The size of each vector (assumes uniform size).
        """
        vector_size = 0
        if len(vectors) != len(docs):
            raise ValueError(f"vectors/docs length mismatch: {len(vectors)} != {len(docs)}")
        for i, doc in enumerate(docs):
            vector = vectors[i].tolist()
            vector_size = len(vector)
            key = vector_key_template % vector_size
            doc[key] = vector
        return vector_size

    @classmethod
    def combine_title_content_vectors(
        cls,
        title_vecs: Optional[np.ndarray],
        content_vecs: np.ndarray,
        title_weight: Optional[float] = None,
    ) -> np.ndarray:
        """Combine title and content vectors with a configurable weight.

        Args:
            title_vecs: Title embedding vectors (may be None).
            content_vecs: Content embedding vectors.
            title_weight: Weight for title vectors (0.0 to 1.0). Defaults to 0.1.

        Returns:
            Combined vector array. If title_vecs is None or shapes don't match,
            returns content_vecs unchanged.
        """
        if title_weight is None:
            title_weight = cls.DEFAULT_TITLE_WEIGHT
        if not title_weight:
            title_weight = cls.DEFAULT_TITLE_WEIGHT

        if title_vecs is not None and content_vecs.ndim == 2 and title_vecs.shape == content_vecs.shape:
            return title_weight * title_vecs + (1 - title_weight) * content_vecs
        return content_vecs

    @classmethod
    def _extract_content(
        cls,
        doc: Dict[str, Any],
        use_question_kwd: bool = True,
    ) -> str:
        """Extract content from a chunk dictionary.

        Priority: question_kwd (joined by newline) -> content_with_weight.
        """
        if use_question_kwd:
            question_kwd = doc.get("question_kwd", [])
            if question_kwd:
                return "\n".join(question_kwd)
        return doc.get("content_with_weight", "")

    @classmethod
    def _normalize_table_html(cls, text: str) -> str:
        """Normalize table HTML tags to spaces.

        Replaces table-related HTML tags (table, td, caption, tr, th) with spaces.
        """
        return re.sub(r"</?(table|td|caption|tr|th)( [^<>]{0,12})?>", " ", text)

    @classmethod
    def _handle_whitespace(cls, text: str) -> str:
        """Replace whitespace-only content with a placeholder.

        Prevents embedding models from receiving empty or meaningless input.
        """
        if not text.strip():
            return cls.CONTENT_PLACEHOLDER_FOR_WHITESPACE
        return text

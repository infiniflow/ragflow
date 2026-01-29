#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
#
"""
Service for adjusting chunk recall weights based on user feedback.

When users upvote or downvote responses, this service updates the pagerank_fea
field of the referenced chunks to improve future retrieval quality.

This feature is disabled by default. Enable it by setting the environment
variable CHUNK_FEEDBACK_ENABLED=true.

Weight adjustments are intentionally small (0.1) to prevent individual votes
from having outsized impact on retrieval rankings.
"""
import logging
import os
from typing import List

from common.constants import PAGERANK_FLD
from common import settings
from rag.nlp.search import index_name


# Feature flag - disabled by default to prevent unintended side effects
CHUNK_FEEDBACK_ENABLED = os.getenv("CHUNK_FEEDBACK_ENABLED", "false").lower() == "true"

# Weight adjustment constants - intentionally small to require many votes for significant impact
UPVOTE_WEIGHT_INCREMENT = 0.1
DOWNVOTE_WEIGHT_DECREMENT = 0.1
MIN_PAGERANK_WEIGHT = 0
MAX_PAGERANK_WEIGHT = 100


class ChunkFeedbackService:
    """Service to update chunk weights based on user feedback."""

    @staticmethod
    def extract_chunk_ids_from_reference(reference: dict) -> List[str]:
        """
        Extract chunk IDs from a conversation message reference.

        Note: After chunks_format(), chunks use 'id' (not 'chunk_id')

        Args:
            reference: The reference dict containing chunks information

        Returns:
            List of chunk IDs that were used in the response
        """
        if not reference:
            return []

        chunks = reference.get("chunks", [])
        chunk_ids = []
        for chunk in chunks:
            # chunks_format() uses 'id', raw chunks use 'chunk_id'
            chunk_id = chunk.get("id") or chunk.get("chunk_id")
            if chunk_id:
                chunk_ids.append(chunk_id)

        return chunk_ids

    @staticmethod
    def get_chunk_kb_mapping(reference: dict) -> dict:
        """
        Extract chunk ID to knowledgebase ID mapping from reference.

        Note: After chunks_format(), chunks use 'id' and 'dataset_id'
              (not 'chunk_id' and 'kb_id')

        Args:
            reference: The reference dict containing chunks information

        Returns:
            Dict mapping chunk_id -> kb_id
        """
        if not reference:
            return {}

        chunks = reference.get("chunks", [])
        mapping = {}
        for chunk in chunks:
            # chunks_format() uses 'id', raw chunks use 'chunk_id'
            chunk_id = chunk.get("id") or chunk.get("chunk_id")
            # chunks_format() uses 'dataset_id', raw chunks use 'kb_id'
            kb_id = chunk.get("dataset_id") or chunk.get("kb_id")
            if chunk_id and kb_id:
                mapping[chunk_id] = kb_id

        return mapping

    @staticmethod
    def update_chunk_weight(
        tenant_id: str,
        chunk_id: str,
        kb_id: str,
        delta: float
    ) -> bool:
        """
        Update the pagerank weight of a single chunk.

        Args:
            tenant_id: The tenant ID for index naming
            chunk_id: The chunk ID to update
            kb_id: The knowledgebase ID
            delta: Weight change (+0.1 for upvote, -0.1 for downvote)

        Returns:
            True if update succeeded, False otherwise
        """
        try:
            idx_name = index_name(tenant_id)

            # Get current chunk to read existing pagerank
            chunk = settings.docStoreConn.get(chunk_id, idx_name, [kb_id])
            if not chunk:
                logging.warning(f"Chunk {chunk_id} not found in index {idx_name}")
                return False

            current_weight = chunk.get(PAGERANK_FLD, 0) or 0
            new_weight = current_weight + delta

            # Clamp to valid range
            new_weight = max(MIN_PAGERANK_WEIGHT, min(MAX_PAGERANK_WEIGHT, new_weight))

            # Update the chunk
            condition = {"id": chunk_id}
            new_value = {PAGERANK_FLD: new_weight}

            success = settings.docStoreConn.update(
                condition, new_value, idx_name, kb_id
            )

            if success:
                logging.info(
                    f"Updated chunk {chunk_id} pagerank: {current_weight} -> {new_weight}"
                )
            else:
                logging.warning(f"Failed to update chunk {chunk_id} pagerank")

            return success

        except Exception as e:
            logging.exception(f"Error updating chunk {chunk_id} weight: {e}")
            return False

    @classmethod
    def apply_feedback(
        cls,
        tenant_id: str,
        reference: dict,
        is_positive: bool
    ) -> dict:
        """
        Apply user feedback to all chunks referenced in a response.

        Args:
            tenant_id: The tenant ID
            reference: The reference dict from the conversation message
            is_positive: True for upvote (thumbup), False for downvote

        Returns:
            Dict with 'success_count', 'fail_count', and 'chunk_ids' processed
        """
        # Check if feature is enabled
        if not CHUNK_FEEDBACK_ENABLED:
            logging.debug("Chunk feedback feature is disabled")
            return {"success_count": 0, "fail_count": 0, "chunk_ids": [], "disabled": True}

        chunk_ids = cls.extract_chunk_ids_from_reference(reference)
        kb_mapping = cls.get_chunk_kb_mapping(reference)

        if not chunk_ids:
            logging.debug("No chunk IDs found in reference for feedback")
            return {"success_count": 0, "fail_count": 0, "chunk_ids": []}

        delta = UPVOTE_WEIGHT_INCREMENT if is_positive else -DOWNVOTE_WEIGHT_DECREMENT

        success_count = 0
        fail_count = 0

        for chunk_id in chunk_ids:
            kb_id = kb_mapping.get(chunk_id)
            if not kb_id:
                logging.warning(f"No kb_id found for chunk {chunk_id}, skipping")
                fail_count += 1
                continue

            if cls.update_chunk_weight(tenant_id, chunk_id, kb_id, delta):
                success_count += 1
            else:
                fail_count += 1

        logging.info(
            f"Applied {'positive' if is_positive else 'negative'} feedback to "
            f"{success_count}/{len(chunk_ids)} chunks"
        )

        return {
            "success_count": success_count,
            "fail_count": fail_count,
            "chunk_ids": chunk_ids
        }

    @classmethod
    def apply_feedback_to_chunks(
        cls,
        tenant_id: str,
        chunk_ids: List[str],
        kb_id: str,
        is_positive: bool
    ) -> dict:
        """
        Apply user feedback to specific chunk IDs (when kb_id is known).

        Args:
            tenant_id: The tenant ID
            chunk_ids: List of chunk IDs to update
            kb_id: The knowledgebase ID
            is_positive: True for upvote, False for downvote

        Returns:
            Dict with 'success_count' and 'fail_count'
        """
        if not chunk_ids:
            return {"success_count": 0, "fail_count": 0}

        delta = UPVOTE_WEIGHT_INCREMENT if is_positive else -DOWNVOTE_WEIGHT_DECREMENT

        success_count = 0
        fail_count = 0

        for chunk_id in chunk_ids:
            if cls.update_chunk_weight(tenant_id, chunk_id, kb_id, delta):
                success_count += 1
            else:
                fail_count += 1

        return {"success_count": success_count, "fail_count": fail_count}

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
Incremental Index Optimization Module

This module provides lightweight chunk diff/hash mechanisms for optimizing
document re-indexing. It identifies unchanged chunks, changed chunks, and
deleted chunks to avoid redundant embedding and storage operations.

Key Features:
- Reuse existing vectors for unchanged chunks
- Only re-embed changed/new chunks
- Delete removed chunks from the document store
- Minimal impact on existing pipeline
"""

import logging
from dataclasses import dataclass, field
from typing import Any, Optional

from common.misc_utils import thread_pool_exec
from rag.nlp import search as nlp_search
from common import settings


logger = logging.getLogger(__name__)


@dataclass
class IncrementalIndexResult:
    """Result of incremental index analysis.

    Attributes:
        chunks_to_embed: Chunks that need re-embedding (new or changed)
        chunks_to_reuse: Chunks that can reuse existing vectors (unchanged)
        chunk_ids_to_delete: Chunk IDs that no longer exist and should be deleted
        existing_chunks_map: Map of existing chunk ID to chunk data (for reference)
        stats: Statistics about the incremental operation
    """
    chunks_to_embed: list[dict] = field(default_factory=list)
    chunks_to_reuse: list[dict] = field(default_factory=list)
    chunk_ids_to_delete: set[str] = field(default_factory=set)
    existing_chunks_map: dict[str, dict] = field(default_factory=dict)
    stats: dict[str, Any] = field(default_factory=dict)

    @property
    def total_new_chunks(self) -> int:
        return len(self.chunks_to_embed)

    @property
    def total_reused_chunks(self) -> int:
        return len(self.chunks_to_reuse)

    @property
    def total_deleted_chunks(self) -> int:
        return len(self.chunk_ids_to_delete)


def _get_vector_field_name(chunk: dict) -> Optional[str]:
    """Extract the vector field name from a chunk.

    Vector fields follow the pattern: q_{dim}_vec (e.g., q_1024_vec)

    Args:
        chunk: The chunk dictionary

    Returns:
        The vector field name if found, None otherwise
    """
    for key in chunk.keys():
        if key.endswith("_vec") and key.startswith("q_"):
            return key
    return None


def _is_toc_chunk(chunk: dict) -> bool:
    """Check if a chunk is a TOC (Table of Contents) chunk.

    TOC chunks have special handling and should not be included in
    incremental optimization logic.

    Args:
        chunk: The chunk dictionary

    Returns:
        True if this is a TOC chunk, False otherwise
    """
    return chunk.get("toc_kwd") == "toc"


def _is_raptor_chunk(chunk: dict) -> bool:
    """Check if a chunk is a RAPTOR summary chunk.

    RAPTOR chunks are generated summaries and should be handled separately.

    Args:
        chunk: The chunk dictionary

    Returns:
        True if this is a RAPTOR chunk, False otherwise
    """
    return chunk.get("raptor_kwd") == "raptor"


def _is_mother_chunk(chunk: dict) -> bool:
    """Check if a chunk is a 'mother' chunk (context chunk).

    Mother chunks are used for parent context and have special handling.

    Args:
        chunk: The chunk dictionary

    Returns:
        True if this is a mother chunk, False otherwise
    """
    return chunk.get("available_int") == 0 and "mom_id" not in chunk and chunk.get("id")


def analyze_incremental_changes(
    doc_id: str,
    tenant_id: str,
    kb_id: str,
    new_chunks: list[dict],
    vector_size: int,
) -> IncrementalIndexResult:
    """Analyze incremental changes between new and existing chunks.

    This function:
    1. Queries existing chunks for the document from the doc store
    2. Compares new chunks with existing chunks using their IDs
    3. Identifies which chunks need re-embedding, which can be reused,
       and which should be deleted

    Args:
        doc_id: Document ID
        tenant_id: Tenant ID
        kb_id: Knowledge base/dataset ID
        new_chunks: List of newly generated chunks from build_chunks()
        vector_size: Expected vector dimension (for validation)

    Returns:
        IncrementalIndexResult containing the analysis results
    """
    result = IncrementalIndexResult()
    vector_field_name = f"q_{vector_size}_vec"

    new_chunk_ids: set[str] = set()
    toc_chunks: list[dict] = []

    for chunk in new_chunks:
        chunk_id = chunk.get("id")
        if not chunk_id:
            continue
        if _is_toc_chunk(chunk):
            toc_chunks.append(chunk)
            continue
        new_chunk_ids.add(chunk_id)

    if not new_chunk_ids:
        logger.warning(f"No valid chunks found for doc_id={doc_id}")
        result.chunks_to_embed = new_chunks
        result.stats = {
            "total_new": len(new_chunks),
            "total_reused": 0,
            "total_deleted": 0,
            "reason": "no_valid_chunk_ids",
        }
        return result

    try:
        existing_chunks = list(
            settings.retriever.chunk_list(
                doc_id=doc_id,
                tenant_id=tenant_id,
                kb_ids=[str(kb_id)],
                max_count=10000,
                fields=[
                    "id",
                    "content_with_weight",
                    vector_field_name,
                    "doc_id",
                    "kb_id",
                    "toc_kwd",
                    "raptor_kwd",
                    "available_int",
                    "img_id",
                ],
                sort_by_position=False,
            )
        )
    except Exception as e:
        logger.warning(
            f"Failed to query existing chunks for doc_id={doc_id}: {e}. "
            "Falling back to full re-indexing."
        )
        result.chunks_to_embed = new_chunks
        result.stats = {
            "total_new": len(new_chunks),
            "total_reused": 0,
            "total_deleted": 0,
            "reason": f"query_failed: {str(e)}",
        }
        return result

    for chunk in existing_chunks:
        chunk_id = chunk.get("id")
        if not chunk_id:
            continue
        if _is_toc_chunk(chunk) or _is_raptor_chunk(chunk):
            continue
        result.existing_chunks_map[chunk_id] = chunk

    existing_chunk_ids = set(result.existing_chunks_map.keys())

    unchanged_ids = new_chunk_ids & existing_chunk_ids
    changed_ids = new_chunk_ids - existing_chunk_ids
    deleted_ids = existing_chunk_ids - new_chunk_ids

    result.chunk_ids_to_delete = deleted_ids

    for chunk in new_chunks:
        chunk_id = chunk.get("id")
        if _is_toc_chunk(chunk):
            result.chunks_to_embed.append(chunk)
            continue

        if chunk_id in unchanged_ids:
            existing_chunk = result.existing_chunks_map.get(chunk_id)
            if existing_chunk:
                existing_vector = existing_chunk.get(vector_field_name)
                if existing_vector is not None:
                    chunk[vector_field_name] = existing_vector
                    result.chunks_to_reuse.append(chunk)
                    continue

            result.chunks_to_embed.append(chunk)
        else:
            result.chunks_to_embed.append(chunk)

    result.stats = {
        "doc_id": doc_id,
        "tenant_id": tenant_id,
        "kb_id": kb_id,
        "total_new_chunks": len(new_chunks),
        "total_existing_chunks": len(existing_chunk_ids),
        "total_unchanged": len(unchanged_ids),
        "total_changed": len(changed_ids),
        "total_deleted": len(deleted_ids),
        "vector_size": vector_size,
        "vector_field": vector_field_name,
    }

    logger.info(
        f"Incremental index analysis for doc={doc_id}: "
        f"total={len(new_chunks)}, "
        f"reused={result.total_reused_chunks}, "
        f"to_embed={result.total_new_chunks}, "
        f"to_delete={result.total_deleted_chunks}"
    )

    return result


async def delete_orphan_chunks_async(
    doc_id: str,
    tenant_id: str,
    kb_id: str,
    chunk_ids: set[str],
) -> int:
    """Async version of delete_orphan_chunks.

    Args:
        doc_id: Document ID
        tenant_id: Tenant ID
        kb_id: Knowledge base/dataset ID
        chunk_ids: Set of chunk IDs to delete

    Returns:
        Number of chunks deleted
    """
    if not chunk_ids:
        return 0

    idx_name = nlp_search.index_name(tenant_id)
    deleted_count = 0

    try:
        for chunk_id in chunk_ids:
            result = await thread_pool_exec(
                settings.docStoreConn.delete,
                {"id": chunk_id},
                idx_name,
                kb_id,
            )
            if result:
                deleted_count += 1
    except Exception as e:
        logger.error(
            f"Failed to delete orphan chunks for doc={doc_id}: {e}"
        )

    logger.info(
        f"Deleted {deleted_count}/{len(chunk_ids)} orphan chunks for doc={doc_id}"
    )
    return deleted_count


def merge_chunks_for_insert(
    chunks_to_embed: list[dict],
    chunks_to_reuse: list[dict],
) -> list[dict]:
    """Merge chunks for final insertion.

    Both embeddable and reusable chunks need to be written to the doc store,
    but reusable chunks already have their vectors populated.

    Note: The insert operation uses upsert semantics, so existing chunks
    will be updated with any new metadata (like position info).

    Args:
        chunks_to_embed: Chunks that were newly embedded
        chunks_to_reuse: Chunks that reused existing vectors

    Returns:
        Combined list of all chunks for insertion
    """
    return chunks_to_embed + chunks_to_reuse

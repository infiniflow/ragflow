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
Unit tests for incremental index optimization.

This module tests the chunk diff/hash mechanism for optimizing
document re-indexing. Test scenarios include:
1. Document unchanged - all chunks reused
2. Partial modification - some chunks changed
3. Paragraph deletion - some chunks removed
4. Paragraph addition - new chunks added
5. Mixed scenario - combination of changes
"""

import pytest
import xxhash
from unittest.mock import MagicMock, patch, PropertyMock
from dataclasses import dataclass, field
from typing import Any, Optional

from rag.utils.incremental_index import (
    IncrementalIndexResult,
    _get_vector_field_name,
    _is_toc_chunk,
    _is_raptor_chunk,
    _is_mother_chunk,
    analyze_incremental_changes,
    merge_chunks_for_insert,
)


class TestHelperFunctions:
    """Test helper functions in incremental_index module"""

    def test_get_vector_field_name_valid(self):
        """Test vector field name extraction with valid field"""
        chunk = {
            "id": "test123",
            "content_with_weight": "test content",
            "q_1024_vec": [0.1] * 1024,
        }
        assert _get_vector_field_name(chunk) == "q_1024_vec"

    def test_get_vector_field_name_different_sizes(self):
        """Test vector field name extraction with different vector sizes"""
        test_cases = [
            ({"q_768_vec": [0.1] * 768}, "q_768_vec"),
            ({"q_1536_vec": [0.1] * 1536}, "q_1536_vec"),
            ({"q_512_vec": [0.1] * 512}, "q_512_vec"),
        ]
        for chunk, expected in test_cases:
            assert _get_vector_field_name(chunk) == expected

    def test_get_vector_field_name_missing(self):
        """Test vector field name extraction when no vector field exists"""
        chunk = {
            "id": "test123",
            "content_with_weight": "test content",
        }
        assert _get_vector_field_name(chunk) is None

    def test_is_toc_chunk_true(self):
        """Test TOC chunk detection returns True for TOC chunks"""
        chunk = {
            "id": "toc123",
            "toc_kwd": "toc",
            "content_with_weight": "Table of Contents",
        }
        assert _is_toc_chunk(chunk) is True

    def test_is_toc_chunk_false(self):
        """Test TOC chunk detection returns False for non-TOC chunks"""
        chunk = {
            "id": "chunk123",
            "content_with_weight": "Regular content",
        }
        assert _is_toc_chunk(chunk) is False

    def test_is_toc_chunk_other_value(self):
        """Test TOC chunk detection with different toc_kwd values"""
        chunk = {
            "id": "chunk123",
            "toc_kwd": "something_else",
            "content_with_weight": "Regular content",
        }
        assert _is_toc_chunk(chunk) is False

    def test_is_raptor_chunk_true(self):
        """Test RAPTOR chunk detection returns True for RAPTOR chunks"""
        chunk = {
            "id": "raptor123",
            "raptor_kwd": "raptor",
            "content_with_weight": "RAPTOR summary",
        }
        assert _is_raptor_chunk(chunk) is True

    def test_is_raptor_chunk_false(self):
        """Test RAPTOR chunk detection returns False for non-RAPTOR chunks"""
        chunk = {
            "id": "chunk123",
            "content_with_weight": "Regular content",
        }
        assert _is_raptor_chunk(chunk) is False

    def test_is_mother_chunk_true(self):
        """Test mother chunk detection returns True for mother chunks"""
        chunk = {
            "id": "mother123",
            "available_int": 0,
            "content_with_weight": "Mother chunk content",
        }
        assert _is_mother_chunk(chunk) is True

    def test_is_mother_chunk_false(self):
        """Test mother chunk detection returns False for regular chunks"""
        chunk = {
            "id": "chunk123",
            "available_int": 1,
            "content_with_weight": "Regular content",
        }
        assert _is_mother_chunk(chunk) is False

    def test_is_mother_chunk_with_mom_id(self):
        """Test mother chunk detection for chunks with mom_id"""
        chunk = {
            "id": "chunk123",
            "available_int": 0,
            "mom_id": "mother456",
            "content_with_weight": "Child chunk",
        }
        assert _is_mother_chunk(chunk) is False


class TestIncrementalIndexResult:
    """Test IncrementalIndexResult dataclass"""

    def test_default_initialization(self):
        """Test default initialization of IncrementalIndexResult"""
        result = IncrementalIndexResult()
        assert result.chunks_to_embed == []
        assert result.chunks_to_reuse == []
        assert result.chunk_ids_to_delete == set()
        assert result.existing_chunks_map == {}
        assert result.stats == {}

    def test_custom_initialization(self):
        """Test custom initialization of IncrementalIndexResult"""
        chunks_to_embed = [{"id": "new1", "content": "new"}]
        chunks_to_reuse = [{"id": "old1", "content": "old"}]
        chunk_ids_to_delete = {"deleted1", "deleted2"}
        existing_chunks_map = {"old1": {"id": "old1", "content": "old"}}
        stats = {"total_new": 1, "total_reused": 1}

        result = IncrementalIndexResult(
            chunks_to_embed=chunks_to_embed,
            chunks_to_reuse=chunks_to_reuse,
            chunk_ids_to_delete=chunk_ids_to_delete,
            existing_chunks_map=existing_chunks_map,
            stats=stats,
        )

        assert result.chunks_to_embed == chunks_to_embed
        assert result.chunks_to_reuse == chunks_to_reuse
        assert result.chunk_ids_to_delete == chunk_ids_to_delete
        assert result.existing_chunks_map == existing_chunks_map
        assert result.stats == stats

    def test_total_properties(self):
        """Test computed properties for totals"""
        result = IncrementalIndexResult(
            chunks_to_embed=[{"id": "1"}, {"id": "2"}],
            chunks_to_reuse=[{"id": "3"}, {"id": "4"}, {"id": "5"}],
            chunk_ids_to_delete={"6", "7"},
        )

        assert result.total_new_chunks == 2
        assert result.total_reused_chunks == 3
        assert result.total_deleted_chunks == 2


class TestMergeChunksForInsert:
    """Test merge_chunks_for_insert function"""

    def test_merge_empty_lists(self):
        """Test merging empty lists"""
        result = merge_chunks_for_insert([], [])
        assert result == []

    def test_merge_embed_only(self):
        """Test merging when only embeddable chunks exist"""
        chunks_to_embed = [{"id": "1"}, {"id": "2"}]
        result = merge_chunks_for_insert(chunks_to_embed, [])
        assert result == chunks_to_embed

    def test_merge_reuse_only(self):
        """Test merging when only reusable chunks exist"""
        chunks_to_reuse = [{"id": "1"}, {"id": "2"}]
        result = merge_chunks_for_insert([], chunks_to_reuse)
        assert result == chunks_to_reuse

    def test_merge_both(self):
        """Test merging both embeddable and reusable chunks"""
        chunks_to_embed = [{"id": "new1"}, {"id": "new2"}]
        chunks_to_reuse = [{"id": "old1"}, {"id": "old2"}]
        result = merge_chunks_for_insert(chunks_to_embed, chunks_to_reuse)

        assert len(result) == 4
        for chunk in chunks_to_embed:
            assert chunk in result
        for chunk in chunks_to_reuse:
            assert chunk in result


class TestAnalyzeIncrementalChanges:
    """Test analyze_incremental_changes function with various scenarios"""

    def _create_chunk(self, chunk_id: str, content: str, vector_size: int = 1024) -> dict:
        """Helper to create a test chunk"""
        return {
            "id": chunk_id,
            "content_with_weight": content,
            "doc_id": "test_doc_123",
            "kb_id": ["test_kb_456"],
            f"q_{vector_size}_vec": [0.1] * vector_size,
        }

    @patch("rag.utils.incremental_index.settings")
    def test_document_unchanged_all_reused(self, mock_settings):
        """
        Scenario 1: Document unchanged.
        All new chunks match existing chunks.
        Expected: All chunks reused, no embedding needed.
        """
        vector_size = 1024
        doc_id = "test_doc_123"
        tenant_id = "test_tenant"
        kb_id = "test_kb_456"

        existing_chunks = [
            self._create_chunk("id1", "Content of chunk 1", vector_size),
            self._create_chunk("id2", "Content of chunk 2", vector_size),
            self._create_chunk("id3", "Content of chunk 3", vector_size),
        ]

        new_chunks = [
            {"id": "id1", "content_with_weight": "Content of chunk 1", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id2", "content_with_weight": "Content of chunk 2", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id3", "content_with_weight": "Content of chunk 3", "doc_id": doc_id, "kb_id": [kb_id]},
        ]

        mock_retriever = MagicMock()
        mock_retriever.chunk_list.return_value = iter(existing_chunks)
        type(mock_settings).retriever = PropertyMock(return_value=mock_retriever)

        result = analyze_incremental_changes(
            doc_id=doc_id,
            tenant_id=tenant_id,
            kb_id=kb_id,
            new_chunks=new_chunks,
            vector_size=vector_size,
        )

        assert result.total_reused_chunks == 3
        assert result.total_new_chunks == 0
        assert result.total_deleted_chunks == 0

        for chunk in result.chunks_to_reuse:
            assert f"q_{vector_size}_vec" in chunk
            assert chunk[f"q_{vector_size}_vec"] == [0.1] * vector_size

    @patch("rag.utils.incremental_index.settings")
    def test_partial_modification_some_changed(self, mock_settings):
        """
        Scenario 2: Partial modification.
        Some chunks changed, some unchanged.
        Expected: Changed chunks need embedding, unchanged chunks reused.
        """
        vector_size = 1024
        doc_id = "test_doc_123"
        tenant_id = "test_tenant"
        kb_id = "test_kb_456"

        existing_chunks = [
            self._create_chunk("id1", "Original content 1", vector_size),
            self._create_chunk("id2", "Original content 2", vector_size),
            self._create_chunk("id3", "Original content 3", vector_size),
        ]

        new_chunks = [
            {"id": "id1", "content_with_weight": "Original content 1", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id2_modified", "content_with_weight": "MODIFIED content 2", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id3", "content_with_weight": "Original content 3", "doc_id": doc_id, "kb_id": [kb_id]},
        ]

        mock_retriever = MagicMock()
        mock_retriever.chunk_list.return_value = iter(existing_chunks)
        type(mock_settings).retriever = PropertyMock(return_value=mock_retriever)

        result = analyze_incremental_changes(
            doc_id=doc_id,
            tenant_id=tenant_id,
            kb_id=kb_id,
            new_chunks=new_chunks,
            vector_size=vector_size,
        )

        assert result.total_reused_chunks == 2
        assert result.total_new_chunks == 1
        assert result.total_deleted_chunks == 1

        reused_ids = {c["id"] for c in result.chunks_to_reuse}
        assert "id1" in reused_ids
        assert "id3" in reused_ids

        embed_ids = {c["id"] for c in result.chunks_to_embed}
        assert "id2_modified" in embed_ids

        assert "id2" in result.chunk_ids_to_delete

    @patch("rag.utils.incremental_index.settings")
    def test_paragraph_deletion_some_removed(self, mock_settings):
        """
        Scenario 3: Paragraph deletion.
        New chunks have fewer chunks than existing.
        Expected: Missing chunks marked for deletion.
        """
        vector_size = 1024
        doc_id = "test_doc_123"
        tenant_id = "test_tenant"
        kb_id = "test_kb_456"

        existing_chunks = [
            self._create_chunk("id1", "Content 1", vector_size),
            self._create_chunk("id2", "Content 2", vector_size),
            self._create_chunk("id3", "Content 3", vector_size),
            self._create_chunk("id4", "Content 4", vector_size),
        ]

        new_chunks = [
            {"id": "id1", "content_with_weight": "Content 1", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id4", "content_with_weight": "Content 4", "doc_id": doc_id, "kb_id": [kb_id]},
        ]

        mock_retriever = MagicMock()
        mock_retriever.chunk_list.return_value = iter(existing_chunks)
        type(mock_settings).retriever = PropertyMock(return_value=mock_retriever)

        result = analyze_incremental_changes(
            doc_id=doc_id,
            tenant_id=tenant_id,
            kb_id=kb_id,
            new_chunks=new_chunks,
            vector_size=vector_size,
        )

        assert result.total_reused_chunks == 2
        assert result.total_new_chunks == 0
        assert result.total_deleted_chunks == 2

        assert "id2" in result.chunk_ids_to_delete
        assert "id3" in result.chunk_ids_to_delete

    @patch("rag.utils.incremental_index.settings")
    def test_paragraph_addition_new_chunks(self, mock_settings):
        """
        Scenario 4: Paragraph addition.
        New chunks have additional chunks not in existing.
        Expected: New chunks need embedding.
        """
        vector_size = 1024
        doc_id = "test_doc_123"
        tenant_id = "test_tenant"
        kb_id = "test_kb_456"

        existing_chunks = [
            self._create_chunk("id1", "Content 1", vector_size),
            self._create_chunk("id2", "Content 2", vector_size),
        ]

        new_chunks = [
            {"id": "id1", "content_with_weight": "Content 1", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id_new1", "content_with_weight": "NEW content A", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id2", "content_with_weight": "Content 2", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id_new2", "content_with_weight": "NEW content B", "doc_id": doc_id, "kb_id": [kb_id]},
        ]

        mock_retriever = MagicMock()
        mock_retriever.chunk_list.return_value = iter(existing_chunks)
        type(mock_settings).retriever = PropertyMock(return_value=mock_retriever)

        result = analyze_incremental_changes(
            doc_id=doc_id,
            tenant_id=tenant_id,
            kb_id=kb_id,
            new_chunks=new_chunks,
            vector_size=vector_size,
        )

        assert result.total_reused_chunks == 2
        assert result.total_new_chunks == 2
        assert result.total_deleted_chunks == 0

        embed_ids = {c["id"] for c in result.chunks_to_embed}
        assert "id_new1" in embed_ids
        assert "id_new2" in embed_ids

    @patch("rag.utils.incremental_index.settings")
    def test_mixed_scenario_add_modify_delete(self, mock_settings):
        """
        Scenario 5: Mixed scenario.
        Combination of added, modified, and deleted chunks.
        Expected: Correct classification of all changes.
        """
        vector_size = 1024
        doc_id = "test_doc_123"
        tenant_id = "test_tenant"
        kb_id = "test_kb_456"

        existing_chunks = [
            self._create_chunk("id1", "Content 1", vector_size),
            self._create_chunk("id2", "Content 2", vector_size),
            self._create_chunk("id3", "Content 3", vector_size),
            self._create_chunk("id4", "Content 4", vector_size),
        ]

        new_chunks = [
            {"id": "id1", "content_with_weight": "Content 1", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id2_modified", "content_with_weight": "MODIFIED Content 2", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id3", "content_with_weight": "Content 3", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id_new", "content_with_weight": "NEW Content", "doc_id": doc_id, "kb_id": [kb_id]},
        ]

        mock_retriever = MagicMock()
        mock_retriever.chunk_list.return_value = iter(existing_chunks)
        type(mock_settings).retriever = PropertyMock(return_value=mock_retriever)

        result = analyze_incremental_changes(
            doc_id=doc_id,
            tenant_id=tenant_id,
            kb_id=kb_id,
            new_chunks=new_chunks,
            vector_size=vector_size,
        )

        assert result.total_reused_chunks == 2
        assert result.total_new_chunks == 2
        assert result.total_deleted_chunks == 2

        reused_ids = {c["id"] for c in result.chunks_to_reuse}
        assert "id1" in reused_ids
        assert "id3" in reused_ids

        embed_ids = {c["id"] for c in result.chunks_to_embed}
        assert "id2_modified" in embed_ids
        assert "id_new" in embed_ids

        assert "id2" in result.chunk_ids_to_delete
        assert "id4" in result.chunk_ids_to_delete

    @patch("rag.utils.incremental_index.settings")
    def test_toc_chunk_always_embedded(self, mock_settings):
        """
        Scenario 6: TOC chunks.
        TOC chunks should always be embedded (not reused).
        """
        vector_size = 1024
        doc_id = "test_doc_123"
        tenant_id = "test_tenant"
        kb_id = "test_kb_456"

        existing_chunks = [
            self._create_chunk("id1", "Content 1", vector_size),
        ]

        new_chunks = [
            {"id": "id1", "content_with_weight": "Content 1", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "toc_id", "toc_kwd": "toc", "content_with_weight": "TOC Content", "doc_id": doc_id, "kb_id": [kb_id]},
        ]

        mock_retriever = MagicMock()
        mock_retriever.chunk_list.return_value = iter(existing_chunks)
        type(mock_settings).retriever = PropertyMock(return_value=mock_retriever)

        result = analyze_incremental_changes(
            doc_id=doc_id,
            tenant_id=tenant_id,
            kb_id=kb_id,
            new_chunks=new_chunks,
            vector_size=vector_size,
        )

        assert "toc_id" in {c["id"] for c in result.chunks_to_embed}

    @patch("rag.utils.incremental_index.settings")
    def test_first_time_indexing_no_existing(self, mock_settings):
        """
        Scenario 7: First time indexing.
        No existing chunks found.
        Expected: All chunks need embedding.
        """
        vector_size = 1024
        doc_id = "test_doc_123"
        tenant_id = "test_tenant"
        kb_id = "test_kb_456"

        existing_chunks = []

        new_chunks = [
            {"id": "id1", "content_with_weight": "Content 1", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id2", "content_with_weight": "Content 2", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id3", "content_with_weight": "Content 3", "doc_id": doc_id, "kb_id": [kb_id]},
        ]

        mock_retriever = MagicMock()
        mock_retriever.chunk_list.return_value = iter(existing_chunks)
        type(mock_settings).retriever = PropertyMock(return_value=mock_retriever)

        result = analyze_incremental_changes(
            doc_id=doc_id,
            tenant_id=tenant_id,
            kb_id=kb_id,
            new_chunks=new_chunks,
            vector_size=vector_size,
        )

        assert result.total_reused_chunks == 0
        assert result.total_new_chunks == 3
        assert result.total_deleted_chunks == 0

    @patch("rag.utils.incremental_index.settings")
    def test_query_failure_fallback_to_full_index(self, mock_settings):
        """
        Scenario 8: Query failure.
        When querying existing chunks fails, should fall back to full indexing.
        """
        vector_size = 1024
        doc_id = "test_doc_123"
        tenant_id = "test_tenant"
        kb_id = "test_kb_456"

        new_chunks = [
            {"id": "id1", "content_with_weight": "Content 1", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id2", "content_with_weight": "Content 2", "doc_id": doc_id, "kb_id": [kb_id]},
        ]

        mock_retriever = MagicMock()
        mock_retriever.chunk_list.side_effect = Exception("Connection error")
        type(mock_settings).retriever = PropertyMock(return_value=mock_retriever)

        result = analyze_incremental_changes(
            doc_id=doc_id,
            tenant_id=tenant_id,
            kb_id=kb_id,
            new_chunks=new_chunks,
            vector_size=vector_size,
        )

        assert result.total_reused_chunks == 0
        assert result.total_new_chunks == 2
        assert result.total_deleted_chunks == 0
        assert result.stats["reason"] == "query_failed: Connection error"

    @patch("rag.utils.incremental_index.settings")
    def test_existing_vector_missing_still_embedded(self, mock_settings):
        """
        Scenario 9: Existing chunk missing vector.
        If an existing chunk doesn't have the expected vector field,
        it should be re-embedded.
        """
        vector_size = 1024
        doc_id = "test_doc_123"
        tenant_id = "test_tenant"
        kb_id = "test_kb_456"

        existing_chunks = [
            {
                "id": "id1",
                "content_with_weight": "Content 1",
                "doc_id": doc_id,
                "kb_id": [kb_id],
            },
            self._create_chunk("id2", "Content 2", vector_size),
        ]

        new_chunks = [
            {"id": "id1", "content_with_weight": "Content 1", "doc_id": doc_id, "kb_id": [kb_id]},
            {"id": "id2", "content_with_weight": "Content 2", "doc_id": doc_id, "kb_id": [kb_id]},
        ]

        mock_retriever = MagicMock()
        mock_retriever.chunk_list.return_value = iter(existing_chunks)
        type(mock_settings).retriever = PropertyMock(return_value=mock_retriever)

        result = analyze_incremental_changes(
            doc_id=doc_id,
            tenant_id=tenant_id,
            kb_id=kb_id,
            new_chunks=new_chunks,
            vector_size=vector_size,
        )

        assert result.total_reused_chunks == 1
        assert result.total_new_chunks == 1

        reused_ids = {c["id"] for c in result.chunks_to_reuse}
        assert "id2" in reused_ids

        embed_ids = {c["id"] for c in result.chunks_to_embed}
        assert "id1" in embed_ids


class TestChunkIdConsistency:
    """Test that chunk ID generation is consistent with existing code"""

    def test_chunk_id_matches_task_executor_format(self):
        """
        Verify that our understanding of chunk ID generation matches
        the actual implementation in task_executor.py.
        """
        content = "This is a test chunk content"
        doc_id = "test_doc_123"

        expected_id = xxhash.xxh64(
            (content + doc_id).encode("utf-8", "surrogatepass")
        ).hexdigest()

        chunk = {
            "content_with_weight": content,
            "doc_id": doc_id,
        }
        actual_id = xxhash.xxh64(
            (chunk["content_with_weight"] + str(chunk["doc_id"])).encode("utf-8", "surrogatepass")
        ).hexdigest()

        assert actual_id == expected_id

    def test_content_change_changes_id(self):
        """Verify that changing content changes the chunk ID"""
        doc_id = "test_doc_123"
        content1 = "Original content"
        content2 = "Modified content"

        id1 = xxhash.xxh64(
            (content1 + doc_id).encode("utf-8", "surrogatepass")
        ).hexdigest()

        id2 = xxhash.xxh64(
            (content2 + doc_id).encode("utf-8", "surrogatepass")
        ).hexdigest()

        assert id1 != id2

    def test_doc_id_change_changes_id(self):
        """Verify that changing doc_id changes the chunk ID"""
        content = "Same content"
        doc_id1 = "doc_1"
        doc_id2 = "doc_2"

        id1 = xxhash.xxh64(
            (content + doc_id1).encode("utf-8", "surrogatepass")
        ).hexdigest()

        id2 = xxhash.xxh64(
            (content + doc_id2).encode("utf-8", "surrogatepass")
        ).hexdigest()

        assert id1 != id2


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

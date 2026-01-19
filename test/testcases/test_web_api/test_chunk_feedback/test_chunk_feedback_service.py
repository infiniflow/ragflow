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
Tests for ChunkFeedbackService - adjusting chunk weights based on user feedback.
"""
from unittest.mock import patch, MagicMock

from api.db.services.chunk_feedback_service import (
    ChunkFeedbackService,
    UPVOTE_WEIGHT_INCREMENT,
    DOWNVOTE_WEIGHT_DECREMENT,
    MIN_PAGERANK_WEIGHT,
    MAX_PAGERANK_WEIGHT,
)


class TestExtractChunkIds:
    """Tests for extract_chunk_ids_from_reference method."""

    def test_empty_reference(self):
        """Should return empty list for empty reference."""
        assert ChunkFeedbackService.extract_chunk_ids_from_reference({}) == []
        assert ChunkFeedbackService.extract_chunk_ids_from_reference(None) == []

    def test_reference_with_id_field(self):
        """Should extract 'id' from formatted chunks (chunks_format output)."""
        reference = {
            "chunks": [
                {"id": "chunk1", "content": "test"},
                {"id": "chunk2", "content": "test2"},
            ]
        }
        result = ChunkFeedbackService.extract_chunk_ids_from_reference(reference)
        assert result == ["chunk1", "chunk2"]

    def test_reference_with_chunk_id_field(self):
        """Should fall back to 'chunk_id' field for raw chunks."""
        reference = {
            "chunks": [
                {"chunk_id": "chunk1", "content": "test"},
                {"chunk_id": "chunk2", "content": "test2"},
            ]
        }
        result = ChunkFeedbackService.extract_chunk_ids_from_reference(reference)
        assert result == ["chunk1", "chunk2"]

    def test_reference_with_no_chunks(self):
        """Should return empty list if no chunks key."""
        reference = {"doc_aggs": [{"doc_id": "doc1"}]}
        result = ChunkFeedbackService.extract_chunk_ids_from_reference(reference)
        assert result == []


class TestGetChunkKbMapping:
    """Tests for get_chunk_kb_mapping method."""

    def test_empty_reference(self):
        """Should return empty dict for empty reference."""
        assert ChunkFeedbackService.get_chunk_kb_mapping({}) == {}
        assert ChunkFeedbackService.get_chunk_kb_mapping(None) == {}

    def test_reference_with_dataset_id(self):
        """Should map id to dataset_id (chunks_format output)."""
        reference = {
            "chunks": [
                {"id": "chunk1", "dataset_id": "kb1"},
                {"id": "chunk2", "dataset_id": "kb2"},
            ]
        }
        result = ChunkFeedbackService.get_chunk_kb_mapping(reference)
        assert result == {"chunk1": "kb1", "chunk2": "kb2"}

    def test_reference_with_kb_id(self):
        """Should fall back to kb_id for raw chunks."""
        reference = {
            "chunks": [
                {"chunk_id": "chunk1", "kb_id": "kb1"},
                {"chunk_id": "chunk2", "kb_id": "kb2"},
            ]
        }
        result = ChunkFeedbackService.get_chunk_kb_mapping(reference)
        assert result == {"chunk1": "kb1", "chunk2": "kb2"}

    def test_reference_missing_kb_id(self):
        """Should skip chunks without kb_id/dataset_id."""
        reference = {
            "chunks": [
                {"id": "chunk1", "dataset_id": "kb1"},
                {"id": "chunk2"},  # No dataset_id
            ]
        }
        result = ChunkFeedbackService.get_chunk_kb_mapping(reference)
        assert result == {"chunk1": "kb1"}


class TestUpdateChunkWeight:
    """Tests for update_chunk_weight method."""

    @patch("api.db.services.chunk_feedback_service.settings")
    def test_update_weight_success(self, mock_settings):
        """Should update chunk weight successfully."""
        mock_doc_store = MagicMock()
        mock_doc_store.get.return_value = {"pagerank_fea": 10}
        mock_doc_store.update.return_value = True
        mock_settings.docStoreConn = mock_doc_store

        result = ChunkFeedbackService.update_chunk_weight(
            tenant_id="tenant1",
            chunk_id="chunk1",
            kb_id="kb1",
            delta=1
        )

        assert result is True
        mock_doc_store.update.assert_called_once()

    @patch("api.db.services.chunk_feedback_service.settings")
    def test_update_weight_chunk_not_found(self, mock_settings):
        """Should return False if chunk not found."""
        mock_doc_store = MagicMock()
        mock_doc_store.get.return_value = None
        mock_settings.docStoreConn = mock_doc_store

        result = ChunkFeedbackService.update_chunk_weight(
            tenant_id="tenant1",
            chunk_id="chunk1",
            kb_id="kb1",
            delta=1
        )

        assert result is False

    @patch("api.db.services.chunk_feedback_service.settings")
    def test_update_weight_clamp_max(self, mock_settings):
        """Should clamp weight to MAX_PAGERANK_WEIGHT."""
        mock_doc_store = MagicMock()
        mock_doc_store.get.return_value = {"pagerank_fea": MAX_PAGERANK_WEIGHT}
        mock_doc_store.update.return_value = True
        mock_settings.docStoreConn = mock_doc_store

        ChunkFeedbackService.update_chunk_weight(
            tenant_id="tenant1",
            chunk_id="chunk1",
            kb_id="kb1",
            delta=10  # Would exceed max
        )

        # Verify the new_value passed to update has clamped weight
        call_args = mock_doc_store.update.call_args
        new_value = call_args[0][1]
        assert new_value["pagerank_fea"] == MAX_PAGERANK_WEIGHT

    @patch("api.db.services.chunk_feedback_service.settings")
    def test_update_weight_clamp_min(self, mock_settings):
        """Should clamp weight to MIN_PAGERANK_WEIGHT."""
        mock_doc_store = MagicMock()
        mock_doc_store.get.return_value = {"pagerank_fea": 0}
        mock_doc_store.update.return_value = True
        mock_settings.docStoreConn = mock_doc_store

        ChunkFeedbackService.update_chunk_weight(
            tenant_id="tenant1",
            chunk_id="chunk1",
            kb_id="kb1",
            delta=-10  # Would go below min
        )

        call_args = mock_doc_store.update.call_args
        new_value = call_args[0][1]
        assert new_value["pagerank_fea"] == MIN_PAGERANK_WEIGHT


class TestApplyFeedback:
    """Tests for apply_feedback method."""

    @patch("api.db.services.chunk_feedback_service.CHUNK_FEEDBACK_ENABLED", False)
    def test_apply_feedback_disabled(self):
        """Should return early when feature is disabled."""
        result = ChunkFeedbackService.apply_feedback(
            tenant_id="tenant1",
            reference={"chunks": [{"id": "chunk1", "dataset_id": "kb1"}]},
            is_positive=True
        )

        assert result["success_count"] == 0
        assert result["fail_count"] == 0
        assert result.get("disabled") is True

    @patch("api.db.services.chunk_feedback_service.CHUNK_FEEDBACK_ENABLED", True)
    @patch.object(ChunkFeedbackService, "update_chunk_weight")
    @patch.object(ChunkFeedbackService, "get_chunk_kb_mapping")
    @patch.object(ChunkFeedbackService, "extract_chunk_ids_from_reference")
    def test_apply_positive_feedback(self, mock_extract, mock_mapping, mock_update):
        """Should apply positive feedback to all chunks."""
        mock_extract.return_value = ["chunk1", "chunk2"]
        mock_mapping.return_value = {"chunk1": "kb1", "chunk2": "kb1"}
        mock_update.return_value = True

        result = ChunkFeedbackService.apply_feedback(
            tenant_id="tenant1",
            reference={"chunks": []},
            is_positive=True
        )

        assert result["success_count"] == 2
        assert result["fail_count"] == 0
        assert mock_update.call_count == 2
        # Verify positive delta
        mock_update.assert_any_call("tenant1", "chunk1", "kb1", UPVOTE_WEIGHT_INCREMENT)

    @patch("api.db.services.chunk_feedback_service.CHUNK_FEEDBACK_ENABLED", True)
    @patch.object(ChunkFeedbackService, "update_chunk_weight")
    @patch.object(ChunkFeedbackService, "get_chunk_kb_mapping")
    @patch.object(ChunkFeedbackService, "extract_chunk_ids_from_reference")
    def test_apply_negative_feedback(self, mock_extract, mock_mapping, mock_update):
        """Should apply negative feedback to all chunks."""
        mock_extract.return_value = ["chunk1"]
        mock_mapping.return_value = {"chunk1": "kb1"}
        mock_update.return_value = True

        result = ChunkFeedbackService.apply_feedback(
            tenant_id="tenant1",
            reference={"chunks": []},
            is_positive=False
        )

        assert result["success_count"] == 1
        # Verify negative delta
        mock_update.assert_called_with("tenant1", "chunk1", "kb1", -DOWNVOTE_WEIGHT_DECREMENT)

    @patch("api.db.services.chunk_feedback_service.CHUNK_FEEDBACK_ENABLED", True)
    @patch.object(ChunkFeedbackService, "extract_chunk_ids_from_reference")
    def test_apply_feedback_no_chunks(self, mock_extract):
        """Should handle empty chunk list gracefully."""
        mock_extract.return_value = []

        result = ChunkFeedbackService.apply_feedback(
            tenant_id="tenant1",
            reference={},
            is_positive=True
        )

        assert result["success_count"] == 0
        assert result["fail_count"] == 0
        assert result["chunk_ids"] == []

    @patch("api.db.services.chunk_feedback_service.CHUNK_FEEDBACK_ENABLED", True)
    @patch.object(ChunkFeedbackService, "update_chunk_weight")
    @patch.object(ChunkFeedbackService, "get_chunk_kb_mapping")
    @patch.object(ChunkFeedbackService, "extract_chunk_ids_from_reference")
    def test_apply_feedback_partial_failure(self, mock_extract, mock_mapping, mock_update):
        """Should count failures correctly."""
        mock_extract.return_value = ["chunk1", "chunk2"]
        mock_mapping.return_value = {"chunk1": "kb1", "chunk2": "kb1"}
        mock_update.side_effect = [True, False]  # First succeeds, second fails

        result = ChunkFeedbackService.apply_feedback(
            tenant_id="tenant1",
            reference={"chunks": []},
            is_positive=True
        )

        assert result["success_count"] == 1
        assert result["fail_count"] == 1

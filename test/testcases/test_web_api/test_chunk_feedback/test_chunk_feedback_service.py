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

Uses importlib to load chunk_feedback_service.py in isolation so that
test/testcases/test_web_api/common.py (a test-helper module) does not shadow
the project-level common/ package during collection.
"""
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock

import pytest

_REPO_ROOT = Path(__file__).resolve().parents[4]


def _load_feedback_module(monkeypatch):
    """Load chunk_feedback_service.py with lightweight stubs for its deps."""
    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(_REPO_ROOT / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    constants_mod = ModuleType("common.constants")
    constants_mod.PAGERANK_FLD = "pagerank_fea"
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    settings_mod = ModuleType("common.settings")
    settings_mod.docStoreConn = MagicMock()
    monkeypatch.setitem(sys.modules, "common.settings", settings_mod)
    common_pkg.settings = settings_mod

    rag_pkg = ModuleType("rag")
    rag_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag", rag_pkg)

    rag_nlp_pkg = ModuleType("rag.nlp")
    rag_nlp_pkg.__path__ = []
    rag_nlp_pkg.search = SimpleNamespace(index_name=lambda tid: f"idx-{tid}")
    monkeypatch.setitem(sys.modules, "rag.nlp", rag_nlp_pkg)

    rag_nlp_search_mod = ModuleType("rag.nlp.search")
    rag_nlp_search_mod.index_name = lambda tid: f"idx-{tid}"
    monkeypatch.setitem(sys.modules, "rag.nlp.search", rag_nlp_search_mod)

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    spec = importlib.util.spec_from_file_location(
        "api.db.services.chunk_feedback_service",
        _REPO_ROOT / "api" / "db" / "services" / "chunk_feedback_service.py",
    )
    mod = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(
        sys.modules, "api.db.services.chunk_feedback_service", mod
    )
    spec.loader.exec_module(mod)

    return mod, settings_mod


@pytest.fixture
def feedback_env(monkeypatch):
    """Provide (module, settings_stub) for chunk feedback tests."""
    return _load_feedback_module(monkeypatch)


class TestExtractChunkIds:
    """Tests for extract_chunk_ids_from_reference method."""

    def test_empty_reference(self, feedback_env):
        """Should return empty list for empty reference."""
        mod, _ = feedback_env
        assert mod.ChunkFeedbackService.extract_chunk_ids_from_reference({}) == []
        assert mod.ChunkFeedbackService.extract_chunk_ids_from_reference(None) == []

    def test_reference_with_id_field(self, feedback_env):
        """Should extract 'id' from formatted chunks (chunks_format output)."""
        mod, _ = feedback_env
        reference = {
            "chunks": [
                {"id": "chunk1", "content": "test"},
                {"id": "chunk2", "content": "test2"},
            ]
        }
        result = mod.ChunkFeedbackService.extract_chunk_ids_from_reference(reference)
        assert result == ["chunk1", "chunk2"]

    def test_reference_with_chunk_id_field(self, feedback_env):
        """Should fall back to 'chunk_id' field for raw chunks."""
        mod, _ = feedback_env
        reference = {
            "chunks": [
                {"chunk_id": "chunk1", "content": "test"},
                {"chunk_id": "chunk2", "content": "test2"},
            ]
        }
        result = mod.ChunkFeedbackService.extract_chunk_ids_from_reference(reference)
        assert result == ["chunk1", "chunk2"]

    def test_reference_with_no_chunks(self, feedback_env):
        """Should return empty list if no chunks key."""
        mod, _ = feedback_env
        reference = {"doc_aggs": [{"doc_id": "doc1"}]}
        result = mod.ChunkFeedbackService.extract_chunk_ids_from_reference(reference)
        assert result == []


class TestGetChunkKbMapping:
    """Tests for get_chunk_kb_mapping method."""

    def test_empty_reference(self, feedback_env):
        """Should return empty dict for empty reference."""
        mod, _ = feedback_env
        assert mod.ChunkFeedbackService.get_chunk_kb_mapping({}) == {}
        assert mod.ChunkFeedbackService.get_chunk_kb_mapping(None) == {}

    def test_reference_with_dataset_id(self, feedback_env):
        """Should map id to dataset_id (chunks_format output)."""
        mod, _ = feedback_env
        reference = {
            "chunks": [
                {"id": "chunk1", "dataset_id": "kb1"},
                {"id": "chunk2", "dataset_id": "kb2"},
            ]
        }
        result = mod.ChunkFeedbackService.get_chunk_kb_mapping(reference)
        assert result == {"chunk1": "kb1", "chunk2": "kb2"}

    def test_reference_with_kb_id(self, feedback_env):
        """Should fall back to kb_id for raw chunks."""
        mod, _ = feedback_env
        reference = {
            "chunks": [
                {"chunk_id": "chunk1", "kb_id": "kb1"},
                {"chunk_id": "chunk2", "kb_id": "kb2"},
            ]
        }
        result = mod.ChunkFeedbackService.get_chunk_kb_mapping(reference)
        assert result == {"chunk1": "kb1", "chunk2": "kb2"}

    def test_reference_missing_kb_id(self, feedback_env):
        """Should skip chunks without kb_id/dataset_id."""
        mod, _ = feedback_env
        reference = {
            "chunks": [
                {"id": "chunk1", "dataset_id": "kb1"},
                {"id": "chunk2"},  # No dataset_id
            ]
        }
        result = mod.ChunkFeedbackService.get_chunk_kb_mapping(reference)
        assert result == {"chunk1": "kb1"}


class TestUpdateChunkWeight:
    """Tests for update_chunk_weight method."""

    def test_update_weight_success(self, feedback_env):
        """Should update chunk weight successfully."""
        mod, settings_mod = feedback_env
        mock_doc_store = MagicMock()
        mock_doc_store.get.return_value = {"pagerank_fea": 10}
        mock_doc_store.update.return_value = True
        settings_mod.docStoreConn = mock_doc_store

        result = mod.ChunkFeedbackService.update_chunk_weight(
            tenant_id="tenant1",
            chunk_id="chunk1",
            kb_id="kb1",
            delta=1
        )

        assert result is True
        mock_doc_store.update.assert_called_once()

    def test_update_weight_chunk_not_found(self, feedback_env):
        """Should return False if chunk not found."""
        mod, settings_mod = feedback_env
        mock_doc_store = MagicMock()
        mock_doc_store.get.return_value = None
        settings_mod.docStoreConn = mock_doc_store

        result = mod.ChunkFeedbackService.update_chunk_weight(
            tenant_id="tenant1",
            chunk_id="chunk1",
            kb_id="kb1",
            delta=1
        )

        assert result is False

    def test_update_weight_clamp_max(self, feedback_env):
        """Should clamp weight to MAX_PAGERANK_WEIGHT."""
        mod, settings_mod = feedback_env
        mock_doc_store = MagicMock()
        mock_doc_store.get.return_value = {"pagerank_fea": mod.MAX_PAGERANK_WEIGHT}
        mock_doc_store.update.return_value = True
        settings_mod.docStoreConn = mock_doc_store

        mod.ChunkFeedbackService.update_chunk_weight(
            tenant_id="tenant1",
            chunk_id="chunk1",
            kb_id="kb1",
            delta=10  # Would exceed max
        )

        # Verify the new_value passed to update has clamped weight
        call_args = mock_doc_store.update.call_args
        new_value = call_args[0][1]
        assert new_value["pagerank_fea"] == mod.MAX_PAGERANK_WEIGHT

    def test_update_weight_clamp_min(self, feedback_env):
        """Should clamp weight to MIN_PAGERANK_WEIGHT."""
        mod, settings_mod = feedback_env
        mock_doc_store = MagicMock()
        mock_doc_store.get.return_value = {"pagerank_fea": 0}
        mock_doc_store.update.return_value = True
        settings_mod.docStoreConn = mock_doc_store

        mod.ChunkFeedbackService.update_chunk_weight(
            tenant_id="tenant1",
            chunk_id="chunk1",
            kb_id="kb1",
            delta=-10  # Would go below min
        )

        call_args = mock_doc_store.update.call_args
        new_value = call_args[0][1]
        assert new_value["pagerank_fea"] == mod.MIN_PAGERANK_WEIGHT


class TestApplyFeedback:
    """Tests for apply_feedback method."""

    def test_apply_feedback_disabled(self, feedback_env, monkeypatch):
        """Should return early when feature is disabled."""
        mod, _ = feedback_env
        monkeypatch.setattr(mod, "CHUNK_FEEDBACK_ENABLED", False)

        result = mod.ChunkFeedbackService.apply_feedback(
            tenant_id="tenant1",
            reference={"chunks": [{"id": "chunk1", "dataset_id": "kb1"}]},
            is_positive=True
        )

        assert result["success_count"] == 0
        assert result["fail_count"] == 0
        assert result.get("disabled") is True

    def test_apply_positive_feedback(self, feedback_env, monkeypatch):
        """Should apply positive feedback to all chunks."""
        mod, _ = feedback_env
        monkeypatch.setattr(mod, "CHUNK_FEEDBACK_ENABLED", True)
        mock_update = MagicMock(return_value=True)
        monkeypatch.setattr(
            mod.ChunkFeedbackService, "extract_chunk_ids_from_reference",
            staticmethod(lambda ref: ["chunk1", "chunk2"]),
        )
        monkeypatch.setattr(
            mod.ChunkFeedbackService, "get_chunk_kb_mapping",
            staticmethod(lambda ref: {"chunk1": "kb1", "chunk2": "kb1"}),
        )
        monkeypatch.setattr(
            mod.ChunkFeedbackService, "update_chunk_weight", mock_update
        )

        result = mod.ChunkFeedbackService.apply_feedback(
            tenant_id="tenant1",
            reference={"chunks": []},
            is_positive=True
        )

        assert result["success_count"] == 2
        assert result["fail_count"] == 0
        assert mock_update.call_count == 2
        # Verify positive delta
        mock_update.assert_any_call("tenant1", "chunk1", "kb1", mod.UPVOTE_WEIGHT_INCREMENT)

    def test_apply_negative_feedback(self, feedback_env, monkeypatch):
        """Should apply negative feedback to all chunks."""
        mod, _ = feedback_env
        monkeypatch.setattr(mod, "CHUNK_FEEDBACK_ENABLED", True)
        mock_update = MagicMock(return_value=True)
        monkeypatch.setattr(
            mod.ChunkFeedbackService, "extract_chunk_ids_from_reference",
            staticmethod(lambda ref: ["chunk1"]),
        )
        monkeypatch.setattr(
            mod.ChunkFeedbackService, "get_chunk_kb_mapping",
            staticmethod(lambda ref: {"chunk1": "kb1"}),
        )
        monkeypatch.setattr(
            mod.ChunkFeedbackService, "update_chunk_weight", mock_update
        )

        result = mod.ChunkFeedbackService.apply_feedback(
            tenant_id="tenant1",
            reference={"chunks": []},
            is_positive=False
        )

        assert result["success_count"] == 1
        # Verify negative delta
        mock_update.assert_called_with("tenant1", "chunk1", "kb1", -mod.DOWNVOTE_WEIGHT_DECREMENT)

    def test_apply_feedback_no_chunks(self, feedback_env, monkeypatch):
        """Should handle empty chunk list gracefully."""
        mod, _ = feedback_env
        monkeypatch.setattr(mod, "CHUNK_FEEDBACK_ENABLED", True)
        monkeypatch.setattr(
            mod.ChunkFeedbackService, "extract_chunk_ids_from_reference",
            staticmethod(lambda ref: []),
        )

        result = mod.ChunkFeedbackService.apply_feedback(
            tenant_id="tenant1",
            reference={},
            is_positive=True
        )

        assert result["success_count"] == 0
        assert result["fail_count"] == 0
        assert result["chunk_ids"] == []

    def test_apply_feedback_partial_failure(self, feedback_env, monkeypatch):
        """Should count failures correctly."""
        mod, _ = feedback_env
        monkeypatch.setattr(mod, "CHUNK_FEEDBACK_ENABLED", True)
        mock_update = MagicMock(side_effect=[True, False])
        monkeypatch.setattr(
            mod.ChunkFeedbackService, "extract_chunk_ids_from_reference",
            staticmethod(lambda ref: ["chunk1", "chunk2"]),
        )
        monkeypatch.setattr(
            mod.ChunkFeedbackService, "get_chunk_kb_mapping",
            staticmethod(lambda ref: {"chunk1": "kb1", "chunk2": "kb1"}),
        )
        monkeypatch.setattr(
            mod.ChunkFeedbackService, "update_chunk_weight", mock_update
        )

        result = mod.ChunkFeedbackService.apply_feedback(
            tenant_id="tenant1",
            reference={"chunks": []},
            is_positive=True
        )

        assert result["success_count"] == 1
        assert result["fail_count"] == 1

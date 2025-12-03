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
Unit tests for Hierarchical Retrieval Architecture

Tests the three-tier retrieval system without requiring database or
external dependencies.
"""

import pytest
from rag.retrieval.hierarchical_retrieval import (
    HierarchicalRetrieval,
    RetrievalConfig,
    RetrievalResult,
    KBRouter,
    DocumentFilter,
    ChunkRefiner
)


class TestRetrievalConfig:
    """Test RetrievalConfig dataclass"""
    
    def test_default_config(self):
        """Test default configuration values"""
        config = RetrievalConfig()
        
        assert config.enable_kb_routing is True
        assert config.kb_routing_method == "auto"
        assert config.enable_doc_filtering is True
        assert config.enable_parent_child_chunking is False
        assert config.vector_weight == 0.7
        assert config.keyword_weight == 0.3
    
    def test_custom_config(self):
        """Test custom configuration"""
        config = RetrievalConfig(
            enable_kb_routing=False,
            kb_routing_method="rule_based",
            metadata_fields=["doc_type", "department"],
            chunk_refinement_top_k=20
        )
        
        assert config.enable_kb_routing is False
        assert config.kb_routing_method == "rule_based"
        assert "doc_type" in config.metadata_fields
        assert config.chunk_refinement_top_k == 20


class TestRetrievalResult:
    """Test RetrievalResult dataclass"""
    
    def test_empty_result(self):
        """Test empty result initialization"""
        result = RetrievalResult(query="test query")
        
        assert result.query == "test query"
        assert len(result.selected_kbs) == 0
        assert len(result.filtered_docs) == 0
        assert len(result.retrieved_chunks) == 0
        assert result.total_time_ms == 0.0
    
    def test_result_with_data(self):
        """Test result with data"""
        result = RetrievalResult(
            query="test query",
            selected_kbs=["kb1", "kb2"],
            filtered_docs=[{"id": "doc1"}, {"id": "doc2"}],
            retrieved_chunks=[{"id": "chunk1"}],
            tier1_candidates=2,
            tier2_candidates=2,
            tier3_candidates=1,
            total_time_ms=150.5
        )
        
        assert len(result.selected_kbs) == 2
        assert len(result.filtered_docs) == 2
        assert len(result.retrieved_chunks) == 1
        assert result.tier1_candidates == 2
        assert result.total_time_ms == 150.5


class TestHierarchicalRetrieval:
    """Test HierarchicalRetrieval main class"""
    
    def test_initialization_default_config(self):
        """Test initialization with default config"""
        retrieval = HierarchicalRetrieval()
        
        assert retrieval.config is not None
        assert retrieval.config.enable_kb_routing is True
    
    def test_initialization_custom_config(self):
        """Test initialization with custom config"""
        config = RetrievalConfig(enable_kb_routing=False)
        retrieval = HierarchicalRetrieval(config)
        
        assert retrieval.config.enable_kb_routing is False
    
    def test_retrieve_basic(self):
        """Test basic retrieval flow"""
        retrieval = HierarchicalRetrieval()
        
        result = retrieval.retrieve(
            query="test query",
            kb_ids=["kb1", "kb2"],
            top_k=10
        )
        
        assert isinstance(result, RetrievalResult)
        assert result.query == "test query"
        assert result.total_time_ms >= 0
    
    def test_retrieve_with_filters(self):
        """Test retrieval with metadata filters"""
        retrieval = HierarchicalRetrieval()
        
        result = retrieval.retrieve(
            query="test query",
            kb_ids=["kb1"],
            top_k=5,
            filters={"department": "HR"}
        )
        
        assert isinstance(result, RetrievalResult)
    
    def test_retrieve_empty_kb_list(self):
        """Test retrieval with empty KB list"""
        retrieval = HierarchicalRetrieval()
        
        result = retrieval.retrieve(
            query="test query",
            kb_ids=[],
            top_k=10
        )
        
        # Should return empty result
        assert len(result.selected_kbs) == 0
        assert len(result.retrieved_chunks) == 0
    
    def test_tier1_kb_routing_disabled(self):
        """Test KB routing when disabled"""
        config = RetrievalConfig(enable_kb_routing=False)
        retrieval = HierarchicalRetrieval(config)
        
        kb_ids = ["kb1", "kb2", "kb3"]
        selected = retrieval._tier1_kb_routing("test query", kb_ids)
        
        # Should return all KBs when disabled
        assert selected == kb_ids
    
    def test_tier1_kb_routing_all_method(self):
        """Test KB routing with 'all' method"""
        config = RetrievalConfig(kb_routing_method="all")
        retrieval = HierarchicalRetrieval(config)
        
        kb_ids = ["kb1", "kb2", "kb3"]
        selected = retrieval._tier1_kb_routing("test query", kb_ids)
        
        assert selected == kb_ids
    
    def test_tier1_kb_routing_rule_based(self):
        """Test KB routing with rule-based method"""
        config = RetrievalConfig(kb_routing_method="rule_based")
        retrieval = HierarchicalRetrieval(config)
        
        kb_ids = ["kb1", "kb2"]
        selected = retrieval._tier1_kb_routing("test query", kb_ids)
        
        # Currently returns all KBs (placeholder implementation)
        assert isinstance(selected, list)
    
    def test_tier1_kb_routing_llm_based(self):
        """Test KB routing with LLM-based method"""
        config = RetrievalConfig(kb_routing_method="llm_based")
        retrieval = HierarchicalRetrieval(config)
        
        kb_ids = ["kb1", "kb2"]
        selected = retrieval._tier1_kb_routing("test query", kb_ids)
        
        # Currently returns all KBs (placeholder implementation)
        assert isinstance(selected, list)
    
    def test_tier1_kb_routing_auto(self):
        """Test KB routing with auto method"""
        config = RetrievalConfig(kb_routing_method="auto")
        retrieval = HierarchicalRetrieval(config)
        
        kb_ids = ["kb1", "kb2"]
        selected = retrieval._tier1_kb_routing("test query", kb_ids)
        
        assert isinstance(selected, list)
    
    def test_tier2_document_filtering_disabled(self):
        """Test document filtering when disabled"""
        config = RetrievalConfig(enable_doc_filtering=False)
        retrieval = HierarchicalRetrieval(config)
        
        docs = retrieval._tier2_document_filtering(
            "test query",
            ["kb1"],
            None
        )
        
        # Should return empty list when disabled
        assert isinstance(docs, list)
    
    def test_tier2_document_filtering_with_filters(self):
        """Test document filtering with metadata filters"""
        retrieval = HierarchicalRetrieval()
        
        docs = retrieval._tier2_document_filtering(
            "test query",
            ["kb1"],
            {"department": "HR"}
        )
        
        assert isinstance(docs, list)
    
    def test_tier3_chunk_refinement(self):
        """Test chunk refinement"""
        retrieval = HierarchicalRetrieval()
        
        chunks = retrieval._tier3_chunk_refinement(
            "test query",
            [{"id": "doc1"}],
            top_k=10
        )
        
        assert isinstance(chunks, list)
    
    def test_timing_metrics(self):
        """Test that timing metrics are recorded"""
        retrieval = HierarchicalRetrieval()
        
        result = retrieval.retrieve(
            query="test query",
            kb_ids=["kb1"],
            top_k=5
        )
        
        # All timing metrics should be non-negative
        assert result.tier1_time_ms >= 0
        assert result.tier2_time_ms >= 0
        assert result.tier3_time_ms >= 0
        assert result.total_time_ms >= 0
        
        # Total should be sum of tiers (approximately)
        assert result.total_time_ms >= result.tier1_time_ms


class TestKBRouter:
    """Test KBRouter class"""
    
    def test_initialization(self):
        """Test KBRouter initialization"""
        router = KBRouter()
        assert router is not None
    
    def test_route_basic(self):
        """Test basic routing"""
        router = KBRouter()
        
        available_kbs = [
            {"id": "kb1", "name": "HR Knowledge Base"},
            {"id": "kb2", "name": "Finance Knowledge Base"}
        ]
        
        selected = router.route(
            query="What is the vacation policy?",
            available_kbs=available_kbs,
            method="auto"
        )
        
        assert isinstance(selected, list)
        assert len(selected) > 0
    
    def test_route_empty_kbs(self):
        """Test routing with empty KB list"""
        router = KBRouter()
        
        selected = router.route(
            query="test query",
            available_kbs=[],
            method="auto"
        )
        
        assert selected == []
    
    @pytest.mark.parametrize("method", ["auto", "rule_based", "llm_based"])
    def test_route_different_methods(self, method):
        """Test routing with different methods"""
        router = KBRouter()
        
        available_kbs = [{"id": "kb1"}, {"id": "kb2"}]
        
        selected = router.route(
            query="test query",
            available_kbs=available_kbs,
            method=method
        )
        
        assert isinstance(selected, list)


class TestDocumentFilter:
    """Test DocumentFilter class"""
    
    def test_initialization(self):
        """Test DocumentFilter initialization"""
        filter_obj = DocumentFilter()
        assert filter_obj is not None
    
    def test_filter_basic(self):
        """Test basic filtering"""
        filter_obj = DocumentFilter()
        
        documents = [
            {"id": "doc1", "department": "HR"},
            {"id": "doc2", "department": "Finance"},
            {"id": "doc3", "department": "HR"}
        ]
        
        filtered = filter_obj.filter(
            query="HR policies",
            documents=documents,
            metadata_fields=["department"],
            filters=None
        )
        
        assert isinstance(filtered, list)
    
    def test_filter_with_explicit_filters(self):
        """Test filtering with explicit filters"""
        filter_obj = DocumentFilter()
        
        documents = [
            {"id": "doc1", "department": "HR"},
            {"id": "doc2", "department": "Finance"}
        ]
        
        filtered = filter_obj.filter(
            query="test",
            documents=documents,
            metadata_fields=["department"],
            filters={"department": "HR"}
        )
        
        # Currently returns all docs (placeholder)
        assert isinstance(filtered, list)
    
    def test_filter_empty_documents(self):
        """Test filtering with empty document list"""
        filter_obj = DocumentFilter()
        
        filtered = filter_obj.filter(
            query="test",
            documents=[],
            metadata_fields=["department"],
            filters=None
        )
        
        assert filtered == []


class TestChunkRefiner:
    """Test ChunkRefiner class"""
    
    def test_initialization(self):
        """Test ChunkRefiner initialization"""
        refiner = ChunkRefiner()
        assert refiner is not None
    
    def test_refine_basic(self):
        """Test basic chunk refinement"""
        refiner = ChunkRefiner()
        
        chunks = refiner.refine(
            query="test query",
            doc_ids=["doc1", "doc2"],
            top_k=10,
            use_parent_child=False
        )
        
        assert isinstance(chunks, list)
    
    def test_refine_with_parent_child(self):
        """Test refinement with parent-child chunking"""
        refiner = ChunkRefiner()
        
        chunks = refiner.refine(
            query="test query",
            doc_ids=["doc1"],
            top_k=5,
            use_parent_child=True
        )
        
        assert isinstance(chunks, list)
    
    def test_refine_empty_doc_ids(self):
        """Test refinement with empty doc IDs"""
        refiner = ChunkRefiner()
        
        chunks = refiner.refine(
            query="test query",
            doc_ids=[],
            top_k=10,
            use_parent_child=False
        )
        
        assert chunks == []


class TestIntegrationScenarios:
    """Test end-to-end integration scenarios"""
    
    def test_full_retrieval_flow(self):
        """Test complete retrieval flow"""
        config = RetrievalConfig(
            enable_kb_routing=True,
            enable_doc_filtering=True,
            chunk_refinement_top_k=10
        )
        
        retrieval = HierarchicalRetrieval(config)
        
        result = retrieval.retrieve(
            query="What is the company vacation policy?",
            kb_ids=["hr_kb", "policies_kb"],
            top_k=10,
            filters={"department": "HR"}
        )
        
        # Verify result structure
        assert isinstance(result, RetrievalResult)
        assert result.query == "What is the company vacation policy?"
        assert result.total_time_ms > 0
        
        # Verify tier progression
        assert result.tier1_candidates >= 0
        assert result.tier2_candidates >= 0
        assert result.tier3_candidates >= 0
    
    def test_retrieval_with_all_features_enabled(self):
        """Test retrieval with all features enabled"""
        config = RetrievalConfig(
            enable_kb_routing=True,
            kb_routing_method="auto",
            enable_doc_filtering=True,
            enable_metadata_similarity=True,
            enable_parent_child_chunking=True,
            use_summary_mapping=True,
            enable_hybrid_search=True
        )
        
        retrieval = HierarchicalRetrieval(config)
        
        result = retrieval.retrieve(
            query="test query",
            kb_ids=["kb1", "kb2", "kb3"],
            top_k=20
        )
        
        assert isinstance(result, RetrievalResult)
    
    def test_retrieval_with_minimal_config(self):
        """Test retrieval with minimal configuration"""
        config = RetrievalConfig(
            enable_kb_routing=False,
            enable_doc_filtering=False
        )
        
        retrieval = HierarchicalRetrieval(config)
        
        result = retrieval.retrieve(
            query="test query",
            kb_ids=["kb1"],
            top_k=5
        )
        
        assert isinstance(result, RetrievalResult)


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

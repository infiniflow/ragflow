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
Unit tests for hierarchical retrieval integration in search.py.
"""

import pytest
from unittest.mock import Mock

from rag.nlp.search import (
    HierarchicalConfig,
    HierarchicalResult,
    KBRetrievalParams,
    Dealer,
    index_name,
)


class TestHierarchicalConfig:
    """Test HierarchicalConfig dataclass."""

    def test_default_values(self):
        """Test default configuration values."""
        config = HierarchicalConfig()
        
        assert config.enabled is False
        assert config.enable_kb_routing is True
        assert config.kb_routing_threshold == 0.3
        assert config.kb_top_k == 3
        assert config.enable_doc_filtering is True
        assert config.doc_top_k == 100
        assert config.chunk_top_k == 10

    def test_custom_values(self):
        """Test custom configuration values."""
        config = HierarchicalConfig(
            enabled=True,
            kb_routing_threshold=0.5,
            kb_top_k=5,
            doc_top_k=50,
        )
        
        assert config.enabled is True
        assert config.kb_routing_threshold == 0.5
        assert config.kb_top_k == 5
        assert config.doc_top_k == 50


class TestHierarchicalResult:
    """Test HierarchicalResult dataclass."""

    def test_default_values(self):
        """Test default result values."""
        result = HierarchicalResult()
        
        assert result.selected_kb_ids == []
        assert result.filtered_doc_ids == []
        assert result.tier1_time_ms == 0.0
        assert result.tier2_time_ms == 0.0
        assert result.tier3_time_ms == 0.0
        assert result.total_time_ms == 0.0

    def test_populated_result(self):
        """Test result with data."""
        result = HierarchicalResult(
            selected_kb_ids=["kb1", "kb2"],
            filtered_doc_ids=["doc1", "doc2", "doc3"],
            tier1_time_ms=10.5,
            tier2_time_ms=25.3,
            tier3_time_ms=100.2,
            total_time_ms=136.0,
        )
        
        assert len(result.selected_kb_ids) == 2
        assert len(result.filtered_doc_ids) == 3
        assert result.total_time_ms == 136.0


class TestTier1KBRouting:
    """Test Tier 1: KB Routing logic."""

    @pytest.fixture
    def mock_dealer(self):
        """Create a mock Dealer instance."""
        mock_datastore = Mock()
        dealer = Dealer(mock_datastore)
        return dealer

    def test_routing_disabled(self, mock_dealer):
        """Test that routing returns all KBs when disabled."""
        config = HierarchicalConfig(enable_kb_routing=False)
        kb_ids = ["kb1", "kb2", "kb3"]
        
        result = mock_dealer._tier1_kb_routing("test query", kb_ids, None, config)
        
        assert result == kb_ids

    def test_routing_no_kb_infos(self, mock_dealer):
        """Test that routing returns all KBs when no KB info provided."""
        config = HierarchicalConfig(enable_kb_routing=True)
        kb_ids = ["kb1", "kb2", "kb3"]
        
        result = mock_dealer._tier1_kb_routing("test query", kb_ids, None, config)
        
        assert result == kb_ids

    def test_routing_few_kbs(self, mock_dealer):
        """Test that routing returns all KBs when count <= kb_top_k."""
        config = HierarchicalConfig(enable_kb_routing=True, kb_top_k=5)
        kb_ids = ["kb1", "kb2", "kb3"]
        kb_infos = [
            {"id": "kb1", "name": "Finance KB", "description": "Financial documents"},
            {"id": "kb2", "name": "HR KB", "description": "Human resources"},
            {"id": "kb3", "name": "Tech KB", "description": "Technical docs"},
        ]
        
        result = mock_dealer._tier1_kb_routing("test query", kb_ids, kb_infos, config)
        
        assert result == kb_ids

    def test_routing_selects_relevant_kbs(self, mock_dealer):
        """Test that routing selects KBs based on keyword overlap."""
        config = HierarchicalConfig(
            enable_kb_routing=True, 
            kb_top_k=2,
            kb_routing_threshold=0.1
        )
        kb_ids = ["kb1", "kb2", "kb3", "kb4"]
        kb_infos = [
            {"id": "kb1", "name": "Finance Reports", "description": "Financial analysis and reports"},
            {"id": "kb2", "name": "HR Policies", "description": "Human resources policies"},
            {"id": "kb3", "name": "Technical Documentation", "description": "Engineering docs"},
            {"id": "kb4", "name": "Financial Statements", "description": "Quarterly financial data"},
        ]
        
        # Query about finance should select finance-related KBs
        result = mock_dealer._tier1_kb_routing("financial report analysis", kb_ids, kb_infos, config)
        
        # Should select at most kb_top_k KBs
        assert len(result) <= config.kb_top_k
        # Should include finance-related KBs
        assert any(kb in result for kb in ["kb1", "kb4"])


class TestTier2DocumentFiltering:
    """Test Tier 2: Document Filtering logic."""

    @pytest.fixture
    def mock_dealer(self):
        """Create a mock Dealer instance."""
        mock_datastore = Mock()
        dealer = Dealer(mock_datastore)
        return dealer

    def test_filtering_disabled(self, mock_dealer):
        """Test that filtering returns empty when disabled."""
        config = HierarchicalConfig(enable_doc_filtering=False)
        
        result = mock_dealer._tier2_document_filtering(
            "test query", ["tenant1"], ["kb1"], None, config
        )
        
        assert result == []

    def test_filtering_with_existing_doc_ids(self, mock_dealer):
        """Test that filtering limits existing doc_ids."""
        config = HierarchicalConfig(enable_doc_filtering=True, doc_top_k=2)
        doc_ids = ["doc1", "doc2", "doc3", "doc4"]
        
        result = mock_dealer._tier2_document_filtering(
            "test query", ["tenant1"], ["kb1"], doc_ids, config
        )
        
        assert len(result) == 2
        assert result == ["doc1", "doc2"]


class TestHierarchicalRetrieval:
    """Test the full hierarchical retrieval flow."""

    @pytest.fixture
    def mock_dealer(self):
        """Create a mock Dealer instance with mocked methods."""
        mock_datastore = Mock()
        dealer = Dealer(mock_datastore)
        
        # Mock the retrieval method
        dealer.retrieval = Mock(return_value={
            "total": 5,
            "chunks": [
                {"chunk_id": "c1", "content_with_weight": "test content 1"},
                {"chunk_id": "c2", "content_with_weight": "test content 2"},
            ],
            "doc_aggs": [],
        })
        
        return dealer

    def test_hierarchical_retrieval_basic(self, mock_dealer):
        """Test basic hierarchical retrieval flow."""
        config = HierarchicalConfig(
            enabled=True,
            enable_kb_routing=False,  # Skip routing for this test
            enable_doc_filtering=False,  # Skip filtering for this test
        )
        
        result = mock_dealer.hierarchical_retrieval(
            question="test query",
            embd_mdl=Mock(),
            tenant_ids=["tenant1"],
            kb_ids=["kb1", "kb2"],
            hierarchical_config=config,
        )
        
        # Should have chunks from retrieval
        assert "chunks" in result
        assert len(result["chunks"]) == 2
        
        # Should have hierarchical metadata
        assert "hierarchical_metadata" in result
        metadata = result["hierarchical_metadata"]
        assert "tier1_time_ms" in metadata
        assert "tier2_time_ms" in metadata
        assert "tier3_time_ms" in metadata
        assert "total_time_ms" in metadata

    def test_hierarchical_retrieval_with_kb_infos(self, mock_dealer):
        """Test hierarchical retrieval with KB information."""
        config = HierarchicalConfig(
            enabled=True,
            enable_kb_routing=True,
            kb_top_k=2,
        )
        
        kb_infos = [
            {"id": "kb1", "name": "Finance", "description": "Financial docs"},
            {"id": "kb2", "name": "HR", "description": "HR policies"},
        ]
        
        result = mock_dealer.hierarchical_retrieval(
            question="financial report",
            embd_mdl=Mock(),
            tenant_ids=["tenant1"],
            kb_ids=["kb1", "kb2"],
            kb_infos=kb_infos,
            hierarchical_config=config,
        )
        
        assert "hierarchical_metadata" in result
        assert "selected_kb_ids" in result["hierarchical_metadata"]

    def test_hierarchical_retrieval_empty_query(self, mock_dealer):
        """Test hierarchical retrieval with empty query."""
        # Mock retrieval to return empty for empty query
        mock_dealer.retrieval = Mock(return_value={
            "total": 0,
            "chunks": [],
            "doc_aggs": [],
        })
        
        config = HierarchicalConfig(enabled=True)
        
        result = mock_dealer.hierarchical_retrieval(
            question="",
            embd_mdl=Mock(),
            tenant_ids=["tenant1"],
            kb_ids=["kb1"],
            hierarchical_config=config,
        )
        
        assert result["total"] == 0
        assert result["chunks"] == []


class TestIndexName:
    """Test index_name utility function."""

    def test_index_name_format(self):
        """Test index name formatting."""
        assert index_name("user123") == "ragflow_user123"
        assert index_name("tenant_abc") == "ragflow_tenant_abc"


class TestHierarchicalConfigAdvanced:
    """Test advanced HierarchicalConfig options."""

    def test_all_config_options(self):
        """Test all configuration options."""
        config = HierarchicalConfig(
            enabled=True,
            enable_kb_routing=True,
            kb_routing_method="llm_based",
            kb_routing_threshold=0.5,
            kb_top_k=5,
            enable_doc_filtering=True,
            doc_top_k=50,
            metadata_fields=["department", "doc_type"],
            enable_metadata_similarity=True,
            metadata_similarity_threshold=0.8,
            use_llm_metadata_filter=True,
            chunk_top_k=20,
            enable_parent_child=True,
            use_summary_mapping=True,
            keyword_extraction_prompt="Extract important technical terms",
            question_generation_prompt="Generate questions about the content",
        )
        
        assert config.kb_routing_method == "llm_based"
        assert config.metadata_fields == ["department", "doc_type"]
        assert config.enable_metadata_similarity is True
        assert config.use_llm_metadata_filter is True
        assert config.enable_parent_child is True
        assert config.use_summary_mapping is True
        assert config.keyword_extraction_prompt is not None


class TestLLMKBRouting:
    """Test LLM-based KB routing."""

    @pytest.fixture
    def mock_dealer(self):
        """Create a mock Dealer instance."""
        mock_datastore = Mock()
        dealer = Dealer(mock_datastore)
        return dealer

    def test_llm_routing_fallback_no_model(self, mock_dealer):
        """Test LLM routing falls back when no model provided."""
        config = HierarchicalConfig(
            enable_kb_routing=True,
            kb_routing_method="llm_based",
            kb_top_k=2
        )
        kb_ids = ["kb1", "kb2", "kb3", "kb4"]
        kb_infos = [
            {"id": "kb1", "name": "Finance", "description": "Financial docs"},
            {"id": "kb2", "name": "HR", "description": "HR policies"},
            {"id": "kb3", "name": "Tech", "description": "Technical docs"},
            {"id": "kb4", "name": "Legal", "description": "Legal documents"},
        ]
        
        # Without chat_mdl, should fall back to rule-based
        result = mock_dealer._tier1_kb_routing(
            "financial report", kb_ids, kb_infos, config, chat_mdl=None
        )
        
        # Should still return results (from rule-based fallback)
        assert len(result) <= config.kb_top_k

    def test_routing_method_all(self, mock_dealer):
        """Test 'all' routing method returns all KBs."""
        config = HierarchicalConfig(
            enable_kb_routing=True,
            kb_routing_method="all",
            kb_top_k=2
        )
        kb_ids = ["kb1", "kb2", "kb3", "kb4"]
        kb_infos = [
            {"id": "kb1", "name": "Finance", "description": "Financial docs"},
            {"id": "kb2", "name": "HR", "description": "HR policies"},
        ]
        
        result = mock_dealer._tier1_kb_routing(
            "any query", kb_ids, kb_infos, config
        )
        
        assert result == kb_ids


class TestMetadataSimilarityFilter:
    """Test metadata similarity filtering."""

    @pytest.fixture
    def mock_dealer(self):
        """Create a mock Dealer instance."""
        mock_datastore = Mock()
        dealer = Dealer(mock_datastore)
        return dealer

    def test_similarity_filter_no_model(self, mock_dealer):
        """Test similarity filter returns empty without embedding model."""
        config = HierarchicalConfig(
            enable_metadata_similarity=True,
            metadata_similarity_threshold=0.7
        )
        
        doc_metadata = [
            {"id": "doc1", "name": "Finance Report", "summary": "Q1 financial analysis"},
            {"id": "doc2", "name": "HR Policy", "summary": "Employee guidelines"},
        ]
        
        result = mock_dealer._metadata_similarity_filter(
            "financial analysis", doc_metadata, config, embd_mdl=None
        )
        
        assert result == []

    def test_similarity_filter_no_metadata(self, mock_dealer):
        """Test similarity filter returns empty without metadata."""
        config = HierarchicalConfig(enable_metadata_similarity=True)
        mock_embd = Mock()
        
        result = mock_dealer._metadata_similarity_filter(
            "test query", [], config, embd_mdl=mock_embd
        )
        
        assert result == []


class TestParentChildRetrieval:
    """Test parent-child chunking with summary mapping."""

    @pytest.fixture
    def mock_dealer(self):
        """Create a mock Dealer instance with mocked methods."""
        mock_datastore = Mock()
        dealer = Dealer(mock_datastore)
        
        # Mock the search method
        mock_search_result = Mock()
        mock_search_result.ids = ["chunk1", "chunk2"]
        mock_search_result.field = {
            "chunk1": {
                "content_ltks": "parent content",
                "content_with_weight": "parent content",
                "doc_id": "doc1",
                "docnm_kwd": "test.pdf",
                "kb_id": "kb1",
                "mom_id": "",
            },
            "chunk2": {
                "content_ltks": "child content",
                "content_with_weight": "child content", 
                "doc_id": "doc1",
                "docnm_kwd": "test.pdf",
                "kb_id": "kb1",
                "mom_id": "chunk1",
            },
        }
        mock_search_result.total = 2
        mock_search_result.query_vector = [0.1] * 768
        mock_search_result.highlight = {}
        
        dealer.search = Mock(return_value=mock_search_result)
        dealer.rerank = Mock(return_value=([0.9, 0.8], [0.5, 0.4], [0.9, 0.8]))
        
        return dealer

    def test_summary_mapping_config(self):
        """Test summary mapping configuration."""
        config = HierarchicalConfig(
            enable_parent_child=True,
            use_summary_mapping=True,
        )
        
        assert config.enable_parent_child is True
        assert config.use_summary_mapping is True


class TestCustomKeywordExtraction:
    """Test customizable keyword extraction."""

    @pytest.fixture
    def mock_dealer(self):
        """Create a mock Dealer instance."""
        mock_datastore = Mock()
        dealer = Dealer(mock_datastore)
        return dealer

    def test_keyword_extraction_important(self, mock_dealer):
        """Test keyword extraction with 'important' prompt."""
        chunks = [
            {
                "content_with_weight": "The API_KEY and DatabaseConnection are critical components.",
                "important_kwd": ["existing_term"],  # Pre-existing keyword
            }
        ]
        
        result = mock_dealer._apply_custom_keyword_extraction(
            chunks, "Extract important technical terms", None
        )
        
        # Should return the chunk (possibly with extracted keywords)
        assert len(result) == 1
        # Should preserve existing keywords at minimum
        assert "existing_term" in result[0].get("important_kwd", [])

    def test_keyword_extraction_question(self, mock_dealer):
        """Test keyword extraction with 'question' prompt."""
        chunks = [
            {
                "content_with_weight": "This section explains the authentication flow for the system. Users must provide valid credentials.",
                "important_kwd": [],
            }
        ]
        
        result = mock_dealer._apply_custom_keyword_extraction(
            chunks, "Generate questions about the content", None
        )
        
        assert len(result) == 1
        # Should have a question hint
        assert "question_hint" in result[0]

    def test_keyword_extraction_empty_content(self, mock_dealer):
        """Test keyword extraction with empty content."""
        chunks = [{"content_with_weight": "", "important_kwd": []}]
        
        result = mock_dealer._apply_custom_keyword_extraction(
            chunks, "Extract important terms", None
        )
        
        assert len(result) == 1


class TestKBRetrievalParams:
    """Test per-KB retrieval parameters."""

    def test_default_params(self):
        """Test default KB params."""
        params = KBRetrievalParams(kb_id="kb1")
        
        assert params.kb_id == "kb1"
        assert params.vector_similarity_weight == 0.7
        assert params.similarity_threshold == 0.2
        assert params.top_k == 1024
        assert params.rerank_enabled is True

    def test_custom_params(self):
        """Test custom KB params."""
        params = KBRetrievalParams(
            kb_id="finance_kb",
            vector_similarity_weight=0.9,
            similarity_threshold=0.3,
            top_k=500,
            rerank_enabled=False
        )
        
        assert params.kb_id == "finance_kb"
        assert params.vector_similarity_weight == 0.9
        assert params.similarity_threshold == 0.3
        assert params.top_k == 500
        assert params.rerank_enabled is False

    def test_kb_params_in_config(self):
        """Test KB params integration in HierarchicalConfig."""
        kb_params = {
            "kb1": KBRetrievalParams(kb_id="kb1", vector_similarity_weight=0.8),
            "kb2": KBRetrievalParams(kb_id="kb2", similarity_threshold=0.4),
        }
        
        config = HierarchicalConfig(
            enabled=True,
            kb_params=kb_params
        )
        
        assert len(config.kb_params) == 2
        assert config.kb_params["kb1"].vector_similarity_weight == 0.8
        assert config.kb_params["kb2"].similarity_threshold == 0.4


class TestLLMQuestionGeneration:
    """Test LLM-based question generation."""

    @pytest.fixture
    def mock_dealer(self):
        """Create a mock Dealer instance."""
        mock_datastore = Mock()
        dealer = Dealer(mock_datastore)
        return dealer

    def test_question_generation_no_model(self, mock_dealer):
        """Test question generation returns chunks unchanged without model."""
        chunks = [{"content_with_weight": "Test content", "important_kwd": []}]
        
        result = mock_dealer._apply_llm_question_generation(chunks, None, None)
        
        assert result == chunks
        assert "generated_questions" not in result[0]

    def test_question_generation_with_mock_model(self, mock_dealer):
        """Test question generation with mock LLM."""
        mock_chat = Mock()
        mock_chat.chat = Mock(return_value="What is the main topic?\nHow does this work?")
        
        chunks = [{
            "content_with_weight": "This is a detailed explanation of the authentication system. It uses OAuth2 for secure access.",
            "important_kwd": []
        }]
        
        result = mock_dealer._apply_llm_question_generation(chunks, None, mock_chat)
        
        assert len(result) == 1
        assert "generated_questions" in result[0]
        assert len(result[0]["generated_questions"]) == 2


class TestDocumentMetadataGeneration:
    """Test document metadata generation."""

    @pytest.fixture
    def mock_dealer(self):
        """Create a mock Dealer instance."""
        mock_datastore = Mock()
        dealer = Dealer(mock_datastore)
        return dealer

    def test_metadata_generation_no_model(self, mock_dealer):
        """Test metadata generation returns empty without model."""
        result = mock_dealer.generate_document_metadata("doc1", "content", None, None)
        
        assert result["doc_id"] == "doc1"
        assert result["generated"] is True
        assert result["summary"] == ""
        assert result["topics"] == []

    def test_metadata_generation_with_mock_model(self, mock_dealer):
        """Test metadata generation with mock LLM."""
        mock_chat = Mock()
        mock_chat.chat = Mock(return_value="""SUMMARY: This document explains authentication.
TOPICS: security, OAuth, authentication
QUESTIONS:
- What is OAuth?
- How does authentication work?
CATEGORY: Technical Documentation""")
        
        result = mock_dealer.generate_document_metadata(
            "doc1", 
            "This is content about authentication and security.",
            mock_chat,
            None
        )
        
        assert result["doc_id"] == "doc1"
        assert "authentication" in result["summary"]
        assert len(result["topics"]) == 3
        assert "security" in result["topics"]
        assert len(result["suggested_questions"]) == 2
        assert result["category"] == "Technical Documentation"


class TestFullHierarchicalConfig:
    """Test complete hierarchical configuration."""

    def test_all_features_enabled(self):
        """Test config with all features enabled."""
        config = HierarchicalConfig(
            enabled=True,
            # Tier 1
            enable_kb_routing=True,
            kb_routing_method="llm_based",
            kb_routing_threshold=0.4,
            kb_top_k=5,
            kb_params={
                "kb1": KBRetrievalParams(kb_id="kb1", vector_similarity_weight=0.9)
            },
            # Tier 2
            enable_doc_filtering=True,
            doc_top_k=50,
            metadata_fields=["department", "author"],
            enable_metadata_similarity=True,
            metadata_similarity_threshold=0.8,
            use_llm_metadata_filter=True,
            # Tier 3
            chunk_top_k=20,
            enable_parent_child=True,
            use_summary_mapping=True,
            # Prompts
            keyword_extraction_prompt="Extract domain-specific terms",
            question_generation_prompt="Generate FAQ questions",
            use_llm_question_generation=True,
        )
        
        assert config.enabled is True
        assert config.kb_routing_method == "llm_based"
        assert len(config.kb_params) == 1
        assert config.enable_metadata_similarity is True
        assert config.use_llm_metadata_filter is True
        assert config.enable_parent_child is True
        assert config.use_summary_mapping is True
        assert config.use_llm_question_generation is True


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

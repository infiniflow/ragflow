#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
Minimal integration tests for Query Intent Router integration.

These tests verify:
1. get_chunk_content() function - priority logic for original chunk content
2. QueryIntentRouter classification logic
3. Multi-condition query decomposition
"""

import pytest
import sys
import types


def _install_stubs():
    """Install minimal stubs for dependencies that might not be available."""
    try:
        import tiktoken
        return
    except Exception:
        pass
    
    stub_encoder = types.ModuleType("tiktoken")
    stub_encoder.Encoding = type("Encoding", (), {})
    sys.modules["tiktoken"] = stub_encoder


_install_stubs()

from rag.prompts.generator import get_chunk_content


class TestChunkContentPriority:
    """Test get_chunk_content() function - priority logic for original chunk content."""

    def test_regular_chunk_uses_content_with_weight(self):
        """Regular chunk (not numeric/tabular) should use content_with_weight."""
        chunk = {
            "content_with_weight": "This is the processed content with weights.",
            "content_ltks": "This is the processed content.",
        }
        
        result = get_chunk_content(chunk)
        assert result == "This is the processed content with weights."

    def test_regular_chunk_uses_content_if_present(self):
        """Regular chunk should prefer 'content' over 'content_with_weight' if 'content' exists."""
        chunk = {
            "content": "This is the direct content.",
            "content_with_weight": "This is the weighted content.",
        }
        
        result = get_chunk_content(chunk)
        assert result == "This is the direct content."

    def test_numeric_tabular_chunk_preserves_original(self):
        """Numeric/tabular chunk with preserve_original_content=True should use original_content_with_weight."""
        chunk = {
            "preserve_original_content": True,
            "original_content_with_weight": "2023年公司总营收：1500万元，同比增长25%。原始表格数据：|月份|销售额|季度|1月|500万|Q1|2月|450万|Q1|",
            "content_with_weight": "2023年营收增长情况。",
        }
        
        result = get_chunk_content(chunk)
        assert result == "2023年公司总营收：1500万元，同比增长25%。原始表格数据：|月份|销售额|季度|1月|500万|Q1|2月|450万|Q1|"

    def test_numeric_tabular_chunk_falls_back_if_original_missing(self):
        """If original_content_with_weight is missing, fall back to regular content."""
        chunk = {
            "preserve_original_content": True,
            "content_with_weight": "This is the fallback content.",
        }
        
        result = get_chunk_content(chunk)
        assert result == "This is the fallback content."

    def test_numeric_tabular_chunk_falls_back_to_content(self):
        """If original_content_with_weight is missing but content exists, use content."""
        chunk = {
            "preserve_original_content": True,
            "content": "This is the direct fallback content.",
            "content_with_weight": "This is the weighted fallback.",
        }
        
        result = get_chunk_content(chunk)
        assert result == "This is the direct fallback content."

    def test_empty_chunk_returns_none(self):
        """Empty chunk should return None."""
        chunk = {}
        result = get_chunk_content(chunk)
        assert result is None


class TestQueryIntentRouter:
    """Test QueryIntentRouter classification and parameter generation."""

    def setup_method(self):
        from rag.nlp.query_router import QueryIntentRouter
        self.router = QueryIntentRouter()

    def test_factual_query_classification(self):
        """Factual queries should be classified correctly."""
        from rag.nlp.query_router import QueryType
        
        intent = self.router.route("什么是人工智能？")
        assert intent.query_type == QueryType.FACTUAL
        assert intent.retrieval_strategy == "exact"
        
        params = self.router.get_retrieval_params(intent)
        assert params["min_match"] == 0.7
        assert params["vector_weight"] == 0.3
        assert params["term_weight"] == 0.7

    def test_multi_condition_query_classification(self):
        """Multi-condition queries should be classified with sub-queries."""
        from rag.nlp.query_router import QueryType
        
        intent = self.router.route("分析机器学习和深度学习的区别，以及它们的应用场景。")
        assert intent.query_type == QueryType.MULTI_CONDITION
        assert intent.retrieval_strategy == "multi_query"
        assert intent.sub_queries is not None
        assert len(intent.sub_queries) >= 1
        
        params = self.router.get_retrieval_params(intent)
        assert params["min_match"] == 0.4
        assert params["vector_weight"] == 0.5
        assert "sub_queries" in params

    def test_open_ended_query_classification(self):
        """Open-ended queries should have topk_multiplier for expanded retrieval."""
        from rag.nlp.query_router import QueryType
        
        intent = self.router.route("谈谈你对人工智能未来发展的看法。")
        assert intent.query_type == QueryType.OPEN_ENDED
        assert intent.retrieval_strategy == "hybrid"
        
        params = self.router.get_retrieval_params(intent)
        assert params["topk_multiplier"] == 1.5
        assert params["min_match"] == 0.3
        assert params["vector_weight"] == 0.7

    def test_numeric_tabular_query_classification(self):
        """Numeric/tabular queries should have preserve_original_chunks flag."""
        from rag.nlp.query_router import QueryType
        
        intent = self.router.route("2023年公司的总营收是多少？")
        assert intent.query_type == QueryType.NUMERIC_TABULAR
        assert intent.retrieval_strategy == "hybrid_with_original_chunks"
        assert intent.metadata is not None
        assert intent.metadata.get("preserve_original_chunks") == True
        
        params = self.router.get_retrieval_params(intent)
        assert params["preserve_original_chunks"] == True
        assert params["highlight_numerics"] == True
        assert params["min_match"] == 0.5

    def test_long_query_classification(self):
        """Long queries (>100 chars) should be classified and compressed."""
        from rag.nlp.query_router import QueryType
        
        long_query = (
            "我想了解人工智能的发展历史，从早期的专家系统到现代的深度学习，"
            "包括各个阶段的关键技术和代表人物。另外，我还想知道神经网络的工作原理，"
            "以及如何选择合适的激活函数。"
        )
        
        intent = self.router.route(long_query)
        assert intent.query_type == QueryType.LONG_QUERY
        assert intent.retrieval_strategy == "compressed"
        assert len(intent.processed_query) <= len(long_query)
        
        params = self.router.get_retrieval_params(intent)
        assert "compressed_query" in params
        assert "original_query" in params
        assert params["min_match"] == 0.4

    def test_empty_query_classification(self):
        """Empty query should be UNKNOWN."""
        from rag.nlp.query_router import QueryType
        
        intent = self.router.route("")
        assert intent.query_type == QueryType.UNKNOWN
        
        intent = self.router.route("   ")
        assert intent.query_type == QueryType.UNKNOWN


class TestMultiConditionDecomposition:
    """Test multi-condition query decomposition logic."""

    def setup_method(self):
        from rag.nlp.query_router import QueryIntentRouter
        self.router = QueryIntentRouter()

    def test_decomposition_with_he_separator(self):
        """Query with '和' should decompose into sub-queries."""
        query = "分析机器学习和深度学习的区别。"
        intent = self.router.route(query)
        
        assert intent.sub_queries is not None
        assert len(intent.sub_queries) >= 1
        for sq in intent.sub_queries:
            assert isinstance(sq, str)
            assert len(sq.strip()) > 0

    def test_decomposition_with_yiji_separator(self):
        """Query with '以及' should decompose."""
        query = "解释Python的列表推导式，以及生成器表达式的用法。"
        intent = self.router.route(query)
        
        assert intent.sub_queries is not None
        assert len(intent.sub_queries) >= 1

    def test_decomposition_with_bujin_haeyao(self):
        """Query with '不仅...还要' should decompose."""
        query = "不仅要说明数据库的类型，还要解释每种类型的优缺点。"
        intent = self.router.route(query)
        
        assert intent.sub_queries is not None

    def test_simple_query_no_decomposition(self):
        """Simple factual query should have sub_queries as None or empty."""
        from rag.nlp.query_router import QueryType
        
        intent = self.router.route("什么是Python？")
        assert intent.query_type == QueryType.FACTUAL
        # Factual queries don't need decomposition


class TestRetrievalParams:
    """Test retrieval parameter generation for each query type."""

    def setup_method(self):
        from rag.nlp.query_router import QueryIntentRouter, QueryType, QueryIntent
        self.router = QueryIntentRouter()
        self.QueryType = QueryType
        self.QueryIntent = QueryIntent

    def test_factual_params_exact_retrieval(self):
        """Factual queries should use high min_match and term weight."""
        intent = self.QueryIntent(
            query_type=self.QueryType.FACTUAL,
            original_query="test",
            processed_query="test",
            retrieval_strategy="exact"
        )
        params = self.router.get_retrieval_params(intent)
        
        assert params["query_type"] == "factual"
        assert params["retrieval_strategy"] == "exact"
        assert params["min_match"] == 0.7
        assert params["vector_weight"] == 0.3
        assert params["term_weight"] == 0.7

    def test_open_ended_params_hybrid_retrieval(self):
        """Open-ended queries should use expanded topk and low min_match."""
        intent = self.QueryIntent(
            query_type=self.QueryType.OPEN_ENDED,
            original_query="test",
            processed_query="test",
            retrieval_strategy="hybrid"
        )
        params = self.router.get_retrieval_params(intent)
        
        assert params["topk_multiplier"] == 1.5
        assert params["min_match"] == 0.3
        assert params["vector_weight"] == 0.7

    def test_unknown_params_default(self):
        """Unknown queries should use default hybrid params."""
        intent = self.QueryIntent(
            query_type=self.QueryType.UNKNOWN,
            original_query="test",
            processed_query="test",
            retrieval_strategy="hybrid"
        )
        params = self.router.get_retrieval_params(intent)
        
        assert params["query_type"] == "unknown"
        assert params["min_match"] == 0.3
        assert params["vector_weight"] == 0.5
        assert params["term_weight"] == 0.5

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

import pytest
from rag.nlp.query_router import (
    QueryIntentRouter,
    QueryType,
    QueryIntent
)


class TestQueryIntentRouter:
    
    def setup_method(self):
        self.router = QueryIntentRouter()
    
    def test_factual_query_detection(self):
        factual_queries = [
            "什么是人工智能？",
            "谁是爱因斯坦？",
            "北京是中国的首都吗？",
            "地球的半径是多少？",
            "Python 中的列表和元组有什么区别？",
            "What is machine learning?",
            "Who is the president of the United States?",
            "How many planets are there in the solar system?",
        ]
        
        for query in factual_queries:
            intent = self.router.route(query)
            assert intent.query_type == QueryType.FACTUAL, f"Query '{query}' should be classified as FACTUAL, got {intent.query_type}"
            assert intent.retrieval_strategy == "exact"
    
    def test_multi_condition_query_detection(self):
        multi_condition_queries = [
            "我想了解人工智能和机器学习的区别，以及它们在实际应用中的例子。",
            "请比较Python和Java的性能，同时分析它们的适用场景。",
            "不仅要说明数据库的类型，还要解释每种类型的优缺点。",
            "一方面要考虑成本因素，另一方面也要考虑性能因素。",
            "分析机器学习和深度学习的区别，以及它们各自的应用场景。",
        ]
        
        for query in multi_condition_queries:
            intent = self.router.route(query)
            assert intent.query_type == QueryType.MULTI_CONDITION, f"Query '{query}' should be classified as MULTI_CONDITION, got {intent.query_type}"
            assert intent.retrieval_strategy == "multi_query"
            assert intent.sub_queries is not None
            assert len(intent.sub_queries) >= 1
    
    def test_open_ended_query_detection(self):
        open_ended_queries = [
            "谈谈你对人工智能未来发展的看法。",
            "如何提高编程能力？",
            "解释一下深度学习的基本概念。",
            "请分析一下当前的经济形势。",
            "什么是幸福？",
        ]
        
        for query in open_ended_queries:
            intent = self.router.route(query)
            assert intent.query_type == QueryType.OPEN_ENDED, f"Query '{query}' should be classified as OPEN_ENDED, got {intent.query_type}"
            assert intent.retrieval_strategy == "hybrid"
    
    def test_numeric_tabular_query_detection(self):
        numeric_tabular_queries = [
            "2023年公司的总营收是多少？",
            "表格中第一列的平均值是多少？",
            "排名前5的产品有哪些？",
            "价格在100到500之间的商品有多少？",
            "统计一下每个类别的数量。",
            "第3行第2列的数据是什么？",
        ]
        
        for query in numeric_tabular_queries:
            intent = self.router.route(query)
            assert intent.query_type == QueryType.NUMERIC_TABULAR, f"Query '{query}' should be classified as NUMERIC_TABULAR, got {intent.query_type}"
            assert intent.retrieval_strategy == "hybrid_with_original_chunks"
            assert intent.metadata is not None
            assert intent.metadata.get("preserve_original_chunks") == True
    
    def test_long_query_detection(self):
        long_query = "我最近在学习人工智能，特别是机器学习和深度学习领域。我想了解一下神经网络的基本原理，包括前向传播和反向传播的过程。另外，我还想知道如何选择合适的激活函数，以及不同的优化算法（如SGD、Adam、RMSprop）之间的区别。最后，我想了解一下过拟合和欠拟合的问题，以及如何通过正则化、dropout等技术来解决这些问题。"
        
        intent = self.router.route(long_query)
        assert intent.query_type == QueryType.LONG_QUERY, f"Long query should be classified as LONG_QUERY, got {intent.query_type}"
        assert intent.retrieval_strategy == "compressed"
        assert intent.processed_query is not None
        assert len(intent.processed_query) < len(long_query)
    
    def test_empty_query(self):
        intent = self.router.route("")
        assert intent.query_type == QueryType.UNKNOWN
        assert intent.original_query == ""
        assert intent.processed_query == ""
        
        intent = self.router.route("   ")
        assert intent.query_type == QueryType.UNKNOWN
    
    def test_retrieval_params_for_factual(self):
        intent = self.router.route("什么是人工智能？")
        params = self.router.get_retrieval_params(intent)
        
        assert params["query_type"] == "factual"
        assert params["retrieval_strategy"] == "exact"
        assert params["min_match"] == 0.7
        assert params["vector_weight"] == 0.3
        assert params["term_weight"] == 0.7
    
    def test_retrieval_params_for_multi_condition(self):
        intent = self.router.route("比较Python和Java的区别，以及它们的适用场景。")
        params = self.router.get_retrieval_params(intent)
        
        assert params["query_type"] == "multi_condition"
        assert params["retrieval_strategy"] == "multi_query"
        assert params["min_match"] == 0.4
        assert params["vector_weight"] == 0.5
        assert params["term_weight"] == 0.5
        assert "sub_queries" in params
        assert len(params["sub_queries"]) >= 1
    
    def test_retrieval_params_for_open_ended(self):
        intent = self.router.route("谈谈你对人工智能未来的看法。")
        params = self.router.get_retrieval_params(intent)
        
        assert params["query_type"] == "open_ended"
        assert params["retrieval_strategy"] == "hybrid"
        assert params["min_match"] == 0.3
        assert params["vector_weight"] == 0.7
        assert params["term_weight"] == 0.3
        assert params["topk_multiplier"] == 1.5
    
    def test_retrieval_params_for_numeric_tabular(self):
        intent = self.router.route("2023年的总销售额是多少？")
        params = self.router.get_retrieval_params(intent)
        
        assert params["query_type"] == "numeric_tabular"
        assert params["retrieval_strategy"] == "hybrid_with_original_chunks"
        assert params["min_match"] == 0.5
        assert params["vector_weight"] == 0.4
        assert params["term_weight"] == 0.6
        assert params["preserve_original_chunks"] == True
        assert params["highlight_numerics"] == True
    
    def test_retrieval_params_for_long_query(self):
        long_query = "我想了解人工智能的发展历史，从早期的专家系统到现代的深度学习，包括各个阶段的关键技术和代表人物。另外，我还想知道人工智能在各个领域的应用，比如医疗、金融、教育等，以及未来的发展趋势。"
        intent = self.router.route(long_query)
        params = self.router.get_retrieval_params(intent)
        
        assert params["query_type"] == "long_query"
        assert params["retrieval_strategy"] == "compressed"
        assert params["min_match"] == 0.4
        assert params["vector_weight"] == 0.6
        assert params["term_weight"] == 0.4
        assert "compressed_query" in params
        assert "original_query" in params
    
    def test_retrieval_params_for_unknown(self):
        intent = QueryIntent(
            query_type=QueryType.UNKNOWN,
            original_query="test",
            processed_query="test"
        )
        params = self.router.get_retrieval_params(intent)
        
        assert params["query_type"] == "unknown"
        assert params["retrieval_strategy"] == "hybrid"
        assert params["min_match"] == 0.3
        assert params["vector_weight"] == 0.5
        assert params["term_weight"] == 0.5
    
    def test_multi_condition_decomposition(self):
        query = "分析机器学习和深度学习的区别，以及它们各自的应用场景。"
        intent = self.router.route(query)
        
        assert intent.query_type == QueryType.MULTI_CONDITION
        assert intent.sub_queries is not None
        
        for sub_query in intent.sub_queries:
            assert isinstance(sub_query, str)
            assert len(sub_query.strip()) > 0
    
    def test_processed_query_preservation(self):
        query = "什么是人工智能？"
        intent = self.router.route(query)
        
        assert intent.original_query == query
        assert intent.processed_query == query
    
    def test_long_query_compression(self):
        long_query = "我最近在学习编程，特别是Python语言。我想了解一下Python的基本语法，包括变量、数据类型、条件语句、循环语句等。另外，我还想知道如何定义函数，以及如何使用模块和包。最后，我想了解一下面向对象编程的基本概念，比如类、对象、继承、多态等。"
        
        intent = self.router.route(long_query)
        
        assert intent.query_type == QueryType.LONG_QUERY
        assert intent.processed_query is not None
        assert len(intent.processed_query) <= len(long_query)
        
        key_phrases = ["Python", "编程", "基本语法", "函数", "面向对象"]
        found_key_phrases = any(phrase in intent.processed_query for phrase in key_phrases)
        assert found_key_phrases, f"Compressed query should retain key information. Compressed: {intent.processed_query}"
    
    def test_mixed_query_types(self):
        test_cases = [
            ("什么是机器学习？它和深度学习有什么区别？", QueryType.MULTI_CONDITION),
            ("2023年公司的营收是多少？与2022年相比增长了多少？", QueryType.MULTI_CONDITION),
            ("请解释一下神经网络的工作原理，包括前向传播和反向传播。", QueryType.MULTI_CONDITION),
        ]
        
        for query, expected_type in test_cases:
            intent = self.router.route(query)
            assert intent.query_type == expected_type, f"Query '{query}' should be {expected_type}, got {intent.query_type}"
    
    def test_query_intent_metadata(self):
        intent = QueryIntent(
            query_type=QueryType.FACTUAL,
            original_query="test",
            processed_query="test",
            metadata={"custom_key": "custom_value"}
        )
        
        assert intent.metadata is not None
        assert intent.metadata["custom_key"] == "custom_value"
    
    def test_query_intent_sub_queries(self):
        intent = QueryIntent(
            query_type=QueryType.MULTI_CONDITION,
            original_query="test1 and test2",
            processed_query="test1 and test2",
            sub_queries=["test1", "test2"]
        )
        
        assert intent.sub_queries is not None
        assert len(intent.sub_queries) == 2
        assert intent.sub_queries[0] == "test1"
        assert intent.sub_queries[1] == "test2"

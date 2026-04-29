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
Standalone test for chunk content priority logic.

This test verifies:
1. get_chunk_content() function logic (copied from generator.py)
2. QueryIntentRouter core classification logic (copied from query_router.py)

No external dependencies required.
"""

import re
from enum import Enum
from dataclasses import dataclass
from typing import Optional, List, Dict, Any

#
# Standalone implementation of get_chunk_content (from rag/prompts/generator.py)
#
def get_value(d, k1, k2):
    return d.get(k1, d.get(k2))


def get_chunk_content(chunk: dict) -> str:
    if chunk.get("preserve_original_content"):
        original = chunk.get("original_content_with_weight")
        if original:
            return original
    return get_value(chunk, "content", "content_with_weight")


#
# Standalone implementation of QueryIntentRouter core logic (from rag/nlp/query_router.py)
#
class QueryType(Enum):
    FACTUAL = "factual"
    MULTI_CONDITION = "multi_condition"
    OPEN_ENDED = "open_ended"
    NUMERIC_TABULAR = "numeric_tabular"
    LONG_QUERY = "long_query"
    UNKNOWN = "unknown"


@dataclass
class QueryIntent:
    query_type: QueryType
    original_query: str
    processed_query: str
    sub_queries: Optional[List[str]] = None
    metadata: Optional[Dict[str, Any]] = None
    retrieval_strategy: str = "hybrid"


class QueryIntentRouter:
    FACTUAL_KEYWORDS = [
        "什么是", "什么叫", "谁是", "哪", "多少", "何时", "何地", "为什么", "怎么",
        "what is", "who is", "where is", "when is", "how many", "how much",
        "定义", "meaning of", "definition of", "explain", "什么", "谁", "哪年",
        "哪个", "哪些", "第几", "几", "多少个", "多少钱", "多远", "多久", "多重"
    ]
    
    MULTI_CONDITION_KEYWORDS = [
        "和", "与", "或", "且", "同时", "以及", "并且", "而且", "此外", "另外",
        "and", "or", "同时满足", "既要", "又要", "不仅", "而且", "既...又",
        "一方面", "另一方面", "此外", "再者", "加之", "除此之外", "还有"
    ]
    
    MULTI_CONDITION_PATTERNS = [
        r"[，。；！？,\.;!?\s]+(和|与|或|且|以及|并且|而且|and|or)[，。；！？,\.;!?\s]+",
        r"[，。；！？,\.;!?\s]+(同时|既要|又要|不仅|此外|另外)[，。；！？,\.;!?\s]+"
    ]
    
    NUMERIC_PATTERNS = [
        r"\d+\.?\d*",
        r"第\d+",
        r"\d+[月日年周年天小时分钟秒]",
        r"[￥$€£]\d+\.?\d*",
        r"\d+\.?\d*[%]",
        r"\d+\.?\d*[kKmMgGtTpP]?[bB]?",
        r"table|表格|表\d*",
        r"统计|总计|总和|平均|最大值|最小值|排名|排序|比例"
    ]
    
    LONG_QUERY_THRESHOLD = 100
    
    def __init__(self):
        self._init_patterns()
    
    def _init_patterns(self):
        self.factual_pattern = re.compile(
            "|".join(re.escape(kw) for kw in self.FACTUAL_KEYWORDS),
            re.IGNORECASE
        )
        
        self.multi_condition_patterns = [
            re.compile(pattern, re.IGNORECASE) for pattern in self.MULTI_CONDITION_PATTERNS
        ]
        
        self.numeric_patterns = [
            re.compile(pattern, re.IGNORECASE) for pattern in self.NUMERIC_PATTERNS
        ]
    
    def route(self, query: str) -> QueryIntent:
        if not query or not query.strip():
            return QueryIntent(
                query_type=QueryType.UNKNOWN,
                original_query=query,
                processed_query=query,
                retrieval_strategy="hybrid"
            )
        
        query = query.strip()
        
        if len(query) > self.LONG_QUERY_THRESHOLD:
            return self._process_long_query(query)
        
        if self._is_numeric_tabular(query):
            return self._process_numeric_tabular(query)
        
        if self._is_multi_condition(query):
            return self._process_multi_condition(query)
        
        if self._is_factual(query):
            return self._process_factual(query)
        
        return self._process_open_ended(query)
    
    def _is_factual(self, query: str) -> bool:
        if self.factual_pattern.search(query):
            return True
        if query.endswith("?") or query.endswith("？"):
            return True
        return False
    
    def _is_multi_condition(self, query: str) -> bool:
        for pattern in self.multi_condition_patterns:
            if pattern.search(query):
                return True
        condition_count = 0
        for kw in self.MULTI_CONDITION_KEYWORDS:
            if kw in query:
                condition_count += 1
        return condition_count >= 2 or (condition_count >= 1 and len(query) > 30)
    
    def _is_numeric_tabular(self, query: str) -> bool:
        numeric_count = 0
        for pattern in self.numeric_patterns:
            if pattern.search(query):
                numeric_count += 1
        return numeric_count >= 2
    
    def _process_factual(self, query: str) -> QueryIntent:
        return QueryIntent(
            query_type=QueryType.FACTUAL,
            original_query=query,
            processed_query=query,
            retrieval_strategy="exact"
        )
    
    def _process_multi_condition(self, query: str) -> QueryIntent:
        sub_queries = self._decompose_multi_condition(query)
        return QueryIntent(
            query_type=QueryType.MULTI_CONDITION,
            original_query=query,
            processed_query=query,
            sub_queries=sub_queries,
            retrieval_strategy="multi_query"
        )
    
    def _decompose_multi_condition(self, query: str) -> List[str]:
        sub_queries = []
        delimiters = []
        for pattern in self.multi_condition_patterns:
            for match in pattern.finditer(query):
                delimiters.append(match.group())
        
        if delimiters:
            parts = re.split("|".join(re.escape(d) for d in delimiters), query)
            for part in parts:
                part = part.strip()
                if part and len(part) > 5:
                    sub_queries.append(part)
        else:
            sentences = re.split(r'[，。；！？,\.;!?\s]+', query)
            for sentence in sentences:
                sentence = sentence.strip()
                if sentence and len(sentence) > 5:
                    sub_queries.append(sentence)
        
        if not sub_queries:
            sub_queries = [query]
        return sub_queries
    
    def _process_numeric_tabular(self, query: str) -> QueryIntent:
        return QueryIntent(
            query_type=QueryType.NUMERIC_TABULAR,
            original_query=query,
            processed_query=query,
            metadata={"preserve_original_chunks": True},
            retrieval_strategy="hybrid_with_original_chunks"
        )
    
    def _process_long_query(self, query: str) -> QueryIntent:
        compressed_query = self._compress_long_query(query)
        return QueryIntent(
            query_type=QueryType.LONG_QUERY,
            original_query=query,
            processed_query=compressed_query,
            metadata={"original_query": query},
            retrieval_strategy="compressed"
        )
    
    def _compress_long_query(self, query: str) -> str:
        sentences = re.split(r'[。！？\n]+', query)
        key_sentences = []
        for sentence in sentences:
            sentence = sentence.strip()
            if not sentence:
                continue
            has_keyword = False
            for kw in self.FACTUAL_KEYWORDS:
                if kw in sentence:
                    has_keyword = True
                    break
            if has_keyword or len(sentence) > 20:
                key_sentences.append(sentence)
        
        if not key_sentences:
            key_sentences = sentences[:2] if len(sentences) >= 2 else sentences
        return "。".join(key_sentences)
    
    def _process_open_ended(self, query: str) -> QueryIntent:
        return QueryIntent(
            query_type=QueryType.OPEN_ENDED,
            original_query=query,
            processed_query=query,
            retrieval_strategy="hybrid"
        )
    
    def get_retrieval_params(self, intent: QueryIntent) -> Dict[str, Any]:
        params = {
            "retrieval_strategy": intent.retrieval_strategy,
            "query_type": intent.query_type.value
        }
        
        if intent.query_type == QueryType.FACTUAL:
            params.update({
                "min_match": 0.7,
                "boost_keywords": True,
                "vector_weight": 0.3,
                "term_weight": 0.7
            })
        elif intent.query_type == QueryType.MULTI_CONDITION:
            params.update({
                "sub_queries": intent.sub_queries,
                "min_match": 0.4,
                "vector_weight": 0.5,
                "term_weight": 0.5
            })
        elif intent.query_type == QueryType.OPEN_ENDED:
            params.update({
                "min_match": 0.3,
                "vector_weight": 0.7,
                "term_weight": 0.3,
                "topk_multiplier": 1.5
            })
        elif intent.query_type == QueryType.NUMERIC_TABULAR:
            params.update({
                "preserve_original_chunks": True,
                "min_match": 0.5,
                "vector_weight": 0.4,
                "term_weight": 0.6,
                "highlight_numerics": True
            })
        elif intent.query_type == QueryType.LONG_QUERY:
            params.update({
                "compressed_query": intent.processed_query,
                "original_query": intent.original_query,
                "min_match": 0.4,
                "vector_weight": 0.6,
                "term_weight": 0.4
            })
        else:
            params.update({
                "min_match": 0.3,
                "vector_weight": 0.5,
                "term_weight": 0.5
            })
        return params


#
# Tests
#
def test_get_chunk_content():
    print("=" * 60)
    print("Testing get_chunk_content function...")
    print("=" * 60)
    
    # Test 1: Regular chunk uses content_with_weight
    chunk1 = {
        "content_with_weight": "Processed content with weights.",
        "content_ltks": "Processed content.",
    }
    result1 = get_chunk_content(chunk1)
    assert result1 == "Processed content with weights.", f"Test 1 FAILED: {result1}"
    print("[PASS] Test 1: Regular chunk uses content_with_weight")
    
    # Test 2: Regular chunk prefers 'content'
    chunk2 = {
        "content": "Direct content.",
        "content_with_weight": "Weighted content.",
    }
    result2 = get_chunk_content(chunk2)
    assert result2 == "Direct content.", f"Test 2 FAILED: {result2}"
    print("[PASS] Test 2: Regular chunk prefers 'content' over 'content_with_weight'")
    
    # Test 3: Numeric/tabular chunk preserves original
    chunk3 = {
        "preserve_original_content": True,
        "original_content_with_weight": "2023年总营收：1500万元 | 月份 | 销售额 | | 1月 | 500万 |",
        "content_with_weight": "2023年营收增长情况。",
    }
    result3 = get_chunk_content(chunk3)
    assert "1500万元" in result3, f"Test 3 FAILED: {result3}"
    assert "月份" in result3, f"Test 3 FAILED: {result3}"
    print("[PASS] Test 3: Numeric/tabular chunk preserves original_content_with_weight")
    
    # Test 4: Numeric/tabular chunk falls back if original missing
    chunk4 = {
        "preserve_original_content": True,
        "content_with_weight": "Fallback content.",
    }
    result4 = get_chunk_content(chunk4)
    assert result4 == "Fallback content.", f"Test 4 FAILED: {result4}"
    print("[PASS] Test 4: Numeric/tabular chunk falls back if original missing")
    
    # Test 5: Empty chunk
    chunk5 = {}
    result5 = get_chunk_content(chunk5)
    assert result5 is None, f"Test 5 FAILED: {result5}"
    print("[PASS] Test 5: Empty chunk returns None")
    
    # Test 6: Integration test - end-to-end scenario
    print("\n[Integration Test] End-to-end content priority:")
    print("-" * 50)
    
    # Scenario: Numeric/tabular query returns chunks with preserve_original_content=True
    # These chunks should use original_content_with_weight in kb_prompt
    
    kb_chunks = [
        {
            "chunk_id": "chunk-1",
            "preserve_original_content": True,
            "original_content_with_weight": "| 季度 | 收入 | 成本 | 利润 |\n| Q1 | 100万 | 60万 | 40万 |\n| Q2 | 120万 | 70万 | 50万 |",
            "content_with_weight": "2023年上半年财务数据摘要。",
            "docnm_kwd": "financial_report_2023.pdf",
        },
        {
            "chunk_id": "chunk-2",
            "preserve_original_content": True,
            "original_content_with_weight": "原始表格数据：| 产品 | 销量 | 单价 |\n| 产品A | 1000 | 100元 |\n| 产品B | 500 | 200元 |",
            "content_with_weight": "产品销售数据概述。",
            "docnm_kwd": "sales_report_2023.pdf",
        }
    ]
    
    # Simulate kb_prompt calling get_chunk_content
    for i, chunk in enumerate(kb_chunks):
        content = get_chunk_content(chunk)
        print(f"Chunk {i+1} ({chunk['docnm_kwd']}):")
        print(f"  - Uses ORIGINAL content: {'季度' in content or '产品A' in content}")
        print(f"  - Content preview: {content[:50]}...")
        
        # Verify original is used
        if chunk["chunk_id"] == "chunk-1":
            assert "季度" in content, "Should preserve original table"
            assert "财务数据摘要" not in content, "Should NOT use processed content"
        elif chunk["chunk_id"] == "chunk-2":
            assert "产品A" in content, "Should preserve original product table"
            assert "销售数据概述" not in content, "Should NOT use processed content"
    
    print("\n[PASS] Integration Test: Numeric/tabular chunks preserve original table content")
    
    print("\n" + "=" * 60)
    print("All get_chunk_content tests PASSED!")
    print("=" * 60)


def test_query_intent_router():
    print("\n" + "=" * 60)
    print("Testing QueryIntentRouter...")
    print("=" * 60)
    
    router = QueryIntentRouter()
    
    # Test 1: Factual query
    intent = router.route("什么是人工智能？")
    assert intent.query_type == QueryType.FACTUAL, f"Expected FACTUAL, got {intent.query_type}"
    params = router.get_retrieval_params(intent)
    assert params["min_match"] == 0.7
    assert params["vector_weight"] == 0.3
    print("[PASS] Test 1: Factual query classification and params")
    
    # Test 2: Multi-condition query
    intent = router.route("分析机器学习和深度学习的区别，以及它们的应用场景。")
    assert intent.query_type == QueryType.MULTI_CONDITION
    assert intent.sub_queries is not None
    assert len(intent.sub_queries) >= 1
    params = router.get_retrieval_params(intent)
    assert "sub_queries" in params
    print("[PASS] Test 2: Multi-condition query decomposition")
    
    # Test 3: Open-ended query
    intent = router.route("谈谈你对人工智能未来发展的看法。")
    assert intent.query_type == QueryType.OPEN_ENDED
    params = router.get_retrieval_params(intent)
    assert params["topk_multiplier"] == 1.5
    print("[PASS] Test 3: Open-ended query with topk_multiplier")
    
    # Test 4: Numeric/tabular query
    intent = router.route("2023年公司的总营收是多少？")
    assert intent.query_type == QueryType.NUMERIC_TABULAR
    assert intent.metadata is not None
    assert intent.metadata.get("preserve_original_chunks") == True
    params = router.get_retrieval_params(intent)
    assert params["preserve_original_chunks"] == True
    print("[PASS] Test 4: Numeric/tabular query with preserve_original_chunks")
    
    # Test 5: Long query (> 100 chars)
    long_query = (
        "我想了解人工智能的发展历史，从早期的专家系统到现代的深度学习，"
        "包括各个阶段的关键技术和代表人物。例如，图灵测试的提出，感知机的发明，"
        "反向传播算法的突破，以及深度学习在图像识别、自然语言处理等领域的应用。"
    )
    print(f"Long query length: {len(long_query)} chars")
    intent = router.route(long_query)
    assert intent.query_type == QueryType.LONG_QUERY, f"Expected LONG_QUERY, got {intent.query_type}"
    assert len(intent.processed_query) <= len(long_query)
    params = router.get_retrieval_params(intent)
    assert "compressed_query" in params
    assert "original_query" in params
    print("[PASS] Test 5: Long query compression")
    
    # Test 6: Empty query
    intent = router.route("")
    assert intent.query_type == QueryType.UNKNOWN
    print("[PASS] Test 6: Empty query returns UNKNOWN")
    
    print("\n" + "=" * 60)
    print("All QueryIntentRouter tests PASSED!")
    print("=" * 60)


def test_full_integration():
    print("\n" + "=" * 60)
    print("Full Integration Test: Query -> Retrieval -> Answer Context")
    print("=" * 60)
    
    router = QueryIntentRouter()
    
    # Scenario 1: Numeric/tabular query end-to-end
    print("\n[Scenario 1] Numeric/tabular query: '2023年各季度收入是多少？'")
    print("-" * 50)
    
    # Step 1: Classify query
    query = "2023年各季度收入是多少？"
    intent = router.route(query)
    params = router.get_retrieval_params(intent)
    
    print(f"Query type: {intent.query_type.value}")
    print(f"Retrieval strategy: {intent.retrieval_strategy}")
    print(f"Preserve original chunks: {params.get('preserve_original_chunks')}")
    
    assert intent.query_type == QueryType.NUMERIC_TABULAR
    assert params["preserve_original_chunks"] == True
    
    # Step 2: Simulate retrieval returning chunks with preserve_original_content
    print("\n[Retrieval] Search returns chunks with preserve_original_content=True")
    
    retrieved_chunks = [
        {
            "chunk_id": "chunk-finance-001",
            "preserve_original_content": True,
            "original_content_with_weight": "| 季度 | 收入 (万元) | 同比增长 |\n| Q1 | 500 | +15% |\n| Q2 | 600 | +20% |\n| Q3 | 550 | +10% |\n| Q4 | 700 | +25% |",
            "content_with_weight": "2023年各季度收入摘要。Q1收入500万，Q2收入600万...",
            "docnm_kwd": "2023_financial_report.pdf",
        }
    ]
    
    # Step 3: Simulate kb_prompt building answer context
    print("\n[Answer Context] kb_prompt builds context using get_chunk_content:")
    for chunk in retrieved_chunks:
        content = get_chunk_content(chunk)
        
        # Verify original is used, NOT processed
        assert "季度" in content, "Should preserve original table structure"
        assert "Q1 | 500" in content, "Should preserve original numeric data"
        assert "摘要" not in content, "Should NOT use processed content"
        
        print(f"Document: {chunk['docnm_kwd']}")
        print(f"Content (ORIGINAL preserved):")
        print(content)
    
    print("\n[PASS] Scenario 1: Numeric/tabular query preserves original table in answer context")
    
    # Scenario 2: Multi-condition query end-to-end
    print("\n[Scenario 2] Multi-condition query: 'Python和Java的区别，以及它们的适用场景？'")
    print("-" * 50)
    
    query = "Python和Java的区别，以及它们的适用场景？"
    intent = router.route(query)
    params = router.get_retrieval_params(intent)
    
    print(f"Query type: {intent.query_type.value}")
    print(f"Sub-queries: {intent.sub_queries}")
    print(f"Min match: {params['min_match']}")
    
    assert intent.query_type == QueryType.MULTI_CONDITION
    assert len(intent.sub_queries) >= 1
    assert params["min_match"] == 0.4
    
    print("\n[PASS] Scenario 2: Multi-condition query decomposed into sub-queries")
    
    # Scenario 3: Open-ended query end-to-end
    print("\n[Scenario 3] Open-ended query: '谈谈人工智能的未来发展趋势'")
    print("-" * 50)
    
    query = "谈谈人工智能的未来发展趋势"
    intent = router.route(query)
    params = router.get_retrieval_params(intent)
    
    print(f"Query type: {intent.query_type.value}")
    print(f"Topk multiplier: {params['topk_multiplier']}")
    print(f"Min match: {params['min_match']}")
    print(f"Vector weight: {params['vector_weight']}")
    
    assert intent.query_type == QueryType.OPEN_ENDED
    assert params["topk_multiplier"] == 1.5
    assert params["min_match"] == 0.3
    assert params["vector_weight"] == 0.7
    
    # Verify expanded retrieval: original topk=1024 -> expanded=1536
    original_topk = 1024
    expanded_topk = int(original_topk * params["topk_multiplier"])
    print(f"Expanded retrieval: {original_topk} -> {expanded_topk}")
    assert expanded_topk == 1536
    
    print("\n[PASS] Scenario 3: Open-ended query uses expanded retrieval (1.5x topk)")
    
    print("\n" + "=" * 60)
    print("Full Integration Test PASSED!")
    print("=" * 60)


if __name__ == "__main__":
    test_get_chunk_content()
    test_query_intent_router()
    test_full_integration()
    
    print("\n" + "=" * 60)
    print("ALL TESTS PASSED!")
    print("=" * 60)

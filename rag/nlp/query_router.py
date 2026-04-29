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

import re
import logging
from enum import Enum
from dataclasses import dataclass
from typing import Optional, List, Dict, Any


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
    
    def __init__(self, llm_bundle=None, enable_llm_classification: bool = False):
        self.llm_bundle = llm_bundle
        self.enable_llm_classification = enable_llm_classification
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
        if self.llm_bundle and self.enable_llm_classification:
            try:
                return self._compress_with_llm(query)
            except Exception as e:
                logging.warning(f"LLM compression failed: {e}")
        
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
    
    def _compress_with_llm(self, query: str) -> str:
        system_prompt = "请将以下长查询压缩为简洁的核心问题，保留关键信息和查询意图。"
        
        if self.llm_bundle:
            try:
                compressed, _ = self.llm_bundle.chat(
                    system=system_prompt,
                    history=[{"role": "user", "content": f"请压缩以下查询：\n{query}"}],
                    gen_conf={"temperature": 0.1, "max_tokens": 200}
                )
                return compressed if compressed else query
            except Exception as e:
                logging.warning(f"LLM compression failed: {e}")
        
        return query
    
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

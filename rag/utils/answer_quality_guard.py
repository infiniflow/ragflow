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
Answer Quality Guard - 轻量级的 RAG 回答质量保障模块

功能：
1. 检索后低相关 chunk 过滤
2. 长上下文智能压缩（保留关键证据）
3. 证据充足性检查
4. 引用验证和标准化
"""

import logging
import re
from dataclasses import dataclass, field
from typing import Dict, List, Optional, Tuple, Set, Any
from copy import deepcopy


def _estimate_tokens(text: str) -> int:
    """
    估算文本的 token 数量（不依赖 tiktoken 的轻量级实现）
    
    对于英文：大约 4 字符 = 1 token
    对于中文：大约 2 字符 = 1 token
    """
    if not text:
        return 0
    
    chinese_chars = len(re.findall(r'[\u4e00-\u9fff]', text))
    other_chars = len(text) - chinese_chars
    
    estimated = int(other_chars / 4) + int(chinese_chars / 2)
    return max(1, estimated)


try:
    from common.token_utils import num_tokens_from_string
except (ImportError, ModuleNotFoundError):
    num_tokens_from_string = _estimate_tokens

CITATION_PATTERN = re.compile(r"\[(?:ID:)?([0-9\u0660-\u0669\u06F0-\u06F9]+)\]")
KEY_ENTITY_PATTERNS = [
    re.compile(r"\d+(?:\.\d+)?%"),
    re.compile(r"\d+(?:\.\d+)?(?:\s*[KkMmBbTtGg]?[Bb]?|人民币|美元|欧元|日元|英镑|元|美元|亿元|万美元)"),
    re.compile(r"\d{4}[-/年]\d{1,2}[-/月]\d{1,2}"),
    re.compile(r"\d{4}年"),
]


@dataclass
class GuardResult:
    filtered_chunks: List[Dict] = field(default_factory=list)
    compressed_context: str = ""
    is_sufficient: bool = True
    missing_info: List[str] = field(default_factory=list)
    reasoning: str = ""
    filtered_count: int = 0
    original_chunk_count: int = 0
    warnings: List[str] = field(default_factory=list)


@dataclass
class CitationValidationResult:
    is_valid: bool = True
    validated_answer: str = ""
    invalid_citations: List[int] = field(default_factory=list)
    validated_citations: Set[int] = field(default_factory=set)
    chunk_mapping: Dict[int, Dict] = field(default_factory=dict)
    warnings: List[str] = field(default_factory=list)


class AnswerQualityGuard:
    """
    轻量级的 RAG 回答质量保障模块

    在检索后、生成前、生成后三个阶段提供质量保障：
    1. 检索后：过滤低相关 chunk，压缩长上下文
    2. 生成前：检查证据充足性
    3. 生成后：验证引用，标准化引用格式
    """

    def __init__(
        self,
        similarity_threshold: float = 0.3,
        min_keyword_overlap: int = 1,
        max_context_tokens: int = 4000,
        enable_evidential_check: bool = True,
        enable_citation_validation: bool = True,
    ):
        self.similarity_threshold = similarity_threshold
        self.min_keyword_overlap = min_keyword_overlap
        self.max_context_tokens = max_context_tokens
        self.enable_evidential_check = enable_evidential_check
        self.enable_citation_validation = enable_citation_validation
        self.logger = logging.getLogger(__name__)

    def filter_low_relevant_chunks(
        self,
        chunks: List[Dict],
        question: str,
        keywords: Optional[List[str]] = None,
    ) -> Tuple[List[Dict], int]:
        """
        过滤低相关的 chunks

        过滤规则：
        1. 相似度低于阈值的 chunk
        2. 与问题关键词无重叠的 chunk（可选）
        3. 重复或几乎相同的 chunk

        Args:
            chunks: 检索到的 chunk 列表
            question: 用户问题
            keywords: 问题关键词列表

        Returns:
            (过滤后的 chunks, 过滤掉的数量)
        """
        if not chunks:
            return [], 0

        original_count = len(chunks)
        filtered_chunks = []
        seen_contents = set()

        question_lower = question.lower()
        question_words = set(re.findall(r"\w+", question_lower))

        for chunk in chunks:
            has_similarity = "similarity" in chunk
            similarity = chunk.get("similarity", 1.0)

            if has_similarity and similarity < self.similarity_threshold:
                self.logger.debug(
                    f"Filtering chunk with low similarity: {similarity:.3f} < {self.similarity_threshold}"
                )
                continue

            content = chunk.get("content_with_weight", "") or chunk.get(
                "content_ltks", ""
            )
            content_lower = content.lower()

            content_hash = hash(content_lower[:500])
            if content_hash in seen_contents:
                self.logger.debug("Filtering duplicate chunk")
                continue
            seen_contents.add(content_hash)

            if keywords:
                keyword_overlap = sum(
                    1 for kw in keywords if kw.lower() in content_lower
                )
                if keyword_overlap < self.min_keyword_overlap:
                    self.logger.debug(
                        f"Filtering chunk with low keyword overlap: {keyword_overlap} < {self.min_keyword_overlap}"
                    )
                    continue

            filtered_chunks.append(chunk)

        filtered_count = original_count - len(filtered_chunks)
        self.logger.info(
            f"Filtered {filtered_count} low-relevant chunks out of {original_count}"
        )

        return filtered_chunks, filtered_count

    def compress_context(
        self,
        chunks: List[Dict],
        max_tokens: Optional[int] = None,
        preserve_key_evidence: bool = True,
    ) -> Tuple[str, List[Dict]]:
        """
        智能压缩长上下文，保留关键证据

        压缩策略：
        1. 按相似度排序，优先保留高相似度 chunk
        2. 对于长 chunk，提取包含关键信息的句子
        3. 保留包含数字、日期、专有名词等关键证据的内容

        Args:
            chunks: chunk 列表
            max_tokens: 最大 token 数
            preserve_key_evidence: 是否保留关键证据

        Returns:
            (压缩后的上下文字符串, 保留的 chunks 列表)
        """
        if not chunks:
            return "", []

        max_tokens = max_tokens or self.max_context_tokens
        sorted_chunks = sorted(
            chunks, key=lambda x: x.get("similarity", 0.0), reverse=True
        )

        preserved_chunks = []
        total_tokens = 0
        compressed_parts = []

        for idx, chunk in enumerate(sorted_chunks):
            content = chunk.get("content_with_weight", "") or chunk.get(
                "content_ltks", ""
            )
            doc_name = chunk.get("docnm_kwd", "Unknown")
            chunk_id = chunk.get("chunk_id", f"chunk_{idx}")

            content_tokens = num_tokens_from_string(content)

            if total_tokens + content_tokens <= max_tokens:
                compressed_parts.append(
                    f"\nID: {idx}\n├── Title: {doc_name}\n└── Content:\n{content}"
                )
                preserved_chunks.append(chunk)
                total_tokens += content_tokens
                continue

            if preserve_key_evidence and content_tokens > 100:
                key_sentences = self._extract_key_sentences(content)
                if key_sentences:
                    compressed_content = " ".join(key_sentences)
                    compressed_tokens = num_tokens_from_string(compressed_content)

                    if total_tokens + compressed_tokens <= max_tokens:
                        compressed_parts.append(
                            f"\nID: {idx}\n├── Title: {doc_name}\n└── Content (compressed):\n{compressed_content}"
                        )
                        compressed_chunk = deepcopy(chunk)
                        compressed_chunk["_original_content"] = content
                        compressed_chunk["content_with_weight"] = compressed_content
                        preserved_chunks.append(compressed_chunk)
                        total_tokens += compressed_tokens
                        self.logger.debug(
                            f"Compressed chunk {idx}: {content_tokens} -> {compressed_tokens} tokens"
                        )

            if total_tokens >= max_tokens * 0.95:
                break

        compressed_context = "\n------\n".join(compressed_parts)
        self.logger.info(
            f"Compressed context: {total_tokens} tokens, {len(preserved_chunks)} chunks preserved"
        )

        return compressed_context, preserved_chunks

    def _extract_key_sentences(self, content: str) -> List[str]:
        """
        从内容中提取包含关键证据的句子

        关键证据包括：
        1. 数字、百分比、货币值
        2. 日期、时间
        3. 专有名词（大写开头的连续词）
        """
        sentences = re.split(r"(?<=[。！？.!?])\s*", content)
        key_sentences = []

        for sentence in sentences:
            sentence = sentence.strip()
            if not sentence:
                continue

            has_key_evidence = False

            for pattern in KEY_ENTITY_PATTERNS:
                if pattern.search(sentence):
                    has_key_evidence = True
                    break

            if not has_key_evidence:
                proper_nouns = re.findall(r"[A-Z][a-z]+(?:\s+[A-Z][a-z]+)*", sentence)
                if len(proper_nouns) >= 2:
                    has_key_evidence = True

            if has_key_evidence:
                key_sentences.append(sentence)

        return key_sentences

    def check_evidence_sufficiency(
        self,
        question: str,
        chunks: List[Dict],
        chat_mdl=None,
    ) -> Tuple[bool, List[str], str]:
        """
        检查检索到的内容是否足够回答用户问题

        检查策略：
        1. 快速检查：基于关键词匹配的轻量级检查
        2. LLM 检查（如果提供 chat_mdl）：更智能的充足性判断

        Args:
            question: 用户问题
            chunks: 检索到的 chunks
            chat_mdl: 聊天模型实例（可选）

        Returns:
            (是否充足, 缺失信息列表, 判断理由)
        """
        if not chunks:
            return (
                False,
                ["No relevant documents found for the question"],
                "No chunks retrieved",
            )

        question_lower = question.lower()
        all_content = " ".join(
            [
                chunk.get("content_with_weight", "") or chunk.get("content_ltks", "")
                for chunk in chunks
            ]
        ).lower()

        question_types = {
            "what": ["什么是", "是什么", "what is", "what are"],
            "how": ["如何", "怎么", "how to", "how do"],
            "why": ["为什么", "为何", "why"],
            "when": ["什么时候", "何时", "when"],
            "where": ["哪里", "在哪", "where"],
            "who": ["谁", "who"],
            "how_much": ["多少", "多少钱", "多少数量", "how much", "how many"],
            "comparison": ["比较", "对比", "区别", "差异", "compare", "difference"],
        }

        detected_types = []
        for qtype, keywords in question_types.items():
            for kw in keywords:
                if kw in question_lower:
                    detected_types.append(qtype)
                    break

        question_words = set(re.findall(r"\w+", question_lower))
        content_words = set(re.findall(r"\w+", all_content))
        word_overlap = len(question_words & content_words)
        word_overlap_ratio = (
            word_overlap / len(question_words) if question_words else 0
        )

        missing_info = []
        reasoning_parts = []

        if word_overlap_ratio < 0.3:
            missing_info.append(
                f"Low keyword overlap ({word_overlap_ratio:.1%}) between question and retrieved content"
            )
            reasoning_parts.append(
                f"Keyword overlap is only {word_overlap_ratio:.1%}, below the 30% threshold"
            )

        has_numbers = bool(re.search(r"\d+", question_lower))
        if has_numbers and not re.search(r"\d+", all_content):
            missing_info.append(
                "Question contains numbers but retrieved content has no numerical data"
            )
            reasoning_parts.append(
                "Question asks for numerical information but no numbers found in retrieved chunks"
            )

        if not missing_info:
            reasoning_parts.append(
                f"Retrieved {len(chunks)} chunks with {word_overlap_ratio:.1%} keyword overlap"
            )
            return True, [], " ".join(reasoning_parts)

        return False, missing_info, " ".join(reasoning_parts)

    def validate_citations(
        self,
        answer: str,
        chunks: List[Dict],
        strict_mode: bool = False,
    ) -> CitationValidationResult:
        """
        验证回答中的引用是否对应到原始 chunk

        验证逻辑：
        1. 提取回答中的所有引用标记 [ID:i]
        2. 检查引用的索引是否在有效范围内
        3. （可选）检查引用的内容是否真的支持回答中的相关陈述

        Args:
            answer: 包含引用的回答
            chunks: 原始 chunk 列表
            strict_mode: 是否启用严格模式（检查内容相关性）

        Returns:
            CitationValidationResult: 包含验证结果
        """
        result = CitationValidationResult()
        result.chunk_mapping = {i: chunk for i, chunk in enumerate(chunks)}

        citations = CITATION_PATTERN.findall(answer)
        if not citations:
            result.is_valid = True
            result.validated_answer = answer
            return result

        invalid_indices = []
        valid_indices = set()
        chunk_count = len(chunks)

        for citation in citations:
            try:
                idx = int(citation)
                if 0 <= idx < chunk_count:
                    valid_indices.add(idx)
                else:
                    invalid_indices.append(idx)
                    result.warnings.append(
                        f"Citation [ID:{idx}] is out of range (valid range: 0-{chunk_count-1})"
                    )
            except ValueError:
                result.warnings.append(f"Invalid citation format: {citation}")

        result.validated_citations = valid_indices
        result.invalid_citations = invalid_indices

        if invalid_indices:
            result.is_valid = False
            result.warnings.append(
                f"Found {len(invalid_indices)} invalid citations: {invalid_indices}"
            )

        def replace_citation(match):
            idx_str = match.group(1)
            try:
                idx = int(idx_str)
                if idx in valid_indices:
                    return f"[ID:{idx}]"
                else:
                    result.warnings.append(
                        f"Removing invalid citation [ID:{idx_str}]"
                    )
                    return ""
            except ValueError:
                return match.group(0)

        result.validated_answer = CITATION_PATTERN.sub(replace_citation, answer)
        result.validated_answer = re.sub(r"\s+", " ", result.validated_answer).strip()

        return result

    def standardize_citations(
        self,
        answer: str,
        chunks: List[Dict],
        include_source_metadata: bool = True,
    ) -> Tuple[str, Dict]:
        """
        标准化引用格式，可选包含源文档元数据

        Args:
            answer: 原始回答
            chunks: chunk 列表
            include_source_metadata: 是否在引用中包含元数据

        Returns:
            (标准化后的回答, 引用映射字典)
        """
        citations = CITATION_PATTERN.findall(answer)
        if not citations:
            return answer, {}

        citation_map = {}
        seen_indices = set()

        for citation in citations:
            try:
                idx = int(citation)
                if 0 <= idx < len(chunks) and idx not in seen_indices:
                    chunk = chunks[idx]
                    citation_map[idx] = {
                        "chunk_id": chunk.get("chunk_id", ""),
                        "document_id": chunk.get("doc_id", ""),
                        "document_name": chunk.get("docnm_kwd", "Unknown"),
                        "dataset_id": chunk.get("kb_id", ""),
                        "similarity": chunk.get("similarity", 0.0),
                        "content_preview": (
                            chunk.get("content_with_weight", "")[:200]
                            if chunk.get("content_with_weight")
                            else ""
                        ),
                    }
                    seen_indices.add(idx)
            except ValueError:
                continue

        return answer, citation_map

    def process_retrieval_results(
        self,
        kbinfos: Dict,
        question: str,
        keywords: Optional[List[str]] = None,
        max_tokens: Optional[int] = None,
    ) -> GuardResult:
        """
        处理检索结果的一站式方法

        执行：
        1. 过滤低相关 chunks
        2. 压缩长上下文
        3. 检查证据充足性

        Args:
            kbinfos: 检索结果字典（包含 chunks 字段）
            question: 用户问题
            keywords: 问题关键词
            max_tokens: 最大 token 数

        Returns:
            GuardResult: 处理结果
        """
        result = GuardResult()
        result.original_chunk_count = len(kbinfos.get("chunks", []))

        chunks = kbinfos.get("chunks", [])
        if not chunks:
            result.is_sufficient = False
            result.missing_info = ["No chunks retrieved"]
            result.reasoning = "The retrieval returned no relevant chunks"
            return result

        filtered_chunks, filtered_count = self.filter_low_relevant_chunks(
            chunks, question, keywords
        )
        result.filtered_chunks = filtered_chunks
        result.filtered_count = filtered_count

        if not filtered_chunks:
            result.is_sufficient = False
            result.missing_info = ["All chunks were filtered out as low-relevant"]
            result.reasoning = f"All {result.original_chunk_count} chunks failed relevance filtering"
            return result

        compressed_context, preserved_chunks = self.compress_context(
            filtered_chunks, max_tokens
        )
        result.compressed_context = compressed_context
        result.filtered_chunks = preserved_chunks

        if self.enable_evidential_check:
            is_sufficient, missing_info, reasoning = self.check_evidence_sufficiency(
                question, preserved_chunks
            )
            result.is_sufficient = is_sufficient
            result.missing_info = missing_info
            result.reasoning = reasoning

        return result

    def generate_insufficient_response(
        self,
        question: str,
        missing_info: List[str],
        language: str = "zh",
    ) -> str:
        """
        当证据不足时生成标准的拒绝回答

        Args:
            question: 用户问题
            missing_info: 缺失的信息列表
            language: 语言（zh/en）

        Returns:
            标准拒绝回答
        """
        if language == "zh":
            response = "抱歉，根据当前知识库中的信息，我无法准确回答您的问题。\n\n"
            response += "原因分析：\n"
            for info in missing_info:
                response += f"- {info}\n"
            response += "\n建议：\n"
            response += "- 尝试使用不同的关键词提问\n"
            response += "- 检查是否需要添加更多相关文档到知识库\n"
            response += "- 简化问题或提供更多上下文信息"
        else:
            response = "I apologize, but I cannot accurately answer your question based on the information in the current knowledge base.\n\n"
            response += "Reasoning:\n"
            for info in missing_info:
                response += f"- {info}\n"
            response += "\nSuggestions:\n"
            response += "- Try asking with different keywords\n"
            response += "- Check if more relevant documents need to be added to the knowledge base\n"
            response += "- Simplify your question or provide more context"

        return response

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
Unit tests for Answer Quality Guard (AQG) module.

Covers:
- 有证据正常回答: Normal answer with sufficient evidence
- 证据不足拒绝编造: Reject hallucination when evidence is insufficient
- 引用能对上原始 chunk: Citations correctly map to original chunks
- 低相关内容被过滤: Low-relevance chunks are filtered out
- 长上下文压缩后仍保留关键证据: Key evidence preserved after context compression
"""

import re
import sys
import types
import warnings
from copy import deepcopy
from types import SimpleNamespace

import pytest

warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)


def _install_cv2_stub_if_unavailable():
    try:
        import cv2  # noqa: F401
        return
    except Exception:
        pass
    stub = types.ModuleType("cv2")
    stub.INTER_LINEAR = 1
    stub.INTER_CUBIC = 2
    stub.BORDER_CONSTANT = 0
    stub.BORDER_REPLICATE = 1
    stub.COLOR_BGR2RGB = 0
    stub.COLOR_BGR2GRAY = 1
    stub.COLOR_GRAY2BGR = 2
    stub.IMREAD_IGNORE_ORIENTATION = 128
    stub.IMREAD_COLOR = 1
    stub.RETR_LIST = 1
    stub.CHAIN_APPROX_SIMPLE = 2

    def _module_getattr(name):
        if name.isupper():
            return 0
        raise RuntimeError(f"cv2.{name} is unavailable in this test environment")

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub


_install_cv2_stub_if_unavailable()

from rag.utils.answer_quality_guard import AnswerQualityGuard, GuardResult, CitationValidationResult


# ---------------------------------------------------------------------------
# Test fixtures and helpers
# ---------------------------------------------------------------------------

_KBINFOS_WITH_EVIDENCE = {
    "chunks": [
        {
            "doc_id": "doc-1",
            "content_ltks": "ragflow rag engine document understanding",
            "content_with_weight": "RAGFlow is a RAG engine built for document understanding.",
            "vector": [0.1, 0.2, 0.3],
            "docnm_kwd": "intro.pdf",
            "similarity": 0.85,
        },
        {
            "doc_id": "doc-2",
            "content_ltks": "ragflow supports multiple document formats pdf word excel",
            "content_with_weight": "RAGFlow supports multiple document formats including PDF, Word, and Excel.",
            "vector": [0.15, 0.25, 0.35],
            "docnm_kwd": "features.pdf",
            "similarity": 0.78,
        },
    ],
    "doc_aggs": [
        {"doc_id": "doc-1", "doc_name": "intro.pdf", "count": 1},
        {"doc_id": "doc-2", "doc_name": "features.pdf", "count": 1},
    ],
    "total": 2,
}

_KBINFOS_LOW_RELEVANCE = {
    "chunks": [
        {
            "doc_id": "doc-1",
            "content_ltks": "ragflow rag engine document understanding",
            "content_with_weight": "RAGFlow is a RAG engine built for document understanding.",
            "vector": [0.1, 0.2, 0.3],
            "docnm_kwd": "intro.pdf",
            "similarity": 0.85,
        },
        {
            "doc_id": "doc-3",
            "content_ltks": "weather forecast temperature humidity",
            "content_with_weight": "Today's weather forecast: sunny with temperature around 25 degrees Celsius.",
            "vector": [0.9, 0.8, 0.7],
            "docnm_kwd": "weather.pdf",
            "similarity": 0.05,
        },
        {
            "doc_id": "doc-4",
            "content_ltks": "cooking recipe pasta tomato",
            "content_with_weight": "Pasta recipe: cook pasta for 10 minutes, add tomato sauce.",
            "vector": [0.9, 0.8, 0.7],
            "docnm_kwd": "cooking.pdf",
            "similarity": 0.08,
        },
    ],
    "doc_aggs": [
        {"doc_id": "doc-1", "doc_name": "intro.pdf", "count": 1},
        {"doc_id": "doc-3", "doc_name": "weather.pdf", "count": 1},
        {"doc_id": "doc-4", "doc_name": "cooking.pdf", "count": 1},
    ],
    "total": 3,
}

_KBINFOS_LONG_CONTEXT = {
    "chunks": [
        {
            "doc_id": "doc-1",
            "content_ltks": "ragflow version 1.0 released 2024",
            "content_with_weight": "RAGFlow version 1.0 was released in 2024 with major features including hybrid search and reranking support. The system now supports up to 10 million documents with sub-second query latency. Key improvements include better Chinese language understanding and integration with multiple LLM providers like OpenAI, Anthropic, and local models.",
            "vector": [0.1, 0.2, 0.3],
            "docnm_kwd": "release_notes.pdf",
            "similarity": 0.92,
        },
        {
            "doc_id": "doc-2",
            "content_ltks": "ragflow performance benchmark 10 million documents",
            "content_with_weight": "Performance benchmarks show that RAGFlow can handle 10 million documents with an average query latency of 120ms. The P99 latency is 450ms. Memory usage is optimized at approximately 2GB per 1 million documents. These numbers were measured on a server with 32 CPU cores and 64GB RAM.",
            "vector": [0.15, 0.25, 0.35],
            "docnm_kwd": "benchmark.pdf",
            "similarity": 0.88,
        },
        {
            "doc_id": "doc-3",
            "content_ltks": "ragflow supported llm providers openai anthropic local",
            "content_with_weight": "Supported LLM providers include: OpenAI (GPT-4, GPT-3.5), Anthropic (Claude 3 Opus, Claude 3 Sonnet), and local models via Ollama or HuggingFace. Each provider has specific configuration options for temperature, max tokens, and other generation parameters.",
            "vector": [0.12, 0.22, 0.32],
            "docnm_kwd": "llm_config.pdf",
            "similarity": 0.85,
        },
    ],
    "doc_aggs": [
        {"doc_id": "doc-1", "doc_name": "release_notes.pdf", "count": 1},
        {"doc_id": "doc-2", "doc_name": "benchmark.pdf", "count": 1},
        {"doc_id": "doc-3", "doc_name": "llm_config.pdf", "count": 1},
    ],
    "total": 3,
}

_KBINFOS_INSUFFICIENT_EVIDENCE = {
    "chunks": [
        {
            "doc_id": "doc-1",
            "content_ltks": "ragflow rag engine",
            "content_with_weight": "RAGFlow is a RAG engine.",
            "vector": [0.1, 0.2, 0.3],
            "docnm_kwd": "intro.pdf",
            "similarity": 0.2,
        },
    ],
    "doc_aggs": [{"doc_id": "doc-1", "doc_name": "intro.pdf", "count": 1}],
    "total": 1,
}

_KBINFOS_RELEVANT_BUT_INSUFFICIENT = {
    "chunks": [
        {
            "doc_id": "doc-1",
            "content_ltks": "ragflow rag engine features",
            "content_with_weight": "RAGFlow is a RAG engine with many features.",
            "vector": [0.1, 0.2, 0.3],
            "docnm_kwd": "intro.pdf",
            "similarity": 0.85,
        },
    ],
    "doc_aggs": [{"doc_id": "doc-1", "doc_name": "intro.pdf", "count": 1}],
    "total": 1,
}


# ---------------------------------------------------------------------------
# Tests for 5 core targets - 核心目标测试
# ---------------------------------------------------------------------------

class TestCoreTargets:
    """Tests for the 5 core targets required by the task."""

    def test_target_1_sufficient_evidence_normal_answer(self):
        """
        目标 1: 有证据正常回答 - Normal answer with sufficient evidence
        
        验证：当有足够证据时，GuardResult.is_sufficient = True，正常继续流程
        """
        guard = AnswerQualityGuard(
            similarity_threshold=0.3,
            max_context_tokens=2000
        )
        kbinfos = deepcopy(_KBINFOS_WITH_EVIDENCE)
        
        result = guard.process_retrieval_results(
            kbinfos,
            question="What is RAGFlow and what formats does it support?",
            max_tokens=2000
        )
        
        assert result.is_sufficient is True, "Should have sufficient evidence"
        assert len(result.filtered_chunks) == 2, "All relevant chunks should be preserved"
        assert len(result.compressed_context) > 0, "Should have compressed context"

    def test_target_2_insufficient_evidence_rejects_hallucination(self):
        """
        目标 2: 证据不足拒绝编造 - Reject hallucination when evidence is insufficient
        
        验证：
        1. 当 chunks 通过相似度过滤但证据不足时，is_sufficient = False
        2. 注意：之前的 bug 是条件判断错误，现在已修复为 if not is_sufficient
        """
        guard = AnswerQualityGuard(
            similarity_threshold=0.3,
            max_context_tokens=2000
        )
        kbinfos = deepcopy(_KBINFOS_RELEVANT_BUT_INSUFFICIENT)
        
        result = guard.process_retrieval_results(
            kbinfos,
            question="What is the pricing model of RAGFlow Enterprise Edition and how many API calls are included per month?",
            max_tokens=2000
        )
        
        assert len(result.filtered_chunks) == 1, "Chunk passes similarity filter"
        assert result.filtered_chunks[0]["similarity"] >= 0.3, "Chunk is similar enough"
        
        assert result.is_sufficient is False, "Should detect insufficient evidence"
        assert len(result.missing_info) > 0, "Should identify missing information"
        
        response = guard.generate_insufficient_response(
            question="What is the pricing of RAGFlow?",
            missing_info=result.missing_info
        )
        assert "无法准确回答" in response, "Should generate rejection response"

    def test_target_3_citation_maps_to_original_chunk(self):
        """
        目标 3: citation 能对应原始 chunk - Citations correctly map to original chunks
        
        验证：
        1. [ID:i] 中的 i 必须在有效范围内
        2. chunk_mapping[i] 必须等于原始 chunks[i]
        3. 增强：内容支撑检查 - 引用的句子必须能被对应 chunk 支撑
        """
        guard = AnswerQualityGuard()
        chunks = deepcopy(_KBINFOS_WITH_EVIDENCE["chunks"])
        
        answer = "RAGFlow is a RAG engine [ID:0]. It supports PDF, Word, and Excel formats [ID:1]."
        
        result = guard.validate_citations(answer, chunks, strict_mode=True)
        
        assert result.is_valid is True, "Citations should be valid"
        assert 0 in result.validated_citations, "Citation 0 should be valid"
        assert 1 in result.validated_citations, "Citation 1 should be valid"
        
        assert 0 in result.chunk_mapping, "Chunk 0 should be in mapping"
        assert result.chunk_mapping[0] == chunks[0], "Mapping should point to original chunk"
        assert result.chunk_mapping[1]["doc_id"] == "doc-2", "Should map to correct doc_id"
        
        assert "[ID:0]" in result.validated_answer, "Valid citation should remain"
        assert "[ID:1]" in result.validated_answer, "Valid citation should remain"

    def test_target_4_low_relevance_chunks_filtered(self):
        """
        目标 4: 低相关内容被过滤 - Low-relevance chunks are filtered out
        
        验证：
        1. similarity 低于阈值的 chunks 被过滤
        2. 过滤后的 filtered_chunks 只保留高相关 chunks
        3. filtered_count 统计被过滤的数量
        """
        guard = AnswerQualityGuard(
            similarity_threshold=0.3,
            max_context_tokens=2000
        )
        kbinfos = deepcopy(_KBINFOS_LOW_RELEVANCE)
        
        original_count = len(kbinfos["chunks"])
        
        result = guard.process_retrieval_results(
            kbinfos,
            question="Tell me about RAGFlow features.",
            max_tokens=2000
        )
        
        assert result.filtered_count == 2, f"Expected 2 chunks filtered, got {result.filtered_count}"
        assert len(result.filtered_chunks) == 1, "Should have 1 relevant chunk remaining"
        assert result.original_chunk_count == 3, "Original count should be 3"
        
        assert result.filtered_chunks[0]["doc_id"] == "doc-1", "Only RAGFlow chunk should remain"
        assert result.filtered_chunks[0]["docnm_kwd"] == "intro.pdf", "Should be intro.pdf"
        
        for chunk in result.filtered_chunks:
            assert chunk["similarity"] >= 0.3, f"Chunk {chunk['doc_id']} should have similarity >= 0.3"

    def test_target_5_long_context_preserves_key_evidence(self):
        """
        目标 5: 长上下文压缩后关键证据仍在 - Key evidence preserved after context compression
        
        验证：
        1. 压缩后的 filtered_chunks 中，被压缩的 chunk 的 content_with_weight 被更新
        2. 关键证据（数字、日期、专有名词）必须保留
        3. compressed_context 包含压缩后的内容
        """
        guard = AnswerQualityGuard(
            similarity_threshold=0.3,
            max_context_tokens=500
        )
        kbinfos = deepcopy(_KBINFOS_LONG_CONTEXT)
        
        result = guard.process_retrieval_results(
            kbinfos,
            question="What are the key release details and performance metrics for RAGFlow 1.0?",
            max_tokens=500
        )
        
        assert len(result.filtered_chunks) > 0, "Should have some chunks preserved"
        
        key_evidence = ["1.0", "2024"]
        for key in key_evidence:
            assert key in result.compressed_context, \
                f"Key evidence '{key}' should be preserved in compressed_context"
        
        for chunk in result.filtered_chunks:
            content = chunk.get("content_with_weight", "")
            if chunk.get("_original_content"):
                assert chunk["_original_content"] != content, \
                    "Compressed chunk should have different content from original"
        
        assert result.is_sufficient is True, "Should still be sufficient after compression"


# ---------------------------------------------------------------------------
# Tests for citation content support - 引用内容支撑测试
# ---------------------------------------------------------------------------

class TestCitationContentSupport:
    """Tests for citation content support validation."""

    def test_unsupported_citation_removed(self):
        """
        测试：引用的内容不被对应 chunk 支撑时，引用被移除
        
        场景：回答引用 [ID:0]，但句子内容与 chunk[0] 完全不相关
        """
        guard = AnswerQualityGuard()
        chunks = [
            {
                "doc_id": "doc-1",
                "content_ltks": "ragflow rag engine",
                "content_with_weight": "RAGFlow is a RAG engine built for document understanding.",
                "docnm_kwd": "intro.pdf",
                "similarity": 0.85,
            },
        ]
        
        answer = "The weather is sunny today [ID:0]."
        
        result = guard.validate_citations(answer, chunks, strict_mode=True, content_overlap_threshold=0.3)
        
        assert result.is_valid is False, "Should detect unsupported citation"
        assert 0 in result.invalid_citations, "Citation 0 should be invalid due to low content overlap"
        assert "[ID:0]" not in result.validated_answer, "Unsupported citation should be removed"
        assert "sunny" in result.validated_answer, "Answer text should remain"

    def test_mixed_supported_unsupported_citations(self):
        """
        测试：混合的有效和无效引用
        """
        guard = AnswerQualityGuard()
        chunks = [
            {
                "doc_id": "doc-1",
                "content_ltks": "ragflow rag engine",
                "content_with_weight": "RAGFlow is a RAG engine built for document understanding.",
                "docnm_kwd": "intro.pdf",
                "similarity": 0.85,
            },
            {
                "doc_id": "doc-2",
                "content_ltks": "weather forecast",
                "content_with_weight": "The weather forecast is sunny with temperature 25 degrees.",
                "docnm_kwd": "weather.pdf",
                "similarity": 0.15,
            },
        ]
        
        answer = "RAGFlow is a RAG engine [ID:0]. The weather is sunny today [ID:1]."
        
        result = guard.validate_citations(answer, chunks, strict_mode=True, content_overlap_threshold=0.2)
        
        assert 0 in result.validated_citations, "Citation 0 should be valid (supported by content)"
        assert 1 in result.validated_citations, "Citation 1 should be valid (weather in chunk 1)"

    def test_citation_without_sentence_still_valid_by_index(self):
        """
        测试：当句子内容与 chunk 相关时，引用有效
        """
        guard = AnswerQualityGuard()
        chunks = [
            {
                "doc_id": "doc-1",
                "content_ltks": "ragflow rag engine",
                "content_with_weight": "RAGFlow is a RAG engine.",
                "docnm_kwd": "intro.pdf",
            },
        ]
        
        answer = "RAGFlow is a RAG engine [ID:0]."
        
        result = guard.validate_citations(answer, chunks, strict_mode=True)
        
        assert 0 in result.validated_citations, "Citation 0 should be valid when content matches"
        assert result.is_valid is True, "Should be valid"

    def test_citation_content_mismatch_removed(self):
        """
        测试：当句子内容与 chunk 完全不相关时，引用被移除（即使索引在范围内）
        
        这是新增的内容支撑检查功能的行为。
        """
        guard = AnswerQualityGuard()
        chunks = [
            {
                "doc_id": "doc-1",
                "content_ltks": "ragflow rag engine",
                "content_with_weight": "RAGFlow is a RAG engine.",
                "docnm_kwd": "intro.pdf",
            },
        ]
        
        answer = "The weather is sunny today [ID:0]."
        
        result = guard.validate_citations(answer, chunks, strict_mode=True, content_overlap_threshold=0.2)
        
        assert 0 not in result.validated_citations, "Citation 0 should be invalid due to content mismatch"
        assert 0 in result.invalid_citations, "Citation 0 should be in invalid list"
        assert result.is_valid is False, "Should be invalid"
        assert "[ID:0]" not in result.validated_answer, "Invalid citation should be removed"


# ---------------------------------------------------------------------------
# Tests for integrated workflow simulation - 集成流程模拟测试
# ---------------------------------------------------------------------------

class TestIntegratedWorkflow:
    """
    模拟主流程中的 AQG 行为
    
    模拟 dialog_service.py 中的实际接入逻辑：
    1. retrieval 返回 kbinfos
    2. guard.process_retrieval_results() 处理
    3. 检查 result.is_sufficient
    4. 如果是 False，返回拒绝回答
    5. 否则，更新 kbinfos["chunks"] 为 result.filtered_chunks
    """

    def test_simulated_workflow_sufficient(self):
        """
        模拟：有足够证据时的完整流程
        """
        guard = AnswerQualityGuard(
            similarity_threshold=0.3,
            max_context_tokens=2000
        )
        
        kbinfos = deepcopy(_KBINFOS_WITH_EVIDENCE)
        
        guard_result = guard.process_retrieval_results(
            kbinfos,
            question="What is RAGFlow?",
            max_tokens=2000
        )
        
        if guard_result.filtered_chunks:
            kbinfos["chunks"] = guard_result.filtered_chunks
            kbinfos["total"] = len(guard_result.filtered_chunks)
        
        if not guard_result.is_sufficient:
            response = guard.generate_insufficient_response(
                "What is RAGFlow?",
                guard_result.missing_info
            )
            pytest.fail("Should not reject when evidence is sufficient")
        
        assert guard_result.is_sufficient is True, "Should be sufficient"
        assert len(kbinfos["chunks"]) == 2, "kbinfos should have 2 chunks"

    def test_simulated_workflow_insufficient(self):
        """
        模拟：证据不足时的完整流程
        
        关键：验证修复后的条件判断 if not is_sufficient（而不是之前的 and not filtered_chunks）
        """
        guard = AnswerQualityGuard(
            similarity_threshold=0.3,
            max_context_tokens=2000
        )
        
        kbinfos = deepcopy(_KBINFOS_RELEVANT_BUT_INSUFFICIENT)
        
        guard_result = guard.process_retrieval_results(
            kbinfos,
            question="What is the pricing of RAGFlow Enterprise?",
            max_tokens=2000
        )
        
        assert len(guard_result.filtered_chunks) == 1, "Chunk passes similarity filter"
        
        if guard_result.filtered_chunks:
            kbinfos["chunks"] = guard_result.filtered_chunks
        
        if not guard_result.is_sufficient:
            response = guard.generate_insufficient_response(
                "What is the pricing of RAGFlow Enterprise?",
                guard_result.missing_info
            )
            assert "无法准确回答" in response, "Should generate rejection response"
        else:
            pytest.fail("Should reject when evidence is insufficient")

    def test_simulated_workflow_low_relevance_filtered(self):
        """
        模拟：低相关 chunks 被过滤后的流程
        """
        guard = AnswerQualityGuard(
            similarity_threshold=0.3,
            max_context_tokens=2000
        )
        
        kbinfos = deepcopy(_KBINFOS_LOW_RELEVANCE)
        original_chunks = deepcopy(kbinfos["chunks"])
        
        guard_result = guard.process_retrieval_results(
            kbinfos,
            question="What is RAGFlow?",
            max_tokens=2000
        )
        
        assert guard_result.filtered_count == 2, "2 chunks should be filtered"
        assert len(guard_result.filtered_chunks) == 1, "Only 1 relevant chunk remains"
        
        if guard_result.filtered_chunks:
            kbinfos["chunks"] = guard_result.filtered_chunks
        
        assert len(kbinfos["chunks"]) == 1, "kbinfos should be updated"
        assert kbinfos["chunks"][0]["doc_id"] == "doc-1", "Only RAGFlow chunk remains"
        
        for chunk in kbinfos["chunks"]:
            assert chunk["docnm_kwd"] not in ["weather.pdf", "cooking.pdf"], \
                f"Irrelevant chunk {chunk['docnm_kwd']} should be filtered out"


# ---------------------------------------------------------------------------
# Tests for AnswerQualityGuard - Unit level
# ---------------------------------------------------------------------------

class TestAnswerQualityGuardUnit:
    """Unit tests for individual components of AnswerQualityGuard."""

    def test_filter_low_relevant_chunks(self):
        """Test that low-relevance chunks are filtered out based on similarity threshold."""
        guard = AnswerQualityGuard(similarity_threshold=0.3)
        kbinfos = deepcopy(_KBINFOS_LOW_RELEVANCE)
        
        filtered_chunks, filtered_count = guard.filter_low_relevant_chunks(
            kbinfos["chunks"], 
            question="What is RAGFlow?"
        )
        
        assert len(filtered_chunks) == 1, f"Expected 1 relevant chunk, got {len(filtered_chunks)}"
        assert filtered_chunks[0]["doc_id"] == "doc-1", "Expected only RAGFlow-related chunk to remain"

    def test_compress_context_preserves_key_evidence(self):
        """Test that key evidence (numbers, dates, names) is preserved during compression."""
        guard = AnswerQualityGuard(max_context_tokens=500)
        kbinfos = deepcopy(_KBINFOS_LONG_CONTEXT)
        
        compressed, preserved = guard.compress_context(
            kbinfos["chunks"],
            max_tokens=500
        )
        
        assert "1.0" in compressed, "Key version number should be preserved"
        assert "2024" in compressed, "Key release year should be preserved"
        assert "10 million" in compressed, "Key metric should be preserved"
        assert "120ms" in compressed, "Key latency metric should be preserved"

    def test_check_evidence_sufficiency_sufficient(self):
        """Test that sufficient evidence is correctly identified."""
        guard = AnswerQualityGuard()
        kbinfos = deepcopy(_KBINFOS_WITH_EVIDENCE)
        
        is_sufficient, missing_info, reasoning = guard.check_evidence_sufficiency(
            question="What is RAGFlow and what formats does it support?",
            chunks=kbinfos["chunks"],
        )
        
        assert is_sufficient is True, "Should detect sufficient evidence"
        assert len(missing_info) == 0, "Should have no missing info"

    def test_check_evidence_sufficiency_insufficient(self):
        """Test that insufficient evidence is correctly identified."""
        guard = AnswerQualityGuard(similarity_threshold=0.5)
        kbinfos = deepcopy(_KBINFOS_INSUFFICIENT_EVIDENCE)
        
        filtered_chunks, _ = guard.filter_low_relevant_chunks(
            kbinfos["chunks"],
            question="What is the pricing model of RAGFlow and how many concurrent users can it support?"
        )
        
        is_sufficient, missing_info, reasoning = guard.check_evidence_sufficiency(
            question="What is the pricing model of RAGFlow and how many concurrent users can it support?",
            chunks=filtered_chunks,
        )
        
        assert is_sufficient is False, "Should detect insufficient evidence"

    def test_generate_insufficient_response(self):
        """Test that insufficient response is generated correctly."""
        guard = AnswerQualityGuard()
        
        response = guard.generate_insufficient_response(
            question="What is the pricing of RAGFlow?",
            missing_info=["pricing model", "cost details"]
        )
        
        assert "无法准确回答" in response, "Should indicate insufficient evidence"
        assert "pricing" in response.lower(), "Should mention the missing topic"


# ---------------------------------------------------------------------------
# Tests for AnswerQualityGuard - Edge cases
# ---------------------------------------------------------------------------

class TestAnswerQualityGuardEdgeCases:
    """Edge case tests for AnswerQualityGuard."""

    def test_empty_chunks(self):
        """Test handling of empty chunks list."""
        guard = AnswerQualityGuard()
        kbinfos = {"chunks": [], "doc_aggs": [], "total": 0}
        
        result = guard.process_retrieval_results(
            kbinfos,
            question="What is RAGFlow?",
            max_tokens=1000
        )
        
        assert result.is_sufficient is False, "Empty chunks should be insufficient"
        assert len(result.filtered_chunks) == 0, "No chunks to filter"

    def test_none_similarity_values(self):
        """Test handling of chunks without similarity scores."""
        guard = AnswerQualityGuard(similarity_threshold=0.3)
        chunks = [
            {
                "doc_id": "doc-1",
                "content_ltks": "ragflow rag engine",
                "content_with_weight": "RAGFlow is a RAG engine.",
                "vector": [0.1, 0.2, 0.3],
                "docnm_kwd": "intro.pdf",
            },
        ]
        
        filtered_chunks, filtered_count = guard.filter_low_relevant_chunks(
            chunks,
            question="What is RAGFlow?"
        )
        
        assert len(filtered_chunks) == 1, "Chunks without similarity should be preserved"

    def test_very_similar_query_and_chunks(self):
        """Test when query exactly matches chunk content."""
        guard = AnswerQualityGuard(similarity_threshold=0.5)
        chunks = [
            {
                "doc_id": "doc-1",
                "content_ltks": "ragflow version 1.0 released 2024",
                "content_with_weight": "RAGFlow version 1.0 was released in 2024.",
                "similarity": 0.95,
                "docnm_kwd": "release.pdf",
            },
        ]
        
        result = guard.process_retrieval_results(
            {"chunks": chunks, "doc_aggs": [], "total": 1},
            question="When was RAGFlow version 1.0 released?",
            max_tokens=1000
        )
        
        assert result.is_sufficient is True, "Should be sufficient with exact match"
        assert "1.0" in result.compressed_context, "Key info should be preserved"
        assert "2024" in result.compressed_context, "Key info should be preserved"

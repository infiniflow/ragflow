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

    def test_filter_low_relevant_chunks_preserves_high_similarity(self):
        """Test that high-relevance chunks are preserved."""
        guard = AnswerQualityGuard(similarity_threshold=0.3)
        kbinfos = deepcopy(_KBINFOS_WITH_EVIDENCE)
        
        filtered_chunks, filtered_count = guard.filter_low_relevant_chunks(
            kbinfos["chunks"],
            question="What document formats does RAGFlow support?"
        )
        
        assert len(filtered_chunks) == 2, f"Expected 2 relevant chunks, got {len(filtered_chunks)}"

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

    def test_validate_citations_valid(self):
        """Test that valid citations are preserved."""
        guard = AnswerQualityGuard()
        chunks = deepcopy(_KBINFOS_WITH_EVIDENCE["chunks"])
        answer = "RAGFlow is a RAG engine [ID:0]. It supports PDF, Word, and Excel formats [ID:1]."
        
        result = guard.validate_citations(answer, chunks)
        
        assert result.is_valid is True, "Should be valid"
        assert 0 in result.validated_citations, "Citation 0 should be valid"
        assert 1 in result.validated_citations, "Citation 1 should be valid"
        assert len(result.invalid_citations) == 0, "Should have no invalid citations"

    def test_validate_citations_invalid(self):
        """Test that invalid citations are removed."""
        guard = AnswerQualityGuard()
        chunks = deepcopy(_KBINFOS_WITH_EVIDENCE["chunks"])
        answer = "RAGFlow is a RAG engine [ID:0]. Some other info [ID:99]."
        
        result = guard.validate_citations(answer, chunks)
        
        assert result.is_valid is False, "Should detect invalid citations"
        assert 99 in result.invalid_citations, "Citation 99 should be invalid"
        assert "[ID:99]" not in result.validated_answer, "Invalid citation should be removed"
        assert "[ID:0]" in result.validated_answer, "Valid citation should remain"

    def test_validate_citations_partial_valid(self):
        """Test that partial valid citations are handled correctly."""
        guard = AnswerQualityGuard()
        chunks = deepcopy(_KBINFOS_WITH_EVIDENCE["chunks"])
        answer = "Valid [ID:0], invalid [ID:100], valid [ID:1]."
        
        result = guard.validate_citations(answer, chunks)
        
        assert 0 in result.validated_citations, "Citation 0 should be valid"
        assert 1 in result.validated_citations, "Citation 1 should be valid"
        assert 100 in result.invalid_citations, "Citation 100 should be invalid"
        assert "[ID:0]" in result.validated_answer, "Valid citation should remain"
        assert "[ID:1]" in result.validated_answer, "Valid citation should remain"
        assert "[ID:100]" not in result.validated_answer, "Invalid citation should be removed"

    def test_generate_insufficient_response(self):
        """Test that insufficient response is generated correctly."""
        guard = AnswerQualityGuard()
        
        response = guard.generate_insufficient_response(
            question="What is the pricing of RAGFlow?",
            missing_info=["pricing model", "cost details"]
        )
        
        assert "无法准确回答" in response or "insufficient" in response.lower() or "unable" in response.lower(), \
            "Should indicate insufficient evidence"
        assert "pricing" in response.lower(), "Should mention the missing topic"


# ---------------------------------------------------------------------------
# Tests for AnswerQualityGuard - Integration level
# ---------------------------------------------------------------------------

class TestAnswerQualityGuardIntegration:
    """Integration tests for the full process_retrieval_results pipeline."""

    def test_process_retrieval_results_sufficient_evidence(self):
        """Test: 有证据正常回答 - Normal answer with sufficient evidence."""
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
        
        assert isinstance(result, GuardResult), "Should return GuardResult"
        assert result.is_sufficient is True, "Should be sufficient"
        assert len(result.filtered_chunks) == 2, "Should have 2 filtered chunks"
        assert result.filtered_count == 0, "Should have 0 chunks filtered out"
        assert result.original_chunk_count == 2, "Original count should be 2"
        assert len(result.compressed_context) > 0, "Should have compressed context"

    def test_process_retrieval_results_filter_low_relevance(self):
        """Test: 低相关内容被过滤 - Low-relevance chunks are filtered out."""
        guard = AnswerQualityGuard(
            similarity_threshold=0.3,
            max_context_tokens=2000
        )
        kbinfos = deepcopy(_KBINFOS_LOW_RELEVANCE)
        
        result = guard.process_retrieval_results(
            kbinfos,
            question="Tell me about RAGFlow features.",
            max_tokens=2000
        )
        
        assert result.filtered_count == 2, f"Expected 2 chunks filtered out, got {result.filtered_count}"
        assert len(result.filtered_chunks) == 1, "Should have 1 relevant chunk remaining"
        assert result.filtered_chunks[0]["doc_id"] == "doc-1", "Only RAGFlow chunk should remain"
        
        irrelevant_docs = ["weather.pdf", "cooking.pdf"]
        for chunk in result.filtered_chunks:
            assert chunk.get("docnm_kwd") not in irrelevant_docs, \
                f"Irrelevant chunk {chunk.get('docnm_kwd')} should have been filtered out"

    def test_process_retrieval_results_long_context_compression(self):
        """Test: 长上下文压缩后仍保留关键证据 - Key evidence preserved after context compression."""
        guard = AnswerQualityGuard(
            similarity_threshold=0.3,
            max_context_tokens=300
        )
        kbinfos = deepcopy(_KBINFOS_LONG_CONTEXT)
        
        result = guard.process_retrieval_results(
            kbinfos,
            question="What are the key release details and performance metrics for RAGFlow 1.0?",
            max_tokens=300
        )
        
        key_evidence = ["1.0", "2024", "10 million", "120ms", "450ms", "2GB"]
        for key in key_evidence:
            assert key in result.compressed_context, \
                f"Key evidence '{key}' should be preserved in compressed context"
        
        assert result.is_sufficient is True, "Should still be sufficient after compression"

    def test_process_retrieval_results_insufficient_evidence(self):
        """Test: 证据不足拒绝编造 - Reject hallucination when evidence is insufficient."""
        guard = AnswerQualityGuard(
            similarity_threshold=0.5,
            max_context_tokens=2000
        )
        kbinfos = deepcopy(_KBINFOS_INSUFFICIENT_EVIDENCE)
        
        result = guard.process_retrieval_results(
            kbinfos,
            question="What is the pricing model of RAGFlow Enterprise Edition and how many API calls are included per month?",
            max_tokens=2000
        )
        
        assert result.is_sufficient is False, "Should detect insufficient evidence"


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

    def test_citation_format_variations(self):
        """Test that different citation formats are handled correctly."""
        guard = AnswerQualityGuard()
        chunks = deepcopy(_KBINFOS_WITH_EVIDENCE["chunks"])
        
        test_cases = [
            "Standard [ID:0] format.",
            "Multiple [ID:0][ID:1] citations.",
            "With space [ID: 0] format.",
            "Lowercase [id:1] format.",
            "Mixed [ID:0] and [id:1].",
        ]
        
        for answer in test_cases:
            result = guard.validate_citations(answer, chunks)
            assert result.validated_answer is not None, f"Should handle: {answer}"
            
        special_case = "Standard [ID:0] format."
        result = guard.validate_citations(special_case, chunks)
        assert 0 in result.validated_citations, "Standard citation should be valid"


# ---------------------------------------------------------------------------
# Tests for AnswerQualityGuard - Citations mapping
# ---------------------------------------------------------------------------

class TestAnswerQualityGuardCitationMapping:
    """Tests for: 引用能对上原始 chunk - Citations correctly map to original chunks."""

    def test_validate_citations_chunk_mapping(self):
        """Test that chunk_mapping correctly maps citation indices to actual chunks."""
        guard = AnswerQualityGuard()
        chunks = deepcopy(_KBINFOS_WITH_EVIDENCE["chunks"])
        answer = "RAGFlow is a RAG engine [ID:0]."
        
        result = guard.validate_citations(answer, chunks)
        
        assert 0 in result.chunk_mapping, "Citation 0 should be in chunk_mapping"
        assert result.chunk_mapping[0] == chunks[0], "Chunk mapping should point to correct chunk"
        assert result.chunk_mapping[0]["doc_id"] == "doc-1", "Should map to correct doc_id"

    def test_standardize_citations_with_metadata(self):
        """Test that citations can be standardized with source metadata."""
        guard = AnswerQualityGuard()
        chunks = [
            {
                "doc_id": "doc-123",
                "content_with_weight": "Test content.",
                "docnm_kwd": "important_document.pdf",
            },
        ]
        
        answer = "According to the source [ID:0]."
        result_answer, result_map = guard.standardize_citations(answer, chunks, include_source_metadata=True)
        
        assert isinstance(result_answer, str), "Should return answer string"
        assert isinstance(result_map, dict), "Should return citation map dict"
        assert 0 in result_map, "Citation 0 should be in result map"
        assert result_map[0]["document_id"] == "doc-123", "Should map to correct doc_id"
        assert result_map[0]["document_name"] == "important_document.pdf", "Should have correct document name"

    def test_citation_validation_preserves_answer_structure(self):
        """Test that answer text structure is preserved during citation validation."""
        guard = AnswerQualityGuard()
        chunks = deepcopy(_KBINFOS_WITH_EVIDENCE["chunks"])
        
        original_answer = "First sentence [ID:0]. Second sentence [ID:1]. Third sentence without citation."
        result = guard.validate_citations(original_answer, chunks)
        
        assert "First sentence" in result.validated_answer, "Text before citation should be preserved"
        assert "Second sentence" in result.validated_answer, "Text between citations should be preserved"
        assert "Third sentence without citation" in result.validated_answer, "Text after citations should be preserved"

    def test_citation_indices_match_original_chunks(self):
        """Test that validated citation indices exactly match the original chunk positions."""
        guard = AnswerQualityGuard()
        chunks = deepcopy(_KBINFOS_LONG_CONTEXT["chunks"])
        
        answer = "Release notes [ID:0], benchmarks [ID:1], LLM config [ID:2]."
        result = guard.validate_citations(answer, chunks)
        
        for idx in result.validated_citations:
            assert idx == 0 or idx == 1 or idx == 2, f"Citation index {idx} should be 0, 1, or 2"
            assert idx < len(chunks), f"Citation index {idx} should be valid for {len(chunks)} chunks"
        
        for idx in result.validated_citations:
            chunk = result.chunk_mapping[idx]
            expected_doc_id = chunks[idx]["doc_id"]
            assert chunk["doc_id"] == expected_doc_id, \
                f"Citation {idx} should map to doc_id {expected_doc_id}, got {chunk['doc_id']}"

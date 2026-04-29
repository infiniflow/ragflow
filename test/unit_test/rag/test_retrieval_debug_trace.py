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
import sys
import types
import logging

import numpy as np

from common.constants import PAGERANK_FLD, TAG_FLD


class _DummyTokenizer:
    def tag(self, *args, **kwargs):
        return []

    def freq(self, *args, **kwargs):
        return 0

    def _tradi2simp(self, text):
        return text

    def _strQ2B(self, text):
        return text


fake_infinity = types.ModuleType("infinity")
fake_infinity_tokenizer = types.ModuleType("infinity.rag_tokenizer")
fake_infinity_tokenizer.RagTokenizer = _DummyTokenizer
fake_infinity_tokenizer.is_chinese = lambda text: False
fake_infinity_tokenizer.is_number = lambda text: False
fake_infinity_tokenizer.is_alphabet = lambda text: True
fake_infinity_tokenizer.naive_qie = lambda text: text.split()
fake_infinity.rag_tokenizer = fake_infinity_tokenizer
sys.modules.setdefault("infinity", fake_infinity)
sys.modules.setdefault("infinity.rag_tokenizer", fake_infinity_tokenizer)

fake_query = types.ModuleType("rag.nlp.query")


class _DummyFulltextQueryer:
    pass


fake_query.FulltextQueryer = _DummyFulltextQueryer
sys.modules.setdefault("rag.nlp.query", fake_query)

fake_settings = types.ModuleType("common.settings")
fake_settings.DOC_ENGINE_INFINITY = False
fake_settings.DOC_ENGINE_OCEANBASE = False
sys.modules.setdefault("common.settings", fake_settings)

from rag.nlp.search import (
    ChunkDebugInfo,
    RetrievalDebugTrace,
    RETRIEVAL_DEBUG_TRACE_ENABLED,
)


class TestChunkDebugInfo:
    def test_to_dict_returns_correct_structure(self):
        chunk = ChunkDebugInfo(
            chunk_id="chunk_001",
            doc_id="doc_001",
            doc_name="test_document.pdf",
            kb_id="kb_001",
            initial_score=0.85,
            term_similarity=0.75,
            vector_similarity=0.90,
            rerank_score=0.82,
            rank_feature_score=0.1,
            filter_reason=None,
            final_position=0,
            content_preview="This is a test document content for retrieval.",
            is_pruned=False,
        )

        result = chunk.to_dict()

        assert result["chunk_id"] == "chunk_001"
        assert result["doc_id"] == "doc_001"
        assert result["doc_name"] == "test_document.pdf"
        assert result["kb_id"] == "kb_001"
        assert result["initial_score"] == 0.85
        assert result["term_similarity"] == 0.75
        assert result["vector_similarity"] == 0.90
        assert result["rerank_score"] == 0.82
        assert result["rank_feature_score"] == 0.1
        assert result["filter_reason"] is None
        assert result["final_position"] == 0
        assert "test document" in result["content_preview"]
        assert result["is_pruned"] is False

    def test_to_dict_truncates_long_content_preview(self):
        long_content = "a" * 200
        chunk = ChunkDebugInfo(
            chunk_id="chunk_001",
            doc_id="doc_001",
            doc_name="test.pdf",
            kb_id="kb_001",
            content_preview=long_content,
        )

        result = chunk.to_dict()

        assert len(result["content_preview"]) == 100

    def test_filter_reason_set_correctly(self):
        chunk_threshold = ChunkDebugInfo(
            chunk_id="c1",
            doc_id="d1",
            doc_name="test.pdf",
            kb_id="kb1",
            filter_reason="threshold",
        )
        assert chunk_threshold.to_dict()["filter_reason"] == "threshold"

        chunk_pagination = ChunkDebugInfo(
            chunk_id="c2",
            doc_id="d2",
            doc_name="test2.pdf",
            kb_id="kb1",
            filter_reason="pagination",
        )
        assert chunk_pagination.to_dict()["filter_reason"] == "pagination"


class TestRetrievalDebugTrace:
    def test_initialization_with_basic_params(self):
        trace = RetrievalDebugTrace(
            query="test query",
            tenant_ids=["tenant_001"],
            kb_ids=["kb_001", "kb_002"],
            top_k=1024,
            top_n=6,
            similarity_threshold=0.2,
            vector_similarity_weight=0.3,
        )

        assert trace.query == "test query"
        assert trace.tenant_ids == ["tenant_001"]
        assert trace.kb_ids == ["kb_001", "kb_002"]
        assert trace.top_k == 1024
        assert trace.top_n == 6
        assert trace.similarity_threshold == 0.2
        assert trace.vector_similarity_weight == 0.3
        assert trace.initial_search_count == 0
        assert trace.pruned_count == 0
        assert trace.rerank_used is False
        assert trace.rerank_model is None
        assert trace.filtered_by_threshold_count == 0
        assert trace.filtered_by_pagination_count == 0
        assert trace.final_chunks_count == 0
        assert trace.doc_engine_score_used is False
        assert trace.all_chunks is None
        assert trace.final_chunks is None

    def test_enable_detail_sets_lists(self):
        trace = RetrievalDebugTrace(
            query="test",
            tenant_ids=["t1"],
            kb_ids=["kb1"],
            top_k=10,
            top_n=5,
            similarity_threshold=0.1,
            vector_similarity_weight=0.5,
        )

        assert trace.all_chunks is None
        assert trace.final_chunks is None

        trace.enable_detail()

        assert trace.all_chunks == []
        assert trace.final_chunks == []

    def test_to_dict_returns_summary_without_detail(self):
        trace = RetrievalDebugTrace(
            query="test query",
            tenant_ids=["t1"],
            kb_ids=["kb1"],
            top_k=10,
            top_n=5,
            similarity_threshold=0.2,
            vector_similarity_weight=0.3,
        )

        trace.initial_search_count = 100
        trace.pruned_count = 5
        trace.rerank_used = True
        trace.rerank_model = "bge-reranker"
        trace.filtered_by_threshold_count = 30
        trace.filtered_by_pagination_count = 60
        trace.final_chunks_count = 5

        result = trace.to_dict()

        assert result["query"] == "test query"
        assert result["tenant_ids"] == ["t1"]
        assert result["kb_ids"] == ["kb1"]
        assert result["initial_search_count"] == 100
        assert result["pruned_count"] == 5
        assert result["rerank_used"] is True
        assert result["rerank_model"] == "bge-reranker"
        assert result["filtered_by_threshold_count"] == 30
        assert result["filtered_by_pagination_count"] == 60
        assert result["final_chunks_count"] == 5

        assert result["summary"]["selected"] == 5
        assert result["summary"]["pruned_deleted_docs"] == 5
        assert result["summary"]["filtered_by_threshold"] == 30
        assert result["summary"]["filtered_by_pagination"] == 60

        assert "all_chunks" not in result
        assert "final_chunks" not in result

    def test_to_dict_includes_detail_when_enabled(self):
        trace = RetrievalDebugTrace(
            query="test",
            tenant_ids=["t1"],
            kb_ids=["kb1"],
            top_k=10,
            top_n=5,
            similarity_threshold=0.2,
            vector_similarity_weight=0.3,
        )
        trace.enable_detail()

        chunk1 = ChunkDebugInfo(
            chunk_id="c1",
            doc_id="d1",
            doc_name="doc1.pdf",
            kb_id="kb1",
            term_similarity=0.8,
            vector_similarity=0.7,
            rerank_score=0.75,
            final_position=0,
        )
        chunk2 = ChunkDebugInfo(
            chunk_id="c2",
            doc_id="d2",
            doc_name="doc2.pdf",
            kb_id="kb1",
            term_similarity=0.3,
            vector_similarity=0.4,
            rerank_score=0.35,
            filter_reason="threshold",
        )

        trace.final_chunks.append(chunk1)
        trace.all_chunks.extend([chunk1, chunk2])
        trace.final_chunks_count = 1

        result = trace.to_dict()

        assert "all_chunks" in result
        assert "final_chunks" in result
        assert len(result["all_chunks"]) == 2
        assert len(result["final_chunks"]) == 1
        assert result["all_chunks"][0]["chunk_id"] == "c1"
        assert result["all_chunks"][1]["filter_reason"] == "threshold"

    def test_doc_engine_score_used_flag(self):
        trace_infinity = RetrievalDebugTrace(
            query="test",
            tenant_ids=["t1"],
            kb_ids=["kb1"],
            top_k=10,
            top_n=5,
            similarity_threshold=0.2,
            vector_similarity_weight=0.3,
        )
        trace_infinity.doc_engine_score_used = True

        result = trace_infinity.to_dict()
        assert result["doc_engine_score_used"] is True

    def test_log_summary_outputs_expected_format(self, caplog):
        trace = RetrievalDebugTrace(
            query="test search query",
            tenant_ids=["tenant_123"],
            kb_ids=["kb_456", "kb_789"],
            top_k=1024,
            top_n=6,
            similarity_threshold=0.2,
            vector_similarity_weight=0.3,
        )

        trace.initial_search_count = 50
        trace.pruned_count = 2
        trace.rerank_used = True
        trace.rerank_model = "bge-reranker-v2-m3"
        trace.doc_engine_score_used = False
        trace.filtered_by_threshold_count = 15
        trace.filtered_by_pagination_count = 27
        trace.final_chunks_count = 6

        trace.enable_detail()
        chunk = ChunkDebugInfo(
            chunk_id="chunk_001",
            doc_id="doc_001",
            doc_name="sample_doc.pdf",
            kb_id="kb_456",
            term_similarity=0.85,
            vector_similarity=0.92,
            rerank_score=0.89,
            final_position=0,
            content_preview="This is sample content for testing.",
        )
        trace.final_chunks.append(chunk)

        with caplog.at_level(logging.INFO):
            trace.log_summary()

        log_output = caplog.text

        assert "RETRIEVAL DEBUG TRACE SUMMARY" in log_output
        assert "test search query" in log_output
        assert "Initial search results: 50 chunks" in log_output
        assert "Pruned (deleted docs): 2 chunks" in log_output
        assert "Rerank used: True" in log_output
        assert "bge-reranker-v2-m3" in log_output
        assert "Filtered by threshold: 15 chunks" in log_output
        assert "Filtered by pagination: 27 chunks" in log_output
        assert "Final selected: 6 chunks" in log_output

        assert "FINAL CHUNKS DETAIL" in log_output
        assert "chunk_001" in log_output
        assert "sample_doc.pdf" in log_output
        assert "term_sim=0.8500" in log_output
        assert "vec_sim=0.9200" in log_output
        assert "rerank=0.8900" in log_output

    def test_log_summary_shows_filtered_chunks(self, caplog):
        trace = RetrievalDebugTrace(
            query="test",
            tenant_ids=["t1"],
            kb_ids=["kb1"],
            top_k=10,
            top_n=2,
            similarity_threshold=0.5,
            vector_similarity_weight=0.3,
        )

        trace.enable_detail()

        filtered1 = ChunkDebugInfo(
            chunk_id="filtered_001",
            doc_id="d1",
            doc_name="filtered_doc1.pdf",
            kb_id="kb1",
            term_similarity=0.3,
            vector_similarity=0.25,
            rerank_score=0.27,
            filter_reason="threshold",
        )
        filtered2 = ChunkDebugInfo(
            chunk_id="filtered_002",
            doc_id="d2",
            doc_name="filtered_doc2.pdf",
            kb_id="kb1",
            term_similarity=0.6,
            vector_similarity=0.7,
            rerank_score=0.65,
            filter_reason="pagination",
        )

        trace.all_chunks.extend([filtered1, filtered2])

        with caplog.at_level(logging.INFO):
            trace.log_summary()

        log_output = caplog.text

        assert "FILTERED CHUNKS" in log_output
        assert "filtered_001" in log_output
        assert "filtered_002" in log_output
        assert "reason=threshold" in log_output
        assert "reason=pagination" in log_output

    def test_log_summary_limits_filtered_chunks_display(self, caplog):
        trace = RetrievalDebugTrace(
            query="test",
            tenant_ids=["t1"],
            kb_ids=["kb1"],
            top_k=100,
            top_n=5,
            similarity_threshold=0.5,
            vector_similarity_weight=0.3,
        )

        trace.enable_detail()

        for i in range(25):
            chunk = ChunkDebugInfo(
                chunk_id=f"filtered_{i:03d}",
                doc_id=f"d_{i}",
                doc_name=f"doc_{i}.pdf",
                kb_id="kb1",
                term_similarity=0.3,
                vector_similarity=0.25,
                rerank_score=0.27,
                filter_reason="threshold",
            )
            trace.all_chunks.append(chunk)

        with caplog.at_level(logging.INFO):
            trace.log_summary()

        log_output = caplog.text

        assert "filtered_000" in log_output
        assert "filtered_019" in log_output
        assert "and 5 more filtered chunks" in log_output


class TestRetrievalDebugTraceConstant:
    def test_retrieval_debug_trace_enabled_is_false_by_default(self):
        assert RETRIEVAL_DEBUG_TRACE_ENABLED is False

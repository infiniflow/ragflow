#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
"""Unit tests for graceful rerank degradation in Dealer.retrieval.

A reranker is an enhancement layer over base retrieval. When the rerank
backend fails (HTTP 500, timeout, malformed response, ...) retrieval must
fall back to base scoring instead of propagating the exception and failing
the whole request. See issue #5579 (GPUStack /v1/rerank 500 crashed
retrieval).
"""

import sys
import types

import pytest

# Stub heavy / circular-importing dependencies before importing search,
# mirroring test_search_pagination.py so the module imports in isolation.
_fake_query = types.ModuleType("rag.nlp.query")


class _DummyFulltextQueryer:
    pass


_fake_query.FulltextQueryer = _DummyFulltextQueryer
sys.modules.setdefault("rag.nlp.query", _fake_query)

_fake_settings = types.ModuleType("common.settings")
_fake_settings.DOC_ENGINE_INFINITY = False
_fake_settings.DOC_ENGINE_OCEANBASE = False
_fake_settings.DOC_ENGINE_SERENEDB = False
sys.modules.setdefault("common.settings", _fake_settings)

from rag.nlp.search import Dealer  # noqa: E402


def _make_sres():
    return Dealer.SearchResult(
        total=2,
        ids=["c1", "c2"],
        query_vector=[0.1, 0.2],
        field={
            "c1": {
                "content_ltks": "alpha beta",
                "content_with_weight": "alpha beta",
                "kb_id": "kb1",
                "doc_id": "d1",
                "docnm_kwd": "doc1",
            },
            "c2": {
                "content_ltks": "gamma delta",
                "content_with_weight": "gamma delta",
                "kb_id": "kb1",
                "doc_id": "d2",
                "docnm_kwd": "doc2",
            },
        },
        highlight=None,
    )


class _RerankMdl:
    """Stand-in for an LLMBundle reranker."""


def _make_dealer(monkeypatch, *, rerank_impl):
    """Build a Dealer with mocked I/O and the given ``rerank_by_model`` impl."""
    monkeypatch.setattr("rag.nlp.search.settings.DOC_ENGINE_INFINITY", False, raising=False)
    monkeypatch.setattr("rag.nlp.search.settings.DOC_ENGINE_OCEANBASE", False, raising=False)
    monkeypatch.setattr("rag.nlp.search.settings.DOC_ENGINE_SERENEDB", False, raising=False)

    calls = {"rerank_by_model": 0, "rerank_with_knn": 0, "knn_scores": 0}

    async def _search(*_args, **_kwargs):
        return _make_sres()

    async def _prune(sres):
        return sres

    async def _knn_scores(*_args, **_kwargs):
        calls["knn_scores"] += 1
        return [0.8, 0.2]

    def _rerank_with_knn(*_args, **_kwargs):
        calls["rerank_with_knn"] += 1
        return [0.8, 0.2], [0.5, 0.5], [0.8, 0.2]

    def _rerank_by_model(*_args, **_kwargs):
        calls["rerank_by_model"] += 1
        return rerank_impl()

    dealer = Dealer.__new__(Dealer)
    dealer.search = _search
    dealer._prune_deleted_chunks = _prune
    dealer._knn_scores = _knn_scores
    dealer.rerank_with_knn = _rerank_with_knn
    dealer.rerank_by_model = _rerank_by_model
    return dealer, calls


async def _run_retrieval(dealer, rerank_mdl):
    return await dealer.retrieval(
        "question",
        None,
        ["t1"],
        ["kb1"],
        1,
        10,
        similarity_threshold=0.2,
        vector_similarity_weight=0.3,
        top=1024,
        doc_ids=None,
        aggs=True,
        rerank_mdl=rerank_mdl,
        highlight=False,
        rank_feature=None,
    )


@pytest.mark.p1
async def test_retrieval_falls_back_when_rerank_fails(monkeypatch):
    """A rerank failure degrades to base scoring instead of crashing."""

    def _raise():
        raise ValueError("500 Server Error: Internal Server Error for url: http://example/v1/rerank")

    dealer, calls = _make_dealer(monkeypatch, rerank_impl=_raise)

    ranks = await _run_retrieval(dealer, _RerankMdl())

    # The reranker was attempted, then the KNN fallback took over.
    assert calls["rerank_by_model"] == 1
    assert calls["rerank_with_knn"] == 1
    assert calls["knn_scores"] == 1
    # Retrieval still returns results, not a 500.
    assert ranks["total"] == 2
    assert len(ranks["chunks"]) == 2


@pytest.mark.p1
async def test_retrieval_uses_rerank_when_it_succeeds(monkeypatch):
    """When the reranker succeeds the base-scoring fallback is skipped."""

    def _ok():
        return [0.9, 0.3], [0.5, 0.5], [0.9, 0.3]

    dealer, calls = _make_dealer(monkeypatch, rerank_impl=_ok)

    ranks = await _run_retrieval(dealer, _RerankMdl())

    assert calls["rerank_by_model"] == 1
    assert calls["rerank_with_knn"] == 0
    assert calls["knn_scores"] == 0
    assert ranks["total"] == 2

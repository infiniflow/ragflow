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
import asyncio

import pytest

from common.doc_store.doc_store_base import MatchSparseExpr, MatchTextExpr, SparseVector
from rag.nlp import search as search_module


class _FakeQueryer:
    def question(self, text, min_match=0.3):
        return MatchTextExpr(["content_ltks^2"], text, 100, {"original_query": text, "minimum_should_match": min_match}), ["alpha"]

    def hybrid_similarity(self, *_args, **_kwargs):
        return [0.1, 0.9], [0.1, 0.9], [0.1, 0.9]

    def token_similarity(self, *_args, **_kwargs):
        return [0.1, 0.9]


class _FakeStore:
    def __init__(self):
        self.last_match_expressions = None

    def search(self, _src, _highlight_fields, _filters, match_expressions, *_args, **_kwargs):
        self.last_match_expressions = match_expressions
        return {
            "hits": {
                "total": {"value": 1},
                "hits": [
                    {
                        "_id": "doc-1",
                        "_score": 0.9,
                        "_source": {
                            "content_ltks": "alpha",
                            "content_with_weight": "alpha",
                            "kb_id": "kb-1",
                            "doc_id": "doc-1",
                            "docnm_kwd": "Doc 1",
                            "q_2_vec": [1.0, 0.0],
                        },
                    }
                ],
            }
        }

    @staticmethod
    def get_total(res):
        return res["hits"]["total"]["value"]

    @staticmethod
    def get_doc_ids(res):
        return [hit["_id"] for hit in res["hits"]["hits"]]

    @staticmethod
    def get_highlight(_res, _keywords, _field_name):
        return {}

    @staticmethod
    def get_aggregation(_res, _field_name):
        return []

    @staticmethod
    def get_fields(res, fields):
        result = {}
        for hit in res["hits"]["hits"]:
            result[hit["_id"]] = {field: hit["_source"].get(field) for field in fields if field in hit["_source"]}
            result[hit["_id"]]["_score"] = hit["_score"]
        return result


class _DenseOnlyEmbeddingModel:
    max_length = 8192

    @staticmethod
    def encode_queries(_text):
        return [1.0, 0.0], 0

    @staticmethod
    def supports_sparse():
        return False


class _HybridEmbeddingModel(_DenseOnlyEmbeddingModel):
    @staticmethod
    def supports_sparse():
        return True

    @staticmethod
    def encode_sparse_queries(_text):
        return SparseVector(indices=[7], values=[1.0]), 0


def test_qdrant_search_uses_sparse_rrf_when_available(monkeypatch):
    monkeypatch.setattr(search_module.settings, "DOC_ENGINE_QDRANT", True, raising=False)
    monkeypatch.setattr(search_module.settings, "DOC_ENGINE_INFINITY", False, raising=False)
    monkeypatch.setattr(search_module.query, "FulltextQueryer", _FakeQueryer)

    async def _thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    monkeypatch.setattr(search_module, "thread_pool_exec", _thread_pool_exec)

    store = _FakeStore()
    dealer = search_module.Dealer(store)

    result = asyncio.run(
        dealer.search(
            {"question": "alpha", "page": 1, "size": 5, "topk": 5, "similarity": 0.1},
            "ragflow_tenant",
            ["kb-1"],
            emb_mdl=_HybridEmbeddingModel(),
        )
    )

    assert any(isinstance(expr, MatchSparseExpr) for expr in store.last_match_expressions)
    assert store.last_match_expressions[-1].method == "rrf"
    assert result.backend_ranked is True
    assert result.query_sparse_vector.indices == [7]


def test_qdrant_search_falls_back_to_dense_only_when_sparse_disabled(monkeypatch):
    monkeypatch.setattr(search_module.settings, "DOC_ENGINE_QDRANT", True, raising=False)
    monkeypatch.setattr(search_module.settings, "DOC_ENGINE_INFINITY", False, raising=False)
    monkeypatch.setattr(search_module.query, "FulltextQueryer", _FakeQueryer)

    async def _thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    monkeypatch.setattr(search_module, "thread_pool_exec", _thread_pool_exec)

    store = _FakeStore()
    dealer = search_module.Dealer(store)

    result = asyncio.run(
        dealer.search(
            {"question": "alpha", "page": 1, "size": 5, "topk": 5, "similarity": 0.1},
            "ragflow_tenant",
            ["kb-1"],
            emb_mdl=_DenseOnlyEmbeddingModel(),
        )
    )

    assert not any(isinstance(expr, MatchSparseExpr) for expr in store.last_match_expressions)
    assert store.last_match_expressions[-1].method == "weighted_sum"
    assert result.backend_ranked is False


def test_qdrant_retrieval_preserves_backend_rrf_scores(monkeypatch):
    monkeypatch.setattr(search_module.settings, "DOC_ENGINE_QDRANT", True, raising=False)
    monkeypatch.setattr(search_module.settings, "DOC_ENGINE_INFINITY", False, raising=False)
    monkeypatch.setattr(search_module.query, "FulltextQueryer", _FakeQueryer)

    dealer = search_module.Dealer(_FakeStore())

    async def _fake_search(*_args, **_kwargs):
        return search_module.Dealer.SearchResult(
            total=2,
            ids=["doc-2", "doc-1"],
            query_vector=[1.0, 0.0],
            field={
                "doc-2": {
                    "_score": 0.9,
                    "content_ltks": "alpha",
                    "content_with_weight": "alpha second",
                    "kb_id": "kb-1",
                    "doc_id": "doc-2",
                    "docnm_kwd": "Doc 2",
                    "q_2_vec": [0.8, 0.2],
                    "position_int": [],
                },
                "doc-1": {
                    "_score": 0.5,
                    "content_ltks": "alpha alpha alpha",
                    "content_with_weight": "alpha first",
                    "kb_id": "kb-1",
                    "doc_id": "doc-1",
                    "docnm_kwd": "Doc 1",
                    "q_2_vec": [1.0, 0.0],
                    "position_int": [],
                },
            },
            highlight={},
            aggregation=[],
            keywords=["alpha"],
            backend_ranked=True,
        )

    monkeypatch.setattr(dealer, "search", _fake_search)

    result = asyncio.run(
        dealer.retrieval(
            question="alpha",
            embd_mdl=_HybridEmbeddingModel(),
            tenant_ids=["tenant-1"],
            kb_ids=["kb-1"],
            page=1,
            page_size=2,
            similarity_threshold=0.0,
            vector_similarity_weight=0.7,
            top=5,
            rerank_mdl=None,
            aggs=False,
            rank_feature={},
        )
    )

    assert [chunk["chunk_id"] for chunk in result["chunks"]] == ["doc-2", "doc-1"]

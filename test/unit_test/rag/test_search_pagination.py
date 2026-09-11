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
"""Unit tests for rerank-candidate pagination in Dealer.retrieval."""

import sys
import types

import pytest

# Stub the heavy / circular-importing dependencies before importing search,
# mirroring test_rank_feature_scores.py so the module imports in isolation.
_fake_query = types.ModuleType("rag.nlp.query")


class _DummyFulltextQueryer:
    pass


_fake_query.FulltextQueryer = _DummyFulltextQueryer
_fake_tokenizer = types.ModuleType("rag.nlp.rag_tokenizer")
sys.modules.setdefault("rag.nlp.query", _fake_query)
sys.modules.setdefault("rag.nlp.rag_tokenizer", _fake_tokenizer)
sys.modules.setdefault("common.settings", types.ModuleType("common.settings"))

from rag.nlp.search import Dealer, settings  # noqa: E402


def _search_result(total):
    ids = [f"chunk-{i}" for i in range(total)]
    fields = {
        chunk_id: {
            "_score": 1.0,
            "content_ltks": chunk_id,
            "content_with_weight": chunk_id,
            "doc_id": "doc-1",
            "docnm_kwd": "doc",
            "kb_id": "kb-1",
        }
        for chunk_id in ids
    }
    return Dealer.SearchResult(total=total, ids=ids, query_vector=[0.1], field=fields, highlight={})


@pytest.mark.asyncio
@pytest.mark.parametrize(("page", "page_size", "rerank_candidates_count"), [(1, 10, 64), (6, 10, 64), (7, 10, 70)])
async def test_retrieval_fetches_rerank_candidates_once_and_slices_requested_page(monkeypatch, page, page_size, rerank_candidates_count):
    requests = []

    async def fake_search(self, req, *args, **kwargs):
        requests.append(req)
        return _search_result(rerank_candidates_count)

    async def keep_all_chunks(self, search_result):
        return search_result

    monkeypatch.setattr(Dealer, "search", fake_search)
    monkeypatch.setattr(Dealer, "_prune_deleted_chunks", keep_all_chunks)
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", True, raising=False)
    monkeypatch.setattr(settings, "DOC_ENGINE_OCEANBASE", False, raising=False)
    monkeypatch.setattr(settings, "DOC_ENGINE_SERENEDB", False, raising=False)
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", False, raising=False)

    ranks = await Dealer.__new__(Dealer).retrieval(
        question="question",
        embd_mdl=object(),
        tenant_ids=["tenant-1"],
        kb_ids=["kb-1"],
        page=page,
        page_size=page_size,
        similarity_threshold=0.0,
        aggs=False,
        rerank_candidates_count=rerank_candidates_count,
    )

    assert len(requests) == 1
    assert requests[0]["page"] == 1
    assert requests[0]["size"] == rerank_candidates_count
    begin = (page - 1) * page_size
    assert [chunk["chunk_id"] for chunk in ranks["chunks"]] == [f"chunk-{i}" for i in range(begin, begin + page_size)]


@pytest.mark.asyncio
async def test_retrieval_rejects_page_beyond_rerank_candidates_count():
    with pytest.raises(Exception, match=r"rerank_candidates_count\(64\) must be greater than page \* page_size\(70\)"):
        await Dealer.__new__(Dealer).retrieval(
            question="question",
            embd_mdl=object(),
            tenant_ids=["tenant-1"],
            kb_ids=["kb-1"],
            page=7,
            page_size=10,
            rerank_candidates_count=64,
        )


@pytest.mark.asyncio
async def test_retrieval_rejects_pagination_with_reranker():
    with pytest.raises(Exception, match="Pagination is not supported when rerank_mdl is specified"):
        await Dealer.__new__(Dealer).retrieval(
            question="question",
            embd_mdl=object(),
            tenant_ids=["tenant-1"],
            kb_ids=["kb-1"],
            page=2,
            page_size=10,
            rerank_mdl=object(),
            rerank_candidates_count=64,
        )

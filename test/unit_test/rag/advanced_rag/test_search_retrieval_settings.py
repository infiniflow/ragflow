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
The agentic search tools must retrieve with the caller's retrieval settings.

`hybrid_search` hardcoded `vector_similarity_weight=0.3`, `similarity_threshold=0.2`
and `top_n=12`, so a chat assistant's own tuning had no effect on any query the
agentic path made. The classic (non-agentic) chat path always honoured these
fields; the agentic search framework (#16859) reimplemented retrieval without them.

`vector_search` and `bm25_search` keep their own weights and thresholds: those are
what those tools mean, not a default standing in for configuration.
"""

import pytest

from rag.advanced_rag.harness.tools import search as search_tools

pytestmark = pytest.mark.p1


class _Tools:
    """Minimal stand-in for RAGTools, carrying only what the search tools read."""

    def __init__(self, **settings):
        self.kb_ids = ["kb-1"]
        self.sql_kbs = []
        self.tenant_ids = ["t-1"]
        self.embed_mdl = object()
        self.search_cache = None
        for name, value in settings.items():
            setattr(self, name, value)


class _Recorder:
    """Captures the arguments of the single retrieval call a search tool makes."""

    def __init__(self):
        self.args = None
        self.kwargs = None

    async def retrieval(self, *args, **kwargs):
        self.args, self.kwargs = args, kwargs
        return {"chunks": [], "doc_aggs": []}

    @staticmethod
    def retrieval_by_children(chunks, _tenant_ids):
        return chunks


@pytest.fixture
def recorder(monkeypatch):
    rec = _Recorder()
    monkeypatch.setattr(search_tools.settings, "retriever", rec)
    monkeypatch.setattr(search_tools, "_normalize", lambda kbinfos, tenant_ids: kbinfos)
    return rec


def _top_k(rec):
    return rec.kwargs.get("knn_top_k")


def _call(rec):
    """(page_size, similarity_threshold, vector_similarity_weight, rerank_candidates_count)."""
    return (
        rec.args[5],
        rec.args[6],
        rec.kwargs["vector_similarity_weight"],
        rec.kwargs.get("rerank_candidates_count"),
    )


async def test_hybrid_search_uses_the_configured_settings(recorder):
    tools = _Tools(vector_similarity_weight=0.8, similarity_threshold=0.35, top_n=20, rerank_candidates_count=256, top_k=4096)

    await search_tools.hybrid_search(tools, query="q")

    assert _call(recorder) == (20, 0.35, 0.8, 256)
    assert _top_k(recorder) == 4096


async def test_hybrid_search_falls_back_when_nothing_is_configured(recorder):
    """An unconfigured caller must behave exactly as before this change."""
    await search_tools.hybrid_search(_Tools(), query="q")

    assert _call(recorder) == (12, 0.2, 0.3, 64)
    assert _top_k(recorder) == 1024


async def test_zero_weight_is_configuration_not_absence(recorder):
    """0.0 is a meaningful weight — it must not be mistaken for "unset"."""
    await search_tools.hybrid_search(_Tools(vector_similarity_weight=0.0, similarity_threshold=0.0), query="q")

    _, threshold, weight, _ = _call(recorder)
    assert weight == 0.0
    assert threshold == 0.0


async def test_no_embedding_model_forces_the_vector_leg_off(recorder):
    """Whatever the caller configured, there is no vector leg without an embedder."""
    tools = _Tools(vector_similarity_weight=0.8)
    tools.embed_mdl = None

    await search_tools.hybrid_search(tools, query="q")

    assert _call(recorder)[2] == 0


async def test_explicit_top_n_argument_beats_the_configuration(recorder):
    await search_tools.hybrid_search(_Tools(top_n=20), query="q", top_n=5)

    assert _call(recorder)[0] == 5


async def test_rerank_candidates_is_never_smaller_than_the_page_it_must_fill(recorder):
    """Dealer.retrieval rejects page * page_size > rerank_candidates_count."""
    await search_tools.hybrid_search(_Tools(top_n=100, rerank_candidates_count=64), query="q")

    page_size, _, _, rerank_candidates = _call(recorder)
    assert page_size == 100
    assert rerank_candidates >= page_size


async def test_vector_search_keeps_its_defining_weight(recorder):
    """Weight 1.0 is what this tool means, not a default standing in for configuration."""
    await search_tools.vector_search(_Tools(vector_similarity_weight=0.3, top_n=7), query="q")

    page_size, threshold, weight, _ = _call(recorder)
    assert (weight, threshold) == (1.0, 0.2)
    assert page_size == 7


async def test_bm25_search_keeps_its_defining_weight(recorder):
    """Weight 0 is what this tool means, not a default standing in for configuration."""
    await search_tools.bm25_search(_Tools(vector_similarity_weight=0.8, top_n=7), query="q")

    page_size, threshold, weight, _ = _call(recorder)
    assert (weight, threshold) == (0, 0.0)
    assert page_size == 7


async def test_top_k_reaches_every_search_tool(recorder):
    """The kNN candidate pool is recall for all three tools, not a hybrid-only knob."""
    for tool in (search_tools.hybrid_search, search_tools.vector_search, search_tools.bm25_search):
        await tool(_Tools(top_k=4096), query="q")
        assert _top_k(recorder) == 4096, tool.__name__


async def test_ragtools_retrieve_honours_the_configuration(monkeypatch):
    """The naive fallback (`_naive_rag` -> `RAGTools.retrieve`) bypasses the search
    tools, so it has to resolve the same settings itself."""
    from rag.advanced_rag import agentic_rag

    rec = _Recorder()
    monkeypatch.setattr(agentic_rag.settings, "retriever", rec)
    monkeypatch.setattr(agentic_rag, "label_question", lambda _q, _kbs: None)

    tools = agentic_rag.RAGTools.__new__(agentic_rag.RAGTools)
    tools.kb_ids = ["kb-1"]
    tools.kbs = []
    tools.tenant_ids = ["t-1"]
    tools.embed_mdl = object()
    tools.doc_scope = None
    tools.similarity_threshold = 0.35
    tools.vector_similarity_weight = 0.8
    tools.top_n = 20
    tools.rerank_candidates_count = 256
    tools.top_k = 4096
    monkeypatch.setattr(type(tools), "scoped_doc_ids", lambda _self, scope: scope, raising=False)

    await tools.retrieve("q", using_embedding=True)

    assert rec.args[5] == 20
    assert rec.args[6] == 0.35
    assert rec.kwargs["vector_similarity_weight"] == 0.8
    assert rec.kwargs["knn_top_k"] == 4096
    assert rec.kwargs["rerank_candidates_count"] == 256


async def test_ragtools_retrieve_keeps_its_own_defaults_when_unconfigured(monkeypatch):
    """Unconfigured callers keep this method's previous behaviour, which is not
    the same as the search tools' (top_n 6, vector weight 0.7)."""
    from rag.advanced_rag import agentic_rag

    rec = _Recorder()
    monkeypatch.setattr(agentic_rag.settings, "retriever", rec)
    monkeypatch.setattr(agentic_rag, "label_question", lambda _q, _kbs: None)

    tools = agentic_rag.RAGTools.__new__(agentic_rag.RAGTools)
    tools.kb_ids = ["kb-1"]
    tools.kbs = []
    tools.tenant_ids = ["t-1"]
    tools.embed_mdl = object()
    tools.doc_scope = None
    for name in ("similarity_threshold", "vector_similarity_weight", "top_n", "rerank_candidates_count", "top_k"):
        setattr(tools, name, None)
    monkeypatch.setattr(type(tools), "scoped_doc_ids", lambda _self, scope: scope, raising=False)

    await tools.retrieve("q", using_embedding=True)

    assert rec.args[5] == 6
    assert rec.args[6] == 0.2
    assert rec.kwargs["vector_similarity_weight"] == 0.7
    assert rec.kwargs["knn_top_k"] == 1024

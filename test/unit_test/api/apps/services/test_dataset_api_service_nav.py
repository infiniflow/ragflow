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
"""Regression tests for dataset navigation tree service helpers (#17301)."""

import importlib.util
import json
import math
import sys
from enum import IntEnum
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import pytest


pytestmark = pytest.mark.p2


class _StubModelTypeBinary(IntEnum):
    CHAT = 1
    EMBEDDING = 2


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    if "." in name:
        parent_name, _, child_name = name.rpartition(".")
        parent_mod = sys.modules.get(parent_name)
        if parent_mod is not None:
            monkeypatch.setattr(parent_mod, child_name, mod, raising=False)
    return mod


class _StubMatchTextExpr:
    """Stand-in for rag.nlp.search.MatchTextExpr (keyword-match fallback expr)."""

    def __init__(self, fields, text, top_k):
        self.fields, self.text, self.top_k = fields, text, top_k


def _load_nav_module(monkeypatch, *, accessible=True, index_pack=("idx-1", None), total=0, field_map=None, retriever=None):
    # Stubbed before the import: importing the real rag.nlp pulls in heavy
    # third-party deps the unit tier does not install.
    _stub(monkeypatch, "rag.nlp")
    _stub(monkeypatch, "rag.nlp.search", MatchTextExpr=_StubMatchTextExpr)

    doc_store = MagicMock()
    doc_store.search = MagicMock(return_value={})
    doc_store.get_fields = MagicMock(return_value=field_map or {})
    doc_store.get_total = MagicMock(return_value=total)
    doc_store.delete = MagicMock(return_value=0)

    if retriever is None:
        retriever = SimpleNamespace(retrieval=AsyncMock(return_value={"chunks": []}))

    kb = SimpleNamespace(tenant_id="tenant-1", id="kb-1")
    knowledgebase_service = SimpleNamespace(
        accessible=MagicMock(return_value=accessible),
        get_by_id=MagicMock(return_value=(True, kb)),
    )

    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_composite_model_name_by_ids=MagicMock(),
        resolve_model_config=MagicMock(),
        resolve_model_id=MagicMock(),
    )
    _stub(
        monkeypatch,
        "common.constants",
        PAGERANK_FLD="pagerank",
        LLMType=SimpleNamespace(),
        FileSource=SimpleNamespace(KNOWLEDGEBASE="knowledgebase"),
        RetCode=SimpleNamespace(SERVER_ERROR=500, ARGUMENT_ERROR=400),
        StatusEnum=SimpleNamespace(),
        TaskStatus=SimpleNamespace(),
        ModelTypeBinary=_StubModelTypeBinary,
    )
    _stub(monkeypatch, "common.settings", docStoreConn=doc_store, retriever=retriever, DOC_ENGINE="infinity")
    _stub(
        monkeypatch,
        "api.db.db_models",
        Connector2Kb=SimpleNamespace(),
        Document=SimpleNamespace(),
        File=SimpleNamespace(),
        SyncLogs=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(get_parsing_status_by_kb_ids=MagicMock()),
        queue_raptor_o_graphrag_tasks=MagicMock(),
    )
    _stub(monkeypatch, "api.db.services.file2document_service", File2DocumentService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.file_service", FileService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=knowledgebase_service,
        validate_dataset_embedding_models=lambda kbs: None,
    )
    _stub(
        monkeypatch,
        "api.db.services.connector_service",
        Connector2KbService=SimpleNamespace(),
        SyncLogsService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.services.task_service",
        GRAPH_RAPTOR_FAKE_DOC_ID="fake-doc",
        TaskService=SimpleNamespace(),
    )
    _stub(monkeypatch, "api.db.services.tenant_model_service", TenantModelService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.user_service",
        TenantService=SimpleNamespace(get_joined_tenants_by_user_id=lambda user_id: [{"tenant_id": "tenant-1"}]),
        UserService=SimpleNamespace(get_by_ids=lambda ids: []),
        UserTenantService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        deep_merge=MagicMock(),
        get_parser_config=MagicMock(),
        remap_dictionary_keys=lambda source_data, key_aliases=None: dict(source_data),
        verify_embedding_availability=MagicMock(),
    )
    _stub(
        monkeypatch,
        "common.misc_utils",
        thread_pool_exec=AsyncMock(side_effect=lambda fn, *args, **kwargs: fn(*args, **kwargs)),
        thread_pool_exec_long_time=AsyncMock(side_effect=lambda fn, *args, **kwargs: fn(*args, **kwargs)),
    )
    _stub(monkeypatch, "rag.advanced_rag.knowlege_compile.wiki", WIKI_PAGE_COMPILE_KWD="wiki")
    # The navigation legs build keyword/dense expressions from these doc-store
    # base classes; the compiled-agg keyword fallback and claim-agg vector leg
    # both import MatchTextExpr from here (not rag.nlp.search).
    _stub(monkeypatch, "common.doc_store.doc_store_base", OrderByExpr=MagicMock, MatchTextExpr=_StubMatchTextExpr)

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "services" / "dataset_api_service.py"
    spec = importlib.util.spec_from_file_location("test_dataset_api_service_nav_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_dataset_api_service_nav_module", module)
    spec.loader.exec_module(module)
    monkeypatch.setattr(module, "_compiled_index_or_none", lambda _tenant_id, _kb_id: index_pack)
    return module, knowledgebase_service, doc_store


def test_nav_item_shapes_cluster_row(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    row = {
        "name": "cluster-a",
        "type_kwd": "nav_cluster",
        "content_with_weight": json.dumps({"description": "Top cluster"}),
        "doc_count_int": 3,
    }
    item = module._nav_item(row)
    assert item == {
        "name": "cluster-a",
        "description": "Top cluster",
        "keywords": [],
        "entities": [],
        "graph_content": "",
        "doc_count": 3,
        "type": "cluster",
        "doc_id": None,
        "has_children": True,
    }


def test_nav_item_shapes_doc_leaf_row(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    row = {
        "name": "doc-leaf",
        "type_kwd": "nav_doc",
        "content_with_weight": "{}",
        "doc_id": "doc-123",
    }
    item = module._nav_item(row)
    assert item["type"] == "doc"
    assert item["doc_id"] == "doc-123"
    assert item["doc_count"] == 1
    assert item["has_children"] is False


@pytest.mark.asyncio
async def test_list_nav_clusters_returns_empty_when_index_missing(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch, index_pack=None)
    ok, payload = await module.list_nav_clusters("kb-1", "tenant-1")
    assert ok is True
    assert payload == {"total": 0, "items": []}


@pytest.mark.asyncio
async def test_list_nav_clusters_denies_inaccessible_dataset(monkeypatch):
    module, knowledgebase_service, doc_store = _load_nav_module(monkeypatch, accessible=False)
    ok, payload = await module.list_nav_clusters("kb-1", "tenant-1")
    assert ok is False
    assert payload == "no authorization"
    doc_store.search.assert_not_called()
    knowledgebase_service.get_by_id.assert_not_called()


@pytest.mark.asyncio
async def test_list_nav_children_uses_parent_name_filter(monkeypatch):
    module, _, doc_store = _load_nav_module(
        monkeypatch,
        total=1,
        field_map={
            "row-1": {
                "name": "child-a",
                "type_kwd": "nav_doc",
                "content_with_weight": "{}",
                "doc_id": "doc-1",
            }
        },
    )

    ok, payload = await module.list_nav_children("kb-1", "tenant-1", "cluster-a")
    assert ok is True
    assert payload["total"] == 1
    assert payload["items"][0]["name"] == "child-a"
    call_kwargs = doc_store.search.call_args.kwargs
    condition = call_kwargs.get("condition") or doc_store.search.call_args.args[2]
    assert condition["parent_kwd"] == ["cluster-a"]


@pytest.mark.asyncio
async def test_delete_nav_returns_zero_when_index_missing(monkeypatch):
    module, _, doc_store = _load_nav_module(monkeypatch, index_pack=None)
    ok, payload = await module.delete_nav("kb-1", "tenant-1")
    assert ok is True
    assert payload == {"deleted": 0}
    doc_store.delete.assert_not_called()


# ---------------------------------------------------------------------------
# Document routing via compiled claim rows
# ---------------------------------------------------------------------------


def _claim_row(doc_id, similarity, name="n"):
    body = {"type": "claim", "name": name, "description": "d"}
    return {"doc_id": doc_id, "similarity": similarity, "content_with_weight": json.dumps(body)}


def test_nav_bucket_compiled_rows_bins_claims_per_document(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    buckets = module._nav_bucket_compiled_rows(
        {
            "r1": _claim_row("doc-a", 0.9),
            "r2": _claim_row("doc-a", 0.5),
            "r3": _claim_row("doc-b", 0.7),
            # A title row is never an evidence row, so it drops.  (fact /
            # conclusion DO count as evidence rows on page_index, so they cannot
            # be used here to exercise the drop path.)
            "r4": {"doc_id": "doc-c", "similarity": 0.6, "content_with_weight": json.dumps({"type": "title"})},
        }
    )
    assert set(buckets["claim"]) == {"doc-a", "doc-b"}
    assert buckets["claim"]["doc-a"]["hits"] == 2
    assert buckets["claim"]["doc-a"]["best"] == pytest.approx(0.9)
    assert buckets["claim"]["doc-a"]["total"] == pytest.approx(1.4)


def test_nav_bucket_compiled_rows_skips_unparseable_payloads(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    buckets = module._nav_bucket_compiled_rows(
        {
            "r1": {"doc_id": "doc-a", "similarity": 0.9, "content_with_weight": "{not json"},
            "r2": {"doc_id": "", "similarity": 0.9, "content_with_weight": json.dumps({"type": "claim"})},
            "r3": {"doc_id": "doc-b", "similarity": 0.9, "content_with_weight": json.dumps([1, 2])},
        }
    )
    assert buckets == {"claim": {}}


def test_nav_rank_compiled_buckets_orders_by_claim_score(monkeypatch):
    """Claims are the only compiled leg, so ranking is a straight DocScore order."""
    module, _, _ = _load_nav_module(monkeypatch)
    buckets = module._nav_bucket_compiled_rows(
        {
            "r1": _claim_row("weak", 0.5),
            "r2": _claim_row("strong", 0.9),
        }
    )
    ranked = module._nav_rank_compiled_buckets(buckets, 10)
    assert [d for d, _ in ranked] == ["strong", "weak"]
    assert ranked[0][1]["legs"] == {"claim"}
    # best stays on the 0..1 cosine scale callers threshold on.
    assert ranked[0][1]["best"] == pytest.approx(0.9)


def test_nav_rank_compiled_buckets_does_not_mutate_input(monkeypatch):
    """Regression: ranking used to attach 'legs' onto the caller's bucket entries."""
    module, _, _ = _load_nav_module(monkeypatch)
    buckets = module._nav_bucket_compiled_rows({"r1": _claim_row("doc-a", 0.8)})
    before = json.dumps(buckets, sort_keys=True, default=str)
    module._nav_rank_compiled_buckets(buckets, 10)
    assert json.dumps(buckets, sort_keys=True, default=str) == before


def test_nav_rank_compiled_buckets_respects_top_k(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    buckets = module._nav_bucket_compiled_rows({f"r{i}": _claim_row(f"doc-{i}", 0.9 - i * 0.1) for i in range(5)})
    assert len(module._nav_rank_compiled_buckets(buckets, 2)) == 2


@pytest.mark.asyncio
def test_nav_aggregate_chunks_rolls_up_per_document(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    chunks = [
        {"doc_id": "doc-a", "similarity": 0.9},
        {"doc_id": "doc-b", "similarity": 0.8},
        {"doc_id": "doc-a", "similarity": 0.7},
        {"doc_id": "   ", "similarity": 0.5},  # blank doc_id — dropped
        {"doc_id": "doc-a", "score": 0.5},  # 'score' fallback key
    ]
    agg = module._nav_aggregate_chunks(chunks)
    assert set(agg) == {"doc-a", "doc-b"}
    assert agg["doc-a"] == {"total": pytest.approx(2.1), "best": 0.9, "hits": 3}
    assert agg["doc-b"] == {"total": pytest.approx(0.8), "best": 0.8, "hits": 1}


def test_nav_doc_score_damps_hit_count(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    single = module._nav_doc_score({"total": 0.9, "best": 0.9, "hits": 1})
    assert single == pytest.approx(0.9 / math.sqrt(2))
    # A long document must not win on volume alone: four weak hits stay below
    # one strong hit.
    weak_many = module._nav_doc_score({"total": 1.0, "best": 0.25, "hits": 4})
    assert weak_many == pytest.approx(1.0 / math.sqrt(5))
    assert weak_many < single


@pytest.mark.asyncio
async def test_chunk_agg_router_ranks_by_doc_score_not_hit_count(monkeypatch):
    """doc-b has one strong hit; doc-a has four weak ones. doc-b must win."""
    retriever = SimpleNamespace(
        retrieval=AsyncMock(
            return_value={
                "chunks": [
                    {"doc_id": "doc-a", "similarity": 0.20},
                    {"doc_id": "doc-a", "similarity": 0.20},
                    {"doc_id": "doc-a", "similarity": 0.20},
                    {"doc_id": "doc-a", "similarity": 0.20},
                    {"doc_id": "doc-b", "similarity": 0.90},
                ]
            }
        )
    )
    module, _, _ = _load_nav_module(monkeypatch, retriever=retriever)

    ok, payload = await module._search_layers_chunk_agg("tenant-1", "kb-1", "q", 5, None)
    assert ok is True
    assert [item["doc_id"] for item in payload["items"]] == ["doc-b", "doc-a"]
    # score stays on the 0..1 chunk-similarity scale callers threshold on.
    assert payload["items"][0]["score"] == pytest.approx(0.9)
    assert payload["items"][1]["score"] == pytest.approx(0.2)
    assert payload["items"][0]["_agg"]["hits"] == 1
    assert payload["items"][1]["_agg"]["hits"] == 4


@pytest.mark.asyncio
async def test_chunk_agg_router_uses_full_candidate_pool(monkeypatch):
    """page_size covers the pool so every fused candidate reaches the aggregation."""
    retrieval = AsyncMock(return_value={"chunks": [{"doc_id": "doc-a", "similarity": 0.5}]})
    module, _, _ = _load_nav_module(monkeypatch, retriever=SimpleNamespace(retrieval=retrieval))

    await module._search_layers_chunk_agg("tenant-1", "kb-1", "q", 12, None)
    args, kw = retrieval.call_args
    # page_size is positional arg 6; no similarity floor at 7.
    assert args[5] == module._NAV_CHUNK_AGG_POOL
    assert args[6] == 0.0
    # The pool is rerank_candidates_count — page_size alone would leave the
    # candidate set at the 64 default, too narrow for DocScore to matter.
    assert kw["rerank_candidates_count"] == module._NAV_CHUNK_AGG_POOL
    # page * page_size must never exceed rerank_candidates_count or retrieval raises.
    assert args[4] * args[5] <= kw["rerank_candidates_count"]


@pytest.mark.asyncio
async def test_chunk_agg_router_does_not_narrow_knn_candidates(monkeypatch):
    """knn_top_k caps what the vector leg feeds fusion; narrowing it shrinks the pool."""
    retrieval = AsyncMock(return_value={"chunks": [{"doc_id": "doc-a", "similarity": 0.5}]})
    module, _, _ = _load_nav_module(monkeypatch, retriever=SimpleNamespace(retrieval=retrieval))

    await module._search_layers_chunk_agg("tenant-1", "kb-1", "q", 12, None)
    assert "knn_top_k" not in retrieval.call_args.kwargs


@pytest.mark.asyncio
async def test_chunk_agg_router_excludes_compiled_rows(monkeypatch):
    """Compiled rows are written with available_int=1, so they need explicit exclusion.

    Without this they land in the chunk candidate set and inflate a document's
    hit count.  Project convention: plain retrieval reads chunks, compiled
    products are served by their own tools.
    """
    retrieval = AsyncMock(return_value={"chunks": [{"doc_id": "doc-a", "similarity": 0.5}]})
    module, _, _ = _load_nav_module(monkeypatch, retriever=SimpleNamespace(retrieval=retrieval))

    await module._search_layers_chunk_agg("tenant-1", "kb-1", "q", 12, None)
    assert retrieval.call_args.kwargs["must_not"] == {"exists": "compile_kwd"}


@pytest.mark.asyncio
async def test_chunk_agg_router_can_include_compiled_rows(monkeypatch):
    """Flipping the flag lets compiled rows A/B as extra per-document entries."""
    retrieval = AsyncMock(return_value={"chunks": [{"doc_id": "doc-a", "similarity": 0.5}]})
    module, _, _ = _load_nav_module(monkeypatch, retriever=SimpleNamespace(retrieval=retrieval))
    monkeypatch.setattr(module, "_NAV_CHUNK_AGG_EXCLUDE_COMPILED", False)

    await module._search_layers_chunk_agg("tenant-1", "kb-1", "q", 12, None)
    assert retrieval.call_args.kwargs["must_not"] is None


async def test_chunk_agg_router_focuses_to_limit(monkeypatch):
    """chunk_agg must cap the returned documents like nav_doc does.

    Without the focus cap this router returned every document above the floor,
    routing the RAGAgent into 3x the evidence and inflating every dynamic LLM
    call — the same regression the nav_doc router had.
    """
    retriever = SimpleNamespace(retrieval=AsyncMock(return_value={"chunks": [{"doc_id": f"doc-{i}", "similarity": s} for i, s in [(5, 0.1), (1, 0.9), (3, 0.4), (2, 0.6), (4, 0.2)]]}))
    module, _, _ = _load_nav_module(monkeypatch, retriever=retriever)
    monkeypatch.setattr(module, "_NAV_DOC_FOCUS_LIMIT", 3)
    kb = SimpleNamespace(tenant_id="tenant-1", id="kb-1")

    ok, payload = await module._search_layers_chunk_agg("tenant-1", "kb-1", "q", 12, None, kb)
    assert ok is True
    # Cap to the focus limit even when top_k is higher.
    assert payload["total"] <= 3
    assert len(payload["items"]) <= 3


@pytest.mark.asyncio
async def test_chunk_agg_router_focus_limit_overrides_top_k(monkeypatch):
    """Focus wins over the caller's top_k — top_k=12 must not widen routing."""
    retriever = SimpleNamespace(retrieval=AsyncMock(return_value={"chunks": [{"doc_id": f"doc-{i}", "similarity": 0.9 - i * 0.1} for i in range(8)]}))
    module, _, _ = _load_nav_module(monkeypatch, retriever=retriever)
    monkeypatch.setattr(module, "_NAV_DOC_FOCUS_LIMIT", 2)
    kb = SimpleNamespace(tenant_id="tenant-1", id="kb-1")

    ok, payload = await module._search_layers_chunk_agg("tenant-1", "kb-1", "q", 12, None, kb)
    assert ok is True
    assert payload["total"] == 2


@pytest.mark.asyncio
async def test_chunk_mode_page_size_stays_bounded(monkeypatch):
    """Chunk mode returns chunks AS evidence, so page_size is the result size.

    Regression: it once shared the chunk_agg aggregation pool (256), which
    inflated the evidence pool ~7x (25 -> 191 chunks/question), bloated every
    downstream LLM prompt and exhausted the 180s research budget on 15/16
    questions.
    """
    retrieval = AsyncMock(return_value={"chunks": [{"doc_id": "doc-a", "similarity": 0.5}]})
    module, _, _ = _load_nav_module(monkeypatch, retriever=SimpleNamespace(retrieval=retrieval))
    kb = SimpleNamespace(tenant_id="tenant-1", id="kb-1")

    await module._search_layers_chunks("tenant-1", "kb-1", "q", 12, None, kb)
    args, kw = retrieval.call_args
    # Historical fetch size, not the aggregation pool.
    assert args[5] == 12 * 3
    # The candidate pool still has to cover the returned page.
    assert kw["rerank_candidates_count"] >= args[5]


@pytest.mark.asyncio
async def test_chunk_agg_router_survives_large_top_k(monkeypatch):
    """Regression: a top_k that pushes page_size past the default pool used to raise."""
    retrieval = AsyncMock(return_value={"chunks": [{"doc_id": "doc-a", "similarity": 0.5}]})
    module, _, _ = _load_nav_module(monkeypatch, retriever=SimpleNamespace(retrieval=retrieval))

    ok, payload = await module._search_layers_chunk_agg("tenant-1", "kb-1", "q", 100, None)
    assert ok is True
    args, kw = retrieval.call_args
    assert args[4] * args[5] <= kw["rerank_candidates_count"]


@pytest.mark.asyncio
async def test_chunk_agg_router_falls_back_to_terms_without_embeddings(monkeypatch):
    """Without an embedding model the dense leg is dropped, not the router."""
    retrieval = AsyncMock(return_value={"chunks": [{"doc_id": "doc-a", "similarity": 0.5}]})
    module, _, _ = _load_nav_module(monkeypatch, retriever=SimpleNamespace(retrieval=retrieval))

    ok, payload = await module._search_layers_chunk_agg("tenant-1", "kb-1", "q", 5, None)
    assert ok is True
    assert payload["total"] == 1
    assert retrieval.call_args.args[7] == 0


@pytest.mark.asyncio
async def test_chunk_agg_router_returns_empty_when_no_chunks(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    ok, payload = await module._search_layers_chunk_agg("tenant-1", "kb-1", "q", 5, None)
    assert ok is True
    assert payload == {"mode": "navigation_tree", "total": 0, "items": []}


async def test_nav_doc_summaries_batch_loads_descriptions(monkeypatch):
    module, _, doc_store = _load_nav_module(
        monkeypatch,
        field_map={
            "row-1": {
                "name": "leaf-a",
                "type_kwd": "nav_doc",
                "content_with_weight": json.dumps({"description": "Summary A"}),
                "doc_id": "doc-a",
            },
            "row-2": {
                "name": "leaf-b",
                "type_kwd": "nav_doc",
                "content_with_weight": json.dumps({"description": "Summary B"}),
                "doc_id": "doc-b",
            },
        },
    )
    kb = SimpleNamespace(tenant_id="tenant-1", id="kb-1")
    summaries = await module._nav_doc_summaries(kb, ["doc-a", "doc-b"])
    assert summaries == {"doc-a": "Summary A", "doc-b": "Summary B"}
    # One query for the whole batch, not one per document.
    assert doc_store.search.call_count == 1
    condition = doc_store.search.call_args.kwargs["condition"]
    assert condition["type_kwd"] == ["nav_doc"]
    assert condition["doc_id"] == ["doc-a", "doc-b"]

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
    _stub(monkeypatch, "common.doc_store.doc_store_base", OrderByExpr=MagicMock)

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
# Document routing via compiled page_index rows
# ---------------------------------------------------------------------------


def _compiled_row(doc_id, rtype, similarity, name="n", payload=None):
    body = payload if payload is not None else {"type": rtype, "name": name, "description": "d"}
    return {"doc_id": doc_id, "similarity": similarity, "content_with_weight": json.dumps(body)}


def test_nav_bucket_compiled_rows_splits_title_and_fact(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    buckets = module._nav_bucket_compiled_rows(
        {
            "r1": _compiled_row("doc-a", "title", 0.9),
            "r2": _compiled_row("doc-a", "fact", 0.5),
            "r3": _compiled_row("doc-a", "conclusion", 0.4),
            "r4": _compiled_row("doc-b", "title", 0.7),
            "r5": _compiled_row("doc-c", "relation", 0.6),  # non-entity type — dropped
        }
    )
    assert set(buckets["title"]) == {"doc-a", "doc-b"}
    assert set(buckets["fact"]) == {"doc-a"}
    # conclusion shares the fact bucket, so doc-a has two fact-class hits.
    assert buckets["fact"]["doc-a"]["hits"] == 2
    assert buckets["fact"]["doc-a"]["best"] == pytest.approx(0.5)


def test_nav_bucket_compiled_rows_skips_unparseable_payloads(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    buckets = module._nav_bucket_compiled_rows(
        {
            "r1": {"doc_id": "doc-a", "similarity": 0.9, "content_with_weight": "{not json"},
            "r2": {"doc_id": "", "similarity": 0.9, "content_with_weight": json.dumps({"type": "title"})},
            "r3": {"doc_id": "doc-b", "similarity": 0.9, "content_with_weight": json.dumps([1, 2])},
        }
    )
    assert buckets == {"title": {}, "fact": {}, "fact_names": {}}


def test_nav_bucket_compiled_rows_tracks_fact_names_per_document(monkeypatch):
    """Fact names must stay per-document or the bridge labels the wrong document."""
    module, _, _ = _load_nav_module(monkeypatch)
    buckets = module._nav_bucket_compiled_rows(
        {
            "r1": _compiled_row("doc-a", "fact", 0.9, name="A1"),
            "r2": _compiled_row("doc-a", "fact", 0.8, name="A2"),
            "r3": _compiled_row("doc-b", "fact", 0.7, name="B1"),
        }
    )
    assert buckets["fact_names"] == {"doc-a": ["A1", "A2"], "doc-b": ["B1"]}


def test_nav_matched_sections_dedups_and_keeps_hit_order(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    bridge = {"b1": "Discussion", "a1": "Methods", "a2": "Methods"}
    out = module._nav_matched_sections(bridge, ["a1", "b1", "a2"])
    assert out == ["Methods", "Discussion"]


def test_nav_matched_sections_omits_unbridged_facts(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    assert module._nav_matched_sections({}, ["a1"]) == []


@pytest.mark.asyncio
async def test_compiled_agg_attaches_matched_sections(monkeypatch):
    """Routing must hand back a document-internal entry point, not just a doc_id."""
    module, _, doc_store = _load_nav_module(
        monkeypatch,
        field_map={"r1": _compiled_row("doc-a", "fact", 0.9, name="F1")},
    )
    doc_store.get_fields = MagicMock(return_value={"r1": _compiled_row("doc-a", "fact", 0.9, name="F1")})
    monkeypatch.setattr(module, "_nav_bridge_sections", AsyncMock(return_value={"f1": "Methods"}))
    module.settings.retriever = SimpleNamespace(
        get_vector=AsyncMock(return_value=SimpleNamespace()),
        _existing_doc_ids=AsyncMock(return_value={"doc-a"}),
    )
    kb = SimpleNamespace(tenant_id="tenant-1", id="kb-1")

    ok, payload = await module._search_layers_compiled_agg("tenant-1", "kb-1", "q", 5, SimpleNamespace(), kb)
    assert ok is True
    assert payload["items"][0]["_nav"]["matched_sections"] == ["Methods"]


def test_nav_rank_compiled_buckets_rewards_both_legs(monkeypatch):
    """A document hit by title AND fact outranks one hit by only one leg."""
    module, _, _ = _load_nav_module(monkeypatch)
    buckets = module._nav_bucket_compiled_rows(
        {
            "r1": _compiled_row("both", "title", 0.8),
            "r2": _compiled_row("both", "fact", 0.8),
            "r3": _compiled_row("titleonly", "title", 0.9),
        }
    )
    ranked = module._nav_rank_compiled_buckets(buckets, 10)
    assert [d for d, _ in ranked] == ["both", "titleonly"]
    assert ranked[0][1]["legs"] == {"title", "fact"}
    # best stays on the 0..1 cosine scale callers threshold on.
    assert ranked[0][1]["best"] == pytest.approx(0.8)


def test_nav_rank_compiled_buckets_does_not_mutate_input(monkeypatch):
    """Regression: ranking used to attach 'legs' onto the caller's bucket entries."""
    module, _, _ = _load_nav_module(monkeypatch)
    buckets = module._nav_bucket_compiled_rows({"r1": _compiled_row("doc-a", "title", 0.8)})
    before = json.dumps(buckets, sort_keys=True, default=str)
    module._nav_rank_compiled_buckets(buckets, 10)
    assert json.dumps(buckets, sort_keys=True, default=str) == before


def test_nav_rank_compiled_buckets_respects_top_k(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    buckets = module._nav_bucket_compiled_rows({f"r{i}": _compiled_row(f"doc-{i}", "fact", 0.9 - i * 0.1) for i in range(5)})
    assert len(module._nav_rank_compiled_buckets(buckets, 2)) == 2


@pytest.mark.asyncio
async def test_compiled_agg_router_uses_keyword_fallback_without_embedding(monkeypatch):
    """No embedding model: the leg degrades to a keyword match, it does not drop out."""
    module, _, doc_store = _load_nav_module(
        monkeypatch,
        field_map={"r1": _compiled_row("doc-a", "fact", 0.5)},
    )
    doc_store.get_fields = MagicMock(return_value={"r1": _compiled_row("doc-a", "fact", 0.5)})
    kb = SimpleNamespace(tenant_id="tenant-1", id="kb-1")

    ok, payload = await module._search_layers_compiled_agg("tenant-1", "kb-1", "q", 5, None, kb)
    assert ok is True
    assert [i["doc_id"] for i in payload["items"]] == ["doc-a"]
    # _nav_doc_summaries issues a second search, so read the first one.
    exprs = doc_store.search.call_args_list[0].kwargs["match_expressions"]
    # A keyword-match expr, not a dense one.
    assert len(exprs) == 1
    assert isinstance(exprs[0], _StubMatchTextExpr)
    assert exprs[0].fields == ["content_ltks", "content_sm_ltks"]


@pytest.mark.asyncio
async def test_compiled_agg_router_filters_deleted_documents(monkeypatch):
    """Compiled rows can outlive deleted docs; the leg must check existence itself."""
    module, _, doc_store = _load_nav_module(
        monkeypatch,
        field_map={"r1": _compiled_row("doc-gone", "fact", 0.9)},
    )
    doc_store.get_fields = MagicMock(return_value={"r1": _compiled_row("doc-gone", "fact", 0.9)})
    retriever = SimpleNamespace(
        get_vector=AsyncMock(return_value=SimpleNamespace()),
        _existing_doc_ids=AsyncMock(return_value=set()),  # nothing survives
    )
    module, _, _ = _load_nav_module(monkeypatch, field_map={"r1": _compiled_row("doc-gone", "fact", 0.9)}, retriever=retriever)
    kb = SimpleNamespace(tenant_id="tenant-1", id="kb-1")

    ok, payload = await module._search_layers_compiled_agg("tenant-1", "kb-1", "q", 5, SimpleNamespace(), kb)
    assert ok is True
    assert payload["total"] == 0


@pytest.mark.asyncio
async def test_compiled_agg_router_builds_vector_with_named_candidates(monkeypatch):
    """Regression: positional passing put the similarity threshold into num_candidates."""
    get_vector = AsyncMock(return_value=SimpleNamespace())
    module, _, doc_store = _load_nav_module(
        monkeypatch,
        field_map={"r1": _compiled_row("doc-a", "fact", 0.9)},
        retriever=SimpleNamespace(get_vector=get_vector, _existing_doc_ids=AsyncMock(return_value={"doc-a"})),
    )
    doc_store.get_fields = MagicMock(return_value={"r1": _compiled_row("doc-a", "fact", 0.9)})
    kb = SimpleNamespace(tenant_id="tenant-1", id="kb-1")

    await module._search_layers_compiled_agg("tenant-1", "kb-1", "q", 5, SimpleNamespace(), kb)
    kw = get_vector.call_args.kwargs
    assert kw["top_k"] == module._NAV_COMPILED_POOL
    # HNSW ef_search must be >= top_k, and must not receive the similarity value.
    assert kw["num_candidates"] == module._NAV_COMPILED_POOL
    assert kw["similarity"] == module._NAV_COMPILED_SIMILARITY
    # _nav_doc_summaries issues a second search, so read the first one.
    cond = doc_store.search.call_args_list[0].kwargs["condition"]
    # Compiled rows carry no available_int, so it must NOT be used as a filter.
    assert "available_int" not in cond
    assert cond["scope_kwd"] == ["doc"]
    assert cond["knowledge_graph_kwd"] == ["entity"]


# ---------------------------------------------------------------------------
# Fusion of the chunk leg and the compiled leg
# ---------------------------------------------------------------------------


def test_nav_fuse_legs_normalizes_each_leg(monkeypatch):
    """Each leg is normalized by its own peak so raw cosine cannot dominate."""
    module, _, _ = _load_nav_module(monkeypatch)
    ranked = module._nav_fuse_legs(
        [
            ("chunk", [{"doc_id": "a", "score": 0.9}, {"doc_id": "b", "score": 0.45}], 1.0),
            ("compiled", [{"doc_id": "c", "score": 0.6}], 1.0),
        ],
        10,
    )
    scores = {d: info["fused"] for d, info in ranked}
    # Peak of each leg becomes 1.0, so a document top-ranked by either leg ties.
    assert scores["a"] == pytest.approx(1.0)
    assert scores["c"] == pytest.approx(1.0)
    assert scores["b"] == pytest.approx(0.5)


def test_nav_fuse_legs_prefers_documents_hit_by_both(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    ranked = module._nav_fuse_legs(
        [
            ("chunk", [{"doc_id": "both", "score": 0.5}, {"doc_id": "chunkonly", "score": 1.0}], 1.0),
            ("compiled", [{"doc_id": "both", "score": 1.0}], 1.0),
        ],
        10,
    )
    assert [d for d, _ in ranked] == ["both", "chunkonly"]
    assert ranked[0][1]["legs"] == {"chunk", "compiled"}


def test_nav_fuse_legs_survives_empty_leg(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    ranked = module._nav_fuse_legs([("chunk", [], 1.0), ("compiled", [], 1.0)], 10)
    assert ranked == []


@pytest.mark.asyncio
async def test_fusion_router_falls_back_to_chunk_leg_without_compiled(monkeypatch):
    """A raptor KB has no page_index rows; the chunk leg must decide alone."""
    retrieval = AsyncMock(return_value={"chunks": [{"doc_id": "doc-a", "similarity": 0.5}]})
    module, _, _ = _load_nav_module(monkeypatch, retriever=SimpleNamespace(retrieval=retrieval))
    kb = SimpleNamespace(tenant_id="tenant-1", id="kb-1")

    ok, payload = await module._search_layers_fusion("tenant-1", "kb-1", "q", 5, None, kb, compiled={"items": []})
    assert ok is True
    assert [i["doc_id"] for i in payload["items"]] == ["doc-a"]
    # No compiled hits means no section bridge lookup either.
    assert payload["items"][0]["_agg"]["legs"] == ["chunk"]


# ---------------------------------------------------------------------------
# Document routing via chunk aggregation (PageIndex semantics recipe)
# ---------------------------------------------------------------------------


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


def test_nav_label_items_wraps_flat_row_for_the_caller(monkeypatch):
    """navigation.py reads the route label from item["_nav"]["description"].

    search_dataset_nav returns the leaf row flat, so without wrapping the route
    comes back unlabelled.
    """
    module, _, _ = _load_nav_module(monkeypatch)
    out = module._nav_label_items([{"doc_id": "doc-a", "name": "Leaf A", "description": "Summary A"}])
    assert out[0]["_nav"] == {"doc_id": "doc-a", "description": "Summary A"}


def test_nav_label_items_falls_back_to_name(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    out = module._nav_label_items([{"doc_id": "doc-a", "name": "Leaf A", "description": ""}])
    assert out[0]["_nav"]["description"] == "Leaf A"


def test_nav_label_items_preserves_existing_nav(monkeypatch):
    """A row that already carries _nav must not be overwritten."""
    module, _, _ = _load_nav_module(monkeypatch)
    out = module._nav_label_items([{"doc_id": "doc-a", "description": "flat", "_nav": {"description": "nested"}}])
    assert out[0]["_nav"]["description"] == "nested"


def test_nav_label_items_tolerates_non_dicts(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    assert module._nav_label_items(["junk", None]) == ["junk", None]


def test_nav_focus_items_sorts_and_truncates_to_limit(monkeypatch):
    """Regression: the flat sweep returned every row above the 0.2 floor.

    Main's cluster descent routes ~2 documents; the flat sweep returned 6-12,
    and routing to 3x the documents made the RAGAgent carry 3x the evidence in
    every round — each dynamic LLM call grew ~33% and per-question time ~25%.
    """
    module, _, _ = _load_nav_module(monkeypatch)
    monkeypatch.setattr(module, "_NAV_DOC_FOCUS_LIMIT", 3)
    # Not sorted — must sort by score before truncating.
    items = [{"doc_id": f"doc-{i}", "score": s} for i, s in [(6, 0.1), (1, 0.9), (3, 0.4), (2, 0.6)]]
    out = module._nav_focus_items(items)
    assert [i["doc_id"] for i in out] == ["doc-1", "doc-2", "doc-3"]
    # The input list is truncated in place.
    assert [i["doc_id"] for i in items] == ["doc-1", "doc-2", "doc-3"]


def test_nav_focus_items_keeps_fewer_than_limit_untouched(monkeypatch):
    """When few docs are relevant, do not pad — routing stays honest."""
    module, _, _ = _load_nav_module(monkeypatch)
    monkeypatch.setattr(module, "_NAV_DOC_FOCUS_LIMIT", 3)
    items = [{"doc_id": "doc-1", "score": 0.9}]
    out = module._nav_focus_items(items)
    assert len(out) == 1


def test_nav_focus_items_disabled_when_limit_is_zero(monkeypatch):
    module, _, _ = _load_nav_module(monkeypatch)
    monkeypatch.setattr(module, "_NAV_DOC_FOCUS_LIMIT", 0)
    items = [{"doc_id": f"doc-{i}", "score": s} for i, s in [(1, 0.9), (2, 0.6), (3, 0.4)]]
    out = module._nav_focus_items(items)
    assert len(out) == 3


@pytest.mark.asyncio
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

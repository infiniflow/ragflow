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
"""Regression tests for the ``get_wiki_graph`` completeness sweep (#19119).

The sweep must run for BOTH canvas modes: overview (Flow A) and click
expansion (Flow B). Entities hydrated only as relation to-targets must still
show their persisted outgoing edges — limited to edges whose ``to`` endpoint
is part of the loaded set (a to-target beyond the entity cap would dangle).
"""

import importlib.util
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import pytest

pytestmark = pytest.mark.p2

_WIKI_ENTITY_KWD = "wiki_entity"
_WIKI_RELATION_KWD = "wiki_relation"


def _entity_row(row_id: str, slug: str, weight: int, name: str, entity_type: str) -> dict:
    return {
        "id": row_id,
        "slug_kwd": slug,
        "weight_int": weight,
        "source_chunk_ids": [f"chunk-{row_id}"],
        "content_with_weight": json.dumps({"slug": slug, "name": name, "type": entity_type}),
    }


def _relation_row(row_id: str, src: str, tgt: str, rel_type: str) -> dict:
    return {
        "id": row_id,
        "from_kwd": src,
        "to_kwd": tgt,
        "content_with_weight": json.dumps({"from": src, "to": tgt, "type": rel_type}),
    }


# Store order emulates ``weight_int DESC``.
ENTITY_ROWS = {
    "ent-1": _entity_row("ent-1", "the-nightingale", 10, "The Nightingale", "work"),
    "ent-2": _entity_row("ent-2", "victorian-era", 2, "Victorian era", "concept"),
    "ent-3": _entity_row("ent-3", "oscar-wilde", 1, "Oscar Wilde", "person"),
}

RELATION_ROWS = {
    "rel-1": _relation_row("rel-1", "the-nightingale", "oscar-wilde", "author"),
    "rel-2": _relation_row("rel-2", "the-nightingale", "victorian-era", "subject"),
    # From a to-target-only entity: only the sweep ever pulls these.
    "rel-3": _relation_row("rel-3", "oscar-wilde", "victorian-era", "subject"),
    # Dangling: ``to`` is never part of the loaded entity set.
    "rel-4": _relation_row("rel-4", "oscar-wilde", "the-happy-prince", "author"),
}


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


def _make_doc_store(entity_rows: dict, relation_rows: dict):
    rows_by_kind = {_WIKI_ENTITY_KWD: entity_rows, _WIKI_RELATION_KWD: relation_rows}

    def _search(select_fields, highlights, condition, match_expressions, order_by, offset, limit, index_nm, tenant_ids):
        return {"kind": condition["compile_kwd"][0], "offset": offset, "limit": limit}

    def _get_fields(res, fields):
        rows = list(rows_by_kind[res["kind"]].items())
        return dict(rows[res["offset"] : res["offset"] + res["limit"]])

    doc_store = MagicMock()
    doc_store.search = MagicMock(side_effect=_search)
    doc_store.get_fields = MagicMock(side_effect=_get_fields)
    doc_store.get_total = MagicMock(side_effect=lambda res: len(rows_by_kind[res["kind"]]))
    return doc_store


def _load_wiki_graph_module(monkeypatch, *, accessible=True):
    doc_store = _make_doc_store(dict(ENTITY_ROWS), dict(RELATION_ROWS))

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
        RetCode=SimpleNamespace(),
        StatusEnum=SimpleNamespace(),
        TaskStatus=SimpleNamespace(),
        ModelTypeBinary=SimpleNamespace(),
    )
    _stub(monkeypatch, "common.settings", docStoreConn=doc_store, DOC_ENGINE="infinity")
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
    _stub(monkeypatch, "api.db.services.connector_service", Connector2KbService=SimpleNamespace(), SyncLogsService=SimpleNamespace())
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
    spec = importlib.util.spec_from_file_location("test_dataset_api_service_wiki_graph_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_dataset_api_service_wiki_graph_module", module)
    spec.loader.exec_module(module)
    monkeypatch.setattr(module, "_wiki_index_or_none", lambda _tenant_id, _dataset_id: ("idx-wiki", None))
    return module, doc_store


def _edges(payload: dict) -> set[tuple[str, str]]:
    return {(r["from"], r["to"]) for r in payload["relations"]}


@pytest.mark.asyncio
async def test_click_expansion_sweep_adds_edges_of_hydrated_targets(monkeypatch):
    module, _ = _load_wiki_graph_module(monkeypatch)

    ok, payload = await module.get_wiki_graph("kb-1", "tenant-1", node="the-nightingale")

    assert ok is True
    assert {e["slug"] for e in payload["entities"]} == {"the-nightingale", "oscar-wilde", "victorian-era"}
    edges = _edges(payload)
    # Centre's own outgoing edges.
    assert ("the-nightingale", "oscar-wilde") in edges
    assert ("the-nightingale", "victorian-era") in edges
    # Sweep-added: the hydrated author's edge to another loaded node.
    assert ("oscar-wilde", "victorian-era") in edges
    # Dangling to-target beyond the loaded set must stay out.
    assert ("oscar-wilde", "the-happy-prince") not in edges
    assert payload["total_relations"] == 4
    assert payload["returned_relations"] == 3


@pytest.mark.asyncio
async def test_overview_sweep_covers_step4_hydrated_targets(monkeypatch):
    module, _ = _load_wiki_graph_module(monkeypatch)
    # One entity per page: page 1 seeds only the heaviest node, so the author
    # is hydrated purely as a to-target and only the sweep ever sees rel-3.
    monkeypatch.setattr(module, "_WIKI_GRAPH_ENTITY_PAGE_SIZE", 1)

    ok, payload = await module.get_wiki_graph("kb-1", "tenant-1")

    assert ok is True
    assert {e["slug"] for e in payload["entities"]} == {"the-nightingale", "victorian-era", "oscar-wilde"}
    edges = _edges(payload)
    assert ("the-nightingale", "oscar-wilde") in edges
    assert ("the-nightingale", "victorian-era") in edges
    assert ("oscar-wilde", "victorian-era") in edges
    assert ("oscar-wilde", "the-happy-prince") not in edges


@pytest.mark.asyncio
async def test_overview_returns_entities_when_relation_store_fails(monkeypatch):
    module, doc_store = _load_wiki_graph_module(monkeypatch)
    original_search = doc_store.search.side_effect

    def _failing_search(select_fields, highlights, condition, *rest):
        if condition["compile_kwd"][0] == _WIKI_RELATION_KWD:
            raise RuntimeError("relation store down")
        return original_search(select_fields, highlights, condition, *rest)

    doc_store.search = MagicMock(side_effect=_failing_search)

    ok, payload = await module.get_wiki_graph("kb-1", "tenant-1")

    assert ok is True
    assert {e["slug"] for e in payload["entities"]} == {"the-nightingale", "victorian-era", "oscar-wilde"}
    assert payload["relations"] == []

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
"""Regression tests for the dataset search endpoints' doc-level filter.

``Dealer.get_filters`` distinguishes ``doc_ids=None`` ("no doc filter") from
``doc_ids=[]`` ("filter on an empty set of documents"): only the latter reaches
the document store as a ``doc_id`` condition, and no backend handles it usefully.
ES, OpenSearch, Infinity, OceanBase and SereneDB all skip falsy condition values,
so the filter is dropped and the search silently widens to the whole dataset;
GaussDB's ``_list_values`` raises ``ValueError("empty condition values are not
supported")``.  A request that omitted ``doc_ids`` -- or supplied nothing but
blank ids -- must therefore forward ``None``, not ``[]``.
"""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock

import pytest

# Imported before the loader stubs ``common.metadata_utils`` so the call sites
# exercise the real normalization rather than a test double.
from common.metadata_utils import normalize_doc_id_filter

pytestmark = pytest.mark.p2


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


class _StubKB:
    id = "kb-1"
    tenant_id = "tenant-1"
    embd_id = "embd-1"
    parser_id = "naive"


def _load_search_module(monkeypatch, retrieval_calls):
    async def _retrieval(*args, **kwargs):
        retrieval_calls.append({"args": args, "kwargs": kwargs})
        return {"total": 0, "chunks": [], "doc_aggs": []}

    _stub(monkeypatch, "api.apps", __path__=[])
    _stub(monkeypatch, "api.apps.services", __path__=[])
    _stub(monkeypatch, "api.apps.services.structure_graph_common")
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_composite_model_name_by_ids=MagicMock(),
        get_tenant_default_model_by_type=lambda *_args, **_kwargs: {"model_type": "chat"},
        resolve_model_config=lambda *_args, **_kwargs: {"model_type": "embedding"},
        resolve_model_id=MagicMock(),
    )
    _stub(
        monkeypatch,
        "common.constants",
        PAGERANK_FLD="pagerank",
        LLMType=SimpleNamespace(CHAT="chat", EMBEDDING="embedding", RERANK="rerank"),
        FileSource=SimpleNamespace(KNOWLEDGEBASE="knowledgebase"),
        PipelineTaskType=SimpleNamespace(),
        StatusEnum=SimpleNamespace(),
        TaskStatus=SimpleNamespace(SCHEDULE="schedule", RUNNING="running", CANCEL="cancel"),
        RetCode=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "common.settings",
        docStoreConn=SimpleNamespace(),
        retriever=SimpleNamespace(
            retrieval=_retrieval,
            retrieval_by_children=lambda chunks, _tenant_ids: chunks,
        ),
    )
    _stub(
        monkeypatch,
        "api.db.db_models",
        Connector2Kb=SimpleNamespace(kb_id="kb_id"),
        Document=SimpleNamespace(kb_id="kb_id"),
        File=SimpleNamespace(),
        SyncLogs=SimpleNamespace(kb_id="kb_id"),
    )
    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(),
        queue_raptor_o_graphrag_tasks=MagicMock(),
    )
    _stub(monkeypatch, "api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace(get_flatted_meta_by_kbs=lambda _kb_ids: {}))
    _stub(monkeypatch, "api.db.services.file2document_service", File2DocumentService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.file_service", FileService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=lambda *_args, **_kwargs: SimpleNamespace(max_length=4096))
    _stub(monkeypatch, "api.db.services.search_service", SearchService=SimpleNamespace(get_detail=lambda _search_id: None))
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(
            accessible=lambda _dataset_id, _tenant_id: True,
            get_by_id=lambda _dataset_id: (True, _StubKB()),
            get_by_ids=lambda _dataset_ids: [_StubKB()],
            query=lambda **_kwargs: [_StubKB()],
        ),
        validate_dataset_embedding_models=lambda _kbs: None,
    )
    _stub(
        monkeypatch,
        "api.db.services.connector_service",
        Connector2KbService=SimpleNamespace(),
        SyncLogsService=SimpleNamespace(),
    )
    _stub(monkeypatch, "api.db.services.task_service", GRAPH_RAPTOR_FAKE_DOC_ID="fake-doc", TaskService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.tenant_model_service", TenantModelService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.user_service",
        TenantService=SimpleNamespace(),
        UserService=SimpleNamespace(),
        UserTenantService=SimpleNamespace(query=lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")]),
    )
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        deep_merge=MagicMock(),
        get_parser_config=MagicMock(),
        remap_dictionary_keys=lambda data: data,
        verify_embedding_availability=MagicMock(),
    )
    _stub(monkeypatch, "common.misc_utils", thread_pool_exec=MagicMock(), thread_pool_exec_long_time=MagicMock())
    _stub(monkeypatch, "common.metadata_utils", apply_meta_data_filter=MagicMock(), normalize_doc_id_filter=normalize_doc_id_filter)
    _stub(monkeypatch, "rag.app", __path__=[])
    _stub(monkeypatch, "rag.app.tag", label_question=lambda _question, _kbs: ["label-1"])
    _stub(monkeypatch, "rag.prompts", __path__=[])
    _stub(monkeypatch, "rag.prompts.generator", cross_languages=MagicMock(), keyword_extraction=MagicMock())
    _stub(monkeypatch, "rag.advanced_rag", __path__=[])
    _stub(monkeypatch, "rag.advanced_rag.knowlege_compile", __path__=[])
    _stub(monkeypatch, "rag.advanced_rag.knowlege_compile.wiki", WIKI_PAGE_COMPILE_KWD="wiki", _chunk_hash=lambda _content: "stub-hash")

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "services" / "dataset_api_service.py"
    spec = importlib.util.spec_from_file_location("test_dataset_api_service_search_doc_ids_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_dataset_api_service_search_doc_ids_module", module)
    spec.loader.exec_module(module)
    return module


def test_search_without_doc_ids_forwards_none(monkeypatch):
    retrieval_calls = []
    module = _load_search_module(monkeypatch, retrieval_calls)

    ok, ranks = asyncio.run(module.search("kb-1", "tenant-1", {"question": "hello"}))

    assert ok, ranks
    assert len(retrieval_calls) == 1
    assert retrieval_calls[0]["kwargs"]["doc_ids"] is None


def test_search_with_doc_ids_forwards_non_blank_ids(monkeypatch):
    retrieval_calls = []
    module = _load_search_module(monkeypatch, retrieval_calls)

    ok, ranks = asyncio.run(module.search("kb-1", "tenant-1", {"question": "hello", "doc_ids": ["doc-1", "", "doc-2"]}))

    assert ok, ranks
    assert retrieval_calls[0]["kwargs"]["doc_ids"] == ["doc-1", "doc-2"]


def test_search_with_only_blank_doc_ids_forwards_none(monkeypatch):
    """All-blank ids must collapse to ``None``, not to an empty active filter."""
    retrieval_calls = []
    module = _load_search_module(monkeypatch, retrieval_calls)

    ok, ranks = asyncio.run(module.search("kb-1", "tenant-1", {"question": "hello", "doc_ids": ["", ""]}))

    assert ok, ranks
    assert retrieval_calls[0]["kwargs"]["doc_ids"] is None


def test_search_with_meta_data_filter_forwards_filtered_ids(monkeypatch):
    """The ``if meta_data_filter:`` guard is what keeps ``None`` from becoming ``[]``.

    ``apply_meta_data_filter`` starts from ``list(base_doc_ids) if base_doc_ids
    else []``, so reaching it unconditionally would silently convert a ``None``
    back into an empty active filter. It must only run when a filter is set, and
    when it does run its result is what reaches the retriever.
    """
    retrieval_calls = []
    module = _load_search_module(monkeypatch, retrieval_calls)

    seen = {}

    async def _apply(_filter, _metas, _question, _chat_mdl, base_doc_ids, **_kwargs):
        seen["base_doc_ids"] = base_doc_ids
        return ["doc-from-metadata"]

    monkeypatch.setattr(sys.modules["common.metadata_utils"], "apply_meta_data_filter", _apply)

    ok, ranks = asyncio.run(
        module.search("kb-1", "tenant-1", {"question": "hello", "meta_data_filter": {"method": "manual", "conditions": []}}),
    )

    assert ok, ranks
    assert seen["base_doc_ids"] is None
    assert retrieval_calls[0]["kwargs"]["doc_ids"] == ["doc-from-metadata"]


def test_search_datasets_without_doc_ids_forwards_none(monkeypatch):
    retrieval_calls = []
    module = _load_search_module(monkeypatch, retrieval_calls)

    ok, ranks = asyncio.run(module.search_datasets("tenant-1", {"dataset_ids": ["kb-1"], "question": "hello"}))

    assert ok, ranks
    assert len(retrieval_calls) == 1
    assert retrieval_calls[0]["kwargs"]["doc_ids"] is None


def test_search_datasets_with_doc_ids_forwards_non_blank_ids(monkeypatch):
    retrieval_calls = []
    module = _load_search_module(monkeypatch, retrieval_calls)

    ok, ranks = asyncio.run(module.search_datasets("tenant-1", {"dataset_ids": ["kb-1"], "question": "hello", "doc_ids": ["doc-1", "", "doc-2"]}))

    assert ok, ranks
    assert retrieval_calls[0]["kwargs"]["doc_ids"] == ["doc-1", "doc-2"]

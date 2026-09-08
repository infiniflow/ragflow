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
"""metadata_condition handling in POST /api/v1/retrieval (chunk_api.retrieval_test).

A metadata_condition with an empty conditions list must not filter anything:
the Go search service (internal/service/metadata_filter.go) returns the base
doc ids unchanged when the manual filter list is empty, but the Python route
scoped the search to the nonexistent doc "-999" and returned zero chunks.
These tests stub the HTTP/service layer and drive the real handler.
"""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _AwaitableValue:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _co():
            return self._value

        return _co().__await__()


def _run(coro):
    return asyncio.run(coro)


RETRIEVAL_CALLS: list = []


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    # Overrides the live-server login fixture from conftest.py; these tests
    # drive the handler with stubbed services and need no server.
    return None


def _load_module(monkeypatch, request_payload):
    repo_root = Path(__file__).resolve().parents[3]
    RETRIEVAL_CALLS.clear()

    def _pkg(name, path=None):
        mod = ModuleType(name)
        if path:
            mod.__path__ = [str(path)]
        monkeypatch.setitem(sys.modules, name, mod)
        return mod

    def _stub(name, **attrs):
        mod = ModuleType(name)
        for key, value in attrs.items():
            setattr(mod, key, value)
        monkeypatch.setitem(sys.modules, name, mod)
        return mod

    _stub("quart", request=SimpleNamespace(args={}))

    api_pkg = _pkg("api", repo_root / "api")
    apps_mod = _pkg("api.apps", repo_root / "api" / "apps")
    apps_mod.login_required = lambda func: func
    apps_mod.current_user = SimpleNamespace(id="user-1")
    api_pkg.apps = apps_mod

    _pkg("api.apps.services", repo_root / "api" / "apps" / "services")
    _stub("api.apps.services.structure_graph_common")

    _pkg("api.db", repo_root / "api" / "db")
    _stub("api.db.db_models", Document=SimpleNamespace, Task=SimpleNamespace)
    _pkg("api.db.joint_services", repo_root / "api" / "db" / "joint_services")
    _stub(
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=lambda *a, **k: {},
        resolve_model_config=lambda *a, **k: {},
    )
    _pkg("api.db.services", repo_root / "api" / "db" / "services")
    _stub("api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace(get_flatted_meta_by_kbs=staticmethod(lambda _kb_ids: {}), filter_doc_ids_by_meta_pushdown=staticmethod(lambda *a, **k: None)))
    _stub("api.db.services.document_counter_service", release_reparse_counters=lambda *a, **k: None)
    _stub("api.db.services.document_service", DocumentService=SimpleNamespace())

    kb = SimpleNamespace(tenant_id="tenant-1", embd_id="embd-1", tenant_embd_id=None)
    kb_svc = SimpleNamespace(
        accessible=staticmethod(lambda kb_id, user_id: True),
        get_by_ids=staticmethod(lambda ids: [kb]),
        get_by_id=staticmethod(lambda _id: (True, kb)),
        list_documents_by_ids=staticmethod(lambda _kb_ids: ["doc-1", "doc-2"]),
    )
    _stub("api.db.services.knowledgebase_service", KnowledgebaseService=kb_svc, validate_dataset_embedding_models=lambda _kbs: None)
    _stub("api.db.services.llm_service", LLMBundle=lambda *a, **k: SimpleNamespace())
    _stub("api.db.services.search_service", SearchService=SimpleNamespace())
    _stub("api.db.services.task_service", TaskService=SimpleNamespace(), cancel_all_task_of=lambda *a, **k: None)
    _stub("api.db.services.tenant_llm_service", TenantLLMService=SimpleNamespace())

    _stub("common.llm_request_context", normalize_llm_user_id=lambda x: x, reset_llm_request_context=lambda *a: None, set_llm_request_context=lambda **k: None)

    _pkg("api.utils", repo_root / "api" / "utils")

    def get_result(data=None, message="success", code=0):
        return {"code": code, "data": data, "message": message}

    utils_mod = ModuleType("api.utils.api_utils")
    utils_mod.add_tenant_id_to_kwargs = lambda func: (lambda *a, **kw: func("tenant-1", *a, **kw))
    utils_mod.check_duplicate_ids = lambda ids, _name: (ids, None)
    utils_mod.construct_json_result = get_result
    utils_mod.get_error_data_result = lambda message="", code=102: {"code": code, "message": message}
    utils_mod.get_request_json = lambda: _AwaitableValue(request_payload)
    utils_mod.get_result = get_result
    utils_mod.server_error_response = lambda e: {"code": 500, "message": str(e)}
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", utils_mod)

    _stub("api.utils.image_utils", store_chunk_image=lambda *a, **k: None)
    _stub(
        "api.utils.pagination_utils",
        DEFAULT_PAGE=1,
        DEFAULT_PAGE_SIZE=30,
        validate_rest_api_ids=lambda ids, _name: None,
        validate_rest_api_page=lambda v: int(v),
        validate_rest_api_page_size=lambda v: int(v),
    )
    _stub("api.utils.reference_metadata_utils", resolve_reference_metadata_preferences=lambda req, cfg=None: (False, None), enrich_chunks_with_document_metadata=lambda *a, **k: None)

    common_pkg = _pkg("common", repo_root / "common")
    constants = _stub("common.constants")
    constants.LLMType = SimpleNamespace(CHAT="chat", EMBEDDING="embedding", RERANK="rerank")
    constants.ParserType = SimpleNamespace()
    constants.RetCode = SimpleNamespace(ARGUMENT_ERROR=101, DATA_ERROR=102)
    constants.TaskStatus = SimpleNamespace()

    async def thread_pool_exec(fn, *a, **kw):
        return fn(*a, **kw)

    _stub("common.misc_utils", thread_pool_exec=thread_pool_exec)

    class _Retriever:
        async def retrieval(self, *args, **kwargs):
            RETRIEVAL_CALLS.append({"args": args, "kwargs": kwargs})
            return {"total": 1, "chunks": [{"chunk_id": "c1", "doc_id": "doc-1", "kb_id": "kb-1", "content_with_weight": "text", "docnm_kwd": "doc"}], "doc_aggs": {}}

        def retrieval_by_children(self, chunks, _tenant_ids):
            return chunks

    settings_mod = _stub("common.settings", retriever=_Retriever(), kg_retriever=SimpleNamespace())
    common_pkg.settings = settings_mod

    _stub("common.string_utils", is_content_empty=lambda s: not s, remove_redundant_spaces=lambda s: s)
    _stub("common.tag_feature_utils", validate_tag_features=lambda *a, **k: None)
    _pkg("common.doc_store", repo_root / "common" / "doc_store")
    _stub("common.doc_store.doc_store_base", OrderByExpr=SimpleNamespace)

    rag_pkg = _pkg("rag", repo_root / "rag")
    _pkg("rag.app", repo_root / "rag" / "app")
    _stub("rag.app.tag", label_question=lambda *a, **k: {})
    nlp_pkg = _pkg("rag.nlp", repo_root / "rag" / "nlp")
    nlp_pkg.search = SimpleNamespace(index_name=lambda tid: f"idx-{tid}")
    _pkg("rag.prompts", repo_root / "rag" / "prompts")
    _stub("rag.prompts.generator", cross_languages=lambda *a, **k: None, keyword_extraction=lambda *a, **k: None)
    rag_pkg.nlp = nlp_pkg

    spec = importlib.util.spec_from_file_location("api.apps.restful_apis.chunk_api", repo_root / "api" / "apps" / "restful_apis" / "chunk_api.py")
    module = importlib.util.module_from_spec(spec)
    pkg_name = "api.apps.restful_apis"
    if pkg_name not in sys.modules:
        pkg = ModuleType(pkg_name)
        pkg.__path__ = [str(repo_root / "api" / "apps" / "restful_apis")]
        monkeypatch.setitem(sys.modules, pkg_name, pkg)
    monkeypatch.setitem(sys.modules, "api.apps.restful_apis.chunk_api", module)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


def _payload(**extra):
    base = {"dataset_ids": ["kb-1"], "question": "what is ragflow", "page": 1, "page_size": 10}
    base.update(extra)
    return base


@pytest.mark.p2
def test_empty_metadata_conditions_do_not_filter(monkeypatch):
    module = _load_module(monkeypatch, _payload(metadata_condition={"conditions": [], "logic": "and"}))
    res = _run(module.retrieval_test())
    assert res["code"] == 0, res
    doc_ids = RETRIEVAL_CALLS[0]["kwargs"]["doc_ids"]
    assert doc_ids is None, f"empty conditions must leave the search unfiltered, got doc_ids={doc_ids!r}"


@pytest.mark.p2
def test_empty_metadata_conditions_keep_explicit_document_ids(monkeypatch):
    module = _load_module(monkeypatch, _payload(document_ids=["doc-1"], metadata_condition={"conditions": [], "logic": "and"}))
    res = _run(module.retrieval_test())
    assert res["code"] == 0, res
    doc_ids = RETRIEVAL_CALLS[0]["kwargs"]["doc_ids"]
    assert doc_ids == ["doc-1"], f"empty conditions must keep the caller's document_ids, got {doc_ids!r}"


@pytest.mark.p2
def test_nonempty_metadata_condition_with_no_match_returns_empty(monkeypatch):
    # The dataset's metadata index is empty in this harness, so any real
    # condition matches nothing and the existing empty-result contract holds.
    module = _load_module(monkeypatch, _payload(metadata_condition={"conditions": [{"name": "author", "comparison_operator": "is", "value": "x"}], "logic": "and"}))
    res = _run(module.retrieval_test())
    assert res["code"] == 0, res
    assert res["data"] == {"total": 0, "chunks": [], "doc_aggs": {}}
    assert RETRIEVAL_CALLS == []

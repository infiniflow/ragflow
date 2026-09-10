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

"""Unit tests for the chunk `available`/`available_int` parsing.

A non-integer value must produce a data error (400-class envelope)
instead of a ValueError that surfaces as a 500, consistent with the
type checks on the sibling fields (`positions`, `tag_kwd`, `tag_feas`).
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


def _module_stub(name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    return mod


REQUEST_JSON: dict = {}


def _load_chunk_api(monkeypatch):
    repo_root = Path(__file__).resolve().parents[5]

    kb_service = SimpleNamespace(
        accessible=lambda **_kw: True,
        get_by_id=lambda _id: (True, SimpleNamespace(tenant_id="tenant-1")),
    )
    document_service = SimpleNamespace(
        query=lambda **_kw: [SimpleNamespace(id="doc-1", kb_id="ds-1")],
        get_embd_id=lambda _id: "embd-1",
    )
    doc_store = SimpleNamespace(
        get=lambda *_a, **_k: {"id": "chunk-1", "doc_id": "doc-1", "document_id": "doc-1", "content_with_weight": "hello"},
    )
    settings_stub = SimpleNamespace(docStoreConn=doc_store)
    rag_tokenizer = SimpleNamespace(tokenize=lambda s: "tok", fine_grained_tokenize=lambda s: "fine")
    search_stub = SimpleNamespace(index_name=lambda _t: "idx")

    monkeypatch.setitem(sys.modules, "xxhash", _module_stub("xxhash"))
    monkeypatch.setitem(sys.modules, "quart", _module_stub("quart", request=SimpleNamespace(args={})))
    api_apps_stub = _module_stub("api.apps", login_required=lambda f: f)
    api_apps_stub.__path__ = []
    monkeypatch.setitem(sys.modules, "api.apps", api_apps_stub)
    monkeypatch.setitem(sys.modules, "api.apps.services", _module_stub("api.apps.services", __path__=[]))
    monkeypatch.setitem(sys.modules, "api.apps.services.structure_graph_common", _module_stub("api.apps.services.structure_graph_common"))
    monkeypatch.setitem(sys.modules, "api.db.db_models", _module_stub("api.db.db_models", Document=None, Task=None))
    monkeypatch.setitem(sys.modules, "api.db.joint_services.tenant_model_service", _module_stub("api.db.joint_services.tenant_model_service", get_tenant_default_model_by_type=None, resolve_model_config=None))
    monkeypatch.setitem(sys.modules, "api.db.services.doc_metadata_service", _module_stub("api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.services.document_counter_service", _module_stub("api.db.services.document_counter_service", release_reparse_counters=None))
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", _module_stub("api.db.services.document_service", DocumentService=document_service))
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", _module_stub("api.db.services.knowledgebase_service", KnowledgebaseService=kb_service, validate_dataset_embedding_models=None))
    monkeypatch.setitem(sys.modules, "api.db.services.llm_service", _module_stub("api.db.services.llm_service", LLMBundle=None))
    monkeypatch.setitem(sys.modules, "api.db.services.search_service", _module_stub("api.db.services.search_service", SearchService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.services.task_service", _module_stub("api.db.services.task_service", TaskService=SimpleNamespace(), cancel_all_task_of=None))
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_llm_service", _module_stub("api.db.services.tenant_llm_service", TenantLLMService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "common.llm_request_context", _module_stub("common.llm_request_context", normalize_llm_user_id=None, reset_llm_request_context=None, set_llm_request_context=None))
    monkeypatch.setitem(
        sys.modules,
        "api.utils.api_utils",
        _module_stub(
            "api.utils.api_utils",
            add_tenant_id_to_kwargs=lambda f: f,
            check_duplicate_ids=None,
            construct_json_result=None,
            get_error_data_result=lambda *a, **kw: {"code": 102, "message": kw.get("message", a[0] if a else ""), "data": None},
            get_request_json=lambda: asyncio.sleep(0, result=REQUEST_JSON),
            get_result=lambda **kw: {"code": kw.get("code", 0), "message": kw.get("message", ""), "data": kw.get("data")},
            server_error_response=lambda e: {"code": 100, "message": str(e), "data": None},
        ),
    )
    monkeypatch.setitem(sys.modules, "api.utils.image_utils", _module_stub("api.utils.image_utils", store_chunk_image=None))
    monkeypatch.setitem(
        sys.modules,
        "api.utils.pagination_utils",
        _module_stub("api.utils.pagination_utils", DEFAULT_PAGE=1, DEFAULT_PAGE_SIZE=30, validate_rest_api_ids=None, validate_rest_api_page=lambda p: int(p), validate_rest_api_page_size=lambda s: int(s)),
    )
    monkeypatch.setitem(sys.modules, "api.utils.reference_metadata_utils", _module_stub("api.utils.reference_metadata_utils", enrich_chunks_with_document_metadata=None, resolve_reference_metadata_preferences=None))
    monkeypatch.setitem(sys.modules, "common", _module_stub("common", settings=settings_stub))
    monkeypatch.setitem(sys.modules, "common.constants", _module_stub("common.constants", LLMType=SimpleNamespace(EMBEDDING=SimpleNamespace(value="embedding")), ParserType=SimpleNamespace(), RetCode=SimpleNamespace(DATA_ERROR=102), TaskStatus=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "common.doc_store.doc_store_base", _module_stub("common.doc_store.doc_store_base", OrderByExpr=None))
    monkeypatch.setitem(sys.modules, "common.metadata_utils", _module_stub("common.metadata_utils", apply_meta_data_filter=None, convert_conditions=None, filter_doc_ids_by_metadata=None))
    monkeypatch.setitem(sys.modules, "common.misc_utils", _module_stub("common.misc_utils", thread_pool_exec=None))
    monkeypatch.setitem(sys.modules, "common.string_utils", _module_stub("common.string_utils", is_content_empty=lambda s: not str(s).strip(), remove_redundant_spaces=lambda s: s))
    monkeypatch.setitem(sys.modules, "common.tag_feature_utils", _module_stub("common.tag_feature_utils", validate_tag_features=None))
    monkeypatch.setitem(sys.modules, "rag.app.tag", _module_stub("rag.app.tag", label_question=None))
    monkeypatch.setitem(sys.modules, "rag.app.qa", _module_stub("rag.app.qa", beAdoc=None, rmPrefix=None))
    monkeypatch.setitem(sys.modules, "rag.nlp", _module_stub("rag.nlp", rag_tokenizer=rag_tokenizer, search=search_stub))
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", _module_stub("rag.prompts.generator", cross_languages=None, keyword_extraction=None))

    module_name = "test_chunk_api_unit_module"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "chunk_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_update_chunk_non_integer_available_returns_data_error_not_500(monkeypatch):
    module = _load_chunk_api(monkeypatch)
    REQUEST_JSON.clear()
    REQUEST_JSON.update({"available": "abc"})
    res = asyncio.run(module.update_chunk(tenant_id="tenant-1", dataset_id="ds-1", document_id="doc-1", chunk_id="chunk-1"))
    assert res["code"] == 102
    assert "`available` should be an integer" in res["message"]


@pytest.mark.p2
def test_switch_chunks_non_integer_available_int_returns_data_error_not_500(monkeypatch):
    module = _load_chunk_api(monkeypatch)
    REQUEST_JSON.clear()
    REQUEST_JSON.update({"chunk_ids": ["chunk-1"], "available_int": "abc"})
    res = asyncio.run(module.switch_chunks(tenant_id="tenant-1", dataset_id="ds-1", document_id="doc-1"))
    assert res["code"] == 102
    assert "`available_int` should be an integer" in res["message"]

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
"""Regression tests for the ordering of manually added chunks (chunk_api.py).

``_next_source_chunk_order`` used to answer 0 on any doc-store read failure, so
a transient engine error inserted a row at the column default - the same value
every unordered legacy row carries. The read now raises and ``add_chunk``
answers an error without writing anything.
"""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import numpy as np
import pytest

import common.doc_store.doc_store_base as _real_doc_store_base


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


class _OrderByExpr:
    def __init__(self):
        self.fields = []

    def asc(self, name):
        self.fields.append((name, 0))

    def desc(self, name):
        self.fields.append((name, 1))


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    if name == "common.doc_store.doc_store_base":
        # Seed from the real module so newly added helpers stay importable.
        mod.__dict__.update({k: v for k, v in vars(_real_doc_store_base).items() if not k.startswith("__")})
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _load_chunk_api(monkeypatch, *, search, fields, inserted=None):
    """Load chunk_api.py with only the import chain stubbed.

    ``search`` is the doc-store search used by ``_next_source_chunk_order`` and
    ``inserted`` records what ``add_chunk`` writes.
    """

    async def _get_request_json():
        return {"content": "manual chunk"}

    async def _thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    doc_store = SimpleNamespace(
        search=search,
        get_fields=lambda _res, _fields: fields,
        insert=lambda documents, *_args, **_kwargs: (inserted.extend(documents) if inserted is not None else []) or [],
    )

    _stub(monkeypatch, "api.apps", current_user=SimpleNamespace(id="tenant-1"), login_required=lambda func: func)
    _stub(monkeypatch, "api.apps.services", structure_graph_common=SimpleNamespace())
    _stub(monkeypatch, "api.apps.services.structure_graph_common")
    _stub(monkeypatch, "api.db.db_models", Document=SimpleNamespace, Task=SimpleNamespace)
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=lambda *_a, **_k: {},
        resolve_model_config=lambda *_a, **_k: {},
    )
    _stub(monkeypatch, "api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace)
    _stub(monkeypatch, "api.db.services.document_counter_service", release_reparse_counters=lambda *_a, **_k: None)
    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(
            query=lambda **_kwargs: [SimpleNamespace(id="doc-1", name="doc.pdf", kb_id="kb-1")],
            get_embd_id=lambda *_a, **_k: "embd-1",
            increment_chunk_num=lambda *_a, **_k: True,
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(
            accessible=lambda **_kwargs: True,
            get_by_id=lambda *_a, **_k: (True, SimpleNamespace(tenant_id="tenant-1")),
        ),
        validate_dataset_embedding_models=lambda *_a, **_k: None,
    )
    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=SimpleNamespace)
    _stub(monkeypatch, "api.db.services.search_service", SearchService=SimpleNamespace)
    _stub(monkeypatch, "api.db.services.task_service", TaskService=SimpleNamespace, cancel_all_task_of=lambda *_a, **_k: None)
    _stub(
        monkeypatch,
        "api.db.services.tenant_llm_service",
        TenantLLMService=SimpleNamespace(model_instance=lambda *_a, **_k: SimpleNamespace(encode=lambda _texts: ([np.array([0.1, 0.2]), np.array([0.3, 0.4])], 5))),
    )
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        add_tenant_id_to_kwargs=lambda func: func,
        check_duplicate_ids=lambda *_a, **_k: None,
        construct_json_result=lambda code=0, message="", data=None: {"code": code, "message": message, "data": data},
        get_error_data_result=lambda message="Sorry", code=100: {"code": code, "message": message},
        get_request_json=_get_request_json,
        get_result=lambda code=0, message="", data=None: {"code": code, "message": message, "data": data},
        server_error_response=lambda exc: {"code": 500, "message": str(exc)},
    )
    _stub(
        monkeypatch,
        "api.utils.image_utils",
        IMAGE_UPDATE_MODE_REMOVE="remove",
        IMAGE_UPDATE_MODES=("append", "replace", "remove"),
        remove_chunk_image=lambda *_a, **_k: None,
        store_chunk_image=lambda *_a, **_k: None,
    )
    _stub(
        monkeypatch,
        "api.utils.pagination_utils",
        DEFAULT_PAGE=1,
        DEFAULT_PAGE_SIZE=30,
        validate_rest_api_ids=lambda *_a, **_k: None,
        validate_rest_api_page=lambda *_a, **_k: 1,
        validate_rest_api_page_size=lambda *_a, **_k: 30,
    )
    _stub(
        monkeypatch,
        "api.utils.reference_metadata_utils",
        enrich_chunks_with_document_metadata=lambda *_a, **_k: None,
        resolve_reference_metadata_preferences=lambda *_a, **_k: [],
    )
    _stub(monkeypatch, "common", settings=SimpleNamespace(docStoreConn=doc_store))
    _stub(
        monkeypatch,
        "common.constants",
        LLMType=SimpleNamespace(EMBEDDING=SimpleNamespace(value="embedding")),
        ParserType=SimpleNamespace(TABLE=SimpleNamespace(value="table")),
        RetCode=SimpleNamespace(SUCCESS=0, EXCEPTION_ERROR=100, DATA_ERROR=102, SERVER_ERROR=500),
        TaskStatus=SimpleNamespace(),
    )
    _stub(monkeypatch, "common.doc_store.doc_store_base", OrderByExpr=_OrderByExpr)
    _stub(
        monkeypatch,
        "common.llm_request_context",
        normalize_llm_user_id=lambda user_id=None: user_id,
        reset_llm_request_context=lambda _token=None: None,
        set_llm_request_context=lambda **_kwargs: None,
    )
    _stub(
        monkeypatch,
        "common.metadata_utils",
        apply_meta_data_filter=lambda *_a, **_k: None,
        convert_conditions=lambda *_a, **_k: None,
        filter_doc_ids_by_metadata=lambda *_a, **_k: [],
    )
    _stub(monkeypatch, "common.misc_utils", thread_pool_exec=_thread_pool_exec)
    _stub(monkeypatch, "common.string_utils", is_content_empty=lambda value: not value, remove_redundant_spaces=lambda value: value)
    _stub(monkeypatch, "common.tag_feature_utils", validate_tag_features=lambda value: value)
    _stub(monkeypatch, "rag.app.tag", label_question=lambda *_a, **_k: None)
    _stub(
        monkeypatch,
        "rag.nlp",
        search=SimpleNamespace(index_name=lambda tenant_id: f"ragflow_{tenant_id}"),
        rag_tokenizer=SimpleNamespace(tokenize=lambda text: text, fine_grained_tokenize=lambda text: text),
    )
    _stub(monkeypatch, "rag.prompts.generator", cross_languages=lambda *_a, **_k: None, keyword_extraction=lambda *_a, **_k: None)
    _stub(monkeypatch, "quart", request=SimpleNamespace())

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "chunk_api.py"
    spec = importlib.util.spec_from_file_location("test_add_chunk_order_chunk_api", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_add_chunk_order_chunk_api", module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p1
class TestNextSourceChunkOrder:
    def test_read_failure_raises_instead_of_returning_zero(self, monkeypatch):
        def _boom(*_args, **_kwargs):
            raise RuntimeError("doc store unavailable")

        module = _load_chunk_api(monkeypatch, search=_boom, fields={})

        with pytest.raises(RuntimeError):
            asyncio.run(module._next_source_chunk_order("tenant-1", "kb-1", "doc-1"))

    def test_document_without_enabled_row_starts_at_zero(self, monkeypatch):
        module = _load_chunk_api(monkeypatch, search=lambda *_a, **_k: object(), fields={})

        assert asyncio.run(module._next_source_chunk_order("tenant-1", "kb-1", "doc-1")) == 0

    def test_orders_after_the_highest_enabled_row(self, monkeypatch):
        module = _load_chunk_api(monkeypatch, search=lambda *_a, **_k: object(), fields={"c1": {"chunk_order_int": 41}})

        assert asyncio.run(module._next_source_chunk_order("tenant-1", "kb-1", "doc-1")) == 42


@pytest.mark.p1
class TestAddChunkOrderFailure:
    def test_read_failure_rejects_the_request_without_inserting(self, monkeypatch):
        def _boom(*_args, **_kwargs):
            raise RuntimeError("doc store unavailable")

        inserted = []
        module = _load_chunk_api(monkeypatch, search=_boom, fields={}, inserted=inserted)

        result = asyncio.run(module.add_chunk(tenant_id="tenant-1", dataset_id="kb-1", document_id="doc-1"))

        assert result["code"] == 500
        assert inserted == []

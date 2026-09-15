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
"""Regression tests for /searchbots/retrieval_test's doc-level filter
(``api/apps/restful_apis/bot_api.py``).

``Dealer.get_filters`` distinguishes ``doc_ids=None`` ("no doc filter") from
``doc_ids=[]`` ("filter on an empty set of documents"): only the latter reaches
the document store as a ``doc_id`` condition, where it is either dropped (ES,
OpenSearch, Infinity, OceanBase, SereneDB skip falsy condition values, widening
the search to the whole dataset) or rejected outright (GaussDB raises).  A
request that omitted ``doc_ids`` -- or supplied nothing but blank ids -- must
therefore forward ``None``, not ``[]``.
"""

import asyncio
import importlib.util
import inspect
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest

# Imported before the loader stubs ``common.metadata_utils`` so the call site
# exercises the real normalization rather than a test double.
from common.metadata_utils import normalize_doc_id_filter

pytestmark = pytest.mark.p2


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


async def _passthrough_thread_pool_exec(fn, *args, **kwargs):
    return fn(*args, **kwargs)


class _StubKB:
    id = "kb-1"
    tenant_id = "tenant-1"
    embd_id = "embd-1"
    parser_id = "naive"


def _load_bot_api(monkeypatch, *, payload, retrieval_calls):
    """Load bot_api.py with the minimum stubs retrieval_test_embedded needs."""

    async def _get_request_json():
        return dict(payload)

    async def _retrieval(*args, **kwargs):
        retrieval_calls.append({"args": args, "kwargs": kwargs})
        return {"total": 0, "chunks": []}

    _stub(monkeypatch, "quart", Response=lambda *a, **k: SimpleNamespace(headers=SimpleNamespace(add_header=lambda *aa, **kk: None)), request=SimpleNamespace())
    _stub(monkeypatch, "api.apps", AUTH_BETA="beta", login_required=lambda *_a, **_k: lambda func: func, __path__=[])
    _stub(monkeypatch, "api.apps.restful_apis", __path__=[])
    _stub(monkeypatch, "agent.canvas", Canvas=lambda *a, **k: SimpleNamespace())
    _stub(monkeypatch, "api.db.db_models", APIToken=SimpleNamespace(query=lambda **_k: []))
    _stub(monkeypatch, "api.db.services.api_service", API4ConversationService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.canvas_service", UserCanvasService=SimpleNamespace(), completion=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.user_canvas_version", UserCanvasVersionService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.conversation_service", async_iframe_completion=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.dialog_service", DialogService=SimpleNamespace(), async_ask=lambda *_a, **_k: None, gen_mindmap=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace(get_flatted_meta_by_kbs=lambda _kb_ids: {}))
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(
            get_by_id=lambda _kb_id: (True, _StubKB()),
            query=lambda **_kwargs: [_StubKB()],
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.llm_service",
        LLMBundle=lambda *_a, **_k: SimpleNamespace(max_length=4096),
        resolve_llm_setting=lambda s: s or {},
    )
    _stub(monkeypatch, "common.metadata_utils", apply_meta_data_filter=lambda *_a, **_k: None, normalize_doc_id_filter=normalize_doc_id_filter)
    _stub(monkeypatch, "api.db.services.search_service", SearchService=SimpleNamespace(get_detail=lambda _search_id: None))
    _stub(
        monkeypatch,
        "api.db.services.user_service",
        TenantService=SimpleNamespace(),
        UserTenantService=SimpleNamespace(query=lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")]),
    )
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=lambda *_a, **_k: {"model_type": "chat"},
        resolve_model_config=lambda *_a, **_k: {"model_type": "embedding"},
    )
    _stub(monkeypatch, "common.misc_utils", get_uuid=lambda: "uuid", thread_pool_exec=_passthrough_thread_pool_exec)
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        add_tenant_id_to_kwargs=lambda func: func,
        get_error_data_result=lambda message="Sorry", **_k: {"code": 102, "message": message, "data": None},
        get_json_result=lambda code=0, message="", data=None: {"code": code, "message": message, "data": data},
        get_result=lambda **kwargs: {"code": 0, "data": kwargs.get("data")},
        get_request_json=_get_request_json,
        server_error_response=lambda exc: {"code": 500, "message": str(exc)},
        validate_request=lambda *_a, **_k: lambda func: func,
    )
    _stub(monkeypatch, "rag.app.tag", label_question=lambda *_a, **_k: ["label-1"])
    _stub(monkeypatch, "rag.prompts.template", load_prompt=lambda *_a, **_k: "")
    _stub(monkeypatch, "rag.prompts.generator", cross_languages=lambda *_a, **_k: None, keyword_extraction=lambda *_a, **_k: None)
    _stub(
        monkeypatch,
        "common.constants",
        RetCode=SimpleNamespace(SUCCESS=0, DATA_ERROR=102, OPERATING_ERROR=103),
        LLMType=SimpleNamespace(CHAT="chat", EMBEDDING="embedding", RERANK="rerank"),
        StatusEnum=SimpleNamespace(),
    )
    settings_stub = SimpleNamespace(retriever=SimpleNamespace(retrieval=_retrieval), kg_retriever=SimpleNamespace())
    _stub(monkeypatch, "common", settings=settings_stub)
    monkeypatch.setitem(sys.modules, "common.settings", settings_stub)
    _stub(
        monkeypatch,
        "api.utils.reference_metadata_utils",
        enrich_chunks_with_document_metadata=lambda *_a, **_k: None,
        resolve_reference_metadata_preferences=lambda *_a, **_k: (False, None),
    )
    _stub(monkeypatch, "rag.utils.web_search_conn", has_web_search_provider=lambda *_a, **_k: False)
    _stub(
        monkeypatch,
        "api.utils.pagination_utils",
        DEFAULT_PAGE=1,
        DEFAULT_PAGE_SIZE=30,
        validate_rest_api_page=lambda value: int(value),
        validate_rest_api_page_size=lambda value: int(value),
    )

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "bot_api.py"
    spec = importlib.util.spec_from_file_location("test_retrieval_test_embedded_bot_api", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_retrieval_test_embedded_bot_api", module)
    spec.loader.exec_module(module)
    return module


def _run_retrieval_test(monkeypatch, payload):
    retrieval_calls = []
    module = _load_bot_api(monkeypatch, payload=payload, retrieval_calls=retrieval_calls)
    res = asyncio.run(inspect.unwrap(module.retrieval_test_embedded)(tenant_id="tenant-1"))
    return res, retrieval_calls


def test_retrieval_test_embedded_without_doc_ids_forwards_none(monkeypatch):
    res, retrieval_calls = _run_retrieval_test(monkeypatch, {"kb_id": "kb-1", "question": "hello"})

    assert res["code"] == 0, res
    assert len(retrieval_calls) == 1
    assert retrieval_calls[0]["kwargs"]["doc_ids"] is None


def test_retrieval_test_embedded_with_doc_ids_forwards_non_blank_ids(monkeypatch):
    res, retrieval_calls = _run_retrieval_test(monkeypatch, {"kb_id": "kb-1", "question": "hello", "doc_ids": ["doc-1", "", "doc-2"]})

    assert res["code"] == 0, res
    assert retrieval_calls[0]["kwargs"]["doc_ids"] == ["doc-1", "doc-2"]


def test_retrieval_test_embedded_with_only_blank_doc_ids_forwards_none(monkeypatch):
    """All-blank ids must collapse to ``None``, not to an empty active filter."""
    res, retrieval_calls = _run_retrieval_test(monkeypatch, {"kb_id": "kb-1", "question": "hello", "doc_ids": ["", ""]})

    assert res["code"] == 0, res
    assert retrieval_calls[0]["kwargs"]["doc_ids"] is None

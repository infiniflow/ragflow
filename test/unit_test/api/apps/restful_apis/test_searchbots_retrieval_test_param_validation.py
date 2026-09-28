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
"""Regression tests for numeric parameter validation in
`POST /searchbots/retrieval_test` (api/apps/restful_apis/bot_api.py).

`top_k`, `rerank_candidates_count`, `similarity_threshold` and
`vector_similarity_weight` used to be parsed with bare `int(...)`/`float(...)`,
so a non-numeric value raised ValueError and surfaced as a 500. They must now
answer with a data error naming the field, matching the sibling validation in
`POST /api/v1/retrieval` (chunk_api.py).
"""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _PassthroughManager:
    """Stand-in for the blueprint manager: route decorators become no-ops."""

    def route(self, *_args, **_kwargs):
        return lambda func: func


def _stub(monkeypatch, name, **attrs):
    """Register a stub module under `name` for the duration of the test."""
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


async def _passthrough_thread_pool_exec(fn, *args, **kwargs):
    """Run `fn` inline so awaited service calls stay synchronous in tests."""
    return fn(*args, **kwargs)


def _load_bot_api(monkeypatch, request_json):
    """Load bot_api.py with the minimum stubs retrieval_test needs."""
    _stub(monkeypatch, "quart", Response=lambda *a, **k: SimpleNamespace(), request=SimpleNamespace())
    _stub(monkeypatch, "api.apps", AUTH_BETA="beta", login_required=lambda *_a, **_k: lambda func: func, __path__=[])
    _stub(monkeypatch, "api.apps.restful_apis", __path__=[])
    _stub(monkeypatch, "agent.canvas", Canvas=lambda *a, **k: SimpleNamespace())
    _stub(monkeypatch, "api.db.db_models", APIToken=SimpleNamespace(query=lambda **_k: []))
    _stub(monkeypatch, "api.db.services.api_service", API4ConversationService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.canvas_service", UserCanvasService=SimpleNamespace(), completion=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.user_canvas_version", UserCanvasVersionService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.conversation_service", async_iframe_completion=lambda *_a, **_k: None)

    async def _ask(*_a, **_k):
        if False:
            yield None

    _stub(
        monkeypatch,
        "api.db.services.dialog_service",
        DialogService=SimpleNamespace(),
        async_ask=_ask,
        gen_mindmap=lambda *_a, **_k: {},
    )
    _stub(monkeypatch, "api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(accessible=lambda *_a, **_k: True),
    )
    _stub(
        monkeypatch,
        "api.db.services.llm_service",
        LLMBundle=SimpleNamespace(),
        resolve_llm_setting=lambda s: s or {},
    )
    _stub(monkeypatch, "common.metadata_utils", apply_meta_data_filter=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.search_service", SearchService=SimpleNamespace(get_detail=lambda _id: None))
    _stub(monkeypatch, "api.db.services.user_service", TenantService=SimpleNamespace(), UserTenantService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=lambda *_a, **_k: None,
        get_model_config_from_provider_instance=lambda *_a, **_k: None,
        resolve_model_config=lambda *_a, **_k: None,
    )
    _stub(monkeypatch, "common.misc_utils", get_uuid=lambda: "uuid", thread_pool_exec=_passthrough_thread_pool_exec)

    async def _get_request_json():
        return request_json

    _stub(
        monkeypatch,
        "api.utils.api_utils",
        add_tenant_id_to_kwargs=lambda func: func,
        check_duplicate_ids=lambda *_a, **_k: None,
        get_error_data_result=lambda message="Sorry", **_k: {"code": 102, "message": message, "data": None},
        get_json_result=lambda code=0, message="", data=None: {"code": code, "message": message, "data": data},
        get_result=lambda **kwargs: {"code": 0, "data": kwargs.get("data")},
        get_request_json=_get_request_json,
        server_error_response=lambda exc: {"code": 500, "message": str(exc)},
        token_required=lambda func: func,
        validate_request=lambda *_a, **_k: lambda func: func,
    )
    _stub(monkeypatch, "rag.app.tag", label_question=lambda *_a, **_k: None)
    _stub(monkeypatch, "rag.prompts.template", load_prompt=lambda *_a, **_k: "")
    _stub(monkeypatch, "rag.prompts.generator", cross_languages=lambda *_a, **_k: None, keyword_extraction=lambda *_a, **_k: None)
    _stub(monkeypatch, "rag.utils.web_search_conn", has_web_search_provider=lambda *_a, **_k: False)
    _stub(monkeypatch, "common.constants", RetCode=SimpleNamespace(), LLMType=SimpleNamespace(), StatusEnum=SimpleNamespace())
    _stub(monkeypatch, "common", settings=SimpleNamespace())
    _stub(monkeypatch, "common.settings", retriever=SimpleNamespace(), kg_retriever=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.utils.reference_metadata_utils",
        enrich_chunks_with_document_metadata=lambda *_a, **_k: None,
        resolve_reference_metadata_preferences=lambda *_a, **_k: None,
    )

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "bot_api.py"
    spec = importlib.util.spec_from_file_location("test_searchbots_retrieval_test_bot_api", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_searchbots_retrieval_test_bot_api", module)
    spec.loader.exec_module(module)
    return module


def _run(monkeypatch, req, tenant_id="tenant-1"):
    """Load bot_api with `req` as the request body and run retrieval_test."""
    module = _load_bot_api(monkeypatch, req)
    return asyncio.run(module.retrieval_test_embedded(tenant_id=tenant_id))


@pytest.mark.p1
class TestSearchbotsRetrievalTestParamValidation:
    """Non-numeric numeric parameters must answer with a data error, not a 500."""

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "field,value,expected",
        [
            ("top_k", "abc", "`top_k` should be an integer"),
            ("top_k", [], "`top_k` should be an integer"),
            ("rerank_candidates_count", "abc", "`rerank_candidates_count` should be an integer"),
            ("rerank_candidates_count", None, "`rerank_candidates_count` should be an integer"),
            ("similarity_threshold", "abc", "`similarity_threshold` should be a number"),
            ("similarity_threshold", [], "`similarity_threshold` should be a number"),
            ("vector_similarity_weight", "abc", "`vector_similarity_weight` should be a number"),
            ("vector_similarity_weight", None, "`vector_similarity_weight` should be a number"),
        ],
    )
    def test_non_numeric_param_returns_data_error(self, monkeypatch, field, value, expected):
        """Each numeric field answers with a data error naming the field."""
        req = {"kb_id": ["kb-1"], "question": "hello", field: value}
        result = _run(monkeypatch, req)
        assert result == {"code": 102, "message": expected, "data": None}

    @pytest.mark.p1
    def test_numeric_params_pass_validation(self, monkeypatch):
        """Valid numbers must get past the parsing stage of the handler."""
        req = {
            "kb_id": ["kb-1"],
            "question": "hello",
            "top_k": 5,
            "rerank_candidates_count": 8,
            "similarity_threshold": 0.5,
            "vector_similarity_weight": 0.7,
        }
        result = _run(monkeypatch, req, tenant_id=None)
        # An empty tenant_id stops the handler at the tenant check, which only
        # happens after every conversion succeeded.
        assert result == {"code": 102, "message": "permission denined.", "data": None}

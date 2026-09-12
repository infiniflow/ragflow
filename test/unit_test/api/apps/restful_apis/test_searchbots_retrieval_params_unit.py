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

"""Unit tests for POST /searchbots/retrieval_test parameter validation.

Non-numeric numeric parameters must produce a data error (400-class
envelope) instead of a ValueError that surfaces as a 500. Mirrors the
sibling validation in chunk_api's retrieval route.
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


def _load_bot_api(monkeypatch):
    repo_root = Path(__file__).resolve().parents[5]

    def _envelope(data=None, message="", code=0):
        return {"code": code, "message": message, "data": data}

    monkeypatch.setitem(sys.modules, "quart", _module_stub("quart", Response=None, request=SimpleNamespace(args={})))
    monkeypatch.setitem(sys.modules, "agent.canvas", _module_stub("agent.canvas", Canvas=None))
    monkeypatch.setitem(
        sys.modules,
        "api.apps",
        _module_stub(
            "api.apps",
            AUTH_BETA="beta",
            login_required=lambda *a, **k: (a[0] if a and callable(a[0]) else (lambda f: f)),
        ),
    )
    monkeypatch.setitem(sys.modules, "api.db.services.api_service", _module_stub("api.db.services.api_service", API4ConversationService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.services.canvas_service", _module_stub("api.db.services.canvas_service", UserCanvasService=SimpleNamespace(), completion=None))
    monkeypatch.setitem(sys.modules, "api.db.services.conversation_service", _module_stub("api.db.services.conversation_service", async_iframe_completion=None))
    monkeypatch.setitem(sys.modules, "api.db.services.dialog_service", _module_stub("api.db.services.dialog_service", DialogService=SimpleNamespace(), async_ask=None, gen_mindmap=None))
    monkeypatch.setitem(sys.modules, "api.db.services.doc_metadata_service", _module_stub("api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", _module_stub("api.db.services.knowledgebase_service", KnowledgebaseService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.services.llm_service", _module_stub("api.db.services.llm_service", LLMBundle=None, resolve_llm_setting=None))
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", _module_stub("api.db.services.user_service", TenantService=SimpleNamespace(), UserTenantService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.services.search_service", _module_stub("api.db.services.search_service", SearchService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.joint_services.tenant_model_service", _module_stub("api.db.joint_services.tenant_model_service", get_tenant_default_model_by_type=None, resolve_model_config=None))
    monkeypatch.setitem(sys.modules, "common.metadata_utils", _module_stub("common.metadata_utils", apply_meta_data_filter=None))
    monkeypatch.setitem(sys.modules, "common.misc_utils", _module_stub("common.misc_utils", thread_pool_exec=None))
    monkeypatch.setitem(sys.modules, "rag.app.tag", _module_stub("rag.app.tag", label_question=None))
    monkeypatch.setitem(sys.modules, "rag.prompts.template", _module_stub("rag.prompts.template", load_prompt=None))
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", _module_stub("rag.prompts.generator", cross_languages=None, keyword_extraction=None))
    monkeypatch.setitem(sys.modules, "rag.utils.web_search_conn", _module_stub("rag.utils.web_search_conn", has_web_search_provider=None))
    monkeypatch.setitem(sys.modules, "api.utils.reference_metadata_utils", _module_stub("api.utils.reference_metadata_utils", enrich_chunks_with_document_metadata=None, resolve_reference_metadata_preferences=None))
    monkeypatch.setitem(
        sys.modules,
        "api.utils.pagination_utils",
        _module_stub("api.utils.pagination_utils", DEFAULT_PAGE=1, DEFAULT_PAGE_SIZE=30, validate_rest_api_page=lambda p: int(p), validate_rest_api_page_size=lambda s: int(s)),
    )
    monkeypatch.setitem(
        sys.modules,
        "api.utils.api_utils",
        _module_stub(
            "api.utils.api_utils",
            get_error_data_result=lambda *a, **kw: _envelope(None, kw.get("message", a[0] if a else ""), 102),
            get_json_result=lambda **kw: _envelope(kw.get("data"), kw.get("message", ""), kw.get("code", 0)),
            get_result=lambda **kw: _envelope(kw.get("data"), kw.get("message", ""), kw.get("code", 0)),
            add_tenant_id_to_kwargs=lambda f: f,
            get_request_json=lambda: asyncio.sleep(0, result=REQUEST_JSON),
            server_error_response=lambda e: _envelope(None, str(e), 100),
            validate_request=lambda *_a, **_k: (lambda f: f),
        ),
    )
    monkeypatch.setitem(
        sys.modules,
        "common.constants",
        _module_stub("common.constants", RetCode=SimpleNamespace(DATA_ERROR=102, ARGUMENT_ERROR=101), LLMType=SimpleNamespace(CHAT="chat"), StatusEnum=SimpleNamespace(VALID=SimpleNamespace(value="1"))),
    )
    monkeypatch.setitem(sys.modules, "common", _module_stub("common", settings=SimpleNamespace()))

    module_name = "test_bot_api_unit_module"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "bot_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _base_request(**overrides):
    req = {"kb_id": "kb-1", "question": "hello"}
    req.update(overrides)
    return req


@pytest.mark.p2
@pytest.mark.parametrize("field", ["top_k", "rerank_candidates_count"])
def test_non_integer_count_params_return_data_error_not_500(monkeypatch, field):
    module = _load_bot_api(monkeypatch)
    REQUEST_JSON.clear()
    REQUEST_JSON.update(_base_request(**{field: "abc"}))
    res = asyncio.run(module.retrieval_test_embedded(tenant_id="tenant-1"))
    assert res["code"] == 102
    assert "must be integers" in res["message"]


@pytest.mark.p2
@pytest.mark.parametrize("field", ["similarity_threshold", "vector_similarity_weight"])
def test_non_numeric_weight_params_return_data_error_not_500(monkeypatch, field):
    module = _load_bot_api(monkeypatch)
    REQUEST_JSON.clear()
    REQUEST_JSON.update(_base_request(**{field: "abc"}))
    res = asyncio.run(module.retrieval_test_embedded(tenant_id="tenant-1"))
    assert res["code"] == 102
    assert "must be numbers" in res["message"]


@pytest.mark.p2
def test_non_positive_top_k_still_rejected(monkeypatch):
    module = _load_bot_api(monkeypatch)
    REQUEST_JSON.clear()
    REQUEST_JSON.update(_base_request(top_k=0))
    res = asyncio.run(module.retrieval_test_embedded(tenant_id="tenant-1"))
    assert res["code"] == 102
    assert "greater than 0" in res["message"]

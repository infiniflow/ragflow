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
"""Regression tests for the cross-tenant access guard on the embedded search
endpoints in api/apps/restful_apis/bot_api.py.

A beta token is per-tenant and any registered user can mint one, so
`/searchbots/ask` and `/searchbots/mindmap` must reject `kb_ids` that the
caller's tenants do not own — otherwise retrieval runs against the target kb's
own tenant and leaks another tenant's document contents (IDOR)."""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import AsyncMock

import pytest


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


async def _thread_pool_exec(fn, *args, **kwargs):
    return fn(*args, **kwargs)


def _load_bot_api(monkeypatch):
    """Load api/apps/restful_apis/bot_api.py with its dependencies stubbed."""
    _stub(monkeypatch, "quart", Response=object, request=SimpleNamespace(headers={"Authorization": "Bearer tok"}))
    _stub(monkeypatch, "agent.canvas", Canvas=SimpleNamespace())
    _stub(monkeypatch, "api.db.db_models", APIToken=SimpleNamespace(query=lambda **_k: [SimpleNamespace(tenant_id="caller")]))
    _stub(monkeypatch, "api.db.services.api_service", API4ConversationService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.canvas_service", UserCanvasService=SimpleNamespace(), completion=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.user_canvas_version", UserCanvasVersionService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.conversation_service", async_iframe_completion=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.dialog_service", DialogService=SimpleNamespace(), async_ask=AsyncMock(), gen_mindmap=AsyncMock())
    _stub(monkeypatch, "api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.knowledgebase_service", KnowledgebaseService=SimpleNamespace(query=lambda **_k: []))
    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=SimpleNamespace())
    _stub(monkeypatch, "common.metadata_utils", apply_meta_data_filter=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.search_service", SearchService=SimpleNamespace(get_detail=lambda *_a, **_k: {}))
    _stub(monkeypatch, "api.db.services.user_service", UserTenantService=SimpleNamespace(query=lambda **_k: []))
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=lambda *_a, **_k: {},
        get_model_config_by_id=lambda *_a, **_k: {},
        get_model_config_by_type_and_name=lambda *_a, **_k: {},
    )
    _stub(monkeypatch, "common.misc_utils", get_uuid=lambda: "uuid", thread_pool_exec=_thread_pool_exec)
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        check_duplicate_ids=lambda *_a, **_k: None,
        get_error_data_result=lambda message="": {"code": 401, "message": message},
        get_json_result=lambda code=0, message="", data=None: {"code": code, "message": message, "data": data},
        get_result=lambda **kwargs: kwargs,
        get_request_json=AsyncMock(return_value={}),
        server_error_response=lambda exc: {"code": 500, "message": str(exc)},
        token_required=lambda func: func,
        validate_request=lambda *_a, **_k: (lambda func: func),
    )
    _stub(monkeypatch, "rag.app.tag", label_question=lambda *_a, **_k: None)
    _stub(monkeypatch, "rag.prompts.template", load_prompt=lambda *_a, **_k: "")
    _stub(monkeypatch, "rag.prompts.generator", cross_languages=lambda *_a, **_k: "", keyword_extraction=lambda *_a, **_k: "")
    _stub(monkeypatch, "common.constants", RetCode=SimpleNamespace(OPERATING_ERROR=100), LLMType=SimpleNamespace(), StatusEnum=SimpleNamespace())
    _stub(monkeypatch, "common.settings", retriever=SimpleNamespace(), kg_retriever=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.utils.reference_metadata_utils",
        enrich_chunks_with_document_metadata=lambda *_a, **_k: None,
        resolve_reference_metadata_preferences=lambda *_a, **_k: None,
    )

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "bot_api.py"
    spec = importlib.util.spec_from_file_location("test_searchbots_bot_api", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_searchbots_bot_api", module)
    spec.loader.exec_module(module)
    return module


def _set_ownership(module, owned):
    """Caller belongs to tenant 'caller'; `owned` is the set of kb_ids it owns."""
    module.UserTenantService.query = lambda **_k: [SimpleNamespace(tenant_id="caller")]
    module.KnowledgebaseService.query = lambda tenant_id=None, id=None, **_k: ([1] if (tenant_id == "caller" and id in owned) else [])


@pytest.mark.p1
class TestAssertCallerOwnsKbs:
    def test_owned_kb_passes(self, monkeypatch):
        module = _load_bot_api(monkeypatch)
        _set_ownership(module, owned={"mine"})
        assert asyncio.run(module._assert_caller_owns_kbs("caller", ["mine"])) is None

    def test_owned_single_string_passes(self, monkeypatch):
        module = _load_bot_api(monkeypatch)
        _set_ownership(module, owned={"mine"})
        assert asyncio.run(module._assert_caller_owns_kbs("caller", "mine")) is None

    def test_foreign_kb_rejected(self, monkeypatch):
        module = _load_bot_api(monkeypatch)
        _set_ownership(module, owned={"mine"})
        result = asyncio.run(module._assert_caller_owns_kbs("caller", ["victim"]))
        assert result == {"code": 100, "message": "Only owner of dataset authorized for this operation.", "data": False}

    def test_mixed_owned_and_foreign_rejected(self, monkeypatch):
        module = _load_bot_api(monkeypatch)
        _set_ownership(module, owned={"mine"})
        result = asyncio.run(module._assert_caller_owns_kbs("caller", ["mine", "victim"]))
        assert result is not None and result["data"] is False


@pytest.mark.p1
class TestMindmapEndpointGuard:
    def test_foreign_kb_blocks_before_generation(self, monkeypatch):
        module = _load_bot_api(monkeypatch)
        _set_ownership(module, owned={"mine"})
        monkeypatch.setattr(module, "get_request_json", AsyncMock(return_value={"question": "q", "kb_ids": ["victim"]}))
        gen = AsyncMock(return_value={"mind_map": {}})
        monkeypatch.setattr(module, "gen_mindmap", gen)

        result = asyncio.run(module.mindmap())

        assert result["data"] is False
        assert result["message"] == "Only owner of dataset authorized for this operation."
        gen.assert_not_awaited()  # retrieval must never run for an unowned kb

    def test_owned_kb_allows_generation(self, monkeypatch):
        module = _load_bot_api(monkeypatch)
        _set_ownership(module, owned={"mine"})
        monkeypatch.setattr(module, "get_request_json", AsyncMock(return_value={"question": "q", "kb_ids": ["mine"]}))
        gen = AsyncMock(return_value={"mind_map": {"root": "ok"}})
        monkeypatch.setattr(module, "gen_mindmap", gen)

        result = asyncio.run(module.mindmap())

        gen.assert_awaited_once()
        assert result == {"code": 0, "message": "", "data": {"mind_map": {"root": "ok"}}}

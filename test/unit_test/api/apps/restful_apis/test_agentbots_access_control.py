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
"""Regression tests for cross-tenant access control on the SDK agent-bot routes
(`api/apps/restful_apis/bot_api.py`).

`POST /agentbots/<agent_id>/completions` and `GET /agentbots/<agent_id>/inputs`
authenticate with a beta API token (which only yields the caller's tenant_id)
and then load/run the agent named in the URL. They must reject an `agent_id`
the caller's tenant cannot access (`UserCanvasService.accessible`) instead of
loading or executing another tenant's agent.
"""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

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


async def _passthrough_thread_pool_exec(fn, *args, **kwargs):
    return fn(*args, **kwargs)


def _load_bot_api(monkeypatch, *, accessible, calls):
    """Load bot_api.py with the minimum stubs required.

    `accessible` is what the stubbed `UserCanvasService.accessible` returns.
    `calls` is a dict used to record whether the agent-loading paths were hit.
    """
    user_canvas_service = SimpleNamespace(
        accessible=lambda *_a, **_k: accessible,
        get_by_id=lambda agent_id: (calls.__setitem__("get_by_id", agent_id), (True, SimpleNamespace(dsl="{}", id=agent_id, title="t", avatar="a")))[1],
        query=lambda **_kwargs: [],
    )

    def _completion(*_a, **_k):
        calls["completion"] = True

        async def _gen():
            yield 'data: {"event":"message","data":{"content":"ok"}}\n\n'
            yield 'data: {"event":"message_end","data":{"content":"ok"}}\n\n'

        return _gen()

    _stub(monkeypatch, "quart", Response=lambda *a, **k: SimpleNamespace(headers=SimpleNamespace(add_header=lambda *aa, **kk: None)), request=SimpleNamespace())
    _stub(monkeypatch, "api.apps", AUTH_BETA="beta", login_required=lambda *_a, **_k: lambda func: func)
    _stub(monkeypatch, "agent.canvas", Canvas=lambda *a, **k: SimpleNamespace(get_component_input_form=lambda _n: {}, get_prologue=lambda: "", get_mode=lambda: "agent"))
    _stub(monkeypatch, "api.db.db_models", APIToken=SimpleNamespace(query=lambda **_k: [SimpleNamespace(tenant_id="attacker-tenant")]))
    _stub(monkeypatch, "api.db.services.api_service", API4ConversationService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.canvas_service", UserCanvasService=user_canvas_service, completion=_completion)
    _stub(monkeypatch, "api.db.services.user_canvas_version", UserCanvasVersionService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.conversation_service", async_iframe_completion=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.dialog_service", DialogService=SimpleNamespace(), async_ask=lambda *_a, **_k: None, gen_mindmap=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.knowledgebase_service", KnowledgebaseService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=SimpleNamespace())
    _stub(monkeypatch, "common.metadata_utils", apply_meta_data_filter=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.search_service", SearchService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.user_service",
        TenantService=SimpleNamespace(),
        UserTenantService=SimpleNamespace(),
    )
    _stub(monkeypatch, "api.db.joint_services.tenant_model_service", get_tenant_default_model_by_type=lambda *_a, **_k: None, get_model_config_from_provider_instance=lambda *_a, **_k: None)
    _stub(monkeypatch, "common.misc_utils", get_uuid=lambda: "uuid", thread_pool_exec=_passthrough_thread_pool_exec)
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        add_tenant_id_to_kwargs=lambda func: func,
        check_duplicate_ids=lambda *_a, **_k: None,
        get_error_data_result=lambda message="Sorry", **_k: {"code": 102, "message": message, "data": None},
        get_json_result=lambda code=0, message="", data=None: {"code": code, "message": message, "data": data},
        get_result=lambda **kwargs: {"code": 0, "data": kwargs.get("data")},
        get_request_json=_async_empty_json,
        server_error_response=lambda exc: {"code": 500, "message": str(exc)},
        token_required=lambda func: func,
        validate_request=lambda *_a, **_k: lambda func: func,
    )
    _stub(monkeypatch, "rag.app.tag", label_question=lambda *_a, **_k: None)
    _stub(monkeypatch, "rag.prompts.template", load_prompt=lambda *_a, **_k: "")
    _stub(monkeypatch, "rag.prompts.generator", cross_languages=lambda *_a, **_k: None, keyword_extraction=lambda *_a, **_k: None)
    _stub(monkeypatch, "common.constants", RetCode=SimpleNamespace(), LLMType=SimpleNamespace(), StatusEnum=SimpleNamespace())
    _stub(monkeypatch, "common", settings=SimpleNamespace())
    _stub(monkeypatch, "common.settings", retriever=SimpleNamespace(), kg_retriever=SimpleNamespace())
    _stub(monkeypatch, "api.utils.reference_metadata_utils", enrich_chunks_with_document_metadata=lambda *_a, **_k: None, resolve_reference_metadata_preferences=lambda *_a, **_k: None)

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "bot_api.py"
    spec = importlib.util.spec_from_file_location("test_agentbots_bot_api", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_agentbots_bot_api", module)
    spec.loader.exec_module(module)
    return module


async def _async_empty_json():
    return {"stream": False}


@pytest.mark.p1
class TestAgentBotAccessControl:
    """Regression: agentbots completions/inputs must enforce tenant ownership."""

    @pytest.mark.p1
    def test_begin_inputs_denied_for_inaccessible_agent(self, monkeypatch):
        calls = {}
        module = _load_bot_api(monkeypatch, accessible=False, calls=calls)

        result = asyncio.run(module.begin_inputs(agent_id="victim-agent"))

        assert result == {"code": 102, "message": "Can't find agent by ID: victim-agent", "data": None}
        # Must short-circuit before ever loading the foreign agent.
        assert "get_by_id" not in calls

    @pytest.mark.p1
    def test_completions_denied_for_inaccessible_agent(self, monkeypatch):
        calls = {}
        module = _load_bot_api(monkeypatch, accessible=False, calls=calls)

        result = asyncio.run(module.agent_bot_completions(agent_id="victim-agent"))

        assert result == {"code": 102, "message": "Can't find agent by ID: victim-agent", "data": None}
        # Must short-circuit before ever running the foreign agent.
        assert "completion" not in calls

    @pytest.mark.p1
    def test_begin_inputs_allowed_for_accessible_agent(self, monkeypatch):
        calls = {}
        module = _load_bot_api(monkeypatch, accessible=True, calls=calls)

        result = asyncio.run(module.begin_inputs(agent_id="own-agent"))

        assert calls.get("get_by_id") == "own-agent"
        assert result["code"] == 0

    @pytest.mark.p1
    def test_completions_allowed_for_accessible_agent(self, monkeypatch):
        calls = {}
        module = _load_bot_api(monkeypatch, accessible=True, calls=calls)

        result = asyncio.run(module.agent_bot_completions(agent_id="own-agent"))

        assert calls.get("completion") is True
        assert result["code"] == 0

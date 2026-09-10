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
"""Regression tests for session ownership in
`POST /api/v1/chats/{chat_id}/sessions` (api/apps/restful_apis/chat_api.py).

An API key belongs to a backend serving its own end users, so it may attribute a
session to one of them. A JWT or session caller is an end user in its own right
and stays pinned to the authenticated principal, otherwise a browser client
could create sessions attributed to somebody else.
"""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest

AUTH_JWT = "JWT"
AUTH_API = "API"
TENANT_ID = "tenant-key-owner"


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _load_chat_api(monkeypatch, auth_type, request_json, saved):
    """Load chat_api.py with the minimum stubs `create_session` needs."""

    async def _get_request_json():
        return request_json

    def _save(**conv):
        saved.append(conv)

    def _get_conversation(conv_id):
        for conv in saved:
            if conv["id"] == conv_id:
                return True, SimpleNamespace(to_dict=lambda conv=conv: dict(conv))
        return False, None

    _stub(
        monkeypatch,
        "quart",
        Response=object,
        g=SimpleNamespace(auth_type=auth_type),
        request=SimpleNamespace(args={}),
    )
    _stub(
        monkeypatch,
        "api.apps",
        AUTH_API=AUTH_API,
        AUTH_JWT=AUTH_JWT,
        current_user=SimpleNamespace(id=TENANT_ID),
        login_required=lambda func=None, **_kwargs: (lambda f: f) if func is None else func,
    )
    _stub(
        monkeypatch,
        "api.apps.restful_apis._generation_params",
        merge_generation_config=lambda *_a, **_k: {},
        pop_generation_config=lambda *_a, **_k: {},
        resolve_llm_setting=lambda *_a, **_k: {},
    )
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_api_key=lambda *_a, **_k: "",
        get_composite_model_name_by_id=lambda *_a, **_k: "",
        get_model_config_by_id=lambda *_a, **_k: {},
        get_tenant_default_model_by_type=lambda *_a, **_k: {},
        resolve_model_config=lambda *_a, **_k: {},
        resolve_model_id=lambda *_a, **_k: "",
    )
    _stub(monkeypatch, "api.db.services.chunk_feedback_service", ChunkFeedbackService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.conversation_service",
        ConversationService=SimpleNamespace(save=_save, get_by_id=_get_conversation),
        structure_answer=lambda *_a, **_k: {},
    )
    _stub(
        monkeypatch,
        "api.db.services.dialog_service",
        DialogService=SimpleNamespace(
            get_by_id=lambda _chat_id: (True, SimpleNamespace(prompt_config={"prologue": "Hello"})),
            query=lambda **_kwargs: [SimpleNamespace(id="chat-1")],
            model=SimpleNamespace(_meta=SimpleNamespace(fields={"id": None, "name": None})),
        ),
        gen_mindmap=lambda *_a, **_k: {},
        rag_agent=lambda *_a, **_k: None,
    )
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(),
        validate_dataset_embedding_models=lambda *_a, **_k: None,
    )
    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.search_service", SearchService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.user_service", TenantService=SimpleNamespace(), UserTenantService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        check_duplicate_ids=lambda ids, _kind="item": (ids, []),
        get_data_error_result=lambda message="Sorry": {"code": 102, "message": message, "data": None},
        get_json_result=lambda code=0, message="", data=None: {"code": code, "message": message, "data": data},
        get_request_json=_get_request_json,
        server_error_response=lambda exc: {"code": 500, "message": str(exc), "data": None},
        validate_request=lambda *_a, **_k: lambda func: func,
    )
    _stub(monkeypatch, "rag.prompts.generator", chunks_format=lambda *_a, **_k: [])
    _stub(monkeypatch, "rag.prompts.template", load_prompt=lambda *_a, **_k: "")

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "chat_api.py"
    spec = importlib.util.spec_from_file_location("test_create_session_user_id_chat_api", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_create_session_user_id_chat_api", module)
    spec.loader.exec_module(module)
    return module


async def _create(monkeypatch, auth_type, request_json):
    saved = []
    module = _load_chat_api(monkeypatch, auth_type, request_json, saved)
    response = await module.create_session("chat-1")
    return response, saved


@pytest.mark.p1
@pytest.mark.asyncio
async def test_api_key_caller_owns_session_by_supplied_user_id(monkeypatch):
    """The end-user id a trusted backend sends is what gets stored."""
    response, saved = await _create(monkeypatch, AUTH_API, {"name": "s", "user_id": "user_2606165351808"})

    assert response["code"] == 0
    assert saved[0]["user_id"] == "user_2606165351808"
    assert response["data"]["user_id"] == "user_2606165351808"


@pytest.mark.p1
@pytest.mark.asyncio
async def test_jwt_caller_cannot_attribute_session_to_another_user(monkeypatch):
    """A browser client's `user_id` is ignored; the JWT principal wins."""
    _, saved = await _create(monkeypatch, AUTH_JWT, {"name": "s", "user_id": "somebody-else"})

    assert saved[0]["user_id"] == TENANT_ID


@pytest.mark.p1
@pytest.mark.asyncio
async def test_jwt_caller_is_not_validated_on_a_field_it_never_uses(monkeypatch):
    """`user_id` is documented as ignored under login-token auth, so a malformed
    one must not turn a request that used to succeed into an argument error."""
    response, saved = await _create(monkeypatch, AUTH_JWT, {"name": "s", "user_id": 12345})

    assert response["code"] == 0
    assert saved[0]["user_id"] == TENANT_ID


@pytest.mark.p1
@pytest.mark.asyncio
async def test_api_key_caller_without_user_id_falls_back_to_key_owner(monkeypatch):
    """Callers that never sent `user_id` keep the behaviour they have today."""
    _, saved = await _create(monkeypatch, AUTH_API, {"name": "s"})

    assert saved[0]["user_id"] == TENANT_ID


@pytest.mark.p1
@pytest.mark.asyncio
async def test_blank_user_id_falls_back_instead_of_storing_whitespace(monkeypatch):
    _, saved = await _create(monkeypatch, AUTH_API, {"name": "s", "user_id": "   "})

    assert saved[0]["user_id"] == TENANT_ID


@pytest.mark.p1
@pytest.mark.asyncio
async def test_non_string_user_id_is_rejected_without_creating_a_session(monkeypatch):
    response, saved = await _create(monkeypatch, AUTH_API, {"name": "s", "user_id": 12345})

    assert response["code"] == 102
    assert "user_id" in response["message"]
    assert saved == []


@pytest.mark.p1
@pytest.mark.asyncio
async def test_user_id_is_truncated_to_the_column_width(monkeypatch):
    """`Conversation.user_id` is a CharField(max_length=255); an over-long id
    must be cut rather than blowing up on insert."""
    _, saved = await _create(monkeypatch, AUTH_API, {"name": "s", "user_id": "u" * 300})

    assert saved[0]["user_id"] == "u" * 255

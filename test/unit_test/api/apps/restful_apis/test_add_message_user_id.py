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
"""Regression tests for message attribution in `POST /api/v1/messages`
(api/apps/restful_apis/memory_api.py).

An API key belongs to an agent backend talking to RAGFlow on behalf of its own
end users, so the `user_id` it sends is the subject the memory is written
against. A JWT or session caller is an end user in its own right and stays
pinned to the authenticated principal so it cannot write memory attributed to
somebody else.
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


def _load_memory_api(monkeypatch, auth_type, request_json, added):
    """Load memory_api.py with the minimum stubs `add_message` needs."""

    async def _get_request_json():
        return request_json

    async def _add_message(memory_ids, message_dict):
        added.append((memory_ids, message_dict))
        return True, "ok"

    _stub(
        monkeypatch,
        "quart",
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
    _stub(monkeypatch, "api.apps.services", memory_api_service=SimpleNamespace(add_message=_add_message))
    _stub(monkeypatch, "api.db.joint_services.tenant_model_service", ensure_tenant_model_ids_for_params=lambda *_a, **_k: {})
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        get_error_argument_result=lambda message="Sorry": {"code": 101, "message": message, "data": None},
        get_json_result=lambda code=0, message="", data=None: {"code": code, "message": message, "data": data},
        get_request_json=_get_request_json,
        validate_request=lambda *_a, **_k: lambda func: func,
    )

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "memory_api.py"
    spec = importlib.util.spec_from_file_location("test_add_message_user_id_memory_api", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_add_message_user_id_memory_api", module)
    spec.loader.exec_module(module)
    return module


async def _add(monkeypatch, auth_type, extra):
    added = []
    request_json = {
        "memory_id": ["mem-1"],
        "agent_id": "agent-1",
        "session_id": "session-1",
        "user_input": "hi",
        "agent_response": "hello",
        **extra,
    }
    module = _load_memory_api(monkeypatch, auth_type, request_json, added)
    response = await module.add_message()
    return response, added


@pytest.mark.p1
@pytest.mark.asyncio
async def test_api_key_caller_attributes_the_message_to_its_end_user(monkeypatch):
    """The subject a trusted agent backend sends is what the memory is written
    against. Before the fix this branch was unreachable, because it keyed off
    `g.auth_via_api_token`, which the Python auth layer never sets."""
    response, added = await _add(monkeypatch, AUTH_API, {"user_id": "end-user-42"})

    assert response["code"] == 0
    assert added[0][1]["user_id"] == "end-user-42"


@pytest.mark.p1
@pytest.mark.asyncio
async def test_jwt_caller_cannot_attribute_a_message_to_another_user(monkeypatch):
    """The guarantee #14745 was written for: a browser client's `user_id` loses
    to the authenticated principal."""
    _, added = await _add(monkeypatch, AUTH_JWT, {"user_id": "somebody-else"})

    assert added[0][1]["user_id"] == TENANT_ID


@pytest.mark.p1
@pytest.mark.asyncio
async def test_api_key_caller_without_user_id_falls_back_to_the_key_owner(monkeypatch):
    """`user_id` is optional, so omitting it must not write an empty subject."""
    _, added = await _add(monkeypatch, AUTH_API, {})

    assert added[0][1]["user_id"] == TENANT_ID


@pytest.mark.p1
@pytest.mark.asyncio
async def test_blank_and_non_string_user_id_fall_back_instead_of_being_stored(monkeypatch):
    _, added = await _add(monkeypatch, AUTH_API, {"user_id": "   "})
    assert added[0][1]["user_id"] == TENANT_ID

    _, added = await _add(monkeypatch, AUTH_API, {"user_id": 12345})
    assert added[0][1]["user_id"] == TENANT_ID


@pytest.mark.p1
@pytest.mark.asyncio
async def test_surrounding_whitespace_is_stripped_from_the_subject(monkeypatch):
    _, added = await _add(monkeypatch, AUTH_API, {"user_id": "  end-user-42  "})

    assert added[0][1]["user_id"] == "end-user-42"

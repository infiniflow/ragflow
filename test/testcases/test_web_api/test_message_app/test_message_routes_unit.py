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

import asyncio
import importlib.util
import inspect
import sys
from copy import deepcopy
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


class _DummyArgs(dict):
    def getlist(self, key):
        value = self.get(key)
        if value is None:
            return []
        if isinstance(value, list):
            return value
        return [value]


class _DummyMemoryApiService:
    async def add_message(self, *_args, **_kwargs):
        return True, "ok"

    async def get_messages(self, *_args, **_kwargs):
        return []


def _run(coro):
    return asyncio.run(coro)


def _load_memory_routes_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="user-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    services_mod = ModuleType("api.apps.services")
    services_mod.memory_api_service = _DummyMemoryApiService()
    monkeypatch.setitem(sys.modules, "api.apps.services", services_mod)

    module_name = "test_message_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "memory_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _set_request_json(monkeypatch, module, payload):
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(deepcopy(payload)))


@pytest.mark.p2
def test_add_message_partial_failure_branch(monkeypatch):
    module = _load_memory_routes_module(monkeypatch)

    _set_request_json(
        monkeypatch,
        module,
        {
            "memory_id": ["memory-1"],
            "agent_id": "agent-1",
            "session_id": "session-1",
            "user_input": "hello",
            "agent_response": "world",
        },
    )

    async def _add_message(_memory_ids, _message_dict):
        return False, "cannot enqueue"

    monkeypatch.setattr(module.memory_api_service, "add_message", _add_message)

    res = _run(inspect.unwrap(module.add_message)())
    assert res["code"] == module.RetCode.SERVER_ERROR, res
    assert "Some messages failed to add" in res["message"], res


@pytest.mark.p2
def test_get_messages_csv_and_missing_memory_ids(monkeypatch):
    module = _load_memory_routes_module(monkeypatch)

    monkeypatch.setattr(module, "request", SimpleNamespace(args=_DummyArgs({})))
    res = _run(inspect.unwrap(module.get_messages)())
    assert res["code"] == module.RetCode.ARGUMENT_ERROR, res
    assert "memory_ids is required." in res["message"], res

    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(args=_DummyArgs({"memory_id": "m1,m2", "agent_id": "a1", "session_id": "s1", "limit": "5"})),
    )

    async def _get_messages(memory_ids, agent_id, session_id, limit):
        assert memory_ids == ["m1", "m2"]
        assert agent_id == "a1"
        assert session_id == "s1"
        assert limit == 5
        return [{"message_id": 1}]

    monkeypatch.setattr(module.memory_api_service, "get_messages", _get_messages)
    res = _run(inspect.unwrap(module.get_messages)())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert isinstance(res["data"], list), res

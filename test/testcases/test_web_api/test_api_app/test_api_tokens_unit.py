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
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _ExprField:
    def __init__(self, name):
        self.name = name

    def __eq__(self, other):
        return (self.name, other)


class _DummyAPITokenModel:
    tenant_id = _ExprField("tenant_id")
    token = _ExprField("token")


def _run(coro):
    return asyncio.run(coro)


def _load_api_app(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    quart_mod = ModuleType("quart")
    quart_mod.request = SimpleNamespace(args={})
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.login_required = lambda fn: fn
    apps_mod.current_user = SimpleNamespace(id="user-1")
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")

    async def _get_request_json():
        return {}

    api_utils_mod.generate_confirmation_token = lambda: "token-123"
    api_utils_mod.get_request_json = _get_request_json
    api_utils_mod.get_json_result = lambda data=None, message="", code=0: {
        "code": code,
        "message": message,
        "data": data,
    }
    api_utils_mod.get_data_error_result = lambda message="", code=400, data=None: {
        "code": code,
        "message": message,
        "data": data,
    }
    api_utils_mod.server_error_response = lambda exc: {
        "code": 500,
        "message": str(exc),
        "data": None,
    }
    api_utils_mod.validate_request = lambda *_args, **_kwargs: (lambda fn: fn)
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    api_service_mod = ModuleType("api.db.services.api_service")

    class _StubAPITokenService:
        @staticmethod
        def save(**_kwargs):
            return True

        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def filter_delete(_conds):
            return True

    class _StubAPI4ConversationService:
        @staticmethod
        def stats(*_args, **_kwargs):
            return []

    api_service_mod.APITokenService = _StubAPITokenService
    api_service_mod.API4ConversationService = _StubAPI4ConversationService
    monkeypatch.setitem(sys.modules, "api.db.services.api_service", api_service_mod)

    user_service_mod = ModuleType("api.db.services.user_service")

    class _StubUserTenantService:
        @staticmethod
        def query(**_kwargs):
            return [SimpleNamespace(tenant_id="tenant-1")]

    user_service_mod.UserTenantService = _StubUserTenantService
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)

    db_models_mod = ModuleType("api.db.db_models")
    db_models_mod.APIToken = _DummyAPITokenModel
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    time_utils_mod = ModuleType("common.time_utils")
    time_utils_mod.current_timestamp = lambda: 123
    time_utils_mod.datetime_format = lambda _dt: "2026-01-01 00:00:00"
    monkeypatch.setitem(sys.modules, "common.time_utils", time_utils_mod)

    module_path = repo_root / "api" / "apps" / "api_app.py"
    spec = importlib.util.spec_from_file_location("test_api_tokens_unit_module", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_new_token_branches_and_error_paths(monkeypatch):
    module = _load_api_app(monkeypatch)

    async def req_canvas():
        return {"canvas_id": "canvas-1"}

    monkeypatch.setattr(module, "get_request_json", req_canvas)
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [])
    res = _run(module.new_token())
    assert res["message"] == "Tenant not found!"

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module.APITokenService, "save", lambda **_kwargs: True)
    res = _run(module.new_token())
    assert res["code"] == 0
    assert res["data"]["tenant_id"] == "tenant-1"
    assert res["data"]["dialog_id"] == "canvas-1"
    assert res["data"]["source"] == "agent"

    monkeypatch.setattr(module.APITokenService, "save", lambda **_kwargs: False)
    res = _run(module.new_token())
    assert res["message"] == "Fail to new a dialog!"

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: (_ for _ in ()).throw(RuntimeError("query failed")))
    res = _run(module.new_token())
    assert res["code"] == 500
    assert "query failed" in res["message"]


@pytest.mark.p2
def test_token_list_tenant_guard_and_exception(monkeypatch):
    module = _load_api_app(monkeypatch)

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module, "request", SimpleNamespace(args={"dialog_id": "d1"}))
    res = module.token_list()
    assert res["message"] == "Tenant not found!"

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module, "request", SimpleNamespace(args={}))
    res = module.token_list()
    assert res["code"] == 500
    assert "canvas_id" in res["message"]


@pytest.mark.p2
def test_rm_exception_path(monkeypatch):
    module = _load_api_app(monkeypatch)

    async def req_rm():
        return {"tokens": ["tok-1"], "tenant_id": "tenant-1"}

    monkeypatch.setattr(module, "get_request_json", req_rm)
    monkeypatch.setattr(
        module.APITokenService,
        "filter_delete",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("delete failed")),
    )

    res = _run(module.rm())
    assert res["code"] == 500
    assert "delete failed" in res["message"]


@pytest.mark.p2
def test_stats_aggregation_and_error_paths(monkeypatch):
    module = _load_api_app(monkeypatch)

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module, "request", SimpleNamespace(args={}))
    res = module.stats()
    assert res["message"] == "Tenant not found!"

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module, "request", SimpleNamespace(args={"canvas_id": "canvas-1"}))
    monkeypatch.setattr(
        module.API4ConversationService,
        "stats",
        lambda *_args, **_kwargs: [
            {
                "dt": "2026-01-01",
                "pv": 3,
                "uv": 2,
                "tokens": 100,
                "duration": 9.9,
                "round": 1,
                "thumb_up": 0,
            }
        ],
    )
    res = module.stats()
    assert res["code"] == 0
    assert res["data"]["pv"] == [("2026-01-01", 3)]
    assert res["data"]["uv"] == [("2026-01-01", 2)]
    assert res["data"]["round"] == [("2026-01-01", 1)]
    assert res["data"]["thumb_up"] == [("2026-01-01", 0)]
    assert res["data"]["tokens"] == [("2026-01-01", 0.1)]
    assert res["data"]["speed"] == [("2026-01-01", 10.0)]

    monkeypatch.setattr(
        module.API4ConversationService,
        "stats",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("stats failed")),
    )
    res = module.stats()
    assert res["code"] == 500
    assert "stats failed" in res["message"]

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
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import Mock

import pytest


ROUTE_PASSWORD_SENTINEL = "cfg703-password-'quoted![]{}'"
ROUTE_DSN_SENTINEL = "postgresql://cfg703-user:cfg703-dsn%27%22@db.example:19995/private"
ROUTE_TOKEN_SENTINEL = "cfg703-token-'quoted![]{}'"


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


def _load_system_module(monkeypatch, *, apps_module=None, manager=None):
    repo_root = Path(__file__).resolve().parents[4]

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    if apps_module is None:
        apps_mod = ModuleType("api.apps")
        apps_mod.__path__ = [str(repo_root / "api" / "apps")]
        apps_mod.login_required = lambda fn: fn
        apps_mod.current_user = SimpleNamespace(id="user-1")
    else:
        apps_mod = apps_module
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    settings_mod = ModuleType("common.settings")
    settings_mod.docStoreConn = SimpleNamespace(health=lambda: {"type": "doc", "status": "green"})
    settings_mod.STORAGE_IMPL = SimpleNamespace(health=lambda: True)
    settings_mod.STORAGE_IMPL_TYPE = "MINIO"
    settings_mod.DATABASE_TYPE = "MYSQL"
    settings_mod.REGISTER_ENABLED = True
    settings_mod.DISABLE_PASSWORD_LOGIN = False
    common_pkg.settings = settings_mod
    monkeypatch.setitem(sys.modules, "common.settings", settings_mod)

    versions_mod = ModuleType("common.versions")
    versions_mod.get_ragflow_version = lambda: "0.0.0-unit"
    monkeypatch.setitem(sys.modules, "common.versions", versions_mod)

    time_utils_mod = ModuleType("common.time_utils")
    time_utils_mod.current_timestamp = lambda: 111
    time_utils_mod.datetime_format = lambda _dt: "2026-01-01 00:00:00"
    monkeypatch.setitem(sys.modules, "common.time_utils", time_utils_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")
    api_utils_mod.get_json_result = lambda data=None, message="success", code=0: {
        "code": code,
        "message": message,
        "data": data,
    }
    api_utils_mod.get_data_error_result = lambda message="", code=102, data=None: {
        "code": code,
        "message": message,
        "data": data,
    }
    api_utils_mod.server_error_response = lambda exc: {
        "code": 100,
        "message": repr(exc),
        "data": None,
    }
    api_utils_mod.generate_confirmation_token = lambda: "ragflow-abcdefghijklmnopqrstuvwxyz0123456789"
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    api_service_mod = ModuleType("api.db.services.api_service")
    api_service_mod.APITokenService = SimpleNamespace(
        save=lambda **_kwargs: True,
        query=lambda **_kwargs: [],
        filter_update=lambda *_args, **_kwargs: True,
        filter_delete=lambda *_args, **_kwargs: True,
    )
    monkeypatch.setitem(sys.modules, "api.db.services.api_service", api_service_mod)

    kb_service_mod = ModuleType("api.db.services.knowledgebase_service")
    kb_service_mod.KnowledgebaseService = SimpleNamespace(get_by_id=lambda _kb_id: True)
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", kb_service_mod)

    user_service_mod = ModuleType("api.db.services.user_service")
    user_service_mod.UserTenantService = SimpleNamespace(query=lambda **_kwargs: [SimpleNamespace(role="owner", tenant_id="tenant-1")])
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)

    db_models_mod = ModuleType("api.db.db_models")
    db_models_mod.APIToken = _DummyAPITokenModel
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    rag_pkg = ModuleType("rag")
    rag_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag", rag_pkg)

    rag_utils_pkg = ModuleType("rag.utils")
    rag_utils_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag.utils", rag_utils_pkg)

    redis_mod = ModuleType("rag.utils.redis_conn")
    redis_mod.REDIS_CONN = SimpleNamespace(
        health=lambda: True,
        smembers=lambda *_args, **_kwargs: set(),
        zrangebyscore=lambda *_args, **_kwargs: [],
    )
    monkeypatch.setitem(sys.modules, "rag.utils.redis_conn", redis_mod)

    health_utils_mod = ModuleType("api.utils.health_utils")
    health_utils_mod.run_health_checks = lambda: ({"status": "ok"}, True)
    health_utils_mod.get_oceanbase_status = lambda: {"status": "alive"}
    health_utils_mod.get_gaussdb_status = lambda: {"status": "alive"}
    monkeypatch.setitem(sys.modules, "api.utils.health_utils", health_utils_mod)

    quart_mod = ModuleType("quart")
    quart_mod.jsonify = lambda payload: payload
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    module_path = repo_root / "api" / "apps" / "restful_apis" / "system_api.py"
    spec = importlib.util.spec_from_file_location("test_gaussdb_system_routes_module", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = manager or _DummyManager()
    monkeypatch.setitem(sys.modules, "test_gaussdb_system_routes_module", module)
    spec.loader.exec_module(module)
    return module


def _load_system_http_app(monkeypatch):
    from test.testcases.test_web_api.test_system_app.test_apps_init_unit import _load_apps_module

    quart_app, apps_module = _load_apps_module(monkeypatch)
    module = _load_system_module(monkeypatch, apps_module=apps_module, manager=quart_app)
    return quart_app, module


def _get_without_auth(quart_app, path):
    async def request():
        response = await quart_app.test_client().get(path)
        return response.status_code, await response.get_json()

    return asyncio.run(request())


@pytest.mark.p1
def test_tc_cfg_704_status_branch_matrix_unit(monkeypatch):
    module = _load_system_module(monkeypatch)
    expected = {
        "db": "ok",
        "redis": "ok",
        "doc_engine": "ok",
        "storage": "ok",
        "status": "ok",
    }
    monkeypatch.setattr(module, "run_health_checks", lambda: (expected, True))

    payload, status_code = module.healthz()

    assert status_code == 200
    assert payload == expected


@pytest.mark.p1
def test_tc_cfg_705_healthz_returns_500_when_any_check_is_not_ok(monkeypatch):
    module = _load_system_module(monkeypatch)
    expected = {
        "db": "ok",
        "redis": "ok",
        "doc_engine": "nok",
        "storage": "ok",
        "status": "nok",
        "_meta": {"doc_engine": {"error": "doc down"}},
    }
    monkeypatch.setattr(module, "run_health_checks", lambda: (expected, False))

    payload, status_code = module.healthz()

    assert status_code == 500
    assert payload == expected


@pytest.mark.p1
def test_tc_cfg_706_healthz_route_allows_real_unauthenticated_request(monkeypatch):
    quart_app, module = _load_system_http_app(monkeypatch)
    expected = {"status": "ok", "doc_engine": "ok"}
    monkeypatch.setattr(module, "run_health_checks", lambda: (expected, True))

    status_code, payload = _get_without_auth(quart_app, "/system/healthz")

    assert status_code == 200
    assert payload == expected


@pytest.mark.p0
def test_tc_cfg_702_gaussdb_status_route_rejects_real_unauthenticated_request(monkeypatch):
    quart_app, module = _load_system_http_app(monkeypatch)
    probe = Mock(return_value={"status": "alive"})
    monkeypatch.setattr(module, "get_gaussdb_status", probe)

    status_code, payload = _get_without_auth(quart_app, "/system/gaussdb/status")

    assert status_code == 401
    assert payload["code"] == 401
    assert "Unauthorized" in payload["message"]
    probe.assert_not_called()


@pytest.mark.p1
def test_tc_cfg_712_gaussdb_status_route_returns_probe_payload(monkeypatch):
    module = _load_system_module(monkeypatch)
    monkeypatch.setattr(module, "get_gaussdb_status", lambda: {"status": "alive"})

    res = module.gaussdb_status()

    assert res["code"] == 0
    assert res == {"code": 0, "message": "success", "data": {"status": "alive"}}


@pytest.mark.p1
def test_tc_cfg_703_gaussdb_status_route_returns_500_when_probe_raises(monkeypatch, caplog):
    module = _load_system_module(monkeypatch)

    def raise_probe_error():
        raise RuntimeError(f"probe failed password=\"{ROUTE_PASSWORD_SENTINEL}\" dsn={ROUTE_DSN_SENTINEL} access_token='{ROUTE_TOKEN_SENTINEL}'")

    monkeypatch.setattr(module, "get_gaussdb_status", raise_probe_error)
    caplog.set_level("ERROR")

    res = module.gaussdb_status()
    serialized = json.dumps(res, sort_keys=True)
    log_text = caplog.text

    assert res["code"] == 500
    assert res["data"]["status"] == "error"
    assert res["data"]["message"].startswith("Failed to get GaussDB status: probe failed")
    assert "***" in res["data"]["message"]
    assert ROUTE_PASSWORD_SENTINEL not in serialized
    assert ROUTE_DSN_SENTINEL not in serialized
    assert ROUTE_TOKEN_SENTINEL not in serialized
    assert "postgresql://" not in serialized
    assert "cfg703-user" not in serialized
    assert "GaussDB status route failed (RuntimeError)" in log_text
    assert "***" in log_text
    assert ROUTE_PASSWORD_SENTINEL not in log_text
    assert ROUTE_DSN_SENTINEL not in log_text
    assert ROUTE_TOKEN_SENTINEL not in log_text

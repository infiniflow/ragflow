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


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


def _load_system_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.login_required = lambda fn: fn
    apps_mod.current_user = SimpleNamespace(id="user-1")
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
    user_service_mod.UserTenantService = SimpleNamespace(
        query=lambda **_kwargs: [SimpleNamespace(role="owner", tenant_id="tenant-1")]
    )
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
    monkeypatch.setitem(sys.modules, "api.utils.health_utils", health_utils_mod)

    quart_mod = ModuleType("quart")
    quart_mod.jsonify = lambda payload: payload
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    module_path = repo_root / "api" / "apps" / "system_app.py"
    spec = importlib.util.spec_from_file_location("test_system_routes_unit_module", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, "test_system_routes_unit_module", module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_status_branch_matrix_unit(monkeypatch):
    module = _load_system_module(monkeypatch)

    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(health=lambda: {"type": "es", "status": "green"}))
    monkeypatch.setattr(module.settings, "STORAGE_IMPL", SimpleNamespace(health=lambda: True))
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: True)
    monkeypatch.setattr(module.REDIS_CONN, "health", lambda: True)
    monkeypatch.setattr(module.REDIS_CONN, "smembers", lambda _key: {"executor-1"})
    monkeypatch.setattr(module.REDIS_CONN, "zrangebyscore", lambda *_args, **_kwargs: ['{"beat": 1}'])

    res = module.status()
    assert res["code"] == 0
    assert res["data"]["doc_engine"]["status"] == "green"
    assert res["data"]["storage"]["status"] == "green"
    assert res["data"]["database"]["status"] == "green"
    assert res["data"]["redis"]["status"] == "green"
    assert res["data"]["task_executor_heartbeats"]["executor-1"][0]["beat"] == 1

    monkeypatch.setattr(
        module.settings,
        "docStoreConn",
        SimpleNamespace(health=lambda: (_ for _ in ()).throw(RuntimeError("doc down"))),
    )
    monkeypatch.setattr(
        module.settings,
        "STORAGE_IMPL",
        SimpleNamespace(health=lambda: (_ for _ in ()).throw(RuntimeError("storage down"))),
    )
    monkeypatch.setattr(
        module.KnowledgebaseService,
        "get_by_id",
        lambda _kb_id: (_ for _ in ()).throw(RuntimeError("db down")),
    )
    monkeypatch.setattr(module.REDIS_CONN, "health", lambda: False)
    monkeypatch.setattr(module.REDIS_CONN, "smembers", lambda _key: (_ for _ in ()).throw(RuntimeError("hb down")))

    res = module.status()
    assert res["code"] == 0
    assert res["data"]["doc_engine"]["status"] == "red"
    assert "doc down" in res["data"]["doc_engine"]["error"]
    assert res["data"]["storage"]["status"] == "red"
    assert "storage down" in res["data"]["storage"]["error"]
    assert res["data"]["database"]["status"] == "red"
    assert "db down" in res["data"]["database"]["error"]
    assert res["data"]["redis"]["status"] == "red"
    assert "Lost connection!" in res["data"]["redis"]["error"]
    assert res["data"]["task_executor_heartbeats"] == {}


@pytest.mark.p2
def test_healthz_and_oceanbase_status_matrix_unit(monkeypatch):
    module = _load_system_module(monkeypatch)

    monkeypatch.setattr(module, "run_health_checks", lambda: ({"status": "ok"}, True))
    payload, status = module.healthz()
    assert status == 200
    assert payload["status"] == "ok"

    monkeypatch.setattr(module, "run_health_checks", lambda: ({"status": "degraded"}, False))
    payload, status = module.healthz()
    assert status == 500
    assert payload["status"] == "degraded"

    monkeypatch.setattr(module, "get_oceanbase_status", lambda: {"status": "alive", "latency_ms": 8})
    res = module.oceanbase_status()
    assert res["code"] == 0
    assert res["data"]["status"] == "alive"

    monkeypatch.setattr(module, "get_oceanbase_status", lambda: (_ for _ in ()).throw(RuntimeError("ocean boom")))
    res = module.oceanbase_status()
    assert res["code"] == 500
    assert res["data"]["status"] == "error"
    assert "ocean boom" in res["data"]["message"]


@pytest.mark.p2
def test_system_token_routes_matrix_unit(monkeypatch):
    module = _load_system_module(monkeypatch)

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [])
    res = module.new_token()
    assert res["message"] == "Tenant not found!"

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(role="owner", tenant_id="tenant-1")])
    monkeypatch.setattr(module.APITokenService, "save", lambda **_kwargs: False)
    res = module.new_token()
    assert res["message"] == "Fail to new a dialog!"

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: (_ for _ in ()).throw(RuntimeError("tenant query boom")))
    res = module.new_token()
    assert res["code"] == 100
    assert "tenant query boom" in res["message"]

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [])
    res = module.token_list()
    assert res["message"] == "Tenant not found!"

    class _Token:
        def __init__(self, token, beta):
            self.token = token
            self.beta = beta

        def to_dict(self):
            return {"token": self.token, "beta": self.beta}

    filter_updates = []
    monkeypatch.setattr(module, "generate_confirmation_token", lambda: "ragflow-abcdefghijklmnopqrstuvwxyz0123456789")
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(role="owner", tenant_id="tenant-9")])
    monkeypatch.setattr(module.APITokenService, "query", lambda **_kwargs: [_Token("tok-1", ""), _Token("tok-2", "beta-2")])
    monkeypatch.setattr(module.APITokenService, "filter_update", lambda conds, payload: filter_updates.append((conds, payload)))
    res = module.token_list()
    assert res["code"] == 0
    assert len(res["data"]) == 2
    assert len(res["data"][0]["beta"]) == 32
    assert res["data"][1]["beta"] == "beta-2"
    assert len(filter_updates) == 1

    monkeypatch.setattr(
        module.APITokenService,
        "query",
        lambda **_kwargs: (_ for _ in ()).throw(RuntimeError("token list boom")),
    )
    res = module.token_list()
    assert res["code"] == 100
    assert "token list boom" in res["message"]

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [])
    res = module.rm("tok-1")
    assert res["message"] == "Tenant not found!"

    deleted = []
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(role="owner", tenant_id="tenant-3")])
    monkeypatch.setattr(module.APITokenService, "filter_delete", lambda conds: deleted.append(conds))
    res = module.rm("tok-1")
    assert res["code"] == 0
    assert res["data"] is True
    assert deleted

    monkeypatch.setattr(
        module.APITokenService,
        "filter_delete",
        lambda _conds: (_ for _ in ()).throw(RuntimeError("delete boom")),
    )
    res = module.rm("tok-1")
    assert res["code"] == 100
    assert "delete boom" in res["message"]


@pytest.mark.p2
def test_get_config_returns_register_enabled_unit(monkeypatch):
    module = _load_system_module(monkeypatch)
    monkeypatch.setattr(module.settings, "REGISTER_ENABLED", False)
    res = module.get_config()
    assert res["code"] == 0
    assert res["data"]["registerEnabled"] is False

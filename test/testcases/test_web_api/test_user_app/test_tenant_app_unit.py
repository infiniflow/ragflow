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


class _AwaitableValue:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _co():
            return self._value

        return _co().__await__()


class _ExprField:
    def __init__(self, name):
        self.name = name

    def __eq__(self, other):
        return (self.name, other)


class _Invitee:
    def __init__(self, user_id="invitee-1", email="invitee@example.com"):
        self.id = user_id
        self.email = email

    def to_dict(self):
        return {
            "id": self.id,
            "avatar": "avatar-url",
            "email": self.email,
            "nickname": "Invitee",
            "password": "ignored",
        }


def _run(coro):
    return asyncio.run(coro)


def _set_request_json(monkeypatch, module, payload):
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(payload))


def _load_tenant_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="tenant-1", email="owner@example.com")
    apps_mod.login_required = lambda fn: fn
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    db_mod = ModuleType("api.db")
    db_mod.UserTenantRole = SimpleNamespace(NORMAL="normal", OWNER="owner", INVITE="invite")
    monkeypatch.setitem(sys.modules, "api.db", db_mod)

    db_models_mod = ModuleType("api.db.db_models")
    db_models_mod.UserTenant = type(
        "UserTenant",
        (),
        {
            "tenant_id": _ExprField("tenant_id"),
            "user_id": _ExprField("user_id"),
        },
    )
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    user_service_mod = ModuleType("api.db.services.user_service")

    class _UserTenantService:
        @staticmethod
        def get_by_tenant_id(_tenant_id):
            return []

        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def save(**_kwargs):
            return True

        @staticmethod
        def filter_delete(_conditions):
            return True

        @staticmethod
        def get_tenants_by_user_id(_user_id):
            return []

        @staticmethod
        def filter_update(_conditions, _payload):
            return True

    class _UserService:
        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def get_by_id(_user_id):
            return False, None

    user_service_mod.UserTenantService = _UserTenantService
    user_service_mod.UserService = _UserService
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")
    api_utils_mod.get_json_result = lambda data=None, message="", code=0: {"code": code, "message": message, "data": data}
    api_utils_mod.get_data_error_result = lambda message="": {"code": 102, "message": message, "data": False}
    api_utils_mod.server_error_response = lambda exc: {"code": 100, "message": repr(exc), "data": False}
    api_utils_mod.validate_request = lambda *_args, **_kwargs: (lambda fn: fn)
    api_utils_mod.get_request_json = lambda: _AwaitableValue({})
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    web_utils_mod = ModuleType("api.utils.web_utils")
    web_utils_mod.send_invite_email = lambda **_kwargs: {"ok": True}
    monkeypatch.setitem(sys.modules, "api.utils.web_utils", web_utils_mod)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    constants_mod = ModuleType("common.constants")
    constants_mod.RetCode = SimpleNamespace(AUTHENTICATION_ERROR=401, SERVER_ERROR=500, DATA_ERROR=102)
    constants_mod.StatusEnum = SimpleNamespace(VALID=SimpleNamespace(value=1))
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    misc_utils_mod = ModuleType("common.misc_utils")
    misc_utils_mod.get_uuid = lambda: "uuid-1"
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    time_utils_mod = ModuleType("common.time_utils")
    time_utils_mod.delta_seconds = lambda _value: 0
    monkeypatch.setitem(sys.modules, "common.time_utils", time_utils_mod)

    settings_mod = ModuleType("common.settings")
    settings_mod.MAIL_FRONTEND_URL = "https://frontend.example/invite"
    monkeypatch.setitem(sys.modules, "common.settings", settings_mod)
    common_pkg.settings = settings_mod

    sys.modules.pop("test_tenant_app_unit_module", None)
    module_path = repo_root / "api" / "apps" / "tenant_app.py"
    spec = importlib.util.spec_from_file_location("test_tenant_app_unit_module", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, "test_tenant_app_unit_module", module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_user_list_auth_success_exception_matrix_unit(monkeypatch):
    module = _load_tenant_module(monkeypatch)

    module.current_user.id = "other-user"
    res = module.user_list("tenant-1")
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res
    assert res["message"] == "No authorization.", res

    module.current_user.id = "tenant-1"
    monkeypatch.setattr(
        module.UserTenantService,
        "get_by_tenant_id",
        lambda _tenant_id: [{"id": "u1", "update_date": "2024-01-01 00:00:00"}],
    )
    monkeypatch.setattr(module, "delta_seconds", lambda _value: 42)
    res = module.user_list("tenant-1")
    assert res["code"] == 0, res
    assert res["data"][0]["delta_seconds"] == 42, res

    monkeypatch.setattr(module.UserTenantService, "get_by_tenant_id", lambda _tenant_id: (_ for _ in ()).throw(RuntimeError("list boom")))
    res = module.user_list("tenant-1")
    assert res["code"] == 100, res
    assert "list boom" in res["message"], res


@pytest.mark.p2
def test_create_invite_role_and_email_failure_matrix_unit(monkeypatch):
    module = _load_tenant_module(monkeypatch)

    module.current_user.id = "other-user"
    _set_request_json(monkeypatch, module, {"email": "invitee@example.com"})
    res = _run(module.create("tenant-1"))
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res
    assert res["message"] == "No authorization.", res

    module.current_user.id = "tenant-1"
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [])
    res = _run(module.create("tenant-1"))
    assert res["message"] == "User not found.", res

    invitee = _Invitee()
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [invitee])
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(role=module.UserTenantRole.NORMAL)])
    res = _run(module.create("tenant-1"))
    assert "already in the team." in res["message"], res

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(role=module.UserTenantRole.OWNER)])
    res = _run(module.create("tenant-1"))
    assert "owner of the team." in res["message"], res

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(role="strange-role")])
    res = _run(module.create("tenant-1"))
    assert "role: strange-role is invalid." in res["message"], res

    saved = []
    scheduled = []
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module.UserTenantService, "save", lambda **kwargs: saved.append(kwargs) or True)
    monkeypatch.setattr(module.UserService, "get_by_id", lambda _user_id: (True, SimpleNamespace(nickname="Inviter Nick")))
    monkeypatch.setattr(module, "send_invite_email", lambda **kwargs: kwargs)
    monkeypatch.setattr(module.asyncio, "create_task", lambda payload: scheduled.append(payload) or SimpleNamespace())
    res = _run(module.create("tenant-1"))
    assert res["code"] == 0, res
    assert saved and saved[-1]["role"] == module.UserTenantRole.INVITE, saved
    assert scheduled and scheduled[-1]["inviter"] == "Inviter Nick", scheduled
    assert sorted(res["data"].keys()) == ["avatar", "email", "id", "nickname"], res

    monkeypatch.setattr(module.asyncio, "create_task", lambda _payload: (_ for _ in ()).throw(RuntimeError("send boom")))
    res = _run(module.create("tenant-1"))
    assert res["code"] == module.RetCode.SERVER_ERROR, res
    assert "Failed to send invite email." in res["message"], res


@pytest.mark.p2
def test_rm_and_tenant_list_matrix_unit(monkeypatch):
    module = _load_tenant_module(monkeypatch)

    module.current_user.id = "outsider"
    res = module.rm("tenant-1", "user-2")
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res
    assert res["message"] == "No authorization.", res

    module.current_user.id = "tenant-1"
    deleted = []
    monkeypatch.setattr(module.UserTenantService, "filter_delete", lambda conditions: deleted.append(conditions) or True)
    res = module.rm("tenant-1", "user-2")
    assert res["code"] == 0, res
    assert res["data"] is True, res
    assert deleted, "filter_delete should be called"

    monkeypatch.setattr(module.UserTenantService, "filter_delete", lambda _conditions: (_ for _ in ()).throw(RuntimeError("rm boom")))
    res = module.rm("tenant-1", "user-2")
    assert res["code"] == 100, res
    assert "rm boom" in res["message"], res

    monkeypatch.setattr(
        module.UserTenantService,
        "get_tenants_by_user_id",
        lambda _user_id: [{"id": "tenant-1", "update_date": "2024-01-01 00:00:00"}],
    )
    monkeypatch.setattr(module, "delta_seconds", lambda _value: 9)
    res = module.tenant_list()
    assert res["code"] == 0, res
    assert res["data"][0]["delta_seconds"] == 9, res

    monkeypatch.setattr(module.UserTenantService, "get_tenants_by_user_id", lambda _user_id: (_ for _ in ()).throw(RuntimeError("tenant boom")))
    res = module.tenant_list()
    assert res["code"] == 100, res
    assert "tenant boom" in res["message"], res


@pytest.mark.p2
def test_agree_success_and_exception_unit(monkeypatch):
    module = _load_tenant_module(monkeypatch)

    calls = []
    monkeypatch.setattr(module.UserTenantService, "filter_update", lambda conditions, payload: calls.append((conditions, payload)) or True)
    res = module.agree("tenant-1")
    assert res["code"] == 0, res
    assert res["data"] is True, res
    assert calls and calls[-1][1]["role"] == module.UserTenantRole.NORMAL

    monkeypatch.setattr(module.UserTenantService, "filter_update", lambda _conditions, _payload: (_ for _ in ()).throw(RuntimeError("agree boom")))
    res = module.agree("tenant-1")
    assert res["code"] == 100, res
    assert "agree boom" in res["message"], res

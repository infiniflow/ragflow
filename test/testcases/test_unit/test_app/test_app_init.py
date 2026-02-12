#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
from pathlib import Path
import re
import sys
import types

import pytest


def _load_app_init_module(monkeypatch):
    root = Path(__file__).resolve().parents[4]
    file_path = root / "api" / "apps" / "__init__.py"
    source = file_path.read_text(encoding="utf-8")
    source = re.sub(
        r"client_urls_prefix\s*=\s*\[.*?\]",
        "client_urls_prefix = []",
        source,
        flags=re.S,
    )

    settings_mod = types.ModuleType("common.settings")
    settings_mod.SECRET_KEY = "secret"
    settings_mod.decrypt_database_config = lambda name=None: None
    settings_mod.init_settings = lambda: None
    monkeypatch.setitem(sys.modules, "common.settings", settings_mod)

    db_models_mod = types.ModuleType("api.db.db_models")

    class _APIToken:
        @classmethod
        def query(cls, token=None):  # noqa: D401 - simple stub
            return []

    db_models_mod.APIToken = _APIToken
    db_models_mod.close_connection = lambda: None
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    services_mod = types.ModuleType("api.db.services")

    class _UserService:
        @classmethod
        def query(cls, **_kwargs):
            return []

    services_mod.UserService = _UserService
    monkeypatch.setitem(sys.modules, "api.db.services", services_mod)

    json_encode_mod = types.ModuleType("api.utils.json_encode")

    class _CustomJSONEncoder:
        pass

    json_encode_mod.CustomJSONEncoder = _CustomJSONEncoder
    monkeypatch.setitem(sys.modules, "api.utils.json_encode", json_encode_mod)

    commands_mod = types.ModuleType("api.utils.commands")
    commands_mod.register_commands = lambda _app: None
    monkeypatch.setitem(sys.modules, "api.utils.commands", commands_mod)

    api_utils_mod = types.ModuleType("api.utils.api_utils")

    def _get_json_result(code=0, message="success", data=None):
        payload = {"code": code, "message": message}
        if data is not None:
            payload["data"] = data
        return payload

    api_utils_mod.get_json_result = _get_json_result
    api_utils_mod.server_error_response = lambda _e: {"code": 500, "message": "server error"}
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    constants_mod = types.ModuleType("api.constants")
    constants_mod.API_VERSION = "v1"
    monkeypatch.setitem(sys.modules, "api.constants", constants_mod)

    api_pkg = types.ModuleType("api")
    api_pkg.__path__ = []
    sys.modules["api"] = api_pkg

    apps_pkg = types.ModuleType("api.apps")
    apps_pkg.__path__ = []
    sys.modules["api.apps"] = apps_pkg

    utils_pkg = types.ModuleType("api.utils")
    utils_pkg.__path__ = []
    sys.modules["api.utils"] = utils_pkg

    module_name = "api.apps_test_init"
    module = types.ModuleType(module_name)
    module.__package__ = "api.apps"
    module.__file__ = str(file_path)
    sys.modules[module_name] = module
    exec(compile(source, str(file_path), "exec"), module.__dict__)
    return module


@pytest.mark.p2
@pytest.mark.asyncio
async def test_load_user_fallback_and_login_logout(monkeypatch):
    mod = _load_app_init_module(monkeypatch)

    class _DummySerializer:
        def __init__(self, secret_key=None):
            self.secret_key = secret_key

        def loads(self, _token):
            raise Exception("bad token")

    class _DummyToken:
        tenant_id = "tenant-id"

    class _DummyUser:
        id = "user-id"
        email = "user@example.com"
        access_token = "token" * 8
        is_active = True

    monkeypatch.setattr(mod, "Serializer", _DummySerializer)
    monkeypatch.setattr(mod.APIToken, "query", classmethod(lambda cls, token=None: [_DummyToken()]))
    monkeypatch.setattr(mod.UserService, "query", classmethod(lambda cls, **_kwargs: [_DummyUser()]))

    headers = {"Authorization": "Bearer token", "Cookie": "remember_token=abc"}
    async with mod.app.test_request_context("/", headers=headers):
        user = mod._load_user()
        assert isinstance(user, _DummyUser)

        assert mod.login_user(user) is True
        assert mod.session.get("_user_id") == user.id
        mod.session["_remember_seconds"] = 10

        assert mod.logout_user() is True
        assert "_user_id" not in mod.session
        assert "_fresh" not in mod.session
        assert "_id" not in mod.session
        assert mod.session.get("_remember") == "clear"
        assert "_remember_seconds" not in mod.session


@pytest.mark.p2
@pytest.mark.asyncio
async def test_unauthorized_message_and_error_handlers(monkeypatch):
    mod = _load_app_init_module(monkeypatch)
    monkeypatch.setenv("RAGFLOW_API_TIMING", "1")

    assert mod._unauthorized_message(None) == mod.UNAUTHORIZED_MESSAGE

    async def _handler():
        return "ok"

    wrapped = mod.login_required(_handler)
    async with mod.app.test_request_context("/"):
        with pytest.raises(mod.QuartAuthUnauthorized):
            await wrapped()

    async with mod.app.test_request_context("/missing"):
        response, status = await mod.not_found(None)
        assert status == mod.RetCode.NOT_FOUND
        payload = await response.get_json()
        assert payload["code"] == mod.RetCode.NOT_FOUND
        assert payload["data"] is None

    async with mod.app.test_request_context("/"):
        response, status = await mod.unauthorized(mod.WerkzeugUnauthorized())
        assert status == mod.RetCode.UNAUTHORIZED
        assert response["code"] == mod.RetCode.UNAUTHORIZED

    calls = {"count": 0}

    def _close_connection():
        calls["count"] += 1

    monkeypatch.setattr(mod, "close_connection", _close_connection)
    mod._db_close(None)
    mod._db_close(Exception("boom"))
    assert calls["count"] == 2

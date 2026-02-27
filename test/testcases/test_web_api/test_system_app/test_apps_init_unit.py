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
import logging
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest
from werkzeug.exceptions import Unauthorized as WerkzeugUnauthorized


class _DummyAPIToken:
    @staticmethod
    def query(**_kwargs):
        return []


class _DummyUserService:
    @staticmethod
    def query(**_kwargs):
        return []


def _run(coro):
    return asyncio.run(coro)


def _load_apps_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    settings_mod = ModuleType("common.settings")
    settings_mod.SECRET_KEY = "test-secret-key"
    settings_mod.init_settings = lambda: None
    settings_mod.decrypt_database_config = lambda name=None: {}
    monkeypatch.setitem(sys.modules, "common.settings", settings_mod)
    common_pkg.settings = settings_mod

    db_models_mod = ModuleType("api.db.db_models")
    db_models_mod.APIToken = _DummyAPIToken
    db_models_mod.close_connection = lambda: None
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    services_mod = ModuleType("api.db.services")
    services_mod.UserService = _DummyUserService
    monkeypatch.setitem(sys.modules, "api.db.services", services_mod)

    commands_mod = ModuleType("api.utils.commands")
    commands_mod.register_commands = lambda _app: None
    monkeypatch.setitem(sys.modules, "api.utils.commands", commands_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")

    def _get_json_result(code=0, message="success", data=None):
        return {"code": code, "message": message, "data": data}

    def _server_error_response(error):
        return {"code": 100, "message": repr(error)}

    api_utils_mod.get_json_result = _get_json_result
    api_utils_mod.server_error_response = _server_error_response
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    module_name = "test_apps_init_unit_module"
    module_path = repo_root / "api" / "apps" / "__init__.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)

    monkeypatch.setattr(Path, "glob", lambda self, _pattern: [])
    spec.loader.exec_module(module)
    return module.app, module


@pytest.mark.p2
def test_module_init_and_unauthorized_message_variants(monkeypatch):
    _quart_app, apps_module = _load_apps_module(monkeypatch)

    assert apps_module.client_urls_prefix == []

    class _BrokenRepr:
        def __repr__(self):
            raise RuntimeError("repr explode")

    class _ExactUnauthorizedRepr:
        def __repr__(self):
            return apps_module.UNAUTHORIZED_MESSAGE

    class _Unauthorized401Repr:
        def __repr__(self):
            return "Unauthorized 401 from upstream"

    class _OtherRepr:
        def __repr__(self):
            return "Forbidden 403"

    assert apps_module._unauthorized_message(None) == apps_module.UNAUTHORIZED_MESSAGE
    assert apps_module._unauthorized_message(_BrokenRepr()) == apps_module.UNAUTHORIZED_MESSAGE
    assert apps_module._unauthorized_message(_ExactUnauthorizedRepr()) == apps_module.UNAUTHORIZED_MESSAGE
    assert apps_module._unauthorized_message(_Unauthorized401Repr()) == "Unauthorized 401 from upstream"
    assert apps_module._unauthorized_message(_OtherRepr()) == apps_module.UNAUTHORIZED_MESSAGE


@pytest.mark.p2
def test_load_user_token_edge_cases(monkeypatch):
    quart_app, apps_module = _load_apps_module(monkeypatch)

    user_with_empty_token = SimpleNamespace(email="empty@example.com", access_token="")

    async def _case():
        async with quart_app.test_request_context("/", headers={"Authorization": "token"}):
            monkeypatch.setattr(apps_module.Serializer, "loads", lambda _self, _auth: "")
            assert apps_module._load_user() is None

        async with quart_app.test_request_context("/", headers={"Authorization": "token"}):
            monkeypatch.setattr(apps_module.Serializer, "loads", lambda _self, _auth: "short-token")
            assert apps_module._load_user() is None

        async with quart_app.test_request_context("/", headers={"Authorization": "token"}):
            monkeypatch.setattr(apps_module.Serializer, "loads", lambda _self, _auth: "a" * 32)
            monkeypatch.setattr(apps_module.UserService, "query", lambda **_kwargs: [user_with_empty_token])
            assert apps_module._load_user() is None

    _run(_case())


@pytest.mark.p2
def test_load_user_api_token_fallback_and_fallback_exception(monkeypatch, caplog):
    quart_app, apps_module = _load_apps_module(monkeypatch)

    def _raise_decode(_self, _auth):
        raise RuntimeError("decode failed")

    monkeypatch.setattr(apps_module.Serializer, "loads", _raise_decode)

    fallback_user_empty_token = SimpleNamespace(email="fallback@example.com", access_token="")

    async def _case():
        monkeypatch.setattr(apps_module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
        monkeypatch.setattr(apps_module.UserService, "query", lambda **_kwargs: [fallback_user_empty_token])
        async with quart_app.test_request_context("/", headers={"Authorization": "Bearer api-token"}):
            assert apps_module._load_user() is None

        def _raise_api_token(**_kwargs):
            raise RuntimeError("api token fallback failed")

        monkeypatch.setattr(apps_module.APIToken, "query", _raise_api_token)
        async with quart_app.test_request_context("/", headers={"Authorization": "Bearer api-token"}):
            with caplog.at_level(logging.WARNING):
                assert apps_module._load_user() is None

    _run(_case())
    assert "api token fallback failed" in caplog.text


@pytest.mark.p2
def test_login_required_timing_and_login_user_inactive(monkeypatch, caplog):
    quart_app, apps_module = _load_apps_module(monkeypatch)

    monkeypatch.setenv("RAGFLOW_API_TIMING", "1")
    monkeypatch.setattr(apps_module, "current_user", SimpleNamespace(id="tenant-1"))

    @apps_module.login_required
    async def _timed_handler():
        return {"ok": True}

    async def _case():
        async with quart_app.test_request_context("/timed"):
            with caplog.at_level(logging.INFO):
                assert await _timed_handler() == {"ok": True}

            inactive_user = SimpleNamespace(id="user-1", is_active=False)
            assert apps_module.login_user(inactive_user) is False

    _run(_case())
    assert "api_timing login_required" in caplog.text


@pytest.mark.p2
def test_logout_user_not_found_and_unauthorized_handlers(monkeypatch):
    quart_app, apps_module = _load_apps_module(monkeypatch)

    async def _case():
        async with quart_app.test_request_context("/logout", headers={"Cookie": "remember_token=abc"}):
            from quart import session

            session["_user_id"] = "user-1"
            session["_fresh"] = True
            session["_id"] = "session-id"
            session["_remember_seconds"] = 5

            assert apps_module.logout_user() is True
            assert "_user_id" not in session
            assert "_fresh" not in session
            assert "_id" not in session
            assert session.get("_remember") == "clear"
            assert "_remember_seconds" not in session

        async with quart_app.test_request_context("/missing/path"):
            not_found_resp, status = await apps_module.not_found(RuntimeError("missing"))
            assert status == apps_module.RetCode.NOT_FOUND
            payload = await not_found_resp.get_json()
            assert payload["code"] == apps_module.RetCode.NOT_FOUND
            assert payload["error"] == "Not Found"
            assert "Not Found:" in payload["message"]

        async with quart_app.test_request_context("/protected"):
            @apps_module.login_required
            async def _protected():
                return {"ok": True}

            monkeypatch.setattr(apps_module, "current_user", None)
            with pytest.raises(apps_module.QuartAuthUnauthorized) as exc_info:
                await _protected()

            quart_payload, quart_status = await apps_module.unauthorized_quart_auth(exc_info.value)
            assert quart_status == apps_module.RetCode.UNAUTHORIZED
            assert quart_payload["code"] == apps_module.RetCode.UNAUTHORIZED

            werk_payload, werk_status = await apps_module.unauthorized_werkzeug(WerkzeugUnauthorized("Unauthorized 401"))
            assert werk_status == apps_module.RetCode.UNAUTHORIZED
            assert werk_payload["code"] == apps_module.RetCode.UNAUTHORIZED

    _run(_case())

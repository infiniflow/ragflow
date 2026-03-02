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
import base64
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


class _Args(dict):
    def get(self, key, default=None, type=None):
        value = super().get(key, default)
        if type is None:
            return value
        try:
            return type(value)
        except (TypeError, ValueError):
            return default


class _DummyResponse:
    def __init__(self, data):
        self.data = data
        self.headers = {}


class _DummyHTTPResponse:
    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return self._payload


class _DummyRedis:
    def __init__(self):
        self.store = {}

    def get(self, key):
        return self.store.get(key)

    def set(self, key, value, _ttl=None):
        self.store[key] = value

    def delete(self, key):
        self.store.pop(key, None)


class _DummyUser:
    def __init__(self, user_id, email, *, password="stored-password", is_active="1", nickname="nick"):
        self.id = user_id
        self.email = email
        self.password = password
        self.is_active = is_active
        self.nickname = nickname
        self.access_token = ""
        self.save_calls = 0

    def save(self):
        self.save_calls += 1

    def get_id(self):
        return self.id

    def to_json(self):
        return {"id": self.id, "email": self.email, "nickname": self.nickname}

    def to_dict(self):
        return {"id": self.id, "email": self.email}


class _Field:
    def __init__(self, name):
        self.name = name

    def __eq__(self, other):
        return (self.name, other)


def _run(coro):
    return asyncio.run(coro)


def _set_request_json(monkeypatch, module, payload):
    async def _request_json():
        return payload

    monkeypatch.setattr(module, "get_request_json", _request_json)


def _set_request_args(monkeypatch, module, args=None):
    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args(args or {})))


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


def _load_user_app(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    quart_mod = ModuleType("quart")
    quart_mod.session = {}
    quart_mod.request = SimpleNamespace(args=_Args({}))

    async def _make_response(data):
        return _DummyResponse(data)

    quart_mod.make_response = _make_response
    quart_mod.redirect = lambda url: {"redirect": url}
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = _DummyUser("current-user", "current@example.com")
    apps_mod.login_required = lambda fn: fn
    apps_mod.login_user = lambda _user: True
    apps_mod.logout_user = lambda: True
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)
    api_pkg.apps = apps_mod

    apps_auth_mod = ModuleType("api.apps.auth")
    apps_auth_mod.get_auth_client = lambda _config: SimpleNamespace(
        get_authorization_url=lambda state: f"https://oauth.example/{state}"
    )
    monkeypatch.setitem(sys.modules, "api.apps.auth", apps_auth_mod)

    db_mod = ModuleType("api.db")
    db_mod.FileType = SimpleNamespace(FOLDER=SimpleNamespace(value="folder"))
    db_mod.UserTenantRole = SimpleNamespace(OWNER="owner")
    monkeypatch.setitem(sys.modules, "api.db", db_mod)
    api_pkg.db = db_mod

    db_models_mod = ModuleType("api.db.db_models")

    class _DummyTenantLLMModel:
        tenant_id = _Field("tenant_id")

        @staticmethod
        def delete():
            class _DeleteQuery:
                def where(self, *_args, **_kwargs):
                    return self

                def execute(self):
                    return 1

            return _DeleteQuery()

    db_models_mod.TenantLLM = _DummyTenantLLMModel
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    file_service_mod = ModuleType("api.db.services.file_service")

    class _StubFileService:
        @staticmethod
        def insert(_data):
            return True

    file_service_mod.FileService = _StubFileService
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service_mod)

    llm_service_mod = ModuleType("api.db.services.llm_service")
    llm_service_mod.get_init_tenant_llm = lambda _user_id: []
    monkeypatch.setitem(sys.modules, "api.db.services.llm_service", llm_service_mod)

    tenant_llm_service_mod = ModuleType("api.db.services.tenant_llm_service")

    class _StubTenantLLMService:
        @staticmethod
        def insert_many(_payload):
            return True

    tenant_llm_service_mod.TenantLLMService = _StubTenantLLMService
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_llm_service", tenant_llm_service_mod)

    user_service_mod = ModuleType("api.db.services.user_service")

    class _StubTenantService:
        @staticmethod
        def insert(**_kwargs):
            return True

        @staticmethod
        def delete_by_id(_tenant_id):
            return True

        @staticmethod
        def get_by_id(_tenant_id):
            return True, SimpleNamespace(id=_tenant_id)

        @staticmethod
        def get_info_by(_user_id):
            return []

        @staticmethod
        def update_by_id(_tenant_id, _payload):
            return True

    class _StubUserService:
        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def query_user(_email, _password):
            return None

        @staticmethod
        def query_user_by_email(**_kwargs):
            return []

        @staticmethod
        def save(**_kwargs):
            return True

        @staticmethod
        def delete_by_id(_user_id):
            return True

        @staticmethod
        def update_by_id(_user_id, _payload):
            return True

        @staticmethod
        def update_user_password(_user_id, _new_password):
            return True

    class _StubUserTenantService:
        @staticmethod
        def insert(**_kwargs):
            return True

        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def delete_by_id(_user_tenant_id):
            return True

    user_service_mod.TenantService = _StubTenantService
    user_service_mod.UserService = _StubUserService
    user_service_mod.UserTenantService = _StubUserTenantService
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")

    async def _default_request_json():
        return {}

    def _get_json_result(code=0, message="success", data=None):
        return {"code": code, "message": message, "data": data}

    def _get_data_error_result(code=102, message="Sorry! Data missing!", data=None):
        return {"code": code, "message": message, "data": data}

    def _server_error_response(error):
        return {"code": 100, "message": repr(error)}

    def _validate_request(*_args, **_kwargs):
        def _decorator(func):
            return func

        return _decorator

    api_utils_mod.get_request_json = _default_request_json
    api_utils_mod.get_json_result = _get_json_result
    api_utils_mod.get_data_error_result = _get_data_error_result
    api_utils_mod.server_error_response = _server_error_response
    api_utils_mod.validate_request = _validate_request
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    crypt_mod = ModuleType("api.utils.crypt")
    crypt_mod.decrypt = lambda value: value
    monkeypatch.setitem(sys.modules, "api.utils.crypt", crypt_mod)

    web_utils_mod = ModuleType("api.utils.web_utils")
    web_utils_mod.send_email_html = lambda *_args, **_kwargs: _AwaitableValue(True)
    web_utils_mod.OTP_LENGTH = 6
    web_utils_mod.OTP_TTL_SECONDS = 600
    web_utils_mod.ATTEMPT_LIMIT = 5
    web_utils_mod.ATTEMPT_LOCK_SECONDS = 600
    web_utils_mod.RESEND_COOLDOWN_SECONDS = 60
    web_utils_mod.otp_keys = lambda email: (
        f"otp:{email}:code",
        f"otp:{email}:attempts",
        f"otp:{email}:last",
        f"otp:{email}:lock",
    )
    web_utils_mod.hash_code = lambda code, _salt: f"hash:{code}"
    web_utils_mod.captcha_key = lambda email: f"captcha:{email}"
    monkeypatch.setitem(sys.modules, "api.utils.web_utils", web_utils_mod)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    settings_mod = ModuleType("common.settings")
    settings_mod.OAUTH_CONFIG = {
        "github": {"display_name": "GitHub", "icon": "gh"},
        "feishu": {"display_name": "Feishu", "icon": "fs"},
    }
    settings_mod.GITHUB_OAUTH = {"url": "https://github.example/oauth", "client_id": "cid", "secret_key": "sk"}
    settings_mod.FEISHU_OAUTH = {
        "app_access_token_url": "https://feishu.example/app_token",
        "user_access_token_url": "https://feishu.example/user_token",
        "app_id": "app-id",
        "app_secret": "app-secret",
        "grant_type": "authorization_code",
    }
    settings_mod.CHAT_MDL = "chat-mdl"
    settings_mod.EMBEDDING_MDL = "embd-mdl"
    settings_mod.ASR_MDL = "asr-mdl"
    settings_mod.PARSERS = []
    settings_mod.IMAGE2TEXT_MDL = "img-mdl"
    settings_mod.RERANK_MDL = "rerank-mdl"
    settings_mod.REGISTER_ENABLED = True
    monkeypatch.setitem(sys.modules, "common.settings", settings_mod)
    common_pkg.settings = settings_mod

    constants_mod = ModuleType("common.constants")
    constants_mod.RetCode = SimpleNamespace(
        AUTHENTICATION_ERROR=401,
        SERVER_ERROR=500,
        FORBIDDEN=403,
        EXCEPTION_ERROR=100,
        OPERATING_ERROR=300,
        ARGUMENT_ERROR=101,
        DATA_ERROR=102,
        NOT_EFFECTIVE=103,
        SUCCESS=0,
    )
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    connection_utils_mod = ModuleType("common.connection_utils")

    async def _construct_response(data=None, auth=None, message=""):
        return {"code": 0, "message": message, "data": data, "auth": auth}

    connection_utils_mod.construct_response = _construct_response
    monkeypatch.setitem(sys.modules, "common.connection_utils", connection_utils_mod)

    time_utils_mod = ModuleType("common.time_utils")
    time_utils_mod.current_timestamp = lambda: 111
    time_utils_mod.datetime_format = lambda _dt: "2024-01-01 00:00:00"
    time_utils_mod.get_format_time = lambda: "2024-01-01 00:00:00"
    monkeypatch.setitem(sys.modules, "common.time_utils", time_utils_mod)

    misc_utils_mod = ModuleType("common.misc_utils")
    misc_utils_mod.download_img = lambda _url: "avatar"
    misc_utils_mod.get_uuid = lambda: "uuid-default"
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    http_client_mod = ModuleType("common.http_client")

    async def _async_request(_method, _url, **_kwargs):
        return _DummyHTTPResponse({})

    http_client_mod.async_request = _async_request
    monkeypatch.setitem(sys.modules, "common.http_client", http_client_mod)

    rag_pkg = ModuleType("rag")
    rag_pkg.__path__ = [str(repo_root / "rag")]
    monkeypatch.setitem(sys.modules, "rag", rag_pkg)

    rag_utils_pkg = ModuleType("rag.utils")
    rag_utils_pkg.__path__ = [str(repo_root / "rag" / "utils")]
    monkeypatch.setitem(sys.modules, "rag.utils", rag_utils_pkg)

    redis_mod = ModuleType("rag.utils.redis_conn")
    redis_mod.REDIS_CONN = _DummyRedis()
    monkeypatch.setitem(sys.modules, "rag.utils.redis_conn", redis_mod)

    module_name = "test_user_app_unit_module"
    module_path = repo_root / "api" / "apps" / "user_app.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_login_route_branch_matrix_unit(monkeypatch):
    module = _load_user_app(monkeypatch)

    _set_request_json(monkeypatch, module, {})
    res = _run(module.login())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert "Unauthorized" in res["message"]

    _set_request_json(monkeypatch, module, {"email": "unknown@example.com", "password": "enc"})
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [])
    res = _run(module.login())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert "not registered" in res["message"]

    _set_request_json(monkeypatch, module, {"email": "known@example.com", "password": "enc"})
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [SimpleNamespace(email="known@example.com")])

    def _raise_decrypt(_value):
        raise RuntimeError("decrypt explode")

    monkeypatch.setattr(module, "decrypt", _raise_decrypt)
    res = _run(module.login())
    assert res["code"] == module.RetCode.SERVER_ERROR
    assert "Fail to crypt password" in res["message"]

    user_inactive = _DummyUser("u-inactive", "known@example.com", is_active="0")
    monkeypatch.setattr(module, "decrypt", lambda value: value)
    monkeypatch.setattr(module.UserService, "query_user", lambda _email, _password: user_inactive)
    res = _run(module.login())
    assert res["code"] == module.RetCode.FORBIDDEN
    assert "disabled" in res["message"]

    monkeypatch.setattr(module.UserService, "query_user", lambda _email, _password: None)
    res = _run(module.login())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert "do not match" in res["message"]


@pytest.mark.p2
def test_login_channels_and_oauth_login_matrix_unit(monkeypatch):
    module = _load_user_app(monkeypatch)

    module.settings.OAUTH_CONFIG = {"github": {"display_name": "GitHub", "icon": "gh"}}
    res = _run(module.get_login_channels())
    assert res["code"] == 0
    assert res["data"][0]["channel"] == "github"

    class _BrokenOAuthConfig:
        @staticmethod
        def items():
            raise RuntimeError("broken oauth config")

    module.settings.OAUTH_CONFIG = _BrokenOAuthConfig()
    res = _run(module.get_login_channels())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "Load channels failure" in res["message"]

    module.settings.OAUTH_CONFIG = {"github": {"display_name": "GitHub", "icon": "gh"}}
    with pytest.raises(ValueError, match="Invalid channel name: missing"):
        _run(module.oauth_login("missing"))

    module.session.clear()
    monkeypatch.setattr(module, "get_uuid", lambda: "state-123")

    class _AuthClient:
        @staticmethod
        def get_authorization_url(state):
            return f"https://oauth.example/{state}"

    monkeypatch.setattr(module, "get_auth_client", lambda _config: _AuthClient())
    res = _run(module.oauth_login("github"))
    assert res["redirect"] == "https://oauth.example/state-123"
    assert module.session["oauth_state"] == "state-123"


@pytest.mark.p2
def test_oauth_callback_matrix_unit(monkeypatch):
    module = _load_user_app(monkeypatch)
    module.settings.OAUTH_CONFIG = {"github": {"display_name": "GitHub", "icon": "gh"}}

    class _SyncAuthClient:
        def __init__(self, token_info, user_info):
            self._token_info = token_info
            self._user_info = user_info

        def exchange_code_for_token(self, _code):
            return self._token_info

        def fetch_user_info(self, _token, id_token=None):
            _ = id_token
            return self._user_info

    class _AsyncAuthClient:
        def __init__(self, token_info, user_info):
            self._token_info = token_info
            self._user_info = user_info

        async def async_exchange_code_for_token(self, _code):
            return self._token_info

        async def async_fetch_user_info(self, _token, id_token=None):
            _ = id_token
            return self._user_info

    _set_request_args(monkeypatch, module, {"state": "x", "code": "c"})
    module.session.clear()
    res = _run(module.oauth_callback("missing"))
    assert "Invalid channel name: missing" in res["redirect"]

    sync_ok = _SyncAuthClient(
        token_info={"access_token": "token-sync", "id_token": "id-sync"},
        user_info=SimpleNamespace(email="sync@example.com", avatar_url="http://img", nickname="sync"),
    )
    monkeypatch.setattr(module, "get_auth_client", lambda _config: sync_ok)

    module.session.clear()
    module.session["oauth_state"] = "expected"
    _set_request_args(monkeypatch, module, {"state": "wrong", "code": "code"})
    res = _run(module.oauth_callback("github"))
    assert res["redirect"] == "/?error=invalid_state"

    module.session.clear()
    module.session["oauth_state"] = "ok-state"
    _set_request_args(monkeypatch, module, {"state": "ok-state"})
    res = _run(module.oauth_callback("github"))
    assert res["redirect"] == "/?error=missing_code"

    sync_missing_token = _SyncAuthClient(
        token_info={"id_token": "id-only"},
        user_info=SimpleNamespace(email="sync@example.com", avatar_url="http://img", nickname="sync"),
    )
    monkeypatch.setattr(module, "get_auth_client", lambda _config: sync_missing_token)
    module.session.clear()
    module.session["oauth_state"] = "token-state"
    _set_request_args(monkeypatch, module, {"state": "token-state", "code": "code"})
    res = _run(module.oauth_callback("github"))
    assert res["redirect"] == "/?error=token_failed"

    sync_missing_email = _SyncAuthClient(
        token_info={"access_token": "token-sync", "id_token": "id-sync"},
        user_info=SimpleNamespace(email=None, avatar_url="http://img", nickname="sync"),
    )
    monkeypatch.setattr(module, "get_auth_client", lambda _config: sync_missing_email)
    module.session.clear()
    module.session["oauth_state"] = "email-state"
    _set_request_args(monkeypatch, module, {"state": "email-state", "code": "code"})
    res = _run(module.oauth_callback("github"))
    assert res["redirect"] == "/?error=email_missing"

    async_new_user = _AsyncAuthClient(
        token_info={"access_token": "token-async", "id_token": "id-async"},
        user_info=SimpleNamespace(email="new@example.com", avatar_url="http://img", nickname="new-user"),
    )
    monkeypatch.setattr(module, "get_auth_client", lambda _config: async_new_user)
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [])

    def _raise_download(_url):
        raise RuntimeError("download explode")

    monkeypatch.setattr(module, "download_img", _raise_download)
    monkeypatch.setattr(module, "user_register", lambda _user_id, _user: None)
    rollback_calls = []
    monkeypatch.setattr(module, "rollback_user_registration", lambda user_id: rollback_calls.append(user_id))
    monkeypatch.setattr(module, "get_uuid", lambda: "new-user-id")
    module.session.clear()
    module.session["oauth_state"] = "new-user-state"
    _set_request_args(monkeypatch, module, {"state": "new-user-state", "code": "code"})
    res = _run(module.oauth_callback("github"))
    assert "Failed to register new@example.com" in res["redirect"]
    assert rollback_calls == ["new-user-id"]

    monkeypatch.setattr(module, "download_img", lambda _url: "avatar")
    monkeypatch.setattr(
        module,
        "user_register",
        lambda _user_id, _user: [_DummyUser("dup-1", "new@example.com"), _DummyUser("dup-2", "new@example.com")],
    )
    rollback_calls.clear()
    module.session.clear()
    module.session["oauth_state"] = "dup-user-state"
    _set_request_args(monkeypatch, module, {"state": "dup-user-state", "code": "code"})
    res = _run(module.oauth_callback("github"))
    assert "Same email: new@example.com exists!" in res["redirect"]
    assert rollback_calls == ["new-user-id"]

    new_user = _DummyUser("new-user", "new@example.com")
    login_calls = []
    monkeypatch.setattr(module, "login_user", lambda user: login_calls.append(user))
    monkeypatch.setattr(module, "user_register", lambda _user_id, _user: [new_user])
    module.session.clear()
    module.session["oauth_state"] = "create-user-state"
    _set_request_args(monkeypatch, module, {"state": "create-user-state", "code": "code"})
    res = _run(module.oauth_callback("github"))
    assert res["redirect"] == "/?auth=new-user"
    assert login_calls and login_calls[-1] is new_user

    async_existing_inactive = _AsyncAuthClient(
        token_info={"access_token": "token-existing", "id_token": "id-existing"},
        user_info=SimpleNamespace(email="existing@example.com", avatar_url="http://img", nickname="existing"),
    )
    monkeypatch.setattr(module, "get_auth_client", lambda _config: async_existing_inactive)
    inactive_user = _DummyUser("existing-user", "existing@example.com", is_active="0")
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [inactive_user])
    module.session.clear()
    module.session["oauth_state"] = "inactive-state"
    _set_request_args(monkeypatch, module, {"state": "inactive-state", "code": "code"})
    res = _run(module.oauth_callback("github"))
    assert res["redirect"] == "/?error=user_inactive"

    async_existing_ok = _AsyncAuthClient(
        token_info={"access_token": "token-existing", "id_token": "id-existing"},
        user_info=SimpleNamespace(email="existing@example.com", avatar_url="http://img", nickname="existing"),
    )
    monkeypatch.setattr(module, "get_auth_client", lambda _config: async_existing_ok)
    existing_user = _DummyUser("existing-user", "existing@example.com")
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [existing_user])
    login_calls.clear()
    monkeypatch.setattr(module, "login_user", lambda user: login_calls.append(user))
    monkeypatch.setattr(module, "get_uuid", lambda: "existing-token")
    module.session.clear()
    module.session["oauth_state"] = "existing-state"
    _set_request_args(monkeypatch, module, {"state": "existing-state", "code": "code"})
    res = _run(module.oauth_callback("github"))
    assert res["redirect"] == "/?auth=existing-user"
    assert existing_user.access_token == "existing-token"
    assert existing_user.save_calls == 1
    assert login_calls and login_calls[-1] is existing_user


@pytest.mark.p2
def test_oauth_external_matrix_unit(monkeypatch):
    module = _load_user_app(monkeypatch)
    module.settings.OAUTH_CONFIG = {"github": {"display_name": "GitHub", "icon": "gh"}}

    class _SyncAuthClient:
        def __init__(self, user_info=None, err=None):
            self._user_info = user_info
            self._err = err
            self.calls = []

        def fetch_user_info(self, token, id_token=None):
            self.calls.append((token, id_token))
            if self._err:
                raise self._err
            return self._user_info

    class _AsyncAuthClient:
        def __init__(self, user_info=None):
            self._user_info = user_info
            self.calls = []

        async def async_fetch_user_info(self, token, id_token=None):
            self.calls.append((token, id_token))
            return self._user_info

    _set_request_json(monkeypatch, module, {"access_token": "token", "id_token": "id"})
    res = _run(module.oauth_external("missing"))
    assert res["code"] == module.RetCode.ARGUMENT_ERROR
    assert "Invalid channel name: missing" in res["message"]

    _set_request_json(monkeypatch, module, {})
    res = _run(module.oauth_external("github"))
    assert res["code"] == module.RetCode.ARGUMENT_ERROR
    assert "Missing access_token." in res["message"]

    sync_err = _SyncAuthClient(err=RuntimeError("bad token"))
    monkeypatch.setattr(module, "get_auth_client", lambda _config: sync_err)
    _set_request_json(monkeypatch, module, {"access_token": "token-err", "id_token": "id-err"})
    res = _run(module.oauth_external("github"))
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert "Failed to validate OAuth/OIDC tokens" in res["message"]
    assert sync_err.calls == [("token-err", "id-err")]

    sync_missing_email = _SyncAuthClient(user_info=SimpleNamespace(email=None, avatar_url="http://img", nickname="sync"))
    monkeypatch.setattr(module, "get_auth_client", lambda _config: sync_missing_email)
    _set_request_json(monkeypatch, module, {"access_token": "token-email", "id_token": "id-email"})
    res = _run(module.oauth_external("github"))
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert "Email missing from OAuth/OIDC user info." in res["message"]
    assert sync_missing_email.calls == [("token-email", "id-email")]

    async_new_user = _AsyncAuthClient(user_info=SimpleNamespace(email="new@example.com", avatar_url="http://img", nickname="new-user"))
    monkeypatch.setattr(module, "get_auth_client", lambda _config: async_new_user)
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module, "download_img", lambda _url: "avatar")
    monkeypatch.setattr(module, "user_register", lambda _user_id, _user: None)
    rollback_calls = []
    monkeypatch.setattr(module, "rollback_user_registration", lambda user_id: rollback_calls.append(user_id))
    monkeypatch.setattr(module, "get_uuid", lambda: "new-user-id")
    _set_request_json(monkeypatch, module, {"access_token": "token-new", "id_token": "id-new"})
    res = _run(module.oauth_external("github"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "Failed to register new@example.com" in res["message"]
    assert rollback_calls == ["new-user-id"]
    assert async_new_user.calls == [("token-new", "id-new")]

    new_user = _DummyUser("new-user", "new@example.com")
    login_calls = []
    monkeypatch.setattr(module, "user_register", lambda _user_id, _user: [new_user])
    monkeypatch.setattr(module, "login_user", lambda user: login_calls.append(user))
    rollback_calls.clear()
    _set_request_json(monkeypatch, module, {"access_token": "token-new-ok", "id_token": "id-new-ok"})
    res = _run(module.oauth_external("github"))
    assert res["code"] == 0
    assert res["auth"] == "new-user"
    assert login_calls and login_calls[-1] is new_user
    assert rollback_calls == []

    async_existing_user = _AsyncAuthClient(
        user_info=SimpleNamespace(email="existing@example.com", avatar_url="http://img", nickname="existing")
    )
    monkeypatch.setattr(module, "get_auth_client", lambda _config: async_existing_user)
    inactive_user = _DummyUser("existing-user", "existing@example.com", is_active="0")
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [inactive_user])
    _set_request_json(monkeypatch, module, {"access_token": "token-existing", "id_token": "id-existing"})
    res = _run(module.oauth_external("github"))
    assert res["code"] == module.RetCode.FORBIDDEN
    assert "disabled" in res["message"]

    existing_user = _DummyUser("existing-user", "existing@example.com")
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [existing_user])
    login_calls.clear()
    monkeypatch.setattr(module, "login_user", lambda user: login_calls.append(user))
    monkeypatch.setattr(module, "get_uuid", lambda: "existing-token")
    _set_request_json(monkeypatch, module, {"access_token": "token-existing-ok", "id_token": "id-existing-ok"})
    res = _run(module.oauth_external("github"))
    assert res["code"] == 0
    assert res["auth"] == "existing-user"
    assert existing_user.access_token == "existing-token"
    assert existing_user.save_calls == 1
    assert login_calls and login_calls[-1] is existing_user
    assert async_existing_user.calls == [("token-existing", "id-existing"), ("token-existing-ok", "id-existing-ok")]


@pytest.mark.p2
def test_github_callback_matrix_unit(monkeypatch):
    module = _load_user_app(monkeypatch)

    _set_request_args(monkeypatch, module, {"code": "code"})
    module.session.clear()

    async def _request_error(_method, _url, **_kwargs):
        return _DummyHTTPResponse({"error": "bad", "error_description": "boom"})

    monkeypatch.setattr(module, "async_request", _request_error)
    res = _run(module.github_callback())
    assert res["redirect"] == "/?error=boom"

    async def _request_scope_missing(_method, _url, **_kwargs):
        return _DummyHTTPResponse({"scope": "repo", "access_token": "token-gh"})

    monkeypatch.setattr(module, "async_request", _request_scope_missing)
    res = _run(module.github_callback())
    assert res["redirect"] == "/?error=user:email not in scope"

    async def _request_token(_method, _url, **_kwargs):
        return _DummyHTTPResponse({"scope": "user:email,repo", "access_token": "token-gh"})

    monkeypatch.setattr(module, "async_request", _request_token)
    monkeypatch.setattr(
        module,
        "user_info_from_github",
        lambda _token: _AwaitableValue({"email": "gh@example.com", "avatar_url": "http://img", "login": "gh-user"}),
    )
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [])
    rollback_calls = []
    monkeypatch.setattr(module, "rollback_user_registration", lambda user_id: rollback_calls.append(user_id))
    monkeypatch.setattr(module, "get_uuid", lambda: "gh-user-id")

    def _raise_download(_url):
        raise RuntimeError("download explode")

    monkeypatch.setattr(module, "download_img", _raise_download)
    monkeypatch.setattr(module, "user_register", lambda _user_id, _user: None)
    res = _run(module.github_callback())
    assert "Fail to register gh@example.com." in res["redirect"]
    assert rollback_calls == ["gh-user-id"]

    monkeypatch.setattr(module, "download_img", lambda _url: "avatar")
    monkeypatch.setattr(
        module,
        "user_register",
        lambda _user_id, _user: [_DummyUser("dup-1", "gh@example.com"), _DummyUser("dup-2", "gh@example.com")],
    )
    rollback_calls.clear()
    res = _run(module.github_callback())
    assert "Same email: gh@example.com exists!" in res["redirect"]
    assert rollback_calls == ["gh-user-id"]

    new_user = _DummyUser("gh-new-user", "gh@example.com")
    login_calls = []
    monkeypatch.setattr(module, "login_user", lambda user: login_calls.append(user))
    monkeypatch.setattr(module, "user_register", lambda _user_id, _user: [new_user])
    res = _run(module.github_callback())
    assert res["redirect"] == "/?auth=gh-new-user"
    assert login_calls and login_calls[-1] is new_user

    inactive_user = _DummyUser("gh-existing", "gh@example.com", is_active="0")
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [inactive_user])
    res = _run(module.github_callback())
    assert res["redirect"] == "/?error=user_inactive"

    existing_user = _DummyUser("gh-existing", "gh@example.com")
    login_calls.clear()
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [existing_user])
    monkeypatch.setattr(module, "login_user", lambda user: login_calls.append(user))
    monkeypatch.setattr(module, "get_uuid", lambda: "gh-existing-token")
    res = _run(module.github_callback())
    assert res["redirect"] == "/?auth=gh-existing"
    assert existing_user.access_token == "gh-existing-token"
    assert existing_user.save_calls == 1
    assert login_calls and login_calls[-1] is existing_user


@pytest.mark.p2
def test_feishu_callback_matrix_unit(monkeypatch):
    module = _load_user_app(monkeypatch)

    _set_request_args(monkeypatch, module, {"code": "code"})
    module.session.clear()

    def _patch_async_queue(payloads):
        queue = list(payloads)

        async def _request(_method, _url, **_kwargs):
            return _DummyHTTPResponse(queue.pop(0))

        monkeypatch.setattr(module, "async_request", _request)

    _patch_async_queue([{"code": 1}])
    res = _run(module.feishu_callback())
    assert "/?error=" in res["redirect"]

    _patch_async_queue(
        [
            {"code": 0, "app_access_token": "app-token"},
            {"code": 1, "message": "bad token"},
        ]
    )
    res = _run(module.feishu_callback())
    assert res["redirect"] == "/?error=bad token"

    _patch_async_queue(
        [
            {"code": 0, "app_access_token": "app-token"},
            {"code": 0, "data": {"scope": "other", "access_token": "feishu-access"}},
        ]
    )
    res = _run(module.feishu_callback())
    assert "contact:user.email:readonly not in scope" in res["redirect"]

    _patch_async_queue(
        [
            {"code": 0, "app_access_token": "app-token"},
            {"code": 0, "data": {"scope": "contact:user.email:readonly", "access_token": "feishu-access"}},
        ]
    )
    monkeypatch.setattr(
        module,
        "user_info_from_feishu",
        lambda _token: _AwaitableValue({"email": "fs@example.com", "avatar_url": "http://img", "en_name": "fs-user"}),
    )
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [])
    rollback_calls = []
    monkeypatch.setattr(module, "rollback_user_registration", lambda user_id: rollback_calls.append(user_id))
    monkeypatch.setattr(module, "get_uuid", lambda: "fs-user-id")

    def _raise_download(_url):
        raise RuntimeError("download explode")

    monkeypatch.setattr(module, "download_img", _raise_download)
    monkeypatch.setattr(module, "user_register", lambda _user_id, _user: None)
    res = _run(module.feishu_callback())
    assert "Fail to register fs@example.com." in res["redirect"]
    assert rollback_calls == ["fs-user-id"]

    _patch_async_queue(
        [
            {"code": 0, "app_access_token": "app-token"},
            {"code": 0, "data": {"scope": "contact:user.email:readonly", "access_token": "feishu-access"}},
        ]
    )
    monkeypatch.setattr(module, "download_img", lambda _url: "avatar")
    monkeypatch.setattr(
        module,
        "user_register",
        lambda _user_id, _user: [_DummyUser("dup-1", "fs@example.com"), _DummyUser("dup-2", "fs@example.com")],
    )
    rollback_calls.clear()
    res = _run(module.feishu_callback())
    assert "Same email: fs@example.com exists!" in res["redirect"]
    assert rollback_calls == ["fs-user-id"]

    _patch_async_queue(
        [
            {"code": 0, "app_access_token": "app-token"},
            {"code": 0, "data": {"scope": "contact:user.email:readonly", "access_token": "feishu-access"}},
        ]
    )
    new_user = _DummyUser("fs-new-user", "fs@example.com")
    login_calls = []
    monkeypatch.setattr(module, "login_user", lambda user: login_calls.append(user))
    monkeypatch.setattr(module, "user_register", lambda _user_id, _user: [new_user])
    res = _run(module.feishu_callback())
    assert res["redirect"] == "/?auth=fs-new-user"
    assert login_calls and login_calls[-1] is new_user

    _patch_async_queue(
        [
            {"code": 0, "app_access_token": "app-token"},
            {"code": 0, "data": {"scope": "contact:user.email:readonly", "access_token": "feishu-access"}},
        ]
    )
    inactive_user = _DummyUser("fs-existing", "fs@example.com", is_active="0")
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [inactive_user])
    res = _run(module.feishu_callback())
    assert res["redirect"] == "/?error=user_inactive"

    _patch_async_queue(
        [
            {"code": 0, "app_access_token": "app-token"},
            {"code": 0, "data": {"scope": "contact:user.email:readonly", "access_token": "feishu-access"}},
        ]
    )
    existing_user = _DummyUser("fs-existing", "fs@example.com")
    login_calls.clear()
    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [existing_user])
    monkeypatch.setattr(module, "login_user", lambda user: login_calls.append(user))
    monkeypatch.setattr(module, "get_uuid", lambda: "fs-existing-token")
    res = _run(module.feishu_callback())
    assert res["redirect"] == "/?auth=fs-existing"
    assert existing_user.access_token == "fs-existing-token"
    assert existing_user.save_calls == 1
    assert login_calls and login_calls[-1] is existing_user


@pytest.mark.p2
def test_oauth_user_info_helpers_unit(monkeypatch):
    module = _load_user_app(monkeypatch)

    async def _request_feishu(_method, _url, **_kwargs):
        return _DummyHTTPResponse({"data": {"email": "", "en_name": "Feishu User"}})

    monkeypatch.setattr(module, "async_request", _request_feishu)
    feishu_user = _run(module.user_info_from_feishu("token-feishu"))
    assert feishu_user["email"] is None
    assert feishu_user["en_name"] == "Feishu User"

    async def _request_github(_method, url, **_kwargs):
        if "emails" in url:
            return _DummyHTTPResponse(
                [
                    {"email": "secondary@example.com", "primary": False},
                    {"email": "primary@example.com", "primary": True},
                ]
            )
        return _DummyHTTPResponse({"login": "gh-user"})

    monkeypatch.setattr(module, "async_request", _request_github)
    github_user = _run(module.user_info_from_github("token-github"))
    assert github_user["login"] == "gh-user"
    assert github_user["email"] == "primary@example.com"


@pytest.mark.p2
def test_logout_setting_profile_matrix_unit(monkeypatch):
    module = _load_user_app(monkeypatch)

    current_user = _DummyUser("current-user", "current@example.com", password="stored-password")
    monkeypatch.setattr(module, "current_user", current_user)
    monkeypatch.setattr(module.secrets, "token_hex", lambda _n: "abcdef")
    logout_calls = []
    monkeypatch.setattr(module, "logout_user", lambda: logout_calls.append(True))

    res = _run(module.log_out())
    assert res["code"] == 0
    assert current_user.access_token == "INVALID_abcdef"
    assert current_user.save_calls == 1
    assert logout_calls == [True]

    _set_request_json(monkeypatch, module, {"password": "old-password", "new_password": "new-password"})
    monkeypatch.setattr(module, "decrypt", lambda value: value)
    monkeypatch.setattr(module, "check_password_hash", lambda _hashed, _plain: False)
    res = _run(module.setting_user())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert "Password error" in res["message"]

    _set_request_json(
        monkeypatch,
        module,
        {
            "password": "old-password",
            "new_password": "new-password",
            "nickname": "neo",
            "email": "blocked@example.com",
            "status": "disabled",
            "theme": "dark",
        },
    )
    monkeypatch.setattr(module, "check_password_hash", lambda _hashed, _plain: True)
    monkeypatch.setattr(module, "decrypt", lambda value: f"dec:{value}")
    monkeypatch.setattr(module, "generate_password_hash", lambda value: f"hash:{value}")
    update_calls = {}

    def _update_by_id(user_id, payload):
        update_calls["user_id"] = user_id
        update_calls["payload"] = payload
        return True

    monkeypatch.setattr(module.UserService, "update_by_id", _update_by_id)
    res = _run(module.setting_user())
    assert res["code"] == 0
    assert res["data"] is True
    assert update_calls["user_id"] == "current-user"
    assert update_calls["payload"]["password"] == "hash:dec:new-password"
    assert update_calls["payload"]["nickname"] == "neo"
    assert update_calls["payload"]["theme"] == "dark"
    assert "email" not in update_calls["payload"]
    assert "status" not in update_calls["payload"]

    _set_request_json(monkeypatch, module, {"nickname": "neo"})

    def _raise_update(_user_id, _payload):
        raise RuntimeError("update explode")

    monkeypatch.setattr(module.UserService, "update_by_id", _raise_update)
    res = _run(module.setting_user())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "Update failure" in res["message"]

    res = _run(module.user_profile())
    assert res["code"] == 0
    assert res["data"] == current_user.to_dict()


@pytest.mark.p2
def test_registration_helpers_and_register_route_matrix_unit(monkeypatch):
    module = _load_user_app(monkeypatch)

    deleted = {"user": 0, "tenant": 0, "user_tenant": 0, "tenant_llm": 0}
    monkeypatch.setattr(module.UserService, "delete_by_id", lambda _user_id: deleted.__setitem__("user", deleted["user"] + 1))
    monkeypatch.setattr(module.TenantService, "delete_by_id", lambda _tenant_id: deleted.__setitem__("tenant", deleted["tenant"] + 1))
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(id="ut-1")])
    monkeypatch.setattr(module.UserTenantService, "delete_by_id", lambda _ut_id: deleted.__setitem__("user_tenant", deleted["user_tenant"] + 1))

    class _DeleteQuery:
        def where(self, *_args, **_kwargs):
            return self

        def execute(self):
            deleted["tenant_llm"] += 1
            return 1

    monkeypatch.setattr(module.TenantLLM, "delete", lambda: _DeleteQuery())
    module.rollback_user_registration("user-1")
    assert deleted == {"user": 1, "tenant": 1, "user_tenant": 1, "tenant_llm": 1}, deleted

    monkeypatch.setattr(module.UserService, "delete_by_id", lambda _user_id: (_ for _ in ()).throw(RuntimeError("u boom")))
    monkeypatch.setattr(module.TenantService, "delete_by_id", lambda _tenant_id: (_ for _ in ()).throw(RuntimeError("t boom")))
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: (_ for _ in ()).throw(RuntimeError("ut boom")))

    class _RaisingDeleteQuery:
        def where(self, *_args, **_kwargs):
            raise RuntimeError("llm boom")

    monkeypatch.setattr(module.TenantLLM, "delete", lambda: _RaisingDeleteQuery())
    module.rollback_user_registration("user-2")

    monkeypatch.setattr(module.UserService, "save", lambda **_kwargs: False)
    res = module.user_register(
        "new-user",
        {
            "nickname": "new",
            "email": "new@example.com",
            "password": "pw",
            "access_token": "tk",
            "login_channel": "password",
            "last_login_time": "2024-01-01 00:00:00",
            "is_superuser": False,
        },
    )
    assert res is None

    monkeypatch.setattr(module.settings, "REGISTER_ENABLED", False)
    _set_request_json(monkeypatch, module, {"nickname": "neo", "email": "neo@example.com", "password": "enc"})
    res = _run(module.user_add())
    assert res["code"] == module.RetCode.OPERATING_ERROR, res
    assert "disabled" in res["message"], res

    monkeypatch.setattr(module.settings, "REGISTER_ENABLED", True)
    _set_request_json(monkeypatch, module, {"nickname": "neo", "email": "bad-email", "password": "enc"})
    res = _run(module.user_add())
    assert res["code"] == module.RetCode.OPERATING_ERROR, res
    assert "Invalid email address" in res["message"], res

    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module, "decrypt", lambda value: value)
    monkeypatch.setattr(module, "get_uuid", lambda: "new-user-id")
    rollback_calls = []
    monkeypatch.setattr(module, "rollback_user_registration", lambda user_id: rollback_calls.append(user_id))

    _set_request_json(monkeypatch, module, {"nickname": "neo", "email": "neo@example.com", "password": "enc"})
    monkeypatch.setattr(module, "user_register", lambda _user_id, _payload: None)
    res = _run(module.user_add())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res
    assert "Fail to register neo@example.com." in res["message"], res
    assert rollback_calls == ["new-user-id"], rollback_calls

    rollback_calls.clear()
    monkeypatch.setattr(
        module,
        "user_register",
        lambda _user_id, _payload: [_DummyUser("dup-1", "neo@example.com"), _DummyUser("dup-2", "neo@example.com")],
    )
    _set_request_json(monkeypatch, module, {"nickname": "neo", "email": "neo@example.com", "password": "enc"})
    res = _run(module.user_add())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res
    assert "Same email: neo@example.com exists!" in res["message"], res
    assert rollback_calls == ["new-user-id"], rollback_calls


@pytest.mark.p2
def test_tenant_info_and_set_tenant_info_exception_matrix_unit(monkeypatch):
    module = _load_user_app(monkeypatch)

    monkeypatch.setattr(module.TenantService, "get_info_by", lambda _uid: [])
    res = _run(module.tenant_info())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Tenant not found" in res["message"], res

    def _raise_tenant_info(_uid):
        raise RuntimeError("tenant info boom")

    monkeypatch.setattr(module.TenantService, "get_info_by", _raise_tenant_info)
    res = _run(module.tenant_info())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res
    assert "tenant info boom" in res["message"], res

    _set_request_json(
        monkeypatch,
        module,
        {"tenant_id": "tenant-1", "llm_id": "l", "embd_id": "e", "asr_id": "a", "img2txt_id": "i"},
    )

    def _raise_update(_tenant_id, _payload):
        raise RuntimeError("tenant update boom")

    monkeypatch.setattr(module.TenantService, "update_by_id", _raise_update)
    res = _run(module.set_tenant_info())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res
    assert "tenant update boom" in res["message"], res


@pytest.mark.p2
def test_forget_captcha_and_send_otp_matrix_unit(monkeypatch):
    module = _load_user_app(monkeypatch)

    class _Headers(dict):
        def set(self, key, value):
            self[key] = value

    async def _make_response(data):
        return SimpleNamespace(data=data, headers=_Headers())

    monkeypatch.setattr(module, "make_response", _make_response)

    captcha_pkg = ModuleType("captcha")
    captcha_image_mod = ModuleType("captcha.image")

    class _ImageCaptcha:
        def __init__(self, **_kwargs):
            pass

        def generate(self, text):
            return SimpleNamespace(read=lambda: f"img:{text}".encode())

    captcha_image_mod.ImageCaptcha = _ImageCaptcha
    monkeypatch.setitem(sys.modules, "captcha", captcha_pkg)
    monkeypatch.setitem(sys.modules, "captcha.image", captcha_image_mod)

    _set_request_args(monkeypatch, module, {"email": ""})
    res = _run(module.forget_get_captcha())
    assert res["code"] == module.RetCode.ARGUMENT_ERROR, res

    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [])
    _set_request_args(monkeypatch, module, {"email": "nobody@example.com"})
    res = _run(module.forget_get_captcha())
    assert res["code"] == module.RetCode.DATA_ERROR, res

    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [_DummyUser("u1", "ok@example.com")])
    monkeypatch.setattr(module.secrets, "choice", lambda _allowed: "A")
    _set_request_args(monkeypatch, module, {"email": "ok@example.com"})
    res = _run(module.forget_get_captcha())
    assert res.data.startswith(b"img:"), res
    assert res.headers["Content-Type"] == "image/JPEG", res.headers
    assert module.REDIS_CONN.get(module.captcha_key("ok@example.com")), module.REDIS_CONN.store

    _set_request_json(monkeypatch, module, {"email": "", "captcha": ""})
    res = _run(module.forget_send_otp())
    assert res["code"] == module.RetCode.ARGUMENT_ERROR, res

    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [])
    _set_request_json(monkeypatch, module, {"email": "none@example.com", "captcha": "AAAA"})
    res = _run(module.forget_send_otp())
    assert res["code"] == module.RetCode.DATA_ERROR, res

    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [_DummyUser("u1", "ok@example.com")])
    _set_request_json(monkeypatch, module, {"email": "ok@example.com", "captcha": "AAAA"})
    module.REDIS_CONN.store.pop(module.captcha_key("ok@example.com"), None)
    res = _run(module.forget_send_otp())
    assert res["code"] == module.RetCode.NOT_EFFECTIVE, res

    module.REDIS_CONN.store[module.captcha_key("ok@example.com")] = "ABCD"
    _set_request_json(monkeypatch, module, {"email": "ok@example.com", "captcha": "ZZZZ"})
    res = _run(module.forget_send_otp())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    monkeypatch.setattr(module.time, "time", lambda: 1000)
    k_code, k_attempts, k_last, k_lock = module.otp_keys("ok@example.com")
    module.REDIS_CONN.store[module.captcha_key("ok@example.com")] = "ABCD"
    module.REDIS_CONN.store[k_last] = "990"
    _set_request_json(monkeypatch, module, {"email": "ok@example.com", "captcha": "ABCD"})
    res = _run(module.forget_send_otp())
    assert res["code"] == module.RetCode.NOT_EFFECTIVE, res
    assert "wait" in res["message"], res

    module.REDIS_CONN.store[module.captcha_key("ok@example.com")] = "ABCD"
    module.REDIS_CONN.store[k_last] = "bad-timestamp"
    monkeypatch.setattr(module.secrets, "choice", lambda _allowed: "B")
    monkeypatch.setattr(module.os, "urandom", lambda _n: b"\x00" * 16)
    monkeypatch.setattr(module, "hash_code", lambda code, _salt: f"HASH_{code}")

    async def _raise_send_email(*_args, **_kwargs):
        raise RuntimeError("send email boom")

    monkeypatch.setattr(module, "send_email_html", _raise_send_email)
    _set_request_json(monkeypatch, module, {"email": "ok@example.com", "captcha": "ABCD"})
    res = _run(module.forget_send_otp())
    assert res["code"] == module.RetCode.SERVER_ERROR, res
    assert "failed to send email" in res["message"], res

    async def _ok_send_email(*_args, **_kwargs):
        return True

    module.REDIS_CONN.store[module.captcha_key("ok@example.com")] = "ABCD"
    module.REDIS_CONN.store.pop(k_last, None)
    monkeypatch.setattr(module, "send_email_html", _ok_send_email)
    _set_request_json(monkeypatch, module, {"email": "ok@example.com", "captcha": "ABCD"})
    res = _run(module.forget_send_otp())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["data"] is True, res
    assert module.REDIS_CONN.get(k_code), module.REDIS_CONN.store
    assert module.REDIS_CONN.get(k_attempts) == 0, module.REDIS_CONN.store
    assert module.REDIS_CONN.get(k_lock) is None, module.REDIS_CONN.store


@pytest.mark.p2
def test_forget_verify_otp_matrix_unit(monkeypatch):
    module = _load_user_app(monkeypatch)
    email = "ok@example.com"
    k_code, k_attempts, k_last, k_lock = module.otp_keys(email)
    salt = b"\x01" * 16
    monkeypatch.setattr(module, "hash_code", lambda code, _salt: f"HASH_{code}")

    _set_request_json(monkeypatch, module, {})
    res = _run(module.forget_verify_otp())
    assert res["code"] == module.RetCode.ARGUMENT_ERROR, res

    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [])
    _set_request_json(monkeypatch, module, {"email": email, "otp": "ABCDEF"})
    res = _run(module.forget_verify_otp())
    assert res["code"] == module.RetCode.DATA_ERROR, res

    monkeypatch.setattr(module.UserService, "query", lambda **_kwargs: [_DummyUser("u1", email)])
    module.REDIS_CONN.store[k_lock] = "1"
    _set_request_json(monkeypatch, module, {"email": email, "otp": "ABCDEF"})
    res = _run(module.forget_verify_otp())
    assert res["code"] == module.RetCode.NOT_EFFECTIVE, res
    module.REDIS_CONN.store.pop(k_lock, None)

    module.REDIS_CONN.store.pop(k_code, None)
    _set_request_json(monkeypatch, module, {"email": email, "otp": "ABCDEF"})
    res = _run(module.forget_verify_otp())
    assert res["code"] == module.RetCode.NOT_EFFECTIVE, res

    module.REDIS_CONN.store[k_code] = "broken"
    _set_request_json(monkeypatch, module, {"email": email, "otp": "ABCDEF"})
    res = _run(module.forget_verify_otp())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res

    module.REDIS_CONN.store[k_code] = f"HASH_CORRECT:{salt.hex()}"
    module.REDIS_CONN.store[k_attempts] = "bad-int"
    _set_request_json(monkeypatch, module, {"email": email, "otp": "wrong"})
    res = _run(module.forget_verify_otp())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res
    assert module.REDIS_CONN.get(k_attempts) == 1, module.REDIS_CONN.store

    module.REDIS_CONN.store[k_code] = f"HASH_CORRECT:{salt.hex()}"
    module.REDIS_CONN.store[k_attempts] = str(module.ATTEMPT_LIMIT - 1)
    _set_request_json(monkeypatch, module, {"email": email, "otp": "wrong"})
    res = _run(module.forget_verify_otp())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res
    assert module.REDIS_CONN.get(k_lock) is not None, module.REDIS_CONN.store
    module.REDIS_CONN.store.pop(k_lock, None)

    module.REDIS_CONN.store[k_code] = f"HASH_ABCDEF:{salt.hex()}"
    module.REDIS_CONN.store[k_attempts] = "0"
    module.REDIS_CONN.store[k_last] = "1000"

    def _set_with_verified_fail(key, value, _ttl=None):
        if key == module._verified_key(email):
            raise RuntimeError("verified set boom")
        module.REDIS_CONN.store[key] = value

    monkeypatch.setattr(module.REDIS_CONN, "set", _set_with_verified_fail)
    _set_request_json(monkeypatch, module, {"email": email, "otp": "abcdef"})
    res = _run(module.forget_verify_otp())
    assert res["code"] == module.RetCode.SERVER_ERROR, res

    monkeypatch.setattr(module.REDIS_CONN, "set", lambda key, value, _ttl=None: module.REDIS_CONN.store.__setitem__(key, value))
    module.REDIS_CONN.store[k_code] = f"HASH_ABCDEF:{salt.hex()}"
    module.REDIS_CONN.store[k_attempts] = "0"
    module.REDIS_CONN.store[k_last] = "1000"
    _set_request_json(monkeypatch, module, {"email": email, "otp": "abcdef"})
    res = _run(module.forget_verify_otp())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert module.REDIS_CONN.get(k_code) is None, module.REDIS_CONN.store
    assert module.REDIS_CONN.get(k_attempts) is None, module.REDIS_CONN.store
    assert module.REDIS_CONN.get(k_last) is None, module.REDIS_CONN.store
    assert module.REDIS_CONN.get(k_lock) is None, module.REDIS_CONN.store
    assert module.REDIS_CONN.get(module._verified_key(email)) == "1", module.REDIS_CONN.store


@pytest.mark.p2
def test_forget_reset_password_matrix_unit(monkeypatch):
    module = _load_user_app(monkeypatch)
    email = "reset@example.com"
    v_key = module._verified_key(email)
    user = _DummyUser("u-reset", email, nickname="reset-user")
    pwd_a = base64.b64encode(b"new-password").decode()
    pwd_b = base64.b64encode(b"confirm-password").decode()
    pwd_same = base64.b64encode(b"same-password").decode()
    monkeypatch.setattr(module, "decrypt", lambda value: value)

    _set_request_json(monkeypatch, module, {"email": email, "new_password": pwd_same, "confirm_new_password": pwd_same})
    module.REDIS_CONN.store.pop(v_key, None)
    res = _run(module.forget_reset_password())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res

    module.REDIS_CONN.store[v_key] = "1"
    monkeypatch.setattr(module, "decrypt", lambda _value: "")
    _set_request_json(monkeypatch, module, {"email": email, "new_password": "", "confirm_new_password": ""})
    res = _run(module.forget_reset_password())
    assert res["code"] == module.RetCode.ARGUMENT_ERROR, res

    monkeypatch.setattr(module, "decrypt", lambda value: value)
    module.REDIS_CONN.store[v_key] = "1"
    _set_request_json(monkeypatch, module, {"email": email, "new_password": pwd_a, "confirm_new_password": pwd_b})
    res = _run(module.forget_reset_password())
    assert res["code"] == module.RetCode.ARGUMENT_ERROR, res
    assert "do not match" in res["message"], res

    module.REDIS_CONN.store[v_key] = "1"
    monkeypatch.setattr(module.UserService, "query_user_by_email", lambda **_kwargs: [])
    _set_request_json(monkeypatch, module, {"email": email, "new_password": pwd_same, "confirm_new_password": pwd_same})
    res = _run(module.forget_reset_password())
    assert res["code"] == module.RetCode.DATA_ERROR, res

    module.REDIS_CONN.store[v_key] = "1"
    monkeypatch.setattr(module.UserService, "query_user_by_email", lambda **_kwargs: [user])

    def _raise_update_password(_user_id, _new_pwd):
        raise RuntimeError("reset boom")

    monkeypatch.setattr(module.UserService, "update_user_password", _raise_update_password)
    _set_request_json(monkeypatch, module, {"email": email, "new_password": pwd_same, "confirm_new_password": pwd_same})
    res = _run(module.forget_reset_password())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res

    module.REDIS_CONN.store[v_key] = "1"
    monkeypatch.setattr(module.UserService, "update_user_password", lambda _user_id, _new_pwd: True)
    monkeypatch.setattr(module.REDIS_CONN, "delete", lambda _key: (_ for _ in ()).throw(RuntimeError("delete boom")))
    _set_request_json(monkeypatch, module, {"email": email, "new_password": pwd_same, "confirm_new_password": pwd_same})
    res = _run(module.forget_reset_password())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["auth"] == user.get_id(), res

    monkeypatch.setattr(module.REDIS_CONN, "delete", lambda key: module.REDIS_CONN.store.pop(key, None))
    module.REDIS_CONN.store[v_key] = "1"
    _set_request_json(monkeypatch, module, {"email": email, "new_password": pwd_same, "confirm_new_password": pwd_same})
    res = _run(module.forget_reset_password())
    assert res["code"] == module.RetCode.SUCCESS, res
    assert res["auth"] == user.get_id(), res
    assert module.REDIS_CONN.get(v_key) is None, module.REDIS_CONN.store

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

    def to_dict(self, flat=True):
        return dict(self)


class _FakeResponse:
    def __init__(self, body, status_code):
        self.body = body
        self.status_code = status_code
        self.headers = {}


class _FakeConnectorRecord:
    def __init__(self, payload):
        self._payload = payload

    def to_dict(self):
        return dict(self._payload)


class _FakeCredentials:
    def __init__(self, raw='{"refresh_token":"rt","access_token":"at"}'):
        self._raw = raw

    def to_json(self):
        return self._raw


class _FakeFlow:
    def __init__(self, client_config, scopes):
        self.client_config = client_config
        self.scopes = scopes
        self.redirect_uri = None
        self.credentials = _FakeCredentials()
        self.auth_kwargs = None
        self.token_code = None

    def authorization_url(self, **kwargs):
        self.auth_kwargs = dict(kwargs)
        return f"https://oauth.example/{kwargs['state']}", kwargs["state"]

    def fetch_token(self, code):
        self.token_code = code


class _FakeBoxToken:
    def __init__(self, access_token, refresh_token):
        self.access_token = access_token
        self.refresh_token = refresh_token


class _FakeBoxOAuth:
    def __init__(self, config):
        self.config = config
        self.exchange_code = None

    def get_authorize_url(self, options):
        return f"https://box.example/auth?state={options.state}&redirect={options.redirect_uri}"

    def get_tokens_authorization_code_grant(self, code):
        self.exchange_code = code

    def retrieve_token(self):
        return _FakeBoxToken("box-access", "box-refresh")


class _FakeRedis:
    def __init__(self):
        self.store = {}
        self.set_calls = []
        self.deleted = []

    def get(self, key):
        return self.store.get(key)

    def set_obj(self, key, obj, ttl):
        self.set_calls.append((key, obj, ttl))
        self.store[key] = json.dumps(obj)

    def delete(self, key):
        self.deleted.append(key)
        self.store.pop(key, None)


def _run(coro):
    return asyncio.run(coro)


def _set_request(module, *, args=None, json_body=None):
    module.request = SimpleNamespace(
        args=_Args(args or {}),
        json=_AwaitableValue({} if json_body is None else json_body),
    )


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


def _load_connector_app(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="tenant-1")
    apps_mod.login_required = lambda fn: fn
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    db_mod = ModuleType("api.db")
    db_mod.InputType = SimpleNamespace(POLL="POLL")
    monkeypatch.setitem(sys.modules, "api.db", db_mod)

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    connector_service_mod = ModuleType("api.db.services.connector_service")

    class _StubConnectorService:
        @staticmethod
        def update_by_id(*_args, **_kwargs):
            return True

        @staticmethod
        def save(**_kwargs):
            return True

        @staticmethod
        def get_by_id(_connector_id):
            return True, _FakeConnectorRecord({"id": _connector_id})

        @staticmethod
        def list(_tenant_id):
            return []

        @staticmethod
        def resume(*_args, **_kwargs):
            return True

        @staticmethod
        def rebuild(*_args, **_kwargs):
            return None

        @staticmethod
        def delete_by_id(*_args, **_kwargs):
            return True

    class _StubSyncLogsService:
        @staticmethod
        def list_sync_tasks(*_args, **_kwargs):
            return [], 0

    connector_service_mod.ConnectorService = _StubConnectorService
    connector_service_mod.SyncLogsService = _StubSyncLogsService
    monkeypatch.setitem(sys.modules, "api.db.services.connector_service", connector_service_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")

    async def _get_request_json():
        return {}

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
    api_utils_mod.validate_request = lambda *_args, **_kwargs: (lambda fn: fn)
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    constants_mod = ModuleType("common.constants")
    constants_mod.RetCode = SimpleNamespace(
        ARGUMENT_ERROR=101,
        SERVER_ERROR=500,
        RUNNING=102,
        PERMISSION_ERROR=403,
    )
    constants_mod.TaskStatus = SimpleNamespace(SCHEDULE="schedule", CANCEL="cancel")
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    config_mod = ModuleType("common.data_source.config")
    config_mod.GOOGLE_DRIVE_WEB_OAUTH_REDIRECT_URI = "https://example.com/drive"
    config_mod.GMAIL_WEB_OAUTH_REDIRECT_URI = "https://example.com/gmail"
    config_mod.BOX_WEB_OAUTH_REDIRECT_URI = "https://example.com/box"
    config_mod.DocumentSource = SimpleNamespace(GMAIL="gmail", GOOGLE_DRIVE="google-drive")
    monkeypatch.setitem(sys.modules, "common.data_source.config", config_mod)

    google_constants_mod = ModuleType("common.data_source.google_util.constant")
    google_constants_mod.WEB_OAUTH_POPUP_TEMPLATE = (
        "<html><head><title>{title}</title></head>"
        "<body><h1>{heading}</h1><p>{message}</p><script>{payload_json}</script><script>{auto_close}</script></body></html>"
    )
    google_constants_mod.GOOGLE_SCOPES = {
        config_mod.DocumentSource.GMAIL: ["scope-gmail"],
        config_mod.DocumentSource.GOOGLE_DRIVE: ["scope-drive"],
    }
    monkeypatch.setitem(sys.modules, "common.data_source.google_util.constant", google_constants_mod)

    misc_mod = ModuleType("common.misc_utils")
    misc_mod.get_uuid = lambda: "uuid-from-helper"
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_mod)

    rag_pkg = ModuleType("rag")
    rag_pkg.__path__ = [str(repo_root / "rag")]
    monkeypatch.setitem(sys.modules, "rag", rag_pkg)

    rag_utils_pkg = ModuleType("rag.utils")
    rag_utils_pkg.__path__ = [str(repo_root / "rag" / "utils")]
    monkeypatch.setitem(sys.modules, "rag.utils", rag_utils_pkg)

    redis_mod = ModuleType("rag.utils.redis_conn")
    redis_mod.REDIS_CONN = _FakeRedis()
    monkeypatch.setitem(sys.modules, "rag.utils.redis_conn", redis_mod)

    quart_mod = ModuleType("quart")
    quart_mod.request = SimpleNamespace(args=_Args(), json=_AwaitableValue({}))

    async def _make_response(body, status_code):
        return _FakeResponse(body, status_code)

    quart_mod.make_response = _make_response
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    google_pkg = ModuleType("google_auth_oauthlib")
    google_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "google_auth_oauthlib", google_pkg)

    google_flow_mod = ModuleType("google_auth_oauthlib.flow")

    class _StubFlow:
        @classmethod
        def from_client_config(cls, client_config, scopes):
            return _FakeFlow(client_config, scopes)

    google_flow_mod.Flow = _StubFlow
    monkeypatch.setitem(sys.modules, "google_auth_oauthlib.flow", google_flow_mod)

    box_mod = ModuleType("box_sdk_gen")

    class _OAuthConfig:
        def __init__(self, client_id, client_secret):
            self.client_id = client_id
            self.client_secret = client_secret

    class _GetAuthorizeUrlOptions:
        def __init__(self, redirect_uri, state):
            self.redirect_uri = redirect_uri
            self.state = state

    box_mod.BoxOAuth = _FakeBoxOAuth
    box_mod.OAuthConfig = _OAuthConfig
    box_mod.GetAuthorizeUrlOptions = _GetAuthorizeUrlOptions
    monkeypatch.setitem(sys.modules, "box_sdk_gen", box_mod)

    module_path = repo_root / "api" / "apps" / "connector_app.py"
    spec = importlib.util.spec_from_file_location("test_connector_routes_unit", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_connector_basic_routes_and_task_controls(monkeypatch):
    module = _load_connector_app(monkeypatch)

    async def _no_sleep(_secs):
        return None

    monkeypatch.setattr(module.asyncio, "sleep", _no_sleep)

    records = {"conn-1": _FakeConnectorRecord({"id": "conn-1", "source": "drive"})}
    update_calls = []
    save_calls = []
    resume_calls = []
    delete_calls = []

    monkeypatch.setattr(module.ConnectorService, "update_by_id", lambda cid, payload: update_calls.append((cid, payload)))

    def _save(**payload):
        save_calls.append(payload)
        records[payload["id"]] = _FakeConnectorRecord(payload)

    monkeypatch.setattr(module.ConnectorService, "save", _save)
    monkeypatch.setattr(module.ConnectorService, "get_by_id", lambda cid: (True, records[cid]))
    monkeypatch.setattr(module.ConnectorService, "list", lambda tenant_id: [{"id": "listed", "tenant": tenant_id}])
    monkeypatch.setattr(module.SyncLogsService, "list_sync_tasks", lambda cid, page, page_size: ([{"id": "log-1"}], 9))
    monkeypatch.setattr(module.ConnectorService, "resume", lambda cid, status: resume_calls.append((cid, status)))
    monkeypatch.setattr(module.ConnectorService, "delete_by_id", lambda cid: delete_calls.append(cid))
    monkeypatch.setattr(module, "get_uuid", lambda: "generated-id")

    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"id": "conn-1", "refresh_freq": 7, "config": {"x": 1}}),
    )
    res = _run(module.set_connector())
    assert update_calls == [("conn-1", {"refresh_freq": 7, "config": {"x": 1}})]
    assert res["data"]["id"] == "conn-1"

    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"name": "new", "source": "gmail", "config": {"y": 2}}),
    )
    res = _run(module.set_connector())
    assert save_calls[-1]["id"] == "generated-id"
    assert save_calls[-1]["tenant_id"] == "tenant-1"
    assert save_calls[-1]["input_type"] == module.InputType.POLL
    assert res["data"]["id"] == "generated-id"

    list_res = module.list_connector()
    assert list_res["data"] == [{"id": "listed", "tenant": "tenant-1"}]

    monkeypatch.setattr(module.ConnectorService, "get_by_id", lambda _cid: (False, None))
    missing_res = module.get_connector("missing")
    assert missing_res["message"] == "Can't find this Connector!"

    monkeypatch.setattr(module.ConnectorService, "get_by_id", lambda cid: (True, _FakeConnectorRecord({"id": cid})))
    found_res = module.get_connector("conn-2")
    assert found_res["data"]["id"] == "conn-2"

    _set_request(module, args={"page": "2", "page_size": "7"})
    logs_res = module.list_logs("conn-log")
    assert logs_res["data"] == {"total": 9, "logs": [{"id": "log-1"}]}

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"resume": True}))
    assert _run(module.resume("conn-r1"))["data"] is True

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"resume": False}))
    assert _run(module.resume("conn-r2"))["data"] is True
    assert ("conn-r1", module.TaskStatus.SCHEDULE) in resume_calls
    assert ("conn-r2", module.TaskStatus.CANCEL) in resume_calls

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"kb_id": "kb-1"}))
    monkeypatch.setattr(module.ConnectorService, "rebuild", lambda *_args: "rebuild-failed")
    failed_rebuild = _run(module.rebuild("conn-rb"))
    assert failed_rebuild["code"] == module.RetCode.SERVER_ERROR
    assert failed_rebuild["data"] is False

    monkeypatch.setattr(module.ConnectorService, "rebuild", lambda *_args: None)
    ok_rebuild = _run(module.rebuild("conn-rb"))
    assert ok_rebuild["data"] is True

    rm_res = module.rm_connector("conn-rm")
    assert rm_res["data"] is True
    assert ("conn-rm", module.TaskStatus.CANCEL) in resume_calls
    assert delete_calls == ["conn-rm"]


@pytest.mark.p2
def test_connector_oauth_helper_functions(monkeypatch):
    module = _load_connector_app(monkeypatch)

    assert module._web_state_cache_key("flow-a", "gmail") == "gmail_web_flow_state:flow-a"
    assert module._web_result_cache_key("flow-b", "google-drive") == "google-drive_web_flow_result:flow-b"

    creds_dict = {"web": {"client_id": "id"}}
    assert module._load_credentials(creds_dict) == creds_dict
    assert module._load_credentials(json.dumps(creds_dict)) == creds_dict

    with pytest.raises(ValueError, match="Invalid Google credentials JSON"):
        module._load_credentials("{not-json")

    assert module._get_web_client_config(creds_dict) == {"web": {"client_id": "id"}}
    with pytest.raises(ValueError, match="must include a 'web'"):
        module._get_web_client_config({"installed": {"client_id": "id"}})

    popup_ok = _run(module._render_web_oauth_popup("flow-1", True, "done", "gmail"))
    assert popup_ok.status_code == 200
    assert popup_ok.headers["Content-Type"] == "text/html; charset=utf-8"
    assert "Authorization complete" in popup_ok.body
    assert "ragflow-gmail-oauth" in popup_ok.body

    popup_error = _run(module._render_web_oauth_popup("flow-2", False, "<denied>", "google-drive"))
    assert popup_error.status_code == 200
    assert "Authorization failed" in popup_error.body
    assert "&lt;denied&gt;" in popup_error.body


@pytest.mark.p2
def test_start_google_web_oauth_matrix(monkeypatch):
    module = _load_connector_app(monkeypatch)

    redis = _FakeRedis()
    monkeypatch.setattr(module, "REDIS_CONN", redis)
    monkeypatch.setattr(module.time, "time", lambda: 1700000000)

    flow_calls = []

    def _from_client_config(client_config, scopes):
        flow = _FakeFlow(client_config, scopes)
        flow_calls.append(flow)
        return flow

    monkeypatch.setattr(module.Flow, "from_client_config", staticmethod(_from_client_config))

    _set_request(module, args={"type": "invalid"})
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"credentials": "{}"}))
    invalid_type = _run(module.start_google_web_oauth())
    assert invalid_type["code"] == module.RetCode.ARGUMENT_ERROR

    monkeypatch.setattr(module, "GMAIL_WEB_OAUTH_REDIRECT_URI", "")
    _set_request(module, args={"type": "gmail"})
    missing_redirect = _run(module.start_google_web_oauth())
    assert missing_redirect["code"] == module.RetCode.SERVER_ERROR

    monkeypatch.setattr(module, "GMAIL_WEB_OAUTH_REDIRECT_URI", "https://example.com/gmail")
    monkeypatch.setattr(module, "GOOGLE_DRIVE_WEB_OAUTH_REDIRECT_URI", "https://example.com/drive")

    _set_request(module, args={"type": "google-drive"})
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"credentials": "{invalid-json"}))
    invalid_credentials = _run(module.start_google_web_oauth())
    assert invalid_credentials["code"] == module.RetCode.ARGUMENT_ERROR

    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"credentials": json.dumps({"web": {"client_id": "id"}, "refresh_token": "rt"})}),
    )
    has_refresh_token = _run(module.start_google_web_oauth())
    assert has_refresh_token["code"] == module.RetCode.ARGUMENT_ERROR

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"credentials": json.dumps({"installed": {"x": 1}})}))
    missing_web = _run(module.start_google_web_oauth())
    assert missing_web["code"] == module.RetCode.ARGUMENT_ERROR

    ids = iter(["flow-gmail", "flow-drive"])
    monkeypatch.setattr(module.uuid, "uuid4", lambda: next(ids))

    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"credentials": json.dumps({"web": {"client_id": "id", "client_secret": "secret"}})}),
    )

    _set_request(module, args={"type": "gmail"})
    gmail_ok = _run(module.start_google_web_oauth())
    assert gmail_ok["code"] == 0
    assert gmail_ok["data"]["flow_id"] == "flow-gmail"
    assert gmail_ok["data"]["authorization_url"].endswith("flow-gmail")

    _set_request(module, args={})
    drive_ok = _run(module.start_google_web_oauth())
    assert drive_ok["code"] == 0
    assert drive_ok["data"]["flow_id"] == "flow-drive"
    assert drive_ok["data"]["authorization_url"].endswith("flow-drive")

    assert any(call.scopes == module.GOOGLE_SCOPES[module.DocumentSource.GMAIL] for call in flow_calls)
    assert any(call.scopes == module.GOOGLE_SCOPES[module.DocumentSource.GOOGLE_DRIVE] for call in flow_calls)
    assert "gmail_web_flow_state:flow-gmail" in redis.store
    assert "google-drive_web_flow_state:flow-drive" in redis.store


@pytest.mark.p2
def test_google_web_oauth_callbacks_matrix(monkeypatch):
    module = _load_connector_app(monkeypatch)

    flow_calls = []

    def _from_client_config(client_config, scopes):
        flow = _FakeFlow(client_config, scopes)
        flow_calls.append(flow)
        return flow

    monkeypatch.setattr(module.Flow, "from_client_config", staticmethod(_from_client_config))

    callback_specs = [
        (
            module.google_gmail_web_oauth_callback,
            "gmail",
            module.GMAIL_WEB_OAUTH_REDIRECT_URI,
            module.GOOGLE_SCOPES[module.DocumentSource.GMAIL],
        ),
        (
            module.google_drive_web_oauth_callback,
            "google-drive",
            module.GOOGLE_DRIVE_WEB_OAUTH_REDIRECT_URI,
            module.GOOGLE_SCOPES[module.DocumentSource.GOOGLE_DRIVE],
        ),
    ]

    for callback, source, expected_redirect, expected_scopes in callback_specs:
        redis = _FakeRedis()
        monkeypatch.setattr(module, "REDIS_CONN", redis)

        _set_request(module, args={})
        missing_state = _run(callback())
        assert "Missing OAuth state parameter." in missing_state.body

        _set_request(module, args={"state": "sid"})
        expired_state = _run(callback())
        assert "Authorization session expired" in expired_state.body

        redis.store[module._web_state_cache_key("sid", source)] = json.dumps({"user_id": "tenant-1"})
        _set_request(module, args={"state": "sid"})
        invalid_state = _run(callback())
        assert "Authorization session was invalid" in invalid_state.body
        assert module._web_state_cache_key("sid", source) in redis.deleted

        redis.store[module._web_state_cache_key("sid", source)] = json.dumps({
            "user_id": "tenant-1",
            "client_config": {"web": {"client_id": "cid"}},
        })
        _set_request(module, args={"state": "sid", "error": "denied", "error_description": "permission denied"})
        oauth_error = _run(callback())
        assert "permission denied" in oauth_error.body

        redis.store[module._web_state_cache_key("sid", source)] = json.dumps({
            "user_id": "tenant-1",
            "client_config": {"web": {"client_id": "cid"}},
        })
        _set_request(module, args={"state": "sid"})
        missing_code = _run(callback())
        assert "Missing authorization code" in missing_code.body

        redis.store[module._web_state_cache_key("sid", source)] = json.dumps({
            "user_id": "tenant-1",
            "client_config": {"web": {"client_id": "cid"}},
        })
        _set_request(module, args={"state": "sid", "code": "code-123"})
        success = _run(callback())
        assert "Authorization completed successfully." in success.body

        result_key = module._web_result_cache_key("sid", source)
        assert result_key in redis.store
        assert module._web_state_cache_key("sid", source) in redis.deleted

        assert flow_calls[-1].redirect_uri == expected_redirect
        assert flow_calls[-1].scopes == expected_scopes
        assert flow_calls[-1].token_code == "code-123"


@pytest.mark.p2
def test_poll_google_web_result_matrix(monkeypatch):
    module = _load_connector_app(monkeypatch)
    redis = _FakeRedis()
    monkeypatch.setattr(module, "REDIS_CONN", redis)

    _set_request(module, args={"type": "invalid"}, json_body={"flow_id": "flow-1"})
    invalid_type = _run(module.poll_google_web_result())
    assert invalid_type["code"] == module.RetCode.ARGUMENT_ERROR

    _set_request(module, args={"type": "gmail"}, json_body={"flow_id": "flow-1"})
    pending = _run(module.poll_google_web_result())
    assert pending["code"] == module.RetCode.RUNNING

    redis.store[module._web_result_cache_key("flow-1", "gmail")] = json.dumps(
        {"user_id": "another-user", "credentials": "token-x"}
    )
    _set_request(module, args={"type": "gmail"}, json_body={"flow_id": "flow-1"})
    permission_error = _run(module.poll_google_web_result())
    assert permission_error["code"] == module.RetCode.PERMISSION_ERROR

    redis.store[module._web_result_cache_key("flow-1", "gmail")] = json.dumps(
        {"user_id": "tenant-1", "credentials": "token-ok"}
    )
    _set_request(module, args={"type": "gmail"}, json_body={"flow_id": "flow-1"})
    success = _run(module.poll_google_web_result())
    assert success["code"] == 0
    assert success["data"] == {"credentials": "token-ok"}
    assert module._web_result_cache_key("flow-1", "gmail") in redis.deleted


@pytest.mark.p2
def test_box_oauth_start_callback_and_poll_matrix(monkeypatch):
    module = _load_connector_app(monkeypatch)
    redis = _FakeRedis()
    monkeypatch.setattr(module, "REDIS_CONN", redis)

    created_auth = []

    class _TrackingBoxOAuth(_FakeBoxOAuth):
        def __init__(self, config):
            super().__init__(config)
            created_auth.append(self)

    monkeypatch.setattr(module, "BoxOAuth", _TrackingBoxOAuth)
    monkeypatch.setattr(module.uuid, "uuid4", lambda: "flow-box")
    monkeypatch.setattr(module.time, "time", lambda: 1800000000)

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({}))
    missing_params = _run(module.start_box_web_oauth())
    assert missing_params["code"] == module.RetCode.ARGUMENT_ERROR

    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"client_id": "cid", "client_secret": "sec", "redirect_uri": "https://box.local/callback"}),
    )
    start_ok = _run(module.start_box_web_oauth())
    assert start_ok["code"] == 0
    assert start_ok["data"]["flow_id"] == "flow-box"
    assert "authorization_url" in start_ok["data"]
    assert module._web_state_cache_key("flow-box", "box") in redis.store

    _set_request(module, args={})
    missing_state = _run(module.box_web_oauth_callback())
    assert "Missing OAuth parameters." in missing_state.body

    _set_request(module, args={"state": "flow-box"})
    missing_code = _run(module.box_web_oauth_callback())
    assert "Missing authorization code from Box." in missing_code.body

    redis.store[module._web_state_cache_key("flow-null", "box")] = "null"
    _set_request(module, args={"state": "flow-null", "code": "abc"})
    invalid_session = _run(module.box_web_oauth_callback())
    assert invalid_session["code"] == module.RetCode.ARGUMENT_ERROR

    redis.store[module._web_state_cache_key("flow-box", "box")] = json.dumps(
        {"user_id": "tenant-1", "client_id": "cid", "client_secret": "sec"}
    )
    _set_request(module, args={"state": "flow-box", "code": "abc", "error": "access_denied", "error_description": "denied"})
    callback_error = _run(module.box_web_oauth_callback())
    assert "denied" in callback_error.body

    redis.store[module._web_state_cache_key("flow-ok", "box")] = json.dumps(
        {"user_id": "tenant-1", "client_id": "cid", "client_secret": "sec"}
    )
    _set_request(module, args={"state": "flow-ok", "code": "code-ok"})
    callback_success = _run(module.box_web_oauth_callback())
    assert "Authorization completed successfully." in callback_success.body
    assert created_auth[-1].exchange_code == "code-ok"
    assert module._web_result_cache_key("flow-ok", "box") in redis.store
    assert module._web_state_cache_key("flow-ok", "box") in redis.deleted

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"flow_id": "flow-ok"}))
    redis.store.pop(module._web_result_cache_key("flow-ok", "box"), None)
    pending = _run(module.poll_box_web_result())
    assert pending["code"] == module.RetCode.RUNNING

    redis.store[module._web_result_cache_key("flow-ok", "box")] = json.dumps({"user_id": "another-user"})
    permission_error = _run(module.poll_box_web_result())
    assert permission_error["code"] == module.RetCode.PERMISSION_ERROR

    redis.store[module._web_result_cache_key("flow-ok", "box")] = json.dumps(
        {"user_id": "tenant-1", "access_token": "at", "refresh_token": "rt"}
    )
    poll_success = _run(module.poll_box_web_result())
    assert poll_success["code"] == 0
    assert poll_success["data"]["credentials"]["access_token"] == "at"
    assert module._web_result_cache_key("flow-ok", "box") in redis.deleted

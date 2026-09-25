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

"""Unit tests for POST /connectors payload validation.

Missing mandatory fields and non-integer frequency values must produce a
data error (400-class envelope) instead of KeyError/ValueError 500s.

The connector handlers also reject encrypted credentials in a request and
return stored credentials decrypted.
"""

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


def _module_stub(name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    return mod


REQUEST_JSON: dict = {}
SAVED_CONNECTORS: list = []


def _validate_request(*required):
    def decorator(func):
        async def wrapper(*args, **kwargs):
            req = REQUEST_JSON
            missing = [k for k in required if k not in req]
            if missing:
                return {"code": 101, "message": f"missing required arguments: {', '.join(missing)}", "data": None}
            return await func(*args, **kwargs)

        return wrapper

    return decorator


def _load_connector_api(monkeypatch):
    repo_root = Path(__file__).resolve().parents[5]

    saved_connector = SimpleNamespace(to_dict=lambda: {"id": "conn-1", "name": "kb", "config": {}})
    connector_service = SimpleNamespace(
        save=lambda **kw: SAVED_CONNECTORS.append(kw),
        get_by_id=lambda _id: (True, saved_connector),
    )

    monkeypatch.setitem(sys.modules, "quart", _module_stub("quart", request=SimpleNamespace(args={}), make_response=None))
    monkeypatch.setitem(sys.modules, "google_auth_oauthlib", ModuleType("google_auth_oauthlib"))
    monkeypatch.setitem(sys.modules, "google_auth_oauthlib.flow", _module_stub("google_auth_oauthlib.flow", Flow=None))
    monkeypatch.setitem(sys.modules, "box_sdk_gen", _module_stub("box_sdk_gen", BoxOAuth=None, OAuthConfig=None, GetAuthorizeUrlOptions=None))
    monkeypatch.setitem(sys.modules, "api.db", _module_stub("api.db", InputType=SimpleNamespace(POLL="poll")))
    monkeypatch.setitem(
        sys.modules,
        "api.db.services.connector_service",
        _module_stub("api.db.services.connector_service", ConnectorAuthorizationError=Exception, ConnectorService=connector_service, SyncLogsService=SimpleNamespace()),
    )
    monkeypatch.setitem(
        sys.modules,
        "api.utils.api_utils",
        _module_stub(
            "api.utils.api_utils",
            get_data_error_result=lambda *a, **kw: {"code": 102, "message": kw.get("message", a[0] if a else ""), "data": None},
            get_json_result=lambda **kw: {"code": kw.get("code", 0), "message": kw.get("message", ""), "data": kw.get("data")},
            get_request_json=lambda: asyncio.sleep(0, result=REQUEST_JSON),
            validate_request=_validate_request,
        ),
    )
    monkeypatch.setitem(
        sys.modules,
        "api.utils.pagination_utils",
        _module_stub("api.utils.pagination_utils", DEFAULT_PAGE=1, DEFAULT_PAGE_SIZE=30, validate_rest_api_page=lambda p: int(p), validate_rest_api_page_size=lambda s: int(s)),
    )
    monkeypatch.setitem(
        sys.modules,
        "common.constants",
        _module_stub("common.constants", FileSource=SimpleNamespace(), RetCode=SimpleNamespace(DATA_ERROR=102, ARGUMENT_ERROR=101), TaskStatus=SimpleNamespace(UNSTART="0")),
    )
    monkeypatch.setitem(
        sys.modules,
        "common.data_source.config",
        _module_stub("common.data_source.config", GOOGLE_DRIVE_WEB_OAUTH_REDIRECT_URI="", GMAIL_WEB_OAUTH_REDIRECT_URI="", BOX_WEB_OAUTH_REDIRECT_URI="", DocumentSource=SimpleNamespace()),
    )
    monkeypatch.setitem(sys.modules, "common.data_source.google_util.constant", _module_stub("common.data_source.google_util.constant", WEB_OAUTH_POPUP_TEMPLATE="", GOOGLE_SCOPES=[]))
    monkeypatch.setitem(sys.modules, "common.misc_utils", _module_stub("common.misc_utils", get_uuid=lambda: "uuid-1"))
    monkeypatch.setitem(sys.modules, "rag.utils.redis_conn", _module_stub("rag.utils.redis_conn", REDIS_CONN=SimpleNamespace()))
    monkeypatch.setitem(
        sys.modules,
        "api.apps",
        _module_stub("api.apps", login_required=lambda f: f, current_user=SimpleNamespace(id="tenant-1")),
    )

    module_name = "test_connector_api_unit_module"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "connector_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    monkeypatch.setattr(module, "asyncio", SimpleNamespace(sleep=lambda *_a, **_k: asyncio.sleep(0)))
    return module


@pytest.mark.p2
def test_missing_mandatory_fields_return_data_error_not_keyerror(monkeypatch):
    module = _load_connector_api(monkeypatch)
    REQUEST_JSON.clear()
    REQUEST_JSON.update({"name": "kb"})  # missing source and config
    res = asyncio.run(module.create_connector())
    assert res["code"] == 101
    assert "source" in res["message"] and "config" in res["message"]


@pytest.mark.p2
@pytest.mark.parametrize("field", ["refresh_freq", "prune_freq", "timeout_secs"])
@pytest.mark.parametrize("value", ["abc", 1.5, True])
def test_non_integer_frequency_returns_data_error_not_500(monkeypatch, field, value):
    module = _load_connector_api(monkeypatch)
    SAVED_CONNECTORS.clear()
    REQUEST_JSON.clear()
    REQUEST_JSON.update({"name": "kb", "source": "local", "config": {}, field: value})
    res = asyncio.run(module.create_connector())
    assert res["code"] == 102
    assert "must be integers" in res["message"]
    assert SAVED_CONNECTORS == []


@pytest.mark.p2
def test_valid_payload_creates_connector_with_defaults(monkeypatch):
    module = _load_connector_api(monkeypatch)
    SAVED_CONNECTORS.clear()
    REQUEST_JSON.clear()
    REQUEST_JSON.update({"name": "kb", "source": "local", "config": {}})
    res = asyncio.run(module.create_connector())
    assert res["code"] == 0
    assert res["data"]["name"] == "kb"
    assert len(SAVED_CONNECTORS) == 1
    assert SAVED_CONNECTORS[0]["refresh_freq"] == 5
    assert SAVED_CONNECTORS[0]["prune_freq"] == 5
    assert SAVED_CONNECTORS[0]["timeout_secs"] == 60 * 29


# Bytes 0..31 and a value encrypted with them; the Go tests use the same pair.
_CONNECTOR_KEY = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
_ENCRYPTED = "enc:v1:ZGVmZ2hpamtsbW5vMzm/FhC2IvFVBzHK4EVIiS2pKzu5X9Feh/PZO57Rh3K0yyGkLTDmruUEeZB6LtRoLedehJBJUg=="
_PLAINTEXT = {"api_token": "tok-123", "user": "ada"}
# Bytes 1..32, not the key _ENCRYPTED was written with.
_WRONG_KEY = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
_ENCRYPTED_INPUT_MESSAGE = "config.credentials must be plaintext, not an encrypted value."


class _StoredConnector:
    tenant_id = "tenant-1"

    def __init__(self, config):
        self.config = config

    def to_dict(self):
        return {"id": "conn-1", "name": "kb", "config": self.config}


def _serve_stored_connector(monkeypatch, module, config):
    stored = _StoredConnector(config)
    calls = []

    def _update_by_id(_id, fields):
        calls.append(("update_by_id", fields))
        stored.config = fields.get("config", stored.config)

    monkeypatch.setattr(module.ConnectorService, "accessible", lambda *_args: True, raising=False)
    monkeypatch.setattr(module.ConnectorService, "get_by_id", lambda _id: (True, stored))
    monkeypatch.setattr(module.ConnectorService, "update_by_id", _update_by_id, raising=False)
    for name in ("cancel_tasks", "schedule_tasks", "delete_by_id"):
        monkeypatch.setattr(module.ConnectorService, name, lambda _id, name=name: calls.append((name, _id)), raising=False)
    monkeypatch.setattr(module, "TaskStatus", SimpleNamespace(UNSTART="0", CANCEL="cancel", SCHEDULE="schedule"))
    return calls


def _stub_connector_builder(monkeypatch, module):
    built = []

    def _build(source, config):
        built.append((source, config))
        return SimpleNamespace(validate_connector_settings=lambda: None)

    monkeypatch.setitem(sys.modules, "common.data_source", _module_stub("common.data_source", build_connector_for_source=_build))
    monkeypatch.setitem(
        sys.modules,
        "common.data_source.exceptions",
        _module_stub("common.data_source.exceptions", ConnectorMissingCredentialError=RuntimeError, ConnectorValidationError=RuntimeError),
    )
    monkeypatch.setattr(module, "FileSource", [])
    monkeypatch.setattr(module, "asyncio", SimpleNamespace(to_thread=asyncio.to_thread))
    return built


@pytest.mark.p2
@pytest.mark.parametrize("credentials", [_ENCRYPTED, "enc:v2:future"])
def test_create_rejects_encrypted_credentials(monkeypatch, credentials):
    module = _load_connector_api(monkeypatch)
    SAVED_CONNECTORS.clear()
    REQUEST_JSON.clear()
    REQUEST_JSON.update({"name": "kb", "source": "rss", "config": {"credentials": credentials}})
    res = asyncio.run(module.create_connector())
    assert res == {"code": 101, "message": _ENCRYPTED_INPUT_MESSAGE, "data": None}
    assert SAVED_CONNECTORS == []


@pytest.mark.p2
@pytest.mark.parametrize(
    "credentials, expected, writes",
    [
        (_ENCRYPTED, {"code": 101, "message": _ENCRYPTED_INPUT_MESSAGE, "data": None}, []),
        (
            {"api_token": "tok-123"},
            {"code": 0, "message": "", "data": {"id": "conn-1", "name": "kb", "config": {"credentials": {"api_token": "tok-123"}}}},
            [("update_by_id", {"id": "conn-1", "refresh_freq": 7, "config": {"credentials": {"api_token": "tok-123"}}})],
        ),
    ],
)
def test_update_rejects_encrypted_credentials(monkeypatch, credentials, expected, writes):
    module = _load_connector_api(monkeypatch)
    calls = _serve_stored_connector(monkeypatch, module, {})
    REQUEST_JSON.clear()
    REQUEST_JSON.update({"refresh_freq": 7, "config": {"credentials": credentials}})
    res = asyncio.run(module.update_connector("conn-1"))
    assert res == expected
    assert calls == writes


@pytest.mark.p2
@pytest.mark.parametrize(
    "credentials, expected, builds",
    [
        (_ENCRYPTED, {"code": 101, "message": _ENCRYPTED_INPUT_MESSAGE, "data": None}, 0),
        ({"api_token": "tok-123"}, {"code": 0, "message": "", "data": True}, 1),
    ],
)
def test_test_connector_rejects_encrypted_credentials(monkeypatch, credentials, expected, builds):
    module = _load_connector_api(monkeypatch)
    _serve_stored_connector(monkeypatch, module, {})
    built = _stub_connector_builder(monkeypatch, module)
    REQUEST_JSON.clear()
    REQUEST_JSON.update({"source": "rss", "config": {"credentials": credentials}})
    res = asyncio.run(module.test_connector("conn-1"))
    assert res == expected
    assert len(built) == builds


@pytest.mark.p2
@pytest.mark.parametrize("handler", ["get", "create", "update"])
def test_responses_return_decrypted_credentials(monkeypatch, handler):
    monkeypatch.setenv("RAGFLOW_CONNECTOR_KEY", _CONNECTOR_KEY)
    module = _load_connector_api(monkeypatch)
    stored_config = {"credentials": _ENCRYPTED, "sync_deleted_files": True}
    _serve_stored_connector(monkeypatch, module, stored_config)
    REQUEST_JSON.clear()
    if handler == "get":
        res = module.get_connector("conn-1")
    elif handler == "create":
        REQUEST_JSON.update({"name": "kb", "source": "rss", "config": {"credentials": _PLAINTEXT}})
        res = asyncio.run(module.create_connector())
    else:
        REQUEST_JSON.update({"refresh_freq": 7})
        res = asyncio.run(module.update_connector("conn-1"))
    assert res["code"] == 0
    assert res["data"]["config"] == {"credentials": _PLAINTEXT, "sync_deleted_files": True}
    assert stored_config["credentials"] == _ENCRYPTED


@pytest.mark.p2
def test_get_reports_a_safe_error_when_the_key_is_missing(monkeypatch):
    monkeypatch.delenv("RAGFLOW_CONNECTOR_KEY", raising=False)
    module = _load_connector_api(monkeypatch)
    _serve_stored_connector(monkeypatch, module, {"credentials": _ENCRYPTED})
    res = module.get_connector("conn-1")
    assert res == {"code": 102, "message": "connector credentials are encrypted but RAGFLOW_CONNECTOR_KEY is not set", "data": None}


@pytest.mark.p2
def test_delete_works_when_the_key_is_lost(monkeypatch):
    monkeypatch.delenv("RAGFLOW_CONNECTOR_KEY", raising=False)
    module = _load_connector_api(monkeypatch)
    calls = _serve_stored_connector(monkeypatch, module, {"credentials": _ENCRYPTED})
    res = module.rm_connector("conn-1")
    assert res == {"code": 0, "message": "", "data": True}
    assert calls == [("cancel_tasks", "conn-1"), ("delete_by_id", "conn-1")]


@pytest.mark.p2
@pytest.mark.parametrize("config", [{"credentials": {"api_token": "tok-456"}}, {"sync_deleted_files": True}])
def test_update_accepts_a_new_config_when_the_key_is_lost(monkeypatch, config):
    monkeypatch.delenv("RAGFLOW_CONNECTOR_KEY", raising=False)
    module = _load_connector_api(monkeypatch)
    calls = _serve_stored_connector(monkeypatch, module, {"credentials": _ENCRYPTED})
    REQUEST_JSON.clear()
    REQUEST_JSON.update({"config": config})
    res = asyncio.run(module.update_connector("conn-1"))
    assert res == {"code": 0, "message": "", "data": {"id": "conn-1", "name": "kb", "config": config}}
    assert calls == [("update_by_id", {"id": "conn-1", "config": config})]


@pytest.mark.p2
@pytest.mark.parametrize(
    "payload, writes",
    [
        ({"refresh_freq": 7}, [("update_by_id", {"id": "conn-1", "refresh_freq": 7})]),
        ({"refresh_freq": 7, "reschedule": True}, [("update_by_id", {"id": "conn-1", "refresh_freq": 7}), ("cancel_tasks", "conn-1"), ("schedule_tasks", "conn-1")]),
        ({"status": "CANCEL"}, [("cancel_tasks", "conn-1")]),
        ({"status": "SCHEDULE"}, [("schedule_tasks", "conn-1")]),
    ],
)
def test_update_without_a_key_works_for_plaintext_credentials(monkeypatch, payload, writes):
    monkeypatch.delenv("RAGFLOW_CONNECTOR_KEY", raising=False)
    module = _load_connector_api(monkeypatch)
    calls = _serve_stored_connector(monkeypatch, module, {"credentials": {"api_token": "tok-123"}})
    REQUEST_JSON.clear()
    REQUEST_JSON.update(payload)
    res = asyncio.run(module.update_connector("conn-1"))
    assert res == {"code": 0, "message": "", "data": {"id": "conn-1", "name": "kb", "config": {"credentials": {"api_token": "tok-123"}}}}
    assert calls == writes


@pytest.mark.p2
@pytest.mark.parametrize("payload", [{"refresh_freq": 7}, {"refresh_freq": 7, "reschedule": True}, {"status": "CANCEL"}, {"status": "SCHEDULE"}, {"config": None}])
@pytest.mark.parametrize(
    "key, message",
    [
        ("", "connector credentials are encrypted but RAGFLOW_CONNECTOR_KEY is not set"),
        (_WRONG_KEY, "cannot decrypt connector credentials: wrong RAGFLOW_CONNECTOR_KEY or corrupted value"),
    ],
    ids=["unset", "wrong"],
)
def test_update_without_new_credentials_writes_nothing_when_the_key_is_lost(monkeypatch, payload, key, message):
    monkeypatch.setenv("RAGFLOW_CONNECTOR_KEY", key)
    module = _load_connector_api(monkeypatch)
    calls = _serve_stored_connector(monkeypatch, module, {"credentials": _ENCRYPTED})
    REQUEST_JSON.clear()
    REQUEST_JSON.update(payload)
    res = asyncio.run(module.update_connector("conn-1"))
    assert res == {"code": 102, "message": message, "data": None}
    assert calls == []

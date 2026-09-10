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

    saved_connector = SimpleNamespace(to_dict=lambda: {"id": "conn-1", "name": "kb"})
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
def test_non_integer_frequency_returns_data_error_not_500(monkeypatch, field):
    module = _load_connector_api(monkeypatch)
    SAVED_CONNECTORS.clear()
    REQUEST_JSON.clear()
    REQUEST_JSON.update({"name": "kb", "source": "local", "config": {}, field: "abc"})
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

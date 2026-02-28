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
import hashlib
import hmac
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
        if value is None or type is None:
            return value
        try:
            return type(value)
        except (TypeError, ValueError):
            return default


class _DummyRequest:
    def __init__(
        self,
        *,
        path="/api/v1/webhook/agent-1",
        method="POST",
        headers=None,
        content_length=0,
        remote_addr="127.0.0.1",
        args=None,
        json_body=None,
        raw_body=b"",
        form=None,
        files=None,
        authorization=None,
    ):
        self.path = path
        self.method = method
        self.headers = headers or {}
        self.content_length = content_length
        self.remote_addr = remote_addr
        self.args = args or {}
        self.authorization = authorization
        self.form = _AwaitableValue(form or {})
        self.files = _AwaitableValue(files or {})
        self._json_body = json_body
        self._raw_body = raw_body

    async def get_json(self):
        return self._json_body

    async def get_data(self):
        return self._raw_body


class _CanvasRecord:
    def __init__(self, *, canvas_category, dsl, user_id="tenant-1"):
        self.canvas_category = canvas_category
        self.dsl = dsl
        self.user_id = user_id

    def to_dict(self):
        return {"user_id": self.user_id, "dsl": self.dsl}


class _StubCanvas:
    def __init__(self, dsl, user_id, agent_id, canvas_id=None):
        self.dsl = dsl
        self.user_id = user_id
        self.agent_id = agent_id
        self.canvas_id = canvas_id

    async def run(self, **_kwargs):
        if False:
            yield {}

    async def get_files_async(self, desc):
        return {"files": desc}

    def __str__(self):
        return "{}"


class _StubRedisConn:
    def __init__(self):
        self.bucket_result = [1]
        self.bucket_exc = None
        self.REDIS = object()

    def lua_token_bucket(self, **_kwargs):
        if self.bucket_exc is not None:
            raise self.bucket_exc
        return self.bucket_result

    def get(self, _key):
        return None

    def set_obj(self, _key, _obj, _ttl):
        return None


def _run(coro):
    return asyncio.run(coro)


def _default_webhook_params(
    *,
    security=None,
    methods=None,
    content_types="application/json",
    schema=None,
    execution_mode="Immediately",
    response=None,
):
    return {
        "mode": "Webhook",
        "methods": methods if methods is not None else ["POST"],
        "security": security if security is not None else {},
        "content_types": content_types,
        "schema": schema
        if schema is not None
        else {
            "query": {"properties": {}, "required": []},
            "headers": {"properties": {}, "required": []},
            "body": {"properties": {}, "required": []},
        },
        "execution_mode": execution_mode,
        "response": response if response is not None else {},
    }


def _make_webhook_cvs(module, *, params=None, dsl=None, canvas_category=None):
    if dsl is None:
        if params is None:
            params = _default_webhook_params()
        dsl = {
            "components": {
                "begin": {
                    "obj": {"component_name": "Begin", "params": params},
                    "downstream": [],
                    "upstream": [],
                }
            }
        }
    if canvas_category is None:
        canvas_category = module.CanvasCategory.Agent
    return _CanvasRecord(canvas_category=canvas_category, dsl=dsl)


def _patch_background_task(monkeypatch, module):
    def _fake_create_task(coro):
        coro.close()
        return None

    monkeypatch.setattr(module.asyncio, "create_task", _fake_create_task)


def _load_agents_app(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    agent_pkg = ModuleType("agent")
    agent_pkg.__path__ = []
    canvas_mod = ModuleType("agent.canvas")
    canvas_mod.Canvas = _StubCanvas
    agent_pkg.canvas = canvas_mod
    monkeypatch.setitem(sys.modules, "agent", agent_pkg)
    monkeypatch.setitem(sys.modules, "agent.canvas", canvas_mod)

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    canvas_service_mod = ModuleType("api.db.services.canvas_service")

    class _StubUserCanvasService:
        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def get_list(*_args, **_kwargs):
            return []

        @staticmethod
        def save(**_kwargs):
            return True

        @staticmethod
        def update_by_id(*_args, **_kwargs):
            return True

        @staticmethod
        def delete_by_id(*_args, **_kwargs):
            return True

        @staticmethod
        def get_by_id(_id):
            return False, None

    canvas_service_mod.UserCanvasService = _StubUserCanvasService
    monkeypatch.setitem(sys.modules, "api.db.services.canvas_service", canvas_service_mod)
    services_pkg.canvas_service = canvas_service_mod

    file_service_mod = ModuleType("api.db.services.file_service")

    class _StubFileService:
        @staticmethod
        def upload_info(*_args, **_kwargs):
            return {"id": "uploaded"}

    file_service_mod.FileService = _StubFileService
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service_mod)
    services_pkg.file_service = file_service_mod

    canvas_version_mod = ModuleType("api.db.services.user_canvas_version")

    class _StubUserCanvasVersionService:
        @staticmethod
        def insert(**_kwargs):
            return True

        @staticmethod
        def delete_all_versions(*_args, **_kwargs):
            return True

        @staticmethod
        def save_or_replace_latest(*_args, **_kwargs):
            return True

        @staticmethod
        def build_version_title(*_args, **_kwargs):
            return "stub_version_title"

    canvas_version_mod.UserCanvasVersionService = _StubUserCanvasVersionService
    monkeypatch.setitem(sys.modules, "api.db.services.user_canvas_version", canvas_version_mod)
    services_pkg.user_canvas_version = canvas_version_mod

    tenant_llm_service_mod = ModuleType("api.db.services.tenant_llm_service")

    class _StubLLMFactoriesService:
        @staticmethod
        def get_api_key(*_args, **_kwargs):
            return None

    tenant_llm_service_mod.LLMFactoriesService = _StubLLMFactoriesService
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_llm_service", tenant_llm_service_mod)
    services_pkg.tenant_llm_service = tenant_llm_service_mod

    user_service_mod = ModuleType("api.db.services.user_service")

    class _StubUserService:
        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def get_by_id(_id):
            return False, None

    user_service_mod.UserService = _StubUserService
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)
    services_pkg.user_service = user_service_mod
    services_pkg.UserService = _StubUserService

    # Stub api.apps package to prevent api/apps/__init__.py from executing
    # (it triggers heavy imports like quart, settings, DB connections).
    api_apps_pkg = ModuleType("api.apps")
    api_apps_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.apps", api_apps_pkg)

    api_apps_services_pkg = ModuleType("api.apps.services")
    api_apps_services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.apps.services", api_apps_services_pkg)
    api_apps_pkg.services = api_apps_services_pkg

    canvas_replica_mod = ModuleType("api.apps.services.canvas_replica_service")

    class _StubCanvasReplicaService:
        @classmethod
        def normalize_dsl(cls, dsl):
            import json
            if isinstance(dsl, str):
                return json.loads(dsl)
            return dsl

        @classmethod
        def bootstrap(cls, *_args, **_kwargs):
            return {}

        @classmethod
        def load_for_run(cls, *_args, **_kwargs):
            return None

        @classmethod
        def commit_after_run(cls, *_args, **_kwargs):
            return True

        @classmethod
        def replace_for_set(cls, *_args, **_kwargs):
            return True

        @classmethod
        def create_if_absent(cls, *_args, **_kwargs):
            return {}

    canvas_replica_mod.CanvasReplicaService = _StubCanvasReplicaService
    monkeypatch.setitem(sys.modules, "api.apps.services.canvas_replica_service", canvas_replica_mod)
    api_apps_services_pkg.canvas_replica_service = canvas_replica_mod

    redis_obj = _StubRedisConn()
    redis_mod = ModuleType("rag.utils.redis_conn")
    redis_mod.REDIS_CONN = redis_obj
    monkeypatch.setitem(sys.modules, "rag.utils.redis_conn", redis_mod)

    module_path = repo_root / "api" / "apps" / "sdk" / "agents.py"
    spec = importlib.util.spec_from_file_location("test_agents_webhook_unit", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


def _assert_bad_request(res, expected_substring):
    assert isinstance(res, tuple), res
    payload, code = res
    assert code == 400, res
    assert payload["code"] == 400, payload
    assert expected_substring in payload["message"], payload


@pytest.mark.p2
def test_agents_crud_unit_branches(monkeypatch):
    module = _load_agents_app(monkeypatch)

    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(args={"id": "missing", "title": "missing", "desc": "false", "page": "1", "page_size": "10"}),
    )
    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [])
    res = module.list_agents.__wrapped__("tenant-1")
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "doesn't exist" in res["message"]

    captured = {}

    def fake_get_list(_tenant_id, _page, _page_size, _orderby, desc, *_rest):
        captured["desc"] = desc
        return [{"id": "agent-1"}]

    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [{"id": "agent-1"}])
    monkeypatch.setattr(module.UserCanvasService, "get_list", fake_get_list)
    monkeypatch.setattr(module, "request", SimpleNamespace(args={"desc": "true"}))
    res = module.list_agents.__wrapped__("tenant-1")
    assert res["code"] == module.RetCode.SUCCESS
    assert captured["desc"] is True

    async def req_no_dsl():
        return {"title": "agent-a"}

    monkeypatch.setattr(module, "get_request_json", req_no_dsl)
    res = _run(module.create_agent.__wrapped__("tenant-1"))
    assert res["code"] == module.RetCode.ARGUMENT_ERROR
    assert "No DSL data in request" in res["message"]

    async def req_no_title():
        return {"dsl": {"components": {}}}

    monkeypatch.setattr(module, "get_request_json", req_no_title)
    res = _run(module.create_agent.__wrapped__("tenant-1"))
    assert res["code"] == module.RetCode.ARGUMENT_ERROR
    assert "No title in request" in res["message"]

    async def req_dup():
        return {"dsl": {"components": {}}, "title": "agent-dup"}

    monkeypatch.setattr(module, "get_request_json", req_dup)
    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [object()])
    res = _run(module.create_agent.__wrapped__("tenant-1"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "already exists" in res["message"]

    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module, "get_uuid", lambda: "agent-created")
    monkeypatch.setattr(module.UserCanvasService, "save", lambda **_kwargs: False)
    res = _run(module.create_agent.__wrapped__("tenant-1"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "Fail to create agent" in res["message"]

    async def req_update():
        return {"dsl": {"nodes": []}, "title": "  webhook-agent  ", "unused": None}

    monkeypatch.setattr(module, "get_request_json", req_update)
    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: False)
    res = _run(module.update_agent.__wrapped__("tenant-1", "agent-1"))
    assert res["code"] == module.RetCode.OPERATING_ERROR

    calls = {"update": 0, "save_or_replace_latest": 0}
    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: True)
    monkeypatch.setattr(
        module.UserCanvasService,
        "update_by_id",
        lambda *_args, **_kwargs: calls.__setitem__("update", calls["update"] + 1),
    )
    monkeypatch.setattr(
        module.UserCanvasVersionService,
        "save_or_replace_latest",
        lambda *_args, **_kwargs: calls.__setitem__("save_or_replace_latest", calls["save_or_replace_latest"] + 1),
    )
    res = _run(module.update_agent.__wrapped__("tenant-1", "agent-1"))
    assert res["code"] == module.RetCode.SUCCESS
    assert calls == {"update": 1, "save_or_replace_latest": 1}

    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: False)
    res = module.delete_agent.__wrapped__("tenant-1", "agent-1")
    assert res["code"] == module.RetCode.OPERATING_ERROR


@pytest.mark.p2
def test_webhook_prechecks(monkeypatch):
    module = _load_agents_app(monkeypatch)
    monkeypatch.setattr(module, "request", _DummyRequest(headers={"Content-Type": "application/json"}, json_body={}))

    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (False, None))
    _assert_bad_request(_run(module.webhook("agent-1")), "Canvas not found")

    cvs = _make_webhook_cvs(module, canvas_category=module.CanvasCategory.DataFlow)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Dataflow can not be triggered")

    cvs = _make_webhook_cvs(module, dsl="invalid-dsl")
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Invalid DSL format")

    cvs = _make_webhook_cvs(
        module,
        dsl={"components": {"begin": {"obj": {"component_name": "Begin", "params": {"mode": "Chat"}}}}},
    )
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Webhook not configured")

    params = _default_webhook_params(methods=["GET"])
    cvs = _make_webhook_cvs(module, params=params)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "not allowed")


@pytest.mark.p2
def test_webhook_security_dispatch(monkeypatch):
    module = _load_agents_app(monkeypatch)
    _patch_background_task(monkeypatch, module)

    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(headers={"Content-Type": "application/json"}, json_body={}, args={"a": "b"}),
    )

    for security in ({}, {"auth_type": "none"}):
        cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=security))
        monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id, _cvs=cvs: (True, _cvs))
        res = _run(module.webhook("agent-1"))
        assert hasattr(res, "status_code"), res
        assert res.status_code == 200

    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security={"auth_type": "unsupported"}))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Unsupported auth_type")


@pytest.mark.p2
def test_webhook_max_body_size(monkeypatch):
    module = _load_agents_app(monkeypatch)
    _patch_background_task(monkeypatch, module)

    base_request = _DummyRequest(headers={"Content-Type": "application/json"}, json_body={})
    monkeypatch.setattr(module, "request", base_request)

    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security={"auth_type": "none"}))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    res = _run(module.webhook("agent-1"))
    assert hasattr(res, "status_code")
    assert res.status_code == 200

    security = {"auth_type": "none", "max_body_size": "123"}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=security))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Invalid max_body_size format")

    security = {"auth_type": "none", "max_body_size": "11mb"}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=security))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "exceeds maximum allowed size")

    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(headers={"Content-Type": "application/json"}, json_body={}, content_length=2048),
    )
    security = {"auth_type": "none", "max_body_size": "1kb"}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=security))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Request body too large")


@pytest.mark.p2
def test_webhook_ip_whitelist(monkeypatch):
    module = _load_agents_app(monkeypatch)
    _patch_background_task(monkeypatch, module)

    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(headers={"Content-Type": "application/json"}, json_body={}, remote_addr="127.0.0.1"),
    )

    for whitelist in ([], ["127.0.0.0/24"], ["127.0.0.1"]):
        security = {"auth_type": "none", "ip_whitelist": whitelist}
        cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=security))
        monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id, _cvs=cvs: (True, _cvs))
        res = _run(module.webhook("agent-1"))
        assert hasattr(res, "status_code"), res
        assert res.status_code == 200

    security = {"auth_type": "none", "ip_whitelist": ["10.0.0.1"]}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=security))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "is not allowed")


@pytest.mark.p2
def test_webhook_rate_limit(monkeypatch):
    module = _load_agents_app(monkeypatch)
    _patch_background_task(monkeypatch, module)

    monkeypatch.setattr(module, "request", _DummyRequest(headers={"Content-Type": "application/json"}, json_body={}))

    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security={"auth_type": "none"}))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    res = _run(module.webhook("agent-1"))
    assert hasattr(res, "status_code")
    assert res.status_code == 200

    bad_limit = {"auth_type": "none", "rate_limit": {"limit": 0, "per": "minute"}}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=bad_limit))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "rate_limit.limit must be > 0")

    bad_per = {"auth_type": "none", "rate_limit": {"limit": 1, "per": "week"}}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=bad_per))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Invalid rate_limit.per")

    module.REDIS_CONN.bucket_result = [0]
    module.REDIS_CONN.bucket_exc = None
    denied = {"auth_type": "none", "rate_limit": {"limit": 1, "per": "minute"}}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=denied))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Too many requests")

    module.REDIS_CONN.bucket_result = [1]
    module.REDIS_CONN.bucket_exc = RuntimeError("redis failure")
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=denied))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Rate limit error")


@pytest.mark.p2
def test_webhook_token_basic_jwt_auth(monkeypatch):
    module = _load_agents_app(monkeypatch)
    _patch_background_task(monkeypatch, module)

    monkeypatch.setattr(module, "request", _DummyRequest(headers={"Content-Type": "application/json"}, json_body={}))

    token_security = {"auth_type": "token", "token": {"token_header": "X-TOKEN", "token_value": "ok"}}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=token_security))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Invalid token authentication")

    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(
            headers={"Content-Type": "application/json"},
            json_body={},
            authorization=SimpleNamespace(username="u", password="bad"),
        ),
    )
    basic_security = {"auth_type": "basic", "basic_auth": {"username": "u", "password": "p"}}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=basic_security))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Invalid Basic Auth credentials")

    monkeypatch.setattr(module, "request", _DummyRequest(headers={"Content-Type": "application/json"}, json_body={}))
    jwt_missing_secret = {"auth_type": "jwt", "jwt": {}}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=jwt_missing_secret))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "JWT secret not configured")

    jwt_base = {"auth_type": "jwt", "jwt": {"secret": "secret"}}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=jwt_base))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Missing Bearer token")

    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(headers={"Content-Type": "application/json", "Authorization": "Bearer   "}, json_body={}),
    )
    _assert_bad_request(_run(module.webhook("agent-1")), "Empty Bearer token")

    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(headers={"Content-Type": "application/json", "Authorization": "Bearer token"}, json_body={}),
    )
    monkeypatch.setattr(module.jwt, "decode", lambda *_args, **_kwargs: (_ for _ in ()).throw(Exception("decode boom")))
    _assert_bad_request(_run(module.webhook("agent-1")), "Invalid JWT")

    monkeypatch.setattr(module.jwt, "decode", lambda *_args, **_kwargs: {"exp": 1})
    jwt_reserved = {"auth_type": "jwt", "jwt": {"secret": "secret", "required_claims": ["exp"]}}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=jwt_reserved))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Reserved JWT claim cannot be required")

    monkeypatch.setattr(module.jwt, "decode", lambda *_args, **_kwargs: {})
    jwt_missing_claim = {"auth_type": "jwt", "jwt": {"secret": "secret", "required_claims": ["role"]}}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=jwt_missing_claim))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    _assert_bad_request(_run(module.webhook("agent-1")), "Missing JWT claim")

    captured = {}

    def fake_decode(token, options, **kwargs):
        captured["token"] = token
        captured["options"] = options
        captured["kwargs"] = kwargs
        return {"role": "admin"}

    monkeypatch.setattr(module.jwt, "decode", fake_decode)
    jwt_success = {
        "auth_type": "jwt",
        "jwt": {
            "secret": "secret",
            "audience": "aud",
            "issuer": "iss",
            "required_claims": "role",
        },
    }
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=jwt_success))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    res = _run(module.webhook("agent-1"))
    assert hasattr(res, "status_code")
    assert res.status_code == 200
    assert captured["kwargs"]["audience"] == "aud"
    assert captured["kwargs"]["issuer"] == "iss"
    assert captured["options"]["verify_aud"] is True
    assert captured["options"]["verify_iss"] is True

    monkeypatch.setattr(module.jwt, "decode", lambda *_args, **_kwargs: {})
    jwt_success_invalid_type = {"auth_type": "jwt", "jwt": {"secret": "secret", "required_claims": 123}}
    cvs = _make_webhook_cvs(module, params=_default_webhook_params(security=jwt_success_invalid_type))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    res = _run(module.webhook("agent-1"))
    assert hasattr(res, "status_code")
    assert res.status_code == 200


@pytest.mark.p2
def test_webhook_parse_request_branches(monkeypatch):
    module = _load_agents_app(monkeypatch)
    _patch_background_task(monkeypatch, module)

    security = {"auth_type": "none"}
    params = _default_webhook_params(security=security, content_types="application/json")
    cvs = _make_webhook_cvs(module, params=params)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))

    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(headers={"Content-Type": "text/plain"}, raw_body=b'{"x":1}', json_body={}),
    )
    with pytest.raises(ValueError, match="Invalid Content-Type"):
        _run(module.webhook("agent-1"))

    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(headers={"Content-Type": "application/json"}, json_body={"x": 1}, args={"q": "1"}),
    )
    res = _run(module.webhook("agent-1"))
    assert hasattr(res, "status_code")
    assert res.status_code == 200

    params = _default_webhook_params(security=security, content_types="multipart/form-data")
    cvs = _make_webhook_cvs(module, params=params)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    files = {f"file{i}": object() for i in range(11)}
    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(
            headers={"Content-Type": "multipart/form-data"},
            form={"key": "value"},
            files=files,
            json_body={},
        ),
    )
    res = _run(module.webhook("agent-1"))
    assert hasattr(res, "status_code")
    assert res.status_code == 200

    uploaded = {"count": 0}
    monkeypatch.setattr(
        module.FileService,
        "upload_info",
        lambda *_args, **_kwargs: uploaded.__setitem__("count", uploaded["count"] + 1) or {"id": "uploaded"},
    )
    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(
            headers={"Content-Type": "multipart/form-data"},
            form={"k": "v"},
            files={"file1": object()},
            json_body={},
        ),
    )
    res = _run(module.webhook("agent-1"))
    assert hasattr(res, "status_code")
    assert res.status_code == 200
    assert uploaded["count"] == 1


@pytest.mark.p2
def test_webhook_canvas_constructor_exception(monkeypatch):
    module = _load_agents_app(monkeypatch)

    params = _default_webhook_params(security={"auth_type": "none"})
    cvs = _make_webhook_cvs(module, params=params)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(headers={"Content-Type": "application/json"}, json_body={}),
    )
    monkeypatch.setattr(module, "Canvas", lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("canvas init failed")))

    def fake_error_result(*, code, message):
        return SimpleNamespace(code=code, message=message)

    monkeypatch.setattr(module, "get_data_error_result", fake_error_result)
    res = _run(module.webhook("agent-1"))
    assert isinstance(res, SimpleNamespace)
    assert res.code == module.RetCode.BAD_REQUEST
    assert "canvas init failed" in res.message
    assert res.status_code == module.RetCode.BAD_REQUEST


@pytest.mark.p2
def test_webhook_trace_polling_branches(monkeypatch):
    module = _load_agents_app(monkeypatch)

    # Missing since_ts.
    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args()))
    res = _run(module.webhook_trace("agent-1"))
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["webhook_id"] is None
    assert res["data"]["events"] == []
    assert res["data"]["finished"] is False

    # since_ts provided but no Redis data.
    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args({"since_ts": "100.0"})))
    monkeypatch.setattr(module.REDIS_CONN, "get", lambda _k: None)
    res = _run(module.webhook_trace("agent-1"))
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["webhook_id"] is None
    assert res["data"]["next_since_ts"] == 100.0
    assert res["data"]["events"] == []
    assert res["data"]["finished"] is False

    webhooks_obj = {
        "webhooks": {
            "101.0": {
                "events": [
                    {"event": "message", "ts": 101.2, "data": {"content": "a"}},
                    {"event": "finished", "ts": 102.5},
                ]
            },
            "99.0": {"events": [{"event": "message", "ts": 99.1}]},
        }
    }
    raw = json.dumps(webhooks_obj)
    monkeypatch.setattr(module.REDIS_CONN, "get", lambda _k: raw)

    # No candidates newer than since_ts.
    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args({"since_ts": "200.0"})))
    res = _run(module.webhook_trace("agent-1"))
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["webhook_id"] is None
    assert res["data"]["next_since_ts"] == 200.0
    assert res["data"]["events"] == []
    assert res["data"]["finished"] is False

    # Candidate exists and webhook id is assigned.
    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args({"since_ts": "100.0"})))
    res = _run(module.webhook_trace("agent-1"))
    assert res["code"] == module.RetCode.SUCCESS
    webhook_id = res["data"]["webhook_id"]
    assert webhook_id
    assert res["data"]["events"] == []
    assert res["data"]["next_since_ts"] == 101.0
    assert res["data"]["finished"] is False

    # Invalid webhook id.
    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(args=_Args({"since_ts": "100.0", "webhook_id": "bad-id"})),
    )
    res = _run(module.webhook_trace("agent-1"))
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["webhook_id"] == "bad-id"
    assert res["data"]["events"] == []
    assert res["data"]["next_since_ts"] == 100.0
    assert res["data"]["finished"] is True

    # Valid webhook id with event filtering and finished flag.
    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(args=_Args({"since_ts": "101.0", "webhook_id": webhook_id})),
    )
    res = _run(module.webhook_trace("agent-1"))
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["webhook_id"] == webhook_id
    assert [event["ts"] for event in res["data"]["events"]] == [101.2, 102.5]
    assert res["data"]["next_since_ts"] == 102.5
    assert res["data"]["finished"] is True


@pytest.mark.p2
def test_webhook_parse_request_form_and_raw_body_paths(monkeypatch):
    module = _load_agents_app(monkeypatch)
    _patch_background_task(monkeypatch, module)

    security = {"auth_type": "none"}

    def _run_with(params, req):
        cvs = _make_webhook_cvs(module, params=params)
        monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id, _cvs=cvs: (True, _cvs))
        monkeypatch.setattr(module, "request", req)
        res = _run(module.webhook("agent-1"))
        assert hasattr(res, "status_code"), res
        assert res.status_code == 200

    _run_with(
        _default_webhook_params(security=security, content_types="application/x-www-form-urlencoded"),
        _DummyRequest(
            headers={"Content-Type": "application/x-www-form-urlencoded"},
            form={"a": "1", "b": "2"},
            json_body={},
        ),
    )

    _run_with(
        _default_webhook_params(security=security, content_types="text/plain"),
        _DummyRequest(headers={"Content-Type": "text/plain"}, raw_body=b'{"k": 1}', json_body={}),
    )

    _run_with(
        _default_webhook_params(security=security, content_types="text/plain"),
        _DummyRequest(headers={"Content-Type": "text/plain"}, raw_body=b"{bad-json}", json_body={}),
    )

    _run_with(
        _default_webhook_params(security=security, content_types="text/plain"),
        _DummyRequest(headers={"Content-Type": "text/plain"}, raw_body=b"", json_body={}),
    )

    class _BrokenRawRequest(_DummyRequest):
        async def get_data(self):
            raise RuntimeError("raw read failed")

    _run_with(
        _default_webhook_params(security=security, content_types="text/plain"),
        _BrokenRawRequest(headers={"Content-Type": "text/plain"}, json_body={}),
    )


@pytest.mark.p2
def test_webhook_schema_extract_cast_defaults_and_validation_errors(monkeypatch):
    module = _load_agents_app(monkeypatch)
    _patch_background_task(monkeypatch, module)

    base_schema = {
        "query": {
            "properties": {
                "q_file": {"type": "file"},
                "q_object": {"type": "object"},
                "q_boolean": {"type": "boolean"},
                "q_number": {"type": "number"},
                "q_string": {"type": "string"},
                "q_array": {"type": "array<string>"},
                "q_null": {"type": "null"},
                "q_default_none": {},
            },
            "required": [],
        },
        "headers": {"properties": {"Content-Type": {"type": "string"}}, "required": []},
        "body": {
            "properties": {
                "bool_true": {"type": "boolean"},
                "bool_false": {"type": "boolean"},
                "number_int": {"type": "number"},
                "number_float": {"type": "number"},
                "obj": {"type": "object"},
                "arr": {"type": "array<number>"},
                "text": {"type": "string"},
                "file_list": {"type": "file"},
                "unknown": {"type": "mystery"},
            },
            "required": [
                "bool_true",
                "number_int",
                "obj",
                "arr",
                "text",
                "file_list",
                "unknown",
            ],
        },
    }

    params = _default_webhook_params(
        security={"auth_type": "none"},
        content_types="application/json",
        schema=base_schema,
    )
    cvs = _make_webhook_cvs(module, params=params)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(
            headers={"Content-Type": "application/json"},
            args={},
            json_body={
                "bool_true": "true",
                "bool_false": "0",
                "number_int": "-3",
                "number_float": "2.5",
                "obj": '{"a": 1}',
                "arr": "[1, 2]",
                "text": "hello",
                "file_list": ["f1"],
                "unknown": "mystery",
            },
        ),
    )
    res = _run(module.webhook("agent-1"))
    assert hasattr(res, "status_code"), res
    assert res.status_code == 200

    failure_cases = [
        (
            {"query": {"properties": {}, "required": []}, "headers": {"properties": {}, "required": []}, "body": {"properties": {"must": {"type": "string"}}, "required": ["must"]}},
            {},
            "missing required field",
        ),
        (
            {"query": {"properties": {}, "required": []}, "headers": {"properties": {}, "required": []}, "body": {"properties": {"flag": {"type": "boolean"}}, "required": ["flag"]}},
            {"flag": "maybe"},
            "auto-cast failed",
        ),
        (
            {"query": {"properties": {}, "required": []}, "headers": {"properties": {}, "required": []}, "body": {"properties": {"num": {"type": "number"}}, "required": ["num"]}},
            {"num": "abc"},
            "auto-cast failed",
        ),
        (
            {"query": {"properties": {}, "required": []}, "headers": {"properties": {}, "required": []}, "body": {"properties": {"obj": {"type": "object"}}, "required": ["obj"]}},
            {"obj": "[]"},
            "auto-cast failed",
        ),
        (
            {"query": {"properties": {}, "required": []}, "headers": {"properties": {}, "required": []}, "body": {"properties": {"arr": {"type": "array<number>"}}, "required": ["arr"]}},
            {"arr": "{}"},
            "auto-cast failed",
        ),
        (
            {"query": {"properties": {}, "required": []}, "headers": {"properties": {}, "required": []}, "body": {"properties": {"num": {"type": "number"}}, "required": ["num"]}},
            {"num": []},
            "type mismatch",
        ),
        (
            {"query": {"properties": {}, "required": []}, "headers": {"properties": {}, "required": []}, "body": {"properties": {"arr": {"type": "array<number>"}}, "required": ["arr"]}},
            {"arr": 3},
            "type mismatch",
        ),
        (
            {"query": {"properties": {}, "required": []}, "headers": {"properties": {}, "required": []}, "body": {"properties": {"arr": {"type": "array<number>"}}, "required": ["arr"]}},
            {"arr": [1, "x"]},
            "type mismatch",
        ),
        (
            {"query": {"properties": {}, "required": []}, "headers": {"properties": {}, "required": []}, "body": {"properties": {"file": {"type": "file"}}, "required": ["file"]}},
            {"file": "inline-file"},
            "type mismatch",
        ),
    ]

    for schema, body_payload, expected_substring in failure_cases:
        params = _default_webhook_params(
            security={"auth_type": "none"},
            content_types="application/json",
            schema=schema,
        )
        cvs = _make_webhook_cvs(module, params=params)
        monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id, _cvs=cvs: (True, _cvs))
        monkeypatch.setattr(
            module,
            "request",
            _DummyRequest(headers={"Content-Type": "application/json"}, json_body=body_payload),
        )
        res = _run(module.webhook("agent-1"))
        _assert_bad_request(res, expected_substring)


@pytest.mark.p2
def test_webhook_immediate_response_status_and_template_validation(monkeypatch):
    module = _load_agents_app(monkeypatch)
    _patch_background_task(monkeypatch, module)

    def _run_case(response_cfg):
        params = _default_webhook_params(
            security={"auth_type": "none"},
            content_types="application/json",
            response=response_cfg,
        )
        cvs = _make_webhook_cvs(module, params=params)
        monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id, _cvs=cvs: (True, _cvs))
        monkeypatch.setattr(module, "request", _DummyRequest(headers={"Content-Type": "application/json"}, json_body={}))
        return _run(module.webhook("agent-1"))

    _assert_bad_request(_run_case({"status": "abc"}), "Invalid response status code")
    _assert_bad_request(_run_case({"status": 500}), "must be between 200 and 399")

    empty_res = _run_case({"status": 204, "body_template": ""})
    assert empty_res.status_code == 204
    assert empty_res.content_type == "application/json"
    assert _run(empty_res.get_data(as_text=True)) == "null"

    json_res = _run_case({"status": 201, "body_template": '{"ok": true}'})
    assert json_res.status_code == 201
    assert json_res.content_type == "application/json"
    assert json.loads(_run(json_res.get_data(as_text=True))) == {"ok": True}

    plain_res = _run_case({"status": 202, "body_template": "plain-text"})
    assert plain_res.status_code == 202
    assert plain_res.content_type == "text/plain"
    assert _run(plain_res.get_data(as_text=True)) == "plain-text"


@pytest.mark.p2
def test_webhook_background_run_success_and_error_trace_paths(monkeypatch):
    module = _load_agents_app(monkeypatch)

    redis_store = {}

    def redis_get(key):
        return redis_store.get(key)

    def redis_set_obj(key, obj, _ttl):
        redis_store[key] = json.dumps(obj)

    monkeypatch.setattr(module.REDIS_CONN, "get", redis_get)
    monkeypatch.setattr(module.REDIS_CONN, "set_obj", redis_set_obj)

    update_calls = []
    monkeypatch.setattr(module.UserCanvasService, "update_by_id", lambda *_args, **_kwargs: update_calls.append(True))

    tasks = []

    def _capture_task(coro):
        tasks.append(coro)
        return SimpleNamespace()

    monkeypatch.setattr(module.asyncio, "create_task", _capture_task)

    class _CanvasSuccess(_StubCanvas):
        async def run(self, **_kwargs):
            yield {"event": "message", "data": {"content": "ok"}}

        def __str__(self):
            return "{}"

    monkeypatch.setattr(module, "Canvas", _CanvasSuccess)

    params = _default_webhook_params(security={"auth_type": "none"}, content_types="application/json")
    cvs = _make_webhook_cvs(module, params=params)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(path="/api/v1/webhook_test/agent-1", headers={"Content-Type": "application/json"}, json_body={}),
    )

    res = _run(module.webhook("agent-1"))
    assert res.status_code == 200
    assert len(tasks) == 1
    _run(tasks.pop(0))
    assert update_calls == [True]

    key = "webhook-trace-agent-1-logs"
    trace_obj = json.loads(redis_store[key])
    ws = next(iter(trace_obj["webhooks"].values()))
    events = ws["events"]
    assert any(event.get("event") == "message" for event in events)
    assert any(event.get("event") == "finished" and event.get("success") is True for event in events)

    class _CanvasError(_StubCanvas):
        async def run(self, **_kwargs):
            raise RuntimeError("run failed")
            yield {}

    monkeypatch.setattr(module, "Canvas", _CanvasError)
    tasks.clear()
    redis_store.clear()
    cvs = _make_webhook_cvs(module, params=params)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id, _cvs=cvs: (True, _cvs))
    res = _run(module.webhook("agent-1"))
    assert res.status_code == 200
    _run(tasks.pop(0))
    trace_obj = json.loads(redis_store[key])
    ws = next(iter(trace_obj["webhooks"].values()))
    events = ws["events"]
    assert any(event.get("event") == "error" for event in events)
    assert any(event.get("event") == "finished" and event.get("success") is False for event in events)

    log_messages = []
    monkeypatch.setattr(module.logging, "exception", lambda msg, *_args, **_kwargs: log_messages.append(str(msg)))
    monkeypatch.setattr(module.REDIS_CONN, "get", lambda _key: "{")
    monkeypatch.setattr(module.REDIS_CONN, "set_obj", lambda *_args, **_kwargs: None)
    tasks.clear()
    cvs = _make_webhook_cvs(module, params=params)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id, _cvs=cvs: (True, _cvs))
    _run(module.webhook("agent-1"))
    _run(tasks.pop(0))
    assert any("Failed to append webhook trace" in msg for msg in log_messages)


@pytest.mark.p2
def test_webhook_sse_success_and_exception_paths(monkeypatch):
    module = _load_agents_app(monkeypatch)

    redis_store = {}
    monkeypatch.setattr(module.REDIS_CONN, "get", lambda key: redis_store.get(key))
    monkeypatch.setattr(module.REDIS_CONN, "set_obj", lambda key, obj, _ttl: redis_store.__setitem__(key, json.dumps(obj)))

    params = _default_webhook_params(
        security={"auth_type": "none"},
        content_types="application/json",
        execution_mode="Deferred",
    )
    cvs = _make_webhook_cvs(module, params=params)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))

    class _CanvasSSESuccess(_StubCanvas):
        async def run(self, **_kwargs):
            yield {"event": "message", "data": {"content": "x", "start_to_think": True}}
            yield {"event": "message", "data": {"content": "y", "end_to_think": True}}
            yield {"event": "message", "data": {"content": "Hello"}}
            yield {"event": "message_end", "data": {"status": "201"}}

    monkeypatch.setattr(module, "Canvas", _CanvasSSESuccess)
    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(path="/api/v1/webhook_test/agent-1", headers={"Content-Type": "application/json"}, json_body={}),
    )
    res = _run(module.webhook("agent-1"))
    assert res.status_code == 201
    payload = json.loads(_run(res.get_data(as_text=True)))
    assert payload == {"message": "<think></think>Hello", "success": True, "code": 201}

    class _CanvasSSEError(_StubCanvas):
        async def run(self, **_kwargs):
            raise RuntimeError("sse failed")
            yield {}

    monkeypatch.setattr(module, "Canvas", _CanvasSSEError)
    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(path="/api/v1/webhook_test/agent-1", headers={"Content-Type": "application/json"}, json_body={}),
    )
    res = _run(module.webhook("agent-1"))
    assert res.status_code == 400
    payload = json.loads(_run(res.get_data(as_text=True)))
    assert payload["code"] == 400
    assert payload["success"] is False
    assert "sse failed" in payload["message"]


@pytest.mark.p2
def test_webhook_trace_encoded_id_generation(monkeypatch):
    module = _load_agents_app(monkeypatch)

    webhooks_obj = {
        "webhooks": {
            "101.0": {
                "events": [{"event": "message", "ts": 101.2}],
            }
        }
    }
    monkeypatch.setattr(module.REDIS_CONN, "get", lambda _key: json.dumps(webhooks_obj))
    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args({"since_ts": "100.0"})))
    res = _run(module.webhook_trace("agent-1"))
    assert res["code"] == module.RetCode.SUCCESS

    expected = base64.urlsafe_b64encode(
        hmac.new(
            b"webhook_id_secret",
            b"101.0",
            hashlib.sha256,
        ).digest()
    ).decode("utf-8").rstrip("=")
    assert res["data"]["webhook_id"] == expected

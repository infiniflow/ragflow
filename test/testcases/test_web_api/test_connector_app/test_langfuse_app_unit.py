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


class _DummyAtomic:
    def __enter__(self):
        return self

    def __exit__(self, _exc_type, _exc, _tb):
        return False


class _FakeApiError(Exception):
    pass


class _FakeLangfuseClient:
    def __init__(self, *, auth_result=True, auth_exc=None, project_payload=None):
        self._auth_result = auth_result
        self._auth_exc = auth_exc
        if project_payload is None:
            project_payload = {"data": [{"id": "project-id", "name": "project-name"}]}
        self.api = SimpleNamespace(
            projects=SimpleNamespace(get=lambda: SimpleNamespace(dict=lambda: project_payload)),
            core=SimpleNamespace(api_error=SimpleNamespace(ApiError=_FakeApiError)),
        )

    def auth_check(self):
        if self._auth_exc is not None:
            raise self._auth_exc
        return self._auth_result


def _run(coro):
    return asyncio.run(coro)


def _load_langfuse_app(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    stub_apps = ModuleType("api.apps")
    stub_apps.current_user = SimpleNamespace(id="tenant-1")
    stub_apps.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", stub_apps)

    stub_langfuse = ModuleType("langfuse")
    stub_langfuse.Langfuse = _FakeLangfuseClient
    monkeypatch.setitem(sys.modules, "langfuse", stub_langfuse)

    module_path = repo_root / "api" / "apps" / "langfuse_app.py"
    spec = importlib.util.spec_from_file_location("test_langfuse_app_unit", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_set_api_key_missing_fields_and_invalid_auth(monkeypatch):
    module = _load_langfuse_app(monkeypatch)
    monkeypatch.setattr(module.DB, "atomic", lambda: _DummyAtomic())

    async def missing_fields():
        return {"secret_key": "", "public_key": "pub", "host": "http://host"}

    monkeypatch.setattr(module, "get_request_json", missing_fields)
    res = _run(module.set_api_key.__wrapped__())
    assert res["code"] == 102
    assert res["message"] == "Missing required fields"

    async def invalid_auth():
        return {"secret_key": "sec", "public_key": "pub", "host": "http://host"}

    monkeypatch.setattr(module, "get_request_json", invalid_auth)
    monkeypatch.setattr(module, "Langfuse", lambda **_kwargs: _FakeLangfuseClient(auth_result=False))
    res = _run(module.set_api_key.__wrapped__())
    assert res["code"] == 102
    assert res["message"] == "Invalid Langfuse keys"


@pytest.mark.p2
def test_set_api_key_create_update_and_atomic_exception(monkeypatch):
    module = _load_langfuse_app(monkeypatch)
    monkeypatch.setattr(module.DB, "atomic", lambda: _DummyAtomic())
    monkeypatch.setattr(module, "Langfuse", lambda **_kwargs: _FakeLangfuseClient(auth_result=True))

    async def payload():
        return {"secret_key": "sec", "public_key": "pub", "host": "http://host"}

    monkeypatch.setattr(module, "get_request_json", payload)

    calls = {"save": 0, "update": 0}
    monkeypatch.setattr(module.TenantLangfuseService, "filter_by_tenant", lambda **_kwargs: None)
    monkeypatch.setattr(
        module.TenantLangfuseService,
        "save",
        lambda **_kwargs: calls.__setitem__("save", calls["save"] + 1),
    )
    monkeypatch.setattr(
        module.TenantLangfuseService,
        "update_by_tenant",
        lambda **_kwargs: calls.__setitem__("update", calls["update"] + 1),
    )
    res = _run(module.set_api_key.__wrapped__())
    assert res["code"] == 0
    assert calls["save"] == 1

    monkeypatch.setattr(module.TenantLangfuseService, "filter_by_tenant", lambda **_kwargs: {"id": "existing"})
    res = _run(module.set_api_key.__wrapped__())
    assert res["code"] == 0
    assert calls["update"] == 1

    monkeypatch.setattr(module.TenantLangfuseService, "filter_by_tenant", lambda **_kwargs: None)

    def raise_save(**_kwargs):
        raise RuntimeError("save failed")

    monkeypatch.setattr(module.TenantLangfuseService, "save", raise_save)
    res = _run(module.set_api_key.__wrapped__())
    assert res["code"] == 100
    assert "save failed" in res["message"]


@pytest.mark.p2
def test_get_api_key_no_record_invalid_auth_api_error_generic_error_success(monkeypatch):
    module = _load_langfuse_app(monkeypatch)

    monkeypatch.setattr(module.TenantLangfuseService, "filter_by_tenant_with_info", lambda **_kwargs: None)
    res = module.get_api_key.__wrapped__()
    assert res["code"] == 0
    assert res["message"] == "Have not record any Langfuse keys."

    base_entry = {"secret_key": "sec", "public_key": "pub", "host": "http://host"}
    monkeypatch.setattr(module.TenantLangfuseService, "filter_by_tenant_with_info", lambda **_kwargs: dict(base_entry))
    monkeypatch.setattr(module, "Langfuse", lambda **_kwargs: _FakeLangfuseClient(auth_result=False))
    res = module.get_api_key.__wrapped__()
    assert res["code"] == 102
    assert res["message"] == "Invalid Langfuse keys loaded"

    monkeypatch.setattr(
        module,
        "Langfuse",
        lambda **_kwargs: _FakeLangfuseClient(auth_exc=_FakeApiError("api exploded")),
    )
    res = module.get_api_key.__wrapped__()
    assert res["code"] == 0
    assert "Error from Langfuse" in res["message"]

    monkeypatch.setattr(
        module,
        "Langfuse",
        lambda **_kwargs: _FakeLangfuseClient(auth_exc=RuntimeError("generic exploded")),
    )
    res = module.get_api_key.__wrapped__()
    assert res["code"] == 100
    assert "generic exploded" in res["message"]

    monkeypatch.setattr(module, "Langfuse", lambda **_kwargs: _FakeLangfuseClient(auth_result=True))
    res = module.get_api_key.__wrapped__()
    assert res["code"] == 0
    assert res["data"]["project_id"] == "project-id"
    assert res["data"]["project_name"] == "project-name"


@pytest.mark.p2
def test_delete_api_key_no_record_success_exception(monkeypatch):
    module = _load_langfuse_app(monkeypatch)
    monkeypatch.setattr(module.DB, "atomic", lambda: _DummyAtomic())

    monkeypatch.setattr(module.TenantLangfuseService, "filter_by_tenant", lambda **_kwargs: None)
    res = module.delete_api_key.__wrapped__()
    assert res["code"] == 0
    assert res["message"] == "Have not record any Langfuse keys."

    monkeypatch.setattr(module.TenantLangfuseService, "filter_by_tenant", lambda **_kwargs: {"id": "entry"})
    monkeypatch.setattr(module.TenantLangfuseService, "delete_model", lambda _entry: None)
    res = module.delete_api_key.__wrapped__()
    assert res["code"] == 0
    assert res["data"] is True

    def raise_delete(_entry):
        raise RuntimeError("delete failed")

    monkeypatch.setattr(module.TenantLangfuseService, "delete_model", raise_delete)
    res = module.delete_api_key.__wrapped__()
    assert res["code"] == 100
    assert "delete failed" in res["message"]

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
from copy import deepcopy
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


class _Args(dict):
    def get(self, key, default=None):
        return super().get(key, default)


class _EnumValue:
    def __init__(self, value):
        self.value = value


class _DummyStatusEnum:
    VALID = _EnumValue("1")


class _DummyRetCode:
    SUCCESS = 0
    EXCEPTION_ERROR = 100
    ARGUMENT_ERROR = 101
    DATA_ERROR = 102
    OPERATING_ERROR = 103
    AUTHENTICATION_ERROR = 109


class _SearchRecord:
    def __init__(self, search_id="search-1", name="search", search_config=None):
        self.id = search_id
        self.name = name
        self.search_config = {} if search_config is None else dict(search_config)

    def to_dict(self):
        return {"id": self.id, "name": self.name, "search_config": dict(self.search_config)}


def _run(coro):
    return asyncio.run(coro)


def _set_request_json(monkeypatch, module, payload):
    async def _request_json():
        return deepcopy(payload)

    monkeypatch.setattr(module, "get_request_json", _request_json)


def _set_request_args(monkeypatch, module, args=None):
    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args(args or {})))


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


def _load_search_app(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    quart_mod = ModuleType("quart")
    quart_mod.request = SimpleNamespace(args=_Args())
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    misc_utils_mod = ModuleType("common.misc_utils")
    misc_utils_mod.get_uuid = lambda: "search-uuid-1"
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)
    common_pkg.misc_utils = misc_utils_mod

    constants_mod = ModuleType("common.constants")
    constants_mod.RetCode = _DummyRetCode
    constants_mod.StatusEnum = _DummyStatusEnum
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)
    common_pkg.constants = constants_mod

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="tenant-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)
    api_pkg.apps = apps_mod

    constants_api_mod = ModuleType("api.constants")
    constants_api_mod.DATASET_NAME_LIMIT = 255
    monkeypatch.setitem(sys.modules, "api.constants", constants_api_mod)

    db_pkg = ModuleType("api.db")
    db_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db", db_pkg)
    api_pkg.db = db_pkg

    db_models_mod = ModuleType("api.db.db_models")

    class _DummyDB:
        @staticmethod
        def atomic():
            return _DummyAtomic()

    db_models_mod.DB = _DummyDB
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    services_pkg.duplicate_name = lambda _checker, **kwargs: kwargs.get("name", "")
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    search_service_mod = ModuleType("api.db.services.search_service")

    class _SearchService:
        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def save(**_kwargs):
            return True

        @staticmethod
        def accessible4deletion(_search_id, _user_id):
            return True

        @staticmethod
        def update_by_id(_search_id, _req):
            return True

        @staticmethod
        def get_by_id(_search_id):
            return True, _SearchRecord(search_id=_search_id, name="updated")

        @staticmethod
        def get_detail(_search_id):
            return {"id": _search_id}

        @staticmethod
        def get_by_tenant_ids(_tenants, _user_id, _page_number, _items_per_page, _orderby, _desc, _keywords):
            return [], 0

        @staticmethod
        def delete_by_id(_search_id):
            return True

    search_service_mod.SearchService = _SearchService
    monkeypatch.setitem(sys.modules, "api.db.services.search_service", search_service_mod)

    user_service_mod = ModuleType("api.db.services.user_service")

    class _TenantService:
        @staticmethod
        def get_by_id(_tenant_id):
            return True, SimpleNamespace(id=_tenant_id)

    class _UserTenantService:
        @staticmethod
        def query(**_kwargs):
            return [SimpleNamespace(tenant_id="tenant-1")]

    user_service_mod.TenantService = _TenantService
    user_service_mod.UserTenantService = _UserTenantService
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)

    utils_pkg = ModuleType("api.utils")
    utils_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.utils", utils_pkg)

    api_utils_mod = ModuleType("api.utils.api_utils")

    async def _default_request_json():
        return {}

    def _get_data_error_result(code=_DummyRetCode.DATA_ERROR, message="Sorry! Data missing!"):
        return {"code": code, "message": message}

    def _get_json_result(code=_DummyRetCode.SUCCESS, message="success", data=None):
        return {"code": code, "message": message, "data": data}

    def _server_error_response(error):
        return {"code": _DummyRetCode.EXCEPTION_ERROR, "message": repr(error)}

    def _validate_request(*_args, **_kwargs):
        def _decorator(func):
            return func

        return _decorator

    def _not_allowed_parameters(*_params):
        def _decorator(func):
            return func

        return _decorator

    api_utils_mod.get_request_json = _default_request_json
    api_utils_mod.get_data_error_result = _get_data_error_result
    api_utils_mod.get_json_result = _get_json_result
    api_utils_mod.server_error_response = _server_error_response
    api_utils_mod.validate_request = _validate_request
    api_utils_mod.not_allowed_parameters = _not_allowed_parameters
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)
    utils_pkg.api_utils = api_utils_mod

    module_name = "test_search_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "search_app.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_create_route_matrix_unit(monkeypatch):
    module = _load_search_app(monkeypatch)

    _set_request_json(monkeypatch, module, {"name": 1})
    res = _run(module.create())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "must be string" in res["message"]

    _set_request_json(monkeypatch, module, {"name": "   "})
    res = _run(module.create())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "empty" in res["message"].lower()

    _set_request_json(monkeypatch, module, {"name": "a" * 256})
    res = _run(module.create())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "255" in res["message"]

    _set_request_json(monkeypatch, module, {"name": "create-auth-fail"})
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tenant_id: (False, None))
    res = _run(module.create())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "authorized identity" in res["message"].lower()

    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tenant_id: (True, SimpleNamespace(id=_tenant_id)))
    monkeypatch.setattr(module, "duplicate_name", lambda _checker, **kwargs: kwargs["name"] + "_dedup")
    _set_request_json(monkeypatch, module, {"name": "create-fail", "description": "d"})
    monkeypatch.setattr(module.SearchService, "save", lambda **_kwargs: False)
    res = _run(module.create())
    assert res["code"] == module.RetCode.DATA_ERROR

    _set_request_json(monkeypatch, module, {"name": "create-ok", "description": "d"})
    monkeypatch.setattr(module.SearchService, "save", lambda **_kwargs: True)
    res = _run(module.create())
    assert res["code"] == 0
    assert res["data"]["search_id"] == "search-uuid-1"

    def _raise_save(**_kwargs):
        raise RuntimeError("save boom")

    monkeypatch.setattr(module.SearchService, "save", _raise_save)
    _set_request_json(monkeypatch, module, {"name": "create-exception", "description": "d"})
    res = _run(module.create())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "save boom" in res["message"]


@pytest.mark.p2
def test_update_and_detail_route_matrix_unit(monkeypatch):
    module = _load_search_app(monkeypatch)

    _set_request_json(monkeypatch, module, {"search_id": "s1", "name": 1, "search_config": {}, "tenant_id": "tenant-1"})
    res = _run(module.update())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "must be string" in res["message"]

    _set_request_json(monkeypatch, module, {"search_id": "s1", "name": "   ", "search_config": {}, "tenant_id": "tenant-1"})
    res = _run(module.update())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "empty" in res["message"].lower()

    _set_request_json(monkeypatch, module, {"search_id": "s1", "name": "a" * 256, "search_config": {}, "tenant_id": "tenant-1"})
    res = _run(module.update())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "large than" in res["message"]

    _set_request_json(monkeypatch, module, {"search_id": "s1", "name": "ok", "search_config": {}, "tenant_id": "tenant-1"})
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tenant_id: (False, None))
    res = _run(module.update())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "authorized identity" in res["message"].lower()

    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tenant_id: (True, SimpleNamespace(id=_tenant_id)))
    monkeypatch.setattr(module.SearchService, "accessible4deletion", lambda _search_id, _user_id: False)
    _set_request_json(monkeypatch, module, {"search_id": "s1", "name": "ok", "search_config": {}, "tenant_id": "tenant-1"})
    res = _run(module.update())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert "authorization" in res["message"].lower()

    monkeypatch.setattr(module.SearchService, "accessible4deletion", lambda _search_id, _user_id: True)
    monkeypatch.setattr(module.SearchService, "query", lambda **_kwargs: [None])
    _set_request_json(monkeypatch, module, {"search_id": "s1", "name": "ok", "search_config": {}, "tenant_id": "tenant-1"})
    res = _run(module.update())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "cannot find search" in res["message"].lower()

    existing = _SearchRecord(search_id="s1", name="old-name", search_config={"existing": 1})

    def _query_duplicate(**kwargs):
        if "id" in kwargs:
            return [existing]
        if "name" in kwargs:
            return [SimpleNamespace(id="dup")]
        return []

    monkeypatch.setattr(module.SearchService, "query", _query_duplicate)
    _set_request_json(monkeypatch, module, {"search_id": "s1", "name": "new-name", "search_config": {}, "tenant_id": "tenant-1"})
    res = _run(module.update())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "duplicated" in res["message"].lower()

    monkeypatch.setattr(module.SearchService, "query", lambda **_kwargs: [existing])
    _set_request_json(monkeypatch, module, {"search_id": "s1", "name": "old-name", "search_config": [], "tenant_id": "tenant-1"})
    res = _run(module.update())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "json object" in res["message"].lower()

    captured = {}

    def _update_fail(search_id, req):
        captured["search_id"] = search_id
        captured["req"] = dict(req)
        return False

    monkeypatch.setattr(module.SearchService, "update_by_id", _update_fail)
    _set_request_json(monkeypatch, module, {"search_id": "s1", "name": "old-name", "search_config": {"top_k": 3}, "tenant_id": "tenant-1"})
    res = _run(module.update())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "failed to update" in res["message"].lower()
    assert captured["search_id"] == "s1"
    assert "search_id" not in captured["req"]
    assert "tenant_id" not in captured["req"]
    assert captured["req"]["search_config"] == {"existing": 1, "top_k": 3}

    monkeypatch.setattr(module.SearchService, "update_by_id", lambda _search_id, _req: True)
    monkeypatch.setattr(module.SearchService, "get_by_id", lambda _search_id: (False, None))
    res = _run(module.update())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "failed to fetch" in res["message"].lower()

    monkeypatch.setattr(
        module.SearchService,
        "get_by_id",
        lambda _search_id: (True, _SearchRecord(search_id=_search_id, name="old-name", search_config={"existing": 1, "top_k": 3})),
    )
    res = _run(module.update())
    assert res["code"] == 0
    assert res["data"]["id"] == "s1"

    def _raise_query(**_kwargs):
        raise RuntimeError("update boom")

    monkeypatch.setattr(module.SearchService, "query", _raise_query)
    _set_request_json(monkeypatch, module, {"search_id": "s1", "name": "old-name", "search_config": {"top_k": 3}, "tenant_id": "tenant-1"})
    res = _run(module.update())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "update boom" in res["message"]

    _set_request_args(monkeypatch, module, {"search_id": "s1"})
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-a")])
    monkeypatch.setattr(module.SearchService, "query", lambda **_kwargs: [])
    res = module.detail()
    assert res["code"] == module.RetCode.OPERATING_ERROR
    assert "permission" in res["message"].lower()

    monkeypatch.setattr(module.SearchService, "query", lambda **_kwargs: [SimpleNamespace(id="s1")])
    monkeypatch.setattr(module.SearchService, "get_detail", lambda _search_id: None)
    res = module.detail()
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "can't find" in res["message"].lower()

    monkeypatch.setattr(module.SearchService, "get_detail", lambda _search_id: {"id": _search_id, "name": "detail-name"})
    res = module.detail()
    assert res["code"] == 0
    assert res["data"]["id"] == "s1"

    def _raise_detail(_search_id):
        raise RuntimeError("detail boom")

    monkeypatch.setattr(module.SearchService, "get_detail", _raise_detail)
    res = module.detail()
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "detail boom" in res["message"]


@pytest.mark.p2
def test_list_and_rm_route_matrix_unit(monkeypatch):
    module = _load_search_app(monkeypatch)

    _set_request_args(
        monkeypatch,
        module,
        {"keywords": "k", "page": "1", "page_size": "2", "orderby": "create_time", "desc": "false"},
    )
    _set_request_json(monkeypatch, module, {"owner_ids": []})
    monkeypatch.setattr(
        module.SearchService,
        "get_by_tenant_ids",
        lambda _tenants, _uid, _page, _size, _orderby, _desc, _keywords: ([{"id": "a", "tenant_id": "tenant-1"}], 1),
    )
    res = _run(module.list_search_app())
    assert res["code"] == 0
    assert res["data"]["total"] == 1
    assert res["data"]["search_apps"][0]["id"] == "a"

    _set_request_args(
        monkeypatch,
        module,
        {"keywords": "k", "page": "1", "page_size": "1", "orderby": "create_time", "desc": "true"},
    )
    _set_request_json(monkeypatch, module, {"owner_ids": ["tenant-1"]})
    monkeypatch.setattr(
        module.SearchService,
        "get_by_tenant_ids",
        lambda _tenants, _uid, _page, _size, _orderby, _desc, _keywords: (
            [{"id": "x", "tenant_id": "tenant-1"}, {"id": "y", "tenant_id": "tenant-2"}],
            2,
        ),
    )
    res = _run(module.list_search_app())
    assert res["code"] == 0
    assert res["data"]["total"] == 1
    assert len(res["data"]["search_apps"]) == 1
    assert res["data"]["search_apps"][0]["tenant_id"] == "tenant-1"

    def _raise_list(*_args, **_kwargs):
        raise RuntimeError("list boom")

    monkeypatch.setattr(module.SearchService, "get_by_tenant_ids", _raise_list)
    _set_request_json(monkeypatch, module, {"owner_ids": []})
    res = _run(module.list_search_app())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "list boom" in res["message"]

    _set_request_json(monkeypatch, module, {"search_id": "search-1"})
    monkeypatch.setattr(module.SearchService, "accessible4deletion", lambda _search_id, _user_id: False)
    res = _run(module.rm())
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert "authorization" in res["message"].lower()

    monkeypatch.setattr(module.SearchService, "accessible4deletion", lambda _search_id, _user_id: True)
    monkeypatch.setattr(module.SearchService, "delete_by_id", lambda _search_id: False)
    res = _run(module.rm())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "failed to delete" in res["message"].lower()

    monkeypatch.setattr(module.SearchService, "delete_by_id", lambda _search_id: True)
    res = _run(module.rm())
    assert res["code"] == 0
    assert res["data"] is True

    def _raise_delete(_search_id):
        raise RuntimeError("rm boom")

    monkeypatch.setattr(module.SearchService, "delete_by_id", _raise_delete)
    res = _run(module.rm())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "rm boom" in res["message"]

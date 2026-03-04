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
import inspect
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from functools import wraps

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
    def get(self, key, default=None):
        return super().get(key, default)


def _run(coro):
    return asyncio.run(coro)


def _set_request_json(monkeypatch, module, payload):
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(payload))


def _set_request_args(monkeypatch, module, args):
    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args(args)))


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


def _load_dialog_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    quart_mod = ModuleType("quart")
    quart_mod.request = SimpleNamespace(args=_Args())
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="tenant-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)
    api_pkg.apps = apps_mod

    db_pkg = ModuleType("api.db")
    db_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db", db_pkg)
    api_pkg.db = db_pkg

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    services_pkg.duplicate_name = lambda _checker, **kwargs: kwargs.get("name", "")
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    dialog_service_mod = ModuleType("api.db.services.dialog_service")

    class _DialogService:
        model = SimpleNamespace(create_time="create_time")

        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def save(**_kwargs):
            return True

        @staticmethod
        def update_by_id(*_args, **_kwargs):
            return True

        @staticmethod
        def get_by_id(_id):
            return True, SimpleNamespace(to_dict=lambda: {"id": _id, "kb_ids": []})

        @staticmethod
        def get_by_tenant_ids(*_args, **_kwargs):
            return [], 0

        @staticmethod
        def update_many_by_id(_payload):
            return True

    dialog_service_mod.DialogService = _DialogService
    monkeypatch.setitem(sys.modules, "api.db.services.dialog_service", dialog_service_mod)

    tenant_llm_service_mod = ModuleType("api.db.services.tenant_llm_service")

    class _TenantLLMService:
        @staticmethod
        def split_model_name_and_factory(embd_id):
            return embd_id.split("@")

    tenant_llm_service_mod.TenantLLMService = _TenantLLMService
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_llm_service", tenant_llm_service_mod)

    knowledgebase_service_mod = ModuleType("api.db.services.knowledgebase_service")

    class _KnowledgebaseService:
        @staticmethod
        def get_by_ids(_ids):
            return []

        @staticmethod
        def get_by_id(_id):
            return False, None

        @staticmethod
        def query(**_kwargs):
            return []

    knowledgebase_service_mod.KnowledgebaseService = _KnowledgebaseService
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", knowledgebase_service_mod)

    user_service_mod = ModuleType("api.db.services.user_service")

    class _TenantService:
        @staticmethod
        def get_by_id(_id):
            return True, SimpleNamespace(llm_id="llm-default")

    class _UserTenantService:
        @staticmethod
        def query(**_kwargs):
            return [SimpleNamespace(tenant_id="tenant-1")]

    user_service_mod.TenantService = _TenantService
    user_service_mod.UserTenantService = _UserTenantService
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")
    from common.constants import RetCode

    async def _default_request_json():
        return {}

    def _get_data_error_result(code=RetCode.DATA_ERROR, message="Sorry! Data missing!"):
        return {"code": code, "message": message}

    def _get_json_result(code=RetCode.SUCCESS, message="success", data=None):
        return {"code": code, "message": message, "data": data}

    def _server_error_response(error):
        return {"code": RetCode.EXCEPTION_ERROR, "message": repr(error)}

    def _validate_request(*_args, **_kwargs):
        def _decorator(func):
            if inspect.iscoroutinefunction(func):
                @wraps(func)
                async def _wrapped(*func_args, **func_kwargs):
                    return await func(*func_args, **func_kwargs)

                return _wrapped

            @wraps(func)
            def _wrapped(*func_args, **func_kwargs):
                return func(*func_args, **func_kwargs)

            return _wrapped

        return _decorator

    api_utils_mod.get_request_json = _default_request_json
    api_utils_mod.get_data_error_result = _get_data_error_result
    api_utils_mod.get_json_result = _get_json_result
    api_utils_mod.server_error_response = _server_error_response
    api_utils_mod.validate_request = _validate_request
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    module_name = "test_dialog_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "dialog_app.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_set_dialog_branch_matrix_unit(monkeypatch):
    module = _load_dialog_module(monkeypatch)
    handler = inspect.unwrap(module.set_dialog)

    _set_request_json(monkeypatch, module, {"name": 1, "prompt_config": {"system": "", "parameters": []}})
    res = _run(handler())
    assert res["message"] == "Dialog name must be string."

    _set_request_json(monkeypatch, module, {"name": " ", "prompt_config": {"system": "", "parameters": []}})
    res = _run(handler())
    assert res["message"] == "Dialog name can't be empty."

    _set_request_json(monkeypatch, module, {"name": "a" * 256, "prompt_config": {"system": "", "parameters": []}})
    res = _run(handler())
    assert res["message"] == "Dialog name length is 256 which is larger than 255"

    captured = {}

    def _dup_name(checker, **kwargs):
        assert checker(name=kwargs["name"]) is True
        return kwargs["name"] + " (1)"

    monkeypatch.setattr(module, "duplicate_name", _dup_name)
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(name="new dialog")])
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _id: (True, SimpleNamespace(llm_id="llm-x")))
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_ids", lambda _ids: [SimpleNamespace(embd_id="embd-a@builtin")])
    monkeypatch.setattr(module.TenantLLMService, "split_model_name_and_factory", lambda embd_id: embd_id.split("@"))
    monkeypatch.setattr(module.DialogService, "save", lambda **kwargs: captured.update(kwargs) or False)
    _set_request_json(
        monkeypatch,
        module,
        {
            "name": "New Dialog",
            "kb_ids": ["kb-1"],
            "prompt_config": {"system": "Use {knowledge}", "parameters": []},
        },
    )
    res = _run(handler())
    assert res["message"] == "Fail to new a dialog!"
    assert captured["name"] == "New Dialog (1)"
    assert captured["prompt_config"]["parameters"] == [{"key": "knowledge", "optional": False}]

    _set_request_json(
        monkeypatch,
        module,
        {
            "dialog_id": "dialog-1",
            "name": "Update",
            "kb_ids": [],
            "prompt_config": {
                "system": "Use {knowledge}",
                "parameters": [{"key": "knowledge", "optional": True}],
            },
        },
    )
    res = _run(handler())
    assert "Please remove `{knowledge}` in system prompt" in res["message"]

    _set_request_json(
        monkeypatch,
        module,
        {"name": "demo", "prompt_config": {"system": "hello", "parameters": [{"key": "must", "optional": False}]}},
    )
    res = _run(handler())
    assert "Parameter 'must' is not used" in res["message"]

    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _id: (False, None))
    _set_request_json(monkeypatch, module, {"name": "demo", "prompt_config": {"system": "hello", "parameters": []}})
    res = _run(handler())
    assert res["message"] == "Tenant not found!"

    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _id: (True, SimpleNamespace(llm_id="llm-x")))
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue(
            {
                "name": "demo",
                "kb_ids": ["kb-1", "kb-2"],
                "prompt_config": {"system": "hello", "parameters": []},
            }
        ),
    )
    monkeypatch.setattr(
        module.KnowledgebaseService,
        "get_by_ids",
        lambda _ids: [SimpleNamespace(embd_id="embd-a@f1"), SimpleNamespace(embd_id="embd-b@f2")],
    )
    monkeypatch.setattr(module.TenantLLMService, "split_model_name_and_factory", lambda embd_id: embd_id.split("@"))
    res = _run(handler())
    assert "Datasets use different embedding models" in res["message"]

    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue(
            {
                "name": "optional-param-dialog",
                "prompt_config": {"system": "hello", "parameters": [{"key": "ignored", "optional": True}]},
            }
        ),
    )
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_ids", lambda _ids: [])
    monkeypatch.setattr(module.DialogService, "save", lambda **_kwargs: False)
    res = _run(handler())
    assert res["message"] == "Fail to new a dialog!"

    monkeypatch.setattr(module.KnowledgebaseService, "get_by_ids", lambda _ids: [])
    monkeypatch.setattr(module.DialogService, "update_by_id", lambda *_args, **_kwargs: False)
    _set_request_json(
        monkeypatch,
        module,
        {
            "dialog_id": "dialog-1",
            "kb_names": ["legacy"],
            "name": "rename",
            "prompt_config": {"system": "hello", "parameters": []},
        },
    )
    res = _run(handler())
    assert res["message"] == "Dialog not found!"

    monkeypatch.setattr(module.DialogService, "update_by_id", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (False, None))
    _set_request_json(
        monkeypatch,
        module,
        {
            "dialog_id": "dialog-1",
            "name": "rename",
            "prompt_config": {"system": "hello", "parameters": []},
        },
    )
    res = _run(handler())
    assert res["message"] == "Fail to update a dialog!"

    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, SimpleNamespace(to_dict=lambda: {"id": _id, "kb_ids": ["kb-1"]})))
    monkeypatch.setattr(
        module.KnowledgebaseService,
        "get_by_id",
        lambda _id: (True, SimpleNamespace(status=module.StatusEnum.VALID.value, name="KB One")),
    )
    _set_request_json(
        monkeypatch,
        module,
        {
            "dialog_id": "dialog-1",
            "kb_names": ["legacy"],
            "name": "new-name",
            "prompt_config": {"system": "hello", "parameters": []},
        },
    )
    res = _run(handler())
    assert res["code"] == 0
    assert res["data"]["name"] == "new-name"
    assert res["data"]["kb_names"] == ["KB One"]

    def _raise_tenant(_id):
        raise RuntimeError("set boom")

    monkeypatch.setattr(module.TenantService, "get_by_id", _raise_tenant)
    _set_request_json(monkeypatch, module, {"name": "demo", "prompt_config": {"system": "hello", "parameters": []}})
    res = _run(handler())
    assert "set boom" in res["message"]


@pytest.mark.p2
def test_get_get_kb_names_and_list_dialogs_exception_matrix_unit(monkeypatch):
    module = _load_dialog_module(monkeypatch)
    get_handler = inspect.unwrap(module.get)

    monkeypatch.setattr(
        module.DialogService,
        "get_by_id",
        lambda _id: (True, SimpleNamespace(to_dict=lambda: {"id": _id, "kb_ids": ["kb-1", "kb-2"]})),
    )
    monkeypatch.setattr(
        module.KnowledgebaseService,
        "get_by_id",
        lambda kid: (
            (True, SimpleNamespace(status=module.StatusEnum.VALID.value, name="KB-1"))
            if kid == "kb-1"
            else (False, None)
        ),
    )
    _set_request_args(monkeypatch, module, {"dialog_id": "dialog-1"})
    res = get_handler()
    assert res["code"] == 0
    assert res["data"]["kb_ids"] == ["kb-1"]
    assert res["data"]["kb_names"] == ["KB-1"]

    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (False, None))
    _set_request_args(monkeypatch, module, {"dialog_id": "dialog-missing"})
    res = get_handler()
    assert res["message"] == "Dialog not found!"

    def _raise_get(_id):
        raise RuntimeError("get boom")

    monkeypatch.setattr(module.DialogService, "get_by_id", _raise_get)
    _set_request_args(monkeypatch, module, {"dialog_id": "dialog-1"})
    res = get_handler()
    assert "get boom" in res["message"]

    monkeypatch.setattr(
        module.KnowledgebaseService,
        "get_by_id",
        lambda kid: (
            (True, SimpleNamespace(status=module.StatusEnum.VALID.value, name=f"KB-{kid}"))
            if kid.startswith("ok")
            else (True, SimpleNamespace(status=module.StatusEnum.INVALID.value, name=f"BAD-{kid}"))
        ),
    )
    ids, names = module.get_kb_names(["ok-1", "bad-1", "ok-2"])
    assert ids == ["ok-1", "ok-2"]
    assert names == ["KB-ok-1", "KB-ok-2"]

    def _raise_list(**_kwargs):
        raise RuntimeError("list boom")

    monkeypatch.setattr(module.DialogService, "query", _raise_list)
    res = module.list_dialogs()
    assert "list boom" in res["message"]


@pytest.mark.p2
def test_list_dialogs_next_owner_desc_and_pagination_matrix_unit(monkeypatch):
    module = _load_dialog_module(monkeypatch)
    handler = inspect.unwrap(module.list_dialogs_next)

    calls = []

    def _get_by_tenant_ids(tenants, user_id, page_number, items_per_page, orderby, desc, keywords, parser_id):
        calls.append(
            {
                "tenants": tenants,
                "user_id": user_id,
                "page_number": page_number,
                "items_per_page": items_per_page,
                "orderby": orderby,
                "desc": desc,
                "keywords": keywords,
                "parser_id": parser_id,
            }
        )
        if tenants:
            return (
                [
                    {"id": "dialog-1", "tenant_id": "tenant-a"},
                    {"id": "dialog-2", "tenant_id": "tenant-x"},
                    {"id": "dialog-3", "tenant_id": "tenant-b"},
                ],
                3,
            )
        return ([{"id": "dialog-0", "tenant_id": "tenant-1"}], 1)

    monkeypatch.setattr(module.DialogService, "get_by_tenant_ids", _get_by_tenant_ids)

    _set_request_args(
        monkeypatch,
        module,
        {
            "keywords": "k",
            "page": "1",
            "page_size": "2",
            "parser_id": "parser-x",
            "orderby": "create_time",
            "desc": "false",
        },
    )
    _set_request_json(monkeypatch, module, {"owner_ids": []})
    res = _run(handler())
    assert res["code"] == 0
    assert res["data"]["total"] == 1
    assert calls[-1]["tenants"] == []
    assert calls[-1]["desc"] is False

    _set_request_args(monkeypatch, module, {"page": "2", "page_size": "1"})
    _set_request_json(monkeypatch, module, {"owner_ids": ["tenant-a", "tenant-b"]})
    res = _run(handler())
    assert res["code"] == 0
    assert res["data"]["total"] == 2
    assert res["data"]["dialogs"] == [{"id": "dialog-3", "tenant_id": "tenant-b"}]
    assert calls[-1]["page_number"] == 0
    assert calls[-1]["items_per_page"] == 0
    assert calls[-1]["desc"] is True

    def _raise_next(*_args, **_kwargs):
        raise RuntimeError("next boom")

    monkeypatch.setattr(module.DialogService, "get_by_tenant_ids", _raise_next)
    _set_request_args(monkeypatch, module, {"page": "1", "page_size": "1"})
    _set_request_json(monkeypatch, module, {"owner_ids": []})
    res = _run(handler())
    assert "next boom" in res["message"]


@pytest.mark.p2
def test_rm_permission_and_exception_matrix_unit(monkeypatch):
    module = _load_dialog_module(monkeypatch)
    handler = inspect.unwrap(module.rm)

    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-a")])
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    _set_request_json(monkeypatch, module, {"dialog_ids": ["dialog-1"]})
    res = _run(handler())
    assert res["code"] == module.RetCode.OPERATING_ERROR
    assert "Only owner of dialog authorized for this operation." in res["message"]

    def _raise_query(**_kwargs):
        raise RuntimeError("rm boom")

    monkeypatch.setattr(module.DialogService, "query", _raise_query)
    _set_request_json(monkeypatch, module, {"dialog_ids": ["dialog-1"]})
    res = _run(handler())
    assert "rm boom" in res["message"]

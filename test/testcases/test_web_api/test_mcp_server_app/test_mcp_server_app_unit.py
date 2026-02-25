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
from functools import wraps
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _Field:
    def __init__(self, name):
        self.name = name

    def __eq__(self, other):
        return (self.name, other)


class _DummyMCPServer:
    id = _Field("id")
    tenant_id = _Field("tenant_id")

    def __init__(self, **kwargs):
        self.id = kwargs.get("id", "")
        self.name = kwargs.get("name", "")
        self.url = kwargs.get("url", "")
        self.server_type = kwargs.get("server_type", "sse")
        self.tenant_id = kwargs.get("tenant_id", "tenant_1")
        self.variables = kwargs.get("variables", {})
        self.headers = kwargs.get("headers", {})

    def to_dict(self):
        return {
            "id": self.id,
            "name": self.name,
            "url": self.url,
            "server_type": self.server_type,
            "tenant_id": self.tenant_id,
            "variables": self.variables,
            "headers": self.headers,
        }


class _DummyMCPServerService:
    @staticmethod
    def get_servers(*_args, **_kwargs):
        return []

    @staticmethod
    def get_or_none(*_args, **_kwargs):
        return None

    @staticmethod
    def get_by_id(*_args, **_kwargs):
        return False, None

    @staticmethod
    def get_by_name_and_tenant(*_args, **_kwargs):
        return False, None

    @staticmethod
    def insert(**_kwargs):
        return True

    @staticmethod
    def filter_update(*_args, **_kwargs):
        return True

    @staticmethod
    def delete_by_ids(*_args, **_kwargs):
        return True


class _DummyTenantService:
    @staticmethod
    def get_by_id(*_args, **_kwargs):
        return True, SimpleNamespace(id="tenant_1")


class _DummyTool:
    def __init__(self, name):
        self._name = name

    def model_dump(self):
        return {"name": self._name}


class _DummyMCPToolCallSession:
    def __init__(self, _mcp_server, _variables):
        self._tools = [_DummyTool("tool_a"), _DummyTool("tool_b")]

    def get_tools(self, _timeout):
        return self._tools

    def tool_call(self, _name, _arguments, _timeout):
        return "ok"


def _run(coro):
    return asyncio.run(coro)


def _set_request_json(monkeypatch, module, payload):
    async def _request_json():
        return payload

    monkeypatch.setattr(module, "get_request_json", _request_json)


def _load_mcp_server_app(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.current_user = SimpleNamespace(id="tenant_1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    db_models_mod = ModuleType("api.db.db_models")
    db_models_mod.MCPServer = _DummyMCPServer
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    mcp_service_mod = ModuleType("api.db.services.mcp_server_service")
    mcp_service_mod.MCPServerService = _DummyMCPServerService
    monkeypatch.setitem(sys.modules, "api.db.services.mcp_server_service", mcp_service_mod)

    user_service_mod = ModuleType("api.db.services.user_service")
    user_service_mod.TenantService = _DummyTenantService
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)

    mcp_conn_mod = ModuleType("common.mcp_tool_call_conn")
    mcp_conn_mod.MCPToolCallSession = _DummyMCPToolCallSession
    mcp_conn_mod.close_multiple_mcp_toolcall_sessions = lambda _sessions: None
    monkeypatch.setitem(sys.modules, "common.mcp_tool_call_conn", mcp_conn_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")

    async def _default_request_json():
        return {}

    def _get_json_result(code=0, message="success", data=None):
        return {"code": code, "message": message, "data": data}

    def _get_data_error_result(code=102, message="Sorry! Data missing!"):
        return {"code": code, "message": message}

    def _server_error_response(error):
        return {"code": 100, "message": repr(error)}

    async def _get_mcp_tools(*_args, **_kwargs):
        return {}

    def _validate_request(*_args, **_kwargs):
        def _decorator(func):
            @wraps(func)
            async def _wrapped(*func_args, **func_kwargs):
                if inspect.iscoroutinefunction(func):
                    return await func(*func_args, **func_kwargs)
                return func(*func_args, **func_kwargs)

            return _wrapped

        return _decorator

    api_utils_mod.get_request_json = _default_request_json
    api_utils_mod.get_json_result = _get_json_result
    api_utils_mod.get_data_error_result = _get_data_error_result
    api_utils_mod.server_error_response = _server_error_response
    api_utils_mod.validate_request = _validate_request
    api_utils_mod.get_mcp_tools = _get_mcp_tools
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    module_name = "test_mcp_server_app_unit_module"
    module_path = repo_root / "api" / "apps" / "mcp_server_app.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_list_mcp_desc_pagination_and_exception(monkeypatch):
    module = _load_mcp_server_app(monkeypatch)

    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(args={"keywords": "k", "page": "2", "page_size": "1", "orderby": "create_time", "desc": "false"}),
    )
    _set_request_json(monkeypatch, module, {"mcp_ids": []})
    monkeypatch.setattr(module.MCPServerService, "get_servers", lambda *_args, **_kwargs: [{"id": "a"}, {"id": "b"}])

    res = _run(module.list_mcp())
    assert res["code"] == 0
    assert res["data"]["total"] == 2
    assert res["data"]["mcp_servers"] == [{"id": "b"}]

    monkeypatch.setattr(module, "request", SimpleNamespace(args={}))
    _set_request_json(monkeypatch, module, {"mcp_ids": []})

    def _raise_list(*_args, **_kwargs):
        raise RuntimeError("list explode")

    monkeypatch.setattr(module.MCPServerService, "get_servers", _raise_list)
    res = _run(module.list_mcp())
    assert res["code"] == 100
    assert "list explode" in res["message"]


@pytest.mark.p2
def test_detail_not_found_success_and_exception(monkeypatch):
    module = _load_mcp_server_app(monkeypatch)
    monkeypatch.setattr(module, "request", SimpleNamespace(args={"mcp_id": "mcp-1"}))

    monkeypatch.setattr(module.MCPServerService, "get_or_none", lambda **_kwargs: None)
    res = module.detail()
    assert res["code"] == module.RetCode.NOT_FOUND

    monkeypatch.setattr(
        module.MCPServerService,
        "get_or_none",
        lambda **_kwargs: _DummyMCPServer(id="mcp-1", name="srv", url="http://a", server_type="sse", tenant_id="tenant_1"),
    )
    res = module.detail()
    assert res["code"] == 0
    assert res["data"]["id"] == "mcp-1"

    def _raise_detail(**_kwargs):
        raise RuntimeError("detail explode")

    monkeypatch.setattr(module.MCPServerService, "get_or_none", _raise_detail)
    res = module.detail()
    assert res["code"] == 100
    assert "detail explode" in res["message"]


@pytest.mark.p2
def test_create_validation_guards(monkeypatch):
    module = _load_mcp_server_app(monkeypatch)

    monkeypatch.setattr(module.MCPServerService, "get_by_name_and_tenant", lambda **_kwargs: (False, None))

    _set_request_json(monkeypatch, module, {"name": "srv", "url": "http://a", "server_type": "invalid"})
    res = _run(module.create.__wrapped__())
    assert "Unsupported MCP server type" in res["message"]

    _set_request_json(monkeypatch, module, {"name": "", "url": "http://a", "server_type": "sse"})
    res = _run(module.create.__wrapped__())
    assert "Invalid MCP name" in res["message"]

    monkeypatch.setattr(module.MCPServerService, "get_by_name_and_tenant", lambda **_kwargs: (True, object()))
    _set_request_json(monkeypatch, module, {"name": "srv", "url": "http://a", "server_type": "sse"})
    res = _run(module.create.__wrapped__())
    assert "Duplicated MCP server name" in res["message"]

    monkeypatch.setattr(module.MCPServerService, "get_by_name_and_tenant", lambda **_kwargs: (False, None))
    _set_request_json(monkeypatch, module, {"name": "srv", "url": "", "server_type": "sse"})
    res = _run(module.create.__wrapped__())
    assert "Invalid url" in res["message"]


@pytest.mark.p2
def test_create_service_paths(monkeypatch):
    module = _load_mcp_server_app(monkeypatch)

    base_payload = {
        "name": "srv",
        "url": "http://server",
        "server_type": "sse",
        "headers": '{"Authorization": "x"}',
        "variables": '{"tools": {"old": 1}, "token": "abc"}',
        "timeout": "2.5",
    }

    monkeypatch.setattr(module, "get_uuid", lambda: "uuid-create")
    monkeypatch.setattr(module.MCPServerService, "get_by_name_and_tenant", lambda **_kwargs: (False, None))

    _set_request_json(monkeypatch, module, dict(base_payload))
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda *_args, **_kwargs: (False, None))
    res = _run(module.create.__wrapped__())
    assert "Tenant not found" in res["message"]

    _set_request_json(monkeypatch, module, dict(base_payload))
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda *_args, **_kwargs: (True, object()))

    async def _thread_pool_tools_error(_func, _servers, _timeout):
        return None, "tools error"

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_tools_error)
    res = _run(module.create.__wrapped__())
    assert res["code"] == "tools error"
    assert "Sorry! Data missing!" in res["message"]

    _set_request_json(monkeypatch, module, dict(base_payload))

    async def _thread_pool_ok(_func, servers, _timeout):
        return {servers[0].name: [{"name": "tool_a"}, {"invalid": True}]}, None

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_ok)
    monkeypatch.setattr(module.MCPServerService, "insert", lambda **_kwargs: False)
    res = _run(module.create.__wrapped__())
    assert res["code"] == "Failed to create MCP server."
    assert "Sorry! Data missing!" in res["message"]

    _set_request_json(monkeypatch, module, dict(base_payload))
    monkeypatch.setattr(module.MCPServerService, "insert", lambda **_kwargs: True)
    res = _run(module.create.__wrapped__())
    assert res["code"] == 0
    assert res["data"]["id"] == "uuid-create"
    assert res["data"]["tenant_id"] == "tenant_1"
    assert res["data"]["variables"]["tools"] == {"tool_a": {"name": "tool_a"}}

    _set_request_json(monkeypatch, module, dict(base_payload))

    async def _thread_pool_raises(_func, _servers, _timeout):
        raise RuntimeError("create explode")

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_raises)
    res = _run(module.create.__wrapped__())
    assert res["code"] == 100
    assert "create explode" in res["message"]


@pytest.mark.p2
def test_update_validation_guards(monkeypatch):
    module = _load_mcp_server_app(monkeypatch)

    existing = _DummyMCPServer(id="mcp-1", name="srv", url="http://server", server_type="sse", tenant_id="tenant_1", variables={}, headers={})

    _set_request_json(monkeypatch, module, {"mcp_id": "mcp-1"})
    monkeypatch.setattr(module.MCPServerService, "get_by_id", lambda _mcp_id: (False, None))
    res = _run(module.update.__wrapped__())
    assert "Cannot find MCP server" in res["message"]

    _set_request_json(monkeypatch, module, {"mcp_id": "mcp-1"})
    monkeypatch.setattr(
        module.MCPServerService,
        "get_by_id",
        lambda _mcp_id: (True, _DummyMCPServer(id="mcp-1", name="srv", url="http://server", server_type="sse", tenant_id="other", variables={}, headers={})),
    )
    res = _run(module.update.__wrapped__())
    assert "Cannot find MCP server" in res["message"]

    _set_request_json(monkeypatch, module, {"mcp_id": "mcp-1", "server_type": "invalid"})
    monkeypatch.setattr(module.MCPServerService, "get_by_id", lambda _mcp_id: (True, existing))
    res = _run(module.update.__wrapped__())
    assert "Unsupported MCP server type" in res["message"]

    _set_request_json(monkeypatch, module, {"mcp_id": "mcp-1", "name": "a" * 256})
    res = _run(module.update.__wrapped__())
    assert "Invalid MCP name" in res["message"]

    _set_request_json(monkeypatch, module, {"mcp_id": "mcp-1", "url": ""})
    res = _run(module.update.__wrapped__())
    assert "Invalid url" in res["message"]


@pytest.mark.p2
def test_update_service_paths(monkeypatch):
    module = _load_mcp_server_app(monkeypatch)

    existing = _DummyMCPServer(
        id="mcp-1",
        name="srv",
        url="http://server",
        server_type="sse",
        tenant_id="tenant_1",
        variables={"tools": {"old": {"enabled": True}}, "token": "abc"},
        headers={"Authorization": "old"},
    )
    updated = _DummyMCPServer(
        id="mcp-1",
        name="srv-new",
        url="http://server-new",
        server_type="sse",
        tenant_id="tenant_1",
        variables={"tools": {"tool_a": {"name": "tool_a"}}},
        headers={"Authorization": "new"},
    )

    base_payload = {
        "mcp_id": "mcp-1",
        "name": "srv-new",
        "url": "http://server-new",
        "server_type": "sse",
        "headers": '{"Authorization": "new"}',
        "variables": '{"tools": {"ignore": 1}, "token": "new"}',
        "timeout": "3.0",
    }

    _set_request_json(monkeypatch, module, dict(base_payload))
    monkeypatch.setattr(module.MCPServerService, "get_by_id", lambda _mcp_id: (True, existing))

    async def _thread_pool_tools_error(_func, _servers, _timeout):
        return None, "update tools error"

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_tools_error)
    res = _run(module.update.__wrapped__())
    assert res["code"] == "update tools error"
    assert "Sorry! Data missing!" in res["message"]

    _set_request_json(monkeypatch, module, dict(base_payload))

    async def _thread_pool_ok(_func, servers, _timeout):
        return {servers[0].name: [{"name": "tool_a"}, {"bad": True}]}, None

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_ok)
    monkeypatch.setattr(module.MCPServerService, "filter_update", lambda *_args, **_kwargs: False)
    res = _run(module.update.__wrapped__())
    assert "Failed to updated MCP server" in res["message"]

    _set_request_json(monkeypatch, module, dict(base_payload))
    monkeypatch.setattr(module.MCPServerService, "filter_update", lambda *_args, **_kwargs: True)

    def _get_by_id_fetch_fail(_mcp_id):
        if _get_by_id_fetch_fail.calls == 0:
            _get_by_id_fetch_fail.calls += 1
            return True, existing
        return False, None

    _get_by_id_fetch_fail.calls = 0
    monkeypatch.setattr(module.MCPServerService, "get_by_id", _get_by_id_fetch_fail)
    res = _run(module.update.__wrapped__())
    assert "Failed to fetch updated MCP server" in res["message"]

    _set_request_json(monkeypatch, module, dict(base_payload))

    def _get_by_id_success(_mcp_id):
        if _get_by_id_success.calls == 0:
            _get_by_id_success.calls += 1
            return True, existing
        return True, updated

    _get_by_id_success.calls = 0
    monkeypatch.setattr(module.MCPServerService, "get_by_id", _get_by_id_success)
    res = _run(module.update.__wrapped__())
    assert res["code"] == 0
    assert res["data"]["id"] == "mcp-1"

    _set_request_json(monkeypatch, module, dict(base_payload))
    monkeypatch.setattr(module.MCPServerService, "get_by_id", lambda _mcp_id: (True, existing))

    async def _thread_pool_raises(_func, _servers, _timeout):
        raise RuntimeError("update explode")

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_raises)
    res = _run(module.update.__wrapped__())
    assert res["code"] == 100
    assert "update explode" in res["message"]


@pytest.mark.p2
def test_rm_failure_success_and_exception(monkeypatch):
    module = _load_mcp_server_app(monkeypatch)

    _set_request_json(monkeypatch, module, {"mcp_ids": ["a", "b"]})
    monkeypatch.setattr(module.MCPServerService, "delete_by_ids", lambda _ids: False)
    res = _run(module.rm.__wrapped__())
    assert "Failed to delete MCP servers" in res["message"]

    _set_request_json(monkeypatch, module, {"mcp_ids": ["a", "b"]})
    monkeypatch.setattr(module.MCPServerService, "delete_by_ids", lambda _ids: True)
    res = _run(module.rm.__wrapped__())
    assert res["code"] == 0
    assert res["data"] is True

    _set_request_json(monkeypatch, module, {"mcp_ids": ["a", "b"]})

    def _raise_rm(_ids):
        raise RuntimeError("rm explode")

    monkeypatch.setattr(module.MCPServerService, "delete_by_ids", _raise_rm)
    res = _run(module.rm.__wrapped__())
    assert res["code"] == 100
    assert "rm explode" in res["message"]


@pytest.mark.p2
def test_import_multiple_missing_servers_and_exception(monkeypatch):
    module = _load_mcp_server_app(monkeypatch)

    _set_request_json(monkeypatch, module, {"mcpServers": {}})
    res = _run(module.import_multiple.__wrapped__())
    assert "No MCP servers provided" in res["message"]

    _set_request_json(monkeypatch, module, {"mcpServers": {"srv": {"type": "sse", "url": "http://x"}}, "timeout": "1"})

    def _raise_import(**_kwargs):
        raise RuntimeError("import explode")

    monkeypatch.setattr(module.MCPServerService, "get_by_name_and_tenant", _raise_import)
    res = _run(module.import_multiple.__wrapped__())
    assert res["code"] == 100
    assert "import explode" in res["message"]


@pytest.mark.p2
def test_import_multiple_mixed_results(monkeypatch):
    module = _load_mcp_server_app(monkeypatch)

    payload = {
        "mcpServers": {
            "missing_fields": {"type": "sse"},
            "": {"type": "sse", "url": "http://empty"},
            "dup": {"type": "sse", "url": "http://dup", "authorization_token": "dup-token"},
            "tool_err": {"type": "sse", "url": "http://err"},
            "insert_fail": {"type": "sse", "url": "http://fail"},
        },
        "timeout": "3",
    }
    _set_request_json(monkeypatch, module, payload)

    monkeypatch.setattr(module, "get_uuid", lambda: "uuid-import")

    def _get_by_name_and_tenant(name, tenant_id):
        if name == "dup" and not _get_by_name_and_tenant.first_dup_seen:
            _get_by_name_and_tenant.first_dup_seen = True
            return True, object()
        return False, None

    _get_by_name_and_tenant.first_dup_seen = False
    monkeypatch.setattr(module.MCPServerService, "get_by_name_and_tenant", _get_by_name_and_tenant)

    async def _thread_pool_exec(func, servers, _timeout):
        mcp_server = servers[0]
        if mcp_server.name == "tool_err":
            return None, "tool call failed"
        return {mcp_server.name: [{"name": "tool_a"}, {"invalid": True}]}, None

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_exec)

    def _insert(**kwargs):
        return kwargs["name"] != "insert_fail"

    monkeypatch.setattr(module.MCPServerService, "insert", _insert)

    res = _run(module.import_multiple.__wrapped__())
    assert res["code"] == 0

    results = {item["server"]: item for item in res["data"]["results"]}
    assert results["missing_fields"]["success"] is False
    assert "Missing required fields" in results["missing_fields"]["message"]
    assert results[""]["success"] is False
    assert "Invalid MCP name" in results[""]["message"]
    assert results["tool_err"]["success"] is False
    assert "tool call failed" in results["tool_err"]["message"]
    assert results["insert_fail"]["success"] is False
    assert "Failed to create MCP server" in results["insert_fail"]["message"]
    assert results["dup"]["success"] is True
    assert results["dup"]["new_name"] == "dup_0"
    assert "Renamed from 'dup' to 'dup_0' avoid duplication" == results["dup"]["message"]


@pytest.mark.p2
def test_export_multiple_missing_ids_success_and_exception(monkeypatch):
    module = _load_mcp_server_app(monkeypatch)

    _set_request_json(monkeypatch, module, {"mcp_ids": []})
    res = _run(module.export_multiple.__wrapped__())
    assert "No MCP server IDs provided" in res["message"]

    _set_request_json(monkeypatch, module, {"mcp_ids": ["id1", "id2", "id3"]})

    def _get_by_id(mcp_id):
        if mcp_id == "id1":
            return True, _DummyMCPServer(
                id="id1",
                name="srv-one",
                url="http://one",
                server_type="sse",
                tenant_id="tenant_1",
                variables={"authorization_token": "tok", "tools": {"tool_a": {"enabled": True}}},
            )
        if mcp_id == "id2":
            return True, _DummyMCPServer(
                id="id2",
                name="srv-two",
                url="http://two",
                server_type="sse",
                tenant_id="other",
                variables={},
            )
        return False, None

    monkeypatch.setattr(module.MCPServerService, "get_by_id", _get_by_id)
    res = _run(module.export_multiple.__wrapped__())
    assert res["code"] == 0
    assert list(res["data"]["mcpServers"].keys()) == ["srv-one"]

    _set_request_json(monkeypatch, module, {"mcp_ids": ["id1"]})

    def _raise_export(_mcp_id):
        raise RuntimeError("export explode")

    monkeypatch.setattr(module.MCPServerService, "get_by_id", _raise_export)
    res = _run(module.export_multiple.__wrapped__())
    assert res["code"] == 100
    assert "export explode" in res["message"]


@pytest.mark.p2
def test_list_tools_missing_ids_success_inner_error_outer_error_and_finally_cleanup(monkeypatch):
    module = _load_mcp_server_app(monkeypatch)

    _set_request_json(monkeypatch, module, {"mcp_ids": []})
    res = _run(module.list_tools.__wrapped__())
    assert "No MCP server IDs provided" in res["message"]

    server = _DummyMCPServer(
        id="id1",
        name="srv-tools",
        url="http://tools",
        server_type="sse",
        tenant_id="tenant_1",
        variables={"tools": {"tool_a": {"enabled": False}}},
    )

    _set_request_json(monkeypatch, module, {"mcp_ids": ["id1"], "timeout": "2.0"})
    monkeypatch.setattr(module.MCPServerService, "get_by_id", lambda _mcp_id: (True, server))

    close_calls = []

    async def _thread_pool_exec_success(func, *args):
        if func is module.close_multiple_mcp_toolcall_sessions:
            close_calls.append(args[0])
            return None
        return func(*args)

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_exec_success)
    res = _run(module.list_tools.__wrapped__())
    assert res["code"] == 0
    assert res["data"]["id1"][0]["name"] == "tool_a"
    assert res["data"]["id1"][0]["enabled"] is False
    assert res["data"]["id1"][1]["enabled"] is True
    assert close_calls and len(close_calls[-1]) == 1

    _set_request_json(monkeypatch, module, {"mcp_ids": ["id1"], "timeout": "2.0"})
    close_calls_inner = []

    async def _thread_pool_exec_inner_error(func, *args):
        if func is module.close_multiple_mcp_toolcall_sessions:
            close_calls_inner.append(args[0])
            return None
        raise RuntimeError("inner tools explode")

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_exec_inner_error)
    res = _run(module.list_tools.__wrapped__())
    assert res["code"] == 102
    assert "MCP list tools error" in res["message"]
    assert close_calls_inner and len(close_calls_inner[-1]) == 1

    _set_request_json(monkeypatch, module, {"mcp_ids": ["id1"], "timeout": "2.0"})
    close_calls_outer = []

    def _raise_get_by_id(_mcp_id):
        raise RuntimeError("outer explode")

    monkeypatch.setattr(module.MCPServerService, "get_by_id", _raise_get_by_id)

    async def _thread_pool_exec_outer(func, *args):
        if func is module.close_multiple_mcp_toolcall_sessions:
            close_calls_outer.append(args[0])
            return None
        return func(*args)

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_exec_outer)
    res = _run(module.list_tools.__wrapped__())
    assert res["code"] == 100
    assert "outer explode" in res["message"]
    assert close_calls_outer


@pytest.mark.p2
def test_test_tool_missing_mcp_id(monkeypatch):
    module = _load_mcp_server_app(monkeypatch)

    _set_request_json(monkeypatch, module, {"mcp_id": "", "tool_name": "tool_a", "arguments": {"x": 1}})
    res = _run(module.test_tool.__wrapped__())
    assert "No MCP server ID provided" in res["message"]

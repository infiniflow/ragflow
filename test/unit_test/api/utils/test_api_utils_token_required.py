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
from enum import Enum
from pathlib import Path
from types import ModuleType, SimpleNamespace

from quart import Quart


def _run(coro):
    return asyncio.run(coro)


def _load_api_utils_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    class _RetCode(int, Enum):
        SUCCESS = 0
        EXCEPTION_ERROR = 100
        ARGUMENT_ERROR = 101
        DATA_ERROR = 102
        OPERATING_ERROR = 103
        PERMISSION_ERROR = 108
        AUTHENTICATION_ERROR = 109
        FORBIDDEN = 403
        UNAUTHORIZED = 401

    class _ActiveEnum(str, Enum):
        ACTIVE = "1"
        INACTIVE = "0"

    class _StatusEnum(str, Enum):
        VALID = "1"
        INVALID = "0"

    common_constants_mod = ModuleType("common.constants")
    common_constants_mod.RetCode = _RetCode
    common_constants_mod.ActiveEnum = _ActiveEnum
    common_constants_mod.StatusEnum = _StatusEnum
    monkeypatch.setitem(sys.modules, "common.constants", common_constants_mod)

    common_settings_mod = ModuleType("common.settings")
    common_settings_mod.get_secret_key = lambda: "test-secret"
    monkeypatch.setitem(sys.modules, "common.settings", common_settings_mod)
    common_pkg.settings = common_settings_mod

    common_misc_utils_mod = ModuleType("common.misc_utils")
    common_misc_utils_mod.thread_pool_exec = lambda func, *args, **kwargs: func(*args, **kwargs)
    monkeypatch.setitem(sys.modules, "common.misc_utils", common_misc_utils_mod)

    common_connection_utils_mod = ModuleType("common.connection_utils")
    common_connection_utils_mod.timeout = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "common.connection_utils", common_connection_utils_mod)

    common_mcp_tool_call_conn_mod = ModuleType("common.mcp_tool_call_conn")
    common_mcp_tool_call_conn_mod.MCPToolCallSession = object
    common_mcp_tool_call_conn_mod.close_multiple_mcp_toolcall_sessions = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "common.mcp_tool_call_conn", common_mcp_tool_call_conn_mod)

    api_db_models_mod = ModuleType("api.db.db_models")

    class _APIToken:
        @staticmethod
        def query(**_kwargs):
            return []

    api_db_models_mod.APIToken = _APIToken
    monkeypatch.setitem(sys.modules, "api.db.db_models", api_db_models_mod)

    tenant_llm_service_mod = ModuleType("api.db.services.tenant_llm_service")
    tenant_llm_service_mod.LLMFactoriesService = object
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_llm_service", tenant_llm_service_mod)

    user_service_mod = ModuleType("api.db.services.user_service")
    user_service_mod.UserService = SimpleNamespace(query=lambda **_kwargs: [])
    user_service_mod.UserTenantService = SimpleNamespace(query=lambda **_kwargs: [])
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)

    json_encode_mod = ModuleType("api.utils.json_encode")
    json_encode_mod.CustomJSONEncoder = json.JSONEncoder
    monkeypatch.setitem(sys.modules, "api.utils.json_encode", json_encode_mod)

    module_name = "test_api_utils_token_required_module"
    module_path = repo_root / "api" / "utils" / "api_utils.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def test_token_required_injects_authenticated_user_id_for_login_tokens(monkeypatch):
    module = _load_api_utils_module(monkeypatch)
    app = Quart(__name__)

    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [])

    user_service_mod = sys.modules["api.db.services.user_service"]
    user_service_mod.UserService = SimpleNamespace(
        query=lambda **kwargs: [SimpleNamespace(id="user-2")] if kwargs.get("access_token") == "raw-login-token" else [],
    )
    user_service_mod.UserTenantService = SimpleNamespace(
        query=lambda **kwargs: [SimpleNamespace(tenant_id="tenant-1")] if kwargs.get("user_id") == "user-2" else [],
    )

    from itsdangerous.url_safe import URLSafeTimedSerializer

    monkeypatch.setattr(URLSafeTimedSerializer, "loads", lambda self, _token: "raw-login-token")

    @module.token_required
    async def _handler(tenant_id=None, authenticated_user_id=None):
        return {
            "tenant_id": tenant_id,
            "authenticated_user_id": authenticated_user_id,
        }

    async def _case():
        async with app.test_request_context("/", headers={"Authorization": "Bearer login-token"}):
            return await _handler()

    assert _run(_case()) == {
        "tenant_id": "tenant-1",
        "authenticated_user_id": "user-2",
    }

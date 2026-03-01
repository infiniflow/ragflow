#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import importlib.util
import sys
from pathlib import Path
from types import ModuleType

import pytest
from common import plugin_llm_tools
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


INVALID_AUTH_CASES = [
    (None, 401, "<Unauthorized '401: Unauthorized'>"),
    (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
]


class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_message", INVALID_AUTH_CASES)
    def test_auth_invalid(self, invalid_auth, expected_code, expected_message):
        res = plugin_llm_tools(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestPluginTools:
    @pytest.mark.p1
    def test_llm_tools(self, WebApiAuth):
        res = plugin_llm_tools(WebApiAuth)
        assert res["code"] == 0, res
        assert isinstance(res["data"], list), res


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func
        return decorator


def _load_plugin_app(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]
    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    stub_apps = ModuleType("api.apps")
    stub_apps.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", stub_apps)

    stub_plugin = ModuleType("agent.plugin")

    class _StubGlobalPluginManager:
        @staticmethod
        def get_llm_tools():
            return []

    stub_plugin.GlobalPluginManager = _StubGlobalPluginManager
    monkeypatch.setitem(sys.modules, "agent.plugin", stub_plugin)

    module_path = Path(__file__).resolve().parents[4] / "api" / "apps" / "plugin_app.py"
    spec = importlib.util.spec_from_file_location("test_plugin_app_unit", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_llm_tools_metadata_shape_unit(monkeypatch):
    module = _load_plugin_app(monkeypatch)

    class _DummyTool:
        def get_metadata(self):
            return {"name": "dummy", "description": "test"}

    monkeypatch.setattr(module.GlobalPluginManager, "get_llm_tools", staticmethod(lambda: [_DummyTool()]))
    res = module.llm_tools()
    assert res["code"] == 0
    assert isinstance(res["data"], list)
    assert res["data"][0]["name"] == "dummy"
    assert res["data"][0]["description"] == "test"

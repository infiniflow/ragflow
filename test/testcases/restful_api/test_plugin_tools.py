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

import importlib.util
import sys
from pathlib import Path
from types import ModuleType

import pytest


@pytest.mark.p2
def test_plugin_tools_requires_auth(rest_client_noauth):
    res = rest_client_noauth.get("/plugin/tools")
    assert res.status_code == 401
    payload = res.json()
    assert payload["code"] == 401, payload


@pytest.mark.p2
def test_plugin_tools_contract(rest_client):
    res = rest_client.get("/plugin/tools")
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert isinstance(payload["data"], list), payload


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


def _load_plugin_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[3]

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

    module_path = repo_root / "api" / "apps" / "restful_apis" / "plugin_api.py"
    spec = importlib.util.spec_from_file_location("restful_plugin_api_unit", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_plugin_tools_metadata_shape_unit(monkeypatch):
    module = _load_plugin_module(monkeypatch)

    class _DummyTool:
        def get_metadata(self):
            return {"name": "dummy", "description": "test"}

    monkeypatch.setattr(module.GlobalPluginManager, "get_llm_tools", staticmethod(lambda: [_DummyTool()]))
    res = module.llm_tools()
    assert res["code"] == 0
    assert isinstance(res["data"], list)
    assert res["data"][0]["name"] == "dummy"
    assert res["data"][0]["description"] == "test"

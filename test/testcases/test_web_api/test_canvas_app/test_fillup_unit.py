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
from types import ModuleType, SimpleNamespace

import pytest


def _load_fillup_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    agent_pkg = ModuleType("agent")
    agent_pkg.__path__ = [str(repo_root / "agent")]
    monkeypatch.setitem(sys.modules, "agent", agent_pkg)

    component_pkg = ModuleType("agent.component")
    component_pkg.__path__ = [str(repo_root / "agent" / "component")]
    monkeypatch.setitem(sys.modules, "agent.component", component_pkg)

    base_mod = ModuleType("agent.component.base")

    class _ComponentParamBase:
        def __init__(self):
            self.inputs = {}
            self.outputs = {}

    class _ComponentBase:
        def get_input_elements(self):
            return self._param.inputs

        def check_if_canceled(self, *_args, **_kwargs):
            return False

        def get_input_elements_from_text(self, *_args, **_kwargs):
            return {}

        def set_output(self, key, value):
            if key not in self._param.outputs:
                self._param.outputs[key] = {"value": None}
            self._param.outputs[key]["value"] = value

        def set_input_value(self, key, value):
            if key not in self._param.inputs:
                self._param.inputs[key] = {"value": None}
            self._param.inputs[key]["value"] = value

    base_mod.ComponentBase = _ComponentBase
    base_mod.ComponentParamBase = _ComponentParamBase
    monkeypatch.setitem(sys.modules, "agent.component.base", base_mod)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = [str(repo_root / "api" / "db" / "services")]
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    file_service_mod = ModuleType("api.db.services.file_service")

    class _FileService:
        @staticmethod
        def get_files(files, layout_recognize=None):
            return {"files": files, "layout_recognize": layout_recognize}

    file_service_mod.FileService = _FileService
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service_mod)

    module_path = repo_root / "agent" / "component" / "fillup.py"
    spec = importlib.util.spec_from_file_location("test_fillup_unit_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_fillup_unit_module", module)
    spec.loader.exec_module(module)
    return module


def _make_fillup(module, *, query, inputs):
    component = module.UserFillUp.__new__(module.UserFillUp)
    component._canvas = SimpleNamespace(
        globals={
            "sys.query": query,
            "sys.__initial_user_input_consumed__": False,
        }
    )
    component._param = SimpleNamespace(
        enable_tips=False,
        tips="",
        layout_recognize="",
        inputs=inputs,
        outputs={},
    )
    return component


@pytest.mark.p2
def test_user_fillup_auto_consumes_initial_query_for_single_field(monkeypatch):
    module = _load_fillup_module(monkeypatch)
    component = _make_fillup(
        module,
        query="code",
        inputs={"demo": {"type": "options", "name": "Demo"}},
    )

    component._invoke(inputs={})

    assert component._param.inputs["demo"]["value"] == "code"
    assert component._param.outputs["demo"]["value"] == "code"
    assert component._canvas.globals["sys.__initial_user_input_consumed__"] is True


@pytest.mark.p2
def test_user_fillup_only_auto_consumes_initial_query_once(monkeypatch):
    module = _load_fillup_module(monkeypatch)
    component = _make_fillup(
        module,
        query="code",
        inputs={"demo": {"type": "options", "name": "Demo"}},
    )

    component._invoke(inputs={})
    component._param.outputs = {}
    component._invoke(inputs={})

    assert component._param.outputs == {}


@pytest.mark.p2
def test_user_fillup_does_not_consume_unmatched_structured_query(monkeypatch):
    module = _load_fillup_module(monkeypatch)
    component = _make_fillup(
        module,
        query={"x": 8},
        inputs={"demo": {"type": "options", "name": "Demo"}},
    )

    component._invoke(inputs={})

    assert component._param.outputs == {}
    assert component._canvas.globals["sys.__initial_user_input_consumed__"] is False

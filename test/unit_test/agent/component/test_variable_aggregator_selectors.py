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
"""Regression test for selector handling in `agent/component/variable_aggregator.py`.

`VariableAggregatorParam.param_refs` accepts variable selectors either
as `{"value": ref}` dicts (the UI form) or as plain reference strings
(SDK/API-built canvases), but `_invoke` indexed `selector["value"]`
unconditionally and crashed with `TypeError: string indices must be
integers` on the string form. `_invoke` now normalizes both forms the
same way `param_refs` does.
"""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import MagicMock

import pytest


def _load_variable_aggregator_module(monkeypatch):
    """Load `agent.component.variable_aggregator` with heavy deps stubbed."""
    repo_root = Path(__file__).resolve().parents[4]

    quart_stub = ModuleType("quart")
    quart_stub.make_response = MagicMock()
    quart_stub.jsonify = MagicMock()
    monkeypatch.setitem(sys.modules, "quart", quart_stub)

    def _timeout(*args, **kwargs):
        def deco(fn):
            return fn

        return deco

    for name in ("api", "api.utils"):
        pkg = ModuleType(name)
        pkg.__path__ = []
        monkeypatch.setitem(sys.modules, name, pkg)
    api_utils_mod = ModuleType("api.utils.api_utils")
    api_utils_mod.timeout = _timeout
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    constants_mod = ModuleType("common.constants")

    class _RetCode:
        SUCCESS = 0
        EXCEPTION_ERROR = 100

    constants_mod.RetCode = _RetCode
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)
    for name in ("connection_utils", "misc_utils"):
        spec = importlib.util.spec_from_file_location(f"common.{name}", repo_root / "common" / f"{name}.py")
        mod = importlib.util.module_from_spec(spec)
        monkeypatch.setitem(sys.modules, f"common.{name}", mod)
        spec.loader.exec_module(mod)

    agent_pkg = ModuleType("agent")
    agent_pkg.__path__ = [str(repo_root / "agent")]
    monkeypatch.setitem(sys.modules, "agent", agent_pkg)
    component_pkg = ModuleType("agent.component")
    component_pkg.__path__ = [str(repo_root / "agent" / "component")]
    monkeypatch.setitem(sys.modules, "agent.component", component_pkg)

    base_spec = importlib.util.spec_from_file_location("agent.component.base", repo_root / "agent" / "component" / "base.py")
    base_mod = importlib.util.module_from_spec(base_spec)
    monkeypatch.setitem(sys.modules, "agent.component.base", base_mod)
    base_spec.loader.exec_module(base_mod)

    spec = importlib.util.spec_from_file_location(
        "agent.component.variable_aggregator", repo_root / "agent" / "component" / "variable_aggregator.py"
    )
    mod = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "agent.component.variable_aggregator", mod)
    spec.loader.exec_module(mod)
    return mod


class _Canvas:
    def __init__(self, variables=None):
        self.variables = variables or {}

    def is_canceled(self):
        return False

    def get_variable_value(self, key):
        return self.variables.get(key)


def _component(mod, canvas, groups):
    cpn = mod.VariableAggregator.__new__(mod.VariableAggregator)
    cpn._canvas = canvas
    cpn._id = "aggregator"
    param = mod.VariableAggregatorParam()
    param.groups = groups
    cpn._param = param
    return cpn


def test_string_selectors_resolve(monkeypatch):
    mod = _load_variable_aggregator_module(monkeypatch)
    cpn = _component(mod, _Canvas({"a@x": "hello"}), [{"group_name": "G", "variables": ["a@x"]}])

    cpn._invoke()

    assert cpn.output("G") == "hello"


def test_dict_selectors_still_resolve(monkeypatch):
    mod = _load_variable_aggregator_module(monkeypatch)
    cpn = _component(mod, _Canvas({"a@x": "hello"}), [{"group_name": "G", "variables": [{"value": "a@x"}]}])

    cpn._invoke()

    assert cpn.output("G") == "hello"


def test_mixed_selectors_pick_first_available(monkeypatch):
    mod = _load_variable_aggregator_module(monkeypatch)
    cpn = _component(
        mod,
        _Canvas({"b@y": "fallback"}),
        [{"group_name": "G", "variables": [{"value": "a@x"}, "b@y"]}],
    )

    cpn._invoke()

    assert cpn.output("G") == "fallback"


def test_whitespace_only_selector_is_skipped(monkeypatch):
    mod = _load_variable_aggregator_module(monkeypatch)
    cpn = _component(mod, _Canvas({"b@y": "fallback"}), [{"group_name": "G", "variables": [" ", "b@y"]}])

    cpn._invoke()

    assert cpn.output("G") == "fallback"

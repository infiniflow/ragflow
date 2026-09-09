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
"""Regression test for numeric comparisons in `agent/component/switch.py`.

When the switched variable is a number, `_invoke` converted the
comparison value with `float(operatee)` unconditionally. An empty or
non-numeric value (easy to produce from the UI, which only requires
the branch target) crashed the whole canvas run with `ValueError`.
The mixed-type fallback inside `process_operator` (`input > value`
with a number and a string) raised `TypeError` the same way.

The fix keeps the raw comparison value when it is not parseable as a
float and falls back to a lexicographic comparison for mixed types, so
a bad comparison value is a non-match instead of a crash.
"""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import MagicMock

import pytest


def _load_switch_module(monkeypatch):
    """Load `agent.component.switch` with heavy deps stubbed."""
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

    spec = importlib.util.spec_from_file_location("agent.component.switch", repo_root / "agent" / "component" / "switch.py")
    mod = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "agent.component.switch", mod)
    spec.loader.exec_module(mod)
    return mod


class _Canvas:
    def __init__(self, variables=None):
        self.variables = variables or {}

    def is_canceled(self):
        return False

    def get_variable_value(self, key):
        return self.variables[key]

    def get_component_name(self, key):
        return key


def _switch(mod, variables, items, end=("else_target",)):
    param = mod.SwitchParam()
    param.conditions = [{"logical_operator": "and", "items": items, "to": ["case_target"]}]
    param.end_cpn_ids = list(end)
    cpn = mod.Switch.__new__(mod.Switch)
    cpn._canvas = _Canvas(variables)
    cpn._id = "switch"
    cpn._param = param
    return cpn


def test_numeric_variable_with_empty_value_falls_through(monkeypatch):
    mod = _load_switch_module(monkeypatch)
    cpn = _switch(mod, {"score": 5}, [{"cpn_id": "score", "operator": "=", "value": ""}])

    cpn._invoke()

    assert cpn.output("_next") == ["else_target"]


def test_numeric_variable_with_non_numeric_value_does_not_crash(monkeypatch):
    mod = _load_switch_module(monkeypatch)
    cpn = _switch(mod, {"score": 5}, [{"cpn_id": "score", "operator": ">", "value": "abc"}])

    cpn._invoke()

    assert cpn.output("_next") == ["else_target"]


def test_numeric_comparison_still_matches(monkeypatch):
    mod = _load_switch_module(monkeypatch)
    cpn = _switch(mod, {"score": 5}, [{"cpn_id": "score", "operator": ">", "value": "3"}])

    cpn._invoke()

    assert cpn.output("_next") == ["case_target"]


def test_string_comparison_still_matches(monkeypatch):
    mod = _load_switch_module(monkeypatch)
    cpn = _switch(mod, {"answer": "yes"}, [{"cpn_id": "answer", "operator": "=", "value": "yes"}])

    cpn._invoke()

    assert cpn.output("_next") == ["case_target"]

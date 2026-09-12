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
"""Regression test for variable resolution in `agent/component/data_operations.py`.

`filter_values` and `append_or_update` resolved `{component@variable}`
templates with

    self._canvas.get_value_with_variable(raw) or raw

When the referenced variable legitimately resolves to an empty/falsy
value (an empty string, for example), the `or raw` fallback silently
kept the UNRESOLVED template text instead:

- `filter_values` compared rows against the literal string
  "{comp@out}" instead of "", so rows that should match were dropped.
- `append_or_update` wrote the literal "{comp@out}" text into the
  output objects.

Additionally, `append_or_update` passed non-string constant values
(numbers, booleans) straight into `get_value_with_variable`, which
runs a regex over the input and raised TypeError.
"""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import MagicMock

import pytest


def _load_data_operations_module(monkeypatch):
    """Load `agent.component.data_operations` with heavy deps stubbed.

    Same isolation strategy as `test_iterationitem_at_split.py`: stub
    `quart` and `api.utils.api_utils`, run the real `common.*` and
    `agent.component.base` modules so the template-substitution logic
    under test is the production code.
    """
    repo_root = Path(__file__).resolve().parents[4]

    quart_stub = ModuleType("quart")
    quart_stub.make_response = MagicMock()
    quart_stub.jsonify = MagicMock()
    monkeypatch.setitem(sys.modules, "quart", quart_stub)

    # Stub `api.utils.api_utils` (imports quart/flask); data_operations
    # only needs a pass-through `timeout` decorator from it.
    def _timeout(*args, **kwargs):
        def deco(fn):
            return fn

        return deco

    api_pkg = ModuleType("api")
    api_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api", api_pkg)
    api_utils_pkg = ModuleType("api.utils")
    api_utils_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.utils", api_utils_pkg)
    api_utils_mod = ModuleType("api.utils.api_utils")
    api_utils_mod.timeout = _timeout
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    constants_mod = ModuleType("common.constants")

    class _RetCode:
        SUCCESS = 0
        EXCEPTION_ERROR = 100

    constants_mod.RetCode = _RetCode
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

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
        "agent.component.data_operations", repo_root / "agent" / "component" / "data_operations.py"
    )
    mod = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "agent.component.data_operations", mod)
    spec.loader.exec_module(mod)
    return mod, base_mod


class _Canvas:
    """Minimal canvas double: resolves `{ref}` templates from a dict."""

    def __init__(self, variables=None):
        self.variables = variables or {}

    def is_canceled(self):
        return False

    def get_variable_value(self, key):
        return self.variables.get(key)

    def get_value_with_variable(self, value):
        base_mod = sys.modules["agent.component.base"]

        def rep(match):
            val = self.variables.get(match.group(1))
            return "" if val is None else str(val)

        return base_mod.ComponentBase._replace_template_matches(base_mod.ComponentBase.variable_ref_patt_re, value, rep)


def _component(mod, canvas):
    cpn = mod.DataOperations.__new__(mod.DataOperations)
    cpn._canvas = canvas
    cpn._id = "data-operations"
    cpn._param = mod.DataOperationsParam()
    return cpn


def test_filter_values_matches_empty_string_variable(monkeypatch):
    mod, _ = _load_data_operations_module(monkeypatch)
    cpn = _component(mod, _Canvas({"comp@out": ""}))

    rule = {"key": "status", "operator": "=", "value": "{comp@out}"}
    assert cpn.match_rule({"status": ""}, rule) is True
    assert cpn.match_rule({"status": "active"}, rule) is False


def test_append_or_update_writes_empty_string_not_template(monkeypatch):
    mod, _ = _load_data_operations_module(monkeypatch)
    cpn = _component(mod, _Canvas({"comp@out": ""}))
    cpn._param.updates = [{"key": "note", "value": "{comp@out}"}]
    cpn.input_objects = [{"a": 1}]

    cpn._append_or_update()

    assert cpn.output("result") == [{"a": 1, "note": ""}]


def test_append_or_update_keeps_nonempty_resolution(monkeypatch):
    mod, _ = _load_data_operations_module(monkeypatch)
    cpn = _component(mod, _Canvas({"comp@out": "hello"}))
    cpn._param.updates = [{"key": "note", "value": "say {comp@out}"}]
    cpn.input_objects = [{"a": 1}]

    cpn._append_or_update()

    assert cpn.output("result") == [{"a": 1, "note": "say hello"}]


def test_append_or_update_accepts_non_string_constant(monkeypatch):
    mod, _ = _load_data_operations_module(monkeypatch)
    cpn = _component(mod, _Canvas())
    cpn._param.updates = [{"key": "n", "value": 5}, {"key": "flag", "value": True}]
    cpn.input_objects = [{"a": 1}]

    cpn._append_or_update()

    assert cpn.output("result") == [{"a": 1, "n": 5, "flag": True}]

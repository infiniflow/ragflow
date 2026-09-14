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
"""VariableAggregator selector shapes (issue #19412).

`param_refs` and `_invoke` must accept both dict selectors (`{"value": "a@x"}`)
and plain-string selectors (`"a@x"`), and share one normalization path so the
two cannot drift.

`Canvas.get_variable_value` raises for a missing component (it does not
return None). These tests pin that contract: missing refs are skipped via
exception fallthrough, not via a fake None-returning canvas.
"""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import MagicMock

import pytest

REPO_ROOT = Path(__file__).resolve().parents[3]


@pytest.fixture
def aggregator_mod(monkeypatch):
    def _pkg(name, path):
        mod = ModuleType(name)
        mod.__path__ = [str(path)]
        monkeypatch.setitem(sys.modules, name, mod)
        return mod

    def _stub(name, **attrs):
        mod = ModuleType(name)
        for key, value in attrs.items():
            setattr(mod, key, value)
        monkeypatch.setitem(sys.modules, name, mod)
        return mod

    def _real(name, relpath):
        spec = importlib.util.spec_from_file_location(name, REPO_ROOT / relpath)
        mod = importlib.util.module_from_spec(spec)
        monkeypatch.setitem(sys.modules, name, mod)
        spec.loader.exec_module(mod)
        return mod

    _pkg("common", REPO_ROOT / "common")
    _pkg("agent", REPO_ROOT / "agent")
    component_pkg = _pkg("agent.component", REPO_ROOT / "agent" / "component")

    _stub("common.connection_utils", timeout=lambda *a, **kw: lambda fn: fn)
    _stub("common.constants", LLMType=MagicMock(), RetCode=MagicMock())
    _stub("common.token_utils", token_usage_sink=MagicMock(), langfuse_run_attrs=MagicMock())
    _stub(
        "common.llm_request_context",
        set_llm_request_context=MagicMock(),
        reset_llm_request_context=MagicMock(),
    )
    _stub("common.exceptions", TaskCanceledException=type("TaskCanceledException", (Exception,), {}))
    _stub("pandas", DataFrame=MagicMock(), Series=MagicMock())
    _real("common.misc_utils", "common/misc_utils.py")
    _real("agent.settings", "agent/settings.py")
    base = _real("agent.component.base", "agent/component/base.py")
    component_pkg.base = base

    canvas_mod = ModuleType("agent.canvas")
    canvas_mod.Graph = type("Graph", (), {})
    monkeypatch.setitem(sys.modules, "agent.canvas", canvas_mod)

    return _real("agent.component.variable_aggregator", "agent/component/variable_aggregator.py")


class _Canvas:
    """Mirrors Canvas.get_variable_value: raises for missing refs."""

    def __init__(self, values: dict):
        self._values = values
        self.requested: list = []

    def get_variable_value(self, key):
        self.requested.append(key)
        if key not in self._values:
            # Same failure mode as agent/canvas.py: missing component raises.
            raise Exception(f"Can't find variable: '{key}'")
        return self._values[key]


def _make_component(mod, groups, values):
    param = mod.VariableAggregatorParam()
    param.groups = groups
    param.check()
    canvas = _Canvas(values)
    component = object.__new__(mod.VariableAggregator)
    component._canvas = canvas
    component._id = "agg"
    component._param = param
    component._unrunnable = ""
    component._unrunnable_error = ""
    return component, canvas


def test_normalize_selector_ref_shapes(aggregator_mod):
    n = aggregator_mod.normalize_selector_ref
    assert n({"value": "a@x"}) == "a@x"
    assert n("a@x") == "a@x"
    assert n("  a@x  ") == "a@x"
    assert n("{a@x}") == "a@x"
    assert n("") == ""
    assert n("   ") == ""
    assert n({"value": ""}) == ""
    assert n({"value": "   "}) == ""
    assert n({}) == ""
    assert n(None) == ""
    assert n(123) == ""


def test_param_refs_accepts_string_and_dict_selectors(aggregator_mod):
    component, _ = _make_component(
        aggregator_mod,
        [{"group_name": "G", "variables": ["a@x", {"value": "b@y"}, "", {"value": " "}]}],
        {},
    )
    assert component.param_refs() == ["a@x", "b@y"]


def test_invoke_resolves_dict_selectors(aggregator_mod):
    component, canvas = _make_component(
        aggregator_mod,
        [{"group_name": "G", "variables": [{"value": "a@x"}]}],
        {"a@x": "from-dict"},
    )
    component._invoke()
    assert canvas.requested == ["a@x"]
    assert component.output("G") == "from-dict"


def test_invoke_resolves_string_selectors(aggregator_mod):
    component, canvas = _make_component(
        aggregator_mod,
        [{"group_name": "G", "variables": ["a@x"]}],
        {"a@x": "from-string"},
    )
    component._invoke()
    assert canvas.requested == ["a@x"]
    assert component.output("G") == "from-string"


def test_invoke_skips_invalid_and_missing_then_uses_next(aggregator_mod):
    """Empty/malformed selectors never reach the canvas; a missing component
    raises from the canvas fake (as in production) and falls through."""
    component, canvas = _make_component(
        aggregator_mod,
        [
            {
                "group_name": "G",
                "variables": [
                    "",
                    {"value": "  "},
                    {},
                    None,
                    "missing@x",
                    "a@x",
                    {"value": "b@y"},
                ],
            }
        ],
        {"a@x": "second", "b@y": "third"},
    )
    component._invoke()
    assert canvas.requested == ["missing@x", "a@x"]
    assert component.output("G") == "second"


def test_invoke_mixed_shapes_pick_first_truthy(aggregator_mod):
    component, canvas = _make_component(
        aggregator_mod,
        [{"group_name": "G", "variables": [{"value": "a@x"}, "b@y"]}],
        {"a@x": "", "b@y": "fallback"},
    )
    component._invoke()
    assert canvas.requested == ["a@x", "b@y"]
    assert component.output("G") == "fallback"

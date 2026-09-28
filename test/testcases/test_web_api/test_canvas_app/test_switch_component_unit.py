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

"""
Unit tests for the Switch component, focused on issue #14235:
the Switch condition should be able to target nested fields inside
object outputs from upstream components, not just the whole output.

The Switch component passes ``item["cpn_id"]`` to
``canvas.get_variable_value``, which already understands the
``cpn_id@root.nested.path`` syntax (see ``Graph.get_variable_value``
and ``Graph.get_variable_param_value`` in agent/canvas.py).  These
tests pin that contract down end-to-end through Switch._invoke.
"""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import MagicMock

import pytest

pytestmark = pytest.mark.p2


def _load_switch_module(monkeypatch):
    """Load the real Switch component module with lightweight stubs for the
    heavy ``agent``/``common`` packages so the test runs without the full
    RAGFlow runtime."""
    repo_root = Path(__file__).resolve().parents[4]

    quart = ModuleType("quart")
    quart.make_response = lambda *a, **kw: None
    quart.jsonify = lambda *a, **kw: None
    monkeypatch.setitem(sys.modules, "quart", quart)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    constants = ModuleType("common.constants")

    class _RetCode:
        SUCCESS = 0
        EXCEPTION_ERROR = 100

    constants.RetCode = _RetCode
    monkeypatch.setitem(sys.modules, "common.constants", constants)

    conn_spec = importlib.util.spec_from_file_location("common.connection_utils", repo_root / "common" / "connection_utils.py")
    conn_mod = importlib.util.module_from_spec(conn_spec)
    monkeypatch.setitem(sys.modules, "common.connection_utils", conn_mod)
    conn_spec.loader.exec_module(conn_mod)

    misc_spec = importlib.util.spec_from_file_location("common.misc_utils", repo_root / "common" / "misc_utils.py")
    misc_mod = importlib.util.module_from_spec(misc_spec)
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_mod)
    misc_spec.loader.exec_module(misc_mod)

    agent_pkg = ModuleType("agent")
    agent_pkg.__path__ = [str(repo_root / "agent")]
    monkeypatch.setitem(sys.modules, "agent", agent_pkg)

    agent_settings = ModuleType("agent.settings")
    agent_settings.FLOAT_ZERO = 1e-8
    agent_settings.PARAM_MAXDEPTH = 5
    monkeypatch.setitem(sys.modules, "agent.settings", agent_settings)

    component_pkg = ModuleType("agent.component")
    component_pkg.__path__ = [str(repo_root / "agent" / "component")]
    monkeypatch.setitem(sys.modules, "agent.component", component_pkg)

    canvas_mod = ModuleType("agent.canvas")

    class Graph:
        pass

    canvas_mod.Graph = Graph
    monkeypatch.setitem(sys.modules, "agent.canvas", canvas_mod)

    base_spec = importlib.util.spec_from_file_location("agent.component.base", repo_root / "agent" / "component" / "base.py")
    base_mod = importlib.util.module_from_spec(base_spec)
    monkeypatch.setitem(sys.modules, "agent.component.base", base_mod)
    base_spec.loader.exec_module(base_mod)

    switch_spec = importlib.util.spec_from_file_location("agent.component.switch", repo_root / "agent" / "component" / "switch.py")
    switch_mod = importlib.util.module_from_spec(switch_spec)
    monkeypatch.setitem(sys.modules, "agent.component.switch", switch_mod)
    switch_spec.loader.exec_module(switch_mod)

    return switch_mod


# --- helpers ---------------------------------------------------------------


def _walk_path(obj, path):
    """Walk a dotted ``path`` through nested dicts/lists/attrs — a subset of
    ``Graph.get_variable_param_value`` used by the canvas resolver."""
    cur = obj
    for key in path.split("."):
        if cur is None:
            return None
        if isinstance(cur, dict):
            cur = cur.get(key)
            continue
        if isinstance(cur, (list, tuple)):
            try:
                cur = cur[int(key)]
            except Exception:
                return None
            continue
        cur = getattr(cur, key, None)
    return cur


def _build_canvas(component_outputs):
    """
    Build a mock canvas whose ``get_variable_value`` resolves
    ``cpn_id@root.nested.path`` against the supplied ``component_outputs``
    dict (mapping cpn_id -> {root_key: root_value}).  This mimics the
    real Graph.get_variable_value parsing rules so the test exercises
    the same contract the Switch component depends on.
    """
    canvas = MagicMock()
    canvas.is_canceled = MagicMock(return_value=False)

    def resolve(exp):
        exp = exp.strip("{").strip("}").strip(" ").strip("{").strip("}")
        if "@" not in exp:
            return None
        cpn_id, var_nm = exp.split("@")
        parts = var_nm.split(".", 1)
        root_key = parts[0]
        rest = parts[1] if len(parts) > 1 else ""
        root_val = component_outputs.get(cpn_id, {}).get(root_key)
        if not rest:
            return root_val
        return _walk_path(root_val, rest)

    canvas.get_variable_value = MagicMock(side_effect=resolve)
    canvas.get_component_name = MagicMock(side_effect=lambda cid: cid)
    return canvas


def _make_switch(module, canvas, conditions, end_cpn_ids):
    """Build a Switch instance bound to the supplied mock canvas, bypassing
    ``ComponentBase.__init__`` (which would require a real Graph)."""
    param = module.SwitchParam.__new__(module.SwitchParam)
    param.conditions = conditions
    param.end_cpn_ids = end_cpn_ids
    param.outputs = {}
    param.inputs = {}
    param.debug_inputs = {}

    inst = module.Switch.__new__(module.Switch)
    inst._canvas = canvas
    inst._param = param
    inst._id = "Switch:test"
    return inst


# --- tests -----------------------------------------------------------------


def test_switch_compares_nested_string_field_with_equals(monkeypatch):
    """Switch routes to the matching branch when a single-level nested string
    field equals the literal value."""
    module = _load_switch_module(monkeypatch)
    canvas = _build_canvas({"categorize:0": {"output": {"category": "tech", "score": 0.9, "meta": {"lang": "en"}}}})
    sw = _make_switch(
        module,
        canvas,
        conditions=[
            {
                "logical_operator": "and",
                "items": [
                    {
                        "cpn_id": "categorize:0@output.category",
                        "operator": "=",
                        "value": "tech",
                    }
                ],
                "to": ["llm:tech"],
            },
            {
                "logical_operator": "and",
                "items": [
                    {
                        "cpn_id": "categorize:0@output.category",
                        "operator": "=",
                        "value": "news",
                    }
                ],
                "to": ["llm:news"],
            },
        ],
        end_cpn_ids=["llm:fallback"],
    )
    sw._invoke()
    assert sw.output("_next") == ["llm:tech"]


def test_switch_compares_doubly_nested_string_field(monkeypatch):
    """Switch resolves a two-level dotted path (`output.meta.lang`)."""
    module = _load_switch_module(monkeypatch)
    canvas = _build_canvas({"categorize:0": {"output": {"category": "tech", "meta": {"lang": "fr"}}}})
    sw = _make_switch(
        module,
        canvas,
        conditions=[
            {
                "logical_operator": "and",
                "items": [
                    {
                        "cpn_id": "categorize:0@output.meta.lang",
                        "operator": "=",
                        "value": "fr",
                    }
                ],
                "to": ["fr_branch"],
            }
        ],
        end_cpn_ids=["else"],
    )
    sw._invoke()
    assert sw.output("_next") == ["fr_branch"]


def test_switch_compares_nested_number_field_with_greater_than(monkeypatch):
    """Switch coerces the literal to float when the resolved value is a
    number, so `> 0.5` works on a nested score field."""
    module = _load_switch_module(monkeypatch)
    canvas = _build_canvas({"categorize:0": {"output": {"score": 0.87}}})
    sw = _make_switch(
        module,
        canvas,
        conditions=[
            {
                "logical_operator": "and",
                "items": [
                    {
                        "cpn_id": "categorize:0@output.score",
                        "operator": ">",
                        "value": "0.5",
                    }
                ],
                "to": ["high"],
            }
        ],
        end_cpn_ids=["low"],
    )
    sw._invoke()
    assert sw.output("_next") == ["high"]


def test_switch_nested_field_contains_operator(monkeypatch):
    """The `contains` operator works against a nested string field."""
    module = _load_switch_module(monkeypatch)
    canvas = _build_canvas({"begin": {"output": {"user": {"email": "alice@example.com"}}}})
    sw = _make_switch(
        module,
        canvas,
        conditions=[
            {
                "logical_operator": "and",
                "items": [
                    {
                        "cpn_id": "begin@output.user.email",
                        "operator": "contains",
                        "value": "@example.com",
                    }
                ],
                "to": ["internal"],
            }
        ],
        end_cpn_ids=["external"],
    )
    sw._invoke()
    assert sw.output("_next") == ["internal"]


def test_switch_nested_field_falls_through_to_else(monkeypatch):
    """When no nested-field condition matches, Switch routes to
    ``end_cpn_ids`` (the ELSE branch)."""
    module = _load_switch_module(monkeypatch)
    canvas = _build_canvas({"categorize:0": {"output": {"category": "music"}}})
    sw = _make_switch(
        module,
        canvas,
        conditions=[
            {
                "logical_operator": "and",
                "items": [
                    {
                        "cpn_id": "categorize:0@output.category",
                        "operator": "=",
                        "value": "tech",
                    }
                ],
                "to": ["tech"],
            }
        ],
        end_cpn_ids=["else_branch"],
    )
    sw._invoke()
    assert sw.output("_next") == ["else_branch"]


def test_switch_nested_list_index_access(monkeypatch):
    """The canvas resolver supports list indexing via the dotted path,
    so Switch can compare e.g. `chunks.0.text`."""
    module = _load_switch_module(monkeypatch)
    canvas = _build_canvas({"retrieval:0": {"output": {"chunks": [{"text": "hello world"}, {"text": "bye"}]}}})
    sw = _make_switch(
        module,
        canvas,
        conditions=[
            {
                "logical_operator": "and",
                "items": [
                    {
                        "cpn_id": "retrieval:0@output.chunks.0.text",
                        "operator": "contains",
                        "value": "hello",
                    }
                ],
                "to": ["matched"],
            }
        ],
        end_cpn_ids=["unmatched"],
    )
    sw._invoke()
    assert sw.output("_next") == ["matched"]


def test_switch_or_logical_operator_short_circuits_on_nested_match(monkeypatch):
    """With ``logical_operator: or``, a single matching nested-field
    condition wins even when an earlier condition fails."""
    module = _load_switch_module(monkeypatch)
    canvas = _build_canvas({"categorize:0": {"output": {"category": "news", "score": 0.1}}})
    sw = _make_switch(
        module,
        canvas,
        conditions=[
            {
                "logical_operator": "or",
                "items": [
                    {
                        "cpn_id": "categorize:0@output.score",
                        "operator": ">",
                        "value": "0.9",
                    },
                    {
                        "cpn_id": "categorize:0@output.category",
                        "operator": "=",
                        "value": "news",
                    },
                ],
                "to": ["news_branch"],
            }
        ],
        end_cpn_ids=["else"],
    )
    sw._invoke()
    assert sw.output("_next") == ["news_branch"]

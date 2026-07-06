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
"""Hardening regression test for `@`-splitting in
`agent/component/iterationitem.py:82` (inside `output_collation`).

Before the fix, `o["ref"].split("@")` raised `ValueError: too many values
to unpack` whenever a user-defined output key contained `@` (e.g. an
output named `foo@bar` referenced from another component). After the
fix, the call uses `split("@", 1)`, preserving the trailing `@`-bearing
key in `var` so the collation append can proceed normally.

The module is loaded via `importlib` after stubbing its heavyweight
imports — same isolation strategy as
`test/testcases/test_web_api/test_canvas_app/test_iterationitem_unit.py`.
"""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock

import pytest

# ─── Module loader ────────────────────────────────────────────────────


def _load_iterationitem_module(monkeypatch):
    """Load `agent.component.iterationitem` in isolation with stubs.

    Unlike `test_canvas_at_split.py`, the iterationitem module's import
    graph is small enough that we can run the real `common.*` modules
    (after stubbing their `quart` dependency). That gives us high
    fidelity on the `_param.outputs` iteration logic without dragging
    in the canvas-level graph (which the tests don't need).

    Returns:
        The loaded `agent.component.iterationitem` module object.
    """
    repo_root = Path(__file__).resolve().parents[4]

    # Stub `quart` because `common.connection_utils` imports from it.
    quart_stub = ModuleType("quart")
    quart_stub.make_response = MagicMock()
    quart_stub.jsonify = MagicMock()
    monkeypatch.setitem(sys.modules, "quart", quart_stub)

    # Stub the heavy transitive imports so we never touch pandas/jinja2/etc.
    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    conn_spec = importlib.util.spec_from_file_location("common.connection_utils", repo_root / "common" / "connection_utils.py")
    conn_mod = importlib.util.module_from_spec(conn_spec)
    monkeypatch.setitem(sys.modules, "common.connection_utils", conn_mod)
    conn_spec.loader.exec_module(conn_mod)

    misc_spec = importlib.util.spec_from_file_location("common.misc_utils", repo_root / "common" / "misc_utils.py")
    misc_mod = importlib.util.module_from_spec(misc_spec)
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_mod)
    misc_spec.loader.exec_module(misc_mod)

    constants_mod = ModuleType("common.constants")

    class _RetCode:
        """Minimal stand-in for `common.constants.RetCode`.

        Only the two integer sentinels actually referenced by
        `iterationitem.py` are defined; everything else would otherwise
        require loading the entire constants module.
        """

        SUCCESS = 0
        EXCEPTION_ERROR = 100

    constants_mod.RetCode = _RetCode
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

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

    # Provide a minimal `ComponentBase` for the iterationitem module to
    # inherit from. Only the surface used by `output_collation` matters:
    # `output(var_nm)`, `set_output(key, value)`, plus the parent lookup
    # `get_parent()` is defined on the test instance directly.
    class _ComponentBaseStub:
        """No-op stand-in for the real `ComponentBase`.

        `iterationitem.IterationItem` extends this class, so any
        instantiation path that isn't bypassed via `__new__` would
        require it to be importable. We deliberately no-op every
        method so that nothing incidental fires during loading.
        """

        def __init__(self, *a, **kw):
            """Accept and discard all args; the stub is purely nominal."""
            pass

        def output(self, var_nm=None):
            """Return an empty string for any variable lookup."""
            return ""

        def set_output(self, key, value):
            """Discard writes; iterationitem test paths verify via
            `parent._param.outputs`, not via the base class."""
            pass

    class _ComponentParamBaseStub:
        """Stand-in for `ComponentParamBase` with mutable `outputs`/`inputs`."""

        def __init__(self):
            """Initialize empty `outputs` and `inputs` dicts."""
            self.outputs = {}
            self.inputs = {}

        def check(self):
            """Always succeed; validation is not exercised in these tests."""
            return True

    base_mod = ModuleType("agent.component.base")
    base_mod.ComponentBase = _ComponentBaseStub
    base_mod.ComponentParamBase = _ComponentParamBaseStub
    monkeypatch.setitem(sys.modules, "agent.component.base", base_mod)

    iterationitem_spec = importlib.util.spec_from_file_location(
        "agent.component.iterationitem",
        repo_root / "agent" / "component" / "iterationitem.py",
    )
    iterationitem_mod = importlib.util.module_from_spec(iterationitem_spec)
    monkeypatch.setitem(sys.modules, "agent.component.iterationitem", iterationitem_mod)
    iterationitem_spec.loader.exec_module(iterationitem_mod)
    return iterationitem_mod


# ─── Fixtures ─────────────────────────────────────────────────────────


class _ParentStub:
    """Parent iteration component.

    `_param.outputs` is keyed by output name; each entry has a `"ref"`
    pointing to a child component (`"<cid>@<var_name>"`).
    """

    def __init__(self, _id, outputs, component_name="Iteration"):
        """Store `_id`, default each output's `"value"` to `[]`, expose
        the canonical `outputs` dict on a `_param` SimpleNamespace."""
        self._id = _id
        self.component_name = component_name
        # Normalize: every output entry must carry a `"value"` key so
        # `p.output(k)` and the post-append `p.set_output(k, res)` work
        # the same way they would on a real `ComponentBase`.
        normalized = {}
        for name, payload in outputs.items():
            entry = dict(payload)
            entry.setdefault("value", [])
            normalized[name] = entry
        self._param = SimpleNamespace(outputs=normalized)

    def output(self, var_nm):
        """Return the `"value"` of the named output, or `""` if absent."""
        return self._param.outputs.get(var_nm, {}).get("value", "")

    def set_output(self, key, value):
        """Write `value` into `_param.outputs[key]["value"]`, creating
        the entry with the appropriate `type` sentinel on first write."""
        if key not in self._param.outputs:
            self._param.outputs[key] = {"value": None, "type": str(type(value))}
        self._param.outputs[key]["value"] = value


class _ChildStub:
    """Child component inside an iteration.

    `_param.outputs` stores the child component's own outputs in the
    `ComponentBase` `{"value": ...}` shape.
    """

    def __init__(self, _id, parent, output_values):
        """Bind to a parent and seed `_param.outputs` from a flat dict.

        Args:
            _id: Child component id used as the `<cid>` half of refs.
            parent: The parent iteration component (for `get_parent`).
            output_values: `{output_name: value}` flat mapping, wrapped
                into the `ComponentBase` `{"value": ..., "type": ...}`
                shape before storage.
        """
        self._id = _id
        self._parent = parent
        self.component_name = "Generate"
        self._param = SimpleNamespace(outputs={k: {"value": v, "type": str(type(v))} for k, v in output_values.items()})

    def get_parent(self):
        """Return the parent iteration component passed at construction."""
        return self._parent

    def output(self, var_nm):
        """Return the `"value"` of the named output, or `""` if absent."""
        return self._param.outputs.get(var_nm, {}).get("value", "")

    def set_output(self, key, value):
        """Write `value` into `_param.outputs[key]["value"]`, creating
        the entry with the appropriate `type` sentinel on first write."""
        if key not in self._param.outputs:
            self._param.outputs[key] = {"value": None, "type": str(type(value))}
        self._param.outputs[key]["value"] = value


class _CanvasStub:
    """Minimal canvas: only `components` (dict) and `get_component_obj`
    are touched by `output_collation`."""

    def __init__(self, components):
        """Store the `{cid: component}` lookup used by `get_component_obj`."""
        self.components = components

    def get_component_obj(self, cid):
        """Look up a component by id; the stub never returns `None`."""
        return self.components[cid]


def _make_iteration_item(module, canvas, parent):
    """Construct an `IterationItem` bypassing its normal `__init__`.

    `output_collation` only touches `self._canvas`, `self._id`, and
    `self.get_parent()`, so a stripped-down instance is sufficient.
    """
    inst = module.IterationItem.__new__(module.IterationItem)
    inst._canvas = canvas
    inst._id = "IterationItem:test"
    inst._param = SimpleNamespace(outputs={}, inputs={})
    inst._idx = 0
    inst.get_parent = MagicMock(return_value=parent)
    return inst


# ─── Tests ────────────────────────────────────────────────────────────


@pytest.mark.p2
def test_output_collation_accepts_at_in_var_name(monkeypatch):
    """The parent iteration's output `result` references the child's
    output `foo@bar` (a user-defined key containing `@`). Before the
    fix, `o["ref"].split("@")` raised `ValueError: too many values to
    unpack`. After the fix, `output_collation` must:
      1. Not raise.
      2. Append the child's `foo@bar` value into the parent's `result`
         list (i.e. treat `var = "foo@bar"` as a literal output name).
    """
    module = _load_iterationitem_module(monkeypatch)

    parent_outputs = {
        "result": {"ref": "child-1@foo@bar"},
    }
    parent = _ParentStub("iter-pid", parent_outputs)

    child = _ChildStub(
        _id="child-1",
        parent=parent,
        output_values={"foo@bar": "value-A"},
    )

    canvas = _CanvasStub({"child-1": child})

    item = _make_iteration_item(module, canvas, parent)

    # Should NOT raise ValueError.
    item.output_collation()

    # The child's `foo@bar` value should have been appended to the
    # parent's `result` list.
    assert parent._param.outputs["result"]["value"] == ["value-A"]


@pytest.mark.p2
def test_output_collation_single_at_still_works(monkeypatch):
    """Sanity check: the existing single-`@` ref path is unaffected."""
    module = _load_iterationitem_module(monkeypatch)

    parent_outputs = {
        "result": {"ref": "child-1@normal_var"},
    }
    parent = _ParentStub("iter-pid", parent_outputs)

    child = _ChildStub(
        _id="child-1",
        parent=parent,
        output_values={"normal_var": "value-B"},
    )

    canvas = _CanvasStub({"child-1": child})

    item = _make_iteration_item(module, canvas, parent)

    item.output_collation()

    assert parent._param.outputs["result"]["value"] == ["value-B"]


@pytest.mark.p2
def test_output_collation_skips_non_matching_cid(monkeypatch):
    """When the ref's `cid` doesn't match the current `cid`, the loop
    must skip silently even if `var` contains `@`. This guards against
    an over-eager fix that might, e.g., re-split on the wrong side."""
    module = _load_iterationitem_module(monkeypatch)

    # The ref points at `other-cid`, not `child-1`.
    parent_outputs = {
        "result": {"ref": "other-cid@foo@bar"},
    }
    parent = _ParentStub("iter-pid", parent_outputs)

    child = _ChildStub(
        _id="child-1",
        parent=parent,
        output_values={"foo@bar": "ignored"},
    )

    canvas = _CanvasStub({"child-1": child})

    item = _make_iteration_item(module, canvas, parent)

    # Must not raise.
    item.output_collation()

    # Nothing should have been appended (the ref didn't match this cid).
    assert parent._param.outputs["result"]["value"] == []

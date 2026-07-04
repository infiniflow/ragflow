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
"""Hardening regression tests for `@`-splitting in agent/canvas.py.

These exercise `get_variable_value` and `set_variable_value` on
`agent/canvas.py` (lines 201 and 248 of the Graph class, inherited by
`Canvas`) with references whose variable name contains `@`. Before the
fix, `exp.split("@")` would raise `ValueError: too many values to unpack`
because Python splits on every `@` by default. After the fix, the calls
use `split("@", 1)`, so the trailing `@`-bearing variable name is
preserved verbatim.

We load `agent/canvas.py` via `importlib` after stubbing its heavyweight
imports (LLM service, task service, etc.) with MagicMock modules — same
isolation strategy as
`test/testcases/test_web_api/test_canvas_app/test_iterationitem_unit.py`,
which lets these tests run without a live DB / Redis / MinIO stack.
"""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock

import pytest

# ─── Module loader ────────────────────────────────────────────────────


def _load_canvas_module(monkeypatch):
    """Load `agent.canvas` in isolation with all heavy deps stubbed out.

    The canvas import graph pulls in `pandas`, `quart`, `jinja2`, the
    real `ComponentBase`, ORM models, Redis, TTS cache, etc. None of
    that is needed for the `get_variable_value` / `set_variable_value`
    paths under test, so we register lightweight `ModuleType` stubs in
    `sys.modules` first and then `exec_module` the real `canvas.py`
    against that fake module table.

    Returns:
        The loaded `agent.canvas` module object.
    """
    repo_root = Path(__file__).resolve().parents[3]

    # `agent.component.base` pulls in pandas, quart, jinja2, etc.
    # Stub the modules it (and the rest of the canvas import graph) needs
    # before loading, so we never touch the real implementations.
    def _stub_module(name, **attrs):
        """Create a fresh `ModuleType`, attach attrs, register in `sys.modules`.

        Used to short-circuit every transitive import that `canvas.py`
        performs at module load time. The returned module is also the
        actual object that `import` statements resolve to during
        `exec_module`.

        Args:
            name: Fully-qualified module name (e.g. ``"common.constants"``).
            **attrs: Attributes to set on the new module before it is
                registered (typically classes, callables, or sentinel
                objects expected by importers).

        Returns:
            The newly created `ModuleType` instance.
        """
        mod = ModuleType(name)
        for k, v in attrs.items():
            setattr(mod, k, v)
        monkeypatch.setitem(sys.modules, name, mod)
        return mod

    # Parent packages must exist as packages with `__path__` so submodule
    # imports resolve correctly.
    for pkg_name, pkg_path in [
        ("common", repo_root / "common"),
        ("agent", repo_root / "agent"),
        ("agent.component", repo_root / "agent" / "component"),
    ]:
        pkg = ModuleType(pkg_name)
        pkg.__path__ = [str(pkg_path)]
        monkeypatch.setitem(sys.modules, pkg_name, pkg)

    _stub_module("agent.settings", FLOAT_ZERO=1e-8, PARAM_MAXDEPTH=5)

    # `agent.canvas` and `agent.component.base` import each other
    # indirectly. Provide a minimal `ComponentBase` so the canvas module
    # can be loaded without dragging the real one in.
    class _ComponentBaseStub:
        """Minimal stand-in for `agent.component.base.ComponentBase`.

        The real class would drag in the entire component registry and
        ORM-layer initialization; we only need an object that
        `canvas.py`'s imports can bind to without side effects.
        """

        def __init__(self, *a, **kw):
            """Accept and discard all args; the stub is purely nominal."""
            pass

    base_stub_mod = _stub_module("agent.component.base")
    base_stub_mod.ComponentBase = _ComponentBaseStub
    base_stub_mod.ComponentParamBase = type("ComponentParamBase", (), {"outputs": {}, "inputs": {}})

    # `agent.component.component_class` is a registry factory used at
    # canvas load time. The tests below never call `Canvas.load()`, so
    # any callable suffices.
    _stub_module("agent.component", component_class=lambda *_a, **_kw: MagicMock())

    _stub_module("agent.dsl_migration", normalize_chunker_dsl=lambda dsl: dsl)

    # `api.*` services imported at the top of canvas.py
    _stub_module("api.db.services.file_service", FileService=MagicMock())
    _stub_module("api.db.services.llm_service", LLMBundle=MagicMock())
    _stub_module("api.db.services.task_service", has_canceled=MagicMock(return_value=False))
    _stub_module(
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=MagicMock(return_value=None),
    )
    _stub_module("common.constants", LLMType=MagicMock())
    _stub_module("common.misc_utils", get_uuid=lambda: "test-uuid", hash_str2int=lambda _s: 0, thread_pool_exec=lambda fn, *a, **kw: fn(*a, **kw))
    _stub_module(
        "common.token_utils",
        token_usage_sink=lambda *_a, **_kw: None,
        langfuse_run_attrs=lambda *_a, **_kw: {},
    )
    _stub_module("common.connection_utils", timeout=lambda *_a, **_kw: lambda fn: fn)
    _stub_module("common.exceptions", TaskCanceledException=type("TaskCanceledException", (Exception,), {}))
    _stub_module("rag.prompts.generator", chunks_format=MagicMock())
    _stub_module("rag.utils.redis_conn", REDIS_CONN=MagicMock())
    _stub_module("rag.utils.tts_cache", synthesize_with_cache=MagicMock())

    spec = importlib.util.spec_from_file_location("agent.canvas", repo_root / "agent" / "canvas.py")
    canvas_mod = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "agent.canvas", canvas_mod)
    spec.loader.exec_module(canvas_mod)
    return canvas_mod


# ─── Fixtures ─────────────────────────────────────────────────────────


class _ComponentObjStub:
    """Minimal stub emulating the `cpn["obj"]` interface used by
    `get_variable_value` / `set_variable_value`.

    Only `.output(var_nm)` and `.set_output(var_nm, value)` are needed,
    mirroring `ComponentBase.output` / `ComponentBase.set_output`.
    """

    def __init__(self):
        """Initialize an empty per-instance output store."""
        self._store: dict = {}

    def output(self, var_nm):
        return self._store.get(var_nm)

    def set_output(self, var_nm, value):
        self._store[var_nm] = value


def _make_canvas(canvas_mod, components=None, globals_=None):
    """Construct a `Graph` instance that bypasses `Canvas.__init__`.

    The methods under test (`get_variable_value`, `set_variable_value`)
    only touch `self.globals` and `self.components` (via `get_component`),
    so a stripped-down instance is sufficient.
    """

    inst = canvas_mod.Graph.__new__(canvas_mod.Graph)
    inst.globals = dict(globals_ or {})
    inst.components = dict(components or {})
    return inst


# ─── Tests ────────────────────────────────────────────────────────────


@pytest.mark.p2
def test_get_variable_value_preserves_at_in_var_name(monkeypatch):
    """`get_variable_value("{cpn@foo@bar}")` must not raise; it should
    return the component's stored value for the literal key `foo@bar`."""
    canvas_mod = _load_canvas_module(monkeypatch)

    cpn_obj = _ComponentObjStub()
    cpn_obj.set_output("foo@bar", "value-1")

    canvas = _make_canvas(canvas_mod, components={"cpn-1": {"obj": cpn_obj}})

    result = canvas.get_variable_value("{cpn-1@foo@bar}")

    assert result == "value-1"


@pytest.mark.p2
def test_set_variable_value_preserves_at_in_var_name(monkeypatch):
    """`set_variable_value` must not raise on a `@`-bearing var name."""
    canvas_mod = _load_canvas_module(monkeypatch)

    cpn_obj = _ComponentObjStub()
    canvas = _make_canvas(canvas_mod, components={"cpn-1": {"obj": cpn_obj}})

    canvas.set_variable_value("{cpn-1@foo@bar}", "value-2")

    assert cpn_obj.output("foo@bar") == "value-2"


@pytest.mark.p2
def test_get_variable_value_legacy_split_would_raise(monkeypatch):
    """Regression: under the previous `exp.split("@")`, this same
    expression raised `ValueError: too many values to unpack`. Asserting
    that the call returns cleanly (and resolves to the stored value)
    proves the `split("@", 1)` hardening is in effect."""
    canvas_mod = _load_canvas_module(monkeypatch)

    cpn_obj = _ComponentObjStub()
    cpn_obj.set_output("nested@with@ats", {"nested": True})

    canvas = _make_canvas(canvas_mod, components={"cpn-1": {"obj": cpn_obj}})

    # If the legacy `split("@")` were in place this would raise
    # `ValueError: too many values to unpack (expected 2)`.
    result = canvas.get_variable_value("{cpn-1@nested@with@ats}")

    assert result == {"nested": True}


@pytest.mark.p2
def test_set_variable_value_then_get_round_trip(monkeypatch):
    """Round-trip: a write through `set_variable_value` with a multi-`@`
    key should be observable through `get_variable_value`."""
    canvas_mod = _load_canvas_module(monkeypatch)

    cpn_obj = _ComponentObjStub()
    canvas = _make_canvas(canvas_mod, components={"cpn-1": {"obj": cpn_obj}})

    canvas.set_variable_value("{cpn-1@user@email}", "alice@example.com")

    assert canvas.get_variable_value("{cpn-1@user@email}") == "alice@example.com"


@pytest.mark.p2
def test_get_variable_value_missing_component_still_raises(monkeypatch):
    """The hardening must NOT swallow the existing `Can't find variable`
    exception for unknown component IDs — that's a legitimate error
    path that callers depend on."""
    canvas_mod = _load_canvas_module(monkeypatch)

    canvas = _make_canvas(canvas_mod, components={})

    with pytest.raises(Exception, match="Can't find variable"):
        canvas.get_variable_value("{missing-cpn@foo@bar}")


@pytest.mark.p2
def test_get_variable_value_single_at_still_works(monkeypatch):
    """Sanity check that the existing single-`@` path is unaffected by
    the hardening (no regression in the common case)."""
    canvas_mod = _load_canvas_module(monkeypatch)

    cpn_obj = _ComponentObjStub()
    cpn_obj.set_output("normal_var", "normal-value")

    canvas = _make_canvas(canvas_mod, components={"cpn-1": {"obj": cpn_obj}})

    assert canvas.get_variable_value("{cpn-1@normal_var}") == "normal-value"

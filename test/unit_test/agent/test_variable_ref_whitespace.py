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
"""Regression tests for template-ref pattern whitespace handling (#18665).

Issue #18665: `ComponentBase.variable_ref_patt` / `iteration_alias_patt`
used `{* *` and ` *}*` to tolerate outer-brace forms like `{ {X@y} }` and
`{{X@y}}`. Because `{*` / `}*` also match the empty string, the
free-standing ` *` consumed the literal spaces immediately before and after
a plain `{X@y}` reference, so the streaming path in `Message._stream` (which
slices around each match) dropped those spaces from the rendered text.

The behavior was fixed on `main` by `_build_template_ref_pattern` (PR #18666,
commit a1131167): whitespace is now only consumed inside a balanced outer
brace pair. These tests pin the issue's Expected behavior contract so the two
patterns cannot regress silently.

We load `agent/component/base.py` via `importlib` after stubbing its
heavyweight imports (pandas, common.* helpers) with MagicMock modules — same
isolation strategy as `test/unit_test/agent/test_canvas_at_split.py`, so these
tests run without a live DB / Redis / MinIO stack.
"""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import MagicMock

import pytest


def _load_component_base_module(monkeypatch):
    """Load `agent.component.base` in isolation with heavyweight deps stubbed.

    `base.py` pulls in `pandas` and `common.*` helpers at import time; none of
    that is needed for the compiled reference patterns under test, so we
    register lightweight `ModuleType` stubs in `sys.modules` first and then
    `exec_module` the real `base.py` against that fake module table.

    Returns:
        The loaded `agent.component.base` module object.
    """
    repo_root = Path(__file__).resolve().parents[3]

    def _stub_module(name, **attrs):
        """Create a fresh `ModuleType`, attach attrs, register in `sys.modules`."""
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
    _stub_module("common.connection_utils", timeout=lambda *_a, **_kw: lambda fn: fn)
    _stub_module("common.misc_utils", thread_pool_exec=lambda fn, *a, **kw: fn(*a, **kw))
    _stub_module("pandas", DataFrame=MagicMock())

    spec = importlib.util.spec_from_file_location("agent.component.base", repo_root / "agent" / "component" / "base.py")
    base_mod = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "agent.component.base", base_mod)
    spec.loader.exec_module(base_mod)
    return base_mod


def _render_with_stream_slicing(base_mod, content, values):
    """Emulate `Message._stream`'s slice-around-the-match rendering.

    The streaming path yields `content[s:r.start()]` (literal text) then the
    resolved value for `r.group(1)`, then continues from `r.end()`. Asserting
    on this reconstruction pins the user-visible contract from issue #18665:
    spaces adjacent to a reference are literal text and must be preserved.
    """
    out = []
    s = 0
    for r in base_mod.ComponentBase.variable_ref_patt_re.finditer(content):
        out.append(content[s : r.start()])
        out.append(values[r.group(1)])
        s = r.end()
    out.append(content[s:])
    return "".join(out)


@pytest.mark.p2
def test_plain_reference_does_not_swallow_adjacent_spaces(monkeypatch):
    """`a {X@y} b` must match exactly `{X@y}`; surrounding spaces are literal."""
    base_mod = _load_component_base_module(monkeypatch)
    matches = list(base_mod.ComponentBase.variable_ref_patt_re.finditer("a {X@y} b"))
    assert [m.group(0) for m in matches] == ["{X@y}"]


@pytest.mark.p2
def test_adjacent_references_keep_their_separating_space(monkeypatch):
    """`{A@x} {B@y}` yields two matches; the space between them is not consumed."""
    base_mod = _load_component_base_module(monkeypatch)
    matches = list(base_mod.ComponentBase.variable_ref_patt_re.finditer("{A@x} {B@y}"))
    assert [m.group(0) for m in matches] == ["{A@x}", "{B@y}"]


@pytest.mark.p2
def test_sys_and_env_references_keep_adjacent_spaces(monkeypatch):
    """The `sys.*` / `env.*` alternates get the same treatment."""
    base_mod = _load_component_base_module(monkeypatch)
    matches = list(base_mod.ComponentBase.variable_ref_patt_re.finditer("a {sys.x} b {env.y} c"))
    assert [m.group(0) for m in matches] == ["{sys.x}", "{env.y}"]


@pytest.mark.p2
def test_whitespace_inside_outer_braces_still_matches(monkeypatch):
    """`{ {X@y} }` / `{{X@y}}` / `{ {X@y}}` still match as a whole (Canvas strips outer layers)."""
    base_mod = _load_component_base_module(monkeypatch)
    for text in ("{ {X@y} }", "{{X@y}}", "{ {X@y}}"):
        matches = list(base_mod.ComponentBase.variable_ref_patt_re.finditer(text))
        assert len(matches) == 1, text
        assert matches[0].group(1) == "X@y", text


@pytest.mark.p2
def test_iteration_alias_pattern_keeps_adjacent_spaces(monkeypatch):
    """`iteration_alias_patt` had the same swallowing shape; `x {item} y` matches exactly `{item}`."""
    base_mod = _load_component_base_module(monkeypatch)
    matches = list(base_mod.ComponentBase.iteration_alias_patt_re.finditer("x {item} y"))
    assert [m.group(0) for m in matches] == ["{item}"]
    outer = list(base_mod.ComponentBase.iteration_alias_patt_re.finditer("{ {result} }"))
    assert len(outer) == 1
    assert outer[0].group(1) == "result"


@pytest.mark.p2
def test_streaming_render_preserves_spaces_around_references(monkeypatch):
    """The issue's end-to-end repro: `Decision: {Agent:x@content} — done` renders with the spaces."""
    base_mod = _load_component_base_module(monkeypatch)
    rendered = _render_with_stream_slicing(
        base_mod,
        "Decision: {Agent:x@content} — done",
        {"Agent:x@content": "VALUE"},
    )
    assert rendered == "Decision: VALUE — done"
    rendered_adjacent = _render_with_stream_slicing(
        base_mod,
        "{A@x} {B@y}",
        {"A@x": "V1", "B@y": "V2"},
    )
    assert rendered_adjacent == "V1 V2"

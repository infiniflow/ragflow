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
Regression tests for `ComponentBase.variable_ref_patt`.

These guard against the runtime template-substitution regex silently
failing on real-world component ids emitted by the frontend. Historically
the regex accepted `[a-zA-Z:0-9]+` for the `cpn_id` half, which dropped
component ids that contain underscores (e.g. `userfillup_abc`,
`retrieval_xyz`, `llm_0`, `message_0`). When that happens an Agent's user
prompt such as `"Repeat: {userfillup_abc@line}"` is left literal and
the LLM responds to its system prompt directive only — exactly the
"unconsidered await response" symptom reported in #16758.

The fix widens `cpn_id` to `[a-zA-Z0-9_]+`, matching the `cpn_id@var_nm`
shape the frontend actually serialises. These tests pin that contract on
both the regex itself and the higher-level `get_input_elements_from_text`
/ `string_format` helpers that depend on it.
"""

import importlib.util
import re
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


@pytest.fixture
def base_module(monkeypatch):
    """Load only `agent.component.base` with minimal stubs.

    `agent.component.base` imports `pandas as pd` at module-load time but
    the symbols we exercise (`variable_ref_patt`, `get_input_elements_from_text`,
    `string_format`) do not touch it. We stub `pandas` before loading so the
    test stays runnable on minimal test environments.

    We also avoid importing the real `agent.canvas` (and its transitive
    deps) because only the regex + helper methods are exercised here.
    """
    repo_root = Path(__file__).resolve().parents[4]

    fake_pandas = ModuleType("pandas")
    fake_pandas.DataFrame = type("DataFrame", (), {})
    monkeypatch.setitem(sys.modules, "pandas", fake_pandas)

    spec = importlib.util.spec_from_file_location(
        "_base_for_regex_test", repo_root / "agent" / "component" / "base.py"
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_variable_ref_patt_matches_underscored_component_ids(base_module):
    """Frontend-emitted ids like `userfillup_abc@line` must be recognised."""
    patt_str = base_module.ComponentBase.variable_ref_patt
    patt = re.compile(patt_str, flags=re.IGNORECASE | re.DOTALL)

    cases = [
        ("{userfillup_abc@line}", "userfillup_abc@line"),
        ("{retrieval_xyz@chunks}", "retrieval_xyz@chunks"),
        ("{llm_0@content}", "llm_0@content"),
        ("{message_0@answer}", "message_0@answer"),
    ]

    for text, expected in cases:
        matches = list(patt.finditer(text))
        assert matches, f"Expected {text!r} to match variable_ref_patt"
        assert matches[0].group(1) == expected, (
            f"{text!r}: wrong capture — got {matches[0].group(1)!r}, "
            f"expected {expected!r}"
        )


@pytest.mark.p2
def test_variable_ref_patt_still_matches_legacy_ids(base_module):
    """Backward-compat: legacy ids without underscores must still resolve."""
    patt_str = base_module.ComponentBase.variable_ref_patt
    patt = re.compile(patt_str, flags=re.IGNORECASE | re.DOTALL)

    cases = [
        ("{begin@line}", "begin@line"),
        ("{retrieval@chunks}", "retrieval@chunks"),
        ("{sys.query}", "sys.query"),
        ("{sys.user_id}", "sys.user_id"),
        ("{env.HOME}", "env.HOME"),
    ]

    for text, expected in cases:
        matches = list(patt.finditer(text))
        assert matches, f"Expected {text!r} to match variable_ref_patt"
        assert matches[0].group(1) == expected


@pytest.mark.p2
def test_get_input_elements_from_text_resolves_underscored_id(base_module):
    """End-to-end: underscored `cpn_id@var_nm` must surface its value in
    `get_input_elements_from_text`. Regression test for #16758.
    """
    cpn = base_module.ComponentBase.__new__(base_module.ComponentBase)
    fake_obj = SimpleNamespace(output=lambda k: "user-text" if k == "line" else "")
    cpn._canvas = SimpleNamespace(
        get_component=lambda _cid: {"obj": fake_obj},
        get_component_name=lambda _cid: "userfillup_abc",
        get_variable_value=lambda exp: "user-text" if exp == "userfillup_abc@line" else None,
    )

    elements = cpn.get_input_elements_from_text("Repeat: {userfillup_abc@line}")
    assert "userfillup_abc@line" in elements, (
        "Underscored `cpn_id@var_nm` template ref was not extracted — "
        "see #16758: Await-response variable ignored by Agent."
    )
    assert elements["userfillup_abc@line"]["value"] == "user-text"
    assert elements["userfillup_abc@line"]["_cpn_id"] == "userfillup_abc"


@pytest.mark.p2
def test_string_format_substitutes_underscored_ref(base_module):
    """If a placeholder survives `get_input_elements_from_text`, it must
    also be substituted by `string_format`. Regression test for #16758.
    """
    cpn = base_module.ComponentBase.__new__(base_module.ComponentBase)
    rendered = cpn.string_format(
        "Repeat: {userfillup_abc@line}",
        {"userfillup_abc@line": "hello world"},
    )
    assert rendered == "Repeat: hello world"


@pytest.mark.p2
def test_variable_ref_patt_does_not_match_bare_var_name(base_module):
    """`{line}` without a cpn_id prefix is intentionally not a template
    ref — it must remain literal so the user sees the literal text in
    their prompt until they wire it up to a real component output.
    """
    patt_str = base_module.ComponentBase.variable_ref_patt
    patt = re.compile(patt_str, flags=re.IGNORECASE | re.DOTALL)
    matches = list(patt.finditer("{line}"))
    assert not matches, (
        "Bare `{line}` should not match — only `cpn_id@var` / `sys.*` / "
        "`env.*` are valid template refs."
    )

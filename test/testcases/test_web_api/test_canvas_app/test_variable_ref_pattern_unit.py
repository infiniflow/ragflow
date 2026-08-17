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
Regression tests for `ComponentBase.variable_ref_patt` and its
pre-compiled sibling `variable_ref_patt_re`.

These guard against the runtime template-substitution regex silently
failing on real-world component ids emitted by the frontend (and on
legacy colon-bearing DSL ids that still appear in test fixtures and
templates).

History
-------

- #16758 — the regex accepted `[a-zA-Z:0-9]+` for the `cpn_id` half,
  which dropped component ids that contain underscores
  (e.g. `userfillup_abc`, `retrieval_xyz`). When that happens an
  Agent's user prompt such as `"Repeat: {userfillup_abc@line}"` is
  left literal and the LLM responds to its system prompt directive
  only.
- An earlier attempt at the fix widened `cpn_id` to `[a-zA-Z0-9_]+`,
  which accidentally dropped colon support. Colon-bearing ids
  (`UserFillUp:CateInput`, `Retrieval:KBSearch`) are real and used
  inside templates + DSL fixtures
  (e.g. `internal/agent/dsl/testdata/all.json`,
  `test/testcases/test_web_api/test_canvas_app/test_iteration_runtime_unit.py`).

The current shape is `[a-zA-Z0-9_:]+` for `cpn_id`, which is a strict
superset of both pre-existing shapes and matches the
`VARIABLE_REF_PATTERN` used by `agent.dsl_migration` for the same
purpose.

These tests pin four contracts:

1. `cpn_id` accepts both underscores (`userfillup_abc`) and colons
   (`UserFillUp:CateInput`).
2. The pre-compiled `variable_ref_patt_re` stays consistent with the
   source pattern string (so a future edit to one cannot drift from
   the other silently — closes CR's "centralize the regex" note).
3. Helper methods (`get_input_elements_from_text`, `string_format`)
   actually use that regex end-to-end.
4. Bare `{line}` (no cpn_id prefix) remains unmatched by design so the
   literal text surfaces to the user until they wire it up.
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
    the symbols we exercise (`variable_ref_patt`, `variable_ref_patt_re`,
    `get_input_elements_from_text`, `string_format`) do not touch it.
    We stub `pandas` before loading so the test stays runnable on
    minimal test environments.

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
    """Frontend-emitted ids like `userfillup_abc@line` must be recognised.

    Regression for the original #16758 underscope fix.
    """
    patt = base_module.ComponentBase.variable_ref_patt_re

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
def test_variable_ref_patt_matches_colon_bearing_component_ids(base_module):
    """Legacy DSL ids like `UserFillUp:CateInput@text` must still resolve.

    Regression for the "Keep colon support" follow-up note from CR on #16792:
    these ids are scattered across ``internal/agent/dsl/testdata/all.json``,
    ``test_iteration_runtime_unit.py``, and other templates. Dropping the
    colon would leave them literal at runtime, silently breaking template
    substitution.
    """
    patt = base_module.ComponentBase.variable_ref_patt_re

    cases = [
        ("{UserFillUp:CateInput@text}", "UserFillUp:CateInput@text"),
        ("{UserFillUp:CodeInput@x}", "UserFillUp:CodeInput@x"),
        ("{UserFillUp:LoopInput@value}", "UserFillUp:LoopInput@value"),
        ("{Retrieval:KBSearch@formalized_content}", "Retrieval:KBSearch@formalized_content"),
        ("{CodeExec:Double@result}", "CodeExec:Double@result"),
        # Mixed underscores + colons (just in case).
        ("{Browser:BusyHatsSink@content}", "Browser:BusyHatsSink@content"),
    ]

    for text, expected in cases:
        matches = list(patt.finditer(text))
        assert matches, (
            f"Expected {text!r} to match variable_ref_patt — colon-bearing "
            f"cqn_id lost its support."
        )
        assert matches[0].group(1) == expected, (
            f"{text!r}: wrong capture — got {matches[0].group(1)!r}, "
            f"expected {expected!r}"
        )


@pytest.mark.p2
def test_variable_ref_patt_still_matches_legacy_ids(base_module):
    """Backward-compat: legacy ids without underscores/colons must still
    resolve."""
    patt = base_module.ComponentBase.variable_ref_patt_re

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
def test_variable_ref_patt_re_matches_variable_ref_patt(base_module):
    """The pre-compiled regex must be built from `variable_ref_patt`.

    Closes CodeRabbit's "centralize the regex pattern" note on #16792:
    the source pattern string and the pre-compiled regex object must
    agree, so a future edit can't make one drift from the other
    silently.
    """
    patt_str = base_module.ComponentBase.variable_ref_patt

    rebuilt = re.compile(patt_str, flags=re.IGNORECASE | re.DOTALL)
    canonical = base_module.ComponentBase.variable_ref_patt_re

    # Same source pattern & flags.
    assert canonical.pattern == rebuilt.pattern, (
        "variable_ref_patt_re must be compiled from variable_ref_patt "
        "(patterns differ)."
    )
    assert canonical.flags == rebuilt.flags, (
        "variable_ref_patt_re flags changed unexpectedly."
    )

    # Same match positions / groups on a representative sample.
    sample = (
        "Repeat: {userfillup_abc@line} / also {Retrieval:KBSearch@f} / "
        "sys={sys.query}"
    )

    canonical_matches = [
        (m.start(), m.end(), m.group(1)) for m in canonical.finditer(sample)
    ]
    rebuilt_matches = [
        (m.start(), m.end(), m.group(1)) for m in rebuilt.finditer(sample)
    ]
    assert canonical_matches == rebuilt_matches, (
        "variable_ref_patt_re produces different matches than a fresh "
        "compile of variable_ref_patt — they have silently diverged."
    )


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
    patt = base_module.ComponentBase.variable_ref_patt_re
    matches = list(patt.finditer("{line}"))
    assert not matches, (
        "Bare `{line}` should not match — only `cpn_id@var` / `sys.*` / "
        "`env.*` are valid template refs."
    )

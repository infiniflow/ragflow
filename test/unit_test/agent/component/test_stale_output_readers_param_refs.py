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
"""Issue #19360 — Switch / VariableAssigner / Iteration / ListOperations
read stale outputs when their sibling producer shares the batch window.

The fix adds a `param_refs()` override to each of those four components so
their cross-component references reach `ComponentBase.get_dependency_ids()` and
the batch scheduler defers them behind the producer. `get_dependency_ids` is
also taught to strip brace-wrapped refs (`{{producer@output}}`) so a
brace-wrapped ref resolves to the same component id the runtime lookup will
use.

These tests construct each component without going through the canvas
constructor (`object.__new__` + a SimpleNamespace stand-in for the canvas)
so the assertions stay focused on `param_refs()` and `get_dependency_ids()`
behaviour rather than the full scheduler path.
"""

from types import SimpleNamespace

import pytest

from agent.component.base import ComponentBase
from agent.component.iteration import Iteration, IterationParam
from agent.component.list_operations import ListOperations, ListOperationsParam
from agent.component.switch import Switch, SwitchParam
from agent.component.variable_assigner import VariableAssigner, VariableAssignerParam


def _new_component(component_cls, param):
    """Bypass the canvas-bound `__init__` and inject the bare minimum state."""
    instance = object.__new__(component_cls)
    instance._canvas = SimpleNamespace()  # the param_refs path does not touch it
    instance._id = "self:0"
    instance._param = param
    return instance


# Test helpers -----------------------------------------------------------


def _switch(conditions, end_cpn_ids=None):
    p = SwitchParam()
    p.conditions = conditions
    p.end_cpn_ids = end_cpn_ids or []
    return _new_component(Switch, p)


def _variable_assigner(variables):
    p = VariableAssignerParam()
    p.variables = variables
    return _new_component(VariableAssigner, p)


def _iteration(items_ref):
    p = IterationParam()
    p.items_ref = items_ref
    return _new_component(Iteration, p)


def _list_operations(query):
    p = ListOperationsParam()
    p.query = query
    return _new_component(ListOperations, p)


# Switch.param_refs() ----------------------------------------------------


class TestSwitchParamRefs:
    def test_empty_conditions_returns_empty(self):
        assert _switch([]).param_refs() == []

    def test_condition_items_cpn_id_is_declared(self):
        c = _switch(
            [
                {"logical_operator": "and", "items": [{"cpn_id": "retrieval:0", "operator": "empty", "value": ""}], "to": []},
            ]
        )
        assert c.param_refs() == ["retrieval:0"]

    def test_multiple_items_across_multiple_conditions(self):
        c = _switch(
            [
                {
                    "logical_operator": "or",
                    "items": [
                        {"cpn_id": "llm:0", "operator": "contains", "value": "yes"},
                        {"cpn_id": "categorize:1", "operator": "empty", "value": ""},
                    ],
                    "to": [],
                },
                {
                    "logical_operator": "and",
                    "items": [
                        {"cpn_id": "retrieval:0", "operator": "not empty", "value": ""},
                    ],
                    "to": [],
                },
            ]
        )
        assert c.param_refs() == ["llm:0", "categorize:1", "retrieval:0"]

    def test_empty_cpn_id_in_item_is_skipped(self):
        c = _switch(
            [
                {
                    "logical_operator": "and",
                    "items": [
                        {"cpn_id": "", "operator": "empty", "value": ""},
                        {"cpn_id": "retrieval:0", "operator": "not empty", "value": ""},
                    ],
                    "to": [],
                },
            ]
        )
        assert c.param_refs() == ["retrieval:0"]

    def test_end_cpn_ids_are_not_declared_as_input_deps(self):
        # `to` / `end_cpn_ids` are output routing, not inputs the
        # condition reads — they must NOT enter the dependency list.
        c = _switch(
            conditions=[{"logical_operator": "and", "items": [{"cpn_id": "llm:0", "operator": "empty", "value": ""}], "to": ["answer:0"]}],
            end_cpn_ids=["fallback:0"],
        )
        assert "answer:0" not in c.param_refs()
        assert "fallback:0" not in c.param_refs()


# VariableAssigner.param_refs() ------------------------------------------


class TestVariableAssignerParamRefs:
    def test_empty_variables_returns_empty(self):
        assert _variable_assigner([]).param_refs() == []

    def test_variable_and_parameter_are_both_declared(self):
        c = _variable_assigner(
            [
                {"variable": "user_input", "operator": "overwrite", "parameter": "llm:0@output"},
            ]
        )
        assert c.param_refs() == ["user_input", "llm:0@output"]

    def test_missing_parameter_is_just_variable(self):
        c = _variable_assigner(
            [
                {"variable": "counter", "operator": "clear"},
            ]
        )
        assert c.param_refs() == ["counter"]

    @pytest.mark.parametrize("operator", ["clear", "remove_first", "remove_last"])
    def test_unused_parameter_is_not_a_dependency(self, operator):
        c = _variable_assigner(
            [{"variable": "user_input", "operator": operator, "parameter": "retrieval:0@output"}]
        )
        assert c.param_refs() == ["user_input"]
        assert c.get_dependency_ids() == []

    def test_multiple_rows_aggregate(self):
        c = _variable_assigner(
            [
                {"variable": "v1", "operator": "set", "parameter": "p1"},
                {"variable": "v2", "operator": "append", "parameter": "p2@output"},
            ]
        )
        assert c.param_refs() == ["v1", "p1", "v2", "p2@output"]


# Iteration.param_refs() -------------------------------------------------


class TestIterationParamRefs:
    def test_empty_items_ref_returns_empty(self):
        # Static-array Iteration has no producer to defer against.
        assert _iteration("").param_refs() == []

    def test_producer_reference_is_declared(self):
        assert _iteration("retrieval:0@output").param_refs() == ["retrieval:0@output"]

    def test_canvas_global_is_declared(self):
        # A bare canvas-global ref is still a ref; get_dependency_ids
        # will skip the `@` split and leave it out of the deps, which is
        # correct — globals are not component deps.
        assert _iteration("user_query").param_refs() == ["user_query"]


# ListOperations.param_refs() --------------------------------------------


class TestListOperationsParamRefs:
    def test_empty_query_returns_empty(self):
        assert _list_operations("").param_refs() == []

    def test_producer_reference_is_declared(self):
        assert _list_operations("retrieval:0@output").param_refs() == ["retrieval:0@output"]


# ComponentBase.get_dependency_ids() normalisation -----------------------


class TestGetDependencyIdsBraceStrip:
    @pytest.fixture(autouse=True)
    def _stub_input_elements(self):
        # Replace get_input_elements so we don't need a real canvas.
        def _no_inputs(_self):
            return {}

        self._orig = ComponentBase.get_input_elements
        ComponentBase.get_input_elements = _no_inputs
        try:
            yield
        finally:
            ComponentBase.get_input_elements = self._orig

    def _deps(self, instance):
        return instance.get_dependency_ids()

    def test_bare_ref_skipped(self):
        # Globals are not component deps; get_dependency_ids skips them.
        c = _iteration("user_query")
        assert self._deps(c) == []

    def test_component_at_ref_resolves_to_cpn_id(self):
        c = _iteration("retrieval:0@output")
        assert self._deps(c) == ["retrieval:0"]

    def test_brace_wrapped_ref_resolves_to_cpn_id(self):
        # The runtime lookup (`Canvas.get_variable_value`) already strips
        # braces; get_dependency_ids must do the same or a brace-wrapped
        # ref like `{{producer@output}}` would resolve to `{{producer`
        # and the scheduler would not defer this node.
        c = _iteration("{{retrieval:0@output}}")
        assert self._deps(c) == ["retrieval:0"]

    def test_brace_with_whitespace_still_strips(self):
        c = _iteration("  {{  retrieval:0@output  }}  ")
        assert self._deps(c) == ["retrieval:0"]

    def test_embedded_ref_uses_atomic_producer(self):
        c = _variable_assigner(
            [{"variable": "user_input", "operator": "set", "parameter": "prefix {{retrieval:0@output}}"}]
        )
        assert self._deps(c) == ["retrieval:0"]

    def test_multiple_embedded_refs(self):
        c = _variable_assigner(
            [{"variable": "user_input", "operator": "set", "parameter": "{{retrieval:0@output}} and {{llm:0@output}}"}]
        )
        assert self._deps(c) == ["retrieval:0", "llm:0"]

    def test_brace_only_no_at_skipped(self):
        # A bare-global inside braces still has no producer, so no dep.
        c = _iteration("{{user_query}}")
        assert self._deps(c) == []

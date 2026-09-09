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

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


def _load_list_operations_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    agent_pkg = ModuleType("agent")
    agent_pkg.__path__ = [str(repo_root / "agent")]
    monkeypatch.setitem(sys.modules, "agent", agent_pkg)

    component_pkg = ModuleType("agent.component")
    component_pkg.__path__ = [str(repo_root / "agent" / "component")]
    monkeypatch.setitem(sys.modules, "agent.component", component_pkg)

    base_mod = ModuleType("agent.component.base")

    class _ComponentParamBase:
        def __init__(self):
            self.outputs = {}

        def check_empty(self, *_args, **_kwargs):
            return None

        def check_valid_value(self, *_args, **_kwargs):
            return None

    class _ComponentBase:
        def set_input_value(self, *_args, **_kwargs):
            return None

    base_mod.ComponentBase = _ComponentBase
    base_mod.ComponentParamBase = _ComponentParamBase
    monkeypatch.setitem(sys.modules, "agent.component.base", base_mod)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    api_utils_mod = ModuleType("api.utils.api_utils")
    api_utils_mod.timeout = lambda *_args, **_kwargs: lambda func: func
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    module_path = repo_root / "agent" / "component" / "list_operations.py"
    spec = importlib.util.spec_from_file_location("test_list_operations_unit_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_list_operations_unit_module", module)
    spec.loader.exec_module(module)
    return module


def _make_component(module, *, inputs, operation, n, strict=False):
    component = module.ListOperations.__new__(module.ListOperations)
    component.inputs = inputs
    component._param = SimpleNamespace(
        n=n,
        strict=strict,
        outputs={
            "result": {"value": []},
            "first": {"value": None},
            "last": {"value": None},
        },
    )
    return component


@pytest.mark.p2
@pytest.mark.parametrize(
    ("n", "expected"),
    [
        (0, []),
        (-1, ["e"]),
        (-5, ["a"]),
        (-6, []),
        (2, ["b"]),
        (5, ["e"]),
        (6, []),
    ],
)
def test_nth_behaves_like_lenient_indexing(monkeypatch, n, expected):
    module = _load_list_operations_module(monkeypatch)
    component = _make_component(module, inputs=["a", "b", "c", "d", "e"], operation="nth", n=n)
    component._nth()
    assert component._param.outputs["result"]["value"] == expected


@pytest.mark.p2
@pytest.mark.parametrize(
    ("strict", "n", "expected"),
    [
        (False, 0, []),
        (False, 2, ["a", "b"]),
        (False, 10, ["a", "b", "c", "d", "e"]),
        (True, 2, ["a", "b"]),
    ],
)
def test_head_supports_lenient_and_strict(monkeypatch, strict, n, expected):
    module = _load_list_operations_module(monkeypatch)
    component = _make_component(module, inputs=["a", "b", "c", "d", "e"], operation="head", n=n, strict=strict)
    component._head()
    assert component._param.outputs["result"]["value"] == expected


@pytest.mark.p2
@pytest.mark.parametrize("n", [0, 10])
def test_head_strict_raises_for_out_of_range(monkeypatch, n):
    module = _load_list_operations_module(monkeypatch)
    component = _make_component(module, inputs=["a", "b", "c", "d", "e"], operation="head", n=n, strict=True)
    with pytest.raises(ValueError, match="head requires n"):
        component._head()


@pytest.mark.p2
@pytest.mark.parametrize(
    ("strict", "n", "expected"),
    [
        (False, 0, []),
        (False, 2, ["d", "e"]),
        (False, 10, ["a", "b", "c", "d", "e"]),
        (True, 2, ["d", "e"]),
    ],
)
def test_tail_supports_lenient_and_strict(monkeypatch, strict, n, expected):
    module = _load_list_operations_module(monkeypatch)
    component = _make_component(module, inputs=["a", "b", "c", "d", "e"], operation="tail", n=n, strict=strict)
    component._tail()
    assert component._param.outputs["result"]["value"] == expected


@pytest.mark.p2
@pytest.mark.parametrize("n", [0, 10])
def test_tail_strict_raises_for_out_of_range(monkeypatch, n):
    module = _load_list_operations_module(monkeypatch)
    component = _make_component(module, inputs=["a", "b", "c", "d", "e"], operation="tail", n=n, strict=True)
    with pytest.raises(ValueError, match="tail requires n"):
        component._tail()


@pytest.mark.p2
@pytest.mark.parametrize("n", [0, 6, -6])
def test_nth_strict_raises_for_out_of_range(monkeypatch, n):
    module = _load_list_operations_module(monkeypatch)
    component = _make_component(module, inputs=["a", "b", "c", "d", "e"], operation="nth", n=n, strict=True)
    with pytest.raises(ValueError, match="nth requires n"):
        component._nth()


@pytest.mark.p2
def test_set_outputs_tracks_first_and_last(monkeypatch):
    module = _load_list_operations_module(monkeypatch)
    component = _make_component(module, inputs=["a", "b", "c", "d", "e"], operation="tail", n=3)
    component._tail()
    assert component._param.outputs["result"]["value"] == ["c", "d", "e"]
    assert component._param.outputs["first"]["value"] == "c"
    assert component._param.outputs["last"]["value"] == "e"


@pytest.mark.p2
def test_topn_operation_alias_normalizes_to_head(monkeypatch):
    module = _load_list_operations_module(monkeypatch)
    param = module.ListOperationsParam()
    param.query = "items"
    param.operations = "topN"
    param.check()
    assert param.operations == "head"


def _make_invoke_component(module, *, resolved):
    component = module.ListOperations.__new__(module.ListOperations)
    component._canvas = SimpleNamespace(get_variable_value=lambda _ref: resolved)
    component._param = SimpleNamespace(
        query="cpn_0@result",
        operations="nth",
        n=1,
        strict=False,
        outputs={
            "result": {"value": []},
            "first": {"value": None},
            "last": {"value": None},
        },
    )
    return component


@pytest.mark.p2
def test_invoke_treats_missing_variable_as_empty_list(monkeypatch):
    module = _load_list_operations_module(monkeypatch)
    component = _make_invoke_component(module, resolved=None)
    component._invoke()
    assert component.inputs == []
    assert component._param.outputs["result"]["value"] == []
    assert component._param.outputs["first"]["value"] is None
    assert component._param.outputs["last"]["value"] is None


@pytest.mark.p2
def test_invoke_still_raises_for_non_list_input(monkeypatch):
    module = _load_list_operations_module(monkeypatch)
    component = _make_invoke_component(module, resolved="not-a-list")
    with pytest.raises(TypeError, match="should be an array"):
        component._invoke()


def _make_sort_component(module, *, inputs, sort_method="asc", sort_by=""):
    component = module.ListOperations.__new__(module.ListOperations)
    component.inputs = inputs
    component._param = SimpleNamespace(
        sort_method=sort_method,
        sort_by=sort_by,
        outputs={
            "result": {"value": []},
            "first": {"value": None},
            "last": {"value": None},
        },
    )
    return component


@pytest.mark.p2
def test_sort_by_tolerates_missing_field_values(monkeypatch):
    module = _load_list_operations_module(monkeypatch)
    items = [{"name": "b", "score": None}, {"name": "a", "score": 5}, {"name": "c", "score": 3}]
    component = _make_sort_component(module, inputs=items, sort_by="score")
    component._sort()
    assert [i["name"] for i in component._param.outputs["result"]["value"]] == ["c", "a", "b"]


@pytest.mark.p2
def test_sort_by_tolerates_mixed_value_types(monkeypatch):
    module = _load_list_operations_module(monkeypatch)
    items = [{"k": 10}, {"k": "9"}, {"k": 2}]
    component = _make_sort_component(module, inputs=items, sort_by="k")
    component._sort()
    assert [i["k"] for i in component._param.outputs["result"]["value"]] == [2, 10, "9"]


@pytest.mark.p2
def test_sort_by_tolerates_non_dict_items(monkeypatch):
    module = _load_list_operations_module(monkeypatch)
    items = [{"k": 1}, "plain", {"k": 0}]
    component = _make_sort_component(module, inputs=items, sort_by="k")
    component._sort()
    result = component._param.outputs["result"]["value"]
    assert result[0] == {"k": 0}
    assert result[1] == {"k": 1}
    assert result[2] == "plain"


@pytest.mark.p2
def test_sort_plain_list_tolerates_mixed_scalars(monkeypatch):
    module = _load_list_operations_module(monkeypatch)
    component = _make_sort_component(module, inputs=[1, "a", None, 2])
    component._sort()
    assert component._param.outputs["result"]["value"] == [1, 2, None, "a"]


@pytest.mark.p2
def test_sort_legacy_hashable_path_tolerates_mixed_values(monkeypatch):
    module = _load_list_operations_module(monkeypatch)
    component = _make_sort_component(module, inputs=[{"a": 1}, {"a": "x"}, {"a": 0}])
    component._sort()
    assert [i["a"] for i in component._param.outputs["result"]["value"]] == [0, 1, "x"]


@pytest.mark.p2
def test_sort_still_orders_numbers_numerically_desc(monkeypatch):
    module = _load_list_operations_module(monkeypatch)
    component = _make_sort_component(module, inputs=[3, 10, 2], sort_method="desc")
    component._sort()
    assert component._param.outputs["result"]["value"] == [10, 3, 2]


@pytest.mark.p2
def test_sort_by_multi_key_still_applies_tiebreak(monkeypatch):
    module = _load_list_operations_module(monkeypatch)
    items = [{"a": 1, "b": 2}, {"a": 1, "b": 1}, {"a": 0, "b": 9}]
    component = _make_sort_component(module, inputs=items, sort_by="a,b")
    component._sort()
    assert [(i["a"], i["b"]) for i in component._param.outputs["result"]["value"]] == [(0, 9), (1, 1), (1, 2)]


@pytest.mark.p2
def test_sort_matches_go_text_order_for_mixed_number_and_string(monkeypatch):
    # Go lessScalar compares a number/string pair by text, so "10" sorts
    # before 2 (internal/agent/component/list_operations.go).
    module = _load_list_operations_module(monkeypatch)
    component = _make_sort_component(module, inputs=[2, "10"])
    component._sort()
    assert component._param.outputs["result"]["value"] == ["10", 2]


@pytest.mark.p2
def test_sort_orders_none_by_go_nil_text_form(monkeypatch):
    # Go renders nil as "<nil>", which sorts after digit text and before
    # letters.
    module = _load_list_operations_module(monkeypatch)
    component = _make_sort_component(module, inputs=[None, "a", "10"])
    component._sort()
    assert component._param.outputs["result"]["value"] == ["10", None, "a"]


@pytest.mark.p2
def test_sort_treats_bool_as_text_not_number(monkeypatch):
    # Mirrors Go's toFloat64OK, which excludes booleans from numeric ordering.
    module = _load_list_operations_module(monkeypatch)
    component = _make_sort_component(module, inputs=[True, 1])
    component._sort()
    assert component._param.outputs["result"]["value"] == [1, True]


@pytest.mark.p2
def test_sort_legacy_path_tolerates_nested_mixed_set(monkeypatch):
    # Raw sorted() inside the legacy canonicalization raised TypeError on
    # mixed set members; the comparator-based canonical form never raises.
    module = _load_list_operations_module(monkeypatch)
    component = _make_sort_component(module, inputs=[{"tags": {1, "x"}}, {"tags": {2}}])
    component._sort()
    assert component._param.outputs["result"]["value"] == [{"tags": {1, "x"}}, {"tags": {2}}]


@pytest.mark.p2
def test_sort_legacy_path_tolerates_mixed_dict_keys(monkeypatch):
    module = _load_list_operations_module(monkeypatch)
    component = _make_sort_component(module, inputs=[{1: "v", "k": "w"}, {"a": 1}])
    component._sort()
    # Deterministic, no TypeError: the nested canonical form compares first.
    assert component._param.outputs["result"]["value"] == [{1: "v", "k": "w"}, {"a": 1}]

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
    api_utils_mod.timeout = lambda *_args, **_kwargs: (lambda func: func)
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    module_path = repo_root / "agent" / "component" / "list_operations.py"
    spec = importlib.util.spec_from_file_location(
        "test_list_operations_unit_module", module_path
    )
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
    component = _make_component(
        module, inputs=["a", "b", "c", "d", "e"], operation="nth", n=n
    )
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
    component = _make_component(
        module, inputs=["a", "b", "c", "d", "e"], operation="head", n=n, strict=strict
    )
    component._head()
    assert component._param.outputs["result"]["value"] == expected


@pytest.mark.p2
@pytest.mark.parametrize("n", [0, 10])
def test_head_strict_raises_for_out_of_range(monkeypatch, n):
    module = _load_list_operations_module(monkeypatch)
    component = _make_component(
        module, inputs=["a", "b", "c", "d", "e"], operation="head", n=n, strict=True
    )
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
    component = _make_component(
        module, inputs=["a", "b", "c", "d", "e"], operation="tail", n=n, strict=strict
    )
    component._tail()
    assert component._param.outputs["result"]["value"] == expected


@pytest.mark.p2
@pytest.mark.parametrize("n", [0, 10])
def test_tail_strict_raises_for_out_of_range(monkeypatch, n):
    module = _load_list_operations_module(monkeypatch)
    component = _make_component(
        module, inputs=["a", "b", "c", "d", "e"], operation="tail", n=n, strict=True
    )
    with pytest.raises(ValueError, match="tail requires n"):
        component._tail()


@pytest.mark.p2
@pytest.mark.parametrize("n", [0, 6, -6])
def test_nth_strict_raises_for_out_of_range(monkeypatch, n):
    module = _load_list_operations_module(monkeypatch)
    component = _make_component(
        module, inputs=["a", "b", "c", "d", "e"], operation="nth", n=n, strict=True
    )
    with pytest.raises(ValueError, match="nth requires n"):
        component._nth()


@pytest.mark.p2
def test_set_outputs_tracks_first_and_last(monkeypatch):
    module = _load_list_operations_module(monkeypatch)
    component = _make_component(
        module, inputs=["a", "b", "c", "d", "e"], operation="tail", n=3
    )
    component._tail()
    assert component._param.outputs["result"]["value"] == ["c", "d", "e"]
    assert component._param.outputs["first"]["value"] == "c"
    assert component._param.outputs["last"]["value"] == "e"

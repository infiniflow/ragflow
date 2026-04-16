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
from unittest.mock import MagicMock

import pytest


def _load_iterationitem_module(monkeypatch):
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

    conn_spec = importlib.util.spec_from_file_location(
        "common.connection_utils", repo_root / "common" / "connection_utils.py"
    )
    conn_mod = importlib.util.module_from_spec(conn_spec)
    monkeypatch.setitem(sys.modules, "common.connection_utils", conn_mod)
    conn_spec.loader.exec_module(conn_mod)

    misc_spec = importlib.util.spec_from_file_location(
        "common.misc_utils", repo_root / "common" / "misc_utils.py"
    )
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

    base_spec = importlib.util.spec_from_file_location(
        "agent.component.base", repo_root / "agent" / "component" / "base.py"
    )
    base_mod = importlib.util.module_from_spec(base_spec)
    monkeypatch.setitem(sys.modules, "agent.component.base", base_mod)
    base_spec.loader.exec_module(base_mod)

    iterationitem_spec = importlib.util.spec_from_file_location(
        "agent.component.iterationitem",
        repo_root / "agent" / "component" / "iterationitem.py",
    )
    iterationitem_mod = importlib.util.module_from_spec(iterationitem_spec)
    monkeypatch.setitem(
        sys.modules, "agent.component.iterationitem", iterationitem_mod
    )
    iterationitem_spec.loader.exec_module(iterationitem_mod)

    return iterationitem_mod


def _make_iterationitem(module, values):
    canvas = MagicMock()
    canvas.is_canceled = MagicMock(return_value=False)
    canvas.get_variable_value = MagicMock(return_value=values)
    canvas.components = {}

    param = module.IterationItemParam()
    param.outputs = {}
    param.inputs = {}

    inst = module.IterationItem.__new__(module.IterationItem)
    inst._canvas = canvas
    inst._id = "IterationItem:test"
    inst._param = param
    inst._idx = 0
    inst.get_parent = MagicMock(
        return_value=SimpleNamespace(
            _id="Iteration:test",
            _param=SimpleNamespace(items_ref="code:1@tempList"),
            component_name="Iteration",
        )
    )
    return inst


@pytest.mark.p2
def test_iterationitem_exposes_result_alias_for_each_item(monkeypatch):
    module = _load_iterationitem_module(monkeypatch)
    item = _make_iterationitem(module, ["a", "b", "c"])

    item._invoke()
    assert item.output("item") == "a"
    assert item.output("result") == "a"
    assert item.output("index") == 0

    item._invoke()
    assert item.output("item") == "b"
    assert item.output("result") == "b"
    assert item.output("index") == 1

    item._invoke()
    assert item.output("item") == "c"
    assert item.output("result") == "c"
    assert item.output("index") == 2

    item._invoke()
    assert item.end() is True

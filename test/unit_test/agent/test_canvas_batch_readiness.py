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
"""Batch readiness in the `Canvas` scheduler (issue #19265).

`_run_impl` dispatches `path[idx:to]` concurrently, so a node in that window may
only read output written by a component that finished in an earlier batch. These
tests drive the real scheduler over graphs whose branches have unequal depth,
which is what puts a join node in the same window as one of its upstreams.

The heavyweight edges (LLM service, task service, Redis, TTS) are stubbed and the
real `agent/canvas.py`, `agent/component/base.py` and
`agent/component/variable_aggregator.py` are loaded by path, the same isolation
strategy as `test/unit_test/agent/test_canvas_at_split.py`.
"""

from __future__ import annotations

import asyncio
import importlib.util
import json
import sys
import threading
import time
from pathlib import Path
from types import ModuleType
from unittest.mock import MagicMock

import pytest

REPO_ROOT = Path(__file__).resolve().parents[3]


def _load_canvas_stack(monkeypatch):
    def _pkg(name, path):
        mod = ModuleType(name)
        mod.__path__ = [str(path)]
        monkeypatch.setitem(sys.modules, name, mod)
        return mod

    def _stub(name, **attrs):
        mod = ModuleType(name)
        for key, value in attrs.items():
            setattr(mod, key, value)
        monkeypatch.setitem(sys.modules, name, mod)
        return mod

    def _real(name, relpath):
        spec = importlib.util.spec_from_file_location(name, REPO_ROOT / relpath)
        mod = importlib.util.module_from_spec(spec)
        monkeypatch.setitem(sys.modules, name, mod)
        spec.loader.exec_module(mod)
        return mod

    _pkg("common", REPO_ROOT / "common")
    _pkg("agent", REPO_ROOT / "agent")
    component_pkg = _pkg("agent.component", REPO_ROOT / "agent" / "component")
    _pkg("api", REPO_ROOT / "api")
    _pkg("rag", REPO_ROOT / "rag")

    _stub("common.constants", LLMType=MagicMock(), RetCode=MagicMock())
    _stub("common.connection_utils", timeout=lambda *a, **kw: lambda fn: fn)
    _stub("common.token_utils", token_usage_sink=MagicMock(), langfuse_run_attrs=MagicMock())
    _stub("common.llm_request_context", set_llm_request_context=MagicMock(), reset_llm_request_context=MagicMock())
    _stub("common.exceptions", TaskCanceledException=type("TaskCanceledException", (Exception,), {}))
    _stub("api.db.joint_services.tenant_model_service", get_tenant_default_model_by_type=MagicMock(return_value=None))
    _stub("api.db.services.file_service", FileService=MagicMock())
    _stub("api.db.services.llm_service", LLMBundle=MagicMock())
    _stub("api.db.services.task_service", has_canceled=MagicMock(return_value=False))
    _stub("rag.prompts.generator", chunks_format=MagicMock())
    _stub("rag.utils.redis_conn", REDIS_CONN=MagicMock())
    _stub("rag.utils.tts_cache", synthesize_with_cache=MagicMock())

    _real("common.misc_utils", "common/misc_utils.py")
    _real("agent.settings", "agent/settings.py")
    _real("agent.dsl_migration", "agent/dsl_migration.py")

    base = _real("agent.component.base", "agent/component/base.py")
    component_pkg.base = base

    registry: dict = {}
    component_pkg.component_class = lambda name: registry[name]

    canvas = _real("agent.canvas", "agent/canvas.py")
    aggregator = _real("agent.component.variable_aggregator", "agent/component/variable_aggregator.py")

    _pkg("agent.tools", REPO_ROOT / "agent" / "tools")
    _stub("common.mcp_tool_call_conn", MCPToolBinding=MagicMock(), MCPToolCallSession=MagicMock(), ToolCallSession=MagicMock())
    _stub("common.settings", SANDBOX_HOST="")
    sys.modules["common.constants"].SANDBOX_ARTIFACT_BUCKET = ""
    sys.modules["common.constants"].SANDBOX_ARTIFACT_EXPIRE_DAYS = 1
    sys.modules["rag.prompts.generator"].kb_prompt = MagicMock()
    _real("agent.tools.base", "agent/tools/base.py")
    code_exec = _real("agent.tools.code_exec", "agent/tools/code_exec.py")
    return canvas, base, aggregator, code_exec, registry


class _Trace:
    def __init__(self):
        self.events: list[tuple[str, str, float]] = []
        self._lock = threading.Lock()
        self._t0 = time.perf_counter()

    def record(self, kind, cpn_id):
        with self._lock:
            self.events.append((kind, cpn_id, time.perf_counter() - self._t0))

    def at(self, kind, cpn_id):
        return next(t for k, c, t in self.events if k == kind and c == cpn_id)

    def runs(self, cpn_id):
        return sum(1 for k, c, _ in self.events if k == "end" and c == cpn_id)

    def ran(self, cpn_id):
        return self.runs(cpn_id) > 0


def _make_components(base, aggregator, trace):
    class BeginParam(base.ComponentParamBase):
        def __init__(self):
            super().__init__()
            self.mode = "conversational"
            self.prologue = ""

        def check(self):
            pass

    class Begin(base.ComponentBase):
        component_name = "Begin"

        def thoughts(self) -> str:
            return ""

        def _invoke(self, **kwargs):
            trace.record("start", self._id)
            trace.record("end", self._id)

    class EchoParam(base.ComponentParamBase):
        def __init__(self):
            super().__init__()
            self.text = ""
            self.delay = 0.0
            self.outputs = {"result": {"value": "", "type": "string"}}

        def check(self):
            pass

    class Echo(base.ComponentBase):
        component_name = "Echo"

        def thoughts(self) -> str:
            return ""

        def get_input_elements(self):
            return self.get_input_elements_from_text(self._param.text)

        def _invoke(self, **kwargs):
            trace.record("start", self._id)
            value = self._canvas.get_value_with_variable(self._param.text)
            if self._param.delay:
                time.sleep(self._param.delay)
            self.set_output("result", value)
            trace.record("end", self._id)

    class TracedAggregator(aggregator.VariableAggregator):
        component_name = "VariableAggregator"

        def _invoke(self, **kwargs):
            trace.record("start", self._id)
            super()._invoke(**kwargs)
            trace.record("end", self._id)

    return {
        "Begin": Begin,
        "BeginParam": BeginParam,
        "Echo": Echo,
        "EchoParam": EchoParam,
        "VariableAggregator": TracedAggregator,
        "VariableAggregatorParam": aggregator.VariableAggregatorParam,
    }


def _node(name, params, downstream, upstream):
    return {"obj": {"component_name": name, "params": params}, "downstream": downstream, "upstream": upstream}


def _dsl(components):
    return json.dumps(
        {
            "components": components,
            "history": [],
            "retrieval": [],
            "memory": [],
            "path": [],
            "globals": {"sys.query": "", "sys.user_id": "u", "sys.conversation_turns": 0, "sys.files": [], "sys.history": []},
        }
    )


def _aggregator_graph(deep_branches):
    """begin fans out to two branches, both joined by a VariableAggregator.

    deep_branches=1 gives the reporter's broken shape (branch one has an extra
    node); deep_branches=2 gives the balanced shape that already worked.
    """
    tail = "d" if deep_branches == 2 else "b"
    components = {
        "begin": _node("Begin", {}, ["a", "b"], []),
        "a": _node("Echo", {"text": "{sys.query}"}, ["c"], ["begin"]),
        "c": _node("Echo", {"text": "{a@result}-c", "delay": 0.30}, ["agg"], ["a"]),
        "b": _node("Echo", {"text": "+test"}, ["d"] if deep_branches == 2 else ["agg"], ["begin"]),
        "agg": _node(
            "VariableAggregator",
            {"groups": [{"group_name": "g1", "variables": [{"value": "c@result"}]}, {"group_name": "g2", "variables": [{"value": f"{tail}@result"}]}]},
            [],
            ["c", tail],
        ),
    }
    if deep_branches == 2:
        components["d"] = _node("Echo", {"text": "{b@result}-d"}, ["agg"], ["b"])
    return _dsl(components)


def _echo_join_graph():
    return _dsl(
        {
            "begin": _node("Begin", {}, ["a", "b"], []),
            "a": _node("Echo", {"text": "{sys.query}"}, ["c"], ["begin"]),
            "c": _node("Echo", {"text": "{a@result}-c", "delay": 0.30}, ["join"], ["a"]),
            "b": _node("Echo", {"text": "+test"}, ["join"], ["begin"]),
            "join": _node("Echo", {"text": "[{c@result}][{b@result}]"}, [], ["c", "b"]),
        }
    )


def _unreachable_dependency_graph():
    return _dsl(
        {
            "begin": _node("Begin", {}, ["a"], []),
            "a": _node("Echo", {"text": "{sys.query}"}, ["sink"], ["begin"]),
            "sink": _node("Echo", {"text": "[{ghost@result}]"}, [], ["ghost"]),
            "ghost": _node("Echo", {"text": "never scheduled"}, [], []),
        }
    )


def _dropped_upstream_graph():
    """`a` and `b` share a window, `a` waits on a node nobody scheduled."""
    return _dsl(
        {
            "begin": _node("Begin", {}, ["a", "b"], []),
            "a": _node("Echo", {"text": "[{ghost@result}]"}, ["b"], ["ghost"]),
            "b": _node("Echo", {"text": "[{a@result}]"}, [], ["a"]),
            "ghost": _node("Echo", {"text": "never scheduled"}, [], []),
        }
    )


def _two_join_graph():
    """Both joins land in the same batch as `c`, so both are held back together."""
    return _dsl(
        {
            "begin": _node("Begin", {}, ["a", "b"], []),
            "a": _node("Echo", {"text": "{sys.query}"}, ["c"], ["begin"]),
            "c": _node("Echo", {"text": "{a@result}-c", "delay": 0.30}, ["j1", "j2"], ["a"]),
            "b": _node("Echo", {"text": "+test"}, ["j1", "j2"], ["begin"]),
            "j1": _node("Echo", {"text": "1[{c@result}][{b@result}]"}, [], ["c", "b"]),
            "j2": _node("Echo", {"text": "2[{c@result}][{b@result}]"}, [], ["c", "b"]),
        }
    )


def _mutually_referencing_graph():
    return _dsl(
        {
            "begin": _node("Begin", {}, ["x", "y"], []),
            "x": _node("Echo", {"text": "[{y@result}]"}, [], ["begin"]),
            "y": _node("Echo", {"text": "[{x@result}]"}, [], ["begin"]),
        }
    )


@pytest.fixture
def canvas_stack(monkeypatch):
    canvas, base, aggregator, code_exec, registry = _load_canvas_stack(monkeypatch)
    trace = _Trace()
    registry.update(_make_components(base, aggregator, trace))
    return canvas, trace, code_exec


def _run(canvas_module, dsl):
    async def _drain():
        graph = canvas_module.Canvas(dsl, tenant_id="t", task_id="task")
        async for _ in graph.run(query="Ragflow"):
            pass
        return graph

    return asyncio.run(_drain())


@pytest.mark.p1
def test_aggregator_waits_for_the_deeper_branch(canvas_stack):
    canvas_module, trace, _ = canvas_stack
    graph = _run(canvas_module, _aggregator_graph(deep_branches=1))
    agg = graph.get_component_obj("agg")

    assert agg.output("g1") == "Ragflow-c"
    assert agg.output("g2") == "+test"
    assert trace.at("start", "agg") >= trace.at("end", "c")


@pytest.mark.p1
def test_join_node_waits_for_the_deeper_branch(canvas_stack):
    canvas_module, trace, _ = canvas_stack
    graph = _run(canvas_module, _echo_join_graph())

    assert graph.get_component_obj("join").output("result") == "[Ragflow-c][+test]"
    assert trace.at("start", "join") >= trace.at("end", "c")


@pytest.mark.p1
def test_balanced_branches_still_run_concurrently(canvas_stack):
    canvas_module, trace, _ = canvas_stack
    graph = _run(canvas_module, _aggregator_graph(deep_branches=2))
    agg = graph.get_component_obj("agg")

    assert agg.output("g1") == "Ragflow-c"
    assert agg.output("g2") == "+test-d"
    # `c` sleeps for 0.3s and `d` does not, so holding the pair back one at a
    # time would push `d` past the end of `c`.
    assert trace.at("start", "d") < trace.at("end", "c")


@pytest.mark.p1
def test_node_whose_upstream_was_never_scheduled_is_dropped(canvas_stack):
    canvas_module, trace, _ = canvas_stack
    _run(canvas_module, _unreachable_dependency_graph())

    assert trace.ran("a")
    assert not trace.ran("sink")


@pytest.mark.p1
def test_a_drop_carries_to_the_node_reading_it(canvas_stack):
    canvas_module, trace, _ = canvas_stack
    _run(canvas_module, _dropped_upstream_graph())

    # `b` is deferred behind `a` and `a` is then dropped, so `b` has nothing to read.
    # Without the cascade it is the only candidate left and the cycle fallback runs it.
    assert not trace.ran("a")
    assert not trace.ran("b")


@pytest.mark.p1
def test_two_joins_held_back_together_each_run_once(canvas_stack):
    canvas_module, trace, _ = canvas_stack
    graph = _run(canvas_module, _two_join_graph())

    assert graph.get_component_obj("j1").output("result") == "1[Ragflow-c][+test]"
    assert graph.get_component_obj("j2").output("result") == "2[Ragflow-c][+test]"
    assert trace.runs("j1") == 1
    assert trace.runs("j2") == 1


@pytest.mark.p1
def test_nodes_that_reference_each_other_fail_instead_of_running(canvas_stack):
    canvas_module, trace, _ = canvas_stack
    graph = _run(canvas_module, _mutually_referencing_graph())

    # Running both would have each read the other's output before it was written.
    assert not trace.ran("x")
    assert not trace.ran("y")
    for cpn_id in ("x", "y"):
        assert "reference each other" in graph.get_component_obj(cpn_id).error()


@pytest.mark.p1
def test_a_cycle_still_terminates_the_run(canvas_stack):
    canvas_module, _, _ = canvas_stack
    graph = canvas_module.Canvas(_mutually_referencing_graph(), tenant_id="t", task_id="task")
    graph.path = ["begin", "x", "y"]

    # The window has to advance past the cycle, or `_run_impl` never leaves it.
    assert graph._schedulable(1, 3) == 3


@pytest.mark.p1
def test_being_unrunnable_once_does_not_disable_the_node(canvas_stack):
    canvas_module, trace, _ = canvas_stack
    graph = canvas_module.Canvas(_echo_join_graph(), tenant_id="t", task_id="task")
    cpn = graph.get_component_obj("b")
    cpn.set_unrunnable("blocked in this batch")

    cpn.invoke(**cpn.get_input())
    assert cpn.error() == "blocked in this batch"

    # A loop back-edge can put the same id in a later batch, where its input is
    # available; the flag describes one dispatch, not the component.
    cpn.invoke(**cpn.get_input())
    assert not cpn.error()
    assert trace.ran("b")


@pytest.mark.p1
def test_resumed_run_keeps_its_stored_order(canvas_stack):
    canvas_module, _, _ = canvas_stack
    graph = canvas_module.Canvas(_echo_join_graph(), tenant_id="t", task_id="task")
    graph.path = ["userfillup_0", "c", "b"]

    assert graph._schedulable(0, 3) == 3
    assert graph.path == ["userfillup_0", "c", "b"]


@pytest.mark.p1
def test_code_node_dependencies_come_from_its_arguments(canvas_stack):
    _, _, code_exec = canvas_stack
    param = code_exec.CodeExecParam()
    param.arguments = {"arg1": "code_1@result", "arg2": "sys.query", "arg3": ""}
    cpn = code_exec.CodeExec.__new__(code_exec.CodeExec)
    cpn._param = param

    assert cpn.get_dependency_ids() == ["code_1"]

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
    """Load the agent canvas stack with lightweight stubs for isolated batch testing."""
    def _pkg(name, path):
        """Register an isolated mock package module in sys.modules."""
        mod = ModuleType(name)
        mod.__path__ = [str(path)]
        monkeypatch.setitem(sys.modules, name, mod)
        return mod

    def _stub(name, **attrs):
        """Register an isolated stub module with mocked attributes in sys.modules."""
        mod = ModuleType(name)
        for key, value in attrs.items():
            setattr(mod, key, value)
        monkeypatch.setitem(sys.modules, name, mod)
        return mod

    def _real(name, relpath):
        """Load a real module from disk into sys.modules."""
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

    _stub("api.utils.api_utils", timeout=lambda *a, **kw: lambda fn: fn)
    base = _real("agent.component.base", "agent/component/base.py")
    component_pkg.base = base

    registry: dict = {}
    component_pkg.component_class = lambda name: registry[name]

    canvas = _real("agent.canvas", "agent/canvas.py")
    aggregator = _real("agent.component.variable_aggregator", "agent/component/variable_aggregator.py")
    switch = _real("agent.component.switch", "agent/component/switch.py")
    assigner = _real("agent.component.variable_assigner", "agent/component/variable_assigner.py")
    iteration = _real("agent.component.iteration", "agent/component/iteration.py")
    list_operations = _real("agent.component.list_operations", "agent/component/list_operations.py")

    _pkg("agent.tools", REPO_ROOT / "agent" / "tools")
    _stub("common.mcp_tool_call_conn", MCPToolBinding=MagicMock(), MCPToolCallSession=MagicMock(), ToolCallSession=MagicMock())
    _stub("common.settings", SANDBOX_HOST="")
    sys.modules["common.constants"].SANDBOX_ARTIFACT_BUCKET = ""
    sys.modules["common.constants"].SANDBOX_ARTIFACT_EXPIRE_DAYS = 1
    sys.modules["rag.prompts.generator"].kb_prompt = MagicMock()
    _real("agent.tools.base", "agent/tools/base.py")
    code_exec = _real("agent.tools.code_exec", "agent/tools/code_exec.py")
    return canvas, base, aggregator, code_exec, switch, assigner, iteration, list_operations, registry


class _Trace:
    """Thread-safe event logger tracking component start and completion times."""

    def __init__(self):
        """Initialize trace event container and start time."""
        self.events: list[tuple[str, str, float]] = []
        self._lock = threading.Lock()
        self._t0 = time.perf_counter()

    def record(self, kind, cpn_id):
        """Record execution timestamp for a component event."""
        with self._lock:
            self.events.append((kind, cpn_id, time.perf_counter() - self._t0))

    def at(self, kind, cpn_id):
        """Return the recorded timestamp for a component event."""
        return next(t for k, c, t in self.events if k == kind and c == cpn_id)

    def runs(self, cpn_id):
        """Return the number of completed runs for a component."""
        return sum(1 for k, c, _ in self.events if k == "end" and c == cpn_id)

    def ran(self, cpn_id):
        """Check if a component completed at least one run."""
        return self.runs(cpn_id) > 0


def _make_components(base, aggregator, trace, switch=None, assigner=None, iteration=None, list_operations=None):
    """Create instrumented test component classes wired to trace recording."""
    class BeginParam(base.ComponentParamBase):
        """Parameter specification for test Begin component."""

        def __init__(self):
            """Initialize Begin component parameters."""
            super().__init__()
            self.mode = "conversational"
            self.prologue = ""

        def check(self):
            """Validate Begin parameters."""
            pass

    class Begin(base.ComponentBase):
        """Test Begin component logging start and end events."""

        component_name = "Begin"

        def thoughts(self) -> str:
            """Return thoughts trace for Begin."""
            return ""

        def _invoke(self, **kwargs):
            """Record execution events for Begin component."""
            trace.record("start", self._id)
            trace.record("end", self._id)

    class EchoParam(base.ComponentParamBase):
        """Parameter specification for test Echo component."""

        def __init__(self):
            """Initialize Echo component parameters."""
            super().__init__()
            self.text = ""
            self.delay = 0.0
            self.outputs = {"result": {"value": "", "type": "string"}}

        def check(self):
            """Validate Echo parameters."""
            pass

    class Echo(base.ComponentBase):
        """Test Echo component outputting interpolated text with optional delay."""

        component_name = "Echo"

        def thoughts(self) -> str:
            """Return thoughts trace for Echo."""
            return ""

        def get_input_elements(self):
            """Extract input element dependencies from text template."""
            return self.get_input_elements_from_text(self._param.text)

        def _invoke(self, **kwargs):
            """Execute Echo component with optional delay and record trace."""
            trace.record("start", self._id)
            value = self._canvas.get_value_with_variable(self._param.text)
            if self._param.delay:
                time.sleep(self._param.delay)
            self.set_output("result", value)
            trace.record("end", self._id)

    class TracedAggregator(aggregator.VariableAggregator):
        """Traced subclass of VariableAggregator recording execution timestamps."""

        component_name = "VariableAggregator"

        def _invoke(self, **kwargs):
            """Execute VariableAggregator and record trace."""
            trace.record("start", self._id)
            super()._invoke(**kwargs)
            trace.record("end", self._id)

    components = {
        "Begin": Begin,
        "BeginParam": BeginParam,
        "Echo": Echo,
        "EchoParam": EchoParam,
        "VariableAggregator": TracedAggregator,
        "VariableAggregatorParam": aggregator.VariableAggregatorParam,
    }

    if switch:
        class TracedSwitch(switch.Switch):
            """Traced subclass of Switch recording execution timestamps."""

            component_name = "Switch"

            def _invoke(self, **kwargs):
                """Execute Switch component and record trace."""
                trace.record("start", self._id)
                super()._invoke(**kwargs)
                trace.record("end", self._id)

        components["Switch"] = TracedSwitch
        components["SwitchParam"] = switch.SwitchParam

    if assigner:
        class TracedVariableAssigner(assigner.VariableAssigner):
            """Traced subclass of VariableAssigner recording execution timestamps."""

            component_name = "VariableAssigner"

            def _invoke(self, **kwargs):
                """Execute VariableAssigner component and record trace."""
                trace.record("start", self._id)
                super()._invoke(**kwargs)
                trace.record("end", self._id)

        components["VariableAssigner"] = TracedVariableAssigner
        components["VariableAssignerParam"] = assigner.VariableAssignerParam

    if iteration:
        class TracedIteration(iteration.Iteration):
            """Traced subclass of Iteration recording execution timestamps."""

            component_name = "Iteration"

            def _invoke(self, **kwargs):
                """Execute Iteration component and record trace."""
                trace.record("start", self._id)
                super()._invoke(**kwargs)
                trace.record("end", self._id)

        components["Iteration"] = TracedIteration
        components["IterationParam"] = iteration.IterationParam

    if list_operations:
        class TracedListOperations(list_operations.ListOperations):
            """Traced subclass of ListOperations recording execution timestamps."""

            component_name = "ListOperations"

            def _invoke(self, **kwargs):
                """Execute ListOperations component and record trace."""
                trace.record("start", self._id)
                super()._invoke(**kwargs)
                trace.record("end", self._id)

        components["ListOperations"] = TracedListOperations
        components["ListOperationsParam"] = list_operations.ListOperationsParam

    return components


def _node(name, params, downstream, upstream):
    """Create a DSL component node dictionary."""
    return {"obj": {"component_name": name, "params": params}, "downstream": downstream, "upstream": upstream}


def _dsl(components):
    """Serialize component graph definitions to Canvas DSL JSON."""
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
    """Create a graph where an Echo join node depends on two unbalanced branches."""
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
    """Create a graph where a sink node depends on an unscheduled ghost node."""
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
    """Create a graph with mutually referencing sibling nodes."""
    return _dsl(
        {
            "begin": _node("Begin", {}, ["x", "y"], []),
            "x": _node("Echo", {"text": "[{y@result}]"}, [], ["begin"]),
            "y": _node("Echo", {"text": "[{x@result}]"}, [], ["begin"]),
        }
    )


def _switch_sibling_graph():
    """Create a graph where Switch condition references a slow sibling producer."""
    return _dsl(
        {
            "begin": _node("Begin", {}, ["producer", "sw"], []),
            "producer": _node("Echo", {"text": "ready", "delay": 0.20}, [], ["begin"]),
            "sw": _node(
                "Switch",
                {
                    "conditions": [
                        {
                            "logical_operator": "and",
                            "items": [{"cpn_id": "producer@result", "operator": "=", "value": "ready"}],
                            "to": ["matched"],
                        }
                    ],
                    "end_cpn_ids": ["else_sink"],
                },
                ["matched", "else_sink"],
                ["begin"],
            ),
            "matched": _node("Echo", {"text": "matched_path"}, [], ["sw"]),
            "else_sink": _node("Echo", {"text": "else_path"}, [], ["sw"]),
        }
    )


def _assigner_sibling_graph():
    """Create a graph where VariableAssigner copies output from a slow sibling producer."""
    return json.dumps(
        {
            "components": {
                "begin": _node("Begin", {}, ["producer", "assigner"], []),
                "producer": _node("Echo", {"text": "computed_val", "delay": 0.20}, [], ["begin"]),
                "assigner": _node(
                    "VariableAssigner",
                    {
                        "variables": [
                            {
                                "variable": "assigned_res",
                                "operator": "set",
                                "parameter": "{producer@result}",
                            }
                        ]
                    },
                    [],
                    ["begin"],
                ),
            },
            "history": [],
            "retrieval": [],
            "memory": [],
            "path": [],
            "globals": {"sys.query": "", "sys.user_id": "u", "sys.conversation_turns": 0, "sys.files": [], "sys.history": [], "assigned_res": ""},
        }
    )


def _iteration_sibling_graph():
    """Create a graph where Iteration items_ref references a slow sibling producer."""
    return _dsl(
        {
            "begin": _node("Begin", {}, ["producer", "iter_node"], []),
            "producer": _node("Echo", {"text": "ready", "delay": 0.20}, [], ["begin"]),
            "iter_node": _node(
                "Iteration",
                {"items_ref": "producer@result"},
                [],
                ["begin"],
            ),
        }
    )


def _list_operations_sibling_graph():
    """Create a graph where ListOperations query references a slow sibling producer."""
    return _dsl(
        {
            "begin": _node("Begin", {}, ["producer", "list_ops"], []),
            "producer": _node("Echo", {"text": "ready", "delay": 0.20}, [], ["begin"]),
            "list_ops": _node(
                "ListOperations",
                {"query": "producer@result", "operations": "nth", "n": 1},
                [],
                ["begin"],
            ),
        }
    )


class _CanvasStack(tuple):
    """Container tuple providing test access to canvas and components."""

    def __new__(cls, canvas, trace, code_exec, switch=None, assigner=None, iteration=None, list_operations=None):
        """Construct a named tuple holding loaded canvas modules and helpers."""
        instance = super().__new__(cls, (canvas, trace, code_exec))
        instance.canvas = canvas
        instance.trace = trace
        instance.code_exec = code_exec
        instance.switch = switch
        instance.assigner = assigner
        instance.iteration = iteration
        instance.list_operations = list_operations
        return instance


@pytest.fixture
def canvas_stack(monkeypatch):
    """Fixture providing isolated Canvas test environment and traced components."""
    canvas, base, aggregator, code_exec, switch, assigner, iteration, list_operations, registry = _load_canvas_stack(monkeypatch)
    trace = _Trace()
    registry.update(_make_components(base, aggregator, trace, switch, assigner, iteration, list_operations))
    return _CanvasStack(canvas, trace, code_exec, switch, assigner, iteration, list_operations)


def _run(canvas_module, dsl):
    """Execute Canvas workflow synchronously and return the graph."""
    async def _drain():
        """Drain the asynchronous Canvas generator to completion."""
        graph = canvas_module.Canvas(dsl, tenant_id="t", task_id="task")
        async for _ in graph.run(query="Ragflow"):
            pass
        return graph

    return asyncio.run(_drain())


@pytest.mark.p1
def test_aggregator_waits_for_the_deeper_branch(canvas_stack):
    """Verify VariableAggregator waits until slower upstream branch finishes."""
    canvas_module, trace, _ = canvas_stack
    graph = _run(canvas_module, _aggregator_graph(deep_branches=1))
    agg = graph.get_component_obj("agg")

    assert agg.output("g1") == "Ragflow-c"
    assert agg.output("g2") == "+test"
    assert trace.at("start", "agg") >= trace.at("end", "c")


@pytest.mark.p1
def test_join_node_waits_for_the_deeper_branch(canvas_stack):
    """Verify an Echo join node waits until slower upstream branch finishes."""
    canvas_module, trace, _ = canvas_stack
    graph = _run(canvas_module, _echo_join_graph())

    assert graph.get_component_obj("join").output("result") == "[Ragflow-c][+test]"
    assert trace.at("start", "join") >= trace.at("end", "c")


@pytest.mark.p1
def test_balanced_branches_still_run_concurrently(canvas_stack):
    """Verify balanced branches are dispatched concurrently in the same batch."""
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
    """Verify nodes depending on unscheduled nodes are dropped instead of stalled."""
    canvas_module, trace, _ = canvas_stack
    _run(canvas_module, _unreachable_dependency_graph())

    assert trace.ran("a")
    assert not trace.ran("sink")


@pytest.mark.p1
def test_a_drop_carries_to_the_node_reading_it(canvas_stack):
    """Verify dropped status cascades to downstream dependent nodes."""
    canvas_module, trace, _ = canvas_stack
    _run(canvas_module, _dropped_upstream_graph())

    # `b` is deferred behind `a` and `a` is then dropped, so `b` has nothing to read.
    # Without the cascade it is the only candidate left and the cycle fallback runs it.
    assert not trace.ran("a")
    assert not trace.ran("b")


@pytest.mark.p1
def test_two_joins_held_back_together_each_run_once(canvas_stack):
    """Verify multiple join nodes held back in the same batch each execute once."""
    canvas_module, trace, _ = canvas_stack
    graph = _run(canvas_module, _two_join_graph())

    assert graph.get_component_obj("j1").output("result") == "1[Ragflow-c][+test]"
    assert graph.get_component_obj("j2").output("result") == "2[Ragflow-c][+test]"
    assert trace.runs("j1") == 1
    assert trace.runs("j2") == 1


@pytest.mark.p1
def test_nodes_that_reference_each_other_fail_instead_of_running(canvas_stack):
    """Verify mutually referencing nodes abort with error instead of deadlock."""
    canvas_module, trace, _ = canvas_stack
    graph = _run(canvas_module, _mutually_referencing_graph())

    # Running both would have each read the other's output before it was written.
    assert not trace.ran("x")
    assert not trace.ran("y")
    for cpn_id in ("x", "y"):
        assert "reference each other" in graph.get_component_obj(cpn_id).error()


@pytest.mark.p1
def test_a_cycle_still_terminates_the_run(canvas_stack):
    """Verify cyclic node references terminate execution without hanging."""
    canvas_module, _, _ = canvas_stack
    graph = canvas_module.Canvas(_mutually_referencing_graph(), tenant_id="t", task_id="task")
    graph.path = ["begin", "x", "y"]

    # The window has to advance past the cycle, or `_run_impl` never leaves it.
    assert graph._schedulable(1, 3) == 3


@pytest.mark.p1
def test_being_unrunnable_once_does_not_disable_the_node(canvas_stack):
    """Verify temporary unrunnable flag does not permanently disable node across loops."""
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
    """Verify resumed workflow run preserves its persisted execution order."""
    canvas_module, _, _ = canvas_stack
    graph = canvas_module.Canvas(_echo_join_graph(), tenant_id="t", task_id="task")
    graph.path = ["userfillup_0", "c", "b"]

    assert graph._schedulable(0, 3) == 3
    assert graph.path == ["userfillup_0", "c", "b"]


@pytest.mark.p1
def test_code_node_dependencies_come_from_its_arguments(canvas_stack):
    """Verify CodeExec derives upstream dependencies from its argument bindings."""
    _, _, code_exec = canvas_stack
    param = code_exec.CodeExecParam()
    param.arguments = {"arg1": "code_1@result", "arg2": "sys.query", "arg3": ""}
    cpn = code_exec.CodeExec.__new__(code_exec.CodeExec)
    cpn._param = param

    assert cpn.get_dependency_ids() == ["code_1"]


@pytest.mark.p1
def test_switch_param_refs_and_dependency_ids(canvas_stack):
    """Verify Switch extracts condition cpn_id references as dependency IDs."""
    switch_mod = canvas_stack.switch
    param = switch_mod.SwitchParam()
    param.conditions = [
        {
            "logical_operator": "and",
            "items": [
                {"cpn_id": "producer_1@output", "operator": "=", "value": "1"},
                {"cpn_id": "producer_2@output", "operator": "contains", "value": "2"},
                {"cpn_id": "", "operator": "=", "value": "3"},
            ],
            "to": ["dest"],
        }
    ]
    param.end_cpn_ids = ["else_dest"]
    cpn = switch_mod.Switch.__new__(switch_mod.Switch)
    cpn._param = param

    assert cpn.param_refs() == ["producer_1@output", "producer_2@output"]
    assert cpn.get_dependency_ids() == ["producer_1", "producer_2"]


@pytest.mark.p1
def test_switch_waits_for_producer_in_same_batch(canvas_stack):
    """Verify Switch sibling is deferred behind producer in the same batch window."""
    canvas_module, trace, _ = canvas_stack
    graph = _run(canvas_module, _switch_sibling_graph())
    sw = graph.get_component_obj("sw")

    assert sw.output("_next") == ["matched"]
    assert trace.at("start", "sw") >= trace.at("end", "producer")


@pytest.mark.p1
def test_variable_assigner_param_refs_and_dependency_ids(canvas_stack):
    """Verify VariableAssigner extracts variable and parameter references correctly."""
    assigner_mod = canvas_stack.assigner
    param = assigner_mod.VariableAssignerParam()
    param.variables = [
        {"variable": "sys.query", "operator": "set", "parameter": "{node_a@result}"},
        {"variable": "node_b@var", "operator": "overwrite", "parameter": "node_c@result"},
        {"variable": "global_val", "operator": "set", "parameter": "plain {node_d@out1} and {node_e@out2}"},
        {"variable": "email_literal", "operator": "set", "parameter": "support@example.com"},
        {"variable": "mixed_email", "operator": "set", "parameter": "email support@example.com or {node_f@email}"},
        {"variable": "sys.count", "operator": "+=", "parameter": 1},
        {"variable": "sys.list", "operator": "clear", "parameter": None},
    ]
    cpn = assigner_mod.VariableAssigner.__new__(assigner_mod.VariableAssigner)
    cpn._param = param

    refs = cpn.param_refs()
    assert "node_a@result" in refs
    assert "node_b@var" in refs
    assert "node_c@result" in refs
    assert "node_d@out1" in refs
    assert "node_e@out2" in refs
    assert "node_f@email" in refs
    assert "support@example.com" not in refs
    dep_ids = cpn.get_dependency_ids()
    assert dep_ids == ["node_a", "node_b", "node_c", "node_d", "node_e", "node_f"]


@pytest.mark.p1
def test_variable_assigner_waits_for_producer_in_same_batch(canvas_stack):
    """Verify VariableAssigner sibling waits for producer in the same batch window."""
    canvas_module, trace, _ = canvas_stack
    graph = _run(canvas_module, _assigner_sibling_graph())

    assert graph.get_variable_value("assigned_res") == "computed_val"
    assert trace.at("start", "assigner") >= trace.at("end", "producer")


@pytest.mark.p1
def test_iteration_param_refs_and_dependency_ids(canvas_stack):
    """Verify Iteration extracts items_ref as parameter reference and dependency ID."""
    iteration_mod = canvas_stack.iteration
    param = iteration_mod.IterationParam()
    param.items_ref = "generator@items"
    cpn = iteration_mod.Iteration.__new__(iteration_mod.Iteration)
    cpn._param = param

    assert cpn.param_refs() == ["generator@items"]
    assert cpn.get_dependency_ids() == ["generator"]


@pytest.mark.p1
def test_iteration_waits_for_producer_in_same_batch(canvas_stack):
    """Verify Iteration sibling waits for producer in the same batch window."""
    canvas_module, trace, _ = canvas_stack
    _run(canvas_module, _iteration_sibling_graph())

    assert trace.at("start", "iter_node") >= trace.at("end", "producer")


@pytest.mark.p1
def test_list_operations_param_refs_and_dependency_ids(canvas_stack):
    """Verify ListOperations extracts query as parameter reference and dependency ID."""
    list_ops_mod = canvas_stack.list_operations
    param = list_ops_mod.ListOperationsParam()
    param.query = "retrieval@chunks"
    cpn = list_ops_mod.ListOperations.__new__(list_ops_mod.ListOperations)
    cpn._param = param

    assert cpn.param_refs() == ["retrieval@chunks"]
    assert cpn.get_dependency_ids() == ["retrieval"]


@pytest.mark.p1
def test_list_operations_waits_for_producer_in_same_batch(canvas_stack):
    """Verify ListOperations sibling waits for producer in the same batch window."""
    canvas_module, trace, _ = canvas_stack
    _run(canvas_module, _list_operations_sibling_graph())

    assert trace.at("start", "list_ops") >= trace.at("end", "producer")


@pytest.mark.p1
def test_dependency_ids_handles_braced_param_refs(canvas_stack):
    """Verify ComponentBase.get_dependency_ids strips outer braces from references."""
    base = sys.modules["agent.component.base"]

    class DummyComponent(base.ComponentBase):
        """Dummy component returning braced and whitespace-padded parameter references."""

        component_name = "Dummy"

        def param_refs(self):
            """Return test parameter references with braces and whitespace."""
            return ["{producer_1@output}", "producer_2@output", "{ sys.query }", "invalid_no_at"]

    c = DummyComponent.__new__(DummyComponent)
    param = base.ComponentParamBase.__new__(base.ComponentParamBase)
    param.inputs = {}
    c._param = param
    assert c.get_dependency_ids() == ["producer_1", "producer_2"]

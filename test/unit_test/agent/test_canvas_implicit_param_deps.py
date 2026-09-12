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
"""Implicit output dependencies must be visible to the `Canvas` batch scheduler.

`Canvas._run_impl` dispatches `path[idx:to]` concurrently, so a node may only
read output written by a component that finished in an earlier batch. Issue
#19265 / PR #19282 fixed this for `VariableAggregator` and `CodeExec` by adding
`param_refs`, but several other components resolve cross-component references
through `Canvas.get_variable_value` inside `_invoke` without exposing them:
`Switch` (condition items), `VariableAssigner` (variable/parameter refs),
`Iteration` (`items_ref`) and `ListOperations` (`query`). When such a node
lands in the same batch as the component it reads - there is no canvas edge
forcing the order - it runs on the producer's stale (usually None) output and
silently routes or writes the wrong value.

These tests drive the real scheduler over graphs that put the reader in the
same window as its producer and assert the reader sees the fresh output.

The heavyweight edges (LLM service, task service, Redis, TTS) are stubbed and
the real component modules are loaded by path, the same isolation strategy as
`test/unit_test/agent/test_canvas_batch_readiness.py`.
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
    _pkg("api.utils", REPO_ROOT / "api" / "utils")
    _pkg("rag", REPO_ROOT / "rag")

    _stub("common.constants", LLMType=MagicMock(), RetCode=MagicMock())
    _stub("common.connection_utils", timeout=lambda *a, **kw: lambda fn: fn)
    _stub("common.token_utils", token_usage_sink=MagicMock(), langfuse_run_attrs=MagicMock())
    _stub("common.llm_request_context", set_llm_request_context=MagicMock(), reset_llm_request_context=MagicMock())
    _stub("common.exceptions", TaskCanceledException=type("TaskCanceledException", (Exception,), {}))
    _stub("api.utils.api_utils", timeout=lambda *a, **kw: lambda fn: fn)
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
    switch = _real("agent.component.switch", "agent/component/switch.py")
    assigner = _real("agent.component.variable_assigner", "agent/component/variable_assigner.py")
    return canvas, base, switch, assigner, registry


class _Trace:
    def __init__(self):
        self.events: list[tuple[str, str, float]] = []
        self._lock = threading.Lock()
        self._t0 = time.perf_counter()

    def record(self, kind, cpn_id):
        with self._lock:
            self.events.append((kind, cpn_id, time.perf_counter() - self._t0))


def _make_components(base, switch, assigner, trace):
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

    class TracedSwitch(switch.Switch):
        component_name = "Switch"

        def _invoke(self, **kwargs):
            trace.record("start", self._id)
            super()._invoke(**kwargs)
            trace.record("end", self._id)

    class TracedAssigner(assigner.VariableAssigner):
        component_name = "VariableAssigner"

        def _invoke(self, **kwargs):
            trace.record("start", self._id)
            super()._invoke(**kwargs)
            trace.record("end", self._id)

    return {
        "Begin": Begin,
        "BeginParam": BeginParam,
        "Echo": Echo,
        "EchoParam": EchoParam,
        "Switch": TracedSwitch,
        "SwitchParam": switch.SwitchParam,
        "VariableAssigner": TracedAssigner,
        "VariableAssignerParam": assigner.VariableAssignerParam,
    }


def _node(name, params, downstream, upstream):
    return {"obj": {"component_name": name, "params": params}, "downstream": downstream, "upstream": upstream}


def _dsl(components, extra_globals=None):
    globals_ = {"sys.query": "hello world", "sys.user_id": "u", "sys.conversation_turns": 0, "sys.files": [], "sys.history": []}
    if extra_globals:
        globals_.update(extra_globals)
    return json.dumps(
        {
            "components": components,
            "history": [],
            "retrieval": [],
            "memory": [],
            "path": [],
            "globals": globals_,
        }
    )


def _switch_graph():
    """begin fans out to a slow producer `a` and a Switch reading a@result.

    There is no edge from `a` to the switch - the condition references the
    output by name - so both land in the same batch window.
    """
    return _dsl(
        {
            "begin": _node("Begin", {}, ["a", "sw"], []),
            "a": _node("Echo", {"text": "hello world", "delay": 0.30}, [], ["begin"]),
            "sw": _node(
                "Switch",
                {
                    "conditions": [
                        {"items": [{"cpn_id": "a@result", "operator": "contains", "value": "hello"}], "logical_operator": "and", "to": ["yes"]}
                    ],
                    "end_cpn_ids": ["no"],
                },
                [],
                ["begin"],
            ),
            "yes": _node("Echo", {"text": "took-yes"}, [], ["sw"]),
            "no": _node("Echo", {"text": "took-no"}, [], ["sw"]),
        }
    )


def _assigner_graph():
    """begin fans out to a slow producer `a` and a VariableAssigner that copies
    a@result into the `g` global; no edge between them."""
    return _dsl(
        {
            "begin": _node("Begin", {}, ["a", "va"], []),
            "a": _node("Echo", {"text": "hello world", "delay": 0.30}, [], ["begin"]),
            "va": _node(
                "VariableAssigner",
                {"variables": [{"variable": "g", "operator": "overwrite", "parameter": "a@result"}]},
                [],
                ["begin"],
            ),
        },
        extra_globals={"g": ""},
    )


@pytest.fixture
def canvas_stack(monkeypatch):
    canvas, base, switch, assigner, registry = _load_canvas_stack(monkeypatch)
    trace = _Trace()
    registry.update(_make_components(base, switch, assigner, trace))
    return canvas, trace


def _run(canvas_module, dsl):
    async def _drain():
        graph = canvas_module.Canvas(dsl, tenant_id="t", task_id="task")
        async for _ in graph.run(query="Ragflow"):
            pass
        return graph

    return asyncio.run(_drain())


def test_switch_sees_producer_output_before_routing(canvas_stack):
    canvas, trace = canvas_stack
    graph = _run(canvas, _switch_graph())
    sw = graph.get_component_obj("sw")
    assert sw.output("_next") == ["yes"], f"switch routed to {sw.output('_next')}: it read a@result before 'a' finished"


def test_variable_assigner_copies_fresh_output(canvas_stack):
    canvas, trace = canvas_stack
    graph = _run(canvas, _assigner_graph())
    assert graph.globals.get("g") == "hello world", f"g = {graph.globals.get('g')!r}: assigner read a@result before 'a' finished"

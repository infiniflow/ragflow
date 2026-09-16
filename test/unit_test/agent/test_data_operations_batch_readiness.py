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
"""DataOperations requires explicit canvas edges for query component refs.

Workflow semantics: DataOperations may execute only when its upstream
producers are explicitly connected on the canvas. A ``producer@result``
(or braced) entry in ``query`` must not create an implicit scheduler
dependency via ``param_refs()``. Unconnected references are rejected;
connected upstream edges drive readiness (path growth and/or
``get_dependency_ids`` from ``upstream``), so a properly wired producer
is not read while still empty.
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


def _load_stack(monkeypatch):
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
    _stub("api.db.joint_services.tenant_model_service", get_tenant_default_model_by_type=MagicMock(return_value=None))
    _stub("api.db.services.file_service", FileService=MagicMock())
    _stub("api.db.services.llm_service", LLMBundle=MagicMock())
    _stub("api.db.services.task_service", has_canceled=MagicMock(return_value=False))
    _stub("rag.prompts.generator", chunks_format=MagicMock())
    _stub("rag.utils.redis_conn", REDIS_CONN=MagicMock())
    _stub("rag.utils.tts_cache", synthesize_with_cache=MagicMock())
    _stub("api.utils.api_utils", timeout=lambda *a, **kw: lambda fn: fn)

    _real("common.misc_utils", "common/misc_utils.py")
    _real("agent.settings", "agent/settings.py")
    _real("agent.dsl_migration", "agent/dsl_migration.py")

    base = _real("agent.component.base", "agent/component/base.py")
    component_pkg.base = base

    registry: dict = {}
    component_pkg.component_class = lambda name: registry[name]

    canvas = _real("agent.canvas", "agent/canvas.py")
    data_ops = _real("agent.component.data_operations", "agent/component/data_operations.py")

    _pkg("agent.tools", REPO_ROOT / "agent" / "tools")
    _stub("common.mcp_tool_call_conn", MCPToolBinding=MagicMock(), MCPToolCallSession=MagicMock(), ToolCallSession=MagicMock())
    _stub("common.settings", SANDBOX_HOST="")
    sys.modules["common.constants"].SANDBOX_ARTIFACT_BUCKET = ""
    sys.modules["common.constants"].SANDBOX_ARTIFACT_EXPIRE_DAYS = 1
    sys.modules["rag.prompts.generator"].kb_prompt = MagicMock()
    _real("agent.tools.base", "agent/tools/base.py")
    return canvas, base, data_ops, registry


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

    def ran(self, cpn_id):
        return any(k == "start" and c == cpn_id for k, c, _ in self.events)


def _make_components(base, data_ops, trace):
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

    class DictProducerParam(base.ComponentParamBase):
        def __init__(self):
            super().__init__()
            self.delay = 0.0
            self.payload = None
            self.outputs = {"result": {"value": None, "type": "Array of Object"}}

        def check(self):
            pass

    class DictProducer(base.ComponentBase):
        component_name = "DictProducer"

        def thoughts(self) -> str:
            return ""

        def _invoke(self, **kwargs):
            trace.record("start", self._id)
            if self._param.delay:
                time.sleep(self._param.delay)
            self.set_output("result", self._param.payload)
            trace.record("end", self._id)

    class TracedDataOperations(data_ops.DataOperations):
        component_name = "DataOperations"

        def _invoke(self, **kwargs):
            trace.record("start", self._id)
            super()._invoke(**kwargs)
            trace.record("end", self._id)

    return {
        "Begin": Begin,
        "BeginParam": BeginParam,
        "DictProducer": DictProducer,
        "DictProducerParam": DictProducerParam,
        "DataOperations": TracedDataOperations,
        "DataOperationsParam": data_ops.DataOperationsParam,
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


def _unconnected_sibling_graph(query=None):
    """Begin fans out to producer and DataOperations with no producer→ops edge."""
    if query is None:
        query = ["producer@result"]
    return _dsl(
        {
            "begin": _node("Begin", {}, ["producer", "ops"], []),
            "producer": _node(
                "DictProducer",
                {"payload": [{"name": "Ragflow", "ok": True}], "delay": 0.30},
                [],
                ["begin"],
            ),
            "ops": _node(
                "DataOperations",
                {
                    "query": query,
                    "operations": "select_keys",
                    "select_keys": ["name"],
                },
                [],
                ["begin"],
            ),
        }
    )


def _connected_chain_graph(query=None):
    """Begin → producer → DataOperations with an explicit canvas edge."""
    if query is None:
        query = ["producer@result"]
    return _dsl(
        {
            "begin": _node("Begin", {}, ["producer"], []),
            "producer": _node(
                "DictProducer",
                {"payload": [{"name": "Ragflow", "ok": True}], "delay": 0.30},
                ["ops"],
                ["begin"],
            ),
            "ops": _node(
                "DataOperations",
                {
                    "query": query,
                    "operations": "select_keys",
                    "select_keys": ["name"],
                },
                [],
                ["producer"],
            ),
        }
    )


def _connected_fanout_graph():
    """Begin fans out to both nodes, but producer is also an explicit upstream of ops.

    Without edge-based ``get_dependency_ids``, ops would share the batch with
    the slow producer and read a stale empty value even though the edge exists.
    """
    return _dsl(
        {
            "begin": _node("Begin", {}, ["producer", "ops"], []),
            "producer": _node(
                "DictProducer",
                {"payload": [{"name": "Ragflow", "ok": True}], "delay": 0.30},
                ["ops"],
                ["begin"],
            ),
            "ops": _node(
                "DataOperations",
                {
                    "query": ["{ producer@result }"],
                    "operations": "select_keys",
                    "select_keys": ["name"],
                },
                [],
                ["begin", "producer"],
            ),
        }
    )


@pytest.fixture
def stack(monkeypatch):
    canvas, base, data_ops, registry = _load_stack(monkeypatch)
    trace = _Trace()
    registry.update(_make_components(base, data_ops, trace))
    return canvas, trace, data_ops


def _run(canvas_module, dsl):
    async def _drain():
        graph = canvas_module.Canvas(dsl, tenant_id="t", task_id="task")
        async for _ in graph.run(query="unused"):
            pass
        return graph

    return asyncio.run(_drain())


@pytest.mark.p1
def test_unconnected_query_ref_is_rejected(stack):
    canvas_module, trace, _ = stack
    graph = _run(canvas_module, _unconnected_sibling_graph())
    ops = graph.get_component_obj("ops")

    err = ops.error() or ""
    assert "explicit canvas edge" in err, err
    assert "producer" in err
    assert ops.output("result") == []
    # Query must not become an implicit scheduler dependency.
    assert ops.param_refs() == []
    assert "producer" not in ops.get_dependency_ids()


@pytest.mark.p1
def test_unconnected_braced_query_ref_is_rejected(stack):
    canvas_module, _, _ = stack
    graph = _run(canvas_module, _unconnected_sibling_graph(query=["{producer@result}"]))
    ops = graph.get_component_obj("ops")
    err = ops.error() or ""
    assert "explicit canvas edge" in err, err
    assert "producer" in err


@pytest.mark.p1
def test_connected_upstream_reads_fresh_producer_output(stack):
    canvas_module, trace, _ = stack
    graph = _run(canvas_module, _connected_chain_graph())
    ops = graph.get_component_obj("ops")

    assert not ops.error(), ops.error()
    assert ops.output("result") == [{"name": "Ragflow"}]
    assert trace.at("start", "ops") >= trace.at("end", "producer")
    assert "producer" in ops.get_dependency_ids()
    assert ops.param_refs() == []


@pytest.mark.p1
def test_connected_fanout_defers_via_explicit_upstream_edge(stack):
    canvas_module, trace, _ = stack
    graph = _run(canvas_module, _connected_fanout_graph())
    ops = graph.get_component_obj("ops")

    assert not ops.error(), ops.error()
    assert ops.output("result") == [{"name": "Ragflow"}]
    assert trace.at("start", "ops") >= trace.at("end", "producer")
    assert "producer" in ops.get_dependency_ids()


@pytest.mark.p1
def test_normalize_query_ref_strips_braces_and_surrounding_spaces(stack):
    _, _, data_ops = stack
    assert data_ops.DataOperations._normalize_query_ref(" { producer@result } ") == "producer@result"


@pytest.mark.p1
def test_connected_braced_query_ref_with_surrounding_spaces(stack):
    canvas_module, _, _ = stack
    graph = _run(canvas_module, _connected_chain_graph(query=[" { producer@result } "]))
    ops = graph.get_component_obj("ops")

    assert not ops.error(), ops.error()
    assert ops.output("result") == [{"name": "Ragflow"}]

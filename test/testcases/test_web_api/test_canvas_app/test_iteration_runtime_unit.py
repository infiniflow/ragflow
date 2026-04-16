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

import asyncio
import importlib.util
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


def _load_canvas_runtime(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    quart = ModuleType("quart")
    quart.make_response = lambda *a, **kw: None
    quart.jsonify = lambda *a, **kw: None
    monkeypatch.setitem(sys.modules, "quart", quart)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    common_constants = ModuleType("common.constants")
    common_constants.LLMType = SimpleNamespace(TTS="tts")
    monkeypatch.setitem(sys.modules, "common.constants", common_constants)

    common_misc = ModuleType("common.misc_utils")
    common_misc.get_uuid = lambda: "uuid"
    common_misc.hash_str2int = lambda x: 1

    async def _thread_pool_exec(fn, *args, **kwargs):
        return fn(*args, **kwargs)

    common_misc.thread_pool_exec = _thread_pool_exec
    monkeypatch.setitem(sys.modules, "common.misc_utils", common_misc)

    common_conn = ModuleType("common.connection_utils")

    def timeout(_seconds):
        def decorator(fn):
            return fn

        return decorator

    common_conn.timeout = timeout
    monkeypatch.setitem(sys.modules, "common.connection_utils", common_conn)

    common_ex = ModuleType("common.exceptions")

    class TaskCanceledException(Exception):
        pass

    common_ex.TaskCanceledException = TaskCanceledException
    monkeypatch.setitem(sys.modules, "common.exceptions", common_ex)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)
    api_db_pkg = ModuleType("api.db")
    api_db_pkg.__path__ = [str(repo_root / "api" / "db")]
    monkeypatch.setitem(sys.modules, "api.db", api_db_pkg)
    api_db_services_pkg = ModuleType("api.db.services")
    api_db_services_pkg.__path__ = [str(repo_root / "api" / "db" / "services")]
    monkeypatch.setitem(sys.modules, "api.db.services", api_db_services_pkg)
    api_db_joint_pkg = ModuleType("api.db.joint_services")
    api_db_joint_pkg.__path__ = [str(repo_root / "api" / "db" / "joint_services")]
    monkeypatch.setitem(sys.modules, "api.db.joint_services", api_db_joint_pkg)

    file_service = ModuleType("api.db.services.file_service")
    file_service.FileService = object
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service)

    llm_service = ModuleType("api.db.services.llm_service")
    llm_service.LLMBundle = object
    monkeypatch.setitem(sys.modules, "api.db.services.llm_service", llm_service)

    task_service = ModuleType("api.db.services.task_service")
    task_service.has_canceled = lambda _task_id: False
    monkeypatch.setitem(sys.modules, "api.db.services.task_service", task_service)

    tenant_model_service = ModuleType("api.db.joint_services.tenant_model_service")
    tenant_model_service.get_tenant_default_model_by_type = lambda *_a, **_kw: None
    monkeypatch.setitem(
        sys.modules,
        "api.db.joint_services.tenant_model_service",
        tenant_model_service,
    )

    rag_pkg = ModuleType("rag")
    rag_pkg.__path__ = [str(repo_root / "rag")]
    monkeypatch.setitem(sys.modules, "rag", rag_pkg)
    rag_prompts_pkg = ModuleType("rag.prompts")
    rag_prompts_pkg.__path__ = [str(repo_root / "rag" / "prompts")]
    monkeypatch.setitem(sys.modules, "rag.prompts", rag_prompts_pkg)
    rag_prompts = ModuleType("rag.prompts.generator")
    rag_prompts.chunks_format = lambda *_a, **_kw: ""
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", rag_prompts)

    rag_utils_pkg = ModuleType("rag.utils")
    rag_utils_pkg.__path__ = [str(repo_root / "rag" / "utils")]
    monkeypatch.setitem(sys.modules, "rag.utils", rag_utils_pkg)
    rag_redis = ModuleType("rag.utils.redis_conn")
    rag_redis.REDIS_CONN = SimpleNamespace(delete=lambda *_a, **_kw: None, set=lambda *_a, **_kw: None)
    monkeypatch.setitem(sys.modules, "rag.utils.redis_conn", rag_redis)

    agent_pkg = ModuleType("agent")
    agent_pkg.__path__ = [str(repo_root / "agent")]
    monkeypatch.setitem(sys.modules, "agent", agent_pkg)

    agent_settings = ModuleType("agent.settings")
    agent_settings.FLOAT_ZERO = 1e-8
    agent_settings.PARAM_MAXDEPTH = 5
    monkeypatch.setitem(sys.modules, "agent.settings", agent_settings)

    dsl_migration = ModuleType("agent.dsl_migration")
    dsl_migration.normalize_chunker_dsl = lambda dsl: dsl
    monkeypatch.setitem(sys.modules, "agent.dsl_migration", dsl_migration)

    component_pkg = ModuleType("agent.component")
    component_pkg.__path__ = [str(repo_root / "agent" / "component")]
    monkeypatch.setitem(sys.modules, "agent.component", component_pkg)

    base_spec = importlib.util.spec_from_file_location(
        "agent.component.base", repo_root / "agent" / "component" / "base.py"
    )
    base_mod = importlib.util.module_from_spec(base_spec)
    monkeypatch.setitem(sys.modules, "agent.component.base", base_mod)
    base_spec.loader.exec_module(base_mod)

    iteration_spec = importlib.util.spec_from_file_location(
        "agent.component.iteration", repo_root / "agent" / "component" / "iteration.py"
    )
    iteration_mod = importlib.util.module_from_spec(iteration_spec)
    monkeypatch.setitem(sys.modules, "agent.component.iteration", iteration_mod)
    iteration_spec.loader.exec_module(iteration_mod)

    iterationitem_spec = importlib.util.spec_from_file_location(
        "agent.component.iterationitem",
        repo_root / "agent" / "component" / "iterationitem.py",
    )
    iterationitem_mod = importlib.util.module_from_spec(iterationitem_spec)
    monkeypatch.setitem(sys.modules, "agent.component.iterationitem", iterationitem_mod)
    iterationitem_spec.loader.exec_module(iterationitem_mod)

    class BeginParam(base_mod.ComponentParamBase):
        def check(self):
            return True

    class Begin(base_mod.ComponentBase):
        component_name = "Begin"

        def _invoke(self, **kwargs):
            return

        def thoughts(self):
            return "begin"

    class ProbeParam(base_mod.ComponentParamBase):
        def __init__(self):
            super().__init__()
            self.query = ""
            self.inputs = {"query": {"value": None}}

        def get_input_form(self):
            return {"query": {"name": "Query", "type": "line"}}

        def check(self):
            return True

    class Probe(base_mod.ComponentBase):
        component_name = "Probe"

        def _invoke(self, **kwargs):
            query_text = kwargs.get("query")
            vars_map = self.get_input_elements_from_text(query_text)
            query = self.string_format(
                query_text, {key: value["value"] for key, value in vars_map.items()}
            )
            calls = self._canvas.globals.setdefault("probe.calls", [])
            calls.append(query)
            self.set_output("result", query)

        def thoughts(self):
            return "probe"

    class SinkParam(base_mod.ComponentParamBase):
        def check(self):
            return True

    class Sink(base_mod.ComponentBase):
        component_name = "Sink"

        def _invoke(self, **kwargs):
            self.set_output("done", True)

        def thoughts(self):
            return "sink"

    class_map = {
        "Begin": Begin,
        "BeginParam": BeginParam,
        "Iteration": iteration_mod.Iteration,
        "IterationParam": iteration_mod.IterationParam,
        "IterationItem": iterationitem_mod.IterationItem,
        "IterationItemParam": iterationitem_mod.IterationItemParam,
        "Probe": Probe,
        "ProbeParam": ProbeParam,
        "Sink": Sink,
        "SinkParam": SinkParam,
    }

    component_pkg.component_class = lambda name: class_map[name]

    canvas_spec = importlib.util.spec_from_file_location(
        "agent.canvas", repo_root / "agent" / "canvas.py"
    )
    canvas_mod = importlib.util.module_from_spec(canvas_spec)
    monkeypatch.setitem(sys.modules, "agent.canvas", canvas_mod)
    canvas_spec.loader.exec_module(canvas_mod)

    return canvas_mod


async def _collect_events(canvas):
    events = []
    async for event in canvas.run():
        events.append(event)
    return events


@pytest.mark.p2
def test_iteration_runtime_processes_all_array_items(monkeypatch):
    canvas_mod = _load_canvas_runtime(monkeypatch)

    dsl = {
        "components": {
            "begin": {
                "obj": {"component_name": "Begin", "params": {}},
                "downstream": ["Iteration:1"],
                "upstream": [],
            },
            "Iteration:1": {
                "obj": {
                    "component_name": "Iteration",
                    "params": {"items_ref": "env.items"},
                },
                "downstream": ["Sink:1"],
                "upstream": ["begin"],
            },
            "IterationItem:1": {
                "obj": {"component_name": "IterationItem", "params": {}},
                "parent_id": "Iteration:1",
                "downstream": ["Probe:1"],
                "upstream": [],
            },
            "Probe:1": {
                "obj": {
                    "component_name": "Probe",
                    "params": {"query": "IterationItem:1@result"},
                },
                "parent_id": "Iteration:1",
                "downstream": [],
                "upstream": ["IterationItem:1"],
            },
            "Sink:1": {
                "obj": {"component_name": "Sink", "params": {}},
                "downstream": [],
                "upstream": ["Iteration:1"],
            },
        },
        "graph": {
            "nodes": [
                {"id": "begin", "data": {"name": "Begin"}},
                {"id": "Iteration:1", "data": {"name": "Iteration"}},
                {"id": "IterationItem:1", "data": {"name": "IterationItem"}},
                {"id": "Probe:1", "data": {"name": "Probe"}},
                {"id": "Sink:1", "data": {"name": "Sink"}},
            ]
        },
        "history": [],
        "path": [],
        "retrieval": [],
        "globals": {
            "sys.query": "",
            "sys.user_id": "",
            "sys.conversation_turns": 0,
            "sys.files": [],
            "sys.history": [],
            "sys.date": "",
            "env.items": ["a", "b", "c"],
        },
    }

    canvas = canvas_mod.Canvas(json.dumps(dsl))
    events = asyncio.run(_collect_events(canvas))

    assert canvas.globals["probe.calls"] == ["a", "b", "c"]
    assert any(event["event"] == "workflow_finished" for event in events)


@pytest.mark.p2
def test_iteration_runtime_supports_bare_item_alias(monkeypatch):
    canvas_mod = _load_canvas_runtime(monkeypatch)

    dsl = {
        "components": {
            "begin": {
                "obj": {"component_name": "Begin", "params": {}},
                "downstream": ["Iteration:1"],
                "upstream": [],
            },
            "Iteration:1": {
                "obj": {
                    "component_name": "Iteration",
                    "params": {"items_ref": "env.items"},
                },
                "downstream": ["Sink:1"],
                "upstream": ["begin"],
            },
            "IterationItem:1": {
                "obj": {"component_name": "IterationItem", "params": {}},
                "parent_id": "Iteration:1",
                "downstream": ["Probe:1"],
                "upstream": [],
            },
            "Probe:1": {
                "obj": {
                    "component_name": "Probe",
                    "params": {"query": "{item}"},
                },
                "parent_id": "Iteration:1",
                "downstream": [],
                "upstream": ["IterationItem:1"],
            },
            "Sink:1": {
                "obj": {"component_name": "Sink", "params": {}},
                "downstream": [],
                "upstream": ["Iteration:1"],
            },
        },
        "graph": {
            "nodes": [
                {"id": "begin", "data": {"name": "Begin"}},
                {"id": "Iteration:1", "data": {"name": "Iteration"}},
                {"id": "IterationItem:1", "data": {"name": "IterationItem"}},
                {"id": "Probe:1", "data": {"name": "Probe"}},
                {"id": "Sink:1", "data": {"name": "Sink"}},
            ]
        },
        "history": [],
        "path": [],
        "retrieval": [],
        "globals": {
            "sys.query": "",
            "sys.user_id": "",
            "sys.conversation_turns": 0,
            "sys.files": [],
            "sys.history": [],
            "sys.date": "",
            "env.items": ["a", "b", "c"],
        },
    }

    canvas = canvas_mod.Canvas(json.dumps(dsl))
    asyncio.run(_collect_events(canvas))

    assert canvas.globals["probe.calls"] == ["a", "b", "c"]

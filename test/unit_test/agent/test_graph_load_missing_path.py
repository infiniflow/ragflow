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
"""Regression tests for Graph.load() when stored DSL has no `path` key.

Issue #18746: a dataflow canvas whose stored DSL only has
`components` / `globals` / `graph` / `messages` / `variables` crashed
on every Test Run and Knowledge Base parse with KeyError: 'path'.

Call path: DataflowService.run_dataflow() -> Pipeline.__init__() ->
Graph.load(). `path` is runtime execution state, not authored graph,
and Graph.__init__ already defaults it to []. Canvas.load() already
reads the sibling `memory` field with .get(..., []).
"""

from __future__ import annotations

import contextvars
import importlib.util
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace


def _stub(monkeypatch, name, **attrs):
    """Register a minimal dependency module for isolated Graph/Pipeline loading."""
    module = ModuleType(name)
    for key, value in attrs.items():
        setattr(module, key, value)
    monkeypatch.setitem(sys.modules, name, module)
    return module


class _Param:
    def update(self, params):
        self.params = params

    def check(self):
        return True


class _Component:
    def __init__(self, canvas, component_id, param):
        self.canvas = canvas
        self.component_id = component_id
        self.param = param


def _component_class(name):
    if str(name).endswith("Param"):
        return _Param
    return _Component


def _dsl_without_path():
    """DSL top-level keys as dumped from user_canvas in #18746."""
    return {
        "components": {
            "File": {
                "obj": {"component_name": "File", "params": {}},
                "downstream": ["Parser"],
                "upstream": [],
            },
            "Parser": {
                "obj": {"component_name": "Parser", "params": {}},
                "downstream": [],
                "upstream": ["File"],
            },
        },
        "globals": {},
        "graph": {},
        "messages": {},
        "variables": {},
    }


def _load_canvas_and_pipeline(monkeypatch):
    """Load the real Graph and Pipeline classes without runtime services."""
    _stub(monkeypatch, "agent.component", component_class=_component_class)
    _stub(monkeypatch, "agent.component.base", ComponentBase=object)
    _stub(monkeypatch, "agent.dsl_migration", normalize_chunker_dsl=lambda dsl: dsl)
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=lambda *_args: None,
    )
    _stub(monkeypatch, "api.db.services.file_service", FileService=object)
    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=object)
    _stub(
        monkeypatch,
        "api.db.services.task_service",
        has_canceled=lambda *_args: False,
        TaskService=object,
        CANVAS_DEBUG_DOC_ID="CANVAS_DEBUG_DOC_ID",
    )
    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(get_knowledgebase_id=lambda *_args: None),
    )
    _stub(monkeypatch, "common.constants", LLMType=SimpleNamespace(CHAT="chat"))
    _stub(
        monkeypatch,
        "common.llm_request_context",
        set_llm_request_context=lambda **_kwargs: None,
        reset_llm_request_context=lambda _token: None,
    )
    _stub(monkeypatch, "common.exceptions", TaskCanceledException=Exception)
    _stub(monkeypatch, "common.misc_utils", get_uuid=lambda: "uuid", hash_str2int=lambda value: hash(value))
    _stub(
        monkeypatch,
        "common.token_utils",
        token_usage_sink=contextvars.ContextVar("token_usage_sink", default=None),
        langfuse_run_attrs=contextvars.ContextVar("langfuse_run_attrs", default=None),
    )
    _stub(monkeypatch, "rag.prompts.generator", chunks_format=lambda *_args: [])
    _stub(monkeypatch, "rag.utils.redis_conn", REDIS_CONN=SimpleNamespace())
    _stub(monkeypatch, "rag.utils.tts_cache", synthesize_with_cache=lambda *_args, **_kwargs: None)

    repo_root = Path(__file__).resolve().parents[3]

    canvas_spec = importlib.util.spec_from_file_location("agent.canvas", repo_root / "agent" / "canvas.py")
    canvas_mod = importlib.util.module_from_spec(canvas_spec)
    monkeypatch.setitem(sys.modules, "agent.canvas", canvas_mod)
    canvas_spec.loader.exec_module(canvas_mod)

    pipeline_spec = importlib.util.spec_from_file_location("rag.flow.pipeline", repo_root / "rag" / "flow" / "pipeline.py")
    pipeline_mod = importlib.util.module_from_spec(pipeline_spec)
    monkeypatch.setitem(sys.modules, "rag.flow.pipeline", pipeline_mod)
    pipeline_spec.loader.exec_module(pipeline_mod)

    return canvas_mod, pipeline_mod


def _graph_load(canvas_mod, dsl):
    """Call Graph.load() on a stripped instance (the KeyError site in #18746)."""
    graph = canvas_mod.Graph.__new__(canvas_mod.Graph)
    graph.custom_header = None
    graph.dsl = dict(dsl)
    graph.load()
    return graph


def test_graph_load_defaults_path_when_dsl_has_only_authored_keys(monkeypatch):
    """Graph.load() must not KeyError when stored DSL has no `path` key."""
    canvas_mod, _pipeline_mod = _load_canvas_and_pipeline(monkeypatch)
    dsl = _dsl_without_path()

    assert set(dsl) == {"components", "globals", "graph", "messages", "variables"}
    assert "path" not in dsl

    graph = _graph_load(canvas_mod, dsl)

    assert graph.path == []


def test_pipeline_init_loads_dsl_missing_path(monkeypatch):
    """Test Run / DataflowService constructs Pipeline(dsl) -> Graph.load().

    DataflowService.run_dataflow() instantiates Pipeline, whose __init__
    dumps a dict DSL and calls Graph.__init__, which always calls load().
    """
    _canvas_mod, pipeline_mod = _load_canvas_and_pipeline(monkeypatch)
    dsl = _dsl_without_path()

    assert "path" not in dsl

    pipeline = pipeline_mod.Pipeline(dsl, tenant_id="tenant", doc_id=None, task_id="task")

    assert pipeline.path == []


def test_graph_load_keeps_existing_path(monkeypatch):
    """A DSL that already recorded execution state must keep that path."""
    canvas_mod, _pipeline_mod = _load_canvas_and_pipeline(monkeypatch)
    dsl = _dsl_without_path()
    dsl["path"] = ["File", "Parser"]

    graph = _graph_load(canvas_mod, dsl)

    assert graph.path == ["File", "Parser"]


def test_pipeline_init_keeps_existing_path(monkeypatch):
    """Pipeline.__init__ must not wipe a path that is already in the DSL."""
    _canvas_mod, pipeline_mod = _load_canvas_and_pipeline(monkeypatch)
    dsl = _dsl_without_path()
    dsl["path"] = ["File"]

    pipeline = pipeline_mod.Pipeline(json.dumps(dsl), tenant_id="tenant")

    assert pipeline.path == ["File"]

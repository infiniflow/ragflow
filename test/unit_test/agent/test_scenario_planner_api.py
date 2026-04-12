#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import inspect
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest

class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func
        return decorator


def _run(coro):
    return asyncio.run(coro)


def _set_request_json(monkeypatch, module, payload):
    async def _req():
        return payload
    monkeypatch.setattr(module, "get_request_json", _req)


def _load_canvas_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[3]
    sys.path.insert(0, str(repo_root))

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    settings_mod = ModuleType("common.settings")
    settings_mod.docStoreConn = SimpleNamespace(index_exist=lambda *_a, **_k: False, delete=lambda *_a, **_k: True)
    common_pkg.settings = settings_mod
    monkeypatch.setitem(sys.modules, "common.settings", settings_mod)

    constants_mod = ModuleType("common.constants")
    constants_mod.RetCode = SimpleNamespace(SUCCESS=0, DATA_ERROR=102, OPERATING_ERROR=103, EXCEPTION_ERROR=100)
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    misc_utils_mod = ModuleType("common.misc_utils")
    misc_utils_mod.get_uuid = lambda: "uuid-1"

    async def _thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    misc_utils_mod.thread_pool_exec = _thread_pool_exec
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    db_pkg = ModuleType("api.db")
    db_pkg.__path__ = [str(repo_root / "api" / "db")]
    db_pkg.CanvasCategory = SimpleNamespace(Agent="Agent", DataFlow="DataFlow")
    monkeypatch.setitem(sys.modules, "api.db", db_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = []
    apps_mod.current_user = SimpleNamespace(id="user-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    apps_services_pkg = ModuleType("api.apps.services")
    apps_services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.apps.services", apps_services_pkg)

    canvas_replica_mod = ModuleType("api.apps.services.canvas_replica_service")
    canvas_replica_mod.CanvasReplicaService = SimpleNamespace(
        normalize_dsl=lambda dsl: dsl,
        bootstrap=lambda *_a, **_k: {},
        load_for_run=lambda *_a, **_k: None,
        commit_after_run=lambda *_a, **_k: True,
        replace_for_set=lambda *_a, **_k: True,
        create_if_absent=lambda *_a, **_k: {},
    )
    monkeypatch.setitem(sys.modules, "api.apps.services.canvas_replica_service", canvas_replica_mod)

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    canvas_service_mod = ModuleType("api.db.services.canvas_service")
    canvas_service_mod.CanvasTemplateService = SimpleNamespace(get_all=lambda: [])
    canvas_service_mod.UserCanvasService = SimpleNamespace(
        accessible=lambda *_a, **_k: True,
        delete_by_id=lambda *_a, **_k: True,
        query=lambda *_a, **_k: [],
        save=lambda **_k: True,
        update_by_id=lambda *_a, **_k: True,
        get_by_canvas_id=lambda cid: (True, {"id": cid}),
        get_by_id=lambda cid: (True, SimpleNamespace(id=cid, user_id="user-1", dsl="{}", canvas_category="Agent", to_dict=lambda: {"id": cid})),
        get_by_tenant_ids=lambda *_a, **_k: ([], 0),
    )
    canvas_service_mod.API4ConversationService = SimpleNamespace()
    canvas_service_mod.completion = lambda *_a, **_k: iter(())
    monkeypatch.setitem(sys.modules, "api.db.services.canvas_service", canvas_service_mod)

    for name in [
        "document_service",
        "file_service",
        "knowledgebase_service",
        "pipeline_operation_log_service",
        "task_service",
        "user_service",
        "user_canvas_version",
    ]:
        mod = ModuleType(f"api.db.services.{name}")
        monkeypatch.setitem(sys.modules, f"api.db.services.{name}", mod)

    sys.modules["api.db.services.document_service"].DocumentService = SimpleNamespace(clear_chunk_num_when_rerun=lambda *_a, **_k: True, update_by_id=lambda *_a, **_k: True)
    sys.modules["api.db.services.file_service"].FileService = SimpleNamespace(upload_info=lambda *_a, **_k: {"ok": True}, get_blob=lambda *_a, **_k: b"")
    sys.modules["api.db.services.knowledgebase_service"].KnowledgebaseService = SimpleNamespace(query=lambda **_k: [])
    sys.modules["api.db.services.pipeline_operation_log_service"].PipelineOperationLogService = SimpleNamespace(get_documents_info=lambda *_a, **_k: [], update_by_id=lambda *_a, **_k: True)
    sys.modules["api.db.services.task_service"].queue_dataflow = lambda *_a, **_k: (True, "")
    sys.modules["api.db.services.task_service"].CANVAS_DEBUG_DOC_ID = "debug-doc"
    sys.modules["api.db.services.task_service"].TaskService = SimpleNamespace(filter_delete=lambda *_a, **_k: True)
    sys.modules["api.db.services.user_service"].TenantService = SimpleNamespace(get_joined_tenants_by_user_id=lambda *_a, **_k: [])
    sys.modules["api.db.services.user_canvas_version"].UserCanvasVersionService = SimpleNamespace(
        list_by_canvas_id=lambda *_a, **_k: [],
        get_by_id=lambda *_a, **_k: (True, None),
        save_or_replace_latest=lambda *_a, **_k: True,
        build_version_title=lambda *_a, **_k: "stub_version_title",
    )

    db_models_mod = ModuleType("api.db.db_models")
    db_models_mod.APIToken = SimpleNamespace(query=lambda **_k: [])
    db_models_mod.Task = SimpleNamespace(doc_id=SimpleNamespace(__eq__=lambda self, other: ("eq", other)))
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    peewee_mod = ModuleType("peewee")
    peewee_mod.MySQLDatabase = type("MySQLDatabase", (), {})
    peewee_mod.PostgresqlDatabase = type("PostgresqlDatabase", (), {})
    monkeypatch.setitem(sys.modules, "peewee", peewee_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")
    api_utils_mod.get_json_result = lambda code=0, message="success", data=None: {"code": code, "message": message, "data": data}
    api_utils_mod.server_error_response = lambda exc: {"code": 100, "message": repr(exc), "data": None}
    api_utils_mod.validate_request = lambda *_a, **_k: (lambda func: func)
    api_utils_mod.get_data_error_result = lambda code=102, message="Sorry! Data missing!": {"code": code, "message": message}
    api_utils_mod.get_request_json = lambda: {}
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    rag_pkg = ModuleType("rag")
    rag_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag", rag_pkg)
    rag_flow_pkg = ModuleType("rag.flow")
    rag_flow_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag.flow", rag_flow_pkg)
    pipeline_mod = ModuleType("rag.flow.pipeline")
    pipeline_mod.Pipeline = lambda *_a, **_k: None
    monkeypatch.setitem(sys.modules, "rag.flow.pipeline", pipeline_mod)
    rag_nlp_mod = ModuleType("rag.nlp")
    rag_nlp_mod.search = SimpleNamespace(index_name=lambda tenant_id: f"idx-{tenant_id}")
    monkeypatch.setitem(sys.modules, "rag.nlp", rag_nlp_mod)
    rag_utils_pkg = ModuleType("rag.utils")
    rag_utils_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag.utils", rag_utils_pkg)
    redis_mod = ModuleType("rag.utils.redis_conn")
    redis_mod.REDIS_CONN = SimpleNamespace(set=lambda *_a, **_k: True, get=lambda *_a, **_k: None)
    monkeypatch.setitem(sys.modules, "rag.utils.redis_conn", redis_mod)

    agent_pkg = ModuleType("agent")
    agent_pkg.__path__ = [str(repo_root / "agent")]
    monkeypatch.setitem(sys.modules, "agent", agent_pkg)

    agent_component_mod = ModuleType("agent.component")
    agent_component_mod.LLM = type("_StubLLM", (), {})
    monkeypatch.setitem(sys.modules, "agent.component", agent_component_mod)

    agent_canvas_mod = ModuleType("agent.canvas")
    agent_canvas_mod.Canvas = type(
        "_StubCanvas",
        (),
        {
            "__init__": lambda self, dsl, _user_id, _agent_id=None, canvas_id=None: None,
            "run": lambda self, **_kwargs: iter(()),
            "cancel_task": lambda self: None,
            "reset": lambda self: None,
            "get_component_input_form": lambda self, _component_id: {},
            "get_component": lambda self, _component_id: {"obj": SimpleNamespace(reset=lambda: None, invoke=lambda **_k: None, output=lambda: {})},
            "__str__": lambda self: "{}",
        },
    )
    monkeypatch.setitem(sys.modules, "agent.canvas", agent_canvas_mod)

    dsl_mod = ModuleType("agent.dsl_migration")
    dsl_mod.normalize_chunker_dsl = lambda dsl: dsl
    monkeypatch.setitem(sys.modules, "agent.dsl_migration", dsl_mod)

    quart_mod = ModuleType("quart")
    quart_mod.request = SimpleNamespace(headers={}, args={}, files={}, method="POST", content_length=0)
    quart_mod.Response = lambda body, mimetype=None, content_type=None: {"body": body, "mimetype": mimetype, "content_type": content_type}
    quart_mod.make_response = lambda blob: {"blob": blob}
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    module_path = repo_root / "api" / "apps" / "canvas_app.py"
    spec = importlib.util.spec_from_file_location("scenario_plan_canvas_module", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, "scenario_plan_canvas_module", module)
    spec.loader.exec_module(module)
    return module


pytestmark = pytest.mark.p2


def test_scenario_plan_route_create_and_modify(monkeypatch):
    module = _load_canvas_module(monkeypatch)
    planner_calls = []

    class _Planner:
        def plan(self, **kwargs):
            planner_calls.append(kwargs)
            return {
                "title": kwargs["title"],
                "mode": "modify" if kwargs.get("existing_dsl") else "create",
                "archetype": "modify_existing" if kwargs.get("existing_dsl") else "qa_basic",
                "operations": [{"type": "stub"}],
                "warnings": [],
                "dsl": {"components": {}, "graph": {}},
            }

    monkeypatch.setattr(module, "ScenarioPlanner", _Planner)

    _set_request_json(monkeypatch, module, {"title": "Draft", "scenario": "Answer questions"})
    res = _run(inspect.unwrap(module.scenario_plan)())
    assert res["code"] == 0
    assert res["data"]["mode"] == "create"
    assert planner_calls[-1]["existing_dsl"] is None

    existing = {"components": {"begin": {}}, "graph": {"edges": [], "nodes": []}}
    _set_request_json(monkeypatch, module, {"title": "Draft", "scenario": "Add a notification step", "existing_dsl": existing, "canvas_category": "Agent"})
    res = _run(inspect.unwrap(module.scenario_plan)())
    assert res["code"] == 0
    assert res["data"]["mode"] == "modify"
    assert planner_calls[-1]["existing_dsl"] == existing


def test_scenario_plan_route_rejects_non_object_existing_dsl(monkeypatch):
    module = _load_canvas_module(monkeypatch)
    _set_request_json(monkeypatch, module, {"title": "Draft", "scenario": "Add a notification step", "existing_dsl": "not-a-dict"})
    res = _run(inspect.unwrap(module.scenario_plan)())
    assert res["code"] == 102
    assert "existing_dsl must be a JSON object" in res["message"]

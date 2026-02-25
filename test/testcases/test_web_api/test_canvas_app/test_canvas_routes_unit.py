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
import inspect
import sys
from copy import deepcopy
from functools import partial
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _AwaitableValue:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _co():
            return self._value

        return _co().__await__()


class _Args(dict):
    def get(self, key, default=None, type=None):
        value = super().get(key, default)
        if value is None or type is None:
            return value
        try:
            return type(value)
        except (TypeError, ValueError):
            return default


class _StubHeaders:
    def __init__(self):
        self._items = []

    def add_header(self, key, value):
        self._items.append((key, value))

    def get(self, key, default=None):
        for existing_key, value in reversed(self._items):
            if existing_key == key:
                return value
        return default


class _StubResponse:
    def __init__(self, body, mimetype=None, content_type=None):
        self.response = body
        self.body = body
        self.mimetype = mimetype
        self.content_type = content_type
        self.headers = _StubHeaders()


class _DummyRequest:
    def __init__(self, *, headers=None, args=None, files=None, method="POST", content_length=0):
        self.headers = headers or {}
        self.args = args or _Args()
        self.files = _AwaitableValue(files if files is not None else {})
        self.method = method
        self.content_length = content_length


class _DummyRetCode:
    SUCCESS = 0
    EXCEPTION_ERROR = 100
    ARGUMENT_ERROR = 101
    DATA_ERROR = 102
    OPERATING_ERROR = 103


class _DummyCanvasCategory:
    Agent = "agent"
    DataFlow = "dataflow"


class _TaskField:
    def __eq__(self, other):
        return ("eq", other)


class _DummyTask:
    doc_id = _TaskField()


class _FileMap(dict):
    def getlist(self, key):
        return list(self.get(key, []))


def _run(coro):
    return asyncio.run(coro)


async def _collect_stream(body):
    items = []
    if hasattr(body, "__aiter__"):
        async for item in body:
            if isinstance(item, bytes):
                item = item.decode("utf-8")
            items.append(item)
    else:
        for item in body:
            if isinstance(item, bytes):
                item = item.decode("utf-8")
            items.append(item)
    return items


def _set_request_json(monkeypatch, module, payload):
    async def _req():
        return deepcopy(payload)

    monkeypatch.setattr(module, "get_request_json", _req)


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


def _load_canvas_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    settings_mod = ModuleType("common.settings")
    settings_mod.docStoreConn = SimpleNamespace(
        index_exist=lambda *_args, **_kwargs: False,
        delete=lambda *_args, **_kwargs: True,
    )
    common_pkg.settings = settings_mod
    monkeypatch.setitem(sys.modules, "common.settings", settings_mod)

    constants_mod = ModuleType("common.constants")
    constants_mod.RetCode = _DummyRetCode
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

    apps_mod = ModuleType("api.apps")
    apps_mod.current_user = SimpleNamespace(id="user-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    db_pkg = ModuleType("api.db")
    db_pkg.CanvasCategory = _DummyCanvasCategory
    monkeypatch.setitem(sys.modules, "api.db", db_pkg)

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    canvas_service_mod = ModuleType("api.db.services.canvas_service")

    class _StubCanvasTemplateService:
        @staticmethod
        def get_all():
            return []

    class _StubUserCanvasService:
        @staticmethod
        def accessible(*_args, **_kwargs):
            return True

        @staticmethod
        def delete_by_id(*_args, **_kwargs):
            return True

        @staticmethod
        def query(*_args, **_kwargs):
            return []

        @staticmethod
        def save(**_kwargs):
            return True

        @staticmethod
        def update_by_id(*_args, **_kwargs):
            return True

        @staticmethod
        def get_by_canvas_id(_canvas_id):
            return True, {"id": _canvas_id}

        @staticmethod
        def get_by_id(_canvas_id):
            return True, SimpleNamespace(
                id=_canvas_id,
                user_id="user-1",
                dsl="{}",
                canvas_category=_DummyCanvasCategory.Agent,
                to_dict=lambda: {"id": _canvas_id},
            )

    class _StubAPI4ConversationService:
        @staticmethod
        def get_names(*_args, **_kwargs):
            return []

    async def _completion(*_args, **_kwargs):
        if False:
            yield {}

    canvas_service_mod.CanvasTemplateService = _StubCanvasTemplateService
    canvas_service_mod.UserCanvasService = _StubUserCanvasService
    canvas_service_mod.API4ConversationService = _StubAPI4ConversationService
    canvas_service_mod.completion = _completion
    monkeypatch.setitem(sys.modules, "api.db.services.canvas_service", canvas_service_mod)

    document_service_mod = ModuleType("api.db.services.document_service")
    document_service_mod.DocumentService = SimpleNamespace(
        clear_chunk_num_when_rerun=lambda *_args, **_kwargs: True,
        update_by_id=lambda *_args, **_kwargs: True,
    )
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", document_service_mod)

    file_service_mod = ModuleType("api.db.services.file_service")
    file_service_mod.FileService = SimpleNamespace(upload_info=lambda *_args, **_kwargs: {"ok": True})
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service_mod)

    pipeline_log_service_mod = ModuleType("api.db.services.pipeline_operation_log_service")
    pipeline_log_service_mod.PipelineOperationLogService = SimpleNamespace(
        get_documents_info=lambda *_args, **_kwargs: [],
        update_by_id=lambda *_args, **_kwargs: True,
    )
    monkeypatch.setitem(sys.modules, "api.db.services.pipeline_operation_log_service", pipeline_log_service_mod)

    task_service_mod = ModuleType("api.db.services.task_service")
    task_service_mod.queue_dataflow = lambda *_args, **_kwargs: (True, "")
    task_service_mod.CANVAS_DEBUG_DOC_ID = "debug-doc"
    task_service_mod.TaskService = SimpleNamespace(filter_delete=lambda *_args, **_kwargs: True)
    monkeypatch.setitem(sys.modules, "api.db.services.task_service", task_service_mod)

    user_service_mod = ModuleType("api.db.services.user_service")
    user_service_mod.TenantService = SimpleNamespace(get_joined_tenants_by_user_id=lambda *_args, **_kwargs: [])
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)

    canvas_version_mod = ModuleType("api.db.services.user_canvas_version")
    canvas_version_mod.UserCanvasVersionService = SimpleNamespace(
        insert=lambda **_kwargs: True,
        delete_all_versions=lambda *_args, **_kwargs: True,
    )
    monkeypatch.setitem(sys.modules, "api.db.services.user_canvas_version", canvas_version_mod)

    db_models_mod = ModuleType("api.db.db_models")

    class _StubAPIToken:
        @staticmethod
        def query(**_kwargs):
            return []

    db_models_mod.APIToken = _StubAPIToken
    db_models_mod.Task = _DummyTask
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")

    def _get_json_result(code=_DummyRetCode.SUCCESS, message="success", data=None):
        return {"code": code, "message": message, "data": data}

    def _get_data_error_result(code=_DummyRetCode.DATA_ERROR, message="Sorry! Data missing!"):
        return {"code": code, "message": message}

    def _server_error_response(exc):
        return {"code": _DummyRetCode.EXCEPTION_ERROR, "message": repr(exc), "data": None}

    async def _get_request_json():
        return {}

    def _validate_request(*_args, **_kwargs):
        def _decorator(func):
            return func

        return _decorator

    api_utils_mod.get_json_result = _get_json_result
    api_utils_mod.server_error_response = _server_error_response
    api_utils_mod.validate_request = _validate_request
    api_utils_mod.get_data_error_result = _get_data_error_result
    api_utils_mod.get_request_json = _get_request_json
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    rag_pkg = ModuleType("rag")
    rag_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag", rag_pkg)

    rag_flow_pkg = ModuleType("rag.flow")
    rag_flow_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag.flow", rag_flow_pkg)

    pipeline_mod = ModuleType("rag.flow.pipeline")

    class _StubPipeline:
        def __init__(self, *_args, **_kwargs):
            pass

    pipeline_mod.Pipeline = _StubPipeline
    monkeypatch.setitem(sys.modules, "rag.flow.pipeline", pipeline_mod)

    rag_nlp_mod = ModuleType("rag.nlp")
    rag_nlp_mod.search = SimpleNamespace(index_name=lambda tenant_id: f"idx-{tenant_id}")
    monkeypatch.setitem(sys.modules, "rag.nlp", rag_nlp_mod)

    rag_utils_pkg = ModuleType("rag.utils")
    rag_utils_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag.utils", rag_utils_pkg)

    redis_mod = ModuleType("rag.utils.redis_conn")
    redis_mod.REDIS_CONN = SimpleNamespace(
        set=lambda *_args, **_kwargs: True,
        get=lambda *_args, **_kwargs: None,
    )
    monkeypatch.setitem(sys.modules, "rag.utils.redis_conn", redis_mod)

    agent_pkg = ModuleType("agent")
    agent_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "agent", agent_pkg)

    agent_component_mod = ModuleType("agent.component")

    class _StubLLM:
        pass

    agent_component_mod.LLM = _StubLLM
    monkeypatch.setitem(sys.modules, "agent.component", agent_component_mod)

    agent_canvas_mod = ModuleType("agent.canvas")

    class _StubCanvas:
        def __init__(self, dsl, _user_id, _agent_id=None, canvas_id=None):
            self.dsl = dsl
            self.id = canvas_id

        async def run(self, **_kwargs):
            if False:
                yield {}

        def cancel_task(self):
            return None

        def reset(self):
            return None

        def get_component_input_form(self, _component_id):
            return {}

        def get_component(self, _component_id):
            return {"obj": SimpleNamespace(reset=lambda: None, invoke=lambda **_kwargs: None, output=lambda: {})}

        def __str__(self):
            return "{}"

    agent_canvas_mod.Canvas = _StubCanvas
    monkeypatch.setitem(sys.modules, "agent.canvas", agent_canvas_mod)

    quart_mod = ModuleType("quart")
    quart_mod.request = _DummyRequest()
    quart_mod.Response = _StubResponse

    async def _make_response(blob):
        return {"blob": blob}

    quart_mod.make_response = _make_response
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    module_path = repo_root / "api" / "apps" / "canvas_app.py"
    spec = importlib.util.spec_from_file_location("test_canvas_routes_unit_module", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, "test_canvas_routes_unit_module", module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_templates_rm_save_get_matrix_unit(monkeypatch):
    module = _load_canvas_module(monkeypatch)

    class _Template:
        def __init__(self, template_id):
            self.template_id = template_id

        def to_dict(self):
            return {"id": self.template_id}

    monkeypatch.setattr(module.CanvasTemplateService, "get_all", lambda: [_Template("tpl-1")])
    res = module.templates()
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"] == [{"id": "tpl-1"}]

    _set_request_json(monkeypatch, module, {"canvas_ids": ["c1", "c2"]})
    monkeypatch.setattr(module.UserCanvasService, "accessible", lambda *_args, **_kwargs: False)
    res = _run(inspect.unwrap(module.rm)())
    assert res["code"] == module.RetCode.OPERATING_ERROR
    assert "Only owner of canvas authorized" in res["message"]

    deleted = []
    _set_request_json(monkeypatch, module, {"canvas_ids": ["c1", "c2"]})
    monkeypatch.setattr(module.UserCanvasService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.UserCanvasService, "delete_by_id", lambda canvas_id: deleted.append(canvas_id))
    res = _run(inspect.unwrap(module.rm)())
    assert res["data"] is True
    assert deleted == ["c1", "c2"]

    _set_request_json(monkeypatch, module, {"title": "  Demo  ", "dsl": {"n": 1}})
    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [object()])
    res = _run(inspect.unwrap(module.save)())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "already exists" in res["message"]

    _set_request_json(monkeypatch, module, {"title": "Demo", "dsl": {"n": 1}})
    monkeypatch.setattr(module, "get_uuid", lambda: "canvas-new")
    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module.UserCanvasService, "save", lambda **_kwargs: False)
    res = _run(inspect.unwrap(module.save)())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "Fail to save canvas." in res["message"]

    created = {"save": [], "versions": []}
    _set_request_json(monkeypatch, module, {"title": "Demo", "dsl": {"n": 1}})
    monkeypatch.setattr(module, "get_uuid", lambda: "canvas-new")
    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module.UserCanvasService, "save", lambda **kwargs: created["save"].append(kwargs) or True)
    monkeypatch.setattr(module.UserCanvasVersionService, "insert", lambda **kwargs: created["versions"].append(("insert", kwargs)))
    monkeypatch.setattr(
        module.UserCanvasVersionService,
        "delete_all_versions",
        lambda canvas_id: created["versions"].append(("delete", canvas_id)),
    )
    res = _run(inspect.unwrap(module.save)())
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["id"] == "canvas-new"
    assert created["save"]
    assert any(item[0] == "insert" for item in created["versions"])
    assert any(item[0] == "delete" for item in created["versions"])

    _set_request_json(monkeypatch, module, {"id": "canvas-1", "title": "Renamed", "dsl": "{\"m\": 1}"})
    monkeypatch.setattr(module.UserCanvasService, "accessible", lambda *_args, **_kwargs: False)
    res = _run(inspect.unwrap(module.save)())
    assert res["code"] == module.RetCode.OPERATING_ERROR

    updates = []
    versions = []
    _set_request_json(monkeypatch, module, {"id": "canvas-1", "title": "Renamed", "dsl": "{\"m\": 1}"})
    monkeypatch.setattr(module.UserCanvasService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.UserCanvasService, "update_by_id", lambda canvas_id, payload: updates.append((canvas_id, payload)))
    monkeypatch.setattr(module.UserCanvasVersionService, "insert", lambda **kwargs: versions.append(("insert", kwargs)))
    monkeypatch.setattr(module.UserCanvasVersionService, "delete_all_versions", lambda canvas_id: versions.append(("delete", canvas_id)))
    res = _run(inspect.unwrap(module.save)())
    assert res["code"] == module.RetCode.SUCCESS
    assert updates and updates[0][0] == "canvas-1"
    assert any(item[0] == "insert" for item in versions)
    assert any(item[0] == "delete" for item in versions)

    monkeypatch.setattr(module.UserCanvasService, "accessible", lambda *_args, **_kwargs: False)
    res = module.get("canvas-1")
    assert res["code"] == module.RetCode.DATA_ERROR
    assert res["message"] == "canvas not found."

    monkeypatch.setattr(module.UserCanvasService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.UserCanvasService, "get_by_canvas_id", lambda _canvas_id: (True, {"id": "canvas-1"}))
    res = module.get("canvas-1")
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["id"] == "canvas-1"


@pytest.mark.p2
def test_getsse_auth_token_and_ownership_matrix_unit(monkeypatch):
    module = _load_canvas_module(monkeypatch)

    monkeypatch.setattr(module, "request", _DummyRequest(headers={"Authorization": "Bearer"}))
    res = module.getsse("canvas-1")
    assert res["message"] == "Authorization is not valid!"

    monkeypatch.setattr(module, "request", _DummyRequest(headers={"Authorization": "Bearer invalid"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [])
    res = module.getsse("canvas-1")
    assert "API key is invalid" in res["message"]

    monkeypatch.setattr(module, "request", _DummyRequest(headers={"Authorization": "Bearer ok"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [])
    res = module.getsse("canvas-1")
    assert res["code"] == module.RetCode.OPERATING_ERROR

    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [object()])
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _canvas_id: (False, None))
    res = module.getsse("canvas-1")
    assert res["message"] == "canvas not found."

    bad_owner = SimpleNamespace(user_id="tenant-2", to_dict=lambda: {"id": "canvas-1"})
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _canvas_id: (True, bad_owner))
    res = module.getsse("canvas-1")
    assert res["message"] == "canvas not found."

    good_owner = SimpleNamespace(user_id="tenant-1", to_dict=lambda: {"id": "canvas-1"})
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _canvas_id: (True, good_owner))
    res = module.getsse("canvas-1")
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["id"] == "canvas-1"


@pytest.mark.p2
def test_run_dataflow_and_canvas_sse_matrix_unit(monkeypatch):
    module = _load_canvas_module(monkeypatch)

    async def _thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    monkeypatch.setattr(module, "thread_pool_exec", _thread_pool_exec)

    _set_request_json(monkeypatch, module, {"id": "c1"})
    monkeypatch.setattr(module.UserCanvasService, "accessible", lambda *_args, **_kwargs: False)
    res = _run(inspect.unwrap(module.run)())
    assert res["code"] == module.RetCode.OPERATING_ERROR

    _set_request_json(monkeypatch, module, {"id": "c1"})
    monkeypatch.setattr(module.UserCanvasService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _canvas_id: (False, None))
    res = _run(inspect.unwrap(module.run)())
    assert res["message"] == "canvas not found."

    class _CanvasRecord:
        def __init__(self, *, canvas_id, dsl, canvas_category):
            self.id = canvas_id
            self.dsl = dsl
            self.canvas_category = canvas_category

        def to_dict(self):
            return {"id": self.id, "dsl": self.dsl}

    pipeline_calls = []
    monkeypatch.setattr(module, "Pipeline", lambda *args, **kwargs: pipeline_calls.append((args, kwargs)))
    monkeypatch.setattr(module, "get_uuid", lambda: "task-1")

    _set_request_json(monkeypatch, module, {"id": "df-1", "files": ["f1"], "user_id": "exp-1"})
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _canvas_id: (True, _CanvasRecord(canvas_id="df-1", dsl={"n": 1}, canvas_category=module.CanvasCategory.DataFlow)))
    monkeypatch.setattr(module, "queue_dataflow", lambda *_args, **_kwargs: (False, "queue failed"))
    res = _run(inspect.unwrap(module.run)())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "queue failed" in res["message"]
    assert pipeline_calls

    _set_request_json(monkeypatch, module, {"id": "df-1", "files": ["f1"], "user_id": "exp-1"})
    monkeypatch.setattr(module, "queue_dataflow", lambda *_args, **_kwargs: (True, ""))
    res = _run(inspect.unwrap(module.run)())
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["message_id"] == "task-1"

    _set_request_json(monkeypatch, module, {"id": "ag-1", "query": "q", "files": [], "inputs": {}})
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _canvas_id: (True, _CanvasRecord(canvas_id="ag-1", dsl={"x": 1}, canvas_category=module.CanvasCategory.Agent)))
    monkeypatch.setattr(module, "Canvas", lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("canvas init failed")))
    res = _run(inspect.unwrap(module.run)())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "canvas init failed" in res["message"]

    updates = []

    class _CanvasSSESuccess:
        def __init__(self, *_args, **_kwargs):
            self.cancelled = False

        async def run(self, **_kwargs):
            yield {"answer": "stream-ok"}

        def cancel_task(self):
            self.cancelled = True

        def __str__(self):
            return '{"updated": true}'

    _set_request_json(monkeypatch, module, {"id": "ag-2", "query": "q", "files": [], "inputs": {}, "user_id": "exp-2"})
    monkeypatch.setattr(module, "Canvas", _CanvasSSESuccess)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _canvas_id: (True, _CanvasRecord(canvas_id="ag-2", dsl="{}", canvas_category=module.CanvasCategory.Agent)))
    monkeypatch.setattr(module.UserCanvasService, "update_by_id", lambda canvas_id, payload: updates.append((canvas_id, payload)))
    resp = _run(inspect.unwrap(module.run)())
    assert isinstance(resp, _StubResponse)
    assert resp.headers.get("Content-Type") == "text/event-stream; charset=utf-8"
    chunks = _run(_collect_stream(resp.response))
    assert any('"answer": "stream-ok"' in chunk for chunk in chunks)
    assert updates and updates[0][0] == "ag-2"

    class _CanvasSSEError:
        last_instance = None

        def __init__(self, *_args, **_kwargs):
            self.cancelled = False
            _CanvasSSEError.last_instance = self

        async def run(self, **_kwargs):
            yield {"answer": "start"}
            raise RuntimeError("stream boom")

        def cancel_task(self):
            self.cancelled = True

        def __str__(self):
            return "{}"

    _set_request_json(monkeypatch, module, {"id": "ag-3", "query": "q", "files": [], "inputs": {}, "user_id": "exp-3"})
    monkeypatch.setattr(module, "Canvas", _CanvasSSEError)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _canvas_id: (True, _CanvasRecord(canvas_id="ag-3", dsl="{}", canvas_category=module.CanvasCategory.Agent)))
    resp = _run(inspect.unwrap(module.run)())
    chunks = _run(_collect_stream(resp.response))
    assert any('"code": 500' in chunk and "stream boom" in chunk for chunk in chunks)
    assert _CanvasSSEError.last_instance.cancelled is True


@pytest.mark.p2
def test_exp_agent_completion_trace_and_filtering_unit(monkeypatch):
    module = _load_canvas_module(monkeypatch)
    _set_request_json(monkeypatch, module, {"return_trace": True})

    async def _agent_completion(*_args, **_kwargs):
        yield "data:not-json"
        yield 'data:{"event":"node_finished","data":{"component_id":"cmp-1","step":"done"}}'
        yield 'data:{"event":"heartbeat","data":{"t":1}}'
        yield 'data:{"event":"message","data":{"content":"hello"}}'
        yield 'data:{"event":"message_end","data":{"content":"bye"}}'

    monkeypatch.setattr(module, "agent_completion", _agent_completion)
    resp = _run(inspect.unwrap(module.exp_agent_completion)("canvas-1"))
    assert isinstance(resp, _StubResponse)
    assert resp.headers.get("Content-Type") == "text/event-stream; charset=utf-8"

    chunks = _run(_collect_stream(resp.response))
    assert any('"event": "node_finished"' in chunk and '"trace"' in chunk for chunk in chunks)
    assert not any('"event":"heartbeat"' in chunk or '"event": "heartbeat"' in chunk for chunk in chunks)
    assert any('"event":"message"' in chunk or '"event": "message"' in chunk for chunk in chunks)
    assert chunks[-1] == "data:[DONE]\n\n"


@pytest.mark.p2
def test_rerun_and_cancel_matrix_unit(monkeypatch):
    module = _load_canvas_module(monkeypatch)
    _set_request_json(monkeypatch, module, {"id": "flow-1", "dsl": {"n": 1}, "component_id": "cmp-1"})

    monkeypatch.setattr(module.PipelineOperationLogService, "get_documents_info", lambda _id: [])
    res = _run(inspect.unwrap(module.rerun)())
    assert res["message"] == "Document not found."

    processing_doc = {"id": "doc-1", "name": "Doc-1", "kb_id": "kb-1", "progress": 0.5}
    monkeypatch.setattr(module.PipelineOperationLogService, "get_documents_info", lambda _id: [dict(processing_doc)])
    res = _run(inspect.unwrap(module.rerun)())
    assert "is processing" in res["message"]

    class _DocStore:
        def __init__(self):
            self.deleted = []

        def index_exist(self, *_args, **_kwargs):
            return True

        def delete(self, *args, **_kwargs):
            self.deleted.append(args)
            return True

    doc_store = _DocStore()
    monkeypatch.setattr(module.settings, "docStoreConn", doc_store)

    doc = {
        "id": "doc-1",
        "name": "Doc-1",
        "kb_id": "kb-1",
        "progress": 1.0,
        "progress_msg": "old",
        "chunk_num": 8,
        "token_num": 12,
    }
    updates = {"doc": [], "pipeline": [], "tasks": [], "queue": []}
    monkeypatch.setattr(module.PipelineOperationLogService, "get_documents_info", lambda _id: [dict(doc)])
    monkeypatch.setattr(module.DocumentService, "clear_chunk_num_when_rerun", lambda doc_id: updates["doc"].append(("clear", doc_id)))
    monkeypatch.setattr(module.DocumentService, "update_by_id", lambda doc_id, payload: updates["doc"].append(("update", doc_id, payload)))
    monkeypatch.setattr(module.TaskService, "filter_delete", lambda expr: updates["tasks"].append(expr))
    monkeypatch.setattr(module.PipelineOperationLogService, "update_by_id", lambda flow_id, payload: updates["pipeline"].append((flow_id, payload)))
    monkeypatch.setattr(
        module,
        "queue_dataflow",
        lambda **kwargs: updates["queue"].append(kwargs) or (True, ""),
    )
    monkeypatch.setattr(module, "get_uuid", lambda: "task-rerun")
    _set_request_json(monkeypatch, module, {"id": "flow-1", "dsl": {"n": 1}, "component_id": "cmp-1"})
    res = _run(inspect.unwrap(module.rerun)())
    assert res["code"] == module.RetCode.SUCCESS
    assert doc_store.deleted
    assert any(item[0] == "clear" and item[1] == "doc-1" for item in updates["doc"])
    assert updates["pipeline"] and updates["pipeline"][0][1]["dsl"]["path"] == ["cmp-1"]
    assert updates["queue"] and updates["queue"][0]["rerun"] is True

    redis_calls = []
    monkeypatch.setattr(module.REDIS_CONN, "set", lambda key, value: redis_calls.append((key, value)))
    res = module.cancel("task-9")
    assert res["code"] == module.RetCode.SUCCESS
    assert redis_calls == [("task-9-cancel", "x")]

    monkeypatch.setattr(module.REDIS_CONN, "set", lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("redis fail")))
    res = module.cancel("task-9")
    assert res["code"] == module.RetCode.SUCCESS


@pytest.mark.p2
def test_reset_upload_input_form_debug_matrix_unit(monkeypatch):
    module = _load_canvas_module(monkeypatch)

    _set_request_json(monkeypatch, module, {"id": "canvas-1"})
    monkeypatch.setattr(module.UserCanvasService, "accessible", lambda *_args, **_kwargs: False)
    res = _run(inspect.unwrap(module.reset)())
    assert res["code"] == module.RetCode.OPERATING_ERROR

    _set_request_json(monkeypatch, module, {"id": "canvas-1"})
    monkeypatch.setattr(module.UserCanvasService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _canvas_id: (False, None))
    res = _run(inspect.unwrap(module.reset)())
    assert res["message"] == "canvas not found."

    class _ResetCanvas:
        def __init__(self, *_args, **_kwargs):
            self.reset_called = False

        def reset(self):
            self.reset_called = True

        def __str__(self):
            return '{"v": 2}'

    updates = []
    _set_request_json(monkeypatch, module, {"id": "canvas-1"})
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _canvas_id: (True, SimpleNamespace(id="canvas-1", dsl={"v": 1})))
    monkeypatch.setattr(module.UserCanvasService, "update_by_id", lambda canvas_id, payload: updates.append((canvas_id, payload)))
    monkeypatch.setattr(module, "Canvas", _ResetCanvas)
    res = _run(inspect.unwrap(module.reset)())
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"] == {"v": 2}
    assert updates == [("canvas-1", {"dsl": {"v": 2}})]

    _set_request_json(monkeypatch, module, {"id": "canvas-1"})
    monkeypatch.setattr(module, "Canvas", lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("reset boom")))
    res = _run(inspect.unwrap(module.reset)())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "reset boom" in res["message"]

    monkeypatch.setattr(module.UserCanvasService, "get_by_canvas_id", lambda _canvas_id: (False, None))
    monkeypatch.setattr(module, "request", _DummyRequest(args=_Args({"url": "http://example.com"}), files=_FileMap()))
    res = _run(module.upload("canvas-1"))
    assert res["message"] == "canvas not found."

    monkeypatch.setattr(module.UserCanvasService, "get_by_canvas_id", lambda _canvas_id: (True, {"user_id": "tenant-1"}))
    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(
            args=_Args({"url": "http://example.com"}),
            files=_FileMap({"file": ["file-1"]}),
        ),
    )
    monkeypatch.setattr(module.FileService, "upload_info", lambda user_id, file_obj, url=None: {"uid": user_id, "file": file_obj, "url": url})
    res = _run(module.upload("canvas-1"))
    assert res["data"]["url"] == "http://example.com"

    monkeypatch.setattr(
        module,
        "request",
        _DummyRequest(
            args=_Args({"url": "http://example.com"}),
            files=_FileMap({"file": ["f1", "f2"]}),
        ),
    )
    monkeypatch.setattr(module.FileService, "upload_info", lambda user_id, file_obj, url=None: {"uid": user_id, "file": file_obj, "url": url})
    res = _run(module.upload("canvas-1"))
    assert len(res["data"]) == 2

    monkeypatch.setattr(module.FileService, "upload_info", lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("upload boom")))
    res = _run(module.upload("canvas-1"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "upload boom" in res["message"]

    monkeypatch.setattr(module, "request", _DummyRequest(args=_Args({"id": "canvas-1", "component_id": "begin"})))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _canvas_id: (False, None))
    res = module.input_form()
    assert res["message"] == "canvas not found."

    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _canvas_id: (True, SimpleNamespace(id="canvas-1", dsl={"n": 1})))
    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [])
    res = module.input_form()
    assert res["code"] == module.RetCode.OPERATING_ERROR

    class _InputCanvas:
        def __init__(self, *_args, **_kwargs):
            pass

        def get_component_input_form(self, component_id):
            return {"component_id": component_id}

    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [object()])
    monkeypatch.setattr(module, "Canvas", _InputCanvas)
    res = module.input_form()
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["component_id"] == "begin"

    monkeypatch.setattr(module, "Canvas", lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("input boom")))
    res = module.input_form()
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "input boom" in res["message"]

    _set_request_json(
        monkeypatch,
        module,
        {"id": "canvas-1", "component_id": "llm-node", "params": {"p": {"value": "v"}}},
    )
    monkeypatch.setattr(module.UserCanvasService, "accessible", lambda *_args, **_kwargs: False)
    res = _run(inspect.unwrap(module.debug)())
    assert res["code"] == module.RetCode.OPERATING_ERROR

    class _DebugComponent(module.LLM):
        def __init__(self):
            self.reset_called = False
            self.debug_inputs = None
            self.invoked = None

        def reset(self):
            self.reset_called = True

        def set_debug_inputs(self, params):
            self.debug_inputs = params

        def invoke(self, **kwargs):
            self.invoked = kwargs

        def output(self):
            async def _gen():
                yield "A"
                yield "B"

            return {"stream": partial(_gen)}

    class _DebugCanvas:
        last_component = None

        def __init__(self, *_args, **_kwargs):
            self.message_id = ""
            self._component = _DebugComponent()
            _DebugCanvas.last_component = self._component

        def reset(self):
            return None

        def get_component(self, _component_id):
            return {"obj": self._component}

    _set_request_json(
        monkeypatch,
        module,
        {"id": "canvas-1", "component_id": "llm-node", "params": {"p": {"value": "v"}}},
    )
    monkeypatch.setattr(module.UserCanvasService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _canvas_id: (True, SimpleNamespace(id="canvas-1", dsl={"n": 1})))
    monkeypatch.setattr(module, "get_uuid", lambda: "msg-1")
    monkeypatch.setattr(module, "Canvas", _DebugCanvas)
    res = _run(inspect.unwrap(module.debug)())
    assert res["code"] == module.RetCode.SUCCESS
    assert res["data"]["stream"] == "AB"
    assert _DebugCanvas.last_component.reset_called is True
    assert _DebugCanvas.last_component.debug_inputs == {"p": {"value": "v"}}
    assert _DebugCanvas.last_component.invoked == {"p": "v"}

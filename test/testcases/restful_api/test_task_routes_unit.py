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
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _Condition:
    def __and__(self, _other):
        return self


class _Field:
    def __add__(self, _other):
        return self

    def __eq__(self, _other):
        return _Condition()

    def __ge__(self, _other):
        return _Condition()

    def __lt__(self, _other):
        return _Condition()


class _FakeTaskModel:
    id = _Field()
    progress = _Field()
    progress_msg = _Field()

    updates = []
    update_calls = 0

    @classmethod
    def update(cls, **payload):
        cls.update_calls += 1
        cls.updates.append(payload)
        return _FakeUpdate()


class _FakeUpdate:
    def where(self, *_args, **_kwargs):
        return self

    def execute(self):
        return 1


class _FakeRedis:
    def __init__(self):
        self.set_calls = []

    def set(self, key, value):
        self.set_calls.append((key, value))


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


def _run(coro):
    return asyncio.run(coro)


def _load_task_module(
    monkeypatch,
    *,
    task,
    document,
    document_accessible=True,
    index_task_kb=None,
    index_accessible=True,
    canvas_messages=None,
):
    repo_root = Path(__file__).resolve().parents[3]

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="user-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)
    api_pkg.apps = apps_mod

    db_pkg = ModuleType("api.db")
    db_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db", db_pkg)
    api_pkg.db = db_pkg

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)
    db_pkg.services = services_pkg

    api_service_mod = ModuleType("api.db.services.api_service")
    api_service_mod.query_calls = []
    api_service_mod.message_lookup_calls = []

    class _StubAPI4ConversationService:
        @staticmethod
        def query(**kwargs):
            api_service_mod.query_calls.append(kwargs)
            return [SimpleNamespace(message=messages) for messages in canvas_messages or []]

        @staticmethod
        def get_workflow_conversations_by_message_id(user_id, message_id):
            api_service_mod.message_lookup_calls.append((user_id, message_id))
            return [SimpleNamespace(message=messages) for messages in canvas_messages or [] if str(message_id).lower() in repr(messages).lower()]

    api_service_mod.API4ConversationService = _StubAPI4ConversationService
    monkeypatch.setitem(sys.modules, "api.db.services.api_service", api_service_mod)
    services_pkg.api_service = api_service_mod

    _FakeTaskModel.updates = []
    _FakeTaskModel.update_calls = 0

    task_service_mod = ModuleType("api.db.services.task_service")
    task_service_mod.CANVAS_DEBUG_DOC_ID = "canvas-debug-doc"
    task_service_mod.GRAPH_RAPTOR_FAKE_DOC_ID = "graph-raptor-doc"

    class _StubTaskService:
        model = _FakeTaskModel

        @staticmethod
        def get_by_id(_task_id):
            return (True, task) if task else (False, None)

    task_service_mod.TaskService = _StubTaskService
    monkeypatch.setitem(sys.modules, "api.db.services.task_service", task_service_mod)
    services_pkg.task_service = task_service_mod

    document_service_mod = ModuleType("api.db.services.document_service")
    document_service_mod.access_checks = []
    document_service_mod.update_calls = []

    class _StubDocumentService:
        @staticmethod
        def accessible(doc_id, user_id):
            document_service_mod.access_checks.append((doc_id, user_id))
            return document_accessible

        @staticmethod
        def get_by_id(_doc_id):
            return True, document

        @staticmethod
        def update_by_id(doc_id, payload):
            document_service_mod.update_calls.append((doc_id, payload))
            return True

    document_service_mod.DocumentService = _StubDocumentService
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", document_service_mod)
    services_pkg.document_service = document_service_mod

    knowledgebase_service_mod = ModuleType("api.db.services.knowledgebase_service")
    knowledgebase_service_mod.access_checks = []
    knowledgebase_service_mod.lookup_calls = []

    class _StubKnowledgebaseService:
        @staticmethod
        def get_or_none(**kwargs):
            knowledgebase_service_mod.lookup_calls.append(kwargs)
            if index_task_kb and kwargs.get(index_task_kb["field"]) == index_task_kb["task_id"]:
                return SimpleNamespace(id=index_task_kb["kb_id"])
            return None

        @staticmethod
        def accessible(kb_id, user_id):
            knowledgebase_service_mod.access_checks.append((kb_id, user_id))
            return index_accessible

    knowledgebase_service_mod.KnowledgebaseService = _StubKnowledgebaseService
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", knowledgebase_service_mod)
    services_pkg.knowledgebase_service = knowledgebase_service_mod

    api_utils_mod = ModuleType("api.utils.api_utils")

    def get_json_result(code=0, message="success", data=None):
        return {"code": code, "message": message, "data": data}

    async def get_request_json():
        return {}

    def validate_request(*_keys):
        def decorator(func):
            return func

        return decorator

    api_utils_mod.get_json_result = get_json_result
    api_utils_mod.get_request_json = get_request_json
    api_utils_mod.validate_request = validate_request
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    constants_mod = ModuleType("common.constants")
    constants_mod.RetCode = SimpleNamespace(ARGUMENT_ERROR=101, AUTHENTICATION_ERROR=109, CONNECTION_ERROR=105)
    constants_mod.TaskStatus = SimpleNamespace(
        RUNNING=SimpleNamespace(value="1"),
        SCHEDULE=SimpleNamespace(value="5"),
        CANCEL=SimpleNamespace(value="2"),
    )
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    redis_mod = ModuleType("rag.utils.redis_conn")
    redis_mod.REDIS_CONN = _FakeRedis()
    monkeypatch.setitem(sys.modules, "rag.utils.redis_conn", redis_mod)

    module_name = "test_task_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "task_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module, document_service_mod, knowledgebase_service_mod, api_service_mod, redis_mod.REDIS_CONN


@pytest.mark.p2
def test_cancel_missing_task_returns_success_without_side_effects(monkeypatch):
    module, document_service, knowledgebase_service, api_service, redis_conn = _load_task_module(
        monkeypatch,
        task=None,
        document=None,
    )

    payload = _run(module._cancel_task("missing-task-id"))

    assert payload["code"] == 0, payload
    assert payload["data"] is True, payload
    assert document_service.access_checks == []
    assert knowledgebase_service.access_checks == []
    assert api_service.query_calls == []
    assert api_service.message_lookup_calls == []
    assert redis_conn.set_calls == []
    assert module.TaskService.model.update_calls == 0
    assert document_service.update_calls == []


@pytest.mark.p2
def test_cancel_task_rejects_inaccessible_document_before_side_effects(monkeypatch):
    task = SimpleNamespace(doc_id="doc-1", progress=0, progress_msg="")
    document = SimpleNamespace(run="1")
    module, document_service, knowledgebase_service, api_service, redis_conn = _load_task_module(
        monkeypatch,
        task=task,
        document=document,
        document_accessible=False,
    )

    payload = _run(module._cancel_task("task-1"))

    assert payload["code"] == 109, payload
    assert payload["data"] is False, payload
    assert payload["message"] == "No authorization.", payload
    assert document_service.access_checks == [("doc-1", "user-1")]
    assert knowledgebase_service.access_checks == []
    assert api_service.query_calls == []
    assert api_service.message_lookup_calls == []
    assert redis_conn.set_calls == []
    assert module.TaskService.model.update_calls == 0
    assert document_service.update_calls == []


@pytest.mark.p2
def test_cancel_task_rejects_missing_doc_id_before_side_effects(monkeypatch):
    task = SimpleNamespace(doc_id="", progress=0, progress_msg="")
    module, document_service, knowledgebase_service, api_service, redis_conn = _load_task_module(
        monkeypatch,
        task=task,
        document=None,
    )

    payload = _run(module._cancel_task("task-1"))

    assert payload["code"] == 109, payload
    assert payload["data"] is False, payload
    assert payload["message"] == "No authorization.", payload
    assert document_service.access_checks == []
    assert knowledgebase_service.access_checks == []
    assert api_service.query_calls == []
    assert api_service.message_lookup_calls == []
    assert redis_conn.set_calls == []
    assert module.TaskService.model.update_calls == 0
    assert document_service.update_calls == []


@pytest.mark.p2
def test_cancel_task_authorized_document_sets_cancel_state(monkeypatch):
    task = SimpleNamespace(doc_id="doc-1", progress=0, progress_msg="")
    document = SimpleNamespace(run="1")
    module, document_service, knowledgebase_service, api_service, redis_conn = _load_task_module(
        monkeypatch,
        task=task,
        document=document,
    )

    payload = _run(module._cancel_task("task-1"))

    assert payload["code"] == 0, payload
    assert payload["data"] is True, payload
    assert document_service.access_checks == [("doc-1", "user-1")]
    assert knowledgebase_service.access_checks == []
    assert api_service.query_calls == []
    assert api_service.message_lookup_calls == []
    assert redis_conn.set_calls == [("task-1-cancel", "x")]
    assert module.TaskService.model.update_calls == 1
    assert module.TaskService.model.updates[0]["progress"] == -1
    assert document_service.update_calls == [("doc-1", {"run": "2", "progress": 0})]


@pytest.mark.p2
def test_cancel_task_authorizes_dataset_index_tasks(monkeypatch):
    task = SimpleNamespace(doc_id="graph-raptor-doc", progress=0, progress_msg="")
    module, document_service, knowledgebase_service, api_service, redis_conn = _load_task_module(
        monkeypatch,
        task=task,
        document=None,
        index_task_kb={"field": "raptor_task_id", "task_id": "task-1", "kb_id": "kb-1"},
    )

    payload = _run(module._cancel_task("task-1"))

    assert payload["code"] == 0, payload
    assert payload["data"] is True, payload
    assert document_service.access_checks == []
    assert knowledgebase_service.lookup_calls == [{"graphrag_task_id": "task-1"}, {"raptor_task_id": "task-1"}]
    assert knowledgebase_service.access_checks == [("kb-1", "user-1")]
    assert api_service.query_calls == []
    assert api_service.message_lookup_calls == []
    assert redis_conn.set_calls == [("task-1-cancel", "x")]
    assert module.TaskService.model.update_calls == 1
    assert document_service.update_calls == []


@pytest.mark.p2
def test_cancel_task_authorizes_graphrag_index_tasks(monkeypatch):
    task = SimpleNamespace(doc_id="graph-raptor-doc", progress=0, progress_msg="")
    module, document_service, knowledgebase_service, api_service, redis_conn = _load_task_module(
        monkeypatch,
        task=task,
        document=None,
        index_task_kb={"field": "graphrag_task_id", "task_id": "task-1", "kb_id": "kb-1"},
    )

    payload = _run(module._cancel_task("task-1"))

    assert payload["code"] == 0, payload
    assert payload["data"] is True, payload
    assert document_service.access_checks == []
    assert knowledgebase_service.lookup_calls == [{"graphrag_task_id": "task-1"}]
    assert knowledgebase_service.access_checks == [("kb-1", "user-1")]
    assert api_service.query_calls == []
    assert api_service.message_lookup_calls == []
    assert redis_conn.set_calls == [("task-1-cancel", "x")]
    assert module.TaskService.model.update_calls == 1
    assert document_service.update_calls == []


@pytest.mark.p2
def test_cancel_task_authorizes_mindmap_index_tasks(monkeypatch):
    task = SimpleNamespace(doc_id="graph-raptor-doc", progress=0, progress_msg="")
    module, document_service, knowledgebase_service, api_service, redis_conn = _load_task_module(
        monkeypatch,
        task=task,
        document=None,
        index_task_kb={"field": "mindmap_task_id", "task_id": "task-1", "kb_id": "kb-1"},
    )

    payload = _run(module._cancel_task("task-1"))

    assert payload["code"] == 0, payload
    assert payload["data"] is True, payload
    assert document_service.access_checks == []
    assert knowledgebase_service.lookup_calls == [
        {"graphrag_task_id": "task-1"},
        {"raptor_task_id": "task-1"},
        {"mindmap_task_id": "task-1"},
    ]
    assert len(knowledgebase_service.lookup_calls) == 3
    assert knowledgebase_service.access_checks == [("kb-1", "user-1")]
    assert api_service.query_calls == []
    assert api_service.message_lookup_calls == []
    assert redis_conn.set_calls == [("task-1-cancel", "x")]
    assert module.TaskService.model.update_calls == 1
    assert document_service.update_calls == []


@pytest.mark.p2
def test_cancel_task_rejects_inaccessible_dataset_index_task(monkeypatch):
    task = SimpleNamespace(doc_id="graph-raptor-doc", progress=0, progress_msg="")
    module, document_service, knowledgebase_service, api_service, redis_conn = _load_task_module(
        monkeypatch,
        task=task,
        document=None,
        index_task_kb={"field": "graphrag_task_id", "task_id": "task-1", "kb_id": "kb-1"},
        index_accessible=False,
    )

    payload = _run(module._cancel_task("task-1"))

    assert payload["code"] == 109, payload
    assert payload["data"] is False, payload
    assert payload["message"] == "No authorization.", payload
    assert document_service.access_checks == []
    assert knowledgebase_service.lookup_calls == [{"graphrag_task_id": "task-1"}]
    assert knowledgebase_service.access_checks == [("kb-1", "user-1")]
    assert api_service.query_calls == []
    assert api_service.message_lookup_calls == []
    assert redis_conn.set_calls == []
    assert module.TaskService.model.update_calls == 0
    assert document_service.update_calls == []


@pytest.mark.p2
def test_cancel_task_authorizes_canvas_debug_tasks_from_user_conversation(monkeypatch):
    task = SimpleNamespace(doc_id="canvas-debug-doc", progress=0, progress_msg="")
    module, document_service, knowledgebase_service, api_service, redis_conn = _load_task_module(
        monkeypatch,
        task=task,
        document=None,
        document_accessible=False,
        canvas_messages=[[{"role": "user", "id": "task-1"}]],
    )

    payload = _run(module._cancel_task("task-1"))

    assert payload["code"] == 0, payload
    assert payload["data"] is True, payload
    assert document_service.access_checks == []
    assert knowledgebase_service.access_checks == []
    assert api_service.query_calls == []
    assert api_service.message_lookup_calls == [("user-1", "task-1")]
    assert redis_conn.set_calls == [("task-1-cancel", "x")]
    assert module.TaskService.model.update_calls == 1
    assert document_service.update_calls == []


@pytest.mark.p2
def test_cancel_task_rejects_canvas_debug_tasks_from_other_users(monkeypatch):
    task = SimpleNamespace(doc_id="canvas-debug-doc", progress=0, progress_msg="")
    module, document_service, knowledgebase_service, api_service, redis_conn = _load_task_module(
        monkeypatch,
        task=task,
        document=None,
        document_accessible=False,
        canvas_messages=[[{"role": "user", "id": "other-task"}]],
    )

    payload = _run(module._cancel_task("task-1"))

    assert payload["code"] == 109, payload
    assert payload["data"] is False, payload
    assert payload["message"] == "No authorization.", payload
    assert document_service.access_checks == []
    assert knowledgebase_service.access_checks == []
    assert api_service.query_calls == []
    assert api_service.message_lookup_calls == [("user-1", "task-1")]
    assert redis_conn.set_calls == []
    assert module.TaskService.model.update_calls == 0
    assert document_service.update_calls == []

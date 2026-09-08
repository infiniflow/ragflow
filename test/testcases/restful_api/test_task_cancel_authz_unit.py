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
"""Authorization tests for POST /tasks/<task_id>/cancel.

Cancelling a task flips its document to CANCEL and sets the Redis cancel
flag, so the route must verify the caller can access the dataset that owns
the task's document. These tests stub the HTTP layer and services and drive
the real handler.
"""

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


def _run(coro):
    return asyncio.run(coro)


REDIS_SETS: list = []


def _load_module(monkeypatch, *, accessible_kb_ids):
    repo_root = Path(__file__).resolve().parents[3]

    quart_mod = ModuleType("quart")
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="user-a")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)
    api_pkg.apps = apps_mod

    api_utils_mod = ModuleType("api.utils.api_utils")

    def get_json_result(data=None, message="success", code=0):
        return {"code": code, "data": data, "message": message}

    async def get_request_json():
        return {}

    def validate_request(*_keys):
        def _decorator(func):
            async def _wrapper(*args, **kwargs):
                return await func(*args, **kwargs)

            return _wrapper

        return _decorator

    api_utils_mod.get_json_result = get_json_result
    api_utils_mod.get_request_json = get_request_json
    api_utils_mod.validate_request = validate_request
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    constants_mod = ModuleType("common.constants")

    class RetCode:
        ARGUMENT_ERROR = 101
        DATA_ERROR = 102
        AUTHENTICATION_ERROR = 109
        CONNECTION_ERROR = 503

    class _Status:
        def __init__(self, v):
            self.value = v

    class TaskStatus:
        RUNNING = _Status("1")
        SCHEDULE = _Status("0")
        CANCEL = _Status("4")

    constants_mod.RetCode = RetCode
    constants_mod.TaskStatus = TaskStatus
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    redis_mod = ModuleType("rag.utils.redis_conn")

    class _Redis:
        @staticmethod
        def set(key, value):
            REDIS_SETS.append(key)

    redis_mod.REDIS_CONN = _Redis
    monkeypatch.setitem(sys.modules, "rag.utils.redis_conn", redis_mod)

    # TaskService stub: task-1 -> doc-1 -> kb-mine; task-2 -> doc-2 -> kb-theirs
    task_svc_mod = ModuleType("api.db.services.task_service")

    class _UpdateChain:
        def where(self, *a):
            return self

        def execute(self):
            return 1

    class _TaskModel:
        id = "task-id"
        progress_msg = ""
        progress = 0

        @staticmethod
        def update(**_kw):
            return _UpdateChain()

    class _TaskService:
        model = _TaskModel

        @staticmethod
        def get_by_id(task_id):
            if task_id == "task-1":
                return True, SimpleNamespace(id="task-1", doc_id="doc-1")
            if task_id == "task-2":
                return True, SimpleNamespace(id="task-2", doc_id="doc-2")
            return False, None

    task_svc_mod.TaskService = _TaskService
    task_svc_mod.CANVAS_DEBUG_DOC_ID = "dataflow_x"
    task_svc_mod.GRAPH_RAPTOR_FAKE_DOC_ID = "graph_raptor_x"
    monkeypatch.setitem(sys.modules, "api.db.services.task_service", task_svc_mod)

    doc_svc_mod = ModuleType("api.db.services.document_service")

    class _DocumentService:
        @staticmethod
        def get_by_id(doc_id):
            if doc_id == "doc-1":
                return True, SimpleNamespace(id="doc-1", kb_id="kb-mine", run="1", progress_msg="")
            if doc_id == "doc-2":
                return True, SimpleNamespace(id="doc-2", kb_id="kb-theirs", run="1", progress_msg="")
            return False, None

        @staticmethod
        def update_by_id(_id, _data):
            return 1

    doc_svc_mod.DocumentService = _DocumentService
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", doc_svc_mod)

    kb_svc_mod = ModuleType("api.db.services.knowledgebase_service")

    class _KnowledgebaseService:
        @staticmethod
        def accessible(kb_id, user_id):
            return kb_id in accessible_kb_ids

    kb_svc_mod.KnowledgebaseService = _KnowledgebaseService
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", kb_svc_mod)

    for mod_name in list(sys.modules.keys()):
        if mod_name.startswith("api.apps.restful_apis.task_api"):
            del sys.modules[mod_name]

    module_name = "api.apps.restful_apis.task_api"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "task_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


@pytest.fixture(scope="session")
def auth():
    return "test-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


@pytest.fixture(autouse=True)
def _clean_redis_log():
    REDIS_SETS.clear()


@pytest.mark.p2
def test_cancel_own_task_succeeds(monkeypatch):
    module = _load_module(monkeypatch, accessible_kb_ids={"kb-mine"})
    res = _run(module.cancel_task("task-1"))
    assert res["code"] == 0, res
    assert "task-1-cancel" in REDIS_SETS


@pytest.mark.p2
def test_cancel_other_tenants_task_denied(monkeypatch):
    module = _load_module(monkeypatch, accessible_kb_ids={"kb-mine"})
    res = _run(module.cancel_task("task-2"))
    assert res["code"] != 0, f"cross-tenant cancel must be denied, got {res}"
    assert "task-2-cancel" not in REDIS_SETS, "cancel flag was set for another tenant's task"

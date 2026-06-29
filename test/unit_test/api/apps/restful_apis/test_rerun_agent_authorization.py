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
"""Regression tests for pipeline rerun authorization in agent_api.rerun_agent."""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest

_MODULE_NAME = "test_rerun_agent_agent_api"
_REPO_ROOT = Path(__file__).resolve().parents[5]
_AGENT_API_PATH = _REPO_ROOT / "api" / "apps" / "restful_apis" / "agent_api.py"


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _load_agent_api_for_rerun(monkeypatch, *, documents_info, accessible):
    monkeypatch.delitem(sys.modules, _MODULE_NAME, raising=False)

    destructive_calls = {"clear": 0, "update": 0, "delete_tasks": 0, "queue": 0, "index_delete": 0}

    acc_fn = accessible if callable(accessible) else (lambda *_a, **_k: accessible)

    _stub(monkeypatch, "api.apps", current_user=SimpleNamespace(id="user-owner"), login_required=lambda func: func)
    _stub(monkeypatch, "api.apps.services.canvas_replica_service", CanvasReplicaService=SimpleNamespace())
    _stub(monkeypatch, "api.db", CanvasCategory=SimpleNamespace())

    task_model = SimpleNamespace()
    task_model.doc_id = "doc_id_field"
    _stub(monkeypatch, "api.db.db_models", Task=task_model)

    _stub(
        monkeypatch,
        "api.db.services.api_service",
        API4ConversationService=SimpleNamespace(
            get_by_id=lambda *_a, **_k: (False, None),
            save=lambda **_kwargs: True,
            delete_by_id=lambda *_a, **_k: True,
            query=lambda **_kwargs: [],
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.canvas_service",
        CanvasTemplateService=SimpleNamespace(),
        UserCanvasService=SimpleNamespace(accessible=lambda *_a, **_k: True, query=lambda **_kwargs: []),
        completion=lambda *_a, **_k: None,
        completion_openai=lambda *_a, **_k: None,
    )
    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(
            accessible=acc_fn,
            clear_chunk_num_when_rerun=lambda _doc_id: destructive_calls.__setitem__("clear", destructive_calls["clear"] + 1),
            update_by_id=lambda *_a, **_k: destructive_calls.__setitem__("update", destructive_calls["update"] + 1) or True,
        ),
    )
    _stub(monkeypatch, "api.db.services.file_service", FileService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.knowledgebase_service", KnowledgebaseService=SimpleNamespace())

    def _update_log(*_a, **_k):
        destructive_calls["update_log"] = True

    _stub(
        monkeypatch,
        "api.db.services.pipeline_operation_log_service",
        PipelineOperationLogService=SimpleNamespace(
            get_documents_info=lambda _log_id: documents_info,
            update_by_id=_update_log,
        ),
    )

    class _TaskService:
        @staticmethod
        def filter_delete(*_a, **_k):
            destructive_calls["delete_tasks"] += 1

    _stub(
        monkeypatch,
        "api.db.services.task_service",
        CANVAS_DEBUG_DOC_ID="",
        TaskService=_TaskService,
        queue_dataflow=lambda *_a, **_k: destructive_calls.__setitem__("queue", destructive_calls["queue"] + 1),
    )
    _stub(monkeypatch, "api.db.services.user_service", TenantService=SimpleNamespace(), UserService=SimpleNamespace(get_by_id=lambda *_a, **_k: (False, None)))
    _stub(monkeypatch, "api.db.services.user_canvas_version", UserCanvasVersionService=SimpleNamespace())

    request_body = {"id": "log-victim", "component_id": "Parser:0", "dsl": {"path": [], "components": {}}}

    _stub(
        monkeypatch,
        "api.utils.api_utils",
        add_tenant_id_to_kwargs=lambda func: func,
        check_duplicate_ids=lambda ids, _kind="item": (ids, []),
        get_data_error_result=lambda message="Sorry": {"code": 102, "message": message, "data": None},
        get_error_data_result=lambda message="Sorry", code=102: {"code": code, "message": message, "data": None},
        get_json_result=lambda code=0, message="", data=None: {"code": code, "message": message, "data": data},
        get_result=lambda **kwargs: kwargs,
        get_request_json=lambda: _awaitable(request_body),
        server_error_response=lambda exc: {"code": 500, "message": str(exc)},
        validate_request=lambda *_a, **_k: lambda func: func,
    )

    doc_store = SimpleNamespace(
        index_exist=lambda *_a, **_k: True,
        delete=lambda *_a, **_k: destructive_calls.__setitem__("index_delete", destructive_calls["index_delete"] + 1),
    )
    common_settings = _stub(
        monkeypatch,
        "common.settings",
        retriever=SimpleNamespace(),
        kg_retriever=SimpleNamespace(),
        docStoreConn=doc_store,
    )
    monkeypatch.setitem(sys.modules, "common", SimpleNamespace(settings=common_settings))
    _stub(monkeypatch, "common.ssrf_guard", assert_host_is_safe=lambda *_a, **_k: None)
    _stub(monkeypatch, "common.constants", RetCode=SimpleNamespace(OPERATING_ERROR=109))
    _stub(
        monkeypatch,
        "common.misc_utils",
        get_uuid=lambda: "task-uuid",
        thread_pool_exec=lambda fn, *a, **k: fn(*a, **k),
    )

    rag_nlp_mod = ModuleType("rag.nlp")
    rag_nlp_mod.search = SimpleNamespace(index_name=lambda _tenant_id: "idx")
    monkeypatch.setitem(sys.modules, "rag.nlp", rag_nlp_mod)

    spec = importlib.util.spec_from_file_location(_MODULE_NAME, _AGENT_API_PATH)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, _MODULE_NAME, module)
    spec.loader.exec_module(module)
    module._destructive_calls = destructive_calls
    module._request_body = request_body
    return module


def _awaitable(value):
    async def _co():
        return value

    return _co()


@pytest.mark.p1
class TestRerunAgentAuthorization:
    def test_cross_tenant_log_id_is_rejected(self, monkeypatch):
        victim_doc = {
            "id": "doc-victim",
            "name": "secret.pdf",
            "progress": 0,
            "kb_id": "kb-victim",
        }
        module = _load_agent_api_for_rerun(
            monkeypatch,
            documents_info=[victim_doc],
            accessible=lambda _doc_id, user_id: user_id == "user-owner",
        )

        result = asyncio.run(module.rerun_agent(tenant_id="user-attacker"))

        assert result == {"code": 102, "message": "Document not found.", "data": None}
        assert module._destructive_calls["clear"] == 0
        assert module._destructive_calls["queue"] == 0
        assert module._destructive_calls["index_delete"] == 0

    def test_missing_log_returns_same_message(self, monkeypatch):
        module = _load_agent_api_for_rerun(
            monkeypatch,
            documents_info=[],
            accessible=lambda *_a, **_k: True,
        )

        missing = asyncio.run(module.rerun_agent(tenant_id="user-owner"))

        module = _load_agent_api_for_rerun(
            monkeypatch,
            documents_info=[{"id": "doc-victim", "name": "x.pdf", "progress": 0, "kb_id": "kb-victim"}],
            accessible=lambda *_a, **_k: False,
        )
        unauthorized = asyncio.run(module.rerun_agent(tenant_id="user-owner"))

        assert missing["message"] == unauthorized["message"] == "Document not found."

    def test_authorized_rerun_proceeds(self, monkeypatch):
        victim_doc = {
            "id": "doc-owner",
            "name": "mine.pdf",
            "progress": 0,
            "kb_id": "kb-owner",
        }
        module = _load_agent_api_for_rerun(
            monkeypatch,
            documents_info=[victim_doc],
            accessible=lambda *_a, **_k: True,
        )

        result = asyncio.run(module.rerun_agent(tenant_id="user-owner"))

        assert result == {"code": 0, "message": "", "data": True}
        assert module._destructive_calls["clear"] == 1
        assert module._destructive_calls["queue"] == 1

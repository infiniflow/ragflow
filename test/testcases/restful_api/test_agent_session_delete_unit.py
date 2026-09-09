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

"""Unit tests for DELETE /agents/<agent_id>/sessions/<session_id>.

On a team-shared agent the single-session delete must be limited to the
canvas owner or the session's creator; other team members see the session
readonly. Mirrors the Go API rule (internal/service/agent_sessions.go).
"""

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


def _module_stub(name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    return mod


def _load_agent_api(monkeypatch, *, tenant_id, canvas_owner_id, session_user_id):
    repo_root = Path(__file__).resolve().parents[3]

    monkeypatch.setitem(sys.modules, "jwt", _module_stub("jwt"))
    monkeypatch.setitem(sys.modules, "quart", _module_stub("quart", Response=None, jsonify=None, request=SimpleNamespace(args={}), make_response=None))
    monkeypatch.setitem(sys.modules, "api.utils.file_response", _module_stub("api.utils.file_response", apply_download_file_response_headers=None, apply_preview_file_response_headers=None, resolve_attachment_content_type=None))

    monkeypatch.setitem(
        sys.modules,
        "api.apps",
        _module_stub(
            "api.apps",
            AUTH_JWT="jwt",
            AUTH_API="api",
            AUTH_BETA="beta",
            current_user=SimpleNamespace(id=tenant_id),
            login_required=lambda *a, **k: (a[0] if a and callable(a[0]) else (lambda f: f)),
        ),
    )
    monkeypatch.setitem(sys.modules, "api.apps.services.canvas_replica_service", _module_stub("api.apps.services.canvas_replica_service", CanvasReplicaService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db", _module_stub("api.db", CanvasCategory=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.db_models", _module_stub("api.db.db_models", Task=SimpleNamespace()))

    conv = SimpleNamespace(id="sess_1", dialog_id="agent_1", user_id=session_user_id)
    deleted = []

    def _delete(session_id):
        deleted.append(session_id)
        return True

    api4 = SimpleNamespace(
        get_by_id=lambda session_id: (True, conv),
        delete_by_id=_delete,
    )
    monkeypatch.setitem(sys.modules, "api.db.services.api_service", _module_stub("api.db.services.api_service", API4ConversationService=api4))

    user_canvas = SimpleNamespace(
        accessible=lambda canvas_id, tid: True,
        query=lambda **kw: [SimpleNamespace(id="agent_1")] if kw.get("user_id") == canvas_owner_id and kw.get("id") == "agent_1" else [],
    )
    monkeypatch.setitem(
        sys.modules,
        "api.db.services.canvas_service",
        _module_stub("api.db.services.canvas_service", CanvasTemplateService=SimpleNamespace(), UserCanvasService=user_canvas, completion=None, completion_openai=None),
    )
    for svc, names in {
        "api.db.services.document_service": ["DocumentService"],
        "api.db.services.file_service": ["FileService"],
        "api.db.services.knowledgebase_service": ["KnowledgebaseService"],
        "api.db.services.pipeline_operation_log_service": ["PipelineOperationLogService"],
        "api.db.services.user_canvas_version": ["UserCanvasVersionService"],
    }.items():
        monkeypatch.setitem(sys.modules, svc, _module_stub(svc, **{n: SimpleNamespace() for n in names}))
    monkeypatch.setitem(
        sys.modules,
        "api.db.services.task_service",
        _module_stub("api.db.services.task_service", CANVAS_DEBUG_DOC_ID="debug", TaskService=SimpleNamespace(), queue_dataflow=None),
    )
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", _module_stub("api.db.services.user_service", TenantService=SimpleNamespace(), UserService=SimpleNamespace()))

    def _envelope(data=None, message="", code=0):
        return {"code": code, "message": message, "data": data}

    monkeypatch.setitem(
        sys.modules,
        "api.utils.api_utils",
        _module_stub(
            "api.utils.api_utils",
            add_tenant_id_to_kwargs=lambda f: f,
            check_duplicate_ids=None,
            get_data_error_result=lambda *a, **kw: _envelope(None, kw.get("message", a[0] if a else ""), 102),
            get_error_data_result=lambda *a, **kw: _envelope(None, kw.get("message", a[0] if a else ""), 102),
            get_json_result=lambda **kw: _envelope(kw.get("data"), kw.get("message", ""), kw.get("code", 0)),
            get_result=lambda **kw: _envelope(kw.get("data"), kw.get("message", ""), kw.get("code", 0)),
            get_request_json=None,
            server_error_response=lambda e: _envelope(None, str(e), 100),
            validate_request=lambda *_a, **_k: (lambda f: f),
        ),
    )
    monkeypatch.setitem(
        sys.modules,
        "api.utils.pagination_utils",
        _module_stub("api.utils.pagination_utils", DEFAULT_PAGE=1, DEFAULT_PAGE_SIZE=30, validate_rest_api_ids=lambda ids, name: ids, validate_rest_api_page=lambda p: int(p), validate_rest_api_page_size=lambda s: int(s)),
    )
    monkeypatch.setitem(sys.modules, "common", _module_stub("common", settings=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "common.ssrf_guard", _module_stub("common.ssrf_guard", assert_host_is_safe=None))
    monkeypatch.setitem(sys.modules, "common.constants", _module_stub("common.constants", RetCode=SimpleNamespace(OPERATING_ERROR=103, AUTHENTICATION_ERROR=109)))

    async def _tpe(fn, *args):
        return fn(*args)

    monkeypatch.setitem(sys.modules, "common.misc_utils", _module_stub("common.misc_utils", get_uuid=lambda: "uuid", thread_pool_exec=_tpe))

    module_name = "test_agent_api_unit_module"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "agent_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module, deleted


@pytest.mark.p2
def test_team_member_cannot_delete_other_users_session(monkeypatch):
    module, deleted = _load_agent_api(monkeypatch, tenant_id="tenant_member", canvas_owner_id="tenant_owner", session_user_id="tenant_owner")
    res = module.delete_agent_session_item(agent_id="agent_1", session_id="sess_1", tenant_id="tenant_member")
    assert res["data"] is False
    assert res["message"] == "shared session is readonly"
    assert deleted == [], "readonly rejection must not delete the session"


@pytest.mark.p2
def test_owner_can_delete_any_session(monkeypatch):
    module, _ = _load_agent_api(monkeypatch, tenant_id="tenant_owner", canvas_owner_id="tenant_owner", session_user_id="tenant_member")
    res = module.delete_agent_session_item(agent_id="agent_1", session_id="sess_1", tenant_id="tenant_owner")
    assert res["data"] is True


@pytest.mark.p2
def test_team_member_can_delete_own_session(monkeypatch):
    module, _ = _load_agent_api(monkeypatch, tenant_id="tenant_member", canvas_owner_id="tenant_owner", session_user_id="tenant_member")
    res = module.delete_agent_session_item(agent_id="agent_1", session_id="sess_1", tenant_id="tenant_member")
    assert res["data"] is True

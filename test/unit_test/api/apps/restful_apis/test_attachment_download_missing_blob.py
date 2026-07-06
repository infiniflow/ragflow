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
"""Regression test for `/api/v1/agents/attachments/<attachment_id>/download` (#15502).

Without the empty-blob guard, `make_response(None)` raises `TypeError`
and the handler's blanket `except Exception` converts it to HTTP 500.
Same bug class as #15365 on document preview. This test pins the
missing-storage path to a structured 4xx response.
"""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


class _LenientModule(ModuleType):
    """A stub module that yields a harmless placeholder for any attribute that
    wasn't explicitly provided. agent_api.py's top-level `from <mod> import a, b`
    only needs every imported name to exist; symbols not on the
    download_attachment path are never called, so a no-op placeholder is safe.
    This keeps the test from rotting each time agent_api.py grows an import.
    """

    def __getattr__(self, _name):
        return lambda *_a, **_k: None


class _Headers(dict):
    def set(self, key, value):
        self[key] = value


def _stub(monkeypatch, name, **attrs):
    mod = _LenientModule(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _load_agent_api(monkeypatch, *, storage_get):
    """Load agent_api.py with the minimum stubs to exercise download_attachment."""

    async def _make_response(payload):
        if payload is None:
            raise TypeError("response value cannot be None")
        return SimpleNamespace(payload=payload, headers=_Headers())

    _stub(
        monkeypatch,
        "api.apps",
        current_user=SimpleNamespace(id="tenant-1"),
        login_required=lambda func: func,
    )
    _stub(monkeypatch, "api.apps.services.canvas_replica_service", CanvasReplicaService=SimpleNamespace())
    _stub(monkeypatch, "api.db", CanvasCategory=SimpleNamespace())
    _stub(monkeypatch, "api.db.db_models", Task=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.api_service",
        API4ConversationService=SimpleNamespace(
            get_by_id=lambda _id: (False, None),
            save=lambda **_k: True,
            delete_by_id=lambda *_a, **_k: True,
            query=lambda **_k: [],
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.canvas_service",
        CanvasTemplateService=SimpleNamespace(),
        UserCanvasService=SimpleNamespace(accessible=lambda *_a, **_k: True, query=lambda **_k: []),
        completion=lambda *_a, **_k: None,
        completion_openai=lambda *_a, **_k: None,
    )
    _stub(monkeypatch, "api.db.services.document_service", DocumentService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.file_service", FileService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.knowledgebase_service", KnowledgebaseService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.pipeline_operation_log_service", PipelineOperationLogService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.task_service",
        CANVAS_DEBUG_DOC_ID="",
        TaskService=SimpleNamespace(),
        queue_dataflow=lambda *_a, **_k: None,
    )
    _stub(
        monkeypatch,
        "api.db.services.user_service",
        TenantService=SimpleNamespace(),
        UserService=SimpleNamespace(get_by_id=lambda *_a, **_k: (False, None)),
    )
    _stub(monkeypatch, "api.db.services.user_canvas_version", UserCanvasVersionService=SimpleNamespace())

    _stub(
        monkeypatch,
        "api.utils.api_utils",
        construct_json_result=lambda **kw: {"kind": "json", **kw},
        get_data_error_result=lambda message="", code=0, data=False: {"kind": "data_error", "message": message},
        get_error_data_result=lambda *_a, **_k: {"kind": "error"},
        get_result=lambda *_a, **_k: {"kind": "result"},
        get_json_result=lambda *_a, **_k: {"kind": "json_result"},
        server_error_response=lambda e: {"kind": "server_error", "error": str(e)},
        add_tenant_id_to_kwargs=lambda func: func,
        get_request_json=lambda: {},
        # Used as `@validate_request(...)` decorator factory at module level, so it
        # must return an identity decorator (the lenient fallback would return None
        # and `@None` raises TypeError during import).
        validate_request=lambda *_a, **_k: lambda func: func,
    )
    _stub(
        monkeypatch,
        "common.settings",
        retriever=SimpleNamespace(),
        kg_retriever=SimpleNamespace(),
        # download_attachment reads settings.STORAGE_IMPL.get after
        # `from common import settings` rebinds the module's `settings` name.
        STORAGE_IMPL=SimpleNamespace(get=storage_get),
    )
    _stub(monkeypatch, "common.ssrf_guard", assert_host_is_safe=lambda *_a, **_k: None)
    _stub(monkeypatch, "common.constants", RetCode=SimpleNamespace())
    _stub(monkeypatch, "api.utils.pagination_utils", validate_rest_api_page_size=lambda *_a, **_k: None)
    _stub(monkeypatch, "peewee")

    async def _thread_pool_exec(fn, *args, **kwargs):
        # download_attachment does `await thread_pool_exec(STORAGE_IMPL.get, ...)`;
        # run the callable inline so the storage stub's return value flows through.
        return fn(*args, **kwargs)

    _stub(monkeypatch, "common.misc_utils", get_uuid=lambda: "uuid", thread_pool_exec=_thread_pool_exec)
    _stub(
        monkeypatch,
        "api.utils.web_utils",
        CONTENT_TYPE_MAP={"markdown": "text/markdown"},
        apply_safe_file_response_headers=lambda *_a, **_k: None,
    )

    # Lenient: agent_api.py imports Response, jsonify, request, make_response from
    # quart; only request and make_response are on the download path, the rest
    # resolve to placeholders so the import line can't break the test.
    quart_stub = _LenientModule("quart")
    quart_stub.request = SimpleNamespace(method="GET", args={"ext": "markdown"})
    quart_stub.make_response = _make_response
    monkeypatch.setitem(sys.modules, "quart", quart_stub)

    # parents[5] = repo root from test/unit_test/api/apps/restful_apis/<file>
    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "agent_api.py"
    spec = importlib.util.spec_from_file_location("test_agent_api_module", module_path)
    module = importlib.util.module_from_spec(spec)
    # `manager` must exist before exec so the @manager.route decorators run.
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_agent_api_module", module)
    spec.loader.exec_module(module)

    # Pin every module global that download_attachment resolves at call time.
    # The sys.modules stubs above only guarantee the import *succeeds*. In the
    # full test environment `from common import settings` (and the api_utils /
    # misc_utils imports) can bind the REAL modules instead of our stubs — e.g.
    # if another test already imported common.settings, it is an attribute on the
    # already-loaded `common` package and the sys.modules stub is bypassed — so
    # settings.STORAGE_IMPL would be the uninitialised real None and the handler
    # would 500 with "'NoneType' object has no attribute 'get'". Overriding the
    # globals on the loaded module makes behaviour identical in both the
    # bare-stub (local) and full-dependency (CI) environments.
    module.settings = SimpleNamespace(STORAGE_IMPL=SimpleNamespace(get=storage_get))
    module.request = SimpleNamespace(method="GET", args={"ext": "markdown"})
    module.make_response = _make_response
    module.thread_pool_exec = _thread_pool_exec
    module.get_data_error_result = lambda message="", **_k: {"kind": "data_error", "message": message}
    module.server_error_response = lambda e: {"kind": "server_error", "error": str(e)}
    module.CONTENT_TYPE_MAP = {"markdown": "text/markdown"}
    module.apply_safe_file_response_headers = lambda *_a, **_k: None
    return module


@pytest.mark.p1
class TestAttachmentDownloadMissingBlob:
    """Regression for #15502: missing-blob → structured 4xx, not HTTP 500."""

    def test_empty_blob_returns_not_found(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Storage returns None (orphaned metadata) → 'Document not found!' 4xx,
        not a TypeError 500 from make_response(None)."""
        module = _load_agent_api(monkeypatch, storage_get=lambda *_a, **_k: None)
        result = asyncio.run(module.download_attachment(tenant_id="t1", attachment_id="orphan"))
        # 'data_error' is the get_data_error_result shape from our stub above.
        # If the empty-blob guard is missing, make_response(None) raises and
        # the result instead has `kind == "server_error"`.
        assert isinstance(result, dict) and result.get("kind") == "data_error", result
        assert "not found" in result["message"].lower()

    def test_empty_bytes_returns_not_found(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Same path when storage returns b'' instead of None."""
        module = _load_agent_api(monkeypatch, storage_get=lambda *_a, **_k: b"")
        result = asyncio.run(module.download_attachment(tenant_id="t1", attachment_id="empty"))
        assert isinstance(result, dict) and result.get("kind") == "data_error"
        assert "not found" in result["message"].lower()

    def test_nonempty_blob_proceeds(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Happy path: real bytes flow through make_response untouched."""
        module = _load_agent_api(monkeypatch, storage_get=lambda *_a, **_k: b"PDFDATA")
        result = asyncio.run(module.download_attachment(tenant_id="t1", attachment_id="good"))
        # SimpleNamespace from our _make_response stub
        assert hasattr(result, "payload") and result.payload == b"PDFDATA"

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
"""Regression test for `/api/v1/documents/<doc_id>/preview` (#15501).

PR #15146 dropped the `DocumentService.accessible(doc_id, user_id)` check
from the REST preview handler. Any authenticated user could then download
any tenant's document bytes by guessing / knowing its doc_id. This test
locks in the restored check by asserting that a caller whose tenant does
NOT own the document gets the same `Document not found!` response a missing
doc would produce, and that the storage backend is never touched.
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


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _login_required(func=None, **_kwargs):
    if func is not None and callable(func):
        return func

    def _decorator(inner):
        return inner

    return _decorator


def _load_document_api(
    monkeypatch,
    *,
    doc_get_by_id,
    accessible_fn,
    storage_get,
):
    """Load api/apps/restful_apis/document_api.py with the minimum stubs."""

    async def _make_response(payload):
        return SimpleNamespace(payload=payload, headers={})

    _stub(
        monkeypatch, "api.apps",
        AUTH_JWT="JWT",
        AUTH_API="API",
        AUTH_BETA="BETA",
        current_user=SimpleNamespace(id="caller-tenant"),
        login_required=_login_required,
    )
    _stub(monkeypatch, "api.constants", FILE_NAME_LEN_LIMIT=128, IMG_BASE64_PREFIX="data:image/")
    _stub(
        monkeypatch, "api.apps.services.document_api_service",
        validate_document_update_fields=lambda *_a, **_k: None,
        map_doc_keys=lambda *_a, **_k: None,
        map_doc_keys_with_run_status=lambda *_a, **_k: None,
        update_document_name_only=lambda *_a, **_k: None,
        update_chunk_method=lambda *_a, **_k: None,
        update_document_status_only=lambda *_a, **_k: None,
        reset_document_for_reparse=lambda *_a, **_k: None,
    )
    _stub(monkeypatch, "api.db", VALID_FILE_TYPES=(), FileType=SimpleNamespace(VISUAL="visual"))
    _stub(monkeypatch, "api.db.services", duplicate_name=lambda *_a, **_k: "x")
    _stub(monkeypatch, "api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace())
    _stub(monkeypatch, "api.db.db_models", Task=SimpleNamespace())
    _stub(
        monkeypatch, "api.db.services.document_service",
        DocumentService=SimpleNamespace(get_by_id=lambda _id: doc_get_by_id, accessible=accessible_fn),
    )
    _stub(
        monkeypatch, "api.db.services.file2document_service",
        File2DocumentService=SimpleNamespace(get_storage_address=lambda **_k: ("bucket", "key")),
    )
    _stub(monkeypatch, "api.db.services.file_service", FileService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.knowledgebase_service", KnowledgebaseService=SimpleNamespace())
    _stub(monkeypatch, "api.common.check_team_permission", check_kb_team_permission=lambda *_a, **_k: True)
    _stub(monkeypatch, "api.db.services.task_service", TaskService=SimpleNamespace(), cancel_all_task_of=lambda *_a, **_k: None)
    _stub(
        monkeypatch, "api.utils.api_utils",
        construct_json_result=lambda **kw: {"kind": "json", **kw},
        get_data_error_result=lambda message="", code=0, data=False: {"kind": "data_error", "message": message},
        get_error_data_result=lambda *_a, **_k: {"kind": "error"},
        get_result=lambda *_a, **_k: {"kind": "result"},
        get_json_result=lambda *_a, **_k: {"kind": "json_result"},
        server_error_response=lambda e: {"kind": "server_error", "error": str(e)},
        add_tenant_id_to_kwargs=lambda *_a, **_k: None,
        get_request_json=lambda: {},
        get_error_argument_result=lambda *_a, **_k: {"kind": "error_argument"},
        check_duplicate_ids=lambda *_a, **_k: True,
    )
    _stub(monkeypatch, "api.utils.pagination_utils", validate_rest_api_page_size=lambda *_a, **_k: 10)
    _stub(
        monkeypatch, "api.utils.validation_utils",
        UpdateDocumentReq=type("UpdateDocumentReq", (), {}),
        DeleteDocumentReq=type("DeleteDocumentReq", (), {}),
        format_validation_error_message=lambda *_a, **_k: "",
        validate_and_parse_json_request=lambda *_a, **_k: ({}, None),
    )
    _stub(monkeypatch, "common", settings=SimpleNamespace(STORAGE_IMPL=SimpleNamespace(get=storage_get)))
    _stub(
        monkeypatch, "common.constants",
        ParserType=SimpleNamespace(NAIVE="naive"),
        RetCode=SimpleNamespace(AUTHENTICATION_ERROR=109, DATA_ERROR=102),
        TaskStatus=SimpleNamespace(RUNNING="1"),
        SANDBOX_ARTIFACT_BUCKET="sandbox",
    )
    _stub(
        monkeypatch, "common.metadata_utils",
        convert_conditions=lambda c: c, meta_filter=lambda *_a, **_k: [],
        turn2jsonschema=lambda *_a, **_k: {},
    )
    _stub(
        monkeypatch, "common.misc_utils",
        get_uuid=lambda: "uuid",
        thread_pool_exec=lambda fn, *a, **k: storage_get(*a, **k),
    )
    _stub(
        monkeypatch, "api.utils.file_utils",
        filename_type=lambda *_a, **_k: "doc",
        thumbnail=lambda *_a, **_k: b"",
    )
    _stub(
        monkeypatch, "api.utils.web_utils",
        CONTENT_TYPE_MAP={"pdf": "application/pdf"},
        html2pdf=lambda *_a, **_k: b"",
        is_valid_url=lambda *_a, **_k: True,
        apply_safe_file_response_headers=lambda *_a, **_k: None,
    )
    _stub(monkeypatch, "common.ssrf_guard", assert_url_is_safe=lambda *_a, **_k: None)
    _stub(monkeypatch, "rag.nlp", search=SimpleNamespace(index_name=lambda *_a, **_k: "idx"))

    quart_stub = ModuleType("quart")
    quart_stub.request = SimpleNamespace(method="GET", args={})
    quart_stub.make_response = _make_response
    quart_stub.send_file = lambda *_a, **_k: None
    monkeypatch.setitem(sys.modules, "quart", quart_stub)

    # Third-party deps imported at module top — stub if not installed.
    if "peewee" not in sys.modules:
        _stub(monkeypatch, "peewee", OperationalError=type("OperationalError", (Exception,), {}))
    if "pydantic" not in sys.modules:
        _stub(monkeypatch, "pydantic", ValidationError=type("ValidationError", (Exception,), {}))

    # api.apps.settings is reached as `settings.STORAGE_IMPL.get`.
    _stub(
        monkeypatch, "api.apps.settings",
        STORAGE_IMPL=SimpleNamespace(get=storage_get),
    )

    # parents[5] = repo root (test/unit_test/api/apps/restful_apis/<file>)
    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "document_api.py"
    spec = importlib.util.spec_from_file_location("test_document_api_module", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    module.settings = SimpleNamespace(STORAGE_IMPL=SimpleNamespace(get=storage_get))
    monkeypatch.setitem(sys.modules, "test_document_api_module", module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p1
class TestDocumentPreviewAccessCheck:
    """Regression for #15501: cross-tenant document preview via /documents/<id>/preview."""

    def test_cross_tenant_preview_is_denied(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """A caller whose tenant does NOT own the document gets the same
        'Document not found!' response a missing doc would, and storage is
        never read."""
        doc = SimpleNamespace(id="DOC_VICTIM", name="secrets.pdf", type="application")
        storage_calls: list[tuple] = []

        def _storage_get(*args, **kwargs):
            storage_calls.append((args, kwargs))
            return b"SECRET BYTES"

        def _accessible_only_for_owner(_doc_id, user_id):
            return user_id == "tenant-owner"

        module = _load_document_api(
            monkeypatch,
            doc_get_by_id=(True, doc),
            accessible_fn=_accessible_only_for_owner,
            storage_get=_storage_get,
        )

        # current_user.id is "caller-tenant" via the api.apps stub above;
        # tenant-owner is "tenant-owner" → not owner → must be denied.
        result = asyncio.run(module.get("DOC_VICTIM"))

        assert isinstance(result, dict) and result.get("kind") == "data_error", result
        assert "not found" in result["message"].lower()
        assert storage_calls == [], "storage was read despite cross-tenant request"

    def test_missing_doc_returns_not_found(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Missing-doc behaviour is unchanged: same 'Document not found!' shape."""

        def _accessible_should_not_be_called(*_a, **_k):
            raise AssertionError("accessible() must not be called for a missing doc")

        module = _load_document_api(
            monkeypatch,
            doc_get_by_id=(False, None),
            accessible_fn=_accessible_should_not_be_called,
            storage_get=lambda *_a, **_k: b"",
        )

        result = asyncio.run(module.get("DOC_DOES_NOT_EXIST"))
        assert isinstance(result, dict) and result.get("kind") == "data_error"
        assert "not found" in result["message"].lower()

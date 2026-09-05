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
"""Regression test for `GET /datasets/<dataset_id>/documents?id=..&ids=..`.

`_get_docs_with_request` builds a DATA_ERROR for the both-provided case, but
returned it as a 2-tuple while `list_docs` unpacks 4, so the request raised
`ValueError` and the caller got EXCEPTION_ERROR with a Python repr instead of
the error the helper wrote.

The cases below pin the shape (four values) and the error the helper already
builds.
"""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest
from werkzeug.datastructures import MultiDict

from common.constants import RetCode

DATASET_ID = "kb-1"
OWNED_DOC_ID = "doc-owned"
OTHER_DOC_ID = "doc-other"


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


class _LenientModule(ModuleType):
    """A stub module that yields a harmless placeholder for any attribute that
    wasn't explicitly provided. document_api.py's top-level
    `from <mod> import a, b` only needs every imported name to exist; symbols
    that are not on the list_docs path are never called, so a no-op placeholder
    is safe. This keeps the test from rotting each time document_api.py grows
    an import.
    """

    def __getattr__(self, _name):
        return lambda *_a, **_k: None


def _stub(monkeypatch, name, **attrs):
    mod = _LenientModule(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _load_document_api(monkeypatch, *, args, owned_doc_ids):
    """Load document_api.py with the minimum stubs to exercise list_docs.

    `common.constants`, `api.db` and `api.utils.pagination_utils` are the real
    modules, so RetCode, VALID_FILE_TYPES and the id-count limit are the ones
    the route actually uses.
    """
    request_stub = SimpleNamespace(method="GET", args=MultiDict(args))

    def _document_query(**kwargs):
        return [SimpleNamespace()] if kwargs.get("id") in owned_doc_ids else []

    _stub(
        monkeypatch,
        "api.apps",
        AUTH_JWT="jwt",
        AUTH_API="api",
        AUTH_BETA="beta",
        current_user=SimpleNamespace(id="tenant-1"),
        login_required=lambda func=None, **_kwargs: (lambda f: f) if func is None else func,
    )
    _stub(monkeypatch, "api.apps.services.document_api_service", map_doc_keys=lambda doc: doc)
    _stub(
        monkeypatch,
        "api.db.db_models",
        API4Conversation=SimpleNamespace(),
        # Applied as `@DB.connection_context()` at module level, so it has to
        # hand back an identity decorator.
        DB=SimpleNamespace(connection_context=lambda *_a, **_k: lambda func: func),
        Task=SimpleNamespace(),
    )
    _stub(monkeypatch, "api.db.services", duplicate_name=lambda *_a, **_k: "")
    _stub(monkeypatch, "api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace(get_flatted_meta_by_kbs=lambda *_a, **_k: {}))
    _stub(monkeypatch, "api.db.services.document_counter_service", release_reparse_counters=lambda *_a, **_k: None)
    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(
            # Only the id/name ownership probes in _get_docs_with_request call
            # this; a hit is a non-empty list.
            query=_document_query,
            get_by_kb_id=lambda *_a, **_k: ([], 0),
        ),
    )
    _stub(monkeypatch, "api.db.services.file2document_service", File2DocumentService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.file_service", FileService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.knowledgebase_service", KnowledgebaseService=SimpleNamespace(accessible=lambda *_a, **_k: True))
    _stub(monkeypatch, "api.db.services.canvas_service", UserCanvasService=SimpleNamespace())
    _stub(monkeypatch, "api.common.check_team_permission", check_kb_team_permission=lambda *_a, **_k: True)
    _stub(monkeypatch, "api.db.services.task_service", TaskService=SimpleNamespace(), cancel_all_task_of=lambda *_a, **_k: None)
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        # Record what the route hands back so a test can read the code the
        # response carries.
        get_data_error_result=lambda message="", code=RetCode.DATA_ERROR, data=False: {"code": code, "message": message},
        get_error_data_result=lambda message="", **_k: {"code": RetCode.DATA_ERROR, "message": message},
        get_json_result=lambda *_a, **kwargs: {"code": RetCode.SUCCESS, "data": kwargs.get("data")},
        server_error_response=lambda e: {"code": RetCode.EXCEPTION_ERROR, "message": repr(e)},
        add_tenant_id_to_kwargs=lambda func: func,
        get_request_json=lambda: {},
        # Used as `@validate_request(...)` at module level, so it must return an
        # identity decorator (the lenient fallback returns None and `@None`
        # raises TypeError during import).
        validate_request=lambda *_a, **_k: lambda func: func,
    )
    _stub(monkeypatch, "api.utils.validation_utils", validate_and_parse_json_request=lambda *_a, **_k: ({}, None))
    _stub(monkeypatch, "common.settings", retriever=SimpleNamespace(), docStoreConn=SimpleNamespace(), STORAGE_IMPL=SimpleNamespace())
    _stub(monkeypatch, "common.metadata_utils", turn2jsonschema=lambda meta: meta)
    _stub(monkeypatch, "common.misc_utils", get_uuid=lambda: "uuid")
    _stub(monkeypatch, "common.ssrf_guard", assert_url_is_safe=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.utils.file_utils", filename_type=lambda *_a, **_k: None, thumbnail=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.utils.file_response", apply_preview_file_response_headers=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.utils.web_utils", CONTENT_TYPE_MAP={}, is_valid_url=lambda *_a, **_k: True)

    quart_stub = _LenientModule("quart")
    quart_stub.request = request_stub
    monkeypatch.setitem(sys.modules, "quart", quart_stub)

    # parents[5] = repo root from test/unit_test/api/apps/restful_apis/<file>
    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "document_api.py"
    spec = importlib.util.spec_from_file_location("test_document_api_module", module_path)
    module = importlib.util.module_from_spec(spec)
    # `manager` must exist before exec so the @manager.route decorators run.
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_document_api_module", module)
    spec.loader.exec_module(module)

    # Pin the module globals list_docs resolves at call time. The sys.modules
    # stubs only guarantee the import succeeds: in a full environment, where a
    # real module is already loaded, `from x import y` binds the real name and
    # the stub is bypassed. Rebinding here makes the run identical in the
    # bare-stub and full-dependency cases.
    module.request = request_stub
    module.KnowledgebaseService = sys.modules["api.db.services.knowledgebase_service"].KnowledgebaseService
    module.DocumentService = sys.modules["api.db.services.document_service"].DocumentService
    module.DocMetadataService = sys.modules["api.db.services.doc_metadata_service"].DocMetadataService
    api_utils = sys.modules["api.utils.api_utils"]
    module.get_data_error_result = api_utils.get_data_error_result
    module.get_error_data_result = api_utils.get_error_data_result
    module.get_json_result = api_utils.get_json_result
    module.server_error_response = api_utils.server_error_response
    module.map_doc_keys = lambda doc: doc
    return module


@pytest.mark.p1
class TestListDocsIdAndIdsConflict:
    """`id` together with `ids` must answer with the helper's own error."""

    def test_helper_returns_four_values_for_id_and_ids(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """The both-provided branch returns what `list_docs` unpacks.

        Before the fix this branch returned two values, so the unpack at the
        call site raised ValueError and the error below never reached anyone.
        """
        module = _load_document_api(
            monkeypatch,
            args=[("id", OWNED_DOC_ID), ("ids", OTHER_DOC_ID)],
            owned_doc_ids={OWNED_DOC_ID},
        )
        result = module._get_docs_with_request(module.request, DATASET_ID)

        assert len(result) == 4, f"list_docs unpacks 4 values, helper returned {len(result)}: {result}"
        err_code, err_msg, payload, total = result
        assert err_code == RetCode.DATA_ERROR
        assert err_msg.startswith("Should not provide both")
        assert payload == []
        assert total == 0

    def test_route_answers_with_the_helper_code(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """End to end: the response carries the code the helper chose.

        Before the fix the unpack raised and Quart's error handler answered
        EXCEPTION_ERROR with `repr(ValueError(...))` as the message. The call
        below routes an escaping exception through `server_error_response` the
        way the app does with `app.errorhandler(Exception)`
        (`api/apps/__init__.py:69`), so a raise reads as the response the
        client would get.
        """
        module = _load_document_api(
            monkeypatch,
            args=[("id", OWNED_DOC_ID), ("ids", OTHER_DOC_ID)],
            owned_doc_ids={OWNED_DOC_ID},
        )
        helper_code, helper_msg = module._get_docs_with_request(module.request, DATASET_ID)[:2]

        try:
            response = module.list_docs(dataset_id=DATASET_ID, tenant_id="tenant-1")
        except Exception as e:  # noqa: BLE001 - mirrors the app's blanket handler
            response = module.server_error_response(e)

        assert response["code"] != RetCode.EXCEPTION_ERROR, response
        assert response["code"] == helper_code
        assert response["message"] == helper_msg

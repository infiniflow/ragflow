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

"""Unit tests for document image / thumbnail route authorization.

GET /documents/images/<image_id> and GET /thumbnails serve document image
bytes. Both must restrict results to datasets the caller may read; these
tests stub the service layer and exercise the route functions directly.
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


class _Args(dict):
    def getlist(self, key):
        value = self.get(key, [])
        if isinstance(value, list):
            return value
        return [value]


def _run(coro):
    return asyncio.run(coro)


def _module_stub(name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    return mod


def _load_document_api(monkeypatch, *, accessible_kb_ids, storage_store, thumbnail_docs):
    repo_root = Path(__file__).resolve().parents[3]

    quart_mod = _module_stub(
        "quart",
        request=SimpleNamespace(args=_Args()),
        make_response=lambda data: _run_identity(data),
        send_file=None,
    )
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    apps_mod = _module_stub(
        "api.apps",
        AUTH_JWT="jwt",
        AUTH_API="api",
        AUTH_BETA="beta",
        current_user=SimpleNamespace(id="tenant_own"),
        login_required=lambda *_a, **_k: (lambda f: f),
    )
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)
    monkeypatch.setitem(sys.modules, "api.constants", _module_stub("api.constants", FILE_NAME_LEN_LIMIT=255, IMG_BASE64_PREFIX="data:image"))

    monkeypatch.setitem(
        sys.modules,
        "api.apps.services.document_api_service",
        _module_stub(
            "api.apps.services.document_api_service",
            validate_document_update_fields=None,
            map_doc_keys=None,
            map_doc_keys_with_run_status=None,
            update_document_name_only=None,
            update_chunk_method=None,
            update_document_status_only=None,
            reset_document_for_reparse=None,
        ),
    )

    monkeypatch.setitem(sys.modules, "api.db", _module_stub("api.db", VALID_FILE_TYPES=[], FileType=SimpleNamespace()))
    monkeypatch.setitem(
        sys.modules,
        "api.db.db_models",
        _module_stub("api.db.db_models", API4Conversation=SimpleNamespace(), DB=SimpleNamespace(connection_context=lambda: (lambda f: f)), Task=SimpleNamespace()),
    )
    monkeypatch.setitem(sys.modules, "api.db.services", _module_stub("api.db.services", duplicate_name=None))
    monkeypatch.setitem(sys.modules, "api.db.services.doc_metadata_service", _module_stub("api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.services.document_counter_service", _module_stub("api.db.services.document_counter_service", release_reparse_counters=None))

    document_service = SimpleNamespace(get_thumbnails=lambda doc_ids: [dict(d) for d in thumbnail_docs if d["id"] in set(doc_ids)])
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", _module_stub("api.db.services.document_service", DocumentService=document_service))
    monkeypatch.setitem(sys.modules, "api.db.services.file2document_service", _module_stub("api.db.services.file2document_service", File2DocumentService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", _module_stub("api.db.services.file_service", FileService=SimpleNamespace()))

    kb_service = SimpleNamespace(accessible=lambda kb_id, user_id: kb_id in accessible_kb_ids)
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", _module_stub("api.db.services.knowledgebase_service", KnowledgebaseService=kb_service))
    monkeypatch.setitem(sys.modules, "api.db.services.canvas_service", _module_stub("api.db.services.canvas_service", UserCanvasService=SimpleNamespace()))
    monkeypatch.setitem(sys.modules, "api.common.check_team_permission", _module_stub("api.common.check_team_permission", check_kb_team_permission=None))
    monkeypatch.setitem(sys.modules, "api.db.services.task_service", _module_stub("api.db.services.task_service", TaskService=SimpleNamespace(), cancel_all_task_of=None))

    def _envelope(data=None, message="", code=0):
        return {"code": code, "message": message, "data": data}

    api_utils_mod = _module_stub(
        "api.utils.api_utils",
        construct_json_result=lambda **kw: _envelope(kw.get("data"), kw.get("message", ""), kw.get("code", 0)),
        get_data_error_result=lambda **kw: _envelope(None, kw.get("message", ""), 102),
        get_error_data_result=lambda *a, **kw: _envelope(None, kw.get("message", a[0] if a else ""), 102),
        get_result=lambda **kw: _envelope(kw.get("data"), kw.get("message", ""), kw.get("code", 0)),
        get_json_result=lambda **kw: _envelope(kw.get("data"), kw.get("message", ""), kw.get("code", 0)),
        server_error_response=lambda e: _envelope(None, str(e), 100),
        add_tenant_id_to_kwargs=lambda f: f,
        get_request_json=None,
        get_error_argument_result=lambda *a, **kw: _envelope(None, a[0] if a else "", 101),
        check_duplicate_ids=None,
        strip_graphrag_raptor_config=None,
        timeout=lambda *_a, **_k: (lambda f: f),
        thread_pool_exec=lambda fn, *a: _async_wrap(fn, *a),
    )
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)
    monkeypatch.setitem(
        sys.modules,
        "api.utils.pagination_utils",
        _module_stub(
            "api.utils.pagination_utils",
            DEFAULT_PAGE=1,
            DEFAULT_PAGE_SIZE=30,
            validate_rest_api_ids=lambda ids, name: ids,
            validate_rest_api_page=lambda p: int(p),
            validate_rest_api_page_size=lambda s: int(s),
        ),
    )
    monkeypatch.setitem(
        sys.modules,
        "api.utils.validation_utils",
        _module_stub(
            "api.utils.validation_utils",
            UpdateDocumentReq=SimpleNamespace,
            format_validation_error_message=None,
            validate_and_parse_json_request=None,
            DeleteDocumentReq=SimpleNamespace,
        ),
    )

    common_mod = _module_stub("common", settings=SimpleNamespace(STORAGE_IMPL=SimpleNamespace(get=lambda b, n: storage_store.get((b, n)))))
    monkeypatch.setitem(sys.modules, "common", common_mod)
    monkeypatch.setitem(
        sys.modules,
        "common.constants",
        _module_stub("common.constants", ParserType=SimpleNamespace(), RetCode=SimpleNamespace(), TaskStatus=SimpleNamespace(), SANDBOX_ARTIFACT_BUCKET="sandbox"),
    )
    monkeypatch.setitem(sys.modules, "common.llm_request_context", _module_stub("common.llm_request_context", normalize_llm_user_id=lambda x: x))

    # thread_pool_exec used by the routes comes from common.misc_utils
    async def _tpe(fn, *args):
        return fn(*args)

    monkeypatch.setitem(sys.modules, "common.metadata_utils", _module_stub("common.metadata_utils", convert_conditions=None, meta_filter=None, turn2jsonschema=None))
    monkeypatch.setitem(sys.modules, "common.misc_utils", _module_stub("common.misc_utils", get_uuid=lambda: "uuid", thread_pool_exec=_tpe, thread_pool_exec_long_time=_tpe))
    monkeypatch.setitem(sys.modules, "api.utils.file_utils", _module_stub("api.utils.file_utils", filename_type=None, thumbnail=None))
    monkeypatch.setitem(sys.modules, "api.utils.file_response", _module_stub("api.utils.file_response", apply_preview_file_response_headers=None))
    monkeypatch.setitem(sys.modules, "api.utils.web_utils", _module_stub("api.utils.web_utils", CONTENT_TYPE_MAP={}, html2pdf=None, is_valid_url=None, apply_safe_file_response_headers=None))
    monkeypatch.setitem(sys.modules, "common.ssrf_guard", _module_stub("common.ssrf_guard", assert_url_is_safe=None))
    rag_mod = _module_stub("rag")
    rag_mod.__path__ = [str(repo_root / "rag")]
    monkeypatch.setitem(sys.modules, "rag", rag_mod)
    rag_nlp_mod = _module_stub("rag.nlp")
    rag_nlp_mod.__path__ = [str(repo_root / "rag" / "nlp")]
    rag_nlp_mod.search = SimpleNamespace(index_name=lambda t: f"idx_{t}")
    monkeypatch.setitem(sys.modules, "rag.nlp", rag_nlp_mod)

    module_name = "test_document_api_unit_module"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "document_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


class _null_ctx:
    def __enter__(self):
        return self

    def __exit__(self, *_a):
        return False


async def _async_wrap(fn, *args):
    return fn(*args)


def _run_identity(data):
    async def _resp():
        return SimpleNamespace(data=data, headers=SimpleNamespace(set=lambda *_a, **_k: None))

    return _resp()


@pytest.mark.p2
def test_document_image_denies_foreign_dataset(monkeypatch):
    storage = {("kb_foreign", "thumb-1.png"): b"\x89PNG\r\n\x1a\n-bytes"}
    module = _load_document_api(monkeypatch, accessible_kb_ids={"kb_own"}, storage_store=storage, thumbnail_docs=[])

    res = _run(module.get_document_image("kb_foreign-thumb-1.png"))
    assert res["code"] == 102
    assert res["message"] == "Image not found."


@pytest.mark.p2
def test_document_image_serves_own_dataset(monkeypatch):
    payload = b"\x89PNG\r\n\x1a\n-real"
    storage = {("kb_own", "thumb-1.png"): payload}
    module = _load_document_api(monkeypatch, accessible_kb_ids={"kb_own"}, storage_store=storage, thumbnail_docs=[])

    res = _run(module.get_document_image("kb_own-thumb-1.png"))
    # The route awaits the make_response stub, so a served image comes back as
    # the response object; an error envelope would be a dict instead.
    assert not isinstance(res, dict), f"expected an image response, got {res!r}"
    assert res.data == payload


@pytest.mark.p2
def test_thumbnails_filters_out_foreign_documents(monkeypatch):
    docs = [
        {"id": "doc_own", "kb_id": "kb_own", "thumbnail": "data:image/png;base64,AAA"},
        {"id": "doc_foreign", "kb_id": "kb_foreign", "thumbnail": "data:image/png;base64,BBB"},
    ]
    module = _load_document_api(monkeypatch, accessible_kb_ids={"kb_own"}, storage_store={}, thumbnail_docs=docs)
    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args({"doc_ids": ["doc_own", "doc_foreign"]})))

    res = module.list_thumbnails()
    assert res["code"] == 0
    assert set(res["data"].keys()) == {"doc_own"}


@pytest.mark.p2
def test_thumbnails_returns_all_for_own_documents(monkeypatch):
    docs = [
        {"id": "doc_a", "kb_id": "kb_own", "thumbnail": "data:image/png;base64,AAA"},
        {"id": "doc_b", "kb_id": "kb_own", "thumbnail": "data:image/png;base64,BBB"},
    ]
    module = _load_document_api(monkeypatch, accessible_kb_ids={"kb_own"}, storage_store={}, thumbnail_docs=docs)
    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args({"doc_ids": ["doc_a", "doc_b"]})))

    res = module.list_thumbnails()
    assert res["code"] == 0
    assert set(res["data"].keys()) == {"doc_a", "doc_b"}

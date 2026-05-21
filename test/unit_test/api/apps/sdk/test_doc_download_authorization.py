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
"""Regression tests for SDK document download authorization in api/apps/sdk/doc.py."""

import asyncio
import importlib.util
import logging
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest

_MODULE_NAME = "test_doc_download_module"
_REPO_ROOT = Path(__file__).resolve().parents[5]
_DOC_PATH = _REPO_ROOT / "api" / "apps" / "sdk" / "doc.py"


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


class _DummyDoc:
    def __init__(self, name="secret.pdf"):
        self.name = name
        self.kb_id = "kb-victim"


def _install_dependency_stubs(monkeypatch, *, accessible, storage_get):
    """Replace dependency modules unconditionally so import order is stable."""
    storage_calls = []

    def _storage_get(*_args, **_kwargs):
        storage_calls.append(True)
        return storage_get(*_args, **_kwargs)

    acc_fn = accessible if callable(accessible) else (lambda *_a, **_k: accessible)

    apps_mod = ModuleType("api.apps")
    apps_mod.current_user = SimpleNamespace(id="user-owner")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    common_settings_mod = ModuleType("common.settings")
    common_settings_mod.STORAGE_IMPL = SimpleNamespace(get=_storage_get)
    monkeypatch.setitem(sys.modules, "common.settings", common_settings_mod)

    common_mod = ModuleType("common")
    common_mod.settings = common_settings_mod
    monkeypatch.setitem(sys.modules, "common", common_mod)

    common_constants_mod = ModuleType("common.constants")
    common_constants_mod.RetCode = SimpleNamespace(DATA_ERROR=102)
    common_constants_mod.LLMType = SimpleNamespace()
    common_constants_mod.TaskStatus = SimpleNamespace()
    monkeypatch.setitem(sys.modules, "common.constants", common_constants_mod)

    common_metadata_mod = ModuleType("common.metadata_utils")
    common_metadata_mod.convert_conditions = lambda conditions: conditions
    common_metadata_mod.meta_filter = lambda *_args, **_kwargs: []
    monkeypatch.setitem(sys.modules, "common.metadata_utils", common_metadata_mod)

    db_models_mod = ModuleType("api.db.db_models")
    db_models_mod.Document = type("Document", (), {})
    db_models_mod.Task = type("Task", (), {})
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    tenant_model_mod = ModuleType("api.db.joint_services.tenant_model_service")
    tenant_model_mod.get_model_config_by_id = lambda *_a, **_k: {}
    tenant_model_mod.get_model_config_by_type_and_name = lambda *_a, **_k: {}
    tenant_model_mod.get_tenant_default_model_by_type = lambda *_a, **_k: {}
    monkeypatch.setitem(sys.modules, "api.db.joint_services.tenant_model_service", tenant_model_mod)

    document_service_mod = ModuleType("api.db.services.document_service")
    document_service_mod.DocumentService = SimpleNamespace(
        query=lambda **_kwargs: [_DummyDoc()],
        accessible=acc_fn,
    )
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", document_service_mod)

    file2document_mod = ModuleType("api.db.services.file2document_service")
    file2document_mod.File2DocumentService = SimpleNamespace(
        get_storage_address=lambda **_kwargs: ("bucket", "object-key"),
    )
    monkeypatch.setitem(sys.modules, "api.db.services.file2document_service", file2document_mod)

    kb_service_mod = ModuleType("api.db.services.knowledgebase_service")
    kb_service_mod.KnowledgebaseService = SimpleNamespace(accessible=lambda *_a, **_k: True)
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", kb_service_mod)

    for name, attrs in (
        ("api.db.services.doc_metadata_service", {"DocMetadataService": SimpleNamespace()}),
        ("api.db.services.llm_service", {"LLMBundle": SimpleNamespace()}),
        ("api.db.services.task_service", {"TaskService": SimpleNamespace(), "cancel_all_task_of": lambda *_a, **_k: None, "queue_tasks": lambda *_a, **_k: None}),
        ("api.db.services.tenant_llm_service", {"TenantLLMService": SimpleNamespace()}),
    ):
        mod = ModuleType(name)
        for key, value in attrs.items():
            setattr(mod, key, value)
        monkeypatch.setitem(sys.modules, name, mod)

    api_utils_mod = ModuleType("api.utils.api_utils")
    api_utils_mod.get_error_data_result = lambda message="", code=102: {"code": code, "message": message}
    api_utils_mod.construct_json_result = lambda message="", code=102: {"code": code, "message": message}
    api_utils_mod.check_duplicate_ids = lambda ids, _kind: (ids, [])
    api_utils_mod.get_request_json = lambda: {}
    api_utils_mod.get_result = lambda **_kwargs: {}
    api_utils_mod.server_error_response = lambda e: {"message": str(e)}
    api_utils_mod.token_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    ref_meta_mod = ModuleType("api.utils.reference_metadata_utils")
    ref_meta_mod.enrich_chunks_with_document_metadata = lambda *_a, **_k: None
    ref_meta_mod.resolve_reference_metadata_preferences = lambda req, _cfg=None: req
    monkeypatch.setitem(sys.modules, "api.utils.reference_metadata_utils", ref_meta_mod)

    rag_tag_mod = ModuleType("rag.app.tag")
    rag_tag_mod.label_question = lambda *_a, **_k: {}
    monkeypatch.setitem(sys.modules, "rag.app.tag", rag_tag_mod)

    rag_nlp_mod = ModuleType("rag.nlp")
    rag_nlp_mod.search = SimpleNamespace(index_name=lambda tenant_id: f"idx_{tenant_id}")
    monkeypatch.setitem(sys.modules, "rag.nlp", rag_nlp_mod)

    rag_prompts_mod = ModuleType("rag.prompts.generator")
    rag_prompts_mod.cross_languages = lambda *_a, **_k: ""
    rag_prompts_mod.keyword_extraction = lambda *_a, **_k: ""
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", rag_prompts_mod)

    quart_mod = ModuleType("quart")

    async def _fake_send_file(file_obj, **kwargs):
        return {"payload": file_obj.read(), "filename": kwargs.get("attachment_filename")}

    quart_mod.send_file = _fake_send_file
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    return storage_calls


def _load_doc_module(monkeypatch, *, accessible, storage_get=None):
    if storage_get is None:
        storage_get = lambda *_a, **_k: b"leaked-bytes"

    monkeypatch.delitem(sys.modules, _MODULE_NAME, raising=False)
    storage_calls = _install_dependency_stubs(monkeypatch, accessible=accessible, storage_get=storage_get)

    spec = importlib.util.spec_from_file_location(_MODULE_NAME, _DOC_PATH)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, _MODULE_NAME, module)
    spec.loader.exec_module(module)

    module._storage_calls = storage_calls
    module._owner_user = SimpleNamespace(id="user-owner")
    module._attacker_user = SimpleNamespace(id="user-attacker")
    return module


@pytest.mark.p1
class TestSdkDocumentDownloadAuthorization:
    def test_dataset_download_missing_doc_returns_generic_message(self, monkeypatch):
        module = _load_doc_module(monkeypatch, accessible=lambda *_a, **_k: True)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])

        result = asyncio.run(module.download("kb-victim", "doc-missing"))

        assert result["message"] == "Document not found!"
        assert module._storage_calls == []

    def test_dataset_download_cross_tenant_is_rejected(self, monkeypatch, caplog):
        module = _load_doc_module(
            monkeypatch,
            accessible=lambda _doc_id, user_id: user_id == "user-owner",
        )
        module.current_user = module._attacker_user
        caplog.set_level(logging.WARNING, logger=module.__name__)

        result = asyncio.run(module.download("kb-victim", "doc-victim"))

        assert result["message"] == "Document not found!"
        assert module._storage_calls == []
        denial_logs = [r for r in caplog.records if r.levelno == logging.WARNING and "cross-tenant" in r.getMessage()]
        assert denial_logs

    def test_dataset_download_same_tenant_succeeds(self, monkeypatch):
        storage_calls = []

        def _storage_get(*_args, **_kwargs):
            storage_calls.append(True)
            return b"ok"

        module = _load_doc_module(
            monkeypatch,
            accessible=lambda _doc_id, _user_id: True,
            storage_get=_storage_get,
        )
        module.current_user = module._owner_user

        result = asyncio.run(module.download("kb-owner", "doc-owner"))

        assert result["filename"] == "secret.pdf"
        assert result["payload"] == b"ok"
        assert storage_calls

    def test_document_download_missing_doc_returns_generic_message(self, monkeypatch):
        module = _load_doc_module(monkeypatch, accessible=lambda *_a, **_k: True)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])

        result = asyncio.run(module.download_document("doc-missing"))

        assert result["message"] == "Document not found!"
        assert module._storage_calls == []

    def test_document_download_cross_tenant_is_rejected(self, monkeypatch, caplog):
        module = _load_doc_module(
            monkeypatch,
            accessible=lambda _doc_id, user_id: user_id == "user-owner",
        )
        module.current_user = module._attacker_user
        caplog.set_level(logging.WARNING, logger=module.__name__)

        result = asyncio.run(module.download_document("doc-victim"))

        assert result["message"] == "Document not found!"
        assert module._storage_calls == []
        denial_logs = [r for r in caplog.records if r.levelno == logging.WARNING and "cross-tenant" in r.getMessage()]
        assert denial_logs

    def test_document_download_same_tenant_succeeds(self, monkeypatch):
        storage_calls = []

        def _storage_get(*_args, **_kwargs):
            storage_calls.append(True)
            return b"ok"

        module = _load_doc_module(
            monkeypatch,
            accessible=lambda _doc_id, _user_id: True,
            storage_get=_storage_get,
        )
        module.current_user = module._owner_user

        result = asyncio.run(module.download_document("doc-owner"))

        assert result["filename"] == "secret.pdf"
        assert result["payload"] == b"ok"
        assert storage_calls

    def test_missing_and_unauthorized_return_same_message(self, monkeypatch):
        module = _load_doc_module(
            monkeypatch,
            accessible=lambda _doc_id, user_id: user_id == "user-owner",
        )
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
        missing = asyncio.run(module.download_document("doc-missing"))

        module = _load_doc_module(
            monkeypatch,
            accessible=lambda _doc_id, user_id: user_id == "user-owner",
        )
        module.current_user = module._attacker_user
        unauthorized = asyncio.run(module.download_document("doc-victim"))

        assert missing["message"] == unauthorized["message"] == "Document not found!"

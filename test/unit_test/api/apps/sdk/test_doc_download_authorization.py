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
"""Regression tests for SDK document download authorization in api/apps/sdk/doc.py.

Cross-tenant file download via GET /api/v1/datasets/<dataset_id>/documents/<document_id>
and GET /api/v1/documents/<document_id> when DocumentService.accessible is not enforced.
"""

import asyncio
import importlib.util
import logging
import sys
from io import BytesIO
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
    if "." in name:
        parent_name, _, child_name = name.rpartition(".")
        parent_mod = sys.modules.get(parent_name)
        if parent_mod is not None:
            monkeypatch.setattr(parent_mod, child_name, mod, raising=False)
    return mod


class _DummyDoc:
    def __init__(self, name="secret.pdf"):
        self.name = name
        self.kb_id = "kb-victim"


def _load_doc_download_module(monkeypatch, *, accessible, storage_get=None):
    owner_user = SimpleNamespace(id="user-owner")
    attacker_user = SimpleNamespace(id="user-attacker")

    apps_mod = ModuleType("api.apps")
    apps_mod.current_user = owner_user
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    storage_calls = []

    def _storage_get(*_args, **_kwargs):
        storage_calls.append(True)
        if storage_get is None:
            return b"leaked-bytes"
        return storage_get(*_args, **_kwargs)

    _stub(
        monkeypatch,
        "common.settings",
        STORAGE_IMPL=SimpleNamespace(get=_storage_get),
    )
    _stub(monkeypatch, "common", settings=sys.modules["common.settings"])
    _stub(
        monkeypatch,
        "common.constants",
        RetCode=SimpleNamespace(DATA_ERROR=102),
        LLMType=SimpleNamespace(),
        TaskStatus=SimpleNamespace(),
    )

    acc_fn = accessible if callable(accessible) else (lambda *_a, **_k: accessible)
    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(
            query=lambda **_kwargs: [_DummyDoc()],
            accessible=acc_fn,
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.file2document_service",
        File2DocumentService=SimpleNamespace(
            get_storage_address=lambda **_kwargs: ("bucket", "object-key"),
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(accessible=lambda *_a, **_k: True),
    )
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        get_error_data_result=lambda message="", code=102: {"code": code, "message": message},
        construct_json_result=lambda message="", code=102: {"code": code, "message": message},
        check_duplicate_ids=lambda ids, _kind: (ids, []),
        get_request_json=lambda: {},
        get_result=lambda **_kwargs: {},
        server_error_response=lambda e: {"message": str(e)},
        token_required=lambda func: func,
    )

    quart_stub = ModuleType("quart")
    sent = {}

    async def _fake_send_file(file_obj, **kwargs):
        sent["payload"] = file_obj.read()
        sent["filename"] = kwargs.get("attachment_filename")
        return sent

    quart_stub.send_file = _fake_send_file
    monkeypatch.setitem(sys.modules, "quart", quart_stub)

    for stub_name in (
        "api.db.db_models",
        "api.db.joint_services.tenant_model_service",
        "api.db.services.doc_metadata_service",
        "api.db.services.llm_service",
        "api.db.services.task_service",
        "api.db.services.tenant_llm_service",
        "api.utils.reference_metadata_utils",
        "common.metadata_utils",
        "rag.app.tag",
        "rag.nlp",
        "rag.prompts.generator",
    ):
        if stub_name not in sys.modules:
            monkeypatch.setitem(sys.modules, stub_name, ModuleType(stub_name))

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "sdk" / "doc.py"
    spec = importlib.util.spec_from_file_location("test_doc_download_module", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_doc_download_module", module)
    spec.loader.exec_module(module)
    module._storage_calls = storage_calls
    module._owner_user = owner_user
    module._attacker_user = attacker_user
    return module


@pytest.mark.p1
class TestSdkDocumentDownloadAuthorization:
    def test_dataset_download_cross_tenant_is_rejected(self, monkeypatch, caplog):
        module = _load_doc_download_module(
            monkeypatch,
            accessible=lambda doc_id, user_id: user_id == "user-owner",
        )
        import api.apps as apps_mod

        apps_mod.current_user = module._attacker_user
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

        module = _load_doc_download_module(
            monkeypatch,
            accessible=lambda _doc_id, _user_id: True,
            storage_get=_storage_get,
        )
        monkeypatch.setattr(
            module.File2DocumentService,
            "get_storage_address",
            lambda **_kwargs: ("bucket", "object-key"),
        )

        result = asyncio.run(module.download("kb-owner", "doc-owner"))

        assert result["filename"] == "secret.pdf"
        assert result["payload"] == b"ok"
        assert storage_calls

    def test_document_download_cross_tenant_is_rejected(self, monkeypatch, caplog):
        module = _load_doc_download_module(
            monkeypatch,
            accessible=lambda doc_id, user_id: user_id == "user-owner",
        )
        import api.apps as apps_mod

        apps_mod.current_user = module._attacker_user
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

        module = _load_doc_download_module(
            monkeypatch,
            accessible=lambda _doc_id, _user_id: True,
            storage_get=_storage_get,
        )
        monkeypatch.setattr(
            module.File2DocumentService,
            "get_storage_address",
            lambda **_kwargs: ("bucket", "object-key"),
        )

        result = asyncio.run(module.download_document("doc-owner"))

        assert result["filename"] == "secret.pdf"
        assert result["payload"] == b"ok"
        assert storage_calls

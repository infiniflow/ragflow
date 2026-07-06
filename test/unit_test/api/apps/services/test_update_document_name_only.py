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
"""Regression tests for update_document_name_only() in document_api_service."""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock

import pytest

pytestmark = pytest.mark.p2


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


def _load_update_document_name_only_module(monkeypatch, *, file_lookup):
    file_update = MagicMock()
    doc_store_update = MagicMock()
    doc = SimpleNamespace(id="doc-1", kb_id="kb-1")
    f2d_row = SimpleNamespace(file_id="missing-file-id")

    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(
            update_by_id=lambda doc_id, data: True,
            get_tenant_id=lambda doc_id: "tenant-1",
            get_by_id=lambda doc_id: (True, doc),
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.file2document_service",
        File2DocumentService=SimpleNamespace(
            get_by_document_id=lambda doc_id: [f2d_row],
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.file_service",
        FileService=SimpleNamespace(
            get_by_id=file_lookup,
            update_by_id=file_update,
        ),
    )
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        get_error_data_result=lambda **kwargs: kwargs,
        server_error_response=lambda e: {"error": str(e)},
        get_parser_config=lambda doc: {},
    )
    _stub(monkeypatch, "api.utils.validation_utils", UpdateDocumentReq=object)
    _stub(monkeypatch, "api.utils", validation_utils=sys.modules["api.utils.validation_utils"])
    _stub(monkeypatch, "common.constants", TaskStatus=SimpleNamespace(RUNNING=SimpleNamespace(value="1")))
    _stub(
        monkeypatch,
        "common.settings",
        docStoreConn=SimpleNamespace(
            index_exist=lambda idx, kb_id: True,
            update=doc_store_update,
        ),
    )
    _stub(
        monkeypatch,
        "rag.nlp.search",
        index_name=lambda tenant_id: f"idx-{tenant_id}",
    )
    _stub(
        monkeypatch,
        "rag.nlp.rag_tokenizer",
        tokenize=lambda text: [text],
        fine_grained_tokenize=lambda tokens: tokens,
    )

    module_path = Path(__file__).resolve().parents[5] / "api" / "apps" / "services" / "document_api_service.py"
    spec = importlib.util.spec_from_file_location("test_update_document_name_only_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_update_document_name_only_module", module)
    spec.loader.exec_module(module)
    return module, file_update, doc_store_update


@pytest.mark.p2
def test_update_document_name_only_skips_missing_linked_file(monkeypatch):
    """Orphan File2Document rows must not crash rename with AttributeError."""
    module, file_update, doc_store_update = _load_update_document_name_only_module(
        monkeypatch,
        file_lookup=lambda file_id: (False, None),
    )

    result = module.update_document_name_only("doc-1", "renamed.pdf")

    assert result is None
    file_update.assert_not_called()
    doc_store_update.assert_called_once()


@pytest.mark.p2
def test_update_document_name_only_updates_linked_file_when_present(monkeypatch):
    linked_file = SimpleNamespace(id="file-1")
    module, file_update, doc_store_update = _load_update_document_name_only_module(
        monkeypatch,
        file_lookup=lambda file_id: (True, linked_file),
    )

    result = module.update_document_name_only("doc-1", "renamed.pdf")

    assert result is None
    file_update.assert_called_once_with("file-1", {"name": "renamed.pdf"})
    doc_store_update.assert_called_once()

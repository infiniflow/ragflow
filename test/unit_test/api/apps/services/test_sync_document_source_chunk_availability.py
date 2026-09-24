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
"""Parent-child parents must stay unavailable when a document is re-enabled."""

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


def _load_module(monkeypatch, *, child_total: int):
    doc_store = SimpleNamespace(
        search=MagicMock(return_value="search-res"),
        get_total=MagicMock(return_value=child_total),
        update=MagicMock(return_value=True),
    )
    _stub(monkeypatch, "api.db.services.document_counter_service", release_reparse_counters=lambda doc_id: None)
    _stub(monkeypatch, "api.db.services.document_service", DocumentService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.file2document_service", File2DocumentService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.file_service", FileService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        get_error_data_result=lambda **kwargs: kwargs,
        server_error_response=lambda e: {"error": str(e)},
        get_parser_config=lambda doc: {},
        strip_graphrag_raptor_config=lambda data: data,
    )
    _stub(monkeypatch, "api.utils", validation_utils=SimpleNamespace())
    _stub(monkeypatch, "api.utils.validation_utils", UpdateDocumentReq=object)
    _stub(monkeypatch, "common", settings=SimpleNamespace(docStoreConn=doc_store))
    _stub(monkeypatch, "common.settings", docStoreConn=doc_store)
    _stub(monkeypatch, "common.constants", TaskStatus=SimpleNamespace())
    _stub(monkeypatch, "rag.nlp", rag_tokenizer=SimpleNamespace(), search=SimpleNamespace(index_name=lambda tid: f"ragflow_{tid}"))
    _stub(monkeypatch, "rag.nlp.search", index_name=lambda tid: f"ragflow_{tid}")
    _stub(monkeypatch, "common.doc_store.doc_store_base", OrderByExpr=object)

    path = Path(__file__).resolve().parents[5] / "api" / "apps" / "services" / "document_api_service.py"
    spec = importlib.util.spec_from_file_location("document_api_service_under_test", path)
    mod = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(mod)
    return mod, doc_store


def test_enable_parent_child_only_updates_children(monkeypatch):
    mod, doc_store = _load_module(monkeypatch, child_total=2)

    assert mod.sync_document_source_chunk_availability("doc-1", "tenant-1", "kb-1", 1) is True

    condition = doc_store.update.call_args.args[0]
    assert condition["doc_id"] == "doc-1"
    assert condition["exists"] == "mom_id"
    assert condition["must_not"] == {"exists": "compile_kwd"}
    assert doc_store.update.call_args.args[1] == {"available_int": 1}


def test_enable_flat_document_updates_all_source_chunks(monkeypatch):
    mod, doc_store = _load_module(monkeypatch, child_total=0)

    assert mod.sync_document_source_chunk_availability("doc-1", "tenant-1", "kb-1", 1) is True

    condition = doc_store.update.call_args.args[0]
    assert condition == {"doc_id": "doc-1", "must_not": {"exists": "compile_kwd"}}
    assert "exists" not in condition


def test_disable_always_updates_all_source_chunks(monkeypatch):
    mod, doc_store = _load_module(monkeypatch, child_total=2)

    assert mod.sync_document_source_chunk_availability("doc-1", "tenant-1", "kb-1", 0) is True

    condition = doc_store.update.call_args.args[0]
    assert condition == {"doc_id": "doc-1", "must_not": {"exists": "compile_kwd"}}
    doc_store.search.assert_not_called()

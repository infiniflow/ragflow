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
"""Regression tests for delete_datasets() in dataset_api_service."""

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


def _load_delete_datasets_module(monkeypatch, *, f2d_rows, file_filter_delete):
    f2d_delete = MagicMock()
    kb = SimpleNamespace(id="kb-1", tenant_id="tenant-1", name="test-kb")
    doc = SimpleNamespace(id="doc-1")

    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(
            query=lambda kb_id: [doc],
            remove_document=lambda doc, tenant_id: True,
        ),
        queue_raptor_o_graphrag_tasks=MagicMock(),
    )
    _stub(
        monkeypatch,
        "api.db.services.file2document_service",
        File2DocumentService=SimpleNamespace(
            get_by_document_id=lambda doc_id: f2d_rows,
            delete_by_document_id=f2d_delete,
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.file_service",
        FileService=SimpleNamespace(filter_delete=file_filter_delete),
    )
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(
            get_or_none=lambda id, tenant_id: kb,
            delete_by_id=lambda kb_id: True,
            query=lambda **kwargs: [],
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.connector_service",
        Connector2KbService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.services.task_service",
        TaskService=SimpleNamespace(),
        GRAPH_RAPTOR_FAKE_DOC_ID="fake-doc",
    )
    _stub(
        monkeypatch,
        "api.db.services.user_service",
        TenantService=SimpleNamespace(),
        UserService=SimpleNamespace(),
        UserTenantService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.services.tenant_llm_service",
        TenantLLMService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_model_config_from_provider_instance=MagicMock(),
    )
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        deep_merge=MagicMock(),
        get_parser_config=MagicMock(),
        remap_dictionary_keys=MagicMock(),
        verify_embedding_availability=MagicMock(),
    )
    _stub(
        monkeypatch,
        "common.settings",
        docStoreConn=SimpleNamespace(delete_idx=lambda *_args, **_kwargs: None),
    )
    _stub(
        monkeypatch,
        "api.db.db_models",
        File=SimpleNamespace(source_type="source_type", id="id", type="type", name="name"),
    )
    _stub(
        monkeypatch,
        "common.constants",
        PAGERANK_FLD="pagerank",
        TAG_FLD="tag",
        FileSource=SimpleNamespace(KNOWLEDGEBASE="knowledgebase"),
        StatusEnum=SimpleNamespace(),
        LLMType=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "rag.nlp.search",
        index_name=lambda tenant_id: f"idx-{tenant_id}",
    )

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "services" / "dataset_api_service.py"
    spec = importlib.util.spec_from_file_location("test_delete_datasets_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_delete_datasets_module", module)
    spec.loader.exec_module(module)
    return module, f2d_delete


@pytest.mark.asyncio
async def test_delete_datasets_skips_file_delete_when_no_file2document(monkeypatch):
    """Documents without a File2Document row must not crash dataset deletion."""
    file_filter_delete = MagicMock(return_value=0)
    module, f2d_delete = _load_delete_datasets_module(
        monkeypatch,
        f2d_rows=[],
        file_filter_delete=file_filter_delete,
    )

    ok, result = await module.delete_datasets("tenant-1", ids=["kb-1"])

    assert ok is True
    assert result == {"success_count": 1}
    file_filter_delete.assert_called_once()
    f2d_delete.assert_called_once_with("doc-1")


@pytest.mark.asyncio
async def test_delete_datasets_deletes_linked_file_when_file2document_exists(monkeypatch):
    f2d_row = SimpleNamespace(file_id="file-1")
    file_filter_delete = MagicMock(side_effect=[1, 0])
    module, _f2d_delete = _load_delete_datasets_module(
        monkeypatch,
        f2d_rows=[f2d_row],
        file_filter_delete=file_filter_delete,
    )

    ok, result = await module.delete_datasets("tenant-1", ids=["kb-1"])

    assert ok is True
    assert result == {"success_count": 1}
    assert file_filter_delete.call_count == 2

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
from enum import IntEnum
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock

import pytest

pytestmark = pytest.mark.p2


class _StubModelTypeBinary(IntEnum):
    CHAT = 1
    EMBEDDING = 2
    ASR = 4
    VISION = 8
    RERANK = 16
    TTS = 32
    OCR = 64


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


def _load_delete_datasets_module(monkeypatch, *, f2d_rows, file_filter_delete, stranded_doc_rows=0):
    f2d_delete = MagicMock()
    calls = []
    cleanup = SimpleNamespace(
        calls=calls,
        doc_filter_delete=MagicMock(side_effect=lambda *_a, **_k: (calls.append("sweep_documents"), stranded_doc_rows)[1]),
        connector2kb_filter_delete=MagicMock(side_effect=lambda *_a, **_k: calls.append("unlink_connectors")),
        sync_logs_filter_delete=MagicMock(side_effect=lambda *_a, **_k: calls.append("delete_sync_logs")),
        sync_logs_filter_update=MagicMock(side_effect=lambda *_a, **_k: calls.append("cancel_running_syncs")),
    )
    kb = SimpleNamespace(id="kb-1", tenant_id="tenant-1", name="test-kb")
    doc = SimpleNamespace(id="doc-1")

    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(
            query=lambda kb_id: [doc],
            remove_document=lambda doc, tenant_id: True,
            filter_delete=cleanup.doc_filter_delete,
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
            get_or_none=lambda **_kwargs: kb,
            delete_by_id=lambda kb_id: calls.append("delete_dataset") is None,
            query=lambda **kwargs: [],
        ),
        validate_dataset_embedding_models=lambda kbs: None,
    )
    _stub(
        monkeypatch,
        "api.db.services.connector_service",
        Connector2KbService=SimpleNamespace(filter_delete=cleanup.connector2kb_filter_delete),
        SyncLogsService=SimpleNamespace(
            filter_delete=cleanup.sync_logs_filter_delete,
            filter_update=cleanup.sync_logs_filter_update,
        ),
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
        get_composite_model_name_by_ids=MagicMock(),
        get_model_config_from_provider_instance=MagicMock(),
        resolve_model_config=MagicMock(),
        resolve_model_id=MagicMock(),
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
        DB=SimpleNamespace(connection_context=lambda: lambda func: func),
        TenantModel=SimpleNamespace(),
        Connector2Kb=SimpleNamespace(kb_id="kb_id"),
        Document=SimpleNamespace(kb_id="kb_id"),
        File=SimpleNamespace(source_type="source_type", id="id", type="type", name="name"),
        SyncLogs=SimpleNamespace(kb_id="kb_id", status=SimpleNamespace(in_=lambda _values: None)),
    )
    _stub(
        monkeypatch,
        "common.constants",
        PAGERANK_FLD="pagerank",
        TAG_FLD="tag",
        FileSource=SimpleNamespace(KNOWLEDGEBASE="knowledgebase"),
        PipelineTaskType=SimpleNamespace(
            PARSE="parse",
            DOWNLOAD="download",
            RAPTOR="raptor",
            GRAPH_RAG="graph_rag",
            MINDMAP="mindmap",
            ARTIFACT="artifact",
            SKILL="skill",
        ),
        StatusEnum=SimpleNamespace(),
        LLMType=SimpleNamespace(),
        RetCode=SimpleNamespace(),
        TaskStatus=SimpleNamespace(SCHEDULE="schedule", RUNNING="running", CANCEL="cancel"),
        ModelTypeBinary=_StubModelTypeBinary,
    )
    _stub(monkeypatch, "rag.advanced_rag", __path__=[])
    _stub(monkeypatch, "rag.advanced_rag.knowlege_compile", __path__=[])
    _stub(
        monkeypatch,
        "rag.advanced_rag.knowlege_compile.wiki",
        WIKI_PAGE_COMPILE_KWD="wiki",
        _chunk_hash=lambda content: "stub-hash",
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
    return module, f2d_delete, cleanup


@pytest.mark.asyncio
async def test_delete_datasets_skips_file_delete_when_no_file2document(monkeypatch):
    """Documents without a File2Document row must not crash dataset deletion."""
    file_filter_delete = MagicMock(return_value=0)
    module, f2d_delete, _cleanup = _load_delete_datasets_module(
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
    module, _f2d_delete, _cleanup = _load_delete_datasets_module(
        monkeypatch,
        f2d_rows=[f2d_row],
        file_filter_delete=file_filter_delete,
    )

    ok, result = await module.delete_datasets("tenant-1", ids=["kb-1"])

    assert ok is True
    assert result == {"success_count": 1}
    assert file_filter_delete.call_count == 2


@pytest.mark.asyncio
async def test_delete_datasets_unwires_connectors_and_sweeps_stranded_documents(monkeypatch):
    """Deleting a dataset must not leave connector state pointing at it.

    Regression test for #18116. Surviving Connector2Kb/SyncLogs rows keep the
    connector scheduler queueing syncs against a kb_id that no longer resolves,
    and a surviving document row is invisible to the user while still able to
    trip the cross-KB id collision guard.
    """
    module, _f2d_delete, cleanup = _load_delete_datasets_module(
        monkeypatch,
        f2d_rows=[],
        file_filter_delete=MagicMock(return_value=0),
        stranded_doc_rows=2,
    )

    ok, result = await module.delete_datasets("tenant-1", ids=["kb-1"])

    assert ok is True
    assert result == {"success_count": 1}
    cleanup.connector2kb_filter_delete.assert_called_once()
    cleanup.sync_logs_filter_delete.assert_called_once()
    cleanup.doc_filter_delete.assert_called_once()
    cleanup.sync_logs_filter_update.assert_called_once()

    # Queued syncs are cancelled while the connector mapping still exists, the
    # mapping is dropped only once the dataset is really gone, and the sweep for
    # rows an in-flight sync may have written runs last.
    assert cleanup.calls == [
        "cancel_running_syncs",
        "delete_dataset",
        "unlink_connectors",
        "delete_sync_logs",
        "sweep_documents",
    ]

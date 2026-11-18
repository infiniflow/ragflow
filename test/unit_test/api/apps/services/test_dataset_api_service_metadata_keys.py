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
"""Regression tests for the GET /datasets/metadata/keys failure contract (#18916).

``DocMetadataService.get_metadata_keys_by_kbs`` used to swallow retrieval
exceptions and return an empty list, so the endpoint answered HTTP success with
``data: []`` even when the metadata store was unavailable. These tests lock in
that a retrieval failure is reported as a failure, while a dataset that simply
has no metadata keys still answers successfully with an empty list.
"""

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


def _identity_remap(source_data, key_aliases=None):
    return dict(source_data)


def _load_dataset_api_service_module(monkeypatch, *, metadata_keys_result):
    """Load ``dataset_api_service`` with stubbed heavy dependencies.

    ``metadata_keys_result`` is either a list (success) or an exception instance
    that ``DocMetadataService.get_metadata_keys_by_kbs`` raises.
    """
    accessible_mock = MagicMock(return_value=True)

    _stub(monkeypatch, "api.apps", __path__=[])
    _stub(monkeypatch, "api.apps.services", __path__=[])
    _stub(monkeypatch, "api.apps.services.structure_graph_common")
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_composite_model_name_by_ids=MagicMock(),
        resolve_model_config=MagicMock(),
        resolve_model_id=MagicMock(),
    )
    _stub(
        monkeypatch,
        "common.constants",
        PAGERANK_FLD="pagerank",
        LLMType=SimpleNamespace(),
        FileSource=SimpleNamespace(KNOWLEDGEBASE="knowledgebase"),
        PipelineTaskType=SimpleNamespace(),
        StatusEnum=SimpleNamespace(),
        TaskStatus=SimpleNamespace(SCHEDULE="schedule", RUNNING="running", CANCEL="cancel"),
        RetCode=SimpleNamespace(),
        ModelTypeBinary=_StubModelTypeBinary,
    )
    _stub(
        monkeypatch,
        "common.settings",
        docStoreConn=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.db_models",
        Connector2Kb=SimpleNamespace(kb_id="kb_id"),
        Document=SimpleNamespace(kb_id="kb_id"),
        File=SimpleNamespace(),
        SyncLogs=SimpleNamespace(kb_id="kb_id"),
    )
    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(),
        queue_raptor_o_graphrag_tasks=MagicMock(),
    )
    _stub(
        monkeypatch,
        "api.db.services.file2document_service",
        File2DocumentService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.services.file_service",
        FileService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(accessible=accessible_mock),
        validate_dataset_embedding_models=lambda kbs: None,
    )
    _stub(
        monkeypatch,
        "api.db.services.connector_service",
        Connector2KbService=SimpleNamespace(),
        SyncLogsService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.services.task_service",
        GRAPH_RAPTOR_FAKE_DOC_ID="fake-doc",
        TaskService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.services.tenant_model_service",
        TenantModelService=SimpleNamespace(),
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
        "api.utils.api_utils",
        deep_merge=MagicMock(),
        get_parser_config=MagicMock(),
        remap_dictionary_keys=_identity_remap,
        verify_embedding_availability=MagicMock(),
    )
    _stub(
        monkeypatch,
        "common.misc_utils",
        thread_pool_exec=MagicMock(),
        thread_pool_exec_long_time=MagicMock(),
    )
    _stub(monkeypatch, "rag.advanced_rag", __path__=[])
    _stub(monkeypatch, "rag.advanced_rag.knowlege_compile", __path__=[])
    _stub(
        monkeypatch,
        "rag.advanced_rag.knowlege_compile.wiki",
        WIKI_PAGE_COMPILE_KWD="wiki",
    )

    def _get_metadata_keys_by_kbs(_kb_ids):
        if isinstance(metadata_keys_result, Exception):
            raise metadata_keys_result
        return metadata_keys_result

    _stub(
        monkeypatch,
        "api.db.services.doc_metadata_service",
        DocMetadataService=SimpleNamespace(get_metadata_keys_by_kbs=_get_metadata_keys_by_kbs),
    )

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "services" / "dataset_api_service.py"
    spec = importlib.util.spec_from_file_location("test_dataset_api_service_metadata_keys_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_dataset_api_service_metadata_keys_module", module)
    spec.loader.exec_module(module)
    return module


def test_get_metadata_keys_returns_keys_on_success(monkeypatch):
    module = _load_dataset_api_service_module(monkeypatch, metadata_keys_result=["author", "title"])

    success, result = module.get_metadata_keys(["kb-1"], "tenant-1")

    assert success is True
    assert result == ["author", "title"]


def test_get_metadata_keys_empty_list_is_a_success(monkeypatch):
    """A dataset without metadata keys must still answer successfully."""
    module = _load_dataset_api_service_module(monkeypatch, metadata_keys_result=[])

    success, result = module.get_metadata_keys(["kb-1"], "tenant-1")

    assert success is True
    assert result == []


def test_get_metadata_keys_reports_failure_when_retrieval_raises(monkeypatch):
    """A metadata backend failure must not be reported as an empty key set."""
    module = _load_dataset_api_service_module(
        monkeypatch,
        metadata_keys_result=RuntimeError("metadata store unavailable"),
    )

    success, result = module.get_metadata_keys(["kb-1"], "tenant-1")

    assert success is False
    assert result
    assert "metadata keys" in result.lower()


def test_get_metadata_keys_reports_failure_without_calling_metadata_store_on_unauthorized_dataset(monkeypatch):
    module = _load_dataset_api_service_module(monkeypatch, metadata_keys_result=["author"])
    module.KnowledgebaseService.accessible = MagicMock(return_value=False)

    success, result = module.get_metadata_keys(["kb-1"], "tenant-1")

    assert success is False
    assert "authorization" in result.lower()

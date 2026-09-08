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
"""Regression tests for language handling in create_dataset() (#15703).

``CreateDatasetReq`` is parsed with ``exclude_unset=False``, so an omitted
``language`` reaches the service as an explicit ``None``. Forwarding that key to
``KnowledgebaseService.create_with_name`` writes NULL into the column and
bypasses the ``Knowledgebase.language`` default ("English"/"Chinese"), so the
service must drop it. An explicitly supplied language must still be forwarded.
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


def _load_create_dataset_module(monkeypatch):
    """Load dataset_api_service with the create path stubbed out.

    Returns the module plus the ``create_with_name`` mock, whose call kwargs are
    what these tests assert on.
    """
    created_kb = SimpleNamespace(to_dict=lambda: {"id": "kb-1", "name": "kb"})

    def _create_with_name(*, name, tenant_id, parser_id=None, **kwargs):
        return True, {"id": "kb-1", "name": name, "tenant_id": tenant_id, "parser_id": parser_id, **kwargs}

    create_with_name = MagicMock(side_effect=_create_with_name)

    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(
            create_with_name=create_with_name,
            save=MagicMock(return_value=True),
            get_by_id=MagicMock(return_value=(True, created_kb)),
        ),
        validate_dataset_embedding_models=lambda kbs: None,
    )
    _stub(
        monkeypatch,
        "api.db.services.user_service",
        TenantService=SimpleNamespace(get_by_id=MagicMock(return_value=(True, SimpleNamespace(embd_id="embd-model")))),
        UserService=SimpleNamespace(),
        UserTenantService=SimpleNamespace(),
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
        "api.db.services.connector_service",
        Connector2KbService=SimpleNamespace(),
        SyncLogsService=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.services.task_service",
        TaskService=SimpleNamespace(),
        GRAPH_RAPTOR_FAKE_DOC_ID="fake-doc",
    )
    _stub(
        monkeypatch,
        "api.db.services.tenant_model_service",
        TenantModelService=SimpleNamespace(),
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
        remap_dictionary_keys=lambda source_data, key_aliases=None: dict(source_data),
        verify_embedding_availability=MagicMock(return_value=(True, None)),
    )
    _stub(
        monkeypatch,
        "common.settings",
        docStoreConn=SimpleNamespace(),
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
    spec = importlib.util.spec_from_file_location("test_create_dataset_language_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_create_dataset_language_module", module)
    spec.loader.exec_module(module)
    return module, create_with_name


@pytest.mark.asyncio
async def test_create_dataset_drops_omitted_language(monkeypatch):
    """An omitted language must not reach the model layer as an explicit None."""
    module, create_with_name = _load_create_dataset_module(monkeypatch)

    # Shape of CreateDatasetReq(name="kb").model_dump(by_alias=True): the key is
    # present with a None value because the request is parsed with exclude_unset=False.
    ok, _result = await module.create_dataset("tenant-1", {"name": "kb", "language": None})

    assert ok is True
    assert "language" not in create_with_name.call_args.kwargs


@pytest.mark.asyncio
async def test_create_dataset_forwards_explicit_language(monkeypatch):
    """An explicit language is forwarded verbatim to the model layer."""
    module, create_with_name = _load_create_dataset_module(monkeypatch)

    ok, _result = await module.create_dataset("tenant-1", {"name": "kb", "language": "Chinese"})

    assert ok is True
    assert create_with_name.call_args.kwargs["language"] == "Chinese"

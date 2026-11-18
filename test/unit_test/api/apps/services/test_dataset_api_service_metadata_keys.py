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
"""Regression tests for metadata-key retrieval failure propagation (#18916).

``GET /datasets/metadata/keys`` must not answer with a successful empty list
when the metadata store is unavailable: "this dataset has no metadata keys" and
"the metadata store could not be reached" have to stay distinguishable for
clients.
"""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock

import pytest


pytestmark = pytest.mark.p2


def _stub(monkeypatch, name, **attrs):
    """Register a lightweight stand-in module under ``name``.

    Registering the fully qualified name in ``sys.modules`` keeps the import
    machinery from pulling in the real (and dependency heavy) package.
    """
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


def _repo_root() -> Path:
    return Path(__file__).resolve().parents[5]


def _load_module(monkeypatch, module_name: str, relative_path):
    """Execute a repository module from its file path under a private name."""
    module_path = _repo_root() / relative_path
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _load_doc_metadata_service(monkeypatch):
    """Load the real ``DocMetadataService`` with its heavy imports stubbed."""
    _stub(monkeypatch, "api.db.db_models", DB=MagicMock(), Document=MagicMock(), Knowledgebase=MagicMock())
    _stub(monkeypatch, "common.settings", docStoreConn=MagicMock())
    _stub(monkeypatch, "common.doc_store.doc_store_base", OrderByExpr=MagicMock)
    _stub(monkeypatch, "common.metadata_utils", dedupe_list=lambda values: list(values))

    return _load_module(
        monkeypatch,
        "test_doc_metadata_service_under_test",
        Path("api") / "db" / "services" / "doc_metadata_service.py",
    )


def _load_dataset_api_service(monkeypatch, *, accessible=True, retrieval=None):
    """Load ``dataset_api_service`` with its heavy imports stubbed.

    ``retrieval`` is the stand-in for
    ``DocMetadataService.get_metadata_keys_by_kbs``; the real module is imported
    lazily inside ``dataset_api_service.get_metadata_keys``.
    """
    _stub(monkeypatch, "common.constants", PAGERANK_FLD="pagerank", LLMType=SimpleNamespace(), FileSource=SimpleNamespace(), RetCode=SimpleNamespace(), StatusEnum=SimpleNamespace(), TaskStatus=SimpleNamespace())
    _stub(monkeypatch, "common.settings", docStoreConn=MagicMock())
    _stub(monkeypatch, "common.doc_store.doc_store_base", OrderByExpr=MagicMock)
    _stub(monkeypatch, "common.misc_utils", thread_pool_exec=MagicMock(), thread_pool_exec_long_time=MagicMock())
    _stub(monkeypatch, "api.db.db_models", Connector2Kb=SimpleNamespace(), Document=SimpleNamespace(), File=SimpleNamespace(), SyncLogs=SimpleNamespace())
    _stub(monkeypatch, "api.db.joint_services.tenant_model_service", get_composite_model_name_by_ids=MagicMock(), resolve_model_config=MagicMock(), resolve_model_id=MagicMock())
    _stub(monkeypatch, "api.db.services.connector_service", Connector2KbService=SimpleNamespace(), SyncLogsService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.document_service", DocumentService=SimpleNamespace(), queue_raptor_o_graphrag_tasks=MagicMock())
    _stub(monkeypatch, "api.db.services.file2document_service", File2DocumentService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.file_service", FileService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(accessible=MagicMock(return_value=accessible)),
        validate_dataset_embedding_models=lambda kbs: None,
    )
    _stub(monkeypatch, "api.db.services.task_service", GRAPH_RAPTOR_FAKE_DOC_ID="fake-doc", TaskService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.tenant_model_service", TenantModelService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.user_service", TenantService=SimpleNamespace(), UserService=SimpleNamespace(), UserTenantService=SimpleNamespace())
    _stub(monkeypatch, "api.utils.api_utils", deep_merge=MagicMock(), get_parser_config=MagicMock(), remap_dictionary_keys=lambda source_data, key_aliases=None: dict(source_data), verify_embedding_availability=MagicMock())
    _stub(monkeypatch, "rag.advanced_rag.knowlege_compile.wiki", WIKI_PAGE_COMPILE_KWD="wiki")

    if retrieval is None:
        retrieval = MagicMock(return_value=[])
    _stub(
        monkeypatch,
        "api.db.services.doc_metadata_service",
        DocMetadataService=SimpleNamespace(get_metadata_keys_by_kbs=retrieval),
    )

    return _load_module(
        monkeypatch,
        "test_dataset_api_service_under_test",
        Path("api") / "apps" / "services" / "dataset_api_service.py",
    )


class TestGetMetadataKeysByKbs:
    """``DocMetadataService.get_metadata_keys_by_kbs`` must not swallow lookup errors."""

    def test_lookup_failure_is_propagated(self, monkeypatch):
        module = _load_doc_metadata_service(monkeypatch)
        monkeypatch.setattr(
            module.DocMetadataService,
            "_search_metadata",
            MagicMock(side_effect=RuntimeError("metadata store unavailable")),
        )

        with pytest.raises(RuntimeError, match="metadata store unavailable"):
            module.DocMetadataService.get_metadata_keys_by_kbs(["kb-1"])

    def test_no_metadata_keys_stays_an_empty_list(self, monkeypatch):
        module = _load_doc_metadata_service(monkeypatch)
        monkeypatch.setattr(module.DocMetadataService, "_search_metadata", MagicMock(return_value=[]))

        assert module.DocMetadataService.get_metadata_keys_by_kbs(["kb-1"]) == []

    def test_keys_are_collected_and_sorted(self, monkeypatch):
        module = _load_doc_metadata_service(monkeypatch)
        monkeypatch.setattr(
            module.DocMetadataService,
            "_search_metadata",
            MagicMock(
                return_value=[
                    {"id": "doc-1", "meta_fields": {"author": "alice", "year": 2020}},
                    {"id": "doc-2", "meta_fields": {"year": 2021, "city": "Hangzhou"}},
                ]
            ),
        )

        assert module.DocMetadataService.get_metadata_keys_by_kbs(["kb-1"]) == ["author", "city", "year"]


class TestGetMetadataKeysService:
    """``dataset_api_service.get_metadata_keys`` must report lookup failures."""

    def test_retrieval_failure_is_reported_as_an_error(self, monkeypatch):
        retrieval = MagicMock(side_effect=RuntimeError("metadata store unavailable"))
        module = _load_dataset_api_service(monkeypatch, retrieval=retrieval)

        success, result = module.get_metadata_keys(["kb-1"], "tenant-1")

        assert success is False
        assert "metadata store unavailable" in result

    def test_empty_key_set_stays_a_success(self, monkeypatch):
        retrieval = MagicMock(return_value=[])
        module = _load_dataset_api_service(monkeypatch, retrieval=retrieval)

        success, result = module.get_metadata_keys(["kb-1"], "tenant-1")

        assert success is True
        assert result == []

    def test_keys_are_returned_on_success(self, monkeypatch):
        retrieval = MagicMock(return_value=["author", "year"])
        module = _load_dataset_api_service(monkeypatch, retrieval=retrieval)

        success, result = module.get_metadata_keys(["kb-1"], "tenant-1")

        assert success is True
        assert result == ["author", "year"]

    def test_unauthorized_dataset_fails_before_retrieval(self, monkeypatch):
        retrieval = MagicMock(return_value=[])
        module = _load_dataset_api_service(monkeypatch, accessible=False, retrieval=retrieval)

        success, result = module.get_metadata_keys(["kb-1"], "tenant-1")

        assert success is False
        assert result == "No authorization for dataset 'kb-1'"
        retrieval.assert_not_called()

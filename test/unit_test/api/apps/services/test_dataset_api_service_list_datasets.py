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
"""Regression tests for list_datasets() honoring include_parsing_status (#16855).

The ``ListDatasetReq`` model declares ``include_parsing_status: bool = False``,
but ``dataset_api_service.list_datasets`` historically ignored the flag and
returned no parsing-status counts. These tests lock in the contract that
``include_parsing_status`` controls whether
``DocumentService.get_parsing_status_by_kb_ids`` is invoked and whether the
result is attached to each kb record.
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
    """Identity stand-in for ``remap_dictionary_keys``.

    The real helper only renames a few keys (e.g. ``chunk_num`` -> ``chunk_count``)
    and otherwise copies through. For these tests we only care that
    ``parsing_status`` is preserved on the output record, so identity is enough.
    """
    if key_aliases is None:
        return dict(source_data)
    out = {}
    for k, v in source_data.items():
        out[key_aliases.get(k, k)] = v
    return out


def _load_list_datasets_module(monkeypatch, *, kbs, parsing_status_by_kb):
    parsing_status_mock = MagicMock(return_value=parsing_status_by_kb)
    get_list_mock = MagicMock(return_value=(list(kbs), len(kbs)))

    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
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
        File=SimpleNamespace(),
    )
    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(
            get_parsing_status_by_kb_ids=parsing_status_mock,
        ),
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
        KnowledgebaseService=SimpleNamespace(
            get_list=get_list_mock,
        ),
        validate_dataset_embedding_models=lambda kbs: None,
    )
    _stub(
        monkeypatch,
        "api.db.services.connector_service",
        Connector2KbService=SimpleNamespace(),
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
        TenantService=SimpleNamespace(
            get_joined_tenants_by_user_id=lambda user_id: [{"tenant_id": "tenant-1"}],
        ),
        UserService=SimpleNamespace(get_by_ids=lambda ids: []),
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
    )

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "services" / "dataset_api_service.py"
    spec = importlib.util.spec_from_file_location("test_dataset_api_service_list_datasets_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_dataset_api_service_list_datasets_module", module)
    spec.loader.exec_module(module)
    return module, parsing_status_mock, get_list_mock


def _stub_kbs():
    return [
        {"id": "kb-a", "tenant_id": "tenant-1", "name": "Alpha"},
        {"id": "kb-b", "tenant_id": "tenant-1", "name": "Beta"},
    ]


def test_list_datasets_without_include_parsing_status_does_not_call_helper(monkeypatch):
    """No flag → no helper call, no parsing_status on response."""
    module, parsing_status_mock, get_list_mock = _load_list_datasets_module(
        monkeypatch,
        kbs=_stub_kbs(),
        parsing_status_by_kb={},
    )

    ok, payload = module.list_datasets("tenant-1", {"page": 1, "page_size": 30})

    assert ok is True
    assert payload["total"] == 2
    assert len(payload["data"]) == 2
    for record in payload["data"]:
        assert "parsing_status" not in record
    parsing_status_mock.assert_not_called()
    get_list_mock.assert_called_once()


def test_list_datasets_with_include_parsing_status_true_attaches_counts(monkeypatch):
    """Flag True → helper called once with the kb ids, counts attached."""
    status_by_kb = {
        "kb-a": {
            "unstart_count": 3,
            "running_count": 1,
            "cancel_count": 0,
            "done_count": 7,
            "fail_count": 2,
        },
        "kb-b": {
            "unstart_count": 0,
            "running_count": 0,
            "cancel_count": 1,
            "done_count": 4,
            "fail_count": 0,
        },
    }
    module, parsing_status_mock, _ = _load_list_datasets_module(
        monkeypatch,
        kbs=_stub_kbs(),
        parsing_status_by_kb=status_by_kb,
    )

    ok, payload = module.list_datasets(
        "tenant-1",
        {"page": 1, "page_size": 30, "include_parsing_status": True},
    )

    assert ok is True
    parsing_status_mock.assert_called_once_with(["kb-a", "kb-b"])
    by_id = {r["id"]: r for r in payload["data"]}
    assert by_id["kb-a"]["parsing_status"] == status_by_kb["kb-a"]
    assert by_id["kb-b"]["parsing_status"] == status_by_kb["kb-b"]


def test_list_datasets_with_include_parsing_status_string_true(monkeypatch):
    """Flag string 'true' is also truthy (matches existing pattern in list_datasets)."""
    module, parsing_status_mock, _ = _load_list_datasets_module(
        monkeypatch,
        kbs=_stub_kbs(),
        parsing_status_by_kb={
            "kb-a": {"unstart_count": 0, "running_count": 0, "cancel_count": 0, "done_count": 1, "fail_count": 0},
            "kb-b": {"unstart_count": 0, "running_count": 0, "cancel_count": 0, "done_count": 0, "fail_count": 0},
        },
    )

    ok, payload = module.list_datasets(
        "tenant-1",
        {"include_parsing_status": "true"},
    )

    assert ok is True
    parsing_status_mock.assert_called_once_with(["kb-a", "kb-b"])
    assert "parsing_status" in payload["data"][0]


def test_list_datasets_with_include_parsing_status_false_skips_helper(monkeypatch):
    """Explicit False behaves like the absent flag."""
    module, parsing_status_mock, _ = _load_list_datasets_module(
        monkeypatch,
        kbs=_stub_kbs(),
        parsing_status_by_kb={},
    )

    ok, payload = module.list_datasets(
        "tenant-1",
        {"include_parsing_status": False},
    )

    assert ok is True
    for record in payload["data"]:
        assert "parsing_status" not in record
    parsing_status_mock.assert_not_called()


def test_list_datasets_with_include_parsing_status_string_false_skips_helper(monkeypatch):
    """String 'false' / '0' / '' are not truthy."""
    module, parsing_status_mock, _ = _load_list_datasets_module(
        monkeypatch,
        kbs=_stub_kbs(),
        parsing_status_by_kb={},
    )

    for falsy in ("false", "False", "0", ""):
        parsing_status_mock.reset_mock()
        ok, payload = module.list_datasets(
            "tenant-1",
            {"include_parsing_status": falsy},
        )
        assert ok is True, falsy
        for record in payload["data"]:
            assert "parsing_status" not in record, falsy
        parsing_status_mock.assert_not_called()


def test_list_datasets_with_empty_kb_list_skips_helper_even_when_flag_true(monkeypatch):
    """Empty page: no kb ids, no helper call, no error."""
    module, parsing_status_mock, _ = _load_list_datasets_module(
        monkeypatch,
        kbs=[],
        parsing_status_by_kb={},
    )

    ok, payload = module.list_datasets(
        "tenant-1",
        {"include_parsing_status": True},
    )

    assert ok is True
    assert payload == {"data": [], "total": 0}
    parsing_status_mock.assert_not_called()


def test_list_datasets_with_include_parsing_status_missing_kb_gets_empty_dict(monkeypatch):
    """If the helper omits a kb_id, the response record gets an empty dict."""
    module, parsing_status_mock, _ = _load_list_datasets_module(
        monkeypatch,
        kbs=_stub_kbs(),
        parsing_status_by_kb={
            "kb-a": {"unstart_count": 1, "running_count": 0, "cancel_count": 0, "done_count": 0, "fail_count": 0},
        },
    )

    ok, payload = module.list_datasets(
        "tenant-1",
        {"include_parsing_status": True},
    )

    assert ok is True
    by_id = {r["id"]: r for r in payload["data"]}
    assert by_id["kb-a"]["parsing_status"]["unstart_count"] == 1
    assert by_id["kb-b"]["parsing_status"] == {}
    parsing_status_mock.assert_called_once()

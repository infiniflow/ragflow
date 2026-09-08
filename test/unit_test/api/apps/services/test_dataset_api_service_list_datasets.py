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
"""Regression tests for list_datasets() honoring include_parsing_status (#16855, #17595).

The ``ListDatasetReq`` model declares ``include_parsing_status: bool = False``,
but ``dataset_api_service.list_datasets`` historically ignored the flag and
returned no parsing-status counts. These tests lock in the contract that
``include_parsing_status`` controls whether
``DocumentService.get_parsing_status_by_kb_ids`` is invoked and whether the
counts are merged into each kb record.

Per the HTTP API and Python SDK references, the counts belong at the *top level*
of each dataset record (``done_count``, ``fail_count``, ...), not under a nested
``parsing_status`` object (#17595).
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
    and otherwise copies through. For these tests we only care that the parsing
    status counts are preserved on the output record, so identity is enough.
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
    get_accessible_ids_mock = MagicMock(return_value={kb["id"] for kb in kbs})

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
            get_accessible_ids=get_accessible_ids_mock,
        ),
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
        thread_pool_exec_long_time=MagicMock(),
    )
    _stub(monkeypatch, "rag.advanced_rag", __path__=[])
    _stub(monkeypatch, "rag.advanced_rag.knowlege_compile", __path__=[])
    _stub(
        monkeypatch,
        "rag.advanced_rag.knowlege_compile.wiki",
        WIKI_PAGE_COMPILE_KWD="wiki",
        _chunk_hash=lambda content: "stub-hash",
    )

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "services" / "dataset_api_service.py"
    spec = importlib.util.spec_from_file_location("test_dataset_api_service_list_datasets_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_dataset_api_service_list_datasets_module", module)
    spec.loader.exec_module(module)
    return module, parsing_status_mock, get_list_mock


#: Fields ``DocumentService.get_parsing_status_by_kb_ids`` reports, as documented
#: for the List Datasets endpoint.
_PARSING_STATUS_FIELDS = (
    "unstart_count",
    "running_count",
    "cancel_count",
    "done_count",
    "fail_count",
)


def _assert_no_counts(record, context=""):
    assert "parsing_status" not in record, context
    for field in _PARSING_STATUS_FIELDS:
        assert field not in record, f"{field} {context}"


def _stub_kbs():
    return [
        {"id": "kb-a", "tenant_id": "tenant-1", "name": "Alpha", "embedding_model": "emb-a"},
        {"id": "kb-b", "tenant_id": "tenant-1", "name": "Beta", "embedding_model": "emb-b"},
    ]


def test_list_datasets_without_include_parsing_status_does_not_call_helper(monkeypatch):
    """No flag → no helper call, no counts on response."""
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
        _assert_no_counts(record)
    parsing_status_mock.assert_not_called()
    get_list_mock.assert_called_once()


def test_list_datasets_with_include_parsing_status_true_attaches_counts(monkeypatch):
    """Flag True → helper called once with the kb ids, counts merged at top level."""
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
    for kb_id, counts in status_by_kb.items():
        assert "parsing_status" not in by_id[kb_id]
        for field, value in counts.items():
            assert by_id[kb_id][field] == value


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
    assert payload["data"][0]["done_count"] == 1


def test_list_datasets_with_ids_filters_query_once(monkeypatch):
    """ids filter is checked once and pushed into the list query."""
    module, _, get_list_mock = _load_list_datasets_module(
        monkeypatch,
        kbs=_stub_kbs(),
        parsing_status_by_kb={},
    )

    ok, payload = module.list_datasets(
        "tenant-1",
        {"page": 1, "page_size": 30, "ids": ["kb-a", "kb-b"]},
    )

    assert ok is True
    assert payload["total"] == 2
    module.KnowledgebaseService.get_accessible_ids.assert_called_once_with(["tenant-1"], "tenant-1", ["kb-a", "kb-b"])
    get_list_mock.assert_called_once_with(["tenant-1"], "tenant-1", 1, 30, "create_time", True, None, None, "", None, ["kb-a", "kb-b"])


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
        _assert_no_counts(record)
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
            _assert_no_counts(record, falsy)
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


def test_list_datasets_with_include_parsing_status_missing_kb_gets_no_counts(monkeypatch):
    """If the helper omits a kb_id, the response record simply carries no counts."""
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
    assert by_id["kb-a"]["unstart_count"] == 1
    _assert_no_counts(by_id["kb-b"])
    parsing_status_mock.assert_called_once()


def test_string_list_decodes_legacy_json_and_native_arrays(monkeypatch):
    module, _, _ = _load_list_datasets_module(
        monkeypatch,
        kbs=[],
        parsing_status_by_kb={},
    )

    assert module._string_list('["doc_1", "doc_2"]') == ["doc_1", "doc_2"]
    assert module._string_list(["doc_1", "doc_2", "doc_1"]) == ["doc_1", "doc_2"]
    assert module._string_list("doc_1###doc_2") == ["doc_1", "doc_2"]


def test_wiki_alteration_treats_wiki_template_as_eligible(monkeypatch):
    module, _, _ = _load_list_datasets_module(
        monkeypatch,
        kbs=[],
        parsing_status_by_kb={},
    )
    _stub(
        monkeypatch,
        "api.db.services.compilation_template_service",
        CompilationTemplateService=SimpleNamespace(
            get_saved=lambda template_id, tenant_id: {
                "id": template_id,
                "kind": "wiki",
                "config": {"kind": "wiki"},
            }
        ),
    )
    _stub(monkeypatch, "rag.svr", __path__=[])
    _stub(monkeypatch, "rag.svr.task_executor_refactor", __path__=[])
    _stub(
        monkeypatch,
        "rag.svr.task_executor_refactor.chunk_post_processor",
        _parser_config_compilation_template_ids=lambda parser_config, tenant_id: parser_config.get("compilation_template_ids", []),
    )

    docs = [
        {
            "id": "doc-wiki",
            "status": "1",
            "parser_config": {"compilation_template_ids": ["template-wiki"]},
        }
    ]

    assert module._eligible_doc_ids_for_kind(docs, "tenant-1", "wiki") == {"doc-wiki"}


@pytest.mark.asyncio
async def test_structure_alteration_chunk_filter_excludes_unparsed_documents(monkeypatch):
    module, _, _ = _load_list_datasets_module(
        monkeypatch,
        kbs=[],
        parsing_status_by_kb={},
    )

    captured = {}

    async def _paged(_index, dataset_id, condition, field, from_list, *, raise_on_error):
        captured.update(dataset_id=dataset_id, condition=condition, field=field, from_list=from_list)
        assert raise_on_error is True
        return {"doc-with-chunk"}

    monkeypatch.setattr(module, "_involved_doc_ids_paged", _paged)

    result = await module._current_chunk_doc_ids(
        "tenant-index",
        "kb-1",
        {"doc-with-chunk", "doc-without-chunk"},
    )

    assert result == {"doc-with-chunk"}
    assert captured == {
        "dataset_id": "kb-1",
        "condition": {
            "doc_id": ["doc-with-chunk", "doc-without-chunk"],
            "available_int": [1],
            "must_not": {"exists": "compile_kwd"},
        },
        "field": "doc_id",
        "from_list": False,
    }


@pytest.mark.asyncio
async def test_current_chunk_doc_ids_propagates_search_errors(monkeypatch):
    module, _, _ = _load_list_datasets_module(
        monkeypatch,
        kbs=[],
        parsing_status_by_kb={},
    )

    async def _paged(*_args, **_kwargs):
        raise RuntimeError("search unavailable")

    monkeypatch.setattr(module, "_involved_doc_ids_paged", _paged)

    with pytest.raises(RuntimeError, match="search unavailable"):
        await module._current_chunk_doc_ids("tenant-index", "kb-1", {"doc-1"})


@pytest.mark.asyncio
async def test_wiki_involved_ids_use_active_map_state_when_pages_are_missing(monkeypatch):
    """Use MAP provenance even when a participating document has no page row."""
    module, _, _ = _load_list_datasets_module(
        monkeypatch,
        kbs=[],
        parsing_status_by_kb={},
    )

    async def _load_active_map_state(tenant_id, dataset_id):
        assert tenant_id == "tenant-1"
        assert dataset_id == "kb-1"
        return {
            "chunk-a": {"doc_id": "doc-with-page"},
            "chunk-b": {"doc_id": "doc-without-page"},
        }

    wiki_stub = sys.modules["rag.advanced_rag.knowlege_compile.wiki"]
    wiki_stub._wiki_load_active_map_state = _load_active_map_state

    involved = await module._involved_doc_ids_for_kind("index", "kb-1", "wiki", "tenant-1")

    assert involved == {"doc-with-page", "doc-without-page"}


@pytest.mark.asyncio
async def test_wiki_chunk_alteration_uses_full_successful_state(monkeypatch):
    """Reuse supplied current and previous states without scanning the store again."""
    module, _, _ = _load_list_datasets_module(
        monkeypatch,
        kbs=[],
        parsing_status_by_kb={},
    )

    previous = {
        "chunk-changed": {"doc_id": "doc-existing", "hash": "old"},
        "chunk-deleted": {"doc_id": "doc-existing", "hash": "same"},
        "chunk-template-off": {"doc_id": "doc-template-off", "hash": "same"},
        "chunk-removed-doc": {"doc_id": "doc-removed", "hash": "same"},
    }
    current = {
        "chunk-changed": {"doc_id": "doc-existing", "hash": "new"},
        "chunk-new-existing": {"doc_id": "doc-existing", "hash": "same"},
        "chunk-new-document": {"doc_id": "doc-new", "hash": "same"},
    }

    async def _load_state(tenant_id, dataset_id):
        return previous

    async def _scan_state(tenant_id, dataset_id, doc_ids):
        assert doc_ids == {"doc-existing", "doc-new"}
        return current

    def _compare_states(old, new):
        old_ids = set(old)
        new_ids = set(new)
        common = old_ids & new_ids
        return {
            "new_chunk_ids": new_ids - old_ids,
            "changed_chunk_ids": {chunk_id for chunk_id in common if old[chunk_id]["hash"] != new[chunk_id]["hash"]},
            "deleted_chunk_ids": old_ids - new_ids,
            "unchanged_chunk_ids": {chunk_id for chunk_id in common if old[chunk_id]["hash"] == new[chunk_id]["hash"]},
        }

    _stub(
        monkeypatch,
        "rag.advanced_rag.knowlege_compile.wiki",
        _wiki_compare_chunk_states=_compare_states,
        _wiki_load_active_map_state=_load_state,
        _wiki_scan_current_chunk_state=_scan_state,
    )

    result = await module._wiki_chunk_alteration(
        "tenant-1",
        "kb-1",
        {"doc-existing", "doc-new"},
        {"doc-existing", "doc-template-off", "doc-removed"},
        current_chunk_state=current,
        previous_map_state=previous,
    )

    assert result == {
        "changed": 1,
        "changed_doc_ids": ["doc-existing"],
    }

    membership = module._alteration_result(
        {"doc-existing", "doc-new"},
        {"doc-existing", "doc-template-off", "doc-removed"},
        {"doc-existing", "doc-new"},
    )
    assert membership["removed_doc_ids"] == ["doc-removed", "doc-template-off"]
    assert membership["newly_uploaded_doc_ids"] == ["doc-new"]

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
"""Regression tests for check_embedding() validating ``check_num`` (issue #19567).

The service path used to call ``int(req.get("check_num", 5))`` with no
guard. A non-numeric value raised ``ValueError`` and bubbled up as a 500;
zero or negative values reached the sampler and produced a misleading
"No embedded chunks are available to compare" result.

These tests pin the contract that ``check_embedding`` rejects both classes
of bad input with a clear argument error before any sampling or model
load happens.
"""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock


def _stub(monkeypatch, name, **attrs):
    """Register a synthetic ``name`` in ``sys.modules`` with the given attrs.

    Stubs the parent chain too so ``from <name> import <attr>`` works.
    """
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    if "." in name:
        parent_name, _, child_name = name.rpartition(".")
        parent_mod = sys.modules.get(parent_name)
        if parent_mod is not None and not hasattr(parent_mod, child_name):
            monkeypatch.setattr(parent_mod, child_name, mod, raising=False)
    return mod


def _load_check_embedding_module(
    monkeypatch,
    *,
    accessible=True,
    kb_exists=True,
    embedding_available=True,
):
    """Load ``dataset_api_service`` with heavy transitive imports stubbed.

    The function does ``from rag.nlp import search`` inside its body, so we
    register a stub ``rag.nlp`` package whose ``search`` attribute is a
    SimpleNamespace — that way Python never loads the real
    ``rag/nlp/__init__.py`` (which transitively pulls in tiktoken / heavy
    deps we don't need here).
    """
    accessible_mock = MagicMock(return_value=accessible)
    get_by_id_mock = MagicMock(return_value=(kb_exists, SimpleNamespace(tenant_id="t-1")))
    verify_mock = MagicMock(return_value=(embedding_available, "OK" if embedding_available else "Embedding unavailable"))
    resolve_model_config_mock = MagicMock(return_value={"model_type": "embedding"})

    encode_mock = MagicMock(return_value=([[0.0] * 4], [0.0] * 4))
    llmbundle_mock = MagicMock()
    llmbundle_mock.return_value.encode = encode_mock

    _stub(monkeypatch, "peewee", fn=lambda *a, **kw: None)
    _stub(monkeypatch, "api.apps", __path__=[])
    _stub(monkeypatch, "api.apps.services", __path__=[])
    _stub(monkeypatch, "api.db", VALID_PIPELINE_TASK_TYPES=set())
    _stub(
        monkeypatch,
        "api.db.db_models",
        Connector2Kb=SimpleNamespace(kb_id="kb_id"),
        Document=SimpleNamespace(kb_id="kb_id"),
        File=SimpleNamespace(),
        SyncLogs=SimpleNamespace(kb_id="kb_id"),
        Task=SimpleNamespace(),
        DB=MagicMock(),
    )
    _stub(monkeypatch, "api.db.services.connector_service", Connector2KbService=SimpleNamespace(), SyncLogsService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.document_service", DocumentService=SimpleNamespace(), queue_raptor_o_graphrag_tasks=MagicMock())
    _stub(monkeypatch, "api.db.services.file2document_service", File2DocumentService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.file_service", FileService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(
            accessible=accessible_mock,
            get_by_id=get_by_id_mock,
        ),
        validate_dataset_embedding_models=lambda kbs: None,
    )
    _stub(monkeypatch, "api.db.services.task_service", GRAPH_RAPTOR_FAKE_DOC_ID="fake-doc", TaskService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.tenant_model_service", TenantModelService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.user_service", TenantService=SimpleNamespace(), UserService=SimpleNamespace(), UserTenantService=SimpleNamespace())
    _stub(monkeypatch, "api.db.joint_services.tenant_model_service", get_composite_model_name_by_ids=MagicMock(), resolve_model_config=resolve_model_config_mock, resolve_model_id=MagicMock())
    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=llmbundle_mock)
    _stub(monkeypatch, "rag.nlp", search=SimpleNamespace(index_name=lambda tenant_id: f"index_{tenant_id}"))
    _stub(monkeypatch, "common.doc_store.doc_store_base", OrderByExpr=SimpleNamespace)
    # Stub only the submodules the service imports at module level, never
    # the ``common`` package itself — replacing ``common`` would hide its
    # submodules from the runtime.
    _stub(
        monkeypatch,
        "common.constants",
        PAGERANK_FLD="pagerank",
        FileSource=SimpleNamespace(KNOWLEDGEBASE="knowledgebase"),
        LLMType=SimpleNamespace(EMBEDDING="embedding"),
        RetCode=SimpleNamespace(),
        StatusEnum=SimpleNamespace(),
        TaskStatus=SimpleNamespace(),
    )
    _stub(monkeypatch, "common.misc_utils", thread_pool_exec=lambda *a, **kw: None, thread_pool_exec_long_time=lambda *a, **kw: None)
    _stub(monkeypatch, "common.settings", docStoreConn=SimpleNamespace())
    _stub(monkeypatch, "rag.advanced_rag.knowlege_compile.wiki", WIKI_PAGE_COMPILE_KWD="wiki_page_compile")

    def _deep_merge(*a, **kw):
        return {}

    def _get_parser_config(*a, **kw):
        return {}

    def _remap_dictionary_keys(d, *a, **kw):
        return dict(d) if d else {}

    def _verify_embedding_availability(*a, **kw):
        return verify_mock()

    _stub(
        monkeypatch,
        "api.utils.api_utils",
        deep_merge=_deep_merge,
        get_parser_config=_get_parser_config,
        remap_dictionary_keys=_remap_dictionary_keys,
        verify_embedding_availability=_verify_embedding_availability,
    )

    service_path = Path(__file__).resolve().parents[5] / "api" / "apps" / "services" / "dataset_api_service.py"
    spec = importlib.util.spec_from_file_location("_das_check_test", str(service_path))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _sample_chunks(size):
    return [
        {
            "chunk_id": f"c{i}",
            "doc_id": f"d{i}",
            "doc_name": f"doc{i}",
            "vector_field": "q_1024_vec",
            "vector_dim": 1024,
            "vector": [0.1] * 4,
            "content_with_weight": "hello world",
            "question_kwd": [],
            "page_num_int": 1,
            "position_int": 1,
            "top_int": 1,
        }
        for i in range(size)
    ]


def _stub_doc_store_with_chunks(monkeypatch, n_chunks):
    """Replace ``settings.docStoreConn`` with a mock that returns ``n_chunks``
    samples from the inner ``sample_random_chunks_with_vectors`` closure.

    The sampler is a closure inside ``check_embedding``, so we can't monkeypatch
    it directly. We patch the doc store it queries instead: ``search`` returns
    a stub carrying the chunk IDs, ``get_doc_ids`` extracts them, ``get`` returns
    a full doc dict with the fields the sampler reads.
    """
    chunks = _sample_chunks(n_chunks)
    chunk_by_id = {c["chunk_id"]: c for c in chunks}
    total = len(chunks)

    stub_result = SimpleNamespace(ids=[c["chunk_id"] for c in chunks])
    doc_store = MagicMock()
    doc_store.search.return_value = stub_result
    doc_store.get_total.return_value = total
    doc_store.get_doc_ids.side_effect = lambda r: r.ids
    doc_store.get.side_effect = lambda cid, *a, **kw: chunk_by_id.get(cid, {})
    doc_store.db_type = MagicMock(return_value="elasticsearch")
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", doc_store)
    return doc_store, chunks


# --------------------------------------------------------------------------- #
# Tests
# --------------------------------------------------------------------------- #


def test_non_numeric_check_num_returns_argument_error(monkeypatch):
    """``check_num="invalid"`` must return (False, 'must be an integer') and
    never call the sampler — the bare ``int(...)`` would have raised
    ValueError, which the route then surfaced as a 500."""
    svc = _load_check_embedding_module(monkeypatch)
    doc_store, _ = _stub_doc_store_with_chunks(monkeypatch, n_chunks=3)
    llmbundle = sys.modules["api.db.services.llm_service"].LLMBundle

    ok, msg = svc.check_embedding("kb-1", "t-1", {"embd_id": "m1", "check_num": "invalid"})

    assert ok is False
    assert "must be an integer" in msg
    assert "500" not in msg and "Internal" not in msg
    doc_store.search.assert_not_called()
    llmbundle.assert_not_called()


def test_boolean_check_num_returns_argument_error(monkeypatch):
    """``check_num=True`` reaches ``int(True) == 1`` on the unfixed code
    (``bool`` is an ``int`` subclass); the guard must reject it."""
    svc = _load_check_embedding_module(monkeypatch)
    doc_store, _ = _stub_doc_store_with_chunks(monkeypatch, n_chunks=3)
    llmbundle = sys.modules["api.db.services.llm_service"].LLMBundle

    ok, msg = svc.check_embedding("kb-1", "t-1", {"embd_id": "m1", "check_num": True})

    assert ok is False
    assert "greater than 0" in msg
    doc_store.search.assert_not_called()
    llmbundle.assert_not_called()


def test_zero_check_num_returns_argument_error(monkeypatch):
    """``check_num=0`` must return (False, 'must be greater than 0') and not
    reach the sampler (which would have produced a misleading "No embedded
    chunks are available to compare" result)."""
    svc = _load_check_embedding_module(monkeypatch)
    doc_store, _ = _stub_doc_store_with_chunks(monkeypatch, n_chunks=3)
    llmbundle = sys.modules["api.db.services.llm_service"].LLMBundle

    ok, msg = svc.check_embedding("kb-1", "t-1", {"embd_id": "m1", "check_num": 0})

    assert ok is False
    assert "greater than 0" in msg
    doc_store.search.assert_not_called()
    llmbundle.assert_not_called()


def test_negative_check_num_returns_argument_error(monkeypatch):
    svc = _load_check_embedding_module(monkeypatch)
    doc_store, _ = _stub_doc_store_with_chunks(monkeypatch, n_chunks=3)
    llmbundle = sys.modules["api.db.services.llm_service"].LLMBundle

    ok, msg = svc.check_embedding("kb-1", "t-1", {"embd_id": "m1", "check_num": -3})

    assert ok is False
    assert "greater than 0" in msg
    doc_store.search.assert_not_called()
    llmbundle.assert_not_called()


def test_valid_check_num_reaches_sampler_with_that_n(monkeypatch):
    """A positive integer must reach the sampler with that exact value
    (``n`` clamped to ``min(n, total)`` and bounded by ``min(total, 1000)``)."""
    svc = _load_check_embedding_module(monkeypatch)
    doc_store, _ = _stub_doc_store_with_chunks(monkeypatch, n_chunks=12)

    svc.check_embedding("kb-1", "t-1", {"embd_id": "m1", "check_num": 12})

    # Sampler called: first call is the existence probe (``limit=1``),
    # then one call per offset up to ``n=12``. We only assert the first
    # call happened — that's enough to prove the guard let control through.
    doc_store.search.assert_called()
    first_call = doc_store.search.call_args_list[0]
    assert first_call.kwargs.get("limit") == 1


def test_missing_check_num_defaults_to_five(monkeypatch):
    """The previous happy-path contract (``check_num`` absent → 5) must keep
    working; the guard must NOT fire when the caller omits the field."""
    svc = _load_check_embedding_module(monkeypatch)
    doc_store, _ = _stub_doc_store_with_chunks(monkeypatch, n_chunks=5)

    svc.check_embedding("kb-1", "t-1", {"embd_id": "m1"})

    doc_store.search.assert_called()


def test_fractional_check_num_is_rejected(monkeypatch):
    """``int(1.9) == 1`` would otherwise pass the positive check and request
    one sample; the guard must reject non-integral floats explicitly."""
    svc = _load_check_embedding_module(monkeypatch)
    doc_store, _ = _stub_doc_store_with_chunks(monkeypatch, n_chunks=3)
    llmbundle = sys.modules["api.db.services.llm_service"].LLMBundle

    ok, msg = svc.check_embedding("kb-1", "t-1", {"embd_id": "m1", "check_num": 1.9})

    assert ok is False
    assert "integer" in msg
    doc_store.search.assert_not_called()
    llmbundle.assert_not_called()


def test_integral_float_check_num_is_accepted(monkeypatch):
    """An integral float (``2.0``) is still accepted — ``int(2.0) == 2``."""
    svc = _load_check_embedding_module(monkeypatch)
    doc_store, _ = _stub_doc_store_with_chunks(monkeypatch, n_chunks=5)

    svc.check_embedding("kb-1", "t-1", {"embd_id": "m1", "check_num": 2.0})

    doc_store.search.assert_called()


def test_check_num_above_1000_is_rejected(monkeypatch):
    """``random.sample(range(min(total, 1000)), n)`` raises ``ValueError`` when
    ``n`` exceeds the population. The guard must reject ``n > 1000`` before
    reaching the sampler."""
    svc = _load_check_embedding_module(monkeypatch)
    doc_store, _ = _stub_doc_store_with_chunks(monkeypatch, n_chunks=5000)
    llmbundle = sys.modules["api.db.services.llm_service"].LLMBundle

    ok, msg = svc.check_embedding("kb-1", "t-1", {"embd_id": "m1", "check_num": 1001})

    assert ok is False
    assert "1000" in msg
    doc_store.search.assert_not_called()
    llmbundle.assert_not_called()


def test_check_num_at_1000_boundary_is_accepted(monkeypatch):
    """The sampler ceiling is 1000; ``n == 1000`` must be accepted (it
    matches the sampler cap exactly)."""
    svc = _load_check_embedding_module(monkeypatch)
    doc_store, _ = _stub_doc_store_with_chunks(monkeypatch, n_chunks=2000)

    svc.check_embedding("kb-1", "t-1", {"embd_id": "m1", "check_num": 1000})

    doc_store.search.assert_called()

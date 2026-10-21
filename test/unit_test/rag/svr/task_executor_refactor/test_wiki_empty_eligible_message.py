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
"""Regression tests for ``_wiki_empty_eligible_message`` in
``rag/svr/task_executor_refactor/dataset_wiki_generator.py``.

Issue: infiniflow/ragflow#18683.

On the Artifacts page of a dataset, with the Wiki template selected,
clicking Generate completed in ~0.1s and produced no wiki pages. The
backend logged ``"No enabled documents are configured for wiki compilation."``
but the documents in the dataset were fully parsed and enabled
(``status="1"``). The real reason was that none of them resolved to a
wiki compilation template. Pre-fix, both failure modes
(no-enabled-documents and no-wiki-template) surfaced as the same
misleading "No enabled documents are configured" message.

The fix extracts the error-message logic into
``_wiki_empty_eligible_message(all_docs)`` which returns one of two
distinct messages based on whether enabled documents are present.
"""

import asyncio
import importlib.util
import os
import sys
from types import ModuleType, SimpleNamespace


# ---- Stub heavy dependencies that the wiki generator imports at module
# load time. Each stub provides only the names the module needs.
def _stub_module(name, **attrs):
    module = ModuleType(name)
    for key, value in attrs.items():
        setattr(module, key, value)
    sys.modules[name] = module
    return module


_REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "..", "..", ".."))


async def _await_inline(fn, *args, **kwargs):
    """Stand-in for ``thread_pool_exec``: run inline, return awaitably."""
    return fn(*args, **kwargs)


_STUB_NAMES = (
    "common",
    "common.constants",
    "common.file_utils",
    "common.misc_utils",
    "common.settings",
    "api",
    "api.db",
    "api.db.services",
    "api.db.joint_services",
    "api.apps",
    "api.apps.restful_apis",
    "api.utils",
    "rag",
    "rag.advanced_rag",
    "rag.advanced_rag.knowlege_compile",
    "rag.advanced_rag.knowlege_compile._common",
    "rag.nlp",
    "rag.utils",
    "rag.utils.es_connection",
    "rag.utils.infinity_conn",
    "rag.utils.opensearch_conn",
    "rag.utils.ob_conn",
)


def _load_wiki_generator(monkeypatch):
    snapshot = {name: sys.modules.get(name) for name in _STUB_NAMES}
    # Common / config stubs.
    common = _stub_module("common")
    common.__path__ = [os.path.join(_REPO_ROOT, "common")]
    common.settings = ModuleType("common.settings")
    sys.modules["common.settings"] = common.settings
    _stub_module(
        "common.constants",
        MAXIMUM_PAGE_NUMBER=1024,
        PAGERANK_F=0.85,
        WIKI_PAGE_COMPILE_KWD="wiki_compiled",
        # LLMType is referenced at module load; an int placeholder is enough
        # because the helper under test does not branch on its value.
        LLMType=0,
    )
    _stub_module("common.file_utils", get_project_base_directory=lambda: _REPO_ROOT)
    _stub_module(
        "common.settings",
        docStoreConn=ModuleType("common.settings.docStoreConn"),
    )
    _stub_module("common.misc_utils", thread_pool_exec=_await_inline)
    # Project-level stubs so ``from api.... import …`` resolves.
    _stub_module("api")
    _stub_module("api.db")
    _stub_module("api.db.services")
    _stub_module("api.db.joint_services")
    _stub_module("api.apps")
    _stub_module("api.apps.restful_apis")
    _stub_module("api.utils")
    # RAG subpackage stubs.
    _stub_module("rag")
    _stub_module("rag.advanced_rag")
    _stub_module("rag.advanced_rag.knowlege_compile")
    # Set __path__ on the knowlege_compile stub so that submodules like
    # ``structure`` resolve as packages rather than attribute lookups.
    sys.modules["rag.advanced_rag.knowlege_compile"].__path__ = [os.path.join(_REPO_ROOT, "rag", "advanced_rag", "knowlege_compile")]
    _stub_module(
        "rag.advanced_rag.knowlege_compile._common",
        env_int=lambda _name, default, **_kwargs: default,
    )
    sys.modules["rag.advanced_rag.knowlege_compile"].structure = ModuleType("rag.advanced_rag.knowlege_compile.structure")
    sys.modules["rag.advanced_rag.knowlege_compile.structure"] = sys.modules["rag.advanced_rag.knowlege_compile"].structure
    sys.modules["rag.advanced_rag.knowlege_compile.structure"].LLMCallPool = type("LLMCallPool", (), {})
    _stub_module(
        "rag.advanced_rag.knowlege_compile.wiki",
        WIKI_MAP_STATE_COMPILE_KWD="wiki_map_state",
        WIKI_MAP_STATE_META_COMPILE_KWD="wiki_map_state_meta",
        _wiki_commit_active_map_state=lambda **_kwargs: None,
        _wiki_compare_chunk_states=lambda *_args, **_kwargs: {"new_chunk_ids": set(), "changed_chunk_ids": set(), "deleted_chunk_ids": set(), "unchanged_chunk_ids": set()},
        _wiki_load_active_map_state=lambda *_args, **_kwargs: {},
        _wiki_load_map_extracts_for_state=lambda *_args, **_kwargs: [],
        _wiki_scan_current_chunk_state=lambda *_args, **_kwargs: [],
        wiki_map_from_chunks=lambda *_args, **_kwargs: [],
        wiki_plan_from_reduction=lambda *_args, **_kwargs: [],
        wiki_refine_from_plan=lambda *_args, **_kwargs: [],
    )
    _stub_module(
        "rag.nlp",
        search=ModuleType("rag.nlp.search"),
    )
    sys.modules["rag.nlp"].search.index_name = lambda tenant_id: f"ragflow_{tenant_id}"
    _stub_module("rag.utils")
    _stub_module("rag.utils.es_connection")
    _stub_module("rag.utils.infinity_conn")
    _stub_module("rag.utils.opensearch_conn")
    _stub_module("rag.utils.ob_conn")

    module_path = os.path.join(_REPO_ROOT, "rag", "svr", "task_executor_refactor", "dataset_wiki_generator.py")
    spec = importlib.util.spec_from_file_location("test_dataset_wiki_generator_unit_module", module_path)
    module = importlib.util.module_from_spec(spec)
    sys.modules["test_dataset_wiki_generator_unit_module"] = module
    try:
        spec.loader.exec_module(module)
        return module
    except Exception:
        sys.modules.pop("test_dataset_wiki_generator_unit_module", None)
        raise
    finally:
        for name in _STUB_NAMES:
            if snapshot[name] is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = snapshot[name]


def test_wiki_empty_eligible_message_no_documents(monkeypatch):
    """Regression for #18683: empty dataset still gets the original
    "no enabled documents" hint."""
    module = _load_wiki_generator(monkeypatch)
    msg = module._wiki_empty_eligible_message([])
    assert msg == "No enabled documents are configured for wiki compilation."


def test_wiki_empty_eligible_message_all_disabled(monkeypatch):
    """All docs present but status != "1" are treated as disabled, so
    the user still gets the no-enabled-documents message."""
    module = _load_wiki_generator(monkeypatch)
    docs = [
        {"id": "d1", "status": "0"},
        {"id": "d2", "status": "2"},
    ]
    msg = module._wiki_empty_eligible_message(docs)
    assert msg == "No enabled documents are configured for wiki compilation."


def test_wiki_empty_eligible_message_enabled_but_no_template(monkeypatch):
    """Regression for #18683: enabled docs but no wiki template — the
    pre-fix message ("No enabled documents are configured") was misleading
    because the docs WERE enabled. The post-fix message names the real
    fix the user should make."""
    module = _load_wiki_generator(monkeypatch)
    docs = [
        {"id": "d1", "status": "1"},
        {"id": "d2", "status": "1"},
        {"id": "d3", "status": "0"},  # disabled, must not be counted
    ]
    msg = module._wiki_empty_eligible_message(docs)
    assert "2 enabled document(s) found" in msg
    assert "Wiki compilation template attached" in msg
    assert "parser_config" in msg


def test_wiki_empty_eligible_message_handles_missing_status(monkeypatch):
    """A doc without a status field defaults to "1" (enabled), matching
    the convention used by ``_wiki_eligible_docs``."""
    module = _load_wiki_generator(monkeypatch)
    docs = [{"id": "d1"}]  # no status
    msg = module._wiki_empty_eligible_message(docs)
    assert "1 enabled document(s) found" in msg
    assert "Wiki compilation template attached" in msg


def test_wiki_empty_eligible_message_handles_none(monkeypatch):
    """``all_docs`` may be ``None`` (e.g. service returns None on error)."""
    module = _load_wiki_generator(monkeypatch)
    msg = module._wiki_empty_eligible_message(None)
    assert msg == "No enabled documents are configured for wiki compilation."


# ---------------------------------------------------------------------------
# wiki_mode_meta column upgrades: an Infinity table created before
# ``mode_kwd`` / ``embedding_model_kwd`` existed fails both read and write.
# ---------------------------------------------------------------------------


class _RecordingConn:
    """Minimal doc-store stand-in recording the calls that matter here."""

    def __init__(self, *, ensure: bool = True, search_error: Exception | None = None):
        self.calls = []
        self._search_error = search_error
        if ensure:

            def _ensure(index, kb_id, column_defs):
                self.calls.append(("ensure_columns", index, kb_id, dict(column_defs)))

            self.ensure_columns = _ensure

    def index_exist(self, index, kb_id):
        return True

    def insert(self, rows, index, kb_id):
        self.calls.append(("insert", index, kb_id, rows))

    def search(self, *args, **kwargs):
        self.calls.append(("search", args))
        if self._search_error is not None:
            raise self._search_error
        return {}

    def get_fields(self, res, fields):
        return {}


def _patch_wiki_module(monkeypatch, module, conn):
    monkeypatch.setattr(module, "settings", SimpleNamespace(docStoreConn=conn))
    monkeypatch.setattr(module, "_WIKI_MODE_COLUMNS_ENSURED", set())
    base = ModuleType("common.doc_store.doc_store_base")
    base.OrderByExpr = type("OrderByExpr", (), {})
    monkeypatch.setitem(sys.modules, "common.doc_store", ModuleType("common.doc_store"))
    monkeypatch.setitem(sys.modules, "common.doc_store.doc_store_base", base)


def test_wiki_save_mode_ensures_columns_before_insert(monkeypatch):
    """The two mode columns are added before the meta row is written."""
    module = _load_wiki_generator(monkeypatch)
    conn = _RecordingConn()
    _patch_wiki_module(monkeypatch, module, conn)

    asyncio.run(module._wiki_save_mode("tenant_1", "kb_1", "entity", "openai:embd:m1"))

    kinds = [call[0] for call in conn.calls]
    assert kinds == ["ensure_columns", "insert"]
    _, index, kb_id, column_defs = conn.calls[0]
    assert (index, kb_id) == ("ragflow_tenant_1", "kb_1")
    assert set(column_defs) == {"mode_kwd", "embedding_model_kwd"}
    row = conn.calls[1][3][0]
    assert row["mode_kwd"] == "entity"
    assert row["embedding_model_kwd"] == "openai:embd:m1"


def test_wiki_save_mode_upgrades_each_table_once(monkeypatch):
    """Later saves reuse the already-upgraded table instead of re-issuing DDL."""
    module = _load_wiki_generator(monkeypatch)
    conn = _RecordingConn()
    _patch_wiki_module(monkeypatch, module, conn)

    asyncio.run(module._wiki_save_mode("tenant_1", "kb_1", "entity"))
    asyncio.run(module._wiki_save_mode("tenant_1", "kb_1", "topic"))

    assert [call[0] for call in conn.calls].count("ensure_columns") == 1
    assert [call[0] for call in conn.calls].count("insert") == 2


def test_wiki_load_mode_ensures_columns_before_reading(monkeypatch):
    """The read path upgrades the table too: otherwise it hits a missing column
    and silently reports "never built", forcing a full rebuild every run."""
    module = _load_wiki_generator(monkeypatch)
    conn = _RecordingConn()
    _patch_wiki_module(monkeypatch, module, conn)

    assert asyncio.run(module._wiki_load_mode("tenant_1", "kb_1")) is None
    assert [call[0] for call in conn.calls] == ["ensure_columns", "search"]


def test_wiki_load_mode_survives_search_failure(monkeypatch):
    """A failing read degrades to "unknown mode", it never aborts the build."""
    module = _load_wiki_generator(monkeypatch)
    conn = _RecordingConn(search_error=RuntimeError("column not found"))
    _patch_wiki_module(monkeypatch, module, conn)

    assert asyncio.run(module._wiki_load_mode("tenant_1", "kb_1")) is None


def test_wiki_ensure_mode_columns_skips_engines_without_helper(monkeypatch):
    """Engines without ``ensure_columns`` (ES/OpenSearch/...) use dynamic
    mappings, so the upgrade is a no-op rather than an error."""
    module = _load_wiki_generator(monkeypatch)
    conn = _RecordingConn(ensure=False)
    _patch_wiki_module(monkeypatch, module, conn)

    asyncio.run(module._wiki_ensure_mode_columns("kb_1", "ragflow_tenant_1"))

    assert conn.calls == []
    assert ("ragflow_tenant_1", "kb_1") in module._WIKI_MODE_COLUMNS_ENSURED

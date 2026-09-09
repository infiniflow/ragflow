"""Conftest for knowledge compile unit tests.

Stubs only modules that can't be imported due to the test-directory
namespace conflict or deep dependency chains.  For loadable modules
(e.g. common.doc_store.doc_store_base), imports the real module so
other test suites are not affected.
"""

import asyncio
import importlib
import os
import sys
import types
from unittest.mock import MagicMock

import pytest


@pytest.fixture(autouse=True)
def _mock_disabled_document_lookup(monkeypatch):
    """Keep knowledge-compile unit tests independent of the MySQL database."""
    try:
        from api.db.services.document_service import DocumentService
    except ModuleNotFoundError:
        module = types.ModuleType("api.db.services.document_service")
        module.DocumentService = type(
            "DocumentService",
            (),
            {"get_disabled_doc_ids_by_kb_id": MagicMock(return_value=set())},
        )
        monkeypatch.setitem(sys.modules, "api.db.services.document_service", module)
        return

    monkeypatch.setattr(DocumentService, "get_disabled_doc_ids_by_kb_id", MagicMock(return_value=set()))


async def _fake_thread_pool_exec(fn, *args, **kwargs):
    """Execute the function directly (no actual thread pool)."""
    if asyncio.iscoroutinefunction(fn):
        return await fn(*args, **kwargs)
    result = fn(*args, **kwargs)
    if asyncio.iscoroutine(result) or asyncio.isfuture(result):
        return await result
    return result


# ---- Safe import: load real module if possible, otherwise return stub ----
def _real_or_stub(mod_name):
    """Return the real module `mod_name` if it's loadable, else a stub."""
    if mod_name in sys.modules:
        return sys.modules[mod_name]
    try:
        return importlib.import_module(mod_name)
    except Exception:
        m = types.ModuleType(mod_name)
        sys.modules[mod_name] = m
        return m


# ---- Stub modules that can't be imported (deep dependency chains) ----
_stub_only = [
    "common.settings",
    "common.exceptions",
    "rag.nlp.search",
    "rag.llm",
    "rag.llm.chat_model",
    "rag.utils.redis_conn",
    "api.db.services.llm_service",
    "rag.prompts",
    "rag.prompts.generator",
]
for name in _stub_only:
    if name not in sys.modules:
        sys.modules[name] = types.ModuleType(name)

# dataset_nav imports RedisDistributedLock at module level but only exercises it
# on the build/upsert paths, so a no-op class is enough to import the search half.
if not hasattr(sys.modules["rag.utils.redis_conn"], "RedisDistributedLock"):

    class _NoOpLock:
        def __init__(self, *args, **kwargs):
            pass

        async def spin_acquire(self):
            return True

        async def __aenter__(self):
            return self

        async def __aexit__(self, *exc):
            return False

        async def release(self):
            return True

    sys.modules["rag.utils.redis_conn"].RedisDistributedLock = _NoOpLock

# message_fit_in is imported by wiki_incremental at module level
if not hasattr(sys.modules["rag.prompts.generator"], "message_fit_in"):

    def _message_fit_in(*args, **kwargs):
        return True

    sys.modules["rag.prompts.generator"].message_fit_in = _message_fit_in
if not hasattr(sys.modules["rag.prompts.generator"], "gen_json"):
    sys.modules["rag.prompts.generator"].gen_json = MagicMock(return_value={})

# ---- Modules that wiki_incremental.py imports at module level — use
#      real import when possible to avoid polluting other test suites.
# ---------------------------------------------------------------------
# Load real common.doc_store.doc_store_base (needed for OrderByExpr /
# MatchDenseExpr at wiki_incremental module level).  If import fails,
# create a minimal class-based stub instead of using MagicMock.
try:
    import common.doc_store.doc_store_base  # noqa: F401
except Exception:
    stub = types.ModuleType("common.doc_store.doc_store_base")
    stub.OrderByExpr = type("OrderByExpr", (), {})
    stub.MatchDenseExpr = type("MatchDenseExpr", (), {})
    sys.modules["common.doc_store.doc_store_base"] = stub

try:
    import common.connection_utils  # noqa: F401
except Exception:
    if "common.connection_utils" not in sys.modules:
        sys.modules["common.connection_utils"] = types.ModuleType("common.connection_utils")

try:
    import common.misc_utils  # noqa: F401
except Exception:
    if "common.misc_utils" not in sys.modules:
        sys.modules["common.misc_utils"] = types.ModuleType("common.misc_utils")

try:
    import api.db.services.task_service  # noqa: F401
except Exception:
    if "api.db.services.task_service" not in sys.modules:
        sys.modules["api.db.services.task_service"] = types.ModuleType("api.db.services.task_service")

# ---- Wire up attributes on whatever module won (real or stub) ----
sys.modules["common.misc_utils"].thread_pool_exec = _fake_thread_pool_exec
sys.modules["rag.nlp.search"].index_name = MagicMock(return_value="test_index")
sys.modules["common.settings"].docStoreConn = MagicMock()
sys.modules["common.connection_utils"].timeout = lambda *a, **kw: lambda fn: fn
sys.modules["api.db.services.task_service"].has_canceled = lambda *a, **kw: False

# ---- Stubs that MUST exist for wiki_incremental.py import ----
for mod_name in [
    "rag",
    "rag.nlp",
    "rag.utils",
    "api",
    "api.db",
    "api.db.services",
    "rag.advanced_rag",
    "rag.advanced_rag.knowlege_compile",
    "rag.advanced_rag.knowlege_compile.structure",
    "rag.advanced_rag.knowlege_compile._common",
]:
    if mod_name not in sys.modules:
        sys.modules[mod_name] = types.ModuleType(mod_name)

# wiki_incremental.py uses relative imports (from ._common import ...), so
# rag.advanced_rag.knowlege_compile MUST be a proper package with __path__
# pointing at the real source directory, otherwise those imports fail.
_KC_DIR = os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "../../../../../rag/advanced_rag/knowlege_compile"))
sys.modules["rag.advanced_rag.knowlege_compile"].__path__ = [_KC_DIR]
if hasattr(sys.modules["rag.advanced_rag.knowlege_compile"], "__package__"):
    sys.modules["rag.advanced_rag.knowlege_compile"].__package__ = "rag.advanced_rag.knowlege_compile"

# _common.py symbols used by wiki_incremental at import time
_common_mod = sys.modules["rag.advanced_rag.knowlege_compile._common"]
_common_mod.build_chunk_batches = lambda *a, **k: ([], {})
_common_mod.bulk_dedup_items = lambda items, *a, **k: items
_common_mod.ensure_llm_bundle = lambda model: model
_common_mod.knowledge_compile_gen_conf = lambda *a, **k: {}
_common_mod.run_chunked_pipeline = MagicMock(return_value={})
_common_mod.stable_row_id = lambda *a, **k: ""


async def _encode(embd_mdl, texts: list):
    """Mirror of ``_common.encode``: run the model in the pool, drop the token
    count. Bound late so the conftest's ``thread_pool_exec`` stub is honoured."""
    if not texts:
        return []
    embeddings, _ = await _fake_thread_pool_exec(embd_mdl.encode, texts)
    return list(embeddings)


_common_mod.encode = _encode

# ---- Test helper constants (same values as structure.py) ----
sys.modules["rag.advanced_rag.knowlege_compile.structure"].CONCEPT_MIN_CLAIMS = 3
sys.modules["rag.advanced_rag.knowlege_compile.structure"].CONCEPT_MIN_SOURCES = 2
sys.modules["rag.advanced_rag.knowlege_compile.structure"]._struct_get = lambda *a, **k: None
sys.modules["rag.advanced_rag.knowlege_compile.structure"]._struct_localize = lambda value, *a, **k: value

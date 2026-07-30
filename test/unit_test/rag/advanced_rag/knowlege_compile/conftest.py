"""Conftest for knowledge compile unit tests.

Stubs all heavy dependencies. Defines test constants directly instead of
importing structure.py (which has deep dependency chains).
All tests import the target module via importlib.
"""

import asyncio
import sys
import types
from unittest.mock import MagicMock


async def _fake_thread_pool_exec(fn, *args, **kwargs):
    """Execute the function directly (no actual thread pool)."""
    if asyncio.iscoroutinefunction(fn):
        return await fn(*args, **kwargs)
    result = fn(*args, **kwargs)
    if asyncio.iscoroutine(result) or asyncio.isfuture(result):
        return await result
    return result


# ---- Pre-stub all heavy modules that the target module imports ----
_modules_to_mock = [
    "common",
    "common.settings",
    "common.connection_utils",
    "common.doc_store",
    "common.doc_store.doc_store_base",
    "common.doc_store.elastic_doc_store",
    "common.misc_utils",
    "common.exceptions",
    "api",
    "api.db",
    "api.db.services",
    "api.db.services.task_service",
    "api.db.services.llm_service",
    "rag",
    "rag.nlp",
    "rag.nlp.search",
    "rag.llm",
    "rag.llm.chat_model",
    "rag.utils",
    "rag.utils.redis_conn",
    "rag.advanced_rag",
    "rag.advanced_rag.knowlege_compile",
    "rag.advanced_rag.knowlege_compile.structure",
]

for mod_name in _modules_to_mock:
    if mod_name not in sys.modules:
        m = types.ModuleType(mod_name)
        sys.modules[mod_name] = m

# Make thread_pool_exec actually execute the function
sys.modules["common.misc_utils"].thread_pool_exec = _fake_thread_pool_exec
sys.modules["rag.nlp.search"].index_name = MagicMock(return_value="test_index")
sys.modules["common.settings"].docStoreConn = MagicMock()
sys.modules["common.connection_utils"].timeout = lambda *a, **kw: lambda fn: fn
sys.modules["api.db.services.task_service"].has_canceled = lambda *a, **kw: False

# Stub MatchDenseExpr and OrderByExpr (used by _knn_search_canonical)
sys.modules["common.doc_store.doc_store_base"].MatchDenseExpr = MagicMock()
sys.modules["common.doc_store.doc_store_base"].OrderByExpr = MagicMock()

# Define test constants (same values as structure.py)
sys.modules["rag.advanced_rag.knowlege_compile.structure"].CONCEPT_MIN_CLAIMS = 3
sys.modules["rag.advanced_rag.knowlege_compile.structure"].CONCEPT_MIN_SOURCES = 2

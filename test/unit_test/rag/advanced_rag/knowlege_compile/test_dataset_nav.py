"""Tests for the incremental-AHC dataset_nav compiler.

Follows the same synthetic-package loading pattern as
``test_raptor_psi_tree_builder.py`` — creates a fake
``rag.advanced_rag.knowlege_compile`` package in ``sys.modules`` *before*
the source file is loaded, so the deep import chain (spacy, DB
connection, config reads) is never triggered.
"""

from __future__ import annotations

import importlib
import importlib.util
import json
import os
import sys
import types
from unittest.mock import AsyncMock, MagicMock, patch

import numpy as np
import pytest

# ---------------------------------------------------------------------------
# Synthetic module loading
# ---------------------------------------------------------------------------

_test_dir = os.path.dirname(os.path.abspath(__file__))
_src = os.path.normpath(os.path.join(_test_dir, "../../../../../rag/advanced_rag/knowlege_compile/dataset_nav.py"))


@pytest.fixture(scope="module")
def nav():
    """Load ``dataset_nav.py`` as a synthetic module with all deep deps mocked.

    Returns the module object.  Tests access functions via ``nav._cosine_sim``,
    ``nav.upsert_dataset_nav_doc`` etc.
    """
    # ── 1. Fake parent packages ──────────────────────────────────────
    _rag = types.ModuleType("rag")
    _rag.__package__ = "rag"
    sys.modules["rag"] = _rag

    _rag_ar = types.ModuleType("rag.advanced_rag")
    _rag_ar.__package__ = "rag.advanced_rag"
    sys.modules["rag.advanced_rag"] = _rag_ar

    _pkg = types.ModuleType("rag.advanced_rag.knowlege_compile")
    _pkg.__package__ = "rag.advanced_rag.knowlege_compile"
    _pkg.__path__ = [os.path.dirname(_src)]
    _pkg.__file__ = os.path.join(os.path.dirname(_src), "__init__.py")

    # ── 2. Fake _common with a deterministic encode stub ──────────────
    _common = types.ModuleType("rag.advanced_rag.knowlege_compile._common")
    _common.__package__ = "rag.advanced_rag.knowlege_compile"

    async def _encode(embd_mdl, texts):
        rng = np.random.RandomState(42)
        dim = 8
        return [rng.randn(dim).tolist() for _ in texts]

    _common.encode = _encode
    _common.stable_row_id = lambda *parts: str(hash(":".join(str(p or "") for p in parts)))

    sys.modules["rag.advanced_rag.knowlege_compile"] = _pkg
    sys.modules["rag.advanced_rag.knowlege_compile._common"] = _common

    # ── 3. Other deps that dataset_nav imports directly ──────────────
    _rag_nlp = types.ModuleType("rag.nlp")
    _rag_nlp.rag_tokenizer = _rag_tokenizer = MagicMock()
    _rag_nlp.rag_tokenizer.tokenize = lambda s: s
    _rag_nlp.rag_tokenizer.fine_grained_tokenize = lambda s: s
    _rag_nlp.search = MagicMock()
    _rag_nlp.search.index_name = MagicMock(return_value="test_idx")
    sys.modules["rag.nlp"] = _rag_nlp
    sys.modules["rag.nlp.rag_tokenizer"] = _rag_nlp.rag_tokenizer
    sys.modules["rag.nlp.search"] = _rag_nlp.search

    sys.modules["rag.prompts"] = types.ModuleType("rag.prompts")

    async def _fake_gen_json(system, user_prompt, chat_mdl, **kwargs):
        return {"merged": user_prompt, "result": user_prompt, "description": "mock summary"}

    sys.modules["rag.prompts"].generator = types.ModuleType("rag.prompts.generator")
    sys.modules["rag.prompts"].generator.gen_json = _fake_gen_json
    sys.modules["rag.prompts.generator"] = sys.modules["rag.prompts"].generator

    sys.modules["rag.utils"] = types.ModuleType("rag.utils")
    sys.modules["rag.utils.redis_conn"] = MagicMock()

    _common_misc = types.ModuleType("common.misc_utils")

    async def _thread_pool_exec(fn, *args, **kwargs):
        import asyncio

        if asyncio.iscoroutinefunction(fn):
            return await fn(*args, **kwargs)
        return fn(*args, **kwargs)

    _common_misc.thread_pool_exec = _thread_pool_exec
    sys.modules["common"] = types.ModuleType("common")
    sys.modules["common"].settings = MagicMock()
    sys.modules["common.misc_utils"] = _common_misc
    sys.modules["common.doc_store"] = MagicMock()

    _dsb = types.ModuleType("common.doc_store.doc_store_base")
    _dsb.OrderByExpr = MagicMock()
    sys.modules["common.doc_store.doc_store_base"] = _dsb

    # token utilities
    _tk = types.ModuleType("common.token_utils")
    _tk.num_tokens_from_string = lambda s: len(s.split())
    sys.modules["common.token_utils"] = _tk

    # ── 4. Load the actual source ────────────────────────────────────
    spec = importlib.util.spec_from_file_location(
        "rag.advanced_rag.knowlege_compile.dataset_nav",
        _src,
    )
    module = importlib.util.module_from_spec(spec)
    # Wire the parent package so relative imports resolve
    module.__package__ = "rag.advanced_rag.knowlege_compile"
    sys.modules["rag.advanced_rag.knowlege_compile.dataset_nav"] = module
    spec.loader.exec_module(module)
    return module


# ---------------------------------------------------------------------------
# Fixtures — build doc-store mocks against the synthetic module's helpers
# ---------------------------------------------------------------------------


@pytest.fixture
def store():
    """Return an in-memory dict that simulates the doc store."""
    return {}


@pytest.fixture
def mock_es(nav, store):
    """Set up an in-memory doc store mock on ``common.settings``.

    All operations run against *store* directly.
    """

    async def _get(row_id, _index, _kb_ids):
        return store.get(row_id)

    async def _insert(rows, _index, _kb_ids):
        for r in rows:
            store[r["id"]] = r

    async def _update(query, updates, _index, _kb_ids):
        rid = query.get("id", "")
        if rid in store:
            store[rid].update(updates)

    async def _delete(query, _index, _kb_ids):
        ids = query.get("id", [])
        if isinstance(ids, str):
            ids = [ids]
        for rid in ids:
            store.pop(rid, None)

    async def _search(fields, _, condition, __, ___, offset, limit, _index, _kb_ids, **extra):
        rows = []
        for row in store.values():
            ok = True
            for k, vals in condition.items():
                if k not in row:
                    ok = False
                    break
                rv = row.get(k)
                if isinstance(vals, list):
                    if rv not in vals:
                        ok = False
                        break
                elif rv != vals:
                    ok = False
                    break
            if ok:
                rows.append(row)
        return {"total": len(rows), "ids": [r["id"] for r in rows[offset : offset + limit]]}

    def _get_fields(res, fields):
        ids = res.get("ids", [])
        return {rid: store.get(rid, {}) for rid in ids}

    conn = MagicMock()
    conn.get = AsyncMock(side_effect=_get)
    conn.insert = AsyncMock(side_effect=_insert)
    conn.update = AsyncMock(side_effect=_update)
    conn.delete = AsyncMock(side_effect=_delete)
    conn.search = AsyncMock(side_effect=_search)
    conn.get_fields = MagicMock(side_effect=_get_fields)

    # Set common.settings.docStoreConn directly on the synthetic module
    _saved = getattr(sys.modules["common"], "settings", None)
    sys.modules["common"].settings = MagicMock()
    sys.modules["common"].settings.docStoreConn = conn
    yield store
    if _saved is not None:
        sys.modules["common"].settings = _saved
    else:
        del sys.modules["common"].settings


@pytest.fixture
def mock_redis(nav):
    lock = MagicMock()
    lock.spin_acquire = AsyncMock()
    lock.release = MagicMock()
    with patch.object(nav, "RedisDistributedLock", return_value=lock):
        yield lock


@pytest.fixture
def mock_embd():
    return MagicMock()


@pytest.fixture
def mock_chat():
    return MagicMock()


# ============================================================================
# Pure-function tests (no doc-store interactions)
# ============================================================================


class TestCosineSim:
    def test_identical(self, nav):
        v = [1.0, 0.0, 0.0]
        assert nav._cosine_sim(v, v) == pytest.approx(1.0)

    def test_orthogonal(self, nav):
        assert nav._cosine_sim([1.0, 0.0], [0.0, 1.0]) == pytest.approx(0.0)

    def test_opposite(self, nav):
        assert nav._cosine_sim([1.0, 0.0], [-1.0, 0.0]) == pytest.approx(-1.0)

    def test_zero_vector(self, nav):
        assert nav._cosine_sim([0.0, 0.0], [1.0, 0.0]) == 0.0

    def test_mismatched_length(self, nav):
        assert nav._cosine_sim([1.0, 0.0], [1.0]) == 0.0


class TestExtractRootSummary:
    def test_from_title(self, nav):
        tree = {"title": "Root"}
        assert nav._extract_root_summary_from_tree(tree) == "Root"

    def test_fallback_summary(self, nav):
        tree = {"summary": "Fallback"}
        assert nav._extract_root_summary_from_tree(tree) == "Fallback"

    def test_fallback_content(self, nav):
        tree = {"content": "Content"}
        assert nav._extract_root_summary_from_tree(tree) == "Content"

    def test_prefers_title(self, nav):
        tree = {"title": "Title", "summary": "Summary"}
        assert nav._extract_root_summary_from_tree(tree) == "Title"

    def test_none(self, nav):
        assert nav._extract_root_summary_from_tree(None) == ""

    def test_empty_dict(self, nav):
        assert nav._extract_root_summary_from_tree({}) == ""


class TestMakeRows:
    def test_nav_doc_shape(self, nav):
        row = nav._make_nav_doc_row(
            "kb1",
            "doc1",
            "Summary",
            parent_kwd="pc",
            depth_int=2,
        )
        assert row["type_kwd"] == "nav_doc"
        assert row["doc_id"] == "doc1"
        assert row["parent_kwd"] == "pc"
        assert row["depth_int"] == 2
        assert row["available_int"] == 0
        payload = json.loads(row["content_with_weight"])
        assert payload["description"] == "Summary"

    def test_nav_cluster_shape(self, nav):
        row = nav._make_nav_cluster_row(
            "kb1",
            "c1",
            "Desc",
            parent_kwd="root",
            depth_int=0,
            doc_ids=["d1", "d2"],
            embedding=[0.1, 0.2, 0.3],
        )
        assert row["type_kwd"] == "nav_cluster"
        assert row["name"] == "c1"
        assert row["parent_kwd"] == "root"
        assert row["depth_int"] == 0
        assert row["doc_ids_kwd"] == ["d1", "d2"]
        assert row["doc_count_int"] == 2
        assert "q_3_vec" in row

    def ids_stable(self, nav):
        assert nav._nav_doc_id("x") == nav._nav_doc_id("x")
        assert nav._nav_cluster_id("kb", "c") == nav._nav_cluster_id("kb", "c")


# ============================================================================
# Integration-style tests (with doc store)
# ============================================================================


class TestFindBestCluster:
    async def test_no_root(self, nav, mock_es, mock_redis):
        name, parent, sim = await nav._find_best_cluster("t1", "kb1", [1.0, 0.0], 2)
        assert name is None
        assert sim == 0.0

    async def test_descends_into_child(self, nav, mock_es, mock_redis):
        s = mock_es
        root = nav._make_nav_cluster_row(
            "kb1",
            "root_c",
            "Root",
            "root",
            0,
            ["d1"],
        )
        root["q_8_vec"] = [1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0]
        s[root["id"]] = root
        child = nav._make_nav_cluster_row(
            "kb1",
            "child_c",
            "Child",
            "root_c",
            1,
            ["d1"],
        )
        child["q_8_vec"] = [0.9, 0.1, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0]
        s[child["id"]] = child

        name, parent, sim = await nav._find_best_cluster(
            "t1",
            "kb1",
            [0.85, 0.15, 0, 0, 0, 0, 0, 0],
            8,
        )
        assert name == "child_c"


class TestUpsert:
    async def test_first_doc(self, nav, mock_es, mock_redis, mock_embd, mock_chat):
        await nav.upsert_dataset_nav_doc(
            "t1",
            "kb1",
            "doc1",
            "ML basics",
            embd_mdl=mock_embd,
            chat_mdl=mock_chat,
        )
        s = mock_es
        docs = [v for v in s.values() if v.get("type_kwd") == "nav_doc"]
        clusters = [v for v in s.values() if v.get("type_kwd") == "nav_cluster"]
        assert len(docs) == 1
        assert len(clusters) == 1
        assert clusters[0]["depth_int"] == 0
        assert docs[0]["parent_kwd"] == clusters[0]["name"]

    async def test_similar_merges(self, nav, mock_es, mock_redis, mock_embd, mock_chat):
        await nav.upsert_dataset_nav_doc(
            "t1",
            "kb1",
            "doc1",
            "Deep learning",
            embd_mdl=mock_embd,
            chat_mdl=mock_chat,
        )
        await nav.upsert_dataset_nav_doc(
            "t1",
            "kb1",
            "doc2",
            "Deep learning concepts",
            embd_mdl=mock_embd,
            chat_mdl=mock_chat,
        )
        s = mock_es
        clusters = [v for v in s.values() if v.get("type_kwd") == "nav_cluster"]
        assert len(clusters) == 1
        assert clusters[0]["doc_count_int"] == 2

    async def test_different_creates_new(self, nav, mock_es, mock_redis, mock_embd, mock_chat):
        await nav.upsert_dataset_nav_doc(
            "t1",
            "kb1",
            "doc1",
            "Deep learning",
            embd_mdl=mock_embd,
            chat_mdl=mock_chat,
        )
        await nav.upsert_dataset_nav_doc(
            "t1",
            "kb1",
            "doc2",
            "Quantum physics",
            embd_mdl=mock_embd,
            chat_mdl=mock_chat,
        )
        s = mock_es
        docs = [v for v in s.values() if v.get("type_kwd") == "nav_doc"]
        assert len(docs) == 2

    async def test_skip_unchanged(self, nav, mock_es, mock_redis, mock_embd, mock_chat):
        await nav.upsert_dataset_nav_doc(
            "t1",
            "kb1",
            "doc1",
            "Same content",
            embd_mdl=mock_embd,
            chat_mdl=mock_chat,
        )
        before = len(mock_es)
        await nav.upsert_dataset_nav_doc(
            "t1",
            "kb1",
            "doc1",
            "Same content",
            embd_mdl=mock_embd,
            chat_mdl=mock_chat,
        )
        assert len(mock_es) == before

    async def test_empty_summary_skipped(self, nav, mock_es, mock_redis, mock_embd, mock_chat):
        before = len(mock_es)
        await nav.upsert_dataset_nav_doc(
            "t1",
            "kb1",
            "doc1",
            "",
            embd_mdl=mock_embd,
            chat_mdl=mock_chat,
        )
        assert len(mock_es) == before

    async def test_accepts_tree_dict(self, nav, mock_es, mock_redis, mock_embd, mock_chat):
        tree = {"title": "Tree root"}
        await nav.upsert_dataset_nav_doc(
            "t1",
            "kb1",
            "doc1",
            tree,
            embd_mdl=mock_embd,
            chat_mdl=mock_chat,
        )
        s = mock_es
        docs = [v for v in s.values() if v.get("type_kwd") == "nav_doc"]
        assert len(docs) == 1
        payload = json.loads(docs[0]["content_with_weight"])
        assert "Tree root" in payload["description"]


class TestRemove:
    async def test_remove_existing(self, nav, mock_es, mock_redis, mock_embd, mock_chat):
        await nav.upsert_dataset_nav_doc(
            "t1",
            "kb1",
            "doc1",
            "Content",
            embd_mdl=mock_embd,
            chat_mdl=mock_chat,
        )
        await nav.remove_dataset_nav_doc("t1", "kb1", "doc1")
        docs = [v for v in mock_es.values() if v.get("type_kwd") == "nav_doc"]
        assert len(docs) == 0

    async def test_remove_non_existent_noop(self, nav, mock_es, mock_redis):
        before = len(mock_es)
        await nav.remove_dataset_nav_doc("t1", "kb1", "nonexistent")
        assert len(mock_es) == before

    async def test_remove_last_cascades(self, nav, mock_es, mock_redis, mock_embd, mock_chat):
        await nav.upsert_dataset_nav_doc(
            "t1",
            "kb1",
            "doc1",
            "Content",
            embd_mdl=mock_embd,
            chat_mdl=mock_chat,
        )
        await nav.remove_dataset_nav_doc("t1", "kb1", "doc1")
        clusters = [v for v in mock_es.values() if v.get("type_kwd") == "nav_cluster"]
        assert len(clusters) == 0

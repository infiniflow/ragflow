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
"""
Tests for the dense-only fallback in Dealer.search() (#17041).

Both doc-engine connectors hand the full-text predicate to the dense (kNN) leg
as a pre-filter, so hybrid retrieval returns 0 chunks for questions with no
lexical overlap with any chunk — e.g. languages rag_tokenizer cannot segment,
such as Thai — regardless of vector_similarity_weight. Dealer.search() now
retries with a dense-only expression list after both lexically-gated attempts
come back empty; a dense-only list is pre-filtered by the scope conditions only
on every doc engine, which the connector-level tests below pin for ES and
Infinity (OpenSearch already pins it in test_opensearch_hybrid_search.py, and
ob_conn builds dense-only lists itself).

Everything runs with the clients mocked, so no cluster is needed.
"""

from __future__ import annotations

import asyncio
import importlib
import sys
import types
from unittest.mock import MagicMock

import pytest


def _real_infinity_or_skip():
    """Some sibling test modules install a bare ``infinity`` stub at collection
    time (e.g. test_rank_feature_scores.py); restore the real package before
    importing the Infinity connector, mirroring rag/conftest.py's
    ``common.data_source`` restore."""
    mod = sys.modules.get("infinity")
    if mod is not None and getattr(mod, "__file__", None) is None and not getattr(mod, "__path__", None):
        for key in [k for k in sys.modules if k == "infinity" or k.startswith("infinity.")]:
            del sys.modules[key]
        importlib.invalidate_caches()
    return pytest.importorskip("infinity")


def _install_module(name: str, **attrs) -> types.ModuleType:
    mod = sys.modules.get(name)
    if mod is None:
        mod = types.ModuleType(name)
        sys.modules[name] = mod
    for key, value in attrs.items():
        if not hasattr(mod, key):
            setattr(mod, key, value)
    return mod


def _install_module_stubs() -> None:
    """Make the heavy import-time dependencies resolvable.

    Prefer the real modules whenever they import cleanly — this file collects
    early (rag/nlp/ sorts before its sibling test modules), so a blanket stub
    would shadow the real ``rag.nlp`` for every later test. Stub only what
    actually fails to import in slim environments.
    """
    _install_module(
        "common.settings",
        ES={"hosts": "stub", "username": "u", "password": "p"},
        OS={},
        INFINITY={"uri": "stub"},
        DOC_ENGINE_INFINITY=False,
        DOC_ENGINE_OCEANBASE=False,
        DOC_ENGINE="elasticsearch",
        docStoreConn=None,
    )
    try:
        import rag.nlp  # noqa: F401
    except Exception:
        rag_tokenizer = MagicMock()
        rag_tokenizer.fine_grained_tokenize.return_value = ""
        nlp = _install_module(
            "rag.nlp",
            is_english=lambda *_args, **_kwargs: False,
            rag_tokenizer=rag_tokenizer,
            query=MagicMock(),
        )
        # Keep the stub importable as a package so rag.nlp.search still resolves.
        import rag

        nlp.__path__ = [str(__import__("pathlib").Path(rag.__file__).parent / "nlp")]


_install_module_stubs()

from common.constants import PAGERANK_FLD  # noqa: E402
from common.doc_store.doc_store_base import (  # noqa: E402
    FusionExpr,
    MatchDenseExpr,
    MatchTextExpr,
    OrderByExpr,
)

THAI_QUESTION = "ปัญหาแจ้งระบบภาษี"


def _text_expr(matching_text=THAI_QUESTION):
    return MatchTextExpr(
        fields=["content_ltks^2", "title_tks^4"],
        matching_text=matching_text,
        topn=10,
        extra_options={"minimum_should_match": 0.3},
    )


def _dense_expr():
    return MatchDenseExpr(
        vector_column_name="q_8_vec",
        embedding_data=[0.1] * 8,
        embedding_data_type="float",
        distance_type="cosine",
        topn=5,
        extra_options={"similarity": 0.1},
    )


def _resolve_singleton_class(module, name: str):
    """``@singleton`` wraps the class in a closure returning a cached instance;
    unwrap it so we can ``__new__`` an instance and bypass the network-dependent
    ``__init__``."""
    candidate = getattr(module, name)
    if isinstance(candidate, type):
        return candidate
    closure = getattr(candidate, "__closure__", None) or ()
    for cell in closure:
        contents = cell.cell_contents
        if isinstance(contents, type):
            return contents
    raise RuntimeError(f"Could not locate the {name} class in module scope")


class TestESDenseOnlyScopeFilter:
    """A dense-only expression list must reach ES with scope-only knn filter."""

    def _search(self, match_expressions, condition):
        pytest.importorskip("elasticsearch_dsl")
        from rag.utils import es_conn

        cls = _resolve_singleton_class(es_conn, "ESConnection")
        conn = cls.__new__(cls)
        conn.logger = MagicMock()
        captured = {}

        def _fake_search_once(index_names, q, **_kwargs):
            captured["q"] = q
            return {"timed_out": False, "hits": {"total": {"value": 0}, "hits": []}}

        conn._es_search_once = _fake_search_once
        conn.search(
            select_fields=["content_ltks"],
            highlight_fields=[],
            condition=condition,
            match_expressions=match_expressions,
            order_by=OrderByExpr(),
            offset=0,
            limit=10,
            index_names=["idx1"],
            knowledgebase_ids=["kb1"],
        )
        return captured["q"]

    def test_dense_only_knn_filter_is_scope_only(self):
        q = self._search([_dense_expr()], condition={"doc_id": ["doc1"]})
        knn = q.get("knn", [])
        knn = knn if isinstance(knn, list) else [knn]
        assert knn, "knn clause must be present"
        for clause in knn:
            knn_filter = str(clause.get("filter", {}))
            assert "query_string" not in knn_filter, "dense-only search must not carry a full-text predicate"
            assert "kb_id" in knn_filter and "doc1" in knn_filter, "dense-only search must keep the scope conditions"


class _RecordingBuilder:
    """Stands in for an Infinity table query builder; records match calls."""

    def __init__(self, record: dict, output: list[str]):
        self._record = record
        self._output = output

    def match_text(self, fields, matching_text, topn, extra_options):
        self._record["match_text"] = {"matching_text": matching_text, "extra_options": extra_options}
        return self

    def match_dense(self, vector_column_name, embedding_data, embedding_data_type, distance_type, topn, extra_options):
        self._record["match_dense"] = {"vector_column_name": vector_column_name, "extra_options": extra_options}
        return self

    def fusion(self, method, topn, fusion_params):
        self._record["fusion"] = {"method": method}
        return self

    def filter(self, cond):
        self._record["plain_filter"] = cond
        return self

    def sort(self, order_by_expr_list):
        return self

    def offset(self, offset):
        return self

    def limit(self, limit):
        return self

    def option(self, options):
        return self

    def to_df(self):
        import pandas as pd

        schema = []
        for field_name in self._output:
            if field_name == "score()":
                schema.append("SCORE")
            elif field_name == "similarity()":
                schema.append("SIMILARITY")
            elif field_name == "row_id()":
                schema.append("row_id")
            else:
                schema.append(field_name)
        return pd.DataFrame(columns=schema), {"total_hits_count": 0}


class TestInfinityDenseOnlyScopeFilter:
    """A dense-only expression list must reach Infinity with scope-only filter."""

    SCOPE_COND = "doc_id IN ('doc1')"

    def _search(self, match_expressions, condition):
        _real_infinity_or_skip()
        from rag.utils import infinity_conn

        cls = _resolve_singleton_class(infinity_conn, "InfinityConnection")
        conn = cls.__new__(cls)
        conn.logger = MagicMock()
        conn.dbName = "default_db"
        conn.equivalent_condition_to_str = lambda _condition, _table: self.SCOPE_COND if condition else None

        record: dict = {}
        table_instance = MagicMock()
        table_instance.output.side_effect = lambda output: _RecordingBuilder(record, output)
        db_instance = MagicMock()
        db_instance.get_table.return_value = table_instance
        inf_conn = MagicMock()
        inf_conn.get_database.return_value = db_instance
        conn.connPool = MagicMock()
        conn.connPool.get_conn.return_value = inf_conn

        conn.search(
            select_fields=["content_with_weight", PAGERANK_FLD],
            highlight_fields=[],
            condition=condition,
            match_expressions=match_expressions,
            order_by=OrderByExpr(),
            offset=0,
            limit=10,
            index_names=["idx1"],
            knowledgebase_ids=["kb1"],
        )
        return record

    def test_dense_only_filter_is_scope_only(self):
        record = self._search([_dense_expr()], condition={"doc_id": ["doc1"]})
        dense_options = record["match_dense"]["extra_options"]
        assert dense_options.get("filter") == self.SCOPE_COND, "dense-only search must be pre-filtered by the scope conditions"
        assert "filter_fulltext(" not in dense_options.get("filter", ""), "dense-only search must not carry a full-text predicate"

    def test_dense_only_unfiltered_without_conditions(self):
        record = self._search([_dense_expr()], condition={})
        assert "filter" not in record["match_dense"]["extra_options"], "dense-only search must not synthesize a filter without scope conditions"


class _FakeDocStore:
    """Records Dealer.search() attempts; yields hits per the scripted plan.

    ``hits_on`` maps attempt index (1-based) to the ids returned for that
    attempt; every other attempt returns no hits.
    """

    def __init__(self, hits_on: dict[int, list[str]]):
        self.hits_on = hits_on
        self.calls: list[list] = []
        self.dense_options_at_call: list[dict | None] = []

    def search(self, src, highlight_fields, filters, match_expressions, order_by, offset, limit, idx_names, kb_ids, rank_feature=None):
        self.calls.append(list(match_expressions))
        dense = next((e for e in match_expressions if isinstance(e, MatchDenseExpr)), None)
        self.dense_options_at_call.append(dict(dense.extra_options) if dense else None)
        return self.hits_on.get(len(self.calls), [])

    def get_total(self, res):
        return len(res)

    def get_doc_ids(self, res):
        return list(res)

    def get_highlight(self, res, keywords, fieldnm):
        return {}

    def get_aggregation(self, res, fieldnm):
        return []

    def get_fields(self, res, fields):
        return {chunk_id: {"doc_id": "doc1"} for chunk_id in res}


class _FakeEmbeddingModel:
    def encode_queries(self, txt):
        return [0.1] * 8, 20


def _make_dealer(datastore):
    from rag.nlp import search as search_module

    dealer = search_module.Dealer.__new__(search_module.Dealer)
    dealer.dataStore = datastore
    dealer.qryr = MagicMock()
    dealer.qryr.question.side_effect = lambda qst, min_match=0.3: (_text_expr(qst), ["ปัญหา"])
    return dealer


def _run_search(datastore, req=None):
    dealer = _make_dealer(datastore)
    return asyncio.run(
        dealer.search(
            req if req is not None else {"question": THAI_QUESTION, "topk": 5},
            "idx1",
            ["kb1"],
            emb_mdl=_FakeEmbeddingModel(),
        )
    )


class TestDealerDenseOnlyFallback:
    def test_fallback_fires_after_both_gated_attempts_are_empty(self):
        """0 hits at min_match 0.3 and 0.1 must trigger a dense-only attempt."""
        store = _FakeDocStore(hits_on={3: ["chunk-1", "chunk-2"]})
        result = _run_search(store)

        assert len(store.calls) == 3, "expected two gated attempts then the dense-only fallback"
        fallback_exprs = store.calls[2]
        assert len(fallback_exprs) == 1 and isinstance(fallback_exprs[0], MatchDenseExpr), "fallback must search the dense leg alone"
        assert result.total == 2 and result.ids == ["chunk-1", "chunk-2"]

    def test_fallback_uses_fresh_dense_expression(self):
        """The doc engine may rewrite the original expression's extra_options in
        place (Infinity attaches the full-text filter); the fallback must not
        inherit that."""
        store = _FakeDocStore(hits_on={3: ["chunk-1"]})

        original_search = store.search

        def polluting_search(src, highlight_fields, filters, match_expressions, *args, **kwargs):
            result = original_search(src, highlight_fields, filters, match_expressions, *args, **kwargs)
            for expr in match_expressions:
                if isinstance(expr, MatchDenseExpr):
                    expr.extra_options.setdefault("filter", "filter_fulltext('content', 'stale')")
            return result

        store.search = polluting_search
        _run_search(store)

        fallback_options = store.dense_options_at_call[2]
        assert "filter_fulltext(" not in str(fallback_options.get("filter", "")), "fallback must not inherit the engine-attached full-text filter"
        assert fallback_options.get("similarity") == 0.17

    def test_no_fallback_when_hybrid_search_has_hits(self):
        store = _FakeDocStore(hits_on={1: ["chunk-1"]})
        result = _run_search(store)

        assert len(store.calls) == 1, "a non-empty hybrid result must not trigger extra attempts"
        assert [type(e) for e in store.calls[0]] == [MatchTextExpr, MatchDenseExpr, FusionExpr]
        assert result.total == 1

    def test_no_fallback_when_low_min_match_retry_has_hits(self):
        store = _FakeDocStore(hits_on={2: ["chunk-1"]})
        result = _run_search(store)

        assert len(store.calls) == 2, "a non-empty low-min_match retry must not trigger the dense-only fallback"
        assert result.total == 1

    def test_doc_id_scoped_zero_result_keeps_listing_behavior(self):
        """With doc_id filters the existing fallback lists the doc's chunks
        without match expressions; the dense-only fallback must not replace it."""
        store = _FakeDocStore(hits_on={2: ["chunk-1"]})
        result = _run_search(store, req={"question": THAI_QUESTION, "topk": 5, "doc_ids": ["doc1"]})

        assert len(store.calls) == 2
        assert store.calls[1] == [], "doc_id-scoped empty result must fall back to listing, not dense-only search"
        assert result.total == 1


if __name__ == "__main__":
    raise SystemExit(pytest.main([__file__, "-v"]))

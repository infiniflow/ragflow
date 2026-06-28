#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
Tests for the query OSConnection.search() builds for hybrid search.

#10747: when a query had both a text and a vector leg, the text leg got dropped
(del q["query"]; q["query"] = {"knn": ...}) and only survived as a knn filter,
so hybrid search on OpenSearch was effectively vector-only. The Elasticsearch
backend doesn't have this problem.

These check the request body/params for each text/vector combination with the
client mocked, so no cluster is needed.
"""

from __future__ import annotations

import sys
import types
from unittest.mock import MagicMock

import pytest


# Importing OSConnection touches opensearchpy at module load, so guard for
# environments where the package isn't installed.
opensearchpy = pytest.importorskip("opensearchpy")


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
    """Replace the heavy modules opensearch_conn imports at load time.

    ``rag.utils.opensearch_conn`` imports ``common.settings`` (which pulls every
    storage backend) and ``rag.nlp`` at module load. We stub just those so the
    real ``OSConnection`` class can be imported without a live environment.
    """
    _install_module(
        "common.settings",
        OS={"hosts": "stub", "username": "u", "password": "p"},
        ES={},
        DOC_ENGINE_INFINITY=False,
        DOC_ENGINE_OCEANBASE=False,
        DOC_ENGINE="opensearch",
        docStoreConn=None,
    )
    _install_module(
        "rag.nlp",
        is_english=lambda *_args, **_kwargs: False,
        rag_tokenizer=MagicMock(),
    )


_install_module_stubs()

from common.doc_store.doc_store_base import (  # noqa: E402
    FusionExpr,
    MatchDenseExpr,
    MatchTextExpr,
)


def _resolve_os_connection_class():
    """Return the real OSConnection class.

    ``@singleton`` wraps the class in a closure that returns a cached instance
    on call, so ``opensearch_conn.OSConnection`` at module scope is a function,
    not a type. Unwrap it so we can ``__new__`` an instance directly and bypass
    the network-dependent ``__init__``.
    """
    from rag.utils import opensearch_conn

    candidate = opensearch_conn.OSConnection
    if isinstance(candidate, type):
        return candidate
    closure = getattr(candidate, "__closure__", None) or ()
    for cell in closure:
        contents = cell.cell_contents
        if isinstance(contents, type):
            return contents
    raise RuntimeError("Could not locate the OSConnection class in module scope")


def _make_os_connection(hybrid_search_enabled: bool = True):
    """Build an OSConnection without invoking its real ``__init__``."""
    cls = _resolve_os_connection_class()
    conn = cls.__new__(cls)
    conn.os = MagicMock()
    conn.os.search.return_value = {
        "hits": {"total": {"value": 0}, "hits": []},
        "timed_out": False,
    }
    conn.info = {"version": {"number": "2.18.0"}}
    conn.hybrid_search_enabled = hybrid_search_enabled
    conn._hybrid_pipeline = "ragflow_hybrid_pipeline"
    return conn


def _text_expr():
    return MatchTextExpr(fields=["content_ltks"], matching_text="what is kubernetes", topn=10, extra_options={})


def _dense_expr():
    return MatchDenseExpr(
        vector_column_name="q_1024_vec",
        embedding_data=[0.1] * 8,
        embedding_data_type="float",
        distance_type="cosine",
        topn=5,
        extra_options={"similarity": 0.0},
    )


def _fusion_expr():
    return FusionExpr(method="weighted_sum", topn=5, fusion_params={"weights": "0.5,0.5"})


def _call_search(conn, match_expressions):
    """Call search() and return (body, params) handed to the OpenSearch client."""
    conn.search(
        select_fields=["content_ltks"],
        highlight_fields=[],
        condition={},
        match_expressions=match_expressions,
        order_by=None,
        offset=0,
        limit=10,
        index_names=["idx1"],
        knowledgebase_ids=["kb1"],
    )
    call = conn.os.search.call_args
    return call.kwargs.get("body"), call.kwargs.get("params")


class TestHybridSearchDSL:
    def test_hybrid_query_structure(self):
        """text + vector must produce a {"hybrid": {"queries": [bool, {"knn": ...}]}}."""
        conn = _make_os_connection()
        body, _ = _call_search(conn, [_text_expr(), _dense_expr(), _fusion_expr()])

        assert "hybrid" in body["query"], "hybrid query not present"
        queries = body["query"]["hybrid"]["queries"]
        assert len(queries) == 2, "hybrid must have exactly two sub-queries"
        keyword_q, knn_q = queries
        assert "bool" in keyword_q, "first hybrid leg must be the keyword bool query"
        assert "knn" in knn_q, "second hybrid leg must be the knn query"

    def test_hybrid_passes_search_pipeline_param(self):
        conn = _make_os_connection()
        _, params = _call_search(conn, [_text_expr(), _dense_expr(), _fusion_expr()])

        assert params is not None, "search_pipeline params must be passed for hybrid search"
        assert params.get("search_pipeline") == "ragflow_hybrid_pipeline"

    def test_knn_only_query_structure(self):
        """vector only must stay a pure knn query with no pipeline param."""
        conn = _make_os_connection()
        body, params = _call_search(conn, [_dense_expr()])

        assert "knn" in body["query"], "knn-only search must use a knn query"
        assert "hybrid" not in body["query"], "knn-only must not be hybrid"
        assert params is None, "knn-only must not pass a search_pipeline"

    def test_text_only_query_structure(self):
        """text only must stay a bool query with no knn/hybrid and no pipeline."""
        conn = _make_os_connection()
        body, params = _call_search(conn, [_text_expr()])

        assert "knn" not in body.get("query", {}), "text-only must not use knn"
        assert "hybrid" not in body.get("query", {}), "text-only must not use hybrid"
        assert params is None, "text-only must not pass a search_pipeline"

    def test_knn_filter_excludes_text_must_clause(self):
        """The KNN pre-filter must carry only filter conditions, never the
        text query_string must-clause (the root cause of #10747)."""
        conn = _make_os_connection()
        body, _ = _call_search(conn, [_text_expr(), _dense_expr(), _fusion_expr()])

        knn_clause = body["query"]["hybrid"]["queries"][1]["knn"]
        vec_params = next(iter(knn_clause.values()))
        knn_filter = vec_params.get("filter", {})
        assert "query_string" not in str(knn_filter), "knn filter must not include the text query_string clause"

    def test_falls_back_to_knn_when_pipeline_unavailable(self):
        """When the normalization pipeline could not be provisioned (e.g. cluster
        < 2.10 or insufficient privileges), a text+vector query must degrade to a
        pure knn query rather than reference a non-existent pipeline."""
        conn = _make_os_connection(hybrid_search_enabled=False)
        body, params = _call_search(conn, [_text_expr(), _dense_expr(), _fusion_expr()])

        assert "hybrid" not in body["query"], "must not build a hybrid query without a pipeline"
        assert "knn" in body["query"], "must fall back to a pure knn query"
        assert params is None, "must not reference a search_pipeline when disabled"


if __name__ == "__main__":
    raise SystemExit(pytest.main([__file__, "-v"]))

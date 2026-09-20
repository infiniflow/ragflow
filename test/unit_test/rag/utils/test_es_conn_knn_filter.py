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

"""Regression tests for kNN filter construction in rag.utils.es_conn.

Regression tests for issue #19949:
Full-text query_string with minimum_should_match was previously passed directly
into the Elasticsearch kNN pre-filter, dropping semantically relevant chunks on
longer or expanded queries across standard vector similarity weights (such as
the default vector_similarity_weight=0.3).
"""

import sys
import types
from unittest.mock import MagicMock
import pytest


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
    _install_module("elastic_transport", ConnectionTimeout=type("ConnectionTimeout", (Exception,), {}))
    es_client_mod = _install_module("elasticsearch.client", IndicesClient=MagicMock())
    _install_module(
        "elasticsearch",
        BadRequestError=type("BadRequestError", (Exception,), {}),
        NotFoundError=type("NotFoundError", (Exception,), {}),
        Elasticsearch=MagicMock(),
        client=es_client_mod,
    )
    _install_module(
        "elasticsearch_dsl",
        Q=MagicMock(),
        Search=MagicMock(),
        UpdateByQuery=MagicMock(),
        Index=MagicMock(),
    )
    _install_module(
        "rag.nlp",
        is_english=lambda *_args, **_kwargs: False,
        rag_tokenizer=MagicMock(),
    )
    _install_module(
        "common.settings",
        ES={},
        docStoreConn=None,
    )


_install_module_stubs()

from rag.utils.es_conn import (
    _build_knn_filter_query,
    _is_query_string_clause,
    _remove_query_string_must_clauses,
)


class MockBoolQuery:
    def __init__(self, query_dict: dict):
        self._dict = query_dict

    def to_dict(self):
        return self._dict


@pytest.mark.p1
def test_is_query_string_clause():
    assert _is_query_string_clause({"query_string": {"query": "sample text"}}) is True
    assert _is_query_string_clause({"term": {"doc_id": "doc123"}}) is False
    assert _is_query_string_clause({"range": {"create_time": {"gte": 0}}}) is False
    assert _is_query_string_clause(None) is False
    assert _is_query_string_clause("query_string") is False


@pytest.mark.p1
def test_remove_query_string_must_clauses():
    clauses = [
        {"term": {"doc_id": "doc123"}},
        {"query_string": {"query": "sample search term", "minimum_should_match": "30%"}},
        {"term": {"kb_id": "kb456"}},
    ]
    filtered = _remove_query_string_must_clauses(clauses)
    assert len(filtered) == 2
    assert {"term": {"doc_id": "doc123"}} in filtered
    assert {"term": {"kb_id": "kb456"}} in filtered
    assert not any("query_string" in c for c in filtered)


@pytest.mark.p1
def test_remove_query_string_single_clause():
    assert _remove_query_string_must_clauses({"query_string": {"query": "text"}}) == []
    assert _remove_query_string_must_clauses({"term": {"doc_id": "doc123"}}) == [{"term": {"doc_id": "doc123"}}]
    assert _remove_query_string_must_clauses([]) == []
    assert _remove_query_string_must_clauses(None) == []


@pytest.mark.p1
@pytest.mark.parametrize("weight", [0.0, 0.1, 0.3, 0.5, 0.8, 0.9, 1.0, None])
def test_build_knn_filter_query_drops_query_string_across_all_weights(weight):
    """Verify that query_string is stripped from kNN filter across all weights including default 0.3."""
    bool_dict = {
        "bool": {
            "must": [
                {"term": {"doc_id": "doc123"}},
                {"query_string": {"query": "sample search text", "minimum_should_match": "30%"}},
            ],
            "filter": [{"term": {"kb_id": "kb456"}}],
        }
    }
    bool_q = MockBoolQuery(bool_dict)
    res = _build_knn_filter_query(bool_q, vector_similarity_weight=weight)

    assert res is not None
    bool_part = res.get("bool", {})
    must_list = bool_part.get("must", [])

    # query_string is stripped, term doc_id is preserved
    assert len(must_list) == 1
    assert must_list[0] == {"term": {"doc_id": "doc123"}}

    # filter clause is preserved
    assert bool_part.get("filter") == [{"term": {"kb_id": "kb456"}}]


@pytest.mark.p1
def test_build_knn_filter_query_returns_none_when_no_other_filters_exist():
    """If the only clause in bool_query was query_string, kNN filter should be None (unfiltered)."""
    bool_dict = {
        "bool": {
            "must": [
                {"query_string": {"query": "sample search text", "minimum_should_match": "30%"}},
            ]
        }
    }
    bool_q = MockBoolQuery(bool_dict)
    res = _build_knn_filter_query(bool_q, vector_similarity_weight=0.3)

    assert res is None


@pytest.mark.p1
def test_build_knn_filter_query_handles_none_input():
    assert _build_knn_filter_query(None) is None

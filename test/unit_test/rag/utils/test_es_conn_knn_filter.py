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
from elasticsearch_dsl import Q
from rag.utils.es_conn import (
    _build_knn_filter_query,
    _is_query_string_clause,
    _remove_query_string_must_clauses,
    KNN_QUERY_STRING_FILTER_WEIGHT_THRESHOLD,
)


def test_is_query_string_clause():
    assert _is_query_string_clause({"query_string": {"query": "test"}}) is True
    assert _is_query_string_clause({"term": {"kb_id": "123"}}) is False
    assert _is_query_string_clause("not_a_dict") is False


def test_remove_query_string_must_clauses():
    clauses = [
        {"query_string": {"query": "hybrid keywords", "minimum_should_match": "30%"}},
        {"term": {"kb_id": "kb_1"}},
    ]
    filtered = _remove_query_string_must_clauses(clauses)
    assert len(filtered) == 1
    assert filtered[0] == {"term": {"kb_id": "kb_1"}}


def test_build_knn_filter_query_drops_query_string_above_threshold():
    bool_q = Q("bool")
    bool_q.must.append(Q("query_string", query="long query with terms", minimum_should_match="30%"))
    bool_q.filter.append(Q("term", kb_id="kb-test"))

    res = _build_knn_filter_query(bool_q, vector_similarity_weight=0.85)
    assert res is not None
    bool_dict = res.get("bool", {})
    assert "must" not in bool_dict or len(bool_dict.get("must", [])) == 0
    assert len(bool_dict.get("filter", [])) == 1


def test_build_knn_filter_query_keeps_query_string_below_threshold():
    bool_q = Q("bool")
    bool_q.must.append(Q("query_string", query="query terms", minimum_should_match="30%"))
    bool_q.filter.append(Q("term", kb_id="kb-test"))

    res = _build_knn_filter_query(bool_q, vector_similarity_weight=0.5)
    assert res is not None
    bool_dict = res.get("bool", {})
    assert len(bool_dict.get("must"), []) == 1
    assert "query_string" in bool_dict["must"][0]

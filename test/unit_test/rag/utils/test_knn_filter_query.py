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

"""Unit tests for _build_knn_filter_query in rag/utils/es_conn.py.

These tests lock down the threshold logic that PR #20070 enables:
- weight > 0.8: query_string must clause is stripped (dense leg not gated by full-text)
- weight <= 0.8: query_string must clause is preserved (hybrid search still constrained)
"""

import sys
from unittest.mock import MagicMock

# Pre-import to avoid circular import: rag.utils.es_conn imports common.doc_store.es_conn_base
# which itself triggers a chain that loops back. Stub the heavy deps before importing.
if "common.doc_store.es_conn_base" not in sys.modules:
    sys.modules["common.doc_store.es_conn_base"] = MagicMock()

from elasticsearch_dsl import Q

from rag.utils.es_conn import KNN_QUERY_STRING_FILTER_WEIGHT_THRESHOLD, _build_knn_filter_query


def _make_bool_query_with_query_string():
    """Build a minimal Bool query carrying a query_string must clause."""
    return Q("bool", must=[Q("query_string", query="test query terms", fields=["content_ltks"])])


class TestBuildKnnFilterQuery:
    def test_threshold_constant_is_point_eight(self):
        assert KNN_QUERY_STRING_FILTER_WEIGHT_THRESHOLD == 0.8

    def test_high_weight_strips_query_string(self):
        """weight > 0.8: query_string must clause is removed.

        When the only must clause was query_string, the function returns None
        (no filter = no gating on the dense leg). This is the intended behavior
        for high-weight hybrid search: dense candidates are not filtered by
        full-text predicates.
        """
        bool_query = _make_bool_query_with_query_string()
        result = _build_knn_filter_query(bool_query, vector_similarity_weight=1.0)

        # Result is None because stripping query_string leaves an empty bool.
        # The caller uses None as "no filter" — dense leg is not gated.
        assert result is None

    def test_low_weight_preserves_query_string(self):
        """weight <= 0.8: query_string must clause is preserved in the filter."""
        bool_query = _make_bool_query_with_query_string()
        result = _build_knn_filter_query(bool_query, vector_similarity_weight=0.3)

        assert result is not None
        bool_part = result.get("bool", {})
        must_clauses = bool_part.get("must", [])
        query_string_clauses = [c for c in must_clauses if "query_string" in c]
        assert len(query_string_clauses) == 1, f"query_string should be preserved at weight=0.3, got {query_string_clauses}"

    def test_boundary_weight_preserves_query_string(self):
        """weight == 0.8 (boundary): query_string must clause is preserved."""
        bool_query = _make_bool_query_with_query_string()
        result = _build_knn_filter_query(bool_query, vector_similarity_weight=0.8)

        assert result is not None
        bool_part = result.get("bool", {})
        must_clauses = bool_part.get("must", [])
        query_string_clauses = [c for c in must_clauses if "query_string" in c]
        assert len(query_string_clauses) == 1

    def test_just_above_threshold_strips(self):
        """weight slightly above 0.8: query_string is stripped, result is None."""
        bool_query = _make_bool_query_with_query_string()
        result = _build_knn_filter_query(bool_query, vector_similarity_weight=0.81)
        assert result is None

    def test_none_input_returns_none(self):
        result = _build_knn_filter_query(None, vector_similarity_weight=1.0)
        assert result is None

    def test_does_not_mutate_original(self):
        """The function must not mutate the input bool_query object."""
        bool_query = _make_bool_query_with_query_string()
        original_dict = bool_query.to_dict()
        _build_knn_filter_query(bool_query, vector_similarity_weight=1.0)
        assert bool_query.to_dict() == original_dict

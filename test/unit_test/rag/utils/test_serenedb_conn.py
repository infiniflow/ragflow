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
Unit tests for the pure SQL-building helpers of the SereneDB connector. These
need no live database: the module-level helpers are imported directly, and
_get_filters is a method that does not touch self, so it is called unbound.
"""

from __future__ import annotations

from rag.utils.serenedb_conn import (
    SereneDBConnection,
    _escape,
    _index_relation,
    _l2_normalize,
    _norm_column,
    _strip_es_query,
)


class TestEscape:
    def test_none(self):
        assert _escape(None) == "NULL"

    def test_bool(self):
        assert _escape(True) == "true"
        assert _escape(False) == "false"

    def test_numbers(self):
        assert _escape(7) == "7"
        assert _escape(1.5) == "1.5"

    def test_string_is_quoted_and_escaped(self):
        assert _escape("a'b") == "'a''b'"

    def test_list_and_dict_serialize_to_json(self):
        assert _escape(["x", "y"]) == '\'["x", "y"]\''
        assert _escape({"a": 1}) == "'{\"a\": 1}'"


class TestStripESQuery:
    def test_boosts_and_syntax_removed_and_deduped(self):
        # weights, quotes and boolean sugar are stripped; tokens de-duplicated
        # in first-seen order.
        assert _strip_es_query('(auto^0.5) (ptr^0.4) "auto _"^0.9 auto') == "auto ptr _"

    def test_plain_query_untouched(self):
        assert _strip_es_query("alpha beta") == "alpha beta"


class TestVectorHelpers:
    def test_l2_normalize_unit_length(self):
        assert _l2_normalize([3.0, 4.0]) == [0.6, 0.8]

    def test_l2_normalize_zero_vector_passthrough(self):
        assert _l2_normalize([0.0, 0.0]) == [0.0, 0.0]

    def test_norm_and_index_names(self):
        assert _norm_column(1024) == "q_1024_vec_n"
        assert _index_relation("ragflow_t1") == "idx_ragflow_t1"


class TestGetFilters:
    # _get_filters uses only module-level state, so it can be called unbound.
    def filters(self, condition):
        return SereneDBConnection._get_filters(None, condition)

    def test_scalar_equals(self):
        assert self.filters({"doc_id": "d1"}) == ["doc_id = 'd1'"]

    def test_list_is_in(self):
        assert self.filters({"doc_id": ["a", "b"]}) == ["doc_id IN ('a', 'b')"]

    def test_array_column_uses_list_contains(self):
        got = self.filters({"tag_kwd": ["a", "b"]})
        assert got == ["(list_contains(tag_kwd, 'a') OR list_contains(tag_kwd, 'b'))"]

    def test_exists_and_must_not(self):
        assert self.filters({"exists": "img_id"}) == ["img_id IS NOT NULL"]
        assert self.filters({"must_not": {"exists": "img_id"}}) == ["img_id IS NULL"]

    def test_unknown_and_empty_are_skipped(self):
        assert self.filters({"not_a_column": "x", "doc_id": ""}) == []

    def test_available_int_keeps_the_zero_predicate(self):
        # 0 is the "disabled only" filter and is falsy, so the generic empty-value skip in
        # _get_filters would drop the predicate and widen the result instead of narrowing it.
        assert self.filters({"available_int": 0}) == ["COALESCE(available_int, 1) < 1"]
        assert self.filters({"available_int": 1}) == ["COALESCE(available_int, 1) >= 1"]

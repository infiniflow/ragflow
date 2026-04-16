#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use it except in compliance with the License.
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

"""Unit tests for OceanBase memory aggregation.

Tests the pure aggregation logic used by OBConnection.get_aggregation,
without requiring a real OceanBase instance or heavy dependencies.
"""

import pytest

from memory.utils.aggregation_utils import aggregate_by_field


class TestAggregateByField:
    """Tests for aggregate_by_field (used by get_aggregation)."""

    def test_empty_messages_returns_empty_list(self):
        assert aggregate_by_field([], "message_type_kwd") == []
        assert aggregate_by_field(None, "message_type_kwd") == []

    def test_aggregates_field_values(self):
        messages = [
            {"id": "m1", "message_type_kwd": "user", "content_ltks": "a", "message_id": "msg1", "memory_id": "mem1", "status_int": 1},
            {"id": "m2", "message_type_kwd": "assistant", "content_ltks": "b", "message_id": "msg2", "memory_id": "mem1", "status_int": 1},
            {"id": "m3", "message_type_kwd": "user", "content_ltks": "c", "message_id": "msg3", "memory_id": "mem1", "status_int": 1},
        ]
        out = aggregate_by_field(messages, "message_type_kwd")
        assert set(out) == {("user", 2), ("assistant", 1)}

    def test_single_doc_result(self):
        messages = [
            {"id": "m1", "message_type_kwd": "user", "content_ltks": "x", "message_id": "msg1", "memory_id": "mem1", "status_int": 1}
        ]
        out = aggregate_by_field(messages, "message_type_kwd")
        assert out == [("user", 1)]

    def test_pre_aggregated_value_count_rows(self):
        messages = [
            {"value": "user", "count": 2},
            {"value": "assistant", "count": 1},
        ]
        out = aggregate_by_field(messages, "message_type_kwd")
        assert set(out) == {("user", 2), ("assistant", 1)}

    @pytest.mark.p2
    def test_aggregates_list_values_and_trims_whitespace(self):
        messages = [
            {"id": "m1", "tags_kwd": [" alpha ", "beta", ""]},
            {"id": "m2", "tags_kwd": ["alpha", " beta "]},
            {"id": "m3", "tags_kwd": ["gamma", None, 1]},
        ]
        out = aggregate_by_field(messages, "tags_kwd")
        assert set(out) == {("alpha", 2), ("beta", 2), ("gamma", 1)}

    @pytest.mark.p2
    def test_ignores_non_string_and_blank_scalar_values(self):
        messages = [
            {"id": "m1", "message_type_kwd": "  "},
            {"id": "m2", "message_type_kwd": None},
            {"id": "m3", "message_type_kwd": 1},
            {"id": "m4", "message_type_kwd": "assistant"},
        ]
        out = aggregate_by_field(messages, "message_type_kwd")
        assert out == [("assistant", 1)]

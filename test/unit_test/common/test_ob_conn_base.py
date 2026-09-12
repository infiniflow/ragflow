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
Unit tests for shared OceanBase dynamic-SQL-identifier validation (V-004 follow-up).
"""

import re

import pytest

from common.doc_store.ob_conn_base import OBConnectionBase, validate_column_name


class TestValidateColumnName:
    """Test cases for the validate_column_name helper."""

    def test_valid_identifier_no_allowlist(self):
        assert validate_column_name("kb_id") == "kb_id"

    def test_non_string_rejected(self):
        with pytest.raises(ValueError):
            validate_column_name(123)

    def test_sql_injection_attempt_rejected(self):
        with pytest.raises(ValueError):
            validate_column_name("id; DROP TABLE users;--")

    def test_syntactically_valid_but_not_in_allowlist_rejected(self):
        with pytest.raises(ValueError):
            validate_column_name("not_a_real_column", {"kb_id", "doc_id"})

    def test_in_allowlist_accepted(self):
        assert validate_column_name("kb_id", {"kb_id", "doc_id"}) == "kb_id"

    def test_pattern_fallback_accepted(self):
        pattern = re.compile(r"q_(?P<vector_size>\d+)_vec")
        assert validate_column_name("q_768_vec", {"kb_id"}, pattern) == "q_768_vec"

    def test_pattern_fallback_does_not_bypass_identifier_check(self):
        pattern = re.compile(r".*")
        with pytest.raises(ValueError):
            validate_column_name("id; DROP TABLE users;--", {"kb_id"}, pattern)


def _unimplemented(self, *args, **kwargs):
    raise NotImplementedError


class _DummyOBConnection(OBConnectionBase):
    """Minimal concrete subclass so the abstract base class's _get_filters() can be exercised directly."""

    get_aggregation = _unimplemented
    get_column_definitions = _unimplemented
    get_doc_ids = _unimplemented
    get_fields = _unimplemented
    get_fulltext_columns = _unimplemented
    get_highlight = _unimplemented
    get_index_columns = _unimplemented
    get_lock_prefix = _unimplemented
    get_total = _unimplemented
    insert = _unimplemented
    search = _unimplemented
    update = _unimplemented


class TestOBConnectionBaseGetFilters:
    """Test cases for OBConnectionBase._get_filters, which underlies memory/utils/ob_conn.py."""

    def _base(self):
        return object.__new__(_DummyOBConnection)

    def test_malicious_column_key_rejected(self):
        with pytest.raises(ValueError):
            self._base()._get_filters({"id; DROP TABLE users;--": "foo"})

    def test_malicious_exists_value_rejected(self):
        with pytest.raises(ValueError):
            self._base()._get_filters({"exists": "id; DROP TABLE users;--"})

    def test_malicious_must_not_exists_value_rejected(self):
        with pytest.raises(ValueError):
            self._base()._get_filters({"must_not": {"exists": "id; DROP TABLE users;--"}})

    def test_legitimate_condition_produces_expected_filter(self):
        filters = self._base()._get_filters({"kb_id": "kb1"})
        assert filters == ["kb_id = 'kb1'"]

    def test_legitimate_exists_condition(self):
        filters = self._base()._get_filters({"exists": "forget_at"})
        assert filters == ["forget_at IS NOT NULL"]

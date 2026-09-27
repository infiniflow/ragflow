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
Unit tests for dynamic-SQL-identifier validation in memory/utils/ob_conn.py (V-004 follow-up).
"""

from unittest.mock import MagicMock

import pytest

from memory.utils.ob_conn import OBConnection


def _ob_connection_class():
    return next(cell.cell_contents for cell in OBConnection.__closure__ if isinstance(cell.cell_contents, type))


class TestConvertFieldName:
    """convert_field_name() must reject any name outside the known schema."""

    def test_malicious_field_name_rejected(self):
        with pytest.raises(ValueError):
            _ob_connection_class().convert_field_name("id; DROP TABLE users;--")

    def test_known_alias_message_type(self):
        assert _ob_connection_class().convert_field_name("message_type") == "message_type_kwd"

    def test_known_alias_status(self):
        assert _ob_connection_class().convert_field_name("status") == "status_int"

    def test_known_alias_content(self):
        assert _ob_connection_class().convert_field_name("content") == "content_ltks"
        assert _ob_connection_class().convert_field_name("content", use_tokenized_content=True) == "tokenized_content_ltks"

    def test_real_column_passthrough(self):
        assert _ob_connection_class().convert_field_name("memory_id") == "memory_id"

    def test_reserved_query_keywords_passthrough(self):
        assert _ob_connection_class().convert_field_name("exists") == "exists"
        assert _ob_connection_class().convert_field_name("must_not") == "must_not"
        assert _ob_connection_class().convert_field_name("remove") == "remove"

    def test_score_passthrough(self):
        assert _ob_connection_class().convert_field_name("_score") == "_score"

    def test_vector_column_passthrough(self):
        assert _ob_connection_class().convert_field_name("q_768_vec") == "q_768_vec"


class TestConnectionGetFilters:
    """_get_filters() is inherited from OBConnectionBase; verify it rejects malicious columns for this connector too."""

    def _connection(self):
        return object.__new__(_ob_connection_class())

    def test_malicious_column_key_rejected(self):
        with pytest.raises(ValueError):
            self._connection()._get_filters({"id; DROP TABLE users;--": "foo"})


class TestUpdateColumnValidation:
    """Regression tests: OBConnection.update() must validate dynamic identifiers before building SQL."""

    def _connection(self):
        connection = object.__new__(_ob_connection_class())
        connection._check_table_exists_cached = lambda index_name: True
        connection.client = MagicMock()
        connection.logger = MagicMock()
        return connection

    def test_malicious_new_value_key_rejected(self):
        connection = self._connection()
        with pytest.raises(ValueError):
            connection.update({}, {"id; DROP TABLE users;--": "foo"}, "test_table", "mem1")
        connection.client.perform_raw_text_sql.assert_not_called()

    def test_malicious_remove_value_rejected(self):
        connection = self._connection()
        with pytest.raises(ValueError):
            connection.update({}, {"remove": "id; DROP TABLE users;--"}, "test_table", "mem1")
        connection.client.perform_raw_text_sql.assert_not_called()

    def test_legitimate_update_succeeds(self):
        connection = self._connection()
        result = connection.update({"id": "m1"}, {"session_id": "s1"}, "test_table", "mem1")
        assert result is True
        connection.client.perform_raw_text_sql.assert_called_once()

    def test_legitimate_remove_succeeds(self):
        connection = self._connection()
        result = connection.update({"id": "m1"}, {"remove": "session_id"}, "test_table", "mem1")
        assert result is True
        connection.client.perform_raw_text_sql.assert_called_once()

    def test_remove_with_alias_resolves_column(self):
        connection = self._connection()
        connection.update({"id": "m1"}, {"remove": "content"}, "test_table", "mem1")
        sql = connection.client.perform_raw_text_sql.call_args[0][0]
        assert "content_ltks = NULL" in sql

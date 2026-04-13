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
Unit tests for OceanBase connection utility functions.
"""

from rag.utils.ob_conn import get_value_str, get_metadata_filter_expression


class TestGetValueStr:
    """Test cases for the get_value_str function."""

    def test_none_value(self):
        """Test that None is converted to NULL."""
        assert get_value_str(None) == "NULL"

    def test_integer_zero(self):
        """Test that integer 0 is correctly converted."""
        assert get_value_str(0) == "0"

    def test_float_zero(self):
        """Test that float 0.0 is correctly converted."""
        assert get_value_str(0.0) == "0.0"

    def test_positive_integer(self):
        """Test positive integer conversion."""
        assert get_value_str(42) == "42"

    def test_negative_integer(self):
        """Test negative integer conversion."""
        assert get_value_str(-42) == "-42"

    def test_positive_float(self):
        """Test positive float conversion."""
        assert get_value_str(3.14) == "3.14"

    def test_negative_float(self):
        """Test negative float conversion."""
        assert get_value_str(-3.14) == "-3.14"

    def test_boolean_true(self):
        """Test that True is converted to lowercase 'true'."""
        assert get_value_str(True) == "true"

    def test_boolean_false(self):
        """Test that False is converted to lowercase 'false'."""
        assert get_value_str(False) == "false"

    def test_empty_string(self):
        """Test that empty string is quoted correctly."""
        assert get_value_str("") == "''"

    def test_simple_string(self):
        """Test simple string is quoted."""
        assert get_value_str("hello") == "'hello'"

    def test_string_with_quotes(self):
        """Test string with single quotes is escaped."""
        result = get_value_str("O'Reilly")
        assert result == "'O\\'Reilly'" or result == "'O''Reilly'"

    def test_string_with_double_quotes(self):
        """Test string with double quotes."""
        result = get_value_str('Say "hello"')
        assert '"' in result or '\\"' in result

    def test_empty_list(self):
        """Test that empty list is converted to JSON string."""
        assert get_value_str([]) == "'[]'"

    def test_list_with_items(self):
        """Test list with items is converted to JSON string."""
        result = get_value_str([1, 2, 3])
        assert result == "'[1, 2, 3]'"

    def test_empty_dict(self):
        """Test that empty dict is converted to JSON string."""
        assert get_value_str({}) == "'{}'"

    def test_dict_with_items(self):
        """Test dict with items is converted to JSON string."""
        result = get_value_str({"key": "value"})
        assert "key" in result
        assert "value" in result
        assert result.startswith("'")
        assert result.endswith("'")

    def test_nested_structure(self):
        """Test nested list/dict structures."""
        result = get_value_str({"list": [1, 2], "nested": {"a": "b"}})
        assert result.startswith("'")
        assert result.endswith("'")

    def test_unicode_string(self):
        """Test Unicode characters in strings."""
        result = get_value_str("你好世界")
        assert "你好世界" in result
        assert result.startswith("'")
        assert result.endswith("'")

    def test_special_characters(self):
        """Test special SQL characters are escaped."""
        result = get_value_str("test\\backslash")
        assert "test" in result


class TestGetMetadataFilterExpression:
    """Test cases for the get_metadata_filter_expression function."""

    def test_simple_is_condition(self):
        """Test simple 'is' comparison."""
        filter_dict = {
            "conditions": [
                {"name": "author", "comparison_operator": "is", "value": "John"}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "JSON_EXTRACT(metadata, '$.author')" in result
        assert "= 'John'" in result

    def test_numeric_comparison_with_zero(self):
        """Test numeric comparison with zero value (regression test for bug)."""
        filter_dict = {
            "conditions": [
                {"name": "count", "comparison_operator": "=", "value": 0}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "JSON_EXTRACT(metadata, '$.count')" in result
        assert "= 0" in result
        assert "= ''" not in result  # Should not produce empty string

    def test_numeric_comparison_with_float_zero(self):
        """Test numeric comparison with 0.0."""
        filter_dict = {
            "conditions": [
                {"name": "rating", "comparison_operator": "=", "value": 0.0}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "JSON_EXTRACT(metadata, '$.rating')" in result
        assert "0.0" in result

    def test_empty_string_condition(self):
        """Test condition with empty string value."""
        filter_dict = {
            "conditions": [
                {"name": "status", "comparison_operator": "is", "value": ""}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "JSON_EXTRACT(metadata, '$.status')" in result
        assert "= ''" in result

    def test_boolean_false_condition(self):
        """Test condition with False value."""
        filter_dict = {
            "conditions": [
                {"name": "active", "comparison_operator": "is", "value": False}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "JSON_EXTRACT(metadata, '$.active')" in result
        assert "false" in result

    def test_empty_list_condition(self):
        """Test condition with empty list."""
        filter_dict = {
            "conditions": [
                {"name": "tags", "comparison_operator": "is", "value": []}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "JSON_EXTRACT(metadata, '$.tags')" in result
        assert "'[]'" in result

    def test_empty_dict_condition(self):
        """Test condition with empty dict."""
        filter_dict = {
            "conditions": [
                {"name": "metadata", "comparison_operator": "is", "value": {}}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "JSON_EXTRACT(metadata, '$.metadata')" in result
        assert "'{}'" in result

    def test_none_value_condition(self):
        """Test condition with None value."""
        filter_dict = {
            "conditions": [
                {"name": "optional", "comparison_operator": "is", "value": None}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "JSON_EXTRACT(metadata, '$.optional')" in result
        assert "NULL" in result

    def test_multiple_conditions_with_and(self):
        """Test multiple conditions with AND operator."""
        filter_dict = {
            "conditions": [
                {"name": "author", "comparison_operator": "is", "value": "John"},
                {"name": "year", "comparison_operator": ">", "value": 2020}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "JSON_EXTRACT(metadata, '$.author')" in result
        assert "JSON_EXTRACT(metadata, '$.year')" in result
        assert " and " in result.lower()

    def test_multiple_conditions_with_or(self):
        """Test multiple conditions with OR operator."""
        filter_dict = {
            "conditions": [
                {"name": "status", "comparison_operator": "is", "value": "active"},
                {"name": "status", "comparison_operator": "is", "value": "pending"}
            ],
            "logical_operator": "or"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "JSON_EXTRACT(metadata, '$.status')" in result
        assert " or " in result.lower()

    def test_greater_than_operator(self):
        """Test greater than comparison."""
        filter_dict = {
            "conditions": [
                {"name": "score", "comparison_operator": ">", "value": 90}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert ">" in result
        assert "90" in result

    def test_less_than_operator(self):
        """Test less than comparison."""
        filter_dict = {
            "conditions": [
                {"name": "age", "comparison_operator": "<", "value": 18}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "<" in result
        assert "18" in result

    def test_contains_operator(self):
        """Test contains operator."""
        filter_dict = {
            "conditions": [
                {"name": "title", "comparison_operator": "contains", "value": "Python"}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "JSON_EXTRACT(metadata, '$.title')" in result

    def test_empty_operator(self):
        """Test empty operator."""
        filter_dict = {
            "conditions": [
                {"name": "description", "comparison_operator": "empty", "value": None}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "JSON_EXTRACT(metadata, '$.description')" in result
        assert "IS NULL" in result or "= ''" in result

    def test_not_empty_operator(self):
        """Test not empty operator."""
        filter_dict = {
            "conditions": [
                {"name": "description", "comparison_operator": "not empty", "value": None}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert "JSON_EXTRACT(metadata, '$.description')" in result

    def test_parentheses_wrapping(self):
        """Test that result is wrapped in parentheses."""
        filter_dict = {
            "conditions": [
                {"name": "field", "comparison_operator": "is", "value": "value"}
            ],
            "logical_operator": "and"
        }
        result = get_metadata_filter_expression(filter_dict)
        assert result.startswith("(")
        assert result.endswith(")")


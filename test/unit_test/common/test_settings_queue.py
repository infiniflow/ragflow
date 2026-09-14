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
"""Test cases for get_svr_queue_name and get_svr_queue_names functions in common.settings."""

from common.settings import get_svr_queue_name, get_svr_queue_names


class TestGetSvrQueueName:
    """Test cases for get_svr_queue_name function."""

    def test_default_suffix(self):
        """Test that default suffix is 'common'."""

        result = get_svr_queue_name(0)
        assert result == "te.0.common"

    def test_priority_zero(self):
        """Test queue name with priority 0 (low)."""

        result = get_svr_queue_name(0)
        assert result == "te.0.common"

    def test_priority_one(self):
        """Test queue name with priority 1 (high)."""

        result = get_svr_queue_name(1)
        assert result == "te.1.common"

    def test_explicit_suffix_common(self):
        """Test with explicit 'common' suffix."""

        result = get_svr_queue_name(0, "common")
        assert result == "te.0.common"

    def test_suffix_parameter_ignored(self):
        """Test that suffix parameter is currently ignored (hardcoded to 'common').

        Note: The function signature accepts a suffix parameter but currently
        hardcodes 'common' in the return value. This test documents this behavior.
        """

        # Even with different suffix values, result should be the same
        result_default = get_svr_queue_name(0, "common")
        result_resume = get_svr_queue_name(0, "resume")
        result_graphrag = get_svr_queue_name(0, "graphrag")

        # All should return the same value since suffix is hardcoded
        assert result_default == result_resume == result_graphrag == "te.0.common"

    def test_format_structure(self):
        """Test that queue name follows expected format: {SVR_QUEUE_NAME}.{priority}.common."""

        for priority in [0, 1]:
            result = get_svr_queue_name(priority)
            parts = result.split(".")
            assert len(parts) == 3
            assert parts[0] == "te"  # SVR_QUEUE_NAME
            assert parts[1] == str(priority)
            assert parts[2] == "common"

    def test_different_priorities_produce_different_results(self):
        """Test that different priorities produce different queue names."""

        result_0 = get_svr_queue_name(0)
        result_1 = get_svr_queue_name(1)

        assert result_0 != result_1
        assert result_0 == "te.0.common"
        assert result_1 == "te.1.common"

    def test_with_various_priority_values(self):
        """Test with various priority values beyond 0 and 1."""

        # Test with other priority values to ensure format is correct
        for priority in [2, 5, 10, 100]:
            result = get_svr_queue_name(priority)
            expected = f"te.{priority}.common"
            assert result == expected

    def test_returns_string_type(self):
        """Test that function returns a string."""

        result = get_svr_queue_name(0)
        assert isinstance(result, str)

    def test_no_whitespace_issues(self):
        """Test that queue name has no unexpected whitespace."""

        for priority in [0, 1]:
            result = get_svr_queue_name(priority)
            assert " " not in result
            assert "\t" not in result
            assert "\n" not in result


class TestGetSvrQueueNames:
    """Test cases for get_svr_queue_names function."""

    def test_returns_list(self):
        """Test that function returns a list."""

        result = get_svr_queue_names("common")
        assert isinstance(result, list)

    def test_returns_two_queues(self):
        """Test that function returns exactly two queue names."""

        result = get_svr_queue_names("common")
        assert len(result) == 2

    def test_sorted_high_to_low(self):
        """Test that queue names are sorted from high priority to low priority."""

        result = get_svr_queue_names("common")
        assert result[0] == "te.1.common"  # High priority first
        assert result[1] == "te.0.common"  # Low priority second

    def test_expected_values(self):
        """Test that returned values match expected queue names."""

        result = get_svr_queue_names("common")
        expected = ["te.1.common", "te.0.common"]
        assert result == expected

    def test_suffix_parameter_passed_through(self):
        """Test that suffix parameter is passed to get_svr_queue_name.

        Note: Since get_svr_queue_name currently hardcodes 'common' as the suffix,
        different suffix values will still produce the same result.
        """

        # All suffixes should produce same result due to hardcoded suffix in get_svr_queue_name
        result_common = get_svr_queue_names("common")
        result_resume = get_svr_queue_names("resume")
        result_graphrag = get_svr_queue_names("graphrag")

        expected = ["te.1.common", "te.0.common"]
        assert result_common == expected
        assert result_resume == expected  # suffix is currently ignored
        assert result_graphrag == expected  # suffix is currently ignored

    def test_all_elements_are_strings(self):
        """Test that all elements in the returned list are strings."""

        result = get_svr_queue_names("common")
        for item in result:
            assert isinstance(item, str)

    def test_consistent_results(self):
        """Test that multiple calls return consistent results."""

        result1 = get_svr_queue_names("common")
        result2 = get_svr_queue_names("common")
        result3 = get_svr_queue_names("common")

        assert result1 == result2 == result3

    def test_with_empty_suffix(self):
        """Test with empty string suffix."""

        result = get_svr_queue_names("")
        # Should still work since suffix is ignored
        assert result == ["te.1.common", "te.0.common"]


class TestGetSvrQueueNameWithMockedConstant:
    """Test cases with mocked SVR_QUEUE_NAME constant."""

    def test_with_custom_queue_name(self):
        """Test with a custom SVR_QUEUE_NAME constant."""
        # Need to patch where the constant is imported in settings module
        import common.settings as settings_mod

        original_value = settings_mod.SVR_QUEUE_NAME
        try:
            settings_mod.SVR_QUEUE_NAME = "custom_queue"
            result = settings_mod.get_svr_queue_name(0)
            assert result == "custom_queue.0.common"

            result = settings_mod.get_svr_queue_name(1)
            assert result == "custom_queue.1.common"
        finally:
            settings_mod.SVR_QUEUE_NAME = original_value

    def test_with_custom_queue_names(self):
        """Test get_svr_queue_names with a custom SVR_QUEUE_NAME constant."""
        import common.settings as settings_mod

        original_value = settings_mod.SVR_QUEUE_NAME
        try:
            settings_mod.SVR_QUEUE_NAME = "custom_queue"
            result = settings_mod.get_svr_queue_names("common")
            assert result == ["custom_queue.1.common", "custom_queue.0.common"]
        finally:
            settings_mod.SVR_QUEUE_NAME = original_value

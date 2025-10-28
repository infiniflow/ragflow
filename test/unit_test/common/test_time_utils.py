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

import time
import datetime
import pytest
from common.time_utils import current_timestamp, timestamp_to_date, date_string_to_timestamp, datetime_format, str2date, \
    elapsed2time, delta_seconds


class TestCurrentTimestamp:
    """Test cases for current_timestamp function"""

    def test_returns_integer(self):
        """Test that function returns an integer"""
        result = current_timestamp()
        assert isinstance(result, int)

    def test_returns_13_digits(self):
        """Test that returned timestamp has 13 digits (milliseconds)"""
        result = current_timestamp()
        assert len(str(result)) == 13

    def test_approximately_correct_value(self):
        """Test that returned value is approximately correct compared to current time"""
        # Get timestamps before and after function call for comparison
        before = int(time.time() * 1000)
        result = current_timestamp()
        after = int(time.time() * 1000)

        assert before <= result <= after

    def test_consistent_with_time_module(self):
        """Test that result matches time.time() * 1000 calculation"""
        expected = int(time.time() * 1000)
        result = current_timestamp()

        # Allow small difference due to execution time (typically 1-2ms)
        assert abs(result - expected) <= 10

    def test_multiple_calls_increase(self):
        """Test that multiple calls return increasing timestamps"""
        results = [current_timestamp() for _ in range(5)]

        # Check if timestamps are monotonically increasing
        # (allow equal values as they might be in the same millisecond)
        for i in range(1, len(results)):
            assert results[i] >= results[i - 1]


class TestTimestampToDate:
    """Test cases for timestamp_to_date function"""

    def test_basic_timestamp_conversion(self):
        """Test basic timestamp to date conversion with default format"""
        # Test with a specific timestamp
        timestamp = 1704067200000  # 2024-01-01 00:00:00 UTC
        result = timestamp_to_date(timestamp)
        expected = "2024-01-01 08:00:00"
        assert result == expected

    def test_custom_format_string(self):
        """Test conversion with custom format string"""
        timestamp = 1704067200000  # 2024-01-01 00:00:00 UTC

        # Test different format strings
        result1 = timestamp_to_date(timestamp, "%Y-%m-%d")
        assert result1 == "2024-01-01"

        result2 = timestamp_to_date(timestamp, "%H:%M:%S")
        assert result2 == "08:00:00"

        result3 = timestamp_to_date(timestamp, "%Y/%m/%d %H:%M")
        assert result3 == "2024/01/01 08:00"

    def test_zero_timestamp(self):
        """Test conversion with zero timestamp (epoch)"""
        timestamp = 0  # 1970-01-01 00:00:00 UTC
        result = timestamp_to_date(timestamp)
        # Note: Actual result depends on local timezone
        assert isinstance(result, str)
        assert len(result) > 0

    def test_negative_timestamp(self):
        """Test conversion with negative timestamp (pre-epoch)"""
        timestamp = -1000000  # Some time before 1970
        result = timestamp_to_date(timestamp)
        assert isinstance(result, str)
        assert len(result) > 0

    def test_string_timestamp_input(self):
        """Test that string timestamp input is handled correctly"""
        timestamp_str = "1704067200000"
        result = timestamp_to_date(timestamp_str)
        expected = "2024-01-01 08:00:00"
        assert result == expected

    def test_float_timestamp_input(self):
        """Test that float timestamp input is handled correctly"""
        timestamp_float = 1704067200000.0
        result = timestamp_to_date(timestamp_float)
        expected = "2024-01-01 08:00:00"
        assert result == expected

    def test_different_timezones_handled(self):
        """Test that function handles timezone conversion properly"""
        timestamp = 1704067200000  # 2024-01-01 00:00:00 UTC

        # The actual result will depend on the system's local timezone
        result = timestamp_to_date(timestamp)
        assert isinstance(result, str)
        # Should contain date components
        assert "2024" in result or "08:00:00" in result

    def test_millisecond_precision(self):
        """Test that milliseconds are properly handled (truncated)"""
        # Test timestamp with milliseconds component
        timestamp = 1704067200123  # 2024-01-01 00:00:00.123 UTC
        result = timestamp_to_date(timestamp)

        # Should still return "08:00:00" since milliseconds are truncated
        assert "08:00:00" in result

    def test_various_timestamps(self):
        """Test conversion with various timestamp values"""
        test_cases = [
            (1609459200000, "2021-01-01 08:00:00"),  # 2020-12-31 16:00:00 UTC
            (4102444800000, "2100-01-01"),  # Future date
        ]

        for timestamp, expected_prefix in test_cases:
            result = timestamp_to_date(timestamp)
            assert expected_prefix in result

    def test_return_type_always_string(self):
        """Test that return type is always string regardless of input"""
        test_inputs = [1704067200000, None, "", 0, -1000, "1704067200000"]

        for timestamp in test_inputs:
            result = timestamp_to_date(timestamp)
            assert isinstance(result, str)

    def test_edge_case_format_strings(self):
        """Test edge cases with unusual format strings"""
        timestamp = 1704067200000

        # Empty format string
        result = timestamp_to_date(timestamp, "")
        assert result == ""

        # Single character format
        result = timestamp_to_date(timestamp, "Y")
        assert isinstance(result, str)

        # Format with only separators
        result = timestamp_to_date(timestamp, "---")
        assert result == "---"


class TestDateStringToTimestamp:
    """Test cases for date_string_to_timestamp function"""

    def test_basic_date_string_conversion(self):
        """Test basic date string to timestamp conversion with default format"""
        date_string = "2024-01-01 08:00:00"
        result = date_string_to_timestamp(date_string)
        expected = 1704067200000
        assert result == expected

    def test_custom_format_string(self):
        """Test conversion with custom format strings"""
        # Test different date formats
        test_cases = [
            ("2024-01-01", "%Y-%m-%d", 1704038400000),
            ("2024/01/01 12:30:45", "%Y/%m/%d %H:%M:%S", 1704083445000),
            ("01-01-2024", "%m-%d-%Y", 1704038400000),
            ("20240101", "%Y%m%d", 1704038400000),
        ]

        for date_string, format_string, expected in test_cases:
            result = date_string_to_timestamp(date_string, format_string)
            assert result == expected

    def test_return_type_integer(self):
        """Test that function always returns integer"""
        date_string = "2024-01-01 00:00:00"
        result = date_string_to_timestamp(date_string)
        assert isinstance(result, int)

    def test_timestamp_in_milliseconds(self):
        """Test that returned timestamp is in milliseconds (13 digits)"""
        date_string = "2024-01-01 00:00:00"
        result = date_string_to_timestamp(date_string)
        assert len(str(result)) == 13

        # Verify it's milliseconds by checking it's 1000x larger than seconds timestamp
        seconds_timestamp = time.mktime(time.strptime(date_string, "%Y-%m-%d %H:%M:%S"))
        expected_milliseconds = int(seconds_timestamp * 1000)
        assert result == expected_milliseconds

    def test_different_dates(self):
        """Test conversion with various date strings"""
        test_cases = [
            ("2024-01-01    00:00:00", 1704038400000),
            ("2020-12-31 16:00:00", 1609401600000),
            ("2023-06-15 14:30:00", 1686810600000),
            ("2025-12-25 23:59:59", 1766678399000),
        ]

        for date_string, expected in test_cases:
            result = date_string_to_timestamp(date_string)
            assert result == expected

    def test_epoch_date(self):
        """Test conversion with epoch date (1970-01-01)"""
        # Note: The actual value depends on the local timezone
        date_string = "1970-01-01 00:00:00"
        result = date_string_to_timestamp(date_string)
        assert isinstance(result, int)
        # Should be a small positive or negative number depending on timezone
        assert abs(result) < 86400000  # Within 24 hours in milliseconds

    def test_leap_year_date(self):
        """Test conversion with leap year date"""
        date_string = "2024-02-29 12:00:00"  # Valid leap year date
        result = date_string_to_timestamp(date_string)
        expected = 1709179200000  # 2024-02-29 12:00:00 in milliseconds
        assert result == expected

    def test_date_only_string(self):
        """Test conversion with date-only format (assumes 00:00:00 time)"""
        date_string = "2024-01-01"
        result = date_string_to_timestamp(date_string, "%Y-%m-%d")
        # Should be equivalent to "2024-01-01 00:00:00"
        expected = 1704038400000
        assert result == expected

    def test_with_whitespace(self):
        """Test that function handles whitespace properly"""
        test_cases = [
            "  2024-01-01 00:00:00  ",
            "\t2024-01-01 00:00:00\n",
        ]

        for date_string in test_cases:
            # These should raise ValueError due to extra whitespace
            with pytest.raises(ValueError):
                date_string_to_timestamp(date_string)

    def test_invalid_date_string(self):
        """Test that invalid date string raises ValueError"""
        invalid_cases = [
            "invalid-date",
            "2024-13-01 00:00:00",  # Invalid month
            "2024-01-32 00:00:00",  # Invalid day
            "2024-01-01 25:00:00",  # Invalid hour
            "2024-01-01 00:60:00",  # Invalid minute
            "2024-02-30 00:00:00",  # Invalid date (Feb 30)
        ]

        for invalid_date in invalid_cases:
            with pytest.raises(ValueError):
                date_string_to_timestamp(invalid_date)

    def test_mismatched_format_string(self):
        """Test that mismatched format string raises ValueError"""
        test_cases = [
            ("2024-01-01 00:00:00", "%Y-%m-%d"),  # Missing time in format
            ("2024-01-01", "%Y-%m-%d %H:%M:%S"),  # Missing time in date string
            ("01/01/2024", "%Y-%m-%d"),  # Wrong separator
        ]

        for date_string, format_string in test_cases:
            with pytest.raises(ValueError):
                date_string_to_timestamp(date_string, format_string)

    def test_empty_string_input(self):
        """Test that empty string input raises ValueError"""
        with pytest.raises(ValueError):
            date_string_to_timestamp("")

    def test_none_input(self):
        """Test that None input raises TypeError"""
        with pytest.raises(TypeError):
            date_string_to_timestamp(None)


class TestDatetimeFormat:
    """Test cases for datetime_format function"""

    def test_remove_microseconds(self):
        """Test that microseconds are removed from datetime object"""
        original_dt = datetime.datetime(2024, 1, 1, 12, 30, 45, 123456)
        result = datetime_format(original_dt)

        # Verify microseconds are 0
        assert result.microsecond == 0
        # Verify other components remain the same
        assert result.year == 2024
        assert result.month == 1
        assert result.day == 1
        assert result.hour == 12
        assert result.minute == 30
        assert result.second == 45

    def test_datetime_with_zero_microseconds(self):
        """Test datetime that already has zero microseconds"""
        original_dt = datetime.datetime(2024, 1, 1, 12, 30, 45, 0)
        result = datetime_format(original_dt)

        # Should remain the same
        assert result == original_dt
        assert result.microsecond == 0

    def test_datetime_with_max_microseconds(self):
        """Test datetime with maximum microseconds value"""
        original_dt = datetime.datetime(2024, 1, 1, 12, 30, 45, 999999)
        result = datetime_format(original_dt)

        # Microseconds should be removed
        assert result.microsecond == 0
        # Other components should remain
        assert result.year == 2024
        assert result.month == 1
        assert result.day == 1
        assert result.hour == 12
        assert result.minute == 30
        assert result.second == 45

    def test_datetime_with_only_date_components(self):
        """Test datetime with only date components (time defaults to 00:00:00)"""
        original_dt = datetime.datetime(2024, 1, 1)
        result = datetime_format(original_dt)

        # Should have zero time components and zero microseconds
        assert result.year == 2024
        assert result.month == 1
        assert result.day == 1
        assert result.hour == 0
        assert result.minute == 0
        assert result.second == 0
        assert result.microsecond == 0

    def test_datetime_with_midnight(self):
        """Test datetime at midnight"""
        original_dt = datetime.datetime(2024, 1, 1, 0, 0, 0, 123456)
        result = datetime_format(original_dt)

        assert result.hour == 0
        assert result.minute == 0
        assert result.second == 0
        assert result.microsecond == 0

    def test_datetime_with_end_of_day(self):
        """Test datetime at end of day (23:59:59)"""
        original_dt = datetime.datetime(2024, 1, 1, 23, 59, 59, 999999)
        result = datetime_format(original_dt)

        assert result.hour == 23
        assert result.minute == 59
        assert result.second == 59
        assert result.microsecond == 0

    def test_leap_year_datetime(self):
        """Test datetime on leap day"""
        original_dt = datetime.datetime(2024, 2, 29, 14, 30, 15, 500000)
        result = datetime_format(original_dt)

        assert result.year == 2024
        assert result.month == 2
        assert result.day == 29
        assert result.hour == 14
        assert result.minute == 30
        assert result.second == 15
        assert result.microsecond == 0

    def test_returns_new_object(self):
        """Test that function returns a new datetime object, not the original"""
        original_dt = datetime.datetime(2024, 1, 1, 12, 30, 45, 123456)
        result = datetime_format(original_dt)

        # Verify it's a different object
        assert result is not original_dt
        # Verify original is unchanged
        assert original_dt.microsecond == 123456

    def test_datetime_with_only_seconds(self):
        """Test datetime with only seconds specified"""
        original_dt = datetime.datetime(2024, 1, 1, 12, 30, 45)
        result = datetime_format(original_dt)

        # Should have zero microseconds
        assert result.microsecond == 0
        # Other components should match
        assert result == original_dt.replace(microsecond=0)

    def test_immutability_of_original(self):
        """Test that original datetime object is not modified"""
        original_dt = datetime.datetime(2024, 1, 1, 12, 30, 45, 123456)
        original_microsecond = original_dt.microsecond

        result = datetime_format(original_dt)

        # Original should remain unchanged
        assert original_dt.microsecond == original_microsecond
        assert original_dt.microsecond == 123456

    def test_minimum_datetime_value(self):
        """Test with minimum datetime value"""
        original_dt = datetime.datetime.min
        result = datetime_format(original_dt)

        # Should have zero microseconds
        assert result.microsecond == 0
        # Other components should match
        assert result.year == original_dt.year
        assert result.month == original_dt.month
        assert result.day == original_dt.day

    def test_maximum_datetime_value(self):
        """Test with maximum datetime value"""
        original_dt = datetime.datetime.max
        result = datetime_format(original_dt)

        # Should have zero microseconds
        assert result.microsecond == 0
        # Other components should match
        assert result.year == original_dt.year
        assert result.month == original_dt.month
        assert result.day == original_dt.day

    def test_timezone_naive_datetime(self):
        """Test with timezone-naive datetime (should remain naive)"""
        original_dt = datetime.datetime(2024, 1, 1, 12, 30, 45, 123456)
        result = datetime_format(original_dt)

        # Should remain timezone-naive
        assert result.tzinfo is None

    def test_equality_with_replaced_datetime(self):
        """Test that result equals datetime.replace(microsecond=0)"""
        original_dt = datetime.datetime(2024, 1, 1, 12, 30, 45, 123456)
        result = datetime_format(original_dt)
        expected = original_dt.replace(microsecond=0)

        assert result == expected

    @pytest.mark.parametrize("year,month,day,hour,minute,second,microsecond", [
        (2024, 1, 1, 0, 0, 0, 0),  # Start of day
        (2024, 12, 31, 23, 59, 59, 999999),  # End of year
        (2000, 6, 15, 12, 30, 45, 500000),  # Random date
        (1970, 1, 1, 0, 0, 0, 123456),  # Epoch equivalent
        (2030, 3, 20, 6, 15, 30, 750000),  # Future date
    ])
    def test_parametrized_datetimes(self, year, month, day, hour, minute, second, microsecond):
        """Test multiple datetime scenarios using parametrization"""
        original_dt = datetime.datetime(year, month, day, hour, minute, second, microsecond)
        result = datetime_format(original_dt)

        # Verify microseconds are removed
        assert result.microsecond == 0

        # Verify other components remain the same
        assert result.year == year
        assert result.month == month
        assert result.day == day
        assert result.hour == hour
        assert result.minute == minute
        assert result.second == second

    def test_consistency_across_multiple_calls(self):
        """Test that multiple calls with same input produce same output"""
        original_dt = datetime.datetime(2024, 1, 1, 12, 30, 45, 123456)

        result1 = datetime_format(original_dt)
        result2 = datetime_format(original_dt)
        result3 = datetime_format(original_dt)

        # All results should be equal
        assert result1 == result2 == result3
        # All should have zero microseconds
        assert result1.microsecond == result2.microsecond == result3.microsecond == 0

    def test_type_return(self):
        """Test that return type is datetime.datetime"""
        original_dt = datetime.datetime(2024, 1, 1, 12, 30, 45, 123456)
        result = datetime_format(original_dt)

        assert isinstance(result, datetime.datetime)


class TestStr2Date:
    """Test cases for str2date function"""

    def test_basic_date_conversion(self):
        """Test basic date string to datetime conversion"""
        date_string = "2024-01-01"
        result = str2date(date_string)
        expected = datetime.datetime(2024, 1, 1, 0, 0)
        assert result == expected

    def test_leap_year_date(self):
        """Test conversion with leap year date"""
        date_string = "2024-02-29"  # Valid leap year date
        result = str2date(date_string)
        expected = datetime.datetime(2024, 2, 29, 0, 0)
        assert result == expected

    def test_different_months_and_days(self):
        """Test conversion with various months and days"""
        test_cases = [
            ("2024-12-31", datetime.datetime(2024, 12, 31, 0, 0)),
            ("2023-06-15", datetime.datetime(2023, 6, 15, 0, 0)),
            ("2000-01-01", datetime.datetime(2000, 1, 1, 0, 0)),
        ]

        for date_string, expected in test_cases:
            result = str2date(date_string)
            assert result == expected

    def test_return_type(self):
        """Test that function returns datetime object"""
        date_string = "2024-01-01"
        result = str2date(date_string)
        assert isinstance(result, datetime.datetime)

    def test_time_components_zero(self):
        """Test that hour and minute components are always zero"""
        date_string = "2024-01-01"
        result = str2date(date_string)
        assert result.hour == 0
        assert result.minute == 0
        assert result.second == 0
        assert result.microsecond == 0

    def test_invalid_date_format(self):
        """Test that invalid date format raises ValueError"""
        invalid_cases = [
            "2024/01/01",  # Wrong separator
            "01-01-2024",  # Wrong order
            # "2024-1-1",  # Missing leading zeros
            "2024-13-01",  # Invalid month
            "2024-01-32",  # Invalid day
            "2024-02-30",  # Invalid date (Feb 30)
            "invalid-date",  # Completely invalid
        ]

        for invalid_date in invalid_cases:
            with pytest.raises(ValueError):
                str2date(invalid_date)

    def test_empty_string(self):
        """Test that empty string raises ValueError"""
        with pytest.raises(ValueError):
            str2date("")

    def test_none_input(self):
        """Test that None input raises TypeError"""
        with pytest.raises(TypeError):
            str2date(None)

    @pytest.mark.parametrize("date_string,expected", [
        ("2024-01-01", datetime.datetime(2024, 1, 1, 0, 0)),
        ("2023-12-31", datetime.datetime(2023, 12, 31, 0, 0)),
        ("2000-02-29", datetime.datetime(2000, 2, 29, 0, 0)),
        ("1999-01-01", datetime.datetime(1999, 1, 1, 0, 0)),
    ])
    def test_parametrized_dates(self, date_string, expected):
        """Test multiple date scenarios using parametrization"""
        result = str2date(date_string)
        assert result == expected


class TestElapsed2Time:
    """Test cases for elapsed2time function"""

    def test_zero_milliseconds(self):
        """Test conversion with zero milliseconds"""
        result = elapsed2time(0)
        expected = "00:00:00"
        assert result == expected

    def test_less_than_one_second(self):
        """Test conversion with less than one second"""
        # 500 milliseconds = 0.5 seconds, should truncate to 0 seconds
        result = elapsed2time(500)
        expected = "00:00:00"
        assert result == expected

    def test_exactly_one_second(self):
        """Test conversion with exactly one second"""
        result = elapsed2time(1000)
        expected = "00:00:01"
        assert result == expected

    def test_multiple_seconds(self):
        """Test conversion with multiple seconds"""
        result = elapsed2time(45000)  # 45 seconds
        expected = "00:00:45"
        assert result == expected

    def test_less_than_one_minute(self):
        """Test conversion with less than one minute"""
        result = elapsed2time(59000)  # 59 seconds
        expected = "00:00:59"
        assert result == expected

    def test_exactly_one_minute(self):
        """Test conversion with exactly one minute"""
        result = elapsed2time(60000)  # 1 minute
        expected = "00:01:00"
        assert result == expected

    def test_minutes_and_seconds(self):
        """Test conversion with minutes and seconds"""
        result = elapsed2time(61500)  # 1 minute, 1.5 seconds (truncated to 1 second)
        expected = "00:01:01"
        assert result == expected

    def test_exactly_one_hour(self):
        """Test conversion with exactly one hour"""
        result = elapsed2time(3600000)  # 1 hour
        expected = "01:00:00"
        assert result == expected

    def test_hours_minutes_seconds(self):
        """Test conversion with hours, minutes, and seconds"""
        result = elapsed2time(3661000)  # 1 hour, 1 minute, 1 second
        expected = "01:01:01"
        assert result == expected

    def test_multiple_hours(self):
        """Test conversion with multiple hours"""
        result = elapsed2time(9015000)  # 2 hours, 30 minutes, 15 seconds
        expected = "02:30:15"
        assert result == expected

    def test_24_hours(self):
        """Test conversion with 24 hours"""
        result = elapsed2time(86400000)  # 24 hours
        expected = "24:00:00"
        assert result == expected

    def test_more_than_24_hours(self):
        """Test conversion with more than 24 hours"""
        result = elapsed2time(90061000)  # 25 hours, 1 minute, 1 second
        expected = "25:01:01"
        assert result == expected

    def test_large_value(self):
        """Test conversion with very large value"""
        result = elapsed2time(123456789)  # 34 hours, 17 minutes, 36.789 seconds
        expected = "34:17:36"
        assert result == expected

    def test_very_large_value(self):
        """Test conversion with very large value (over 100 hours)"""
        result = elapsed2time(372300000)  # 103 hours, 25 minutes, 0 seconds
        expected = "103:25:00"
        assert result == expected

    def test_fractional_seconds_truncation(self):
        """Test that fractional seconds are truncated, not rounded"""
        # 1.9 seconds should become 1 second, not 2 seconds
        result = elapsed2time(1999)
        expected = "00:00:01"
        assert result == expected

    def test_return_type_string(self):
        """Test that function returns string"""
        result = elapsed2time(1000)
        assert isinstance(result, str)

    def test_string_format_correct(self):
        """Test that returned string has correct HH:MM:SS format"""
        result = elapsed2time(3661000)
        # Check string length and separator positions
        assert len(result) == 8
        assert result[2] == ":"
        assert result[5] == ":"
        # Check that all parts are 2-digit
        parts = result.split(":")
        assert len(parts[0]) == 2  # HH
        assert len(parts[1]) == 2  # MM
        assert len(parts[2]) == 2  # SS

    def test_negative_input_behavior(self):
        """Test behavior with negative input"""
        # Note: The function doesn't handle negative values explicitly
        # This tests the current behavior which may be unexpected
        result = elapsed2time(-1000)
        # divmod with negatives: -1000/1000 = -1 seconds
        # divmod(-1, 60) = (-1, 59) in Python
        # So -1000 ms becomes "00:00:59" which is unexpected
        assert isinstance(result, str)

    def test_float_input(self):
        """Test conversion with float input"""
        result = elapsed2time(1500.5)  # 1.5005 seconds
        expected = "00:00:01"
        assert result == expected

    @pytest.mark.parametrize("milliseconds,expected", [
        (0, "00:00:00"),
        (999, "00:00:00"),  # Less than 1 second
        (1000, "00:00:01"),  # Exactly 1 second
        (59999, "00:00:59"),  # 59.999 seconds
        (60000, "00:01:00"),  # Exactly 1 minute
        (3599999, "00:59:59"),  # 59 minutes, 59.999 seconds
        (3600000, "01:00:00"),  # Exactly 1 hour
        (3661000, "01:01:01"),  # 1 hour, 1 minute, 1 second
        (86399999, "23:59:59"),  # 23 hours, 59 minutes, 59.999 seconds
        (86400000, "24:00:00"),  # Exactly 24 hours
    ])
    def test_parametrized_conversions(self, milliseconds, expected):
        """Test multiple conversion scenarios using parametrization"""
        result = elapsed2time(milliseconds)
        assert result == expected

    def test_consistency_across_calls(self):
        """Test that same input produces same output"""
        result1 = elapsed2time(3661000)
        result2 = elapsed2time(3661000)
        result3 = elapsed2time(3661000)

        assert result1 == result2 == result3 == "01:01:01"

    def test_edge_case_59_minutes_59_seconds(self):
        """Test edge case: 59 minutes, 59 seconds"""
        milliseconds = 59 * 60 * 1000 + 59 * 1000  # 59:59
        result = elapsed2time(milliseconds)
        expected = "00:59:59"
        assert result == expected

    def test_edge_case_59_hours_59_minutes_59_seconds(self):
        """Test edge case: 59 hours, 59 minutes, 59 seconds"""
        milliseconds = 59 * 3600 * 1000 + 59 * 60 * 1000 + 59 * 1000
        result = elapsed2time(milliseconds)
        expected = "59:59:59"
        assert result == expected


class TestDeltaSeconds:
    """Test cases for delta_seconds function"""

    def test_zero_seconds_difference(self):
        """Test when given time equals current time"""
        # Use a time very close to now to minimize test flakiness
        now = datetime.datetime.now()
        date_string = now.strftime("%Y-%m-%d %H:%M:%S")
        result = delta_seconds(date_string)
        # Should be very close to 0
        assert abs(result) < 1.0

    def test_positive_seconds_difference(self):
        """Test positive time difference (past date)"""
        now = datetime.datetime.now()
        past_time = now - datetime.timedelta(hours=1)
        date_string = past_time.strftime("%Y-%m-%d %H:%M:%S")
        result = delta_seconds(date_string)
        # Should be approximately 3600 seconds (1 hour)
        assert abs(result - 3600.0) < 1.0

    def test_negative_seconds_difference(self):
        """Test negative time difference (future date)"""
        now = datetime.datetime.now()
        future_time = now + datetime.timedelta(hours=1)
        date_string = future_time.strftime("%Y-%m-%d %H:%M:%S")
        result = delta_seconds(date_string)
        # Should be approximately -3600 seconds (1 hour)
        assert abs(result + 3600.0) < 1.0

    def test_minutes_difference(self):
        """Test difference in minutes"""
        now = datetime.datetime.now()
        past_time = now - datetime.timedelta(minutes=5)
        date_string = past_time.strftime("%Y-%m-%d %H:%M:%S")
        result = delta_seconds(date_string)
        # Should be approximately 300 seconds (5 minutes)
        assert abs(result - 300.0) < 1.0

    def test_return_type_float(self):
        """Test that function returns float"""
        now = datetime.datetime.now()
        date_string = now.strftime("%Y-%m-%d %H:%M:%S")
        result = delta_seconds(date_string)
        assert isinstance(result, float)

    def test_days_difference(self):
        """Test difference across multiple days"""
        now = datetime.datetime.now()
        past_time = now - datetime.timedelta(days=1)
        date_string = past_time.strftime("%Y-%m-%d %H:%M:%S")
        result = delta_seconds(date_string)
        # Should be approximately 86400 seconds (24 hours)
        assert abs(result - 86400.0) < 1.0

    def test_complex_time_difference(self):
        """Test complex time difference with all components"""
        now = datetime.datetime.now()
        past_time = now - datetime.timedelta(hours=2, minutes=30, seconds=15)
        date_string = past_time.strftime("%Y-%m-%d %H:%M:%S")
        result = delta_seconds(date_string)
        expected = 2 * 3600 + 30 * 60 + 15  # 2 hours + 30 minutes + 15 seconds
        assert abs(result - expected) < 1.0

    def test_invalid_date_format(self):
        """Test that invalid date format raises ValueError"""
        invalid_cases = [
            "2024-01-01",  # Missing time
            "2024-01-01 12:00",  # Missing seconds
            "2024/01/01 12:00:00",  # Wrong date separator
            "01-01-2024 12:00:00",  # Wrong date format
            "2024-13-01 12:00:00",  # Invalid month
            "2024-01-32 12:00:00",  # Invalid day
            "2024-01-01 25:00:00",  # Invalid hour
            "2024-01-01 12:60:00",  # Invalid minute
            "2024-01-01 12:00:60",  # Invalid second
            "invalid datetime string",  # Completely invalid
        ]

        for invalid_date in invalid_cases:
            with pytest.raises(ValueError):
                delta_seconds(invalid_date)

    def test_empty_string(self):
        """Test that empty string raises ValueError"""
        with pytest.raises(ValueError):
            delta_seconds("")

    def test_none_input(self):
        """Test that None input raises TypeError"""
        with pytest.raises(TypeError):
            delta_seconds(None)

    def test_whitespace_string(self):
        """Test that whitespace-only string raises ValueError"""
        with pytest.raises(ValueError):
            delta_seconds("   ")

    def test_very_old_date(self):
        """Test with very old date"""
        date_string = "2000-01-01 12:00:00"
        result = delta_seconds(date_string)
        # Should be a large positive number (many years in seconds)
        assert result > 0
        assert isinstance(result, float)

    def test_very_future_date(self):
        """Test with very future date"""
        date_string = "2030-01-01 12:00:00"
        result = delta_seconds(date_string)
        # Should be a large negative number
        assert result < 0
        assert isinstance(result, float)

    def test_consistency_across_calls(self):
        """Test that same input produces consistent results"""
        now = datetime.datetime.now()
        past_time = now - datetime.timedelta(minutes=10)
        date_string = past_time.strftime("%Y-%m-%d %H:%M:%S")

        result1 = delta_seconds(date_string)
        result2 = delta_seconds(date_string)
        result3 = delta_seconds(date_string)

        # All results should be very close (within 0.1 seconds)
        assert abs(result1 - result2) < 0.1
        assert abs(result2 - result3) < 0.1

    def test_leap_year_date(self):
        """Test with leap year date (basic functionality)"""
        # This test verifies the function can handle leap year dates
        # without checking specific time differences
        date_string = "2024-02-29 12:00:00"
        result = delta_seconds(date_string)
        assert isinstance(result, float)

    def test_month_boundary(self):
        """Test crossing month boundary"""
        now = datetime.datetime.now()
        # Use first day of current month at a specific time
        first_day = datetime.datetime(now.year, now.month, 1, 12, 0, 0)
        if first_day < now:
            date_string = first_day.strftime("%Y-%m-%d %H:%M:%S")
            result = delta_seconds(date_string)
            assert result > 0  # Should be positive if first_day is in past
        else:
            # If we're testing on the first day of month
            date_string = "2024-01-31 12:00:00"  # Use a known past date
            result = delta_seconds(date_string)
            assert result > 0
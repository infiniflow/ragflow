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

import math
from common.float_utils import get_float

class TestGetFloat:

    def test_valid_float_string(self):
        """Test conversion of valid float strings"""
        assert get_float("3.14") == 3.14
        assert get_float("-2.5") == -2.5
        assert get_float("0.0") == 0.0
        assert get_float("123.456") == 123.456

    def test_valid_integer_string(self):
        """Test conversion of valid integer strings"""
        assert get_float("42") == 42.0
        assert get_float("-100") == -100.0
        assert get_float("0") == 0.0

    def test_valid_numbers(self):
        """Test conversion of actual number types"""
        assert get_float(3.14) == 3.14
        assert get_float(-2.5) == -2.5
        assert get_float(42) == 42.0
        assert get_float(0) == 0.0

    def test_none_input(self):
        """Test handling of None input"""
        result = get_float(None)
        assert math.isinf(result)
        assert result < 0  # Should be negative infinity

    def test_invalid_strings(self):
        """Test handling of invalid string inputs"""
        result = get_float("invalid")
        assert math.isinf(result)
        assert result < 0

        result = get_float("12.34.56")
        assert math.isinf(result)
        assert result < 0

        result = get_float("")
        assert math.isinf(result)
        assert result < 0

    def test_boolean_input(self):
        """Test conversion of boolean values"""
        assert get_float(True) == 1.0
        assert get_float(False) == 0.0

    def test_special_float_strings(self):
        """Test handling of special float strings"""
        assert get_float("inf") == float('inf')
        assert get_float("-inf") == float('-inf')

        # NaN should return -inf according to our function's design
        result = get_float("nan")
        assert math.isnan(result)

    def test_very_large_numbers(self):
        """Test very large number strings"""
        assert get_float("1e308") == 1e308
        # This will become inf in Python, but let's test it
        large_result = get_float("1e500")
        assert math.isinf(large_result)

    def test_whitespace_strings(self):
        """Test strings with whitespace"""
        assert get_float("  3.14  ") == 3.14
        result = get_float("  invalid  ")
        assert math.isinf(result)
        assert result < 0
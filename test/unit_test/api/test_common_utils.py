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
Unit tests for api.utils.common module.
"""

import pytest
from api.utils.common import string_to_bytes, bytes_to_string


class TestStringToBytes:
    """Test cases for string_to_bytes function"""

    def test_string_input_returns_bytes(self):
        """Test that string input is converted to bytes"""
        input_string = "hello world"
        result = string_to_bytes(input_string)
        
        assert isinstance(result, bytes)
        assert result == b"hello world"

    def test_bytes_input_returns_same_bytes(self):
        """Test that bytes input is returned unchanged"""
        input_bytes = b"hello world"
        result = string_to_bytes(input_bytes)
        
        assert isinstance(result, bytes)
        assert result == input_bytes
        assert result is input_bytes  # Should be the same object

    @pytest.mark.parametrize("input_val,expected", [
        ("test", b"test"),
        ("", b""),
        ("123", b"123"),
        ("Hello World", b"Hello World"),
        ("Hello 世界 🌍", "Hello 世界 🌍".encode("utf-8")),
        ("Hello, world! @#$%^&*()", b"Hello, world! @#$%^&*()"),
        ("Newline\nTab\tQuote\"", b"Newline\nTab\tQuote\""),
    ])
    def test_various_string_inputs(self, input_val, expected):
        """Test conversion of various string inputs including unicode and special characters"""
        result = string_to_bytes(input_val)
        assert isinstance(result, bytes)
        assert result == expected


class TestBytesToString:
    """Test cases for bytes_to_string function"""

    @pytest.mark.parametrize("input_bytes,expected", [
        (b"hello world", "hello world"),
        (b"test", "test"),
        (b"", ""),
        (b"123", "123"),
        (b"Hello World", "Hello World"),
        ("Hello 世界 🌍".encode("utf-8"), "Hello 世界 🌍"),
        (b"Special: @#$%^&*()", "Special: @#$%^&*()"),
    ])
    def test_various_bytes_inputs(self, input_bytes, expected):
        """Test conversion of various bytes inputs including unicode"""
        result = bytes_to_string(input_bytes)
        assert isinstance(result, str)
        assert result == expected

    def test_invalid_utf8_raises_error(self):
        """Test that invalid UTF-8 bytes raise an error"""
        # Invalid UTF-8 sequence
        invalid_bytes = b"\xff\xfe"
        
        with pytest.raises(UnicodeDecodeError):
            bytes_to_string(invalid_bytes)


class TestRoundtripConversion:
    """Test roundtrip conversions between string and bytes"""

    @pytest.mark.parametrize("test_string", [
        "Simple text",
        "Hello, World! 世界",
        "Unicode: 你好世界 🌍",
        "Special: !@#$%^&*()",
        "Multiline\nWith\tTabs",
        "",
    ])
    def test_string_to_bytes_to_string(self, test_string):
        """Test converting string to bytes and back for various inputs"""
        as_bytes = string_to_bytes(test_string)
        back_to_string = bytes_to_string(as_bytes)
        assert back_to_string == test_string

    @pytest.mark.parametrize("test_bytes", [
        b"Simple text",
        b"Hello, World!",
        "Unicode: 你好世界 🌍".encode("utf-8"),
        b"Special: !@#$%^&*()",
        b"",
    ])
    def test_bytes_to_string_to_bytes(self, test_bytes):
        """Test converting bytes to string and back for various inputs"""
        as_string = bytes_to_string(test_bytes)
        back_to_bytes = string_to_bytes(as_string)
        assert back_to_bytes == test_bytes


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

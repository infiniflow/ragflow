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

    def test_empty_string(self):
        """Test conversion of empty string"""
        result = string_to_bytes("")
        
        assert isinstance(result, bytes)
        assert result == b""
        assert len(result) == 0

    def test_unicode_characters(self):
        """Test conversion of Unicode characters"""
        input_string = "Hello 世界 🌍"
        result = string_to_bytes(input_string)
        
        assert isinstance(result, bytes)
        # Verify it can be decoded back
        assert result.decode("utf-8") == input_string

    def test_special_characters(self):
        """Test conversion of special characters"""
        input_string = "Hello, world! @#$%^&*()"
        result = string_to_bytes(input_string)
        
        assert isinstance(result, bytes)
        assert result.decode("utf-8") == input_string

    @pytest.mark.parametrize("input_val,expected", [
        ("test", b"test"),
        ("", b""),
        ("123", b"123"),
        ("Hello World", b"Hello World"),
    ])
    def test_various_string_inputs(self, input_val, expected):
        """Test various string inputs"""
        result = string_to_bytes(input_val)
        assert result == expected


class TestBytesToString:
    """Test cases for bytes_to_string function"""

    def test_bytes_input_returns_string(self):
        """Test that bytes input is converted to string"""
        input_bytes = b"hello world"
        result = bytes_to_string(input_bytes)
        
        assert isinstance(result, str)
        assert result == "hello world"

    def test_empty_bytes(self):
        """Test conversion of empty bytes"""
        result = bytes_to_string(b"")
        
        assert isinstance(result, str)
        assert result == ""
        assert len(result) == 0

    def test_unicode_bytes(self):
        """Test conversion of Unicode bytes"""
        input_bytes = "Hello 世界 🌍".encode("utf-8")
        result = bytes_to_string(input_bytes)
        
        assert isinstance(result, str)
        assert result == "Hello 世界 🌍"

    @pytest.mark.parametrize("input_bytes,expected", [
        (b"test", "test"),
        (b"", ""),
        (b"123", "123"),
        (b"Hello World", "Hello World"),
    ])
    def test_various_bytes_inputs(self, input_bytes, expected):
        """Test various bytes inputs"""
        result = bytes_to_string(input_bytes)
        assert result == expected

    def test_invalid_utf8_raises_error(self):
        """Test that invalid UTF-8 bytes raise an error"""
        # Invalid UTF-8 sequence
        invalid_bytes = b"\xff\xfe"
        
        with pytest.raises(UnicodeDecodeError):
            bytes_to_string(invalid_bytes)


class TestRoundtripConversion:
    """Test roundtrip conversions between string and bytes"""

    def test_string_to_bytes_to_string(self):
        """Test converting string to bytes and back"""
        original = "Hello, World! 世界"
        
        as_bytes = string_to_bytes(original)
        back_to_string = bytes_to_string(as_bytes)
        
        assert back_to_string == original

    def test_bytes_to_string_to_bytes(self):
        """Test converting bytes to string and back"""
        original = b"Hello, World!"
        
        as_string = bytes_to_string(original)
        back_to_bytes = string_to_bytes(as_string)
        
        assert back_to_bytes == original

    @pytest.mark.parametrize("test_string", [
        "Simple text",
        "Unicode: 你好世界 🌍",
        "Special: !@#$%^&*()",
        "Multiline\nWith\tTabs",
        "",
    ])
    def test_roundtrip_various_strings(self, test_string):
        """Test roundtrip conversion for various strings"""
        result = bytes_to_string(string_to_bytes(test_string))
        assert result == test_string


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

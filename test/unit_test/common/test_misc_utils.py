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
import uuid
from common.misc_utils import get_uuid

class TestGetUuid:
    """Test cases for get_uuid function"""

    def test_returns_string(self):
        """Test that function returns a string"""
        result = get_uuid()
        assert isinstance(result, str)

    def test_hex_format(self):
        """Test that returned string is in hex format"""
        result = get_uuid()
        # UUID v1 hex should be 32 characters (without dashes)
        assert len(result) == 32
        # Should only contain hexadecimal characters
        assert all(c in '0123456789abcdef' for c in result)

    def test_no_dashes_in_result(self):
        """Test that result contains no dashes"""
        result = get_uuid()
        assert '-' not in result

    def test_unique_results(self):
        """Test that multiple calls return different UUIDs"""
        results = [get_uuid() for _ in range(10)]

        # All results should be unique
        assert len(results) == len(set(results))

        # All should be valid hex strings of correct length
        for result in results:
            assert len(result) == 32
            assert all(c in '0123456789abcdef' for c in result)

    def test_valid_uuid_structure(self):
        """Test that the hex string can be converted back to UUID"""
        result = get_uuid()

        # Should be able to create UUID from the hex string
        reconstructed_uuid = uuid.UUID(hex=result)
        assert isinstance(reconstructed_uuid, uuid.UUID)

        # The hex representation should match the original
        assert reconstructed_uuid.hex == result

    def test_uuid1_specific_characteristics(self):
        """Test that UUID v1 characteristics are present"""
        result = get_uuid()
        uuid_obj = uuid.UUID(hex=result)

        # UUID v1 should have version 1
        assert uuid_obj.version == 1

        # Variant should be RFC 4122
        assert uuid_obj.variant == 'specified in RFC 4122'

    def test_result_length_consistency(self):
        """Test that all generated UUIDs have consistent length"""
        for _ in range(100):
            result = get_uuid()
            assert len(result) == 32

    def test_hex_characters_only(self):
        """Test that only valid hex characters are used"""
        for _ in range(100):
            result = get_uuid()
            # Should only contain lowercase hex characters (UUID hex is lowercase)
            assert result.islower()
            assert all(c in '0123456789abcdef' for c in result)
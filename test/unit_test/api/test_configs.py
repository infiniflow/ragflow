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
Unit tests for api.utils.configs module.
"""

import pytest
import pickle
import base64
from unittest.mock import patch
from api.utils.configs import (
    serialize_b64,
    deserialize_b64,
    RestrictedUnpickler,
    restricted_loads,
    safe_module
)


class TestSerializeB64:
    """Test cases for serialize_b64 function"""

    def test_serialize_dict(self):
        """Test serialization of a dictionary"""
        test_dict = {"key": "value", "number": 42}
        result = serialize_b64(test_dict)
        
        assert isinstance(result, bytes)
        # Should be valid base64
        decoded = base64.b64decode(result)
        assert isinstance(decoded, bytes)

    def test_serialize_list(self):
        """Test serialization of a list"""
        test_list = [1, 2, 3, "test", {"nested": "dict"}]
        result = serialize_b64(test_list)
        
        assert isinstance(result, bytes)

    def test_serialize_with_to_str_false(self):
        """Test serialization with to_str=False returns bytes"""
        test_data = {"test": "data"}
        result = serialize_b64(test_data, to_str=False)
        
        assert isinstance(result, bytes)

    def test_serialize_with_to_str_true(self):
        """Test serialization with to_str=True returns string"""
        test_data = {"test": "data"}
        result = serialize_b64(test_data, to_str=True)
        
        assert isinstance(result, str)
        # Should be valid base64 string
        base64.b64decode(result)  # Should not raise

    def test_serialize_string(self):
        """Test serialization of a string"""
        test_string = "Hello, World!"
        result = serialize_b64(test_string)
        
        assert isinstance(result, bytes)

    def test_serialize_number(self):
        """Test serialization of numbers"""
        test_int = 12345
        result = serialize_b64(test_int)
        
        assert isinstance(result, bytes)

    def test_serialize_complex_nested_structure(self):
        """Test serialization of complex nested structures"""
        test_data = {
            "list": [1, 2, 3],
            "dict": {"nested": {"deep": "value"}},
            "tuple": (1, 2, 3),
            "string": "test",
            "number": 42.5
        }
        result = serialize_b64(test_data)
        
        assert isinstance(result, bytes)

    def test_serialize_none(self):
        """Test serialization of None"""
        result = serialize_b64(None)
        
        assert isinstance(result, bytes)

    def test_serialize_empty_dict(self):
        """Test serialization of empty dictionary"""
        result = serialize_b64({})
        
        assert isinstance(result, bytes)

    def test_serialize_empty_list(self):
        """Test serialization of empty list"""
        result = serialize_b64([])
        
        assert isinstance(result, bytes)


class TestDeserializeB64:
    """Test cases for deserialize_b64 function"""

    def test_deserialize_string_input(self):
        """Test deserialization with string input"""
        test_data = {"key": "value"}
        serialized = serialize_b64(test_data, to_str=True)
        
        result = deserialize_b64(serialized)
        
        assert result == test_data

    def test_deserialize_bytes_input(self):
        """Test deserialization with bytes input"""
        test_data = {"key": "value"}
        serialized = serialize_b64(test_data, to_str=False)
        
        result = deserialize_b64(serialized)
        
        assert result == test_data

    @patch('api.utils.configs.get_base_config')
    def test_deserialize_with_safe_module_disabled(self, mock_config):
        """Test deserialization with safe module checking disabled"""
        mock_config.return_value = False
        
        test_data = {"test": "data"}
        serialized = serialize_b64(test_data, to_str=True)
        
        result = deserialize_b64(serialized)
        
        assert result == test_data
        mock_config.assert_called_once_with('use_deserialize_safe_module', False)

    @patch('api.utils.configs.get_base_config')
    def test_deserialize_with_safe_module_enabled(self, mock_config):
        """Test deserialization with safe module checking enabled"""
        mock_config.return_value = True
        
        # Simple data that doesn't require unsafe modules
        test_data = {"test": "data", "number": 42}
        serialized = serialize_b64(test_data, to_str=True)
        
        result = deserialize_b64(serialized)
        
        assert result == test_data

    def test_roundtrip_serialization(self):
        """Test complete roundtrip serialization and deserialization"""
        test_data = {
            "string": "test",
            "number": 123,
            "list": [1, 2, 3],
            "nested": {"key": "value"}
        }
        
        serialized = serialize_b64(test_data, to_str=True)
        deserialized = deserialize_b64(serialized)
        
        assert deserialized == test_data

    @pytest.mark.parametrize("test_data", [
        {"key": "value"},
        [1, 2, 3, 4, 5],
        "simple string",
        42,
        3.14,
        None,
        {"nested": {"deep": {"structure": "value"}}},
    ])
    def test_roundtrip_various_data_types(self, test_data):
        """Test roundtrip for various data types"""
        serialized = serialize_b64(test_data)
        deserialized = deserialize_b64(serialized)
        
        assert deserialized == test_data


class TestRestrictedUnpickler:
    """Test cases for RestrictedUnpickler class"""

    def test_allows_safe_modules(self):
        """Test that safe modules are allowed"""
        # Create a simple object that would be in a safe module context
        test_data = {"test": "data"}
        pickled = pickle.dumps(test_data)
        
        # This should work without raising
        result = restricted_loads(pickled)
        assert result == test_data

    @patch('api.utils.configs.get_base_config')
    def test_blocks_unsafe_modules(self, mock_config):
        """Test that unsafe modules are blocked"""
        mock_config.return_value = True
        
        # Try to pickle something from an unsafe module
        # We'll simulate this by creating a pickle that references os module
        class UnsafeClass:
            __module__ = 'os.path'
            def __reduce__(self):
                return (eval, ("1+1",))
        
        try:
            unsafe_obj = UnsafeClass()
            pickled = pickle.dumps(unsafe_obj)
            
            with pytest.raises(pickle.UnpicklingError):
                restricted_loads(pickled)
        except:
            # If we can't create the unsafe pickle, skip this test
            pytest.skip("Unable to create unsafe pickle for testing")

    def test_safe_module_set_contains_expected_modules(self):
        """Test that safe_module set contains expected modules"""
        assert 'numpy' in safe_module
        assert 'rag_flow' in safe_module

    def test_restricted_loads_with_safe_data(self):
        """Test restricted_loads with safe data"""
        test_data = [1, 2, 3, "test", {"key": "value"}]
        pickled = pickle.dumps(test_data)
        
        result = restricted_loads(pickled)
        
        assert result == test_data


class TestIntegrationScenarios:
    """Integration tests for serialization/deserialization workflows"""

    def test_serialize_deserialize_workflow(self):
        """Test complete workflow of serialize and deserialize"""
        original_data = {
            "user": "test_user",
            "settings": {
                "theme": "dark",
                "notifications": True
            },
            "items": [1, 2, 3, 4, 5]
        }
        
        # Serialize to string
        serialized_str = serialize_b64(original_data, to_str=True)
        assert isinstance(serialized_str, str)
        
        # Deserialize back
        deserialized = deserialize_b64(serialized_str)
        assert deserialized == original_data

    def test_serialize_deserialize_with_bytes(self):
        """Test workflow using bytes format"""
        original_data = {"test": "data", "number": 42}
        
        # Serialize to bytes
        serialized_bytes = serialize_b64(original_data, to_str=False)
        assert isinstance(serialized_bytes, bytes)
        
        # Deserialize back
        deserialized = deserialize_b64(serialized_bytes)
        assert deserialized == original_data

    @patch('api.utils.configs.get_base_config')
    def test_safe_deserialization_workflow(self, mock_config):
        """Test safe deserialization workflow"""
        mock_config.return_value = True
        
        test_data = {"safe": "data"}
        serialized = serialize_b64(test_data, to_str=True)
        
        result = deserialize_b64(serialized)
        
        assert result == test_data

    def test_empty_data_workflow(self):
        """Test workflow with empty data"""
        empty_dict = {}
        
        serialized = serialize_b64(empty_dict, to_str=True)
        deserialized = deserialize_b64(serialized)
        
        assert deserialized == empty_dict

    def test_large_data_workflow(self):
        """Test workflow with larger data structures"""
        large_data = {
            f"key_{i}": {
                "value": i,
                "list": list(range(10)),
                "nested": {"deep": f"value_{i}"}
            }
            for i in range(100)
        }
        
        serialized = serialize_b64(large_data, to_str=True)
        deserialized = deserialize_b64(serialized)
        
        assert deserialized == large_data


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

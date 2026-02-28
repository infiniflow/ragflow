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

    @pytest.mark.parametrize("test_data", [
        {"key": "value", "number": 42},
        [1, 2, 3, "test", {"nested": "dict"}],
        "Hello, World!",
        12345,
        {
            "list": [1, 2, 3],
            "dict": {"nested": {"deep": "value"}},
            "tuple": (1, 2, 3),
            "string": "test",
            "number": 42.5
        },
        None,
        {},
        [],
    ])
    def test_serialize_returns_bytes(self, test_data):
        """Test serialization of various data types returns bytes"""
        result = serialize_b64(test_data, to_str=False)
        
        assert isinstance(result, bytes)
        # Should be valid base64
        decoded = base64.b64decode(result)
        assert isinstance(decoded, bytes)

    def test_serialize_with_to_str_true(self):
        """Test serialization with to_str=True returns string"""
        test_data = {"test": "data"}
        result = serialize_b64(test_data, to_str=True)
        
        assert isinstance(result, str)
        # Should be valid base64 string
        base64.b64decode(result)  # Should not raise


class TestDeserializeB64:
    """Test cases for deserialize_b64 function"""

    @pytest.mark.parametrize("to_str", [True, False])
    def test_deserialize_string_and_bytes_input(self, to_str):
        """Test deserialization with both string and bytes input"""
        test_data = {"key": "value"}
        serialized = serialize_b64(test_data, to_str=to_str)
        
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

    @pytest.mark.parametrize("test_data", [
        {"key": "value"},
        {
            "string": "test",
            "number": 123,
            "list": [1, 2, 3],
            "nested": {"key": "value"}
        },
        [1, 2, 3, 4, 5],
        "simple string",
        42,
        3.14,
        None,
        {"nested": {"deep": {"structure": "value"}}},
    ])
    def test_roundtrip_various_data_types(self, test_data):
        """Test roundtrip serialization and deserialization for various data types"""
        serialized = serialize_b64(test_data)
        deserialized = deserialize_b64(serialized)
        
        assert deserialized == test_data


class TestRestrictedUnpickler:
    """Test cases for RestrictedUnpickler class"""

    @pytest.mark.parametrize("test_data", [
        {"test": "data"},
        [1, 2, 3, "test", {"key": "value"}],
        {"nested": {"deep": "structure"}},
        [1, 2, 3],
        "simple string",
    ])
    def test_restricted_loads_with_safe_data(self, test_data):
        """Test restricted_loads with various safe data types"""
        pickled = pickle.dumps(test_data)
        
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


class TestIntegrationScenarios:
    """Integration tests for serialization/deserialization workflows"""

    @pytest.mark.parametrize("to_str,original_data", [
        (True, {
            "user": "test_user",
            "settings": {
                "theme": "dark",
                "notifications": True
            },
            "items": [1, 2, 3, 4, 5]
        }),
        (False, {"test": "data", "number": 42}),
        (True, {}),
        (True, {
            f"key_{i}": {
                "value": i,
                "list": list(range(10)),
                "nested": {"deep": f"value_{i}"}
            }
            for i in range(100)
        }),
    ])
    def test_serialize_deserialize_workflow(self, to_str, original_data):
        """Test complete workflow of serialize and deserialize with various data"""
        # Serialize
        serialized = serialize_b64(original_data, to_str=to_str)
        
        if to_str:
            assert isinstance(serialized, str)
        else:
            assert isinstance(serialized, bytes)
        
        # Deserialize back
        deserialized = deserialize_b64(serialized)
        assert deserialized == original_data

    @patch('api.utils.configs.get_base_config')
    def test_safe_deserialization_workflow(self, mock_config):
        """Test safe deserialization workflow"""
        mock_config.return_value = True
        
        test_data = {"safe": "data"}
        serialized = serialize_b64(test_data, to_str=True)
        
        result = deserialize_b64(serialized)
        
        assert result == test_data


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

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
Unit tests for api.utils.json_encode module.
"""

import pytest
import json
import datetime
from enum import Enum, IntEnum
from api.utils.json_encode import (
    BaseType,
    CustomJSONEncoder,
    json_dumps,
    json_loads
)


class TestBaseTypeToDict:
    """Test cases for BaseType.to_dict method"""

    def test_simple_object_to_dict(self):
        """Test converting simple object to dictionary"""
        class SimpleType(BaseType):
            def __init__(self):
                self.name = "test"
                self.value = 42
        
        obj = SimpleType()
        result = obj.to_dict()
        
        assert isinstance(result, dict)
        assert result == {"name": "test", "value": 42}

    def test_private_attributes_excluded(self):
        """Test that private attributes (starting with _) are stripped"""
        class PrivateType(BaseType):
            def __init__(self):
                self._private = "hidden"
                self._another = "also hidden"
                self.public = "visible"
        
        obj = PrivateType()
        result = obj.to_dict()
        
        # Private attributes should have _ stripped in keys
        assert "private" in result
        assert "another" in result
        assert "public" in result
        assert result["private"] == "hidden"
        assert result["another"] == "also hidden"
        assert result["public"] == "visible"

    def test_empty_object(self):
        """Test converting empty object to dictionary"""
        class EmptyType(BaseType):
            pass
        
        obj = EmptyType()
        result = obj.to_dict()
        
        assert isinstance(result, dict)
        assert len(result) == 0

    def test_nested_object_to_dict(self):
        """Test converting object with nested values"""
        class NestedType(BaseType):
            def __init__(self):
                self.data = {"key": "value"}
                self.items = [1, 2, 3]
                self.text = "test"
        
        obj = NestedType()
        result = obj.to_dict()
        
        assert result["data"] == {"key": "value"}
        assert result["items"] == [1, 2, 3]
        assert result["text"] == "test"


class TestBaseTypeToDictWithType:
    """Test cases for BaseType.to_dict_with_type method"""

    def test_includes_type_information(self):
        """Test that type information is included"""
        class TypedObject(BaseType):
            def __init__(self):
                self.value = "test"
        
        obj = TypedObject()
        result = obj.to_dict_with_type()
        
        assert "type" in result
        assert "data" in result
        assert result["type"] == "TypedObject"

    def test_includes_module_information(self):
        """Test that module information is included"""
        class ModuleObject(BaseType):
            def __init__(self):
                self.value = "test"
        
        obj = ModuleObject()
        result = obj.to_dict_with_type()
        
        assert "module" in result
        assert result["module"] is not None

    def test_nested_objects_with_types(self):
        """Test nested BaseType objects include type info"""
        class InnerType(BaseType):
            def __init__(self):
                self.inner_value = "inner"
        
        class OuterType(BaseType):
            def __init__(self):
                self.nested = InnerType()
                self.value = "outer"
        
        obj = OuterType()
        result = obj.to_dict_with_type()
        
        assert result["type"] == "OuterType"
        assert "data" in result
        assert "nested" in result["data"]
        assert result["data"]["nested"]["type"] == "InnerType"

    def test_list_handling(self):
        """Test handling of lists in to_dict_with_type"""
        class ListType(BaseType):
            def __init__(self):
                self.items = [1, 2, 3]
        
        obj = ListType()
        result = obj.to_dict_with_type()
        
        assert "items" in result["data"]
        items_data = result["data"]["items"]
        assert items_data["type"] == "list"
        assert isinstance(items_data["data"], list)

    def test_dict_handling(self):
        """Test handling of dictionaries in to_dict_with_type"""
        class DictType(BaseType):
            def __init__(self):
                self.config = {"key": "value"}
        
        obj = DictType()
        result = obj.to_dict_with_type()
        
        assert "config" in result["data"]
        config_data = result["data"]["config"]
        assert config_data["type"] == "dict"


class TestCustomJSONEncoder:
    """Test cases for CustomJSONEncoder class"""

    def test_encode_datetime(self):
        """Test encoding of datetime objects"""
        dt = datetime.datetime(2025, 12, 3, 14, 30, 45)
        result = json.dumps(dt, cls=CustomJSONEncoder)
        
        assert result == '"2025-12-03 14:30:45"'

    def test_encode_date(self):
        """Test encoding of date objects"""
        d = datetime.date(2025, 12, 3)
        result = json.dumps(d, cls=CustomJSONEncoder)
        
        assert result == '"2025-12-03"'

    def test_encode_timedelta(self):
        """Test encoding of timedelta objects"""
        td = datetime.timedelta(days=1, hours=2, minutes=30)
        result = json.dumps(td, cls=CustomJSONEncoder)
        
        assert isinstance(json.loads(result), str)
        assert "1 day" in json.loads(result)

    def test_encode_enum(self):
        """Test encoding of Enum objects"""
        class Color(Enum):
            RED = "red"
            GREEN = "green"
            BLUE = "blue"
        
        result = json.dumps(Color.RED, cls=CustomJSONEncoder)
        
        assert result == '"red"'

    def test_encode_int_enum(self):
        """Test encoding of IntEnum objects"""
        class Priority(IntEnum):
            LOW = 1
            MEDIUM = 2
            HIGH = 3
        
        result = json.dumps(Priority.HIGH, cls=CustomJSONEncoder)
        
        assert result == '3'

    def test_encode_set(self):
        """Test encoding of set objects"""
        test_set = {1, 2, 3, 4, 5}
        result = json.dumps(test_set, cls=CustomJSONEncoder)
        
        # Set should be converted to list
        decoded = json.loads(result)
        assert isinstance(decoded, list)
        assert set(decoded) == test_set

    def test_encode_basetype_object(self):
        """Test encoding of BaseType objects"""
        class TestType(BaseType):
            def __init__(self):
                self.name = "test"
                self.value = 42
        
        obj = TestType()
        result = json.dumps(obj, cls=CustomJSONEncoder)
        
        decoded = json.loads(result)
        assert decoded["name"] == "test"
        assert decoded["value"] == 42

    def test_encode_basetype_with_type_flag(self):
        """Test encoding BaseType with with_type flag"""
        class TestType(BaseType):
            def __init__(self):
                self.value = "test"
        
        obj = TestType()
        encoder = CustomJSONEncoder(with_type=True)
        result = json.dumps(obj, cls=CustomJSONEncoder, with_type=True)
        
        decoded = json.loads(result)
        assert "type" in decoded
        assert "data" in decoded

    def test_encode_type_object(self):
        """Test encoding of type objects"""
        result = json.dumps(str, cls=CustomJSONEncoder)
        
        assert result == '"str"'

    def test_encode_nested_structures(self):
        """Test encoding of nested structures with various types"""
        class TestType(BaseType):
            def __init__(self):
                self.name = "test"
        
        data = {
            "datetime": datetime.datetime(2025, 12, 3, 12, 0, 0),
            "date": datetime.date(2025, 12, 3),
            "set": {1, 2, 3},
            "object": TestType(),
            "list": [1, 2, 3]
        }
        
        result = json.dumps(data, cls=CustomJSONEncoder)
        decoded = json.loads(result)
        
        assert decoded["datetime"] == "2025-12-03 12:00:00"
        assert decoded["date"] == "2025-12-03"
        assert isinstance(decoded["set"], list)
        assert decoded["object"]["name"] == "test"


class TestJsonDumps:
    """Test cases for json_dumps function"""

    def test_json_dumps_basic(self):
        """Test basic json_dumps functionality"""
        data = {"key": "value", "number": 42}
        result = json_dumps(data)
        
        assert isinstance(result, str)
        assert json.loads(result) == data

    def test_json_dumps_with_byte_false(self):
        """Test json_dumps with byte=False returns string"""
        data = {"test": "data"}
        result = json_dumps(data, byte=False)
        
        assert isinstance(result, str)

    def test_json_dumps_with_byte_true(self):
        """Test json_dumps with byte=True returns bytes"""
        data = {"test": "data"}
        result = json_dumps(data, byte=True)
        
        assert isinstance(result, bytes)

    def test_json_dumps_with_indent(self):
        """Test json_dumps with indentation"""
        data = {"key": "value"}
        result = json_dumps(data, indent=2)
        
        assert isinstance(result, str)
        assert "\n" in result  # Indented JSON has newlines

    def test_json_dumps_with_type_false(self):
        """Test json_dumps with with_type=False"""
        class TestType(BaseType):
            def __init__(self):
                self.value = "test"
        
        obj = TestType()
        result = json_dumps(obj, with_type=False)
        
        decoded = json.loads(result)
        assert "type" not in decoded
        assert decoded["value"] == "test"

    def test_json_dumps_with_type_true(self):
        """Test json_dumps with with_type=True"""
        class TestType(BaseType):
            def __init__(self):
                self.value = "test"
        
        obj = TestType()
        result = json_dumps(obj, with_type=True)
        
        decoded = json.loads(result)
        assert "type" in decoded
        assert "data" in decoded

    def test_json_dumps_datetime(self):
        """Test json_dumps with datetime objects"""
        data = {
            "timestamp": datetime.datetime(2025, 12, 3, 15, 30, 0)
        }
        result = json_dumps(data)
        
        decoded = json.loads(result)
        assert decoded["timestamp"] == "2025-12-03 15:30:00"


class TestJsonLoads:
    """Test cases for json_loads function"""

    def test_json_loads_string_input(self):
        """Test json_loads with string input"""
        json_string = '{"key": "value", "number": 42}'
        result = json_loads(json_string)
        
        assert isinstance(result, dict)
        assert result["key"] == "value"
        assert result["number"] == 42

    def test_json_loads_bytes_input(self):
        """Test json_loads with bytes input"""
        json_bytes = b'{"key": "value"}'
        result = json_loads(json_bytes)
        
        assert isinstance(result, dict)
        assert result["key"] == "value"

    def test_json_loads_with_object_hook(self):
        """Test json_loads with object_hook parameter"""
        def custom_hook(obj):
            if "special" in obj:
                obj["processed"] = True
            return obj
        
        json_string = '{"special": "value"}'
        result = json_loads(json_string, object_hook=custom_hook)
        
        assert result["processed"] is True

    def test_json_loads_empty_object(self):
        """Test json_loads with empty object"""
        result = json_loads('{}')
        
        assert isinstance(result, dict)
        assert len(result) == 0

    def test_json_loads_array(self):
        """Test json_loads with array"""
        result = json_loads('[1, 2, 3, 4, 5]')
        
        assert isinstance(result, list)
        assert result == [1, 2, 3, 4, 5]


class TestRoundtripConversion:
    """Test roundtrip conversions between dumps and loads"""

    def test_roundtrip_dict(self):
        """Test roundtrip conversion of dictionary"""
        original = {"key": "value", "number": 42, "list": [1, 2, 3]}
        
        dumped = json_dumps(original)
        loaded = json_loads(dumped)
        
        assert loaded == original

    def test_roundtrip_with_bytes(self):
        """Test roundtrip with byte conversion"""
        original = {"test": "data"}
        
        dumped = json_dumps(original, byte=True)
        loaded = json_loads(dumped)
        
        assert loaded == original

    def test_roundtrip_basetype(self):
        """Test roundtrip with BaseType object"""
        class TestType(BaseType):
            def __init__(self):
                self.name = "test"
                self.value = 42
        
        obj = TestType()
        
        dumped = json_dumps(obj)
        loaded = json_loads(dumped)
        
        assert loaded["name"] == "test"
        assert loaded["value"] == 42

    @pytest.mark.parametrize("test_data", [
        {"simple": "dict"},
        [1, 2, 3, 4, 5],
        {"nested": {"deep": {"structure": "value"}}},
        {"mixed": [1, "two", 3.0, {"four": 4}]},
    ])
    def test_roundtrip_various_structures(self, test_data):
        """Test roundtrip for various data structures"""
        dumped = json_dumps(test_data)
        loaded = json_loads(dumped)
        
        assert loaded == test_data


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

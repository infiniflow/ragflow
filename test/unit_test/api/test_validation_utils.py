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
Unit tests for api.utils.validation_utils module.
"""

import pytest
from unittest.mock import Mock, AsyncMock
from uuid import UUID, uuid1
from pydantic import BaseModel, Field, ValidationError
from werkzeug.exceptions import BadRequest, UnsupportedMediaType

from api.utils.validation_utils import (
    validate_and_parse_json_request,
    validate_and_parse_request_args,
    format_validation_error_message,
    normalize_str,
    validate_uuid1_hex,
    CreateDatasetReq,
    UpdateDatasetReq,
    DeleteDatasetReq,
    ListDatasetReq,
    ParserConfig,
    RaptorConfig,
    GraphragConfig,
)


class TestNormalizeStr:
    """Test cases for normalize_str function"""

    def test_normalize_string_with_whitespace(self):
        """Test normalization of string with leading/trailing whitespace"""
        result = normalize_str("  Admin  ")
        assert result == "admin"

    def test_normalize_string_uppercase(self):
        """Test normalization converts to lowercase"""
        result = normalize_str("UPPERCASE")
        assert result == "uppercase"

    def test_normalize_mixed_case(self):
        """Test normalization of mixed case string"""
        result = normalize_str("  MiXeD CaSe  ")
        assert result == "mixed case"

    def test_normalize_empty_string(self):
        """Test normalization of empty string"""
        result = normalize_str("")
        assert result == ""

    def test_normalize_whitespace_only(self):
        """Test normalization of whitespace-only string"""
        result = normalize_str("   ")
        assert result == ""

    def test_preserve_non_string_integer(self):
        """Test that integers are preserved"""
        result = normalize_str(42)
        assert result == 42

    def test_preserve_non_string_none(self):
        """Test that None is preserved"""
        result = normalize_str(None)
        assert result is None

    def test_preserve_non_string_list(self):
        """Test that lists are preserved"""
        input_list = ["User", "Admin"]
        result = normalize_str(input_list)
        assert result == input_list

    def test_preserve_non_string_dict(self):
        """Test that dicts are preserved"""
        input_dict = {"role": "Admin"}
        result = normalize_str(input_dict)
        assert result == input_dict

    @pytest.mark.parametrize("input_val,expected", [
        ("ReadOnly", "readonly"),
        ("  ADMIN  ", "admin"),
        ("User", "user"),
        ("", ""),
        (123, 123),
        (False, False),
        (0, 0),
    ])
    def test_various_inputs(self, input_val, expected):
        """Test various input types"""
        result = normalize_str(input_val)
        assert result == expected


class TestValidateUuid1Hex:
    """Test cases for validate_uuid1_hex function"""

    def test_valid_uuid1_string(self):
        """Test validation of valid UUID1 string"""
        uuid1_obj = uuid1()
        uuid1_str = str(uuid1_obj)
        
        result = validate_uuid1_hex(uuid1_str)
        
        assert isinstance(result, str)
        assert len(result) == 32
        assert result == uuid1_obj.hex

    def test_valid_uuid1_object(self):
        """Test validation of valid UUID1 object"""
        uuid1_obj = uuid1()
        
        result = validate_uuid1_hex(uuid1_obj)
        
        assert isinstance(result, str)
        assert result == uuid1_obj.hex

    def test_uuid1_hex_no_hyphens(self):
        """Test that result has no hyphens"""
        uuid1_obj = uuid1()
        result = validate_uuid1_hex(uuid1_obj)
        
        assert "-" not in result

    def test_invalid_uuid_string_raises_error(self):
        """Test that invalid UUID string raises error"""
        from pydantic_core import PydanticCustomError
        
        with pytest.raises(PydanticCustomError) as exc_info:
            validate_uuid1_hex("not-a-uuid")
        
        assert exc_info.value.type == "invalid_UUID1_format"

    def test_non_uuid1_version_raises_error(self):
        """Test that non-UUID1 version raises error"""
        from pydantic_core import PydanticCustomError
        from uuid import uuid4
        
        uuid4_obj = uuid4()
        
        with pytest.raises(PydanticCustomError) as exc_info:
            validate_uuid1_hex(uuid4_obj)
        
        assert exc_info.value.type == "invalid_UUID1_format"

    def test_integer_input_raises_error(self):
        """Test that integer input raises error"""
        from pydantic_core import PydanticCustomError
        
        with pytest.raises(PydanticCustomError):
            validate_uuid1_hex(12345)

    def test_none_input_raises_error(self):
        """Test that None input raises error"""
        from pydantic_core import PydanticCustomError
        
        with pytest.raises(PydanticCustomError):
            validate_uuid1_hex(None)


class TestFormatValidationErrorMessage:
    """Test cases for format_validation_error_message function"""

    def test_single_validation_error(self):
        """Test formatting of single validation error"""
        class TestModel(BaseModel):
            name: str
        
        try:
            TestModel(name=123)
        except ValidationError as e:
            result = format_validation_error_message(e)
            
            assert "Field: <name>" in result
            assert "Message:" in result
            assert "Value: <123>" in result

    def test_multiple_validation_errors(self):
        """Test formatting of multiple validation errors"""
        class TestModel(BaseModel):
            name: str
            age: int
        
        try:
            TestModel(name=123, age="not_an_int")
        except ValidationError as e:
            result = format_validation_error_message(e)
            
            assert "Field: <name>" in result
            assert "Field: <age>" in result
            assert "\n" in result  # Multiple errors separated by newlines

    def test_long_value_truncation(self):
        """Test that long values are truncated"""
        class TestModel(BaseModel):
            text: str
        
        long_value = "x" * 200
        
        try:
            TestModel(text=123)  # Wrong type to trigger error
        except ValidationError as e:
            # Manually create error with long value
            pass
        
        # Create a model with max_length constraint
        class TestModel2(BaseModel):
            text: str = Field(max_length=10)
        
        try:
            TestModel2(text="x" * 200)
        except ValidationError as e:
            result = format_validation_error_message(e)
            
            # Check that value is truncated
            assert "..." in result or len(result) < 500

    def test_nested_field_error(self):
        """Test formatting of nested field errors"""
        class NestedModel(BaseModel):
            value: int
        
        class ParentModel(BaseModel):
            nested: NestedModel
        
        try:
            ParentModel(nested={"value": "not_an_int"})
        except ValidationError as e:
            result = format_validation_error_message(e)
            
            assert "nested.value" in result


class TestValidateAndParseRequestArgs:
    """Test cases for validate_and_parse_request_args function"""

    def test_valid_request_args(self):
        """Test validation of valid request arguments"""
        class TestValidator(BaseModel):
            param1: str
            param2: int = 10
        
        mock_request = Mock()
        mock_request.args.to_dict.return_value = {"param1": "value", "param2": "20"}
        
        result, error = validate_and_parse_request_args(mock_request, TestValidator)
        
        assert error is None
        assert result is not None
        assert result["param1"] == "value"
        assert result["param2"] == 20

    def test_missing_required_field(self):
        """Test validation with missing required field"""
        class TestValidator(BaseModel):
            required_field: str
        
        mock_request = Mock()
        mock_request.args.to_dict.return_value = {}
        
        result, error = validate_and_parse_request_args(mock_request, TestValidator)
        
        assert result is None
        assert error is not None
        assert "required_field" in error

    def test_with_extras_parameter(self):
        """Test validation with extras parameter"""
        class TestValidator(BaseModel):
            param1: str
            internal_id: int
        
        mock_request = Mock()
        mock_request.args.to_dict.return_value = {"param1": "value"}
        
        result, error = validate_and_parse_request_args(
            mock_request, 
            TestValidator, 
            extras={"internal_id": 123}
        )
        
        assert error is None
        assert result is not None
        assert result["param1"] == "value"
        assert "internal_id" not in result  # Extras should be removed

    def test_type_conversion(self):
        """Test that Pydantic performs type conversion"""
        class TestValidator(BaseModel):
            number: int
        
        mock_request = Mock()
        mock_request.args.to_dict.return_value = {"number": "42"}
        
        result, error = validate_and_parse_request_args(mock_request, TestValidator)
        
        assert error is None
        assert result["number"] == 42
        assert isinstance(result["number"], int)


class TestValidateAndParseJsonRequest:
    """Test cases for validate_and_parse_json_request function"""

    @pytest.mark.anyio
    async def test_valid_json_request(self):
        """Test validation of valid JSON request"""
        class TestValidator(BaseModel):
            name: str
            value: int
        
        mock_request = AsyncMock()
        mock_request.get_json = AsyncMock(return_value={"name": "test", "value": 42})
        
        result, error = await validate_and_parse_json_request(mock_request, TestValidator)
        
        assert error is None
        assert result is not None
        assert result["name"] == "test"
        assert result["value"] == 42

    @pytest.mark.anyio
    async def test_unsupported_content_type(self):
        """Test handling of unsupported content type"""
        class TestValidator(BaseModel):
            name: str
        
        mock_request = AsyncMock()
        mock_request.get_json = AsyncMock(side_effect=UnsupportedMediaType())
        mock_request.content_type = "text/xml"
        
        result, error = await validate_and_parse_json_request(mock_request, TestValidator)
        
        assert result is None
        assert error is not None
        assert "Unsupported content type" in error
        assert "text/xml" in error

    @pytest.mark.anyio
    async def test_malformed_json(self):
        """Test handling of malformed JSON"""
        class TestValidator(BaseModel):
            name: str
        
        mock_request = AsyncMock()
        mock_request.get_json = AsyncMock(side_effect=BadRequest())
        
        result, error = await validate_and_parse_json_request(mock_request, TestValidator)
        
        assert result is None
        assert error is not None
        assert "Malformed JSON syntax" in error

    @pytest.mark.anyio
    async def test_invalid_payload_type(self):
        """Test handling of non-dict payload"""
        class TestValidator(BaseModel):
            name: str
        
        mock_request = AsyncMock()
        mock_request.get_json = AsyncMock(return_value=["not", "a", "dict"])
        
        result, error = await validate_and_parse_json_request(mock_request, TestValidator)
        
        assert result is None
        assert error is not None
        assert "Invalid request payload" in error
        assert "list" in error

    @pytest.mark.anyio
    async def test_validation_error(self):
        """Test handling of Pydantic validation errors"""
        class TestValidator(BaseModel):
            name: str
            age: int
        
        mock_request = AsyncMock()
        mock_request.get_json = AsyncMock(return_value={"name": 123, "age": "not_int"})
        
        result, error = await validate_and_parse_json_request(mock_request, TestValidator)
        
        assert result is None
        assert error is not None
        assert "Field:" in error

    @pytest.mark.anyio
    async def test_with_extras_parameter(self):
        """Test validation with extras parameter"""
        class TestValidator(BaseModel):
            name: str
            user_id: str
        
        mock_request = AsyncMock()
        mock_request.get_json = AsyncMock(return_value={"name": "test"})
        
        result, error = await validate_and_parse_json_request(
            mock_request,
            TestValidator,
            extras={"user_id": "user_123"}
        )
        
        assert error is None
        assert result is not None
        assert result["name"] == "test"
        assert "user_id" not in result  # Extras should be removed

    @pytest.mark.anyio
    async def test_exclude_unset_parameter(self):
        """Test exclude_unset parameter"""
        class TestValidator(BaseModel):
            name: str
            optional: str = "default"
        
        mock_request = AsyncMock()
        mock_request.get_json = AsyncMock(return_value={"name": "test"})
        
        result, error = await validate_and_parse_json_request(
            mock_request,
            TestValidator,
            exclude_unset=True
        )
        
        assert error is None
        assert result is not None
        assert "name" in result
        assert "optional" not in result  # Not set, should be excluded


class TestCreateDatasetReq:
    """Test cases for CreateDatasetReq validation"""

    def test_valid_dataset_creation(self):
        """Test valid dataset creation request"""
        data = {
            "name": "Test Dataset",
            "embedding_model": "text-embedding-3-large@openai"
        }
        
        dataset = CreateDatasetReq(**data)
        
        assert dataset.name == "Test Dataset"
        assert dataset.embedding_model == "text-embedding-3-large@openai"

    def test_name_whitespace_stripping(self):
        """Test that name whitespace is stripped"""
        data = {
            "name": "  Test Dataset  "
        }
        
        dataset = CreateDatasetReq(**data)
        
        assert dataset.name == "Test Dataset"

    def test_empty_name_raises_error(self):
        """Test that empty name raises error"""
        with pytest.raises(ValidationError):
            CreateDatasetReq(name="")

    def test_invalid_embedding_model_format(self):
        """Test that invalid embedding model format raises error"""
        with pytest.raises(ValidationError) as exc_info:
            CreateDatasetReq(
                name="Test",
                embedding_model="invalid_model"
            )
        
        assert "format_invalid" in str(exc_info.value)

    def test_embedding_model_without_provider(self):
        """Test embedding model without provider raises error"""
        with pytest.raises(ValidationError):
            CreateDatasetReq(
                name="Test",
                embedding_model="model_name@"
            )

    def test_valid_avatar_base64(self):
        """Test valid base64 avatar"""
        data = {
            "name": "Test",
            "avatar": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
        }
        
        dataset = CreateDatasetReq(**data)
        
        assert dataset.avatar.startswith("data:image/png")

    def test_invalid_avatar_mime_type(self):
        """Test invalid avatar MIME type raises error"""
        with pytest.raises(ValidationError):
            CreateDatasetReq(
                name="Test",
                avatar="data:video/mp4;base64,abc123"
            )

    def test_avatar_missing_data_prefix(self):
        """Test avatar missing data: prefix raises error"""
        with pytest.raises(ValidationError):
            CreateDatasetReq(
                name="Test",
                avatar="image/png;base64,abc123"
            )

    def test_default_chunk_method(self):
        """Test default chunk_method is set to 'naive'"""
        dataset = CreateDatasetReq(name="Test")
        
        assert dataset.chunk_method == "naive"

    def test_valid_chunk_methods(self):
        """Test various valid chunk methods"""
        valid_methods = ["naive", "book", "email", "laws", "manual", "one", 
                        "paper", "picture", "presentation", "qa", "table", "tag"]
        
        for method in valid_methods:
            dataset = CreateDatasetReq(name="Test", chunk_method=method)
            assert dataset.chunk_method == method

    def test_invalid_chunk_method(self):
        """Test invalid chunk method raises error"""
        with pytest.raises(ValidationError):
            CreateDatasetReq(name="Test", chunk_method="invalid_method")

    def test_pipeline_id_validation(self):
        """Test pipeline_id validation"""
        # Valid 32-char hex
        dataset = CreateDatasetReq(
            name="Test",
            parse_type=1,
            pipeline_id="a" * 32
        )
        assert dataset.pipeline_id == "a" * 32

    def test_pipeline_id_wrong_length(self):
        """Test pipeline_id with wrong length raises error"""
        with pytest.raises(ValidationError):
            CreateDatasetReq(
                name="Test",
                parse_type=1,
                pipeline_id="abc"  # Too short
            )

    def test_pipeline_id_non_hex(self):
        """Test pipeline_id with non-hex characters raises error"""
        with pytest.raises(ValidationError):
            CreateDatasetReq(
                name="Test",
                parse_type=1,
                pipeline_id="g" * 32  # 'g' is not hex
            )


class TestUpdateDatasetReq:
    """Test cases for UpdateDatasetReq validation"""

    def test_valid_update_request(self):
        """Test valid dataset update request"""
        uuid1_obj = uuid1()
        
        data = {
            "dataset_id": str(uuid1_obj),
            "name": "Updated Dataset"
        }
        
        dataset = UpdateDatasetReq(**data)
        
        assert dataset.dataset_id == uuid1_obj.hex
        assert dataset.name == "Updated Dataset"

    def test_dataset_id_uuid1_validation(self):
        """Test that dataset_id must be UUID1"""
        from uuid import uuid4
        
        with pytest.raises(ValidationError):
            UpdateDatasetReq(
                dataset_id=str(uuid4()),
                name="Test"
            )

    def test_pagerank_validation(self):
        """Test pagerank field validation"""
        uuid1_obj = uuid1()
        
        dataset = UpdateDatasetReq(
            dataset_id=str(uuid1_obj),
            name="Test",
            pagerank=50
        )
        
        assert dataset.pagerank == 50

    def test_pagerank_out_of_range(self):
        """Test pagerank out of range raises error"""
        uuid1_obj = uuid1()
        
        with pytest.raises(ValidationError):
            UpdateDatasetReq(
                dataset_id=str(uuid1_obj),
                name="Test",
                pagerank=101  # Max is 100
            )


class TestDeleteDatasetReq:
    """Test cases for DeleteDatasetReq validation"""

    def test_valid_delete_request(self):
        """Test valid delete request"""
        uuid1_obj1 = uuid1()
        uuid1_obj2 = uuid1()
        
        req = DeleteDatasetReq(ids=[str(uuid1_obj1), str(uuid1_obj2)])
        
        assert len(req.ids) == 2
        assert uuid1_obj1.hex in req.ids
        assert uuid1_obj2.hex in req.ids

    def test_duplicate_ids_raises_error(self):
        """Test that duplicate IDs raise error"""
        uuid1_obj = uuid1()
        
        with pytest.raises(ValidationError) as exc_info:
            DeleteDatasetReq(ids=[str(uuid1_obj), str(uuid1_obj)])
        
        assert "duplicate" in str(exc_info.value).lower()

    def test_empty_ids_list(self):
        """Test empty IDs list"""
        req = DeleteDatasetReq(ids=[])
        
        assert req.ids == []

    def test_none_ids(self):
        """Test None IDs"""
        req = DeleteDatasetReq(ids=None)
        
        assert req.ids is None


class TestListDatasetReq:
    """Test cases for ListDatasetReq validation"""

    def test_default_values(self):
        """Test default values for list request"""
        req = ListDatasetReq()
        
        assert req.page == 1
        assert req.page_size == 30
        assert req.orderby == "create_time"
        assert req.desc is True

    def test_custom_pagination(self):
        """Test custom pagination values"""
        req = ListDatasetReq(page=2, page_size=50)
        
        assert req.page == 2
        assert req.page_size == 50

    def test_page_minimum_value(self):
        """Test page minimum value validation"""
        with pytest.raises(ValidationError):
            ListDatasetReq(page=0)

    def test_valid_orderby_values(self):
        """Test valid orderby values"""
        req1 = ListDatasetReq(orderby="create_time")
        req2 = ListDatasetReq(orderby="update_time")
        
        assert req1.orderby == "create_time"
        assert req2.orderby == "update_time"

    def test_invalid_orderby_value(self):
        """Test invalid orderby value raises error"""
        with pytest.raises(ValidationError):
            ListDatasetReq(orderby="invalid_field")


class TestParserConfig:
    """Test cases for ParserConfig validation"""

    def test_default_parser_config(self):
        """Test default parser configuration"""
        config = ParserConfig()
        
        assert config.chunk_token_num == 512
        assert config.auto_keywords == 0
        assert config.auto_questions == 0

    def test_custom_parser_config(self):
        """Test custom parser configuration"""
        config = ParserConfig(
            chunk_token_num=1024,
            auto_keywords=5,
            auto_questions=3
        )
        
        assert config.chunk_token_num == 1024
        assert config.auto_keywords == 5
        assert config.auto_questions == 3

    def test_chunk_token_num_range(self):
        """Test chunk_token_num range validation"""
        with pytest.raises(ValidationError):
            ParserConfig(chunk_token_num=3000)  # Max is 2048

    def test_raptor_config_integration(self):
        """Test raptor config integration"""
        config = ParserConfig(
            raptor=RaptorConfig(use_raptor=True, max_token=512)
        )
        
        assert config.raptor.use_raptor is True
        assert config.raptor.max_token == 512


class TestRaptorConfig:
    """Test cases for RaptorConfig validation"""

    def test_default_raptor_config(self):
        """Test default raptor configuration"""
        config = RaptorConfig()
        
        assert config.use_raptor is False
        assert config.max_token == 256
        assert config.threshold == 0.1

    def test_custom_raptor_config(self):
        """Test custom raptor configuration"""
        config = RaptorConfig(
            use_raptor=True,
            max_token=512,
            threshold=0.2
        )
        
        assert config.use_raptor is True
        assert config.max_token == 512
        assert config.threshold == 0.2

    def test_threshold_range(self):
        """Test threshold range validation"""
        with pytest.raises(ValidationError):
            RaptorConfig(threshold=1.5)  # Max is 1.0


class TestGraphragConfig:
    """Test cases for GraphragConfig validation"""

    def test_default_graphrag_config(self):
        """Test default graphrag configuration"""
        config = GraphragConfig()
        
        assert config.use_graphrag is False
        assert config.method == "light"
        assert "organization" in config.entity_types

    def test_custom_graphrag_config(self):
        """Test custom graphrag configuration"""
        config = GraphragConfig(
            use_graphrag=True,
            method="general",
            entity_types=["person", "location"]
        )
        
        assert config.use_graphrag is True
        assert config.method == "general"
        assert config.entity_types == ["person", "location"]

    def test_invalid_method(self):
        """Test invalid method raises error"""
        with pytest.raises(ValidationError):
            GraphragConfig(method="invalid")


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

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
from collections import Counter
from enum import auto
from typing import Annotated, Any

from flask import Request
from pydantic import UUID1, BaseModel, Field, StringConstraints, ValidationError, field_serializer, field_validator
from pydantic_core import PydanticCustomError
from strenum import StrEnum
from werkzeug.exceptions import BadRequest, UnsupportedMediaType

from api.constants import DATASET_NAME_LIMIT


def validate_and_parse_json_request(request: Request, validator: type[BaseModel], *, extras: dict[str, Any] | None = None, exclude_unset: bool = False) -> tuple[dict[str, Any] | None, str | None]:
    """
    Validates and parses JSON requests through a multi-stage validation pipeline.

    Implements a four-stage validation process:
    1. Content-Type verification (must be application/json)
    2. JSON syntax validation
    3. Payload structure type checking
    4. Pydantic model validation with error formatting

    Args:
        request (Request): Flask request object containing HTTP payload
        validator (type[BaseModel]): Pydantic model class for data validation
        extras (dict[str, Any] | None): Additional fields to merge into payload
            before validation. These fields will be removed from the final output
        exclude_unset (bool): Whether to exclude fields that have not been explicitly set

    Returns:
        tuple[Dict[str, Any] | None, str | None]:
        - First element:
            - Validated dictionary on success
            - None on validation failure
        - Second element:
            - None on success
            - Diagnostic error message on failure

    Raises:
        UnsupportedMediaType: When Content-Type header is not application/json
        BadRequest: For structural JSON syntax errors
        ValidationError: When payload violates Pydantic schema rules

    Examples:
        >>> validate_and_parse_json_request(valid_request, DatasetSchema)
        ({"name": "Dataset1", "format": "csv"}, None)

        >>> validate_and_parse_json_request(xml_request, DatasetSchema)
        (None, "Unsupported content type: Expected application/json, got text/xml")

        >>> validate_and_parse_json_request(bad_json_request, DatasetSchema)
        (None, "Malformed JSON syntax: Missing commas/brackets or invalid encoding")

    Notes:
        1. Validation Priority:
            - Content-Type verification precedes JSON parsing
            - Structural validation occurs before schema validation
        2. Extra fields added via `extras` parameter are automatically removed
           from the final output after validation
    """
    try:
        payload = request.get_json() or {}
    except UnsupportedMediaType:
        return None, f"Unsupported content type: Expected application/json, got {request.content_type}"
    except BadRequest:
        return None, "Malformed JSON syntax: Missing commas/brackets or invalid encoding"

    if not isinstance(payload, dict):
        return None, f"Invalid request payload: expected object, got {type(payload).__name__}"

    try:
        if extras is not None:
            payload.update(extras)
        validated_request = validator(**payload)
    except ValidationError as e:
        return None, format_validation_error_message(e)

    parsed_payload = validated_request.model_dump(by_alias=True, exclude_unset=exclude_unset)

    if extras is not None:
        for key in list(parsed_payload.keys()):
            if key in extras:
                del parsed_payload[key]

    return parsed_payload, None


def format_validation_error_message(e: ValidationError) -> str:
    """
    Formats validation errors into a standardized string format.

    Processes pydantic ValidationError objects to create human-readable error messages
    containing field locations, error descriptions, and input values.

    Args:
        e (ValidationError): The validation error instance containing error details

    Returns:
        str: Formatted error messages joined by newlines. Each line contains:
            - Field path (dot-separated)
            - Error message
            - Truncated input value (max 128 chars)

    Example:
        >>> try:
        ...     UserModel(name=123, email="invalid")
        ... except ValidationError as e:
        ...     print(format_validation_error_message(e))
        Field: <name> - Message: <Input should be a valid string> - Value: <123>
        Field: <email> - Message: <value is not a valid email address> - Value: <invalid>
    """
    error_messages = []

    for error in e.errors():
        field = ".".join(map(str, error["loc"]))
        msg = error["msg"]
        input_val = error["input"]
        input_str = str(input_val)

        if len(input_str) > 128:
            input_str = input_str[:125] + "..."

        error_msg = f"Field: <{field}> - Message: <{msg}> - Value: <{input_str}>"
        error_messages.append(error_msg)

    return "\n".join(error_messages)


class PermissionEnum(StrEnum):
    me = auto()
    team = auto()


class ChunkMethodnEnum(StrEnum):
    naive = auto()
    book = auto()
    email = auto()
    laws = auto()
    manual = auto()
    one = auto()
    paper = auto()
    picture = auto()
    presentation = auto()
    qa = auto()
    table = auto()
    tag = auto()


class GraphragMethodEnum(StrEnum):
    light = auto()
    general = auto()


class Base(BaseModel):
    class Config:
        extra = "forbid"


class RaptorConfig(Base):
    use_raptor: bool = Field(default=False)
    prompt: Annotated[
        str,
        StringConstraints(strip_whitespace=True, min_length=1),
        Field(
            default="Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize."
        ),
    ]
    max_token: int = Field(default=256, ge=1, le=2048)
    threshold: float = Field(default=0.1, ge=0.0, le=1.0)
    max_cluster: int = Field(default=64, ge=1, le=1024)
    random_seed: int = Field(default=0, ge=0)


class GraphragConfig(Base):
    use_graphrag: bool = Field(default=False)
    entity_types: list[str] = Field(default_factory=lambda: ["organization", "person", "geo", "event", "category"])
    method: GraphragMethodEnum = Field(default=GraphragMethodEnum.light)
    community: bool = Field(default=False)
    resolution: bool = Field(default=False)


class ParserConfig(Base):
    auto_keywords: int = Field(default=0, ge=0, le=32)
    auto_questions: int = Field(default=0, ge=0, le=10)
    chunk_token_num: int = Field(default=128, ge=1, le=2048)
    delimiter: str = Field(default=r"\n", min_length=1)
    graphrag: GraphragConfig | None = None
    html4excel: bool = False
    layout_recognize: str = "DeepDOC"
    raptor: RaptorConfig | None = None
    tag_kb_ids: list[str] = Field(default_factory=list)
    topn_tags: int = Field(default=1, ge=1, le=10)
    filename_embd_weight: float | None = Field(default=None, ge=0.0, le=1.0)
    task_page_size: int | None = Field(default=None, ge=1)
    pages: list[list[int]] | None = None


class CreateDatasetReq(Base):
    name: Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=DATASET_NAME_LIMIT), Field(...)]
    avatar: str | None = Field(default=None, max_length=65535)
    description: str | None = Field(default=None, max_length=65535)
    embedding_model: Annotated[str, StringConstraints(strip_whitespace=True, max_length=255), Field(default="", serialization_alias="embd_id")]
    permission: Annotated[PermissionEnum, StringConstraints(strip_whitespace=True, min_length=1, max_length=16), Field(default=PermissionEnum.me)]
    chunk_method: Annotated[ChunkMethodnEnum, StringConstraints(strip_whitespace=True, min_length=1, max_length=32), Field(default=ChunkMethodnEnum.naive, serialization_alias="parser_id")]
    pagerank: int = Field(default=0, ge=0, le=100)
    parser_config: ParserConfig | None = Field(default=None)

    @field_validator("avatar")
    @classmethod
    def validate_avatar_base64(cls, v: str | None) -> str | None:
        """
        Validates Base64-encoded avatar string format and MIME type compliance.

        Implements a three-stage validation workflow:
        1. MIME prefix existence check
        2. MIME type format validation
        3. Supported type verification

        Args:
            v (str): Raw avatar field value

        Returns:
            str: Validated Base64 string

        Raises:
            PydanticCustomError: For structural errors in these cases:
                - Missing MIME prefix header
                - Invalid MIME prefix format
                - Unsupported image MIME type

        Example:
            ```python
            # Valid case
            CreateDatasetReq(avatar="data:image/png;base64,iVBORw0KGg...")

            # Invalid cases
            CreateDatasetReq(avatar="image/jpeg;base64,...")  # Missing 'data:' prefix
            CreateDatasetReq(avatar="data:video/mp4;base64,...")  # Unsupported MIME type
            ```
        """
        if v is None:
            return v

        if "," in v:
            prefix, _ = v.split(",", 1)
            if not prefix.startswith("data:"):
                raise PydanticCustomError("format_invalid", "Invalid MIME prefix format. Must start with 'data:'")

            mime_type = prefix[5:].split(";")[0]
            supported_mime_types = ["image/jpeg", "image/png"]
            if mime_type not in supported_mime_types:
                raise PydanticCustomError("format_invalid", "Unsupported MIME type. Allowed: {supported_mime_types}", {"supported_mime_types": supported_mime_types})

            return v
        else:
            raise PydanticCustomError("format_invalid", "Missing MIME prefix. Expected format: data:<mime>;base64,<data>")

    @field_validator("embedding_model", mode="after")
    @classmethod
    def validate_embedding_model(cls, v: str) -> str:
        """
        Validates embedding model identifier format compliance.

        Validation pipeline:
        1. Structural format verification
        2. Component non-empty check
        3. Value normalization

        Args:
            v (str): Raw model identifier

        Returns:
            str: Validated <model_name>@<provider> format

        Raises:
            PydanticCustomError: For these violations:
                - Missing @ separator
                - Empty model_name/provider
                - Invalid component structure

        Examples:
            Valid: "text-embedding-3-large@openai"
            Invalid: "invalid_model" (no @)
            Invalid: "@openai" (empty model_name)
            Invalid: "text-embedding-3-large@" (empty provider)
        """
        if "@" not in v:
            raise PydanticCustomError("format_invalid", "Embedding model identifier must follow <model_name>@<provider> format")

        components = v.split("@", 1)
        if len(components) != 2 or not all(components):
            raise PydanticCustomError("format_invalid", "Both model_name and provider must be non-empty strings")

        model_name, provider = components
        if not model_name.strip() or not provider.strip():
            raise PydanticCustomError("format_invalid", "Model name and provider cannot be whitespace-only strings")
        return v

    @field_validator("permission", mode="before")
    @classmethod
    def permission_auto_lowercase(cls, v: Any) -> Any:
        """
        Normalize permission input to lowercase for consistent PermissionEnum matching.

        Args:
            v (Any): Raw input value for the permission field

        Returns:
            Lowercase string if input is string type, otherwise returns original value

        Behavior:
            - Converts string inputs to lowercase (e.g., "ME" → "me")
            - Non-string values pass through unchanged
            - Works in validation pre-processing stage (before enum conversion)
        """
        return v.lower() if isinstance(v, str) else v

    @field_validator("parser_config", mode="before")
    @classmethod
    def normalize_empty_parser_config(cls, v: Any) -> Any:
        """
        Normalizes empty parser configuration by converting empty dictionaries to None.

        This validator ensures consistent handling of empty parser configurations across
        the application by converting empty dicts to None values.

        Args:
            v (Any): Raw input value for the parser config field

        Returns:
            Any: Returns None if input is an empty dict, otherwise returns the original value

        Example:
            >>> normalize_empty_parser_config({})
            None

            >>> normalize_empty_parser_config({"key": "value"})
            {"key": "value"}
        """
        if v == {}:
            return None
        return v

    @field_validator("parser_config", mode="after")
    @classmethod
    def validate_parser_config_json_length(cls, v: ParserConfig | None) -> ParserConfig | None:
        """
        Validates serialized JSON length constraints for parser configuration.

        Implements a two-stage validation workflow:
        1. Null check - bypass validation for empty configurations
        2. Model serialization - convert Pydantic model to JSON string
        3. Size verification - enforce maximum allowed payload size

        Args:
            v (ParserConfig | None): Raw parser configuration object

        Returns:
            ParserConfig | None: Validated configuration object

        Raises:
            PydanticCustomError: When serialized JSON exceeds 65,535 characters
        """
        if v is None:
            return None

        if (json_str := v.model_dump_json()) and len(json_str) > 65535:
            raise PydanticCustomError("string_too_long", "Parser config exceeds size limit (max 65,535 characters). Current size: {actual}", {"actual": len(json_str)})
        return v


class UpdateDatasetReq(CreateDatasetReq):
    dataset_id: UUID1 = Field(...)
    name: Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=DATASET_NAME_LIMIT), Field(default="")]

    @field_serializer("dataset_id")
    def serialize_uuid_to_hex(self, v: uuid.UUID) -> str:
        """
        Serializes a UUID version 1 object to its hexadecimal string representation.

        This field serializer specifically handles UUID version 1 objects, converting them
        to their canonical 32-character hexadecimal format without hyphens. The conversion
        is designed for consistent serialization in API responses and database storage.

        Args:
            v (uuid.UUID1): The UUID version 1 object to serialize. Must be a valid
                        UUID1 instance generated by Python's uuid module.

        Returns:
            str: 32-character lowercase hexadecimal string representation
                Example: "550e8400e29b41d4a716446655440000"

        Raises:
            AttributeError: If input is not a proper UUID object (missing hex attribute)
            TypeError: If input is not a UUID1 instance (when type checking is enabled)

        Notes:
            - Version 1 UUIDs contain timestamp and MAC address information
            - The .hex property automatically converts to lowercase hexadecimal
            - For cross-version compatibility, consider typing as uuid.UUID instead
        """
        return v.hex


class DeleteReq(Base):
    ids: list[UUID1] | None = Field(...)

    @field_validator("ids", mode="after")
    def check_duplicate_ids(cls, v: list[UUID1] | None) -> list[str] | None:
        """
        Validates and converts a list of UUID1 objects to hexadecimal strings while checking for duplicates.

        This validator implements a three-stage processing pipeline:
        1. Null Handling - returns None for empty/null input
        2. UUID Conversion - transforms UUID objects to hex strings
        3. Duplicate Validation - ensures all IDs are unique

        Behavior Specifications:
        - Input: None → Returns None (indicates no operation)
        - Input: [] → Returns [] (empty list for explicit no-op)
        - Input: [UUID1,...] → Returns validated hex strings
        - Duplicates: Raises formatted PydanticCustomError

        Args:
            v (list[UUID1] | None):
                - None: Indicates no datasets should be processed
                - Empty list: Explicit empty operation
                - Populated list: Dataset UUIDs to validate/convert

        Returns:
            list[str] | None:
                - None when input is None
                - List of 32-character hex strings (lowercase, no hyphens)
                Example: ["550e8400e29b41d4a716446655440000"]

        Raises:
            PydanticCustomError: When duplicates detected, containing:
                - Error type: "duplicate_uuids"
                - Template message: "Duplicate ids: '{duplicate_ids}'"
                - Context: {"duplicate_ids": "id1, id2, ..."}

        Example:
            >>> validate([UUID("..."), UUID("...")])
            ["2cdf0456e9a711ee8000000000000000", ...]

            >>> validate([UUID("..."), UUID("...")])  # Duplicates
            PydanticCustomError: Duplicate ids: '2cdf0456e9a711ee8000000000000000'
        """
        if not v:
            return v

        uuid_hex_list = [ids.hex for ids in v]
        duplicates = [item for item, count in Counter(uuid_hex_list).items() if count > 1]

        if duplicates:
            duplicates_str = ", ".join(duplicates)
            raise PydanticCustomError("duplicate_uuids", "Duplicate ids: '{duplicate_ids}'", {"duplicate_ids": duplicates_str})

        return uuid_hex_list


class DeleteDatasetReq(DeleteReq): ...

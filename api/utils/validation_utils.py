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
from collections import Counter
from typing import Annotated, Any, Literal
from uuid import UUID

from flask import Request
from pydantic import (
    BaseModel,
    ConfigDict,
    Field,
    StringConstraints,
    ValidationError,
    field_validator,
)
from pydantic_core import PydanticCustomError
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


def validate_and_parse_request_args(request: Request, validator: type[BaseModel], *, extras: dict[str, Any] | None = None) -> tuple[dict[str, Any] | None, str | None]:
    """
    Validates and parses request arguments against a Pydantic model.

    This function performs a complete request validation workflow:
    1. Extracts query parameters from the request
    2. Merges with optional extra values (if provided)
    3. Validates against the specified Pydantic model
    4. Cleans the output by removing extra values
    5. Returns either parsed data or an error message

    Args:
        request (Request): Web framework request object containing query parameters
        validator (type[BaseModel]): Pydantic model class for validation
        extras (dict[str, Any] | None): Optional additional values to include in validation
                                      but exclude from final output. Defaults to None.

    Returns:
        tuple[dict[str, Any] | None, str | None]:
            - First element: Validated/parsed arguments as dict if successful, None otherwise
            - Second element: Formatted error message if validation failed, None otherwise

    Behavior:
        - Query parameters are merged with extras before validation
        - Extras are automatically removed from the final output
        - All validation errors are formatted into a human-readable string

    Raises:
        TypeError: If validator is not a Pydantic BaseModel subclass

    Examples:
        Successful validation:
            >>> validate_and_parse_request_args(request, MyValidator)
            ({'param1': 'value'}, None)

        Failed validation:
            >>> validate_and_parse_request_args(request, MyValidator)
            (None, "param1: Field required")

        With extras:
            >>> validate_and_parse_request_args(request, MyValidator, extras={'internal_id': 123})
            ({'param1': 'value'}, None)  # internal_id removed from output

    Notes:
        - Uses request.args.to_dict() for Flask-compatible parameter extraction
        - Maintains immutability of original request arguments
        - Preserves type conversion from Pydantic validation
    """
    args = request.args.to_dict(flat=True)
    try:
        if extras is not None:
            args.update(extras)
        validated_args = validator(**args)
    except ValidationError as e:
        return None, format_validation_error_message(e)

    parsed_args = validated_args.model_dump()
    if extras is not None:
        for key in list(parsed_args.keys()):
            if key in extras:
                del parsed_args[key]

    return parsed_args, None


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


def normalize_str(v: Any) -> Any:
    """
    Normalizes string values to a standard format while preserving non-string inputs.

    Performs the following transformations when input is a string:
    1. Trims leading/trailing whitespace (str.strip())
    2. Converts to lowercase (str.lower())

    Non-string inputs are returned unchanged, making this function safe for mixed-type
    processing pipelines.

    Args:
        v (Any): Input value to normalize. Accepts any Python object.

    Returns:
        Any: Normalized string if input was string-type, original value otherwise.

    Behavior Examples:
        String Input: "  Admin " → "admin"
        Empty String: "   " → "" (empty string)
        Non-String:
            - 123 → 123
            - None → None
            - ["User"] → ["User"]

    Typical Use Cases:
        - Standardizing user input
        - Preparing data for case-insensitive comparison
        - Cleaning API parameters
        - Normalizing configuration values

    Edge Cases:
        - Unicode whitespace is handled by str.strip()
        - Locale-independent lowercasing (str.lower())
        - Preserves falsy values (0, False, etc.)

    Example:
        >>> normalize_str("  ReadOnly  ")
        'readonly'
        >>> normalize_str(42)
        42
    """
    if isinstance(v, str):
        stripped = v.strip()
        normalized = stripped.lower()
        return normalized
    return v


def validate_uuid1_hex(v: Any) -> str:
    """
    Validates and converts input to a UUID version 1 hexadecimal string.

    This function performs strict validation and normalization:
    1. Accepts either UUID objects or UUID-formatted strings
    2. Verifies the UUID is version 1 (time-based)
    3. Returns the 32-character hexadecimal representation

    Args:
        v (Any): Input value to validate. Can be:
                - UUID object (must be version 1)
                - String in UUID format (e.g. "550e8400-e29b-41d4-a716-446655440000")

    Returns:
        str: 32-character lowercase hexadecimal string without hyphens
             Example: "550e8400e29b41d4a716446655440000"

    Raises:
        PydanticCustomError: With code "invalid_UUID1_format" when:
            - Input is not a UUID object or valid UUID string
            - UUID version is not 1
            - String doesn't match UUID format

    Examples:
        Valid cases:
            >>> validate_uuid1_hex("550e8400-e29b-41d4-a716-446655440000")
            '550e8400e29b41d4a716446655440000'
            >>> validate_uuid1_hex(UUID('550e8400-e29b-41d4-a716-446655440000'))
            '550e8400e29b41d4a716446655440000'

        Invalid cases:
            >>> validate_uuid1_hex("not-a-uuid")  # raises PydanticCustomError
            >>> validate_uuid1_hex(12345)  # raises PydanticCustomError
            >>> validate_uuid1_hex(UUID(int=0))  # v4, raises PydanticCustomError

    Notes:
        - Uses Python's built-in UUID parser for format validation
        - Version check prevents accidental use of other UUID versions
        - Hyphens in input strings are automatically removed in output
    """
    try:
        uuid_obj = UUID(v) if isinstance(v, str) else v
        if uuid_obj.version != 1:
            raise PydanticCustomError("invalid_UUID1_format", "Must be a UUID1 format")
        return uuid_obj.hex
    except (AttributeError, ValueError, TypeError):
        raise PydanticCustomError("invalid_UUID1_format", "Invalid UUID1 format")


class Base(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)


class RaptorConfig(Base):
    use_raptor: Annotated[bool, Field(default=False)]
    prompt: Annotated[
        str,
        StringConstraints(strip_whitespace=True, min_length=1),
        Field(
            default="Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize."
        ),
    ]
    max_token: Annotated[int, Field(default=256, ge=1, le=2048)]
    threshold: Annotated[float, Field(default=0.1, ge=0.0, le=1.0)]
    max_cluster: Annotated[int, Field(default=64, ge=1, le=1024)]
    random_seed: Annotated[int, Field(default=0, ge=0)]


class GraphragConfig(Base):
    use_graphrag: Annotated[bool, Field(default=False)]
    entity_types: Annotated[list[str], Field(default_factory=lambda: ["organization", "person", "geo", "event", "category"])]
    method: Annotated[Literal["light", "general"], Field(default="light")]
    community: Annotated[bool, Field(default=False)]
    resolution: Annotated[bool, Field(default=False)]


class ParserConfig(Base):
    auto_keywords: Annotated[int, Field(default=0, ge=0, le=32)]
    auto_questions: Annotated[int, Field(default=0, ge=0, le=10)]
    chunk_token_num: Annotated[int, Field(default=512, ge=1, le=2048)]
    delimiter: Annotated[str, Field(default=r"\n", min_length=1)]
    graphrag: Annotated[GraphragConfig, Field(default_factory=lambda: GraphragConfig(use_graphrag=False))]
    html4excel: Annotated[bool, Field(default=False)]
    layout_recognize: Annotated[str, Field(default="DeepDOC")]
    raptor: Annotated[RaptorConfig, Field(default_factory=lambda: RaptorConfig(use_raptor=False))]
    tag_kb_ids: Annotated[list[str], Field(default_factory=list)]
    topn_tags: Annotated[int, Field(default=1, ge=1, le=10)]
    filename_embd_weight: Annotated[float | None, Field(default=0.1, ge=0.0, le=1.0)]
    task_page_size: Annotated[int | None, Field(default=None, ge=1)]
    pages: Annotated[list[list[int]] | None, Field(default=None)]


class CreateDatasetReq(Base):
    name: Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=DATASET_NAME_LIMIT), Field(...)]
    avatar: Annotated[str | None, Field(default=None, max_length=65535)]
    description: Annotated[str | None, Field(default=None, max_length=65535)]
    embedding_model: Annotated[str | None, Field(default=None, max_length=255, serialization_alias="embd_id")]
    permission: Annotated[Literal["me", "team"], Field(default="me", min_length=1, max_length=16)]
    chunk_method: Annotated[
        Literal["naive", "book", "email", "laws", "manual", "one", "paper", "picture", "presentation", "qa", "table", "tag"],
        Field(default="naive", min_length=1, max_length=32, serialization_alias="parser_id"),
    ]
    parser_config: Annotated[ParserConfig | None, Field(default=None)]

    @field_validator("avatar", mode="after")
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

    @field_validator("embedding_model", mode="before")
    @classmethod
    def normalize_embedding_model(cls, v: Any) -> Any:
        """Normalize embedding model string by stripping whitespace"""
        if isinstance(v, str):
            return v.strip()
        return v

    @field_validator("embedding_model", mode="after")
    @classmethod
    def validate_embedding_model(cls, v: str | None) -> str | None:
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
        if isinstance(v, str):
            if "@" not in v:
                raise PydanticCustomError("format_invalid", "Embedding model identifier must follow <model_name>@<provider> format")

            components = v.split("@", 1)
            if len(components) != 2 or not all(components):
                raise PydanticCustomError("format_invalid", "Both model_name and provider must be non-empty strings")

            model_name, provider = components
            if not model_name.strip() or not provider.strip():
                raise PydanticCustomError("format_invalid", "Model name and provider cannot be whitespace-only strings")
        return v

    # @field_validator("permission", mode="before")
    # @classmethod
    # def normalize_permission(cls, v: Any) -> Any:
    #     return normalize_str(v)

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
    dataset_id: Annotated[str, Field(...)]
    name: Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=DATASET_NAME_LIMIT), Field(default="")]
    pagerank: Annotated[int, Field(default=0, ge=0, le=100)]

    @field_validator("dataset_id", mode="before")
    @classmethod
    def validate_dataset_id(cls, v: Any) -> str:
        return validate_uuid1_hex(v)


class DeleteReq(Base):
    ids: Annotated[list[str] | None, Field(...)]

    @field_validator("ids", mode="after")
    @classmethod
    def validate_ids(cls, v_list: list[str] | None) -> list[str] | None:
        """
        Validates and normalizes a list of UUID strings with None handling.

        This post-processing validator performs:
        1. None input handling (pass-through)
        2. UUID version 1 validation for each list item
        3. Duplicate value detection
        4. Returns normalized UUID hex strings or None

        Args:
            v_list (list[str] | None): Input list that has passed initial validation.
                                    Either a list of UUID strings or None.

        Returns:
            list[str] | None:
            - None if input was None
            - List of normalized UUID hex strings otherwise:
            * 32-character lowercase
            * Valid UUID version 1
            * Unique within list

        Raises:
            PydanticCustomError: With structured error details when:
                - "invalid_UUID1_format": Any string fails UUIDv1 validation
                - "duplicate_uuids": If duplicate IDs are detected

        Validation Rules:
            - None input returns None
            - Empty list returns empty list
            - All non-None items must be valid UUIDv1
            - No duplicates permitted
            - Original order preserved

        Examples:
            Valid cases:
                >>> validate_ids(None)
                None
                >>> validate_ids([])
                []
                >>> validate_ids(["550e8400-e29b-41d4-a716-446655440000"])
                ["550e8400e29b41d4a716446655440000"]

            Invalid cases:
                >>> validate_ids(["invalid"])
                # raises PydanticCustomError(invalid_UUID1_format)
                >>> validate_ids(["550e...", "550e..."])
                # raises PydanticCustomError(duplicate_uuids)

        Security Notes:
            - Validates UUID version to prevent version spoofing
            - Duplicate check prevents data injection
            - None handling maintains pipeline integrity
        """
        if v_list is None:
            return None

        ids_list = []
        for v in v_list:
            try:
                ids_list.append(validate_uuid1_hex(v))
            except PydanticCustomError as e:
                raise e

        duplicates = [item for item, count in Counter(ids_list).items() if count > 1]
        if duplicates:
            duplicates_str = ", ".join(duplicates)
            raise PydanticCustomError("duplicate_uuids", "Duplicate ids: '{duplicate_ids}'", {"duplicate_ids": duplicates_str})

        return ids_list


class DeleteDatasetReq(DeleteReq): ...


class BaseListReq(BaseModel):
    model_config = ConfigDict(extra="forbid")

    id: Annotated[str | None, Field(default=None)]
    name: Annotated[str | None, Field(default=None)]
    page: Annotated[int, Field(default=1, ge=1)]
    page_size: Annotated[int, Field(default=30, ge=1)]
    orderby: Annotated[Literal["create_time", "update_time"], Field(default="create_time")]
    desc: Annotated[bool, Field(default=True)]

    @field_validator("id", mode="before")
    @classmethod
    def validate_id(cls, v: Any) -> str:
        return validate_uuid1_hex(v)


class ListDatasetReq(BaseListReq): ...

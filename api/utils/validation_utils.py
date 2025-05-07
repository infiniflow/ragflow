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
from enum import auto
from typing import Annotated, Any

from flask import Request
from pydantic import BaseModel, Field, StringConstraints, ValidationError, field_validator
from strenum import StrEnum
from werkzeug.exceptions import BadRequest, UnsupportedMediaType

from api.constants import DATASET_NAME_LIMIT


def validate_and_parse_json_request(request: Request, validator: type[BaseModel]) -> tuple[dict[str, Any] | None, str | None]:
    """Validates and parses JSON requests through a multi-stage validation pipeline.

    Implements a robust four-stage validation process:
    1. Content-Type verification (must be application/json)
    2. JSON syntax validation
    3. Payload structure type checking
    4. Pydantic model validation with error formatting

    Args:
        request (Request): Flask request object containing HTTP payload

    Returns:
        tuple[Dict[str, Any] | None, str | None]:
        - First element:
            - Validated dictionary on success
            - None on validation failure
        - Second element:
            - None on success
            - Diagnostic error message on failure

    Raises:
        UnsupportedMediaType: When Content-Type ≠ application/json
        BadRequest: For structural JSON syntax errors
        ValidationError: When payload violates Pydantic schema rules

    Examples:
        Successful validation:
        ```python
        # Input: {"name": "Dataset1", "format": "csv"}
        # Returns: ({"name": "Dataset1", "format": "csv"}, None)
        ```

        Invalid Content-Type:
        ```python
        # Returns: (None, "Unsupported content type: Expected application/json, got text/xml")
        ```

        Malformed JSON:
        ```python
        # Returns: (None, "Malformed JSON syntax: Missing commas/brackets or invalid encoding")
        ```
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
        validated_request = validator(**payload)
    except ValidationError as e:
        return None, format_validation_error_message(e)

    parsed_payload = validated_request.model_dump(by_alias=True)

    return parsed_payload, None


def format_validation_error_message(e: ValidationError) -> str:
    """Formats validation errors into a standardized string format.

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
        json_schema_extra = {"charset": "utf8mb4", "collation": "utf8mb4_0900_ai_ci"}


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
    embedding_model: Annotated[str | None, StringConstraints(strip_whitespace=True, max_length=255), Field(default=None, serialization_alias="embd_id")]
    permission: Annotated[PermissionEnum, StringConstraints(strip_whitespace=True, min_length=1, max_length=16), Field(default=PermissionEnum.me)]
    chunk_method: Annotated[ChunkMethodnEnum, StringConstraints(strip_whitespace=True, min_length=1, max_length=32), Field(default=ChunkMethodnEnum.naive, serialization_alias="parser_id")]
    pagerank: int = Field(default=0, ge=0, le=100)
    parser_config: ParserConfig | None = Field(default=None)

    @field_validator("avatar")
    @classmethod
    def validate_avatar_base64(cls, v: str) -> str:
        """Validates Base64-encoded avatar string format and MIME type compliance.

        Implements a three-stage validation workflow:
        1. MIME prefix existence check
        2. MIME type format validation
        3. Supported type verification

        Args:
            v (str): Raw avatar field value

        Returns:
            str: Validated Base64 string

        Raises:
            ValueError: For structural errors in these cases:
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
                raise ValueError("Invalid MIME prefix format. Must start with 'data:'")

            mime_type = prefix[5:].split(";")[0]
            supported_mime_types = ["image/jpeg", "image/png"]
            if mime_type not in supported_mime_types:
                raise ValueError(f"Unsupported MIME type. Allowed: {supported_mime_types}")

            return v
        else:
            raise ValueError("Missing MIME prefix. Expected format: data:<mime>;base64,<data>")

    @field_validator("embedding_model", mode="after")
    @classmethod
    def validate_embedding_model(cls, v: str) -> str:
        """Validates embedding model identifier format compliance.

        Validation pipeline:
        1. Structural format verification
        2. Component non-empty check
        3. Value normalization

        Args:
            v (str): Raw model identifier

        Returns:
            str: Validated <model_name>@<provider> format

        Raises:
            ValueError: For these violations:
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
            raise ValueError("Embedding model identifier must follow <model_name>@<provider> format")

        components = v.split("@", 1)
        if len(components) != 2 or not all(components):
            raise ValueError("Both model_name and provider must be non-empty strings")

        model_name, provider = components
        if not model_name.strip() or not provider.strip():
            raise ValueError("Model name and provider cannot be whitespace-only strings")
        return v

    @field_validator("permission", mode="before")
    @classmethod
    def permission_auto_lowercase(cls, v: str) -> str:
        """Normalize permission input to lowercase for consistent PermissionEnum matching.

        Args:
            v (str): Raw input value for the permission field

        Returns:
            Lowercase string if input is string type, otherwise returns original value

        Behavior:
            - Converts string inputs to lowercase (e.g., "ME" → "me")
            - Non-string values pass through unchanged
            - Works in validation pre-processing stage (before enum conversion)
        """
        return v.lower() if isinstance(v, str) else v

    @field_validator("parser_config", mode="after")
    @classmethod
    def validate_parser_config_json_length(cls, v: ParserConfig | None) -> ParserConfig | None:
        """Validates serialized JSON length constraints for parser configuration.

        Implements a three-stage validation workflow:
        1. Null check - bypass validation for empty configurations
        2. Model serialization - convert Pydantic model to JSON string
        3. Size verification - enforce maximum allowed payload size

        Args:
            v (ParserConfig | None): Raw parser configuration object

        Returns:
            ParserConfig | None: Validated configuration object

        Raises:
            ValueError: When serialized JSON exceeds 65,535 characters
        """
        if v is None:
            return v

        if (json_str := v.model_dump_json()) and len(json_str) > 65535:
            raise ValueError(f"Parser config exceeds size limit (max 65,535 characters). Current size: {len(json_str):,}")
        return v

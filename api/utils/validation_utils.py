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
from typing import Annotated, List, Optional

from pydantic import BaseModel, Field, StringConstraints, ValidationError, field_validator
from strenum import StrEnum


def format_validation_error_message(e: ValidationError) -> str:
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
    entity_types: List[str] = Field(default_factory=lambda: ["organization", "person", "geo", "event", "category"])
    method: GraphragMethodEnum = Field(default=GraphragMethodEnum.light)
    community: bool = Field(default=False)
    resolution: bool = Field(default=False)


class ParserConfig(Base):
    auto_keywords: int = Field(default=0, ge=0, le=32)
    auto_questions: int = Field(default=0, ge=0, le=10)
    chunk_token_num: int = Field(default=128, ge=1, le=2048)
    delimiter: str = Field(default=r"\n", min_length=1)
    graphrag: Optional[GraphragConfig] = None
    html4excel: bool = False
    layout_recognize: str = "DeepDOC"
    raptor: Optional[RaptorConfig] = None
    tag_kb_ids: List[str] = Field(default_factory=list)
    topn_tags: int = Field(default=1, ge=1, le=10)
    filename_embd_weight: Optional[float] = Field(default=None, ge=0.0, le=1.0)
    task_page_size: Optional[int] = Field(default=None, ge=1)
    pages: Optional[List[List[int]]] = None


class CreateDatasetReq(Base):
    name: Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=128), Field(...)]
    avatar: Optional[str] = Field(default=None, max_length=65535)
    description: Optional[str] = Field(default=None, max_length=65535)
    embedding_model: Annotated[Optional[str], StringConstraints(strip_whitespace=True, max_length=255), Field(default=None, serialization_alias="embd_id")]
    permission: Annotated[PermissionEnum, StringConstraints(strip_whitespace=True, min_length=1, max_length=16), Field(default=PermissionEnum.me)]
    chunk_method: Annotated[ChunkMethodnEnum, StringConstraints(strip_whitespace=True, min_length=1, max_length=32), Field(default=ChunkMethodnEnum.naive, serialization_alias="parser_id")]
    pagerank: int = Field(default=0, ge=0, le=100)
    parser_config: Optional[ParserConfig] = Field(default=None)

    @field_validator("avatar")
    @classmethod
    def validate_avatar_base64(cls, v: str) -> str:
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
        if "@" not in v:
            raise ValueError("Embedding model must be xxx@yyy")
        return v

    @field_validator("permission", mode="before")
    @classmethod
    def permission_auto_lowercase(cls, v: str) -> str:
        if isinstance(v, str):
            return v.lower()
        return v

    @field_validator("parser_config", mode="after")
    @classmethod
    def validate_parser_config_json_length(cls, v: Optional[ParserConfig]) -> Optional[ParserConfig]:
        if v is not None:
            json_str = v.model_dump_json()
            if len(json_str) > 65535:
                raise ValueError("Parser config have at most 65535 characters")
        return v

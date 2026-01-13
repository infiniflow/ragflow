#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

from pydantic import Field, BaseModel, model_validator, AliasChoices

from core.types.doc_engine import DocumentEngineType


class ElasticSearchConfig(BaseModel):
    hosts: list[str] = Field(default_factory=lambda: ["http://localhost:1200"])
    username: str = Field("elastic")
    password: str = Field(default="")
    verify_certs: bool = Field(default=False)

    @model_validator(mode="before")
    @classmethod
    def parse_hosts(cls, values):
        hosts = values.get("hosts")
        if isinstance(hosts, str):
            values["hosts"] = [h.strip() for h in hosts.split(",") if h.strip()]
        return values


class OpenSearchConfig(BaseModel):
    hosts: list[str] = Field(default_factory=lambda: ["http://localhost:1201"])
    username: str = Field("admin")
    password: str = Field(default="")

    @model_validator(mode="before")
    @classmethod
    def parse_hosts(cls, values):
        hosts = values.get("hosts")
        if isinstance(hosts, str):
            values["hosts"] = [h.strip() for h in hosts.split(",") if h.strip()]
        return values


class InfinityConfig(BaseModel):
    uri: str = Field("localhost:23817")
    db_name: str = Field("default_db")
    host: str = Field(default="")
    port: int = Field(default=23817)

    @model_validator(mode="before")
    @classmethod
    def _handle_uri(cls, values: dict) -> dict:
        """
        Parse uri into host and port if not separately specified.
        Supports:
          - uri="host:port"
          - uri="host" (uses default port)
        """
        uri = values.get("uri")
        if uri and ":" in uri:
            host, port = uri.split(":", 1)
            values["host"] = host
            values["port"] = int(port)
        elif uri:
            values["host"] = uri
        return values


class DocumentEngineConfig(BaseModel):
    active: DocumentEngineType = Field(default=DocumentEngineType.ELASTICSEARCH)
    es: ElasticSearchConfig = Field(
        default_factory=ElasticSearchConfig,
        validation_alias=AliasChoices("es", "elasticsearch")
    )
    os: OpenSearchConfig = Field(
        default_factory=OpenSearchConfig,
        validation_alias=AliasChoices("os", "opensearch")
    )
    infinity: InfinityConfig = Field(default_factory=InfinityConfig)

    @property
    def current(self) -> BaseModel:
        name = self.active.value
        try:
            return getattr(self, name)
        except AttributeError:
            raise ValueError(f"Document engine '{name}' is not configured")

    @property
    def is_elasticsearch(self) -> bool:
        return self.active == DocumentEngineType.ELASTICSEARCH

    @property
    def is_infinity(self) -> bool:
        return self.active == DocumentEngineType.INFINITY

    @model_validator(mode="after")
    def validate_engine_supported(self):
        if self.active not in DocumentEngineType:
            raise ValueError(f"Unsupported document engine: {self.active}")
        return self

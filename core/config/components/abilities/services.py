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

import logging
import secrets
from typing import Optional

from pydantic import BaseModel, Field, AliasChoices, field_validator, model_validator

from core.config.utils.converters import str_to_bool


class RAGFlowConfig(BaseModel):
    # Web
    host: str = Field(default="0.0.0.0")
    http_port: int = Field(default=9380)
    max_content_length: int = Field(default=1024 * 1024 * 1024)
    response_timeout: int = Field(default=600)
    body_timeout: int = Field(default=600)
    strong_test_count: int = Field(default=8)

    default_superuser_nickname: Optional[str] = Field(default="admin")
    default_superuser_email: Optional[str] = Field(default="admin@ragflow.io")
    default_superuser_password: Optional[str] = Field(default="admin")

    # Features
    secret_key: Optional[str] = Field(default=None)
    register_enabled: bool = Field(default=True)
    crypto_enabled: bool = Field(default=False)
    use_docling: bool = Field(default=False)

    def __init__(self, **data):
        super().__init__(**data)
        if not self.secret_key or len(self.secret_key) < 32:
            new_key = secrets.token_hex(32)
            self.secret_key = new_key
            logging.warning(f"SECURITY WARNING: Using auto-generated SECRET_KEY: {new_key}")

    @model_validator(mode="before")
    @classmethod
    def parse_bool(cls, values):
        fields = ('register_enabled',)
        for f in fields:
            if f in values and isinstance(values[f], str):
                values[f] = str_to_bool(values[f])
        return values


class SMTPConfig(BaseModel):
    enabled: bool = Field(default=False, validation_alias=AliasChoices("enabled", "mail_enabled"))
    server: str = Field(default="", validation_alias=AliasChoices("server", "mail_server"))
    port: int = Field(default=465, validation_alias=AliasChoices("port", "mail_port"))
    username: str = Field(default="", validation_alias=AliasChoices("username", "mail_username"))
    password: str = Field(default="", validation_alias=AliasChoices("password", "mail_password"))
    use_ssl: bool = Field(default=True, validation_alias=AliasChoices("use_ssl", "mail_use_ssl"))
    use_tls: bool = Field(default=False, validation_alias=AliasChoices("use_tls", "mail_use_tls"))
    default_sender: tuple[str, str] = Field(
        default=("RAGFlow", ""), validation_alias=AliasChoices("default_sender", "mail_default_sender"))
    frontend_url: str = Field(
        default="", validation_alias=AliasChoices("frontend_url", "mail_frontend_url"))

    @field_validator("default_sender", mode="before")
    @classmethod
    def normalize_default_sender(cls, v):
        if v is None:
            return None

        if isinstance(v, str):
            return "", v
        # list / tuple
        if isinstance(v, (list, tuple)):
            if len(v) != 2:
                raise ValueError("mail_default_sender must be [name, email]")
            return tuple(v)

        raise TypeError("mail_default_sender must be str or [name, email]")


class AdminConfig(BaseModel):
    host: str = Field(default="0.0.0.0")
    http_port: int = Field(default=9381)


class SandboxConfig(BaseModel):
    enabled: bool = Field(default=True)
    host: str = Field(default="sandbox-executor-manager")
    max_memory: str = Field(default="256m", description="b, k, m, g")
    timeout: str = Field(default="10s", description="Timeout in seconds, s, m, e.g. 1m30s")
    base_python_image: str = Field(default="sandbox-base-python:latest")
    base_nodejs_image: str = Field(default="sandbox-base-nodejs:latest")
    executor_manager_port: int = Field(default=9385)
    executor_manager_pool_size: int = Field(default=3)
    enable_seccomp: bool = Field(default=False)


class TaskExecutorConfig(BaseModel):
    name: str = Field(default="task_executor")
    host: str = Field(default="localhost")
    port: int = Field(default=0)
    message_queue_type: str = Field(default="redis")

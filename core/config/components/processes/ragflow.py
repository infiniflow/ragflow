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


class APIConfig(BaseModel):
    # Web
    host: str = Field(default="0.0.0.0")
    http_port: int = Field(default=9380)
    max_content_length: int = Field(default=1024 * 1024 * 1024)
    response_timeout: int = Field(default=600)
    body_timeout: int = Field(default=600)
    strong_test_count: int = Field(default=8)

    # Features
    secret_key: Optional[str] = None
    register_enabled: bool = Field(default=True)
    crypto_enabled: bool = Field(default=False)

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
        default="", validation_alias=AliasChoices("default_sender", "mail_default_sender"))
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

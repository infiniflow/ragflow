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

from typing import Optional

from pydantic import BaseModel, Field, RedisDsn, model_validator

from core.config.components.abilities.security import PasswordConfig
from core.config.utils.decrypt import decrypt_password
from core.config.types import CacheType


class RedisConfig(BaseModel):
    host: str = Field(default="localhost")
    port: int = Field(default=6379)
    username: Optional[str] = Field(default=None)
    password: Optional[str] = Field(default=None)
    db: int = Field(default=1)
    decode_responses: bool = Field(default=True)

    dsn: Optional[RedisDsn] = None

    @property
    def endpoint(self) -> str:
        return f"{self.host}:{self.port}"

    @model_validator(mode="before")
    @classmethod
    def _handle_host_port(cls, values: dict) -> dict:
        """
        Support both:
        1. host="127.0.0.1:6380" format
        2. host and port defined separately
        """
        # If host contains ':', split into host and port
        host = values.get("host")
        if host and ":" in host:
            h, p = host.split(":", 1)
            values["host"] = h
            values["port"] = int(p)
        return values

    @model_validator(mode="after")
    def _build_dsn(self):
        if not self.dsn:
            if self.password:
                self.dsn = RedisDsn(f"redis://:{self.password}@{self.host}:{self.port}/{self.db}")
            else:
                self.dsn = RedisDsn(f"redis://{self.host}:{self.port}/{self.db}")
        return self

    @property
    def connection_params(self) -> dict:
        """
        Return a dict suitable for redis-py connection.
        Exclude username/password if not set.
        """
        conn = {
            "host": self.host,
            "port": self.port,
            "db": self.db,
            "decode_responses": self.decode_responses,
        }
        if self.username:
            conn["username"] = self.username
        if self.password:
            conn["password"] = self.password
        return conn


class CacheConfig(BaseModel):
    active: CacheType = Field(default=CacheType.REDIS)
    redis: RedisConfig = Field(default_factory=RedisConfig)

    @property
    def current(self) -> RedisConfig:
        """
        Return the active cache configuration with password decrypted if needed.
        """
        name = self.active.value
        try:
            return getattr(self, name)
        except AttributeError:
            raise ValueError(f"Cache '{name}' is not configured")

    @property
    def is_redis(self) -> bool:
        return self.active == CacheType.REDIS

    def decrypt_password(self, password_conf: PasswordConfig) -> None:
        """Decrypt password of the currently active cache if needed."""
        conf = self.current
        conf.password = decrypt_password(conf.password, password_conf)

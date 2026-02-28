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

from typing import Optional, Literal
from urllib.parse import quote_plus

from pydantic import BaseModel, Field, PostgresDsn, MySQLDsn, model_validator

from core.config.utils.decrypt import decrypt_password
from core.config.types import DatabaseType


class MySQLConfig(BaseModel):
    host: str = Field(default="localhost")
    port: int = Field(default=5455)
    user: str = Field(default="root")
    password: Optional[str] = Field(default=None)
    name: str = Field(default="rag_flow")
    max_connections: int = Field(default=100)
    stale_timeout: int = Field(default=300)
    max_packet: int = Field(default=1073741824)
    dsn: Optional[MySQLDsn] = None

    @property
    def database(self) -> str:
        return self.name

    @model_validator(mode="after")
    def build_dsn(self):
        if not self.dsn:
            auth = ""
            if self.user:
                auth = quote_plus(self.user)
                if self.password:
                    auth += f":{quote_plus(self.password)}"
                auth += "@"
            self.dsn = MySQLDsn(
                f"mysql+pymysql://{auth}{self.host}:{self.port}/{self.name}"
            )
        return self


class PostgresConfig(BaseModel):
    host: str = Field(default="localhost")
    port: int = Field(default=5432)
    user: str = Field(default="rag_flow")
    password: Optional[str] = Field(default=None)
    database: str = Field(default="rag_flow")
    max_connections: int = Field(default=100)
    stale_timeout: int = Field(default=30)
    dsn: Optional[PostgresDsn] = None

    @model_validator(mode="after")
    def build_dsn(self):
        if not self.dsn:
            auth = ""
            if self.user:
                auth = quote_plus(self.user)
                if self.password:
                    auth += f":{quote_plus(self.password)}"
                auth += "@"

            self.dsn = PostgresDsn(
                f"postgresql+asyncpg://{auth}{self.host}:{self.port}/{self.database}"
            )
        return self


class OceanBaseInnerConfig(BaseModel):
    db_name: str = Field(default="test")
    user: str = Field(default="root")
    password: Optional[str] = Field(default=None)
    host: str = Field(default="localhost")
    port: int = Field(default=2881)
    max_connections: int = Field(default=300)


class OceanBaseConfig(BaseModel):
    scheme: Literal["mysql", "oceanbase"] = Field(default="oceanbase")  # 'oceanbase' or 'mysql'
    config: OceanBaseInnerConfig = Field(default_factory=OceanBaseInnerConfig)

    @property
    def user(self) -> str:
        return self.config.user

    @property
    def password(self) -> str:
        return self.config.password

    @property
    def database(self) -> str:
        return self.config.db_name

    @property
    def host(self) -> str:
        return self.config.host

    @property
    def port(self) -> int:
        return self.config.port

    @property
    def max_connections(self) -> int:
        return self.config.max_connections

    @property
    def dsn(self) -> str:
        user = quote_plus(self.user)
        password = quote_plus(self.password)
        return f"{self.scheme}://{user}:{password}@{self.config.host}:{self.config.port}/{self.database}"


class DatabaseConfig(BaseModel):
    active: DatabaseType = Field(default=DatabaseType.MYSQL)
    mysql: MySQLConfig = Field(default_factory=MySQLConfig)
    postgres: PostgresConfig = Field(default_factory=PostgresConfig)
    oceanbase: OceanBaseConfig = Field(default_factory=OceanBaseConfig)

    @property
    def current(self) -> BaseModel:
        name = self.active.value
        try:
            return getattr(self, name)
        except AttributeError:
            raise ValueError(f"Database '{name}' is not configured")

    def decrypt_passwords(self, password_conf) -> None:
        """
        Only decrypt / validate password of the currently active database.
        """
        current_db = self.current
        if isinstance(current_db, OceanBaseConfig):
            db_conf = current_db.config
        else:
            db_conf = current_db

        db_conf.password = decrypt_password(db_conf.password, password_conf)

    @model_validator(mode="after")
    def normalize_ob_config(self):
        """
        If OceanBase scheme is 'mysql', reuse MySQL configuration
        to override OceanBase inner config.
        """
        if self.oceanbase.scheme.lower() != "mysql":
            return self

        mysql = self.mysql

        self.oceanbase.config = OceanBaseInnerConfig(
            host=mysql.host,
            port=mysql.port,
            user=mysql.user,
            password=mysql.password,
            db_name=mysql.name,
            max_connections=mysql.max_connections,
        )
        return self

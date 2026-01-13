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
from enum import Enum
from typing import Protocol, Any, Callable


class DatabaseType(str, Enum):
    MYSQL = "mysql"
    POSTGRES = "postgres"
    OCEANBASE = "oceanbase"


class DatabaseConfigProtocol(Protocol):
    @property
    def dsn(self) -> str: ...

    @property
    def user(self) -> str: ...

    @property
    def password(self) -> str: ...

    @property
    def database(self) -> str: ...

    def close_stale(self, age: int = ...) -> None: ...

    def connection_context(self): ...


class DatabaseConnection(Protocol):
    def execute_sql(self, sql: str, params: Any = None, commit: bool = True): ...
    def begin(self): ...
    def close(self): ...


class DatabaseWithLockProtocol(DatabaseConfigProtocol, Protocol):
    lock: Callable[[str, int], Any]

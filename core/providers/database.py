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

from typing import Optional, cast

from core.config.app import AppConfig
from core.config.components.base.database import MySQLConfig, PostgresConfig
from core.db.locks import MysqlDatabaseLock, PostgresDatabaseLock
from core.db.pool import RetryingPooledMySQLDatabase, RetryingPooledPostgresqlDatabase
from core.providers.base import ProviderBase
from core.types.database import DatabaseWithLockProtocol


class DatabaseProvider(ProviderBase):
    def __init__(self, config: Optional[AppConfig] = None):
        super().__init__(config)

    def conn(self) -> DatabaseWithLockProtocol:
        cfg = self._config.database.current

        if isinstance(cfg, MySQLConfig):
            conn = RetryingPooledMySQLDatabase(
                cfg.name,
                user=cfg.user,
                password=cfg.password,
                host=cfg.host,
                port=cfg.port,
                max_connections=cfg.max_connections,
                stale_timeout=cfg.stale_timeout,
                max_retries=5,
                retry_delay=1,
            )

            conn.lock = lambda name, timeout=10: MysqlDatabaseLock(conn, name, timeout,)

        elif isinstance(cfg, PostgresConfig):
            conn = RetryingPooledPostgresqlDatabase(
                cfg.database,
                user=cfg.user,
                password=cfg.password,
                host=cfg.host,
                port=cfg.port,
                max_connections=cfg.max_connections,
                stale_timeout=cfg.stale_timeout,
                max_retries=5,
                retry_delay=1,
            )

            conn.lock = lambda name, timeout=10: PostgresDatabaseLock(conn, name, timeout)
        else:
            raise TypeError(f"Unsupported DB config type: {type(cfg)}")

        return cast(DatabaseWithLockProtocol, cast(object, conn))

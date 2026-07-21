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
import logging
import os
import time

import infinity
from infinity.connection_pool import ConnectionPool
from infinity.errors import ErrorCode

from common import settings
from common.decorator import singleton

MAX_RETRIES = 8
HEALTH_CHECK_BASE_DELAY_SECONDS = 5
HEALTH_CHECK_MAX_DELAY_SECONDS = 60


@singleton
class InfinityConnectionPool:
    def __init__(self):
        if hasattr(settings, "INFINITY"):
            self.INFINITY_CONFIG = settings.INFINITY
        else:
            self.INFINITY_CONFIG = settings.get_base_config("infinity", {"uri": "infinity:23817", "postgres_port": 5432, "db_name": "default_db"})

        raw_pool_max_size = os.environ.get("INFINITY_POOL_MAX_SIZE", "4")
        try:
            self.pool_max_size = int(raw_pool_max_size)
        except ValueError as e:
            raise ValueError("INFINITY_POOL_MAX_SIZE must be a positive integer") from e
        if self.pool_max_size < 1:
            raise ValueError("INFINITY_POOL_MAX_SIZE must be >= 1")

        infinity_uri = self.INFINITY_CONFIG["uri"]
        if ":" in infinity_uri:
            host, port = infinity_uri.split(":")
            self.infinity_uri = infinity.common.NetworkAddress(host, int(port))

        self.conn_pool = None
        for attempt in range(MAX_RETRIES):
            conn_pool = None
            inf_conn = None
            try:
                conn_pool = ConnectionPool(self.infinity_uri, max_size=self.pool_max_size)
                inf_conn = conn_pool.get_conn()
                res = inf_conn.show_current_node()
                if res.error_code == ErrorCode.OK and res.server_status in ["started", "alive"]:
                    self.conn_pool = conn_pool
                    break
                logging.warning(
                    "Infinity status %s on attempt %d/%d, waiting Infinity %s to be healthy.",
                    res.server_status,
                    attempt + 1,
                    MAX_RETRIES,
                    infinity_uri,
                )
                if attempt == MAX_RETRIES - 1:
                    msg = f"Infinity {infinity_uri} status {res.server_status} after {MAX_RETRIES} attempts."
                    logging.error(msg)
                    raise Exception(msg)
                time.sleep(min(HEALTH_CHECK_BASE_DELAY_SECONDS * (2 ** attempt), HEALTH_CHECK_MAX_DELAY_SECONDS))
            except Exception as e:
                logging.warning(
                    "Infinity connection attempt %d/%d to %s failed: %s",
                    attempt + 1,
                    MAX_RETRIES,
                    infinity_uri,
                    e,
                )
                if attempt == MAX_RETRIES - 1:
                    raise
                time.sleep(min(HEALTH_CHECK_BASE_DELAY_SECONDS * (2 ** attempt), HEALTH_CHECK_MAX_DELAY_SECONDS))
            finally:
                if inf_conn is not None and conn_pool is not None:
                    conn_pool.release_conn(inf_conn)
                if conn_pool is not None and conn_pool is not self.conn_pool:
                    conn_pool.destroy()

        if self.conn_pool is None:
            msg = f"Infinity {infinity_uri} is unhealthy after {MAX_RETRIES} attempts."
            logging.error(msg)
            raise Exception(msg)

        logging.info(f"Infinity {infinity_uri} is healthy. Connection pool max_size={self.pool_max_size}")

    def get_conn_pool(self):
        return self.conn_pool

    def get_conn_uri(self):
        """
        Get connection URI for PostgreSQL protocol.
        """
        infinity_uri = self.INFINITY_CONFIG["uri"]
        postgres_port = self.INFINITY_CONFIG["postgres_port"]
        db_name = self.INFINITY_CONFIG["db_name"]

        if ":" in infinity_uri:
            host, _ = infinity_uri.split(":")
            return f"host={host} port={postgres_port} dbname={db_name}"
        return f"host=localhost port={postgres_port} dbname={db_name}"

    def refresh_conn_pool(self):
        try:
            inf_conn = self.conn_pool.get_conn()
            res = inf_conn.show_current_node()
            if res.error_code == ErrorCode.OK and res.server_status in ["started", "alive"]:
                return self.conn_pool
            else:
                raise Exception(f"{res.error_code}: {res.server_status}")

        except Exception as e:
            logging.error(str(e))
            if hasattr(self, "conn_pool") and self.conn_pool:
                self.conn_pool.destroy()
                self.conn_pool = ConnectionPool(self.infinity_uri, max_size=self.pool_max_size)
                return self.conn_pool

    def __del__(self):
            if hasattr(self, "conn_pool") and self.conn_pool:
                self.conn_pool.destroy()


INFINITY_CONN = InfinityConnectionPool()

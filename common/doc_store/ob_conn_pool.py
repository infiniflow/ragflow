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

from pyobvector import ObVecClient
from pyobvector.client import ClusterVersionException
from pyobvector.client.hybrid_search import HybridSearch
from pyobvector.util import ObVersion

from common import settings
from common.decorator import singleton

ATTEMPT_TIME = 2
OB_QUERY_TIMEOUT = int(os.environ.get("OB_QUERY_TIMEOUT", "100_000_000"))

logger = logging.getLogger('ragflow.ob_conn_pool')


@singleton
class OceanBaseConnectionPool:

    def __init__(self):
        self.client = None
        self.es = None  # HybridSearch client

        if hasattr(settings, "OB"):
            self.OB_CONFIG = settings.OB
        else:
            self.OB_CONFIG = settings.get_base_config("oceanbase", {})

        scheme = self.OB_CONFIG.get("scheme")
        ob_config = self.OB_CONFIG.get("config", {})

        if scheme and scheme.lower() == "mysql":
            mysql_config = settings.get_base_config("mysql", {})
            logger.info("Use MySQL scheme to create OceanBase connection.")
            host = mysql_config.get("host", "localhost")
            port = mysql_config.get("port", 2881)
            self.username = mysql_config.get("user", "root@test")
            self.password = mysql_config.get("password", "infini_rag_flow")
            max_connections = mysql_config.get("max_connections", 300)
        else:
            logger.info("Use customized config to create OceanBase connection.")
            host = ob_config.get("host", "localhost")
            port = ob_config.get("port", 2881)
            self.username = ob_config.get("user", "root@test")
            self.password = ob_config.get("password", "infini_rag_flow")
            max_connections = ob_config.get("max_connections", 300)

        self.db_name = ob_config.get("db_name", "test")
        self.uri = f"{host}:{port}"

        logger.info(f"Use OceanBase '{self.uri}' as the doc engine.")

        max_overflow = int(os.environ.get("OB_MAX_OVERFLOW", max(max_connections // 2, 10)))
        pool_timeout = int(os.environ.get("OB_POOL_TIMEOUT", "30"))

        for _ in range(ATTEMPT_TIME):
            try:
                self.client = ObVecClient(
                    uri=self.uri,
                    user=self.username,
                    password=self.password,
                    db_name=self.db_name,
                    pool_pre_ping=True,
                    pool_recycle=3600,
                    pool_size=max_connections,
                    max_overflow=max_overflow,
                    pool_timeout=pool_timeout,
                )
                break
            except Exception as e:
                logger.warning(f"{str(e)}. Waiting OceanBase {self.uri} to be healthy.")
                time.sleep(5)

        if self.client is None:
            msg = f"OceanBase {self.uri} connection failed after {ATTEMPT_TIME} attempts."
            logger.error(msg)
            raise Exception(msg)

        self._check_ob_version()
        self._try_to_update_ob_query_timeout()
        self._init_hybrid_search(max_connections, max_overflow, pool_timeout)

        logger.info(f"OceanBase {self.uri} is healthy.")

    def _check_ob_version(self):
        try:
            res = self.client.perform_raw_text_sql("SELECT OB_VERSION() FROM DUAL").fetchone()
            version_str = res[0] if res else None
            logger.info(f"OceanBase {self.uri} version is {version_str}")
        except Exception as e:
            raise Exception(f"Failed to get OceanBase version from {self.uri}, error: {str(e)}")

        if not version_str:
            raise Exception(f"Failed to get OceanBase version from {self.uri}.")

        ob_version = ObVersion.from_db_version_string(version_str)
        if ob_version < ObVersion.from_db_version_nums(4, 3, 5, 1):
            raise Exception(
                f"The version of OceanBase needs to be higher than or equal to 4.3.5.1, current version is {version_str}"
            )

    def _try_to_update_ob_query_timeout(self):
        try:
            rows = self.client.perform_raw_text_sql("SHOW VARIABLES LIKE 'ob_query_timeout'")
            for row in rows:
                val = row[1]
                if val and int(val) >= OB_QUERY_TIMEOUT:
                    return
        except Exception as e:
            logger.warning("Failed to get 'ob_query_timeout' variable: %s", str(e))

        try:
            self.client.perform_raw_text_sql(f"SET GLOBAL ob_query_timeout={OB_QUERY_TIMEOUT}")
            logger.info("Set GLOBAL variable 'ob_query_timeout' to %d.", OB_QUERY_TIMEOUT)
            self.client.engine.dispose()
            logger.info("Disposed all connections in engine pool to refresh connection pool")
        except Exception as e:
            logger.warning(f"Failed to set 'ob_query_timeout' variable: {str(e)}")

    def _init_hybrid_search(self, max_connections, max_overflow, pool_timeout):
        enable_hybrid_search = os.getenv('ENABLE_HYBRID_SEARCH', 'false').lower() in ['true', '1', 'yes', 'y']
        if enable_hybrid_search:
            try:
                self.es = HybridSearch(
                    uri=self.uri,
                    user=self.username,
                    password=self.password,
                    db_name=self.db_name,
                    pool_pre_ping=True,
                    pool_recycle=3600,
                    pool_size=max_connections,
                    max_overflow=max_overflow,
                    pool_timeout=pool_timeout,
                )
                logger.info("OceanBase Hybrid Search feature is enabled")
            except ClusterVersionException as e:
                logger.info("Failed to initialize HybridSearch client, fallback to use SQL", exc_info=e)
                self.es = None

    def get_client(self) -> ObVecClient:
        return self.client

    def get_hybrid_search_client(self) -> HybridSearch | None:
        return self.es

    def get_db_name(self) -> str:
        return self.db_name

    def get_uri(self) -> str:
        return self.uri

    def refresh_client(self) -> ObVecClient:
        try:
            self.client.perform_raw_text_sql("SELECT 1 FROM DUAL")
            return self.client
        except Exception as e:
            logger.warning(f"OceanBase connection unhealthy: {str(e)}, refreshing...")
            self.client.engine.dispose()
            return self.client

    def __del__(self):
        if hasattr(self, "client") and self.client:
            try:
                self.client.engine.dispose()
            except Exception:
                pass
        if hasattr(self, "es") and self.es:
            try:
                self.es.engine.dispose()
            except Exception:
                pass


OB_CONN = OceanBaseConnectionPool()

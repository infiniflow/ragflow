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
import re
import threading
from dataclasses import dataclass
from typing import Any

from psycopg2 import pool as psycopg2_pool

logger = logging.getLogger("ragflow.gaussdb_conn_pool")
_IDENTIFIER_PATTERN = re.compile(r"^(?:[A-Za-z_]|[^\x00-\x7F])(?:[A-Za-z0-9_#$]|[^\x00-\x7F]){0,62}$")
_DECIMAL_PORT_PATTERN = re.compile(r"^[0-9]+$")
_DATABASE_KEYS = ("database", "db_name", "name")


class GaussDBError(Exception):
    """Base exception for GaussDB DocEngine setup and connectivity."""


class InvalidGaussDBConfig(ValueError, GaussDBError):
    """Raised when GaussDB DocEngine config is missing or invalid."""


class GaussDBConnectionError(GaussDBError):
    """Raised when the GaussDB connection cannot be established or used."""


class GaussDBAuthenticationError(GaussDBConnectionError):
    """Raised when GaussDB rejects configured credentials."""


class GaussDBPermissionError(GaussDBConnectionError):
    """Raised when the configured user cannot use the target schema."""


@dataclass(frozen=True)
class GaussDBConfig:
    host: str
    port: int
    database: str
    user: str
    password: str
    schema: str = "public"


def _normalize_schema(schema: Any) -> str:
    value = str(schema or "").strip() or "public"
    if not _IDENTIFIER_PATTERN.match(value):
        raise InvalidGaussDBConfig(f"invalid gaussdb schema: {value}")
    return value


def _normalize_required_string(value: Any, field: str, *, strip: bool = True) -> str:
    if not isinstance(value, str) or not value.strip():
        raise InvalidGaussDBConfig(f"invalid gaussdb config field: {field}")
    return value.strip() if strip else value


def _normalize_port(value: Any) -> int:
    if isinstance(value, bool):
        raise InvalidGaussDBConfig("invalid gaussdb config field: port")
    if isinstance(value, int):
        port = value
    elif isinstance(value, str) and _DECIMAL_PORT_PATTERN.fullmatch(value):
        port = int(value)
    else:
        raise InvalidGaussDBConfig("invalid gaussdb config field: port")
    if port <= 0 or port > 65535:
        raise InvalidGaussDBConfig("invalid gaussdb config field: port")
    return port


def load_gaussdb_config(raw: dict[str, Any] | None = None) -> GaussDBConfig:
    if raw is None:
        from common import settings

        raw = getattr(settings, "GAUSSDB", None) or settings.get_base_config("gaussdb", {})
    config = raw if isinstance(raw, dict) else {}
    database_key = next((key for key in _DATABASE_KEYS if key in config), None)
    configured_keys = {
        "host": "host" if "host" in config else None,
        "port": "port" if "port" in config else None,
        "database": database_key,
        "user": "user" if "user" in config else None,
        "password": "password" if "password" in config else None,
    }
    missing = [field for field, key in configured_keys.items() if key is None]
    if missing:
        raise InvalidGaussDBConfig(f"missing gaussdb config field(s): {', '.join(missing)}")

    return GaussDBConfig(
        host=_normalize_required_string(config["host"], "host"),
        port=_normalize_port(config["port"]),
        database=_normalize_required_string(config[database_key], "database"),
        user=_normalize_required_string(config["user"], "user"),
        password=_normalize_required_string(config["password"], "password", strip=False),
        schema=_normalize_schema(config.get("schema")),
    )


def mask_gaussdb_uri(cfg: GaussDBConfig) -> str:
    return f"{cfg.user}@{cfg.host}:{cfg.port}/{cfg.database}?schema={cfg.schema}"


def classify_gaussdb_exception(exc: Exception) -> GaussDBConnectionError:
    if isinstance(exc, GaussDBConnectionError):
        return exc
    text = str(exc).lower()
    if "password" in text or "authentication" in text or "invalid username" in text:
        return GaussDBAuthenticationError(str(exc))
    if "permission" in text or "privilege" in text:
        return GaussDBPermissionError(str(exc))
    return GaussDBConnectionError(str(exc))


class GaussDBConnectionPool:
    def __init__(
        self,
        config: GaussDBConfig | None = None,
        pool: Any | None = None,
        minconn: int = 1,
        maxconn: int = 8,
    ):
        self.config = config or load_gaussdb_config()
        self.resolved_schema = self.config.schema
        self.masked_uri = mask_gaussdb_uri(self.config)
        self._pool = pool or self._create_pool(minconn=minconn, maxconn=maxconn)

    def _create_pool(self, minconn: int, maxconn: int):
        try:
            return psycopg2_pool.ThreadedConnectionPool(
                minconn,
                maxconn,
                host=self.config.host,
                port=self.config.port,
                dbname=self.config.database,
                user=self.config.user,
                password=self.config.password,
                options=(f"-c search_path={self.resolved_schema} -c client_encoding=UTF8 -c default_transaction_read_only=off"),
            )
        except Exception as exc:
            raise classify_gaussdb_exception(exc) from exc

    def _discard_conn(self, conn) -> None:
        if conn is None:
            return
        self._pool.putconn(conn, close=True)

    def _validate_conn(self, conn) -> None:
        if getattr(conn, "closed", False):
            raise GaussDBConnectionError("GaussDB connection is closed")
        cur = None
        try:
            cur = conn.cursor()
            cur.execute("SELECT 1")
            conn.rollback()
        finally:
            if cur is not None:
                cur.close()

    def get_conn(self):
        last_exc: Exception = RuntimeError("GaussDB connection checkout failed without an exception")
        for _attempt in range(2):
            conn = None
            try:
                conn = self._pool.getconn()
                self._validate_conn(conn)
                return conn
            except Exception as exc:
                last_exc = exc
                self._discard_conn(conn)
        raise classify_gaussdb_exception(last_exc) from last_exc

    def put_conn(self, conn) -> None:
        if conn is None:
            return
        try:
            conn.rollback()
        except Exception as exc:
            logger.warning("Discarding GaussDB connection after rollback failure: %s", type(exc).__name__)
            try:
                self._discard_conn(conn)
            except Exception as discard_exc:
                logger.warning("Failed to discard GaussDB connection: %s", type(discard_exc).__name__)
            return
        self._pool.putconn(conn)

    def close_all(self) -> None:
        self._pool.closeall()

    def check_schema_access(self) -> None:
        conn = self.get_conn()
        cur = None
        try:
            cur = conn.cursor()
            cur.execute(
                """
                SELECT
                  has_schema_privilege(%s, %s, %s) AS has_usage,
                  has_schema_privilege(%s, %s, %s) AS has_create
                """,
                (
                    self.config.user,
                    self.resolved_schema,
                    "USAGE",
                    self.config.user,
                    self.resolved_schema,
                    "CREATE",
                ),
            )
            row = cur.fetchone()
            has_usage, has_create = (bool(row[0]), bool(row[1])) if row else (False, False)
            missing = []
            if not has_usage:
                missing.append("USAGE")
            if not has_create:
                missing.append("CREATE")
            if missing:
                raise GaussDBPermissionError(f"GaussDB user {self.config.user} lacks {', '.join(missing)} on schema {self.resolved_schema}")
        except GaussDBPermissionError:
            raise
        except Exception as exc:
            raise classify_gaussdb_exception(exc) from exc
        finally:
            if cur is not None:
                cur.close()
            self.put_conn(conn)

    def fetch_one(self, sql: str, params: tuple[Any, ...] | None = None):
        conn = self.get_conn()
        cur = None
        try:
            cur = conn.cursor()
            cur.execute(sql, params)
            return cur.fetchone()
        except Exception as exc:
            raise classify_gaussdb_exception(exc) from exc
        finally:
            if cur is not None:
                cur.close()
            self.put_conn(conn)


class _LazyGaussDBConnectionPool:
    """Shared lazy pool used by the GaussDB DocEngine and Memory Store."""

    def __init__(self):
        self._lock = threading.Lock()
        self._pool: GaussDBConnectionPool | None = None

    def get_pool(self) -> GaussDBConnectionPool:
        if self._pool is None:
            with self._lock:
                if self._pool is None:
                    self._pool = GaussDBConnectionPool()
        return self._pool

    def close_all(self) -> None:
        with self._lock:
            if self._pool is not None:
                self._pool.close_all()
                self._pool = None

    def __getattr__(self, name: str):
        return getattr(self.get_pool(), name)


GAUSSDB_CONN = _LazyGaussDBConnectionPool()

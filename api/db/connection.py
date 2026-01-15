#
# Database connection initialization and utilities
#
from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from common import settings
from common.decorator import singleton

# Import all pooling, locking, and diagnostic components
from api.db.diagnostics import PoolDiagnostics
from api.db.locks import DatabaseLock

# Type hints only - actual imports happen at module end for proper exports
if TYPE_CHECKING:
    from playhouse.pool import PooledMySQLDatabase, PooledPostgresqlDatabase
    from api.db.pool import PooledDatabase

DB: PooledMySQLDatabase | PooledPostgresqlDatabase


def get_database_config():
    """
    Extract and normalize database configuration from settings.

    Returns dict with keys: 'type', 'name', 'host', 'port', 'user', 'password'
    """
    database_config = (settings.DATABASE or {}).copy()
    db_type = settings.DATABASE_TYPE.lower()

    return {
        "type": db_type,
        "name": database_config.get("name"),
        "host": database_config.get("host", "localhost"),
        "port": database_config.get("port", 5432 if db_type == "postgres" else 3306),
        "user": database_config.get("user"),
        "password": database_config.get("password"),
    }


def ensure_database_exists():
    """
    Create the target database if it doesn't exist.

    Uses the configured database user credentials (expected to be superuser for initial setup).
    Mirrors MySQL approach: assumes user has CREATE DATABASE permission.

    For PostgreSQL: Connects to 'postgres' system database to create target DB.
    For MySQL: Connects without database to create target DB.

    Idempotent—safe to call multiple times. Non-blocking: logs warnings on failure.

    Security Note:
        By default, expects superuser credentials (postgres/root) for database creation.
        For sandboxed environments with restricted users, see docs/POSTGRESQL_SECURITY.md
        on pre-creating the database or granting CREATE DATABASE permission.
    """
    try:
        config = get_database_config()
        db_type = config["type"]
        db_name = config["name"]
        db_host = config["host"]
        db_port = config["port"]
        db_user = config["user"]
        db_pass = config["password"]

        if db_type == "postgres":
            try:
                import psycopg2
                from psycopg2 import sql
            except ImportError:
                logging.warning("psycopg2 not available; skipping database creation for PostgreSQL")
                return

            try:
                # Connect to postgres system database using configured credentials
                conn = psycopg2.connect(host=db_host, port=db_port, user=db_user, password=db_pass, database="postgres")
                conn.autocommit = True
                cursor = conn.cursor()

                # Check if database exists first (idempotent)
                cursor.execute(sql.SQL("SELECT 1 FROM pg_database WHERE datname = %s"), (db_name,))

                if cursor.fetchone() is None:
                    # Database doesn't exist, create it
                    cursor.execute(sql.SQL("CREATE DATABASE {}").format(sql.Identifier(db_name)))
                    logging.info(f"Created PostgreSQL database '{db_name}' at {db_host}:{db_port}")
                else:
                    logging.info(f"PostgreSQL database '{db_name}' already exists at {db_host}:{db_port}")

                cursor.close()
                conn.close()

            except Exception as e:
                logging.warning(
                    f"Failed to create PostgreSQL database '{db_name}': {e}. "
                    f"If using restricted user, ensure database is pre-created or user has CREATE DATABASE permission. "
                    f"See docs/POSTGRESQL_SECURITY.md for sandboxed setup."
                )

        elif db_type == "mysql":
            try:
                import mysql.connector
            except ImportError:
                logging.warning("mysql.connector not available; skipping pre-flight DB creation for MySQL")
                return

            try:
                # Validate identifier to prevent SQL injection (must be alphanumeric or underscore)
                if not db_name or not all(c.isalnum() or c == '_' for c in db_name):
                    raise ValueError(f"Invalid database name: {db_name}. Database names must contain only alphanumeric characters and underscores.")

                conn = mysql.connector.connect(host=db_host, port=db_port, user=db_user, password=db_pass)
                cursor = conn.cursor()
                cursor.execute(f"CREATE DATABASE IF NOT EXISTS `{db_name}`")
                cursor.close()
                conn.close()
                logging.info(f"Ensured MySQL database '{db_name}' exists at {db_host}:{db_port}")
            except Exception as e:
                logging.warning(f"Failed to pre-create MySQL database '{db_name}': {e}. Migrations may handle creation.")

        else:
            logging.warning(f"Unknown database type '{db_type}'; skipping pre-flight DB creation")

    except Exception as e:
        logging.warning(f"Unexpected error in ensure_database_exists: {e}")


class TransactionLogger:
    """
    Backward compatibility re-export.

    TransactionLogger moved to api.db.transaction module.
    """

    @staticmethod
    def log_transaction_state(db, operation="begin", extra_info=None):
        from api.db.transaction import TransactionLogger as TL

        return TL.log_transaction_state(db, operation, extra_info)

    @staticmethod
    def log_transaction_error(db, exception, context=None):
        from api.db.transaction import TransactionLogger as TL

        return TL.log_transaction_error(db, exception, context)


@singleton
class BaseDataBase:
    def __init__(self):
        # Ensure database exists before creating connection pool
        ensure_database_exists()

        # Import at runtime to avoid circular dependency issues
        from api.db.pool import PooledDatabase

        database_config = (settings.DATABASE or {}).copy()
        db_name = database_config.pop("name")

        pool_config = {
            "max_retries": 5,
            "retry_delay": 1,
        }
        database_config.update(pool_config)
        self.database_connection = PooledDatabase[settings.DATABASE_TYPE.upper()].value(db_name, **database_config)

        # Log initial pool configuration
        db_type = settings.DATABASE_TYPE.upper()
        max_conn = database_config.get("max_connections", 32)
        logging.info(f"Initialized {db_type} connection pool: max_connections={max_conn}, max_retries={pool_config['max_retries']}, retry_delay={pool_config['retry_delay']}s")

        # Log initial pool stats
        stats = PoolDiagnostics.get_pool_stats(self.database_connection)
        logging.info(f"Connection pool stats: {stats}")

        # Start background health monitoring
        PoolDiagnostics.start_health_monitoring(self.database_connection)

        logging.info("Database connection pool initialized")


# Initialize DB singleton
DB = BaseDataBase().database_connection
DB.lock = DatabaseLock[settings.DATABASE_TYPE.upper()].value  # type: ignore[attr-defined]


def close_connection():
    """Close stale database connections."""
    try:
        if DB:
            DB.close_stale(age=30)
    except Exception:
        logging.exception("Failed to close stale DB connections")


def log_connection_stats():
    """
    Log current connection pool statistics

    This is a convenience function that can be called from anywhere
    to check the current state of the connection pool.
    """
    try:
        if DB:
            PoolDiagnostics.log_pool_health(DB)
    except Exception as e:
        logging.error(f"Failed to log connection stats: {e}")


def wait_for_schema_ready(max_retries: int = 30, retry_delay: float = 0.5):
    """
    Wait for database schema to be ready before accessing tables.

    This ensures init_database_tables() has completed before any code
    tries to access all critical tables. Prevents race conditions
    during startup.

    Args:
        max_retries: Maximum number of retry attempts (30 retries * 0.5s = 15s timeout)
        retry_delay: Delay in seconds between retries

    Raises:
        RuntimeError: If schema is not ready after max_retries
    """
    import time

    critical_tables = ["user", "sync_logs", "system_settings"]
    # Use portable identifier quoting across DBs: Postgres uses double quotes, MySQL uses backticks
    db_type = settings.DATABASE_TYPE.lower()
    quote_char = '"' if db_type == "postgres" else '`'

    for attempt in range(max_retries):
        try:
            # Try to query all critical tables to verify schema exists
            for table in critical_tables:
                cursor = None
                try:
                    cursor = DB.execute_sql(f"SELECT 1 FROM {quote_char}{table}{quote_char} LIMIT 1")
                finally:
                    if cursor:
                        cursor.close()
            logging.info(f"✓ Database schema is ready (attempt {attempt + 1}/{max_retries})")
            return
        except Exception as e:
            if attempt < max_retries - 1:
                logging.debug(f"Schema not ready yet (attempt {attempt + 1}/{max_retries}): {e}")
                time.sleep(retry_delay)
            else:
                logging.error(f"✗ Database schema still not ready after {max_retries} attempts")
                raise RuntimeError(f"Database schema initialization timeout. Critical tables {critical_tables} not accessible after {max_retries * retry_delay}s") from e


# Backward compatibility: re-export lock classes from locks module
from api.db.locks import MysqlDatabaseLock, PostgresDatabaseLock  # noqa: E402, F401

# Backward compatibility: re-export pool classes from pool module
from api.db.pool import (  # noqa: E402, F401
    PooledDatabase,
    RetryingPooledMySQLDatabase,
    RetryingPooledPostgresqlDatabase,
    with_retry,
)

# Also export playhouse pooled database classes for tests
from playhouse.pool import PooledMySQLDatabase, PooledPostgresqlDatabase  # noqa: E402, F401

__all__ = [
    "BaseDataBase",
    "DB",
    "DatabaseLock",
    "MysqlDatabaseLock",
    "PostgresDatabaseLock",
    "PoolDiagnostics",
    "PooledDatabase",
    "RetryingPooledMySQLDatabase",
    "RetryingPooledPostgresqlDatabase",
    "PooledMySQLDatabase",
    "PooledPostgresqlDatabase",
    "with_retry",
    "get_database_config",
    "ensure_database_exists",
    "wait_for_schema_ready",
    "close_connection",
    "log_connection_stats",
    "TransactionLogger",
]

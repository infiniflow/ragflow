#
# Database connection pooling and retry logic
#
from __future__ import annotations

import logging
import time
from enum import Enum
from functools import wraps
from typing import Callable, TypeVar

from peewee import InterfaceError, OperationalError
from playhouse.migrate import MySQLMigrator, PostgresqlMigrator
from playhouse.pool import PooledMySQLDatabase, PooledPostgresqlDatabase

F = TypeVar("F", bound=Callable)


def with_retry(
    max_retries: int = 3,
    retry_delay: float = 1.0,
    retry_predicate: Callable[[Exception], bool] | None = None,
):
    """
    Decorator that retries a function call on transient errors.

    Args:
        max_retries: Maximum number of retry attempts
        retry_delay: Initial delay between retries (exponential backoff applied)
        retry_predicate: Optional callable that takes an exception and returns True if retryable.
                        If None, no retries are attempted (function calls through once).

    Returns:
        The decorated function. On failure after all retries, raises the last exception.
    """

    def decorator(func: F) -> F:
        @wraps(func)
        def wrapper(*args, **kwargs):  # type: ignore[override]
            for attempt in range(max_retries + 1):
                try:
                    return func(*args, **kwargs)
                except Exception as exc:  # noqa: BLE001
                    func_name = getattr(func, "__name__", "unknown")

                    # Check if this exception is retryable
                    is_retryable = retry_predicate(exc) if retry_predicate else False
                    should_retry = is_retryable and attempt < max_retries

                    if should_retry:
                        current_delay = retry_delay * (2**attempt)
                        logging.warning(f"{func_name} failed: {exc}, retrying ({attempt + 1}/{max_retries + 1})")
                        time.sleep(current_delay)
                    else:
                        logging.error(f"{func_name} failed after {attempt + 1} attempt(s): {exc}")
                        raise

        return wrapper  # type: ignore[return-value]

    return decorator


class RetryingPooledMySQLDatabase(PooledMySQLDatabase):
    """MySQL connection pool with automatic retry on transient errors."""

    def __init__(self, *args, **kwargs):
        self.max_retries = kwargs.pop("max_retries", 5)
        self.retry_delay = kwargs.pop("retry_delay", 1)
        super().__init__(*args, **kwargs)

    def execute_sql(self, sql, params=None, commit=True):
        """Execute SQL with automatic retry on connection loss."""
        from api.db.diagnostics import PoolDiagnostics

        for attempt in range(self.max_retries + 1):
            try:
                result = super().execute_sql(sql, params, commit)
                if attempt == 0:
                    logging.debug("MySQL query executed successfully")
                return result
            except (OperationalError, InterfaceError) as e:
                should_retry = self._should_retry_mysql_error(e)

                if should_retry and attempt < self.max_retries:
                    logging.warning(f"MySQL connection issue (attempt {attempt + 1}/{self.max_retries}): {e}")
                    PoolDiagnostics.log_pool_health(self)
                    self._handle_connection_loss()
                    time.sleep(self.retry_delay * (2**attempt))
                else:
                    logging.error(f"MySQL execution failure after {attempt + 1} attempts: {e}")
                    PoolDiagnostics.log_pool_health(self)
                    raise

    def _should_retry_mysql_error(self, e):
        """Check if a MySQL error is transient and retryable."""
        error_codes = [2013, 2006]
        error_substrings = ["Lost connection", "MySQL server has gone away"]
        error_str = str(e)

        # Check for retryable errors: error code, substring match, or InterfaceError
        has_retryable_code = hasattr(e, "args") and e.args and e.args[0] in error_codes
        has_retryable_message = any(sub in error_str for sub in error_substrings)
        is_interface_error = isinstance(e, InterfaceError)
        return has_retryable_code or has_retryable_message or is_interface_error

    def _handle_connection_loss(self):
        """Close and reset connection pool after connection loss."""
        try:
            self.close()
            if hasattr(self, "_connections"):
                self._connections.clear()
        except Exception:
            pass
        try:
            self.connect()
        except Exception as e:
            logging.error(f"MySQL reconnection attempt 1 failed: {e}")
            time.sleep(0.1)
            try:
                self.connect()
            except Exception as e2:
                logging.error(f"MySQL reconnection attempt 2 failed: {e2}")
                raise

    def begin(self):
        """Begin transaction with retry on connection loss."""
        from api.db.diagnostics import PoolDiagnostics
        from api.db.transaction import TransactionLogger

        for attempt in range(self.max_retries + 1):
            try:
                result = super().begin()
                if attempt > 0:
                    logging.info(f"MySQL transaction started after {attempt} retries")
                else:
                    TransactionLogger.log_transaction_state(self, "begin")
                return result
            except (OperationalError, InterfaceError) as e:
                should_retry = self._should_retry_mysql_error(e)

                if should_retry and attempt < self.max_retries:
                    logging.warning(f"Lost MySQL connection during transaction (attempt {attempt + 1}/{self.max_retries})")
                    PoolDiagnostics.log_pool_health(self)
                    self._handle_connection_loss()
                    time.sleep(self.retry_delay * (2**attempt))
                else:
                    PoolDiagnostics.log_pool_health(self)
                    raise


class RetryingPooledPostgresqlDatabase(PooledPostgresqlDatabase):
    """PostgreSQL connection pool with automatic retry on transient errors."""

    def __init__(self, *args, **kwargs):
        self.max_retries = kwargs.pop("max_retries", 5)
        self.retry_delay = kwargs.pop("retry_delay", 1)
        super().__init__(*args, **kwargs)

    def execute_sql(self, sql, params=None, commit=True):
        """Execute SQL with automatic retry on connection loss."""
        from api.db.diagnostics import PoolDiagnostics

        for attempt in range(self.max_retries + 1):
            try:
                result = super().execute_sql(sql, params, commit)
                if attempt == 0:
                    logging.debug("PostgreSQL query executed successfully")
                return result
            except (OperationalError, InterfaceError) as e:
                should_retry = self._should_retry_postgres_error(e)

                if should_retry and attempt < self.max_retries:
                    logging.warning(f"PostgreSQL connection issue (attempt {attempt + 1}/{self.max_retries}): {e}")
                    PoolDiagnostics.log_pool_health(self)
                    self._handle_connection_loss()
                    time.sleep(self.retry_delay * (2**attempt))
                else:
                    logging.error(f"PostgreSQL execution failure after {attempt + 1} attempts: {e}")
                    PoolDiagnostics.log_pool_health(self)
                    raise

    def _handle_connection_loss(self):
        """Close and reset connection pool after connection loss."""
        try:
            self.close()
            if hasattr(self, "_connections"):
                self._connections.clear()
        except Exception:
            pass
        try:
            self.connect()
        except Exception as e:
            logging.error(f"Failed to reconnect to PostgreSQL (attempt 1): {e}")
            time.sleep(0.1)
            try:
                self.connect()
            except Exception as e2:
                logging.error(f"Failed to reconnect to PostgreSQL (attempt 2): {e2}")
                raise e2

    def _should_retry_postgres_error(self, e):
        """Check if a PostgreSQL error is transient and retryable."""
        transient_errors = [
            "lost connection",
            "connection lost",
            "server closed",
            "connection refused",
            "no connection to the server",
            "terminating connection",
            "could not connect",
            "connection timed out",
            "connection already closed",
            "connection reset by peer",
        ]
        error_str = str(e).lower()
        return any(msg in error_str for msg in transient_errors) or isinstance(e, InterfaceError)

    def begin(self):
        """Begin transaction with retry on connection loss."""
        from api.db.diagnostics import PoolDiagnostics
        from api.db.transaction import TransactionLogger

        for attempt in range(self.max_retries + 1):
            try:
                result = super().begin()
                if attempt > 0:
                    logging.info(f"PostgreSQL transaction started after {attempt} retries")
                else:
                    TransactionLogger.log_transaction_state(self, "begin")
                return result
            except (OperationalError, InterfaceError) as e:
                should_retry = self._should_retry_postgres_error(e)

                if should_retry and attempt < self.max_retries:
                    logging.warning(f"PostgreSQL connection lost during transaction (attempt {attempt + 1}/{self.max_retries})")
                    PoolDiagnostics.log_pool_health(self)
                    self._handle_connection_loss()
                    time.sleep(self.retry_delay * (2**attempt))
                else:
                    PoolDiagnostics.log_pool_health(self)
                    raise


class PooledDatabase(Enum):
    """Enum for selecting between MySQL and PostgreSQL pooled databases."""

    MYSQL = RetryingPooledMySQLDatabase
    POSTGRES = RetryingPooledPostgresqlDatabase


class DatabaseMigrator(Enum):
    """Enum for selecting between MySQL and PostgreSQL migrators."""

    MYSQL = MySQLMigrator
    POSTGRES = PostgresqlMigrator

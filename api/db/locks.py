#
# Database locking primitives for MySQL and PostgreSQL
#
import hashlib
import time
from enum import Enum
from functools import wraps
from typing import Callable, TypeVar

from playhouse.pool import PooledMySQLDatabase, PooledPostgresqlDatabase

from api.db.pool import with_retry

F = TypeVar("F", bound=Callable)


class PostgresDatabaseLock:
    """PostgreSQL advisory lock using native pg_advisory_lock functions.

    WARNING: Advisory locks are session-bound. When using with connection pooling,
    the same connection must be held for the lock's lifetime. The lock will NOT
    be automatically released if the connection is returned to the pool.

    For connection-pooled environments, consider using transaction-scoped locks
    (pg_advisory_xact_lock) which are automatically released on transaction commit/rollback.
    """

    def __init__(self, lock_name, timeout=10, db=None):
        self.lock_name = lock_name
        self.lock_id = int(hashlib.md5(lock_name.encode()).hexdigest(), 16) % (2**31 - 1)
        self.timeout = int(timeout)
        # db will be injected at runtime from connection module to avoid circular imports
        self.db = db

    def _get_db(self):
        """Get database connection, deferring import to avoid circular dependency."""
        if self.db is None:
            from api.db.connection import DB

            self.db = DB
        return self.db

    @with_retry(max_retries=3, retry_delay=1.0)
    def lock(self):
        """
        Acquire PostgreSQL advisory lock with timeout.

        Mimics MySQL GET_LOCK behavior by retrying pg_try_advisory_lock
        until the lock is acquired or timeout is reached. This ensures
        consistent behavior between MySQL and PostgreSQL implementations.
        """
        db = self._get_db()
        start_time = time.time()
        retry_interval = 0.1  # 100ms between retries

        while True:
            cursor = db.execute_sql("SELECT pg_try_advisory_lock(%s)", (self.lock_id,))
            if cursor is None:
                raise RuntimeError("postgres lock acquisition returned no cursor")
            ret = cursor.fetchone()

            # Lock acquired successfully
            if ret[0] == 1:
                return True

            # Lock is held by another session - check if we should retry
            # ret[0] == 0 means another session currently holds the lock
            elapsed = time.time() - start_time
            if elapsed >= self.timeout:
                raise Exception(f"acquire postgres lock {self.lock_name} timeout after {elapsed:.2f}s: lock is held by another session")

            # Wait before retrying, but don't exceed timeout
            remaining = self.timeout - elapsed
            sleep_time = min(retry_interval, remaining)
            if sleep_time > 0:
                time.sleep(sleep_time)
            else:
                raise Exception(f"acquire postgres lock {self.lock_name} timeout: lock is held by another session")

    @with_retry(max_retries=3, retry_delay=1.0)
    def unlock(self):
        """Release PostgreSQL advisory lock."""
        db = self._get_db()
        cursor = db.execute_sql("SELECT pg_advisory_unlock(%s)", (self.lock_id,))
        if cursor is None:
            raise RuntimeError("postgres unlock returned no cursor")
        ret = cursor.fetchone()
        if ret[0] == 0:
            raise Exception(f"postgres lock {self.lock_name} was not established by this thread")
        if ret[0] == 1:
            return True
        raise Exception(f"postgres lock {self.lock_name} does not exist")

    def __enter__(self):
        db = self._get_db()
        if not isinstance(db, PooledPostgresqlDatabase):
            raise RuntimeError(f"PostgreSQL advisory locks are only supported for PooledPostgresqlDatabase. Current database type: {type(db).__name__}. Lock '{self.lock_name}' cannot be acquired.")
        self.lock()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        db = self._get_db()
        if not isinstance(db, PooledPostgresqlDatabase):
            raise RuntimeError(f"PostgreSQL advisory locks are only supported for PooledPostgresqlDatabase. Current database type: {type(db).__name__}. Lock '{self.lock_name}' cannot be released.")
        self.unlock()

    def __call__(self, func: F) -> F:
        @wraps(func)
        def magic(*args, **kwargs):  # type: ignore[override]
            # Wrap the entire locked operation in an atomic transaction/savepoint
            # This ensures that if the locked operation fails, the transaction is
            # properly rolled back and the connection isn't left in an aborted state
            # for the next user of the pool.
            db = self._get_db()
            with db.atomic():
                with self:
                    return func(*args, **kwargs)

        return magic  # type: ignore[return-value]


class MysqlDatabaseLock:
    """MySQL named lock using native GET_LOCK and RELEASE_LOCK functions."""

    def __init__(self, lock_name, timeout=10, db=None):
        self.lock_name = lock_name
        self.timeout = int(timeout)
        # db will be injected at runtime from connection module to avoid circular imports
        self.db = db

    def _get_db(self):
        """Get database connection, deferring import to avoid circular dependency."""
        if self.db is None:
            from api.db.connection import DB

            self.db = DB
        return self.db

    @with_retry(max_retries=3, retry_delay=1.0)
    def lock(self):
        """
        Acquire MySQL named lock.

        MySQL GET_LOCK returns:
        - 1 if the lock was obtained successfully
        - 0 if the attempt timed out
        - NULL if an error occurred (out of memory, thread killed, etc.)
        """
        db = self._get_db()
        cursor = db.execute_sql("SELECT GET_LOCK(%s, %s)", (self.lock_name, self.timeout))
        if cursor is None:
            raise RuntimeError("mysql lock acquisition returned no cursor")
        ret = cursor.fetchone()

        # Check for NULL result - indicates an error (not timeout or lock held)
        if ret is None or ret[0] is None:
            raise RuntimeError(f"mysql lock {self.lock_name} acquisition failed: GET_LOCK returned NULL (possible error: out of memory, thread killed, or system failure)")

        # Check for timeout
        if ret[0] == 0:
            # Non-retriable lock acquisition timeout: signal with TimeoutError so
            # any retry wrappers will not retry (only transient DB errors should).
            raise TimeoutError(f"acquire mysql lock {self.lock_name} timeout")

        # Check for success
        if ret[0] == 1:
            return True

        # Unexpected return value
        raise Exception(f"failed to acquire lock {self.lock_name}: unexpected GET_LOCK return value {ret[0]}")

    @with_retry(max_retries=3, retry_delay=1.0)
    def unlock(self):
        """
        Release MySQL named lock.

        MySQL RELEASE_LOCK returns:
        - 1 if the lock was released successfully
        - 0 if the lock was not established by this thread
        - NULL if the named lock did not exist
        """
        db = self._get_db()
        cursor = db.execute_sql("SELECT RELEASE_LOCK(%s)", (self.lock_name,))
        if cursor is None:
            raise RuntimeError("mysql unlock returned no cursor")
        ret = cursor.fetchone()

        # Check for NULL result - indicates lock did not exist
        if ret is None or ret[0] is None:
            raise RuntimeError(f"mysql lock {self.lock_name} release failed: RELEASE_LOCK returned NULL (lock did not exist)")

        # Check if lock was not held by this thread
        if ret[0] == 0:
            raise Exception(f"mysql lock {self.lock_name} was not established by this thread")

        # Check for success
        if ret[0] == 1:
            return True

        # Unexpected return value
        raise Exception(f"failed to release lock {self.lock_name}: unexpected RELEASE_LOCK return value {ret[0]}")

    def __enter__(self):
        db = self._get_db()
        if not isinstance(db, PooledMySQLDatabase):
            raise RuntimeError(f"MySQL named locks are only supported for PooledMySQLDatabase. Current database type: {type(db).__name__}. Lock '{self.lock_name}' cannot be acquired.")
        self.lock()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        db = self._get_db()
        if not isinstance(db, PooledMySQLDatabase):
            raise RuntimeError(f"MySQL named locks are only supported for PooledMySQLDatabase. Current database type: {type(db).__name__}. Lock '{self.lock_name}' cannot be released.")
        self.unlock()

    def __call__(self, func: F) -> F:
        @wraps(func)
        def magic(*args, **kwargs):  # type: ignore[override]
            with self:
                return func(*args, **kwargs)

        return magic  # type: ignore[return-value]


class DatabaseLock(Enum):
    """Enum for selecting between MySQL and PostgreSQL lock implementations."""

    MYSQL = MysqlDatabaseLock
    POSTGRES = PostgresDatabaseLock

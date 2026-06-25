"""
Tests for PostgreSQL advisory lock behavior.
"""

from contextlib import contextmanager
from unittest.mock import MagicMock

import pytest
from peewee import OperationalError
from playhouse.pool import PooledPostgresqlDatabase

from api.db.db_models import PostgresDatabaseLock, PostgresLockTimeoutError


@contextmanager
def _null_connection_context():
    yield


def _pg_pool_db(**kwargs):
    db = MagicMock(spec=PooledPostgresqlDatabase, **kwargs)
    db.connection_context.return_value = _null_connection_context()
    return db


class TestPostgresDatabaseLock:
    @pytest.mark.p1
    def test_lock_blocks_indefinitely_when_timeout_negative(self):
        db = _pg_pool_db()
        lock = PostgresDatabaseLock("update_progress", timeout=-1, db=db)

        with lock:
            pass

        assert db.connection_context.called
        assert db.execute_sql.call_args_list == [
            (("SET lock_timeout = %s", ("0",)),),
            (("SELECT pg_advisory_lock(%s)", (lock.lock_id,)),),
            (("SELECT pg_advisory_unlock(%s)", (lock.lock_id,)),),
            (("SET lock_timeout = DEFAULT",),),
        ]

    @pytest.mark.p2
    def test_lock_uses_postgres_lock_timeout(self):
        db = _pg_pool_db()
        lock = PostgresDatabaseLock("update_progress", timeout=5, db=db)

        with lock:
            pass

        assert db.execute_sql.call_args_list == [
            (("SET lock_timeout = %s", ("5s",)),),
            (("SELECT pg_advisory_lock(%s)", (lock.lock_id,)),),
            (("SELECT pg_advisory_unlock(%s)", (lock.lock_id,)),),
            (("SET lock_timeout = DEFAULT",),),
        ]

    @pytest.mark.p2
    def test_lock_raises_when_postgres_lock_timeout(self):
        db = _pg_pool_db()
        db.execute_sql.side_effect = [
            None,
            OperationalError("canceling statement due to lock timeout"),
        ]
        lock = PostgresDatabaseLock("update_progress", timeout=5, db=db)

        with pytest.raises(PostgresLockTimeoutError, match="acquire postgres lock update_progress timeout"):
            with lock:
                pass

        assert db.execute_sql.call_count == 3
        db.execute_sql.assert_any_call("SET lock_timeout = %s", ("5s",))
        db.execute_sql.assert_any_call("SET lock_timeout = DEFAULT")

    @pytest.mark.p2
    def test_unlock_warns_when_lock_not_held_by_session(self):
        db = _pg_pool_db()
        db.execute_sql.side_effect = [
            None,
            None,
            MagicMock(fetchone=MagicMock(return_value=(0,))),
            None,
        ]
        lock = PostgresDatabaseLock("update_progress", timeout=-1, db=db)

        with lock:
            pass

        db.execute_sql.assert_any_call("SELECT pg_advisory_unlock(%s)", (lock.lock_id,))

    @pytest.mark.p2
    def test_acquire_lock_reconnects_on_connection_error(self, monkeypatch):
        db = MagicMock()
        db.max_retries = 2
        db.retry_delay = 0
        db._handle_connection_loss = MagicMock()
        db.execute_sql.side_effect = [
            OperationalError("connection refused"),
            None,
            None,
        ]
        lock = PostgresDatabaseLock("update_progress", timeout=-1, db=db)
        monkeypatch.setattr("api.db.db_models.time.sleep", lambda _seconds: None)

        assert lock._acquire_lock() is True

        db._handle_connection_loss.assert_called_once()
        assert db.execute_sql.call_count == 3

    @pytest.mark.p2
    def test_acquire_lock_does_not_retry_lock_timeout(self):
        db = MagicMock()
        db.max_retries = 5
        db.retry_delay = 0
        db._handle_connection_loss = MagicMock()
        db.execute_sql.side_effect = [
            None,
            OperationalError("canceling statement due to lock timeout"),
        ]
        lock = PostgresDatabaseLock("update_progress", timeout=5, db=db)

        with pytest.raises(PostgresLockTimeoutError):
            lock._acquire_lock()

        db._handle_connection_loss.assert_not_called()

    @pytest.mark.p2
    def test_reconnect_db_uses_handle_connection_loss(self):
        db = MagicMock()
        db._handle_connection_loss = MagicMock()
        lock = PostgresDatabaseLock("update_progress", db=db)

        lock._reconnect_db()

        db._handle_connection_loss.assert_called_once()
        db.close.assert_not_called()

    @pytest.mark.p2
    def test_reconnect_db_falls_back_to_close_and_connect(self):
        db = MagicMock()
        lock = PostgresDatabaseLock("update_progress", db=db)

        lock._reconnect_db()

        db.close.assert_called_once()
        db.connect.assert_called_once()

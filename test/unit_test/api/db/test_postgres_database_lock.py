"""
Tests for PostgreSQL advisory lock behavior.
"""

from contextlib import contextmanager
from unittest.mock import MagicMock

import pytest
from peewee import OperationalError

from api.db.db_models import PostgresDatabaseLock, PostgresLockTimeoutError


@contextmanager
def _null_connection_context():
    yield


class TestPostgresDatabaseLock:
    @pytest.mark.p1
    def test_lock_blocks_indefinitely_when_timeout_negative(self):
        db = MagicMock()
        db.connection_context.return_value = _null_connection_context()
        lock = PostgresDatabaseLock("update_progress", timeout=-1, db=db)

        with lock:
            pass

        assert db.execute_sql.call_args_list == [
            (("SET lock_timeout = %s", ("0",)),),
            (("SELECT pg_advisory_lock(%s)", (lock.lock_id,)),),
            (("SELECT pg_advisory_unlock(%s)", (lock.lock_id,)),),
            (("SET lock_timeout = DEFAULT",),),
        ]

    @pytest.mark.p2
    def test_lock_uses_postgres_lock_timeout(self):
        db = MagicMock()
        db.connection_context.return_value = _null_connection_context()
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
        db = MagicMock()
        db.connection_context.return_value = _null_connection_context()
        db.execute_sql.side_effect = [
            None,
            OperationalError("canceling statement due to lock timeout"),
        ]
        lock = PostgresDatabaseLock("update_progress", timeout=5, db=db)

        with pytest.raises(PostgresLockTimeoutError, match="acquire postgres lock update_progress timeout"):
            with lock:
                pass

        assert db.execute_sql.call_count == 2
        db.execute_sql.assert_any_call("SET lock_timeout = %s", ("5s",))

    @pytest.mark.p2
    def test_unlock_warns_when_lock_not_held_by_session(self):
        db = MagicMock()
        db.connection_context.return_value = _null_connection_context()
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

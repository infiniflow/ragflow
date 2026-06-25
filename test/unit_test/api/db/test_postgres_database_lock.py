"""
Tests for PostgreSQL advisory lock behavior.
"""

from unittest.mock import MagicMock

import pytest
from peewee import OperationalError

from api.db.db_models import PostgresDatabaseLock


class TestPostgresDatabaseLock:
    def test_lock_blocks_indefinitely_when_timeout_negative(self):
        db = MagicMock()
        lock = PostgresDatabaseLock("update_progress", timeout=-1, db=db)

        lock.lock()

        db.execute_sql.assert_called_once_with("SELECT pg_advisory_lock(%s)", (lock.lock_id,))

    def test_lock_uses_postgres_lock_timeout(self):
        db = MagicMock()
        lock = PostgresDatabaseLock("update_progress", timeout=5, db=db)

        lock.lock()

        assert db.execute_sql.call_args_list == [
            (("SET lock_timeout = %s", ("5s",)),),
            (("SELECT pg_advisory_lock(%s)", (lock.lock_id,)),),
            (("SET lock_timeout = DEFAULT",),),
        ]

    def test_lock_raises_when_postgres_lock_timeout(self):
        db = MagicMock()
        db.execute_sql.side_effect = [
            None,
            OperationalError("canceling statement due to lock timeout"),
            None,
        ]
        lock = PostgresDatabaseLock("update_progress", timeout=5, db=db)

        with pytest.raises(Exception, match="acquire postgres lock update_progress timeout"):
            lock.lock()

        db.execute_sql.assert_any_call("SET lock_timeout = DEFAULT")

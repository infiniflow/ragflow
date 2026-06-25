"""
Tests for PostgreSQL advisory lock behavior.
"""

from unittest.mock import MagicMock

import pytest

from api.db.db_models import PostgresDatabaseLock


def _try_lock_result(acquired: int):
    cursor = MagicMock()
    cursor.fetchone.return_value = (acquired,)
    return cursor


class TestPostgresDatabaseLock:
    def test_lock_blocks_indefinitely_when_timeout_negative(self):
        db = MagicMock()
        lock = PostgresDatabaseLock("update_progress", timeout=-1, db=db)

        lock.lock()

        db.execute_sql.assert_called_once_with("SELECT pg_advisory_lock(%s)", (lock.lock_id,))

    def test_lock_polls_until_acquired(self):
        db = MagicMock()
        db.execute_sql.side_effect = [_try_lock_result(0), _try_lock_result(1)]
        lock = PostgresDatabaseLock("update_progress", timeout=5, db=db)

        lock.lock()

        assert db.execute_sql.call_count == 2
        db.execute_sql.assert_any_call("SELECT pg_try_advisory_lock(%s)", (lock.lock_id,))

    def test_lock_raises_when_timeout_elapsed(self, monkeypatch):
        db = MagicMock()
        db.execute_sql.return_value = _try_lock_result(0)
        lock = PostgresDatabaseLock("update_progress", timeout=0, db=db)

        timeline = iter([0.0, 0.0])
        monkeypatch.setattr("api.db.db_models.time.monotonic", lambda: next(timeline))
        monkeypatch.setattr("api.db.db_models.time.sleep", lambda _seconds: None)

        with pytest.raises(Exception, match="acquire postgres lock update_progress timeout"):
            lock.lock()

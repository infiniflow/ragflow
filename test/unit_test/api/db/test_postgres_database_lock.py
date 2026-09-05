from unittest.mock import MagicMock, call, patch

import pytest
from playhouse.pool import PooledPostgresqlDatabase

from api.db.db_models import PostgresDatabaseLock


def _cursor_with_results(*results):
    cursor = MagicMock()
    cursor.fetchone.side_effect = [(result,) for result in results]
    return cursor


@pytest.mark.p2
def test_postgres_lock_returns_immediately_when_available():
    db = MagicMock()
    db.execute_sql.return_value = _cursor_with_results(True)
    lock = PostgresDatabaseLock("add_chunk:1", timeout=5, db=db)

    with patch("api.db.db_models.time.sleep") as sleep:
        assert lock.lock()

    sleep.assert_not_called()
    db.execute_sql.assert_called_once_with("SELECT pg_try_advisory_lock(%s)", (lock.lock_id,))


@pytest.mark.p2
def test_postgres_lock_waits_through_contention_until_available():
    db = MagicMock()
    db.execute_sql.return_value = _cursor_with_results(False, False, True)

    with patch("api.db.db_models.time.sleep") as sleep:
        assert PostgresDatabaseLock("add_chunk:1", timeout=5, db=db).lock()

    assert db.execute_sql.call_count == 3
    assert sleep.call_args_list == [call(0.1), call(0.1)]


@pytest.mark.p2
def test_postgres_lock_stops_after_supplied_timeout_without_retrying_timeout():
    db = MagicMock()
    cursor = MagicMock()
    cursor.fetchone.return_value = (False,)
    db.execute_sql.return_value = cursor
    lock = PostgresDatabaseLock("add_chunk:1", timeout=0.25, db=db)
    now = [20.0]

    def _sleep(seconds):
        now[0] += seconds

    with (
        patch("api.db.db_models.time.monotonic", side_effect=lambda: now[0]),
        patch("api.db.db_models.time.sleep", side_effect=_sleep),
        patch("api.db.db_models.logging.warning") as warning,
        pytest.raises(Exception, match="acquire postgres lock add_chunk:1 timeout"),
    ):
        lock.lock()

    assert now[0] == pytest.approx(20.25)
    assert db.execute_sql.call_count == 4
    warning.assert_called_once_with(
        "Timed out acquiring %s advisory lock: lock_id=%s timeout=%s",
        "postgres",
        lock.lock_id,
        0.25,
    )


@pytest.mark.p2
def test_postgres_lock_timeout_zero_only_tries_once():
    db = MagicMock()
    db.execute_sql.return_value = _cursor_with_results(False)

    with (
        patch("api.db.db_models.time.monotonic", return_value=30.0),
        patch("api.db.db_models.time.sleep") as sleep,
        pytest.raises(Exception, match="acquire postgres lock add_chunk:1 timeout"),
    ):
        PostgresDatabaseLock("add_chunk:1", timeout=0, db=db).lock()

    db.execute_sql.assert_called_once()
    sleep.assert_not_called()


@pytest.mark.p2
def test_postgres_lock_negative_timeout_waits_indefinitely():
    db = MagicMock()
    db.execute_sql.return_value = _cursor_with_results(None)
    lock = PostgresDatabaseLock("update_progress", timeout=-1, db=db)

    with (
        patch("api.db.db_models.time.monotonic") as monotonic,
        patch("api.db.db_models.time.sleep") as sleep,
    ):
        assert lock.lock()

    db.execute_sql.assert_called_once_with("SELECT pg_advisory_lock(%s)", (lock.lock_id,))
    monotonic.assert_not_called()
    sleep.assert_not_called()


@pytest.mark.p2
def test_postgres_lock_propagates_database_error():
    db = MagicMock()
    db.execute_sql.side_effect = RuntimeError("database unavailable")

    with (
        patch("api.db.db_models.time.sleep") as sleep,
        pytest.raises(RuntimeError, match="database unavailable"),
    ):
        PostgresDatabaseLock("add_chunk:1", timeout=5, db=db).lock()

    db.execute_sql.assert_called_once()
    sleep.assert_not_called()


@pytest.mark.p2
def test_postgres_lock_rejects_unexpected_database_result():
    db = MagicMock()
    cursor = MagicMock()
    cursor.fetchone.return_value = (None,)
    db.execute_sql.return_value = cursor

    with (
        patch("api.db.db_models.time.sleep") as sleep,
        pytest.raises(Exception, match="failed to acquire lock add_chunk:1"),
    ):
        PostgresDatabaseLock("add_chunk:1", timeout=5, db=db).lock()

    db.execute_sql.assert_called_once()
    sleep.assert_not_called()


@pytest.mark.p2
def test_postgres_lock_context_acquires_and_releases_on_postgres_pool():
    db = MagicMock(spec=PooledPostgresqlDatabase)
    acquire_cursor = _cursor_with_results(True)
    release_cursor = _cursor_with_results(True)
    db.execute_sql.side_effect = [acquire_cursor, release_cursor]
    lock = PostgresDatabaseLock("add_chunk:1", timeout=5, db=db)

    with lock:
        pass

    assert db.execute_sql.call_args_list == [
        call("SELECT pg_try_advisory_lock(%s)", (lock.lock_id,)),
        call("SELECT pg_advisory_unlock(%s)", (lock.lock_id,)),
    ]

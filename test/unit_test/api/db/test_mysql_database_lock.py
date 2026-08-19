"""
Tests for MySQL/MariaDB advisory lock timeouts.
"""

import pytest

from api.db.db_models import MysqlDatabaseLock


class TestMysqlDatabaseLockTimeout:
    """MariaDB returns NULL for a negative GET_LOCK() timeout, so negative
    timeouts have to be turned into a finite wait."""

    @pytest.mark.parametrize("timeout", [-1, -60])
    def test_negative_timeout_falls_back_to_blocking_timeout(self, timeout):
        lock = MysqlDatabaseLock("unit_test_lock", timeout)
        assert lock.timeout == MysqlDatabaseLock.BLOCKING_TIMEOUT

    @pytest.mark.parametrize("timeout", [0, 10, 60])
    def test_non_negative_timeout_is_preserved(self, timeout):
        lock = MysqlDatabaseLock("unit_test_lock", timeout)
        assert lock.timeout == timeout

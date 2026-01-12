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
"""
Unit tests for connection pool handling, specifically testing the fix
for the PostgreSQL "connection already closed" issue.
"""

import sys
from unittest.mock import Mock, MagicMock, patch

import pytest
from peewee import OperationalError
from playhouse.pool import PooledMySQLDatabase, PooledPostgresqlDatabase


@pytest.fixture(autouse=True)
def mock_common_modules(monkeypatch):
    """
    Mock common modules before importing api.db.connection.
    Uses monkeypatch for proper cleanup and isolation between tests.
    """
    # Create mocks for common modules
    mock_settings = MagicMock()
    mock_settings.DATABASE_TYPE = "postgres"
    mock_settings.DATABASE = {"name": "test_db"}

    # Inject mocks into sys.modules
    monkeypatch.setitem(sys.modules, "common.settings", mock_settings)
    monkeypatch.setitem(sys.modules, "common.config_utils", MagicMock())
    monkeypatch.setitem(sys.modules, "common.decorator", MagicMock())

    yield

    # Cleanup: monkeypatch automatically reverts sys.modules on fixture teardown


@pytest.fixture
def pg_db_class(mock_common_modules):
    """Import Postgres pool class after mocks are applied."""
    from api.db.connection import RetryingPooledPostgresqlDatabase  # noqa: E402

    return RetryingPooledPostgresqlDatabase


@pytest.fixture
def mysql_db_class(mock_common_modules, monkeypatch):
    """Import MySQL pool class after mocks are applied and set mysql db type."""
    mock_settings = sys.modules.get("common.settings")
    if mock_settings:
        monkeypatch.setattr(mock_settings, "DATABASE_TYPE", "mysql", raising=False)
    from api.db.connection import RetryingPooledMySQLDatabase  # noqa: E402

    return RetryingPooledMySQLDatabase


class TestPostgreSQLConnectionPoolFix:
    """Test the fix for PostgreSQL connection pool reusing closed connections"""

    def test_handle_connection_loss_clears_pool(self, pg_db_class):
        """Test that _handle_connection_loss properly clears the connection pool"""
        db = pg_db_class("test", user="test", password="test")

        # Mock the _connections attribute that exists in pooled databases
        db._connections = MagicMock()
        db._connections.clear = Mock()

        # Mock close and connect methods
        with patch.object(db, 'close') as mock_close, \
             patch.object(db, 'connect') as mock_connect:

            db._handle_connection_loss()

            # Verify close was called
            mock_close.assert_called_once()

            # Verify pool was cleared
            db._connections.clear.assert_called_once()

            # Verify reconnect was attempted
            mock_connect.assert_called_once()

    def test_handle_connection_loss_without_connections_attribute(self, pg_db_class):
        """Test that _handle_connection_loss works even if _connections doesn't exist"""
        db = pg_db_class("test", user="test", password="test")

        # Ensure _connections doesn't exist
        if hasattr(db, '_connections'):
            delattr(db, '_connections')

        # Should not raise an error
        with patch.object(db, 'close'), \
             patch.object(db, 'connect'):
            db._handle_connection_loss()  # Should complete without error

    def test_execute_sql_retries_and_clears_pool_on_connection_error(self, pg_db_class):
        """Test that execute_sql properly handles connection errors and clears pool"""
        db = pg_db_class("test", user="test", password="test", max_retries=2)
        db._connections = MagicMock()
        db._connections.clear = Mock()

        call_count = 0

        def mock_execute_sql(*args, **kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                # First call fails with "connection already closed"
                raise OperationalError("connection already closed")
            # Second call succeeds
            return Mock()

        with patch.object(PooledPostgresqlDatabase, 'execute_sql', side_effect=mock_execute_sql), \
             patch.object(db, 'close'), \
             patch.object(db, 'connect'):

            result = db.execute_sql("SELECT 1")

            # Verify it succeeded after retry
            assert result is not None

            # Verify pool was cleared during retry
            assert db._connections.clear.call_count >= 1

            # Verify retry happened
            assert call_count == 2

    def test_begin_retries_and_clears_pool_on_connection_error(self, pg_db_class):
        """Test that begin() properly handles connection errors and clears pool"""
        db = pg_db_class("test", user="test", password="test", max_retries=2)
        db._connections = MagicMock()
        db._connections.clear = Mock()

        call_count = 0

        def mock_begin():
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                raise OperationalError("server closed the connection unexpectedly")
            return Mock()

        with patch.object(PooledPostgresqlDatabase, 'begin', side_effect=mock_begin), \
             patch.object(db, 'close'), \
             patch.object(db, 'connect'):

            result = db.begin()

            # Verify it succeeded after retry
            assert result is not None

            # Verify retry actually occurred
            assert call_count == 2

            # Verify pool was cleared during retry
            assert db._connections.clear.call_count >= 1


class TestMySQLConnectionPoolFix:
    """Test that MySQL connection pool handling matches PostgreSQL fix"""

    def test_handle_connection_loss_clears_pool(self, mysql_db_class):
        """Test that MySQL _handle_connection_loss also properly clears the connection pool"""
        db = mysql_db_class("test", user="test", password="test")

        # Mock the _connections attribute
        db._connections = MagicMock()
        db._connections.clear = Mock()

        with patch.object(db, 'close'), \
             patch.object(db, 'connect'):

            db._handle_connection_loss()

            # Verify pool was cleared (same as PostgreSQL)
            db._connections.clear.assert_called_once()

    def test_execute_sql_clears_pool_on_retry(self, mysql_db_class):
        """Test that MySQL execute_sql clears pool when retrying"""
        db = mysql_db_class("test", user="test", password="test", max_retries=2)
        db._connections = MagicMock()
        db._connections.clear = Mock()

        call_count = 0

        def mock_execute_sql(*args, **kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                # MySQL error code 2013 = Lost connection
                error = OperationalError()
                error.args = (2013,)
                raise error
            return Mock()

        with patch.object(PooledMySQLDatabase, 'execute_sql', side_effect=mock_execute_sql), \
             patch.object(db, 'close'), \
             patch.object(db, 'connect'):

            result = db.execute_sql("SELECT 1")

            assert result is not None
            assert db._connections.clear.call_count >= 1

    def test_begin_retries_and_clears_pool_on_connection_error(self, mysql_db_class):
        """Test that MySQL begin() properly handles connection errors and clears pool"""
        db = mysql_db_class("test", user="test", password="test", max_retries=2)
        db._connections = MagicMock()
        db._connections.clear = Mock()

        call_count = 0

        def mock_begin():
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                # MySQL error code 2006 = MySQL server has gone away
                error = OperationalError()
                error.args = (2006,)
                raise error
            return Mock()

        with patch.object(PooledMySQLDatabase, 'begin', side_effect=mock_begin), \
             patch.object(db, 'close'), \
             patch.object(db, 'connect'):

            result = db.begin()

            # Verify it succeeded after retry
            assert result is not None

            # Verify pool was cleared during retry
            assert db._connections.clear.call_count >= 1

            # Verify retry happened
            assert call_count == 2

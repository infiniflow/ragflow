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
Unit tests for database connection utilities.
These tests verify behavior without requiring an actual database connection.
"""

import pytest
import sys
from unittest.mock import MagicMock


@pytest.fixture(autouse=True)
def mock_common_modules(monkeypatch):
    """Mock common modules to prevent import issues during testing"""
    # Mock settings before importing api.db.connection
    _mock_settings = MagicMock()
    _mock_settings.DATABASE_TYPE = "postgres"
    _mock_settings.DATABASE = {"name": "test_db"}
    
    # Store original modules if they exist
    original_modules = {}
    modules_to_mock = ["common.settings", "common.config_utils", "common.decorator"]
    
    for module_name in modules_to_mock:
        if module_name in sys.modules:
            original_modules[module_name] = sys.modules[module_name]
    
    # Apply mocks
    monkeypatch.setitem(sys.modules, "common.settings", _mock_settings)
    monkeypatch.setitem(sys.modules, "common.config_utils", MagicMock())
    monkeypatch.setitem(sys.modules, "common.decorator", MagicMock())
    
    yield
    
    # Cleanup is handled automatically by monkeypatch


def test_database_lock_enum():
    """Test DatabaseLock enum exists and is accessible"""
    from api.db.connection import DatabaseLock
    
    assert DatabaseLock is not None
    # DatabaseLock is an Enum, verify it has some values
    assert len(list(DatabaseLock)) > 0


def test_db_singleton():
    """Test DB.connection is accessible"""
    from api.db.connection import DB
    
    conn = DB.connection
    assert conn is not None


def test_retrying_mysql_database_class():
    """Test RetryingPooledMySQLDatabase class exists and has expected base"""
    from api.db.connection import RetryingPooledMySQLDatabase
    from playhouse.pool import PooledMySQLDatabase

    # Verify class inheritance
    assert issubclass(RetryingPooledMySQLDatabase, PooledMySQLDatabase)

    # Verify it has the expected methods
    assert hasattr(RetryingPooledMySQLDatabase, "execute_sql")


def test_retrying_postgres_database_class():
    """Test RetryingPooledPostgresqlDatabase class exists and has expected base"""
    from api.db.connection import RetryingPooledPostgresqlDatabase
    from playhouse.pool import PooledPostgresqlDatabase

    # Verify class inheritance
    assert issubclass(RetryingPooledPostgresqlDatabase, PooledPostgresqlDatabase)

    # Verify it has the expected methods
    assert hasattr(RetryingPooledPostgresqlDatabase, "execute_sql")


def test_connection_imports_from_legacy():
    """Ensure connection utilities can be imported from legacy location"""
    from api.db.connection import DatabaseLock, RetryingPooledMySQLDatabase
    from api.db.db_models import (
        DatabaseLock as LegacyDatabaseLock,
        RetryingPooledMySQLDatabase as LegacyRetryingMySQL,
    )

    # Verify they're the same classes (should be aliases re-exported from legacy location)
    assert LegacyDatabaseLock is DatabaseLock
    assert LegacyRetryingMySQL is RetryingPooledMySQLDatabase

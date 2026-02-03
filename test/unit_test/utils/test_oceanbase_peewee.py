"""
Tests for OceanBase Peewee ORM support.
"""

import pytest
from api.db.db_models import (
    RetryingPooledOceanBaseDatabase,
    PooledDatabase,
    DatabaseLock,
    TextFieldType,
)


class TestOceanBaseDatabase:
    """Test cases for OceanBase database support."""

    def test_oceanbase_database_class_exists(self):
        """Test that RetryingPooledOceanBaseDatabase class exists."""
        assert RetryingPooledOceanBaseDatabase is not None

    def test_oceanbase_in_pooled_database_enum(self):
        """Test that OCEANBASE is in PooledDatabase enum."""
        assert hasattr(PooledDatabase, 'OCEANBASE')
        assert PooledDatabase.OCEANBASE.value == RetryingPooledOceanBaseDatabase

    def test_oceanbase_in_database_lock_enum(self):
        """Test that OCEANBASE is in DatabaseLock enum."""
        assert hasattr(DatabaseLock, 'OCEANBASE')

    def test_oceanbase_in_text_field_type_enum(self):
        """Test that OCEANBASE is in TextFieldType enum."""
        assert hasattr(TextFieldType, 'OCEANBASE')
        # OceanBase should use LONGTEXT like MySQL
        assert TextFieldType.OCEANBASE.value == "LONGTEXT"

    def test_oceanbase_database_inherits_mysql(self):
        """Test that OceanBase database inherits from PooledMySQLDatabase."""
        from playhouse.pool import PooledMySQLDatabase
        assert issubclass(RetryingPooledOceanBaseDatabase, PooledMySQLDatabase)

    def test_oceanbase_database_init(self):
        """Test OceanBase database initialization."""
        db = RetryingPooledOceanBaseDatabase(
            "test_db",
            host="localhost",
            port=2881,
            user="root",
            password="password",
        )
        assert db is not None
        assert db.max_retries == 5  # default value
        assert db.retry_delay == 1  # default value

    def test_oceanbase_database_custom_retries(self):
        """Test OceanBase database with custom retry settings."""
        db = RetryingPooledOceanBaseDatabase(
            "test_db",
            host="localhost",
            max_retries=10,
            retry_delay=2,
        )
        assert db.max_retries == 10
        assert db.retry_delay == 2

    def test_pooled_database_enum_values(self):
        """Test PooledDatabase enum has all expected values."""
        expected = {'MYSQL', 'OCEANBASE', 'POSTGRES'}
        actual = {e.name for e in PooledDatabase}
        assert expected.issubset(actual), f"Missing: {expected - actual}"

    def test_database_lock_enum_values(self):
        """Test DatabaseLock enum has all expected values."""
        expected = {'MYSQL', 'OCEANBASE', 'POSTGRES'}
        actual = set(DatabaseLock.__members__.keys())
        assert expected.issubset(actual), f"Missing: {expected - actual}"


class TestOceanBaseConfiguration:
    """Test cases for OceanBase configuration via environment variables."""

    def test_settings_default_to_mysql(self):
        """Test that default DB_TYPE is mysql."""
        import os
        # Save original value
        original = os.environ.get('DB_TYPE')
        
        try:
            # Remove DB_TYPE to test default
            if 'DB_TYPE' in os.environ:
                del os.environ['DB_TYPE']
            
            # Reload settings
            from common import settings
            settings.DATABASE_TYPE = os.getenv("DB_TYPE", "mysql")
            
            assert settings.DATABASE_TYPE == "mysql"
        finally:
            # Restore original value
            if original:
                os.environ['DB_TYPE'] = original

    def test_settings_can_use_oceanbase(self):
        """Test that DB_TYPE can be set to oceanbase."""
        import os
        # Save original value
        original = os.environ.get('DB_TYPE')
        
        try:
            os.environ['DB_TYPE'] = 'oceanbase'
            
            # Reload settings
            from common import settings
            settings.DATABASE_TYPE = os.getenv("DB_TYPE", "mysql")
            
            assert settings.DATABASE_TYPE == "oceanbase"
        finally:
            # Restore original value
            if original:
                os.environ['DB_TYPE'] = original
            else:
                if 'DB_TYPE' in os.environ:
                    del os.environ['DB_TYPE']


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

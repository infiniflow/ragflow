"""
Test suite for database migration tracking system (Phase 3) and transaction safety (Phase 5)

This module tests the MigrationHistory model, MigrationTracker utility class,
and TransactionLogger, ensuring migrations are properly tracked, duplicate executions
are prevented, and transactions are atomic with proper rollback capability.
"""

from __future__ import annotations

import os
import sys
import unittest
from datetime import datetime
from unittest.mock import MagicMock, patch, Mock

import pytest
import peewee

# Check if we're running in Docker (with real database) or locally (need mocks)
IN_DOCKER = os.path.exists('/ragflow/.venv') or os.environ.get('DOCKER_CONTAINER') == 'true'

if not IN_DOCKER:
    # Only mock settings when running locally without real database
    mock_settings = MagicMock()
    mock_settings.DATABASE_TYPE = "postgres"
    mock_settings.DATABASE = {"name": "test_db", "user": "test", "password": "test", "host": "test", "port": 5432}
    sys.modules["common.settings"] = mock_settings
    sys.modules["common.config_utils"] = MagicMock()
    sys.modules["common.decorator"] = MagicMock()

    # Mock the singleton decorator
    def mock_singleton(cls):
        return cls

    sys.modules["common.decorator"].singleton = mock_singleton

# Now we can import after conditional mocking
from api.db.migrations import (  # noqa: E402
    MigrationHistory,
    MigrationTracker,
    migrate_db,
)
from api.db.connection import TransactionLogger, DB  # noqa: E402


def setup_test_database():
    """Set up test database - use in-memory SQLite for tests without external dependencies"""
    if IN_DOCKER and DB and hasattr(DB, 'database'):
        # Use real PostgreSQL database in Docker
        MigrationTracker.init_tracking_table()
        return DB
    else:
        # Use in-memory SQLite for local testing
        from playhouse.sqlite_ext import SqliteExtDatabase
        test_db = SqliteExtDatabase(':memory:')
        
        # Bind MigrationHistory to test database
        test_db.bind([MigrationHistory])
        test_db.connect()
        test_db.create_tables([MigrationHistory])
        
        # Patch the global DB object
        import api.db.connection
        api.db.connection.DB = test_db
        
        return test_db


def teardown_test_database():
    """Clean up test database - just clear data, don't drop tables"""
    try:
        # Clear test data but keep the table structure
        if MigrationHistory.table_exists():
            MigrationHistory.delete().execute()
    except Exception:
        pass  # Table might not exist in some test scenarios


class TestMigrationHistoryModel(unittest.TestCase):
    """Test the MigrationHistory database model"""

    @classmethod
    def setUpClass(cls):
        """Set up test database"""
        cls.test_db = setup_test_database()

    @classmethod
    def tearDownClass(cls):
        """Clean up test database"""
        teardown_test_database()
        if not IN_DOCKER and cls.test_db:
            cls.test_db.close()

    def setUp(self):
        """Clean up before each test"""
        # Clear migration history before each test
        if MigrationHistory.table_exists():
            MigrationHistory.delete().execute()

    def test_migration_history_table_creation(self):
        """Verify migration_history table can be created"""
        # Table should exist after setup
        self.assertTrue(MigrationHistory.table_exists())

    def test_migration_history_fields(self):
        """Verify MigrationHistory has all required fields"""
        fields = MigrationHistory._meta.fields

        self.assertIn("id", fields)
        self.assertIn("migration_name", fields)
        self.assertIn("applied_at", fields)
        self.assertIn("status", fields)
        self.assertIn("error_message", fields)
        self.assertIn("duration_ms", fields)
        self.assertIn("db_type", fields)

    def test_migration_history_insert(self):
        """Test inserting a migration record"""
        # Insert a test record
        MigrationHistory.insert(migration_name="test_migration_1", applied_at=datetime.now(), status="success", duration_ms=150, db_type="postgres").execute()

        # Verify it was inserted
        result = MigrationHistory.select().where(MigrationHistory.migration_name == "test_migration_1").first()

        self.assertIsNotNone(result)
        self.assertEqual(result.migration_name, "test_migration_1")
        self.assertEqual(result.status, "success")
        self.assertEqual(result.duration_ms, 150)


class TestMigrationTracker(unittest.TestCase):
    """Test the MigrationTracker utility class"""

    @classmethod
    def setUpClass(cls):
        """Set up test database"""
        cls.test_db = setup_test_database()

    @classmethod
    def tearDownClass(cls):
        """Clean up test database"""
        teardown_test_database()
        if not IN_DOCKER and cls.test_db:
            cls.test_db.close()

    def setUp(self):
        """Clean up before each test"""
        if MigrationHistory.table_exists():
            MigrationHistory.delete().execute()

    def test_init_tracking_table(self):
        """Test tracking table initialization"""
        # Should not raise an exception
        MigrationTracker.init_tracking_table()

        # Table should exist
        self.assertTrue(MigrationHistory.table_exists())

    def test_has_migration_run_false(self):
        """Test has_migration_run returns False for new migration"""
        result = MigrationTracker.has_migration_run("new_migration")
        self.assertFalse(result)

    def test_has_migration_run_true(self):
        """Test has_migration_run returns True for applied migration"""
        # Record a successful migration
        MigrationTracker.record_migration("test_migration", "success")

        # Check if it's recorded
        result = MigrationTracker.has_migration_run("test_migration")
        self.assertTrue(result)

    def test_has_migration_run_failed_migration(self):
        """Test has_migration_run returns False for failed migration"""
        # Record a failed migration
        MigrationTracker.record_migration("failed_migration", "failed", error="Test error")

        # Should return False (failed migrations don't count as applied)
        result = MigrationTracker.has_migration_run("failed_migration")
        self.assertFalse(result)

    def test_record_migration_success(self):
        """Test recording a successful migration"""
        MigrationTracker.record_migration(migration_name="test_record_success", status="success", duration_ms=200)

        # Verify it was recorded
        result = MigrationHistory.select().where(MigrationHistory.migration_name == "test_record_success").first()

        self.assertIsNotNone(result)
        self.assertEqual(result.status, "success")
        self.assertEqual(result.duration_ms, 200)

    def test_record_migration_failed(self):
        """Test recording a failed migration"""
        error_msg = "Test error message"
        MigrationTracker.record_migration(migration_name="test_record_failed", status="failed", error=error_msg, duration_ms=50)

        # Verify it was recorded with error
        result = MigrationHistory.select().where(MigrationHistory.migration_name == "test_record_failed").first()

        self.assertIsNotNone(result)
        self.assertEqual(result.status, "failed")
        self.assertEqual(result.error_message, error_msg)

    def test_get_migration_history(self):
        """Test retrieving migration history"""
        # Insert multiple migrations
        for i in range(5):
            MigrationTracker.record_migration(f"migration_{i}", "success")

        # Retrieve history
        history = MigrationTracker.get_migration_history()

        self.assertGreaterEqual(len(history), 5)
        # Should be ordered by applied_at descending (most recent first)
        self.assertTrue(all(isinstance(h, MigrationHistory) for h in history))

    def test_get_failed_migrations(self):
        """Test retrieving only failed migrations"""
        # Insert mixed migrations
        MigrationTracker.record_migration("success_1", "success")
        MigrationTracker.record_migration("failed_1", "failed", error="Error 1")
        MigrationTracker.record_migration("success_2", "success")
        MigrationTracker.record_migration("failed_2", "failed", error="Error 2")

        # Retrieve only failed
        failed = MigrationTracker.get_failed_migrations()

        self.assertEqual(len(failed), 2)
        self.assertTrue(all(m.status == "failed" for m in failed))
        self.assertTrue(any(m.migration_name == "failed_1" for m in failed))
        self.assertTrue(any(m.migration_name == "failed_2" for m in failed))


class TestMigrationIntegration(unittest.TestCase):
    """Integration tests for the migration system"""

    @classmethod
    def setUpClass(cls):
        """Set up test database"""
        cls.test_db = setup_test_database()

    @classmethod
    def tearDownClass(cls):
        """Clean up test database"""
        teardown_test_database()
        if not IN_DOCKER and cls.test_db:
            cls.test_db.close()

    def setUp(self):
        """Clean up before each test"""
        if MigrationHistory.table_exists():
            MigrationHistory.delete().execute()

    @patch("api.db.migrations.DatabaseMigrator")
    @patch("api.db.migrations.alter_db_add_column")
    def test_migrate_db_prevents_duplicate_runs(self, mock_add_column, mock_migrator):
        """Test that migrate_db doesn't run migrations twice"""
        # Mock the migrator
        mock_migrator_instance = MagicMock()
        mock_migrator.__getitem__.return_value.value.return_value = mock_migrator_instance

        # First run - migrations should be executed
        migrate_db()

        # Record call count after first run
        first_run_call_count = mock_add_column.call_count

        # Second run - migrations should be skipped
        migrate_db()

        # Record call count after second run
        second_run_call_count = mock_add_column.call_count

        # Second run should not increase call count (no new migrations executed)
        self.assertEqual(first_run_call_count, second_run_call_count, 
                        "Call count should not increase on second run since migrations are idempotent")

    def test_migration_timing(self):
        """Test that migration duration is recorded"""
        # Record a migration with duration
        MigrationTracker.record_migration("timed_migration", "success", duration_ms=150)

        # Retrieve and verify
        result = MigrationHistory.select().where(MigrationHistory.migration_name == "timed_migration").first()

        self.assertIsNotNone(result.duration_ms)
        self.assertEqual(result.duration_ms, 150)

    def test_migration_db_type_tracking(self):
        """Test that database type is tracked"""
        # Get expected DB type dynamically
        from common import settings
        expected_db_type = settings.DATABASE_TYPE.lower() if hasattr(settings, 'DATABASE_TYPE') else "postgres"
        
        MigrationTracker.record_migration("db_type_test", "success")

        result = MigrationHistory.select().where(MigrationHistory.migration_name == "db_type_test").first()

        # Should match current database type
        self.assertEqual(result.db_type, expected_db_type)


class TestMigrationErrorHandling(unittest.TestCase):
    """Test error handling in migration tracking"""

    @classmethod
    def setUpClass(cls):
        """Set up test database"""
        cls.test_db = setup_test_database()

    @classmethod
    def tearDownClass(cls):
        """Clean up test database"""
        teardown_test_database()
        if not IN_DOCKER and cls.test_db:
            cls.test_db.close()

    def setUp(self):
        """Clean up before each test"""
        if MigrationHistory.table_exists():
            MigrationHistory.delete().execute()

    def test_migration_continues_on_tracking_failure(self):
        """Test that migrations continue even if tracking fails"""
        # This test verifies fail-safe behavior
        with patch.object(MigrationHistory, "insert") as mock_insert:
            # Make insert fail
            mock_insert.side_effect = Exception("Database error")

            # Should not raise exception
            try:
                MigrationTracker.record_migration("test", "success")
                # If we get here, the error was handled gracefully
                success = True
            except Exception:
                success = False

            self.assertTrue(success, "Migration tracking should handle errors gracefully")

    def test_has_migration_run_handles_missing_table(self):
        """Test that has_migration_run handles missing table gracefully"""
        # Mock the database layer to simulate missing table error
        with patch.object(MigrationHistory, 'table_exists', return_value=False):
            with patch.object(MigrationHistory, 'select') as mock_select:
                # Simulate the same error as a missing table
                mock_select.side_effect = Exception("Table 'migration_history' doesn't exist")
                
                # Should not raise exception, should return False
                result = MigrationTracker.has_migration_run("test_migration")
                self.assertFalse(result)


@pytest.mark.p1
class TestMigrationTrackerP1:
    """Priority 1 tests for migration tracking (pytest style)"""

    def test_migration_idempotency(self):
        """Test that running migrations multiple times is safe"""
        MigrationTracker.init_tracking_table()

        # Record same migration multiple times
        migration_name = "idempotent_test"

        # First time
        MigrationTracker.record_migration(migration_name, "success")
        assert MigrationTracker.has_migration_run(migration_name) is True

        # Count records
        count = MigrationHistory.select().where(MigrationHistory.migration_name == migration_name).count()

        # Should have at least one record
        # (Multiple records allowed as per current implementation)
        assert count >= 1

    def test_migration_status_values(self):
        """Test that valid status values are accepted"""
        MigrationTracker.init_tracking_table()

        valid_statuses = ["success", "failed", "skipped"]

        for status in valid_statuses:
            migration_name = f"status_test_{status}"
            MigrationTracker.record_migration(migration_name, status)

            result = MigrationHistory.select().where(MigrationHistory.migration_name == migration_name).first()

            assert result.status == status


class TestTransactionLogger(unittest.TestCase):
    """Test the TransactionLogger utility class (Phase 5)"""

    def setUp(self):
        """Set up mocks for testing"""
        self.mock_db = MagicMock()
        # Simulate PostgreSQL database
        from api.db.connection import PooledPostgresqlDatabase

        self.mock_db.__class__ = PooledPostgresqlDatabase

    def test_log_transaction_begin(self):
        """Test logging transaction begin"""
        with patch("logging.debug") as mock_debug:
            TransactionLogger.log_transaction_state(self.mock_db, "begin", "test operation")

            # Verify debug log was called
            mock_debug.assert_called_once()
            call_args = mock_debug.call_args[0][0]
            self.assertIn("transaction started", call_args.lower())
            self.assertIn("test operation", call_args)

    def test_log_transaction_commit(self):
        """Test logging transaction commit"""
        with patch("logging.debug") as mock_debug:
            TransactionLogger.log_transaction_state(self.mock_db, "commit", "5 migrations applied")

            # Verify debug log was called
            mock_debug.assert_called_once()
            call_args = mock_debug.call_args[0][0]
            self.assertIn("transaction committed", call_args.lower())
            self.assertIn("5 migrations applied", call_args)

    def test_log_transaction_rollback(self):
        """Test logging transaction rollback"""
        with patch("logging.warning") as mock_warning:
            TransactionLogger.log_transaction_state(self.mock_db, "rollback", "migration failed")

            # Verify warning log was called (rollbacks are warnings)
            mock_warning.assert_called_once()
            call_args = mock_warning.call_args[0][0]
            self.assertIn("transaction rolled back", call_args.lower())
            self.assertIn("migration failed", call_args)

    def test_log_transaction_error(self):
        """Test logging transaction errors"""
        test_exception = Exception("Test database error")

        with patch("logging.error") as mock_error:
            TransactionLogger.log_transaction_error(self.mock_db, test_exception, "migration 'test_migration'")

            # Verify error log was called
            mock_error.assert_called_once()
            call_args = mock_error.call_args[0][0]
            self.assertIn("transaction error", call_args.lower())
            self.assertIn("test_migration", call_args)
            self.assertIn("test database error", call_args.lower())

    def test_log_transaction_mysql_detection(self):
        """Test that MySQL database is correctly identified in logs"""
        from api.db.connection import PooledMySQLDatabase

        mysql_db = MagicMock()
        mysql_db.__class__ = PooledMySQLDatabase

        with patch("logging.debug") as mock_debug:
            TransactionLogger.log_transaction_state(mysql_db, "begin")

            call_args = mock_debug.call_args[0][0]
            self.assertIn("mysql", call_args.lower())


class TestTransactionSafety(unittest.TestCase):
    """Test atomic transaction behavior in migrations (Phase 5)"""

    @classmethod
    def setUpClass(cls):
        """Set up test database"""
        cls.test_db = setup_test_database()

    @classmethod
    def tearDownClass(cls):
        """Clean up test database"""
        teardown_test_database()
        if not IN_DOCKER and cls.test_db:
            cls.test_db.close()

    def setUp(self):
        """Clean up before each test"""
        if MigrationHistory.table_exists():
            MigrationHistory.delete().execute()

    def setUp(self):
        """Clean up before each test"""
        try:
            MigrationHistory.delete().execute()
        except Exception:
            pass

    @patch("api.db.migrations.DB")
    @patch("api.db.migrations.DatabaseMigrator")
    def test_migrate_db_uses_atomic_transaction(self, mock_migrator, mock_db):
        """Test that migrate_db wraps migrations in atomic transaction"""
        # Setup mocks
        mock_migrator_instance = MagicMock()
        mock_migrator.__getitem__.return_value.value.return_value = mock_migrator_instance

        # Mock atomic context manager
        mock_atomic = MagicMock()
        mock_db.atomic.return_value = mock_atomic
        mock_atomic.__enter__ = MagicMock(return_value=mock_atomic)
        mock_atomic.__exit__ = MagicMock(return_value=False)

        # Mock all migration functions to succeed
        with patch("api.db.migrations.alter_db_add_column"):
            with patch("api.db.migrations.alter_db_column_type"):
                with patch("api.db.migrations.alter_db_rename_column"):
                    migrate_db()

        # Verify atomic was called to start transaction
        mock_db.atomic.assert_called()

    @patch("api.db.migrations.DB")
    @patch("api.db.migrations.DatabaseMigrator")
    @patch("api.db.connection.TransactionLogger")
    def test_transaction_rollback_on_migration_failure(self, mock_logger, mock_migrator, mock_db):
        """Test that transaction rolls back when a migration fails"""
        # Setup mocks
        mock_migrator_instance = MagicMock()
        mock_migrator.__getitem__.return_value.value.return_value = mock_migrator_instance

        # Mock atomic context manager
        mock_atomic = MagicMock()
        mock_db.atomic.return_value = mock_atomic
        mock_atomic.__enter__ = MagicMock(return_value=mock_atomic)
        mock_atomic.__exit__ = MagicMock(return_value=False)

        # Make one migration fail
        with patch("api.db.migrations.alter_db_add_column") as mock_add_column:
            mock_add_column.side_effect = [None, None, Exception("Migration failed")]

            # Should raise exception and rollback
            with self.assertRaises(Exception):
                migrate_db()

        # Verify rollback was logged
        mock_logger.log_transaction_state.assert_any_call(mock_db, "rollback", unittest.mock.ANY)

    @patch("api.db.migrations.DB")
    @patch("api.db.migrations.DatabaseMigrator")
    @patch("api.db.connection.TransactionLogger")
    def test_transaction_commit_on_success(self, mock_logger, mock_migrator, mock_db):
        """Test that transaction commits when all migrations succeed"""
        # Setup mocks
        mock_migrator_instance = MagicMock()
        mock_migrator.__getitem__.return_value.value.return_value = mock_migrator_instance

        # Mock atomic context manager
        mock_atomic = MagicMock()
        mock_db.atomic.return_value = mock_atomic
        mock_atomic.__enter__ = MagicMock(return_value=mock_atomic)
        mock_atomic.__exit__ = MagicMock(return_value=False)

        # All migrations succeed
        with patch("api.db.migrations.alter_db_add_column"):
            with patch("api.db.migrations.alter_db_column_type"):
                with patch("api.db.migrations.alter_db_rename_column"):
                    migrate_db()

        # Verify begin and commit were logged
        begin_calls = [call for call in mock_logger.log_transaction_state.call_args_list if "begin" in str(call)]
        commit_calls = [call for call in mock_logger.log_transaction_state.call_args_list if "commit" in str(call)]

        # Should have at least one begin call
        self.assertGreater(len(begin_calls), 0, "Transaction begin should be logged")
        # Should have at least one commit call
        self.assertGreater(len(commit_calls), 0, "Transaction commit should be logged")
        # Commit calls should match or be close to begin calls
        self.assertGreaterEqual(len(commit_calls), len(begin_calls) - 1, "Commit count should be at least as many as begin (minus potential rollbacks)")

    def test_record_failed_migration(self):
        """Test that MigrationTracker.record_migration correctly records a failed migration status.
        
        Note: This test verifies record_migration behavior only. It does not exercise
        the transactional rollback path. For full rollback testing, see integration tests.
        """
        MigrationTracker.init_tracking_table()

        migration_name = "test_failing_migration"

        # Record a failed migration status
        MigrationTracker.record_migration(migration_name, "failed", error="Test error", duration_ms=100)

        # Verify it was recorded correctly
        result = MigrationHistory.select().where(MigrationHistory.migration_name == migration_name).first()

        self.assertIsNotNone(result)
        self.assertEqual(result.status, "failed")
        self.assertEqual(result.error_message, "Test error")


@pytest.mark.p1
class TestTransactionSafetyP1:
    """Priority 1 tests for transaction safety (pytest style)"""

    @classmethod
    def setup_class(cls):
        """Set up test database for pytest class"""
        cls.test_db = setup_test_database()

    @classmethod
    def teardown_class(cls):
        """Clean up test database for pytest class"""
        teardown_test_database()
        if not IN_DOCKER and cls.test_db:
            cls.test_db.close()

    def setup_method(self):
        """Clean up before each test"""
        if MigrationHistory.table_exists():
            MigrationHistory.delete().execute()

    def test_atomic_all_or_nothing(self):
        """Test that migrations are truly atomic (all-or-nothing)"""
        import pytest
        
        # Create a test migration that fails partway through
        def failing_migration(migrator):
            # This should succeed
            MigrationHistory.create(migration_name="test_atomic_fail", status="failed")
            # This should cause rollback
            raise Exception("Simulated migration failure")
        
        # Verify no migration record exists before
        initial_count = MigrationHistory.select().where(MigrationHistory.migration_name == "test_atomic_fail").count()
        
        # Run migration in atomic context and expect failure
        with pytest.raises(Exception):
            with DB.atomic():
                failing_migration(None)
        
        # Verify rollback - no migration record should exist
        final_count = MigrationHistory.select().where(MigrationHistory.migration_name == "test_atomic_fail").count()
        assert initial_count == final_count, "Migration should have been rolled back completely"

    def test_transaction_logger_handles_both_databases(self):
        """Test that TransactionLogger works with both MySQL and PostgreSQL"""
        from api.db.connection import PooledMySQLDatabase, PooledPostgresqlDatabase

        # Test with PostgreSQL - use spec_set to ensure isinstance() works
        pg_db = MagicMock(spec_set=PooledPostgresqlDatabase)
        pg_db.__class__ = PooledPostgresqlDatabase

        with patch("logging.debug") as mock_debug:
            TransactionLogger.log_transaction_state(pg_db, "begin")
            call_args = mock_debug.call_args[0][0]
            assert "postgresql" in call_args.lower()

        # Test with MySQL - use spec_set to ensure isinstance() works
        mysql_db = MagicMock(spec_set=PooledMySQLDatabase)
        mysql_db.__class__ = PooledMySQLDatabase

        with patch("logging.debug") as mock_debug:
            TransactionLogger.log_transaction_state(mysql_db, "begin")
            call_args = mock_debug.call_args[0][0]
            assert "mysql" in call_args.lower()


if __name__ == "__main__":
    # Run tests
    unittest.main()

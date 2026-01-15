"""
Unit tests for StandardErrorHandler in api.db.error_handlers

Tests error categorization and handling for both MySQL and PostgreSQL databases.
"""

import sys
import unittest
from unittest.mock import patch, MagicMock


# Mock database connections and configuration before importing api.db.migrations
with patch.dict(sys.modules, {
    "common.settings": MagicMock(),
    "common.config_utils": MagicMock(),
    "common.decorator": MagicMock(),
}):
    from api.db.error_handlers import StandardErrorHandler


class MockPostgresError(Exception):
    """Mock PostgreSQL exception with pgcode attribute."""
    def __init__(self, message, pgcode):
        super().__init__(message)
        self.pgcode = pgcode
        self.args = (message,)


class MockMySQLError(Exception):
    """Mock MySQL exception with error code in args[0]."""
    def __init__(self, error_code, message):
        super().__init__(message)
        self.args = (error_code, message)


class TestStandardErrorHandlerCategorization(unittest.TestCase):
    """Test error categorization logic."""

    def test_postgres_duplicate_column_by_pgcode(self):
        """Test PostgreSQL duplicate column detection via pgcode."""
        exc = MockPostgresError("column already exists", "42701")
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(
            exc, db_type="postgres"
        )

        self.assertEqual(category, "duplicate_column")
        self.assertTrue(is_expected)
        self.assertTrue(should_skip)

    def test_mysql_duplicate_column_by_code(self):
        """Test MySQL duplicate column detection via error code 1060."""
        exc = MockMySQLError(1060, "Duplicate column name")
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(
            exc, db_type="mysql"
        )

        self.assertEqual(category, "duplicate_column")
        self.assertTrue(is_expected)
        self.assertTrue(should_skip)

    def test_postgres_duplicate_entry_by_pgcode(self):
        """Test PostgreSQL duplicate table detection via pgcode 42P07."""
        exc = MockPostgresError("relation already exists", "42P07")
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(
            exc, db_type="postgres"
        )

        self.assertEqual(category, "duplicate_entry")
        self.assertTrue(is_expected)
        self.assertFalse(should_skip)

    def test_mysql_duplicate_entry_by_code(self):
        """Test MySQL duplicate entry detection via error code 1062."""
        exc = MockMySQLError(1062, "Duplicate entry for key 'email'")
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(
            exc, db_type="mysql"
        )

        self.assertEqual(category, "duplicate_entry")
        self.assertTrue(is_expected)
        self.assertFalse(should_skip)

    def test_postgres_incompatible_type_by_pgcode(self):
        """Test PostgreSQL type mismatch detection via pgcode."""
        exc = MockPostgresError("cannot be cast to", "42804")
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(
            exc, db_type="postgres"
        )

        self.assertEqual(category, "incompatible_type")
        self.assertTrue(is_expected)
        self.assertFalse(should_skip)

    def test_mysql_incompatible_type_by_code(self):
        """Test MySQL type mismatch detection via error code."""
        exc = MockMySQLError(1064, "Syntax error in SQL")
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(
            exc, db_type="mysql"
        )

        self.assertEqual(category, "incompatible_type")
        self.assertTrue(is_expected)
        self.assertFalse(should_skip)

    def test_duplicate_column_by_message_pattern(self):
        """Test duplicate column detection via error message."""
        exc = Exception("duplicate column 'user_id' in table")
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(exc)

        self.assertEqual(category, "duplicate_column")
        self.assertTrue(is_expected)
        self.assertTrue(should_skip)

    def test_missing_column_by_pgcode(self):
        """Test PostgreSQL missing column detection."""
        exc = MockPostgresError("column does not exist", "42703")
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(
            exc, db_type="postgres"
        )

        self.assertEqual(category, "missing_column")
        self.assertTrue(is_expected)
        self.assertFalse(should_skip)

    def test_missing_column_by_mysql_code(self):
        """Test MySQL missing column detection via error code 1054."""
        exc = MockMySQLError(1054, "Unknown column 'name' in table")
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(
            exc, db_type="mysql"
        )

        self.assertEqual(category, "missing_column")
        self.assertTrue(is_expected)
        self.assertFalse(should_skip)

    def test_missing_column_by_message_pattern(self):
        """Test missing column detection via error message."""
        exc = Exception("no such column: user_id")
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(exc)

        self.assertEqual(category, "missing_column")
        self.assertTrue(is_expected)
        self.assertFalse(should_skip)

    def test_unknown_error(self):
        """Test unknown error categorization."""
        exc = Exception("Something went wrong but it's not a known error")
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(exc)

        self.assertEqual(category, "unknown")
        self.assertFalse(is_expected)
        self.assertFalse(should_skip)

    def test_case_insensitive_message_matching(self):
        """Test that error message matching is case-insensitive."""
        exc = Exception("DUPLICATE COLUMN 'user_id' IN table")
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(exc)

        self.assertEqual(category, "duplicate_column")
        self.assertTrue(is_expected)
        self.assertTrue(should_skip)

    def test_db_type_case_insensitive(self):
        """Test that db_type parameter is case-insensitive."""
        exc = MockPostgresError("column already exists", "42701")

        for db_type in ["postgres", "POSTGRES", "PostgreSQL", "pOsTgReS"]:
            category, is_expected, should_skip = StandardErrorHandler.categorize_error(
                exc, db_type=db_type
            )
            self.assertEqual(category, "duplicate_column")


class TestStandardErrorHandlerMigrationErrorHandling(unittest.TestCase):
    """Test error handling for migration operations."""

    def test_handle_duplicate_column_logs_debug_and_does_not_skip(self):
        """Test that duplicate column errors log debug but return a falsy skip indicator."""
        exc = MockPostgresError("column already exists", "42701")

        # Patch the module-level logger used by error_handlers, not the global logging module
        with patch('api.db.error_handlers.logging.debug') as mock_debug:
            should_skip = StandardErrorHandler.handle_migration_error(
                exc, "users", "email", "add_column", db_type="postgres", db_name="POSTGRES"
            )

            self.assertFalse(should_skip)
            # Verify debug log was called via the module's logging reference
            mock_debug.assert_called_once()
            call_args = str(mock_debug.call_args)
            self.assertIn("skipped", call_args)

    def test_handle_incompatible_type_logs_warning(self):
        """Test that type incompatibility is logged at warning level."""
        exc = MockPostgresError("cannot be cast to integer", "42804")

        with patch('api.db.error_handlers.logging.warning') as mock_warning:
            should_skip = StandardErrorHandler.handle_migration_error(
                exc, "users", "age", "alter_column_type", db_type="postgres", db_name="POSTGRES"
            )

            self.assertFalse(should_skip)
            # Verify warning log was called
            mock_warning.assert_called_once()
            call_args = str(mock_warning.call_args)
            self.assertIn("expected issue", call_args)

    def test_handle_unknown_error_logs_critical(self):
        """Test that unknown errors are logged at critical level."""
        exc = Exception("Unknown database error occurred")

        with patch('api.db.error_handlers.logging.critical') as mock_critical:
            should_skip = StandardErrorHandler.handle_migration_error(
                exc, "users", "profile", "add_column", db_type="postgres", db_name="POSTGRES"
            )

            self.assertFalse(should_skip)
            # Verify critical log was called
            mock_critical.assert_called_once()
            call_args = str(mock_critical.call_args)
            self.assertIn("unexpected", call_args)

    def test_handle_missing_column_with_mysql(self):
        """Test handling of missing column error for MySQL."""
        exc = MockMySQLError(1054, "Unknown column 'user_id' in 'where clause'")

        with patch('api.db.error_handlers.logging.warning') as mock_warning:
            should_skip = StandardErrorHandler.handle_migration_error(
                exc, "profiles", "user_id", "rename_column", db_type="mysql", db_name="MYSQL"
            )

            self.assertFalse(should_skip)
            # Verify warning log was called
            mock_warning.assert_called_once()
            call_args = str(mock_warning.call_args)
            self.assertIn("expected issue", call_args)


class TestStandardErrorHandlerErrorCodes(unittest.TestCase):
    """Test error code configurations."""

    def test_error_codes_structure(self):
        """Test that ERROR_CODES has expected structure."""
        error_codes = StandardErrorHandler.ERROR_CODES

        # Check that all categories exist
        expected_categories = {"duplicate_column", "duplicate_entry", "incompatible_type", "missing_column"}
        self.assertEqual(set(error_codes.keys()), expected_categories)

        # Check that each category has both mysql and postgres codes
        for category, codes in error_codes.items():
            self.assertIn("mysql", codes)
            self.assertIn("postgres", codes)
            self.assertIsInstance(codes["mysql"], list)
            self.assertIsInstance(codes["postgres"], list)
            self.assertTrue(len(codes["mysql"]) > 0)
            self.assertTrue(len(codes["postgres"]) > 0)

    def test_error_messages_structure(self):
        """Test that ERROR_MESSAGES has expected structure."""
        error_messages = StandardErrorHandler.ERROR_MESSAGES

        # Check that all categories exist
        expected_categories = {"duplicate_column", "duplicate_entry", "incompatible_type", "missing_column"}
        self.assertEqual(set(error_messages.keys()), expected_categories)

        # Check that each category has message patterns
        for category, patterns in error_messages.items():
            self.assertIsInstance(patterns, list)
            self.assertTrue(len(patterns) > 0)
            for pattern in patterns:
                self.assertIsInstance(pattern, str)


class TestStandardErrorHandlerEdgeCases(unittest.TestCase):
    """Test edge cases and unusual inputs."""

    def test_exception_without_args(self):
        """Test handling of exception without args attribute."""
        exc = Exception()
        # Don't set args to None - just use the default empty tuple

        category, is_expected, should_skip = StandardErrorHandler.categorize_error(exc)
        self.assertEqual(category, "unknown")
        self.assertFalse(is_expected)

    def test_exception_with_non_integer_error_code(self):
        """Test handling of exception with non-integer error code."""
        exc = Exception("some error")
        exc.args = ("string error code", "message")

        category, is_expected, should_skip = StandardErrorHandler.categorize_error(
            exc, db_type="mysql"
        )
        # Should not crash and should check message patterns
        self.assertEqual(category, "unknown")

    def test_postgres_error_with_empty_string_pgcode(self):
        """Test PostgreSQL error with empty string pgcode."""
        exc = MockPostgresError("some error", "")

        category, is_expected, should_skip = StandardErrorHandler.categorize_error(
            exc, db_type="postgres"
        )
        self.assertEqual(category, "unknown")

    def test_long_error_message(self):
        """Test handling of very long error messages."""
        long_msg = "duplicate column " + "x" * 1000
        exc = Exception(long_msg)

        category, is_expected, should_skip = StandardErrorHandler.categorize_error(exc)
        self.assertEqual(category, "duplicate_column")
        self.assertTrue(is_expected)

    def test_multiple_error_patterns_in_message(self):
        """Test message containing multiple error patterns - should match first."""
        # Message contains both "duplicate" and "cannot be cast"
        exc = Exception("duplicate column 'id' cannot be cast to integer")

        category, is_expected, should_skip = StandardErrorHandler.categorize_error(exc)
        # Should match the first pattern found (duplicate_column is checked first)
        self.assertEqual(category, "duplicate_column", "Should match the first pattern encountered: duplicate_column")


if __name__ == "__main__":
    unittest.main()

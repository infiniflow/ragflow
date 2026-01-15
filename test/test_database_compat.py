"""
Test suite for database compatibility layer (Phase 4)

This module tests the DatabaseCompat class which provides:
- Database capability matrix and checks
- Field type compatibility validation
- Database-specific decorators for migrations
- Type equivalence mapping between MySQL and PostgreSQL
"""

from __future__ import annotations

import os
import sys
import unittest
from unittest.mock import MagicMock, patch

# Check if we're running in Docker (with real database) or locally (need mocks)
IN_DOCKER = os.path.exists('/ragflow/.venv') or os.environ.get('DOCKER_CONTAINER') == 'true'

if not IN_DOCKER:
    # Only mock settings when running locally without real database
    mock_settings = MagicMock()
    mock_settings.DATABASE_TYPE = "postgres"
    sys.modules["common.settings"] = mock_settings
    sys.modules["common.config_utils"] = MagicMock()
    sys.modules["common.time_utils"] = MagicMock()
    sys.modules["api.db.connection"] = MagicMock()

from peewee import CharField  # noqa: E402

# Now we can import after mocking dependencies
from api.db.migrations import DatabaseCompat  # noqa: E402


# Create mock field classes for testing
class JSONField:
    def __init__(self):
        self.null = True
        self.index = False
        self.unique = False
        self.default = {}


class LongTextField:
    def __init__(self):
        self.null = True
        self.index = False
        self.unique = False
        self.default = None


class DateTimeTzField:
    def __init__(self):
        self.null = True
        self.index = False
        self.unique = False
        self.default = None


class TestDatabaseCapabilities(unittest.TestCase):
    """Test database capability matrix and checks"""

    def test_mysql_capabilities(self):
        """Verify MySQL capabilities are correctly defined"""
        caps = DatabaseCompat.get_capabilities("mysql")

        self.assertIsNotNone(caps)
        self.assertTrue(caps["full_text_search"])
        self.assertTrue(caps["json_functions"])
        self.assertTrue(caps["auto_increment"])
        self.assertFalse(caps["sequence_support"])
        self.assertEqual(caps["type_casting"], "limited")
        self.assertEqual(caps["max_varchar"], 65535)
        self.assertFalse(caps["jsonb_support"])
        self.assertFalse(caps["transaction_ddl"])

    def test_postgres_capabilities(self):
        """Verify PostgreSQL capabilities are correctly defined"""
        caps = DatabaseCompat.get_capabilities("postgres")

        self.assertIsNotNone(caps)
        self.assertTrue(caps["full_text_search"])
        self.assertTrue(caps["json_functions"])
        self.assertFalse(caps["auto_increment"])
        self.assertTrue(caps["sequence_support"])
        self.assertEqual(caps["type_casting"], "full")
        self.assertIsNone(caps["max_varchar"])  # No limit
        self.assertTrue(caps["jsonb_support"])
        self.assertTrue(caps["transaction_ddl"])

    def test_is_capable_postgres(self):
        """Test capability check for PostgreSQL"""
        self.assertTrue(DatabaseCompat.is_capable("postgres", "jsonb_support"))
        self.assertTrue(DatabaseCompat.is_capable("postgres", "sequence_support"))
        self.assertFalse(DatabaseCompat.is_capable("postgres", "auto_increment"))

    def test_is_capable_mysql(self):
        """Test capability check for MySQL"""
        self.assertTrue(DatabaseCompat.is_capable("mysql", "auto_increment"))
        self.assertFalse(DatabaseCompat.is_capable("mysql", "jsonb_support"))
        self.assertFalse(DatabaseCompat.is_capable("mysql", "sequence_support"))

    def test_is_capable_case_insensitive(self):
        """Verify is_capable handles case-insensitive database names"""
        self.assertTrue(DatabaseCompat.is_capable("MYSQL", "auto_increment"))
        self.assertTrue(DatabaseCompat.is_capable("MySQL", "auto_increment"))
        self.assertTrue(DatabaseCompat.is_capable("POSTGRES", "jsonb_support"))

    def test_is_capable_unknown_database(self):
        """Test is_capable with unknown database type"""
        result = DatabaseCompat.is_capable("oracle", "auto_increment")
        self.assertFalse(result)

    def test_get_capabilities_unknown_db(self):
        """Test get_capabilities with unknown database"""
        caps = DatabaseCompat.get_capabilities("oracle")
        self.assertEqual(caps, {})


class TestTypeEquivalence(unittest.TestCase):
    """Test field type equivalence between databases"""

    def test_mysql_to_postgres_text_types(self):
        """Test text type conversion from MySQL to PostgreSQL"""
        self.assertEqual(DatabaseCompat.get_equivalent_type("LONGTEXT", "mysql", "postgres"), "TEXT")
        self.assertEqual(DatabaseCompat.get_equivalent_type("MEDIUMTEXT", "mysql", "postgres"), "TEXT")
        self.assertEqual(DatabaseCompat.get_equivalent_type("TINYTEXT", "mysql", "postgres"), "TEXT")

    def test_mysql_to_postgres_numeric_types(self):
        """Test numeric type conversion from MySQL to PostgreSQL"""
        self.assertEqual(DatabaseCompat.get_equivalent_type("INT", "mysql", "postgres"), "INTEGER")
        self.assertEqual(DatabaseCompat.get_equivalent_type("FLOAT", "mysql", "postgres"), "REAL")
        self.assertEqual(DatabaseCompat.get_equivalent_type("DOUBLE", "mysql", "postgres"), "DOUBLE PRECISION")

    def test_mysql_to_postgres_json_type(self):
        """Test JSON type conversion from MySQL to PostgreSQL"""
        self.assertEqual(
            DatabaseCompat.get_equivalent_type("JSON", "mysql", "postgres"),
            "JSONB",  # Prefer JSONB for performance
        )

    def test_postgres_to_mysql_text_types(self):
        """Test text type conversion from PostgreSQL to MySQL"""
        self.assertEqual(DatabaseCompat.get_equivalent_type("TEXT", "postgres", "mysql"), "LONGTEXT")

    def test_postgres_to_mysql_array_no_equivalent(self):
        """Test that PostgreSQL ARRAY has no MySQL equivalent"""
        result = DatabaseCompat.get_equivalent_type("ARRAY", "postgres", "mysql")
        self.assertIsNone(result)

    def test_same_database_no_conversion(self):
        """Test that same database returns original type"""
        self.assertEqual(DatabaseCompat.get_equivalent_type("VARCHAR", "mysql", "mysql"), "VARCHAR")
        self.assertEqual(DatabaseCompat.get_equivalent_type("TEXT", "postgres", "postgres"), "TEXT")

    def test_case_insensitive_type_lookup(self):
        """Verify type lookup is case-insensitive"""
        self.assertEqual(DatabaseCompat.get_equivalent_type("longtext", "mysql", "postgres"), "TEXT")


class TestFieldValidation(unittest.TestCase):
    """Test field compatibility validation"""

    def test_validate_json_field_mysql(self):
        """Test JSONField validation for MySQL"""
        field = JSONField()
        is_compatible, warning = DatabaseCompat.validate_field_for_db(field, "mysql")

        self.assertTrue(is_compatible)
        self.assertIsNotNone(warning)
        self.assertIn("text", warning.lower())

    def test_validate_json_field_postgres(self):
        """Test JSONField validation for PostgreSQL"""
        field = JSONField()
        is_compatible, warning = DatabaseCompat.validate_field_for_db(field, "postgres")

        self.assertTrue(is_compatible)
        # PostgreSQL should have a note about using JSONB
        self.assertIsNotNone(warning)

    def test_validate_long_text_field(self):
        """Test LongTextField validation"""
        field = LongTextField()

        # Both databases should support LongTextField
        is_compat_mysql, _ = DatabaseCompat.validate_field_for_db(field, "mysql")
        is_compat_postgres, _ = DatabaseCompat.validate_field_for_db(field, "postgres")

        self.assertTrue(is_compat_mysql)
        self.assertTrue(is_compat_postgres)

    def test_validate_varchar_exceeds_max(self):
        """Test VARCHAR field that exceeds database maximum"""
        # Create a CharField with length exceeding MySQL limit
        field = CharField(max_length=70000)

        is_compatible, warning = DatabaseCompat.validate_field_for_db(field, "mysql")

        self.assertFalse(is_compatible)
        self.assertIn("65535", warning)

    def test_validate_varchar_postgres_no_limit(self):
        """Test VARCHAR field in PostgreSQL (no limit)"""
        # PostgreSQL has no practical VARCHAR limit
        field = CharField(max_length=70000)

        is_compatible, warning = DatabaseCompat.validate_field_for_db(field, "postgres")

        # Should be compatible since PostgreSQL has no limit
        self.assertTrue(is_compatible)

    def test_validate_datetime_tz_field(self):
        """Test DateTimeTzField validation"""
        field = DateTimeTzField()

        is_compat_mysql, warning_mysql = DatabaseCompat.validate_field_for_db(field, "mysql")
        is_compat_postgres, warning_postgres = DatabaseCompat.validate_field_for_db(field, "postgres")

        # Both compatible but MySQL may have warning
        self.assertTrue(is_compat_mysql)
        self.assertTrue(is_compat_postgres)

        # PostgreSQL should have no warning (native support)
        self.assertIsNone(warning_postgres)


class TestCapabilityDecorators(unittest.TestCase):
    """Test database capability decorators"""

    @patch("api.db.migrations.settings")
    def test_requires_decorator_supported_capability(self, mock_settings):
        """Test @requires decorator with supported capability"""
        mock_settings.DATABASE_TYPE = "postgres"

        @DatabaseCompat.requires("jsonb_support")
        def test_migration():
            return "executed"

        # Should execute since PostgreSQL supports JSONB
        result = test_migration()
        self.assertEqual(result, "executed")

    @patch("api.db.migrations.settings")
    def test_requires_decorator_unsupported_capability(self, mock_settings):
        """Test @requires decorator with unsupported capability"""
        mock_settings.DATABASE_TYPE = "mysql"

        @DatabaseCompat.requires("jsonb_support")
        def test_migration():
            return "executed"

        # Should not execute since MySQL doesn't support JSONB
        result = test_migration()
        self.assertIsNone(result)

    @patch("api.db.migrations.settings")
    def test_db_specific_decorator_matching_db(self, mock_settings):
        """Test @db_specific decorator with matching database"""
        mock_settings.DATABASE_TYPE = "mysql"

        @DatabaseCompat.db_specific("mysql")
        def mysql_only_migration():
            return "mysql executed"

        result = mysql_only_migration()
        self.assertEqual(result, "mysql executed")

    @patch("api.db.migrations.settings")
    def test_db_specific_decorator_non_matching_db(self, mock_settings):
        """Test @db_specific decorator with non-matching database"""
        mock_settings.DATABASE_TYPE = "postgres"

        @DatabaseCompat.db_specific("mysql")
        def mysql_only_migration():
            return "mysql executed"

        # Should not execute on PostgreSQL
        result = mysql_only_migration()
        self.assertIsNone(result)

    @patch("api.db.migrations.settings")
    def test_requires_with_specific_db_types(self, mock_settings):
        """Test @requires decorator with specific database types"""
        mock_settings.DATABASE_TYPE = "postgres"

        @DatabaseCompat.requires("transaction_ddl", ["postgres"])
        def postgres_transaction_migration():
            return "transactional"

        result = postgres_transaction_migration()
        self.assertEqual(result, "transactional")

    @patch("api.db.migrations.settings")
    def test_requires_with_list_of_db_types(self, mock_settings):
        """Test @requires decorator with list of allowed databases"""
        mock_settings.DATABASE_TYPE = "mysql"

        @DatabaseCompat.requires("full_text_search", ["mysql", "postgres"])
        def full_text_migration():
            return "full text"

        # Should execute since MySQL is in allowed list
        result = full_text_migration()
        self.assertEqual(result, "full text")


class TestDatabaseModelValidation(unittest.TestCase):
    """Test DataBaseModel field validation methods"""

    @unittest.skip("Requires full database setup - test in integration suite")
    def test_validate_fields_called(self):
        """Test that validate_fields can be called on a model"""
        pass

    @unittest.skip("Requires full database setup - test in integration suite")
    def test_get_field_info(self):
        """Test get_field_info returns detailed field information"""
        pass

    @unittest.skip("Requires full database setup - test in integration suite")
    def test_get_field_info_nonexistent(self):
        """Test get_field_info with non-existent field"""
        pass


class TestIntegrationScenarios(unittest.TestCase):
    """Test realistic database compatibility scenarios"""

    def test_migration_field_compatibility_check(self):
        """Test checking field compatibility before migration"""
        # Simulate checking if a new field is compatible with current DB
        field = JSONField()
        current_db = "postgres"

        is_compatible, warning = DatabaseCompat.validate_field_for_db(field, current_db)

        self.assertTrue(is_compatible)
        if warning:
            # Warning should be informational, not critical
            self.assertNotIn("not compatible", warning.lower())

    @patch("api.db.migrations.settings")
    def test_conditional_migration_execution(self, mock_settings):
        """Test that migrations only run on appropriate databases"""
        # Simulate a PostgreSQL-specific migration
        mock_settings.DATABASE_TYPE = "postgres"

        executed = []

        @DatabaseCompat.db_specific("postgres")
        def postgres_migration():
            executed.append("postgres")

        @DatabaseCompat.db_specific("mysql")
        def mysql_migration():
            executed.append("mysql")

        postgres_migration()
        mysql_migration()

        # Only postgres migration should execute
        self.assertEqual(executed, ["postgres"])

    def test_type_conversion_for_database_migration(self):
        """Test converting field types when migrating between databases"""
        # MySQL to PostgreSQL
        mysql_json = DatabaseCompat.get_equivalent_type("JSON", "mysql", "postgres")
        self.assertEqual(mysql_json, "JSONB")

        # PostgreSQL to MySQL
        postgres_text = DatabaseCompat.get_equivalent_type("TEXT", "postgres", "mysql")
        self.assertEqual(postgres_text, "LONGTEXT")


# Test priority markers can be added using pytest.mark.priority1, etc.
# instead of creating wrapper classes that cause duplicate test discovery


if __name__ == "__main__":
    # Run tests with verbose output
    unittest.main(verbosity=2)

#
# Centralized error handling for database migrations
#
# Provides consistent error categorization and handling across MySQL and PostgreSQL,
# enabling appropriate logging levels and error suppression based on error type.
#
from __future__ import annotations

import logging


class StandardErrorHandler:
    """
    Centralized error handling for database operations.

    Provides consistent error categorization and handling across MySQL and PostgreSQL,
    enabling appropriate logging levels and error suppression based on error type.
    """

    # Database-specific error codes for common migration issues
    ERROR_CODES = {
        "duplicate_column": {
            "mysql": [1060],  # Duplicate column name
            "postgres": ["42701"],  # Duplicate column
        },
        "duplicate_entry": {
            "mysql": [1062],  # Duplicate entry (constraint/unique)
            "postgres": ["23505", "42P07"],  # Unique violation, Relation already exists
        },
        "incompatible_type": {
            "mysql": [1366, 1292],  # Incorrect value for column, incorrect datetime value
            "postgres": ["42804"],  # Type mismatch
        },
        "missing_column": {
            "mysql": [1054],  # Unknown column
            "postgres": ["42703"],  # Undefined column
        },
    }

    # Common error message patterns for both databases
    ERROR_MESSAGES = {
        "duplicate_column": ["duplicate column", "already exists", "column already"],
        "duplicate_entry": ["duplicate entry", "unique constraint", "duplicate key"],
        "incompatible_type": ["cannot be cast", "incompatible", "type mismatch", "cannot alter", "data type"],
        "missing_column": ["does not exist", "no such column", "unknown column"],
    }

    @staticmethod
    def categorize_error(exception, db_type="postgres"):
        """
        Categorize an exception into a known error type.

        Returns:
            tuple: (category, is_expected, should_skip)
            - category: str, one of "duplicate_column", "incompatible_type", "missing_column", or "unknown"
            - is_expected: bool, True if this is a common/expected error
            - should_skip: bool, True if migration should be skipped (error already applied)
        """
        db_type = db_type.lower()
        error_str = str(exception).lower()

        # Check PostgreSQL SQLSTATE code
        pgcode = getattr(exception, "pgcode", None)
        if pgcode:
            for category, codes in StandardErrorHandler.ERROR_CODES.items():
                if pgcode in codes.get("postgres", []):
                    return (category, True, category == "duplicate_column")

        # Check MySQL error code (in exception.args[0])
        if isinstance(exception, Exception) and hasattr(exception, "args") and exception.args:
            error_code = exception.args[0]
            if isinstance(error_code, int):
                for category, codes in StandardErrorHandler.ERROR_CODES.items():
                    if error_code in codes.get("mysql", []):
                        return (category, True, category == "duplicate_column")

        # Check error message patterns
        for category, patterns in StandardErrorHandler.ERROR_MESSAGES.items():
            if any(pattern in error_str for pattern in patterns):
                return (category, True, category == "duplicate_column")

        # Unknown error - treat as unexpected
        return ("unknown", False, False)

    @staticmethod
    def handle_migration_error(exception, table, column, operation, db_type="postgres", db_name=None):
        """
        Handle migration error with appropriate logging level.

        This function performs error categorization and logging only. It does not return
        a value and should not be used to determine whether to skip migrations.

        Args:
            exception: The exception raised during migration
            table: Table name where operation was attempted
            column: Column name involved in operation
            operation: Operation name (add_column, alter_column_type, rename_column)
            db_type: Database type (mysql or postgres), defaults to postgres
            db_name: Database name for logging (optional, defaults to db_type.upper())
        """
        # Normalize db_type for consistent categorization and logging
        db_type = db_type.lower()
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(exception, db_type)

        # Use provided db_name or generate from db_type
        if db_name is None:
            db_name = db_type.upper()
        error_str = str(exception).lower()

        # Check for transaction abort (Postgres-specific, expected on rollback)
        if "transaction is aborted" in error_str:
            logging.info(f"Migration {operation} skipped (transaction aborted from prior failure): {db_name}.{table}.{column}")
            return

        if should_skip or (operation == "rename_column" and category == "missing_column"):
            # This is an expected error that means the migration was already applied
            # (e.g. column already exists, or for rename, the old column is gone)
            logging.debug(f"Migration {operation} skipped (already applied): {db_name}.{table}.{column} - {category}")
            return

        if is_expected:
            # This is an expected error but the operation needs attention
            logging.warning(f"Migration {operation} encountered expected issue: {db_name}.{table}.{column} - {category}: {exception}")
            return

        # Unexpected error - log as critical
        logging.critical(f"Migration {operation} failed with unexpected error: {db_name}.{table}.{column}: {exception}")

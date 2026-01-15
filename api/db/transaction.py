#
# Transaction logging and tracking
#
import logging

from peewee import PostgresqlDatabase, MySQLDatabase, SqliteDatabase
from playhouse.pool import PooledPostgresqlDatabase, PooledMySQLDatabase
from playhouse.postgres_ext import PostgresqlExtDatabase

try:
    from playhouse.pool import PooledPostgresqlExtDatabase
except ImportError:
    PooledPostgresqlExtDatabase = None


class TransactionLogger:
    """
    Log transaction state changes for debugging and monitoring.

    Provides visibility into transaction lifecycle (begin, commit, rollback)
    and helps track transaction isolation levels and nesting.
    """

    @staticmethod
    def _get_db_type(db):
        """
        Determine database type from database instance.

        Args:
            db: Database connection instance

        Returns:
            str: Database type ("PostgreSQL", "MySQL", "SQLite", or "Unknown")
        """
        # Check PostgreSQL variants
        if isinstance(db, (PostgresqlDatabase, PooledPostgresqlDatabase)):
            return "PostgreSQL"
        if PooledPostgresqlExtDatabase and isinstance(db, PooledPostgresqlExtDatabase):
            return "PostgreSQL"
        if isinstance(db, PostgresqlExtDatabase):
            return "PostgreSQL"

        # Check MySQL variants
        if isinstance(db, (MySQLDatabase, PooledMySQLDatabase)):
            return "MySQL"

        # Check SQLite
        if isinstance(db, SqliteDatabase):
            return "SQLite"

        return "Unknown"

    @staticmethod
    def log_transaction_state(db, operation="begin", extra_info=None):
        """
        Log transaction state changes with appropriate context.

        Args:
            db: Database connection instance
            operation: Transaction operation ("begin"|"commit"|"rollback")
            extra_info: Optional additional information to log
        """
        db_type = TransactionLogger._get_db_type(db)

        if operation == "begin":
            # Log transaction start with isolation level if available
            isolation = getattr(db, "isolation_level", "default")
            msg = f"{db_type} transaction started (isolation: {isolation})"
            if extra_info:
                msg += f" - {extra_info}"
            logging.debug(msg)

        elif operation == "commit":
            msg = f"{db_type} transaction committed"
            if extra_info:
                msg += f" - {extra_info}"
            logging.debug(msg)

        elif operation == "rollback":
            msg = f"{db_type} transaction rolled back"
            if extra_info:
                msg += f" - {extra_info}"
            logging.warning(msg)

        else:
            logging.warning(f"Unknown transaction operation: {operation}")

    @staticmethod
    def log_transaction_error(db, exception, context=None):
        """
        Log transaction errors with full context.

        Args:
            db: Database connection instance
            exception: The exception that occurred
            context: Optional context about what was being executed
        """
        db_type = TransactionLogger._get_db_type(db)
        msg = f"{db_type} transaction error"
        if context:
            msg += f" in {context}"
        msg += f": {exception}"
        logging.error(msg)

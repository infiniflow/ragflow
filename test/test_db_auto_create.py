"""
Unit tests for automatic database creation (ensure_database_exists).

Tests both PostgreSQL and MySQL database creation flows,
including error handling and idempotency.
"""

import sys
import unittest
from unittest.mock import MagicMock, patch

# Mock settings and other dependencies BEFORE any api imports
# This prevents errors during module initialization
_mock_settings = MagicMock()
_mock_settings.DATABASE_TYPE = "postgres"
_mock_settings.DATABASE = {"name": "test_db"}
sys.modules["common.settings"] = _mock_settings
sys.modules["common.config_utils"] = MagicMock()
sys.modules["common.decorator"] = MagicMock()

from api.db.connection import ensure_database_exists, get_database_config  # noqa: E402


class TestGetDatabaseConfig(unittest.TestCase):
    """Test database config extraction helper."""

    @patch("api.db.connection.settings")
    def test_postgres_config_extraction(self, mock_settings):
        """Extract PostgreSQL configuration correctly."""
        mock_settings.DATABASE_TYPE = "postgres"
        mock_settings.DATABASE = {
            "name": "ragflow_db",
            "host": "localhost",
            "port": 5432,
            "user": "ragflow_user",
            "password": "secret123",
        }

        config = get_database_config()

        self.assertEqual(config["type"], "postgres")
        self.assertEqual(config["name"], "ragflow_db")
        self.assertEqual(config["host"], "localhost")
        self.assertEqual(config["port"], 5432)
        self.assertEqual(config["user"], "ragflow_user")
        self.assertEqual(config["password"], "secret123")

    @patch("api.db.connection.settings")
    def test_mysql_config_extraction(self, mock_settings):
        """Extract MySQL configuration correctly."""
        mock_settings.DATABASE_TYPE = "mysql"
        mock_settings.DATABASE = {
            "name": "rag_flow",
            "host": "mysql-server",
            "port": 3306,
            "user": "root",
            "password": "password",
        }

        config = get_database_config()

        self.assertEqual(config["type"], "mysql")
        self.assertEqual(config["name"], "rag_flow")
        self.assertEqual(config["host"], "mysql-server")
        self.assertEqual(config["port"], 3306)

    @patch("api.db.connection.settings")
    def test_config_defaults(self, mock_settings):
        """Use defaults when host/port not specified."""
        mock_settings.DATABASE_TYPE = "postgres"
        mock_settings.DATABASE = {"name": "test_db", "user": "user", "password": "pass"}

        config = get_database_config()

        self.assertEqual(config["host"], "localhost")
        self.assertEqual(config["port"], 5432)  # Postgres default

    @patch("api.db.connection.settings")
    def test_mysql_port_default(self, mock_settings):
        """Use MySQL default port when not specified."""
        mock_settings.DATABASE_TYPE = "mysql"
        mock_settings.DATABASE = {"name": "db", "user": "user", "password": "pass"}

        config = get_database_config()

        self.assertEqual(config["port"], 3306)


class TestEnsureDatabaseExistsPostgres(unittest.TestCase):
    """Test PostgreSQL database creation."""

    @patch("api.db.connection.logging")
    @patch("api.db.connection.get_database_config")
    def test_postgres_creates_database(self, mock_get_config, mock_logging):
        """Create PostgreSQL database if missing using configured user."""
        mock_get_config.return_value = {
            "type": "postgres",
            "name": "ragflow_db",
            "host": "localhost",
            "port": 5432,
            "user": "postgres",
            "password": "secret",
        }

        # Mock psycopg2 and its sql module
        mock_psycopg2 = MagicMock()
        mock_sql = MagicMock()
        mock_sql.SQL = MagicMock(side_effect=lambda x: f"SQL({x})")
        mock_sql.Identifier = MagicMock(side_effect=lambda x: f'"{x}"')

        mock_conn = MagicMock()
        mock_cursor = MagicMock()
        mock_cursor.fetchone.return_value = None  # Database doesn't exist
        mock_psycopg2.connect.return_value = mock_conn
        mock_psycopg2.sql = mock_sql
        mock_conn.cursor.return_value = mock_cursor

        with patch.dict(sys.modules, {"psycopg2": mock_psycopg2, "psycopg2.sql": mock_sql}):
            ensure_database_exists()

            # Verify connection to postgres system DB with configured credentials
            mock_psycopg2.connect.assert_called_once_with(
                host="localhost",
                port=5432,
                user="postgres",
                password="secret",
                database="postgres"
            )

            # Verify database existence check and creation
            self.assertEqual(mock_cursor.execute.call_count, 2)  # SELECT + CREATE
            mock_cursor.close.assert_called_once()
            mock_conn.close.assert_called_once()

    @patch("api.db.connection.logging")
    @patch("api.db.connection.get_database_config")
    def test_postgres_missing_driver(self, mock_get_config, mock_logging):
        """Handle missing psycopg2 gracefully."""
        mock_get_config.return_value = {
            "type": "postgres",
            "name": "ragflow_db",
            "host": "localhost",
            "port": 5432,
            "user": "postgres",
            "password": "secret",
        }

        # Remove psycopg2 from modules to simulate ImportError
        with patch.dict(sys.modules, {"psycopg2": None}):
            ensure_database_exists()
            # Should warn but not raise
            self.assertTrue(mock_logging.warning.called and not mock_logging.error.called)

    @patch("api.db.connection.logging")
    @patch("api.db.connection.get_database_config")
    def test_postgres_connection_error_handled(self, mock_get_config, mock_logging):
        """Handle connection errors gracefully."""
        mock_get_config.return_value = {
            "type": "postgres",
            "name": "ragflow_db",
            "host": "bad-host",
            "port": 5432,
            "user": "postgres",
            "password": "secret",
        }

        # Mock psycopg2 to raise connection error
        mock_psycopg2 = MagicMock()
        mock_psycopg2.connect.side_effect = Exception("Connection refused")
        mock_sql = MagicMock()
        mock_psycopg2.sql = mock_sql

        with patch.dict(sys.modules, {"psycopg2": mock_psycopg2, "psycopg2.sql": mock_sql}):
            ensure_database_exists()
            # Should warn but not raise
            self.assertTrue(mock_logging.warning.called)


class TestEnsureDatabaseExistsMySQL(unittest.TestCase):
    """Test MySQL database creation."""

    @patch("api.db.connection.logging")
    @patch("api.db.connection.get_database_config")
    def test_mysql_creates_database(self, mock_get_config, mock_logging):
        """Create MySQL database if missing."""
        mock_get_config.return_value = {
            "type": "mysql",
            "name": "rag_flow",
            "host": "localhost",
            "port": 3306,
            "user": "root",
            "password": "secret",
        }

        # Mock mysql.connector
        mock_mysql = MagicMock()
        mock_mysql_connector = MagicMock()
        mock_mysql.connector = mock_mysql_connector

        mock_conn = MagicMock()
        mock_cursor = MagicMock()
        mock_mysql_connector.connect.return_value = mock_conn
        mock_conn.cursor.return_value = mock_cursor

        with patch.dict(sys.modules, {"mysql": mock_mysql, "mysql.connector": mock_mysql_connector}):
            ensure_database_exists()

            # Verify connection
            mock_mysql_connector.connect.assert_called_once()
            call_kwargs = mock_mysql_connector.connect.call_args[1]
            self.assertEqual(call_kwargs["host"], "localhost")
            self.assertEqual(call_kwargs["port"], 3306)

            # Verify CREATE DATABASE was executed
            mock_cursor.execute.assert_called_once()
            mock_cursor.close.assert_called_once()
            mock_conn.close.assert_called_once()

    @patch("api.db.connection.logging")
    @patch("api.db.connection.get_database_config")
    def test_mysql_missing_driver(self, mock_get_config, mock_logging):
        """Handle missing mysql.connector gracefully."""
        mock_get_config.return_value = {
            "type": "mysql",
            "name": "rag_flow",
            "host": "localhost",
            "port": 3306,
            "user": "root",
            "password": "secret",
        }

        # Remove mysql from modules to simulate ImportError
        with patch.dict(sys.modules, {"mysql": None, "mysql.connector": None}):
            ensure_database_exists()
            # Should warn but not raise
            self.assertTrue(mock_logging.warning.called)
            self.assertFalse(mock_logging.error.called)

    @patch("api.db.connection.logging")
    @patch("api.db.connection.get_database_config")
    def test_mysql_connection_error_handled(self, mock_get_config, mock_logging):
        """Handle connection errors gracefully."""
        mock_get_config.return_value = {
            "type": "mysql",
            "name": "rag_flow",
            "host": "bad-host",
            "port": 3306,
            "user": "root",
            "password": "secret",
        }

        # Mock mysql.connector to raise connection error
        mock_mysql = MagicMock()
        mock_mysql_connector = MagicMock()
        mock_mysql.connector = mock_mysql_connector
        mock_mysql_connector.connect.side_effect = Exception("Connection refused")

        with patch.dict(sys.modules, {"mysql": mock_mysql, "mysql.connector": mock_mysql_connector}):
            ensure_database_exists()
            # Should warn but not raise
            self.assertTrue(mock_logging.warning.called)


class TestEnsureDatabaseExistsIdempotency(unittest.TestCase):
    """Test idempotency of database creation."""

    @patch("api.db.connection.logging")
    @patch("api.db.connection.get_database_config")
    def test_postgres_idempotent(self, mock_get_config, mock_logging):
        """PostgreSQL creation is idempotent (safe to call multiple times)."""
        mock_get_config.return_value = {
            "type": "postgres",
            "name": "ragflow_db",
            "host": "localhost",
            "port": 5432,
            "user": "postgres",
            "password": "secret",
        }

        mock_psycopg2 = MagicMock()
        mock_sql = MagicMock()
        mock_sql.SQL = MagicMock(side_effect=lambda x: f"SQL({x})")
        mock_sql.Identifier = MagicMock(side_effect=lambda x: f'"{x}"')

        mock_conn = MagicMock()
        mock_cursor = MagicMock()
        mock_psycopg2.connect.return_value = mock_conn
        mock_psycopg2.sql = mock_sql
        mock_conn.cursor.return_value = mock_cursor

        with patch.dict(sys.modules, {"psycopg2": mock_psycopg2, "psycopg2.sql": mock_sql}):
            # Call multiple times
            ensure_database_exists()
            ensure_database_exists()

            # Both calls should succeed (CREATE IF NOT EXISTS)
            self.assertEqual(mock_psycopg2.connect.call_count, 2)

    @patch("api.db.connection.logging")
    @patch("api.db.connection.get_database_config")
    def test_mysql_idempotent(self, mock_get_config, mock_logging):
        """MySQL creation is idempotent (safe to call multiple times)."""
        mock_get_config.return_value = {
            "type": "mysql",
            "name": "rag_flow",
            "host": "localhost",
            "port": 3306,
            "user": "root",
            "password": "secret",
        }

        mock_mysql = MagicMock()
        mock_mysql_connector = MagicMock()
        mock_mysql.connector = mock_mysql_connector

        mock_conn = MagicMock()
        mock_cursor = MagicMock()
        mock_mysql_connector.connect.return_value = mock_conn
        mock_conn.cursor.return_value = mock_cursor

        with patch.dict(sys.modules, {"mysql": mock_mysql, "mysql.connector": mock_mysql_connector}):
            # Call multiple times
            ensure_database_exists()
            ensure_database_exists()

            # Both calls should succeed (CREATE IF NOT EXISTS)
            self.assertEqual(mock_mysql_connector.connect.call_count, 2)


class TestEnsureDatabaseExistsUnknownDatabase(unittest.TestCase):
    """Test handling of unknown database types."""

    @patch("api.db.connection.logging")
    @patch("api.db.connection.get_database_config")
    def test_unknown_database_type(self, mock_get_config, mock_logging):
        """Handle unknown database types gracefully."""
        mock_get_config.return_value = {
            "type": "sqlite",
            "name": "app.db",
            "host": "localhost",
            "port": 0,
            "user": "user",
            "password": "pass",
        }

        # Should not raise for unknown types
        ensure_database_exists()
        # No specific error required; just shouldn't crash


if __name__ == "__main__":
    unittest.main()

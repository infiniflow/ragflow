"""
Tests for Connection Pool Diagnostics (Phase 2 of Database Improvements)

These tests validate the PoolDiagnostics class functionality without requiring
a full database connection.
"""
import sys
import time
import unittest
from unittest.mock import MagicMock, patch

# Create comprehensive mocks before importing
mock_settings = MagicMock()
mock_settings.DATABASE_TYPE = "POSTGRES"
mock_settings.DATABASE = {"name": "test_db"}
sys.modules['common.settings'] = mock_settings
sys.modules['common.decorator'] = MagicMock()

# Mock the singleton decorator to do nothing
def mock_singleton(cls):
    return cls

sys.modules['common.decorator'].singleton = mock_singleton

# Now we can import
from api.db import connection  # noqa: E402

# Replace the DB initialization to prevent actual connection
connection.DB = MagicMock()

# Import the class we're testing
from api.db.connection import PoolDiagnostics  # noqa: E402


class TestPoolDiagnostics(unittest.TestCase):
    """Test suite for PoolDiagnostics class"""

    def setUp(self):
        """Set up test fixtures"""
        # Stop any existing monitoring
        PoolDiagnostics.stop_health_monitoring()

        # Create mock database
        self.mock_db = MagicMock()
        self.mock_db.max_connections = 10
        self.mock_db._in_use = {}
        self.mock_db._connections = []

    def tearDown(self):
        """Clean up after tests"""
        PoolDiagnostics.stop_health_monitoring()

    def test_get_pool_stats_empty_pool(self):
        """Test getting stats from an empty connection pool"""
        stats = PoolDiagnostics.get_pool_stats(self.mock_db)

        self.assertEqual(stats["max"], 10)
        self.assertEqual(stats["active"], 0)
        self.assertEqual(stats["idle"], 0)
        self.assertEqual(stats["utilization_percent"], 0.0)
        self.assertEqual(stats["total_created"], 0)

    def test_get_pool_stats_with_active_connections(self):
        """Test getting stats with active connections"""
        # Simulate 5 active connections
        self.mock_db._in_use = {f"conn_{i}": MagicMock() for i in range(5)}

        stats = PoolDiagnostics.get_pool_stats(self.mock_db)

        self.assertEqual(stats["active"], 5)
        self.assertEqual(stats["utilization_percent"], 50.0)
        self.assertEqual(stats["total_created"], 5)

    def test_get_pool_stats_with_idle_connections(self):
        """Test getting stats with idle connections in pool"""
        # Simulate 3 idle connections
        self.mock_db._connections = [MagicMock() for _ in range(3)]

        stats = PoolDiagnostics.get_pool_stats(self.mock_db)

        self.assertEqual(stats["idle"], 3)
        self.assertEqual(stats["active"], 0)
        self.assertEqual(stats["total_created"], 3)

    def test_get_pool_stats_with_mixed_connections(self):
        """Test getting stats with both active and idle connections"""
        # Simulate 7 active and 2 idle connections
        self.mock_db._in_use = {f"conn_{i}": MagicMock() for i in range(7)}
        self.mock_db._connections = [MagicMock() for _ in range(2)]

        stats = PoolDiagnostics.get_pool_stats(self.mock_db)

        self.assertEqual(stats["active"], 7)
        self.assertEqual(stats["idle"], 2)
        self.assertEqual(stats["utilization_percent"], 70.0)
        self.assertEqual(stats["total_created"], 9)

    def test_get_pool_stats_at_capacity(self):
        """Test getting stats when pool is at max capacity"""
        # Simulate pool at max capacity
        self.mock_db._in_use = {f"conn_{i}": MagicMock() for i in range(10)}

        stats = PoolDiagnostics.get_pool_stats(self.mock_db)

        self.assertEqual(stats["active"], 10)
        self.assertEqual(stats["utilization_percent"], 100.0)

    def test_get_pool_stats_handles_missing_attributes(self):
        """Test that get_pool_stats handles databases without expected attributes"""
        # Use spec=[] to create a mock that raises AttributeError for undefined attributes
        mock_db_minimal = MagicMock(spec=[])
        # MagicMock with spec=[] won't auto-create _in_use, _connections, max_connections
        # so accessing them will raise AttributeError as expected

        stats = PoolDiagnostics.get_pool_stats(mock_db_minimal)

        # Should return safe defaults
        self.assertEqual(stats["max"], 32)  # Default
        self.assertIsInstance(stats["utilization_percent"], (int, float))

    @patch('logging.debug')
    @patch('logging.warning')
    @patch('logging.critical')
    def test_log_pool_health_normal(self, mock_critical, mock_warning, mock_debug):
        """Test logging at normal utilization levels"""
        # 30% utilization (below warning threshold)
        self.mock_db._in_use = {f"conn_{i}": MagicMock() for i in range(3)}

        PoolDiagnostics.log_pool_health(self.mock_db)

        # Should log at debug level
        mock_debug.assert_called_once()
        mock_warning.assert_not_called()
        mock_critical.assert_not_called()

    @patch('logging.debug')
    @patch('logging.warning')
    @patch('logging.critical')
    def test_log_pool_health_warning_threshold(self, mock_critical, mock_warning, mock_debug):
        """Test logging when utilization exceeds warning threshold (80%)"""
        # 90% utilization
        self.mock_db._in_use = {f"conn_{i}": MagicMock() for i in range(9)}

        PoolDiagnostics.log_pool_health(self.mock_db)

        # Should log at warning level
        mock_warning.assert_called_once()
        self.assertIn("HIGH UTILIZATION", str(mock_warning.call_args))
        mock_critical.assert_not_called()
        mock_debug.assert_not_called()

    @patch('logging.debug')
    @patch('logging.warning')
    @patch('logging.critical')
    def test_log_pool_health_critical_threshold(self, mock_critical, mock_warning, mock_debug):
        """Test logging when utilization exceeds critical threshold (95%)"""
        # 100% utilization
        self.mock_db._in_use = {f"conn_{i}": MagicMock() for i in range(10)}

        PoolDiagnostics.log_pool_health(self.mock_db)

        # Should log at critical level
        mock_critical.assert_called_once()
        self.assertIn("CRITICAL", str(mock_critical.call_args))
        mock_warning.assert_not_called()
        mock_debug.assert_not_called()

    @patch('logging.info')
    def test_start_health_monitoring(self, mock_info):
        """Test starting background health monitoring"""
        PoolDiagnostics.start_health_monitoring(self.mock_db, interval=0.1)

        # Wait a moment for thread to start
        time.sleep(0.05)

        # Verify monitoring thread was created
        self.assertIsNotNone(PoolDiagnostics._monitoring_thread)
        self.assertTrue(PoolDiagnostics._monitoring_active)
        mock_info.assert_called_once()

    def test_start_health_monitoring_idempotent(self):
        """Test that starting monitoring multiple times doesn't create multiple threads"""
        PoolDiagnostics.start_health_monitoring(self.mock_db, interval=1)
        thread1 = PoolDiagnostics._monitoring_thread

        # Try to start again
        PoolDiagnostics.start_health_monitoring(self.mock_db, interval=1)
        thread2 = PoolDiagnostics._monitoring_thread

        # Should be the same thread
        self.assertIs(thread1, thread2)

    @patch('logging.info')
    def test_stop_health_monitoring(self, mock_info):
        """Test stopping background health monitoring"""
        PoolDiagnostics.start_health_monitoring(self.mock_db, interval=0.1)
        time.sleep(0.05)

        PoolDiagnostics.stop_health_monitoring()

        # Verify monitoring stopped
        self.assertIsNone(PoolDiagnostics._monitoring_thread)
        self.assertFalse(PoolDiagnostics._monitoring_active)
        # Verify info was called for start and stop
        self.assertEqual(mock_info.call_count, 2)

    @patch('logging.debug')
    @patch('logging.warning')
    @patch('logging.critical')
    def test_monitoring_loop_executes(self, mock_critical, mock_warning, mock_debug):
        """Test that the monitoring loop actually executes periodically"""
        PoolDiagnostics.start_health_monitoring(self.mock_db, interval=0.1)

        # Wait for at least 2 monitoring cycles
        time.sleep(0.25)

        # Should have logged health at least once
        # (either debug, warning, or critical depending on utilization)
        call_count = (
            mock_debug.call_count +
            mock_warning.call_count +
            mock_critical.call_count
        )
        self.assertGreater(call_count, 0)

    @patch('logging.error')
    @patch('logging.info')
    def test_monitoring_handles_exceptions(self, mock_info, mock_error):
        """Test that monitoring loop handles exceptions gracefully"""
        # Create a database mock that raises exceptions
        bad_db = MagicMock()
        # Make get_pool_stats fail instead
        with patch.object(PoolDiagnostics, 'get_pool_stats', side_effect=Exception("Simulated error")):
            PoolDiagnostics.start_health_monitoring(bad_db, interval=0.05)

            # Wait for monitoring cycle to encounter the error
            time.sleep(0.15)

            # Monitoring should still be active despite errors
            self.assertTrue(PoolDiagnostics._monitoring_active)

            # Should have logged an error
            self.assertGreater(mock_error.call_count, 0, "Expected error to be logged")


class TestPoolDiagnosticsIntegration(unittest.TestCase):
    """Integration tests that verify pool diagnostics work with real database objects"""

    @patch('logging.debug')
    def test_diagnostics_with_postgres_mock(self, mock_debug):
        """Test diagnostics with a PostgreSQL-like database mock"""
        from playhouse.pool import PooledPostgresqlDatabase

        # Create a mock that looks like PooledPostgresqlDatabase
        mock_postgres = MagicMock(spec=PooledPostgresqlDatabase)
        mock_postgres.max_connections = 20
        mock_postgres._in_use = {f"conn_{i}": MagicMock() for i in range(5)}
        mock_postgres._connections = [MagicMock() for _ in range(3)]

        stats = PoolDiagnostics.get_pool_stats(mock_postgres)

        self.assertEqual(stats["active"], 5)
        self.assertEqual(stats["idle"], 3)
        self.assertEqual(stats["max"], 20)

        # Log health should mention PostgreSQL
        PoolDiagnostics.log_pool_health(mock_postgres)
        call_args_str = str(mock_debug.call_args)
        self.assertIn("PostgreSQL", call_args_str)

    @patch('logging.warning')
    def test_diagnostics_with_mysql_mock(self, mock_warning):
        """Test diagnostics with a MySQL-like database mock"""
        from playhouse.pool import PooledMySQLDatabase

        # Create a mock that looks like PooledMySQLDatabase
        mock_mysql = MagicMock(spec=PooledMySQLDatabase)
        mock_mysql.max_connections = 15
        mock_mysql._in_use = {f"conn_{i}": MagicMock() for i in range(12)}
        mock_mysql._connections = []

        stats = PoolDiagnostics.get_pool_stats(mock_mysql)

        self.assertEqual(stats["active"], 12)
        self.assertEqual(stats["idle"], 0)
        self.assertEqual(stats["utilization_percent"], 80.0)

        # Log health should mention MySQL and warning
        PoolDiagnostics.log_pool_health(mock_mysql)
        call_args_str = str(mock_warning.call_args)
        self.assertIn("MySQL", call_args_str)


if __name__ == "__main__":
    unittest.main()

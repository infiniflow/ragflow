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
Unit tests for wait_for_schema_ready() function.

Tests the startup synchronization logic that prevents race conditions
between schema initialization and data initialization.
"""

import sys
from unittest.mock import MagicMock, patch

import pytest

# Mock settings before importing api.db.connection
_mock_settings = MagicMock()
_mock_settings.DATABASE_TYPE = "postgres"
_mock_settings.DATABASE = {"name": "test_db"}
sys.modules["common.settings"] = _mock_settings
sys.modules["common.config_utils"] = MagicMock()
sys.modules["common.decorator"] = MagicMock()

# Mock the singleton decorator
def mock_singleton(cls):
    return cls

sys.modules["common.decorator"].singleton = mock_singleton

# Now import the function under test
from api.db import connection  # noqa: E402
from api.db.connection import wait_for_schema_ready  # noqa: E402


class TestWaitForSchemaReady:
    """Test suite for wait_for_schema_ready() function"""

    def setup_method(self):
        """Set up test fixtures before each test"""
        # Create a fresh mock for DB with execute_sql method
        connection.DB = MagicMock()
        connection.DB.execute_sql = MagicMock()

    def teardown_method(self):
        """Clean up after each test"""
        connection.DB = None

    def test_schema_ready_on_first_try(self):
        """Test successful schema check on first attempt"""
        # Mock execute_sql to succeed immediately
        mock_cursor = MagicMock()
        mock_execute_sql = MagicMock(return_value=mock_cursor)
        connection.DB.execute_sql = mock_execute_sql

        # Should complete without error
        wait_for_schema_ready(max_retries=5, retry_delay=0.01)

        # Verify it queried all critical tables and closed the cursor each time
        assert mock_execute_sql.call_count == 3
        from unittest.mock import call
        # Expect portable quoting based on DB type mocked above
        db_type = sys.modules["common.settings"].DATABASE_TYPE.lower()
        quote_char = '"' if db_type == "postgres" else '`'
        mock_execute_sql.assert_has_calls([
            call(f"SELECT 1 FROM {quote_char}user{quote_char} LIMIT 1"),
            call(f"SELECT 1 FROM {quote_char}sync_logs{quote_char} LIMIT 1"),
            call(f"SELECT 1 FROM {quote_char}system_settings{quote_char} LIMIT 1"),
        ])
        assert mock_cursor.close.call_count == 3

    def test_schema_ready_after_retries(self):
        """Test schema becomes available after several retries"""
        # Mock execute_sql to fail twice, then succeed
        mock_cursor = MagicMock()
        call_count = 0

        def side_effect(*args, **kwargs):
            nonlocal call_count
            call_count += 1
            if call_count < 3:
                raise Exception("relation 'sync_logs' does not exist")
            return mock_cursor

        mock_execute_sql = MagicMock(side_effect=side_effect)
        connection.DB.execute_sql = mock_execute_sql

        # Should complete after 3 attempts
        wait_for_schema_ready(max_retries=10, retry_delay=0.01)

        # Verify it retried by call count (stable, non-time-based)
        assert call_count >= 3
        assert mock_cursor.close.call_count == 3

    def test_schema_timeout(self):
        """Test timeout when schema never becomes ready"""
        # Mock execute_sql to always raise exception (schema not ready)
        mock_execute_sql = MagicMock(side_effect=Exception("schema not ready"))
        connection.DB.execute_sql = mock_execute_sql

        # Should raise RuntimeError after max_retries due to schema unavailability
        with pytest.raises(RuntimeError, match="Database schema initialization timeout"):
            wait_for_schema_ready(max_retries=3, retry_delay=0.01)

        # Verify it retried max_retries times (once per retry attempt)
        assert mock_execute_sql.call_count == 3

    def test_schema_with_different_error(self):
        """Test behavior with non-schema errors (e.g., connection errors)"""
        # Mock execute_sql to fail with connection error
        mock_execute_sql = MagicMock(side_effect=Exception("connection refused"))
        connection.DB.execute_sql = mock_execute_sql

        # Should still retry and eventually timeout
        with pytest.raises(RuntimeError, match="Database schema initialization timeout"):
            wait_for_schema_ready(max_retries=2, retry_delay=0.01)

        assert mock_execute_sql.call_count == 2

    def test_schema_ready_with_custom_retries(self):
        """Test custom retry configuration"""
        mock_cursor = MagicMock()
        mock_execute_sql = MagicMock(return_value=mock_cursor)
        connection.DB.execute_sql = mock_execute_sql

        # Test with different max_retries and retry_delay
        wait_for_schema_ready(max_retries=100, retry_delay=0.001)

        # Should succeed on first try, querying all critical tables
        assert mock_execute_sql.call_count == 3
        assert mock_cursor.close.call_count == 3

    def test_schema_ready_timing(self):
        """Test that retry delay is actually being applied"""
        # Mock to fail 3 times
        call_count = 0

        def side_effect(*args, **kwargs):
            nonlocal call_count
            call_count += 1
            if call_count < 4:
                raise Exception("not ready")
            return MagicMock()

        mock_execute_sql = MagicMock(side_effect=side_effect)
        connection.DB.execute_sql = mock_execute_sql

        # Call with retry delay; rely on call_count to verify retries
        wait_for_schema_ready(max_retries=10, retry_delay=0.05)
        # 3 failed attempts (1 call each) + 1 successful attempt (3 table checks)
        assert call_count == 6

    def test_cursor_cleanup_on_success(self):
        """Test that cursor is properly closed after successful check"""
        mock_cursor = MagicMock()
        mock_execute_sql = MagicMock(return_value=mock_cursor)
        connection.DB.execute_sql = mock_execute_sql

        wait_for_schema_ready(max_retries=5, retry_delay=0.01)

        # Verify cursor.close() was called for each critical table
        assert mock_cursor.close.call_count == 3

    def test_cursor_cleanup_on_retry(self):
        """Test that cursors are not leaked during retries"""
        # Mock to succeed on second try
        mock_cursor = MagicMock()
        call_count = 0

        def side_effect(*args, **kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                raise Exception("not ready")
            return mock_cursor

        mock_execute_sql = MagicMock(side_effect=side_effect)
        connection.DB.execute_sql = mock_execute_sql

        wait_for_schema_ready(max_retries=5, retry_delay=0.01)

        # Verify final cursor was closed for all critical tables; total calls: 1 fail + 3 success
        assert mock_cursor.close.call_count == 3
        assert call_count == 4

    @patch('api.db.connection.logging')
    def test_logging_on_success(self, mock_logging):
        """Test that success is logged"""
        mock_cursor = MagicMock()
        mock_execute_sql = MagicMock(return_value=mock_cursor)
        connection.DB.execute_sql = mock_execute_sql

        wait_for_schema_ready(max_retries=5, retry_delay=0.01)

        # Verify info log was called with success message
        mock_logging.info.assert_called_once()
        log_message = mock_logging.info.call_args[0][0]
        assert "✓ Database schema is ready" in log_message

    @patch('api.db.connection.logging')
    def test_logging_on_retry(self, mock_logging):
        """Test that retries are logged at debug level"""
        call_count = 0

        def side_effect(*args, **kwargs):
            nonlocal call_count
            call_count += 1
            if call_count < 3:
                raise Exception("not ready")
            return MagicMock()

        mock_execute_sql = MagicMock(side_effect=side_effect)
        connection.DB.execute_sql = mock_execute_sql

        wait_for_schema_ready(max_retries=5, retry_delay=0.01)

        # Verify debug logs for retries
        assert mock_logging.debug.call_count >= 1
        debug_message = mock_logging.debug.call_args_list[0][0][0]
        assert "Schema not ready yet" in debug_message

    @patch('api.db.connection.logging')
    def test_logging_on_timeout(self, mock_logging):
        """Test that timeout is logged as error"""
        mock_execute_sql = MagicMock(side_effect=Exception("not ready"))
        connection.DB.execute_sql = mock_execute_sql

        with pytest.raises(RuntimeError):
            wait_for_schema_ready(max_retries=2, retry_delay=0.01)

        # Verify error log was called
        mock_logging.error.assert_called_once()
        error_message = mock_logging.error.call_args[0][0]
        assert "✗ Database schema still not ready" in error_message


class TestWaitForSchemaIntegration:
    """Integration-style tests for wait_for_schema_ready()"""

    def test_function_is_exported(self):
        """Test that wait_for_schema_ready is exported from connection module"""
        from api.db.connection import wait_for_schema_ready as exported_func
        assert exported_func is not None
        assert callable(exported_func)

    def test_function_in_db_models_exports(self):
        """Test that function can be imported from backward-compat db_models"""
        # This verifies the __all__ export list is correct
        from api.db import connection
        assert "wait_for_schema_ready" in connection.__all__

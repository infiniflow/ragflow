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
Unit tests for OceanBase health check and performance monitoring functionality.
"""
import inspect
import os
import types
import pytest
from unittest.mock import Mock, patch

from api.utils.health_utils import get_oceanbase_status, check_oceanbase_health


class TestOceanBaseHealthCheck:
    """Test cases for OceanBase health check functionality."""
    
    @patch('api.utils.health_utils.OBConnection')
    @patch.dict(os.environ, {'DOC_ENGINE': 'oceanbase'})
    def test_get_oceanbase_status_success(self, mock_ob_class):
        """Test successful OceanBase status retrieval."""
        # Setup mock
        mock_ob_connection = Mock()
        mock_ob_connection.uri = "localhost:2881"
        mock_ob_connection.health.return_value = {
            "uri": "localhost:2881",
            "version_comment": "OceanBase 4.3.5.1",
            "status": "healthy",
            "connection": "connected"
        }
        mock_ob_connection.get_performance_metrics.return_value = {
            "connection": "connected",
            "latency_ms": 5.2,
            "storage_used": "1.2MB",
            "storage_total": "100GB",
            "query_per_second": 150,
            "slow_queries": 2,
            "active_connections": 10,
            "max_connections": 300
        }
        mock_ob_class.return_value = mock_ob_connection
        
        # Execute
        result = get_oceanbase_status()
        
        # Assert
        assert result["status"] == "alive"
        assert "message" in result
        assert "health" in result["message"]
        assert "performance" in result["message"]
        assert result["message"]["health"]["status"] == "healthy"
        assert result["message"]["performance"]["latency_ms"] == 5.2
    
    @patch.dict(os.environ, {'DOC_ENGINE': 'elasticsearch'})
    def test_get_oceanbase_status_not_configured(self):
        """Test OceanBase status when not configured."""
        with pytest.raises(Exception) as exc_info:
            get_oceanbase_status()
        assert "OceanBase is not in use" in str(exc_info.value)
    
    @patch('api.utils.health_utils.OBConnection')
    @patch.dict(os.environ, {'DOC_ENGINE': 'oceanbase'})
    def test_get_oceanbase_status_connection_error(self, mock_ob_class):
        """Test OceanBase status when connection fails."""
        mock_ob_class.side_effect = Exception("Connection failed")
        
        result = get_oceanbase_status()
        
        assert result["status"] == "timeout"
        assert "error" in result["message"]
    
    @patch('api.utils.health_utils.OBConnection')
    @patch.dict(os.environ, {'DOC_ENGINE': 'oceanbase'})
    def test_check_oceanbase_health_healthy(self, mock_ob_class):
        """Test OceanBase health check returns healthy status."""
        mock_ob_connection = Mock()
        mock_ob_connection.health.return_value = {
            "uri": "localhost:2881",
            "version_comment": "OceanBase 4.3.5.1",
            "status": "healthy",
            "connection": "connected"
        }
        mock_ob_connection.get_performance_metrics.return_value = {
            "connection": "connected",
            "latency_ms": 5.2,
            "storage_used": "1.2MB",
            "storage_total": "100GB",
            "query_per_second": 150,
            "slow_queries": 0,
            "active_connections": 10,
            "max_connections": 300
        }
        mock_ob_class.return_value = mock_ob_connection
        
        result = check_oceanbase_health()
        
        assert result["status"] == "healthy"
        assert result["details"]["connection"] == "connected"
        assert result["details"]["latency_ms"] == 5.2
        assert result["details"]["query_per_second"] == 150
    
    @patch('api.utils.health_utils.OBConnection')
    @patch.dict(os.environ, {'DOC_ENGINE': 'oceanbase'})
    def test_check_oceanbase_health_degraded(self, mock_ob_class):
        """Test OceanBase health check returns degraded status for high latency."""
        mock_ob_connection = Mock()
        mock_ob_connection.health.return_value = {
            "uri": "localhost:2881",
            "version_comment": "OceanBase 4.3.5.1",
            "status": "healthy",
            "connection": "connected"
        }
        mock_ob_connection.get_performance_metrics.return_value = {
            "connection": "connected",
            "latency_ms": 1500.0,  # High latency > 1000ms
            "storage_used": "1.2MB",
            "storage_total": "100GB",
            "query_per_second": 50,
            "slow_queries": 5,
            "active_connections": 10,
            "max_connections": 300
        }
        mock_ob_class.return_value = mock_ob_connection
        
        result = check_oceanbase_health()
        
        assert result["status"] == "degraded"
        assert result["details"]["latency_ms"] == 1500.0
    
    @patch('api.utils.health_utils.OBConnection')
    @patch.dict(os.environ, {'DOC_ENGINE': 'oceanbase'})
    def test_check_oceanbase_health_unhealthy(self, mock_ob_class):
        """Test OceanBase health check returns unhealthy status."""
        mock_ob_connection = Mock()
        mock_ob_connection.health.return_value = {
            "uri": "localhost:2881",
            "status": "unhealthy",
            "connection": "disconnected",
            "error": "Connection timeout"
        }
        mock_ob_connection.get_performance_metrics.return_value = {
            "connection": "disconnected",
            "error": "Connection timeout"
        }
        mock_ob_class.return_value = mock_ob_connection
        
        result = check_oceanbase_health()
        
        assert result["status"] == "unhealthy"
        assert result["details"]["connection"] == "disconnected"
        assert "error" in result["details"]
    
    @patch.dict(os.environ, {'DOC_ENGINE': 'elasticsearch'})
    def test_check_oceanbase_health_not_configured(self):
        """Test OceanBase health check when not configured."""
        result = check_oceanbase_health()
        
        assert result["status"] == "not_configured"
        assert result["details"]["connection"] == "not_configured"
        assert "not configured" in result["details"]["message"].lower()


class TestOBConnectionPerformanceMetrics:
    """Test cases for OBConnection performance metrics methods."""
    
    def _create_mock_connection(self):
        """Create a mock OBConnection with actual methods."""
        # Create a simple object and bind the real methods to it
        class MockConn:
            pass
        conn = MockConn()
        # Get the actual class from the singleton wrapper's closure
        from rag.utils import ob_conn
        # OBConnection is wrapped by @singleton decorator, so it's a function
        # The original class is stored in the closure of the singleton function
        # Find the class by checking all closure cells
        ob_connection_class = None
        if hasattr(ob_conn.OBConnection, '__closure__') and ob_conn.OBConnection.__closure__:
            for cell in ob_conn.OBConnection.__closure__:
                cell_value = cell.cell_contents
                if inspect.isclass(cell_value):
                    ob_connection_class = cell_value
                    break
        
        if ob_connection_class is None:
            raise ValueError("Could not find OBConnection class in closure")
        
        # Bind the actual methods to our mock object
        conn.get_performance_metrics = types.MethodType(ob_connection_class.get_performance_metrics, conn)
        conn._get_storage_info = types.MethodType(ob_connection_class._get_storage_info, conn)
        conn._get_connection_pool_stats = types.MethodType(ob_connection_class._get_connection_pool_stats, conn)
        conn._get_slow_query_count = types.MethodType(ob_connection_class._get_slow_query_count, conn)
        conn._estimate_qps = types.MethodType(ob_connection_class._estimate_qps, conn)
        return conn
    
    def test_get_performance_metrics_success(self):
        """Test successful retrieval of performance metrics."""
        # Create mock connection with actual methods
        conn = self._create_mock_connection()
        mock_client = Mock()
        conn.client = mock_client
        conn.uri = "localhost:2881"
        conn.db_name = "test"
        
        # Mock client methods - create separate mock results for each call
        mock_result1 = Mock()
        mock_result1.fetchone.return_value = (1,)
        
        mock_result2 = Mock()
        mock_result2.fetchone.return_value = (100.5,)
        
        mock_result3 = Mock()
        mock_result3.fetchone.return_value = (100.0,)
        
        mock_result4 = Mock()
        mock_result4.fetchall.return_value = [
            (1, 'user', 'host', 'db', 'Query', 0, 'executing', 'SELECT 1')
        ]
        mock_result4.fetchone.return_value = ('max_connections', '300')
        
        mock_result5 = Mock()
        mock_result5.fetchone.return_value = (0,)
        
        mock_result6 = Mock()
        mock_result6.fetchone.return_value = (5,)
        
        # Setup side_effect to return different mocks for different queries
        def sql_side_effect(query):
            if "SELECT 1" in query:
                return mock_result1
            elif "information_schema.tables" in query:
                return mock_result2
            elif "__all_disk_stat" in query:
                return mock_result3
            elif "SHOW PROCESSLIST" in query:
                return mock_result4
            elif "SHOW VARIABLES LIKE 'max_connections'" in query:
                return mock_result4
            elif "information_schema.processlist" in query and "time >" in query:
                return mock_result5
            elif "information_schema.processlist" in query and "COUNT" in query:
                return mock_result6
            return Mock()
        
        mock_client.perform_raw_text_sql.side_effect = sql_side_effect
        mock_client.pool_size = 300
        
        # Mock logger
        import logging
        conn.logger = logging.getLogger('test')
        
        result = conn.get_performance_metrics()
        
        assert result["connection"] == "connected"
        assert result["latency_ms"] >= 0
        assert "storage_used" in result
        assert "storage_total" in result
    
    def test_get_performance_metrics_connection_error(self):
        """Test performance metrics when connection fails."""
        # Create mock connection with actual methods
        conn = self._create_mock_connection()
        mock_client = Mock()
        conn.client = mock_client
        conn.uri = "localhost:2881"
        conn.logger = Mock()
        
        mock_client.perform_raw_text_sql.side_effect = Exception("Connection failed")
        
        result = conn.get_performance_metrics()
        
        assert result["connection"] == "disconnected"
        assert "error" in result
    
    def test_get_storage_info_success(self):
        """Test successful retrieval of storage information."""
        # Create mock connection with actual methods
        conn = self._create_mock_connection()
        mock_client = Mock()
        conn.client = mock_client
        conn.db_name = "test"
        conn.logger = Mock()
        
        mock_result1 = Mock()
        mock_result1.fetchone.return_value = (100.5,)
        mock_result2 = Mock()
        mock_result2.fetchone.return_value = (100.0,)
        
        def sql_side_effect(query):
            if "information_schema.tables" in query:
                return mock_result1
            elif "__all_disk_stat" in query:
                return mock_result2
            return Mock()
        
        mock_client.perform_raw_text_sql.side_effect = sql_side_effect
        
        result = conn._get_storage_info()
        
        assert "storage_used" in result
        assert "storage_total" in result
        assert "MB" in result["storage_used"]
    
    def test_get_storage_info_fallback(self):
        """Test storage info with fallback when total space unavailable."""
        # Create mock connection with actual methods
        conn = self._create_mock_connection()
        mock_client = Mock()
        conn.client = mock_client
        conn.db_name = "test"
        conn.logger = Mock()
        
        # First query succeeds, second fails
        def side_effect(query):
            if "information_schema.tables" in query:
                mock_result = Mock()
                mock_result.fetchone.return_value = (100.5,)
                return mock_result
            else:
                raise Exception("Table not found")
        
        mock_client.perform_raw_text_sql.side_effect = side_effect
        
        result = conn._get_storage_info()
        
        assert "storage_used" in result
        assert "storage_total" in result
    
    def test_get_connection_pool_stats(self):
        """Test retrieval of connection pool statistics."""
        # Create mock connection with actual methods
        conn = self._create_mock_connection()
        mock_client = Mock()
        conn.client = mock_client
        conn.logger = Mock()
        mock_client.pool_size = 300
        
        mock_result1 = Mock()
        mock_result1.fetchall.return_value = [
            (1, 'user', 'host', 'db', 'Query', 0, 'executing', 'SELECT 1'),
            (2, 'user', 'host', 'db', 'Sleep', 10, None, None)
        ]
        
        mock_result2 = Mock()
        mock_result2.fetchone.return_value = ('max_connections', '300')
        
        def sql_side_effect(query):
            if "SHOW PROCESSLIST" in query:
                return mock_result1
            elif "SHOW VARIABLES LIKE 'max_connections'" in query:
                return mock_result2
            return Mock()
        
        mock_client.perform_raw_text_sql.side_effect = sql_side_effect
        
        result = conn._get_connection_pool_stats()
        
        assert "active_connections" in result
        assert "max_connections" in result
        assert result["active_connections"] >= 0
    
    def test_get_slow_query_count(self):
        """Test retrieval of slow query count."""
        # Create mock connection with actual methods
        conn = self._create_mock_connection()
        mock_client = Mock()
        conn.client = mock_client
        conn.logger = Mock()
        
        mock_result = Mock()
        mock_result.fetchone.return_value = (5,)
        mock_client.perform_raw_text_sql.return_value = mock_result
        
        result = conn._get_slow_query_count(threshold_seconds=1)
        
        assert isinstance(result, int)
        assert result >= 0
    
    def test_estimate_qps(self):
        """Test QPS estimation."""
        # Create mock connection with actual methods
        conn = self._create_mock_connection()
        mock_client = Mock()
        conn.client = mock_client
        conn.logger = Mock()
        
        mock_result = Mock()
        mock_result.fetchone.return_value = (10,)
        mock_client.perform_raw_text_sql.return_value = mock_result
        
        result = conn._estimate_qps()
        
        assert isinstance(result, int)
        assert result >= 0


if __name__ == "__main__":
    pytest.main([__file__, "-v"])


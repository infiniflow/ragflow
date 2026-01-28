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
import os
import pytest
from unittest.mock import Mock, patch, MagicMock
from timeit import default_timer as timer

from api.utils.health_utils import get_oceanbase_status, check_oceanbase_health
from rag.utils.ob_conn import OBConnection


class TestOceanBaseHealthCheck:
    """Test cases for OceanBase health check functionality."""
    
    @pytest.fixture
    def mock_ob_connection(self):
        """Create a mock OceanBase connection."""
        mock_conn = Mock(spec=OBConnection)
        mock_conn.uri = "localhost:2881"
        return mock_conn
    
    @patch('api.utils.health_utils.OBConnection')
    @patch.dict(os.environ, {'DOC_ENGINE': 'oceanbase'})
    def test_get_oceanbase_status_success(self, mock_ob_class, mock_ob_connection):
        """Test successful OceanBase status retrieval."""
        # Setup mock
        mock_ob_class.return_value = mock_ob_connection
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
    def test_check_oceanbase_health_healthy(self, mock_ob_class, mock_ob_connection):
        """Test OceanBase health check returns healthy status."""
        mock_ob_class.return_value = mock_ob_connection
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
        
        result = check_oceanbase_health()
        
        assert result["status"] == "healthy"
        assert result["details"]["connection"] == "connected"
        assert result["details"]["latency_ms"] == 5.2
        assert result["details"]["query_per_second"] == 150
    
    @patch('api.utils.health_utils.OBConnection')
    @patch.dict(os.environ, {'DOC_ENGINE': 'oceanbase'})
    def test_check_oceanbase_health_degraded(self, mock_ob_class, mock_ob_connection):
        """Test OceanBase health check returns degraded status for high latency."""
        mock_ob_class.return_value = mock_ob_connection
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
        
        result = check_oceanbase_health()
        
        assert result["status"] == "degraded"
        assert result["details"]["latency_ms"] == 1500.0
    
    @patch('api.utils.health_utils.OBConnection')
    @patch.dict(os.environ, {'DOC_ENGINE': 'oceanbase'})
    def test_check_oceanbase_health_unhealthy(self, mock_ob_class, mock_ob_connection):
        """Test OceanBase health check returns unhealthy status."""
        mock_ob_class.return_value = mock_ob_connection
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
    
    @pytest.fixture
    def mock_client(self):
        """Create a mock OceanBase client."""
        mock_client = Mock()
        return mock_client
    
    @patch('rag.utils.ob_conn.OBConnection.__init__', lambda self: None)
    def test_get_performance_metrics_success(self, mock_client):
        """Test successful retrieval of performance metrics."""
        conn = OBConnection()
        conn.client = mock_client
        conn.uri = "localhost:2881"
        conn.db_name = "test"
        
        # Mock client methods
        mock_client.perform_raw_text_sql.return_value.fetchone.side_effect = [
            (1,),  # SELECT 1
            (100.5,),  # Database size
            (100.0,),  # Total space
            [(1, 'user', 'host', 'db', 'Query', 0, 'executing', 'SELECT 1')],  # Processlist
            (300,),  # Max connections
            (0,),  # Slow queries
            (5,)  # Active queries
        ]
        mock_client.perform_raw_text_sql.return_value.fetchall.return_value = [
            (1, 'user', 'host', 'db', 'Query', 0, 'executing', 'SELECT 1')
        ]
        
        # Mock logger
        import logging
        conn.logger = logging.getLogger('test')
        
        result = conn.get_performance_metrics()
        
        assert result["connection"] == "connected"
        assert result["latency_ms"] >= 0
        assert "storage_used" in result
        assert "storage_total" in result
    
    @patch('rag.utils.ob_conn.OBConnection.__init__', lambda self: None)
    def test_get_performance_metrics_connection_error(self, mock_client):
        """Test performance metrics when connection fails."""
        conn = OBConnection()
        conn.client = mock_client
        conn.uri = "localhost:2881"
        conn.logger = Mock()
        
        mock_client.perform_raw_text_sql.side_effect = Exception("Connection failed")
        
        result = conn.get_performance_metrics()
        
        assert result["connection"] == "disconnected"
        assert "error" in result
    
    @patch('rag.utils.ob_conn.OBConnection.__init__', lambda self: None)
    def test_get_storage_info_success(self, mock_client):
        """Test successful retrieval of storage information."""
        conn = OBConnection()
        conn.client = mock_client
        conn.db_name = "test"
        conn.logger = Mock()
        
        mock_client.perform_raw_text_sql.return_value.fetchone.side_effect = [
            (100.5,),  # Database size in MB
            (100.0,)  # Total space in GB
        ]
        
        result = conn._get_storage_info()
        
        assert "storage_used" in result
        assert "storage_total" in result
        assert "MB" in result["storage_used"]
    
    @patch('rag.utils.ob_conn.OBConnection.__init__', lambda self: None)
    def test_get_storage_info_fallback(self, mock_client):
        """Test storage info with fallback when total space unavailable."""
        conn = OBConnection()
        conn.client = mock_client
        conn.db_name = "test"
        conn.logger = Mock()
        
        # First query succeeds, second fails
        mock_client.perform_raw_text_sql.return_value.fetchone.side_effect = [
            (100.5,),  # Database size
            Exception("Table not found")  # Total space query fails
        ]
        mock_client.perform_raw_text_sql.side_effect = [
            Mock(fetchone=lambda: (100.5,)),
            Exception("Table not found")
        ]
        
        # Reset side_effect for the actual call
        def side_effect(*args):
            if "information_schema.tables" in args[0]:
                return Mock(fetchone=lambda: (100.5,))
            else:
                raise Exception("Table not found")
        
        mock_client.perform_raw_text_sql.side_effect = side_effect
        
        result = conn._get_storage_info()
        
        assert "storage_used" in result
        assert "storage_total" in result
    
    @patch('rag.utils.ob_conn.OBConnection.__init__', lambda self: None)
    def test_get_connection_pool_stats(self, mock_client):
        """Test retrieval of connection pool statistics."""
        conn = OBConnection()
        conn.client = mock_client
        conn.logger = Mock()
        conn.client.pool_size = 300
        
        mock_client.perform_raw_text_sql.return_value.fetchall.return_value = [
            (1, 'user', 'host', 'db', 'Query', 0, 'executing', 'SELECT 1'),
            (2, 'user', 'host', 'db', 'Sleep', 10, None, None)
        ]
        mock_client.perform_raw_text_sql.return_value.fetchone.side_effect = [
            ('max_connections', '300')
        ]
        
        result = conn._get_connection_pool_stats()
        
        assert "active_connections" in result
        assert "max_connections" in result
        assert result["active_connections"] >= 0
    
    @patch('rag.utils.ob_conn.OBConnection.__init__', lambda self: None)
    def test_get_slow_query_count(self, mock_client):
        """Test retrieval of slow query count."""
        conn = OBConnection()
        conn.client = mock_client
        conn.logger = Mock()
        
        mock_client.perform_raw_text_sql.return_value.fetchone.return_value = (5,)
        
        result = conn._get_slow_query_count(threshold_seconds=1)
        
        assert isinstance(result, int)
        assert result >= 0
    
    @patch('rag.utils.ob_conn.OBConnection.__init__', lambda self: None)
    def test_estimate_qps(self, mock_client):
        """Test QPS estimation."""
        conn = OBConnection()
        conn.client = mock_client
        conn.logger = Mock()
        
        mock_client.perform_raw_text_sql.return_value.fetchone.return_value = (10,)
        
        result = conn._estimate_qps()
        
        assert isinstance(result, int)
        assert result >= 0


if __name__ == "__main__":
    pytest.main([__file__, "-v"])


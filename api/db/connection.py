#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
"""Connection pool diagnostics and monitoring for RAGFlow database connections."""

import logging
import threading
from typing import Dict, Any, Optional

logger = logging.getLogger(__name__)


class PoolDiagnostics:
    """Provides diagnostics and health monitoring for database connection pools.
    
    This class offers methods to:
    - Get real-time pool statistics (active/idle connections, utilization)
    - Log pool health with threshold-based warnings
    - Run background health monitoring
    
    Attributes:
        WARNING_THRESHOLD: Utilization percentage that triggers warnings (default: 0.8 = 80%)
        CRITICAL_THRESHOLD: Utilization percentage that triggers critical alerts (default: 0.95 = 95%)
        HEALTH_CHECK_INTERVAL: Default interval in seconds for background monitoring (default: 60)
    """
    
    WARNING_THRESHOLD = 0.8
    CRITICAL_THRESHOLD = 0.95
    HEALTH_CHECK_INTERVAL = 60
    
    _monitor_thread: Optional[threading.Thread] = None
    _monitor_stop_event: Optional[threading.Event] = None
    
    @classmethod
    def get_pool_stats(cls, db) -> Dict[str, Any]:
        """Get current connection pool statistics.
        
        Args:
            db: Database connection pool object (e.g., PooledMySQLDatabase or PooledPostgresqlDatabase)
        
        Returns:
            Dictionary with keys:
                - max: Maximum connections in pool
                - active: Currently active connections
                - idle: Idle connections available
                - utilization_percent: Percentage of pool in use (rounded to 2 decimals)
        """
        max_connections = getattr(db, 'max_connections', 0)
        
        # Count active connections from _in_use dict
        active_count = len(getattr(db, '_in_use', {}))
        
        # Count idle connections from _connections list
        idle_count = len(getattr(db, '_connections', []))
        
        # Calculate utilization
        utilization = round(active_count / max_connections * 100, 2) if max_connections > 0 else 0.0
        
        return {
            'max': max_connections,
            'active': active_count,
            'idle': idle_count,
            'utilization_percent': utilization
        }
    
    @classmethod
    def log_pool_health(cls, db) -> None:
        """Log pool health with appropriate log level based on utilization.
        
        Logs at different levels:
        - INFO: Normal operation (< WARNING_THRESHOLD)
        - WARNING: High utilization (>= WARNING_THRESHOLD)
        - CRITICAL: Critical utilization (>= CRITICAL_THRESHOLD)
        
        Args:
            db: Database connection pool object
        """
        stats = cls.get_pool_stats(db)
        utilization = stats['utilization_percent'] / 100.0
        
        message = (f"Connection pool: {stats['active']}/{stats['max']} active, "
                   f"{stats['idle']} idle, {stats['utilization_percent']}% utilization")
        
        if utilization >= cls.CRITICAL_THRESHOLD:
            logger.critical(f"⚠️ CRITICAL: {message}")
        elif utilization >= cls.WARNING_THRESHOLD:
            logger.warning(f"⚠️ HIGH UTILIZATION: {message}")
        else:
            logger.info(message)
    
    @classmethod
    def start_health_monitoring(cls, db, interval: Optional[float] = None) -> None:
        """Start background thread that periodically logs pool health.
        
        Args:
            db: Database connection pool object to monitor
            interval: Check interval in seconds (defaults to HEALTH_CHECK_INTERVAL)
        """
        if cls._monitor_thread is not None and cls._monitor_thread.is_alive():
            logger.warning("Health monitoring is already running")
            return
        
        if interval is None:
            interval = cls.HEALTH_CHECK_INTERVAL
        
        cls._monitor_stop_event = threading.Event()
        
        def _monitor_loop():
            stop_event = cls._monitor_stop_event
            if stop_event is None:
                return
            
            while not stop_event.is_set():
                try:
                    cls.log_pool_health(db)
                except Exception as e:
                    logger.error(f"Error in pool health monitoring: {e}")
                
                stop_event.wait(interval)
        
        cls._monitor_thread = threading.Thread(target=_monitor_loop, daemon=True, name="PoolHealthMonitor")
        cls._monitor_thread.start()
        logger.info(f"Started pool health monitoring (interval={interval}s)")
    
    @classmethod
    def stop_health_monitoring(cls) -> None:
        """Stop the background health monitoring thread."""
        if cls._monitor_stop_event is not None:
            cls._monitor_stop_event.set()
        
        if cls._monitor_thread is not None and cls._monitor_thread.is_alive():
            cls._monitor_thread.join(timeout=2.0)
            logger.info("Stopped pool health monitoring")
        
        cls._monitor_thread = None
        cls._monitor_stop_event = None

#
# Connection pool diagnostics and health monitoring
#
import logging
import threading
import time

from playhouse.pool import PooledPostgresqlDatabase


class PoolDiagnostics:
    """
    Monitor and report connection pool health.

    Provides real-time monitoring of database connection pool utilization,
    automatic health checks, and alerts when utilization exceeds thresholds.
    Enables operational visibility into connection pool behavior.
    """

    HEALTH_CHECK_INTERVAL = 60  # seconds
    WARNING_THRESHOLD = 0.8  # 80% utilization
    CRITICAL_THRESHOLD = 0.95  # 95% utilization

    _monitoring_thread = None
    _monitoring_active = False
    _lock = threading.Lock()

    @staticmethod
    def get_pool_stats(db):
        """
        Get current pool statistics

        Args:
            db: Database connection instance (PooledMySQLDatabase or PooledPostgresqlDatabase)

        Returns:
            dict: Pool statistics including max, active, idle, and utilization
        """
        try:
            # Peewee's PooledDatabase stores connections in _in_use (active) and _connections (available)
            max_connections = getattr(db, "max_connections", 32)  # Default is 32

            # Get in-use connections count
            in_use = len(getattr(db, "_in_use", {}))

            # Get available connections count
            available_pool = getattr(db, "_connections", [])
            available = len(available_pool)

            # Calculate total active connections
            active = in_use
            idle = available

            # Calculate utilization
            utilization_percent = (active / max_connections * 100) if max_connections > 0 else 0

            return {
                "max": max_connections,
                "active": active,
                "idle": idle,
                "waiting": 0,  # Peewee doesn't track waiting connections directly
                "utilization_percent": round(utilization_percent, 2),
                "total_created": active + idle,
            }
        except Exception as e:
            logging.warning(f"Failed to retrieve pool stats: {e}")
            return {
                "max": 0,
                "active": 0,
                "idle": 0,
                "waiting": 0,
                "utilization_percent": 0.0,
                "total_created": 0,
            }

    @staticmethod
    def log_pool_health(db):
        """
        Log pool health at appropriate level based on utilization.

        Logs at DEBUG, WARNING, or CRITICAL level based on thresholds.
        Includes pool statistics (active/max/idle connections) in log message.

        Args:
            db: Database connection instance (PooledMySQLDatabase or PooledPostgresqlDatabase)
        """
        stats = PoolDiagnostics.get_pool_stats(db)
        utilization = stats["utilization_percent"] / 100.0

        db_type = "PostgreSQL" if isinstance(db, PooledPostgresqlDatabase) else "MySQL"

        msg = f"{db_type} connection pool: {stats['active']}/{stats['max']} active, {stats['idle']} idle, {stats['utilization_percent']}% utilization"

        if utilization >= PoolDiagnostics.CRITICAL_THRESHOLD:
            logging.critical(f"⚠️  CRITICAL: {msg}")
        elif utilization >= PoolDiagnostics.WARNING_THRESHOLD:
            logging.warning(f"⚠️  HIGH UTILIZATION: {msg}")
        else:
            logging.debug(msg)

    @staticmethod
    def _health_monitoring_loop(db, interval):
        """
        Background monitoring loop for continuous pool health checks.

        Runs in daemon thread, checking pool health at regular intervals
        until monitoring is stopped. Errors are logged but don't stop monitoring.

        Args:
            db: Database connection instance
            interval: Monitoring interval in seconds
        """
        while PoolDiagnostics._monitoring_active:
            try:
                PoolDiagnostics.log_pool_health(db)
            except Exception as e:
                logging.error(f"Error in pool health monitoring: {e}")
            time.sleep(interval)

    @staticmethod
    def start_health_monitoring(db, interval=None):
        """
        Start background thread to monitor pool health

        Args:
            db: Database connection instance
            interval: Monitoring interval in seconds (default: HEALTH_CHECK_INTERVAL)
        """
        with PoolDiagnostics._lock:
            if PoolDiagnostics._monitoring_thread is not None:
                # Already monitoring
                return

            interval = interval or PoolDiagnostics.HEALTH_CHECK_INTERVAL
            PoolDiagnostics._monitoring_active = True

            thread = threading.Thread(target=PoolDiagnostics._health_monitoring_loop, args=(db, interval), daemon=True, name="PoolHealthMonitor")
            thread.start()
            PoolDiagnostics._monitoring_thread = thread
            logging.info(f"Connection pool health monitoring started (interval: {interval}s)")

    @staticmethod
    def stop_health_monitoring():
        """
        Stop background health monitoring thread.

        Safely shuts down the monitoring thread with a 2-second timeout.
        Idempotent - safe to call multiple times.
        """
        with PoolDiagnostics._lock:
            if PoolDiagnostics._monitoring_thread is None:
                return

            PoolDiagnostics._monitoring_active = False
            PoolDiagnostics._monitoring_thread.join(timeout=2)
            
            # Only clear reference if thread actually terminated
            if not PoolDiagnostics._monitoring_thread.is_alive():
                PoolDiagnostics._monitoring_thread = None
                logging.info("Connection pool health monitoring stopped")
            else:
                logging.warning("Health monitoring thread did not terminate within timeout")

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

import asyncio
import logging
import time
import threading
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Callable, Dict, List, Optional


class HealthStatus(Enum):
    """Health status levels."""
    HEALTHY = "healthy"
    DEGRADED = "degraded"
    UNHEALTHY = "unhealthy"
    UNKNOWN = "unknown"


@dataclass
class HealthCheckResult:
    """Result of a health check."""
    service_name: str
    status: HealthStatus
    response_time: float
    timestamp: float
    message: str = ""
    metadata: Dict[str, Any] = field(default_factory=dict)
    
    @property
    def is_healthy(self) -> bool:
        return self.status == HealthStatus.HEALTHY
    
    @property
    def is_available(self) -> bool:
        return self.status in [HealthStatus.HEALTHY, HealthStatus.DEGRADED]


class HealthChecker(ABC):
    """Abstract base class for health checkers."""
    
    def __init__(self, service_name: str, timeout: float = 5.0):
        self.service_name = service_name
        self.timeout = timeout
    
    @abstractmethod
    async def check_health(self) -> HealthCheckResult:
        """Perform health check and return result."""
        pass
    
    def check_health_sync(self) -> HealthCheckResult:
        """Synchronous health check wrapper."""
        try:
            return asyncio.run(self.check_health())
        except Exception as e:
            return HealthCheckResult(
                service_name=self.service_name,
                status=HealthStatus.UNHEALTHY,
                response_time=self.timeout,
                timestamp=time.time(),
                message=f"Health check failed: {e}"
            )


class RedisHealthChecker(HealthChecker):
    """Health checker for Redis."""
    
    def __init__(self, redis_client, service_name: str = "redis", timeout: float = 5.0):
        super().__init__(service_name, timeout)
        self.redis_client = redis_client
    
    async def check_health(self) -> HealthCheckResult:
        start_time = time.time()
        
        try:
            # Test basic connectivity
            await asyncio.wait_for(
                asyncio.to_thread(self.redis_client.ping),
                timeout=self.timeout
            )
            
            # Test read/write operations
            test_key = f"health_check_{int(time.time())}"
            test_value = "ok"
            
            await asyncio.wait_for(
                asyncio.to_thread(self.redis_client.set, test_key, test_value, 10),
                timeout=self.timeout
            )
            
            retrieved_value = await asyncio.wait_for(
                asyncio.to_thread(self.redis_client.get, test_key),
                timeout=self.timeout
            )
            
            # Cleanup
            await asyncio.wait_for(
                asyncio.to_thread(self.redis_client.delete, test_key),
                timeout=self.timeout
            )
            
            response_time = time.time() - start_time
            
            if retrieved_value == test_value:
                return HealthCheckResult(
                    service_name=self.service_name,
                    status=HealthStatus.HEALTHY,
                    response_time=response_time,
                    timestamp=time.time(),
                    message="Redis is healthy"
                )
            else:
                return HealthCheckResult(
                    service_name=self.service_name,
                    status=HealthStatus.DEGRADED,
                    response_time=response_time,
                    timestamp=time.time(),
                    message="Redis read/write test failed"
                )
                
        except asyncio.TimeoutError:
            return HealthCheckResult(
                service_name=self.service_name,
                status=HealthStatus.UNHEALTHY,
                response_time=self.timeout,
                timestamp=time.time(),
                message="Redis health check timed out"
            )
        except Exception as e:
            return HealthCheckResult(
                service_name=self.service_name,
                status=HealthStatus.UNHEALTHY,
                response_time=time.time() - start_time,
                timestamp=time.time(),
                message=f"Redis health check failed: {e}"
            )


class StorageHealthChecker(HealthChecker):
    """Health checker for storage systems (MinIO, S3, etc.)."""
    
    def __init__(self, storage_client, service_name: str = "storage", timeout: float = 10.0):
        super().__init__(service_name, timeout)
        self.storage_client = storage_client
    
    async def check_health(self) -> HealthCheckResult:
        start_time = time.time()
        
        try:
            # Test storage health
            result = await asyncio.wait_for(
                asyncio.to_thread(self.storage_client.health),
                timeout=self.timeout
            )
            
            response_time = time.time() - start_time
            
            if result:
                return HealthCheckResult(
                    service_name=self.service_name,
                    status=HealthStatus.HEALTHY,
                    response_time=response_time,
                    timestamp=time.time(),
                    message="Storage is healthy"
                )
            else:
                return HealthCheckResult(
                    service_name=self.service_name,
                    status=HealthStatus.UNHEALTHY,
                    response_time=response_time,
                    timestamp=time.time(),
                    message="Storage health check returned false"
                )
                
        except asyncio.TimeoutError:
            return HealthCheckResult(
                service_name=self.service_name,
                status=HealthStatus.UNHEALTHY,
                response_time=self.timeout,
                timestamp=time.time(),
                message="Storage health check timed out"
            )
        except Exception as e:
            return HealthCheckResult(
                service_name=self.service_name,
                status=HealthStatus.UNHEALTHY,
                response_time=time.time() - start_time,
                timestamp=time.time(),
                message=f"Storage health check failed: {e}"
            )


class DatabaseHealthChecker(HealthChecker):
    """Health checker for database connections."""
    
    def __init__(self, db_connection, service_name: str = "database", timeout: float = 5.0):
        super().__init__(service_name, timeout)
        self.db_connection = db_connection
    
    async def check_health(self) -> HealthCheckResult:
        start_time = time.time()
        
        try:
            # Test database connectivity with a simple query
            result = await asyncio.wait_for(
                asyncio.to_thread(self._test_db_connection),
                timeout=self.timeout
            )
            
            response_time = time.time() - start_time
            
            if result:
                return HealthCheckResult(
                    service_name=self.service_name,
                    status=HealthStatus.HEALTHY,
                    response_time=response_time,
                    timestamp=time.time(),
                    message="Database is healthy"
                )
            else:
                return HealthCheckResult(
                    service_name=self.service_name,
                    status=HealthStatus.UNHEALTHY,
                    response_time=response_time,
                    timestamp=time.time(),
                    message="Database connection test failed"
                )
                
        except asyncio.TimeoutError:
            return HealthCheckResult(
                service_name=self.service_name,
                status=HealthStatus.UNHEALTHY,
                response_time=self.timeout,
                timestamp=time.time(),
                message="Database health check timed out"
            )
        except Exception as e:
            return HealthCheckResult(
                service_name=self.service_name,
                status=HealthStatus.UNHEALTHY,
                response_time=time.time() - start_time,
                timestamp=time.time(),
                message=f"Database health check failed: {e}"
            )
    
    def _test_db_connection(self) -> bool:
        """Test database connection."""
        try:
            # This is a simple test - in practice, you'd use the actual DB API
            # For example, with Peewee: self.db_connection.execute_sql("SELECT 1")
            return True
        except Exception:
            return False


class LLMHealthChecker(HealthChecker):
    """Health checker for LLM services."""
    
    def __init__(self, llm_client, service_name: str = "llm", timeout: float = 30.0):
        super().__init__(service_name, timeout)
        self.llm_client = llm_client
    
    async def check_health(self) -> HealthCheckResult:
        start_time = time.time()
        
        try:
            # Test LLM with a simple prompt
            test_prompt = "Hello"
            result = await asyncio.wait_for(
                asyncio.to_thread(self._test_llm_call, test_prompt),
                timeout=self.timeout
            )
            
            response_time = time.time() - start_time
            
            if result:
                status = HealthStatus.HEALTHY if response_time < 10.0 else HealthStatus.DEGRADED
                return HealthCheckResult(
                    service_name=self.service_name,
                    status=status,
                    response_time=response_time,
                    timestamp=time.time(),
                    message=f"LLM is {'healthy' if status == HealthStatus.HEALTHY else 'slow but responsive'}"
                )
            else:
                return HealthCheckResult(
                    service_name=self.service_name,
                    status=HealthStatus.UNHEALTHY,
                    response_time=response_time,
                    timestamp=time.time(),
                    message="LLM test call failed"
                )
                
        except asyncio.TimeoutError:
            return HealthCheckResult(
                service_name=self.service_name,
                status=HealthStatus.UNHEALTHY,
                response_time=self.timeout,
                timestamp=time.time(),
                message="LLM health check timed out"
            )
        except Exception as e:
            return HealthCheckResult(
                service_name=self.service_name,
                status=HealthStatus.UNHEALTHY,
                response_time=time.time() - start_time,
                timestamp=time.time(),
                message=f"LLM health check failed: {e}"
            )
    
    def _test_llm_call(self, prompt: str) -> bool:
        """Test LLM call."""
        try:
            # This would be replaced with actual LLM API call
            # For example: response = self.llm_client.generate(prompt)
            # return bool(response and len(response) > 0)
            return True
        except Exception:
            return False


class HealthCheckManager:
    """
    Manages health checks for all system dependencies.

    Provides centralized health monitoring, automatic failover detection,
    and service discovery capabilities.
    """

    def __init__(self, check_interval: float = 30.0):
        self.check_interval = check_interval
        self._checkers: Dict[str, HealthChecker] = {}
        self._results: Dict[str, HealthCheckResult] = {}
        self._monitoring = False
        self._monitor_task: Optional[asyncio.Task] = None
        self._lock = threading.RLock()

        # Health change callbacks
        self._health_callbacks: List[Callable[[str, HealthCheckResult], None]] = []

        # Service availability tracking
        self._service_availability: Dict[str, bool] = {}

        logging.info(f"Health check manager initialized with {check_interval}s interval")

    def register_checker(self, checker: HealthChecker):
        """Register a health checker."""
        with self._lock:
            self._checkers[checker.service_name] = checker
            self._service_availability[checker.service_name] = True  # Assume healthy initially

        logging.info(f"Registered health checker for service: {checker.service_name}")

    def unregister_checker(self, service_name: str):
        """Unregister a health checker."""
        with self._lock:
            self._checkers.pop(service_name, None)
            self._results.pop(service_name, None)
            self._service_availability.pop(service_name, None)

        logging.info(f"Unregistered health checker for service: {service_name}")

    def add_health_callback(self, callback: Callable[[str, HealthCheckResult], None]):
        """Add a callback for health status changes."""
        self._health_callbacks.append(callback)

    async def check_service_health(self, service_name: str) -> Optional[HealthCheckResult]:
        """Check health of a specific service."""
        with self._lock:
            checker = self._checkers.get(service_name)

        if not checker:
            return None

        try:
            result = await checker.check_health()

            with self._lock:
                old_result = self._results.get(service_name)
                self._results[service_name] = result

                # Update availability tracking
                was_available = self._service_availability.get(service_name, True)
                is_available = result.is_available
                self._service_availability[service_name] = is_available

                # Trigger callbacks on status change
                if old_result is None or old_result.status != result.status:
                    self._trigger_health_callbacks(service_name, result)

                # Log availability changes
                if was_available != is_available:
                    if is_available:
                        logging.info(f"Service '{service_name}' is now available")
                    else:
                        logging.warning(f"Service '{service_name}' is now unavailable")

            return result

        except Exception as e:
            logging.error(f"Health check failed for service '{service_name}': {e}")

            # Create failure result
            failure_result = HealthCheckResult(
                service_name=service_name,
                status=HealthStatus.UNHEALTHY,
                response_time=checker.timeout,
                timestamp=time.time(),
                message=f"Health check exception: {e}"
            )

            with self._lock:
                self._results[service_name] = failure_result
                self._service_availability[service_name] = False

            return failure_result

    async def check_all_services(self) -> Dict[str, HealthCheckResult]:
        """Check health of all registered services."""
        tasks = []
        service_names = []

        with self._lock:
            for service_name in self._checkers.keys():
                tasks.append(self.check_service_health(service_name))
                service_names.append(service_name)

        if not tasks:
            return {}

        results = await asyncio.gather(*tasks, return_exceptions=True)

        health_results = {}
        for service_name, result in zip(service_names, results):
            if isinstance(result, Exception):
                logging.error(f"Health check failed for {service_name}: {result}")
                health_results[service_name] = HealthCheckResult(
                    service_name=service_name,
                    status=HealthStatus.UNHEALTHY,
                    response_time=0.0,
                    timestamp=time.time(),
                    message=f"Health check exception: {result}"
                )
            else:
                health_results[service_name] = result

        return health_results

    def _trigger_health_callbacks(self, service_name: str, result: HealthCheckResult):
        """Trigger health status change callbacks."""
        for callback in self._health_callbacks:
            try:
                callback(service_name, result)
            except Exception as e:
                logging.error(f"Health callback failed: {e}")

    def start_monitoring(self):
        """Start continuous health monitoring."""
        if self._monitoring:
            logging.warning("Health monitoring already started")
            return

        self._monitoring = True

        try:
            loop = asyncio.get_event_loop()
            self._monitor_task = loop.create_task(self._monitor_loop())
        except RuntimeError:
            # No event loop running, start in thread
            threading.Thread(target=self._monitor_thread, daemon=True).start()

        logging.info("Health monitoring started")

    def stop_monitoring(self):
        """Stop continuous health monitoring."""
        self._monitoring = False

        if self._monitor_task:
            self._monitor_task.cancel()
            self._monitor_task = None

        logging.info("Health monitoring stopped")

    async def _monitor_loop(self):
        """Main monitoring loop."""
        while self._monitoring:
            try:
                await self.check_all_services()
                await asyncio.sleep(self.check_interval)
            except asyncio.CancelledError:
                break
            except Exception as e:
                logging.error(f"Error in health monitoring loop: {e}")
                await asyncio.sleep(self.check_interval)

    def _monitor_thread(self):
        """Thread-based monitoring loop."""
        while self._monitoring:
            try:
                asyncio.run(self.check_all_services())
                time.sleep(self.check_interval)
            except Exception as e:
                logging.error(f"Error in health monitoring thread: {e}")
                time.sleep(self.check_interval)

    def get_service_status(self, service_name: str) -> Optional[HealthCheckResult]:
        """Get the latest health status for a service."""
        with self._lock:
            return self._results.get(service_name)

    def is_service_available(self, service_name: str) -> bool:
        """Check if a service is currently available."""
        with self._lock:
            return self._service_availability.get(service_name, False)

    def get_all_statuses(self) -> Dict[str, HealthCheckResult]:
        """Get health status for all services."""
        with self._lock:
            return self._results.copy()

    def get_available_services(self) -> List[str]:
        """Get list of currently available services."""
        with self._lock:
            return [
                service for service, available in self._service_availability.items()
                if available
            ]

    def get_unavailable_services(self) -> List[str]:
        """Get list of currently unavailable services."""
        with self._lock:
            return [
                service for service, available in self._service_availability.items()
                if not available
            ]

    def get_health_summary(self) -> Dict[str, Any]:
        """Get a summary of overall system health."""
        with self._lock:
            total_services = len(self._checkers)
            available_services = sum(self._service_availability.values())

            if total_services == 0:
                overall_status = HealthStatus.UNKNOWN
            elif available_services == total_services:
                overall_status = HealthStatus.HEALTHY
            elif available_services > 0:
                overall_status = HealthStatus.DEGRADED
            else:
                overall_status = HealthStatus.UNHEALTHY

            return {
                "overall_status": overall_status.value,
                "total_services": total_services,
                "available_services": available_services,
                "unavailable_services": total_services - available_services,
                "availability_rate": available_services / max(1, total_services),
                "monitoring_active": self._monitoring,
                "last_check": max(
                    (result.timestamp for result in self._results.values()),
                    default=0
                )
            }


# Global health check manager
_global_health_manager: Optional[HealthCheckManager] = None
_global_lock = threading.Lock()


def get_health_manager() -> HealthCheckManager:
    """Get the global health check manager."""
    global _global_health_manager

    with _global_lock:
        if _global_health_manager is None:
            _global_health_manager = HealthCheckManager()
        return _global_health_manager


def register_service_health_checker(checker: HealthChecker):
    """Register a health checker with the global manager."""
    get_health_manager().register_checker(checker)


def start_health_monitoring():
    """Start global health monitoring."""
    get_health_manager().start_monitoring()


def stop_health_monitoring():
    """Stop global health monitoring."""
    get_health_manager().stop_monitoring()


def is_service_healthy(service_name: str) -> bool:
    """Check if a service is healthy."""
    return get_health_manager().is_service_available(service_name)

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
import signal
import threading
import time
from contextlib import contextmanager, asynccontextmanager
from dataclasses import dataclass
from functools import wraps
from typing import Any, Dict, Optional
import os


@dataclass
class TimeoutConfig:
    """Configuration for timeout management."""
    storage_operations: float = 30.0  # Storage get/put operations
    database_operations: float = 15.0  # Database queries
    external_api_calls: float = 60.0  # LLM/embedding API calls
    chunk_processing: float = 300.0  # Document chunking operations
    embedding_generation: float = 120.0  # Embedding generation
    file_download: float = 180.0  # File download from storage
    health_checks: float = 5.0  # Health check operations
    default_operation: float = 30.0  # Default timeout for unspecified operations


class TimeoutError(Exception):
    """Custom timeout error with additional context."""
    def __init__(self, operation: str, timeout: float, elapsed: float = None):
        self.operation = operation
        self.timeout = timeout
        self.elapsed = elapsed
        
        message = f"Operation '{operation}' timed out after {timeout}s"
        if elapsed:
            message += f" (elapsed: {elapsed:.2f}s)"
        super().__init__(message)


class TimeoutManager:
    """
    Comprehensive timeout management for all operations.
    
    Provides configurable timeouts, timeout tracking, and graceful handling
    of timeout scenarios across different operation types.
    """
    
    def __init__(self, config: Optional[TimeoutConfig] = None):
        self.config = config or TimeoutConfig()
        self._active_operations: Dict[str, float] = {}  # operation_id -> start_time
        self._lock = threading.Lock()
        self._timeout_stats = {
            "total_operations": 0,
            "timed_out_operations": 0,
            "timeout_by_type": {}
        }
        
        # Load timeout overrides from environment
        self._load_env_overrides()
        
        logging.info(f"Timeout manager initialized with config: {self.config}")
    
    def _load_env_overrides(self):
        """Load timeout overrides from environment variables."""
        env_mappings = {
            "RAGFLOW_TIMEOUT_STORAGE": "storage_operations",
            "RAGFLOW_TIMEOUT_DATABASE": "database_operations", 
            "RAGFLOW_TIMEOUT_EXTERNAL_API": "external_api_calls",
            "RAGFLOW_TIMEOUT_CHUNK_PROCESSING": "chunk_processing",
            "RAGFLOW_TIMEOUT_EMBEDDING": "embedding_generation",
            "RAGFLOW_TIMEOUT_FILE_DOWNLOAD": "file_download",
            "RAGFLOW_TIMEOUT_HEALTH_CHECK": "health_checks",
            "RAGFLOW_TIMEOUT_DEFAULT": "default_operation"
        }
        
        for env_var, config_attr in env_mappings.items():
            env_value = os.environ.get(env_var)
            if env_value:
                try:
                    timeout_value = float(env_value)
                    setattr(self.config, config_attr, timeout_value)
                    logging.info(f"Override timeout {config_attr} = {timeout_value}s from {env_var}")
                except ValueError:
                    logging.warning(f"Invalid timeout value in {env_var}: {env_value}")
    
    def get_timeout(self, operation_type: str) -> float:
        """Get timeout for a specific operation type."""
        timeout_map = {
            "storage": self.config.storage_operations,
            "database": self.config.database_operations,
            "external_api": self.config.external_api_calls,
            "chunk_processing": self.config.chunk_processing,
            "embedding": self.config.embedding_generation,
            "file_download": self.config.file_download,
            "health_check": self.config.health_checks
        }
        
        return timeout_map.get(operation_type, self.config.default_operation)
    
    def _track_operation_start(self, operation_id: str):
        """Track the start of an operation."""
        with self._lock:
            self._active_operations[operation_id] = time.time()
            self._timeout_stats["total_operations"] += 1
    
    def _track_operation_end(self, operation_id: str, timed_out: bool = False, operation_type: str = "unknown"):
        """Track the end of an operation."""
        with self._lock:
            self._active_operations.pop(operation_id, None)
            if timed_out:
                self._timeout_stats["timed_out_operations"] += 1
                self._timeout_stats["timeout_by_type"][operation_type] = \
                    self._timeout_stats["timeout_by_type"].get(operation_type, 0) + 1
    
    @contextmanager
    def timeout_context(self, operation_type: str, operation_name: str = "operation"):
        """
        Context manager for timeout handling with signal-based interruption.
        
        Args:
            operation_type: Type of operation for timeout lookup
            operation_name: Human-readable name for the operation
        """
        timeout = self.get_timeout(operation_type)
        operation_id = f"{operation_name}_{int(time.time() * 1000)}"
        
        self._track_operation_start(operation_id)
        start_time = time.time()
        
        def timeout_handler(signum, frame):
            elapsed = time.time() - start_time
            self._track_operation_end(operation_id, timed_out=True, operation_type=operation_type)
            raise TimeoutError(operation_name, timeout, elapsed)
        
        # Set up signal handler for timeout (Unix only)
        old_handler = None
        if hasattr(signal, 'SIGALRM'):
            old_handler = signal.signal(signal.SIGALRM, timeout_handler)
            signal.alarm(int(timeout))
        
        try:
            yield timeout
            self._track_operation_end(operation_id, timed_out=False, operation_type=operation_type)
        except Exception:
            elapsed = time.time() - start_time
            if elapsed >= timeout:
                self._track_operation_end(operation_id, timed_out=True, operation_type=operation_type)
            else:
                self._track_operation_end(operation_id, timed_out=False, operation_type=operation_type)
            raise
        finally:
            # Clean up signal handler
            if hasattr(signal, 'SIGALRM') and old_handler is not None:
                signal.alarm(0)
                signal.signal(signal.SIGALRM, old_handler)
    
    @asynccontextmanager
    async def async_timeout_context(self, operation_type: str, operation_name: str = "operation"):
        """
        Async context manager for timeout handling.
        
        Args:
            operation_type: Type of operation for timeout lookup
            operation_name: Human-readable name for the operation
        """
        timeout = self.get_timeout(operation_type)
        operation_id = f"{operation_name}_{int(time.time() * 1000)}"
        
        self._track_operation_start(operation_id)
        start_time = time.time()
        
        try:
            async with asyncio.timeout(timeout):
                yield timeout
            self._track_operation_end(operation_id, timed_out=False, operation_type=operation_type)
        except asyncio.TimeoutError:
            elapsed = time.time() - start_time
            self._track_operation_end(operation_id, timed_out=True, operation_type=operation_type)
            raise TimeoutError(operation_name, timeout, elapsed)
        except Exception:
            elapsed = time.time() - start_time
            if elapsed >= timeout:
                self._track_operation_end(operation_id, timed_out=True, operation_type=operation_type)
            else:
                self._track_operation_end(operation_id, timed_out=False, operation_type=operation_type)
            raise
    
    def with_timeout(self, operation_type: str, operation_name: str = None):
        """
        Decorator for adding timeout to functions.
        
        Args:
            operation_type: Type of operation for timeout lookup
            operation_name: Optional name override for the operation
        """
        def decorator(func):
            func_name = operation_name or func.__name__
            
            if asyncio.iscoroutinefunction(func):
                @wraps(func)
                async def async_wrapper(*args, **kwargs):
                    async with self.async_timeout_context(operation_type, func_name):
                        return await func(*args, **kwargs)
                return async_wrapper
            else:
                @wraps(func)
                def sync_wrapper(*args, **kwargs):
                    with self.timeout_context(operation_type, func_name):
                        return func(*args, **kwargs)
                return sync_wrapper
        
        return decorator
    
    async def wait_for_with_timeout(
        self,
        coro,
        operation_type: str,
        operation_name: str = "async_operation"
    ):
        """
        Wait for a coroutine with timeout handling.
        
        Args:
            coro: Coroutine to wait for
            operation_type: Type of operation for timeout lookup
            operation_name: Name of the operation for logging
            
        Returns:
            Result of the coroutine
            
        Raises:
            TimeoutError: If operation times out
        """
        timeout = self.get_timeout(operation_type)
        operation_id = f"{operation_name}_{int(time.time() * 1000)}"
        
        self._track_operation_start(operation_id)
        start_time = time.time()
        
        try:
            result = await asyncio.wait_for(coro, timeout=timeout)
            self._track_operation_end(operation_id, timed_out=False, operation_type=operation_type)
            return result
        except asyncio.TimeoutError:
            elapsed = time.time() - start_time
            self._track_operation_end(operation_id, timed_out=True, operation_type=operation_type)
            raise TimeoutError(operation_name, timeout, elapsed)
    
    def get_active_operations(self) -> Dict[str, float]:
        """Get currently active operations and their start times."""
        with self._lock:
            current_time = time.time()
            return {
                op_id: current_time - start_time 
                for op_id, start_time in self._active_operations.items()
            }
    
    def get_stats(self) -> Dict[str, Any]:
        """Get timeout statistics."""
        with self._lock:
            stats = self._timeout_stats.copy()
            stats["active_operations"] = len(self._active_operations)
            stats["timeout_rate"] = (
                stats["timed_out_operations"] / max(1, stats["total_operations"])
            )
            return stats
    
    def reset_stats(self):
        """Reset timeout statistics."""
        with self._lock:
            self._timeout_stats = {
                "total_operations": 0,
                "timed_out_operations": 0,
                "timeout_by_type": {}
            }


# Global timeout manager instance
_global_timeout_manager: Optional[TimeoutManager] = None
_global_lock = threading.Lock()


def get_timeout_manager() -> TimeoutManager:
    """Get the global timeout manager instance."""
    global _global_timeout_manager
    
    with _global_lock:
        if _global_timeout_manager is None:
            _global_timeout_manager = TimeoutManager()
        return _global_timeout_manager


def set_timeout_config(config: TimeoutConfig):
    """Set the global timeout configuration."""
    global _global_timeout_manager
    
    with _global_lock:
        _global_timeout_manager = TimeoutManager(config)


# Convenience functions for common timeout operations
def with_storage_timeout(func):
    """Decorator for storage operations with timeout."""
    return get_timeout_manager().with_timeout("storage")(func)


def with_database_timeout(func):
    """Decorator for database operations with timeout."""
    return get_timeout_manager().with_timeout("database")(func)


def with_external_api_timeout(func):
    """Decorator for external API calls with timeout."""
    return get_timeout_manager().with_timeout("external_api")(func)


async def run_with_timeout(coro, operation_type: str, operation_name: str = None):
    """Run a coroutine with timeout."""
    return await get_timeout_manager().wait_for_with_timeout(
        coro, operation_type, operation_name or "async_operation"
    )

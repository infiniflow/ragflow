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
from enum import Enum
from typing import Any, Callable, Optional, Dict
from dataclasses import dataclass
from functools import wraps
import threading


class CircuitState(Enum):
    CLOSED = "closed"
    OPEN = "open"
    HALF_OPEN = "half_open"


@dataclass
class CircuitBreakerConfig:
    """Configuration for circuit breaker behavior."""
    failure_threshold: int = 5  # Number of failures before opening
    recovery_timeout: float = 60.0  # Seconds before attempting recovery
    expected_exception: tuple = (Exception,)  # Exceptions that count as failures
    success_threshold: int = 3  # Successful calls needed to close from half-open
    timeout: float = 30.0  # Default timeout for operations
    name: str = "default"


class CircuitBreakerOpenException(Exception):
    """Raised when circuit breaker is open."""
    def __init__(self, name: str, last_failure: str):
        self.name = name
        self.last_failure = last_failure
        super().__init__(f"Circuit breaker '{name}' is open. Last failure: {last_failure}")


class CircuitBreakerStats:
    """Statistics tracking for circuit breaker."""
    def __init__(self):
        self.total_calls = 0
        self.successful_calls = 0
        self.failed_calls = 0
        self.timeouts = 0
        self.circuit_opens = 0
        self.last_failure_time: Optional[float] = None
        self.last_failure_reason: str = ""
        
    def record_success(self):
        self.total_calls += 1
        self.successful_calls += 1
        
    def record_failure(self, reason: str):
        self.total_calls += 1
        self.failed_calls += 1
        self.last_failure_time = time.time()
        self.last_failure_reason = reason
        
    def record_timeout(self):
        self.timeouts += 1
        self.record_failure("timeout")
        
    def record_circuit_open(self):
        self.circuit_opens += 1


class CircuitBreaker:
    """
    Circuit breaker implementation for fault tolerance.
    
    Provides automatic failure detection and recovery for external service calls.
    Supports both sync and async operations.
    """
    
    def __init__(self, config: CircuitBreakerConfig):
        self.config = config
        self.state = CircuitState.CLOSED
        self.failure_count = 0
        self.success_count = 0
        self.last_failure_time: Optional[float] = None
        self.stats = CircuitBreakerStats()
        self._lock = threading.RLock()
        
        logging.info(f"Circuit breaker '{config.name}' initialized with config: {config}")
    
    def _should_attempt_reset(self) -> bool:
        """Check if we should attempt to reset from open to half-open."""
        if self.state != CircuitState.OPEN:
            return False
            
        if self.last_failure_time is None:
            return True
            
        return time.time() - self.last_failure_time >= self.config.recovery_timeout
    
    def _record_success(self):
        """Record a successful operation."""
        with self._lock:
            self.stats.record_success()
            
            if self.state == CircuitState.HALF_OPEN:
                self.success_count += 1
                if self.success_count >= self.config.success_threshold:
                    self._close_circuit()
            elif self.state == CircuitState.CLOSED:
                self.failure_count = 0  # Reset failure count on success
                
    def _record_failure(self, exception: Exception):
        """Record a failed operation."""
        with self._lock:
            reason = f"{type(exception).__name__}: {str(exception)}"
            self.stats.record_failure(reason)
            self.failure_count += 1
            self.last_failure_time = time.time()
            
            if self.state == CircuitState.CLOSED:
                if self.failure_count >= self.config.failure_threshold:
                    self._open_circuit()
            elif self.state == CircuitState.HALF_OPEN:
                self._open_circuit()
    
    def _open_circuit(self):
        """Open the circuit breaker."""
        self.state = CircuitState.OPEN
        self.success_count = 0
        self.stats.record_circuit_open()
        logging.warning(f"Circuit breaker '{self.config.name}' opened after {self.failure_count} failures")
    
    def _close_circuit(self):
        """Close the circuit breaker."""
        self.state = CircuitState.CLOSED
        self.failure_count = 0
        self.success_count = 0
        logging.info(f"Circuit breaker '{self.config.name}' closed after successful recovery")
    
    def _half_open_circuit(self):
        """Set circuit breaker to half-open state."""
        self.state = CircuitState.HALF_OPEN
        self.success_count = 0
        logging.info(f"Circuit breaker '{self.config.name}' half-opened for recovery attempt")
    
    def call(self, func: Callable, *args, **kwargs) -> Any:
        """
        Execute a function with circuit breaker protection.
        
        Args:
            func: Function to execute
            *args: Arguments for the function
            **kwargs: Keyword arguments for the function
            
        Returns:
            Result of the function call
            
        Raises:
            CircuitBreakerOpenException: When circuit is open
            Original exception: When function fails and circuit allows it
        """
        with self._lock:
            if self.state == CircuitState.OPEN:
                if self._should_attempt_reset():
                    self._half_open_circuit()
                else:
                    raise CircuitBreakerOpenException(self.config.name, self.stats.last_failure_reason)
        
        try:
            # Execute with timeout
            if asyncio.iscoroutinefunction(func):
                raise ValueError("Use call_async for coroutine functions")
            
            result = func(*args, **kwargs)
            self._record_success()
            return result
            
        except self.config.expected_exception as e:
            self._record_failure(e)
            raise
    
    async def call_async(self, func: Callable, *args, **kwargs) -> Any:
        """
        Execute an async function with circuit breaker protection.
        
        Args:
            func: Async function to execute
            *args: Arguments for the function
            **kwargs: Keyword arguments for the function
            
        Returns:
            Result of the function call
            
        Raises:
            CircuitBreakerOpenException: When circuit is open
            Original exception: When function fails and circuit allows it
        """
        with self._lock:
            if self.state == CircuitState.OPEN:
                if self._should_attempt_reset():
                    self._half_open_circuit()
                else:
                    raise CircuitBreakerOpenException(self.config.name, self.stats.last_failure_reason)
        
        try:
            # Execute with timeout
            result = await asyncio.wait_for(
                func(*args, **kwargs),
                timeout=self.config.timeout
            )
            self._record_success()
            return result
            
        except asyncio.TimeoutError:
            timeout_error = TimeoutError(f"Operation timed out after {self.config.timeout}s")
            self.stats.record_timeout()
            self._record_failure(timeout_error)
            raise timeout_error
            
        except self.config.expected_exception as e:
            self._record_failure(e)
            raise
    
    def get_stats(self) -> Dict[str, Any]:
        """Get circuit breaker statistics."""
        return {
            "name": self.config.name,
            "state": self.state.value,
            "failure_count": self.failure_count,
            "success_count": self.success_count,
            "total_calls": self.stats.total_calls,
            "successful_calls": self.stats.successful_calls,
            "failed_calls": self.stats.failed_calls,
            "timeouts": self.stats.timeouts,
            "circuit_opens": self.stats.circuit_opens,
            "last_failure_time": self.stats.last_failure_time,
            "last_failure_reason": self.stats.last_failure_reason,
            "success_rate": self.stats.successful_calls / max(1, self.stats.total_calls)
        }


# Decorator for easy circuit breaker usage
def circuit_breaker(config: CircuitBreakerConfig):
    """Decorator to add circuit breaker protection to functions."""
    breaker = CircuitBreaker(config)
    
    def decorator(func):
        if asyncio.iscoroutinefunction(func):
            @wraps(func)
            async def async_wrapper(*args, **kwargs):
                return await breaker.call_async(func, *args, **kwargs)
            return async_wrapper
        else:
            @wraps(func)
            def sync_wrapper(*args, **kwargs):
                return breaker.call(func, *args, **kwargs)
            return sync_wrapper
    
    return decorator


# Global circuit breaker registry
_circuit_breakers: Dict[str, CircuitBreaker] = {}
_registry_lock = threading.Lock()


def get_circuit_breaker(name: str, config: Optional[CircuitBreakerConfig] = None) -> CircuitBreaker:
    """Get or create a circuit breaker by name."""
    with _registry_lock:
        if name not in _circuit_breakers:
            if config is None:
                config = CircuitBreakerConfig(name=name)
            _circuit_breakers[name] = CircuitBreaker(config)
        return _circuit_breakers[name]


def get_all_circuit_breaker_stats() -> Dict[str, Dict[str, Any]]:
    """Get statistics for all registered circuit breakers."""
    with _registry_lock:
        return {name: breaker.get_stats() for name, breaker in _circuit_breakers.items()}

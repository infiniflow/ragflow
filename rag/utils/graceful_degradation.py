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
import threading
import time
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Callable, Dict, List, Optional, Union
import weakref


class DegradationLevel(Enum):
    """Levels of service degradation."""
    FULL_SERVICE = "full_service"
    REDUCED_FEATURES = "reduced_features"
    ESSENTIAL_ONLY = "essential_only"
    EMERGENCY_MODE = "emergency_mode"


@dataclass
class DegradationConfig:
    """Configuration for graceful degradation."""
    enable_fallbacks: bool = True
    cache_fallback_results: bool = True
    fallback_timeout: float = 10.0
    max_degradation_time: float = 300.0  # 5 minutes
    auto_recovery_enabled: bool = True
    recovery_check_interval: float = 30.0


@dataclass
class ServiceCapability:
    """Represents a service capability that can be degraded."""
    name: str
    required_services: List[str]
    fallback_handler: Optional[Callable] = None
    degradation_level: DegradationLevel = DegradationLevel.FULL_SERVICE
    is_essential: bool = False
    metadata: Dict[str, Any] = field(default_factory=dict)


class FallbackHandler(ABC):
    """Abstract base class for fallback handlers."""
    
    @abstractmethod
    async def handle_fallback(self, original_args: tuple, original_kwargs: dict) -> Any:
        """Handle fallback when primary service is unavailable."""
        pass
    
    @abstractmethod
    def can_handle_fallback(self, service_name: str) -> bool:
        """Check if this handler can provide fallback for the service."""
        pass


class CachedFallbackHandler(FallbackHandler):
    """Fallback handler that uses cached results."""
    
    def __init__(self, cache_duration: float = 3600.0):
        self.cache_duration = cache_duration
        self._cache: Dict[str, tuple] = {}  # key -> (result, timestamp)
        self._lock = threading.RLock()
    
    async def handle_fallback(self, original_args: tuple, original_kwargs: dict) -> Any:
        cache_key = self._generate_cache_key(original_args, original_kwargs)
        
        with self._lock:
            if cache_key in self._cache:
                result, timestamp = self._cache[cache_key]
                if time.time() - timestamp < self.cache_duration:
                    logging.info(f"Using cached fallback result for key: {cache_key}")
                    return result
        
        # No valid cache entry
        raise Exception("No cached fallback available")
    
    def can_handle_fallback(self, service_name: str) -> bool:
        return True  # Can handle any service if cache is available
    
    def cache_result(self, args: tuple, kwargs: dict, result: Any):
        """Cache a result for future fallback use."""
        cache_key = self._generate_cache_key(args, kwargs)
        with self._lock:
            self._cache[cache_key] = (result, time.time())
    
    def _generate_cache_key(self, args: tuple, kwargs: dict) -> str:
        """Generate cache key from arguments."""
        return f"{hash(args)}_{hash(tuple(sorted(kwargs.items())))}"


class SimpleFallbackHandler(FallbackHandler):
    """Simple fallback handler with predefined responses."""
    
    def __init__(self, fallback_responses: Dict[str, Any]):
        self.fallback_responses = fallback_responses
    
    async def handle_fallback(self, original_args: tuple, original_kwargs: dict) -> Any:
        # Return a simple fallback response
        service_name = original_kwargs.get('service_name', 'unknown')
        return self.fallback_responses.get(service_name, None)
    
    def can_handle_fallback(self, service_name: str) -> bool:
        return service_name in self.fallback_responses


class GracefulDegradationManager:
    """
    Manages graceful degradation of services when dependencies fail.
    
    Provides automatic fallback mechanisms, service capability management,
    and recovery detection for resilient system operation.
    """
    
    def __init__(self, config: Optional[DegradationConfig] = None):
        self.config = config or DegradationConfig()
        self._capabilities: Dict[str, ServiceCapability] = {}
        self._fallback_handlers: List[FallbackHandler] = []
        self._current_degradation_level = DegradationLevel.FULL_SERVICE
        self._degraded_services: Dict[str, float] = {}  # service -> degradation_start_time
        self._lock = threading.RLock()
        
        # Recovery monitoring
        self._recovery_task: Optional[asyncio.Task] = None
        self._monitoring_recovery = False
        
        # Degradation callbacks
        self._degradation_callbacks: List[Callable[[DegradationLevel], None]] = []
        
        logging.info(f"Graceful degradation manager initialized with config: {self.config}")
    
    def register_capability(self, capability: ServiceCapability):
        """Register a service capability."""
        with self._lock:
            self._capabilities[capability.name] = capability
        
        logging.info(f"Registered capability: {capability.name}")
    
    def register_fallback_handler(self, handler: FallbackHandler):
        """Register a fallback handler."""
        self._fallback_handlers.append(handler)
        logging.info(f"Registered fallback handler: {type(handler).__name__}")
    
    def add_degradation_callback(self, callback: Callable[[DegradationLevel], None]):
        """Add callback for degradation level changes."""
        self._degradation_callbacks.append(callback)
    
    async def handle_service_failure(self, service_name: str):
        """Handle failure of a service."""
        with self._lock:
            if service_name not in self._degraded_services:
                self._degraded_services[service_name] = time.time()
                logging.warning(f"Service '{service_name}' marked as degraded")
        
        # Recalculate degradation level
        await self._update_degradation_level()
        
        # Start recovery monitoring if not already running
        if self.config.auto_recovery_enabled and not self._monitoring_recovery:
            await self._start_recovery_monitoring()
    
    async def handle_service_recovery(self, service_name: str):
        """Handle recovery of a service."""
        with self._lock:
            if service_name in self._degraded_services:
                del self._degraded_services[service_name]
                logging.info(f"Service '{service_name}' recovered")
        
        # Recalculate degradation level
        await self._update_degradation_level()
    
    async def _update_degradation_level(self):
        """Update the current degradation level based on failed services."""
        with self._lock:
            degraded_services = set(self._degraded_services.keys())
            
            # Determine new degradation level
            new_level = self._calculate_degradation_level(degraded_services)
            
            if new_level != self._current_degradation_level:
                old_level = self._current_degradation_level
                self._current_degradation_level = new_level
                
                logging.info(f"Degradation level changed: {old_level.value} -> {new_level.value}")
                
                # Trigger callbacks
                for callback in self._degradation_callbacks:
                    try:
                        callback(new_level)
                    except Exception as e:
                        logging.error(f"Degradation callback failed: {e}")
    
    def _calculate_degradation_level(self, degraded_services: set) -> DegradationLevel:
        """Calculate degradation level based on failed services."""
        if not degraded_services:
            return DegradationLevel.FULL_SERVICE
        
        # Check if any essential capabilities are affected
        essential_affected = False
        total_affected_capabilities = 0
        total_capabilities = len(self._capabilities)
        
        for capability in self._capabilities.values():
            if any(service in degraded_services for service in capability.required_services):
                total_affected_capabilities += 1
                if capability.is_essential:
                    essential_affected = True
        
        # Determine degradation level
        if essential_affected:
            return DegradationLevel.EMERGENCY_MODE
        elif total_capabilities > 0:
            affected_ratio = total_affected_capabilities / total_capabilities
            if affected_ratio >= 0.75:
                return DegradationLevel.ESSENTIAL_ONLY
            elif affected_ratio >= 0.5:
                return DegradationLevel.REDUCED_FEATURES
            else:
                return DegradationLevel.REDUCED_FEATURES
        
        return DegradationLevel.REDUCED_FEATURES
    
    async def execute_with_fallback(
        self,
        capability_name: str,
        primary_func: Callable,
        *args,
        **kwargs
    ) -> Any:
        """Execute a function with fallback handling."""
        capability = self._capabilities.get(capability_name)
        if not capability:
            # No capability registered, execute normally
            return await self._execute_function(primary_func, *args, **kwargs)
        
        # Check if any required services are degraded
        degraded_services = set(self._degraded_services.keys())
        required_services = set(capability.required_services)
        
        if not (required_services & degraded_services):
            # All required services are available, execute normally
            try:
                result = await self._execute_function(primary_func, *args, **kwargs)
                
                # Cache successful result if enabled
                if self.config.cache_fallback_results:
                    self._cache_successful_result(args, kwargs, result)
                
                return result
            except Exception as e:
                logging.warning(f"Primary function failed for capability '{capability_name}': {e}")
                # Fall through to fallback handling
        
        # Try fallback mechanisms
        if self.config.enable_fallbacks:
            return await self._try_fallback(capability, args, kwargs)
        else:
            raise Exception(f"Capability '{capability_name}' unavailable and fallbacks disabled")
    
    async def _execute_function(self, func: Callable, *args, **kwargs) -> Any:
        """Execute a function (sync or async)."""
        if asyncio.iscoroutinefunction(func):
            return await func(*args, **kwargs)
        else:
            return await asyncio.to_thread(func, *args, **kwargs)
    
    async def _try_fallback(self, capability: ServiceCapability, args: tuple, kwargs: dict) -> Any:
        """Try fallback mechanisms for a capability."""
        # Try capability-specific fallback first
        if capability.fallback_handler:
            try:
                return await asyncio.wait_for(
                    self._execute_function(capability.fallback_handler, *args, **kwargs),
                    timeout=self.config.fallback_timeout
                )
            except Exception as e:
                logging.warning(f"Capability fallback failed: {e}")
        
        # Try registered fallback handlers
        for handler in self._fallback_handlers:
            if handler.can_handle_fallback(capability.name):
                try:
                    return await asyncio.wait_for(
                        handler.handle_fallback(args, kwargs),
                        timeout=self.config.fallback_timeout
                    )
                except Exception as e:
                    logging.warning(f"Fallback handler {type(handler).__name__} failed: {e}")
        
        # No fallback available
        raise Exception(f"No fallback available for capability '{capability.name}'")
    
    def _cache_successful_result(self, args: tuple, kwargs: dict, result: Any):
        """Cache successful result for future fallback use."""
        for handler in self._fallback_handlers:
            if isinstance(handler, CachedFallbackHandler):
                handler.cache_result(args, kwargs, result)
                break
    
    async def _start_recovery_monitoring(self):
        """Start monitoring for service recovery."""
        if self._monitoring_recovery:
            return
        
        self._monitoring_recovery = True
        
        try:
            loop = asyncio.get_event_loop()
            self._recovery_task = loop.create_task(self._recovery_monitor_loop())
        except RuntimeError:
            # No event loop, start in thread
            threading.Thread(target=self._recovery_monitor_thread, daemon=True).start()
        
        logging.info("Started recovery monitoring")
    
    async def _recovery_monitor_loop(self):
        """Recovery monitoring loop."""
        while self._monitoring_recovery:
            try:
                await self._check_for_recovery()
                await asyncio.sleep(self.config.recovery_check_interval)
            except asyncio.CancelledError:
                break
            except Exception as e:
                logging.error(f"Error in recovery monitoring: {e}")
                await asyncio.sleep(self.config.recovery_check_interval)
    
    def _recovery_monitor_thread(self):
        """Thread-based recovery monitoring."""
        while self._monitoring_recovery:
            try:
                asyncio.run(self._check_for_recovery())
                time.sleep(self.config.recovery_check_interval)
            except Exception as e:
                logging.error(f"Error in recovery monitoring thread: {e}")
                time.sleep(self.config.recovery_check_interval)
    
    async def _check_for_recovery(self):
        """Check for service recovery."""
        # This would integrate with the health check system
        # For now, we'll implement a simple timeout-based recovery
        current_time = time.time()
        recovered_services = []
        
        with self._lock:
            for service_name, degradation_time in list(self._degraded_services.items()):
                if current_time - degradation_time > self.config.max_degradation_time:
                    recovered_services.append(service_name)
        
        # Handle recovered services
        for service_name in recovered_services:
            await self.handle_service_recovery(service_name)
        
        # Stop monitoring if no degraded services remain
        with self._lock:
            if not self._degraded_services:
                self._monitoring_recovery = False
                if self._recovery_task:
                    self._recovery_task.cancel()
                    self._recovery_task = None
                logging.info("Stopped recovery monitoring - all services recovered")
    
    def get_current_degradation_level(self) -> DegradationLevel:
        """Get current degradation level."""
        return self._current_degradation_level
    
    def get_degraded_services(self) -> List[str]:
        """Get list of currently degraded services."""
        with self._lock:
            return list(self._degraded_services.keys())
    
    def is_capability_available(self, capability_name: str) -> bool:
        """Check if a capability is currently available."""
        capability = self._capabilities.get(capability_name)
        if not capability:
            return True  # Unknown capabilities are assumed available
        
        degraded_services = set(self._degraded_services.keys())
        required_services = set(capability.required_services)
        
        return not (required_services & degraded_services)
    
    def get_degradation_summary(self) -> Dict[str, Any]:
        """Get summary of current degradation state."""
        with self._lock:
            return {
                "degradation_level": self._current_degradation_level.value,
                "degraded_services": list(self._degraded_services.keys()),
                "total_capabilities": len(self._capabilities),
                "available_capabilities": sum(
                    1 for cap_name in self._capabilities.keys()
                    if self.is_capability_available(cap_name)
                ),
                "monitoring_recovery": self._monitoring_recovery,
                "fallback_handlers": len(self._fallback_handlers)
            }


# Global degradation manager
_global_degradation_manager: Optional[GracefulDegradationManager] = None
_global_lock = threading.Lock()


def get_degradation_manager() -> GracefulDegradationManager:
    """Get the global degradation manager."""
    global _global_degradation_manager
    
    with _global_lock:
        if _global_degradation_manager is None:
            _global_degradation_manager = GracefulDegradationManager()
        return _global_degradation_manager


async def execute_with_graceful_degradation(
    capability_name: str,
    primary_func: Callable,
    *args,
    **kwargs
) -> Any:
    """Execute function with graceful degradation."""
    return await get_degradation_manager().execute_with_fallback(
        capability_name, primary_func, *args, **kwargs
    )

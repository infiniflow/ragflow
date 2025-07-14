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
import gc
import logging
import os
import psutil
import threading
import time
from dataclasses import dataclass
from enum import Enum
from typing import Callable, Dict, List, Optional, Any
import weakref


class MemoryPressureLevel(Enum):
    """Memory pressure levels."""
    LOW = "low"
    MODERATE = "moderate"
    HIGH = "high"
    CRITICAL = "critical"


@dataclass
class MemoryThresholds:
    """Memory usage thresholds for different pressure levels."""
    moderate_threshold: float = 0.7  # 70% memory usage
    high_threshold: float = 0.85     # 85% memory usage
    critical_threshold: float = 0.95  # 95% memory usage


@dataclass
class MemoryStats:
    """Current memory statistics."""
    total_memory: int
    available_memory: int
    used_memory: int
    memory_percent: float
    process_memory: int
    process_memory_percent: float
    pressure_level: MemoryPressureLevel
    timestamp: float


class MemoryPressureMonitor:
    """
    Monitor system and process memory usage with dynamic concurrency adjustment.
    
    Provides real-time memory monitoring, pressure level detection, and automatic
    concurrency adjustment to prevent out-of-memory conditions.
    """
    
    def __init__(
        self,
        thresholds: Optional[MemoryThresholds] = None,
        check_interval: float = 5.0,
        enable_gc_on_pressure: bool = True
    ):
        self.thresholds = thresholds or MemoryThresholds()
        self.check_interval = check_interval
        self.enable_gc_on_pressure = enable_gc_on_pressure
        
        self._process = psutil.Process()
        self._monitoring = False
        self._monitor_task: Optional[asyncio.Task] = None
        self._lock = threading.Lock()
        
        # Memory statistics history
        self._stats_history: List[MemoryStats] = []
        self._max_history_size = 100
        
        # Callbacks for pressure level changes
        self._pressure_callbacks: Dict[MemoryPressureLevel, List[Callable]] = {
            level: [] for level in MemoryPressureLevel
        }
        
        # Concurrency limiters that can be adjusted
        self._managed_limiters: List[weakref.ref] = []
        self._original_limits: Dict[int, int] = {}  # limiter_id -> original_limit
        
        # Current pressure level
        self._current_pressure = MemoryPressureLevel.LOW
        
        logging.info(f"Memory pressure monitor initialized with thresholds: {self.thresholds}")
    
    def get_current_stats(self) -> MemoryStats:
        """Get current memory statistics."""
        # System memory
        system_memory = psutil.virtual_memory()
        
        # Process memory
        process_memory_info = self._process.memory_info()
        process_memory = process_memory_info.rss
        process_memory_percent = (process_memory / system_memory.total) * 100
        
        # Determine pressure level
        pressure_level = self._calculate_pressure_level(system_memory.percent / 100)
        
        return MemoryStats(
            total_memory=system_memory.total,
            available_memory=system_memory.available,
            used_memory=system_memory.used,
            memory_percent=system_memory.percent,
            process_memory=process_memory,
            process_memory_percent=process_memory_percent,
            pressure_level=pressure_level,
            timestamp=time.time()
        )
    
    def _calculate_pressure_level(self, memory_usage_ratio: float) -> MemoryPressureLevel:
        """Calculate memory pressure level based on usage ratio."""
        if memory_usage_ratio >= self.thresholds.critical_threshold:
            return MemoryPressureLevel.CRITICAL
        elif memory_usage_ratio >= self.thresholds.high_threshold:
            return MemoryPressureLevel.HIGH
        elif memory_usage_ratio >= self.thresholds.moderate_threshold:
            return MemoryPressureLevel.MODERATE
        else:
            return MemoryPressureLevel.LOW
    
    def start_monitoring(self):
        """Start memory monitoring."""
        with self._lock:
            if self._monitoring:
                logging.warning("Memory monitoring already started")
                return
            
            self._monitoring = True
            
        # Start monitoring in background
        try:
            loop = asyncio.get_event_loop()
            self._monitor_task = loop.create_task(self._monitor_loop())
        except RuntimeError:
            # No event loop running, start in thread
            threading.Thread(target=self._monitor_thread, daemon=True).start()
        
        logging.info("Memory pressure monitoring started")
    
    def stop_monitoring(self):
        """Stop memory monitoring."""
        with self._lock:
            self._monitoring = False
            
        if self._monitor_task:
            self._monitor_task.cancel()
            self._monitor_task = None
        
        logging.info("Memory pressure monitoring stopped")
    
    async def _monitor_loop(self):
        """Main monitoring loop (async)."""
        while self._monitoring:
            try:
                await self._check_memory_pressure()
                await asyncio.sleep(self.check_interval)
            except asyncio.CancelledError:
                break
            except Exception as e:
                logging.error(f"Error in memory monitoring loop: {e}")
                await asyncio.sleep(self.check_interval)
    
    def _monitor_thread(self):
        """Main monitoring loop (thread-based)."""
        while self._monitoring:
            try:
                asyncio.run(self._check_memory_pressure())
                time.sleep(self.check_interval)
            except Exception as e:
                logging.error(f"Error in memory monitoring thread: {e}")
                time.sleep(self.check_interval)
    
    async def _check_memory_pressure(self):
        """Check current memory pressure and take action."""
        stats = self.get_current_stats()
        
        # Store in history
        with self._lock:
            self._stats_history.append(stats)
            if len(self._stats_history) > self._max_history_size:
                self._stats_history.pop(0)
        
        # Check for pressure level change
        if stats.pressure_level != self._current_pressure:
            old_level = self._current_pressure
            self._current_pressure = stats.pressure_level
            
            logging.info(
                f"Memory pressure level changed: {old_level.value} -> {stats.pressure_level.value} "
                f"(Memory: {stats.memory_percent:.1f}%, Process: {stats.process_memory_percent:.1f}%)"
            )
            
            # Execute callbacks for new pressure level
            await self._execute_pressure_callbacks(stats.pressure_level, stats)
            
            # Adjust concurrency based on pressure level
            self._adjust_concurrency(stats.pressure_level)
            
            # Trigger garbage collection on high pressure
            if (self.enable_gc_on_pressure and 
                stats.pressure_level in [MemoryPressureLevel.HIGH, MemoryPressureLevel.CRITICAL]):
                gc.collect()
                logging.info("Triggered garbage collection due to memory pressure")
    
    async def _execute_pressure_callbacks(self, pressure_level: MemoryPressureLevel, stats: MemoryStats):
        """Execute callbacks for pressure level changes."""
        callbacks = self._pressure_callbacks.get(pressure_level, [])
        for callback in callbacks:
            try:
                if asyncio.iscoroutinefunction(callback):
                    await callback(pressure_level, stats)
                else:
                    callback(pressure_level, stats)
            except Exception as e:
                logging.error(f"Error executing pressure callback: {e}")
    
    def _adjust_concurrency(self, pressure_level: MemoryPressureLevel):
        """Adjust concurrency limits based on memory pressure."""
        # Calculate adjustment factor based on pressure level
        adjustment_factors = {
            MemoryPressureLevel.LOW: 1.0,      # No adjustment
            MemoryPressureLevel.MODERATE: 0.8,  # Reduce by 20%
            MemoryPressureLevel.HIGH: 0.5,     # Reduce by 50%
            MemoryPressureLevel.CRITICAL: 0.25  # Reduce by 75%
        }
        
        factor = adjustment_factors[pressure_level]
        
        # Adjust all managed limiters
        dead_refs = []
        for limiter_ref in self._managed_limiters:
            limiter = limiter_ref()
            if limiter is None:
                dead_refs.append(limiter_ref)
                continue
            
            limiter_id = id(limiter)
            
            # Store original limit if not already stored
            if limiter_id not in self._original_limits:
                if hasattr(limiter, '_value'):  # Semaphore
                    self._original_limits[limiter_id] = limiter._value
                elif hasattr(limiter, 'total_tokens'):  # CapacityLimiter
                    self._original_limits[limiter_id] = limiter.total_tokens
                else:
                    continue
            
            # Calculate new limit
            original_limit = self._original_limits[limiter_id]
            new_limit = max(1, int(original_limit * factor))
            
            # Apply new limit
            try:
                if hasattr(limiter, '_value'):  # Semaphore
                    limiter._value = new_limit
                elif hasattr(limiter, 'total_tokens'):  # CapacityLimiter
                    limiter.total_tokens = new_limit
                
                logging.debug(f"Adjusted limiter capacity: {original_limit} -> {new_limit} (factor: {factor})")
            except Exception as e:
                logging.error(f"Failed to adjust limiter: {e}")
        
        # Clean up dead references
        for dead_ref in dead_refs:
            self._managed_limiters.remove(dead_ref)
    
    def register_limiter(self, limiter):
        """Register a concurrency limiter for automatic adjustment."""
        self._managed_limiters.append(weakref.ref(limiter))
        logging.debug(f"Registered limiter for memory pressure adjustment: {type(limiter).__name__}")
    
    def add_pressure_callback(self, pressure_level: MemoryPressureLevel, callback: Callable):
        """Add a callback for specific pressure level changes."""
        self._pressure_callbacks[pressure_level].append(callback)
    
    def get_memory_history(self, max_entries: Optional[int] = None) -> List[MemoryStats]:
        """Get memory statistics history."""
        with self._lock:
            history = self._stats_history.copy()
            if max_entries:
                history = history[-max_entries:]
            return history
    
    def get_current_pressure_level(self) -> MemoryPressureLevel:
        """Get current memory pressure level."""
        return self._current_pressure
    
    def force_gc(self):
        """Force garbage collection."""
        collected = gc.collect()
        logging.info(f"Forced garbage collection: {collected} objects collected")
        return collected
    
    def get_memory_summary(self) -> Dict[str, Any]:
        """Get a summary of current memory status."""
        stats = self.get_current_stats()
        
        return {
            "pressure_level": stats.pressure_level.value,
            "system_memory_percent": stats.memory_percent,
            "process_memory_mb": stats.process_memory / (1024 * 1024),
            "process_memory_percent": stats.process_memory_percent,
            "available_memory_mb": stats.available_memory / (1024 * 1024),
            "managed_limiters": len(self._managed_limiters),
            "monitoring_active": self._monitoring,
            "thresholds": {
                "moderate": self.thresholds.moderate_threshold,
                "high": self.thresholds.high_threshold,
                "critical": self.thresholds.critical_threshold
            }
        }


# Global memory monitor instance
_global_memory_monitor: Optional[MemoryPressureMonitor] = None
_global_lock = threading.Lock()


def get_memory_monitor() -> MemoryPressureMonitor:
    """Get the global memory monitor instance."""
    global _global_memory_monitor
    
    with _global_lock:
        if _global_memory_monitor is None:
            _global_memory_monitor = MemoryPressureMonitor()
        return _global_memory_monitor


def start_memory_monitoring():
    """Start global memory monitoring."""
    get_memory_monitor().start_monitoring()


def stop_memory_monitoring():
    """Stop global memory monitoring."""
    monitor = get_memory_monitor()
    monitor.stop_monitoring()


def register_limiter_for_adjustment(limiter):
    """Register a limiter for automatic memory pressure adjustment."""
    get_memory_monitor().register_limiter(limiter)


def get_current_memory_pressure() -> MemoryPressureLevel:
    """Get current memory pressure level."""
    return get_memory_monitor().get_current_pressure_level()


def force_garbage_collection():
    """Force garbage collection."""
    return get_memory_monitor().force_gc()

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
import weakref
from abc import ABC, abstractmethod
from contextlib import asynccontextmanager, contextmanager
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional, Set, Callable, Union
from enum import Enum
import time
import uuid


class ResourceType(Enum):
    """Types of resources that can be managed."""
    STORAGE_OBJECT = "storage_object"
    DATABASE_CONNECTION = "database_connection"
    CHUNK_DATA = "chunk_data"
    TEMPORARY_FILE = "temporary_file"
    MEMORY_BUFFER = "memory_buffer"
    EXTERNAL_API_SESSION = "external_api_session"


@dataclass
class ResourceInfo:
    """Information about a managed resource."""
    resource_id: str
    resource_type: ResourceType
    resource_ref: Any  # Weak reference to the actual resource
    cleanup_func: Optional[Callable] = None
    metadata: Dict[str, Any] = field(default_factory=dict)
    created_at: float = field(default_factory=time.time)
    last_accessed: float = field(default_factory=time.time)
    cleanup_priority: int = 0  # Higher priority cleaned up first


class ResourceCleanupError(Exception):
    """Raised when resource cleanup fails."""
    pass


class ResourceManager:
    """
    Comprehensive resource manager for tracking and cleaning up resources.
    
    Provides automatic cleanup on failures, resource tracking, and lifecycle management.
    Thread-safe and supports both sync and async operations.
    """
    
    def __init__(self, name: str = "default"):
        self.name = name
        self._resources: Dict[str, ResourceInfo] = {}
        self._lock = threading.RLock()
        self._cleanup_callbacks: List[Callable] = []
        self._is_cleaning_up = False
        
        # Weak reference to track manager instances for global cleanup
        _register_manager(self)
        
        logging.info(f"Resource manager '{name}' initialized")
    
    def register_resource(
        self,
        resource: Any,
        resource_type: ResourceType,
        cleanup_func: Optional[Callable] = None,
        metadata: Optional[Dict[str, Any]] = None,
        cleanup_priority: int = 0
    ) -> str:
        """
        Register a resource for tracking and cleanup.
        
        Args:
            resource: The resource to track
            resource_type: Type of the resource
            cleanup_func: Function to call for cleanup
            metadata: Additional metadata about the resource
            cleanup_priority: Priority for cleanup order (higher first)
            
        Returns:
            Resource ID for tracking
        """
        resource_id = str(uuid.uuid4())
        
        with self._lock:
            if self._is_cleaning_up:
                logging.warning(f"Attempting to register resource during cleanup: {resource_id}")
                return resource_id
                
            resource_info = ResourceInfo(
                resource_id=resource_id,
                resource_type=resource_type,
                resource_ref=weakref.ref(resource) if resource is not None else None,
                cleanup_func=cleanup_func,
                metadata=metadata or {},
                cleanup_priority=cleanup_priority
            )
            
            self._resources[resource_id] = resource_info
            
        logging.debug(f"Registered resource {resource_id} of type {resource_type.value}")
        return resource_id
    
    def unregister_resource(self, resource_id: str) -> bool:
        """
        Unregister a resource from tracking.
        
        Args:
            resource_id: ID of the resource to unregister
            
        Returns:
            True if resource was found and removed
        """
        with self._lock:
            if resource_id in self._resources:
                del self._resources[resource_id]
                logging.debug(f"Unregistered resource {resource_id}")
                return True
        return False
    
    def update_resource_access(self, resource_id: str):
        """Update the last accessed time for a resource."""
        with self._lock:
            if resource_id in self._resources:
                self._resources[resource_id].last_accessed = time.time()
    
    def get_resource_info(self, resource_id: str) -> Optional[ResourceInfo]:
        """Get information about a registered resource."""
        with self._lock:
            return self._resources.get(resource_id)
    
    def list_resources(self, resource_type: Optional[ResourceType] = None) -> List[ResourceInfo]:
        """List all registered resources, optionally filtered by type."""
        with self._lock:
            resources = list(self._resources.values())
            if resource_type:
                resources = [r for r in resources if r.resource_type == resource_type]
            return resources
    
    def cleanup_resource(self, resource_id: str) -> bool:
        """
        Clean up a specific resource.
        
        Args:
            resource_id: ID of the resource to clean up
            
        Returns:
            True if cleanup was successful
        """
        with self._lock:
            resource_info = self._resources.get(resource_id)
            if not resource_info:
                return False
            
            success = self._cleanup_single_resource(resource_info)
            if success:
                del self._resources[resource_id]
            
            return success
    
    def _cleanup_single_resource(self, resource_info: ResourceInfo) -> bool:
        """Clean up a single resource."""
        try:
            # Get the actual resource if it still exists
            resource = None
            if resource_info.resource_ref:
                resource = resource_info.resource_ref()
            
            if resource_info.cleanup_func:
                if asyncio.iscoroutinefunction(resource_info.cleanup_func):
                    # Handle async cleanup function
                    try:
                        loop = asyncio.get_event_loop()
                        if loop.is_running():
                            # Create a task for async cleanup
                            task = loop.create_task(resource_info.cleanup_func(resource))
                            # Don't wait for completion to avoid blocking
                        else:
                            # Run in new event loop
                            asyncio.run(resource_info.cleanup_func(resource))
                    except Exception as e:
                        logging.error(f"Async cleanup failed for resource {resource_info.resource_id}: {e}")
                        return False
                else:
                    # Sync cleanup function
                    resource_info.cleanup_func(resource)
            
            logging.debug(f"Successfully cleaned up resource {resource_info.resource_id}")
            return True
            
        except Exception as e:
            logging.error(f"Failed to cleanup resource {resource_info.resource_id}: {e}")
            return False
    
    def cleanup_all(self, resource_type: Optional[ResourceType] = None) -> Dict[str, bool]:
        """
        Clean up all registered resources.
        
        Args:
            resource_type: Optional filter for resource type
            
        Returns:
            Dictionary mapping resource IDs to cleanup success status
        """
        with self._lock:
            if self._is_cleaning_up:
                logging.warning("Cleanup already in progress")
                return {}
                
            self._is_cleaning_up = True
            
        try:
            resources_to_cleanup = []
            
            with self._lock:
                for resource_info in self._resources.values():
                    if resource_type is None or resource_info.resource_type == resource_type:
                        resources_to_cleanup.append(resource_info)
            
            # Sort by cleanup priority (higher priority first)
            resources_to_cleanup.sort(key=lambda r: r.cleanup_priority, reverse=True)
            
            results = {}
            for resource_info in resources_to_cleanup:
                success = self._cleanup_single_resource(resource_info)
                results[resource_info.resource_id] = success
                
                if success:
                    with self._lock:
                        self._resources.pop(resource_info.resource_id, None)
            
            # Execute cleanup callbacks
            for callback in self._cleanup_callbacks:
                try:
                    callback()
                except Exception as e:
                    logging.error(f"Cleanup callback failed: {e}")
            
            logging.info(f"Resource manager '{self.name}' cleaned up {len(results)} resources")
            return results
            
        finally:
            with self._lock:
                self._is_cleaning_up = False
    
    def add_cleanup_callback(self, callback: Callable):
        """Add a callback to be executed during cleanup."""
        self._cleanup_callbacks.append(callback)
    
    def get_stats(self) -> Dict[str, Any]:
        """Get statistics about managed resources."""
        with self._lock:
            stats = {
                "name": self.name,
                "total_resources": len(self._resources),
                "resource_types": {},
                "oldest_resource_age": 0,
                "is_cleaning_up": self._is_cleaning_up
            }
            
            current_time = time.time()
            oldest_age = 0
            
            for resource_info in self._resources.values():
                # Count by type
                type_name = resource_info.resource_type.value
                stats["resource_types"][type_name] = stats["resource_types"].get(type_name, 0) + 1
                
                # Track oldest resource
                age = current_time - resource_info.created_at
                oldest_age = max(oldest_age, age)
            
            stats["oldest_resource_age"] = oldest_age
            return stats


# Context managers for automatic resource management

@contextmanager
def managed_resource(
    manager: ResourceManager,
    resource: Any,
    resource_type: ResourceType,
    cleanup_func: Optional[Callable] = None,
    **kwargs
):
    """Context manager for automatic resource cleanup."""
    resource_id = manager.register_resource(
        resource, resource_type, cleanup_func, **kwargs
    )
    try:
        yield resource_id
    finally:
        manager.cleanup_resource(resource_id)


@asynccontextmanager
async def managed_resource_async(
    manager: ResourceManager,
    resource: Any,
    resource_type: ResourceType,
    cleanup_func: Optional[Callable] = None,
    **kwargs
):
    """Async context manager for automatic resource cleanup."""
    resource_id = manager.register_resource(
        resource, resource_type, cleanup_func, **kwargs
    )
    try:
        yield resource_id
    finally:
        manager.cleanup_resource(resource_id)


# Global resource manager registry
_global_managers: Set[weakref.ref] = set()
_global_lock = threading.Lock()


def _register_manager(manager: ResourceManager):
    """Register a manager for global cleanup."""
    with _global_lock:
        _global_managers.add(weakref.ref(manager))


def cleanup_all_managers():
    """Clean up all registered resource managers."""
    with _global_lock:
        managers_to_cleanup = []
        dead_refs = set()
        
        for manager_ref in _global_managers:
            manager = manager_ref()
            if manager is not None:
                managers_to_cleanup.append(manager)
            else:
                dead_refs.add(manager_ref)
        
        # Remove dead references
        _global_managers -= dead_refs
    
    # Cleanup all managers
    for manager in managers_to_cleanup:
        try:
            manager.cleanup_all()
        except Exception as e:
            logging.error(f"Failed to cleanup manager '{manager.name}': {e}")


def get_global_resource_stats() -> Dict[str, Dict[str, Any]]:
    """Get statistics for all registered resource managers."""
    with _global_lock:
        stats = {}
        dead_refs = set()
        
        for manager_ref in _global_managers:
            manager = manager_ref()
            if manager is not None:
                stats[manager.name] = manager.get_stats()
            else:
                dead_refs.add(manager_ref)
        
        # Remove dead references
        _global_managers -= dead_refs
        
        return stats

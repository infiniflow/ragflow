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
Trace Service for Agent Execution Logging

This module provides the TraceService class that manages trace data persistence,
retrieval, and analysis. It integrates with the trace collector and formatter
modules to provide a complete tracing solution.
"""

import json
import logging
import time
from datetime import datetime, timedelta
from typing import Any, Optional, Tuple
from collections import defaultdict

from agent.trace.trace_models import (
    TraceEventType,
    TraceLevel,
    TraceSession,
    TraceEvent,
    TraceMetadata,
)
from agent.trace.trace_collector import (
    TraceCollector,
    get_trace_collector,
    create_trace_collector,
)
from agent.trace.trace_formatter import (
    TraceFormatterFactory,
    format_trace_for_api,
)
from rag.utils.redis_conn import REDIS_CONN


TRACE_KEY_PREFIX = "agent_trace:"
TRACE_SESSION_TTL = 86400 * 7
TRACE_EVENT_TTL = 86400 * 3


class TraceService:
    """
    Service for managing agent execution traces.
    
    Provides methods for creating, storing, retrieving, and analyzing
    trace data from agent executions.
    """

    @staticmethod
    def create_trace_session(
        task_id: str,
        agent_id: str,
        session_id: str,
        user_id: str,
        tenant_id: str,
        trace_level: str = "standard",
    ) -> Tuple[bool, str]:
        """
        Create a new trace session for an agent execution.
        
        Args:
            task_id: Unique identifier for the task
            agent_id: ID of the agent being executed
            session_id: ID of the conversation session
            user_id: ID of the user
            tenant_id: ID of the tenant
            trace_level: Verbosity level (minimal, standard, detailed, debug)
            
        Returns:
            Tuple of (success, trace_id or error message)
        """
        try:
            level = TraceLevel(trace_level)
        except ValueError:
            level = TraceLevel.STANDARD
        
        try:
            collector = create_trace_collector(
                task_id=task_id,
                agent_id=agent_id,
                session_id=session_id,
                user_id=user_id,
                tenant_id=tenant_id,
                trace_level=level,
            )
            
            session_data = {
                "task_id": task_id,
                "agent_id": agent_id,
                "session_id": session_id,
                "user_id": user_id,
                "tenant_id": tenant_id,
                "trace_level": trace_level,
                "created_at": datetime.utcnow().isoformat(),
                "status": "running",
            }
            
            key = f"{TRACE_KEY_PREFIX}session:{task_id}"
            REDIS_CONN.setex(key, TRACE_SESSION_TTL, json.dumps(session_data))
            
            return True, task_id
        except Exception as e:
            logging.exception(f"Failed to create trace session: {e}")
            return False, str(e)

    @staticmethod
    def get_trace_collector(task_id: str) -> Optional[TraceCollector]:
        """Get an active trace collector by task ID."""
        return get_trace_collector(task_id)

    @staticmethod
    def save_trace_session(task_id: str) -> Tuple[bool, str]:
        """
        Save the current trace session to persistent storage.
        
        Args:
            task_id: ID of the task/trace session
            
        Returns:
            Tuple of (success, message)
        """
        try:
            collector = get_trace_collector(task_id)
            if not collector:
                return False, "Trace collector not found"
            
            session = collector.get_session()
            session_dict = session.to_dict()
            
            key = f"{TRACE_KEY_PREFIX}session:{task_id}"
            REDIS_CONN.setex(key, TRACE_SESSION_TTL, json.dumps(session_dict))
            
            events_key = f"{TRACE_KEY_PREFIX}events:{task_id}"
            events_data = json.dumps([e.to_dict() for e in session.events])
            REDIS_CONN.setex(events_key, TRACE_EVENT_TTL, events_data)
            
            return True, "Trace session saved successfully"
        except Exception as e:
            logging.exception(f"Failed to save trace session: {e}")
            return False, str(e)

    @staticmethod
    def get_trace_session(task_id: str) -> Optional[dict[str, Any]]:
        """
        Retrieve a trace session by task ID.
        
        Args:
            task_id: ID of the task/trace session
            
        Returns:
            Trace session data or None if not found
        """
        try:
            collector = get_trace_collector(task_id)
            if collector:
                return collector.get_session().to_dict()
            
            key = f"{TRACE_KEY_PREFIX}session:{task_id}"
            data = REDIS_CONN.get(key)
            if data:
                return json.loads(data)
            
            return None
        except Exception as e:
            logging.exception(f"Failed to get trace session: {e}")
            return None

    @staticmethod
    def get_trace_events(
        task_id: str,
        event_types: Optional[list[str]] = None,
        component_id: Optional[str] = None,
        limit: int = 100,
        offset: int = 0,
    ) -> list[dict[str, Any]]:
        """
        Retrieve trace events with optional filtering.
        
        Args:
            task_id: ID of the task/trace session
            event_types: Filter by event types
            component_id: Filter by component ID
            limit: Maximum number of events to return
            offset: Number of events to skip
            
        Returns:
            List of trace events
        """
        try:
            collector = get_trace_collector(task_id)
            if collector:
                events = collector.get_events()
            else:
                events_key = f"{TRACE_KEY_PREFIX}events:{task_id}"
                data = REDIS_CONN.get(events_key)
                if not data:
                    return []
                events = [TraceEvent.from_dict(e) for e in json.loads(data)]
            
            if event_types:
                type_set = set(event_types)
                events = [e for e in events if e.event_type.value in type_set]
            
            if component_id:
                events = [e for e in events if e.component_id == component_id]
            
            events = events[offset:offset + limit]
            return [e.to_dict() for e in events]
        except Exception as e:
            logging.exception(f"Failed to get trace events: {e}")
            return []

    @staticmethod
    def get_trace_summary(task_id: str) -> Optional[dict[str, Any]]:
        """
        Get a summary of the trace session.
        
        Args:
            task_id: ID of the task/trace session
            
        Returns:
            Summary data or None if not found
        """
        try:
            collector = get_trace_collector(task_id)
            if collector:
                return collector.get_summary()
            
            session_data = TraceService.get_trace_session(task_id)
            if session_data and "summary" in session_data:
                return session_data["summary"]
            
            return None
        except Exception as e:
            logging.exception(f"Failed to get trace summary: {e}")
            return None

    @staticmethod
    def format_trace(
        task_id: str,
        format_type: str = "streaming",
        **kwargs
    ) -> Optional[dict[str, Any]]:
        """
        Format a trace session using the specified formatter.
        
        Args:
            task_id: ID of the task/trace session
            format_type: Type of formatter (streaming, compact, detailed)
            **kwargs: Additional formatter options
            
        Returns:
            Formatted trace data or None if not found
        """
        try:
            session_data = TraceService.get_trace_session(task_id)
            if not session_data:
                return None
            
            collector = get_trace_collector(task_id)
            if collector:
                session = collector.get_session()
                return format_trace_for_api(session, format_type, **kwargs)
            
            return session_data
        except Exception as e:
            logging.exception(f"Failed to format trace: {e}")
            return None

    @staticmethod
    def list_traces(
        tenant_id: str,
        agent_id: Optional[str] = None,
        user_id: Optional[str] = None,
        status: Optional[str] = None,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
        page: int = 1,
        page_size: int = 20,
    ) -> dict[str, Any]:
        """
        List trace sessions with filtering and pagination.
        
        Args:
            tenant_id: ID of the tenant
            agent_id: Filter by agent ID
            user_id: Filter by user ID
            status: Filter by status (running, completed, failed)
            start_time: Filter by start time
            end_time: Filter by end time
            page: Page number
            page_size: Items per page
            
        Returns:
            Paginated list of trace sessions
        """
        try:
            pattern = f"{TRACE_KEY_PREFIX}session:*"
            keys = REDIS_CONN.keys(pattern)
            
            sessions = []
            for key in keys:
                data = REDIS_CONN.get(key)
                if data:
                    session = json.loads(data)
                    if session.get("tenant_id") == tenant_id:
                        sessions.append(session)
            
            if agent_id:
                sessions = [s for s in sessions if s.get("agent_id") == agent_id]
            if user_id:
                sessions = [s for s in sessions if s.get("user_id") == user_id]
            if status:
                sessions = [s for s in sessions if s.get("status") == status]
            
            if start_time:
                sessions = [s for s in sessions 
                           if datetime.fromisoformat(s.get("created_at", "")) >= start_time]
            if end_time:
                sessions = [s for s in sessions 
                           if datetime.fromisoformat(s.get("created_at", "")) <= end_time]
            
            sessions.sort(key=lambda x: x.get("created_at", ""), reverse=True)
            
            total = len(sessions)
            start_idx = (page - 1) * page_size
            end_idx = start_idx + page_size
            paginated = sessions[start_idx:end_idx]
            
            return {
                "total": total,
                "page": page,
                "page_size": page_size,
                "sessions": paginated,
            }
        except Exception as e:
            logging.exception(f"Failed to list traces: {e}")
            return {"total": 0, "page": page, "page_size": page_size, "sessions": []}

    @staticmethod
    def delete_trace(task_id: str) -> Tuple[bool, str]:
        """
        Delete a trace session.
        
        Args:
            task_id: ID of the task/trace session
            
        Returns:
            Tuple of (success, message)
        """
        try:
            session_key = f"{TRACE_KEY_PREFIX}session:{task_id}"
            events_key = f"{TRACE_KEY_PREFIX}events:{task_id}"
            
            REDIS_CONN.delete(session_key)
            REDIS_CONN.delete(events_key)
            
            return True, "Trace deleted successfully"
        except Exception as e:
            logging.exception(f"Failed to delete trace: {e}")
            return False, str(e)

    @staticmethod
    def analyze_trace(task_id: str) -> Optional[dict[str, Any]]:
        """
        Analyze a trace session and provide insights.
        
        Args:
            task_id: ID of the task/trace session
            
        Returns:
            Analysis results or None if not found
        """
        try:
            session_data = TraceService.get_trace_session(task_id)
            if not session_data:
                return None
            
            events = TraceService.get_trace_events(task_id, limit=1000)
            
            analysis = {
                "task_id": task_id,
                "total_events": len(events),
                "event_distribution": defaultdict(int),
                "component_execution_times": {},
                "bottlenecks": [],
                "errors": [],
                "recommendations": [],
            }
            
            for event in events:
                event_type = event.get("event_type", "unknown")
                analysis["event_distribution"][event_type] += 1
                
                if event.get("error"):
                    analysis["errors"].append({
                        "component": event.get("component_name"),
                        "error": event.get("error"),
                        "timestamp": event.get("timestamp"),
                    })
                
                if event.get("elapsed_time"):
                    component = event.get("component_name", "unknown")
                    if component not in analysis["component_execution_times"]:
                        analysis["component_execution_times"][component] = []
                    analysis["component_execution_times"][component].append(
                        event.get("elapsed_time")
                    )
            
            for component, times in analysis["component_execution_times"].items():
                avg_time = sum(times) / len(times)
                if avg_time > 5.0:
                    analysis["bottlenecks"].append({
                        "component": component,
                        "avg_execution_time": round(avg_time, 2),
                        "executions": len(times),
                    })
            
            if analysis["bottlenecks"]:
                analysis["recommendations"].append(
                    "Consider optimizing slow components or adding caching"
                )
            if len(analysis["errors"]) > 0:
                analysis["recommendations"].append(
                    "Review and fix error-prone components"
                )
            
            analysis["event_distribution"] = dict(analysis["event_distribution"])
            
            return analysis
        except Exception as e:
            logging.exception(f"Failed to analyze trace: {e}")
            return None

    @staticmethod
    def cleanup_old_traces(days: int = 7) -> Tuple[int, str]:
        """
        Clean up trace sessions older than specified days.
        
        Args:
            days: Number of days to keep traces
            
        Returns:
            Tuple of (deleted count, message)
        """
        try:
            cutoff = datetime.utcnow() - timedelta(days=days)
            pattern = f"{TRACE_KEY_PREFIX}session:*"
            keys = REDIS_CONN.keys(pattern)
            
            deleted = 0
            for key in keys:
                data = REDIS_CONN.get(key)
                if data:
                    session = json.loads(data)
                    created_at = session.get("created_at")
                    if created_at:
                        session_time = datetime.fromisoformat(created_at)
                        if session_time < cutoff:
                            task_id = key.split(":")[-1]
                            TraceService.delete_trace(task_id)
                            deleted += 1
            
            return deleted, f"Deleted {deleted} old trace sessions"
        except Exception as e:
            logging.exception(f"Failed to cleanup traces: {e}")
            return 0, str(e)

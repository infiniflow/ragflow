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
Trace Collector for Agent Execution

This module provides the TraceCollector class that captures and manages
trace events during agent workflow execution. It supports real-time
event streaming and session management.
"""

import asyncio
import logging
import threading
import time
import uuid
from contextlib import contextmanager
from datetime import datetime
from typing import Any, Callable, Optional, Generator, AsyncGenerator

from agent.trace.trace_models import (
    TraceEventType,
    TraceLevel,
    TraceMetadata,
    TraceEvent,
    TraceSession,
    LLMCallTrace,
    RetrievalTrace,
    ToolCallTrace,
    ComponentInfo,
)


_trace_collectors: dict[str, "TraceCollector"] = {}
_collectors_lock = threading.Lock()


def get_trace_collector(task_id: str) -> Optional["TraceCollector"]:
    """Get an existing trace collector by task ID."""
    with _collectors_lock:
        return _trace_collectors.get(task_id)


def create_trace_collector(
    task_id: str,
    agent_id: str,
    session_id: str,
    user_id: str,
    tenant_id: str,
    trace_level: TraceLevel = TraceLevel.STANDARD,
) -> "TraceCollector":
    """Create a new trace collector for an agent execution."""
    with _collectors_lock:
        if task_id in _trace_collectors:
            return _trace_collectors[task_id]
        
        collector = TraceCollector(
            task_id=task_id,
            agent_id=agent_id,
            session_id=session_id,
            user_id=user_id,
            tenant_id=tenant_id,
            trace_level=trace_level,
        )
        _trace_collectors[task_id] = collector
        return collector


def remove_trace_collector(task_id: str) -> None:
    """Remove a trace collector from the registry."""
    with _collectors_lock:
        if task_id in _trace_collectors:
            del _trace_collectors[task_id]


class TraceCollector:
    """
    Collects and manages trace events during agent execution.
    
    This class provides methods to record various types of trace events,
    manage trace sessions, and stream events in real-time.
    """

    def __init__(
        self,
        task_id: str,
        agent_id: str,
        session_id: str,
        user_id: str,
        tenant_id: str,
        trace_level: TraceLevel = TraceLevel.STANDARD,
    ):
        """Initialize the trace collector."""
        self.task_id = task_id
        self.trace_level = trace_level
        self._lock = threading.Lock()
        self._event_queue: asyncio.Queue[TraceEvent] = asyncio.Queue()
        self._subscribers: list[Callable[[TraceEvent], None]] = []
        self._is_active = True
        
        metadata = TraceMetadata(
            agent_id=agent_id,
            session_id=session_id,
            user_id=user_id,
            tenant_id=tenant_id,
            trace_level=trace_level,
        )
        
        self.session = TraceSession(
            session_id=session_id,
            metadata=metadata,
        )
        
        self._component_start_times: dict[str, float] = {}
        self._llm_call_starts: dict[str, tuple[float, LLMCallTrace]] = {}
        self._retrieval_starts: dict[str, tuple[float, RetrievalTrace]] = {}
        self._tool_call_starts: dict[str, tuple[float, ToolCallTrace]] = {}

    def _should_trace(self, required_level: TraceLevel) -> bool:
        """Check if the current trace level allows this event."""
        level_order = [TraceLevel.MINIMAL, TraceLevel.STANDARD, TraceLevel.DETAILED, TraceLevel.DEBUG]
        current_idx = level_order.index(self.trace_level)
        required_idx = level_order.index(required_level)
        return current_idx >= required_idx

    def _create_event(
        self,
        event_type: TraceEventType,
        component_id: Optional[str] = None,
        component_name: Optional[str] = None,
        component_type: Optional[str] = None,
        inputs: Optional[dict[str, Any]] = None,
        outputs: Optional[dict[str, Any]] = None,
        error: Optional[str] = None,
        elapsed_time: Optional[float] = None,
        thoughts: Optional[str] = None,
        metadata: Optional[dict[str, Any]] = None,
    ) -> TraceEvent:
        """Create a new trace event."""
        return TraceEvent(
            event_id=str(uuid.uuid4()),
            event_type=event_type,
            timestamp=datetime.utcnow(),
            component_id=component_id,
            component_name=component_name,
            component_type=component_type,
            inputs=inputs,
            outputs=outputs,
            error=error,
            elapsed_time=elapsed_time,
            thoughts=thoughts,
            metadata=metadata or {},
        )

    def _record_event(self, event: TraceEvent) -> None:
        """Record an event and notify subscribers."""
        with self._lock:
            self.session.add_event(event)
            for subscriber in self._subscribers:
                try:
                    subscriber(event)
                except Exception as e:
                    logging.warning(f"Trace subscriber error: {e}")

    def subscribe(self, callback: Callable[[TraceEvent], None]) -> None:
        """Subscribe to trace events."""
        with self._lock:
            self._subscribers.append(callback)

    def unsubscribe(self, callback: Callable[[TraceEvent], None]) -> None:
        """Unsubscribe from trace events."""
        with self._lock:
            if callback in self._subscribers:
                self._subscribers.remove(callback)

    def workflow_started(self, inputs: Optional[dict[str, Any]] = None) -> None:
        """Record workflow start event."""
        event = self._create_event(
            event_type=TraceEventType.WORKFLOW_STARTED,
            inputs=inputs,
        )
        self._record_event(event)

    def workflow_completed(self, outputs: Optional[dict[str, Any]] = None) -> None:
        """Record workflow completion event."""
        event = self._create_event(
            event_type=TraceEventType.WORKFLOW_COMPLETED,
            outputs=outputs,
            elapsed_time=(datetime.utcnow() - self.session.started_at).total_seconds(),
        )
        self._record_event(event)
        self.session.complete()

    def workflow_failed(self, error: str) -> None:
        """Record workflow failure event."""
        event = self._create_event(
            event_type=TraceEventType.WORKFLOW_FAILED,
            error=error,
            elapsed_time=(datetime.utcnow() - self.session.started_at).total_seconds(),
        )
        self._record_event(event)
        self.session.complete(error=error)

    def node_started(
        self,
        component_id: str,
        component_name: str,
        component_type: str,
        inputs: Optional[dict[str, Any]] = None,
        thoughts: Optional[str] = None,
    ) -> None:
        """Record node start event."""
        self._component_start_times[component_id] = time.perf_counter()
        
        event = self._create_event(
            event_type=TraceEventType.NODE_STARTED,
            component_id=component_id,
            component_name=component_name,
            component_type=component_type,
            inputs=inputs,
            thoughts=thoughts,
        )
        self._record_event(event)

    def node_finished(
        self,
        component_id: str,
        component_name: str,
        component_type: str,
        inputs: Optional[dict[str, Any]] = None,
        outputs: Optional[dict[str, Any]] = None,
        error: Optional[str] = None,
    ) -> None:
        """Record node completion event."""
        start_time = self._component_start_times.pop(component_id, None)
        elapsed_time = time.perf_counter() - start_time if start_time else None
        
        event_type = TraceEventType.NODE_FAILED if error else TraceEventType.NODE_FINISHED
        
        event = self._create_event(
            event_type=event_type,
            component_id=component_id,
            component_name=component_name,
            component_type=component_type,
            inputs=inputs,
            outputs=outputs,
            error=error,
            elapsed_time=elapsed_time,
        )
        self._record_event(event)

    def llm_call_started(
        self,
        call_id: str,
        model_name: str,
        prompt: str,
        temperature: float = 0.0,
        max_tokens: Optional[int] = None,
    ) -> None:
        """Record LLM call start."""
        if not self._should_trace(TraceLevel.DETAILED):
            return
            
        llm_trace = LLMCallTrace(
            call_id=call_id,
            model_name=model_name,
            prompt=prompt,
            temperature=temperature,
            max_tokens=max_tokens,
        )
        self._llm_call_starts[call_id] = (time.perf_counter(), llm_trace)
        
        event = self._create_event(
            event_type=TraceEventType.LLM_CALL_STARTED,
            metadata={"call_id": call_id, "model_name": model_name},
        )
        self._record_event(event)

    def llm_call_completed(
        self,
        call_id: str,
        response: str,
        prompt_tokens: int = 0,
        completion_tokens: int = 0,
        error: Optional[str] = None,
    ) -> None:
        """Record LLM call completion."""
        if not self._should_trace(TraceLevel.DETAILED):
            return
            
        start_data = self._llm_call_starts.pop(call_id, None)
        if start_data:
            start_time, llm_trace = start_data
            llm_trace.response = response
            llm_trace.prompt_tokens = prompt_tokens
            llm_trace.completion_tokens = completion_tokens
            llm_trace.total_tokens = prompt_tokens + completion_tokens
            llm_trace.latency_ms = (time.perf_counter() - start_time) * 1000
            llm_trace.completed_at = datetime.utcnow()
            llm_trace.error = error
            self.session.add_llm_call(llm_trace)
        
        event = self._create_event(
            event_type=TraceEventType.LLM_CALL_COMPLETED,
            error=error,
            metadata={"call_id": call_id, "tokens": prompt_tokens + completion_tokens},
        )
        self._record_event(event)

    def retrieval_started(
        self,
        retrieval_id: str,
        query: str,
        knowledge_bases: list[str],
        top_k: int = 10,
        similarity_threshold: float = 0.0,
        rerank_enabled: bool = False,
    ) -> None:
        """Record retrieval operation start."""
        retrieval_trace = RetrievalTrace(
            retrieval_id=retrieval_id,
            query=query,
            knowledge_bases=knowledge_bases,
            top_k=top_k,
            similarity_threshold=similarity_threshold,
            rerank_enabled=rerank_enabled,
        )
        self._retrieval_starts[retrieval_id] = (time.perf_counter(), retrieval_trace)
        
        event = self._create_event(
            event_type=TraceEventType.RETRIEVAL_STARTED,
            metadata={"retrieval_id": retrieval_id, "query": query[:100]},
        )
        self._record_event(event)

    def retrieval_completed(
        self,
        retrieval_id: str,
        chunks: list[dict[str, Any]],
        error: Optional[str] = None,
    ) -> None:
        """Record retrieval operation completion."""
        start_data = self._retrieval_starts.pop(retrieval_id, None)
        if start_data:
            start_time, retrieval_trace = start_data
            retrieval_trace.chunks = chunks
            retrieval_trace.chunks_retrieved = len(chunks)
            retrieval_trace.latency_ms = (time.perf_counter() - start_time) * 1000
            retrieval_trace.completed_at = datetime.utcnow()
            retrieval_trace.error = error
            self.session.add_retrieval(retrieval_trace)
        
        event = self._create_event(
            event_type=TraceEventType.RETRIEVAL_COMPLETED,
            error=error,
            metadata={"retrieval_id": retrieval_id, "chunks_count": len(chunks)},
        )
        self._record_event(event)

    def tool_call_started(
        self,
        call_id: str,
        tool_name: str,
        tool_type: str,
        arguments: dict[str, Any],
    ) -> None:
        """Record tool call start."""
        tool_trace = ToolCallTrace(
            call_id=call_id,
            tool_name=tool_name,
            tool_type=tool_type,
            arguments=arguments,
        )
        self._tool_call_starts[call_id] = (time.perf_counter(), tool_trace)
        
        event = self._create_event(
            event_type=TraceEventType.TOOL_CALL_STARTED,
            metadata={"call_id": call_id, "tool_name": tool_name},
        )
        self._record_event(event)

    def tool_call_completed(
        self,
        call_id: str,
        result: Any,
        error: Optional[str] = None,
    ) -> None:
        """Record tool call completion."""
        start_data = self._tool_call_starts.pop(call_id, None)
        if start_data:
            start_time, tool_trace = start_data
            tool_trace.result = result
            tool_trace.latency_ms = (time.perf_counter() - start_time) * 1000
            tool_trace.completed_at = datetime.utcnow()
            tool_trace.error = error
            self.session.add_tool_call(tool_trace)
        
        event = self._create_event(
            event_type=TraceEventType.TOOL_CALL_COMPLETED,
            error=error,
            metadata={"call_id": call_id},
        )
        self._record_event(event)

    def message_generated(
        self,
        content: str,
        component_id: Optional[str] = None,
        component_name: Optional[str] = None,
    ) -> None:
        """Record message generation event."""
        event = self._create_event(
            event_type=TraceEventType.MESSAGE_GENERATED,
            component_id=component_id,
            component_name=component_name,
            outputs={"content": content[:500] if len(content) > 500 else content},
        )
        self._record_event(event)

    def thinking_started(self, component_id: str, thoughts: str) -> None:
        """Record thinking/reasoning start."""
        if not self._should_trace(TraceLevel.DETAILED):
            return
            
        event = self._create_event(
            event_type=TraceEventType.THINKING_STARTED,
            component_id=component_id,
            thoughts=thoughts,
        )
        self._record_event(event)

    def thinking_completed(self, component_id: str, thoughts: str) -> None:
        """Record thinking/reasoning completion."""
        if not self._should_trace(TraceLevel.DETAILED):
            return
            
        event = self._create_event(
            event_type=TraceEventType.THINKING_COMPLETED,
            component_id=component_id,
            thoughts=thoughts,
        )
        self._record_event(event)

    def error_occurred(
        self,
        error: str,
        component_id: Optional[str] = None,
        component_name: Optional[str] = None,
    ) -> None:
        """Record an error event."""
        event = self._create_event(
            event_type=TraceEventType.ERROR_OCCURRED,
            component_id=component_id,
            component_name=component_name,
            error=error,
        )
        self._record_event(event)

    def get_session(self) -> TraceSession:
        """Get the current trace session."""
        return self.session

    def get_events(self) -> list[TraceEvent]:
        """Get all recorded events."""
        return self.session.events

    def get_summary(self) -> dict[str, Any]:
        """Get a summary of the trace session."""
        return self.session.get_summary()

    def close(self) -> None:
        """Close the trace collector and cleanup resources."""
        self._is_active = False
        remove_trace_collector(self.task_id)

    @contextmanager
    def trace_component(
        self,
        component_id: str,
        component_name: str,
        component_type: str,
        inputs: Optional[dict[str, Any]] = None,
    ) -> Generator[None, None, None]:
        """Context manager for tracing component execution."""
        self.node_started(component_id, component_name, component_type, inputs)
        error = None
        outputs = None
        try:
            yield
        except Exception as e:
            error = str(e)
            raise
        finally:
            self.node_finished(
                component_id, component_name, component_type,
                inputs=inputs, outputs=outputs, error=error
            )

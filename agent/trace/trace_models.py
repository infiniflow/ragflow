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
Trace Models for Agent Execution Logging

This module provides data models for capturing and representing trace information
during agent execution. It includes models for individual trace events, component
execution details, and complete trace sessions.
"""

from dataclasses import dataclass, field, asdict
from datetime import datetime
from enum import Enum
from typing import Any, Optional, Union
import json
import uuid


class TraceEventType(Enum):
    """Enumeration of trace event types."""
    WORKFLOW_STARTED = "workflow_started"
    WORKFLOW_COMPLETED = "workflow_completed"
    WORKFLOW_FAILED = "workflow_failed"
    NODE_STARTED = "node_started"
    NODE_FINISHED = "node_finished"
    NODE_FAILED = "node_failed"
    RETRIEVAL_STARTED = "retrieval_started"
    RETRIEVAL_COMPLETED = "retrieval_completed"
    LLM_CALL_STARTED = "llm_call_started"
    LLM_CALL_COMPLETED = "llm_call_completed"
    TOOL_CALL_STARTED = "tool_call_started"
    TOOL_CALL_COMPLETED = "tool_call_completed"
    MESSAGE_GENERATED = "message_generated"
    ERROR_OCCURRED = "error_occurred"
    THINKING_STARTED = "thinking_started"
    THINKING_COMPLETED = "thinking_completed"


class TraceLevel(Enum):
    """Trace verbosity levels."""
    MINIMAL = "minimal"
    STANDARD = "standard"
    DETAILED = "detailed"
    DEBUG = "debug"


@dataclass
class TraceMetadata:
    """Metadata associated with a trace session."""
    agent_id: str
    session_id: str
    user_id: str
    tenant_id: str
    trace_level: TraceLevel = TraceLevel.STANDARD
    created_at: datetime = field(default_factory=datetime.utcnow)
    tags: list[str] = field(default_factory=list)
    custom_data: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert metadata to dictionary representation."""
        return {
            "agent_id": self.agent_id,
            "session_id": self.session_id,
            "user_id": self.user_id,
            "tenant_id": self.tenant_id,
            "trace_level": self.trace_level.value,
            "created_at": self.created_at.isoformat(),
            "tags": self.tags,
            "custom_data": self.custom_data
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "TraceMetadata":
        """Create TraceMetadata from dictionary."""
        return cls(
            agent_id=data.get("agent_id", ""),
            session_id=data.get("session_id", ""),
            user_id=data.get("user_id", ""),
            tenant_id=data.get("tenant_id", ""),
            trace_level=TraceLevel(data.get("trace_level", "standard")),
            created_at=datetime.fromisoformat(data["created_at"]) if "created_at" in data else datetime.utcnow(),
            tags=data.get("tags", []),
            custom_data=data.get("custom_data", {})
        )


@dataclass
class ComponentInfo:
    """Information about a component in the agent workflow."""
    component_id: str
    component_name: str
    component_type: str
    params: dict[str, Any] = field(default_factory=dict)
    upstream: list[str] = field(default_factory=list)
    downstream: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        """Convert component info to dictionary."""
        return {
            "component_id": self.component_id,
            "component_name": self.component_name,
            "component_type": self.component_type,
            "params": self.params,
            "upstream": self.upstream,
            "downstream": self.downstream
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "ComponentInfo":
        """Create ComponentInfo from dictionary."""
        return cls(
            component_id=data.get("component_id", ""),
            component_name=data.get("component_name", ""),
            component_type=data.get("component_type", ""),
            params=data.get("params", {}),
            upstream=data.get("upstream", []),
            downstream=data.get("downstream", [])
        )


@dataclass
class TraceEvent:
    """Represents a single trace event during agent execution."""
    event_id: str
    event_type: TraceEventType
    timestamp: datetime
    component_id: Optional[str] = None
    component_name: Optional[str] = None
    component_type: Optional[str] = None
    inputs: Optional[dict[str, Any]] = None
    outputs: Optional[dict[str, Any]] = None
    error: Optional[str] = None
    elapsed_time: Optional[float] = None
    thoughts: Optional[str] = None
    metadata: dict[str, Any] = field(default_factory=dict)

    def __post_init__(self):
        """Initialize event_id if not provided."""
        if not self.event_id:
            self.event_id = str(uuid.uuid4())

    def to_dict(self) -> dict[str, Any]:
        """Convert trace event to dictionary representation."""
        result = {
            "event_id": self.event_id,
            "event_type": self.event_type.value,
            "timestamp": self.timestamp.isoformat(),
        }
        if self.component_id:
            result["component_id"] = self.component_id
        if self.component_name:
            result["component_name"] = self.component_name
        if self.component_type:
            result["component_type"] = self.component_type
        if self.inputs is not None:
            result["inputs"] = self.inputs
        if self.outputs is not None:
            result["outputs"] = self.outputs
        if self.error:
            result["error"] = self.error
        if self.elapsed_time is not None:
            result["elapsed_time"] = self.elapsed_time
        if self.thoughts:
            result["thoughts"] = self.thoughts
        if self.metadata:
            result["metadata"] = self.metadata
        return result

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "TraceEvent":
        """Create TraceEvent from dictionary."""
        return cls(
            event_id=data.get("event_id", ""),
            event_type=TraceEventType(data.get("event_type", "node_started")),
            timestamp=datetime.fromisoformat(data["timestamp"]) if "timestamp" in data else datetime.utcnow(),
            component_id=data.get("component_id"),
            component_name=data.get("component_name"),
            component_type=data.get("component_type"),
            inputs=data.get("inputs"),
            outputs=data.get("outputs"),
            error=data.get("error"),
            elapsed_time=data.get("elapsed_time"),
            thoughts=data.get("thoughts"),
            metadata=data.get("metadata", {})
        )

    def to_json(self) -> str:
        """Convert trace event to JSON string."""
        return json.dumps(self.to_dict(), ensure_ascii=False)


@dataclass
class LLMCallTrace:
    """Trace information for LLM API calls."""
    call_id: str
    model_name: str
    prompt: str
    response: Optional[str] = None
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0
    latency_ms: float = 0.0
    temperature: float = 0.0
    max_tokens: Optional[int] = None
    error: Optional[str] = None
    started_at: datetime = field(default_factory=datetime.utcnow)
    completed_at: Optional[datetime] = None

    def to_dict(self) -> dict[str, Any]:
        """Convert LLM call trace to dictionary."""
        return {
            "call_id": self.call_id,
            "model_name": self.model_name,
            "prompt": self.prompt[:500] + "..." if len(self.prompt) > 500 else self.prompt,
            "response": self.response[:500] + "..." if self.response and len(self.response) > 500 else self.response,
            "prompt_tokens": self.prompt_tokens,
            "completion_tokens": self.completion_tokens,
            "total_tokens": self.total_tokens,
            "latency_ms": self.latency_ms,
            "temperature": self.temperature,
            "max_tokens": self.max_tokens,
            "error": self.error,
            "started_at": self.started_at.isoformat(),
            "completed_at": self.completed_at.isoformat() if self.completed_at else None
        }


@dataclass
class RetrievalTrace:
    """Trace information for retrieval operations."""
    retrieval_id: str
    query: str
    knowledge_bases: list[str] = field(default_factory=list)
    top_k: int = 10
    similarity_threshold: float = 0.0
    chunks_retrieved: int = 0
    chunks: list[dict[str, Any]] = field(default_factory=list)
    latency_ms: float = 0.0
    rerank_enabled: bool = False
    error: Optional[str] = None
    started_at: datetime = field(default_factory=datetime.utcnow)
    completed_at: Optional[datetime] = None

    def to_dict(self) -> dict[str, Any]:
        """Convert retrieval trace to dictionary."""
        return {
            "retrieval_id": self.retrieval_id,
            "query": self.query,
            "knowledge_bases": self.knowledge_bases,
            "top_k": self.top_k,
            "similarity_threshold": self.similarity_threshold,
            "chunks_retrieved": self.chunks_retrieved,
            "chunks": self.chunks[:5],
            "latency_ms": self.latency_ms,
            "rerank_enabled": self.rerank_enabled,
            "error": self.error,
            "started_at": self.started_at.isoformat(),
            "completed_at": self.completed_at.isoformat() if self.completed_at else None
        }


@dataclass
class ToolCallTrace:
    """Trace information for tool/function calls."""
    call_id: str
    tool_name: str
    tool_type: str
    arguments: dict[str, Any] = field(default_factory=dict)
    result: Optional[Any] = None
    latency_ms: float = 0.0
    error: Optional[str] = None
    started_at: datetime = field(default_factory=datetime.utcnow)
    completed_at: Optional[datetime] = None

    def to_dict(self) -> dict[str, Any]:
        """Convert tool call trace to dictionary."""
        result_str = str(self.result)[:500] if self.result else None
        return {
            "call_id": self.call_id,
            "tool_name": self.tool_name,
            "tool_type": self.tool_type,
            "arguments": self.arguments,
            "result": result_str,
            "latency_ms": self.latency_ms,
            "error": self.error,
            "started_at": self.started_at.isoformat(),
            "completed_at": self.completed_at.isoformat() if self.completed_at else None
        }


@dataclass
class TraceSession:
    """Complete trace session for an agent execution."""
    session_id: str
    metadata: TraceMetadata
    events: list[TraceEvent] = field(default_factory=list)
    llm_calls: list[LLMCallTrace] = field(default_factory=list)
    retrievals: list[RetrievalTrace] = field(default_factory=list)
    tool_calls: list[ToolCallTrace] = field(default_factory=list)
    started_at: datetime = field(default_factory=datetime.utcnow)
    completed_at: Optional[datetime] = None
    total_elapsed_time: float = 0.0
    status: str = "running"
    error: Optional[str] = None

    def add_event(self, event: TraceEvent) -> None:
        """Add a trace event to the session."""
        self.events.append(event)

    def add_llm_call(self, llm_call: LLMCallTrace) -> None:
        """Add an LLM call trace to the session."""
        self.llm_calls.append(llm_call)

    def add_retrieval(self, retrieval: RetrievalTrace) -> None:
        """Add a retrieval trace to the session."""
        self.retrievals.append(retrieval)

    def add_tool_call(self, tool_call: ToolCallTrace) -> None:
        """Add a tool call trace to the session."""
        self.tool_calls.append(tool_call)

    def complete(self, error: Optional[str] = None) -> None:
        """Mark the session as completed."""
        self.completed_at = datetime.utcnow()
        self.total_elapsed_time = (self.completed_at - self.started_at).total_seconds()
        self.status = "failed" if error else "completed"
        self.error = error

    def to_dict(self) -> dict[str, Any]:
        """Convert trace session to dictionary representation."""
        return {
            "session_id": self.session_id,
            "metadata": self.metadata.to_dict(),
            "events": [e.to_dict() for e in self.events],
            "llm_calls": [c.to_dict() for c in self.llm_calls],
            "retrievals": [r.to_dict() for r in self.retrievals],
            "tool_calls": [t.to_dict() for t in self.tool_calls],
            "started_at": self.started_at.isoformat(),
            "completed_at": self.completed_at.isoformat() if self.completed_at else None,
            "total_elapsed_time": self.total_elapsed_time,
            "status": self.status,
            "error": self.error,
            "summary": self.get_summary()
        }

    def get_summary(self) -> dict[str, Any]:
        """Get a summary of the trace session."""
        return {
            "total_events": len(self.events),
            "total_llm_calls": len(self.llm_calls),
            "total_retrievals": len(self.retrievals),
            "total_tool_calls": len(self.tool_calls),
            "total_tokens": sum(c.total_tokens for c in self.llm_calls),
            "total_chunks_retrieved": sum(r.chunks_retrieved for r in self.retrievals),
            "nodes_executed": len(set(e.component_id for e in self.events if e.component_id)),
            "errors_count": len([e for e in self.events if e.error])
        }

    def to_json(self) -> str:
        """Convert trace session to JSON string."""
        return json.dumps(self.to_dict(), ensure_ascii=False, indent=2)

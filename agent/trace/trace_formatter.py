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
Trace Formatter for Agent Execution Logs

This module provides various formatters for converting trace events and sessions
into different output formats suitable for API responses, logging, and debugging.
"""

import json
from abc import ABC, abstractmethod
from datetime import datetime
from typing import Any, Optional, Generator

from agent.trace.trace_models import (
    TraceEventType,
    TraceLevel,
    TraceEvent,
    TraceSession,
    LLMCallTrace,
    RetrievalTrace,
    ToolCallTrace,
)


class TraceFormatter(ABC):
    """Abstract base class for trace formatters."""

    @abstractmethod
    def format_event(self, event: TraceEvent) -> dict[str, Any]:
        """Format a single trace event."""
        pass

    @abstractmethod
    def format_session(self, session: TraceSession) -> dict[str, Any]:
        """Format a complete trace session."""
        pass

    @abstractmethod
    def format_for_stream(self, event: TraceEvent) -> str:
        """Format an event for SSE streaming."""
        pass


class StreamingTraceFormatter(TraceFormatter):
    """Formatter optimized for real-time SSE streaming."""

    def __init__(self, include_inputs: bool = True, include_outputs: bool = True):
        """Initialize the streaming formatter."""
        self.include_inputs = include_inputs
        self.include_outputs = include_outputs

    def format_event(self, event: TraceEvent) -> dict[str, Any]:
        """Format a trace event for streaming."""
        result = {
            "event_id": event.event_id,
            "event_type": event.event_type.value,
            "timestamp": event.timestamp.isoformat(),
        }
        
        if event.component_id:
            result["component_id"] = event.component_id
        if event.component_name:
            result["component_name"] = event.component_name
        if event.component_type:
            result["component_type"] = event.component_type
        
        if self.include_inputs and event.inputs:
            result["inputs"] = self._truncate_dict(event.inputs, max_length=200)
        
        if self.include_outputs and event.outputs:
            result["outputs"] = self._truncate_dict(event.outputs, max_length=500)
        
        if event.error:
            result["error"] = event.error
        if event.elapsed_time is not None:
            result["elapsed_time"] = round(event.elapsed_time, 3)
        if event.thoughts:
            result["thoughts"] = event.thoughts[:300] if len(event.thoughts) > 300 else event.thoughts
        
        return result

    def format_session(self, session: TraceSession) -> dict[str, Any]:
        """Format a trace session for streaming response."""
        return {
            "session_id": session.session_id,
            "status": session.status,
            "started_at": session.started_at.isoformat(),
            "completed_at": session.completed_at.isoformat() if session.completed_at else None,
            "total_elapsed_time": round(session.total_elapsed_time, 3),
            "summary": session.get_summary(),
            "events": [self.format_event(e) for e in session.events],
        }

    def format_for_stream(self, event: TraceEvent) -> str:
        """Format an event as SSE data."""
        data = self.format_event(event)
        return f"data:{json.dumps({'event': 'trace', 'data': data}, ensure_ascii=False)}\n\n"

    def _truncate_dict(self, d: dict[str, Any], max_length: int = 200) -> dict[str, Any]:
        """Truncate string values in a dictionary."""
        result = {}
        for key, value in d.items():
            if isinstance(value, str) and len(value) > max_length:
                result[key] = value[:max_length] + "..."
            elif isinstance(value, dict):
                result[key] = self._truncate_dict(value, max_length)
            elif isinstance(value, list) and len(value) > 10:
                result[key] = value[:10] + ["..."]
            else:
                result[key] = value
        return result


class CompactTraceFormatter(TraceFormatter):
    """Compact formatter for minimal trace output."""

    def __init__(self):
        """Initialize the compact formatter."""
        self._event_icons = {
            TraceEventType.WORKFLOW_STARTED: "🚀",
            TraceEventType.WORKFLOW_COMPLETED: "✅",
            TraceEventType.WORKFLOW_FAILED: "❌",
            TraceEventType.NODE_STARTED: "▶️",
            TraceEventType.NODE_FINISHED: "✔️",
            TraceEventType.NODE_FAILED: "❌",
            TraceEventType.RETRIEVAL_STARTED: "🔍",
            TraceEventType.RETRIEVAL_COMPLETED: "📚",
            TraceEventType.LLM_CALL_STARTED: "🤖",
            TraceEventType.LLM_CALL_COMPLETED: "💬",
            TraceEventType.TOOL_CALL_STARTED: "🔧",
            TraceEventType.TOOL_CALL_COMPLETED: "⚙️",
            TraceEventType.MESSAGE_GENERATED: "📝",
            TraceEventType.ERROR_OCCURRED: "⚠️",
            TraceEventType.THINKING_STARTED: "💭",
            TraceEventType.THINKING_COMPLETED: "💡",
        }

    def format_event(self, event: TraceEvent) -> dict[str, Any]:
        """Format a trace event in compact form."""
        icon = self._event_icons.get(event.event_type, "•")
        
        result = {
            "type": event.event_type.value,
            "icon": icon,
            "time": event.timestamp.strftime("%H:%M:%S.%f")[:-3],
        }
        
        if event.component_name:
            result["component"] = event.component_name
        if event.elapsed_time is not None:
            result["duration_ms"] = round(event.elapsed_time * 1000, 1)
        if event.error:
            result["error"] = event.error[:100]
        
        return result

    def format_session(self, session: TraceSession) -> dict[str, Any]:
        """Format a trace session in compact form."""
        summary = session.get_summary()
        
        return {
            "session_id": session.session_id,
            "status": session.status,
            "duration_s": round(session.total_elapsed_time, 2),
            "nodes": summary["nodes_executed"],
            "llm_calls": summary["total_llm_calls"],
            "retrievals": summary["total_retrievals"],
            "tool_calls": summary["total_tool_calls"],
            "tokens": summary["total_tokens"],
            "errors": summary["errors_count"],
            "timeline": [self.format_event(e) for e in session.events],
        }

    def format_for_stream(self, event: TraceEvent) -> str:
        """Format an event for SSE streaming in compact form."""
        data = self.format_event(event)
        return f"data:{json.dumps({'event': 'trace_compact', 'data': data}, ensure_ascii=False)}\n\n"

    def format_timeline(self, session: TraceSession) -> list[str]:
        """Format session as a text timeline."""
        lines = []
        for event in session.events:
            icon = self._event_icons.get(event.event_type, "•")
            time_str = event.timestamp.strftime("%H:%M:%S")
            component = event.component_name or ""
            duration = f" ({event.elapsed_time*1000:.0f}ms)" if event.elapsed_time else ""
            
            line = f"{time_str} {icon} {event.event_type.value}"
            if component:
                line += f" [{component}]"
            line += duration
            if event.error:
                line += f" ERROR: {event.error[:50]}"
            
            lines.append(line)
        
        return lines


class DetailedTraceFormatter(TraceFormatter):
    """Detailed formatter for comprehensive trace output."""

    def __init__(self, include_raw_data: bool = False):
        """Initialize the detailed formatter."""
        self.include_raw_data = include_raw_data

    def format_event(self, event: TraceEvent) -> dict[str, Any]:
        """Format a trace event with full details."""
        result = {
            "event_id": event.event_id,
            "event_type": event.event_type.value,
            "timestamp": event.timestamp.isoformat(),
            "timestamp_unix": event.timestamp.timestamp(),
        }
        
        if event.component_id:
            result["component"] = {
                "id": event.component_id,
                "name": event.component_name,
                "type": event.component_type,
            }
        
        if event.inputs is not None:
            result["inputs"] = event.inputs
        if event.outputs is not None:
            result["outputs"] = event.outputs
        if event.error:
            result["error"] = {
                "message": event.error,
                "occurred_at": event.timestamp.isoformat(),
            }
        if event.elapsed_time is not None:
            result["timing"] = {
                "elapsed_seconds": round(event.elapsed_time, 4),
                "elapsed_ms": round(event.elapsed_time * 1000, 2),
            }
        if event.thoughts:
            result["thoughts"] = event.thoughts
        if event.metadata:
            result["metadata"] = event.metadata
        
        return result

    def format_session(self, session: TraceSession) -> dict[str, Any]:
        """Format a complete trace session with all details."""
        result = {
            "session_id": session.session_id,
            "metadata": session.metadata.to_dict(),
            "status": session.status,
            "timing": {
                "started_at": session.started_at.isoformat(),
                "completed_at": session.completed_at.isoformat() if session.completed_at else None,
                "total_elapsed_seconds": round(session.total_elapsed_time, 4),
            },
            "summary": session.get_summary(),
            "events": [self.format_event(e) for e in session.events],
            "llm_calls": [self._format_llm_call(c) for c in session.llm_calls],
            "retrievals": [self._format_retrieval(r) for r in session.retrievals],
            "tool_calls": [self._format_tool_call(t) for t in session.tool_calls],
        }
        
        if session.error:
            result["error"] = session.error
        
        return result

    def format_for_stream(self, event: TraceEvent) -> str:
        """Format an event for SSE streaming with full details."""
        data = self.format_event(event)
        return f"data:{json.dumps({'event': 'trace_detailed', 'data': data}, ensure_ascii=False)}\n\n"

    def _format_llm_call(self, call: LLMCallTrace) -> dict[str, Any]:
        """Format an LLM call trace."""
        result = {
            "call_id": call.call_id,
            "model_name": call.model_name,
            "tokens": {
                "prompt": call.prompt_tokens,
                "completion": call.completion_tokens,
                "total": call.total_tokens,
            },
            "latency_ms": round(call.latency_ms, 2),
            "temperature": call.temperature,
            "started_at": call.started_at.isoformat(),
            "completed_at": call.completed_at.isoformat() if call.completed_at else None,
        }
        
        if self.include_raw_data:
            result["prompt"] = call.prompt
            result["response"] = call.response
        else:
            result["prompt_preview"] = call.prompt[:200] + "..." if len(call.prompt) > 200 else call.prompt
            result["response_preview"] = call.response[:200] + "..." if call.response and len(call.response) > 200 else call.response
        
        if call.max_tokens:
            result["max_tokens"] = call.max_tokens
        if call.error:
            result["error"] = call.error
        
        return result

    def _format_retrieval(self, retrieval: RetrievalTrace) -> dict[str, Any]:
        """Format a retrieval trace."""
        result = {
            "retrieval_id": retrieval.retrieval_id,
            "query": retrieval.query,
            "knowledge_bases": retrieval.knowledge_bases,
            "config": {
                "top_k": retrieval.top_k,
                "similarity_threshold": retrieval.similarity_threshold,
                "rerank_enabled": retrieval.rerank_enabled,
            },
            "results": {
                "chunks_retrieved": retrieval.chunks_retrieved,
                "chunks_preview": retrieval.chunks[:3] if self.include_raw_data else [
                    {"id": c.get("id"), "score": c.get("score")} for c in retrieval.chunks[:3]
                ],
            },
            "latency_ms": round(retrieval.latency_ms, 2),
            "started_at": retrieval.started_at.isoformat(),
            "completed_at": retrieval.completed_at.isoformat() if retrieval.completed_at else None,
        }
        
        if retrieval.error:
            result["error"] = retrieval.error
        
        return result

    def _format_tool_call(self, tool: ToolCallTrace) -> dict[str, Any]:
        """Format a tool call trace."""
        result = {
            "call_id": tool.call_id,
            "tool_name": tool.tool_name,
            "tool_type": tool.tool_type,
            "arguments": tool.arguments,
            "latency_ms": round(tool.latency_ms, 2),
            "started_at": tool.started_at.isoformat(),
            "completed_at": tool.completed_at.isoformat() if tool.completed_at else None,
        }
        
        if self.include_raw_data:
            result["result"] = tool.result
        else:
            result_str = str(tool.result) if tool.result else None
            result["result_preview"] = result_str[:200] + "..." if result_str and len(result_str) > 200 else result_str
        
        if tool.error:
            result["error"] = tool.error
        
        return result


class TraceFormatterFactory:
    """Factory for creating trace formatters."""

    _formatters = {
        "streaming": StreamingTraceFormatter,
        "compact": CompactTraceFormatter,
        "detailed": DetailedTraceFormatter,
    }

    @classmethod
    def create(cls, format_type: str = "streaming", **kwargs) -> TraceFormatter:
        """Create a trace formatter by type."""
        formatter_class = cls._formatters.get(format_type)
        if not formatter_class:
            raise ValueError(f"Unknown formatter type: {format_type}. Available: {list(cls._formatters.keys())}")
        return formatter_class(**kwargs)

    @classmethod
    def register(cls, name: str, formatter_class: type) -> None:
        """Register a custom formatter."""
        if not issubclass(formatter_class, TraceFormatter):
            raise TypeError("Formatter must be a subclass of TraceFormatter")
        cls._formatters[name] = formatter_class

    @classmethod
    def available_formatters(cls) -> list[str]:
        """Get list of available formatter types."""
        return list(cls._formatters.keys())


def format_trace_for_api(
    session: TraceSession,
    format_type: str = "streaming",
    **kwargs
) -> dict[str, Any]:
    """Convenience function to format a trace session for API response."""
    formatter = TraceFormatterFactory.create(format_type, **kwargs)
    return formatter.format_session(session)


def generate_trace_stream(
    events: Generator[TraceEvent, None, None],
    format_type: str = "streaming",
    **kwargs
) -> Generator[str, None, None]:
    """Generate SSE stream from trace events."""
    formatter = TraceFormatterFactory.create(format_type, **kwargs)
    for event in events:
        yield formatter.format_for_stream(event)

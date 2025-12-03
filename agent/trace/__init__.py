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
Agent Trace Module

This module provides comprehensive tracing capabilities for agent execution,
including trace models, collectors, formatters, and services.
"""

from agent.trace.trace_models import (
    TraceEventType,
    TraceLevel,
    TraceMetadata,
    ComponentInfo,
    TraceEvent,
    LLMCallTrace,
    RetrievalTrace,
    ToolCallTrace,
    TraceSession,
)
from agent.trace.trace_collector import (
    TraceCollector,
    get_trace_collector,
    create_trace_collector,
)
from agent.trace.trace_formatter import (
    TraceFormatter,
    StreamingTraceFormatter,
    CompactTraceFormatter,
    DetailedTraceFormatter,
)

__all__ = [
    # Models
    "TraceEventType",
    "TraceLevel",
    "TraceMetadata",
    "ComponentInfo",
    "TraceEvent",
    "LLMCallTrace",
    "RetrievalTrace",
    "ToolCallTrace",
    "TraceSession",
    # Collector
    "TraceCollector",
    "get_trace_collector",
    "create_trace_collector",
    # Formatters
    "TraceFormatter",
    "StreamingTraceFormatter",
    "CompactTraceFormatter",
    "DetailedTraceFormatter",
]

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

"""
Task Context Module.

Provides [`TaskContext`](rag/svr/task_executor_refactor/task_context.py) as a typed wrapper
around the task dictionary, providing convenient property accessors for all
commonly used task attributes throughout the task executor refactor codebase.

This module defines:
- [`TaskDict`](rag/svr/task_executor_refactor/task_context.py): TypedDict for the raw task dictionary.
- [`TaskLimiters`](rag/svr/task_executor_refactor/task_context.py): Dataclass encapsulating all rate limiters.
- [`TaskCallbacks`](rag/svr/task_executor_refactor/task_context.py): Dataclass encapsulating all callback functions.
- [`TaskContext`](rag/svr/task_executor_refactor/task_context.py): Main facade combining the above components.

Usage example::

    from rag.svr.task_executor_refactor.task_context import TaskContext, TaskLimiters, TaskCallbacks

    ctx = TaskContext(
        task=task_dict,
        limiters=TaskLimiters(
            chat=chat_limiter,
            minio=minio_limiter,
            chunk=chunk_limiter,
            embed=embed_limiter,
            kg=kg_limiter,
        ),
        callbacks=TaskCallbacks(
            progress=progress_callback,
            has_canceled=has_canceled_func,
        ),
        write_interceptor=write_interceptor,
        recording_context=recording_context,
    )

    # Access task properties directly
    task_id = ctx.id
    tenant_id = ctx.tenant_id
    kb_id = ctx.kb_id
"""

import asyncio
from dataclasses import dataclass, field
from functools import partial
from typing import Any, Callable, Dict, List, Optional, Required, TypedDict

from rag.svr.task_executor_refactor.recording_context import BaseRecordingContext
from rag.svr.task_executor_refactor.write_operation_interceptor import WriteOperationInterceptor


# ============================================================================
# Type Definitions
# ============================================================================


class TaskDict(TypedDict, total=False):
    """TypedDict defining the structure of the raw task dictionary.

    All fields are optional except 'id' and 'tenant_id' which are required.
    """

    id: Required[str]
    """Task identifier (required)."""

    tenant_id: Required[str]
    """Tenant identifier (required)."""

    kb_id: str
    """Knowledge base / dataset identifier."""

    doc_id: str
    """Document identifier."""

    doc_ids: List[str]
    """List of document identifiers (for batch tasks like RAPTOR/GraphRAG)."""

    name: str
    """Document name."""

    location: str
    """Document location/path."""

    size: int
    """Document file size in bytes."""

    parser_id: str
    """Parser identifier (e.g., 'naive', 'table', 'paper')."""

    parser_config: Dict[str, Any]
    """Document-level parser configuration."""

    kb_parser_config: Dict[str, Any]

    """Knowledge base level parser configuration."""

    language: str
    """Document language (e.g., 'en', 'zh')."""

    llm_id: str
    """LLM model identifier."""

    embd_id: str
    """Embedding model identifier."""

    from_page: int
    """Starting page number for processing (0-based)."""

    to_page: int
    """Ending page number for processing (-1 means all pages)."""

    task_type: str
    """Task type (e.g., 'dataflow', 'raptor', 'graphrag', 'memory')."""

    dataflow_id: str
    """Dataflow/pipeline identifier."""

    pagerank: int
    """PageRank value for document scoring."""

    file: Any
    """File object for dataflow processing."""

    memory_id: str
    """Memory identifier for memory tasks."""

    source_id: str
    """Source identifier for memory tasks."""

    message_dict: Dict[str, Any]
    """Message dictionary for memory tasks."""

# ============================================================================
# Data Classes
# ============================================================================


@dataclass
class TaskLimiters:
    """Encapsulates all rate limiters for task execution.

    Each limiter is an asyncio.Semaphore used to control concurrency
    for different types of operations.
    """

    chat: asyncio.Semaphore = None
    """Asyncio semaphore for chat model rate limiting."""

    minio: asyncio.Semaphore = None
    """Asyncio semaphore for MinIO rate limiting."""

    chunk: asyncio.Semaphore = None
    """Asyncio semaphore for chunk building rate limiting."""

    embed: asyncio.Semaphore = None
    """Asyncio semaphore for embedding rate limiting."""

    kg: asyncio.Semaphore = None
    """Asyncio semaphore for knowledge graph rate limiting."""


def _noop_progress(**kwargs: Any) -> None:
    """No-op progress callback."""
    pass


def _not_canceled(task_id: str) -> bool:
    """Default cancellation check - always returns False."""
    return False


@dataclass
class TaskCallbacks:
    """Encapsulates all callback functions for task execution."""

    progress: Callable = field(default_factory=lambda: _noop_progress)
    """Callback function for progress updates (raw, requires task_id, from_page, to_page)."""

    has_canceled: Callable = field(default_factory=lambda: _not_canceled)
    """Function to check if task is canceled."""


# ============================================================================
# Main Class
# ============================================================================


class TaskContext:
    """Typed wrapper around the task dictionary providing convenient property accessors.

    This class uses composition to encapsulate:
    1. The raw task dictionary (TaskDict)
    2. Execution limiters (TaskLimiters)
    3. Callback functions (TaskCallbacks)
    4. Optional write operation interceptor
    5. Optional recording context for intermediate results

    The properties provide a clean interface for accessing task attributes
    without needing to use dictionary access with string keys throughout
    the codebase.
    """

    # Default values for optional task fields
    _DEFAULTS: Dict[str, Any] = {
        "kb_id": "",
        "doc_id": "",
        "doc_ids": [],
        "name": "",
        "location": "",
        "size": 0,
        "parser_id": "",
        "parser_config": {},
        "kb_parser_config": {},
        "language": "en",
        "llm_id": "",
        "embd_id": "",
        "from_page": 0,
        "to_page": -1,
        "task_type": "",
        "dataflow_id": "",
        "pagerank": 0,
        "memory_id": "",
        "source_id": "",
        "message_dict": {},
    }

    def __init__(
        self,
        task: TaskDict,
        limiters: TaskLimiters,
        callbacks: TaskCallbacks,
        write_interceptor: WriteOperationInterceptor = None,
        recording_context: BaseRecordingContext = None,
    ):
        """Initialize TaskContext.

        Args:
            task: The raw task dictionary containing all task attributes.
            limiters: TaskLimiters dataclass containing all rate limiters.
            callbacks: TaskCallbacks dataclass containing all callback functions.
            write_interceptor: Optional interceptor for write operations.
            recording_context: Optional BaseRecordingContext for intermediate result
                capture. Must be injected via constructor.

        Raises:
            ValueError: If required fields ('id', 'tenant_id') are missing from task.
        """
        # Validate required fields
        if "id" not in task:
            raise ValueError("Task must contain 'id'")
        if "tenant_id" not in task:
            raise ValueError("Task must contain 'tenant_id'")

        self._task = task
        self.limiters = limiters
        self.callbacks = callbacks
        self._write_interceptor = write_interceptor
        self._recording_context = recording_context


        # Prepare progress callback and set it on the context
        progress_cb = partial(
            callbacks.progress,
            self.id,
            self.from_page,
            self.to_page,
        )
        self._progress_cb = progress_cb

    # =========================================================================
    # Core task identity properties
    # =========================================================================

    @property
    def id(self) -> str:
        """Task identifier."""
        return self._task["id"]

    @property
    def tenant_id(self) -> str:
        """Tenant identifier."""
        return self._task["tenant_id"]

    @property
    def kb_id(self) -> str:
        """Knowledge base / dataset identifier."""
        return self._task.get("kb_id", self._DEFAULTS["kb_id"])

    @property
    def doc_id(self) -> str:
        """Document identifier."""
        return self._task.get("doc_id", self._DEFAULTS["doc_id"])

    @property
    def doc_ids(self) -> List[str]:
        """List of document identifiers (for batch tasks like RAPTOR/GraphRAG)."""
        return self._task.get("doc_ids", list(self._DEFAULTS["doc_ids"]))

    # =========================================================================
    # Document metadata properties
    # =========================================================================

    @property
    def name(self) -> str:
        """Document name."""
        return self._task.get("name", self._DEFAULTS["name"])

    @property
    def location(self) -> str:
        """Document location/path."""
        return self._task.get("location", self._DEFAULTS["location"])

    @property
    def size(self) -> int:
        """Document file size in bytes."""
        return self._task.get("size", self._DEFAULTS["size"])

    # =========================================================================
    # Parser configuration properties
    # =========================================================================

    @property
    def parser_id(self) -> str:
        """Parser identifier (e.g., 'naive', 'table', 'paper')."""
        return self._task.get("parser_id", self._DEFAULTS["parser_id"])

    @property
    def parser_config(self) -> Dict[str, Any]:
        """Document-level parser configuration."""
        return self._task.get("parser_config", {})

    @property
    def kb_parser_config(self) -> Dict[str, Any]:
        """Knowledge base level parser configuration."""
        return self._task.get("kb_parser_config", {})

    # =========================================================================
    # Language and model properties
    # =========================================================================

    @property
    def language(self) -> str:
        """Document language (e.g., 'en', 'zh')."""
        return self._task.get("language", self._DEFAULTS["language"])

    @property
    def llm_id(self) -> str:
        """LLM model identifier."""
        return self._task.get("llm_id", self._DEFAULTS["llm_id"])

    @property
    def embd_id(self) -> str:
        """Embedding model identifier."""
        return self._task.get("embd_id", self._DEFAULTS["embd_id"])

    # =========================================================================
    # Page range properties
    # =========================================================================

    @property
    def from_page(self) -> int:
        """Starting page number for processing (0-based)."""
        return self._task.get("from_page", self._DEFAULTS["from_page"])

    @property
    def to_page(self) -> int:
        """Ending page number for processing (-1 means all pages)."""
        return self._task.get("to_page", self._DEFAULTS["to_page"])

    # =========================================================================
    # Task type and routing properties
    # =========================================================================

    @property
    def task_type(self) -> str:
        """Task type (e.g., 'dataflow', 'raptor', 'graphrag', 'memory')."""
        return self._task.get("task_type", self._DEFAULTS["task_type"])

    @property
    def dataflow_id(self) -> str:
        """Dataflow/pipeline identifier."""
        return self._task.get("dataflow_id", self._DEFAULTS["dataflow_id"])

    # =========================================================================
    # Additional properties
    # =========================================================================

    @property
    def pagerank(self) -> int:
        """PageRank value for document scoring."""
        return self._task.get("pagerank", self._DEFAULTS["pagerank"])

    @property
    def file(self) -> Optional[Any]:
        """File object for dataflow processing."""
        return self._task.get("file")

    # =========================================================================
    # Memory task specific properties
    # =========================================================================

    @property
    def memory_id(self) -> str:
        """Memory identifier for memory tasks."""
        return self._task.get("memory_id", self._DEFAULTS["memory_id"])

    @property
    def source_id(self) -> str:
        """Source identifier for memory tasks."""
        return self._task.get("source_id", self._DEFAULTS["source_id"])

    @property
    def message_dict(self) -> Dict[str, Any]:
        """Message dictionary for memory tasks."""
        return self._task.get("message_dict", {})

    # =========================================================================
    # Raw task dictionary access
    # =========================================================================

    @property
    def raw_task(self) -> Dict[str, Any]:
        """Return the raw task dictionary."""
        return self._task

    def get(self, key: str, default: Any = None) -> Any:
        """Get a value from the task dictionary with a default.

        Args:
            key: The key to look up.
            default: Default value if key is not found.

        Returns:
            The value associated with the key, or default if not found.
        """
        return self._task.get(key, default)

    # =========================================================================
    # Limiter properties (proxies to TaskLimiters dataclass)
    # =========================================================================

    @property
    def chat_limiter(self) -> asyncio.Semaphore:
        """Asyncio semaphore for chat model rate limiting."""
        return self.limiters.chat or asyncio.Semaphore(1)

    @property
    def minio_limiter(self) -> asyncio.Semaphore:
        """Asyncio semaphore for MinIO rate limiting."""
        return self.limiters.minio or asyncio.Semaphore(1)

    @property
    def chunk_limiter(self) -> asyncio.Semaphore:
        """Asyncio semaphore for chunk building rate limiting."""
        return self.limiters.chunk or asyncio.Semaphore(1)

    @property
    def embed_limiter(self) -> asyncio.Semaphore:
        """Asyncio semaphore for embedding rate limiting."""
        return self.limiters.embed or asyncio.Semaphore(1)

    @property
    def kg_limiter(self) -> asyncio.Semaphore:
        """Asyncio semaphore for knowledge graph rate limiting."""
        return self.limiters.kg or asyncio.Semaphore(1)

    # =========================================================================
    # Context and interceptor properties
    # =========================================================================

    @property
    def recording_context(self) -> BaseRecordingContext:
        """BaseRecordingContext for this task.

        Must be injected via constructor. Raises RuntimeError if accessed
        before initialization or if no context was provided.
        """
        if self._recording_context is None:
            raise RuntimeError("recording_context accessed but not injected into TaskContext")
        return self._recording_context

    @property
    def write_interceptor(self) -> WriteOperationInterceptor:
        """Write operation interceptor for comparison mode."""
        return self._write_interceptor

    # =========================================================================
    # Callback properties (proxies to TaskCallbacks dataclass)
    # =========================================================================

    @property
    def has_canceled_func(self) -> Callable:
        """Function to check if task is canceled."""
        return self.callbacks.has_canceled

    # =========================================================================
    # Pre-bound progress callback
    # =========================================================================

    @property
    def progress_cb(self) -> Callable:
        """Pre-bound progress callback (task_id, from_page, to_page already bound).

        Use this property in services for progress updates.
        Falls back to progress_callback if progress_cb is not set.
        """
        return self._progress_cb

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

"""Recording Context Module.

This module provides the [`BaseRecordingContext`](rag/svr/task_executor_refactor/recording_context.py:48) abstract base class,
[`RecordingContext`](rag/svr/task_executor_refactor/recording_context.py:89) concrete class, and
[`NullRecordingContext`](rag/svr/task_executor_refactor/recording_context.py:204) no-op class, which capture
actual execution results from the production code path (e.g., [`do_handle_task()`](rag/svr/task_executor.py))
for later comparison with dry-run results.

The recording context is used throughout the task execution pipeline to collect
intermediate metrics and final results at various stages:

1. **File validation**: Records file size check results and parser ID
2. **Chunking**: Records raw chunks after document splitting
3. **Outline extraction**: Records whether outline was extracted and entry count
4. **MinIO upload**: Records document count after image upload
5. **Post-processing**: Records counts for keywords, questions, metadata, and tags
6. **Final results**: Records final chunks and their IDs for comparison

The module also provides context variable management functions and a timing
decorator that automatically integrates with the current recording context.

Usage example::

    from rag.svr.task_executor_refactor.recording_context import RecordingContext

    ctx = RecordingContext()
    ctx.record("raw_chunk_count", 42)
    ctx.record("final_chunks", chunks)

    # Later, in comparison:
    comparator.compare(task_id, ctx, dry_run_records)
"""

import contextvars
import functools
import time
from abc import ABC, abstractmethod
from contextlib import contextmanager
from typing import Any, Callable, Dict, List, Tuple


class BaseRecordingContext(ABC):
    """Abstract base class for recording context implementations.

    Defines the common interface shared by
    [`RecordingContext`](rag/svr/task_executor_refactor/recording_context.py:89) and
    [`NullRecordingContext`](rag/svr/task_executor_refactor/recording_context.py:204).

    Variables typed as ``BaseRecordingContext`` can hold either implementation,
    enabling production/dry-run polymorphism without conditional branches.
    """

    @abstractmethod
    def record(self, key: str, value: Any) -> None:
        """Record a value with the given key."""

    @abstractmethod
    def save_func_return_value(self, func_name: str, return_value: Any) -> None:
        """Record a function's return value into a list associated with func_name."""

    @abstractmethod
    def get_func_return_values(self, func_name: str) -> List[Any]:
        """Get the list of recorded return values for a function."""

    @abstractmethod
    def get(self, key: str, default: Any = None) -> Any:
        """Get a recorded value by key."""

    @abstractmethod
    def get_all_func_return_values(self) -> Dict[str, Any]:
        """Get all recorded data."""

    @abstractmethod
    def has(self, key: str) -> bool:
        """Check if a key exists in recorded data."""

    @abstractmethod
    def clear(self) -> None:
        """Clear all recorded data."""

    @abstractmethod
    def reset(self) -> None:
        """Clear all recorded data and timing records."""

    @abstractmethod
    @contextmanager
    def measure(self, name: str):
        """Timing context manager to record execution duration."""

    @abstractmethod
    def __repr__(self) -> str:
        """Return a string representation."""


class RecordingContext(BaseRecordingContext):
    """Captures actual execution results from production code for comparison.

    This class acts as a dictionary-like container that stores key-value pairs
    representing various metrics and intermediate results collected during
    the production execution of a document processing task. It also supports
    timing measurements via the [`measure()`](rag/svr/task_executor_refactor/recording_context.py:78) context manager.

    The recorded data is later consumed by the [`Comparator`](rag/svr/task_executor_refactor/comparator.py:130)
    to compare against dry-run execution results.

    Example:
        >>> ctx = RecordingContext()
        >>> ctx.record("chunk_count", 100)
        >>> ctx.get("chunk_count")
        100
        >>> ctx.get("missing_key", "default")
        'default'
    """

    def __init__(self) -> None:
        """Initialize a new RecordingContext."""
        self._data: Dict[str, Any] = {}
        self.records: List[Tuple[str, float]] = []

    def record(self, key: str, value: Any) -> None:
        """Record a value with the given key.

        This method stores the provided value under the specified key in the
        internal data dictionary. If the key already exists, the value will
        be overwritten.

        Args:
            key: The key to store the value under. Should be a descriptive
                string that identifies the metric or result being recorded.
            value: The value to record. Can be any Python object, including
                primitives, lists, dicts, or complex objects.
        """
        self._data[key] = value

    def save_func_return_value(self, func_name: str, return_value: Any) -> None:
        """Record a function's return value into a list associated with func_name.

        Each func_name has a corresponding return_values_list. This method appends
        the return_value to the list for the given func_name. If the list does not
        exist, it will be created.

        Args:
            func_name: The name of the function whose return value is being recorded.
            return_value: The return value to record.
        """
        if func_name not in self._data:
            self._data[func_name] = []
        self._data[func_name].append(return_value)

    def get_func_return_values(self, func_name: str) -> List[Any]:
        """Get the list of recorded return values for a function.

        Args:
            func_name: The name of the function.

        Returns:
            A list of recorded return values, or an empty list if not found.
        """
        return self._data.get(func_name, [])

    def get(self, key: str, default: Any = None) -> Any:
        """Get a recorded value by key.

        Retrieves the value associated with the given key. If the key does
        not exist, returns the provided default value.

        Args:
            key: The key to look up in the recorded data.
            default: Default value to return if the key is not found.
                Defaults to None.

        Returns:
            The recorded value associated with the key, or the default value
            if the key does not exist.
        """
        return self._data.get(key, default)

    def get_all_func_return_values(self) -> Dict[str, Any]:
        """Get all recorded data.

        Returns a shallow copy of all recorded data as a dictionary.
        Modifications to the returned dictionary will not affect the
        internal state of this context.

        Returns:
            A new dictionary containing all recorded key-value pairs.
        """
        return dict(self._data)

    def has(self, key: str) -> bool:
        """Check if a key exists in recorded data.

        Args:
            key: The key to check for existence.

        Returns:
            True if the key exists in the recorded data, False otherwise.
        """
        return key in self._data

    def clear(self) -> None:
        """Clear all recorded data.

        Removes all key-value pairs from the internal data dictionary
        and clears all timing records, resetting the context to its
        initial empty state.
        """
        self._data.clear()
        self.records.clear()

    @contextmanager
    def measure(self, name: str):
        """Timing context manager to record execution duration.

        Records the elapsed time (in seconds) for the operation specified
        by `name`.

        Usage::

            with ctx.measure("build_chunks"):
                ...

        Args:
            name: A descriptive name for the timed operation.
        """
        start = time.perf_counter()
        try:
            yield
        finally:
            elapsed = time.perf_counter() - start
            self.records.append((name, elapsed))

    def reset(self) -> None:
        """Clear all recorded data and timing records."""
        self.clear()

    def __repr__(self) -> str:
        """Return a string representation of the RecordingContext.

        Returns:
            A string showing the class name and all recorded data.
        """
        return f"RecordingContext({self._data})"


class NullRecordingContext(BaseRecordingContext):
    """No-op RecordingContext for production mode.

    Accepts all RecordingContext API calls but performs no allocation.
    Eliminates memory overhead in production where recorded data is unused.

    Uses __slots__ for zero instance memory footprint.

    Usage:
        >>> ctx = NullRecordingContext()
        >>> ctx.record("chunks", large_list)  # no-op, no memory allocated
        >>> ctx.get("chunks")                 # always returns None
    """

    __slots__ = ()

    def record(self, key: str, value: Any) -> None:
        pass

    def save_func_return_value(self, func_name: str, return_value: Any) -> None:
        pass

    def get_func_return_values(self, func_name: str) -> List[Any]:
        return []

    def get(self, key: str, default: Any = None) -> Any:
        return default

    def get_all_func_return_values(self) -> Dict[str, Any]:
        return {}

    def has(self, key: str) -> bool:
        return False

    def clear(self) -> None:
        pass

    def reset(self) -> None:
        pass

    @contextmanager
    def measure(self, name: str):
        yield

    def __repr__(self) -> str:
        return "NullRecordingContext()"


# Module-level singleton to avoid repeated allocations
_NULL_RECORDING_CONTEXT = NullRecordingContext()


# Context variable for coroutine / thread isolation
_recording_ctx_var: contextvars.ContextVar[BaseRecordingContext] = contextvars.ContextVar("recording_context")


def get_recording_context() -> BaseRecordingContext:
    """Get the BaseRecordingContext for the current execution context.

    Returns the BaseRecordingContext bound to the current coroutine / thread.
    If no context has been bound, raise RuntimeError.

    Returns:
        The current BaseRecordingContext, raise RuntimeError if not set.
    """
    context = _recording_ctx_var.get(None)
    if context is None:
        raise RuntimeError("no context")
    return context


def set_recording_context(ctx: BaseRecordingContext) -> None:
    """Bind a BaseRecordingContext to the current execution context.

    Args:
        ctx: The BaseRecordingContext to bind, or None to unbind.
    """
    _recording_ctx_var.set(ctx)


@contextmanager
def recording_context_manager(ctx: BaseRecordingContext = None):
    """Context manager that sets and restores the BaseRecordingContext.

    Usage::

        with recording_context_manager(RecordingContext()) as ctx:
            ctx.record("key", "value")

    Args:
        ctx: The BaseRecordingContext to use. If None, a new one is created.

    Yields:
        The BaseRecordingContext that was set.
    """
    if ctx is None:
        ctx = RecordingContext()
    token = _recording_ctx_var.set(ctx)
    try:
        yield ctx
    finally:
        _recording_ctx_var.reset(token)


def timed_with_recording(
    func: Callable = None,
    *,
    recording_context: BaseRecordingContext = None,
) -> Callable:
    """Decorator that automatically uses the current BaseRecordingContext for timing.

    Supports two usage forms:

    1. Direct decoration (automatically uses context variable):

        @timed_with_recording
        def foo(): ...

    2. Parameterized decoration with explicit BaseRecordingContext:

        @timed_with_recording(recording_context=my_ctx)
        def foo(): ...

    The decorator records the execution time of the decorated function
    into the BaseRecordingContext's timing records.

    Args:
        func: The function to decorate (used when called without parentheses).
        recording_context: Optional BaseRecordingContext to use for timing.
            If not provided, uses the context variable's current value.

    Returns:
        The decorated function.
    """
    from common.decorator import timing

    if func is not None and callable(func):
        # Used as @timed_with_recording without parentheses
        @functools.wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            ctx = recording_context or get_recording_context()
            if ctx is not None:
                return timing(context=ctx)(func)(*args, **kwargs)
            return func(*args, **kwargs)

        return wrapper

    # Used as @timed_with_recording(...) with parentheses
    def decorator(the_func: Callable) -> Callable:
        @functools.wraps(the_func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            ctx = recording_context or get_recording_context()
            if ctx is not None:
                return timing(context=ctx)(the_func)(*args, **kwargs)
            return the_func(*args, **kwargs)

        return wrapper

    return decorator

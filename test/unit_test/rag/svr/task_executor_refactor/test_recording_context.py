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
Unit tests for RecordingContext module.
"""

import time
import pytest
from rag.svr.task_executor_refactor.recording_context import (
    RecordingContext,
    get_recording_context,
    set_recording_context,
    recording_context_manager,
    timed_with_recording,
)


class TestRecordingContextInit:
    """Tests for RecordingContext initialization."""

    def test_init_creates_empty_data(self):
        """Test that __init__ creates empty _data dict."""
        ctx = RecordingContext()
        assert ctx._data == {}

    def test_init_creates_empty_records(self):
        """Test that __init__ creates empty records list."""
        ctx = RecordingContext()
        assert ctx.records == []


class TestRecordingContextRecord:
    """Tests for RecordingContext.record method."""

    def test_record_single_value(self):
        """Test recording a single value."""
        ctx = RecordingContext()
        ctx.record("chunk_count", 100)
        assert ctx.get("chunk_count") == 100

    def test_record_overwrites_existing_value(self):
        """Test that recording with same key overwrites previous value."""
        ctx = RecordingContext()
        ctx.record("key", "value1")
        ctx.record("key", "value2")
        assert ctx.get("key") == "value2"

    def test_record_none_value(self):
        """Test recording None value."""
        ctx = RecordingContext()
        ctx.record("key", None)
        assert ctx.get("key") is None

    def test_record_complex_object(self):
        """Test recording a complex object like list or dict."""
        ctx = RecordingContext()
        ctx.record("chunks", [{"id": 1}, {"id": 2}])
        assert ctx.get("chunks") == [{"id": 1}, {"id": 2}]


class TestRecordingContextFuncReturnValues:
    """Tests for function return value recording."""

    def test_save_func_return_value_first_call(self):
        """Test saving first return value for a function."""
        ctx = RecordingContext()
        ctx.save_func_return_value("test_func", 42)
        assert ctx.get_func_return_values("test_func") == [42]

    def test_save_func_return_value_multiple_calls(self):
        """Test saving multiple return values for same function."""
        ctx = RecordingContext()
        ctx.save_func_return_value("test_func", 1)
        ctx.save_func_return_value("test_func", 2)
        ctx.save_func_return_value("test_func", 3)
        assert ctx.get_func_return_values("test_func") == [1, 2, 3]

    def test_get_func_return_values_nonexistent_function(self):
        """Test getting return values for nonexistent function returns empty list."""
        ctx = RecordingContext()
        assert ctx.get_func_return_values("nonexistent") == []

    def test_get_func_return_values_multiple_functions(self):
        """Test getting return values for different functions."""
        ctx = RecordingContext()
        ctx.save_func_return_value("func_a", "a1")
        ctx.save_func_return_value("func_b", "b1")
        ctx.save_func_return_value("func_a", "a2")
        assert ctx.get_func_return_values("func_a") == ["a1", "a2"]
        assert ctx.get_func_return_values("func_b") == ["b1"]


class TestRecordingContextGet:
    """Tests for RecordingContext.get method."""

    def test_get_existing_key(self):
        """Test getting an existing key."""
        ctx = RecordingContext()
        ctx.record("key", "value")
        assert ctx.get("key") == "value"

    def test_get_nonexistent_key_returns_none(self):
        """Test getting nonexistent key returns None."""
        ctx = RecordingContext()
        assert ctx.get("missing") is None

    def test_get_nonexistent_key_returns_default(self):
        """Test getting nonexistent key returns provided default."""
        ctx = RecordingContext()
        assert ctx.get("missing", "default") == "default"

    def test_get_with_none_default(self):
        """Test getting with None as default."""
        ctx = RecordingContext()
        assert ctx.get("missing", None) is None


class TestRecordingContextGetAllFuncReturnValues:
    """Tests for get_all_func_return_values method."""

    def test_get_all_func_return_values_empty(self):
        """Test getting all values when none recorded."""
        ctx = RecordingContext()
        assert ctx.get_all_func_return_values() == {}

    def test_get_all_func_return_values_with_data(self):
        """Test getting all values with some data."""
        ctx = RecordingContext()
        ctx.save_func_return_value("func_a", 1)
        ctx.save_func_return_value("func_b", 2)
        result = ctx.get_all_func_return_values()
        assert result == {"func_a": [1], "func_b": [2]}

    def test_get_all_func_return_values_returns_copy(self):
        """Test that returned dict is a copy, not the original."""
        ctx = RecordingContext()
        ctx.save_func_return_value("func", 1)
        result = ctx.get_all_func_return_values()
        result["func"] = []
        # Original should be unchanged
        assert ctx.get_func_return_values("func") == [1]


class TestRecordingContextHas:
    """Tests for RecordingContext.has method."""

    def test_has_existing_key(self):
        """Test has returns True for existing key."""
        ctx = RecordingContext()
        ctx.record("key", "value")
        assert ctx.has("key") is True

    def test_has_nonexistent_key(self):
        """Test has returns False for nonexistent key."""
        ctx = RecordingContext()
        assert ctx.has("missing") is False

    def test_has_after_clear(self):
        """Test has returns False after clear."""
        ctx = RecordingContext()
        ctx.record("key", "value")
        ctx.clear()
        assert ctx.has("key") is False


class TestRecordingContextClear:
    """Tests for RecordingContext.clear method."""

    def test_clear_removes_all_data(self):
        """Test that clear removes all recorded data."""
        ctx = RecordingContext()
        ctx.record("key1", "value1")
        ctx.record("key2", "value2")
        ctx.clear()
        assert ctx._data == {}

    def test_clear_removes_all_records(self):
        """Test that clear removes all timing records."""
        ctx = RecordingContext()
        with ctx.measure("op1"):
            pass
        ctx.clear()
        assert ctx.records == []


class TestRecordingContextMeasure:
    """Tests for RecordingContext.measure context manager."""

    def test_measure_records_elapsed_time(self):
        """Test that measure records elapsed time."""
        ctx = RecordingContext()
        with ctx.measure("test_op"):
            time.sleep(0.01)
        assert len(ctx.records) == 1
        assert ctx.records[0][0] == "test_op"
        assert ctx.records[0][1] >= 0.01

    def test_measure_multiple_operations(self):
        """Test measuring multiple operations."""
        ctx = RecordingContext()
        with ctx.measure("op1"):
            time.sleep(0.01)
        with ctx.measure("op2"):
            time.sleep(0.02)
        assert len(ctx.records) == 2
        assert ctx.records[0][0] == "op1"
        assert ctx.records[1][0] == "op2"

    def test_measure_preserves_context_on_exception(self):
        """Test that measure still records time on exception."""
        ctx = RecordingContext()
        with pytest.raises(ValueError):
            with ctx.measure("failing_op"):
                raise ValueError("test error")
        assert len(ctx.records) == 1
        assert ctx.records[0][0] == "failing_op"


class TestRecordingContextReset:
    """Tests for RecordingContext.reset method."""

    def test_reset_clears_data(self):
        """Test that reset clears all data."""
        ctx = RecordingContext()
        ctx.record("key", "value")
        ctx.reset()
        assert ctx._data == {}

    def test_reset_clears_records(self):
        """Test that reset clears all records."""
        ctx = RecordingContext()
        with ctx.measure("op"):
            pass
        ctx.reset()
        assert ctx.records == []


class TestRecordingContextRepr:
    """Tests for RecordingContext.__repr__ method."""

    def test_repr_empty_context(self):
        """Test repr of empty context."""
        ctx = RecordingContext()
        assert "RecordingContext" in repr(ctx)

    def test_repr_with_data(self):
        """Test repr with some data."""
        ctx = RecordingContext()
        ctx.record("key", "value")
        r = repr(ctx)
        assert "RecordingContext" in r
        assert "key" in r


class TestContextVariableFunctions:
    """Tests for context variable functions."""

    def test_set_and_get_recording_context(self):
        """Test set and get recording context."""
        ctx = RecordingContext()
        set_recording_context(ctx)
        assert get_recording_context() is ctx

    def test_set_recording_context_none_unbinds(self):
        """Test setting None unbinds the context."""
        ctx = RecordingContext()
        set_recording_context(ctx)
        set_recording_context(None)
        # After unbinding, get should raise RuntimeError
        with pytest.raises(RuntimeError, match="no context"):
            get_recording_context()


class TestRecordingContextManager:
    """Tests for recording_context_manager context manager."""

    def test_context_manager_with_provided_context(self):
        """Test context manager with provided context."""
        ctx = RecordingContext()
        with recording_context_manager(ctx) as mgr:
            assert mgr is ctx
            mgr.record("key", "value")
        assert ctx.get("key") == "value"

    def test_context_manager_creates_new_context(self):
        """Test context manager creates new context when none provided."""
        with recording_context_manager() as ctx:
            assert isinstance(ctx, RecordingContext)
            ctx.record("key", "value")
            assert ctx.get("key") == "value"

    def test_context_manager_restores_previous_context(self):
        """Test context manager restores previous context after exit."""
        outer_ctx = RecordingContext()
        set_recording_context(outer_ctx)

        inner_ctx = RecordingContext()
        with recording_context_manager(inner_ctx):
            assert get_recording_context() is inner_ctx

        # After exiting, should restore outer_ctx
        assert get_recording_context() is outer_ctx


class TestTimedWithRecordingDecorator:
    """Tests for timed_with_recording decorator."""

    def test_decorator_without_parentheses(self):
        """Test decorator used without parentheses."""
        ctx = RecordingContext()
        set_recording_context(ctx)

        @timed_with_recording
        def test_func():
            time.sleep(0.01)
            return 42

        result = test_func()
        assert result == 42

    def test_decorator_with_parentheses_and_context(self):
        """Test decorator with explicit context."""
        ctx = RecordingContext()

        @timed_with_recording(recording_context=ctx)
        def test_func():
            time.sleep(0.01)
            return "hello"

        result = test_func()
        assert result == "hello"

    def test_decorator_without_context_raises_error(self):
        """Test decorator raises RuntimeError when no context is available."""
        # Ensure no context is set
        set_recording_context(None)

        @timed_with_recording
        def test_func():
            return 123

        # Should raise RuntimeError because no context is available
        with pytest.raises(RuntimeError, match="no context"):
            test_func()

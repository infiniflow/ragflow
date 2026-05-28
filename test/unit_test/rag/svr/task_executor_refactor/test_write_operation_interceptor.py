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
Unit tests for WriteOperationInterceptor module.
"""

import pytest
from rag.svr.task_executor_refactor.write_operation_interceptor import (
    WriteOperationInterceptor,
    ALLOWED_METHOD_NAMES,
)


def _create_valid_recorded_values():
    """Helper to create valid recorded_values dict."""
    return {method: [] for method in ALLOWED_METHOD_NAMES}


@pytest.fixture
def valid_recorded_values():
    """Provide a valid recorded_values dict for testing."""
    return _create_valid_recorded_values()


class TestAllowedMethodNames:
    """Tests for ALLOWED_METHOD_NAMES constant."""

    def test_allowed_method_names_count(self):
        """Test that ALLOWED_METHOD_NAMES contains exactly 8 methods."""
        assert len(ALLOWED_METHOD_NAMES) == 10

    def test_allowed_method_names_contains_expected_methods(self):
        """Test that ALLOWED_METHOD_NAMES contains all expected methods."""
        expected_methods = {
            "KnowledgebaseService.update_by_id",
            "TaskService.update_chunk_ids",
            "DocumentService.increment_chunk_num",
            "DocMetadataService.update_document_metadata",
            "PipelineOperationLogService.record_pipeline_operation",
            "handle_save_to_memory_task",
            "PipelineOperationLogService.create",
            "delete_raptor_chunks",
            "docStoreConn.insert",
            "docStoreConn.delete"
        }
        assert ALLOWED_METHOD_NAMES == expected_methods


class TestWriteOperationInterceptorInit:
    """Tests for WriteOperationInterceptor.__init__."""

    def test_init_with_valid_empty_values(self, valid_recorded_values):
        """Test initialization with valid but empty values for all methods."""
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        assert interceptor is not None

    def test_init_with_valid_values(self, valid_recorded_values):
        """Test initialization with valid recorded values."""
        valid_recorded_values["KnowledgebaseService.update_by_id"] = [1, 0]
        valid_recorded_values["handle_save_to_memory_task"] = [None]
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        assert interceptor is not None

    def test_init_with_extra_keys_ignored(self, valid_recorded_values):
        """Test that extra keys in recorded_values are ignored."""
        valid_recorded_values["invalid_method_name"] = [1, 2, 3]
        # Should not raise an error, extra keys are simply ignored
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        assert interceptor is not None
        # The extra key should not be accessible
        assert "invalid_method_name" not in interceptor._recorded_values


class TestWriteOperationInterceptorIntercept:
    """Tests for WriteOperationInterceptor.intercept."""

    def test_intercept_returns_first_value(self, valid_recorded_values):
        """Test that intercept returns the first value in the list."""
        valid_recorded_values["KnowledgebaseService.update_by_id"] = [1, 0, 2]
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        result = interceptor.intercept("KnowledgebaseService.update_by_id")
        assert result == 1

    def test_intercept_returns_subsequent_values(self, valid_recorded_values):
        """Test that intercept returns subsequent values on each call."""
        valid_recorded_values["KnowledgebaseService.update_by_id"] = [1, 0, 2]
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        assert interceptor.intercept("KnowledgebaseService.update_by_id") == 1
        assert interceptor.intercept("KnowledgebaseService.update_by_id") == 0
        assert interceptor.intercept("KnowledgebaseService.update_by_id") == 2

    def test_intercept_invalid_method_raises_value_error(self, valid_recorded_values):
        """Test that intercepting an invalid method raises ValueError."""
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        with pytest.raises(ValueError, match="Cannot intercept method"):
            interceptor.intercept("invalid_method_name")

    def test_intercept_empty_list_raises_index_error(self, valid_recorded_values):
        """Test that intercepting when list is empty raises IndexError."""
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        with pytest.raises(IndexError, match="No more recorded values"):
            interceptor.intercept("KnowledgebaseService.update_by_id")

    def test_intercept_pops_value(self, valid_recorded_values):
        """Test that intercept pops the value from the internal list."""
        valid_recorded_values["KnowledgebaseService.update_by_id"] = [42]
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        interceptor.intercept("KnowledgebaseService.update_by_id")
        # Check internal state, not the original input list (which is copied)
        assert len(interceptor._recorded_values["KnowledgebaseService.update_by_id"]) == 0

    def test_intercept_with_none_value(self, valid_recorded_values):
        """Test that intercept can return None values."""
        valid_recorded_values["handle_save_to_memory_task"] = [None]
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        result = interceptor.intercept("handle_save_to_memory_task")
        assert result is None

    def test_intercept_with_default_value_when_empty(self, valid_recorded_values):
        """Test that intercept returns default_value when list is empty."""
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        result = interceptor.intercept("KnowledgebaseService.update_by_id", default_value=42)
        assert result == 42

    def test_intercept_with_default_value_none_when_empty(self, valid_recorded_values):
        """Test that intercept returns None when default_value is None and list is empty."""
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        # When default_value is None, it should return None (not raise IndexError)
        # because None is a valid default value (different from _NO_DEFAULT sentinel)
        result = interceptor.intercept("KnowledgebaseService.update_by_id", default_value=None)
        assert result is None

    def test_intercept_default_value_does_not_affect_existing_values(self, valid_recorded_values):
        """Test that default_value is only used when list is empty."""
        valid_recorded_values["KnowledgebaseService.update_by_id"] = [100]
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        # Should return the recorded value, not the default_value
        result = interceptor.intercept("KnowledgebaseService.update_by_id", default_value=999)
        assert result == 100

    @pytest.mark.parametrize("default_value", [
        "default_string",
        {"status": "success", "data": [1, 2, 3]},
        [1, 2, 3, 4, 5],
        (1, "two", 3.0),
        True,
        False,
        0,
        "",
        [],
        {},
    ])
    def test_intercept_with_various_default_values(self, valid_recorded_values, default_value):
        """Test that intercept returns various default_value types when list is empty."""
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        result = interceptor.intercept("KnowledgebaseService.update_by_id", default_value=default_value)
        assert result == default_value

    def test_intercept_with_complex_values(self, valid_recorded_values):
        """Test that intercept can return complex values like dicts and tuples."""
        complex_value = {"key": "value", "nested": [1, 2, 3]}
        valid_recorded_values["DocMetadataService.update_document_metadata"] = [complex_value]
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        result = interceptor.intercept("DocMetadataService.update_document_metadata")
        assert result == complex_value

class TestWriteOperationInterceptorRemainingCount:
    """Tests for WriteOperationInterceptor.remaining_count."""

    def test_remaining_count_with_values(self, valid_recorded_values):
        """Test remaining_count returns correct count."""
        valid_recorded_values["KnowledgebaseService.update_by_id"] = [1, 2, 3]
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        assert interceptor.remaining_count("KnowledgebaseService.update_by_id") == 3

    def test_remaining_count_empty_list(self, valid_recorded_values):
        """Test remaining_count returns 0 for empty list."""
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        assert interceptor.remaining_count("KnowledgebaseService.update_by_id") == 0

        with pytest.raises(IndexError):
            interceptor.intercept("KnowledgebaseService.update_by_id")

    def test_remaining_count_after_intercept(self, valid_recorded_values):
        """Test remaining_count decreases after intercept calls."""
        valid_recorded_values["KnowledgebaseService.update_by_id"] = [1, 2, 3]
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        assert interceptor.remaining_count("KnowledgebaseService.update_by_id") == 3
        interceptor.intercept("KnowledgebaseService.update_by_id")
        assert interceptor.remaining_count("KnowledgebaseService.update_by_id") == 2
        interceptor.intercept("KnowledgebaseService.update_by_id")
        assert interceptor.remaining_count("KnowledgebaseService.update_by_id") == 1
        interceptor.intercept("KnowledgebaseService.update_by_id")
        assert interceptor.remaining_count("KnowledgebaseService.update_by_id") == 0

    def test_remaining_count_invalid_method(self, valid_recorded_values):
        """Test remaining_count returns 0 for invalid method names."""
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        assert interceptor.remaining_count("invalid_method") == 0


class TestWriteOperationInterceptorRepr:
    """Tests for WriteOperationInterceptor.__repr__."""

    def test_repr_contains_class_name(self, valid_recorded_values):
        """Test that repr contains the class name."""
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        repr_str = repr(interceptor)
        assert "WriteOperationInterceptor" in repr_str

    def test_repr_contains_total_recorded(self, valid_recorded_values):
        """Test that repr contains total_recorded."""
        interceptor = WriteOperationInterceptor(valid_recorded_values)
        repr_str = repr(interceptor)
        assert "total_recorded=" in repr_str

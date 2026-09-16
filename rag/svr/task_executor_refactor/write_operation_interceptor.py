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
Write Operation Interceptor Module

Provides a mechanism to intercept write operations during comparison mode.
The interceptor consumes pre-recorded return values (from production execution)
and returns them one by one when the corresponding methods are called.
"""

import logging
from typing import Any, Dict, List

# Set of allowed method names that can be intercepted
ALLOWED_METHOD_NAMES = {
    "KnowledgebaseService.update_by_id",
    "TaskService.update_chunk_ids",
    "DocumentService.increment_chunk_num",
    "DocMetadataService.update_document_metadata",
    "PipelineOperationLogService.record_pipeline_operation",
    "PipelineOperationLogService.create",
    "delete_raptor_chunks",
    "handle_save_to_memory_task",
    "docStoreConn.insert",
    "docStoreConn.delete",
}

_NO_DEFAULT = object()


class WriteOperationInterceptor:
    """Intercepts write operations and returns pre-recorded values.

    This interceptor is used in comparison mode to replay production execution
    results. When a method is called, the interceptor pops the first recorded
    return value from the corresponding list and returns it.

    Usage:
        # Create interceptor with pre-recorded values
        interceptor = WriteOperationInterceptor({
            "build_chunks": [chunks1, chunks2],
            "embedding": [(token_count1, vector_size1)],
            ...
        })

        # Intercept a method call
        result = interceptor.intercept("build_chunks")  # Returns chunks1
        result = interceptor.intercept("build_chunks")  # Returns chunks2
    """

    def __init__(self, recorded_values: Dict[str, List[Any]]):
        """Initialize the interceptor with pre-recorded values.

        Args:
            recorded_values: A dictionary where keys are method names and
                values are lists of pre-recorded return values. Each call
                to intercept() will pop and return the first value from
                the corresponding list.

        Note:
            If a key from ALLOWED_METHOD_NAMES is not in recorded_values,
            it will be initialized with an empty list. This allows the
            interceptor to be created even if not all methods have recorded
            values, and it will fall through to original execution when
            no recorded values are available.
        """
        self._recorded_values: Dict[str, List[Any]] = dict()
        for key in ALLOWED_METHOD_NAMES:
            self._recorded_values[key] = list(recorded_values.get(key, []))

    def intercept(self, method_name: str, default_value=_NO_DEFAULT) -> Any:
        """Intercept a method call and return the next pre-recorded value.

        Args:
            method_name: Name of the method being intercepted.
            default_value: default value

        Returns:
            The next pre-recorded return value for this method.

        Raises:
            ValueError: If method_name is not in the allowed method names set.
            KeyError: If method_name has no recorded values list.
            IndexError: If the recorded values list for method_name is empty.
        """
        if method_name not in ALLOWED_METHOD_NAMES:
            raise ValueError(f"Cannot intercept method '{method_name}'. Allowed method names: {ALLOWED_METHOD_NAMES}")

        if method_name not in self._recorded_values:
            raise KeyError(f"No recorded values found for method '{method_name}'")

        values_list = self._recorded_values[method_name]
        if not values_list:
            if default_value is not _NO_DEFAULT:
                logging.info(f"return default value for {method_name}")
                return default_value
            raise IndexError(f"No more recorded values for method '{method_name}'")

        return values_list.pop(0)

    def remaining_count(self, method_name: str) -> int:
        """Get the number of remaining recorded values for a method.

        Args:
            method_name: Name of the method to check.

        Returns:
            Number of remaining recorded values.
        """
        if method_name not in self._recorded_values:
            return 0
        return len(self._recorded_values[method_name])

    def remaining_values(self):
        return {k: list(v) for k, v in self._recorded_values.items()}

    def remaining_values_count(self):
        return sum(len(values) for values in self._recorded_values.values())

    def __repr__(self) -> str:
        return f"WriteOperationInterceptor(total_recorded={self._recorded_values})"

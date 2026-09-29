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
Unit tests for constants module.
"""

import pytest
from rag.svr.task_executor_refactor.constants import CANVAS_DEBUG_DOC_ID


class TestConstants:
    """Tests for constants module."""

    def test_canvas_debug_doc_id_exists(self):
        """Test that CANVAS_DEBUG_DOC_ID constant exists."""
        assert CANVAS_DEBUG_DOC_ID is not None

    @pytest.mark.parametrize("expected_type", [str])
    def test_canvas_debug_doc_id_type(self, expected_type):
        """Test that CANVAS_DEBUG_DOC_ID is a string."""
        assert isinstance(CANVAS_DEBUG_DOC_ID, expected_type)

    @pytest.mark.parametrize("expected_value", ["dataflow_x"])
    def test_canvas_debug_doc_id_value(self, expected_value):
        """Test that CANVAS_DEBUG_DOC_ID has expected value."""
        assert CANVAS_DEBUG_DOC_ID == expected_value

    def test_canvas_debug_doc_id_not_empty(self):
        """Test that CANVAS_DEBUG_DOC_ID is not empty."""
        assert len(CANVAS_DEBUG_DOC_ID) > 0

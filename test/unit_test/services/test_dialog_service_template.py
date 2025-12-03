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
Template for testing RAGFlow services following owner's feedback.

This demonstrates the correct approach:
1. Instantiate real service classes (don't mock them)
2. Mock only external dependencies (database, APIs, file system)
3. Test actual business logic that exists in the codebase

Use this as a template for testing other services.
"""

import pytest
from unittest.mock import Mock, patch, MagicMock
import sys

# =============================================================================
# STEP 1: Mock all heavy dependencies BEFORE any RAGFlow imports
# =============================================================================
sys.modules['nltk'] = MagicMock()
sys.modules['nltk.tokenize'] = MagicMock()
sys.modules['nltk.corpus'] = MagicMock()
sys.modules['nltk.stem'] = MagicMock()
sys.modules['tiktoken'] = MagicMock()
sys.modules['transformers'] = MagicMock()
sys.modules['torch'] = MagicMock()
sys.modules['agentic_reasoning'] = MagicMock()
sys.modules['langfuse'] = MagicMock()
sys.modules['trio'] = MagicMock()

# =============================================================================
# STEP 2: Mock database connection with context manager support
# =============================================================================
mock_db = MagicMock()
mock_db.connect = Mock()
mock_db.close = Mock()
mock_db.execute_sql = Mock()
# Make atomic() work as context manager
mock_db.atomic.return_value.__enter__ = Mock(return_value=None)
mock_db.atomic.return_value.__exit__ = Mock(return_value=None)
# Make connection_context() work as context manager
mock_db.connection_context.return_value.__enter__ = Mock(return_value=None)
mock_db.connection_context.return_value.__exit__ = Mock(return_value=None)

# =============================================================================
# STEP 3: Import RAGFlow modules with mocked dependencies
# =============================================================================
with patch('api.db.db_models.DB', mock_db):
    from api.db.services.dialog_service import (
        DialogService,
        meta_filter,
        repair_bad_citation_formats,
        convert_conditions
    )
    from common.constants import StatusEnum


# =============================================================================
# STEP 4: Write test class
# =============================================================================
class TestDialogServiceTemplate:
    """
    Template test class demonstrating correct testing approach.
    
    Key principles:
    - Test real service instance (DialogService)
    - Mock only database model (Dialog)
    - Test actual business logic functions directly
    """

    @pytest.fixture(autouse=True)
    def setup_database_mocks(self):
        """
        Setup database mocks for all tests.
        
        This mocks the Dialog model to avoid actual database operations
        while allowing us to test the service's business logic.
        """
        with patch('api.db.db_models.Dialog') as mock_dialog_model:
            # Create a mock instance that will be returned by the model
            mock_dialog_instance = MagicMock()
            mock_dialog_instance.id = "test_dialog_id"
            mock_dialog_instance.name = "Test Dialog"
            mock_dialog_instance.save = Mock(return_value=mock_dialog_instance)
            
            # Setup model class methods
            mock_dialog_model.return_value = mock_dialog_instance
            mock_dialog_model.get = Mock(return_value=mock_dialog_instance)
            mock_dialog_model.select = Mock()
            mock_dialog_model.update = Mock()
            mock_dialog_model.delete = Mock()
            
            # Replace the service's model with our mock
            DialogService.model = mock_dialog_model
            
            yield mock_dialog_model

    # =========================================================================
    # Test Service Methods (Database Operations)
    # =========================================================================

    def test_save_method_calls_database_correctly(self, setup_database_mocks):
        """
        Test that save() method correctly calls the database.
        
        The actual implementation is:
            sample_obj = cls.model(**kwargs).save(force_insert=True)
        
        We verify:
        1. Model is instantiated with correct parameters
        2. save() is called with force_insert=True
        """
        # Arrange
        dialog_data = {
            "name": "Test Dialog",
            "tenant_id": "tenant_123",
            "status": StatusEnum.VALID.value
        }
        
        # Act
        result = DialogService.save(**dialog_data)
        
        # Assert
        setup_database_mocks.assert_called_once_with(**dialog_data)
        setup_database_mocks.return_value.save.assert_called_once_with(force_insert=True)
        assert result is not None

    def test_update_many_by_id_uses_atomic_transaction(self, setup_database_mocks):
        """
        Test that update_many_by_id() uses atomic transaction.
        
        The actual implementation uses:
            with DB.atomic():
                for data in data_list:
                    # update with timestamps
        
        We verify the atomic transaction is used.
        """
        # Arrange
        data_list = [
            {"id": "1", "name": "Updated 1"},
            {"id": "2", "name": "Updated 2"}
        ]
        
        # Mock the update chain
        mock_query = MagicMock()
        mock_query.where.return_value = mock_query
        mock_query.execute.return_value = 1
        setup_database_mocks.update.return_value = mock_query
        
        # Act
        DialogService.update_many_by_id(data_list)
        
        # Assert
        assert setup_database_mocks.update.call_count == 2
        mock_db.atomic.assert_called_once()

    # =========================================================================
    # Test Business Logic Functions
    # =========================================================================

    def test_meta_filter_with_and_logic(self):
        """
        Test meta_filter() function with AND logic.
        
        This tests the actual business logic for metadata filtering.
        The function should return documents that match ALL filters.
        """
        # Arrange
        metas = {
            "category": {
                "technology": ["doc1", "doc2", "doc3"],
                "business": ["doc2", "doc4"]
            },
            "status": {
                "published": ["doc1", "doc3"],
                "draft": ["doc2", "doc4"]
            }
        }
        
        filters = [
            {"key": "category", "op": "=", "value": "technology"},
            {"key": "status", "op": "=", "value": "published"}
        ]
        
        # Act
        result = meta_filter(metas, filters, logic="and")
        
        # Assert - should return intersection (docs that are technology AND published)
        expected = ["doc1", "doc3"]
        assert sorted(result) == sorted(expected)

    def test_meta_filter_with_or_logic(self):
        """
        Test meta_filter() function with OR logic.
        
        The function should return documents that match ANY filter.
        """
        # Arrange
        metas = {
            "category": {
                "technology": ["doc1", "doc2"],
                "business": ["doc3", "doc4"]
            }
        }
        
        filters = [
            {"key": "category", "op": "=", "value": "technology"},
            {"key": "category", "op": "=", "value": "business"}
        ]
        
        # Act
        result = meta_filter(metas, filters, logic="or")
        
        # Assert - should return union (docs that are technology OR business)
        expected = ["doc1", "doc2", "doc3", "doc4"]
        assert sorted(result) == sorted(expected)

    def test_meta_filter_comparison_operators(self):
        """
        Test meta_filter() with various comparison operators.
        
        Tests: >, <, contains, empty, etc.
        """
        # Test greater than operator
        metas = {
            "price": {
                "10.99": ["doc1"],
                "25.50": ["doc2"],
                "100.00": ["doc3"]
            }
        }
        
        filters = [{"key": "price", "op": ">", "value": "20"}]
        result = meta_filter(metas, filters, logic="and")
        expected = ["doc2", "doc3"]  # Prices > 20
        assert sorted(result) == sorted(expected)
        
        # Test contains operator
        metas = {
            "name": {
                "Apple iPhone": ["doc1"],
                "Samsung Galaxy": ["doc2"],
                "Google Pixel": ["doc3"]
            }
        }
        
        filters = [{"key": "name", "op": "contains", "value": "Galaxy"}]
        result = meta_filter(metas, filters, logic="and")
        assert result == ["doc2"]

    def test_convert_conditions_transforms_operators(self):
        """
        Test convert_conditions() function.
        
        This function transforms metadata conditions from UI format
        to internal format, including operator mapping.
        """
        # Arrange
        metadata_condition = {
            "conditions": [
                {
                    "name": "category",
                    "comparison_operator": "is",
                    "value": "technology"
                },
                {
                    "name": "status",
                    "comparison_operator": "not is",
                    "value": "draft"
                }
            ]
        }
        
        # Act
        result = convert_conditions(metadata_condition)
        
        # Assert
        expected = [
            {"op": "=", "key": "category", "value": "technology"},
            {"op": "≠", "key": "status", "value": "draft"}
        ]
        assert result == expected

    def test_convert_conditions_handles_empty_input(self):
        """Test convert_conditions() handles None and empty inputs."""
        # Test None input
        result = convert_conditions(None)
        assert result == []
        
        # Test empty conditions
        result = convert_conditions({"conditions": []})
        assert result == []

    def test_repair_bad_citation_formats_standardizes_citations(self):
        """
        Test repair_bad_citation_formats() function.
        
        This function finds various citation formats and standardizes them
        to [ID:x] format while tracking which document indices are referenced.
        """
        # Arrange
        kbinfos = {
            "chunks": [
                {"doc_id": "doc1", "content": "Content 1"},
                {"doc_id": "doc2", "content": "Content 2"}
            ]
        }
        
        answer = "According to research (ID: 1), this is important."
        idx = set()
        
        # Act
        repaired_answer, final_idx = repair_bad_citation_formats(answer, kbinfos, idx)
        
        # Assert
        assert "[ID:1]" in repaired_answer  # Standardized format
        assert 1 in final_idx  # Tracked the reference

    def test_repair_bad_citation_formats_handles_out_of_bounds(self):
        """Test citation repair handles invalid indices gracefully."""
        # Arrange
        kbinfos = {
            "chunks": [
                {"doc_id": "doc1", "content": "Content 1"}
            ]
        }
        
        answer = "This references (ID: 999) which doesn't exist."
        idx = set()
        
        # Act
        repaired_answer, final_idx = repair_bad_citation_formats(answer, kbinfos, idx)
        
        # Assert - should not crash, should not add invalid index
        assert isinstance(repaired_answer, str)
        assert 999 not in final_idx

    # =========================================================================
    # Test Edge Cases
    # =========================================================================

    def test_meta_filter_with_no_matching_documents(self):
        """Test meta_filter returns empty list when no documents match."""
        metas = {
            "category": {
                "technology": ["doc1", "doc2"]
            }
        }
        
        filters = [{"key": "category", "op": "=", "value": "nonexistent"}]
        result = meta_filter(metas, filters, logic="and")
        
        assert result == []

    def test_meta_filter_with_empty_filters(self):
        """Test meta_filter with empty filter list."""
        metas = {
            "category": {
                "technology": ["doc1", "doc2"]
            }
        }
        
        filters = []
        result = meta_filter(metas, filters, logic="and")
        
        # With no filters, should return empty (no documents match nothing)
        assert result == []

    # =========================================================================
    # Parameterized Tests
    # =========================================================================

    @pytest.mark.parametrize("operator,value,expected_docs", [
        (">", "50", ["doc3"]),  # Greater than (only 75 > 50)
        ("<", "50", ["doc1"]),  # Less than (only 25 < 50)
        ("≥", "50", ["doc2", "doc3"]),  # Greater than or equal (50 and 75)
        ("≤", "50", ["doc1", "doc2"]),  # Less than or equal (25 and 50)
        ("=", "50", ["doc2"]),  # Equal (only 50)
        ("≠", "50", ["doc1", "doc3"]),  # Not equal (25 and 75)
    ])
    def test_meta_filter_numeric_operators(self, operator, value, expected_docs):
        """Test all numeric comparison operators."""
        metas = {
            "score": {
                "25": ["doc1"],
                "50": ["doc2"],
                "75": ["doc3"]
            }
        }
        
        filters = [{"key": "score", "op": operator, "value": value}]
        result = meta_filter(metas, filters, logic="and")
        
        assert sorted(result) == sorted(expected_docs)


# =============================================================================
# How to run these tests:
# =============================================================================
# cd /root/74/ragflow
# python -m pytest test/unit_test/services/test_dialog_service_template.py -v
#
# Run with coverage:
# python -m pytest test/unit_test/services/test_dialog_service_template.py --cov=api.db.services.dialog_service -v
#
# Run specific test:
# python -m pytest test/unit_test/services/test_dialog_service_template.py::TestDialogServiceTemplate::test_meta_filter_with_and_logic -v
# =============================================================================

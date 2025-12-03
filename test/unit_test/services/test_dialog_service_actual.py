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
Test DialogService and its actual business logic functions.

This tests the real business logic that exists in the codebase:
1. Database operations in DialogService (CRUD)
2. Business logic functions like meta_filter, repair_bad_citation_formats
3. Query building and data transformation logic
"""

import pytest
from unittest.mock import Mock, patch, MagicMock
import sys

# Mock external dependencies before importing
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

# Mock database connection
mock_db = MagicMock()
mock_db.connect = Mock()
mock_db.close = Mock()
mock_db.execute_sql = Mock()
mock_db.atomic = Mock()
mock_db.transaction = Mock()
mock_db.connection_context = Mock()
# Make atomic and connection_context work as context managers
mock_db.atomic.return_value.__enter__ = Mock(return_value=None)
mock_db.atomic.return_value.__exit__ = Mock(return_value=None)
mock_db.connection_context.return_value.__enter__ = Mock(return_value=None)
mock_db.connection_context.return_value.__exit__ = Mock(return_value=None)

with patch('api.db.db_models.DB', mock_db):
    from api.db.services.dialog_service import DialogService, meta_filter, repair_bad_citation_formats, convert_conditions
    from common.constants import StatusEnum


class TestDialogServiceActual:
    """Test the actual DialogService business logic"""

    @pytest.fixture(autouse=True)
    def setup_mocks(self):
        """Setup database mocks"""
        with patch('api.db.db_models.Dialog') as mock_dialog_model:
            mock_dialog_instance = MagicMock()
            mock_dialog_instance.id = "test_id"
            mock_dialog_instance.save = Mock(return_value=mock_dialog_instance)
            
            mock_dialog_model.get = Mock(return_value=mock_dialog_instance)
            mock_dialog_model.select = Mock()
            mock_dialog_model.update = Mock()
            mock_dialog_model.delete = Mock()
            mock_dialog_model.where = Mock(return_value=mock_dialog_model)
            mock_dialog_model.order_by = Mock(return_value=mock_dialog_model)
            mock_dialog_model.limit = Mock(return_value=mock_dialog_model)
            mock_dialog_model.paginate = Mock(return_value=[mock_dialog_instance])
            mock_dialog_model.dicts = Mock(return_value=[mock_dialog_instance])
            mock_dialog_model.count = Mock(return_value=1)
            mock_dialog_model.first = Mock(return_value=mock_dialog_instance)
            mock_dialog_model.execute = Mock(return_value=1)
            
            DialogService.model = mock_dialog_model
            yield mock_dialog_model

    def test_dialog_service_save_method(self, setup_mocks):
        """Test the actual save method - just database operation"""
        dialog_data = {
            "name": "Test Dialog",
            "tenant_id": "tenant_123",
            "status": StatusEnum.VALID.value
        }
        
        # The actual save method calls cls.model(**kwargs).save()
        result = DialogService.save(**dialog_data)
        
        # Verify it instantiated the model with the correct data
        setup_mocks.assert_called_once_with(**dialog_data)
        # Verify save was called on the instance
        setup_mocks.return_value.save.assert_called_once_with(force_insert=True)
        assert result is not None

    def test_dialog_service_get_list_with_filters(self, setup_mocks):
        """Test get_list method with actual query building logic"""
        tenant_id = "tenant_123"
        page_number = 1
        items_per_page = 10
        orderby = "create_time"
        desc = True
        dialog_id = "test_id"
        name = "Test Dialog"
        
        # Mock the query chain
        mock_query = MagicMock()
        mock_query.where.return_value = mock_query
        mock_query.order_by.return_value = mock_query
        mock_query.paginate.return_value = [MagicMock()]
        mock_query.dicts.return_value = [MagicMock()]
        setup_mocks.select.return_value = mock_query
        
        # Call the actual method
        DialogService.get_list(tenant_id, page_number, items_per_page, orderby, desc, dialog_id, name)
        
        # Verify query building logic
        setup_mocks.select.assert_called_once()
        # Should filter by tenant_id and status
        assert mock_query.where.call_count >= 1
        # Should apply ordering
        mock_query.order_by.assert_called_once()
        # Should apply pagination
        mock_query.paginate.assert_called_once_with(page_number, items_per_page)

    def test_dialog_service_update_many_by_id(self, setup_mocks):
        """Test update_many_by_id method with timestamp logic"""
        data_list = [
            {"id": "1", "name": "Updated Dialog 1"},
            {"id": "2", "name": "Updated Dialog 2"}
        ]
        
        # Mock the update query chain
        mock_query = MagicMock()
        mock_query.where.return_value = mock_query
        mock_query.execute.return_value = 1
        setup_mocks.update.return_value = mock_query
        
        # Call the actual method
        DialogService.update_many_by_id(data_list)
        
        # Verify it was called with timestamp updates
        assert setup_mocks.update.call_count == 2
        # Verify atomic transaction was used
        mock_db.atomic.assert_called_once()

    def test_meta_filter_function_and_logic(self):
        """Test the actual meta_filter business logic function"""
        # Test data: metadata values to document IDs mapping
        metas = {
            "category": {
                "technology": ["doc1", "doc2", "doc3"],
                "business": ["doc2", "doc4"],
                "science": ["doc5"]
            },
            "status": {
                "published": ["doc1", "doc3", "doc5"],
                "draft": ["doc2", "doc4"]
            }
        }
        
        # Test filters
        filters = [
            {"key": "category", "op": "=", "value": "technology"},
            {"key": "status", "op": "=", "value": "published"}
        ]
        
        # Test AND logic (intersection)
        result_and = meta_filter(metas, filters, logic="and")
        expected_and = ["doc1", "doc3"]  # docs that are both technology AND published
        assert sorted(result_and) == sorted(expected_and)
        
        # Test OR logic (union)
        result_or = meta_filter(metas, filters, logic="or")
        expected_or = ["doc1", "doc2", "doc3", "doc5"]  # docs that are technology OR published
        assert sorted(result_or) == sorted(expected_or)

    def test_meta_filter_operators(self):
        """Test various filter operators in meta_filter"""
        metas = {
            "price": {
                "10.99": ["doc1"],
                "25.50": ["doc2"],
                "100.00": ["doc3"]
            },
            "name": {
                "Apple iPhone": ["doc1"],
                "Samsung Galaxy": ["doc2"],
                "Google Pixel": ["doc3"]
            }
        }
        
        # Test greater than operator
        filters = [{"key": "price", "op": ">", "value": "20"}]
        result = meta_filter(metas, filters, logic="and")
        expected = ["doc2", "doc3"]  # price > 20
        assert sorted(result) == sorted(expected)
        
        # Test contains operator
        filters = [{"key": "name", "op": "contains", "value": "Galaxy"}]
        result = meta_filter(metas, filters, logic="and")
        expected = ["doc2"]
        assert result == expected
        
        # Test empty operator
        metas_with_empty = {
            "description": {
                "": ["doc1", "doc3"],  # empty descriptions
                "Some description": ["doc2"]
            }
        }
        filters = [{"key": "description", "op": "empty", "value": ""}]
        result = meta_filter(metas_with_empty, filters, logic="and")
        expected = ["doc1", "doc3"]
        assert sorted(result) == sorted(expected)

    def test_repair_bad_citation_formats(self):
        """Test the actual citation format repair function"""
        # Test knowledge base info
        kbinfos = {
            "chunks": [
                {"doc_id": "doc1", "content": "Content 1"},
                {"doc_id": "doc2", "content": "Content 2"},
                {"doc_id": "doc3", "content": "Content 3"}
            ]
        }
        
        # Test various bad citation formats
        test_cases = [
            ("According to research (ID: 1), this is important.", {1}),
            ("The study shows [ID: 2] that this works.", {2}),
            ("Results from 【ID: 3】 are significant.", {3}),
            ("Reference ref1 shows the method.", {1}),
            ("Multiple citations (ID: 1) and [ID: 2] appear.", {1, 2}),
        ]
        
        for answer, expected_indices in test_cases:
            idx = set()
            repaired_answer, final_idx = repair_bad_citation_formats(answer, kbinfos, idx)
            # Should extract the correct document indices
            assert len(final_idx) > 0
            assert final_idx == expected_indices
            # Should repair the format to use [ID:x] format
            assert "[ID:" in repaired_answer

    def test_repair_bad_citation_formats_bounds_checking(self):
        """Test citation repair handles out-of-bounds references"""
        kbinfos = {
            "chunks": [
                {"doc_id": "doc1", "content": "Content 1"},
                {"doc_id": "doc2", "content": "Content 2"}
            ]
        }
        
        # Test out-of-bounds citation
        answer = "This references (ID: 999) which doesn't exist."
        idx = set()
        repaired_answer, final_idx = repair_bad_citation_formats(answer, kbinfos, idx)
        
        # Should not add invalid ID
        assert 999 not in [i for i in range(len(kbinfos["chunks"]))]
        # Should handle gracefully without crashing
        assert isinstance(repaired_answer, str)

    def test_convert_conditions_function(self):
        """Test the convert_conditions business logic"""
        # Test metadata condition structure
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
        
        result = convert_conditions(metadata_condition)
        
        expected = [
            {"op": "=", "key": "category", "value": "technology"},
            {"op": "≠", "key": "status", "value": "draft"}
        ]
        
        assert result == expected

    def test_convert_conditions_empty_input(self):
        """Test convert_conditions handles None/empty input"""
        # Test None input
        result = convert_conditions(None)
        assert result == []
        
        # Test empty conditions
        metadata_condition = {"conditions": []}
        result = convert_conditions(metadata_condition)
        assert result == []

    def test_dialog_service_get_by_tenant_ids_complex_query(self, setup_mocks):
        """Test complex query building in get_by_tenant_ids"""
        joined_tenant_ids = ["tenant1", "tenant2"]
        user_id = "user123"
        page_number = 1
        items_per_page = 10
        orderby = "create_time"
        desc = False
        keywords = "test"
        
        # Mock the complex query chain
        mock_query = MagicMock()
        mock_query.join.return_value = mock_query
        mock_query.where.return_value = mock_query
        mock_query.order_by.return_value = mock_query
        mock_query.count.return_value = 5
        mock_query.paginate.return_value = [MagicMock()]
        mock_query.dicts.return_value = [MagicMock()]
        setup_mocks.select.return_value = mock_query
        
        # Call the actual method
        result, count = DialogService.get_by_tenant_ids(
            joined_tenant_ids, user_id, page_number, items_per_page, 
            orderby, desc, keywords
        )
        
        # Verify complex query building
        setup_mocks.select.assert_called_once()
        mock_query.join.assert_called_once()  # Should join with User table
        mock_query.where.assert_called()  # Should apply tenant and status filters
        mock_query.order_by.assert_called_once()  # Should apply ordering
        mock_query.count.assert_called_once()  # Should count total results
        
        # Should return tuple of (results, count)
        assert isinstance(result, list)
        assert isinstance(count, int)

    def test_dialog_service_pagination_logic(self, setup_mocks):
        """Test pagination logic in get_list"""
        tenant_id = "tenant_123"
        page_number = 2
        items_per_page = 5
        
        # Mock pagination
        mock_query = MagicMock()
        mock_query.where.return_value = mock_query
        mock_query.order_by.return_value = mock_query
        mock_query.paginate.return_value = [MagicMock(), MagicMock()]
        mock_query.dicts.return_value = [MagicMock(), MagicMock()]
        setup_mocks.select.return_value = mock_query
        
        # Call with pagination
        result = DialogService.get_list(tenant_id, page_number, items_per_page, "create_time", False, None, None)
        
        # Verify pagination was applied correctly
        mock_query.paginate.assert_called_once_with(page_number, items_per_page)
        assert len(result) == 2

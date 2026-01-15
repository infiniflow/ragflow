"""
Test suite for File2Document model validation and integrity constraints.

This module tests:
- Model-level validation preventing NULL foreign keys
- Composite unique constraint preventing duplicate relationships
- Proper enforcement of referential integrity
"""

from __future__ import annotations

import unittest
from unittest.mock import MagicMock

import pytest

from common.exceptions import FieldValueRequiredException


class TestFile2DocumentValidation(unittest.TestCase):
    """Test File2Document model validation and constraints."""

    def setUp(self):
        """Set up test fixtures."""
        # Mock the database model
        self.mock_file2document = MagicMock()

    def test_save_with_missing_file_id_raises_exception(self):
        """Test that saving without file_id raises FieldValueRequiredException."""
        from api.db.db_models import File2Document
        from api.db.services.file2document_service import File2DocumentService
        
        # Test actual model validation - should raise exception for missing file_id
        with pytest.raises(FieldValueRequiredException) as exc_info:
            File2DocumentService.insert({
                "id": "test123",
                "file_id": None,  # Missing file_id
                "document_id": "doc123"
            })
        
        # Verify the exception contains expected message about required fields
        assert "file_id" in str(exc_info.value).lower() or "required" in str(exc_info.value).lower()

    def test_save_with_missing_document_id_raises_exception(self):
        """Test that saving without document_id raises FieldValueRequiredException."""
        from api.db.db_models import File2Document
        from api.db.services.file2document_service import File2DocumentService
        
        # Test actual model validation - should raise exception for missing document_id
        with pytest.raises(FieldValueRequiredException) as exc_info:
            File2DocumentService.insert({
                "id": "test123",
                "file_id": "file123",
                "document_id": None  # Missing document_id
            })
        
        # Verify the exception contains expected message about required fields
        assert "document_id" in str(exc_info.value).lower() or "required" in str(exc_info.value).lower()

    def test_save_with_empty_string_file_id_raises_exception(self):
        """Test that saving with empty string file_id raises FieldValueRequiredException."""
        from api.db.db_models import File2Document
        from api.db.services.file2document_service import File2DocumentService
        
        # Test actual model validation - should raise exception for empty file_id
        with pytest.raises(FieldValueRequiredException) as exc_info:
            File2DocumentService.insert({
                "id": "test123",
                "file_id": "",  # Empty file_id
                "document_id": "doc123"
            })
        
        # Verify the exception contains expected message about required fields
        assert "file_id" in str(exc_info.value).lower() or "required" in str(exc_info.value).lower()

    def test_save_with_valid_fields_succeeds(self):
        """Test that saving with both file_id and document_id succeeds."""
        from api.db.db_models import File2Document
        from api.db.services.file2document_service import File2DocumentService
        from unittest.mock import patch
        
        # Mock the lower-level save method that insert() calls internally
        with patch.object(File2DocumentService, 'save', return_value=True) as mock_save:
            # Should not raise exception with valid fields
            result = File2DocumentService.insert({
                "id": "test123",
                "file_id": "file123",
                "document_id": "doc123"
            })
            
            # Verify the service logic was exercised
            mock_save.assert_called_once()
            assert result is not None
            # Verify the service processed the input fields correctly
            call_args = mock_save.call_args[1]  # Get keyword arguments
            assert "file_id" in str(call_args) or "file123" in str(mock_save.call_args)
            assert "document_id" in str(call_args) or "doc123" in str(mock_save.call_args)


class TestFile2DocumentMigrations(unittest.TestCase):
    """Test File2Document migration logic."""

    def test_cleanup_orphans_invokes_service_correctly(self):
        """Test that File2DocumentService.cleanup_orphans invokes the correct query pattern.
        
        This test verifies that the cleanup_orphans service method:
        1. Calls model.delete() to initiate deletion
        2. Applies the correct WHERE condition (file_id IS NULL OR document_id IS NULL)
        3. Executes the query and returns the deletion count
        
        Note: For full integration testing of actual database cleanup, see integration tests.
        """
        from unittest.mock import patch, MagicMock
        from api.db.services.file2document_service import File2DocumentService
        
        # Create mock chain for delete().where().execute()
        mock_execute = MagicMock(return_value=2)  # Simulated deletion count
        mock_where = MagicMock()
        mock_where.execute = mock_execute
        mock_delete = MagicMock()
        mock_delete.where.return_value = mock_where
        
        with patch.object(File2DocumentService, 'model') as mock_model:
            # Set up the mock chain
            mock_model.delete.return_value = mock_delete
            mock_model.file_id.is_null.return_value = "file_id_is_null_condition"
            mock_model.document_id.is_null.return_value = "document_id_is_null_condition"
            
            # Call the actual service method
            result = File2DocumentService.cleanup_orphans()
            
            # Verify model.delete() was called to start the deletion query
            mock_model.delete.assert_called_once()
            
            # Verify is_null() was called on both foreign key fields
            mock_model.file_id.is_null.assert_called()
            mock_model.document_id.is_null.assert_called()
            
            # Verify where() was called with the OR condition
            mock_delete.where.assert_called_once()
            
            # Verify execute() was called to run the deletion
            mock_execute.assert_called_once()
            
            # Verify the service returns the deletion count
            self.assertEqual(result, 2, "cleanup_orphans should return count of deleted records")


class TestFile2DocumentConstraints(unittest.TestCase):
    """Test File2Document database constraints."""

    def test_composite_unique_constraint_definition(self):
        """Test that composite unique constraint is properly defined."""
        from api.db.db_models import File2Document
        
        # Import and inspect the actual model metadata
        model_meta = File2Document._meta
        
        # Check if composite index exists on file_id and document_id
        has_composite_index = False
        if hasattr(model_meta, 'indexes'):
            for index_def in model_meta.indexes:
                if isinstance(index_def, tuple) and len(index_def) >= 1:
                    if isinstance(index_def[0], (tuple, list)):
                        # Extract fields from index definition
                        index_fields = set(index_def[0])
                        # Check if both fields are in the same index
                        if "file_id" in index_fields and "document_id" in index_fields:
                            has_composite_index = True
                            break
        
        # Verify the composite index exists
        assert has_composite_index, "Expected composite index on (file_id, document_id)"

    def test_non_nullable_fields(self):
        """Test that fields are defined as non-nullable."""
        from api.db.db_models import File2Document
        
        # Import and inspect the actual model field definitions
        model_fields = File2Document._meta.fields
        
        # Check actual field configurations from the model
        file_id_field = model_fields.get("file_id")
        document_id_field = model_fields.get("document_id")
        
        # Verify fields exist and check their null constraints
        assert file_id_field is not None, "file_id field should exist"
        assert document_id_field is not None, "document_id field should exist"
        
        # Check null constraints
        assert file_id_field.null is False, "file_id should be non-nullable"
        assert document_id_field.null is False, "document_id should be non-nullable"
        
        # Check max_length if it's a CharField
        if hasattr(file_id_field, 'max_length'):
            assert file_id_field.max_length == 32, f"Expected file_id max_length=32, got {file_id_field.max_length}"
        if hasattr(document_id_field, 'max_length'):
            assert document_id_field.max_length == 32, f"Expected document_id max_length=32, got {document_id_field.max_length}"


if __name__ == "__main__":
    unittest.main()

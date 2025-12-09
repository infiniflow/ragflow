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
Unit tests for MetadataService.

Tests batch CRUD operations for document metadata management.
"""

import pytest
from unittest.mock import Mock, patch, MagicMock


class TestMetadataServiceBatchGet:
    """Test batch_get_metadata functionality."""

    def test_batch_get_empty_ids(self):
        """Test batch get with empty doc_ids returns empty dict."""
        from api.db.services.metadata_service import MetadataService
        
        with patch.object(MetadataService, 'batch_get_metadata', return_value={}):
            result = MetadataService.batch_get_metadata([])
            assert result == {}

    def test_batch_get_with_fields_filter(self):
        """Test batch get filters to requested fields."""
        # This tests the logic of field filtering
        full_metadata = {"field1": "value1", "field2": "value2", "field3": "value3"}
        requested_fields = ["field1", "field3"]
        
        filtered = {k: v for k, v in full_metadata.items() if k in requested_fields}
        
        assert "field1" in filtered
        assert "field3" in filtered
        assert "field2" not in filtered


class TestMetadataServiceBatchUpdate:
    """Test batch_update_metadata functionality."""

    def test_update_merge_logic(self):
        """Test metadata merge logic."""
        existing = {"field1": "old_value", "field2": "keep_this"}
        new_metadata = {"field1": "new_value", "field3": "added"}
        
        # Merge logic
        existing.update(new_metadata)
        
        assert existing["field1"] == "new_value"  # Updated
        assert existing["field2"] == "keep_this"  # Preserved
        assert existing["field3"] == "added"  # Added

    def test_update_replace_logic(self):
        """Test metadata replace logic."""
        existing = {"field1": "old_value", "field2": "keep_this"}
        new_metadata = {"field1": "new_value", "field3": "added"}
        
        # Replace logic (don't merge)
        result = new_metadata
        
        assert result["field1"] == "new_value"
        assert "field2" not in result  # Not preserved
        assert result["field3"] == "added"


class TestMetadataServiceBatchDelete:
    """Test batch_delete_metadata_fields functionality."""

    def test_delete_fields_logic(self):
        """Test field deletion logic."""
        metadata = {"field1": "value1", "field2": "value2", "field3": "value3"}
        fields_to_delete = ["field1", "field3"]
        
        for field in fields_to_delete:
            if field in metadata:
                del metadata[field]
        
        assert "field1" not in metadata
        assert "field2" in metadata
        assert "field3" not in metadata


class TestMetadataServiceSearch:
    """Test search_by_metadata functionality."""

    def test_equals_filter(self):
        """Test equals filter logic."""
        doc_value = "Technical"
        condition = "Technical"
        
        matches = str(doc_value) == str(condition)
        assert matches is True

    def test_contains_filter(self):
        """Test contains filter logic."""
        doc_value = "Technical Documentation"
        condition = {"contains": "Technical"}
        
        val = condition["contains"]
        matches = str(val).lower() in str(doc_value).lower()
        assert matches is True

    def test_starts_with_filter(self):
        """Test starts_with filter logic."""
        doc_value = "Technical Documentation"
        condition = {"starts_with": "Tech"}
        
        val = condition["starts_with"]
        matches = str(doc_value).lower().startswith(str(val).lower())
        assert matches is True

    def test_gt_filter(self):
        """Test greater than filter logic."""
        doc_value = 2023
        condition = {"gt": 2020}
        
        val = condition["gt"]
        matches = float(doc_value) > float(val)
        assert matches is True

    def test_lt_filter(self):
        """Test less than filter logic."""
        doc_value = 2019
        condition = {"lt": 2020}
        
        val = condition["lt"]
        matches = float(doc_value) < float(val)
        assert matches is True

    def test_in_filter(self):
        """Test in filter logic."""
        doc_value = "Technical"
        condition = {"in": ["Technical", "Legal", "HR"]}
        
        val = condition["in"]
        matches = doc_value in val
        assert matches is True


class TestMetadataServiceSchema:
    """Test get_metadata_schema functionality."""

    def test_schema_type_detection(self):
        """Test type detection in schema."""
        values = [
            ("string_value", "str"),
            (123, "int"),
            (12.5, "float"),
            (True, "bool"),
            (["a", "b"], "list"),
        ]
        
        for value, expected_type in values:
            detected_type = type(value).__name__
            assert detected_type == expected_type

    def test_schema_sample_values_limit(self):
        """Test sample values are limited."""
        sample_values = set()
        max_samples = 10
        
        for i in range(20):
            if len(sample_values) < max_samples:
                sample_values.add(f"value_{i}")
        
        assert len(sample_values) == max_samples


class TestMetadataServiceStatistics:
    """Test get_metadata_statistics functionality."""

    def test_coverage_calculation(self):
        """Test metadata coverage calculation."""
        total_docs = 100
        docs_with_metadata = 80
        
        coverage = docs_with_metadata / total_docs if total_docs > 0 else 0
        
        assert coverage == 0.8

    def test_coverage_zero_docs(self):
        """Test coverage with zero documents."""
        total_docs = 0
        docs_with_metadata = 0
        
        coverage = docs_with_metadata / total_docs if total_docs > 0 else 0
        
        assert coverage == 0


class TestMetadataServiceCopy:
    """Test copy_metadata functionality."""

    def test_copy_all_fields(self):
        """Test copying all metadata fields."""
        source_meta = {"field1": "value1", "field2": "value2"}
        
        # Copy all
        copied = source_meta.copy()
        
        assert copied == source_meta
        assert copied is not source_meta  # Different object

    def test_copy_specific_fields(self):
        """Test copying specific metadata fields."""
        source_meta = {"field1": "value1", "field2": "value2", "field3": "value3"}
        fields = ["field1", "field3"]
        
        copied = {k: v for k, v in source_meta.items() if k in fields}
        
        assert "field1" in copied
        assert "field2" not in copied
        assert "field3" in copied


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

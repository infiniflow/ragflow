"""
Tests for RAGFlow schema conversion.

This module tests:
- RAGFlowSchemaConverter: Analyzes ES mappings and generates OB column definitions
- RAGFlowDataConverter: Converts ES documents to OceanBase row format
- Vector field pattern matching
- Schema constants
"""

import json
from es_ob_migration.schema import (
    RAGFlowSchemaConverter,
    RAGFlowDataConverter,
    RAGFLOW_COLUMNS,
    ARRAY_COLUMNS,
    JSON_COLUMNS,
    VECTOR_FIELD_PATTERN,
    FTS_COLUMNS_ORIGIN,
    FTS_COLUMNS_TKS,
)


class TestRAGFlowSchemaConverter:
    """Test RAGFlowSchemaConverter class."""

    def test_analyze_ragflow_mapping(self):
        """Test analyzing a RAGFlow ES mapping."""
        converter = RAGFlowSchemaConverter()
        
        # Simulate a RAGFlow ES mapping
        es_mapping = {
            "properties": {
                "id": {"type": "keyword"},
                "kb_id": {"type": "keyword"},
                "doc_id": {"type": "keyword"},
                "docnm_kwd": {"type": "keyword"},
                "content_with_weight": {"type": "text"},
                "content_ltks": {"type": "text"},
                "available_int": {"type": "integer"},
                "important_kwd": {"type": "keyword"},
                "q_768_vec": {"type": "dense_vector", "dims": 768},
            }
        }
        
        analysis = converter.analyze_es_mapping(es_mapping)
        
        # Check known fields
        assert "id" in analysis["known_fields"]
        assert "kb_id" in analysis["known_fields"]
        assert "content_with_weight" in analysis["known_fields"]
        
        # Check vector fields
        assert len(analysis["vector_fields"]) == 1
        assert analysis["vector_fields"][0]["name"] == "q_768_vec"
        assert analysis["vector_fields"][0]["dimension"] == 768

    def test_detect_vector_size(self):
        """Test automatic vector size detection."""
        converter = RAGFlowSchemaConverter()
        
        es_mapping = {
            "properties": {
                "q_1536_vec": {"type": "dense_vector", "dims": 1536},
            }
        }
        
        converter.analyze_es_mapping(es_mapping)
        
        assert converter.detected_vector_size == 1536

    def test_unknown_fields(self):
        """Test that unknown fields are properly identified."""
        converter = RAGFlowSchemaConverter()
        
        es_mapping = {
            "properties": {
                "id": {"type": "keyword"},
                "custom_field": {"type": "text"},
                "another_field": {"type": "integer"},
            }
        }
        
        analysis = converter.analyze_es_mapping(es_mapping)
        
        assert "custom_field" in analysis["unknown_fields"]
        assert "another_field" in analysis["unknown_fields"]

    def test_get_column_definitions(self):
        """Test getting RAGFlow column definitions."""
        converter = RAGFlowSchemaConverter()
        
        # First analyze to detect vector fields
        es_mapping = {
            "properties": {
                "q_768_vec": {"type": "dense_vector", "dims": 768},
            }
        }
        converter.analyze_es_mapping(es_mapping)
        
        columns = converter.get_column_definitions()
        
        # Check that all RAGFlow columns are present
        column_names = [c["name"] for c in columns]
        
        for col_name in RAGFLOW_COLUMNS:
            assert col_name in column_names, f"Missing column: {col_name}"
        
        # Check vector column is added
        assert "q_768_vec" in column_names


class TestRAGFlowDataConverter:
    """Test RAGFlowDataConverter class."""

    def test_convert_basic_document(self):
        """Test converting a basic RAGFlow document."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "test-id-123",
            "_source": {
                "id": "test-id-123",
                "kb_id": "kb-001",
                "doc_id": "doc-001",
                "docnm_kwd": "test_document.pdf",
                "content_with_weight": "This is test content",
                "available_int": 1,
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert row["id"] == "test-id-123"
        assert row["kb_id"] == "kb-001"
        assert row["doc_id"] == "doc-001"
        assert row["docnm_kwd"] == "test_document.pdf"
        assert row["content_with_weight"] == "This is test content"
        assert row["available_int"] == 1

    def test_convert_with_vector(self):
        """Test converting document with vector embedding."""
        converter = RAGFlowDataConverter()
        
        embedding = [0.1] * 768
        es_doc = {
            "_id": "vec-doc-001",
            "_source": {
                "id": "vec-doc-001",
                "kb_id": "kb-001",
                "q_768_vec": embedding,
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert row["id"] == "vec-doc-001"
        assert row["q_768_vec"] == embedding
        assert "q_768_vec" in converter.vector_fields

    def test_convert_array_fields(self):
        """Test converting array fields."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "array-doc",
            "_source": {
                "id": "array-doc",
                "kb_id": "kb-001",
                "important_kwd": ["keyword1", "keyword2", "keyword3"],
                "question_kwd": ["What is this?", "How does it work?"],
                "tag_kwd": ["tag1", "tag2"],
            }
        }
        
        row = converter.convert_document(es_doc)
        
        # Array fields should be JSON strings
        assert isinstance(row["important_kwd"], str)
        parsed = json.loads(row["important_kwd"])
        assert parsed == ["keyword1", "keyword2", "keyword3"]

    def test_convert_json_fields(self):
        """Test converting JSON fields."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "json-doc",
            "_source": {
                "id": "json-doc",
                "kb_id": "kb-001",
                "tag_feas": {"tag1": 0.8, "tag2": 0.5},
                "metadata": {"author": "John", "date": "2024-01-01"},
            }
        }
        
        row = converter.convert_document(es_doc)
        
        # JSON fields should be JSON strings
        assert isinstance(row["tag_feas"], str)
        assert isinstance(row["metadata"], str)
        
        tag_feas = json.loads(row["tag_feas"])
        assert tag_feas == {"tag1": 0.8, "tag2": 0.5}

    def test_convert_unknown_fields_to_extra(self):
        """Test that unknown fields are stored in 'extra'."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "extra-doc",
            "_source": {
                "id": "extra-doc",
                "kb_id": "kb-001",
                "custom_field": "custom_value",
                "another_custom": 123,
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert "extra" in row
        extra = json.loads(row["extra"])
        assert extra["custom_field"] == "custom_value"
        assert extra["another_custom"] == 123

    def test_convert_kb_id_list(self):
        """Test converting kb_id when it's a list (ES format)."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "kb-list-doc",
            "_source": {
                "id": "kb-list-doc",
                "kb_id": ["kb-001", "kb-002"],  # Some ES docs have list
            }
        }
        
        row = converter.convert_document(es_doc)
        
        # Should take first element
        assert row["kb_id"] == "kb-001"

    def test_convert_content_with_weight_dict(self):
        """Test converting content_with_weight when it's a dict."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "content-dict-doc",
            "_source": {
                "id": "content-dict-doc",
                "kb_id": "kb-001",
                "content_with_weight": {
                    "text": "Some content",
                    "weight": 1.0,
                },
            }
        }
        
        row = converter.convert_document(es_doc)
        
        # Dict should be JSON serialized
        assert isinstance(row["content_with_weight"], str)
        parsed = json.loads(row["content_with_weight"])
        assert parsed["text"] == "Some content"

    def test_convert_batch(self):
        """Test batch conversion."""
        converter = RAGFlowDataConverter()
        
        es_docs = [
            {"_id": f"doc-{i}", "_source": {"id": f"doc-{i}", "kb_id": "kb-001"}}
            for i in range(5)
        ]
        
        rows = converter.convert_batch(es_docs)
        
        assert len(rows) == 5
        for i, row in enumerate(rows):
            assert row["id"] == f"doc-{i}"


class TestVectorFieldPattern:
    """Test vector field pattern matching."""

    def test_valid_patterns(self):
        """Test valid vector field patterns."""
        valid_names = [
            "q_768_vec",
            "q_1024_vec",
            "q_1536_vec",
            "q_3072_vec",
        ]
        
        for name in valid_names:
            match = VECTOR_FIELD_PATTERN.match(name)
            assert match is not None, f"Should match: {name}"

    def test_invalid_patterns(self):
        """Test invalid vector field patterns."""
        invalid_names = [
            "q_vec",
            "768_vec",
            "q_768",
            "vector_768",
            "content_with_weight",
        ]
        
        for name in invalid_names:
            match = VECTOR_FIELD_PATTERN.match(name)
            assert match is None, f"Should not match: {name}"

    def test_extract_dimension(self):
        """Test extracting dimension from pattern."""
        match = VECTOR_FIELD_PATTERN.match("q_1536_vec")
        assert match is not None
        assert int(match.group("vector_size")) == 1536


class TestConstants:
    """Test schema constants."""

    def test_array_columns(self):
        """Test ARRAY_COLUMNS list."""
        expected = [
            "important_kwd", "question_kwd", "tag_kwd", "source_id",
            "entities_kwd", "position_int", "page_num_int", "top_int"
        ]
        
        for col in expected:
            assert col in ARRAY_COLUMNS, f"Missing array column: {col}"

    def test_json_columns(self):
        """Test JSON_COLUMNS list."""
        expected = ["tag_feas", "metadata", "extra"]
        
        for col in expected:
            assert col in JSON_COLUMNS, f"Missing JSON column: {col}"

    def test_ragflow_columns_completeness(self):
        """Test that RAGFLOW_COLUMNS has all required fields."""
        required_fields = [
            "id", "kb_id", "doc_id", "content_with_weight",
            "available_int", "metadata", "extra",
        ]
        
        for field in required_fields:
            assert field in RAGFLOW_COLUMNS, f"Missing required field: {field}"

    def test_fts_columns(self):
        """Test fulltext search column lists."""
        assert "content_with_weight" in FTS_COLUMNS_ORIGIN
        assert "content_ltks" in FTS_COLUMNS_TKS

    def test_ragflow_columns_types(self):
        """Test column type definitions."""
        # Primary key
        assert RAGFLOW_COLUMNS["id"]["is_primary"] is True
        assert RAGFLOW_COLUMNS["id"]["nullable"] is False
        
        # Indexed columns
        assert RAGFLOW_COLUMNS["kb_id"]["index"] is True
        assert RAGFLOW_COLUMNS["doc_id"]["index"] is True
        
        # Array columns
        assert RAGFLOW_COLUMNS["important_kwd"]["is_array"] is True
        assert RAGFLOW_COLUMNS["question_kwd"]["is_array"] is True
        
        # JSON columns
        assert RAGFLOW_COLUMNS["metadata"]["is_json"] is True
        assert RAGFLOW_COLUMNS["extra"]["is_json"] is True


class TestRAGFlowSchemaConverterEdgeCases:
    """Test edge cases for RAGFlowSchemaConverter."""

    def test_empty_mapping(self):
        """Test analyzing empty mapping."""
        converter = RAGFlowSchemaConverter()
        
        analysis = converter.analyze_es_mapping({})
        
        assert analysis["known_fields"] == []
        assert analysis["vector_fields"] == []
        assert analysis["unknown_fields"] == []

    def test_mapping_without_properties(self):
        """Test mapping without properties key."""
        converter = RAGFlowSchemaConverter()
        
        analysis = converter.analyze_es_mapping({"some_other_key": {}})
        
        assert analysis["known_fields"] == []

    def test_multiple_vector_fields(self):
        """Test detecting multiple vector fields."""
        converter = RAGFlowSchemaConverter()
        
        es_mapping = {
            "properties": {
                "q_768_vec": {"type": "dense_vector", "dims": 768},
                "q_1024_vec": {"type": "dense_vector", "dims": 1024},
            }
        }
        
        analysis = converter.analyze_es_mapping(es_mapping)
        
        assert len(analysis["vector_fields"]) == 2
        # First detected should be set
        assert converter.detected_vector_size in [768, 1024]

    def test_get_column_definitions_without_analysis(self):
        """Test getting columns without prior analysis."""
        converter = RAGFlowSchemaConverter()
        
        columns = converter.get_column_definitions()
        
        # Should have all RAGFlow columns but no vector columns
        column_names = [c["name"] for c in columns]
        assert "id" in column_names
        assert "kb_id" in column_names

    def test_get_vector_fields(self):
        """Test getting vector fields."""
        converter = RAGFlowSchemaConverter()
        
        es_mapping = {
            "properties": {
                "q_1536_vec": {"type": "dense_vector", "dims": 1536},
            }
        }
        converter.analyze_es_mapping(es_mapping)
        
        vec_fields = converter.get_vector_fields()
        
        assert len(vec_fields) == 1
        assert vec_fields[0]["name"] == "q_1536_vec"
        assert vec_fields[0]["dimension"] == 1536


class TestRAGFlowDataConverterEdgeCases:
    """Test edge cases for RAGFlowDataConverter."""

    def test_convert_empty_document(self):
        """Test converting empty document."""
        converter = RAGFlowDataConverter()
        
        es_doc = {"_id": "empty_doc", "_source": {}}
        row = converter.convert_document(es_doc)
        
        assert row["id"] == "empty_doc"

    def test_convert_document_without_source(self):
        """Test converting document without _source."""
        converter = RAGFlowDataConverter()
        
        es_doc = {"_id": "no_source", "id": "no_source", "kb_id": "kb_001"}
        row = converter.convert_document(es_doc)
        
        assert row["id"] == "no_source"
        assert row["kb_id"] == "kb_001"

    def test_convert_boolean_to_integer(self):
        """Test converting boolean to integer."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "bool_doc",
            "_source": {
                "id": "bool_doc",
                "kb_id": "kb_001",
                "available_int": True,
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert row["available_int"] == 1

    def test_convert_invalid_integer(self):
        """Test converting invalid integer value."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "invalid_int",
            "_source": {
                "id": "invalid_int",
                "kb_id": "kb_001",
                "available_int": "not_a_number",
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert row["available_int"] is None

    def test_convert_float_field(self):
        """Test converting float fields."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "float_doc",
            "_source": {
                "id": "float_doc",
                "kb_id": "kb_001",
                "weight_flt": 0.85,
                "rank_flt": "0.95",  # String that should become float
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert row["weight_flt"] == 0.85
        assert row["rank_flt"] == 0.95

    def test_convert_array_with_special_characters(self):
        """Test converting array with special characters."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "special_array",
            "_source": {
                "id": "special_array",
                "kb_id": "kb_001",
                "important_kwd": ["key\nwith\nnewlines", "key\twith\ttabs"],
            }
        }
        
        row = converter.convert_document(es_doc)
        
        # Should be JSON string with escaped characters
        assert isinstance(row["important_kwd"], str)
        parsed = json.loads(row["important_kwd"])
        assert len(parsed) == 2

    def test_convert_already_json_array(self):
        """Test converting already JSON-encoded array."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "json_array",
            "_source": {
                "id": "json_array",
                "kb_id": "kb_001",
                "important_kwd": '["already", "json"]',
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert row["important_kwd"] == '["already", "json"]'

    def test_convert_single_value_to_array(self):
        """Test converting single value to array."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "single_to_array",
            "_source": {
                "id": "single_to_array",
                "kb_id": "kb_001",
                "important_kwd": "single_keyword",
            }
        }
        
        row = converter.convert_document(es_doc)
        
        parsed = json.loads(row["important_kwd"])
        assert parsed == ["single_keyword"]

    def test_detect_vector_fields_from_document(self):
        """Test detecting vector fields from document."""
        converter = RAGFlowDataConverter()
        
        doc = {
            "q_768_vec": [0.1] * 768,
            "q_1024_vec": [0.2] * 1024,
        }
        
        converter.detect_vector_fields(doc)
        
        assert "q_768_vec" in converter.vector_fields
        assert "q_1024_vec" in converter.vector_fields

    def test_convert_with_default_values(self):
        """Test conversion uses default values."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "default_test",
            "_source": {
                "id": "default_test",
                "kb_id": "kb_001",
                # available_int not provided, should get default
            }
        }
        
        row = converter.convert_document(es_doc)
        
        # available_int has default of 1
        assert row.get("available_int") == 1

    def test_convert_list_content(self):
        """Test converting list content to JSON."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "list_content",
            "_source": {
                "id": "list_content",
                "kb_id": "kb_001",
                "content_with_weight": ["part1", "part2", "part3"],
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert isinstance(row["content_with_weight"], str)
        parsed = json.loads(row["content_with_weight"])
        assert parsed == ["part1", "part2", "part3"]

    def test_convert_batch_empty(self):
        """Test batch conversion with empty list."""
        converter = RAGFlowDataConverter()
        
        rows = converter.convert_batch([])
        
        assert rows == []

    def test_existing_extra_field_merged(self):
        """Test that existing extra field is merged with unknown fields."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "merge_extra",
            "_source": {
                "id": "merge_extra",
                "kb_id": "kb_001",
                "extra": {"existing_key": "existing_value"},
                "custom_field": "custom_value",
            }
        }
        
        row = converter.convert_document(es_doc)
        
        # extra should contain both existing and new fields
        extra = json.loads(row["extra"])
        assert "custom_field" in extra

"""
Tests for RAGFlow schema conversion.

RAGFlow uses dynamic templates based on field naming patterns.
This module tests the pattern-based type detection and data conversion.
"""

import json
import pytest
from es_ob_migration.schema import (
    RAGFlowSchemaConverter,
    RAGFlowDataConverter,
    VECTOR_FIELD_PATTERN,
    FIELD_PATTERNS,
    INDEX_COLUMNS,
    FTS_COLUMNS,
    get_ob_type,
)


class TestGetObType:
    """Test get_ob_type function."""

    def test_vector_field(self):
        """Test vector field type detection."""
        assert get_ob_type("q_768_vec") == "VECTOR"
        assert get_ob_type("q_1024_vec") == "VECTOR"
        assert get_ob_type("q_1536_vec") == "VECTOR"

    def test_integer_fields(self):
        """Test integer field patterns."""
        assert get_ob_type("page_num_int") == "INTEGER"
        assert get_ob_type("position_int") == "INTEGER"
        assert get_ob_type("top_int") == "INTEGER"
        assert get_ob_type("available_int") == "INTEGER"

    def test_long_fields(self):
        """Test long field patterns."""
        assert get_ob_type("count_long") == "BIGINT"
        assert get_ob_type("size_ulong") == "BIGINT UNSIGNED"
        assert get_ob_type("value_short") == "SMALLINT"

    def test_float_fields(self):
        """Test float field patterns."""
        assert get_ob_type("weight_flt") == "DOUBLE"
        assert get_ob_type("score_flt") == "DOUBLE"
        assert get_ob_type("create_timestamp_flt") == "DOUBLE"

    def test_text_fields(self):
        """Test text field patterns."""
        assert get_ob_type("title_tks") == "TEXT"
        assert get_ob_type("content_ltks") == "LONGTEXT"
        assert get_ob_type("content_sm_ltks") == "LONGTEXT"
        assert get_ob_type("content_with_weight") == "LONGTEXT"

    def test_keyword_fields(self):
        """Test keyword field patterns."""
        assert get_ob_type("kb_id") == "VARCHAR(256)"
        assert get_ob_type("doc_id") == "VARCHAR(256)"
        assert get_ob_type("docnm_kwd") == "VARCHAR(256)"
        assert get_ob_type("img_id") == "VARCHAR(256)"

    def test_date_fields(self):
        """Test date field patterns."""
        assert get_ob_type("create_time") == "VARCHAR(32)"
        assert get_ob_type("update_dt") == "VARCHAR(32)"
        assert get_ob_type("created_at") == "VARCHAR(32)"

    def test_json_fields(self):
        """Test JSON field patterns."""
        assert get_ob_type("data_nst") == "JSON"
        assert get_ob_type("config_obj") == "JSON"
        assert get_ob_type("tag_feas") == "JSON"
        assert get_ob_type("lat_lon") == "JSON"

    def test_rank_feature(self):
        """Test rank_feature pattern."""
        assert get_ob_type("pagerank_fea") == "DOUBLE"

    def test_binary_fields(self):
        """Test binary field patterns."""
        assert get_ob_type("data_bin") == "LONGBLOB"



class TestRAGFlowSchemaConverter:
    """Test RAGFlowSchemaConverter class."""

    def test_analyze_real_mapping(self):
        """Test analyzing a real RAGFlow ES mapping."""
        converter = RAGFlowSchemaConverter()
        
        # Simplified real mapping
        es_mapping = {
            "properties": {
                "content_ltks": {"type": "text"},
                "content_with_weight": {"type": "text"},
                "create_time": {"type": "date"},
                "doc_id": {"type": "keyword"},
                "kb_id": {"type": "keyword"},
                "lat_lon": {"type": "geo_point"},
                "page_num_int": {"type": "integer"},
                "q_1024_vec": {"type": "dense_vector", "dims": 1024},
            }
        }
        
        result = converter.analyze_es_mapping(es_mapping)
        
        # Should include all fields + id
        assert len(result["fields"]) == 9  # 8 from mapping + id
        assert "id" in result["fields"]
        assert "content_ltks" in result["fields"]
        assert "lat_lon" in result["fields"]
        assert len(result["vector_fields"]) == 1
        assert result["vector_fields"][0]["dimension"] == 1024

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

    def test_multiple_vector_fields(self):
        """Test detecting multiple vector fields."""
        converter = RAGFlowSchemaConverter()
        
        es_mapping = {
            "properties": {
                "q_768_vec": {"type": "dense_vector", "dims": 768},
                "q_1024_vec": {"type": "dense_vector", "dims": 1024},
            }
        }
        
        result = converter.analyze_es_mapping(es_mapping)
        
        assert len(result["vector_fields"]) == 2

    def test_empty_mapping(self):
        """Test analyzing empty mapping."""
        converter = RAGFlowSchemaConverter()
        
        result = converter.analyze_es_mapping({})
        
        # Should still have id field
        assert "id" in result["fields"]
        assert result["vector_fields"] == []

    def test_get_column_definitions(self):
        """Test getting column definitions."""
        converter = RAGFlowSchemaConverter()
        
        es_mapping = {
            "properties": {
                "kb_id": {"type": "keyword"},
                "doc_id": {"type": "keyword"},
                "content_ltks": {"type": "text"},
                "q_768_vec": {"type": "dense_vector"},
            }
        }
        converter.analyze_es_mapping(es_mapping)
        
        columns = converter.get_column_definitions()
        
        assert len(columns) == 5  # id + 4 from mapping
        
        # Check id column
        id_col = next(c for c in columns if c["name"] == "id")
        assert id_col["primary_key"] is True

    def test_get_index_columns(self):
        """Test getting indexed columns."""
        converter = RAGFlowSchemaConverter()
        
        es_mapping = {
            "properties": {
                "kb_id": {"type": "keyword"},
                "doc_id": {"type": "keyword"},
                "content_ltks": {"type": "text"},
            }
        }
        converter.analyze_es_mapping(es_mapping)
        
        index_cols = converter.get_index_columns()
        
        assert "kb_id" in index_cols
        assert "doc_id" in index_cols

    def test_get_fts_columns(self):
        """Test getting fulltext columns."""
        converter = RAGFlowSchemaConverter()
        
        es_mapping = {
            "properties": {
                "title_tks": {"type": "text"},
                "content_ltks": {"type": "text"},
                "kb_id": {"type": "keyword"},
            }
        }
        converter.analyze_es_mapping(es_mapping)
        
        fts_cols = converter.get_fts_columns()
        
        assert "title_tks" in fts_cols
        assert "content_ltks" in fts_cols
        assert "kb_id" not in fts_cols


class TestRAGFlowDataConverter:
    """Test RAGFlowDataConverter class."""

    def test_convert_basic_document(self):
        """Test converting a basic document."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "test-id-123",
            "_source": {
                "kb_id": "kb-001",
                "doc_id": "doc-001",
                "docnm_kwd": "test.pdf",
                "content_with_weight": "Test content",
                "available_int": 1,
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert row["id"] == "test-id-123"
        assert row["kb_id"] == "kb-001"
        assert row["content_with_weight"] == "Test content"
        assert row["available_int"] == 1

    def test_convert_with_vector(self):
        """Test converting document with vector."""
        converter = RAGFlowDataConverter()
        
        embedding = [0.1] * 1024
        es_doc = {
            "_id": "vec-doc",
            "_source": {
                "kb_id": "kb-001",
                "q_1024_vec": embedding,
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert row["q_1024_vec"] == embedding
        assert "q_1024_vec" in converter.vector_fields

    def test_convert_geo_point(self):
        """Test converting geo_point field."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "geo-doc",
            "_source": {
                "kb_id": "kb-001",
                "lat_lon": {"lat": 40.7128, "lon": -74.0060},
            }
        }
        
        row = converter.convert_document(es_doc)
        
        # geo_point should be JSON
        assert isinstance(row["lat_lon"], str)
        parsed = json.loads(row["lat_lon"])
        assert parsed["lat"] == 40.7128

    def test_convert_nested_object(self):
        """Test converting nested/object fields."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "nested-doc",
            "_source": {
                "kb_id": "kb-001",
                "data_obj": {"key": "value", "num": 123},
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert isinstance(row["data_obj"], str)
        parsed = json.loads(row["data_obj"])
        assert parsed["key"] == "value"

    def test_convert_rank_features(self):
        """Test converting rank_features field."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "feas-doc",
            "_source": {
                "kb_id": "kb-001",
                "tag_feas": {"tag1": 0.8, "tag2": 0.5},
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert isinstance(row["tag_feas"], str)
        parsed = json.loads(row["tag_feas"])
        assert parsed["tag1"] == 0.8

    def test_convert_kb_id_list(self):
        """Test converting kb_id when it's a list."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "kb-list",
            "_source": {
                "kb_id": ["kb-001", "kb-002"],
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert row["kb_id"] == "kb-001"

    def test_convert_integer_from_bool(self):
        """Test converting boolean to integer."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "bool-doc",
            "_source": {
                "kb_id": "kb-001",
                "available_int": True,
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert row["available_int"] == 1

    def test_convert_float_from_string(self):
        """Test converting string to float."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "float-doc",
            "_source": {
                "kb_id": "kb-001",
                "score_flt": "0.95",
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert row["score_flt"] == 0.95

    def test_convert_batch(self):
        """Test batch conversion."""
        converter = RAGFlowDataConverter()
        
        es_docs = [
            {"_id": f"doc-{i}", "_source": {"kb_id": "kb-001"}}
            for i in range(5)
        ]
        
        rows = converter.convert_batch(es_docs)
        
        assert len(rows) == 5

    def test_convert_empty_batch(self):
        """Test empty batch."""
        converter = RAGFlowDataConverter()
        
        rows = converter.convert_batch([])
        
        assert rows == []

    def test_convert_content_dict(self):
        """Test converting content_with_weight as dict."""
        converter = RAGFlowDataConverter()
        
        es_doc = {
            "_id": "content-doc",
            "_source": {
                "kb_id": "kb-001",
                "content_with_weight": {"text": "content", "weight": 1.0},
            }
        }
        
        row = converter.convert_document(es_doc)
        
        assert isinstance(row["content_with_weight"], str)


class TestVectorFieldPattern:
    """Test vector field pattern matching."""

    def test_valid_patterns(self):
        """Test valid vector field patterns."""
        valid = ["q_512_vec", "q_768_vec", "q_1024_vec", "q_1536_vec", "q_3072_vec"]
        
        for name in valid:
            assert VECTOR_FIELD_PATTERN.match(name), f"Should match: {name}"

    def test_invalid_patterns(self):
        """Test invalid patterns."""
        invalid = ["q_vec", "768_vec", "q_768", "vector", "content_ltks"]
        
        for name in invalid:
            assert not VECTOR_FIELD_PATTERN.match(name), f"Should not match: {name}"

    def test_extract_dimension(self):
        """Test extracting dimension."""
        match = VECTOR_FIELD_PATTERN.match("q_1536_vec")
        assert int(match.group(1)) == 1536


class TestConstants:
    """Test schema constants."""

    def test_index_columns(self):
        """Test INDEX_COLUMNS list."""
        assert "kb_id" in INDEX_COLUMNS
        assert "doc_id" in INDEX_COLUMNS

    def test_fts_columns(self):
        """Test FTS_COLUMNS list."""
        assert "content_ltks" in FTS_COLUMNS
        assert "title_tks" in FTS_COLUMNS

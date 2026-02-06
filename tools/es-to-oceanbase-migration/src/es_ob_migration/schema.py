"""
RAGFlow-specific schema conversion from Elasticsearch to OceanBase.

This module handles the fixed RAGFlow table structure migration.
RAGFlow uses a predefined schema for both ES and OceanBase.
"""

import json
import logging
import re
from typing import Any

logger = logging.getLogger(__name__)


# RAGFlow fixed column definitions (from rag/utils/ob_conn.py)
# These are the actual columns used by RAGFlow
RAGFLOW_COLUMNS = {
    # Primary identifiers
    "id": {"ob_type": "String(256)", "nullable": False, "is_primary": True},
    "kb_id": {"ob_type": "String(256)", "nullable": False, "index": True},
    "doc_id": {"ob_type": "String(256)", "nullable": True, "index": True},
    
    # Document metadata
    "docnm_kwd": {"ob_type": "String(256)", "nullable": True},  # document name
    "doc_type_kwd": {"ob_type": "String(256)", "nullable": True},  # document type
    
    # Title fields
    "title_tks": {"ob_type": "String(256)", "nullable": True},  # title tokens
    "title_sm_tks": {"ob_type": "String(256)", "nullable": True},  # fine-grained title tokens
    
    # Content fields
    "content_with_weight": {"ob_type": "LONGTEXT", "nullable": True},  # original content
    "content_ltks": {"ob_type": "LONGTEXT", "nullable": True},  # long text tokens
    "content_sm_ltks": {"ob_type": "LONGTEXT", "nullable": True},  # fine-grained tokens
    
    # Feature fields
    "pagerank_fea": {"ob_type": "Integer", "nullable": True},  # page rank priority
    
    # Array fields
    "important_kwd": {"ob_type": "ARRAY(String(256))", "nullable": True, "is_array": True},  # keywords
    "important_tks": {"ob_type": "TEXT", "nullable": True},  # keyword tokens
    "question_kwd": {"ob_type": "ARRAY(String(1024))", "nullable": True, "is_array": True},  # questions
    "question_tks": {"ob_type": "TEXT", "nullable": True},  # question tokens
    "tag_kwd": {"ob_type": "ARRAY(String(256))", "nullable": True, "is_array": True},  # tags
    "tag_feas": {"ob_type": "JSON", "nullable": True, "is_json": True},  # tag features
    
    # Status fields
    "available_int": {"ob_type": "Integer", "nullable": False, "default": 1},
    
    # Time fields
    "create_time": {"ob_type": "String(19)", "nullable": True},
    "create_timestamp_flt": {"ob_type": "Double", "nullable": True},
    
    # Image field
    "img_id": {"ob_type": "String(128)", "nullable": True},
    
    # Position fields (arrays)
    "position_int": {"ob_type": "ARRAY(ARRAY(Integer))", "nullable": True, "is_array": True},
    "page_num_int": {"ob_type": "ARRAY(Integer)", "nullable": True, "is_array": True},
    "top_int": {"ob_type": "ARRAY(Integer)", "nullable": True, "is_array": True},
    
    # Knowledge graph fields
    "knowledge_graph_kwd": {"ob_type": "String(256)", "nullable": True, "index": True},
    "source_id": {"ob_type": "ARRAY(String(256))", "nullable": True, "is_array": True},
    "entity_kwd": {"ob_type": "String(256)", "nullable": True},
    "entity_type_kwd": {"ob_type": "String(256)", "nullable": True, "index": True},
    "from_entity_kwd": {"ob_type": "String(256)", "nullable": True},
    "to_entity_kwd": {"ob_type": "String(256)", "nullable": True},
    "weight_int": {"ob_type": "Integer", "nullable": True},
    "weight_flt": {"ob_type": "Double", "nullable": True},
    "entities_kwd": {"ob_type": "ARRAY(String(256))", "nullable": True, "is_array": True},
    "rank_flt": {"ob_type": "Double", "nullable": True},
    
    # Status
    "removed_kwd": {"ob_type": "String(256)", "nullable": True, "index": True, "default": "N"},
    
    # JSON fields
    "metadata": {"ob_type": "JSON", "nullable": True, "is_json": True},
    "extra": {"ob_type": "JSON", "nullable": True, "is_json": True},
    
    # New columns
    "_order_id": {"ob_type": "Integer", "nullable": True},
    "group_id": {"ob_type": "String(256)", "nullable": True},
    "mom_id": {"ob_type": "String(256)", "nullable": True},
}

# Array column names for special handling
ARRAY_COLUMNS = [
    "important_kwd", "question_kwd", "tag_kwd", "source_id", 
    "entities_kwd", "position_int", "page_num_int", "top_int"
]

# JSON column names
JSON_COLUMNS = ["tag_feas", "metadata", "extra"]

# Fulltext search columns (for reference)
FTS_COLUMNS_ORIGIN = ["docnm_kwd", "content_with_weight", "important_tks", "question_tks"]
FTS_COLUMNS_TKS = ["title_tks", "title_sm_tks", "important_tks", "question_tks", "content_ltks", "content_sm_ltks"]

# Vector field pattern: q_{vector_size}_vec
VECTOR_FIELD_PATTERN = re.compile(r"q_(?P<vector_size>\d+)_vec")


class RAGFlowSchemaConverter:
    """
    Convert RAGFlow Elasticsearch documents to OceanBase format.
    
    RAGFlow uses a fixed schema, so this converter knows exactly
    what fields to expect and how to map them.
    """

    def __init__(self):
        self.vector_fields: list[dict[str, Any]] = []
        self.detected_vector_size: int | None = None

    def analyze_es_mapping(self, es_mapping: dict[str, Any]) -> dict[str, Any]:
        """
        Analyze ES mapping to extract vector field dimensions.
        
        Args:
            es_mapping: Elasticsearch index mapping
            
        Returns:
            Analysis result with detected fields
        """
        result = {
            "known_fields": [],
            "vector_fields": [],
            "unknown_fields": [],
        }
        
        properties = es_mapping.get("properties", {})
        
        for field_name, field_def in properties.items():
            # Check if it's a known RAGFlow field
            if field_name in RAGFLOW_COLUMNS:
                result["known_fields"].append(field_name)
            # Check if it's a vector field
            elif VECTOR_FIELD_PATTERN.match(field_name):
                match = VECTOR_FIELD_PATTERN.match(field_name)
                vec_size = int(match.group("vector_size"))
                result["vector_fields"].append({
                    "name": field_name,
                    "dimension": vec_size,
                })
                self.vector_fields.append({
                    "name": field_name,
                    "dimension": vec_size,
                })
                if self.detected_vector_size is None:
                    self.detected_vector_size = vec_size
            else:
                # Unknown field - might be custom field stored in 'extra'
                result["unknown_fields"].append(field_name)
        
        logger.info(
            f"Analyzed ES mapping: {len(result['known_fields'])} known fields, "
            f"{len(result['vector_fields'])} vector fields, "
            f"{len(result['unknown_fields'])} unknown fields"
        )
        
        return result

    def get_column_definitions(self) -> list[dict[str, Any]]:
        """
        Get RAGFlow column definitions for OceanBase table creation.
        
        Returns:
            List of column definitions
        """
        columns = []
        
        for col_name, col_def in RAGFLOW_COLUMNS.items():
            columns.append({
                "name": col_name,
                "ob_type": col_def["ob_type"],
                "nullable": col_def.get("nullable", True),
                "is_primary": col_def.get("is_primary", False),
                "index": col_def.get("index", False),
                "is_array": col_def.get("is_array", False),
                "is_json": col_def.get("is_json", False),
                "default": col_def.get("default"),
            })
        
        # Add detected vector fields
        for vec_field in self.vector_fields:
            columns.append({
                "name": vec_field["name"],
                "ob_type": f"VECTOR({vec_field['dimension']})",
                "nullable": True,
                "is_vector": True,
                "dimension": vec_field["dimension"],
            })
        
        return columns

    def get_vector_fields(self) -> list[dict[str, Any]]:
        """Get list of vector fields for index creation."""
        return self.vector_fields


class RAGFlowDataConverter:
    """
    Convert RAGFlow ES documents to OceanBase row format.
    
    This converter handles the specific data transformations needed
    for RAGFlow's data structure.
    """

    def __init__(self):
        """Initialize data converter."""
        self.vector_fields: set[str] = set()

    def detect_vector_fields(self, doc: dict[str, Any]) -> None:
        """Detect vector fields from a sample document."""
        for key in doc.keys():
            if VECTOR_FIELD_PATTERN.match(key):
                self.vector_fields.add(key)

    def convert_document(self, es_doc: dict[str, Any]) -> dict[str, Any]:
        """
        Convert an ES document to OceanBase row format.
        
        Args:
            es_doc: Elasticsearch document (with _id and _source)
            
        Returns:
            Dictionary ready for OceanBase insertion
        """
        # Extract _id and _source
        doc_id = es_doc.get("_id")
        source = es_doc.get("_source", es_doc)
        
        row = {}
        
        # Set document ID
        if doc_id:
            row["id"] = str(doc_id)
        elif "id" in source:
            row["id"] = str(source["id"])
        
        # Process each field
        for field_name, field_def in RAGFLOW_COLUMNS.items():
            if field_name == "id":
                continue  # Already handled
            
            value = source.get(field_name)
            
            if value is None:
                # Use default if available
                default = field_def.get("default")
                if default is not None:
                    row[field_name] = default
                continue
            
            # Convert based on field type
            row[field_name] = self._convert_field_value(
                field_name, value, field_def
            )
        
        # Handle vector fields
        for key, value in source.items():
            if VECTOR_FIELD_PATTERN.match(key):
                if isinstance(value, list):
                    row[key] = value
                self.vector_fields.add(key)
        
        # Handle unknown fields -> store in 'extra'
        extra_fields = {}
        for key, value in source.items():
            if key not in RAGFLOW_COLUMNS and not VECTOR_FIELD_PATTERN.match(key):
                extra_fields[key] = value
        
        if extra_fields:
            existing_extra = row.get("extra")
            if existing_extra and isinstance(existing_extra, dict):
                existing_extra.update(extra_fields)
            else:
                row["extra"] = json.dumps(extra_fields, ensure_ascii=False)
        
        return row

    def _convert_field_value(
        self, 
        field_name: str, 
        value: Any, 
        field_def: dict[str, Any]
    ) -> Any:
        """
        Convert a field value to the appropriate format for OceanBase.
        
        Args:
            field_name: Field name
            value: Original value from ES
            field_def: Field definition from RAGFLOW_COLUMNS
            
        Returns:
            Converted value
        """
        if value is None:
            return None
        
        ob_type = field_def.get("ob_type", "")
        is_array = field_def.get("is_array", False)
        is_json = field_def.get("is_json", False)
        
        # Handle array fields
        if is_array:
            return self._convert_array_value(value)
        
        # Handle JSON fields
        if is_json:
            return self._convert_json_value(value)
        
        # Handle specific types
        if "Integer" in ob_type:
            return self._convert_integer(value)
        
        if "Double" in ob_type or "Float" in ob_type:
            return self._convert_float(value)
        
        if "LONGTEXT" in ob_type or "TEXT" in ob_type:
            return self._convert_text(value)
        
        if "String" in ob_type:
            return self._convert_string(value, field_name)
        
        # Default: convert to string
        return str(value) if value is not None else None

    def _convert_array_value(self, value: Any) -> str | None:
        """Convert array value to JSON string for OceanBase."""
        if value is None:
            return None
        
        if isinstance(value, str):
            # Already a JSON string
            try:
                # Validate it's valid JSON
                json.loads(value)
                return value
            except json.JSONDecodeError:
                # Not valid JSON, wrap in array
                return json.dumps([value], ensure_ascii=False)
        
        if isinstance(value, list):
            # Clean array values
            cleaned = []
            for item in value:
                if isinstance(item, str):
                    # Clean special characters
                    cleaned_str = item.strip()
                    cleaned_str = cleaned_str.replace('\\', '\\\\')
                    cleaned_str = cleaned_str.replace('\n', '\\n')
                    cleaned_str = cleaned_str.replace('\r', '\\r')
                    cleaned_str = cleaned_str.replace('\t', '\\t')
                    cleaned.append(cleaned_str)
                else:
                    cleaned.append(item)
            return json.dumps(cleaned, ensure_ascii=False)
        
        # Single value - wrap in array
        return json.dumps([value], ensure_ascii=False)

    def _convert_json_value(self, value: Any) -> str | None:
        """Convert JSON value to string for OceanBase."""
        if value is None:
            return None
        
        if isinstance(value, str):
            # Already a string, validate JSON
            try:
                json.loads(value)
                return value
            except json.JSONDecodeError:
                # Not valid JSON, return as-is
                return value
        
        if isinstance(value, (dict, list)):
            return json.dumps(value, ensure_ascii=False)
        
        return str(value)

    def _convert_integer(self, value: Any) -> int | None:
        """Convert to integer."""
        if value is None:
            return None
        
        if isinstance(value, bool):
            return 1 if value else 0
        
        try:
            return int(value)
        except (ValueError, TypeError):
            return None

    def _convert_float(self, value: Any) -> float | None:
        """Convert to float."""
        if value is None:
            return None
        
        try:
            return float(value)
        except (ValueError, TypeError):
            return None

    def _convert_text(self, value: Any) -> str | None:
        """Convert to text/longtext."""
        if value is None:
            return None
        
        if isinstance(value, dict):
            # content_with_weight might be stored as dict
            return json.dumps(value, ensure_ascii=False)
        
        if isinstance(value, list):
            return json.dumps(value, ensure_ascii=False)
        
        return str(value)

    def _convert_string(self, value: Any, field_name: str) -> str | None:
        """Convert to string with length considerations."""
        if value is None:
            return None
        
        # Handle kb_id which might be a list in ES
        if field_name == "kb_id" and isinstance(value, list):
            return str(value[0]) if value else None
        
        if isinstance(value, (dict, list)):
            return json.dumps(value, ensure_ascii=False)
        
        return str(value)

    def convert_batch(self, es_docs: list[dict[str, Any]]) -> list[dict[str, Any]]:
        """
        Convert a batch of ES documents.
        
        Args:
            es_docs: List of Elasticsearch documents
            
        Returns:
            List of dictionaries ready for OceanBase insertion
        """
        return [self.convert_document(doc) for doc in es_docs]


# Backwards compatibility aliases
SchemaConverter = RAGFlowSchemaConverter
DataConverter = RAGFlowDataConverter

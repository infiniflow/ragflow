"""
RAGFlow schema conversion from Elasticsearch to OceanBase.

RAGFlow uses dynamic templates in ES based on field naming patterns.
This module detects field types and converts data accordingly.
"""

import json
import logging
import re
from typing import Any

logger = logging.getLogger(__name__)


# RAGFlow field naming patterns and their OceanBase types
FIELD_PATTERNS = [
    # Vector fields: q_768_vec, q_1024_vec, etc.
    (re.compile(r"^q_(\d+)_vec$"), "VECTOR"),
    
    # Integer types
    (re.compile(r".*_int$"), "INTEGER"),
    (re.compile(r".*_ulong$"), "BIGINT UNSIGNED"),
    (re.compile(r".*_long$"), "BIGINT"),
    (re.compile(r".*_short$"), "SMALLINT"),
    
    # Float types
    (re.compile(r".*_flt$"), "DOUBLE"),
    
    # Text types (fulltext search)
    (re.compile(r".*_tks$"), "TEXT"),
    (re.compile(r".*_ltks$"), "LONGTEXT"),
    (re.compile(r".*_(with_weight|list)$"), "LONGTEXT"),
    
    # Keyword types
    (re.compile(r".*_(kwd|id|ids|uid|uids)$"), "VARCHAR(256)"),
    (re.compile(r"^uid$"), "VARCHAR(256)"),
    
    # Date types
    (re.compile(r".*_(dt|time|at)$"), "VARCHAR(32)"),  # Store as string for compatibility
    
    # JSON types
    (re.compile(r".*_nst$"), "JSON"),  # nested
    (re.compile(r".*_obj$"), "JSON"),  # object
    (re.compile(r".*_fea$"), "DOUBLE"),  # rank_feature -> double
    (re.compile(r".*_feas$"), "JSON"),  # rank_features -> json
    
    # Binary
    (re.compile(r".*_bin$"), "LONGBLOB"),
    
    # Geo point (lat_lon)
    (re.compile(r"^lat_lon$"), "JSON"),
]

# Vector field pattern for dimension extraction
VECTOR_FIELD_PATTERN = re.compile(r"^q_(\d+)_vec$")

# Columns that need regular indexes (B-Tree) for WHERE clause filtering
# Source: ragflow/rag/utils/ob_conn.py:105-110 index_columns
INDEX_COLUMNS = ["kb_id", "doc_id", "available_int", "knowledge_graph_kwd", "entity_type_kwd", "removed_kwd"]

# Columns that need fulltext indexes for text search (MATCH AGAINST)
# Source: ragflow/rag/utils/ob_conn.py:114-128
# - fts_columns_origin: used when SEARCH_ORIGINAL_CONTENT=true (default)
# - fts_columns_tks: used when SEARCH_ORIGINAL_CONTENT=false or HybridSearch enabled
FTS_COLUMNS = [
    # fts_columns_tks
    "title_tks", "title_sm_tks", "important_tks", "question_tks", "content_ltks", "content_sm_ltks",
    # fts_columns_origin (additional)
    "docnm_kwd", "content_with_weight",
]


def get_ob_type(field_name: str) -> str:
    """
    Get OceanBase type for a field based on RAGFlow naming pattern.
    
    Args:
        field_name: ES field name
        
    Returns:
        OceanBase column type string
    """
    for pattern, ob_type in FIELD_PATTERNS:
        if pattern.match(field_name):
            return ob_type
    
    # Default for unknown fields
    return "TEXT"


class RAGFlowSchemaConverter:
    """
    Analyze ES mapping and generate OceanBase column definitions.
    """

    def __init__(self):
        self.fields: dict[str, dict[str, Any]] = {}  # field_name -> {ob_type, ...}
        self.vector_fields: list[dict[str, Any]] = []
        self.detected_vector_size: int | None = None

    def analyze_es_mapping(self, es_mapping: dict[str, Any]) -> dict[str, Any]:
        """
        Analyze ES mapping to detect all fields and their types.
        
        Args:
            es_mapping: Elasticsearch index mapping
            
        Returns:
            Analysis result with fields and vector info
        """
        properties = es_mapping.get("properties", {})
        
        # Always add 'id' field (primary key, from ES _id)
        self.fields["id"] = {
            "ob_type": "VARCHAR(256)",
            "primary_key": True,
            "nullable": False,
        }
        
        for field_name in properties:
            ob_type = get_ob_type(field_name)
            
            field_info = {
                "ob_type": ob_type,
                "primary_key": False,
                "nullable": True,
            }
            
            # Detect vector fields and extract dimension
            match = VECTOR_FIELD_PATTERN.match(field_name)
            if match:
                vec_size = int(match.group(1))
                field_info["vector_dim"] = vec_size
                self.vector_fields.append({
                    "name": field_name,
                    "dimension": vec_size,
                })
                if self.detected_vector_size is None:
                    self.detected_vector_size = vec_size
            
            # Mark indexed columns
            if field_name in INDEX_COLUMNS:
                field_info["index"] = True
            
            # Mark fulltext columns
            if field_name in FTS_COLUMNS:
                field_info["fulltext"] = True
            
            self.fields[field_name] = field_info
        
        logger.info(f"Detected {len(self.fields)} fields, {len(self.vector_fields)} vector fields")
        
        return {
            "fields": self.fields,
            "vector_fields": self.vector_fields,
        }

    def discover_vector_fields(self, documents: list[dict[str, Any]]) -> None:
        """
        Discover vector fields from actual document data.
        
        RAGFlow dynamically creates vector fields (q_768_vec, q_1024_vec, etc.)
        based on the embedding model, which may not be in the ES mapping.
        
        Args:
            documents: Sample documents from ES (with _source)
        """
        for doc in documents:
            source = doc.get("_source", doc)
            for field_name in source:
                # Skip fields already known
                if field_name in self.fields:
                    continue
                
                # Only detect vector fields (other fields should be in ES mapping)
                match = VECTOR_FIELD_PATTERN.match(field_name)
                if match:
                    vec_size = int(match.group(1))
                    field_info = {
                        "ob_type": "VECTOR",
                        "primary_key": False,
                        "nullable": True,
                        "vector_dim": vec_size,
                    }
                    self.vector_fields.append({
                        "name": field_name,
                        "dimension": vec_size,
                    })
                    if self.detected_vector_size is None:
                        self.detected_vector_size = vec_size
                    self.fields[field_name] = field_info
                    logger.info(f"Discovered vector field from data: {field_name} (dim={vec_size})")

    def get_column_definitions(self) -> list[dict[str, Any]]:
        """
        Get column definitions for OceanBase table creation.
        
        Returns:
            List of column definitions
        """
        return [
            {"name": name, **info}
            for name, info in self.fields.items()
        ]

    def get_vector_fields(self) -> list[dict[str, Any]]:
        """Get list of detected vector fields."""
        return self.vector_fields

    def get_index_columns(self) -> list[str]:
        """Get columns that need regular indexes."""
        return [name for name, info in self.fields.items() if info.get("index")]

    def get_fts_columns(self) -> list[str]:
        """Get columns that need fulltext indexes."""
        return [name for name, info in self.fields.items() if info.get("fulltext")]


class RAGFlowDataConverter:
    """
    Convert ES documents to OceanBase row format.
    
    Handles type conversion based on field naming patterns.
    """

    def __init__(self):
        self.vector_fields: set[str] = set()

    def convert_document(self, es_doc: dict[str, Any]) -> dict[str, Any]:
        """
        Convert an ES document to OceanBase row format.
        
        Args:
            es_doc: Elasticsearch document (with _id and _source)
            
        Returns:
            Dictionary ready for OceanBase insertion
        """
        doc_id = es_doc.get("_id")
        source = es_doc.get("_source", es_doc)
        
        row = {}
        
        # Set document ID (from ES _id)
        if doc_id:
            row["id"] = str(doc_id)
        elif "id" in source:
            row["id"] = str(source["id"])
        
        # Fields to exclude from OceanBase (ES metadata, not actual data)
        exclude_fields = {"_id", "_score", "_source"}
        
        # Convert all fields from source
        for field_name, value in source.items():
            # Skip excluded fields and duplicate id
            if field_name in exclude_fields:
                continue
            if field_name == "id" and "id" in row:
                continue
            
            row[field_name] = self._convert_value(field_name, value)
            
            # Track vector fields
            if VECTOR_FIELD_PATTERN.match(field_name):
                self.vector_fields.add(field_name)
        
        return row

    def _convert_value(self, field_name: str, value: Any) -> Any:
        """Convert a value based on field naming pattern."""
        if value is None:
            return None
        
        ob_type = get_ob_type(field_name)
        
        # Vector - keep as list
        if ob_type == "VECTOR":
            return value if isinstance(value, list) else None
        
        # Integer types
        if ob_type in ("INTEGER", "SMALLINT", "TINYINT", "BIGINT", "BIGINT UNSIGNED"):
            return self._to_int(value)
        
        # Float types
        if ob_type == "DOUBLE":
            return self._to_float(value)
        
        # JSON types (nested, object, geo_point, rank_features)
        if ob_type == "JSON":
            return self._to_json(value)
        
        # Text/Longtext
        if ob_type in ("TEXT", "LONGTEXT"):
            return self._to_text(value)
        
        # VARCHAR (keyword types)
        if ob_type.startswith("VARCHAR"):
            return self._to_string(value, field_name)
        
        # Binary
        if ob_type == "LONGBLOB":
            return value
        
        # Default: string
        return str(value) if value is not None else None

    def _to_int(self, value: Any) -> int | None:
        """Convert to integer."""
        if value is None:
            return None
        if isinstance(value, bool):
            return 1 if value else 0
        try:
            return int(value)
        except (ValueError, TypeError):
            return None

    def _to_float(self, value: Any) -> float | None:
        """Convert to float."""
        if value is None:
            return None
        try:
            return float(value)
        except (ValueError, TypeError):
            return None

    def _to_json(self, value: Any) -> str | None:
        """Convert to JSON string for OceanBase JSON column."""
        if value is None:
            return None
        if isinstance(value, str):
            # Check if already a valid JSON string to avoid double serialization
            # e.g., '{"a":1}' should stay as '{"a":1}', not become '"{\"a\":1}"'
            try:
                json.loads(value)
                return value  # Valid JSON string, return as-is
            except json.JSONDecodeError:
                return json.dumps(value, ensure_ascii=False)  # Plain string, serialize it
        # dict/list -> serialize to JSON string
        return json.dumps(value, ensure_ascii=False)

    def _to_text(self, value: Any) -> str | None:
        """Convert to text."""
        if value is None:
            return None
        if isinstance(value, (dict, list)):
            return json.dumps(value, ensure_ascii=False)
        return str(value)

    def _to_string(self, value: Any, field_name: str) -> str | None:
        """Convert to string."""
        if value is None:
            return None
        # Handle kb_id which might be a list
        if field_name == "kb_id" and isinstance(value, list):
            return str(value[0]) if value else None
        if isinstance(value, (dict, list)):
            return json.dumps(value, ensure_ascii=False)
        return str(value)

    def convert_batch(self, es_docs: list[dict[str, Any]]) -> list[dict[str, Any]]:
        """Convert a batch of ES documents."""
        return [self.convert_document(doc) for doc in es_docs]

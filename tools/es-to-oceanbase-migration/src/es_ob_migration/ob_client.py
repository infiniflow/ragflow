"""
OceanBase Client for RAGFlow data migration.

This client is specifically designed for RAGFlow's data structure.
"""

import logging
from typing import Any

from pyobvector import ObVecClient, FtsIndexParam, FtsParser, VECTOR, ARRAY
from sqlalchemy import Column, String, Integer, Float, JSON, Double
from sqlalchemy.dialects.mysql import LONGTEXT, TEXT as MYSQL_TEXT

from .schema import RAGFLOW_COLUMNS, FTS_COLUMNS_TKS

logger = logging.getLogger(__name__)


# Index naming templates (from RAGFlow ob_conn.py)
INDEX_NAME_TEMPLATE = "ix_%s_%s"
FULLTEXT_INDEX_NAME_TEMPLATE = "fts_idx_%s"
VECTOR_INDEX_NAME_TEMPLATE = "%s_idx"

# Columns that need regular indexes
INDEX_COLUMNS = [
    "kb_id",
    "doc_id", 
    "available_int",
    "knowledge_graph_kwd",
    "entity_type_kwd",
    "removed_kwd",
]


class OBClient:
    """OceanBase client wrapper for RAGFlow migration operations."""

    def __init__(
        self,
        host: str = "localhost",
        port: int = 2881,
        user: str = "root",
        password: str = "",
        database: str = "test",
        pool_size: int = 10,
    ):
        """
        Initialize OceanBase client.

        Args:
            host: OceanBase host address
            port: OceanBase port
            user: Database user (format: user@tenant for OceanBase)
            password: Database password
            database: Database name
            pool_size: Connection pool size
        """
        self.host = host
        self.port = port
        self.user = user
        self.password = password
        self.database = database

        # Initialize pyobvector client
        self.uri = f"{host}:{port}"
        self.client = ObVecClient(
            uri=self.uri,
            user=user,
            password=password,
            db_name=database,
            pool_pre_ping=True,
            pool_recycle=3600,
            pool_size=pool_size,
        )
        logger.info(f"Connected to OceanBase at {self.uri}, database: {database}")

    def health_check(self) -> bool:
        """Check database connectivity."""
        try:
            result = self.client.perform_raw_text_sql("SELECT 1 FROM DUAL")
            result.fetchone()
            return True
        except Exception as e:
            logger.error(f"OceanBase health check failed: {e}")
            return False

    def get_version(self) -> str | None:
        """Get OceanBase version."""
        try:
            result = self.client.perform_raw_text_sql("SELECT OB_VERSION() FROM DUAL")
            row = result.fetchone()
            return row[0] if row else None
        except Exception as e:
            logger.warning(f"Failed to get OceanBase version: {e}")
            return None

    def table_exists(self, table_name: str) -> bool:
        """Check if a table exists."""
        try:
            return self.client.check_table_exists(table_name)
        except Exception:
            return False

    def create_ragflow_table(
        self,
        table_name: str,
        vector_size: int = 768,
        create_indexes: bool = True,
        create_fts_indexes: bool = True,
    ):
        """
        Create a RAGFlow-compatible table in OceanBase.
        
        This creates a table with the exact schema that RAGFlow expects,
        including all columns, indexes, and vector columns.

        Args:
            table_name: Name of the table (usually the ES index name)
            vector_size: Vector dimension (e.g., 768, 1024, 1536)
            create_indexes: Whether to create regular indexes
            create_fts_indexes: Whether to create fulltext indexes
        """
        # Build column definitions
        columns = self._build_ragflow_columns()
        
        # Add vector column
        vector_column_name = f"q_{vector_size}_vec"
        columns.append(
            Column(vector_column_name, VECTOR(vector_size), nullable=True,
                   comment=f"vector embedding ({vector_size} dimensions)")
        )
        
        # Table options (from RAGFlow)
        table_options = {
            "mysql_charset": "utf8mb4",
            "mysql_collate": "utf8mb4_unicode_ci",
            "mysql_organization": "heap",
        }
        
        # Create table
        self.client.create_table(
            table_name=table_name,
            columns=columns,
            **table_options,
        )
        logger.info(f"Created table: {table_name}")
        
        # Create regular indexes
        if create_indexes:
            self._create_regular_indexes(table_name)
        
        # Create fulltext indexes
        if create_fts_indexes:
            self._create_fulltext_indexes(table_name)
        
        # Create vector index
        self._create_vector_index(table_name, vector_column_name)
        
        # Refresh metadata
        self.client.refresh_metadata([table_name])

    def _build_ragflow_columns(self) -> list[Column]:
        """Build SQLAlchemy Column objects for RAGFlow schema."""
        columns = []
        
        for col_name, col_def in RAGFLOW_COLUMNS.items():
            ob_type = col_def["ob_type"]
            nullable = col_def.get("nullable", True)
            default = col_def.get("default")
            is_primary = col_def.get("is_primary", False)
            is_array = col_def.get("is_array", False)
            
            # Parse type and create appropriate Column
            col = self._create_column(col_name, ob_type, nullable, default, is_primary, is_array)
            columns.append(col)
        
        return columns

    def _create_column(
        self, 
        name: str, 
        ob_type: str, 
        nullable: bool,
        default: Any,
        is_primary: bool,
        is_array: bool,
    ) -> Column:
        """Create a SQLAlchemy Column object based on type string."""
        
        # Handle array types
        if is_array or ob_type.startswith("ARRAY"):
            # Extract inner type
            if "String" in ob_type:
                inner_type = String(256)
            elif "Integer" in ob_type:
                inner_type = Integer
            else:
                inner_type = String(256)
            
            # Nested array (e.g., ARRAY(ARRAY(Integer)))
            if ob_type.count("ARRAY") > 1:
                return Column(name, ARRAY(ARRAY(inner_type)), nullable=nullable)
            else:
                return Column(name, ARRAY(inner_type), nullable=nullable)
        
        # Handle String types with length
        if ob_type.startswith("String"):
            # Extract length: String(256) -> 256
            import re
            match = re.search(r'\((\d+)\)', ob_type)
            length = int(match.group(1)) if match else 256
            return Column(
                name, String(length), 
                primary_key=is_primary, 
                nullable=nullable,
                server_default=f"'{default}'" if default else None
            )
        
        # Map other types
        type_map = {
            "Integer": Integer,
            "Double": Double,
            "Float": Float,
            "JSON": JSON,
            "LONGTEXT": LONGTEXT,
            "TEXT": MYSQL_TEXT,
        }
        
        for type_name, type_class in type_map.items():
            if type_name in ob_type:
                return Column(
                    name, type_class, 
                    primary_key=is_primary,
                    nullable=nullable,
                    server_default=str(default) if default is not None else None
                )
        
        # Default to String
        return Column(name, String(256), nullable=nullable)

    def _create_regular_indexes(self, table_name: str):
        """Create regular indexes for indexed columns."""
        for col_name in INDEX_COLUMNS:
            index_name = INDEX_NAME_TEMPLATE % (table_name, col_name)
            try:
                self.client.create_index(
                    table_name=table_name,
                    is_vec_index=False,
                    index_name=index_name,
                    column_names=[col_name],
                )
                logger.debug(f"Created index: {index_name}")
            except Exception as e:
                if "Duplicate" in str(e):
                    logger.debug(f"Index {index_name} already exists")
                else:
                    logger.warning(f"Failed to create index {index_name}: {e}")

    def _create_fulltext_indexes(self, table_name: str):
        """Create fulltext indexes for text columns."""
        for fts_column in FTS_COLUMNS_TKS:
            col_name = fts_column.split("^")[0]  # Remove weight suffix
            index_name = FULLTEXT_INDEX_NAME_TEMPLATE % col_name
            try:
                self.client.create_fts_idx_with_fts_index_param(
                    table_name=table_name,
                    fts_idx_param=FtsIndexParam(
                        index_name=index_name,
                        field_names=[col_name],
                        parser_type=FtsParser.IK,
                    ),
                )
                logger.debug(f"Created fulltext index: {index_name}")
            except Exception as e:
                if "Duplicate" in str(e):
                    logger.debug(f"Fulltext index {index_name} already exists")
                else:
                    logger.warning(f"Failed to create fulltext index {index_name}: {e}")

    def _create_vector_index(self, table_name: str, vector_column_name: str):
        """Create vector index for embedding column."""
        index_name = VECTOR_INDEX_NAME_TEMPLATE % vector_column_name
        try:
            self.client.create_index(
                table_name=table_name,
                is_vec_index=True,
                index_name=index_name,
                column_names=[vector_column_name],
                vidx_params="distance=cosine, type=hnsw, lib=vsag",
            )
            logger.info(f"Created vector index: {index_name}")
        except Exception as e:
            if "Duplicate" in str(e):
                logger.debug(f"Vector index {index_name} already exists")
            else:
                logger.warning(f"Failed to create vector index {index_name}: {e}")

    def add_vector_column(self, table_name: str, vector_size: int):
        """Add a vector column to an existing table."""
        vector_column_name = f"q_{vector_size}_vec"
        
        # Check if column exists
        if self._column_exists(table_name, vector_column_name):
            logger.info(f"Vector column {vector_column_name} already exists")
            return
        
        try:
            self.client.add_columns(
                table_name=table_name,
                columns=[Column(vector_column_name, VECTOR(vector_size), nullable=True)],
            )
            logger.info(f"Added vector column: {vector_column_name}")
            
            # Create index
            self._create_vector_index(table_name, vector_column_name)
        except Exception as e:
            logger.error(f"Failed to add vector column: {e}")
            raise

    def _column_exists(self, table_name: str, column_name: str) -> bool:
        """Check if a column exists in a table."""
        try:
            result = self.client.perform_raw_text_sql(
                f"SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS "
                f"WHERE TABLE_SCHEMA = '{self.database}' "
                f"AND TABLE_NAME = '{table_name}' "
                f"AND COLUMN_NAME = '{column_name}'"
            )
            count = result.fetchone()[0]
            return count > 0
        except Exception:
            return False

    def _index_exists(self, table_name: str, index_name: str) -> bool:
        """Check if an index exists."""
        try:
            result = self.client.perform_raw_text_sql(
                f"SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS "
                f"WHERE TABLE_SCHEMA = '{self.database}' "
                f"AND TABLE_NAME = '{table_name}' "
                f"AND INDEX_NAME = '{index_name}'"
            )
            count = result.fetchone()[0]
            return count > 0
        except Exception:
            return False

    def insert_batch(
        self,
        table_name: str,
        documents: list[dict[str, Any]],
    ) -> int:
        """
        Insert a batch of documents using upsert.

        Args:
            table_name: Name of the table
            documents: List of documents to insert

        Returns:
            Number of documents inserted
        """
        if not documents:
            return 0

        try:
            self.client.upsert(table_name=table_name, data=documents)
            return len(documents)
        except Exception as e:
            logger.error(f"Batch insert failed: {e}")
            raise

    def count_rows(self, table_name: str, kb_id: str | None = None) -> int:
        """
        Count rows in a table.
        
        Args:
            table_name: Table name
            kb_id: Optional knowledge base ID filter
        """
        try:
            sql = f"SELECT COUNT(*) FROM `{table_name}`"
            if kb_id:
                sql += f" WHERE kb_id = '{kb_id}'"
            result = self.client.perform_raw_text_sql(sql)
            return result.fetchone()[0]
        except Exception:
            return 0

    def get_sample_rows(
        self, 
        table_name: str, 
        limit: int = 10,
        kb_id: str | None = None,
    ) -> list[dict[str, Any]]:
        """Get sample rows from a table."""
        try:
            sql = f"SELECT * FROM `{table_name}`"
            if kb_id:
                sql += f" WHERE kb_id = '{kb_id}'"
            sql += f" LIMIT {limit}"
            
            result = self.client.perform_raw_text_sql(sql)
            columns = result.keys()
            rows = []
            for row in result:
                rows.append(dict(zip(columns, row)))
            return rows
        except Exception as e:
            logger.error(f"Failed to get sample rows: {e}")
            return []

    def get_row_by_id(self, table_name: str, doc_id: str) -> dict[str, Any] | None:
        """Get a single row by ID."""
        try:
            result = self.client.get(table_name=table_name, ids=[doc_id])
            row = result.fetchone()
            if row:
                columns = result.keys()
                return dict(zip(columns, row))
            return None
        except Exception as e:
            logger.error(f"Failed to get row: {e}")
            return None

    def drop_table(self, table_name: str):
        """Drop a table if exists."""
        try:
            self.client.drop_table_if_exist(table_name)
            logger.info(f"Dropped table: {table_name}")
        except Exception as e:
            logger.warning(f"Failed to drop table: {e}")

    def execute_sql(self, sql: str) -> Any:
        """Execute raw SQL."""
        return self.client.perform_raw_text_sql(sql)

    def close(self):
        """Close the OB client connection."""
        self.client.engine.dispose()
        logger.info("OceanBase connection closed")

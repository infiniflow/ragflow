"""
OceanBase Client for RAGFlow data migration.

Creates tables dynamically based on ES mapping analysis.
"""

import logging
from typing import Any

from pyobvector import ObVecClient, FtsIndexParam, FtsParser, VECTOR, ARRAY
from sqlalchemy import Column, String, Integer, Float, JSON, Text, Double, SmallInteger, BigInteger
from sqlalchemy.dialects.mysql import LONGTEXT, TEXT as MYSQL_TEXT, LONGBLOB, TINYINT

from .schema import RAGFlowSchemaConverter

logger = logging.getLogger(__name__)


# Index naming templates (from RAGFlow ob_conn.py)
INDEX_NAME_TEMPLATE = "ix_%s_%s"
FULLTEXT_INDEX_NAME_TEMPLATE = "fts_idx_%s"
VECTOR_INDEX_NAME_TEMPLATE = "%s_idx"


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

    def create_table_from_schema(
        self,
        table_name: str,
        schema_converter: RAGFlowSchemaConverter,
        create_indexes: bool = True,
        create_fts_indexes: bool = True,
    ):
        """
        Create OceanBase table based on analyzed ES schema.
        
        Args:
            table_name: Name of the table
            schema_converter: Analyzed schema from ES mapping
            create_indexes: Whether to create regular indexes
            create_fts_indexes: Whether to create fulltext indexes
        """
        # Build columns from analyzed schema
        columns = []
        vector_columns = []
        
        for col_def in schema_converter.get_column_definitions():
            name = col_def["name"]
            ob_type = col_def["ob_type"]
            primary_key = col_def.get("primary_key", False)
            nullable = col_def.get("nullable", True)
            
            # Handle vector columns separately
            if ob_type == "VECTOR":
                vec_dim = col_def.get("vector_dim")
                if vec_dim is None:
                    raise ValueError(f"Vector field '{name}' missing dimension")
                vector_columns.append((name, vec_dim))
            else:
                col = self._create_column(name, ob_type, primary_key, nullable)
                if col is not None:
                    columns.append(col)
        
        # Add vector columns
        for vec_name, vec_dim in vector_columns:
            logger.info(f"Adding vector column: {vec_name} with dimension {vec_dim}")
            columns.append(
                Column(vec_name, VECTOR(vec_dim), nullable=True,
                       comment=f"vector embedding ({vec_dim} dimensions)")
            )
        
        # Table options (from RAGFlow)
        table_options = {
            "mysql_charset": "utf8mb4",
            "mysql_collate": "utf8mb4_unicode_ci",
            "mysql_organization": "heap",
        }
        
        # Create table
        try:
            logger.info(f"Creating table {table_name} with {len(columns)} columns (including {len(vector_columns)} vector columns)")
            self.client.create_table(
                table_name=table_name,
                columns=columns,
                **table_options,
            )
            logger.info(f"Created table: {table_name} with {len(columns)} columns")
        except Exception as e:
            logger.error(f"Failed to create table: {e}")
            raise
        
        # Create regular indexes
        if create_indexes:
            for col_name in schema_converter.get_index_columns():
                self._create_index(table_name, col_name)
        
        # Create fulltext indexes
        if create_fts_indexes:
            for col_name in schema_converter.get_fts_columns():
                self._create_fts_index(table_name, col_name)
        
        # Create vector indexes
        for vec_name, _ in vector_columns:
            self._create_vector_index(table_name, vec_name)
        
        # Refresh metadata
        self.client.refresh_metadata([table_name])

    def _create_column(
        self, 
        name: str, 
        ob_type: str, 
        primary_key: bool,
        nullable: bool,
    ) -> Column | None:
        """Create a SQLAlchemy Column object based on type string."""
        
        # Skip vector columns (handled separately)
        if ob_type == "VECTOR":
            return None
        
        # Type mapping
        type_map = {
            "VARCHAR(256)": String(256),
            "VARCHAR(32)": String(32),
            "INTEGER": Integer,
            "SMALLINT": SmallInteger,
            "TINYINT": TINYINT,
            "BIGINT": BigInteger,
            "BIGINT UNSIGNED": BigInteger,
            "DOUBLE": Double,
            "FLOAT": Float,
            "TEXT": MYSQL_TEXT,
            "LONGTEXT": LONGTEXT,
            "JSON": JSON,
            "LONGBLOB": LONGBLOB,
        }
        
        col_type = type_map.get(ob_type, String(256))
        
        return Column(
            name, 
            col_type, 
            primary_key=primary_key,
            nullable=nullable,
        )

    def _create_index(self, table_name: str, col_name: str):
        """Create regular index."""
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

    def _create_fts_index(self, table_name: str, col_name: str):
        """Create fulltext index."""
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

    def get_table_columns(self, table_name: str) -> set[str]:
        """Get column names for a table."""
        try:
            sql = f"""
                SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS 
                WHERE TABLE_SCHEMA = '{self.database}' AND TABLE_NAME = '{table_name}'
            """
            result = self.client.perform_raw_text_sql(sql)
            return {row[0] for row in result.fetchall()}
        except Exception:
            return set()

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
            # Get valid columns for the table
            valid_columns = self.get_table_columns(table_name)
            
            if valid_columns:
                # Filter documents to only include valid columns and non-None values
                filtered_docs = []
                skipped_columns = set()
                for doc in documents:
                    filtered_doc = {}
                    for k, v in doc.items():
                        if k not in valid_columns:
                            skipped_columns.add(k)
                        elif v is not None:
                            # Only include non-None values (OceanBase handles missing columns as NULL)
                            filtered_doc[k] = v
                    filtered_docs.append(filtered_doc)
                
                if skipped_columns:
                    logger.warning(f"Skipped columns not in table schema: {skipped_columns}")
                
                documents = filtered_docs
            
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

    def close(self):
        """Close the OB client connection."""
        self.client.engine.dispose()
        logger.info("OceanBase connection closed")

"""RDBMS (MySQL/PostgreSQL) data source connector for importing data from relational databases."""

import hashlib
import json
import logging
from datetime import datetime, timezone
from enum import Enum
from typing import Any, Dict, Generator, Optional, Union

from common.data_source.config import DocumentSource, INDEX_BATCH_SIZE
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
)
from common.data_source.interfaces import LoadConnector, PollConnector, SecondsSinceUnixEpoch
from common.data_source.models import Document


class DatabaseType(str, Enum):
    """Supported database types."""
    MYSQL = "mysql"
    POSTGRESQL = "postgresql"


class RDBMSConnector(LoadConnector, PollConnector):
    """
    RDBMS connector for importing data from MySQL and PostgreSQL databases.
    
    This connector allows users to:
    1. Connect to a MySQL or PostgreSQL database
    2. Execute a SQL query to extract data
    3. Map columns to content (for vectorization) and metadata
    4. Sync data in batch or incremental mode using a timestamp column
    """
    def __init__(
        self,
        db_type: str,
        host: str,
        port: int,
        database: str,
        query: str,
        content_columns: str,
        metadata_columns: Optional[str] = None,
        id_column: Optional[str] = None,
        timestamp_column: Optional[str] = None,
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        """
        Initialize the RDBMS connector.
        
        Args:
            db_type: Database type ('mysql' or 'postgresql')
            host: Database host
            port: Database port
            database: Database name
            query: SQL query to execute (e.g., "SELECT * FROM products WHERE status = 'active'")
            content_columns: Comma-separated column names to use for document content
            metadata_columns: Comma-separated column names to use as metadata (optional)
            id_column: Column to use as unique document ID (optional, will generate hash if not provided)
            timestamp_column: Column to use for incremental sync (optional, must be datetime/timestamp type)
            batch_size: Number of documents per batch
        """
        self.db_type = DatabaseType(db_type.lower())
        self.host = host.strip()
        self.port = port
        self.database = database.strip()
        self.query = query.strip()
        self.content_columns = [c.strip() for c in content_columns.split(",") if c.strip()]
        self.metadata_columns = [c.strip() for c in (metadata_columns or "").split(",") if c.strip()]
        self.id_column = id_column.strip() if id_column else None
        self.timestamp_column = timestamp_column.strip() if timestamp_column else None
        self.batch_size = batch_size
        
        self._connection = None
        self._credentials: Dict[str, Any] = {}

    def load_credentials(self, credentials: Dict[str, Any]) -> Dict[str, Any] | None:
        """Load database credentials."""
        logging.debug(f"Loading credentials for {self.db_type} database: {self.database}")
        
        required_keys = ["username", "password"]
        for key in required_keys:
            if not credentials.get(key):
                raise ConnectorMissingCredentialError(f"RDBMS ({self.db_type}): missing {key}")
        
        self._credentials = credentials
        return None

    def _get_connection(self):
        """Create and return a database connection."""
        if self._connection is not None:
            return self._connection
            
        username = self._credentials.get("username")
        password = self._credentials.get("password")
        
        if self.db_type == DatabaseType.MYSQL:
            try:
                import mysql.connector
            except ImportError:
                raise ConnectorValidationError(
                    "MySQL connector not installed. Please install mysql-connector-python."
                )
            try:
                self._connection = mysql.connector.connect(
                    host=self.host,
                    port=self.port,
                    database=self.database,
                    user=username,
                    password=password,
                    charset='utf8mb4',
                    use_unicode=True,
                )
            except Exception as e:
                raise ConnectorValidationError(f"Failed to connect to MySQL: {e}")
        elif self.db_type == DatabaseType.POSTGRESQL:
            try:
                import psycopg2
            except ImportError:
                raise ConnectorValidationError(
                    "PostgreSQL connector not installed. Please install psycopg2-binary."
                )
            try:
                self._connection = psycopg2.connect(
                    host=self.host,
                    port=self.port,
                    dbname=self.database,
                    user=username,
                    password=password,
                )
            except Exception as e:
                raise ConnectorValidationError(f"Failed to connect to PostgreSQL: {e}")
        
        return self._connection

    def _close_connection(self):
        """Close the database connection."""
        if self._connection is not None:
            try:
                self._connection.close()
            except Exception:
                pass
            self._connection = None

    def _get_tables(self) -> list[str]:
        """Get list of all tables in the database."""
        connection = self._get_connection()
        cursor = connection.cursor()
        
        try:
            if self.db_type == DatabaseType.MYSQL:
                cursor.execute("SHOW TABLES")
            else:
                cursor.execute(
                    "SELECT table_name FROM information_schema.tables "
                    "WHERE table_schema = 'public' AND table_type = 'BASE TABLE'"
                )
            tables = [row[0] for row in cursor.fetchall()]
            return tables
        finally:
            cursor.close()

    def _build_query_with_time_filter(
        self,
        start: Optional[datetime] = None,
        end: Optional[datetime] = None,
    ) -> str:
        """Build the query with optional time filtering for incremental sync."""
        if not self.query:
            return ""  # Will be handled by table discovery
        base_query = self.query.rstrip(";")
        
        if not self.timestamp_column or (start is None and end is None):
            return base_query
        
        has_where = "where" in base_query.lower()
        connector = " AND" if has_where else " WHERE"
        
        time_conditions = []
        if start is not None:
            if self.db_type == DatabaseType.MYSQL:
                time_conditions.append(f"{self.timestamp_column} > '{start.strftime('%Y-%m-%d %H:%M:%S')}'")
            else:
                time_conditions.append(f"{self.timestamp_column} > '{start.isoformat()}'")
        
        if end is not None:
            if self.db_type == DatabaseType.MYSQL:
                time_conditions.append(f"{self.timestamp_column} <= '{end.strftime('%Y-%m-%d %H:%M:%S')}'")
            else:
                time_conditions.append(f"{self.timestamp_column} <= '{end.isoformat()}'")
        
        if time_conditions:
            return f"{base_query}{connector} {' AND '.join(time_conditions)}"
        
        return base_query

    def _row_to_document(self, row: Union[tuple, list, Dict[str, Any]], column_names: list) -> Document:
        """Convert a database row to a Document."""
        row_dict = dict(zip(column_names, row)) if isinstance(row, (list, tuple)) else row
        
        content_parts = []
        for col in self.content_columns:
            if col in row_dict and row_dict[col] is not None:
                value = row_dict[col]
                if isinstance(value, (dict, list)):
                    value = json.dumps(value, ensure_ascii=False)
                content_parts.append(f"{col}: {value}")
        
        content = "\n".join(content_parts)
        
        if self.id_column and self.id_column in row_dict:
            doc_id = f"{self.db_type}:{self.database}:{row_dict[self.id_column]}"
        else:
            content_hash = hashlib.md5(content.encode()).hexdigest()
            doc_id = f"{self.db_type}:{self.database}:{content_hash}"
        
        metadata = {}
        for col in self.metadata_columns:
            if col in row_dict and row_dict[col] is not None:
                value = row_dict[col]
                if isinstance(value, datetime):
                    value = value.isoformat()
                elif isinstance(value, (dict, list)):
                    value = json.dumps(value, ensure_ascii=False)
                else:
                    value = str(value)
                metadata[col] = value
        
        doc_updated_at = datetime.now(timezone.utc)
        if self.timestamp_column and self.timestamp_column in row_dict:
            ts_value = row_dict[self.timestamp_column]
            if isinstance(ts_value, datetime):
                if ts_value.tzinfo is None:
                    doc_updated_at = ts_value.replace(tzinfo=timezone.utc)
                else:
                    doc_updated_at = ts_value
        
        first_content_col = self.content_columns[0] if self.content_columns else "record"
        semantic_id = str(row_dict.get(first_content_col, "database_record"))[:100]
        
        return Document(
            id=doc_id,
            blob=content.encode("utf-8"),
            source=DocumentSource(self.db_type.value),
            semantic_identifier=semantic_id,
            extension=".txt",
            doc_updated_at=doc_updated_at,
            size_bytes=len(content.encode("utf-8")),
            metadata=metadata if metadata else None,
        )

    def _yield_documents_from_query(
        self,
        query: str,
    ) -> Generator[list[Document], None, None]:
        """Generate documents from a single query."""
        connection = self._get_connection()
        cursor = connection.cursor()
        
        try:
            logging.info(f"Executing query: {query[:200]}...")
            cursor.execute(query)
            column_names = [desc[0] for desc in cursor.description]
            
            batch: list[Document] = []
            for row in cursor:
                try:
                    doc = self._row_to_document(row, column_names)
                    batch.append(doc)
                    
                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []
                except Exception as e:
                    logging.warning(f"Error converting row to document: {e}")
                    continue
            
            if batch:
                yield batch
                
        finally:
            try:
                cursor.fetchall()
            except Exception:
                pass
            cursor.close()

    def _yield_documents(
        self,
        start: Optional[datetime] = None,
        end: Optional[datetime] = None,
    ) -> Generator[list[Document], None, None]:
        """Generate documents from database query results."""
        if self.query:
            query = self._build_query_with_time_filter(start, end)
            yield from self._yield_documents_from_query(query)
        else:
            tables = self._get_tables()
            logging.info(f"No query specified. Loading all {len(tables)} tables: {tables}")
            for table in tables:
                query = f"SELECT * FROM {table}"
                logging.info(f"Loading table: {table}")
                yield from self._yield_documents_from_query(query)
        
        self._close_connection()

    def load_from_state(self) -> Generator[list[Document], None, None]:
        """Load all documents from the database (full sync)."""
        logging.debug(f"Loading all records from {self.db_type} database: {self.database}")
        return self._yield_documents()

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> Generator[list[Document], None, None]:
        """Poll for new/updated documents since the last sync (incremental sync)."""
        if not self.timestamp_column:
            logging.warning(
                "No timestamp column configured for incremental sync. "
                "Falling back to full sync."
            )
            return self.load_from_state()
        
        start_datetime = datetime.fromtimestamp(start, tz=timezone.utc)
        end_datetime = datetime.fromtimestamp(end, tz=timezone.utc)
        
        logging.debug(
            f"Polling {self.db_type} database {self.database} "
            f"from {start_datetime} to {end_datetime}"
        )
        
        return self._yield_documents(start_datetime, end_datetime)

    def validate_connector_settings(self) -> None:
        """Validate connector settings by testing the connection."""
        if not self._credentials:
            raise ConnectorMissingCredentialError("RDBMS credentials not loaded.")
        
        if not self.host:
            raise ConnectorValidationError("Database host is required.")
        
        if not self.database:
            raise ConnectorValidationError("Database name is required.")
        
        if not self.content_columns:
            raise ConnectorValidationError(
                "At least one content column must be specified."
            )
        
        try:
            connection = self._get_connection()
            cursor = connection.cursor()
            
            test_query = "SELECT 1"
            cursor.execute(test_query)
            cursor.fetchone()
            cursor.close()
            
            logging.info(f"Successfully connected to {self.db_type} database: {self.database}")
            
        except ConnectorValidationError:
            self._close_connection()
            raise
        except Exception as e:
            self._close_connection()
            raise ConnectorValidationError(
                f"Failed to connect to {self.db_type} database: {str(e)}"
            )
        finally:
            self._close_connection()


if __name__ == "__main__":
    import os
    
    credentials_dict = {
        "username": os.environ.get("DB_USERNAME", "root"),
        "password": os.environ.get("DB_PASSWORD", ""),
    }
    
    connector = RDBMSConnector(
        db_type="mysql",
        host=os.environ.get("DB_HOST", "localhost"),
        port=int(os.environ.get("DB_PORT", "3306")),
        database=os.environ.get("DB_NAME", "test"),
        query="SELECT * FROM products LIMIT 10",
        content_columns="name,description",
        metadata_columns="id,category,price",
        id_column="id",
        timestamp_column="updated_at",
    )
    
    try:
        connector.load_credentials(credentials_dict)
        connector.validate_connector_settings()
        
        for batch in connector.load_from_state():
            print(f"Batch of {len(batch)} documents:")
            for doc in batch:
                print(f"  - {doc.id}: {doc.semantic_identifier}")
            break
            
    except Exception as e:
        print(f"Error: {e}")

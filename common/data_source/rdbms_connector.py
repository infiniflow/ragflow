"""RDBMS (MySQL/PostgreSQL) data source connector for importing data from relational databases."""

import copy
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
from common.data_source.interfaces import (
    LoadConnector,
    PollConnector,
    SecondsSinceUnixEpoch,
    SlimConnectorWithPermSync,
)
from common.data_source.models import Document, SlimDocument


class DatabaseType(str, Enum):
    """Supported database types."""
    MYSQL = "mysql"
    POSTGRESQL = "postgresql"


class RDBMSConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    """
    Import rows from MySQL or PostgreSQL into documents.

    The flow is:
    1. Connect to the configured database.
    2. Read rows from a custom SQL query, or from every table when no query is provided.
    3. Build document content from the selected content columns.
    4. Copy the selected metadata columns into document metadata.
    5. Use the configured ID column as the stable document ID, or hash the content when no ID column is set.
    6. For incremental sync, treat the timestamp column as an ordered cursor and only compare values by size.
    7. For deleted-file sync, read a slim snapshot of current row IDs and let the sync worker remove stale documents.
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
        self._sync_connector_id: str | None = None
        self._sync_config: Dict[str, Any] | None = None
        self._pending_sync_cursor_value: Any = None

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


    def _get_base_queries(self) -> list[str]:
        if self.query:
            return [self.query.rstrip(";")]
        return [f"SELECT * FROM {table}" for table in self._get_tables()]


    def _wrap_query(self, base_query: str, select_clause: str = "*") -> str:
        return f"SELECT {select_clause} FROM ({base_query}) AS ragflow_src"


    @staticmethod
    def serialize_cursor_value(value: Any) -> Any:
        # Example:
        # - int cursor 42 is stored as 42
        # - datetime cursor 2026-05-07T12:34:56+00:00 is stored as
        #   {"__ragflow_rdbms_cursor_type__": "datetime", "value": "..."}
        # Only datetime needs wrapping because connector config is JSON.
        if isinstance(value, datetime):
            return {
                "__ragflow_rdbms_cursor_type__": "datetime",
                "value": value.isoformat(),
            }
        return value


    @staticmethod
    def deserialize_cursor_value(value: Any) -> Any:
        # Reverse the datetime wrapper above.
        # Non-datetime cursors such as int/str/float are returned as-is.
        if (
            isinstance(value, dict)
            and value.get("__ragflow_rdbms_cursor_type__") == "datetime"
        ):
            return datetime.fromisoformat(value["value"])
        return value


    def _format_sql_value(self, value: Any) -> str:
        if isinstance(value, datetime):
            if value.tzinfo is None:
                value = value.replace(tzinfo=timezone.utc)
            if self.db_type == DatabaseType.MYSQL:
                rendered = value.astimezone(timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
            else:
                rendered = value.astimezone(timezone.utc).isoformat()
            return f"'{rendered}'"
        if isinstance(value, bool):
            if self.db_type == DatabaseType.POSTGRESQL:
                return "TRUE" if value else "FALSE"
            return "1" if value else "0"
        if isinstance(value, (int, float)):
            return str(value)
        if isinstance(value, str):
            return "'" + value.replace("'", "''") + "'"
        raise ConnectorValidationError(
            f"Unsupported timestamp cursor value type: {type(value).__name__}"
        )


    def _build_time_filtered_query(
        self,
        base_query: str,
        start: Any = None,
        end: Any = None,
    ) -> str:
        if not self.timestamp_column or (start is None and end is None):
            return self._wrap_query(base_query)

        conditions = []
        if start is not None:
            conditions.append(
                f"ragflow_src.{self.timestamp_column} > {self._format_sql_value(start)}"
            )
        if end is not None:
            conditions.append(
                f"ragflow_src.{self.timestamp_column} <= {self._format_sql_value(end)}"
            )

        query = self._wrap_query(base_query)
        if conditions:
            query = f"{query} WHERE {' AND '.join(conditions)}"
        return query


    def _build_max_timestamp_query(self, base_query: str) -> str:
        return (
            f"SELECT MAX(ragflow_src.{self.timestamp_column}) "
            f"FROM ({base_query}) AS ragflow_src"
        )


    def _build_slim_query(self, base_query: str) -> str:
        columns = [self.id_column] if self.id_column else self.content_columns
        select_clause = ", ".join(f"ragflow_src.{column}" for column in columns)
        return self._wrap_query(base_query, select_clause)


    def _build_content(self, row_dict: Dict[str, Any]) -> str:
        content_parts = []
        for col in self.content_columns:
            if col not in row_dict or row_dict[col] is None:
                continue
            value = row_dict[col]
            if isinstance(value, (dict, list)):
                value = json.dumps(value, ensure_ascii=False)
            content_parts.append(f"【{col}】:\n{value}")
        return "\n\n".join(content_parts)


    def _build_document_id_from_row(self, row_dict: Dict[str, Any]) -> str:
        if self.id_column and self.id_column in row_dict and row_dict[self.id_column] is not None:
            return f"{self.db_type}:{self.database}:{row_dict[self.id_column]}"
        content = self._build_content(row_dict)
        content_hash = hashlib.md5(content.encode()).hexdigest()
        return f"{self.db_type}:{self.database}:{content_hash}"


    def _row_to_document(
        self,
        row: Union[tuple, list, Dict[str, Any]],
        column_names: list[str],
    ) -> Document:
        """Convert a database row to a Document."""
        row_dict = dict(zip(column_names, row)) if isinstance(row, (list, tuple)) else row
        content = self._build_content(row_dict)
        metadata = {}
        for col in self.metadata_columns:
            if col not in row_dict or row_dict[col] is None:
                continue
            value = row_dict[col]
            if isinstance(value, datetime):
                value = value.isoformat()
            elif isinstance(value, (dict, list)):
                value = json.dumps(value, ensure_ascii=False)
            else:
                value = str(value)
            metadata[col] = value

        doc_updated_at = datetime.now(timezone.utc)
        if self.timestamp_column and self.timestamp_column in row_dict and row_dict[self.timestamp_column] is not None:
            ts_value = row_dict[self.timestamp_column]
            if isinstance(ts_value, datetime):
                if ts_value.tzinfo is None:
                    doc_updated_at = ts_value.replace(tzinfo=timezone.utc)
                else:
                    doc_updated_at = ts_value.astimezone(timezone.utc)

        first_content_col = self.content_columns[0] if self.content_columns else "record"
        semantic_id = (
            str(row_dict.get(first_content_col, "database_record"))
            .replace("\n", " ")
            .replace("\r", " ")
            .strip()[:100]
        )
        blob = content.encode("utf-8")

        return Document(
            id=self._build_document_id_from_row(row_dict),
            blob=blob,
            source=DocumentSource(self.db_type.value),
            semantic_identifier=semantic_id,
            extension=".txt",
            doc_updated_at=doc_updated_at,
            size_bytes=len(blob),
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


    def _yield_slim_documents_from_query(
        self,
        query: str,
    ) -> Generator[list[SlimDocument], None, None]:
        connection = self._get_connection()
        cursor = connection.cursor()

        try:
            logging.debug(f"Executing slim query: {query[:200]}...")
            cursor.execute(query)
            column_names = [desc[0] for desc in cursor.description]

            batch: list[SlimDocument] = []
            for row in cursor:
                row_dict = dict(zip(column_names, row)) if isinstance(row, (list, tuple)) else row
                batch.append(SlimDocument(id=self._build_document_id_from_row(row_dict)))
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

            if batch:
                yield batch
        finally:
            try:
                cursor.fetchall()
            except Exception:
                pass
            cursor.close()


    def get_max_cursor_value(self) -> Any:
        if not self.timestamp_column:
            return None

        max_cursor_value = None
        connection = self._get_connection()
        cursor = connection.cursor()

        try:
            for base_query in self._get_base_queries():
                query = self._build_max_timestamp_query(base_query)
                logging.debug(f"Executing max timestamp query: {query[:200]}...")
                cursor.execute(query)
                row = cursor.fetchone()
                if row is None or row[0] is None:
                    continue
                if max_cursor_value is None or row[0] > max_cursor_value:
                    max_cursor_value = row[0]
        finally:
            cursor.close()

        return max_cursor_value


    def _yield_documents(
        self,
        start: Any = None,
        end: Any = None,
    ) -> Generator[list[Document], None, None]:
        """Generate documents from database query results."""
        base_queries = self._get_base_queries()
        if not self.query:
            logging.info(f"No query specified. Loading all {len(base_queries)} tables.")

        try:
            for base_query in base_queries:
                query = self._build_time_filtered_query(base_query, start, end)
                yield from self._yield_documents_from_query(query)
        finally:
            self._close_connection()


    def load_from_state(self) -> Generator[list[Document], None, None]:
        """Load all documents from the database (full sync)."""
        logging.debug(f"Loading all records from {self.db_type} database: {self.database}")
        return self._yield_documents()


    def retrieve_all_slim_docs_perm_sync(
        self,
        callback: Any = None,
    ) -> Generator[list[SlimDocument], None, None]:
        del callback

        base_queries = self._get_base_queries()
        if not self.query:
            logging.info(f"No query specified. Retrieving slim documents from all {len(base_queries)} tables.")

        try:
            for base_query in base_queries:
                yield from self._yield_slim_documents_from_query(
                    self._build_slim_query(base_query)
                )
        finally:
            self._close_connection()

    def prepare_sync_state(self, connector_id: str, config: Dict[str, Any]) -> None:
        self._sync_connector_id = connector_id
        self._sync_config = copy.deepcopy(config)
        if not self.timestamp_column:
            self._pending_sync_cursor_value = None
            return
        self._pending_sync_cursor_value = self.get_max_cursor_value()


    def get_saved_sync_cursor_value(self) -> Any:
        if self._sync_config is None:
            return None
        return self.deserialize_cursor_value(self._sync_config.get("sync_cursor_value"))


    def persist_sync_state(self) -> None:
        if not self.timestamp_column or self._sync_connector_id is None or self._sync_config is None:
            return

        from api.db.services.connector_service import ConnectorService

        updated_conf = copy.deepcopy(self._sync_config)
        updated_conf["sync_cursor_value"] = self.serialize_cursor_value(
            self._pending_sync_cursor_value
        )
        ConnectorService.update_by_id(self._sync_connector_id, {"config": updated_conf})
        self._sync_config = updated_conf


    def load_from_cursor_range(
        self,
        start_value: Any = None,
        end_value: Any = None,
    ) -> Generator[list[Document], None, None]:
        if end_value is None:
            self._close_connection()
            return iter(())
        if start_value is not None and end_value <= start_value:
            self._close_connection()
            return iter(())
        return self._yield_documents(start_value, end_value)


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
        return self._yield_documents(start, end)


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

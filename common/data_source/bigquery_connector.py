"""Google BigQuery data source connector for importing query/table rows into documents.

This connector shares the user-facing row model of ``RDBMSConnector`` (MySQL/PostgreSQL):
selected content columns become document text, selected metadata columns become metadata,
an optional ID column produces stable document IDs, and an optional timestamp column enables
cursor-based incremental sync plus deleted-row pruning.

Unlike the RDBMS connector, BigQuery is a Google Cloud query-job service. This implementation
therefore uses the official ``google-cloud-bigquery`` client, service-account authentication,
explicit billing controls (``maximum_bytes_billed``, ``job_timeout_ms``), dry-run validation,
parameterized cursors with a resolved column type, and BigQuery-aware value serialization.
"""

import base64
import copy
import hashlib
import json
import logging
from datetime import date, datetime, time, timezone
from decimal import Decimal
from typing import Any, Dict, Generator, List, Optional, Tuple

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

try:
    from google.cloud import bigquery
    from google.oauth2 import service_account
except ImportError:  # pragma: no cover - import guarded at runtime
    bigquery = None
    service_account = None


# Marker keys used to round-trip non-JSON-native cursor values through the
# connector config (which is persisted as JSON).
_CURSOR_TYPE_KEY = "__ragflow_bq_cursor_type__"

# Default cost guard: 1 GiB. Users can raise this explicitly.
DEFAULT_MAXIMUM_BYTES_BILLED = 1024 * 1024 * 1024

# Maps a BigQuery field type to the ScalarQueryParameter type used for cursors.
_CURSOR_PARAM_TYPE_MAP = {
    "TIMESTAMP": "TIMESTAMP",
    "DATETIME": "DATETIME",
    "DATE": "DATE",
    "TIME": "TIME",
    "INTEGER": "INT64",
    "INT64": "INT64",
    "NUMERIC": "NUMERIC",
    "BIGNUMERIC": "BIGNUMERIC",
    "FLOAT": "FLOAT64",
    "FLOAT64": "FLOAT64",
}


class BigQueryConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    """Import rows from a BigQuery table or custom query into documents.

    The flow mirrors ``RDBMSConnector``:
    1. Authenticate with a service account and build a BigQuery client.
    2. Read rows from a single configured table or a single custom SQL query.
    3. Build document content from the selected content columns.
    4. Copy the selected metadata columns into document metadata.
    5. Use the configured ID column as the stable document ID, or hash the content.
    6. For incremental sync, treat the timestamp column as an ordered cursor and filter by it.
    7. For deleted-file sync, read a slim snapshot of current row IDs.

    "Empty query means all tables" is intentionally NOT supported here: scanning every table
    is dangerous and expensive in BigQuery. Either a custom ``query`` or both ``dataset_id``
    and ``table_id`` must be provided.
    """

    def __init__(
        self,
        project_id: str,
        dataset_id: Optional[str] = None,
        table_id: Optional[str] = None,
        location: Optional[str] = None,
        query: str = "",
        content_columns: str = "",
        metadata_columns: Optional[str] = None,
        id_column: Optional[str] = None,
        timestamp_column: Optional[str] = None,
        batch_size: int = INDEX_BATCH_SIZE,
        page_size: int = 1000,
        maximum_bytes_billed: Optional[int] = DEFAULT_MAXIMUM_BYTES_BILLED,
        job_timeout_ms: Optional[int] = None,
        use_query_cache: bool = True,
    ) -> None:
        """Initialize the BigQuery connector.

        Args:
            project_id: GCP project that owns the query jobs.
            dataset_id: Dataset id (table mode).
            table_id: Table id (table mode).
            location: Default location for the client and query jobs (e.g. "US", "EU").
            query: Custom GoogleSQL query (custom query mode). Takes precedence over table mode.
            content_columns: Comma-separated column names used for document content.
            metadata_columns: Comma-separated column names used as metadata (optional).
            id_column: Column used as the stable document ID (optional; hash fallback otherwise).
            timestamp_column: Column used for incremental cursor sync (optional).
            batch_size: Number of documents per yielded batch.
            page_size: BigQuery result page size (a fetch hint, not the batch size).
            maximum_bytes_billed: Hard cost guard passed to every query job.
            job_timeout_ms: Optional per-job timeout in milliseconds.
            use_query_cache: Whether to allow BigQuery's query result cache.
        """
        self.project_id = (project_id or "").strip()
        self.dataset_id = (dataset_id or "").strip()
        self.table_id = (table_id or "").strip()
        self.location = (location or "").strip()
        self.query = (query or "").strip()
        self.content_columns = [c.strip() for c in (content_columns or "").split(",") if c.strip()]
        self.metadata_columns = [c.strip() for c in (metadata_columns or "").split(",") if c.strip()]
        self.id_column = id_column.strip() if id_column else None
        self.timestamp_column = timestamp_column.strip() if timestamp_column else None
        self.batch_size = batch_size
        self.page_size = page_size
        self.maximum_bytes_billed = maximum_bytes_billed
        self.job_timeout_ms = job_timeout_ms
        self.use_query_cache = use_query_cache

        self._client = None
        self._credentials: Dict[str, Any] = {}
        self._cursor_param_type: Optional[str] = None
        self._sync_connector_id: str | None = None
        self._sync_config: Dict[str, Any] | None = None
        self._pending_sync_cursor_value: Any = None
        self._pending_sync_cursor_id: Any = None

    # ------------------------------------------------------------------ #
    # Credentials & client
    # ------------------------------------------------------------------ #
    def load_credentials(self, credentials: Dict[str, Any]) -> Dict[str, Any] | None:
        """Load BigQuery service-account credentials.

        Accepts ``service_account_json`` as either a dict or a JSON string.
        """
        logging.debug("Loading credentials for BigQuery project: %s", self.project_id)

        raw = (credentials or {}).get("service_account_json")
        if not raw:
            raise ConnectorMissingCredentialError("BigQuery: missing service_account_json")

        if isinstance(raw, str):
            try:
                service_account_info = json.loads(raw)
            except json.JSONDecodeError as exc:
                raise ConnectorMissingCredentialError(f"BigQuery: service_account_json is not valid JSON: {exc}")
        elif isinstance(raw, dict):
            service_account_info = raw
        else:
            raise ConnectorMissingCredentialError("BigQuery: service_account_json must be a JSON string or object")

        self._credentials = {"service_account_info": service_account_info}
        return None

    def _get_client(self):
        """Create and cache a BigQuery client from the loaded service account."""
        if self._client is not None:
            return self._client

        if bigquery is None or service_account is None:
            raise ConnectorValidationError("BigQuery client not installed. Please install google-cloud-bigquery.")

        service_account_info = self._credentials.get("service_account_info")
        if not service_account_info:
            raise ConnectorMissingCredentialError("BigQuery credentials not loaded.")

        try:
            creds = service_account.Credentials.from_service_account_info(service_account_info)
        except Exception as exc:
            raise ConnectorValidationError(f"Failed to build BigQuery credentials: {exc}")

        try:
            self._client = bigquery.Client(
                project=self.project_id or None,
                credentials=creds,
                location=self.location or None,
            )
        except Exception as exc:
            raise ConnectorValidationError(f"Failed to create BigQuery client: {exc}")

        return self._client

    # ------------------------------------------------------------------ #
    # Query construction
    # ------------------------------------------------------------------ #
    def _build_base_query(self) -> str:
        """Return the single base query (custom query takes precedence over table mode)."""
        if self.query:
            return self.query.rstrip(";")
        if self.dataset_id and self.table_id:
            return f"SELECT * FROM `{self.project_id}.{self.dataset_id}.{self.table_id}`"
        raise ConnectorValidationError("BigQuery requires either a custom query or both dataset_id and table_id.")

    @staticmethod
    def _wrap_query(base_query: str, select_clause: str = "*") -> str:
        return f"SELECT {select_clause} FROM ({base_query}) AS ragflow_src"

    def _build_query_job_config(
        self,
        query_parameters: Optional[List[Any]] = None,
        dry_run: bool = False,
    ):
        config = bigquery.QueryJobConfig()
        config.use_legacy_sql = False
        config.use_query_cache = self.use_query_cache
        if self.maximum_bytes_billed:
            config.maximum_bytes_billed = int(self.maximum_bytes_billed)
        if self.job_timeout_ms:
            config.job_timeout_ms = int(self.job_timeout_ms)
        if query_parameters:
            config.query_parameters = query_parameters
        if dry_run:
            config.dry_run = True
            config.use_query_cache = False
        return config

    def _resolve_schema(self, client: Any, dry_run_job: Any = None) -> List[Any]:
        if not self.query and self.dataset_id and self.table_id:
            table = client.get_table(f"{self.project_id}.{self.dataset_id}.{self.table_id}")
            return table.schema
        else:
            if dry_run_job is None:
                dry_run_job = client.query(
                    self._wrap_query(self._build_base_query()),
                    job_config=self._build_query_job_config(dry_run=True),
                    location=self.location or None,
                )
            return dry_run_job.schema or []

    def _get_cursor_column_field_type(self) -> str:
        """Resolve the BigQuery field type of the timestamp column."""
        client = self._get_client()
        schema = self._resolve_schema(client)

        for field in schema:
            if field.name == self.timestamp_column:
                return field.field_type
        raise ConnectorValidationError(f"BigQuery timestamp column '{self.timestamp_column}' was not found in the schema.")

    def _resolve_cursor_param_type(self) -> str:
        if self._cursor_param_type is not None:
            return self._cursor_param_type
        field_type = (self._get_cursor_column_field_type() or "").upper()
        param_type = _CURSOR_PARAM_TYPE_MAP.get(field_type)
        if param_type is None:
            raise ConnectorValidationError(f"BigQuery timestamp column type '{field_type}' is not supported as a cursor.")
        self._cursor_param_type = param_type
        return param_type

    def _make_cursor_param(self, name: str, value: Any, param_type: str):
        return bigquery.ScalarQueryParameter(name, param_type, value)

    def _build_time_filtered_query(
        self,
        base_query: str,
        start: Any = None,
        end: Any = None,
        start_id: Any = None,
    ) -> Tuple[str, List[Any]]:
        wrapped = self._wrap_query(base_query)
        if not self.timestamp_column or (start is None and end is None):
            return wrapped, []

        param_type = self._resolve_cursor_param_type()
        conditions: List[str] = []
        params: List[Any] = []
        if start is not None:
            if self.id_column and start_id is not None:
                conditions.append(f"(ragflow_src.{self.timestamp_column} > @start_cursor OR (ragflow_src.{self.timestamp_column} = @start_cursor AND ragflow_src.{self.id_column} > @start_cursor_id))")
                params.append(self._make_cursor_param("start_cursor", start, param_type))
                params.append(self._make_cursor_param("start_cursor_id", start_id, "STRING"))
            else:
                conditions.append(f"ragflow_src.{self.timestamp_column} >= @start_cursor")
                params.append(self._make_cursor_param("start_cursor", start, param_type))
        if end is not None:
            conditions.append(f"ragflow_src.{self.timestamp_column} <= @end_cursor")
            params.append(self._make_cursor_param("end_cursor", end, param_type))

        if conditions:
            wrapped = f"{wrapped} WHERE {' AND '.join(conditions)}"
        return wrapped, params

    def _build_max_timestamp_query(self, base_query: str) -> str:
        if self.id_column:
            # We need both the max timestamp and the max id for that timestamp
            return (
                f"SELECT ragflow_src.{self.timestamp_column}, MAX(ragflow_src.{self.id_column}) "
                f"FROM ({base_query}) AS ragflow_src "
                f"WHERE ragflow_src.{self.timestamp_column} = ("
                f"  SELECT MAX({self.timestamp_column}) FROM ({base_query})"
                f") "
                f"GROUP BY ragflow_src.{self.timestamp_column}"
            )
        return f"SELECT MAX(ragflow_src.{self.timestamp_column}), NULL FROM ({base_query}) AS ragflow_src"

    def _build_slim_query(self, base_query: str) -> str:
        columns = [self.id_column] if self.id_column else self.content_columns
        select_clause = ", ".join(f"ragflow_src.{column}" for column in columns)
        return self._wrap_query(base_query, select_clause)

    # ------------------------------------------------------------------ #
    # Cursor (de)serialization
    # ------------------------------------------------------------------ #
    @staticmethod
    def serialize_cursor_value(value: Any) -> Any:
        # Connector config is JSON, so datetime/date must be wrapped. Other
        # scalar cursors (int/float/str) round-trip natively.
        if isinstance(value, datetime):
            return {_CURSOR_TYPE_KEY: "datetime", "value": value.isoformat()}
        if isinstance(value, date):
            return {_CURSOR_TYPE_KEY: "date", "value": value.isoformat()}
        if isinstance(value, time):
            return {_CURSOR_TYPE_KEY: "time", "value": value.isoformat()}
        if isinstance(value, Decimal):
            return {_CURSOR_TYPE_KEY: "decimal", "value": str(value)}
        return value

    @staticmethod
    def deserialize_cursor_value(value: Any) -> Any:
        if isinstance(value, dict) and _CURSOR_TYPE_KEY in value:
            kind = value.get(_CURSOR_TYPE_KEY)
            if kind == "datetime":
                return datetime.fromisoformat(value["value"])
            if kind == "date":
                return date.fromisoformat(value["value"])
            if kind == "time":
                return time.fromisoformat(value["value"])
            if kind == "decimal":
                return Decimal(value["value"])
        return value

    # ------------------------------------------------------------------ #
    # Value rendering
    # ------------------------------------------------------------------ #
    @staticmethod
    def _render_content_value(value: Any) -> Optional[str]:
        """Render a value for document content. Returns None to skip the value."""
        if isinstance(value, (bytes, bytearray)):
            # Binary content is not meaningful as document text; skip it.
            return None
        if isinstance(value, (datetime, date, time)):
            return value.isoformat()
        if isinstance(value, (dict, list)):
            return json.dumps(value, ensure_ascii=False, default=str)
        return str(value)

    @staticmethod
    def _render_metadata_value(value: Any) -> str:
        if isinstance(value, (bytes, bytearray)):
            return base64.b64encode(bytes(value)).decode("ascii")
        if isinstance(value, (datetime, date, time)):
            return value.isoformat()
        if isinstance(value, (dict, list)):
            return json.dumps(value, ensure_ascii=False, default=str)
        return str(value)

    def _build_content(self, row_dict: Dict[str, Any]) -> str:
        content_parts = []
        for col in self.content_columns:
            if col not in row_dict or row_dict[col] is None:
                continue
            rendered = self._render_content_value(row_dict[col])
            if rendered is None:
                continue
            content_parts.append(f"【{col}】:\n{rendered}")
        return "\n\n".join(content_parts)

    def _id_prefix(self) -> str:
        if not self.query and self.dataset_id and self.table_id:
            return f"bigquery:{self.project_id}:{self.dataset_id}.{self.table_id}"
        return f"bigquery:{self.project_id}:query"

    def _build_document_id_from_row(self, row_dict: Dict[str, Any]) -> str:
        prefix = self._id_prefix()
        if self.id_column and self.id_column in row_dict and row_dict[self.id_column] is not None:
            return f"{prefix}:{row_dict[self.id_column]}"
        content = self._build_content(row_dict)
        content_hash = hashlib.md5(content.encode()).hexdigest()
        return f"{prefix}:{content_hash}"

    def _row_to_document(self, row_dict: Dict[str, Any]) -> Document:
        content = self._build_content(row_dict)
        metadata = {}
        for col in self.metadata_columns:
            if col not in row_dict or row_dict[col] is None:
                continue
            metadata[col] = self._render_metadata_value(row_dict[col])

        doc_updated_at = datetime.now(timezone.utc)
        if self.timestamp_column and row_dict.get(self.timestamp_column) is not None:
            ts_value = row_dict[self.timestamp_column]
            if isinstance(ts_value, datetime):
                if ts_value.tzinfo is None:
                    doc_updated_at = ts_value.replace(tzinfo=timezone.utc)
                else:
                    doc_updated_at = ts_value.astimezone(timezone.utc)
            elif isinstance(ts_value, date):
                doc_updated_at = datetime(ts_value.year, ts_value.month, ts_value.day, tzinfo=timezone.utc)

        first_content_col = self.content_columns[0] if self.content_columns else "record"
        semantic_id = str(row_dict.get(first_content_col, "bigquery_record")).replace("\n", " ").replace("\r", " ").strip()[:100]
        blob = content.encode("utf-8")

        return Document(
            id=self._build_document_id_from_row(row_dict),
            blob=blob,
            source=DocumentSource.BIGQUERY,
            semantic_identifier=semantic_id,
            extension=".txt",
            doc_updated_at=doc_updated_at,
            size_bytes=len(blob),
            metadata=metadata if metadata else None,
        )

    # ------------------------------------------------------------------ #
    # Query execution
    # ------------------------------------------------------------------ #
    def _yield_rows_from_query(
        self,
        query: str,
        query_parameters: Optional[List[Any]] = None,
    ) -> Generator[list[Document], None, None]:
        client = self._get_client()
        logging.info("Executing BigQuery query: %s...", query[:200])
        job = client.query(
            query,
            job_config=self._build_query_job_config(query_parameters=query_parameters),
            location=self.location or None,
        )
        result = job.result(page_size=self.page_size)

        batch: list[Document] = []
        for row in result:
            try:
                doc = self._row_to_document(dict(row))
                batch.append(doc)
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []
            except Exception as exc:
                logging.warning("Error converting BigQuery row to document: %s", exc)
                continue

        if batch:
            yield batch

    def _yield_slim_documents_from_query(
        self,
        query: str,
    ) -> Generator[list[SlimDocument], None, None]:
        client = self._get_client()
        logging.debug("Executing BigQuery slim query: %s...", query[:200])
        job = client.query(
            query,
            job_config=self._build_query_job_config(),
            location=self.location or None,
        )
        result = job.result(page_size=self.page_size)

        batch: list[SlimDocument] = []
        for row in result:
            batch.append(SlimDocument(id=self._build_document_id_from_row(dict(row))))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    def _yield_documents(
        self,
        start: Any = None,
        end: Any = None,
        start_id: Any = None,
    ) -> Generator[list[Document], None, None]:
        base_query = self._build_base_query()
        query, params = self._build_time_filtered_query(base_query, start, end, start_id)
        yield from self._yield_rows_from_query(query, params)

    def get_max_cursor_value(self) -> Tuple[Any, Any]:
        if not self.timestamp_column:
            return None, None

        client = self._get_client()
        query = self._build_max_timestamp_query(self._build_base_query())
        logging.debug("Executing BigQuery max timestamp query: %s...", query[:200])
        job = client.query(
            query,
            job_config=self._build_query_job_config(),
            location=self.location or None,
        )
        row = next(iter(job.result()), None)
        if row is None or row[0] is None:
            return None, None
        return row[0], row[1]

    # ------------------------------------------------------------------ #
    # LoadConnector / PollConnector
    # ------------------------------------------------------------------ #
    def load_from_state(self) -> Generator[list[Document], None, None]:
        """Load all rows from the configured table/query (full sync)."""
        logging.debug("Loading all records from BigQuery project: %s", self.project_id)
        return self._yield_documents()

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> Generator[list[Document], None, None]:
        """Poll for new/updated rows. Provided for interface completeness.

        Orchestration drives full/incremental sync via ``load_from_state`` /
        ``load_from_cursor_range``; this falls back to a full sync without a
        timestamp column.
        """
        if not self.timestamp_column:
            logging.warning("No timestamp column configured for incremental sync. Falling back to full sync.")
            return self.load_from_state()
        start_dt = datetime.fromtimestamp(start, tz=timezone.utc) if start else None
        end_dt = datetime.fromtimestamp(end, tz=timezone.utc) if end else None
        return self._yield_documents(start_dt, end_dt)

    def load_from_cursor_range(
        self,
        start_value: Any = None,
        end_value: Any = None,
        start_id: Any = None,
    ) -> Generator[list[Document], None, None]:
        if end_value is None:
            return iter(())
        if start_value is not None and end_value < start_value:
            return iter(())
        return self._yield_documents(start_value, end_value, start_id)

    def retrieve_all_slim_docs_perm_sync(
        self,
        callback: Any = None,
    ) -> Generator[list[SlimDocument], None, None]:
        del callback
        yield from self._yield_slim_documents_from_query(self._build_slim_query(self._build_base_query()))

    # ------------------------------------------------------------------ #
    # Sync-state persistence (success-only cursor)
    # ------------------------------------------------------------------ #
    def prepare_sync_state(self, connector_id: str, config: Dict[str, Any]) -> None:
        self._sync_connector_id = connector_id
        self._sync_config = copy.deepcopy(config)
        if not self.timestamp_column:
            self._pending_sync_cursor_value = None
            self._pending_sync_cursor_id = None
            return
        self._pending_sync_cursor_value, self._pending_sync_cursor_id = self.get_max_cursor_value()

    def get_saved_sync_cursor_value(self) -> Any:
        if self._sync_config is None:
            return None
        return self.deserialize_cursor_value(self._sync_config.get("sync_cursor_value"))

    def get_saved_sync_cursor_id(self) -> Any:
        if self._sync_config is None:
            return None
        return self._sync_config.get("sync_cursor_id")

    def persist_sync_state(self) -> None:
        if not self.timestamp_column or self._sync_connector_id is None or self._sync_config is None:
            return

        from api.db.services.connector_service import ConnectorService

        updated_conf = copy.deepcopy(self._sync_config)
        updated_conf["sync_cursor_value"] = self.serialize_cursor_value(self._pending_sync_cursor_value)
        updated_conf["sync_cursor_id"] = self._pending_sync_cursor_id
        ConnectorService.update_by_id(self._sync_connector_id, {"config": updated_conf})
        self._sync_config = updated_conf

    # ------------------------------------------------------------------ #
    # Validation
    # ------------------------------------------------------------------ #
    def validate_connector_settings(self) -> None:
        """Validate settings via SELECT 1 plus a dry-run of the configured base query."""
        if not self._credentials:
            raise ConnectorMissingCredentialError("BigQuery credentials not loaded.")
        if not self.project_id:
            raise ConnectorValidationError("BigQuery project_id is required.")
        if not self.content_columns:
            raise ConnectorValidationError("At least one content column must be specified.")
        if not self.query and not (self.dataset_id and self.table_id):
            raise ConnectorValidationError("BigQuery requires either a custom query or both dataset_id and table_id.")

        try:
            client = self._get_client()

            # Cheap connectivity check.
            client.query(
                "SELECT 1",
                job_config=self._build_query_job_config(),
                location=self.location or None,
            ).result()

            # Free cost/validity check of the actual base query.
            dry_run_job = client.query(
                self._wrap_query(self._build_base_query()),
                job_config=self._build_query_job_config(dry_run=True),
                location=self.location or None,
            )
            estimated_bytes = getattr(dry_run_job, "total_bytes_processed", None)
            if estimated_bytes is not None:
                logging.info("BigQuery base query dry-run estimate: %s bytes processed.", estimated_bytes)

            schema = self._resolve_schema(client, dry_run_job)
            schema_columns = {field.name for field in schema}

            required = set(self.content_columns)
            optional = set(self.metadata_columns)
            if self.id_column:
                optional.add(self.id_column)
            if self.timestamp_column:
                optional.add(self.timestamp_column)

            missing = (required | optional) - schema_columns
            if missing:
                raise ConnectorValidationError(f"BigQuery configured columns not found in schema: {', '.join(sorted(missing))}")

            if self.timestamp_column:
                self._resolve_cursor_param_type()
        except (ConnectorValidationError, ConnectorMissingCredentialError):
            raise
        except Exception as exc:
            raise ConnectorValidationError(f"BigQuery validation failed: {exc}")

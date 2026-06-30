"""Databricks Lakehouse data-source connector.

Ingests two kinds of content into a RAGFlow knowledge base:

  1. **SQL-warehouse tables** — rows from Unity Catalog tables, rendered
     the same way the RDBMS connector renders relational rows (selected
     content columns joined into the document body, selected metadata
     columns copied into metadata). Incremental sync filters on a
     user-specified watermark/ingest-timestamp column.
  2. **Unity Catalog volume files** — binary files (pdf/docx/md/txt …)
     listed and downloaded from ``/Volumes/<catalog>/<schema>/<volume>``.
     Incremental sync filters on each file's modification time (mtime).

Auth supports two mutually exclusive modes selected by ``auth_mode``:

  * ``pat`` — a personal access token (``access_token``).
  * ``oauth`` — OAuth machine-to-machine (``client_id`` + ``client_secret``).

The SQL path uses ``databricks-sql-connector``; the volume path uses
``databricks-sdk``. Both are imported lazily so the rest of the
connector framework loads even when the optional libraries are absent;
a missing library surfaces as a clear ``ConnectorValidationError``.

Incremental runs are scoped by the poll time window
(``since_epoch`` < watermark/mtime <= ``until_epoch``). The connector
keeps no cross-run state; incrementality is owned by the global
``poll_range_start`` watermark the sync framework persists, and the
connector **fails closed** (any table/volume error aborts the run) so a
partial failure can never advance that watermark past content it never
ingested.
"""

from __future__ import annotations

import hashlib
import json
import logging
import re
from datetime import datetime, timezone
from typing import Any, Generator, Iterable

from common.data_source.config import INDEX_BATCH_SIZE
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    InsufficientPermissionsError,
    UnexpectedValidationError,
)
from common.data_source.interfaces import (
    CheckpointedConnectorWithPermSync,
    SecondsSinceUnixEpoch,
    SlimConnectorWithPermSync,
)
from common.data_source.models import ConnectorCheckpoint, SlimDocument

logger = logging.getLogger(__name__)

# Binary file types ingested from volumes; the issue names pdf/docx/md/txt
# and we include the rest of the common document set for consistency with
# the other file-based connectors.
_SUPPORTED_EXTENSIONS = {
    ".pdf", ".docx", ".doc", ".md", ".txt",
    ".xlsx", ".xls", ".pptx", ".ppt", ".csv",
    ".html", ".htm", ".json", ".xml",
}

# Unity Catalog / SQL identifiers we are willing to interpolate into a
# query. Identifiers cannot be passed as bind parameters, so we whitelist
# a conservative shape (``catalog.schema.table`` of word characters) and
# reject anything else rather than risk injection.
_IDENT_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
_TABLE_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*){0,2}$")

# Page size for volume directory listings.
_LIST_PAGE_SIZE = 1000

# Guard against a pathological volume tree.
_MAX_VOLUME_ENTRIES = 5_000_000


class DatabricksCheckpoint(ConnectorCheckpoint):
    """Checkpoint marker for the Databricks connector.

    The connector keeps no cross-run state of its own: a single pass walks
    the configured tables and volumes once and sets ``has_more=False``.
    Incremental scoping comes from the poll time window.
    """


class DatabricksConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """Databricks Lakehouse connector (SQL-warehouse tables + UC volumes)."""

    def __init__(
        self,
        server_hostname: str,
        http_path: str | None = None,
        auth_mode: str | None = None,
        tables: Iterable[str] | None = None,
        content_columns: str | None = None,
        metadata_columns: str | None = None,
        id_column: str | None = None,
        timestamp_column: str | None = None,
        volume_paths: Iterable[str] | None = None,
        batch_size: int = INDEX_BATCH_SIZE,
        allow_images: bool = False,
    ) -> None:
        self.server_hostname = _normalize_host(server_hostname)
        self.http_path = (http_path or "").strip()
        self.auth_mode = (auth_mode or "").strip().lower()
        self.tables = [t.strip() for t in (tables or []) if str(t).strip()]
        self.content_columns = [c.strip() for c in (content_columns or "").split(",") if c.strip()]
        self.metadata_columns = [c.strip() for c in (metadata_columns or "").split(",") if c.strip()]
        self.id_column = (id_column or "").strip() or None
        self.timestamp_column = (timestamp_column or "").strip() or None
        self.volume_paths = [p.strip() for p in (volume_paths or []) if str(p).strip()]
        self.batch_size = batch_size
        self.allow_images = allow_images

        self._access_token: str | None = None
        self._client_id: str | None = None
        self._client_secret: str | None = None

    # ------------------------------------------------------------------
    # Auth
    # ------------------------------------------------------------------

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        if not self.server_hostname:
            raise ConnectorMissingCredentialError(
                "Databricks: server_hostname is required"
            )

        access_token = credentials.get("access_token") or credentials.get("token")
        client_id = credentials.get("client_id")
        client_secret = credentials.get("client_secret")

        mode = self.auth_mode
        if not mode:
            mode = "oauth" if (client_id and client_secret) else "pat"

        if mode == "pat":
            if not access_token:
                raise ConnectorMissingCredentialError(
                    "Databricks: access_token is required for the pat auth mode"
                )
            self._access_token = access_token
        elif mode == "oauth":
            if not (client_id and client_secret):
                raise ConnectorMissingCredentialError(
                    "Databricks: client_id and client_secret are required for the oauth auth mode"
                )
            self._client_id = client_id
            self._client_secret = client_secret
        else:
            raise ConnectorMissingCredentialError(
                "Databricks credentials are incomplete. Provide one of: "
                "(a) access_token (PAT), or (b) client_id + client_secret (OAuth M2M)."
            )
        self.auth_mode = mode
        return None

    # ------------------------------------------------------------------
    # Validation
    # ------------------------------------------------------------------

    def validate_connector_settings(self) -> None:
        if not (self._access_token or (self._client_id and self._client_secret)):
            raise ConnectorMissingCredentialError("Databricks")

        if not self.tables and not self.volume_paths:
            raise ConnectorValidationError(
                "Databricks: configure at least one of 'tables' or 'volume_paths'."
            )

        for table in self.tables:
            if not _TABLE_RE.match(table):
                raise ConnectorValidationError(
                    f"Databricks: invalid table name '{table}' "
                    "(expected catalog.schema.table of word characters)."
                )
        if self.tables:
            if not self.http_path:
                raise ConnectorValidationError(
                    "Databricks: http_path (SQL warehouse) is required to ingest tables."
                )
            if not self.content_columns:
                raise ConnectorValidationError(
                    "Databricks: at least one content column is required to ingest tables."
                )
            for col in [*self.content_columns, *self.metadata_columns,
                        *( [self.id_column] if self.id_column else [] ),
                        *( [self.timestamp_column] if self.timestamp_column else [] )]:
                if not _IDENT_RE.match(col):
                    raise ConnectorValidationError(
                        f"Databricks: invalid column name '{col}'."
                    )

        for path in self.volume_paths:
            if not path.startswith("/Volumes/"):
                raise ConnectorValidationError(
                    f"Databricks: volume path '{path}' must start with /Volumes/."
                )

        # Light connectivity probe: SELECT 1 over the SQL warehouse when
        # tables are configured (proves host + http_path + credentials).
        if self.tables:
            try:
                with self._sql_connection() as conn:
                    cur = conn.cursor()
                    cur.execute("SELECT 1")
                    cur.fetchone()
                    cur.close()
            except (ConnectorMissingCredentialError, ConnectorValidationError,
                    InsufficientPermissionsError):
                raise
            except Exception as exc:
                raise UnexpectedValidationError(
                    f"Databricks: SQL warehouse connection failed: {exc}"
                ) from exc

    # ------------------------------------------------------------------
    # Checkpoint helpers
    # ------------------------------------------------------------------

    def build_dummy_checkpoint(self) -> DatabricksCheckpoint:
        return DatabricksCheckpoint(has_more=True)

    def validate_checkpoint_json(self, checkpoint_json: str) -> DatabricksCheckpoint:
        try:
            return DatabricksCheckpoint.model_validate_json(checkpoint_json)
        except Exception:
            return self.build_dummy_checkpoint()

    # ------------------------------------------------------------------
    # Core data loading
    # ------------------------------------------------------------------

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> Any:
        return self._iter_documents(since_epoch=start, until_epoch=end)

    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> Any:
        if not isinstance(checkpoint, DatabricksCheckpoint):
            checkpoint = self.build_dummy_checkpoint()
        since = start if start else None
        until = end if end else None
        return self._iter_documents(
            checkpoint=checkpoint, since_epoch=since, until_epoch=until
        )

    def load_from_checkpoint_with_perm_sync(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> Any:
        return self.load_from_checkpoint(start, end, checkpoint)

    def retrieve_all_slim_docs_perm_sync(
        self,
        callback: Any = None,
    ) -> Generator[list[SlimDocument], None, None]:
        """Yield batches of slim documents for prune / permission sync.

        Fails closed: if any table/volume enumeration errors, the error
        propagates so the prune collector aborts and skips deletion rather
        than treating a partial snapshot as authoritative and wrongly
        deleting still-valid documents.
        """
        batch: list[SlimDocument] = []

        if self.tables:
            with self._sql_connection() as conn:
                for table in self.tables:
                    for doc_id in self._iter_table_slim_ids(conn, table):
                        if callback:
                            callback(doc_id, table)
                        batch.append(SlimDocument(id=doc_id))
                        if len(batch) >= self.batch_size:
                            yield batch
                            batch = []

        if self.volume_paths:
            client = self._workspace_client()
            for root in self.volume_paths:
                for entry in self._walk_volume(client, root):
                    name = entry["path"].rsplit("/", 1)[-1]
                    if not _has_supported_extension(name, self.allow_images):
                        continue
                    doc_id = f"volume:{entry['path']}"
                    if callback:
                        callback(doc_id, root)
                    batch.append(SlimDocument(id=doc_id))
                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []

        if batch:
            yield batch

    # ------------------------------------------------------------------
    # Internal iteration
    # ------------------------------------------------------------------

    def _iter_documents(
        self,
        checkpoint: DatabricksCheckpoint | None = None,
        since_epoch: float | None = None,
        until_epoch: float | None = None,
    ):
        batch: list[Any] = []

        # Tables first, then volumes. Any error propagates (fail closed) so
        # the run aborts and the global watermark is not advanced.
        if self.tables:
            with self._sql_connection() as conn:
                for table in self.tables:
                    for doc in self._iter_table_documents(conn, table, since_epoch, until_epoch):
                        batch.append(doc)
                        if len(batch) >= self.batch_size:
                            yield batch
                            batch = []

        if self.volume_paths:
            client = self._workspace_client()
            for root in self.volume_paths:
                for doc in self._iter_volume_documents(client, root, since_epoch, until_epoch):
                    batch.append(doc)
                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []

        if batch:
            yield batch

        if checkpoint is not None:
            checkpoint.has_more = False

    # ---- Tables -------------------------------------------------------

    def _iter_table_documents(
        self, conn: Any, table: str, since_epoch: float | None, until_epoch: float | None
    ):
        from common.data_source.models import Document

        query = self._build_table_query(table, since_epoch, until_epoch)
        cursor = conn.cursor()
        try:
            cursor.execute(query)
            column_names = [desc[0] for desc in cursor.description]
            for row in cursor:
                row_dict = dict(zip(column_names, row))
                content = _build_content(row_dict, self.content_columns)
                blob = content.encode("utf-8")
                doc_id = _row_doc_id(table, row_dict, self.id_column, content)
                doc_updated_at = _row_updated_at(row_dict, self.timestamp_column)
                metadata = _row_metadata(row_dict, self.metadata_columns)
                metadata["table"] = table
                first_col = self.content_columns[0] if self.content_columns else "record"
                semantic_id = (
                    str(row_dict.get(first_col, "databricks_record"))
                    .replace("\n", " ").replace("\r", " ").strip()[:100]
                )
                yield Document(
                    id=doc_id,
                    source="databricks",
                    semantic_identifier=semantic_id or doc_id,
                    extension=".txt",
                    blob=blob,
                    doc_updated_at=doc_updated_at,
                    size_bytes=len(blob),
                    fingerprint=None,
                    metadata=metadata,
                )
        finally:
            cursor.close()

    def _iter_table_slim_ids(self, conn: Any, table: str) -> Generator[str, None, None]:
        cols = [self.id_column] if self.id_column else self.content_columns
        select_clause = ", ".join(f"`{c}`" for c in cols)
        query = f"SELECT {select_clause} FROM {_quote_table(table)}"
        cursor = conn.cursor()
        try:
            cursor.execute(query)
            column_names = [desc[0] for desc in cursor.description]
            for row in cursor:
                row_dict = dict(zip(column_names, row))
                content = _build_content(row_dict, self.content_columns)
                yield _row_doc_id(table, row_dict, self.id_column, content)
        finally:
            cursor.close()

    def _build_table_query(
        self, table: str, since_epoch: float | None, until_epoch: float | None
    ) -> str:
        """Build a SELECT with the strict-lower / inclusive-upper watermark
        window. Identifiers are validated + backtick-quoted. The window bounds
        are framework-supplied datetimes (never user input) inlined as
        ``TIMESTAMP`` literals, so this is injection-safe without depending on
        a particular DBAPI paramstyle across databricks-sql-connector
        versions (qmark/native vs pyformat)."""
        query = f"SELECT * FROM {_quote_table(table)}"
        if self.timestamp_column and (since_epoch or until_epoch):
            conditions = []
            col = f"`{self.timestamp_column}`"
            if since_epoch:
                conditions.append(f"{col} > {_ts_literal(since_epoch)}")
            if until_epoch:
                conditions.append(f"{col} <= {_ts_literal(until_epoch)}")
            query = f"{query} WHERE {' AND '.join(conditions)}"
        return query

    # ---- Volumes ------------------------------------------------------

    def _iter_volume_documents(
        self, client: Any, root: str, since_epoch: float | None, until_epoch: float | None
    ):
        from common.data_source.models import Document

        for entry in self._walk_volume(client, root):
            path = entry["path"]
            name = path.rsplit("/", 1)[-1]
            if not _has_supported_extension(name, self.allow_images):
                continue

            mtime = entry.get("mtime")
            if mtime is not None:
                if since_epoch and mtime <= since_epoch:
                    continue
                if until_epoch and mtime > until_epoch:
                    continue

            try:
                data = self._download_file(client, path)
            except _FileGone:
                logger.warning("Databricks: volume file %s vanished; skipping", path)
                continue

            doc_updated_at = (
                datetime.fromtimestamp(mtime, tz=timezone.utc)
                if mtime is not None else datetime.now(timezone.utc)
            )
            yield Document(
                id=f"volume:{path}",
                source="databricks",
                semantic_identifier=name,
                extension=_extension(name),
                blob=data,
                doc_updated_at=doc_updated_at,
                size_bytes=len(data),
                fingerprint=None,
                metadata={"volume_path": path, "root": root},
            )

    def _walk_volume(self, client: Any, root: str) -> Generator[dict[str, Any], None, None]:
        """Yield file entries under a volume path, recursing into directories.

        Each entry is ``{"path": str, "mtime": float|None}``. Uses the
        Files API ``list_directory_contents``; mtime is normalized to epoch
        seconds.
        """
        queue = [root.rstrip("/")]
        seen = 0
        while queue:
            directory = queue.pop(0)
            try:
                listing = client.files.list_directory_contents(
                    directory_path=directory, page_size=_LIST_PAGE_SIZE
                )
            except Exception as exc:
                if _is_not_found(exc):
                    logger.warning("Databricks: volume path %s not found; skipping", directory)
                    continue
                if _is_permission(exc):
                    raise InsufficientPermissionsError(
                        f"Databricks: insufficient permissions listing {directory}: {exc}"
                    ) from exc
                raise UnexpectedValidationError(
                    f"Databricks: listing volume {directory} failed: {exc}"
                ) from exc

            for item in listing:
                seen += 1
                if seen > _MAX_VOLUME_ENTRIES:
                    raise UnexpectedValidationError(
                        "Databricks: volume entry limit exceeded; aborting to avoid a runaway crawl"
                    )
                path = getattr(item, "path", None)
                if not path:
                    continue
                if getattr(item, "is_directory", False):
                    queue.append(path.rstrip("/"))
                    continue
                yield {"path": path, "mtime": _ms_to_epoch(getattr(item, "last_modified", None))}

    def _download_file(self, client: Any, path: str) -> bytes:
        try:
            resp = client.files.download(file_path=path)
        except Exception as exc:
            if _is_not_found(exc):
                raise _FileGone(path) from exc
            if _is_permission(exc):
                raise InsufficientPermissionsError(
                    f"Databricks: insufficient permissions reading {path}: {exc}"
                ) from exc
            raise UnexpectedValidationError(
                f"Databricks: downloading {path} failed: {exc}"
            ) from exc

        contents = getattr(resp, "contents", resp)
        if hasattr(contents, "read"):
            return contents.read()
        if isinstance(contents, (bytes, bytearray)):
            return bytes(contents)
        raise UnexpectedValidationError(
            f"Databricks: unexpected download payload for {path}"
        )

    # ------------------------------------------------------------------
    # Client builders (lazy imports)
    # ------------------------------------------------------------------

    def _sql_connection(self):
        try:
            from databricks import sql  # type: ignore
        except ImportError as exc:
            raise ConnectorValidationError(
                "Databricks SQL connector not installed. Please install databricks-sql-connector."
            ) from exc

        try:
            if self.auth_mode == "oauth":
                return sql.connect(
                    server_hostname=self.server_hostname,
                    http_path=self.http_path,
                    credentials_provider=self._oauth_credentials_provider(),
                )
            return sql.connect(
                server_hostname=self.server_hostname,
                http_path=self.http_path,
                access_token=self._access_token,
            )
        except Exception as exc:
            raise UnexpectedValidationError(
                f"Databricks: SQL warehouse connection failed: {exc}"
            ) from exc

    def _oauth_credentials_provider(self):
        from databricks.sdk.core import Config, oauth_service_principal  # type: ignore

        config = Config(
            host=f"https://{self.server_hostname}",
            client_id=self._client_id,
            client_secret=self._client_secret,
        )
        return lambda: oauth_service_principal(config)

    def _workspace_client(self):
        try:
            from databricks.sdk import WorkspaceClient  # type: ignore
        except ImportError as exc:
            raise ConnectorValidationError(
                "Databricks SDK not installed. Please install databricks-sdk."
            ) from exc

        host = f"https://{self.server_hostname}"
        try:
            if self.auth_mode == "oauth":
                return WorkspaceClient(
                    host=host,
                    client_id=self._client_id,
                    client_secret=self._client_secret,
                )
            return WorkspaceClient(host=host, token=self._access_token)
        except Exception as exc:
            raise UnexpectedValidationError(
                f"Databricks: workspace client init failed: {exc}"
            ) from exc


# ----------------------------------------------------------------------
# Module-level helpers
# ----------------------------------------------------------------------

class _FileGone(Exception):
    """A volume file listed moments earlier no longer exists."""


def _normalize_host(host: str) -> str:
    h = (host or "").strip()
    h = re.sub(r"^https?://", "", h)
    return h.rstrip("/")


def _quote_table(table: str) -> str:
    # table already validated against _TABLE_RE; quote each part.
    return ".".join(f"`{part}`" for part in table.split("."))


def _extension(name: str) -> str:
    if "." not in name:
        return ""
    return "." + name.rsplit(".", 1)[-1].lower()


def _has_supported_extension(name: str, allow_images: bool) -> bool:
    ext = _extension(name)
    if ext in _SUPPORTED_EXTENSIONS:
        return True
    if allow_images and ext in {".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".tiff"}:
        return True
    return False


def _build_content(row_dict: dict[str, Any], content_columns: list[str]) -> str:
    parts = []
    for col in content_columns:
        if col not in row_dict or row_dict[col] is None:
            continue
        value = row_dict[col]
        if isinstance(value, (dict, list)):
            value = json.dumps(value, ensure_ascii=False)
        parts.append(f"【{col}】:\n{value}")
    return "\n\n".join(parts)


def _row_metadata(row_dict: dict[str, Any], metadata_columns: list[str]) -> dict[str, Any]:
    metadata: dict[str, Any] = {}
    for col in metadata_columns:
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
    return metadata


def _row_doc_id(table: str, row_dict: dict[str, Any], id_column: str | None, content: str) -> str:
    if id_column and row_dict.get(id_column) is not None:
        return f"databricks:{table}:{row_dict[id_column]}"
    content_hash = hashlib.md5(content.encode()).hexdigest()
    return f"databricks:{table}:{content_hash}"


def _row_updated_at(row_dict: dict[str, Any], timestamp_column: str | None) -> datetime:
    if timestamp_column and row_dict.get(timestamp_column) is not None:
        value = row_dict[timestamp_column]
        if isinstance(value, datetime):
            return value.replace(tzinfo=timezone.utc) if value.tzinfo is None else value.astimezone(timezone.utc)
    return datetime.now(timezone.utc)


def _ts_literal(epoch: float) -> str:
    # Render a framework-supplied epoch as a Databricks SQL TIMESTAMP literal.
    # Value is fully controlled (never user input), so inlining is safe.
    dt = datetime.fromtimestamp(epoch, tz=timezone.utc)
    return "TIMESTAMP '" + dt.strftime("%Y-%m-%d %H:%M:%S") + "'"


def _ms_to_epoch(ms: Any) -> float | None:
    if ms is None:
        return None
    try:
        return float(ms) / 1000.0
    except (TypeError, ValueError):
        return None


def _is_not_found(exc: Exception) -> bool:
    if getattr(exc, "status_code", None) == 404:
        return True
    text = f"{type(exc).__name__}: {exc}"
    return "NotFound" in text or "RESOURCE_DOES_NOT_EXIST" in text or "404" in text


def _is_permission(exc: Exception) -> bool:
    if getattr(exc, "status_code", None) in (401, 403):
        return True
    text = f"{type(exc).__name__}: {exc}"
    return "PermissionDenied" in text or "Unauthorized" in text or "403" in text or "401" in text

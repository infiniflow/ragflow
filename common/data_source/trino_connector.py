from __future__ import annotations

import hashlib
import logging
import time
from collections.abc import Generator, Iterator
from datetime import datetime, timezone
from typing import Any
from urllib.parse import urljoin

import requests

from common.data_source.config import DocumentSource, INDEX_BATCH_SIZE, REQUEST_TIMEOUT_SECONDS
from common.data_source.interfaces import LoadConnector, PollConnector, SlimConnectorWithPermSync
from common.data_source.models import Document, SecondsSinceUnixEpoch, SlimDocument


logger = logging.getLogger(__name__)


class TrinoConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    def __init__(
        self,
        server_url: str,
        catalog: str,
        schema: str,
        query: str,
        content_columns: str,
        metadata_columns: str = "",
        id_column: str = "",
        timestamp_column: str = "",
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        self.server_url = server_url.rstrip("/")
        self.catalog = catalog.strip()
        self.schema = schema.strip()
        self.query = query.strip().rstrip(";")
        self.content_columns = [c.strip() for c in content_columns.split(",") if c.strip()]
        self.metadata_columns = [c.strip() for c in metadata_columns.split(",") if c.strip()]
        self.id_column = id_column.strip()
        self.timestamp_column = timestamp_column.strip()
        self.batch_size = batch_size or INDEX_BATCH_SIZE
        self.session = requests.Session()
        self._credentials: dict[str, Any] = {}
        logger.info(
            "Initialized Trino connector server_url=%s catalog=%s schema=%s batch_size=%s content_columns=%s metadata_columns=%s has_id_column=%s has_timestamp_column=%s",
            self.server_url,
            self.catalog,
            self.schema,
            self.batch_size,
            len(self.content_columns),
            len(self.metadata_columns),
            bool(self.id_column),
            bool(self.timestamp_column),
        )

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self._credentials = credentials or {}
        username = self._credentials.get("trino_username")
        password = self._credentials.get("trino_password")
        token = self._credentials.get("trino_bearer_token")
        if username:
            self.session.headers.update({"X-Trino-User": username})
        if token:
            self.session.headers.update({"Authorization": f"Bearer {token}"})
            logger.info("Loaded Trino connector credentials auth_mode=bearer has_username=%s", bool(username))
        elif username and password:
            self.session.auth = (username, password)
            logger.info("Loaded Trino connector credentials auth_mode=basic has_username=%s", bool(username))
        else:
            logger.info("Loaded Trino connector credentials auth_mode=none has_username=%s", bool(username))
        return None

    def validate_connector_settings(self) -> None:
        if not self.server_url:
            raise ValueError("Trino connector requires server_url")
        if not self.query:
            raise ValueError("Trino connector requires query")
        if not self.content_columns:
            raise ValueError("Trino connector requires content_columns")
        if not self._credentials.get("trino_username"):
            raise ValueError("Trino connector requires trino_username")

    def load_from_state(self) -> Generator[list[Document], None, None]:
        yield from self._batch_documents(self._iter_documents())

    def poll_source(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
    ) -> Generator[list[Document], None, None]:
        start_dt = datetime.fromtimestamp(start, tz=timezone.utc)
        end_dt = datetime.fromtimestamp(end, tz=timezone.utc)
        docs = (
            doc
            for doc in self._iter_documents()
            if start_dt <= doc.doc_updated_at <= end_dt
        )
        yield from self._batch_documents(docs)

    def retrieve_all_slim_docs_perm_sync(self, callback: Any = None) -> Generator[list[SlimDocument], None, None]:
        del callback
        batch: list[SlimDocument] = []
        for doc in self._iter_documents():
            batch.append(SlimDocument(id=doc.id))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []
        if batch:
            yield batch

    def _iter_documents(self) -> Iterator[Document]:
        for row in self._query_rows():
            yield self._document_from_row(row)

    def _query_rows(self) -> Iterator[dict[str, Any]]:
        started_at = time.perf_counter()
        row_count = 0
        logger.info("Starting Trino query catalog=%s schema=%s", self.catalog, self.schema)
        try:
            response = self.session.post(
                urljoin(f"{self.server_url}/", "v1/statement"),
                data=self.query,
                headers=self._statement_headers(),
                timeout=REQUEST_TIMEOUT_SECONDS,
            )
            response.raise_for_status()
            payload = response.json()
            while True:
                self._raise_for_trino_error(payload)
                for row in self._rows_from_payload(payload):
                    row_count += 1
                    yield row
                next_uri = payload.get("nextUri")
                if not next_uri:
                    break
                response = self.session.get(next_uri, timeout=REQUEST_TIMEOUT_SECONDS)
                response.raise_for_status()
                payload = response.json()
            logger.info(
                "Finished Trino query rows=%s elapsed_seconds=%.3f",
                row_count,
                time.perf_counter() - started_at,
            )
        except Exception:
            logger.exception(
                "Trino query failed rows=%s elapsed_seconds=%.3f",
                row_count,
                time.perf_counter() - started_at,
            )
            raise

    def _statement_headers(self) -> dict[str, str]:
        headers = {"X-Trino-Source": "ragflow"}
        if self.catalog:
            headers["X-Trino-Catalog"] = self.catalog
        if self.schema:
            headers["X-Trino-Schema"] = self.schema
        return headers

    def _raise_for_trino_error(self, payload: dict[str, Any]) -> None:
        error = payload.get("error")
        if not error:
            return
        message = error.get("message") or "Trino query failed"
        name = error.get("errorName")
        if name:
            message = f"{name}: {message}"
        logger.error("Trino returned query error error_name=%s message=%s", name, message)
        raise ValueError(message)

    @staticmethod
    def _rows_from_payload(payload: dict[str, Any]) -> Iterator[dict[str, Any]]:
        columns = [column["name"] for column in payload.get("columns") or []]
        for row in payload.get("data") or []:
            yield dict(zip(columns, row))

    def _document_from_row(self, row: dict[str, Any]) -> Document:
        content_lines = [f"{column}: {row.get(column, '')}" for column in self.content_columns]
        metadata = {column: row.get(column) for column in self.metadata_columns}
        row_id = str(row.get(self.id_column) if self.id_column else self._row_hash(row))
        updated_at = self._parse_datetime(row.get(self.timestamp_column)) if self.timestamp_column else self._parse_datetime(None)
        text = "\n".join(content_lines)
        blob = text.encode("utf-8")
        metadata.update(
            {
                "catalog": self.catalog,
                "schema": self.schema,
                "row_id": row_id,
            }
        )
        return Document(
            id=f"trino:{row_id}",
            source=DocumentSource.TRINO,
            semantic_identifier=row_id,
            extension="txt",
            blob=blob,
            doc_updated_at=updated_at,
            size_bytes=len(blob),
            metadata=metadata,
            fingerprint=hashlib.sha256(blob).hexdigest(),
        )

    @staticmethod
    def _row_hash(row: dict[str, Any]) -> str:
        return hashlib.sha256(repr(sorted(row.items())).encode("utf-8")).hexdigest()

    @staticmethod
    def _parse_datetime(value: Any) -> datetime:
        if isinstance(value, datetime):
            return value.astimezone(timezone.utc) if value.tzinfo else value.replace(tzinfo=timezone.utc)
        if isinstance(value, str) and value:
            try:
                return datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(timezone.utc)
            except ValueError:
                pass
        return datetime.fromtimestamp(0, tz=timezone.utc)

    def _batch_documents(self, docs: Iterator[Document]) -> Generator[list[Document], None, None]:
        batch: list[Document] = []
        for doc in docs:
            batch.append(doc)
            if len(batch) >= self.batch_size:
                yield batch
                batch = []
        if batch:
            yield batch

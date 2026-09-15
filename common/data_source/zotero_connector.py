"""Zotero library connector for syncing PDF attachments."""

from __future__ import annotations

import io
import logging
import zipfile
from collections.abc import Generator
from datetime import datetime, timezone
from typing import Any
from urllib.parse import urljoin

from common.data_source.config import BLOB_STORAGE_SIZE_THRESHOLD, INDEX_BATCH_SIZE, DocumentSource
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError,
)
from common.data_source.interfaces import LoadConnector, PollConnector, SecondsSinceUnixEpoch, SlimConnectorWithPermSync
from common.data_source.models import Document, GenerateDocumentsOutput, GenerateSlimDocumentOutput, SlimDocument
from common.data_source.utils import rl_requests
from common.ssrf_guard import assert_url_is_safe, pin_dns

logger = logging.getLogger(__name__)

ZOTERO_API_BASE = "https://api.zotero.org"
DEFAULT_WEBDAV_URL = ""
STORAGE_MODE_ZOTERO = "zotero_storage"
STORAGE_MODE_WEBDAV = "webdav"
PAGE_SIZE = 100


class ZoteroConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    def __init__(
        self,
        zotero_user_id: str | None = None,
        storage_mode: str = STORAGE_MODE_ZOTERO,
        webdav_url: str | None = None,
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        self.user_id = (zotero_user_id or "").strip()
        self.storage_mode = (storage_mode or STORAGE_MODE_ZOTERO).strip()
        self.webdav_url = (webdav_url or "").rstrip("/")
        self.batch_size = batch_size
        self.api_key: str | None = None
        self.webdav_password: str | None = None
        self.size_threshold = BLOB_STORAGE_SIZE_THRESHOLD

    @classmethod
    def build_connector(cls, config: dict[str, Any]) -> "ZoteroConnector":
        credentials = config.get("credentials") or {}
        user_id = (config.get("zotero_user_id") or credentials.get("zotero_user_id") or "").strip()
        connector = cls(
            zotero_user_id=user_id,
            storage_mode=config.get("storage_mode", STORAGE_MODE_ZOTERO),
            webdav_url=config.get("webdav_url", DEFAULT_WEBDAV_URL),
            batch_size=int(config.get("batch_size") or INDEX_BATCH_SIZE),
        )
        connector.load_credentials(credentials)
        return connector

    def load_credentials(self, credentials: dict[str, Any]) -> None:
        api_key = (credentials.get("zotero_api_key") or "").strip()
        if not api_key:
            raise ConnectorMissingCredentialError("Zotero API key is required")
        self.api_key = api_key
        self.webdav_password = credentials.get("webdav_password")

    def validate_connector_settings(self) -> None:
        if not self.user_id:
            raise ConnectorMissingCredentialError("Zotero user ID is required")
        if not self.api_key:
            raise ConnectorMissingCredentialError("Zotero API key is required")
        if self.storage_mode not in {STORAGE_MODE_ZOTERO, STORAGE_MODE_WEBDAV}:
            raise ConnectorValidationError("storage_mode must be 'zotero_storage' or 'webdav'")
        if self.storage_mode == STORAGE_MODE_WEBDAV:
            if not self.webdav_url:
                raise ConnectorValidationError("webdav_url is required when storage_mode is webdav")
            if not self.webdav_password:
                raise ConnectorMissingCredentialError("WebDAV password is required when storage_mode is webdav")
            self._assert_webdav_url(self.webdav_url)
            self._probe_webdav()
        self._list_attachment_items(start=0)

    def load_from_state(self) -> GenerateDocumentsOutput:
        yield from self._yield_documents()

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> GenerateDocumentsOutput:
        start_dt = datetime.fromtimestamp(start, tz=timezone.utc)
        end_dt = datetime.fromtimestamp(end, tz=timezone.utc)
        yield from self._yield_documents(start_dt=start_dt, end_dt=end_dt)

    def retrieve_all_slim_docs_perm_sync(self, callback: Any = None) -> GenerateSlimDocumentOutput:
        del callback
        batch: list[SlimDocument] = []
        for item in self._iter_pdf_attachments():
            batch.append(SlimDocument(id=self._document_id(item["key"])))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []
        if batch:
            yield batch

    def _yield_documents(
        self,
        start_dt: datetime | None = None,
        end_dt: datetime | None = None,
    ) -> GenerateDocumentsOutput:
        batch: list[Document] = []
        for item in self._iter_pdf_attachments():
            modified = self._parse_time(item["data"].get("dateModified"))
            if start_dt and modified <= start_dt:
                continue
            if end_dt and modified > end_dt:
                continue
            blob, filename = self._download_pdf(item)
            batch.append(self._build_document(item, blob, filename, modified))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []
        if batch:
            yield batch

    def _build_document(self, item: dict[str, Any], blob: bytes, filename: str, modified: datetime) -> Document:
        data = item.get("data") or {}
        title = (data.get("title") or filename or item.get("key", "")).strip()
        extension = "pdf"
        if "." in filename:
            extension = filename.rsplit(".", 1)[-1].lower()
        return Document(
            id=self._document_id(item["key"]),
            source=DocumentSource.ZOTERO.value,
            semantic_identifier=title,
            extension=extension,
            blob=blob,
            doc_updated_at=modified,
            size_bytes=len(blob),
            metadata={
                "source": "zotero",
                "zotero_item_key": item["key"],
                "zotero_parent": data.get("parentItem"),
                "filename": filename,
            },
        )

    def _document_id(self, attachment_key: str) -> str:
        return f"zotero:{self.user_id}:{attachment_key}"

    def _iter_pdf_attachments(self) -> Generator[dict[str, Any], None, None]:
        start = 0
        while True:
            items = self._list_attachment_items(start=start)
            if not items:
                break
            for item in items:
                data = item.get("data") or {}
                if data.get("itemType") != "attachment":
                    continue
                link_mode = (data.get("linkMode") or "").lower()
                if link_mode in {"linked_file", "linked_url"}:
                    continue
                content_type = (data.get("contentType") or "").lower()
                filename = (data.get("filename") or "").lower()
                if "pdf" in content_type or filename.endswith(".pdf"):
                    yield item
            start += len(items)
            if len(items) < PAGE_SIZE:
                break

    def _list_attachment_items(self, start: int) -> list[dict[str, Any]]:
        url = f"{ZOTERO_API_BASE}/users/{self.user_id}/items"
        params = {"itemType": "attachment", "format": "json", "limit": PAGE_SIZE, "start": start}
        response = rl_requests.get(url, headers=self._headers(), params=params, timeout=60)
        if response.status_code in {401, 403}:
            raise CredentialExpiredError("Zotero API key is invalid or expired")
        if response.status_code == 404:
            raise InsufficientPermissionsError("Zotero library was not found")
        response.raise_for_status()
        payload = response.json()
        if not isinstance(payload, list):
            raise ConnectorValidationError("Unexpected Zotero API response")
        return payload

    def _download_pdf(self, item: dict[str, Any]) -> tuple[bytes, str]:
        data = item.get("data") or {}
        filename = (data.get("filename") or f"{data.get('title', item['key'])}.pdf").strip()
        if self.storage_mode == STORAGE_MODE_WEBDAV:
            return self._download_via_webdav(item["key"], filename)
        return self._download_via_zotero_api(item["key"], filename)

    def _download_via_zotero_api(self, attachment_key: str, filename: str) -> tuple[bytes, str]:
        url = f"{ZOTERO_API_BASE}/users/{self.user_id}/items/{attachment_key}/file"
        response = rl_requests.get(url, headers=self._headers(), timeout=120, allow_redirects=True)
        if response.status_code >= 400:
            raise ConnectorValidationError(f"Failed to download Zotero attachment {attachment_key}: HTTP {response.status_code}")
        blob = response.content
        if len(blob) > self.size_threshold:
            raise ConnectorValidationError(f"Zotero attachment {attachment_key} exceeds maximum size")
        if not blob:
            raise ConnectorValidationError(f"Zotero attachment {attachment_key} returned no content")
        return blob, filename

    def _download_via_webdav(self, attachment_key: str, filename: str) -> tuple[bytes, str]:
        zip_url = urljoin(self.webdav_url + "/", f"{attachment_key}.zip")
        hostname, resolved_ip = self._assert_webdav_url(zip_url)
        with pin_dns(hostname, resolved_ip):
            response = rl_requests.get(
                zip_url,
                auth=(self.user_id, self.webdav_password or ""),
                timeout=120,
            )
        if response.status_code >= 400:
            raise ConnectorValidationError(f"Failed to download Zotero WebDAV archive {attachment_key}: HTTP {response.status_code}")
        return self._extract_pdf_from_zip(response.content, filename)

    def _assert_webdav_url(self, raw_url: str) -> tuple[str, str]:
        parsed_scheme = (raw_url.split(":", 1)[0] or "").lower()
        if parsed_scheme != "https":
            raise ConnectorValidationError("WebDAV URL must use HTTPS")
        try:
            return assert_url_is_safe(raw_url, allowed_schemes=frozenset({"https"}))
        except ValueError as exc:
            raise ConnectorValidationError(f"WebDAV URL is not allowed: {exc}") from exc

    def _probe_webdav(self) -> None:
        hostname, resolved_ip = self._assert_webdav_url(self.webdav_url)
        with pin_dns(hostname, resolved_ip):
            response = rl_requests.get(
                self.webdav_url,
                auth=(self.user_id, self.webdav_password or ""),
                timeout=30,
            )
        if response.status_code in {401, 403}:
            raise ConnectorMissingCredentialError("WebDAV authentication failed")
        if response.status_code >= 500:
            raise ConnectorValidationError(f"WebDAV probe failed: HTTP {response.status_code}")

    def _extract_pdf_from_zip(self, zip_bytes: bytes, fallback_name: str) -> tuple[bytes, str]:
        if len(zip_bytes) > self.size_threshold:
            raise ConnectorValidationError("Zotero WebDAV archive exceeds maximum size")
        with zipfile.ZipFile(io.BytesIO(zip_bytes)) as archive:
            for info in archive.infolist():
                name = info.filename
                if name.endswith("/") or not name.lower().endswith(".pdf"):
                    continue
                if info.file_size > self.size_threshold:
                    raise ConnectorValidationError("Zotero WebDAV PDF exceeds maximum size")
                if info.compress_size and info.file_size // max(info.compress_size, 1) > 100:
                    raise ConnectorValidationError("Zotero WebDAV archive compression ratio is too high")
                with archive.open(info) as handle:
                    chunks: list[bytes] = []
                    total = 0
                    while True:
                        chunk = handle.read(64 * 1024)
                        if not chunk:
                            break
                        total += len(chunk)
                        if total > self.size_threshold:
                            raise ConnectorValidationError("Zotero WebDAV PDF exceeds maximum size")
                        chunks.append(chunk)
                blob = b"".join(chunks)
                if blob:
                    return blob, name.split("/")[-1]
        raise ConnectorValidationError(f"No PDF found in Zotero WebDAV archive for {fallback_name}")

    def _headers(self) -> dict[str, str]:
        return {"Zotero-API-Key": self.api_key or "", "Zotero-API-Version": "3"}

    @staticmethod
    def _parse_time(value: str | None) -> datetime:
        if not value:
            return datetime.now(timezone.utc)
        normalized = value.replace("Z", "+00:00")
        try:
            return datetime.fromisoformat(normalized).astimezone(timezone.utc)
        except ValueError:
            return datetime.now(timezone.utc)

"""Zotero library connector for syncing PDF attachments."""

from __future__ import annotations

import io
import logging
import zipfile
from collections.abc import Generator
from datetime import datetime, timezone
from typing import Any
from urllib.parse import urljoin, urlparse

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
ZOTERO_API_KEY_HOSTS = frozenset({"api.zotero.org"})
MAX_FILE_REDIRECTS = 10
_READ_CHUNK_BYTES = 64 * 1024


def _read_response_capped(response, max_bytes: int) -> bytes:
    """Read a response body up to max_bytes without loading unbounded data."""
    chunks: list[bytes] = []
    total = 0
    try:
        for chunk in response.iter_content(chunk_size=_READ_CHUNK_BYTES):
            if not chunk:
                continue
            total += len(chunk)
            if total > max_bytes:
                response.close()
                return b""
            chunks.append(chunk)
    finally:
        response.close()
    return b"".join(chunks)


class ZoteroConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    def __init__(
        self,
        zotero_user_id: str | None,
        storage_mode: str = STORAGE_MODE_ZOTERO,
        webdav_url: str = DEFAULT_WEBDAV_URL,
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        self.user_id = (zotero_user_id or "").strip()
        self.storage_mode = (storage_mode or STORAGE_MODE_ZOTERO).strip()
        self.webdav_url = (webdav_url or DEFAULT_WEBDAV_URL).rstrip("/")
        self.batch_size = batch_size
        self.api_key: str | None = None
        self.webdav_username: str | None = None
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
        self.webdav_username = (credentials.get("webdav_username") or "").strip() or None
        self.webdav_password = credentials.get("webdav_password")

    def validate_local_settings(self) -> None:
        if not self.user_id:
            raise ConnectorMissingCredentialError("Zotero user ID is required")
        if not self.api_key:
            raise ConnectorMissingCredentialError("Zotero API key is required")
        if self.storage_mode not in {STORAGE_MODE_ZOTERO, STORAGE_MODE_WEBDAV}:
            raise ConnectorValidationError("storage_mode must be 'zotero_storage' or 'webdav'")
        if self.storage_mode != STORAGE_MODE_WEBDAV:
            return
        if not self.webdav_url:
            raise ConnectorValidationError("webdav_url is required when storage_mode is webdav")
        if urlparse(self.webdav_url).scheme.lower() != "https":
            raise ConnectorValidationError("WebDAV URL must use HTTPS")
        if not self.webdav_username:
            raise ConnectorMissingCredentialError("WebDAV username is required when storage_mode is webdav")
        if not self.webdav_password:
            raise ConnectorMissingCredentialError("WebDAV password is required when storage_mode is webdav")

    def validate_connector_settings(self) -> None:
        self.validate_local_settings()
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
        documents: list[Document] = []
        for item in self._iter_pdf_attachments():
            modified = self._parse_time(item["data"].get("dateModified"))
            if start_dt and modified <= start_dt:
                continue
            if end_dt and modified > end_dt:
                continue
            blob, filename = self._download_pdf(item)
            if not blob:
                continue
            documents.append(self._build_document(item, blob, filename, modified))
            if len(documents) >= self.batch_size:
                yield documents
                documents = []
        if documents:
            yield documents

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
        response = rl_requests.get(
            url,
            headers=self._headers(),
            params=params,
            timeout=60,
            allow_redirects=False,
        )
        if 300 <= response.status_code < 400:
            response.close()
            raise ConnectorValidationError("Unexpected redirect from Zotero API")
        if response.status_code in {401, 403}:
            raise CredentialExpiredError("Zotero API key is invalid or expired")
        if response.status_code == 404:
            raise InsufficientPermissionsError("Zotero library was not found")
        try:
            response.raise_for_status()
            payload = response.json()
        finally:
            response.close()
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
        blob = self._get_zotero_file(url)
        if blob is None:
            return b"", filename
        if len(blob) > self.size_threshold:
            logger.warning("Skipping oversized Zotero attachment %s", attachment_key)
            return b"", filename
        return blob, filename

    def _get_zotero_file(self, start_url: str) -> bytes | None:
        current = start_url
        for _ in range(MAX_FILE_REDIRECTS):
            parsed = urlparse(current)
            headers = {"Zotero-API-Version": "3"}
            if (parsed.hostname or "").lower() in ZOTERO_API_KEY_HOSTS:
                headers["Zotero-API-Key"] = self.api_key or ""
            try:
                host, ip = assert_url_is_safe(current, allowed_schemes=frozenset({"https"}))
            except ValueError as exc:
                logger.warning("Blocked Zotero file URL %s: %s", current, exc)
                return None
            with pin_dns(host, ip):
                response = rl_requests.get(current, headers=headers, timeout=120, allow_redirects=False)
            if 300 <= response.status_code < 400:
                location = response.headers.get("Location")
                response.close()
                if not location:
                    logger.warning("Zotero file redirect missing Location header")
                    return None
                current = urljoin(current, location)
                continue
            if response.status_code >= 400:
                logger.warning("Failed to download Zotero attachment: HTTP %s", response.status_code)
                response.close()
                return None
            blob = _read_response_capped(response, self.size_threshold + 1)
            if len(blob) > self.size_threshold:
                logger.warning("Skipping oversized Zotero attachment download")
                return None
            return blob
        logger.warning("Stopped after too many Zotero file redirects")
        return None

    def _download_via_webdav(self, attachment_key: str, filename: str) -> tuple[bytes, str]:
        self.validate_local_settings()
        zip_url = urljoin(self.webdav_url + "/", f"zotero/{attachment_key}.zip")
        try:
            host, ip = assert_url_is_safe(zip_url, allowed_schemes=frozenset({"https"}))
        except ValueError as exc:
            logger.warning("Blocked Zotero WebDAV URL %s: %s", zip_url, exc)
            return b"", filename
        with pin_dns(host, ip):
            response = rl_requests.get(
                zip_url,
                auth=(self.webdav_username or "", self.webdav_password or ""),
                timeout=120,
                allow_redirects=False,
                stream=True,
            )
        if response.status_code >= 400:
            logger.warning("Failed to download Zotero WebDAV archive %s: HTTP %s", attachment_key, response.status_code)
            response.close()
            return b"", filename
        zip_bytes = _read_response_capped(response, self.size_threshold + 1)
        if len(zip_bytes) > self.size_threshold:
            logger.warning("Skipping oversized Zotero WebDAV archive %s", attachment_key)
            return b"", filename
        return self._extract_pdf_from_zip(zip_bytes, filename)

    def _extract_pdf_from_zip(self, zip_bytes: bytes, fallback_name: str) -> tuple[bytes, str]:
        try:
            archive_ctx = zipfile.ZipFile(io.BytesIO(zip_bytes))
        except zipfile.BadZipFile:
            logger.warning("Malformed Zotero WebDAV archive for %s", fallback_name)
            return b"", fallback_name
        with archive_ctx as archive:
            for name in archive.namelist():
                if name.endswith("/") or not name.lower().endswith(".pdf"):
                    continue
                with archive.open(name) as handle:
                    blob = handle.read(self.size_threshold + 1)
                if len(blob) > self.size_threshold:
                    logger.warning("Skipping oversized PDF in Zotero WebDAV archive %s", name)
                    continue
                if blob:
                    return blob, name.split("/")[-1]
        logger.warning("No PDF found in Zotero WebDAV archive for %s", fallback_name)
        return b"", fallback_name

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

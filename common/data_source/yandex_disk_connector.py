"""Yandex Disk connector.

Syncs files and folders from Yandex Disk (https://cloud-api.yandex.net/v1/disk)
into RAGFlow knowledge bases. Mirrors the blob-storage connectors' shape:

- ``load_from_state()`` performs a full walk of the configured folder;
- ``list_keys()`` / ``get_value()`` expose a fingerprint-based incremental path
  (Yandex reports an md5 per file, so unchanged blobs are not re-downloaded);
- ``retrieve_all_slim_docs_perm_sync()`` backs the prune (deleted-files) task.
"""

import logging
from collections.abc import Iterator
from datetime import datetime, timezone
from typing import Any, Optional

import requests
import xxhash

from common.data_source.config import (
    BLOB_STORAGE_SIZE_THRESHOLD,
    INDEX_BATCH_SIZE,
    REQUEST_TIMEOUT_SECONDS,
    DocumentSource,
)
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError,
)
from common.data_source.interfaces import (
    FingerprintConnector,
    LoadConnector,
    PollConnector,
)
from common.data_source.models import (
    Document,
    GenerateDocumentsOutput,
    GenerateSlimDocumentOutput,
    KeyRecord,
    SecondsSinceUnixEpoch,
    SlimDocument,
)
from common.data_source.utils import get_file_ext

_YANDEX_DISK_API_BASE = "https://cloud-api.yandex.net/v1/disk"
_DEFAULT_PAGE_SIZE = 100


def _normalize_fingerprint(raw_md5: Optional[str]) -> Optional[str]:
    """Return a uniform 32-char hex fingerprint derived from a Yandex md5.

    Yandex already reports a 32-char hex md5 for files, but hashing it keeps the
    column format uniform with the other connectors and is robust to providers
    that occasionally omit or shorten the field.
    """
    if not raw_md5:
        return None
    return xxhash.xxh128(raw_md5.encode()).hexdigest()


class YandexDiskConnector(LoadConnector, PollConnector, FingerprintConnector):
    """Connect to a Yandex Disk folder and yield its files as Documents."""

    def __init__(
        self,
        path: str = "/",
        batch_size: int = INDEX_BATCH_SIZE,
        size_threshold: int | None = BLOB_STORAGE_SIZE_THRESHOLD,
    ) -> None:
        self.path = (path or "/").strip() or "/"
        self.batch_size = batch_size
        self.size_threshold = size_threshold
        self._token: Optional[str] = None
        self._session: Optional[requests.Session] = None
        self._allow_images: bool | None = None
        # Populated by list_keys() so a subsequent get_value(key) can find the
        # raw item metadata (path, name, size, modified, md5) without a second
        # API round-trip. Lifetime is one list_keys() pass.
        self._listing_cache: dict[str, dict[str, Any]] = {}
        self._filename_counts: dict[str, int] = {}

    @property
    def _http(self) -> requests.Session:
        if self._session is None:
            self._session = requests.Session()
        return self._session

    def set_allow_images(self, allow_images: bool) -> None:
        """Set whether to process images."""
        logging.info(f"Setting allow_images to {allow_images}.")
        self._allow_images = allow_images

    def load_credentials(self, credentials: dict[str, Any]) -> None:
        """Validate and store the Yandex Disk OAuth token."""
        token = credentials.get("oauth_token")
        if not token:
            raise ConnectorMissingCredentialError("Yandex Disk")
        self._token = token

    def _headers(self) -> dict[str, str]:
        # The Yandex Disk API only supports application/json; see
        # https://yandex.com/dev/disk-api/doc/en/concepts/quickstart.md
        return {
            "Authorization": f"OAuth {self._token}",
            "Accept": "application/json",
        }

    def _request(self, method: str, url: str, **kwargs: Any) -> requests.Response:
        kwargs.setdefault("timeout", REQUEST_TIMEOUT_SECONDS)
        kwargs.setdefault("headers", self._headers())
        resp = self._http.request(method, url, **kwargs)

        if resp.status_code == 401:
            raise CredentialExpiredError("Yandex Disk token is invalid or expired")
        if resp.status_code == 403:
            raise InsufficientPermissionsError(
                "Yandex Disk token lacks permission to access this resource"
            )
        if resp.status_code == 404:
            raise ConnectorValidationError(
                f"Yandex Disk path does not exist: {self.path}"
            )

        resp.raise_for_status()
        return resp

    @staticmethod
    def _parse_modified(raw: str) -> datetime:
        if raw.endswith("Z"):
            raw = raw[:-1] + "+00:00"
        return datetime.fromisoformat(raw).astimezone(timezone.utc)

    def _list_dir(self, path: str) -> list[dict[str, Any]]:
        """Return all items (files and folders) in one directory, following pagination."""
        items: list[dict[str, Any]] = []
        offset = 0
        while True:
            resp = self._request(
                "GET",
                f"{_YANDEX_DISK_API_BASE}/resources",
                params={"path": path, "limit": _DEFAULT_PAGE_SIZE, "offset": offset},
            )
            payload = resp.json()
            embedded = payload.get("_embedded") or {}
            page_items = embedded.get("items") or []
            items.extend(page_items)
            if len(page_items) < _DEFAULT_PAGE_SIZE:
                break
            offset += _DEFAULT_PAGE_SIZE
        return items

    def _iter_files(self) -> Iterator[dict[str, Any]]:
        """Depth-first walk of the configured folder, yielding file items only."""
        stack = [self.path]
        while stack:
            current = stack.pop()
            for item in self._list_dir(current):
                if item.get("type") == "dir":
                    stack.append(item["path"])
                elif item.get("type") == "file":
                    yield item

    def _build_document_from_item(
        self,
        item: dict[str, Any],
        filename_counts: dict[str, int],
    ) -> Optional[Document]:
        file_name = item["name"]
        size_bytes = item.get("size") or 0
        if (
            self.size_threshold is not None
            and isinstance(size_bytes, int)
            and size_bytes > self.size_threshold
        ):
            logging.warning(
                f"{file_name} exceeds size threshold of {self.size_threshold}. Skipping."
            )
            return None

        blob = self._download_file(item["path"])
        if blob is None:
            return None

        return Document(
            id=f"yandex_disk:{item['path']}",
            blob=blob,
            source=DocumentSource.YANDEX_DISK.value,
            semantic_identifier=self._get_semantic_id(item["path"], file_name, filename_counts),
            extension=get_file_ext(file_name),
            doc_updated_at=self._parse_modified(item["modified"]),
            size_bytes=size_bytes if size_bytes else 0,
            fingerprint=_normalize_fingerprint(item.get("md5")),
        )

    def _download_file(self, item_path: str) -> Optional[bytes]:
        """Resolve the temporary download href and fetch the file body."""
        try:
            resp = self._request(
                "GET",
                f"{_YANDEX_DISK_API_BASE}/resources/download",
                params={"path": item_path},
            )
            href = resp.json().get("href")
            if not href:
                return None
            resp = self._request("GET", href)
            return resp.content
        except Exception:
            logging.exception(f"Error downloading file {item_path}")
            return None

    def _get_semantic_id(self, path: str, file_name: str, filename_counts: dict[str, int]) -> str:
        """Use the full path only when filenames collide across folders."""
        if filename_counts.get(file_name, 0) > 1:
            relative_path = path
            if self.path != "/" and path.startswith(self.path):
                relative_path = path[len(self.path):]
            return relative_path.replace("/", " / ") if relative_path else file_name
        return file_name

    def _collect_files(
        self, start: datetime, end: datetime
    ) -> tuple[list[dict[str, Any]], dict[str, int]]:
        """Collect file items modified within the requested window."""
        all_items: list[dict[str, Any]] = []
        for item in self._iter_files():
            modified = self._parse_modified(item["modified"])
            if start < modified <= end:
                all_items.append(item)

        filename_counts: dict[str, int] = {}
        for item in all_items:
            filename_counts[item["name"]] = filename_counts.get(item["name"], 0) + 1
        return all_items, filename_counts

    def _yield_batches(
        self, start: datetime, end: datetime
    ) -> GenerateDocumentsOutput:
        all_items, filename_counts = self._collect_files(start, end)

        batch: list[Document] = []
        for item in all_items:
            try:
                doc = self._build_document_from_item(item, filename_counts)
                if doc is None:
                    continue
                batch.append(doc)
                if len(batch) == self.batch_size:
                    yield batch
                    batch = []
            except Exception:
                logging.exception(f"Error processing file {item.get('path')}")

        if batch:
            yield batch

    def list_keys(self) -> Iterator[KeyRecord]:
        """Enumerate the full folder keyspace with per-file fingerprints."""
        if self._token is None:
            raise ConnectorMissingCredentialError("Yandex Disk")

        all_items, filename_counts = self._collect_files(
            start=datetime(1970, 1, 1, tzinfo=timezone.utc),
            end=datetime.now(timezone.utc),
        )
        self._filename_counts = filename_counts
        self._listing_cache = {}

        for item in all_items:
            doc_id = f"yandex_disk:{item['path']}"
            self._listing_cache[doc_id] = item
            yield KeyRecord(
                key=doc_id,
                fingerprint=_normalize_fingerprint(item.get("md5")),
            )

    def get_value(self, key: str) -> Document:
        """Materialize the Document for a key previously yielded by list_keys()."""
        item = self._listing_cache.get(key)
        if item is None:
            raise KeyError(
                f"get_value({key!r}) called before list_keys() yielded the key, "
                "or after a subsequent list_keys() reset the cache"
            )
        doc = self._build_document_from_item(item, self._filename_counts)
        if doc is None:
            raise RuntimeError(f"Failed to materialize Document for key {key!r}")
        return doc

    def retrieve_all_slim_docs_perm_sync(
        self,
        callback: Any = None,
    ) -> GenerateSlimDocumentOutput:
        """Return a full current snapshot of file IDs without downloading content."""
        del callback

        all_items, _ = self._collect_files(
            start=datetime(1970, 1, 1, tzinfo=timezone.utc),
            end=datetime.now(timezone.utc),
        )

        batch: list[SlimDocument] = []
        for item in all_items:
            batch.append(SlimDocument(id=f"yandex_disk:{item['path']}"))
            if len(batch) == self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    def load_from_state(self) -> GenerateDocumentsOutput:
        """Load documents from state (full walk of the configured folder)."""
        logging.debug("Loading Yandex Disk files")
        return self._yield_batches(
            start=datetime(1970, 1, 1, tzinfo=timezone.utc),
            end=datetime.now(timezone.utc),
        )

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> GenerateDocumentsOutput:
        """Poll the source for documents modified in the given window."""
        if self._token is None:
            raise ConnectorMissingCredentialError("Yandex Disk")

        start_datetime = datetime.fromtimestamp(start, tz=timezone.utc)
        end_datetime = datetime.fromtimestamp(end, tz=timezone.utc)
        return self._yield_batches(start_datetime, end_datetime)

    def validate_connector_settings(self) -> None:
        """Validate the token and that the configured folder exists."""
        if self._token is None:
            raise ConnectorMissingCredentialError("Yandex Disk credentials not loaded.")

        # Lightweight validation: fetch disk info and list the configured path.
        self._request("GET", _YANDEX_DISK_API_BASE)
        self._list_dir(self.path)

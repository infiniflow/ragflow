"""Highspot sales-enablement data-source connector.

Syncs Highspot Spots and their items into a RAGFlow knowledge base via the
Highspot REST API. For each item it ingests the title + description as a
text document and, when the item is file-backed (pdf/docx/pptx/…),
downloads the file via the item ``content`` endpoint and ingests it too.

Auth is HTTP Basic with an API key + secret against a configurable base
URL (default ``https://api.highspot.com``).

Scope is chosen by the caller:

  * ``spot_ids`` — only these Spots are crawled.
  * empty — every Spot the key can see (discovered via ``/v1.0/spots``).

Incremental runs are scoped by the poll time window
(``since_epoch`` < item ``date_updated`` <= ``until_epoch``). Each item's
``date_updated`` is emitted as the document fingerprint, which the
indexing pipeline persists as ``content_hash`` so unchanged items are not
re-embedded. The connector keeps no cross-run state; incrementality is
owned by the global ``poll_range_start`` watermark the sync framework
persists, and the connector **fails closed** (any Spot/item error aborts
the run) so a partial failure can never advance that watermark past
content it never ingested.
"""

from __future__ import annotations

import logging
from datetime import datetime, timezone
from typing import Any, Generator, Iterable

import requests

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

_DEFAULT_BASE_URL = "https://api.highspot.com"
_API_PREFIX = "/v1.0"

# Highspot paginates list endpoints with start/limit; 100 is the documented
# maximum page size.
_PAGE_SIZE = 100

# Guard against a pathological / mis-paginating API never terminating.
_MAX_PAGES_PER_QUERY = 100_000

# HTTP timeout (connect, read) seconds.
_HTTP_TIMEOUT = (10, 60)

# File types we download from an item's content endpoint; the issue names
# pdf/docx/pptx and we include the rest of the common document set.
_SUPPORTED_EXTENSIONS = {
    ".pdf", ".docx", ".doc", ".pptx", ".ppt",
    ".xlsx", ".xls", ".txt", ".md", ".csv",
    ".html", ".htm", ".json", ".xml",
}


class HighspotCheckpoint(ConnectorCheckpoint):
    """Checkpoint marker for the Highspot connector.

    The connector keeps no cross-run state of its own: a single pass walks
    the configured Spots once and sets ``has_more=False``. Incremental
    scoping comes from the poll time window.
    """


class HighspotConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """Highspot sales-enablement connector.

    Authenticates with an API key + secret (HTTP Basic), resolves the
    configured Spots, and pages through each Spot's items. Each item's
    ``date_updated`` is surfaced as the document fingerprint so the
    pipeline can skip re-embedding unchanged items across runs.
    """

    def __init__(
        self,
        base_url: str | None = None,
        spot_ids: Iterable[str] | None = None,
        batch_size: int = INDEX_BATCH_SIZE,
        include_files: bool = True,
        allow_images: bool = False,
    ) -> None:
        self.base_url = (base_url or _DEFAULT_BASE_URL).rstrip("/")
        self.api_base = f"{self.base_url}{_API_PREFIX}"
        self.spot_ids = [str(s).strip() for s in (spot_ids or []) if str(s).strip()]
        self.batch_size = batch_size
        self.include_files = include_files
        self.allow_images = allow_images
        self._session: requests.Session | None = None

    # ------------------------------------------------------------------
    # Auth
    # ------------------------------------------------------------------

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        api_key = credentials.get("api_key") or credentials.get("key")
        api_secret = credentials.get("api_secret") or credentials.get("secret")
        if not (api_key and api_secret):
            raise ConnectorMissingCredentialError(
                "Highspot: api_key and api_secret are required"
            )
        session = requests.Session()
        session.auth = (str(api_key), str(api_secret))
        session.headers["Accept"] = "application/json"
        self._session = session
        return None

    # ------------------------------------------------------------------
    # Validation
    # ------------------------------------------------------------------

    def validate_connector_settings(self) -> None:
        if self._session is None:
            raise ConnectorMissingCredentialError("Highspot")

        # One cheap call proving base URL + credentials are valid.
        resp = self._get(f"{self.api_base}/spots", params={"start": 0, "limit": 1})
        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"Highspot credentials rejected or insufficient permissions "
                f"(HTTP {resp.status_code}): {resp.text[:300]}"
            )
        if resp.status_code == 404:
            raise ConnectorValidationError(
                f"Highspot: REST API not found at {self.api_base} — check the base URL "
                f"(HTTP 404): {resp.text[:300]}"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"Highspot validation failed (HTTP {resp.status_code}): {resp.text[:300]}"
            )

        # Verify each configured Spot exists, so a typo'd Spot surfaces as a
        # clear error rather than a silently empty crawl.
        for spot_id in self.spot_ids:
            sresp = self._get(f"{self.api_base}/spots/{spot_id}")
            if sresp.status_code == 404:
                raise ConnectorValidationError(
                    f"Highspot: Spot '{spot_id}' not found."
                )
            if sresp.status_code in (401, 403):
                raise InsufficientPermissionsError(
                    f"Highspot: insufficient permissions for Spot '{spot_id}' "
                    f"(HTTP {sresp.status_code})"
                )
            if sresp.status_code >= 400:
                raise UnexpectedValidationError(
                    f"Highspot: failed to resolve Spot '{spot_id}' "
                    f"(HTTP {sresp.status_code}): {sresp.text[:300]}"
                )

    # ------------------------------------------------------------------
    # Checkpoint helpers
    # ------------------------------------------------------------------

    def build_dummy_checkpoint(self) -> HighspotCheckpoint:
        return HighspotCheckpoint(has_more=True)

    def validate_checkpoint_json(self, checkpoint_json: str) -> HighspotCheckpoint:
        try:
            return HighspotCheckpoint.model_validate_json(checkpoint_json)
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
        if not isinstance(checkpoint, HighspotCheckpoint):
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

        Fails closed: if any Spot/item enumeration errors, the error
        propagates so the prune collector aborts and skips deletion rather
        than treating a partial snapshot as authoritative and wrongly
        deleting still-valid documents.
        """
        if self._session is None:
            raise ConnectorMissingCredentialError("Highspot")

        batch: list[SlimDocument] = []
        for spot_id in self._resolve_spot_ids():
            for item in self._iter_spot_items(spot_id, since_epoch=None, until_epoch=None):
                item_id = item.get("id")
                if not item_id:
                    continue
                # Text document id.
                text_id = f"item:{item_id}"
                if callback:
                    callback(text_id, spot_id)
                batch.append(SlimDocument(id=text_id))
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []
                # File document id (only when the item is file-backed and we
                # ingest files), so prune reconciles both sides.
                if self.include_files and _item_filename(item):
                    file_id = f"item:{item_id}:content"
                    if callback:
                        callback(file_id, spot_id)
                    batch.append(SlimDocument(id=file_id))
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
        checkpoint: HighspotCheckpoint | None = None,
        since_epoch: float | None = None,
        until_epoch: float | None = None,
    ):
        from common.data_source.models import Document

        if self._session is None:
            raise ConnectorMissingCredentialError("Highspot")

        batch: list[Document] = []

        for spot_id in self._resolve_spot_ids():
            for item in self._iter_spot_items(spot_id, since_epoch, until_epoch):
                item_id = item.get("id")
                if not item_id:
                    continue

                updated = _parse_time(item.get("date_updated") or item.get("date_added"))
                doc_updated_at = updated or datetime.now(timezone.utc)
                fingerprint = updated.isoformat() if updated else None

                title = _str(item.get("title")) or str(item_id)
                description = _str(item.get("description"))
                url = _str(item.get("url"))

                # Title + description text document (always present).
                body_parts = [f"Title: {title}"]
                if description:
                    body_parts.append(f"Description: {description}")
                if url:
                    body_parts.append(f"URL: {url}")
                body = "\n\n".join(body_parts)
                blob = body.encode("utf-8")
                metadata = {"spot": spot_id, "item_id": str(item_id)}
                if url:
                    metadata["url"] = url

                batch.append(Document(
                    id=f"item:{item_id}",
                    source="highspot",
                    semantic_identifier=title,
                    extension=".txt",
                    blob=blob,
                    doc_updated_at=doc_updated_at,
                    size_bytes=len(blob),
                    fingerprint=fingerprint,
                    metadata=metadata,
                ))
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

                # Downloadable file content, when file-backed.
                if self.include_files:
                    filename = _item_filename(item)
                    if filename and _has_supported_extension(filename, self.allow_images):
                        file_doc = self._download_item_content(
                            item_id, filename, spot_id, doc_updated_at, fingerprint
                        )
                        if file_doc is not None:
                            batch.append(file_doc)
                            if len(batch) >= self.batch_size:
                                yield batch
                                batch = []

        if batch:
            yield batch

        if checkpoint is not None:
            checkpoint.has_more = False

    def _resolve_spot_ids(self) -> Generator[str, None, None]:
        """Yield the Spot IDs to crawl — the configured set, or every Spot."""
        if self.spot_ids:
            yield from self.spot_ids
            return
        for spot in self._paginate(f"{self.api_base}/spots", params=None, context="spots"):
            spot_id = spot.get("id")
            if spot_id:
                yield str(spot_id)

    def _iter_spot_items(
        self, spot_id: str, since_epoch: float | None, until_epoch: float | None
    ) -> Generator[dict[str, Any], None, None]:
        """Yield items in a Spot within the time window.

        Highspot's items endpoint has no server-side ``date_updated`` filter,
        so we page through every item and apply the strict-lower /
        inclusive-upper window client-side. An item with no parseable
        ``date_updated`` is always included (better to re-index than to drop
        it; the content-hash dedup makes a re-index cheap).
        """
        params = {"spot": spot_id}
        for item in self._paginate(
            f"{self.api_base}/items", params=params, context=f"items(spot={spot_id})"
        ):
            ts = _epoch_of(item.get("date_updated") or item.get("date_added"))
            if ts is not None:
                if since_epoch and ts <= since_epoch:
                    continue
                if until_epoch and ts > until_epoch:
                    continue
            yield item

    def _paginate(
        self, url: str, params: dict[str, Any] | None, context: str
    ) -> Generator[dict[str, Any], None, None]:
        """Yield entries from a Highspot ``collection`` list, following
        ``start``/``limit`` pagination until a short/empty page is returned."""
        start = 0
        page = 0
        while True:
            if page > _MAX_PAGES_PER_QUERY:
                raise UnexpectedValidationError(
                    f"Highspot: page limit exceeded for {context}; aborting runaway crawl"
                )
            page_params = dict(params or {})
            page_params.update({"start": start, "limit": _PAGE_SIZE})
            resp = self._get(url, params=page_params)
            if resp.status_code in (401, 403):
                raise InsufficientPermissionsError(
                    f"Highspot: insufficient permissions ({context}, HTTP {resp.status_code})"
                )
            if resp.status_code >= 400:
                raise UnexpectedValidationError(
                    f"Highspot: {context} failed (HTTP {resp.status_code}): {resp.text[:300]}"
                )
            try:
                payload = resp.json()
            except ValueError as exc:
                raise UnexpectedValidationError(
                    f"Highspot: non-JSON response ({context}): {exc}"
                ) from exc

            entries = _collection(payload)
            for entry in entries:
                if isinstance(entry, dict):
                    yield entry

            if len(entries) < _PAGE_SIZE:
                return
            start += _PAGE_SIZE
            page += 1

    def _download_item_content(
        self,
        item_id: str,
        filename: str,
        spot_id: str,
        doc_updated_at: datetime,
        fingerprint: str | None,
    ):
        """Download an item's file content. Returns a Document, or ``None``
        when the item has no downloadable content. Fails closed on transient
        errors so the run aborts rather than advancing the watermark past a
        file it never ingested."""
        from common.data_source.models import Document

        assert self._session is not None
        url = f"{self.api_base}/items/{item_id}/content"
        try:
            resp = self._session.get(url, timeout=_HTTP_TIMEOUT)
        except requests.RequestException as exc:
            raise UnexpectedValidationError(
                f"Highspot: failed to download content for item {item_id}: {exc}"
            ) from exc

        # 404/415/422 → the item simply has no downloadable file (e.g. a URL
        # bookmark): not an error, just skip the file side.
        if resp.status_code in (404, 415, 422):
            return None
        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"Highspot: insufficient permissions reading item {item_id} content "
                f"(HTTP {resp.status_code})"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"Highspot: content download for item {item_id} failed "
                f"(HTTP {resp.status_code})"
            )

        data = resp.content
        return Document(
            id=f"item:{item_id}:content",
            source="highspot",
            semantic_identifier=filename,
            extension=_extension(filename),
            blob=data,
            doc_updated_at=doc_updated_at,
            size_bytes=len(data),
            fingerprint=fingerprint,
            metadata={"spot": spot_id, "item_id": str(item_id), "filename": filename},
        )

    # ------------------------------------------------------------------
    # HTTP helper
    # ------------------------------------------------------------------

    def _get(self, url: str, params: dict[str, Any] | None = None) -> requests.Response:
        assert self._session is not None
        try:
            return self._session.get(url, params=params, timeout=_HTTP_TIMEOUT)
        except requests.RequestException as exc:
            raise UnexpectedValidationError(
                f"Highspot: request to {url} failed: {exc}"
            ) from exc


# ----------------------------------------------------------------------
# Module-level helpers
# ----------------------------------------------------------------------

def _str(value: Any) -> str:
    return value if isinstance(value, str) else ("" if value is None else str(value))


def _collection(payload: Any) -> list[Any]:
    """Pull the list out of a Highspot response (``{"collection": [...]}``)."""
    if isinstance(payload, dict):
        coll = payload.get("collection")
        if isinstance(coll, list):
            return coll
    if isinstance(payload, list):
        return payload
    return []


def _item_filename(item: dict[str, Any]) -> str:
    """Best-effort filename for a file-backed item; empty for URL/web items."""
    name = _str(item.get("content_name") or item.get("filename"))
    if name:
        return name
    # Some items only expose a content_type; derive an extension when the
    # title already carries one.
    title = _str(item.get("title"))
    if "." in title and _extension(title) in _SUPPORTED_EXTENSIONS:
        return title
    return ""


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


def _epoch_of(value: Any) -> float | None:
    parsed = _parse_time(value)
    return parsed.timestamp() if parsed else None


def _parse_time(value: Any) -> datetime | None:
    """Parse a Highspot timestamp into an aware UTC datetime.

    Highspot emits ISO-8601 (``2024-05-01T10:20:30Z`` / ``...+0000``); some
    fields are epoch seconds/milliseconds. Anything unparseable yields
    ``None`` so a single odd timestamp never aborts the crawl.
    """
    if value is None:
        return None
    if isinstance(value, (int, float)):
        # Heuristic: ms vs s. Highspot epoch fields are seconds; values far in
        # the future are treated as milliseconds.
        seconds = value / 1000.0 if value > 10_000_000_000 else float(value)
        try:
            return datetime.fromtimestamp(seconds, tz=timezone.utc)
        except (ValueError, OverflowError):
            return None
    if not isinstance(value, str):
        return None
    text = value.strip()
    if not text:
        return None
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    elif len(text) >= 5 and text[-5] in "+-" and text[-3] != ":":
        text = text[:-2] + ":" + text[-2:]
    try:
        parsed = datetime.fromisoformat(text)
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)

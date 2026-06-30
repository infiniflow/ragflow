"""Axero (Communifire) data-source connector.

Syncs spaces and their content from an Axero / Communifire intranet into a
RAGFlow knowledge base via the Axero REST API.

Auth is a single REST API key sent in the ``Rest-Api-Key`` header against a
configurable base URL.

Scope is chosen by the caller:

  * ``space_ids`` — only these spaces are crawled.
  * empty — every space the key can see (discovered via ``api/spaces``).

Content types map to Axero ``EntityType`` codes (article, wiki, blog,
forum); the caller picks which to ingest. Each content item's HTML body is
ingested as a document; optionally its attached files are downloaded too.

Incremental runs are scoped by the poll time window
(``since_epoch`` < ``DateUpdated`` <= ``until_epoch``). ``api/content/list``
is sorted by ``DateUpdated`` descending, so within each space/type we stop
paginating as soon as we cross below the lower bound. Each item's
``DateUpdated`` is emitted as the document fingerprint, which the indexing
pipeline persists as ``content_hash`` so unchanged items are not
re-embedded. The connector itself keeps no cross-run state.
"""

from __future__ import annotations

import logging
import re
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

# Axero EntityType codes for the content types we ingest. Names are what the
# UI / config exposes; values are the integers the REST API expects.
_ENTITY_TYPES: dict[str, int] = {
    "article": 3,
    "wiki": 9,
    "blog": 4,
    "forum": 1,
}
_DEFAULT_CONTENT_TYPES = list(_ENTITY_TYPES.keys())

# api/content/list returns at most this many items per page.
_PAGE_SIZE = 100

# Guard against a pathological / mis-paginating API never terminating.
_MAX_PAGES_PER_QUERY = 100_000

# HTTP timeout (connect, read) seconds.
_HTTP_TIMEOUT = (10, 60)

# Extensions accepted for attachment ingestion; mirrors the other
# file-based connectors.
_SUPPORTED_ATTACHMENT_EXTENSIONS = {
    ".pdf", ".docx", ".doc", ".xlsx", ".xls",
    ".pptx", ".ppt", ".txt", ".md", ".csv",
    ".html", ".htm", ".json", ".xml",
}


class AxeroCheckpoint(ConnectorCheckpoint):
    """Checkpoint marker for the Axero connector.

    The connector keeps no cross-run state of its own: a single
    ``load_from_checkpoint`` pass walks the configured spaces / content
    types once and sets ``has_more=False``. Incremental scoping comes from
    the poll time window, and per-item change detection from the document
    fingerprint (``DateUpdated``) the pipeline persists as ``content_hash``.
    """


class AxeroConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """Axero (Communifire) data-source connector.

    Authenticates with a REST API key, resolves the configured spaces, and
    pages through each selected content type sorted by ``DateUpdated``.
    Each item's ``DateUpdated`` is surfaced as the document fingerprint so
    the pipeline can skip re-embedding unchanged items across runs.
    """

    def __init__(
        self,
        base_url: str,
        space_ids: Iterable[str] | None = None,
        content_types: Iterable[str] | None = None,
        batch_size: int = INDEX_BATCH_SIZE,
        include_attachments: bool = False,
    ) -> None:
        self.base_url = (base_url or "").rstrip("/")
        self.api_base = f"{self.base_url}/api"
        self.space_ids = [str(s).strip() for s in (space_ids or []) if str(s).strip()]
        # Map requested content-type names to EntityType codes, ignoring
        # anything unknown; fall back to the full set when none given.
        requested = [str(c).strip().lower() for c in (content_types or []) if str(c).strip()]
        if not requested:
            requested = _DEFAULT_CONTENT_TYPES
        self.entity_types = [
            _ENTITY_TYPES[name] for name in requested if name in _ENTITY_TYPES
        ]
        self.batch_size = batch_size
        self.include_attachments = include_attachments
        self._session: requests.Session | None = None

    # ------------------------------------------------------------------
    # Auth
    # ------------------------------------------------------------------

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        if not self.base_url:
            raise ConnectorMissingCredentialError("Axero: base_url is required")

        api_key = credentials.get("api_key") or credentials.get("axero_api_token")
        if not api_key:
            raise ConnectorMissingCredentialError(
                "Axero: api_key is required"
            )

        session = requests.Session()
        session.headers["Rest-Api-Key"] = str(api_key)
        session.headers["Accept"] = "application/json"
        self._session = session
        return None

    # ------------------------------------------------------------------
    # Validation
    # ------------------------------------------------------------------

    def validate_connector_settings(self) -> None:
        if self._session is None:
            raise ConnectorMissingCredentialError("Axero")
        if not self.entity_types:
            raise ConnectorValidationError(
                "Axero: no valid content types selected — choose at least one of "
                f"{sorted(_ENTITY_TYPES)}."
            )

        # One cheap call proving base URL + API key are valid.
        try:
            resp = self._session.get(
                f"{self.api_base}/spaces",
                params={"StartPage": 1, "PageSize": 1},
                timeout=_HTTP_TIMEOUT,
            )
        except requests.RequestException as exc:
            raise UnexpectedValidationError(
                f"Axero: cannot reach {self.base_url}: {exc}"
            ) from exc

        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"Axero API key rejected or insufficient permissions "
                f"(HTTP {resp.status_code}): {resp.text[:300]}"
            )
        if resp.status_code == 404:
            raise ConnectorValidationError(
                f"Axero: REST API not found at {self.api_base} — check the base URL "
                f"(HTTP 404): {resp.text[:300]}"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"Axero validation failed (HTTP {resp.status_code}): {resp.text[:300]}"
            )

    # ------------------------------------------------------------------
    # Checkpoint helpers
    # ------------------------------------------------------------------

    def build_dummy_checkpoint(self) -> AxeroCheckpoint:
        return AxeroCheckpoint(has_more=True)

    def validate_checkpoint_json(self, checkpoint_json: str) -> AxeroCheckpoint:
        try:
            return AxeroCheckpoint.model_validate_json(checkpoint_json)
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
        if not isinstance(checkpoint, AxeroCheckpoint):
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
        """Yield batches of slim documents for prune / permission sync."""
        if self._session is None:
            raise ConnectorMissingCredentialError("Axero")

        batch: list[SlimDocument] = []
        for item in self._walk_content(since_epoch=None, until_epoch=None):
            doc_id = _content_doc_id(item)
            if not doc_id:
                continue
            if callback:
                callback(doc_id, _str(item.get("ContentTitle")))
            batch.append(SlimDocument(id=doc_id))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    # ------------------------------------------------------------------
    # Internal content iteration
    # ------------------------------------------------------------------

    def _iter_documents(
        self,
        checkpoint: AxeroCheckpoint | None = None,
        since_epoch: float | None = None,
        until_epoch: float | None = None,
    ):
        from common.data_source.models import Document

        if self._session is None:
            raise ConnectorMissingCredentialError("Axero")

        batch: list[Document] = []

        for item in self._walk_content(since_epoch, until_epoch):
            content_id = _content_doc_id(item)
            if not content_id:
                continue

            updated = _parse_axero_time(item.get("DateUpdated") or item.get("DateCreated"))
            doc_updated_at = updated or datetime.now(timezone.utc)
            fingerprint = updated.isoformat() if updated else None

            title = _str(item.get("ContentTitle")) or content_id
            body_html = _str(item.get("ContentBody")) or _str(item.get("ContentSummary"))
            url = _str(item.get("ContentURL"))
            space_name = _str(item.get("SpaceName"))

            data = body_html.encode("utf-8")
            metadata = {
                "content_id": content_id,
                "space": space_name,
                "url": url,
            }

            yield_doc = Document(
                id=content_id,
                source="axero",
                semantic_identifier=title,
                extension=".html",
                blob=data,
                doc_updated_at=doc_updated_at,
                size_bytes=len(data),
                fingerprint=fingerprint,
                metadata=metadata,
            )
            batch.append(yield_doc)
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

            if self.include_attachments:
                for att in self._iter_attachments(item, doc_updated_at):
                    batch.append(att)
                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []

        if batch:
            yield batch

        if checkpoint is not None:
            checkpoint.has_more = False

    def _walk_content(
        self, since_epoch: float | None, until_epoch: float | None
    ) -> Generator[dict[str, Any], None, None]:
        """Yield content items across the configured spaces and entity types.

        ``api/content/list`` is sorted by ``DateUpdated`` descending, so for
        each (space, entity-type) pair we page from newest to oldest and
        stop as soon as an item falls at or below the lower bound — there is
        nothing older worth fetching.
        """
        # ``None`` means "every space" — one query with no SpaceID filter.
        spaces: list[str | None] = list(self.space_ids) if self.space_ids else [None]

        for space_id in spaces:
            for entity_type in self.entity_types:
                page = 1
                stop = False
                while not stop:
                    if page > _MAX_PAGES_PER_QUERY:
                        raise UnexpectedValidationError(
                            "Axero: page limit exceeded; aborting to avoid a runaway crawl"
                        )
                    items = self._list_content(entity_type, space_id, page)
                    if not items:
                        break

                    for item in items:
                        ts = _epoch_of(item.get("DateUpdated") or item.get("DateCreated"))
                        if ts is not None:
                            if since_epoch and ts <= since_epoch:
                                # Sorted descending: everything after this is
                                # older still, so we're done with this query.
                                stop = True
                                break
                            if until_epoch and ts > until_epoch:
                                # Too new for this snapshot; skip but keep
                                # scanning — older items may still be in range.
                                continue
                        yield item

                    page += 1

    def _list_content(
        self, entity_type: int, space_id: str | None, page: int
    ) -> list[dict[str, Any]]:
        """Fetch one page of content for an entity type (and optional space)."""
        assert self._session is not None
        params: dict[str, Any] = {
            "EntityType": entity_type,
            "SortColumn": "DateUpdated",
            "SortOrder": "Descending",
            "StartPage": page,
            "PageSize": _PAGE_SIZE,
        }
        if space_id:
            params["SpaceID"] = space_id

        url = f"{self.api_base}/content/list"
        try:
            resp = self._session.get(url, params=params, timeout=_HTTP_TIMEOUT)
        except requests.RequestException as exc:
            raise UnexpectedValidationError(
                f"Axero: listing content (type={entity_type}) failed: {exc}"
            ) from exc

        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"Axero: insufficient permissions listing content "
                f"(HTTP {resp.status_code})"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"Axero: listing content (type={entity_type}) failed "
                f"(HTTP {resp.status_code}): {resp.text[:300]}"
            )

        try:
            payload = resp.json()
        except ValueError as exc:
            raise UnexpectedValidationError(
                f"Axero: non-JSON content response (type={entity_type}): {exc}"
            ) from exc

        return _extract_list(payload)

    def _iter_attachments(
        self, item: dict[str, Any], doc_updated_at: datetime
    ) -> Generator[Any, None, None]:
        """Yield Document objects for an item's downloadable attachments.

        Defensive about shape: Axero exposes attachments under a few field
        names and each entry may carry its URL under different keys. Anything
        we can't resolve to a supported file URL is skipped.
        """
        from common.data_source.models import Document

        assert self._session is not None
        attachments = (
            item.get("Attachments")
            or item.get("AttachmentList")
            or item.get("Files")
            or []
        )
        if not isinstance(attachments, list):
            return

        for att in attachments:
            if not isinstance(att, dict):
                continue
            name = _str(
                att.get("FileName") or att.get("Name") or att.get("Title")
            )
            link = _str(
                att.get("DownloadUrl")
                or att.get("DownloadURL")
                or att.get("AttachmentUrl")
                or att.get("Url")
                or att.get("URL")
            )
            if not link or not _has_supported_extension(name):
                continue

            download_url = link if link.startswith("http") else f"{self.base_url}/{link.lstrip('/')}"
            try:
                resp = self._session.get(download_url, timeout=_HTTP_TIMEOUT)
            except requests.RequestException as exc:
                raise UnexpectedValidationError(
                    f"Axero: failed to download attachment {name}: {exc}"
                ) from exc
            if resp.status_code == 404:
                logger.warning("Axero: attachment %s missing (404); skipping", name)
                continue
            if resp.status_code >= 400:
                raise UnexpectedValidationError(
                    f"Axero: failed to download attachment {name} "
                    f"(HTTP {resp.status_code})"
                )

            data = resp.content
            att_id = _str(att.get("AttachmentID") or att.get("FileID") or att.get("ID")) or name
            yield Document(
                id=f"attachment:{att_id}",
                source="axero",
                semantic_identifier=name,
                extension=_extension(name),
                blob=data,
                doc_updated_at=doc_updated_at,
                size_bytes=len(data),
                fingerprint=None,
                metadata={"attachment": "true", "filename": name},
            )


# ----------------------------------------------------------------------
# Module-level helpers
# ----------------------------------------------------------------------

def _str(value: Any) -> str:
    return value if isinstance(value, str) else ("" if value is None else str(value))


def _content_doc_id(item: dict[str, Any]) -> str:
    cid = item.get("ContentID")
    if cid is None:
        cid = item.get("ContentId") or item.get("ID")
    return "" if cid is None else f"content:{cid}"


def _extract_list(payload: Any) -> list[dict[str, Any]]:
    """Pull the content array out of an Axero response of varying shape."""
    if isinstance(payload, list):
        return [x for x in payload if isinstance(x, dict)]
    if isinstance(payload, dict):
        for key in ("ResponseData", "Data", "Results", "Entities", "Content"):
            value = payload.get(key)
            if isinstance(value, list):
                return [x for x in value if isinstance(x, dict)]
    return []


def _extension(name: str) -> str:
    if "." not in name:
        return ""
    return "." + name.rsplit(".", 1)[-1].lower()


def _has_supported_extension(name: str) -> bool:
    return _extension(name) in _SUPPORTED_ATTACHMENT_EXTENSIONS


def _epoch_of(value: Any) -> float | None:
    parsed = _parse_axero_time(value)
    return parsed.timestamp() if parsed else None


def _parse_axero_time(value: Any) -> datetime | None:
    """Parse an Axero timestamp into an aware UTC datetime.

    Axero emits ISO-8601 (``2024-05-01T10:20:30Z`` / ``...+0000``) and
    sometimes the legacy .NET ``/Date(1714558830000)/`` form. Anything
    unparseable yields ``None`` so a single odd timestamp never aborts the
    crawl.
    """
    if value is None:
        return None
    if isinstance(value, (int, float)):
        return datetime.fromtimestamp(value / 1000.0, tz=timezone.utc)
    if not isinstance(value, str):
        return None

    text = value.strip()
    if not text:
        return None

    # Legacy .NET "/Date(1714558830000)/" (optionally with a TZ offset).
    dotnet = re.search(r"/Date\((-?\d+)", text)
    if dotnet:
        try:
            return datetime.fromtimestamp(int(dotnet.group(1)) / 1000.0, tz=timezone.utc)
        except (ValueError, OverflowError):
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

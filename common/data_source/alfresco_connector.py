"""Hyland Alfresco data-source connector.

Crawls a Hyland Alfresco repository's document-library nodes via the
Alfresco REST API and ingests file content into a RAGFlow knowledge base.

Auth supports two mutually exclusive modes, selected explicitly by the
caller-supplied ``auth_mode`` (the UI hides the other mode's fields but
does not clear them, so we must not guess from whichever field happens to
be populated). When ``auth_mode`` is absent (older configs / direct API
callers) we fall back to field precedence:

  1. **Basic auth** — ``username`` + ``password``; the classic Alfresco
     credential.
  2. **OAuth2** — ``access_token`` bearer token issued by the Alfresco
     Identity Service / an external IdP.

Scope is chosen by the caller:

  * ``site_ids`` — each site's ``documentLibrary`` container is resolved
    and crawled.
  * ``root_node_ids`` — explicit folder node IDs to crawl.
  * neither — the whole repository (``-root-``).

Incremental runs are scoped by the poll time window
(``since_epoch`` < node ``modifiedAt`` <= ``until_epoch``). Each node's
version label (or ``modifiedAt`` when versioning is off) is emitted as the
document fingerprint, which the indexing pipeline persists as
``content_hash`` so unchanged nodes are not re-embedded. The connector
itself keeps no cross-run state.
"""

from __future__ import annotations

import logging
from collections import deque
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

# Extensions we ingest; mirrors the set used by the other file-based
# connectors so behaviour is consistent across all sources.
_SUPPORTED_EXTENSIONS = {
    ".pdf", ".docx", ".doc", ".xlsx", ".xls",
    ".pptx", ".ppt", ".txt", ".md", ".csv",
    ".html", ".htm", ".json", ".xml",
}

# Alfresco public REST API path under the configured base URL.
_API_PATH = "/alfresco/api/-default-/public/alfresco/versions/1"

# Page size for the children listing endpoint.
_PAGE_SIZE = 100

# Guard against pathological / cyclic structures.
_MAX_NODES_SCANNED = 5_000_000

# Fields the listing endpoint should return so we can filter and emit
# documents without a second round-trip per node.
_LISTING_INCLUDE = "properties,path"

# HTTP timeout (connect, read) seconds.
_HTTP_TIMEOUT = (10, 60)

# Well-known node aliases that always resolve, so they need no existence
# check during validation.
_NODE_ALIASES = {"-root-", "-my-", "-shared-"}

# Cap on version-history entries fetched per node (newest first).
_VERSION_HISTORY_LIMIT = 100


class AlfrescoCheckpoint(ConnectorCheckpoint):
    """Checkpoint marker for the Alfresco connector.

    The connector keeps no cross-run state of its own: a single
    ``load_from_checkpoint`` pass crawls the configured roots once and
    sets ``has_more=False``. Incremental scoping comes from the poll time
    window, and per-node change detection from the document fingerprint
    (version label / ``modifiedAt``) the pipeline persists as
    ``content_hash``.
    """


class AlfrescoConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """Hyland Alfresco data-source connector.

    Authenticates with basic auth or an OAuth2 bearer token, resolves the
    configured sites/folders to node IDs, and recursively crawls their
    children. Each file node's version label is surfaced as the document
    fingerprint so the pipeline can skip re-embedding unchanged nodes
    across runs.
    """

    def __init__(
        self,
        base_url: str,
        site_ids: Iterable[str] | None = None,
        root_node_ids: Iterable[str] | None = None,
        batch_size: int = INDEX_BATCH_SIZE,
        include_version_history: bool = False,
        allow_images: bool = False,
        auth_mode: str | None = None,
    ) -> None:
        self.base_url = (base_url or "").rstrip("/")
        self.api_base = f"{self.base_url}{_API_PATH}"
        self.site_ids = [s.strip() for s in (site_ids or []) if s and s.strip()]
        self.root_node_ids = [
            n.strip() for n in (root_node_ids or []) if n and n.strip()
        ]
        self.batch_size = batch_size
        self.include_version_history = include_version_history
        self.allow_images = allow_images
        # Explicitly selected credential mode: "basic" or "oauth2".
        # Empty falls back to precedence in load_credentials.
        self.auth_mode = (auth_mode or "").strip().lower()
        self._session: requests.Session | None = None

    # ------------------------------------------------------------------
    # Auth
    # ------------------------------------------------------------------

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        if not self.base_url:
            raise ConnectorMissingCredentialError(
                "Alfresco: base_url is required"
            )

        username = credentials.get("username")
        password = credentials.get("password")
        access_token = credentials.get("access_token")

        # Honor the explicitly selected auth mode. The UI hides the
        # inactive mode's fields but does not clear them, so selecting by
        # field precedence could authenticate with stale values. Fall back
        # to precedence only when no auth_mode was supplied.
        mode = self.auth_mode
        if not mode:
            if access_token:
                mode = "oauth2"
            elif username and password:
                mode = "basic"

        session = requests.Session()
        if mode == "oauth2":
            if not access_token:
                raise ConnectorMissingCredentialError(
                    "Alfresco: access_token is required for the oauth2 auth mode"
                )
            session.headers["Authorization"] = f"Bearer {access_token}"
        elif mode == "basic":
            if not (username and password):
                raise ConnectorMissingCredentialError(
                    "Alfresco: username and password are required for the basic auth mode"
                )
            session.auth = (username, password)
        else:
            raise ConnectorMissingCredentialError(
                "Alfresco credentials are incomplete. Provide one of: "
                "(a) username + password, or (b) access_token."
            )

        session.headers["Accept"] = "application/json"
        self._session = session
        return None

    # ------------------------------------------------------------------
    # Validation
    # ------------------------------------------------------------------

    def validate_connector_settings(self) -> None:
        if self._session is None:
            raise ConnectorMissingCredentialError("Alfresco")

        # One cheap call proving base URL + credentials are valid: fetch
        # the well-known repository root node.
        try:
            resp = self._session.get(
                f"{self.api_base}/nodes/-root-", timeout=_HTTP_TIMEOUT
            )
        except requests.RequestException as exc:
            raise UnexpectedValidationError(
                f"Alfresco: cannot reach {self.base_url}: {exc}"
            ) from exc

        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"Alfresco credential rejected or insufficient permissions "
                f"(HTTP {resp.status_code}): {resp.text[:300]}"
            )
        if resp.status_code == 404:
            raise ConnectorValidationError(
                f"Alfresco: REST API not found at {self.api_base} — check the base URL "
                f"(HTTP 404): {resp.text[:300]}"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"Alfresco validation failed (HTTP {resp.status_code}): {resp.text[:300]}"
            )

        # Resolve configured sites up-front so a typo'd site surfaces as a
        # clear validation error rather than a silently empty crawl.
        for site_id in self.site_ids:
            self._resolve_site_library(site_id, validating=True)

        # Validate explicit root node IDs the same way. Without this a
        # typo'd node ID would 404 during listing (treated as an empty
        # folder) and silently ingest nothing — which looks like a
        # successful sync and can also cause prune to delete documents
        # ingested by earlier runs. Well-known aliases always resolve.
        for node_id in self.root_node_ids:
            if node_id in _NODE_ALIASES:
                continue
            self._validate_node_exists(node_id)

    # ------------------------------------------------------------------
    # Checkpoint helpers
    # ------------------------------------------------------------------

    def build_dummy_checkpoint(self) -> AlfrescoCheckpoint:
        return AlfrescoCheckpoint(has_more=True)

    def validate_checkpoint_json(self, checkpoint_json: str) -> AlfrescoCheckpoint:
        try:
            return AlfrescoCheckpoint.model_validate_json(checkpoint_json)
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
        if not isinstance(checkpoint, AlfrescoCheckpoint):
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
            raise ConnectorMissingCredentialError("Alfresco")

        batch: list[SlimDocument] = []
        for node in self._walk_files():
            name = node.get("name", "")
            if not _has_supported_extension(name, self.allow_images):
                continue
            node_id = node["id"]
            if callback:
                callback(node_id, name)
            batch.append(SlimDocument(id=node_id))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    # ------------------------------------------------------------------
    # Internal node traversal
    # ------------------------------------------------------------------

    def _iter_documents(
        self,
        checkpoint: AlfrescoCheckpoint | None = None,
        since_epoch: float | None = None,
        until_epoch: float | None = None,
    ):
        from common.data_source.models import Document

        if self._session is None:
            raise ConnectorMissingCredentialError("Alfresco")

        batch: list[Document] = []

        for node in self._walk_files():
            name = node.get("name", "")
            if not _has_supported_extension(name, self.allow_images):
                continue

            node_id = node["id"]

            # Time-window filter: strict lower bound, inclusive upper bound
            # (since_epoch < modifiedAt <= until_epoch). Excluding
            # modifiedAt == since_epoch (the prior run's watermark, which
            # that run already yielded) avoids stable duplicate re-fetches
            # on the boundary. Enforcing the upper bound keeps nodes
            # modified mid-run from leaking into this window; they're
            # picked up by the next run (whose lower bound is this run's
            # upper bound), so an update can never fall into a gap.
            modified_at = _parse_alfresco_time(node.get("modifiedAt"))
            if modified_at:
                ts = modified_at.timestamp()
                if since_epoch and ts <= since_epoch:
                    continue
                if until_epoch and ts > until_epoch:
                    continue

            # Download node content. A node deleted between listing and
            # fetch is genuinely gone — skip it. Any other failure
            # (throttling, transient 5xx, network) must abort the run: the
            # sync framework advances its watermark from successfully
            # yielded docs, so silently skipping a transiently-failed node
            # while newer nodes succeed would drop it permanently.
            try:
                data = self._download_content(node_id)
            except _NodeGone:
                logger.warning(
                    "Alfresco: node %s (%s) vanished between listing and fetch; skipping",
                    node_id,
                    name,
                )
                continue

            doc_updated_at = modified_at or datetime.now(timezone.utc)
            version_label = (node.get("properties") or {}).get("cm:versionLabel")
            fingerprint = version_label or (
                modified_at.isoformat() if modified_at else None
            )

            metadata = {
                "node_id": node_id,
                "path": ((node.get("path") or {}).get("name") or ""),
            }
            if version_label:
                metadata["version"] = version_label

            # When version history is requested, retrieve the node's full
            # version list and surface it as metadata so the toggle has an
            # observable effect.
            if self.include_version_history:
                history = self._get_version_history(node_id)
                if history:
                    metadata["version_history"] = ", ".join(history)
                    metadata["version_count"] = str(len(history))

            doc = Document(
                id=node_id,
                source="alfresco",
                semantic_identifier=name,
                extension=_extension(name),
                blob=data,
                doc_updated_at=doc_updated_at,
                size_bytes=len(data),
                fingerprint=fingerprint,
                metadata=metadata,
            )
            batch.append(doc)

            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

        if checkpoint is not None:
            checkpoint.has_more = False

    def _walk_files(self) -> Generator[dict[str, Any], None, None]:
        """Yield file node entries reachable from the configured roots.

        Iterative breadth-first traversal of the ``children`` endpoint.
        Folders are descended into; files are yielded. A node-count guard
        protects against pathologically large or cyclic structures.
        """
        seen: set[str] = set()
        queue: deque[str] = deque(self._start_node_ids())
        scanned = 0

        while queue:
            parent_id = queue.popleft()
            if parent_id in seen:
                continue
            seen.add(parent_id)

            for entry in self._list_children(parent_id):
                scanned += 1
                if scanned > _MAX_NODES_SCANNED:
                    raise UnexpectedValidationError(
                        "Alfresco: node scan limit exceeded; aborting to avoid a runaway crawl"
                    )
                if entry.get("isFolder"):
                    child_id = entry.get("id")
                    if child_id and child_id not in seen:
                        queue.append(child_id)
                elif entry.get("isFile"):
                    yield entry

    def _start_node_ids(self) -> list[str]:
        """Resolve the configured scope to a list of starting node IDs."""
        roots: list[str] = []
        for site_id in self.site_ids:
            container = self._resolve_site_library(site_id)
            if container:
                roots.append(container)
        roots.extend(self.root_node_ids)
        if not roots:
            roots = ["-root-"]
        return roots

    def _resolve_site_library(
        self, site_id: str, validating: bool = False
    ) -> str | None:
        """Resolve a site's ``documentLibrary`` container to a node ID."""
        assert self._session is not None
        url = f"{self.api_base}/sites/{site_id}/containers/documentLibrary"
        try:
            resp = self._session.get(url, timeout=_HTTP_TIMEOUT)
        except requests.RequestException as exc:
            raise UnexpectedValidationError(
                f"Alfresco: failed to resolve site '{site_id}': {exc}"
            ) from exc

        if resp.status_code == 404:
            msg = (
                f"Alfresco: site '{site_id}' or its document library was not found"
            )
            if validating:
                raise ConnectorValidationError(msg)
            logger.warning("%s; skipping", msg)
            return None
        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"Alfresco: insufficient permissions for site '{site_id}' "
                f"(HTTP {resp.status_code})"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"Alfresco: failed to resolve site '{site_id}' "
                f"(HTTP {resp.status_code}): {resp.text[:300]}"
            )

        try:
            return resp.json()["entry"]["id"]
        except (ValueError, KeyError) as exc:
            raise UnexpectedValidationError(
                f"Alfresco: unexpected site-container response for '{site_id}': {exc}"
            ) from exc

    def _validate_node_exists(self, node_id: str) -> None:
        """Confirm a configured root node ID exists.

        Raises a clear ``ConnectorValidationError`` on 404 so a typo'd node
        ID is reported at validation time instead of producing a silently
        empty crawl.
        """
        assert self._session is not None
        url = f"{self.api_base}/nodes/{node_id}"
        try:
            resp = self._session.get(url, timeout=_HTTP_TIMEOUT)
        except requests.RequestException as exc:
            raise UnexpectedValidationError(
                f"Alfresco: failed to resolve node '{node_id}': {exc}"
            ) from exc
        if resp.status_code == 404:
            raise ConnectorValidationError(
                f"Alfresco: root node '{node_id}' was not found."
            )
        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"Alfresco: insufficient permissions for node '{node_id}' "
                f"(HTTP {resp.status_code})"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"Alfresco: failed to resolve node '{node_id}' "
                f"(HTTP {resp.status_code}): {resp.text[:300]}"
            )

    def _get_version_history(self, node_id: str) -> list[str]:
        """Return a node's version labels (newest first), empty when it is
        not versioned.

        Used only when ``include_version_history`` is enabled. A 404 means
        the node has no version history (versioning off) and is not an
        error; transient/other failures propagate to abort the run.
        """
        assert self._session is not None
        url = f"{self.api_base}/nodes/{node_id}/versions"
        try:
            resp = self._session.get(
                url, params={"maxItems": _VERSION_HISTORY_LIMIT}, timeout=_HTTP_TIMEOUT
            )
        except requests.RequestException as exc:
            raise UnexpectedValidationError(
                f"Alfresco: failed to fetch versions for {node_id}: {exc}"
            ) from exc
        if resp.status_code == 404:
            return []
        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"Alfresco: insufficient permissions reading versions of {node_id} "
                f"(HTTP {resp.status_code})"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"Alfresco: failed to fetch versions for {node_id} "
                f"(HTTP {resp.status_code}): {resp.text[:300]}"
            )
        try:
            entries = resp.json()["list"]["entries"]
        except (ValueError, KeyError, TypeError):
            return []
        labels: list[str] = []
        for item in entries:
            entry = item.get("entry") if isinstance(item, dict) else None
            if not isinstance(entry, dict):
                continue
            label = entry.get("id") or entry.get("versionLabel")
            if label:
                labels.append(str(label))
        return labels

    def _list_children(self, parent_id: str) -> Generator[dict[str, Any], None, None]:
        """Yield child node entries of ``parent_id``, following pagination."""
        assert self._session is not None
        skip = 0
        while True:
            params = {
                "skipCount": skip,
                "maxItems": _PAGE_SIZE,
                "include": _LISTING_INCLUDE,
            }
            url = f"{self.api_base}/nodes/{parent_id}/children"
            try:
                resp = self._session.get(url, params=params, timeout=_HTTP_TIMEOUT)
            except requests.RequestException as exc:
                raise UnexpectedValidationError(
                    f"Alfresco: listing children of {parent_id} failed: {exc}"
                ) from exc

            if resp.status_code in (401, 403):
                raise InsufficientPermissionsError(
                    f"Alfresco: insufficient permissions listing {parent_id} "
                    f"(HTTP {resp.status_code})"
                )
            if resp.status_code == 404:
                # Folder removed mid-crawl: nothing more to list here.
                return
            if resp.status_code >= 400:
                raise UnexpectedValidationError(
                    f"Alfresco: listing children of {parent_id} failed "
                    f"(HTTP {resp.status_code}): {resp.text[:300]}"
                )

            try:
                payload = resp.json()["list"]
            except (ValueError, KeyError) as exc:
                raise UnexpectedValidationError(
                    f"Alfresco: unexpected children response for {parent_id}: {exc}"
                ) from exc

            for item in payload.get("entries", []):
                entry = item.get("entry")
                if entry:
                    yield entry

            pagination = payload.get("pagination") or {}
            if not pagination.get("hasMoreItems"):
                return
            skip += _PAGE_SIZE

    def _download_content(self, node_id: str) -> bytes:
        """Fetch a node's binary content, raising ``_NodeGone`` on 404."""
        assert self._session is not None
        url = f"{self.api_base}/nodes/{node_id}/content"
        try:
            resp = self._session.get(
                url, params={"attachment": "true"}, timeout=_HTTP_TIMEOUT
            )
        except requests.RequestException as exc:
            raise UnexpectedValidationError(
                f"Alfresco: failed to download node {node_id}: {exc}"
            ) from exc

        if resp.status_code == 404:
            raise _NodeGone(node_id)
        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"Alfresco: insufficient permissions to read node {node_id} "
                f"(HTTP {resp.status_code})"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"Alfresco: failed to download node {node_id} "
                f"(HTTP {resp.status_code}): {resp.text[:300]}"
            )
        return resp.content


# ----------------------------------------------------------------------
# Module-level helpers
# ----------------------------------------------------------------------

class _NodeGone(Exception):
    """A node listed moments earlier no longer exists (HTTP 404 on fetch)."""


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


def _parse_alfresco_time(value: Any) -> datetime | None:
    """Parse an Alfresco ISO-8601 timestamp into an aware UTC datetime.

    Alfresco emits e.g. ``2024-05-01T10:20:30.123+0000``. We normalise the
    trailing ``+0000`` (no colon) and a trailing ``Z`` so the stdlib parser
    accepts it across Python versions; any unparseable value yields ``None``
    so a single odd timestamp never aborts the crawl.
    """
    if not value or not isinstance(value, str):
        return None
    text = value.strip()
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    elif len(text) >= 5 and text[-5] in "+-" and text[-3] != ":":
        # "+0000" -> "+00:00"
        text = text[:-2] + ":" + text[-2:]
    try:
        parsed = datetime.fromisoformat(text)
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)

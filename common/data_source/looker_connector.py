"""Looker BI data-source connector.

Syncs Looker dashboards and Looks (with their folder context) into a
RAGFlow knowledge base so analytics teams can RAG over dashboard
documentation and metric definitions.

For each dashboard it ingests a text document built from the title,
description and tile metadata (tile titles, subtitles and note text). For
each Look it ingests title + description + query info, and — when
``include_exports`` is enabled — the rendered CSV result of the Look.

Auth uses Looker API3 credentials (``client_id`` / ``client_secret``)
against a configurable base URL: the connector logs in once to obtain a
short-lived access token and sends it as ``Authorization: token <token>``
(Looker's scheme). This talks to the Looker REST API 4.0 directly with
``requests`` — the same operations the official ``looker_sdk`` wraps
(``all_dashboards``/``dashboard``/``all_looks``/``look``/``run_look``) —
so no extra third-party dependency is required.

Incremental runs are scoped by the poll time window
(``since_epoch`` < ``updated_at`` <= ``until_epoch``). Each item's
``updated_at`` is emitted as the document fingerprint, which the indexing
pipeline persists as ``content_hash`` so unchanged items are not
re-embedded. The connector keeps no cross-run state; incrementality is
owned by the global ``poll_range_start`` watermark the sync framework
persists, and the connector **fails closed** (any error aborts the run)
so a partial failure can never advance that watermark past content it
never ingested.
"""

from __future__ import annotations

import logging
from datetime import datetime, timezone
from typing import Any, Generator

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

_API_VERSION = "4.0"

# Looker list endpoints page with limit/offset.
_PAGE_SIZE = 100

# Guard against a pathological / mis-paginating API never terminating.
_MAX_PAGES_PER_QUERY = 100_000

# Row cap for optional rendered Look CSV exports, to bound payload size.
_CSV_ROW_LIMIT = 5000

# HTTP timeout (connect, read) seconds.
_HTTP_TIMEOUT = (10, 120)

# Lean field set for list calls so we get updated_at cheaply without
# pulling every nested structure; the full object is fetched per in-window
# item to render tiles / query info.
_LIST_FIELDS = "id,title,description,updated_at,folder"


class LookerCheckpoint(ConnectorCheckpoint):
    """Checkpoint marker for the Looker connector.

    The connector keeps no cross-run state of its own: a single pass walks
    dashboards and Looks once and sets ``has_more=False``. Incremental
    scoping comes from the poll time window.
    """


class LookerConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """Looker BI connector (dashboards + Looks)."""

    def __init__(
        self,
        base_url: str,
        client_id: str | None = None,
        client_secret: str | None = None,
        include_dashboards: bool = True,
        include_looks: bool = True,
        include_exports: bool = False,
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        self.base_url = _normalize_base(base_url)
        self.api_base = f"{self.base_url}/api/{_API_VERSION}"
        # Credentials are stored in load_credentials, not here.
        self._client_id = client_id
        self._client_secret = client_secret
        self.include_dashboards = include_dashboards
        self.include_looks = include_looks
        self.include_exports = include_exports
        self.batch_size = batch_size
        self._session: requests.Session | None = None

    # ------------------------------------------------------------------
    # Auth
    # ------------------------------------------------------------------

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        if not self.base_url:
            raise ConnectorMissingCredentialError("Looker: base_url is required")

        client_id = credentials.get("client_id") or self._client_id
        client_secret = credentials.get("client_secret") or self._client_secret
        if not (client_id and client_secret):
            raise ConnectorMissingCredentialError(
                "Looker: client_id and client_secret (API3 credentials) are required"
            )
        self._client_id = client_id
        self._client_secret = client_secret

        session = requests.Session()
        session.headers["Accept"] = "application/json"
        try:
            resp = session.post(
                f"{self.api_base}/login",
                data={"client_id": client_id, "client_secret": client_secret},
                timeout=_HTTP_TIMEOUT,
            )
        except requests.RequestException as exc:
            raise ConnectorMissingCredentialError(
                f"Looker: cannot reach {self.base_url}: {exc}"
            )
        if resp.status_code in (401, 403, 404):
            raise ConnectorMissingCredentialError(
                f"Looker login failed (HTTP {resp.status_code}); check base URL and API3 credentials."
            )
        if not resp.ok:
            raise ConnectorMissingCredentialError(
                f"Looker login failed (HTTP {resp.status_code}): {resp.text[:200]}"
            )
        try:
            access_token = resp.json().get("access_token")
        except ValueError:
            access_token = None
        if not access_token:
            raise ConnectorMissingCredentialError("Looker login returned no access_token.")

        # Looker authenticates subsequent calls with the "token" scheme.
        session.headers["Authorization"] = f"token {access_token}"
        self._session = session
        return None

    # ------------------------------------------------------------------
    # Validation
    # ------------------------------------------------------------------

    def validate_connector_settings(self) -> None:
        if self._session is None:
            raise ConnectorMissingCredentialError("Looker")
        if not (self.include_dashboards or self.include_looks):
            raise ConnectorValidationError(
                "Looker: enable at least one of dashboards or Looks."
            )

        # Cheap authenticated probe.
        resp = self._get(f"{self.api_base}/user")
        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"Looker token rejected or insufficient permissions (HTTP {resp.status_code})."
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"Looker validation failed (HTTP {resp.status_code}): {resp.text[:200]}"
            )

    # ------------------------------------------------------------------
    # Checkpoint helpers
    # ------------------------------------------------------------------

    def build_dummy_checkpoint(self) -> LookerCheckpoint:
        return LookerCheckpoint(has_more=True)

    def validate_checkpoint_json(self, checkpoint_json: str) -> LookerCheckpoint:
        try:
            return LookerCheckpoint.model_validate_json(checkpoint_json)
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
        if not isinstance(checkpoint, LookerCheckpoint):
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

        Fails closed: any enumeration error propagates so the prune
        collector aborts and skips deletion rather than treating a partial
        snapshot as authoritative and wrongly deleting valid documents.
        """
        if self._session is None:
            raise ConnectorMissingCredentialError("Looker")

        batch: list[SlimDocument] = []
        if self.include_dashboards:
            for summary in self._paginate("dashboards", "dashboards"):
                did = summary.get("id")
                if did is None:
                    continue
                doc_id = f"dashboard:{did}"
                if callback:
                    callback(doc_id, "dashboards")
                batch.append(SlimDocument(id=doc_id))
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

        if self.include_looks:
            for summary in self._paginate("looks", "looks"):
                lid = summary.get("id")
                if lid is None:
                    continue
                doc_id = f"look:{lid}"
                if callback:
                    callback(doc_id, "looks")
                batch.append(SlimDocument(id=doc_id))
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []
                if self.include_exports:
                    csv_id = f"look:{lid}:csv"
                    if callback:
                        callback(csv_id, "looks")
                    batch.append(SlimDocument(id=csv_id))
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
        checkpoint: LookerCheckpoint | None = None,
        since_epoch: float | None = None,
        until_epoch: float | None = None,
    ):
        if self._session is None:
            raise ConnectorMissingCredentialError("Looker")

        batch: list[Any] = []

        if self.include_dashboards:
            for summary in self._iter_in_window("dashboards", since_epoch, until_epoch):
                doc = self._dashboard_document(summary)
                if doc is not None:
                    batch.append(doc)
                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []

        if self.include_looks:
            for summary in self._iter_in_window("looks", since_epoch, until_epoch):
                for doc in self._look_documents(summary):
                    batch.append(doc)
                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []

        if batch:
            yield batch

        if checkpoint is not None:
            checkpoint.has_more = False

    def _iter_in_window(
        self, kind: str, since_epoch: float | None, until_epoch: float | None
    ) -> Generator[dict[str, Any], None, None]:
        """Yield list summaries within the strict-lower / inclusive-upper
        ``updated_at`` window. Items with no parseable ``updated_at`` are
        always included (re-index rather than drop; dedup makes it cheap)."""
        for summary in self._paginate(kind, kind):
            ts = _epoch_of(summary.get("updated_at"))
            if ts is not None:
                if since_epoch and ts <= since_epoch:
                    continue
                if until_epoch and ts > until_epoch:
                    continue
            yield summary

    def _paginate(self, path: str, context: str) -> Generator[dict[str, Any], None, None]:
        """Yield entries from a Looker list endpoint with limit/offset."""
        offset = 0
        page = 0
        while True:
            if page > _MAX_PAGES_PER_QUERY:
                raise UnexpectedValidationError(
                    f"Looker: page limit exceeded for {context}; aborting runaway crawl"
                )
            resp = self._get(
                f"{self.api_base}/{path}",
                params={"fields": _LIST_FIELDS, "limit": _PAGE_SIZE, "offset": offset},
            )
            if resp.status_code in (401, 403):
                raise InsufficientPermissionsError(
                    f"Looker: insufficient permissions ({context}, HTTP {resp.status_code})"
                )
            if resp.status_code >= 400:
                raise UnexpectedValidationError(
                    f"Looker: {context} failed (HTTP {resp.status_code}): {resp.text[:300]}"
                )
            try:
                results = resp.json()
            except ValueError as exc:
                raise UnexpectedValidationError(
                    f"Looker: non-JSON response ({context}): {exc}"
                ) from exc
            if not isinstance(results, list):
                results = []

            for entry in results:
                if isinstance(entry, dict):
                    yield entry

            if len(results) < _PAGE_SIZE:
                return
            offset += _PAGE_SIZE
            page += 1

    def _dashboard_document(self, summary: dict[str, Any]):
        from common.data_source.models import Document

        dash_id = summary.get("id")
        if dash_id is None:
            return None
        full = self._get_json(f"{self.api_base}/dashboards/{dash_id}", f"dashboard {dash_id}")
        if full is None:
            return None

        title = _str(full.get("title")) or f"dashboard {dash_id}"
        description = _str(full.get("description"))
        folder = _folder_name(full.get("folder"))

        lines = [f"Looker dashboard: {title}"]
        if description:
            lines.append(f"Description: {description}")
        if folder:
            lines.append(f"Folder: {folder}")

        for element in full.get("dashboard_elements", []) or []:
            if not isinstance(element, dict):
                continue
            tile_bits = [
                _str(element.get("title")),
                _str(element.get("subtitle_text")),
                _str(element.get("note_text")),
                _str(element.get("body_text")),
            ]
            tile = " — ".join(b for b in tile_bits if b)
            query = element.get("query") or {}
            if isinstance(query, dict):
                model = _str(query.get("model"))
                view = _str(query.get("view"))
                if model or view:
                    tile = f"{tile} [{model}/{view}]" if tile else f"[{model}/{view}]"
            if tile:
                lines.append(f"Tile: {tile}")

        body = "\n".join(lines)
        blob = body.encode("utf-8")
        updated = _parse_time(full.get("updated_at") or summary.get("updated_at"))
        return Document(
            id=f"dashboard:{dash_id}",
            source="looker",
            semantic_identifier=title,
            extension=".txt",
            blob=blob,
            doc_updated_at=updated or datetime.now(timezone.utc),
            size_bytes=len(blob),
            fingerprint=updated.isoformat() if updated else None,
            metadata={"type": "dashboard", "dashboard_id": str(dash_id), "folder": folder},
        )

    def _look_documents(self, summary: dict[str, Any]):
        from common.data_source.models import Document

        look_id = summary.get("id")
        if look_id is None:
            return
        full = self._get_json(f"{self.api_base}/looks/{look_id}", f"look {look_id}")
        if full is None:
            return

        title = _str(full.get("title")) or f"look {look_id}"
        description = _str(full.get("description"))
        folder = _folder_name(full.get("folder"))
        updated = _parse_time(full.get("updated_at") or summary.get("updated_at"))
        fingerprint = updated.isoformat() if updated else None

        lines = [f"Looker Look: {title}"]
        if description:
            lines.append(f"Description: {description}")
        if folder:
            lines.append(f"Folder: {folder}")
        query = full.get("query") or {}
        if isinstance(query, dict):
            model = _str(query.get("model"))
            view = _str(query.get("view"))
            fields = query.get("fields")
            if model or view:
                lines.append(f"Explore: {model}/{view}")
            if isinstance(fields, list) and fields:
                lines.append("Fields: " + ", ".join(str(f) for f in fields))

        body = "\n".join(lines)
        blob = body.encode("utf-8")
        yield Document(
            id=f"look:{look_id}",
            source="looker",
            semantic_identifier=title,
            extension=".txt",
            blob=blob,
            doc_updated_at=updated or datetime.now(timezone.utc),
            size_bytes=len(blob),
            fingerprint=fingerprint,
            metadata={"type": "look", "look_id": str(look_id), "folder": folder},
        )

        if self.include_exports:
            csv_bytes = self._run_look_csv(look_id)
            if csv_bytes:
                yield Document(
                    id=f"look:{look_id}:csv",
                    source="looker",
                    semantic_identifier=f"{title}.csv",
                    extension=".csv",
                    blob=csv_bytes,
                    doc_updated_at=updated or datetime.now(timezone.utc),
                    size_bytes=len(csv_bytes),
                    fingerprint=fingerprint,
                    metadata={"type": "look_csv", "look_id": str(look_id)},
                )

    def _run_look_csv(self, look_id: Any) -> bytes | None:
        """Render a Look to CSV (run_look). Returns None when the Look has no
        runnable result; transient errors propagate (fail closed)."""
        assert self._session is not None
        url = f"{self.api_base}/looks/{look_id}/run/csv"
        try:
            resp = self._session.get(
                url, params={"limit": _CSV_ROW_LIMIT}, timeout=_HTTP_TIMEOUT
            )
        except requests.RequestException as exc:
            raise UnexpectedValidationError(
                f"Looker: run_look CSV for {look_id} failed: {exc}"
            ) from exc
        if resp.status_code in (404, 422):
            return None
        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"Looker: insufficient permissions to run Look {look_id} (HTTP {resp.status_code})"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"Looker: run_look CSV for {look_id} failed (HTTP {resp.status_code})"
            )
        return resp.content or None

    # ------------------------------------------------------------------
    # HTTP helpers
    # ------------------------------------------------------------------

    def _get(self, url: str, params: dict[str, Any] | None = None) -> requests.Response:
        assert self._session is not None
        try:
            return self._session.get(url, params=params, timeout=_HTTP_TIMEOUT)
        except requests.RequestException as exc:
            raise UnexpectedValidationError(f"Looker: request to {url} failed: {exc}") from exc

    def _get_json(self, url: str, context: str) -> dict[str, Any] | None:
        resp = self._get(url)
        if resp.status_code == 404:
            # Item removed between listing and fetch — genuinely gone.
            logger.warning("Looker: %s not found (404); skipping", context)
            return None
        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"Looker: insufficient permissions ({context}, HTTP {resp.status_code})"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"Looker: {context} failed (HTTP {resp.status_code}): {resp.text[:300]}"
            )
        try:
            payload = resp.json()
        except ValueError as exc:
            raise UnexpectedValidationError(
                f"Looker: non-JSON response ({context}): {exc}"
            ) from exc
        return payload if isinstance(payload, dict) else None


# ----------------------------------------------------------------------
# Module-level helpers
# ----------------------------------------------------------------------

def _normalize_base(base_url: str) -> str:
    h = (base_url or "").strip().rstrip("/")
    if h and not h.startswith(("http://", "https://")):
        h = "https://" + h
    return h


def _str(value: Any) -> str:
    return value if isinstance(value, str) else ("" if value is None else str(value))


def _folder_name(folder: Any) -> str:
    if isinstance(folder, dict):
        return _str(folder.get("name"))
    return ""


def _epoch_of(value: Any) -> float | None:
    parsed = _parse_time(value)
    return parsed.timestamp() if parsed else None


def _parse_time(value: Any) -> datetime | None:
    """Parse a Looker ``updated_at`` into an aware UTC datetime.

    Looker emits ISO-8601 (``2024-05-01T10:20:30.000+00:00`` / ``...Z``).
    Anything unparseable yields ``None`` so a single odd timestamp never
    aborts the crawl.
    """
    if value is None:
        return None
    if isinstance(value, (int, float)):
        try:
            return datetime.fromtimestamp(float(value), tz=timezone.utc)
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

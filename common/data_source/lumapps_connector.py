"""LumApps intranet data-source connector.

Syncs LumApps digital-workplace content (articles, posts, pages) into a
RAGFlow knowledge base via the LumApps API (``/v1/content/list`` +
``/v1/content/get``). For each content item it ingests a text document
built from the title, excerpt and body text, and — when
``include_attachments`` is enabled — downloads attached files.

Auth uses an OAuth2 bearer token against a configurable organization /
base URL. The token can be supplied directly (``access_token``, obtained
via the caller's OAuth / customer-API flow) or, when ``client_id`` +
``client_secret`` + ``token_url`` are all provided, fetched with an
OAuth2 client-credentials grant.

Incremental runs are scoped by the poll time window
(``since_epoch`` < content ``updatedAt`` <= ``until_epoch``). Each item's
``updatedAt`` is emitted as the document fingerprint, which the indexing
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

_API_PREFIX = "/v1"

# content/list paginates with an opaque cursor; this is the per-page size.
_PAGE_SIZE = 50

# Guard against a pathological / mis-paginating API never terminating.
_MAX_PAGES = 100_000

# Preferred language when a field is a localized {lang: text} map.
_DEFAULT_LANG = "en"

# Recursion bounds for best-effort body-text extraction from the content tree.
_MAX_BODY_DEPTH = 8
_MAX_BODY_CHARS = 500_000

# Keys in the content tree whose string values are real human text.
_TEXT_KEYS = {"htmlContent", "html", "content", "text", "value", "description", "excerpt"}

# HTTP timeout (connect, read) seconds.
_HTTP_TIMEOUT = (10, 60)

# Attachment file types we ingest.
_SUPPORTED_EXTENSIONS = {
    ".pdf", ".docx", ".doc", ".pptx", ".ppt",
    ".xlsx", ".xls", ".txt", ".md", ".csv",
    ".html", ".htm", ".json", ".xml",
}


class LumAppsCheckpoint(ConnectorCheckpoint):
    """Checkpoint marker for the LumApps connector.

    The connector keeps no cross-run state of its own: a single pass walks
    the configured content once and sets ``has_more=False``. Incremental
    scoping comes from the poll time window.
    """


class LumAppsConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """LumApps intranet connector."""

    def __init__(
        self,
        base_url: str,
        community_ids: Iterable[str] | None = None,
        content_types: Iterable[str] | None = None,
        lang: str | None = None,
        token_url: str | None = None,
        include_attachments: bool = False,
        batch_size: int = INDEX_BATCH_SIZE,
        allow_images: bool = False,
    ) -> None:
        self.base_url = _normalize_base(base_url)
        self.api_base = f"{self.base_url}{_API_PREFIX}"
        self.community_ids = [str(c).strip() for c in (community_ids or []) if str(c).strip()]
        self.content_types = [str(t).strip() for t in (content_types or []) if str(t).strip()]
        self.lang = (lang or _DEFAULT_LANG).strip() or _DEFAULT_LANG
        self.token_url = (token_url or "").strip()
        self.include_attachments = include_attachments
        self.batch_size = batch_size
        self.allow_images = allow_images
        self._session: requests.Session | None = None

    # ------------------------------------------------------------------
    # Auth
    # ------------------------------------------------------------------

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        if not self.base_url:
            raise ConnectorMissingCredentialError("LumApps: base_url is required")

        access_token = credentials.get("access_token") or credentials.get("token")
        client_id = credentials.get("client_id")
        client_secret = credentials.get("client_secret")

        if not access_token:
            if client_id and client_secret and self.token_url:
                access_token = self._fetch_token_client_credentials(
                    client_id, client_secret
                )
            else:
                raise ConnectorMissingCredentialError(
                    "LumApps: provide an access_token, or client_id + client_secret "
                    "together with token_url for an OAuth2 client-credentials grant."
                )

        session = requests.Session()
        session.headers["Authorization"] = f"Bearer {access_token}"
        session.headers["Accept"] = "application/json"
        self._session = session
        return None

    def _fetch_token_client_credentials(self, client_id: str, client_secret: str) -> str:
        try:
            resp = requests.post(
                self.token_url,
                data={
                    "grant_type": "client_credentials",
                    "client_id": client_id,
                    "client_secret": client_secret,
                },
                timeout=_HTTP_TIMEOUT,
            )
        except requests.RequestException as exc:
            raise ConnectorMissingCredentialError(
                f"LumApps: token request to {self.token_url} failed: {exc}"
            )
        if not resp.ok:
            raise ConnectorMissingCredentialError(
                f"LumApps: token request failed (HTTP {resp.status_code})."
            )
        try:
            token = resp.json().get("access_token")
        except ValueError:
            token = None
        if not token:
            raise ConnectorMissingCredentialError(
                "LumApps: token endpoint returned no access_token."
            )
        return token

    # ------------------------------------------------------------------
    # Validation
    # ------------------------------------------------------------------

    def validate_connector_settings(self) -> None:
        if self._session is None:
            raise ConnectorMissingCredentialError("LumApps")

        # One cheap call proving base URL + token are valid.
        resp = self._get(f"{self.api_base}/content/list", params={"maxResults": 1})
        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"LumApps token rejected or insufficient permissions "
                f"(HTTP {resp.status_code}): {resp.text[:300]}"
            )
        if resp.status_code == 404:
            raise ConnectorValidationError(
                f"LumApps: API not found at {self.api_base} — check the base URL "
                f"(HTTP 404): {resp.text[:300]}"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"LumApps validation failed (HTTP {resp.status_code}): {resp.text[:300]}"
            )

    # ------------------------------------------------------------------
    # Checkpoint helpers
    # ------------------------------------------------------------------

    def build_dummy_checkpoint(self) -> LumAppsCheckpoint:
        return LumAppsCheckpoint(has_more=True)

    def validate_checkpoint_json(self, checkpoint_json: str) -> LumAppsCheckpoint:
        try:
            return LumAppsCheckpoint.model_validate_json(checkpoint_json)
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
        if not isinstance(checkpoint, LumAppsCheckpoint):
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
            raise ConnectorMissingCredentialError("LumApps")

        batch: list[SlimDocument] = []
        for summary in self._list_content():
            content_id = _content_id(summary)
            if not content_id:
                continue
            doc_id = f"content:{content_id}"
            if callback:
                callback(doc_id, "lumapps")
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
        checkpoint: LumAppsCheckpoint | None = None,
        since_epoch: float | None = None,
        until_epoch: float | None = None,
    ):
        from common.data_source.models import Document

        if self._session is None:
            raise ConnectorMissingCredentialError("LumApps")

        batch: list[Document] = []

        for summary in self._list_content():
            content_id = _content_id(summary)
            if not content_id:
                continue

            updated = _parse_time(summary.get("updatedAt") or summary.get("updated_at"))
            if updated is not None:
                ts = updated.timestamp()
                if since_epoch and ts <= since_epoch:
                    continue
                if until_epoch and ts > until_epoch:
                    continue

            full = self._get_content(content_id)
            if full is None:
                continue

            updated = (
                _parse_time(full.get("updatedAt") or full.get("updated_at")) or updated
            )
            doc_updated_at = updated or datetime.now(timezone.utc)
            fingerprint = updated.isoformat() if updated else None

            title = _localized(full.get("title"), self.lang) or str(content_id)
            excerpt = _localized(full.get("excerpt"), self.lang)
            body_text = _harvest_text(full)

            lines = [f"Title: {title}"]
            if excerpt:
                lines.append(f"Excerpt: {excerpt}")
            if body_text:
                lines.append(body_text)
            body = "\n\n".join(lines)
            blob = body.encode("utf-8")

            metadata = {
                "content_id": str(content_id),
                "type": _str(full.get("type")),
                "slug": _localized(full.get("slug"), self.lang),
            }
            community = _community_of(full)
            if community:
                metadata["community"] = community

            batch.append(Document(
                id=f"content:{content_id}",
                source="lumapps",
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

            if self.include_attachments:
                for att in self._iter_attachments(content_id, full, doc_updated_at, fingerprint):
                    batch.append(att)
                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []

        if batch:
            yield batch

        if checkpoint is not None:
            checkpoint.has_more = False

    def _list_content(self) -> Generator[dict[str, Any], None, None]:
        """Yield content summaries across the configured communities (or all),
        following the API's cursor pagination.

        When ``community_ids`` is set, one paginated listing is issued per
        community; otherwise a single instance-wide listing is issued.
        """
        scopes: list[str | None] = list(self.community_ids) if self.community_ids else [None]
        for community in scopes:
            cursor: str | None = None
            page = 0
            while True:
                if page > _MAX_PAGES:
                    raise UnexpectedValidationError(
                        "LumApps: page limit exceeded; aborting runaway crawl"
                    )
                body: dict[str, Any] = {"maxResults": _PAGE_SIZE, "lang": self.lang}
                if community:
                    body["instanceId"] = community
                if self.content_types:
                    body["type"] = self.content_types
                if cursor:
                    body["cursor"] = cursor

                resp = self._post(f"{self.api_base}/content/list", json=body, context="content/list")
                payload = self._json(resp, "content/list")

                items = _items(payload)
                for item in items:
                    if isinstance(item, dict):
                        yield item

                cursor = payload.get("cursor") if isinstance(payload, dict) else None
                more = payload.get("more") if isinstance(payload, dict) else None
                # Stop when the API signals no more, gives no cursor, or a short page.
                if more is False or not cursor or len(items) < _PAGE_SIZE:
                    break
                page += 1

    def _get_content(self, content_id: str) -> dict[str, Any] | None:
        resp = self._get(f"{self.api_base}/content/get", params={"uid": content_id})
        if resp.status_code == 404:
            logger.warning("LumApps: content %s not found (404); skipping", content_id)
            return None
        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"LumApps: insufficient permissions reading content {content_id} "
                f"(HTTP {resp.status_code})"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"LumApps: content/get {content_id} failed (HTTP {resp.status_code}): "
                f"{resp.text[:300]}"
            )
        payload = self._json(resp, f"content/get {content_id}")
        return payload if isinstance(payload, dict) else None

    def _iter_attachments(
        self,
        content_id: str,
        full: dict[str, Any],
        doc_updated_at: datetime,
        fingerprint: str | None,
    ):
        from common.data_source.models import Document

        assert self._session is not None
        attachments = (
            full.get("files") or full.get("attachments") or full.get("media") or []
        )
        if not isinstance(attachments, list):
            return

        for idx, att in enumerate(attachments):
            if not isinstance(att, dict):
                continue
            name = _str(att.get("name") or att.get("fileName") or att.get("title"))
            link = _str(
                att.get("downloadUrl")
                or att.get("url")
                or att.get("contentUrl")
                or att.get("servingUrl")
            )
            if not link or not _has_supported_extension(name, self.allow_images):
                continue

            try:
                resp = self._session.get(link, timeout=_HTTP_TIMEOUT)
            except requests.RequestException as exc:
                raise UnexpectedValidationError(
                    f"LumApps: failed to download attachment {name}: {exc}"
                ) from exc
            if resp.status_code == 404:
                logger.warning("LumApps: attachment %s missing (404); skipping", name)
                continue
            if resp.status_code >= 400:
                raise UnexpectedValidationError(
                    f"LumApps: attachment {name} download failed (HTTP {resp.status_code})"
                )

            data = resp.content
            yield Document(
                id=f"content:{content_id}:file:{idx}",
                source="lumapps",
                semantic_identifier=name,
                extension=_extension(name),
                blob=data,
                doc_updated_at=doc_updated_at,
                size_bytes=len(data),
                fingerprint=fingerprint,
                metadata={"content_id": str(content_id), "filename": name},
            )

    # ------------------------------------------------------------------
    # HTTP helpers
    # ------------------------------------------------------------------

    def _get(self, url: str, params: dict[str, Any] | None = None) -> requests.Response:
        assert self._session is not None
        try:
            return self._session.get(url, params=params, timeout=_HTTP_TIMEOUT)
        except requests.RequestException as exc:
            raise UnexpectedValidationError(f"LumApps: request to {url} failed: {exc}") from exc

    def _post(self, url: str, json: dict[str, Any], context: str) -> requests.Response:
        assert self._session is not None
        try:
            return self._session.post(url, json=json, timeout=_HTTP_TIMEOUT)
        except requests.RequestException as exc:
            raise UnexpectedValidationError(f"LumApps: {context} request failed: {exc}") from exc

    def _json(self, resp: requests.Response, context: str) -> dict[str, Any]:
        if resp.status_code in (401, 403):
            raise InsufficientPermissionsError(
                f"LumApps: insufficient permissions ({context}, HTTP {resp.status_code})"
            )
        if resp.status_code >= 400:
            raise UnexpectedValidationError(
                f"LumApps: {context} failed (HTTP {resp.status_code}): {resp.text[:300]}"
            )
        try:
            payload = resp.json()
        except ValueError as exc:
            raise UnexpectedValidationError(
                f"LumApps: non-JSON response ({context}): {exc}"
            ) from exc
        return payload if isinstance(payload, dict) else {}


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


def _items(payload: Any) -> list[Any]:
    if isinstance(payload, dict):
        for key in ("items", "content", "results", "data"):
            value = payload.get(key)
            if isinstance(value, list):
                return value
    if isinstance(payload, list):
        return payload
    return []


def _content_id(item: dict[str, Any]) -> str:
    cid = item.get("uid") or item.get("id")
    return "" if cid is None else str(cid)


def _localized(value: Any, lang: str) -> str:
    """Resolve a possibly-localized field ({lang: text}) to a string."""
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    if isinstance(value, dict):
        if lang in value and isinstance(value[lang], str):
            return value[lang]
        if _DEFAULT_LANG in value and isinstance(value[_DEFAULT_LANG], str):
            return value[_DEFAULT_LANG]
        for v in value.values():
            if isinstance(v, str) and v:
                return v
    return ""


def _community_of(full: dict[str, Any]) -> str:
    for key in ("instanceId", "community", "containerId", "container"):
        val = full.get(key)
        if isinstance(val, str) and val:
            return val
        if isinstance(val, dict):
            cid = val.get("uid") or val.get("id")
            if cid:
                return str(cid)
    return ""


def _harvest_text(node: Any, depth: int = 0, acc: list[str] | None = None) -> str:
    """Best-effort recursive harvest of human-readable text from a content
    tree, pulling string values under known text keys. Bounded by depth and
    total size so a deep/large structure can't blow up the document."""
    if acc is None:
        acc = []
    total = sum(len(s) for s in acc)
    if depth > _MAX_BODY_DEPTH or total > _MAX_BODY_CHARS:
        return "\n".join(acc)
    if isinstance(node, dict):
        for key, value in node.items():
            if key in _TEXT_KEYS and isinstance(value, str) and value.strip():
                acc.append(value.strip())
            else:
                _harvest_text(value, depth + 1, acc)
    elif isinstance(node, list):
        for entry in node:
            _harvest_text(entry, depth + 1, acc)
    return "\n".join(acc)


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


def _parse_time(value: Any) -> datetime | None:
    """Parse a LumApps timestamp into an aware UTC datetime.

    LumApps emits ISO-8601 (``2024-05-01T10:20:30.000Z`` / ``...+0000``);
    some fields are epoch seconds/milliseconds. Anything unparseable yields
    ``None`` so a single odd timestamp never aborts the crawl.
    """
    if value is None:
        return None
    if isinstance(value, (int, float)):
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

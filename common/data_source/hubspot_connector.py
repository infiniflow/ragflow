"""HubSpot CRM data source connector.

Talks to the HubSpot CRM v3 REST API, turns each selected object's
records (contacts, companies, deals, tickets) and Knowledge Base
articles into a Document, and uses ``hs_lastmodifieddate`` (or
``updatedAt`` for KB articles) as the incremental cursor so re-syncs
only fetch what changed.

Auth supports either a HubSpot **private app access token** (simplest;
what the issue asks for first) or an **OAuth 2.0 access token**; both
are passed as the Authorization bearer the same way upstream, so the
connector treats them uniformly once acquired.
"""

from __future__ import annotations

import logging
import time
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

_API_BASE = "https://api.hubapi.com"

# CRM objects available via the v3 search endpoint. The connector ships
# with this allow-list so a stock install indexes what the issue
# explicitly names; operators can extend it via the ``objects`` config.
_DEFAULT_CRM_OBJECTS = ["contacts", "companies", "deals", "tickets"]

# Per-object display fields used to build the document body. Pulling
# only the fields we render keeps payloads small and avoids HubSpot's
# 1024-property hard cap on the search endpoint.
_DEFAULT_PROPERTIES: dict[str, list[str]] = {
    "contacts": ["firstname", "lastname", "email", "phone", "company", "jobtitle", "lifecyclestage", "notes_last_contacted"],
    "companies": ["name", "domain", "industry", "city", "country", "description", "phone", "website"],
    "deals": ["dealname", "dealstage", "pipeline", "amount", "closedate", "dealtype", "description"],
    "tickets": ["subject", "content", "hs_pipeline_stage", "hs_ticket_priority", "hs_ticket_category", "source_type"],
}

# Search results are capped at 10k records per query; HubSpot's docs
# direct callers to page through windows using ``after`` until the
# cursor exhausts, then advance the timestamp filter.
_SEARCH_PAGE_SIZE = 100

# Knowledge Base articles live on a separate CMS endpoint (v3 of the
# kb_articles API). Default to syncing only published articles.
_KB_PUBLISHED_STATE = "PUBLISHED"


class HubSpotCheckpoint(ConnectorCheckpoint):
    """Checkpoint marker for the HubSpot connector.

    The connector keeps no cross-run state of its own. Incrementality is
    owned entirely by the global ``poll_range_start`` watermark the sync
    framework persists: each run queries every object from that lower
    bound, and the connector **fails closed** (a partial object failure
    aborts the whole run) so the watermark only advances on a fully
    successful sync. A failed run leaves the watermark pinned and simply
    retries the same window next time — so a partial failure can never
    advance the watermark past records it never ingested.

    We deliberately do *not* persist a per-object cursor here: the sync
    framework rebuilds a fresh checkpoint each run, so a per-object cursor
    would not survive and relying on it would silently drop the gap between
    a failure point and the advanced global watermark.
    """


class HubSpotConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """HubSpot CRM + Knowledge Base connector.

    Requires either:
      - a HubSpot **private app** access token with at least the
        ``crm.objects.{contacts,companies,deals,tickets}.read`` scopes,
        plus ``content`` for the Knowledge Base, **or**
      - an OAuth 2.0 access token issued for the same scopes.

    Respects HubSpot's published rate limits (100 requests / 10 s for
    most CRM endpoints) by honoring ``429`` with the ``Retry-After``
    header rather than failing the sync.
    """

    def __init__(
        self,
        batch_size: int = INDEX_BATCH_SIZE,
        objects: list[str] | None = None,
        include_knowledge_base: bool = True,
        properties: dict[str, list[str]] | None = None,
        max_retries: int = 5,
    ) -> None:
        self.batch_size = batch_size
        self.objects = [obj.strip() for obj in (objects or _DEFAULT_CRM_OBJECTS) if obj and obj.strip()]
        self.include_knowledge_base = include_knowledge_base
        # Caller-supplied property overrides win; we never strip out
        # the defaults wholesale so partial overrides still work.
        merged = {k: list(v) for k, v in _DEFAULT_PROPERTIES.items()}
        for k, v in (properties or {}).items():
            merged[k] = list(v)
        self.properties = merged
        self.max_retries = max(1, max_retries)
        self._access_token: str | None = None

    # ------------------------------------------------------------------
    # Auth
    # ------------------------------------------------------------------

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        # Either credential form lands in the same place — private app
        # tokens and OAuth tokens are both passed as Bearer upstream.
        token = (
            credentials.get("access_token")
            or credentials.get("private_app_token")
            or credentials.get("hubspot_access_token")
        )
        if not token:
            raise ConnectorMissingCredentialError(
                "HubSpot credentials are incomplete (access_token or private_app_token required)"
            )
        self._access_token = token
        return None

    # ------------------------------------------------------------------
    # Validation
    # ------------------------------------------------------------------

    def validate_connector_settings(self) -> None:
        if not self._access_token:
            raise ConnectorMissingCredentialError("HubSpot")

        # /account-info/v3/details is the cheapest authenticated probe;
        # it returns the portal ID and never costs read quota on a
        # specific object, which is what we want for plain validation.
        resp = self._request("GET", f"{_API_BASE}/account-info/v3/details")
        if resp.status_code == 401:
            raise ConnectorMissingCredentialError(
                "HubSpot access token is invalid or expired."
            )
        if resp.status_code == 403:
            raise InsufficientPermissionsError(
                "HubSpot token lacks the required scopes (need crm.objects.*.read and content for KB)."
            )
        if not resp.ok:
            raise UnexpectedValidationError(
                f"HubSpot validation failed (HTTP {resp.status_code}): {resp.text[:200]}"
            )

        try:
            payload = resp.json()
        except ValueError as exc:
            raise ConnectorValidationError(
                f"HubSpot /account-info response is not JSON: {exc}"
            )
        if "portalId" not in payload:
            raise ConnectorValidationError(
                "Unexpected response format from HubSpot /account-info."
            )

    # ------------------------------------------------------------------
    # Checkpoint helpers
    # ------------------------------------------------------------------

    def build_dummy_checkpoint(self) -> HubSpotCheckpoint:
        return HubSpotCheckpoint(has_more=True)

    def validate_checkpoint_json(self, checkpoint_json: str) -> HubSpotCheckpoint:
        try:
            return HubSpotCheckpoint.model_validate_json(checkpoint_json)
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
        if not isinstance(checkpoint, HubSpotCheckpoint):
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

        Slim IDs are scoped as ``<object>/<id>`` so a contact and a deal
        sharing an autoincrement value cannot collide when the prune
        collector reconciles deletes.
        """
        if not self._access_token:
            raise ConnectorMissingCredentialError("HubSpot")

        # Fail closed: the prune flow treats the returned slim-doc snapshot
        # as authoritative and deletes anything missing from it. If a single
        # object/KB enumeration fails (429, 5xx, permission, transient API
        # error) we must NOT return a partial snapshot — doing so would make
        # the still-valid documents from the failed object look stale and get
        # wrongly deleted. Letting the error propagate makes the prune
        # collector abort and skip deletion for this run instead.
        batch: list[SlimDocument] = []
        for obj in self.objects:
            for record in self._search_records(obj, properties=["hs_object_id"], since_ms=None):
                rec_id = record.get("id")
                if not rec_id:
                    continue
                doc_id = f"{obj}/{rec_id}"
                if callback:
                    callback(doc_id, obj)
                batch.append(SlimDocument(id=doc_id))
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

        if self.include_knowledge_base:
            # allow_missing=False: an unavailable KB aborts prune rather than
            # producing a CRM-only snapshot that would wrongly delete KB docs.
            for article in self._iter_kb_articles(since_ms=None, allow_missing=False):
                article_id = article.get("id")
                if not article_id:
                    continue
                doc_id = f"kb_articles/{article_id}"
                if callback:
                    callback(doc_id, "kb_articles")
                batch.append(SlimDocument(id=doc_id))
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

        if batch:
            yield batch

    # ------------------------------------------------------------------
    # HTTP helpers
    # ------------------------------------------------------------------

    def _request(self, method: str, url: str, **kwargs) -> requests.Response:
        """Send an authenticated request, honoring HubSpot's documented
        rate-limit signal (``429`` + ``Retry-After``) with bounded
        retries. We do *not* retry generic 5xx here — those are
        propagated so ``_get_json`` can raise and the caller can keep
        the checkpoint pinned to the last successful record."""
        headers = kwargs.pop("headers", {}) or {}
        headers.setdefault("Authorization", f"Bearer {self._access_token}")
        headers.setdefault("Accept", "application/json")
        if method.upper() in {"POST", "PUT", "PATCH"}:
            headers.setdefault("Content-Type", "application/json")

        last_resp: requests.Response | None = None
        for attempt in range(self.max_retries):
            resp = requests.request(method, url, headers=headers, timeout=60, **kwargs)
            if resp.status_code != 429:
                return resp
            last_resp = resp
            retry_after = resp.headers.get("Retry-After")
            try:
                delay = float(retry_after) if retry_after else 2 ** attempt
            except ValueError:
                delay = 2 ** attempt
            delay = min(delay, 30)
            logger.warning(
                "HubSpot rate-limited (attempt %s/%s), sleeping %ss",
                attempt + 1,
                self.max_retries,
                delay,
            )
            time.sleep(delay)
        # Exhausted retries; return the last 429 response so the caller
        # surfaces a proper error rather than masquerading as a non-2xx.
        return last_resp  # type: ignore[return-value]

    def _json(self, resp: requests.Response, *, context: str) -> dict:
        if not resp.ok:
            # Never log or surface the response body: HubSpot CRM/KB
            # payloads can carry customer PII. The correlation id is the
            # safe handle to give HubSpot support for diagnosing a 4xx/5xx.
            correlation_id = resp.headers.get("x-hubspot-correlation-id", "")
            logger.error(
                "HubSpot request failed (%s): HTTP %s correlation_id=%s",
                context,
                resp.status_code,
                correlation_id,
            )
            # 401/403 are auth/scope problems — surface them as an actionable
            # InsufficientPermissionsError so a missing object/KB scope reads
            # as "fix your token" rather than an opaque unexpected failure.
            if resp.status_code in (401, 403):
                raise InsufficientPermissionsError(
                    f"HubSpot request lacks access ({context}): HTTP {resp.status_code} "
                    f"(correlation_id={correlation_id})"
                )
            raise UnexpectedValidationError(
                f"HubSpot request failed ({context}): HTTP {resp.status_code} "
                f"(correlation_id={correlation_id})"
            )
        try:
            return resp.json()
        except ValueError as exc:
            raise UnexpectedValidationError(
                f"HubSpot response is not JSON ({context}): {exc}"
            )

    # ------------------------------------------------------------------
    # Search / pagination
    # ------------------------------------------------------------------

    def _search_records(
        self,
        obj: str,
        properties: list[str],
        since_ms: int | None,
        until_ms: int | None = None,
    ) -> Generator[dict, None, None]:
        """Yield CRM records for *obj* via the v3 search endpoint.

        Sorts ascending by ``hs_lastmodifieddate`` so the per-object
        cursor advances monotonically across runs. The 10k results /
        query cap is handled by re-issuing the search with the latest
        seen timestamp as the new lower bound once a window exhausts.
        """
        url = f"{_API_BASE}/crm/v3/objects/{obj}/search"
        current_since = since_ms
        # IDs already emitted at exactly ``current_since``. Re-windowing
        # uses a ``GTE`` (not ``GT``) floor so records sharing the
        # boundary millisecond are never skipped; this set lets us drop
        # the ones we already yielded so the overlap doesn't double-emit.
        # A strict ``GT`` would silently lose every record tied on the
        # boundary timestamp when a window hits the 10k Search API cap.
        boundary_seen_ids: set[str] = set()
        # Outer loop advances the timestamp window when one query
        # exhausts its 10k-result allotment. The inner loop pages
        # through ``after`` cursors inside a single window.
        while True:
            after: str | None = None
            page_count = 0
            latest_in_window: int | None = None
            ids_at_latest: set[str] = set()
            new_yielded = 0

            while True:
                filters = []
                if current_since:
                    filters.append({
                        "propertyName": "hs_lastmodifieddate",
                        "operator": "GTE",
                        "value": str(current_since),
                    })

                body = {
                    "limit": _SEARCH_PAGE_SIZE,
                    "properties": properties,
                    "sorts": [{"propertyName": "hs_lastmodifieddate", "direction": "ASCENDING"}],
                    "filterGroups": [{"filters": filters}] if filters else [],
                }
                if after:
                    body["after"] = after

                resp = self._request("POST", url, json=body)
                page = self._json(resp, context=f"search {obj}")

                results = page.get("results", []) or []
                for record in results:
                    last_modified = _record_lastmodified_ms(record)
                    rec_id = str(record.get("id") or "")
                    # Skip boundary records already emitted by the prior
                    # window's GTE overlap.
                    if (
                        last_modified is not None
                        and current_since is not None
                        and last_modified == current_since
                        and rec_id in boundary_seen_ids
                    ):
                        continue

                    # Inclusive upper bound. Results are sorted ascending, so
                    # the first record past ``until_ms`` means every remaining
                    # record (this window and beyond) is also out of range —
                    # stop the whole search here.
                    if (
                        until_ms is not None
                        and last_modified is not None
                        and last_modified > until_ms
                    ):
                        return

                    if last_modified is not None:
                        if latest_in_window is None or last_modified > latest_in_window:
                            latest_in_window = last_modified
                            ids_at_latest = {rec_id}
                        elif last_modified == latest_in_window:
                            ids_at_latest.add(rec_id)
                    yield record
                    new_yielded += 1

                page_count += 1
                paging = page.get("paging", {}).get("next") or {}
                after = paging.get("after")
                if not after:
                    break
                # HubSpot caps a single search at 10k records (100
                # pages of 100). Bail before we run off the cliff.
                if page_count >= 100:
                    break

            # No new records past the overlap dedup — the stream is
            # exhausted (also guards the pathological >10k-records-sharing-
            # one-timestamp case from looping forever).
            if new_yielded == 0:
                return
            if latest_in_window is None:
                # No usable timestamps — can't safely re-window.
                return
            if latest_in_window == current_since:
                # Window never advanced past the floor (every record
                # shared the boundary ms); accumulate seen ids so the
                # next overlap query can make progress.
                boundary_seen_ids |= ids_at_latest
            else:
                boundary_seen_ids = ids_at_latest
            # Advance the floor to the latest record we just saw; GTE
            # keeps boundary-tied records in range, dedup drops repeats.
            current_since = latest_in_window

    def _iter_kb_articles(
        self,
        since_ms: int | None,
        until_ms: int | None = None,
        allow_missing: bool = True,
    ) -> Generator[dict, None, None]:
        """Yield Knowledge Base articles via the CMS knowledge endpoint.

        KB articles aren't exposed via CRM search, so the connector
        pages the dedicated ``/cms/v3/knowledge/articles`` endpoint and
        filters client-side by ``updatedAt`` (strict-lower / inclusive-upper
        window). This is bounded by the ``after`` cursor and stops the moment
        we see an article at or older than the floor (results come back
        newest-first by default).

        ``allow_missing`` controls how a ``404`` (portal without a Knowledge
        Base) is treated: the sync path passes ``True`` so an absent KB does
        not abort the run, while the prune path passes ``False`` so a
        missing/unavailable KB aborts prune (fail closed) instead of
        returning a CRM-only snapshot that would wrongly delete KB documents.
        """
        url = f"{_API_BASE}/cms/v3/knowledge/articles"
        after: str | None = None
        while True:
            params = {
                "limit": _SEARCH_PAGE_SIZE,
                "state": _KB_PUBLISHED_STATE,
                "sort": "-updatedAt",
            }
            if after:
                params["after"] = after

            resp = self._request("GET", url, params=params)
            if resp.status_code == 404:
                if allow_missing:
                    # Portal doesn't have Knowledge Base enabled — skip
                    # silently rather than abort the surrounding sync.
                    return
                raise UnexpectedValidationError(
                    "HubSpot Knowledge Base is unavailable (HTTP 404); refusing to "
                    "return a partial prune snapshot that could delete KB documents."
                )
            page = self._json(resp, context="kb_articles list")

            results = page.get("results", []) or []
            for article in results:
                updated_at = _iso_to_ms(article.get("updatedAt"))
                if since_ms is not None and updated_at is not None and updated_at <= since_ms:
                    # Sorted descending; first stale entry means everything
                    # after it is also stale.
                    return
                if until_ms is not None and updated_at is not None and updated_at > until_ms:
                    # Too new for this snapshot window; descending order means
                    # newer articles come first, so skip until we drop in-range.
                    continue
                yield article

            after = (page.get("paging", {}).get("next") or {}).get("after")
            if not after:
                return

    # ------------------------------------------------------------------
    # Document construction
    # ------------------------------------------------------------------

    def _iter_documents(
        self,
        checkpoint: HubSpotCheckpoint | None = None,
        since_epoch: float | None = None,
        until_epoch: float | None = None,
    ):
        from common.data_source.models import Document

        if self._access_token is None:
            raise ConnectorMissingCredentialError("HubSpot")

        global_since_ms: int | None = None
        if since_epoch:
            global_since_ms = int(since_epoch * 1000)
        global_until_ms: int | None = None
        if until_epoch:
            global_until_ms = int(until_epoch * 1000)

        batch: list[Document] = []

        # Strict-lower / inclusive-upper window: records are pulled with
        # ``since`` < hs_lastmodifieddate and bounded above by ``until`` (the
        # framework's snapshot end). Honoring the upper bound keeps records
        # modified mid-run from leaking in and pushing the watermark past the
        # true window (they're picked up next run, whose lower bound is this
        # run's upper bound — so an update can never fall into a gap).
        #
        # Fail closed: every object is queried from the global watermark and
        # any object/KB failure propagates so the whole run aborts. The sync
        # framework advances the global watermark only when a run completes,
        # so aborting keeps it pinned and the next run retries the same window
        # — a partial object failure can never move the watermark past records
        # it never ingested. Re-running re-fetches already-seen records, which
        # the content-hash dedup drops. (A 401/403 surfaces as
        # InsufficientPermissionsError via _json; a genuinely absent KB
        # returns quietly from _iter_kb_articles on 404 here, while prune
        # treats a missing KB as fail-closed.)
        for obj in self.objects:
            props = self.properties.get(obj, _DEFAULT_PROPERTIES.get(obj, ["hs_object_id"]))
            for record in self._search_records(obj, props, global_since_ms, global_until_ms):
                doc, _ = self._record_to_document(obj, record)
                if doc is None:
                    continue
                batch.append(doc)
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

        if self.include_knowledge_base:
            for article in self._iter_kb_articles(global_since_ms, global_until_ms):
                doc, _ = self._kb_article_to_document(article)
                if doc is None:
                    continue
                batch.append(doc)
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

        if batch:
            yield batch

        if checkpoint is not None:
            checkpoint.has_more = False

    def _record_to_document(self, obj: str, record: dict):
        from common.data_source.models import Document

        rec_id = record.get("id")
        if not rec_id:
            return None, None

        props = record.get("properties", {}) or {}
        # Deterministic key order matters for content hashing: HubSpot
        # may reorder property responses, and without a stable sort
        # every poll would mark every record "changed" and we'd
        # re-embed the entire portal on each run.
        lines = [f"HubSpot {obj}"]
        for key in sorted(props.keys()):
            value = props[key]
            if value in (None, ""):
                continue
            lines.append(f"{key}: {value}")
        body = "\n".join(lines)
        blob = body.encode("utf-8")

        last_modified_ms = _record_lastmodified_ms(record)
        if last_modified_ms is not None:
            doc_updated_at = datetime.fromtimestamp(last_modified_ms / 1000, tz=timezone.utc)
        else:
            doc_updated_at = datetime.now(timezone.utc)

        name = (
            props.get("name")
            or props.get("dealname")
            or props.get("subject")
            or _full_name(props)
            or f"{obj}/{rec_id}"
        )

        doc = Document(
            id=f"{obj}/{rec_id}",
            source="hubspot",
            semantic_identifier=str(name),
            extension=".txt",
            blob=blob,
            doc_updated_at=doc_updated_at,
            size_bytes=len(blob),
            metadata={
                "object": obj,
                "record_id": rec_id,
                "web_url": _crm_url(obj, rec_id),
            },
        )
        return doc, last_modified_ms

    def _kb_article_to_document(self, article: dict):
        from common.data_source.models import Document

        article_id = article.get("id")
        if not article_id:
            return None, None

        title = article.get("title") or f"kb_articles/{article_id}"
        # The CMS KB endpoint returns ``htmlBody``/``body`` and a few
        # adjacent fields; we render what's present and rely on the
        # parser to strip HTML downstream.
        body_fields = ["htmlBody", "body", "description", "subTitle", "category"]
        lines = [f"HubSpot kb_article: {title}"]
        for key in body_fields:
            value = article.get(key)
            if value:
                lines.append(f"{key}: {value}")
        body = "\n".join(lines)
        blob = body.encode("utf-8")

        updated_ms = _iso_to_ms(article.get("updatedAt"))
        if updated_ms is not None:
            doc_updated_at = datetime.fromtimestamp(updated_ms / 1000, tz=timezone.utc)
        else:
            doc_updated_at = datetime.now(timezone.utc)

        doc = Document(
            id=f"kb_articles/{article_id}",
            source="hubspot",
            semantic_identifier=str(title),
            extension=".txt",
            blob=blob,
            doc_updated_at=doc_updated_at,
            size_bytes=len(blob),
            metadata={
                "object": "kb_articles",
                "record_id": article_id,
                "web_url": article.get("url", ""),
            },
        )
        return doc, updated_ms


# ----------------------------------------------------------------------
# Module-level helpers
# ----------------------------------------------------------------------

def _record_lastmodified_ms(record: dict) -> int | None:
    """Extract ``hs_lastmodifieddate`` (ISO string) from a record and
    convert it to epoch milliseconds. Falls back to ``updatedAt`` (an
    ISO string at the record root) when properties don't carry the
    timestamp — both forms exist in v3 responses depending on the
    object family."""
    iso = (record.get("properties") or {}).get("hs_lastmodifieddate") or record.get("updatedAt")
    return _iso_to_ms(iso)


def _iso_to_ms(value: Any) -> int | None:
    if not value:
        return None
    if isinstance(value, (int, float)):
        return int(value)
    try:
        dt = datetime.fromisoformat(str(value).replace("Z", "+00:00"))
    except ValueError:
        return None
    return int(dt.timestamp() * 1000)


def _full_name(props: dict) -> str:
    first = (props.get("firstname") or "").strip()
    last = (props.get("lastname") or "").strip()
    full = f"{first} {last}".strip()
    return full or ""


def _crm_url(obj: str, rec_id: str) -> str:
    # HubSpot CRM record URLs follow ``/<obj-key>/<id>``; without the
    # portal ID we can only emit the relative path the UI later
    # composes against the user's portal.
    obj_path = {
        "contacts": "contacts",
        "companies": "companies",
        "deals": "deals",
        "tickets": "tickets",
    }.get(obj, obj)
    return f"/{obj_path}/{rec_id}"

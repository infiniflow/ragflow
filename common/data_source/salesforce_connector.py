"""Salesforce data source connector.

Talks to a Salesforce org over the REST + SOQL APIs, turns each selected
object's records into a Document, and uses ``SystemModstamp`` as the
incremental cursor so re-syncs only fetch what changed.

Auth is OAuth 2.0 client-credentials (Salesforce "Connected App"); the
caller supplies the org's ``instance_url`` so we never have to guess the
pod hostname. A small allow-list of objects ships out of the box
(Account, Contact, Opportunity, Case, Knowledge__kav) so the connector
boots without per-org configuration, while the ``objects`` field lets
operators add or replace entries.
"""

from __future__ import annotations

import logging
from datetime import datetime, timezone
from typing import Any, Generator
from urllib.parse import urljoin

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

_DEFAULT_API_VERSION = "v59.0"

# CRM objects we index by default. Operators can override via the
# ``objects`` config list; Knowledge__kav is included because the issue
# (#15461) calls out Knowledge articles explicitly, but it silently
# downgrades to a skip when the org doesn't have Salesforce Knowledge
# enabled (the SObject describe returns 404).
_DEFAULT_OBJECTS = ["Account", "Contact", "Opportunity", "Case", "Knowledge__kav"]

# Optional default object: Knowledge articles only exist when the org has
# Salesforce Knowledge enabled. It is the one object we skip silently when
# absent — every other configured object is required, so its absence/failure
# is treated as an error rather than quietly dropped.
_OPTIONAL_OBJECTS = frozenset({"Knowledge__kav"})


class SalesforceObjectUnavailable(UnexpectedValidationError):
    """An SObject is genuinely absent or not queryable for this org/user.

    Raised for HTTP 404 (describe of a non-existent object) and HTTP 400
    ``INVALID_TYPE`` (SOQL against a non-existent object). It subclasses
    ``UnexpectedValidationError`` so existing broad handlers still catch it,
    while letting prune distinguish "object genuinely missing" (safe to
    skip — it has no records to orphan) from a transient failure (5xx/429,
    permission, partial page) that must abort rather than delete live docs.
    """


def _is_object_unavailable(resp: requests.Response) -> bool:
    """True when *resp* indicates the SObject simply does not exist for this
    org/user, as opposed to a transient/permission error.

    Salesforce returns 404 for describe of an unknown object and 400 with
    ``errorCode == "INVALID_TYPE"`` for SOQL against one. 403 (no access) and
    5xx/429 (transient) are deliberately NOT treated as "unavailable" — those
    must propagate so callers don't act on an incomplete picture.
    """
    if resp.status_code == 404:
        return True
    if resp.status_code == 400:
        try:
            payload = resp.json()
        except ValueError:
            return "INVALID_TYPE" in (resp.text or "")
        entries = payload if isinstance(payload, list) else [payload]
        for entry in entries:
            if isinstance(entry, dict) and entry.get("errorCode") == "INVALID_TYPE":
                return True
    return False


class SalesforceCheckpoint(ConnectorCheckpoint):
    """Per-object SystemModstamp cursor.

    Stored as ISO-8601 strings (Salesforce's native format) keyed by
    SObject name so each object advances independently — a sync that
    fails halfway through ``Case`` does not rewind ``Account``.
    """

    cursors: dict[str, str] | None = None


class SalesforceConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """Salesforce CRM connector.

    Requires a Connected App with:
      - ``Client Credentials Flow`` enabled
      - OAuth scopes: ``api``, ``refresh_token`` (refresh_token not used
        but Salesforce requires the scope set to issue an access token)

    The execution user must have read access to every object listed in
    ``objects`` — missing permissions surface as ``403`` during
    validation rather than silent empty pages.
    """

    def __init__(
        self,
        batch_size: int = INDEX_BATCH_SIZE,
        objects: list[str] | None = None,
        api_version: str = _DEFAULT_API_VERSION,
    ) -> None:
        self.batch_size = batch_size
        self.api_version = api_version
        self.objects = [obj.strip() for obj in (objects or _DEFAULT_OBJECTS) if obj and obj.strip()]
        self._instance_url: str | None = None
        self._access_token: str | None = None

    # ------------------------------------------------------------------
    # Auth
    # ------------------------------------------------------------------

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        instance_url = (credentials.get("instance_url") or "").rstrip("/")
        client_id = credentials.get("client_id")
        client_secret = credentials.get("client_secret")

        if not all([instance_url, client_id, client_secret]):
            raise ConnectorMissingCredentialError("Salesforce credentials are incomplete (instance_url, client_id, client_secret required)")

        token_url = urljoin(instance_url + "/", "services/oauth2/token")
        try:
            resp = requests.post(
                token_url,
                data={
                    "grant_type": "client_credentials",
                    "client_id": client_id,
                    "client_secret": client_secret,
                },
                timeout=60,
            )
        except requests.RequestException as exc:
            raise ConnectorMissingCredentialError(f"Salesforce token request failed: {exc}")

        if not resp.ok:
            # Salesforce returns {"error": "...", "error_description": "..."}
            try:
                body = resp.json()
                detail = body.get("error_description") or body.get("error") or resp.text
            except ValueError:
                detail = resp.text[:200]
            raise ConnectorMissingCredentialError(f"Failed to acquire Salesforce access token (HTTP {resp.status_code}): {detail}")

        data = resp.json()
        token = data.get("access_token")
        # Salesforce echoes back the *canonical* instance for the org —
        # prefer it so multi-pod orgs (NA1 → NA45 migrations) hit the
        # correct host even when the configured URL went stale.
        canonical = (data.get("instance_url") or "").rstrip("/")
        if not token:
            raise ConnectorMissingCredentialError("Salesforce token response did not contain access_token")

        self._access_token = token
        self._instance_url = canonical or instance_url
        return None

    # ------------------------------------------------------------------
    # Validation
    # ------------------------------------------------------------------

    def validate_connector_settings(self) -> None:
        if not self._access_token or not self._instance_url:
            raise ConnectorMissingCredentialError("Salesforce")

        # Cheap reachability + permission probe: the /sobjects endpoint
        # lists every object the user can describe; a 401/403 here means
        # the connected app or the user lacks API access altogether.
        resp = self._get(f"{self._base()}/sobjects")
        if resp.status_code == 401:
            raise ConnectorMissingCredentialError("Salesforce access token is invalid or expired.")
        if resp.status_code == 403:
            raise InsufficientPermissionsError("The Salesforce execution user lacks API access; enable the 'API Enabled' profile permission.")
        if not resp.ok:
            raise UnexpectedValidationError(f"Salesforce validation failed (HTTP {resp.status_code}): {resp.text[:200]}")

        try:
            payload = resp.json()
        except ValueError as exc:
            raise ConnectorValidationError(f"Salesforce /sobjects response is not JSON: {exc}")
        if "sobjects" not in payload:
            raise ConnectorValidationError("Unexpected response format from Salesforce /sobjects.")

        # Fail fast on typos / inaccessible objects instead of silently
        # missing their data during sync. The global describe lists every
        # object the user can see plus its queryable flag, so we can vet the
        # configured objects without an extra call per object.
        queryable = {so["name"]: bool(so.get("queryable", False)) for so in payload.get("sobjects", []) if isinstance(so, dict) and so.get("name")}
        unknown: list[str] = []
        not_queryable: list[str] = []
        for obj in self.objects:
            if obj not in queryable:
                # Knowledge__kav is an optional default — absent unless the
                # org has Salesforce Knowledge. Don't fail validation for it.
                if obj in _OPTIONAL_OBJECTS:
                    logger.warning(
                        "Salesforce: optional object %s not present in this org; it will be skipped.",
                        obj,
                    )
                    continue
                unknown.append(obj)
            elif not queryable[obj]:
                not_queryable.append(obj)

        if unknown or not_queryable:
            problems = []
            if unknown:
                problems.append(f"unknown object(s): {', '.join(sorted(unknown))}")
            if not_queryable:
                problems.append(f"non-queryable object(s): {', '.join(sorted(not_queryable))}")
            raise ConnectorValidationError("Salesforce 'objects' configuration is invalid — " + "; ".join(problems) + ". Check for typos and that the execution user has read access to each object.")

    # ------------------------------------------------------------------
    # Checkpoint helpers
    # ------------------------------------------------------------------

    def build_dummy_checkpoint(self) -> SalesforceCheckpoint:
        return SalesforceCheckpoint(has_more=True, cursors={})

    def validate_checkpoint_json(self, checkpoint_json: str) -> SalesforceCheckpoint:
        try:
            return SalesforceCheckpoint.model_validate_json(checkpoint_json)
        except Exception:
            return self.build_dummy_checkpoint()

    # ------------------------------------------------------------------
    # Core data loading
    # ------------------------------------------------------------------

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> Any:
        return self._iter_documents(since_epoch=start, until_epoch=end if end else None)

    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> Any:
        if not isinstance(checkpoint, SalesforceCheckpoint):
            checkpoint = self.build_dummy_checkpoint()
        since = start if start else None
        until = end if end else None
        return self._iter_documents(checkpoint=checkpoint, since_epoch=since, until_epoch=until)

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

        Salesforce records use 15/18-character object IDs; we surface
        the SObject-prefixed form (``Account/0015g00000...``) so the
        prune collector can disambiguate IDs that collide across object
        types and so deletes are scoped to the connector instance.
        """
        if not self._access_token:
            raise ConnectorMissingCredentialError("Salesforce")

        batch: list[SlimDocument] = []
        for obj in self.objects:
            try:
                for record in self._query_records(obj, fields=["Id"], since_epoch=None):
                    rec_id = record.get("Id")
                    if not rec_id:
                        continue
                    doc_id = f"{obj}/{rec_id}"
                    if callback:
                        callback(doc_id, obj)
                    batch.append(SlimDocument(id=doc_id))
                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []
            except SalesforceObjectUnavailable:
                # Object genuinely absent (e.g. Knowledge__kav without
                # Salesforce Knowledge). It has no records, so omitting it
                # cannot orphan documents — safe to skip.
                logger.warning("Salesforce prune skipping %s (object unavailable)", obj)
                continue
            # Any OTHER failure (transient 5xx/429, permission, a partial
            # page mid-enumeration) propagates: prune must NOT run on an
            # incomplete snapshot, or the collector would treat the missing
            # IDs as stale and delete documents that still exist.
        if batch:
            yield batch

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _base(self) -> str:
        return f"{self._instance_url}/services/data/{self.api_version}"

    def _get(self, url: str) -> requests.Response:
        return requests.get(
            url,
            headers={
                "Authorization": f"Bearer {self._access_token}",
                "Accept": "application/json",
            },
            timeout=60,
        )

    def _get_json(self, url: str, *, context: str) -> dict:
        """GET *url* and decode JSON. Raise on non-2xx so a 429 / 5xx
        never silently advances the checkpoint past missing records."""
        resp = self._get(url)
        if not resp.ok:
            body_snippet = resp.text[:200] if resp.text else ""
            logger.error(
                "Salesforce request failed (%s): HTTP %s url=%s body=%s",
                context,
                resp.status_code,
                url,
                body_snippet,
            )
            if _is_object_unavailable(resp):
                raise SalesforceObjectUnavailable(f"Salesforce object unavailable ({context}): HTTP {resp.status_code} {body_snippet}")
            raise UnexpectedValidationError(f"Salesforce request failed ({context}): HTTP {resp.status_code} {body_snippet}")
        try:
            return resp.json()
        except ValueError as exc:
            raise UnexpectedValidationError(f"Salesforce response is not JSON ({context}): {exc}")

    def _describe_fields(self, obj: str) -> list[str]:
        """Return field API names for *obj*. Filters out compound types
        (address, location) that SOQL can only project via sub-fields."""
        data = self._get_json(
            f"{self._base()}/sobjects/{obj}/describe",
            context=f"describe {obj}",
        )
        fields = []
        for field in data.get("fields", []):
            if field.get("type") in {"address", "location"}:
                continue
            name = field.get("name")
            if name:
                fields.append(name)
        if "Id" not in fields:
            fields.insert(0, "Id")
        return fields

    def _query_records(
        self,
        obj: str,
        fields: list[str],
        since_epoch: float | None,
        until_epoch: float | None = None,
    ) -> Generator[dict, None, None]:
        """Yield raw record dicts for *obj*, page by page, oldest first.

        Orders by ``SystemModstamp`` so the per-object cursor advances
        monotonically even when paging is interrupted: the next run
        resumes from the latest persisted timestamp and Salesforce
        returns records strictly newer than that.
        """
        field_list = ",".join(fields)
        filters = []
        if since_epoch:
            since_iso = datetime.fromtimestamp(since_epoch, tz=timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
            filters.append(f"SystemModstamp > {since_iso}")
        if until_epoch:
            until_iso = datetime.fromtimestamp(until_epoch, tz=timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
            filters.append(f"SystemModstamp <= {until_iso}")
        where = f" WHERE {' AND '.join(filters)}" if filters else ""
        soql = f"SELECT {field_list} FROM {obj}{where} ORDER BY SystemModstamp ASC"

        url: str | None = f"{self._base()}/query?q={requests.utils.quote(soql)}"
        while url:
            page = self._get_json(url, context=f"query {obj}")
            for record in page.get("records", []):
                yield record
            next_url = page.get("nextRecordsUrl")
            url = f"{self._instance_url}{next_url}" if next_url else None

    @staticmethod
    def _record_to_text(obj: str, record: dict) -> str:
        """Flatten a SOQL record into a deterministic plain-text body.

        Keeping the formatter deterministic (sorted field order, stable
        ``key: value`` lines) matters for content hashing — without it,
        Salesforce field-reordering on the server would tag every
        record as "changed" on every poll and re-embed the entire
        org each sync.
        """
        lines = [f"Salesforce {obj}"]
        for key in sorted(record.keys()):
            if key in ("attributes",):
                continue
            value = record[key]
            if value is None or value == "":
                continue
            if isinstance(value, (dict, list)):
                # Nested relationships (e.g. Account on Contact) — keep
                # the type name + id rather than recursing arbitrarily.
                lines.append(f"{key}: {value}")
            else:
                lines.append(f"{key}: {value}")
        return "\n".join(lines)

    def _iter_documents(
        self,
        checkpoint: SalesforceCheckpoint | None = None,
        since_epoch: float | None = None,
        until_epoch: float | None = None,
    ):
        from common.data_source.models import Document

        cursors: dict[str, str] = {}
        if checkpoint and checkpoint.cursors:
            cursors = dict(checkpoint.cursors)

        batch: list[Document] = []

        for obj in self.objects:
            try:
                fields = self._describe_fields(obj)
            except SalesforceObjectUnavailable:
                # Object genuinely absent (e.g. Knowledge__kav without
                # Salesforce Knowledge): skip it. Transient describe
                # failures are NOT swallowed — they raise below so the run
                # doesn't silently miss an object's data.
                logger.warning(
                    "Salesforce skipping %s (object not present in this org)",
                    obj,
                )
                continue

            # Per-object cursor takes precedence over the caller's
            # window. The cursor was persisted from the *last successful
            # record* so it cannot rewind past records we already
            # ingested even if the caller passes an older since_epoch.
            obj_since = since_epoch
            cursor_iso = cursors.get(obj)
            if cursor_iso:
                try:
                    cur_dt = datetime.fromisoformat(cursor_iso.replace("Z", "+00:00"))
                    cur_ts = cur_dt.timestamp()
                    obj_since = max(obj_since or 0, cur_ts)
                except ValueError:
                    pass

            latest_iso: str | None = cursor_iso
            try:
                for record in self._query_records(obj, fields, obj_since, until_epoch):
                    rec_id = record.get("Id")
                    if not rec_id:
                        continue

                    modified_str: str = record.get("SystemModstamp", "")
                    modified_dt: datetime | None = None
                    if modified_str:
                        try:
                            modified_dt = datetime.fromisoformat(modified_str.replace("Z", "+00:00"))
                        except ValueError:
                            modified_dt = None

                    doc_updated_at = modified_dt or datetime.now(timezone.utc)

                    # Display name: prefer ``Name``; fall back to
                    # ``Subject`` (Case) or ``Title`` (Knowledge); last
                    # resort is ``<Object>/<Id>`` so the doc list is
                    # never blank-titled.
                    name = record.get("Name") or record.get("Subject") or record.get("Title") or f"{obj}/{rec_id}"

                    body = self._record_to_text(obj, record)
                    blob = body.encode("utf-8")

                    doc = Document(
                        id=f"{obj}/{rec_id}",
                        source="salesforce",
                        semantic_identifier=str(name),
                        extension=".txt",
                        blob=blob,
                        doc_updated_at=doc_updated_at,
                        size_bytes=len(blob),
                        metadata={
                            "object": obj,
                            "record_id": rec_id,
                            "web_url": f"{self._instance_url}/{rec_id}",
                        },
                    )
                    batch.append(doc)
                    if modified_str:
                        latest_iso = modified_str
                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []
            except UnexpectedValidationError as exc:
                # Do not continue: advancing to the next object would let
                # the task finish as DONE and move the global poll window
                # past the failed object's missing records permanently.
                logger.warning("Salesforce %s query failed: %s", obj, exc)
                raise

            if latest_iso:
                cursors[obj] = latest_iso

        if batch:
            yield batch

        if checkpoint is not None:
            checkpoint.cursors = cursors
            checkpoint.has_more = False

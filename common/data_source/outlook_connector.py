"""Outlook / Microsoft 365 mail data source connector"""

from datetime import datetime, timezone
from typing import Any

import msal
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
from common.data_source.models import (
    BasicExpertInfo,
    ConnectorCheckpoint,
    Document,
    TextSection,
)

_GRAPH_BASE = "https://graph.microsoft.com/v1.0"
_GRAPH_SCOPE = ["https://graph.microsoft.com/.default"]

# Default folder when none specified; "inbox" is a well-known folder ID.
_DEFAULT_FOLDER = "inbox"


class OutlookCheckpoint(ConnectorCheckpoint):
    """Outlook-specific checkpoint tracking delta links per user mailbox."""
    delta_links: dict[str, str] | None = None


def _strip_html(html: str) -> str:
    """Tiny HTML-to-text fallback. Avoids pulling in BeautifulSoup just for this."""
    if not html:
        return ""
    text = html
    # remove script/style blocks crudely
    for tag in ("script", "style"):
        while True:
            start = text.lower().find(f"<{tag}")
            if start == -1:
                break
            end = text.lower().find(f"</{tag}>", start)
            if end == -1:
                text = text[:start]
                break
            text = text[:start] + text[end + len(tag) + 3 :]
    # drop remaining tags
    out: list[str] = []
    in_tag = False
    for ch in text:
        if ch == "<":
            in_tag = True
            continue
        if ch == ">":
            in_tag = False
            continue
        if not in_tag:
            out.append(ch)
    return "".join(out).strip()


class OutlookConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """
    Outlook / Microsoft 365 mail connector.

    Uses Microsoft Graph delta queries against
    `/users/{id}/mailFolders/{folder}/messages/delta`, persisting per-user
    delta links so incremental syncs only fetch changed messages.

    Required Azure AD application permission:
      - Mail.Read
      - User.Read.All  (only needed when no explicit user_ids are provided,
                       so the connector can enumerate mailboxes)
    """

    def __init__(
        self,
        batch_size: int = INDEX_BATCH_SIZE,
        folder: str = _DEFAULT_FOLDER,
        user_ids: list[str] | None = None,
    ) -> None:
        self.batch_size = batch_size
        self.folder = folder or _DEFAULT_FOLDER
        # Optional list of UPNs / object IDs to limit which mailboxes are synced.
        self.user_ids = user_ids or []
        self._access_token: str | None = None
        self._tenant_id: str | None = None

    # ------------------------------------------------------------------
    # Auth
    # ------------------------------------------------------------------

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        tenant_id = credentials.get("tenant_id")
        client_id = credentials.get("client_id")
        client_secret = credentials.get("client_secret")

        if not all([tenant_id, client_id, client_secret]):
            raise ConnectorMissingCredentialError(
                "Outlook credentials are incomplete (tenant_id, client_id, "
                "client_secret required)"
            )

        self._tenant_id = tenant_id

        app = msal.ConfidentialClientApplication(
            client_id=client_id,
            client_credential=client_secret,
            authority=f"https://login.microsoftonline.com/{tenant_id}",
        )
        result = app.acquire_token_for_client(scopes=_GRAPH_SCOPE)

        if "access_token" not in result:
            error = result.get("error_description", result.get("error", "unknown"))
            raise ConnectorMissingCredentialError(
                f"Failed to acquire Outlook access token: {error}"
            )

        self._access_token = result["access_token"]
        return None

    # ------------------------------------------------------------------
    # Validation
    # ------------------------------------------------------------------

    def validate_connector_settings(self) -> None:
        if not self._access_token:
            raise ConnectorMissingCredentialError("Outlook")

        # Probe: list one user (or check explicit user mailbox).
        probe_url = (
            f"{_GRAPH_BASE}/users/{self.user_ids[0]}"
            if self.user_ids
            else f"{_GRAPH_BASE}/users?$top=1"
        )
        resp = self._get(probe_url)

        if resp.status_code == 401:
            raise ConnectorMissingCredentialError(
                "Outlook access token is invalid or expired."
            )
        if resp.status_code == 403:
            raise InsufficientPermissionsError(
                "The service principal lacks the 'Mail.Read' (and possibly "
                "'User.Read.All') permission required by the Outlook connector."
            )
        if resp.status_code == 404 and self.user_ids:
            raise ConnectorValidationError(
                f"Configured Outlook mailbox '{self.user_ids[0]}' does not exist "
                "in this tenant."
            )
        if not resp.ok:
            raise UnexpectedValidationError(
                f"Outlook validation failed (HTTP {resp.status_code}): "
                f"{resp.text[:200]}"
            )

    # ------------------------------------------------------------------
    # Checkpoint helpers
    # ------------------------------------------------------------------

    def build_dummy_checkpoint(self) -> OutlookCheckpoint:
        return OutlookCheckpoint(has_more=True, delta_links={})

    def validate_checkpoint_json(self, checkpoint_json: str) -> OutlookCheckpoint:
        try:
            return OutlookCheckpoint.model_validate_json(checkpoint_json)
        except Exception:
            return self.build_dummy_checkpoint()

    # ------------------------------------------------------------------
    # Core data loading
    # ------------------------------------------------------------------

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> Any:
        return self._iter_documents(since_epoch=start)

    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> Any:
        if not isinstance(checkpoint, OutlookCheckpoint):
            checkpoint = self.build_dummy_checkpoint()
        return self._iter_documents(checkpoint=checkpoint)

    def load_from_checkpoint_with_perm_sync(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> Any:
        return self.load_from_checkpoint(start, end, checkpoint)

    def retrieve_all_slim_docs_perm_sync(self, callback: Any = None) -> Any:
        """Yield lightweight (id, subject) tuples for prune-sync."""
        for user_id in self._list_user_ids():
            url: str | None = self._delta_url(user_id)
            while url:
                resp = self._get(url)
                if not resp.ok:
                    break
                data = resp.json()
                for msg in data.get("value", []):
                    if msg.get("@removed"):
                        continue
                    if callback:
                        callback(msg["id"], msg.get("subject", ""))
                    yield {"id": msg["id"], "subject": msg.get("subject", "")}
                url = data.get("@odata.nextLink")

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _get(self, url: str) -> requests.Response:
        return requests.get(
            url,
            headers={"Authorization": f"Bearer {self._access_token}"},
            timeout=60,
        )

    def _list_user_ids(self) -> list[str]:
        """Return mailbox identifiers to sync."""
        if self.user_ids:
            return list(self.user_ids)

        ids: list[str] = []
        url: str | None = f"{_GRAPH_BASE}/users?$select=id,userPrincipalName,mail"
        while url:
            resp = self._get(url)
            if not resp.ok:
                break
            data = resp.json()
            for user in data.get("value", []):
                # Skip users with no mailbox provisioned.
                if user.get("mail") or user.get("userPrincipalName"):
                    ids.append(user["id"])
            url = data.get("@odata.nextLink")
        return ids

    def _delta_url(self, user_id: str, delta_link: str | None = None) -> str:
        if delta_link:
            return delta_link
        return (
            f"{_GRAPH_BASE}/users/{user_id}/mailFolders/"
            f"{self.folder}/messages/delta"
        )

    def _message_to_document(
        self, msg: dict[str, Any], user_id: str
    ) -> Document | None:
        subject: str = msg.get("subject") or "(no subject)"

        body_obj = msg.get("body") or {}
        body_content_type: str = body_obj.get("contentType", "text").lower()
        body_content: str = body_obj.get("content") or ""
        if body_content_type == "html":
            body_text = _strip_html(body_content)
        else:
            body_text = body_content

        received_str: str = msg.get("receivedDateTime") or ""
        received_dt: datetime | None = None
        if received_str:
            try:
                received_dt = datetime.fromisoformat(
                    received_str.replace("Z", "+00:00")
                )
            except ValueError:
                pass

        from_addr = (
            msg.get("from", {}).get("emailAddress", {}) if msg.get("from") else {}
        )
        to_recipients: list[str] = [
            r.get("emailAddress", {}).get("address", "")
            for r in (msg.get("toRecipients") or [])
            if r.get("emailAddress", {}).get("address")
        ]
        cc_recipients: list[str] = [
            r.get("emailAddress", {}).get("address", "")
            for r in (msg.get("ccRecipients") or [])
            if r.get("emailAddress", {}).get("address")
        ]

        header_lines = [
            f"From: {from_addr.get('name', '')} <{from_addr.get('address', '')}>",
            f"To: {', '.join(to_recipients)}",
        ]
        if cc_recipients:
            header_lines.append(f"Cc: {', '.join(cc_recipients)}")
        header_lines.append(f"Subject: {subject}")

        section_text = "\n".join(header_lines) + "\n\n" + body_text

        primary_owners: list[BasicExpertInfo] = []
        if from_addr.get("address"):
            primary_owners.append(
                BasicExpertInfo(
                    email=from_addr["address"],
                    display_name=from_addr.get("name") or None,
                )
            )

        return Document(
            id=msg["id"],
            semantic_identifier=subject,
            doc_updated_at=received_dt,
            sections=[
                TextSection(
                    link=msg.get("webLink", ""),
                    text=section_text,
                )
            ],
            primary_owners=primary_owners or None,
            metadata={
                "user_id": user_id,
                "folder": self.folder,
                "from": from_addr.get("address", ""),
                "to": ",".join(to_recipients),
                "cc": ",".join(cc_recipients),
                "has_attachments": str(bool(msg.get("hasAttachments"))),
                "conversation_id": msg.get("conversationId", ""),
            },
            source="outlook",
        )

    def _iter_documents(
        self,
        checkpoint: OutlookCheckpoint | None = None,
        since_epoch: float | None = None,
    ):
        """Generator that yields batches of Document objects."""
        delta_links: dict[str, str] = {}
        if checkpoint and checkpoint.delta_links:
            delta_links = dict(checkpoint.delta_links)

        batch: list[Document] = []

        for user_id in self._list_user_ids():
            start_url = self._delta_url(user_id, delta_links.get(user_id))
            url: str | None = start_url
            next_delta: str | None = None

            while url:
                resp = self._get(url)
                if not resp.ok:
                    break
                data = resp.json()

                for msg in data.get("value", []):
                    # Skip removed/deleted messages signalled by delta semantics
                    if msg.get("@removed"):
                        continue

                    received_str = msg.get("receivedDateTime") or ""
                    received_ts: float | None = None
                    if received_str:
                        try:
                            received_ts = datetime.fromisoformat(
                                received_str.replace("Z", "+00:00")
                            ).timestamp()
                        except ValueError:
                            pass

                    if since_epoch and received_ts and received_ts < since_epoch:
                        continue

                    doc = self._message_to_document(msg, user_id)
                    if doc is None:
                        continue
                    if doc.doc_updated_at is None:
                        doc.doc_updated_at = datetime.now(timezone.utc)
                    batch.append(doc)
                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []

                next_delta = data.get("@odata.deltaLink")
                url = data.get("@odata.nextLink")

            if next_delta:
                delta_links[user_id] = next_delta

        if batch:
            yield batch

        if checkpoint is not None:
            checkpoint.delta_links = delta_links
            checkpoint.has_more = False

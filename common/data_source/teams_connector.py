"""Microsoft Teams connector

Ingests Microsoft Teams channel conversations (posts and their replies) via the
Microsoft Graph API (Office365-REST-Python-Client). Authentication uses MSAL
client-credentials (app-only) flow, so it requires an Azure AD app with the
``Team.ReadBasic.All`` and ``ChannelMessage.Read.All`` application permissions
(admin-consented).

Each top-level channel post is flattened together with its replies into one
blob-based ``Document``. Incremental syncs are bounded by the post
``lastModifiedDateTime`` (falling back to ``createdDateTime``).
"""

import logging
from datetime import datetime, timezone
from typing import Any, Generator

import msal
from office365.graph_client import GraphClient

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
    ConnectorCheckpoint,
    ConnectorFailure,
    Document,
    DocumentFailure,
    SlimDocument,
)

_SLIM_DOC_BATCH_SIZE = 5000
GRAPH_SCOPES = ["https://graph.microsoft.com/.default"]


class TeamsCheckpoint(ConnectorCheckpoint):
    """Teams-specific checkpoint"""

    todo_team_ids: list[str] | None = None


class TeamsConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """Microsoft Teams connector for accessing Teams messages and channels."""

    def __init__(self, batch_size: int = _SLIM_DOC_BATCH_SIZE) -> None:
        self.batch_size = batch_size
        self.graph_client: GraphClient | None = None

    # -- credentials ---------------------------------------------------------

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Configure a Microsoft Graph client from app-only credentials.

        Uses a lazy MSAL token callback (the form ``GraphClient`` expects), so
        this performs no network call; the first request acquires the token.
        """
        tenant_id = credentials.get("tenant_id")
        client_id = credentials.get("client_id")
        client_secret = credentials.get("client_secret")

        if not all([tenant_id, client_id, client_secret]):
            raise ConnectorMissingCredentialError("Microsoft Teams credentials are incomplete")

        authority = f"https://login.microsoftonline.com/{tenant_id}"
        # Build the MSAL app once and reuse it across token acquisitions so its
        # in-memory token cache is honored. Re-creating the app on every call
        # (as the callback previously did) defeats the cache and triggers an
        # Azure AD round-trip for each request.
        app = msal.ConfidentialClientApplication(
            client_id=client_id,
            client_credential=client_secret,
            authority=authority,
        )

        def _acquire_token() -> dict[str, Any]:
            """Return a cached or freshly minted app-only Graph token."""
            token = app.acquire_token_for_client(scopes=GRAPH_SCOPES)
            if "access_token" not in token:
                detail = token.get("error_description") or token.get("error") or token
                raise ConnectorMissingCredentialError(f"Failed to acquire Microsoft Teams access token: {detail}")
            return token

        self.graph_client = GraphClient(_acquire_token)
        return None

    def validate_connector_settings(self) -> None:
        """Validate credentials by listing teams."""
        if self.graph_client is None:
            raise ConnectorMissingCredentialError("Microsoft Teams")

        try:
            self.graph_client.teams.get().execute_query()
        except ConnectorValidationError:
            raise
        except Exception as e:
            message = str(e)
            if "401" in message or "403" in message:
                raise InsufficientPermissionsError("Invalid credentials or insufficient permissions for Microsoft Teams")
            raise UnexpectedValidationError(f"Microsoft Teams validation error: {e}")

    # -- helpers -------------------------------------------------------------

    @staticmethod
    def _prop(obj: Any, name: str) -> Any:
        """Read a property by name, falling back to the OData ``properties`` dict."""
        value = getattr(obj, name, None)
        if value is None:
            value = getattr(obj, "properties", {}).get(name)
        return value

    @staticmethod
    def _parse_dt(value: Any) -> datetime | None:
        """Parse a Graph datetime (ISO string or datetime) into a tz-aware UTC datetime."""
        if value is None:
            return None
        if isinstance(value, datetime):
            dt = value
        elif isinstance(value, str):
            try:
                dt = datetime.fromisoformat(value.replace("Z", "+00:00"))
            except ValueError:
                return None
        else:
            return None
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        return dt

    @classmethod
    def _message_body(cls, message: Any) -> tuple[str, str]:
        """Return ``(content, content_type)`` from a message's ItemBody."""
        body = getattr(message, "body", None)
        if body is None:
            return "", "text"
        content = getattr(body, "content", None)
        if content is None:
            content = getattr(body, "properties", {}).get("content")
        content_type = getattr(body, "contentType", None)
        if content_type is None:
            content_type = getattr(body, "properties", {}).get("contentType")
        return content or "", (content_type or "text").lower()

    def _message_to_document(
        self,
        message: Any,
        replies: list[Any],
        team_id: str,
        team_name: str,
        channel_id: str,
        channel_name: str,
    ) -> Document:
        """Flatten a post and its replies into a single blob-based Document."""
        thread = [message, *replies]

        contents = []
        content_type = "text"
        latest = None
        for item in thread:
            text, ctype = self._message_body(item)
            if text:
                contents.append(text)
            if ctype == "html":
                content_type = "html"
            modified = self._parse_dt(self._prop(item, "lastModifiedDateTime")) or self._parse_dt(self._prop(item, "createdDateTime"))
            if modified is not None and (latest is None or modified > latest):
                latest = modified

        joined = "\n\n".join(contents)
        blob = joined.encode("utf-8")

        snippet = joined.strip().replace("\n", " ")
        if len(snippet) > 50:
            snippet = snippet[:50].rstrip() + "..."
        semantic_identifier = f"{channel_name}: {snippet}" if snippet else f"{channel_name} message"

        metadata = {"team": team_name, "channel": channel_name}
        web_url = self._prop(message, "web_url") or self._prop(message, "webUrl")
        if web_url:
            metadata["web_url"] = web_url

        return Document(
            id=f"{team_id}__{channel_id}__{message.id}",
            source="teams",
            semantic_identifier=semantic_identifier,
            extension=".html" if content_type == "html" else ".txt",
            blob=blob,
            size_bytes=len(blob),
            doc_updated_at=latest or datetime.now(timezone.utc),
            metadata=metadata,
        )

    def _iter_channel_messages(self):
        """Yield (team_id, team_name, channel_id, channel_name, message) tuples.

        Uses ``get_all()`` for every collection so Microsoft Graph's
        ``@odata.nextLink`` pages are followed; ``get().execute_query()`` would
        only return the first page and silently drop the rest on larger tenants.
        """
        teams = self.graph_client.teams.get_all().execute_query()
        for team in teams:
            team_id = str(team.id)
            team_name = self._prop(team, "displayName") or team_id
            channels = team.channels.get_all().execute_query()
            for channel in channels:
                channel_id = str(channel.id)
                channel_name = self._prop(channel, "displayName") or channel_id
                messages = channel.messages.get_all().execute_query()
                for message in messages:
                    yield team_id, team_name, channel_id, channel_name, message

    def _generate_documents(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
    ) -> Generator[Document | ConnectorFailure, None, None]:
        """Yield a Document per in-window channel post, or a failure per error."""
        if self.graph_client is None:
            raise ConnectorMissingCredentialError("Microsoft Teams")

        for team_id, team_name, channel_id, channel_name, message in self._iter_channel_messages():
            try:
                modified = self._parse_dt(self._prop(message, "lastModifiedDateTime")) or self._parse_dt(self._prop(message, "createdDateTime"))
                if modified is not None:
                    ts = modified.timestamp()
                    # start is an exclusive lower bound; full reindex passes start=0.
                    if not (start < ts <= end):
                        continue

                replies = list(message.replies.get_all().execute_query())
                yield self._message_to_document(message, replies, team_id, team_name, channel_id, channel_name)
            except Exception as e:
                logging.exception("Microsoft Teams failed to process message")
                yield ConnectorFailure(
                    failed_document=DocumentFailure(
                        document_id=str(getattr(message, "id", "unknown")),
                        document_link=self._prop(message, "web_url") or "",
                    ),
                    failure_message=str(e),
                    exception=e,
                )

    # -- checkpointed connector interface ------------------------------------

    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> Generator[Document | ConnectorFailure, None, ConnectorCheckpoint]:
        """Yield a Document per channel post (with replies), then finish.

        All teams/channels are enumerated in one pass, so the returned
        checkpoint always has ``has_more=False``.
        """
        yield from self._generate_documents(start, end)
        return TeamsCheckpoint(has_more=False)

    def load_from_checkpoint_with_perm_sync(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> Generator[Document | ConnectorFailure, None, ConnectorCheckpoint]:
        """Permission-aware variant.

        Teams ACL -> ExternalAccess mapping is not yet wired through the sync
        pipeline (it does not persist ExternalAccess), so this currently yields
        the same documents as ``load_from_checkpoint``.
        """
        return self.load_from_checkpoint(start, end, checkpoint)

    def build_dummy_checkpoint(self) -> ConnectorCheckpoint:
        return TeamsCheckpoint(has_more=True)

    def validate_checkpoint_json(self, checkpoint_json: str) -> ConnectorCheckpoint:
        return TeamsCheckpoint(has_more=True)

    def retrieve_all_slim_docs_perm_sync(
        self,
        callback: Any = None,
    ) -> Generator[list[SlimDocument], None, None]:
        """Yield batches of slim documents (ids only) for prune/permission sync."""
        if self.graph_client is None:
            raise ConnectorMissingCredentialError("Microsoft Teams")

        batch: list[SlimDocument] = []
        for team_id, _team_name, channel_id, _channel_name, message in self._iter_channel_messages():
            batch.append(SlimDocument(id=f"{team_id}__{channel_id}__{message.id}"))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []
        if batch:
            yield batch

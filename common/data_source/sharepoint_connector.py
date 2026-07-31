"""SharePoint connector

Ingests files from SharePoint document libraries via the Microsoft Graph API
(Office365-REST-Python-Client). Authentication uses MSAL client-credentials
(app-only) flow, so it requires an Azure AD app with the ``Sites.Read.All`` and
``Files.Read.All`` application permissions (admin-consented).

The connector implements the checkpointed-connector interface used by the sync
worker: ``load_from_checkpoint`` walks every document library under the
configured site, downloads each file, and yields blob-based ``Document``
objects. Incremental syncs are bounded by the file ``lastModifiedDateTime``.
"""

import logging
from datetime import datetime, timezone
from typing import Any, Generator

import msal
from office365.graph_client import GraphClient

from common.data_source.config import INDEX_BATCH_SIZE
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
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

GRAPH_SCOPES = ["https://graph.microsoft.com/.default"]


class SharePointConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """SharePoint connector for accessing SharePoint sites and documents."""

    def __init__(self, batch_size: int = INDEX_BATCH_SIZE) -> None:
        self.batch_size = batch_size
        self.graph_client: GraphClient | None = None
        self._site_url: str | None = None

    # -- credentials ---------------------------------------------------------

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Configure a Microsoft Graph client from app-only credentials.

        The token is acquired lazily through a callback (the way
        ``GraphClient`` expects it), so this method performs no network call;
        the first real request triggers ``acquire_token_for_client``.
        """
        tenant_id = credentials.get("tenant_id")
        client_id = credentials.get("client_id")
        client_secret = credentials.get("client_secret")
        site_url = credentials.get("site_url")

        if not all([tenant_id, client_id, client_secret, site_url]):
            raise ConnectorMissingCredentialError("SharePoint credentials are incomplete")

        self._site_url = site_url
        authority = f"https://login.microsoftonline.com/{tenant_id}"

        def _acquire_token() -> dict[str, Any]:
            app = msal.ConfidentialClientApplication(
                client_id=client_id,
                client_credential=client_secret,
                authority=authority,
            )
            token = app.acquire_token_for_client(scopes=GRAPH_SCOPES)
            if "access_token" not in token:
                detail = token.get("error_description") or token.get("error") or token
                raise ConnectorMissingCredentialError(f"Failed to acquire SharePoint access token: {detail}")
            return token

        self.graph_client = GraphClient(_acquire_token)
        return None

    def validate_connector_settings(self) -> None:
        """Validate credentials by resolving the configured site."""
        if self.graph_client is None or not self._site_url:
            raise ConnectorMissingCredentialError("SharePoint")

        try:
            site = self.graph_client.sites.get_by_url(self._site_url).execute_query()
            if not site:
                raise ConnectorValidationError("Failed to access SharePoint site")
        except ConnectorValidationError:
            raise
        except Exception as e:
            message = str(e)
            if "401" in message or "403" in message:
                raise ConnectorValidationError("Invalid credentials or insufficient permissions for SharePoint")
            raise ConnectorValidationError(f"SharePoint validation error: {e}")

    # -- traversal helpers ---------------------------------------------------

    def _iter_drives(self):
        site = self.graph_client.sites.get_by_url(self._site_url).execute_query()
        return site.drives.get().execute_query()

    @staticmethod
    def _is_folder(drive_item: Any) -> bool:
        return "folder" in getattr(drive_item, "properties", {})

    def _walk_files(self, root_item: Any) -> Generator[Any, None, None]:
        """Depth-first walk of a drive yielding file (non-folder) driveItems."""
        stack = [root_item]
        while stack:
            folder = stack.pop()
            children = folder.children.get().execute_query()
            for child in children:
                if self._is_folder(child):
                    stack.append(child)
                else:
                    yield child

    @staticmethod
    def _modified_dt(drive_item: Any) -> datetime | None:
        value = getattr(drive_item, "last_modified_datetime", None)
        if value is None:
            value = getattr(drive_item, "properties", {}).get("lastModifiedDateTime")
        if value is None:
            return None
        if isinstance(value, str):
            try:
                value = datetime.fromisoformat(value.replace("Z", "+00:00"))
            except ValueError:
                return None
        if value.tzinfo is None:
            value = value.replace(tzinfo=timezone.utc)
        return value

    @staticmethod
    def _composite_doc_id(drive_id: Any, drive_item: Any) -> str:
        # Graph driveItem IDs are only unique within a single drive. A site can
        # expose multiple document libraries (drives), so we namespace the item
        # ID by drive ID to keep document identifiers globally unique.
        return f"{drive_id}:{drive_item.id}"

    def _drive_item_to_document(self, drive_item: Any, drive_id: Any, drive_name: str) -> Document:
        name = drive_item.name or str(drive_item.id)
        content_result = drive_item.get_content().execute_query()
        blob = content_result.value or b""
        if isinstance(blob, str):
            blob = blob.encode("utf-8")

        extension = ""
        if "." in name:
            extension = "." + name.rsplit(".", 1)[1]

        size_bytes = getattr(drive_item, "properties", {}).get("size")
        if not size_bytes:
            size_bytes = len(blob)

        modified = self._modified_dt(drive_item) or datetime.now(timezone.utc)

        metadata = {"drive": drive_name, "drive_id": str(drive_id), "drive_item_id": str(drive_item.id)}
        web_url = getattr(drive_item, "web_url", None)
        if web_url:
            metadata["web_url"] = web_url

        return Document(
            id=self._composite_doc_id(drive_id, drive_item),
            source="sharepoint",
            semantic_identifier=name,
            extension=extension,
            blob=blob,
            size_bytes=int(size_bytes),
            doc_updated_at=modified,
            metadata=metadata,
        )

    def _generate_documents(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
    ) -> Generator[Document | ConnectorFailure, None, None]:
        if self.graph_client is None or not self._site_url:
            raise ConnectorMissingCredentialError("SharePoint")

        for drive in self._iter_drives():
            drive_name = getattr(drive, "name", None) or getattr(drive, "properties", {}).get("name", "")
            drive_id = getattr(drive, "id", None) or getattr(drive, "properties", {}).get("id", "")
            for drive_item in self._walk_files(drive.root):
                try:
                    modified = self._modified_dt(drive_item)
                    if modified is not None:
                        ts = modified.timestamp()
                        # start is an exclusive lower bound; full reindex passes start=0.
                        if not (start < ts <= end):
                            continue
                    yield self._drive_item_to_document(drive_item, drive_id, drive_name)
                except Exception as e:
                    logging.exception("SharePoint failed to process drive item")
                    yield ConnectorFailure(
                        failed_document=DocumentFailure(
                            document_id=self._composite_doc_id(drive_id, drive_item) if getattr(drive_item, "id", None) is not None else "unknown",
                            document_link=getattr(drive_item, "web_url", "") or "",
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
        """Yield every file under the site as a Document, then finish.

        The whole library is enumerated in a single pass, so the returned
        checkpoint always has ``has_more=False``.
        """
        yield from self._generate_documents(start, end)
        return ConnectorCheckpoint(has_more=False)

    def load_from_checkpoint_with_perm_sync(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> Generator[Document | ConnectorFailure, None, ConnectorCheckpoint]:
        """Permission-aware variant.

        SharePoint ACL -> ExternalAccess mapping is not yet wired through the
        sync pipeline (the pipeline does not persist ExternalAccess), so this
        currently yields the same documents as ``load_from_checkpoint``.
        """
        return self.load_from_checkpoint(start, end, checkpoint)

    def build_dummy_checkpoint(self) -> ConnectorCheckpoint:
        return ConnectorCheckpoint(has_more=True)

    def validate_checkpoint_json(self, checkpoint_json: str) -> ConnectorCheckpoint:
        return ConnectorCheckpoint(has_more=True)

    def retrieve_all_slim_docs_perm_sync(
        self,
        callback: Any = None,
    ) -> Generator[list[SlimDocument], None, None]:
        """Yield batches of slim documents (ids only) for prune/permission sync."""
        if self.graph_client is None or not self._site_url:
            raise ConnectorMissingCredentialError("SharePoint")

        batch: list[SlimDocument] = []
        for drive in self._iter_drives():
            drive_id = getattr(drive, "id", None) or getattr(drive, "properties", {}).get("id", "")
            for drive_item in self._walk_files(drive.root):
                batch.append(SlimDocument(id=self._composite_doc_id(drive_id, drive_item)))
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []
        if batch:
            yield batch

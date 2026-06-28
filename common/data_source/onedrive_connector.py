"""OneDrive data source connector"""

import logging
from typing import Any, Generator

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
from common.data_source.models import ConnectorCheckpoint, SlimDocument

logger = logging.getLogger(__name__)

_GRAPH_BASE = "https://graph.microsoft.com/v1.0"
_GRAPH_SCOPE = ["https://graph.microsoft.com/.default"]

# File extensions we support for ingestion
_SUPPORTED_EXTENSIONS = {
    ".pdf", ".docx", ".doc", ".xlsx", ".xls",
    ".pptx", ".ppt", ".txt", ".md", ".csv",
}


def _normalize_folder_path(folder_path: str | None) -> str | None:
    """Normalize Graph path-based addressing segment (root:{path}:/delta)."""
    if folder_path is None:
        return None
    path = folder_path.strip()
    if not path:
        return None
    segments = [segment for segment in path.split("/") if segment]
    if ".." in segments:
        raise ConnectorValidationError("folder_path must not contain '..' segments.")
    if not segments:
        return None
    return "/" + "/".join(segments)


class OneDriveCheckpoint(ConnectorCheckpoint):
    """OneDrive-specific checkpoint tracking delta links per drive."""
    delta_links: dict[str, str] | None = None


class OneDriveConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """
    OneDrive / OneDrive for Business connector.

    Uses Microsoft Graph delta queries so incremental syncs only fetch
    changed items.  Requires application permissions:
      - Files.Read.All
    """

    def __init__(
        self,
        batch_size: int = INDEX_BATCH_SIZE,
        folder_path: str | None = None,
    ) -> None:
        self.batch_size = batch_size
        self.folder_path = _normalize_folder_path(folder_path)
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
                "OneDrive credentials are incomplete (tenant_id, client_id, client_secret required)"
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
                f"Failed to acquire OneDrive access token: {error}"
            )

        self._access_token = result["access_token"]
        return None

    # ------------------------------------------------------------------
    # Validation
    # ------------------------------------------------------------------

    def validate_connector_settings(self) -> None:
        if not self._access_token:
            raise ConnectorMissingCredentialError("OneDrive")

        # Probe: list the first page of drives in the tenant.
        # Requires Files.Read.All.
        resp = self._get(f"{_GRAPH_BASE}/drives?$top=1")
        if resp.status_code == 401:
            raise ConnectorMissingCredentialError(
                "OneDrive access token is invalid or expired."
            )
        if resp.status_code == 403:
            raise InsufficientPermissionsError(
                "The service principal lacks the 'Files.Read.All' permission "
                "required by the OneDrive connector."
            )
        if not resp.ok:
            raise UnexpectedValidationError(
                f"OneDrive validation failed (HTTP {resp.status_code}): {resp.text[:200]}"
            )

        data = resp.json()
        if "value" not in data:
            raise ConnectorValidationError(
                "Unexpected response format from Microsoft Graph /drives."
            )

    # ------------------------------------------------------------------
    # Checkpoint helpers
    # ------------------------------------------------------------------

    def build_dummy_checkpoint(self) -> OneDriveCheckpoint:
        return OneDriveCheckpoint(has_more=True, delta_links={})

    def validate_checkpoint_json(self, checkpoint_json: str) -> OneDriveCheckpoint:
        try:
            return OneDriveCheckpoint.model_validate_json(checkpoint_json)
        except Exception:
            return self.build_dummy_checkpoint()

    # ------------------------------------------------------------------
    # Core data loading
    # ------------------------------------------------------------------

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> Any:
        """Return documents modified at or after *start* (epoch seconds).

        Kept for callers that prefer the time-window interface; internally
        defers to the same delta-walk used by load_from_checkpoint and
        filters in-window items by lastModifiedDateTime.
        """
        return self._iter_documents(since_epoch=start)

    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> Any:
        """Resume from *checkpoint*'s delta_links and apply the start filter.

        The delta_links map carries per-drive @odata.deltaLink values from the
        previous run; when present the walk resumes from those links instead
        of crawling each drive's root, which is what makes incremental syncs
        cheap. The start_time is still applied as a lastModifiedDateTime
        floor so callers that pass a window (and have no persisted delta
        link yet) don't have to re-process everything.
        """
        if not isinstance(checkpoint, OneDriveCheckpoint):
            checkpoint = self.build_dummy_checkpoint()
        since = start if start else None
        return self._iter_documents(checkpoint=checkpoint, since_epoch=since)

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

        The prune collector in rag/svr/sync_data_source._collect_prune_snapshot
        calls list.extend(batch) on each yielded value and then accesses
        `.id` on every retained item (see
        api/db/services/connector_service.cleanup_stale_documents_for_task).
        Yielding SlimDocument batches matches both contracts.
        """
        if not self._access_token:
            raise ConnectorMissingCredentialError("OneDrive")

        batch: list[SlimDocument] = []
        for drive_id in self._list_drive_ids():
            url: str | None = self._delta_url(drive_id)
            while url:
                data = self._get_json(url, context=f"prune drive={drive_id}")
                for item in data.get("value", []):
                    if "file" not in item or item.get("deleted"):
                        continue
                    item_id = item.get("id")
                    if not item_id:
                        continue
                    if callback:
                        callback(item_id, item.get("name", ""))
                    batch.append(SlimDocument(id=item_id))
                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []
                url = data.get("@odata.nextLink")
        if batch:
            yield batch

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _get(self, url: str) -> requests.Response:
        return requests.get(
            url,
            headers={"Authorization": f"Bearer {self._access_token}"},
            timeout=60,
        )

    def _get_json(self, url: str, *, context: str) -> dict:
        """GET *url* and decode JSON. Raise on non-2xx so the caller never
        treats a 429 / 5xx as an empty page and silently advances the
        checkpoint past missing data.
        """
        resp = self._get(url)
        if not resp.ok:
            body_snippet = resp.text[:200] if resp.text else ""
            logger.error(
                "OneDrive Graph request failed (%s): HTTP %s url=%s body=%s",
                context,
                resp.status_code,
                url,
                body_snippet,
            )
            raise UnexpectedValidationError(
                f"OneDrive Graph request failed ({context}): HTTP {resp.status_code} {body_snippet}"
            )
        try:
            return resp.json()
        except ValueError as exc:
            raise UnexpectedValidationError(
                f"OneDrive Graph response is not JSON ({context}): {exc}"
            )

    def _list_drive_ids(self) -> list[str]:
        """Return all drive IDs visible to the service principal."""
        ids: list[str] = []
        url: str | None = f"{_GRAPH_BASE}/drives"
        while url:
            data = self._get_json(url, context="list drives")
            ids.extend(d["id"] for d in data.get("value", []) if d.get("id"))
            url = data.get("@odata.nextLink")
        return ids

    def _delta_url(self, drive_id: str, delta_link: str | None = None) -> str:
        if delta_link:
            return delta_link
        base = f"{_GRAPH_BASE}/drives/{drive_id}/root/delta"
        if self.folder_path:
            # Use /drive/root:/{path}:/delta for scoped delta
            base = f"{_GRAPH_BASE}/drives/{drive_id}/root:{self.folder_path}:/delta"
        return base

    def _iter_documents(
        self,
        checkpoint: OneDriveCheckpoint | None = None,
        since_epoch: float | None = None,
    ):
        """
        Generator that yields batches of Document objects.

        Uses Graph delta queries.  When *checkpoint* is supplied its
        delta links are used; otherwise a full crawl is performed.
        """
        from datetime import datetime, timezone

        from common.data_source.models import Document

        delta_links: dict[str, str] = {}
        if checkpoint and checkpoint.delta_links:
            delta_links = dict(checkpoint.delta_links)

        batch: list[Document] = []

        for drive_id in self._list_drive_ids():
            start_url = self._delta_url(drive_id, delta_links.get(drive_id))
            url: str | None = start_url
            next_delta: str | None = None

            while url:
                data = self._get_json(url, context=f"delta drive={drive_id}")

                for item in data.get("value", []):
                    # Skip folders and deleted items
                    if "file" not in item or item.get("deleted"):
                        continue

                    name: str = item.get("name", "")
                    ext = "." + name.rsplit(".", 1)[-1].lower() if "." in name else ""
                    if ext not in _SUPPORTED_EXTENSIONS:
                        continue

                    modified_str: str = item.get("lastModifiedDateTime", "")
                    modified_ts: float | None = None
                    if modified_str:
                        try:
                            dt = datetime.fromisoformat(
                                modified_str.replace("Z", "+00:00")
                            )
                            modified_ts = dt.timestamp()
                        except ValueError:
                            pass

                    # For poll_source: skip items outside the time window
                    if since_epoch and modified_ts and modified_ts < since_epoch:
                        continue

                    doc_updated_at = (
                        datetime.fromtimestamp(modified_ts, tz=timezone.utc)
                        if modified_ts
                        else datetime.now(timezone.utc)
                    )
                    doc = Document(
                        id=item["id"],
                        source="onedrive",
                        semantic_identifier=name,
                        extension=ext,
                        blob=b"",
                        doc_updated_at=doc_updated_at,
                        size_bytes=int(item.get("size", 0) or 0),
                        metadata={
                            "drive_id": drive_id,
                            "web_url": item.get("webUrl", ""),
                            "created_by": (
                                item.get("createdBy", {})
                                .get("user", {})
                                .get("displayName", "")
                            ),
                        },
                    )
                    batch.append(doc)
                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []

                next_delta = data.get("@odata.deltaLink")
                url = data.get("@odata.nextLink")

            if next_delta:
                delta_links[drive_id] = next_delta

        if batch:
            yield batch

        # Update checkpoint
        if checkpoint is not None:
            checkpoint.delta_links = delta_links
            checkpoint.has_more = False

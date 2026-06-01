"""Azure Blob Storage data-source connector.

Ingests blobs from a user's Azure container into a RAGFlow knowledge
base.  This is distinct from RAGFlow's own Azure storage *backend*
(``rag/utils/azure_sas_conn.py``, ``rag/utils/azure_spn_conn.py``),
which stores RAGFlow's own files.

Auth supports three mutually exclusive modes, tried in order of
precedence:

  1. **Connection string** — ``connection_string`` credential; one line,
     everything embedded.  Good for dev / testing.
  2. **Account key** — ``account_name`` + ``account_key``; maps to the
     same underlying SAS-less AccountKey credential.
  3. **SAS token** — ``container_url`` + ``sas_token``; the shape that
     ``RAGFlowAzureSasBlob`` already uses.

Change detection uses blob ETag (an opaque content hash that Azure
updates on every write) stored per blob-name as the fingerprint, so
unchanged blobs are skipped without a download on incremental syncs.
"""

from __future__ import annotations

import logging
from datetime import datetime, timezone
from typing import Any, Generator

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

# Extensions we ingest; mirrors the same set used by the OneDrive
# connector so behaviour is consistent across all file-based sources.
_SUPPORTED_EXTENSIONS = {
    ".pdf", ".docx", ".doc", ".xlsx", ".xls",
    ".pptx", ".ppt", ".txt", ".md", ".csv",
    ".html", ".htm", ".json", ".xml",
}

_AZURE_ENDPOINT_SUFFIX = "blob.core.windows.net"


class AzureBlobCheckpoint(ConnectorCheckpoint):
    """Per-blob ETag fingerprints.

    Stored as ``{blob_name: etag}`` so a re-sync can skip every blob
    whose ETag hasn't changed without downloading it first.  On a full
    reindex (``has_more=True, etags={}``) every blob is fetched fresh.
    """

    etags: dict[str, str] | None = None


class AzureBlobConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """Azure Blob Storage data-source connector.

    Authenticates with one of three credential modes (connection string,
    account key, or SAS token) and enumerates blobs in the configured
    container under an optional prefix.  ETag fingerprints skip
    unchanged blobs so incremental re-syncs are cheap.
    """

    def __init__(
        self,
        batch_size: int = INDEX_BATCH_SIZE,
        prefix: str | None = None,
        allow_images: bool = False,
    ) -> None:
        self.batch_size = batch_size
        self.prefix = (prefix or "").lstrip("/")
        self.allow_images = allow_images
        self._container_client = None

    # ------------------------------------------------------------------
    # Auth
    # ------------------------------------------------------------------

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        from azure.storage.blob import BlobServiceClient, ContainerClient

        conn_str = credentials.get("connection_string")
        account_name = credentials.get("account_name")
        account_key = credentials.get("account_key")
        container_url = (credentials.get("container_url") or "").rstrip("/")
        sas_token = credentials.get("sas_token")
        container_name = credentials.get("container_name") or ""

        try:
            if conn_str:
                # Mode 1: connection string — most permissive; has
                # container name embedded if it follows the standard
                # format, but we still require it separately for safety.
                if not container_name:
                    raise ConnectorMissingCredentialError(
                        "Azure Blob: container_name is required together with connection_string"
                    )
                svc = BlobServiceClient.from_connection_string(conn_str)
                self._container_client = svc.get_container_client(container_name)
            elif account_name and account_key:
                # Mode 2: account key
                if not container_name:
                    raise ConnectorMissingCredentialError(
                        "Azure Blob: container_name is required together with account_name + account_key"
                    )
                account_url = f"https://{account_name}.{_AZURE_ENDPOINT_SUFFIX}"
                svc = BlobServiceClient(
                    account_url=account_url,
                    credential=account_key,
                )
                self._container_client = svc.get_container_client(container_name)
            elif container_url and sas_token:
                # Mode 3: SAS token — mirrors RAGFlowAzureSasBlob
                full_url = f"{container_url}?{sas_token}"
                self._container_client = ContainerClient.from_container_url(full_url)
            else:
                raise ConnectorMissingCredentialError(
                    "Azure Blob credentials are incomplete. Provide one of: "
                    "(a) connection_string + container_name, "
                    "(b) account_name + account_key + container_name, "
                    "(c) container_url + sas_token."
                )
        except ConnectorMissingCredentialError:
            raise
        except Exception as exc:
            raise ConnectorMissingCredentialError(
                f"Failed to initialise Azure Blob client: {exc}"
            ) from exc

        return None

    # ------------------------------------------------------------------
    # Validation
    # ------------------------------------------------------------------

    def validate_connector_settings(self) -> None:
        if self._container_client is None:
            raise ConnectorMissingCredentialError("Azure Blob")

        try:
            # get_container_properties() costs one API call; it returns
            # the ETag and last-modified of the container, proving both
            # the credential and the container name are valid.
            self._container_client.get_container_properties()
        except Exception as exc:
            msg = str(exc)
            code = getattr(getattr(exc, "error_code", None), "value", None) or getattr(exc, "error_code", "")
            if "AuthenticationFailed" in msg or "InvalidAuthenticationInfo" in msg:
                raise ConnectorMissingCredentialError(
                    f"Azure Blob credential rejected: {msg[:300]}"
                ) from exc
            if "AuthorizationPermissionMismatch" in msg or "403" in msg:
                raise InsufficientPermissionsError(
                    f"Azure Blob: insufficient permissions on container: {msg[:300]}"
                ) from exc
            if "ContainerNotFound" in msg or "404" in msg:
                raise ConnectorValidationError(
                    f"Azure Blob: container not found: {msg[:300]}"
                ) from exc
            raise UnexpectedValidationError(
                f"Azure Blob validation failed ({code}): {msg[:300]}"
            ) from exc

    # ------------------------------------------------------------------
    # Checkpoint helpers
    # ------------------------------------------------------------------

    def build_dummy_checkpoint(self) -> AzureBlobCheckpoint:
        return AzureBlobCheckpoint(has_more=True, etags={})

    def validate_checkpoint_json(self, checkpoint_json: str) -> AzureBlobCheckpoint:
        try:
            return AzureBlobCheckpoint.model_validate_json(checkpoint_json)
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
        if not isinstance(checkpoint, AzureBlobCheckpoint):
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
        """Yield batches of slim documents for prune / permission sync."""
        if self._container_client is None:
            raise ConnectorMissingCredentialError("Azure Blob")

        batch: list[SlimDocument] = []
        try:
            for blob_props in self._container_client.list_blobs(name_starts_with=self.prefix or None):
                name = blob_props.name
                if not _has_supported_extension(name, self.allow_images):
                    continue
                if callback:
                    callback(name, name)
                batch.append(SlimDocument(id=name))
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []
        except Exception as exc:
            raise UnexpectedValidationError(
                f"Azure Blob prune listing failed: {exc}"
            ) from exc

        if batch:
            yield batch

    # ------------------------------------------------------------------
    # Internal document iteration
    # ------------------------------------------------------------------

    def _iter_documents(
        self,
        checkpoint: AzureBlobCheckpoint | None = None,
        since_epoch: float | None = None,
    ):
        from common.data_source.models import Document

        if self._container_client is None:
            raise ConnectorMissingCredentialError("Azure Blob")

        etags: dict[str, str] = {}
        if checkpoint and checkpoint.etags:
            etags = dict(checkpoint.etags)

        batch: list[Document] = []

        try:
            blobs = list(self._container_client.list_blobs(name_starts_with=self.prefix or None))
        except Exception as exc:
            raise UnexpectedValidationError(
                f"Azure Blob listing failed: {exc}"
            ) from exc

        for blob_props in blobs:
            name: str = blob_props.name

            if not _has_supported_extension(name, self.allow_images):
                continue

            # ETag fingerprint check — skip blobs whose content hasn't
            # changed.  Use the raw ETag (always present) as the hash;
            # Azure updates it on every write so it's a reliable change
            # signal without downloading.
            current_etag = (blob_props.etag or "").strip('"')
            if current_etag and etags.get(name) == current_etag:
                continue

            # Time-window filter — for poll_source callers.
            last_modified: datetime | None = blob_props.last_modified
            if since_epoch and last_modified:
                if last_modified.timestamp() < since_epoch:
                    continue

            # Download blob content
            try:
                blob_client = self._container_client.get_blob_client(name)
                data = blob_client.download_blob().readall()
            except Exception as exc:
                logger.warning("Azure Blob: failed to download %s: %s", name, exc)
                continue

            doc_updated_at = (
                last_modified.astimezone(timezone.utc)
                if last_modified
                else datetime.now(timezone.utc)
            )

            ext = _extension(name)
            doc = Document(
                id=name,
                source="azure_blob",
                semantic_identifier=name,
                extension=ext,
                blob=data,
                doc_updated_at=doc_updated_at,
                size_bytes=len(data),
                metadata={
                    "container": _container_name(self._container_client),
                    "etag": current_etag,
                    "prefix": self.prefix,
                },
            )
            batch.append(doc)
            if current_etag:
                etags[name] = current_etag

            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

        if checkpoint is not None:
            checkpoint.etags = etags
            checkpoint.has_more = False


# ----------------------------------------------------------------------
# Module-level helpers
# ----------------------------------------------------------------------

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


def _container_name(client: Any) -> str:
    """Extract the container name from a ContainerClient without
    importing the Azure SDK at module level."""
    try:
        return client.container_name
    except AttributeError:
        return ""

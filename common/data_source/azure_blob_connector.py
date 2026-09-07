"""Azure Blob Storage data-source connector.

Ingests blobs from a user's Azure container into a RAGFlow knowledge
base.  This is distinct from RAGFlow's own Azure storage *backend*
(``rag/utils/azure_sas_conn.py``, ``rag/utils/azure_spn_conn.py``),
which stores RAGFlow's own files.

Auth supports three mutually exclusive modes, selected explicitly by the
caller-supplied ``auth_mode`` (the UI hides the other modes' fields but
does not clear them, so we must not guess from whichever field happens to
be populated). When ``auth_mode`` is absent (older configs / direct API
callers) we fall back to field precedence:

  1. **Connection string** — ``connection_string`` credential; one line,
     everything embedded.  Good for dev / testing.
  2. **Account key** — ``account_name`` + ``account_key``; maps to the
     same underlying SAS-less AccountKey credential.
  3. **SAS token** — ``container_url`` + ``sas_token``; the shape that
     ``RAGFlowAzureSasBlob`` already uses.

Incremental runs are scoped by the poll time window
(``since_epoch`` < last-modified <= ``until_epoch``).
Each blob's ETag is also emitted as the document fingerprint, which the
indexing pipeline persists as ``content_hash`` so unchanged blobs are not
re-embedded. The connector itself keeps no cross-run ETag state.
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
    """Checkpoint marker for the Azure Blob connector.

    The connector keeps no cross-run state of its own: a single
    ``load_from_checkpoint`` pass lists the container once and sets
    ``has_more=False``. Incremental scoping comes from the poll time
    window, and per-blob change detection from the document fingerprint
    (ETag) the pipeline persists as ``content_hash``.
    """


class AzureBlobConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """Azure Blob Storage data-source connector.

    Authenticates with one of three credential modes (connection string,
    account key, or SAS token), chosen by ``auth_mode``, and enumerates
    blobs in the configured container under an optional prefix. Each blob's
    ETag is surfaced as the document fingerprint so the pipeline can skip
    re-embedding unchanged blobs across runs.
    """

    def __init__(
        self,
        batch_size: int = INDEX_BATCH_SIZE,
        prefix: str | None = None,
        allow_images: bool = False,
        auth_mode: str | None = None,
    ) -> None:
        self.batch_size = batch_size
        self.prefix = (prefix or "").lstrip("/")
        self.allow_images = allow_images
        # Explicitly selected credential mode: "connection_string",
        # "account_key", or "sas_token". Empty falls back to precedence.
        self.auth_mode = (auth_mode or "").strip().lower()
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

        # Honor the explicitly selected auth mode. The UI hides inactive
        # credential fields but does not clear them, so a user who fills one
        # mode and then switches can leave stale values behind; selecting by
        # field precedence would then authenticate with the wrong mode.
        # Fall back to precedence only when no auth_mode was supplied.
        mode = self.auth_mode
        if not mode:
            if conn_str:
                mode = "connection_string"
            elif account_name and account_key:
                mode = "account_key"
            elif container_url and sas_token:
                mode = "sas_token"

        try:
            if mode == "connection_string":
                if not conn_str:
                    raise ConnectorMissingCredentialError(
                        "Azure Blob: connection_string is required for the connection_string auth mode"
                    )
                if not container_name:
                    raise ConnectorMissingCredentialError(
                        "Azure Blob: container_name is required together with connection_string"
                    )
                svc = BlobServiceClient.from_connection_string(conn_str)
                self._container_client = svc.get_container_client(container_name)
            elif mode == "account_key":
                if not (account_name and account_key):
                    raise ConnectorMissingCredentialError(
                        "Azure Blob: account_name and account_key are required for the account_key auth mode"
                    )
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
            elif mode == "sas_token":
                if not (container_url and sas_token):
                    raise ConnectorMissingCredentialError(
                        "Azure Blob: container_url and sas_token are required for the sas_token auth mode"
                    )
                # mirrors RAGFlowAzureSasBlob; strip a leading "?" so we
                # never produce a double-"?" that breaks SAS auth.
                normalized_sas = str(sas_token).lstrip("?")
                full_url = f"{container_url}?{normalized_sas}"
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
        return AzureBlobCheckpoint(has_more=True)

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
        return self._iter_documents(since_epoch=start, until_epoch=end)

    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> Any:
        if not isinstance(checkpoint, AzureBlobCheckpoint):
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
        until_epoch: float | None = None,
    ):
        from common.data_source.models import Document

        if self._container_client is None:
            raise ConnectorMissingCredentialError("Azure Blob")

        batch: list[Document] = []

        try:
            for blob_props in self._container_client.list_blobs(
                name_starts_with=self.prefix or None
            ):
                name: str = blob_props.name

                if not _has_supported_extension(name, self.allow_images):
                    continue

                # Raw ETag (always present); Azure updates it on every
                # write. Emitted below as the document fingerprint so the
                # pipeline persists it as content_hash and skips re-embedding
                # unchanged blobs across runs.
                current_etag = (blob_props.etag or "").strip('"')

                # Time-window filter: strict lower bound, inclusive upper
                # bound (``since_epoch`` < last-modified <= ``until_epoch``).
                # Excluding last-modified == since_epoch (the prior run's
                # watermark, which that run already yielded) avoids stable
                # duplicate re-fetches on the boundary — matching the
                # Salesforce connector's ``> since``. Enforcing the upper
                # bound keeps blobs modified mid-run from leaking into this
                # window; they're picked up by the next run (whose lower bound
                # is this run's upper bound), so an update can never fall into
                # a gap between windows.
                last_modified: datetime | None = blob_props.last_modified
                if last_modified:
                    ts = last_modified.timestamp()
                    if since_epoch and ts <= since_epoch:
                        continue
                    if until_epoch and ts > until_epoch:
                        continue

                # Download blob content. A blob that was deleted between the
                # listing and this fetch is genuinely gone — skip it. Any
                # other failure (throttling, transient 5xx, network) must
                # abort the run: the sync framework advances its watermark
                # from successfully yielded docs, so silently skipping a
                # transiently-failed blob while newer blobs succeed would
                # move the watermark past it and drop it permanently.
                try:
                    blob_client = self._container_client.get_blob_client(name)
                    data = blob_client.download_blob().readall()
                except Exception as exc:
                    if _is_blob_gone(exc):
                        logger.warning(
                            "Azure Blob: %s vanished between listing and fetch; skipping",
                            name,
                        )
                        continue
                    raise UnexpectedValidationError(
                        f"Azure Blob: failed to download {name}: {exc}"
                    ) from exc

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
                    fingerprint=current_etag or None,
                    metadata={
                        "container": _container_name(self._container_client),
                        "etag": current_etag,
                        "prefix": self.prefix,
                    },
                )
                batch.append(doc)

                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []
        except UnexpectedValidationError:
            raise
        except Exception as exc:
            raise UnexpectedValidationError(
                f"Azure Blob listing failed: {exc}"
            ) from exc

        if batch:
            yield batch

        if checkpoint is not None:
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


def _is_blob_gone(exc: Exception) -> bool:
    """True when a download failed because the blob no longer exists.

    Azure raises ``ResourceNotFoundError`` (status 404, error code
    ``BlobNotFound``) when a blob listed moments earlier has since been
    deleted. That is not data loss — the blob is gone — so it is safe to
    skip. Detected by attribute and string so we need not import the Azure
    exception type at module load.
    """
    if getattr(exc, "status_code", None) == 404:
        return True
    code = getattr(exc, "error_code", "") or ""
    if "BlobNotFound" in str(code):
        return True
    msg = str(exc)
    return "BlobNotFound" in msg or "ResourceNotFound" in msg


def _container_name(client: Any) -> str:
    """Extract the container name from a ContainerClient without
    importing the Azure SDK at module level."""
    try:
        return client.container_name
    except AttributeError:
        return ""

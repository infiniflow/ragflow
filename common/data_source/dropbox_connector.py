"""Dropbox connector"""

import logging
from datetime import timezone
from typing import Any

from dropbox import Dropbox
from dropbox.exceptions import ApiError, AuthError
from dropbox.files import FileMetadata, FolderMetadata

from common.data_source.config import INDEX_BATCH_SIZE, DocumentSource
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    InsufficientPermissionsError,
)
from common.data_source.interfaces import LoadConnector, PollConnector, SecondsSinceUnixEpoch, SlimConnectorWithPermSync
from common.data_source.models import Document, GenerateDocumentsOutput, GenerateSlimDocumentOutput, SlimDocument
from common.data_source.utils import get_file_ext

logger = logging.getLogger(__name__)


class DropboxConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    """Dropbox connector for accessing Dropbox files and folders"""

    def __init__(self, batch_size: int = INDEX_BATCH_SIZE) -> None:
        self.batch_size = batch_size
        self.dropbox_client: Dropbox | None = None

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load Dropbox credentials"""
        access_token = credentials.get("dropbox_access_token")
        if not access_token:
            raise ConnectorMissingCredentialError("Dropbox access token is required")

        self.dropbox_client = Dropbox(access_token)
        return None

    def validate_connector_settings(self) -> None:
        """Validate Dropbox connector settings"""
        if self.dropbox_client is None:
            raise ConnectorMissingCredentialError("Dropbox")

        try:
            self.dropbox_client.files_list_folder(path="", limit=1)
        except AuthError as e:
            logger.exception("[Dropbox]: Failed to validate Dropbox credentials")
            raise ConnectorValidationError(f"Dropbox credential is invalid: {e}")
        except ApiError as e:
            if e.error is not None and "insufficient_permissions" in str(e.error).lower():
                raise InsufficientPermissionsError("Your Dropbox token does not have sufficient permissions.")
            raise ConnectorValidationError(f"Unexpected Dropbox error during validation: {e.user_message_text or e}")
        except Exception as e:
            raise ConnectorValidationError(f"Unexpected error during Dropbox settings validation: {e}")

    def _download_file(self, path: str) -> bytes:
        """Download a single file from Dropbox."""
        if self.dropbox_client is None:
            raise ConnectorMissingCredentialError("Dropbox")
        _, resp = self.dropbox_client.files_download(path)
        return resp.content

    def _get_shared_link(self, path: str) -> str:
        """Create a shared link for a file in Dropbox."""
        if self.dropbox_client is None:
            raise ConnectorMissingCredentialError("Dropbox")

        try:
            shared_links = self.dropbox_client.sharing_list_shared_links(path=path)
            if shared_links.links:
                return shared_links.links[0].url

            link_metadata = self.dropbox_client.sharing_create_shared_link_with_settings(path)
            return link_metadata.url
        except ApiError as err:
            logger.exception(f"[Dropbox]: Failed to create a shared link for {path}: {err}")
            return ""

    def _yield_files_recursive(
        self,
        path: str,
        start: SecondsSinceUnixEpoch | None,
        end: SecondsSinceUnixEpoch | None,
    ) -> GenerateDocumentsOutput:
        """Yield files in batches from a specified Dropbox folder, including subfolders."""
        if self.dropbox_client is None:
            raise ConnectorMissingCredentialError("Dropbox")

        all_files: list[FileMetadata] = []
        self._collect_file_entries_recursive(path, start, end, all_files)

        # Count filename occurrences
        filename_counts: dict[str, int] = {}
        for entry in all_files:
            filename_counts[entry.name] = filename_counts.get(entry.name, 0) + 1

        # Process files in batches
        batch: list[Document] = []
        for entry in all_files:
            try:
                downloaded_file = self._download_file(entry.path_display)
            except Exception:
                logger.exception(f"[Dropbox]: Error downloading file {entry.path_display}")
                continue

            batch.append(
                Document(
                    id=f"dropbox:{entry.id}",
                    blob=downloaded_file,
                    source=DocumentSource.DROPBOX,
                    semantic_identifier=self._get_semantic_identifier(entry, filename_counts),
                    extension=get_file_ext(entry.name),
                    doc_updated_at=self._normalize_modified_time(entry.client_modified),
                    size_bytes=entry.size if getattr(entry, "size", None) is not None else len(downloaded_file),
                )
            )

            if len(batch) == self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    def _collect_file_entries_recursive(
        self,
        path: str,
        start: SecondsSinceUnixEpoch | None,
        end: SecondsSinceUnixEpoch | None,
        all_files: list[FileMetadata],
    ) -> None:
        """Recursively collect all files matching time criteria."""
        if self.dropbox_client is None:
            raise ConnectorMissingCredentialError("Dropbox")

        result = self.dropbox_client.files_list_folder(
            path,
            recursive=False,
            include_non_downloadable_files=False,
        )

        while True:
            for entry in result.entries:
                if isinstance(entry, FileMetadata):
                    time_as_seconds = self._normalize_modified_time(entry.client_modified).timestamp()
                    if start is not None and time_as_seconds <= start:
                        continue
                    if end is not None and time_as_seconds > end:
                        continue

                    all_files.append(entry)

                elif isinstance(entry, FolderMetadata):
                    self._collect_file_entries_recursive(entry.path_lower, start, end, all_files)

            if not result.has_more:
                break

            result = self.dropbox_client.files_list_folder_continue(result.cursor)

    def _normalize_modified_time(self, modified_time):
        if modified_time.tzinfo is None:
            return modified_time.replace(tzinfo=timezone.utc)
        return modified_time.astimezone(timezone.utc)

    def _get_semantic_identifier(self, entry: FileMetadata, filename_counts: dict[str, int]) -> str:
        if filename_counts.get(entry.name, 0) <= 1:
            return entry.name

        relative_path = entry.path_display.lstrip("/")
        return relative_path.replace("/", " / ") if relative_path else entry.name

    def retrieve_all_slim_docs_perm_sync(
        self,
        callback: Any = None,
    ) -> GenerateSlimDocumentOutput:
        del callback

        if self.dropbox_client is None:
            raise ConnectorMissingCredentialError("Dropbox")

        all_files: list[FileMetadata] = []
        self._collect_file_entries_recursive("", None, None, all_files)

        batch: list[SlimDocument] = []
        for entry in all_files:
            batch.append(SlimDocument(id=f"dropbox:{entry.id}"))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> GenerateDocumentsOutput:
        """Poll Dropbox for recent file changes"""
        if self.dropbox_client is None:
            raise ConnectorMissingCredentialError("Dropbox")

        for batch in self._yield_files_recursive("", start, end):
            yield batch

    def load_from_state(self) -> GenerateDocumentsOutput:
        """Load files from Dropbox state"""
        return self._yield_files_recursive("", None, None)


if __name__ == "__main__":
    import os

    logging.basicConfig(level=logging.DEBUG)
    connector = DropboxConnector()
    connector.load_credentials({"dropbox_access_token": os.environ.get("DROPBOX_ACCESS_TOKEN")})
    connector.validate_connector_settings()
    document_batches = connector.load_from_state()
    try:
        first_batch = next(document_batches)
        print(f"Loaded {len(first_batch)} documents in first batch.")
        for doc in first_batch:
            print(f"- {doc.semantic_identifier} ({doc.size_bytes} bytes)")
    except StopIteration:
        print("No documents available in Dropbox.")

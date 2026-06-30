"""Feishu (Lark) Drive connector.

Scope: this connector treats Feishu Drive as a cloud file store. It walks a
Drive folder tree and downloads *uploaded files* (``type == "file"``), handing
the raw bytes to RAGFlow's parser. Feishu-native documents (docx/sheet/bitable)
are NOT downloadable binaries and are intentionally skipped here; reading their
content requires the docx ``raw_content`` API and is tracked as a follow-up.

Auth: an internal (self-built) app's ``app_id`` + ``app_secret``. The lark-oapi
client manages the ``tenant_access_token`` automatically. Note that a tenant
token can only see files explicitly shared with the app, so the target folder
must be shared with the app for it to appear.

API references (open.feishu.cn):
- List files:  GET /open-apis/drive/v1/files
- Download:    GET /open-apis/drive/v1/files/{file_token}/download
"""

import logging
from datetime import datetime, timezone
from typing import Any

import lark_oapi as lark
from lark_oapi.api.drive.v1 import DownloadFileRequest, ListFileRequest

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

# Feishu Drive entry types. Only FILE is a downloadable binary; the rest are
# native cloud documents handled by other APIs.
_FEISHU_FILE_TYPE = "file"
_FEISHU_FOLDER_TYPE = "folder"
# Permission errors surface as these Feishu server codes.
_FEISHU_FORBIDDEN_CODES = {1061002, 1062002, 1061004}


class FeishuDriveConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    """Feishu Drive connector for downloading uploaded files from a folder tree.

    Scoped to Drive *files* only (hence ``FEISHU_DRIVE``, not ``FEISHU``); native
    docs/wiki are separate surfaces that would warrant their own connectors.
    """

    def __init__(self, folder_token: str = "", batch_size: int = INDEX_BATCH_SIZE) -> None:
        # Empty folder_token lists the app's Drive root.
        self.folder_token = folder_token or ""
        self.batch_size = batch_size
        self.client: lark.Client | None = None

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Build a lark-oapi client from app_id / app_secret.

        ``feishu_domain`` selects the API host: "feishu" (open.feishu.cn,
        default) for Feishu/China apps, or "lark" (open.larksuite.com) for Lark
        international apps. A full URL is also accepted.
        """
        app_id = credentials.get("feishu_app_id")
        app_secret = credentials.get("feishu_app_secret")
        if not app_id or not app_secret:
            raise ConnectorMissingCredentialError("Feishu app_id and app_secret are required")

        # A per-connector folder scope may also arrive via credentials.
        folder_token = credentials.get("feishu_folder_token")
        if folder_token:
            self.folder_token = folder_token

        # "feishu" / "lark" map to the known hosts; anything else is treated as a full URL.
        domain = (credentials.get("feishu_domain") or "feishu").strip()
        domain_url = {"feishu": lark.FEISHU_DOMAIN, "lark": lark.LARK_DOMAIN}.get(domain.lower(), domain)

        self.client = lark.Client.builder().app_id(app_id).app_secret(app_secret).domain(domain_url).build()
        return None

    def validate_connector_settings(self) -> None:
        """Validate credentials by listing the configured folder once."""
        if self.client is None:
            raise ConnectorMissingCredentialError("Feishu")

        try:
            self._list_folder_page(self.folder_token, page_token=None, page_size=1)
        except InsufficientPermissionsError:
            raise
        except ConnectorValidationError:
            raise
        except Exception as e:
            raise ConnectorValidationError(f"Unexpected error during Feishu settings validation: {e}")

    def _list_folder_page(self, folder_token: str, page_token: str | None, page_size: int = 200):
        """Call drive.v1.file.list for a single page and return its response data."""
        if self.client is None:
            raise ConnectorMissingCredentialError("Feishu")

        builder = (
            ListFileRequest.builder()
            .page_size(page_size)
            .order_by("EditedTime")
            .direction("DESC")
        )
        if folder_token:
            builder = builder.folder_token(folder_token)
        if page_token:
            builder = builder.page_token(page_token)

        resp = self.client.drive.v1.file.list(builder.build())
        if not resp.success():
            if resp.code in _FEISHU_FORBIDDEN_CODES:
                raise InsufficientPermissionsError(
                    f"Feishu app lacks permission for folder '{folder_token or 'root'}': "
                    f"{resp.code} {resp.msg}. Share the folder with the app and grant drive:drive:readonly."
                )
            raise ConnectorValidationError(f"Feishu list files failed: {resp.code} {resp.msg} log_id={resp.get_log_id()}")
        return resp.data

    def _download_file(self, file_token: str) -> bytes:
        """Download a single uploaded file's bytes."""
        if self.client is None:
            raise ConnectorMissingCredentialError("Feishu")

        resp = self.client.drive.v1.file.download(DownloadFileRequest.builder().file_token(file_token).build())
        if not resp.success():
            raise ConnectorValidationError(
                f"Feishu download failed for {file_token}: {resp.code} {resp.msg} log_id={resp.get_log_id()}"
            )
        return resp.file.read()

    def _collect_file_entries_recursive(
        self,
        folder_token: str,
        start: SecondsSinceUnixEpoch | None,
        end: SecondsSinceUnixEpoch | None,
        all_files: list[dict[str, Any]],
    ) -> None:
        """Recursively collect downloadable files matching the time window."""
        page_token: str | None = None
        while True:
            data = self._list_folder_page(folder_token, page_token=page_token)
            for entry in getattr(data, "files", None) or []:
                entry_type = getattr(entry, "type", None)
                if entry_type == _FEISHU_FILE_TYPE:
                    modified = self._normalize_modified_time(getattr(entry, "modified_time", None))
                    time_as_seconds = modified.timestamp()
                    if start is not None and time_as_seconds <= start:
                        continue
                    if end is not None and time_as_seconds > end:
                        continue
                    all_files.append(
                        {
                            "token": getattr(entry, "token", None),
                            "name": getattr(entry, "name", "") or "",
                            "modified": modified,
                            "url": getattr(entry, "url", "") or "",
                            "parent": getattr(entry, "parent_token", "") or "",
                        }
                    )
                elif entry_type == _FEISHU_FOLDER_TYPE:
                    child_token = getattr(entry, "token", None)
                    if child_token:
                        self._collect_file_entries_recursive(child_token, start, end, all_files)
                # native docs (docx/sheet/bitable/...) are skipped on purpose.

            if not getattr(data, "has_more", False):
                break
            page_token = getattr(data, "next_page_token", None)
            if not page_token:
                break

    def _yield_files_recursive(
        self,
        folder_token: str,
        start: SecondsSinceUnixEpoch | None,
        end: SecondsSinceUnixEpoch | None,
    ) -> GenerateDocumentsOutput:
        """Yield downloaded files in batches from the folder tree."""
        if self.client is None:
            raise ConnectorMissingCredentialError("Feishu")

        all_files: list[dict[str, Any]] = []
        self._collect_file_entries_recursive(folder_token, start, end, all_files)

        filename_counts: dict[str, int] = {}
        for entry in all_files:
            filename_counts[entry["name"]] = filename_counts.get(entry["name"], 0) + 1

        batch: list[Document] = []
        for entry in all_files:
            token = entry["token"]
            if not token:
                continue
            try:
                downloaded_file = self._download_file(token)
            except Exception:
                logger.exception(f"[Feishu]: Error downloading file {entry['name']} ({token})")
                continue

            batch.append(
                Document(
                    id=f"feishu_drive:{token}",
                    blob=downloaded_file,
                    source=DocumentSource.FEISHU_DRIVE,
                    semantic_identifier=self._get_semantic_identifier(entry, filename_counts),
                    extension=get_file_ext(entry["name"]),
                    doc_updated_at=entry["modified"],
                    size_bytes=len(downloaded_file),
                )
            )

            if len(batch) == self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    def _normalize_modified_time(self, modified_time: Any) -> datetime:
        """Feishu returns modified_time as seconds-since-epoch (str or int)."""
        if modified_time is None:
            return datetime.now(timezone.utc)
        try:
            return datetime.fromtimestamp(int(modified_time), tz=timezone.utc)
        except (ValueError, TypeError):
            return datetime.now(timezone.utc)

    def _get_semantic_identifier(self, entry: dict[str, Any], filename_counts: dict[str, int]) -> str:
        name = entry["name"]
        if filename_counts.get(name, 0) <= 1:
            return name
        # Disambiguate duplicate filenames by appending the parent folder token.
        parent = entry.get("parent", "")
        return f"{parent} / {name}" if parent else name

    def retrieve_all_slim_docs_perm_sync(self, callback: Any = None) -> GenerateSlimDocumentOutput:
        del callback
        if self.client is None:
            raise ConnectorMissingCredentialError("Feishu")

        all_files: list[dict[str, Any]] = []
        self._collect_file_entries_recursive(self.folder_token, None, None, all_files)

        batch: list[SlimDocument] = []
        for entry in all_files:
            if not entry["token"]:
                continue
            batch.append(SlimDocument(id=f"feishu_drive:{entry['token']}"))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> GenerateDocumentsOutput:
        """Poll Feishu Drive for files changed within the window."""
        if self.client is None:
            raise ConnectorMissingCredentialError("Feishu")
        for batch in self._yield_files_recursive(self.folder_token, start, end):
            yield batch

    def load_from_state(self) -> GenerateDocumentsOutput:
        """Load all downloadable files from the configured folder tree."""
        return self._yield_files_recursive(self.folder_token, None, None)


if __name__ == "__main__":
    import os

    logging.basicConfig(level=logging.DEBUG)
    connector = FeishuDriveConnector(folder_token=os.environ.get("FEISHU_FOLDER_TOKEN", ""))
    connector.load_credentials(
        {
            "feishu_app_id": os.environ.get("FEISHU_APP_ID"),
            "feishu_app_secret": os.environ.get("FEISHU_APP_SECRET"),
            "feishu_domain": os.environ.get("FEISHU_DOMAIN", "feishu"),
        }
    )
    connector.validate_connector_settings()
    document_batches = connector.load_from_state()
    try:
        first_batch = next(document_batches)
        print(f"Loaded {len(first_batch)} documents in first batch.")
        for doc in first_batch:
            print(f"- {doc.semantic_identifier} ({doc.size_bytes} bytes)")
    except StopIteration:
        print("No downloadable files available in Feishu Drive.")

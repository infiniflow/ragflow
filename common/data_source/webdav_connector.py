"""WebDAV connector"""
import logging
import os
from datetime import datetime, timezone
from typing import Any, Optional

from webdav4.client import Client as WebDAVClient

from common.data_source.utils import (
    get_file_ext,
)
from common.data_source.config import DocumentSource, INDEX_BATCH_SIZE, BLOB_STORAGE_SIZE_THRESHOLD
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError
)
from common.data_source.interfaces import LoadConnector, PollConnector
from common.data_source.models import Document, SecondsSinceUnixEpoch, GenerateDocumentsOutput


class WebDAVConnector(LoadConnector, PollConnector):
    """WebDAV connector for syncing files from WebDAV servers"""

    def __init__(
        self,
        base_url: str,
        remote_path: str = "/",
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        """Initialize WebDAV connector
        
        Args:
            base_url: Base URL of the WebDAV server (e.g., "https://webdav.example.com")
            remote_path: Remote path to sync from (default: "/")
            batch_size: Number of documents per batch
        """
        self.base_url = base_url.rstrip("/")
        if not remote_path:
            remote_path = "/"
        if not remote_path.startswith("/"):
            remote_path = f"/{remote_path}"
        if remote_path.endswith("/") and remote_path != "/":
            remote_path = remote_path.rstrip("/")
        self.remote_path = remote_path
        self.batch_size = batch_size
        self.client: Optional[WebDAVClient] = None
        self._allow_images: bool | None = None
        self.size_threshold: int | None = BLOB_STORAGE_SIZE_THRESHOLD

    def set_allow_images(self, allow_images: bool) -> None:
        """Set whether to process images"""
        logging.info(f"Setting allow_images to {allow_images}.")
        self._allow_images = allow_images

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load credentials and initialize WebDAV client
        
        Args:
            credentials: Dictionary containing 'username' and 'password'
        
        Returns:
            None
        
        Raises:
            ConnectorMissingCredentialError: If required credentials are missing
        """
        logging.debug(f"Loading credentials for WebDAV server {self.base_url}")

        username = credentials.get("username")
        password = credentials.get("password")
        
        if not username or not password:
            raise ConnectorMissingCredentialError(
                "WebDAV requires 'username' and 'password' credentials"
            )

        try:
            # Initialize WebDAV client
            self.client = WebDAVClient(
                base_url=self.base_url,
                auth=(username, password)
            )
        except Exception as e:
            logging.error(f"Failed to connect to WebDAV server: {e}")
            raise ConnectorMissingCredentialError(
                f"Failed to authenticate with WebDAV server: {e}"
            )

        return None

    def _list_files_recursive(
        self, 
        path: str,
        start: datetime,
        end: datetime,
    ) -> list[tuple[str, dict]]:
        """Recursively list all files in the given path
        
        Args:
            path: Path to list files from
            start: Start datetime for filtering
            end: End datetime for filtering
            
        Returns:
            List of tuples containing (file_path, file_info)
        """
        if self.client is None:
            raise ConnectorMissingCredentialError("WebDAV client not initialized")

        files = []
        
        try:
            logging.debug(f"Listing directory: {path}")
            for item in self.client.ls(path, detail=True):
                item_path = item['name']
            
                if item_path == path or item_path == path + '/':
                    continue
                
                logging.debug(f"Found item: {item_path}, type: {item.get('type')}")

                if item.get('type') == 'directory':
                    try:
                        files.extend(self._list_files_recursive(item_path, start, end))
                    except Exception as e:
                        logging.error(f"Error recursing into directory {item_path}: {e}")
                        continue
                else:
                    try:
                        modified_time = item.get('modified')
                        if modified_time:
                            if isinstance(modified_time, datetime):
                                modified = modified_time
                                if modified.tzinfo is None:
                                    modified = modified.replace(tzinfo=timezone.utc)
                            elif isinstance(modified_time, str):
                                try:
                                    modified = datetime.strptime(modified_time, '%a, %d %b %Y %H:%M:%S %Z')
                                    modified = modified.replace(tzinfo=timezone.utc)
                                except (ValueError, TypeError):
                                    try:
                                        modified = datetime.fromisoformat(modified_time.replace('Z', '+00:00'))
                                    except (ValueError, TypeError):
                                        logging.warning(f"Could not parse modified time for {item_path}: {modified_time}")
                                        modified = datetime.now(timezone.utc)
                            else:
                                modified = datetime.now(timezone.utc)
                        else:
                            modified = datetime.now(timezone.utc)
                        

                        logging.debug(f"File {item_path}: modified={modified}, start={start}, end={end}, include={start < modified <= end}")
                        if start < modified <= end:
                            files.append((item_path, item))
                        else:
                            logging.debug(f"File {item_path} filtered out by time range")
                    except Exception as e:
                        logging.error(f"Error processing file {item_path}: {e}")
                        continue
                        
        except Exception as e:
            logging.error(f"Error listing directory {path}: {e}")
            
        return files

    def _yield_webdav_documents(
        self,
        start: datetime,
        end: datetime,
    ) -> GenerateDocumentsOutput:
        """Generate documents from WebDAV server
        
        Args:
            start: Start datetime for filtering
            end: End datetime for filtering
            
        Yields:
            Batches of documents
        """
        if self.client is None:
            raise ConnectorMissingCredentialError("WebDAV client not initialized")

        logging.info(f"Searching for files in {self.remote_path} between {start} and {end}")
        files = self._list_files_recursive(self.remote_path, start, end)
        logging.info(f"Found {len(files)} files matching time criteria")
        
        filename_counts: dict[str, int] = {}
        for file_path, _ in files:
            file_name = os.path.basename(file_path)
            filename_counts[file_name] = filename_counts.get(file_name, 0) + 1
        
        batch: list[Document] = []
        for file_path, file_info in files:
            file_name = os.path.basename(file_path)
            
            size_bytes = file_info.get('size', 0)
            if (
                self.size_threshold is not None
                and isinstance(size_bytes, int)
                and size_bytes > self.size_threshold
            ):
                logging.warning(
                    f"{file_name} exceeds size threshold of {self.size_threshold}. Skipping."
                )
                continue
            
            try:
                logging.debug(f"Downloading file: {file_path}")
                from io import BytesIO
                buffer = BytesIO()
                self.client.download_fileobj(file_path, buffer)
                blob = buffer.getvalue()
                
                if blob is None or len(blob) == 0:
                    logging.warning(f"Downloaded content is empty for {file_path}")
                    continue

                modified_time = file_info.get('modified')
                if modified_time:
                    if isinstance(modified_time, datetime):
                        modified = modified_time
                        if modified.tzinfo is None:
                            modified = modified.replace(tzinfo=timezone.utc)
                    elif isinstance(modified_time, str):
                        try:
                            modified = datetime.strptime(modified_time, '%a, %d %b %Y %H:%M:%S %Z')
                            modified = modified.replace(tzinfo=timezone.utc)
                        except (ValueError, TypeError):
                            try:
                                modified = datetime.fromisoformat(modified_time.replace('Z', '+00:00'))
                            except (ValueError, TypeError):
                                logging.warning(f"Could not parse modified time for {file_path}: {modified_time}")
                                modified = datetime.now(timezone.utc)
                    else:
                        modified = datetime.now(timezone.utc)
                else:
                    modified = datetime.now(timezone.utc)

                if filename_counts.get(file_name, 0) > 1:
                    relative_path = file_path
                    if file_path.startswith(self.remote_path):
                        relative_path = file_path[len(self.remote_path):]
                    if relative_path.startswith('/'):
                        relative_path = relative_path[1:]
                    semantic_id = relative_path.replace('/', ' / ') if relative_path else file_name
                else:
                    semantic_id = file_name

                batch.append(
                    Document(
                        id=f"webdav:{self.base_url}:{file_path}",
                        blob=blob,
                        source=DocumentSource.WEBDAV,
                        semantic_identifier=semantic_id,
                        extension=get_file_ext(file_name),
                        doc_updated_at=modified,
                        size_bytes=size_bytes if size_bytes else 0
                    )
                )
                
                if len(batch) == self.batch_size:
                    yield batch
                    batch = []

            except Exception as e:
                logging.exception(f"Error downloading file {file_path}: {e}")
        
        if batch:
            yield batch

    def load_from_state(self) -> GenerateDocumentsOutput:
        """Load all documents from WebDAV server
        
        Yields:
            Batches of documents
        """
        logging.debug(f"Loading documents from WebDAV server {self.base_url}")
        return self._yield_webdav_documents(
            start=datetime(1970, 1, 1, tzinfo=timezone.utc),
            end=datetime.now(timezone.utc),
        )

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> GenerateDocumentsOutput:
        """Poll WebDAV server for updated documents
        
        Args:
            start: Start timestamp (seconds since Unix epoch)
            end: End timestamp (seconds since Unix epoch)
            
        Yields:
            Batches of documents
        """
        if self.client is None:
            raise ConnectorMissingCredentialError("WebDAV client not initialized")

        start_datetime = datetime.fromtimestamp(start, tz=timezone.utc)
        end_datetime = datetime.fromtimestamp(end, tz=timezone.utc)

        for batch in self._yield_webdav_documents(start_datetime, end_datetime):
            yield batch

    def validate_connector_settings(self) -> None:
        """Validate WebDAV connector settings.

        Validation should exercise the same code-paths used by the connector
        (directory listing / PROPFIND), avoiding exists() which may probe with
        methods that differ across servers.
        """
        if self.client is None:
            raise ConnectorMissingCredentialError("WebDAV credentials not loaded.")

        if not self.base_url:
            raise ConnectorValidationError("No base URL was provided in connector settings.")

        # Normalize directory path: for collections, many servers behave better with trailing '/'
        test_path = self.remote_path or "/"
        if not test_path.startswith("/"):
            test_path = f"/{test_path}"
        if test_path != "/" and not test_path.endswith("/"):
            test_path = f"{test_path}/"

        try:
            # Use the same behavior as real sync: list directory with details (PROPFIND)
            self.client.ls(test_path, detail=True)

        except Exception as e:
            # Prefer structured status codes if present on the exception/response
            status = None
            for attr in ("status_code", "code"):
                v = getattr(e, attr, None)
                if isinstance(v, int):
                    status = v
                    break
            if status is None:
                resp = getattr(e, "response", None)
                v = getattr(resp, "status_code", None)
                if isinstance(v, int):
                    status = v

            # If we can classify by status code, do it
            if status == 401:
                raise CredentialExpiredError("WebDAV credentials appear invalid or expired.")
            if status == 403:
                raise InsufficientPermissionsError(
                    f"Insufficient permissions to access path '{self.remote_path}' on WebDAV server."
                )
            if status == 404:
                raise ConnectorValidationError(
                    f"Remote path '{self.remote_path}' does not exist on WebDAV server."
                )

            # Fallback: avoid brittle substring matching that caused false positives.
            # Provide the original exception for diagnosis.
            raise ConnectorValidationError(
                f"WebDAV validation failed for path '{test_path}': {repr(e)}"
            )



if __name__ == "__main__":
    credentials_dict = {
        "username": os.environ.get("WEBDAV_USERNAME"),
        "password": os.environ.get("WEBDAV_PASSWORD"),
    }

    credentials_dict = {
        "username": "user",
        "password": "pass",
    }



    connector = WebDAVConnector(
        base_url="http://172.17.0.1:8080/",
        remote_path="/",
    )

    try:
        connector.load_credentials(credentials_dict)
        connector.validate_connector_settings()
        
        document_batch_generator = connector.load_from_state()
        for document_batch in document_batch_generator:
            print("First batch of documents:")
            for doc in document_batch:
                print(f"Document ID: {doc.id}")
                print(f"Semantic Identifier: {doc.semantic_identifier}")
                print(f"Source: {doc.source}")
                print(f"Updated At: {doc.doc_updated_at}")
                print("---")
            break

    except ConnectorMissingCredentialError as e:
        print(f"Error: {e}")
    except Exception as e:
        print(f"An unexpected error occurred: {e}")

"""SeaFile connector"""
import logging
from datetime import datetime, timezone
from typing import Any, Optional

from retry import retry

from common.data_source.utils import (
    get_file_ext,
    rl_requests,
)
from common.data_source.config import (
    DocumentSource,
    INDEX_BATCH_SIZE,
    BLOB_STORAGE_SIZE_THRESHOLD,
)
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError,
)
from common.data_source.interfaces import LoadConnector, PollConnector
from common.data_source.models import (
    Document,
    SecondsSinceUnixEpoch,
    GenerateDocumentsOutput,
)

logger = logging.getLogger(__name__)


class SeaFileConnector(LoadConnector, PollConnector):
    """SeaFile connector for syncing files from SeaFile servers"""

    def __init__(
        self,
        seafile_url: str,
        batch_size: int = INDEX_BATCH_SIZE,
        include_shared: bool = True,
    ) -> None:
        """Initialize SeaFile connector.

        Args:
            seafile_url: Base URL of the SeaFile server (e.g., https://seafile.example.com)
            batch_size: Number of documents to yield per batch
            include_shared: Whether to include shared libraries
        """
     
        self.seafile_url = seafile_url.rstrip("/")
        self.api_url = f"{self.seafile_url}/api2"
        self.batch_size = batch_size
        self.include_shared = include_shared
        self.token: Optional[str] = None
        self.current_user_email: Optional[str] = None
        self.size_threshold: int = BLOB_STORAGE_SIZE_THRESHOLD

    def _get_headers(self) -> dict[str, str]:
        """Get authorization headers for API requests"""
        if not self.token:
            raise ConnectorMissingCredentialError("SeaFile token not set")
        return {
            "Authorization": f"Token {self.token}",
            "Accept": "application/json",
        }

    def _make_get_request(self, endpoint: str, params: Optional[dict] = None):
        """Make authenticated GET request"""
        url = f"{self.api_url}/{endpoint.lstrip('/')}"
        response = rl_requests.get(
            url,
            headers=self._get_headers(),
            params=params,
            timeout=60,
        )
        return response

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load and validate SeaFile credentials.

        Args:
            credentials: Dictionary containing 'seafile_token' or 'username'/'password'

        Returns:
            None

        Raises:
            ConnectorMissingCredentialError: If required credentials are missing
        """
        logger.debug(f"Loading credentials for SeaFile server {self.seafile_url}")

        token = credentials.get("seafile_token")
        username = credentials.get("username")
        password = credentials.get("password")

        if token:
            self.token = token
        elif username and password:
            self.token = self._authenticate_with_password(username, password)
        else:
            raise ConnectorMissingCredentialError(
                "SeaFile requires 'seafile_token' or 'username'/'password' credentials"
            )

        # Validate token and get current user info
        try:
            self._validate_token()
        except Exception as e:
            raise CredentialExpiredError(f"SeaFile token validation failed: {e}")

        return None

    def _authenticate_with_password(self, username: str, password: str) -> str:
        """Authenticate with username/password and return API token"""
        try:
            response = rl_requests.post(
                f"{self.api_url}/auth-token/",
                data={"username": username, "password": password},
                timeout=30,
            )
            response.raise_for_status()
            data = response.json()
            token = data.get("token")
            if not token:
                raise CredentialExpiredError("No token returned from SeaFile")
            return token
        except Exception as e:
            raise ConnectorMissingCredentialError(
                f"Failed to authenticate with SeaFile: {e}"
            )

    def _validate_token(self) -> dict:
        """Validate token by fetching account info"""
        response = self._make_get_request("/account/info/")
        response.raise_for_status()
        account_info = response.json()
        self.current_user_email = account_info.get("email")
        logger.info(f"SeaFile authenticated as: {self.current_user_email}")
        return account_info

    def validate_connector_settings(self) -> None:
        """Validate SeaFile connector settings"""
        if self.token is None:
            raise ConnectorMissingCredentialError("SeaFile credentials not loaded.")

        if not self.seafile_url:
            raise ConnectorValidationError("No SeaFile URL was provided.")

        try:
            account_info = self._validate_token()
            if not account_info.get("email"):
                raise InsufficientPermissionsError("Invalid SeaFile API response")

            # Check if we can list libraries
            libraries = self._get_libraries()
            logger.info(f"SeaFile connection validated. Found {len(libraries)} libraries.")

        except Exception as e:
            status = None
            resp = getattr(e, "response", None)
            if resp is not None:
                status = getattr(resp, "status_code", None)

            if status == 401:
                raise CredentialExpiredError("SeaFile token is invalid or expired.")
            if status == 403:
                raise InsufficientPermissionsError(
                    "Insufficient permissions to access SeaFile API."
                )
            raise ConnectorValidationError(f"SeaFile validation failed: {repr(e)}")

    @retry(tries=3, delay=1, backoff=2)
    def _get_libraries(self) -> list[dict]:
        """Fetch all accessible libraries (repos)"""
        response = self._make_get_request("/repos/")
        response.raise_for_status()
        libraries = response.json()

        logger.debug(f"Found {len(libraries)} total libraries")

        if not self.include_shared and self.current_user_email:
            # Filter to only owned libraries
            owned_libraries = [
                lib for lib in libraries
                if lib.get("owner") == self.current_user_email
                or lib.get("owner_email") == self.current_user_email
            ]
            logger.debug(
                f"Filtered to {len(owned_libraries)} owned libraries "
                f"(excluded {len(libraries) - len(owned_libraries)} shared)"
            )
            return owned_libraries

        return libraries

    @retry(tries=3, delay=1, backoff=2)
    def _get_directory_entries(self, repo_id: str, path: str = "/") -> list[dict]:
        """Fetch directory entries for a given path"""
        try:
            response = self._make_get_request(
                f"/repos/{repo_id}/dir/",
                params={"p": path},
            )
            response.raise_for_status()
            return response.json()
        except Exception as e:
            logger.warning(f"Error fetching directory {path} in repo {repo_id}: {e}")
            return []

    @retry(tries=3, delay=1, backoff=2)
    def _get_file_download_link(self, repo_id: str, path: str) -> Optional[str]:
        """Get download link for a file"""
        try:
            response = self._make_get_request(
                f"/repos/{repo_id}/file/",
                params={"p": path, "reuse": 1},
            )
            response.raise_for_status()
            return response.text.strip('"')
        except Exception as e:
            logger.warning(f"Error getting download link for {path}: {e}")
            return None

    def _list_files_recursive(
        self,
        repo_id: str,
        repo_name: str,
        path: str,
        start: datetime,
        end: datetime,
    ) -> list[tuple[str, dict, dict]]:
        """Recursively list all files in the given path within time range.

        Returns:
            List of tuples: (file_path, file_entry, library_info)
        """
        files = []
        entries = self._get_directory_entries(repo_id, path)

        for entry in entries:
            entry_type = entry.get("type")
            entry_name = entry.get("name", "")
            entry_path = f"{path.rstrip('/')}/{entry_name}"

            if entry_type == "dir":
                # Recursively process subdirectories
                files.extend(
                    self._list_files_recursive(repo_id, repo_name, entry_path, start, end)
                )
            elif entry_type == "file":
                # Check modification time
                mtime = entry.get("mtime", 0)
                if mtime:
                    modified = datetime.fromtimestamp(mtime, tz=timezone.utc)
                    if start < modified <= end:
                        files.append((entry_path, entry, {"id": repo_id, "name": repo_name}))

        return files

    def _yield_seafile_documents(
        self,
        start: datetime,
        end: datetime,
    ) -> GenerateDocumentsOutput:
        """Generate documents from SeaFile server.

        Args:
            start: Start datetime for filtering
            end: End datetime for filtering

        Yields:
            Batches of documents
        """
        logger.info(f"Searching for files between {start} and {end}")

        libraries = self._get_libraries()
        logger.info(f"Processing {len(libraries)} libraries")

        all_files = []
        for lib in libraries:
            repo_id = lib.get("id")
            repo_name = lib.get("name", "Unknown")

            if not repo_id:
                continue

            logger.debug(f"Scanning library: {repo_name}")
            try:
                files = self._list_files_recursive(repo_id, repo_name, "/", start, end)
                all_files.extend(files)
                logger.debug(f"Found {len(files)} files in {repo_name}")
            except Exception as e:
                logger.error(f"Error processing library {repo_name}: {e}")

        logger.info(f"Found {len(all_files)} total files matching time criteria")

        batch: list[Document] = []
        for file_path, file_entry, library in all_files:
            file_name = file_entry.get("name", "")
            file_size = file_entry.get("size", 0)
            file_id = file_entry.get("id", "")
            mtime = file_entry.get("mtime", 0)
            repo_id = library["id"]
            repo_name = library["name"]

            # Skip files that are too large
            if file_size > self.size_threshold:
                logger.warning(
                    f"Skipping large file: {file_path} ({file_size} bytes)"
                )
                continue

            try:
                # Get download link
                download_link = self._get_file_download_link(repo_id, file_path)
                if not download_link:
                    logger.warning(f"Could not get download link for {file_path}")
                    continue

                # Download file content
                logger.debug(f"Downloading: {file_path}")
                response = rl_requests.get(download_link, timeout=120)
                response.raise_for_status()
                blob = response.content

                if not blob:
                    logger.warning(f"Downloaded content is empty for {file_path}")
                    continue

                # Build semantic identifier
                semantic_id = f"{repo_name}{file_path}"

                # Get modification time
                modified = datetime.fromtimestamp(mtime, tz=timezone.utc) if mtime else datetime.now(timezone.utc)

                batch.append(
                    Document(
                        id=f"seafile:{repo_id}:{file_id}",
                        blob=blob,
                        source=DocumentSource.SEAFILE,
                        semantic_identifier=semantic_id,
                        extension=get_file_ext(file_name),
                        doc_updated_at=modified,
                        size_bytes=len(blob),
                    )
                )

                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

            except Exception as e:
                logger.error(f"Error downloading file {file_path}: {e}")

        if batch:
            yield batch

    def load_from_state(self) -> GenerateDocumentsOutput:
        """Load all documents from SeaFile server.

        Yields:
            Batches of documents
        """
        logger.info(f"Loading all documents from SeaFile server {self.seafile_url}")
        return self._yield_seafile_documents(
            start=datetime(1970, 1, 1, tzinfo=timezone.utc),
            end=datetime.now(timezone.utc),
        )

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> GenerateDocumentsOutput:
        """Poll SeaFile server for updated documents.

        Args:
            start: Start timestamp (seconds since Unix epoch)
            end: End timestamp (seconds since Unix epoch)

        Yields:
            Batches of documents
        """
        start_datetime = datetime.fromtimestamp(start, tz=timezone.utc)
        end_datetime = datetime.fromtimestamp(end, tz=timezone.utc)

        logger.info(f"Polling SeaFile for updates from {start_datetime} to {end_datetime}")

        for batch in self._yield_seafile_documents(start_datetime, end_datetime):
            yield batch



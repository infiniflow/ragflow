"""Dropbox connector"""

from typing import Any

from dropbox import Dropbox
from dropbox.exceptions import ApiError, AuthError

from common.data_source.config import INDEX_BATCH_SIZE
from common.data_source.exceptions import ConnectorValidationError, InsufficientPermissionsError, ConnectorMissingCredentialError
from common.data_source.interfaces import LoadConnector, PollConnector, SecondsSinceUnixEpoch


class DropboxConnector(LoadConnector, PollConnector):
    """Dropbox connector for accessing Dropbox files and folders"""

    def __init__(self, batch_size: int = INDEX_BATCH_SIZE) -> None:
        self.batch_size = batch_size
        self.dropbox_client: Dropbox | None = None

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load Dropbox credentials"""
        try:
            access_token = credentials.get("dropbox_access_token")
            if not access_token:
                raise ConnectorMissingCredentialError("Dropbox access token is required")
            
            self.dropbox_client = Dropbox(access_token)
            return None
        except Exception as e:
            raise ConnectorMissingCredentialError(f"Dropbox: {e}")

    def validate_connector_settings(self) -> None:
        """Validate Dropbox connector settings"""
        if not self.dropbox_client:
            raise ConnectorMissingCredentialError("Dropbox")
        
        try:
            # Test connection by getting current account info
            self.dropbox_client.users_get_current_account()
        except (AuthError, ApiError) as e:
            if "invalid_access_token" in str(e).lower():
                raise InsufficientPermissionsError("Invalid Dropbox access token")
            else:
                raise ConnectorValidationError(f"Dropbox validation error: {e}")

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
            # Try to get existing shared links first
            shared_links = self.dropbox_client.sharing_list_shared_links(path=path)
            if shared_links.links:
                return shared_links.links[0].url
            
            # Create a new shared link
            link_settings = self.dropbox_client.sharing_create_shared_link_with_settings(path)
            return link_settings.url
        except Exception:
            # Fallback to basic link format
            return f"https://www.dropbox.com/home{path}"

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> Any:
        """Poll Dropbox for recent file changes"""
        # Simplified implementation - in production this would handle actual polling
        return []

    def load_from_state(self) -> Any:
        """Load files from Dropbox state"""
        # Simplified implementation
        return []
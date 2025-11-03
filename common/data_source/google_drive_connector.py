"""Google Drive connector"""

from typing import Any
from googleapiclient.errors import HttpError

from common.data_source.config import INDEX_BATCH_SIZE
from common.data_source.exceptions import (
    ConnectorValidationError,
    InsufficientPermissionsError, ConnectorMissingCredentialError
)
from common.data_source.interfaces import (
    LoadConnector,
    PollConnector,
    SecondsSinceUnixEpoch,
    SlimConnectorWithPermSync
)
from common.data_source.utils import (
    get_google_creds,
    get_gmail_service
)



class GoogleDriveConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    """Google Drive connector for accessing Google Drive files and folders"""

    def __init__(self, batch_size: int = INDEX_BATCH_SIZE) -> None:
        self.batch_size = batch_size
        self.drive_service = None
        self.credentials = None

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load Google Drive credentials"""
        try:
            creds, new_creds = get_google_creds(credentials, "drive")
            self.credentials = creds
            
            if creds:
                self.drive_service = get_gmail_service(creds, credentials.get("primary_admin_email", ""))
            
            return new_creds
        except Exception as e:
            raise ConnectorMissingCredentialError(f"Google Drive: {e}")

    def validate_connector_settings(self) -> None:
        """Validate Google Drive connector settings"""
        if not self.drive_service:
            raise ConnectorMissingCredentialError("Google Drive")
        
        try:
            # Test connection by listing files
            self.drive_service.files().list(pageSize=1).execute()
        except HttpError as e:
            if e.resp.status in [401, 403]:
                raise InsufficientPermissionsError("Invalid credentials or insufficient permissions")
            else:
                raise ConnectorValidationError(f"Google Drive validation error: {e}")

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> Any:
        """Poll Google Drive for recent file changes"""
        # Simplified implementation - in production this would handle actual polling
        return []

    def load_from_state(self) -> Any:
        """Load files from Google Drive state"""
        # Simplified implementation
        return []

    def retrieve_all_slim_docs_perm_sync(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
        callback: Any = None,
    ) -> Any:
        """Retrieve all simplified documents with permission sync"""
        # Simplified implementation
        return []
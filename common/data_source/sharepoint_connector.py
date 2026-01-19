"""SharePoint connector"""

from typing import Any
import msal
from office365.graph_client import GraphClient
from office365.runtime.client_request import ClientRequestException
from office365.sharepoint.client_context import ClientContext

from common.data_source.config import INDEX_BATCH_SIZE
from common.data_source.exceptions import ConnectorValidationError, ConnectorMissingCredentialError
from common.data_source.interfaces import (
    CheckpointedConnectorWithPermSync,
    SecondsSinceUnixEpoch,
    SlimConnectorWithPermSync
)
from common.data_source.models import (
    ConnectorCheckpoint
)


class SharePointConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """SharePoint connector for accessing SharePoint sites and documents"""

    def __init__(self, batch_size: int = INDEX_BATCH_SIZE) -> None:
        self.batch_size = batch_size
        self.sharepoint_client = None
        self.graph_client = None

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load SharePoint credentials"""
        try:
            tenant_id = credentials.get("tenant_id")
            client_id = credentials.get("client_id")
            client_secret = credentials.get("client_secret")
            site_url = credentials.get("site_url")
            
            if not all([tenant_id, client_id, client_secret, site_url]):
                raise ConnectorMissingCredentialError("SharePoint credentials are incomplete")
            
            # Create MSAL confidential client
            app = msal.ConfidentialClientApplication(
                client_id=client_id,
                client_credential=client_secret,
                authority=f"https://login.microsoftonline.com/{tenant_id}"
            )
            
            # Get access token
            result = app.acquire_token_for_client(scopes=["https://graph.microsoft.com/.default"])
            
            if "access_token" not in result:
                raise ConnectorMissingCredentialError("Failed to acquire SharePoint access token")
            
            # Create Graph client
            self.graph_client = GraphClient(result["access_token"])
            
            # Create SharePoint client context
            self.sharepoint_client = ClientContext(site_url).with_access_token(result["access_token"])
            
            return None
        except Exception as e:
            raise ConnectorMissingCredentialError(f"SharePoint: {e}")

    def validate_connector_settings(self) -> None:
        """Validate SharePoint connector settings"""
        if not self.sharepoint_client or not self.graph_client:
            raise ConnectorMissingCredentialError("SharePoint")
        
        try:
            # Test connection by getting site info
            site = self.sharepoint_client.site.get().execute_query()
            if not site:
                raise ConnectorValidationError("Failed to access SharePoint site")
        except ClientRequestException as e:
            if "401" in str(e) or "403" in str(e):
                raise ConnectorValidationError("Invalid credentials or insufficient permissions")
            else:
                raise ConnectorValidationError(f"SharePoint validation error: {e}")

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> Any:
        """Poll SharePoint for recent documents"""
        # Simplified implementation - in production this would handle actual polling
        return []

    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> Any:
        """Load documents from checkpoint"""
        # Simplified implementation
        return []

    def load_from_checkpoint_with_perm_sync(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> Any:
        """Load documents from checkpoint with permission sync"""
        # Simplified implementation
        return []

    def build_dummy_checkpoint(self) -> ConnectorCheckpoint:
        """Build dummy checkpoint"""
        return ConnectorCheckpoint()

    def validate_checkpoint_json(self, checkpoint_json: str) -> ConnectorCheckpoint:
        """Validate checkpoint JSON"""
        # Simplified implementation
        return ConnectorCheckpoint()

    def retrieve_all_slim_docs_perm_sync(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
        callback: Any = None,
    ) -> Any:
        """Retrieve all simplified documents with permission sync"""
        # Simplified implementation
        return []
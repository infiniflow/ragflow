"""Microsoft Teams connector"""

from typing import Any

import msal
from office365.graph_client import GraphClient
from office365.runtime.client_request_exception import ClientRequestException

from common.data_source.exceptions import (
    ConnectorValidationError,
    InsufficientPermissionsError,
    UnexpectedValidationError, ConnectorMissingCredentialError
)
from common.data_source.interfaces import (
    SecondsSinceUnixEpoch,
    SlimConnectorWithPermSync, CheckpointedConnectorWithPermSync
)
from common.data_source.models import (
    ConnectorCheckpoint
)

_SLIM_DOC_BATCH_SIZE = 5000


class TeamsCheckpoint(ConnectorCheckpoint):
    """Teams-specific checkpoint"""
    todo_team_ids: list[str] | None = None


class TeamsConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """Microsoft Teams connector for accessing Teams messages and channels"""

    def __init__(self, batch_size: int = _SLIM_DOC_BATCH_SIZE) -> None:
        self.batch_size = batch_size
        self.teams_client = None

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load Microsoft Teams credentials"""
        try:
            tenant_id = credentials.get("tenant_id")
            client_id = credentials.get("client_id")
            client_secret = credentials.get("client_secret")
            
            if not all([tenant_id, client_id, client_secret]):
                raise ConnectorMissingCredentialError("Microsoft Teams credentials are incomplete")
            
            # Create MSAL confidential client
            app = msal.ConfidentialClientApplication(
                client_id=client_id,
                client_credential=client_secret,
                authority=f"https://login.microsoftonline.com/{tenant_id}"
            )
            
            # Get access token
            result = app.acquire_token_for_client(scopes=["https://graph.microsoft.com/.default"])
            
            if "access_token" not in result:
                raise ConnectorMissingCredentialError("Failed to acquire Microsoft Teams access token")
            
            # Create Graph client for Teams
            self.teams_client = GraphClient(result["access_token"])
            
            return None
        except Exception as e:
            raise ConnectorMissingCredentialError(f"Microsoft Teams: {e}")

    def validate_connector_settings(self) -> None:
        """Validate Microsoft Teams connector settings"""
        if not self.teams_client:
            raise ConnectorMissingCredentialError("Microsoft Teams")
        
        try:
            # Test connection by getting teams
            teams = self.teams_client.teams.get().execute_query()
            if not teams:
                raise ConnectorValidationError("Failed to access Microsoft Teams")
        except ClientRequestException as e:
            if "401" in str(e) or "403" in str(e):
                raise InsufficientPermissionsError("Invalid credentials or insufficient permissions")
            else:
                raise UnexpectedValidationError(f"Microsoft Teams validation error: {e}")

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> Any:
        """Poll Microsoft Teams for recent messages"""
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

    def build_dummy_checkpoint(self) -> ConnectorCheckpoint:
        """Build dummy checkpoint"""
        return TeamsCheckpoint()

    def validate_checkpoint_json(self, checkpoint_json: str) -> ConnectorCheckpoint:
        """Validate checkpoint JSON"""
        # Simplified implementation
        return TeamsCheckpoint()

    def retrieve_all_slim_docs_perm_sync(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
        callback: Any = None,
    ) -> Any:
        """Retrieve all simplified documents with permission sync"""
        # Simplified implementation
        return []
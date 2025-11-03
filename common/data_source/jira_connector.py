"""Jira connector"""

from typing import Any

from jira import JIRA

from common.data_source.config import INDEX_BATCH_SIZE
from common.data_source.exceptions import (
    ConnectorValidationError,
    InsufficientPermissionsError,
    UnexpectedValidationError, ConnectorMissingCredentialError
)
from common.data_source.interfaces import (
    CheckpointedConnectorWithPermSync,
    SecondsSinceUnixEpoch,
    SlimConnectorWithPermSync
)
from common.data_source.models import (
    ConnectorCheckpoint
)


class JiraConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """Jira connector for accessing Jira issues and projects"""

    def __init__(self, batch_size: int = INDEX_BATCH_SIZE) -> None:
        self.batch_size = batch_size
        self.jira_client: JIRA | None = None

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load Jira credentials"""
        try:
            url = credentials.get("url")
            username = credentials.get("username")
            password = credentials.get("password")
            token = credentials.get("token")
            
            if not url:
                raise ConnectorMissingCredentialError("Jira URL is required")
            
            if token:
                # API token authentication
                self.jira_client = JIRA(server=url, token_auth=token)
            elif username and password:
                # Basic authentication
                self.jira_client = JIRA(server=url, basic_auth=(username, password))
            else:
                raise ConnectorMissingCredentialError("Jira credentials are incomplete")
            
            return None
        except Exception as e:
            raise ConnectorMissingCredentialError(f"Jira: {e}")

    def validate_connector_settings(self) -> None:
        """Validate Jira connector settings"""
        if not self.jira_client:
            raise ConnectorMissingCredentialError("Jira")
        
        try:
            # Test connection by getting server info
            self.jira_client.server_info()
        except Exception as e:
            if "401" in str(e) or "403" in str(e):
                raise InsufficientPermissionsError("Invalid credentials or insufficient permissions")
            elif "404" in str(e):
                raise ConnectorValidationError("Jira instance not found")
            else:
                raise UnexpectedValidationError(f"Jira validation error: {e}")

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> Any:
        """Poll Jira for recent issues"""
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
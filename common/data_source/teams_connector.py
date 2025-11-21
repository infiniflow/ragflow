"""Microsoft Teams connector"""

from typing import Any
import logging
import msal
from office365.graph_client import GraphClient
from office365.runtime.client_request_exception import ClientRequestException
from common.data_source.utils import run_with_timeout

from common.data_source.exceptions import (
    ConnectorValidationError,
    InsufficientPermissionsError,
    UnexpectedValidationError, 
    ConnectorMissingCredentialError
)
from common.data_source.interfaces import (
    SecondsSinceUnixEpoch,
    SlimConnectorWithPermSync, 
    CheckpointedConnectorWithPermSync
)
from common.data_source.models import (
    ConnectorCheckpoint
)



_SLIM_DOC_BATCH_SIZE = 5000
_MAX_WORKERS = 10

class TeamsCheckpoint(ConnectorCheckpoint):
    """Teams-specific checkpoint"""
    todo_team_ids: list[str] | None = None


class TeamsConnector(CheckpointedConnectorWithPermSync, SlimConnectorWithPermSync):
    """Microsoft Teams connector for accessing Teams messages and channels"""

    def __init__(self, teams_lst: list[str] = None, max_workers: int = _MAX_WORKERS) -> None:
        self.teams_lst = teams_lst
        self.max_workers = max_workers
        self.teams_client: GraphClient | None = None
        self.msal_app: msal.ConfidentialClientApplication | None = None


    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load Microsoft Teams credentials"""
        try:
            tenant_id = credentials.get("tenant_id")
            client_id = credentials.get("client_id")
            client_secret = credentials.get("client_secret")
            
            if not all([tenant_id, client_id, client_secret]):
                raise ConnectorMissingCredentialError("Microsoft Teams credentials are incomplete")
            
            # Create MSAL confidential client
            self.msal_app = msal.ConfidentialClientApplication(
                client_id=client_id,
                client_credential=client_secret,
                authority=f"https://login.microsoftonline.com/{tenant_id}"
            )
            
            def _acquire_token_callback() -> dict[str, Any]:
                if self.msal_app is None:
                    raise RuntimeError("Failed to create MSAL ConfidentialClientApplication")
            
                # Get access token
                token = self.msal_app.acquire_token_for_client(scopes=["https://graph.microsoft.com/.default"])
                if not isinstance(token, dict) or "access_token" not in token:
                    raise RuntimeError("Failed to acquire token for Microsoft Graph API")
            
                return token

            # Create Graph client for Teams
            self.teams_client = GraphClient(token_callback=_acquire_token_callback)
            
            return None
        except Exception as e:
            raise ConnectorMissingCredentialError(f"Microsoft Teams: {e}")

    def validate_connector_settings(self) -> None:
        """Validate Microsoft Teams connector settings"""
        if not self.teams_client:
            raise ConnectorMissingCredentialError("Microsoft Teams")
        
        # Check for special characters in team names
        has_special_chars = self._has_odata_incompatible_chars(self.teams_lst)
        if has_special_chars:
            logging.info(
                "Some requested team names contain special characters (&, (, )) that require "
                "client-side filtering during data retrieval."
            )

        timeout = 10
        try:
            logging.info(
                f"Requested team count: {len(self.teams_lst) if self.teams_lst else 0}, "
                f"Has special chars: {has_special_chars}"
            )

            validation_query = self.teams_client.teams.get().top(1)
            run_with_timeout(
                timeout=timeout,
                func=lambda: validation_query.execute_query()
            )
            
            logging.info("Microsoft Teams connector settings validated successfully.")

        except TimeoutError as e:
            raise ConnectorValidationError(
                f"Timeout while validating Teams access (waited {timeout}s). "
                f"This may indicate network issues or authentication problems. "
                f"Error: {e}"
            )
        except ClientRequestException as e:
            if not e.response:
                raise RuntimeError(f"No response provided in {e=}")
            status_code = e.response.status_code
            if status_code == 401:
                raise ConnectorValidationError(
                    "Invalid or expired Microsoft Teams credentials. (401 Unauthorized)"
                )
            elif status_code == 403:
                raise InsufficientPermissionsError(
                    "Microsoft Teams connector lacks necessary permissions. (403 Forbidden)"
                )
            raise UnexpectedValidationError(
                f"Unexpected error during Teams validation: {e} (Status code: {status_code})"
            )
        except Exception as e:
            error_str = str(e).lower()
            if (
                "unauthorized" in error_str
                or "401" in error_str
                or "invalid_grant" in error_str
            ):
                raise ConnectorValidationError(
                    "Invalid or expired Microsoft Teams credentials."
                )
            elif "forbidden" in error_str or "403" in error_str:
                raise InsufficientPermissionsError(
                    "App lacks required permissions to read from Microsoft Teams."
                )
            raise ConnectorValidationError(
                f"Unexpected error during Teams validation: {e}"
            )

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
    
    def load_from_checkpoint_with_perm_sync(self, start, end, checkpoint):
        return super().load_from_checkpoint_with_perm_sync(start, end, checkpoint)
    

    def _has_odata_incompatible_chars(self, team_names: list[str] | None) -> bool:
        """Check if any team name contains characters that break Microsoft Graph OData filters.

        The Microsoft Graph Teams API has limited OData support. Characters like
        &, (, and ) cause parsing errors and require client-side filtering instead.
        """
        if not team_names:
            return False
        return any(char in name for name in team_names for char in ["&", "(", ")"])

if __name__ == "__main__":
    connector = TeamsConnector()
    creds = {
        "tenant_id": "",
        "client_id": "",
        "client_secret": ""
    }
    connector.load_credentials(creds)
    connector.validate_connector_settings()
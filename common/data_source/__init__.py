# KevinHu2 - Refactored Connectors
# This package contains refactored connectors for various services
# All code is self-contained and independent of the original codebase

from .blob_connector import BlobStorageConnector
from .slack_connector import SlackConnector
from .gmail_connector import GmailConnector
from .notion_connector import NotionConnector
from .confluence_connector import ConfluenceConnector
from .discord_connector import DiscordConnector
from .dropbox_connector import DropboxConnector
from .google_drive_connector import GoogleDriveConnector
from .jira_connector import JiraConnector
from .sharepoint_connector import SharePointConnector
from .teams_connector import TeamsConnector
from .config import BlobType, DocumentSource
from .models import Document, TextSection, ImageSection, BasicExpertInfo
from .exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError,
    UnexpectedValidationError
)

__all__ = [
    "BlobStorageConnector",
    "SlackConnector",
    "GmailConnector",
    "NotionConnector",
    "ConfluenceConnector",
    "DiscordConnector",
    "DropboxConnector",
    "GoogleDriveConnector",
    "JiraConnector",
    "SharePointConnector",
    "TeamsConnector",
    "BlobType",
    "DocumentSource",
    "Document",
    "TextSection",
    "ImageSection",
    "BasicExpertInfo",
    "ConnectorMissingCredentialError",
    "ConnectorValidationError",
    "CredentialExpiredError",
    "InsufficientPermissionsError",
    "UnexpectedValidationError"
]
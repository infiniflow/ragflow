"""
Thanks to https://github.com/onyx-dot-app/onyx

Content of this directory is under the "MIT Expat" license as defined below.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
"""

from typing import Any

from common.constants import FileSource

from .airtable_connector import AirtableConnector
from .asana_connector import AsanaConnector
from .azure_blob_connector import AzureBlobConnector
from .bigquery_connector import BigQueryConnector
from .azure_devops.connector import AzureDevOpsConnector
from .bitbucket.connector import BitbucketConnector
from .blob_connector import BlobStorageConnector
from .box_connector import BoxConnector
from .confluence_connector import ConfluenceConnector
from .config import BlobType, DocumentSource
from .dingtalk_ai_table_connector import DingTalkAITableConnector
from .discord_connector import DiscordConnector
from .dropbox_connector import DropboxConnector
from .exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError,
    UnexpectedValidationError,
)
from .github.connector import GithubConnector
from .gitlab_connector import GitlabConnector
from .gmail_connector import GmailConnector
from .google_drive.connector import GoogleDriveConnector
from .imap_connector import ImapConnector
from .jira.connector import JiraConnector
from .models import BasicExpertInfo, Document, ImageSection, TextSection
from .moodle_connector import MoodleConnector
from .notion_connector import NotionConnector
from .onedrive_connector import OneDriveConnector
from .outlook_connector import OutlookConnector
from .rdbms_connector import RDBMSConnector
from .rest_api_connector import RestAPIConnector
from .rss_connector import RSSConnector
from .salesforce_connector import SalesforceConnector
from .seafile_connector import SeaFileConnector
from .sharepoint_connector import SharePointConnector
from .slack_connector import SlackConnector
from .teams_connector import TeamsConnector
from .webdav_connector import WebDAVConnector
from .xquik_connector import XquikConnector
from .zendesk_connector import ZendeskConnector

CONNECTOR_BY_SOURCE: dict[str, type] = {
    FileSource.S3: BlobStorageConnector,
    FileSource.R2: BlobStorageConnector,
    FileSource.OCI_STORAGE: BlobStorageConnector,
    FileSource.GOOGLE_CLOUD_STORAGE: BlobStorageConnector,
    FileSource.RSS: RSSConnector,
    FileSource.CONFLUENCE: ConfluenceConnector,
    FileSource.NOTION: NotionConnector,
    FileSource.DISCORD: DiscordConnector,
    FileSource.GMAIL: GmailConnector,
    FileSource.DROPBOX: DropboxConnector,
    FileSource.GOOGLE_DRIVE: GoogleDriveConnector,
    FileSource.JIRA: JiraConnector,
    FileSource.SHAREPOINT: SharePointConnector,
    FileSource.SLACK: SlackConnector,
    FileSource.TEAMS: TeamsConnector,
    FileSource.WEBDAV: WebDAVConnector,
    FileSource.MOODLE: MoodleConnector,
    FileSource.BOX: BoxConnector,
    FileSource.AIRTABLE: AirtableConnector,
    FileSource.ASANA: AsanaConnector,
    FileSource.GITHUB: GithubConnector,
    FileSource.IMAP: ImapConnector,
    FileSource.ZENDESK: ZendeskConnector,
    FileSource.GITLAB: GitlabConnector,
    FileSource.BITBUCKET: BitbucketConnector,
    FileSource.AZURE_DEVOPS: AzureDevOpsConnector,
    FileSource.SEAFILE: SeaFileConnector,
    FileSource.DINGTALK_AI_TABLE: DingTalkAITableConnector,
    FileSource.MYSQL: RDBMSConnector,
    FileSource.POSTGRESQL: RDBMSConnector,
    FileSource.REST_API: RestAPIConnector,
    FileSource.XQUIK: XquikConnector,
    FileSource.BIGQUERY: BigQueryConnector,
    FileSource.ONEDRIVE: OneDriveConnector,
    FileSource.OUTLOOK: OutlookConnector,
    FileSource.SALESFORCE: SalesforceConnector,
    FileSource.AZURE_BLOB: AzureBlobConnector,
}


def build_connector_for_source(source: str, config: dict[str, Any]) -> Any:
    connector_cls = CONNECTOR_BY_SOURCE.get(source)
    if connector_cls is None:
        raise ConnectorValidationError(f"Unsupported data source type: {source}")
    if connector_cls is BlobStorageConnector:
        return connector_cls.build_connector(config, bucket_type=config.get("bucket_type") or source)
    if connector_cls is RDBMSConnector:
        return connector_cls.build_connector(config, db_type=source)
    return connector_cls.build_connector(config)


__all__ = [
    "CONNECTOR_BY_SOURCE",
    "AirtableConnector",
    "AsanaConnector",
    "AzureBlobConnector",
    "AzureDevOpsConnector",
    "BasicExpertInfo",
    "BigQueryConnector",
    "BitbucketConnector",
    "BlobStorageConnector",
    "BlobType",
    "BoxConnector",
    "ConfluenceConnector",
    "ConnectorMissingCredentialError",
    "ConnectorValidationError",
    "CredentialExpiredError",
    "DingTalkAITableConnector",
    "DiscordConnector",
    "Document",
    "DocumentSource",
    "DropboxConnector",
    "GithubConnector",
    "GitlabConnector",
    "GmailConnector",
    "GoogleDriveConnector",
    "ImageSection",
    "ImapConnector",
    "InsufficientPermissionsError",
    "JiraConnector",
    "MoodleConnector",
    "NotionConnector",
    "OneDriveConnector",
    "OutlookConnector",
    "RDBMSConnector",
    "RSSConnector",
    "RestAPIConnector",
    "SalesforceConnector",
    "SeaFileConnector",
    "SharePointConnector",
    "SlackConnector",
    "TeamsConnector",
    "TextSection",
    "UnexpectedValidationError",
    "WebDAVConnector",
    "XquikConnector",
    "ZendeskConnector",
    "build_connector_for_source",
]

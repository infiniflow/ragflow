"""Focused import support for Feishu Wiki tests without the global conftest."""

import sys
from types import ModuleType

_UNRELATED_PROVIDERS = {
    "airtable_connector": "AirtableConnector",
    "asana_connector": "AsanaConnector",
    "azure_blob_connector": "AzureBlobConnector",
    "bigquery_connector": "BigQueryConnector",
    "blob_connector": "BlobStorageConnector",
    "box_connector": "BoxConnector",
    "confluence_connector": "ConfluenceConnector",
    "dingtalk_ai_table_connector": "DingTalkAITableConnector",
    "discord_connector": "DiscordConnector",
    "dropbox_connector": "DropboxConnector",
    "gitlab_connector": "GitlabConnector",
    "gmail_connector": "GmailConnector",
    "imap_connector": "ImapConnector",
    "moodle_connector": "MoodleConnector",
    "notion_connector": "NotionConnector",
    "onedrive_connector": "OneDriveConnector",
    "outlook_connector": "OutlookConnector",
    "rdbms_connector": "RDBMSConnector",
    "rest_api_connector": "RestAPIConnector",
    "rss_connector": "RSSConnector",
    "salesforce_connector": "SalesforceConnector",
    "seafile_connector": "SeaFileConnector",
    "sharepoint_connector": "SharePointConnector",
    "sitemap_connector": "SitemapConnector",
    "slack_connector": "SlackConnector",
    "teams_connector": "TeamsConnector",
    "webdav_connector": "WebDAVConnector",
    "xquik_connector": "XquikConnector",
    "zendesk_connector": "ZendeskConnector",
    "azure_devops.connector": "AzureDevOpsConnector",
    "bitbucket.connector": "BitbucketConnector",
    "github.connector": "GithubConnector",
    "google_drive.connector": "GoogleDriveConnector",
    "jira.connector": "JiraConnector",
}


def install_unrelated_provider_stubs(*, include_sitemap_runtime: bool = False) -> None:
    """Install import-only substitutes for providers outside the focused tests."""
    for relative_name, class_name in _UNRELATED_PROVIDERS.items():
        module_name = f"common.data_source.{relative_name}"
        module = ModuleType(module_name)
        setattr(module, class_name, type(class_name, (), {}))
        if relative_name == "sitemap_connector" and include_sitemap_runtime:
            module.iter_in_worker_thread = lambda value, **_kwargs: value
            module.validate_connector_in_thread = lambda *_args, **_kwargs: None
        sys.modules[module_name] = module

        if "." in relative_name:
            package_name = module_name.rsplit(".", 1)[0]
            package = ModuleType(package_name)
            package.__path__ = []
            sys.modules[package_name] = package

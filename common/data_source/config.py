"""Configuration constants and enum definitions"""
import json
import os
from datetime import datetime, timezone
from enum import Enum
from typing import cast


def get_current_tz_offset() -> int:
    # datetime now() gets local time, datetime.now(timezone.utc) gets UTC time.
    # remove tzinfo to compare non-timezone-aware objects.
    time_diff = datetime.now() - datetime.now(timezone.utc).replace(tzinfo=None)
    return round(time_diff.total_seconds() / 3600)


# Default request timeout, mostly used by connectors
REQUEST_TIMEOUT_SECONDS = int(os.environ.get("REQUEST_TIMEOUT_SECONDS") or 60)

ONE_MINUTE = 60
ONE_HOUR = 3600
ONE_DAY = ONE_HOUR * 24

# Slack API limits
_SLACK_LIMIT = 900

# Redis lock configuration
ONYX_SLACK_LOCK_TTL = 1800
ONYX_SLACK_LOCK_BLOCKING_TIMEOUT = 60
ONYX_SLACK_LOCK_TOTAL_BLOCKING_TIMEOUT = 3600


class BlobType(str, Enum):
    """Supported storage types"""
    S3 = "s3"
    R2 = "r2"
    GOOGLE_CLOUD_STORAGE = "google_cloud_storage"
    OCI_STORAGE = "oci_storage"
    S3_COMPATIBLE = "s3_compatible"


class DocumentSource(str, Enum):
    """Document sources"""
    S3 = "s3"
    NOTION = "notion"
    R2 = "r2"
    GOOGLE_CLOUD_STORAGE = "google_cloud_storage"
    OCI_STORAGE = "oci_storage"
    SLACK = "slack"
    CONFLUENCE = "confluence"
    JIRA = "jira"
    GOOGLE_DRIVE = "google_drive"
    GMAIL = "gmail"
    DISCORD = "discord"
    WEBDAV = "webdav"
    MOODLE = "moodle"
    S3_COMPATIBLE = "s3_compatible"
    DROPBOX = "dropbox"
    BOX = "box"
    AIRTABLE = "airtable"
    ASANA = "asana"
    GITHUB = "github"
    GITLAB = "gitlab"
    IMAP = "imap"
    BITBUCKET = "bitbucket"
    ZENDESK = "zendesk"
    SEAFILE = "seafile"
    MYSQL = "mysql"
    POSTGRESQL = "postgresql"


class FileOrigin(str, Enum):
    """File origins"""
    CONNECTOR = "connector"


# Standard image MIME types supported by most vision LLMs
IMAGE_MIME_TYPES = [
    "image/png",
    "image/jpeg",
    "image/jpg",
    "image/webp",
]

# Image types that should be excluded from processing
EXCLUDED_IMAGE_TYPES = [
    "image/bmp",
    "image/tiff",
    "image/gif",
    "image/svg+xml",
    "image/avif",
]


_PAGE_EXPANSION_FIELDS = [
    "body.storage.value",
    "version",
    "space",
    "metadata.labels",
    "history.lastUpdated",
    "ancestors",
]


# Configuration constants
BLOB_STORAGE_SIZE_THRESHOLD = 20 * 1024 * 1024  # 20MB
INDEX_BATCH_SIZE = 2
SLACK_NUM_THREADS = 4
ENABLE_EXPENSIVE_EXPERT_CALLS = False

# Slack related constants
_SLACK_LIMIT = 900
FAST_TIMEOUT = 1
MAX_RETRIES = 7
MAX_CHANNELS_TO_LOG = 50
BOT_CHANNEL_MIN_BATCH_SIZE = 256
BOT_CHANNEL_PERCENTAGE_THRESHOLD = 0.95

# Download configuration
DOWNLOAD_CHUNK_SIZE = 1024 * 1024  # 1MB
SIZE_THRESHOLD_BUFFER = 64

NOTION_CONNECTOR_DISABLE_RECURSIVE_PAGE_LOOKUP = (
    os.environ.get("NOTION_CONNECTOR_DISABLE_RECURSIVE_PAGE_LOOKUP", "").lower()
    == "true"
)

SLIM_BATCH_SIZE = 100

# Notion API constants
_NOTION_PAGE_SIZE = 100
_NOTION_CALL_TIMEOUT = 30  # 30 seconds

_ITERATION_LIMIT = 100_000

#####
# Indexing Configs
#####
# NOTE: Currently only supported in the Confluence and Google Drive connectors +
# only handles some failures (Confluence = handles API call failures, Google
# Drive = handles failures pulling files / parsing them)
CONTINUE_ON_CONNECTOR_FAILURE = os.environ.get(
    "CONTINUE_ON_CONNECTOR_FAILURE", ""
).lower() not in ["false", ""]


#####
# Confluence Connector Configs
#####

CONFLUENCE_CONNECTOR_LABELS_TO_SKIP = [
    ignored_tag
    for ignored_tag in os.environ.get("CONFLUENCE_CONNECTOR_LABELS_TO_SKIP", "").split(
        ","
    )
    if ignored_tag
]

# Avoid to get archived pages
CONFLUENCE_CONNECTOR_INDEX_ARCHIVED_PAGES = (
    os.environ.get("CONFLUENCE_CONNECTOR_INDEX_ARCHIVED_PAGES", "").lower() == "true"
)

# Attachments exceeding this size will not be retrieved (in bytes)
CONFLUENCE_CONNECTOR_ATTACHMENT_SIZE_THRESHOLD = int(
    os.environ.get("CONFLUENCE_CONNECTOR_ATTACHMENT_SIZE_THRESHOLD", 10 * 1024 * 1024)
)
# Attachments with more chars than this will not be indexed. This is to prevent extremely
# large files from freezing indexing. 200,000 is ~100 google doc pages.
CONFLUENCE_CONNECTOR_ATTACHMENT_CHAR_COUNT_THRESHOLD = int(
    os.environ.get("CONFLUENCE_CONNECTOR_ATTACHMENT_CHAR_COUNT_THRESHOLD", 200_000)
)

_RAW_CONFLUENCE_CONNECTOR_USER_PROFILES_OVERRIDE = os.environ.get(
    "CONFLUENCE_CONNECTOR_USER_PROFILES_OVERRIDE", ""
)
CONFLUENCE_CONNECTOR_USER_PROFILES_OVERRIDE = cast(
    list[dict[str, str]] | None,
    (
        json.loads(_RAW_CONFLUENCE_CONNECTOR_USER_PROFILES_OVERRIDE)
        if _RAW_CONFLUENCE_CONNECTOR_USER_PROFILES_OVERRIDE
        else None
    ),
)

# enter as a floating point offset from UTC in hours (-24 < val < 24)
# this will be applied globally, so it probably makes sense to transition this to per
# connector as some point.
# For the default value, we assume that the user's local timezone is more likely to be
# correct (i.e. the configured user's timezone or the default server one) than UTC.
# https://developer.atlassian.com/cloud/confluence/cql-fields/#created
CONFLUENCE_TIMEZONE_OFFSET = float(
    os.environ.get("CONFLUENCE_TIMEZONE_OFFSET", get_current_tz_offset())
)

CONFLUENCE_SYNC_TIME_BUFFER_SECONDS = int(
    os.environ.get("CONFLUENCE_SYNC_TIME_BUFFER_SECONDS", ONE_DAY)
)

GOOGLE_DRIVE_CONNECTOR_SIZE_THRESHOLD = int(
    os.environ.get("GOOGLE_DRIVE_CONNECTOR_SIZE_THRESHOLD", 10 * 1024 * 1024)
)

JIRA_CONNECTOR_LABELS_TO_SKIP = [
    ignored_tag
    for ignored_tag in os.environ.get("JIRA_CONNECTOR_LABELS_TO_SKIP", "").split(",")
    if ignored_tag
]
JIRA_CONNECTOR_MAX_TICKET_SIZE = int(
    os.environ.get("JIRA_CONNECTOR_MAX_TICKET_SIZE", 100 * 1024)
)
JIRA_SYNC_TIME_BUFFER_SECONDS = int(
    os.environ.get("JIRA_SYNC_TIME_BUFFER_SECONDS", ONE_MINUTE)
)
JIRA_TIMEZONE_OFFSET = float(
    os.environ.get("JIRA_TIMEZONE_OFFSET", get_current_tz_offset())
)

OAUTH_SLACK_CLIENT_ID = os.environ.get("OAUTH_SLACK_CLIENT_ID", "")
OAUTH_SLACK_CLIENT_SECRET = os.environ.get("OAUTH_SLACK_CLIENT_SECRET", "")
OAUTH_CONFLUENCE_CLOUD_CLIENT_ID = os.environ.get(
    "OAUTH_CONFLUENCE_CLOUD_CLIENT_ID", ""
)

OAUTH_CONFLUENCE_CLOUD_CLIENT_SECRET = os.environ.get(
    "OAUTH_CONFLUENCE_CLOUD_CLIENT_SECRET", ""
)

OAUTH_JIRA_CLOUD_CLIENT_ID = os.environ.get("OAUTH_JIRA_CLOUD_CLIENT_ID", "")
OAUTH_JIRA_CLOUD_CLIENT_SECRET = os.environ.get("OAUTH_JIRA_CLOUD_CLIENT_SECRET", "")
OAUTH_GOOGLE_DRIVE_CLIENT_ID = os.environ.get("OAUTH_GOOGLE_DRIVE_CLIENT_ID", "")
OAUTH_GOOGLE_DRIVE_CLIENT_SECRET = os.environ.get(
    "OAUTH_GOOGLE_DRIVE_CLIENT_SECRET", ""
)
GOOGLE_DRIVE_WEB_OAUTH_REDIRECT_URI = os.environ.get("GOOGLE_DRIVE_WEB_OAUTH_REDIRECT_URI", "http://localhost:9380/v1/connector/google-drive/oauth/web/callback")
GMAIL_WEB_OAUTH_REDIRECT_URI = os.environ.get("GMAIL_WEB_OAUTH_REDIRECT_URI", "http://localhost:9380/v1/connector/gmail/oauth/web/callback")

CONFLUENCE_OAUTH_TOKEN_URL = "https://auth.atlassian.com/oauth/token"
RATE_LIMIT_MESSAGE_LOWERCASE = "Rate limit exceeded".lower()

_DEFAULT_PAGINATION_LIMIT = 1000

_PROBLEMATIC_EXPANSIONS = "body.storage.value"
_REPLACEMENT_EXPANSIONS = "body.view.value"

BOX_WEB_OAUTH_REDIRECT_URI = os.environ.get("BOX_WEB_OAUTH_REDIRECT_URI", "http://localhost:9380/v1/connector/box/oauth/web/callback")

GITHUB_CONNECTOR_BASE_URL = os.environ.get("GITHUB_CONNECTOR_BASE_URL") or None

class HtmlBasedConnectorTransformLinksStrategy(str, Enum):
    # remove links entirely
    STRIP = "strip"
    # turn HTML links into markdown links
    MARKDOWN = "markdown"


HTML_BASED_CONNECTOR_TRANSFORM_LINKS_STRATEGY = os.environ.get(
    "HTML_BASED_CONNECTOR_TRANSFORM_LINKS_STRATEGY",
    HtmlBasedConnectorTransformLinksStrategy.STRIP,
)

PARSE_WITH_TRAFILATURA = os.environ.get("PARSE_WITH_TRAFILATURA", "").lower() == "true"

WEB_CONNECTOR_IGNORED_CLASSES = os.environ.get(
    "WEB_CONNECTOR_IGNORED_CLASSES", "sidebar,footer"
).split(",")
WEB_CONNECTOR_IGNORED_ELEMENTS = os.environ.get(
    "WEB_CONNECTOR_IGNORED_ELEMENTS", "nav,footer,meta,script,style,symbol,aside"
).split(",")

AIRTABLE_CONNECTOR_SIZE_THRESHOLD = int(
    os.environ.get("AIRTABLE_CONNECTOR_SIZE_THRESHOLD", 10 * 1024 * 1024)
)

ASANA_CONNECTOR_SIZE_THRESHOLD = int(
    os.environ.get("ASANA_CONNECTOR_SIZE_THRESHOLD", 10 * 1024 * 1024)
)

IMAP_CONNECTOR_SIZE_THRESHOLD = int(
    os.environ.get("IMAP_CONNECTOR_SIZE_THRESHOLD", 10 * 1024 * 1024)
)

ZENDESK_CONNECTOR_SKIP_ARTICLE_LABELS = os.environ.get(
    "ZENDESK_CONNECTOR_SKIP_ARTICLE_LABELS", ""
).split(",")

_USER_NOT_FOUND = "Unknown Confluence User"

_COMMENT_EXPANSION_FIELDS = ["body.storage.value"]

_ATTACHMENT_EXPANSION_FIELDS = [
    "version",
    "space",
    "metadata.labels",
]

_RESTRICTIONS_EXPANSION_FIELDS = [
    "space",
    "restrictions.read.restrictions.user",
    "restrictions.read.restrictions.group",
    "ancestors.restrictions.read.restrictions.user",
    "ancestors.restrictions.read.restrictions.group",
]


_SLIM_DOC_BATCH_SIZE = 5000

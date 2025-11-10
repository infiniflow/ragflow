from enum import Enum

from common.data_source.config import DocumentSource

SLIM_BATCH_SIZE = 500
# NOTE: do not need https://www.googleapis.com/auth/documents.readonly
# this is counted under `/auth/drive.readonly`
GOOGLE_SCOPES = {
    DocumentSource.GOOGLE_DRIVE: [
        "https://www.googleapis.com/auth/drive.readonly",
        "https://www.googleapis.com/auth/drive.metadata.readonly",
        "https://www.googleapis.com/auth/admin.directory.group.readonly",
        "https://www.googleapis.com/auth/admin.directory.user.readonly",
    ],
    DocumentSource.GMAIL: [
        "https://www.googleapis.com/auth/gmail.readonly",
        "https://www.googleapis.com/auth/admin.directory.user.readonly",
        "https://www.googleapis.com/auth/admin.directory.group.readonly",
    ],
}


# This is the Oauth token
DB_CREDENTIALS_DICT_TOKEN_KEY = "google_tokens"
# This is the service account key
DB_CREDENTIALS_DICT_SERVICE_ACCOUNT_KEY = "google_service_account_key"
# The email saved for both auth types
DB_CREDENTIALS_PRIMARY_ADMIN_KEY = "google_primary_admin"


# https://developers.google.com/workspace/guides/create-credentials
# Internally defined authentication method type.
# The value must be one of "oauth_interactive" or "uploaded"
# Used to disambiguate whether credentials have already been created via
# certain methods and what actions we allow users to take
DB_CREDENTIALS_AUTHENTICATION_METHOD = "authentication_method"


class GoogleOAuthAuthenticationMethod(str, Enum):
    OAUTH_INTERACTIVE = "oauth_interactive"
    UPLOADED = "uploaded"


USER_FIELDS = "nextPageToken, users(primaryEmail)"


# Error message substrings
MISSING_SCOPES_ERROR_STR = "client not authorized for any of the scopes requested"
SCOPE_INSTRUCTIONS = ""

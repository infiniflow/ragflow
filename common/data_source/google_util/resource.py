import logging
from collections.abc import Callable
from typing import Any

from google.auth.exceptions import RefreshError  # type: ignore
from google.oauth2.credentials import Credentials as OAuthCredentials  # type: ignore  # type: ignore
from google.oauth2.service_account import Credentials as ServiceAccountCredentials  # type: ignore  # type: ignore
from googleapiclient.discovery import (
    Resource,  # type: ignore
    build,  # type: ignore
)


class GoogleDriveService(Resource):
    pass


class GoogleDocsService(Resource):
    pass


class AdminService(Resource):
    pass


class GmailService(Resource):
    pass


class RefreshableDriveObject:
    """
    Running Google Drive service retrieval functions
    involves accessing methods of the service object (i.e. files().list())
    which can raise a RefreshError if the access token is expired.
    This class is a wrapper that propagates the ability to refresh the access token
    and retry the final retrieval function until execute() is called.
    """

    def __init__(
        self,
        call_stack: Callable[[ServiceAccountCredentials | OAuthCredentials], Any],
        creds: ServiceAccountCredentials | OAuthCredentials,
        creds_getter: Callable[..., ServiceAccountCredentials | OAuthCredentials],
    ):
        self.call_stack = call_stack
        self.creds = creds
        self.creds_getter = creds_getter

    def __getattr__(self, name: str) -> Any:
        if name == "execute":
            return self.make_refreshable_execute()
        return RefreshableDriveObject(
            lambda creds: getattr(self.call_stack(creds), name),
            self.creds,
            self.creds_getter,
        )

    def __call__(self, *args: Any, **kwargs: Any) -> Any:
        return RefreshableDriveObject(
            lambda creds: self.call_stack(creds)(*args, **kwargs),
            self.creds,
            self.creds_getter,
        )

    def make_refreshable_execute(self) -> Callable:
        def execute(*args: Any, **kwargs: Any) -> Any:
            try:
                return self.call_stack(self.creds).execute(*args, **kwargs)
            except RefreshError as e:
                logging.warning(f"RefreshError, going to attempt a creds refresh and retry: {e}")
                # Refresh the access token
                self.creds = self.creds_getter()
                return self.call_stack(self.creds).execute(*args, **kwargs)

        return execute


def _get_google_service(
    service_name: str,
    service_version: str,
    creds: ServiceAccountCredentials | OAuthCredentials,
    user_email: str | None = None,
) -> GoogleDriveService | GoogleDocsService | AdminService | GmailService:
    service: Resource
    if isinstance(creds, ServiceAccountCredentials):
        # NOTE: https://developers.google.com/identity/protocols/oauth2/service-account#error-codes
        creds = creds.with_subject(user_email)
        service = build(service_name, service_version, credentials=creds)
    elif isinstance(creds, OAuthCredentials):
        service = build(service_name, service_version, credentials=creds)

    return service


def get_google_docs_service(
    creds: ServiceAccountCredentials | OAuthCredentials,
    user_email: str | None = None,
) -> GoogleDocsService:
    return _get_google_service("docs", "v1", creds, user_email)


def get_drive_service(
    creds: ServiceAccountCredentials | OAuthCredentials,
    user_email: str | None = None,
) -> GoogleDriveService:
    return _get_google_service("drive", "v3", creds, user_email)


def get_admin_service(
    creds: ServiceAccountCredentials | OAuthCredentials,
    user_email: str | None = None,
) -> AdminService:
    return _get_google_service("admin", "directory_v1", creds, user_email)


def get_gmail_service(
    creds: ServiceAccountCredentials | OAuthCredentials,
    user_email: str | None = None,
) -> GmailService:
    return _get_google_service("gmail", "v1", creds, user_email)

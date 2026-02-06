import logging
from typing import Any
from google.oauth2.credentials import Credentials as OAuthCredentials
from google.oauth2.service_account import Credentials as ServiceAccountCredentials
from googleapiclient.errors import HttpError

from common.data_source.config import INDEX_BATCH_SIZE, SLIM_BATCH_SIZE, DocumentSource
from common.data_source.google_util.auth import get_google_creds
from common.data_source.google_util.constant import DB_CREDENTIALS_PRIMARY_ADMIN_KEY, MISSING_SCOPES_ERROR_STR, SCOPE_INSTRUCTIONS, USER_FIELDS
from common.data_source.google_util.resource import get_admin_service, get_gmail_service
from common.data_source.google_util.util import _execute_single_retrieval, execute_paginated_retrieval, clean_string
from common.data_source.interfaces import LoadConnector, PollConnector, SecondsSinceUnixEpoch, SlimConnectorWithPermSync
from common.data_source.models import BasicExpertInfo, Document, ExternalAccess, GenerateDocumentsOutput, GenerateSlimDocumentOutput, SlimDocument, TextSection
from common.data_source.utils import build_time_range_query, clean_email_and_extract_name, get_message_body, is_mail_service_disabled_error, gmail_time_str_to_utc, sanitize_filename

# Constants for Gmail API fields
THREAD_LIST_FIELDS = "nextPageToken, threads(id)"
PARTS_FIELDS = "parts(body(data), mimeType)"
PAYLOAD_FIELDS = f"payload(headers, {PARTS_FIELDS})"
MESSAGES_FIELDS = f"messages(id, {PAYLOAD_FIELDS})"
THREADS_FIELDS = f"threads(id, {MESSAGES_FIELDS})"
THREAD_FIELDS = f"id, {MESSAGES_FIELDS}"

EMAIL_FIELDS = ["cc", "bcc", "from", "to"]


def _get_owners_from_emails(emails: dict[str, str | None]) -> list[BasicExpertInfo]:
    """Convert email dictionary to list of BasicExpertInfo objects."""
    owners = []
    for email, names in emails.items():
        if names:
            name_parts = names.split(" ")
            first_name = " ".join(name_parts[:-1])
            last_name = name_parts[-1]
        else:
            first_name = None
            last_name = None
        owners.append(BasicExpertInfo(email=email, first_name=first_name, last_name=last_name))
    return owners


def message_to_section(message: dict[str, Any]) -> tuple[TextSection, dict[str, str]]:
    """Convert Gmail message to text section and metadata."""
    link = f"https://mail.google.com/mail/u/0/#inbox/{message['id']}"

    payload = message.get("payload", {})
    headers = payload.get("headers", [])
    metadata: dict[str, Any] = {}

    for header in headers:
        name = header.get("name", "").lower()
        value = header.get("value", "")
        if name in EMAIL_FIELDS:
            metadata[name] = value
        if name == "subject":
            metadata["subject"] = value
        if name == "date":
            metadata["updated_at"] = value

    if labels := message.get("labelIds"):
        metadata["labels"] = labels

    message_data = ""
    for name, value in metadata.items():
        if name != "updated_at":
            message_data += f"{name}: {value}\n"

    message_body_text: str = get_message_body(payload)
    return TextSection(link=link, text=message_body_text + message_data), metadata


def thread_to_document(full_thread: dict[str, Any], email_used_to_fetch_thread: str) -> Document | None:
    """Convert Gmail thread to Document object."""
    all_messages = full_thread.get("messages", [])
    if not all_messages:
        return None

    sections = []
    semantic_identifier = ""
    updated_at = None
    from_emails: dict[str, str | None] = {}
    other_emails: dict[str, str | None] = {}

    for message in all_messages:
        section, message_metadata = message_to_section(message)
        sections.append(section)

        for name, value in message_metadata.items():
            if name in EMAIL_FIELDS:
                email, display_name = clean_email_and_extract_name(value)
                if name == "from":
                    from_emails[email] = display_name if not from_emails.get(email) else None
                else:
                    other_emails[email] = display_name if not other_emails.get(email) else None

        if not semantic_identifier:
            semantic_identifier = message_metadata.get("subject", "")
            semantic_identifier = clean_string(semantic_identifier)
            semantic_identifier = sanitize_filename(semantic_identifier)

        if message_metadata.get("updated_at"):
            updated_at = message_metadata.get("updated_at")
            
    updated_at_datetime = None
    if updated_at:
        updated_at_datetime = gmail_time_str_to_utc(updated_at)

    thread_id = full_thread.get("id")
    if not thread_id:
        raise ValueError("Thread ID is required")

    primary_owners = _get_owners_from_emails(from_emails)
    secondary_owners = _get_owners_from_emails(other_emails)

    if not semantic_identifier:
        semantic_identifier = "(no subject)"

    combined_sections = "\n\n".join(
        sec.text for sec in sections if hasattr(sec, "text")
    )
    blob = combined_sections
    size_bytes = len(blob)
    extension = '.txt'

    return Document(
        id=thread_id,
        semantic_identifier=semantic_identifier,
        blob=blob,
        size_bytes=size_bytes,
        extension=extension,
        source=DocumentSource.GMAIL,
        primary_owners=primary_owners,
        secondary_owners=secondary_owners,
        doc_updated_at=updated_at_datetime,
        metadata=message_metadata,
        external_access=ExternalAccess(
            external_user_emails={email_used_to_fetch_thread},
            external_user_group_ids=set(),
            is_public=False,
        ),
    )


class GmailConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    """Gmail connector for synchronizing emails from Gmail accounts."""

    def __init__(self, batch_size: int = INDEX_BATCH_SIZE) -> None:
        self.batch_size = batch_size
        self._creds: OAuthCredentials | ServiceAccountCredentials | None = None
        self._primary_admin_email: str | None = None

    @property
    def primary_admin_email(self) -> str:
        """Get primary admin email."""
        if self._primary_admin_email is None:
            raise RuntimeError("Primary admin email missing, should not call this property before calling load_credentials")
        return self._primary_admin_email

    @property
    def google_domain(self) -> str:
        """Get Google domain from email."""
        if self._primary_admin_email is None:
            raise RuntimeError("Primary admin email missing, should not call this property before calling load_credentials")
        return self._primary_admin_email.split("@")[-1]

    @property
    def creds(self) -> OAuthCredentials | ServiceAccountCredentials:
        """Get Google credentials."""
        if self._creds is None:
            raise RuntimeError("Creds missing, should not call this property before calling load_credentials")
        return self._creds

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, str] | None:
        """Load Gmail credentials."""
        primary_admin_email = credentials[DB_CREDENTIALS_PRIMARY_ADMIN_KEY]
        self._primary_admin_email = primary_admin_email

        self._creds, new_creds_dict = get_google_creds(
            credentials=credentials,
            source=DocumentSource.GMAIL,
        )
        return new_creds_dict

    def _get_all_user_emails(self) -> list[str]:
        """Get all user emails for Google Workspace domain."""
        try:
            admin_service = get_admin_service(self.creds, self.primary_admin_email)
            emails = []
            for user in execute_paginated_retrieval(
                retrieval_function=admin_service.users().list,
                list_key="users",
                fields=USER_FIELDS,
                domain=self.google_domain,
            ):
                if email := user.get("primaryEmail"):
                    emails.append(email)
            return emails
        except HttpError as e:
            if e.resp.status == 404:
                logging.warning("Received 404 from Admin SDK; this may indicate a personal Gmail account with no Workspace domain. Falling back to single user.")
                return [self.primary_admin_email]
            raise
        except Exception:
            raise

    def _fetch_threads(
        self,
        time_range_start: SecondsSinceUnixEpoch | None = None,
        time_range_end: SecondsSinceUnixEpoch | None = None,
    ) -> GenerateDocumentsOutput:
        """Fetch Gmail threads within time range."""
        query = build_time_range_query(time_range_start, time_range_end)
        doc_batch = []

        for user_email in self._get_all_user_emails():
            gmail_service = get_gmail_service(self.creds, user_email)
            try:
                for thread in execute_paginated_retrieval(
                    retrieval_function=gmail_service.users().threads().list,
                    list_key="threads",
                    userId=user_email,
                    fields=THREAD_LIST_FIELDS,
                    q=query,
                    continue_on_404_or_403=True,
                ):
                    full_thread = _execute_single_retrieval(
                        retrieval_function=gmail_service.users().threads().get,
                        userId=user_email,
                        fields=THREAD_FIELDS,
                        id=thread["id"],
                        continue_on_404_or_403=True,
                    )
                    doc = thread_to_document(full_thread, user_email)
                    if doc is None:
                        continue

                    doc_batch.append(doc)
                    if len(doc_batch) > self.batch_size:
                        yield doc_batch
                        doc_batch = []
            except HttpError as e:
                if is_mail_service_disabled_error(e):
                    logging.warning(
                        "Skipping Gmail sync for %s because the mailbox is disabled.",
                        user_email,
                    )
                    continue
                raise

        if doc_batch:
            yield doc_batch

    def load_from_state(self) -> GenerateDocumentsOutput:
        """Load all documents from Gmail."""
        try:
            yield from self._fetch_threads()
        except Exception as e:
            if MISSING_SCOPES_ERROR_STR in str(e):
                raise PermissionError(SCOPE_INSTRUCTIONS) from e
            raise e

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> GenerateDocumentsOutput:
        """Poll Gmail for documents within time range."""
        try:
            yield from self._fetch_threads(start, end)
        except Exception as e:
            if MISSING_SCOPES_ERROR_STR in str(e):
                raise PermissionError(SCOPE_INSTRUCTIONS) from e
            raise e

    def retrieve_all_slim_docs_perm_sync(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
        callback=None,
    ) -> GenerateSlimDocumentOutput:
        """Retrieve slim documents for permission synchronization."""
        query = build_time_range_query(start, end)
        doc_batch = []

        for user_email in self._get_all_user_emails():
            logging.info(f"Fetching slim threads for user: {user_email}")
            gmail_service = get_gmail_service(self.creds, user_email)
            try:
                for thread in execute_paginated_retrieval(
                    retrieval_function=gmail_service.users().threads().list,
                    list_key="threads",
                    userId=user_email,
                    fields=THREAD_LIST_FIELDS,
                    q=query,
                    continue_on_404_or_403=True,
                ):
                    doc_batch.append(
                        SlimDocument(
                            id=thread["id"],
                            external_access=ExternalAccess(
                                external_user_emails={user_email},
                                external_user_group_ids=set(),
                                is_public=False,
                            ),
                        )
                    )
                    if len(doc_batch) > SLIM_BATCH_SIZE:
                        yield doc_batch
                        doc_batch = []
            except HttpError as e:
                if is_mail_service_disabled_error(e):
                    logging.warning(
                        "Skipping slim Gmail sync for %s because the mailbox is disabled.",
                        user_email,
                    )
                    continue
                raise

        if doc_batch:
            yield doc_batch


if __name__ == "__main__":
    import time
    import os
    from common.data_source.google_util.util import get_credentials_from_env
    logging.basicConfig(level=logging.INFO)
    try:
        email = os.environ.get("GMAIL_TEST_EMAIL", "newyorkupperbay@gmail.com")
        creds = get_credentials_from_env(email, oauth=True, source="gmail")
        print("Credentials loaded successfully")
        print(f"{creds=}")

        connector = GmailConnector(batch_size=2)
        print("GmailConnector initialized")
        connector.load_credentials(creds)
        print("Credentials loaded into connector")

        print("Gmail is ready to use")

        for file in connector._fetch_threads(
            int(time.time()) - 1 * 24 * 60 * 60,
            int(time.time()),
        ):
            print("new batch","-"*80)
            for f in file:
                print(f)
                print("\n\n")
    except Exception as e:
        logging.exception(f"Error loading credentials: {e}")
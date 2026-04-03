import copy
import email
from email.header import decode_header
import imaplib
import logging
import os
import re
from datetime import datetime, timedelta
from datetime import timezone
from email.message import Message
from email.utils import collapse_rfc2231_value, parseaddr
from enum import Enum
from typing import Any
from typing import cast
import uuid

import bs4
from pydantic import BaseModel

from common.data_source.config import IMAP_CONNECTOR_SIZE_THRESHOLD, DocumentSource
from common.data_source.interfaces import CheckpointOutput, CheckpointedConnectorWithPermSync, CredentialsConnector, CredentialsProviderInterface
from common.data_source.models import BasicExpertInfo, ConnectorCheckpoint, Document, ExternalAccess, SecondsSinceUnixEpoch

_DEFAULT_IMAP_PORT_NUMBER = int(os.environ.get("IMAP_PORT", 993))
_IMAP_OKAY_STATUS = "OK"
_PAGE_SIZE = 100
_USERNAME_KEY = "imap_username"
_PASSWORD_KEY = "imap_password"

class Header(str, Enum):
    SUBJECT_HEADER = "subject"
    FROM_HEADER = "from"
    TO_HEADER = "to"
    CC_HEADER = "cc"
    DELIVERED_TO_HEADER = (
        "Delivered-To"  # Used in mailing lists instead of the "to" header.
    )
    DATE_HEADER = "date"
    MESSAGE_ID_HEADER = "Message-ID"


class EmailHeaders(BaseModel):
    """
    Model for email headers extracted from IMAP messages.
    """

    id: str
    subject: str
    sender: str
    recipients: str | None
    cc: str | None
    date: datetime

    @classmethod
    def from_email_msg(cls, email_msg: Message) -> "EmailHeaders":
        def _decode(header: str, default: str | None = None) -> str | None:
            value = email_msg.get(header, default)
            if not value:
                return None

            decoded_fragments = decode_header(value)
            decoded_strings: list[str] = []

            for decoded_value, encoding in decoded_fragments:
                if isinstance(decoded_value, bytes):
                    try:
                        decoded_strings.append(
                            decoded_value.decode(encoding or "utf-8", errors="replace")
                        )
                    except LookupError:
                        decoded_strings.append(
                            decoded_value.decode("utf-8", errors="replace")
                        )
                elif isinstance(decoded_value, str):
                    decoded_strings.append(decoded_value)
                else:
                    decoded_strings.append(str(decoded_value))

            return "".join(decoded_strings)

        def _parse_date(date_str: str | None) -> datetime | None:
            if not date_str:
                return None
            try:
                return email.utils.parsedate_to_datetime(date_str)
            except (TypeError, ValueError):
                return None

        message_id = _decode(header=Header.MESSAGE_ID_HEADER)
        if not message_id:
            message_id = f"<generated-{uuid.uuid4()}@imap.local>"
        # It's possible for the subject line to not exist or be an empty string.
        subject = _decode(header=Header.SUBJECT_HEADER) or "Unknown Subject"
        from_ = _decode(header=Header.FROM_HEADER)
        to = _decode(header=Header.TO_HEADER)
        if not to:
            to = _decode(header=Header.DELIVERED_TO_HEADER)
        cc = _decode(header=Header.CC_HEADER)
        date_str = _decode(header=Header.DATE_HEADER)
        date = _parse_date(date_str=date_str)

        if not date:
            date = datetime.now(tz=timezone.utc)

        # If any of the above are `None`, model validation will fail.
        # Therefore, no guards (i.e.: `if <header> is None: raise RuntimeError(..)`) were written.
        return cls.model_validate(
            {
                "id": message_id,
                "subject": subject,
                "sender": from_,
                "recipients": to,
                "cc": cc,
                "date": date,
            }
        )

class CurrentMailbox(BaseModel):
    mailbox: str
    todo_email_ids: list[str]


# An email has a list of mailboxes.
# Each mailbox has a list of email-ids inside of it.
#
# Usage:
# To use this checkpointer, first fetch all the mailboxes.
# Then, pop a mailbox and fetch all of its email-ids.
# Then, pop each email-id and fetch its content (and parse it, etc..).
# When you have popped all email-ids for this mailbox, pop the next mailbox and repeat the above process until you're done.
#
# For initial checkpointing, set both fields to `None`.
class ImapCheckpoint(ConnectorCheckpoint):
    todo_mailboxes: list[str] | None = None
    current_mailbox: CurrentMailbox | None = None


class LoginState(str, Enum):
    LoggedIn = "logged_in"
    LoggedOut = "logged_out"


class ImapConnector(
    CredentialsConnector,
    CheckpointedConnectorWithPermSync,
):
    def __init__(
        self,
        host: str,
        port: int = _DEFAULT_IMAP_PORT_NUMBER,
        mailboxes: list[str] | None = None,
    ) -> None:
        self._host = host
        self._port = port
        self._mailboxes = mailboxes
        self._credentials: dict[str, Any] | None = None

    @property
    def credentials(self) -> dict[str, Any]:
        if not self._credentials:
            raise RuntimeError(
                "Credentials have not been initialized; call `set_credentials_provider` first"
            )
        return self._credentials

    def _get_mail_client(self) -> imaplib.IMAP4_SSL:
        """
        Returns a new `imaplib.IMAP4_SSL` instance.

        The `imaplib.IMAP4_SSL` object is supposed to be an "ephemeral" object; it's not something that you can login,
        logout, then log back into again. I.e., the following will fail:

        ```py
        mail_client.login(..)
        mail_client.logout();
        mail_client.login(..)
        ```

        Therefore, you need a fresh, new instance in order to operate with IMAP. This function gives one to you.

        # Notes
        This function will throw an error if the credentials have not yet been set.
        """

        def get_or_raise(name: str) -> str:
            value = self.credentials.get(name)
            if not value:
                raise RuntimeError(f"Credential item {name=} was not found")
            if not isinstance(value, str):
                raise RuntimeError(
                    f"Credential item {name=} must be of type str, instead received {type(name)=}"
                )
            return value

        username = get_or_raise(_USERNAME_KEY)
        password = get_or_raise(_PASSWORD_KEY)

        mail_client = imaplib.IMAP4_SSL(host=self._host, port=self._port)
        status, _data = mail_client.login(user=username, password=password)

        if status != _IMAP_OKAY_STATUS:
            raise RuntimeError(f"Failed to log into imap server; {status=}")

        return mail_client

    def _load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ImapCheckpoint,
        include_perm_sync: bool,
    ) -> CheckpointOutput[ImapCheckpoint]:
        checkpoint = cast(ImapCheckpoint, copy.deepcopy(checkpoint))
        checkpoint.has_more = True

        mail_client = self._get_mail_client()

        if checkpoint.todo_mailboxes is None:
            # This is the dummy checkpoint.
            # Fill it with mailboxes first.
            if self._mailboxes:
                checkpoint.todo_mailboxes = _sanitize_mailbox_names(self._mailboxes)
            else:
                fetched_mailboxes = _fetch_all_mailboxes_for_email_account(
                    mail_client=mail_client
                )
                if not fetched_mailboxes:
                    raise RuntimeError(
                        "Failed to find any mailboxes for this email account"
                    )
                checkpoint.todo_mailboxes = _sanitize_mailbox_names(fetched_mailboxes)

            return checkpoint

        if (
            not checkpoint.current_mailbox
            or not checkpoint.current_mailbox.todo_email_ids
        ):
            if not checkpoint.todo_mailboxes:
                checkpoint.has_more = False
                return checkpoint

            mailbox = checkpoint.todo_mailboxes.pop()
            email_ids = _fetch_email_ids_in_mailbox(
                mail_client=mail_client,
                mailbox=mailbox,
                start=start,
                end=end,
            )
            checkpoint.current_mailbox = CurrentMailbox(
                mailbox=mailbox,
                todo_email_ids=email_ids,
            )

        _select_mailbox(
            mail_client=mail_client, mailbox=checkpoint.current_mailbox.mailbox
        )
        current_todos = cast(
            list, copy.deepcopy(checkpoint.current_mailbox.todo_email_ids[:_PAGE_SIZE])
        )
        checkpoint.current_mailbox.todo_email_ids = (
            checkpoint.current_mailbox.todo_email_ids[_PAGE_SIZE:]
        )

        for email_id in current_todos:
            email_msg = _fetch_email(mail_client=mail_client, email_id=email_id)
            if not email_msg:
                logging.warning(f"Failed to fetch message {email_id=}; skipping")
                continue

            email_headers = EmailHeaders.from_email_msg(email_msg=email_msg)
            msg_dt = email_headers.date
            if msg_dt.tzinfo is None:
                msg_dt = msg_dt.replace(tzinfo=timezone.utc)
            else:
                msg_dt = msg_dt.astimezone(timezone.utc)

            start_dt = datetime.fromtimestamp(start, tz=timezone.utc)
            end_dt = datetime.fromtimestamp(end, tz=timezone.utc)

            if not (start_dt < msg_dt <= end_dt):
                continue

            email_doc = _convert_email_headers_and_body_into_document(
                email_msg=email_msg,
                email_headers=email_headers,
                include_perm_sync=include_perm_sync,
            )
            yield email_doc
            attachments = extract_attachments(email_msg)
            for att in attachments:
                yield attachment_to_document(email_doc, att, email_headers)

        return checkpoint

    # impls for BaseConnector

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self._credentials = credentials
        return None

    def validate_connector_settings(self) -> None:
        self._get_mail_client()

    # impls for CredentialsConnector

    def set_credentials_provider(
        self, credentials_provider: CredentialsProviderInterface
    ) -> None:
        self._credentials = credentials_provider.get_credentials()

    # impls for CheckpointedConnector

    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ImapCheckpoint,
    ) -> CheckpointOutput[ImapCheckpoint]:
        return self._load_from_checkpoint(
            start=start, end=end, checkpoint=checkpoint, include_perm_sync=False
        )

    def build_dummy_checkpoint(self) -> ImapCheckpoint:
        return ImapCheckpoint(has_more=True)

    def validate_checkpoint_json(self, checkpoint_json: str) -> ImapCheckpoint:
        return ImapCheckpoint.model_validate_json(json_data=checkpoint_json)

    # impls for CheckpointedConnectorWithPermSync

    def load_from_checkpoint_with_perm_sync(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ImapCheckpoint,
    ) -> CheckpointOutput[ImapCheckpoint]:
        return self._load_from_checkpoint(
            start=start, end=end, checkpoint=checkpoint, include_perm_sync=True
        )


def _fetch_all_mailboxes_for_email_account(mail_client: imaplib.IMAP4_SSL) -> list[str]:
    status, mailboxes_data = mail_client.list('""', "*")
    if status != _IMAP_OKAY_STATUS:
        raise RuntimeError(f"Failed to fetch mailboxes; {status=}")

    mailboxes = []

    for mailboxes_raw in mailboxes_data:
        if isinstance(mailboxes_raw, bytes):
            mailboxes_str = mailboxes_raw.decode()
        elif isinstance(mailboxes_raw, str):
            mailboxes_str = mailboxes_raw
        else:
            logging.warning(
                f"Expected the mailbox data to be of type str, instead got {type(mailboxes_raw)=} {mailboxes_raw}; skipping"
            )
            continue

        # The mailbox LIST response output can be found here:
        # https://www.rfc-editor.org/rfc/rfc3501.html#section-7.2.2
        #
        # The general format is:
        # `(<name-attributes>) <hierarchy-delimiter> <mailbox-name>`
        #
        # The below regex matches on that pattern; from there, we select the 3rd match (index 2), which is the mailbox-name.
        match = re.match(r'\([^)]*\)\s+"([^"]+)"\s+"?(.+?)"?$', mailboxes_str)
        if not match:
            logging.warning(
                f"Invalid mailbox-data formatting structure: {mailboxes_str=}; skipping"
            )
            continue

        mailbox = match.group(2)
        mailboxes.append(mailbox)
    if not mailboxes:
        logging.warning(
            "No mailboxes parsed from LIST response; falling back to INBOX"
        )
        return ["INBOX"]

    return mailboxes


def _select_mailbox(mail_client: imaplib.IMAP4_SSL, mailbox: str) -> bool:
    try:
        status, _ = mail_client.select(mailbox=mailbox, readonly=True)
        if status != _IMAP_OKAY_STATUS:
            return False
        return True
    except Exception:
        return False


def _fetch_email_ids_in_mailbox(
    mail_client: imaplib.IMAP4_SSL,
    mailbox: str,
    start: SecondsSinceUnixEpoch,
    end: SecondsSinceUnixEpoch,
) -> list[str]:
    if not _select_mailbox(mail_client, mailbox):
        logging.warning(f"Skip mailbox: {mailbox}")
        return []

    start_dt = datetime.fromtimestamp(start, tz=timezone.utc)
    end_dt = datetime.fromtimestamp(end, tz=timezone.utc) + timedelta(days=1)

    start_str = start_dt.strftime("%d-%b-%Y")
    end_str = end_dt.strftime("%d-%b-%Y")
    search_criteria = f'(SINCE "{start_str}" BEFORE "{end_str}")'

    status, email_ids_byte_array = mail_client.search(None, search_criteria)

    if status != _IMAP_OKAY_STATUS or not email_ids_byte_array:
        raise RuntimeError(f"Failed to fetch email ids; {status=}")

    email_ids: bytes = email_ids_byte_array[0]

    return [email_id.decode() for email_id in email_ids.split()]


def _fetch_email(mail_client: imaplib.IMAP4_SSL, email_id: str) -> Message | None:
    status, msg_data = mail_client.fetch(message_set=email_id, message_parts="(RFC822)")
    if status != _IMAP_OKAY_STATUS or not msg_data:
        return None

    data = msg_data[0]
    if not isinstance(data, tuple):
        raise RuntimeError(
            f"Message data should be a tuple; instead got a {type(data)=} {data=}"
        )

    _, raw_email = data
    return email.message_from_bytes(raw_email)


def _convert_email_headers_and_body_into_document(
    email_msg: Message,
    email_headers: EmailHeaders,
    include_perm_sync: bool,
) -> Document:
    sender_name, sender_addr = _parse_singular_addr(raw_header=email_headers.sender)
    to_addrs = (
        _parse_addrs(email_headers.recipients)
        if email_headers.recipients
        else []
    )
    cc_addrs = (
        _parse_addrs(email_headers.cc)
        if email_headers.cc
        else []
    )
    all_participants = to_addrs + cc_addrs

    expert_info_map = {
        recipient_addr: BasicExpertInfo(
            display_name=recipient_name, email=recipient_addr
        )
        for recipient_name, recipient_addr in all_participants
    }
    if sender_addr not in expert_info_map:
        expert_info_map[sender_addr] = BasicExpertInfo(
            display_name=sender_name, email=sender_addr
        )

    email_body = _parse_email_body(email_msg=email_msg, email_headers=email_headers)
    primary_owners = list(expert_info_map.values())
    external_access = (
        ExternalAccess(
            external_user_emails=set(expert_info_map.keys()),
            external_user_group_ids=set(),
            is_public=False,
        )
        if include_perm_sync
        else None
    )
    return Document(
        id=email_headers.id,
        title=email_headers.subject,
        blob=email_body,
        size_bytes=len(email_body),
        semantic_identifier=email_headers.subject,
        metadata={},
        extension='.txt',
        doc_updated_at=email_headers.date,
        source=DocumentSource.IMAP,
        primary_owners=primary_owners,
        external_access=external_access,
    )

def extract_attachments(email_msg: Message, max_bytes: int = IMAP_CONNECTOR_SIZE_THRESHOLD):
    attachments = []

    if not email_msg.is_multipart():
        return attachments

    for part in email_msg.walk():
        if part.get_content_maintype() == "multipart":
            continue

        disposition = (part.get("Content-Disposition") or "").lower()
        filename = part.get_filename()

        if not (
            disposition.startswith("attachment")
            or (disposition.startswith("inline") and filename)
        ):
            continue

        payload = part.get_payload(decode=True)
        if not payload:
            continue

        if len(payload) > max_bytes:
            continue

        attachments.append({
            "filename": filename or "attachment.bin",
            "content_type": part.get_content_type(),
            "content_bytes": payload,
            "size_bytes": len(payload),
        })

    return attachments

def decode_mime_filename(raw: str | None) -> str | None:
    if not raw:
        return None

    try:
        raw = collapse_rfc2231_value(raw)
    except Exception:
        pass

    parts = decode_header(raw)
    decoded = []

    for value, encoding in parts:
        if isinstance(value, bytes):
            decoded.append(value.decode(encoding or "utf-8", errors="replace"))
        else:
            decoded.append(value)

    return "".join(decoded)

def attachment_to_document(
    parent_doc: Document,
    att: dict,
    email_headers: EmailHeaders,
):
    raw_filename = att["filename"]
    filename = decode_mime_filename(raw_filename) or "attachment.bin"
    ext = "." + filename.split(".")[-1] if "." in filename else ""

    return Document(
        id=f"{parent_doc.id}#att:{filename}",
        source=DocumentSource.IMAP,
        semantic_identifier=filename,
        extension=ext,
        blob=att["content_bytes"],
        size_bytes=att["size_bytes"],
        doc_updated_at=email_headers.date,
        primary_owners=parent_doc.primary_owners,
        metadata={
            "parent_email_id": parent_doc.id,
            "parent_subject": email_headers.subject,
            "attachment_filename": filename,
            "attachment_content_type": att["content_type"],
        },
    )

def _parse_email_body(
    email_msg: Message,
    email_headers: EmailHeaders,
) -> str:
    body = None
    for part in email_msg.walk():
        if part.is_multipart():
            # Multipart parts are *containers* for other parts, not the actual content itself.
            # Therefore, we skip until we find the individual parts instead.
            continue

        charset = part.get_content_charset() or "utf-8"

        try:
            raw_payload = part.get_payload(decode=True)
            if not isinstance(raw_payload, bytes):
                logging.warning(
                    "Payload section from email was expected to be an array of bytes, instead got "
                    f"{type(raw_payload)=}, {raw_payload=}"
                )
                continue
            body = raw_payload.decode(charset)
            break
        except (UnicodeDecodeError, LookupError) as e:
            logging.warning(f"Could not decode part with charset {charset}. Error: {e}")
            continue

    if not body:
        logging.warning(
            f"Email with {email_headers.id=} has an empty body; returning an empty string"
        )
        return ""

    soup = bs4.BeautifulSoup(markup=body, features="html.parser")

    return " ".join(str_section for str_section in soup.stripped_strings)


def _sanitize_mailbox_names(mailboxes: list[str]) -> list[str]:
    """
    Mailboxes with special characters in them must be enclosed by double-quotes, as per the IMAP protocol.
    Just to be safe, we wrap *all* mailboxes with double-quotes.
    """
    return [f'"{mailbox}"' for mailbox in mailboxes if mailbox]


def _parse_addrs(raw_header: str) -> list[tuple[str, str]]:
    addrs = raw_header.split(",")
    name_addr_pairs = [parseaddr(addr=addr) for addr in addrs if addr]
    return [(name, addr) for name, addr in name_addr_pairs if addr]


def _parse_singular_addr(raw_header: str) -> tuple[str, str]:
    addrs = _parse_addrs(raw_header=raw_header)
    if not addrs:
        return ("Unknown", "unknown@example.com")
    elif len(addrs) >= 2:
        raise RuntimeError(
            f"Expected a singular address, but instead got multiple; {raw_header=} {addrs=}"
        )

    return addrs[0]


if __name__ == "__main__":
    import time
    from types import TracebackType
    from common.data_source.utils import load_all_docs_from_checkpoint_connector


    class OnyxStaticCredentialsProvider(
        CredentialsProviderInterface["OnyxStaticCredentialsProvider"]
    ):
        """Implementation (a very simple one!) to handle static credentials."""

        def __init__(
            self,
            tenant_id: str | None,
            connector_name: str,
            credential_json: dict[str, Any],
        ):
            self._tenant_id = tenant_id
            self._connector_name = connector_name
            self._credential_json = credential_json

            self._provider_key = str(uuid.uuid4())

        def __enter__(self) -> "OnyxStaticCredentialsProvider":
            return self

        def __exit__(
            self,
            exc_type: type[BaseException] | None,
            exc_value: BaseException | None,
            traceback: TracebackType | None,
        ) -> None:
            pass

        def get_tenant_id(self) -> str | None:
            return self._tenant_id

        def get_provider_key(self) -> str:
            return self._provider_key

        def get_credentials(self) -> dict[str, Any]:
            return self._credential_json

        def set_credentials(self, credential_json: dict[str, Any]) -> None:
            self._credential_json = credential_json

        def is_dynamic(self) -> bool:
            return False
    # from tests.daily.connectors.utils import load_all_docs_from_checkpoint_connector
    # from onyx.connectors.credentials_provider import OnyxStaticCredentialsProvider

    host = os.environ.get("IMAP_HOST")
    mailboxes_str = os.environ.get("IMAP_MAILBOXES","INBOX")
    username = os.environ.get("IMAP_USERNAME")
    password = os.environ.get("IMAP_PASSWORD")

    mailboxes = (
        [mailbox.strip() for mailbox in mailboxes_str.split(",")]
        if mailboxes_str
        else []
    )

    if not host:
        raise RuntimeError("`IMAP_HOST` must be set")

    imap_connector = ImapConnector(
        host=host,
        mailboxes=mailboxes,
    )

    imap_connector.set_credentials_provider(
        OnyxStaticCredentialsProvider(
            tenant_id=None,
            connector_name=DocumentSource.IMAP,
            credential_json={
                _USERNAME_KEY: username,
                _PASSWORD_KEY: password,
            },
        )
    )
    END = time.time()
    START = END - 1 * 24 * 60 * 60
    for doc in load_all_docs_from_checkpoint_connector(
        connector=imap_connector,
        start=START,
        end=END,
    ):
        print(doc.id,doc.extension)
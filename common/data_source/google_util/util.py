import json
import logging
import os
import re
import socket
from collections.abc import Callable, Iterator
from enum import Enum
from typing import Any
import unicodedata
from googleapiclient.errors import HttpError  # type: ignore  # type: ignore

from common.data_source.config import DocumentSource
from common.data_source.google_drive.model import GoogleDriveFileType
from common.data_source.google_util.oauth_flow import ensure_oauth_token_dict


# See https://developers.google.com/drive/api/reference/rest/v3/files/list for more
class GoogleFields(str, Enum):
    ID = "id"
    CREATED_TIME = "createdTime"
    MODIFIED_TIME = "modifiedTime"
    NAME = "name"
    SIZE = "size"
    PARENTS = "parents"


NEXT_PAGE_TOKEN_KEY = "nextPageToken"
PAGE_TOKEN_KEY = "pageToken"
ORDER_BY_KEY = "orderBy"


def get_file_owners(file: GoogleDriveFileType, primary_admin_email: str) -> list[str]:
    """
    Get the owners of a file if the attribute is present.
    """
    return [email for owner in file.get("owners", []) if (email := owner.get("emailAddress")) and email.split("@")[-1] == primary_admin_email.split("@")[-1]]


# included for type purposes; caller should not need to address
# Nones unless max_num_pages is specified. Use
# execute_paginated_retrieval_with_max_pages instead if you want
# the early stop + yield None after max_num_pages behavior.
def execute_paginated_retrieval(
    retrieval_function: Callable,
    list_key: str | None = None,
    continue_on_404_or_403: bool = False,
    **kwargs: Any,
) -> Iterator[GoogleDriveFileType]:
    for item in _execute_paginated_retrieval(
        retrieval_function,
        list_key,
        continue_on_404_or_403,
        **kwargs,
    ):
        if not isinstance(item, str):
            yield item


def execute_paginated_retrieval_with_max_pages(
    retrieval_function: Callable,
    max_num_pages: int,
    list_key: str | None = None,
    continue_on_404_or_403: bool = False,
    **kwargs: Any,
) -> Iterator[GoogleDriveFileType | str]:
    yield from _execute_paginated_retrieval(
        retrieval_function,
        list_key,
        continue_on_404_or_403,
        max_num_pages=max_num_pages,
        **kwargs,
    )


def _execute_paginated_retrieval(
    retrieval_function: Callable,
    list_key: str | None = None,
    continue_on_404_or_403: bool = False,
    max_num_pages: int | None = None,
    **kwargs: Any,
) -> Iterator[GoogleDriveFileType | str]:
    """Execute a paginated retrieval from Google Drive API
    Args:
        retrieval_function: The specific list function to call (e.g., service.files().list)
        list_key: If specified, each object returned by the retrieval function
                  will be accessed at the specified key and yielded from.
        continue_on_404_or_403: If True, the retrieval will continue even if the request returns a 404 or 403 error.
        max_num_pages: If specified, the retrieval will stop after the specified number of pages and yield None.
        **kwargs: Arguments to pass to the list function
    """
    if "fields" not in kwargs or "nextPageToken" not in kwargs["fields"]:
        raise ValueError("fields must contain nextPageToken for execute_paginated_retrieval")
    next_page_token = kwargs.get(PAGE_TOKEN_KEY, "")
    num_pages = 0
    while next_page_token is not None:
        if max_num_pages is not None and num_pages >= max_num_pages:
            yield next_page_token
            return
        num_pages += 1
        request_kwargs = kwargs.copy()
        if next_page_token:
            request_kwargs[PAGE_TOKEN_KEY] = next_page_token
        results = _execute_single_retrieval(
            retrieval_function,
            continue_on_404_or_403,
            **request_kwargs,
        )

        next_page_token = results.get(NEXT_PAGE_TOKEN_KEY)
        if list_key:
            for item in results.get(list_key, []):
                yield item
        else:
            yield results


def _execute_single_retrieval(
    retrieval_function: Callable,
    continue_on_404_or_403: bool = False,
    **request_kwargs: Any,
) -> GoogleDriveFileType:
    """Execute a single retrieval from Google Drive API"""
    try:
        results = retrieval_function(**request_kwargs).execute()

    except HttpError as e:
        if e.resp.status >= 500:
            results = retrieval_function()
        elif e.resp.status == 400:
            if "pageToken" in request_kwargs and "Invalid Value" in str(e) and "pageToken" in str(e):
                logging.warning(f"Invalid page token: {request_kwargs['pageToken']}, retrying from start of request")
                request_kwargs.pop("pageToken")
                return _execute_single_retrieval(
                    retrieval_function,
                    continue_on_404_or_403,
                    **request_kwargs,
                )
            logging.error(f"Error executing request: {e}")
            raise e
        elif e.resp.status == 404 or e.resp.status == 403:
            if continue_on_404_or_403:
                logging.debug(f"Error executing request: {e}")
                results = {}
            else:
                raise e
        elif e.resp.status == 429:
            results = retrieval_function()
        else:
            logging.exception("Error executing request:")
            raise e
    except (TimeoutError, socket.timeout) as error:
        logging.warning(
            "Timed out executing Google API request; retrying with backoff. Details: %s",
            error,
        )
        results = retrieval_function()
    return results


def get_credentials_from_env(email: str, oauth: bool = False, source="drive") -> dict:
    try:
        if oauth:
            raw_credential_string = os.environ["GOOGLE_OAUTH_CREDENTIALS_JSON_STR"]
        else:
            raw_credential_string = os.environ["GOOGLE_SERVICE_ACCOUNT_JSON_STR"]
    except KeyError:
        raise ValueError("Missing Google Drive credentials in environment variables")

    try:
        credential_dict = json.loads(raw_credential_string)
    except json.JSONDecodeError:
        raise ValueError("Invalid JSON in Google Drive credentials")

    if oauth and source == "drive":
        credential_dict = ensure_oauth_token_dict(credential_dict, DocumentSource.GOOGLE_DRIVE)
    else:
        credential_dict = ensure_oauth_token_dict(credential_dict, DocumentSource.GMAIL)

    refried_credential_string = json.dumps(credential_dict)

    DB_CREDENTIALS_DICT_TOKEN_KEY = "google_tokens"
    DB_CREDENTIALS_DICT_SERVICE_ACCOUNT_KEY = "google_service_account_key"
    DB_CREDENTIALS_PRIMARY_ADMIN_KEY = "google_primary_admin"
    DB_CREDENTIALS_AUTHENTICATION_METHOD = "authentication_method"

    cred_key = DB_CREDENTIALS_DICT_TOKEN_KEY if oauth else DB_CREDENTIALS_DICT_SERVICE_ACCOUNT_KEY

    return {
        cred_key: refried_credential_string,
        DB_CREDENTIALS_PRIMARY_ADMIN_KEY: email,
        DB_CREDENTIALS_AUTHENTICATION_METHOD: "uploaded",
    }


def clean_string(text: str | None) -> str | None:
    """
    Clean a string to make it safe for insertion into MySQL (utf8mb4).
    - Normalize Unicode
    - Remove control characters / zero-width characters
    - Optionally remove high-plane emoji and symbols
    """
    if text is None:
        return None

    # 0. Ensure the value is a string
    text = str(text)

    # 1. Normalize Unicode (NFC)
    text = unicodedata.normalize("NFC", text)

    # 2. Remove ASCII control characters (except tab, newline, carriage return)
    text = re.sub(r"[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]", "", text)

    # 3. Remove zero-width characters / BOM
    text = re.sub(r"[\u200b-\u200d\uFEFF]", "", text)

    # 4. Remove high Unicode characters (emoji, special symbols)
    text = re.sub(r"[\U00010000-\U0010FFFF]", "", text)

    # 5. Final fallback: strip any invalid UTF-8 sequences
    try:
        text.encode("utf-8")
    except UnicodeEncodeError:
        text = text.encode("utf-8", errors="ignore").decode("utf-8")

    return text
"""Google Drive connector"""

import copy
import logging
import os
import sys
import threading
from collections.abc import Callable, Generator, Iterator
from enum import Enum
from functools import partial
from typing import Any, Protocol, cast
from urllib.parse import urlparse

from google.auth.exceptions import RefreshError  # type: ignore  # type: ignore
from google.oauth2.credentials import Credentials as OAuthCredentials  # type: ignore  # type: ignore  # type: ignore
from google.oauth2.service_account import Credentials as ServiceAccountCredentials  # type: ignore  # type: ignore
from googleapiclient.errors import HttpError  # type: ignore  # type: ignore
from typing_extensions import override

from common.data_source.config import GOOGLE_DRIVE_CONNECTOR_SIZE_THRESHOLD, INDEX_BATCH_SIZE, SLIM_BATCH_SIZE, DocumentSource
from common.data_source.exceptions import ConnectorMissingCredentialError, ConnectorValidationError, CredentialExpiredError, InsufficientPermissionsError
from common.data_source.google_drive.doc_conversion import PermissionSyncContext, build_slim_document, convert_drive_item_to_document, onyx_document_id_from_drive_file
from common.data_source.google_drive.file_retrieval import (
    DriveFileFieldType,
    crawl_folders_for_files,
    get_all_files_for_oauth,
    get_all_files_in_my_drive_and_shared,
    get_files_in_shared_drive,
    get_root_folder_id,
)
from common.data_source.google_drive.model import DriveRetrievalStage, GoogleDriveCheckpoint, GoogleDriveFileType, RetrievedDriveFile, StageCompletion
from common.data_source.google_util.auth import get_google_creds
from common.data_source.google_util.constant import DB_CREDENTIALS_PRIMARY_ADMIN_KEY, MISSING_SCOPES_ERROR_STR, USER_FIELDS
from common.data_source.google_util.resource import GoogleDriveService, get_admin_service, get_drive_service
from common.data_source.google_util.util import GoogleFields, execute_paginated_retrieval, get_file_owners
from common.data_source.google_util.util_threadpool_concurrency import ThreadSafeDict
from common.data_source.interfaces import (
    CheckpointedConnectorWithPermSync,
    IndexingHeartbeatInterface,
    SlimConnectorWithPermSync,
)
from common.data_source.models import CheckpointOutput, ConnectorFailure, Document, EntityFailure, GenerateSlimDocumentOutput, SecondsSinceUnixEpoch
from common.data_source.utils import datetime_from_string, parallel_yield, run_functions_tuples_in_parallel

MAX_DRIVE_WORKERS = int(os.environ.get("MAX_DRIVE_WORKERS", 4))
SHARED_DRIVE_PAGES_PER_CHECKPOINT = 2
MY_DRIVE_PAGES_PER_CHECKPOINT = 2
OAUTH_PAGES_PER_CHECKPOINT = 2
FOLDERS_PER_CHECKPOINT = 1


def _extract_str_list_from_comma_str(string: str | None) -> list[str]:
    if not string:
        return []
    return [s.strip() for s in string.split(",") if s.strip()]


def _extract_ids_from_urls(urls: list[str]) -> list[str]:
    return [urlparse(url).path.strip("/").split("/")[-1] for url in urls]


def _clean_requested_drive_ids(
    requested_drive_ids: set[str],
    requested_folder_ids: set[str],
    all_drive_ids_available: set[str],
) -> tuple[list[str], list[str]]:
    invalid_requested_drive_ids = requested_drive_ids - all_drive_ids_available
    filtered_folder_ids = requested_folder_ids - all_drive_ids_available
    if invalid_requested_drive_ids:
        logging.warning(f"Some shared drive IDs were not found. IDs: {invalid_requested_drive_ids}")
        logging.warning("Checking for folder access instead...")
        filtered_folder_ids.update(invalid_requested_drive_ids)

    valid_requested_drive_ids = requested_drive_ids - invalid_requested_drive_ids
    return sorted(valid_requested_drive_ids), sorted(filtered_folder_ids)


def add_retrieval_info(
    drive_files: Iterator[GoogleDriveFileType | str],
    user_email: str,
    completion_stage: DriveRetrievalStage,
    parent_id: str | None = None,
) -> Iterator[RetrievedDriveFile | str]:
    for file in drive_files:
        if isinstance(file, str):
            yield file
            continue
        yield RetrievedDriveFile(
            drive_file=file,
            user_email=user_email,
            parent_id=parent_id,
            completion_stage=completion_stage,
        )


class CredentialedRetrievalMethod(Protocol):
    def __call__(
        self,
        field_type: DriveFileFieldType,
        checkpoint: GoogleDriveCheckpoint,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> Iterator[RetrievedDriveFile]: ...


class DriveIdStatus(str, Enum):
    AVAILABLE = "available"
    IN_PROGRESS = "in_progress"
    FINISHED = "finished"


class GoogleDriveConnector(SlimConnectorWithPermSync, CheckpointedConnectorWithPermSync):
    def __init__(
        self,
        include_shared_drives: bool = False,
        include_my_drives: bool = False,
        include_files_shared_with_me: bool = False,
        shared_drive_urls: str | None = None,
        my_drive_emails: str | None = None,
        shared_folder_urls: str | None = None,
        specific_user_emails: str | None = None,
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        if not any(
            (
                include_shared_drives,
                include_my_drives,
                include_files_shared_with_me,
                shared_folder_urls,
                my_drive_emails,
                shared_drive_urls,
            )
        ):
            raise ConnectorValidationError(
                "Nothing to index. Please specify at least one of the following: include_shared_drives, include_my_drives, include_files_shared_with_me, shared_folder_urls, or my_drive_emails"
            )

        specific_requests_made = False
        if bool(shared_drive_urls) or bool(my_drive_emails) or bool(shared_folder_urls):
            specific_requests_made = True
        self.specific_requests_made = specific_requests_made

        # NOTE: potentially modified in load_credentials if using service account
        self.include_files_shared_with_me = False if specific_requests_made else include_files_shared_with_me
        self.include_my_drives = False if specific_requests_made else include_my_drives
        self.include_shared_drives = False if specific_requests_made else include_shared_drives

        shared_drive_url_list = _extract_str_list_from_comma_str(shared_drive_urls)
        self._requested_shared_drive_ids = set(_extract_ids_from_urls(shared_drive_url_list))

        self._requested_my_drive_emails = set(_extract_str_list_from_comma_str(my_drive_emails))

        shared_folder_url_list = _extract_str_list_from_comma_str(shared_folder_urls)
        self._requested_folder_ids = set(_extract_ids_from_urls(shared_folder_url_list))
        self._specific_user_emails = _extract_str_list_from_comma_str(specific_user_emails)

        self._primary_admin_email: str | None = None

        self._creds: OAuthCredentials | ServiceAccountCredentials | None = None
        self._creds_dict: dict[str, Any] | None = None

        # ids of folders and shared drives that have been traversed
        self._retrieved_folder_and_drive_ids: set[str] = set()

        self.allow_images = False

        self.size_threshold = GOOGLE_DRIVE_CONNECTOR_SIZE_THRESHOLD

        self.logger = logging.getLogger(self.__class__.__name__)

    def set_allow_images(self, value: bool) -> None:
        self.allow_images = value

    @property
    def primary_admin_email(self) -> str:
        if self._primary_admin_email is None:
            raise RuntimeError("Primary admin email missing, should not call this property before calling load_credentials")
        return self._primary_admin_email

    @property
    def google_domain(self) -> str:
        if self._primary_admin_email is None:
            raise RuntimeError("Primary admin email missing, should not call this property before calling load_credentials")
        return self._primary_admin_email.split("@")[-1]

    @property
    def creds(self) -> OAuthCredentials | ServiceAccountCredentials:
        if self._creds is None:
            raise RuntimeError("Creds missing, should not call this property before calling load_credentials")
        return self._creds

    # TODO: ensure returned new_creds_dict is actually persisted when this is called?
    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        try:
            self._primary_admin_email = credentials[DB_CREDENTIALS_PRIMARY_ADMIN_KEY]
        except KeyError:
            raise ValueError("Credentials json missing primary admin key")

        self._creds, new_creds_dict = get_google_creds(
            credentials=credentials,
            source=DocumentSource.GOOGLE_DRIVE,
        )

        # Service account connectors don't have a specific setting determining whether
        # to include "shared with me" for each user, so we default to true unless the connector
        # is in specific folders/drives mode. Note that shared files are only picked up during
        # the My Drive stage, so this does nothing if the connector is set to only index shared drives.
        if isinstance(self._creds, ServiceAccountCredentials) and not self.specific_requests_made:
            self.include_files_shared_with_me = True

        self._creds_dict = new_creds_dict

        return new_creds_dict

    def _update_traversed_parent_ids(self, folder_id: str) -> None:
        self._retrieved_folder_and_drive_ids.add(folder_id)

    def _get_all_user_emails(self) -> list[str]:
        if self._specific_user_emails:
            return self._specific_user_emails

        # Start with primary admin email
        user_emails = [self.primary_admin_email]

        # Only fetch additional users if using service account
        if isinstance(self.creds, OAuthCredentials):
            return user_emails

        admin_service = get_admin_service(
            creds=self.creds,
            user_email=self.primary_admin_email,
        )

        # Get admins first since they're more likely to have access to most files
        for is_admin in [True, False]:
            query = "isAdmin=true" if is_admin else "isAdmin=false"
            for user in execute_paginated_retrieval(
                retrieval_function=admin_service.users().list,
                list_key="users",
                fields=USER_FIELDS,
                domain=self.google_domain,
                query=query,
            ):
                if email := user.get("primaryEmail"):
                    if email not in user_emails:
                        user_emails.append(email)
        return user_emails

    def get_all_drive_ids(self) -> set[str]:
        return self._get_all_drives_for_user(self.primary_admin_email)

    def _get_all_drives_for_user(self, user_email: str) -> set[str]:
        drive_service = get_drive_service(self.creds, user_email)
        is_service_account = isinstance(self.creds, ServiceAccountCredentials)
        self.logger.info(f"Getting all drives for user {user_email} with service account: {is_service_account}")
        all_drive_ids: set[str] = set()
        for drive in execute_paginated_retrieval(
            retrieval_function=drive_service.drives().list,
            list_key="drives",
            useDomainAdminAccess=is_service_account,
            fields="drives(id),nextPageToken",
        ):
            all_drive_ids.add(drive["id"])

        if not all_drive_ids:
            self.logger.warning("No drives found even though indexing shared drives was requested.")

        return all_drive_ids

    def make_drive_id_getter(self, drive_ids: list[str], checkpoint: GoogleDriveCheckpoint) -> Callable[[str], str | None]:
        status_lock = threading.Lock()

        in_progress_drive_ids = {
            completion.current_folder_or_drive_id: user_email
            for user_email, completion in checkpoint.completion_map.items()
            if completion.stage == DriveRetrievalStage.SHARED_DRIVE_FILES and completion.current_folder_or_drive_id is not None
        }
        drive_id_status: dict[str, DriveIdStatus] = {}
        for drive_id in drive_ids:
            if drive_id in self._retrieved_folder_and_drive_ids:
                drive_id_status[drive_id] = DriveIdStatus.FINISHED
            elif drive_id in in_progress_drive_ids:
                drive_id_status[drive_id] = DriveIdStatus.IN_PROGRESS
            else:
                drive_id_status[drive_id] = DriveIdStatus.AVAILABLE

        def get_available_drive_id(thread_id: str) -> str | None:
            completion = checkpoint.completion_map[thread_id]
            with status_lock:
                future_work = None
                for drive_id, status in drive_id_status.items():
                    if drive_id in self._retrieved_folder_and_drive_ids:
                        drive_id_status[drive_id] = DriveIdStatus.FINISHED
                        continue
                    if drive_id in completion.processed_drive_ids:
                        continue

                    if status == DriveIdStatus.AVAILABLE:
                        # add to processed drive ids so if this user fails to retrieve once
                        # they won't try again on the next checkpoint run
                        completion.processed_drive_ids.add(drive_id)
                        return drive_id
                    elif status == DriveIdStatus.IN_PROGRESS:
                        self.logger.debug(f"Drive id in progress: {drive_id}")
                        future_work = drive_id

                if future_work:
                    # in this case, all drive ids are either finished or in progress.
                    # This thread will pick up one of the in progress ones in case it fails.
                    # This is a much simpler approach than waiting for a failure picking it up,
                    # at the cost of some repeated work until all shared drives are retrieved.
                    # we avoid apocalyptic cases like all threads focusing on one huge drive
                    # because the drive id is added to _retrieved_folder_and_drive_ids after any thread
                    # manages to retrieve any file from it (unfortunately, this is also the reason we currently
                    # sometimes fail to retrieve restricted access folders/files)
                    completion.processed_drive_ids.add(future_work)
                    return future_work
            return None  # no work available, return None

        return get_available_drive_id

    def _impersonate_user_for_retrieval(
        self,
        user_email: str,
        field_type: DriveFileFieldType,
        checkpoint: GoogleDriveCheckpoint,
        get_new_drive_id: Callable[[str], str | None],
        sorted_filtered_folder_ids: list[str],
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> Iterator[RetrievedDriveFile]:
        self.logger.info(f"Impersonating user {user_email}")
        curr_stage = checkpoint.completion_map[user_email]
        resuming = True
        if curr_stage.stage == DriveRetrievalStage.START:
            self.logger.info(f"Setting stage to {DriveRetrievalStage.MY_DRIVE_FILES.value}")
            curr_stage.stage = DriveRetrievalStage.MY_DRIVE_FILES
            resuming = False
        drive_service = get_drive_service(self.creds, user_email)

        # validate that the user has access to the drive APIs by performing a simple
        # request and checking for a 401
        try:
            self.logger.debug(f"Getting root folder id for user {user_email}")
            get_root_folder_id(drive_service)
        except HttpError as e:
            if e.status_code == 401:
                # fail gracefully, let the other impersonations continue
                # one user without access shouldn't block the entire connector
                self.logger.warning(f"User '{user_email}' does not have access to the drive APIs.")
                # mark this user as done so we don't try to retrieve anything for them
                # again
                curr_stage.stage = DriveRetrievalStage.DONE
                return
            raise
        except RefreshError as e:
            self.logger.warning(f"User '{user_email}' could not refresh their token. Error: {e}")
            # mark this user as done so we don't try to retrieve anything for them
            # again
            yield RetrievedDriveFile(
                completion_stage=DriveRetrievalStage.DONE,
                drive_file={},
                user_email=user_email,
                error=e,
            )
            curr_stage.stage = DriveRetrievalStage.DONE
            return
        # if we are including my drives, try to get the current user's my
        # drive if any of the following are true:
        # - include_my_drives is true
        # - the current user's email is in the requested emails
        if curr_stage.stage == DriveRetrievalStage.MY_DRIVE_FILES:
            if self.include_my_drives or user_email in self._requested_my_drive_emails:
                self.logger.info(
                    f"Getting all files in my drive as '{user_email}. Resuming: {resuming}. Stage completed until: {curr_stage.completed_until}. Next page token: {curr_stage.next_page_token}"
                )

                for file_or_token in add_retrieval_info(
                    get_all_files_in_my_drive_and_shared(
                        service=drive_service,
                        update_traversed_ids_func=self._update_traversed_parent_ids,
                        field_type=field_type,
                        include_shared_with_me=self.include_files_shared_with_me,
                        max_num_pages=MY_DRIVE_PAGES_PER_CHECKPOINT,
                        start=curr_stage.completed_until if resuming else start,
                        end=end,
                        cache_folders=not bool(curr_stage.completed_until),
                        page_token=curr_stage.next_page_token,
                    ),
                    user_email,
                    DriveRetrievalStage.MY_DRIVE_FILES,
                ):
                    if isinstance(file_or_token, str):
                        self.logger.debug(f"Done with max num pages for user {user_email}")
                        checkpoint.completion_map[user_email].next_page_token = file_or_token
                        return  # done with the max num pages, return checkpoint
                    yield file_or_token

            checkpoint.completion_map[user_email].next_page_token = None
            curr_stage.stage = DriveRetrievalStage.SHARED_DRIVE_FILES
            curr_stage.current_folder_or_drive_id = None
            return  # resume from next stage on the next run

        if curr_stage.stage == DriveRetrievalStage.SHARED_DRIVE_FILES:

            def _yield_from_drive(drive_id: str, drive_start: SecondsSinceUnixEpoch | None) -> Iterator[RetrievedDriveFile | str]:
                yield from add_retrieval_info(
                    get_files_in_shared_drive(
                        service=drive_service,
                        drive_id=drive_id,
                        field_type=field_type,
                        max_num_pages=SHARED_DRIVE_PAGES_PER_CHECKPOINT,
                        update_traversed_ids_func=self._update_traversed_parent_ids,
                        cache_folders=not bool(drive_start),  # only cache folders for 0 or None
                        start=drive_start,
                        end=end,
                        page_token=curr_stage.next_page_token,
                    ),
                    user_email,
                    DriveRetrievalStage.SHARED_DRIVE_FILES,
                    parent_id=drive_id,
                )

            # resume from a checkpoint
            if resuming and (drive_id := curr_stage.current_folder_or_drive_id):
                resume_start = curr_stage.completed_until
                for file_or_token in _yield_from_drive(drive_id, resume_start):
                    if isinstance(file_or_token, str):
                        checkpoint.completion_map[user_email].next_page_token = file_or_token
                        return  # done with the max num pages, return checkpoint
                    yield file_or_token

            drive_id = get_new_drive_id(user_email)
            if drive_id:
                self.logger.info(f"Getting files in shared drive '{drive_id}' as '{user_email}. Resuming: {resuming}")
                curr_stage.completed_until = 0
                curr_stage.current_folder_or_drive_id = drive_id
                for file_or_token in _yield_from_drive(drive_id, start):
                    if isinstance(file_or_token, str):
                        checkpoint.completion_map[user_email].next_page_token = file_or_token
                        return  # done with the max num pages, return checkpoint
                    yield file_or_token
                curr_stage.current_folder_or_drive_id = None
                return  # get a new drive id on the next run

            checkpoint.completion_map[user_email].next_page_token = None
            curr_stage.stage = DriveRetrievalStage.FOLDER_FILES
            curr_stage.current_folder_or_drive_id = None
            return  # resume from next stage on the next run

        # In the folder files section of service account retrieval we take extra care
        # to not retrieve duplicate docs. In particular, we only add a folder to
        # retrieved_folder_and_drive_ids when all users are finished retrieving files
        # from that folder, and maintain a set of all file ids that have been retrieved
        # for each folder. This might get rather large; in practice we assume that the
        # specific folders users choose to index don't have too many files.
        if curr_stage.stage == DriveRetrievalStage.FOLDER_FILES:

            def _yield_from_folder_crawl(folder_id: str, folder_start: SecondsSinceUnixEpoch | None) -> Iterator[RetrievedDriveFile]:
                for retrieved_file in crawl_folders_for_files(
                    service=drive_service,
                    parent_id=folder_id,
                    field_type=field_type,
                    user_email=user_email,
                    traversed_parent_ids=self._retrieved_folder_and_drive_ids,
                    update_traversed_ids_func=self._update_traversed_parent_ids,
                    start=folder_start,
                    end=end,
                ):
                    yield retrieved_file

            # resume from a checkpoint
            last_processed_folder = None
            if resuming:
                folder_id = curr_stage.current_folder_or_drive_id
                if folder_id is None:
                    self.logger.warning(f"folder id not set in checkpoint for user {user_email}. This happens occasionally when the connector is interrupted and resumed.")
                else:
                    resume_start = curr_stage.completed_until
                    yield from _yield_from_folder_crawl(folder_id, resume_start)
                last_processed_folder = folder_id

            skipping_seen_folders = last_processed_folder is not None
            # NOTE: this assumes a small number of folders to crawl. If someone
            # really wants to specify a large number of folders, we should use
            # binary search to find the first unseen folder.
            num_completed_folders = 0
            for folder_id in sorted_filtered_folder_ids:
                if skipping_seen_folders:
                    skipping_seen_folders = folder_id != last_processed_folder
                    continue

                if folder_id in self._retrieved_folder_and_drive_ids:
                    continue

                curr_stage.completed_until = 0
                curr_stage.current_folder_or_drive_id = folder_id

                if num_completed_folders >= FOLDERS_PER_CHECKPOINT:
                    return  # resume from this folder on the next run

                self.logger.info(f"Getting files in folder '{folder_id}' as '{user_email}'")
                yield from _yield_from_folder_crawl(folder_id, start)
                num_completed_folders += 1

        curr_stage.stage = DriveRetrievalStage.DONE

    def _manage_service_account_retrieval(
        self,
        field_type: DriveFileFieldType,
        checkpoint: GoogleDriveCheckpoint,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> Iterator[RetrievedDriveFile]:
        """
        The current implementation of the service account retrieval does some
        initial setup work using the primary admin email, then runs MAX_DRIVE_WORKERS
        concurrent threads, each of which impersonates a different user and retrieves
        files for that user. Technically, the actual work each thread does is "yield the
        next file retrieved by the user", at which point it returns to the thread pool;
        see parallel_yield for more details.
        """
        if checkpoint.completion_stage == DriveRetrievalStage.START:
            checkpoint.completion_stage = DriveRetrievalStage.USER_EMAILS

        if checkpoint.completion_stage == DriveRetrievalStage.USER_EMAILS:
            all_org_emails: list[str] = self._get_all_user_emails()
            checkpoint.user_emails = all_org_emails
            checkpoint.completion_stage = DriveRetrievalStage.DRIVE_IDS
        else:
            if checkpoint.user_emails is None:
                raise ValueError("user emails not set")
            all_org_emails = checkpoint.user_emails

        sorted_drive_ids, sorted_folder_ids = self._determine_retrieval_ids(checkpoint, DriveRetrievalStage.MY_DRIVE_FILES)

        # Setup initial completion map on first connector run
        for email in all_org_emails:
            # don't overwrite existing completion map on resuming runs
            if email in checkpoint.completion_map:
                continue
            checkpoint.completion_map[email] = StageCompletion(
                stage=DriveRetrievalStage.START,
                completed_until=0,
                processed_drive_ids=set(),
            )

        # we've found all users and drives, now time to actually start
        # fetching stuff
        self.logger.info(f"Found {len(all_org_emails)} users to impersonate")
        self.logger.debug(f"Users: {all_org_emails}")
        self.logger.info(f"Found {len(sorted_drive_ids)} drives to retrieve")
        self.logger.debug(f"Drives: {sorted_drive_ids}")
        self.logger.info(f"Found {len(sorted_folder_ids)} folders to retrieve")
        self.logger.debug(f"Folders: {sorted_folder_ids}")

        drive_id_getter = self.make_drive_id_getter(sorted_drive_ids, checkpoint)

        # only process emails that we haven't already completed retrieval for
        non_completed_org_emails = [user_email for user_email, stage_completion in checkpoint.completion_map.items() if stage_completion.stage != DriveRetrievalStage.DONE]

        self.logger.debug(f"Non-completed users remaining: {len(non_completed_org_emails)}")

        # don't process too many emails before returning a checkpoint. This is
        # to resolve the case where there are a ton of emails that don't have access
        # to the drive APIs. Without this, we could loop through these emails for
        # more than 3 hours, causing a timeout and stalling progress.
        email_batch_takes_us_to_completion = True
        MAX_EMAILS_TO_PROCESS_BEFORE_CHECKPOINTING = MAX_DRIVE_WORKERS
        if len(non_completed_org_emails) > MAX_EMAILS_TO_PROCESS_BEFORE_CHECKPOINTING:
            non_completed_org_emails = non_completed_org_emails[:MAX_EMAILS_TO_PROCESS_BEFORE_CHECKPOINTING]
            email_batch_takes_us_to_completion = False

        user_retrieval_gens = [
            self._impersonate_user_for_retrieval(
                email,
                field_type,
                checkpoint,
                drive_id_getter,
                sorted_folder_ids,
                start,
                end,
            )
            for email in non_completed_org_emails
        ]
        yield from parallel_yield(user_retrieval_gens, max_workers=MAX_DRIVE_WORKERS)

        # if there are more emails to process, don't mark as complete
        if not email_batch_takes_us_to_completion:
            return

        remaining_folders = (set(sorted_drive_ids) | set(sorted_folder_ids)) - self._retrieved_folder_and_drive_ids
        if remaining_folders:
            self.logger.warning(f"Some folders/drives were not retrieved. IDs: {remaining_folders}")
        if any(checkpoint.completion_map[user_email].stage != DriveRetrievalStage.DONE for user_email in all_org_emails):
            self.logger.info("some users did not complete retrieval, returning checkpoint for another run")
            return
        checkpoint.completion_stage = DriveRetrievalStage.DONE

    def _determine_retrieval_ids(
        self,
        checkpoint: GoogleDriveCheckpoint,
        next_stage: DriveRetrievalStage,
    ) -> tuple[list[str], list[str]]:
        all_drive_ids = self.get_all_drive_ids()
        sorted_drive_ids: list[str] = []
        sorted_folder_ids: list[str] = []
        if checkpoint.completion_stage == DriveRetrievalStage.DRIVE_IDS:
            if self._requested_shared_drive_ids or self._requested_folder_ids:
                (
                    sorted_drive_ids,
                    sorted_folder_ids,
                ) = _clean_requested_drive_ids(
                    requested_drive_ids=self._requested_shared_drive_ids,
                    requested_folder_ids=self._requested_folder_ids,
                    all_drive_ids_available=all_drive_ids,
                )
            elif self.include_shared_drives:
                sorted_drive_ids = sorted(all_drive_ids)

            checkpoint.drive_ids_to_retrieve = sorted_drive_ids
            checkpoint.folder_ids_to_retrieve = sorted_folder_ids
            checkpoint.completion_stage = next_stage
        else:
            if checkpoint.drive_ids_to_retrieve is None:
                raise ValueError("drive ids to retrieve not set in checkpoint")
            if checkpoint.folder_ids_to_retrieve is None:
                raise ValueError("folder ids to retrieve not set in checkpoint")
            # When loading from a checkpoint, load the previously cached drive and folder ids
            sorted_drive_ids = checkpoint.drive_ids_to_retrieve
            sorted_folder_ids = checkpoint.folder_ids_to_retrieve

        return sorted_drive_ids, sorted_folder_ids

    def _oauth_retrieval_drives(
        self,
        field_type: DriveFileFieldType,
        drive_service: GoogleDriveService,
        drive_ids_to_retrieve: list[str],
        checkpoint: GoogleDriveCheckpoint,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> Iterator[RetrievedDriveFile | str]:
        def _yield_from_drive(drive_id: str, drive_start: SecondsSinceUnixEpoch | None) -> Iterator[RetrievedDriveFile | str]:
            yield from add_retrieval_info(
                get_files_in_shared_drive(
                    service=drive_service,
                    drive_id=drive_id,
                    field_type=field_type,
                    max_num_pages=SHARED_DRIVE_PAGES_PER_CHECKPOINT,
                    cache_folders=not bool(drive_start),  # only cache folders for 0 or None
                    update_traversed_ids_func=self._update_traversed_parent_ids,
                    start=drive_start,
                    end=end,
                    page_token=checkpoint.completion_map[self.primary_admin_email].next_page_token,
                ),
                self.primary_admin_email,
                DriveRetrievalStage.SHARED_DRIVE_FILES,
                parent_id=drive_id,
            )

        # If we are resuming from a checkpoint, we need to finish retrieving the files from the last drive we retrieved
        if checkpoint.completion_map[self.primary_admin_email].stage == DriveRetrievalStage.SHARED_DRIVE_FILES:
            drive_id = checkpoint.completion_map[self.primary_admin_email].current_folder_or_drive_id
            if drive_id is None:
                raise ValueError("drive id not set in checkpoint")
            resume_start = checkpoint.completion_map[self.primary_admin_email].completed_until
            for file_or_token in _yield_from_drive(drive_id, resume_start):
                if isinstance(file_or_token, str):
                    checkpoint.completion_map[self.primary_admin_email].next_page_token = file_or_token
                    return  # done with the max num pages, return checkpoint
                yield file_or_token
            checkpoint.completion_map[self.primary_admin_email].next_page_token = None

        for drive_id in drive_ids_to_retrieve:
            if drive_id in self._retrieved_folder_and_drive_ids:
                self.logger.info(f"Skipping drive '{drive_id}' as it has already been retrieved")
                continue
            self.logger.info(f"Getting files in shared drive '{drive_id}' as '{self.primary_admin_email}'")
            for file_or_token in _yield_from_drive(drive_id, start):
                if isinstance(file_or_token, str):
                    checkpoint.completion_map[self.primary_admin_email].next_page_token = file_or_token
                    return  # done with the max num pages, return checkpoint
                yield file_or_token
            checkpoint.completion_map[self.primary_admin_email].next_page_token = None

    def _oauth_retrieval_folders(
        self,
        field_type: DriveFileFieldType,
        drive_service: GoogleDriveService,
        drive_ids_to_retrieve: set[str],
        folder_ids_to_retrieve: set[str],
        checkpoint: GoogleDriveCheckpoint,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> Iterator[RetrievedDriveFile]:
        """
        If there are any remaining folder ids to retrieve found earlier in the
        retrieval process, we recursively descend the file tree and retrieve all
        files in the folder(s).
        """
        # Even if no folders were requested, we still check if any drives were requested
        # that could be folders.
        remaining_folders = folder_ids_to_retrieve - self._retrieved_folder_and_drive_ids

        def _yield_from_folder_crawl(folder_id: str, folder_start: SecondsSinceUnixEpoch | None) -> Iterator[RetrievedDriveFile]:
            yield from crawl_folders_for_files(
                service=drive_service,
                parent_id=folder_id,
                field_type=field_type,
                user_email=self.primary_admin_email,
                traversed_parent_ids=self._retrieved_folder_and_drive_ids,
                update_traversed_ids_func=self._update_traversed_parent_ids,
                start=folder_start,
                end=end,
            )

        # resume from a checkpoint
        # TODO: actually checkpoint folder retrieval. Since we moved towards returning from
        # generator functions to indicate when a checkpoint should be returned, this code
        # shouldn't be used currently. Unfortunately folder crawling is quite difficult to checkpoint
        # effectively (likely need separate folder crawling and file retrieval stages),
        # so we'll revisit this later.
        if checkpoint.completion_map[self.primary_admin_email].stage == DriveRetrievalStage.FOLDER_FILES and (
            folder_id := checkpoint.completion_map[self.primary_admin_email].current_folder_or_drive_id
        ):
            resume_start = checkpoint.completion_map[self.primary_admin_email].completed_until
            yield from _yield_from_folder_crawl(folder_id, resume_start)

        # the times stored in the completion_map aren't used due to the crawling behavior
        # instead, the traversed_parent_ids are used to determine what we have left to retrieve
        for folder_id in remaining_folders:
            self.logger.info(f"Getting files in folder '{folder_id}' as '{self.primary_admin_email}'")
            yield from _yield_from_folder_crawl(folder_id, start)

        remaining_folders = (drive_ids_to_retrieve | folder_ids_to_retrieve) - self._retrieved_folder_and_drive_ids
        if remaining_folders:
            self.logger.warning(f"Some folders/drives were not retrieved. IDs: {remaining_folders}")

    def _load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: GoogleDriveCheckpoint,
        include_permissions: bool,
    ) -> CheckpointOutput:
        """
        Entrypoint for the connector; first run is with an empty checkpoint.
        """
        if self._creds is None or self._primary_admin_email is None:
            raise RuntimeError("Credentials missing, should not call this method before calling load_credentials")

        self.logger.info(f"Loading from checkpoint with completion stage: {checkpoint.completion_stage},num retrieved ids: {len(checkpoint.all_retrieved_file_ids)}")
        checkpoint = copy.deepcopy(checkpoint)
        self._retrieved_folder_and_drive_ids = checkpoint.retrieved_folder_and_drive_ids
        try:
            yield from self._extract_docs_from_google_drive(checkpoint, start, end, include_permissions)
        except Exception as e:
            if MISSING_SCOPES_ERROR_STR in str(e):
                raise PermissionError() from e
            raise e
        checkpoint.retrieved_folder_and_drive_ids = self._retrieved_folder_and_drive_ids

        self.logger.info(f"num drive files retrieved: {len(checkpoint.all_retrieved_file_ids)}")
        if checkpoint.completion_stage == DriveRetrievalStage.DONE:
            checkpoint.has_more = False
        return checkpoint

    def _checkpointed_retrieval(
        self,
        retrieval_method: CredentialedRetrievalMethod,
        field_type: DriveFileFieldType,
        checkpoint: GoogleDriveCheckpoint,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> Iterator[RetrievedDriveFile]:
        drive_files = retrieval_method(
            field_type=field_type,
            checkpoint=checkpoint,
            start=start,
            end=end,
        )

        for file in drive_files:
            document_id = onyx_document_id_from_drive_file(file.drive_file)
            logging.debug(f"Updating checkpoint for file: {file.drive_file.get('name')}. Seen: {document_id in checkpoint.all_retrieved_file_ids}")
            checkpoint.completion_map[file.user_email].update(
                stage=file.completion_stage,
                completed_until=datetime_from_string(file.drive_file[GoogleFields.MODIFIED_TIME.value]).timestamp(),
                current_folder_or_drive_id=file.parent_id,
            )
            if document_id not in checkpoint.all_retrieved_file_ids:
                checkpoint.all_retrieved_file_ids.add(document_id)
                yield file

    def _oauth_retrieval_all_files(
        self,
        field_type: DriveFileFieldType,
        drive_service: GoogleDriveService,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
        page_token: str | None = None,
    ) -> Iterator[RetrievedDriveFile | str]:
        if not self.include_files_shared_with_me and not self.include_my_drives:
            return

        self.logger.info(
            f"Getting shared files/my drive files for OAuth "
            f"with include_files_shared_with_me={self.include_files_shared_with_me}, "
            f"include_my_drives={self.include_my_drives}, "
            f"include_shared_drives={self.include_shared_drives}."
            f"Using '{self.primary_admin_email}' as the account."
        )
        yield from add_retrieval_info(
            get_all_files_for_oauth(
                service=drive_service,
                include_files_shared_with_me=self.include_files_shared_with_me,
                include_my_drives=self.include_my_drives,
                include_shared_drives=self.include_shared_drives,
                field_type=field_type,
                max_num_pages=OAUTH_PAGES_PER_CHECKPOINT,
                start=start,
                end=end,
                page_token=page_token,
            ),
            self.primary_admin_email,
            DriveRetrievalStage.OAUTH_FILES,
        )

    def _manage_oauth_retrieval(
        self,
        field_type: DriveFileFieldType,
        checkpoint: GoogleDriveCheckpoint,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> Iterator[RetrievedDriveFile]:
        if checkpoint.completion_stage == DriveRetrievalStage.START:
            checkpoint.completion_stage = DriveRetrievalStage.OAUTH_FILES
            checkpoint.completion_map[self.primary_admin_email] = StageCompletion(
                stage=DriveRetrievalStage.START,
                completed_until=0,
                current_folder_or_drive_id=None,
            )

        drive_service = get_drive_service(self.creds, self.primary_admin_email)

        if checkpoint.completion_stage == DriveRetrievalStage.OAUTH_FILES:
            completion = checkpoint.completion_map[self.primary_admin_email]
            all_files_start = start
            # if resuming from a checkpoint
            if completion.stage == DriveRetrievalStage.OAUTH_FILES:
                all_files_start = completion.completed_until

            for file_or_token in self._oauth_retrieval_all_files(
                field_type=field_type,
                drive_service=drive_service,
                start=all_files_start,
                end=end,
                page_token=checkpoint.completion_map[self.primary_admin_email].next_page_token,
            ):
                if isinstance(file_or_token, str):
                    checkpoint.completion_map[self.primary_admin_email].next_page_token = file_or_token
                    return  # done with the max num pages, return checkpoint
                yield file_or_token
            checkpoint.completion_stage = DriveRetrievalStage.DRIVE_IDS
            checkpoint.completion_map[self.primary_admin_email].next_page_token = None
            return  # create a new checkpoint

        all_requested = self.include_files_shared_with_me and self.include_my_drives and self.include_shared_drives
        if all_requested:
            # If all 3 are true, we already yielded from get_all_files_for_oauth
            checkpoint.completion_stage = DriveRetrievalStage.DONE
            return

        sorted_drive_ids, sorted_folder_ids = self._determine_retrieval_ids(checkpoint, DriveRetrievalStage.SHARED_DRIVE_FILES)

        if checkpoint.completion_stage == DriveRetrievalStage.SHARED_DRIVE_FILES:
            for file_or_token in self._oauth_retrieval_drives(
                field_type=field_type,
                drive_service=drive_service,
                drive_ids_to_retrieve=sorted_drive_ids,
                checkpoint=checkpoint,
                start=start,
                end=end,
            ):
                if isinstance(file_or_token, str):
                    checkpoint.completion_map[self.primary_admin_email].next_page_token = file_or_token
                    return  # done with the max num pages, return checkpoint
                yield file_or_token
            checkpoint.completion_stage = DriveRetrievalStage.FOLDER_FILES
            checkpoint.completion_map[self.primary_admin_email].next_page_token = None
            return  # create a new checkpoint

        if checkpoint.completion_stage == DriveRetrievalStage.FOLDER_FILES:
            yield from self._oauth_retrieval_folders(
                field_type=field_type,
                drive_service=drive_service,
                drive_ids_to_retrieve=set(sorted_drive_ids),
                folder_ids_to_retrieve=set(sorted_folder_ids),
                checkpoint=checkpoint,
                start=start,
                end=end,
            )

        checkpoint.completion_stage = DriveRetrievalStage.DONE

    def _fetch_drive_items(
        self,
        field_type: DriveFileFieldType,
        checkpoint: GoogleDriveCheckpoint,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> Iterator[RetrievedDriveFile]:
        retrieval_method = self._manage_service_account_retrieval if isinstance(self.creds, ServiceAccountCredentials) else self._manage_oauth_retrieval

        return self._checkpointed_retrieval(
            retrieval_method=retrieval_method,
            field_type=field_type,
            checkpoint=checkpoint,
            start=start,
            end=end,
        )

    def _extract_docs_from_google_drive(
        self,
        checkpoint: GoogleDriveCheckpoint,
        start: SecondsSinceUnixEpoch | None,
        end: SecondsSinceUnixEpoch | None,
        include_permissions: bool,
    ) -> Iterator[Document | ConnectorFailure]:
        """
        Retrieves and converts Google Drive files to documents.
        """
        field_type = DriveFileFieldType.WITH_PERMISSIONS if include_permissions else DriveFileFieldType.STANDARD

        try:
            # Prepare a partial function with the credentials and admin email
            convert_func = partial(
                convert_drive_item_to_document,
                self.creds,
                self.allow_images,
                self.size_threshold,
                (
                    PermissionSyncContext(
                        primary_admin_email=self.primary_admin_email,
                        google_domain=self.google_domain,
                    )
                    if include_permissions
                    else None
                ),
            )
            # Fetch files in batches
            batches_complete = 0
            files_batch: list[RetrievedDriveFile] = []

            def _yield_batch(
                files_batch: list[RetrievedDriveFile],
            ) -> Iterator[Document | ConnectorFailure]:
                nonlocal batches_complete
                # Process the batch using run_functions_tuples_in_parallel
                func_with_args = [
                    (
                        convert_func,
                        (
                            [file.user_email, self.primary_admin_email] + get_file_owners(file.drive_file, self.primary_admin_email),
                            file.drive_file,
                        ),
                    )
                    for file in files_batch
                ]
                results = cast(
                    list[Document | ConnectorFailure | None],
                    run_functions_tuples_in_parallel(func_with_args, max_workers=8),
                )
                self.logger.debug(f"finished processing batch {batches_complete} with {len(results)} results")

                docs_and_failures = [result for result in results if result is not None]
                self.logger.debug(f"batch {batches_complete} has {len(docs_and_failures)} docs or failures")

                if docs_and_failures:
                    yield from docs_and_failures
                    batches_complete += 1
                self.logger.debug(f"finished yielding batch {batches_complete}")

            for retrieved_file in self._fetch_drive_items(
                field_type=field_type,
                checkpoint=checkpoint,
                start=start,
                end=end,
            ):
                if retrieved_file.error is None:
                    files_batch.append(retrieved_file)
                    continue

                # handle retrieval errors
                failure_stage = retrieved_file.completion_stage.value
                failure_message = f"retrieval failure during stage: {failure_stage},"
                failure_message += f"user: {retrieved_file.user_email},"
                failure_message += f"parent drive/folder: {retrieved_file.parent_id},"
                failure_message += f"error: {retrieved_file.error}"
                self.logger.error(failure_message)
                yield ConnectorFailure(
                    failed_entity=EntityFailure(
                        entity_id=failure_stage,
                    ),
                    failure_message=failure_message,
                    exception=retrieved_file.error,
                )

            yield from _yield_batch(files_batch)
            checkpoint.retrieved_folder_and_drive_ids = self._retrieved_folder_and_drive_ids

        except Exception as e:
            self.logger.exception(f"Error extracting documents from Google Drive: {e}")
            raise e

    @override
    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: GoogleDriveCheckpoint,
    ) -> CheckpointOutput:
        return self._load_from_checkpoint(start, end, checkpoint, include_permissions=False)

    @override
    def load_from_checkpoint_with_perm_sync(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: GoogleDriveCheckpoint,
    ) -> CheckpointOutput:
        return self._load_from_checkpoint(start, end, checkpoint, include_permissions=True)

    def _extract_slim_docs_from_google_drive(
        self,
        checkpoint: GoogleDriveCheckpoint,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
        callback: IndexingHeartbeatInterface | None = None,
    ) -> GenerateSlimDocumentOutput:
        slim_batch = []
        for file in self._fetch_drive_items(
            field_type=DriveFileFieldType.SLIM,
            checkpoint=checkpoint,
            start=start,
            end=end,
        ):
            if file.error is not None:
                raise file.error
            if doc := build_slim_document(
                self.creds,
                file.drive_file,
                # for now, always fetch permissions for slim runs
                # TODO: move everything to load_from_checkpoint
                # and only fetch permissions if needed
                PermissionSyncContext(
                    primary_admin_email=self.primary_admin_email,
                    google_domain=self.google_domain,
                ),
            ):
                slim_batch.append(doc)
            if len(slim_batch) >= SLIM_BATCH_SIZE:
                yield slim_batch
                slim_batch = []
                if callback:
                    if callback.should_stop():
                        raise RuntimeError("_extract_slim_docs_from_google_drive: Stop signal detected")
                    callback.progress("_extract_slim_docs_from_google_drive", 1)
        yield slim_batch

    def retrieve_all_slim_docs_perm_sync(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
        callback: IndexingHeartbeatInterface | None = None,
    ) -> GenerateSlimDocumentOutput:
        try:
            checkpoint = self.build_dummy_checkpoint()
            while checkpoint.completion_stage != DriveRetrievalStage.DONE:
                yield from self._extract_slim_docs_from_google_drive(
                    checkpoint=checkpoint,
                    start=start,
                    end=end,
                )
            self.logger.info("Drive perm sync: Slim doc retrieval complete")

        except Exception as e:
            if MISSING_SCOPES_ERROR_STR in str(e):
                raise PermissionError() from e
            raise e

    def validate_connector_settings(self) -> None:
        if self._creds is None:
            raise ConnectorMissingCredentialError("Google Drive credentials not loaded.")

        if self._primary_admin_email is None:
            raise ConnectorValidationError("Primary admin email not found in credentials. Ensure DB_CREDENTIALS_PRIMARY_ADMIN_KEY is set.")

        try:
            drive_service = get_drive_service(self._creds, self._primary_admin_email)
            drive_service.files().list(pageSize=1, fields="files(id)").execute()

            if isinstance(self._creds, ServiceAccountCredentials):
                # default is ~17mins of retries, don't do that here since this is called from
                # the UI
                get_root_folder_id(drive_service)

        except HttpError as e:
            status_code = e.resp.status if e.resp else None
            if status_code == 401:
                raise CredentialExpiredError("Invalid or expired Google Drive credentials (401).")
            elif status_code == 403:
                raise InsufficientPermissionsError("Google Drive app lacks required permissions (403). Please ensure the necessary scopes are granted and Drive apps are enabled.")
            else:
                raise ConnectorValidationError(f"Unexpected Google Drive error (status={status_code}): {e}")

        except Exception as e:
            # Check for scope-related hints from the error message
            if MISSING_SCOPES_ERROR_STR in str(e):
                raise InsufficientPermissionsError("Google Drive credentials are missing required scopes.")
            raise ConnectorValidationError(f"Unexpected error during Google Drive validation: {e}")

    @override
    def build_dummy_checkpoint(self) -> GoogleDriveCheckpoint:
        return GoogleDriveCheckpoint(
            retrieved_folder_and_drive_ids=set(),
            completion_stage=DriveRetrievalStage.START,
            completion_map=ThreadSafeDict(),
            all_retrieved_file_ids=set(),
            has_more=True,
        )

    @override
    def validate_checkpoint_json(self, checkpoint_json: str) -> GoogleDriveCheckpoint:
        return GoogleDriveCheckpoint.model_validate_json(checkpoint_json)


class CheckpointOutputWrapper:
    """
    Wraps a CheckpointOutput generator to give things back in a more digestible format.
    The connector format is easier for the connector implementor (e.g. it enforces exactly
    one new checkpoint is returned AND that the checkpoint is at the end), thus the different
    formats.
    """

    def __init__(self) -> None:
        self.next_checkpoint: GoogleDriveCheckpoint | None = None

    def __call__(
        self,
        checkpoint_connector_generator: CheckpointOutput,
    ) -> Generator[
        tuple[Document | None, ConnectorFailure | None, GoogleDriveCheckpoint | None],
        None,
        None,
    ]:
        # grabs the final return value and stores it in the `next_checkpoint` variable
        def _inner_wrapper(
            checkpoint_connector_generator: CheckpointOutput,
        ) -> CheckpointOutput:
            self.next_checkpoint = yield from checkpoint_connector_generator
            return self.next_checkpoint  # not used

        for document_or_failure in _inner_wrapper(checkpoint_connector_generator):
            if isinstance(document_or_failure, Document):
                yield document_or_failure, None, None
            elif isinstance(document_or_failure, ConnectorFailure):
                yield None, document_or_failure, None
            else:
                raise ValueError(f"Invalid document_or_failure type: {type(document_or_failure)}")

        if self.next_checkpoint is None:
            raise RuntimeError("Checkpoint is None. This should never happen - the connector should always return a checkpoint.")

        yield None, None, self.next_checkpoint


def yield_all_docs_from_checkpoint_connector(
    connector: GoogleDriveConnector,
    start: SecondsSinceUnixEpoch,
    end: SecondsSinceUnixEpoch,
) -> Iterator[Document | ConnectorFailure]:
    num_iterations = 0

    checkpoint = connector.build_dummy_checkpoint()
    while checkpoint.has_more:
        doc_batch_generator = CheckpointOutputWrapper()(connector.load_from_checkpoint(start, end, checkpoint))
        for document, failure, next_checkpoint in doc_batch_generator:
            if failure is not None:
                yield failure
            if document is not None:
                yield document
            if next_checkpoint is not None:
                checkpoint = next_checkpoint

        num_iterations += 1
        if num_iterations > 100_000:
            raise RuntimeError("Too many iterations. Infinite loop?")


if __name__ == "__main__":
    import time
    from common.data_source.google_util.util import get_credentials_from_env
    logging.basicConfig(level=logging.DEBUG)

    try:
        # Get credentials from environment
        email = os.environ.get("GOOGLE_DRIVE_PRIMARY_ADMIN_EMAIL", "yongtengrey@gmail.com")
        creds = get_credentials_from_env(email, oauth=True)
        print("Credentials loaded successfully")
        print(f"{creds=}")
        # sys.exit(0)
        connector = GoogleDriveConnector(
            include_shared_drives=False,
            shared_drive_urls=None,
            include_my_drives=True,
            my_drive_emails="yongtengrey@gmail.com",
            shared_folder_urls="https://drive.google.com/drive/folders/1fAKwbmf3U2oM139ZmnOzgIZHGkEwnpfy",
            include_files_shared_with_me=False,
            specific_user_emails=None,
        )
        print("GoogleDriveConnector initialized successfully")
        connector.load_credentials(creds)
        print("Credentials loaded into connector successfully")

        print("Google Drive connector is ready to use!")
        max_fsize = 0
        biggest_fsize = 0
        num_errors = 0
        docs_processed = 0
        start_time = time.time()
        with open("stats.txt", "w") as f:
            for num, doc_or_failure in enumerate(yield_all_docs_from_checkpoint_connector(connector, 0, time.time())):
                if num % 200 == 0:
                    f.write(f"Processed {num} files\n")
                    f.write(f"Max file size: {max_fsize / 1000_000:.2f} MB\n")
                    f.write(f"Time so far: {time.time() - start_time:.2f} seconds\n")
                    f.write(f"Docs per minute: {num / (time.time() - start_time) * 60:.2f}\n")
                    biggest_fsize = max(biggest_fsize, max_fsize)
                if isinstance(doc_or_failure, Document):
                    docs_processed += 1
                    max_fsize = max(max_fsize, doc_or_failure.size_bytes)
                    print(f"{doc_or_failure=}")
                elif isinstance(doc_or_failure, ConnectorFailure):
                    num_errors += 1
            print(f"Num errors: {num_errors}")
            print(f"Biggest file size: {biggest_fsize / 1000_000:.2f} MB")
            print(f"Time taken: {time.time() - start_time:.2f} seconds")
            print(f"Total documents produced: {docs_processed}")

    except Exception as e:
        print(f"Error: {e}")
        import traceback

        traceback.print_exc()
        sys.exit(1)

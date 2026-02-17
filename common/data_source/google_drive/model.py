from enum import Enum
from typing import Any

from pydantic import BaseModel, ConfigDict, field_serializer, field_validator

from common.data_source.google_util.util_threadpool_concurrency import ThreadSafeDict
from common.data_source.models import ConnectorCheckpoint, SecondsSinceUnixEpoch

GoogleDriveFileType = dict[str, Any]


class GDriveMimeType(str, Enum):
    DOC = "application/vnd.google-apps.document"
    SPREADSHEET = "application/vnd.google-apps.spreadsheet"
    SPREADSHEET_OPEN_FORMAT = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
    SPREADSHEET_MS_EXCEL = "application/vnd.ms-excel"
    PDF = "application/pdf"
    WORD_DOC = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
    PPT = "application/vnd.google-apps.presentation"
    POWERPOINT = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
    PLAIN_TEXT = "text/plain"
    MARKDOWN = "text/markdown"


# These correspond to The major stages of retrieval for Google Drive.
# The stages for the oauth flow are:
# get_all_files_for_oauth(),
# get_all_drive_ids(),
# get_files_in_shared_drive(),
# crawl_folders_for_files()
#
# The stages for the service account flow are roughly:
# get_all_user_emails(),
# get_all_drive_ids(),
# get_files_in_shared_drive(),
# Then for each user:
#   get_files_in_my_drive()
#   get_files_in_shared_drive()
#   crawl_folders_for_files()
class DriveRetrievalStage(str, Enum):
    START = "start"
    DONE = "done"
    # OAuth specific stages
    OAUTH_FILES = "oauth_files"

    # Service account specific stages
    USER_EMAILS = "user_emails"
    MY_DRIVE_FILES = "my_drive_files"

    # Used for both oauth and service account flows
    DRIVE_IDS = "drive_ids"
    SHARED_DRIVE_FILES = "shared_drive_files"
    FOLDER_FILES = "folder_files"


class StageCompletion(BaseModel):
    """
    Describes the point in the retrieval+indexing process that the
    connector is at. completed_until is the timestamp of the latest
    file that has been retrieved or error that has been yielded.
    Optional fields are used for retrieval stages that need more information
    for resuming than just the timestamp of the latest file.
    """

    stage: DriveRetrievalStage
    completed_until: SecondsSinceUnixEpoch
    current_folder_or_drive_id: str | None = None
    next_page_token: str | None = None

    # only used for shared drives
    processed_drive_ids: set[str] = set()

    def update(
        self,
        stage: DriveRetrievalStage,
        completed_until: SecondsSinceUnixEpoch,
        current_folder_or_drive_id: str | None = None,
    ) -> None:
        self.stage = stage
        self.completed_until = completed_until
        self.current_folder_or_drive_id = current_folder_or_drive_id


class GoogleDriveCheckpoint(ConnectorCheckpoint):
    # Checkpoint version of _retrieved_ids
    retrieved_folder_and_drive_ids: set[str]

    # Describes the point in the retrieval+indexing process that the
    # checkpoint is at. when this is set to a given stage, the connector
    # has finished yielding all values from the previous stage.
    completion_stage: DriveRetrievalStage

    # The latest timestamp of a file that has been retrieved per user email.
    # StageCompletion is used to track the completion of each stage, but the
    # timestamp part is not used for folder crawling.
    completion_map: ThreadSafeDict[str, StageCompletion]

    # all file ids that have been retrieved
    all_retrieved_file_ids: set[str] = set()

    # cached version of the drive and folder ids to retrieve
    drive_ids_to_retrieve: list[str] | None = None
    folder_ids_to_retrieve: list[str] | None = None

    # cached user emails
    user_emails: list[str] | None = None

    @field_serializer("completion_map")
    def serialize_completion_map(self, completion_map: ThreadSafeDict[str, StageCompletion], _info: Any) -> dict[str, StageCompletion]:
        return completion_map._dict

    @field_validator("completion_map", mode="before")
    def validate_completion_map(cls, v: Any) -> ThreadSafeDict[str, StageCompletion]:
        assert isinstance(v, dict) or isinstance(v, ThreadSafeDict)
        return ThreadSafeDict({k: StageCompletion.model_validate(val) for k, val in v.items()})


class RetrievedDriveFile(BaseModel):
    """
    Describes a file that has been retrieved from Google Drive.
    user_email is the email of the user that the file was retrieved
    by impersonating. If an error worthy of being reported is encountered,
    error should be set and later propagated as a ConnectorFailure.
    """

    # The stage at which this file was retrieved
    completion_stage: DriveRetrievalStage

    # The file that was retrieved
    drive_file: GoogleDriveFileType

    # The email of the user that the file was retrieved by impersonating
    user_email: str

    # The id of the parent folder or drive of the file
    parent_id: str | None = None

    # Any unexpected error that occurred while retrieving the file.
    # In particular, this is not used for 403/404 errors, which are expected
    # in the context of impersonating all the users to try to retrieve all
    # files from all their Drives and Folders.
    error: Exception | None = None

    model_config = ConfigDict(arbitrary_types_allowed=True)

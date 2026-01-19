import fnmatch
import itertools
from collections import deque
from collections.abc import Iterable
from collections.abc import Iterator
from datetime import datetime
from datetime import timezone
from typing import Any
from typing import TypeVar
import gitlab
from gitlab.v4.objects import Project

from common.data_source.config import DocumentSource, INDEX_BATCH_SIZE
from common.data_source.exceptions import ConnectorMissingCredentialError
from common.data_source.exceptions import ConnectorValidationError
from common.data_source.exceptions import CredentialExpiredError
from common.data_source.exceptions import InsufficientPermissionsError
from common.data_source.exceptions import UnexpectedValidationError
from common.data_source.interfaces import GenerateDocumentsOutput
from common.data_source.interfaces import LoadConnector
from common.data_source.interfaces import PollConnector
from common.data_source.interfaces import SecondsSinceUnixEpoch
from common.data_source.models import BasicExpertInfo
from common.data_source.models import Document
from common.data_source.utils import get_file_ext

T = TypeVar("T")



# List of directories/Files to exclude
exclude_patterns = [
    "logs",
    ".github/",
    ".gitlab/",
    ".pre-commit-config.yaml",
]


def _batch_gitlab_objects(git_objs: Iterable[T], batch_size: int) -> Iterator[list[T]]:
    it = iter(git_objs)
    while True:
        batch = list(itertools.islice(it, batch_size))
        if not batch:
            break
        yield batch


def get_author(author: Any) -> BasicExpertInfo:
    return BasicExpertInfo(
        display_name=author.get("name"),
    )


def _convert_merge_request_to_document(mr: Any) -> Document:
    mr_text = mr.description or ""
    doc = Document(
        id=mr.web_url,
        blob=mr_text,
        source=DocumentSource.GITLAB,
        semantic_identifier=mr.title,
        extension=".md",
        # updated_at is UTC time but is timezone unaware, explicitly add UTC
        # as there is logic in indexing to prevent wrong timestamped docs
        # due to local time discrepancies with UTC
        doc_updated_at=mr.updated_at.replace(tzinfo=timezone.utc),
        size_bytes=len(mr_text.encode("utf-8")),
        primary_owners=[get_author(mr.author)],
        metadata={"state": mr.state, "type": "MergeRequest", "web_url": mr.web_url},
    )
    return doc


def _convert_issue_to_document(issue: Any) -> Document:
    issue_text = issue.description or ""
    doc = Document(
        id=issue.web_url,
        blob=issue_text,
        source=DocumentSource.GITLAB,
        semantic_identifier=issue.title,
        extension=".md",
        # updated_at is UTC time but is timezone unaware, explicitly add UTC
        # as there is logic in indexing to prevent wrong timestamped docs
        # due to local time discrepancies with UTC
        doc_updated_at=issue.updated_at.replace(tzinfo=timezone.utc),
        size_bytes=len(issue_text.encode("utf-8")),
        primary_owners=[get_author(issue.author)],
        metadata={
            "state": issue.state,
            "type": issue.type if issue.type else "Issue",
            "web_url": issue.web_url,
        },
    )
    return doc


def _convert_code_to_document(
    project: Project, file: Any, url: str, projectName: str, projectOwner: str
) -> Document:
    
    # Dynamically get the default branch from the project object
    default_branch = project.default_branch

    # Fetch the file content using the correct branch
    file_content_obj = project.files.get(
        file_path=file["path"], ref=default_branch  # Use the default branch
    )
    # BoxConnector uses raw bytes for blob. Keep the same here.
    file_content_bytes = file_content_obj.decode()
    file_url = f"{url}/{projectOwner}/{projectName}/-/blob/{default_branch}/{file['path']}"

    # Try to use the last commit timestamp for incremental sync.
    # Falls back to "now" if the commit lookup fails.
    last_commit_at = None
    try:
        # Query commit history for this file on the default branch.
        commits = project.commits.list(
            ref_name=default_branch,
            path=file["path"],
            per_page=1,
        )
        if commits:
            # committed_date is ISO string like "2024-01-01T00:00:00.000+00:00"
            committed_date = commits[0].committed_date
            if isinstance(committed_date, str):
                last_commit_at = datetime.strptime(
                    committed_date, "%Y-%m-%dT%H:%M:%S.%f%z"
                ).astimezone(timezone.utc)
            elif isinstance(committed_date, datetime):
                last_commit_at = committed_date.astimezone(timezone.utc)
    except Exception:
        last_commit_at = None

    # Create and return a Document object
    doc = Document(
        # Use a stable ID so reruns don't create duplicates.
        id=file_url,
        blob=file_content_bytes,
        source=DocumentSource.GITLAB,
        semantic_identifier=file.get("name"),
        extension=get_file_ext(file.get("name")),
        doc_updated_at=last_commit_at or datetime.now(tz=timezone.utc),
        size_bytes=len(file_content_bytes) if file_content_bytes is not None else 0,
        primary_owners=[],  # Add owners if needed
        metadata={
            "type": "CodeFile",
            "path": file.get("path"),
            "ref": default_branch,
            "project": f"{projectOwner}/{projectName}",
            "web_url": file_url,
        },
    )
    return doc


def _should_exclude(path: str) -> bool:
    """Check if a path matches any of the exclude patterns."""
    return any(fnmatch.fnmatch(path, pattern) for pattern in exclude_patterns)


class GitlabConnector(LoadConnector, PollConnector):
    def __init__(
        self,
        project_owner: str,
        project_name: str,
        batch_size: int = INDEX_BATCH_SIZE,
        state_filter: str = "all",
        include_mrs: bool = True,
        include_issues: bool = True,
        include_code_files: bool = False,
    ) -> None:
        self.project_owner = project_owner
        self.project_name = project_name
        self.batch_size = batch_size
        self.state_filter = state_filter
        self.include_mrs = include_mrs
        self.include_issues = include_issues
        self.include_code_files = include_code_files
        self.gitlab_client: gitlab.Gitlab | None = None

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self.gitlab_client = gitlab.Gitlab(
            credentials["gitlab_url"], private_token=credentials["gitlab_access_token"]
        )
        return None

    def validate_connector_settings(self) -> None:
        if self.gitlab_client is None:
            raise ConnectorMissingCredentialError("GitLab")

        try:
            self.gitlab_client.auth()
            self.gitlab_client.projects.get(
                f"{self.project_owner}/{self.project_name}",
                lazy=True,
            )

        except gitlab.exceptions.GitlabAuthenticationError as e:
            raise CredentialExpiredError(
                "Invalid or expired GitLab credentials."
            ) from e

        except gitlab.exceptions.GitlabAuthorizationError as e:
            raise InsufficientPermissionsError(
                "Insufficient permissions to access GitLab resources."
            ) from e

        except gitlab.exceptions.GitlabGetError as e:
            raise ConnectorValidationError(
                "GitLab project not found or not accessible."
            ) from e

        except Exception as e:
            raise UnexpectedValidationError(
                f"Unexpected error while validating GitLab settings: {e}"
            ) from e

    def _fetch_from_gitlab(
        self, start: datetime | None = None, end: datetime | None = None
    ) -> GenerateDocumentsOutput:
        if self.gitlab_client is None:
            raise ConnectorMissingCredentialError("Gitlab")
        project: Project = self.gitlab_client.projects.get(
            f"{self.project_owner}/{self.project_name}"
        )

        start_utc = start.astimezone(timezone.utc) if start else None
        end_utc = end.astimezone(timezone.utc) if end else None

        # Fetch code files
        if self.include_code_files:
            # Fetching using BFS as project.report_tree with recursion causing slow load
            queue = deque([""])  # Start with the root directory
            while queue:
                current_path = queue.popleft()
                files = project.repository_tree(path=current_path, all=True)
                for file_batch in _batch_gitlab_objects(files, self.batch_size):
                    code_doc_batch: list[Document] = []
                    for file in file_batch:
                        if _should_exclude(file["path"]):
                            continue

                        if file["type"] == "blob":

                            doc = _convert_code_to_document(
                                project,
                                file,
                                self.gitlab_client.url,
                                self.project_name,
                                self.project_owner,
                            )

                            # Apply incremental window filtering for code files too.
                            if start_utc is not None and doc.doc_updated_at <= start_utc:
                                continue
                            if end_utc is not None and doc.doc_updated_at > end_utc:
                                continue

                            code_doc_batch.append(doc)
                        elif file["type"] == "tree":
                            queue.append(file["path"])

                    if code_doc_batch:
                        yield code_doc_batch

        if self.include_mrs:
            merge_requests = project.mergerequests.list(
                state=self.state_filter,
                order_by="updated_at",
                sort="desc",
                iterator=True,
            )

            for mr_batch in _batch_gitlab_objects(merge_requests, self.batch_size):
                mr_doc_batch: list[Document] = []
                for mr in mr_batch:
                    mr.updated_at = datetime.strptime(
                        mr.updated_at, "%Y-%m-%dT%H:%M:%S.%f%z"
                    )
                    if start_utc is not None and mr.updated_at <= start_utc:
                        yield mr_doc_batch
                        return
                    if end_utc is not None and mr.updated_at > end_utc:
                        continue
                    mr_doc_batch.append(_convert_merge_request_to_document(mr))
                yield mr_doc_batch

        if self.include_issues:
            issues = project.issues.list(state=self.state_filter, iterator=True)

            for issue_batch in _batch_gitlab_objects(issues, self.batch_size):
                issue_doc_batch: list[Document] = []
                for issue in issue_batch:
                    issue.updated_at = datetime.strptime(
                        issue.updated_at, "%Y-%m-%dT%H:%M:%S.%f%z"
                    )
                    # Avoid re-syncing the last-seen item.
                    if start_utc is not None and issue.updated_at <= start_utc:
                        yield issue_doc_batch
                        return
                    if end_utc is not None and issue.updated_at > end_utc:
                        continue
                    issue_doc_batch.append(_convert_issue_to_document(issue))
                yield issue_doc_batch

    def load_from_state(self) -> GenerateDocumentsOutput:
        return self._fetch_from_gitlab()

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> GenerateDocumentsOutput:
        start_datetime = datetime.fromtimestamp(start, tz=timezone.utc)
        end_datetime = datetime.fromtimestamp(end, tz=timezone.utc)
        return self._fetch_from_gitlab(start_datetime, end_datetime)


if __name__ == "__main__":
    import os

    connector = GitlabConnector(
        # gitlab_url="https://gitlab.com/api/v4",
        project_owner=os.environ["PROJECT_OWNER"],
        project_name=os.environ["PROJECT_NAME"],
        batch_size=INDEX_BATCH_SIZE,
        state_filter="all",
        include_mrs=True,
        include_issues=True,
        include_code_files=True,
    )

    connector.load_credentials(
        {
            "gitlab_access_token": os.environ["GITLAB_ACCESS_TOKEN"],
            "gitlab_url": os.environ["GITLAB_URL"],
        }
    )
    document_batches = connector.load_from_state()
    for f in document_batches:
        print("Batch:", f)
    print("Finished loading from state.")
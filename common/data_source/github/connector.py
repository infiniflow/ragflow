import copy
import logging
from collections.abc import Callable
from collections.abc import Generator
from datetime import datetime
from datetime import timedelta
from datetime import timezone
from enum import Enum
from typing import Any
from typing import cast

from github import Github, Auth
from github import RateLimitExceededException
from github import Repository
from github.GithubException import GithubException
from github.Issue import Issue
from github.NamedUser import NamedUser
from github.PaginatedList import PaginatedList
from github.PullRequest import PullRequest
from pydantic import BaseModel
from typing_extensions import override
from common.data_source.utils import sanitize_filename
from common.data_source.config import DocumentSource, GITHUB_CONNECTOR_BASE_URL
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError,
    UnexpectedValidationError,
)
from common.data_source.interfaces import CheckpointedConnectorWithPermSyncGH, CheckpointOutput
from common.data_source.models import (
    ConnectorCheckpoint,
    ConnectorFailure,
    Document,
    DocumentFailure,
    ExternalAccess,
    SecondsSinceUnixEpoch,
)
from common.data_source.connector_runner import ConnectorRunner
from .models import SerializedRepository
from .rate_limit_utils import sleep_after_rate_limit_exception
from .utils import deserialize_repository
from .utils import get_external_access_permission

ITEMS_PER_PAGE = 100
CURSOR_LOG_FREQUENCY = 50

_MAX_NUM_RATE_LIMIT_RETRIES = 5

ONE_DAY = timedelta(days=1)
SLIM_BATCH_SIZE = 100
# Cases
# X (from start) standard run, no fallback to cursor-based pagination
# X (from start) standard run errors, fallback to cursor-based pagination
#  X error in the middle of a page
#  X no errors: run to completion
# X (from checkpoint) standard run, no fallback to cursor-based pagination
# X (from checkpoint) continue from cursor-based pagination
#  - retrying
#  - no retrying

# things to check:
# checkpoint state on return
# checkpoint progress (no infinite loop)


class DocMetadata(BaseModel):
    repo: str


def get_nextUrl_key(pag_list: PaginatedList[PullRequest | Issue]) -> str:
    if "_PaginatedList__nextUrl" in pag_list.__dict__:
        return "_PaginatedList__nextUrl"
    for key in pag_list.__dict__:
        if "__nextUrl" in key:
            return key
    for key in pag_list.__dict__:
        if "nextUrl" in key:
            return key
    return ""


def get_nextUrl(
    pag_list: PaginatedList[PullRequest | Issue], nextUrl_key: str
) -> str | None:
    return getattr(pag_list, nextUrl_key) if nextUrl_key else None


def set_nextUrl(
    pag_list: PaginatedList[PullRequest | Issue], nextUrl_key: str, nextUrl: str
) -> None:
    if nextUrl_key:
        setattr(pag_list, nextUrl_key, nextUrl)
    elif nextUrl:
        raise ValueError("Next URL key not found: " + str(pag_list.__dict__))


def _paginate_until_error(
    git_objs: Callable[[], PaginatedList[PullRequest | Issue]],
    cursor_url: str | None,
    prev_num_objs: int,
    cursor_url_callback: Callable[[str | None, int], None],
    retrying: bool = False,
) -> Generator[PullRequest | Issue, None, None]:
    num_objs = prev_num_objs
    pag_list = git_objs()
    nextUrl_key = get_nextUrl_key(pag_list)
    if cursor_url:
        set_nextUrl(pag_list, nextUrl_key, cursor_url)
    elif retrying:
        # if we are retrying, we want to skip the objects retrieved
        # over previous calls. Unfortunately, this WILL retrieve all
        # pages before the one we are resuming from, so we really
        # don't want this case to be hit often
        logging.warning(
            "Retrying from a previous cursor-based pagination call. "
            "This will retrieve all pages before the one we are resuming from, "
            "which may take a while and consume many API calls."
        )
        pag_list = cast(PaginatedList[PullRequest | Issue], pag_list[prev_num_objs:])
        num_objs = 0

    try:
        # this for loop handles cursor-based pagination
        for issue_or_pr in pag_list:
            num_objs += 1
            yield issue_or_pr
            # used to store the current cursor url in the checkpoint. This value
            # is updated during iteration over pag_list.
            cursor_url_callback(get_nextUrl(pag_list, nextUrl_key), num_objs)

            if num_objs % CURSOR_LOG_FREQUENCY == 0:
                logging.info(
                    f"Retrieved {num_objs} objects with current cursor url: {get_nextUrl(pag_list, nextUrl_key)}"
                )

    except Exception as e:
        logging.exception(f"Error during cursor-based pagination: {e}")
        if num_objs - prev_num_objs > 0:
            raise

        if get_nextUrl(pag_list, nextUrl_key) is not None and not retrying:
            logging.info(
                "Assuming that this error is due to cursor "
                "expiration because no objects were retrieved. "
                "Retrying from the first page."
            )
            yield from _paginate_until_error(
                git_objs, None, prev_num_objs, cursor_url_callback, retrying=True
            )
            return

        # for no cursor url or if we reach this point after a retry, raise the error
        raise


def _get_batch_rate_limited(
    # We pass in a callable because we want git_objs to produce a fresh
    # PaginatedList each time it's called to avoid using the same object for cursor-based pagination
    # from a partial offset-based pagination call.
    git_objs: Callable[[], PaginatedList],
    page_num: int,
    cursor_url: str | None,
    prev_num_objs: int,
    cursor_url_callback: Callable[[str | None, int], None],
    github_client: Github,
    attempt_num: int = 0,
) -> Generator[PullRequest | Issue, None, None]:
    if attempt_num > _MAX_NUM_RATE_LIMIT_RETRIES:
        raise RuntimeError(
            "Re-tried fetching batch too many times. Something is going wrong with fetching objects from Github"
        )
    try:
        if cursor_url:
            # when this is set, we are resuming from an earlier
            # cursor-based pagination call.
            yield from _paginate_until_error(
                git_objs, cursor_url, prev_num_objs, cursor_url_callback
            )
            return
        objs = list(git_objs().get_page(page_num))
        # fetch all data here to disable lazy loading later
        # this is needed to capture the rate limit exception here (if one occurs)
        for obj in objs:
            if hasattr(obj, "raw_data"):
                getattr(obj, "raw_data")
        yield from objs
    except RateLimitExceededException:
        sleep_after_rate_limit_exception(github_client)
        yield from _get_batch_rate_limited(
            git_objs,
            page_num,
            cursor_url,
            prev_num_objs,
            cursor_url_callback,
            github_client,
            attempt_num + 1,
        )
    except GithubException as e:
        if not (
            e.status == 422
            and (
                "cursor" in (e.message or "")
                or "cursor" in (e.data or {}).get("message", "")
            )
        ):
            raise
        # Fallback to a cursor-based pagination strategy
        # This can happen for "large datasets," but there's no documentation
        # On the error on the web as far as we can tell.
        # Error message:
        # "Pagination with the page parameter is not supported for large datasets,
        # please use cursor based pagination (after/before)"
        yield from _paginate_until_error(
            git_objs, cursor_url, prev_num_objs, cursor_url_callback
        )


def _get_userinfo(user: NamedUser) -> dict[str, str]:
    def _safe_get(attr_name: str) -> str | None:
        try:
            return cast(str | None, getattr(user, attr_name))
        except GithubException:
            logging.debug(f"Error getting {attr_name} for user")
            return None

    return {
        k: v
        for k, v in {
            "login": _safe_get("login"),
            "name": _safe_get("name"),
            "email": _safe_get("email"),
        }.items()
        if v is not None
    }


def _convert_pr_to_document(
    pull_request: PullRequest, repo_external_access: ExternalAccess | None
) -> Document:
    repo_name = pull_request.base.repo.full_name if pull_request.base else ""
    doc_metadata = DocMetadata(repo=repo_name)
    file_content_byte = pull_request.body.encode('utf-8') if pull_request.body else b""
    name = sanitize_filename(pull_request.title, "md")

    return Document(
        id=pull_request.html_url,
        blob= file_content_byte,
        source=DocumentSource.GITHUB,
        external_access=repo_external_access,
        semantic_identifier=f"{pull_request.number}:{name}",
        # updated_at is UTC time but is timezone unaware, explicitly add UTC
        # as there is logic in indexing to prevent wrong timestamped docs
        # due to local time discrepancies with UTC
        doc_updated_at=(
            pull_request.updated_at.replace(tzinfo=timezone.utc)
            if pull_request.updated_at
            else None
        ),
        extension=".md",
        # this metadata is used in perm sync
        size_bytes=len(file_content_byte) if file_content_byte else 0,
        primary_owners=[],
        doc_metadata=doc_metadata.model_dump(),
        metadata={
            k: [str(vi) for vi in v] if isinstance(v, list) else str(v)
            for k, v in {
                "object_type": "PullRequest",
                "id": pull_request.number,
                "merged": pull_request.merged,
                "state": pull_request.state,
                "user": _get_userinfo(pull_request.user) if pull_request.user else None,
                "assignees": [
                    _get_userinfo(assignee) for assignee in pull_request.assignees
                ],
                "repo": (
                    pull_request.base.repo.full_name if pull_request.base else None
                ),
                "num_commits": str(pull_request.commits),
                "num_files_changed": str(pull_request.changed_files),
                "labels": [label.name for label in pull_request.labels],
                "created_at": (
                    pull_request.created_at.replace(tzinfo=timezone.utc)
                    if pull_request.created_at
                    else None
                ),
                "updated_at": (
                    pull_request.updated_at.replace(tzinfo=timezone.utc)
                    if pull_request.updated_at
                    else None
                ),
                "closed_at": (
                    pull_request.closed_at.replace(tzinfo=timezone.utc)
                    if pull_request.closed_at
                    else None
                ),
                "merged_at": (
                    pull_request.merged_at.replace(tzinfo=timezone.utc)
                    if pull_request.merged_at
                    else None
                ),
                "merged_by": (
                    _get_userinfo(pull_request.merged_by)
                    if pull_request.merged_by
                    else None
                ),
            }.items()
            if v is not None
        },
    )


def _fetch_issue_comments(issue: Issue) -> str:
    comments = issue.get_comments()
    return "\nComment: ".join(comment.body for comment in comments)


def _convert_issue_to_document(
    issue: Issue, repo_external_access: ExternalAccess | None
) -> Document:
    repo_name = issue.repository.full_name if issue.repository else ""
    doc_metadata = DocMetadata(repo=repo_name)
    file_content_byte = issue.body.encode('utf-8') if issue.body else b""
    name = sanitize_filename(issue.title, "md")

    return Document(
        id=issue.html_url,
        blob=file_content_byte,
        source=DocumentSource.GITHUB,
        extension=".md",
        external_access=repo_external_access,
        semantic_identifier=f"{issue.number}:{name}",
        # updated_at is UTC time but is timezone unaware
        doc_updated_at=issue.updated_at.replace(tzinfo=timezone.utc),
        # this metadata is used in perm sync
        doc_metadata=doc_metadata.model_dump(),
        size_bytes=len(file_content_byte) if file_content_byte else 0,
        primary_owners=[_get_userinfo(issue.user) if issue.user else None],
        metadata={
            k: [str(vi) for vi in v] if isinstance(v, list) else str(v)
            for k, v in {
                "object_type": "Issue",
                "id": issue.number,
                "state": issue.state,
                "user": _get_userinfo(issue.user) if issue.user else None,
                "assignees": [_get_userinfo(assignee) for assignee in issue.assignees],
                "repo": issue.repository.full_name if issue.repository else None,
                "labels": [label.name for label in issue.labels],
                "created_at": (
                    issue.created_at.replace(tzinfo=timezone.utc)
                    if issue.created_at
                    else None
                ),
                "updated_at": (
                    issue.updated_at.replace(tzinfo=timezone.utc)
                    if issue.updated_at
                    else None
                ),
                "closed_at": (
                    issue.closed_at.replace(tzinfo=timezone.utc)
                    if issue.closed_at
                    else None
                ),
                "closed_by": (
                    _get_userinfo(issue.closed_by) if issue.closed_by else None
                ),
            }.items()
            if v is not None
        },
    )


class GithubConnectorStage(Enum):
    START = "start"
    PRS = "prs"
    ISSUES = "issues"


class GithubConnectorCheckpoint(ConnectorCheckpoint):
    stage: GithubConnectorStage
    curr_page: int

    cached_repo_ids: list[int] | None = None
    cached_repo: SerializedRepository | None = None

    # Used for the fallback cursor-based pagination strategy
    num_retrieved: int
    cursor_url: str | None = None

    def reset(self) -> None:
        """
        Resets curr_page, num_retrieved, and cursor_url to their initial values (0, 0, None)
        """
        self.curr_page = 0
        self.num_retrieved = 0
        self.cursor_url = None


def make_cursor_url_callback(
    checkpoint: GithubConnectorCheckpoint,
) -> Callable[[str | None, int], None]:
    def cursor_url_callback(cursor_url: str | None, num_objs: int) -> None:
        # we want to maintain the old cursor url so code after retrieval
        # can determine that we are using the fallback cursor-based pagination strategy
        if cursor_url:
            checkpoint.cursor_url = cursor_url
        checkpoint.num_retrieved = num_objs

    return cursor_url_callback


class GithubConnector(CheckpointedConnectorWithPermSyncGH[GithubConnectorCheckpoint]):
    def __init__(
        self,
        repo_owner: str,
        repositories: str | None = None,
        state_filter: str = "all",
        include_prs: bool = True,
        include_issues: bool = False,
    ) -> None:
        self.repo_owner = repo_owner
        self.repositories = repositories
        self.state_filter = state_filter
        self.include_prs = include_prs
        self.include_issues = include_issues
        self.github_client: Github | None = None

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        # defaults to 30 items per page, can be set to as high as 100
        token = credentials["github_access_token"]
        auth = Auth.Token(token)

        if GITHUB_CONNECTOR_BASE_URL:
            self.github_client = Github(
                auth=auth,
                base_url=GITHUB_CONNECTOR_BASE_URL,
                per_page=ITEMS_PER_PAGE,
            )
        else:
            self.github_client = Github(
                auth=auth,
                per_page=ITEMS_PER_PAGE,
            )

        return None

    def get_github_repo(
        self, github_client: Github, attempt_num: int = 0
    ) -> Repository.Repository:
        if attempt_num > _MAX_NUM_RATE_LIMIT_RETRIES:
            raise RuntimeError(
                "Re-tried fetching repo too many times. Something is going wrong with fetching objects from Github"
            )

        try:
            return github_client.get_repo(f"{self.repo_owner}/{self.repositories}")
        except RateLimitExceededException:
            sleep_after_rate_limit_exception(github_client)
            return self.get_github_repo(github_client, attempt_num + 1)

    def get_github_repos(
        self, github_client: Github, attempt_num: int = 0
    ) -> list[Repository.Repository]:
        """Get specific repositories based on comma-separated repo_name string."""
        if attempt_num > _MAX_NUM_RATE_LIMIT_RETRIES:
            raise RuntimeError(
                "Re-tried fetching repos too many times. Something is going wrong with fetching objects from Github"
            )

        try:
            repos = []
            # Split repo_name by comma and strip whitespace
            repo_names = [
                name.strip() for name in (cast(str, self.repositories)).split(",")
            ]

            for repo_name in repo_names:
                if repo_name:  # Skip empty strings
                    try:
                        repo = github_client.get_repo(f"{self.repo_owner}/{repo_name}")
                        repos.append(repo)
                    except GithubException as e:
                        logging.warning(
                            f"Could not fetch repo {self.repo_owner}/{repo_name}: {e}"
                        )

            return repos
        except RateLimitExceededException:
            sleep_after_rate_limit_exception(github_client)
            return self.get_github_repos(github_client, attempt_num + 1)

    def get_all_repos(
        self, github_client: Github, attempt_num: int = 0
    ) -> list[Repository.Repository]:
        if attempt_num > _MAX_NUM_RATE_LIMIT_RETRIES:
            raise RuntimeError(
                "Re-tried fetching repos too many times. Something is going wrong with fetching objects from Github"
            )

        try:
            # Try to get organization first
            try:
                org = github_client.get_organization(self.repo_owner)
                return list(org.get_repos())

            except GithubException:
                # If not an org, try as a user
                user = github_client.get_user(self.repo_owner)
                return list(user.get_repos())
        except RateLimitExceededException:
            sleep_after_rate_limit_exception(github_client)
            return self.get_all_repos(github_client, attempt_num + 1)

    def _pull_requests_func(
        self, repo: Repository.Repository
    ) -> Callable[[], PaginatedList[PullRequest]]:
        return lambda: repo.get_pulls(
            state=self.state_filter, sort="updated", direction="desc"
        )

    def _issues_func(
        self, repo: Repository.Repository
    ) -> Callable[[], PaginatedList[Issue]]:
        return lambda: repo.get_issues(
            state=self.state_filter, sort="updated", direction="desc"
        )

    def _fetch_from_github(
        self,
        checkpoint: GithubConnectorCheckpoint,
        start: datetime | None = None,
        end: datetime | None = None,
        include_permissions: bool = False,
    ) -> Generator[Document | ConnectorFailure, None, GithubConnectorCheckpoint]:
        if self.github_client is None:
            raise ConnectorMissingCredentialError("GitHub")

        checkpoint = copy.deepcopy(checkpoint)

        # First run of the connector, fetch all repos and store in checkpoint
        if checkpoint.cached_repo_ids is None:
            repos = []
            if self.repositories:
                if "," in self.repositories:
                    # Multiple repositories specified
                    repos = self.get_github_repos(self.github_client)
                else:
                    # Single repository (backward compatibility)
                    repos = [self.get_github_repo(self.github_client)]
            else:
                # All repositories
                repos = self.get_all_repos(self.github_client)
            if not repos:
                checkpoint.has_more = False
                return checkpoint

            curr_repo = repos.pop()
            checkpoint.cached_repo_ids = [repo.id for repo in repos]
            checkpoint.cached_repo = SerializedRepository(
                id=curr_repo.id,
                headers=curr_repo.raw_headers,
                raw_data=curr_repo.raw_data,
            )
            checkpoint.stage = GithubConnectorStage.PRS
            checkpoint.curr_page = 0
            # save checkpoint with repo ids retrieved
            return checkpoint

        if checkpoint.cached_repo is None:
            raise ValueError("No repo saved in checkpoint")

        # Deserialize the repository from the checkpoint
        repo = deserialize_repository(checkpoint.cached_repo, self.github_client)

        cursor_url_callback = make_cursor_url_callback(checkpoint)
        repo_external_access: ExternalAccess | None = None
        if include_permissions:
            repo_external_access = get_external_access_permission(
                repo, self.github_client
            )
        if self.include_prs and checkpoint.stage == GithubConnectorStage.PRS:
            logging.info(f"Fetching PRs for repo: {repo.name}")

            pr_batch = _get_batch_rate_limited(
                self._pull_requests_func(repo),
                checkpoint.curr_page,
                checkpoint.cursor_url,
                checkpoint.num_retrieved,
                cursor_url_callback,
                self.github_client,
            )
            checkpoint.curr_page += 1  # NOTE: not used for cursor-based fallback
            done_with_prs = False
            num_prs = 0
            pr = None
            print("start: ", start)
            for pr in pr_batch:
                num_prs += 1
                print("-"*40)
                print("PR name", pr.title)
                print("updated at", pr.updated_at)
                print("-"*40)
                print("\n")
                # we iterate backwards in time, so at this point we stop processing prs
                if (
                    start is not None
                    and pr.updated_at
                    and pr.updated_at.replace(tzinfo=timezone.utc) <= start
                ):
                    done_with_prs = True
                    break
                # Skip PRs updated after the end date
                if (
                    end is not None
                    and pr.updated_at
                    and pr.updated_at.replace(tzinfo=timezone.utc) > end
                ):
                    continue
                try:
                    yield _convert_pr_to_document(
                        cast(PullRequest, pr), repo_external_access
                    )
                except Exception as e:
                    error_msg = f"Error converting PR to document: {e}"
                    logging.exception(error_msg)
                    yield ConnectorFailure(
                        failed_document=DocumentFailure(
                            document_id=str(pr.id), document_link=pr.html_url
                        ),
                        failure_message=error_msg,
                        exception=e,
                    )
                    continue

            # If we reach this point with a cursor url in the checkpoint, we were using
            # the fallback cursor-based pagination strategy. That strategy tries to get all
            # PRs, so having curosr_url set means we are done with prs. However, we need to
            # return AFTER the checkpoint reset to avoid infinite loops.

            # if we found any PRs on the page and there are more PRs to get, return the checkpoint.
            # In offset mode, while indexing without time constraints, the pr batch
            # will be empty when we're done.
            used_cursor = checkpoint.cursor_url is not None
            if num_prs > 0 and not done_with_prs and not used_cursor:
                return checkpoint

            # if we went past the start date during the loop or there are no more
            # prs to get, we move on to issues
            checkpoint.stage = GithubConnectorStage.ISSUES
            checkpoint.reset()

            if used_cursor:
                # save the checkpoint after changing stage; next run will continue from issues
                return checkpoint

        checkpoint.stage = GithubConnectorStage.ISSUES

        if self.include_issues and checkpoint.stage == GithubConnectorStage.ISSUES:
            logging.info(f"Fetching issues for repo: {repo.name}")

            issue_batch = list(
                _get_batch_rate_limited(
                    self._issues_func(repo),
                    checkpoint.curr_page,
                    checkpoint.cursor_url,
                    checkpoint.num_retrieved,
                    cursor_url_callback,
                    self.github_client,
                )
            )
            checkpoint.curr_page += 1
            done_with_issues = False
            num_issues = 0
            for issue in issue_batch:
                num_issues += 1
                issue = cast(Issue, issue)
                # we iterate backwards in time, so at this point we stop processing prs
                if (
                    start is not None
                    and issue.updated_at.replace(tzinfo=timezone.utc) <= start
                ):
                    done_with_issues = True
                    break
                # Skip PRs updated after the end date
                if (
                    end is not None
                    and issue.updated_at.replace(tzinfo=timezone.utc) > end
                ):
                    continue

                if issue.pull_request is not None:
                    # PRs are handled separately
                    continue

                try:
                    yield _convert_issue_to_document(issue, repo_external_access)
                except Exception as e:
                    error_msg = f"Error converting issue to document: {e}"
                    logging.exception(error_msg)
                    yield ConnectorFailure(
                        failed_document=DocumentFailure(
                            document_id=str(issue.id),
                            document_link=issue.html_url,
                        ),
                        failure_message=error_msg,
                        exception=e,
                    )
                    continue

            # if we found any issues on the page, and we're not done, return the checkpoint.
            # don't return if we're using cursor-based pagination to avoid infinite loops
            if num_issues > 0 and not done_with_issues and not checkpoint.cursor_url:
                return checkpoint

            # if we went past the start date during the loop or there are no more
            # issues to get, we move on to the next repo
            checkpoint.stage = GithubConnectorStage.PRS
            checkpoint.reset()

        checkpoint.has_more = len(checkpoint.cached_repo_ids) > 0
        if checkpoint.cached_repo_ids:
            next_id = checkpoint.cached_repo_ids.pop()
            next_repo = self.github_client.get_repo(next_id)
            checkpoint.cached_repo = SerializedRepository(
                id=next_id,
                headers=next_repo.raw_headers,
                raw_data=next_repo.raw_data,
            )
            checkpoint.stage = GithubConnectorStage.PRS
            checkpoint.reset()

        if checkpoint.cached_repo_ids:
            logging.info(
                f"{len(checkpoint.cached_repo_ids)} repos remaining (IDs: {checkpoint.cached_repo_ids})"
            )
        else:
            logging.info("No more repos remaining")

        return checkpoint

    def _load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: GithubConnectorCheckpoint,
        include_permissions: bool = False,
    ) -> CheckpointOutput[GithubConnectorCheckpoint]:
        start_datetime = datetime.fromtimestamp(start, tz=timezone.utc)
        # add a day for timezone safety
        end_datetime = datetime.fromtimestamp(end, tz=timezone.utc) + ONE_DAY

        # Move start time back by 3 hours, since some Issues/PRs are getting dropped
        # Could be due to delayed processing on GitHub side
        # The non-updated issues since last poll will be shortcut-ed and not embedded
        # adjusted_start_datetime = start_datetime - timedelta(hours=3)

        adjusted_start_datetime = start_datetime

        epoch = datetime.fromtimestamp(0, tz=timezone.utc)
        if adjusted_start_datetime < epoch:
            adjusted_start_datetime = epoch

        return self._fetch_from_github(
            checkpoint,
            start=adjusted_start_datetime,
            end=end_datetime,
            include_permissions=include_permissions,
        )

    @override
    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: GithubConnectorCheckpoint,
    ) -> CheckpointOutput[GithubConnectorCheckpoint]:
        return self._load_from_checkpoint(
            start, end, checkpoint, include_permissions=False
        )

    @override
    def load_from_checkpoint_with_perm_sync(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: GithubConnectorCheckpoint,
    ) -> CheckpointOutput[GithubConnectorCheckpoint]:
        return self._load_from_checkpoint(
            start, end, checkpoint, include_permissions=True
        )

    def validate_connector_settings(self) -> None:
        if self.github_client is None:
            raise ConnectorMissingCredentialError("GitHub credentials not loaded.")

        if not self.repo_owner:
            raise ConnectorValidationError(
                "Invalid connector settings: 'repo_owner' must be provided."
            )

        try:
            if self.repositories:
                if "," in self.repositories:
                    # Multiple repositories specified
                    repo_names = [name.strip() for name in self.repositories.split(",")]
                    if not repo_names:
                        raise ConnectorValidationError(
                            "Invalid connector settings: No valid repository names provided."
                        )

                    # Validate at least one repository exists and is accessible
                    valid_repos = False
                    validation_errors = []

                    for repo_name in repo_names:
                        if not repo_name:
                            continue

                        try:
                            test_repo = self.github_client.get_repo(
                                f"{self.repo_owner}/{repo_name}"
                            )
                            logging.info(
                                f"Successfully accessed repository: {self.repo_owner}/{repo_name}"
                            )
                            test_repo.get_contents("")
                            valid_repos = True
                            # If at least one repo is valid, we can proceed
                            break
                        except GithubException as e:
                            validation_errors.append(
                                f"Repository '{repo_name}': {e.data.get('message', str(e))}"
                            )

                    if not valid_repos:
                        error_msg = (
                            "None of the specified repositories could be accessed: "
                        )
                        error_msg += ", ".join(validation_errors)
                        raise ConnectorValidationError(error_msg)
                else:
                    # Single repository (backward compatibility)
                    test_repo = self.github_client.get_repo(
                        f"{self.repo_owner}/{self.repositories}"
                    )
                    test_repo.get_contents("")
            else:
                # Try to get organization first
                try:
                    org = self.github_client.get_organization(self.repo_owner)
                    total_count = org.get_repos().totalCount
                    if total_count == 0:
                        raise ConnectorValidationError(
                            f"Found no repos for organization: {self.repo_owner}. "
                            "Does the credential have the right scopes?"
                        )
                except GithubException as e:
                    # Check for missing SSO
                    MISSING_SSO_ERROR_MESSAGE = "You must grant your Personal Access token access to this organization".lower()
                    if MISSING_SSO_ERROR_MESSAGE in str(e).lower():
                        SSO_GUIDE_LINK = (
                            "https://docs.github.com/en/enterprise-cloud@latest/authentication/"
                            "authenticating-with-saml-single-sign-on/"
                            "authorizing-a-personal-access-token-for-use-with-saml-single-sign-on"
                        )
                        raise ConnectorValidationError(
                            f"Your GitHub token is missing authorization to access the "
                            f"`{self.repo_owner}` organization. Please follow the guide to "
                            f"authorize your token: {SSO_GUIDE_LINK}"
                        )
                    # If not an org, try as a user
                    user = self.github_client.get_user(self.repo_owner)

                    # Check if we can access any repos
                    total_count = user.get_repos().totalCount
                    if total_count == 0:
                        raise ConnectorValidationError(
                            f"Found no repos for user: {self.repo_owner}. "
                            "Does the credential have the right scopes?"
                        )

        except RateLimitExceededException:
            raise UnexpectedValidationError(
                "Validation failed due to GitHub rate-limits being exceeded. Please try again later."
            )

        except GithubException as e:
            if e.status == 401:
                raise CredentialExpiredError(
                    "GitHub credential appears to be invalid or expired (HTTP 401)."
                )
            elif e.status == 403:
                raise InsufficientPermissionsError(
                    "Your GitHub token does not have sufficient permissions for this repository (HTTP 403)."
                )
            elif e.status == 404:
                if self.repositories:
                    if "," in self.repositories:
                        raise ConnectorValidationError(
                            f"None of the specified GitHub repositories could be found for owner: {self.repo_owner}"
                        )
                    else:
                        raise ConnectorValidationError(
                            f"GitHub repository not found with name: {self.repo_owner}/{self.repositories}"
                        )
                else:
                    raise ConnectorValidationError(
                        f"GitHub user or organization not found: {self.repo_owner}"
                    )
            else:
                raise ConnectorValidationError(
                    f"Unexpected GitHub error (status={e.status}): {e.data}"
                )

        except Exception as exc:
            raise Exception(
                f"Unexpected error during GitHub settings validation: {exc}"
            )

    def validate_checkpoint_json(
        self, checkpoint_json: str
    ) -> GithubConnectorCheckpoint:
        return GithubConnectorCheckpoint.model_validate_json(checkpoint_json)

    def build_dummy_checkpoint(self) -> GithubConnectorCheckpoint:
        return GithubConnectorCheckpoint(
            stage=GithubConnectorStage.PRS, curr_page=0, has_more=True, num_retrieved=0
        )


if __name__ == "__main__":
    # Initialize the connector
    connector = GithubConnector(
        repo_owner="EvoAgentX",
        repositories="EvoAgentX",
        include_issues=True,
        include_prs=False,
    )
    connector.load_credentials(
        {"github_access_token": "<Your_GitHub_Access_Token>"}
    )

    if connector.github_client:
        get_external_access_permission(
            connector.get_github_repos(connector.github_client).pop(),
            connector.github_client,
        )

    # Create a time range from epoch to now
    end_time = datetime.now(timezone.utc)
    start_time = datetime.fromtimestamp(0, tz=timezone.utc)
    time_range = (start_time, end_time)

    # Initialize the runner with a batch size of 10
    runner: ConnectorRunner[GithubConnectorCheckpoint] = ConnectorRunner(
        connector, batch_size=10, include_permissions=False, time_range=time_range
    )

    # Get initial checkpoint
    checkpoint = connector.build_dummy_checkpoint()

    # Run the connector
    while checkpoint.has_more:
        for doc_batch, failure, next_checkpoint in runner.run(checkpoint):
            if doc_batch:
                print(f"Retrieved batch of {len(doc_batch)} documents")
                for doc in doc_batch:
                    print(f"Document: {doc.semantic_identifier}")
            if failure:
                print(f"Failure: {failure.failure_message}")
            if next_checkpoint:
                checkpoint = next_checkpoint
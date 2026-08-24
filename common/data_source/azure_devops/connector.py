from __future__ import annotations

import copy
from collections.abc import Iterator
from datetime import datetime, timezone
from typing import Any, TYPE_CHECKING

from typing_extensions import override

from common.data_source.azure_devops.utils import (
    build_auth_client,
    code_document_id,
    default_branch_of,
    fetch_file_content,
    get_json,
    list_items,
    list_projects,
    list_repositories,
    map_item_to_document,
    map_pull_request_to_document,
    organization_url,
    pull_request_document_id,
    pull_request_updated_at,
    raise_for_auth,
    API_VERSION,
)
from common.data_source.config import INDEX_BATCH_SIZE
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    CredentialExpiredError,
    InsufficientPermissionsError,
    UnexpectedValidationError,
)
from common.data_source.interfaces import (
    CheckpointedConnector,
    CheckpointOutput,
    IndexingHeartbeatInterface,
    SecondsSinceUnixEpoch,
    SlimConnectorWithPermSync,
)
from common.data_source.models import ConnectorCheckpoint, ConnectorFailure, DocumentFailure, SlimDocument

if TYPE_CHECKING:
    import httpx

# Which repositories to index. Mirrors the scope selector other Git connectors use.
INDEX_MODE_ORGANIZATION = "organization"
INDEX_MODE_PROJECTS = "projects"
INDEX_MODE_REPOSITORIES = "repositories"

# Which document families to produce.
CONTENT_CODE = "code"
CONTENT_PULL_REQUESTS = "pull_requests"
CONTENT_BOTH = "both"

STAGE_CODE = "code"
STAGE_PULL_REQUESTS = "pull_requests"

# Files handled per invocation before the checkpoint is persisted. Large
# monorepos would otherwise run for a long time with no resumable progress.
MAX_FILES_PER_CALL = 500
PR_PAGE_SIZE = 100


class AzureDevOpsConnectorCheckpoint(ConnectorCheckpoint):
    """Checkpoint state for resumable Azure DevOps indexing.

    Fields:
        repos_queue: Repositories to visit, each as ``{project, name, branch}``.
        current_repo_index: Repository currently being processed.
        stage: Which document family is being indexed for that repository.
        file_offset: Resume position within the repository file listing.
        pr_skip: Resume position within pull request pagination.
    """

    repos_queue: list[dict[str, str]] = []
    current_repo_index: int = 0
    stage: str = STAGE_CODE
    file_offset: int = 0
    pr_skip: int = 0


class AzureDevOpsConnector(
    CheckpointedConnector[AzureDevOpsConnectorCheckpoint],
    SlimConnectorWithPermSync,
):
    """Connector for indexing Azure DevOps (Azure Repos) source files and pull requests.

    Works against both Azure DevOps Services and self-hosted Azure DevOps Server:
    pass a bare organization name for the former, or the full collection URL for
    the latter.

    Args:
        organization: Organization name, or base URL of a self-hosted collection.
        index_mode: Scope selector — ``organization``, ``projects`` or ``repositories``.
        projects: Comma-separated team projects, used when ``index_mode`` is ``projects``.
        repositories: Comma-separated repositories, used when ``index_mode`` is
            ``repositories``. Entries may be ``project/repo`` or a bare repository name.
        content_types: ``code``, ``pull_requests`` or ``both``.
        batch_size: Max number of slim documents to yield per batch.
    """

    def __init__(
        self,
        organization: str,
        index_mode: str = INDEX_MODE_ORGANIZATION,
        projects: str | None = None,
        repositories: str | None = None,
        content_types: str = CONTENT_BOTH,
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        self.organization = organization
        self.index_mode = index_mode or INDEX_MODE_ORGANIZATION
        self._projects = self._split(projects)
        self._repositories = self._split(repositories)
        self.content_types = content_types or CONTENT_BOTH
        self.batch_size = batch_size
        self.personal_access_token: str | None = None

    @staticmethod
    def _split(value: str | None) -> list[str] | None:
        items = [part.strip() for part in value.split(",")] if value else []
        return [item for item in items if item] or None

    @classmethod
    def build_connector(cls, config: dict[str, Any]) -> "AzureDevOpsConnector":
        credentials = config.get("credentials") or {}
        connector = cls(
            organization=config.get("organization"),
            index_mode=config.get("index_mode") or INDEX_MODE_ORGANIZATION,
            projects=config.get("projects"),
            repositories=config.get("repositories"),
            content_types=config.get("content_types") or CONTENT_BOTH,
        )
        connector.load_credentials({"azure_devops_pat": credentials.get("azure_devops_pat")})
        return connector

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load the personal access token. Requires the ``Code (Read)`` scope."""
        self.personal_access_token = credentials.get("azure_devops_pat")
        if not self.personal_access_token:
            raise ConnectorMissingCredentialError("Azure DevOps")
        return None

    @property
    def _org_url(self) -> str:
        return organization_url(self.organization)

    def _client(self) -> "httpx.Client":
        if not self.personal_access_token:
            raise ConnectorMissingCredentialError("Azure DevOps")
        return build_auth_client(self.personal_access_token)

    def _repo_api_url(self, project: str, repo_name: str) -> str:
        return f"{self._org_url}/{project}/_apis/git/repositories/{repo_name}"

    def _indexes_code(self) -> bool:
        return self.content_types in (CONTENT_CODE, CONTENT_BOTH)

    def _indexes_pull_requests(self) -> bool:
        return self.content_types in (CONTENT_PULL_REQUESTS, CONTENT_BOTH)

    def _matches_repository_filter(self, project: str, repo_name: str) -> bool:
        """Repository entries may be ``project/repo`` or a bare repository name.

        Azure DevOps repository names are unique per project, not per
        organization, so the qualified form is the unambiguous one.
        """
        if self.index_mode != INDEX_MODE_REPOSITORIES or not self._repositories:
            return True
        return repo_name in self._repositories or f"{project}/{repo_name}" in self._repositories

    def _discover_repositories(self, client: "httpx.Client") -> list[dict[str, str]]:
        """Resolve the repositories to index.

        Without an explicit project list the organization-scoped endpoint returns
        every repository in one request; otherwise each project is queried.
        """
        if self.index_mode == INDEX_MODE_PROJECTS and self._projects:
            scopes: list[str | None] = list(self._projects)
        elif self.index_mode == INDEX_MODE_REPOSITORIES and self._repositories:
            qualified = {entry.split("/", 1)[0] for entry in self._repositories if "/" in entry}
            scopes = sorted(qualified) if qualified else [None]
        else:
            scopes = [None]

        discovered: list[dict[str, str]] = []

        for scope in scopes:
            for repo in list_repositories(client, self._org_url, scope):
                name = repo.get("name")
                project = (repo.get("project") or {}).get("name") or scope or ""
                if not name or not project:
                    continue
                if not self._matches_repository_filter(project, name):
                    continue
                discovered.append({"project": project, "name": name, "branch": default_branch_of(repo)})

        return sorted(discovered, key=lambda repo: (repo["project"], repo["name"]))

    def _iter_pull_request_page(self, client: "httpx.Client", project: str, repo_name: str, skip: int) -> list[dict[str, Any]]:
        payload = get_json(
            client,
            f"{self._repo_api_url(project, repo_name)}/pullrequests",
            {"searchCriteria.status": "all", "$top": PR_PAGE_SIZE, "$skip": skip},
        )
        return payload.get("value") or []

    @override
    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: AzureDevOpsConnectorCheckpoint,
    ) -> CheckpointOutput[AzureDevOpsConnectorCheckpoint]:
        """Resumable ingestion across repositories, files and pull requests.

        One repository stage is processed per invocation; the returned checkpoint
        records the exact resume position.
        """
        new_checkpoint = copy.deepcopy(checkpoint)
        window_start = datetime.fromtimestamp(start, tz=timezone.utc)
        window_end = datetime.fromtimestamp(end, tz=timezone.utc)

        with self._client() as client:
            if not new_checkpoint.repos_queue:
                new_checkpoint.repos_queue = self._discover_repositories(client)
                new_checkpoint.current_repo_index = 0
                new_checkpoint.stage = STAGE_CODE
                new_checkpoint.file_offset = 0
                new_checkpoint.pr_skip = 0

            repos = new_checkpoint.repos_queue
            if not repos or new_checkpoint.current_repo_index >= len(repos):
                new_checkpoint.has_more = False
                return new_checkpoint

            repo = repos[new_checkpoint.current_repo_index]
            project, repo_name, branch = repo["project"], repo["name"], repo["branch"]

            if new_checkpoint.stage == STAGE_CODE:
                if self._indexes_code():
                    yield from self._load_code(client, new_checkpoint, project, repo_name, branch, window_start, window_end)
                else:
                    new_checkpoint.stage = STAGE_PULL_REQUESTS
            elif new_checkpoint.stage == STAGE_PULL_REQUESTS:
                if self._indexes_pull_requests():
                    yield from self._load_pull_requests(client, new_checkpoint, project, repo_name, window_start, window_end)
                else:
                    self._advance_repo(new_checkpoint)

            new_checkpoint.has_more = new_checkpoint.current_repo_index < len(new_checkpoint.repos_queue)

        return new_checkpoint

    def _load_code(
        self,
        client: "httpx.Client",
        checkpoint: AzureDevOpsConnectorCheckpoint,
        project: str,
        repo_name: str,
        branch: str,
        window_start: datetime,
        window_end: datetime,
    ) -> Iterator[Any]:
        """Yield source files, resuming from ``checkpoint.file_offset``."""
        repo_api_url = self._repo_api_url(project, repo_name)
        items = list_items(client, repo_api_url, branch)
        window = items[checkpoint.file_offset : checkpoint.file_offset + MAX_FILES_PER_CALL]

        for item in window:
            path = item.get("path", "")
            try:
                change = item.get("latestProcessedChange") or {}
                committer = change.get("committer") or change.get("author") or {}
                changed_at = committer.get("date")
                if isinstance(changed_at, str):
                    changed = datetime.fromisoformat(changed_at.replace("Z", "+00:00")).astimezone(timezone.utc)
                    if not (window_start <= changed <= window_end):
                        continue

                content = fetch_file_content(client, repo_api_url, path, branch)
                if content is None:
                    continue

                yield map_item_to_document(item, content, self.organization, self._org_url, project, repo_name, branch)
            except Exception as e:
                yield ConnectorFailure(
                    failed_document=DocumentFailure(
                        document_id=code_document_id(self.organization, project, repo_name, path.lstrip("/")),
                        document_link=f"{self._org_url}/{project}/_git/{repo_name}?path={path}",
                    ),
                    failure_message=f"Failed to process Azure DevOps file {path}: {e}",
                    exception=e,
                )

        checkpoint.file_offset += len(window)
        if checkpoint.file_offset >= len(items):
            checkpoint.file_offset = 0
            if self._indexes_pull_requests():
                checkpoint.stage = STAGE_PULL_REQUESTS
            else:
                self._advance_repo(checkpoint)

    def _load_pull_requests(
        self,
        client: "httpx.Client",
        checkpoint: AzureDevOpsConnectorCheckpoint,
        project: str,
        repo_name: str,
        window_start: datetime,
        window_end: datetime,
    ) -> Iterator[Any]:
        """Yield one page of pull requests, resuming from ``checkpoint.pr_skip``.

        Azure DevOps has no reliable "updated since" filter on this endpoint, so
        the time window is applied client-side.
        """
        pull_requests = self._iter_pull_request_page(client, project, repo_name, checkpoint.pr_skip)

        for pull_request in pull_requests:
            pr_id = pull_request.get("pullRequestId")
            try:
                updated_at = pull_request_updated_at(pull_request)
                if updated_at and not (window_start <= updated_at <= window_end):
                    continue
                yield map_pull_request_to_document(pull_request, self.organization, self._org_url, project, repo_name)
            except Exception as e:
                yield ConnectorFailure(
                    failed_document=DocumentFailure(
                        document_id=pull_request_document_id(self.organization, project, repo_name, pr_id),
                        document_link=f"{self._org_url}/{project}/_git/{repo_name}/pullrequest/{pr_id}",
                    ),
                    failure_message=f"Failed to process Azure DevOps pull request {pr_id}: {e}",
                    exception=e,
                )

        if len(pull_requests) < PR_PAGE_SIZE:
            self._advance_repo(checkpoint)
        else:
            checkpoint.pr_skip += PR_PAGE_SIZE

    @staticmethod
    def _advance_repo(checkpoint: AzureDevOpsConnectorCheckpoint) -> None:
        checkpoint.current_repo_index += 1
        checkpoint.stage = STAGE_CODE
        checkpoint.file_offset = 0
        checkpoint.pr_skip = 0

    @override
    def build_dummy_checkpoint(self) -> AzureDevOpsConnectorCheckpoint:
        """Create an initial checkpoint with work remaining."""
        return AzureDevOpsConnectorCheckpoint(has_more=True)

    @override
    def validate_checkpoint_json(self, checkpoint_json: str) -> AzureDevOpsConnectorCheckpoint:
        """Validate and deserialize a checkpoint instance from JSON."""
        return AzureDevOpsConnectorCheckpoint.model_validate_json(checkpoint_json)

    def retrieve_all_slim_docs_perm_sync(
        self,
        callback: IndexingHeartbeatInterface | None = None,
    ) -> Iterator[list[SlimDocument]]:
        """Return document ids only, for pruning deleted files and pull requests."""
        batch: list[SlimDocument] = []

        with self._client() as client:
            for repo in self._discover_repositories(client):
                project, repo_name, branch = repo["project"], repo["name"], repo["branch"]
                repo_api_url = self._repo_api_url(project, repo_name)

                if self._indexes_code():
                    for item in list_items(client, repo_api_url, branch):
                        path = item.get("path", "").lstrip("/")
                        batch.append(SlimDocument(id=code_document_id(self.organization, project, repo_name, path)))
                        if len(batch) >= self.batch_size:
                            yield batch
                            batch = []
                            if callback:
                                if callback.should_stop():
                                    raise RuntimeError("azure_devops_sync: Stop signal detected")
                                callback.progress("azure_devops_sync", len(batch))

                if self._indexes_pull_requests():
                    skip = 0
                    while True:
                        pull_requests = self._iter_pull_request_page(client, project, repo_name, skip)
                        for pull_request in pull_requests:
                            batch.append(
                                SlimDocument(
                                    id=pull_request_document_id(
                                        self.organization, project, repo_name, pull_request.get("pullRequestId")
                                    )
                                )
                            )
                        if len(pull_requests) < PR_PAGE_SIZE:
                            break
                        skip += PR_PAGE_SIZE

        if batch:
            yield batch

    def validate_connector_settings(self) -> None:
        """Probe a lightweight endpoint to verify credentials and organization access.

        Raises:
            CredentialExpiredError: on HTTP 401, or on the HTTP 203 sign-in page
                Azure DevOps returns for an invalid token.
            InsufficientPermissionsError: on HTTP 403.
            UnexpectedValidationError: on any other failure.
        """
        try:
            with self._client() as client:
                response = client.get(
                    f"{self._org_url}/_apis/projects",
                    params={"api-version": API_VERSION, "$top": 1},
                )
                raise_for_auth(response)
                if response.status_code == 404:
                    raise UnexpectedValidationError(f"Azure DevOps organization not found: {self.organization}")
                if response.status_code < 200 or response.status_code >= 300:
                    raise UnexpectedValidationError(f"Unexpected Azure DevOps error (status={response.status_code}).")

                if self.index_mode == INDEX_MODE_PROJECTS and not self._projects:
                    raise UnexpectedValidationError("At least one team project is required when indexing by project.")
                if self.index_mode == INDEX_MODE_REPOSITORIES and not self._repositories:
                    raise UnexpectedValidationError("At least one repository is required when indexing by repository.")

                if self._projects:
                    known = set(list_projects(client, self._org_url))
                    missing = [project for project in self._projects if project not in known]
                    if missing:
                        raise UnexpectedValidationError(f"Team project(s) not found or not visible: {', '.join(missing)}")
        except Exception as e:
            if isinstance(
                e,
                (
                    CredentialExpiredError,
                    InsufficientPermissionsError,
                    UnexpectedValidationError,
                    ConnectorMissingCredentialError,
                ),
            ):
                raise
            raise UnexpectedValidationError(f"Unexpected error while validating Azure DevOps settings: {e}")


if __name__ == "__main__":
    connector = AzureDevOpsConnector(organization="<YOUR_ORGANIZATION>", index_mode=INDEX_MODE_PROJECTS, projects="<YOUR_PROJECT>")
    connector.load_credentials({"azure_devops_pat": "<YOUR_PAT>"})
    connector.validate_connector_settings()
    print("Credentials validated successfully.")

    azure_checkpoint = connector.build_dummy_checkpoint()
    start_time = datetime.fromtimestamp(0, tz=timezone.utc)
    end_time = datetime.now(timezone.utc)

    while azure_checkpoint.has_more:
        generator = connector.load_from_checkpoint(
            start=start_time.timestamp(),
            end=end_time.timestamp(),
            checkpoint=azure_checkpoint,
        )
        while True:
            try:
                print(next(generator))
            except StopIteration as stop:
                azure_checkpoint = stop.value
                break

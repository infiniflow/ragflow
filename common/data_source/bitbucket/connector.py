from __future__ import annotations

import copy
from collections.abc import Callable
from collections.abc import Iterator
from datetime import datetime
from datetime import timezone
from typing import Any
from typing import TYPE_CHECKING

from typing_extensions import override

from common.data_source.config import INDEX_BATCH_SIZE
from common.data_source.config import DocumentSource
from common.data_source.config import REQUEST_TIMEOUT_SECONDS
from common.data_source.exceptions import ( 
    ConnectorMissingCredentialError,
    CredentialExpiredError,
    InsufficientPermissionsError,
    UnexpectedValidationError,
)
from common.data_source.interfaces import CheckpointedConnector
from common.data_source.interfaces import CheckpointOutput
from common.data_source.interfaces import IndexingHeartbeatInterface
from common.data_source.interfaces import SecondsSinceUnixEpoch
from common.data_source.interfaces import SlimConnectorWithPermSync
from common.data_source.models import ConnectorCheckpoint
from common.data_source.models import ConnectorFailure
from common.data_source.models import DocumentFailure
from common.data_source.models import SlimDocument
from common.data_source.bitbucket.utils import (
    build_auth_client,
    list_repositories,
    map_pr_to_document,
    paginate,
    PR_LIST_RESPONSE_FIELDS,
    SLIM_PR_LIST_RESPONSE_FIELDS,
)

if TYPE_CHECKING:
    import httpx


class BitbucketConnectorCheckpoint(ConnectorCheckpoint):
    """Checkpoint state for resumable Bitbucket PR indexing.

    Fields:
        repos_queue: Materialized list of repository slugs to process.
        current_repo_index: Index of the repository currently being processed.
        next_url: Bitbucket "next" URL for continuing pagination within the current repo.
    """

    repos_queue: list[str] = []
    current_repo_index: int = 0
    next_url: str | None = None


class BitbucketConnector(
    CheckpointedConnector[BitbucketConnectorCheckpoint],
    SlimConnectorWithPermSync,
):
    """Connector for indexing Bitbucket Cloud pull requests.

    Args:
        workspace: Bitbucket workspace ID.
        repositories: Comma-separated list of repository slugs to index.
        projects: Comma-separated list of project keys to index all repositories within.
        batch_size: Max number of documents to yield per batch.
    """

    def __init__(
        self,
        workspace: str,
        repositories: str | None = None,
        projects: str | None = None,
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        self.workspace = workspace
        self._repositories = (
            [s.strip() for s in repositories.split(",") if s.strip()]
            if repositories
            else None
        )
        self._projects: list[str] | None = (
            [s.strip() for s in projects.split(",") if s.strip()] if projects else None
        )
        self.batch_size = batch_size
        self.email: str | None = None
        self.api_token: str | None = None

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load API token-based credentials.

        Expects a dict with keys: `bitbucket_email`, `bitbucket_api_token`.
        """
        self.email = credentials.get("bitbucket_email")
        self.api_token = credentials.get("bitbucket_api_token")
        if not self.email or not self.api_token:
            raise ConnectorMissingCredentialError("Bitbucket")
        return None

    def _client(self) -> httpx.Client:
        """Build an authenticated HTTP client or raise if credentials missing."""
        if not self.email or not self.api_token:
            raise ConnectorMissingCredentialError("Bitbucket")
        return build_auth_client(self.email, self.api_token)

    def _iter_pull_requests_for_repo(
        self,
        client: httpx.Client,
        repo_slug: str,
        params: dict[str, Any] | None = None,
        start_url: str | None = None,
        on_page: Callable[[str | None], None] | None = None,
    ) -> Iterator[dict[str, Any]]:
        base = f"https://api.bitbucket.org/2.0/repositories/{self.workspace}/{repo_slug}/pullrequests"
        yield from paginate(
            client,
            base,
            params,
            start_url=start_url,
            on_page=on_page,
        )

    def _build_params(
        self,
        fields: str = PR_LIST_RESPONSE_FIELDS,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> dict[str, Any]:
        """Build Bitbucket fetch params.

        Always include OPEN, MERGED, and DECLINED PRs. If both ``start`` and
        ``end`` are provided, apply a single updated_on time window.
        """

        def _iso(ts: SecondsSinceUnixEpoch) -> str:
            return datetime.fromtimestamp(ts, tz=timezone.utc).isoformat()

        def _tc_epoch(
            lower_epoch: SecondsSinceUnixEpoch | None,
            upper_epoch: SecondsSinceUnixEpoch | None,
        ) -> str | None:
            if lower_epoch is not None and upper_epoch is not None:
                lower_iso = _iso(lower_epoch)
                upper_iso = _iso(upper_epoch)
                return f'(updated_on > "{lower_iso}" AND updated_on <= "{upper_iso}")'
            return None

        params: dict[str, Any] = {"fields": fields, "pagelen": 50}
        time_clause = _tc_epoch(start, end)
        q = '(state = "OPEN" OR state = "MERGED" OR state = "DECLINED")'
        if time_clause:
            q = f"{q} AND {time_clause}"
        params["q"] = q
        return params

    def _iter_target_repositories(self, client: httpx.Client) -> Iterator[str]:
        """Yield repository slugs based on configuration.

        Priority:
        - repositories list
        - projects list (list repos by project key)
        - workspace (all repos)
        """
        if self._repositories:
            for slug in self._repositories:
                yield slug
            return
        if self._projects:
            for project_key in self._projects:
                for repo in list_repositories(client, self.workspace, project_key):
                    slug_val = repo.get("slug")
                    if isinstance(slug_val, str) and slug_val:
                        yield slug_val
            return
        for repo in list_repositories(client, self.workspace, None):
            slug_val = repo.get("slug")
            if isinstance(slug_val, str) and slug_val:
                yield slug_val

    @override
    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: BitbucketConnectorCheckpoint,
    ) -> CheckpointOutput[BitbucketConnectorCheckpoint]:
        """Resumable PR ingestion across repos and pages within a time window.

        Yields Documents (or ConnectorFailure for per-PR mapping failures) and returns
        an updated checkpoint that records repo position and next page URL.
        """
        new_checkpoint = copy.deepcopy(checkpoint)

        with self._client() as client:
            # Materialize target repositories once
            if not new_checkpoint.repos_queue:
                # Preserve explicit order; otherwise ensure deterministic ordering
                repos_list = list(self._iter_target_repositories(client))
                new_checkpoint.repos_queue = sorted(set(repos_list))
                new_checkpoint.current_repo_index = 0
                new_checkpoint.next_url = None

            repos = new_checkpoint.repos_queue
            if not repos or new_checkpoint.current_repo_index >= len(repos):
                new_checkpoint.has_more = False
                return new_checkpoint

            repo_slug = repos[new_checkpoint.current_repo_index]

            first_page_params = self._build_params(
                fields=PR_LIST_RESPONSE_FIELDS,
                start=start,
                end=end,
            )

            def _on_page(next_url: str | None) -> None:
                new_checkpoint.next_url = next_url

            for pr in self._iter_pull_requests_for_repo(
                client,
                repo_slug,
                params=first_page_params,
                start_url=new_checkpoint.next_url,
                on_page=_on_page,
            ):
                try:
                    document = map_pr_to_document(pr, self.workspace, repo_slug)
                    yield document
                except Exception as e:
                    pr_id = pr.get("id")
                    pr_link = (
                        f"https://bitbucket.org/{self.workspace}/{repo_slug}/pull-requests/{pr_id}"
                        if pr_id is not None
                        else None
                    )
                    yield ConnectorFailure(
                        failed_document=DocumentFailure(
                            document_id=(
                                f"{DocumentSource.BITBUCKET.value}:{self.workspace}:{repo_slug}:pr:{pr_id}"
                                if pr_id is not None
                                else f"{DocumentSource.BITBUCKET.value}:{self.workspace}:{repo_slug}:pr:unknown"
                            ),
                            document_link=pr_link,
                        ),
                        failure_message=f"Failed to process Bitbucket PR: {e}",
                        exception=e,
                    )

            # Advance to next repository (if any) and set has_more accordingly
            new_checkpoint.current_repo_index += 1
            new_checkpoint.next_url = None
            new_checkpoint.has_more = new_checkpoint.current_repo_index < len(repos)

        return new_checkpoint

    @override
    def build_dummy_checkpoint(self) -> BitbucketConnectorCheckpoint:
        """Create an initial checkpoint with work remaining."""
        return BitbucketConnectorCheckpoint(has_more=True)

    @override
    def validate_checkpoint_json(
        self, checkpoint_json: str
    ) -> BitbucketConnectorCheckpoint:
        """Validate and deserialize a checkpoint instance from JSON."""
        return BitbucketConnectorCheckpoint.model_validate_json(checkpoint_json)

    def retrieve_all_slim_docs_perm_sync(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
        callback: IndexingHeartbeatInterface | None = None,
    ) -> Iterator[list[SlimDocument]]:
        """Return only document IDs for all existing pull requests."""
        batch: list[SlimDocument] = []
        params = self._build_params(
            fields=SLIM_PR_LIST_RESPONSE_FIELDS,
            start=start,
            end=end,
        )
        with self._client() as client:
            for slug in self._iter_target_repositories(client):
                for pr in self._iter_pull_requests_for_repo(
                    client, slug, params=params
                ):
                    pr_id = pr["id"]
                    doc_id = f"{DocumentSource.BITBUCKET.value}:{self.workspace}:{slug}:pr:{pr_id}"
                    batch.append(SlimDocument(id=doc_id))
                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []
                        if callback:
                            if callback.should_stop():
                                # Note: this is not actually used for permission sync yet, just pruning
                                raise RuntimeError(
                                    "bitbucket_pr_sync: Stop signal detected"
                                )
                            callback.progress("bitbucket_pr_sync", len(batch))
        if batch:
            yield batch

    def validate_connector_settings(self) -> None:
        """Validate Bitbucket credentials and workspace access by probing a lightweight endpoint.

        Raises:
            CredentialExpiredError: on HTTP 401
            InsufficientPermissionsError: on HTTP 403
            UnexpectedValidationError: on any other failure
        """
        try:
            with self._client() as client:
                url = f"https://api.bitbucket.org/2.0/repositories/{self.workspace}"
                resp = client.get(
                    url,
                    params={"pagelen": 1, "fields": "pagelen"},
                    timeout=REQUEST_TIMEOUT_SECONDS,
                )
                if resp.status_code == 401:
                    raise CredentialExpiredError(
                        "Invalid or expired Bitbucket credentials (HTTP 401)."
                    )
                if resp.status_code == 403:
                    raise InsufficientPermissionsError(
                        "Insufficient permissions to access Bitbucket workspace (HTTP 403)."
                    )
                if resp.status_code < 200 or resp.status_code >= 300:
                    raise UnexpectedValidationError(
                        f"Unexpected Bitbucket error (status={resp.status_code})."
                    )
        except Exception as e:
            # Network or other unexpected errors
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
            raise UnexpectedValidationError(
                f"Unexpected error while validating Bitbucket settings: {e}"
            )

if __name__ == "__main__":
    bitbucket = BitbucketConnector(
        workspace="<YOUR_WORKSPACE>"
    )

    bitbucket.load_credentials({
        "bitbucket_email": "<YOUR_EMAIL>",
        "bitbucket_api_token": "<YOUR_API_TOKEN>",
    })

    bitbucket.validate_connector_settings()
    print("Credentials validated successfully.")

    start_time = datetime.fromtimestamp(0, tz=timezone.utc)
    end_time = datetime.now(timezone.utc)

    for doc_batch in bitbucket.retrieve_all_slim_docs_perm_sync(
        start=start_time.timestamp(),
        end=end_time.timestamp(),
    ):
        for doc in doc_batch:
            print(doc)


    bitbucket_checkpoint = bitbucket.build_dummy_checkpoint()
    
    while bitbucket_checkpoint.has_more:
        gen = bitbucket.load_from_checkpoint(
            start=start_time.timestamp(),
            end=end_time.timestamp(),
            checkpoint=bitbucket_checkpoint,
        )

        while True:
            try:
                doc = next(gen)  
                print(doc)
            except StopIteration as e:
                bitbucket_checkpoint = e.value  
                break
        
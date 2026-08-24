from __future__ import annotations

import time
from collections.abc import Iterator
from datetime import datetime, timezone
from typing import Any
from urllib.parse import quote

import httpx

from common.data_source.config import REQUEST_TIMEOUT_SECONDS, DocumentSource
from common.data_source.cross_connector_utils.rate_limit_wrapper import rate_limit_builder
from common.data_source.cross_connector_utils.retry_wrapper import retry_builder
from common.data_source.exceptions import (
    CredentialExpiredError,
    InsufficientPermissionsError,
    UnexpectedValidationError,
)
from common.data_source.models import BasicExpertInfo, Document
from common.data_source.utils import get_file_ext

API_VERSION = "7.1"

# Azure DevOps returns binary blobs for these; indexing them adds noise, not signal.
BINARY_EXTENSIONS: frozenset[str] = frozenset(
    {
        ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".svg", ".webp", ".pdf",
        ".zip", ".gz", ".tar", ".7z", ".rar", ".jar", ".war", ".dll", ".exe",
        ".so", ".dylib", ".pdb", ".class", ".pyc", ".woff", ".woff2", ".ttf",
        ".eot", ".otf", ".mp3", ".mp4", ".avi", ".mov", ".psd", ".xlsx", ".docx",
    }
)

# Directories that are build output or vendored dependencies.
EXCLUDED_PATH_SEGMENTS: tuple[str, ...] = (
    "/node_modules/", "/bin/", "/obj/", "/dist/", "/build/", "/target/",
    "/vendor/", "/packages/", "/.git/", "/__pycache__/", "/.venv/",
)

# Version-control metadata: present in every repository, no retrievable signal.
SKIPPED_FILENAMES: frozenset[str] = frozenset(
    {".gitattributes", ".gitignore", ".gitkeep", ".gitmodules", ".dockerignore", ".editorconfig"}
)

MAX_FILE_BYTES = 1_000_000


def build_auth_client(personal_access_token: str) -> httpx.Client:
    """Create an authenticated client for the Azure DevOps REST API.

    Azure DevOps expects the PAT as the password of a basic-auth pair with an
    empty username.
    """
    return httpx.Client(auth=("", personal_access_token), timeout=REQUEST_TIMEOUT_SECONDS)


def organization_url(organization: str) -> str:
    """Resolve the API root for a hosted organization or a self-hosted server.

    ``organization`` may be a bare organization name (Azure DevOps Services) or a
    full base URL such as ``https://tfs.contoso.com/DefaultCollection`` for
    Azure DevOps Server.
    """
    if organization.startswith(("http://", "https://")):
        return organization.rstrip("/")
    return f"https://dev.azure.com/{quote(organization, safe='')}"


def raise_for_auth(response: httpx.Response) -> None:
    """Translate Azure DevOps auth failures into connector errors.

    Azure DevOps does not answer an invalid or unauthorized PAT with 401. It
    answers **203 Non-Authoritative Information** and returns the HTML sign-in
    page, so a naive ``raise_for_status`` succeeds and the JSON parse fails
    later with an unrelated error. Detect it here instead.
    """
    if response.status_code == 203 or "text/html" in response.headers.get("content-type", ""):
        raise CredentialExpiredError("Invalid or expired Azure DevOps personal access token (HTTP 203 sign-in page).")
    if response.status_code == 401:
        raise CredentialExpiredError("Invalid or expired Azure DevOps personal access token (HTTP 401).")
    if response.status_code == 403:
        raise InsufficientPermissionsError("Personal access token lacks the required 'Code (Read)' scope (HTTP 403).")


class AzureDevOpsRetriableError(Exception):
    """Raised for throttling and server-side failures worth retrying."""


class AzureDevOpsNonRetriableError(Exception):
    """Raised for client errors that retrying cannot fix."""


@retry_builder(
    tries=6,
    delay=1,
    backoff=2,
    max_delay=30,
    exceptions=(AzureDevOpsRetriableError, httpx.RequestError),
)
@rate_limit_builder(max_calls=120, period=60)
def azure_devops_get(client: httpx.Client, url: str, params: dict[str, Any] | None = None) -> httpx.Response:
    """Perform a GET against Azure DevOps with retry and rate limiting.

    Azure DevOps throttles on consumed throughput units and answers with 429
    plus a ``Retry-After`` header; a full organization crawl touches every
    repository, so honouring it matters. Authentication failures are mapped
    first and deliberately left non-retriable.
    """
    response = client.get(url, params=params, timeout=REQUEST_TIMEOUT_SECONDS)
    raise_for_auth(response)

    status = response.status_code
    if status == 429:
        retry_after = response.headers.get("Retry-After")
        if retry_after is not None:
            try:
                time.sleep(int(retry_after))
            except (TypeError, ValueError):
                pass
        raise AzureDevOpsRetriableError("Azure DevOps rate limit exceeded (429).")
    if 500 <= status < 600:
        raise AzureDevOpsRetriableError(f"Azure DevOps server error: {status}")
    if 400 <= status < 500:
        raise AzureDevOpsNonRetriableError(f"Azure DevOps client error: {status} for {url}")

    return response


def get_json(client: httpx.Client, url: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
    """GET a JSON payload, mapping auth and HTTP failures to connector errors."""
    query: dict[str, Any] = {"api-version": API_VERSION}
    if params:
        query.update(params)

    response = azure_devops_get(client, url, query)
    if response.status_code < 200 or response.status_code >= 300:
        raise UnexpectedValidationError(f"Azure DevOps request failed (status={response.status_code}) for {url}")
    return response.json()


def list_projects(client: httpx.Client, org_url: str) -> Iterator[str]:
    """Yield every team project name visible to the token."""
    payload = get_json(client, f"{org_url}/_apis/projects", {"$top": 1000})
    for project in payload.get("value") or []:
        name = project.get("name")
        if name:
            yield name


def list_repositories(client: httpx.Client, org_url: str, project: str | None) -> Iterator[dict[str, Any]]:
    """Yield repositories for one project, or for the whole organization.

    The organization-scoped form returns repositories across every project in a
    single request, which keeps repository discovery to one round trip.
    """
    scope = f"{org_url}/{quote(project, safe='')}" if project else org_url
    payload = get_json(client, f"{scope}/_apis/git/repositories")
    for repo in payload.get("value") or []:
        if not repo.get("isDisabled"):
            yield repo


def default_branch_of(repo: dict[str, Any]) -> str:
    """Return the repository default branch name without the ``refs/heads/`` prefix."""
    return (repo.get("defaultBranch") or "refs/heads/main").removeprefix("refs/heads/")


def should_skip_path(path: str) -> bool:
    """Skip build output, vendored code, VCS metadata and binary assets."""
    lowered = path.lower()
    if any(segment in lowered for segment in EXCLUDED_PATH_SEGMENTS):
        return True
    if lowered.rsplit("/", 1)[-1] in SKIPPED_FILENAMES:
        return True
    return get_file_ext(lowered) in BINARY_EXTENSIONS


def document_extension(path: str) -> str:
    """Resolve the extension RAGFlow parses the file with.

    Files such as ``Dockerfile``, ``Makefile`` and ``LICENSE`` carry no
    extension; they are plain text, and an empty extension would leave the
    downstream parser without a handler.
    """
    return get_file_ext(path) or ".txt"


def list_items(client: httpx.Client, repo_api_url: str, branch: str) -> list[dict[str, Any]]:
    """List every file in a repository at ``branch``.

    ``latestProcessedChange=true`` makes Azure DevOps return the last commit for
    each item, which supplies ``doc_updated_at`` without one extra request per
    file.
    """
    payload = get_json(
        client,
        f"{repo_api_url}/items",
        {
            "recursionLevel": "Full",
            "includeContentMetadata": "true",
            "latestProcessedChange": "true",
            "versionDescriptor.versionType": "branch",
            "versionDescriptor.version": branch,
        },
    )
    return [
        item
        for item in (payload.get("value") or [])
        if item.get("gitObjectType") == "blob" and not item.get("isFolder") and not should_skip_path(item.get("path", ""))
    ]


def fetch_file_content(client: httpx.Client, repo_api_url: str, path: str, branch: str) -> bytes | None:
    """Download a single file, or return ``None`` when it is too large to index."""
    response = azure_devops_get(
        client,
        f"{repo_api_url}/items",
        {
            "api-version": API_VERSION,
            "path": path,
            "includeContent": "true",
            "$format": "text",
            "versionDescriptor.versionType": "branch",
            "versionDescriptor.version": branch,
        },
    )
    if response.status_code < 200 or response.status_code >= 300:
        raise UnexpectedValidationError(f"Failed to read {path} (status={response.status_code}).")

    content = response.content
    return None if len(content) > MAX_FILE_BYTES else content


def _parse_timestamp(value: Any) -> datetime | None:
    """Parse an Azure DevOps ISO-8601 timestamp into an aware UTC datetime."""
    if not isinstance(value, str):
        return None
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(timezone.utc)
    except ValueError:
        return None


def code_document_id(organization: str, project: str, repo: str, path: str) -> str:
    """Build the stable document id for a source file."""
    return f"{DocumentSource.AZURE_DEVOPS.value}:{organization}:{project}:{repo}:file:{path}"


def pull_request_document_id(organization: str, project: str, repo: str, pr_id: Any) -> str:
    """Build the stable document id for a pull request."""
    return f"{DocumentSource.AZURE_DEVOPS.value}:{organization}:{project}:{repo}:pr:{pr_id}"


def map_item_to_document(
    item: dict[str, Any],
    content: bytes,
    organization: str,
    org_url: str,
    project: str,
    repo_name: str,
    branch: str,
) -> Document:
    """Map a repository file to a Document."""
    path = item.get("path", "").lstrip("/")
    change = item.get("latestProcessedChange") or {}
    committer = change.get("committer") or change.get("author") or {}
    updated_at = _parse_timestamp(committer.get("date"))
    web_url = f"{org_url}/{quote(project, safe='')}/_git/{quote(repo_name, safe='')}?path=/{path}&version=GB{branch}"

    return Document(
        id=code_document_id(organization, project, repo_name, path),
        blob=content,
        source=DocumentSource.AZURE_DEVOPS,
        semantic_identifier=path.rsplit("/", 1)[-1] or path,
        extension=document_extension(path),
        doc_updated_at=updated_at or datetime.now(tz=timezone.utc),
        size_bytes=len(content),
        primary_owners=[BasicExpertInfo(display_name=committer["name"])] if committer.get("name") else [],
        metadata={
            "type": "CodeFile",
            "path": path,
            "ref": branch,
            "project": project,
            "repository": repo_name,
            "commit_id": change.get("commitId") or "",
            "web_url": web_url,
        },
    )


def map_pull_request_to_document(
    pull_request: dict[str, Any],
    organization: str,
    org_url: str,
    project: str,
    repo_name: str,
) -> Document:
    """Map a pull request, with its description and review metadata, to a Document."""
    pr_id = pull_request.get("pullRequestId")
    title = pull_request.get("title") or f"PR {pr_id}"
    description = pull_request.get("description") or ""
    status = pull_request.get("status") or "unknown"
    created_by = (pull_request.get("createdBy") or {}).get("displayName") or ""
    reviewers = [r.get("displayName", "") for r in pull_request.get("reviewers") or []]
    source_branch = (pull_request.get("sourceRefName") or "").removeprefix("refs/heads/")
    target_branch = (pull_request.get("targetRefName") or "").removeprefix("refs/heads/")
    created_at = _parse_timestamp(pull_request.get("creationDate"))
    closed_at = _parse_timestamp(pull_request.get("closedDate"))
    web_url = f"{org_url}/{quote(project, safe='')}/_git/{quote(repo_name, safe='')}/pullrequest/{pr_id}"

    text = (
        f"Pull Request Information:\n"
        f"- Pull Request ID: {pr_id}\n"
        f"- Title: {title}\n"
        f"- Status: {status}\n"
        f"- Repository: {project}/{repo_name}\n"
        f"- Source Branch: {source_branch}\n"
        f"- Target Branch: {target_branch}\n"
        f"- Created By: {created_by}\n"
        f"- Reviewers: {', '.join(name for name in reviewers if name) or 'N/A'}\n"
        f"- Created On: {created_at.date().isoformat() if created_at else 'N/A'}\n"
        f"- Closed On: {closed_at.date().isoformat() if closed_at else 'N/A'}\n"
        f"\nDescription:\n{description}\n"
    )
    blob = text.encode("utf-8")

    return Document(
        id=pull_request_document_id(organization, project, repo_name, pr_id),
        blob=blob,
        source=DocumentSource.AZURE_DEVOPS,
        semantic_identifier=f"PR #{pr_id}: {title}",
        extension=".txt",
        doc_updated_at=closed_at or created_at or datetime.now(tz=timezone.utc),
        size_bytes=len(blob),
        primary_owners=[BasicExpertInfo(display_name=created_by)] if created_by else [],
        metadata={
            "type": "PullRequest",
            "pull_request_id": str(pr_id),
            "status": status,
            "project": project,
            "repository": repo_name,
            "source_branch": source_branch,
            "target_branch": target_branch,
            "web_url": web_url,
        },
    )


def pull_request_updated_at(pull_request: dict[str, Any]) -> datetime | None:
    """Best available "last activity" timestamp for time-window filtering."""
    return _parse_timestamp(pull_request.get("closedDate")) or _parse_timestamp(pull_request.get("creationDate"))

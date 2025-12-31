from __future__ import annotations

import time
from collections.abc import Callable
from collections.abc import Iterator
from datetime import datetime
from datetime import timezone
from typing import Any

import httpx

from common.data_source.config import REQUEST_TIMEOUT_SECONDS, DocumentSource
from common.data_source.cross_connector_utils.rate_limit_wrapper import (
    rate_limit_builder,
)
from common.data_source.utils import sanitize_filename
from common.data_source.models import BasicExpertInfo, Document
from common.data_source.cross_connector_utils.retry_wrapper import retry_builder

# Fields requested from Bitbucket PR list endpoint to ensure rich PR data
PR_LIST_RESPONSE_FIELDS: str = ",".join(
    [
        "next",
        "page",
        "pagelen",
        "values.author",
        "values.close_source_branch",
        "values.closed_by",
        "values.comment_count",
        "values.created_on",
        "values.description",
        "values.destination",
        "values.draft",
        "values.id",
        "values.links",
        "values.merge_commit",
        "values.participants",
        "values.reason",
        "values.rendered",
        "values.reviewers",
        "values.source",
        "values.state",
        "values.summary",
        "values.task_count",
        "values.title",
        "values.type",
        "values.updated_on",
    ]
)

# Minimal fields for slim retrieval (IDs only)
SLIM_PR_LIST_RESPONSE_FIELDS: str = ",".join(
    [
        "next",
        "page",
        "pagelen",
        "values.id",
    ]
)


# Minimal fields for repository list calls
REPO_LIST_RESPONSE_FIELDS: str = ",".join(
    [
        "next",
        "page",
        "pagelen",
        "values.slug",
        "values.full_name",
        "values.project.key",
    ]
)


class BitbucketRetriableError(Exception):
    """Raised for retriable Bitbucket conditions (429, 5xx)."""


class BitbucketNonRetriableError(Exception):
    """Raised for non-retriable Bitbucket client errors (4xx except 429)."""


@retry_builder(
    tries=6,
    delay=1,
    backoff=2,
    max_delay=30,
    exceptions=(BitbucketRetriableError, httpx.RequestError),
)
@rate_limit_builder(max_calls=60, period=60)
def bitbucket_get(
    client: httpx.Client, url: str, params: dict[str, Any] | None = None
) -> httpx.Response:
    """Perform a GET against Bitbucket with retry and rate limiting.

    Retries on 429 and 5xx responses, and on transport errors. Honors
    `Retry-After` header for 429 when present by sleeping before retrying.
    """
    try:
        response = client.get(url, params=params, timeout=REQUEST_TIMEOUT_SECONDS)
    except httpx.RequestError:
        # Allow retry_builder to handle retries of transport errors
        raise

    try:
        response.raise_for_status()
    except httpx.HTTPStatusError as e:
        status = e.response.status_code if e.response is not None else None
        if status == 429:
            retry_after = e.response.headers.get("Retry-After") if e.response else None
            if retry_after is not None:
                try:
                    time.sleep(int(retry_after))
                except (TypeError, ValueError):
                    pass
            raise BitbucketRetriableError("Bitbucket rate limit exceeded (429)") from e
        if status is not None and 500 <= status < 600:
            raise BitbucketRetriableError(f"Bitbucket server error: {status}") from e
        if status is not None and 400 <= status < 500:
            raise BitbucketNonRetriableError(f"Bitbucket client error: {status}") from e
        # Unknown status, propagate
        raise

    return response


def build_auth_client(email: str, api_token: str) -> httpx.Client:
    """Create an authenticated httpx client for Bitbucket Cloud API."""
    return httpx.Client(auth=(email, api_token), http2=True)


def paginate(
    client: httpx.Client,
    url: str,
    params: dict[str, Any] | None = None,
    start_url: str | None = None,
    on_page: Callable[[str | None], None] | None = None,
) -> Iterator[dict[str, Any]]:
    """Iterate over paginated Bitbucket API responses yielding individual values.

    Args:
        client: Authenticated HTTP client.
        url: Base collection URL (first page when start_url is None).
        params: Query params for the first page.
        start_url: If provided, start from this absolute URL (ignores params).
        on_page: Optional callback invoked after each page with the next page URL.
    """
    next_url = start_url or url
    # If resuming from a next URL, do not pass params again
    query = params.copy() if params else None
    query = None if start_url else query
    while next_url:
        resp = bitbucket_get(client, next_url, params=query)
        data = resp.json()
        values = data.get("values", [])
        for item in values:
            yield item
        next_url = data.get("next")
        if on_page is not None:
            on_page(next_url)
        # only include params on first call, next_url will contain all necessary params
        query = None


def list_repositories(
    client: httpx.Client, workspace: str, project_key: str | None = None
) -> Iterator[dict[str, Any]]:
    """List repositories in a workspace, optionally filtered by project key."""
    base_url = f"https://api.bitbucket.org/2.0/repositories/{workspace}"
    params: dict[str, Any] = {
        "fields": REPO_LIST_RESPONSE_FIELDS,
        "pagelen": 100,
        # Ensure deterministic ordering
        "sort": "full_name",
    }
    if project_key:
        params["q"] = f'project.key="{project_key}"'
    yield from paginate(client, base_url, params)


def map_pr_to_document(pr: dict[str, Any], workspace: str, repo_slug: str) -> Document:
    """Map a Bitbucket pull request JSON to Onyx Document."""
    pr_id = pr["id"]
    title = pr.get("title") or f"PR {pr_id}"
    description = pr.get("description") or ""
    state = pr.get("state")
    draft = pr.get("draft", False)
    author = pr.get("author", {})
    reviewers = pr.get("reviewers", [])
    participants = pr.get("participants", [])

    link = pr.get("links", {}).get("html", {}).get("href") or (
        f"https://bitbucket.org/{workspace}/{repo_slug}/pull-requests/{pr_id}"
    )

    created_on = pr.get("created_on")
    updated_on = pr.get("updated_on")
    updated_dt = (
        datetime.fromisoformat(updated_on.replace("Z", "+00:00")).astimezone(
            timezone.utc
        )
        if isinstance(updated_on, str)
        else None
    )

    source_branch = pr.get("source", {}).get("branch", {}).get("name", "")
    destination_branch = pr.get("destination", {}).get("branch", {}).get("name", "")

    approved_by = [
        _get_user_name(p.get("user", {})) for p in participants if p.get("approved")
    ]

    primary_owner = None
    if author:
        primary_owner = BasicExpertInfo(
            display_name=_get_user_name(author),
        )

    # secondary_owners = [ 
    #     BasicExpertInfo(display_name=_get_user_name(r)) for r in reviewers
    # ] or None 

    reviewer_names = [_get_user_name(r) for r in reviewers]

    # Create a concise summary of key PR info
    created_date = created_on.split("T")[0] if created_on else "N/A"
    updated_date = updated_on.split("T")[0] if updated_on else "N/A"
    content_text = (
        "Pull Request Information:\n"
        f"- Pull Request ID: {pr_id}\n"
        f"- Title: {title}\n"
        f"- State: {state or 'N/A'} {'(Draft)' if draft else ''}\n"
    )
    if state == "DECLINED":
        content_text += f"- Reason: {pr.get('reason', 'N/A')}\n"
    content_text += (
        f"- Author: {_get_user_name(author) if author else 'N/A'}\n"
        f"- Reviewers: {', '.join(reviewer_names) if reviewer_names else 'N/A'}\n"
        f"- Branch: {source_branch} -> {destination_branch}\n"
        f"- Created: {created_date}\n"
        f"- Updated: {updated_date}"
    )
    if description:
        content_text += f"\n\nDescription:\n{description}"

    metadata: dict[str, str | list[str]] = {
        "object_type": "PullRequest",
        "workspace": workspace,
        "repository": repo_slug,
        "pr_key": f"{workspace}/{repo_slug}#{pr_id}",
        "id": str(pr_id),
        "title": title,
        "state": state or "",
        "draft": str(bool(draft)),
        "link": link,
        "author": _get_user_name(author) if author else "",
        "reviewers": reviewer_names,
        "approved_by": approved_by,
        "comment_count": str(pr.get("comment_count", "")),
        "task_count": str(pr.get("task_count", "")),
        "created_on": created_on or "",
        "updated_on": updated_on or "",
        "source_branch": source_branch,
        "destination_branch": destination_branch,
        "closed_by": (
            _get_user_name(pr.get("closed_by", {})) if pr.get("closed_by") else ""
        ),
        "close_source_branch": str(bool(pr.get("close_source_branch", False))),
    }

    name = sanitize_filename(title, "md")

    return Document(
        id=f"{DocumentSource.BITBUCKET.value}:{workspace}:{repo_slug}:pr:{pr_id}",
        blob=content_text.encode("utf-8"),
        source=DocumentSource.BITBUCKET,
        extension=".md",
        semantic_identifier=f"#{pr_id}: {name}",
        size_bytes=len(content_text.encode("utf-8")),
        doc_updated_at=updated_dt,
        primary_owners=[primary_owner] if primary_owner else None,
        # secondary_owners=secondary_owners,
        metadata=metadata,
    )


def _get_user_name(user: dict[str, Any]) -> str:
    return user.get("display_name") or user.get("nickname") or "unknown"
"""Helper utilities for the Jira connector."""

from __future__ import annotations

import os
from collections.abc import Collection
from datetime import datetime, timezone
from typing import Any, Iterable

from jira.resources import Issue

from common.data_source.utils import datetime_from_string

JIRA_SERVER_API_VERSION = os.environ.get("JIRA_SERVER_API_VERSION", "2")
JIRA_CLOUD_API_VERSION = os.environ.get("JIRA_CLOUD_API_VERSION", "3")


def build_issue_url(base_url: str, issue_key: str) -> str:
    """Return the canonical UI URL for a Jira issue."""
    return f"{base_url.rstrip('/')}/browse/{issue_key}"


def parse_jira_datetime(value: Any) -> datetime | None:
    """Best-effort parse of Jira datetime values to aware UTC datetimes."""
    if value is None:
        return None
    if isinstance(value, datetime):
        return value.astimezone(timezone.utc) if value.tzinfo else value.replace(tzinfo=timezone.utc)
    if isinstance(value, str):
        return datetime_from_string(value)
    return None


def extract_named_value(value: Any) -> str | None:
    """Extract a readable string out of Jira's typed objects."""
    if value is None:
        return None
    if isinstance(value, str):
        return value
    if isinstance(value, dict):
        return value.get("name") or value.get("value")
    return getattr(value, "name", None)


def extract_user(value: Any) -> tuple[str | None, str | None]:
    """Return display name + email tuple for a Jira user blob."""
    if value is None:
        return None, None
    if isinstance(value, dict):
        return value.get("displayName"), value.get("emailAddress")

    display = getattr(value, "displayName", None)
    email = getattr(value, "emailAddress", None)
    return display, email


def extract_text_from_adf(adf: Any) -> str:
    """Flatten Atlassian Document Format (ADF) structures to text."""
    texts: list[str] = []

    def _walk(node: Any) -> None:
        if node is None:
            return
        if isinstance(node, dict):
            node_type = node.get("type")
            if node_type == "text":
                texts.append(node.get("text", ""))
            for child in node.get("content", []):
                _walk(child)
        elif isinstance(node, list):
            for child in node:
                _walk(child)

    _walk(adf)
    return "\n".join(part for part in texts if part)


def extract_body_text(value: Any) -> str:
    """Normalize Jira description/comments (raw/adf/str) into plain text."""
    if value is None:
        return ""
    if isinstance(value, str):
        return value.strip()
    if isinstance(value, dict):
        return extract_text_from_adf(value).strip()
    return str(value).strip()


def format_comments(
    comment_block: Any,
    *,
    blacklist: Collection[str],
) -> str:
    """Convert Jira comments into a markdown-ish bullet list."""
    if not isinstance(comment_block, dict):
        return ""

    comments = comment_block.get("comments") or []
    lines: list[str] = []
    normalized_blacklist = {email.lower() for email in blacklist if email}

    for comment in comments:
        author = comment.get("author") or {}
        author_email = (author.get("emailAddress") or "").lower()
        if author_email and author_email in normalized_blacklist:
            continue

        author_name = author.get("displayName") or author.get("name") or author_email or "Unknown"
        created = parse_jira_datetime(comment.get("created"))
        created_str = created.isoformat() if created else "Unknown time"
        body = extract_body_text(comment.get("body"))
        if not body:
            continue

        lines.append(f"- {author_name} ({created_str}):\n{body}")

    return "\n\n".join(lines)


def format_attachments(attachments: Any) -> str:
    """List Jira attachments as bullet points."""
    if not isinstance(attachments, list):
        return ""

    attachment_lines: list[str] = []
    for attachment in attachments:
        filename = attachment.get("filename")
        if not filename:
            continue
        size = attachment.get("size")
        size_text = f" ({size} bytes)" if isinstance(size, int) else ""
        content_url = attachment.get("content") or ""
        url_suffix = f" -> {content_url}" if content_url else ""
        attachment_lines.append(f"- {filename}{size_text}{url_suffix}")

    return "\n".join(attachment_lines)


def should_skip_issue(issue: Issue, labels_to_skip: set[str]) -> bool:
    """Return True if the issue contains any label from the skip list."""
    if not labels_to_skip:
        return False

    fields = getattr(issue, "raw", {}).get("fields", {})
    labels: Iterable[str] = fields.get("labels") or []
    for label in labels:
        if (label or "").lower() in labels_to_skip:
            return True
    return False

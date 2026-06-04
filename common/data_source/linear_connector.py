import hashlib
import logging
from datetime import datetime, timezone
from typing import Any

import requests

from common.data_source.config import INDEX_BATCH_SIZE, REQUEST_TIMEOUT_SECONDS, DocumentSource
from common.data_source.exceptions import ConnectorMissingCredentialError, ConnectorValidationError
from common.data_source.interfaces import LoadConnector, PollConnector, SlimConnectorWithPermSync
from common.data_source.models import (
    BasicExpertInfo,
    Document,
    GenerateDocumentsOutput,
    GenerateSlimDocumentOutput,
    SecondsSinceUnixEpoch,
    SlimDocument,
)

logger = logging.getLogger(__name__)

_DEFAULT_LINEAR_API_URL = "https://api.linear.app/graphql"
_USER_AGENT = "RAGFlow"

_ISSUES_QUERY = """
query LinearIssues($first: Int!, $after: String, $includeArchived: Boolean!) {
  issues(first: $first, after: $after, includeArchived: $includeArchived, orderBy: updatedAt) {
    nodes {
      id
      identifier
      title
      description
      url
      createdAt
      updatedAt
      archivedAt
      priority
      priorityLabel
      state { id name type }
      assignee { id name displayName email }
      creator { id name displayName email }
      team { id name key }
      project { id name description url state updatedAt }
      cycle { id name number startsAt endsAt }
      labels { nodes { id name } }
      comments(first: 50) {
        nodes {
          id
          body
          createdAt
          updatedAt
          url
          user { id name displayName email }
        }
      }
      attachments(first: 50) {
        nodes {
          id
          title
          subtitle
          url
          createdAt
          updatedAt
          creator { id name displayName email }
        }
      }
    }
    pageInfo { endCursor hasNextPage }
  }
}
"""

_PROJECTS_QUERY = """
query LinearProjects($first: Int!, $after: String, $includeArchived: Boolean!) {
  projects(first: $first, after: $after, includeArchived: $includeArchived, orderBy: updatedAt) {
    nodes {
      id
      name
      description
      url
      state
      startDate
      targetDate
      createdAt
      updatedAt
      archivedAt
      creator { id name displayName email }
      lead { id name displayName email }
      teams { nodes { id name key } }
    }
    pageInfo { endCursor hasNextPage }
  }
}
"""

_VIEWER_QUERY = "query LinearViewer { viewer { id name displayName email } }"


class LinearConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    def __init__(
        self,
        team_ids: list[str] | str | None = None,
        project_ids: list[str] | str | None = None,
        include_archived: bool = False,
        include_comments: bool = True,
        include_attachments: bool = True,
        include_projects: bool = True,
        batch_size: int = INDEX_BATCH_SIZE,
        api_url: str = _DEFAULT_LINEAR_API_URL,
    ) -> None:
        self.team_ids = self._normalize_id_list(team_ids)
        self.project_ids = self._normalize_id_list(project_ids)
        self.include_archived = include_archived
        self.include_comments = include_comments
        self.include_attachments = include_attachments
        self.include_projects = include_projects
        self.batch_size = batch_size
        self.api_url = api_url
        self.credentials: dict[str, Any] = {}
        self.session = requests.Session()
        self.session.headers.update({"User-Agent": _USER_AGENT})

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self.credentials = credentials or {}
        return None

    def validate_connector_settings(self) -> None:
        self._require_credentials()
        if self.batch_size < 1:
            raise ConnectorValidationError("batch_size must be at least 1")
        self._graphql(_VIEWER_QUERY)

    def load_from_state(self) -> GenerateDocumentsOutput:
        yield from self._load_documents()

    def poll_source(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
    ) -> GenerateDocumentsOutput:
        yield from self._load_documents(start=start, end=end)

    def retrieve_all_slim_docs_perm_sync(
        self,
        callback: Any = None,
    ) -> GenerateSlimDocumentOutput:
        del callback

        batch: list[SlimDocument] = []
        for issue in self._iter_issues():
            batch.append(SlimDocument(id=self._issue_document_id(issue)))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if self.include_projects:
            for project in self._iter_projects():
                batch.append(SlimDocument(id=self._project_document_id(project)))
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

        if batch:
            yield batch

    def _load_documents(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> GenerateDocumentsOutput:
        batch: list[Document] = []
        for issue in self._iter_issues():
            updated_at = self._parse_linear_datetime(issue.get("updatedAt"))
            if not self._is_in_poll_range(updated_at, start, end):
                continue
            batch.append(self._build_issue_document(issue, updated_at))
            if len(batch) >= self.batch_size:
                logger.debug("Emitting Linear issue document batch size=%s", len(batch))
                yield batch
                batch = []

        if self.include_projects:
            for project in self._iter_projects():
                updated_at = self._parse_linear_datetime(project.get("updatedAt"))
                if not self._is_in_poll_range(updated_at, start, end):
                    continue
                batch.append(self._build_project_document(project, updated_at))
                if len(batch) >= self.batch_size:
                    logger.debug("Emitting Linear document batch size=%s", len(batch))
                    yield batch
                    batch = []

        if batch:
            logger.debug("Emitting final Linear document batch size=%s", len(batch))
            yield batch

    def _iter_issues(self) -> list[dict[str, Any]]:
        issues: list[dict[str, Any]] = []
        after = None
        while True:
            payload = self._graphql(
                _ISSUES_QUERY,
                {
                    "first": min(max(self.batch_size, 1), 100),
                    "after": after,
                    "includeArchived": self.include_archived,
                },
            )
            connection = payload.get("issues") or {}
            page_issues = [
                issue
                for issue in connection.get("nodes") or []
                if self._matches_issue_scope(issue)
            ]
            issues.extend(page_issues)
            page_info = connection.get("pageInfo") or {}
            if not page_info.get("hasNextPage"):
                break
            after = page_info.get("endCursor")
        logger.info("Loaded %s Linear issue(s)", len(issues))
        return issues

    def _iter_projects(self) -> list[dict[str, Any]]:
        projects: list[dict[str, Any]] = []
        after = None
        while True:
            payload = self._graphql(
                _PROJECTS_QUERY,
                {
                    "first": min(max(self.batch_size, 1), 100),
                    "after": after,
                    "includeArchived": self.include_archived,
                },
            )
            connection = payload.get("projects") or {}
            page_projects = [
                project
                for project in connection.get("nodes") or []
                if self._matches_project_scope(project)
            ]
            projects.extend(page_projects)
            page_info = connection.get("pageInfo") or {}
            if not page_info.get("hasNextPage"):
                break
            after = page_info.get("endCursor")
        logger.info("Loaded %s Linear project(s)", len(projects))
        return projects

    def _build_issue_document(
        self,
        issue: dict[str, Any],
        updated_at: datetime,
    ) -> Document:
        content = self._build_issue_content(issue)
        blob = content.encode("utf-8")
        labels = self._connection_nodes(issue.get("labels"))
        metadata = {
            "type": "issue",
            "issue_id": issue.get("id"),
            "identifier": issue.get("identifier"),
            "url": issue.get("url"),
            "state": (issue.get("state") or {}).get("name"),
            "priority": issue.get("priority"),
            "priority_label": issue.get("priorityLabel"),
            "team_id": (issue.get("team") or {}).get("id"),
            "team_key": (issue.get("team") or {}).get("key"),
            "project_id": (issue.get("project") or {}).get("id"),
            "project_name": (issue.get("project") or {}).get("name"),
            "labels": [label.get("name") for label in labels if label.get("name")],
            "archived_at": issue.get("archivedAt"),
        }
        return Document(
            id=self._issue_document_id(issue),
            source=DocumentSource.LINEAR,
            semantic_identifier=issue.get("identifier")
            or issue.get("title")
            or issue.get("id"),
            extension=".md",
            blob=blob,
            doc_updated_at=updated_at,
            size_bytes=len(blob),
            primary_owners=[
                self._user_to_expert(issue.get("assignee") or issue.get("creator"))
            ],
            metadata=metadata,
            fingerprint=hashlib.md5(blob).hexdigest(),
        )

    def _build_project_document(
        self,
        project: dict[str, Any],
        updated_at: datetime,
    ) -> Document:
        content = self._build_project_content(project)
        blob = content.encode("utf-8")
        teams = self._connection_nodes(project.get("teams"))
        metadata = {
            "type": "project",
            "project_id": project.get("id"),
            "url": project.get("url"),
            "state": project.get("state"),
            "team_ids": [team.get("id") for team in teams if team.get("id")],
            "team_keys": [team.get("key") for team in teams if team.get("key")],
            "archived_at": project.get("archivedAt"),
        }
        return Document(
            id=self._project_document_id(project),
            source=DocumentSource.LINEAR,
            semantic_identifier=project.get("name") or project.get("id"),
            extension=".md",
            blob=blob,
            doc_updated_at=updated_at,
            size_bytes=len(blob),
            primary_owners=[
                self._user_to_expert(project.get("lead") or project.get("creator"))
            ],
            metadata=metadata,
            fingerprint=hashlib.md5(blob).hexdigest(),
        )

    def _build_issue_content(self, issue: dict[str, Any]) -> str:
        parts = [
            f"Issue: {issue.get('identifier', '')} {issue.get('title', '')}".strip(),
            f"URL: {issue.get('url', '')}",
            f"State: {(issue.get('state') or {}).get('name', '')}",
            f"Team: {(issue.get('team') or {}).get('name', '')}",
        ]
        project = issue.get("project") or {}
        if project:
            parts.append(f"Project: {project.get('name', '')}")
        if issue.get("priorityLabel"):
            parts.append(f"Priority: {issue.get('priorityLabel')}")
        if issue.get("description"):
            parts.append("Description:\n" + issue["description"].strip())

        comments = (
            self._connection_nodes(issue.get("comments"))
            if self.include_comments
            else []
        )
        if comments:
            parts.append("Comments:")
            for comment in comments:
                user = comment.get("user") or {}
                author = user.get("displayName") or user.get("name") or "Unknown"
                parts.append(
                    f"- {author} ({comment.get('updatedAt') or comment.get('createdAt', '')}): "
                    f"{comment.get('body', '')}"
                )

        attachments = (
            self._connection_nodes(issue.get("attachments"))
            if self.include_attachments
            else []
        )
        if attachments:
            parts.append("Attachments:")
            for attachment in attachments:
                title = (
                    attachment.get("title")
                    or attachment.get("subtitle")
                    or attachment.get("id")
                )
                parts.append(f"- {title}: {attachment.get('url', '')}".strip())

        return "\n\n".join(part for part in parts if part).strip()

    def _build_project_content(self, project: dict[str, Any]) -> str:
        teams = self._connection_nodes(project.get("teams"))
        parts = [
            f"Project: {project.get('name', '')}",
            f"URL: {project.get('url', '')}",
            f"State: {project.get('state', '')}",
        ]
        if teams:
            parts.append(
                "Teams: "
                + ", ".join(
                    team.get("name") or team.get("key") or team.get("id")
                    for team in teams
                )
            )
        if project.get("targetDate"):
            parts.append(f"Target date: {project.get('targetDate')}")
        if project.get("description"):
            parts.append("Description:\n" + project["description"].strip())
        return "\n\n".join(part for part in parts if part).strip()

    def _graphql(
        self,
        query: str,
        variables: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        self._require_credentials()
        headers = {
            "Authorization": self.credentials["linear_api_key"],
            "Content-Type": "application/json",
            "Accept": "application/json",
        }
        try:
            response = self.session.post(
                self.api_url,
                json={"query": query, "variables": variables or {}},
                headers=headers,
                timeout=REQUEST_TIMEOUT_SECONDS,
            )
        except requests.RequestException:
            logger.exception("Linear GraphQL request failed before response")
            raise

        if response.status_code < 200 or response.status_code >= 300:
            logger.warning(
                "Linear GraphQL request failed status_code=%s",
                response.status_code,
            )
            raise requests.exceptions.HTTPError(
                f"{response.status_code} Error for url: {self.api_url}",
                response=response,
            )

        payload = response.json()
        if payload.get("errors"):
            messages = ", ".join(
                str(error.get("message", error)) for error in payload["errors"]
            )
            raise ConnectorValidationError(f"Linear GraphQL error: {messages}")
        return payload.get("data") or {}

    def _require_credentials(self) -> None:
        if not self.credentials.get("linear_api_key"):
            raise ConnectorMissingCredentialError("Linear")

    def _matches_issue_scope(self, issue: dict[str, Any]) -> bool:
        team_id = (issue.get("team") or {}).get("id")
        project_id = (issue.get("project") or {}).get("id")
        if self.team_ids and team_id not in self.team_ids:
            return False
        if self.project_ids and project_id not in self.project_ids:
            return False
        return True

    def _matches_project_scope(self, project: dict[str, Any]) -> bool:
        if self.project_ids and project.get("id") not in self.project_ids:
            return False
        if not self.team_ids:
            return True
        teams = self._connection_nodes(project.get("teams"))
        return any(team.get("id") in self.team_ids for team in teams)

    @staticmethod
    def _connection_nodes(connection: Any) -> list[dict[str, Any]]:
        if not isinstance(connection, dict):
            return []
        nodes = connection.get("nodes")
        if isinstance(nodes, list):
            return [node for node in nodes if isinstance(node, dict)]
        return []

    @staticmethod
    def _is_in_poll_range(
        value: datetime,
        start: SecondsSinceUnixEpoch | None,
        end: SecondsSinceUnixEpoch | None,
    ) -> bool:
        timestamp = value.timestamp()
        if start is not None and timestamp <= start:
            return False
        if end is not None and timestamp > end:
            return False
        return True

    @staticmethod
    def _parse_linear_datetime(value: Any) -> datetime:
        if not isinstance(value, str) or not value.strip():
            return datetime.fromtimestamp(0, tz=timezone.utc)
        normalized = value.replace("Z", "+00:00")
        try:
            parsed = datetime.fromisoformat(normalized)
        except ValueError:
            logger.warning("Invalid Linear timestamp value: %r", value)
            return datetime.fromtimestamp(0, tz=timezone.utc)
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=timezone.utc)
        return parsed.astimezone(timezone.utc)

    @staticmethod
    def _normalize_id_list(value: list[str] | str | None) -> list[str]:
        if value is None:
            return []
        if isinstance(value, str):
            raw_items = value.split(",")
        else:
            raw_items = value
        return [str(item).strip() for item in raw_items if str(item).strip()]

    @staticmethod
    def _user_to_expert(user: Any) -> BasicExpertInfo:
        user = user or {}
        return BasicExpertInfo(
            display_name=user.get("displayName") or user.get("name"),
            email=user.get("email"),
        )

    @staticmethod
    def _issue_document_id(issue: dict[str, Any]) -> str:
        return f"linear:issue:{issue['id']}"

    @staticmethod
    def _project_document_id(project: dict[str, Any]) -> str:
        return f"linear:project:{project['id']}"

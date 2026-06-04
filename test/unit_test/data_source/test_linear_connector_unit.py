from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

import pytest

from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
)
from common.data_source.linear_connector import LinearConnector


class FakeResponse:
    def __init__(self, payload: dict[str, Any], status_code: int = 200) -> None:
        self.payload = payload
        self.status_code = status_code

    def json(self) -> dict[str, Any]:
        return self.payload


class FakeSession:
    def __init__(self, payload: dict[str, Any] | None = None) -> None:
        self.payload = payload
        self.calls: list[dict[str, Any]] = []

    def post(
        self,
        url: str,
        json: dict[str, Any],
        headers: dict[str, str],
        timeout: int,
    ) -> FakeResponse:
        del url, timeout
        self.calls.append({"json": json, "headers": headers})
        query = json["query"]
        if "viewer" in query:
            return FakeResponse({"data": {"viewer": {"id": "u1", "name": "Alice"}}})
        if "issues" in query:
            return FakeResponse({"data": {"issues": self._issues_connection()}})
        if "projects" in query:
            return FakeResponse({"data": {"projects": self._projects_connection()}})
        return FakeResponse(self.payload or {"data": {}})

    @staticmethod
    def _issues_connection() -> dict[str, Any]:
        return {
            "nodes": [
                {
                    "id": "issue-1",
                    "identifier": "ENG-1",
                    "title": "Ship Linear connector",
                    "description": "Implement Linear sync.",
                    "url": "https://linear.app/acme/issue/ENG-1",
                    "createdAt": "2026-06-01T10:00:00.000Z",
                    "updatedAt": "2026-06-01T12:00:00.000Z",
                    "archivedAt": None,
                    "priority": 1,
                    "priorityLabel": "Urgent",
                    "state": {"id": "state-1", "name": "In Progress", "type": "started"},
                    "assignee": {
                        "id": "u1",
                        "displayName": "Alice",
                        "email": "alice@example.com",
                    },
                    "creator": {
                        "id": "u2",
                        "displayName": "Bob",
                        "email": "bob@example.com",
                    },
                    "team": {"id": "team-1", "name": "Engineering", "key": "ENG"},
                    "project": {
                        "id": "project-1",
                        "name": "Connectors",
                        "description": "Data source connectors",
                        "url": "https://linear.app/acme/project/connectors",
                        "state": "started",
                        "updatedAt": "2026-06-01T12:00:00.000Z",
                    },
                    "cycle": {"id": "cycle-1", "name": "Cycle 1", "number": 1},
                    "labels": {"nodes": [{"id": "label-1", "name": "Backend"}]},
                    "comments": {
                        "nodes": [
                            {
                                "id": "comment-1",
                                "body": "Looks good.",
                                "createdAt": "2026-06-01T13:00:00.000Z",
                                "updatedAt": "2026-06-01T13:00:00.000Z",
                                "url": "https://linear.app/acme/issue/ENG-1#comment-comment-1",
                                "user": {"id": "u3", "displayName": "Carol"},
                            }
                        ]
                    },
                    "attachments": {
                        "nodes": [
                            {
                                "id": "attachment-1",
                                "title": "Design",
                                "url": "https://example.com/design",
                                "createdAt": "2026-06-01T14:00:00.000Z",
                                "updatedAt": "2026-06-01T14:00:00.000Z",
                            }
                        ]
                    },
                },
                {
                    "id": "issue-2",
                    "identifier": "OPS-1",
                    "title": "Other team issue",
                    "description": "",
                    "url": "https://linear.app/acme/issue/OPS-1",
                    "updatedAt": "2026-05-01T12:00:00.000Z",
                    "team": {"id": "team-2", "name": "Operations", "key": "OPS"},
                    "project": None,
                    "labels": {"nodes": []},
                    "comments": {"nodes": []},
                    "attachments": {"nodes": []},
                },
            ],
            "pageInfo": {"endCursor": None, "hasNextPage": False},
        }

    @staticmethod
    def _projects_connection() -> dict[str, Any]:
        return {
            "nodes": [
                {
                    "id": "project-1",
                    "name": "Connectors",
                    "description": "Data source connectors",
                    "url": "https://linear.app/acme/project/connectors",
                    "state": "started",
                    "startDate": "2026-06-01",
                    "targetDate": "2026-06-30",
                    "createdAt": "2026-06-01T09:00:00.000Z",
                    "updatedAt": "2026-06-02T12:00:00.000Z",
                    "archivedAt": None,
                    "creator": {"id": "u2", "displayName": "Bob"},
                    "lead": {"id": "u1", "displayName": "Alice"},
                    "teams": {
                        "nodes": [{"id": "team-1", "name": "Engineering", "key": "ENG"}]
                    },
                }
            ],
            "pageInfo": {"endCursor": None, "hasNextPage": False},
        }


def make_connector(**kwargs: Any) -> LinearConnector:
    connector = LinearConnector(api_url="https://linear.test/graphql", **kwargs)
    connector.session = FakeSession()
    connector.load_credentials({"linear_api_key": "linear-key"})
    return connector


@pytest.mark.p1
def test_linear_connector_builds_issue_and_project_documents() -> None:
    connector = make_connector(team_ids=["team-1"])

    batches = list(connector.load_from_state())

    assert [[doc.id for doc in batch] for batch in batches] == [
        ["linear:issue:issue-1", "linear:project:project-1"]
    ]
    issue_doc = batches[0][0]
    issue_text = issue_doc.blob.decode("utf-8")
    assert issue_doc.source == "linear"
    assert issue_doc.semantic_identifier == "ENG-1"
    assert issue_doc.metadata["team_key"] == "ENG"
    assert issue_doc.metadata["project_name"] == "Connectors"
    assert "Implement Linear sync." in issue_text
    assert "Looks good." in issue_text
    assert "Design" in issue_text
    assert issue_doc.fingerprint


@pytest.mark.p1
def test_linear_connector_poll_source_filters_by_updated_at() -> None:
    connector = make_connector(team_ids=["team-1"], include_projects=False)
    start = datetime(2026, 6, 1, 11, 0, tzinfo=timezone.utc).timestamp()
    end = datetime(2026, 6, 1, 13, 0, tzinfo=timezone.utc).timestamp()

    batches = list(connector.poll_source(start, end))

    assert [[doc.id for doc in batch] for batch in batches] == [["linear:issue:issue-1"]]


@pytest.mark.p1
def test_linear_connector_retrieves_slim_docs() -> None:
    connector = make_connector(team_ids=["team-1"])

    batches = list(connector.retrieve_all_slim_docs_perm_sync())

    assert [[doc.id for doc in batch] for batch in batches] == [
        ["linear:issue:issue-1", "linear:project:project-1"]
    ]


@pytest.mark.p1
def test_linear_connector_requires_credentials() -> None:
    connector = LinearConnector()

    with pytest.raises(ConnectorMissingCredentialError):
        connector.validate_connector_settings()


@pytest.mark.p1
def test_linear_connector_raises_graphql_errors() -> None:
    connector = LinearConnector(api_url="https://linear.test/graphql")
    connector.session = FakeSession({"errors": [{"message": "Invalid API key"}]})
    connector.load_credentials({"linear_api_key": "linear-key"})

    with pytest.raises(ConnectorValidationError, match="Invalid API key"):
        connector._graphql("query Something { something }")

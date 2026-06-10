from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

import pytest

from common.data_source.trino_connector import TrinoConnector


class FakeResponse:
    def __init__(self, payload: dict[str, Any]) -> None:
        self.payload = payload

    def json(self) -> dict[str, Any]:
        return self.payload

    def raise_for_status(self) -> None:
        return None


class FakeSession:
    def __init__(self) -> None:
        self.headers: dict[str, str] = {}
        self.auth: tuple[str, str] | None = None
        self.calls: list[str] = []

    def post(self, url: str, data: str, headers: dict[str, str], timeout: int) -> FakeResponse:
        del timeout
        self.calls.append(f"POST {url} {data} {headers.get('X-Trino-Catalog')}")
        return FakeResponse(
            {
                "columns": [
                    {"name": "id"},
                    {"name": "title"},
                    {"name": "body"},
                    {"name": "updated_at"},
                ],
                "data": [["1", "Policy", "Quarterly policy text", "2026-06-01T10:00:00Z"]],
            }
        )

    def get(self, url: str, timeout: int) -> FakeResponse:
        del url, timeout
        return FakeResponse({})


class ErrorSession(FakeSession):
    def post(self, url: str, data: str, headers: dict[str, str], timeout: int) -> FakeResponse:
        del url, data, headers, timeout
        return FakeResponse({"error": {"errorName": "USER_ERROR", "message": "bad query"}})


class PaginatedSession(FakeSession):
    def post(self, url: str, data: str, headers: dict[str, str], timeout: int) -> FakeResponse:
        del timeout
        self.calls.append(f"POST {url} {data} {headers.get('X-Trino-Catalog')}")
        return FakeResponse(
            {
                "columns": [
                    {"name": "id"},
                    {"name": "title"},
                    {"name": "body"},
                    {"name": "updated_at"},
                ],
                "data": [["1", "Policy", "Quarterly policy text", "2026-06-01T10:00:00Z"]],
                "nextUri": "https://trino.test/v1/statement/queued/2",
            }
        )

    def get(self, url: str, timeout: int) -> FakeResponse:
        del timeout
        self.calls.append(f"GET {url}")
        return FakeResponse(
            {
                "columns": [
                    {"name": "id"},
                    {"name": "title"},
                    {"name": "body"},
                    {"name": "updated_at"},
                ],
                "data": [["2", "Runbook", "Incident response steps", "2026-06-01T11:00:00Z"]],
            }
        )


def make_connector() -> TrinoConnector:
    connector = TrinoConnector(
        server_url="https://trino.test",
        catalog="hive",
        schema="default",
        query="SELECT id, title, body, updated_at FROM docs",
        content_columns="title,body",
        metadata_columns="updated_at",
        id_column="id",
        timestamp_column="updated_at",
    )
    connector.session = FakeSession()
    connector.load_credentials({"trino_username": "user", "trino_password": "pass"})
    return connector


@pytest.mark.p1
def test_trino_connector_builds_row_documents() -> None:
    connector = make_connector()

    batches = list(connector.load_from_state())

    assert len(batches) == 1
    doc = batches[0][0]
    text = doc.blob.decode("utf-8")
    assert doc.id == "trino:1"
    assert doc.source == "trino"
    assert doc.semantic_identifier == "1"
    assert "title: Policy" in text
    assert "body: Quarterly policy text" in text
    assert doc.metadata["catalog"] == "hive"
    assert doc.fingerprint


@pytest.mark.p1
def test_trino_connector_poll_source_filters_by_date() -> None:
    connector = make_connector()
    start = datetime(2026, 5, 30, tzinfo=timezone.utc).timestamp()
    end = datetime(2026, 6, 2, tzinfo=timezone.utc).timestamp()

    batches = list(connector.poll_source(start, end))

    assert [[doc.id for doc in batch] for batch in batches] == [["trino:1"]]


@pytest.mark.p1
def test_trino_connector_retrieves_slim_docs() -> None:
    connector = make_connector()

    batches = list(connector.retrieve_all_slim_docs_perm_sync())

    assert [[doc.id for doc in batch] for batch in batches] == [["trino:1"]]


@pytest.mark.p1
def test_trino_connector_follows_next_uri_pages() -> None:
    connector = make_connector()
    session = PaginatedSession()
    connector.session = session

    batches = list(connector.load_from_state())

    assert [[doc.id for doc in batch] for batch in batches] == [["trino:1", "trino:2"]]
    assert "GET https://trino.test/v1/statement/queued/2" in session.calls


@pytest.mark.p1
def test_trino_connector_requires_query() -> None:
    connector = TrinoConnector("https://trino.test", "hive", "default", "", "title")
    connector.load_credentials({"trino_username": "user"})

    with pytest.raises(ValueError, match="query"):
        connector.validate_connector_settings()


@pytest.mark.p1
def test_trino_connector_raises_trino_errors() -> None:
    connector = make_connector()
    connector.session = ErrorSession()

    with pytest.raises(ValueError, match="USER_ERROR: bad query"):
        list(connector.load_from_state())

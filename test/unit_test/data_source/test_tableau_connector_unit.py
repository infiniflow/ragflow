from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

import pytest

from common.data_source.tableau_connector import TableauConnector


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
        self.calls: list[str] = []

    def post(self, url: str, json: dict[str, Any], timeout: int) -> FakeResponse:
        del json, timeout
        self.calls.append(url.split("https://tableau.test/", 1)[1])
        return FakeResponse({"credentials": {"token": "token", "site": {"id": "site1"}}})

    def get(self, url: str, params: dict[str, Any], timeout: int) -> FakeResponse:
        del params, timeout
        path = url.split("https://tableau.test/", 1)[1]
        self.calls.append(path)
        payloads = {
            "api/3.24/sites/site1/workbooks": {
                "pagination": {"totalAvailable": "1"},
                "workbooks": {
                    "workbook": [
                        {
                            "id": "wb1",
                            "name": "Revenue",
                            "updatedAt": "2026-06-01T10:00:00Z",
                            "webpageUrl": "https://tableau.test/#/workbooks/wb1",
                            "project": {"name": "Finance"},
                        }
                    ]
                },
            },
            "api/3.24/sites/site1/workbooks/wb1/views": {
                "pagination": {"totalAvailable": "1"},
                "views": {
                    "view": [
                        {
                            "id": "view1",
                            "name": "Revenue Summary",
                            "updatedAt": "2026-06-01T11:00:00Z",
                            "webpageUrl": "https://tableau.test/#/views/view1",
                        }
                    ]
                },
            },
        }
        return FakeResponse(payloads[path])


def make_connector() -> TableauConnector:
    connector = TableauConnector("https://tableau.test", project_names="Finance")
    connector.session = FakeSession()
    connector.load_credentials({"tableau_pat_name": "name", "tableau_pat_secret": "secret"})
    return connector


@pytest.mark.p1
def test_tableau_connector_builds_view_documents() -> None:
    connector = make_connector()

    batches = list(connector.load_from_state())

    assert len(batches) == 1
    doc = batches[0][0]
    text = doc.blob.decode("utf-8")
    assert doc.id == "tableau:view:view1"
    assert doc.source == "tableau"
    assert doc.semantic_identifier == "Revenue Summary"
    assert "Workbook: Revenue" in text
    assert doc.metadata["project_name"] == "Finance"
    assert doc.fingerprint


@pytest.mark.p1
def test_tableau_connector_poll_source_filters_by_date() -> None:
    connector = make_connector()
    start = datetime(2026, 5, 30, tzinfo=timezone.utc).timestamp()
    end = datetime(2026, 6, 2, tzinfo=timezone.utc).timestamp()

    batches = list(connector.poll_source(start, end))

    assert [[doc.id for doc in batch] for batch in batches] == [["tableau:view:view1"]]


@pytest.mark.p1
def test_tableau_connector_retrieves_slim_docs() -> None:
    connector = make_connector()

    batches = list(connector.retrieve_all_slim_docs_perm_sync())

    assert [[doc.id for doc in batch] for batch in batches] == [["tableau:view:view1"]]


@pytest.mark.p1
def test_tableau_connector_requires_pat_credentials() -> None:
    connector = TableauConnector("https://tableau.test")
    connector.load_credentials({})

    with pytest.raises(ValueError, match="personal access token"):
        connector.validate_connector_settings()

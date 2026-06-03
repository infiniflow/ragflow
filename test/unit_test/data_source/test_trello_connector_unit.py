from __future__ import annotations

from datetime import datetime, timezone
from types import SimpleNamespace
from typing import Any

import pytest
import requests

from common.data_source.trello_connector import TrelloConnector


class FakeResponse:
    def __init__(self, payload: Any, status_code: int = 200) -> None:
        self.payload = payload
        self.status_code = status_code

    def json(self) -> Any:
        return self.payload

    def raise_for_status(self) -> None:
        if self.status_code >= 400:
            raise RuntimeError(f"status {self.status_code}")


class FakeSession:
    def __init__(self) -> None:
        self.calls: list[tuple[str, dict[str, Any]]] = []

    def get(self, url: str, params: dict[str, Any], timeout: int) -> FakeResponse:
        del timeout
        path = url.split("/1/", 1)[1]
        self.calls.append((path, params))
        payloads: dict[str, Any] = {
            "members/me/boards": [
                {
                    "id": "b1",
                    "name": "Roadmap",
                    "url": "https://trello.com/b/b1/roadmap",
                    "dateLastActivity": "2026-06-01T10:00:00.000Z",
                }
            ],
            "boards/b1": {
                "id": "b1",
                "name": "Roadmap",
                "url": "https://trello.com/b/b1/roadmap",
                "dateLastActivity": "2026-06-01T10:00:00.000Z",
            },
            "boards/b1/lists": [
                {"id": "l1", "name": "Doing"},
            ],
            "boards/b1/cards": [
                {
                    "id": "c1",
                    "name": "Ship Trello connector",
                    "desc": "Implement card sync.",
                    "url": "https://trello.com/c/c1",
                    "shortUrl": "https://trello.com/c/c1",
                    "dateLastActivity": "2026-06-01T11:00:00.000Z",
                    "due": "2026-06-10T00:00:00.000Z",
                    "dueComplete": False,
                    "idList": "l1",
                    "labels": [{"name": "Backend", "color": "blue"}],
                    "closed": False,
                },
                {
                    "id": "c2",
                    "name": "Old card",
                    "desc": "",
                    "url": "https://trello.com/c/c2",
                    "dateLastActivity": "2026-05-01T11:00:00.000Z",
                    "idList": "l1",
                    "labels": [],
                    "closed": False,
                },
            ],
            "cards/c1/actions": [
                {
                    "date": "2026-06-01T12:00:00.000Z",
                    "data": {"text": "Looks good."},
                    "memberCreator": {"fullName": "Alice"},
                }
            ],
            "cards/c2/actions": [],
            "cards/c1/attachments": [
                {
                    "id": "a1",
                    "name": "spec.pdf",
                    "url": "https://trello.com/1/cards/c1/attachments/a1/download/spec.pdf",
                    "mimeType": "application/pdf",
                    "bytes": 1024,
                    "date": "2026-06-01T12:30:00.000Z",
                }
            ],
            "cards/c2/attachments": [],
        }
        return FakeResponse(payloads[path])


class FailingSession:
    def get(self, url: str, params: dict[str, Any], timeout: int) -> FakeResponse:
        del timeout
        response = FakeResponse({"error": "unauthorized"}, status_code=401)
        response.request = SimpleNamespace(
            url=f"{url}?key={params['key']}&token={params['token']}&fields=id,name"
        )
        return response


def make_connector() -> TrelloConnector:
    connector = TrelloConnector(api_base="https://trello.test/1")
    connector.session = FakeSession()
    connector.load_credentials(
        {
            "trello_api_key": "key",
            "trello_api_token": "token",
        }
    )
    return connector


@pytest.mark.p1
def test_trello_connector_builds_card_documents() -> None:
    connector = make_connector()

    batches = list(connector.load_from_state())

    assert len(batches) == 1
    assert len(batches[0]) == 2
    doc = batches[0][0]
    text = doc.blob.decode("utf-8")
    assert doc.id == "trello:c1"
    assert doc.source == "trello"
    assert doc.semantic_identifier == "Ship Trello connector"
    assert doc.metadata["board_name"] == "Roadmap"
    assert doc.metadata["list_name"] == "Doing"
    assert "Description:\nImplement card sync." in text
    assert "Alice" in text
    assert "spec.pdf" in text
    assert doc.fingerprint


@pytest.mark.p1
def test_trello_connector_poll_source_filters_by_date() -> None:
    connector = make_connector()
    start = datetime(2026, 5, 15, tzinfo=timezone.utc).timestamp()
    end = datetime(2026, 6, 2, tzinfo=timezone.utc).timestamp()

    batches = list(connector.poll_source(start, end))

    assert [[doc.id for doc in batch] for batch in batches] == [["trello:c1"]]


@pytest.mark.p1
def test_trello_connector_retrieves_slim_docs() -> None:
    connector = make_connector()

    batches = list(connector.retrieve_all_slim_docs_perm_sync())

    assert [[doc.id for doc in batch] for batch in batches] == [["trello:c1", "trello:c2"]]


@pytest.mark.p1
def test_trello_connector_falls_back_to_card_id_timestamp() -> None:
    connector = make_connector()
    expected = datetime(2026, 6, 1, 11, 0, tzinfo=timezone.utc)
    card_id = f"{int(expected.timestamp()):08x}0000000000000000"

    assert connector._parse_trello_datetime("not-a-date") is None
    assert connector._resolve_card_updated_at({"id": card_id, "dateLastActivity": "not-a-date"}) == expected
    assert connector._resolve_card_updated_at({"id": "nothex", "dateLastActivity": ""}) == datetime.fromtimestamp(
        0, tz=timezone.utc
    )


@pytest.mark.p1
def test_trello_connector_requires_credentials() -> None:
    connector = TrelloConnector(api_base="https://trello.test/1")

    with pytest.raises(ValueError, match="trello_api_key"):
        connector.validate_connector_settings()


@pytest.mark.p1
def test_trello_connector_redacts_credentials_from_http_errors() -> None:
    connector = TrelloConnector(api_base="https://trello.test/1")
    connector.session = FailingSession()
    connector.load_credentials(
        {
            "trello_api_key": "secret-key",
            "trello_api_token": "secret-token",
        }
    )

    with pytest.raises(requests.exceptions.HTTPError) as exc_info:
        connector.validate_connector_settings()

    message = str(exc_info.value)
    assert "secret-key" not in message
    assert "secret-token" not in message
    assert "fields=id%2Cname" in message

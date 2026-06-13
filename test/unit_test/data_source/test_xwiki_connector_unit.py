from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

import pytest

from common.data_source.xwiki_connector import XWikiConnector


class FakeResponse:
    def __init__(self, payload: dict[str, Any]) -> None:
        self.payload = payload

    def json(self) -> dict[str, Any]:
        return self.payload

    def raise_for_status(self) -> None:
        return None


class FakeSession:
    def __init__(self) -> None:
        self.auth: tuple[str, str] | None = None
        self.headers: dict[str, str] = {}
        self.calls: list[str] = []
        self.closed = False

    def close(self) -> None:
        self.closed = True

    def get(self, url: str, timeout: int) -> FakeResponse:
        del timeout
        if not url.startswith("https://xwiki.test/"):
            raise ValueError(f"unexpected XWiki test URL: {url}")
        path = url.split("https://xwiki.test/", 1)[1]
        self.calls.append(path)
        payloads = {
            "rest/wikis/xwiki/spaces/Main/pages": {
                "pageSummaries": [
                    {
                        "id": "xwiki:Main.WebHome",
                        "fullName": "Main.WebHome",
                        "title": "Home",
                        "modified": "2026-06-01T10:00:00Z",
                        "xwikiRelativeUrl": "bin/view/Main/",
                    }
                ]
            },
            "rest/wikis/xwiki/spaces/Main/pages/WebHome": {
                "id": "xwiki:Main.WebHome",
                "fullName": "Main.WebHome",
                "title": "Home",
                "content": "Runbooks and onboarding notes.",
                "modified": "2026-06-01T10:00:00Z",
                "xwikiRelativeUrl": "bin/view/Main/",
            },
        }
        if path not in payloads:
            raise KeyError(f"unexpected XWiki test path: url={url}, path={path}")
        return FakeResponse(payloads[path])


class InvalidTimestampSession(FakeSession):
    def get(self, url: str, timeout: int) -> FakeResponse:
        response = super().get(url, timeout)
        payload = response.json()
        if payload.get("fullName") == "Main.WebHome":
            payload = {**payload, "modified": "not-a-date"}
        return FakeResponse(payload)


def make_connector() -> XWikiConnector:
    connector = XWikiConnector("https://xwiki.test", space="Main")
    connector.session = FakeSession()
    connector.load_credentials({"xwiki_username": "user", "xwiki_password": "pass"})
    return connector


@pytest.mark.p1
def test_xwiki_connector_builds_page_documents() -> None:
    connector = make_connector()

    batches = list(connector.load_from_state())

    assert len(batches) == 1
    doc = batches[0][0]
    text = doc.blob.decode("utf-8")
    assert doc.id == "xwiki:xwiki:Main.WebHome"
    assert doc.source == "xwiki"
    assert doc.semantic_identifier == "Home"
    assert "Runbooks and onboarding notes." in text
    assert doc.metadata["wiki"] == "xwiki"
    assert doc.metadata["space"] == "Main"
    assert doc.metadata["url"] == "https://xwiki.test/bin/view/Main/"
    assert doc.fingerprint and len(doc.fingerprint) == 32


@pytest.mark.p1
def test_xwiki_connector_poll_source_filters_by_date() -> None:
    connector = make_connector()
    start = datetime(2026, 5, 30, tzinfo=timezone.utc).timestamp()
    end = datetime(2026, 6, 2, tzinfo=timezone.utc).timestamp()

    batches = list(connector.poll_source(start, end))

    assert [[doc.id for doc in batch] for batch in batches] == [["xwiki:xwiki:Main.WebHome"]]


@pytest.mark.p1
def test_xwiki_connector_poll_source_skips_invalid_timestamps() -> None:
    connector = make_connector()
    connector.session = InvalidTimestampSession()
    start = datetime(2026, 5, 30, tzinfo=timezone.utc).timestamp()
    end = datetime(2026, 6, 2, tzinfo=timezone.utc).timestamp()

    batches = list(connector.poll_source(start, end))

    assert batches == []


@pytest.mark.p1
def test_xwiki_connector_retrieves_slim_docs() -> None:
    connector = make_connector()

    batches = list(connector.retrieve_all_slim_docs_perm_sync())

    assert [[doc.id for doc in batch] for batch in batches] == [["xwiki:xwiki:Main.WebHome"]]


@pytest.mark.p1
def test_xwiki_connector_requires_base_url() -> None:
    connector = XWikiConnector("")

    with pytest.raises(ValueError, match="base_url"):
        connector.validate_connector_settings()


@pytest.mark.p1
def test_xwiki_connector_closes_session() -> None:
    connector = make_connector()
    session = connector.session

    connector.close()

    assert session.closed

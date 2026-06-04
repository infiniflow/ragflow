from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

import pytest

from common.data_source.opentext_connector import OpenTextConnector


class FakeResponse:
    def __init__(self, payload: Any = None, content: bytes = b"") -> None:
        self.payload = payload if payload is not None else {}
        self.content = content

    def json(self) -> Any:
        return self.payload

    def raise_for_status(self) -> None:
        return None


class FakeSession:
    def __init__(self) -> None:
        self.auth: tuple[str, str] | None = None
        self.headers: dict[str, str] = {}
        self.calls: list[str] = []

    def get(self, url: str, timeout: int) -> FakeResponse:
        del timeout
        path = url.split("https://otcs.test/", 1)[1]
        self.calls.append(path)
        if path == "api/v1/nodes/2000/content":
            return FakeResponse(content=b"Quarterly policy text")
        payloads = {
            "api/v1/nodes/1000": {
                "data": {"properties": {"id": 1000, "name": "Policies", "container": True, "type_name": "Folder"}}
            },
            "api/v1/nodes/1000/nodes": {
                "data": [
                    {
                        "data": {
                            "properties": {
                                "id": 2000,
                                "name": "policy.txt",
                                "type_name": "Document",
                                "mime_type": "text/plain",
                                "modify_date": "2026-06-01T10:00:00Z",
                            }
                        }
                    }
                ]
            },
            "api/v1/nodes/2000": {
                "data": {
                    "properties": {
                        "id": 2000,
                        "name": "policy.txt",
                        "type_name": "Document",
                        "mime_type": "text/plain",
                        "modify_date": "2026-06-01T10:00:00Z",
                    }
                }
            },
        }
        return FakeResponse(payloads[path])


def make_connector() -> OpenTextConnector:
    connector = OpenTextConnector("https://otcs.test", "1000")
    connector.session = FakeSession()
    connector.load_credentials({"opentext_ticket": "ticket"})
    return connector


@pytest.mark.p1
def test_opentext_connector_builds_document_nodes() -> None:
    connector = make_connector()

    batches = list(connector.load_from_state())

    assert len(batches) == 1
    doc = batches[0][0]
    assert doc.id == "opentext:2000"
    assert doc.source == "opentext"
    assert doc.semantic_identifier == "policy.txt"
    assert doc.blob == b"Quarterly policy text"
    assert doc.extension == "txt"
    assert doc.metadata["node_id"] == "2000"
    assert doc.fingerprint


@pytest.mark.p1
def test_opentext_connector_poll_source_filters_by_date() -> None:
    connector = make_connector()
    start = datetime(2026, 5, 30, tzinfo=timezone.utc).timestamp()
    end = datetime(2026, 6, 2, tzinfo=timezone.utc).timestamp()

    batches = list(connector.poll_source(start, end))

    assert [[doc.id for doc in batch] for batch in batches] == [["opentext:2000"]]


@pytest.mark.p1
def test_opentext_connector_retrieves_slim_docs() -> None:
    connector = make_connector()

    batches = list(connector.retrieve_all_slim_docs_perm_sync())

    assert [[doc.id for doc in batch] for batch in batches] == [["opentext:2000"]]


@pytest.mark.p1
def test_opentext_connector_requires_root_node_ids() -> None:
    connector = OpenTextConnector("https://otcs.test", "")

    with pytest.raises(ValueError, match="root_node_id"):
        connector.validate_connector_settings()

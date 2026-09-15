import json
from unittest.mock import MagicMock, patch

import pytest

from common.data_source.zotero_connector import ZoteroConnector, STORAGE_MODE_ZOTERO


def _attachment_item(key: str, modified: str) -> dict:
    return {
        "key": key,
        "data": {
            "itemType": "attachment",
            "title": "Sample Paper",
            "filename": "sample.pdf",
            "contentType": "application/pdf",
            "dateModified": modified,
            "parentItem": "PARENT1",
        },
    }


def test_zotero_connector_builds_pdf_document():
    connector = ZoteroConnector(zotero_user_id="12345678", batch_size=2)
    connector.load_credentials({"zotero_api_key": "secret"})

    items = [_attachment_item("ATTACH1", "2026-01-02T00:00:00Z")]

    with patch("common.data_source.zotero_connector.rl_requests.get") as mock_get:
        list_response = MagicMock()
        list_response.status_code = 200
        list_response.json.return_value = items
        file_response = MagicMock()
        file_response.status_code = 200
        file_response.content = b"%PDF-1.4 test"
        mock_get.side_effect = [list_response, file_response]

        batches = list(connector.load_from_state())

    assert len(batches) == 1
    assert len(batches[0]) == 1
    document = batches[0][0]
    assert document.id == "zotero:12345678:ATTACH1"
    assert document.extension == "pdf"
    assert document.blob == b"%PDF-1.4 test"


def test_zotero_connector_poll_filters_by_modified_time():
    connector = ZoteroConnector(zotero_user_id="12345678", batch_size=10)
    connector.load_credentials({"zotero_api_key": "secret"})

    items = [
        _attachment_item("OLD", "2024-01-01T00:00:00Z"),
        _attachment_item("NEW", "2026-01-02T00:00:00Z"),
    ]

    with patch("common.data_source.zotero_connector.rl_requests.get") as mock_get:
        list_response = MagicMock()
        list_response.status_code = 200
        list_response.json.return_value = items
        file_response = MagicMock()
        file_response.status_code = 200
        file_response.content = b"%PDF"
        mock_get.side_effect = [list_response, file_response]

        batches = list(
            connector.poll_source(
                start=1735689600,  # 2025-01-01
                end=1893456000,  # 2030-01-01
            )
        )

    assert len(batches) == 1
    assert len(batches[0]) == 1
    assert batches[0][0].id.endswith(":NEW")


def test_zotero_connector_requires_api_key():
    connector = ZoteroConnector(zotero_user_id="12345678")
    with pytest.raises(Exception):
        connector.load_credentials({})

from contextlib import nullcontext as _nullcontext
from unittest.mock import MagicMock, patch

import pytest

from common.data_source import zotero_connector as zotero_mod
from common.data_source.exceptions import ConnectorMissingCredentialError
from common.data_source.zotero_connector import ZoteroConnector


def _file_response(content: bytes) -> MagicMock:
    response = MagicMock()
    response.status_code = 200
    response.iter_content = lambda chunk_size=None: iter([content])
    response.close = MagicMock()
    return response


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


def test_zotero_connector_builds_pdf_document(monkeypatch):
    monkeypatch.setattr(
        zotero_mod,
        "assert_url_is_safe",
        lambda url, allowed_schemes=None: ("api.zotero.org", "1.2.3.4"),
    )
    monkeypatch.setattr(
        zotero_mod,
        "pin_dns",
        lambda host, ip: _nullcontext(),
    )
    connector = ZoteroConnector(zotero_user_id="12345678", batch_size=2)
    connector.load_credentials({"zotero_api_key": "secret"})

    items = [_attachment_item("ATTACH1", "2026-01-02T00:00:00Z")]

    with patch("common.data_source.zotero_connector.rl_requests.get") as mock_get:
        list_response = MagicMock()
        list_response.status_code = 200
        list_response.json.return_value = items
        mock_get.side_effect = [list_response, _file_response(b"%PDF-1.4 test")]

        batches = list(connector.load_from_state())

    assert len(batches) == 1
    assert len(batches[0]) == 1
    document = batches[0][0]
    assert document.id == "zotero:12345678:ATTACH1"
    assert document.extension == "pdf"
    assert document.blob == b"%PDF-1.4 test"


def test_zotero_connector_poll_filters_by_modified_time(monkeypatch):
    monkeypatch.setattr(
        zotero_mod,
        "assert_url_is_safe",
        lambda url, allowed_schemes=None: ("api.zotero.org", "1.2.3.4"),
    )
    monkeypatch.setattr(
        zotero_mod,
        "pin_dns",
        lambda host, ip: _nullcontext(),
    )
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
        mock_get.side_effect = [list_response, _file_response(b"%PDF")]

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
    with pytest.raises(ConnectorMissingCredentialError):
        connector.load_credentials({})


def test_zotero_connector_requires_user_id():
    connector = ZoteroConnector(zotero_user_id=None)
    connector.load_credentials({"zotero_api_key": "secret"})
    with pytest.raises(ConnectorMissingCredentialError, match="user ID"):
        connector.validate_local_settings()


def test_zotero_rejects_blocked_webdav_url(monkeypatch):
    def _blocked(url, allowed_schemes=None):
        if url.endswith("/zotero/"):
            raise ValueError("blocked host")
        return ("example.com", "1.2.3.4")

    monkeypatch.setattr(zotero_mod, "assert_url_is_safe", _blocked)
    connector = ZoteroConnector(
        zotero_user_id="12345678",
        storage_mode="webdav",
        webdav_url="https://example.com/dav",
    )
    connector.load_credentials({"zotero_api_key": "secret", "webdav_username": "dav", "webdav_password": "pw"})
    with pytest.raises(Exception, match="not allowed"):
        connector.validate_local_settings()


def test_zotero_rejects_http_webdav_url():
    connector = ZoteroConnector(
        zotero_user_id="12345678",
        storage_mode="webdav",
        webdav_url="http://example.com",
    )
    connector.load_credentials({"zotero_api_key": "secret", "webdav_username": "dav", "webdav_password": "pw"})
    with pytest.raises(Exception, match="HTTPS"):
        connector.validate_local_settings()


def test_zotero_download_retries_before_skip(monkeypatch):
    monkeypatch.setattr(
        zotero_mod,
        "assert_url_is_safe",
        lambda url, allowed_schemes=None: ("api.zotero.org", "1.2.3.4"),
    )
    monkeypatch.setattr(
        zotero_mod,
        "pin_dns",
        lambda host, ip: _nullcontext(),
    )
    monkeypatch.setattr(zotero_mod, "DOWNLOAD_RETRY_BACKOFF_SEC", 0)
    connector = ZoteroConnector(zotero_user_id="12345678", batch_size=10)
    connector.load_credentials({"zotero_api_key": "secret"})
    attempts = {"count": 0}

    def fake_download(_item):
        attempts["count"] += 1
        if attempts["count"] < 3:
            return b"", "paper.pdf"
        return b"%PDF", "paper.pdf"

    monkeypatch.setattr(connector, "_download_pdf", fake_download)
    items = [_attachment_item("ATTACH1", "2026-01-02T00:00:00Z")]
    with patch.object(connector, "_iter_pdf_attachments", return_value=iter(items)):
        batches = list(connector.load_from_state())
    assert len(batches) == 1
    assert attempts["count"] == 3


def test_zotero_yields_batches_incrementally(monkeypatch):
    monkeypatch.setattr(
        zotero_mod,
        "assert_url_is_safe",
        lambda url, allowed_schemes=None: ("api.zotero.org", "1.2.3.4"),
    )
    monkeypatch.setattr(
        zotero_mod,
        "pin_dns",
        lambda host, ip: _nullcontext(),
    )
    connector = ZoteroConnector(zotero_user_id="12345678", batch_size=1)
    connector.load_credentials({"zotero_api_key": "secret"})
    items = [
        _attachment_item("A1", "2026-01-02T00:00:00Z"),
        _attachment_item("A2", "2026-01-03T00:00:00Z"),
    ]
    with patch("common.data_source.zotero_connector.rl_requests.get") as mock_get:
        list_response = MagicMock()
        list_response.status_code = 200
        list_response.json.return_value = items
        mock_get.side_effect = [
            list_response,
            _file_response(b"%PDF"),
            _file_response(b"%PDF"),
        ]
        batches = list(connector.load_from_state())
    assert len(batches) == 2
    assert batches[0][0].id.endswith(":A1")
    assert batches[1][0].id.endswith(":A2")

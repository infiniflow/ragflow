"""Unit tests for OneDriveConnector."""

import pytest
from unittest.mock import MagicMock, patch, call

from common.data_source.onedrive_connector import OneDriveConnector, OneDriveCheckpoint
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    InsufficientPermissionsError,
    UnexpectedValidationError,
)


_GOOD_CREDS = {
    "tenant_id": "tenant-123",
    "client_id": "client-abc",
    "client_secret": "secret-xyz",
}


# ---------------------------------------------------------------------------
# load_credentials
# ---------------------------------------------------------------------------

@pytest.mark.p2
def test_load_credentials_missing_fields_raises():
    connector = OneDriveConnector()
    with pytest.raises(ConnectorMissingCredentialError):
        connector.load_credentials({"tenant_id": "t", "client_id": "c"})  # missing secret


@pytest.mark.p1
def test_load_credentials_success():
    connector = OneDriveConnector()
    mock_app = MagicMock()
    mock_app.acquire_token_for_client.return_value = {"access_token": "tok-abc"}

    with patch("common.data_source.onedrive_connector.msal.ConfidentialClientApplication", return_value=mock_app):
        result = connector.load_credentials(_GOOD_CREDS)

    assert result is None
    assert connector._access_token == "tok-abc"
    assert connector._tenant_id == "tenant-123"


@pytest.mark.p2
def test_load_credentials_msal_failure_raises():
    connector = OneDriveConnector()
    mock_app = MagicMock()
    mock_app.acquire_token_for_client.return_value = {
        "error": "invalid_client",
        "error_description": "AADSTS70011",
    }

    with patch("common.data_source.onedrive_connector.msal.ConfidentialClientApplication", return_value=mock_app):
        with pytest.raises(ConnectorMissingCredentialError, match="AADSTS70011"):
            connector.load_credentials(_GOOD_CREDS)


# ---------------------------------------------------------------------------
# validate_connector_settings
# ---------------------------------------------------------------------------

@pytest.mark.p2
def test_validate_without_credentials_raises():
    connector = OneDriveConnector()
    with pytest.raises(ConnectorMissingCredentialError):
        connector.validate_connector_settings()


@pytest.mark.p1
def test_validate_success():
    connector = OneDriveConnector()
    connector._access_token = "tok"

    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.ok = True
    mock_resp.json.return_value = {"value": [{"id": "drive-1"}]}

    with patch.object(connector, "_get", return_value=mock_resp):
        connector.validate_connector_settings()  # should not raise


@pytest.mark.p2
def test_validate_401_raises_missing_credential():
    connector = OneDriveConnector()
    connector._access_token = "expired"

    mock_resp = MagicMock()
    mock_resp.status_code = 401
    mock_resp.ok = False

    with patch.object(connector, "_get", return_value=mock_resp):
        with pytest.raises(ConnectorMissingCredentialError):
            connector.validate_connector_settings()


@pytest.mark.p2
def test_validate_403_raises_insufficient_permissions():
    connector = OneDriveConnector()
    connector._access_token = "tok"

    mock_resp = MagicMock()
    mock_resp.status_code = 403
    mock_resp.ok = False

    with patch.object(connector, "_get", return_value=mock_resp):
        with pytest.raises(InsufficientPermissionsError):
            connector.validate_connector_settings()


@pytest.mark.p2
def test_validate_unexpected_status_raises():
    connector = OneDriveConnector()
    connector._access_token = "tok"

    mock_resp = MagicMock()
    mock_resp.status_code = 500
    mock_resp.ok = False
    mock_resp.text = "internal error"

    with patch.object(connector, "_get", return_value=mock_resp):
        with pytest.raises(UnexpectedValidationError):
            connector.validate_connector_settings()


# ---------------------------------------------------------------------------
# Checkpoint helpers
# ---------------------------------------------------------------------------

@pytest.mark.p2
def test_build_dummy_checkpoint():
    connector = OneDriveConnector()
    ckpt = connector.build_dummy_checkpoint()
    assert isinstance(ckpt, OneDriveCheckpoint)
    assert ckpt.has_more is True
    assert ckpt.delta_links == {}


@pytest.mark.p2
def test_validate_checkpoint_json_invalid_returns_dummy():
    connector = OneDriveConnector()
    ckpt = connector.validate_checkpoint_json("not-json")
    assert isinstance(ckpt, OneDriveCheckpoint)


# ---------------------------------------------------------------------------
# _iter_documents (via poll_source)
# ---------------------------------------------------------------------------

@pytest.mark.p1
def test_poll_source_yields_supported_files():
    connector = OneDriveConnector(batch_size=10)
    connector._access_token = "tok"

    # drive list response
    drives_resp = MagicMock()
    drives_resp.ok = True
    drives_resp.json.return_value = {"value": [{"id": "drive-1"}]}

    # delta response with one docx file
    delta_resp = MagicMock()
    delta_resp.ok = True
    delta_resp.json.return_value = {
        "value": [
            {
                "id": "file-1",
                "name": "report.docx",
                "file": {},
                "lastModifiedDateTime": "2026-05-20T10:00:00Z",
                "webUrl": "https://example.com/report.docx",
                "size": 1024,
                "createdBy": {"user": {"displayName": "Alice"}},
            }
        ],
        "@odata.deltaLink": "https://graph.microsoft.com/delta-link",
    }

    get_calls = [drives_resp, delta_resp]
    with patch.object(connector, "_get", side_effect=get_calls):
        batches = list(connector.poll_source(0.0, 9999999999.0))

    assert len(batches) == 1
    assert len(batches[0]) == 1
    assert batches[0][0].semantic_identifier == "report.docx"


@pytest.mark.p2
def test_poll_source_skips_unsupported_extensions():
    connector = OneDriveConnector(batch_size=10)
    connector._access_token = "tok"

    drives_resp = MagicMock()
    drives_resp.ok = True
    drives_resp.json.return_value = {"value": [{"id": "drive-1"}]}

    delta_resp = MagicMock()
    delta_resp.ok = True
    delta_resp.json.return_value = {
        "value": [
            {
                "id": "img-1",
                "name": "photo.png",  # not in _SUPPORTED_EXTENSIONS
                "file": {},
                "lastModifiedDateTime": "2026-05-20T10:00:00Z",
                "webUrl": "https://example.com/photo.png",
                "size": 512,
            }
        ],
    }

    with patch.object(connector, "_get", side_effect=[drives_resp, delta_resp]):
        batches = list(connector.poll_source(0.0, 9999999999.0))

    assert batches == []


@pytest.mark.p2
def test_poll_source_skips_deleted_items():
    connector = OneDriveConnector(batch_size=10)
    connector._access_token = "tok"

    drives_resp = MagicMock()
    drives_resp.ok = True
    drives_resp.json.return_value = {"value": [{"id": "drive-1"}]}

    delta_resp = MagicMock()
    delta_resp.ok = True
    delta_resp.json.return_value = {
        "value": [
            {
                "id": "file-del",
                "name": "gone.docx",
                "file": {},
                "deleted": {"state": "deleted"},
            }
        ],
    }

    with patch.object(connector, "_get", side_effect=[drives_resp, delta_resp]):
        batches = list(connector.poll_source(0.0, 9999999999.0))

    assert batches == []

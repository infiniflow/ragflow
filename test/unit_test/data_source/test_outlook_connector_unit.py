"""Unit tests for OutlookConnector."""

import pytest
from unittest.mock import MagicMock, patch

from common.data_source.outlook_connector import (
    OutlookCheckpoint,
    OutlookConnector,
    _strip_html,
)
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
# _strip_html
# ---------------------------------------------------------------------------

@pytest.mark.p3
def test_strip_html_removes_tags_and_script():
    html = "<html><body><script>evil()</script><p>Hello <b>world</b></p></body></html>"
    assert "evil" not in _strip_html(html)
    assert "Hello world" in _strip_html(html)


@pytest.mark.p3
def test_strip_html_empty_returns_empty():
    assert _strip_html("") == ""


# ---------------------------------------------------------------------------
# load_credentials
# ---------------------------------------------------------------------------

@pytest.mark.p2
def test_load_credentials_missing_fields_raises():
    connector = OutlookConnector()
    with pytest.raises(ConnectorMissingCredentialError):
        connector.load_credentials({"tenant_id": "t", "client_id": "c"})


@pytest.mark.p1
def test_load_credentials_success():
    connector = OutlookConnector()
    mock_app = MagicMock()
    mock_app.acquire_token_for_client.return_value = {"access_token": "tok"}

    with patch(
        "common.data_source.outlook_connector.msal.ConfidentialClientApplication",
        return_value=mock_app,
    ):
        result = connector.load_credentials(_GOOD_CREDS)

    assert result is None
    assert connector._access_token == "tok"
    assert connector._tenant_id == "tenant-123"


@pytest.mark.p2
def test_load_credentials_msal_failure_raises():
    connector = OutlookConnector()
    mock_app = MagicMock()
    mock_app.acquire_token_for_client.return_value = {
        "error": "invalid_client",
        "error_description": "AADSTS70011",
    }

    with patch(
        "common.data_source.outlook_connector.msal.ConfidentialClientApplication",
        return_value=mock_app,
    ):
        with pytest.raises(ConnectorMissingCredentialError, match="AADSTS70011"):
            connector.load_credentials(_GOOD_CREDS)


# ---------------------------------------------------------------------------
# validate_connector_settings
# ---------------------------------------------------------------------------

@pytest.mark.p2
def test_validate_without_credentials_raises():
    connector = OutlookConnector()
    with pytest.raises(ConnectorMissingCredentialError):
        connector.validate_connector_settings()


@pytest.mark.p1
def test_validate_success_tenant_wide():
    connector = OutlookConnector()
    connector._access_token = "tok"

    mock_resp = MagicMock(status_code=200, ok=True)
    mock_resp.json.return_value = {"value": [{"id": "user-1"}]}

    with patch.object(connector, "_get", return_value=mock_resp) as mock_get:
        connector.validate_connector_settings()
        called_url = mock_get.call_args[0][0]
        assert "/users?$top=1" in called_url


@pytest.mark.p1
def test_validate_success_specific_user():
    connector = OutlookConnector(user_ids=["alice@example.com"])
    connector._access_token = "tok"

    mock_resp = MagicMock(status_code=200, ok=True)
    mock_resp.json.return_value = {"id": "user-1"}

    with patch.object(connector, "_get", return_value=mock_resp) as mock_get:
        connector.validate_connector_settings()
        called_url = mock_get.call_args[0][0]
        assert "alice@example.com" in called_url


@pytest.mark.p2
def test_validate_401_raises_missing_credential():
    connector = OutlookConnector()
    connector._access_token = "bad"
    mock_resp = MagicMock(status_code=401, ok=False)
    with patch.object(connector, "_get", return_value=mock_resp):
        with pytest.raises(ConnectorMissingCredentialError):
            connector.validate_connector_settings()


@pytest.mark.p2
def test_validate_403_raises_insufficient_permissions():
    connector = OutlookConnector()
    connector._access_token = "tok"
    mock_resp = MagicMock(status_code=403, ok=False)
    with patch.object(connector, "_get", return_value=mock_resp):
        with pytest.raises(InsufficientPermissionsError):
            connector.validate_connector_settings()


@pytest.mark.p2
def test_validate_404_with_user_ids_raises_validation_error():
    connector = OutlookConnector(user_ids=["ghost@example.com"])
    connector._access_token = "tok"
    mock_resp = MagicMock(status_code=404, ok=False)
    with patch.object(connector, "_get", return_value=mock_resp):
        with pytest.raises(ConnectorValidationError, match="ghost@example.com"):
            connector.validate_connector_settings()


@pytest.mark.p2
def test_validate_5xx_raises_unexpected():
    connector = OutlookConnector()
    connector._access_token = "tok"
    mock_resp = MagicMock(status_code=503, ok=False)
    mock_resp.text = "service unavailable"
    with patch.object(connector, "_get", return_value=mock_resp):
        with pytest.raises(UnexpectedValidationError):
            connector.validate_connector_settings()


# ---------------------------------------------------------------------------
# Checkpoint helpers
# ---------------------------------------------------------------------------

@pytest.mark.p2
def test_build_dummy_checkpoint():
    connector = OutlookConnector()
    ckpt = connector.build_dummy_checkpoint()
    assert isinstance(ckpt, OutlookCheckpoint)
    assert ckpt.has_more is True
    assert ckpt.delta_links == {}


@pytest.mark.p2
def test_validate_checkpoint_json_invalid_returns_dummy():
    connector = OutlookConnector()
    ckpt = connector.validate_checkpoint_json("garbage")
    assert isinstance(ckpt, OutlookCheckpoint)


# ---------------------------------------------------------------------------
# _list_user_ids
# ---------------------------------------------------------------------------

@pytest.mark.p2
def test_list_user_ids_returns_configured_ids():
    connector = OutlookConnector(user_ids=["a@x.com", "b@x.com"])
    connector._access_token = "tok"
    assert connector._list_user_ids() == ["a@x.com", "b@x.com"]


@pytest.mark.p2
def test_list_user_ids_paginates_when_unset():
    connector = OutlookConnector()
    connector._access_token = "tok"

    page_1 = MagicMock(ok=True)
    page_1.json.return_value = {
        "value": [
            {"id": "u1", "userPrincipalName": "u1@x.com", "mail": "u1@x.com"},
            {"id": "u2-no-mail"},  # filtered out (no mail, no UPN)
        ],
        "@odata.nextLink": "https://graph.example/next",
    }
    page_2 = MagicMock(ok=True)
    page_2.json.return_value = {
        "value": [{"id": "u3", "userPrincipalName": "u3@x.com"}],
    }

    with patch.object(connector, "_get", side_effect=[page_1, page_2]):
        ids = connector._list_user_ids()
    assert ids == ["u1", "u3"]


# ---------------------------------------------------------------------------
# _iter_documents (via poll_source)
# ---------------------------------------------------------------------------

@pytest.mark.p1
def test_poll_source_yields_messages():
    connector = OutlookConnector(
        batch_size=10, user_ids=["alice@example.com"]
    )
    connector._access_token = "tok"

    delta_resp = MagicMock(ok=True)
    delta_resp.json.return_value = {
        "value": [
            {
                "id": "msg-1",
                "subject": "Hello",
                "body": {"contentType": "text", "content": "Body text"},
                "receivedDateTime": "2026-05-20T10:00:00Z",
                "webLink": "https://outlook.office.com/mail/1",
                "from": {
                    "emailAddress": {"name": "Bob", "address": "bob@example.com"}
                },
                "toRecipients": [
                    {"emailAddress": {"address": "alice@example.com"}}
                ],
                "ccRecipients": [],
                "hasAttachments": False,
                "conversationId": "conv-1",
            }
        ],
        "@odata.deltaLink": "https://graph.example/delta-1",
    }

    with patch.object(connector, "_get", return_value=delta_resp):
        batches = list(connector.poll_source(0.0, 9999999999.0))

    assert len(batches) == 1
    doc = batches[0][0]
    assert doc.semantic_identifier == "Hello"
    assert "Bob" in doc.sections[0].text
    assert "Body text" in doc.sections[0].text
    assert doc.metadata["conversation_id"] == "conv-1"


@pytest.mark.p2
def test_poll_source_filters_old_messages():
    connector = OutlookConnector(
        batch_size=10, user_ids=["alice@example.com"]
    )
    connector._access_token = "tok"

    delta_resp = MagicMock(ok=True)
    delta_resp.json.return_value = {
        "value": [
            {
                "id": "old-msg",
                "subject": "old",
                "body": {"contentType": "text", "content": "x"},
                "receivedDateTime": "2020-01-01T00:00:00Z",
            }
        ],
    }

    with patch.object(connector, "_get", return_value=delta_resp):
        # since_epoch in 2030 -> 2020 message is older, must be skipped
        batches = list(connector.poll_source(1893456000.0, 9999999999.0))
    assert batches == []


@pytest.mark.p2
def test_poll_source_skips_removed_messages():
    connector = OutlookConnector(
        batch_size=10, user_ids=["alice@example.com"]
    )
    connector._access_token = "tok"

    delta_resp = MagicMock(ok=True)
    delta_resp.json.return_value = {
        "value": [
            {"id": "removed", "@removed": {"reason": "deleted"}},
            {
                "id": "kept",
                "subject": "kept",
                "body": {"contentType": "text", "content": "y"},
                "receivedDateTime": "2026-05-20T10:00:00Z",
            },
        ],
    }

    with patch.object(connector, "_get", return_value=delta_resp):
        batches = list(connector.poll_source(0.0, 9999999999.0))

    ids = [d.id for batch in batches for d in batch]
    assert ids == ["kept"]


@pytest.mark.p2
def test_poll_source_html_body_is_stripped():
    connector = OutlookConnector(
        batch_size=10, user_ids=["alice@example.com"]
    )
    connector._access_token = "tok"

    delta_resp = MagicMock(ok=True)
    delta_resp.json.return_value = {
        "value": [
            {
                "id": "html-msg",
                "subject": "html",
                "body": {
                    "contentType": "html",
                    "content": "<p>Hello <b>world</b></p>",
                },
                "receivedDateTime": "2026-05-20T10:00:00Z",
            }
        ],
    }

    with patch.object(connector, "_get", return_value=delta_resp):
        batches = list(connector.poll_source(0.0, 9999999999.0))

    text = batches[0][0].sections[0].text
    assert "<p>" not in text
    assert "Hello world" in text

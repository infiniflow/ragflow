"""Unit tests for OutlookConnector."""

import pytest
from unittest.mock import MagicMock, patch

from common.data_source.outlook_connector import (
    OutlookCheckpoint,
    OutlookConnector,
    _redact,
    _strip_html,
)
from common.data_source.models import SlimDocument
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
    connector = OutlookConnector(batch_size=10, user_ids=["alice@example.com"])
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
                "from": {"emailAddress": {"name": "Bob", "address": "bob@example.com"}},
                "toRecipients": [{"emailAddress": {"address": "alice@example.com"}}],
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
    body = doc.blob.decode("utf-8")
    assert "Bob" in body
    assert "Body text" in body
    assert doc.metadata["conversation_id"] == "conv-1"


@pytest.mark.p2
def test_poll_source_filters_old_messages():
    connector = OutlookConnector(batch_size=10, user_ids=["alice@example.com"])
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
    connector = OutlookConnector(batch_size=10, user_ids=["alice@example.com"])
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
    connector = OutlookConnector(batch_size=10, user_ids=["alice@example.com"])
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

    text = batches[0][0].blob.decode("utf-8")
    assert "<p>" not in text
    assert "Hello world" in text


# ---------------------------------------------------------------------------
# Non-2xx Graph responses must raise (no silent partial syncs)
# ---------------------------------------------------------------------------


def _ok(json_value):
    resp = MagicMock(ok=True, status_code=200)
    resp.json.return_value = json_value
    return resp


def _err(status, text=""):
    resp = MagicMock(ok=False, status_code=status)
    resp.text = text
    return resp


@pytest.mark.p1
def test_iter_documents_raises_on_http_500():
    """A 500 from the delta endpoint must surface; silently breaking would
    advance the checkpoint past data we never saw."""
    connector = OutlookConnector(batch_size=10, user_ids=["alice@example.com"])
    connector._access_token = "tok"
    with patch.object(connector, "_get", side_effect=[_err(500, "boom")]):
        with pytest.raises(UnexpectedValidationError):
            list(connector.poll_source(0.0, 9999999999.0))


@pytest.mark.p1
def test_iter_documents_raises_on_http_429():
    """Throttling must propagate so the orchestrator retries instead of
    treating the run as a clean empty sync."""
    connector = OutlookConnector(batch_size=10, user_ids=["alice@example.com"])
    connector._access_token = "tok"
    with patch.object(connector, "_get", side_effect=[_err(429, "throttled")]):
        with pytest.raises(UnexpectedValidationError):
            list(connector.poll_source(0.0, 9999999999.0))


@pytest.mark.p1
def test_list_user_ids_raises_on_http_error():
    connector = OutlookConnector()  # no user_ids -> hits Graph
    connector._access_token = "tok"
    with patch.object(connector, "_get", side_effect=[_err(503, "down")]):
        with pytest.raises(UnexpectedValidationError):
            connector._list_user_ids()


# ---------------------------------------------------------------------------
# retrieve_all_slim_docs_perm_sync: yields list[SlimDocument] for prune
# ---------------------------------------------------------------------------


@pytest.mark.p1
def test_retrieve_slim_docs_yields_slimdocument_batches():
    """The prune collector calls file_list.extend(batch) and reads `.id` on
    every retained item, so retrieve_all_slim_docs_perm_sync must yield
    lists of SlimDocument, not bare dicts."""
    connector = OutlookConnector(batch_size=2, user_ids=["alice@example.com"])
    connector._access_token = "tok"

    delta_resp = _ok(
        {
            "value": [
                {"id": "m1", "subject": "a"},
                {"id": "m2", "subject": "b"},
                {"id": "m3", "subject": "c"},
            ],
        }
    )

    with patch.object(connector, "_get", return_value=delta_resp):
        batches = list(connector.retrieve_all_slim_docs_perm_sync())

    assert len(batches) == 2
    assert [len(b) for b in batches] == [2, 1]
    flat = [item for batch in batches for item in batch]
    assert all(isinstance(item, SlimDocument) for item in flat)
    assert {item.id for item in flat} == {"m1", "m2", "m3"}


@pytest.mark.p2
def test_retrieve_slim_docs_skips_removed():
    connector = OutlookConnector(batch_size=10, user_ids=["alice@example.com"])
    connector._access_token = "tok"

    delta_resp = _ok(
        {
            "value": [
                {"id": "del", "@removed": {"reason": "deleted"}},
                {"id": "keep", "subject": "kept"},
            ],
        }
    )
    with patch.object(connector, "_get", return_value=delta_resp):
        batches = list(connector.retrieve_all_slim_docs_perm_sync())
    flat = [item for batch in batches for item in batch]
    assert [item.id for item in flat] == ["keep"]


@pytest.mark.p2
def test_retrieve_slim_docs_raises_on_http_error():
    connector = OutlookConnector(batch_size=10, user_ids=["alice@example.com"])
    connector._access_token = "tok"
    with patch.object(connector, "_get", side_effect=[_err(502, "bad gateway")]):
        with pytest.raises(UnexpectedValidationError):
            list(connector.retrieve_all_slim_docs_perm_sync())


@pytest.mark.p2
def test_retrieve_slim_docs_requires_credentials():
    connector = OutlookConnector()
    with pytest.raises(ConnectorMissingCredentialError):
        list(connector.retrieve_all_slim_docs_perm_sync())


# ---------------------------------------------------------------------------
# load_from_checkpoint: resumes from delta_links
# ---------------------------------------------------------------------------


@pytest.mark.p1
def test_load_from_checkpoint_uses_persisted_delta_link():
    """With a delta_link for a user the connector must hit that URL — not
    the per-user mailbox root — so incremental runs resume properly."""
    connector = OutlookConnector(batch_size=10, user_ids=["alice@example.com"])
    connector._access_token = "tok"

    saved = "https://graph.microsoft.com/v1.0/users/alice@example.com/delta?$skiptoken=ABC"
    ckpt = OutlookCheckpoint(has_more=True, delta_links={"alice@example.com": saved})

    visited: list[str] = []

    def _stub(url):
        visited.append(url)
        return _ok({"value": [], "@odata.deltaLink": "next-link"})

    with patch.object(connector, "_get", side_effect=_stub):
        list(connector.load_from_checkpoint(0.0, 0.0, ckpt))

    # First (and only) call must be the saved delta link.
    assert visited == [saved]
    assert ckpt.delta_links == {"alice@example.com": "next-link"}


# ---------------------------------------------------------------------------
# _redact: keep debugging hint, drop PII
# ---------------------------------------------------------------------------


@pytest.mark.p3
def test_redact_email_masks_local_and_domain():
    assert _redact("alice@example.com") == "al***@***"


@pytest.mark.p3
def test_redact_short_email_keeps_local():
    assert _redact("a@x.com") == "a@***"


@pytest.mark.p3
def test_redact_object_id_keeps_prefix():
    assert _redact("12345678-1234-1234-1234-123456789012") == "1234***"


@pytest.mark.p3
def test_redact_empty_value():
    assert _redact("") == "<empty>"
    assert _redact(None) == "<empty>"

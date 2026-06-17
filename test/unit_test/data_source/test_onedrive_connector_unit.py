"""Unit tests for OneDriveConnector."""

import pytest
from unittest.mock import MagicMock, patch

from common.data_source.onedrive_connector import OneDriveConnector, OneDriveCheckpoint
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

_GRAPH_BASE = "https://graph.microsoft.com/v1.0"


# ---------------------------------------------------------------------------
# folder_path / _delta_url
# ---------------------------------------------------------------------------

@pytest.mark.p2
def test_folder_path_prepends_leading_slash_for_delta_url():
    connector = OneDriveConnector(folder_path="Documents/Reports")
    assert connector.folder_path == "/Documents/Reports"
    assert connector._delta_url("drive-1") == (
        f"{_GRAPH_BASE}/drives/drive-1/root:/Documents/Reports:/delta"
    )


@pytest.mark.p2
def test_folder_path_preserves_leading_slash():
    connector = OneDriveConnector(folder_path="/Documents/Reports/")
    assert connector.folder_path == "/Documents/Reports"
    assert connector._delta_url("drive-1") == (
        f"{_GRAPH_BASE}/drives/drive-1/root:/Documents/Reports:/delta"
    )


@pytest.mark.p2
def test_folder_path_rejects_parent_segments():
    with pytest.raises(ConnectorValidationError, match="\\.\\."):
        OneDriveConnector(folder_path="/Documents/../secret")


@pytest.mark.p2
def test_folder_path_normalizes_consecutive_slashes():
    connector = OneDriveConnector(folder_path="//Documents//Reports")
    assert connector.folder_path == "/Documents/Reports"
    assert connector._delta_url("drive-1") == (
        f"{_GRAPH_BASE}/drives/drive-1/root:/Documents/Reports:/delta"
    )


@pytest.mark.p2
def test_folder_path_strips_whitespace():
    connector = OneDriveConnector(folder_path="  Documents/Reports  ")
    assert connector.folder_path == "/Documents/Reports"


@pytest.mark.p2
def test_folder_path_root_uses_drive_root_delta():
    connector = OneDriveConnector(folder_path="/")
    assert connector.folder_path is None
    assert connector._delta_url("drive-1") == f"{_GRAPH_BASE}/drives/drive-1/root/delta"


@pytest.mark.p2
def test_folder_path_double_slash_only_uses_drive_root_delta():
    connector = OneDriveConnector(folder_path="//")
    assert connector.folder_path is None
    assert connector._delta_url("drive-1") == f"{_GRAPH_BASE}/drives/drive-1/root/delta"


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

def _ok(json_value):
    """Tiny helper: build a successful MagicMock response with .ok / .json()."""
    resp = MagicMock()
    resp.ok = True
    resp.status_code = 200
    resp.json.return_value = json_value
    return resp


def _err(status: int, text: str = ""):
    """Tiny helper: build a non-ok MagicMock response for failure tests."""
    resp = MagicMock()
    resp.ok = False
    resp.status_code = status
    resp.text = text
    return resp


@pytest.mark.p1
def test_poll_source_yields_supported_files():
    connector = OneDriveConnector(batch_size=10)
    connector._access_token = "tok"

    drives_resp = _ok({"value": [{"id": "drive-1"}]})
    delta_resp = _ok({
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
    })

    with patch.object(connector, "_get", side_effect=[drives_resp, delta_resp]):
        batches = list(connector.poll_source(0.0, 9999999999.0))

    assert len(batches) == 1
    assert len(batches[0]) == 1
    assert batches[0][0].semantic_identifier == "report.docx"


@pytest.mark.p2
def test_poll_source_skips_unsupported_extensions():
    connector = OneDriveConnector(batch_size=10)
    connector._access_token = "tok"

    drives_resp = _ok({"value": [{"id": "drive-1"}]})
    delta_resp = _ok({
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
    })

    with patch.object(connector, "_get", side_effect=[drives_resp, delta_resp]):
        batches = list(connector.poll_source(0.0, 9999999999.0))

    assert batches == []


@pytest.mark.p2
def test_poll_source_skips_deleted_items():
    connector = OneDriveConnector(batch_size=10)
    connector._access_token = "tok"

    drives_resp = _ok({"value": [{"id": "drive-1"}]})
    delta_resp = _ok({
        "value": [
            {
                "id": "file-del",
                "name": "gone.docx",
                "file": {},
                "deleted": {"state": "deleted"},
            }
        ],
    })

    with patch.object(connector, "_get", side_effect=[drives_resp, delta_resp]):
        batches = list(connector.poll_source(0.0, 9999999999.0))

    assert batches == []


# ---------------------------------------------------------------------------
# Non-2xx Graph responses must raise (no silent partial syncs)
# ---------------------------------------------------------------------------

@pytest.mark.p1
def test_iter_documents_raises_on_graph_http_500():
    """A 500 from the delta endpoint must surface — silently breaking would
    advance the checkpoint past data we never saw."""
    connector = OneDriveConnector(batch_size=10)
    connector._access_token = "tok"

    drives_resp = _ok({"value": [{"id": "drive-1"}]})
    delta_resp = _err(500, "internal error")

    with patch.object(connector, "_get", side_effect=[drives_resp, delta_resp]):
        with pytest.raises(UnexpectedValidationError):
            list(connector.poll_source(0.0, 9999999999.0))


@pytest.mark.p1
def test_iter_documents_raises_on_graph_http_429():
    """Throttling must propagate so the orchestrator retries instead of
    treating the run as a clean empty sync."""
    connector = OneDriveConnector(batch_size=10)
    connector._access_token = "tok"

    drives_resp = _ok({"value": [{"id": "drive-1"}]})
    throttled = _err(429, "Too Many Requests")

    with patch.object(connector, "_get", side_effect=[drives_resp, throttled]):
        with pytest.raises(UnexpectedValidationError):
            list(connector.poll_source(0.0, 9999999999.0))


@pytest.mark.p1
def test_list_drive_ids_raises_on_http_error():
    connector = OneDriveConnector()
    connector._access_token = "tok"

    with patch.object(connector, "_get", side_effect=[_err(503, "unavailable")]):
        with pytest.raises(UnexpectedValidationError):
            connector._list_drive_ids()


# ---------------------------------------------------------------------------
# retrieve_all_slim_docs_perm_sync: yields SlimDocument batches for prune
# ---------------------------------------------------------------------------

@pytest.mark.p1
def test_retrieve_slim_docs_yields_slimdocument_batches():
    """The prune collector does file_list.extend(batch) and reads .id on each
    retained item, so retrieve_all_slim_docs_perm_sync must yield lists of
    SlimDocument (not lists of plain dicts and not bare dicts)."""
    connector = OneDriveConnector(batch_size=2)
    connector._access_token = "tok"

    drives_resp = _ok({"value": [{"id": "drive-1"}]})
    delta_resp = _ok({
        "value": [
            {"id": "f1", "name": "a.docx", "file": {}},
            {"id": "f2", "name": "b.pdf", "file": {}},
            {"id": "f3", "name": "c.txt", "file": {}},
        ],
    })

    with patch.object(connector, "_get", side_effect=[drives_resp, delta_resp]):
        batches = list(connector.retrieve_all_slim_docs_perm_sync())

    # batch_size=2 -> first batch has 2 items, second has the trailing one
    assert len(batches) == 2
    assert [len(b) for b in batches] == [2, 1]
    flat = [item for batch in batches for item in batch]
    assert all(isinstance(item, SlimDocument) for item in flat)
    assert {item.id for item in flat} == {"f1", "f2", "f3"}


@pytest.mark.p2
def test_retrieve_slim_docs_skips_folders_and_deleted():
    connector = OneDriveConnector(batch_size=10)
    connector._access_token = "tok"

    drives_resp = _ok({"value": [{"id": "drive-1"}]})
    delta_resp = _ok({
        "value": [
            {"id": "folder-1", "name": "Docs", "folder": {}},  # folder, no "file"
            {"id": "del-1", "name": "gone.pdf", "file": {}, "deleted": {"state": "deleted"}},
            {"id": "ok-1", "name": "keep.pdf", "file": {}},
        ],
    })

    with patch.object(connector, "_get", side_effect=[drives_resp, delta_resp]):
        batches = list(connector.retrieve_all_slim_docs_perm_sync())

    flat = [item for batch in batches for item in batch]
    assert [item.id for item in flat] == ["ok-1"]


@pytest.mark.p2
def test_retrieve_slim_docs_raises_on_http_error():
    connector = OneDriveConnector(batch_size=10)
    connector._access_token = "tok"

    drives_resp = _ok({"value": [{"id": "drive-1"}]})
    failed = _err(502, "bad gateway")

    with patch.object(connector, "_get", side_effect=[drives_resp, failed]):
        with pytest.raises(UnexpectedValidationError):
            list(connector.retrieve_all_slim_docs_perm_sync())


@pytest.mark.p2
def test_retrieve_slim_docs_requires_credentials():
    connector = OneDriveConnector()
    # _access_token is None
    with pytest.raises(ConnectorMissingCredentialError):
        list(connector.retrieve_all_slim_docs_perm_sync())


# ---------------------------------------------------------------------------
# load_from_checkpoint: resumes from delta_links and honors start floor
# ---------------------------------------------------------------------------

@pytest.mark.p1
def test_load_from_checkpoint_uses_persisted_delta_link():
    """When the checkpoint carries a delta_link for a drive, the connector
    must hit THAT URL — not the drive root — so incremental runs resume
    from where the previous one left off."""
    connector = OneDriveConnector(batch_size=10)
    connector._access_token = "tok"

    saved_delta = "https://graph.microsoft.com/v1.0/drives/drive-1/root/delta?token=ABC"
    ckpt = OneDriveCheckpoint(has_more=True, delta_links={"drive-1": saved_delta})

    drives_resp = _ok({"value": [{"id": "drive-1"}]})
    delta_resp = _ok({"value": [], "@odata.deltaLink": "next-link"})

    visited: list[str] = []

    def _stub_get(url):
        visited.append(url)
        return drives_resp if url.endswith("/drives") else delta_resp

    with patch.object(connector, "_get", side_effect=_stub_get):
        list(connector.load_from_checkpoint(0.0, 0.0, ckpt))

    # Second call (after /drives) is the delta fetch — it must be the saved
    # delta link, not the drive root.
    assert visited[1] == saved_delta
    assert ckpt.delta_links == {"drive-1": "next-link"}

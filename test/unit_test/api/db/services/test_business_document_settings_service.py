import json
from types import SimpleNamespace
from unittest.mock import Mock

import pytest

from api.db.services import business_document_settings_service as service


@pytest.fixture
def stored(monkeypatch):
    values = {}
    monkeypatch.setattr(service.SystemSettingsService, "get_by_name", lambda name: [SimpleNamespace(value=values[name])] if name in values else [])
    monkeypatch.setenv("RAGFLOW_CREDENTIALS_KEY", "test-documents-key")
    return values


def config(**changes):
    return {"api_base_url": "https://eva.example/", "web_base_url": "https://eva.example/", "project_id": "CmfProject:docs", "eva_api_token": "shared-secret", **changes}


def save(stored, value):
    result = service.prepare_business_documents_connection(value)
    stored[service.BUSINESS_DOCUMENTS_EVA_CONNECTION_SETTING] = json.dumps(result)
    return result


def test_connection_works_without_creating_connector_and_never_serializes_token(stored, monkeypatch):
    access = Mock(side_effect=AssertionError("No data source should be used"))
    monkeypatch.setattr(service.Connector, "get_or_none", access)
    result = save(stored, config())
    assert len(result["id"]) == 32
    assert "shared-secret" not in repr(stored)
    assert "encrypted_token" not in repr(service.get_business_documents_settings())
    assert "shared-secret" not in repr(service.get_business_documents_settings())
    assert service.get_business_documents_settings()["eva_connection"]["token_configured"]
    assert "credentials" not in service.get_documents_eva_connection().config
    assert service.get_documents_eva_connection(with_token=True).config["credentials"]["eva_api_token"] == "shared-secret"
    assert service.get_business_documents_eva_connector_id() == result["id"]
    access.assert_not_called()


def test_blank_token_preserves_secret_and_clear_removes_it(stored):
    first = save(stored, config())
    second = save(stored, config(eva_api_token="  ", verify_ssl=False))
    assert first["id"] == second["id"]
    assert service.get_documents_eva_connection(with_token=True).config["credentials"]["eva_api_token"] == "shared-secret"
    save(stored, config(clear_token=True))
    assert not service.get_business_documents_settings()["eva_connection"]["token_configured"]


def test_new_api_does_not_receive_old_secret_and_changes_binding_id(stored):
    first = save(stored, config())
    second = save(stored, config(api_base_url="https://another.example", eva_api_token=""))
    assert first["id"] != second["id"]
    assert not second["encrypted_token"]


def test_new_space_changes_binding_id(stored):
    first = save(stored, config())
    second = save(stored, config(project_id="CmfProject:other", eva_api_token=""))
    assert first["id"] != second["id"]


def test_disable_does_not_fall_back_to_old_source(stored):
    stored[service.BUSINESS_DOCUMENTS_EVA_CONNECTOR_SETTING] = "old-connector"
    stored[service.BUSINESS_DOCUMENTS_EVA_CONNECTION_SETTING] = "{}"
    assert service.get_documents_eva_connection() is None
    assert service.get_business_documents_eva_connector_id() is None
    assert service.get_business_documents_settings()["eva_connection"]["api_base_url"] == ""


def test_import_existing_connection_keeps_persisted_binding_id(stored, monkeypatch):
    old_config = config()
    old_config["credentials"] = {"eva_api_token": old_config.pop("eva_api_token")}
    old = SimpleNamespace(id="old-connector", name="Source", source="eva_wiki", config=old_config)
    stored[service.BUSINESS_DOCUMENTS_EVA_CONNECTOR_SETTING] = old.id
    monkeypatch.setattr(service.Connector, "get_or_none", lambda *_args: old)
    form = service.get_business_documents_settings()["eva_connection"]
    assert form["token_configured"]
    result = save(stored, form)
    assert result["id"] == old.id
    assert service.get_documents_eva_connection(with_token=True).config["credentials"]["eva_api_token"] == "shared-secret"
    assert old.config == old_config


@pytest.mark.parametrize(
    "changes",
    [
        {"api_base_url": "file:///tmp/eva"},
        {"api_base_url": "https://user:password@eva.example"},
        {"api_base_url": "https://eva.example:bad"},
        {"web_base_url": "https://eva.example#bad"},
        {"project_id": ""},
        {"verify_ssl": "false"},
        {"include_archived": 1},
        {"eva_api_token": None},
        {"clear_token": "false"},
    ],
)
def test_invalid_settings_are_rejected(stored, changes):
    with pytest.raises(ValueError):
        save(stored, config(**changes))
    assert not stored


def test_cannot_decrypt_token_with_another_key(stored, monkeypatch):
    save(stored, config())
    monkeypatch.setenv("RAGFLOW_CREDENTIALS_KEY", "another-key")
    with pytest.raises(service.ConnectorValidationError, match="could not be decrypted"):
        service.get_documents_eva_connection(with_token=True)
    assert service.get_business_documents_settings()["eva_connection"]["token_configured"]


def test_discovery_uses_draft_settings_and_does_not_save(stored, monkeypatch):
    client = Mock()
    client.list_projects.return_value = [{"id": "CmfProject:docs", "name": "Documents"}]
    constructor = Mock(return_value=client)
    monkeypatch.setattr(service, "EvaWikiConnector", constructor)
    assert service.discover_business_documents_eva_spaces(config(project_id="")) == client.list_projects.return_value
    client.load_credentials.assert_called_once_with({"eva_api_token": "shared-secret"})
    client._session.close.assert_called_once()
    assert constructor.call_args.kwargs["include_attachments"] is False
    assert not stored


def test_replacing_unreadable_token_recovers_connection(stored, monkeypatch):
    first = save(stored, config())
    monkeypatch.setenv("RAGFLOW_CREDENTIALS_KEY", "replacement-key")
    second = save(stored, config(eva_api_token="new-secret"))
    assert second["id"] == first["id"]
    assert service.get_documents_eva_connection(with_token=True).config["credentials"]["eva_api_token"] == "new-secret"

#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

from pathlib import Path
import sys
from types import ModuleType, SimpleNamespace
from unittest.mock import patch

import pytest
from peewee import SqliteDatabase

if "api.apps" not in sys.modules:
    api_apps = ModuleType("api.apps")
    api_apps.__path__ = [str(Path(__file__).resolve().parents[5] / "api" / "apps")]
    sys.modules["api.apps"] = api_apps

from api.apps.business_documents.errors import BusinessDocumentError
from api.apps.business_documents.eva_changes import EvaChangeState, EvaDocumentChangeService
from api.db.db_models import BusinessDocumentEvaChange, BusinessDocumentEvaChangeEvent, Connector
from api.db.services.connector_service import ConnectorService
from api.db.services.user_external_credential_service import (
    ExternalCredentialDecryptionError,
    ExternalCredentialMissingError,
    UserExternalCredentialService,
)
from common.time_utils import current_timestamp


TENANT = "tenant-1"
AUTHOR = "author-1"
CONNECTOR = SimpleNamespace(id="connector-1", name="EVA Wiki", config={})
EVA_CHANGES_MODULE = sys.modules[EvaDocumentChangeService.__module__]


class FakeEvaClient:
    def __init__(self):
        self.publish_calls = 0
        self.document = {
            "id": "CmfDocument:doc-1",
            "name": "Переводы одной кнопкой",
            "code": "BR-42",
            "project_id": "CmfProject:portal",
            "version": "1|CmfVersion:v1|2026-08-26T09:00:00+03:00",
            "modified_at": "2026-08-26T09:00:00+03:00",
            "web_url": "https://eva.example.com/project/Document/BR-42",
            "html": "<h1>Бизнес-требования</h1><h2>Цель</h2><p>Старый текст.</p>",
            "draft_html": "",
        }

    def get_document_for_edit(self, document_id):
        assert document_id == self.document["id"]
        return dict(self.document)

    def update_document_draft(self, document_id, html, name=None):
        assert document_id == self.document["id"]
        self.document["draft_html"] = html
        if name is not None:
            self.document["name"] = name
        return True

    def publish_document(self, document_id):
        assert document_id == self.document["id"]
        self.publish_calls += 1
        self.document["html"] = self.document["draft_html"]
        self.document["version"] = "2|CmfVersion:v2|2026-08-26T10:00:00+03:00"
        return True


class FakeEvaMutationClient:
    def __init__(self, document):
        self.document = document
        self.update_calls = 0
        self.publish_calls = 0

    def update_document_draft(self, document_id, html, name=None):
        assert document_id == self.document["id"]
        self.update_calls += 1
        self.document["draft_html"] = html
        if name is not None:
            self.document["name"] = name
        return True

    def publish_document(self, document_id):
        assert document_id == self.document["id"]
        self.publish_calls += 1
        self.document["html"] = self.document["draft_html"]
        self.document["version"] = "2|CmfVersion:v2|2026-08-26T10:00:00+03:00"
        return True


class FakeEvaChangeAI:
    def __init__(self, draft_markdown, summary="Подготовлена тестовая доработка"):
        self.draft_markdown = draft_markdown
        self.summary = summary
        self.calls = []

    def generate_eva_change(self, tenant_id, base_markdown, change_request):
        self.calls.append((tenant_id, base_markdown, change_request))
        return {
            "schema_version": "1",
            "draft_markdown": self.draft_markdown,
            "summary": self.summary,
        }


@pytest.fixture()
def database():
    database = SqliteDatabase(":memory:")
    tables = [BusinessDocumentEvaChange, BusinessDocumentEvaChangeEvent]
    with database.bind_ctx(tables, bind_refs=False, bind_backrefs=False):
        database.connect()
        database.create_tables(tables)
        yield database
        database.drop_tables(tables)
        database.close()


@pytest.fixture(autouse=True)
def personal_mutation_client(monkeypatch):
    original = EvaDocumentChangeService._mutation_client
    monkeypatch.setattr(
        EvaDocumentChangeService,
        "_mutation_client",
        staticmethod(lambda connector, _actor_id: (connector.mutation_client, 1)),
    )
    return original


@pytest.fixture(autouse=True)
def configured_documents_eva_space(monkeypatch):
    monkeypatch.setattr(EVA_CHANGES_MODULE, "get_documents_eva_connection", lambda **_kwargs: None)
    monkeypatch.setattr(
        EVA_CHANGES_MODULE,
        "get_business_documents_eva_connector_id",
        lambda: CONNECTOR.id,
    )
    monkeypatch.setattr(
        ConnectorService,
        "accessible",
        staticmethod(lambda _connector_id, _actor_id: True),
    )


def _create(client):
    CONNECTOR.mutation_client = client
    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        return EvaDocumentChangeService.create_change(
            TENANT,
            AUTHOR,
            {
                "connector_id": CONNECTOR.id,
                "document_id": client.document["id"],
                "change_summary": "Уточнить ожидаемый результат и ограничения.",
            },
        )


def _generate(created, draft_markdown, refinement=""):
    return EvaDocumentChangeService.generate_draft(
        TENANT,
        AUTHOR,
        created["change_id"],
        {
            "expected_state_version": created["state_version"],
            **({"refinement": refinement} if refinement else {}),
        },
        ai=FakeEvaChangeAI(draft_markdown),
    )


def _eva_connector(credentials):
    return SimpleNamespace(
        id="connector-1",
        name="EVA Wiki",
        source="eva_wiki",
        config={
            "api_base_url": "https://eva.example.com/api",
            "web_base_url": "https://eva.example.com",
            "project_id": "CmfProject:portal",
            "credentials": credentials,
        },
    )


def _patch_accessible_connector(monkeypatch, connector):
    monkeypatch.setattr(
        ConnectorService,
        "get_by_id",
        staticmethod(lambda connector_id: (connector_id == connector.id, connector)),
    )
    monkeypatch.setattr(
        ConnectorService,
        "accessible",
        staticmethod(lambda connector_id, actor_id: connector_id == connector.id and actor_id == AUTHOR),
    )


def test_eva_search_requires_configured_documents_space(monkeypatch):
    monkeypatch.setattr(
        EVA_CHANGES_MODULE,
        "get_business_documents_eva_connector_id",
        lambda: None,
    )

    with pytest.raises(BusinessDocumentError) as exc_info:
        EvaDocumentChangeService.search_sources(AUTHOR)

    assert exc_info.value.code == "EVA_SPACE_NOT_CONFIGURED"
    assert exc_info.value.status == 409


def test_eva_connector_rejects_document_outside_configured_space(monkeypatch):
    monkeypatch.setattr(
        EVA_CHANGES_MODULE,
        "get_business_documents_eva_connector_id",
        lambda: "connector-documents",
    )

    with pytest.raises(BusinessDocumentError) as exc_info:
        EvaDocumentChangeService._connector("connector-other", AUTHOR)

    assert exc_info.value.code == "EVA_SPACE_SCOPE_VIOLATION"
    assert exc_info.value.status == 403


@pytest.mark.parametrize(
    "credentials",
    [
        {},
        {"eva_api_token": ""},
        {"eva_api_token": "   "},
    ],
)
def test_eva_reader_falls_back_to_personal_token_when_shared_token_is_blank(monkeypatch, credentials):
    connector = _eva_connector(credentials)
    _patch_accessible_connector(monkeypatch, connector)
    credential_requests = []

    def get_personal_token(actor_id, api_base_url):
        credential_requests.append((actor_id, api_base_url))
        return SimpleNamespace(secret="personal-token")

    monkeypatch.setattr(UserExternalCredentialService, "get_eva_wiki_token", staticmethod(get_personal_token))

    _, reader = EvaDocumentChangeService._connector(connector.id, AUTHOR)

    assert reader._session.headers["X-Eva-Token"] == "personal-token"
    assert credential_requests == [(AUTHOR, "https://eva.example.com/api")]


def test_eva_reader_prefers_shared_token_without_loading_personal_credential(monkeypatch):
    connector = _eva_connector({"eva_api_token": "shared-token"})
    _patch_accessible_connector(monkeypatch, connector)

    def reject_personal_lookup(*_args):
        raise AssertionError("personal credential must not be loaded when the shared token is configured")

    monkeypatch.setattr(UserExternalCredentialService, "get_eva_wiki_token", staticmethod(reject_personal_lookup))

    _, reader = EvaDocumentChangeService._connector(connector.id, AUTHOR)

    assert reader._session.headers["X-Eva-Token"] == "shared-token"


@pytest.mark.parametrize(
    ("credential_error", "expected_code", "expected_status"),
    [
        (ExternalCredentialMissingError("missing"), "EVA_CREDENTIALS_MISSING", 422),
        (ExternalCredentialDecryptionError("unavailable"), "EVA_USER_TOKEN_UNAVAILABLE", 503),
    ],
)
def test_eva_reader_maps_missing_and_unavailable_personal_credentials(
    monkeypatch,
    credential_error,
    expected_code,
    expected_status,
):
    connector = _eva_connector({})
    _patch_accessible_connector(monkeypatch, connector)

    def fail_personal_lookup(*_args):
        raise credential_error

    monkeypatch.setattr(UserExternalCredentialService, "get_eva_wiki_token", staticmethod(fail_personal_lookup))

    with pytest.raises(BusinessDocumentError) as exc_info:
        EvaDocumentChangeService.search_sources(AUTHOR)

    assert exc_info.value.code == expected_code
    assert exc_info.value.status == expected_status


def test_resolve_page_url_materializes_connectors_before_nested_access_queries(monkeypatch):
    connector = SimpleNamespace(id="connector-1", name="EVA Wiki")

    class CursorSensitiveQuery:
        iterating = False

        def where(self, *_args):
            return self

        def order_by(self, *_args):
            return self

        def __iter__(self):
            self.iterating = True
            try:
                yield connector
            finally:
                self.iterating = False

    query = CursorSensitiveQuery()
    monkeypatch.setattr(Connector, "select", lambda *_args, **_kwargs: query)

    def accessible(_connector_id, _actor_id):
        assert query.iterating is False
        return True

    monkeypatch.setattr(ConnectorService, "accessible", staticmethod(accessible))
    monkeypatch.setattr(
        EvaDocumentChangeService,
        "_connector",
        classmethod(lambda _cls, _connector_id, _actor_id: (connector, SimpleNamespace(web_base_url="https://other.example.com"))),
    )

    binding = EvaDocumentChangeService.resolve_page_url(AUTHOR, "https://eva.example.com/project/Document/BR-42")

    assert binding["status"] == "LINK_ONLY"
    assert binding["capabilities"] == ["OPEN"]


def test_resolve_page_url_accepts_configured_api_origin_and_returns_web_url(monkeypatch):
    connector = SimpleNamespace(id="connector-1", name="EVA Wiki")

    class Query:
        def where(self, *_args):
            return self

        def order_by(self, *_args):
            return self

        def __iter__(self):
            yield connector

    client = SimpleNamespace(
        api_base_url="http://host.docker.internal:8084",
        web_base_url="https://eva.example.com",
        search_documents=lambda query, _limit: [
            {
                "id": "CmfDocument:doc-1",
                "code": query,
                "web_url": f"https://eva.example.com/project/Document/{query}",
            }
        ],
        get_document_for_edit=lambda _document_id: {
            "id": "CmfDocument:doc-1",
            "code": "DOC-001883",
            "name": "Документ1",
            "project_id": "CmfProject:business-documents",
            "web_url": "https://eva.example.com/project/Document/DOC-001883",
            "version": "1",
            "html": "<p>Требования</p>",
        },
    )
    monkeypatch.setattr(Connector, "select", lambda *_args, **_kwargs: Query())
    monkeypatch.setattr(ConnectorService, "accessible", staticmethod(lambda *_args: True))
    monkeypatch.setattr(
        EvaDocumentChangeService,
        "_connector",
        classmethod(lambda _cls, _connector_id, _actor_id: (connector, client)),
    )

    binding = EvaDocumentChangeService.resolve_page_url(
        AUTHOR,
        "http://host.docker.internal:8084//project/Document/DOC-001883",
    )

    assert binding["status"] == "CONNECTED"
    assert binding["page_url"] == "https://eva.example.com/project/Document/DOC-001883"
    assert binding["document_id"] == "CmfDocument:doc-1"


def test_resolve_page_url_rejects_embedded_credentials():
    with pytest.raises(BusinessDocumentError) as exc_info:
        EvaDocumentChangeService.resolve_page_url(
            AUTHOR,
            "https://user:password@eva.example.com/project/Document/BR-42",
        )

    assert exc_info.value.code == "INVALID_EVA_PAGE_URL"


def test_full_eva_change_flow_keeps_publish_as_separate_action(database):
    client = FakeEvaClient()
    created = _create(client)

    assert created["workflow_state"] == "EDITING"
    assert created["diff"]["changed"] is False
    assert created["allowed_actions"] == ["GENERATE_DRAFT"]

    draft = _generate(
        created,
        "# Бизнес-требования\n\n## Цель\n\nНовый проверяемый текст.",
    )
    assert draft["diff"]["changed_sections"] == 1
    assert draft["diff"]["added_lines"] == 1
    assert draft["diff"]["removed_lines"] == 1
    assert draft["allowed_actions"] == ["GENERATE_DRAFT", "APPROVE"]

    approved = EvaDocumentChangeService.approve(
        TENANT,
        AUTHOR,
        created["change_id"],
        {"expected_state_version": draft["state_version"]},
    )
    assert approved["workflow_state"] == "APPROVED"
    assert approved["allowed_actions"] == ["PREPARE_EVA_DRAFT"]
    assert client.document["draft_html"] == ""

    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        prepared = EvaDocumentChangeService.prepare_eva_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": approved["state_version"]},
        )
    assert prepared["workflow_state"] == "EVA_DRAFT_READY"
    assert prepared["allowed_actions"] == ["PUBLISH_EVA"]
    assert "Новый проверяемый текст" in client.document["draft_html"]
    assert "Старый текст" in client.document["html"]

    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        published = EvaDocumentChangeService.publish(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": prepared["state_version"]},
        )
    assert published["workflow_state"] == "PUBLISHED"
    assert published["allowed_actions"] == []
    assert published["published_version"].startswith("2|")
    assert "Новый проверяемый текст" in client.document["html"]
    assert [event["event_type"] for event in published["events"]] == [
        "CHANGE_REQUEST_CREATED",
        "AI_DRAFT_REQUESTED",
        "AI_DRAFT_GENERATED",
        "DRAFT_APPROVED",
        "EVA_DRAFT_SAVED",
        "EVA_DOCUMENT_PUBLISHED",
    ]


def test_state_transitions_load_eva_settings_outside_transactions(database, monkeypatch):
    def configured_connector_id():
        assert not database.in_transaction()
        return CONNECTOR.id

    monkeypatch.setattr(
        EVA_CHANGES_MODULE,
        "get_business_documents_eva_connector_id",
        configured_connector_id,
    )
    client = FakeEvaClient()
    created = _create(client)
    draft = _generate(
        created,
        "# Бизнес-требования\n\n## Цель\n\nТранзакционно безопасный текст.",
    )
    approved = EvaDocumentChangeService.approve(
        TENANT,
        AUTHOR,
        created["change_id"],
        {"expected_state_version": draft["state_version"]},
    )
    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        prepared = EvaDocumentChangeService.prepare_eva_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": approved["state_version"]},
        )
        published = EvaDocumentChangeService.publish(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": prepared["state_version"]},
        )

    assert published["workflow_state"] == "PUBLISHED"


def test_agent_generation_uses_refinement_and_manual_prefill_is_rejected(database):
    client = FakeEvaClient()
    created = _create(client)
    ai = FakeEvaChangeAI("# Бизнес-требования\n\n## Цель\n\nИзмеримый результат.")

    generated = EvaDocumentChangeService.generate_draft(
        TENANT,
        AUTHOR,
        created["change_id"],
        {
            "expected_state_version": created["state_version"],
            "refinement": "Добавить измеримый результат.",
        },
        ai=ai,
    )

    assert ai.calls == [
        (
            TENANT,
            created["base_markdown"],
            "Уточнить ожидаемый результат и ограничения.\n\nУточнение автора:\nДобавить измеримый результат.",
        )
    ]
    assert generated["change_summary"].endswith("Добавить измеримый результат.")
    assert generated["draft_markdown"].endswith("Измеримый результат.")
    assert "GENERATE_DRAFT" in generated["allowed_actions"]

    with (
        patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)),
        pytest.raises(BusinessDocumentError) as exc_info,
    ):
        EvaDocumentChangeService.create_change(
            TENANT,
            AUTHOR,
            {
                "connector_id": CONNECTOR.id,
                "document_id": client.document["id"],
                "change_summary": "Обойти агента.",
                "draft_markdown": "# Ручной текст",
            },
        )

    assert exc_info.value.code == "MANUAL_EVA_DRAFT_DISABLED"
    assert exc_info.value.message == "Нельзя передать готовый текст доработки EVA: его формирует агент по задаче автора."


def test_unchanged_agent_result_restores_retryable_state(database):
    client = FakeEvaClient()
    created = _create(client)

    with pytest.raises(BusinessDocumentError) as exc_info:
        _generate(created, created["base_markdown"])

    assert exc_info.value.code == "EVA_AI_DRAFT_UNCHANGED"
    assert exc_info.value.message == "Агент не изменил документ. Уточните, что именно нужно исправить, и повторите подготовку."
    restored = EvaDocumentChangeService.get_change(TENANT, AUTHOR, created["change_id"])
    assert restored["workflow_state"] == "EDITING"
    assert restored["last_error"]["code"] == "EVA_AI_DRAFT_UNCHANGED"
    assert restored["last_error"]["message"] == exc_info.value.message
    assert restored["allowed_actions"] == ["GENERATE_DRAFT"]


def test_agent_failure_explains_retry_and_restores_editing_state(database):
    client = FakeEvaClient()
    created = _create(client)

    class FailingEvaChangeAI:
        @staticmethod
        def generate_eva_change(*_args):
            raise RuntimeError("provider failed")

    with pytest.raises(BusinessDocumentError) as exc_info:
        EvaDocumentChangeService.generate_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": created["state_version"]},
            ai=FailingEvaChangeAI(),
        )

    assert exc_info.value.code == "EVA_AI_GENERATION_FAILED"
    assert exc_info.value.message == ("Агент не смог подготовить доработку EVA. Повторите попытку; если ошибка сохранится, обратитесь к администратору.")
    restored = EvaDocumentChangeService.get_change(TENANT, AUTHOR, created["change_id"])
    assert restored["workflow_state"] == "EDITING"
    assert restored["last_error"]["message"] == exc_info.value.message
    assert restored["allowed_actions"] == ["GENERATE_DRAFT"]


def test_generation_refinement_limit_is_explained_in_russian(database):
    client = FakeEvaClient()
    created = _create(client)

    with pytest.raises(BusinessDocumentError) as exc_info:
        EvaDocumentChangeService.generate_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {
                "expected_state_version": created["state_version"],
                "refinement": "x" * 10_001,
            },
        )

    assert exc_info.value.code == "INVALID_EVA_CHANGE"
    assert exc_info.value.message == "Поле «уточнение для агента» не должно превышать 10 000 символов."


def test_eva_writes_use_personal_mutation_client_while_reads_use_connector(database, monkeypatch):
    reader = FakeEvaClient()
    writer = FakeEvaMutationClient(reader.document)
    created = _create(reader)
    draft = _generate(
        created,
        "# Бизнес-требования\n\n## Цель\n\nИзменение пользователя.",
    )
    approved = EvaDocumentChangeService.approve(
        TENANT,
        AUTHOR,
        created["change_id"],
        {"expected_state_version": draft["state_version"]},
    )
    monkeypatch.setattr(
        EvaDocumentChangeService,
        "_mutation_client",
        staticmethod(lambda _connector, _actor_id: (writer, 7)),
    )

    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, reader)):
        prepared = EvaDocumentChangeService.prepare_eva_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": approved["state_version"]},
        )
        published = EvaDocumentChangeService.publish(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": prepared["state_version"]},
        )

    assert writer.update_calls == 1
    assert writer.publish_calls == 1
    assert published["workflow_state"] == "PUBLISHED"
    assert published["events"][-2]["payload"]["user_credential_version"] == 7
    assert published["events"][-1]["payload"]["user_credential_version"] == 7


def test_missing_personal_eva_token_blocks_write_and_restores_state(database, monkeypatch):
    reader = FakeEvaClient()
    created = _create(reader)
    draft = _generate(
        created,
        "# Бизнес-требования\n\n## Цель\n\nИзменение пользователя.",
    )
    approved = EvaDocumentChangeService.approve(
        TENANT,
        AUTHOR,
        created["change_id"],
        {"expected_state_version": draft["state_version"]},
    )
    monkeypatch.setattr(
        EvaDocumentChangeService,
        "_mutation_client",
        staticmethod(lambda _connector, _actor_id: (_ for _ in ()).throw(ExternalCredentialMissingError("missing"))),
    )

    with (
        patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, reader)),
        pytest.raises(BusinessDocumentError) as exc_info,
    ):
        EvaDocumentChangeService.prepare_eva_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": approved["state_version"]},
        )

    assert exc_info.value.code == "EVA_USER_TOKEN_MISSING"
    restored = EvaDocumentChangeService.get_change(TENANT, AUTHOR, created["change_id"])
    assert restored["workflow_state"] == "APPROVED"
    assert reader.document["draft_html"] == ""


def test_change_can_start_from_an_agreed_business_document_revision(database):
    client = FakeEvaClient()
    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        created = EvaDocumentChangeService.create_change(
            TENANT,
            AUTHOR,
            {
                "connector_id": CONNECTOR.id,
                "document_id": client.document["id"],
                "document_name": "Новые бизнес-требования",
                "change_summary": "Синхронизация согласованной ревизии.",
                "draft_markdown": "# Бизнес-требования\n\n## Цель\n\nТекст из конструктора.",
            },
            allow_prefilled_draft=True,
        )

    assert created["workflow_state"] == "EDITING"
    assert created["diff"]["changed"] is True
    assert "APPROVE" in created["allowed_actions"]
    assert created["draft_markdown"].endswith("Текст из конструктора.")
    assert created["source"]["document_name"] == "Новые бизнес-требования"


def test_agreed_business_document_can_initialize_and_publish_empty_eva_page(database):
    client = FakeEvaClient()
    client.document["html"] = ""
    client.document["draft_html"] = ""
    client.document["version"] = "1||2026-08-26T09:00:00+03:00"
    CONNECTOR.mutation_client = client

    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        created = EvaDocumentChangeService.create_change(
            TENANT,
            AUTHOR,
            {
                "connector_id": CONNECTOR.id,
                "document_id": client.document["id"],
                "document_name": "Первоначальные бизнес-требования",
                "change_summary": "Первоначальная публикация согласованной ревизии.",
                "draft_markdown": ("# Бизнес-требования\n\n## Цель\n\nПервоначальный текст.\n\n```plantuml\n@startuml\nПользователь -> Система: Войти\n@enduml\n```"),
            },
            allow_prefilled_draft=True,
        )

    assert created["base_markdown"] == ""
    assert created["diff"]["changed"] is True
    assert "APPROVE" in created["allowed_actions"]

    approved = EvaDocumentChangeService.approve(
        TENANT,
        AUTHOR,
        created["change_id"],
        {"expected_state_version": created["state_version"]},
    )
    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        prepared = EvaDocumentChangeService.prepare_eva_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": approved["state_version"]},
        )
        published = EvaDocumentChangeService.publish(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": prepared["state_version"]},
        )

    assert published["workflow_state"] == "PUBLISHED"
    assert client.document["name"] == "Первоначальные бизнес-требования"
    assert "Первоначальный текст" in client.document["html"]


def test_empty_eva_initial_publish_recovers_after_prior_verification_failure(database):
    client = FakeEvaClient()
    client.document["html"] = ""
    client.document["draft_html"] = ""
    client.document["version"] = "1||2026-08-26T09:00:00+03:00"
    CONNECTOR.mutation_client = client

    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        created = EvaDocumentChangeService.create_change(
            TENANT,
            AUTHOR,
            {
                "connector_id": CONNECTOR.id,
                "document_id": client.document["id"],
                "change_summary": "Повтор первоначальной публикации.",
                "draft_markdown": "# Документ\n\n```plantuml\n@startuml\nA -> B\n@enduml\n```",
            },
            allow_prefilled_draft=True,
        )
    approved = EvaDocumentChangeService.approve(
        TENANT,
        AUTHOR,
        created["change_id"],
        {"expected_state_version": created["state_version"]},
    )

    stored = BusinessDocumentEvaChange.get_by_id(created["change_id"])
    client.document["draft_html"] = stored.draft_html
    client.document["html"] = stored.draft_html
    client.document["version"] = "2|CmfVersion:v2|2026-08-26T10:00:00+03:00"

    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        prepared = EvaDocumentChangeService.prepare_eva_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": approved["state_version"]},
        )
        published = EvaDocumentChangeService.publish(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": prepared["state_version"]},
        )

    assert prepared["workflow_state"] == "EVA_DRAFT_READY"
    assert published["workflow_state"] == "PUBLISHED"
    assert client.publish_calls == 0


def test_empty_eva_page_still_requires_an_initial_draft(database):
    client = FakeEvaClient()
    client.document["html"] = ""

    with (
        patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)),
        pytest.raises(BusinessDocumentError) as exc_info,
    ):
        EvaDocumentChangeService.create_change(
            TENANT,
            AUTHOR,
            {
                "connector_id": CONNECTOR.id,
                "document_id": client.document["id"],
                "change_summary": "Изменение без исходного текста.",
            },
        )

    assert exc_info.value.code == "EVA_DOCUMENT_EMPTY"


def test_prepare_rejects_changed_published_source_and_keeps_it_untouched(database):
    client = FakeEvaClient()
    created = _create(client)
    draft = _generate(
        created,
        "# Бизнес-требования\n\n## Цель\n\nНовый текст.",
    )
    approved = EvaDocumentChangeService.approve(
        TENANT,
        AUTHOR,
        created["change_id"],
        {"expected_state_version": draft["state_version"]},
    )
    client.document["html"] = "<h1>Бизнес-требования</h1><p>Изменено другим автором.</p>"

    with (
        patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)),
        pytest.raises(BusinessDocumentError) as exc_info,
    ):
        EvaDocumentChangeService.prepare_eva_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": approved["state_version"]},
        )

    assert exc_info.value.code == "EVA_SOURCE_VERSION_CONFLICT"
    assert exc_info.value.details["confirmation_required"] is True
    assert exc_info.value.details["confirmation_action"] == "OVERWRITE_EVA_DOCUMENT"
    restored = EvaDocumentChangeService.get_change(TENANT, AUTHOR, created["change_id"])
    assert restored["workflow_state"] == "APPROVED"
    assert restored["last_error"]["code"] == "EVA_SOURCE_VERSION_CONFLICT"
    assert client.document["draft_html"] == ""


def test_prepare_force_overwrites_changed_published_source(database):
    client = FakeEvaClient()
    created = _create(client)
    draft = _generate(
        created,
        "# Бизнес-требования\n\n## Цель\n\nПодтверждённая перезапись.",
    )
    approved = EvaDocumentChangeService.approve(
        TENANT,
        AUTHOR,
        created["change_id"],
        {"expected_state_version": draft["state_version"]},
    )
    client.document["html"] = "<h1>Чужое изменение</h1>"

    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        prepared = EvaDocumentChangeService.prepare_eva_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {
                "expected_state_version": approved["state_version"],
                "force_overwrite": True,
            },
        )

    assert prepared["workflow_state"] == "EVA_DRAFT_READY"
    assert "Подтверждённая перезапись" in client.document["draft_html"]


def test_force_overwrite_must_be_boolean(database):
    client = FakeEvaClient()
    created = _create(client)
    draft = _generate(
        created,
        "# Бизнес-требования\n\nИзменение.",
    )
    approved = EvaDocumentChangeService.approve(
        TENANT,
        AUTHOR,
        created["change_id"],
        {"expected_state_version": draft["state_version"]},
    )

    with pytest.raises(BusinessDocumentError) as exc_info:
        EvaDocumentChangeService.prepare_eva_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {
                "expected_state_version": approved["state_version"],
                "force_overwrite": "true",
            },
        )

    assert exc_info.value.code == "INVALID_EVA_CHANGE"


def test_publish_changed_source_requires_confirmation_and_force_overwrites(database):
    client = FakeEvaClient()
    created = _create(client)
    draft = _generate(
        created,
        "# Бизнес-требования\n\n## Цель\n\nПодтверждённая публикация.",
    )
    approved = EvaDocumentChangeService.approve(
        TENANT,
        AUTHOR,
        created["change_id"],
        {"expected_state_version": draft["state_version"]},
    )
    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        prepared = EvaDocumentChangeService.prepare_eva_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": approved["state_version"]},
        )

    client.document["html"] = "<h1>Чужое изменение после подготовки</h1>"
    client.document["version"] = "2|CmfVersion:external|2026-08-26T10:00:00+03:00"

    with (
        patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)),
        pytest.raises(BusinessDocumentError) as exc_info,
    ):
        EvaDocumentChangeService.publish(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": prepared["state_version"]},
        )

    assert exc_info.value.code == "EVA_SOURCE_VERSION_CONFLICT"
    assert exc_info.value.details["confirmation_required"] is True
    restored = EvaDocumentChangeService.get_change(TENANT, AUTHOR, created["change_id"])
    assert restored["workflow_state"] == "EVA_DRAFT_READY"

    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        published = EvaDocumentChangeService.publish(
            TENANT,
            AUTHOR,
            created["change_id"],
            {
                "expected_state_version": restored["state_version"],
                "force_overwrite": True,
            },
        )

    assert published["workflow_state"] == "PUBLISHED"
    assert "Подтверждённая публикация" in client.document["html"]
    assert client.document["name"] == created["source"]["document_name"]


def test_publish_requires_confirmation_when_only_eva_name_changed(database):
    client = FakeEvaClient()
    created = _create(client)
    draft = _generate(
        created,
        "# Бизнес-требования\n\n## Цель\n\nУже опубликованный текст.",
    )
    approved = EvaDocumentChangeService.approve(
        TENANT,
        AUTHOR,
        created["change_id"],
        {"expected_state_version": draft["state_version"]},
    )
    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        prepared = EvaDocumentChangeService.prepare_eva_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": approved["state_version"]},
        )

    client.document["html"] = client.document["draft_html"]
    client.document["name"] = "Чужое название"
    client.document["version"] = "2|CmfVersion:external|2026-08-26T10:00:00+03:00"

    with (
        patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)),
        pytest.raises(BusinessDocumentError) as exc_info,
    ):
        EvaDocumentChangeService.publish(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": prepared["state_version"]},
        )

    assert exc_info.value.code == "EVA_SOURCE_VERSION_CONFLICT"
    assert exc_info.value.details["actual_name"] == "Чужое название"
    assert client.publish_calls == 0


def test_markdown_html_is_sanitized_before_eva_draft(database):
    client = FakeEvaClient()
    created = _create(client)
    draft = _generate(
        created,
        "# Title\n\n<script>alert(1)</script>\n\n[bad](javascript:alert(2))",
    )

    stored = BusinessDocumentEvaChange.get_by_id(draft["change_id"])
    assert "<script" not in stored.draft_html
    assert "javascript:" not in stored.draft_html


def test_publish_recovers_when_eva_committed_before_client_error(database):
    client = FakeEvaClient()
    created = _create(client)
    draft = _generate(
        created,
        "# Бизнес-требования\n\n## Цель\n\nОпубликованный текст.",
    )
    approved = EvaDocumentChangeService.approve(
        TENANT,
        AUTHOR,
        created["change_id"],
        {"expected_state_version": draft["state_version"]},
    )
    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        prepared = EvaDocumentChangeService.prepare_eva_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": approved["state_version"]},
        )

    publish_document = client.publish_document

    def publish_then_fail(document_id):
        publish_document(document_id)
        raise RuntimeError("response lost after EVA committed the publish")

    with (
        patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)),
        patch.object(client, "publish_document", side_effect=publish_then_fail),
    ):
        published = EvaDocumentChangeService.publish(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": prepared["state_version"]},
        )

    assert published["workflow_state"] == "PUBLISHED"
    assert published["published_version"].startswith("2|")
    assert client.publish_calls == 1


def test_stale_publishing_reservation_can_resume_after_process_loss(database):
    client = FakeEvaClient()
    created = _create(client)
    draft = _generate(
        created,
        "# Бизнес-требования\n\n## Цель\n\nТекст после восстановления.",
    )
    approved = EvaDocumentChangeService.approve(
        TENANT,
        AUTHOR,
        created["change_id"],
        {"expected_state_version": draft["state_version"]},
    )
    with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
        prepared = EvaDocumentChangeService.prepare_eva_draft(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": approved["state_version"]},
        )

    reserved_version = prepared["state_version"] + 1
    BusinessDocumentEvaChange.update(
        workflow_state="PUBLISHING",
        state_version=reserved_version,
        update_time=current_timestamp(),
    ).where(BusinessDocumentEvaChange.id == created["change_id"]).execute()

    busy = EvaDocumentChangeService.get_change(TENANT, AUTHOR, created["change_id"])
    assert busy["allowed_actions"] == []
    with (
        patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)),
        pytest.raises(BusinessDocumentError) as exc_info,
    ):
        EvaDocumentChangeService.publish(
            TENANT,
            AUTHOR,
            created["change_id"],
            {"expected_state_version": reserved_version},
        )
    assert exc_info.value.code == "EVA_CHANGE_BUSY"

    client.document["html"] = client.document["draft_html"]
    client.document["version"] = "2|CmfVersion:v2|2026-08-26T10:00:00+03:00"
    reserved = BusinessDocumentEvaChange.get_by_id(created["change_id"])
    retry_time = int(reserved.update_time) + 120_001

    with patch("api.apps.business_documents.eva_changes.current_timestamp", return_value=retry_time):
        retryable = EvaDocumentChangeService.get_change(TENANT, AUTHOR, created["change_id"])
        assert retryable["workflow_state"] == "PUBLISHING"
        assert retryable["operation_retry_after_ms"] == 0
        assert retryable["allowed_actions"] == ["PUBLISH_EVA"]
        with patch.object(EvaDocumentChangeService, "_connector", return_value=(CONNECTOR, client)):
            published = EvaDocumentChangeService.publish(
                TENANT,
                AUTHOR,
                created["change_id"],
                {"expected_state_version": reserved_version},
            )

    assert published["workflow_state"] == "PUBLISHED"
    assert client.publish_calls == 0
    assert "EXTERNAL_OPERATION_RETRIED" in [event["event_type"] for event in published["events"]]

    EvaDocumentChangeService._restore_after_external_failure(
        created["change_id"],
        reserved_version,
        EvaChangeState.PUBLISHING,
        EvaChangeState.EVA_DRAFT_READY,
        BusinessDocumentError("LATE_FAILURE", "late failure", 502),
    )
    assert EvaDocumentChangeService.get_change(TENANT, AUTHOR, created["change_id"])["workflow_state"] == "PUBLISHED"


def test_standalone_connection_search_and_binding_do_not_use_data_sources(monkeypatch):
    from api.db.services.business_document_settings_service import DocumentsEvaConnection

    connection = DocumentsEvaConnection(id="documents-connection", config=_eva_connector({"eva_api_token": "shared"}).config)
    monkeypatch.setattr(EVA_CHANGES_MODULE, "get_business_documents_eva_connector_id", lambda: connection.id)
    monkeypatch.setattr(EVA_CHANGES_MODULE, "get_documents_eva_connection", lambda **_kwargs: connection)

    def unexpected(*_args):
        raise AssertionError("Standalone Documents must not require a data source")

    monkeypatch.setattr(ConnectorService, "accessible", unexpected)
    monkeypatch.setattr(ConnectorService, "get_by_id", unexpected)
    remote = FakeEvaClient().document
    monkeypatch.setattr(EVA_CHANGES_MODULE.EvaWikiConnector, "search_documents", lambda *_args: [{"id": remote["id"], "code": remote["code"], "web_url": remote["web_url"]}])
    monkeypatch.setattr(EVA_CHANGES_MODULE.EvaWikiConnector, "get_document_for_edit", lambda *_args: remote)
    result = EvaDocumentChangeService.search_sources(AUTHOR)
    assert result["items"][0]["connector_id"] == connection.id
    binding = EvaDocumentChangeService.resolve_page_url(AUTHOR, remote["web_url"])
    assert binding["status"] == "CONNECTED"
    assert binding["connector_id"] == connection.id
    with pytest.raises(BusinessDocumentError) as error:
        EvaDocumentChangeService._connector("old-connection", AUTHOR)
    assert error.value.code == "EVA_SPACE_SCOPE_VIOLATION"


def test_standalone_writes_use_personal_token_only(monkeypatch, personal_mutation_client):
    from api.db.services.business_document_settings_service import DocumentsEvaConnection
    from unittest.mock import Mock

    connection = DocumentsEvaConnection(id="documents-connection", config=_eva_connector({"eva_api_token": "shared-read-token"}).config)
    credential = SimpleNamespace(secret="personal-write-token", credential_version=7)
    get_token = Mock(return_value=credential)
    monkeypatch.setattr(UserExternalCredentialService, "get_eva_wiki_token", get_token)
    client = Mock()
    monkeypatch.setattr(EVA_CHANGES_MODULE, "EvaWikiMutationClient", Mock(return_value=client))
    writer, version = personal_mutation_client(connection, AUTHOR)
    get_token.assert_called_once_with(AUTHOR, connection.config["api_base_url"])
    assert writer is client and version == 7
    client.load_credentials.assert_called_once_with({"eva_api_token": "personal-write-token"})

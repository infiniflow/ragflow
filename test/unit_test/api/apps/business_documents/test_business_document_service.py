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

from copy import deepcopy
from pathlib import Path
import sys
from types import ModuleType

import pytest
from peewee import IntegrityError, SqliteDatabase

# Importing a submodule normally executes api.apps, which boots the complete
# server and external document store. Domain tests deliberately isolate that
# package boundary.
if "api.apps" not in sys.modules:
    api_apps = ModuleType("api.apps")
    api_apps.__path__ = [str(Path(__file__).resolve().parents[5] / "api" / "apps")]
    sys.modules["api.apps"] = api_apps

from api.apps.business_documents.assets import published_template, render_document_ast, render_section_text, section_hash, validate_document_ast
from api.apps.business_documents.errors import BusinessDocumentError
from api.apps.business_documents.service import BusinessDocumentService
from api.apps.business_documents.worker import BusinessDocumentJobQueue
from api.db.db_models import (
    BusinessDocument,
    BusinessDocumentAnswer,
    BusinessDocumentCommand,
    BusinessDocumentComment,
    BusinessDocumentEvent,
    BusinessDocumentEvaBinding,
    BusinessDocumentJob,
    BusinessDocumentProposal,
    BusinessDocumentProposalDecision,
    BusinessDocumentQuestion,
    BusinessDocumentRevision,
    User,
)
from test.unit_test.api.apps.business_documents.helpers import VALID_ACTIVITY_SCENARIO, required_section_blocks


TENANT = "tenant-1"
AUTHOR = "author-1"


@pytest.fixture()
def database():
    database = SqliteDatabase(":memory:")
    tables = (*BusinessDocumentService.model_tables(), User)
    with database.bind_ctx(tables, bind_refs=False, bind_backrefs=False):
        database.connect()
        database.create_tables(tables)
        yield database
        database.drop_tables(tables)
        database.close()


def _create(**overrides):
    request = {
        "schema_version": "1",
        "document_type": "business_requirements",
        "title": "Переводы одной кнопкой",
        "idea": "Создать безопасный сервис переводов",
        **overrides,
    }
    return BusinessDocumentService.create_document(TENANT, AUTHOR, request)


@pytest.mark.p0
def test_document_titles_are_unique_after_unicode_case_and_whitespace_normalization(database):
    first = _create(title="  Требования   CRM  ")

    with pytest.raises(BusinessDocumentError) as caught:
        _create(title="ТРЕБОВАНИЯ CRM")

    assert caught.value.code == "DOCUMENT_TITLE_ALREADY_EXISTS"
    assert BusinessDocument.select().count() == 1
    assert first["title"] == "Требования CRM"


@pytest.mark.p0
def test_v1_cannot_bind_an_eva_page_without_the_v2_confirmation_contract(database):
    with pytest.raises(BusinessDocumentError) as caught:
        _create(eva_page_url="https://eva.example.com/project/Document/BR-42")

    assert caught.value.code == "EVA_BINDING_REQUIRES_SCHEMA_V2"
    assert BusinessDocument.select().count() == 0


@pytest.mark.p0
def test_v2_creation_requires_an_explicit_decision_for_matching_eva_pages(database, monkeypatch):
    from api.apps.business_documents.eva_changes import EvaDocumentChangeService

    match = {
        "id": "CmfDocument:doc-1",
        "name": "Переводы одной кнопкой",
        "code": "BR-42",
        "project_id": "CmfProject:portal",
        "web_url": "https://eva.example.com/project/Document/BR-42",
        "breadcrumbs": [{"id": "CmfDocument:doc-1", "name": "Переводы одной кнопкой", "web_url": "https://eva.example.com/project/Document/BR-42"}],
        "hierarchy": "Переводы одной кнопкой",
        "connector_id": "connector-1",
        "connector_name": "EVA Wiki",
        "eva_origin": "https://eva.example.com",
    }
    monkeypatch.setattr(EvaDocumentChangeService, "find_title_matches", staticmethod(lambda *_args: [match]))

    with pytest.raises(BusinessDocumentError) as caught:
        _create(schema_version="2")

    assert caught.value.code == "EVA_BINDING_DECISION_REQUIRED"
    assert caught.value.details["matches"][0]["binding_available"] is True
    assert BusinessDocument.select().count() == 0


@pytest.mark.p0
def test_v2_skip_creates_the_document_without_an_eva_binding(database, monkeypatch):
    from api.apps.business_documents.eva_changes import EvaDocumentChangeService

    monkeypatch.setattr(
        EvaDocumentChangeService,
        "find_title_matches",
        staticmethod(lambda *_args: pytest.fail("SKIP must not repeat EVA title search")),
    )

    created = _create(schema_version="2", eva_decision={"mode": "SKIP"})

    assert created["eva_binding"] is None
    assert BusinessDocumentEvaBinding.select().count() == 0


@pytest.mark.p0
def test_v2_binding_requires_replace_confirmation_and_matching_remote_title(database, monkeypatch):
    from api.apps.business_documents.eva_changes import EvaDocumentChangeService

    binding = {
        "page_url": "https://eva.example.com/project/Document/BR-42",
        "status": "CONNECTED",
        "connector_id": "connector-1",
        "eva_origin": "https://eva-api.example.com",
        "project_id": "CmfProject:portal",
        "document_id": "CmfDocument:doc-1",
        "document_name": "Переводы одной кнопкой",
    }
    monkeypatch.setattr(EvaDocumentChangeService, "resolve_page_url", staticmethod(lambda *_args: binding))

    with pytest.raises(BusinessDocumentError) as missing_confirmation:
        _create(
            schema_version="2",
            eva_page_url=binding["page_url"],
            eva_decision={"mode": "BIND", "confirm_replace": False},
        )

    assert missing_confirmation.value.code == "EVA_REPLACE_CONFIRMATION_REQUIRED"
    created = _create(
        schema_version="2",
        eva_page_url=binding["page_url"],
        eva_decision={"mode": "BIND", "confirm_replace": True},
    )
    assert created["eva_binding"]["document_id"] == "CmfDocument:doc-1"


@pytest.mark.p0
def test_one_eva_page_cannot_be_linked_to_different_documents(database, monkeypatch):
    binding = {
        "page_url": "https://eva.example.com/project/Document/BR-42",
        "status": "CONNECTED",
        "capabilities": ["OPEN", "PULL_FROM_EVA", "CREATE_EVA_CHANGE"],
        "connector_id": "connector-1",
        "eva_origin": "https://eva-api.example.com",
        "project_id": "CmfProject:portal",
        "document_id": "CmfDocument:doc-1",
        "document_code": "BR-42",
        "document_name": "Переводы одной кнопкой",
    }
    monkeypatch.setattr(BusinessDocumentService, "_resolve_create_eva_binding", staticmethod(lambda *_args: binding))
    confirmed = {"mode": "BIND", "confirm_replace": True}
    _create(schema_version="2", eva_page_url=binding["page_url"], eva_decision=confirmed)

    with pytest.raises(BusinessDocumentError) as caught:
        _create(title="Другой документ", schema_version="2", eva_page_url=binding["page_url"], eva_decision=confirmed)

    assert caught.value.code == "EVA_PAGE_ALREADY_LINKED"
    assert BusinessDocument.select().count() == 1
    assert BusinessDocumentEvaBinding.select().count() == 1


def test_legacy_event_backfill_populates_unique_eva_binding_projection(database, monkeypatch):
    from api.apps.business_documents.eva_changes import EvaDocumentChangeService
    from api.db.db_models import migrate_business_document_eva_bindings

    binding = {
        "page_url": "https://eva.example.com/project/Document/BR-42",
        "status": "CONNECTED",
        "connector_id": "connector-1",
        "eva_origin": "https://eva-api.example.com",
        "project_id": "CmfProject:portal",
        "document_id": "CmfDocument:doc-1",
        "document_name": "Переводы одной кнопкой",
    }
    monkeypatch.setattr(EvaDocumentChangeService, "resolve_page_url", staticmethod(lambda *_args: binding))
    created = _create(
        schema_version="2",
        eva_page_url=binding["page_url"],
        eva_decision={"mode": "BIND", "confirm_replace": True},
    )
    pull_event_id = BusinessDocumentService._create_event(
        created["document_id"],
        2,
        "EvaDocumentPulled",
        "USER",
        AUTHOR,
        {
            "remote_version": "7",
            "remote_content_hash": "sha256:remote-content",
            "review_cycle": 3,
        },
        "legacy-pull",
    )
    BusinessDocumentEvaBinding.delete().execute()

    migrate_business_document_eva_bindings()
    migrate_business_document_eva_bindings()

    projection = BusinessDocumentEvaBinding.get_by_id(created["document_id"])
    assert projection.binding["page_url"] == binding["page_url"]
    assert projection.binding["remote_version"] == "7"
    assert projection.binding["last_pulled_content_hash"] == "sha256:remote-content"
    assert projection.binding["last_pull_event_id"] == pull_event_id
    assert projection.binding["last_pull_review_cycle"] == 3


def test_matching_eva_page_reports_the_document_that_already_owns_it(database, monkeypatch):
    from api.apps.business_documents.eva_changes import EvaDocumentChangeService

    binding = {
        "page_url": "https://eva.example.com/project/Document/BR-42",
        "status": "CONNECTED",
        "connector_id": "connector-1",
        "eva_origin": "https://eva-api.example.com",
        "project_id": "CmfProject:portal",
        "document_id": "CmfDocument:doc-1",
        "document_name": "Переводы одной кнопкой",
    }
    monkeypatch.setattr(EvaDocumentChangeService, "resolve_page_url", staticmethod(lambda *_args: binding))
    owner = _create(
        schema_version="2",
        eva_page_url=binding["page_url"],
        eva_decision={"mode": "BIND", "confirm_replace": True},
    )
    match = {
        "id": binding["document_id"],
        "name": "Другой документ",
        "code": "BR-42",
        "project_id": binding["project_id"],
        "web_url": binding["page_url"],
        "breadcrumbs": [],
        "hierarchy": "Проекты › Другой документ",
        "connector_id": binding["connector_id"],
        "connector_name": "EVA Wiki",
        "eva_origin": binding["eva_origin"],
    }
    monkeypatch.setattr(EvaDocumentChangeService, "find_title_matches", staticmethod(lambda *_args: [match]))

    with pytest.raises(BusinessDocumentError) as caught:
        _create(title="Другой документ", schema_version="2")

    assert caught.value.code == "EVA_BINDING_DECISION_REQUIRED"
    occupied = caught.value.details["matches"][0]
    assert occupied["binding_available"] is False
    assert occupied["linked_document"] == {"document_id": owner["document_id"], "title": "Переводы одной кнопкой"}


def _command(document, command_type, payload=None, *, key=None, command_id=None, expected=None):
    return {
        "schema_version": "1",
        "command_id": command_id or f"cmd-{command_type.lower()}-{document['state_version']}",
        "idempotency_key": key or f"idem-{command_type.lower()}-{document['state_version']}",
        "expected_state_version": expected or document["state_version"],
        "type": command_type,
        "payload": payload or {},
    }


def _complete(job_id, output, worker="worker-1"):
    job = BusinessDocumentJobQueue.claim(worker)
    assert job is not None and job.id == job_id
    return BusinessDocumentService.complete_job(TENANT, worker, job_id, output, job.lease_token)


def _question_batch(stage="INTAKE", semantic_tag="audience"):
    return {
        "schema_version": "1",
        "outcome": "NEEDS_INPUT",
        "questions": [
            {
                "semantic_tag": semantic_tag,
                "stage": stage,
                "target_section_id": "3.1",
                "text": "Кто будет пользоваться сервисом?",
                "options": [
                    {"option_id": "individuals", "label": "Физические лица"},
                    {"option_id": "companies", "label": "Юридические лица"},
                ],
                "allow_custom_answer": True,
            }
        ],
    }


def _draft(suffix="исходная версия"):
    template = published_template()
    return {
        "schema_version": "1",
        "document_type": "business_requirements",
        "template_version": template["template_version"],
        "sections": [
            {
                "id": section["id"],
                "title": section["title"],
                "blocks": required_section_blocks(section["id"], f"Раздел {section['id']}: {suffix}."),
            }
            for section in template["sections"]
        ],
    }


def _request_and_complete_draft(document, *, review_questions=None, proposals=None):
    if "REQUEST_DRAFT" not in document["allowed_commands"]:
        assessment = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
        document = _complete(
            assessment["job_id"],
            {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
        )
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_DRAFT"))
    created_event_id = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "DocumentCreated")).id
    normalized_proposals = [
        {
            "target_section_id": proposal.get("target_section_id", "5.5"),
            "text": proposal["text"],
            "rationale": proposal.get("rationale", "Уточняет проверяемость требований"),
            "source_event_ids": proposal.get("source_event_ids", [created_event_id]),
        }
        for proposal in (proposals or [])
    ]
    return _complete(
        requested["job_id"],
        {
            "draft": _draft(),
            "review_questions": review_questions or {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
            "proposals": normalized_proposals,
        },
    )


def _complete_review_assessment(document, *, comment_disposition="CONFIRMED_CHANGE"):
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_REVIEW_ASSESSMENT"))
    dispositions = [{"comment_event_id": comment["source_event_id"], "disposition": comment_disposition} for comment in document["protocol"]["comments"]]
    return _complete(
        requested["job_id"],
        {"schema_version": "1", "questions": [], "proposals": [], "comment_dispositions": dispositions},
    )


@pytest.mark.p0
def test_create_contract_rejects_unknown_fields(database):
    with pytest.raises(BusinessDocumentError) as caught:
        _create(attachment_ids=["not-yet-supported"])
    assert caught.value.code == "INVALID_CREATE_DOCUMENT"
    assert BusinessDocument.select().count() == 0


@pytest.mark.p0
def test_failed_optimistic_cas_rolls_back_child_write_but_records_rejection(database, monkeypatch):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    document = _complete(requested["job_id"], _question_batch())
    question = document["protocol"]["questions"][0]
    command = _command(
        document,
        "ANSWER_QUESTION",
        {"question_id": question["question_id"], "selected_option_id": "individuals", "custom_answer": None},
        key="cas-after-child-write",
    )

    def lose_cas(_document, _changes):
        raise BusinessDocumentError("STATE_VERSION_CONFLICT", "The document changed concurrently", 409)

    monkeypatch.setattr(BusinessDocumentService, "_optimistic_update", staticmethod(lose_cas))
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], command)

    assert caught.value.code == "STATE_VERSION_CONFLICT"
    assert BusinessDocumentAnswer.select().count() == 0
    ledger = BusinessDocumentCommand.get(BusinessDocumentCommand.idempotency_key == "cas-after-child-write")
    assert ledger.response["accepted"] is False
    assert ledger.response["error"]["code"] == "STATE_VERSION_CONFLICT"


@pytest.mark.p0
def test_job_insert_failure_rolls_back_claimed_document_version(database, monkeypatch):
    document = _create()
    event_count = BusinessDocumentEvent.select().count()

    def fail_insert(**kwargs):
        raise RuntimeError("job storage unavailable")

    monkeypatch.setattr(BusinessDocumentJob, "create", fail_insert)
    with pytest.raises(RuntimeError, match="job storage unavailable"):
        BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    persisted = BusinessDocument.get_by_id(document["document_id"])
    assert persisted.state_version == document["state_version"]
    assert persisted.operation_state == document["operation_state"]
    assert BusinessDocumentJob.select().count() == 0
    assert BusinessDocumentCommand.select().count() == 0
    assert BusinessDocumentEvent.select().count() == event_count


@pytest.mark.p0
def test_concurrent_idempotency_insert_replays_winning_ledger(database, monkeypatch):
    document = _create()
    command = _command(document, "REQUEST_INTAKE_ASSESSMENT", key="same-key-race")
    first = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], command)
    winner = BusinessDocumentCommand.get(BusinessDocumentCommand.idempotency_key == "same-key-race")
    original_get_or_none = BusinessDocumentCommand.get_or_none
    lookups = 0

    def racing_lookup(*query):
        nonlocal lookups
        lookups += 1
        return None if lookups == 1 else original_get_or_none(*query)

    def lose_insert(**_kwargs):
        raise IntegrityError("simulated concurrent unique-key winner")

    monkeypatch.setattr(BusinessDocumentCommand, "get_or_none", racing_lookup)
    monkeypatch.setattr(BusinessDocumentCommand, "create", lose_insert)
    replay = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], command)

    assert replay["idempotent_replay"] is True
    assert replay["job_id"] == first["job_id"]
    assert winner.request_hash == BusinessDocumentCommand.get_by_id(winner.id).request_hash


@pytest.mark.p0
def test_full_workflow_is_versioned_idempotent_and_append_only(database):
    document = _create()
    assert document["lifecycle_state"] == "INTAKE"
    assert document["state_version"] == 1
    assert document["current_revision"] is None

    assessment = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    document = _complete(assessment["job_id"], _question_batch())
    question = document["protocol"]["questions"][0]
    answer_command = _command(
        document,
        "ANSWER_QUESTION",
        {"question_id": question["question_id"], "selected_option_id": "individuals", "custom_answer": None},
    )
    document = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], answer_command)
    document = BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)

    document = _request_and_complete_draft(
        document,
        review_questions=_question_batch("REVIEW", "monitoring"),
        proposals=[
            {"id": "proposal-accepted", "target_section_id": "5.5", "text": "Добавить метрику ошибок"},
            {"id": "proposal-rejected", "target_section_id": "5.5", "text": "Удалить мониторинг"},
        ],
    )
    revision_one = deepcopy(document["current_revision"])
    review_question = document["protocol"]["questions"][0]

    proposal_ids = {proposal["text"]: proposal["proposal_id"] for proposal in document["protocol"]["proposals"]}
    for proposal_id, decision in (
        (proposal_ids["Добавить метрику ошибок"], "ACCEPTED"),
        (proposal_ids["Удалить мониторинг"], "REJECTED"),
    ):
        response = BusinessDocumentService.execute_command(
            TENANT,
            AUTHOR,
            document["document_id"],
            _command(document, "DECIDE_PROPOSAL", {"proposal_id": proposal_id, "decision": decision}),
        )
        document = BusinessDocumentService.get_document(TENANT, response["document_id"], AUTHOR)

    response = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ANSWER_QUESTION",
            {"question_id": review_question["question_id"], "selected_option_id": "individuals", "custom_answer": None},
        ),
    )
    document = BusinessDocumentService.get_document(TENANT, response["document_id"], AUTHOR)
    document = _complete_review_assessment(document)
    accepted_event = next(
        event
        for event in BusinessDocumentEvent.select().where((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "ProposalDecided"))
        if event.payload["decision"] == "ACCEPTED"
    )
    review_answer_event = next(
        event
        for event in BusinessDocumentEvent.select().where((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "QuestionAnswered"))
        if event.payload["question_id"] == review_question["question_id"]
    )
    apply_command = _command(document, "APPLY_CHANGES", {"base_revision_id": revision_one["revision_id"]}, key="apply-once")
    apply_response = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], apply_command)
    replay = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], apply_command)
    assert replay["idempotent_replay"] is True
    assert replay["job_id"] == apply_response["job_id"]
    assert BusinessDocumentJob.select().where(BusinessDocumentJob.job_type == "PLAN_CHANGES").count() == 1

    document = _complete(
        apply_response["job_id"],
        {
            "change_plan": {
                "schema_version": "1",
                "base_revision_id": revision_one["revision_id"],
                "source_state_version": apply_response["state_version"],
                "acknowledged_no_change_event_ids": [review_answer_event.id],
                "operations": [
                    {
                        "operation_id": "op-1",
                        "type": "REPLACE_SECTION_CONTENT",
                        "section_id": "5.5",
                        "expected_section_hash": section_hash(next(section for section in revision_one["document_ast"]["sections"] if section["id"] == "5.5")),
                        "source_event_ids": [accepted_event.id],
                        "content": {"blocks": [{"type": "paragraph", "text": "Контроль ошибок"}]},
                    }
                ],
            },
        },
    )

    assert document["lifecycle_state"] == "AGREED"
    assert document["operation_state"] == "IDLE"
    assert document["current_revision"]["revision_number"] == 2
    assert document["current_revision"]["content_hash"] != revision_one["content_hash"]
    before_sections = {section["id"]: section for section in revision_one["document_ast"]["sections"]}
    after_sections = {section["id"]: section for section in document["current_revision"]["document_ast"]["sections"]}
    assert after_sections["1"] == before_sections["1"]
    assert after_sections["5.5"]["blocks"] == [{"type": "paragraph", "text": "Контроль ошибок"}]
    revisions = BusinessDocumentService.list_revisions(TENANT, document["document_id"], AUTHOR)
    assert revisions[0] == revision_one
    assert len(revisions) == 2
    assert BusinessDocumentAnswer.select().count() == 2
    assert BusinessDocumentProposalDecision.select().count() == 2
    assert all(event.create_time for event in BusinessDocumentEvent.select())


@pytest.mark.p0
@pytest.mark.parametrize("feedback_type", ["ANSWER_QUESTION", "DECIDE_PROPOSAL", "ADD_COMMENT"])
def test_partial_review_feedback_can_be_assessed_once_without_changing_revision(database, feedback_type):
    questions = _question_batch("REVIEW")
    questions["questions"].extend(_question_batch("REVIEW", "other_audience")["questions"])
    document = _request_and_complete_draft(_create(), review_questions=questions, proposals=[{"text": "Уточнить метрику"}, {"text": "Добавить ограничение"}])
    revision = deepcopy(document["current_revision"])
    assert "REQUEST_REVIEW_ASSESSMENT" not in document["allowed_commands"]
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_REVIEW_ASSESSMENT"))
    assert caught.value.code == "OPEN_REVIEW_QUESTIONS"

    payload = {
        "ANSWER_QUESTION": {"question_id": document["protocol"]["questions"][0]["question_id"], "selected_option_id": "individuals", "custom_answer": None},
        "DECIDE_PROPOSAL": {"proposal_id": document["protocol"]["proposals"][0]["proposal_id"], "decision": "ACCEPTED"},
        "ADD_COMMENT": {"revision_id": revision["revision_id"], "section_id": None, "text": "Уточнить сроки", "anchor": None},
    }[feedback_type]
    BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, feedback_type, payload))
    document = BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)
    assert "REQUEST_REVIEW_ASSESSMENT" in document["allowed_commands"]
    assert "ANSWER_QUESTION" in document["allowed_commands"]
    assert "APPLY_CHANGES" not in document["allowed_commands"]

    document = _complete_review_assessment(document)
    assert document["current_revision"] == revision
    assert any(question["status"] == "OPEN" for question in document["protocol"]["questions"])
    assert len(document["protocol"]["questions"]) == 2
    assert len(document["protocol"]["proposals"]) == 2
    assert "REQUEST_REVIEW_ASSESSMENT" not in document["allowed_commands"]
    assert "APPLY_CHANGES" not in document["allowed_commands"]
    assessment = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "ReviewAssessed"))
    assert assessment.payload["outcome"] == "NEEDS_INPUT"
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_REVIEW_ASSESSMENT"))
    assert caught.value.code == "OPEN_REVIEW_QUESTIONS"

    BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "ADD_COMMENT", {"revision_id": revision["revision_id"], "section_id": None, "text": "Ещё одно уточнение", "anchor": None}),
    )
    document = BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)
    assert "REQUEST_REVIEW_ASSESSMENT" in document["allowed_commands"]


@pytest.mark.p0
def test_apply_is_rejected_while_review_question_is_open_without_mutation_or_job(database):
    document = _request_and_complete_draft(_create(), review_questions=_question_batch("REVIEW"))
    revision = deepcopy(document["current_revision"])
    command = _command(document, "APPLY_CHANGES", {"base_revision_id": revision["revision_id"]}, key="blocked-apply")

    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], command)

    assert caught.value.code == "OPEN_REVIEW_QUESTIONS"
    after = BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)
    assert after["state_version"] == document["state_version"]
    assert after["current_revision"] == revision
    assert BusinessDocumentJob.select().where(BusinessDocumentJob.job_type == "PLAN_CHANGES").count() == 0
    assert BusinessDocumentCommand.get(BusinessDocumentCommand.idempotency_key == "blocked-apply").response["accepted"] is False
    with pytest.raises(BusinessDocumentError) as replayed:
        BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], command)
    assert replayed.value.code == "OPEN_REVIEW_QUESTIONS"


@pytest.mark.p0
def test_state_version_and_idempotency_payload_conflicts_are_rejected(database):
    document = _create()
    stale = _command(document, "REQUEST_DRAFT", expected=document["state_version"] + 1)
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], stale)
    assert caught.value.code == "STATE_VERSION_CONFLICT"

    valid = _command(document, "REQUEST_INTAKE_ASSESSMENT", key="same-key")
    BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], valid)
    conflicting = {**valid, "command_id": "different-command"}
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], conflicting)
    assert caught.value.code == "IDEMPOTENCY_CONFLICT"


@pytest.mark.p0
def test_authors_can_open_foreign_documents_while_unknown_ids_stay_non_enumerable(database):
    document = _create()
    projection = BusinessDocumentService.get_document("another-tenant", document["document_id"], "another-author")
    assert projection["document_id"] == document["document_id"]
    assert projection["permissions"] == {"read": True, "edit": False, "delete": False, "assign": False}
    assert projection["allowed_commands"] == []

    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.get_document("another-tenant", "missing-document", AUTHOR)
    assert caught.value.status == 404
    assert caught.value.code == "DOCUMENT_NOT_FOUND"


@pytest.mark.p0
def test_shared_access_list_and_server_assigned_chat(database):
    User.create(id=AUTHOR, nickname="Первый автор", email="author-1@example.com")
    first = _create(title="Первый")
    second = _create(title="Второй")
    listing = BusinessDocumentService.list_documents(TENANT, AUTHOR, page=1, page_size=1)
    assert listing["total"] == 2
    assert len(listing["items"]) == 1
    assert listing["items"][0]["document_id"] in {first["document_id"], second["document_id"]}
    assert listing["items"][0]["owner_name"] == "Первый автор"
    assert first["chat_id"].startswith("business-document:")

    shared = BusinessDocumentService.get_document("another-tenant", first["document_id"], "another-user")
    assert shared["document_id"] == first["document_id"]
    assert shared["owner_name"] == "Первый автор"
    assert shared["permissions"]["edit"] is False
    with pytest.raises(BusinessDocumentError) as caught:
        _create(chat_id="client-controlled")
    assert caught.value.code == "CHAT_ID_NOT_ALLOWED"


@pytest.mark.p0
def test_business_document_role_matrix_controls_create_edit_delete_and_assignment(database):
    owned = _create(title="Документ автора")
    foreign = BusinessDocumentService.create_document(
        "tenant-2",
        "author-2",
        {
            "schema_version": "1",
            "document_type": "business_requirements",
            "title": "Чужой документ",
            "idea": "Проверить административный доступ",
        },
    )

    author_listing = BusinessDocumentService.list_documents(TENANT, AUTHOR)
    assert {item["document_id"] for item in author_listing["items"]} == {owned["document_id"], foreign["document_id"]}
    assert all(item["access_role"] == "AUTHOR_CREATOR" for item in author_listing["items"])
    permissions_by_id = {item["document_id"]: item["permissions"] for item in author_listing["items"]}
    assert permissions_by_id[owned["document_id"]] == {"read": True, "edit": True, "delete": False, "assign": False}
    assert permissions_by_id[foreign["document_id"]] == {"read": True, "edit": False, "delete": False, "assign": False}
    assert author_listing["capabilities"] == {
        "read": True,
        "create": True,
        "edit_own": True,
        "edit_all": False,
        "delete": False,
        "assign": False,
    }
    mine_listing = BusinessDocumentService.list_documents(TENANT, AUTHOR, scope="mine")
    assert [item["document_id"] for item in mine_listing["items"]] == [owned["document_id"]]
    assert mine_listing["scope"] == "mine"

    foreign_projection = BusinessDocumentService.get_document(TENANT, foreign["document_id"], AUTHOR)
    assert foreign_projection["permissions"]["edit"] is False
    assert foreign_projection["allowed_commands"] == []

    with pytest.raises(BusinessDocumentError) as denied:
        BusinessDocumentService.execute_command(
            TENANT,
            AUTHOR,
            foreign["document_id"],
            _command(foreign_projection, "REQUEST_INTAKE_ASSESSMENT", key="author-command"),
        )
    assert denied.value.code == "DOCUMENT_PERMISSION_DENIED"

    command = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        foreign["document_id"],
        _command(foreign_projection, "REQUEST_INTAKE_ASSESSMENT", key="moderator-command"),
        access_role="MODERATOR_CREATOR",
    )
    event = BusinessDocumentEvent.get_by_id(command["event_id"])
    job = BusinessDocumentJob.get_by_id(command["job_id"])
    ledger = BusinessDocumentCommand.get(BusinessDocumentCommand.document_id == foreign["document_id"])
    assert event.actor_id == AUTHOR
    assert job.tenant_id == "tenant-2"
    assert job.payload["requested_by_actor_id"] == AUTHOR
    assert ledger.tenant_id == "tenant-2"

    with pytest.raises(BusinessDocumentError) as denied_create:
        BusinessDocumentService.create_document(
            TENANT,
            AUTHOR,
            {"schema_version": "1", "document_type": "business_requirements", "title": "Нельзя", "idea": "Нет права"},
            access_role="AUTHOR_EDITOR",
        )
    assert denied_create.value.code == "DOCUMENT_PERMISSION_DENIED"

    admin_listing = BusinessDocumentService.list_documents("admin-tenant", "admin-user", is_admin=True)
    assert {item["document_id"] for item in admin_listing["items"]} == {owned["document_id"], foreign["document_id"]}
    assert all(item["access_role"] == "ADMIN" for item in admin_listing["items"])
    assert all(item["permissions"] == {"read": True, "edit": True, "delete": True, "assign": True} for item in admin_listing["items"])

    admin_projection = BusinessDocumentService.get_document("admin-tenant", foreign["document_id"], "admin-user", is_admin=True)
    assert admin_projection["access_role"] == "ADMIN"
    assert admin_projection["permissions"] == {"read": True, "edit": True, "delete": True, "assign": True}
    assert admin_projection["allowed_commands"] == []

    admin_created = BusinessDocumentService.create_document(
        "admin-user",
        "admin-user",
        {
            "schema_version": "1",
            "document_type": "business_requirements",
            "title": "Документ администратора",
            "idea": "Проверить роль в mutation-ответе",
        },
        is_admin=True,
    )
    assert admin_created["access_role"] == "ADMIN"
    assert admin_created["permissions"] == {"read": True, "edit": True, "delete": True, "assign": True}


@pytest.mark.p0
def test_extended_moderator_assigns_document_and_admin_manages_document_roles(database):
    User.create(id=AUTHOR, nickname="Первый автор", email="author-1@example.com")
    User.create(id="author-2", nickname="Второй автор", email="author-2@example.com")
    document = _create()

    users = BusinessDocumentService.list_access_users("moderator-1", access_role="EXTENDED_MODERATOR")
    assert {(item["nickname"], item["email"]) for item in users["items"]} == {
        ("Первый автор", "author-1@example.com"),
        ("Второй автор", "author-2@example.com"),
    }
    assert {(item["user_id"], item["role"]) for item in users["items"]} == {
        (AUTHOR, "AUTHOR_CREATOR"),
        ("author-2", "AUTHOR_CREATOR"),
    }

    with pytest.raises(BusinessDocumentError) as denied:
        BusinessDocumentService.assign_document(
            "moderator-1",
            document["document_id"],
            {"owner_id": "author-2", "expected_state_version": document["state_version"]},
            access_role="MODERATOR_CREATOR",
        )
    assert denied.value.code == "DOCUMENT_PERMISSION_DENIED"

    assigned = BusinessDocumentService.assign_document(
        "moderator-1",
        document["document_id"],
        {"owner_id": "author-2", "expected_state_version": document["state_version"]},
        access_role="EXTENDED_MODERATOR",
    )
    assert assigned["owner_id"] == "author-2"
    assert assigned["owner_name"] == "Второй автор"
    assert assigned["state_version"] == document["state_version"] + 1
    assignment_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "DocumentAssigned"))
    assert assignment_event.actor_id == "moderator-1"
    assert assignment_event.payload == {"previous_owner_id": AUTHOR, "owner_id": "author-2"}

    assert BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)["permissions"]["edit"] is False
    assert BusinessDocumentService.get_document(TENANT, document["document_id"], "author-2")["permissions"]["edit"] is True

    changed_role = BusinessDocumentService.update_user_access_role(
        "admin-user",
        "author-2",
        {"role": "AUTHOR_EDITOR"},
        is_admin=True,
    )
    assert changed_role["role"] == "AUTHOR_EDITOR"
    assert User.get_by_id("author-2").business_document_role == "AUTHOR_EDITOR"


@pytest.mark.p0
def test_revision_history_records_the_collaborator_who_requested_an_ai_draft(database):
    document = _create()
    assessment = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    document = _complete(
        assessment["job_id"],
        {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
    )
    requested = BusinessDocumentService.execute_command(
        "collaborator-tenant",
        "author-2",
        document["document_id"],
        _command(document, "REQUEST_DRAFT", key="collaborator-draft"),
        access_role="MODERATOR_CREATOR",
    )
    document = _complete(
        requested["job_id"],
        {
            "draft": _draft(),
            "review_questions": {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
            "proposals": [],
        },
    )

    initial_draft = next(item for item in document["current_revision"]["change_basis"] if item["type"] == "INITIAL_DRAFT")
    assert document["current_revision"]["author_id"] == "author-2"
    assert document["current_revision"]["author_name"] == "author-2"
    assert initial_draft["initiated_by_actor_id"] == "author-2"
    assert initial_draft["actor_type"] == "AI"
    assert initial_draft["actor_id"] == "worker-1"


@pytest.mark.p0
def test_extended_moderator_can_delete_document_and_all_owned_rows_are_removed(database):
    document = _create()

    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.delete_document(AUTHOR, document["document_id"])
    assert caught.value.status == 403
    assert caught.value.code == "DOCUMENT_PERMISSION_DENIED"

    foreign = BusinessDocumentService.create_document(
        "tenant-2",
        "author-2",
        {
            "schema_version": "1",
            "document_type": "business_requirements",
            "title": "Чужой документ",
            "idea": "Проверить запрет удаления",
        },
    )
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.delete_document(AUTHOR, foreign["document_id"])
    assert caught.value.code == "DOCUMENT_PERMISSION_DENIED"

    result = BusinessDocumentService.delete_document(
        "extended-moderator",
        document["document_id"],
        access_role="EXTENDED_MODERATOR",
    )
    assert result == {
        "document_id": document["document_id"],
        "deleted": True,
        "deleted_artifacts": 0,
        "storage_cleanup_failures": 0,
    }
    assert BusinessDocument.select().where(BusinessDocument.id == document["document_id"]).count() == 0
    assert BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == document["document_id"]).count() == 0


@pytest.mark.p0
def test_admin_cannot_delete_document_while_background_operation_is_active(database):
    document = _create()
    BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))

    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.delete_document("admin-user", document["document_id"], is_admin=True)
    assert caught.value.status == 409
    assert caught.value.code == "OPERATION_IN_PROGRESS"
    assert BusinessDocument.select().where(BusinessDocument.id == document["document_id"]).exists()


@pytest.mark.p0
def test_draft_requires_complete_assessment_after_last_intake_answer(database):
    document = _create()
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_DRAFT"))
    assert caught.value.code == "INTAKE_ASSESSMENT_REQUIRED"

    assessment = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    document = _complete(assessment["job_id"], _question_batch(), worker="worker")
    question = document["protocol"]["questions"][0]
    response = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ANSWER_QUESTION",
            {"question_id": question["question_id"], "selected_option_id": "individuals", "custom_answer": None},
        ),
    )
    document = BusinessDocumentService.get_document(TENANT, response["document_id"], AUTHOR)
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_DRAFT"))
    assert caught.value.code == "INTAKE_ASSESSMENT_REQUIRED"
    assert "REQUEST_INTAKE_ASSESSMENT" in document["allowed_commands"]


@pytest.mark.p0
def test_question_schema_boundary_rejects_five_options_and_rolls_back(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    invalid = _question_batch()
    invalid["questions"][0]["options"] += [
        {"option_id": "third", "label": "Третий"},
        {"option_id": "fourth", "label": "Четвертый"},
        {"option_id": "fifth", "label": "Пятый"},
    ]
    with pytest.raises(BusinessDocumentError) as caught:
        _complete(requested["job_id"], invalid)
    assert caught.value.code == "INVALID_QUESTION_BATCH"
    job = BusinessDocumentJob.get_by_id(requested["job_id"])
    assert job.status == "RUNNING"
    assert BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)["operation_state"] == "ANALYZING"


@pytest.mark.p0
def test_question_answers_and_proposal_decisions_are_immutable(database):
    document = _create()
    assessment = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    document = _complete(assessment["job_id"], _question_batch(), worker="worker")
    question_id = document["protocol"]["questions"][0]["question_id"]
    first = _command(
        document,
        "ANSWER_QUESTION",
        {"question_id": question_id, "selected_option_id": "individuals", "custom_answer": None},
    )
    BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], first)
    document = BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(
            TENANT,
            AUTHOR,
            document["document_id"],
            _command(document, "ANSWER_QUESTION", {"question_id": question_id, "selected_option_id": "companies", "custom_answer": None}),
        )
    assert caught.value.code == "QUESTION_ALREADY_CLOSED"
    assert BusinessDocumentAnswer.select().count() == 1

    document = _request_and_complete_draft(document, proposals=[{"text": "Добавить метрику"}])
    proposal_id = document["protocol"]["proposals"][0]["proposal_id"]
    first = _command(document, "DECIDE_PROPOSAL", {"proposal_id": proposal_id, "decision": "ACCEPTED"})
    BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], first)
    document = BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(
            TENANT,
            AUTHOR,
            document["document_id"],
            _command(document, "DECIDE_PROPOSAL", {"proposal_id": proposal_id, "decision": "REJECTED"}),
        )
    assert caught.value.code == "PROPOSAL_ALREADY_DECIDED"
    assert BusinessDocumentProposalDecision.select().count() == 1


@pytest.mark.p0
def test_rejected_proposal_cannot_authorize_change(database):
    document = _request_and_complete_draft(_create(), proposals=[{"text": "Удалить мониторинг"}])
    proposal_id = document["protocol"]["proposals"][0]["proposal_id"]
    decision = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "DECIDE_PROPOSAL", {"proposal_id": proposal_id, "decision": "REJECTED"}),
    )
    document = BusinessDocumentService.get_document(TENANT, decision["document_id"], AUTHOR)
    document = _complete_review_assessment(document)
    rejected_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "ProposalDecided"))
    request = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "APPLY_CHANGES", {"base_revision_id": document["current_revision"]["revision_id"]}),
    )
    with pytest.raises(BusinessDocumentError) as caught:
        _complete(
            request["job_id"],
            {
                "change_plan": {
                    "schema_version": "1",
                    "base_revision_id": document["current_revision"]["revision_id"],
                    "source_state_version": request["state_version"],
                    "acknowledged_no_change_event_ids": [],
                    "operations": [
                        {
                            "operation_id": "op-1",
                            "type": "REPLACE_SECTION_CONTENT",
                            "section_id": "5.5",
                            "expected_section_hash": section_hash(next(section for section in document["current_revision"]["document_ast"]["sections"] if section["id"] == "5.5")),
                            "source_event_ids": [rejected_event.id],
                            "content": {"blocks": []},
                        }
                    ],
                },
            },
        )
    assert caught.value.code == "REJECTED_PROPOSAL_SOURCE"
    assert BusinessDocumentRevision.select().count() == 1


@pytest.mark.p1
def test_stale_worker_result_cannot_create_revision(database):
    document = _create()
    assessment = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    document = _complete(assessment["job_id"], {"schema_version": "1", "outcome": "COMPLETE", "questions": []}, worker="worker")
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_DRAFT"))
    BusinessDocument.update(state_version=requested["state_version"] + 1).where(BusinessDocument.id == document["document_id"]).execute()
    with pytest.raises(BusinessDocumentError) as caught:
        _complete(
            requested["job_id"],
            {"draft": _draft(), "review_questions": {"schema_version": "1", "outcome": "COMPLETE", "questions": []}, "proposals": []},
        )
    assert caught.value.code == "STALE_AI_RESULT"
    assert BusinessDocumentRevision.select().count() == 0


@pytest.mark.p0
def test_required_sections_cannot_be_empty_and_child_headings_are_nested(database):
    draft = _draft()
    section = next(item for item in draft["sections"] if item["id"] == "5.5")
    section["blocks"] = []
    with pytest.raises(BusinessDocumentError) as caught:
        validate_document_ast(draft)
    assert caught.value.code == "REQUIRED_SECTION_EMPTY"

    markdown = render_document_ast(_draft())
    assert "## 3. Целевая аудитория" in markdown
    assert "### 3.1. Пользователи и их категории" in markdown

    activity = _draft()
    scenario = next(item for item in activity["sections"] if item["id"] == "4.3")
    scenario["blocks"] = [
        {"type": "paragraph", "text": "Основной и негативный клиентские сценарии."},
        {"type": "plantuml", "source": VALID_ACTIVITY_SCENARIO},
    ]
    assert "```plantuml" in render_document_ast(validate_document_ast(activity))

    wrong_diagram = _draft()
    scenario = next(item for item in wrong_diagram["sections"] if item["id"] == "4.3")
    scenario["blocks"] = [{"type": "paragraph", "text": "Только текст"}]
    with pytest.raises(BusinessDocumentError) as caught:
        validate_document_ast(wrong_diagram)
    assert caught.value.code == "ACTIVITY_SCENARIO_REQUIRED"

    missing_concept = _draft()
    conceptual = next(item for item in missing_concept["sections"] if item["id"] == "4.1")
    conceptual["blocks"] = [{"type": "paragraph", "text": "Только текст"}]
    with pytest.raises(BusinessDocumentError) as caught:
        validate_document_ast(missing_concept)
    assert caught.value.code == "CONCEPTUAL_DIAGRAM_REQUIRED"

    incomplete_activity = _draft()
    scenario = next(item for item in incomplete_activity["sections"] if item["id"] == "4.3")
    scenario["blocks"] = [
        {"type": "paragraph", "text": "Сопровождающий текст"},
        {
            "type": "plantuml",
            "source": "@startuml\nstart\n:Основной путь;\nstop\n@enduml",
        },
    ]
    with pytest.raises(BusinessDocumentError) as caught:
        validate_document_ast(incomplete_activity)
    assert caught.value.code == "INCOMPLETE_ACTIVITY_SCENARIO"


@pytest.mark.p0
def test_change_plan_cannot_rewrite_unlisted_sections_or_use_stale_section_hash(database):
    document = _request_and_complete_draft(_create())
    original = deepcopy(document["current_revision"])
    comment = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ADD_COMMENT",
            {"revision_id": original["revision_id"], "section_id": None, "text": "Добавить метрику ошибок", "anchor": None},
        ),
    )
    document = BusinessDocumentService.get_document(TENANT, comment["document_id"], AUTHOR)
    comment_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "AuthorCommentAdded"))
    document = _complete_review_assessment(document)
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "APPLY_CHANGES", {"base_revision_id": original["revision_id"]}),
    )
    with pytest.raises(BusinessDocumentError) as caught:
        _complete(
            requested["job_id"],
            {
                "change_plan": {
                    "schema_version": "1",
                    "base_revision_id": original["revision_id"],
                    "source_state_version": requested["state_version"],
                    "acknowledged_no_change_event_ids": [],
                    "operations": [
                        {
                            "operation_id": "op-1",
                            "type": "REPLACE_SECTION_CONTENT",
                            "section_id": "5.5",
                            "expected_section_hash": "sha256:" + "0" * 64,
                            "source_event_ids": [comment_event.id],
                            "content": {"blocks": [{"type": "paragraph", "text": "Новая метрика"}]},
                        }
                    ],
                }
            },
        )
    assert caught.value.code == "SECTION_HASH_CONFLICT"
    assert BusinessDocumentService.list_revisions(TENANT, document["document_id"], AUTHOR) == [original]


@pytest.mark.p0
def test_no_op_review_agrees_existing_revision_without_duplicate(database):
    document = _request_and_complete_draft(_create())
    original_revision = deepcopy(document["current_revision"])
    document = _complete_review_assessment(document)
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "APPLY_CHANGES", {"base_revision_id": original_revision["revision_id"]}),
    )
    document = _complete(
        requested["job_id"],
        {
            "change_plan": {
                "schema_version": "1",
                "base_revision_id": original_revision["revision_id"],
                "source_state_version": requested["state_version"],
                "acknowledged_no_change_event_ids": [],
                "operations": [],
            }
        },
    )
    assert document["lifecycle_state"] == "AGREED"
    assert document["current_revision"] == original_revision
    assert BusinessDocumentRevision.select().count() == 1
    assert BusinessDocumentEvent.select().where(BusinessDocumentEvent.event_type == "ReviewAgreedWithoutChanges").count() == 1


@pytest.mark.p0
def test_no_op_requires_explicit_disposition_for_current_review_inputs(database):
    document = _request_and_complete_draft(_create())
    original_revision = deepcopy(document["current_revision"])
    response = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ADD_COMMENT",
            {"revision_id": original_revision["revision_id"], "section_id": None, "text": "Проверено, менять не нужно", "anchor": None},
        ),
    )
    document = BusinessDocumentService.get_document(TENANT, response["document_id"], AUTHOR)
    comment_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "AuthorCommentAdded"))
    document = _complete_review_assessment(document, comment_disposition="NO_CHANGE")
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "APPLY_CHANGES", {"base_revision_id": original_revision["revision_id"]}),
    )
    job = BusinessDocumentJobQueue.claim("no-op-worker")
    assert job is not None and job.id == requested["job_id"]
    plan = {
        "change_plan": {
            "schema_version": "1",
            "base_revision_id": original_revision["revision_id"],
            "source_state_version": requested["state_version"],
            "acknowledged_no_change_event_ids": [],
            "operations": [],
        }
    }
    base_section = next(section for section in original_revision["document_ast"]["sections"] if section["id"] == "5.5")
    plan["change_plan"]["operations"] = [
        {
            "operation_id": "invalid-no-change-source",
            "type": "REPLACE_SECTION_CONTENT",
            "section_id": "5.5",
            "expected_section_hash": section_hash(base_section),
            "source_event_ids": [comment_event.id],
            "content": {"blocks": [{"type": "paragraph", "text": "Не должно примениться"}]},
        }
    ]
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.complete_job(TENANT, "no-op-worker", job.id, plan, job.lease_token)
    assert caught.value.code == "COMMENT_CHANGE_NOT_CONFIRMED"

    plan["change_plan"]["operations"] = []
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.complete_job(TENANT, "no-op-worker", job.id, plan, job.lease_token)
    assert caught.value.code == "CHANGE_INPUT_OMITTED"

    plan["change_plan"]["acknowledged_no_change_event_ids"] = [comment_event.id]
    agreed = BusinessDocumentService.complete_job(TENANT, "no-op-worker", job.id, plan, job.lease_token)
    assert agreed["lifecycle_state"] == "AGREED"
    assert agreed["current_revision"] == original_revision
    assert BusinessDocumentRevision.select().count() == 1


@pytest.mark.p0
def test_accepted_proposal_cannot_be_silently_omitted(database):
    document = _request_and_complete_draft(_create(), proposals=[{"text": "Добавить метрику"}])
    proposal_id = document["protocol"]["proposals"][0]["proposal_id"]
    response = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "DECIDE_PROPOSAL", {"proposal_id": proposal_id, "decision": "ACCEPTED"}),
    )
    document = BusinessDocumentService.get_document(TENANT, response["document_id"], AUTHOR)
    comment = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ADD_COMMENT",
            {"revision_id": document["current_revision"]["revision_id"], "section_id": None, "text": "Уточнить описание", "anchor": None},
        ),
    )
    document = BusinessDocumentService.get_document(TENANT, comment["document_id"], AUTHOR)
    comment_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "AuthorCommentAdded"))
    document = _complete_review_assessment(document)
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "APPLY_CHANGES", {"base_revision_id": document["current_revision"]["revision_id"]}),
    )
    base_section = next(section for section in document["current_revision"]["document_ast"]["sections"] if section["id"] == "5.5")
    with pytest.raises(BusinessDocumentError) as caught:
        _complete(
            requested["job_id"],
            {
                "change_plan": {
                    "schema_version": "1",
                    "base_revision_id": document["current_revision"]["revision_id"],
                    "source_state_version": requested["state_version"],
                    "acknowledged_no_change_event_ids": [],
                    "operations": [
                        {
                            "operation_id": "op-comment-only",
                            "type": "REPLACE_SECTION_CONTENT",
                            "section_id": "5.5",
                            "expected_section_hash": section_hash(base_section),
                            "source_event_ids": [comment_event.id],
                            "content": {"blocks": [{"type": "paragraph", "text": "Обновлено"}]},
                        }
                    ],
                }
            },
        )
    assert caught.value.code == "ACCEPTED_PROPOSAL_OMITTED"
    assert BusinessDocumentRevision.select().count() == 1


@pytest.mark.p0
def test_accepted_proposal_cannot_be_acknowledged_as_no_change(database):
    document = _request_and_complete_draft(_create(), proposals=[{"text": "Добавить метрику"}])
    proposal_id = document["protocol"]["proposals"][0]["proposal_id"]
    response = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "DECIDE_PROPOSAL", {"proposal_id": proposal_id, "decision": "ACCEPTED"}),
    )
    document = BusinessDocumentService.get_document(TENANT, response["document_id"], AUTHOR)
    document = _complete_review_assessment(document)
    accepted_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "ProposalDecided"))
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "APPLY_CHANGES", {"base_revision_id": document["current_revision"]["revision_id"]}),
    )

    with pytest.raises(BusinessDocumentError) as caught:
        _complete(
            requested["job_id"],
            {
                "change_plan": {
                    "schema_version": "1",
                    "base_revision_id": document["current_revision"]["revision_id"],
                    "source_state_version": requested["state_version"],
                    "acknowledged_no_change_event_ids": [accepted_event.id],
                    "operations": [],
                }
            },
        )

    assert caught.value.code == "ACCEPTED_PROPOSAL_ACKNOWLEDGED_NO_CHANGE"
    assert BusinessDocumentRevision.select().count() == 1


@pytest.mark.p0
def test_change_source_must_match_active_cycle_and_target_section(database):
    document = _request_and_complete_draft(_create(), review_questions=_question_batch("REVIEW", "audience"))
    question = document["protocol"]["questions"][0]
    response = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ANSWER_QUESTION",
            {"question_id": question["question_id"], "selected_option_id": "individuals", "custom_answer": None},
        ),
    )
    document = BusinessDocumentService.get_document(TENANT, response["document_id"], AUTHOR)
    answer_event = next(
        event
        for event in BusinessDocumentEvent.select().where((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "QuestionAnswered"))
        if event.payload["question_id"] == question["question_id"]
    )
    document = _complete_review_assessment(document)
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "APPLY_CHANGES", {"base_revision_id": document["current_revision"]["revision_id"]}),
    )
    wrong_section = next(section for section in document["current_revision"]["document_ast"]["sections"] if section["id"] == "5.5")
    with pytest.raises(BusinessDocumentError) as caught:
        _complete(
            requested["job_id"],
            {
                "change_plan": {
                    "schema_version": "1",
                    "base_revision_id": document["current_revision"]["revision_id"],
                    "source_state_version": requested["state_version"],
                    "acknowledged_no_change_event_ids": [],
                    "operations": [
                        {
                            "operation_id": "op-wrong-section",
                            "type": "REPLACE_SECTION_CONTENT",
                            "section_id": "5.5",
                            "expected_section_hash": section_hash(wrong_section),
                            "source_event_ids": [answer_event.id],
                            "content": {"blocks": [{"type": "paragraph", "text": "Не должно примениться"}]},
                        }
                    ],
                }
            },
        )
    assert caught.value.code == "CHANGE_SOURCE_SECTION_CONFLICT"


@pytest.mark.p0
def test_confirmed_anchored_comment_may_request_a_cross_section_change(database):
    document = _request_and_complete_draft(_create())
    revision = document["current_revision"]
    selected_text = revision["section_texts"]["3.3"]
    response = BusinessDocumentService.execute_command(
        "collaborator-tenant",
        "author-2",
        document["document_id"],
        _command(
            document,
            "ADD_COMMENT",
            {
                "revision_id": revision["revision_id"],
                "section_id": "3.3",
                "text": "Добавить чатбот и описать через него подачу заявки",
                "anchor": {
                    "revision_id": revision["revision_id"],
                    "section_id": "3.3",
                    "selected_text": selected_text,
                    "prefix": "",
                    "suffix": "",
                    "start_offset": 0,
                    "end_offset": len(selected_text),
                },
            },
        ),
        access_role="MODERATOR_CREATOR",
    )
    document = BusinessDocumentService.get_document(TENANT, response["document_id"], AUTHOR)
    comment_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "AuthorCommentAdded"))
    document = _complete_review_assessment(document)
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "APPLY_CHANGES", {"base_revision_id": revision["revision_id"]}),
    )
    target = next(section for section in revision["document_ast"]["sections"] if section["id"] == "4.3")
    changed = _complete(
        requested["job_id"],
        {
            "change_plan": {
                "schema_version": "1",
                "base_revision_id": revision["revision_id"],
                "source_state_version": requested["state_version"],
                "acknowledged_no_change_event_ids": [],
                "operations": [
                    {
                        "operation_id": "op-cross-section-comment",
                        "type": "REPLACE_SECTION_CONTENT",
                        "section_id": "4.3",
                        "expected_section_hash": section_hash(target),
                        "source_event_ids": [comment_event.id],
                        "content": {"blocks": required_section_blocks("4.3", "Подача заявки через чатбот.")},
                    }
                ],
            }
        },
    )

    assert changed["lifecycle_state"] == "AGREED"
    assert changed["current_revision"]["revision_number"] == 2
    assert "Подача заявки через чатбот" in changed["current_revision"]["section_texts"]["4.3"]
    assert changed["current_revision"]["change_basis"] == [
        {
            "event_id": comment_event.id,
            "actor_id": "author-2",
            "actor_type": "USER",
            "created_at": comment_event.create_time,
            "type": "COMMENT",
            "title": "Комментарий автора",
            "summary": "Добавить чатбот и описать через него подачу заявки",
            "details": selected_text,
            "section_id": "3.3",
        }
    ]


@pytest.mark.p0
def test_confirmed_document_comment_may_authorize_changes_in_multiple_sections(database):
    document = _request_and_complete_draft(_create())
    revision = document["current_revision"]
    response = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ADD_COMMENT",
            {
                "revision_id": revision["revision_id"],
                "section_id": None,
                "text": "Унифицировать терминологию во всём документе",
                "anchor": None,
            },
        ),
    )
    document = BusinessDocumentService.get_document(TENANT, response["document_id"], AUTHOR)
    comment_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "AuthorCommentAdded"))
    document = _complete_review_assessment(document)
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "APPLY_CHANGES", {"base_revision_id": revision["revision_id"]}),
    )
    targets = {section["id"]: section for section in revision["document_ast"]["sections"] if section["id"] in {"3.3", "5.5"}}
    changed = _complete(
        requested["job_id"],
        {
            "change_plan": {
                "schema_version": "1",
                "base_revision_id": revision["revision_id"],
                "source_state_version": requested["state_version"],
                "acknowledged_no_change_event_ids": [],
                "operations": [
                    {
                        "operation_id": f"op-document-comment-{section_id}",
                        "type": "REPLACE_SECTION_CONTENT",
                        "section_id": section_id,
                        "expected_section_hash": section_hash(targets[section_id]),
                        "source_event_ids": [comment_event.id],
                        "content": {"blocks": [{"type": "paragraph", "text": text}]},
                    }
                    for section_id, text in (
                        ("3.3", "Единый термин в описании потребности."),
                        ("5.5", "Единый термин в критериях приёмки."),
                    )
                ],
            }
        },
    )

    assert changed["current_revision"]["revision_number"] == 2
    assert "Единый термин" in changed["current_revision"]["section_texts"]["3.3"]
    assert "Единый термин" in changed["current_revision"]["section_texts"]["5.5"]
    assert changed["current_revision"]["change_basis"][0]["section_id"] is None


@pytest.mark.p0
def test_link_only_eva_binding_can_be_reconnected_without_rewriting_creation_event(database, monkeypatch):
    from api.apps.business_documents.eva_changes import EvaDocumentChangeService

    link_only = {
        "page_url": "http://host.docker.internal:8084//project/Document/DOC-001883",
        "status": "LINK_ONLY",
        "capabilities": ["OPEN"],
        "connector_id": None,
        "project_id": None,
        "document_id": None,
        "document_code": "DOC-001883",
        "document_name": None,
    }
    connected = {
        **link_only,
        "page_url": "https://eva.example.com/project/Document/DOC-001883",
        "status": "CONNECTED",
        "capabilities": ["OPEN", "PULL_FROM_EVA", "CREATE_EVA_CHANGE"],
        "connector_id": "connector-business-documents",
        "project_id": "CmfProject:business-documents",
        "document_id": "CmfDocument:doc-1",
        "document_name": "Документ1",
    }
    monkeypatch.setattr(EvaDocumentChangeService, "resolve_page_url", staticmethod(lambda _actor_id, _page_url: link_only))
    document = _create(
        schema_version="2",
        eva_page_url=link_only["page_url"],
        eva_decision={"mode": "BIND", "confirm_replace": True},
    )
    created_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "DocumentCreated"))
    original_created_payload = deepcopy(created_event.payload)
    monkeypatch.setattr(EvaDocumentChangeService, "resolve_page_url", staticmethod(lambda _actor_id, _page_url: connected))

    rebound = BusinessDocumentService.rebind_eva(
        TENANT,
        AUTHOR,
        document["document_id"],
        {"expected_state_version": document["state_version"]},
        is_admin=True,
    )

    assert rebound["state_version"] == document["state_version"] + 1
    assert rebound["eva_binding"] == connected
    assert rebound["permissions"]["delete"] is True
    assert BusinessDocumentEvent.get_by_id(created_event.id).payload == original_created_payload
    resolution_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "EvaBindingResolved"))
    assert resolution_event.payload == {"eva_binding": connected}


@pytest.mark.p0
def test_verified_eva_binding_supports_governed_pull_and_outbound_change(database, monkeypatch):
    from api.apps.business_documents.eva_changes import EvaDocumentChangeService

    binding = {
        "page_url": "https://eva.example.com/project/Document/BR-42",
        "status": "CONNECTED",
        "capabilities": ["OPEN", "PULL_FROM_EVA", "CREATE_EVA_CHANGE"],
        "connector_id": "connector-1",
        "project_id": "project-1",
        "document_id": "eva-document-1",
        "document_code": "BR-42",
        "document_name": "Требования EVA",
        "remote_version": "1|published-1|2026-09-01",
        "remote_content_hash": "sha256:initial",
        "last_pulled_content_hash": None,
    }
    monkeypatch.setattr(
        EvaDocumentChangeService,
        "resolve_page_url",
        staticmethod(lambda _actor_id, _page_url: binding),
    )
    document = _request_and_complete_draft(
        _create(
            title=binding["document_name"],
            schema_version="2",
            eva_page_url=binding["page_url"],
            eva_decision={"mode": "BIND", "confirm_replace": True},
        )
    )
    document = _complete_review_assessment(document)
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "APPLY_CHANGES",
            {"base_revision_id": document["current_revision"]["revision_id"]},
        ),
    )
    document = _complete(
        requested["job_id"],
        {
            "change_plan": {
                "schema_version": "1",
                "base_revision_id": document["current_revision"]["revision_id"],
                "source_state_version": requested["state_version"],
                "acknowledged_no_change_event_ids": [],
                "operations": [],
            }
        },
    )
    assert document["lifecycle_state"] == "AGREED"
    assert document["eva_binding"]["status"] == "CONNECTED"

    captured_change = {}

    def create_change(_tenant_id, _actor_id, raw):
        captured_change.update(raw)
        return {"change_id": "eva-change-1", "draft_markdown": raw["draft_markdown"]}

    monkeypatch.setattr(EvaDocumentChangeService, "create_change", staticmethod(create_change))
    outbound = BusinessDocumentService.create_eva_change_from_revision(
        TENANT,
        AUTHOR,
        document["document_id"],
        {"expected_state_version": document["state_version"]},
    )
    assert outbound["change_id"] == "eva-change-1"
    assert captured_change["connector_id"] == "connector-1"
    assert captured_change["document_id"] == "eva-document-1"
    assert captured_change["document_name"] == document["title"]
    assert captured_change["draft_markdown"] == document["current_revision"]["body_markdown"]

    class EvaClient:
        @staticmethod
        def get_document_for_edit(_document_id):
            return {
                "id": "eva-document-1",
                "version": "2|published-2|2026-09-01",
                "html": "<h1>Требования EVA</h1><p>Добавлен новый процесс.</p>",
            }

    monkeypatch.setattr(
        EvaDocumentChangeService,
        "_connector",
        staticmethod(lambda _connector_id, _actor_id: (None, EvaClient())),
    )
    pulled = BusinessDocumentService.pull_from_eva(
        TENANT,
        AUTHOR,
        document["document_id"],
        {"expected_state_version": document["state_version"]},
        is_admin=True,
    )
    assert pulled["sync"]["changed"] is True
    assert pulled["document"]["lifecycle_state"] == "REVIEW"
    assert pulled["document"]["active_review_cycle"] == document["active_review_cycle"] + 1
    assert pulled["document"]["permissions"]["delete"] is True
    assert pulled["document"]["allowed_commands"] == [
        "DECIDE_PROPOSAL",
        "ADD_COMMENT",
        "ARCHIVE",
        "REQUEST_REVIEW_ASSESSMENT",
    ]
    assert pulled["document"]["eva_binding"]["last_pulled_content_hash"].startswith("sha256:")
    pull_event = BusinessDocumentEvent.get(BusinessDocumentEvent.id == pulled["sync"]["event_id"])
    assert pull_event.event_type == "EvaDocumentPulled"
    assert "Добавлен новый процесс" in pull_event.payload["remote_markdown"]


@pytest.mark.p0
def test_export_from_review_requires_agreed_revision(database):
    document = _request_and_complete_draft(_create())
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(
            TENANT,
            AUTHOR,
            document["document_id"],
            _command(
                document,
                "REQUEST_EXPORT",
                {"revision_id": document["current_revision"]["revision_id"], "format": "DOCX"},
            ),
        )
    assert caught.value.code == "AGREED_REVISION_REQUIRED"


@pytest.mark.p1
@pytest.mark.parametrize(
    ("page", "page_size"),
    [(0, 20), (1, 0), (1, 101), ("1", 20)],
)
def test_list_pagination_boundaries_are_rejected(database, page, page_size):
    _create()

    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.list_documents(TENANT, AUTHOR, page=page, page_size=page_size)

    assert caught.value.code == "INVALID_PAGINATION"


@pytest.mark.p0
def test_answer_requires_exactly_one_valid_option_or_custom_text(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    document = _complete(requested["job_id"], _question_batch(), worker="worker")
    question_id = document["protocol"]["questions"][0]["question_id"]

    invalid_cases = [
        (
            {"question_id": question_id, "selected_option_id": "individuals", "custom_answer": "Свой ответ"},
            "INVALID_ANSWER",
        ),
        ({"question_id": question_id, "selected_option_id": None, "custom_answer": "  "}, "INVALID_ANSWER"),
        ({"question_id": question_id, "selected_option_id": "missing", "custom_answer": None}, "INVALID_OPTION"),
    ]
    for index, (payload, error_code) in enumerate(invalid_cases):
        with pytest.raises(BusinessDocumentError) as caught:
            BusinessDocumentService.execute_command(
                TENANT,
                AUTHOR,
                document["document_id"],
                _command(document, "ANSWER_QUESTION", payload, key=f"invalid-answer-{index}"),
            )
        assert caught.value.code == error_code

    assert BusinessDocumentAnswer.select().count() == 0
    after = BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)
    assert after["state_version"] == document["state_version"]


@pytest.mark.p0
def test_comment_anchor_requires_current_revision_and_preserves_selected_text(database):
    document = _request_and_complete_draft(_create())
    revision = document["current_revision"]
    section = next(item for item in revision["document_ast"]["sections"] if item["id"] == "5.5")
    section_text = render_section_text(section)
    selected_text = "Раздел 5.5: исходная версия."
    start_offset = section_text.index(selected_text)

    def anchor(*, anchor_revision=None, anchor_section="5.5", start=start_offset, end=None, selected=selected_text):
        end = start + len(selected) if end is None else end
        return {
            "revision_id": anchor_revision or revision["revision_id"],
            "section_id": anchor_section,
            "selected_text": selected,
            "prefix": section_text[max(0, start - 64) : start],
            "suffix": section_text[end : end + 64],
            "start_offset": start,
            "end_offset": end,
        }

    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(
            TENANT,
            AUTHOR,
            document["document_id"],
            _command(
                document,
                "ADD_COMMENT",
                {
                    "revision_id": "stale-revision",
                    "section_id": "5.5",
                    "text": "Уточнить",
                    "anchor": anchor(anchor_revision="stale-revision"),
                },
                key="stale-comment",
            ),
        )
    assert caught.value.code == "COMMENT_REVISION_CONFLICT"

    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(
            TENANT,
            AUTHOR,
            document["document_id"],
            _command(
                document,
                "ADD_COMMENT",
                {
                    "revision_id": revision["revision_id"],
                    "section_id": "5.5",
                    "text": "Уточнить",
                    "anchor": anchor(selected="Фрагмент, которого нет"),
                },
                key="missing-anchor",
            ),
        )
    assert caught.value.code == "INVALID_COMMENT_ANCHOR"

    for key, payload in (
        (
            "missing-section",
            {
                "revision_id": revision["revision_id"],
                "section_id": "9.9",
                "text": "Уточнить",
                "anchor": anchor(anchor_section="9.9"),
            },
        ),
        (
            "anchor-revision-mismatch",
            {
                "revision_id": revision["revision_id"],
                "section_id": "5.5",
                "text": "Уточнить",
                "anchor": anchor(anchor_revision="other-revision"),
            },
        ),
        (
            "anchor-offset-mismatch",
            {
                "revision_id": revision["revision_id"],
                "section_id": "5.5",
                "text": "Уточнить",
                "anchor": anchor(start=1, end=1 + len(selected_text)),
            },
        ),
    ):
        with pytest.raises(BusinessDocumentError) as caught:
            BusinessDocumentService.execute_command(
                TENANT,
                AUTHOR,
                document["document_id"],
                _command(document, "ADD_COMMENT", payload, key=key),
            )
        assert caught.value.code in {"COMMENT_SECTION_NOT_FOUND", "INVALID_COMMENT_ANCHOR"}

    immutable_anchor = anchor()
    response = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ADD_COMMENT",
            {
                "revision_id": revision["revision_id"],
                "section_id": "5.5",
                "text": "Добавить метрику отказов",
                "anchor": immutable_anchor,
            },
            key="valid-anchor",
        ),
    )
    stored = BusinessDocumentComment.get(BusinessDocumentComment.document_id == document["document_id"])
    assert stored.anchor == immutable_anchor
    assert response["state_version"] == document["state_version"] + 1
    document = BusinessDocumentService.get_document(TENANT, response["document_id"], AUTHOR)
    assert document["protocol"]["comments"][0]["anchor_status"] == "ANCHORED"

    comment_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "AuthorCommentAdded"))
    document = _complete_review_assessment(document)
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "APPLY_CHANGES", {"base_revision_id": revision["revision_id"]}),
    )
    base_section = next(section for section in revision["document_ast"]["sections"] if section["id"] == "5.5")
    document = _complete(
        requested["job_id"],
        {
            "change_plan": {
                "schema_version": "1",
                "base_revision_id": revision["revision_id"],
                "source_state_version": requested["state_version"],
                "acknowledged_no_change_event_ids": [],
                "operations": [
                    {
                        "operation_id": "op-anchor",
                        "type": "REPLACE_SECTION_CONTENT",
                        "section_id": "5.5",
                        "expected_section_hash": section_hash(base_section),
                        "source_event_ids": [comment_event.id],
                        "content": {"blocks": [{"type": "paragraph", "text": "Обновленный раздел"}]},
                    }
                ],
            }
        },
        worker="anchor-worker",
    )
    assert document["protocol"]["comments"][0]["anchor_status"] == "ORPHANED"
    assert BusinessDocumentComment.get_by_id(stored.id).anchor == immutable_anchor


@pytest.mark.p0
def test_comment_anchor_context_window_is_utf16_surrogate_safe(database):
    document = _create()
    assessed = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    document = _complete(assessed["job_id"], {"schema_version": "1", "outcome": "COMPLETE", "questions": []})
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_DRAFT"))
    draft = _draft()
    section = next(item for item in draft["sections"] if item["id"] == "5.5")
    section["blocks"] = [{"type": "paragraph", "text": "😀" + "a" * 63 + "SELECT"}]
    document = _complete(
        requested["job_id"],
        {
            "draft": draft,
            "review_questions": {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
            "proposals": [],
        },
        worker="emoji-anchor-worker",
    )
    revision = document["current_revision"]
    response = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ADD_COMMENT",
            {
                "revision_id": revision["revision_id"],
                "section_id": "5.5",
                "text": "Emoji boundary",
                "anchor": {
                    "revision_id": revision["revision_id"],
                    "section_id": "5.5",
                    "selected_text": "SELECT",
                    "prefix": "a" * 63,
                    "suffix": "",
                    "start_offset": 65,
                    "end_offset": 71,
                },
            },
        ),
    )
    assert response["accepted"] is True


@pytest.mark.p0
def test_review_reassessment_appends_questions_and_proposals_without_changing_body(database):
    document = _request_and_complete_draft(_create())
    assert document["current_revision"]["section_texts"] == {section["id"]: render_section_text(section) for section in document["current_revision"]["document_ast"]["sections"]}
    original_revision = deepcopy(document["current_revision"])
    comment_response = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ADD_COMMENT",
            {
                "revision_id": original_revision["revision_id"],
                "section_id": None,
                "text": "Нужна метрика бизнес-отказов",
                "anchor": None,
            },
        ),
    )
    document = BusinessDocumentService.get_document(TENANT, comment_response["document_id"], AUTHOR)
    comment_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "AuthorCommentAdded"))
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "REQUEST_REVIEW_ASSESSMENT"),
    )
    document = _complete(
        requested["job_id"],
        {
            "schema_version": "1",
            "questions": [
                {
                    "semantic_tag": "monitoring.business_failures",
                    "target_section_id": "5.5",
                    "text": "Какие бизнес-отказы учитывать?",
                    "options": [
                        {"option_id": "all", "label": "Все отказы"},
                        {"option_id": "final", "label": "Только финальные"},
                    ],
                    "allow_custom_answer": True,
                }
            ],
            "proposals": [
                {
                    "target_section_id": "5.5",
                    "text": "Добавить долю бизнес-отказов",
                    "rationale": "Комментарий автора требует наблюдаемой метрики",
                    "source_event_ids": [comment_event.id],
                }
            ],
            "comment_dispositions": [
                {
                    "comment_event_id": comment_event.id,
                    "disposition": "NEEDS_QUESTION",
                    "question_semantic_tag": "monitoring.business_failures",
                }
            ],
        },
    )

    assert document["current_revision"] == original_revision
    assert len(document["protocol"]["questions"]) == 1
    assert len(document["protocol"]["proposals"]) == 1
    assert document["protocol"]["proposals"][0]["decision"] == "PENDING"
    assert "ANSWER_QUESTION" in document["allowed_commands"]
    assert "APPLY_CHANGES" not in document["allowed_commands"]
    proposal = BusinessDocumentProposal.get(BusinessDocumentProposal.document_id == document["document_id"])
    question = BusinessDocumentQuestion.get(BusinessDocumentQuestion.document_id == document["document_id"])
    assert comment_event.id in proposal.source_event_ids
    assert comment_event.id in question.source_event_ids
    assert document["protocol"]["comments"][0]["disposition"]["question_id"] == question.id
    assert BusinessDocumentQuestion.select().where(BusinessDocumentQuestion.document_id == document["document_id"]).count() == 1


@pytest.mark.p0
def test_review_plan_requires_complete_comment_dispositions(database):
    document = _request_and_complete_draft(_create())
    response = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ADD_COMMENT",
            {
                "revision_id": document["current_revision"]["revision_id"],
                "section_id": None,
                "text": "Проверить сценарий отказа",
                "anchor": None,
            },
        ),
    )
    document = BusinessDocumentService.get_document(TENANT, response["document_id"], AUTHOR)
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_REVIEW_ASSESSMENT"))

    with pytest.raises(BusinessDocumentError) as caught:
        _complete(
            requested["job_id"],
            {"schema_version": "1", "questions": [], "proposals": [], "comment_dispositions": []},
        )

    assert caught.value.code == "COMMENT_DISPOSITION_INCOMPLETE"


@pytest.mark.p0
def test_review_reassessment_rejects_reused_answered_question_tag_before_insertion(database):
    document = _request_and_complete_draft(_create())
    response = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ADD_COMMENT",
            {
                "revision_id": document["current_revision"]["revision_id"],
                "section_id": None,
                "text": "Добавить модераторов",
                "anchor": None,
            },
        ),
    )
    document = BusinessDocumentService.get_document(TENANT, response["document_id"], AUTHOR)
    comment_event = BusinessDocumentEvent.get(
        (BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "AuthorCommentAdded")
    )
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_REVIEW_ASSESSMENT"))
    document = _complete(
        requested["job_id"],
        {
            "schema_version": "1",
            "questions": [
                {
                    "semantic_tag": "MODERATOR_SCOPE",
                    "target_section_id": "3.1",
                    "text": "Каких модераторов добавить?",
                    "options": [{"option_id": "all", "label": "Всех"}, {"option_id": "selected", "label": "Только отдельных"}],
                    "allow_custom_answer": True,
                }
            ],
            "proposals": [],
            "comment_dispositions": [
                {
                    "comment_event_id": comment_event.id,
                    "disposition": "NEEDS_QUESTION",
                    "question_semantic_tag": "moderator_scope",
                }
            ],
        },
    )
    question = document["protocol"]["questions"][0]
    answered = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ANSWER_QUESTION",
            {"question_id": question["question_id"], "selected_option_id": "all", "custom_answer": None},
        ),
    )
    document = BusinessDocumentService.get_document(TENANT, answered["document_id"], AUTHOR)
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "REQUEST_REVIEW_ASSESSMENT", key="reassess-closed-question"),
    )

    with pytest.raises(BusinessDocumentError) as caught:
        _complete(
            requested["job_id"],
            {
                "schema_version": "1",
                "questions": [
                    {
                        "semantic_tag": "MODERATOR_SCOPE",
                        "target_section_id": "3.1",
                        "text": "Нужно ли добавить всех модераторов без исключения?",
                        "options": [{"option_id": "yes", "label": "Да"}, {"option_id": "no", "label": "Нет"}],
                        "allow_custom_answer": True,
                    }
                ],
                "proposals": [],
                "comment_dispositions": [
                    {
                        "comment_event_id": comment_event.id,
                        "disposition": "NEEDS_QUESTION",
                        "question_semantic_tag": "MODERATOR_SCOPE",
                    }
                ],
            },
        )

    assert caught.value.code == "COMMENT_DISPOSITION_QUESTION_CLOSED"
    assert caught.value.details["question_semantic_tag"] == "moderator_scope"
    assert "new semantic_tag" in caught.value.details["required_action"]
    assert BusinessDocumentQuestion.select().where(BusinessDocumentQuestion.document_id == document["document_id"]).count() == 1


@pytest.mark.p0
def test_semantic_dedupe_keeps_questions_and_proposals_immutable(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    document = _complete(requested["job_id"], _question_batch(semantic_tag="AUDIENCE.USERS"))
    question = document["protocol"]["questions"][0]
    answered = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ANSWER_QUESTION",
            {"question_id": question["question_id"], "selected_option_id": "individuals", "custom_answer": None},
        ),
    )
    document = BusinessDocumentService.get_document(TENANT, answered["document_id"], AUTHOR)
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT", key="repeat-intake"))
    document = _complete(requested["job_id"], _question_batch(semantic_tag="audience.users"))
    assert BusinessDocumentQuestion.select().where(BusinessDocumentQuestion.document_id == document["document_id"]).count() == 1
    intake_event = (
        BusinessDocumentEvent.select()
        .where((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "IntakeAssessed"))
        .order_by(BusinessDocumentEvent.sequence.desc())
        .get()
    )
    assert intake_event.payload["question_count"] == 0
    assert intake_event.payload["outcome"] == "COMPLETE"
    assert "REQUEST_DRAFT" in document["allowed_commands"]

    document = _request_and_complete_draft(document)
    comment_response = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ADD_COMMENT",
            {
                "revision_id": document["current_revision"]["revision_id"],
                "section_id": None,
                "text": "Добавить метрику отказов",
                "anchor": None,
            },
        ),
    )
    document = BusinessDocumentService.get_document(TENANT, comment_response["document_id"], AUTHOR)
    comment_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "AuthorCommentAdded"))

    def assess(proposal_text, rationale):
        nonlocal document
        request = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_REVIEW_ASSESSMENT"))
        document = _complete(
            request["job_id"],
            {
                "schema_version": "1",
                "questions": [],
                "proposals": [
                    {
                        "target_section_id": "5.5",
                        "text": proposal_text,
                        "rationale": rationale,
                        "source_event_ids": [comment_event.id],
                    }
                ],
                "comment_dispositions": [{"comment_event_id": comment_event.id, "disposition": "CONFIRMED_CHANGE"}],
            },
        )

    assess("Добавить метрику отказов", "Первоначальное обоснование")
    original = BusinessDocumentProposal.get(BusinessDocumentProposal.document_id == document["document_id"])
    original_snapshot = (original.id, original.text, original.rationale, original.source_event_ids, original.create_time)
    assess("  ДОБАВИТЬ   МЕТРИКУ ОТКАЗОВ  ", "Новое обоснование не должно перезаписать строку")
    proposals = list(BusinessDocumentProposal.select().where(BusinessDocumentProposal.document_id == document["document_id"]))
    assert len(proposals) == 1
    assert (proposals[0].id, proposals[0].text, proposals[0].rationale, proposals[0].source_event_ids, proposals[0].create_time) == original_snapshot


@pytest.mark.p0
def test_export_requires_agreed_lifecycle_and_current_revision(database):
    document = _request_and_complete_draft(_create())
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentService.execute_command(
            TENANT,
            AUTHOR,
            document["document_id"],
            _command(
                document,
                "REQUEST_EXPORT",
                {"revision_id": document["current_revision"]["revision_id"], "format": "EVA_WIKI"},
            ),
        )

    assert caught.value.code == "AGREED_REVISION_REQUIRED"
    assert BusinessDocumentJob.select().where(BusinessDocumentJob.job_type == "GENERATE_EXPORT").count() == 0

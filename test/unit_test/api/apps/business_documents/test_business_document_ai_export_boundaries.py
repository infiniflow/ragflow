#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#

from __future__ import annotations

import hashlib
from datetime import datetime
from pathlib import Path
import sys
from types import ModuleType, SimpleNamespace

import pytest
from peewee import SqliteDatabase


if "api.apps" not in sys.modules:
    api_apps = ModuleType("api.apps")
    api_apps.__path__ = [str(Path(__file__).resolve().parents[5] / "api" / "apps")]
    sys.modules["api.apps"] = api_apps

from api.apps.business_documents.ai import BusinessDocumentAI
from api.apps.business_documents.assets import (
    apply_change_plan,
    bind_change_plan_section_hashes,
    prompt_descriptor,
    prompt_text,
    published_template,
    render_document_ast,
    section_hash,
    validate_document_ast,
)
from api.apps.business_documents.errors import BusinessDocumentError
from api.apps.business_documents.exports import BusinessDocumentExportService
from api.apps.business_documents.service import BusinessDocumentService
from api.apps.business_documents.worker import BusinessDocumentJobQueue, BusinessDocumentWorker
from api.db.db_models import BusinessDocument, BusinessDocumentEvent, BusinessDocumentExportArtifact, BusinessDocumentJob, BusinessDocumentRevision
from test.unit_test.api.apps.business_documents.helpers import required_section_blocks
from common.time_utils import current_timestamp


TENANT = "tenant-ai-boundary"
AUTHOR = "author-ai-boundary"


@pytest.fixture()
def database():
    database = SqliteDatabase(":memory:")
    tables = BusinessDocumentService.model_tables()
    with database.bind_ctx(tables, bind_refs=False, bind_backrefs=False):
        database.connect()
        database.create_tables(tables)
        yield database
        database.drop_tables(tables)
        database.close()


def _create(*, title="Проверяемый документ"):
    return BusinessDocumentService.create_document(
        TENANT,
        AUTHOR,
        {
            "schema_version": "1",
            "document_type": "business_requirements",
            "title": title,
            "idea": "Создать проверяемый сервис",
        },
    )


def _command(document, command_type, payload=None, suffix=""):
    return {
        "schema_version": "1",
        "command_id": f"cmd-{command_type}-{document['state_version']}{suffix}",
        "idempotency_key": f"idem-{command_type}-{document['state_version']}{suffix}",
        "expected_state_version": document["state_version"],
        "type": command_type,
        "payload": payload or {},
    }


def _draft():
    return {
        "schema_version": "1",
        "document_type": "business_requirements",
        "template_version": published_template()["template_version"],
        "sections": [
            {
                "id": section["id"],
                "title": section["title"],
                "blocks": required_section_blocks(section["id"], f"Содержание раздела {section['id']}."),
            }
            for section in published_template()["sections"]
        ],
    }


@pytest.mark.p0
def test_document_ast_restores_missing_optional_sections_from_published_template(database):
    draft = _draft()
    optional_ids = {section["id"] for section in published_template()["sections"] if not section["required"]}
    draft["sections"] = [section for section in draft["sections"] if section["id"] not in optional_ids]

    normalized = validate_document_ast(draft)

    assert [section["id"] for section in normalized["sections"]] == [section["id"] for section in published_template()["sections"]]
    assert all(section["blocks"] == [] for section in normalized["sections"] if section["id"] in optional_ids)


@pytest.mark.p0
def test_document_ast_removes_template_only_section_metadata(database):
    draft = _draft()
    draft["sections"][3]["parent_id"] = "3"
    draft["sections"][3]["required"] = True
    draft["sections"][3]["allowed_blocks"] = ["paragraph"]
    draft["sections"][3]["semantic_requirements"] = ["TEMPLATE_ONLY_RULE"]

    normalized = validate_document_ast(draft)

    assert set(normalized["sections"][3]) == {"id", "title", "blocks"}


@pytest.mark.p0
def test_document_ast_flattens_template_sections_returned_inside_parent_blocks(database):
    draft = _draft()
    child = draft["sections"].pop(3)
    child["parent_id"] = "3"
    draft["sections"][2]["blocks"].append(child)

    normalized = validate_document_ast(draft)

    assert [section["id"] for section in normalized["sections"]] == [section["id"] for section in published_template()["sections"]]
    assert all("id" not in block for block in normalized["sections"][2]["blocks"])
    assert normalized["sections"][3]["id"] == "3.1"
    assert set(normalized["sections"][3]) == {"id", "title", "blocks"}


@pytest.mark.p0
def test_document_ast_accepts_a_complete_plantuml_activity_scenario(database):
    draft = _draft()

    validated = validate_document_ast(draft)

    scenario = next(section for section in validated["sections"] if section["id"] == "4.3")
    activity = next(block for block in scenario["blocks"] if block["type"] == "plantuml")
    assert "if (Проверка успешна?) then (Да)" in activity["source"]
    assert "else (Нет)" in activity["source"]


@pytest.mark.p0
@pytest.mark.parametrize(
    ("source", "missing"),
    [
        ("@startuml\nif (Успех?) then (Да)\nelse (Нет)\nendif\nstop\n@enduml", "start"),
        ("@startuml\nstart\nif (Успех?) then (Да)\nelse (Нет)\nendif\n@enduml", "end"),
        ("@startuml\nstart\n:Действие;\nstop\n@enduml", "decision"),
        ("@startuml\nstart\nif (Успех?) then (Да)\nelse (Да)\nendif\nstop\n@enduml", "negative path"),
    ],
)
def test_document_ast_rejects_incomplete_plantuml_activity_scenarios(database, source, missing):
    draft = _draft()
    scenario = next(section for section in draft["sections"] if section["id"] == "4.3")
    scenario["blocks"][-1]["source"] = source

    with pytest.raises(BusinessDocumentError) as caught:
        validate_document_ast(draft)

    assert caught.value.code == "INCOMPLETE_ACTIVITY_SCENARIO", missing


@pytest.mark.p0
def test_document_ast_restores_required_structural_parents_from_child_content(database):
    draft = _draft()
    draft["sections"] = [section for section in draft["sections"] if section["id"] not in {"3", "4", "5"}]

    normalized = validate_document_ast(draft)

    sections = {section["id"]: section for section in normalized["sections"]}
    assert sections["3"]["blocks"]
    assert sections["4"]["blocks"][0]["type"] == "paragraph"
    assert sections["5"]["blocks"]


@pytest.mark.p0
def test_change_plan_hashes_are_bound_from_server_snapshot(database):
    base = _draft()
    target = next(section for section in base["sections"] if section["id"] == "3.3")
    plan = {
        "schema_version": "1",
        "base_revision_id": "revision-1",
        "source_state_version": 7,
        "acknowledged_no_change_event_ids": [],
        "operations": [
            {
                "operation_id": "replace-need",
                "type": "REPLACE_SECTION_CONTENT",
                "section_id": "3.3",
                "expected_section_hash": "sha256:not-a-model-owned-value",
                "source_event_ids": ["event-1"],
                "content": {"blocks": [{"type": "paragraph", "text": "Уточнённая потребность."}]},
            }
        ],
    }

    bound = bind_change_plan_section_hashes(base, plan)

    assert bound["operations"][0]["expected_section_hash"] == section_hash(target)
    assert plan["operations"][0]["expected_section_hash"] == "sha256:not-a-model-owned-value"
    assert apply_change_plan(base, bound)["sections"][5]["blocks"][0]["text"] == "Уточнённая потребность."


@pytest.mark.p0
def test_change_plan_drops_no_change_acknowledgements_from_old_review_cycles(database):
    base = _draft()
    job = BusinessDocumentJob(
        id="plan-job",
        tenant_id=TENANT,
        document_id="document-1",
        job_type="PLAN_CHANGES",
        status="RUNNING",
        dedupe_key="plan-job",
        source_state_version=7,
        base_revision_id="revision-1",
        payload={
            "active_change_input_event_ids": ["active-comment"],
            "current_revision": {"document_ast": base},
        },
        attempt=1,
        max_attempts=3,
        available_at=0,
        correlation_id="plan-job",
    )
    output = {
        "schema_version": "1",
        "base_revision_id": "revision-1",
        "source_state_version": 7,
        "acknowledged_no_change_event_ids": ["old-answer-1", "old-answer-2"],
        "operations": [
            {
                "operation_id": "replace-need",
                "type": "REPLACE_SECTION_CONTENT",
                "section_id": "3.3",
                "expected_section_hash": "sha256:" + "0" * 64,
                "source_event_ids": ["active-comment"],
                "content": {"blocks": [{"type": "paragraph", "text": "Добавить чатбот."}]},
            }
        ],
    }

    validated = BusinessDocumentAI._validate(job, output)

    assert validated["change_plan"]["acknowledged_no_change_event_ids"] == []


def _claim_complete(job_id, output, worker_id="boundary-worker"):
    job = BusinessDocumentJobQueue.claim(worker_id, lease_ms=60_000)
    assert job is not None and job.id == job_id
    return BusinessDocumentService.complete_job(TENANT, worker_id, job.id, output, job.lease_token)


class CapturingAdapter:
    def __init__(self, response):
        self.response = response
        self.calls = []

    def generate(self, tenant_id, system_prompt, input_payload):
        self.calls.append((tenant_id, system_prompt, input_payload))
        return self.response


@pytest.mark.p0
def test_eva_change_ai_uses_versioned_contract_and_untrusted_input_boundary():
    adapter = CapturingAdapter(
        {
            "schema_version": 1,
            "draft_markdown": "# Требования\n\nНовый результат.",
            "summary": "Уточнён результат",
        }
    )

    result = BusinessDocumentAI(adapter).generate_eva_change(
        TENANT,
        "# Требования\n\nСтарый результат.",
        "Сделать результат измеримым.",
    )

    assert result["schema_version"] == "1"
    tenant_id, system_prompt, input_payload = adapter.calls[0]
    assert tenant_id == TENANT
    assert "недоверенные данные" in system_prompt
    assert '"draft_markdown"' in system_prompt
    assert input_payload["prompt"] == prompt_descriptor("GENERATE_EVA_CHANGE")
    assert input_payload["job_input"]["base_markdown"].endswith("Старый результат.")
    assert input_payload["job_input"]["change_request"] == "Сделать результат измеримым."


@pytest.mark.p0
def test_eva_change_ai_rejects_unstructured_output():
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentAI(CapturingAdapter({"draft_markdown": "# Без контракта"})).generate_eva_change(
            TENANT,
            "# Требования",
            "Изменить документ",
        )

    assert caught.value.code == "INVALID_EVA_CHANGE_DRAFT"


@pytest.mark.p0
def test_ai_repairs_json_validates_schema_and_persists_pinned_prompt_audit(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    adapter = CapturingAdapter("```json\n{'schema_version':'1','outcome':'COMPLETE','questions':[],}\n```")
    worker = BusinessDocumentWorker(worker_id="ai-worker", ai=BusinessDocumentAI(adapter), lease_ms=60_000)
    assert worker.run_once() is True

    job = BusinessDocumentJob.get_by_id(requested["job_id"])
    prompt = job.payload["prompt"]
    assert job.status == "COMPLETED"
    assert job.result["prompt_version"] == prompt["version"]
    assert job.result["prompt_hash"] == prompt["content_hash"]
    assert job.result["output"] == {"schema_version": "1", "outcome": "COMPLETE", "questions": []}
    tenant_id, system_prompt, input_payload = adapter.calls[0]
    assert tenant_id == TENANT
    assert prompt_text("intake").splitlines()[0] in system_prompt
    assert "{{context_json}}" not in system_prompt
    assert '"id": "5.5"' in system_prompt
    assert '"policy_id": "business-requirements-process"' in system_prompt
    assert "Не создавай и не угадывай идентификаторы событий" in system_prompt
    assert input_payload["prompt"] == prompt
    assert "schemas" not in input_payload
    allowed_event_ids = {event["event_id"] for event in input_payload["job_input"]["source_events"]}
    assert input_payload["job_input"]["idea_source_event_id"] in allowed_event_ids
    event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "IntakeAssessed"))
    assert event.payload["prompt_version"] == prompt["version"]
    assert event.payload["prompt_hash"] == prompt["content_hash"]

    next_request = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR), "REQUEST_DRAFT"),
    )
    invalid = CapturingAdapter("{}")
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentAI(invalid).process(BusinessDocumentJob.get_by_id(next_request["job_id"]))
    assert caught.value.code == "INVALID_DRAFT_BUNDLE"


@pytest.mark.p0
def test_ai_unwraps_exact_contract_name_envelope(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    adapter = CapturingAdapter(
        {
            "question_batch": {
                "schema_version": "1",
                "outcome": "COMPLETE",
                "questions": [],
            }
        }
    )

    worker = BusinessDocumentWorker(worker_id="ai-worker", ai=BusinessDocumentAI(adapter), lease_ms=60_000)
    assert worker.run_once() is True

    job = BusinessDocumentJob.get_by_id(requested["job_id"])
    assert job.status == "COMPLETED"
    assert job.result["output"] == {"schema_version": "1", "outcome": "COMPLETE", "questions": []}


@pytest.mark.p0
def test_ai_drops_redundant_contract_name_property_when_root_contract_is_valid(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    contract = {"schema_version": "1", "outcome": "COMPLETE", "questions": []}
    adapter = CapturingAdapter({**contract, "question_batch": contract})

    worker = BusinessDocumentWorker(worker_id="ai-worker", ai=BusinessDocumentAI(adapter), lease_ms=60_000)
    assert worker.run_once() is True

    job = BusinessDocumentJob.get_by_id(requested["job_id"])
    assert job.status == "COMPLETED"
    assert job.result["output"] == contract


@pytest.mark.p0
def test_ai_normalizes_numeric_published_schema_versions_recursively():
    output = {
        "schema_version": 1,
        "nested": {"schema_version": 1},
        "items": [{"schema_version": 2}],
    }

    assert BusinessDocumentAI._normalize_schema_versions(output) == {
        "schema_version": "1",
        "nested": {"schema_version": "1"},
        "items": [{"schema_version": 2}],
    }


@pytest.mark.p0
def test_ai_uses_valid_named_contract_when_sibling_properties_are_invalid(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    contract = {"schema_version": "1", "outcome": "COMPLETE", "questions": []}
    adapter = CapturingAdapter({"question_batch": contract, "explanation": "done"})

    worker = BusinessDocumentWorker(worker_id="ai-worker", ai=BusinessDocumentAI(adapter), lease_ms=60_000)
    assert worker.run_once() is True

    job = BusinessDocumentJob.get_by_id(requested["job_id"])
    assert job.status == "COMPLETED"
    assert job.result["output"] == contract


@pytest.mark.p0
def test_ai_drops_echoed_schema_and_restores_its_constant_version(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    contract = {"schema_version": "1", "outcome": "COMPLETE", "questions": []}
    echoed_schema = {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "type": "object",
        "properties": {"schema_version": {"const": "1"}},
    }
    adapter = CapturingAdapter({"question_batch": echoed_schema, "outcome": "COMPLETE", "questions": []})

    worker = BusinessDocumentWorker(worker_id="ai-worker", ai=BusinessDocumentAI(adapter), lease_ms=60_000)
    assert worker.run_once() is True

    job = BusinessDocumentJob.get_by_id(requested["job_id"])
    assert job.status == "COMPLETED"
    assert job.result["output"] == contract


@pytest.mark.p0
def test_ai_treats_an_exact_repeated_answered_question_as_complete():
    text = "Какие ключевые потребности решает сервис?"
    job = SimpleNamespace(
        job_type="ASSESS_INTAKE",
        payload={
            "protocol": {
                "questions": [
                    {
                        "stage": "INTAKE",
                        "target_section_id": "3.3",
                        "semantic_tag": "original_tag",
                        "text": text,
                        "status": "ANSWERED",
                    }
                ]
            }
        },
    )
    repeated = {
        "schema_version": "1",
        "outcome": "NEEDS_INPUT",
        "questions": [
            {
                "stage": "INTAKE",
                "target_section_id": "3.3",
                "semantic_tag": "renamed_tag",
                "text": f"  {text.upper()}  ",
                "options": [{"option_id": "a", "label": "A"}, {"option_id": "b", "label": "B"}],
                "allow_custom_answer": True,
            }
        ],
    }

    assert BusinessDocumentAI._drop_answered_questions(job, repeated) == {
        "schema_version": "1",
        "outcome": "COMPLETE",
        "questions": [],
    }


@pytest.mark.p0
def test_ai_retry_prompt_contains_previous_validation_error(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    job = BusinessDocumentJob.get_by_id(requested["job_id"])
    job.attempt = 2
    job.error = {"code": "INCOMPLETE_ACTIVITY_SCENARIO", "message": "Negative alternative path is missing", "details": {}}

    prompt = BusinessDocumentAI._prompt(job)

    assert prompt.input_payload["job_input"]["retry_feedback"] == job.error
    assert "Исправление предыдущей попытки" in prompt.system
    assert "retry_feedback" not in job.payload


@pytest.mark.p0
def test_review_prompt_forbids_reusing_answered_question_tags_for_comments():
    job = SimpleNamespace(
        job_type="ASSESS_REVIEW",
        attempt=1,
        error=None,
        payload={
            "prompt": prompt_descriptor("ASSESS_REVIEW"),
            "protocol": {
                "questions": [
                    {
                        "question_id": "question-1",
                        "semantic_tag": "eva_sync_completeness",
                        "target_section_id": "4.3",
                        "status": "ANSWERED",
                        "answer": {"selected_option_id": "negative_path_absent_or_incomplete"},
                    }
                ],
                "comments": [
                    {
                        "source_event_id": "comment-event-1",
                        "disposition": {
                            "disposition": "NEEDS_QUESTION",
                            "question_id": "question-1",
                            "question_semantic_tag": "eva_sync_completeness",
                        },
                    }
                ],
            },
        },
    )

    prompt = BusinessDocumentAI._prompt(job)

    assert prompt.input_payload["job_input"]["closed_review_question_semantic_tags"] == ["eva_sync_completeness"]
    assert prompt.input_payload["job_input"]["resolved_comment_questions"] == [
        {
            "comment_event_id": "comment-event-1",
            "question_id": "question-1",
            "question_semantic_tag": "eva_sync_completeness",
            "answer": {"selected_option_id": "negative_path_absent_or_incomplete"},
        }
    ]
    assert "не используй закрытый semantic_tag в NEEDS_QUESTION" in prompt.system


@pytest.mark.p0
@pytest.mark.parametrize(("questions", "expected"), [([], "COMPLETE"), ([{"stage": "REVIEW"}], "NEEDS_INPUT")])
def test_ai_maps_draft_review_lifecycle_marker_to_question_outcome(questions, expected):
    job = SimpleNamespace(job_type="GENERATE_DRAFT")
    output = {
        "draft": {},
        "review_questions": {"schema_version": "1", "outcome": "REVIEW", "questions": questions},
        "proposals": [],
    }

    normalized = BusinessDocumentAI._normalize_draft_review_outcome(job, output)

    assert normalized["review_questions"]["outcome"] == expected
    assert normalized["review_questions"]["questions"] == questions


@pytest.mark.p0
def test_ai_drops_dispositions_for_events_that_are_not_comments():
    job = SimpleNamespace(
        job_type="ASSESS_REVIEW",
        payload={"protocol": {"comments": [{"source_event_id": "real-comment"}]}},
    )
    output = {
        "schema_version": "1",
        "questions": [],
        "proposals": [],
        "comment_dispositions": [
            {"comment_event_id": "real-comment", "disposition": "NO_CHANGE"},
            {"comment_event_id": "answer-event", "disposition": "CONFIRMED_CHANGE"},
        ],
    }

    normalized = BusinessDocumentAI._drop_unknown_comment_dispositions(job, output)

    assert normalized["comment_dispositions"] == [{"comment_event_id": "real-comment", "disposition": "NO_CHANGE"}]


@pytest.mark.p0
def test_ai_drops_an_exact_proposal_repeat_from_the_active_review_protocol():
    proposal = {
        "target_section_id": "4.3",
        "text": "Добавить явный негативный путь",
        "rationale": "Проверяемость",
        "source_event_ids": ["review-request-2"],
    }
    job = SimpleNamespace(
        job_type="ASSESS_REVIEW",
        payload={
            "protocol": {
                "proposals": [
                    {
                        **proposal,
                        "text": "  ДОБАВИТЬ  ЯВНЫЙ НЕГАТИВНЫЙ ПУТЬ ",
                        "source_event_ids": ["review-request-1"],
                    }
                ]
            }
        },
    )
    output = {
        "schema_version": "1",
        "questions": [],
        "proposals": [proposal],
        "comment_dispositions": [],
    }

    normalized = BusinessDocumentAI._drop_existing_proposals(job, output)

    assert normalized == {**output, "proposals": []}


@pytest.mark.p0
def test_ai_keeps_only_evidence_refs_from_the_pinned_snapshot():
    allowed = "ragflow://dataset/d/document/doc/chunk/allowed"
    invented = "ragflow://dataset/d/document/doc/chunk/invented"
    output = {
        "questions": [{"evidence_refs": [allowed, invented]}],
        "proposals": [{"evidence_refs": [invented]}],
    }
    evidence = {"chunks": [{"source_ref": allowed}]}

    normalized = BusinessDocumentAI._filter_evidence_refs(output, evidence)

    assert normalized == {
        "questions": [{"evidence_refs": [allowed]}],
        "proposals": [{"evidence_refs": []}],
    }


@pytest.mark.p0
def test_ai_routes_change_sources_to_the_operation_for_their_declared_section():
    job = SimpleNamespace(
        job_type="PLAN_CHANGES",
        payload={
            "protocol": {
                "questions": [
                    {
                        "target_section_id": "4.3",
                        "answer": {"source_event_id": "answer-43"},
                    }
                ],
                "proposals": [
                    {
                        "target_section_id": "4.1",
                        "decision": "ACCEPTED",
                        "decision_event_id": "proposal-41",
                    },
                    {
                        "target_section_id": "4.3",
                        "decision": "ACCEPTED",
                        "decision_event_id": "proposal-43-omitted",
                    },
                ],
            }
        },
    )
    output = {
        "operations": [
            {"section_id": "4.1", "source_event_ids": ["answer-43", "proposal-41", "comment"]},
            {"section_id": "4.3", "source_event_ids": []},
        ]
    }

    normalized = BusinessDocumentAI._bind_change_plan_source_sections(job, output)

    assert normalized["operations"] == [
        {"section_id": "4.1", "source_event_ids": ["proposal-41", "comment"]},
        {"section_id": "4.3", "source_event_ids": ["answer-43", "proposal-43-omitted"]},
    ]
    assert output["operations"][0]["source_event_ids"] == ["answer-43", "proposal-41", "comment"]


@pytest.mark.p0
def test_ai_acknowledges_confirming_answer_when_no_operation_targets_its_section():
    job = SimpleNamespace(
        job_type="PLAN_CHANGES",
        payload={
            "protocol": {
                "questions": [{"target_section_id": "3.1", "answer": {"source_event_id": "answer-confirmed"}}],
                "proposals": [],
            }
        },
    )
    output = {
        "acknowledged_no_change_event_ids": [],
        "operations": [{"section_id": "4.3", "source_event_ids": ["answer-confirmed", "comment-change"]}],
    }

    normalized = BusinessDocumentAI._bind_change_plan_source_sections(job, output)

    assert normalized["operations"][0]["source_event_ids"] == ["comment-change"]
    assert normalized["acknowledged_no_change_event_ids"] == ["answer-confirmed"]
    assert output["operations"][0]["source_event_ids"] == ["answer-confirmed", "comment-change"]


@pytest.mark.p0
def test_ai_binds_entity_aliases_and_completes_active_eva_change_dispositions():
    job = SimpleNamespace(
        job_type="PLAN_CHANGES",
        payload={
            "active_change_input_event_ids": ["answer-4", "answer-43", "proposal-43-event", "eva-pull-event"],
            "source_events": [
                {"event_id": "answer-4", "event_type": "QuestionAnswered"},
                {"event_id": "answer-43", "event_type": "QuestionAnswered"},
                {"event_id": "proposal-43-event", "event_type": "ProposalDecided"},
                {"event_id": "eva-pull-event", "event_type": "EvaDocumentPulled"},
            ],
            "protocol": {
                "questions": [
                    {
                        "question_id": "question-4",
                        "target_section_id": "4",
                        "answer": {"source_event_id": "answer-4"},
                    },
                    {
                        "question_id": "question-43",
                        "target_section_id": "4.3",
                        "answer": {"source_event_id": "answer-43"},
                    },
                ],
                "proposals": [
                    {
                        "proposal_id": "proposal-43",
                        "target_section_id": "4.3",
                        "decision": "ACCEPTED",
                        "decision_event_id": "proposal-43-event",
                    }
                ],
                "comments": [],
            },
        },
    )
    output = {
        "acknowledged_no_change_event_ids": [],
        "operations": [
            {
                "section_id": "4.3",
                "source_event_ids": ["proposal-43"],
            }
        ],
    }

    normalized = BusinessDocumentAI._bind_change_plan_source_sections(job, output)

    assert set(normalized["operations"][0]["source_event_ids"]) == {
        "answer-43",
        "proposal-43-event",
        "eva-pull-event",
    }
    assert normalized["acknowledged_no_change_event_ids"] == ["answer-4"]
    assert output["operations"][0]["source_event_ids"] == ["proposal-43"]


class MemoryStorage:
    def __init__(self, *, discard=False):
        self.discard = discard
        self.objects = {}
        self.removed = []

    def put(self, bucket, key, content):
        if not self.discard:
            self.objects[(bucket, key)] = content

    def get(self, bucket, key):
        return self.objects.get((bucket, key))

    def rm(self, bucket, key):
        self.removed.append((bucket, key))
        self.objects.pop((bucket, key), None)


def _agreed_document():
    projection = _create()
    ast = _draft()
    body = render_document_ast(ast)
    created = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == projection["document_id"]) & (BusinessDocumentEvent.event_type == "DocumentCreated"))
    now_ms = current_timestamp()
    revision = BusinessDocumentRevision.create(
        id="agreed-revision-ai-boundary",
        document_id=projection["document_id"],
        revision_number=1,
        document_ast=ast,
        body_markdown=body,
        content_hash=f"sha256:{hashlib.sha256(body.encode()).hexdigest()}",
        source_event_ids=[created.id],
        create_time=now_ms,
        create_date=datetime.now(),
        update_time=now_ms,
        update_date=datetime.now(),
    )
    BusinessDocument.update(
        lifecycle_state="AGREED",
        operation_state="IDLE",
        state_version=2,
        current_revision_id=revision.id,
    ).where(BusinessDocument.id == projection["document_id"]).execute()
    return BusinessDocumentService.get_document(TENANT, projection["document_id"], AUTHOR)


@pytest.mark.p0
def test_docx_export_requires_verified_storage_write(database):
    document = _agreed_document()
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "REQUEST_EXPORT",
            {"revision_id": document["current_revision"]["revision_id"], "format": "DOCX"},
        ),
    )
    job = BusinessDocumentJobQueue.claim("export-boundary-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]
    discarded = MemoryStorage(discard=True)
    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentExportService.generate(job, storage=discarded)
    assert caught.value.code == "EXPORT_STORAGE_WRITE_FAILED"
    assert discarded.removed
    assert BusinessDocumentExportArtifact.select().count() == 0

    storage = MemoryStorage()
    artifact = BusinessDocumentExportService.generate(job, storage=storage)
    row = BusinessDocumentExportArtifact.get_by_id(artifact["artifact_id"])
    assert storage.get(row.storage_bucket, row.storage_key).startswith(b"PK")


@pytest.mark.p0
@pytest.mark.parametrize("unsafe_url", ["javascript:alert(1)", "data:text/html;base64,PHNjcmlwdD4="])
def test_document_schema_rejects_unsafe_external_urls(database, unsafe_url):
    draft = _draft()
    draft["sections"][0]["blocks"] = [{"type": "reference", "label": "Unsafe", "url": unsafe_url}]
    with pytest.raises(BusinessDocumentError) as caught:
        validate_document_ast(draft)
    assert caught.value.code == "INVALID_DOCUMENT_DRAFT"


@pytest.mark.p0
def test_dead_operation_can_be_retried_and_draft_sources_are_snapshot_bound(database):
    class FailingAI:
        def process(self, _job):
            raise RuntimeError("permanent failure")

    document = _create()
    failed_request = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    BusinessDocumentJob.update(max_attempts=1).where(BusinessDocumentJob.id == failed_request["job_id"]).execute()
    assert BusinessDocumentWorker(worker_id="dead-worker", ai=FailingAI()).run_once() is True
    failed = BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)
    assert failed["operation_state"] == "FAILED"
    assert "REQUEST_INTAKE_ASSESSMENT" in failed["allowed_commands"]
    retried = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(failed, "REQUEST_INTAKE_ASSESSMENT", suffix="-retry"),
    )
    assert retried["operation_state"] == "ANALYZING"

    completed = _claim_complete(
        retried["job_id"],
        {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
    )
    draft_request = BusinessDocumentService.execute_command(TENANT, AUTHOR, completed["document_id"], _command(completed, "REQUEST_DRAFT"))
    job = BusinessDocumentJob.get_by_id(draft_request["job_id"])
    idea_event_id = job.payload["idea_source_event_id"]
    assert idea_event_id in {event["event_id"] for event in job.payload["source_events"]}
    proposal = {
        "target_section_id": "5.5",
        "text": "Добавить мониторинг",
        "rationale": "Следует из постановки",
        "source_event_ids": [idea_event_id],
    }
    reviewed = _claim_complete(
        job.id,
        {
            "draft": _draft(),
            "review_questions": {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
            "proposals": [proposal],
        },
    )
    assert idea_event_id in reviewed["protocol"]["proposals"][0]["source_event_ids"]

    other = _create(title="Другой проверяемый документ")
    assessment = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        other["document_id"],
        _command(other, "REQUEST_INTAKE_ASSESSMENT", suffix="-other"),
    )
    other = _claim_complete(
        assessment["job_id"],
        {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
    )
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        other["document_id"],
        _command(other, "REQUEST_DRAFT", suffix="-other"),
    )
    with pytest.raises(BusinessDocumentError) as unknown:
        _claim_complete(
            requested["job_id"],
            {
                "draft": _draft(),
                "review_questions": {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
                "proposals": [{**proposal, "source_event_ids": ["invented-event-id"]}],
            },
        )
    assert unknown.value.code == "SOURCE_NOT_IN_JOB_SNAPSHOT"

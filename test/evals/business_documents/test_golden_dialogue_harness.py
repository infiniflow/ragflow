"""Golden dialogue runner over the real business-document state machine."""

from __future__ import annotations

from collections import deque
from copy import deepcopy
from dataclasses import dataclass
import io
import json
from pathlib import Path
import sys
from types import ModuleType
from typing import Any
from unittest.mock import patch
import zipfile

import pytest
from peewee import SqliteDatabase


if "api.apps" not in sys.modules:
    api_apps = ModuleType("api.apps")
    api_apps.__path__ = [str(Path(__file__).resolve().parents[3] / "api" / "apps")]
    sys.modules["api.apps"] = api_apps

from api.apps.business_documents.ai import BusinessDocumentAI
from api.apps.business_documents.assets import published_template, render_section_text, section_hash
from api.apps.business_documents.contracts import CommandType
from api.apps.business_documents.evidence import BusinessDocumentEvidence
from api.apps.business_documents.errors import BusinessDocumentError
from api.apps.business_documents.exports import BusinessDocumentExportService
from api.apps.business_documents.service import BusinessDocumentService
from api.apps.business_documents.worker import BusinessDocumentWorker
from api.db.db_models import (
    BusinessDocument,
    BusinessDocumentAnswer,
    BusinessDocumentComment,
    BusinessDocumentEvent,
    BusinessDocumentEvidenceSnapshot,
    BusinessDocumentExportArtifact,
    BusinessDocumentJob,
    BusinessDocumentProposalDecision,
    BusinessDocumentRevision,
)
from test.unit_test.api.apps.business_documents.helpers import required_section_blocks


GOLDEN_PATH = Path(__file__).parents[3] / "agent" / "business_requirements" / "golden_dialogs" / "v2.json"
KNOWN_P1_GAPS: dict[str, str] = {}


def load_golden_cases(case_ids: set[str]) -> dict[str, dict[str, Any]]:
    suite = json.loads(GOLDEN_PATH.read_text(encoding="utf-8"))
    cases = {case["id"]: case for case in suite["cases"] if case["id"] in case_ids}
    missing = case_ids - cases.keys()
    if missing:
        raise AssertionError(f"Golden cases are missing: {sorted(missing)}")
    return cases


@dataclass(frozen=True)
class GoldenObservation:
    values: dict[str, Any]
    facts: frozenset[str]


class HardAssertionMatcher:
    @staticmethod
    def failures(assertions: list[str], observation: GoldenObservation) -> list[str]:
        failures: list[str] = []
        for assertion in assertions:
            if "=" not in assertion:
                if assertion not in observation.facts:
                    failures.append(f"missing fact: {assertion}")
                continue
            key, expected = assertion.split("=", 1)
            actual = observation.values.get(key)
            expected_values = expected.split("_or_")
            if str(actual) not in expected_values:
                failures.append(f"{key}: expected one of {expected_values}, got {actual!r}")
        return failures


class ScriptedAIAdapter:
    """Deterministic injected model; captured inputs support provenance checks."""

    def __init__(self, responses: dict[str, list[dict[str, Any]]]):
        self.responses = {job_type: deque(items) for job_type, items in responses.items()}
        self.requests: list[dict[str, Any]] = []

    def generate(self, tenant_id: str, system_prompt: str, input_payload: dict[str, Any]):
        assert tenant_id
        assert "недовер" in system_prompt.lower()
        self.requests.append(input_payload)
        job_type = input_payload["job_input"]["task_type"]
        if not self.responses.get(job_type):
            raise AssertionError(f"No scripted AI response for {job_type}")
        return self.responses[job_type].popleft()


class MemoryStorage:
    def __init__(self):
        self.objects: dict[tuple[str, str], bytes] = {}

    def put(self, bucket: str, key: str, content: bytes):
        self.objects[(bucket, key)] = content

    def get(self, bucket: str, key: str):
        return self.objects.get((bucket, key))

    def rm(self, bucket: str, key: str):
        self.objects.pop((bucket, key), None)


class EvidenceSearchAdapter:
    def __init__(self, chunks):
        self.chunks = chunks
        self.requests: list[dict[str, Any]] = []

    def search(self, actor_id: str, request: dict[str, Any]):
        assert actor_id
        self.requests.append(request)
        return True, {"chunks": self.chunks}


class GoldenDialogueRunner:
    def __init__(self, tenant_id="golden-tenant", actor_id="golden-author"):
        self.tenant_id = tenant_id
        self.actor_id = actor_id
        self.sequence = 0

    def run(self, case: dict[str, Any]) -> GoldenObservation:
        return getattr(self, f"_{case['id'].split('_', 1)[0].lower()}")(case)

    def _create(self, case, *, dataset_ids=None):
        return BusinessDocumentService.create_document(
            self.tenant_id,
            self.actor_id,
            {
                "schema_version": "1",
                "document_type": "business_requirements",
                "title": case["id"],
                "idea": "\n".join(turn["text"] for turn in case["turns"]),
                **({"dataset_ids": dataset_ids} if dataset_ids is not None else {}),
            },
        )

    def _command(self, document, command_type, payload=None, *, expected=None, key=None):
        self.sequence += 1
        return {
            "schema_version": "1",
            "command_id": f"golden-cmd-{self.sequence}",
            "idempotency_key": key or f"golden-idem-{self.sequence}",
            "expected_state_version": expected if expected is not None else document["state_version"],
            "type": command_type,
            "payload": payload or {},
        }

    def _work(self, adapter: ScriptedAIAdapter):
        worker = BusinessDocumentWorker(
            worker_id=f"golden-worker-{self.sequence}",
            ai=BusinessDocumentAI(adapter),
            retry_base_ms=0,
            lease_ms=60_000,
        )
        assert worker.run_once() is True

    @staticmethod
    def _complete_assessment():
        return {"schema_version": "1", "outcome": "COMPLETE", "questions": []}

    @staticmethod
    def _question_batch(stage="INTAKE", *, target_section_id="3.1", count=2):
        return {
            "schema_version": "1",
            "outcome": "NEEDS_INPUT",
            "questions": [
                {
                    "semantic_tag": f"golden-{stage.lower()}-{index}",
                    "stage": stage,
                    "target_section_id": target_section_id,
                    "text": f"Уточняющий вопрос {index}?",
                    "options": [
                        {"option_id": "yes", "label": "Да"},
                        {"option_id": "no", "label": "Нет"},
                    ],
                    "allow_custom_answer": True,
                }
                for index in range(1, count + 1)
            ],
        }

    @staticmethod
    def _draft():
        template = published_template()
        return {
            "schema_version": "1",
            "document_type": "business_requirements",
            "template_version": template["template_version"],
            "sections": [
                {
                    "id": section["id"],
                    "title": section["title"],
                    "blocks": required_section_blocks(section["id"], f"Проверяемое содержание раздела {section['id']}."),
                }
                for section in template["sections"]
            ],
        }

    def _to_review(self, case, *, review_questions=None, proposals=None, draft=None):
        document = self._create(case)
        created_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "DocumentCreated"))
        normalized_proposals = [
            {
                **proposal,
                "source_event_ids": [created_event.id if event_id == "$IDEA" else event_id for event_id in proposal["source_event_ids"]],
            }
            for proposal in (proposals or [])
        ]
        adapter = ScriptedAIAdapter({"ASSESS_INTAKE": [self._complete_assessment()]})
        BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            document["document_id"],
            self._command(document, "REQUEST_INTAKE_ASSESSMENT"),
        )
        self._work(adapter)
        document = BusinessDocumentService.get_document(self.tenant_id, document["document_id"], self.actor_id)
        draft_adapter = ScriptedAIAdapter(
            {
                "GENERATE_DRAFT": [
                    {
                        "draft": draft or self._draft(),
                        "review_questions": review_questions or self._complete_assessment(),
                        "proposals": normalized_proposals,
                    }
                ]
            }
        )
        BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            document["document_id"],
            self._command(document, "REQUEST_DRAFT"),
        )
        self._work(draft_adapter)
        return BusinessDocumentService.get_document(self.tenant_id, document["document_id"], self.actor_id)

    def _assess_review(self, document):
        dispositions = [{"comment_event_id": comment["source_event_id"], "disposition": "CONFIRMED_CHANGE"} for comment in document["protocol"]["comments"]]
        adapter = ScriptedAIAdapter(
            {
                "ASSESS_REVIEW": [
                    {
                        "schema_version": "1",
                        "questions": [],
                        "proposals": [],
                        "comment_dispositions": dispositions,
                    }
                ]
            }
        )
        BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            document["document_id"],
            self._command(document, "REQUEST_REVIEW_ASSESSMENT"),
        )
        self._work(adapter)
        return BusinessDocumentService.get_document(self.tenant_id, document["document_id"], self.actor_id)

    def _apply(self, document, operations_factory):
        requested = BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            document["document_id"],
            self._command(
                document,
                "APPLY_CHANGES",
                {"base_revision_id": document["current_revision"]["revision_id"]},
            ),
        )
        job = BusinessDocumentJob.get_by_id(requested["job_id"])
        operations = operations_factory(job) if callable(operations_factory) else operations_factory
        used_event_ids = {event_id for operation in operations for event_id in operation.get("source_event_ids", [])}
        active_event_ids = BusinessDocumentService._active_change_input_event_ids(  # noqa: SLF001
            BusinessDocument.get_by_id(document["document_id"])
        )
        adapter = ScriptedAIAdapter(
            {
                "PLAN_CHANGES": [
                    {
                        "schema_version": "1",
                        "base_revision_id": job.base_revision_id,
                        "source_state_version": job.source_state_version,
                        "acknowledged_no_change_event_ids": sorted(active_event_ids - used_event_ids),
                        "operations": operations,
                    }
                ]
            }
        )
        self._work(adapter)
        return BusinessDocumentService.get_document(self.tenant_id, document["document_id"], self.actor_id)

    def _agree_without_changes(self, document):
        return self._apply(self._assess_review(document), [])

    @staticmethod
    def _anchor(revision, section_id, selected_text):
        section = next(item for item in revision["document_ast"]["sections"] if item["id"] == section_id)
        section_text = render_section_text(section)
        start = section_text.index(selected_text)
        end = start + len(selected_text)
        return {
            "revision_id": revision["revision_id"],
            "section_id": section_id,
            "selected_text": selected_text,
            "prefix": section_text[max(0, start - 64) : start],
            "suffix": section_text[end : end + 64],
            "start_offset": start,
            "end_offset": end,
        }

    def _question_document(self, case):
        document = self._create(case)
        question_batch = {
            "schema_version": "1",
            "outcome": "NEEDS_INPUT",
            "questions": [
                {
                    "semantic_tag": "geography",
                    "stage": "INTAKE",
                    "target_section_id": "3.1",
                    "text": "Какая география?",
                    "options": [
                        {"option_id": "moscow", "label": "Москва"},
                        {"option_id": "russia", "label": "РФ"},
                        {"option_id": "pilot", "label": "Пилотный регион"},
                    ],
                    "allow_custom_answer": True,
                }
            ],
        }
        adapter = ScriptedAIAdapter({"ASSESS_INTAKE": [question_batch]})
        BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            document["document_id"],
            self._command(document, "REQUEST_INTAKE_ASSESSMENT"),
        )
        self._work(adapter)
        projection = BusinessDocumentService.get_document(self.tenant_id, document["document_id"], self.actor_id)
        return projection, projection["protocol"]["questions"][0]

    def _g01(self, case):
        document = self._create(case)
        adapter = ScriptedAIAdapter({"ASSESS_INTAKE": [self._question_batch()]})
        BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            document["document_id"],
            self._command(document, "REQUEST_INTAKE_ASSESSMENT"),
        )
        self._work(adapter)
        projection = BusinessDocumentService.get_document(self.tenant_id, document["document_id"], self.actor_id)
        questions = projection["protocol"]["questions"]
        facts = {
            "no_revision_created" if projection["current_revision"] is None else "",
            "questions_count_between_2_and_4" if 2 <= len(questions) <= 4 else "",
            "every_question_has_2_to_4_options" if all(2 <= len(item["options"]) <= 4 for item in questions) else "",
        }
        return GoldenObservation(values={"lifecycle_state": projection["lifecycle_state"]}, facts=frozenset(facts - {""}))

    def _g02(self, case):
        projection = self._to_review(case)
        revision = projection["current_revision"]
        section_ids = [section["id"] for section in revision["document_ast"]["sections"]]
        body = revision["body_markdown"]
        facts = {
            "template_section_order_exact" if section_ids == [item["id"] for item in published_template()["sections"]] else "",
            "section_5_5_not_empty" if "Проверяемое содержание раздела 5.5." in body else "",
            "protocol_not_embedded_in_body" if all(marker not in body for marker in ("Комментарии автора", "Предложения агента", "Вопросы агента")) else "",
        }
        return GoldenObservation(
            values={"lifecycle_state": projection["lifecycle_state"], "revision_number": revision["revision_number"]},
            facts=frozenset(facts - {""}),
        )

    def _g03(self, case):
        projection, question = self._question_document(case)
        before_events = BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == projection["document_id"]).count()
        response = BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            projection["document_id"],
            self._command(
                projection,
                "ANSWER_QUESTION",
                {"question_id": question["question_id"], "selected_option_id": "russia", "custom_answer": None},
            ),
        )
        after = BusinessDocumentService.get_document(self.tenant_id, projection["document_id"], self.actor_id)
        stored = BusinessDocumentAnswer.get(BusinessDocumentAnswer.question_id == question["question_id"])
        answer_event = BusinessDocumentEvent.get_by_id(response["event_id"])
        facts = {
            "answer_resolves_option_2" if stored.selected_option_id == question["options"][1]["option_id"] else "",
            "answer_event_is_append_only"
            if answer_event.event_type == "QuestionAnswered" and BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == projection["document_id"]).count() == before_events + 1
            else "",
            "question_is_closed" if after["protocol"]["questions"][0]["status"] == "ANSWERED" else "",
        }
        return GoldenObservation(values={}, facts=frozenset(facts - {""}))

    def _g04(self, case):
        projection, question = self._question_document(case)
        custom_answer = case["turns"][-1]["text"].split(":", 1)[-1].strip()
        BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            projection["document_id"],
            self._command(
                projection,
                "ANSWER_QUESTION",
                {"question_id": question["question_id"], "selected_option_id": None, "custom_answer": custom_answer},
            ),
        )
        after = BusinessDocumentService.get_document(self.tenant_id, projection["document_id"], self.actor_id)
        stored = BusinessDocumentAnswer.get(BusinessDocumentAnswer.question_id == question["question_id"])
        facts = {
            "custom_answer_preserved_verbatim" if stored.custom_answer == custom_answer else "",
            "question_is_closed" if after["protocol"]["questions"][0]["status"] == "ANSWERED" else "",
        }
        return GoldenObservation(values={}, facts=frozenset(facts - {""}))

    @staticmethod
    def _proposal():
        return {
            "target_section_id": "5.5",
            "text": "Добавить метрику доли отмененных записей",
            "rationale": "Метрика нужна для контроля результата",
            "source_event_ids": ["$IDEA"],
        }

    def _g05(self, case):
        projection = self._to_review(case, proposals=[self._proposal()])
        proposal = projection["protocol"]["proposals"][0]
        original_revision = projection["current_revision"]
        decision = BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            projection["document_id"],
            self._command(
                projection,
                "DECIDE_PROPOSAL",
                {"proposal_id": proposal["proposal_id"], "decision": "ACCEPTED"},
            ),
        )
        projection = BusinessDocumentService.get_document(self.tenant_id, projection["document_id"], self.actor_id)
        projection = self._assess_review(projection)
        base_section = next(section for section in projection["current_revision"]["document_ast"]["sections"] if section["id"] == "5.5")
        applied = self._apply(
            projection,
            [
                {
                    "operation_id": "apply-accepted-proposal",
                    "type": "REPLACE_SECTION_CONTENT",
                    "section_id": "5.5",
                    "expected_section_hash": section_hash(base_section),
                    "source_event_ids": [decision["event_id"]],
                    "content": {"blocks": [{"type": "paragraph", "text": proposal["text"]}]},
                }
            ],
        )
        decision_row = BusinessDocumentProposalDecision.get(BusinessDocumentProposalDecision.proposal_id == proposal["proposal_id"])
        applied_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == projection["document_id"]) & (BusinessDocumentEvent.event_type == "ChangesApplied"))
        facts = {
            "new_revision_created" if applied["current_revision"]["revision_number"] == original_revision["revision_number"] + 1 else "",
            "accepted_proposal_is_applied" if proposal["text"] in applied["current_revision"]["body_markdown"] else "",
            "source_events_are_traceable" if decision["event_id"] in applied_event.payload["source_event_ids"] and decision["event_id"] in applied["current_revision"]["source_event_ids"] else "",
        }
        return GoldenObservation(
            values={"proposal_decision": decision_row.decision},
            facts=frozenset(facts - {""}),
        )

    def _g06(self, case):
        projection = self._to_review(case, proposals=[self._proposal()])
        proposal = projection["protocol"]["proposals"][0]
        original = deepcopy(projection["current_revision"])
        BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            projection["document_id"],
            self._command(
                projection,
                "DECIDE_PROPOSAL",
                {"proposal_id": proposal["proposal_id"], "decision": "REJECTED"},
            ),
        )
        projection = BusinessDocumentService.get_document(self.tenant_id, projection["document_id"], self.actor_id)
        agreed = self._agree_without_changes(projection)
        decision = BusinessDocumentProposalDecision.get(BusinessDocumentProposalDecision.proposal_id == proposal["proposal_id"])
        return GoldenObservation(
            values={"proposal_decision": decision.decision},
            facts=frozenset(
                {"rejected_proposal_not_applied"}
                if agreed["current_revision"]["content_hash"] == original["content_hash"] and proposal["text"] not in agreed["current_revision"]["body_markdown"]
                else set()
            ),
        )

    def _g07(self, case):
        accepted_proposal = self._proposal()
        pending_proposal = {
            **self._proposal(),
            "text": "Добавить SLA ответа CRM",
            "rationale": "SLA требует отдельного решения автора",
        }
        projection = self._to_review(case, proposals=[accepted_proposal, pending_proposal])
        review_cycle = projection["active_review_cycle"]
        proposals = {proposal["text"]: proposal for proposal in projection["protocol"]["proposals"]}
        original = deepcopy(projection["current_revision"])
        decision = BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            projection["document_id"],
            self._command(
                projection,
                "DECIDE_PROPOSAL",
                {
                    "proposal_id": proposals[accepted_proposal["text"]]["proposal_id"],
                    "decision": "ACCEPTED",
                },
            ),
        )
        projection = BusinessDocumentService.get_document(self.tenant_id, projection["document_id"], self.actor_id)
        projection = self._assess_review(projection)
        base_section = next(section for section in projection["current_revision"]["document_ast"]["sections"] if section["id"] == "5.5")
        partially_applied = self._apply(
            projection,
            [
                {
                    "operation_id": "apply-one-of-two-proposals",
                    "type": "REPLACE_SECTION_CONTENT",
                    "section_id": "5.5",
                    "expected_section_hash": section_hash(base_section),
                    "source_event_ids": [decision["event_id"]],
                    "content": {
                        "blocks": [
                            {
                                "type": "paragraph",
                                "text": accepted_proposal["text"],
                            }
                        ]
                    },
                }
            ],
        )
        remaining = next(proposal for proposal in partially_applied["protocol"]["proposals"] if proposal["proposal_id"] == proposals[pending_proposal["text"]]["proposal_id"])
        facts = {
            "proposal_decision_is_empty" if remaining["decision"] == "PENDING" else "",
            "unanswered_proposal_not_applied" if pending_proposal["text"] not in partially_applied["current_revision"]["body_markdown"] else "",
            "accepted_proposal_is_applied" if accepted_proposal["text"] in partially_applied["current_revision"]["body_markdown"] else "",
            "new_revision_created" if partially_applied["current_revision"]["revision_number"] == original["revision_number"] + 1 else "",
            "proposal_decision_available" if "DECIDE_PROPOSAL" in partially_applied["allowed_commands"] else "",
            "review_cycle_unchanged" if partially_applied["active_review_cycle"] == review_cycle else "",
        }
        return GoldenObservation(
            values={"lifecycle_state": partially_applied["lifecycle_state"]},
            facts=frozenset(facts - {""}),
        )

    def _g08(self, case):
        projection = self._to_review(case, review_questions=self._question_batch("REVIEW", count=1))
        revision = projection["current_revision"]
        before_count = BusinessDocumentRevision.select().where(BusinessDocumentRevision.document_id == projection["document_id"]).count()
        error_code = None
        try:
            BusinessDocumentService.execute_command(
                self.tenant_id,
                self.actor_id,
                projection["document_id"],
                self._command(projection, "APPLY_CHANGES", {"base_revision_id": revision["revision_id"]}),
            )
        except BusinessDocumentError as error:
            error_code = error.code
        after = BusinessDocumentService.get_document(self.tenant_id, projection["document_id"], self.actor_id)
        after_count = BusinessDocumentRevision.select().where(BusinessDocumentRevision.document_id == projection["document_id"]).count()
        facts = {
            "revision_unchanged" if before_count == after_count else "",
            "body_hash_unchanged" if after["current_revision"]["content_hash"] == revision["content_hash"] else "",
        }
        return GoldenObservation(values={"command_rejected": error_code}, facts=frozenset(facts - {""}))

    def _g09(self, case):
        projection = self._to_review(case)
        original_hash = projection["current_revision"]["content_hash"]
        direct_edit_available = "DIRECT_EDIT" in {command.value for command in CommandType}
        response = BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            projection["document_id"],
            self._command(
                projection,
                "ADD_COMMENT",
                {
                    "revision_id": projection["current_revision"]["revision_id"],
                    "section_id": None,
                    "text": case["turns"][0]["text"],
                    "anchor": None,
                },
            ),
        )
        after = BusinessDocumentService.get_document(self.tenant_id, projection["document_id"], self.actor_id)
        comment = BusinessDocumentComment.get(BusinessDocumentComment.document_id == projection["document_id"])
        event = BusinessDocumentEvent.get_by_id(response["event_id"])
        facts = {
            "direct_edit_not_available" if not direct_edit_available else "",
            "author_comment_created_or_clarification_requested" if comment.text == case["turns"][0]["text"] and event.event_type == "AuthorCommentAdded" else "",
            "body_hash_unchanged" if after["current_revision"]["content_hash"] == original_hash else "",
        }
        return GoldenObservation(values={}, facts=frozenset(facts - {""}))

    def _g10(self, case):
        projection = self._to_review(case)
        revision = deepcopy(projection["current_revision"])
        selected_text = "Проверяемое содержание раздела 3.1."
        BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            projection["document_id"],
            self._command(
                projection,
                "ADD_COMMENT",
                {
                    "revision_id": revision["revision_id"],
                    "section_id": "3.1",
                    "text": case["turns"][0]["text"],
                    "anchor": self._anchor(revision, "3.1", selected_text),
                },
            ),
        )
        projection = BusinessDocumentService.get_document(self.tenant_id, projection["document_id"], self.actor_id)
        adapter = ScriptedAIAdapter(
            {
                "ASSESS_REVIEW": [
                    {
                        "schema_version": "1",
                        "questions": [
                            {
                                "semantic_tag": "ambiguous-anchor",
                                "target_section_id": "3.1",
                                "text": "Как именно переформулировать выделенный фрагмент?",
                                "options": [
                                    {"option_id": "short", "label": "Сократить"},
                                    {"option_id": "explain", "label": "Пояснить"},
                                ],
                                "allow_custom_answer": True,
                            }
                        ],
                        "proposals": [],
                        "comment_dispositions": [
                            {
                                "comment_event_id": projection["protocol"]["comments"][0]["source_event_id"],
                                "disposition": "NEEDS_QUESTION",
                                "question_semantic_tag": "ambiguous-anchor",
                            }
                        ],
                    }
                ]
            }
        )
        BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            projection["document_id"],
            self._command(projection, "REQUEST_REVIEW_ASSESSMENT"),
        )
        self._work(adapter)
        after = BusinessDocumentService.get_document(self.tenant_id, projection["document_id"], self.actor_id)
        comment = after["protocol"]["comments"][0]
        question = after["protocol"]["questions"][0]
        facts = {
            "anchored_comment_preserved" if comment["anchor"] == self._anchor(revision, "3.1", selected_text) else "",
            "clarifying_question_created" if question["status"] == "OPEN" else "",
            "question_has_2_to_4_options" if 2 <= len(question["options"]) <= 4 else "",
            "body_hash_unchanged" if after["current_revision"]["content_hash"] == revision["content_hash"] else "",
        }
        return GoldenObservation(values={}, facts=frozenset(facts - {""}))

    def _g11(self, case):
        projection = self._to_review(case)
        original_revision = deepcopy(projection["current_revision"])
        selected_text = "Проверяемое содержание раздела 3.1."
        comment_response = BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            projection["document_id"],
            self._command(
                projection,
                "ADD_COMMENT",
                {
                    "revision_id": original_revision["revision_id"],
                    "section_id": "3.1",
                    "text": case["turns"][0]["text"],
                    "anchor": self._anchor(original_revision, "3.1", selected_text),
                },
            ),
        )
        projection = BusinessDocumentService.get_document(self.tenant_id, projection["document_id"], self.actor_id)
        projection = self._assess_review(projection)
        base_section = next(section for section in projection["current_revision"]["document_ast"]["sections"] if section["id"] == "3.1")
        changed = self._apply(
            projection,
            [
                {
                    "operation_id": "replace-anchored-fragment",
                    "type": "REPLACE_SECTION_CONTENT",
                    "section_id": "3.1",
                    "expected_section_hash": section_hash(base_section),
                    "source_event_ids": [comment_response["event_id"]],
                    "content": {
                        "blocks": [
                            {
                                "type": "paragraph",
                                "text": "Новая формулировка без прежнего фрагмента.",
                            }
                        ]
                    },
                }
            ],
        )
        comment = changed["protocol"]["comments"][0]
        facts = {
            "comment_preserved" if comment["text"] == case["turns"][0]["text"] and comment["anchor"]["selected_text"] == selected_text else "",
            "silent_reanchor_forbidden"
            if comment["revision_id"] == original_revision["revision_id"]
            and comment["revision_id"] != changed["current_revision"]["revision_id"]
            and selected_text not in changed["current_revision"]["body_markdown"]
            else "",
        }
        return GoldenObservation(
            values={"anchor_status": comment["anchor_status"]},
            facts=frozenset(facts - {""}),
        )

    def _g12(self, case):
        document = self._create(case)
        intake_adapter = ScriptedAIAdapter({"ASSESS_INTAKE": [self._complete_assessment()]})
        BusinessDocumentService.execute_command(self.tenant_id, self.actor_id, document["document_id"], self._command(document, "REQUEST_INTAKE_ASSESSMENT"))
        self._work(intake_adapter)
        document = BusinessDocumentService.get_document(self.tenant_id, document["document_id"], self.actor_id)
        command = self._command(document, "REQUEST_DRAFT", key="golden-duplicate")
        first = BusinessDocumentService.execute_command(self.tenant_id, self.actor_id, document["document_id"], command)
        second = BusinessDocumentService.execute_command(self.tenant_id, self.actor_id, document["document_id"], command)
        draft_adapter = ScriptedAIAdapter({"GENERATE_DRAFT": [{"draft": self._draft(), "review_questions": self._complete_assessment(), "proposals": []}]})
        self._work(draft_adapter)
        comparable_second = {key: value for key, value in second.items() if key != "idempotent_replay"}
        requested_events = BusinessDocumentEvent.select().where(
            (BusinessDocumentEvent.document_id == document["document_id"])
            & (BusinessDocumentEvent.event_type == "BusinessDocumentJobRequested")
            & (BusinessDocumentEvent.correlation_id == command["command_id"])
        )
        revision_count = BusinessDocumentRevision.select().where(BusinessDocumentRevision.document_id == document["document_id"]).count()
        facts = {
            "same_response_returned" if first == comparable_second and second["idempotent_replay"] is True else "",
            "single_event_appended" if requested_events.count() == 1 else "",
            "single_revision_created" if revision_count == 1 else "",
        }
        return GoldenObservation(values={}, facts=frozenset(facts - {""}))

    def _g13(self, case):
        document = self._create(case)
        BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            document["document_id"],
            self._command(document, "ARCHIVE"),
        )
        before = BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == document["document_id"]).count()
        error_code = status = None
        try:
            BusinessDocumentService.execute_command(
                self.tenant_id,
                self.actor_id,
                document["document_id"],
                self._command(document, "ARCHIVE", expected=document["state_version"], key="golden-stale"),
            )
        except BusinessDocumentError as error:
            error_code, status = error.code, error.status
        after = BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == document["document_id"]).count()
        return GoldenObservation(
            values={"http_status": status, "error_code": error_code},
            facts=frozenset({"no_events_appended"} if before == after else set()),
        )

    def _g14(self, case):
        projection, question = self._question_document(case)
        first = BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            projection["document_id"],
            self._command(
                projection,
                "ANSWER_QUESTION",
                {"question_id": question["question_id"], "selected_option_id": "moscow", "custom_answer": None},
            ),
        )
        projection = BusinessDocumentService.get_document(self.tenant_id, projection["document_id"], self.actor_id)
        original = BusinessDocumentAnswer.get(BusinessDocumentAnswer.question_id == question["question_id"])
        error_code = None
        try:
            BusinessDocumentService.execute_command(
                self.tenant_id,
                self.actor_id,
                projection["document_id"],
                self._command(
                    projection,
                    "ANSWER_QUESTION",
                    {"question_id": question["question_id"], "selected_option_id": "russia", "custom_answer": None},
                ),
            )
        except BusinessDocumentError as error:
            error_code = error.code
        stored = BusinessDocumentAnswer.get(BusinessDocumentAnswer.question_id == question["question_id"])
        return GoldenObservation(
            values={"command_rejected": error_code},
            facts=frozenset(
                {"original_answer_unchanged"}
                if stored.id == original.id and stored.selected_option_id == "moscow" and BusinessDocumentEvent.get_by_id(first["event_id"]).event_type == "QuestionAnswered"
                else set()
            ),
        )

    def _invalid_question_observation(self, case, option_count):
        document = self._create(case)
        invalid = self._question_batch(count=1)
        invalid["questions"][0]["options"] = [{"option_id": f"option-{index}", "label": f"Вариант {index}"} for index in range(1, option_count + 1)]
        adapter = ScriptedAIAdapter({"ASSESS_INTAKE": [invalid]})
        requested = BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            document["document_id"],
            self._command(document, "REQUEST_INTAKE_ASSESSMENT"),
        )
        self._work(adapter)
        projection = BusinessDocumentService.get_document(self.tenant_id, document["document_id"], self.actor_id)
        job = BusinessDocumentJob.get_by_id(requested["job_id"])
        facts = {
            "structured_output_rejected" if job.status == "RETRY" and job.error["code"] == "INVALID_QUESTION_BATCH" else "",
            "no_question_published" if projection["protocol"]["questions"] == [] else "",
            "no_invalid_question_published" if projection["protocol"]["questions"] == [] else "",
            "question_must_be_split" if option_count > 4 and job.error["code"] == "INVALID_QUESTION_BATCH" else "",
        }
        return GoldenObservation(
            values={"operation_state": "retry" if job.status == "RETRY" else projection["operation_state"]},
            facts=frozenset(facts - {""}),
        )

    def _g15(self, case):
        return self._invalid_question_observation(case, 1)

    def _g16(self, case):
        return self._invalid_question_observation(case, 5)

    def _g17(self, case):
        projection = self._to_review(case, review_questions=self._question_batch("REVIEW", target_section_id="5.5", count=1))
        question = projection["protocol"]["questions"][0]
        error_code = None
        try:
            BusinessDocumentService.execute_command(
                self.tenant_id,
                self.actor_id,
                projection["document_id"],
                self._command(projection, "APPLY_CHANGES", {"base_revision_id": projection["current_revision"]["revision_id"]}),
            )
        except BusinessDocumentError as error:
            error_code = error.code
        return GoldenObservation(
            values={"lifecycle_state": projection["lifecycle_state"], "question_targets_section": question["target_section_id"]},
            facts=frozenset({"agreed_transition_forbidden"} if error_code else set()),
        )

    def _g18(self, case):
        draft = deepcopy(self._draft())
        section_two = next(section for section in draft["sections"] if section["id"] == "2")
        section_two["blocks"] = [{"type": "paragraph", "text": "НПА для продукта не применяются."}]
        projection = self._to_review(case, draft=draft)
        rendered = projection["current_revision"]["body_markdown"]
        section_ids = [section["id"] for section in projection["current_revision"]["document_ast"]["sections"]]
        facts = {
            "section_2_present" if "2" in section_ids else "",
            "section_2_explicitly_not_applicable" if "НПА для продукта не применяются." in rendered else "",
            "no_fake_regulation_generated" if all(marker not in rendered for marker in ("Федеральный закон", "Постановление", "Приказ №")) else "",
        }
        return GoldenObservation(values={}, facts=frozenset(facts - {""}))

    def _g19(self, case):
        projection = self._to_review(case)
        section_ids = [section["id"] for section in projection["current_revision"]["document_ast"]["sections"]]
        template_ids = [section["id"] for section in published_template()["sections"]]
        facts = {
            "section_5_1_absent" if "5.1" not in section_ids else "",
            "section_5_2_follows_section_5" if section_ids.index("5.2") == section_ids.index("5") + 1 else "",
            "no_extra_sections" if section_ids == template_ids else "",
        }
        return GoldenObservation(values={}, facts=frozenset(facts - {""}))

    def _g20(self, case):
        dataset_id = "dataset-injection"
        with patch("api.apps.business_documents.service.ensure_dataset_access", return_value=None):
            document = self._create(case, dataset_ids=[dataset_id])
        adapter = ScriptedAIAdapter({"ASSESS_INTAKE": [self._question_batch(count=2)]})
        requested = BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            document["document_id"],
            self._command(document, "REQUEST_INTAKE_ASSESSMENT"),
        )
        injection = case["turns"][0]["text"]
        search = EvidenceSearchAdapter(
            [
                {
                    "dataset_id": dataset_id,
                    "document_id": "uploaded-document",
                    "chunk_id": "injection-chunk",
                    "content": injection,
                    "similarity": 0.99,
                }
            ]
        )
        evidence = BusinessDocumentEvidence(
            search_adapter=search,
            access_checker=lambda candidate, actor: candidate == dataset_id and actor == self.actor_id,
        )
        worker = BusinessDocumentWorker(
            worker_id="golden-evidence-injection",
            ai=BusinessDocumentAI(adapter),
            evidence=evidence,
            retry_base_ms=0,
            lease_ms=60_000,
        )
        assert worker.run_once() is True
        projection = BusinessDocumentService.get_document(self.tenant_id, document["document_id"], self.actor_id)
        model_input = adapter.requests[0]
        evidence_input = model_input["evidence"]
        snapshot = BusinessDocumentEvidenceSnapshot.get(BusinessDocumentEvidenceSnapshot.job_id == requested["job_id"])
        completed_job = BusinessDocumentJob.get_by_id(requested["job_id"])
        source_ref = evidence_input["chunks"][0]["source_ref"]
        facts = {
            "instruction_not_executed" if projection["lifecycle_state"] == "INTAKE" and projection["current_revision"] is None else "",
            "evidence_provenance_preserved"
            if evidence_input["chunks"][0]["content"] == injection
            and snapshot.evidence_hash == evidence_input["evidence_hash"]
            and completed_job.result["execution"]["retrieval"]["source_refs"] == [source_ref]
            else "",
            "lifecycle_transition_unchanged" if projection["lifecycle_state"] == "INTAKE" else "",
        }
        return GoldenObservation(values={}, facts=frozenset(facts - {""}))

    def _g21(self, case):
        dataset_id = "dataset-conflict"
        with patch("api.apps.business_documents.service.ensure_dataset_access", return_value=None):
            document = self._create(case, dataset_ids=[dataset_id])
        adapter = ScriptedAIAdapter(
            {
                "ASSESS_INTAKE": [
                    {
                        "schema_version": "1",
                        "outcome": "NEEDS_INPUT",
                        "questions": [
                            {
                                "semantic_tag": "conflicting-sla",
                                "stage": "INTAKE",
                                "target_section_id": "5.5",
                                "text": "Какое SLA зафиксировать при конфликте источников?",
                                "options": [
                                    {"option_id": "one-second", "label": "1 секунда"},
                                    {"option_id": "three-seconds", "label": "3 секунды"},
                                ],
                                "allow_custom_answer": True,
                            }
                        ],
                    }
                ]
            }
        )
        requested = BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            document["document_id"],
            self._command(document, "REQUEST_INTAKE_ASSESSMENT"),
        )
        search = EvidenceSearchAdapter(
            [
                {
                    "dataset_id": dataset_id,
                    "document_id": "source-a",
                    "chunk_id": "sla-a",
                    "content": "Источник A: SLA 1 сек.",
                    "similarity": 0.95,
                },
                {
                    "dataset_id": dataset_id,
                    "document_id": "source-b",
                    "chunk_id": "sla-b",
                    "content": "Источник B: SLA 3 сек.",
                    "similarity": 0.94,
                },
            ]
        )
        evidence = BusinessDocumentEvidence(
            search_adapter=search,
            access_checker=lambda candidate, actor: candidate == dataset_id and actor == self.actor_id,
        )
        worker = BusinessDocumentWorker(
            worker_id="golden-evidence-conflict",
            ai=BusinessDocumentAI(adapter),
            evidence=evidence,
            retry_base_ms=0,
            lease_ms=60_000,
        )
        assert worker.run_once() is True
        projection = BusinessDocumentService.get_document(self.tenant_id, document["document_id"], self.actor_id)
        evidence_input = adapter.requests[0]["evidence"]
        source_refs = [chunk["source_ref"] for chunk in evidence_input["chunks"]]
        completed_job = BusinessDocumentJob.get_by_id(requested["job_id"])
        question = projection["protocol"]["questions"][0]
        facts = {
            "conflict_not_silently_resolved" if projection["current_revision"] is None and question["status"] == "OPEN" else "",
            "clarifying_question_created" if len(question["options"]) == 2 else "",
            "both_sources_traceable" if len(source_refs) == 2 and len(set(source_refs)) == 2 and completed_job.result["execution"]["retrieval"]["source_refs"] == source_refs else "",
        }
        return GoldenObservation(values={}, facts=frozenset(facts - {""}))

    def _g22(self, case):
        projection = self._to_review(case)
        protocol_text = "PROTOCOL_ONLY_DO_NOT_EXPORT"
        comment_response = BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            projection["document_id"],
            self._command(
                projection,
                "ADD_COMMENT",
                {
                    "revision_id": projection["current_revision"]["revision_id"],
                    "section_id": None,
                    "text": protocol_text,
                    "anchor": None,
                },
            ),
        )
        projection = BusinessDocumentService.get_document(self.tenant_id, projection["document_id"], self.actor_id)
        projection = self._assess_review(projection)
        base_section = next(section for section in projection["current_revision"]["document_ast"]["sections"] if section["id"] == "5.5")
        agreed = self._apply(
            projection,
            [
                {
                    "operation_id": "apply-monitoring-comment",
                    "type": "REPLACE_SECTION_CONTENT",
                    "section_id": "5.5",
                    "expected_section_hash": section_hash(base_section),
                    "source_event_ids": [comment_response["event_id"]],
                    "content": {"blocks": [{"type": "paragraph", "text": "Добавить измеримые метрики мониторинга."}]},
                }
            ],
        )
        revision = deepcopy(agreed["current_revision"])
        revision_count = BusinessDocumentRevision.select().where(BusinessDocumentRevision.document_id == agreed["document_id"]).count()
        storage = MemoryStorage()
        downloaded: dict[str, bytes] = {}
        for export_format in ("MARKDOWN", "DOCX"):
            requested = BusinessDocumentService.execute_command(
                self.tenant_id,
                self.actor_id,
                agreed["document_id"],
                self._command(
                    agreed,
                    "REQUEST_EXPORT",
                    {"revision_id": revision["revision_id"], "format": export_format},
                ),
            )
            worker = BusinessDocumentWorker(
                worker_id=f"golden-export-{export_format.lower()}",
                export_service=BusinessDocumentExportService,
                storage=storage,
                lease_ms=60_000,
            )
            assert worker.run_once() is True
            assert BusinessDocumentJob.get_by_id(requested["job_id"]).status == "COMPLETED"
            agreed = BusinessDocumentService.get_document(self.tenant_id, agreed["document_id"], self.actor_id)

        artifacts = BusinessDocumentExportService.list_artifacts(self.tenant_id, self.actor_id, agreed["document_id"])
        for artifact in artifacts:
            _, downloaded[artifact["format"]] = BusinessDocumentExportService.download(
                self.tenant_id,
                self.actor_id,
                agreed["document_id"],
                artifact["artifact_id"],
                storage=storage,
            )
        markdown_text = downloaded["MARKDOWN"].decode("utf-8")
        with zipfile.ZipFile(io.BytesIO(downloaded["DOCX"])) as archive:
            docx_text = archive.read("word/document.xml").decode("utf-8")
        protocol_still_present = any(comment["text"] == protocol_text for comment in agreed["protocol"]["comments"])
        after_revision_count = BusinessDocumentRevision.select().where(BusinessDocumentRevision.document_id == agreed["document_id"]).count()
        facts = {
            "exports_reference_same_revision" if {artifact["revision_id"] for artifact in artifacts} == {revision["revision_id"]} else "",
            "protocol_excluded" if protocol_still_present and protocol_text not in markdown_text and protocol_text not in docx_text else "",
            "export_does_not_mutate_revision"
            if agreed["current_revision"]["content_hash"] == revision["content_hash"]
            and agreed["current_revision"]["revision_id"] == revision["revision_id"]
            and after_revision_count == revision_count
            else "",
        }
        return GoldenObservation(
            values={"formats_include": "md,docx" if {artifact["format"] for artifact in artifacts} == {"MARKDOWN", "DOCX"} else ""},
            facts=frozenset(facts - {""}),
        )

    def _g23(self, case):
        projection = self._to_review(case)
        error_code = None
        try:
            BusinessDocumentService.execute_command(
                self.tenant_id,
                self.actor_id,
                projection["document_id"],
                self._command(
                    projection,
                    "REQUEST_EXPORT",
                    {"revision_id": projection["current_revision"]["revision_id"], "format": "EVA_WIKI"},
                ),
            )
        except BusinessDocumentError as error:
            error_code = error.code
        artifacts = BusinessDocumentExportArtifact.select().where(BusinessDocumentExportArtifact.document_id == projection["document_id"]).count()
        return GoldenObservation(
            values={"command_rejected": error_code},
            facts=frozenset({"no_artifact_created"} if artifacts == 0 else set()),
        )

    def _g24(self, case):
        agreed = self._agree_without_changes(self._to_review(case))
        document_id = agreed["document_id"]
        chat_id = agreed["chat_id"]
        before_events = [event.id for event in BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == document_id).order_by(BusinessDocumentEvent.sequence.asc())]
        resumed = BusinessDocumentService.get_document(self.tenant_id, document_id, self.actor_id)
        BusinessDocumentService.execute_command(
            self.tenant_id,
            self.actor_id,
            document_id,
            self._command(resumed, "START_REVIEW"),
        )
        reviewed = BusinessDocumentService.get_document(self.tenant_id, document_id, self.actor_id)
        after_events = [event.id for event in BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == document_id).order_by(BusinessDocumentEvent.sequence.asc())]
        new_document = self._create({**case, "id": f"{case['id']}-new-chat"})
        facts = {
            "same_chat_keeps_document_id" if resumed["document_id"] == document_id and resumed["chat_id"] == chat_id else "",
            "new_review_cycle_is_append_only"
            if reviewed["active_review_cycle"] == agreed["active_review_cycle"] + 1
            and after_events[: len(before_events)] == before_events
            and BusinessDocumentEvent.get_by_id(after_events[-1]).event_type == "ReviewCycleStarted"
            else "",
            "new_chat_requires_new_document_id" if new_document["document_id"] != document_id and new_document["chat_id"] != chat_id else "",
        }
        return GoldenObservation(values={}, facts=frozenset(facts - {""}))


@dataclass(frozen=True)
class ReleaseGateReport:
    case_failures: dict[str, list[str]]
    assertion_counts: dict[str, tuple[int, int]]
    priorities: dict[str, str]
    gaps: dict[str, str]

    @property
    def p0_case_rate(self):
        case_ids = [case_id for case_id, priority in self.priorities.items() if priority == "P0"]
        return sum(not self.case_failures[case_id] for case_id in case_ids) / len(case_ids)

    @property
    def p0_assertion_rate(self):
        counts = [self.assertion_counts[case_id] for case_id, priority in self.priorities.items() if priority == "P0"]
        return sum(passed for passed, _ in counts) / sum(total for _, total in counts)

    @property
    def all_case_rate(self):
        return sum(not failures for failures in self.case_failures.values()) / len(self.case_failures)

    @property
    def all_assertion_rate(self):
        return sum(passed for passed, _ in self.assertion_counts.values()) / sum(total for _, total in self.assertion_counts.values())

    @property
    def p1_case_rate(self):
        case_ids = [case_id for case_id, priority in self.priorities.items() if priority == "P1"]
        return sum(not self.case_failures[case_id] for case_id in case_ids) / len(case_ids)


def _run_isolated(case):
    database = SqliteDatabase(":memory:")
    tables = BusinessDocumentService.model_tables()
    with database.bind_ctx(tables, bind_refs=False, bind_backrefs=False):
        database.connect()
        database.create_tables(tables)
        try:
            return GoldenDialogueRunner().run(case)
        finally:
            database.drop_tables(tables)
            database.close()


def _release_gate_report():
    suite = json.loads(GOLDEN_PATH.read_text(encoding="utf-8"))
    cases = {case["id"]: case for case in suite["cases"]}
    failures: dict[str, list[str]] = {}
    assertion_counts: dict[str, tuple[int, int]] = {}
    for case_id, case in cases.items():
        if case_id in KNOWN_P1_GAPS:
            failures[case_id] = [f"automation gap: {KNOWN_P1_GAPS[case_id]}"]
            assertion_counts[case_id] = (0, len(case["assertions"]))
            continue
        try:
            observation = _run_isolated(case)
            case_failures = HardAssertionMatcher.failures(case["assertions"], observation)
            failures[case_id] = case_failures
            assertion_counts[case_id] = (len(case["assertions"]) - len(case_failures), len(case["assertions"]))
        except Exception as error:
            failures[case_id] = [f"runner error: {type(error).__name__}: {error}"]
            assertion_counts[case_id] = (0, len(case["assertions"]))
    return ReleaseGateReport(
        case_failures=failures,
        assertion_counts=assertion_counts,
        priorities={case_id: case["priority"] for case_id, case in cases.items()},
        gaps=KNOWN_P1_GAPS,
    )


@pytest.mark.p0
def test_release_gate_executes_every_p0_assertion_and_reports_all_case_rate():
    report = _release_gate_report()
    p0_ids = {case_id for case_id, priority in report.priorities.items() if priority == "P0"}
    p1_ids = {case_id for case_id, priority in report.priorities.items() if priority == "P1"}

    assert len(report.priorities) == 24
    assert len(p0_ids) == 19
    assert len(p1_ids) == 5
    assert not (p0_ids & report.gaps.keys())
    assert report.gaps == {}
    assert report.p0_case_rate == 1.0, report.case_failures
    assert report.p0_assertion_rate == 1.0, report.case_failures
    assert report.p1_case_rate == 1.0, report.case_failures
    assert report.all_case_rate == 1.0, report.case_failures
    assert report.all_assertion_rate == 1.0, report.case_failures


def test_matcher_reports_missing_facts_and_value_mismatches():
    observation = GoldenObservation(values={"http_status": 200}, facts=frozenset())
    assert HardAssertionMatcher.failures(["http_status=409", "error_code=STATE_VERSION_CONFLICT", "no_events_appended"], observation) == [
        "http_status: expected one of ['409'], got 200",
        "error_code: expected one of ['STATE_VERSION_CONFLICT'], got None",
        "missing fact: no_events_appended",
    ]

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
import io
from datetime import UTC, datetime as DateTime
from pathlib import Path
import sys
import threading
from types import ModuleType
import zipfile

import pytest
from peewee import SqliteDatabase


if "api.apps" not in sys.modules:
    api_apps = ModuleType("api.apps")
    api_apps.__path__ = [str(Path(__file__).resolve().parents[5] / "api" / "apps")]
    sys.modules["api.apps"] = api_apps

from api.apps.business_documents import exports as exports_module
from api.apps.business_documents.assets import published_template, render_document_ast
from api.apps.business_documents.errors import BusinessDocumentError
from api.apps.business_documents.exports import BusinessDocumentExportService
from api.apps.business_documents.service import BusinessDocumentService
from api.apps.business_documents import worker as worker_module
from api.apps.business_documents.worker import BusinessDocumentJobQueue, BusinessDocumentWorker
from api.db.db_models import (
    BusinessDocument,
    BusinessDocumentEvent,
    BusinessDocumentExportArtifact,
    BusinessDocumentExportStage,
    BusinessDocumentJob,
    BusinessDocumentProposal,
    BusinessDocumentRevision,
)
from test.unit_test.api.apps.business_documents.helpers import required_section_blocks
from common.time_utils import current_timestamp


TENANT = "tenant-worker"
AUTHOR = "author-worker"


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


class MemoryStorage:
    def __init__(self):
        self.objects: dict[tuple[str, str], bytes] = {}
        self.put_count = 0

    def put(self, bucket: str, key: str, content: bytes):
        self.put_count += 1
        self.objects[(bucket, key)] = content

    def get(self, bucket: str, key: str):
        return self.objects.get((bucket, key))

    def rm(self, bucket: str, key: str):
        self.objects.pop((bucket, key), None)

    def obj_exist(self, bucket: str, key: str):
        return (bucket, key) in self.objects

    def remove_and_confirm_absent(self, bucket: str, key: str):
        self.rm(bucket, key)
        return not self.obj_exist(bucket, key)


class FailingAI:
    def process(self, _job):
        raise RuntimeError("transient model failure")


class CompleteIntakeAI:
    def process(self, _job):
        return {"schema_version": "1", "outcome": "COMPLETE", "questions": []}


def _create():
    return BusinessDocumentService.create_document(
        TENANT,
        AUTHOR,
        {
            "schema_version": "1",
            "document_type": "business_requirements",
            "title": "Безопасный экспорт / требования",
            "idea": "Подготовить проверяемые бизнес-требования",
        },
    )


def _command(document, command_type: str, payload=None, *, suffix=""):
    version = document["state_version"]
    return {
        "schema_version": "1",
        "command_id": f"cmd-{command_type.lower()}-{version}{suffix}",
        "idempotency_key": f"idem-{command_type.lower()}-{version}{suffix}",
        "expected_state_version": version,
        "type": command_type,
        "payload": payload or {},
    }


def _question_batch():
    return {
        "schema_version": "1",
        "outcome": "NEEDS_INPUT",
        "questions": [
            {
                "semantic_tag": "audience",
                "stage": "INTAKE",
                "target_section_id": "3.1",
                "text": "Кто использует сервис?",
                "options": [
                    {"option_id": "individuals", "label": "Физические лица"},
                    {"option_id": "companies", "label": "Юридические лица"},
                ],
                "allow_custom_answer": True,
            }
        ],
    }


def _claim_and_complete(job_id: str, output: dict, *, worker_id="worker"):
    job = BusinessDocumentJobQueue.claim(worker_id, lease_ms=60_000)
    assert job is not None and job.id == job_id and job.lease_token
    return BusinessDocumentService.complete_job(TENANT, worker_id, job.id, output, job.lease_token)


def _minimal_ast():
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


def _agreed_document():
    projection = _create()
    document = BusinessDocument.get_by_id(projection["document_id"])
    ast = _minimal_ast()
    body = render_document_ast(ast)
    revision_id = "revision-agreed"
    now_ms = current_timestamp()
    now = DateTime.now()
    created_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document.id) & (BusinessDocumentEvent.event_type == "DocumentCreated"))
    BusinessDocumentRevision.create(
        id=revision_id,
        document_id=document.id,
        revision_number=1,
        document_ast=ast,
        body_markdown=body,
        content_hash=f"sha256:{hashlib.sha256(body.encode('utf-8')).hexdigest()}",
        source_event_ids=[created_event.id],
        create_time=now_ms,
        create_date=now,
        update_time=now_ms,
        update_date=now,
    )
    BusinessDocument.update(
        lifecycle_state="AGREED",
        operation_state="IDLE",
        current_revision_id=revision_id,
        state_version=2,
    ).where(BusinessDocument.id == document.id).execute()
    return BusinessDocumentService.get_document(TENANT, document.id, AUTHOR)


def _request_export(document, storage: MemoryStorage, export_format="EVA_WIKI"):
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "REQUEST_EXPORT",
            {"revision_id": document["current_revision"]["revision_id"], "format": export_format},
        ),
    )
    job = BusinessDocumentJobQueue.claim("export-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]
    prepared = BusinessDocumentExportService.generate(job, storage=storage)
    assert BusinessDocumentExportService.generate(job, storage=storage) == prepared
    assert BusinessDocumentExportArtifact.select().count() == 0
    assert BusinessDocumentExportStage.get_by_id(prepared.stage_id).state == "STORED"
    projection = BusinessDocumentService.complete_job(TENANT, "export-worker", job.id, prepared, job.lease_token)
    assert BusinessDocumentExportStage.get_by_id(prepared.stage_id).state == "COMMITTED"
    BusinessDocumentExportService.discard(prepared, storage=storage)
    assert BusinessDocumentExportStage.select().count() == 0
    artifact = BusinessDocumentExportService.list_artifacts(TENANT, AUTHOR, document["document_id"])[0]
    return artifact, projection


@pytest.mark.p0
def test_expired_lease_is_fenced_and_reclaimed_without_duplicate_completion(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    now_ms = current_timestamp() + 10
    stale = BusinessDocumentJobQueue.claim("worker-a", lease_ms=10, now_ms=now_ms)
    assert stale is not None and stale.id == requested["job_id"]
    stale_token = stale.lease_token

    retry_count, dead_count = BusinessDocumentJobQueue.recover_stale(now_ms=stale.lease_expires_at + 1)
    assert (retry_count, dead_count) == (1, 0)
    current = BusinessDocumentJobQueue.claim("worker-b", lease_ms=60_000, now_ms=stale.lease_expires_at + 1)
    assert current is not None and current.attempt == 2 and current.lease_token != stale_token

    with pytest.raises(BusinessDocumentError, match="Worker no longer owns") as lost:
        BusinessDocumentService.complete_job(
            TENANT,
            "worker-a",
            stale.id,
            {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
            stale_token,
        )
    assert lost.value.code == "JOB_LEASE_LOST"

    projection = BusinessDocumentService.complete_job(
        TENANT,
        "worker-b",
        current.id,
        {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
        current.lease_token,
    )
    assert projection["operation_state"] == "IDLE"
    assert BusinessDocumentJob.get_by_id(current.id).status == "COMPLETED"
    assert BusinessDocumentEvent.select().where((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "IntakeAssessed")).count() == 1


@pytest.mark.p0
def test_job_progress_is_fenced_and_projected_in_list_and_detail(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))

    queued = BusinessDocumentJob.get_by_id(requested["job_id"])
    assert (queued.progress, queued.progress_stage, queued.progress_message) == (0.02, "QUEUED", "Ожидает запуска")
    list_item = BusinessDocumentService.list_documents(TENANT, AUTHOR)["items"][0]
    assert list_item["latest_job"]["progress"] == 0.02
    assert list_item["latest_job"]["progress_stage"] == "QUEUED"

    claimed = BusinessDocumentJobQueue.claim("progress-worker", lease_ms=60_000)
    assert claimed is not None and claimed.lease_token
    assert claimed.progress_stage == "STARTING"
    assert BusinessDocumentJobQueue.update_progress(claimed.id, "wrong-worker", claimed.lease_token, 0.4, "GENERATING", "Формируем результат") is False
    assert BusinessDocumentJobQueue.update_progress(claimed.id, "progress-worker", claimed.lease_token, 0.4, "GENERATING", "Формируем результат") is True

    detail = BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)
    assert detail["latest_job"]["progress"] == 0.4
    assert detail["latest_job"]["progress_message"] == "Формируем результат"

    completed = BusinessDocumentService.complete_job(
        TENANT,
        "progress-worker",
        claimed.id,
        {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
        claimed.lease_token,
    )
    assert completed["latest_job"]["progress"] == 1.0
    assert completed["latest_job"]["progress_stage"] == "COMPLETED"


@pytest.mark.p0
def test_heartbeat_records_lease_loss_when_renewal_is_rejected(database, monkeypatch):
    document = _create()
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "REQUEST_INTAKE_ASSESSMENT"),
    )
    job = BusinessDocumentJobQueue.claim("heartbeat-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]
    renew_called = threading.Event()

    def reject_renewal(*_args, **_kwargs):
        renew_called.set()
        return False

    monkeypatch.setattr(BusinessDocumentJobQueue, "renew", reject_renewal)
    heartbeat = worker_module._LeaseHeartbeat(job, "heartbeat-worker", 1)
    heartbeat.start()
    try:
        assert renew_called.wait(timeout=2)
        with pytest.raises(BusinessDocumentError) as caught:
            heartbeat.ensure_current()
        assert caught.value.code == "JOB_LEASE_LOST"
    finally:
        heartbeat.stop()


@pytest.mark.p0
def test_worker_abandons_rejected_progress_without_export_or_retry(database, monkeypatch):
    document = _agreed_document()
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "REQUEST_EXPORT",
            {"revision_id": document["current_revision"]["revision_id"], "format": "MARKDOWN"},
        ),
    )
    storage = MemoryStorage()
    monkeypatch.setattr(BusinessDocumentJobQueue, "update_progress", lambda *_args, **_kwargs: False)

    worker = BusinessDocumentWorker(
        worker_id="lost-export-worker",
        storage=storage,
        lease_ms=60_000,
        retry_base_ms=0,
    )
    assert worker.run_once() is True

    job = BusinessDocumentJob.get_by_id(requested["job_id"])
    assert job.status == "RUNNING"
    assert job.lease_owner == "lost-export-worker"
    assert job.attempt == 1
    assert storage.put_count == 0
    assert BusinessDocumentExportArtifact.select().count() == 0


@pytest.mark.p0
def test_expired_exhausted_lease_is_dead_lettered_with_persistable_system_identity(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    BusinessDocumentJob.update(max_attempts=1).where(BusinessDocumentJob.id == requested["job_id"]).execute()
    now_ms = current_timestamp() + 10
    stale = BusinessDocumentJobQueue.claim("worker-a", lease_ms=10, now_ms=now_ms)
    assert stale is not None

    retry_count, dead_count = BusinessDocumentJobQueue.recover_stale(now_ms=stale.lease_expires_at + 1)

    assert (retry_count, dead_count) == (0, 1)
    assert BusinessDocumentJob.get_by_id(stale.id).status == "DEAD"
    assert BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)["operation_state"] == "FAILED"


@pytest.mark.p0
def test_retry_backoff_exhaustion_dead_letters_once_and_rejects_wrong_token(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    claimed = BusinessDocumentJobQueue.claim("probe-worker", lease_ms=60_000)
    assert claimed is not None
    assert BusinessDocumentJobQueue.retry(claimed.id, "probe-worker", "wrong-token", {}, delay_ms=0) is False
    assert BusinessDocumentJob.get_by_id(claimed.id).status == "RUNNING"
    assert BusinessDocumentJobQueue.retry(claimed.id, "probe-worker", claimed.lease_token, {}, delay_ms=0) is True

    worker = BusinessDocumentWorker(ai=FailingAI(), retry_base_ms=0, lease_ms=60_000)
    assert len(worker.worker_id) == 32
    assert worker.run_once() is True
    assert BusinessDocumentJob.get_by_id(requested["job_id"]).status == "RETRY"
    assert worker.run_once() is True

    job = BusinessDocumentJob.get_by_id(requested["job_id"])
    projection = BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)
    assert job.status == "DEAD"
    assert job.attempt == job.max_attempts == 3
    assert job.error == {"code": "WORKER_FAILURE", "message": "transient model failure"}
    assert projection["operation_state"] == "FAILED"
    assert worker.run_once() is False
    assert BusinessDocumentEvent.select().where((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "BusinessDocumentJobFailed")).count() == 1

    retry_command = _command(projection, "REQUEST_INTAKE_ASSESSMENT", suffix="-after-dead")
    retried = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], retry_command)
    assert retried["job_id"] != requested["job_id"]
    recovery_worker = BusinessDocumentWorker(worker_id="recovery-worker", ai=CompleteIntakeAI(), retry_base_ms=0, lease_ms=60_000)
    assert recovery_worker.run_once() is True
    recovered = BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)
    assert recovered["operation_state"] == "IDLE"
    assert BusinessDocumentJob.get_by_id(retried["job_id"]).status == "COMPLETED"
    assert BusinessDocumentJob.get_by_id(requested["job_id"]).status == "DEAD"


@pytest.mark.p0
def test_retry_error_remains_visible_while_next_attempt_is_running(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    first = BusinessDocumentJobQueue.claim("first-worker", lease_ms=60_000)
    assert first is not None and first.id == requested["job_id"]
    previous_error = {"code": "INVALID_MODEL_OUTPUT", "message": "Model output failed validation"}
    assert BusinessDocumentJobQueue.retry(first.id, "first-worker", first.lease_token, previous_error, delay_ms=0)

    second = BusinessDocumentJobQueue.claim("second-worker", lease_ms=60_000)

    assert second is not None and second.attempt == 2
    assert second.error == previous_error


@pytest.mark.p1
def test_worker_start_is_singleton_and_wake_interrupts_idle_wait(database, monkeypatch):
    ready = threading.Event()
    stop = threading.Event()

    class FakeWorker:
        def run_forever(self, stop_event, *, poll_seconds):
            assert poll_seconds == 0.25
            ready.set()
            stop_event.wait(2)

    monkeypatch.setattr(worker_module, "BusinessDocumentWorker", FakeWorker)
    monkeypatch.setattr(worker_module, "_WORKER_THREAD", None)
    monkeypatch.setenv("BUSINESS_DOCUMENT_WORKER_ENABLED", "true")
    monkeypatch.setenv("BUSINESS_DOCUMENT_WORKER_POLL_SECONDS", "0.25")
    worker_module._WAKE_EVENT.clear()

    thread = worker_module.start_business_document_worker(stop)
    assert thread is not None and ready.wait(1)
    assert worker_module.start_business_document_worker(stop) is thread
    worker_module.wake_business_document_worker()
    assert worker_module._WAKE_EVENT.is_set()

    stop.set()
    thread.join(timeout=1)
    assert not thread.is_alive()


@pytest.mark.p0
def test_worker_reconciles_export_stages_at_startup_and_when_idle(database, monkeypatch):
    calls = []
    stop = threading.Event()
    worker = BusinessDocumentWorker()

    monkeypatch.setattr(worker, "recover_stale", lambda: calls.append("recover") or (0, 0))
    monkeypatch.setattr(
        worker,
        "reconcile_exports",
        lambda: calls.append("reconcile") or {"scanned": 0, "cleaned": 0, "deferred": 0, "failures": 0},
    )

    def stop_after_empty_poll():
        calls.append("poll")
        stop.set()
        return False

    monkeypatch.setattr(worker, "run_once", stop_after_empty_poll)
    worker_module._WAKE_EVENT.clear()

    worker.run_forever(stop, poll_seconds=0)

    assert calls == ["recover", "reconcile", "poll", "recover", "reconcile"]


@pytest.mark.p0
def test_worker_reconciles_export_stages_on_busy_maintenance_interval(database, monkeypatch):
    calls = []
    stop = threading.Event()
    worker = BusinessDocumentWorker(maintenance_interval_seconds=60)
    clock = iter((0.0, 61.0, 62.0))

    monkeypatch.setattr(worker_module, "monotonic", lambda: next(clock))
    monkeypatch.setattr(worker, "recover_stale", lambda: calls.append("recover") or (0, 0))
    monkeypatch.setattr(
        worker,
        "reconcile_exports",
        lambda: calls.append("reconcile") or {"scanned": 0, "cleaned": 0, "deferred": 0, "failures": 0},
    )

    def stop_after_busy_poll():
        calls.append("poll")
        stop.set()
        return True

    monkeypatch.setattr(worker, "run_once", stop_after_busy_poll)

    worker.run_forever(stop, poll_seconds=0)

    assert calls == ["recover", "reconcile", "poll", "recover", "reconcile"]


@pytest.mark.p0
def test_ai_snapshot_uses_real_answer_event_ids(database):
    document = _create()
    requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT"))
    document = _claim_and_complete(requested["job_id"], _question_batch())
    question = document["protocol"]["questions"][0]
    answered = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "ANSWER_QUESTION",
            {"question_id": question["question_id"], "selected_option_id": "companies", "custom_answer": None},
        ),
    )
    answer_event = BusinessDocumentEvent.get_by_id(answered["event_id"])
    document = BusinessDocumentService.get_document(TENANT, document["document_id"], AUTHOR)
    reassessment = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(document, "REQUEST_INTAKE_ASSESSMENT", suffix="-again"),
    )
    snapshot = BusinessDocumentJob.get_by_id(reassessment["job_id"]).payload
    snapshot_answer = snapshot["protocol"]["questions"][0]["answer"]

    assert snapshot_answer["source_event_id"] == answer_event.id
    assert answer_event.event_type == "QuestionAnswered"
    assert answer_event.payload["question_id"] == question["question_id"]
    assert BusinessDocumentEvent.get_or_none(BusinessDocumentEvent.id == snapshot_answer["source_event_id"]) is not None
    snapshot_event_ids = [event["event_id"] for event in snapshot["source_events"]]
    persisted_event_ids = [event.id for event in BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == document["document_id"]).order_by(BusinessDocumentEvent.sequence.asc())]
    assert snapshot_event_ids == [event_id for event_id in persisted_event_ids if event_id != reassessment["event_id"]]
    assert reassessment["event_id"] not in snapshot_event_ids
    assert snapshot["idea_source_event_id"] == persisted_event_ids[0]


@pytest.mark.p0
def test_draft_proposal_accepts_pinned_idea_event_and_rejects_unknown_source(database):
    def prepare(title_suffix):
        document = BusinessDocumentService.create_document(
            TENANT,
            AUTHOR,
            {
                "schema_version": "1",
                "document_type": "business_requirements",
                "title": f"Draft source {title_suffix}",
                "idea": "Источник идеи должен быть трассируемым",
            },
        )
        assessment = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_INTAKE_ASSESSMENT", suffix=title_suffix))
        document = _claim_and_complete(assessment["job_id"], {"schema_version": "1", "outcome": "COMPLETE", "questions": []})
        requested = BusinessDocumentService.execute_command(TENANT, AUTHOR, document["document_id"], _command(document, "REQUEST_DRAFT", suffix=title_suffix))
        job = BusinessDocumentJobQueue.claim(f"draft-worker-{title_suffix}", lease_ms=60_000)
        assert job is not None and job.id == requested["job_id"]
        return document, job

    document, job = prepare("valid")
    idea_event_id = job.payload["idea_source_event_id"]
    output = {
        "draft": _minimal_ast(),
        "review_questions": {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
        "proposals": [
            {
                "target_section_id": "5.5",
                "text": "Добавить метрику ошибок",
                "rationale": "Повышает проверяемость",
                "source_event_ids": [idea_event_id],
            }
        ],
    }
    completed = BusinessDocumentService.complete_job(TENANT, "draft-worker-valid", job.id, output, job.lease_token)
    proposal = BusinessDocumentProposal.get(BusinessDocumentProposal.document_id == document["document_id"])
    assert completed["lifecycle_state"] == "REVIEW"
    assert idea_event_id in proposal.source_event_ids
    assert BusinessDocumentEvent.get_by_id(idea_event_id).event_type == "DocumentCreated"

    invalid_document, invalid_job = prepare("invalid")
    invalid_output = {
        **output,
        "proposals": [
            {
                **output["proposals"][0],
                "source_event_ids": ["unknown-source-event"],
            }
        ],
    }
    with pytest.raises(BusinessDocumentError) as unknown:
        BusinessDocumentService.complete_job(TENANT, "draft-worker-invalid", invalid_job.id, invalid_output, invalid_job.lease_token)
    assert unknown.value.code == "SOURCE_NOT_IN_JOB_SNAPSHOT"
    assert BusinessDocumentRevision.select().where(BusinessDocumentRevision.document_id == invalid_document["document_id"]).count() == 0
    assert BusinessDocumentProposal.select().where(BusinessDocumentProposal.document_id == invalid_document["document_id"]).count() == 0


@pytest.mark.p0
def test_export_generation_is_idempotent_listable_downloadable_and_hash_verified(database):
    document = _agreed_document()
    storage = MemoryStorage()
    artifact, projection = _request_export(document, storage, "MARKDOWN")
    assert storage.put_count == 1
    assert artifact["revision_number"] == 1
    assert projection["current_revision"]["revision_id"] == document["current_revision"]["revision_id"]
    assert projection["current_revision"]["content_hash"] == document["current_revision"]["content_hash"]
    assert BusinessDocumentExportService.list_artifacts(TENANT, AUTHOR, document["document_id"]) == [artifact]

    metadata, content = BusinessDocumentExportService.download(TENANT, AUTHOR, document["document_id"], artifact["artifact_id"], storage=storage)
    assert metadata == artifact
    assert f"sha256:{hashlib.sha256(content).hexdigest()}" == artifact["content_hash"]
    collaborator_metadata, collaborator_content = BusinessDocumentExportService.download("another-tenant", "another-author", document["document_id"], artifact["artifact_id"], storage=storage)
    assert collaborator_metadata == artifact
    assert collaborator_content == content
    assert BusinessDocumentExportService.list_artifacts("admin-tenant", "admin-user", document["document_id"], is_admin=True) == [artifact]
    admin_metadata, admin_content = BusinessDocumentExportService.download("admin-tenant", "admin-user", document["document_id"], artifact["artifact_id"], storage=storage, is_admin=True)
    assert admin_metadata == artifact
    assert admin_content == content

    row = BusinessDocumentExportArtifact.get_by_id(artifact["artifact_id"])
    storage.objects[(row.storage_bucket, row.storage_key)] = b"tampered"
    with pytest.raises(BusinessDocumentError) as corrupt:
        BusinessDocumentExportService.download(TENANT, AUTHOR, document["document_id"], artifact["artifact_id"], storage=storage)
    assert corrupt.value.code == "EXPORT_STORAGE_CORRUPT"


@pytest.mark.p0
def test_valid_export_is_reused_after_document_owner_changes(database):
    document = _agreed_document()
    storage = MemoryStorage()
    artifact, projection = _request_export(document, storage, "MARKDOWN")
    new_owner = "reassigned-author"
    BusinessDocument.update(
        owner_id=new_owner,
        state_version=projection["state_version"] + 1,
    ).where(BusinessDocument.id == document["document_id"]).execute()
    reassigned = BusinessDocumentService.get_document(TENANT, document["document_id"], new_owner)
    requested = BusinessDocumentService.execute_command(
        TENANT,
        new_owner,
        document["document_id"],
        _command(
            reassigned,
            "REQUEST_EXPORT",
            {"revision_id": reassigned["current_revision"]["revision_id"], "format": "MARKDOWN"},
            suffix="-after-assignment",
        ),
    )
    job = BusinessDocumentJobQueue.claim("reassigned-export-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]

    prepared = BusinessDocumentExportService.generate(job, storage=storage)
    assert prepared.created_blob is False
    assert prepared.artifact_id == artifact["artifact_id"]
    BusinessDocumentService.complete_job(
        TENANT,
        "reassigned-export-worker",
        job.id,
        prepared,
        job.lease_token,
    )
    BusinessDocumentExportService.discard(prepared, storage=storage)

    assert BusinessDocumentExportService.list_artifacts(TENANT, new_owner, document["document_id"]) == [artifact]
    assert storage.put_count == 1


@pytest.mark.p0
def test_export_event_failure_rolls_back_artifact_and_discards_staged_blob(database, monkeypatch):
    document = _agreed_document()
    storage = MemoryStorage()
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "REQUEST_EXPORT",
            {"revision_id": document["current_revision"]["revision_id"], "format": "MARKDOWN"},
        ),
    )
    job = BusinessDocumentJobQueue.claim("rollback-export-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]
    prepared = BusinessDocumentExportService.generate(job, storage=storage)
    before = BusinessDocument.get_by_id(document["document_id"])

    def fail_event(*_args, **_kwargs):
        raise RuntimeError("event insert failed")

    monkeypatch.setattr(BusinessDocumentService, "_create_event", fail_event)
    with pytest.raises(RuntimeError, match="event insert failed"):
        BusinessDocumentService.complete_job(
            TENANT,
            "rollback-export-worker",
            job.id,
            prepared,
            job.lease_token,
        )
    BusinessDocumentExportService.discard(prepared, storage=storage)

    after = BusinessDocument.get_by_id(document["document_id"])
    persisted_job = BusinessDocumentJob.get_by_id(job.id)
    assert (after.state_version, after.operation_state) == (before.state_version, before.operation_state)
    assert persisted_job.status == "RUNNING"
    assert BusinessDocumentExportArtifact.select().count() == 0
    assert BusinessDocumentExportStage.select().count() == 0
    assert storage.objects == {}


@pytest.mark.p0
def test_admin_delete_removes_export_bytes_and_complete_document_history(database):
    document = _agreed_document()
    storage = MemoryStorage()
    artifact, _ = _request_export(document, storage, "MARKDOWN")
    row = BusinessDocumentExportArtifact.get_by_id(artifact["artifact_id"])
    storage_location = (row.storage_bucket, row.storage_key)

    result = BusinessDocumentService.delete_document("admin-user", document["document_id"], is_admin=True, storage=storage)

    assert result["deleted"] is True
    assert result["deleted_artifacts"] == 1
    assert result["storage_cleanup_failures"] == 0
    assert storage_location not in storage.objects
    for model in (BusinessDocumentEvent, BusinessDocumentExportArtifact, BusinessDocumentJob, BusinessDocumentProposal, BusinessDocumentRevision):
        assert model.select().where(model.document_id == document["document_id"]).count() == 0
    assert BusinessDocument.select().where(BusinessDocument.id == document["document_id"]).count() == 0


@pytest.mark.p0
def test_admin_delete_retains_cleanup_ledger_until_blob_removal_is_verified(database):
    class ToggleRemovalStorage(MemoryStorage):
        allow_removal = False

        def rm(self, bucket: str, key: str):
            if self.allow_removal:
                super().rm(bucket, key)

    document = _agreed_document()
    storage = ToggleRemovalStorage()
    artifact, _ = _request_export(document, storage, "MARKDOWN")
    row = BusinessDocumentExportArtifact.get_by_id(artifact["artifact_id"])
    storage_location = (row.storage_bucket, row.storage_key)

    result = BusinessDocumentService.delete_document("admin-user", document["document_id"], is_admin=True, storage=storage)

    assert result == {
        "document_id": document["document_id"],
        "deleted": True,
        "deleted_artifacts": 0,
        "storage_cleanup_failures": 1,
    }
    stage = BusinessDocumentExportStage.get()
    assert stage.state == "CLEANING"
    assert storage_location in storage.objects
    assert BusinessDocumentExportArtifact.select().count() == 0
    assert BusinessDocument.select().where(BusinessDocument.id == document["document_id"]).count() == 0

    storage.allow_removal = True
    reconciled = BusinessDocumentExportService.reconcile_staging(
        storage=storage,
        document_id=document["document_id"],
        now_ms=stage.cleanup_after,
    )
    assert reconciled == {"scanned": 1, "cleaned": 1, "deferred": 0, "failures": 0}
    assert BusinessDocumentExportStage.select().count() == 0
    assert storage_location not in storage.objects


@pytest.mark.p0
def test_admin_delete_retains_cleanup_ledger_when_storage_reports_ambiguous_absence(database):
    class AmbiguousRemovalStorage:
        def __init__(self, objects):
            self.objects = dict(objects)

        def rm(self, _bucket: str, _key: str):
            return None

        def obj_exist(self, _bucket: str, _key: str):
            return False

        def get(self, _bucket: str, _key: str):
            return None

        def health(self):
            return True

    document = _agreed_document()
    source_storage = MemoryStorage()
    artifact, _ = _request_export(document, source_storage, "MARKDOWN")
    row = BusinessDocumentExportArtifact.get_by_id(artifact["artifact_id"])
    storage_location = (row.storage_bucket, row.storage_key)
    storage = AmbiguousRemovalStorage(source_storage.objects)

    result = BusinessDocumentService.delete_document("admin-user", document["document_id"], is_admin=True, storage=storage)

    assert result["storage_cleanup_failures"] == 1
    stage = BusinessDocumentExportStage.get()
    assert stage.state == "CLEANING"
    assert storage_location in storage.objects
    assert BusinessDocumentExportArtifact.select().count() == 0
    assert BusinessDocument.select().where(BusinessDocument.id == document["document_id"]).count() == 0


@pytest.mark.p0
@pytest.mark.parametrize("damage", ["missing", "corrupt"])
def test_repeated_export_atomically_repairs_poisoned_artifact_metadata(database, damage):
    document = _agreed_document()
    storage = MemoryStorage()
    original, document = _request_export(document, storage, "MARKDOWN")
    original_row = BusinessDocumentExportArtifact.get_by_id(original["artifact_id"])
    original_key = (original_row.storage_bucket, original_row.storage_key)
    if damage == "missing":
        storage.objects.pop(original_key)
    else:
        storage.objects[original_key] = b"corrupt"

    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "REQUEST_EXPORT",
            {"revision_id": document["current_revision"]["revision_id"], "format": "MARKDOWN"},
            suffix=f"-{damage}",
        ),
    )
    job = BusinessDocumentJobQueue.claim(f"repair-{damage}", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]
    prepared = BusinessDocumentExportService.generate(job, storage=storage)
    assert BusinessDocumentExportArtifact.get_by_id(original["artifact_id"]).id == original["artifact_id"]
    BusinessDocumentService.complete_job(TENANT, f"repair-{damage}", job.id, prepared, job.lease_token)
    BusinessDocumentExportService.discard(prepared, storage=storage)
    repaired = BusinessDocumentExportService.list_artifacts(TENANT, AUTHOR, document["document_id"])[0]

    assert repaired["artifact_id"] != original["artifact_id"]
    assert BusinessDocumentExportArtifact.get_or_none(BusinessDocumentExportArtifact.id == original["artifact_id"]) is None
    assert (
        BusinessDocumentExportArtifact.select()
        .where(
            (BusinessDocumentExportArtifact.document_id == document["document_id"])
            & (BusinessDocumentExportArtifact.revision_id == document["current_revision"]["revision_id"])
            & (BusinessDocumentExportArtifact.export_format == "MARKDOWN")
        )
        .count()
        == 1
    )
    assert original_key not in storage.objects
    metadata, content = BusinessDocumentExportService.download(TENANT, AUTHOR, document["document_id"], repaired["artifact_id"], storage=storage)
    assert metadata == repaired
    assert f"sha256:{hashlib.sha256(content).hexdigest()}" == repaired["content_hash"]


@pytest.mark.p0
def test_reconciler_finishes_replacement_cleanup_after_committed_process_interruption(database):
    document = _agreed_document()
    storage = MemoryStorage()
    original, projection = _request_export(document, storage, "MARKDOWN")
    original_row = BusinessDocumentExportArtifact.get_by_id(original["artifact_id"])
    original_location = (original_row.storage_bucket, original_row.storage_key)
    storage.objects[original_location] = b"corrupt"

    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            projection,
            "REQUEST_EXPORT",
            {"revision_id": projection["current_revision"]["revision_id"], "format": "MARKDOWN"},
            suffix="-committed-interruption",
        ),
    )
    job = BusinessDocumentJobQueue.claim("replacement-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]
    prepared = BusinessDocumentExportService.generate(job, storage=storage)
    BusinessDocumentService.complete_job(TENANT, "replacement-worker", job.id, prepared, job.lease_token)

    stage = BusinessDocumentExportStage.get_by_id(prepared.stage_id)
    replacement_location = (prepared.storage_bucket, prepared.storage_key)
    assert stage.state == "COMMITTED"
    assert original_location in storage.objects
    assert replacement_location in storage.objects

    reconciled = BusinessDocumentExportService.reconcile_staging(storage=storage)

    assert reconciled == {"scanned": 1, "cleaned": 1, "deferred": 0, "failures": 0}
    assert BusinessDocumentExportStage.select().count() == 0
    assert original_location not in storage.objects
    assert replacement_location in storage.objects
    assert BusinessDocumentExportArtifact.get_by_id(prepared.artifact_id).storage_key == prepared.storage_key


@pytest.mark.p0
def test_interrupted_export_after_put_is_deferred_while_live_and_reconciled_after_lease_recovery(database):
    document = _agreed_document()
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "REQUEST_EXPORT",
            {"revision_id": document["current_revision"]["revision_id"], "format": "MARKDOWN"},
        ),
    )
    job = BusinessDocumentJobQueue.claim("stale-export-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]

    class InterruptedAfterPutStorage(MemoryStorage):
        def put(self, bucket: str, key: str, content: bytes):
            super().put(bucket, key, content)
            raise SystemExit("simulated hard interruption after object PUT")

    storage = InterruptedAfterPutStorage()
    with pytest.raises(SystemExit, match="simulated hard interruption"):
        BusinessDocumentExportService.generate(job, storage=storage)

    stage = BusinessDocumentExportStage.get()
    assert stage.state == "RESERVED"
    assert (stage.storage_bucket, stage.storage_key) in storage.objects
    live_result = BusinessDocumentExportService.reconcile_staging(storage=storage, now_ms=job.lease_expires_at - 1)
    assert live_result == {"scanned": 1, "cleaned": 0, "deferred": 1, "failures": 0}
    assert BusinessDocumentExportStage.get_by_id(stage.id).state == "RESERVED"

    expired_at = current_timestamp() - 1
    BusinessDocumentJob.update(max_attempts=1, lease_expires_at=expired_at).where(BusinessDocumentJob.id == job.id).execute()
    assert BusinessDocumentJobQueue.recover_stale(now_ms=expired_at + 1) == (0, 1)
    cleanup_at = BusinessDocumentExportStage.get_by_id(stage.id).cleanup_after
    reconciled = BusinessDocumentExportService.reconcile_staging(storage=storage, now_ms=max(expired_at + 1, cleanup_at))

    assert reconciled == {"scanned": 1, "cleaned": 1, "deferred": 0, "failures": 0}
    assert BusinessDocumentExportStage.select().count() == 0
    assert BusinessDocumentExportArtifact.select().count() == 0
    assert storage.objects == {}


@pytest.mark.p0
def test_reconciler_rotates_a_deferred_live_stage_past_the_batch_limit(database):
    document = _agreed_document()
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "REQUEST_EXPORT",
            {"revision_id": document["current_revision"]["revision_id"], "format": "MARKDOWN"},
        ),
    )
    job = BusinessDocumentJobQueue.claim("live-export-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]
    storage = MemoryStorage()
    prepared = BusinessDocumentExportService.generate(job, storage=storage)
    live_stage = BusinessDocumentExportStage.get_by_id(prepared.stage_id)

    orphan_content = b"orphaned export"
    orphan_bucket = live_stage.storage_bucket
    orphan_key = f"{live_stage.storage_key}.orphan"
    orphan_id = "orphan-stage"
    BusinessDocumentExportStage.create(
        id=orphan_id,
        job_id=None,
        document_id=live_stage.document_id,
        tenant_id=live_stage.tenant_id,
        owner_id=live_stage.owner_id,
        artifact_id="orphan-artifact",
        lease_token=None,
        revision_id=live_stage.revision_id,
        export_format=live_stage.export_format,
        filename=live_stage.filename,
        mime_type=live_stage.mime_type,
        size=len(orphan_content),
        content_hash=f"sha256:{hashlib.sha256(orphan_content).hexdigest()}",
        storage_identity=exports_module._storage_identity(orphan_bucket, orphan_key),
        storage_bucket=orphan_bucket,
        storage_key=orphan_key,
        state="RESERVED",
        cleanup_after=1,
        create_time=current_timestamp(),
        create_date=DateTime(2001, 1, 1),
        update_time=current_timestamp(),
        update_date=DateTime(2001, 1, 1),
    )
    storage.objects[(orphan_bucket, orphan_key)] = orphan_content

    first = BusinessDocumentExportService.reconcile_staging(
        storage=storage,
        limit=1,
        now_ms=job.lease_expires_at - 1,
    )
    second = BusinessDocumentExportService.reconcile_staging(
        storage=storage,
        limit=1,
        now_ms=job.lease_expires_at - 1,
    )

    assert first == {"scanned": 1, "cleaned": 0, "deferred": 1, "failures": 0}
    assert second == {"scanned": 1, "cleaned": 1, "deferred": 0, "failures": 0}
    assert BusinessDocumentExportStage.get_by_id(live_stage.id).state == "STORED"
    assert BusinessDocumentExportStage.get_or_none(BusinessDocumentExportStage.id == orphan_id) is None
    assert (prepared.storage_bucket, prepared.storage_key) in storage.objects
    assert (orphan_bucket, orphan_key) not in storage.objects


@pytest.mark.p0
def test_reclaimed_export_uses_lease_isolated_staging_keys(database, monkeypatch):
    document = _agreed_document()
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "REQUEST_EXPORT",
            {"revision_id": document["current_revision"]["revision_id"], "format": "MARKDOWN"},
        ),
    )
    rendered = iter((b"attempt-a", b"attempt-b"))
    monkeypatch.setattr(
        BusinessDocumentExportService,
        "_render",
        classmethod(lambda _cls, _export_format, _revision: next(rendered)),
    )
    storage = MemoryStorage()
    first = BusinessDocumentJobQueue.claim("export-worker-a", lease_ms=60_000)
    assert first is not None and first.id == requested["job_id"] and first.lease_token
    first_prepared = BusinessDocumentExportService.generate(first, storage=storage)
    assert BusinessDocumentJobQueue.retry(
        first.id,
        "export-worker-a",
        first.lease_token,
        {"code": "ATTEMPT_INTERRUPTED"},
        delay_ms=0,
    )
    second = BusinessDocumentJobQueue.claim("export-worker-b", lease_ms=60_000)
    assert second is not None and second.attempt == 2 and second.lease_token
    second_prepared = BusinessDocumentExportService.generate(second, storage=storage)

    with pytest.raises(BusinessDocumentError) as stale:
        BusinessDocumentService.complete_job(
            TENANT,
            "export-worker-a",
            first.id,
            first_prepared,
            first.lease_token,
        )
    assert stale.value.code == "JOB_LEASE_LOST"
    assert second_prepared.storage_key != first_prepared.storage_key
    assert first.lease_token in first_prepared.storage_key
    assert second.lease_token in second_prepared.storage_key
    assert second_prepared.lease_token == second.lease_token
    assert storage.put_count == 2
    BusinessDocumentService.complete_job(
        TENANT,
        "export-worker-b",
        second.id,
        second_prepared,
        second.lease_token,
    )
    BusinessDocumentExportService.discard(first_prepared, storage=storage)
    BusinessDocumentExportService.discard(second_prepared, storage=storage)

    artifact = BusinessDocumentExportArtifact.get_by_id(second.id)
    assert artifact.storage_key == second_prepared.storage_key
    assert (artifact.storage_bucket, artifact.storage_key) in storage.objects
    assert storage.objects[(artifact.storage_bucket, artifact.storage_key)] == b"attempt-b"
    assert artifact.content_hash == f"sha256:{hashlib.sha256(b'attempt-b').hexdigest()}"
    assert (first_prepared.storage_bucket, first_prepared.storage_key) not in storage.objects


@pytest.mark.p0
def test_export_requires_durable_storage_write_and_docx_is_valid_zip(database):
    class NoOpStorage(MemoryStorage):
        def put(self, bucket: str, key: str, content: bytes):
            self.put_count += 1

    document = _agreed_document()
    storage = NoOpStorage()
    requested = BusinessDocumentService.execute_command(
        TENANT,
        AUTHOR,
        document["document_id"],
        _command(
            document,
            "REQUEST_EXPORT",
            {"revision_id": document["current_revision"]["revision_id"], "format": "MARKDOWN"},
        ),
    )
    job = BusinessDocumentJobQueue.claim("no-op-storage-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]
    with pytest.raises(BusinessDocumentError) as write_failure:
        BusinessDocumentExportService.generate(job, storage=storage)
    assert write_failure.value.code == "EXPORT_STORAGE_WRITE_FAILED"
    assert BusinessDocumentExportArtifact.select().count() == 0
    assert storage.objects == {}

    docx = BusinessDocumentExportService._render_docx(_minimal_ast())
    assert docx.startswith(b"PK\x03\x04")
    with zipfile.ZipFile(io.BytesIO(docx)) as archive:
        assert "[Content_Types].xml" in archive.namelist()
        assert "word/document.xml" in archive.namelist()


@pytest.mark.p0
def test_eva_wiki_exact_shape_escapes_content_and_excludes_protocol(database, monkeypatch):
    class FrozenDateTime:
        @classmethod
        def now(cls, timezone=None):
            value = DateTime(2026, 8, 26, 12, 0, tzinfo=UTC)
            return value if timezone is not None else value.replace(tzinfo=None)

    monkeypatch.setattr(exports_module, "datetime", FrozenDateTime)
    policy = exports_module.rendering_policy()["eva_wiki"]
    ast = {
        "sections": [
            {
                "id": "1",
                "title": "Scope <trusted>",
                "blocks": [
                    {"type": "paragraph", "text": "A & B"},
                    {"type": "list", "items": ["one", "<two>"]},
                    {"type": "table", "headers": ["Key"], "rows": [["<value>"]]},
                    {"type": "plantuml", "source": "Alice -> Bob"},
                    {"type": "image", "alt": "diagram", "url": "https://example.test/a?x=1&y=2"},
                    {"type": "reference", "label": "Spec & source", "url": "https://example.test/spec"},
                ],
            }
        ]
    }

    rendered = BusinessDocumentExportService._render_eva_wiki(ast, 7)
    assert rendered == "\n".join(
        [
            f'<div class="business-requirements" data-template-version="{published_template()["template_version"]}" data-revision="7" data-generated-at="2026-08-26">',
            '<h2 data-id="br-r7-s1">1. Scope &lt;trusted&gt;</h2>',
            '<p data-id="br-r7-s1-b1">A &amp; B</p>',
            f'<ul class="{policy["root_list_class"]}" style="list-style-type: disc;" data-id="br-r7-s1-b2">'
            '<li data-id="br-r7-s1-b2-li1"><p data-id="br-r7-s1-b2-li1-p">one</p></li>'
            '<li data-id="br-r7-s1-b2-li2"><p data-id="br-r7-s1-b2-li2-p">&lt;two&gt;</p></li></ul>',
            f'<div class="{policy["table_wrapper_class"]}" data-macros="{policy["table_wrapper_macro"]}" data-id="br-r7-s1-b3">'
            '<table data-id="br-r7-s1-b3-table"><thead><tr data-id="br-r7-s1-b3-r0">'
            '<th colspan="1" rowspan="1" data-x="0" data-y="0" data-id="br-r7-s1-b3-h0">'
            '<p data-id="br-r7-s1-b3-h0-p">Key</p></th></tr></thead><tbody><tr data-id="br-r7-s1-b3-r1">'
            '<td colspan="1" rowspan="1" data-x="0" data-y="1" data-id="br-r7-s1-b3-c0-1">'
            '<p data-id="br-r7-s1-b3-c0-1-p">&lt;value&gt;</p></td></tr></tbody></table></div>',
            f'<pre class="{policy["plantuml_class"]}" data-id="br-r7-s1-b4"><code data-id="br-r7-s1-b4-code">Alice -&gt; Bob</code></pre>',
            '<img data-id="br-r7-s1-b5" alt="diagram" src="https://example.test/a?x=1&amp;y=2" />',
            '<p data-id="br-r7-s1-b6"><a data-id="br-r7-s1-b6-a" href="https://example.test/spec">Spec &amp; source</a></p>',
            "</div>",
        ]
    )
    assert "Комментарии автора" not in rendered
    assert "Предложения агента" not in rendered
    assert "Вопросы агента" not in rendered

    legitimate = {"sections": [{"id": "1", "title": "Scope", "blocks": [{"type": "paragraph", "text": "Комментарии автора учтены"}]}]}
    assert "Комментарии автора учтены" in BusinessDocumentExportService._render_eva_wiki(legitimate, 1)

    for unsafe_url in ("javascript:alert(1)", "data:text/html;base64,PHNjcmlwdD4="):
        unsafe = {
            "sections": [
                {
                    "id": "1",
                    "title": "Scope",
                    "blocks": [{"type": "reference", "label": "Unsafe", "url": unsafe_url}],
                }
            ]
        }
        with pytest.raises(BusinessDocumentError) as unsafe_error:
            BusinessDocumentExportService._render_eva_wiki(unsafe, 1)
        assert unsafe_error.value.code == "UNSAFE_EXPORT_URL"

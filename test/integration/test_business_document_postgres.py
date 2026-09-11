"""Real transaction races. Set BUSINESS_DOCUMENT_TEST_POSTGRES_DSN to opt in.

Every test creates and removes its own randomly named PostgreSQL schema. No
application tables or shared records are changed. The account needs CREATE
SCHEMA permission. Separate worker threads use separate database connections.
"""

from concurrent.futures import ThreadPoolExecutor
from datetime import datetime
from io import BytesIO
import os
from pathlib import Path
import sys
from threading import Barrier, Event, local
from time import monotonic, sleep
from types import ModuleType
from uuid import uuid4

import pytest
from minio import Minio
from minio.error import S3Error
from peewee import PostgresqlDatabase
from playhouse.db_url import connect


if "api.apps" not in sys.modules:
    package = ModuleType("api.apps")
    package.__path__ = [str(Path(__file__).resolve().parents[2] / "api" / "apps")]
    sys.modules["api.apps"] = package

from api.apps.business_documents.adapters import assignment as assignment_adapter
from api.apps.business_documents.adapters.storage import BusinessDocumentStorageAdapter
from api.apps.business_documents.evidence import BusinessDocumentEvidence
from api.apps.business_documents.errors import BusinessDocumentError
from api.apps.business_documents.exports import BusinessDocumentExportService
from api.apps.business_documents.service import BusinessDocumentService
from api.apps.business_documents.worker import BusinessDocumentJobQueue
from api.db.db_models import (
    BusinessDocument,
    BusinessDocumentCommand,
    BusinessDocumentEvent,
    BusinessDocumentEvaBinding,
    BusinessDocumentEvidenceSnapshot,
    BusinessDocumentExportArtifact,
    BusinessDocumentExportStage,
    BusinessDocumentJob,
    BusinessDocumentRevision,
    User,
)
from business_documents.application.assign_document import AssignDocument, AssignDocumentCommand, AssignDocumentError
from common.time_utils import current_timestamp


@pytest.fixture
def postgres_database():
    dsn = os.environ.get("BUSINESS_DOCUMENT_TEST_POSTGRES_DSN")
    if not dsn:
        pytest.skip("Set BUSINESS_DOCUMENT_TEST_POSTGRES_DSN for isolated PostgreSQL transaction tests")
    database = connect(dsn, field_types={"LONGTEXT": "TEXT"}, options="-c statement_timeout=15000 -c lock_timeout=10000")
    assert isinstance(database, PostgresqlDatabase), "A PostgreSQL DSN is required"
    schema = "regression_business_documents_" + uuid4().hex
    tables = (*BusinessDocumentService.model_tables(), User)
    schemas = {model: model._meta.schema for model in tables}
    database.connect()
    database.execute_sql(f'CREATE SCHEMA "{schema}"')
    try:
        with database.bind_ctx(tables, bind_refs=False, bind_backrefs=False):
            for model in tables:
                model._meta.schema = schema
            database.create_tables(tables)
            yield database
    finally:
        for model, original in schemas.items():
            model._meta.schema = original
        database.execute_sql(f'DROP SCHEMA "{schema}" CASCADE')
        assert database.execute_sql("SELECT 1 FROM pg_namespace WHERE nspname = %s", (schema,)).fetchone() is None
        database.close()


def _document():
    return BusinessDocumentService.create_document(
        "pg-tenant",
        "pg-author",
        {
            "schema_version": "1",
            "document_type": "business_requirements",
            "title": "Transaction regression",
            "idea": "Test concurrent commands",
        },
    )


def _agreed_document():
    projection = _document()
    document_id = projection["document_id"]
    revision_id = "pg-agreed-revision"
    now_ms = current_timestamp()
    now = datetime.now()
    created_event = BusinessDocumentEvent.get((BusinessDocumentEvent.document_id == document_id) & (BusinessDocumentEvent.event_type == "DocumentCreated"))
    BusinessDocumentRevision.create(
        id=revision_id,
        document_id=document_id,
        revision_number=1,
        document_ast={"sections": []},
        body_markdown="# Transaction-safe export",
        content_hash="sha256:transaction-safe-export",
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
    ).where(BusinessDocument.id == document_id).execute()
    return BusinessDocumentService.get_document("pg-tenant", document_id, "pg-author")


class _MemoryStorage:
    def __init__(self):
        self.objects = {}

    def put(self, bucket, key, content):
        self.objects[(bucket, key)] = content

    def get(self, bucket, key):
        return self.objects.get((bucket, key))

    def rm(self, bucket, key):
        self.objects.pop((bucket, key), None)

    def remove_and_confirm_absent(self, bucket, key):
        self.rm(bucket, key)
        return (bucket, key) not in self.objects


class _MinioStorage:
    def __init__(self, client: Minio):
        self.client = client
        self.conn = client

    @staticmethod
    def _resolve_bucket_and_path(bucket, key):
        return bucket, key

    def put(self, bucket, key, content):
        if not self.client.bucket_exists(bucket):
            self.client.make_bucket(bucket)
        self.client.put_object(bucket, key, BytesIO(content), len(content))

    def get(self, bucket, key):
        try:
            response = self.client.get_object(bucket, key)
        except S3Error as error:
            if error.code in {"NoSuchBucket", "NoSuchKey", "NoSuchObject"}:
                return None
            raise
        try:
            return response.read()
        finally:
            response.close()
            response.release_conn()

    def rm(self, bucket, key):
        self.client.remove_object(bucket, key)


@pytest.fixture
def minio_storage():
    endpoint = os.environ.get("BUSINESS_DOCUMENT_TEST_MINIO_ENDPOINT")
    access_key = os.environ.get("BUSINESS_DOCUMENT_TEST_MINIO_USER")
    secret_key = os.environ.get("BUSINESS_DOCUMENT_TEST_MINIO_PASSWORD")
    if not endpoint or not access_key or not secret_key:
        if os.environ.get("BUSINESS_DOCUMENT_TEST_POSTGRES_DSN"):
            pytest.fail("The PostgreSQL race lane also requires disposable MinIO endpoint/user/password")
        pytest.skip("Set BUSINESS_DOCUMENT_TEST_MINIO_ENDPOINT/USER/PASSWORD for the real MinIO export-stage test")
    client = Minio(
        endpoint.removeprefix("http://").removeprefix("https://"),
        access_key=access_key,
        secret_key=secret_key,
        secure=endpoint.startswith("https://"),
    )
    client.list_buckets()
    return BusinessDocumentStorageAdapter(_MinioStorage(client), "MINIO")


def _evidence_snapshot(job):
    return {
        "schema_version": "1",
        "job_id": job.id,
        "dataset_ids": [],
        "query_hash": "sha256:evidence-query",
        "chunks": [],
        "total_chars": 0,
        "evidence_hash": "sha256:evidence-snapshot",
        "retrieved_at": "2026-09-10T00:00:00+00:00",
    }


def _command(document, key):
    return {
        "schema_version": "1",
        "command_id": key,
        "idempotency_key": key,
        "expected_state_version": document["state_version"],
        "type": "REQUEST_INTAKE_ASSESSMENT",
        "payload": {},
    }


def _parallel(database, operation):
    start = Barrier(2)

    def run(index):
        with database.connection_context():
            backend_pid = database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
            start.wait(timeout=10)
            try:
                result = operation(index)
            except BusinessDocumentError as error:
                result = error
            return backend_pid, result

    with ThreadPoolExecutor(max_workers=2) as pool:
        futures = [pool.submit(run, index) for index in range(2)]
        results = [future.result(timeout=30) for future in futures]
    assert results[0][0] != results[1][0], "Race must use independent PostgreSQL connections"
    return [result for _, result in results]


def _wait_for_backend_lock(database, backend_pid):
    deadline = monotonic() + 10
    last_activity = None
    while monotonic() < deadline:
        last_activity = database.execute_sql(
            "SELECT state, wait_event_type, wait_event FROM pg_stat_activity WHERE pid = %s",
            (backend_pid,),
        ).fetchone()
        if last_activity is not None and last_activity[1] == "Lock":
            return
        sleep(0.02)
    pytest.fail(f"PostgreSQL backend {backend_pid} did not wait for a lock: {last_activity!r}")


def _synchronize_document_reads(monkeypatch):
    barrier = Barrier(2)
    original = BusinessDocumentService._get_editable_document

    def get_document(*args, **kwargs):
        document = original(*args, **kwargs)
        barrier.wait(timeout=10)
        return document

    monkeypatch.setattr(BusinessDocumentService, "_get_editable_document", staticmethod(get_document))


@pytest.mark.p0
def test_concurrent_document_creation_enforces_normalized_title_uniqueness(postgres_database, monkeypatch):
    barrier = Barrier(2)
    create = BusinessDocument.create

    def synchronized_create(*args, **kwargs):
        barrier.wait(timeout=10)
        return create(*args, **kwargs)

    monkeypatch.setattr(BusinessDocument, "create", synchronized_create)
    results = _parallel(
        postgres_database,
        lambda index: BusinessDocumentService.create_document(
            "pg-tenant",
            "pg-author",
            {
                "schema_version": "1",
                "document_type": "business_requirements",
                "title": "  Concurrent   title  " if index == 0 else "CONCURRENT TITLE",
                "idea": f"Concurrent request {index}",
            },
        ),
    )

    assert sum(isinstance(result, dict) for result in results) == 1
    rejected = next(result for result in results if isinstance(result, BusinessDocumentError))
    assert rejected.code == "DOCUMENT_TITLE_ALREADY_EXISTS"
    assert BusinessDocument.select().count() == 1


@pytest.mark.p0
def test_concurrent_creation_enforces_unique_eva_identity(postgres_database, monkeypatch):
    barrier = Barrier(2)
    create_binding = BusinessDocumentEvaBinding.create

    def synchronized_binding_create(*args, **kwargs):
        barrier.wait(timeout=10)
        return create_binding(*args, **kwargs)

    def resolve_binding(_actor_id, raw, _schema_version, display_title):
        suffix = "one" if display_title.endswith("one") else "two"
        return {
            "page_url": f"https://eva.example.test/project/Document/{suffix}",
            "status": "CONNECTED",
            "connector_id": "pg-connector",
            "eva_origin": "https://eva-api.example.test",
            "project_id": "pg-project",
            "document_id": "shared-eva-document",
            "document_name": display_title,
        }

    monkeypatch.setattr(BusinessDocumentEvaBinding, "create", synchronized_binding_create)
    monkeypatch.setattr(BusinessDocumentService, "_resolve_create_eva_binding", staticmethod(resolve_binding))
    results = _parallel(
        postgres_database,
        lambda index: BusinessDocumentService.create_document(
            "pg-tenant",
            "pg-author",
            {
                "schema_version": "2",
                "document_type": "business_requirements",
                "title": f"Concurrent EVA {'one' if index == 0 else 'two'}",
                "idea": f"Concurrent EVA request {index}",
                "eva_page_url": f"https://eva.example.test/request/{index}",
                "eva_decision": {"mode": "BIND", "confirm_replace": True},
            },
        ),
    )

    assert sum(isinstance(result, dict) for result in results) == 1
    rejected = next(result for result in results if isinstance(result, BusinessDocumentError))
    assert rejected.code == "EVA_PAGE_ALREADY_LINKED"
    assert BusinessDocument.select().count() == 1
    assert BusinessDocumentEvaBinding.select().count() == 1


@pytest.mark.p0
@pytest.mark.parametrize("same_key", [True, False], ids=["same-idempotency-key", "different-keys-same-version"])
def test_concurrent_commands_commit_one_transition(postgres_database, monkeypatch, same_key):
    document = _document()
    before_events = BusinessDocumentEvent.select().count()
    _synchronize_document_reads(monkeypatch)
    results = _parallel(
        postgres_database,
        lambda index: BusinessDocumentService.execute_command(
            "pg-tenant",
            "pg-author",
            document["document_id"],
            _command(document, "race" if same_key else f"race-{index}"),
        ),
    )
    accepted = [result for result in results if isinstance(result, dict)]
    if same_key:
        assert len(accepted) == 2, results
        assert accepted[0]["job_id"] == accepted[1]["job_id"]
        assert sum(bool(result.get("idempotent_replay")) for result in accepted) == 1
        assert BusinessDocumentCommand.select().count() == 1
    else:
        assert len(accepted) == 1, results
        rejected = next(result for result in results if isinstance(result, BusinessDocumentError))
        assert rejected.code == "STATE_VERSION_CONFLICT"
        ledger = list(BusinessDocumentCommand.select())
        assert len(ledger) == 2
        assert sum(bool(row.response["accepted"]) for row in ledger) == 1
    assert BusinessDocumentJob.select().count() == 1
    assert BusinessDocumentRevision.select().count() == 0
    assert BusinessDocumentEvent.select().count() == before_events + 1
    assert BusinessDocument.get_by_id(document["document_id"]).state_version == document["state_version"] + 1


@pytest.mark.p0
def test_concurrent_owner_assignments_commit_one_transition(postgres_database):
    document = _document()
    for index in range(2):
        User.create(
            id=f"pg-new-owner-{index}",
            nickname=f"PostgreSQL owner {index}",
            email=f"pg-new-owner-{index}@example.test",
        )

    results = _parallel(
        postgres_database,
        lambda index: assignment_adapter.assign_business_document(
            f"pg-moderator-{index}",
            document["document_id"],
            {
                "owner_id": f"pg-new-owner-{index}",
                "expected_state_version": document["state_version"],
            },
            access_role="EXTENDED_MODERATOR",
        ),
    )

    accepted = [result for result in results if isinstance(result, dict)]
    assert len(accepted) == 1, results
    rejected = next(result for result in results if isinstance(result, BusinessDocumentError))
    assert rejected.code == "STATE_VERSION_CONFLICT"
    assert rejected.status == 409
    assert rejected.details == {
        "expected": document["state_version"],
        "actual": document["state_version"] + 1,
    }

    persisted = BusinessDocument.get_by_id(document["document_id"])
    assert persisted.owner_id == accepted[0]["owner_id"]
    assert persisted.state_version == document["state_version"] + 1
    assignment_events = list(BusinessDocumentEvent.select().where((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "DocumentAssigned")))
    assert len(assignment_events) == 1
    event = assignment_events[0]
    winner_index = int(persisted.owner_id.rsplit("-", 1)[1])
    assert event.sequence == persisted.state_version
    assert event.event_type == "DocumentAssigned"
    assert event.actor_type == "USER"
    assert event.actor_id == f"pg-moderator-{winner_index}"
    assert event.payload == {
        "previous_owner_id": "pg-author",
        "owner_id": persisted.owner_id,
    }
    assert len(event.id) == 32
    assert len(event.correlation_id) == 32
    assert event.id != event.correlation_id
    assert event.causation_id is None
    assert event.create_time == event.update_time
    assert event.create_date == event.update_date


@pytest.mark.p0
def test_owner_assignment_rejects_active_job_without_invalidating_worker_state(postgres_database):
    document = _document()
    owner_id = "pg-owner-active-job"
    User.create(
        id=owner_id,
        nickname="PostgreSQL owner active job",
        email=f"{owner_id}@example.test",
    )
    accepted = BusinessDocumentService.execute_command(
        "pg-tenant",
        "pg-author",
        document["document_id"],
        _command(document, "active-assignment"),
    )
    before = BusinessDocument.get_by_id(document["document_id"])
    before_events = BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == document["document_id"]).count()

    with pytest.raises(BusinessDocumentError) as caught:
        assignment_adapter.assign_business_document(
            "pg-moderator",
            document["document_id"],
            {
                "owner_id": owner_id,
                "expected_state_version": before.state_version,
            },
            access_role="EXTENDED_MODERATOR",
        )

    after = BusinessDocument.get_by_id(document["document_id"])
    job = BusinessDocumentJob.get_by_id(accepted["job_id"])
    assert caught.value.code == "OPERATION_IN_PROGRESS"
    assert caught.value.status == 409
    assert caught.value.details == {}
    assert (after.owner_id, after.state_version, after.operation_state) == (
        before.owner_id,
        before.state_version,
        before.operation_state,
    )
    assert job.status == "PENDING"
    assert BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == document["document_id"]).count() == before_events


@pytest.mark.p0
@pytest.mark.parametrize("disabled_field", ["status", "is_active"], ids=["status", "is-active"])
@pytest.mark.parametrize("first_operation", ["assignment", "deactivation"], ids=["assignment-first", "deactivation-first"])
def test_owner_assignment_and_recipient_deactivation_serialize(postgres_database, monkeypatch, disabled_field, first_operation):
    document = _document()
    owner_id = f"pg-owner-{disabled_field}-{first_operation}"
    User.create(
        id=owner_id,
        nickname=f"PostgreSQL owner {disabled_field} {first_operation}",
        email=f"{owner_id}@example.test",
    )
    winner_locked = Event()
    release_winner = Event()
    assignment_pid_ready = Event()
    deactivation_pid_ready = Event()
    backend_pids = {}
    original_find = assignment_adapter._PeeweeAssignmentUnitOfWork.find_active_user_id

    def find_active_user_id(unit_of_work, requested_owner_id):
        result = original_find(unit_of_work, requested_owner_id)
        if first_operation == "assignment":
            winner_locked.set()
            assert release_winner.wait(timeout=10), "Assignment winner was not released"
        return result

    monkeypatch.setattr(
        assignment_adapter._PeeweeAssignmentUnitOfWork,
        "find_active_user_id",
        find_active_user_id,
    )

    def assign_owner():
        with postgres_database.connection_context():
            backend_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
            backend_pids["assignment"] = backend_pid
            assignment_pid_ready.set()
            try:
                command = AssignDocumentCommand.from_payload(
                    "pg-moderator",
                    document["document_id"],
                    {
                        "owner_id": owner_id,
                        "expected_state_version": document["state_version"],
                    },
                    access_role="EXTENDED_MODERATOR",
                )
                result = AssignDocument(
                    lambda: assignment_adapter._PeeweeAssignmentUnitOfWork(postgres_database),
                    lambda: uuid4().hex,
                ).execute(command)
            except AssignDocumentError as error:
                result = error
            return backend_pid, result

    def deactivate_owner():
        with postgres_database.connection_context():
            backend_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
            backend_pids["deactivation"] = backend_pid
            deactivation_pid_ready.set()
            with postgres_database.atomic():
                changed = User.update({getattr(User, disabled_field): "0"}).where(User.id == owner_id).execute()
                if first_operation == "deactivation":
                    winner_locked.set()
                    assert release_winner.wait(timeout=10), "Deactivation winner was not released"
            return backend_pid, changed

    with ThreadPoolExecutor(max_workers=2) as pool:
        if first_operation == "assignment":
            assignment_future = pool.submit(assign_owner)
            assert winner_locked.wait(timeout=10), "Assignment did not lock the recipient"
            deactivation_future = pool.submit(deactivate_owner)
            assert deactivation_pid_ready.wait(timeout=10), "Deactivation backend did not start"
            loser_pid = backend_pids["deactivation"]
        else:
            deactivation_future = pool.submit(deactivate_owner)
            assert winner_locked.wait(timeout=10), "Deactivation did not lock the recipient"
            assignment_future = pool.submit(assign_owner)
            assert assignment_pid_ready.wait(timeout=10), "Assignment backend did not start"
            loser_pid = backend_pids["assignment"]
        try:
            _wait_for_backend_lock(postgres_database, loser_pid)
        finally:
            release_winner.set()
        assignment_pid, assignment = assignment_future.result(timeout=30)
        deactivation_pid, changed = deactivation_future.result(timeout=30)

    assert assignment_pid != deactivation_pid, "Race must use independent PostgreSQL connections"
    assert changed == 1
    assert getattr(User.get_by_id(owner_id), disabled_field) == "0"
    persisted = BusinessDocument.get_by_id(document["document_id"])
    assignment_events = BusinessDocumentEvent.select().where((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "DocumentAssigned"))
    if first_operation == "assignment":
        assert assignment.owner_id == owner_id
        assert assignment.state_version == document["state_version"] + 1
        assert persisted.owner_id == owner_id
        assert persisted.state_version == document["state_version"] + 1
        assert assignment_events.count() == 1
    else:
        assert isinstance(assignment, AssignDocumentError)
        assert assignment.code == "USER_NOT_FOUND"
        assert persisted.owner_id == "pg-author"
        assert persisted.state_version == document["state_version"]
        assert assignment_events.count() == 0


@pytest.mark.p0
@pytest.mark.parametrize("first_operation", ["assignment", "delete"], ids=["assignment-first", "delete-first"])
def test_owner_assignment_and_delete_serialize_without_orphan_event(postgres_database, monkeypatch, first_operation):
    document = _document()
    owner_id = f"pg-owner-{first_operation}"
    User.create(
        id=owner_id,
        nickname=f"PostgreSQL owner {first_operation}",
        email=f"{owner_id}@example.test",
    )
    winner_locked = Event()
    release_winner = Event()
    assignment_pid_ready = Event()
    delete_pid_ready = Event()
    backend_pids = {}
    original_assignment_get = assignment_adapter._PeeweeAssignmentUnitOfWork.get_document
    original_delete_get = BusinessDocumentService._get_document_for_update

    def get_assignment_document(unit_of_work, document_id):
        result = original_assignment_get(unit_of_work, document_id)
        if first_operation == "assignment":
            winner_locked.set()
            assert release_winner.wait(timeout=10), "Assignment winner was not released"
        return result

    def get_delete_document(document_id):
        result = original_delete_get(document_id)
        if first_operation == "delete":
            winner_locked.set()
            assert release_winner.wait(timeout=10), "Delete winner was not released"
        return result

    monkeypatch.setattr(
        assignment_adapter._PeeweeAssignmentUnitOfWork,
        "get_document",
        get_assignment_document,
    )
    monkeypatch.setattr(
        BusinessDocumentService,
        "_get_document_for_update",
        staticmethod(get_delete_document),
    )

    def assign_owner():
        with postgres_database.connection_context():
            backend_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
            backend_pids["assignment"] = backend_pid
            assignment_pid_ready.set()
            try:
                command = AssignDocumentCommand.from_payload(
                    "pg-moderator",
                    document["document_id"],
                    {
                        "owner_id": owner_id,
                        "expected_state_version": document["state_version"],
                    },
                    access_role="EXTENDED_MODERATOR",
                )
                result = AssignDocument(
                    lambda: assignment_adapter._PeeweeAssignmentUnitOfWork(postgres_database),
                    lambda: uuid4().hex,
                ).execute(command)
            except AssignDocumentError as error:
                result = error
            return backend_pid, result

    def delete_document():
        with postgres_database.connection_context():
            backend_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
            backend_pids["delete"] = backend_pid
            delete_pid_ready.set()
            result = BusinessDocumentService.delete_document(
                "pg-moderator",
                document["document_id"],
                access_role="EXTENDED_MODERATOR",
            )
            return backend_pid, result

    with ThreadPoolExecutor(max_workers=2) as pool:
        if first_operation == "assignment":
            assignment_future = pool.submit(assign_owner)
            assert winner_locked.wait(timeout=10), "Assignment did not lock the document"
            delete_future = pool.submit(delete_document)
            assert delete_pid_ready.wait(timeout=10), "Delete backend did not start"
            loser_pid = backend_pids["delete"]
        else:
            delete_future = pool.submit(delete_document)
            assert winner_locked.wait(timeout=10), "Delete did not lock the document"
            assignment_future = pool.submit(assign_owner)
            assert assignment_pid_ready.wait(timeout=10), "Assignment backend did not start"
            loser_pid = backend_pids["assignment"]
        try:
            _wait_for_backend_lock(postgres_database, loser_pid)
        finally:
            release_winner.set()
        assignment_pid, assignment = assignment_future.result(timeout=30)
        delete_pid, deletion = delete_future.result(timeout=30)

    assert assignment_pid != delete_pid, "Race must use independent PostgreSQL connections"
    assert deletion["deleted"] is True
    assert BusinessDocument.select().where(BusinessDocument.id == document["document_id"]).count() == 0
    assert BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == document["document_id"]).count() == 0
    if first_operation == "assignment":
        assert assignment.owner_id == owner_id
        assert assignment.state_version == document["state_version"] + 1
    else:
        assert isinstance(assignment, AssignDocumentError)
        assert assignment.code == "DOCUMENT_NOT_FOUND"


@pytest.mark.p0
@pytest.mark.parametrize("first_operation", ["command", "delete"], ids=["command-first", "delete-first"])
def test_command_and_delete_serialize_without_orphan_ledger(postgres_database, monkeypatch, first_operation):
    document = _document()
    winner_locked = Event()
    release_winner = Event()
    command_pid_ready = Event()
    delete_pid_ready = Event()
    backend_pids = {}
    thread_state = local()
    original_get = BusinessDocumentService._get_document_for_update

    def get_document_for_update(document_id):
        result = original_get(document_id)
        if getattr(thread_state, "operation", None) == first_operation:
            winner_locked.set()
            assert release_winner.wait(timeout=10), f"{first_operation} winner was not released"
        return result

    monkeypatch.setattr(
        BusinessDocumentService,
        "_get_document_for_update",
        staticmethod(get_document_for_update),
    )

    def execute_command():
        with postgres_database.connection_context():
            thread_state.operation = "command"
            backend_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
            backend_pids["command"] = backend_pid
            command_pid_ready.set()
            try:
                result = BusinessDocumentService.execute_command(
                    "pg-tenant",
                    "pg-author",
                    document["document_id"],
                    _command(document, f"command-delete-{first_operation}"),
                )
            except BusinessDocumentError as error:
                result = error
            return backend_pid, result

    def delete_document():
        with postgres_database.connection_context():
            thread_state.operation = "delete"
            backend_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
            backend_pids["delete"] = backend_pid
            delete_pid_ready.set()
            try:
                result = BusinessDocumentService.delete_document(
                    "pg-moderator",
                    document["document_id"],
                    access_role="EXTENDED_MODERATOR",
                )
            except BusinessDocumentError as error:
                result = error
            return backend_pid, result

    with ThreadPoolExecutor(max_workers=2) as pool:
        if first_operation == "command":
            command_future = pool.submit(execute_command)
            assert winner_locked.wait(timeout=10), "Command did not lock the document"
            delete_future = pool.submit(delete_document)
            assert delete_pid_ready.wait(timeout=10), "Delete backend did not start"
            loser_pid = backend_pids["delete"]
        else:
            delete_future = pool.submit(delete_document)
            assert winner_locked.wait(timeout=10), "Delete did not lock the document"
            command_future = pool.submit(execute_command)
            assert command_pid_ready.wait(timeout=10), "Command backend did not start"
            loser_pid = backend_pids["command"]
        try:
            _wait_for_backend_lock(postgres_database, loser_pid)
        finally:
            release_winner.set()
        command_pid, command_result = command_future.result(timeout=30)
        delete_pid, deletion = delete_future.result(timeout=30)

    assert command_pid != delete_pid, "Race must use independent PostgreSQL connections"
    if first_operation == "command":
        assert isinstance(command_result, dict)
        assert isinstance(deletion, BusinessDocumentError)
        assert deletion.code == "OPERATION_IN_PROGRESS"
        assert deletion.status == 409
        assert BusinessDocument.select().where(BusinessDocument.id == document["document_id"]).count() == 1
        assert BusinessDocumentCommand.select().where(BusinessDocumentCommand.document_id == document["document_id"]).count() == 1
        assert BusinessDocumentJob.select().where(BusinessDocumentJob.document_id == document["document_id"]).count() == 1
    else:
        assert deletion["deleted"] is True
        assert isinstance(command_result, BusinessDocumentError)
        assert command_result.code == "DOCUMENT_NOT_FOUND"
        assert command_result.status == 404
        assert BusinessDocument.select().where(BusinessDocument.id == document["document_id"]).count() == 0
        assert BusinessDocumentCommand.select().where(BusinessDocumentCommand.document_id == document["document_id"]).count() == 0
        assert BusinessDocumentJob.select().where(BusinessDocumentJob.document_id == document["document_id"]).count() == 0
        assert BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == document["document_id"]).count() == 0


@pytest.mark.p0
@pytest.mark.parametrize("mutation", ["delete", "assignment"], ids=["delete", "assignment"])
def test_stale_export_is_fenced_after_lease_recovery(postgres_database, monkeypatch, mutation):
    document = _agreed_document()
    requested = BusinessDocumentService.execute_command(
        "pg-tenant",
        "pg-author",
        document["document_id"],
        {
            "schema_version": "1",
            "command_id": f"pg-export-{mutation}",
            "idempotency_key": f"pg-export-{mutation}",
            "expected_state_version": document["state_version"],
            "type": "REQUEST_EXPORT",
            "payload": {
                "revision_id": document["current_revision"]["revision_id"],
                "format": "MARKDOWN",
            },
        },
    )
    job = BusinessDocumentJobQueue.claim("pg-stale-export-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]
    storage = _MemoryStorage()
    render_started = Event()
    release_render = Event()
    original_render = BusinessDocumentExportService._render

    def blocked_render(_service, export_format, revision):
        render_started.set()
        assert release_render.wait(timeout=10), "Stale export render was not released"
        return original_render(export_format, revision)

    monkeypatch.setattr(BusinessDocumentExportService, "_render", classmethod(blocked_render))

    def generate_export():
        with postgres_database.connection_context():
            backend_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
            prepared = None
            try:
                prepared = BusinessDocumentExportService.generate(job, storage=storage)
                result = BusinessDocumentService.complete_job(
                    job.tenant_id,
                    "pg-stale-export-worker",
                    job.id,
                    prepared,
                    job.lease_token,
                )
            except BusinessDocumentError as error:
                result = error
            finally:
                if prepared is not None:
                    BusinessDocumentExportService.discard(prepared, storage=storage)
            return backend_pid, result

    main_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
    with ThreadPoolExecutor(max_workers=1) as pool:
        export_future = pool.submit(generate_export)
        assert render_started.wait(timeout=10), "Export did not reach the render boundary"
        expired_at = current_timestamp() - 1
        BusinessDocumentJob.update(max_attempts=1, lease_expires_at=expired_at).where(BusinessDocumentJob.id == job.id).execute()
        assert BusinessDocumentJobQueue.recover_stale(now_ms=expired_at + 1) == (0, 1)

        if mutation == "delete":
            deleted = BusinessDocumentService.delete_document(
                "pg-moderator",
                document["document_id"],
                access_role="EXTENDED_MODERATOR",
                storage=storage,
            )
            assert deleted["deleted"] is True
        else:
            owner_id = "pg-owner-after-export-recovery"
            User.create(
                id=owner_id,
                nickname="PostgreSQL owner after export recovery",
                email=f"{owner_id}@example.test",
            )
            recovered = BusinessDocument.get_by_id(document["document_id"])
            assigned = assignment_adapter.assign_business_document(
                "pg-moderator",
                document["document_id"],
                {"owner_id": owner_id, "expected_state_version": recovered.state_version},
                access_role="EXTENDED_MODERATOR",
            )
            assert assigned["owner_id"] == owner_id

        release_render.set()
        export_pid, export_result = export_future.result(timeout=30)

    assert export_pid != main_pid, "Export must run on an independent PostgreSQL connection"
    assert isinstance(export_result, BusinessDocumentError)
    assert export_result.code == "JOB_LEASE_LOST"
    assert BusinessDocumentExportArtifact.select().count() == 0
    assert BusinessDocumentExportStage.select().count() == 0
    assert storage.objects == {}
    if mutation == "delete":
        assert BusinessDocument.select().where(BusinessDocument.id == document["document_id"]).count() == 0
    else:
        persisted = BusinessDocument.get_by_id(document["document_id"])
        assert persisted.owner_id == "pg-owner-after-export-recovery"


@pytest.mark.p0
def test_real_minio_blob_is_reconciled_after_interruption_immediately_after_put(minio_storage, postgres_database):
    document = _agreed_document()
    requested = BusinessDocumentService.execute_command(
        "pg-tenant",
        "pg-author",
        document["document_id"],
        {
            "schema_version": "1",
            "command_id": "pg-minio-export-interruption",
            "idempotency_key": "pg-minio-export-interruption",
            "expected_state_version": document["state_version"],
            "type": "REQUEST_EXPORT",
            "payload": {
                "revision_id": document["current_revision"]["revision_id"],
                "format": "MARKDOWN",
            },
        },
    )
    job = BusinessDocumentJobQueue.claim("pg-minio-interrupted-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]

    class InterruptAfterPut:
        def put(self, bucket, key, content):
            minio_storage.put(bucket, key, content)
            raise SystemExit("simulated process interruption after real MinIO PUT")

        def get(self, bucket, key):
            return minio_storage.get(bucket, key)

        def rm(self, bucket, key):
            return minio_storage.rm(bucket, key)

        def health(self):
            return minio_storage.health()

    location = None
    try:
        with pytest.raises(SystemExit, match="simulated process interruption"):
            BusinessDocumentExportService.generate(job, storage=InterruptAfterPut())
        stage = BusinessDocumentExportStage.get(BusinessDocumentExportStage.document_id == document["document_id"])
        location = (stage.storage_bucket, stage.storage_key)
        assert stage.state == "RESERVED"
        assert minio_storage.get(*location) == b"# Transaction-safe export"
        assert BusinessDocumentExportService.reconcile_staging(storage=minio_storage, now_ms=job.lease_expires_at - 1) == {
            "scanned": 1,
            "cleaned": 0,
            "deferred": 1,
            "failures": 0,
        }

        expired_at = current_timestamp() - 1
        BusinessDocumentJob.update(max_attempts=1, lease_expires_at=expired_at).where(BusinessDocumentJob.id == job.id).execute()
        assert BusinessDocumentJobQueue.recover_stale(now_ms=expired_at + 1) == (0, 1)
        cleanup_at = BusinessDocumentExportStage.get_by_id(stage.id).cleanup_after
        assert BusinessDocumentExportService.reconcile_staging(storage=minio_storage, now_ms=max(expired_at + 1, cleanup_at)) == {
            "scanned": 1,
            "cleaned": 1,
            "deferred": 0,
            "failures": 0,
        }
        assert BusinessDocumentExportStage.select().count() == 0
        assert BusinessDocumentExportArtifact.select().count() == 0
        assert minio_storage.get(*location) is None
    finally:
        if location is not None:
            minio_storage.rm(*location)


@pytest.mark.p0
def test_export_artifact_is_published_atomically_with_job_completion(postgres_database, monkeypatch):
    document = _agreed_document()
    requested = BusinessDocumentService.execute_command(
        "pg-tenant",
        "pg-author",
        document["document_id"],
        {
            "schema_version": "1",
            "command_id": "pg-export-atomic",
            "idempotency_key": "pg-export-atomic",
            "expected_state_version": document["state_version"],
            "type": "REQUEST_EXPORT",
            "payload": {
                "revision_id": document["current_revision"]["revision_id"],
                "format": "MARKDOWN",
            },
        },
    )
    job = BusinessDocumentJobQueue.claim("pg-export-atomic-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]
    storage = _MemoryStorage()
    prepared = BusinessDocumentExportService.generate(job, storage=storage)
    assert BusinessDocumentExportArtifact.select().count() == 0
    assert BusinessDocumentExportStage.get_by_id(prepared.stage_id).state == "STORED"
    artifact_staged = Event()
    release_completion = Event()
    original_complete_export = BusinessDocumentService._complete_export

    def blocked_complete_export(_service, current_document, current_job, actor_id, output, execution):
        artifact_staged.set()
        assert release_completion.wait(timeout=10), "Export completion was not released"
        return original_complete_export(current_document, current_job, actor_id, output, execution)

    monkeypatch.setattr(
        BusinessDocumentService,
        "_complete_export",
        classmethod(blocked_complete_export),
    )

    def complete_export():
        with postgres_database.connection_context():
            try:
                return BusinessDocumentService.complete_job(
                    "pg-tenant",
                    "pg-export-atomic-worker",
                    job.id,
                    prepared,
                    job.lease_token,
                )
            finally:
                BusinessDocumentExportService.discard(prepared, storage=storage)

    with ThreadPoolExecutor(max_workers=1) as pool:
        completion_future = pool.submit(complete_export)
        assert artifact_staged.wait(timeout=10), "Export artifact was not staged in the completion transaction"
        assert BusinessDocumentExportArtifact.select().count() == 0
        assert BusinessDocumentExportStage.get_by_id(prepared.stage_id).state == "STORED"
        assert BusinessDocumentJob.get_by_id(job.id).status == "RUNNING"
        release_completion.set()
        projection = completion_future.result(timeout=30)

    assert projection["operation_state"] == "IDLE"
    assert BusinessDocumentJob.get_by_id(job.id).status == "COMPLETED"
    assert BusinessDocumentExportArtifact.select().where(BusinessDocumentExportArtifact.id == prepared.artifact_id).count() == 1
    assert BusinessDocumentExportStage.select().count() == 0
    assert BusinessDocumentEvent.select().where((BusinessDocumentEvent.document_id == document["document_id"]) & (BusinessDocumentEvent.event_type == "ExportGenerated")).count() == 1
    assert (prepared.storage_bucket, prepared.storage_key) in storage.objects


@pytest.mark.p0
@pytest.mark.parametrize("finalizer", ["complete", "fail"], ids=["complete", "fail"])
def test_job_finalization_serializes_against_recovery_and_reclaim(postgres_database, monkeypatch, finalizer):
    document = _document()
    requested = BusinessDocumentService.execute_command(
        "pg-tenant",
        "pg-author",
        document["document_id"],
        _command(document, f"lease-finalize-{finalizer}"),
    )
    job = BusinessDocumentJobQueue.claim("pg-worker-a", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]
    lease_checked = Event()
    release_finalizer = Event()
    takeover_pid_ready = Event()
    backend_pids = {}
    original_require = BusinessDocumentService._require_current_job_lease

    def require_current_job_lease(current_job, worker_id, lease_token):
        original_require(current_job, worker_id, lease_token)
        if worker_id == "pg-worker-a":
            lease_checked.set()
            assert release_finalizer.wait(timeout=10), "Finalizer was not released"

    monkeypatch.setattr(
        BusinessDocumentService,
        "_require_current_job_lease",
        staticmethod(require_current_job_lease),
    )

    def finalize_job():
        with postgres_database.connection_context():
            backend_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
            if finalizer == "complete":
                result = BusinessDocumentService.complete_job(
                    "pg-tenant",
                    "pg-worker-a",
                    job.id,
                    {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
                    job.lease_token,
                )
            else:
                result = BusinessDocumentService.fail_job(
                    "pg-tenant",
                    "pg-worker-a",
                    job.id,
                    {"code": "FINALIZER_TEST", "message": "forced failure"},
                    job.lease_token,
                )
            return backend_pid, result

    def recover_and_reclaim():
        with postgres_database.connection_context():
            backend_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
            backend_pids["takeover"] = backend_pid
            takeover_pid_ready.set()
            expired_at = current_timestamp() - 1
            changed = (
                BusinessDocumentJob.update(lease_expires_at=expired_at)
                .where((BusinessDocumentJob.id == job.id) & (BusinessDocumentJob.status == "RUNNING") & (BusinessDocumentJob.lease_token == job.lease_token))
                .execute()
            )
            recovered = BusinessDocumentJobQueue.recover_stale(now_ms=expired_at + 1) if changed else (0, 0)
            successor = BusinessDocumentJobQueue.claim("pg-worker-b", lease_ms=60_000, now_ms=expired_at + 1) if changed else None
            return backend_pid, changed, recovered, successor

    with ThreadPoolExecutor(max_workers=2) as pool:
        finalizer_future = pool.submit(finalize_job)
        assert lease_checked.wait(timeout=10), "Finalizer did not pass the lease check"
        takeover_future = pool.submit(recover_and_reclaim)
        assert takeover_pid_ready.wait(timeout=10), "Recovery backend did not start"
        try:
            _wait_for_backend_lock(postgres_database, backend_pids["takeover"])
        finally:
            release_finalizer.set()
        finalizer_pid, result = finalizer_future.result(timeout=30)
        recovery_pid, changed, recovered, successor = takeover_future.result(timeout=30)

    assert finalizer_pid != recovery_pid
    assert isinstance(result, dict)
    assert changed == 0
    assert recovered == (0, 0)
    assert successor is None
    persisted = BusinessDocumentJob.get_by_id(job.id)
    assert persisted.status == ("COMPLETED" if finalizer == "complete" else "DEAD")
    assert persisted.lease_owner is None and persisted.lease_token is None


@pytest.mark.p0
@pytest.mark.parametrize("finalizer", ["complete", "fail"], ids=["complete", "fail"])
def test_reclaimed_job_rejects_previous_worker_finalization(postgres_database, finalizer):
    document = _document()
    requested = BusinessDocumentService.execute_command(
        "pg-tenant",
        "pg-author",
        document["document_id"],
        _command(document, f"lease-reclaimed-{finalizer}"),
    )
    first = BusinessDocumentJobQueue.claim("pg-worker-a", lease_ms=60_000)
    assert first is not None and first.id == requested["job_id"] and first.lease_token
    first_token = first.lease_token
    expired_at = current_timestamp() - 1
    changed = (
        BusinessDocumentJob.update(lease_expires_at=expired_at)
        .where((BusinessDocumentJob.id == first.id) & (BusinessDocumentJob.status == "RUNNING") & (BusinessDocumentJob.lease_token == first_token))
        .execute()
    )
    assert changed == 1

    def recover_and_reclaim():
        with postgres_database.connection_context():
            backend_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
            recovered = BusinessDocumentJobQueue.recover_stale(now_ms=expired_at + 1)
            successor = BusinessDocumentJobQueue.claim("pg-worker-b", lease_ms=60_000, now_ms=expired_at + 1)
            return backend_pid, recovered, successor

    with ThreadPoolExecutor(max_workers=1) as pool:
        takeover_pid, recovered, successor = pool.submit(recover_and_reclaim).result(timeout=30)
    stale_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
    assert takeover_pid != stale_pid
    assert recovered == (1, 0)
    assert successor is not None and successor.id == first.id and successor.attempt == 2 and successor.lease_token

    persisted_before = BusinessDocument.get_by_id(document["document_id"])
    document_state_before = (
        persisted_before.lifecycle_state,
        persisted_before.operation_state,
        persisted_before.state_version,
        persisted_before.current_revision_id,
    )
    event_ids_before = list(
        BusinessDocumentEvent.select(BusinessDocumentEvent.id).where(BusinessDocumentEvent.document_id == document["document_id"]).order_by(BusinessDocumentEvent.sequence).tuples()
    )

    with pytest.raises(BusinessDocumentError) as stale:
        if finalizer == "complete":
            BusinessDocumentService.complete_job(
                "pg-tenant",
                "pg-worker-a",
                first.id,
                {"schema_version": "1", "outcome": "COMPLETE", "questions": []},
                first_token,
            )
        else:
            BusinessDocumentService.fail_job(
                "pg-tenant",
                "pg-worker-a",
                first.id,
                {"code": "STALE_FINALIZER", "message": "must be rejected"},
                first_token,
            )

    assert stale.value.code == "JOB_LEASE_LOST"
    persisted_after = BusinessDocument.get_by_id(document["document_id"])
    assert (
        persisted_after.lifecycle_state,
        persisted_after.operation_state,
        persisted_after.state_version,
        persisted_after.current_revision_id,
    ) == document_state_before
    assert (
        list(BusinessDocumentEvent.select(BusinessDocumentEvent.id).where(BusinessDocumentEvent.document_id == document["document_id"]).order_by(BusinessDocumentEvent.sequence).tuples())
        == event_ids_before
    )
    persisted_job = BusinessDocumentJob.get_by_id(first.id)
    assert persisted_job.status == "RUNNING"
    assert persisted_job.lease_owner == "pg-worker-b"
    assert persisted_job.lease_token == successor.lease_token
    assert persisted_job.attempt == 2


@pytest.mark.p0
def test_evidence_pin_and_delete_serialize_without_orphan_snapshot(postgres_database, monkeypatch):
    document = _document()
    requested = BusinessDocumentService.execute_command(
        "pg-tenant",
        "pg-author",
        document["document_id"],
        _command(document, "evidence-pin-delete"),
    )
    job = BusinessDocumentJobQueue.claim("pg-evidence-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]
    BusinessDocumentJob.update(max_attempts=1).where(BusinessDocumentJob.id == job.id).execute()
    pin_locked = Event()
    release_pin = Event()
    cleanup_pid_ready = Event()
    backend_pids = {}
    original_create = BusinessDocumentEvidenceSnapshot.create

    def blocked_create(*args, **kwargs):
        pin_locked.set()
        assert release_pin.wait(timeout=10), "Evidence pin was not released"
        return original_create(*args, **kwargs)

    monkeypatch.setattr(BusinessDocumentEvidenceSnapshot, "create", blocked_create)

    def pin_evidence():
        with postgres_database.connection_context():
            backend_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
            result = BusinessDocumentEvidence._pin(job, _evidence_snapshot(job))
            return backend_pid, result

    def recover_and_delete():
        with postgres_database.connection_context():
            backend_pid = postgres_database.execute_sql("SELECT pg_backend_pid()").fetchone()[0]
            backend_pids["cleanup"] = backend_pid
            cleanup_pid_ready.set()
            expired_at = current_timestamp() - 1
            changed = (
                BusinessDocumentJob.update(lease_expires_at=expired_at)
                .where((BusinessDocumentJob.id == job.id) & (BusinessDocumentJob.status == "RUNNING") & (BusinessDocumentJob.lease_token == job.lease_token))
                .execute()
            )
            recovered = BusinessDocumentJobQueue.recover_stale(now_ms=expired_at + 1)
            deleted = BusinessDocumentService.delete_document(
                "pg-moderator",
                document["document_id"],
                access_role="EXTENDED_MODERATOR",
            )
            return backend_pid, changed, recovered, deleted

    with ThreadPoolExecutor(max_workers=2) as pool:
        pin_future = pool.submit(pin_evidence)
        assert pin_locked.wait(timeout=10), "Evidence pin did not acquire its root/job locks"
        cleanup_future = pool.submit(recover_and_delete)
        assert cleanup_pid_ready.wait(timeout=10), "Cleanup backend did not start"
        try:
            _wait_for_backend_lock(postgres_database, backend_pids["cleanup"])
        finally:
            release_pin.set()
        pin_pid, pinned = pin_future.result(timeout=30)
        cleanup_pid, changed, recovered, deleted = cleanup_future.result(timeout=30)

    assert pin_pid != cleanup_pid
    assert pinned["job_id"] == job.id
    assert changed == 1
    assert recovered == (0, 1)
    assert deleted["deleted"] is True
    assert BusinessDocument.select().where(BusinessDocument.id == document["document_id"]).count() == 0
    assert BusinessDocumentJob.select().where(BusinessDocumentJob.id == job.id).count() == 0
    assert BusinessDocumentEvidenceSnapshot.select().where(BusinessDocumentEvidenceSnapshot.job_id == job.id).count() == 0


@pytest.mark.p0
def test_recovery_and_delete_precede_stale_evidence_pin(postgres_database):
    document = _document()
    requested = BusinessDocumentService.execute_command(
        "pg-tenant",
        "pg-author",
        document["document_id"],
        _command(document, "evidence-delete-first"),
    )
    job = BusinessDocumentJobQueue.claim("pg-stale-evidence-worker", lease_ms=60_000)
    assert job is not None and job.id == requested["job_id"]
    expired_at = current_timestamp() - 1
    BusinessDocumentJob.update(max_attempts=1, lease_expires_at=expired_at).where(BusinessDocumentJob.id == job.id).execute()
    assert BusinessDocumentJobQueue.recover_stale(now_ms=expired_at + 1) == (0, 1)
    assert (
        BusinessDocumentService.delete_document(
            "pg-moderator",
            document["document_id"],
            access_role="EXTENDED_MODERATOR",
        )["deleted"]
        is True
    )

    with pytest.raises(BusinessDocumentError) as caught:
        BusinessDocumentEvidence._pin(job, _evidence_snapshot(job))

    assert caught.value.code == "JOB_LEASE_LOST"
    assert BusinessDocumentEvidenceSnapshot.select().count() == 0


@pytest.mark.p0
def test_concurrent_workers_claim_a_job_once(postgres_database, monkeypatch):
    document = _document()
    accepted = BusinessDocumentService.execute_command("pg-tenant", "pg-author", document["document_id"], _command(document, "job"))
    barrier = Barrier(2)
    thread_state = local()
    execute = postgres_database.execute_sql

    def execute_sql(sql, params=None, *args, **kwargs):
        if sql.startswith("UPDATE ") and '"business_document_job"' in sql and not getattr(thread_state, "waited", False):
            thread_state.waited = True
            barrier.wait(timeout=10)
        return execute(sql, params, *args, **kwargs)

    monkeypatch.setattr(postgres_database, "execute_sql", execute_sql)
    results = _parallel(postgres_database, lambda index: BusinessDocumentJobQueue.claim(f"pg-worker-{index}"))
    monkeypatch.setattr(postgres_database, "execute_sql", execute)
    claims = [result for result in results if result is not None]
    assert len(claims) == 1, results
    persisted = BusinessDocumentJob.get_by_id(accepted["job_id"])
    assert persisted.status == "RUNNING" and persisted.attempt == 1
    assert persisted.lease_token == claims[0].lease_token
    assert persisted.lease_owner == claims[0].lease_owner
    assert not BusinessDocumentJobQueue.renew(persisted.id, "wrong-worker", persisted.lease_token, lease_ms=60000)

"""Real transaction races. Set BUSINESS_DOCUMENT_TEST_POSTGRES_DSN to opt in.

Every test creates and removes its own randomly named PostgreSQL schema. No
application tables or shared records are changed. The account needs CREATE
SCHEMA permission. Separate worker threads use separate database connections.
"""

from concurrent.futures import ThreadPoolExecutor
import os
from pathlib import Path
import sys
from threading import Barrier, local
from types import ModuleType
from uuid import uuid4

import pytest
from peewee import PostgresqlDatabase
from playhouse.db_url import connect


if "api.apps" not in sys.modules:
    package = ModuleType("api.apps")
    package.__path__ = [str(Path(__file__).resolve().parents[2] / "api" / "apps")]
    sys.modules["api.apps"] = package

from api.apps.business_documents.errors import BusinessDocumentError
from api.apps.business_documents.service import BusinessDocumentService
from api.apps.business_documents.worker import BusinessDocumentJobQueue
from api.db.db_models import BusinessDocument, BusinessDocumentCommand, BusinessDocumentEvent, BusinessDocumentEvaBinding, BusinessDocumentJob, BusinessDocumentRevision, User


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

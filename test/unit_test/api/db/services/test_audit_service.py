"""Audit persistence contracts against disposable local storage."""

import pytest
from peewee import SqliteDatabase

from api.db.db_models import SystemAuditEvent
from api.db.services import audit_service


@pytest.fixture
def audit_database(monkeypatch):
    database = SqliteDatabase(":memory:")
    monkeypatch.setattr(audit_service, "_next_retention_check", float("inf"))
    monkeypatch.setattr(audit_service, "get_ragflow_version", lambda: "regression-test")
    with database.bind_ctx([SystemAuditEvent], bind_refs=False, bind_backrefs=False):
        database.connect()
        database.create_tables([SystemAuditEvent])
        yield database
        database.close()


def test_audit_persists_actor_tenant_correlation_and_safe_metadata(audit_database, monkeypatch):
    monkeypatch.setattr(audit_service, "get_log_context", lambda: {"correlation_id": "operation-1", "request_id": "request-1", "job_id": "job-1"})
    correlation = audit_service.record_audit_event(
        action="DocumentUpdated",
        outcome="success",
        actor_id="author-1",
        actor_type="USER",
        tenant_id="tenant-1",
        object_type="business_document",
        object_id="document-1",
        metadata={"attempt": 2, "path": "/documents", "token": "synthetic-secret", "stage": {"nested": "excluded"}},
    )
    event = SystemAuditEvent.get()
    assert correlation == event.correlation_id == "operation-1"
    assert (event.actor_id, event.actor_type, event.tenant_id) == ("author-1", "USER", "tenant-1")
    assert (event.object_id, event.action, event.outcome) == ("document-1", "DocumentUpdated", "success")
    assert (event.request_id, event.job_id) == ("request-1", "job-1")
    assert event.event_metadata == {"attempt": 2, "path": "/documents"}
    assert event.create_time is not None


def test_failed_audit_append_returns_no_success_receipt(audit_database, monkeypatch):
    def unavailable(**_values):
        raise OSError("synthetic storage failure")

    monkeypatch.setattr(SystemAuditEvent, "create", unavailable)
    assert audit_service.record_audit_event(action="DocumentUpdated", outcome="failure") is None
    assert SystemAuditEvent.select().count() == 0


def test_retention_failure_does_not_prevent_failure_event(audit_database, monkeypatch):
    def unavailable():
        raise OSError("synthetic retention failure")

    monkeypatch.setattr(audit_service, "_next_retention_check", 0)
    monkeypatch.setattr(audit_service, "purge_expired_audit_events", unavailable)
    assert audit_service.record_audit_event(action="DocumentUpdateFailed", outcome="failure", reason_code="VERSION_CONFLICT")
    event = SystemAuditEvent.get()
    assert (event.action, event.outcome, event.reason_code) == ("DocumentUpdateFailed", "failure", "VERSION_CONFLICT")


def test_retention_keeps_exact_cutoff_and_newer_events(audit_database, monkeypatch):
    now = 2_000_000_000_000
    cutoff = now - 30 * 24 * 60 * 60 * 1000
    monkeypatch.setattr(audit_service, "current_timestamp", lambda: now)
    for event_id, timestamp in (("expired", cutoff - 1), ("boundary", cutoff), ("recent", cutoff + 1)):
        SystemAuditEvent.create(id=event_id, action="DocumentUpdated", outcome="success", correlation_id=event_id)
        SystemAuditEvent.update(create_time=timestamp).where(SystemAuditEvent.id == event_id).execute()
    assert audit_service.purge_expired_audit_events() == 1
    assert {event.id for event in SystemAuditEvent.select()} == {"boundary", "recent"}
    assert audit_service.purge_expired_audit_events() == 0

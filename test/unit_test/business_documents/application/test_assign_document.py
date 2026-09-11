from __future__ import annotations

from dataclasses import dataclass, field
import json
from pathlib import Path
import subprocess
import sys

import pytest

from business_documents.application.assign_document import (
    AssignDocument,
    AssignDocumentCommand,
    AssignDocumentError,
    DocumentAssignedEvent,
)
from business_documents.domain.access import BusinessDocumentRole, can_assign_document, normalize_role
from business_documents.domain.assignment import DocumentOwnership, StateVersionMismatch, decide_assignment
from business_documents.domain.workflow import (
    ACTIVE_JOB_STATUSES,
    OperationState,
    is_operation_quiescent,
)


@dataclass
class FakeStore:
    active_user_ids: set[str] = field(default_factory=lambda: {"author-2"})
    document: DocumentOwnership | None = field(default_factory=lambda: DocumentOwnership("doc-1", "author-1", 7))
    active_job: bool = False
    events: list[DocumentAssignedEvent] = field(default_factory=list)
    cas_succeeds: bool = True
    fail_event: bool = False


class FakeAssignmentUnitOfWork:
    def __init__(self, store: FakeStore):
        self.store = store
        self.entered = False
        self.commits = 0
        self.rollbacks = 0
        self.calls: list[tuple[object, ...]] = []
        self._document_snapshot: DocumentOwnership | None = None
        self._event_snapshot: list[DocumentAssignedEvent] = []

    def __enter__(self):
        assert not self.entered
        self.entered = True
        self._document_snapshot = self.store.document
        self._event_snapshot = list(self.store.events)
        self.calls.append(("enter",))
        return self

    def __exit__(self, exc_type, exc, traceback):
        assert self.entered
        self.entered = False
        if exc_type is None:
            self.commits += 1
            self.calls.append(("commit",))
        else:
            self.store.document = self._document_snapshot
            self.store.events[:] = self._event_snapshot
            self.rollbacks += 1
            self.calls.append(("rollback", exc_type))
        return False

    def find_active_user_id(self, owner_id: str) -> str | None:
        self._require_entered()
        self.calls.append(("find_active_user_id", owner_id))
        return owner_id if owner_id in self.store.active_user_ids else None

    def get_document(self, document_id: str) -> DocumentOwnership | None:
        self._require_entered()
        self.calls.append(("get_document", document_id))
        document = self.store.document
        if document is None or document.document_id != document_id:
            return None
        return document

    def has_active_job(self, document_id: str) -> bool:
        self._require_entered()
        self.calls.append(("has_active_job", document_id))
        return self.store.active_job

    def compare_and_set_owner(
        self,
        document_id: str,
        expected_state_version: int,
        owner_id: str,
        new_state_version: int,
    ) -> bool:
        self._require_entered()
        self.calls.append(("compare_and_set_owner", document_id, expected_state_version, owner_id, new_state_version))
        document = self.store.document
        if not self.store.cas_succeeds or document is None or document.document_id != document_id or document.state_version != expected_state_version:
            return False
        self.store.document = DocumentOwnership(
            document_id,
            owner_id,
            new_state_version,
            document.operation_state,
        )
        return True

    def add_event(self, event: DocumentAssignedEvent) -> None:
        self._require_entered()
        self.calls.append(("add_event", event))
        if self.store.fail_event:
            raise RuntimeError("event insert failed")
        self.store.events.append(event)

    def _require_entered(self) -> None:
        assert self.entered, "repository operation escaped the unit of work"


class FakeUnitOfWorkFactory:
    def __init__(self, store: FakeStore):
        self.store = store
        self.instances: list[FakeAssignmentUnitOfWork] = []

    def __call__(self) -> FakeAssignmentUnitOfWork:
        uow = FakeAssignmentUnitOfWork(self.store)
        self.instances.append(uow)
        return uow


def command(
    *,
    owner_id: str = "author-2",
    expected_state_version: int = 7,
    access_role: BusinessDocumentRole | str = BusinessDocumentRole.EXTENDED_MODERATOR,
    is_admin: bool = False,
) -> AssignDocumentCommand:
    return AssignDocumentCommand.from_payload(
        "moderator-1",
        "doc-1",
        {"owner_id": owner_id, "expected_state_version": expected_state_version},
        is_admin=is_admin,
        access_role=access_role,
    )


def application(store: FakeStore | None = None):
    store = store or FakeStore()
    factory = FakeUnitOfWorkFactory(store)
    identifiers = iter(("event-1", "correlation-1"))
    return AssignDocument(factory, lambda: next(identifiers)), store, factory


def test_workflow_policy_defines_exact_stable_and_active_states():
    assert {state.value for state in OperationState} == {
        "IDLE",
        "ANALYZING",
        "ANALYZING_REVIEW",
        "GENERATING_DRAFT",
        "APPLYING_CHANGES",
        "EXPORTING",
        "FAILED",
    }
    assert ACTIVE_JOB_STATUSES == ("PENDING", "RUNNING", "RETRY")


@pytest.mark.parametrize(
    ("operation_state", "has_active_job", "expected"),
    [
        (OperationState.IDLE, False, True),
        (OperationState.FAILED.value, False, True),
        (OperationState.IDLE, True, False),
        (OperationState.FAILED, True, False),
        (OperationState.ANALYZING, False, False),
        ("UNKNOWN", False, False),
        (None, False, False),
    ],
)
def test_workflow_quiescence_requires_stable_state_without_active_job(operation_state, has_active_job, expected):
    assert is_operation_quiescent(operation_state, has_active_job) is expected


@pytest.mark.parametrize(
    ("value", "is_admin", "role", "can_assign"),
    [
        (BusinessDocumentRole.AUTHOR_CREATOR, False, BusinessDocumentRole.AUTHOR_CREATOR, False),
        (BusinessDocumentRole.AUTHOR_EDITOR, False, BusinessDocumentRole.AUTHOR_EDITOR, False),
        (BusinessDocumentRole.MODERATOR_CREATOR, False, BusinessDocumentRole.MODERATOR_CREATOR, False),
        (BusinessDocumentRole.EXTENDED_MODERATOR, False, BusinessDocumentRole.EXTENDED_MODERATOR, True),
        (BusinessDocumentRole.ADMIN, False, BusinessDocumentRole.AUTHOR_EDITOR, False),
        ("ADMIN", False, BusinessDocumentRole.AUTHOR_EDITOR, False),
        ("unknown", False, BusinessDocumentRole.AUTHOR_EDITOR, False),
        (None, False, BusinessDocumentRole.AUTHOR_EDITOR, False),
        (BusinessDocumentRole.AUTHOR_CREATOR, True, BusinessDocumentRole.ADMIN, True),
    ],
)
def test_role_normalization_and_assignment_policy(value, is_admin, role, can_assign):
    assert normalize_role(value, is_admin) == role
    assert can_assign_document(value, is_admin) is can_assign


@pytest.mark.parametrize(
    "raw",
    [
        None,
        [],
        {},
        {"owner_id": None, "expected_state_version": 7},
        {"owner_id": "", "expected_state_version": 7},
        {"owner_id": "   ", "expected_state_version": 7},
        {"owner_id": "author-2"},
        {"owner_id": "author-2", "expected_state_version": True},
        {"owner_id": "author-2", "expected_state_version": "7"},
        {"owner_id": "author-2", "expected_state_version": 7.0},
    ],
)
def test_command_rejects_invalid_payload(raw):
    with pytest.raises(AssignDocumentError) as caught:
        AssignDocumentCommand.from_payload(
            "moderator-1",
            "doc-1",
            raw,
            access_role=BusinessDocumentRole.EXTENDED_MODERATOR,
        )

    assert caught.value.code == "INVALID_DOCUMENT_ASSIGNMENT"
    assert caught.value.details == {}


def test_command_trims_owner_and_normalizes_forged_admin_role():
    parsed = AssignDocumentCommand.from_payload(
        "actor-1",
        "doc-1",
        {"owner_id": "  author-2  ", "expected_state_version": 7},
        access_role="ADMIN",
    )

    assert parsed.owner_id == "author-2"
    assert parsed.access_role == BusinessDocumentRole.AUTHOR_EDITOR


@pytest.mark.parametrize(
    "access_role",
    [
        BusinessDocumentRole.AUTHOR_CREATOR,
        BusinessDocumentRole.AUTHOR_EDITOR,
        BusinessDocumentRole.MODERATOR_CREATOR,
        "ADMIN",
    ],
)
def test_denied_role_does_not_open_unit_of_work(access_role):
    use_case, store, factory = application()

    with pytest.raises(AssignDocumentError) as caught:
        use_case.execute(command(access_role=access_role))

    assert caught.value.code == "DOCUMENT_PERMISSION_DENIED"
    assert factory.instances == []
    assert store.document == DocumentOwnership("doc-1", "author-1", 7)
    assert store.events == []


def test_real_admin_may_assign_even_with_non_privileged_stored_role():
    use_case, store, factory = application()

    result = use_case.execute(command(access_role=BusinessDocumentRole.AUTHOR_CREATOR, is_admin=True))

    assert result.changed is True
    assert store.document == DocumentOwnership("doc-1", "author-2", 8)
    assert factory.instances[0].commits == 1


def test_missing_or_inactive_user_aborts_inside_unit_of_work():
    store = FakeStore(active_user_ids=set())
    use_case, _, factory = application(store)

    with pytest.raises(AssignDocumentError) as caught:
        use_case.execute(command())

    assert caught.value.code == "USER_NOT_FOUND"
    uow = factory.instances[0]
    assert uow.rollbacks == 1
    assert uow.calls[0:2] == [("enter",), ("find_active_user_id", "author-2")]
    assert not any(call[0] == "get_document" for call in uow.calls)


def test_missing_document_aborts_after_active_user_lookup():
    store = FakeStore(document=None)
    use_case, _, factory = application(store)

    with pytest.raises(AssignDocumentError) as caught:
        use_case.execute(command())

    assert caught.value.code == "DOCUMENT_NOT_FOUND"
    assert factory.instances[0].calls[:3] == [
        ("enter",),
        ("find_active_user_id", "author-2"),
        ("get_document", "doc-1"),
    ]


def test_stale_version_precedes_busy_state_and_same_owner_no_op():
    store = FakeStore(
        document=DocumentOwnership("doc-1", "author-2", 8, "ANALYZING"),
        active_job=True,
    )
    use_case, _, factory = application(store)

    with pytest.raises(AssignDocumentError) as caught:
        use_case.execute(command(expected_state_version=7))

    assert caught.value.code == "STATE_VERSION_CONFLICT"
    assert caught.value.details == {"expected": 7, "actual": 8}
    assert store.document == DocumentOwnership("doc-1", "author-2", 8, "ANALYZING")
    assert store.events == []
    assert not any(call[0] == "compare_and_set_owner" for call in factory.instances[0].calls)


def test_current_same_owner_is_no_op_without_disrupting_active_operation():
    store = FakeStore(
        document=DocumentOwnership("doc-1", "author-2", 7, "ANALYZING"),
        active_job=True,
    )
    factory = FakeUnitOfWorkFactory(store)

    def unexpected_id():
        raise AssertionError("no-op must not allocate event identifiers")

    result = AssignDocument(factory, unexpected_id).execute(command())

    assert result.changed is False
    assert result.previous_owner_id == "author-2"
    assert result.owner_id == "author-2"
    assert result.state_version == 7
    assert result.event_id is None
    assert result.correlation_id is None
    assert store.document == DocumentOwnership("doc-1", "author-2", 7, "ANALYZING")
    assert store.events == []
    calls = factory.instances[0].calls
    assert not any(call[0] in {"compare_and_set_owner", "add_event"} for call in calls)


@pytest.mark.parametrize(
    ("operation_state", "active_job"),
    [("ANALYZING", False), ("IDLE", True), ("FAILED", True)],
)
def test_changed_assignment_rejects_active_operation_or_job(operation_state, active_job):
    store = FakeStore(
        document=DocumentOwnership("doc-1", "author-1", 7, operation_state),
        active_job=active_job,
    )
    use_case, _, factory = application(store)

    with pytest.raises(AssignDocumentError) as caught:
        use_case.execute(command())

    assert caught.value.code == "OPERATION_IN_PROGRESS"
    assert caught.value.details == {}
    assert store.document == DocumentOwnership("doc-1", "author-1", 7, operation_state)
    assert store.events == []
    assert factory.instances[0].rollbacks == 1
    assert not any(call[0] in {"compare_and_set_owner", "add_event"} for call in factory.instances[0].calls)


def test_change_cas_and_event_commit_in_one_unit_of_work():
    use_case, store, factory = application()

    result = use_case.execute(command())

    assert result.document_id == "doc-1"
    assert result.previous_owner_id == "author-1"
    assert result.owner_id == "author-2"
    assert result.state_version == 8
    assert result.changed is True
    assert result.event_id == "event-1"
    assert result.correlation_id == "correlation-1"
    assert store.document == DocumentOwnership("doc-1", "author-2", 8)
    assert store.events == [
        DocumentAssignedEvent(
            id="event-1",
            document_id="doc-1",
            sequence=8,
            event_type="DocumentAssigned",
            actor_type="USER",
            actor_id="moderator-1",
            payload={"previous_owner_id": "author-1", "owner_id": "author-2"},
            correlation_id="correlation-1",
        )
    ]
    uow = factory.instances[0]
    assert uow.commits == 1
    assert [call[0] for call in uow.calls] == [
        "enter",
        "find_active_user_id",
        "get_document",
        "has_active_job",
        "compare_and_set_owner",
        "add_event",
        "commit",
    ]


def test_cas_loser_rolls_back_without_event():
    store = FakeStore(cas_succeeds=False)
    use_case, _, factory = application(store)

    with pytest.raises(AssignDocumentError) as caught:
        use_case.execute(command())

    assert caught.value.code == "STATE_VERSION_CONFLICT"
    assert caught.value.message == "The document changed concurrently"
    assert caught.value.details == {}
    assert store.document == DocumentOwnership("doc-1", "author-1", 7)
    assert store.events == []
    uow = factory.instances[0]
    assert uow.rollbacks == 1
    assert not any(call[0] == "add_event" for call in uow.calls)


def test_event_failure_rolls_back_owner_and_version():
    store = FakeStore(fail_event=True)
    use_case, _, factory = application(store)

    with pytest.raises(RuntimeError, match="event insert failed"):
        use_case.execute(command())

    assert store.document == DocumentOwnership("doc-1", "author-1", 7)
    assert store.events == []
    uow = factory.instances[0]
    assert uow.rollbacks == 1
    assert [call[0] for call in uow.calls][-3:] == ["compare_and_set_owner", "add_event", "rollback"]


def test_domain_transition_checks_version_before_same_owner():
    document = DocumentOwnership("doc-1", "author-1", 4)

    with pytest.raises(StateVersionMismatch) as caught:
        decide_assignment(document, "author-1", 3)

    assert caught.value.expected == 3
    assert caught.value.actual == 4


def test_pure_assignment_modules_import_without_runtime_bootstrap():
    root = Path(__file__).resolve().parents[4]
    script = f"""
import json
import sys
import threading
sys.path.insert(0, {str(root)!r})
from business_documents.domain import access, assignment
from business_documents.application import assign_document
forbidden_roots = {{"api", "common", "peewee", "quart", "redis"}}
forbidden = sorted(name for name in sys.modules if name.partition(".")[0] in forbidden_roots)
background = [thread.name for thread in threading.enumerate() if thread is not threading.main_thread()]
print(json.dumps({{"forbidden": forbidden, "background": background}}))
"""

    completed = subprocess.run(
        [sys.executable, "-I", "-B", "-c", script],
        cwd=root,
        check=False,
        capture_output=True,
        text=True,
        timeout=20,
    )

    assert completed.returncode == 0, completed.stderr
    assert json.loads(completed.stdout) == {"forbidden": [], "background": []}

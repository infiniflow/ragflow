"""Application scenario for assigning a Business Document owner."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from types import TracebackType
from typing import Any, Protocol, Self

from business_documents.domain.access import BusinessDocumentRole, can_assign_document, normalize_role
from business_documents.domain.assignment import AssignmentOperationInProgress, DocumentOwnership, StateVersionMismatch, decide_assignment


class AssignDocumentError(Exception):
    """Stable application error without transport-specific status information."""

    def __init__(
        self,
        code: str,
        message: str,
        details: dict[str, Any] | None = None,
    ) -> None:
        super().__init__(message)
        self.code = code
        self.message = message
        self.details = details or {}


@dataclass(frozen=True, slots=True)
class AssignDocumentCommand:
    actor_id: str
    document_id: str
    owner_id: str
    expected_state_version: int
    access_role: BusinessDocumentRole
    is_admin: bool = False

    @classmethod
    def from_payload(
        cls,
        actor_id: str,
        document_id: str,
        raw: object,
        is_admin: bool = False,
        access_role: BusinessDocumentRole | str = BusinessDocumentRole.AUTHOR_CREATOR,
    ) -> AssignDocumentCommand:
        if not isinstance(raw, dict):
            raise AssignDocumentError(
                "INVALID_DOCUMENT_ASSIGNMENT",
                "Request body must be a JSON object",
            )
        owner_id = raw.get("owner_id")
        if not isinstance(owner_id, str) or not owner_id.strip():
            raise AssignDocumentError(
                "INVALID_DOCUMENT_ASSIGNMENT",
                "owner_id must be a non-empty string",
            )
        expected_state_version = raw.get("expected_state_version")
        if isinstance(expected_state_version, bool) or not isinstance(expected_state_version, int):
            raise AssignDocumentError(
                "INVALID_DOCUMENT_ASSIGNMENT",
                "expected_state_version must be an integer",
            )
        return cls(
            actor_id=actor_id,
            document_id=document_id,
            owner_id=owner_id.strip(),
            expected_state_version=expected_state_version,
            access_role=normalize_role(access_role, is_admin),
            is_admin=is_admin,
        )


@dataclass(frozen=True, slots=True)
class AssignDocumentResult:
    document_id: str
    previous_owner_id: str
    owner_id: str
    state_version: int
    changed: bool
    event_id: str | None = None
    correlation_id: str | None = None


@dataclass(frozen=True, slots=True)
class DocumentAssignedEvent:
    id: str
    document_id: str
    sequence: int
    event_type: str
    actor_type: str
    actor_id: str
    payload: dict[str, str]
    correlation_id: str
    causation_id: str | None = None


class AssignmentUnitOfWork(Protocol):
    def __enter__(self) -> Self: ...

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        traceback: TracebackType | None,
    ) -> bool | None: ...

    def find_active_user_id(self, owner_id: str) -> str | None: ...

    def get_document(self, document_id: str) -> DocumentOwnership | None: ...

    def has_active_job(self, document_id: str) -> bool: ...

    def compare_and_set_owner(
        self,
        document_id: str,
        expected_state_version: int,
        owner_id: str,
        new_state_version: int,
    ) -> bool: ...

    def add_event(self, event: DocumentAssignedEvent) -> None: ...


class AssignDocument:
    def __init__(
        self,
        uow_factory: Callable[[], AssignmentUnitOfWork],
        id_factory: Callable[[], str],
    ) -> None:
        self._uow_factory = uow_factory
        self._id_factory = id_factory

    @staticmethod
    def require_permission(
        access_role: BusinessDocumentRole | str,
        is_admin: bool = False,
    ) -> None:
        """Reject unauthorized callers before parsing request-controlled data."""

        if not can_assign_document(access_role, is_admin):
            raise AssignDocumentError(
                "DOCUMENT_PERMISSION_DENIED",
                "Only an extended moderator or administrator can assign business documents",
            )

    def execute(self, command: AssignDocumentCommand) -> AssignDocumentResult:
        self.require_permission(command.access_role, command.is_admin)

        with self._uow_factory() as uow:
            owner_id = uow.find_active_user_id(command.owner_id)
            if owner_id is None:
                raise AssignDocumentError(
                    "USER_NOT_FOUND",
                    "Assigned user not found",
                )

            document = uow.get_document(command.document_id)
            if document is None:
                raise AssignDocumentError(
                    "DOCUMENT_NOT_FOUND",
                    "Business document not found",
                )

            try:
                decision = decide_assignment(
                    document,
                    owner_id,
                    command.expected_state_version,
                    has_active_job=uow.has_active_job(document.document_id),
                )
            except StateVersionMismatch as error:
                raise AssignDocumentError(
                    "STATE_VERSION_CONFLICT",
                    "The document changed since it was loaded",
                    {"expected": error.expected, "actual": error.actual},
                ) from error
            except AssignmentOperationInProgress as error:
                raise AssignDocumentError(
                    "OPERATION_IN_PROGRESS",
                    "A document with an active operation cannot be assigned",
                ) from error

            if not decision.changed:
                return AssignDocumentResult(
                    document_id=document.document_id,
                    previous_owner_id=decision.previous_owner_id,
                    owner_id=decision.owner_id,
                    state_version=decision.state_version,
                    changed=False,
                )

            updated = uow.compare_and_set_owner(
                document.document_id,
                document.state_version,
                decision.owner_id,
                decision.state_version,
            )
            if not updated:
                raise AssignDocumentError(
                    "STATE_VERSION_CONFLICT",
                    "The document changed concurrently",
                )

            event_id = self._id_factory()
            correlation_id = self._id_factory()
            uow.add_event(
                DocumentAssignedEvent(
                    id=event_id,
                    document_id=document.document_id,
                    sequence=decision.state_version,
                    event_type="DocumentAssigned",
                    actor_type="USER",
                    actor_id=command.actor_id,
                    payload={
                        "previous_owner_id": decision.previous_owner_id,
                        "owner_id": decision.owner_id,
                    },
                    correlation_id=correlation_id,
                )
            )
            return AssignDocumentResult(
                document_id=document.document_id,
                previous_owner_id=decision.previous_owner_id,
                owner_id=decision.owner_id,
                state_version=decision.state_version,
                changed=True,
                event_id=event_id,
                correlation_id=correlation_id,
            )

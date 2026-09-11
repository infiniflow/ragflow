"""Pure state transition for assigning a Business Document owner."""

from __future__ import annotations

from dataclasses import dataclass

from business_documents.domain.workflow import OperationState, is_operation_quiescent


@dataclass(frozen=True, slots=True)
class DocumentOwnership:
    document_id: str
    owner_id: str
    state_version: int
    operation_state: str = OperationState.IDLE.value


@dataclass(frozen=True, slots=True)
class AssignmentDecision:
    changed: bool
    previous_owner_id: str
    owner_id: str
    state_version: int


class StateVersionMismatch(Exception):
    def __init__(self, expected: int, actual: int):
        super().__init__(f"Expected state version {expected}, got {actual}")
        self.expected = expected
        self.actual = actual


class AssignmentOperationInProgress(Exception):
    """A changed assignment would invalidate an active aggregate operation."""


def decide_assignment(
    document: DocumentOwnership,
    owner_id: str,
    expected_state_version: int,
    *,
    has_active_job: bool = False,
) -> AssignmentDecision:
    """Validate the optimistic version and derive the owner transition."""

    if document.state_version != expected_state_version:
        raise StateVersionMismatch(expected_state_version, document.state_version)
    if document.owner_id == owner_id:
        return AssignmentDecision(
            changed=False,
            previous_owner_id=document.owner_id,
            owner_id=document.owner_id,
            state_version=document.state_version,
        )
    if not is_operation_quiescent(document.operation_state, has_active_job):
        raise AssignmentOperationInProgress
    return AssignmentDecision(
        changed=True,
        previous_owner_id=document.owner_id,
        owner_id=owner_id,
        state_version=document.state_version + 1,
    )

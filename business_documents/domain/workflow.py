"""Shared workflow states and quiescence rules for Business Documents."""

from __future__ import annotations

from enum import StrEnum


class OperationState(StrEnum):
    IDLE = "IDLE"
    ANALYZING = "ANALYZING"
    ANALYZING_REVIEW = "ANALYZING_REVIEW"
    GENERATING_DRAFT = "GENERATING_DRAFT"
    APPLYING_CHANGES = "APPLYING_CHANGES"
    EXPORTING = "EXPORTING"
    FAILED = "FAILED"


ACTIVE_JOB_STATUSES = ("PENDING", "RUNNING", "RETRY")
_QUIESCENT_OPERATION_STATES = frozenset(
    {
        OperationState.IDLE.value,
        OperationState.FAILED.value,
    }
)


def is_operation_quiescent(
    operation_state: object,
    has_active_job: bool = False,
) -> bool:
    """Return whether a mutation may safely proceed without invalidating work."""

    return not has_active_job and operation_state in _QUIESCENT_OPERATION_STATES

"""Peewee composition for the owner-assignment application scenario."""

from __future__ import annotations

from datetime import datetime
from types import TracebackType
from typing import Any

from api.apps.business_documents.errors import BusinessDocumentError
from api.apps.business_documents.service import BusinessDocumentService
from api.db.db_models import BusinessDocument, BusinessDocumentEvent, BusinessDocumentJob, User
from business_documents.application.assign_document import (
    AssignDocument,
    AssignDocumentCommand,
    AssignDocumentError,
    AssignmentUnitOfWork,
    DocumentAssignedEvent,
)
from business_documents.domain.access import BusinessDocumentRole
from business_documents.domain.assignment import DocumentOwnership
from business_documents.domain.workflow import ACTIVE_JOB_STATUSES
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp


_ERROR_STATUS = {
    "INVALID_DOCUMENT_ASSIGNMENT": 422,
    "DOCUMENT_PERMISSION_DENIED": 403,
    "USER_NOT_FOUND": 404,
    "DOCUMENT_NOT_FOUND": 404,
    "STATE_VERSION_CONFLICT": 409,
    "OPERATION_IN_PROGRESS": 409,
}


class _PeeweeAssignmentUnitOfWork(AssignmentUnitOfWork):
    """Keep recipient validation, owner CAS and audit append in one transaction."""

    def __init__(self, database: Any | None = None) -> None:
        self._database = database or BusinessDocument._meta.database
        self._transaction: Any | None = None

    def __enter__(self) -> _PeeweeAssignmentUnitOfWork:
        self._transaction = self._database.atomic()
        self._transaction.__enter__()
        return self

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        traceback: TracebackType | None,
    ) -> bool | None:
        if self._transaction is None:
            return None
        return self._transaction.__exit__(exc_type, exc, traceback)

    def _lock(self, query: Any) -> Any:
        if getattr(self._database, "for_update", False):
            return query.for_update()
        return query

    def find_active_user_id(self, owner_id: str) -> str | None:
        query = User.select(User.id).where((User.id == owner_id) & (User.status == "1") & (User.is_active == "1"))
        user = self._lock(query).first()
        return None if user is None else user.id

    def get_document(self, document_id: str) -> DocumentOwnership | None:
        query = BusinessDocument.select(
            BusinessDocument.id,
            BusinessDocument.owner_id,
            BusinessDocument.state_version,
            BusinessDocument.operation_state,
        ).where(BusinessDocument.id == document_id)
        document = self._lock(query).first()
        if document is None:
            return None
        return DocumentOwnership(
            document_id=document.id,
            owner_id=document.owner_id,
            state_version=document.state_version,
            operation_state=document.operation_state,
        )

    def has_active_job(self, document_id: str) -> bool:
        return BusinessDocumentJob.select(BusinessDocumentJob.id).where((BusinessDocumentJob.document_id == document_id) & (BusinessDocumentJob.status.in_(ACTIVE_JOB_STATUSES))).exists()

    def compare_and_set_owner(
        self,
        document_id: str,
        expected_state_version: int,
        owner_id: str,
        new_state_version: int,
    ) -> bool:
        changed = (
            BusinessDocument.update(
                owner_id=owner_id,
                state_version=new_state_version,
                update_time=current_timestamp(),
                update_date=datetime.now(),
            )
            .where((BusinessDocument.id == document_id) & (BusinessDocument.state_version == expected_state_version))
            .execute()
        )
        return changed == 1

    def add_event(self, event: DocumentAssignedEvent) -> None:
        now = datetime.now()
        timestamp = current_timestamp()
        BusinessDocumentEvent.create(
            id=event.id,
            document_id=event.document_id,
            sequence=event.sequence,
            event_type=event.event_type,
            actor_type=event.actor_type,
            actor_id=event.actor_id,
            payload=event.payload,
            correlation_id=event.correlation_id,
            causation_id=event.causation_id,
            create_time=timestamp,
            create_date=now,
            update_time=timestamp,
            update_date=now,
        )


def assign_business_document(
    actor_id: str,
    document_id: str,
    raw: object,
    is_admin: bool = False,
    access_role: BusinessDocumentRole | str = BusinessDocumentRole.AUTHOR_CREATOR,
) -> dict[str, Any]:
    """Execute assignment and return the existing full HTTP projection."""

    try:
        AssignDocument.require_permission(access_role, is_admin)
        command = AssignDocumentCommand.from_payload(
            actor_id,
            document_id,
            raw,
            is_admin,
            access_role,
        )
        AssignDocument(
            lambda: _PeeweeAssignmentUnitOfWork(BusinessDocument._meta.database),
            get_uuid,
        ).execute(command)
    except AssignDocumentError as error:
        raise BusinessDocumentError(
            error.code,
            error.message,
            _ERROR_STATUS[error.code],
            error.details,
        ) from error

    return BusinessDocumentService.get_document(
        actor_id,
        document_id,
        actor_id,
        is_admin,
        command.access_role,
    )

"""Pure role rules for the Business Documents extension."""

from __future__ import annotations

from enum import StrEnum


class BusinessDocumentRole(StrEnum):
    AUTHOR_CREATOR = "AUTHOR_CREATOR"
    AUTHOR_EDITOR = "AUTHOR_EDITOR"
    MODERATOR_CREATOR = "MODERATOR_CREATOR"
    EXTENDED_MODERATOR = "EXTENDED_MODERATOR"
    ADMIN = "ADMIN"


def normalize_role(
    value: object,
    is_admin: bool = False,
) -> BusinessDocumentRole:
    """Resolve the effective role without trusting a claimed administrator role."""

    if is_admin:
        return BusinessDocumentRole.ADMIN
    try:
        role = BusinessDocumentRole(value)
    except (TypeError, ValueError):
        return BusinessDocumentRole.AUTHOR_EDITOR
    if role == BusinessDocumentRole.ADMIN:
        return BusinessDocumentRole.AUTHOR_EDITOR
    return role


def can_assign_document(value: object, is_admin: bool = False) -> bool:
    """Return whether the effective role may assign a document owner."""

    return normalize_role(value, is_admin) in {
        BusinessDocumentRole.EXTENDED_MODERATOR,
        BusinessDocumentRole.ADMIN,
    }

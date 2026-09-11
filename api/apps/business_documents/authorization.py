#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

from __future__ import annotations

from dataclasses import dataclass

from api.apps.business_documents.errors import PermissionDeniedError
from business_documents.domain.access import BusinessDocumentRole as _BusinessDocumentRole
from business_documents.domain.access import can_assign_document, normalize_role


__all__ = ["BusinessDocumentAccess"]


@dataclass(frozen=True)
class BusinessDocumentAccess:
    actor_id: str
    assigned_role: _BusinessDocumentRole | str = _BusinessDocumentRole.AUTHOR_CREATOR
    is_admin: bool = False

    @property
    def role(self) -> _BusinessDocumentRole:
        return normalize_role(self.assigned_role, self.is_admin)

    def capabilities(self) -> dict[str, bool]:
        role = self.role
        return {
            "read": True,
            "create": role
            in {
                _BusinessDocumentRole.AUTHOR_CREATOR,
                _BusinessDocumentRole.MODERATOR_CREATOR,
                _BusinessDocumentRole.EXTENDED_MODERATOR,
                _BusinessDocumentRole.ADMIN,
            },
            "edit_own": True,
            "edit_all": role
            in {
                _BusinessDocumentRole.MODERATOR_CREATOR,
                _BusinessDocumentRole.EXTENDED_MODERATOR,
                _BusinessDocumentRole.ADMIN,
            },
            "delete": role in {_BusinessDocumentRole.EXTENDED_MODERATOR, _BusinessDocumentRole.ADMIN},
            "assign": can_assign_document(self.assigned_role, self.is_admin),
        }

    def permissions(self, owner_id: str) -> dict[str, bool]:
        capabilities = self.capabilities()
        return {
            "read": True,
            "edit": capabilities["edit_all"] or owner_id == self.actor_id,
            "delete": capabilities["delete"],
            "assign": capabilities["assign"],
        }

    def require_create(self) -> None:
        if not self.capabilities()["create"]:
            raise PermissionDeniedError("This role cannot create business documents")

    def require_edit(self, owner_id: str) -> None:
        if not self.permissions(owner_id)["edit"]:
            raise PermissionDeniedError("Only the document owner or a moderator can edit this business document")

    def require_delete(self) -> None:
        if not self.capabilities()["delete"]:
            raise PermissionDeniedError("Only an extended moderator or administrator can delete business documents")

    def require_assign(self) -> None:
        if not can_assign_document(self.assigned_role, self.is_admin):
            raise PermissionDeniedError("Only an extended moderator or administrator can assign business documents")

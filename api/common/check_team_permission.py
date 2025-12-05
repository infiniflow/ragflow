#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

from typing import Dict, Any, Optional, List, Literal

from api.db import TenantPermission
from api.db.db_models import File, Knowledgebase
from api.db.services.user_service import UserTenantService
from api.common.permission_utils import has_permission
from common.constants import StatusEnum


def check_kb_team_permission(
    kb: Dict[str, Any] | Knowledgebase,
    other: str,
    required_permission: Literal["create", "read", "update", "delete"] = "read"
) -> bool:
    kb = kb.to_dict() if isinstance(kb, Knowledgebase) else kb

    kb_tenant_id = kb["tenant_id"]

    # If user owns the tenant where the KB was created, always allow
    if kb_tenant_id == other:
        return True

    # If permission is not "team", deny access
    if kb["permission"] != TenantPermission.TEAM:
        return False

    # Determine which tenant to check permissions for
    shared_tenant_id: Optional[str] = kb.get("shared_tenant_id")
    target_tenant_id: str = shared_tenant_id if shared_tenant_id else kb_tenant_id
    
    # Check if user is a member of the target tenant
    user_tenant = UserTenantService.filter_by_tenant_and_user_id(target_tenant_id, other)
    if not user_tenant or user_tenant.status != StatusEnum.VALID.value:
        return False
    
    # Check CRUD permissions
    return has_permission(target_tenant_id, other, "dataset", required_permission)


def check_file_team_permission(file: Dict[str, Any] | File, other: str) -> bool:
    # Import here to avoid circular import
    from api.db.services.file_service import FileService
    
    file = file.to_dict() if isinstance(file, File) else file

    file_tenant_id = file["tenant_id"]
    if file_tenant_id == other:
        return True

    file_id: str = file["id"]

    kb_ids: List[str] = [kb_info["kb_id"] for kb_info in FileService.get_kb_id_by_file_id(file_id)]

    # Import here to avoid circular import
    from api.db.services.knowledgebase_service import KnowledgebaseService
    
    for kb_id in kb_ids:
        ok: bool
        kb: Optional[Knowledgebase]
        ok, kb = KnowledgebaseService.get_by_id(kb_id)
        if not ok or kb is None:
            continue

        if check_kb_team_permission(kb, other):
            return True

    return False

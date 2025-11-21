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

from typing import Dict, Any, Optional, Literal

from api.db.services.user_service import UserTenantService
from api.db.db_models import UserTenant
from api.db import UserTenantRole
from common.constants import StatusEnum


def get_user_permissions(tenant_id: str, user_id: str) -> Dict[str, Dict[str, bool]]:
    """
    Get CRUD permissions for a user in a tenant.
    
    Args:
        tenant_id: The tenant ID.
        user_id: The user ID.
        
    Returns:
        Dictionary with permissions for dataset and canvas:
        {
            "dataset": {"create": bool, "read": bool, "update": bool, "delete": bool},
            "canvas": {"create": bool, "read": bool, "update": bool, "delete": bool}
        }
        Returns default permissions (read-only) if user not found or no permissions set.
    """
    user_tenant = UserTenantService.filter_by_tenant_and_user_id(tenant_id, user_id)
    
    if not user_tenant or user_tenant.status != StatusEnum.VALID.value:
        # Return default read-only permissions
        return {
            "dataset": {"create": False, "read": True, "update": False, "delete": False},
            "canvas": {"create": False, "read": True, "update": False, "delete": False}
        }
    
    # Owners and admins have full permissions
    if user_tenant.role in [UserTenantRole.OWNER, UserTenantRole.ADMIN]:
        return {
            "dataset": {"create": True, "read": True, "update": True, "delete": True},
            "canvas": {"create": True, "read": True, "update": True, "delete": True}
        }
    
    # Get permissions from user_tenant.permissions, with defaults
    permissions = user_tenant.permissions if user_tenant.permissions else {}
    
    default_permissions = {
        "dataset": {"create": False, "read": True, "update": False, "delete": False},
        "canvas": {"create": False, "read": True, "update": False, "delete": False}
    }
    
    # Merge with defaults to ensure all fields are present
    result: Dict[str, Dict[str, bool]] = {}
    for resource_type in ["dataset", "canvas"]:
        result[resource_type] = {
            "create": permissions.get(resource_type, {}).get("create", default_permissions[resource_type]["create"]),
            "read": permissions.get(resource_type, {}).get("read", default_permissions[resource_type]["read"]),
            "update": permissions.get(resource_type, {}).get("update", default_permissions[resource_type]["update"]),
            "delete": permissions.get(resource_type, {}).get("delete", default_permissions[resource_type]["delete"]),
        }
    
    return result


def has_permission(
    tenant_id: str,
    user_id: str,
    resource_type: Literal["dataset", "canvas"],
    permission: Literal["create", "read", "update", "delete"]
) -> bool:
    """
    Check if a user has a specific permission for a resource type in a tenant.
    
    Args:
        tenant_id: The tenant ID.
        user_id: The user ID.
        resource_type: Type of resource ("dataset" or "canvas").
        permission: The permission to check ("create", "read", "update", or "delete").
        
    Returns:
        True if user has the permission, False otherwise.
    """
    permissions = get_user_permissions(tenant_id, user_id)
    return permissions.get(resource_type, {}).get(permission, False)


def update_user_permissions(
    tenant_id: str,
    user_id: str,
    permissions: Dict[str, Dict[str, bool]]
) -> bool:
    """
    Update CRUD permissions for a user in a tenant.
    
    Args:
        tenant_id: The tenant ID.
        user_id: The user ID.
        permissions: Dictionary with permissions to update:
            {
                "dataset": {"create": bool, "read": bool, "update": bool, "delete": bool},
                "canvas": {"create": bool, "read": bool, "update": bool, "delete": bool}
            }
            Only provided fields will be updated, others will remain unchanged.
            
    Returns:
        True if update was successful, False otherwise.
    """
    user_tenant = UserTenantService.filter_by_tenant_and_user_id(tenant_id, user_id)
    
    if not user_tenant:
        return False
    
    # Get current permissions or defaults
    current_permissions = user_tenant.permissions if user_tenant.permissions else {
        "dataset": {"create": False, "read": True, "update": False, "delete": False},
        "canvas": {"create": False, "read": True, "update": False, "delete": False}
    }
    
    # Merge new permissions with current permissions
    updated_permissions: Dict[str, Dict[str, bool]] = {}
    for resource_type in ["dataset", "canvas"]:
        updated_permissions[resource_type] = current_permissions.get(resource_type, {}).copy()
        if resource_type in permissions:
            updated_permissions[resource_type].update(permissions[resource_type])
    
    # Update in database
    from api.db.db_models import UserTenant as UserTenantModel
    UserTenantService.filter_update(
        [UserTenantModel.tenant_id == tenant_id, UserTenantModel.user_id == user_id],
        {"permissions": updated_permissions}
    )
    
    return True


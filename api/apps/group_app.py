#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
"""Group management API endpoints."""

import logging
from typing import Any, Dict, List, Optional

from flask import Blueprint, Response, request
from flask_login import current_user, login_required

from api.db import UserTenantRole
from api.db.db_models import DB, Group, GroupUser, Tenant, User, UserTenant
from api.db.services.user_service import (
    GroupService,
    GroupUserService,
    TenantService,
    UserService,
    UserTenantService,
)
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    server_error_response,
    validate_request,
)

from common.constants import RetCode, StatusEnum
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp, datetime_format
from datetime import datetime

manager = Blueprint("group", __name__)


def is_team_member(tenant_id: str, user_id: str) -> bool:
    """Check if a user is a member of a team (tenant).
    
    Args:
        tenant_id: The team/tenant ID.
        user_id: The user ID to check.
        
    Returns:
        True if user is a member of the team, False otherwise.
    """
    user_tenant: Optional[UserTenant] = UserTenantService.filter_by_tenant_and_user_id(
        tenant_id, user_id
    )
    return user_tenant is not None and user_tenant.status == StatusEnum.VALID.value


def is_team_admin_or_owner(tenant_id: str, user_id: str) -> bool:
    """Check if a user is an OWNER or ADMIN of a team.
    
    Args:
        tenant_id: The team/tenant ID.
        user_id: The user ID to check.
        
    Returns:
        True if user is OWNER or ADMIN, False otherwise.
    """
    user_tenant: Optional[UserTenant] = UserTenantService.filter_by_tenant_and_user_id(
        tenant_id, user_id
    )
    if not user_tenant:
        return False
    return user_tenant.role in [UserTenantRole.OWNER, UserTenantRole.ADMIN]


@manager.route("/create", methods=["POST"])  # noqa: F821
@login_required
@validate_request("name", "tenant_id")
def create_group() -> Response:
    """Create a new group within a team.
    
    Groups are collections of users that hold specific rights/permissions.
    Only team owners or admins can create groups.
    
    ---
    tags:
      - Group
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        required: true
        schema:
          type: object
          required:
            - name
            - tenant_id
          properties:
            name:
              type: string
              description: Group name.
            tenant_id:
              type: string
              description: Team/tenant ID that the group belongs to.
            description:
              type: string
              description: Optional group description.
    responses:
      200:
        description: Group created successfully.
        schema:
          type: object
          properties:
            data:
              type: object
              description: Created group information.
            message:
              type: string
              description: Success message.
      400:
        description: Invalid request or team not found.
      401:
        description: Unauthorized.
      403:
        description: Forbidden - not team owner or admin.
    """
    if request.json is None:
        return get_json_result(
            data=False,
            message="Request body is required!",
            code=RetCode.ARGUMENT_ERROR,
        )
    
    req: Dict[str, Any] = request.json
    name: str = req.get("name", "").strip()
    tenant_id: str = req.get("tenant_id", "").strip()
    description: Optional[str] = req.get("description", "").strip() or None
    
    if not name:
        return get_json_result(
            data=False,
            message="Group name cannot be empty!",
            code=RetCode.ARGUMENT_ERROR,
        )
    
    if len(name) > 128:
        return get_json_result(
            data=False,
            message="Group name must be 128 characters or less!",
            code=RetCode.ARGUMENT_ERROR,
        )
    
    if not tenant_id:
        return get_json_result(
            data=False,
            message="Tenant ID is required!",
            code=RetCode.ARGUMENT_ERROR,
        )
    
    # Check if user is team owner or admin
    if not is_team_admin_or_owner(tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message="Only team owners or admins can create groups.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    # Verify tenant exists
    tenants_query = TenantService.query(id=tenant_id, status=StatusEnum.VALID.value)
    tenants: List[Tenant] = list(tenants_query)
    if not tenants:
        return get_data_error_result(message="Team not found.")
    
    try:
        # Check if group with same name already exists in this tenant
        existing_group: Optional[Group] = GroupService.get_by_tenant_and_name(tenant_id, name)
        if existing_group:
            return get_json_result(
                data=False,
                message=f"Group with name '{name}' already exists in this team.",
                code=RetCode.DATA_ERROR,
            )
        
        # Create group
        group_id: str = get_uuid()
        group_data: Dict[str, Any] = {
            "id": group_id,
            "tenant_id": tenant_id,
            "name": name,
            "description": description,
            "created_by": current_user.id,
            "status": StatusEnum.VALID.value,
        }
        
        GroupService.save(**group_data)
        
        # Get created group
        success: bool
        group: Optional[Group]
        success, group = GroupService.get_by_id(group_id)
        
        if not success or not group:
            return get_data_error_result(message="Failed to create group.")
        
        return get_json_result(
            data=group.to_dict(),
            message=f"Group '{name}' created successfully!",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


@manager.route("/<group_id>", methods=["DELETE"])  # noqa: F821
@login_required
def delete_group(group_id: str) -> Response:
    """Delete a group.
    
    Only team owners or admins can delete groups.
    This will also remove all user-group relationships for this group.
    
    ---
    tags:
      - Group
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: group_id
        required: true
        type: string
        description: Group ID
    responses:
      200:
        description: Group deleted successfully.
        schema:
          type: object
          properties:
            data:
              type: boolean
              description: Deletion success status.
            message:
              type: string
              description: Success message.
      401:
        description: Unauthorized.
      403:
        description: Forbidden - not team owner or admin.
      404:
        description: Group not found.
    """
    # Get group and verify it exists
    success: bool
    group: Optional[Group]
    success, group = GroupService.get_by_id(group_id)
    
    if not success or not group:
        return get_data_error_result(message="Group not found.")
    
    # Check if user is team owner or admin
    if not is_team_admin_or_owner(group.tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message="Only team owners or admins can delete groups.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    try:
        # Soft delete the group and all related group_user records
        with DB.connection_context():
            # Soft delete all user-group relationships for this group
            GroupUser.update({"status": StatusEnum.INVALID.value}).where(
                (GroupUser.group_id == group_id) &
                (GroupUser.status == StatusEnum.VALID.value)
            ).execute()
            
            # Soft delete the group itself
            Group.update({
                "status": StatusEnum.INVALID.value,
                "update_time": current_timestamp(),
                "update_date": datetime_format(datetime.now()),
            }).where(
                (Group.id == group_id) &
                (Group.status == StatusEnum.VALID.value)
            ).execute()
        
        return get_json_result(
            data=True,
            message="Group and all its member relationships deleted successfully!",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


@manager.route("/<group_id>/members/add", methods=["POST"])  # noqa: F821
@login_required
@validate_request("user_ids")
def add_members(group_id: str) -> Response:
    """Add members to a group.
    
    Users must be members of the team (tenant) that the group belongs to.
    Only team owners or admins can add members to groups.
    
    ---
    tags:
      - Group
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: group_id
        required: true
        type: string
        description: Group ID
      - in: body
        name: body
        required: true
        schema:
          type: object
          required:
            - user_ids
          properties:
            user_ids:
              type: array
              description: List of user IDs to add to the group.
              items:
                type: string
    responses:
      200:
        description: Members added successfully.
        schema:
          type: object
          properties:
            data:
              type: object
              properties:
                added:
                  type: array
                  description: Successfully added user IDs
                failed:
                  type: array
                  description: Users that failed to be added with error messages
            message:
              type: string
      400:
        description: Invalid request
      401:
        description: Unauthorized
      403:
        description: Forbidden - not team owner or admin
    """
    if request.json is None:
        return get_json_result(
            data=False,
            message="Request body is required!",
            code=RetCode.ARGUMENT_ERROR,
        )
    
    req: Dict[str, Any] = request.json
    user_ids: List[str] = req.get("user_ids", [])
    
    if not isinstance(user_ids, list) or len(user_ids) == 0:
        return get_json_result(
            data=False,
            message="'user_ids' must be a non-empty array.",
            code=RetCode.ARGUMENT_ERROR,
        )
    
    # Get group and verify it exists
    success: bool
    group: Optional[Group]
    success, group = GroupService.get_by_id(group_id)
    
    if not success or not group:
        return get_data_error_result(message="Group not found.")
    
    # Check if user is team owner or admin
    if not is_team_admin_or_owner(group.tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message="Only team owners or admins can add members to groups.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    added_user_ids: List[str] = []
    failed_users: List[Dict[str, Any]] = []
    
    try:
        for user_id in user_ids:
            if not isinstance(user_id, str) or not user_id.strip():
                failed_users.append({
                    "user_id": user_id,
                    "error": "Invalid user ID format."
                })
                continue
            
            user_id = user_id.strip()
            
            # Verify user exists
            user_exists: bool
            user: Optional[User]
            user_exists, user = UserService.get_by_id(user_id)
            
            if not user_exists or not user:
                failed_users.append({
                    "user_id": user_id,
                    "error": "User not found."
                })
                continue
            
            # Verify user is a member of the team
            if not is_team_member(group.tenant_id, user_id):
                failed_users.append({
                    "user_id": user_id,
                    "error": f"User {user.email} is not a member of the team."
                })
                continue
            
            # Check if user is already in the group
            existing_member: Optional[GroupUser] = GroupUserService.filter_by_group_and_user_id(
                group_id, user_id
            )
            
            if existing_member and existing_member.status == StatusEnum.VALID.value:
                failed_users.append({
                    "user_id": user_id,
                    "error": f"User {user.email} is already a member of this group."
                })
                continue
            
            # Add user to group
            try:
                GroupUserService.save(
                    id=get_uuid(),
                    group_id=group_id,
                    user_id=user_id,
                    status=StatusEnum.VALID.value,
                )
                added_user_ids.append(user_id)
            except Exception as e:
                logging.exception(e)
                failed_users.append({
                    "user_id": user_id,
                    "error": f"Failed to add user: {str(e)}"
                })
        
        return get_json_result(
            data={
                "added": added_user_ids,
                "failed": failed_users,
            },
            message=f"Added {len(added_user_ids)} member(s) to group.",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


@manager.route("/<group_id>/members/<user_id>", methods=["DELETE"])  # noqa: F821
@login_required
def remove_member(group_id: str, user_id: str) -> Response:
    """Remove a user from a group.
    
    Only team owners or admins can remove members from groups.
    
    ---
    tags:
      - Group
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: group_id
        required: true
        type: string
        description: Group ID
      - in: path
        name: user_id
        required: true
        type: string
        description: User ID to remove
    responses:
      200:
        description: User removed successfully.
        schema:
          type: object
          properties:
            data:
              type: boolean
              description: Removal success status.
            message:
              type: string
              description: Success message.
      400:
        description: Invalid request or user not found in group.
      401:
        description: Unauthorized
      403:
        description: Forbidden - not team owner or admin
    """
    # Get group and verify it exists
    success: bool
    group: Optional[Group]
    success, group = GroupService.get_by_id(group_id)
    
    if not success or not group:
        return get_data_error_result(message="Group not found.")
    
    # Check if user is team owner or admin
    if not is_team_admin_or_owner(group.tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message="Only team owners or admins can remove members from groups.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    try:
        # Check if user is in the group
        group_user: Optional[GroupUser] = GroupUserService.filter_by_group_and_user_id(
            group_id, user_id
        )
        
        if not group_user:
            return get_data_error_result(
                message="User is not a member of this group."
            )
        
        # Soft delete by setting status to invalid
        with DB.connection_context():
            GroupUser.update({"status": StatusEnum.INVALID.value}).where(
                (GroupUser.id == group_user.id)
            ).execute()
        
        return get_json_result(
            data=True,
            message="User removed from group successfully!",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


@manager.route("/<group_id>/members", methods=["GET"])  # noqa: F821
@login_required
def list_members(group_id: str) -> Response:
    """List all users in a group.
    
    Any team member can list users from any group in their team.
    
    ---
    tags:
      - Group
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: group_id
        required: true
        type: string
        description: Group ID
    responses:
      200:
        description: Group members retrieved successfully.
        schema:
          type: object
          properties:
            data:
              type: array
              items:
                type: object
                properties:
                  id:
                    type: string
                    description: User-group relationship ID.
                  user_id:
                    type: string
                    description: User ID.
                  status:
                    type: string
                    description: Relationship status.
                  nickname:
                    type: string
                    description: User nickname.
                  email:
                    type: string
                    description: User email.
                  avatar:
                    type: string
                    description: User avatar.
                  is_active:
                    type: boolean
                    description: Whether user is active.
            message:
              type: string
              description: Success message.
      401:
        description: Unauthorized.
      403:
        description: Forbidden - not a team member.
      404:
        description: Group not found.
    """
    # Get group and verify it exists
    success, group = GroupService.get_by_id(group_id)
    
    if not success or not group:
        return get_data_error_result(message="Group not found.")
    
    # Check if user is a member of the team (any team member can view group members)
    if not is_team_member(group.tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message="You must be a member of the team to view group members.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    try:
        # Get all users in the group
        members: List[Dict[str, Any]] = GroupUserService.get_by_group_id(group_id)
        
        # Filter only valid members (status == VALID)
        valid_members: List[Dict[str, Any]] = [
            member for member in members
            if member.get("status") == StatusEnum.VALID.value
        ]
        
        return get_json_result(
            data=valid_members,
            message=f"Retrieved {len(valid_members)} member(s) from group.",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


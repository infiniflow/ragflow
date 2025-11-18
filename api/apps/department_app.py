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
"""Department management API endpoints."""

import logging
from typing import Any, Dict, List, Optional

from flask import Blueprint, Response, request
from flask_login import current_user, login_required

from api.db import UserTenantRole
from api.db.db_models import DB, Department, Tenant, User, UserDepartment, UserTenant
from api.db.services.user_service import (
    DepartmentService,
    TenantService,
    UserDepartmentService,
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

manager = Blueprint("department", __name__)


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
def create_department() -> Response:
    """Create a new department within a team.
    
    Only team owners or admins can create departments.
    
    ---
    tags:
      - Department
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
              description: Department name.
            tenant_id:
              type: string
              description: Team/tenant ID that the department belongs to.
            description:
              type: string
              description: Optional department description.
    responses:
      200:
        description: Department created successfully.
        schema:
          type: object
          properties:
            data:
              type: object
              description: Created department information.
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
            message="Department name cannot be empty!",
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
            message="Only team owners or admins can create departments.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    # Verify tenant exists
    tenants_query = TenantService.query(id=tenant_id, status=StatusEnum.VALID.value)
    tenants: List[Tenant] = list(tenants_query)
    if not tenants:
        return get_data_error_result(message="Team not found.")
    
    try:
        # Create department
        department_id: str = get_uuid()
        department_data: Dict[str, Any] = {
            "id": department_id,
            "tenant_id": tenant_id,
            "name": name,
            "description": description,
            "created_by": current_user.id,
            "status": StatusEnum.VALID.value,
        }
        
        DepartmentService.save(**department_data)
        
        # Get created department
        success: bool
        department: Optional[Department]
        success, department = DepartmentService.get_by_id(department_id)
        
        if not success or not department:
            return get_data_error_result(message="Failed to create department.")
        
        return get_json_result(
            data=department.to_dict(),
            message=f"Department '{name}' created successfully!",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


@manager.route("/<department_id>", methods=["PUT"])  # noqa: F821
@login_required
def update_department(department_id: str) -> Response:
    """Update a department's details.
    
    Only department members can update departments.
    
    ---
    tags:
      - Department
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: department_id
        required: true
        type: string
        description: Department ID
      - in: body
        name: body
        required: true
        schema:
          type: object
          properties:
            name:
              type: string
              description: Department name (optional).
            description:
              type: string
              description: Department description (optional).
    responses:
      200:
        description: Department updated successfully.
        schema:
          type: object
          properties:
            data:
              type: object
              description: Updated department information.
            message:
              type: string
              description: Success message.
      400:
        description: Invalid request.
      401:
        description: Unauthorized.
      403:
        description: Forbidden - not a department member.
      404:
        description: Department not found.
    """
    if request.json is None:
        return get_json_result(
            data=False,
            message="Request body is required!",
            code=RetCode.ARGUMENT_ERROR,
        )
    
    # Get department and verify it exists
    success: bool
    department: Optional[Department]
    success, department = DepartmentService.get_by_id(department_id)
    
    if not success or not department:
        return get_data_error_result(message="Department not found.")
    
    # Check if user is a member of the department
    user_department: Optional[UserDepartment] = UserDepartmentService.filter_by_department_and_user_id(
        department_id, current_user.id
    )
    is_department_member: bool = (
        user_department is not None and 
        user_department.status == StatusEnum.VALID.value
    )
    
    # User must be a member of the department to update it
    if not is_department_member:
        return get_json_result(
            data=False,
            message="You must be a member of this department to update it.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    req: Dict[str, Any] = request.json
    update_data: Dict[str, Any] = {
        "update_time": current_timestamp(),
        "update_date": datetime_format(datetime.now()),
    }
    
    # Update name if provided
    if "name" in req:
        name: str = req.get("name", "").strip()
        if name:
            if len(name) > 128:
                return get_json_result(
                    data=False,
                    message="Department name must be 128 characters or less!",
                    code=RetCode.ARGUMENT_ERROR,
                )
            update_data["name"] = name
        else:
            return get_json_result(
                data=False,
                message="Department name cannot be empty!",
                code=RetCode.ARGUMENT_ERROR,
            )
    
    # Update description if provided
    if "description" in req:
        description: Optional[str] = req.get("description")
        if description is not None:
            description = description.strip() if isinstance(description, str) else None
            update_data["description"] = description if description else None
    
    # If no fields to update (only update_time and update_date were set), return error
    if len(update_data) == 2:  # Only update_time and update_date
        return get_json_result(
            data=False,
            message="No fields provided to update. Please provide 'name' and/or 'description'.",
            code=RetCode.ARGUMENT_ERROR,
        )
    
    try:
        # Update the department
        with DB.connection_context():
            Department.update(update_data).where(
                (Department.id == department_id) &
                (Department.status == StatusEnum.VALID.value)
            ).execute()
        
        # Get updated department
        success, updated_department = DepartmentService.get_by_id(department_id)
        
        if not success or not updated_department:
            return get_data_error_result(message="Department updated but could not retrieve updated data.")
        
        return get_json_result(
            data=updated_department.to_dict(),
            message="Department updated successfully!",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


@manager.route("/<department_id>", methods=["DELETE"])  # noqa: F821
@login_required
def delete_department(department_id: str) -> Response:
    """Delete a department.
    
    Only team owners or admins who are also department members can delete departments.
    This will also remove all user-department relationships for this department.
    
    ---
    tags:
      - Department
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: department_id
        required: true
        type: string
        description: Department ID
    responses:
      200:
        description: Department deleted successfully.
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
        description: Forbidden - not a department member or not team owner/admin.
      404:
        description: Department not found.
    """
    # Get department and verify it exists
    success: bool
    department: Optional[Department]
    success, department = DepartmentService.get_by_id(department_id)
    
    if not success or not department:
        return get_data_error_result(message="Department not found.")
    
    # Check if user is a member of the department
    user_department: Optional[UserDepartment] = UserDepartmentService.filter_by_department_and_user_id(
        department_id, current_user.id
    )
    is_department_member: bool = (
        user_department is not None and 
        user_department.status == StatusEnum.VALID.value
    )
    
    # User must be a member of the department to delete it
    if not is_department_member:
        return get_json_result(
            data=False,
            message="You must be a member of this department to delete it.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    # Additionally, user must be team owner or admin to delete
    if not is_team_admin_or_owner(department.tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message="Only team owners or admins can delete departments.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    try:
        # Soft delete the department and all related user_department records
        with DB.connection_context():
            # Soft delete all user-department relationships for this department
            UserDepartment.update({"status": StatusEnum.INVALID.value}).where(
                (UserDepartment.department_id == department_id) &
                (UserDepartment.status == StatusEnum.VALID.value)
            ).execute()
            
            # Soft delete the department itself
            Department.update({
                "status": StatusEnum.INVALID.value,
                "update_time": current_timestamp(),
                "update_date": datetime_format(datetime.now()),
            }).where(
                (Department.id == department_id) &
                (Department.status == StatusEnum.VALID.value)
            ).execute()
        
        return get_json_result(
            data=True,
            message="Department and all its member relationships deleted successfully!",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


@manager.route("/<department_id>/members/add", methods=["POST"])  # noqa: F821
@login_required
@validate_request("user_ids")
def add_members(department_id: str) -> Response:
    """Add members to a department.
    
    Users must be members of the team (tenant) that the department belongs to.
    Only team owners or admins can add members to departments.
    
    ---
    tags:
      - Department
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: department_id
        required: true
        type: string
        description: Department ID
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
              description: List of user IDs to add to the department.
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
    
    # Get department and verify it exists
    success: bool
    department: Optional[Department]
    success, department = DepartmentService.get_by_id(department_id)
    
    if not success or not department:
        return get_data_error_result(message="Department not found.")
    
    # Check if user is team owner or admin
    if not is_team_admin_or_owner(department.tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message="Only team owners or admins can add members to departments.",
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
            if not is_team_member(department.tenant_id, user_id):
                failed_users.append({
                    "user_id": user_id,
                    "error": f"User {user.email} is not a member of the team."
                })
                continue
            
            # Check if user is already in the department
            existing_member: Optional[UserDepartment] = UserDepartmentService.filter_by_department_and_user_id(
                department_id, user_id
            )
            
            if existing_member and existing_member.status == StatusEnum.VALID.value:
                failed_users.append({
                    "user_id": user_id,
                    "error": f"User {user.email} is already a member of this department."
                })
                continue
            
            # Add user to department
            try:
                UserDepartmentService.save(
                    id=get_uuid(),
                    department_id=department_id,
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
            message=f"Added {len(added_user_ids)} member(s) to department.",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


@manager.route("/<department_id>/members/<user_id>", methods=["DELETE"])  # noqa: F821
@login_required
def remove_member(department_id: str, user_id: str) -> Response:
    """Remove a user from a department.
    
    Only team owners or admins can remove members from departments.
    
    ---
    tags:
      - Department
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: department_id
        required: true
        type: string
        description: Department ID
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
        description: Invalid request or user not found in department.
      401:
        description: Unauthorized
      403:
        description: Forbidden - not team owner or admin
    """
    # Get department and verify it exists
    success: bool
    department: Optional[Department]
    success, department = DepartmentService.get_by_id(department_id)
    
    if not success or not department:
        return get_data_error_result(message="Department not found.")
    
    # Check if user is team owner or admin
    if not is_team_admin_or_owner(department.tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message="Only team owners or admins can remove members from departments.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    try:
        # Check if user is in the department
        user_department: Optional[UserDepartment] = UserDepartmentService.filter_by_department_and_user_id(
            department_id, user_id
        )
        
        if not user_department:
            return get_data_error_result(
                message="User is not a member of this department."
            )
        
        # Soft delete by setting status to invalid
        with DB.connection_context():
            UserDepartment.update({"status": StatusEnum.INVALID.value}).where(
                (UserDepartment.id == user_department.id)
            ).execute()
        
        return get_json_result(
            data=True,
            message="User removed from department successfully!",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


@manager.route("/<department_id>/members", methods=["GET"])  # noqa: F821
@login_required
def list_members(department_id: str) -> Response:
    """List all users in a department.
    
    Any team member can list users from any department in their team.
    
    ---
    tags:
      - Department
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: department_id
        required: true
        type: string
        description: Department ID
    responses:
      200:
        description: Department members retrieved successfully.
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
                    description: User-department relationship ID.
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
        description: Department not found.
    """
    # Get department and verify it exists
    success, department = DepartmentService.get_by_id(department_id)
    
    if not success or not department:
        return get_data_error_result(message="Department not found.")
    
    # Check if user is a member of the team (any team member can view department members)
    if not is_team_member(department.tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message="You must be a member of the team to view department members.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    try:
        # Get all users in the department
        members: List[Dict[str, Any]] = UserDepartmentService.get_by_department_id(department_id)
        
        # Filter only valid members (status == VALID)
        valid_members: List[Dict[str, Any]] = [
            member for member in members
            if member.get("status") == StatusEnum.VALID.value
        ]
        
        return get_json_result(
            data=valid_members,
            message=f"Retrieved {len(valid_members)} member(s) from department.",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


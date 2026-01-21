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
import logging
import asyncio
from typing import Annotated
from pydantic import BaseModel, ConfigDict, Field
from quart_schema import validate_request as qs_validate_request, validate_response, tag

from api.db import UserTenantRole
from api.db.db_models import UserTenant
from api.db.services.user_service import UserTenantService, UserService

from common.constants import RetCode, StatusEnum
from common.misc_utils import get_uuid
from common.time_utils import delta_seconds
from api.utils.api_utils import get_data_error_result, get_json_result, get_request_json, server_error_response, validate_request
from api.utils.web_utils import send_invite_email
from common import settings
from api.apps import login_required, current_user


# Pydantic Schemas for OpenAPI Documentation

class BaseSchema(BaseModel):
    """Base schema with common configuration.

    Designed for OpenAPI documentation generation without affecting
    existing request validation logic. Uses Pydantic v2 defaults.
    """
    model_config = ConfigDict(
        extra='ignore',      # Silently ignore extra fields
        strict=False,        # Allow type coercion
        validate_default=False,  # Don't validate defaults
        validate_assignment=False,  # Don't validate on assignment
        arbitrary_types_allowed=True  # Allow any Python type
    )

class CreateUserRequest(BaseSchema):
    """Request schema for creating/inviting a user to a tenant."""
    email: Annotated[str, Field(..., description="Email address of the user to invite", min_length=1)]


class CreateUserResponse(BaseModel):
    """Response schema for creating/inviting a user to a tenant."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(
        ...,
        description="Created user information including id, avatar, email, and nickname"
    )]
    message: Annotated[str, Field("Success", description="Response message")]


class UserListResponse(BaseModel):
    """Response schema for listing users in a tenant."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[list[dict], Field(
        ...,
        description="List of users with their details including id, email, nickname, "
                    "avatar, role, status, delta_seconds, and update_date"
    )]
    message: Annotated[str, Field("Success", description="Response message")]


class DeleteUserResponse(BaseModel):
    """Response schema for removing a user from a tenant."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="Deletion status, True if successful")]
    message: Annotated[str, Field("Success", description="Response message")]


class TenantListResponse(BaseModel):
    """Response schema for listing tenants for a user."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[list[dict], Field(
        ...,
        description="List of tenants with their details including tenant_id, tenant_name, "
                    "role, status, delta_seconds, and update_date"
    )]
    message: Annotated[str, Field("Success", description="Response message")]


class AgreeTenantResponse(BaseModel):
    """Response schema for agreeing to join a tenant."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="Agreement status, True if successful")]
    message: Annotated[str, Field("Success", description="Response message")]


# API Tag for grouping
tenant_tag = tag(["tenant"])


@manager.route("/<tenant_id>/user/list", methods=["GET"])  # noqa: F821
@login_required
@validate_response(200, UserListResponse)
@tenant_tag
def user_list(tenant_id):
    """
    List users in a tenant.

    Retrieves a list of all users associated with the specified tenant_id.
    Only the tenant owner (user whose ID matches tenant_id) can access this endpoint.
    Returns user information including their role, status, and time since last update.
    """
    if current_user.id != tenant_id:
        return get_json_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR)

    try:
        users = UserTenantService.get_by_tenant_id(tenant_id)
        for u in users:
            u["delta_seconds"] = delta_seconds(str(u["update_date"]))
        return get_json_result(data=users)
    except Exception as e:
        return server_error_response(e)


@manager.route('/<tenant_id>/user', methods=['POST'])  # noqa: F821
@login_required
@validate_request("email")
@qs_validate_request(CreateUserRequest)
@validate_response(200, CreateUserResponse)
@tenant_tag
async def create(tenant_id):
    """
    Invite a user to join a tenant.

    Invites a user (by email) to join the specified tenant. The user must already exist
    in the system. An invitation email will be sent to the user. The invited user will
    have an INVITE role initially and can accept the invitation to become a NORMAL member.
    Only the tenant owner can invite users.

    Returns error if:
    - The user email is not found in the system
    - The user is already a member of the tenant
    - The user is the owner of the tenant
    """
    if current_user.id != tenant_id:
        return get_json_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR)

    req = await get_request_json()
    invite_user_email = req["email"]
    invite_users = UserService.query(email=invite_user_email)
    if not invite_users:
        return get_data_error_result(message="User not found.")

    user_id_to_invite = invite_users[0].id
    user_tenants = UserTenantService.query(user_id=user_id_to_invite, tenant_id=tenant_id)
    if user_tenants:
        user_tenant_role = user_tenants[0].role
        if user_tenant_role == UserTenantRole.NORMAL:
            return get_data_error_result(message=f"{invite_user_email} is already in the team.")
        if user_tenant_role == UserTenantRole.OWNER:
            return get_data_error_result(message=f"{invite_user_email} is the owner of the team.")
        return get_data_error_result(
            message=f"{invite_user_email} is in the team, but the role: {user_tenant_role} is invalid.")

    UserTenantService.save(
        id=get_uuid(),
        user_id=user_id_to_invite,
        tenant_id=tenant_id,
        invited_by=current_user.id,
        role=UserTenantRole.INVITE,
        status=StatusEnum.VALID.value)

    try:

        user_name = ""
        _, user = UserService.get_by_id(current_user.id)
        if user:
            user_name = user.nickname

        asyncio.create_task(
            send_invite_email(
                to_email=invite_user_email,
                invite_url=settings.MAIL_FRONTEND_URL,
                tenant_id=tenant_id,
                inviter=user_name or current_user.email
            )
        )
    except Exception as e:
        logging.exception(f"Failed to send invite email to {invite_user_email}: {e}")
        return get_json_result(data=False, message="Failed to send invite email.", code=RetCode.SERVER_ERROR)
    usr = invite_users[0].to_dict()
    usr = {k: v for k, v in usr.items() if k in ["id", "avatar", "email", "nickname"]}

    return get_json_result(data=usr)


@manager.route('/<tenant_id>/user/<user_id>', methods=['DELETE'])  # noqa: F821
@login_required
@validate_response(200, DeleteUserResponse)
@tenant_tag
def rm(tenant_id, user_id):
    """
    Remove a user from a tenant.

    Removes the specified user from the tenant. This action can be performed by:
    - The tenant owner (current_user.id == tenant_id)
    - The user themselves (current_user.id == user_id)

    This operation is irreversible and will revoke the user's access to the tenant's resources.
    """
    if current_user.id != tenant_id and current_user.id != user_id:
        return get_json_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR)

    try:
        UserTenantService.filter_delete([UserTenant.tenant_id == tenant_id, UserTenant.user_id == user_id])
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route("/list", methods=["GET"])  # noqa: F821
@login_required
@validate_response(200, TenantListResponse)
@tenant_tag
def tenant_list():
    """
    List tenants for the current user.

    Retrieves a list of all tenants that the current user is a member of.
    Returns tenant information including the user's role, status, and time since last update.
    """
    try:
        users = UserTenantService.get_tenants_by_user_id(current_user.id)
        for u in users:
            u["delta_seconds"] = delta_seconds(str(u["update_date"]))
        return get_json_result(data=users)
    except Exception as e:
        return server_error_response(e)


@manager.route("/agree/<tenant_id>", methods=["PUT"])  # noqa: F821
@login_required
@validate_response(200, AgreeTenantResponse)
@tenant_tag
def agree(tenant_id):
    """
    Accept tenant invitation.

    Accepts an invitation to join a tenant. This endpoint should be called by a user
    who has been invited to join a tenant (with INVITE role). Upon accepting, the user's
    role will be updated to NORMAL, granting them full access to the tenant's resources.
    """
    try:
        UserTenantService.filter_update([UserTenant.tenant_id == tenant_id, UserTenant.user_id == current_user.id],
                                        {"role": UserTenantRole.NORMAL})
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)

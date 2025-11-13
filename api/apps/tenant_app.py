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
from typing import Any, Dict, Optional

from flask import Response, request
from flask_login import current_user, login_required

from api.apps import smtp_mail_server
from api.db import FileType, UserTenantRole
from api.db.db_models import UserTenant
from api.db.services.file_service import FileService
from api.db.services.llm_service import get_init_tenant_llm
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.user_service import (
    TenantService,
    UserService,
    UserTenantService,
)

from common.constants import RetCode, StatusEnum
from common.misc_utils import get_uuid
from common.time_utils import delta_seconds
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    server_error_response,
    validate_request,
)
from api.utils.web_utils import send_invite_email
from common import settings


def is_team_admin_or_owner(tenant_id: str, user_id: str) -> bool:
    """
    Check if a user is an OWNER or ADMIN of a team.
    
    Args:
        tenant_id: The team/tenant ID
        user_id: The user ID to check
        
    Returns:
        True if user is OWNER or ADMIN, False otherwise
    """
    user_tenant = UserTenantService.filter_by_tenant_and_user_id(tenant_id, user_id)
    if not user_tenant:
        return False
    return user_tenant.role in [UserTenantRole.OWNER, UserTenantRole.ADMIN]


@manager.route("/<tenant_id>/user/list", methods=["GET"])  # noqa: F821
@login_required
def user_list(tenant_id):
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
def create(tenant_id):
    if current_user.id != tenant_id:
        return get_json_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR)

    req = request.json
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

    if smtp_mail_server and settings.SMTP_CONF:
        from threading import Thread

        user_name = ""
        _, user = UserService.get_by_id(current_user.id)
        if user:
            user_name = user.nickname

        Thread(
            target=send_invite_email,
            args=(invite_user_email, settings.MAIL_FRONTEND_URL, tenant_id, user_name or current_user.email),
            daemon=True
        ).start()

    usr = invite_users[0].to_dict()
    usr = {k: v for k, v in usr.items() if k in ["id", "avatar", "email", "nickname"]}

    return get_json_result(data=usr)


@manager.route('/<tenant_id>/user/<user_id>', methods=['DELETE'])  # noqa: F821
@login_required
def rm(tenant_id, user_id):
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


@manager.route("/create", methods=["POST"])  # noqa: F821
@login_required
@validate_request("name")
def create_team() -> Response:
    """
    Create a new team (tenant). Requires authentication - any registered user can create a team.

    ---
    tags:
      - Team
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        description: Team creation details.
        required: true
        schema:
          type: object
          required:
            - name
          properties:
            name:
              type: string
              description: Team name.
            user_id:
              type: string
              description: User ID to set as team owner (optional, defaults to
                current authenticated user).
            llm_id:
              type: string
              description: LLM model ID (optional, defaults to system default).
            embd_id:
              type: string
              description: Embedding model ID (optional, defaults to system default).
            asr_id:
              type: string
              description: ASR model ID (optional, defaults to system default).
            parser_ids:
              type: string
              description: Document parser IDs (optional, defaults to system default).
            img2txt_id:
              type: string
              description: Image-to-text model ID (optional, defaults to system default).
            rerank_id:
              type: string
              description: Rerank model ID (optional, defaults to system default).
            credit:
              type: integer
              description: Initial credit amount (optional, defaults to 512).
    responses:
      200:
        description: Team created successfully.
        schema:
          type: object
          properties:
            data:
              type: object
              description: Created team information.
            message:
              type: string
              description: Success message.
      401:
        description: Unauthorized - authentication required.
        schema:
          type: object
      400:
        description: Invalid request or user not found.
        schema:
          type: object
      500:
        description: Server error during team creation.
        schema:
          type: object
    """
    # Explicitly check authentication status
    if not current_user.is_authenticated:
        return get_json_result(
            data=False,
            message="Unauthorized",
            code=RetCode.UNAUTHORIZED,
        )
    
    if request.json is None:
        return get_json_result(
            data=False,
            message="Request body is required!",
            code=RetCode.ARGUMENT_ERROR,
        )

    req: Dict[str, Any] = request.json
    team_name: str = req.get("name", "").strip()
    user_id: Optional[str] = req.get("user_id")
    
    # Optional configuration parameters (use defaults from settings if not provided)
    llm_id: Optional[str] = req.get("llm_id")
    embd_id: Optional[str] = req.get("embd_id")
    asr_id: Optional[str] = req.get("asr_id")
    parser_ids: Optional[str] = req.get("parser_ids")
    img2txt_id: Optional[str] = req.get("img2txt_id")
    rerank_id: Optional[str] = req.get("rerank_id")
    credit: Optional[int] = req.get("credit")

    # Validate team name
    if not team_name:
        return get_json_result(
            data=False,
            message="Team name is required!",
            code=RetCode.ARGUMENT_ERROR,
        )

    if len(team_name) > 100:
        return get_json_result(
            data=False,
            message="Team name must be 100 characters or less!",
            code=RetCode.ARGUMENT_ERROR,
        )

    # Determine user_id (use provided or current_user as default)
    owner_user_id: Optional[str] = user_id
    if not owner_user_id:
        # Use current authenticated user as default
        owner_user_id = current_user.id

    # Verify user exists
    user: Optional[Any] = UserService.filter_by_id(owner_user_id)
    if not user:
        return get_json_result(
            data=False,
            message=f"User with ID {owner_user_id} not found!",
            code=RetCode.DATA_ERROR,
        )

    # Validate that provided LLM models have been added by the user
    # Models are stored in TenantLLM with tenant_id = user_id
    def validate_model_id(user_id: str, model_id: Optional[str], model_type: str) -> Optional[str]:
        """Validate that a model ID has been added by the user. Returns error message if invalid, None if valid."""
        if model_id is None:
            return None  # Optional parameter, skip validation if not provided
        
        # Check if the model exists in TenantLLM for this user
        model_config = TenantLLMService.get_api_key(user_id, model_id)
        if not model_config:
            return f"{model_type} model '{model_id}' has not been added. Please add the model first before creating a team."
        
        # Check if the model is valid (status = "1")
        if model_config.status != StatusEnum.VALID.value:
            return f"{model_type} model '{model_id}' is not active. Please enable the model first."
        
        return None

    # Validate all provided model IDs
    validation_errors = []
    
    if llm_id is not None:
        error = validate_model_id(owner_user_id, llm_id, "LLM")
        if error:
            validation_errors.append(error)
    
    if embd_id is not None:
        error = validate_model_id(owner_user_id, embd_id, "Embedding")
        if error:
            validation_errors.append(error)
    
    if asr_id is not None:
        error = validate_model_id(owner_user_id, asr_id, "ASR")
        if error:
            validation_errors.append(error)
    
    if img2txt_id is not None:
        error = validate_model_id(owner_user_id, img2txt_id, "Image-to-text")
        if error:
            validation_errors.append(error)
    
    if rerank_id is not None:
        error = validate_model_id(owner_user_id, rerank_id, "Rerank")
        if error:
            validation_errors.append(error)
    
    if validation_errors:
        return get_json_result(
            data=False,
            message="; ".join(validation_errors),
            code=RetCode.DATA_ERROR,
        )

    # Generate tenant ID
    tenant_id: str = get_uuid()

    # Create tenant with optional parameters (use defaults from settings if not provided)
    tenant: Dict[str, Any] = {
        "id": tenant_id,
        "name": team_name,
        "llm_id": llm_id if llm_id is not None else settings.CHAT_MDL,
        "embd_id": embd_id if embd_id is not None else settings.EMBEDDING_MDL,
        "asr_id": asr_id if asr_id is not None else settings.ASR_MDL,
        "parser_ids": parser_ids if parser_ids is not None else settings.PARSERS,
        "img2txt_id": img2txt_id if img2txt_id is not None else settings.IMAGE2TEXT_MDL,
        "rerank_id": rerank_id if rerank_id is not None else settings.RERANK_MDL,
        "credit": credit if credit is not None else 512,
        "status": StatusEnum.VALID.value,
    }

    # Create user-tenant relationship
    usr_tenant: Dict[str, Any] = {
        "id": get_uuid(),
        "tenant_id": tenant_id,
        "user_id": owner_user_id,
        "invited_by": owner_user_id,
        "role": UserTenantRole.OWNER,
        "status": StatusEnum.VALID.value,
    }

    # Create root file folder
    file_id: str = get_uuid()
    file: Dict[str, Any] = {
        "id": file_id,
        "parent_id": file_id,
        "tenant_id": tenant_id,
        "created_by": owner_user_id,
        "name": "/",
        "type": FileType.FOLDER.value,
        "size": 0,
        "location": "",
    }

    try:
        # Get tenant LLM configurations
        tenant_llm: list[Dict[str, Any]] = get_init_tenant_llm(tenant_id)

        # Insert all records
        TenantService.insert(**tenant)
        UserTenantService.insert(**usr_tenant)
        TenantLLMService.insert_many(tenant_llm)
        FileService.insert(file)

        # Return created team info
        team_data: Dict[str, Any] = {
            "id": tenant_id,
            "name": team_name,
            "owner_id": owner_user_id,
            "llm_id": tenant["llm_id"],
            "embd_id": tenant["embd_id"],
        }

        return get_json_result(
            data=team_data,
            message=f"Team '{team_name}' created successfully!",
        )
    except Exception as e:
        logging.exception(e)
        # Rollback on error
        try:
            TenantService.delete_by_id(tenant_id)
        except Exception:
            pass
        try:
            UserTenantService.filter_delete(
                [
                    UserTenant.tenant_id == tenant_id,
                    UserTenant.user_id == owner_user_id,
                ]
            )
        except Exception:
            pass
        try:
            TenantLLMService.delete_by_tenant_id(tenant_id)
        except Exception:
            pass
        try:
            FileService.delete_by_id(file_id)
        except Exception:
            pass

        return get_json_result(
            data=False,
            message=f"Team creation failure, error: {str(e)}",
            code=RetCode.EXCEPTION_ERROR,
        )


@manager.route("/list", methods=["GET"])  # noqa: F821
@login_required
def tenant_list():
    try:
        users = UserTenantService.get_tenants_by_user_id(current_user.id)
        for u in users:
            u["delta_seconds"] = delta_seconds(str(u["update_date"]))
        return get_json_result(data=users)
    except Exception as e:
        return server_error_response(e)


@manager.route("/update-request/<tenant_id>", methods=["PUT"])  # noqa: F821
@login_required
def update_request(tenant_id):
    """
    Accept or reject a team invitation. User must have INVITE role.
    Takes an 'accept' boolean in the request body to accept (true) or reject (false) the invitation.
    
    ---
    tags:
      - Team
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: tenant_id
        required: true
        type: string
        description: Team ID
      - in: body
        name: body
        required: true
        schema:
          type: object
          required:
            - accept
          properties:
            accept:
              type: boolean
              description: true to accept the invitation, false to reject it
            role:
              type: string
              description: Role to assign after acceptance (normal, admin). Only used when accept=true. Defaults to normal.
              enum: [normal, admin]
    responses:
      200:
        description: Invitation processed successfully
      400:
        description: Invalid request
      401:
        description: Unauthorized
      404:
        description: Invitation not found
    """
    try:
        # Check if user has an invitation for this team
        user_tenant = UserTenantService.filter_by_tenant_and_user_id(tenant_id, current_user.id)
        if not user_tenant:
            return get_json_result(
                data=False,
                message="No invitation found for this team.",
                code=RetCode.DATA_ERROR
            )
        
        # Only allow processing if user has INVITE role
        if user_tenant.role != UserTenantRole.INVITE:
            return get_json_result(
                data=False,
                message=f"Cannot process invitation. Current role is '{user_tenant.role}', expected 'invite'.",
                code=RetCode.DATA_ERROR
            )
        
        # Get accept boolean from request body
        req = request.json or {}
        accept = req.get("accept")
        
        # Validate accept parameter
        if accept is None:
            return get_json_result(
                data=False,
                message="'accept' parameter is required in request body (true to accept, false to reject).",
                code=RetCode.ARGUMENT_ERROR
            )
        
        if not isinstance(accept, bool):
            return get_json_result(
                data=False,
                message="'accept' must be a boolean value (true or false).",
                code=RetCode.ARGUMENT_ERROR
            )
        
        if accept:
            # Accept invitation - update role from INVITE to the specified role
            role = UserTenantRole.NORMAL.value
             
            # Update role from INVITE to the specified role (defaults to NORMAL)
            UserTenantService.filter_update(
                [UserTenant.tenant_id == tenant_id, UserTenant.user_id == current_user.id],
                {"role": role, "status": StatusEnum.VALID.value}
            )
            return get_json_result(data=True, message=f"Successfully joined the team with role '{role}'.")
        else:
            # Reject invitation - delete the user-tenant relationship
            UserTenantService.filter_delete([
                UserTenant.tenant_id == tenant_id,
                UserTenant.user_id == current_user.id
            ])
            return get_json_result(data=True, message="Invitation rejected successfully.")
    except Exception as e:
        return server_error_response(e)


@manager.route('/<tenant_id>/users/add', methods=['POST'])  # noqa: F821
@login_required
@validate_request("users")
def add_users(tenant_id):
    """
    Send invitations to one or more users to join a team. Only OWNER or ADMIN can send invitations.
    Users must accept the invitation before they are added to the team.
    Supports both single user and bulk operations.
    
    ---
    tags:
      - Team
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: tenant_id
        required: true
        type: string
        description: Team ID
      - in: body
        name: body
        required: true
        schema:
          type: object
          required:
            - users
          properties:
            users:
              type: array
              description: List of users to add. Each user can be an email string or an object with email and role.
              items:
                oneOf:
                  - type: string
                    description: User email (will be added with 'normal' role)
                  - type: object
                    properties:
                      email:
                        type: string
                        description: User email
                      role:
                        type: string
                        description: Role to assign (normal, admin). Defaults to normal.
                        enum: [normal, admin]
    responses:
      200:
        description: Users added successfully
        schema:
          type: object
          properties:
            data:
              type: object
              properties:
                added:
                  type: array
                  description: Successfully added users
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
        description: Forbidden - not owner or admin
    """
    # Check if current user is OWNER or ADMIN of the team
    if not is_team_admin_or_owner(tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message='Only team owners or admins can add users.',
            code=RetCode.PERMISSION_ERROR
        )
    
    req = request.json
    users_input = req.get("users", [])
    
    if not isinstance(users_input, list) or len(users_input) == 0:
        return get_json_result(
            data=False,
            message="'users' must be a non-empty array.",
            code=RetCode.ARGUMENT_ERROR
        )
    
    added_users = []
    failed_users = []
    
    for user_input in users_input:
        # Handle both string (email) and object formats
        if isinstance(user_input, str):
            email = user_input
            role = UserTenantRole.NORMAL.value
        elif isinstance(user_input, dict):
            email = user_input.get("email")
            role = user_input.get("role", UserTenantRole.NORMAL.value)
        else:
            failed_users.append({
                "email": str(user_input),
                "error": "Invalid format. Must be a string (email) or object with 'email' and optional 'role'."
            })
            continue
        
        if not email:
            failed_users.append({
                "email": str(user_input),
                "error": "Email is required."
            })
            continue
        
        # Validate role
        if role not in [UserTenantRole.NORMAL.value, UserTenantRole.ADMIN.value]:
            failed_users.append({
                "email": email,
                "error": f"Invalid role '{role}'. Allowed roles: {UserTenantRole.NORMAL.value}, {UserTenantRole.ADMIN.value}"
            })
            continue
        
        try:
            # Find user by email
            invite_users = UserService.query(email=email)
            if not invite_users:
                failed_users.append({
                    "email": email,
                    "error": f"User with email '{email}' not found."
                })
                continue
            
            user_id_to_add = invite_users[0].id
            
            # Check if user is already in the team
            existing_user_tenants = UserTenantService.query(user_id=user_id_to_add, tenant_id=tenant_id)
            if existing_user_tenants:
                existing_role = existing_user_tenants[0].role
                if existing_role in [UserTenantRole.NORMAL, UserTenantRole.ADMIN]:
                    failed_users.append({
                        "email": email,
                        "error": f"User is already a member of the team with role '{existing_role}'."
                    })
                    continue
                if existing_role == UserTenantRole.OWNER:
                    failed_users.append({
                        "email": email,
                        "error": "User is the owner of the team and cannot be added again."
                    })
                    continue
                # If user has INVITE role, resend invitation with new role (update the invitation)
                if existing_role == UserTenantRole.INVITE:
                    # Update invitation - keep INVITE role, user needs to accept again
                    # Note: The intended role will be applied when user accepts via /agree endpoint
                    # For now, we'll store it by updating the invitation (user will need to accept)
                    usr = invite_users[0].to_dict()
                    usr = {k: v for k, v in usr.items() if k in ["id", "avatar", "email", "nickname"]}
                    usr["role"] = "invite"  # Still pending acceptance
                    usr["intended_role"] = role  # Store intended role for reference
                    added_users.append({
                        "email": email,
                        "status": "invitation_resent",
                        "intended_role": role
                    })
                    continue
            
            # Send invitation - create user with INVITE role (user must accept to join)
            UserTenantService.save(
                id=get_uuid(),
                user_id=user_id_to_add,
                tenant_id=tenant_id,
                invited_by=current_user.id,
                role=UserTenantRole.INVITE,  # Start with INVITE role
                status=StatusEnum.VALID.value
            )
            
            # Send invitation email if configured
            if smtp_mail_server and settings.SMTP_CONF:
                from threading import Thread
                user_name = ""
                _, user = UserService.get_by_id(current_user.id)
                if user:
                    user_name = user.nickname
                Thread(
                    target=send_invite_email,
                    args=(email, settings.MAIL_FRONTEND_URL, tenant_id, user_name or current_user.email),
                    daemon=True
                ).start()
            
            usr = invite_users[0].to_dict()
            usr = {k: v for k, v in usr.items() if k in ["id", "avatar", "email", "nickname"]}
            usr["role"] = "invite"  # User is invited, not yet added
            usr["intended_role"] = role  # Role they will get after acceptance
            added_users.append(usr)
            
        except Exception as e:
            logging.exception(f"Error adding user {email}: {e}")
            failed_users.append({
                "email": email,
                "error": f"Failed to add user: {str(e)}"
            })
    
    result = {
        "added": added_users,
        "failed": failed_users
    }
    
    if failed_users and not added_users:
        return get_json_result(
            data=result,
            message=f"Failed to add all users. {len(failed_users)} error(s).",
            code=RetCode.DATA_ERROR
        )
    elif failed_users:
        return get_json_result(
            data=result,
            message=f"Sent {len(added_users)} invitation(s). {len(failed_users)} user(s) failed."
        )
    else:
        return get_json_result(
            data=result,
            message=f"Successfully sent {len(added_users)} invitation(s). Users must accept to join the team."
        )


@manager.route('/<tenant_id>/users/remove', methods=['POST'])  # noqa: F821
@login_required
@validate_request("user_ids")
def remove_users(tenant_id):
    """
    Remove one or more users from a team. Only OWNER or ADMIN can remove users.
    Owners cannot be removed. Supports both single user and bulk operations.
    
    ---
    tags:
      - Team
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: tenant_id
        required: true
        type: string
        description: Team ID
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
              description: List of user IDs to remove
              items:
                type: string
    responses:
      200:
        description: Users removed successfully
        schema:
          type: object
          properties:
            data:
              type: object
              properties:
                removed:
                  type: array
                  description: Successfully removed user IDs
                failed:
                  type: array
                  description: Users that failed to be removed with error messages
            message:
              type: string
      400:
        description: Invalid request
      401:
        description: Unauthorized
      403:
        description: Forbidden - not owner or admin
    """
    # Check if current user is OWNER or ADMIN of the team
    if not is_team_admin_or_owner(tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message='Only team owners or admins can remove users.',
            code=RetCode.PERMISSION_ERROR
        )
    
    req = request.json
    user_ids = req.get("user_ids", [])
    
    if not isinstance(user_ids, list) or len(user_ids) == 0:
        return get_json_result(
            data=False,
            message="'user_ids' must be a non-empty array.",
            code=RetCode.ARGUMENT_ERROR
        )
    
    removed_users = []
    failed_users = []
    
    # Get all admins/owners for validation (check if removing would leave team without admin/owner)
    all_user_tenants = UserTenantService.query(tenant_id=tenant_id)
    admin_owner_ids = {
        ut.user_id for ut in all_user_tenants 
        if ut.role in [UserTenantRole.OWNER, UserTenantRole.ADMIN] and ut.status == StatusEnum.VALID.value
    }
    
    for user_id in user_ids:
        if not isinstance(user_id, str):
            failed_users.append({
                "user_id": str(user_id),
                "error": "Invalid user ID format."
            })
            continue
        
        try:
            # Check if user exists in the team
            user_tenant = UserTenantService.filter_by_tenant_and_user_id(tenant_id, user_id)
            if not user_tenant:
                failed_users.append({
                    "user_id": user_id,
                    "error": "User is not a member of this team."
                })
                continue
            
            # Prevent removing the owner
            if user_tenant.role == UserTenantRole.OWNER:
                failed_users.append({
                    "user_id": user_id,
                    "error": "Cannot remove the team owner."
                })
                continue
            
            # Prevent removing yourself if you're the only admin
            if user_id == current_user.id and user_tenant.role == UserTenantRole.ADMIN:
                remaining_admins = admin_owner_ids - {user_id}
                if len(remaining_admins) == 0:
                    failed_users.append({
                        "user_id": user_id,
                        "error": "Cannot remove yourself. At least one owner or admin must remain in the team."
                    })
                    continue
            
            # Remove user from team
            UserTenantService.filter_delete([
                UserTenant.tenant_id == tenant_id,
                UserTenant.user_id == user_id
            ])
            
            # Get user info for response
            user = UserService.filter_by_id(user_id)
            user_email = user.email if user else "Unknown"
            
            removed_users.append({
                "user_id": user_id,
                "email": user_email
            })
            
        except Exception as e:
            logging.exception(f"Error removing user {user_id}: {e}")
            failed_users.append({
                "user_id": user_id,
                "error": f"Failed to remove user: {str(e)}"
            })
    
    result = {
        "removed": removed_users,
        "failed": failed_users
    }
    
    if failed_users and not removed_users:
        return get_json_result(
            data=result,
            message=f"Failed to remove all users. {len(failed_users)} error(s).",
            code=RetCode.DATA_ERROR
        )
    elif failed_users:
        return get_json_result(
            data=result,
            message=f"Removed {len(removed_users)} user(s). {len(failed_users)} user(s) failed."
        )
    else:
        return get_json_result(
            data=result,
            message=f"Successfully removed {len(removed_users)} user(s)."
        )

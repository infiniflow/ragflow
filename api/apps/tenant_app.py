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
from threading import Thread
from typing import Any, Dict, List, Optional, Set, Union

from quart import request, Response
from api.db import UserTenantRole
from api.db.db_models import UserTenant
from api.db.services.user_service import UserTenantService, UserService

from api.apps import (
    current_user,
    login_required,
    smtp_mail_server,
)
from api.common.permission_utils import (
    get_user_permissions as get_member_permissions,
    update_user_permissions as update_member_permissions,
)
from api.db import FileType
from api.db.services.file_service import FileService
from api.db.services.llm_service import get_init_tenant_llm
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.user_service import (
    TenantService,
)
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    server_error_response,
    validate_request,
)
from api.utils.web_utils import send_invite_email
from common import settings
from common.constants import RetCode, StatusEnum
from common.misc_utils import get_uuid
from common.time_utils import delta_seconds
from api.utils.api_utils import get_request_json

# manager = Blueprint("tenant", __name__)
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


def is_team_owner(tenant_id: str, user_id: str) -> bool:
    """
    Check if a user is the OWNER of a team (not just admin).
    
    Args:
        tenant_id: The team/tenant ID
        user_id: The user ID to check
        
    Returns:
        True if user is OWNER, False otherwise
    """
    user_tenant = UserTenantService.filter_by_tenant_and_user_id(tenant_id, user_id)
    if not user_tenant:
        return False
    return user_tenant.role == UserTenantRole.OWNER


def validate_model_id(user_id: str, model_id: Optional[str], model_type: str, context: str = "team") -> Optional[str]:
    """
    Validate that a model ID has been added by the user. Returns error message if invalid, None if valid.
    
    Args:
        user_id: The user ID to check models for
        model_id: The model ID to validate (optional)
        model_type: The type of model (e.g., "LLM", "Embedding", "ASR")
        context: The context for the error message (e.g., "team", "creating a team", "updating the team")
        
    Returns:
        Error message string if invalid, None if valid
    """
    if model_id is None:
        return None  # Optional parameter, skip validation if not provided
    
    # Check if the model exists in TenantLLM for this user
    model_config = TenantLLMService.get_api_key(user_id, model_id)
    if not model_config:
        return f"{model_type} model '{model_id}' has not been added. Please add the model first before {context}."
    
    # Check if the model is valid (status = "1")
    if model_config.status != StatusEnum.VALID.value:
        return f"{model_type} model '{model_id}' is not active. Please enable the model first."
    
    return None



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
async def create(tenant_id):
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

    if smtp_mail_server and settings.SMTP_CONF:
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
async def create_team() -> Response:
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
    req_json = await request.json
    if req_json is None:
        return get_json_result(
            data=False,
            message="Request body is required!",
            code=RetCode.ARGUMENT_ERROR,
        )

    req: Dict[str, Any] = req_json
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

    # Validate credit value (must be non-negative if provided)
    if credit is not None and credit < 0:
        return get_json_result(
            data=False,
            message="Credit must be a non-negative integer!",
            code=RetCode.ARGUMENT_ERROR,
        )

    # Determine user_id (use provided or current_user as default)
    owner_user_id: Optional[str] = user_id
    if not owner_user_id:
        # Use current authenticated user as default
        if not current_user or not hasattr(current_user, 'id') or not current_user.id:
            return get_json_result(
                data=False,
                message="Unable to determine user ID. Please ensure you are authenticated.",
                code=RetCode.UNAUTHORIZED,
            )
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
    validation_errors = []
    
    if llm_id is not None:
        error = validate_model_id(owner_user_id, llm_id, "LLM", "creating a team")
        if error:
            validation_errors.append(error)
    
    if embd_id is not None:
        error = validate_model_id(owner_user_id, embd_id, "Embedding", "creating a team")
        if error:
            validation_errors.append(error)
    
    if asr_id is not None:
        error = validate_model_id(owner_user_id, asr_id, "ASR", "creating a team")
        if error:
            validation_errors.append(error)
    
    if img2txt_id is not None:
        error = validate_model_id(owner_user_id, img2txt_id, "Image-to-text", "creating a team")
        if error:
            validation_errors.append(error)
    
    if rerank_id is not None:
        error = validate_model_id(owner_user_id, rerank_id, "Rerank", "creating a team")
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


@manager.route("/<tenant_id>", methods=["PUT"])  # noqa: F821
@login_required
async def update_team(tenant_id: str) -> Response:
    """
    Update team details. Only OWNER or ADMIN can update team information.
    
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
          properties:
            name:
              type: string
              description: Team name (optional, max 100 characters).
            llm_id:
              type: string
              description: LLM model ID (optional, must be added by the user).
            embd_id:
              type: string
              description: Embedding model ID (optional, must be added by the user).
            asr_id:
              type: string
              description: ASR model ID (optional, must be added by the user).
            img2txt_id:
              type: string
              description: Image-to-text model ID (optional, must be added by the user).
            rerank_id:
              type: string
              description: Rerank model ID (optional, must be added by the user).
            tts_id:
              type: string
              description: TTS model ID (optional, must be added by the user).
            parser_ids:
              type: string
              description: Document parser IDs (optional).
            credit:
              type: integer
              description: Credit amount (optional).
    responses:
      200:
        description: Team updated successfully.
        schema:
          type: object
          properties:
            data:
              type: object
              description: Updated team information.
            message:
              type: string
              description: Success message.
      400:
        description: Invalid request.
      401:
        description: Unauthorized.
      403:
        description: Forbidden - not owner or admin.
      404:
        description: Team not found.
    """
    try:
        # Check if current user is OWNER or ADMIN of the team
        if not is_team_admin_or_owner(tenant_id, current_user.id):
            return get_json_result(
                data=False,
                message='Only team owners or admins can update team details.',
                code=RetCode.PERMISSION_ERROR
            )
        
        # Verify tenant exists
        success, tenant = TenantService.get_by_id(tenant_id)
        if not success or not tenant:
            return get_json_result(
                data=False,
                message=f"Team with ID '{tenant_id}' not found.",
                code=RetCode.DATA_ERROR
            )
        
        try:
            req_json = await request.json
        except Exception:
            # Handle malformed or missing JSON body explicitly
            return get_json_result(
                data=False,
                message="Request body is required!",
                code=RetCode.ARGUMENT_ERROR,
            )

        if req_json is None:
            return get_json_result(
                data=False,
                message="Request body is required!",
                code=RetCode.ARGUMENT_ERROR,
            )

        req: Dict[str, Any] = req_json
        
        # Extract update fields (all optional)
        update_data: Dict[str, Any] = {}
        
        # Update team name if provided
        if "name" in req:
            team_name: str = req.get("name", "").strip()
            if team_name:
                if len(team_name) > 100:
                    return get_json_result(
                        data=False,
                        message="Team name must be 100 characters or less!",
                        code=RetCode.ARGUMENT_ERROR,
                    )
                update_data["name"] = team_name
            else:
                return get_json_result(
                    data=False,
                    message="Team name cannot be empty!",
                    code=RetCode.ARGUMENT_ERROR,
                )
        
        # Validate model IDs if provided (reuse validate_model_id function)
        validation_errors: List[str] = []
        
        llm_id: Optional[str] = req.get("llm_id")
        if llm_id is not None:
            error = validate_model_id(current_user.id, llm_id, "LLM", "updating the team")
            if error:
                validation_errors.append(error)
            else:
                update_data["llm_id"] = llm_id
        
        embd_id: Optional[str] = req.get("embd_id")
        if embd_id is not None:
            error = validate_model_id(current_user.id, embd_id, "Embedding", "updating the team")
            if error:
                validation_errors.append(error)
            else:
                update_data["embd_id"] = embd_id
        
        asr_id: Optional[str] = req.get("asr_id")
        if asr_id is not None:
            error = validate_model_id(current_user.id, asr_id, "ASR", "updating the team")
            if error:
                validation_errors.append(error)
            else:
                update_data["asr_id"] = asr_id
        
        img2txt_id: Optional[str] = req.get("img2txt_id")
        if img2txt_id is not None:
            error = validate_model_id(current_user.id, img2txt_id, "Image-to-text", "updating the team")
            if error:
                validation_errors.append(error)
            else:
                update_data["img2txt_id"] = img2txt_id
        
        rerank_id: Optional[str] = req.get("rerank_id")
        if rerank_id is not None:
            error = validate_model_id(current_user.id, rerank_id, "Rerank", "updating the team")
            if error:
                validation_errors.append(error)
            else:
                update_data["rerank_id"] = rerank_id
        
        tts_id: Optional[str] = req.get("tts_id")
        if tts_id is not None:
            error = validate_model_id(current_user.id, tts_id, "TTS", "updating the team")
            if error:
                validation_errors.append(error)
            else:
                update_data["tts_id"] = tts_id
        
        parser_ids: Optional[str] = req.get("parser_ids")
        if parser_ids is not None:
            update_data["parser_ids"] = parser_ids
        
        credit: Optional[int] = req.get("credit")
        if credit is not None:
            if not isinstance(credit, int) or credit < 0:
                return get_json_result(
                    data=False,
                    message="Credit must be a non-negative integer!",
                    code=RetCode.ARGUMENT_ERROR,
                )
            update_data["credit"] = credit
        
        if validation_errors:
            return get_json_result(
                data=False,
                message="; ".join(validation_errors),
                code=RetCode.DATA_ERROR,
            )
        
        # Check if there's anything to update
        if not update_data:
            return get_json_result(
                data=False,
                message="No fields provided to update.",
                code=RetCode.ARGUMENT_ERROR,
            )
        
        # Update the tenant
        TenantService.update_by_id(tenant_id, update_data)
        
        # Get updated tenant info
        success, updated_tenant = TenantService.get_by_id(tenant_id)
        if not success or not updated_tenant:
            return get_json_result(
                data=False,
                message="Failed to retrieve updated team information.",
                code=RetCode.EXCEPTION_ERROR,
            )
        
        # Return updated team info
        team_data: Dict[str, Any] = {
            "id": updated_tenant.id,
            "name": updated_tenant.name,
            "llm_id": updated_tenant.llm_id,
            "embd_id": updated_tenant.embd_id,
            "asr_id": updated_tenant.asr_id,
            "img2txt_id": updated_tenant.img2txt_id,
            "rerank_id": updated_tenant.rerank_id,
            "tts_id": updated_tenant.tts_id,
            "parser_ids": updated_tenant.parser_ids,
            "credit": updated_tenant.credit,
        }
        
        return get_json_result(
            data=team_data,
            message="Team updated successfully!",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


@manager.route("/update-request/<tenant_id>", methods=["PUT"])  # noqa: F821
@login_required
async def update_request(tenant_id: str) -> Response:
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
        req_json = await request.json
        req: Dict[str, Any] = req_json if req_json is not None else {}
        accept: Optional[bool] = req.get("accept")
        
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
            role_input: Optional[str] = req.get("role")
            role: str = UserTenantRole.NORMAL.value
            
            # Validate and set role if provided
            if role_input is not None:
                if isinstance(role_input, str):
                    role_input = role_input.lower().strip()
                    if role_input in [UserTenantRole.NORMAL.value, UserTenantRole.ADMIN.value]:
                        role = role_input
                    else:
                        return get_json_result(
                            data=False,
                            message=f"Invalid role '{role_input}'. Allowed roles: {UserTenantRole.NORMAL.value}, {UserTenantRole.ADMIN.value}",
                            code=RetCode.ARGUMENT_ERROR
                        )
                else:
                    return get_json_result(
                        data=False,
                        message=f"Role must be a string. Allowed values: {UserTenantRole.NORMAL.value}, {UserTenantRole.ADMIN.value}",
                        code=RetCode.ARGUMENT_ERROR
                    )
            
            # Set default permissions: read-only access to datasets and canvases
            default_permissions: Dict[str, Any] = {
                "dataset": {"create": False, "read": True, "update": False, "delete": False},
                "canvas": {"create": False, "read": True, "update": False, "delete": False}
            }
             
            # Update role from INVITE to the specified role (defaults to NORMAL) and set default permissions
            UserTenantService.filter_update(
                [UserTenant.tenant_id == tenant_id, UserTenant.user_id == current_user.id],
                {"role": role, "status": StatusEnum.VALID.value, "permissions": default_permissions}
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
async def add_users(tenant_id: str) -> Response:
    """
    Add one or more users directly to a team. Only OWNER or ADMIN can add users.
    Users are added immediately without requiring invitation acceptance.
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
                        description: Role to assign (normal, admin, invite). Defaults to normal.
                        enum: [normal, admin, invite]
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
    
    req_json = await request.json
    req: Dict[str, Any] = req_json if req_json is not None else {}
    users_input: List[Union[str, Dict[str, Any]]] = req.get("users", [])
    
    if not isinstance(users_input, list) or len(users_input) == 0:
        return get_json_result(
            data=False,
            message="'users' must be a non-empty array.",
            code=RetCode.ARGUMENT_ERROR
        )
    
    added_users: List[Dict[str, Any]] = []
    failed_users: List[Dict[str, Any]] = []
    
    for user_input in users_input:
        # Handle both string (email) and object formats
        email: Optional[str] = None
        role: str = UserTenantRole.NORMAL.value
        if isinstance(user_input, str):
            email = user_input
            role = UserTenantRole.NORMAL.value
        elif isinstance(user_input, dict):
            email = user_input.get("email")
            role_input = user_input.get("role")
            # Normalize role to lowercase and validate
            if role_input is not None:
                if isinstance(role_input, str):
                    role_input = role_input.lower().strip()
                    if role_input in [UserTenantRole.NORMAL.value, UserTenantRole.ADMIN.value, UserTenantRole.INVITE.value]:
                        role = role_input
                    else:
                        failed_users.append({
                            "email": email or str(user_input),
                            "error": f"Invalid role '{role_input}'. Allowed roles: {UserTenantRole.NORMAL.value}, {UserTenantRole.ADMIN.value}, {UserTenantRole.INVITE.value}"
                        })
                        continue
                else:
                    failed_users.append({
                        "email": email or str(user_input),
                        "error": f"Role must be a string. Allowed values: {UserTenantRole.NORMAL.value}, {UserTenantRole.ADMIN.value}, {UserTenantRole.INVITE.value}"
                    })
                    continue
            # If role not provided, default to NORMAL
            else:
                role = UserTenantRole.NORMAL.value
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
        
        try:
            # Find user by email
            invite_users: List[Any] = UserService.query(email=email)
            if not invite_users:
                failed_users.append({
                    "email": email,
                    "error": f"User with email '{email}' not found."
                })
                continue
            
            user_id_to_add: str = invite_users[0].id
            
            # Check if user is already in the team
            existing_user_tenants: List[Any] = UserTenantService.query(user_id=user_id_to_add, tenant_id=tenant_id)
            if existing_user_tenants:
                existing_role: Any = existing_user_tenants[0].role
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
                # If user has INVITE role, convert it to the actual role (forcefully add them)
                if existing_role == UserTenantRole.INVITE:
                    # Set default permissions: read-only access to datasets and canvases
                    default_permissions: Dict[str, Any] = {
                        "dataset": {"create": False, "read": True, "update": False, "delete": False},
                        "canvas": {"create": False, "read": True, "update": False, "delete": False}
                    }
                    # Update from INVITE to the specified role
                    UserTenantService.filter_update(
                        [UserTenant.tenant_id == tenant_id, UserTenant.user_id == user_id_to_add],
                        {"role": role, "status": StatusEnum.VALID.value, "permissions": default_permissions}
                    )
                    usr: Dict[str, Any] = invite_users[0].to_dict()
                    usr = {k: v for k, v in usr.items() if k in ["id", "avatar", "email", "nickname"]}
                    usr["role"] = role
                    added_users.append(usr)
                    continue
            
            # Set default permissions: read-only access to datasets and canvases
            default_permissions: Dict[str, Any] = {
                "dataset": {"create": False, "read": True, "update": False, "delete": False},
                "canvas": {"create": False, "read": True, "update": False, "delete": False}
            }
            
            # Add user directly with the specified role (NORMAL, ADMIN, or INVITE)
            UserTenantService.save(
                id=get_uuid(),
                user_id=user_id_to_add,
                tenant_id=tenant_id,
                invited_by=current_user.id,
                role=role,  # Directly assign the role (NORMAL, ADMIN, or INVITE)
                status=StatusEnum.VALID.value,
                permissions=default_permissions
            )
            
            usr: Dict[str, Any] = invite_users[0].to_dict()
            usr = {k: v for k, v in usr.items() if k in ["id", "avatar", "email", "nickname"]}
            usr["role"] = role
            added_users.append(usr)
            
        except Exception as e:
            logging.exception(f"Error adding user {email}: {e}")
            failed_users.append({
                "email": email,
                "error": f"Failed to add user: {str(e)}"
            })
    
    result: Dict[str, List[Dict[str, Any]]] = {
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
            message=f"Added {len(added_users)} user(s). {len(failed_users)} user(s) failed."
        )
    else:
        return get_json_result(
            data=result,
            message=f"Successfully added {len(added_users)} user(s) to the team."
        )


@manager.route('/<tenant_id>/user/remove', methods=['POST'])  # noqa: F821
@login_required
@validate_request("user_id")
async def remove_user(tenant_id: str) -> Response:
    """
    Remove a user from a team. Only OWNER or ADMIN can remove users.
    Owners cannot be removed.
    
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
            - user_id
          properties:
            user_id:
              type: string
              description: User ID to remove
    responses:
      200:
        description: User removed successfully
        schema:
          type: object
          properties:
            data:
              type: object
              properties:
                user_id:
                  type: string
                  description: Removed user ID
                email:
                  type: string
                  description: Removed user email
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
    
    req_json = await request.json
    req: Dict[str, Any] = req_json if req_json is not None else {}
    user_id: Optional[str] = req.get("user_id")
    
    if not user_id or not isinstance(user_id, str):
        return get_json_result(
            data=False,
            message="'user_id' must be a non-empty string.",
            code=RetCode.ARGUMENT_ERROR
        )
    
    try:
        # Check if user exists in the team
        user_tenant = UserTenantService.filter_by_tenant_and_user_id(tenant_id, user_id)
        if not user_tenant:
            return get_json_result(
                data=False,
                message="User is not a member of this team.",
                code=RetCode.DATA_ERROR
            )
        
        # Prevent removing the owner
        if user_tenant.role == UserTenantRole.OWNER:
            return get_json_result(
                data=False,
                message="Cannot remove the team owner.",
                code=RetCode.DATA_ERROR
            )
        
        # Get all admins/owners for validation (check if removing would leave team without admin/owner)
        all_user_tenants: List[Any] = UserTenantService.query(tenant_id=tenant_id)
        admin_owner_ids: Set[str] = {
            ut.user_id for ut in all_user_tenants 
            if ut.role in [UserTenantRole.OWNER, UserTenantRole.ADMIN] and ut.status == StatusEnum.VALID.value
        }
        
        # Prevent removing yourself if you're the only admin
        if user_id == current_user.id and user_tenant.role == UserTenantRole.ADMIN:
            remaining_admins: Set[str] = admin_owner_ids - {user_id}
            if len(remaining_admins) == 0:
                return get_json_result(
                    data=False,
                    message="Cannot remove yourself. At least one owner or admin must remain in the team.",
                    code=RetCode.DATA_ERROR
                )
        
        # Remove user from team
        UserTenantService.filter_delete([
            UserTenant.tenant_id == tenant_id,
            UserTenant.user_id == user_id
        ])
        
        # Get user info for response
        user: Optional[Any] = UserService.filter_by_id(user_id)
        user_email: str = user.email if user else "Unknown"
        
        return get_json_result(
            data={
                "user_id": user_id,
                "email": user_email
            },
            message="User removed successfully."
        )
        
    except Exception as e:
        logging.exception(f"Error removing user {user_id}: {e}")
        return get_json_result(
            data=False,
            message=f"Failed to remove user: {str(e)}",
            code=RetCode.EXCEPTION_ERROR
        )


@manager.route('/<tenant_id>/admin/<user_id>/promote', methods=['POST'])  # noqa: F821
@login_required
def promote_admin(tenant_id: str, user_id: str) -> Response:
    """Promote a team member to admin role.
    
    Only team owners can promote members to admin.
    Cannot promote the team owner (owner role is permanent).
    
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
      - in: path
        name: user_id
        required: true
        type: string
        description: User ID to promote to admin
    responses:
      200:
        description: User promoted to admin successfully.
        schema:
          type: object
          properties:
            data:
              type: boolean
              description: Success status.
            message:
              type: string
              description: Success message.
      400:
        description: Invalid request or user not found.
      401:
        description: Unauthorized.
      403:
        description: Forbidden - not team owner.
      404:
        description: User not found in team.
    """
    # Check if current user is team owner (only owners can promote admins)
    if not is_team_owner(tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message="Only team owners can promote members to admin.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    try:
        # Check if target user exists in the team
        user_tenant: Optional[UserTenant] = UserTenantService.filter_by_tenant_and_user_id(
            tenant_id, user_id
        )
        
        if not user_tenant:
            return get_json_result(
                data=False,
                message="User is not a member of this team.",
                code=RetCode.DATA_ERROR,
            )
        
        # Cannot promote the owner (owner role is permanent)
        if user_tenant.role == UserTenantRole.OWNER:
            return get_json_result(
                data=False,
                message="Cannot promote the team owner. Owner role is permanent.",
                code=RetCode.DATA_ERROR,
            )
        
        # Check if user is already an admin
        if user_tenant.role == UserTenantRole.ADMIN:
            # Get user info for response
            user: Optional[Any] = UserService.filter_by_id(user_id)
            user_email: str = user.email if user else "Unknown"
            return get_json_result(
                data=True,
                message=f"User {user_email} is already an admin.",
            )
        
        # Promote user to admin (update role from NORMAL or INVITE to ADMIN)
        UserTenantService.filter_update(
            [UserTenant.tenant_id == tenant_id, UserTenant.user_id == user_id],
            {"role": UserTenantRole.ADMIN.value}
        )
        
        # Get user info for response
        user: Optional[Any] = UserService.filter_by_id(user_id)
        user_email: str = user.email if user else "Unknown"
        
        return get_json_result(
            data=True,
            message=f"User {user_email} has been promoted to admin successfully!",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


@manager.route('/<tenant_id>/admin/<user_id>/demote', methods=['POST'])  # noqa: F821
@login_required
def demote_admin(tenant_id: str, user_id: str) -> Response:
    """Demote a team admin to normal member.
    
    Only team owners can demote admins.
    Cannot demote the team owner (owner role is permanent).
    Cannot demote yourself if you're the only admin/owner.
    
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
      - in: path
        name: user_id
        required: true
        type: string
        description: User ID to demote from admin
    responses:
      200:
        description: Admin demoted to normal member successfully.
        schema:
          type: object
          properties:
            data:
              type: boolean
              description: Success status.
            message:
              type: string
              description: Success message.
      400:
        description: Invalid request or user not found.
      401:
        description: Unauthorized.
      403:
        description: Forbidden - not team owner.
      404:
        description: User not found in team or not an admin.
    """
    try:
        # Check if target user exists in the team
        user_tenant: Optional[UserTenant] = UserTenantService.filter_by_tenant_and_user_id(
            tenant_id, user_id
        )

        if not user_tenant:
            return get_json_result(
                data=False,
                message="User is not a member of this team.",
                code=RetCode.DATA_ERROR,
            )

        # Cannot demote the owner (owner role is permanent)
        if user_tenant.role == UserTenantRole.OWNER:
            return get_json_result(
                data=False,
                message="Cannot demote the team owner. Owner role is permanent.",
                code=RetCode.DATA_ERROR,
            )

        # Self-demotion: allow admins to demote themselves with last-admin protection,
        # even if they are not the team owner.
        if user_id == current_user.id:
            if user_tenant.role != UserTenantRole.ADMIN:
                user: Optional[Any] = UserService.filter_by_id(user_id)
                user_email: str = user.email if user else "Unknown"
                return get_json_result(
                    data=False,
                    message=f"User {user_email} is not an admin. Only admins can be demoted.",
                    code=RetCode.DATA_ERROR,
                )

            # Get all admins and owners in the team
            all_admins_owners: List[UserTenant] = list(
                UserTenantService.model.select().where(
                    (UserTenant.tenant_id == tenant_id)
                    & (UserTenant.status == StatusEnum.VALID.value)
                    & (UserTenant.role.in_([UserTenantRole.OWNER, UserTenantRole.ADMIN]))
                )
            )

            # If this is the only admin/owner, prevent demotion
            if len(all_admins_owners) <= 1:
                return get_json_result(
                    data=False,
                    message="Cannot demote yourself. At least one owner or admin must remain in the team.",
                    code=RetCode.DATA_ERROR,
                )

            # Safe to demote self
            UserTenantService.filter_update(
                [UserTenant.tenant_id == tenant_id, UserTenant.user_id == user_id],
                {"role": UserTenantRole.NORMAL.value},
            )
        else:
            # Demoting someone else: only team owners can demote admins
            if not is_team_owner(tenant_id, current_user.id):
                return get_json_result(
                    data=False,
                    message="Only team owners can demote admins.",
                    code=RetCode.PERMISSION_ERROR,
                )

            if user_tenant.role != UserTenantRole.ADMIN:
                user: Optional[Any] = UserService.filter_by_id(user_id)
                user_email: str = user.email if user else "Unknown"
                return get_json_result(
                    data=False,
                    message=f"User {user_email} is not an admin. Only admins can be demoted.",
                    code=RetCode.DATA_ERROR,
                )

            UserTenantService.filter_update(
                [UserTenant.tenant_id == tenant_id, UserTenant.user_id == user_id],
                {"role": UserTenantRole.NORMAL.value},
            )

        # Get user info for response
        user: Optional[Any] = UserService.filter_by_id(user_id)
        user_email: str = user.email if user else "Unknown"

        return get_json_result(
            data=True,
            message=f"User {user_email} has been demoted to normal member successfully!",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


@manager.route('/<tenant_id>/users/<user_id>/permissions', methods=['GET'])  # noqa: F821
@login_required
def get_user_permissions(tenant_id: str, user_id: str) -> Response:
    """
    Get CRUD permissions for a team member.
    
    Only team owners or admins can view permissions.
    
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
      - in: path
        name: user_id
        required: true
        type: string
        description: User ID to get permissions for
    responses:
      200:
        description: Permissions retrieved successfully.
        schema:
          type: object
          properties:
            data:
              type: object
              properties:
                dataset:
                  type: object
                  properties:
                    create:
                      type: boolean
                    read:
                      type: boolean
                    update:
                      type: boolean
                    delete:
                      type: boolean
                canvas:
                  type: object
                  properties:
                    create:
                      type: boolean
                    read:
                      type: boolean
                    update:
                      type: boolean
                    delete:
                      type: boolean
            message:
              type: string
      401:
        description: Unauthorized.
      403:
        description: Forbidden - not team owner or admin.
      404:
        description: User not found in team.
    """
    # Check if current user is team owner or admin
    if not is_team_admin_or_owner(tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message="Only team owners or admins can view permissions.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    try:
        # Check if target user exists in the team
        user_tenant: Optional[UserTenant] = UserTenantService.filter_by_tenant_and_user_id(
            tenant_id, user_id
        )
        
        if not user_tenant:
            return get_json_result(
                data=False,
                message="User is not a member of this team.",
                code=RetCode.DATA_ERROR,
            )
        
        permissions: Dict[str, Dict[str, bool]] = get_member_permissions(tenant_id, user_id)
        
        return get_json_result(
            data=permissions,
            message="Permissions retrieved successfully.",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)


@manager.route('/<tenant_id>/users/<user_id>/permissions', methods=['PUT'])  # noqa: F821
@login_required
@validate_request("permissions")
async def update_user_permissions(tenant_id: str, user_id: str) -> Response:
    """
    Update CRUD permissions for a team member.
    
    Only team owners or admins can update permissions.
    Owners and admins always have full permissions and cannot be restricted.
    
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
      - in: path
        name: user_id
        required: true
        type: string
        description: User ID to update permissions for
      - in: body
        name: body
        required: true
        schema:
          type: object
          required:
            - permissions
          properties:
            permissions:
              type: object
              description: Permissions to update (only provided fields will be updated)
              properties:
                dataset:
                  type: object
                  properties:
                    create:
                      type: boolean
                    read:
                      type: boolean
                    update:
                      type: boolean
                    delete:
                      type: boolean
                canvas:
                  type: object
                  properties:
                    create:
                      type: boolean
                    read:
                      type: boolean
                    update:
                      type: boolean
                    delete:
                      type: boolean
    responses:
      200:
        description: Permissions updated successfully.
        schema:
          type: object
          properties:
            data:
              type: object
              description: Updated permissions
            message:
              type: string
      400:
        description: Invalid request.
      401:
        description: Unauthorized.
      403:
        description: Forbidden - not team owner or admin, or trying to restrict owner/admin.
      404:
        description: User not found in team.
    """
    # Check if current user is team owner or admin
    if not is_team_admin_or_owner(tenant_id, current_user.id):
        return get_json_result(
            data=False,
            message="Only team owners or admins can update permissions.",
            code=RetCode.PERMISSION_ERROR,
        )
    
    req_json = await request.json
    if req_json is None:
        return get_json_result(
            data=False,
            message="Request body is required!",
            code=RetCode.ARGUMENT_ERROR,
        )
    
    req: Dict[str, Any] = req_json
    permissions: Optional[Dict[str, Any]] = req.get("permissions")
    
    if not permissions or not isinstance(permissions, dict):
        return get_json_result(
            data=False,
            message="'permissions' must be a non-empty object.",
            code=RetCode.ARGUMENT_ERROR,
        )
    
    try:
        # Check if target user exists in the team
        user_tenant: Optional[UserTenant] = UserTenantService.filter_by_tenant_and_user_id(
            tenant_id, user_id
        )
        
        if not user_tenant:
            return get_json_result(
                data=False,
                message="User is not a member of this team.",
                code=RetCode.DATA_ERROR,
            )
        
        # Owners and admins always have full permissions - cannot be restricted
        if user_tenant.role in [UserTenantRole.OWNER, UserTenantRole.ADMIN]:
            return get_json_result(
                data=False,
                message="Cannot update permissions for team owners or admins. They always have full permissions.",
                code=RetCode.DATA_ERROR,
            )
        
        # Validate permissions structure
        valid_resource_types = {"dataset", "canvas"}
        valid_permissions = {"create", "read", "update", "delete"}
        
        validated_permissions: Dict[str, Dict[str, bool]] = {}
        for resource_type, perms in permissions.items():
            if resource_type not in valid_resource_types:
                return get_json_result(
                    data=False,
                    message=f"Invalid resource type '{resource_type}'. Must be one of: {', '.join(valid_resource_types)}",
                    code=RetCode.ARGUMENT_ERROR,
                )
            
            if not isinstance(perms, dict):
                return get_json_result(
                    data=False,
                    message=f"Permissions for '{resource_type}' must be an object.",
                    code=RetCode.ARGUMENT_ERROR,
                )
            
            validated_permissions[resource_type] = {}
            for perm_name, perm_value in perms.items():
                if perm_name not in valid_permissions:
                    return get_json_result(
                        data=False,
                        message=f"Invalid permission '{perm_name}' for '{resource_type}'. Must be one of: {', '.join(valid_permissions)}",
                        code=RetCode.ARGUMENT_ERROR,
                    )
                
                if not isinstance(perm_value, bool):
                    return get_json_result(
                        data=False,
                        message=f"Permission value for '{resource_type}.{perm_name}' must be a boolean.",
                        code=RetCode.ARGUMENT_ERROR,
                    )
                
                validated_permissions[resource_type][perm_name] = perm_value
        
        # Update permissions
        success: bool = update_member_permissions(tenant_id, user_id, validated_permissions)
        
        if not success:
            return get_json_result(
                data=False,
                message="Failed to update permissions.",
                code=RetCode.EXCEPTION_ERROR,
            )
        
        # Get updated permissions for response
        updated_permissions: Dict[str, Dict[str, bool]] = get_member_permissions(tenant_id, user_id)
        
        return get_json_result(
            data=updated_permissions,
            message="Permissions updated successfully.",
        )
    except Exception as e:
        logging.exception(e)
        return server_error_response(e)

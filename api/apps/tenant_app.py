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


@manager.route("/agree/<tenant_id>", methods=["PUT"])  # noqa: F821
@login_required
def agree(tenant_id):
    try:
        UserTenantService.filter_update([UserTenant.tenant_id == tenant_id, UserTenant.user_id == current_user.id],
                                        {"role": UserTenantRole.NORMAL})
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)

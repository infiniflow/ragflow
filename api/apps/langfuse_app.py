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

from typing import Annotated
from langfuse import Langfuse
from pydantic import BaseModel, ConfigDict, Field
from quart_schema import validate_request as qs_validate_request, validate_response, tag

from api.apps import current_user, login_required
from api.db.db_models import DB
from api.db.services.langfuse_service import TenantLangfuseService
from api.utils.api_utils import get_error_data_result, get_json_result, get_request_json, server_error_response, validate_request


# Pydantic Schemas for OpenAPI Documentation

class BaseSchema(BaseModel):
    """Base schema with common configuration."""
    model_config = ConfigDict(extra="ignore", strict=False)


class SetApiKeyRequest(BaseSchema):
    """Request schema for setting Langfuse API keys."""
    secret_key: Annotated[str, Field(..., description="Langfuse secret key", min_length=1)]
    public_key: Annotated[str, Field(..., description="Langfuse public key", min_length=1)]
    host: Annotated[str, Field(..., description="Langfuse host URL", min_length=1)]


class SetApiKeyResponse(BaseModel):
    """Response schema for setting Langfuse API keys."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Stored Langfuse configuration data")]
    message: Annotated[str, Field("Success", description="Response message")]


class GetApiKeyResponse(BaseModel):
    """Response schema for retrieving Langfuse API keys."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Langfuse configuration with project info")]
    message: Annotated[str, Field("Success", description="Response message")]


class DeleteApiKeyResponse(BaseModel):
    """Response schema for deleting Langfuse API keys."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="Deletion status")]
    message: Annotated[str, Field("Success", description="Response message")]


# Langfuse API Endpoints

langfuse_tag = tag(["langfuse"])


@manager.route("/api_key", methods=["POST", "PUT"])  # noqa: F821
@login_required
@validate_request("secret_key", "public_key", "host")
@qs_validate_request(SetApiKeyRequest)
@validate_response(200, SetApiKeyResponse)
@langfuse_tag
async def set_api_key():
    """
    Set or update Langfuse API keys.

    Configures Langfuse integration by storing the secret key, public key, and host URL.
    Validates the provided credentials before storing them. If keys already exist for
    the current tenant, they will be updated.

    The keys are validated against the Langfuse API before being saved.
    """
    req = await get_request_json()
    secret_key = req.get("secret_key", "")
    public_key = req.get("public_key", "")
    host = req.get("host", "")
    if not all([secret_key, public_key, host]):
        return get_error_data_result(message="Missing required fields")

    current_user_id = current_user.id
    langfuse_keys = dict(
        tenant_id=current_user_id,
        secret_key=secret_key,
        public_key=public_key,
        host=host,
    )

    langfuse = Langfuse(public_key=langfuse_keys["public_key"], secret_key=langfuse_keys["secret_key"], host=langfuse_keys["host"])
    if not langfuse.auth_check():
        return get_error_data_result(message="Invalid Langfuse keys")

    langfuse_entry = TenantLangfuseService.filter_by_tenant(tenant_id=current_user_id)
    with DB.atomic():
        try:
            if not langfuse_entry:
                TenantLangfuseService.save(**langfuse_keys)
            else:
                TenantLangfuseService.update_by_tenant(tenant_id=current_user_id, langfuse_keys=langfuse_keys)
            return get_json_result(data=langfuse_keys)
        except Exception as e:
            return server_error_response(e)


@manager.route("/api_key", methods=["GET"])  # noqa: F821
@login_required
@validate_request()
@validate_response(200, GetApiKeyResponse)
@langfuse_tag
def get_api_key():
    """
    Retrieve Langfuse API keys and project information.

    Fetches the stored Langfuse configuration for the current tenant and validates it.
    Returns the public key, host, and associated project information including project ID and name.
    Validates the credentials against the Langfuse API before returning.
    """
    current_user_id = current_user.id
    langfuse_entry = TenantLangfuseService.filter_by_tenant_with_info(tenant_id=current_user_id)
    if not langfuse_entry:
        return get_json_result(message="Have not record any Langfuse keys.")

    langfuse = Langfuse(public_key=langfuse_entry["public_key"], secret_key=langfuse_entry["secret_key"], host=langfuse_entry["host"])
    try:
        if not langfuse.auth_check():
            return get_error_data_result(message="Invalid Langfuse keys loaded")
    except langfuse.api.core.api_error.ApiError as api_err:
        return get_json_result(message=f"Error from Langfuse: {api_err}")
    except Exception as e:
        return server_error_response(e)

    langfuse_entry["project_id"] = langfuse.api.projects.get().dict()["data"][0]["id"]
    langfuse_entry["project_name"] = langfuse.api.projects.get().dict()["data"][0]["name"]

    return get_json_result(data=langfuse_entry)


@manager.route("/api_key", methods=["DELETE"])  # noqa: F821
@login_required
@validate_request()
@validate_response(200, DeleteApiKeyResponse)
@langfuse_tag
def delete_api_key():
    """
    Delete Langfuse API keys.

    Removes the stored Langfuse configuration for the current tenant.
    This action cannot be undone.
    """
    current_user_id = current_user.id
    langfuse_entry = TenantLangfuseService.filter_by_tenant(tenant_id=current_user_id)
    if not langfuse_entry:
        return get_json_result(message="Have not record any Langfuse keys.")

    with DB.atomic():
        try:
            TenantLangfuseService.delete_model(langfuse_entry)
            return get_json_result(data=True)
        except Exception as e:
            return server_error_response(e)

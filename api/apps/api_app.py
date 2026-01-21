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
from datetime import datetime, timedelta
from typing import Annotated
from quart import request
from pydantic import BaseModel, ConfigDict, Field
from quart_schema import validate_request as qs_validate_request, validate_response, tag

from api.db.db_models import APIToken
from api.db.services.api_service import APITokenService, API4ConversationService
from api.db.services.user_service import UserTenantService
from api.utils.api_utils import generate_confirmation_token, get_data_error_result, get_json_result, get_request_json, server_error_response, validate_request
from common.time_utils import current_timestamp, datetime_format
from api.apps import login_required, current_user


# Pydantic Schemas for OpenAPI Documentation

class BaseSchema(BaseModel):
    """Base schema with common configuration."""
    model_config = ConfigDict(extra="ignore", strict=False)


class NewTokenRequest(BaseSchema):
    """Request schema for creating a new API token."""
    dialog_id: Annotated[str, Field(..., description="Dialog ID associated with the token")]
    canvas_id: Annotated[str | None, Field(None, description="Canvas ID for agent tokens (alternative to dialog_id)")]


class NewTokenResponse(BaseModel):
    """Response schema for creating a new API token."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Created token data including token, tenant_id, dialog_id, etc.")]
    message: Annotated[str, Field("Success", description="Response message")]


class TokenListResponse(BaseModel):
    """Response schema for listing API tokens."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[list[dict], Field(..., description="List of API tokens with their details")]
    message: Annotated[str, Field("Success", description="Response message")]


class DeleteTokensRequest(BaseSchema):
    """Request schema for deleting API tokens."""
    tokens: Annotated[list[str], Field(..., description="List of token strings to delete")]
    tenant_id: Annotated[str, Field(..., description="Tenant ID owning the tokens")]


class DeleteTokensResponse(BaseModel):
    """Response schema for deleting API tokens."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="Deletion status, True if successful")]
    message: Annotated[str, Field("Success", description="Response message")]


class StatsResponse(BaseModel):
    """Response schema for API usage statistics."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(
        ...,
        description="Usage statistics containing pv (page views), uv (unique visitors), "
                    "speed (tokens per second), tokens (thousands), round (conversation rounds), "
                    "and thumb_up (user feedback) data points"
    )]
    message: Annotated[str, Field("Success", description="Response message")]


# API Tag for grouping
api_tag = tag(["api"])


@manager.route('/new_token', methods=['POST'])  # noqa: F821
@login_required
@qs_validate_request(NewTokenRequest)
@validate_response(200, NewTokenResponse)
@api_tag
async def new_token():
    """
    Create a new API token.

    Creates a new API token for the current user's tenant. The token can be associated
    with either a dialog or a canvas (agent). The generated token can be used to authenticate
    API requests for the associated dialog or agent.
    """
    req = await get_request_json()
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")

        tenant_id = tenants[0].tenant_id
        obj = {"tenant_id": tenant_id, "token": generate_confirmation_token(),
               "create_time": current_timestamp(),
               "create_date": datetime_format(datetime.now()),
               "update_time": None,
               "update_date": None
               }
        if req.get("canvas_id"):
            obj["dialog_id"] = req["canvas_id"]
            obj["source"] = "agent"
        else:
            obj["dialog_id"] = req["dialog_id"]

        if not APITokenService.save(**obj):
            return get_data_error_result(message="Fail to new a dialog!")

        return get_json_result(data=obj)
    except Exception as e:
        return server_error_response(e)


@manager.route('/token_list', methods=['GET'])  # noqa: F821
@login_required
@validate_response(200, TokenListResponse)
@api_tag
def token_list():
    """
    List API tokens for a dialog or canvas.

    Retrieves all API tokens associated with the specified dialog_id or canvas_id
    for the current user's tenant. Returns a list of tokens with their details.
    """
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")

        id = request.args["dialog_id"] if "dialog_id" in request.args else request.args["canvas_id"]
        objs = APITokenService.query(tenant_id=tenants[0].tenant_id, dialog_id=id)
        return get_json_result(data=[o.to_dict() for o in objs])
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['POST'])  # noqa: F821
@validate_request("tokens", "tenant_id")
@qs_validate_request(DeleteTokensRequest)
@validate_response(200, DeleteTokensResponse)
@api_tag
@login_required
async def rm():
    """
    Delete API tokens.

    Deletes the specified API tokens for the given tenant_id. This operation is irreversible
    and any applications using these tokens will no longer be able to authenticate.
    """
    req = await get_request_json()
    try:
        for token in req["tokens"]:
            APITokenService.filter_delete(
                [APIToken.tenant_id == req["tenant_id"], APIToken.token == token])
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/stats', methods=['GET'])  # noqa: F821
@login_required
@validate_response(200, StatsResponse)
@api_tag
def stats():
    """
    Get API usage statistics.

    Retrieves usage statistics for API conversations within a specified date range.
    Returns metrics including page views (pv), unique visitors (uv), processing speed,
    token consumption, conversation rounds, and user feedback (thumb_up).
    The date range defaults to the last 7 days if not specified.
    """
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")
        objs = API4ConversationService.stats(
            tenants[0].tenant_id,
            request.args.get(
                "from_date",
                (datetime.now() -
                 timedelta(
                     days=7)).strftime("%Y-%m-%d 00:00:00")),
            request.args.get(
                "to_date",
                datetime.now().strftime("%Y-%m-%d %H:%M:%S")),
            "agent" if "canvas_id" in request.args else None)

        res = {"pv": [], "uv": [], "speed": [], "tokens": [], "round": [], "thumb_up": []}

        for obj in objs:
            dt = obj["dt"]
            res["pv"].append((dt, obj["pv"]))
            res["uv"].append((dt, obj["uv"]))
            res["speed"].append((dt, float(obj["tokens"]) / (float(obj["duration"]) + 0.1))) # +0.1 to avoid division by zero
            res["tokens"].append((dt, float(obj["tokens"]) / 1000.0)) # convert to thousands
            res["round"].append((dt, obj["round"]))
            res["thumb_up"].append((dt, obj["thumb_up"]))

        return get_json_result(data=res)
    except Exception as e:
        return server_error_response(e)

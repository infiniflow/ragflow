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
from quart import request
from pydantic import BaseModel, ConfigDict, Field
from quart_schema import validate_request as qs_validate_request, validate_response, tag

from api.apps import current_user, login_required
from api.constants import DATASET_NAME_LIMIT
from api.db.db_models import DB
from api.db.services import duplicate_name
from api.db.services.search_service import SearchService
from api.db.services.user_service import TenantService, UserTenantService
from common.misc_utils import get_uuid
from common.constants import StatusEnum, RetCode
from api.utils.api_utils import get_data_error_result, get_json_result, not_allowed_parameters, get_request_json, server_error_response, validate_request


# Pydantic Schemas for OpenAPI Documentation

class BaseSchema(BaseModel):
    """Base schema with common configuration."""
    model_config = ConfigDict(extra="forbid", strict=True)


class CreateSearchRequest(BaseSchema):
    """Request schema for creating a search application."""
    name: Annotated[str, Field(..., description="Search application name", min_length=1, max_length=255)]
    description: Annotated[str | None, Field(None, description="Search application description", max_length=65535)]


class CreateSearchResponse(BaseModel):
    """Response schema for creating a search application."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Created search application data containing search_id")]
    message: Annotated[str, Field("Success", description="Response message")]


class UpdateSearchRequest(BaseSchema):
    """Request schema for updating a search application."""
    search_id: Annotated[str, Field(..., description="Search application ID to update")]
    name: Annotated[str, Field(..., description="Updated search application name", min_length=1, max_length=DATASET_NAME_LIMIT)]
    search_config: Annotated[dict, Field(..., description="Search configuration settings")]
    tenant_id: Annotated[str, Field(..., description="Tenant ID")]


class UpdateSearchResponse(BaseModel):
    """Response schema for updating a search application."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Updated search application data")]
    message: Annotated[str, Field("Success", description="Response message")]


class SearchDetailResponse(BaseModel):
    """Response schema for search application detail."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Search application detail data")]
    message: Annotated[str, Field("Success", description="Response message")]


class ListSearchRequest(BaseSchema):
    """Request schema for listing search applications."""
    owner_ids: Annotated[list[str] | None, Field(None, description="Filter by owner IDs")]


class ListSearchResponse(BaseModel):
    """Response schema for listing search applications."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="List of search applications with total count")]
    message: Annotated[str, Field("Success", description="Response message")]


class DeleteSearchRequest(BaseSchema):
    """Request schema for deleting a search application."""
    search_id: Annotated[str, Field(..., description="Search application ID to delete")]


class DeleteSearchResponse(BaseModel):
    """Response schema for deleting a search application."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="Deletion status")]
    message: Annotated[str, Field("Success", description="Response message")]


# Search API Endpoints

search_tag = tag("search", "Search Application Management APIs")


@manager.route("/create", methods=["POST"])  # noqa: F821
@login_required
@validate_request("name")
@qs_validate_request(CreateSearchRequest)
@validate_response(200, CreateSearchResponse)
@search_tag
async def create():
    """
    Create a new search application.

    Creates a new search application with the provided name and optional description.
    The search application ID will be automatically generated and returned in the response.
    """
    req = await get_request_json()
    search_name = req["name"]
    description = req.get("description", "")
    if not isinstance(search_name, str):
        return get_data_error_result(message="Search name must be string.")
    if search_name.strip() == "":
        return get_data_error_result(message="Search name can't be empty.")
    if len(search_name.encode("utf-8")) > 255:
        return get_data_error_result(message=f"Search name length is {len(search_name)} which is large than 255.")
    e, _ = TenantService.get_by_id(current_user.id)
    if not e:
        return get_data_error_result(message="Authorized identity.")

    search_name = search_name.strip()
    search_name = duplicate_name(SearchService.query, name=search_name, tenant_id=current_user.id, status=StatusEnum.VALID.value)

    req["id"] = get_uuid()
    req["name"] = search_name
    req["description"] = description
    req["tenant_id"] = current_user.id
    req["created_by"] = current_user.id
    with DB.atomic():
        try:
            if not SearchService.save(**req):
                return get_data_error_result()
            return get_json_result(data={"search_id": req["id"]})
        except Exception as e:
            return server_error_response(e)


@manager.route("/update", methods=["POST"])  # noqa: F821
@login_required
@validate_request("search_id", "name", "search_config", "tenant_id")
@not_allowed_parameters("id", "created_by", "create_time", "update_time", "create_date", "update_date", "created_by")
@qs_validate_request(UpdateSearchRequest)
@validate_response(200, UpdateSearchResponse)
@search_tag
async def update():
    """
    Update an existing search application.

    Updates the search application with new configuration settings.
    Only the owner of the search application can update it.
    """
    req = await get_request_json()
    if not isinstance(req["name"], str):
        return get_data_error_result(message="Search name must be string.")
    if req["name"].strip() == "":
        return get_data_error_result(message="Search name can't be empty.")
    if len(req["name"].encode("utf-8")) > DATASET_NAME_LIMIT:
        return get_data_error_result(message=f"Search name length is {len(req['name'])} which is large than {DATASET_NAME_LIMIT}")
    req["name"] = req["name"].strip()
    tenant_id = req["tenant_id"]
    e, _ = TenantService.get_by_id(tenant_id)
    if not e:
        return get_data_error_result(message="Authorized identity.")

    search_id = req["search_id"]
    if not SearchService.accessible4deletion(search_id, current_user.id):
        return get_json_result(data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)

    try:
        search_app = SearchService.query(tenant_id=tenant_id, id=search_id)[0]
        if not search_app:
            return get_json_result(data=False, message=f"Cannot find search {search_id}", code=RetCode.DATA_ERROR)

        if req["name"].lower() != search_app.name.lower() and len(SearchService.query(name=req["name"], tenant_id=tenant_id, status=StatusEnum.VALID.value)) >= 1:
            return get_data_error_result(message="Duplicated search name.")

        if "search_config" in req:
            current_config = search_app.search_config or {}
            new_config = req["search_config"]

            if not isinstance(new_config, dict):
                return get_data_error_result(message="search_config must be a JSON object")

            updated_config = {**current_config, **new_config}
            req["search_config"] = updated_config

        req.pop("search_id", None)
        req.pop("tenant_id", None)

        updated = SearchService.update_by_id(search_id, req)
        if not updated:
            return get_data_error_result(message="Failed to update search")

        e, updated_search = SearchService.get_by_id(search_id)
        if not e:
            return get_data_error_result(message="Failed to fetch updated search")

        return get_json_result(data=updated_search.to_dict())

    except Exception as e:
        return server_error_response(e)


@manager.route("/detail", methods=["GET"])  # noqa: F821
@login_required
@validate_response(200, SearchDetailResponse)
@search_tag
def detail():
    """
    Get search application details.

    Retrieves detailed information about a specific search application by its ID.
    The user must have permission to access this search application.
    """
    search_id = request.args["search_id"]
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        for tenant in tenants:
            if SearchService.query(tenant_id=tenant.tenant_id, id=search_id):
                break
        else:
            return get_json_result(data=False, message="Has no permission for this operation.", code=RetCode.OPERATING_ERROR)

        search = SearchService.get_detail(search_id)
        if not search:
            return get_data_error_result(message="Can't find this Search App!")
        return get_json_result(data=search)
    except Exception as e:
        return server_error_response(e)


@manager.route("/list", methods=["POST"])  # noqa: F821
@login_required
@qs_validate_request(ListSearchRequest)
@validate_response(200, ListSearchResponse)
@search_tag
async def list_search_app():
    """
    List search applications.

    Retrieves a paginated list of search applications with optional filtering.
    Supports filtering by owner IDs and pagination with customizable page size and ordering.
    """
    keywords = request.args.get("keywords", "")
    page_number = int(request.args.get("page", 0))
    items_per_page = int(request.args.get("page_size", 0))
    orderby = request.args.get("orderby", "create_time")
    if request.args.get("desc", "true").lower() == "false":
        desc = False
    else:
        desc = True

    req = await get_request_json()
    owner_ids = req.get("owner_ids", [])
    try:
        if not owner_ids:
            # tenants = TenantService.get_joined_tenants_by_user_id(current_user.id)
            # tenants = [m["tenant_id"] for m in tenants]
            tenants = []
            search_apps, total = SearchService.get_by_tenant_ids(tenants, current_user.id, page_number, items_per_page, orderby, desc, keywords)
        else:
            tenants = owner_ids
            search_apps, total = SearchService.get_by_tenant_ids(tenants, current_user.id, 0, 0, orderby, desc, keywords)
            search_apps = [search_app for search_app in search_apps if search_app["tenant_id"] in tenants]
            total = len(search_apps)
            if page_number and items_per_page:
                search_apps = search_apps[(page_number - 1) * items_per_page : page_number * items_per_page]
        return get_json_result(data={"search_apps": search_apps, "total": total})
    except Exception as e:
        return server_error_response(e)


@manager.route("/rm", methods=["POST"])  # noqa: F821
@login_required
@validate_request("search_id")
@qs_validate_request(DeleteSearchRequest)
@validate_response(200, DeleteSearchResponse)
@search_tag
async def rm():
    """
    Delete a search application.

    Permanently deletes a search application by its ID.
    Only the owner of the search application can delete it.
    """
    req = await get_request_json()
    search_id = req["search_id"]
    if not SearchService.accessible4deletion(search_id, current_user.id):
        return get_json_result(data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)

    try:
        if not SearchService.delete_by_id(search_id):
            return get_data_error_result(message=f"Failed to delete search App {search_id}")
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)

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

from typing import Annotated
from quart import request
from pydantic import BaseModel, ConfigDict, Field
from quart_schema import validate_request as qs_validate_request, validate_response, tag
from api.db.services import duplicate_name
from api.db.services.dialog_service import DialogService
from common.constants import StatusEnum
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import TenantService, UserTenantService
from api.utils.api_utils import get_data_error_result, get_json_result, get_request_json, server_error_response, validate_request
from common.misc_utils import get_uuid
from common.constants import RetCode
from api.apps import login_required, current_user
import logging


# =============================================================================
# Pydantic Schemas for OpenAPI Documentation
# =============================================================================

class BaseSchema(BaseModel):
    """Base schema with common configuration."""
    model_config = ConfigDict(extra="ignore", strict=False)


class PromptConfigParameter(BaseModel):
    """Parameter configuration for prompt templates."""
    key: Annotated[str, Field(..., description="Parameter key/name")]
    optional: Annotated[bool, Field(..., description="Whether the parameter is optional")]


class PromptConfig(BaseModel):
    """Prompt configuration for dialog."""
    system: Annotated[str, Field(..., description="System prompt template")]
    parameters: Annotated[list[dict] | None, Field(None, description="List of prompt parameters")]  # Using dict to match existing structure


class SetDialogRequest(BaseSchema):
    """Request schema for creating or updating a dialog."""
    dialog_id: Annotated[str | None, Field(None, description="Dialog ID (None for new dialog, existing ID for update)")]
    name: Annotated[str, Field(..., description="Dialog name", min_length=1, max_length=255)]
    description: Annotated[str, Field("A helpful dialog", description="Dialog description")]
    icon: Annotated[str, Field("", description="Dialog icon")]
    top_n: Annotated[int, Field(6, description="Number of top chunks to retrieve", ge=0, le=100)]
    top_k: Annotated[int, Field(1024, description="Number of top tokens to retrieve", ge=0)]
    rerank_id: Annotated[str, Field("", description="Rerank model ID")]
    similarity_threshold: Annotated[float, Field(0.1, description="Similarity threshold for retrieval", ge=0.0, le=1.0)]
    vector_similarity_weight: Annotated[float, Field(0.3, description="Weight for vector similarity", ge=0.0, le=1.0)]
    llm_id: Annotated[str | None, Field(None, description="LLM model ID")]
    llm_setting: Annotated[dict, Field({}, description="LLM settings")]
    kb_ids: Annotated[list[str], Field([], description="List of knowledge base IDs")]
    meta_data_filter: Annotated[dict, Field({}, description="Metadata filter for retrieval")]
    prompt_config: Annotated[dict, Field(..., description="Prompt configuration with system prompt and parameters")]


class DialogData(BaseModel):
    """Dialog data model."""
    id: Annotated[str, Field(..., description="Dialog ID")]
    tenant_id: Annotated[str, Field(..., description="Tenant ID")]
    name: Annotated[str, Field(..., description="Dialog name")]
    description: Annotated[str, Field(..., description="Dialog description")]
    icon: Annotated[str, Field(..., description="Dialog icon")]
    kb_ids: Annotated[list[str], Field(..., description="Knowledge base IDs")]
    kb_names: Annotated[list[str], Field(..., description="Knowledge base names")]
    llm_id: Annotated[str, Field(..., description="LLM model ID")]
    llm_setting: Annotated[dict, Field(..., description="LLM settings")]
    prompt_config: Annotated[dict, Field(..., description="Prompt configuration")]
    meta_data_filter: Annotated[dict, Field(..., description="Metadata filter")]
    top_n: Annotated[int, Field(..., description="Top N retrieval parameter")]
    top_k: Annotated[int, Field(..., description="Top K retrieval parameter")]
    rerank_id: Annotated[str, Field(..., description="Rerank model ID")]
    similarity_threshold: Annotated[float, Field(..., description="Similarity threshold")]
    vector_similarity_weight: Annotated[float, Field(..., description="Vector similarity weight")]


class SetDialogResponse(BaseModel):
    """Response schema for setting a dialog."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[DialogData, Field(..., description="Dialog data")]
    message: Annotated[str, Field("Success", description="Response message")]


class GetDialogResponse(BaseModel):
    """Response schema for getting a dialog."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[DialogData, Field(..., description="Dialog details")]
    message: Annotated[str, Field("Success", description="Response message")]


class ListDialogsResponse(BaseModel):
    """Response schema for listing dialogs."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[list[DialogData], Field(..., description="List of dialogs")]
    message: Annotated[str, Field("Success", description="Response message")]


class ListDialogsNextRequest(BaseSchema):
    """Request schema for listing dialogs with pagination."""
    owner_ids: Annotated[list[str] | None, Field(None, description="Filter by owner/tenant IDs")]


class ListDialogsNextResponse(BaseModel):
    """Response schema for listing dialogs with pagination."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Paginated dialog list with total count")]
    message: Annotated[str, Field("Success", description="Response message")]


class DeleteDialogRequest(BaseSchema):
    """Request schema for deleting dialogs."""
    dialog_ids: Annotated[list[str], Field(..., description="List of dialog IDs to delete", min_length=1)]


class DeleteDialogResponse(BaseModel):
    """Response schema for deleting dialogs."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="Deletion status")]
    message: Annotated[str, Field("Success", description="Response message")]


# Dialog API Tag
dialog_tag = tag(["dialog"])


@manager.route('/set', methods=['POST'])  # noqa: F821
@validate_request("prompt_config")
@qs_validate_request(SetDialogRequest)
@validate_response(200, SetDialogResponse)
@login_required
@dialog_tag
async def set_dialog():
    """
    Create or update a dialog.

    Creates a new dialog or updates an existing one with the provided configuration.
    Supports knowledge base integration, prompt configuration, and various retrieval parameters.
    Automatic name deduplication is applied for new dialogs.
    """
    req = await get_request_json()
    dialog_id = req.get("dialog_id", "")
    is_create = not dialog_id
    name = req.get("name", "New Dialog")
    if not isinstance(name, str):
        return get_data_error_result(message="Dialog name must be string.")
    if name.strip() == "":
        return get_data_error_result(message="Dialog name can't be empty.")
    if len(name.encode("utf-8")) > 255:
        return get_data_error_result(message=f"Dialog name length is {len(name)} which is larger than 255")

    name = name.strip()
    if is_create:
        # only for chat creating
        existing_names = {
            d.name.casefold()
            for d in DialogService.query(tenant_id=current_user.id, status=StatusEnum.VALID.value)
            if d.name
        }
        if name.casefold() in existing_names:
            def _name_exists(name: str, **_kwargs) -> bool:
                return name.casefold() in existing_names

            name = duplicate_name(_name_exists, name=name)

    description = req.get("description", "A helpful dialog")
    icon = req.get("icon", "")
    top_n = req.get("top_n", 6)
    top_k = req.get("top_k", 1024)
    rerank_id = req.get("rerank_id", "")
    if not rerank_id:
        req["rerank_id"] = ""
    similarity_threshold = req.get("similarity_threshold", 0.1)
    vector_similarity_weight = req.get("vector_similarity_weight", 0.3)
    llm_setting = req.get("llm_setting", {})
    meta_data_filter = req.get("meta_data_filter", {})
    prompt_config = req["prompt_config"]

    # Set default parameters for datasets with knowledge retrieval
    # All datasets with {knowledge} in system prompt need "knowledge" parameter to enable retrieval
    kb_ids = req.get("kb_ids", [])
    parameters = prompt_config.get("parameters")
    logging.debug(f"set_dialog: kb_ids={kb_ids}, parameters={parameters}, is_create={not is_create}")
    # Check if parameters is missing, None, or empty list
    if kb_ids and not parameters:
        # Check if system prompt uses {knowledge} placeholder
        if "{knowledge}" in prompt_config.get("system", ""):
            # Set default parameters for any dataset with knowledge placeholder
            prompt_config["parameters"] = [{"key": "knowledge", "optional": False}]
            logging.debug(f"Set default parameters for datasets with knowledge placeholder: {kb_ids}")

    if not is_create:
        # only for chat updating
        if not req.get("kb_ids", []) and not prompt_config.get("tavily_api_key") and "{knowledge}" in prompt_config.get("system", ""):
            return get_data_error_result(message="Please remove `{knowledge}` in system prompt since no dataset / Tavily used here.")

    for p in prompt_config.get("parameters", []):
        if p["optional"]:
            continue
        if prompt_config.get("system", "").find("{%s}" % p["key"]) < 0:
            return get_data_error_result(
                message="Parameter '{}' is not used".format(p["key"]))

    try:
        e, tenant = TenantService.get_by_id(current_user.id)
        if not e:
            return get_data_error_result(message="Tenant not found!")
        kbs = KnowledgebaseService.get_by_ids(req.get("kb_ids", []))
        embd_ids = [TenantLLMService.split_model_name_and_factory(kb.embd_id)[0] for kb in kbs]  # remove vendor suffix for comparison
        embd_count = len(set(embd_ids))
        if embd_count > 1:
            return get_data_error_result(message=f'Datasets use different embedding models: {[kb.embd_id for kb in kbs]}"')

        llm_id = req.get("llm_id", tenant.llm_id)
        if not dialog_id:
            dia = {
                "id": get_uuid(),
                "tenant_id": current_user.id,
                "name": name,
                "kb_ids": req.get("kb_ids", []),
                "description": description,
                "llm_id": llm_id,
                "llm_setting": llm_setting,
                "prompt_config": prompt_config,
                "meta_data_filter": meta_data_filter,
                "top_n": top_n,
                "top_k": top_k,
                "rerank_id": rerank_id,
                "similarity_threshold": similarity_threshold,
                "vector_similarity_weight": vector_similarity_weight,
                "icon": icon
            }
            if not DialogService.save(**dia):
                return get_data_error_result(message="Fail to new a dialog!")
            return get_json_result(data=dia)
        else:
            del req["dialog_id"]
            if "kb_names" in req:
                del req["kb_names"]
            if not DialogService.update_by_id(dialog_id, req):
                return get_data_error_result(message="Dialog not found!")
            e, dia = DialogService.get_by_id(dialog_id)
            if not e:
                return get_data_error_result(message="Fail to update a dialog!")
            dia = dia.to_dict()
            dia.update(req)
            dia["kb_ids"], dia["kb_names"] = get_kb_names(dia["kb_ids"])
            return get_json_result(data=dia)
    except Exception as e:
        return server_error_response(e)


@manager.route('/get', methods=['GET'])  # noqa: F821
@login_required
@validate_response(200, GetDialogResponse)
@dialog_tag
def get():
    """
    Get dialog details.

    Retrieves detailed information about a specific dialog by its ID,
    including associated knowledge bases and configuration.
    """
    dialog_id = request.args["dialog_id"]
    try:
        e, dia = DialogService.get_by_id(dialog_id)
        if not e:
            return get_data_error_result(message="Dialog not found!")
        dia = dia.to_dict()
        dia["kb_ids"], dia["kb_names"] = get_kb_names(dia["kb_ids"])
        return get_json_result(data=dia)
    except Exception as e:
        return server_error_response(e)


def get_kb_names(kb_ids):
    ids, nms = [], []
    for kid in kb_ids:
        e, kb = KnowledgebaseService.get_by_id(kid)
        if not e or kb.status != StatusEnum.VALID.value:
            continue
        ids.append(kid)
        nms.append(kb.name)
    return ids, nms


@manager.route('/list', methods=['GET'])  # noqa: F821
@login_required
@validate_response(200, ListDialogsResponse)
@dialog_tag
def list_dialogs():
    """
    List all dialogs for current user.

    Retrieves a list of all valid dialogs belonging to the current tenant/user.
    Returns dialogs with their associated knowledge base information.
    """
    try:
        conversations = DialogService.query(
            tenant_id=current_user.id,
            status=StatusEnum.VALID.value,
            reverse=True,
            order_by=DialogService.model.create_time)
        conversations = [d.to_dict() for d in conversations]
        for conversation in conversations:
            conversation["kb_ids"], conversation["kb_names"] = get_kb_names(conversation["kb_ids"])
        return get_json_result(data=conversations)
    except Exception as e:
        return server_error_response(e)


@manager.route('/next', methods=['POST'])  # noqa: F821
@login_required
@qs_validate_request(ListDialogsNextRequest)
@validate_response(200, ListDialogsNextResponse)
@dialog_tag
async def list_dialogs_next():
    """
    List dialogs with pagination and filtering.

    Retrieves a paginated list of dialogs with support for filtering by owner/tenant IDs,
    keyword search, and custom sorting. Supports both personal and shared dialogs.
    """
    args = request.args
    keywords = args.get("keywords", "")
    page_number = int(args.get("page", 0))
    items_per_page = int(args.get("page_size", 0))
    parser_id = args.get("parser_id")
    orderby = args.get("orderby", "create_time")
    if args.get("desc", "true").lower() == "false":
        desc = False
    else:
        desc = True

    req = await get_request_json()
    owner_ids = req.get("owner_ids", [])
    try:
        if not owner_ids:
            # tenants = TenantService.get_joined_tenants_by_user_id(current_user.id)
            # tenants = [tenant["tenant_id"] for tenant in tenants]
            tenants = [] # keep it here
            dialogs, total = DialogService.get_by_tenant_ids(
                tenants, current_user.id, page_number,
                items_per_page, orderby, desc, keywords, parser_id)
        else:
            tenants = owner_ids
            dialogs, total = DialogService.get_by_tenant_ids(
                tenants, current_user.id, 0,
                0, orderby, desc, keywords, parser_id)
            dialogs = [dialog for dialog in dialogs if dialog["tenant_id"] in tenants]
            total = len(dialogs)
            if page_number and items_per_page:
                dialogs = dialogs[(page_number-1)*items_per_page:page_number*items_per_page]
        return get_json_result(data={"dialogs": dialogs, "total": total})
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['POST'])  # noqa: F821
@login_required
@validate_request("dialog_ids")
@qs_validate_request(DeleteDialogRequest)
@validate_response(200, DeleteDialogResponse)
@dialog_tag
async def rm():
    """
    Delete dialogs.

    Permanently deletes one or more dialogs by marking them as invalid.
    Only the owner of a dialog can delete it. Authorization is checked for each dialog.
    """
    req = await get_request_json()
    dialog_list=[]
    tenants = UserTenantService.query(user_id=current_user.id)
    try:
        for id in req["dialog_ids"]:
            for tenant in tenants:
                if DialogService.query(tenant_id=tenant.tenant_id, id=id):
                    break
            else:
                return get_json_result(
                    data=False, message='Only owner of dialog authorized for this operation.',
                    code=RetCode.OPERATING_ERROR)
            dialog_list.append({"id": id,"status":StatusEnum.INVALID.value})
        DialogService.update_many_by_id(dialog_list)
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)

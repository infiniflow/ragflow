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
import json

from api.apps import login_required, current_user
from api.db import TenantPermission
from api.db.services.memory_service import MemoryService
from api.utils.api_utils import validate_request, request_json, get_error_argument_result, get_json_result, \
    not_allowed_parameters
from api.utils.memory_utils import format_ret_data_from_memory
from api.constants import MEMORY_NAME_LIMIT, MEMORY_SIZE_LIMIT
from common.constants import MemoryType, RetCode, ForgettingPolicy


@manager.route("/create", methods=["POST"])  # noqa: F821
@login_required
@validate_request("memory_name", "memory_type", "embedding", "llm")
async def create_memory():
    req = await request_json()
    # check name length
    name = req["memory_name"]
    memory_name = name.strip()
    if len(memory_name) > MEMORY_NAME_LIMIT:
        return get_error_argument_result(f"Memory name '{memory_name}' exceeds limit of {MEMORY_NAME_LIMIT}.")
    # check memory_type valid
    memory_type = set(req["memory_type"])
    invalid_type = memory_type - {e.value for e in MemoryType}
    if invalid_type:
        return get_error_argument_result(f"Memory type '{invalid_type}' is not supported.")
    memory_type = list(memory_type)

    try:
        res, memory = MemoryService.create_memory(
            tenant_id=current_user.tenant_id,
            name=memory_name,
            memory_type=memory_type,
            embedding=req["embedding"],
            llm=req["llm"]
        )

        if res:
            return get_json_result(message=True, data=format_ret_data_from_memory(memory))

        else:
            return get_json_result(message=memory, code=RetCode.SERVER_ERROR)

    except Exception as e:
        return get_json_result(message=str(e), code=RetCode.SERVER_ERROR)


@manager.route("/update/<memory_id>", methods=["PUT"])  # noqa: F821
@login_required
@not_allowed_parameters("memory_id", "tenant_id", "memory_type", "storage_type", "embedding")
async def update_memory(memory_id):
    req = await request_json()
    update_dict = {}
    # check name length
    if req.get("memory_name"):
        name = req["memory_name"]
        memory_name = name.strip()
        if len(memory_name) > MEMORY_NAME_LIMIT:
            return get_error_argument_result(f"Memory name '{memory_name}' exceeds limit of {MEMORY_NAME_LIMIT}.")
        update_dict["memory_name"] = memory_name
    # check memory_type valid
    if req.get("memory_type"):
        memory_type = set(req["memory_type"])
        invalid_type = memory_type - {e.value for e in MemoryType}
        if invalid_type:
            return get_error_argument_result(f"Memory type '{invalid_type}' is not supported.")
        update_dict["memory_type"] = list(memory_type)
    # check permissions valid
    if req.get("permissions"):
        if req["permissions"] not in [e.value for e in TenantPermission]:
            return get_error_argument_result(f"Unknown permission '{req['permissions']}'.")
        update_dict["permissions"] = req["permissions"]
    if req.get("llm"):
        update_dict["llm"] = req["llm"]
    # check memory_size valid
    if req.get("memory_size"):
        if not 0 < int(req["memory_size"]) <= MEMORY_SIZE_LIMIT:
            return get_error_argument_result(f"Memory size should be in range (0, {MEMORY_SIZE_LIMIT}] Bytes.")
        update_dict["memory_size"] = req["memory_size"]
    # check forgetting_policy valid
    if req.get("forgetting_policy"):
        if req["forgetting_policy"] not in [e.value for e in ForgettingPolicy]:
            return get_error_argument_result(f"Forgetting policy '{req['forgetting_policy']}' is not supported.")
        update_dict["forgetting_policy"] = req["forgetting_policy"]
    # check temperature valid
    if "temperature" in req:
        temperature = float(req["temperature"])
        if not 0 <= temperature <= 1:
            return get_error_argument_result(f"Temperature should be in range [0, 1].")
        update_dict["temperature"] = temperature
    # allow update to empty fields
    for field in ["avatar", "description", "system_prompt", "user_prompt"]:
        if field in req:
            update_dict[field] = req[field]
    current_memory = MemoryService.get_by_memory_id(memory_id)
    if not current_memory:
        return get_json_result(code=RetCode.NOT_FOUND, message=f"Memory '{memory_id}' not found.")

    memory_dict = current_memory.to_dict()
    memory_dict.update({"memory_type": json.loads(current_memory.memory_type)})
    to_update = {}
    for k, v in update_dict.items():
        if isinstance(v, list) and set(memory_dict[k]) != set(v):
            to_update[k] = v
        elif memory_dict[k] != v:
            to_update[k] = v

    if not to_update:
        return get_json_result(message=True, data=memory_dict)

    try:
        MemoryService.update_memory(memory_id, to_update)
        updated_memory = MemoryService.get_by_memory_id(memory_id)
        return get_json_result(message=True, data=format_ret_data_from_memory(updated_memory))

    except Exception as e:
        logging.error(e)
        return get_json_result(message=str(e), code=RetCode.SERVER_ERROR)


@manager.route("/rm/<memory_id>", methods=["DELETE"]) # noqa: F821
@login_required
async def delete_memory(memory_id):
    memory = MemoryService.get_by_memory_id(memory_id)
    if not memory:
        return get_json_result(message=True, code=RetCode.NOT_FOUND)
    try:
        MemoryService.delete_memory(memory_id)
        return get_json_result(message=True)
    except Exception as e:
        logging.error(e)
        return get_json_result(message=str(e), code=RetCode.SERVER_ERROR)


@manager.route("/list", methods=["POST"]) # noqa: F821
@login_required
async def list_memory():
    req = await request_json()
    try:
        memory_list, count = MemoryService.get_by_filter(req["filter"], req["keywords"], req["page"], req["page_size"])
        [memory.update({"memory_type": json.loads(memory["memory_type"]), "temperature": json.dumps(memory["temperature"])}) for memory in memory_list]
        return get_json_result(message=True, data={"memory_list": memory_list, "count": count})

    except Exception as e:
        logging.error(e)
        return get_json_result(message=str(e), code=RetCode.SERVER_ERROR)

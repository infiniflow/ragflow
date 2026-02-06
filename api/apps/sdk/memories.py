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
import logging
import os
import time

from quart import request
from api.apps import login_required, current_user
from api.db import TenantPermission
from api.db.services.memory_service import MemoryService
from api.db.services.user_service import UserTenantService
from api.db.services.canvas_service import UserCanvasService
from api.db.services.task_service import TaskService
from api.db.joint_services.memory_message_service import get_memory_size_cache, judge_system_prompt_is_default
from api.utils.api_utils import validate_request, get_request_json, get_error_argument_result, get_json_result
from api.utils.memory_utils import format_ret_data_from_memory, get_memory_type_human
from api.constants import MEMORY_NAME_LIMIT, MEMORY_SIZE_LIMIT
from memory.services.messages import MessageService
from memory.utils.prompt_util import PromptAssembler
from common.constants import MemoryType, RetCode, ForgettingPolicy


@manager.route("/memories", methods=["POST"])  # noqa: F821
@login_required
@validate_request("name", "memory_type", "embd_id", "llm_id")
async def create_memory():
    timing_enabled = os.getenv("RAGFLOW_API_TIMING")
    t_start = time.perf_counter() if timing_enabled else None
    req = await get_request_json()
    t_parsed = time.perf_counter() if timing_enabled else None
    # check name length
    name = req["name"]
    memory_name = name.strip()
    if len(memory_name) == 0:
        if timing_enabled:
            logging.info(
                "api_timing create_memory invalid_name parse_ms=%.2f total_ms=%.2f path=%s",
                (t_parsed - t_start) * 1000,
                (time.perf_counter() - t_start) * 1000,
                request.path,
            )
        return get_error_argument_result("Memory name cannot be empty or whitespace.")
    if len(memory_name) > MEMORY_NAME_LIMIT:
        if timing_enabled:
            logging.info(
                "api_timing create_memory invalid_name parse_ms=%.2f total_ms=%.2f path=%s",
                (t_parsed - t_start) * 1000,
                (time.perf_counter() - t_start) * 1000,
                request.path,
            )
        return get_error_argument_result(f"Memory name '{memory_name}' exceeds limit of {MEMORY_NAME_LIMIT}.")
    # check memory_type valid
    if not isinstance(req["memory_type"], list):
        if timing_enabled:
            logging.info(
                "api_timing create_memory invalid_memory_type parse_ms=%.2f total_ms=%.2f path=%s",
                (t_parsed - t_start) * 1000,
                (time.perf_counter() - t_start) * 1000,
                request.path,
            )
        return get_error_argument_result("Memory type must be a list.")
    memory_type = set(req["memory_type"])
    invalid_type = memory_type - {e.name.lower() for e in MemoryType}
    if invalid_type:
        if timing_enabled:
            logging.info(
                "api_timing create_memory invalid_memory_type parse_ms=%.2f total_ms=%.2f path=%s",
                (t_parsed - t_start) * 1000,
                (time.perf_counter() - t_start) * 1000,
                request.path,
            )
        return get_error_argument_result(f"Memory type '{invalid_type}' is not supported.")
    memory_type = list(memory_type)

    try:
        t_before_db = time.perf_counter() if timing_enabled else None
        res, memory = MemoryService.create_memory(
            tenant_id=current_user.id,
            name=memory_name,
            memory_type=memory_type,
            embd_id=req["embd_id"],
            llm_id=req["llm_id"]
        )
        if timing_enabled:
            logging.info(
                "api_timing create_memory parse_ms=%.2f validate_ms=%.2f db_ms=%.2f total_ms=%.2f path=%s",
                (t_parsed - t_start) * 1000,
                (t_before_db - t_parsed) * 1000,
                (time.perf_counter() - t_before_db) * 1000,
                (time.perf_counter() - t_start) * 1000,
                request.path,
            )

        if res:
            return get_json_result(message=True, data=format_ret_data_from_memory(memory))
        else:
            return get_json_result(message=memory, code=RetCode.SERVER_ERROR)

    except Exception as e:
        return get_json_result(message=str(e), code=RetCode.SERVER_ERROR)


@manager.route("/memories/<memory_id>", methods=["PUT"])  # noqa: F821
@login_required
async def update_memory(memory_id):
    req = await get_request_json()
    update_dict = {}
    # check name length
    if "name" in req:
        name = req["name"]
        memory_name = name.strip()
        if len(memory_name) == 0:
            return get_error_argument_result("Memory name cannot be empty or whitespace.")
        if len(memory_name) > MEMORY_NAME_LIMIT:
            return get_error_argument_result(f"Memory name '{memory_name}' exceeds limit of {MEMORY_NAME_LIMIT}.")
        update_dict["name"] = memory_name
    # check permissions valid
    if req.get("permissions"):
        if req["permissions"] not in [e.value for e in TenantPermission]:
            return get_error_argument_result(f"Unknown permission '{req['permissions']}'.")
        update_dict["permissions"] = req["permissions"]
    if req.get("llm_id"):
        update_dict["llm_id"] = req["llm_id"]
    if req.get("embd_id"):
        update_dict["embd_id"] = req["embd_id"]
    if req.get("memory_type"):
        memory_type = set(req["memory_type"])
        invalid_type = memory_type - {e.name.lower() for e in MemoryType}
        if invalid_type:
            return get_error_argument_result(f"Memory type '{invalid_type}' is not supported.")
        update_dict["memory_type"] = list(memory_type)
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
            return get_error_argument_result("Temperature should be in range [0, 1].")
        update_dict["temperature"] = temperature
    # allow update to empty fields
    for field in ["avatar", "description", "system_prompt", "user_prompt"]:
        if field in req:
            update_dict[field] = req[field]
    current_memory = MemoryService.get_by_memory_id(memory_id)
    if not current_memory:
        return get_json_result(code=RetCode.NOT_FOUND, message=f"Memory '{memory_id}' not found.")

    memory_dict = current_memory.to_dict()
    memory_dict.update({"memory_type": get_memory_type_human(current_memory.memory_type)})
    to_update = {}
    for k, v in update_dict.items():
        if isinstance(v, list) and set(memory_dict[k]) != set(v):
            to_update[k] = v
        elif memory_dict[k] != v:
            to_update[k] = v

    if not to_update:
        return get_json_result(message=True, data=memory_dict)
    # check memory empty when update embd_id, memory_type
    memory_size = get_memory_size_cache(memory_id, current_memory.tenant_id)
    not_allowed_update = [f for f in ["embd_id", "memory_type"] if f in to_update and memory_size > 0]
    if not_allowed_update:
        return get_error_argument_result(f"Can't update {not_allowed_update} when memory isn't empty.")
    if "memory_type" in to_update:
        if "system_prompt" not in to_update and judge_system_prompt_is_default(current_memory.system_prompt, current_memory.memory_type):
            # update old default prompt, assemble a new one
            to_update["system_prompt"] = PromptAssembler.assemble_system_prompt({"memory_type": to_update["memory_type"]})

    try:
        MemoryService.update_memory(current_memory.tenant_id, memory_id, to_update)
        updated_memory = MemoryService.get_by_memory_id(memory_id)
        return get_json_result(message=True, data=format_ret_data_from_memory(updated_memory))

    except Exception as e:
        logging.error(e)
        return get_json_result(message=str(e), code=RetCode.SERVER_ERROR)


@manager.route("/memories/<memory_id>", methods=["DELETE"])  # noqa: F821
@login_required
async def delete_memory(memory_id):
    memory = MemoryService.get_by_memory_id(memory_id)
    if not memory:
        return get_json_result(message=True, code=RetCode.NOT_FOUND)
    try:
        MemoryService.delete_memory(memory_id)
        if MessageService.has_index(memory.tenant_id, memory_id):
            MessageService.delete_message({"memory_id": memory_id}, memory.tenant_id, memory_id)
        return get_json_result(message=True)
    except Exception as e:
        logging.error(e)
        return get_json_result(message=str(e), code=RetCode.SERVER_ERROR)


@manager.route("/memories", methods=["GET"])  # noqa: F821
@login_required
async def list_memory():
    args = request.args
    try:
        tenant_ids = args.getlist("tenant_id")
        memory_types = args.getlist("memory_type")
        storage_type = args.get("storage_type")
        keywords = args.get("keywords", "")
        page = int(args.get("page", 1))
        page_size = int(args.get("page_size", 50))
        # make filter dict
        filter_dict: dict = {"storage_type": storage_type}
        if not tenant_ids:
            # restrict to current user's tenants
            user_tenants = UserTenantService.get_user_tenant_relation_by_user_id(current_user.id)
            filter_dict["tenant_id"] = [tenant["tenant_id"] for tenant in user_tenants]
        else:
            if len(tenant_ids) == 1 and ',' in tenant_ids[0]:
                tenant_ids = tenant_ids[0].split(',')
            filter_dict["tenant_id"] = tenant_ids
        if memory_types and len(memory_types) == 1 and ',' in memory_types[0]:
            memory_types = memory_types[0].split(',')
        filter_dict["memory_type"] = memory_types

        memory_list, count = MemoryService.get_by_filter(filter_dict, keywords, page, page_size)
        [memory.update({"memory_type": get_memory_type_human(memory["memory_type"])}) for memory in memory_list]
        return get_json_result(message=True, data={"memory_list": memory_list, "total_count": count})

    except Exception as e:
        logging.error(e)
        return get_json_result(message=str(e), code=RetCode.SERVER_ERROR)


@manager.route("/memories/<memory_id>/config", methods=["GET"])  # noqa: F821
@login_required
async def get_memory_config(memory_id):
    memory = MemoryService.get_with_owner_name_by_id(memory_id)
    if not memory:
        return get_json_result(code=RetCode.NOT_FOUND, message=f"Memory '{memory_id}' not found.")
    return get_json_result(message=True, data=format_ret_data_from_memory(memory))


@manager.route("/memories/<memory_id>", methods=["GET"])  # noqa: F821
@login_required
async def get_memory_detail(memory_id):
    args = request.args
    agent_ids = args.getlist("agent_id")
    if len(agent_ids) == 1 and ',' in agent_ids[0]:
        agent_ids = agent_ids[0].split(',')
    keywords = args.get("keywords", "")
    keywords = keywords.strip()
    page = int(args.get("page", 1))
    page_size = int(args.get("page_size", 50))
    memory = MemoryService.get_by_memory_id(memory_id)
    if not memory:
        return get_json_result(code=RetCode.NOT_FOUND, message=f"Memory '{memory_id}' not found.")
    messages = MessageService.list_message(
        memory.tenant_id, memory_id, agent_ids, keywords, page, page_size)
    agent_name_mapping = {}
    extract_task_mapping = {}
    if messages["message_list"]:
        agent_list = UserCanvasService.get_basic_info_by_canvas_ids([message["agent_id"] for message in messages["message_list"]])
        agent_name_mapping = {agent["id"]: agent["title"] for agent in agent_list}
        task_list = TaskService.get_tasks_progress_by_doc_ids([memory_id])
        if task_list:
            task_list.sort(key=lambda t: t["create_time"]) # asc, use newer when exist more than one task
            for task in task_list:
                # the 'digest' field carries the source_id when a task is created, so use 'digest' as key
                extract_task_mapping.update({int(task["digest"]): task})
    for message in messages["message_list"]:
        message["agent_name"] = agent_name_mapping.get(message["agent_id"], "Unknown")
        message["task"] = extract_task_mapping.get(message["message_id"], {})
        for extract_msg in message["extract"]:
            extract_msg["agent_name"] = agent_name_mapping.get(extract_msg["agent_id"], "Unknown")
    return get_json_result(data={"messages": messages, "storage_type": memory.storage_type}, message=True)

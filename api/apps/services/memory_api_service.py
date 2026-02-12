#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
from api.apps import current_user
from api.db import TenantPermission
from api.db.services.memory_service import MemoryService
from api.db.services.user_service import UserTenantService
from api.db.services.canvas_service import UserCanvasService
from api.db.services.task_service import TaskService
from api.db.joint_services.memory_message_service import get_memory_size_cache, judge_system_prompt_is_default, queue_save_to_memory_task, query_message
from api.utils.memory_utils import format_ret_data_from_memory, get_memory_type_human
from api.constants import MEMORY_NAME_LIMIT, MEMORY_SIZE_LIMIT
from memory.services.messages import MessageService
from memory.utils.prompt_util import PromptAssembler
from common.constants import MemoryType, ForgettingPolicy
from common.exceptions import ArgumentException, NotFoundException
from common.time_utils import current_timestamp, timestamp_to_date


async def create_memory(memory_info: dict):
    """
    :param memory_info: {
        "name": str,
        "memory_type": list[str],
        "embd_id": str,
        "llm_id": str
    }
    """
    # check name length
    name = memory_info["name"]
    memory_name = name.strip()
    if len(memory_name) == 0:
        raise ArgumentException("Memory name cannot be empty or whitespace.")
    if len(memory_name) > MEMORY_NAME_LIMIT:
        raise ArgumentException(f"Memory name '{memory_name}' exceeds limit of {MEMORY_NAME_LIMIT}.")
    # check memory_type valid
    if not isinstance(memory_info["memory_type"], list):
        raise ArgumentException("Memory type must be a list.")
    memory_type = set(memory_info["memory_type"])
    invalid_type = memory_type - {e.name.lower() for e in MemoryType}
    if invalid_type:
        raise ArgumentException(f"Memory type '{invalid_type}' is not supported.")
    memory_type = list(memory_type)
    success, res = MemoryService.create_memory(
        tenant_id=current_user.id,
        name=memory_name,
        memory_type=memory_type,
        embd_id=memory_info["embd_id"],
        llm_id=memory_info["llm_id"]
    )
    if success:
        return True, format_ret_data_from_memory(res)
    else:
        return False, res


async def update_memory(memory_id: str, new_memory_setting: dict):
    """
    :param memory_id: str
    :param new_memory_setting: {
        "name": str,
        "permissions": str,
        "llm_id": str,
        "embd_id": str,
        "memory_type": list[str],
        "memory_size": int,
        "forgetting_policy": str,
        "temperature": float,
        "avatar": str,
        "description": str,
        "system_prompt": str,
        "user_prompt": str
    }
    """
    update_dict = {}
    # check name length
    if "name" in new_memory_setting:
        name = new_memory_setting["name"]
        memory_name = name.strip()
        if len(memory_name) == 0:
            raise ArgumentException("Memory name cannot be empty or whitespace.")
        if len(memory_name) > MEMORY_NAME_LIMIT:
            raise ArgumentException(f"Memory name '{memory_name}' exceeds limit of {MEMORY_NAME_LIMIT}.")
        update_dict["name"] = memory_name
    # check permissions valid
    if new_memory_setting.get("permissions"):
        if new_memory_setting["permissions"] not in [e.value for e in TenantPermission]:
            raise ArgumentException(f"Unknown permission '{new_memory_setting['permissions']}'.")
        update_dict["permissions"] = new_memory_setting["permissions"]
    if new_memory_setting.get("llm_id"):
        update_dict["llm_id"] = new_memory_setting["llm_id"]
    if new_memory_setting.get("embd_id"):
        update_dict["embd_id"] = new_memory_setting["embd_id"]
    if new_memory_setting.get("memory_type"):
        memory_type = set(new_memory_setting["memory_type"])
        invalid_type = memory_type - {e.name.lower() for e in MemoryType}
        if invalid_type:
            raise ArgumentException(f"Memory type '{invalid_type}' is not supported.")
        update_dict["memory_type"] = list(memory_type)
    # check memory_size valid
    if new_memory_setting.get("memory_size"):
        if not 0 < int(new_memory_setting["memory_size"]) <= MEMORY_SIZE_LIMIT:
            raise ArgumentException(f"Memory size should be in range (0, {MEMORY_SIZE_LIMIT}] Bytes.")
        update_dict["memory_size"] = new_memory_setting["memory_size"]
    # check forgetting_policy valid
    if new_memory_setting.get("forgetting_policy"):
        if new_memory_setting["forgetting_policy"] not in [e.value for e in ForgettingPolicy]:
            raise ArgumentException(f"Forgetting policy '{new_memory_setting['forgetting_policy']}' is not supported.")
        update_dict["forgetting_policy"] = new_memory_setting["forgetting_policy"]
    # check temperature valid
    if "temperature" in new_memory_setting:
        temperature = float(new_memory_setting["temperature"])
        if not 0 <= temperature <= 1:
            raise ArgumentException("Temperature should be in range [0, 1].")
        update_dict["temperature"] = temperature
    # allow update to empty fields
    for field in ["avatar", "description", "system_prompt", "user_prompt"]:
        if field in new_memory_setting:
            update_dict[field] = new_memory_setting[field]
    current_memory = MemoryService.get_by_memory_id(memory_id)
    if not current_memory:
        raise NotFoundException(f"Memory '{memory_id}' not found.")

    memory_dict = current_memory.to_dict()
    memory_dict.update({"memory_type": get_memory_type_human(current_memory.memory_type)})
    to_update = {}
    for k, v in update_dict.items():
        if isinstance(v, list) and set(memory_dict[k]) != set(v):
            to_update[k] = v
        elif memory_dict[k] != v:
            to_update[k] = v

    if not to_update:
        return True, memory_dict
    # check memory empty when update embd_id, memory_type
    memory_size = get_memory_size_cache(memory_id, current_memory.tenant_id)
    not_allowed_update = [f for f in ["embd_id", "memory_type"] if f in to_update and memory_size > 0]
    if not_allowed_update:
        raise ArgumentException(f"Can't update {not_allowed_update} when memory isn't empty.")
    if "memory_type" in to_update:
        if "system_prompt" not in to_update and judge_system_prompt_is_default(current_memory.system_prompt, current_memory.memory_type):
            # update old default prompt, assemble a new one
            to_update["system_prompt"] = PromptAssembler.assemble_system_prompt({"memory_type": to_update["memory_type"]})

    MemoryService.update_memory(current_memory.tenant_id, memory_id, to_update)
    updated_memory = MemoryService.get_by_memory_id(memory_id)
    return True, format_ret_data_from_memory(updated_memory)


async def delete_memory(memory_id):
    memory = MemoryService.get_by_memory_id(memory_id)
    if not memory:
        raise NotFoundException(f"Memory '{memory_id}' not found.")
    MemoryService.delete_memory(memory_id)
    if MessageService.has_index(memory.tenant_id, memory_id):
        MessageService.delete_message({"memory_id": memory_id}, memory.tenant_id, memory_id)
    return True


async def list_memory(filter_params: dict, keywords: str, page: int=1, page_size: int = 50):
    """
    :param filter_params: {
        "memory_type": list[str],
        "tenant_id": list[str],
        "storage_type": str
    }
    :param keywords: str
    :param page: int
    :param page_size: int
    """
    filter_dict: dict = {"storage_type": filter_params.get("storage_type")}
    tenant_ids = filter_params.get("tenant_id")
    if not filter_params.get("tenant_id"):
        # restrict to current user's tenants
        user_tenants = UserTenantService.get_user_tenant_relation_by_user_id(current_user.id)
        filter_dict["tenant_id"] = [tenant["tenant_id"] for tenant in user_tenants]
    else:
        if len(tenant_ids) == 1 and ',' in tenant_ids[0]:
            tenant_ids = tenant_ids[0].split(',')
        filter_dict["tenant_id"] = tenant_ids
    memory_types = filter_params.get("memory_type")
    if memory_types and len(memory_types) == 1 and ',' in memory_types[0]:
        memory_types = memory_types[0].split(',')
    filter_dict["memory_type"] = memory_types

    memory_list, count = MemoryService.get_by_filter(filter_dict, keywords, page, page_size)
    [memory.update({"memory_type": get_memory_type_human(memory["memory_type"])}) for memory in memory_list]
    return {
        "memory_list": memory_list, "total_count": count
    }


async def get_memory_config(memory_id):
    memory = MemoryService.get_with_owner_name_by_id(memory_id)
    if not memory:
        raise NotFoundException(f"Memory '{memory_id}' not found.")
    return format_ret_data_from_memory(memory)


async def get_memory_messages(memory_id, agent_ids: list[str], keywords: str, page: int=1, page_size: int = 50):
    memory = MemoryService.get_by_memory_id(memory_id)
    if not memory:
        raise NotFoundException(f"Memory '{memory_id}' not found.")
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
    return {"messages": messages, "storage_type": memory.storage_type}


async def add_message(memory_ids: list[str], message_dict: dict):
    """
    :param memory_ids: list[str]
    :param message_dict: {
        "agent_id": str,
        "session_id": str,
        "user_input": str,
        "agent_response": str,
        "message_type": str
    }
    """
    return await queue_save_to_memory_task(memory_ids, message_dict)


async def forget_message(memory_id: str, message_id: int):
    memory = MemoryService.get_by_memory_id(memory_id)
    if not memory:
        raise NotFoundException(f"Memory '{memory_id}' not found.")

    forget_time = timestamp_to_date(current_timestamp())
    update_succeed = MessageService.update_message(
        {"memory_id": memory_id, "message_id": int(message_id)},
        {"forget_at": forget_time},
        memory.tenant_id, memory_id)
    if update_succeed:
        return True
    raise Exception(f"Failed to forget message '{message_id}' in memory '{memory_id}'.")


async def update_message_status(memory_id: str, message_id: int, status: bool):
    memory = MemoryService.get_by_memory_id(memory_id)
    if not memory:
        raise NotFoundException(f"Memory '{memory_id}' not found.")

    update_succeed = MessageService.update_message(
        {"memory_id": memory_id, "message_id": int(message_id)},
        {"status": status},
        memory.tenant_id, memory_id)
    if update_succeed:
        return True
    raise Exception(f"Failed to set status for message '{message_id}' in memory '{memory_id}'.")


async def search_message(filter_dict: dict, params: dict):
    """
    :param filter_dict: {
        "memory_id": list[str],
        "agent_id": str,
        "session_id": str
    }
    :param params: {
        "query": str,
        "similarity_threshold": float,
        "keywords_similarity_weight": float,
        "top_n": int
    }
    """
    return query_message(filter_dict, params)


async def get_messages(memory_ids: list[str], agent_id: str = "", session_id: str = "", limit: int = 10):
    """
    Get recent messages from specified memories.

    :param memory_ids: list of memory IDs
    :param agent_id: optional agent ID for filtering
    :param session_id: optional session ID for filtering
    :param limit: maximum number of messages to return
    :return: list of recent messages
    """
    memory_list = MemoryService.get_by_ids(memory_ids)
    uids = [memory.tenant_id for memory in memory_list]
    res = MessageService.get_recent_messages(
        uids,
        memory_ids,
        agent_id,
        session_id,
        limit
    )
    return res


async def get_message_content(memory_id: str, message_id: int):
    """
    Get content of a specific message from a memory.

    :param memory_id: memory ID
    :param message_id: message ID
    :return: message content
    :raises NotFoundException: if memory or message not found
    """
    memory = MemoryService.get_by_memory_id(memory_id)
    if not memory:
        raise NotFoundException(f"Memory '{memory_id}' not found.")

    res = MessageService.get_by_message_id(memory_id, message_id, memory.tenant_id)
    if res:
        return res
    raise NotFoundException(f"Message '{message_id}' in memory '{memory_id}' not found.")
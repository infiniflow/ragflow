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
from typing import List

from common import settings
from common.time_utils import current_timestamp, timestamp_to_date, format_iso_8601_to_ymd_hms
from common.constants import MemoryType, LLMType
from common.doc_store.doc_store_base import FusionExpr
from common.misc_utils import get_uuid
from api.db.db_utils import bulk_insert_into_db
from api.db.db_models import Task
from api.db.services.task_service import TaskService
from api.db.services.memory_service import MemoryService
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.llm_service import LLMBundle
from api.utils.memory_utils import get_memory_type_human
from memory.services.messages import MessageService
from memory.services.query import MsgTextQuery, get_vector
from memory.utils.prompt_util import PromptAssembler
from memory.utils.msg_util import get_json_result_from_llm_response
from rag.utils.redis_conn import REDIS_CONN


async def save_to_memory(memory_id: str, message_dict: dict):
    """
    :param memory_id:
    :param message_dict: {
        "user_id": str,
        "agent_id": str,
        "session_id": str,
        "user_input": str,
        "agent_response": str
    }
    """
    memory = MemoryService.get_by_memory_id(memory_id)
    if not memory:
        return False, f"Memory '{memory_id}' not found."

    tenant_id = memory.tenant_id
    extracted_content = await extract_by_llm(
        tenant_id,
        memory.llm_id,
        {"temperature": memory.temperature},
        get_memory_type_human(memory.memory_type),
        message_dict.get("user_input", ""),
        message_dict.get("agent_response", "")
    ) if memory.memory_type != MemoryType.RAW.value else []  # if only RAW, no need to extract
    raw_message_id = REDIS_CONN.generate_auto_increment_id(namespace="memory")
    message_list = [{
        "message_id": raw_message_id,
        "message_type": MemoryType.RAW.name.lower(),
        "source_id": 0,
        "memory_id": memory_id,
        "user_id": "",
        "agent_id": message_dict["agent_id"],
        "session_id": message_dict["session_id"],
        "content": f"User Input: {message_dict.get('user_input')}\nAgent Response: {message_dict.get('agent_response')}",
        "valid_at": timestamp_to_date(current_timestamp()),
        "invalid_at": None,
        "forget_at": None,
        "status": True
    }, *[{
        "message_id": REDIS_CONN.generate_auto_increment_id(namespace="memory"),
        "message_type": content["message_type"],
        "source_id": raw_message_id,
        "memory_id": memory_id,
        "user_id": "",
        "agent_id": message_dict["agent_id"],
        "session_id": message_dict["session_id"],
        "content": content["content"],
        "valid_at": content["valid_at"],
        "invalid_at": content["invalid_at"] if content["invalid_at"] else None,
        "forget_at": None,
        "status": True
    } for content in extracted_content]]
    return await embed_and_save(memory, message_list)


async def save_extracted_to_memory_only(memory_id: str, message_dict, source_message_id: int, task_id: str=None):
    memory = MemoryService.get_by_memory_id(memory_id)
    if not memory:
        msg = f"Memory '{memory_id}' not found."
        if task_id:
            TaskService.update_progress(task_id, {"progress": -1, "progress_msg": timestamp_to_date(current_timestamp())+ " " + msg})
        return False, msg

    if memory.memory_type == MemoryType.RAW.value:
        msg = f"Memory '{memory_id}' don't need to extract."
        if task_id:
            TaskService.update_progress(task_id, {"progress": 1.0, "progress_msg": timestamp_to_date(current_timestamp())+ " " + msg})
        return True, msg

    tenant_id = memory.tenant_id
    extracted_content = await extract_by_llm(
        tenant_id,
        memory.llm_id,
        {"temperature": memory.temperature},
        get_memory_type_human(memory.memory_type),
        message_dict.get("user_input", ""),
        message_dict.get("agent_response", ""),
        task_id=task_id
    )
    message_list = [{
        "message_id": REDIS_CONN.generate_auto_increment_id(namespace="memory"),
        "message_type": content["message_type"],
        "source_id": source_message_id,
        "memory_id": memory_id,
        "user_id": "",
        "agent_id": message_dict["agent_id"],
        "session_id": message_dict["session_id"],
        "content": content["content"],
        "valid_at": content["valid_at"],
        "invalid_at": content["invalid_at"] if content["invalid_at"] else None,
        "forget_at": None,
        "status": True
    } for content in extracted_content]
    if not message_list:
        msg = "No memory extracted from raw message."
        if task_id:
            TaskService.update_progress(task_id, {"progress": 1.0, "progress_msg": timestamp_to_date(current_timestamp())+ " " + msg})
        return True, msg

    if task_id:
        TaskService.update_progress(task_id, {"progress": 0.5, "progress_msg": timestamp_to_date(current_timestamp())+ " " + f"Extracted {len(message_list)} messages from raw dialogue."})
    return await embed_and_save(memory, message_list, task_id)


async def extract_by_llm(tenant_id: str, llm_id: str, extract_conf: dict, memory_type: List[str], user_input: str,
                         agent_response: str, system_prompt: str = "", user_prompt: str="", task_id: str=None) -> List[dict]:
    llm_type = TenantLLMService.llm_id2llm_type(llm_id)
    if not llm_type:
        raise RuntimeError(f"Unknown type of LLM '{llm_id}'")
    if not system_prompt:
        system_prompt = PromptAssembler.assemble_system_prompt({"memory_type": memory_type})
    conversation_content = f"User Input: {user_input}\nAgent Response: {agent_response}"
    conversation_time = timestamp_to_date(current_timestamp())
    user_prompts = []
    if user_prompt:
        user_prompts.append({"role": "user", "content": user_prompt})
        user_prompts.append({"role": "user", "content": f"Conversation: {conversation_content}\nConversation Time: {conversation_time}\nCurrent Time: {conversation_time}"})
    else:
        user_prompts.append({"role": "user", "content": PromptAssembler.assemble_user_prompt(conversation_content, conversation_time, conversation_time)})
    llm = LLMBundle(tenant_id, llm_type, llm_id)
    if task_id:
        TaskService.update_progress(task_id, {"progress": 0.15, "progress_msg": timestamp_to_date(current_timestamp())+ " " + "Prepared prompts and LLM."})
    res = await llm.async_chat(system_prompt, user_prompts, extract_conf)
    res_json = get_json_result_from_llm_response(res)
    if task_id:
        TaskService.update_progress(task_id, {"progress": 0.35, "progress_msg": timestamp_to_date(current_timestamp())+ " " + "Get extracted result from LLM."})
    return [{
        "content": extracted_content["content"],
        "valid_at": format_iso_8601_to_ymd_hms(extracted_content["valid_at"]),
        "invalid_at": format_iso_8601_to_ymd_hms(extracted_content["invalid_at"]) if extracted_content.get("invalid_at") else "",
        "message_type": message_type
    } for message_type, extracted_content_list in res_json.items() for extracted_content in extracted_content_list]


async def embed_and_save(memory, message_list: list[dict], task_id: str=None):
    embedding_model = LLMBundle(memory.tenant_id, llm_type=LLMType.EMBEDDING, llm_name=memory.embd_id)
    if task_id:
        TaskService.update_progress(task_id, {"progress": 0.65, "progress_msg": timestamp_to_date(current_timestamp())+ " " + "Prepared embedding model."})
    vector_list, _ = embedding_model.encode([msg["content"] for msg in message_list])
    for idx, msg in enumerate(message_list):
        msg["content_embed"] = vector_list[idx]
    if task_id:
        TaskService.update_progress(task_id, {"progress": 0.85, "progress_msg": timestamp_to_date(current_timestamp())+ " " + "Embedded extracted content."})
    vector_dimension = len(vector_list[0])
    if not MessageService.has_index(memory.tenant_id, memory.id):
        created = MessageService.create_index(memory.tenant_id, memory.id, vector_size=vector_dimension)
        if not created:
            error_msg = "Failed to create message index."
            if task_id:
                TaskService.update_progress(task_id, {"progress": -1, "progress_msg": timestamp_to_date(current_timestamp())+ " " + error_msg})
            return False, error_msg

    new_msg_size = sum([MessageService.calculate_message_size(m) for m in message_list])
    current_memory_size = get_memory_size_cache(memory.tenant_id, memory.id)
    if new_msg_size + current_memory_size > memory.memory_size:
        size_to_delete = current_memory_size + new_msg_size - memory.memory_size
        if memory.forgetting_policy == "FIFO":
            message_ids_to_delete, delete_size = MessageService.pick_messages_to_delete_by_fifo(memory.id, memory.tenant_id,
                                                                                                size_to_delete)
            MessageService.delete_message({"message_id": message_ids_to_delete}, memory.tenant_id, memory.id)
            decrease_memory_size_cache(memory.id, delete_size)
        else:
            error_msg = "Failed to insert message into memory. Memory size reached limit and cannot decide which to delete."
            if task_id:
                TaskService.update_progress(task_id, {"progress": -1, "progress_msg": timestamp_to_date(current_timestamp())+ " " + error_msg})
            return False, error_msg
    fail_cases = MessageService.insert_message(message_list, memory.tenant_id, memory.id)
    if fail_cases:
        error_msg = "Failed to insert message into memory. Details: " + "; ".join(fail_cases)
        if task_id:
            TaskService.update_progress(task_id, {"progress": -1, "progress_msg": timestamp_to_date(current_timestamp())+ " " + error_msg})
        return False, error_msg

    if task_id:
        TaskService.update_progress(task_id, {"progress": 0.95, "progress_msg": timestamp_to_date(current_timestamp())+ " " + "Saved messages to storage."})
    increase_memory_size_cache(memory.id, new_msg_size)
    return True, "Message saved successfully."


def query_message(filter_dict: dict, params: dict):
    """
    :param filter_dict: {
        "memory_id": List[str],
        "agent_id": optional
        "session_id": optional
    }
    :param params: {
        "query": question str,
        "similarity_threshold": float,
        "keywords_similarity_weight": float,
        "top_n": int
    }
    """
    memory_ids = filter_dict["memory_id"]
    memory_list = MemoryService.get_by_ids(memory_ids)
    if not memory_list:
        return []

    condition_dict = {k: v for k, v in filter_dict.items() if v}
    uids = [memory.tenant_id for memory in memory_list]

    question = params["query"]
    question = question.strip()
    memory = memory_list[0]
    embd_model = LLMBundle(memory.tenant_id, llm_type=LLMType.EMBEDDING, llm_name=memory.embd_id)
    match_dense = get_vector(question, embd_model, similarity=params["similarity_threshold"])
    match_text, _ = MsgTextQuery().question(question, min_match=params["similarity_threshold"])
    keywords_similarity_weight = params.get("keywords_similarity_weight", 0.7)
    fusion_expr = FusionExpr("weighted_sum", params["top_n"], {"weights": ",".join([str(1 - keywords_similarity_weight), str(keywords_similarity_weight)])})

    return MessageService.search_message(memory_ids, condition_dict, uids, [match_text, match_dense, fusion_expr], params["top_n"])


def init_message_id_sequence():
    message_id_redis_key = "id_generator:memory"
    if REDIS_CONN.exist(message_id_redis_key):
        current_max_id = REDIS_CONN.get(message_id_redis_key)
        logging.info(f"No need to init message_id sequence, current max id is {current_max_id}.")
    else:
        max_id = 1
        exist_memory_list = MemoryService.get_all_memory()
        if not exist_memory_list:
            REDIS_CONN.set(message_id_redis_key, max_id)
        else:
            max_id = MessageService.get_max_message_id(
                uid_list=[m.tenant_id for m in exist_memory_list],
                memory_ids=[m.id for m in exist_memory_list]
            )
            REDIS_CONN.set(message_id_redis_key, max_id)
        logging.info(f"Init message_id sequence done, current max id is {max_id}.")


def get_memory_size_cache(memory_id: str, uid: str):
    redis_key = f"memory_{memory_id}"
    if REDIS_CONN.exist(redis_key):
        return int(REDIS_CONN.get(redis_key))
    else:
        memory_size_map = MessageService.calculate_memory_size(
            [memory_id],
            [uid]
        )
        memory_size = memory_size_map.get(memory_id, 0)
        set_memory_size_cache(memory_id, memory_size)
        return memory_size


def set_memory_size_cache(memory_id: str, size: int):
    redis_key = f"memory_{memory_id}"
    return REDIS_CONN.set(redis_key, size)


def increase_memory_size_cache(memory_id: str, size: int):
    redis_key = f"memory_{memory_id}"
    return REDIS_CONN.incrby(redis_key, size)


def decrease_memory_size_cache(memory_id: str, size: int):
    redis_key = f"memory_{memory_id}"
    return REDIS_CONN.decrby(redis_key, size)


def init_memory_size_cache():
    memory_list = MemoryService.get_all_memory()
    if not memory_list:
        logging.info("No memory found, no need to init memory size.")
    else:
        for m in memory_list:
            get_memory_size_cache(m.id, m.tenant_id)
        logging.info("Memory size cache init done.")


def fix_missing_tokenized_memory():
    if settings.DOC_ENGINE != "elasticsearch":
        logging.info("Not using elasticsearch as doc engine, no need to fix missing tokenized memory.")
        return
    memory_list = MemoryService.get_all_memory()
    if not memory_list:
        logging.info("No memory found, no need to fix missing tokenized memory.")
    else:
        for m in memory_list:
            message_list = MessageService.get_missing_field_messages(m.id, m.tenant_id, "tokenized_content_ltks")
            for msg in message_list:
                # update content to refresh tokenized field
                MessageService.update_message({"message_id": msg["message_id"], "memory_id": m.id}, {"content": msg["content"]}, m.tenant_id, m.id)
            if message_list:
                logging.info(f"Fixed {len(message_list)} messages missing tokenized field in memory: {m.name}.")
        logging.info("Fix missing tokenized memory done.")


def judge_system_prompt_is_default(system_prompt: str, memory_type: int|list[str]):
    memory_type_list = memory_type if isinstance(memory_type, list) else get_memory_type_human(memory_type)
    return system_prompt == PromptAssembler.assemble_system_prompt({"memory_type": memory_type_list})


async def queue_save_to_memory_task(memory_ids: list[str], message_dict: dict):
    """
    :param memory_ids:
    :param message_dict: {
        "user_id": str,
        "agent_id": str,
        "session_id": str,
        "user_input": str,
        "agent_response": str
    }
    """
    def new_task(_memory_id: str, _source_id: int):
        return {
            "id": get_uuid(),
            "doc_id": _memory_id,
            "task_type": "memory",
            "progress": 0.0,
            "digest": str(_source_id)
        }

    not_found_memory = []
    failed_memory = []
    for memory_id in memory_ids:
        memory = MemoryService.get_by_memory_id(memory_id)
        if not memory:
            not_found_memory.append(memory_id)
            continue

        raw_message_id = REDIS_CONN.generate_auto_increment_id(namespace="memory")
        raw_message = {
            "message_id": raw_message_id,
            "message_type": MemoryType.RAW.name.lower(),
            "source_id": 0,
            "memory_id": memory_id,
            "user_id": "",
            "agent_id": message_dict["agent_id"],
            "session_id": message_dict["session_id"],
            "content": f"User Input: {message_dict.get('user_input')}\nAgent Response: {message_dict.get('agent_response')}",
            "valid_at": timestamp_to_date(current_timestamp()),
            "invalid_at": None,
            "forget_at": None,
            "status": True
        }
        res, msg = await embed_and_save(memory, [raw_message])
        if not res:
            failed_memory.append({"memory_id": memory_id, "fail_msg": msg})
            continue

        task = new_task(memory_id, raw_message_id)
        bulk_insert_into_db(Task, [task], replace_on_conflict=True)
        task_message = {
            "id": task["id"],
            "task_id": task["id"],
            "task_type": task["task_type"],
            "memory_id": memory_id,
            "source_id": raw_message_id,
            "message_dict": message_dict
        }
        if not REDIS_CONN.queue_product(settings.get_svr_queue_name(priority=0), message=task_message):
            failed_memory.append({"memory_id": memory_id, "fail_msg": "Can't access Redis."})

    error_msg = ""
    if not_found_memory:
        error_msg = f"Memory {not_found_memory} not found."
    if failed_memory:
        error_msg += "".join([f"Memory {fm['memory_id']} failed. Detail: {fm['fail_msg']}" for fm in failed_memory])

    if error_msg:
        return False, error_msg

    return True, "All add to task."


async def handle_save_to_memory_task(task_param: dict):
    """
    :param task_param: {
        "id": task_id
        "memory_id": id
        "source_id": id
        "message_dict": {
            "user_id": str,
            "agent_id": str,
            "session_id": str,
            "user_input": str,
            "agent_response": str
        }
    }
    """
    _, task = TaskService.get_by_id(task_param["id"])
    if not task:
        return False, f"Task {task_param['id']} is not found."
    if task.progress == -1:
        return False, f"Task {task_param['id']} is already failed."
    now_time = current_timestamp()
    TaskService.update_by_id(task_param["id"], {"begin_at": timestamp_to_date(now_time)})

    memory_id = task_param["memory_id"]
    source_id = task_param["source_id"]
    message_dict = task_param["message_dict"]
    success, msg = await save_extracted_to_memory_only(memory_id, message_dict, source_id, task.id)
    if success:
        TaskService.update_progress(task.id, {"progress": 1.0,  "progress_msg": timestamp_to_date(current_timestamp())+ " " + msg})
        return True, msg

    logging.error(msg)
    TaskService.update_progress(task.id, {"progress": -1, "progress_msg": timestamp_to_date(current_timestamp())+ " " + msg})
    return False, msg

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

from typing import List

from common.time_utils import current_timestamp, timestamp_to_date, format_iso_8601_to_ymd_hms
from common.constants import MemoryType, LLMType
from common.vector_store_base import FusionExpr
from api.db.services.memory_service import MemoryService
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.llm_service import LLMBundle
from api.utils.memory_utils import get_memory_type_human
from memory.services.messages import MessageService
from memory.services.query import MsgTextQueryer, get_vector
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
    embedding_model = LLMBundle(tenant_id, llm_type=LLMType.EMBEDDING, llm_name=memory.embd_id)
    vector_list, _ = embedding_model.encode([msg["content"] for msg in message_list])
    for idx, msg in enumerate(message_list):
        msg["content_embed"] = vector_list[idx]
    vector_dimension = len(vector_list[0])
    if not MessageService.has_index(tenant_id):
        created = MessageService.create_index(tenant_id, memory_id, vector_size=vector_dimension)
        if not created:
            return False, "Failed to create message index."

    fail_cases = MessageService.insert_message(message_list, tenant_id, memory_id)
    if fail_cases:
        return False, "Failed to insert message into memory. Details: " + "; ".join(fail_cases)

    return True, "Message saved successfully."


async def extract_by_llm(tenant_id: str, llm_id: str, extract_conf: dict, memory_type: List[str], user_input: str,
                         agent_response: str, system_prompt: str = "", user_prompt: str="") -> List[dict]:
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
    res = await llm.async_chat(system_prompt, user_prompts, extract_conf)
    res_json = get_json_result_from_llm_response(res)
    return [{
        "content": extracted_content["content"],
        "valid_at": format_iso_8601_to_ymd_hms(extracted_content["valid_at"]),
        "invalid_at": format_iso_8601_to_ymd_hms(extracted_content["invalid_at"]) if extracted_content.get("invalid_at") else "",
        "message_type": message_type
    } for message_type, extracted_content_list in res_json.items() for extracted_content in extracted_content_list]


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
    match_text, _ = MsgTextQueryer().question(question, min_match=0.3)
    keywords_similarity_weight = params.get("keywords_similarity_weight", 0.7)
    fusion_expr = FusionExpr("weighted_sum", params["top_n"], {"weights": ",".join([str(keywords_similarity_weight), str(1 - keywords_similarity_weight)])})

    return MessageService.search_message(memory_ids, condition_dict, uids, [match_text, match_dense, fusion_expr], params["top_n"])

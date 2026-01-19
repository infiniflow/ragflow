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
from quart import request
from api.apps import login_required
from api.db.services.memory_service import MemoryService
from common.time_utils import current_timestamp, timestamp_to_date

from memory.services.messages import MessageService
from api.db.joint_services import memory_message_service
from api.utils.api_utils import validate_request, get_request_json, get_error_argument_result, get_json_result
from common.constants import RetCode


@manager.route("", methods=["POST"]) # noqa: F821
@login_required
@validate_request("memory_id", "agent_id", "session_id", "user_input", "agent_response")
async def add_message():

    req = await get_request_json()
    memory_ids = req["memory_id"]
    agent_id = req["agent_id"]
    session_id = req["session_id"]
    user_id = req["user_id"] if req.get("user_id") else ""
    user_input = req["user_input"]
    agent_response = req["agent_response"]

    res = []
    for memory_id in memory_ids:
        success, msg = await memory_message_service.save_to_memory(
            memory_id,
            {
                "user_id": user_id,
                "agent_id": agent_id,
                "session_id": session_id,
                "user_input": user_input,
                "agent_response": agent_response
            }
        )
        res.append({
            "memory_id": memory_id,
            "success": success,
            "message": msg
        })

    if all([r["success"] for r in res]):
        return get_json_result(message="Successfully added to memories.")

    return get_json_result(code=RetCode.SERVER_ERROR, message="Some messages failed to add.", data=res)


@manager.route("/<memory_id>:<message_id>", methods=["DELETE"]) # noqa: F821
@login_required
async def forget_message(memory_id: str, message_id: int):

    memory = MemoryService.get_by_memory_id(memory_id)
    if not memory:
        return get_json_result(code=RetCode.NOT_FOUND, message=f"Memory '{memory_id}' not found.")

    forget_time = timestamp_to_date(current_timestamp())
    update_succeed = MessageService.update_message(
        {"memory_id": memory_id, "message_id": int(message_id)},
        {"forget_at": forget_time},
        memory.tenant_id, memory_id)
    if update_succeed:
        return get_json_result(message=update_succeed)
    else:
        return get_json_result(code=RetCode.SERVER_ERROR, message=f"Failed to forget message '{message_id}' in memory '{memory_id}'.")


@manager.route("/<memory_id>:<message_id>", methods=["PUT"]) # noqa: F821
@login_required
@validate_request("status")
async def update_message(memory_id: str, message_id: int):
    req = await get_request_json()
    status = req["status"]
    if not isinstance(status, bool):
        return get_error_argument_result("Status must be a boolean.")

    memory = MemoryService.get_by_memory_id(memory_id)
    if not memory:
        return get_json_result(code=RetCode.NOT_FOUND, message=f"Memory '{memory_id}' not found.")

    update_succeed = MessageService.update_message({"memory_id": memory_id, "message_id": int(message_id)}, {"status": status}, memory.tenant_id, memory_id)
    if update_succeed:
        return get_json_result(message=update_succeed)
    else:
        return get_json_result(code=RetCode.SERVER_ERROR, message=f"Failed to set status for message '{message_id}' in memory '{memory_id}'.")


@manager.route("/search", methods=["GET"]) # noqa: F821
@login_required
async def search_message():
    args = request.args
    print(args, flush=True)
    empty_fields = [f for f in ["memory_id", "query"] if not args.get(f)]
    if empty_fields:
        return get_error_argument_result(f"{', '.join(empty_fields)} can't be empty.")

    memory_ids = args.getlist("memory_id")
    query = args.get("query")
    similarity_threshold = float(args.get("similarity_threshold", 0.2))
    keywords_similarity_weight = float(args.get("keywords_similarity_weight", 0.7))
    top_n = int(args.get("top_n", 5))
    agent_id = args.get("agent_id", "")
    session_id = args.get("session_id", "")

    filter_dict = {
        "memory_id": memory_ids,
        "agent_id": agent_id,
        "session_id": session_id
    }
    params = {
        "query": query,
        "similarity_threshold": similarity_threshold,
        "keywords_similarity_weight": keywords_similarity_weight,
        "top_n": top_n
    }
    res = memory_message_service.query_message(filter_dict, params)
    return get_json_result(message=True, data=res)


@manager.route("", methods=["GET"]) # noqa: F821
@login_required
async def get_messages():
    args = request.args
    memory_ids = args.getlist("memory_id")
    agent_id = args.get("agent_id", "")
    session_id = args.get("session_id", "")
    limit = int(args.get("limit", 10))
    if not memory_ids:
        return get_error_argument_result("memory_ids is required.")
    memory_list = MemoryService.get_by_ids(memory_ids)
    uids = [memory.tenant_id for memory in memory_list]
    res = MessageService.get_recent_messages(
        uids,
        memory_ids,
        agent_id,
        session_id,
        limit
    )
    return get_json_result(message=True, data=res)


@manager.route("/<memory_id>:<message_id>/content", methods=["GET"]) # noqa: F821
@login_required
async def get_message_content(memory_id:str, message_id: int):
    memory = MemoryService.get_by_memory_id(memory_id)
    if not memory:
        return get_json_result(code=RetCode.NOT_FOUND, message=f"Memory '{memory_id}' not found.")

    res = MessageService.get_by_message_id(memory_id, message_id, memory.tenant_id)
    if res:
        return get_json_result(message=True, data=res)
    else:
        return get_json_result(code=RetCode.NOT_FOUND, message=f"Message '{message_id}' in memory '{memory_id}' not found.")

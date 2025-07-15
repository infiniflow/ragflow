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

from flask import request
from api.db import StatusEnum
from api.db.services.dialog_service import DialogService
from api.db.services.memory_service import memory_service
from api.utils.api_utils import get_error_data_result, get_result, token_required


@manager.route("/chats/<chat_id>/memories", methods=["GET"])  # noqa: F821
@token_required
def list_memories(tenant_id, chat_id):
    """List all memories for a chat"""
    if not DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value):
        return get_error_data_result(message=f"You don't own the chat {chat_id}")
    
    if not memory_service.is_enabled():
        return get_error_data_result(message="Memory service is not enabled")
    
    user_id = request.args.get("user_id", tenant_id)
    
    try:
        memories = memory_service.get_all_memories(user_id=user_id, dialog_id=chat_id)
        return get_result(data=memories)
    except Exception as e:
        return get_error_data_result(message=f"Failed to retrieve memories: {str(e)}")


@manager.route("/chats/<chat_id>/memories/<memory_id>", methods=["DELETE"])  # noqa: F821
@token_required
def delete_memory(tenant_id, chat_id, memory_id):
    """Delete a specific memory"""
    if not DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value):
        return get_error_data_result(message=f"You don't own the chat {chat_id}")
    
    if not memory_service.is_enabled():
        return get_error_data_result(message="Memory service is not enabled")
    
    user_id = request.args.get("user_id", tenant_id)
    
    try:
        success = memory_service.delete_memory(user_id=user_id, dialog_id=chat_id, memory_id=memory_id)
        if success:
            return get_result(message="Memory deleted successfully")
        else:
            return get_error_data_result(message="Failed to delete memory")
    except Exception as e:
        return get_error_data_result(message=f"Failed to delete memory: {str(e)}")


@manager.route("/chats/<chat_id>/memories/search", methods=["POST"])  # noqa: F821
@token_required
def search_memories(tenant_id, chat_id):
    """Search memories by query"""
    if not DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value):
        return get_error_data_result(message=f"You don't own the chat {chat_id}")
    
    if not memory_service.is_enabled():
        return get_error_data_result(message="Memory service is not enabled")
    
    req = request.json
    if not req or not req.get("query"):
        return get_error_data_result(message="Query is required")
    
    user_id = req.get("user_id", tenant_id)
    query = req.get("query")
    limit = req.get("limit", 10)
    
    try:
        memories = memory_service.get_relevant_memories(
            user_id=user_id,
            dialog_id=chat_id,
            query=query,
            limit=limit
        )
        return get_result(data=memories)
    except Exception as e:
        return get_error_data_result(message=f"Failed to search memories: {str(e)}")


@manager.route("/chats/<chat_id>/memories", methods=["DELETE"])  # noqa: F821
@token_required
def clear_memories(tenant_id, chat_id):
    """Clear all memories for a chat"""
    if not DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value):
        return get_error_data_result(message=f"You don't own the chat {chat_id}")
    
    if not memory_service.is_enabled():
        return get_error_data_result(message="Memory service is not enabled")
    
    user_id = request.args.get("user_id", tenant_id)
    
    try:
        success = memory_service.clear_memories(user_id=user_id, dialog_id=chat_id)
        if success:
            return get_result(message="All memories cleared successfully")
        else:
            return get_error_data_result(message="Failed to clear memories")
    except Exception as e:
        return get_error_data_result(message=f"Failed to clear memories: {str(e)}")


@manager.route("/chats/<chat_id>/memories/stats", methods=["GET"])  # noqa: F821
@token_required
def get_memory_stats(tenant_id, chat_id):
    """Get memory statistics for a chat"""
    if not DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value):
        return get_error_data_result(message=f"You don't own the chat {chat_id}")
    
    if not memory_service.is_enabled():
        return get_result(data={"enabled": False, "total_memories": 0})
    
    user_id = request.args.get("user_id", tenant_id)
    
    try:
        stats = memory_service.get_memory_stats(user_id=user_id, dialog_id=chat_id)
        return get_result(data=stats)
    except Exception as e:
        return get_error_data_result(message=f"Failed to get memory stats: {str(e)}")

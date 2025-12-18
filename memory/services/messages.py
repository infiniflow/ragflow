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

from common import settings
from memory.utils.msg_store_conn import OrderByExpr, MatchExpr
from memory.utils.msg_util import map_message_to_storage_fields, get_message_from_storage_doc


def index_name(uid: str): return f"memory_{uid}"


class MessageService:

    @classmethod
    def has_index(cls, uid: str):
        index = index_name(uid)
        return settings.msgStoreConn.indexExist(index)

    @classmethod
    def create_index(cls, uid: str, memory_id: str, vector_size: int):
        index = index_name(uid)
        return settings.msgStoreConn.createIdx(index, memory_id, vector_size)

    @classmethod
    def delete_index(cls, uid: str):
        index = index_name(uid)
        return settings.msgStoreConn.deleteIdx(index)

    @classmethod
    def insert_message(cls, messages: List[dict], uid: str, memory_id: str):
        index = index_name(uid)
        docs = [map_message_to_storage_fields(m) for m in messages]
        [d.update({"id": f'{memory_id}_{d["message_id"]}'}) for d in docs]
        return settings.msgStoreConn.insert(docs, index, memory_id)

    @classmethod
    def update_message(cls, condition: dict, update_dict: dict, uid: str, memory_id: str):
        index = index_name(uid)
        if "status" in update_dict:
            target_status = update_dict.pop("status")
            update_dict["status_int"] = 1 if target_status else 0
        return settings.msgStoreConn.update(condition, update_dict, index, memory_id)

    @classmethod
    def delete_message(cls, condition: dict, uid: str, memory_id: str):
        index = index_name(uid)
        return settings.msgStoreConn.delete(condition, index, memory_id)

    @classmethod
    def list_message(cls, uid: str, memory_id: str, agent_ids: List[str]=None, keywords: str=None, page: int=1, page_size: int=50):
        index = index_name(uid)
        filter_dict = {}
        if agent_ids:
            filter_dict["agent_id"] = agent_ids
        if keywords:
            filter_dict["session_id"] = keywords
        order_by = OrderByExpr()
        order_by.desc("valid_at")
        res = settings.msgStoreConn.search(
            selectFields=[
                "message_id", "message_type_kwd", "source_id", "memory_id", "user_id", "agent_id", "session_id", "valid_at",
                "invalid_at", "forget_at", "status_int"
            ],
            highlightFields=[],
            condition=filter_dict,
            matchExprs=[], orderBy=order_by,
            offset=(page-1)*page_size, limit=page_size,
            indexNames=index, memoryIds=[memory_id],
        )
        total_count = settings.msgStoreConn.get_total(res)
        doc_mapping = settings.msgStoreConn.get_fields(res, [
            "message_id", "message_type_kwd", "source_id", "memory_id","user_id", "agent_id", "session_id",
            "valid_at", "invalid_at", "forget_at", "status_int"
        ])
        return {
            "message_list": [get_message_from_storage_doc(d) for d in doc_mapping.values()],
            "total_count": total_count
        }

    @classmethod
    def get_recent_messages(cls, uids: List[str], memory_ids: List[str], agent_id: str, session_id: str, limit: int):
        index_names = [index_name(uid) for uid in uids]
        condition_dict = {
            "agent_id": agent_id,
            "session_id": session_id
        }
        order_by = OrderByExpr()
        order_by.desc("valid_at")
        res = settings.msgStoreConn.search(
            selectFields=[
                "message_id", "message_type_kwd", "source_id", "memory_id", "user_id", "agent_id", "session_id", "valid_at",
                "invalid_at", "forget_at", "status_int", "content"
            ],
            highlightFields=[],
            condition=condition_dict,
            matchExprs=[], orderBy=order_by,
            offset=0, limit=limit,
            indexNames=index_names, memoryIds=memory_ids,
        )
        doc_mapping = settings.msgStoreConn.get_fields(res, [
            "message_id", "message_type_kwd", "source_id", "memory_id","user_id", "agent_id", "session_id",
            "valid_at", "invalid_at", "forget_at", "status_int", "content"
        ])
        return [get_message_from_storage_doc(d) for d in doc_mapping.values()]


    @classmethod
    def search_message(cls, memory_ids: List[str], condition_dict: dict, uids: List[str], match_expressions:list[MatchExpr], top_n: int):
        index_names = [index_name(uid) for uid in uids]
        # filter only valid messages by default
        if "status" not in condition_dict and "status_int" not in condition_dict:
            condition_dict["status_int"] = 1

        order_by = OrderByExpr()
        order_by.desc("valid_at")
        res = settings.msgStoreConn.search(
            selectFields=[
                "message_id", "message_type_kwd", "source_id", "memory_id", "user_id", "agent_id", "session_id",
                "valid_at",
                "invalid_at", "forget_at", "status_int", "content"
            ],
            highlightFields=[],
            condition=condition_dict,
            matchExprs=match_expressions,
            orderBy=order_by,
            offset=0, limit=top_n,
            indexNames=index_names, memoryIds=memory_ids,
        )
        docs = settings.msgStoreConn.get_fields(res, [
            "message_id", "message_type_kwd", "source_id", "memory_id", "user_id", "agent_id", "session_id", "valid_at",
            "invalid_at", "forget_at", "status_int", "content"
        ])
        return [get_message_from_storage_doc(d) for d in docs.values()]


    @classmethod
    def get_by_message_id(cls, memory_id: str, message_id: int, uid: str):
        index = index_name(uid)
        doc_id = f'{memory_id}_{message_id}'
        raw_doc = settings.msgStoreConn.get(doc_id, index, [memory_id])
        return get_message_from_storage_doc(raw_doc)

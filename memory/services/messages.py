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
import sys
from typing import List

from common import settings
from common.constants import MemoryType
from common.doc_store.doc_store_base import OrderByExpr, MatchExpr


def index_name(uid: str): return f"memory_{uid}"


class MessageService:

    @classmethod
    def has_index(cls, uid: str, memory_id: str):
        index = index_name(uid)
        return settings.msgStoreConn.index_exist(index, memory_id)

    @classmethod
    def create_index(cls, uid: str, memory_id: str, vector_size: int):
        index = index_name(uid)
        return settings.msgStoreConn.create_idx(index, memory_id, vector_size)

    @classmethod
    def delete_index(cls, uid: str, memory_id: str):
        index = index_name(uid)
        return settings.msgStoreConn.delete_idx(index, memory_id)

    @classmethod
    def insert_message(cls, messages: List[dict], uid: str, memory_id: str):
        index = index_name(uid)
        [m.update({
            "id": f'{memory_id}_{m["message_id"]}',
            "status": 1 if m["status"] else 0
        }) for m in messages]
        return settings.msgStoreConn.insert(messages, index, memory_id)

    @classmethod
    def update_message(cls, condition: dict, update_dict: dict, uid: str, memory_id: str):
        index = index_name(uid)
        if "status" in update_dict:
            update_dict["status"] = 1 if update_dict["status"] else 0
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
        select_fields = [
            "message_id", "message_type", "source_id", "memory_id", "user_id", "agent_id", "session_id", "valid_at",
            "invalid_at", "forget_at", "status"
        ]
        order_by = OrderByExpr()
        order_by.desc("valid_at")
        res, total_count = settings.msgStoreConn.search(
            select_fields=select_fields,
            highlight_fields=[],
            condition={**filter_dict, "message_type": MemoryType.RAW.name.lower()},
            match_expressions=[], order_by=order_by,
            offset=(page-1)*page_size, limit=page_size,
            index_names=index, memory_ids=[memory_id], agg_fields=[], hide_forgotten=False
        )
        if not total_count:
            return {
            "message_list": [],
            "total_count": 0
        }

        raw_msg_mapping = settings.msgStoreConn.get_fields(res, select_fields)
        raw_messages = list(raw_msg_mapping.values())
        extract_filter = {"source_id": [r["message_id"] for r in raw_messages]}
        extract_res, _ = settings.msgStoreConn.search(
            select_fields=select_fields,
            highlight_fields=[],
            condition=extract_filter,
            match_expressions=[], order_by=order_by,
            offset=0, limit=512,
            index_names=index, memory_ids=[memory_id], agg_fields=[], hide_forgotten=False
        )
        extract_msg = settings.msgStoreConn.get_fields(extract_res, select_fields)
        grouped_extract_msg = {}
        for msg in extract_msg.values():
            if grouped_extract_msg.get(msg["source_id"]):
                grouped_extract_msg[msg["source_id"]].append(msg)
            else:
                grouped_extract_msg[msg["source_id"]] = [msg]

        for raw_msg in raw_messages:
            raw_msg["extract"] = grouped_extract_msg.get(raw_msg["message_id"], [])

        return {
            "message_list": raw_messages,
            "total_count": total_count
        }

    @classmethod
    def get_recent_messages(cls, uid_list: List[str], memory_ids: List[str], agent_id: str, session_id: str, limit: int):
        index_names = [index_name(uid) for uid in uid_list]
        condition_dict = {
            "agent_id": agent_id,
            "session_id": session_id
        }
        order_by = OrderByExpr()
        order_by.desc("valid_at")
        res, total_count = settings.msgStoreConn.search(
            select_fields=[
                "message_id", "message_type", "source_id", "memory_id", "user_id", "agent_id", "session_id", "valid_at",
                "invalid_at", "forget_at", "status", "content"
            ],
            highlight_fields=[],
            condition=condition_dict,
            match_expressions=[], order_by=order_by,
            offset=0, limit=limit,
            index_names=index_names, memory_ids=memory_ids, agg_fields=[]
        )
        if not total_count:
            return []

        doc_mapping = settings.msgStoreConn.get_fields(res, [
            "message_id", "message_type", "source_id", "memory_id","user_id", "agent_id", "session_id",
            "valid_at", "invalid_at", "forget_at", "status", "content"
        ])
        return list(doc_mapping.values())

    @classmethod
    def search_message(cls, memory_ids: List[str], condition_dict: dict, uid_list: List[str], match_expressions:list[MatchExpr], top_n: int):
        index_names = [index_name(uid) for uid in uid_list]
        # filter only valid messages by default
        if "status" not in condition_dict:
            condition_dict["status"] = 1

        order_by = OrderByExpr()
        order_by.desc("valid_at")
        res, total_count = settings.msgStoreConn.search(
            select_fields=[
                "message_id", "message_type", "source_id", "memory_id", "user_id", "agent_id", "session_id",
                "valid_at",
                "invalid_at", "forget_at", "status", "content"
            ],
            highlight_fields=[],
            condition=condition_dict,
            match_expressions=match_expressions,
            order_by=order_by,
            offset=0, limit=top_n,
            index_names=index_names, memory_ids=memory_ids, agg_fields=[]
        )
        if not total_count:
            return []

        docs = settings.msgStoreConn.get_fields(res, [
            "message_id", "message_type", "source_id", "memory_id", "user_id", "agent_id", "session_id", "valid_at",
            "invalid_at", "forget_at", "status", "content"
        ])
        return list(docs.values())

    @staticmethod
    def calculate_message_size(message: dict):
        return sys.getsizeof(message["content"]) + sys.getsizeof(message["content_embed"][0]) * len(message["content_embed"])

    @classmethod
    def calculate_memory_size(cls, memory_ids: List[str], uid_list: List[str]):
        index_names = [index_name(uid) for uid in uid_list]
        order_by = OrderByExpr()
        order_by.desc("valid_at")

        res, count = settings.msgStoreConn.search(
            select_fields=["memory_id", "content", "content_embed"],
            highlight_fields=[],
            condition={},
            match_expressions=[],
            order_by=order_by,
            offset=0, limit=2048*len(memory_ids),
            index_names=index_names, memory_ids=memory_ids, agg_fields=[], hide_forgotten=False
        )

        if count == 0:
            return {}

        docs = settings.msgStoreConn.get_fields(res, ["memory_id", "content", "content_embed"])
        size_dict = {}
        for doc in docs.values():
            if size_dict.get(doc["memory_id"]):
                size_dict[doc["memory_id"]] += cls.calculate_message_size(doc)
            else:
                size_dict[doc["memory_id"]] = cls.calculate_message_size(doc)
        return size_dict

    @classmethod
    def pick_messages_to_delete_by_fifo(cls, memory_id: str, uid: str, size_to_delete: int):
        select_fields = ["message_id", "content", "content_embed"]
        _index_name = index_name(uid)
        res = settings.msgStoreConn.get_forgotten_messages(select_fields, _index_name, memory_id)
        current_size = 0
        ids_to_remove = []
        if res:
            message_list = settings.msgStoreConn.get_fields(res, select_fields)
            for message in message_list.values():
                if current_size < size_to_delete:
                    current_size += cls.calculate_message_size(message)
                    ids_to_remove.append(message["message_id"])
                else:
                    return ids_to_remove, current_size
            if current_size >= size_to_delete:
                return ids_to_remove, current_size

        order_by = OrderByExpr()
        order_by.asc("valid_at")
        res, total_count = settings.msgStoreConn.search(
            select_fields=select_fields,
            highlight_fields=[],
            condition={},
            match_expressions=[],
            order_by=order_by,
            offset=0, limit=512,
            index_names=[_index_name], memory_ids=[memory_id], agg_fields=[]
        )
        docs = settings.msgStoreConn.get_fields(res, select_fields)
        for doc in docs.values():
            if current_size < size_to_delete:
                current_size += cls.calculate_message_size(doc)
                ids_to_remove.append(doc["message_id"])
            else:
                return ids_to_remove, current_size
        return ids_to_remove, current_size

    @classmethod
    def get_missing_field_messages(cls, memory_id: str, uid: str, field_name: str):
        select_fields = ["message_id", "content"]
        _index_name = index_name(uid)
        res = settings.msgStoreConn.get_missing_field_message(
            select_fields=select_fields,
            index_name=_index_name,
            memory_id=memory_id,
            field_name=field_name
        )
        if not res:
            return []
        docs = settings.msgStoreConn.get_fields(res, select_fields)
        return list(docs.values())

    @classmethod
    def get_by_message_id(cls, memory_id: str, message_id: int, uid: str):
        index = index_name(uid)
        doc_id = f'{memory_id}_{message_id}'
        return settings.msgStoreConn.get(doc_id, index, [memory_id])

    @classmethod
    def get_max_message_id(cls, uid_list: List[str], memory_ids: List[str]):
        order_by = OrderByExpr()
        order_by.desc("message_id")
        index_names = [index_name(uid) for uid in uid_list]
        res, total_count = settings.msgStoreConn.search(
            select_fields=["message_id"],
            highlight_fields=[],
            condition={},
            match_expressions=[],
            order_by=order_by,
            offset=0, limit=1,
            index_names=index_names, memory_ids=memory_ids,
            agg_fields=[], hide_forgotten=False
        )
        if not total_count:
            return 1

        docs = settings.msgStoreConn.get_fields(res, ["message_id"])
        if not docs:
            return 1
        else:
            latest_msg = list(docs.values())[0]
            return int(latest_msg["message_id"])

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

from api.db.db_models import DB, Memory, User
from api.db.services import duplicate_name
from api.db.services.common_service import CommonService
from api.utils.memory_utils import calculate_memory_type
from api.constants import MEMORY_NAME_LIMIT
from common.misc_utils import get_uuid
from common.time_utils import get_format_time, current_timestamp
from memory.utils.prompt_util import PromptAssembler


class MemoryService(CommonService):
    # Service class for manage memory operations
    model = Memory

    @classmethod
    @DB.connection_context()
    def get_by_memory_id(cls, memory_id: str):
        return cls.model.select().where(cls.model.id == memory_id).first()

    @classmethod
    @DB.connection_context()
    def get_by_tenant_id(cls, tenant_id: str):
        return cls.model.select().where(cls.model.tenant_id == tenant_id)

    @classmethod
    @DB.connection_context()
    def get_all_memory(cls):
        memory_list = cls.model.select()
        return list(memory_list)

    @classmethod
    @DB.connection_context()
    def get_with_owner_name_by_id(cls, memory_id: str):
        fields = [
            cls.model.id,
            cls.model.name,
            cls.model.avatar,
            cls.model.tenant_id,
            User.nickname.alias("owner_name"),
            cls.model.memory_type,
            cls.model.storage_type,
            cls.model.embd_id,
            cls.model.llm_id,
            cls.model.permissions,
            cls.model.description,
            cls.model.memory_size,
            cls.model.forgetting_policy,
            cls.model.temperature,
            cls.model.system_prompt,
            cls.model.user_prompt,
            cls.model.create_date,
            cls.model.create_time
        ]
        memory = cls.model.select(*fields).join(User, on=(cls.model.tenant_id == User.id)).where(
            cls.model.id == memory_id
        ).first()
        return memory

    @classmethod
    @DB.connection_context()
    def get_by_filter(cls, filter_dict: dict, keywords: str, page: int = 1, page_size: int = 50):
        fields = [
            cls.model.id,
            cls.model.name,
            cls.model.avatar,
            cls.model.tenant_id,
            User.nickname.alias("owner_name"),
            cls.model.memory_type,
            cls.model.storage_type,
            cls.model.permissions,
            cls.model.description,
            cls.model.create_time,
            cls.model.create_date
        ]
        memories = cls.model.select(*fields).join(User, on=(cls.model.tenant_id == User.id))
        if filter_dict.get("tenant_id"):
            memories = memories.where(cls.model.tenant_id.in_(filter_dict["tenant_id"]))
        if filter_dict.get("memory_type"):
            memory_type_int = calculate_memory_type(filter_dict["memory_type"])
            memories = memories.where(cls.model.memory_type.bin_and(memory_type_int) > 0)
        if filter_dict.get("storage_type"):
            memories = memories.where(cls.model.storage_type == filter_dict["storage_type"])
        if keywords:
            memories = memories.where(cls.model.name.contains(keywords))
        count = memories.count()
        memories = memories.order_by(cls.model.update_time.desc())
        memories = memories.paginate(page, page_size)

        return list(memories.dicts()), count

    @classmethod
    @DB.connection_context()
    def create_memory(cls, tenant_id: str, name: str, memory_type: List[str], embd_id: str, llm_id: str):
        # Deduplicate name within tenant
        memory_name = duplicate_name(
            cls.query,
            name=name,
            tenant_id=tenant_id
        )
        if len(memory_name) > MEMORY_NAME_LIMIT:
            return False, f"Memory name {memory_name} exceeds limit of {MEMORY_NAME_LIMIT}."

        timestamp = current_timestamp()
        format_time = get_format_time()
        # build create dict
        memory_info = {
            "id": get_uuid(),
            "name": memory_name,
            "memory_type": calculate_memory_type(memory_type),
            "tenant_id": tenant_id,
            "embd_id": embd_id,
            "llm_id": llm_id,
            "system_prompt": PromptAssembler.assemble_system_prompt({"memory_type": memory_type}),
            "create_time": timestamp,
            "create_date": format_time,
            "update_time": timestamp,
            "update_date": format_time,
        }
        obj = cls.model(**memory_info).save(force_insert=True)

        if not obj:
            return False, "Could not create new memory."

        db_row = cls.model.select().where(cls.model.id == memory_info["id"]).first()

        return obj, db_row

    @classmethod
    @DB.connection_context()
    def update_memory(cls, tenant_id: str, memory_id: str, update_dict: dict):
        if not update_dict:
            return 0
        if "temperature" in update_dict and isinstance(update_dict["temperature"], str):
            update_dict["temperature"] = float(update_dict["temperature"])
        if "memory_type" in update_dict and isinstance(update_dict["memory_type"], list):
            update_dict["memory_type"] = calculate_memory_type(update_dict["memory_type"])
        if "name" in update_dict:
            update_dict["name"] = duplicate_name(
                cls.query,
                name=update_dict["name"],
                tenant_id=tenant_id
            )
        update_dict.update({
            "update_time": current_timestamp(),
            "update_date": get_format_time()
        })

        return cls.model.update(update_dict).where(cls.model.id == memory_id).execute()

    @classmethod
    @DB.connection_context()
    def delete_memory(cls, memory_id: str):
        return cls.delete_by_id(memory_id)

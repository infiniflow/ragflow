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
from common.constants import MemoryType

def format_ret_data_from_memory(memory):
    return {
        "id": memory.id,
        "name": memory.name,
        "avatar": memory.avatar,
        "tenant_id": memory.tenant_id,
        "owner_name": memory.owner_name if hasattr(memory, "owner_name") else None,
        "memory_type": get_memory_type_human(memory.memory_type),
        "storage_type": memory.storage_type,
        "embd_id": memory.embd_id,
        "llm_id": memory.llm_id,
        "permissions": memory.permissions,
        "description": memory.description,
        "memory_size": memory.memory_size,
        "forgetting_policy": memory.forgetting_policy,
        "temperature": memory.temperature,
        "system_prompt": memory.system_prompt,
        "user_prompt": memory.user_prompt,
        "create_time": memory.create_time,
        "create_date": memory.create_date,
        "update_time": memory.update_time,
        "update_date": memory.update_date
    }


def get_memory_type_human(memory_type: int) -> List[str]:
    return [mem_type.name.lower() for mem_type in MemoryType if memory_type & mem_type.value]


def calculate_memory_type(memory_type_name_list: List[str]) -> int:
    memory_type = 0
    type_value_map = {mem_type.name.lower(): mem_type.value for mem_type in MemoryType}
    for mem_type in memory_type_name_list:
        if mem_type in type_value_map:
            memory_type |= type_value_map[mem_type]
    return memory_type

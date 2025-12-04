#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import json

def format_ret_data_from_memory(memory):
    return {
        "memory_id": memory.memory_id,
        "memory_name": memory.memory_name,
        "avatar": memory.avatar,
        "tenant_id": memory.tenant_id,
        "memory_type": json.loads(memory.memory_type),
        "storage_type": memory.storage_type,
        "embedding": memory.embedding,
        "llm": memory.llm,
        "permissions": memory.permissions,
        "description": memory.description,
        "memory_size": memory.memory_size,
        "forgetting_policy": memory.forgetting_policy,
        "temperature": json.dumps(memory.temperature),
        "system_prompt": memory.system_prompt,
        "user_prompt": memory.user_prompt,
        "create_time": memory.create_time,
        "create_date": memory.create_date,
        "update_time": memory.update_time,
        "update_date": memory.update_date
    }
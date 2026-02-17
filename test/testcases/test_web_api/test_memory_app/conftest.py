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
import pytest
import random
from test_web_api.common import create_memory, list_memory, delete_memory

@pytest.fixture(scope="function")
def add_memory_func(request, WebApiAuth):
    def cleanup():
        memory_list_res = list_memory(WebApiAuth)
        exist_memory_ids = [memory["id"] for memory in memory_list_res["data"]["memory_list"]]
        for memory_id in exist_memory_ids:
            delete_memory(WebApiAuth, memory_id)

    request.addfinalizer(cleanup)

    memory_ids = []
    for i in range(3):
        payload = {
            "name": f"test_memory_{i}",
            "memory_type": ["raw"] + random.choices(["semantic", "episodic", "procedural"], k=random.randint(0, 3)),
            "embd_id": "BAAI/bge-large-zh-v1.5@SILICONFLOW",
            "llm_id": "glm-4-flash@ZHIPU-AI"
        }
        res = create_memory(WebApiAuth, payload)
        memory_ids.append(res["data"]["id"])
    return memory_ids

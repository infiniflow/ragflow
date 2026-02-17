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

@pytest.fixture(scope="class")
def add_memory_func(client, request):
    def cleanup():
        memory_list_res = client.list_memory()
        exist_memory_ids = [memory.id for memory in memory_list_res["memory_list"]]
        for memory_id in exist_memory_ids:
            client.delete_memory(memory_id)

    request.addfinalizer(cleanup)

    memory_ids = []
    for i in range(3):
        payload = {
            "name": f"test_memory_{i}",
            "memory_type": ["raw"] + random.choices(["semantic", "episodic", "procedural"], k=random.randint(0, 3)),
            "embd_id": "BAAI/bge-large-zh-v1.5@SILICONFLOW",
            "llm_id": "glm-4-flash@ZHIPU-AI"
        }
        res = client.create_memory(**payload)
        memory_ids.append(res.id)
    request.cls.memory_ids = memory_ids
    return memory_ids


@pytest.fixture(scope="class")
def delete_test_memory(client, request):
    def cleanup():
        memory_list_res = client.list_memory()
        exist_memory_ids = [memory.id for memory in memory_list_res["memory_list"]]
        for memory_id in exist_memory_ids:
            client.delete_memory(memory_id)

    request.addfinalizer(cleanup)
    return

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
import time
import uuid

import pytest
import random
from test_web_api.common import create_memory, list_memory, add_message, delete_memory


@pytest.fixture(scope="class")
def add_empty_raw_type_memory(request, WebApiAuth):
    def cleanup():
        memory_list_res = list_memory(WebApiAuth)
        exist_memory_ids = [memory["id"] for memory in memory_list_res["data"]["memory_list"]]
        for _memory_id in exist_memory_ids:
            delete_memory(WebApiAuth, _memory_id)
    request.addfinalizer(cleanup)
    payload = {
        "name": "test_memory_0",
        "memory_type": ["raw"],
        "embd_id": "BAAI/bge-small-en-v1.5@Builtin",
        "llm_id": "glm-4-flash@ZHIPU-AI"
    }
    res = create_memory(WebApiAuth, payload)
    memory_id = res["data"]["id"]
    request.cls.memory_id = memory_id
    request.cls.memory_type = payload["memory_type"]
    return memory_id


@pytest.fixture(scope="class")
def add_empty_multiple_type_memory(request, WebApiAuth):
    def cleanup():
        memory_list_res = list_memory(WebApiAuth)
        exist_memory_ids = [memory["id"] for memory in memory_list_res["data"]["memory_list"]]
        for _memory_id in exist_memory_ids:
            delete_memory(WebApiAuth, _memory_id)
    request.addfinalizer(cleanup)
    payload = {
        "name": "test_memory_0",
        "memory_type": ["raw"] + random.choices(["semantic", "episodic", "procedural"], k=random.randint(1, 3)),
        "embd_id": "BAAI/bge-small-en-v1.5@Builtin",
        "llm_id": "glm-4-flash@ZHIPU-AI"
    }
    res = create_memory(WebApiAuth, payload)
    memory_id = res["data"]["id"]
    request.cls.memory_id = memory_id
    request.cls.memory_type = payload["memory_type"]
    return memory_id


@pytest.fixture(scope="class")
def add_2_multiple_type_memory(request, WebApiAuth):
    def cleanup():
        memory_list_res = list_memory(WebApiAuth)
        exist_memory_ids = [memory["id"] for memory in memory_list_res["data"]["memory_list"]]
        for _memory_id in exist_memory_ids:
            delete_memory(WebApiAuth, _memory_id)

    request.addfinalizer(cleanup)
    memory_ids = []
    for i in range(2):
        payload = {
            "name": f"test_memory_{i}",
            "memory_type": ["raw"] + random.choices(["semantic", "episodic", "procedural"], k=random.randint(1, 3)),
            "embd_id": "BAAI/bge-small-en-v1.5@Builtin",
            "llm_id": "glm-4-flash@ZHIPU-AI"
        }
        res = create_memory(WebApiAuth, payload)
        memory_ids.append(res["data"]["id"])
    request.cls.memory_ids = memory_ids
    return memory_ids


@pytest.fixture(scope="class")
def add_memory_with_multiple_type_message_func(request, WebApiAuth):
    def cleanup():
        memory_list_res = list_memory(WebApiAuth)
        exist_memory_ids = [memory["id"] for memory in memory_list_res["data"]["memory_list"]]
        for _memory_id in exist_memory_ids:
            delete_memory(WebApiAuth, _memory_id)

    request.addfinalizer(cleanup)

    payload = {
        "name": "test_memory_0",
        "memory_type": ["raw"] + random.choices(["semantic", "episodic", "procedural"], k=random.randint(1, 3)),
        "embd_id": "BAAI/bge-small-en-v1.5@Builtin",
        "llm_id": "glm-4-flash@ZHIPU-AI"
    }
    res = create_memory(WebApiAuth, payload)
    memory_id = res["data"]["id"]
    agent_id = uuid.uuid4().hex
    message_payload = {
        "memory_id": [memory_id],
        "agent_id": agent_id,
        "session_id": uuid.uuid4().hex,
        "user_id": "",
        "user_input": "what is coriander?",
        "agent_response": """
Coriander is a versatile herb with two main edible parts, and its name can refer to both:
1. Leaves and Stems (often called Cilantro or Fresh Coriander): These are the fresh, green, fragrant leaves and tender stems of the plant Coriandrum sativum. They have a bright, citrusy, and sometimes pungent flavor. Cilantro is widely used as a garnish or key ingredient in cuisines like Mexican, Indian, Thai, and Middle Eastern.
2. Seeds (called Coriander Seeds): These are the dried, golden-brown seeds of the same plant. When ground, they become coriander powder. The seeds have a warm, nutty, floral, and slightly citrusy taste, completely different from the fresh leaves. They are a fundamental spice in curries, stews, pickles, and baking.
Key Point of Confusion: The naming differs by region. In North America, "coriander" typically refers to the seeds, while "cilantro" refers to the fresh leaves. In the UK, Europe, and many other parts of the world, "coriander" refers to the fresh herb, and the seeds are called "coriander seeds."
"""
    }
    add_message(WebApiAuth, message_payload)
    request.cls.memory_id = memory_id
    request.cls.agent_id = agent_id
    time.sleep(2)  # make sure refresh to index before search
    return memory_id


@pytest.fixture(scope="class")
def add_memory_with_5_raw_message_func(request, WebApiAuth):
    def cleanup():
        memory_list_res = list_memory(WebApiAuth)
        exist_memory_ids = [memory["id"] for memory in memory_list_res["data"]["memory_list"]]
        for _memory_id in exist_memory_ids:
            delete_memory(WebApiAuth, _memory_id)

    request.addfinalizer(cleanup)

    payload = {
        "name": "test_memory_1",
        "memory_type": ["raw"],
        "embd_id": "BAAI/bge-small-en-v1.5@Builtin",
        "llm_id": "glm-4-flash@ZHIPU-AI"
    }
    res = create_memory(WebApiAuth, payload)
    memory_id = res["data"]["id"]
    agent_ids = [uuid.uuid4().hex for _ in range(2)]
    session_ids = [uuid.uuid4().hex for _ in range(5)]
    for i in range(5):
        message_payload = {
            "memory_id": [memory_id],
            "agent_id": agent_ids[i % 2],
            "session_id": session_ids[i],
            "user_id": "",
            "user_input": "what is coriander?",
            "agent_response": """
Coriander is a versatile herb with two main edible parts, and its name can refer to both:
1. Leaves and Stems (often called Cilantro or Fresh Coriander): These are the fresh, green, fragrant leaves and tender stems of the plant Coriandrum sativum. They have a bright, citrusy, and sometimes pungent flavor. Cilantro is widely used as a garnish or key ingredient in cuisines like Mexican, Indian, Thai, and Middle Eastern.
2. Seeds (called Coriander Seeds): These are the dried, golden-brown seeds of the same plant. When ground, they become coriander powder. The seeds have a warm, nutty, floral, and slightly citrusy taste, completely different from the fresh leaves. They are a fundamental spice in curries, stews, pickles, and baking.
Key Point of Confusion: The naming differs by region. In North America, "coriander" typically refers to the seeds, while "cilantro" refers to the fresh leaves. In the UK, Europe, and many other parts of the world, "coriander" refers to the fresh herb, and the seeds are called "coriander seeds."
"""
        }
        add_message(WebApiAuth, message_payload)
    request.cls.memory_id = memory_id
    request.cls.agent_ids = agent_ids
    request.cls.session_ids = session_ids
    time.sleep(2) # make sure refresh to index before search
    return memory_id

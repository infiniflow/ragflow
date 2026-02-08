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
import random
import re

import pytest
from configs import INVALID_API_TOKEN, HOST_ADDRESS
from ragflow_sdk import RAGFlow
from hypothesis import example, given, settings
from utils.hypothesis_utils import valid_names


class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "invalid_auth, expected_message",
        [
            (None, "<Unauthorized '401: Unauthorized'>"),
            (INVALID_API_TOKEN, "<Unauthorized '401: Unauthorized'>"),
        ],
        ids=["empty_auth", "invalid_api_token"]
    )
    def test_auth_invalid(self, invalid_auth, expected_message):
        client = RAGFlow(invalid_auth, HOST_ADDRESS)
        with pytest.raises(Exception) as exception_info:
            client.create_memory(**{"name": "test_memory", "memory_type": ["raw"], "embd_id": "BAAI/bge-large-zh-v1.5@SILICONFLOW", "llm_id": "glm-4-flash@ZHIPU-AI"})
        assert str(exception_info.value) == expected_message, str(exception_info.value)


@pytest.mark.usefixtures("delete_test_memory")
class TestMemoryCreate:
    @pytest.mark.p1
    @given(name=valid_names())
    @example("e" * 128)
    @settings(max_examples=20)
    def test_name(self, client, name):
        payload = {
            "name": name,
            "memory_type": ["raw"] + random.choices(["semantic", "episodic", "procedural"], k=random.randint(0, 3)),
            "embd_id": "BAAI/bge-large-zh-v1.5@SILICONFLOW",
            "llm_id": "glm-4-flash@ZHIPU-AI"
        }
        memory = client.create_memory(**payload)
        pattern = rf'^{name}|{name}(?:\((\d+)\))?$'
        escaped_name = re.escape(memory.name)
        assert re.match(pattern, escaped_name), str(memory)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, expected_message",
        [
            ("", "Memory name cannot be empty or whitespace."),
            (" ", "Memory name cannot be empty or whitespace."),
            ("a" * 129, f"Memory name '{'a'*129}' exceeds limit of 128."),
        ],
        ids=["empty_name", "space_name", "too_long_name"],
    )
    def test_name_invalid(self, client, name, expected_message):
        payload = {
            "name": name,
            "memory_type": ["raw"] + random.choices(["semantic", "episodic", "procedural"], k=random.randint(0, 3)),
            "embd_id": "BAAI/bge-large-zh-v1.5@SILICONFLOW",
            "llm_id": "glm-4-flash@ZHIPU-AI"
        }
        with pytest.raises(Exception) as exception_info:
            client.create_memory(**payload)
        assert str(exception_info.value) == expected_message, str(exception_info.value)

    @pytest.mark.p2
    @given(name=valid_names())
    def test_type_invalid(self, client, name):
        payload = {
            "name": name,
            "memory_type": ["something"],
            "embd_id": "BAAI/bge-large-zh-v1.5@SILICONFLOW",
            "llm_id": "glm-4-flash@ZHIPU-AI"
        }
        with pytest.raises(Exception) as exception_info:
            client.create_memory(**payload)
        assert str(exception_info.value) == f"Memory type '{ {'something'} }' is not supported.", str(exception_info.value)

    @pytest.mark.p3
    def test_name_duplicated(self, client):
        name = "duplicated_name_test"
        payload = {
            "name": name,
            "memory_type": ["raw"] + random.choices(["semantic", "episodic", "procedural"], k=random.randint(0, 3)),
            "embd_id": "BAAI/bge-large-zh-v1.5@SILICONFLOW",
            "llm_id": "glm-4-flash@ZHIPU-AI"
        }
        res1 = client.create_memory(**payload)
        assert res1.name == name, str(res1)

        res2 = client.create_memory(**payload)
        assert res2.name == f"{name}(1)", str(res2)

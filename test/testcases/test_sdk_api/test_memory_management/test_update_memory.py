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
import pytest
from configs import INVALID_API_TOKEN, HOST_ADDRESS
from ragflow_sdk import RAGFlow, Memory
from hypothesis import HealthCheck, example, given, settings
from utils import encode_avatar
from utils.file_utils import create_image_file
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

        with pytest.raises(Exception) as exception_info:
            client = RAGFlow(invalid_auth, HOST_ADDRESS)
            memory = Memory(client, {"id": "memory_id"})
            memory.update({"name": "New_Name"})
        assert str(exception_info.value) == expected_message, str(exception_info.value)

@pytest.mark.usefixtures("add_memory_func")
class TestMemoryUpdate:

    @pytest.mark.p1
    @given(name=valid_names())
    @example("f" * 128)
    @settings(max_examples=20, suppress_health_check=[HealthCheck.function_scoped_fixture])
    def test_name(self, client, name):
        memory_ids = self.memory_ids
        update_dict = {"name": name}
        memory = Memory(client, {"id": random.choice(memory_ids)})
        res = memory.update(update_dict)
        assert res.name == name, str(res)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, expected_message",
        [
            ("", "Memory name cannot be empty or whitespace."),
            (" ", "Memory name cannot be empty or whitespace."),
            ("a" * 129, f"Memory name '{'a' * 129}' exceeds limit of 128."),
        ]
    )
    def test_name_invalid(self, client, name, expected_message):
        memory_ids = self.memory_ids
        update_dict = {"name": name}
        memory = Memory(client, {"id": random.choice(memory_ids)})
        with pytest.raises(Exception) as exception_info:
            memory.update(update_dict)
        assert str(exception_info.value) == expected_message, str(exception_info.value)

    @pytest.mark.p2
    def test_duplicate_name(self, client):
        memory_ids = self.memory_ids
        update_dict = {"name": "Test_Memory"}
        memory_0 = Memory(client, {"id": memory_ids[0]})
        res_0 = memory_0.update(update_dict)
        assert res_0.name == "Test_Memory", str(res_0)

        memory_1 = Memory(client, {"id": memory_ids[1]})
        res_1 = memory_1.update(update_dict)
        assert res_1.name == "Test_Memory(1)", str(res_1)

    @pytest.mark.p2
    def test_avatar(self, client, tmp_path):
        memory_ids = self.memory_ids
        fn = create_image_file(tmp_path / "ragflow_test.png")
        update_dict = {"avatar": f"data:image/png;base64,{encode_avatar(fn)}"}
        memory = Memory(client, {"id": random.choice(memory_ids)})
        res = memory.update(update_dict)
        assert res.avatar == f"data:image/png;base64,{encode_avatar(fn)}", str(res)

    @pytest.mark.p2
    def test_description(self, client):
        memory_ids = self.memory_ids
        description = "This is a test description."
        update_dict = {"description": description}
        memory = Memory(client, {"id": random.choice(memory_ids)})
        res = memory.update(update_dict)
        assert res.description == description, str(res)

    @pytest.mark.p1
    def test_llm(self, client):
        memory_ids = self.memory_ids
        llm_id = "glm-4@ZHIPU-AI"
        update_dict = {"llm_id": llm_id}
        memory = Memory(client, {"id": random.choice(memory_ids)})
        res = memory.update(update_dict)
        assert res.llm_id == llm_id, str(res)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "permission",
        [
            "me",
            "team"
        ],
        ids=["me", "team"]
    )
    def test_permission(self, client, permission):
        memory_ids = self.memory_ids
        update_dict = {"permissions": permission}
        memory = Memory(client, {"id": random.choice(memory_ids)})
        res = memory.update(update_dict)
        assert res.permissions == permission.lower().strip(), str(res)

    @pytest.mark.p1
    def test_memory_size(self, client):
        memory_ids = self.memory_ids
        memory_size = 1048576  # 1 MB
        update_dict = {"memory_size": memory_size}
        memory = Memory(client, {"id": random.choice(memory_ids)})
        res = memory.update(update_dict)
        assert res.memory_size == memory_size, str(res)

    @pytest.mark.p1
    def test_temperature(self, client):
        memory_ids = self.memory_ids
        temperature = 0.7
        update_dict = {"temperature": temperature}
        memory = Memory(client, {"id": random.choice(memory_ids)})
        res = memory.update(update_dict)
        assert res.temperature == temperature, str(res)

    @pytest.mark.p1
    def test_system_prompt(self, client):
        memory_ids = self.memory_ids
        system_prompt = "This is a system prompt."
        update_dict = {"system_prompt": system_prompt}
        memory = Memory(client, {"id": random.choice(memory_ids)})
        res = memory.update(update_dict)
        assert res.system_prompt == system_prompt, str(res)

    @pytest.mark.p1
    def test_user_prompt(self, client):
        memory_ids = self.memory_ids
        user_prompt = "This is a user prompt."
        update_dict = {"user_prompt": user_prompt}
        memory = Memory(client, {"id": random.choice(memory_ids)})
        res = memory.update(update_dict)
        assert res.user_prompt == user_prompt, res

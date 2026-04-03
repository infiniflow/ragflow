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
from test_web_api.common import update_memory
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth
from hypothesis import HealthCheck, example, given, settings
from utils import encode_avatar
from utils.file_utils import create_image_file
from utils.hypothesis_utils import valid_names


class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
        ids=["empty_auth", "invalid_api_token"]
    )
    def test_auth_invalid(self, invalid_auth, expected_code, expected_message):
        res = update_memory(invalid_auth, "memory_id")
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestMemoryUpdate:

    @pytest.mark.p1
    @given(name=valid_names())
    @example("f" * 128)
    @settings(max_examples=20, suppress_health_check=[HealthCheck.function_scoped_fixture])
    def test_name(self, WebApiAuth, add_memory_func, name):
        memory_ids = add_memory_func
        payload = {"name": name}
        res = update_memory(WebApiAuth, memory_ids[0], payload)
        assert res["code"] == 0, res
        assert res["data"]["name"] == name, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, expected_message",
        [
            ("", "Memory name cannot be empty or whitespace."),
            (" ", "Memory name cannot be empty or whitespace."),
            ("a" * 129, f"Memory name '{'a' * 129}' exceeds limit of 128."),
        ]
    )
    def test_name_invalid(self, WebApiAuth, add_memory_func, name, expected_message):
        memory_ids = add_memory_func
        payload = {"name": name}
        res = update_memory(WebApiAuth, memory_ids[0], payload)
        assert res["code"] == 101, res
        assert res["message"] == expected_message, res

    @pytest.mark.p2
    def test_duplicate_name(self, WebApiAuth, add_memory_func):
        memory_ids = add_memory_func
        payload = {"name": "Test_Memory"}
        res = update_memory(WebApiAuth, memory_ids[0], payload)
        assert res["code"] == 0, res

        payload = {"name": "Test_Memory"}
        res = update_memory(WebApiAuth, memory_ids[1], payload)
        assert res["code"] == 0, res
        assert res["data"]["name"] == "Test_Memory(1)", res

    @pytest.mark.p2
    def test_avatar(self, WebApiAuth, add_memory_func, tmp_path):
        memory_ids = add_memory_func
        fn = create_image_file(tmp_path / "ragflow_test.png")
        payload = {"avatar": f"data:image/png;base64,{encode_avatar(fn)}"}
        res = update_memory(WebApiAuth, memory_ids[0], payload)
        assert res["code"] == 0, res
        assert res["data"]["avatar"] == f"data:image/png;base64,{encode_avatar(fn)}", res

    @pytest.mark.p2
    def test_description(self, WebApiAuth, add_memory_func):
        memory_ids = add_memory_func
        description = "This is a test description."
        payload = {"description": description}
        res = update_memory(WebApiAuth, memory_ids[0], payload)
        assert res["code"] == 0, res
        assert res["data"]["description"] == description, res

    @pytest.mark.p1
    def test_llm(self, WebApiAuth, add_memory_func):
        memory_ids = add_memory_func
        llm_id = "glm-4@ZHIPU-AI"
        payload = {"llm_id": llm_id}
        res = update_memory(WebApiAuth, memory_ids[0], payload)
        assert res["code"] == 0, res
        assert res["data"]["llm_id"] == llm_id, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "permission",
        [
            "me",
            "team"
        ],
        ids=["me", "team"]
    )
    def test_permission(self, WebApiAuth, add_memory_func, permission):
        memory_ids = add_memory_func
        payload = {"permissions": permission}
        res = update_memory(WebApiAuth, memory_ids[0], payload)
        assert res["code"] == 0, res
        assert res["data"]["permissions"] == permission.lower().strip(), res


    @pytest.mark.p1
    def test_memory_size(self, WebApiAuth, add_memory_func):
        memory_ids = add_memory_func
        memory_size = 1048576  # 1 MB
        payload = {"memory_size": memory_size}
        res = update_memory(WebApiAuth, memory_ids[0], payload)
        assert res["code"] == 0, res
        assert res["data"]["memory_size"] == memory_size, res

    @pytest.mark.p1
    def test_temperature(self, WebApiAuth, add_memory_func):
        memory_ids = add_memory_func
        temperature = 0.7
        payload = {"temperature": temperature}
        res = update_memory(WebApiAuth, memory_ids[0], payload)
        assert res["code"] == 0, res
        assert res["data"]["temperature"] == temperature, res

    @pytest.mark.p1
    def test_system_prompt(self, WebApiAuth, add_memory_func):
        memory_ids = add_memory_func
        system_prompt = "This is a system prompt."
        payload = {"system_prompt": system_prompt}
        res = update_memory(WebApiAuth, memory_ids[0], payload)
        assert res["code"] == 0, res
        assert res["data"]["system_prompt"] == system_prompt, res

    @pytest.mark.p1
    def test_user_prompt(self, WebApiAuth, add_memory_func):
        memory_ids = add_memory_func
        user_prompt = "This is a user prompt."
        payload = {"user_prompt": user_prompt}
        res = update_memory(WebApiAuth, memory_ids[0], payload)
        assert res["code"] == 0, res
        assert res["data"]["user_prompt"] == user_prompt, res

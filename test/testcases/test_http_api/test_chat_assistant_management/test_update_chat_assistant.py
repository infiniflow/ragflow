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
from common import create_chat_assistant, get_chat_assistant, patch_chat_assistant, update_chat_assistant
from configs import CHAT_ASSISTANT_NAME_LIMIT, INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth
from utils import encode_avatar
from utils.file_utils import create_image_file


@pytest.mark.p1
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowHttpApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = update_chat_assistant(invalid_auth, "chat_assistant_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestChatAssistantUpdate:
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            pytest.param({"name": "valid_name"}, 0, "", marks=pytest.mark.p1),
            pytest.param({"name": "a" * (CHAT_ASSISTANT_NAME_LIMIT + 1)}, 102, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param({"name": 1}, 100, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param({"name": ""}, 102, "`name` cannot be empty.", marks=pytest.mark.p3),
            pytest.param({"name": "test_chat_assistant_1"}, 102, "Duplicated chat name.", marks=pytest.mark.p3),
            pytest.param({"name": "TEST_CHAT_ASSISTANT_1"}, 102, "Duplicated chat name.", marks=pytest.mark.p3),
        ],
    )
    def test_name(self, HttpApiAuth, add_chat_assistants_func, payload, expected_code, expected_message):
        _, _, chat_assistant_ids = add_chat_assistants_func

        res = patch_chat_assistant(HttpApiAuth, chat_assistant_ids[0], payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            res = get_chat_assistant(HttpApiAuth, chat_assistant_ids[0])
            assert res["data"]["name"] == payload.get("name")
        else:
            assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "dataset_ids, expected_code, expected_message",
        [
            pytest.param([], 0, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param(lambda r: [r], 0, "", marks=pytest.mark.p1),
            pytest.param(["invalid_dataset_id"], 102, "You don't own the dataset invalid_dataset_id", marks=pytest.mark.p3),
            pytest.param("invalid_dataset_id", 102, "`dataset_ids` should be a list.", marks=pytest.mark.p3),
        ],
    )
    def test_dataset_ids(self, HttpApiAuth, add_chat_assistants_func, dataset_ids, expected_code, expected_message):
        dataset_id, _, chat_assistant_ids = add_chat_assistants_func
        payload = {"name": "ragflow test"}
        if callable(dataset_ids):
            payload["dataset_ids"] = dataset_ids(dataset_id)
        else:
            payload["dataset_ids"] = dataset_ids

        res = update_chat_assistant(HttpApiAuth, chat_assistant_ids[0], payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            res = get_chat_assistant(HttpApiAuth, chat_assistant_ids[0])
            assert res["data"]["name"] == payload.get("name")
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    def test_avatar(self, HttpApiAuth, add_chat_assistants_func, tmp_path):
        dataset_id, _, chat_assistant_ids = add_chat_assistants_func
        fn = create_image_file(tmp_path / "ragflow_test.png")
        payload = {"name": "avatar_test", "icon": encode_avatar(fn), "dataset_ids": [dataset_id]}
        res = update_chat_assistant(HttpApiAuth, chat_assistant_ids[0], payload)
        assert res["code"] == 0

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "llm, expected_code, expected_message",
        [
            ({}, 0, ""),
            ({"llm_id": "glm-4"}, 0, ""),
            ({"llm_id": "unknown"}, 102, "`llm_id` unknown doesn't exist"),
            ({"temperature": 0}, 0, ""),
            ({"temperature": 1}, 0, ""),
            pytest.param({"temperature": -1}, 0, "", marks=pytest.mark.skip),
            pytest.param({"temperature": 10}, 0, "", marks=pytest.mark.skip),
            pytest.param({"temperature": "a"}, 0, "", marks=pytest.mark.skip),
            ({"top_p": 0}, 0, ""),
            ({"top_p": 1}, 0, ""),
            pytest.param({"top_p": -1}, 0, "", marks=pytest.mark.skip),
            pytest.param({"top_p": 10}, 0, "", marks=pytest.mark.skip),
            pytest.param({"top_p": "a"}, 0, "", marks=pytest.mark.skip),
            ({"presence_penalty": 0}, 0, ""),
            ({"presence_penalty": 1}, 0, ""),
            pytest.param({"presence_penalty": -1}, 0, "", marks=pytest.mark.skip),
            pytest.param({"presence_penalty": 10}, 0, "", marks=pytest.mark.skip),
            pytest.param({"presence_penalty": "a"}, 0, "", marks=pytest.mark.skip),
            ({"frequency_penalty": 0}, 0, ""),
            ({"frequency_penalty": 1}, 0, ""),
            pytest.param({"frequency_penalty": -1}, 0, "", marks=pytest.mark.skip),
            pytest.param({"frequency_penalty": 10}, 0, "", marks=pytest.mark.skip),
            pytest.param({"frequency_penalty": "a"}, 0, "", marks=pytest.mark.skip),
            ({"max_token": 0}, 0, ""),
            ({"max_token": 1024}, 0, ""),
            pytest.param({"max_token": -1}, 0, "", marks=pytest.mark.skip),
            pytest.param({"max_token": 10}, 0, "", marks=pytest.mark.skip),
            pytest.param({"max_token": "a"}, 0, "", marks=pytest.mark.skip),
            pytest.param({"unknown": "unknown"}, 0, "", marks=pytest.mark.skip),
        ],
    )
    def test_llm(self, HttpApiAuth, add_chat_assistants_func, chat_assistant_llm_model_type, llm, expected_code, expected_message):
        dataset_id, _, chat_assistant_ids = add_chat_assistants_func
        llm_setting = {k: v for k, v in llm.items() if k != "llm_id"}
        llm_setting.setdefault("model_type", chat_assistant_llm_model_type)

        payload = {"name": "llm_test", "dataset_ids": [dataset_id]}
        if "llm_id" in llm:
            payload["llm_id"] = llm["llm_id"]
        payload["llm_setting"] = llm_setting

        res = update_chat_assistant(HttpApiAuth, chat_assistant_ids[0], payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            res = get_chat_assistant(HttpApiAuth, chat_assistant_ids[0])
            for k, v in llm.items():
                if k == "llm_id":
                    assert res["data"]["llm_id"] == v
                else:
                    assert res["data"]["llm_setting"][k] == v
        else:
            assert expected_message in res["message"]

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "prompt, expected_code, expected_message",
        [
            ({}, 0, ""),
            ({"similarity_threshold": 0}, 0, ""),
            ({"similarity_threshold": 1}, 0, ""),
            pytest.param({"similarity_threshold": -1}, 0, "", marks=pytest.mark.skip),
            pytest.param({"similarity_threshold": 10}, 0, "", marks=pytest.mark.skip),
            pytest.param({"similarity_threshold": "a"}, 0, "", marks=pytest.mark.skip),
            ({"vector_similarity_weight": 0}, 0, ""),
            ({"vector_similarity_weight": 1}, 0, ""),
            pytest.param({"vector_similarity_weight": -1}, 0, "", marks=pytest.mark.skip),
            pytest.param({"vector_similarity_weight": 10}, 0, "", marks=pytest.mark.skip),
            pytest.param({"vector_similarity_weight": "a"}, 0, "", marks=pytest.mark.skip),
            ({"parameters": []}, 0, ""),
            ({"top_n": 0}, 0, ""),
            ({"top_n": 1}, 0, ""),
            pytest.param({"top_n": -1}, 0, "", marks=pytest.mark.skip),
            pytest.param({"top_n": 10}, 0, "", marks=pytest.mark.skip),
            pytest.param({"top_n": "a"}, 0, "", marks=pytest.mark.skip),
            ({"empty_response": "Hello World"}, 0, ""),
            ({"empty_response": ""}, 0, ""),
            ({"empty_response": "!@#$%^&*()"}, 0, ""),
            ({"empty_response": "中文测试"}, 0, ""),
            pytest.param({"empty_response": 123}, 0, "", marks=pytest.mark.skip),
            pytest.param({"empty_response": True}, 0, "", marks=pytest.mark.skip),
            pytest.param({"empty_response": " "}, 0, "", marks=pytest.mark.skip),
            ({"prologue": "Hello World"}, 0, ""),
            ({"prologue": ""}, 0, ""),
            ({"prologue": "!@#$%^&*()"}, 0, ""),
            ({"prologue": "中文测试"}, 0, ""),
            pytest.param({"prologue": 123}, 0, "", marks=pytest.mark.skip),
            pytest.param({"prologue": True}, 0, "", marks=pytest.mark.skip),
            pytest.param({"prologue": " "}, 0, "", marks=pytest.mark.skip),
            ({"quote": True}, 0, ""),
            ({"quote": False}, 0, ""),
            ({"system": "Hello World {knowledge}"}, 0, ""),
            ({"system": "{knowledge}"}, 0, ""),
            ({"system": "!@#$%^&*() {knowledge}"}, 0, ""),
            ({"system": "中文测试 {knowledge}"}, 0, ""),
            ({"system": "Hello World"}, 0, ""),
            ({"system": "Hello World", "parameters": []}, 0, ""),
            pytest.param({"system": 123}, 100, """AttributeError("\'int\' object has no attribute \'find\'")""", marks=pytest.mark.skip),
            pytest.param({"system": True}, 100, """AttributeError("\'int\' object has no attribute \'find\'")""", marks=pytest.mark.skip),
            pytest.param({"unknown": "unknown"}, 0, "", marks=pytest.mark.skip),
        ],
    )
    def test_prompt(self, HttpApiAuth, add_chat_assistants_func, prompt, expected_code, expected_message):
        dataset_id, _, chat_assistant_ids = add_chat_assistants_func

        _PROMPT_CONFIG_KEYS = {"prologue", "quote", "system", "parameters", "empty_response"}

        payload = {"name": "prompt_test", "dataset_ids": [dataset_id]}
        prompt_config = {}
        for k, v in prompt.items():
            if k in _PROMPT_CONFIG_KEYS:
                prompt_config[k] = v
            else:
                payload[k] = v
        if prompt_config:
            payload["prompt_config"] = prompt_config

        res = update_chat_assistant(HttpApiAuth, chat_assistant_ids[0], payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            if not prompt:
                return
            res = get_chat_assistant(HttpApiAuth, chat_assistant_ids[0])
            for k, v in prompt.items():
                if k in _PROMPT_CONFIG_KEYS:
                    assert res["data"]["prompt_config"][k] == v
                else:
                    assert res["data"][k] == v
        else:
            assert expected_message in res["message"]

    @pytest.mark.p2
    def test_update_mapping_and_validation_branches_p2(self, HttpApiAuth, add_chat_assistants_func, chat_assistant_llm_model_type):
        dataset_id, _, chat_assistant_ids = add_chat_assistants_func
        chat_id = chat_assistant_ids[0]

        # Auth: non-owned chat returns 109 "No authorization."
        res = patch_chat_assistant(HttpApiAuth, "invalid-chat-id", {"name": "anything"})
        assert res["code"] == 109
        assert res["message"] == "No authorization."

        # PATCH: toggle quote via prompt_config
        res = patch_chat_assistant(HttpApiAuth, chat_id, {"prompt_config": {"quote": False}})
        assert res["code"] == 0

        # PATCH: invalid llm_id
        res = patch_chat_assistant(
            HttpApiAuth,
            chat_id,
            {"llm_id": "unknown-llm-model", "llm_setting": {"model_type": chat_assistant_llm_model_type}},
        )
        assert res["code"] == 102
        assert "`llm_id` unknown-llm-model doesn't exist" in res["message"]

        # PATCH: invalid rerank_id
        res = patch_chat_assistant(HttpApiAuth, chat_id, {"rerank_id": "unknown-rerank-model"})
        assert res["code"] == 102
        assert "`rerank_id` unknown-rerank-model doesn't exist" in res["message"]

        # PATCH: empty name
        res = patch_chat_assistant(HttpApiAuth, chat_id, {"name": ""})
        assert res["code"] == 102
        assert res["message"] == "`name` cannot be empty."

        # PATCH: duplicate name
        res = patch_chat_assistant(HttpApiAuth, chat_id, {"name": "test_chat_assistant_1"})
        assert res["code"] == 102
        assert res["message"] == "Duplicated chat name."

        # PATCH: prompt_config without placeholder is allowed
        res = patch_chat_assistant(
            HttpApiAuth,
            chat_id,
            {"prompt_config": {"system": "No required placeholder", "parameters": [{"key": "knowledge", "optional": False}]}},
        )
        assert res["code"] == 0

        # PATCH: icon (was "avatar" in old SDK)
        res = patch_chat_assistant(HttpApiAuth, chat_id, {"icon": "raw-avatar-value"})
        assert res["code"] == 0
        listed = get_chat_assistant(HttpApiAuth, chat_id)
        assert listed["code"] == 0
        assert listed["data"]["icon"] == "raw-avatar-value"

    @pytest.mark.p2
    def test_update_unparsed_dataset_guard_p2(self, HttpApiAuth, add_dataset_func, clear_chat_assistants):
        dataset_id = add_dataset_func
        create_res = create_chat_assistant(HttpApiAuth, {"name": "update-unparsed-target", "dataset_ids": []})
        assert create_res["code"] == 0

        chat_id = create_res["data"]["id"]
        res = patch_chat_assistant(HttpApiAuth, chat_id, {"dataset_ids": [dataset_id]})
        assert res["code"] == 102
        assert "doesn't own parsed file" in res["message"]

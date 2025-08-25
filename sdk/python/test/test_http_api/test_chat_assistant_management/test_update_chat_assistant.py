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
from common import CHAT_ASSISTANT_NAME_LIMIT, INVALID_API_TOKEN, list_chat_assistants, update_chat_assistant
from libs.auth import RAGFlowHttpApiAuth
from libs.utils import encode_avatar
from libs.utils.file_utils import create_image_file


@pytest.mark.p1
class TestAuthorization:
    @pytest.mark.parametrize(
        "auth, expected_code, expected_message",
        [
            (None, 0, "`Authorization` can't be empty"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                109,
                "Authentication error: API key is invalid!",
            ),
        ],
    )
    def test_invalid_auth(self, auth, expected_code, expected_message):
        res = update_chat_assistant(auth, "chat_assistant_id")
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
            pytest.param({"name": "test_chat_assistant_1"}, 102, "Duplicated chat name in updating chat.", marks=pytest.mark.p3),
            pytest.param({"name": "TEST_CHAT_ASSISTANT_1"}, 102, "Duplicated chat name in updating chat.", marks=pytest.mark.p3),
        ],
    )
    def test_name(self, get_http_api_auth, add_chat_assistants_func, payload, expected_code, expected_message):
        _, _, chat_assistant_ids = add_chat_assistants_func

        res = update_chat_assistant(get_http_api_auth, chat_assistant_ids[0], payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            res = list_chat_assistants(get_http_api_auth, {"id": chat_assistant_ids[0]})
            assert res["data"][0]["name"] == payload.get("name")
        else:
            assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "dataset_ids, expected_code, expected_message",
        [
            pytest.param([], 0, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param(lambda r: [r], 0, "", marks=pytest.mark.p1),
            pytest.param(["invalid_dataset_id"], 102, "You don't own the dataset invalid_dataset_id", marks=pytest.mark.p3),
            pytest.param("invalid_dataset_id", 102, "You don't own the dataset i", marks=pytest.mark.p3),
        ],
    )
    def test_dataset_ids(self, get_http_api_auth, add_chat_assistants_func, dataset_ids, expected_code, expected_message):
        dataset_id, _, chat_assistant_ids = add_chat_assistants_func
        payload = {"name": "ragflow test"}
        if callable(dataset_ids):
            payload["dataset_ids"] = dataset_ids(dataset_id)
        else:
            payload["dataset_ids"] = dataset_ids

        res = update_chat_assistant(get_http_api_auth, chat_assistant_ids[0], payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            res = list_chat_assistants(get_http_api_auth, {"id": chat_assistant_ids[0]})
            assert res["data"][0]["name"] == payload.get("name")
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    def test_avatar(self, get_http_api_auth, add_chat_assistants_func, tmp_path):
        dataset_id, _, chat_assistant_ids = add_chat_assistants_func
        fn = create_image_file(tmp_path / "ragflow_test.png")
        payload = {"name": "avatar_test", "avatar": encode_avatar(fn), "dataset_ids": [dataset_id]}
        res = update_chat_assistant(get_http_api_auth, chat_assistant_ids[0], payload)
        assert res["code"] == 0

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "llm, expected_code, expected_message",
        [
            ({}, 100, "ValueError"),
            ({"model_name": "glm-4"}, 0, ""),
            ({"model_name": "unknown"}, 102, "`model_name` unknown doesn't exist"),
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
    def test_llm(self, get_http_api_auth, add_chat_assistants_func, llm, expected_code, expected_message):
        dataset_id, _, chat_assistant_ids = add_chat_assistants_func
        payload = {"name": "llm_test", "dataset_ids": [dataset_id], "llm": llm}
        res = update_chat_assistant(get_http_api_auth, chat_assistant_ids[0], payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            res = list_chat_assistants(get_http_api_auth, {"id": chat_assistant_ids[0]})
            if llm:
                for k, v in llm.items():
                    assert res["data"][0]["llm"][k] == v
            else:
                assert res["data"][0]["llm"]["model_name"] == "glm-4-flash@ZHIPU-AI"
                assert res["data"][0]["llm"]["temperature"] == 0.1
                assert res["data"][0]["llm"]["top_p"] == 0.3
                assert res["data"][0]["llm"]["presence_penalty"] == 0.4
                assert res["data"][0]["llm"]["frequency_penalty"] == 0.7
                assert res["data"][0]["llm"]["max_tokens"] == 512
        else:
            assert expected_message in res["message"]

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "prompt, expected_code, expected_message",
        [
            ({}, 100, "ValueError"),
            ({"similarity_threshold": 0}, 0, ""),
            ({"similarity_threshold": 1}, 0, ""),
            pytest.param({"similarity_threshold": -1}, 0, "", marks=pytest.mark.skip),
            pytest.param({"similarity_threshold": 10}, 0, "", marks=pytest.mark.skip),
            pytest.param({"similarity_threshold": "a"}, 0, "", marks=pytest.mark.skip),
            ({"keywords_similarity_weight": 0}, 0, ""),
            ({"keywords_similarity_weight": 1}, 0, ""),
            pytest.param({"keywords_similarity_weight": -1}, 0, "", marks=pytest.mark.skip),
            pytest.param({"keywords_similarity_weight": 10}, 0, "", marks=pytest.mark.skip),
            pytest.param({"keywords_similarity_weight": "a"}, 0, "", marks=pytest.mark.skip),
            ({"variables": []}, 0, ""),
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
            ({"opener": "Hello World"}, 0, ""),
            ({"opener": ""}, 0, ""),
            ({"opener": "!@#$%^&*()"}, 0, ""),
            ({"opener": "中文测试"}, 0, ""),
            pytest.param({"opener": 123}, 0, "", marks=pytest.mark.skip),
            pytest.param({"opener": True}, 0, "", marks=pytest.mark.skip),
            pytest.param({"opener": " "}, 0, "", marks=pytest.mark.skip),
            ({"show_quote": True}, 0, ""),
            ({"show_quote": False}, 0, ""),
            ({"prompt": "Hello World {knowledge}"}, 0, ""),
            ({"prompt": "{knowledge}"}, 0, ""),
            ({"prompt": "!@#$%^&*() {knowledge}"}, 0, ""),
            ({"prompt": "中文测试 {knowledge}"}, 0, ""),
            ({"prompt": "Hello World"}, 102, "Parameter 'knowledge' is not used"),
            ({"prompt": "Hello World", "variables": []}, 0, ""),
            pytest.param({"prompt": 123}, 100, """AttributeError("\'int\' object has no attribute \'find\'")""", marks=pytest.mark.skip),
            pytest.param({"prompt": True}, 100, """AttributeError("\'int\' object has no attribute \'find\'")""", marks=pytest.mark.skip),
            pytest.param({"unknown": "unknown"}, 0, "", marks=pytest.mark.skip),
        ],
    )
    def test_prompt(self, get_http_api_auth, add_chat_assistants_func, prompt, expected_code, expected_message):
        dataset_id, _, chat_assistant_ids = add_chat_assistants_func
        payload = {"name": "prompt_test", "dataset_ids": [dataset_id], "prompt": prompt}
        res = update_chat_assistant(get_http_api_auth, chat_assistant_ids[0], payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            res = list_chat_assistants(get_http_api_auth, {"id": chat_assistant_ids[0]})
            if prompt:
                for k, v in prompt.items():
                    if k == "keywords_similarity_weight":
                        assert res["data"][0]["prompt"][k] == 1 - v
                    else:
                        assert res["data"][0]["prompt"][k] == v
            else:
                assert res["data"]["prompt"][0]["similarity_threshold"] == 0.2
                assert res["data"]["prompt"][0]["keywords_similarity_weight"] == 0.7
                assert res["data"]["prompt"][0]["top_n"] == 6
                assert res["data"]["prompt"][0]["variables"] == [{"key": "knowledge", "optional": False}]
                assert res["data"]["prompt"][0]["rerank_model"] == ""
                assert res["data"]["prompt"][0]["empty_response"] == "Sorry! No relevant content was found in the knowledge base!"
                assert res["data"]["prompt"][0]["opener"] == "Hi! I'm your assistant. What can I do for you?"
                assert res["data"]["prompt"][0]["show_quote"] is True
                assert (
                    res["data"]["prompt"][0]["prompt"]
                    == 'You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the knowledge base!" Answers need to consider chat history.\n      Here is the knowledge base:\n      {knowledge}\n      The above is the knowledge base.'
                )
        else:
            assert expected_message in res["message"]

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
from common import create_chat_assistant
from configs import CHAT_ASSISTANT_NAME_LIMIT, INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth
from utils import encode_avatar
from utils.file_utils import create_image_file


@pytest.mark.p1
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 0, "`Authorization` can't be empty"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                109,
                "Authentication error: API key is invalid!",
            ),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = create_chat_assistant(invalid_auth)
        assert res["code"] == expected_code
        assert res["message"] == expected_message


@pytest.mark.usefixtures("clear_chat_assistants")
class TestChatAssistantCreate:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"name": "valid_name"}, 0, ""),
            pytest.param({"name": "a" * (CHAT_ASSISTANT_NAME_LIMIT + 1)}, 102, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param({"name": 1}, 100, "", marks=pytest.mark.skip(reason="issues/")),
            ({"name": ""}, 102, "`name` is required."),
            ({"name": "duplicated_name"}, 102, "Duplicated chat name in creating chat."),
            ({"name": "case insensitive"}, 102, "Duplicated chat name in creating chat."),
        ],
    )
    def test_name(self, HttpApiAuth, add_chunks, payload, expected_code, expected_message):
        payload["dataset_ids"] = []  # issues/
        if payload["name"] == "duplicated_name":
            create_chat_assistant(HttpApiAuth, payload)
        elif payload["name"] == "case insensitive":
            create_chat_assistant(HttpApiAuth, {"name": payload["name"].upper()})

        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert res["data"]["name"] == payload["name"]
        else:
            assert res["message"] == expected_message

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "dataset_ids, expected_code, expected_message",
        [
            ([], 0, ""),
            (lambda r: [r], 0, ""),
            (["invalid_dataset_id"], 102, "You don't own the dataset invalid_dataset_id"),
            ("invalid_dataset_id", 102, "You don't own the dataset i"),
        ],
    )
    def test_dataset_ids(self, HttpApiAuth, add_chunks, dataset_ids, expected_code, expected_message):
        dataset_id, _, _ = add_chunks
        payload = {"name": "ragflow test"}
        if callable(dataset_ids):
            payload["dataset_ids"] = dataset_ids(dataset_id)
        else:
            payload["dataset_ids"] = dataset_ids

        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert res["data"]["name"] == payload["name"]
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    def test_avatar(self, HttpApiAuth, tmp_path):
        fn = create_image_file(tmp_path / "ragflow_test.png")
        payload = {"name": "avatar_test", "avatar": encode_avatar(fn), "dataset_ids": []}
        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] == 0

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "llm, expected_code, expected_message",
        [
            ({}, 0, ""),
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
    def test_llm(self, HttpApiAuth, add_chunks, llm, expected_code, expected_message):
        dataset_id, _, _ = add_chunks
        payload = {"name": "llm_test", "dataset_ids": [dataset_id], "llm": llm}
        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            if llm:
                for k, v in llm.items():
                    assert res["data"]["llm"][k] == v
            else:
                assert res["data"]["llm"]["model_name"] == "glm-4-flash@ZHIPU-AI"
                assert res["data"]["llm"]["temperature"] == 0.1
                assert res["data"]["llm"]["top_p"] == 0.3
                assert res["data"]["llm"]["presence_penalty"] == 0.4
                assert res["data"]["llm"]["frequency_penalty"] == 0.7
                assert res["data"]["llm"]["max_tokens"] == 512
        else:
            assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "prompt, expected_code, expected_message",
        [
            ({}, 0, ""),
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
    def test_prompt(self, HttpApiAuth, add_chunks, prompt, expected_code, expected_message):
        dataset_id, _, _ = add_chunks
        payload = {"name": "prompt_test", "dataset_ids": [dataset_id], "prompt": prompt}
        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            if prompt:
                for k, v in prompt.items():
                    if k == "keywords_similarity_weight":
                        assert res["data"]["prompt"][k] == 1 - v
                    else:
                        assert res["data"]["prompt"][k] == v
            else:
                assert res["data"]["prompt"]["similarity_threshold"] == 0.2
                assert res["data"]["prompt"]["keywords_similarity_weight"] == 0.7
                assert res["data"]["prompt"]["top_n"] == 6
                assert res["data"]["prompt"]["variables"] == [{"key": "knowledge", "optional": False}]
                assert res["data"]["prompt"]["rerank_model"] == ""
                assert res["data"]["prompt"]["empty_response"] == "Sorry! No relevant content was found in the knowledge base!"
                assert res["data"]["prompt"]["opener"] == "Hi! I'm your assistant. What can I do for you?"
                assert res["data"]["prompt"]["show_quote"] is True
                assert (
                    res["data"]["prompt"]["prompt"]
                    == 'You are an intelligent assistant. Please summarize the content of the dataset to answer the question. Please list the data in the dataset and answer in detail. When all dataset content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the dataset!" Answers need to consider chat history.\n      Here is the knowledge base:\n      {knowledge}\n      The above is the knowledge base.'
                )
        else:
            assert res["message"] == expected_message


class TestChatAssistantCreate2:
    @pytest.mark.p2
    def test_unparsed_document(self, HttpApiAuth, add_document):
        dataset_id, _ = add_document
        payload = {"name": "prompt_test", "dataset_ids": [dataset_id]}
        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] == 102
        assert "doesn't own parsed file" in res["message"]

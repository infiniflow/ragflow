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
from operator import attrgetter

import pytest
from configs import CHAT_ASSISTANT_NAME_LIMIT
from ragflow_sdk import Chat
from utils import encode_avatar
from utils.file_utils import create_image_file


class TestChatAssistantUpdate:
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            pytest.param({"name": "valid_name"}, "", marks=pytest.mark.p1),
            pytest.param({"name": "a" * (CHAT_ASSISTANT_NAME_LIMIT + 1)}, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param({"name": 1}, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param({"name": ""}, "`name` cannot be empty.", marks=pytest.mark.p3),
            pytest.param({"name": "test_chat_assistant_1"}, "Duplicated chat name in updating chat.", marks=pytest.mark.p3),
            pytest.param({"name": "TEST_CHAT_ASSISTANT_1"}, "Duplicated chat name in updating chat.", marks=pytest.mark.p3),
        ],
    )
    def test_name(self, client, add_chat_assistants_func, payload, expected_message):
        _, _, chat_assistants = add_chat_assistants_func
        chat_assistant = chat_assistants[0]

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                chat_assistant.update(payload)
            assert expected_message in str(excinfo.value)
        else:
            chat_assistant.update(payload)
            updated_chat = client.list_chats(id=chat_assistant.id)[0]
            assert updated_chat.name == payload["name"], str(updated_chat)

    @pytest.mark.p3
    def test_avatar(self, client, add_chat_assistants_func, tmp_path):
        dataset, _, chat_assistants = add_chat_assistants_func
        chat_assistant = chat_assistants[0]

        fn = create_image_file(tmp_path / "ragflow_test.png")
        payload = {"name": "avatar_test", "avatar": encode_avatar(fn), "dataset_ids": [dataset.id]}

        chat_assistant.update(payload)
        updated_chat = client.list_chats(id=chat_assistant.id)[0]
        assert updated_chat.name == payload["name"], str(updated_chat)
        assert updated_chat.avatar is not None, str(updated_chat)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "llm, expected_message",
        [
            ({}, "ValueError"),
            ({"model_name": "glm-4"}, ""),
            ({"model_name": "unknown"}, "`model_name` unknown doesn't exist"),
            ({"temperature": 0}, ""),
            ({"temperature": 1}, ""),
            pytest.param({"temperature": -1}, "", marks=pytest.mark.skip),
            pytest.param({"temperature": 10}, "", marks=pytest.mark.skip),
            pytest.param({"temperature": "a"}, "", marks=pytest.mark.skip),
            ({"top_p": 0}, ""),
            ({"top_p": 1}, ""),
            pytest.param({"top_p": -1}, "", marks=pytest.mark.skip),
            pytest.param({"top_p": 10}, "", marks=pytest.mark.skip),
            pytest.param({"top_p": "a"}, "", marks=pytest.mark.skip),
            ({"presence_penalty": 0}, ""),
            ({"presence_penalty": 1}, ""),
            pytest.param({"presence_penalty": -1}, "", marks=pytest.mark.skip),
            pytest.param({"presence_penalty": 10}, "", marks=pytest.mark.skip),
            pytest.param({"presence_penalty": "a"}, "", marks=pytest.mark.skip),
            ({"frequency_penalty": 0}, ""),
            ({"frequency_penalty": 1}, ""),
            pytest.param({"frequency_penalty": -1}, "", marks=pytest.mark.skip),
            pytest.param({"frequency_penalty": 10}, "", marks=pytest.mark.skip),
            pytest.param({"frequency_penalty": "a"}, "", marks=pytest.mark.skip),
            ({"max_token": 0}, ""),
            ({"max_token": 1024}, ""),
            pytest.param({"max_token": -1}, "", marks=pytest.mark.skip),
            pytest.param({"max_token": 10}, "", marks=pytest.mark.skip),
            pytest.param({"max_token": "a"}, "", marks=pytest.mark.skip),
            pytest.param({"unknown": "unknown"}, "", marks=pytest.mark.skip),
        ],
    )
    def test_llm(self, client, add_chat_assistants_func, llm, expected_message):
        dataset, _, chat_assistants = add_chat_assistants_func
        chat_assistant = chat_assistants[0]
        payload = {"name": "llm_test", "llm": llm, "dataset_ids": [dataset.id]}

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                chat_assistant.update(payload)
            assert expected_message in str(excinfo.value)
        else:
            chat_assistant.update(payload)
            updated_chat = client.list_chats(id=chat_assistant.id)[0]
            if llm:
                for k, v in llm.items():
                    assert attrgetter(k)(updated_chat.llm) == v, str(updated_chat)
            else:
                excepted_value = Chat.LLM(
                    client,
                    {
                        "model_name": "glm-4-flash@ZHIPU-AI",
                        "temperature": 0.1,
                        "top_p": 0.3,
                        "presence_penalty": 0.4,
                        "frequency_penalty": 0.7,
                        "max_tokens": 512,
                    },
                )
                assert str(updated_chat.llm) == str(excepted_value), str(updated_chat)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "prompt, expected_message",
        [
            ({}, "ValueError"),
            ({"similarity_threshold": 0}, ""),
            ({"similarity_threshold": 1}, ""),
            pytest.param({"similarity_threshold": -1}, "", marks=pytest.mark.skip),
            pytest.param({"similarity_threshold": 10}, "", marks=pytest.mark.skip),
            pytest.param({"similarity_threshold": "a"}, "", marks=pytest.mark.skip),
            ({"keywords_similarity_weight": 0}, ""),
            ({"keywords_similarity_weight": 1}, ""),
            pytest.param({"keywords_similarity_weight": -1}, "", marks=pytest.mark.skip),
            pytest.param({"keywords_similarity_weight": 10}, "", marks=pytest.mark.skip),
            pytest.param({"keywords_similarity_weight": "a"}, "", marks=pytest.mark.skip),
            ({"variables": []}, ""),
            ({"top_n": 0}, ""),
            ({"top_n": 1}, ""),
            pytest.param({"top_n": -1}, "", marks=pytest.mark.skip),
            pytest.param({"top_n": 10}, "", marks=pytest.mark.skip),
            pytest.param({"top_n": "a"}, "", marks=pytest.mark.skip),
            ({"empty_response": "Hello World"}, ""),
            ({"empty_response": ""}, ""),
            ({"empty_response": "!@#$%^&*()"}, ""),
            ({"empty_response": "中文测试"}, ""),
            pytest.param({"empty_response": 123}, "", marks=pytest.mark.skip),
            pytest.param({"empty_response": True}, "", marks=pytest.mark.skip),
            pytest.param({"empty_response": " "}, "", marks=pytest.mark.skip),
            ({"opener": "Hello World"}, ""),
            ({"opener": ""}, ""),
            ({"opener": "!@#$%^&*()"}, ""),
            ({"opener": "中文测试"}, ""),
            pytest.param({"opener": 123}, "", marks=pytest.mark.skip),
            pytest.param({"opener": True}, "", marks=pytest.mark.skip),
            pytest.param({"opener": " "}, "", marks=pytest.mark.skip),
            ({"show_quote": True}, ""),
            ({"show_quote": False}, ""),
            ({"prompt": "Hello World {knowledge}"}, ""),
            ({"prompt": "{knowledge}"}, ""),
            ({"prompt": "!@#$%^&*() {knowledge}"}, ""),
            ({"prompt": "中文测试 {knowledge}"}, ""),
            ({"prompt": "Hello World"}, ""),
            ({"prompt": "Hello World", "variables": []}, ""),
            pytest.param({"prompt": 123}, """AttributeError("\'int\' object has no attribute \'find\'")""", marks=pytest.mark.skip),
            pytest.param({"prompt": True}, """AttributeError("\'int\' object has no attribute \'find\'")""", marks=pytest.mark.skip),
            pytest.param({"unknown": "unknown"}, "", marks=pytest.mark.skip),
        ],
    )
    def test_prompt(self, client, add_chat_assistants_func, prompt, expected_message):
        dataset, _, chat_assistants = add_chat_assistants_func
        chat_assistant = chat_assistants[0]
        payload = {"name": "prompt_test", "prompt": prompt, "dataset_ids": [dataset.id]}

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                chat_assistant.update(payload)
            assert expected_message in str(excinfo.value)
        else:
            chat_assistant.update(payload)
            updated_chat = client.list_chats(id=chat_assistant.id)[0]
            if prompt:
                for k, v in prompt.items():
                    if k == "keywords_similarity_weight":
                        assert attrgetter(k)(updated_chat.prompt) == 1 - v, str(updated_chat)
                    else:
                        assert attrgetter(k)(updated_chat.prompt) == v, str(updated_chat)
            else:
                excepted_value = Chat.LLM(
                    client,
                    {
                        "similarity_threshold": 0.2,
                        "keywords_similarity_weight": 0.7,
                        "top_n": 6,
                        "variables": [{"key": "knowledge", "optional": False}],
                        "rerank_model": "",
                        "empty_response": "Sorry! No relevant content was found in the knowledge base!",
                        "opener": "Hi! I'm your assistant. What can I do for you?",
                        "show_quote": True,
                        "prompt": 'You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the knowledge base!" Answers need to consider chat history.\n      Here is the knowledge base:\n      {knowledge}\n      The above is the knowledge base.',
                    },
                )
                assert str(updated_chat.prompt) == str(excepted_value), str(updated_chat)

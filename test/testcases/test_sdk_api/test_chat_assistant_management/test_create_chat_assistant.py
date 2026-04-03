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


@pytest.mark.usefixtures("clear_chat_assistants")
class TestChatAssistantCreate:
    @pytest.mark.p1
    @pytest.mark.usefixtures("add_chunks")
    @pytest.mark.parametrize(
        "name, expected_message",
        [
            ("valid_name", ""),
            pytest.param("a" * (CHAT_ASSISTANT_NAME_LIMIT + 1), "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param(1, "", marks=pytest.mark.skip(reason="issues/")),
            ("", "`name` is required."),
            ("duplicated_name", "Duplicated chat name in creating chat."),
            ("case insensitive", "Duplicated chat name in creating chat."),
        ],
    )
    def test_name(self, client, name, expected_message):
        if name == "duplicated_name":
            client.create_chat(name=name)
        elif name == "case insensitive":
            client.create_chat(name=name.upper())

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                client.create_chat(name=name)
            assert expected_message in str(exception_info.value)
        else:
            chat_assistant = client.create_chat(name=name)
            assert chat_assistant.name == name

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "dataset_ids, expected_message",
        [
            ([], ""),
            (lambda r: [r], ""),
            (["invalid_dataset_id"], "You don't own the dataset invalid_dataset_id"),
            ("invalid_dataset_id", "You don't own the dataset i"),
        ],
    )
    def test_dataset_ids(self, client, add_chunks, dataset_ids, expected_message):
        dataset, _, _ = add_chunks
        if callable(dataset_ids):
            dataset_ids = dataset_ids(dataset.id)

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                client.create_chat(name="ragflow test", dataset_ids=dataset_ids)
            assert expected_message in str(exception_info.value)
        else:
            chat_assistant = client.create_chat(name="ragflow test", dataset_ids=dataset_ids)
            assert chat_assistant.name == "ragflow test"

    @pytest.mark.p3
    def test_avatar(self, client, tmp_path):
        fn = create_image_file(tmp_path / "ragflow_test.png")
        chat_assistant = client.create_chat(name="avatar_test", avatar=encode_avatar(fn), dataset_ids=[])
        assert chat_assistant.name == "avatar_test"

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "llm, expected_message",
        [
            ({}, ""),
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
    def test_llm(self, client, add_chunks, llm, expected_message):
        dataset, _, _ = add_chunks
        llm_o = Chat.LLM(client, llm)

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                client.create_chat(name="llm_test", dataset_ids=[dataset.id], llm=llm_o)
            assert expected_message in str(exception_info.value)
        else:
            chat_assistant = client.create_chat(name="llm_test", dataset_ids=[dataset.id], llm=llm_o)
            if llm:
                for k, v in llm.items():
                    assert attrgetter(k)(chat_assistant.llm) == v
            else:
                assert attrgetter("model_name")(chat_assistant.llm) == "glm-4-flash@ZHIPU-AI"
                assert attrgetter("temperature")(chat_assistant.llm) == 0.1
                assert attrgetter("top_p")(chat_assistant.llm) == 0.3
                assert attrgetter("presence_penalty")(chat_assistant.llm) == 0.4
                assert attrgetter("frequency_penalty")(chat_assistant.llm) == 0.7
                assert attrgetter("max_tokens")(chat_assistant.llm) == 512

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "prompt, expected_message",
        [
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
    def test_prompt(self, client, add_chunks, prompt, expected_message):
        dataset, _, _ = add_chunks
        prompt_o = Chat.Prompt(client, prompt)

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                client.create_chat(name="prompt_test", dataset_ids=[dataset.id], prompt=prompt_o)
            assert expected_message in str(exception_info.value)
        else:
            chat_assistant = client.create_chat(name="prompt_test", dataset_ids=[dataset.id], prompt=prompt_o)
            if prompt:
                for k, v in prompt.items():
                    if k == "keywords_similarity_weight":
                        assert attrgetter(k)(chat_assistant.prompt) == 1 - v
                    else:
                        assert attrgetter(k)(chat_assistant.prompt) == v
            else:
                assert attrgetter("similarity_threshold")(chat_assistant.prompt) == 0.2
                assert attrgetter("keywords_similarity_weight")(chat_assistant.prompt) == 0.7
                assert attrgetter("top_n")(chat_assistant.prompt) == 6
                assert attrgetter("variables")(chat_assistant.prompt) == [{"key": "knowledge", "optional": False}]
                assert attrgetter("rerank_model")(chat_assistant.prompt) == ""
                assert attrgetter("empty_response")(chat_assistant.prompt) == "Sorry! No relevant content was found in the knowledge base!"
                assert attrgetter("opener")(chat_assistant.prompt) == "Hi! I'm your assistant. What can I do for you?"
                assert attrgetter("show_quote")(chat_assistant.prompt) is True
                assert (
                    attrgetter("prompt")(chat_assistant.prompt)
                    == 'You are an intelligent assistant. Please summarize the content of the dataset to answer the question. Please list the data in the dataset and answer in detail. When all dataset content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the dataset!" Answers need to consider chat history.\n      Here is the knowledge base:\n      {knowledge}\n      The above is the knowledge base.'
                )


class TestChatAssistantCreate2:
    @pytest.mark.p2
    def test_unparsed_document(self, client, add_document):
        dataset, _ = add_document
        with pytest.raises(Exception) as exception_info:
            client.create_chat(name="prompt_test", dataset_ids=[dataset.id])
        assert "doesn't own parsed file" in str(exception_info.value)

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
from configs import CHAT_ASSISTANT_NAME_LIMIT
from utils import encode_avatar
from utils.file_utils import create_image_file


class TestChatAssistantUpdate:
    @pytest.mark.p2
    def test_update_rejects_non_dict(self, add_chat_assistants_func):
        _, _, chat_assistants = add_chat_assistants_func
        chat_assistant = chat_assistants[0]

        with pytest.raises(Exception) as exception_info:
            chat_assistant.update.__wrapped__(chat_assistant, "bad")
        assert "`update_message` must be a dict" in str(exception_info.value)

    @pytest.mark.p2
    def test_update_raises_on_nonzero_response(self, add_chat_assistants_func, monkeypatch):
        _, _, chat_assistants = add_chat_assistants_func
        chat_assistant = chat_assistants[0]

        class _DummyResponse:
            def json(self):
                return {"code": 1, "message": "boom"}

        monkeypatch.setattr(chat_assistant, "patch", lambda *_args, **_kwargs: _DummyResponse())

        with pytest.raises(Exception) as exception_info:
            chat_assistant.update({"name": "error-case"})
        assert "boom" in str(exception_info.value)

    @pytest.mark.p1
    def test_update_uses_patch_for_partial_payload(self, add_chat_assistants_func, monkeypatch):
        _, _, chat_assistants = add_chat_assistants_func
        chat_assistant = chat_assistants[0]
        captured = {}

        class _DummyResponse:
            def json(self):
                return {"code": 0, "message": "ok"}

        def _patch(path, payload):
            captured["path"] = path
            captured["payload"] = payload
            return _DummyResponse()

        monkeypatch.setattr(chat_assistant, "patch", _patch)
        monkeypatch.setattr(chat_assistant, "put", lambda *_args, **_kwargs: pytest.fail("update() should not use PUT"))

        chat_assistant.update({"name": "renamed"})

        assert captured["path"] == f"/chats/{chat_assistant.id}"
        assert captured["payload"] == {"name": "renamed"}

    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            pytest.param({"name": "valid_name"}, "", marks=pytest.mark.p1),
            pytest.param({"name": "a" * (CHAT_ASSISTANT_NAME_LIMIT + 1)}, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param({"name": 1}, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param({"name": ""}, "`name` cannot be empty.", marks=pytest.mark.p3),
            pytest.param({"name": "test_chat_assistant_1"}, "Duplicated chat name.", marks=pytest.mark.p3),
            pytest.param({"name": "TEST_CHAT_ASSISTANT_1"}, "Duplicated chat name.", marks=pytest.mark.p3),
        ],
    )
    def test_name(self, client, add_chat_assistants_func, payload, expected_message):
        _, _, chat_assistants = add_chat_assistants_func
        chat_assistant = chat_assistants[0]

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chat_assistant.update(payload)
            assert expected_message in str(exception_info.value)
        else:
            chat_assistant.update(payload)
            updated_chat = client.get_chat(chat_assistant.id)
            assert updated_chat.name == payload["name"], str(updated_chat)

    @pytest.mark.p3
    def test_icon(self, client, add_chat_assistants_func, tmp_path):
        dataset, _, chat_assistants = add_chat_assistants_func
        chat_assistant = chat_assistants[0]

        fn = create_image_file(tmp_path / "ragflow_test.png")
        payload = {"name": "icon_test", "icon": encode_avatar(fn), "dataset_ids": [dataset.id]}

        chat_assistant.update(payload)
        updated_chat = client.get_chat(chat_assistant.id)
        assert updated_chat.name == payload["name"], str(updated_chat)
        assert updated_chat.icon is not None, str(updated_chat)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "llm_setting, expected_message",
        [
            ({"model_name": "glm-4"}, ""),
            ({"model_name": "unknown"}, "`llm_id` unknown doesn't exist"),
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
    def test_llm_setting(self, client, add_chat_assistants_func, llm_setting, expected_message):
        dataset, _, chat_assistants = add_chat_assistants_func
        chat_assistant = chat_assistants[0]
        llm_id = llm_setting.pop("model_name", None)
        payload = {"name": "llm_test", "dataset_ids": [dataset.id], "llm_setting": llm_setting}
        if llm_id is not None:
            payload["llm_id"] = llm_id

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chat_assistant.update(payload)
            assert expected_message in str(exception_info.value)
        else:
            chat_assistant.update(payload)
            updated_chat = client.get_chat(chat_assistant.id)
            if llm_id:
                assert updated_chat.llm_id == llm_id, str(updated_chat)
            for k, v in llm_setting.items():
                assert getattr(updated_chat.llm_setting, k) == v, str(updated_chat)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "prompt_config, expected_message",
        [
            ({"empty_response": "Hello World"}, ""),
            ({"empty_response": ""}, ""),
            ({"empty_response": "!@#$%^&*()"}, ""),
            ({"empty_response": "中文测试"}, ""),
            pytest.param({"empty_response": 123}, "", marks=pytest.mark.skip),
            pytest.param({"empty_response": True}, "", marks=pytest.mark.skip),
            pytest.param({"empty_response": " "}, "", marks=pytest.mark.skip),
            ({"prologue": "Hello World"}, ""),
            ({"prologue": ""}, ""),
            ({"prologue": "!@#$%^&*()"}, ""),
            ({"prologue": "中文测试"}, ""),
            pytest.param({"prologue": 123}, "", marks=pytest.mark.skip),
            pytest.param({"prologue": True}, "", marks=pytest.mark.skip),
            pytest.param({"prologue": " "}, "", marks=pytest.mark.skip),
            ({"quote": True}, ""),
            ({"quote": False}, ""),
            ({"system": "Hello World {knowledge}"}, ""),
            ({"system": "{knowledge}"}, ""),
            ({"system": "!@#$%^&*() {knowledge}"}, ""),
            ({"system": "中文测试 {knowledge}"}, ""),
            ({"system": "Hello World"}, ""),
            ({"system": "Hello World", "parameters": []}, ""),
            pytest.param({"system": 123}, "", marks=pytest.mark.skip),
            pytest.param({"unknown": "unknown"}, "", marks=pytest.mark.skip),
        ],
    )
    def test_prompt_config(self, client, add_chat_assistants_func, prompt_config, expected_message):
        dataset, _, chat_assistants = add_chat_assistants_func
        chat_assistant = chat_assistants[0]
        payload = {"name": "prompt_test", "prompt_config": prompt_config, "dataset_ids": [dataset.id]}

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chat_assistant.update(payload)
            assert expected_message in str(exception_info.value)
        else:
            chat_assistant.update(payload)
            updated_chat = client.get_chat(chat_assistant.id)
            for k, v in prompt_config.items():
                assert getattr(updated_chat.prompt_config, k) == v, str(updated_chat)

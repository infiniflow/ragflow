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
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from common import batch_create_chat_assistants


class TestChatAssistantsDelete:
    @pytest.mark.parametrize(
        "payload, expected_message, remaining",
        [
            pytest.param(None, "", 0, marks=pytest.mark.p3),
            pytest.param({"ids": []}, "", 0, marks=pytest.mark.p3),
            pytest.param({"ids": ["invalid_id"]}, "Assistant(invalid_id) not found.", 5, marks=pytest.mark.p3),
            pytest.param({"ids": ["\n!?。；！？\"'"]}, """Assistant(\n!?。；！？"\') not found.""", 5, marks=pytest.mark.p3),
            pytest.param(lambda r: {"ids": r[:1]}, "", 4, marks=pytest.mark.p3),
            pytest.param(lambda r: {"ids": r}, "", 0, marks=pytest.mark.p1),
        ],
    )
    def test_basic_scenarios(self, client, add_chat_assistants_func, payload, expected_message, remaining):
        _, _, chat_assistants = add_chat_assistants_func
        if callable(payload):
            payload = payload([chat_assistant.id for chat_assistant in chat_assistants])

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                client.delete_chats(**payload)
            assert expected_message in str(exception_info.value)
        else:
            if payload is None:
                client.delete_chats(payload)
            else:
                client.delete_chats(**payload)

        assistants = client.list_chats()
        assert len(assistants) == remaining

    @pytest.mark.parametrize(
        "payload",
        [
            pytest.param(lambda r: {"ids": ["invalid_id"] + r}, marks=pytest.mark.p3),
            pytest.param(lambda r: {"ids": r[:1] + ["invalid_id"] + r[1:5]}, marks=pytest.mark.p1),
            pytest.param(lambda r: {"ids": r + ["invalid_id"]}, marks=pytest.mark.p3),
        ],
    )
    def test_delete_partial_invalid_id(self, client, add_chat_assistants_func, payload):
        _, _, chat_assistants = add_chat_assistants_func
        payload = payload([chat_assistant.id for chat_assistant in chat_assistants])
        client.delete_chats(**payload)

        assistants = client.list_chats()
        assert len(assistants) == 0

    @pytest.mark.p3
    def test_repeated_deletion(self, client, add_chat_assistants_func):
        _, _, chat_assistants = add_chat_assistants_func
        chat_ids = [chat.id for chat in chat_assistants]
        client.delete_chats(ids=chat_ids)

        with pytest.raises(Exception) as exception_info:
            client.delete_chats(ids=chat_ids)
        assert "not found" in str(exception_info.value)

    @pytest.mark.p3
    def test_duplicate_deletion(self, client, add_chat_assistants_func):
        _, _, chat_assistants = add_chat_assistants_func
        chat_ids = [chat.id for chat in chat_assistants]
        client.delete_chats(ids=chat_ids + chat_ids)

        assistants = client.list_chats()
        assert len(assistants) == 0

    @pytest.mark.p3
    def test_concurrent_deletion(self, client):
        count = 100
        chat_ids = [client.create_chat(name=f"test_{i}").id for i in range(count)]

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(client.delete_chats, ids=[chat_ids[i]]) for i in range(count)]
            responses = list(as_completed(futures))

        assert len(responses) == count
        assert all(future.exception() is None for future in futures)

    @pytest.mark.p3
    def test_delete_1k(self, client):
        chat_assistants = batch_create_chat_assistants(client, 1_000)
        client.delete_chats(ids=[chat_assistants.id for chat_assistants in chat_assistants])

        assistants = client.list_chats()
        assert len(assistants) == 0

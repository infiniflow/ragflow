#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
from uuid import uuid4

import pytest
from ragflow_sdk import RAGFlow


@pytest.mark.p3
def test_delete_chats_nonzero_response_raises(monkeypatch):
    class DummyResponse:
        def json(self):
            return {"code": 1, "message": "boom"}

    client = RAGFlow("token", "http://sdk.invalid")
    monkeypatch.setattr(client, "delete", lambda *_args, **_kwargs: DummyResponse())
    with pytest.raises(Exception, match="boom"):
        client.delete_chats(ids=["chat-1"])


@pytest.mark.p2
@pytest.mark.parametrize("error", ["Chat(invalid_id) not found.", "Duplicate chat ids: chat-1"])
def test_delete_chats_partial_success_returns_no_value(monkeypatch, error):
    class DummyResponse:
        def json(self):
            return {"code": 0, "data": {"success_count": 1, "errors": [error]}}

    client = RAGFlow("token", "http://sdk.invalid")
    calls = []

    def delete(path, payload):
        calls.append((path, payload))
        return DummyResponse()

    monkeypatch.setattr(client, "delete", delete)
    assert client.delete_chats(ids=["chat-1", "invalid_id"]) is None
    assert calls == [("/chats", {"ids": ["chat-1", "invalid_id"], "delete_all": False})]


@pytest.mark.integration
class TestChatAssistantsDelete:
    @pytest.mark.p3
    @pytest.mark.parametrize("selection", ["omitted", "empty", "single", "bulk"])
    def test_basic_scenarios(self, client, chat_factory, selection):
        chats = [chat_factory() for _ in range(3)]
        ids = [chat.id for chat in chats]
        if selection == "omitted":
            client.delete_chats()
            deleted = set()
        elif selection == "empty":
            client.delete_chats(ids=[])
            deleted = set()
        elif selection == "single":
            client.delete_chats(ids=ids[:1])
            deleted = set(ids[:1])
        else:
            client.delete_chats(ids=ids)
            deleted = set(ids)

        for chat in chats:
            assert bool(client.list_chats(id=chat.id)) == (chat.id not in deleted)

    @pytest.mark.p3
    @pytest.mark.parametrize("invalid_id", ["invalid_id", "\n!?。；！？\"'"])
    def test_invalid_id(self, client, chat_factory, invalid_id):
        chat = chat_factory()
        with pytest.raises(Exception, match="not found"):
            client.delete_chats(ids=[invalid_id])
        assert [item.id for item in client.list_chats(id=chat.id)] == [chat.id]

    @pytest.mark.p2
    def test_single_chat_endpoint(self, client, chat_factory):
        chat = chat_factory()
        response = client.delete(f"/chats/{chat.id}", None).json()
        assert response["code"] == 0, response
        assert response["data"] is True
        assert client.list_chats(id=chat.id) == []

    @pytest.mark.p2
    @pytest.mark.parametrize("selection", ["omitted", "none", "empty"])
    def test_delete_all_on_verified_empty_tenant(self, client, chat_factory, isolated_tenant, selection):
        chats = [chat_factory() for _ in range(2)]
        ids = {chat.id for chat in chats}
        # The session preflight verifies an empty dedicated tenant; refuse deletion if it changed.
        response = client.get("/chats", {"page_size": 0}).json()
        assert response["code"] == 0, response
        assert response["data"]["total"] == len(ids)
        assert {chat["id"] for chat in response["data"]["chats"]} == ids
        assert {chat.id for chat in client.list_chats(page_size=0)} == ids
        if selection == "omitted":
            client.delete_chats(delete_all=True)
        elif selection == "none":
            client.delete_chats(ids=None, delete_all=True)
        else:
            client.delete_chats(ids=[], delete_all=True)
        assert all(client.list_chats(id=chat_id) == [] for chat_id in ids)

    @pytest.mark.p2
    def test_explicit_ids_take_precedence_over_delete_all(self, client, chat_factory):
        chats = [chat_factory() for _ in range(2)]
        client.delete_chats(ids=[chats[0].id], delete_all=True)
        assert client.list_chats(id=chats[0].id) == []
        assert [chat.id for chat in client.list_chats(id=chats[1].id)] == [chats[1].id]

    @pytest.mark.p3
    @pytest.mark.parametrize("invalid_position", ["first", "middle", "last"])
    def test_delete_partial_invalid_id(self, client, chat_factory, invalid_position):
        chats = [chat_factory() for _ in range(3)]
        ids = [chat.id for chat in chats]
        position = {"first": 0, "middle": 1, "last": len(ids)}[invalid_position]
        ids.insert(position, "invalid_id")
        response = client.delete("/chats", {"ids": ids}).json()
        assert response["code"] == 0, response
        assert response["data"]["success_count"] == len(chats)
        assert "Chat(invalid_id) not found." in response["data"]["errors"]
        assert all(client.list_chats(id=chat.id) == [] for chat in chats)

    @pytest.mark.p3
    def test_repeated_deletion(self, client, chat_factory):
        chat = chat_factory()
        client.delete_chats(ids=[chat.id])
        with pytest.raises(Exception, match="not found"):
            client.delete_chats(ids=[chat.id])
        assert client.list_chats(id=chat.id) == []

    @pytest.mark.p3
    def test_duplicate_deletion(self, client, chat_factory):
        chat = chat_factory()
        response = client.delete("/chats", {"ids": [chat.id, chat.id]}).json()
        assert response["code"] == 0, response
        assert response["data"]["success_count"] == 1
        assert f"Duplicate chat ids: {chat.id}" in response["data"]["errors"]
        assert client.list_chats(id=chat.id) == []

    @pytest.mark.p3
    def test_concurrent_deletion(self, client, chat_factory):
        chats = [chat_factory(name=f"sdk-chat-delete-{uuid4().hex}") for _ in range(20)]
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(client.delete_chats, ids=[chat.id]) for chat in chats]
            for future in as_completed(futures):
                assert future.result() is None
        assert all(client.list_chats(id=chat.id) == [] for chat in chats)

    @pytest.mark.p3
    def test_bulk_delete_spans_default_list_page(self, client, chat_factory):
        chats = [chat_factory() for _ in range(31)]
        client.delete_chats(ids=[chat.id for chat in chats])
        assert all(client.list_chats(id=chat.id) == [] for chat in chats)

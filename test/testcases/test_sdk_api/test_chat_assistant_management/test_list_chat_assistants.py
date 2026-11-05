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
from beartype.roar import BeartypeCallHintParamViolation
from ragflow_sdk import RAGFlow
from ragflow_sdk.modules.chat import Chat


@pytest.fixture
def chats(chat_factory):
    prefix = f"sdk-chat-list-{uuid4().hex}"
    return [chat_factory(name=f"{prefix}-{index}") for index in range(5)]


@pytest.mark.p2
@pytest.mark.parametrize(
    "params",
    [
        {"page": "a"},
        {"page": "3"},
        {"page_size": "a"},
        {"page_size": "1"},
        {"desc": None},
        {"desc": "true"},
        {"desc": "False"},
        {"desc": "unknown"},
    ],
)
def test_invalid_sdk_parameter_types_before_get(monkeypatch, params):
    client = RAGFlow("token", "http://sdk.invalid")
    calls = []
    monkeypatch.setattr(client, "get", lambda *args, **kwargs: calls.append((args, kwargs)))

    with pytest.raises(BeartypeCallHintParamViolation, match="not instance of"):
        client.list_chats(**params)
    assert calls == []


@pytest.mark.integration
class TestChatAssistantsList:
    @pytest.mark.p3
    def test_default(self, client, chats):
        listed = client.list_chats()
        assert sorted(chat.id for chat in listed) == sorted(chat.id for chat in chats)
        assert all(isinstance(chat, Chat) for chat in listed)

    @pytest.mark.p3
    @pytest.mark.parametrize("page, expected", [(1, (0, 1)), (2, (2, 3)), (3, (4,))])
    def test_page(self, client, chats, page, expected):
        prefix = chats[0].name.rsplit("-", 1)[0]
        listed = client.list_chats(page=page, page_size=2, orderby="name", desc=False, keywords=prefix)
        assert [chat.id for chat in listed] == [chats[index].id for index in expected]

    @pytest.mark.p2
    @pytest.mark.parametrize("page", [0, -1])
    def test_nonpositive_page_is_unpaginated(self, client, chats, page):
        prefix = chats[0].name.rsplit("-", 1)[0]
        listed = client.list_chats(page=page, page_size=2, keywords=prefix)
        assert {chat.id for chat in listed} == {chat.id for chat in chats}

    @pytest.mark.p1
    @pytest.mark.parametrize("page_size", [1, 2, 6])
    def test_page_size(self, client, chats, page_size):
        prefix = chats[0].name.rsplit("-", 1)[0]
        listed = client.list_chats(page_size=page_size, orderby="name", desc=False, keywords=prefix)
        assert [chat.id for chat in listed] == [chat.id for chat in chats[:page_size]]

    @pytest.mark.p2
    @pytest.mark.parametrize("page_size", [0, -1])
    def test_nonpositive_page_size_is_unpaginated(self, client, chats, page_size):
        prefix = chats[0].name.rsplit("-", 1)[0]
        listed = client.list_chats(page_size=page_size, keywords=prefix)
        assert {chat.id for chat in listed} == {chat.id for chat in chats}

    @pytest.mark.p3
    @pytest.mark.parametrize("orderby", ["create_time", "update_time", "name"])
    @pytest.mark.parametrize("desc", [True, False])
    def test_orderby(self, client, chats, orderby, desc):
        prefix = chats[0].name.rsplit("-", 1)[0]
        listed = client.list_chats(orderby=orderby, desc=desc, keywords=prefix)
        assert {chat.id for chat in listed} == {chat.id for chat in chats}
        values = [getattr(chat, orderby) for chat in listed]
        assert values == sorted(values, reverse=desc)

    @pytest.mark.p3
    def test_invalid_orderby(self, client):
        with pytest.raises(Exception, match="invalid orderby field: unknown"):
            client.list_chats(orderby="unknown")

    @pytest.mark.p1
    @pytest.mark.parametrize("term, expected", [(None, 5), ("", 5), ("-1", 1), ("-missing", 0)])
    def test_keywords(self, client, chats, term, expected):
        prefix = chats[0].name.rsplit("-", 1)[0]
        keywords = f"{prefix}{term}" if term else term
        listed = client.list_chats(keywords=keywords)
        if term in (None, ""):
            assert sorted(chat.id for chat in listed) == sorted(chat.id for chat in chats)
        else:
            assert len(listed) == expected
            assert {chat.id for chat in listed} <= {chat.id for chat in chats}
            if expected:
                assert listed[0].name == chats[1].name

    @pytest.mark.p1
    def test_exact_id_and_name_filters(self, client, chats):
        target = chats[1]
        assert [chat.id for chat in client.list_chats(id=target.id)] == [target.id]
        assert [chat.id for chat in client.list_chats(name=target.name)] == [target.id]
        assert [chat.id for chat in client.list_chats(name=target.name, keywords="missing")] == [target.id]
        assert client.list_chats(id=uuid4().hex) == []

    @pytest.mark.p2
    @pytest.mark.parametrize("owner_format", ["string", "list", "other"])
    def test_owner_ids(self, client, chats, owner_format):
        owner_id = chats[0].tenant_id
        owners = [owner_id] if owner_format == "list" else owner_id
        if owner_format == "other":
            owners = uuid4().hex
        prefix = chats[0].name.rsplit("-", 1)[0]
        listed = client.list_chats(owner_ids=owners, keywords=prefix)
        assert {chat.id for chat in listed} == (set() if owner_format == "other" else {chat.id for chat in chats})

    @pytest.mark.p1
    def test_get_chat(self, client, chats):
        target = chats[0]
        result = client.get_chat(target.id)
        assert isinstance(result, Chat)
        assert result.id == target.id
        assert result.name == target.name
        with pytest.raises(Exception, match="Resource not found"):
            client.get_chat(uuid4().hex)

    @pytest.mark.p3
    def test_get_and_keywords_are_separate_lookups(self, client, chats):
        assert client.get_chat(chats[0].id).id == chats[0].id
        assert [chat.id for chat in client.list_chats(keywords=chats[1].name)] == [chats[1].id]
        assert client.get_chat(chats[0].id).name == chats[0].name

    @pytest.mark.p3
    def test_concurrent_list(self, client, chats):
        prefix = chats[0].name.rsplit("-", 1)[0]
        expected_ids = {chat.id for chat in chats}
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(client.list_chats, keywords=prefix) for _ in range(20)]
            for future in as_completed(futures):
                assert {chat.id for chat in future.result()} == expected_ids

    @pytest.mark.p3
    def test_list_chats_after_deleting_associated_dataset(self, client, chat_factory, dataset_factory):
        dataset = dataset_factory()
        document = dataset.upload_documents([{"display_name": "chat-association.txt", "blob": b"chat association"}])[0]
        document.add_chunk(content="chat association")
        chat = chat_factory(dataset_ids=[dataset.id])
        client.delete_datasets(ids=[dataset.id])
        listed = client.list_chats(id=chat.id)
        assert [result.id for result in listed] == [chat.id]
        assert listed[0].dataset_ids == []

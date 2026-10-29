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

from copy import deepcopy
from inspect import unwrap

import conftest as chat_fixtures
import pytest
import requests
from beartype.roar import BeartypeCallHintParamViolation
from ragflow_sdk import Chat, RAGFlow
from ragflow_sdk.modules.session import Session


class _DummyResponse:
    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return deepcopy(self._payload)


@pytest.fixture
def sdk_client():
    return RAGFlow("unit-token", "http://sdk.invalid")


@pytest.fixture
def wire(monkeypatch):
    state = {"response": {"code": 0, "data": {}}, "calls": []}

    def send(_session, method, url, **kwargs):
        state["calls"].append((method.upper(), url, kwargs))
        return _DummyResponse(state["response"])

    monkeypatch.setattr(requests.sessions.Session, "request", send)
    return state


@pytest.mark.parametrize(
    "options",
    [
        {},
        {"dataset_ids": None},
        {"icon": "data:image/png;base64,aWNvbg==", "dataset_ids": ["dataset-1"]},
        {"llm_id": "model@instance@provider", "llm_setting": {"temperature": 0}, "prompt_config": {"quote": False}, "top_n": 4},
        {"llm_id": "", "llm_setting": {}, "prompt_config": {}},
    ],
)
def test_create_chat_request_and_response(sdk_client, wire, options):
    wire["response"]["data"] = {"id": "chat-1", "name": "unit-chat", "llm_setting": {"temperature": 0}}
    chat = sdk_client.create_chat("unit-chat", **options)
    expected = {"name": "unit-chat", "icon": "", "dataset_ids": []}
    expected.update(options)
    expected["dataset_ids"] = options.get("dataset_ids") or []
    method, url, args = wire["calls"].pop()
    assert (method, url) == ("POST", "http://sdk.invalid/api/v1/chats")
    assert args["json"] == expected
    assert args["headers"] == {"Authorization": "Bearer unit-token"}
    assert isinstance(chat, Chat)
    assert (chat.id, chat.name, chat.to_json()["llm_setting"]["temperature"]) == ("chat-1", "unit-chat", 0)


@pytest.mark.parametrize("owner_ids", [None, "owner-1", ["owner-1", "owner-2"]])
def test_list_chats_request_and_response(sdk_client, wire, owner_ids):
    wire["response"]["data"] = {"total": 1, "chats": [{"id": "chat-1", "dataset_ids": ["dataset-1"], "prompt_config": {"quote": False}}]}
    options = {"page": 2, "page_size": 3, "orderby": "name", "desc": False, "id": "chat-1", "name": "exact", "keywords": "fuzzy", "owner_ids": owner_ids}
    chats = sdk_client.list_chats(**options)
    method, url, args = wire["calls"].pop()
    assert (method, url) == ("GET", "http://sdk.invalid/api/v1/chats")
    assert args["params"] == options
    assert len(chats) == 1 and isinstance(chats[0], Chat)
    assert chats[0].id == "chat-1"
    assert chats[0].dataset_ids == ["dataset-1"]
    assert chats[0].to_json()["prompt_config"]["quote"] is False


def test_list_chats_empty(sdk_client, wire):
    wire["response"]["data"] = {"total": 0, "chats": []}
    assert sdk_client.list_chats(page_size=0) == []
    assert wire["calls"][0][2]["params"]["page_size"] == 0


def test_get_chat_request_and_response(sdk_client, wire):
    wire["response"]["data"] = {"id": "chat-1", "name": "detail", "llm_id": "model", "llm_setting": {"max_tokens": 64}, "prompt_config": {"system": "sys"}}
    chat = sdk_client.get_chat("chat-1")
    assert wire["calls"][0][:2] == ("GET", "http://sdk.invalid/api/v1/chats/chat-1")
    assert isinstance(chat, Chat)
    assert (chat.id, chat.name, chat.llm_id) == ("chat-1", "detail", "model")
    assert chat.to_json()["llm_setting"]["max_tokens"] == 64
    assert chat.to_json()["prompt_config"]["system"] == "sys"


@pytest.mark.parametrize("options", [{}, {"ids": None}, {"ids": []}, {"ids": ["chat-1"]}, {"delete_all": True}, {"ids": [], "delete_all": True}, {"ids": ["chat-1"], "delete_all": True}])
def test_delete_chats_explicit_payload(sdk_client, wire, options):
    sdk_client.delete_chats(**options)
    method, url, args = wire["calls"].pop()
    assert (method, url) == ("DELETE", "http://sdk.invalid/api/v1/chats")
    assert args["json"] == {"ids": options.get("ids"), "delete_all": options.get("delete_all", False)}


@pytest.mark.parametrize("operation", ["create", "list", "get", "delete", "update", "create_session", "list_sessions", "delete_sessions"])
def test_chat_operations_propagate_server_errors(sdk_client, wire, operation):
    wire["response"] = {"code": 102, "message": "contract sentinel error"}
    chat = Chat(sdk_client, {"id": "chat-1"})
    operations = {
        "create": lambda: sdk_client.create_chat("name"),
        "list": sdk_client.list_chats,
        "get": lambda: sdk_client.get_chat("chat-1"),
        "delete": lambda: sdk_client.delete_chats(["chat-1"]),
        "update": lambda: chat.update({"name": "renamed"}),
        "create_session": chat.create_session,
        "list_sessions": chat.list_sessions,
        "delete_sessions": lambda: chat.delete_sessions(["session-1"]),
    }
    with pytest.raises(Exception, match="^contract sentinel error$"):
        operations[operation]()
    assert len(wire["calls"]) == 1


@pytest.mark.parametrize("payload", [None, [], "bad", 1, False])
def test_update_rejects_non_dict(sdk_client, payload):
    with pytest.raises(BeartypeCallHintParamViolation):
        Chat(sdk_client, {"id": "chat-1"}).update(payload)


def test_update_rejects_non_dict_without_type_wrapper(sdk_client):
    chat = Chat(sdk_client, {"id": "chat-1"})
    with pytest.raises(Exception, match="`update_message` must be a dict"):
        unwrap(Chat.update)(chat, "bad")


@pytest.mark.parametrize("payload", [{}, {"name": "renamed"}, {"llm_setting": {"temperature": 0}}, {"prompt_config": {"quote": False}}])
def test_update_uses_patch_for_partial_payload(sdk_client, wire, payload):
    chat = Chat(sdk_client, {"id": "chat-1"})
    assert chat.update(payload) is None
    method, url, args = wire["calls"].pop()
    assert (method, url) == ("PATCH", "http://sdk.invalid/api/v1/chats/chat-1")
    assert args["json"] == payload


@pytest.mark.parametrize("operation, options", [("list", {"page": "bad"}), ("list", {"page_size": []}), ("list", {"desc": "false"}), ("get", {"chat_id": 123}), ("delete", {"ids": "chat-1"})])
def test_sdk_rejects_typed_arguments_before_http(sdk_client, operation, options):
    call = {"list": sdk_client.list_chats, "get": sdk_client.get_chat, "delete": sdk_client.delete_chats}[operation]
    with pytest.raises(BeartypeCallHintParamViolation):
        call(**options)


@pytest.mark.parametrize(
    "options",
    [{"name": 1}, {"dataset_ids": "dataset-1"}, {"llm_setting": "invalid"}, {"prompt_config": "invalid"}, {"icon": 1}, {"llm_id": 123}],
)
def test_create_chat_rejects_typed_arguments_before_http(sdk_client, options):
    with pytest.raises(BeartypeCallHintParamViolation):
        sdk_client.create_chat(**{"name": "typed-chat", **options})


def test_chat_create_session_raises_server_error_message(sdk_client, wire):
    wire["response"] = {"code": 102, "message": "`name` can not be empty"}
    with pytest.raises(Exception, match="`name` can not be empty"):
        Chat(sdk_client, {"id": "chat-1"}).create_session(name="")
    method, url, args = wire["calls"][0]
    assert (method, url) == ("POST", "http://sdk.invalid/api/v1/chats/chat-1/sessions")
    assert args["json"] == {"name": ""}


def test_chat_list_sessions_forwards_restful_query_params(sdk_client, wire):
    wire["response"]["data"] = [{"id": "session-1", "chat_id": "chat-1", "name": "one"}, {"id": "session-2", "chat_id": "chat-1", "name": "two"}]
    chat = Chat(sdk_client, {"id": "chat-1"})
    options = {"page": 2, "page_size": 2, "orderby": "create_time", "desc": False, "id": "session-1", "name": "one", "user_id": "user-1"}
    sessions = chat.list_sessions(**options)
    method, url, args = wire["calls"].pop()
    assert (method, url) == ("GET", "http://sdk.invalid/api/v1/chats/chat-1/sessions")
    assert args["params"] == options
    assert all(isinstance(item, Session) for item in sessions)
    assert [item.id for item in sessions] == ["session-1", "session-2"]
    assert len(chat.list_sessions(page_size=0)) == 2
    assert wire["calls"][-1][2]["params"]["page_size"] == 0


def test_integration_settings_require_explicit_isolation(monkeypatch):
    for key in ("RAGFLOW_SDK_BASE_URL", "RAGFLOW_SDK_API_KEY", "RAGFLOW_SDK_ISOLATED"):
        monkeypatch.delenv(key, raising=False)
    with pytest.raises(pytest.fail.Exception, match="Integration environment required"):
        unwrap(chat_fixtures.integration_settings)()


def test_fixture_cleanup_only_deletes_owned_resources(monkeypatch):
    chats = {"unrelated-chat"}
    datasets = {"unrelated-dataset"}
    deleted = []

    class FakeClient:
        def create_chat(self, **kwargs):
            chats.add("owned-chat")
            return type("Chat", (), {"id": "owned-chat"})()

        def create_dataset(self, **kwargs):
            datasets.add("owned-dataset")
            return type("Dataset", (), {"id": "owned-dataset"})()

        def list_chats(self, id):
            return [id] if id in chats else []

        def list_datasets(self, id):
            return [id] if id in datasets else []

        def delete_chats(self, ids):
            deleted.extend(ids)
            chats.difference_update(ids)

        def delete_datasets(self, ids):
            deleted.extend(ids)
            datasets.difference_update(ids)

    monkeypatch.setattr(chat_fixtures, "RAGFlow", lambda *_args: FakeClient())
    lifecycle = unwrap(chat_fixtures.client)({"api_key": "fake", "base_url": "http://sdk.invalid"}, monkeypatch)
    client = next(lifecycle)
    client.create_chat(name="owned")
    client.create_dataset(name="owned")
    with pytest.raises(StopIteration):
        next(lifecycle)
    assert deleted == ["owned-chat", "owned-dataset"]
    assert chats == {"unrelated-chat"}
    assert datasets == {"unrelated-dataset"}

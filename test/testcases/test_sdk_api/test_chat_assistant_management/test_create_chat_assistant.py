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

import base64
from uuid import uuid4

import pytest
from PIL import Image

pytestmark = pytest.mark.integration


@pytest.mark.p3
@pytest.mark.parametrize(
    "name, expected",
    [
        ("valid_name", None),
        ("a" * 255, None),
        ("a" * 256, "larger than 255"),
        ("", "`name` is required"),
        ("   ", "`name` is required"),
    ],
)
def test_name(chat_factory, name, expected):
    if expected:
        with pytest.raises(Exception, match=expected):
            chat_factory(name=name)
    else:
        chat = chat_factory(name=name)
        assert chat.id and chat.name == name


@pytest.mark.p3
@pytest.mark.parametrize("case_variant", [False, True], ids=["same-case", "case-insensitive"])
def test_duplicate_name(chat_factory, case_variant):
    name = f"sdk-chat-duplicate-{uuid4().hex}"
    first = chat_factory(name=name)
    second = chat_factory(name=name.upper() if case_variant else name)
    assert first.id != second.id
    assert second.name == f"{name.upper() if case_variant else name}(1)"


@pytest.mark.p3
@pytest.mark.parametrize("dataset_ids", [[], ["invalid_dataset_id"]])
def test_dataset_ids(chat_factory, dataset_ids):
    if dataset_ids == []:
        chat = chat_factory(dataset_ids=dataset_ids)
        assert chat.dataset_ids == []
    else:
        with pytest.raises(Exception, match="Resource not found"):
            chat_factory(dataset_ids=dataset_ids)


@pytest.mark.p3
def test_dataset_association(chat_factory, dataset_factory):
    dataset = dataset_factory()
    # A manually added chunk satisfies the Go chat endpoint without starting a parser.
    document = dataset.upload_documents([{"display_name": "chat-association.txt", "blob": b"chat association"}])[0]
    document.add_chunk(content="chat association")

    chat = chat_factory(dataset_ids=[dataset.id])
    assert chat.dataset_ids == [dataset.id]


@pytest.mark.p3
def test_icon(chat_factory, client, tmp_path):
    image = tmp_path / "chat-icon.png"
    Image.new("RGB", (1, 1), color="blue").save(image)
    icon = base64.b64encode(image.read_bytes()).decode("ascii")
    chat = chat_factory(icon=icon)
    assert chat.icon == icon
    assert client.get_chat(chat.id).icon == icon


@pytest.mark.p3
def test_default_settings(chat_factory, client):
    chat = chat_factory()
    persisted = client.get_chat(chat.id)
    assert persisted.dataset_ids == []
    assert persisted.llm_id == chat.llm_id
    assert persisted.prompt_config.system == ""
    assert persisted.prompt_config.quote is True
    assert persisted.prompt_config.parameters == [{"key": "knowledge", "optional": False}]


@pytest.mark.p3
def test_optional_fields(chat_factory, client):
    chat = chat_factory(description="SDK chat", top_n=8, similarity_threshold=0.4)
    persisted = client.get_chat(chat.id)
    assert persisted.description == "SDK chat"
    assert persisted.top_n == 8
    assert persisted.similarity_threshold == 0.4


@pytest.mark.p3
@pytest.mark.parametrize(
    "llm_setting",
    [
        {},
        {"temperature": 0},
        {"temperature": 1},
        {"top_p": 0},
        {"top_p": 1},
        {"presence_penalty": 0},
        {"presence_penalty": 1},
        {"frequency_penalty": 0},
        {"frequency_penalty": 1},
        {"max_tokens": 1024},
    ],
)
def test_llm_setting(chat_factory, client, llm_setting):
    chat = chat_factory(llm_setting=llm_setting)
    persisted = client.get_chat(chat.id)
    assert chat.llm_setting.to_json() == llm_setting
    assert persisted.llm_setting.to_json() == llm_setting


@pytest.mark.p3
@pytest.mark.parametrize(
    "llm_setting",
    [
        {"temperature": "invalid"},
        {"top_p": "invalid"},
        {"presence_penalty": "invalid"},
        {"frequency_penalty": "invalid"},
        {"max_tokens": "invalid"},
    ],
)
@pytest.mark.skip(reason="Go ChatService.Create accepts invalid generation parameter types without validation")
def test_invalid_llm_setting(chat_factory, llm_setting):
    with pytest.raises(Exception, match="llm_setting"):
        chat_factory(llm_setting=llm_setting)


@pytest.mark.p3
@pytest.mark.skip(reason="Go ChatService.Create discards non-object llm_setting rather than rejecting it")
def test_create_rejects_invalid_llm_setting_object(client):
    response = client.post("/chats", {"name": f"sdk-chat-{uuid4().hex}", "llm_setting": "invalid"}).json()
    try:
        assert response["code"] != 0
        assert "`llm_setting` should be an object" in response["message"]
    finally:
        if (response.get("data") or {}).get("id"):
            client.delete_chats(ids=[response["data"]["id"]])


@pytest.mark.p3
@pytest.mark.parametrize("valid", [True, False], ids=["configured", "unknown"])
def test_llm_id(chat_factory, client, valid):
    llm_id = chat_factory().llm_id if valid else "sdk-no-such-model"
    if valid:
        assert llm_id, "Explicit llm_id test requires a configured default chat model"
        chat = chat_factory(llm_id=llm_id)
        assert chat.llm_id == llm_id
        assert client.get_chat(chat.id).llm_id == llm_id
    else:
        with pytest.raises(Exception, match="`llm_id` .* doesn't exist"):
            chat_factory(llm_id=llm_id)


@pytest.mark.p3
@pytest.mark.parametrize(
    "prompt_config",
    [
        {"empty_response": "Hello World"},
        {"empty_response": ""},
        {"empty_response": "!@#$%^&*()"},
        {"empty_response": "中文测试"},
        {"empty_response": " "},
        {"prologue": "Hello World"},
        {"prologue": ""},
        {"prologue": "!@#$%^&*()"},
        {"prologue": "中文测试"},
        {"prologue": " "},
        {"quote": True},
        {"quote": False},
        {"system": "Hello World {knowledge}"},
        {"system": "{knowledge}"},
        {"system": "!@#$%^&*() {knowledge}"},
        {"system": "中文测试 {knowledge}"},
        {"system": "Hello World"},
        {"system": "Hello World", "parameters": []},
    ],
)
def test_prompt_config(chat_factory, client, prompt_config):
    chat = chat_factory(prompt_config=prompt_config)
    persisted = client.get_chat(chat.id)
    for key, value in prompt_config.items():
        assert getattr(chat.prompt_config, key) == value
        assert getattr(persisted.prompt_config, key) == value


@pytest.mark.p3
@pytest.mark.parametrize(
    "prompt_config",
    [
        {"empty_response": 123},
        {"empty_response": True},
        {"prologue": 123},
        {"prologue": True},
        {"system": 123},
        {"parameters": "invalid"},
        {"quote": "invalid"},
    ],
)
@pytest.mark.skip(reason="Go validatePromptConfigParameters only checks duplicate keys; Create persists invalid prompt field types")
def test_invalid_prompt_fields(chat_factory, prompt_config):
    with pytest.raises(Exception, match="prompt_config"):
        chat_factory(prompt_config=prompt_config)


@pytest.mark.p3
def test_create_rejects_invalid_prompt_config_object(client):
    response = client.post("/chats", {"name": f"sdk-chat-{uuid4().hex}", "prompt_config": "invalid"}).json()
    try:
        assert response["code"] != 0
        assert "`prompt_config` should be an object" in response["message"]
    finally:
        if (response.get("data") or {}).get("id"):
            client.delete_chats(ids=[response["data"]["id"]])


@pytest.mark.p3
def test_duplicate_prompt_parameters(chat_factory):
    with pytest.raises(Exception, match="`parameters` contains duplicate key: knowledge"):
        chat_factory(prompt_config={"parameters": [{"key": "knowledge"}, {"key": "knowledge"}]})


@pytest.mark.p3
def test_unparsed_dataset(chat_factory, dataset_factory):
    dataset = dataset_factory()
    with pytest.raises(Exception, match="doesn't own parsed file"):
        chat_factory(dataset_ids=[dataset.id])

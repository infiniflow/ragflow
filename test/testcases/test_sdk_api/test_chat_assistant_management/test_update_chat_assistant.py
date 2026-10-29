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
CHAT_NAME_MAX_BYTES = 255


@pytest.mark.p3
@pytest.mark.parametrize(
    "name_case, expected_message",
    [
        ("unique", None),
        ("max_length", None),
        ("too_long", "larger than 255"),
        ("non_string", "chat name must be a string"),
        ("empty", "`name` cannot be empty"),
        ("duplicate", "duplicated chat name"),
        ("duplicate_case_insensitive", "duplicated chat name"),
    ],
)
def test_name(client, chat_factory, name_case, expected_message):
    chat = chat_factory()
    original_name = chat.name
    name = {
        "unique": f"renamed-{uuid4().hex}",
        "max_length": "a" * CHAT_NAME_MAX_BYTES,
        "too_long": "a" * (CHAT_NAME_MAX_BYTES + 1),
        "non_string": 1,
        "empty": "",
    }.get(name_case)
    if name_case.startswith("duplicate"):
        other = chat_factory()
        name = other.name.upper() if name_case.endswith("insensitive") else other.name

    if expected_message:
        with pytest.raises(Exception, match=expected_message):
            chat.update({"name": name})
        assert client.get_chat(chat.id).name == original_name
    else:
        chat.update({"name": name})
        assert client.get_chat(chat.id).name == name


@pytest.mark.p3
def test_partial_patch_preserves_unmentioned_fields(client, chat_factory):
    chat = chat_factory()
    before = client.get_chat(chat.id)
    chat.update({"top_n": before.top_n + 1})
    after = client.get_chat(chat.id)
    assert after.top_n == before.top_n + 1
    assert after.name == before.name
    assert after.icon == before.icon
    assert after.dataset_ids == before.dataset_ids
    assert after.llm_id == before.llm_id
    assert after.llm_setting.to_json() == before.llm_setting.to_json()
    assert after.prompt_config.to_json() == before.prompt_config.to_json()


@pytest.mark.p3
def test_empty_patch_preserves_settings(client, chat_factory):
    chat = chat_factory()
    before = client.get_chat(chat.id)
    chat.update({})
    after = client.get_chat(chat.id)
    assert after.name == before.name
    assert after.dataset_ids == before.dataset_ids
    assert after.llm_setting.to_json() == before.llm_setting.to_json()
    assert after.prompt_config.to_json() == before.prompt_config.to_json()


@pytest.mark.p3
def test_icon(client, chat_factory, tmp_path):
    chat = chat_factory()
    image = tmp_path / "ragflow_test.png"
    Image.new("RGB", (1, 1), color="blue").save(image)
    icon = base64.b64encode(image.read_bytes()).decode("ascii")

    chat.update({"icon": icon})

    updated = client.get_chat(chat.id)
    assert updated.icon == icon
    assert updated.name == chat.name


@pytest.mark.p3
@pytest.mark.parametrize(
    "valid, expected_message",
    [
        (True, None),
        (False, "`llm_id` unknown doesn't exist"),
    ],
)
def test_llm_id(client, chat_factory, valid, expected_message):
    configured_model = chat_factory().llm_id
    chat = chat_factory(llm_id="")
    llm_id = configured_model if valid else "unknown"
    if expected_message:
        with pytest.raises(Exception, match=expected_message):
            chat.update({"llm_id": llm_id})
        assert client.get_chat(chat.id).llm_id == chat.llm_id
    else:
        assert llm_id, "Explicit llm_id test requires a configured default chat model"
        assert client.get_chat(chat.id).llm_id != llm_id
        chat.update({"llm_id": llm_id})
        assert client.get_chat(chat.id).llm_id == llm_id


@pytest.mark.p3
@pytest.mark.parametrize(
    "setting",
    [
        {"temperature": 0},
        {"temperature": 1},
        {"top_p": 0},
        {"top_p": 1},
        {"presence_penalty": 0},
        {"presence_penalty": 1},
        {"frequency_penalty": 0},
        {"frequency_penalty": 1},
        {"max_tokens": 0},
        {"max_tokens": 10},
        {"max_tokens": 1024},
    ],
)
def test_llm_setting(client, chat_factory, setting):
    chat = chat_factory()
    before = client.get_chat(chat.id).llm_setting.to_json()

    chat.update({"llm_setting": setting})

    assert client.get_chat(chat.id).llm_setting.to_json() == {**before, **setting}


@pytest.mark.p3
@pytest.mark.parametrize(
    "field",
    [
        pytest.param(
            field,
            marks=pytest.mark.skip(reason="Go PATCH persists string-valued numeric llm_setting fields (internal/service/chat.go:updateChatREST)"),
        )
        for field in (
            "temperature",
            "top_p",
            "presence_penalty",
            "frequency_penalty",
            "max_tokens",
        )
    ],
)
def test_llm_setting_rejects_non_numeric(client, chat_factory, field):
    chat = chat_factory()
    before = client.get_chat(chat.id).llm_setting.to_json()
    with pytest.raises(Exception, match=field):
        chat.update({"llm_setting": {field: "invalid"}})
    assert client.get_chat(chat.id).llm_setting.to_json() == before


@pytest.mark.p3
def test_dataset_association_updates(client, chat_factory, dataset_factory):
    chat = chat_factory()
    first, second = dataset_factory(), dataset_factory()
    for dataset in (first, second):
        document = dataset.upload_documents([{"display_name": "chat-association.txt", "blob": b"chat association"}])[0]
        document.add_chunk(content="chat association")

    chat.update({"dataset_ids": [first.id]})
    assert client.get_chat(chat.id).dataset_ids == [first.id]

    chat.update({"dataset_ids": [second.id]})
    assert client.get_chat(chat.id).dataset_ids == [second.id]

    chat.update({"dataset_ids": []})
    assert client.get_chat(chat.id).dataset_ids == []


@pytest.mark.p3
@pytest.mark.parametrize(
    "field, value, expected_message",
    [
        ("llm_setting", "invalid", "`llm_setting` should be an object"),
        ("prompt_config", "invalid", "`prompt_config` should be an object"),
        ("dataset_ids", "invalid", "`dataset_ids` should be a list"),
    ],
)
def test_rejects_non_object_settings_and_dataset_ids(client, chat_factory, field, value, expected_message):
    chat = chat_factory()
    original = client.get_chat(chat.id)
    before = original.dataset_ids if field == "dataset_ids" else getattr(original, field).to_json()
    with pytest.raises(Exception, match=expected_message):
        chat.update({field: value})
    persisted = client.get_chat(chat.id)
    after = persisted.dataset_ids if field == "dataset_ids" else getattr(persisted, field).to_json()
    assert after == before


@pytest.mark.p3
def test_rejects_unparsed_dataset(client, chat_factory, dataset_factory):
    chat = chat_factory()
    dataset = dataset_factory()
    with pytest.raises(Exception, match="doesn't own parsed file"):
        chat.update({"dataset_ids": [dataset.id]})
    assert client.get_chat(chat.id).dataset_ids == []


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
def test_prompt_config(client, chat_factory, prompt_config):
    chat = chat_factory()
    before = client.get_chat(chat.id).prompt_config.to_json()

    chat.update({"prompt_config": prompt_config})

    assert client.get_chat(chat.id).prompt_config.to_json() == {**before, **prompt_config}


@pytest.mark.p3
@pytest.mark.parametrize(
    "field, value",
    [
        pytest.param(
            field,
            value,
            id=f"{field}-{value}",
            marks=pytest.mark.skip(reason="Go PATCH merges prompt_config without validating documented string fields (internal/service/chat.go:updateChatREST)"),
        )
        for field, values in (
            ("empty_response", (123, True)),
            ("prologue", (123, True)),
            ("system", (123,)),
        )
        for value in values
    ],
)
def test_prompt_config_rejects_invalid_text(client, chat_factory, field, value):
    chat = chat_factory()
    before = client.get_chat(chat.id).prompt_config.to_json()
    with pytest.raises(Exception, match=field):
        chat.update({"prompt_config": {field: value}})
    assert client.get_chat(chat.id).prompt_config.to_json() == before


@pytest.mark.p3
@pytest.mark.skip(reason="Go PATCH replaces nested prompt_config.reference_metadata instead of deep-merging it (internal/service/chat.go:mergeJSONMap)")
def test_prompt_config_preserves_nested_reference_metadata(client, chat_factory):
    chat = chat_factory(prompt_config={"reference_metadata": {"include": False, "fields": ["source"]}})
    before = client.get_chat(chat.id).prompt_config.reference_metadata.to_json()
    assert before == {"include": False, "fields": ["source"]}

    chat.update({"prompt_config": {"reference_metadata": {"include": True}}})
    after = client.get_chat(chat.id).prompt_config.reference_metadata.to_json()
    assert after == {"include": True, "fields": ["source"]}


@pytest.mark.p3
def test_prompt_config_rejects_duplicate_parameter_keys(client, chat_factory):
    chat = chat_factory()
    before = client.get_chat(chat.id).prompt_config.to_json()
    with pytest.raises(Exception, match="`parameters` contains duplicate key: knowledge"):
        chat.update({"prompt_config": {"parameters": [{"key": "knowledge"}, {"key": "knowledge"}]}})
    assert client.get_chat(chat.id).prompt_config.to_json() == before

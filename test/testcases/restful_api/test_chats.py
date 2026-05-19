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

import pytest

from test.testcases.utils import encode_avatar
from test.testcases.utils.file_utils import create_image_file


DEFAULT_CHAT_EMPTY_RESPONSE = "Sorry! No relevant content was found in the knowledge base!"
DEFAULT_CHAT_PROLOGUE = "Hi! I'm your assistant. What can I do for you?"
DEFAULT_CHAT_SYSTEM_PROMPT = (
    'You are an intelligent assistant. Please summarize the content of the dataset to answer the question. '
    'Please list the data in the dataset and answer in detail. When all dataset content is irrelevant to the '
    'question, your answer must include the sentence "The answer you are looking for is not found in the dataset!" '
    "Answers need to consider chat history.\n"
    "      Here is the knowledge base:\n"
    "      {knowledge}\n"
    "      The above is the knowledge base."
)


def _get_nested(data, path):
    current = data
    for key in path:
        current = current[key]
    return current


@pytest.mark.p1
class TestChatsAuthorization:
    def test_create_requires_auth(self, rest_client_noauth):
        res = rest_client_noauth.post("/chats", json={"name": "chat_auth", "dataset_ids": []})
        assert res.status_code == 401


@pytest.mark.p1
def test_chat_crud_cycle(rest_client, clear_chats):
    create_res = rest_client.post(
        "/chats",
        json={"name": "restful_chat_crud", "dataset_ids": []},
    )
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    chat_id = create_payload["data"]["id"]

    list_res = rest_client.get("/chats", params={"id": chat_id})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    chats = list_payload["data"]["chats"]
    assert len(chats) == 1, list_payload
    assert chats[0]["id"] == chat_id, list_payload

    get_res = rest_client.get(f"/chats/{chat_id}")
    assert get_res.status_code == 200
    get_payload = get_res.json()
    assert get_payload["code"] == 0, get_payload
    assert get_payload["data"]["id"] == chat_id, get_payload

    update_res = rest_client.put(
        f"/chats/{chat_id}",
        json={"name": "restful_chat_crud_updated", "dataset_ids": []},
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload
    assert update_payload["data"]["name"] == "restful_chat_crud_updated", update_payload

    patch_res = rest_client.patch(f"/chats/{chat_id}", json={"name": "restful_chat_crud_patched"})
    assert patch_res.status_code == 200
    patch_payload = patch_res.json()
    assert patch_payload["code"] == 0, patch_payload
    assert patch_payload["data"]["name"] == "restful_chat_crud_patched", patch_payload

    delete_res = rest_client.delete("/chats", json={"ids": [chat_id]})
    assert delete_res.status_code == 200
    delete_payload = delete_res.json()
    assert delete_payload["code"] == 0, delete_payload
    assert delete_payload["data"]["success_count"] == 1, delete_payload

    list_after_delete = rest_client.get("/chats", params={"id": chat_id})
    assert list_after_delete.status_code == 200
    list_after_delete_payload = list_after_delete.json()
    assert list_after_delete_payload["code"] == 0, list_after_delete_payload
    assert list_after_delete_payload["data"]["chats"] == [], list_after_delete_payload


@pytest.mark.p2
@pytest.mark.parametrize(
    "name, expected_fragment",
    [
        ("", "`name` is required."),
        (" ", "`name` is required."),
    ],
)
def test_chat_create_name_validation(rest_client, clear_chats, name, expected_fragment):
    res = rest_client.post("/chats", json={"name": name, "dataset_ids": []})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert expected_fragment in payload["message"], payload


@pytest.mark.p2
def test_chat_duplicate_name_validation(rest_client, clear_chats):
    first = rest_client.post("/chats", json={"name": "duplicate_chat_name", "dataset_ids": []})
    assert first.status_code == 200
    first_payload = first.json()
    assert first_payload["code"] == 0, first_payload

    second = rest_client.post("/chats", json={"name": "duplicate_chat_name", "dataset_ids": []})
    assert second.status_code == 200
    second_payload = second.json()
    assert second_payload["code"] == 102, second_payload
    assert "Duplicated chat name" in second_payload["message"], second_payload


@pytest.mark.p2
def test_chat_list_pagination(rest_client, clear_chats):
    for i in range(3):
        res = rest_client.post("/chats", json={"name": f"chat_page_{i}", "dataset_ids": []})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 0, payload

    page_res = rest_client.get("/chats", params={"page": 1, "page_size": 2, "orderby": "create_time", "desc": "true"})
    assert page_res.status_code == 200
    page_payload = page_res.json()
    assert page_payload["code"] == 0, page_payload
    assert len(page_payload["data"]["chats"]) == 2, page_payload
    assert page_payload["data"]["total"] >= 3, page_payload


@pytest.mark.p1
def test_chat_create_dataset_ids_contract(rest_client, clear_chats, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    cases = [
        ("empty dataset_ids", [], 0, "", []),
        ("owned parsed dataset", [dataset_id], 0, "", [dataset_id]),
        ("invalid dataset id", ["invalid_dataset_id"], 102, "You don't own the dataset invalid_dataset_id", None),
        ("dataset_ids wrong type", "invalid_dataset_id", 102, "`dataset_ids` should be a list.", None),
    ]

    for index, (scenario_name, dataset_ids, expected_code, expected_message, expected_dataset_ids) in enumerate(cases, start=1):
        res = rest_client.post(
            "/chats",
            json={"name": f"restful_chat_dataset_ids_{index}", "dataset_ids": dataset_ids},
        )
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == expected_code, (scenario_name, payload)
        if expected_code == 0:
            assert payload["data"]["dataset_ids"] == expected_dataset_ids, (scenario_name, payload)
        else:
            assert payload["message"] == expected_message, (scenario_name, payload)


@pytest.mark.p2
def test_chat_create_avatar_contract(rest_client, clear_chats, tmp_path):
    image_path = create_image_file(tmp_path / "restful_chat_avatar.png")
    encoded_avatar = encode_avatar(image_path)

    res = rest_client.post(
        "/chats",
        json={"name": "restful_chat_avatar", "dataset_ids": [], "icon": encoded_avatar},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert payload["data"]["icon"] == encoded_avatar, payload


@pytest.mark.p2
def test_chat_create_llm_contract(rest_client, clear_chats, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    cases = [
        ("default llm", {}, 0, "", "glm-4-flash@ZHIPU-AI", {}),
        ("explicit llm_id", {"llm_id": "glm-4"}, 0, "", "glm-4", {}),
        ("unknown llm_id", {"llm_id": "unknown"}, 102, "`llm_id` unknown doesn't exist", None, None),
        ("temperature zero", {"llm_setting": {"temperature": 0}}, 0, "", "glm-4-flash@ZHIPU-AI", {"temperature": 0}),
        ("temperature one", {"llm_setting": {"temperature": 1}}, 0, "", "glm-4-flash@ZHIPU-AI", {"temperature": 1}),
        ("temperature negative one", {"llm_setting": {"temperature": -1}}, 0, "", "glm-4-flash@ZHIPU-AI", {"temperature": -1}),
        ("temperature ten", {"llm_setting": {"temperature": 10}}, 0, "", "glm-4-flash@ZHIPU-AI", {"temperature": 10}),
        ("temperature string", {"llm_setting": {"temperature": "a"}}, 0, "", "glm-4-flash@ZHIPU-AI", {"temperature": "a"}),
        ("top_p zero", {"llm_setting": {"top_p": 0}}, 0, "", "glm-4-flash@ZHIPU-AI", {"top_p": 0}),
        ("top_p one", {"llm_setting": {"top_p": 1}}, 0, "", "glm-4-flash@ZHIPU-AI", {"top_p": 1}),
        ("top_p negative one", {"llm_setting": {"top_p": -1}}, 0, "", "glm-4-flash@ZHIPU-AI", {"top_p": -1}),
        ("top_p ten", {"llm_setting": {"top_p": 10}}, 0, "", "glm-4-flash@ZHIPU-AI", {"top_p": 10}),
        ("top_p string", {"llm_setting": {"top_p": "a"}}, 0, "", "glm-4-flash@ZHIPU-AI", {"top_p": "a"}),
        ("presence_penalty zero", {"llm_setting": {"presence_penalty": 0}}, 0, "", "glm-4-flash@ZHIPU-AI", {"presence_penalty": 0}),
        ("presence_penalty one", {"llm_setting": {"presence_penalty": 1}}, 0, "", "glm-4-flash@ZHIPU-AI", {"presence_penalty": 1}),
        ("presence_penalty negative one", {"llm_setting": {"presence_penalty": -1}}, 0, "", "glm-4-flash@ZHIPU-AI", {"presence_penalty": -1}),
        ("presence_penalty ten", {"llm_setting": {"presence_penalty": 10}}, 0, "", "glm-4-flash@ZHIPU-AI", {"presence_penalty": 10}),
        ("presence_penalty string", {"llm_setting": {"presence_penalty": "a"}}, 0, "", "glm-4-flash@ZHIPU-AI", {"presence_penalty": "a"}),
        ("frequency_penalty zero", {"llm_setting": {"frequency_penalty": 0}}, 0, "", "glm-4-flash@ZHIPU-AI", {"frequency_penalty": 0}),
        ("frequency_penalty one", {"llm_setting": {"frequency_penalty": 1}}, 0, "", "glm-4-flash@ZHIPU-AI", {"frequency_penalty": 1}),
        ("frequency_penalty negative one", {"llm_setting": {"frequency_penalty": -1}}, 0, "", "glm-4-flash@ZHIPU-AI", {"frequency_penalty": -1}),
        ("frequency_penalty ten", {"llm_setting": {"frequency_penalty": 10}}, 0, "", "glm-4-flash@ZHIPU-AI", {"frequency_penalty": 10}),
        ("frequency_penalty string", {"llm_setting": {"frequency_penalty": "a"}}, 0, "", "glm-4-flash@ZHIPU-AI", {"frequency_penalty": "a"}),
        ("max_token zero", {"llm_setting": {"max_token": 0}}, 0, "", "glm-4-flash@ZHIPU-AI", {"max_token": 0}),
        ("max_token 1024", {"llm_setting": {"max_token": 1024}}, 0, "", "glm-4-flash@ZHIPU-AI", {"max_token": 1024}),
        ("max_token negative one", {"llm_setting": {"max_token": -1}}, 0, "", "glm-4-flash@ZHIPU-AI", {"max_token": -1}),
        ("max_token ten", {"llm_setting": {"max_token": 10}}, 0, "", "glm-4-flash@ZHIPU-AI", {"max_token": 10}),
        ("max_token string", {"llm_setting": {"max_token": "a"}}, 0, "", "glm-4-flash@ZHIPU-AI", {"max_token": "a"}),
        ("unknown llm setting key", {"llm_setting": {"unknown": "unknown"}}, 0, "", "glm-4-flash@ZHIPU-AI", {"unknown": "unknown"}),
    ]

    for index, (scenario_name, extra_payload, expected_code, expected_message, expected_llm_id, expected_llm_setting) in enumerate(cases, start=1):
        payload = {
            "name": f"restful_chat_llm_{index}",
            "dataset_ids": [dataset_id],
        }
        payload.update(extra_payload)
        res = rest_client.post("/chats", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if expected_code == 0:
            assert body["data"]["llm_id"] == expected_llm_id, (scenario_name, body)
            assert body["data"]["llm_setting"] == expected_llm_setting, (scenario_name, body)
        else:
            assert body["message"] == expected_message, (scenario_name, body)


@pytest.mark.p2
def test_chat_create_prompt_contract(rest_client, clear_chats):
    cases = [
        (
            "default prompt config",
            {},
            {
                ("similarity_threshold",): 0.1,
                ("vector_similarity_weight",): 0.3,
                ("top_n",): 6,
                ("rerank_id",): "",
                ("prompt_config", "parameters"): [{"key": "knowledge", "optional": False}],
                ("prompt_config", "empty_response"): DEFAULT_CHAT_EMPTY_RESPONSE,
                ("prompt_config", "prologue"): DEFAULT_CHAT_PROLOGUE,
                ("prompt_config", "quote"): True,
                ("prompt_config", "system"): DEFAULT_CHAT_SYSTEM_PROMPT,
            },
        ),
        ("similarity_threshold zero", {"similarity_threshold": 0}, {("similarity_threshold",): 0}),
        ("similarity_threshold one", {"similarity_threshold": 1}, {("similarity_threshold",): 1}),
        ("similarity_threshold negative one", {"similarity_threshold": -1}, {("similarity_threshold",): -1.0}),
        ("similarity_threshold ten", {"similarity_threshold": 10}, {("similarity_threshold",): 10.0}),
        ("similarity_threshold string", {"similarity_threshold": "a"}, {("similarity_threshold",): 0.0}),
        ("vector_similarity_weight one", {"vector_similarity_weight": 1}, {("vector_similarity_weight",): 1}),
        ("vector_similarity_weight zero", {"vector_similarity_weight": 0}, {("vector_similarity_weight",): 0}),
        ("vector_similarity_weight two", {"vector_similarity_weight": 2}, {("vector_similarity_weight",): 2.0}),
        ("vector_similarity_weight negative nine", {"vector_similarity_weight": -9}, {("vector_similarity_weight",): -9.0}),
        ("vector_similarity_weight string", {"vector_similarity_weight": "a"}, {("vector_similarity_weight",): 0.0}),
        ("empty prompt parameters", {"prompt_config": {"parameters": []}}, {("prompt_config", "parameters"): []}),
        ("top_n zero", {"top_n": 0}, {("top_n",): 0}),
        ("top_n one", {"top_n": 1}, {("top_n",): 1}),
        ("top_n negative one", {"top_n": -1}, {("top_n",): -1}),
        ("top_n ten", {"top_n": 10}, {("top_n",): 10}),
        ("top_n string", {"top_n": "a"}, {("top_n",): 0}),
        ("empty_response plain text", {"prompt_config": {"empty_response": "Hello World"}}, {("prompt_config", "empty_response"): "Hello World"}),
        ("empty_response empty string", {"prompt_config": {"empty_response": ""}}, {("prompt_config", "empty_response"): ""}),
        ("empty_response punctuation", {"prompt_config": {"empty_response": "!@#$%^&*()"}}, {("prompt_config", "empty_response"): "!@#$%^&*()"}),
        ("empty_response chinese text", {"prompt_config": {"empty_response": "中文测试"}}, {("prompt_config", "empty_response"): "中文测试"}),
        ("empty_response integer", {"prompt_config": {"empty_response": 123}}, {("prompt_config", "empty_response"): 123}),
        ("empty_response boolean", {"prompt_config": {"empty_response": True}}, {("prompt_config", "empty_response"): True}),
        ("empty_response space", {"prompt_config": {"empty_response": " "}}, {("prompt_config", "empty_response"): " "}),
        ("prologue plain text", {"prompt_config": {"prologue": "Hello World"}}, {("prompt_config", "prologue"): "Hello World"}),
        ("prologue empty string", {"prompt_config": {"prologue": ""}}, {("prompt_config", "prologue"): ""}),
        ("prologue punctuation", {"prompt_config": {"prologue": "!@#$%^&*()"}}, {("prompt_config", "prologue"): "!@#$%^&*()"}),
        ("prologue chinese text", {"prompt_config": {"prologue": "中文测试"}}, {("prompt_config", "prologue"): "中文测试"}),
        ("prologue integer", {"prompt_config": {"prologue": 123}}, {("prompt_config", "prologue"): 123}),
        ("prologue boolean", {"prompt_config": {"prologue": True}}, {("prompt_config", "prologue"): True}),
        ("prologue space", {"prompt_config": {"prologue": " "}}, {("prompt_config", "prologue"): " "}),
        ("quote true", {"prompt_config": {"quote": True}}, {("prompt_config", "quote"): True}),
        ("quote false", {"prompt_config": {"quote": False}}, {("prompt_config", "quote"): False}),
        ("system prompt with knowledge prefix", {"prompt_config": {"system": "Hello World {knowledge}"}}, {("prompt_config", "system"): "Hello World {knowledge}"}),
        ("system prompt only knowledge", {"prompt_config": {"system": "{knowledge}"}}, {("prompt_config", "system"): "{knowledge}"}),
        ("system prompt punctuation", {"prompt_config": {"system": "!@#$%^&*() {knowledge}"}}, {("prompt_config", "system"): "!@#$%^&*() {knowledge}"}),
        ("system prompt chinese text", {"prompt_config": {"system": "中文测试 {knowledge}"}}, {("prompt_config", "system"): "中文测试 {knowledge}"}),
        ("system prompt plain text", {"prompt_config": {"system": "Hello World"}}, {("prompt_config", "system"): "Hello World"}),
        (
            "system prompt with explicit empty parameters",
            {"prompt_config": {"system": "Hello World", "parameters": []}},
            {("prompt_config", "system"): "Hello World", ("prompt_config", "parameters"): []},
        ),
        ("system prompt integer", {"prompt_config": {"system": 123}}, {("prompt_config", "system"): 123}),
        ("system prompt boolean", {"prompt_config": {"system": True}}, {("prompt_config", "system"): True}),
        ("unknown prompt_config key", {"prompt_config": {"unknown": "unknown"}}, {("prompt_config", "unknown"): "unknown"}),
    ]

    for index, (scenario_name, extra_payload, expected_values) in enumerate(cases, start=1):
        res = rest_client.post(
            "/chats",
            json={"name": f"restful_chat_prompt_{index}", "dataset_ids": [], **extra_payload},
        )
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 0, (scenario_name, payload)
        for path, expected_value in expected_values.items():
            assert _get_nested(payload["data"], path) == expected_value, (scenario_name, path, payload)


@pytest.mark.p2
def test_chat_create_additional_guards_contract(rest_client, clear_chats):
    cases = [
        ("reject tenant_id override", {"tenant_id": "tenant-should-not-pass"}, "`tenant_id` must not be provided."),
        ("reject unknown rerank_id", {"rerank_id": "unknown-rerank-model"}, "`rerank_id` unknown-rerank-model doesn't exist"),
    ]

    for index, (scenario_name, extra_payload, expected_message) in enumerate(cases, start=1):
        res = rest_client.post(
            "/chats",
            json={"name": f"restful_chat_guard_{index}", "dataset_ids": [], **extra_payload},
        )
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 102, (scenario_name, payload)
        assert expected_message in payload["message"], (scenario_name, payload)


@pytest.mark.p2
def test_chat_create_rejects_unparsed_document(rest_client, clear_chats, create_document):
    dataset_id, _ = create_document()
    res = rest_client.post(
        "/chats",
        json={"name": "restful_chat_unparsed_document", "dataset_ids": [dataset_id]},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert "doesn't own parsed file" in payload["message"], payload

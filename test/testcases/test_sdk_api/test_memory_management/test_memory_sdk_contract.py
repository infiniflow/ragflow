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

import json
from unittest.mock import Mock
from urllib.parse import parse_qs, urlsplit

import pytest
from ragflow_sdk import Memory, RAGFlow

CREATE_ARGUMENTS = {
    "name": "sdk-memory",
    "memory_type": ["raw", "semantic"],
    "embd_id": "BAAI/bge-small-en-v1.5@Local@Builtin",
    "llm_id": "glm-4-flash@CI@ZHIPU-AI",
}


@pytest.fixture
def sdk():
    return RAGFlow("memory-contract-token", "https://unused.invalid")


@pytest.fixture
def http(monkeypatch):
    send = Mock(return_value=Mock(json=Mock(return_value={"code": 0})))
    monkeypatch.setattr("requests.sessions.Session.send", send)
    return send


def assert_request(http, method, path):
    http.assert_called_once()
    request = http.call_args.args[0]
    url = urlsplit(request.url)
    assert (request.method, url.scheme, url.netloc, url.path) == (method, "https", "unused.invalid", f"/api/v1{path}")
    assert request.headers["Authorization"] == "Bearer memory-contract-token"
    return request


@pytest.mark.parametrize(
    "model_ids",
    [
        {"embd_id": "embedding@provider", "llm_id": "chat@provider"},
        {"embd_id": "embedding@instance@provider", "llm_id": "chat@instance@provider"},
        {"embd_id": "cda680e08a9c4e4e8c140d223fcde610", "llm_id": "cf5485e08a9c4e4e8c140d223fcde610"},
    ],
    ids=["default-instance", "named-instance", "tenant-model-ids"],
)
def test_create_payload_and_memory_response(sdk, http, model_ids):
    arguments = {**CREATE_ARGUMENTS, **model_ids}
    row = {"id": "memory-id", **arguments}
    http.return_value.json.return_value = {"code": 0, "data": row}

    memory = sdk.create_memory(**arguments)

    assert isinstance(memory, Memory)
    assert memory.rag is sdk
    for field, value in row.items():
        assert getattr(memory, field) == value
    request = assert_request(http, "POST", "/memories")
    assert json.loads(request.body) == arguments


@pytest.mark.parametrize("missing", ["name", "memory_type", "embd_id", "llm_id"])
def test_create_requires_each_argument(sdk, http, missing):
    arguments = {key: value for key, value in CREATE_ARGUMENTS.items() if key != missing}
    with pytest.raises(TypeError, match=missing):
        sdk.create_memory(**arguments)
    http.assert_not_called()


@pytest.mark.parametrize("unsupported", ["description", "storage_type"])
def test_create_rejects_unsupported_sdk_arguments(sdk, http, unsupported):
    with pytest.raises(TypeError, match=unsupported):
        sdk.create_memory(**CREATE_ARGUMENTS, **{unsupported: "value"})
    http.assert_not_called()


@pytest.mark.parametrize(
    "filters,expected_query",
    [
        ({}, {"page": ["1"], "page_size": ["50"]}),
        (
            {"page": 2, "page_size": 1, "tenant_id": "tenant-1", "memory_type": "raw", "storage_type": "table", "keywords": "sdk memory"},
            {"page": ["2"], "page_size": ["1"], "tenant_id": ["tenant-1"], "memory_type": ["raw"], "storage_type": ["table"], "keywords": ["sdk memory"]},
        ),
        (
            {"tenant_id": ["tenant-1", "tenant-2"], "memory_type": ["raw", "episodic"]},
            {"page": ["1"], "page_size": ["50"], "tenant_id": ["tenant-1", "tenant-2"], "memory_type": ["raw", "episodic"]},
        ),
    ],
    ids=["defaults", "all-filters", "repeated-filter-keys"],
)
def test_list_query_and_response_mapping(sdk, http, filters, expected_query):
    http.return_value.json.return_value = {
        "code": 0,
        "message": "success",
        "data": {"memory_list": [{"id": "memory-id", "name": "sdk-memory", "memory_type": ["raw", "semantic"]}], "total_count": 7},
    }

    result = sdk.list_memory(**filters)

    assert (result["code"], result["message"], result["total_count"]) == (0, "success", 7)
    assert len(result["memory_list"]) == 1
    memory = result["memory_list"][0]
    assert isinstance(memory, Memory)
    assert memory.rag is sdk
    assert (memory.id, memory.name, memory.memory_type) == ("memory-id", "sdk-memory", ["raw", "semantic"])
    request = assert_request(http, "GET", "/memories")
    assert parse_qs(urlsplit(request.url).query) == expected_query
    assert request.body is None


@pytest.mark.parametrize("operation", ["update", "get_config"])
def test_configuration_hydrates_server_values(sdk, http, operation):
    memory = Memory(sdk, {"id": "memory-id"})
    updates = {
        "name": "persisted-name",
        "avatar": "data:image/png;base64,aGVsbG8=",
        "description": "persisted-description",
        "llm_id": "gpt-4o@default@OpenAI",
        "permissions": "team",
        "memory_size": 1024 * 1024,
        "temperature": 0,
        "system_prompt": "Persisted system prompt.",
        "user_prompt": "Persisted user prompt.",
    }
    persisted = {
        "id": memory.id,
        "tenant_id": "tenant-id",
        "owner_name": "memory-owner",
        "memory_type": ["raw", "episodic"],
        "storage_type": "table",
        "embd_id": CREATE_ARGUMENTS["embd_id"],
        "forgetting_policy": "FIFO",
        **updates,
    }
    http.return_value.json.return_value = {"code": 0, "data": persisted}

    result = memory.update(updates) if operation == "update" else memory.get_config()

    assert result is memory
    assert memory.rag is sdk
    for field, value in persisted.items():
        assert getattr(memory, field) == value
    if operation == "update":
        request = assert_request(http, "PUT", "/memories/memory-id")
        assert json.loads(request.body) == updates
    else:
        request = assert_request(http, "GET", "/memories/memory-id/config")
        assert request.body is None


def test_delete_returns_none_and_sends_owned_id(sdk, http):
    assert sdk.delete_memory("memory-id") is None
    request = assert_request(http, "DELETE", "/memories/memory-id")
    assert json.loads(request.body) == {}


@pytest.mark.parametrize(
    "operation,method,path",
    [
        ("create", "POST", "/memories"),
        ("list", "GET", "/memories"),
        ("update", "PUT", "/memories/memory-id"),
        ("get_config", "GET", "/memories/memory-id/config"),
        ("delete", "DELETE", "/memories/memory-id"),
    ],
)
def test_api_errors_propagate(sdk, http, operation, method, path):
    http.return_value.json.return_value = {"code": 401, "message": "memory request rejected"}
    memory = Memory(sdk, {"id": "memory-id", "name": "unchanged"})
    operations = {
        "create": lambda: sdk.create_memory(**CREATE_ARGUMENTS),
        "list": sdk.list_memory,
        "update": lambda: memory.update({"name": "new-name"}),
        "get_config": memory.get_config,
        "delete": lambda: sdk.delete_memory(memory.id),
    }

    with pytest.raises(Exception, match="^memory request rejected$"):
        operations[operation]()

    assert memory.name == "unchanged"
    assert_request(http, method, path)

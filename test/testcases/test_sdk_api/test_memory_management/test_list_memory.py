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
from concurrent.futures import ThreadPoolExecutor
from uuid import uuid4

import pytest
from ragflow_sdk import Memory, RAGFlow

pytestmark = pytest.mark.integration


def ids(result):
    assert result["code"] == 0, result
    assert isinstance(result["total_count"], int), result
    assert all(isinstance(memory, Memory) for memory in result["memory_list"])
    return {memory.id for memory in result["memory_list"]}


@pytest.mark.parametrize("invalid_auth", [None, "invalid-api-token"])
def test_auth_invalid(integration_settings, invalid_auth):
    rag = RAGFlow(invalid_auth, integration_settings["base_url"])
    with pytest.raises(Exception, match="^Invalid access token$"):
        rag.list_memory()


def test_capability(client, memory_group):
    prefix, memories = memory_group
    expected = {memory.id for memory in memories}
    with ThreadPoolExecutor(max_workers=3) as executor:
        futures = [executor.submit(client.list_memory, keywords=prefix) for _ in range(6)]
        for future in futures:
            result = future.result()
            assert ids(result) == expected
            assert result["total_count"] == len(expected)


def test_params_unset(client, memory_group):
    _, memories = memory_group
    result = client.list_memory()
    assert {memory.id for memory in memories} <= ids(result)
    assert len(result["memory_list"]) <= 50
    assert result["total_count"] >= len(memories)


@pytest.mark.parametrize("page,page_size,expected_count", [(1, 10, 3), (2, 10, 0), (1, 2, 2), (2, 2, 1), (5, 10, 0)])
def test_page(client, memory_group, page, page_size, expected_count):
    prefix, memories = memory_group
    result = client.list_memory(keywords=prefix, page=page, page_size=page_size)
    returned = ids(result)
    assert len(result["memory_list"]) == len(returned) == expected_count
    assert returned <= {memory.id for memory in memories}
    assert result["total_count"] == len(memories)


def test_filter_memory_type(client, memory_group):
    prefix, memories = memory_group
    result = client.list_memory(keywords=prefix, memory_type="semantic")
    assert ids(result) == {memories[1].id}
    assert result["total_count"] == 1
    assert result["memory_list"][0].memory_type == ["raw", "semantic"]


@pytest.mark.parametrize("types", [["semantic", "procedural"], "semantic,procedural"])
def test_filter_multi_memory_type(client, memory_group, types):
    prefix, memories = memory_group
    result = client.list_memory(keywords=prefix, memory_type=types)
    assert ids(result) == {memories[1].id, memories[2].id}
    assert result["total_count"] == 2


@pytest.mark.parametrize("storage_type,expected", [("table", 3), ("unknown", 0)])
def test_filter_storage_type(client, memory_group, storage_type, expected):
    prefix, memories = memory_group
    result = client.list_memory(keywords=prefix, storage_type=storage_type)
    assert ids(result) == ({memory.id for memory in memories} if expected else set())
    assert result["total_count"] == expected
    assert all(memory.storage_type == storage_type for memory in result["memory_list"])


def test_match_keyword(client, memory_group):
    prefix, memories = memory_group
    result = client.list_memory(keywords=f"{prefix}-1")
    assert ids(result) == {memories[1].id}
    assert result["total_count"] == 1
    assert result["memory_list"][0].name == f"{prefix}-1"


def test_get_config(client, memory):
    changed = {"description": "persisted config", "memory_size": 1048576, "temperature": 0.7, "avatar": "data:image/png;base64,aGVsbG8=", "user_prompt": "User prompt"}
    memory.update(changed)
    fetched = Memory(client, {"id": memory.id}).get_config()
    assert fetched.id == memory.id
    for field, value in changed.items():
        assert getattr(fetched, field) == value
    for field in ("name", "tenant_id", "memory_type", "storage_type", "embd_id", "llm_id", "permissions", "forgetting_policy", "system_prompt"):
        assert getattr(fetched, field) == getattr(memory, field)
    payload = client.get(f"/memories/{memory.id}/config").json()
    assert payload["code"] == 0, payload
    assert fetched.owner_name == payload["data"].get("owner_name")


def test_get_config_invalid_memory_id_raises(client):
    with pytest.raises(Exception, match="^Resource not found$"):
        Memory(client, {"id": uuid4().hex}).get_config()


@pytest.mark.parametrize("multiple", [False, True])
def test_tenant_filter(client, memory_group, multiple):
    prefix, memories = memory_group
    tenant = memories[0].tenant_id
    result = client.list_memory(keywords=prefix, tenant_id=[tenant, uuid4().hex] if multiple else tenant)
    assert ids(result) == {memory.id for memory in memories}
    assert result["total_count"] == len(memories)
    assert all(memory.tenant_id == tenant for memory in result["memory_list"])
    absent = client.list_memory(keywords=prefix, tenant_id=uuid4().hex)
    assert ids(absent) == set()
    assert absent["total_count"] == 0


@pytest.mark.parametrize("params", [{"page": 0}, {"page": -1}, {"page": "bad"}, {"page_size": 0}, {"page_size": -1}, {"page_size": "bad"}])
def test_invalid_pagination_uses_defaults(client, memory_group, params):
    prefix, memories = memory_group
    result = client.list_memory(keywords=prefix, **params)
    assert ids(result) == {memory.id for memory in memories}
    assert result["total_count"] == len(memories)


def test_unknown_memory_type(client, memory_group):
    prefix, _ = memory_group
    result = client.list_memory(keywords=prefix, memory_type="unknown")
    assert ids(result) == set()
    assert result["total_count"] == 0


def test_pages_partition_owned_memories(client, memory_group):
    prefix, memories = memory_group
    first = ids(client.list_memory(keywords=prefix, page=1, page_size=2))
    second = ids(client.list_memory(keywords=prefix, page=2, page_size=2))
    assert not first & second
    assert first | second == {memory.id for memory in memories}


@pytest.mark.parametrize("invalid_auth", [None, "invalid-api-token"])
def test_get_config_unauthorized(memory, integration_settings, invalid_auth):
    rag = RAGFlow(invalid_auth, integration_settings["base_url"])
    with pytest.raises(Exception, match="^Invalid access token$"):
        Memory(rag, {"id": memory.id}).get_config()

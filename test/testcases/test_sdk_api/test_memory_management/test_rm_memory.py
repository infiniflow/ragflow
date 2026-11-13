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
from uuid import uuid4

import pytest
from ragflow_sdk import Memory, RAGFlow

pytestmark = pytest.mark.integration


@pytest.mark.parametrize("invalid_auth", [None, "invalid-api-token"])
def test_auth_invalid(client, memory, integration_settings, invalid_auth):
    rag = RAGFlow(invalid_auth, integration_settings["base_url"])
    with pytest.raises(Exception, match="^Invalid access token$"):
        rag.delete_memory(memory.id)
    assert Memory(client, {"id": memory.id}).get_config().id == memory.id


def test_memory_id(client, memory_group):
    prefix, memories = memory_group
    assert client.delete_memory(memories[0].id) is None
    result = client.list_memory(keywords=prefix)
    assert {memory.id for memory in result["memory_list"]} == {memory.id for memory in memories[1:]}
    assert result["total_count"] == 2
    with pytest.raises(Exception, match="^Resource not found$"):
        Memory(client, {"id": memories[0].id}).get_config()


@pytest.mark.parametrize("memory_id", ["not-a-uuid", None])
def test_id_wrong_uuid(client, memory_group, memory_id):
    prefix, memories = memory_group
    with pytest.raises(Exception, match="^Resource not found$"):
        client.delete_memory(memory_id)
    result = client.list_memory(keywords=prefix)
    assert {memory.id for memory in result["memory_list"]} == {memory.id for memory in memories}
    assert result["total_count"] == len(memories)


def test_nonexistent_memory(client):
    with pytest.raises(Exception, match="^Resource not found$"):
        client.delete_memory(uuid4().hex)


def test_repeated_delete(client, memory):
    client.delete_memory(memory.id)
    with pytest.raises(Exception, match="^Resource not found$"):
        client.delete_memory(memory.id)

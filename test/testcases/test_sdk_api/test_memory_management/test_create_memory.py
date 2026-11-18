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
from ragflow_sdk import RAGFlow

pytestmark = pytest.mark.integration


@pytest.mark.parametrize("invalid_auth", [None, "invalid-api-token"])
def test_auth_invalid(client, integration_settings, invalid_auth, model_ids, monkeypatch):
    rag = RAGFlow(invalid_auth, integration_settings["base_url"])
    with monkeypatch.context() as patch:
        patch.setattr(client, "authorization_header", rag.authorization_header)
        with pytest.raises(Exception, match="^Invalid access token$"):
            client.create_memory(name=f"unauthorized-{uuid4().hex}", memory_type=["raw"], **model_ids)


@pytest.mark.parametrize("name", ["a", "e" * 128, "记忆 café [test].+"])
def test_name(memory_factory, name):
    memory = memory_factory(name=name)
    assert memory.id
    assert memory.name == name
    assert memory.get_config().name == name


@pytest.mark.parametrize("name", ["", " ", "a" * 129])
def test_name_invalid(memory_factory, name):
    with pytest.raises(Exception, match="(?i)(name|required|128)"):
        memory_factory(name=name)


@pytest.mark.parametrize("memory_type", [["something"], ["raw", "something"], [], "raw", None])
def test_type_invalid(client, model_ids, memory_type):
    with pytest.raises(Exception, match="(?i)(memory.?type|memory type|required|unmarshal)"):
        client.create_memory(name=f"sdk-invalid-{uuid4().hex}", memory_type=memory_type, **model_ids)


@pytest.mark.parametrize("memory_type", [["raw"], ["raw", "semantic"], ["raw", "episodic", "procedural"], ["RAW", "raw", "semantic"]])
def test_types(memory_factory, memory_type):
    memory = memory_factory(memory_type=memory_type)
    expected = {value.lower() for value in memory_type}
    assert set(memory.memory_type) == expected
    assert set(memory.get_config().memory_type) == expected


def test_name_duplicated(memory_factory):
    name = f"sdk-memory-{uuid4().hex}"
    first = memory_factory(name=name)
    second = memory_factory(name=name)
    assert first.id != second.id
    assert first.name == name
    assert second.name == f"{name}(1)"
    assert first.get_config().name == name
    assert second.get_config().name == f"{name}(1)"


@pytest.mark.parametrize("field", ["embd_id", "llm_id"])
@pytest.mark.parametrize("value", ["", None, []])
def test_required_model_id(memory_factory, field, value):
    with pytest.raises(Exception, match="(?i)(required|unmarshal|model)"):
        memory_factory(**{field: value})


def test_create_defaults(memory_factory):
    memory = memory_factory().get_config()
    assert memory.storage_type == "table"
    assert memory.permissions == "me"
    assert memory.memory_size == 5242880
    assert memory.forgetting_policy == "FIFO"
    assert memory.temperature == 0.5
    assert memory.system_prompt

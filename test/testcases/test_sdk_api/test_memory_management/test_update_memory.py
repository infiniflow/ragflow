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
import os

import pytest
from ragflow_sdk import Memory, RAGFlow


pytestmark = pytest.mark.integration


def _assert_persisted(client, memory, changes):
    result = memory.update(changes)
    assert result is memory
    fresh = Memory(client, {"id": memory.id}).get_config()
    for field, expected in changes.items():
        assert getattr(result, field) == expected
        assert getattr(fresh, field) == expected
    return fresh


class TestAuthorization:
    @pytest.mark.parametrize("invalid_auth", [None, "invalid-sdk-memory-token"], ids=["missing", "invalid"])
    def test_auth_invalid(self, client, memory, integration_settings, invalid_auth):
        original = Memory(client, {"id": memory.id}).get_config().name
        unauthorized = RAGFlow(invalid_auth, integration_settings["base_url"])
        with pytest.raises(Exception, match="^Invalid access token$") as error:
            Memory(unauthorized, {"id": memory.id}).update({"name": "Unauthorized change"})
        assert str(error.value)
        assert Memory(client, {"id": memory.id}).get_config().name == original


class TestMemoryUpdate:
    @pytest.mark.parametrize("kind", ["ascii", "unicode", "trimmed", "max_length"])
    def test_name(self, client, memory, kind):
        name = f"renamed-{memory.id}"
        if kind == "unicode":
            name = f"记忆-{memory.id}"
        elif kind == "max_length":
            name = "a" * (128 - len(memory.id)) + memory.id
        submitted = f"  {name}  " if kind == "trimmed" else name
        result = memory.update({"name": submitted})
        assert result.name == name
        assert Memory(client, {"id": memory.id}).get_config().name == name

    @pytest.mark.parametrize(
        "name, message",
        [
            ("", "name cannot be empty"),
            (" \t\n", "name cannot be empty"),
            ("a" * 129, None),
            ("a" * 257, "name length"),
        ],
    )
    def test_name_invalid(self, client, memory, name, message):
        original = Memory(client, {"id": memory.id}).get_config().name
        with pytest.raises(Exception, match=message):
            memory.update({"name": name})
        assert Memory(client, {"id": memory.id}).get_config().name == original

    def test_duplicate_name(self, client, memory, memory_factory):
        other = memory_factory()
        original = Memory(client, {"id": memory.id}).get_config().name
        with pytest.raises(Exception, match="duplicated memory name"):
            memory.update({"name": other.name})
        assert Memory(client, {"id": memory.id}).get_config().name == original
        assert Memory(client, {"id": other.id}).get_config().name == other.name

    @pytest.mark.parametrize("avatar", ["data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=", ""])
    def test_avatar(self, client, memory, avatar):
        if not avatar:
            memory.update({"avatar": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII="})
        _assert_persisted(client, memory, {"avatar": avatar})

    @pytest.mark.parametrize("description", ["SDK memory description", ""])
    def test_description(self, client, memory, description):
        memory.update({"description": "previous description"})
        _assert_persisted(client, memory, {"description": description})

    def test_llm(self, client, memory, model_ids):
        llm_id = os.environ.get("RAGFLOW_SDK_MEMORY_UPDATE_LLM_ID")
        if not llm_id or llm_id == model_ids["llm_id"]:
            pytest.fail("Requires RAGFLOW_SDK_MEMORY_UPDATE_LLM_ID naming a second configured chat model")
        _assert_persisted(client, memory, {"llm_id": llm_id})

    @pytest.mark.parametrize("permission", ["me", "team", "all", " TEAM "])
    def test_permission(self, client, memory, permission):
        memory.update({"permissions": "team" if permission == "me" else "me"})
        result = memory.update({"permissions": permission})
        expected = permission.strip().lower()
        assert result.permissions == expected
        assert Memory(client, {"id": memory.id}).get_config().permissions == expected

    @pytest.mark.parametrize("memory_size", [1, 1048576, 5242880])
    def test_memory_size(self, client, memory, memory_size):
        _assert_persisted(client, memory, {"memory_size": memory_size})

    @pytest.mark.parametrize("temperature", [0, 0.7, 1])
    def test_temperature(self, client, memory, temperature):
        _assert_persisted(client, memory, {"temperature": temperature})

    @pytest.mark.parametrize("system_prompt", ["custom", ""])
    def test_system_prompt(self, client, memory, system_prompt):
        original = Memory(client, {"id": memory.id}).get_config().system_prompt or ""
        if system_prompt:
            system_prompt = original + "\nUse concise language when extracting memories."
        else:
            memory.update({"system_prompt": original + "\nPrevious custom instruction."})
        _assert_persisted(client, memory, {"system_prompt": system_prompt})

    @pytest.mark.parametrize("user_prompt", ["Remember these preferences.", ""])
    def test_user_prompt(self, client, memory, user_prompt):
        memory.update({"user_prompt": "previous user prompt"})
        _assert_persisted(client, memory, {"user_prompt": user_prompt})

    @pytest.mark.parametrize(
        "changes, message",
        [
            ({"permissions": ""}, "unknown permission"),
            ({"permissions": "owner"}, "unknown permission"),
            ({"memory_size": -1}, "memory size should be in range"),
            ({"memory_size": 0}, "memory size should be in range"),
            ({"memory_size": 5242881}, "memory size should be in range"),
            ({"temperature": -0.1}, "temperature should be in range"),
            ({"temperature": 1.1}, "temperature should be in range"),
            ({"forgetting_policy": "LRU"}, "forgetting policy.*not supported"),
            ({"memory_type": ["unknown"]}, "memory type.*not supported"),
            ({"temperature": "warm"}, "cannot unmarshal"),
            ({"memory_size": "large"}, "cannot unmarshal"),
        ],
    )
    def test_invalid_update(self, client, memory, changes, message):
        original = Memory(client, {"id": memory.id}).get_config()
        with pytest.raises(Exception, match=message):
            memory.update({"description": "must not persist", **changes})
        fresh = Memory(client, {"id": memory.id}).get_config()
        assert fresh.description == original.description
        for field in changes:
            assert getattr(fresh, field) == getattr(original, field)

    @pytest.mark.parametrize("policy", ["FIFO", " fifo "])
    def test_forgetting_policy(self, client, memory, policy):
        result = memory.update({"forgetting_policy": policy})
        assert result.forgetting_policy == "FIFO"
        assert Memory(client, {"id": memory.id}).get_config().forgetting_policy == "FIFO"

    def test_identity_fields_immutable(self, client, memory):
        original = Memory(client, {"id": memory.id}).get_config()
        result = memory.update(
            {
                "id": "cannot-reassign-id",
                "tenant_id": "cannot-reassign-owner",
                "storage_type": "unsupported-storage",
                "description": "supported field still updates",
            }
        )
        assert result.id == original.id
        fresh = Memory(client, {"id": original.id}).get_config()
        for field in ("id", "tenant_id", "storage_type"):
            assert getattr(result, field) == getattr(original, field)
            assert getattr(fresh, field) == getattr(original, field)
        assert fresh.description == "supported field still updates"

    def test_empty_update(self, client, memory):
        original = Memory(client, {"id": memory.id}).get_config()
        result = memory.update({})
        assert result is memory
        fresh = Memory(client, {"id": memory.id}).get_config()
        for field in ("name", "memory_type", "permissions", "memory_size", "temperature"):
            assert getattr(result, field) == getattr(original, field)
            assert getattr(fresh, field) == getattr(original, field)

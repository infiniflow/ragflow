#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import time
import uuid
import pytest
from ragflow_sdk import RAGFlow, Memory
from configs import INVALID_API_TOKEN, HOST_ADDRESS

class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "invalid_auth, expected_message",
        [
            (None, "<Unauthorized '401: Unauthorized'>"),
            (INVALID_API_TOKEN, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_auth_invalid(self, invalid_auth, expected_message):
        client = RAGFlow(invalid_auth, HOST_ADDRESS)
        with pytest.raises(Exception) as exception_info:
            client.add_message(**{
                "memory_id": [""],
                "agent_id": "",
                "session_id": "",
                "user_id": "",
                "user_input": "what is pineapple?",
                "agent_response": ""
            })
        assert str(exception_info.value) == expected_message, str(exception_info.value)


@pytest.mark.usefixtures("add_empty_raw_type_memory")
class TestAddRawMessage:

    @pytest.mark.p1
    def test_add_raw_message(self, client):
        memory_id = self.memory_id
        agent_id = uuid.uuid4().hex
        session_id = uuid.uuid4().hex
        message_payload = {
            "memory_id": [memory_id],
            "agent_id": agent_id,
            "session_id": session_id,
            "user_id": "",
            "user_input": "what is pineapple?",
            "agent_response": """
A pineapple is a tropical fruit known for its sweet, tangy flavor and distinctive, spiky appearance. Here are the key facts:
Scientific Name: Ananas comosus
Physical Description: It has a tough, spiky, diamond-patterned outer skin (rind) that is usually green, yellow, or brownish. Inside, the juicy yellow flesh surrounds a fibrous core.
Growth: Unlike most fruits, pineapples do not grow on trees. They grow from a central stem as a composite fruit, meaning they are formed from many individual berries that fuse together around the core. They grow on a short, leafy plant close to the ground.
Uses: Pineapples are eaten fresh, cooked, grilled, juiced, or canned. They are a popular ingredient in desserts, fruit salads, savory dishes (like pizzas or ham glazes), smoothies, and cocktails.
Nutrition: They are a good source of Vitamin C, manganese, and contain an enzyme called bromelain, which aids in digestion and can tenderize meat.
Symbolism: The pineapple is a traditional symbol of hospitality and welcome in many cultures.
Are you asking about the fruit itself, or its use in a specific context?
"""
        }
        add_res = client.add_message(**message_payload)
        assert add_res == "All add to task.", str(add_res)
        time.sleep(2)  # make sure refresh to index before search
        memory = Memory(client, {"id": memory_id})
        message_res = memory.list_memory_messages(**{"agent_id": agent_id, "keywords": session_id})
        assert message_res["messages"]["total_count"] > 0
        for message in message_res["messages"]["message_list"]:
            assert message["agent_id"] == agent_id, message
            assert message["session_id"] == session_id, message


@pytest.mark.usefixtures("add_empty_multiple_type_memory")
class TestAddMultipleTypeMessage:

    @pytest.mark.p1
    def test_add_multiple_type_message(self, client):
        memory_id = self.memory_id
        agent_id = uuid.uuid4().hex
        session_id = uuid.uuid4().hex
        message_payload = {
            "memory_id": [memory_id],
            "agent_id": agent_id,
            "session_id": session_id,
            "user_id": "",
            "user_input": "what is pineapple?",
            "agent_response": """
A pineapple is a tropical fruit known for its sweet, tangy flavor and distinctive, spiky appearance. Here are the key facts:
Scientific Name: Ananas comosus
Physical Description: It has a tough, spiky, diamond-patterned outer skin (rind) that is usually green, yellow, or brownish. Inside, the juicy yellow flesh surrounds a fibrous core.
Growth: Unlike most fruits, pineapples do not grow on trees. They grow from a central stem as a composite fruit, meaning they are formed from many individual berries that fuse together around the core. They grow on a short, leafy plant close to the ground.
Uses: Pineapples are eaten fresh, cooked, grilled, juiced, or canned. They are a popular ingredient in desserts, fruit salads, savory dishes (like pizzas or ham glazes), smoothies, and cocktails.
Nutrition: They are a good source of Vitamin C, manganese, and contain an enzyme called bromelain, which aids in digestion and can tenderize meat.
Symbolism: The pineapple is a traditional symbol of hospitality and welcome in many cultures.
Are you asking about the fruit itself, or its use in a specific context?
"""
        }
        add_res = client.add_message(**message_payload)
        assert add_res == "All add to task.", str(add_res)
        time.sleep(2)  # make sure refresh to index before search
        memory = Memory(client, {"id": memory_id})
        message_res = memory.list_memory_messages(**{"agent_id": agent_id, "keywords": session_id})
        assert message_res["messages"]["total_count"] > 0
        for message in message_res["messages"]["message_list"]:
            assert message["agent_id"] == agent_id, message
            assert message["session_id"] == session_id, message


@pytest.mark.usefixtures("add_2_multiple_type_memory")
class TestAddToMultipleMemory:

    @pytest.mark.p1
    def test_add_to_multiple_memory(self, client):
        memory_ids = self.memory_ids
        agent_id = uuid.uuid4().hex
        session_id = uuid.uuid4().hex
        message_payload = {
            "memory_id": memory_ids,
            "agent_id": agent_id,
            "session_id": session_id,
            "user_id": "",
            "user_input": "what is pineapple?",
            "agent_response": """
A pineapple is a tropical fruit known for its sweet, tangy flavor and distinctive, spiky appearance. Here are the key facts:
Scientific Name: Ananas comosus
Physical Description: It has a tough, spiky, diamond-patterned outer skin (rind) that is usually green, yellow, or brownish. Inside, the juicy yellow flesh surrounds a fibrous core.
Growth: Unlike most fruits, pineapples do not grow on trees. They grow from a central stem as a composite fruit, meaning they are formed from many individual berries that fuse together around the core. They grow on a short, leafy plant close to the ground.
Uses: Pineapples are eaten fresh, cooked, grilled, juiced, or canned. They are a popular ingredient in desserts, fruit salads, savory dishes (like pizzas or ham glazes), smoothies, and cocktails.
Nutrition: They are a good source of Vitamin C, manganese, and contain an enzyme called bromelain, which aids in digestion and can tenderize meat.
Symbolism: The pineapple is a traditional symbol of hospitality and welcome in many cultures.
Are you asking about the fruit itself, or its use in a specific context?
"""
        }
        add_res = client.add_message(**message_payload)
        assert add_res == "All add to task.", str(add_res)
        time.sleep(2)  # make sure refresh to index before search
        for memory_id in memory_ids:
            memory = Memory(client, {"id": memory_id})
            message_res = memory.list_memory_messages(**{"agent_id": agent_id, "keywords": session_id})
            assert message_res["messages"]["total_count"] > 0
            for message in message_res["messages"]["message_list"]:
                assert message["agent_id"] == agent_id, message
                assert message["session_id"] == session_id, message

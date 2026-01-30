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

from test_web_api.common import list_memory_message, add_message
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth

class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_auth_invalid(self, invalid_auth, expected_code, expected_message):
        res = add_message(invalid_auth, {})
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


@pytest.mark.usefixtures("add_empty_raw_type_memory")
class TestAddRawMessage:

    @pytest.mark.p1
    def test_add_raw_message(self, WebApiAuth):
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
        add_res = add_message(WebApiAuth, message_payload)
        assert add_res["code"] == 0, add_res
        time.sleep(2)  # make sure refresh to index before search
        message_res = list_memory_message(WebApiAuth, memory_id, params={"agent_id": agent_id, "keywords": session_id})
        assert message_res["code"] == 0, message_res
        assert message_res["data"]["messages"]["total_count"] > 0
        for message in message_res["data"]["messages"]["message_list"]:
            assert message["agent_id"] == agent_id, message
            assert message["session_id"] == session_id, message


@pytest.mark.usefixtures("add_empty_multiple_type_memory")
class TestAddMultipleTypeMessage:

    @pytest.mark.p1
    def test_add_multiple_type_message(self, WebApiAuth):
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
        add_res = add_message(WebApiAuth, message_payload)
        assert add_res["code"] == 0, add_res
        time.sleep(2)  # make sure refresh to index before search
        message_res = list_memory_message(WebApiAuth, memory_id, params={"agent_id": agent_id, "keywords": session_id})
        assert message_res["code"] == 0, message_res
        assert message_res["data"]["messages"]["total_count"] > 0
        for message in message_res["data"]["messages"]["message_list"]:
            assert message["agent_id"] == agent_id, message
            assert message["session_id"] == session_id, message


@pytest.mark.usefixtures("add_2_multiple_type_memory")
class TestAddToMultipleMemory:

    @pytest.mark.p1
    def test_add_to_multiple_memory(self, WebApiAuth):
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
        add_res = add_message(WebApiAuth, message_payload)
        assert add_res["code"] == 0, add_res
        time.sleep(2)  # make sure refresh to index before search
        for memory_id in memory_ids:
            message_res = list_memory_message(WebApiAuth, memory_id, params={"agent_id": agent_id, "keywords": session_id})
            assert message_res["code"] == 0, message_res
            assert message_res["data"]["messages"]["total_count"] > 0
            for message in message_res["data"]["messages"]["message_list"]:
                assert message["agent_id"] == agent_id, message
                assert message["session_id"] == session_id, message

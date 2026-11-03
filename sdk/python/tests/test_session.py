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

from unittest.mock import MagicMock

from ragflow_sdk.modules.session import Session


class MockRAG:
    def post(self, *args, **kwargs):
        pass


def test_structure_answer_agent_non_streaming():
    """Verify non-streaming agent responses do not fail with KeyError: 'data'."""
    session = Session(MockRAG(), {"id": "sess_agent_1", "agent_id": "ag_1", "name": "Agent 1"})

    # Non-streaming Session.ask passes the already unwrapped json_data["data"] payload
    non_streaming_data = {"content": "This is a non-streaming agent answer.", "id": "msg_agent_123", "reference": {"chunks": [{"id": "chk_1", "content": "Knowledge reference 1"}]}}

    message = session._structure_answer(non_streaming_data)
    assert message.content == "This is a non-streaming agent answer."
    assert message.id == "msg_agent_123"
    assert message.role == "assistant"
    assert len(message.reference) == 1
    assert message.reference[0]["id"] == "chk_1"


def test_structure_answer_agent_streaming():
    """Verify streaming agent SSE messages unwrap the 'data' field properly."""
    session = Session(MockRAG(), {"id": "sess_agent_2", "agent_id": "ag_2", "name": "Agent 2"})

    # Streaming mode passes the outer SSE event dictionary
    streaming_event = {"event": "message", "message_id": "msg_sse_456", "data": {"content": "Streaming token delta", "reference": {"chunks": [{"id": "chk_2", "content": "Knowledge reference 2"}]}}}

    message = session._structure_answer(streaming_event)
    assert message.content == "Streaming token delta"
    assert message.id == "msg_sse_456"
    assert message.role == "assistant"
    assert len(message.reference) == 1


def test_structure_answer_agent_dict_chunks():
    """Verify agent responses with dictionary-encoded chunk mapping are normalized."""
    session = Session(MockRAG(), {"id": "sess_agent_chunks", "agent_id": "ag_chunks", "name": "Agent Chunks"})

    payload = {
        "content": "Answer with dict chunks",
        "message_id": "msg_chunk_dict",
        "data": {"content": "Answer with dict chunks", "reference": {"chunks": {"0": {"id": "chk_map_1", "content": "Map item 1"}, "1": {"id": "chk_map_2", "content": "Map item 2"}}}},
    }

    message = session._structure_answer(payload)
    assert message.content == "Answer with dict chunks"
    assert len(message.reference) == 2
    assert message.reference[0]["id"] == "chk_map_1"
    assert message.reference[1]["id"] == "chk_map_2"


def test_structure_answer_agent_fallback_id():
    """Verify fallback message ID resolution when message_id key is absent."""
    session = Session(MockRAG(), {"id": "sess_agent_id", "agent_id": "ag_id", "name": "Agent ID"})

    payload_with_id = {"content": "Fallback ID test", "id": "msg_fallback_id", "reference": {}}

    message = session._structure_answer(payload_with_id)
    assert message.content == "Fallback ID test"
    assert message.id == "msg_fallback_id"


def test_structure_answer_chat():
    """Verify chat sessions structure answer from 'answer' field."""
    session = Session(MockRAG(), {"id": "sess_chat_1", "chat_id": "chat_1", "name": "Chat 1"})

    chat_data = {"answer": "This is a direct chat answer.", "id": "msg_chat_789", "reference": {}}

    message = session._structure_answer(chat_data)
    assert message.content == "This is a direct chat answer."
    assert message.id == "msg_chat_789"
    assert message.role == "assistant"


def test_ask_agent_non_streaming_end_to_end():
    """Verify end-to-end Session.ask with stream=False on an agent session."""
    mock_rag = MockRAG()
    mock_response = MagicMock()
    mock_response.json.return_value = {
        "code": 0,
        "data": {
            "event": "message",
            "message_id": "msg_agent_end_to_end",
            "data": {
                "content": "Final workflow conclusion.",
                "reference": {"chunks": [{"id": "chk_3", "content": "Evidence doc"}]},
            },
        },
    }
    mock_rag.post = MagicMock(return_value=mock_response)

    session = Session(mock_rag, {"id": "sess_agent_3", "agent_id": "ag_3", "name": "Agent 3"})
    answers = list(session.ask("Analyze data", stream=False))

    assert len(answers) == 1
    assert answers[0].content == "Final workflow conclusion."
    assert answers[0].id == "msg_agent_end_to_end"
    assert len(answers[0].reference) == 1
    assert answers[0].reference[0]["id"] == "chk_3"

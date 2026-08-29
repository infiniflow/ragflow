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

from types import SimpleNamespace
from unittest.mock import AsyncMock, Mock

import pytest

from api.db.joint_services import memory_message_service
from common.constants import MemoryType


def make_memory():
    return SimpleNamespace(
        id="memory-1",
        tenant_id="tenant-1",
        tenant_llm_id="tenant-llm-1",
        temperature=0.2,
        memory_type=MemoryType.SEMANTIC.value,
        llm_id="model-1",
        system_prompt="Custom system prompt",
        user_prompt="Custom user prompt",
    )


def make_message():
    return {
        "user_id": "user-1",
        "agent_id": "agent-1",
        "session_id": "session-1",
        "user_input": "Remember this",
        "agent_response": "I will remember it",
    }


def assert_custom_prompts_forwarded(extract_by_llm):
    extract_by_llm.assert_awaited_once()
    assert extract_by_llm.await_args.kwargs["system_prompt"] == "Custom system prompt"
    assert extract_by_llm.await_args.kwargs["user_prompt"] == "Custom user prompt"


@pytest.mark.p1
@pytest.mark.parametrize(
    ("keywords_similarity_weight", "expected"),
    [(0.0, True), (0.5, True), (0.5001, False), (0.9, False), (1.0, False)],
)
def test_query_message_limits_dense_fallback_to_semantic_dominant_searches(monkeypatch, keywords_similarity_weight, expected):
    memory = SimpleNamespace(id="memory-1", tenant_id="tenant-1", embd_id="embedding-1")
    search_message = Mock(return_value=[])

    monkeypatch.setattr(memory_message_service.MemoryService, "get_by_ids", lambda _memory_ids: [memory])
    monkeypatch.setattr(memory_message_service, "resolve_model_config", lambda *_args: SimpleNamespace())
    monkeypatch.setattr(memory_message_service, "LLMBundle", lambda *_args: object())
    monkeypatch.setattr(memory_message_service, "get_vector", lambda *_args, **_kwargs: object())
    monkeypatch.setattr(memory_message_service, "MsgTextQuery", lambda: SimpleNamespace(question=lambda *_args, **_kwargs: (object(), None)))
    monkeypatch.setattr(memory_message_service.MemoryService, "search_message", search_message)

    result = memory_message_service.query_message(
        {"memory_id": ["memory-1"]},
        {
            "query": "needle",
            "similarity_threshold": 0.2,
            "keywords_similarity_weight": keywords_similarity_weight,
            "top_n": 5,
        },
    )

    assert result == []
    assert search_message.call_args.kwargs["allow_dense_fallback"] is expected


@pytest.mark.p1
async def test_save_to_memory_forwards_custom_extraction_prompts(monkeypatch):
    memory = make_memory()
    extract_by_llm = AsyncMock(return_value=[])

    monkeypatch.setattr(memory_message_service.MemoryService, "get_by_memory_id", lambda _memory_id: memory)
    monkeypatch.setattr(memory_message_service, "extract_by_llm", extract_by_llm)
    monkeypatch.setattr(memory_message_service.REDIS_CONN, "generate_auto_increment_id", lambda **_kwargs: 1)
    monkeypatch.setattr(memory_message_service, "embed_and_save", AsyncMock(return_value=(True, "Message saved successfully.")))

    result = await memory_message_service.save_to_memory(memory.id, make_message())

    assert result == (True, "Message saved successfully.")
    assert_custom_prompts_forwarded(extract_by_llm)


@pytest.mark.p1
async def test_save_extracted_to_memory_only_forwards_custom_extraction_prompts(monkeypatch):
    memory = make_memory()
    extract_by_llm = AsyncMock(return_value=[])

    monkeypatch.setattr(memory_message_service.MemoryService, "get_by_memory_id", lambda _memory_id: memory)
    monkeypatch.setattr(memory_message_service, "extract_by_llm", extract_by_llm)

    result = await memory_message_service.save_extracted_to_memory_only(memory.id, make_message(), source_message_id=1)

    assert result == (True, "No memory extracted from raw message.")
    assert_custom_prompts_forwarded(extract_by_llm)

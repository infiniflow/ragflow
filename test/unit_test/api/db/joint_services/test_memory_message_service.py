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
from unittest.mock import AsyncMock

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


@pytest.mark.p1
def test_query_message_puts_the_keyword_weight_in_the_text_slot(monkeypatch):
    """FusionExpr weights are "text,vector", so keywords_similarity_weight belongs first.

    The adapters behind msgStoreConn all read slot 1 as the vector weight, so sending
    the keyword weight there inverted hybrid ranking for every memory search.
    """
    captured = {}

    def fake_search_message(_memory_ids, _condition_dict, _uid_list, match_expressions, _top_n):
        captured["match_expressions"] = match_expressions
        return []

    memory = SimpleNamespace(id="memory-1", tenant_id="tenant-1", embd_id="embd-1")
    monkeypatch.setattr(memory_message_service.MemoryService, "get_by_ids", lambda _ids: [memory])
    monkeypatch.setattr(memory_message_service, "resolve_model_config", lambda *_a, **_k: {})
    monkeypatch.setattr(memory_message_service, "LLMBundle", lambda *_a, **_k: object())
    monkeypatch.setattr(memory_message_service, "get_vector", lambda *_a, **_k: SimpleNamespace())
    monkeypatch.setattr(memory_message_service, "MsgTextQuery", lambda: SimpleNamespace(question=lambda *_a, **_k: (SimpleNamespace(), None)))
    monkeypatch.setattr(memory_message_service.MessageService, "search_message", fake_search_message)

    memory_message_service.query_message(
        {"memory_id": ["memory-1"]},
        {"query": "hello", "similarity_threshold": 0.2, "keywords_similarity_weight": 0.7, "top_n": 5},
    )

    fusion_expr = captured["match_expressions"][2]
    text_weight, vector_weight = (float(part) for part in fusion_expr.fusion_params["weights"].split(","))

    assert text_weight == pytest.approx(0.7)
    assert vector_weight == pytest.approx(0.3)

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

from contextlib import nullcontext
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest

from api.db.joint_services import memory_message_service


@pytest.mark.p1
async def test_extract_by_llm_replaces_invalid_timestamps(monkeypatch):
    llm = SimpleNamespace(async_chat=AsyncMock(return_value="response"))
    extracted = {
        "semantic": [
            {
                "content": "A timeless fact",
                "valid_at": "not-a-date",
                "invalid_at": "also-not-a-date",
            }
        ]
    }

    monkeypatch.setattr(memory_message_service, "current_timestamp", lambda: 1)
    monkeypatch.setattr(memory_message_service, "timestamp_to_date", lambda _timestamp: "2026-08-18 12:00:00")
    monkeypatch.setattr(memory_message_service, "resolve_model_config", lambda *_args: SimpleNamespace())
    monkeypatch.setattr(memory_message_service, "LLMBundle", lambda *_args: nullcontext(llm))
    monkeypatch.setattr(memory_message_service, "get_json_result_from_llm_response", lambda _response: extracted)

    result = await memory_message_service.extract_by_llm(
        tenant_id="tenant-1",
        tenant_llm_id=None,
        extract_conf={},
        memory_type=["semantic"],
        user_input="Remember this",
        agent_response="I will remember it",
        llm_id="model-1",
    )

    assert result[0]["valid_at"] == "2026-08-18 12:00:00"
    assert result[0]["invalid_at"] == ""

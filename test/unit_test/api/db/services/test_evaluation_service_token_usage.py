#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use it except in compliance with the License.
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

"""Unit tests for token_usage tracking in EvaluationService._evaluate_single_case."""

import pytest
from types import SimpleNamespace

from api.db.services.evaluation_service import EvaluationService


@pytest.fixture
def mock_dialog():
    """Minimal dialog object for evaluation."""
    return SimpleNamespace(
        kb_ids=["kb-1"],
        prompt_config={"quote": True},
        llm_id="test-llm",
        tenant_id="tenant-1",
    )


@pytest.fixture
def minimal_case():
    """Minimal test case for evaluation."""
    return {
        "id": "case-1",
        "question": "What is the capital of France?",
        "reference_answer": None,
        "relevant_chunk_ids": None,
    }


@pytest.mark.p2
def test_token_usage_structure_when_prompt_available(monkeypatch, mock_dialog, minimal_case):
    """Verify token_usage dict has correct structure when ans contains 'prompt'."""
    captured_create = {}

    async def mock_async_chat(dialog, messages, stream, **kwargs):
        # Simulate async_chat yielding one result with full prompt
        yield {
            "answer": "Paris is the capital of France.",
            "reference": {"chunks": []},
            "prompt": "System instructions here.\n\nKnowledge: Some context.\n\nQuery: What is the capital of France?",
        }

    def capture_create(**kwargs):
        captured_create.update(kwargs)

    monkeypatch.setattr(
        "api.db.services.dialog_service.async_chat",
        mock_async_chat,
    )
    monkeypatch.setattr(
        "api.db.services.evaluation_service.EvaluationResult.create",
        capture_create,
    )

    result = EvaluationService._evaluate_single_case("run-1", minimal_case, mock_dialog)

    assert result is not None
    assert "token_usage" in captured_create
    token_usage = captured_create["token_usage"]
    assert "prompt_tokens" in token_usage
    assert "completion_tokens" in token_usage
    assert "total_tokens" in token_usage
    assert token_usage["total_tokens"] == token_usage["prompt_tokens"] + token_usage["completion_tokens"]
    assert token_usage["prompt_tokens"] > 0
    assert token_usage["completion_tokens"] > 0


@pytest.mark.p2
def test_token_usage_fallback_when_prompt_missing(monkeypatch, mock_dialog, minimal_case):
    """Verify fallback to question-only count when ans has no 'prompt' key."""
    captured_create = {}

    async def mock_async_chat_no_prompt(dialog, messages, stream, **kwargs):
        # Simulate response without 'prompt' (e.g. async_chat_solo)
        yield {
            "answer": "Paris.",
            "reference": {"chunks": []},
        }

    def capture_create(**kwargs):
        captured_create.update(kwargs)

    monkeypatch.setattr(
        "api.db.services.dialog_service.async_chat",
        mock_async_chat_no_prompt,
    )
    monkeypatch.setattr(
        "api.db.services.evaluation_service.EvaluationResult.create",
        capture_create,
    )

    result = EvaluationService._evaluate_single_case("run-1", minimal_case, mock_dialog)

    assert result is not None
    assert "token_usage" in captured_create
    token_usage = captured_create["token_usage"]
    assert "prompt_tokens" in token_usage
    assert "completion_tokens" in token_usage
    assert "total_tokens" in token_usage
    assert token_usage["total_tokens"] == token_usage["prompt_tokens"] + token_usage["completion_tokens"]
    # With fallback, prompt_tokens should reflect question only (smaller than full prompt)
    assert token_usage["prompt_tokens"] >= 0
    assert token_usage["completion_tokens"] > 0


@pytest.mark.p2
def test_token_usage_no_answer_logs_warning(monkeypatch, mock_dialog, minimal_case, caplog):
    """When chat yields no answers, we still record token_usage and log a warning."""
    captured_create = {}

    async def mock_async_chat_empty(dialog, messages, stream, **kwargs):
        # Simulate async_chat that yields no items at all
        if False:
            yield {}

    def capture_create(**kwargs):
        captured_create.update(kwargs)

    monkeypatch.setattr(
        "api.db.services.dialog_service.async_chat",
        mock_async_chat_empty,
    )
    monkeypatch.setattr(
        "api.db.services.evaluation_service.EvaluationResult.create",
        capture_create,
    )

    with caplog.at_level("WARNING"):
        result = EvaluationService._evaluate_single_case("run-1", minimal_case, mock_dialog)

    assert result is not None
    token_usage = captured_create["token_usage"]
    # No answer tokens in this case
    assert token_usage["completion_tokens"] == 0
    assert token_usage["prompt_tokens"] >= 0
    assert token_usage["total_tokens"] == token_usage["prompt_tokens"]
    assert any("produced no answer from chat" in msg for msg in caplog.messages)


@pytest.mark.p2
def test_compute_summary_metrics_aggregates_metrics():
    """_compute_summary_metrics should average numeric metrics correctly."""
    results = [
        {"execution_time": 1.0, "metrics": {"precision": 0.5, "answer_length": 10}},
        {"execution_time": 3.0, "metrics": {"precision": 1.0, "answer_length": 20}},
    ]

    summary = EvaluationService._compute_summary_metrics(results)

    assert summary["total_cases"] == 2
    assert summary["avg_execution_time"] == pytest.approx(2.0)
    assert summary["avg_precision"] == pytest.approx(0.75)
    assert summary["avg_answer_length"] == pytest.approx(15.0)

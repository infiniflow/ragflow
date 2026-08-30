from pathlib import Path
from types import SimpleNamespace

import pytest

from rag.advanced_rag.harness.action_session import _exec_calculate


@pytest.mark.asyncio
async def test_exec_calculate_returns_computed_value(monkeypatch):
    async def fake_compute_from_facts(model, question, facts):
        assert model is tools.chat_mdl
        assert question == "What is the total?"
        assert facts == ["First value: 2", "Second value: 3"]
        return {
            "needed": True,
            "label": "Total",
            "value": "5",
            "expression": "2 + 3",
            "uses": [0, 1],
        }

    monkeypatch.setattr("rag.advanced_rag.harness.arithmetic.compute_from_facts", fake_compute_from_facts)
    tools = SimpleNamespace(chat_mdl=object())

    results, evidence_ids = await _exec_calculate(
        tools,
        {
            "question": "What is the total?",
            "facts": ["First value: 2", "Second value: 3"],
        },
    )

    assert results == [{"kind": "calculate", "expression": "2 + 3", "result": "5"}]
    assert evidence_ids == []


def test_initialize_state_prompt_uses_runtime_answer_variable():
    prompt_path = Path(__file__).resolve().parents[4] / "rag" / "prompts" / "action_initialize_state.md"
    prompt = prompt_path.read_text(encoding="utf-8")

    assert '"answer_variable"' in prompt
    assert '"answer_slot"' not in prompt

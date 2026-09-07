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

    oc = await _exec_calculate(
        tools,
        {
            "question": "What is the total?",
            "facts": ["First value: 2", "Second value: 3"],
        },
    )

    assert oc.status == "ok"
    assert oc.payload == [{"kind": "calculate", "expression": "2 + 3", "result": "5"}]
    assert oc.evidence_ids == []

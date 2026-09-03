import sys
from types import ModuleType, SimpleNamespace

import pytest

from rag.advanced_rag.harness.structure_qa import _ask_structure


@pytest.mark.asyncio
async def test_ask_structure_normalizes_non_string_answer(monkeypatch):
    generator = ModuleType("rag.prompts.generator")
    generator.form_message = lambda system, user: [
        {"role": "system", "content": system},
        {"role": "user", "content": user},
    ]
    generator.message_fit_in = lambda messages, _budget: (0, messages)

    prompts = ModuleType("rag.prompts")
    prompts.__path__ = []
    prompts.generator = generator
    monkeypatch.setitem(sys.modules, "rag.prompts", prompts)
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", generator)

    class FakeChatModel:
        max_length = 8192

        async def async_chat(self, system, messages, config):
            assert system
            assert messages
            assert config == {"temperature": 0.2}
            return '{"is_sufficient": true, "answer": 42, "relevant_entities": ["Revenue", 7]}'

    tools = SimpleNamespace(chat_mdl=FakeChatModel())

    answer, relevant = await _ask_structure(
        tools,
        "What is the revenue?",
        [{"name": "Revenue", "type": "metric", "description": "Annual revenue is 42."}],
        [],
        "outline",
        "Structure test",
    )

    assert answer == "42"
    assert relevant == ["Revenue"]

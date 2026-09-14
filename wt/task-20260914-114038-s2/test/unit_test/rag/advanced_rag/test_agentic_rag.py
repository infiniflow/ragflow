import pytest

from rag.advanced_rag.agentic_rag import RAGTools


class FakeChatModel:
    max_length = 8192

    def clone(self):
        return self


@pytest.mark.asyncio
async def test_rag_tool_adds_text_attachment_to_user_question(monkeypatch):
    captured = {}

    async def fake_run_agentic_rag(tools, messages):
        captured["messages"] = messages
        yield "answer"

    monkeypatch.setattr("rag.advanced_rag.agentic_rag_graph.run_agentic_rag", fake_run_agentic_rag)

    tools = RAGTools([], FakeChatModel(), text_attachments_content="attached facts")

    assert await tools.rag("What is attached?") == "answer"
    assert captured["messages"] == [{"role": "user", "content": "What is attached?attached facts"}]

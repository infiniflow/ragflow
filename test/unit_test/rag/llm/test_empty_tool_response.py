import asyncio
from types import SimpleNamespace

import httpx
import pytest
from openai import AsyncOpenAI

from rag.llm.chat_model import Base


@pytest.mark.parametrize("content", [None, "", "   "])
@pytest.mark.parametrize("finish_reason", ["stop", "length"])
@pytest.mark.parametrize("max_retries", [0, 2])
def test_empty_final_answer_is_explicit_model_error(content, finish_reason, max_retries):
    answer, tokens, requests = asyncio.run(_run_tool_rounds(content, finish_reason, max_retries=max_retries))
    assert answer.startswith("**ERROR**: MODEL_ERROR")
    assert "empty" in answer.lower()
    assert finish_reason in answer
    assert tokens == 6
    assert len(requests) == 2


@pytest.mark.parametrize("reasoning_field", ["reasoning_content", "reasoning"])
def test_tool_call_with_text_preserves_reasoning_and_length_notice(reasoning_field):
    answer, tokens, requests = asyncio.run(
        _run_tool_rounds("Found a document.", "length", reasoning_field=reasoning_field, reasoning="Reasoning.")
    )
    assert answer == "<think>Reasoning.</think>Found a document....\nThe answer is truncated by your chosen LLM due to its limitation on context length."
    assert "**ERROR**" not in answer
    assert tokens == 6
    assert len(requests) == 2


@pytest.mark.parametrize("content", [None, "", "   "])
def test_empty_final_answer_preserves_accumulated_verbose_tool_output(content):
    answer, tokens, requests = asyncio.run(
        _run_tool_rounds(
            content,
            "stop",
            reasoning_field="reasoning_content",
            reasoning="Reasoning.",
            verbose_tool_use=True,
        )
    )

    assert answer == (
        '<tool_call>{\n  "name": "search",\n  "args": {},\n  "result": "Synthetic document."\n}'
        "</tool_call><think>Reasoning.</think>"
    )
    assert tokens == 6
    assert len(requests) == 2


async def _run_tool_rounds(content, finish_reason, *, max_retries=0, reasoning_field=None, reasoning=None, verbose_tool_use=False):
    requests = []

    def respond(request):
        requests.append(request)
        if len(requests) == 1:
            message = {"role": "assistant", "content": None, "tool_calls": [{"id": "call-test", "type": "function", "function": {"name": "search", "arguments": "{}"}}]}
            reason = "tool_calls"
        else:
            message = {"role": "assistant", "content": content}
            if reasoning_field:
                message[reasoning_field] = reasoning
            reason = finish_reason
        return httpx.Response(200, json={"id": "test-completion", "object": "chat.completion", "created": 0, "model": "test-model", "choices": [{"index": 0, "message": message, "finish_reason": reason}], "usage": {"prompt_tokens": 2, "completion_tokens": 1, "total_tokens": 3}})

    async def search(name, arguments):
        assert name == "search" and arguments == {}
        return "Synthetic document."

    model = Base.__new__(Base)
    model.model_name = "test-model"
    model.max_retries = max_retries
    model.max_rounds = 5
    model.tools = [{"type": "function", "function": {"name": "search", "parameters": {"type": "object"}}}]
    model.toolcall_session = SimpleNamespace(tool_call_async=search)
    if not verbose_tool_use:
        model._verbose_tool_use = lambda *_args: ""
    async with AsyncOpenAI(api_key="offline-placeholder", base_url="https://offline.invalid/v1", http_client=httpx.AsyncClient(transport=httpx.MockTransport(respond), trust_env=False)) as client:
        model.async_client = client
        answer, tokens = await model.async_chat_with_tools("Use the supplied document.", [{"role": "user", "content": "Find it."}])
        return answer, tokens, requests

import asyncio
from functools import partial
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest

from agent.component.message import Message, MessageParam


async def _collect_stream(message: Message, template: str) -> str:
    return "".join([chunk async for chunk in message._stream(template)])


def _build_message(source_stream, source):
    message = object.__new__(Message)
    message._canvas = SimpleNamespace(
        get_variable_value=lambda _: partial(source_stream),
        get_component_obj=lambda _: source,
        is_canceled=lambda: False,
        task_id="test-task",
    )
    message._param = MessageParam()
    message._save_to_memory = AsyncMock()
    return message


@pytest.mark.p1
def test_stream_propagates_deferred_source_error():
    source = SimpleNamespace(error_value=None)
    source.error = lambda: source.error_value

    async def source_stream():
        yield "partial"
        source.error_value = "provider stream failed"

    message = _build_message(source_stream, source)

    content = asyncio.run(_collect_stream(message, "{llm@content}"))

    assert content == "partial"
    assert message.error() == "provider stream failed"
    assert message.output("content") == ""
    assert message.get_input_value("llm@content") is None
    message._save_to_memory.assert_not_awaited()


@pytest.mark.p1
def test_stream_finalizes_successful_deferred_source():
    source = SimpleNamespace(error=lambda: None)

    async def source_stream():
        yield "complete"

    message = _build_message(source_stream, source)

    content = asyncio.run(_collect_stream(message, "{llm@content}"))

    assert content == "complete"
    assert message.error() is None
    assert message.output("content") == "complete"
    assert message.get_input_value("llm@content") == "complete"
    message._save_to_memory.assert_awaited_once_with("complete")

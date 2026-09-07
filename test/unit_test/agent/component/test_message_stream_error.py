import asyncio
import logging
import time
from functools import partial
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest

from agent.canvas import Canvas
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
def test_stream_propagates_deferred_source_error(caplog):
    source = SimpleNamespace(error_value=None)
    source.error = lambda: source.error_value

    async def source_stream():
        yield "partial"
        source.error_value = "provider stream failed"

    message = _build_message(source_stream, source)

    with caplog.at_level(logging.WARNING):
        content = asyncio.run(_collect_stream(message, "{llm@content}"))

    assert content == "partial"
    assert message.error() == "provider stream failed"
    assert message.output("content") == ""
    assert message.get_input_value("llm@content") is None
    message._save_to_memory.assert_not_awaited()
    assert "source component error: llm" in caplog.text
    assert "provider stream failed" not in caplog.text


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


def _build_canvas_component(component_id, component_name, invoke_async):
    outputs = {}
    component = SimpleNamespace()

    async def _invoke_async(**kwargs):
        outputs["_created_time"] = time.perf_counter()
        await invoke_async(component, **kwargs)

    component._id = component_id
    component._param = SimpleNamespace(layout_recognize=None)
    component.component_name = component_name
    component._invoke_async = _invoke_async
    component.invoke_async = _invoke_async
    component.invoke = lambda **_kwargs: None
    component.reset = lambda only_output=False: None
    component.get_input_elements = dict
    component.get_input = dict
    component.get_input_values = dict
    component.output = lambda key=None: outputs.get(key) if key else dict(outputs)
    component.set_output = lambda key, value: outputs.__setitem__(key, value)
    component.error = lambda: outputs.get("_ERROR")
    component.exception_handler = lambda: None
    component.get_param = lambda _key: False
    component.get_parent = lambda: None
    component.thoughts = lambda: ""
    return component


def _build_canvas(invoke_message):
    async def invoke_begin(_component, **_kwargs):
        return None

    begin = _build_canvas_component("begin", "Begin", invoke_begin)
    message = _build_canvas_component("message", "Message", invoke_message)
    canvas = Canvas.__new__(Canvas)
    canvas.__dict__.update(
        _thread_pool=SimpleNamespace(_max_workers=1),
        _run_token_usage={"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0, "calls": 0},
        task_id="task",
        path=[],
        error="",
        history=[],
        retrieval=[],
        globals={"sys.history": [], "sys.conversation_turns": 0, "sys.date": ""},
        components={
            "begin": {"obj": begin, "downstream": ["message"], "upstream": []},
            "message": {"obj": message, "downstream": [], "upstream": ["begin"]},
        },
    )
    canvas.is_canceled = lambda: False
    canvas.get_component_name = lambda component_id: component_id
    return canvas, message


@pytest.mark.p1
@pytest.mark.parametrize(
    ("chunk", "source_error", "completion_count"),
    [
        pytest.param("partial", "provider stream failed", 0, id="failed"),
        pytest.param("complete", None, 1, id="successful"),
    ],
)
def test_canvas_message_completion_follows_stream_error(caplog, chunk, source_error, completion_count):
    async def invoke_message(component, **_kwargs):
        async def stream():
            yield chunk
            if source_error:
                component.set_output("_ERROR", source_error)

        component.set_output("content", partial(stream))

    canvas, message = _build_canvas(invoke_message)

    with caplog.at_level(logging.DEBUG):
        events = asyncio.run(_collect_canvas_events(canvas))

    assert [event["data"]["content"] for event in events if event["event"] == "message"] == [chunk]
    assert sum(event["event"] == "message_end" for event in events) == completion_count
    assert sum(event["event"] == "workflow_finished" for event in events) == completion_count
    message_finished = next(event for event in events if event["event"] == "node_finished" and event["data"]["component_id"] == "message")
    assert message_finished["data"]["error"] == source_error
    assert message.output("content") == (None if source_error else chunk)
    assert canvas.error == ("Component execution failed: message" if source_error else "")
    if source_error:
        assert "Runtime Error: Component execution failed: message" in caplog.text
        assert source_error not in caplog.text


async def _collect_canvas_events(canvas):
    return [event async for event in canvas._run_impl(query="hello", inputs={})]

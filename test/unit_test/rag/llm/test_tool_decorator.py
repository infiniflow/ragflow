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
"""Smoke tests for the ``@tool`` decorator and ``FunctionToolSession`` adapter.

Covers the contract that :meth:`rag.llm.chat_model.Base.bind_tools` relies
on: each ``@tool`` callable carries a well-formed OpenAI function schema,
required vs. optional params are derived from defaults, and the session
dispatches both sync and async callables by name.
"""
import asyncio

import pytest

from rag.llm.tool_decorator import FunctionToolSession, is_tool, tool


@tool
def get_weather(city: str) -> str:
    """Get current weather for a city.

    :param city: City name to look up.
    """
    return f"{city}: 21 C, partly cloudy"


@tool
def add(a: int, b: int = 1) -> int:
    """Return the sum of two numbers.

    :param a: first addend
    :param b: second addend (defaults to 1)
    """
    return a + b


@tool
async def echo_async(text: str) -> str:
    """Return the input untouched."""
    return text


def test_tool_marks_callable_and_attaches_schema():
    assert is_tool(get_weather)
    schema = get_weather.openai_schema
    assert schema["type"] == "function"
    fn = schema["function"]
    assert fn["name"] == "get_weather"
    assert "current weather" in fn["description"].lower()
    params = fn["parameters"]
    assert params["type"] == "object"
    assert params["properties"]["city"]["type"] == "string"
    assert params["properties"]["city"]["description"] == "City name to look up."
    assert params["required"] == ["city"]


def test_required_vs_optional_from_defaults():
    params = add.openai_schema["function"]["parameters"]
    assert params["properties"]["a"]["type"] == "integer"
    assert params["properties"]["b"]["type"] == "integer"
    assert params["required"] == ["a"]


def test_session_dispatches_sync_tool():
    session = FunctionToolSession([get_weather, add])
    result = session.tool_call("get_weather", {"city": "Beijing"})
    assert "Beijing" in result
    assert session.tool_call("add", {"a": 2, "b": 3}) == 5


def test_session_dispatches_async_tool():
    session = FunctionToolSession([echo_async])
    result = asyncio.run(session.tool_call_async("echo_async", {"text": "hi"}))
    assert result == "hi"


def test_session_rejects_unknown_name():
    session = FunctionToolSession([get_weather])
    with pytest.raises(KeyError):
        asyncio.run(session.tool_call_async("nope", {}))


def test_session_rejects_non_mapping_arguments():
    session = FunctionToolSession([get_weather])
    with pytest.raises(TypeError):
        asyncio.run(session.tool_call_async("get_weather", ["Beijing"]))


def test_session_rejects_non_tool_callable():
    def plain(x): return x

    with pytest.raises(TypeError):
        FunctionToolSession([plain])


def test_schemas_passthrough_for_bind_tools():
    session = FunctionToolSession([get_weather, add])
    schemas = session.schemas
    assert [s["function"]["name"] for s in schemas] == ["get_weather", "add"]


def test_async_tool_timeout_raises():
    @tool
    async def slow() -> str:
        """Sleep longer than the timeout."""
        await asyncio.sleep(0.5)
        return "done"

    session = FunctionToolSession([slow])
    with pytest.raises(asyncio.TimeoutError):
        asyncio.run(session.tool_call_async("slow", {}, request_timeout=0.05))

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
"""Lightweight ``@tool`` decorator and matching ``ToolCallSession`` adapter.

Lets callers register plain Python functions as LLM tools without having to
hand-write the OpenAI function schema or build an MCP-style session::

    from rag.llm.tool_decorator import tool

    @tool
    def get_weather(city: str) -> str:
        \"\"\"Get current weather for a city.

        :param city: City name to look up.
        \"\"\"
        return f"{city}: 21 C, partly cloudy"

    chat_mdl.bind_tools(tools=[get_weather])

The decorator introspects the function signature, type hints, and docstring,
attaches an OpenAI-format schema as ``fn.openai_schema``, and marks the
function with ``fn._is_tool = True`` so :meth:`Base.bind_tools` can detect
the new style.
"""

from __future__ import annotations

import asyncio
import inspect
import logging
import re
from collections.abc import Mapping
from typing import Any, Callable, Union, get_args, get_origin, get_type_hints

from common.misc_utils import thread_pool_exec


_PY_TO_JSON: dict[type, str] = {
    str: "string",
    int: "integer",
    float: "number",
    bool: "boolean",
    list: "array",
    dict: "object",
    type(None): "null",
}


def _py_type_to_json(py_type: Any) -> dict[str, Any]:
    """Best-effort mapping from a Python annotation to a JSON-schema fragment.

    Handles ``Optional[T]`` / ``T | None`` by unwrapping the non-None branch
    and lets the ``required`` list (built from defaults) carry optionality.
    Unknown types fall back to ``{"type": "string"}`` so the schema stays
    valid even when annotations are missing.
    """
    if py_type is inspect.Parameter.empty or py_type is Any:
        return {"type": "string"}

    origin = get_origin(py_type)
    if origin is Union:
        non_none = [a for a in get_args(py_type) if a is not type(None)]
        if len(non_none) == 1:
            return _py_type_to_json(non_none[0])
        return {"type": "string"}

    if origin in (list, tuple, set, frozenset):
        item_args = get_args(py_type)
        item_schema = _py_type_to_json(item_args[0]) if item_args else {"type": "string"}
        return {"type": "array", "items": item_schema}

    if origin is dict:
        return {"type": "object"}

    if isinstance(py_type, type):
        return {"type": _PY_TO_JSON.get(py_type, "string")}

    return {"type": "string"}


_PARAM_RE = re.compile(r"^\s*:param\s+(?P<name>\w+)\s*:\s*(?P<desc>.+?)\s*$")


def _parse_param_docs(docstring: str | None) -> tuple[str, dict[str, str]]:
    """Pull a short function description and ``:param name:`` lines out of a docstring.

    Intentionally minimal — Google/NumPy styles are not parsed. Anything
    before the first ``:param`` line becomes the function description.
    """
    if not docstring:
        return "", {}

    lines = inspect.cleandoc(docstring).splitlines()
    desc_lines: list[str] = []
    param_docs: dict[str, str] = {}
    for line in lines:
        m = _PARAM_RE.match(line)
        if m:
            param_docs[m.group("name")] = m.group("desc")
        elif not param_docs:
            desc_lines.append(line)
    return "\n".join(desc_lines).strip(), param_docs


def _build_openai_schema(fn: Callable[..., Any]) -> dict[str, Any]:
    sig = inspect.signature(fn)
    try:
        hints = get_type_hints(fn)
    except Exception:
        hints = {}

    description, param_docs = _parse_param_docs(fn.__doc__)

    properties: dict[str, dict[str, Any]] = {}
    required: list[str] = []
    for name, param in sig.parameters.items():
        if name in ("self", "cls") or param.kind in (
            inspect.Parameter.VAR_POSITIONAL,
            inspect.Parameter.VAR_KEYWORD,
        ):
            continue
        schema = _py_type_to_json(hints.get(name, param.annotation))
        if name in param_docs:
            schema["description"] = param_docs[name]
        properties[name] = schema
        if param.default is inspect.Parameter.empty:
            required.append(name)

    return {
        "type": "function",
        "function": {
            "name": fn.__name__,
            "description": description or fn.__name__,
            "parameters": {
                "type": "object",
                "properties": properties,
                "required": required,
            },
        },
    }


def tool(fn: Callable[..., Any]) -> Callable[..., Any]:
    """Mark ``fn`` as an LLM tool and attach an OpenAI-format schema to it.

    The wrapped callable is the same callable — we only set two attributes:

    * ``fn._is_tool = True`` — sentinel so :meth:`Base.bind_tools` can tell a
      ``@tool`` callable apart from a raw schema dict.
    * ``fn.openai_schema`` — the schema dict passed verbatim to the LLM
      provider in the ``tools=[...]`` request field.
    """
    fn.openai_schema = _build_openai_schema(fn)  # type: ignore[attr-defined]
    fn._is_tool = True  # type: ignore[attr-defined]
    return fn


def is_tool(obj: Any) -> bool:
    return callable(obj) and getattr(obj, "_is_tool", False)


class FunctionToolSession:
    """Adapter that lets a list of ``@tool``-decorated callables satisfy the
    :class:`common.mcp_tool_call_conn.ToolCallSession` protocol used by the
    chat model tool loop (duck-typed, no explicit inheritance to avoid
    pulling the MCP client SDK into this module's import graph).

    The chat model only ever calls ``tool_call`` / ``tool_call_async`` with
    ``(name, arguments)`` — this class looks the name up in ``tools_map`` and
    invokes the callable, awaiting it if it is a coroutine and otherwise
    pushing it through ``thread_pool_exec`` so the event loop is not blocked.
    """

    def __init__(self, tools: list[Callable[..., Any]]):
        self.tools_map: dict[str, Callable[..., Any]] = {}
        for fn in tools:
            if not is_tool(fn):
                raise TypeError(
                    f"{getattr(fn, '__name__', fn)!r} is not a @tool-decorated callable"
                )
            self.tools_map[fn.openai_schema["function"]["name"]] = fn

    @property
    def schemas(self) -> list[dict[str, Any]]:
        return [fn.openai_schema for fn in self.tools_map.values()]

    def tool_call(self, name: str, arguments: dict[str, Any], timeout: float | int = 10) -> Any:
        return asyncio.run(self.tool_call_async(name, arguments, request_timeout=timeout))

    async def tool_call_async(self, name: str, arguments: dict[str, Any], request_timeout: float | int = 10) -> Any:
        if name not in self.tools_map:
            raise KeyError(f"Tool {name!r} is not registered")
        if not isinstance(arguments, Mapping):
            raise TypeError(
                f"Tool arguments for {name} must be an object, got {type(arguments).__name__}"
            )
        fn = self.tools_map[name]
        logging.info(f"[FunctionTool] invoke name={name} args={str(arguments)[:200]}")
        if asyncio.iscoroutinefunction(fn):
            coro = fn(**arguments)
        else:
            # Sync callables run in the thread pool. asyncio.wait_for cancels
            # the awaiting task on timeout, but Python cannot interrupt the
            # underlying worker thread — the function keeps running in the
            # background until it returns. Callers should treat sync tools
            # that block on I/O accordingly.
            coro = thread_pool_exec(fn, **arguments)
        return await asyncio.wait_for(coro, timeout=request_timeout)

import asyncio
from concurrent.futures import ThreadPoolExecutor
import logging
from string import Template
from typing import Any, Literal
from typing_extensions import override

from mcp.client.session import ClientSession
from mcp.client.sse import sse_client
from mcp.client.streamable_http import streamablehttp_client
from mcp.types import CallToolResult, ListToolsResult, TextContent, Tool

from api.db import MCPServerType
from rag.llm.chat_model import ToolCallSession


MCPTaskType = Literal["list_tools", "tool_call", "stop"]
MCPTask = tuple[MCPTaskType, dict[str, Any], asyncio.Queue[Any]]


class MCPToolCallSession(ToolCallSession):
    _EVENT_LOOP = asyncio.new_event_loop()
    _THREAD_POOL = ThreadPoolExecutor(max_workers=1)

    _mcp_server: Any
    _server_variables: dict[str, Any]
    _queue: asyncio.Queue[MCPTask]
    _stop = False

    @classmethod
    def _init_thread_pool(cls) -> None:
        cls._THREAD_POOL.submit(cls._EVENT_LOOP.run_forever)

    def __init__(self, mcp_server: Any, server_variables: dict[str, Any] | None = None) -> None:
        self._mcp_server = mcp_server
        self._server_variables = server_variables or {}
        self._queue = asyncio.Queue()

        asyncio.run_coroutine_threadsafe(self._mcp_server_loop(), MCPToolCallSession._EVENT_LOOP)

    async def _mcp_server_loop(self) -> None:
        url = self._mcp_server.url
        raw_headers: dict[str, str] = self._mcp_server.headers or {}
        headers: dict[str, str] = {}

        for h, v in raw_headers.items():
            nh = Template(h).safe_substitute(self._server_variables)
            nv = Template(v).safe_substitute(self._server_variables)
            headers[nh] = nv

        _streams_source: Any

        if self._mcp_server.server_type == MCPServerType.SSE:
            _streams_source = sse_client(url, headers)
        elif self._mcp_server.server_type == MCPServerType.StreamableHttp:
            _streams_source = streamablehttp_client(url, headers)
        else:
            raise ValueError(f"Unsupported MCP server type {self._mcp_server.server_type} id {self._mcp_server.id}")

        async with _streams_source as streams:
            async with ClientSession(*streams) as client_session:
                await client_session.initialize()

                while not self._stop:
                    mcp_task, arguments, result_queue = await self._queue.get()
                    logging.debug(f"Got MCP task {mcp_task} arguments {arguments}")

                    r: Any

                    try:
                        if mcp_task == "list_tools":
                            r = await client_session.list_tools()
                        elif mcp_task == "tool_call":
                            r = await client_session.call_tool(**arguments)
                        elif mcp_task == "stop":
                            logging.debug(f"Shutting down MCPToolCallSession for server {self._mcp_server.id}")
                            self._stop = True
                            continue
                        else:
                            r = ValueError(f"MCPToolCallSession for server {self._mcp_server.id} received an unknown task {mcp_task}")
                    except Exception as e:
                        r = e

                    await result_queue.put(r)

    async def _call_mcp_server(self, task_type: MCPTaskType, **kwargs) -> Any:
        results = asyncio.Queue()
        await self._queue.put((task_type, kwargs, results))
        result: CallToolResult | Exception = await results.get()

        if isinstance(result, Exception):
            raise result

        return result

    async def _call_mcp_tool(self, name: str, arguments: dict[str, Any]) -> str:
        result: CallToolResult = await self._call_mcp_server("tool_call", name=name, arguments=arguments)

        if result.isError:
            return f"MCP server error: {result.content}"

        # For now we only support text content
        if isinstance(result.content[0], TextContent):
            return result.content[0].text
        else:
            return f"Unsupported content type {type(result.content)}"

    async def _get_tools_from_mcp_server(self) -> list[Tool]:
        # For now we only fetch the first page of tools
        result: ListToolsResult = await self._call_mcp_server("list_tools")
        return result.tools

    def get_tools(self) -> list[Tool]:
        return asyncio.run_coroutine_threadsafe(self._get_tools_from_mcp_server(), MCPToolCallSession._EVENT_LOOP).result()

    @override
    def tool_call(self, name: str, arguments: dict[str, Any]) -> str:
        return asyncio.run_coroutine_threadsafe(self._call_mcp_tool(name, arguments), MCPToolCallSession._EVENT_LOOP).result()

    async def close(self) -> None:
        await self._call_mcp_server("stop")

    def close_sync(self) -> None:
        asyncio.run_coroutine_threadsafe(self.close(), MCPToolCallSession._EVENT_LOOP).result()


MCPToolCallSession._init_thread_pool()


def close_multiple_mcp_toolcall_sessions(sessions: list[MCPToolCallSession]) -> None:
    async def _gather() -> None:
        await asyncio.gather(*[s.close() for s in sessions], return_exceptions=True)

    asyncio.run_coroutine_threadsafe(_gather(), MCPToolCallSession._EVENT_LOOP).result()


def mcp_tool_metadata_to_openai_tool(mcp_tool: Tool) -> dict[str, Any]:
    return {
        "type": "function",
        "function": {
            "name": mcp_tool.name,
            "description": mcp_tool.description,
            "parameters": mcp_tool.inputSchema,
        },
    }

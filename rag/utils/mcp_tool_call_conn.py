#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

import asyncio
import logging
import threading
import weakref
from concurrent.futures import ThreadPoolExecutor
from concurrent.futures import TimeoutError as FuturesTimeoutError
from string import Template
from typing import Any, Literal

from typing_extensions import override

from api.db import MCPServerType
from mcp.client.session import ClientSession
from mcp.client.sse import sse_client
from mcp.client.streamable_http import streamablehttp_client
from mcp.types import CallToolResult, ListToolsResult, TextContent, Tool
from rag.llm.chat_model import ToolCallSession

MCPTaskType = Literal["list_tools", "tool_call"]
MCPTask = tuple[MCPTaskType, dict[str, Any], asyncio.Queue[Any]]


class MCPToolCallSession(ToolCallSession):
    _ALL_INSTANCES: weakref.WeakSet["MCPToolCallSession"] = weakref.WeakSet()

    def __init__(self, mcp_server: Any, server_variables: dict[str, Any] | None = None) -> None:
        self.__class__._ALL_INSTANCES.add(self)

        self._mcp_server = mcp_server
        self._server_variables = server_variables or {}
        self._queue = asyncio.Queue()
        self._close = False

        self._event_loop = asyncio.new_event_loop()
        self._thread_pool = ThreadPoolExecutor(max_workers=1)
        self._thread_pool.submit(self._event_loop.run_forever)

        asyncio.run_coroutine_threadsafe(self._mcp_server_loop(), self._event_loop)

    async def _mcp_server_loop(self) -> None:
        url = self._mcp_server.url.strip()
        raw_headers: dict[str, str] = self._mcp_server.headers or {}
        headers: dict[str, str] = {}

        for h, v in raw_headers.items():
            nh = Template(h).safe_substitute(self._server_variables)
            nv = Template(v).safe_substitute(self._server_variables)
            headers[nh] = nv

        if self._mcp_server.server_type == MCPServerType.SSE:
            # SSE transport
            try:
                async with sse_client(url, headers) as stream:
                    async with ClientSession(*stream) as client_session:
                        try:
                            await asyncio.wait_for(client_session.initialize(), timeout=5)
                            logging.info("client_session initialized successfully")
                            await self._process_mcp_tasks(client_session)
                        except asyncio.TimeoutError:
                            msg = f"Timeout initializing client_session for server {self._mcp_server.id}"
                            logging.error(msg)
                            await self._process_mcp_tasks(None, msg)
            except Exception:
                msg = "Connection failed (possibly due to auth error). Please check authentication settings first"
                await self._process_mcp_tasks(None, msg)

        elif self._mcp_server.server_type == MCPServerType.STREAMABLE_HTTP:
            # Streamable HTTP transport
            try:
                async with streamablehttp_client(url, headers) as (read_stream, write_stream, _):
                    async with ClientSession(read_stream, write_stream) as client_session:
                        try:
                            await asyncio.wait_for(client_session.initialize(), timeout=5)
                            logging.info("client_session initialized successfully")
                            await self._process_mcp_tasks(client_session)
                        except asyncio.TimeoutError:
                            msg = f"Timeout initializing client_session for server {self._mcp_server.id}"
                            logging.error(msg)
                            await self._process_mcp_tasks(None, msg)
            except Exception:
                msg = "Connection failed (possibly due to auth error). Please check authentication settings first"
                await self._process_mcp_tasks(None, msg)

        else:
            await self._process_mcp_tasks(None, f"Unsupported MCP server type: {self._mcp_server.server_type}, id: {self._mcp_server.id}")

    async def _process_mcp_tasks(self, client_session: ClientSession | None, error_message: str | None = None) -> None:
        while not self._close:
            try:
                mcp_task, arguments, result_queue = await asyncio.wait_for(self._queue.get(), timeout=1)
            except asyncio.TimeoutError:
                continue

            logging.debug(f"Got MCP task {mcp_task} arguments {arguments}")

            r: Any = None

            if not client_session or error_message:
                r = ValueError(error_message)
                await result_queue.put(r)
                continue

            try:
                if mcp_task == "list_tools":
                    r = await client_session.list_tools()
                elif mcp_task == "tool_call":
                    r = await client_session.call_tool(**arguments)
                else:
                    r = ValueError(f"Unknown MCP task {mcp_task}")
            except Exception as e:
                r = e

            await result_queue.put(r)

    async def _call_mcp_server(self, task_type: MCPTaskType, timeout: float | int = 8, **kwargs) -> Any:
        results = asyncio.Queue()
        await self._queue.put((task_type, kwargs, results))

        try:
            result: CallToolResult | Exception = await asyncio.wait_for(results.get(), timeout=timeout)
            if isinstance(result, Exception):
                raise result
            return result
        except asyncio.TimeoutError:
            raise asyncio.TimeoutError(f"MCP task '{task_type}' timeout after {timeout}s")
        except Exception:
            raise

    async def _call_mcp_tool(self, name: str, arguments: dict[str, Any], timeout: float | int = 10) -> str:
        result: CallToolResult = await self._call_mcp_server("tool_call", name=name, arguments=arguments, timeout=timeout)

        if result.isError:
            return f"MCP server error: {result.content}"

        # For now we only support text content
        if isinstance(result.content[0], TextContent):
            return result.content[0].text
        else:
            return f"Unsupported content type {type(result.content)}"

    async def _get_tools_from_mcp_server(self, timeout: float | int = 8) -> list[Tool]:
        try:
            result: ListToolsResult = await self._call_mcp_server("list_tools", timeout=timeout)
            return result.tools
        except Exception:
            raise

    def get_tools(self, timeout: float | int = 10) -> list[Tool]:
        future = asyncio.run_coroutine_threadsafe(self._get_tools_from_mcp_server(timeout=timeout), self._event_loop)
        try:
            return future.result(timeout=timeout)
        except FuturesTimeoutError:
            msg = f"Timeout when fetching tools from MCP server: {self._mcp_server.id} (timeout={timeout})"
            logging.error(msg)
            raise RuntimeError(msg)
        except Exception:
            logging.exception(f"Error fetching tools from MCP server: {self._mcp_server.id}")
            raise

    @override
    def tool_call(self, name: str, arguments: dict[str, Any], timeout: float | int = 10) -> str:
        future = asyncio.run_coroutine_threadsafe(self._call_mcp_tool(name, arguments), self._event_loop)
        try:
            return future.result(timeout=timeout)
        except FuturesTimeoutError:
            logging.error(f"Timeout calling tool '{name}' on MCP server: {self._mcp_server.id} (timeout={timeout})")
            return f"Timeout calling tool '{name}' (timeout={timeout})."
        except Exception as e:
            logging.exception(f"Error calling tool '{name}' on MCP server: {self._mcp_server.id}")
            return f"Error calling tool '{name}': {e}."

    async def close(self) -> None:
        if self._close:
            return

        self._close = True
        self._event_loop.call_soon_threadsafe(self._event_loop.stop)
        self._thread_pool.shutdown(wait=True)
        self.__class__._ALL_INSTANCES.discard(self)

    def close_sync(self, timeout: float | int = 5) -> None:
        if not self._event_loop.is_running():
            logging.warning(f"Event loop already stopped for {self._mcp_server.id}")
            return

        future = asyncio.run_coroutine_threadsafe(self.close(), self._event_loop)
        try:
            future.result(timeout=timeout)
        except FuturesTimeoutError:
            logging.error(f"Timeout while closing session for server {self._mcp_server.id} (timeout={timeout})")
        except Exception:
            logging.exception(f"Unexpected error during close_sync for {self._mcp_server.id}")


def close_multiple_mcp_toolcall_sessions(sessions: list[MCPToolCallSession]) -> None:
    logging.info(f"Want to clean up {len(sessions)} MCP sessions")

    async def _gather_and_stop() -> None:
        try:
            await asyncio.gather(*[s.close() for s in sessions if s is not None], return_exceptions=True)
        finally:
            loop.call_soon_threadsafe(loop.stop)

    loop = asyncio.new_event_loop()
    thread = threading.Thread(target=loop.run_forever, daemon=True)
    thread.start()

    asyncio.run_coroutine_threadsafe(_gather_and_stop(), loop).result()

    thread.join()
    logging.info(f"{len(sessions)} MCP sessions has been cleaned up. {len(list(MCPToolCallSession._ALL_INSTANCES))} in global context.")


def shutdown_all_mcp_sessions():
    """Gracefully shutdown all active MCPToolCallSession instances."""
    sessions = list(MCPToolCallSession._ALL_INSTANCES)
    if not sessions:
        logging.info("No MCPToolCallSession instances to close.")
        return

    logging.info(f"Shutting down {len(sessions)} MCPToolCallSession instances...")
    close_multiple_mcp_toolcall_sessions(sessions)
    logging.info("All MCPToolCallSession instances have been closed.")


def mcp_tool_metadata_to_openai_tool(mcp_tool: Tool|dict) -> dict[str, Any]:
    if isinstance(mcp_tool, dict):
        return {
            "type": "function",
            "function": {
                "name": mcp_tool["name"],
                "description": mcp_tool["description"],
                "parameters": mcp_tool["inputSchema"],
            },
        }

    return {
        "type": "function",
        "function": {
            "name": mcp_tool.name,
            "description": mcp_tool.description,
            "parameters": mcp_tool.inputSchema,
        },
    }

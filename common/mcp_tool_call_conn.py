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
import time
import weakref
from concurrent.futures import CancelledError as FuturesCancelledError
from concurrent.futures import Future, ThreadPoolExecutor
from concurrent.futures import TimeoutError as FuturesTimeoutError
from dataclasses import dataclass
from string import Template
from typing import Any, Literal, Protocol

from typing_extensions import override

from common.constants import MCPServerType
from mcp.client.session import ClientSession
from mcp.client.sse import sse_client
from mcp.client.streamable_http import streamablehttp_client
from mcp.types import CallToolResult, ListToolsResult, TextContent, Tool

logger = logging.getLogger(__name__)

MCPTaskType = Literal["list_tools", "tool_call"]
MCPTask = tuple[MCPTaskType, dict[str, Any], asyncio.Queue[Any], float, asyncio.Event]


class ToolCallSession(Protocol):
    def tool_call(self, name: str, arguments: dict[str, Any], timeout: float | int = 10) -> str: ...


@dataclass(frozen=True)
class MCPToolBinding:
    session: ToolCallSession
    original_name: str


class MCPToolCallSession(ToolCallSession):
    _ALL_INSTANCES: weakref.WeakSet["MCPToolCallSession"] = weakref.WeakSet()
    _INSTANCES_LOCK = threading.Lock()
    _GLOBAL_SHUTDOWN_COUNT = 0

    def __init__(self, mcp_server: Any, server_variables: dict[str, Any] | None = None, custom_header=None) -> None:
        self._custom_header = custom_header
        self._mcp_server = mcp_server
        self._server_variables = server_variables or {}
        self._queue: asyncio.Queue[MCPTask] = asyncio.Queue()
        self._close = False
        self._pending_calls: dict[asyncio.Task[Any], asyncio.Queue[Any]] = {}
        self._shutdown_lock = threading.Lock()
        self._shutdown_future: Future[None] | None = None
        self._shutdown_complete = threading.Event()
        self._server_task: asyncio.Task[None] | None = None
        self._server_task_started = asyncio.Event()
        self._owner_close_waiters = 0
        self._stop_handle: asyncio.TimerHandle | None = None

        self._event_loop = asyncio.new_event_loop()
        self._thread = threading.Thread(target=self._run_event_loop, name=f"mcp-session-{mcp_server.id}", daemon=True)
        coroutine = self._run_mcp_server_loop()
        thread_started = False
        coroutine_submitted = False
        try:
            with self.__class__._INSTANCES_LOCK:
                if self.__class__._GLOBAL_SHUTDOWN_COUNT:
                    raise RuntimeError("Cannot create MCP session during global shutdown")
                self._thread.start()
                thread_started = True
                self._server_future = asyncio.run_coroutine_threadsafe(coroutine, self._event_loop)
                coroutine_submitted = True
                self.__class__._ALL_INSTANCES.add(self)
        except BaseException as startup_error:  # noqa: BLE001
            startup_traceback = startup_error.__traceback__
            was_started = thread_started or self._thread.ident is not None
            if was_started:
                try:
                    if not self._event_loop.is_closed():
                        self._event_loop.call_soon_threadsafe(self._event_loop.stop)
                except BaseException:
                    logger.exception("Failed to stop MCP session event loop during startup rollback")
                try:
                    self._thread.join()
                except BaseException:
                    logger.exception("Failed to join MCP session thread during startup rollback")
            else:
                try:
                    self._event_loop.close()
                except BaseException:
                    logger.exception("Failed to close MCP session event loop during startup rollback")
                finally:
                    self._shutdown_complete.set()
            try:
                if not coroutine_submitted and coroutine.cr_frame is not None:
                    coroutine.close()
            except BaseException:
                logger.exception("Failed to close MCP server coroutine during startup rollback")
            try:
                with self.__class__._INSTANCES_LOCK:
                    self.__class__._ALL_INSTANCES.discard(self)
            except BaseException:
                logger.exception("Failed to unregister MCP session during startup rollback")
            raise startup_error.with_traceback(startup_traceback)

    @classmethod
    def _active_instances(cls) -> list["MCPToolCallSession"]:
        with cls._INSTANCES_LOCK:
            return list(cls._ALL_INSTANCES)

    @classmethod
    def _begin_global_shutdown(cls) -> list["MCPToolCallSession"]:
        with cls._INSTANCES_LOCK:
            cls._GLOBAL_SHUTDOWN_COUNT += 1
        try:
            return cls._active_instances()
        except BaseException:
            cls._end_global_shutdown()
            raise

    @classmethod
    def _end_global_shutdown(cls) -> None:
        with cls._INSTANCES_LOCK:
            cls._GLOBAL_SHUTDOWN_COUNT -= 1

    def _run_event_loop(self) -> None:
        asyncio.set_event_loop(self._event_loop)
        try:
            self._event_loop.run_forever()
        finally:
            try:
                try:
                    self._drain_pending_tasks()
                except Exception:
                    logger.exception("Failed to drain pending tasks for MCP server %s", self._mcp_server.id)
                try:
                    self._event_loop.run_until_complete(self._event_loop.shutdown_asyncgens())
                except Exception:
                    logger.exception("Failed to shut down async generators for MCP server %s", self._mcp_server.id)
                try:
                    self._drain_pending_tasks()
                except Exception:
                    logger.exception("Failed to drain late tasks for MCP server %s", self._mcp_server.id)
                try:
                    self._event_loop.run_until_complete(self._event_loop.shutdown_default_executor())
                except Exception:
                    logger.exception("Failed to shut down default executor for MCP server %s", self._mcp_server.id)
            finally:
                try:
                    self._event_loop.close()
                finally:
                    with self.__class__._INSTANCES_LOCK:
                        self.__class__._ALL_INSTANCES.discard(self)
                    self._shutdown_complete.set()

    def _drain_pending_tasks(self) -> None:
        pending_tasks = [task for task in asyncio.all_tasks(self._event_loop) if not task.done()]
        for task in pending_tasks:
            task.cancel()
        if pending_tasks:
            self._event_loop.run_until_complete(asyncio.gather(*pending_tasks, return_exceptions=True))

    async def _run_mcp_server_loop(self) -> None:
        self._server_task = asyncio.current_task()
        self._server_task_started.set()
        await self._mcp_server_loop()

    async def _mcp_server_loop(self) -> None:
        url = self._mcp_server.url.strip()
        raw_headers: dict[str, str] = self._mcp_server.headers or {}
        custom_header: dict[str, str] = self._custom_header or {}
        headers: dict[str, str] = {}

        for h, v in raw_headers.items():
            nh = Template(h).safe_substitute(self._server_variables)
            nv = Template(v).safe_substitute(self._server_variables)
            if nh.strip() and nv.strip().strip("Bearer"):
                headers[nh] = nv

        for h, v in custom_header.items():
            nh = Template(h).safe_substitute(custom_header)
            nv = Template(v).safe_substitute(custom_header)
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
                        except asyncio.CancelledError:
                            logging.warning(f"SSE transport MCP session cancelled for server {self._mcp_server.id}")
                            return
            except Exception:
                if self._close:
                    raise
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
                        except asyncio.CancelledError:
                            logging.warning(f"STREAMABLE_HTTP MCP session cancelled for server {self._mcp_server.id}")
                            return
            except Exception as e:
                if self._close:
                    raise
                logging.exception(e)
                msg = "Connection failed (possibly due to auth error). Please check authentication settings first"
                await self._process_mcp_tasks(None, msg)

        else:
            await self._process_mcp_tasks(None, f"Unsupported MCP server type: {self._mcp_server.server_type}, id: {self._mcp_server.id}")

    async def _process_mcp_tasks(self, client_session: ClientSession | None, error_message: str | None = None) -> None:
        while not self._close:
            try:
                mcp_task, arguments, result_queue, deadline, abandoned = await asyncio.wait_for(self._queue.get(), timeout=1)
            except asyncio.TimeoutError:
                continue
            except asyncio.CancelledError:
                break

            logging.debug(f"Got MCP task {mcp_task} arguments {arguments}")

            remaining = deadline - time.monotonic()
            if abandoned.is_set() or remaining <= 0:
                logger.debug(f"Skipping expired MCP task {mcp_task}")
                continue

            r: Any = None

            if not client_session or error_message:
                r = ValueError(error_message)
                try:
                    await result_queue.put(r)
                except asyncio.CancelledError:
                    break
                continue

            try:
                if mcp_task == "list_tools":
                    r = await asyncio.wait_for(client_session.list_tools(), timeout=remaining)
                elif mcp_task == "tool_call":
                    r = await asyncio.wait_for(client_session.call_tool(**arguments), timeout=remaining)
                else:
                    r = ValueError(f"Unknown MCP task {mcp_task}")
            except TimeoutError:
                r = TimeoutError(f"MCP task '{mcp_task}' timeout")
            except Exception as e:
                r = e
            except asyncio.CancelledError:
                break

            try:
                await result_queue.put(r)
            except asyncio.CancelledError:
                break

    async def _call_mcp_server(self, task_type: MCPTaskType, request_timeout: float | int = 8, deadline: float | None = None, **kwargs) -> Any:
        if self._close:
            raise ValueError("Session is closed")

        results = asyncio.Queue()
        caller_task = asyncio.current_task()
        if caller_task is None:
            raise RuntimeError("MCP calls require an asyncio task")
        self._pending_calls[caller_task] = results
        deadline = deadline if deadline is not None else time.monotonic() + request_timeout
        abandoned = asyncio.Event()
        await self._queue.put((task_type, kwargs, results, deadline, abandoned))

        try:
            result: CallToolResult | Exception = await asyncio.wait_for(results.get(), timeout=deadline - time.monotonic())
            if isinstance(result, Exception):
                raise result
            return result
        except asyncio.TimeoutError:
            raise asyncio.TimeoutError(f"MCP task '{task_type}' timeout after {request_timeout}s")
        except Exception:
            raise
        finally:
            abandoned.set()
            self._pending_calls.pop(caller_task, None)

    async def _call_mcp_tool(self, name: str, arguments: dict[str, Any], request_timeout: float | int = 10, deadline: float | None = None) -> str:
        result: CallToolResult = await self._call_mcp_server("tool_call", name=name, arguments=arguments, request_timeout=request_timeout, deadline=deadline)

        if result.isError:
            return f"MCP server error: {result.content}"

        # For now, we only support text content
        if not result.content:
            return "MCP server returned empty content."
        if isinstance(result.content[0], TextContent):
            return result.content[0].text
        else:
            return f"Unsupported content type {type(result.content)}"

    async def _get_tools_from_mcp_server(self, request_timeout: float | int = 8, deadline: float | None = None) -> list[Tool]:
        try:
            result: ListToolsResult = await self._call_mcp_server("list_tools", request_timeout=request_timeout, deadline=deadline)
            return result.tools
        except Exception:
            raise

    def get_tools(self, timeout: float | int = 10) -> list[Tool]:
        deadline = time.monotonic() + timeout
        with self._shutdown_lock:
            if self._close:
                raise ValueError("Session is closed")
            coroutine = self._get_tools_from_mcp_server(request_timeout=timeout, deadline=deadline)
            try:
                future = asyncio.run_coroutine_threadsafe(coroutine, self._event_loop)
            except Exception:
                coroutine.close()
                raise
        try:
            return future.result(timeout=max(0, deadline - time.monotonic()))
        except FuturesTimeoutError:
            future.cancel()
            msg = f"Timeout when fetching tools from MCP server: {self._mcp_server.id} (timeout={timeout})"
            logging.error(msg)
            raise RuntimeError(msg)
        except Exception:
            logging.exception(f"Error fetching tools from MCP server: {self._mcp_server.id}")
            raise

    @override
    def tool_call(self, name: str, arguments: dict[str, Any], timeout: float | int = 10) -> str:
        deadline = time.monotonic() + timeout
        with self._shutdown_lock:
            if self._close:
                return "Error: Session is closed"
            coroutine = self._call_mcp_tool(name, arguments, request_timeout=timeout, deadline=deadline)
            try:
                future = asyncio.run_coroutine_threadsafe(coroutine, self._event_loop)
            except Exception:
                coroutine.close()
                raise
        try:
            return future.result(timeout=max(0, deadline - time.monotonic()))
        except FuturesTimeoutError:
            future.cancel()
            logging.error(f"Timeout calling tool '{name}' on MCP server: {self._mcp_server.id} (timeout={timeout})")
            return f"Timeout calling tool '{name}' (timeout={timeout})."
        except Exception as e:
            logging.exception(f"Error calling tool '{name}' on MCP server: {self._mcp_server.id}")
            return f"Error calling tool '{name}': {e}."

    async def _shutdown_on_event_loop(self) -> None:
        pending_calls = list(self._pending_calls.items())
        for _, result_queue in pending_calls:
            result_queue.put_nowait(ValueError("Session is closing"))

        while not self._queue.empty():
            self._queue.get_nowait()

        try:
            await self._server_task_started.wait()
            if self._server_task and not self._server_task.done():
                self._server_task.cancel()

            request_tasks = [task for task, _ in pending_calls]
            if request_tasks:
                await asyncio.gather(*request_tasks, return_exceptions=True)
            if self._server_task:
                try:
                    await self._server_task
                except asyncio.CancelledError:
                    pass
        finally:
            try:
                await self._event_loop.shutdown_asyncgens()
            finally:
                await self._event_loop.shutdown_default_executor()

    def _begin_shutdown(self) -> Future[None]:
        with self._shutdown_lock:
            if self._shutdown_future is None:
                self._close = True
                logger.info("Closing MCP session for server %s", self._mcp_server.id)
                coroutine = self._shutdown_on_event_loop()
                try:
                    self._shutdown_future = asyncio.run_coroutine_threadsafe(coroutine, self._event_loop)
                except Exception:
                    coroutine.close()
                    raise
                self._shutdown_future.add_done_callback(self._request_event_loop_stop)
            return self._shutdown_future

    def _request_event_loop_stop(self, _: Future[None]) -> None:
        if self._event_loop.is_closed():
            return
        self._event_loop.call_soon_threadsafe(self._schedule_event_loop_stop)

    def _schedule_event_loop_stop(self) -> None:
        if self._owner_close_waiters or self._event_loop.is_closed():
            return
        if self._stop_handle is None or self._stop_handle.cancelled():
            self._stop_handle = self._event_loop.call_later(0, self._event_loop.stop)

    async def close(self, timeout: float = 5) -> None:  # noqa: ASYNC109
        deadline = time.monotonic() + timeout
        shutdown_finished = False
        shutdown_error = None
        shutdown_traceback = None
        owner_loop = asyncio.get_running_loop() is self._event_loop
        if owner_loop:
            self._owner_close_waiters += 1
            if self._stop_handle is not None:
                self._stop_handle.cancel()
                self._stop_handle = None

        try:
            future = self._begin_shutdown()
            wrapped_future = asyncio.wrap_future(future)
            wrapped_future.add_done_callback(lambda completed: None if completed.cancelled() else completed.exception())
            done, _ = await asyncio.wait({wrapped_future}, timeout=max(0, deadline - time.monotonic()))
            if not done:
                logger.error("Timeout while closing session for server %s (timeout=%s)", self._mcp_server.id, timeout)
            else:
                shutdown_finished = True
                await wrapped_future
        except BaseException as error:  # noqa: BLE001
            shutdown_error = error
            shutdown_traceback = error.__traceback__
        finally:
            if owner_loop:
                self._owner_close_waiters -= 1
                if self._owner_close_waiters == 0 and self._shutdown_future is not None and self._shutdown_future.done():
                    self._schedule_event_loop_stop()

        join_error = None
        join_traceback = None
        if not owner_loop:
            remaining = max(0, deadline - time.monotonic())
            if self._thread.is_alive() and remaining:
                join_task = asyncio.create_task(asyncio.to_thread(self._thread.join, remaining))
                join_task.add_done_callback(lambda completed: None if completed.cancelled() else completed.exception())
                try:
                    done, _ = await asyncio.wait({join_task}, timeout=remaining)
                    if done:
                        await join_task
                except BaseException as error:  # noqa: BLE001
                    join_error = error
                    join_traceback = error.__traceback__
            if self._thread.is_alive() and shutdown_finished and join_error is None:
                logger.error("Timeout while joining MCP session thread for server %s (timeout=%s)", self._mcp_server.id, timeout)

        if shutdown_error is not None:
            if join_error is not None:
                logger.error(
                    "Exception while joining MCP session thread for server %s",
                    self._mcp_server.id,
                    exc_info=(type(join_error), join_error, join_traceback),
                )
            raise shutdown_error.with_traceback(shutdown_traceback)
        if join_error is not None:
            raise join_error.with_traceback(join_traceback)

    def close_sync(self, timeout: float | int = 5) -> None:
        try:
            running_loop = asyncio.get_running_loop()
        except RuntimeError:
            running_loop = None
        if running_loop is self._event_loop:
            raise RuntimeError("close_sync() cannot block the MCP session event loop")

        deadline = time.monotonic() + timeout
        future = self._begin_shutdown()

        try:
            shutdown_error = future.exception(timeout=max(0, deadline - time.monotonic()))
        except FuturesTimeoutError:
            logger.error("Timeout while closing session for server %s (timeout=%s)", self._mcp_server.id, timeout)
            return
        except FuturesCancelledError as error:
            shutdown_error = error

        join_error = None
        join_traceback = None
        try:
            self._thread.join(timeout=max(0, deadline - time.monotonic()))
        except Exception as error:  # noqa: BLE001
            join_error = error
            join_traceback = error.__traceback__
        if self._thread.is_alive() and join_error is None:
            logger.error("Timeout while joining MCP session thread for server %s (timeout=%s)", self._mcp_server.id, timeout)
        if shutdown_error is not None:
            if join_error is not None:
                logger.error(
                    "Exception while joining MCP session thread for server %s",
                    self._mcp_server.id,
                    exc_info=(type(join_error), join_error, join_traceback),
                )
            raise shutdown_error
        if join_error is not None:
            raise join_error.with_traceback(join_traceback)


def close_multiple_mcp_toolcall_sessions(sessions: list[MCPToolCallSession]) -> int:
    unique_sessions = list({id(session): session for session in sessions if session is not None}.values())
    current_thread = threading.current_thread()
    if any(session._thread is current_thread for session in unique_sessions):
        raise RuntimeError("MCP sessions must be closed outside their event loop threads")

    logger.info("Want to clean up %s MCP sessions", len(unique_sessions))

    cleanup_errors = 0
    with ThreadPoolExecutor() as executor:
        futures = {executor.submit(session.close_sync): session for session in unique_sessions}
        for future, session in futures.items():
            try:
                future.result()
            except Exception:
                cleanup_errors += 1
                logger.exception("Exception while closing MCP session for server %s", session._mcp_server.id)

    closed_count = sum(session._shutdown_complete.is_set() for session in unique_sessions)
    if closed_count != len(unique_sessions):
        logger.warning("%s of %s MCP sessions are still shutting down", len(unique_sessions) - closed_count, len(unique_sessions))
    if cleanup_errors:
        logger.warning("%s MCP sessions stopped; transport cleanup failure count: %s. %s in global context.", closed_count, cleanup_errors, len(MCPToolCallSession._active_instances()))
    else:
        logger.info("%s MCP sessions have been cleaned up. %s in global context.", closed_count, len(MCPToolCallSession._active_instances()))
    return cleanup_errors


def shutdown_all_mcp_sessions():
    """Gracefully shutdown all active MCPToolCallSession instances."""
    sessions = MCPToolCallSession._begin_global_shutdown()
    try:
        if not sessions:
            logging.info("No MCPToolCallSession instances to close.")
            return

        logging.info(f"Shutting down {len(sessions)} MCPToolCallSession instances...")
        cleanup_errors = close_multiple_mcp_toolcall_sessions(sessions)
        active_sessions = MCPToolCallSession._active_instances()
        if active_sessions:
            logger.warning("%s MCPToolCallSession instances are still shutting down.", len(active_sessions))
        elif cleanup_errors:
            logger.warning("All MCPToolCallSession event loops stopped; transport cleanup failure count: %s.", cleanup_errors)
        else:
            logger.info("All MCPToolCallSession instances have been closed.")
    finally:
        MCPToolCallSession._end_global_shutdown()


def mcp_tool_metadata_to_openai_tool(mcp_tool: Tool | dict, function_name: str | None = None) -> dict[str, Any]:
    if isinstance(mcp_tool, dict):
        return {
            "type": "function",
            "function": {
                "name": function_name or mcp_tool["name"],
                "description": mcp_tool["description"],
                "parameters": mcp_tool["inputSchema"],
            },
        }

    return {
        "type": "function",
        "function": {
            "name": function_name or mcp_tool.name,
            "description": mcp_tool.description,
            "parameters": mcp_tool.inputSchema,
        },
    }

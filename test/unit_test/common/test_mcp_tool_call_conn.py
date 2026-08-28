import asyncio
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from types import SimpleNamespace

import pytest

from common import mcp_tool_call_conn
from common.mcp_tool_call_conn import MCPToolCallSession


def _make_session() -> MCPToolCallSession:
    session = object.__new__(MCPToolCallSession)
    session._queue = asyncio.Queue()
    session._close = False
    return session


async def _call_tool(session: MCPToolCallSession, name: str, budget: float):
    return await session._call_mcp_server(
        "tool_call",
        request_timeout=budget,
        name=name,
        arguments={},
    )


async def _stop_tasks(session: MCPToolCallSession, *tasks: asyncio.Task) -> None:
    session._close = True
    for task in tasks:
        if not task.done():
            task.cancel()
    await asyncio.gather(*tasks, return_exceptions=True)
    assert all(task.done() for task in tasks)


@pytest.mark.parametrize("task_type", ["tool_call", "list_tools"])
@pytest.mark.asyncio
async def test_in_flight_timeout_cancels_client_request_and_allows_next_call(task_type):
    class ClientSession:
        def __init__(self):
            self.started = asyncio.Event()
            self.cancelled = asyncio.Event()

        async def _slow_request(self):
            self.started.set()
            try:
                await asyncio.Future()
            except asyncio.CancelledError:
                self.cancelled.set()
                raise

        async def call_tool(self, name, arguments):
            if name == "slow":
                return await self._slow_request()
            return f"{name}-result"

        async def list_tools(self):
            return await self._slow_request()

    session = _make_session()
    client_session = ClientSession()
    processor = asyncio.create_task(session._process_mcp_tasks(client_session))

    try:
        kwargs = {"name": "slow", "arguments": {}} if task_type == "tool_call" else {}
        call = asyncio.create_task(session._call_mcp_server(task_type, request_timeout=0.05, **kwargs))
        await asyncio.wait_for(client_session.started.wait(), timeout=1)

        with pytest.raises(asyncio.TimeoutError, match=f"MCP task '{task_type}' timeout"):
            await call

        await asyncio.wait_for(client_session.cancelled.wait(), timeout=1)
        result = await _call_tool(session, "next", budget=1)

        assert result == "next-result"
    finally:
        await _stop_tasks(session, call, processor)


@pytest.mark.asyncio
async def test_queued_request_expiring_before_dispatch_is_never_called():
    class ClientSession:
        def __init__(self):
            self.calls = []
            self.first_started = asyncio.Event()
            self.release_first = asyncio.Event()

        async def call_tool(self, name, arguments):
            self.calls.append(name)
            if name == "first":
                self.first_started.set()
                await self.release_first.wait()
            return f"{name}-result"

    session = _make_session()
    client_session = ClientSession()
    processor = asyncio.create_task(session._process_mcp_tasks(client_session))

    try:
        first_call = asyncio.create_task(_call_tool(session, "first", budget=1))
        await asyncio.wait_for(client_session.first_started.wait(), timeout=1)

        with pytest.raises(asyncio.TimeoutError, match="MCP task 'tool_call' timeout"):
            await _call_tool(session, "expired", budget=0.05)

        client_session.release_first.set()
        assert await first_call == "first-result"
        assert await _call_tool(session, "last", budget=1) == "last-result"
        assert client_session.calls == ["first", "last"]
    finally:
        await _stop_tasks(session, first_call, processor)


@pytest.fixture
def timed_out_session(monkeypatch):
    class TimedOutFuture:
        def __init__(self):
            self.was_cancelled = False

        def result(self, timeout):
            raise mcp_tool_call_conn.FuturesTimeoutError

        def cancel(self):
            self.was_cancelled = True

    timed_out_future = TimedOutFuture()

    def run_coroutine_threadsafe(coroutine, event_loop):
        coroutine.close()
        return timed_out_future

    monkeypatch.setattr(mcp_tool_call_conn.asyncio, "run_coroutine_threadsafe", run_coroutine_threadsafe)

    session = object.__new__(MCPToolCallSession)
    session._close = False
    session._event_loop = object()
    session._mcp_server = SimpleNamespace(id="server-1")
    return session, timed_out_future


def test_tool_call_timeout_preserves_message_and_cancels_coroutine(timed_out_session):
    session, timed_out_future = timed_out_session

    assert session.tool_call("send_email", {}, timeout=3) == "Timeout calling tool 'send_email' (timeout=3)."
    assert timed_out_future.was_cancelled


def test_get_tools_timeout_preserves_error_and_cancels_coroutine(timed_out_session):
    session, timed_out_future = timed_out_session

    with pytest.raises(RuntimeError, match="Timeout when fetching tools from MCP server: server-1"):
        session.get_tools(timeout=3)
    assert timed_out_future.was_cancelled


def test_public_tool_call_uses_one_absolute_deadline(monkeypatch):
    timeout = 0.6
    loop_delay = 0.3
    tool_delay = 0.45

    loop = asyncio.new_event_loop()
    loop_thread = threading.Thread(target=loop.run_forever, daemon=True)
    loop_thread.start()
    real_run_coroutine_threadsafe = asyncio.run_coroutine_threadsafe

    session = object.__new__(MCPToolCallSession)
    session._queue = asyncio.Queue()
    session._close = False
    session._event_loop = loop
    session._mcp_server = SimpleNamespace(id="server-1")

    tool_started = threading.Event()
    tool_cancelled = threading.Event()
    side_effect = threading.Event()

    class ClientSession:
        async def call_tool(self, name, arguments):
            tool_started.set()
            try:
                await asyncio.sleep(tool_delay)
                side_effect.set()
                return SimpleNamespace(isError=False, content=[])
            except asyncio.CancelledError:
                tool_cancelled.set()
                raise

    processor = real_run_coroutine_threadsafe(session._process_mcp_tasks(ClientSession()), loop)
    loop_blocked = threading.Event()
    release_loop = threading.Event()

    def block_event_loop():
        loop_blocked.set()
        release_loop.wait(timeout=2)

    loop.call_soon_threadsafe(block_event_loop)
    assert loop_blocked.wait(timeout=1)

    public_call_scheduled = threading.Event()

    def observed_run_coroutine_threadsafe(coroutine, target_loop):
        future = real_run_coroutine_threadsafe(coroutine, target_loop)
        public_call_scheduled.set()
        return future

    monkeypatch.setattr(mcp_tool_call_conn.asyncio, "run_coroutine_threadsafe", observed_run_coroutine_threadsafe)

    caller_pool = ThreadPoolExecutor(max_workers=1)
    caller = caller_pool.submit(session.tool_call, "side_effect", {}, timeout)
    result = None
    cancelled_seen = False

    try:
        assert public_call_scheduled.wait(timeout=1)
        time.sleep(loop_delay)
        release_loop.set()

        result = caller.result(timeout=2)
        cancelled_seen = tool_cancelled.wait(timeout=1)
    finally:
        release_loop.set()
        caller_pool.shutdown(wait=True)

        async def cancel_pending_tasks():
            session._close = True
            current = asyncio.current_task()
            tasks = [task for task in asyncio.all_tasks() if task is not current]
            for task in tasks:
                task.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)

        cleanup = real_run_coroutine_threadsafe(cancel_pending_tasks(), loop)
        cleanup.result(timeout=2)
        loop.call_soon_threadsafe(loop.stop)
        loop_thread.join(timeout=2)
        assert not loop_thread.is_alive()
        loop.close()

    assert result == "Timeout calling tool 'side_effect' (timeout=0.6)."
    assert tool_started.is_set()
    assert cancelled_seen
    assert not side_effect.is_set()
    assert processor.done()

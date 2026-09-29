import asyncio
import threading
import time
from concurrent.futures import CancelledError as FuturesCancelledError
from concurrent.futures import ThreadPoolExecutor
from concurrent.futures import TimeoutError as FuturesTimeoutError
from contextlib import asynccontextmanager
from types import SimpleNamespace

import pytest

from common import mcp_tool_call_conn
from common.constants import MCPServerType
from common.mcp_tool_call_conn import MCPToolCallSession


def _make_session() -> MCPToolCallSession:
    session = object.__new__(MCPToolCallSession)
    session._queue = asyncio.Queue()
    session._close = False
    session._pending_calls = {}
    session._shutdown_lock = threading.Lock()
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


@pytest.mark.parametrize("server_type", [MCPServerType.SSE, MCPServerType.STREAMABLE_HTTP])
def test_close_sync_waits_for_transport_context_cleanup(monkeypatch, server_type):
    initialized = threading.Event()
    client_closed = threading.Event()
    transport_closed = threading.Event()

    @asynccontextmanager
    async def fake_sse_client(url, headers):
        try:
            yield object(), object()
        finally:
            transport_closed.set()

    @asynccontextmanager
    async def fake_streamable_http_client(url, headers):
        try:
            yield object(), object(), None
        finally:
            transport_closed.set()

    class FakeClientSession:
        def __init__(self, *streams):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, traceback):
            client_closed.set()

        async def initialize(self):
            initialized.set()

    monkeypatch.setattr(mcp_tool_call_conn, "sse_client", fake_sse_client)
    monkeypatch.setattr(mcp_tool_call_conn, "streamablehttp_client", fake_streamable_http_client)
    monkeypatch.setattr(mcp_tool_call_conn, "ClientSession", FakeClientSession)

    server = SimpleNamespace(id="server-1", url="http://mcp.test", headers={}, server_type=server_type)
    session = MCPToolCallSession(server)

    try:
        assert initialized.wait(timeout=1)
        session.close_sync(timeout=1)

        assert client_closed.is_set()
        assert transport_closed.is_set()
        assert not session._event_loop.is_running()
        assert session._event_loop.is_closed()
        assert not asyncio.all_tasks(session._event_loop)
        assert session not in MCPToolCallSession._ALL_INSTANCES
    finally:
        session.close_sync(timeout=1)


def test_close_sync_cancels_blocked_initialization(monkeypatch):
    initialize_started = threading.Event()
    initialize_cancelled = threading.Event()
    client_closed = threading.Event()
    transport_closed = threading.Event()

    @asynccontextmanager
    async def fake_sse_client(url, headers):
        try:
            yield object(), object()
        finally:
            transport_closed.set()

    class FakeClientSession:
        def __init__(self, *streams):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, traceback):
            client_closed.set()

        async def initialize(self):
            initialize_started.set()
            try:
                await asyncio.Future()
            except asyncio.CancelledError:
                initialize_cancelled.set()
                raise

    monkeypatch.setattr(mcp_tool_call_conn, "sse_client", fake_sse_client)
    monkeypatch.setattr(mcp_tool_call_conn, "ClientSession", FakeClientSession)

    server = SimpleNamespace(id="server-1", url="http://mcp.test", headers={}, server_type=MCPServerType.SSE)
    session = MCPToolCallSession(server)

    try:
        assert initialize_started.wait(timeout=1)
        session.close_sync(timeout=1)

        assert initialize_cancelled.is_set()
        assert client_closed.is_set()
        assert transport_closed.is_set()
        assert session._event_loop.is_closed()
    finally:
        session.close_sync(timeout=1)


def test_async_close_waits_for_thread_exit(monkeypatch):
    server_started = threading.Event()
    finalized = threading.Event()

    async def fake_server_loop(self):
        server_started.set()
        try:
            await asyncio.Future()
        finally:
            finalized.set()

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))

    try:
        assert server_started.wait(timeout=1)
        asyncio.run(session.close())

        assert finalized.is_set()
        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert session not in MCPToolCallSession._ALL_INSTANCES
    finally:
        session.close_sync(timeout=1)


def test_async_close_runs_on_session_event_loop(monkeypatch):
    server_started = threading.Event()
    finalized = threading.Event()

    async def fake_server_loop(self):
        server_started.set()
        try:
            await asyncio.Future()
        finally:
            finalized.set()

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))

    try:
        assert server_started.wait(timeout=1)
        close_future = asyncio.run_coroutine_threadsafe(session.close(), session._event_loop)
        close_future.result(timeout=1)
        session._thread.join(timeout=1)

        assert finalized.is_set()
        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert session not in MCPToolCallSession._ALL_INSTANCES
    finally:
        session.close_sync(timeout=1)


def test_late_owner_close_is_cancelled_when_event_loop_stops(monkeypatch):
    server_started = threading.Event()
    stop_scheduled = threading.Event()
    release_stop_scheduler = threading.Event()

    async def fake_server_loop(self):
        server_started.set()
        await asyncio.Future()

    original_schedule_stop = MCPToolCallSession._schedule_event_loop_stop

    def blocking_schedule_stop(self):
        original_schedule_stop(self)
        stop_scheduled.set()
        release_stop_scheduler.wait(timeout=1)

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)
    monkeypatch.setattr(MCPToolCallSession, "_schedule_event_loop_stop", blocking_schedule_stop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))
    late_close = None

    try:
        assert server_started.wait(timeout=1)
        shutdown_future = session._begin_shutdown()
        shutdown_future.result(timeout=1)
        assert stop_scheduled.wait(timeout=1)

        late_close = asyncio.run_coroutine_threadsafe(session.close(), session._event_loop)
        release_stop_scheduler.set()

        with pytest.raises(FuturesCancelledError):
            late_close.result(timeout=1)
        session._thread.join(timeout=1)
        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert not asyncio.all_tasks(session._event_loop)
    finally:
        release_stop_scheduler.set()
        if late_close is not None and not late_close.done():
            late_close.cancel()
        session.close_sync(timeout=1)


def test_owner_close_queued_by_stop_callback_is_drained(monkeypatch):
    server_started = threading.Event()
    late_close_submitted = threading.Event()
    late_close_holder = {}

    async def fake_server_loop(self):
        server_started.set()
        await asyncio.Future()

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))
    original_stop = session._event_loop.stop
    submitted = False

    def submit_close_then_stop():
        nonlocal submitted
        if not submitted:
            submitted = True
            late_close_holder["future"] = asyncio.run_coroutine_threadsafe(session.close(), session._event_loop)
            late_close_submitted.set()
        original_stop()

    monkeypatch.setattr(session._event_loop, "stop", submit_close_then_stop)

    try:
        assert server_started.wait(timeout=1)
        session.close_sync(timeout=1)
        assert late_close_submitted.is_set()

        late_close = late_close_holder["future"]
        with pytest.raises(FuturesCancelledError):
            late_close.result(timeout=1)
        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert not asyncio.all_tasks(session._event_loop)
    finally:
        session.close_sync(timeout=1)


def test_close_sync_propagates_transport_cleanup_error(monkeypatch, caplog):
    initialized = threading.Event()
    join_attempted = threading.Event()

    @asynccontextmanager
    async def fake_sse_client(url, headers):
        try:
            yield object(), object()
        finally:
            raise RuntimeError("transport cleanup failed")

    class FakeClientSession:
        def __init__(self, *streams):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, traceback):
            pass

        async def initialize(self):
            initialized.set()

    monkeypatch.setattr(mcp_tool_call_conn, "sse_client", fake_sse_client)
    monkeypatch.setattr(mcp_tool_call_conn, "ClientSession", FakeClientSession)

    server = SimpleNamespace(id="server-1", url="http://mcp.test", headers={}, server_type=MCPServerType.SSE)
    session = MCPToolCallSession(server)
    real_join = session._thread.join

    def failing_join(timeout=None):
        join_attempted.set()
        raise ValueError("join failed")

    monkeypatch.setattr(session._thread, "join", failing_join)

    try:
        assert initialized.wait(timeout=1)
        with caplog.at_level("ERROR"), pytest.raises(RuntimeError, match="transport cleanup failed"):
            session.close_sync(timeout=1)

        assert join_attempted.is_set()
        assert "Exception while joining MCP session thread for server server-1" in caplog.messages
        join_record = next(record for record in caplog.records if record.getMessage() == "Exception while joining MCP session thread for server server-1")
        assert join_record.exc_info is not None
        assert join_record.exc_info[0] is ValueError
        assert str(join_record.exc_info[1]) == "join failed"

        monkeypatch.setattr(session._thread, "join", real_join)
        assert session._shutdown_complete.wait(timeout=1)
        real_join(timeout=1)
        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert session not in MCPToolCallSession._ALL_INSTANCES
    finally:
        monkeypatch.setattr(session._thread, "join", real_join)
        if session._thread.is_alive():
            try:
                session.close_sync(timeout=1)
            except RuntimeError as error:
                if str(error) != "transport cleanup failed":
                    raise


def test_close_sync_propagates_thread_join_error(monkeypatch):
    server_started = threading.Event()

    async def fake_server_loop(self):
        server_started.set()
        await asyncio.Future()

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))
    real_join = session._thread.join

    def failing_join(timeout=None):
        raise ValueError("join failed")

    monkeypatch.setattr(session._thread, "join", failing_join)

    try:
        assert server_started.wait(timeout=1)
        with pytest.raises(ValueError, match="join failed"):
            session.close_sync(timeout=1)
    finally:
        monkeypatch.setattr(session._thread, "join", real_join)
        session.close_sync(timeout=1)


def test_close_sync_cancels_in_flight_call_and_wakes_caller(monkeypatch):
    call_started = threading.Event()
    call_finalized = threading.Event()

    class FakeClientSession:
        async def call_tool(self, name, arguments):
            call_started.set()
            try:
                await asyncio.Future()
            finally:
                call_finalized.set()

    async def fake_server_loop(self):
        await self._process_mcp_tasks(FakeClientSession())

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    server = SimpleNamespace(id="server-1")
    session = MCPToolCallSession(server)
    caller_pool = ThreadPoolExecutor(max_workers=1)
    caller = caller_pool.submit(session.tool_call, "slow", {}, 3)

    try:
        assert call_started.wait(timeout=1)
        session.close_sync(timeout=1)

        result = caller.result(timeout=1)
        assert "Session is closing" in result
        assert "Timeout" not in result
        assert call_finalized.is_set()
    finally:
        session.close_sync(timeout=1)
        caller_pool.shutdown(wait=True)


def test_close_sync_wakes_in_flight_get_tools(monkeypatch):
    call_started = threading.Event()
    call_finalized = threading.Event()

    class FakeClientSession:
        async def list_tools(self):
            call_started.set()
            try:
                await asyncio.Future()
            finally:
                call_finalized.set()

    async def fake_server_loop(self):
        await self._process_mcp_tasks(FakeClientSession())

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))
    caller_pool = ThreadPoolExecutor(max_workers=1)
    caller = caller_pool.submit(session.get_tools, 3)

    try:
        assert call_started.wait(timeout=1)
        session.close_sync(timeout=1)

        with pytest.raises(ValueError, match="Session is closing"):
            caller.result(timeout=1)
        assert call_finalized.is_set()
    finally:
        session.close_sync(timeout=1)
        caller_pool.shutdown(wait=True)


def test_close_sync_wakes_queued_call_without_dispatching_it(monkeypatch):
    first_started = threading.Event()
    first_finalized = threading.Event()
    queued_enqueued = threading.Event()
    calls = []
    queue_type = asyncio.Queue

    class ObservedQueue(queue_type):
        async def put(self, item):
            await super().put(item)
            if isinstance(item, tuple) and len(item) > 1 and item[1].get("name") == "queued":
                queued_enqueued.set()

    class FakeClientSession:
        async def call_tool(self, name, arguments):
            calls.append(name)
            if name == "first":
                first_started.set()
                try:
                    await asyncio.Future()
                finally:
                    first_finalized.set()
            return SimpleNamespace(isError=False, content=[])

    async def fake_server_loop(self):
        await self._process_mcp_tasks(FakeClientSession())

    monkeypatch.setattr(mcp_tool_call_conn.asyncio, "Queue", ObservedQueue)
    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))
    caller_pool = ThreadPoolExecutor(max_workers=2)
    first = caller_pool.submit(session.tool_call, "first", {}, 3)
    queued = None

    try:
        assert first_started.wait(timeout=1)
        queued = caller_pool.submit(session.tool_call, "queued", {}, 3)
        assert queued_enqueued.wait(timeout=1)

        session.close_sync(timeout=1)

        assert "Session is closing" in first.result(timeout=1)
        assert "Session is closing" in queued.result(timeout=1)
        assert calls == ["first"]
        assert first_finalized.is_set()
    finally:
        session.close_sync(timeout=1)
        caller_pool.shutdown(wait=True)


def test_concurrent_close_is_idempotent(monkeypatch, caplog):
    server_started = threading.Event()
    finalized = threading.Event()
    finalizer_count = 0
    count_lock = threading.Lock()

    async def fake_server_loop(self):
        nonlocal finalizer_count
        server_started.set()
        try:
            await asyncio.Future()
        finally:
            with count_lock:
                finalizer_count += 1
            finalized.set()

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))
    barrier = threading.Barrier(4)

    def close_at_barrier():
        barrier.wait(timeout=1)
        session.close_sync(timeout=1)

    closer_pool = ThreadPoolExecutor(max_workers=3)
    closers = [closer_pool.submit(close_at_barrier) for _ in range(3)]

    try:
        assert server_started.wait(timeout=1)
        with caplog.at_level("INFO"):
            barrier.wait(timeout=1)
            for closer in closers:
                closer.result(timeout=2)

            session.close_sync(timeout=1)
        assert finalized.is_set()
        assert finalizer_count == 1
        assert caplog.messages.count("Closing MCP session for server server-1") == 1
        assert session.tool_call("after-close", {}, timeout=1) == "Error: Session is closed"
        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert session not in MCPToolCallSession._ALL_INSTANCES
    finally:
        session.close_sync(timeout=1)
        closer_pool.shutdown(wait=True)


@pytest.mark.parametrize("operation", ["tool_call", "get_tools"])
def test_close_waits_for_public_call_submission(monkeypatch, operation):
    server_started = threading.Event()
    submission_started = threading.Event()
    release_submission = threading.Event()
    close_started = threading.Event()

    async def fake_server_loop(self):
        server_started.set()
        await asyncio.Future()

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))
    real_run_coroutine_threadsafe = asyncio.run_coroutine_threadsafe
    first_submission = True

    def blocking_first_submission(coroutine, event_loop):
        nonlocal first_submission
        if first_submission:
            first_submission = False
            submission_started.set()
            release_submission.wait(timeout=1)
        return real_run_coroutine_threadsafe(coroutine, event_loop)

    monkeypatch.setattr(mcp_tool_call_conn.asyncio, "run_coroutine_threadsafe", blocking_first_submission)

    caller_pool = ThreadPoolExecutor(max_workers=2)
    if operation == "tool_call":
        caller = caller_pool.submit(session.tool_call, "queued", {}, 3)
    else:
        caller = caller_pool.submit(session.get_tools, 3)

    def close_session():
        close_started.set()
        session.close_sync(timeout=1)

    closer = None
    try:
        assert server_started.wait(timeout=1)
        assert submission_started.wait(timeout=1)
        closer = caller_pool.submit(close_session)
        assert close_started.wait(timeout=1)
        assert not session._close

        release_submission.set()
        closer.result(timeout=2)

        if operation == "tool_call":
            assert "Session is clos" in caller.result(timeout=1)
        else:
            with pytest.raises(ValueError, match="Session is clos"):
                caller.result(timeout=1)
        assert session._event_loop.is_closed()
    finally:
        release_submission.set()
        if closer is not None:
            closer.result(timeout=2)
        session.close_sync(timeout=1)
        caller_pool.shutdown(wait=True)


def test_close_timeout_keeps_shared_shutdown_running(monkeypatch):
    server_started = threading.Event()
    finalizer_started = threading.Event()
    finalizer_finished = threading.Event()
    release_finalizer = asyncio.Event()

    async def fake_server_loop(self):
        server_started.set()
        try:
            await asyncio.Future()
        finally:
            finalizer_started.set()
            await release_finalizer.wait()
            finalizer_finished.set()

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))

    try:
        assert server_started.wait(timeout=1)
        session.close_sync(timeout=0.01)

        assert finalizer_started.wait(timeout=1)
        assert not session._shutdown_future.done()
        assert session._thread.is_alive()
        assert session in MCPToolCallSession._ALL_INSTANCES

        shutdown_future = session._shutdown_future
        session._event_loop.call_soon_threadsafe(release_finalizer.set)
        assert session._shutdown_complete.wait(timeout=1)
        session._thread.join(timeout=1)

        assert session._shutdown_future is shutdown_future
        assert finalizer_finished.is_set()
        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert session not in MCPToolCallSession._ALL_INSTANCES
    finally:
        if not session._event_loop.is_closed():
            session._event_loop.call_soon_threadsafe(release_finalizer.set)
        session.close_sync(timeout=1)


@pytest.mark.parametrize("owner_loop", [False, True], ids=["external-loop", "owner-loop"])
def test_async_close_timeout_keeps_shared_shutdown_running(monkeypatch, owner_loop):
    server_started = threading.Event()
    finalizer_started = threading.Event()
    finalizer_finished = threading.Event()
    release_finalizer = asyncio.Event()

    async def fake_server_loop(self):
        server_started.set()
        try:
            await asyncio.Future()
        finally:
            finalizer_started.set()
            await release_finalizer.wait()
            finalizer_finished.set()

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))

    try:
        assert server_started.wait(timeout=1)
        if owner_loop:
            close_future = asyncio.run_coroutine_threadsafe(session.close(timeout=0.01), session._event_loop)
            close_future.result(timeout=1)
        else:
            asyncio.run(asyncio.wait_for(session.close(timeout=0.01), timeout=1))

        assert finalizer_started.wait(timeout=1)
        shutdown_future = session._shutdown_future
        assert shutdown_future is not None
        assert not shutdown_future.done()
        assert not shutdown_future.cancelled()
        assert session._owner_close_waiters == 0
        assert session._thread.is_alive()
        assert session in MCPToolCallSession._ALL_INSTANCES

        session._event_loop.call_soon_threadsafe(release_finalizer.set)
        assert session._shutdown_complete.wait(timeout=1)
        session._thread.join(timeout=1)

        assert session._shutdown_future is shutdown_future
        assert finalizer_finished.is_set()
        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert session not in MCPToolCallSession._ALL_INSTANCES
    finally:
        if not session._event_loop.is_closed():
            session._event_loop.call_soon_threadsafe(release_finalizer.set)
        session.close_sync(timeout=1)


def test_async_close_timeout_bounds_thread_join(monkeypatch):
    server_started = threading.Event()
    stop_scheduled = threading.Event()
    release_stop_scheduler = threading.Event()
    join_attempted = threading.Event()
    join_timeouts = []

    async def fake_server_loop(self):
        server_started.set()
        await asyncio.Future()

    original_schedule_stop = MCPToolCallSession._schedule_event_loop_stop

    def blocking_schedule_stop(self):
        stop_scheduled.set()
        release_stop_scheduler.wait(timeout=5)
        original_schedule_stop(self)

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)
    monkeypatch.setattr(MCPToolCallSession, "_schedule_event_loop_stop", blocking_schedule_stop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))
    real_join = session._thread.join

    def observed_join(timeout=None):
        join_timeouts.append(timeout)
        join_attempted.set()
        real_join(timeout)

    monkeypatch.setattr(session._thread, "join", observed_join)

    try:
        assert server_started.wait(timeout=1)
        shutdown_future = session._begin_shutdown()
        shutdown_future.result(timeout=1)
        assert stop_scheduled.wait(timeout=1)

        asyncio.run(asyncio.wait_for(session.close(timeout=0.5), timeout=1))

        assert join_attempted.is_set()
        assert 0 < join_timeouts[0] <= 0.5
        assert session._shutdown_future is shutdown_future
        assert session._thread.is_alive()

        release_stop_scheduler.set()
        assert session._shutdown_complete.wait(timeout=1)
        real_join(timeout=1)
        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert session not in MCPToolCallSession._ALL_INSTANCES
    finally:
        monkeypatch.setattr(session._thread, "join", real_join)
        release_stop_scheduler.set()
        session.close_sync(timeout=1)


def test_async_close_propagates_transport_cleanup_error(monkeypatch, caplog):
    server_started = threading.Event()
    release_stop_scheduler = threading.Event()
    join_attempted = threading.Event()

    async def fake_server_loop(self):
        server_started.set()
        try:
            await asyncio.Future()
        finally:
            raise RuntimeError("transport cleanup failed")

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))
    real_join = session._thread.join

    def blocking_schedule_stop():
        release_stop_scheduler.wait(timeout=5)
        session._event_loop.stop()

    def failing_join(timeout=None):
        join_attempted.set()
        raise ValueError("join failed")

    monkeypatch.setattr(session, "_schedule_event_loop_stop", blocking_schedule_stop)
    monkeypatch.setattr(session._thread, "join", failing_join)

    try:
        assert server_started.wait(timeout=1)
        with caplog.at_level("ERROR"), pytest.raises(RuntimeError, match="transport cleanup failed"):
            asyncio.run(asyncio.wait_for(session.close(timeout=1), timeout=2))

        assert join_attempted.is_set()
        assert "Exception while joining MCP session thread for server server-1" in caplog.messages

        monkeypatch.setattr(session._thread, "join", real_join)
        release_stop_scheduler.set()
        assert session._shutdown_complete.wait(timeout=1)
        real_join(timeout=1)

        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert session not in MCPToolCallSession._ALL_INSTANCES
    finally:
        monkeypatch.setattr(session._thread, "join", real_join)
        release_stop_scheduler.set()
        if session._thread.is_alive():
            try:
                session.close_sync(timeout=1)
            except RuntimeError as error:
                if str(error) != "transport cleanup failed":
                    raise


def test_async_close_propagates_thread_join_error(monkeypatch):
    server_started = threading.Event()
    release_stop_scheduler = threading.Event()

    async def fake_server_loop(self):
        server_started.set()
        await asyncio.Future()

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))
    real_join = session._thread.join

    def blocking_schedule_stop():
        release_stop_scheduler.wait(timeout=5)
        session._event_loop.stop()

    def failing_join(timeout=None):
        raise ValueError("join failed")

    monkeypatch.setattr(session, "_schedule_event_loop_stop", blocking_schedule_stop)
    monkeypatch.setattr(session._thread, "join", failing_join)

    try:
        assert server_started.wait(timeout=1)
        with pytest.raises(ValueError, match="join failed"):
            asyncio.run(asyncio.wait_for(session.close(timeout=1), timeout=2))
    finally:
        monkeypatch.setattr(session._thread, "join", real_join)
        release_stop_scheduler.set()
        session.close_sync(timeout=1)


def test_close_keeps_session_active_until_default_executor_finishes(monkeypatch):
    worker_started = threading.Event()
    release_worker = threading.Event()
    worker_finished = threading.Event()
    worker_holder = {}

    def blocking_worker():
        worker_holder["thread"] = threading.current_thread()
        worker_started.set()
        release_worker.wait(timeout=2)
        worker_finished.set()

    async def fake_server_loop(self):
        await asyncio.get_running_loop().run_in_executor(None, blocking_worker)

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))

    try:
        assert worker_started.wait(timeout=1)
        session.close_sync(timeout=0.01)

        assert session._thread.is_alive()
        assert worker_holder["thread"].is_alive()
        assert not session._shutdown_complete.is_set()
        assert session in MCPToolCallSession._ALL_INSTANCES

        release_worker.set()
        session.close_sync(timeout=1)

        assert worker_finished.is_set()
        assert not worker_holder["thread"].is_alive()
        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert session not in MCPToolCallSession._ALL_INSTANCES
    finally:
        release_worker.set()
        session.close_sync(timeout=1)


def test_transport_cleanup_error_waits_for_default_executor(monkeypatch):
    server_started = threading.Event()
    worker_started = threading.Event()
    release_worker = threading.Event()
    worker_finished = threading.Event()
    worker_holder = {}

    def blocking_worker():
        worker_holder["thread"] = threading.current_thread()
        worker_started.set()
        release_worker.wait(timeout=2)
        worker_finished.set()

    async def fake_server_loop(self):
        asyncio.get_running_loop().run_in_executor(None, blocking_worker)
        server_started.set()
        try:
            await asyncio.Future()
        finally:
            raise RuntimeError("transport cleanup failed")

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))

    try:
        assert server_started.wait(timeout=1)
        assert worker_started.wait(timeout=1)
        session.close_sync(timeout=0.01)

        assert session._thread.is_alive()
        assert worker_holder["thread"].is_alive()
        assert not session._shutdown_complete.is_set()
        assert session in MCPToolCallSession._ALL_INSTANCES

        release_worker.set()
        with pytest.raises(RuntimeError, match="transport cleanup failed"):
            session.close_sync(timeout=1)

        assert worker_finished.is_set()
        assert not worker_holder["thread"].is_alive()
        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert session not in MCPToolCallSession._ALL_INSTANCES
    finally:
        release_worker.set()
        if session._thread.is_alive():
            try:
                session.close_sync(timeout=1)
            except RuntimeError as error:
                assert str(error) == "transport cleanup failed"


def test_close_multiple_deduplicates_sessions_and_waits_for_cleanup(monkeypatch):
    started = {"server-1": threading.Event(), "server-2": threading.Event()}
    finalized = {"server-1": threading.Event(), "server-2": threading.Event()}
    finalizer_counts = {"server-1": 0, "server-2": 0}

    async def fake_server_loop(self):
        server_id = self._mcp_server.id
        started[server_id].set()
        try:
            await asyncio.Future()
        finally:
            finalizer_counts[server_id] += 1
            finalized[server_id].set()

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    first = MCPToolCallSession(SimpleNamespace(id="server-1"))
    second = MCPToolCallSession(SimpleNamespace(id="server-2"))

    try:
        assert all(event.wait(timeout=1) for event in started.values())
        mcp_tool_call_conn.close_multiple_mcp_toolcall_sessions([first, None, first, second])

        assert all(event.is_set() for event in finalized.values())
        assert finalizer_counts == {"server-1": 1, "server-2": 1}
        assert all(session._event_loop.is_closed() for session in (first, second))
        assert all(session not in MCPToolCallSession._ALL_INSTANCES for session in (first, second))
    finally:
        first.close_sync(timeout=1)
        second.close_sync(timeout=1)


def test_close_multiple_rejects_session_event_loop_thread(monkeypatch):
    server_started = threading.Event()

    async def fake_server_loop(self):
        server_started.set()
        await asyncio.Future()

    async def close_from_owner_loop(session):
        mcp_tool_call_conn.close_multiple_mcp_toolcall_sessions([session])

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)

    session = MCPToolCallSession(SimpleNamespace(id="server-1"))

    try:
        assert server_started.wait(timeout=1)
        future = asyncio.run_coroutine_threadsafe(close_from_owner_loop(session), session._event_loop)

        with pytest.raises(RuntimeError, match="outside their event loop threads"):
            future.result(timeout=1)
        assert not session._close
    finally:
        session.close_sync(timeout=1)


def test_constructor_preserves_error_when_thread_start_fails_after_start(monkeypatch):
    class StartupInterrupted(BaseException):
        pass

    interruption = StartupInterrupted("startup interrupted")
    loop_started = threading.Event()
    loop_holder = {}
    thread_holder = {}
    session_holder = {}
    instances_before = set(MCPToolCallSession._active_instances())
    real_new_event_loop = asyncio.new_event_loop
    real_thread_start = threading.Thread.start

    def observed_new_event_loop():
        event_loop = real_new_event_loop()
        loop_holder["event_loop"] = event_loop
        return event_loop

    def start_then_fail(thread):
        thread_holder["thread"] = thread
        session_holder["session"] = thread._target.__self__
        real_thread_start(thread)
        loop_holder["event_loop"].call_soon_threadsafe(loop_started.set)
        assert loop_started.wait(timeout=1)
        raise interruption

    monkeypatch.setattr(mcp_tool_call_conn.asyncio, "new_event_loop", observed_new_event_loop)
    monkeypatch.setattr(threading.Thread, "start", start_then_fail)

    with pytest.raises(StartupInterrupted) as raised:
        MCPToolCallSession(SimpleNamespace(id="startup-failure"))

    session = session_holder["session"]
    assert raised.value is interruption
    assert not thread_holder["thread"].is_alive()
    assert loop_holder["event_loop"].is_closed()
    assert session._shutdown_complete.is_set()
    assert session not in MCPToolCallSession._ALL_INSTANCES
    assert set(MCPToolCallSession._active_instances()) == instances_before


def test_constructor_shutdowns_default_executor_when_registration_fails(monkeypatch):
    class StartupInterrupted(BaseException):
        pass

    interruption = StartupInterrupted("startup interrupted")
    worker_started = threading.Event()
    release_worker = threading.Event()
    worker_finished = threading.Event()
    session_holder = {}
    worker_holder = {}
    instances_before = set(MCPToolCallSession._active_instances())
    real_registry_add = MCPToolCallSession._ALL_INSTANCES.add

    def blocking_worker():
        worker_holder["thread"] = threading.current_thread()
        worker_started.set()
        release_worker.wait(timeout=5)
        worker_finished.set()

    async def fake_server_loop(self):
        session_holder["session"] = self
        await asyncio.get_running_loop().run_in_executor(None, blocking_worker)
        await asyncio.Future()

    def fail_registration(session):
        assert worker_started.wait(timeout=1)
        raise interruption

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)
    monkeypatch.setattr(MCPToolCallSession._ALL_INSTANCES, "add", fail_registration)

    pool = ThreadPoolExecutor(max_workers=1)
    constructor = pool.submit(MCPToolCallSession, SimpleNamespace(id="startup-failure"))

    try:
        assert worker_started.wait(timeout=1)
        with pytest.raises(FuturesTimeoutError):
            constructor.result(timeout=0.05)

        release_worker.set()
        with pytest.raises(StartupInterrupted) as raised:
            constructor.result(timeout=2)

        session = session_holder["session"]
        assert raised.value is interruption
        assert worker_finished.is_set()
        assert not worker_holder["thread"].is_alive()
        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert session._shutdown_complete.is_set()
        assert session not in MCPToolCallSession._ALL_INSTANCES
        assert set(MCPToolCallSession._active_instances()) == instances_before
    finally:
        release_worker.set()
        try:
            constructor.result(timeout=2)
        except StartupInterrupted:
            pass
        finally:
            pool.shutdown(wait=True)
            monkeypatch.setattr(MCPToolCallSession._ALL_INSTANCES, "add", real_registry_add)


def test_shutdown_all_waits_for_session_registration(monkeypatch):
    server_started = threading.Event()
    submission_started = threading.Event()
    release_submission = threading.Event()
    shutdown_started = threading.Event()

    async def fake_server_loop(self):
        server_started.set()
        await asyncio.Future()

    real_run_coroutine_threadsafe = asyncio.run_coroutine_threadsafe

    def blocking_submission(coroutine, event_loop):
        future = real_run_coroutine_threadsafe(coroutine, event_loop)
        submission_started.set()
        release_submission.wait(timeout=5)
        return future

    real_begin_global_shutdown = MCPToolCallSession._begin_global_shutdown.__func__

    def observed_begin_global_shutdown(cls):
        shutdown_started.set()
        return real_begin_global_shutdown(cls)

    monkeypatch.setattr(MCPToolCallSession, "_mcp_server_loop", fake_server_loop)
    monkeypatch.setattr(mcp_tool_call_conn.asyncio, "run_coroutine_threadsafe", blocking_submission)
    monkeypatch.setattr(MCPToolCallSession, "_begin_global_shutdown", classmethod(observed_begin_global_shutdown))

    pool = ThreadPoolExecutor(max_workers=2)
    constructor = pool.submit(MCPToolCallSession, SimpleNamespace(id="server-1"))
    shutdown = None
    session = None

    try:
        assert submission_started.wait(timeout=1)
        shutdown = pool.submit(mcp_tool_call_conn.shutdown_all_mcp_sessions)
        assert shutdown_started.wait(timeout=1)
        with pytest.raises(FuturesTimeoutError):
            shutdown.result(timeout=0.05)

        release_submission.set()
        session = constructor.result(timeout=2)
        shutdown.result(timeout=2)

        assert server_started.is_set()
        assert session._shutdown_complete.is_set()
        assert not session._thread.is_alive()
        assert session._event_loop.is_closed()
        assert session not in MCPToolCallSession._ALL_INSTANCES
        assert MCPToolCallSession._GLOBAL_SHUTDOWN_COUNT == 0
    finally:
        release_submission.set()
        try:
            if session is None:
                session = constructor.result(timeout=2)
            if shutdown is not None:
                shutdown.result(timeout=2)
        finally:
            try:
                if session is not None and session._thread.is_alive():
                    session.close_sync(timeout=1)
            finally:
                pool.shutdown(wait=True)


def test_session_creation_is_rejected_during_global_shutdown(monkeypatch):
    shutdown_started = threading.Event()
    release_shutdown = threading.Event()
    active_instances = iter([[SimpleNamespace()], []])

    def blocking_close_multiple(sessions):
        shutdown_started.set()
        assert release_shutdown.wait(timeout=5)
        return 0

    monkeypatch.setattr(MCPToolCallSession, "_active_instances", classmethod(lambda cls: next(active_instances)))
    monkeypatch.setattr(mcp_tool_call_conn, "close_multiple_mcp_toolcall_sessions", blocking_close_multiple)

    pool = ThreadPoolExecutor(max_workers=1)
    shutdown = pool.submit(mcp_tool_call_conn.shutdown_all_mcp_sessions)

    try:
        assert shutdown_started.wait(timeout=1)
        with pytest.raises(RuntimeError, match="Cannot create MCP session during global shutdown"):
            MCPToolCallSession(SimpleNamespace(id="rejected-server"))
    finally:
        release_shutdown.set()
        try:
            shutdown.result(timeout=2)
        finally:
            pool.shutdown(wait=True)

    assert MCPToolCallSession._GLOBAL_SHUTDOWN_COUNT == 0


def test_shutdown_all_reports_sessions_still_shutting_down(monkeypatch, caplog):
    session = SimpleNamespace()

    monkeypatch.setattr(MCPToolCallSession, "_active_instances", classmethod(lambda cls: [session]))
    monkeypatch.setattr(mcp_tool_call_conn, "close_multiple_mcp_toolcall_sessions", lambda sessions: 0)

    with caplog.at_level("INFO"):
        mcp_tool_call_conn.shutdown_all_mcp_sessions()

    assert "1 MCPToolCallSession instances are still shutting down." in caplog.messages
    assert "All MCPToolCallSession instances have been closed." not in caplog.messages


def test_close_multiple_reports_transport_cleanup_error_once(caplog):
    class FailingSession:
        _thread = object()
        _mcp_server = SimpleNamespace(id="server-1")
        _shutdown_complete = threading.Event()

        def close_sync(self):
            self._shutdown_complete.set()
            raise RuntimeError("transport cleanup failed")

    with caplog.at_level("INFO"):
        cleanup_errors = mcp_tool_call_conn.close_multiple_mcp_toolcall_sessions([FailingSession()])

    assert cleanup_errors == 1
    assert caplog.messages.count("Exception while closing MCP session for server server-1") == 1
    assert any("1 MCP sessions stopped; transport cleanup failure count: 1" in message for message in caplog.messages)
    assert not any("MCP sessions have been cleaned up" in message for message in caplog.messages)


def test_shutdown_all_reports_transport_cleanup_errors(monkeypatch, caplog):
    active_instances = iter([[SimpleNamespace()], []])

    monkeypatch.setattr(MCPToolCallSession, "_active_instances", classmethod(lambda cls: next(active_instances)))
    monkeypatch.setattr(mcp_tool_call_conn, "close_multiple_mcp_toolcall_sessions", lambda sessions: 1)

    with caplog.at_level("INFO"):
        mcp_tool_call_conn.shutdown_all_mcp_sessions()

    assert "All MCPToolCallSession event loops stopped; transport cleanup failure count: 1." in caplog.messages
    assert "All MCPToolCallSession instances have been closed." not in caplog.messages


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
    session._shutdown_lock = threading.Lock()
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
    session._pending_calls = {}
    session._shutdown_lock = threading.Lock()
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

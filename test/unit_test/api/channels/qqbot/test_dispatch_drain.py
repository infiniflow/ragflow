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
"""
Tests for the in-flight dispatch drain in ``QQBotChannel._run_ws_thread``.

A QQ gateway event is handled by a task created with ``asyncio.create_task``,
and that task is suspended across a database write and a full model call before
it sends the reply. The channel therefore keeps a strong reference to every
such task and drains them when the websocket thread stops.

The drain is bounded by ``DISPATCH_DRAIN_TIMEOUT`` so a stuck reply cannot hold
the shutdown path open. What the timeout must not do is leave the task pending:
closing the loop underneath a pending task drops the reply silently and prints
``Task was destroyed but it is pending!``, and the task would stay in
``_dispatch_tasks`` into the next start, where it can never complete and makes
every later drain wait out the whole timeout.

Both tests drive the real payload path, so no gateway, database or model is
involved: only ``_run`` (the websocket read loop) and the message handler are
replaced.
"""

import asyncio
import json
import threading
import time

from api.channels.qqbot import channel as qqbot_channel


class _FakeWebSocket:
    closed = False

    async def close(self):
        self.closed = True


def _c2c_payload(seq: int) -> str:
    return json.dumps(
        {
            "op": 0,
            "s": seq,
            "t": "C2C_MESSAGE_CREATE",
            "d": {
                "id": f"msg-{seq}",
                "content": f"hello {seq}",
                "author": {"user_openid": f"user-{seq}"},
            },
        }
    )


class _HarnessChannel(qqbot_channel.QQBotChannel):
    """Feeds a fixed number of gateway events, then ends the read loop."""

    def __init__(self, account, message_count):
        super().__init__(account)
        self._message_count = message_count

    async def _run(self) -> None:
        ws = _FakeWebSocket()
        for seq in range(self._message_count):
            await self._handle_ws_payload(ws, _c2c_payload(seq), "token", 30000)


def _run_channel(channel, handler) -> float:
    """Run one websocket-thread lifecycle end to end and return its duration."""
    channel.set_message_handler(handler)
    thread = threading.Thread(target=channel._run_ws_thread, name="qqbot-test-ws")
    started = time.monotonic()
    thread.start()
    thread.join(30)
    assert not thread.is_alive(), "websocket thread did not finish"
    return time.monotonic() - started


def _account():
    return qqbot_channel.QQBotAccount(account_id="acct", app_id="app", client_secret="secret")


def test_dispatch_exceeding_the_drain_timeout_is_cancelled(monkeypatch):
    monkeypatch.setattr(qqbot_channel, "DISPATCH_DRAIN_TIMEOUT", 0.1)
    channel = _HarnessChannel(_account(), message_count=3)
    cancelled = []

    async def never_finishes(message):
        try:
            await asyncio.Event().wait()
        except asyncio.CancelledError:
            cancelled.append(message.message_id)
            raise

    _run_channel(channel, never_finishes)

    assert sorted(cancelled) == ["msg-0", "msg-1", "msg-2"]
    assert channel._dispatch_tasks == set()


def test_restart_after_a_timed_out_drain_keeps_delivering(monkeypatch):
    monkeypatch.setattr(qqbot_channel, "DISPATCH_DRAIN_TIMEOUT", 0.1)
    channel = _HarnessChannel(_account(), message_count=3)

    async def never_finishes(_message):
        await asyncio.Event().wait()

    _run_channel(channel, never_finishes)

    # Restart the same channel object. A leftover task from the closed loop
    # would make this drain wait out the full timeout without ever completing.
    monkeypatch.setattr(qqbot_channel, "DISPATCH_DRAIN_TIMEOUT", 5)
    delivered = []

    async def replies(message):
        delivered.append(message.message_id)

    elapsed = _run_channel(channel, replies)

    assert sorted(delivered) == ["msg-0", "msg-1", "msg-2"]
    assert elapsed < 2, f"shutdown waited {elapsed:.2f}s on tasks from the previous loop"
    assert channel._dispatch_tasks == set()

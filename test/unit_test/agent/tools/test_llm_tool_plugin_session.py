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
"""Verify that LLMToolPluginCallSession applies its configurable default timeout to MCP tool calls."""

import asyncio
import time
from functools import partial

from agent.tools.base import LLMToolPluginCallSession
from common.mcp_tool_call_conn import MCPToolBinding


class _BlockingSession:
    """Fake MCP session that waits until the caller-provided timeout elapses, mimicking MCPToolCallSession."""

    def tool_call(self, name: str, arguments: dict, timeout: float = 10) -> str:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            time.sleep(0.005)
        return "done"


def _noop_callback(*args, **kwargs):
    pass


def _make_session(default_timeout):
    return LLMToolPluginCallSession(
        {"transcribe_0": MCPToolBinding(_BlockingSession(), "transcribe")},
        partial(_noop_callback),
        default_timeout=default_timeout,
    )


def test_default_timeout_is_applied_to_mcp_tool_call():
    session = _make_session(default_timeout=0.1)
    start = time.monotonic()
    result = asyncio.run(session.tool_call_async("transcribe_0", {}))
    elapsed = time.monotonic() - start
    assert result == "done"
    # Without the session default the 10s tool_call default would run far longer.
    assert elapsed < 1.0


def test_explicit_request_timeout_overrides_default():
    session = _make_session(default_timeout=0.5)
    start = time.monotonic()
    result = asyncio.run(session.tool_call_async("transcribe_0", {}, request_timeout=0.05))
    elapsed = time.monotonic() - start
    assert result == "done"
    assert elapsed < 0.3

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
from functools import partial

from agent.tools.base import LLMToolPluginCallSession
from common.mcp_tool_call_conn import MCPToolBinding


class _RecordingSession:
    """Fake MCP session that records the timeout supplied by each call."""

    def __init__(self):
        self.timeouts = []

    def tool_call(self, name: str, arguments: dict, timeout: float = 10) -> str:
        self.timeouts.append(timeout)
        return "done"


def _noop_callback(*args, **kwargs):
    pass


def _make_session(default_timeout=None):
    recording = _RecordingSession()
    session = LLMToolPluginCallSession(
        {"transcribe_0": MCPToolBinding(recording, "transcribe")},
        partial(_noop_callback),
        default_timeout=default_timeout,
    )
    return session, recording


def test_default_timeout_is_applied_to_mcp_tool_call():
    session, recording = _make_session(default_timeout=3)
    result = asyncio.run(session.tool_call_async("transcribe_0", {}))
    assert result == "done"
    assert recording.timeouts == [3]


def test_explicit_request_timeout_overrides_default():
    session, recording = _make_session(default_timeout=5)
    result = asyncio.run(session.tool_call_async("transcribe_0", {}, request_timeout=2))
    assert result == "done"
    assert recording.timeouts == [2]


def test_constructor_default_timeout_is_ten_seconds():
    for configured in (None, 0, 0.5, -1):
        session, recording = _make_session(default_timeout=configured)
        result = asyncio.run(session.tool_call_async("transcribe_0", {}))
        assert result == "done"
        assert recording.timeouts == [10]


def test_tool_call_wrapper_uses_default_timeout():
    session, recording = _make_session(default_timeout=4)
    result = session.tool_call("transcribe_0", {})
    assert result == "done"
    assert recording.timeouts == [4]

from types import SimpleNamespace

import pytest

from common.constants import MCPServerType
from common.mcp_tool_call_conn import MCPToolCallSession


class _AsyncContext:
    def __init__(self, value):
        self.value = value

    async def __aenter__(self):
        return self.value

    async def __aexit__(self, *_args):
        return False


class _ClientSession:
    def __init__(self, *_streams):
        pass

    async def __aenter__(self):
        return self

    async def __aexit__(self, *_args):
        return False

    async def initialize(self):
        return None


@pytest.mark.asyncio
async def test_mcp_session_preserves_nonempty_header_values(monkeypatch):
    captured = {}

    def fake_sse_client(url, headers):
        captured["url"] = url
        captured["headers"] = headers
        return _AsyncContext(("read", "write"))

    session = MCPToolCallSession.__new__(MCPToolCallSession)
    session._mcp_server = SimpleNamespace(
        id="server-1",
        url="https://mcp.example/sse",
        headers={
            "X-Api-Key": "Bear",
            "X-Environment": "Bearer",
            "Authorization": "Bearer ",
        },
        server_type=MCPServerType.SSE,
    )
    session._server_variables = {}
    session._custom_header = None

    async def process_tasks(_client_session, _error_message=None):
        return None

    session._process_mcp_tasks = process_tasks

    monkeypatch.setattr("common.mcp_tool_call_conn.sse_client", fake_sse_client)
    monkeypatch.setattr("common.mcp_tool_call_conn.ClientSession", _ClientSession)

    await session._mcp_server_loop()

    assert captured["headers"] == {
        "X-Api-Key": "Bear",
        "X-Environment": "Bearer",
    }

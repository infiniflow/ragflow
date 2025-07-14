from api.db.db_models import MCPServer
from rag.utils.mcp_tool_call_conn import MCPToolCallSession, close_multiple_mcp_toolcall_sessions


def get_mcp_tools(mcp_servers: list[MCPServer], timeout: float | int = 10) -> tuple[dict, str]:
    results = {}
    tool_call_sessions = []
    try:
        for mcp_server in mcp_servers:
            server_key = mcp_server.id

            cached_tools = mcp_server.variables.get("tools", {})

            tool_call_session = MCPToolCallSession(mcp_server, mcp_server.variables)
            tool_call_sessions.append(tool_call_session)

            try:
                tools = tool_call_session.get_tools(timeout)
            except Exception:
                tools = []

            results[server_key] = []
            for tool in tools:
                tool_dict = tool.model_dump()
                cached_tool = cached_tools.get(tool_dict["name"], {})

                tool_dict["enabled"] = cached_tool.get("enabled", True)
                results[server_key].append(tool_dict)

        # PERF: blocking call to close sessions — consider moving to background thread or task queue
        close_multiple_mcp_toolcall_sessions(tool_call_sessions)
        return results, ""
    except Exception as e:
        return {}, str(e)

from flask import Response, request
from flask_login import current_user, login_required

from api.db import VALID_MCP_SERVER_TYPES
from api.db.db_models import MCPServer
from api.db.services.mcp_server_service import MCPServerService
from api.db.services.user_service import TenantService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import get_data_error_result, get_json_result, server_error_response, validate_request
from api.utils.web_utils import get_float, safe_json_parse
from rag.utils.mcp_tool_call_conn import MCPToolCallSession, close_multiple_mcp_toolcall_sessions


@manager.route("/list", methods=["POST"])  # noqa: F821
@login_required
def list_mcp() -> Response:
    keywords = request.args.get("keywords", "")
    page_number = int(request.args.get("page", 0))
    items_per_page = int(request.args.get("page_size", 0))
    orderby = request.args.get("orderby", "create_time")
    if request.args.get("desc", "true").lower() == "false":
        desc = False
    else:
        desc = True

    req = request.get_json()
    mcp_ids = req.get("mcp_ids", [])
    try:
        servers = MCPServerService.get_servers(current_user.id, mcp_ids, page_number, items_per_page, orderby, desc, keywords) or []

        return get_json_result(data={"mcp_servers": servers, "total": len(servers)})
    except Exception as e:
        return server_error_response(e)


@manager.route("/detail", methods=["GET"])  # noqa: F821
@login_required
def detail() -> Response:
    mcp_id = request.args["mcp_id"]
    try:
        mcp_server = MCPServerService.get_or_none(id=mcp_id, tenant_id=current_user.id)

        if mcp_server is None:
            return get_json_result(code=RetCode.NOT_FOUND, data=None)

        return get_json_result(data=mcp_server.to_dict())
    except Exception as e:
        return server_error_response(e)


@manager.route("/create", methods=["POST"])  # noqa: F821
@login_required
@validate_request("name", "url", "server_type")
def create() -> Response:
    req = request.get_json()

    server_type = req.get("server_type", "")
    if server_type not in VALID_MCP_SERVER_TYPES:
        return get_data_error_result(message="Unsupported MCP server type.")

    server_name = req.get("name", "")
    if not server_name or len(server_name.encode("utf-8")) > 255:
        return get_data_error_result(message=f"Invaild MCP name or length is {len(server_name)} which is large than 255.")

    req["headers"] = safe_json_parse(req.get("headers", {}))
    req["variables"] = safe_json_parse(req.get("variables", {}))

    try:
        req["id"] = get_uuid()
        req["tenant_id"] = current_user.id

        e, _ = TenantService.get_by_id(current_user.id)

        if not e:
            return get_data_error_result(message="Tenant not found.")

        if not MCPServerService.insert(**req):
            return get_data_error_result()

        return get_json_result(data={"id": req["id"]})
    except Exception as e:
        return server_error_response(e)


@manager.route("/update", methods=["POST"])  # noqa: F821
@login_required
@validate_request("mcp_id")
def update() -> Response:
    req = request.get_json()

    server_type = req.get("server_type", "")
    if server_type and server_type not in VALID_MCP_SERVER_TYPES:
        return get_data_error_result(message="Unsupported MCP server type.")
    server_name = req.get("name", "")
    if server_name and len(server_name.encode("utf-8")) > 255:
        return get_data_error_result(message=f"Invaild MCP name or length is {len(server_name)} which is large than 255.")

    mcp_id = req.get("mcp_id", "")
    e, mcp_server = MCPServerService.get_by_id(mcp_id)
    if not e or mcp_server.tenant_id != current_user.id:
        return get_data_error_result(message=f"Cannot find MCP server {mcp_id} for user {current_user.id}")

    req["headers"] = safe_json_parse(req.get("headers", mcp_server.headers))
    req["variables"] = safe_json_parse(req.get("variables", mcp_server.variables))

    try:
        req["tenant_id"] = current_user.id
        req.pop("mcp_id", None)
        req["id"] = mcp_id

        if not MCPServerService.filter_update([MCPServer.id == mcp_id, MCPServer.tenant_id == current_user.id], req):
            return get_data_error_result(message="Failed to updated MCP server.")

        e, updated_mcp = MCPServerService.get_by_id(req["id"])
        if not e:
            return get_data_error_result(message="Failed to fetch updated MCP server.")

        return get_json_result(data=updated_mcp.to_dict())
    except Exception as e:
        return server_error_response(e)


@manager.route("/rm", methods=["POST"])  # noqa: F821
@login_required
@validate_request("mcp_ids")
def rm() -> Response:
    req = request.get_json()
    mcp_ids = req.get("mcp_ids", [])

    try:
        req["tenant_id"] = current_user.id

        if not MCPServerService.delete_by_ids(mcp_ids):
            return get_data_error_result(message=f"Failed to delete MCP servers {mcp_ids}")

        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route("/import", methods=["POST"])  # noqa: F821
@login_required
@validate_request("mcpServers")
def import_multiple() -> Response:
    req = request.get_json()
    servers = req.get("mcpServers", {})

    if not servers:
        return get_data_error_result(message="No MCP servers provided.")

    results = []
    try:
        for server_name, config in servers.items():
            if not all(key in config for key in ["type", "url"]):
                results.append({"server": server_name, "success": False, "message": "Missing required fields (type or url)"})
                continue

            base_name = server_name
            new_name = base_name
            counter = 0

            while True:
                e, _ = MCPServerService.get_by_name_and_tenant(name=new_name, tenant_id=current_user.id)
                if not e:
                    break
                new_name = f"{base_name}_{counter}"
                counter += 1

            create_data = {
                "id": get_uuid(),
                "tenant_id": current_user.id,
                "name": new_name,
                "url": config["url"],
                "server_type": config["type"],
                "variables": {"authorization_token": config.get("authorization_token", ""), "tool_configuration": config.get("tool_configuration", {})},
            }

            if MCPServerService.insert(**create_data):
                result = {"server": server_name, "success": True, "action": "created", "id": create_data["id"], "new_name": new_name}
                if new_name != base_name:
                    result["message"] = f"Renamed from '{base_name}' to avoid duplication"

                results.append(result)
            else:
                results.append({"server": server_name, "success": False, "message": "Failed to create MCP server."})

        return get_json_result(data={"results": results})
    except Exception as e:
        return server_error_response(e)


@manager.route("/export", methods=["POST"])  # noqa: F821
@login_required
@validate_request("mcp_ids")
def export_multiple() -> Response:
    req = request.get_json()
    mcp_ids = req.get("mcp_ids", [])

    if not mcp_ids:
        return get_data_error_result(message="No MCP server IDs provided.")

    try:
        exported_servers = {}

        for mcp_id in mcp_ids:
            e, mcp_server = MCPServerService.get_by_id(mcp_id)

            if e and mcp_server.tenant_id == current_user.id:
                server_key = mcp_server.name

                exported_servers[server_key] = {
                    "type": mcp_server.server_type,
                    "url": mcp_server.url,
                    "name": mcp_server.name,
                    "authorization_token": mcp_server.variables.get("authorization_token", ""),
                    "tool_configuration": mcp_server.variables.get("tool_configuration", {}),
                }

        return get_json_result(data={"mcpServers": exported_servers})
    except Exception as e:
        return server_error_response(e)


@manager.route("/list_tools", methods=["POST"])  # noqa: F821
@login_required
@validate_request("mcp_ids")
def list_tools() -> Response:
    req = request.get_json()
    mcp_ids = req.get("mcp_ids", [])
    if not mcp_ids:
        return get_data_error_result(message="No MCP server IDs provided.")

    timeout = get_float(req, "timeout", 10)

    results = {}
    tool_call_sessions = []
    try:
        for mcp_id in mcp_ids:
            e, mcp_server = MCPServerService.get_by_id(mcp_id)

            if e and mcp_server.tenant_id == current_user.id:
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
                    cached_tool = cached_tools.get(tool_dict["name"])

                    tool_dict["enabled"] = cached_tool.get("enabled") if cached_tool and "enabled" in cached_tool else True
                    results[server_key].append(tool_dict)

        # PERF: blocking call to close sessions — consider moving to background thread or task queue
        close_multiple_mcp_toolcall_sessions(tool_call_sessions)
        return get_json_result(data=results)
    except Exception as e:
        return server_error_response(e)


@manager.route("/test_tool", methods=["POST"])  # noqa: F821
@login_required
@validate_request("mcp_id", "tool_name", "arguments")
def test_tool() -> Response:
    req = request.get_json()
    mcp_id = req.get("mcp_id", "")
    if not mcp_id:
        return get_data_error_result(message="No MCP server ID provided.")

    timeout = get_float(req, "timeout", 10)

    tool_name = req.get("tool_name", "")
    arguments = req.get("arguments", {})
    if not all([tool_name, arguments]):
        return get_data_error_result(message="Require provide tool name and arguments.")

    tool_call_sessions = []
    try:
        e, mcp_server = MCPServerService.get_by_id(mcp_id)
        if not e or mcp_server.tenant_id != current_user.id:
            return get_data_error_result(message=f"Cannot find MCP server {mcp_id} for user {current_user.id}")

        tool_call_session = MCPToolCallSession(mcp_server, mcp_server.variables)
        tool_call_sessions.append(tool_call_session)
        result = tool_call_session.tool_call(tool_name, arguments, timeout)

        # PERF: blocking call to close sessions — consider moving to background thread or task queue
        close_multiple_mcp_toolcall_sessions(tool_call_sessions)
        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)


@manager.route("/cache_tools", methods=["POST"])  # noqa: F821
@login_required
@validate_request("mcp_id", "tools")
def cache_tool() -> Response:
    req = request.get_json()
    mcp_id = req.get("mcp_id", "")
    if not mcp_id:
        return get_data_error_result(message="No MCP server ID provided.")
    tools = req.get("tools", [])

    e, mcp_server = MCPServerService.get_by_id(mcp_id)
    if not e or mcp_server.tenant_id != current_user.id:
        return get_data_error_result(message=f"Cannot find MCP server {mcp_id} for user {current_user.id}")

    variables = mcp_server.variables
    tools = {tool["name"]: tool for tool in tools if isinstance(tool, dict) and "name" in tool}
    variables["tools"] = tools

    if not MCPServerService.filter_update([MCPServer.id == mcp_id, MCPServer.tenant_id == current_user.id], {"variables": variables}):
        return get_data_error_result(message="Failed to updated MCP server.")

    return get_json_result(data=tools)

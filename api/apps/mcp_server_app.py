#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
from quart import Response, request
from api.apps import current_user, login_required

from api.db.db_models import MCPServer
from api.db.services.mcp_server_service import MCPServerService
from api.db.services.user_service import TenantService
from common.constants import RetCode, VALID_MCP_SERVER_TYPES

from common.misc_utils import get_uuid, thread_pool_exec
from api.utils.api_utils import get_data_error_result, get_json_result, get_mcp_tools, get_request_json, server_error_response, validate_request
from api.utils.web_utils import get_float, safe_json_parse
from common.mcp_tool_call_conn import MCPToolCallSession, close_multiple_mcp_toolcall_sessions

@manager.route("/list", methods=["POST"])  # noqa: F821
@login_required
async def list_mcp() -> Response:
    keywords = request.args.get("keywords", "")
    page_number = int(request.args.get("page", 0))
    items_per_page = int(request.args.get("page_size", 0))
    orderby = request.args.get("orderby", "create_time")
    if request.args.get("desc", "true").lower() == "false":
        desc = False
    else:
        desc = True

    req = await get_request_json()
    mcp_ids = req.get("mcp_ids", [])
    try:
        servers = MCPServerService.get_servers(current_user.id, mcp_ids, 0, 0, orderby, desc, keywords) or []
        total = len(servers)

        if page_number and items_per_page:
            servers = servers[(page_number - 1) * items_per_page : page_number * items_per_page]

        return get_json_result(data={"mcp_servers": servers, "total": total})
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
async def create() -> Response:
    req = await get_request_json()

    server_type = req.get("server_type", "")
    if server_type not in VALID_MCP_SERVER_TYPES:
        return get_data_error_result(message="Unsupported MCP server type.")

    server_name = req.get("name", "")
    if not server_name or len(server_name.encode("utf-8")) > 255:
        return get_data_error_result(message=f"Invalid MCP name or length is {len(server_name)} which is large than 255.")

    e, _ = MCPServerService.get_by_name_and_tenant(name=server_name, tenant_id=current_user.id)
    if e:
        return get_data_error_result(message="Duplicated MCP server name.")

    url = req.get("url", "")
    if not url:
        return get_data_error_result(message="Invalid url.")

    headers = safe_json_parse(req.get("headers", {}))
    req["headers"] = headers
    variables = safe_json_parse(req.get("variables", {}))
    variables.pop("tools", None)

    timeout = get_float(req, "timeout", 10)

    try:
        req["id"] = get_uuid()
        req["tenant_id"] = current_user.id

        e, _ = TenantService.get_by_id(current_user.id)
        if not e:
            return get_data_error_result(message="Tenant not found.")

        mcp_server = MCPServer(id=server_name, name=server_name, url=url, server_type=server_type, variables=variables, headers=headers)
        server_tools, err_message = await thread_pool_exec(get_mcp_tools, [mcp_server], timeout)
        if err_message:
            return get_data_error_result(err_message)

        tools = server_tools[server_name]
        tools = {tool["name"]: tool for tool in tools if isinstance(tool, dict) and "name" in tool}
        variables["tools"] = tools
        req["variables"] = variables

        if not MCPServerService.insert(**req):
            return get_data_error_result("Failed to create MCP server.")

        return get_json_result(data=req)
    except Exception as e:
        return server_error_response(e)


@manager.route("/update", methods=["POST"])  # noqa: F821
@login_required
@validate_request("mcp_id")
async def update() -> Response:
    req = await get_request_json()

    mcp_id = req.get("mcp_id", "")
    e, mcp_server = MCPServerService.get_by_id(mcp_id)
    if not e or mcp_server.tenant_id != current_user.id:
        return get_data_error_result(message=f"Cannot find MCP server {mcp_id} for user {current_user.id}")

    server_type = req.get("server_type", mcp_server.server_type)
    if server_type and server_type not in VALID_MCP_SERVER_TYPES:
        return get_data_error_result(message="Unsupported MCP server type.")
    server_name = req.get("name", mcp_server.name)
    if server_name and len(server_name.encode("utf-8")) > 255:
        return get_data_error_result(message=f"Invalid MCP name or length is {len(server_name)} which is large than 255.")
    url = req.get("url", mcp_server.url)
    if not url:
        return get_data_error_result(message="Invalid url.")

    headers = safe_json_parse(req.get("headers", mcp_server.headers))
    req["headers"] = headers

    variables = safe_json_parse(req.get("variables", mcp_server.variables))
    variables.pop("tools", None)

    timeout = get_float(req, "timeout", 10)

    try:
        req["tenant_id"] = current_user.id
        req.pop("mcp_id", None)
        req["id"] = mcp_id

        mcp_server = MCPServer(id=server_name, name=server_name, url=url, server_type=server_type, variables=variables, headers=headers)
        server_tools, err_message = await thread_pool_exec(get_mcp_tools, [mcp_server], timeout)
        if err_message:
            return get_data_error_result(err_message)

        tools = server_tools[server_name]
        tools = {tool["name"]: tool for tool in tools if isinstance(tool, dict) and "name" in tool}
        variables["tools"] = tools
        req["variables"] = variables

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
async def rm() -> Response:
    req = await get_request_json()
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
async def import_multiple() -> Response:
    req = await get_request_json()
    servers = req.get("mcpServers", {})
    if not servers:
        return get_data_error_result(message="No MCP servers provided.")

    timeout = get_float(req, "timeout", 10)

    results = []
    try:
        for server_name, config in servers.items():
            if not all(key in config for key in {"type", "url"}):
                results.append({"server": server_name, "success": False, "message": "Missing required fields (type or url)"})
                continue

            if not server_name or len(server_name.encode("utf-8")) > 255:
                results.append({"server": server_name, "success": False, "message": f"Invalid MCP name or length is {len(server_name)} which is large than 255."})
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
                "variables": {"authorization_token": config.get("authorization_token", "")},
            }

            headers = {"authorization_token": config["authorization_token"]} if "authorization_token" in config else {}
            variables = {k: v for k, v in config.items() if k not in {"type", "url", "headers"}}
            mcp_server = MCPServer(id=new_name, name=new_name, url=config["url"], server_type=config["type"], variables=variables, headers=headers)
            server_tools, err_message = await thread_pool_exec(get_mcp_tools, [mcp_server], timeout)
            if err_message:
                results.append({"server": base_name, "success": False, "message": err_message})
                continue

            tools = server_tools[new_name]
            tools = {tool["name"]: tool for tool in tools if isinstance(tool, dict) and "name" in tool}
            create_data["variables"]["tools"] = tools

            if MCPServerService.insert(**create_data):
                result = {"server": server_name, "success": True, "action": "created", "id": create_data["id"], "new_name": new_name}
                if new_name != base_name:
                    result["message"] = f"Renamed from '{base_name}' to '{new_name}' avoid duplication"
                results.append(result)
            else:
                results.append({"server": server_name, "success": False, "message": "Failed to create MCP server."})

        return get_json_result(data={"results": results})
    except Exception as e:
        return server_error_response(e)


@manager.route("/export", methods=["POST"])  # noqa: F821
@login_required
@validate_request("mcp_ids")
async def export_multiple() -> Response:
    req = await get_request_json()
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
                    "tools": mcp_server.variables.get("tools", {}),
                }

        return get_json_result(data={"mcpServers": exported_servers})
    except Exception as e:
        return server_error_response(e)


@manager.route("/list_tools", methods=["POST"])  # noqa: F821
@login_required
@validate_request("mcp_ids")
async def list_tools() -> Response:
    req = await get_request_json()
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
                    tools = await thread_pool_exec(tool_call_session.get_tools, timeout)
                except Exception as e:
                    return get_data_error_result(message=f"MCP list tools error: {e}")

                results[server_key] = []
                for tool in tools:
                    tool_dict = tool.model_dump()
                    cached_tool = cached_tools.get(tool_dict["name"], {})

                    tool_dict["enabled"] = cached_tool.get("enabled", True)
                    results[server_key].append(tool_dict)

        return get_json_result(data=results)
    except Exception as e:
        return server_error_response(e)
    finally:
        # PERF: blocking call to close sessions — consider moving to background thread or task queue
        await thread_pool_exec(close_multiple_mcp_toolcall_sessions, tool_call_sessions)


@manager.route("/test_tool", methods=["POST"])  # noqa: F821
@login_required
@validate_request("mcp_id", "tool_name", "arguments")
async def test_tool() -> Response:
    req = await get_request_json()
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
        result = await thread_pool_exec(tool_call_session.tool_call, tool_name, arguments, timeout)

        # PERF: blocking call to close sessions — consider moving to background thread or task queue
        await thread_pool_exec(close_multiple_mcp_toolcall_sessions, tool_call_sessions)
        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)


@manager.route("/cache_tools", methods=["POST"])  # noqa: F821
@login_required
@validate_request("mcp_id", "tools")
async def cache_tool() -> Response:
    req = await get_request_json()
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


@manager.route("/test_mcp", methods=["POST"])  # noqa: F821
@validate_request("url", "server_type")
async def test_mcp() -> Response:
    req = await get_request_json()

    url = req.get("url", "")
    if not url:
        return get_data_error_result(message="Invalid MCP url.")

    server_type = req.get("server_type", "")
    if server_type not in VALID_MCP_SERVER_TYPES:
        return get_data_error_result(message="Unsupported MCP server type.")

    timeout = get_float(req, "timeout", 10)
    headers = safe_json_parse(req.get("headers", {}))
    variables = safe_json_parse(req.get("variables", {}))

    mcp_server = MCPServer(id=f"{server_type}: {url}", server_type=server_type, url=url, headers=headers, variables=variables)

    result = []
    try:
        tool_call_session = MCPToolCallSession(mcp_server, mcp_server.variables)

        try:
            tools = await thread_pool_exec(tool_call_session.get_tools, timeout)
        except Exception as e:
            return get_data_error_result(message=f"Test MCP error: {e}")
        finally:
            # PERF: blocking call to close sessions — consider moving to background thread or task queue
            await thread_pool_exec(close_multiple_mcp_toolcall_sessions, [tool_call_session])

        for tool in tools:
            tool_dict = tool.model_dump()
            tool_dict["enabled"] = True
            result.append(tool_dict)

        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)

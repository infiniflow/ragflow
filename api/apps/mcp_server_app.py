from flask import Response, request
from flask_login import current_user, login_required

from api.db import VALID_MCP_SERVER_TYPES
from api.db.db_models import MCPServer
from api.db.services.mcp_server_service import MCPServerService
from api.db.services.user_service import TenantService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import get_data_error_result, get_json_result, server_error_response, validate_request
from api.utils.web_utils import safe_json_parse


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
def get() -> Response:
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
@validate_request("id")
def update() -> Response:
    req = request.get_json()

    server_type = req.get("server_type", "")
    if server_type and server_type not in VALID_MCP_SERVER_TYPES:
        return get_data_error_result(message="Unsupported MCP server type.")
    server_name = req.get("name", "")
    if server_name and len(server_name.encode("utf-8")) > 255:
        return get_data_error_result(message=f"Invaild MCP name or length is {len(server_name)} which is large than 255.")

    req["headers"] = safe_json_parse(req.get("headers", {}))
    req["variables"] = safe_json_parse(req.get("variables", {}))

    try:
        req["tenant_id"] = current_user.id

        if not MCPServerService.filter_update([MCPServer.id == req["id"], MCPServer.tenant_id == req["tenant_id"]], req):
            return get_data_error_result()

        e, updated_mcp = MCPServerService.get_by_id(req["id"])
        if not e:
            return get_data_error_result(message="Failed to fetch updated MCP server.")

        return get_json_result(data=updated_mcp.to_dict())
    except Exception as e:
        return server_error_response(e)


@manager.route("/rm", methods=["POST"])  # noqa: F821
@login_required
@validate_request("id")
def rm() -> Response:
    req = request.get_json()
    mcp_id = req["id"]

    try:
        req["tenant_id"] = current_user.id

        if not MCPServerService.filter_delete([MCPServer.id == mcp_id, MCPServer.tenant_id == req["tenant_id"]]):
            return get_data_error_result(message=f"Failed to delete mcp server {mcp_id}")

        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)

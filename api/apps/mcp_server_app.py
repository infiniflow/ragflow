from flask import Response, request
from flask_login import current_user, login_required
from api.db.db_models import MCPServer
from api.db.services.mcp_server_service import MCPServerService
from api.db.services.user_service import TenantService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import get_data_error_result, get_json_result, server_error_response, validate_request


@manager.route("/list", methods=["GET"])  # noqa: F821
@login_required
def get_list() -> Response:
    try:
        return get_json_result(data=MCPServerService.get_servers(current_user.id) or [])
    except Exception as e:
        return server_error_response(e)


@manager.route("/get_multiple", methods=["POST"])  # noqa: F821
@login_required
@validate_request("id_list")
def get_multiple() -> Response:
    req = request.json

    try:
        return get_json_result(data=MCPServerService.get_servers(current_user.id, id_list=req["id_list"]) or [])
    except Exception as e:
        return server_error_response(e)


@manager.route("/get/<ms_id>", methods=["GET"])  # noqa: F821
@login_required
def get(ms_id: str) -> Response:
    try:
        mcp_server = MCPServerService.get_or_none(id=ms_id, tenant_id=current_user.id)

        if mcp_server is None:
            return get_json_result(code=RetCode.NOT_FOUND, data=None)

        return get_json_result(data=mcp_server.to_dict())
    except Exception as e:
        return server_error_response(e)


@manager.route("/create", methods=["POST"])  # noqa: F821
@login_required
@validate_request("name", "url", "server_type")
def create() -> Response:
    req = request.json

    try:
        req["id"] = get_uuid()
        req["tenant_id"] = current_user.id

        e, _ = TenantService.get_by_id(current_user.id)

        if not e:
            return get_data_error_result(message="Tenant not found.")

        if not req.get("headers"):
            req["headers"] = {}

        if not MCPServerService.insert(**req):
            return get_data_error_result()

        return get_json_result(data={"id": req["id"]})
    except Exception as e:
        return server_error_response(e)


@manager.route("/update", methods=["POST"])  # noqa: F821
@login_required
@validate_request("id", "name", "url", "server_type")
def update() -> Response:
    req = request.json

    if not req.get("headers"):
        req["headers"] = {}

    try:
        req["tenant_id"] = current_user.id

        if not MCPServerService.filter_update([MCPServer.id == req["id"], MCPServer.tenant_id == req["tenant_id"]], req):
            return get_data_error_result()

        return get_json_result(data={"id": req["id"]})
    except Exception as e:
        return server_error_response(e)


@manager.route("/rm", methods=["POST"])  # noqa: F821
@login_required
@validate_request("id")
def rm() -> Response:
    req = request.json
    ms_id = req["id"]

    try:
        req["tenant_id"] = current_user.id

        if not MCPServerService.filter_delete([MCPServer.id == ms_id, MCPServer.tenant_id == req["tenant_id"]]):
            return get_data_error_result()

        return get_json_result(data={"id": req["id"]})
    except Exception as e:
        return server_error_response(e)

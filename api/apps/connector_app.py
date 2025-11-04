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
import time

from flask import request
from flask_login import login_required, current_user

from api.db import TaskStatus, InputType
from api.db.services.connector_service import ConnectorService, Connector2KbService, SyncLogsService
from api.utils.api_utils import get_json_result, validate_request, get_data_error_result
from common.misc_utils import get_uuid
from common.contants import RetCode

@manager.route("/set", methods=["POST"])  # noqa: F821
@login_required
def set_connector():
    req = request.json
    if req.get("id"):
        conn = {fld: req[fld] for fld in ["prune_freq", "refresh_freq", "config", "timeout_secs"] if fld in req}
        ConnectorService.update_by_id(req["id"], conn)
    else:
        req["id"] = get_uuid()
        conn = {
            "id": req["id"],
            "tenant_id": current_user.id,
            "name": req["name"],
            "source": req["source"],
            "input_type": InputType.POLL,
            "config": req["config"],
            "refresh_freq": int(req["refresh_freq"]),
            "prune_freq": int(req["prune_freq"]),
            "timeout_secs": int(req["timeout_secs"]),
            "status": TaskStatus.SCHEDULE
        }
        conn["status"] = TaskStatus.SCHEDULE

    ConnectorService.save(**conn)
    time.sleep(1)
    e, conn = ConnectorService.get_by_id(req["id"])

    return get_json_result(data=conn.to_dict())


@manager.route("/list", methods=["GET"])  # noqa: F821
@login_required
def list_connector():
    return get_json_result(data=ConnectorService.list(current_user.id))


@manager.route("/<connector_id>", methods=["GET"])  # noqa: F821
@login_required
def get_connector(connector_id):
    e, conn = ConnectorService.get_by_id(connector_id)
    if not e:
        return get_data_error_result(message="Can't find this Connector!")
    return get_json_result(data=conn.to_dict())


@manager.route("/<connector_id>/logs", methods=["GET"])  # noqa: F821
@login_required
def list_logs(connector_id):
    req = request.args.to_dict(flat=True)
    return get_json_result(data=SyncLogsService.list_sync_tasks(connector_id, int(req.get("page", 1)), int(req.get("page_size", 15))))


@manager.route("/<connector_id>/resume", methods=["PUT"])  # noqa: F821
@login_required
def resume(connector_id):
    req = request.json
    if req.get("resume"):
        ConnectorService.resume(connector_id, TaskStatus.SCHEDULE)
    else:
        ConnectorService.resume(connector_id, TaskStatus.CANCEL)
    return get_json_result(data=True)


@manager.route("/<connector_id>/link", methods=["POST"])  # noqa: F821
@validate_request("kb_ids")
@login_required
def link_kb(connector_id):
    req = request.json
    errors = Connector2KbService.link_kb(connector_id, req["kb_ids"], current_user.id)
    if errors:
        return get_json_result(data=False, message=errors, code=RetCode.SERVER_ERROR)
    return get_json_result(data=True)


@manager.route("/<connector_id>/rm", methods=["POST"])  # noqa: F821
@login_required
def rm_connector(connector_id):
    ConnectorService.resume(connector_id, TaskStatus.CANCEL)
    ConnectorService.delete_by_id(connector_id)
    return get_json_result(data=True)
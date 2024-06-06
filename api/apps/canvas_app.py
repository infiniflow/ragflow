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
import json

from flask import request
from flask_login import login_required, current_user

from api.db.services.canvas_service import CanvasTemplateService, UserCanvasService
from api.utils import get_uuid
from api.utils.api_utils import get_json_result, server_error_response, validate_request
from graph.canvas import Canvas


@manager.route('/templates', methods=['GET'])
@login_required
def templates():
    return get_json_result(data=[c.to_dict() for c in CanvasTemplateService.get_all()])


@manager.route('/list', methods=['GET'])
@login_required
def canvas_list():

    return get_json_result(data=[c.to_dict() for c in UserCanvasService.query(user_id=current_user.id)])


@manager.route('/rm', methods=['POST'])
@validate_request("canvas_ids")
@login_required
def rm():
    for i in request.json["canvas_ids"]:
        UserCanvasService.delete_by_id(i)
    return get_json_result(data=True)


@manager.route('/set', methods=['POST'])
@validate_request("dsl", "title")
@login_required
def save():
    req = request.json
    req["user_id"] = current_user.id
    if not isinstance(req["dsl"], str):req["dsl"] = json.dumps(req["dsl"], ensure_ascii=False)
    try:
        Canvas(req["dsl"])
    except Exception as e:
        return server_error_response(e)

    req["dsl"] = json.loads(req["dsl"])
    if "id" not in req:
        req["id"] = get_uuid()
        if not UserCanvasService.save(**req):
            return server_error_response("Fail to save canvas.")
    else:
        UserCanvasService.update_by_id(req["id"], req)

    return get_json_result(data=req)


@manager.route('/get/<canvas_id>', methods=['GET'])
@login_required
def get(canvas_id):
    e, c = UserCanvasService.get_by_id(canvas_id)
    if not e:
        return server_error_response("canvas not found.")
    return get_json_result(data=c.to_dict())


@manager.route('/run', methods=['POST'])
@validate_request("id", "dsl")
@login_required
def run():
    req = request.json
    if not isinstance(req["dsl"], str): req["dsl"] = json.dumps(req["dsl"], ensure_ascii=False)
    try:
        canvas = Canvas(req["dsl"], current_user.id)
        ans = canvas.run()
        req["dsl"] = json.loads(str(canvas))
        UserCanvasService.update_by_id(req["id"], dsl=req["dsl"])
        return get_json_result(data=req["dsl"])
    except Exception as e:
        return server_error_response(e)


@manager.route('/reset', methods=['POST'])
@validate_request("canvas_id")
@login_required
def reset():
    req = request.json
    try:
        user_canvas = UserCanvasService.get_by_id(req["canvas_id"])
        canvas = Canvas(req["dsl"], current_user.id)
        canvas.reset()
        req["dsl"] = json.loads(str(canvas))
        UserCanvasService.update_by_id(req["canvas_id"], dsl=req["dsl"])
        return get_json_result(data=req["dsl"])
    except Exception as e:
        return server_error_response(e)



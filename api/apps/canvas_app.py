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
from functools import partial
from flask import request, Response
from flask_login import login_required, current_user
from api.db.services.canvas_service import CanvasTemplateService, UserCanvasService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import get_json_result, server_error_response, validate_request, get_data_error_result
from agent.canvas import Canvas
from peewee import MySQLDatabase, PostgresqlDatabase


@manager.route('/templates', methods=['GET'])
@login_required
def templates():
    return get_json_result(data=[c.to_dict() for c in CanvasTemplateService.get_all()])


@manager.route('/list', methods=['GET'])
@login_required
def canvas_list():
    return get_json_result(data=sorted([c.to_dict() for c in \
                                 UserCanvasService.query(user_id=current_user.id)], key=lambda x: x["update_time"]*-1)
                           )


@manager.route('/rm', methods=['POST'])
@validate_request("canvas_ids")
@login_required
def rm():
    for i in request.json["canvas_ids"]:
        if not UserCanvasService.query(user_id=current_user.id,id=i):
            return get_json_result(
                data=False, retmsg=f'Only owner of canvas authorized for this operation.',
                retcode=RetCode.OPERATING_ERROR)
        UserCanvasService.delete_by_id(i)
    return get_json_result(data=True)


@manager.route('/set', methods=['POST'])
@validate_request("dsl", "title")
@login_required
def save():
    req = request.json
    req["user_id"] = current_user.id
    if not isinstance(req["dsl"], str): req["dsl"] = json.dumps(req["dsl"], ensure_ascii=False)

    req["dsl"] = json.loads(req["dsl"])
    if "id" not in req:
        if UserCanvasService.query(user_id=current_user.id, title=req["title"].strip()):
            return server_error_response(ValueError("Duplicated title."))
        req["id"] = get_uuid()
        if not UserCanvasService.save(**req):
            return get_data_error_result(retmsg="Fail to save canvas.")
    else:
        if not UserCanvasService.query(user_id=current_user.id, id=req["id"]):
            return get_json_result(
                data=False, retmsg=f'Only owner of canvas authorized for this operation.',
                retcode=RetCode.OPERATING_ERROR)
        UserCanvasService.update_by_id(req["id"], req)
    return get_json_result(data=req)


@manager.route('/get/<canvas_id>', methods=['GET'])
@login_required
def get(canvas_id):
    e, c = UserCanvasService.get_by_id(canvas_id)
    if not e:
        return get_data_error_result(retmsg="canvas not found.")
    return get_json_result(data=c.to_dict())


@manager.route('/completion', methods=['POST'])
@validate_request("id")
@login_required
def run():
    req = request.json
    stream = req.get("stream", True)
    e, cvs = UserCanvasService.get_by_id(req["id"])
    if not e:
        return get_data_error_result(retmsg="canvas not found.")
    if not UserCanvasService.query(user_id=current_user.id, id=req["id"]):
        return get_json_result(
            data=False, retmsg=f'Only owner of canvas authorized for this operation.',
            retcode=RetCode.OPERATING_ERROR)

    if not isinstance(cvs.dsl, str):
        cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)

    final_ans = {"reference": [], "content": ""}
    message_id = req.get("message_id", get_uuid())
    try:
        canvas = Canvas(cvs.dsl, current_user.id)
        if "message" in req:
            canvas.messages.append({"role": "user", "content": req["message"], "id": message_id})
            if len([m for m in canvas.messages if m["role"] == "user"]) > 1:
                #ten = TenantService.get_info_by(current_user.id)[0]
                #req["message"] = full_question(ten["tenant_id"], ten["llm_id"], canvas.messages)
                pass
            canvas.add_user_input(req["message"])
        answer = canvas.run(stream=stream)
        print(canvas)
    except Exception as e:
        return server_error_response(e)

    assert answer is not None, "Nothing. Is it over?"

    if stream:
        assert isinstance(answer, partial), "Nothing. Is it over?"

        def sse():
            nonlocal answer, cvs
            try:
                for ans in answer():
                    for k in ans.keys():
                        final_ans[k] = ans[k]
                    ans = {"answer": ans["content"], "reference": ans.get("reference", [])}
                    yield "data:" + json.dumps({"retcode": 0, "retmsg": "", "data": ans}, ensure_ascii=False) + "\n\n"

                canvas.messages.append({"role": "assistant", "content": final_ans["content"], "id": message_id})
                if final_ans.get("reference"):
                    canvas.reference.append(final_ans["reference"])
                cvs.dsl = json.loads(str(canvas))
                UserCanvasService.update_by_id(req["id"], cvs.to_dict())
            except Exception as e:
                yield "data:" + json.dumps({"retcode": 500, "retmsg": str(e),
                                            "data": {"answer": "**ERROR**: " + str(e), "reference": []}},
                                           ensure_ascii=False) + "\n\n"
            yield "data:" + json.dumps({"retcode": 0, "retmsg": "", "data": True}, ensure_ascii=False) + "\n\n"

        resp = Response(sse(), mimetype="text/event-stream")
        resp.headers.add_header("Cache-control", "no-cache")
        resp.headers.add_header("Connection", "keep-alive")
        resp.headers.add_header("X-Accel-Buffering", "no")
        resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
        return resp

    final_ans["content"] = "\n".join(answer["content"]) if "content" in answer else ""
    canvas.messages.append({"role": "assistant", "content": final_ans["content"], "id": message_id})
    if final_ans.get("reference"):
        canvas.reference.append(final_ans["reference"])
    cvs.dsl = json.loads(str(canvas))
    UserCanvasService.update_by_id(req["id"], cvs.to_dict())
    return get_json_result(data={"answer": final_ans["content"], "reference": final_ans.get("reference", [])})


@manager.route('/reset', methods=['POST'])
@validate_request("id")
@login_required
def reset():
    req = request.json
    try:
        e, user_canvas = UserCanvasService.get_by_id(req["id"])
        if not e:
            return get_data_error_result(retmsg="canvas not found.")
        if not UserCanvasService.query(user_id=current_user.id, id=req["id"]):
            return get_json_result(
                data=False, retmsg=f'Only owner of canvas authorized for this operation.',
                retcode=RetCode.OPERATING_ERROR)

        canvas = Canvas(json.dumps(user_canvas.dsl), current_user.id)
        canvas.reset()
        req["dsl"] = json.loads(str(canvas))
        UserCanvasService.update_by_id(req["id"], {"dsl": req["dsl"]})
        return get_json_result(data=req["dsl"])
    except Exception as e:
        return server_error_response(e)


@manager.route('/test_db_connect', methods=['POST'])
@validate_request("db_type", "database", "username", "host", "port", "password")
@login_required
def test_db_connect():
    req = request.json
    try:
        if req["db_type"] in ["mysql", "mariadb"]:
            db = MySQLDatabase(req["database"], user=req["username"], host=req["host"], port=req["port"],
                               password=req["password"])
        elif req["db_type"] == 'postgresql':
            db = PostgresqlDatabase(req["database"], user=req["username"], host=req["host"], port=req["port"],
                                    password=req["password"])
        db.connect()
        db.close()
        return get_json_result(data="Database Connection Successful!")
    except Exception as e:
        return server_error_response(e)

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
from copy import deepcopy

from db.services.user_service import UserTenantService
from flask import request, Response
from flask_login import login_required, current_user

from api.db import LLMType
from api.db.services.dialog_service import DialogService, ConversationService, chat
from api.db.services.llm_service import LLMBundle, TenantService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import get_json_result
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request


@manager.route('/set', methods=['POST'])
@login_required
def set_conversation():
    req = request.json
    conv_id = req.get("conversation_id")
    if conv_id:
        del req["conversation_id"]
        try:
            if not ConversationService.update_by_id(conv_id, req):
                return get_data_error_result(retmsg="Conversation not found!")
            e, conv = ConversationService.get_by_id(conv_id)
            if not e:
                return get_data_error_result(
                    retmsg="Fail to update a conversation!")
            conv = conv.to_dict()
            return get_json_result(data=conv)
        except Exception as e:
            return server_error_response(e)

    try:
        e, dia = DialogService.get_by_id(req["dialog_id"])
        if not e:
            return get_data_error_result(retmsg="Dialog not found")
        conv = {
            "id": get_uuid(),
            "dialog_id": req["dialog_id"],
            "name": req.get("name", "New conversation"),
            "message": [{"role": "assistant", "content": dia.prompt_config["prologue"]}]
        }
        ConversationService.save(**conv)
        e, conv = ConversationService.get_by_id(conv["id"])
        if not e:
            return get_data_error_result(retmsg="Fail to new a conversation!")
        conv = conv.to_dict()
        return get_json_result(data=conv)
    except Exception as e:
        return server_error_response(e)


@manager.route('/get', methods=['GET'])
@login_required
def get():
    conv_id = request.args["conversation_id"]
    try:
        e, conv = ConversationService.get_by_id(conv_id)
        if not e:
            return get_data_error_result(retmsg="Conversation not found!")
        tenants = UserTenantService.query(user_id=current_user.id)
        for tenant in tenants:
            if DialogService.query(tenant_id=tenant.tenant_id, id=conv.dialog_id):
                break
        else:
            return get_json_result(
                data=False, retmsg=f'Only owner of conversation authorized for this operation.',
                retcode=RetCode.OPERATING_ERROR)
        conv = conv.to_dict()
        return get_json_result(data=conv)
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['POST'])
@login_required
def rm():
    conv_ids = request.json["conversation_ids"]
    try:
        for cid in conv_ids:
            exist, conv = ConversationService.get_by_id(cid)
            if not exist:
                return get_data_error_result(retmsg="Conversation not found!")
            tenants = UserTenantService.query(user_id=current_user.id)
            for tenant in tenants:
                if DialogService.query(tenant_id=tenant.tenant_id, id=conv.dialog_id):
                    break
            else:
                return get_json_result(
                    data=False, retmsg=f'Only owner of conversation authorized for this operation.',
                    retcode=RetCode.OPERATING_ERROR)
            ConversationService.delete_by_id(cid)
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/list', methods=['GET'])
@login_required
def list_convsersation():
    dialog_id = request.args["dialog_id"]
    try:
        if not DialogService.query(tenant_id=current_user.id, id=dialog_id):
            return get_json_result(
                data=False, retmsg=f'Only owner of dialog authorized for this operation.',
                retcode=RetCode.OPERATING_ERROR)
        convs = ConversationService.query(
            dialog_id=dialog_id,
            order_by=ConversationService.model.create_time,
            reverse=True)
        convs = [d.to_dict() for d in convs]
        return get_json_result(data=convs)
    except Exception as e:
        return server_error_response(e)


@manager.route('/completion', methods=['POST'])
@login_required
@validate_request("conversation_id", "messages")
def completion():
    req = request.json
    # req = {"conversation_id": "9aaaca4c11d311efa461fa163e197198", "messages": [
    #    {"role": "user", "content": "上海有吗？"}
    # ]}
    msg = []
    for m in req["messages"]:
        if m["role"] == "system":
            continue
        if m["role"] == "assistant" and not msg:
            continue
        msg.append(m)
    message_id = msg[-1].get("id")
    try:
        e, conv = ConversationService.get_by_id(req["conversation_id"])
        if not e:
            return get_data_error_result(retmsg="Conversation not found!")
        conv.message = deepcopy(req["messages"])
        e, dia = DialogService.get_by_id(conv.dialog_id)
        if not e:
            return get_data_error_result(retmsg="Dialog not found!")
        del req["conversation_id"]
        del req["messages"]

        if not conv.reference:
            conv.reference = []
        conv.message.append({"role": "assistant", "content": "", "id": message_id})
        conv.reference.append({"chunks": [], "doc_aggs": []})

        def fillin_conv(ans):
            nonlocal conv, message_id
            if not conv.reference:
                conv.reference.append(ans["reference"])
            else:
                conv.reference[-1] = ans["reference"]
            conv.message[-1] = {"role": "assistant", "content": ans["answer"],
                                "id": message_id, "prompt": ans.get("prompt", "")}
            ans["id"] = message_id

        def stream():
            nonlocal dia, msg, req, conv
            try:
                for ans in chat(dia, msg, True, **req):
                    fillin_conv(ans)
                    yield "data:" + json.dumps({"retcode": 0, "retmsg": "", "data": ans}, ensure_ascii=False) + "\n\n"
                ConversationService.update_by_id(conv.id, conv.to_dict())
            except Exception as e:
                yield "data:" + json.dumps({"retcode": 500, "retmsg": str(e),
                                            "data": {"answer": "**ERROR**: " + str(e), "reference": []}},
                                           ensure_ascii=False) + "\n\n"
            yield "data:" + json.dumps({"retcode": 0, "retmsg": "", "data": True}, ensure_ascii=False) + "\n\n"

        if req.get("stream", True):
            resp = Response(stream(), mimetype="text/event-stream")
            resp.headers.add_header("Cache-control", "no-cache")
            resp.headers.add_header("Connection", "keep-alive")
            resp.headers.add_header("X-Accel-Buffering", "no")
            resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
            return resp

        else:
            answer = None
            for ans in chat(dia, msg, **req):
                answer = ans
                fillin_conv(ans)
                ConversationService.update_by_id(conv.id, conv.to_dict())
                break
            return get_json_result(data=answer)
    except Exception as e:
        return server_error_response(e)


@manager.route('/tts', methods=['POST'])
@login_required
def tts():
    req = request.json
    text = req["text"]

    tenants = TenantService.get_by_user_id(current_user.id)
    if not tenants:
        return get_data_error_result(retmsg="Tenant not found!")

    tts_id = tenants[0]["tts_id"]
    if not tts_id:
        return get_data_error_result(retmsg="No default TTS model is set")

    tts_mdl = LLMBundle(tenants[0]["tenant_id"], LLMType.TTS, tts_id)

    def stream_audio():
        try:
            for chunk in tts_mdl.tts(text):
                yield chunk
        except Exception as e:
            yield ("data:" + json.dumps({"retcode": 500, "retmsg": str(e),
                                         "data": {"answer": "**ERROR**: " + str(e)}},
                                        ensure_ascii=False)).encode('utf-8')

    resp = Response(stream_audio(), mimetype="audio/mpeg")
    resp.headers.add_header("Cache-Control", "no-cache")
    resp.headers.add_header("Connection", "keep-alive")
    resp.headers.add_header("X-Accel-Buffering", "no")

    return resp


@manager.route('/delete_msg', methods=['POST'])
@login_required
@validate_request("conversation_id", "message_id")
def delete_msg():
    req = request.json
    e, conv = ConversationService.get_by_id(req["conversation_id"])
    if not e:
        return get_data_error_result(retmsg="Conversation not found!")

    conv = conv.to_dict()
    for i, msg in enumerate(conv["message"]):
        if req["message_id"] != msg.get("id", ""):
            continue
        assert conv["message"][i + 1]["id"] == req["message_id"]
        conv["message"].pop(i)
        conv["message"].pop(i)
        conv["reference"].pop(max(0, i // 2 - 1))
        break

    ConversationService.update_by_id(conv["id"], conv)
    return get_json_result(data=conv)


@manager.route('/thumbup', methods=['POST'])
@login_required
@validate_request("conversation_id", "message_id")
def thumbup():
    req = request.json
    e, conv = ConversationService.get_by_id(req["conversation_id"])
    if not e:
        return get_data_error_result(retmsg="Conversation not found!")
    up_down = req.get("set")
    feedback = req.get("feedback", "")
    conv = conv.to_dict()
    for i, msg in enumerate(conv["message"]):
        if req["message_id"] == msg.get("id", "") and msg.get("role", "") == "assistant":
            if up_down:
                msg["thumbup"] = True
                if "feedback" in msg: del msg["feedback"]
            else:
                msg["thumbup"] = False
                if feedback: msg["feedback"] = feedback
            break

    ConversationService.update_by_id(conv["id"], conv)
    return get_json_result(data=conv)

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
from uuid import uuid4

from flask import request, Response

from api.db import StatusEnum
from api.db.services.dialog_service import DialogService, ConversationService, chat
from api.utils import get_uuid
from api.utils.api_utils import get_error_data_result
from api.utils.api_utils import get_result, token_required

@manager.route('/chat/<chat_id>/session', methods=['POST'])
@token_required
def create(tenant_id,chat_id):
    req = request.json
    req["dialog_id"] = chat_id
    dia = DialogService.query(tenant_id=tenant_id, id=req["dialog_id"], status=StatusEnum.VALID.value)
    if not dia:
        return get_error_data_result(retmsg="You do not own the assistant")
    conv = {
        "id": get_uuid(),
        "dialog_id": req["dialog_id"],
        "name": req.get("name", "New session"),
        "message": [{"role": "assistant", "content": "Hi! I am your assistant，can I help you?"}]
    }
    if not conv.get("name"):
        return get_error_data_result(retmsg="`name` can not be empty.")
    ConversationService.save(**conv)
    e, conv = ConversationService.get_by_id(conv["id"])
    if not e:
        return get_error_data_result(retmsg="Fail to create a session!")
    conv = conv.to_dict()
    conv['messages'] = conv.pop("message")
    conv["chat_id"] = conv.pop("dialog_id")
    del conv["reference"]
    return get_result(data=conv)

@manager.route('/chat/<chat_id>/session/<session_id>', methods=['PUT'])
@token_required
def update(tenant_id,chat_id,session_id):
    req = request.json
    req["dialog_id"] = chat_id
    conv_id = session_id
    conv = ConversationService.query(id=conv_id,dialog_id=chat_id)
    if not conv:
        return get_error_data_result(retmsg="Session does not exist")
    if not DialogService.query(id=chat_id, tenant_id=tenant_id, status=StatusEnum.VALID.value):
        return get_error_data_result(retmsg="You do not own the session")
    if "message" in req or "messages" in req:
        return get_error_data_result(retmsg="`message` can not be change")
    if "reference" in req:
        return get_error_data_result(retmsg="`reference` can not be change")
    if "name" in req and not req.get("name"):
        return get_error_data_result(retmsg="`name` can not be empty.")
    if not ConversationService.update_by_id(conv_id, req):
        return get_error_data_result(retmsg="Session updates error")
    return get_result()


@manager.route('/chat/<chat_id>/completion', methods=['POST'])
@token_required
def completion(tenant_id,chat_id):
    req = request.json
    # req = {"conversation_id": "9aaaca4c11d311efa461fa163e197198", "messages": [
    #    {"role": "user", "content": "上海有吗？"}
    # ]}
    if not req.get("session_id"):
        conv = {
            "id": get_uuid(),
            "dialog_id": chat_id,
            "name": req.get("name", "New session"),
            "message": [{"role": "assistant", "content": "Hi! I am your assistant，can I help you?"}]
        }
        if not conv.get("name"):
            return get_error_data_result(retmsg="`name` can not be empty.")
        ConversationService.save(**conv)
        e, conv = ConversationService.get_by_id(conv["id"])
        session_id=conv.id
    else:
        session_id = req.get("session_id")
    if not req.get("question"):
        return get_error_data_result(retmsg="Please input your question.")
    conv = ConversationService.query(id=session_id,dialog_id=chat_id)
    if not conv:
        return get_error_data_result(retmsg="Session does not exist")
    conv = conv[0]
    if not DialogService.query(id=chat_id, tenant_id=tenant_id, status=StatusEnum.VALID.value):
        return get_error_data_result(retmsg="You do not own the session")
    msg = []
    question = {
        "content": req.get("question"),
        "role": "user",
        "id": str(uuid4())
    }
    conv.message.append(question)
    for m in conv.message:
        if m["role"] == "system": continue
        if m["role"] == "assistant" and not msg: continue
        msg.append(m)
    message_id = msg[-1].get("id")
    e, dia = DialogService.get_by_id(conv.dialog_id)

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
        ans["session_id"]=session_id

    def stream():
        nonlocal dia, msg, req, conv
        try:
            for ans in chat(dia, msg, **req):
                fillin_conv(ans)
                yield "data:" + json.dumps({"code": 0,  "data": ans}, ensure_ascii=False) + "\n\n"
            ConversationService.update_by_id(conv.id, conv.to_dict())
        except Exception as e:
            yield "data:" + json.dumps({"code": 500, "message": str(e),
                                        "data": {"answer": "**ERROR**: " + str(e),"reference": []}},
                                       ensure_ascii=False) + "\n\n"
        yield "data:" + json.dumps({"code": 0, "data": True}, ensure_ascii=False) + "\n\n"

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
        return get_result(data=answer)

@manager.route('/chat/<chat_id>/session', methods=['GET'])
@token_required
def list(chat_id,tenant_id):
    if not DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value):
        return get_error_data_result(retmsg=f"You don't own the assistant {chat_id}.")
    id = request.args.get("id")
    name = request.args.get("name")
    session = ConversationService.query(id=id,name=name,dialog_id=chat_id)
    if not session:
        return get_error_data_result(retmsg="The session doesn't exist")
    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 1024))
    orderby = request.args.get("orderby", "create_time")
    if request.args.get("desc") == "False" or request.args.get("desc") == "false":
        desc = False
    else:
        desc = True
    convs = ConversationService.get_list(chat_id,page_number,items_per_page,orderby,desc,id,name)
    if not convs:
        return get_result(data=[])
    for conv in convs:
        conv['messages'] = conv.pop("message")
        conv["chat"] = conv.pop("dialog_id")
        if conv["reference"]:
            messages = conv["messages"]
            message_num = 0
            chunk_num = 0
            while message_num < len(messages):
                if message_num != 0 and messages[message_num]["role"] != "user":
                    chunk_list = []
                    if "chunks" in conv["reference"][chunk_num]:
                        chunks = conv["reference"][chunk_num]["chunks"]
                        for chunk in chunks:
                            new_chunk = {
                                "id": chunk["chunk_id"],
                                "content": chunk["content_with_weight"],
                                "document_id": chunk["doc_id"],
                                "document_name": chunk["docnm_kwd"],
                                "dataset_id": chunk["kb_id"],
                                "image_id": chunk["img_id"],
                                "similarity": chunk["similarity"],
                                "vector_similarity": chunk["vector_similarity"],
                                "term_similarity": chunk["term_similarity"],
                                "positions": chunk["positions"],
                            }
                            chunk_list.append(new_chunk)
                    chunk_num += 1
                    messages[message_num]["reference"] = chunk_list
                message_num += 1
        del conv["reference"]
    return get_result(data=convs)

@manager.route('/chat/<chat_id>/session', methods=["DELETE"])
@token_required
def delete(tenant_id,chat_id):
    if not DialogService.query(id=chat_id, tenant_id=tenant_id, status=StatusEnum.VALID.value):
        return get_error_data_result(retmsg="You don't own the chat")
    ids = request.json.get("ids")
    if not ids:
        return get_error_data_result(retmsg="`ids` is required in deleting operation")
    for id in ids:
        conv = ConversationService.query(id=id,dialog_id=chat_id)
        if not conv:
            return get_error_data_result(retmsg="The chat doesn't own the session")
        ConversationService.delete_by_id(id)
    return get_result()

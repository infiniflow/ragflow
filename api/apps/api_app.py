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
import os
import re
from datetime import datetime, timedelta
from flask import request, Response
from flask_login import login_required, current_user

from api.db import FileType, ParserType
from api.db.db_models import APIToken, API4Conversation, Task
from api.db.services import duplicate_name
from api.db.services.api_service import APITokenService, API4ConversationService
from api.db.services.dialog_service import DialogService, chat
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.task_service import queue_tasks, TaskService
from api.db.services.user_service import UserTenantService
from api.settings import RetCode
from api.utils import get_uuid, current_timestamp, datetime_format
from api.utils.api_utils import server_error_response, get_data_error_result, get_json_result, validate_request
from itsdangerous import URLSafeTimedSerializer

from api.utils.file_utils import filename_type, thumbnail
from rag.utils.minio_conn import MINIO

from rag.utils.es_conn import ELASTICSEARCH
from rag.nlp import search
from elasticsearch_dsl import Q

def generate_confirmation_token(tenent_id):
    serializer = URLSafeTimedSerializer(tenent_id)
    return "ragflow-" + serializer.dumps(get_uuid(), salt=tenent_id)[2:34]


@manager.route('/new_token', methods=['POST'])
@validate_request("dialog_id")
@login_required
def new_token():
    req = request.json
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(retmsg="Tenant not found!")

        tenant_id = tenants[0].tenant_id
        obj = {"tenant_id": tenant_id, "token": generate_confirmation_token(tenant_id),
               "dialog_id": req["dialog_id"],
               "create_time": current_timestamp(),
               "create_date": datetime_format(datetime.now()),
               "update_time": None,
               "update_date": None
               }
        if not APITokenService.save(**obj):
            return get_data_error_result(retmsg="Fail to new a dialog!")

        return get_json_result(data=obj)
    except Exception as e:
        return server_error_response(e)


@manager.route('/token_list', methods=['GET'])
@login_required
def token_list():
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(retmsg="Tenant not found!")

        objs = APITokenService.query(tenant_id=tenants[0].tenant_id, dialog_id=request.args["dialog_id"])
        return get_json_result(data=[o.to_dict() for o in objs])
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['POST'])
@validate_request("tokens", "tenant_id")
@login_required
def rm():
    req = request.json
    try:
        for token in req["tokens"]:
            APITokenService.filter_delete(
                [APIToken.tenant_id == req["tenant_id"], APIToken.token == token])
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/stats', methods=['GET'])
@login_required
def stats():
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(retmsg="Tenant not found!")
        objs = API4ConversationService.stats(
            tenants[0].tenant_id,
            request.args.get(
                "from_date",
                (datetime.now() -
                 timedelta(
                    days=7)).strftime("%Y-%m-%d 24:00:00")),
            request.args.get(
                "to_date",
                datetime.now().strftime("%Y-%m-%d %H:%M:%S")))
        res = {
            "pv": [(o["dt"], o["pv"]) for o in objs],
            "uv": [(o["dt"], o["uv"]) for o in objs],
            "speed": [(o["dt"], float(o["tokens"])/(float(o["duration"]+0.1))) for o in objs],
            "tokens": [(o["dt"], float(o["tokens"])/1000.) for o in objs],
            "round": [(o["dt"], o["round"]) for o in objs],
            "thumb_up": [(o["dt"], o["thumb_up"]) for o in objs]
        }
        return get_json_result(data=res)
    except Exception as e:
        return server_error_response(e)


@manager.route('/new_conversation', methods=['GET'])
def set_conversation():
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, retmsg='Token is not valid!"', retcode=RetCode.AUTHENTICATION_ERROR)
    req = request.json
    try:
        e, dia = DialogService.get_by_id(objs[0].dialog_id)
        if not e:
            return get_data_error_result(retmsg="Dialog not found")
        conv = {
            "id": get_uuid(),
            "dialog_id": dia.id,
            "user_id": request.args.get("user_id", ""),
            "message": [{"role": "assistant", "content": dia.prompt_config["prologue"]}]
        }
        API4ConversationService.save(**conv)
        e, conv = API4ConversationService.get_by_id(conv["id"])
        if not e:
            return get_data_error_result(retmsg="Fail to new a conversation!")
        conv = conv.to_dict()
        return get_json_result(data=conv)
    except Exception as e:
        return server_error_response(e)


@manager.route('/completion', methods=['POST'])
@validate_request("conversation_id", "messages")
def completion():
    token = request.headers.get('Authorization').split()[1]
    if not APIToken.query(token=token):
        return get_json_result(
            data=False, retmsg='Token is not valid!"', retcode=RetCode.AUTHENTICATION_ERROR)
    req = request.json
    e, conv = API4ConversationService.get_by_id(req["conversation_id"])
    if not e:
        return get_data_error_result(retmsg="Conversation not found!")
    if "quote" not in req: req["quote"] = False

    msg = []
    for m in req["messages"]:
        if m["role"] == "system":
            continue
        if m["role"] == "assistant" and not msg:
            continue
        msg.append({"role": m["role"], "content": m["content"]})

    try:
        conv.message.append(msg[-1])
        e, dia = DialogService.get_by_id(conv.dialog_id)
        if not e:
            return get_data_error_result(retmsg="Dialog not found!")
        del req["conversation_id"]
        del req["messages"]

        if not conv.reference:
            conv.reference = []
        conv.message.append({"role": "assistant", "content": ""})
        conv.reference.append({"chunks": [], "doc_aggs": []})

        def fillin_conv(ans):
            nonlocal conv
            if not conv.reference:
                conv.reference.append(ans["reference"])
            else: conv.reference[-1] = ans["reference"]
            conv.message[-1] = {"role": "assistant", "content": ans["answer"]}

        def stream():
            nonlocal dia, msg, req, conv
            try:
                for ans in chat(dia, msg, True, **req):
                    fillin_conv(ans)
                    yield "data:"+json.dumps({"retcode": 0, "retmsg": "", "data": ans}, ensure_ascii=False) + "\n\n"
                API4ConversationService.append_message(conv.id, conv.to_dict())
            except Exception as e:
                yield "data:" + json.dumps({"retcode": 500, "retmsg": str(e),
                                            "data": {"answer": "**ERROR**: "+str(e), "reference": []}},
                                           ensure_ascii=False) + "\n\n"
            yield "data:"+json.dumps({"retcode": 0, "retmsg": "", "data": True}, ensure_ascii=False) + "\n\n"

        if req.get("stream", True):
            resp = Response(stream(), mimetype="text/event-stream")
            resp.headers.add_header("Cache-control", "no-cache")
            resp.headers.add_header("Connection", "keep-alive")
            resp.headers.add_header("X-Accel-Buffering", "no")
            resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
            return resp
        else:
            ans = chat(dia, msg, False, **req)
            fillin_conv(ans)
            API4ConversationService.append_message(conv.id, conv.to_dict())
            return get_json_result(data=ans)

    except Exception as e:
        return server_error_response(e)


@manager.route('/conversation/<conversation_id>', methods=['GET'])
# @login_required
def get(conversation_id):
    try:
        e, conv = API4ConversationService.get_by_id(conversation_id)
        if not e:
            return get_data_error_result(retmsg="Conversation not found!")

        return get_json_result(data=conv.to_dict())
    except Exception as e:
        return server_error_response(e)


@manager.route('/document/upload', methods=['POST'])
@validate_request("kb_name")
def upload():
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, retmsg='Token is not valid!"', retcode=RetCode.AUTHENTICATION_ERROR)

    kb_name = request.form.get("kb_name").strip()
    tenant_id = objs[0].tenant_id

    try:
        e, kb = KnowledgebaseService.get_by_name(kb_name, tenant_id)
        if not e:
            return get_data_error_result(
                retmsg="Can't find this knowledgebase!")
        kb_id = kb.id
    except Exception as e:
        return server_error_response(e)

    if 'file' not in request.files:
        return get_json_result(
            data=False, retmsg='No file part!', retcode=RetCode.ARGUMENT_ERROR)

    file = request.files['file']
    if file.filename == '':
        return get_json_result(
            data=False, retmsg='No file selected!', retcode=RetCode.ARGUMENT_ERROR)

    root_folder = FileService.get_root_folder(tenant_id)
    pf_id = root_folder["id"]
    FileService.init_knowledgebase_docs(pf_id, tenant_id)
    kb_root_folder = FileService.get_kb_folder(tenant_id)
    kb_folder = FileService.new_a_file_from_kb(kb.tenant_id, kb.name, kb_root_folder["id"])

    try:
        if DocumentService.get_doc_count(kb.tenant_id) >= int(os.environ.get('MAX_FILE_NUM_PER_USER', 8192)):
            return get_data_error_result(
                retmsg="Exceed the maximum file number of a free user!")

        filename = duplicate_name(
            DocumentService.query,
            name=file.filename,
            kb_id=kb_id)
        filetype = filename_type(filename)
        if not filetype:
            return get_data_error_result(
                retmsg="This type of file has not been supported yet!")

        location = filename
        while MINIO.obj_exist(kb_id, location):
            location += "_"
        blob = request.files['file'].read()
        MINIO.put(kb_id, location, blob)
        doc = {
            "id": get_uuid(),
            "kb_id": kb.id,
            "parser_id": kb.parser_id,
            "parser_config": kb.parser_config,
            "created_by": kb.tenant_id,
            "type": filetype,
            "name": filename,
            "location": location,
            "size": len(blob),
            "thumbnail": thumbnail(filename, blob)
        }

        form_data=request.form
        if "parser_id" in form_data.keys():
            if request.form.get("parser_id").strip() in list(vars(ParserType).values())[1:-3]:
                doc["parser_id"] = request.form.get("parser_id").strip()
        if doc["type"] == FileType.VISUAL:
            doc["parser_id"] = ParserType.PICTURE.value
        if re.search(r"\.(ppt|pptx|pages)$", filename):
            doc["parser_id"] = ParserType.PRESENTATION.value

        doc_result = DocumentService.insert(doc)
        FileService.add_file_from_kb(doc, kb_folder["id"], kb.tenant_id)
    except Exception as e:
        return server_error_response(e)

    if "run" in form_data.keys():
        if request.form.get("run").strip() == "1":
            try:
                info = {"run": 1, "progress": 0}
                info["progress_msg"] = ""
                info["chunk_num"] = 0
                info["token_num"] = 0
                DocumentService.update_by_id(doc["id"], info)
                # if str(req["run"]) == TaskStatus.CANCEL.value:
                tenant_id = DocumentService.get_tenant_id(doc["id"])
                if not tenant_id:
                    return get_data_error_result(retmsg="Tenant not found!")

                #e, doc = DocumentService.get_by_id(doc["id"])
                TaskService.filter_delete([Task.doc_id == doc["id"]])
                e, doc = DocumentService.get_by_id(doc["id"])
                doc = doc.to_dict()
                doc["tenant_id"] = tenant_id
                bucket, name = File2DocumentService.get_minio_address(doc_id=doc["id"])
                queue_tasks(doc, bucket, name)
            except Exception as e:
                 return server_error_response(e)

    return get_json_result(data=doc_result.to_json())


@manager.route('/list_chunks', methods=['POST'])
# @login_required
def list_chunks():
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, retmsg='Token is not valid!"', retcode=RetCode.AUTHENTICATION_ERROR)

    form_data = request.form

    try:
        if "doc_name" in form_data.keys():
            tenant_id = DocumentService.get_tenant_id_by_name(form_data['doc_name'])
            q = Q("match", docnm_kwd=form_data['doc_name'])

        elif "doc_id" in form_data.keys():
            tenant_id = DocumentService.get_tenant_id(form_data['doc_id'])
            q = Q("match", doc_id=form_data['doc_id'])
        else:
            return get_json_result(
                data=False,retmsg="Can't find doc_name or doc_id"
            )

        res_es_search = ELASTICSEARCH.search(q,idxnm=search.index_name(tenant_id),timeout="600s")

        res = [{} for _ in range(len(res_es_search['hits']['hits']))]

        for index , chunk in enumerate(res_es_search['hits']['hits']):
            res[index]['doc_name'] = chunk['_source']['docnm_kwd']
            res[index]['content'] = chunk['_source']['content_with_weight']
            if 'img_id' in chunk['_source'].keys():
                res[index]['img_id'] = chunk['_source']['img_id']

    except Exception as e:
        return server_error_response(e)

    return get_json_result(data=res)

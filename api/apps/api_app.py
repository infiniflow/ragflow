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
from api.db.services.llm_service import LLMBundle
from flask_login import login_required, current_user

from api.db import VALID_FILE_TYPES, VALID_TASK_STATUS, FileType, LLMType, ParserType, FileSource
from api.db.db_models import APIToken, Task, File
from api.db.services import duplicate_name
from api.db.services.api_service import APITokenService, API4ConversationService
from api.db.services.dialog_service import DialogService, chat
from api.db.services.document_service import DocumentService, doc_upload_and_parse
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.task_service import queue_tasks, TaskService
from api.db.services.user_service import UserTenantService
from api import settings
from api.utils import get_uuid, current_timestamp, datetime_format
from api.utils.api_utils import server_error_response, get_data_error_result, get_json_result, validate_request, \
    generate_confirmation_token

from api.utils.file_utils import filename_type, thumbnail
from rag.app.tag import label_question
from rag.prompts import keyword_extraction
from rag.utils.storage_factory import STORAGE_IMPL

from api.db.services.canvas_service import UserCanvasService
from agent.canvas import Canvas
from functools import partial
from pathlib import Path


@manager.route('/new_token', methods=['POST'])  # noqa: F821
@login_required
def new_token():
    req = request.json
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")

        tenant_id = tenants[0].tenant_id
        obj = {"tenant_id": tenant_id, "token": generate_confirmation_token(tenant_id),
               "create_time": current_timestamp(),
               "create_date": datetime_format(datetime.now()),
               "update_time": None,
               "update_date": None
               }
        if req.get("canvas_id"):
            obj["dialog_id"] = req["canvas_id"]
            obj["source"] = "agent"
        else:
            obj["dialog_id"] = req["dialog_id"]

        if not APITokenService.save(**obj):
            return get_data_error_result(message="Fail to new a dialog!")

        return get_json_result(data=obj)
    except Exception as e:
        return server_error_response(e)


@manager.route('/token_list', methods=['GET'])  # noqa: F821
@login_required
def token_list():
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")

        id = request.args["dialog_id"] if "dialog_id" in request.args else request.args["canvas_id"]
        objs = APITokenService.query(tenant_id=tenants[0].tenant_id, dialog_id=id)
        return get_json_result(data=[o.to_dict() for o in objs])
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['POST'])  # noqa: F821
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


@manager.route('/stats', methods=['GET'])  # noqa: F821
@login_required
def stats():
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")
        objs = API4ConversationService.stats(
            tenants[0].tenant_id,
            request.args.get(
                "from_date",
                (datetime.now() -
                 timedelta(
                     days=7)).strftime("%Y-%m-%d 00:00:00")),
            request.args.get(
                "to_date",
                datetime.now().strftime("%Y-%m-%d %H:%M:%S")),
            "agent" if "canvas_id" in request.args else None)
        res = {
            "pv": [(o["dt"], o["pv"]) for o in objs],
            "uv": [(o["dt"], o["uv"]) for o in objs],
            "speed": [(o["dt"], float(o["tokens"]) / (float(o["duration"] + 0.1))) for o in objs],
            "tokens": [(o["dt"], float(o["tokens"]) / 1000.) for o in objs],
            "round": [(o["dt"], o["round"]) for o in objs],
            "thumb_up": [(o["dt"], o["thumb_up"]) for o in objs]
        }
        return get_json_result(data=res)
    except Exception as e:
        return server_error_response(e)


@manager.route('/new_conversation', methods=['GET'])  # noqa: F821
def set_conversation():
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, message='Authentication error: API key is invalid!"', code=settings.RetCode.AUTHENTICATION_ERROR)
    try:
        if objs[0].source == "agent":
            e, cvs = UserCanvasService.get_by_id(objs[0].dialog_id)
            if not e:
                return server_error_response("canvas not found.")
            if not isinstance(cvs.dsl, str):
                cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)
            canvas = Canvas(cvs.dsl, objs[0].tenant_id)
            conv = {
                "id": get_uuid(),
                "dialog_id": cvs.id,
                "user_id": request.args.get("user_id", ""),
                "message": [{"role": "assistant", "content": canvas.get_prologue()}],
                "source": "agent"
            }
            API4ConversationService.save(**conv)
            return get_json_result(data=conv)
        else:
            e, dia = DialogService.get_by_id(objs[0].dialog_id)
            if not e:
                return get_data_error_result(message="Dialog not found")
            conv = {
                "id": get_uuid(),
                "dialog_id": dia.id,
                "user_id": request.args.get("user_id", ""),
                "message": [{"role": "assistant", "content": dia.prompt_config["prologue"]}]
            }
            API4ConversationService.save(**conv)
            return get_json_result(data=conv)
    except Exception as e:
        return server_error_response(e)


@manager.route('/completion', methods=['POST'])  # noqa: F821
@validate_request("conversation_id", "messages")
def completion():
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, message='Authentication error: API key is invalid!"', code=settings.RetCode.AUTHENTICATION_ERROR)
    req = request.json
    e, conv = API4ConversationService.get_by_id(req["conversation_id"])
    if not e:
        return get_data_error_result(message="Conversation not found!")
    if "quote" not in req:
        req["quote"] = False

    msg = []
    for m in req["messages"]:
        if m["role"] == "system":
            continue
        if m["role"] == "assistant" and not msg:
            continue
        msg.append(m)
    if not msg[-1].get("id"):
        msg[-1]["id"] = get_uuid()
    message_id = msg[-1]["id"]

    def fillin_conv(ans):
        nonlocal conv, message_id
        if not conv.reference:
            conv.reference.append(ans["reference"])
        else:
            conv.reference[-1] = ans["reference"]
        conv.message[-1] = {"role": "assistant", "content": ans["answer"], "id": message_id}
        ans["id"] = message_id

    def rename_field(ans):
        reference = ans['reference']
        if not isinstance(reference, dict):
            return
        for chunk_i in reference.get('chunks', []):
            if 'docnm_kwd' in chunk_i:
                chunk_i['doc_name'] = chunk_i['docnm_kwd']
                chunk_i.pop('docnm_kwd')

    try:
        if conv.source == "agent":
            stream = req.get("stream", True)
            conv.message.append(msg[-1])
            e, cvs = UserCanvasService.get_by_id(conv.dialog_id)
            if not e:
                return server_error_response("canvas not found.")
            del req["conversation_id"]
            del req["messages"]

            if not isinstance(cvs.dsl, str):
                cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)

            if not conv.reference:
                conv.reference = []
            conv.message.append({"role": "assistant", "content": "", "id": message_id})
            conv.reference.append({"chunks": [], "doc_aggs": []})

            final_ans = {"reference": [], "content": ""}
            canvas = Canvas(cvs.dsl, objs[0].tenant_id)

            canvas.messages.append(msg[-1])
            canvas.add_user_input(msg[-1]["content"])
            answer = canvas.run(stream=stream)

            assert answer is not None, "Nothing. Is it over?"

            if stream:
                assert isinstance(answer, partial), "Nothing. Is it over?"

                def sse():
                    nonlocal answer, cvs, conv
                    try:
                        for ans in answer():
                            for k in ans.keys():
                                final_ans[k] = ans[k]
                            ans = {"answer": ans["content"], "reference": ans.get("reference", [])}
                            fillin_conv(ans)
                            rename_field(ans)
                            yield "data:" + json.dumps({"code": 0, "message": "", "data": ans},
                                                       ensure_ascii=False) + "\n\n"

                        canvas.messages.append({"role": "assistant", "content": final_ans["content"], "id": message_id})
                        canvas.history.append(("assistant", final_ans["content"]))
                        if final_ans.get("reference"):
                            canvas.reference.append(final_ans["reference"])
                        cvs.dsl = json.loads(str(canvas))
                        API4ConversationService.append_message(conv.id, conv.to_dict())
                    except Exception as e:
                        yield "data:" + json.dumps({"code": 500, "message": str(e),
                                                    "data": {"answer": "**ERROR**: " + str(e), "reference": []}},
                                                   ensure_ascii=False) + "\n\n"
                    yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"

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

            result = {"answer": final_ans["content"], "reference": final_ans.get("reference", [])}
            fillin_conv(result)
            API4ConversationService.append_message(conv.id, conv.to_dict())
            rename_field(result)
            return get_json_result(data=result)

        # ******************For dialog******************
        conv.message.append(msg[-1])
        e, dia = DialogService.get_by_id(conv.dialog_id)
        if not e:
            return get_data_error_result(message="Dialog not found!")
        del req["conversation_id"]
        del req["messages"]

        if not conv.reference:
            conv.reference = []
        conv.message.append({"role": "assistant", "content": "", "id": message_id})
        conv.reference.append({"chunks": [], "doc_aggs": []})

        def stream():
            nonlocal dia, msg, req, conv
            try:
                for ans in chat(dia, msg, True, **req):
                    fillin_conv(ans)
                    rename_field(ans)
                    yield "data:" + json.dumps({"code": 0, "message": "", "data": ans},
                                               ensure_ascii=False) + "\n\n"
                API4ConversationService.append_message(conv.id, conv.to_dict())
            except Exception as e:
                yield "data:" + json.dumps({"code": 500, "message": str(e),
                                            "data": {"answer": "**ERROR**: " + str(e), "reference": []}},
                                           ensure_ascii=False) + "\n\n"
            yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"

        if req.get("stream", True):
            resp = Response(stream(), mimetype="text/event-stream")
            resp.headers.add_header("Cache-control", "no-cache")
            resp.headers.add_header("Connection", "keep-alive")
            resp.headers.add_header("X-Accel-Buffering", "no")
            resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
            return resp

        answer = None
        for ans in chat(dia, msg, **req):
            answer = ans
            fillin_conv(ans)
            API4ConversationService.append_message(conv.id, conv.to_dict())
            break
        rename_field(answer)
        return get_json_result(data=answer)

    except Exception as e:
        return server_error_response(e)


@manager.route('/conversation/<conversation_id>', methods=['GET'])  # noqa: F821
# @login_required
def get_conversation(conversation_id):
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, message='Authentication error: API key is invalid!"', code=settings.RetCode.AUTHENTICATION_ERROR)

    try:
        e, conv = API4ConversationService.get_by_id(conversation_id)
        if not e:
            return get_data_error_result(message="Conversation not found!")

        conv = conv.to_dict()
        if token != APIToken.query(dialog_id=conv['dialog_id'])[0].token:
            return get_json_result(data=False, message='Authentication error: API key is invalid for this conversation_id!"',
                                   code=settings.RetCode.AUTHENTICATION_ERROR)

        for referenct_i in conv['reference']:
            if referenct_i is None or len(referenct_i) == 0:
                continue
            for chunk_i in referenct_i['chunks']:
                if 'docnm_kwd' in chunk_i.keys():
                    chunk_i['doc_name'] = chunk_i['docnm_kwd']
                    chunk_i.pop('docnm_kwd')
        return get_json_result(data=conv)
    except Exception as e:
        return server_error_response(e)


@manager.route('/document/upload', methods=['POST'])  # noqa: F821
@validate_request("kb_name")
def upload():
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, message='Authentication error: API key is invalid!"', code=settings.RetCode.AUTHENTICATION_ERROR)

    kb_name = request.form.get("kb_name").strip()
    tenant_id = objs[0].tenant_id

    try:
        e, kb = KnowledgebaseService.get_by_name(kb_name, tenant_id)
        if not e:
            return get_data_error_result(
                message="Can't find this knowledgebase!")
        kb_id = kb.id
    except Exception as e:
        return server_error_response(e)

    if 'file' not in request.files:
        return get_json_result(
            data=False, message='No file part!', code=settings.RetCode.ARGUMENT_ERROR)

    file = request.files['file']
    if file.filename == '':
        return get_json_result(
            data=False, message='No file selected!', code=settings.RetCode.ARGUMENT_ERROR)

    root_folder = FileService.get_root_folder(tenant_id)
    pf_id = root_folder["id"]
    FileService.init_knowledgebase_docs(pf_id, tenant_id)
    kb_root_folder = FileService.get_kb_folder(tenant_id)
    kb_folder = FileService.new_a_file_from_kb(kb.tenant_id, kb.name, kb_root_folder["id"])

    try:
        if DocumentService.get_doc_count(kb.tenant_id) >= int(os.environ.get('MAX_FILE_NUM_PER_USER', 8192)):
            return get_data_error_result(
                message="Exceed the maximum file number of a free user!")

        filename = duplicate_name(
            DocumentService.query,
            name=file.filename,
            kb_id=kb_id)
        filetype = filename_type(filename)
        if not filetype:
            return get_data_error_result(
                message="This type of file has not been supported yet!")

        location = filename
        while STORAGE_IMPL.obj_exist(kb_id, location):
            location += "_"
        blob = request.files['file'].read()
        STORAGE_IMPL.put(kb_id, location, blob)
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
            "thumbnail": thumbnail(filename, blob),
            "suffix": Path(filename).suffix.lstrip("."),
        }

        form_data = request.form
        if "parser_id" in form_data.keys():
            if request.form.get("parser_id").strip() in list(vars(ParserType).values())[1:-3]:
                doc["parser_id"] = request.form.get("parser_id").strip()
        if doc["type"] == FileType.VISUAL:
            doc["parser_id"] = ParserType.PICTURE.value
        if doc["type"] == FileType.AURAL:
            doc["parser_id"] = ParserType.AUDIO.value
        if re.search(r"\.(ppt|pptx|pages)$", filename):
            doc["parser_id"] = ParserType.PRESENTATION.value
        if re.search(r"\.(eml)$", filename):
            doc["parser_id"] = ParserType.EMAIL.value

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
                    return get_data_error_result(message="Tenant not found!")

                # e, doc = DocumentService.get_by_id(doc["id"])
                TaskService.filter_delete([Task.doc_id == doc["id"]])
                e, doc = DocumentService.get_by_id(doc["id"])
                doc = doc.to_dict()
                doc["tenant_id"] = tenant_id
                bucket, name = File2DocumentService.get_storage_address(doc_id=doc["id"])
                queue_tasks(doc, bucket, name, 0)
            except Exception as e:
                return server_error_response(e)

    return get_json_result(data=doc_result.to_json())


@manager.route('/document/upload_and_parse', methods=['POST'])  # noqa: F821
@validate_request("conversation_id")
def upload_parse():
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, message='Authentication error: API key is invalid!"', code=settings.RetCode.AUTHENTICATION_ERROR)

    if 'file' not in request.files:
        return get_json_result(
            data=False, message='No file part!', code=settings.RetCode.ARGUMENT_ERROR)

    file_objs = request.files.getlist('file')
    for file_obj in file_objs:
        if file_obj.filename == '':
            return get_json_result(
                data=False, message='No file selected!', code=settings.RetCode.ARGUMENT_ERROR)

    doc_ids = doc_upload_and_parse(request.form.get("conversation_id"), file_objs, objs[0].tenant_id)
    return get_json_result(data=doc_ids)


@manager.route('/list_chunks', methods=['POST'])  # noqa: F821
# @login_required
def list_chunks():
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, message='Authentication error: API key is invalid!"', code=settings.RetCode.AUTHENTICATION_ERROR)

    req = request.json

    try:
        if "doc_name" in req.keys():
            tenant_id = DocumentService.get_tenant_id_by_name(req['doc_name'])
            doc_id = DocumentService.get_doc_id_by_doc_name(req['doc_name'])

        elif "doc_id" in req.keys():
            tenant_id = DocumentService.get_tenant_id(req['doc_id'])
            doc_id = req['doc_id']
        else:
            return get_json_result(
                data=False, message="Can't find doc_name or doc_id"
            )
        kb_ids = KnowledgebaseService.get_kb_ids(tenant_id)

        res = settings.retrievaler.chunk_list(doc_id, tenant_id, kb_ids)
        res = [
            {
                "content": res_item["content_with_weight"],
                "doc_name": res_item["docnm_kwd"],
                "image_id": res_item["img_id"]
            } for res_item in res
        ]

    except Exception as e:
        return server_error_response(e)

    return get_json_result(data=res)

@manager.route('/get_chunk/<chunk_id>', methods=['GET'])  # noqa: F821
# @login_required
def get_chunk(chunk_id):
    from rag.nlp import search
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, message='Authentication error: API key is invalid!"', code=settings.RetCode.AUTHENTICATION_ERROR)
    try:
        tenant_id = objs[0].tenant_id
        kb_ids = KnowledgebaseService.get_kb_ids(tenant_id)
        chunk = settings.docStoreConn.get(chunk_id, search.index_name(tenant_id), kb_ids)
        if chunk is None:
            return server_error_response(Exception("Chunk not found"))
        k = []
        for n in chunk.keys():
            if re.search(r"(_vec$|_sm_|_tks|_ltks)", n):
                k.append(n)
        for n in k:
            del chunk[n]

        return get_json_result(data=chunk)
    except Exception as e:
        return server_error_response(e)

@manager.route('/list_kb_docs', methods=['POST'])  # noqa: F821
# @login_required
def list_kb_docs():
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, message='Authentication error: API key is invalid!"', code=settings.RetCode.AUTHENTICATION_ERROR)

    req = request.json
    tenant_id = objs[0].tenant_id
    kb_name = req.get("kb_name", "").strip()

    try:
        e, kb = KnowledgebaseService.get_by_name(kb_name, tenant_id)
        if not e:
            return get_data_error_result(
                message="Can't find this knowledgebase!")
        kb_id = kb.id

    except Exception as e:
        return server_error_response(e)

    page_number = int(req.get("page", 1))
    items_per_page = int(req.get("page_size", 15))
    orderby = req.get("orderby", "create_time")
    desc = req.get("desc", True)
    keywords = req.get("keywords", "")
    status = req.get("status", [])
    if status:
        invalid_status = {s for s in status if s not in VALID_TASK_STATUS}
        if invalid_status:
            return get_data_error_result(
                message=f"Invalid filter status conditions: {', '.join(invalid_status)}"
            )
    types = req.get("types", [])
    if types:
        invalid_types = {t for t in types if t not in VALID_FILE_TYPES}
        if invalid_types:
            return get_data_error_result(
                message=f"Invalid filter conditions: {', '.join(invalid_types)} type{'s' if len(invalid_types) > 1 else ''}"
            )
    try:
        docs, tol = DocumentService.get_by_kb_id(
            kb_id, page_number, items_per_page, orderby, desc, keywords, status, types)
        docs = [{"doc_id": doc['id'], "doc_name": doc['name']} for doc in docs]

        return get_json_result(data={"total": tol, "docs": docs})

    except Exception as e:
        return server_error_response(e)


@manager.route('/document/infos', methods=['POST'])  # noqa: F821
@validate_request("doc_ids")
def docinfos():
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, message='Authentication error: API key is invalid!"', code=settings.RetCode.AUTHENTICATION_ERROR)
    req = request.json
    doc_ids = req["doc_ids"]
    docs = DocumentService.get_by_ids(doc_ids)
    return get_json_result(data=list(docs.dicts()))


@manager.route('/document', methods=['DELETE'])  # noqa: F821
# @login_required
def document_rm():
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, message='Authentication error: API key is invalid!"', code=settings.RetCode.AUTHENTICATION_ERROR)

    tenant_id = objs[0].tenant_id
    req = request.json
    try:
        doc_ids = DocumentService.get_doc_ids_by_doc_names(req.get("doc_names", []))
        for doc_id in req.get("doc_ids", []):
            if doc_id not in doc_ids:
                doc_ids.append(doc_id)

        if not doc_ids:
            return get_json_result(
                data=False, message="Can't find doc_names or doc_ids"
            )

    except Exception as e:
        return server_error_response(e)

    root_folder = FileService.get_root_folder(tenant_id)
    pf_id = root_folder["id"]
    FileService.init_knowledgebase_docs(pf_id, tenant_id)

    errors = ""
    docs = DocumentService.get_by_ids(doc_ids)
    doc_dic = {}
    for doc in docs:
        doc_dic[doc.id] = doc

    for doc_id in doc_ids:
        try:
            if doc_id not in doc_dic:
                return get_data_error_result(message="Document not found!")
            doc = doc_dic[doc_id]
            tenant_id = DocumentService.get_tenant_id(doc_id)
            if not tenant_id:
                return get_data_error_result(message="Tenant not found!")

            b, n = File2DocumentService.get_storage_address(doc_id=doc_id)

            if not DocumentService.remove_document(doc, tenant_id):
                return get_data_error_result(
                    message="Database error (Document removal)!")

            f2d = File2DocumentService.get_by_document_id(doc_id)
            FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == f2d[0].file_id])
            File2DocumentService.delete_by_document_id(doc_id)

            STORAGE_IMPL.rm(b, n)
        except Exception as e:
            errors += str(e)

    if errors:
        return get_json_result(data=False, message=errors, code=settings.RetCode.SERVER_ERROR)

    return get_json_result(data=True)


@manager.route('/completion_aibotk', methods=['POST'])  # noqa: F821
@validate_request("Authorization", "conversation_id", "word")
def completion_faq():
    import base64
    req = request.json

    token = req["Authorization"]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, message='Authentication error: API key is invalid!"', code=settings.RetCode.AUTHENTICATION_ERROR)

    e, conv = API4ConversationService.get_by_id(req["conversation_id"])
    if not e:
        return get_data_error_result(message="Conversation not found!")
    if "quote" not in req:
        req["quote"] = True

    msg = []
    msg.append({"role": "user", "content": req["word"]})
    if not msg[-1].get("id"):
        msg[-1]["id"] = get_uuid()
    message_id = msg[-1]["id"]

    def fillin_conv(ans):
        nonlocal conv, message_id
        if not conv.reference:
            conv.reference.append(ans["reference"])
        else:
            conv.reference[-1] = ans["reference"]
        conv.message[-1] = {"role": "assistant", "content": ans["answer"], "id": message_id}
        ans["id"] = message_id

    try:
        if conv.source == "agent":
            conv.message.append(msg[-1])
            e, cvs = UserCanvasService.get_by_id(conv.dialog_id)
            if not e:
                return server_error_response("canvas not found.")

            if not isinstance(cvs.dsl, str):
                cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)

            if not conv.reference:
                conv.reference = []
            conv.message.append({"role": "assistant", "content": "", "id": message_id})
            conv.reference.append({"chunks": [], "doc_aggs": []})

            final_ans = {"reference": [], "doc_aggs": []}
            canvas = Canvas(cvs.dsl, objs[0].tenant_id)

            canvas.messages.append(msg[-1])
            canvas.add_user_input(msg[-1]["content"])
            answer = canvas.run(stream=False)

            assert answer is not None, "Nothing. Is it over?"

            data_type_picture = {
                "type": 3,
                "url": "base64 content"
            }
            data = [
                {
                    "type": 1,
                    "content": ""
                }
            ]
            final_ans["content"] = "\n".join(answer["content"]) if "content" in answer else ""
            canvas.messages.append({"role": "assistant", "content": final_ans["content"], "id": message_id})
            if final_ans.get("reference"):
                canvas.reference.append(final_ans["reference"])
            cvs.dsl = json.loads(str(canvas))

            ans = {"answer": final_ans["content"], "reference": final_ans.get("reference", [])}
            data[0]["content"] += re.sub(r'##\d\$\$', '', ans["answer"])
            fillin_conv(ans)
            API4ConversationService.append_message(conv.id, conv.to_dict())

            chunk_idxs = [int(match[2]) for match in re.findall(r'##\d\$\$', ans["answer"])]
            for chunk_idx in chunk_idxs[:1]:
                if ans["reference"]["chunks"][chunk_idx]["img_id"]:
                    try:
                        bkt, nm = ans["reference"]["chunks"][chunk_idx]["img_id"].split("-")
                        response = STORAGE_IMPL.get(bkt, nm)
                        data_type_picture["url"] = base64.b64encode(response).decode('utf-8')
                        data.append(data_type_picture)
                        break
                    except Exception as e:
                        return server_error_response(e)

            response = {"code": 200, "msg": "success", "data": data}
            return response

        # ******************For dialog******************
        conv.message.append(msg[-1])
        e, dia = DialogService.get_by_id(conv.dialog_id)
        if not e:
            return get_data_error_result(message="Dialog not found!")
        del req["conversation_id"]

        if not conv.reference:
            conv.reference = []
        conv.message.append({"role": "assistant", "content": "", "id": message_id})
        conv.reference.append({"chunks": [], "doc_aggs": []})

        data_type_picture = {
            "type": 3,
            "url": "base64 content"
        }
        data = [
            {
                "type": 1,
                "content": ""
            }
        ]
        ans = ""
        for a in chat(dia, msg, stream=False, **req):
            ans = a
            break
        data[0]["content"] += re.sub(r'##\d\$\$', '', ans["answer"])
        fillin_conv(ans)
        API4ConversationService.append_message(conv.id, conv.to_dict())

        chunk_idxs = [int(match[2]) for match in re.findall(r'##\d\$\$', ans["answer"])]
        for chunk_idx in chunk_idxs[:1]:
            if ans["reference"]["chunks"][chunk_idx]["img_id"]:
                try:
                    bkt, nm = ans["reference"]["chunks"][chunk_idx]["img_id"].split("-")
                    response = STORAGE_IMPL.get(bkt, nm)
                    data_type_picture["url"] = base64.b64encode(response).decode('utf-8')
                    data.append(data_type_picture)
                    break
                except Exception as e:
                    return server_error_response(e)

        response = {"code": 200, "msg": "success", "data": data}
        return response

    except Exception as e:
        return server_error_response(e)


@manager.route('/retrieval', methods=['POST'])  # noqa: F821
@validate_request("kb_id", "question")
def retrieval():
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, message='Authentication error: API key is invalid!"', code=settings.RetCode.AUTHENTICATION_ERROR)

    req = request.json
    kb_ids = req.get("kb_id", [])
    doc_ids = req.get("doc_ids", [])
    question = req.get("question")
    page = int(req.get("page", 1))
    size = int(req.get("page_size", 30))
    similarity_threshold = float(req.get("similarity_threshold", 0.2))
    vector_similarity_weight = float(req.get("vector_similarity_weight", 0.3))
    top = int(req.get("top_k", 1024))
    highlight = bool(req.get("highlight", False)) 

    try:
        kbs = KnowledgebaseService.get_by_ids(kb_ids)
        embd_nms = list(set([kb.embd_id for kb in kbs]))
        if len(embd_nms) != 1:
            return get_json_result(
                data=False, message='Knowledge bases use different embedding models or does not exist."',
                code=settings.RetCode.AUTHENTICATION_ERROR)

        embd_mdl = LLMBundle(kbs[0].tenant_id, LLMType.EMBEDDING, llm_name=kbs[0].embd_id)
        rerank_mdl = None
        if req.get("rerank_id"):
            rerank_mdl = LLMBundle(kbs[0].tenant_id, LLMType.RERANK, llm_name=req["rerank_id"])
        if req.get("keyword", False):
            chat_mdl = LLMBundle(kbs[0].tenant_id, LLMType.CHAT)
            question += keyword_extraction(chat_mdl, question)
        ranks = settings.retrievaler.retrieval(question, embd_mdl, kbs[0].tenant_id, kb_ids, page, size,
                                               similarity_threshold, vector_similarity_weight, top,
                                               doc_ids, rerank_mdl=rerank_mdl, highlight= highlight,
                                               rank_feature=label_question(question, kbs))
        for c in ranks["chunks"]:
            c.pop("vector", None)
        return get_json_result(data=ranks)
    except Exception as e:
        if str(e).find("not_found") > 0:
            return get_json_result(data=False, message='No chunk found! Check the chunk status please!',
                                   code=settings.RetCode.DATA_ERROR)
        return server_error_response(e)

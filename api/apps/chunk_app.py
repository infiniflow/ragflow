#
#  Copyright 2019 The RAG Flow Authors. All Rights Reserved.
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
import base64
import hashlib
import pathlib
import re

from elasticsearch_dsl import Q
from flask import request
from flask_login import login_required, current_user

from rag.nlp import search, huqie
from rag.utils import ELASTICSEARCH, rmSpace
from web_server.db import LLMType
from web_server.db.services import duplicate_name
from web_server.db.services.kb_service import KnowledgebaseService
from web_server.db.services.llm_service import TenantLLMService
from web_server.db.services.user_service import UserTenantService
from web_server.utils.api_utils import server_error_response, get_data_error_result, validate_request
from web_server.utils import get_uuid
from web_server.db.services.document_service import DocumentService
from web_server.settings import RetCode
from web_server.utils.api_utils import get_json_result
from rag.utils.minio_conn import MINIO
from web_server.utils.file_utils import filename_type

retrival = search.Dealer(ELASTICSEARCH, None)

@manager.route('/list', methods=['POST'])
@login_required
@validate_request("doc_id")
def list():
    req = request.json
    doc_id = req["doc_id"]
    page = req.get("page", 1)
    size = req.get("size", 30)
    question = req.get("keywords", "")
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(retmsg="Tenant not found!")
        res = retrival.search({
            "doc_ids": [doc_id], "page": page, "size": size, "question": question
        }, search.index_name(tenants[0].tenant_id))
        return get_json_result(data=res)
    except Exception as e:
        if str(e).find("not_found") > 0:
            return get_json_result(data=False, retmsg=f'Index not found!',
                            retcode=RetCode.DATA_ERROR)
        return server_error_response(e)


@manager.route('/get', methods=['GET'])
@login_required
def get():
    chunk_id = request.args["chunk_id"]
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(retmsg="Tenant not found!")
        res = ELASTICSEARCH.get(chunk_id, search.index_name(tenants[0].tenant_id))
        if not res.get("found"):return server_error_response("Chunk not found")
        id = res["_id"]
        res = res["_source"]
        res["chunk_id"] = id
        k = []
        for n in res.keys():
            if re.search(r"(_vec$|_sm_)", n):
                k.append(n)
            if re.search(r"(_tks|_ltks)", n):
                res[n] = rmSpace(res[n])
        for n in k: del res[n]

        return get_json_result(data=res)
    except Exception as e:
        if str(e).find("NotFoundError") >= 0:
            return get_json_result(data=False, retmsg=f'Chunk not found!',
                                   retcode=RetCode.DATA_ERROR)
        return server_error_response(e)


@manager.route('/set', methods=['POST'])
@login_required
@validate_request("doc_id", "chunk_id", "content_ltks", "important_kwd", "docnm_kwd")
def set():
    req = request.json
    d = {"id": req["chunk_id"]}
    d["content_ltks"] = huqie.qie(req["content_ltks"])
    d["content_sm_ltks"] = huqie.qieqie(d["content_ltks"])
    d["important_kwd"] = req["important_kwd"]
    d["important_tks"] = huqie.qie(" ".join(req["important_kwd"]))

    try:
        tenant_id = DocumentService.get_tenant_id(req["doc_id"])
        if not tenant_id: return get_data_error_result(retmsg="Tenant not found!")
        embd_mdl = TenantLLMService.model_instance(tenant_id, LLMType.EMBEDDING.value)
        v, c = embd_mdl.encode([req["docnm_kwd"], req["content_ltks"]])
        v = 0.1 * v[0] + 0.9 * v[1]
        d["q_%d_vec"%len(v)] = v.tolist()
        ELASTICSEARCH.upsert([d], search.index_name(tenant_id))
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/create', methods=['POST'])
@login_required
@validate_request("doc_id", "content_ltks", "important_kwd")
def set():
    req = request.json
    md5 = hashlib.md5()
    md5.update((req["content_ltks"] + req["doc_id"]).encode("utf-8"))
    chunck_id = md5.hexdigest()
    d = {"id": chunck_id, "content_ltks": huqie.qie(req["content_ltks"])}
    d["content_sm_ltks"] = huqie.qieqie(d["content_ltks"])
    d["important_kwd"] = req["important_kwd"]
    d["important_tks"] = huqie.qie(" ".join(req["important_kwd"]))

    try:
        e, doc = DocumentService.get_by_id(req["doc_id"])
        if not e: return get_data_error_result(retmsg="Document not found!")
        d["kb_id"] = [doc.kb_id]
        d["docnm_kwd"] = doc.name
        d["doc_id"] = doc.id

        tenant_id = DocumentService.get_tenant_id(req["doc_id"])
        if not tenant_id: return get_data_error_result(retmsg="Tenant not found!")

        embd_mdl = TenantLLMService.model_instance(tenant_id, LLMType.EMBEDDING.value)
        v, c = embd_mdl.encode([doc.name, req["content_ltks"]])
        DocumentService.increment_chunk_num(req["doc_id"], doc.kb_id, c, 1, 0)
        v = 0.1 * v[0] + 0.9 * v[1]
        d["q_%d_vec"%len(v)] = v.tolist()
        ELASTICSEARCH.upsert([d], search.index_name(tenant_id))
        return get_json_result(data={"chunk_id": chunck_id})
    except Exception as e:
        return server_error_response(e)

#
#  Copyright 2019 The InfiniFlow Authors. All Rights Reserved.
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
import hashlib
import re

import numpy as np
from flask import request
from flask_login import login_required, current_user

from rag.nlp import search, huqie
from rag.utils import ELASTICSEARCH, rmSpace
from api.db import LLMType
from api.db.services import duplicate_name
from api.db.services.kb_service import KnowledgebaseService
from api.db.services.llm_service import TenantLLMService
from api.db.services.user_service import UserTenantService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request
from api.db.services.document_service import DocumentService
from api.settings import RetCode
from api.utils.api_utils import get_json_result

retrival = search.Dealer(ELASTICSEARCH)

@manager.route('/list', methods=['POST'])
@login_required
@validate_request("doc_id")
def list():
    req = request.json
    doc_id = req["doc_id"]
    page = int(req.get("page", 1))
    size = int(req.get("size", 30))
    question = req.get("keywords", "")
    try:
        tenant_id = DocumentService.get_tenant_id(req["doc_id"])
        if not tenant_id: return get_data_error_result(retmsg="Tenant not found!")
        query = {
            "doc_ids": [doc_id], "page": page, "size": size, "question": question
        }
        if "available_int" in req: query["available_int"] = int(req["available_int"])
        sres = retrival.search(query, search.index_name(tenant_id))
        res = {"total": sres.total, "chunks": []}
        for id in sres.ids:
            d = {
                "chunk_id": id,
                "content_ltks": rmSpace(sres.highlight[id]) if question else sres.field[id]["content_ltks"],
                "doc_id": sres.field[id]["doc_id"],
                "docnm_kwd": sres.field[id]["docnm_kwd"],
                "important_kwd": sres.field[id].get("important_kwd", []),
                "img_id": sres.field[id].get("img_id", ""),
                "available_int": sres.field[id].get("available_int", 1),
            }
            res["chunks"].append(d)
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
    if "available_int" in req: d["available_int"] = req["available_int"]

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


@manager.route('/switch', methods=['POST'])
@login_required
@validate_request("chunk_ids", "available_int", "doc_id")
def switch():
    req = request.json
    try:
        tenant_id = DocumentService.get_tenant_id(req["doc_id"])
        if not tenant_id: return get_data_error_result(retmsg="Tenant not found!")
        if not ELASTICSEARCH.upsert([{"id": i, "available_int": int(req["available_int"])} for i in req["chunk_ids"]],
                             search.index_name(tenant_id)):
            return get_data_error_result(retmsg="Index updating failure")
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)



@manager.route('/create', methods=['POST'])
@login_required
@validate_request("doc_id", "content_ltks", "important_kwd")
def create():
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


@manager.route('/retrieval_test', methods=['POST'])
@login_required
@validate_request("kb_id", "question")
def retrieval_test():
    req = request.json
    page = int(req.get("page", 1))
    size = int(req.get("size", 30))
    question = req["question"]
    kb_id = req["kb_id"]
    doc_ids = req.get("doc_ids", [])
    similarity_threshold = float(req.get("similarity_threshold", 0.4))
    vector_similarity_weight = float(req.get("vector_similarity_weight", 0.3))
    top = int(req.get("top", 1024))
    try:
        e, kb = KnowledgebaseService.get_by_id(kb_id)
        if not e:
            return get_data_error_result(retmsg="Knowledgebase not found!")

        embd_mdl = TenantLLMService.model_instance(kb.tenant_id, LLMType.EMBEDDING.value)
        sres = retrival.search({"kb_ids": [kb_id], "doc_ids": doc_ids, "size": top,
                                "question": question, "vector": True,
                                "similarity": similarity_threshold},
                               search.index_name(kb.tenant_id),
                               embd_mdl)

        sim, tsim, vsim = retrival.rerank(sres, question, 1-vector_similarity_weight, vector_similarity_weight)
        idx = np.argsort(sim*-1)
        ranks = {"total": 0, "chunks": [], "doc_aggs": {}}
        start_idx = (page-1)*size
        for i in idx:
            ranks["total"] += 1
            if sim[i] < similarity_threshold: break
            start_idx -= 1
            if start_idx >= 0:continue
            if len(ranks["chunks"]) == size:continue
            id = sres.ids[i]
            dnm = sres.field[id]["docnm_kwd"]
            d = {
                "chunk_id": id,
                "content_ltks": sres.field[id]["content_ltks"],
                "doc_id": sres.field[id]["doc_id"],
                "docnm_kwd": dnm,
                "kb_id": sres.field[id]["kb_id"],
                "important_kwd": sres.field[id].get("important_kwd", []),
                "img_id": sres.field[id].get("img_id", ""),
                "similarity": sim[i],
                "vector_similarity": vsim[i],
                "term_similarity": tsim[i]
            }
            ranks["chunks"].append(d)
            if dnm not in ranks["doc_aggs"]:ranks["doc_aggs"][dnm] = 0
            ranks["doc_aggs"][dnm] += 1

        return get_json_result(data=ranks)
    except Exception as e:
        if str(e).find("not_found") > 0:
            return get_json_result(data=False, retmsg=f'Index not found!',
                            retcode=RetCode.DATA_ERROR)
        return server_error_response(e)
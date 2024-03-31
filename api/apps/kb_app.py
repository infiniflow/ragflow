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
from elasticsearch_dsl import Q
from flask import request
from flask_login import login_required, current_user

from api.db.services import duplicate_name
from api.db.services.document_service import DocumentService
from api.db.services.user_service import TenantService, UserTenantService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request
from api.utils import get_uuid, get_format_time
from api.db import StatusEnum, UserTenantRole
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.db_models import Knowledgebase
from api.settings import stat_logger, RetCode
from api.utils.api_utils import get_json_result
from rag.nlp import search
from rag.utils import ELASTICSEARCH


@manager.route('/create', methods=['post'])
@login_required
@validate_request("name")
def create():
    req = request.json
    req["name"] = req["name"].strip()
    req["name"] = duplicate_name(
        KnowledgebaseService.query,
        name=req["name"],
        tenant_id=current_user.id,
        status=StatusEnum.VALID.value)
    try:
        req["id"] = get_uuid()
        req["tenant_id"] = current_user.id
        req["created_by"] = current_user.id
        e, t = TenantService.get_by_id(current_user.id)
        if not e:
            return get_data_error_result(retmsg="Tenant not found.")
        req["embd_id"] = t.embd_id
        if not KnowledgebaseService.save(**req):
            return get_data_error_result()
        return get_json_result(data={"kb_id": req["id"]})
    except Exception as e:
        return server_error_response(e)


@manager.route('/update', methods=['post'])
@login_required
@validate_request("kb_id", "name", "description", "permission", "parser_id")
def update():
    req = request.json
    req["name"] = req["name"].strip()
    try:
        if not KnowledgebaseService.query(
                created_by=current_user.id, id=req["kb_id"]):
            return get_json_result(
                data=False, retmsg=f'Only owner of knowledgebase authorized for this operation.', retcode=RetCode.OPERATING_ERROR)

        e, kb = KnowledgebaseService.get_by_id(req["kb_id"])
        if not e:
            return get_data_error_result(
                retmsg="Can't find this knowledgebase!")

        if req["name"].lower() != kb.name.lower() \
                and len(KnowledgebaseService.query(name=req["name"], tenant_id=current_user.id, status=StatusEnum.VALID.value)) > 1:
            return get_data_error_result(
                retmsg="Duplicated knowledgebase name.")

        del req["kb_id"]
        if not KnowledgebaseService.update_by_id(kb.id, req):
            return get_data_error_result()

        e, kb = KnowledgebaseService.get_by_id(kb.id)
        if not e:
            return get_data_error_result(
                retmsg="Database error (Knowledgebase rename)!")

        return get_json_result(data=kb.to_json())
    except Exception as e:
        return server_error_response(e)


@manager.route('/detail', methods=['GET'])
@login_required
def detail():
    kb_id = request.args["kb_id"]
    try:
        kb = KnowledgebaseService.get_detail(kb_id)
        if not kb:
            return get_data_error_result(
                retmsg="Can't find this knowledgebase!")
        return get_json_result(data=kb)
    except Exception as e:
        return server_error_response(e)


@manager.route('/list', methods=['GET'])
@login_required
def list():
    page_number = request.args.get("page", 1)
    items_per_page = request.args.get("page_size", 15)
    orderby = request.args.get("orderby", "create_time")
    desc = request.args.get("desc", True)
    try:
        tenants = TenantService.get_joined_tenants_by_user_id(current_user.id)
        kbs = KnowledgebaseService.get_by_tenant_ids(
            [m["tenant_id"] for m in tenants], current_user.id, page_number, items_per_page, orderby, desc)
        return get_json_result(data=kbs)
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['post'])
@login_required
@validate_request("kb_id")
def rm():
    req = request.json
    try:
        kbs = KnowledgebaseService.query(
                created_by=current_user.id, id=req["kb_id"])
        if not kbs:
            return get_json_result(
                data=False, retmsg=f'Only owner of knowledgebase authorized for this operation.', retcode=RetCode.OPERATING_ERROR)

        for doc in DocumentService.query(kb_id=req["kb_id"]):
            ELASTICSEARCH.deleteByQuery(
                Q("match", doc_id=doc.id), idxnm=search.index_name(kbs[0].tenant_id))

            DocumentService.increment_chunk_num(
                doc.id, doc.kb_id, doc.token_num * -1, doc.chunk_num * -1, 0)
            if not DocumentService.delete(doc):
                return get_data_error_result(
                    retmsg="Database error (Document removal)!")

        if not KnowledgebaseService.update_by_id(
                req["kb_id"], {"status": StatusEnum.INVALID.value}):
            return get_data_error_result(
                retmsg="Database error (Knowledgebase removal)!")
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)

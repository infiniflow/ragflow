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
from flask import request

from api.db import StatusEnum, FileSource
from api.db.db_models import File
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import TenantService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import get_json_result, token_required, get_data_error_result


@manager.route('/save', methods=['POST'])
@token_required
def save(tenant_id):
    req = request.json
    e, t = TenantService.get_by_id(tenant_id)
    if "id" not in req:
        if "tenant_id" in req or "embd_id" in req:
            return get_data_error_result(
                retmsg="Tenant_id or embedding_model must not be provided")
        if "name" not in req:
            return get_data_error_result(
                retmsg="Name is not empty!")
        req['id'] = get_uuid()
        req["name"] = req["name"].strip()
        if req["name"] == "":
            return get_data_error_result(
                retmsg="Name is not empty string!")
        if KnowledgebaseService.query(name=req["name"], tenant_id=tenant_id, status=StatusEnum.VALID.value):
            return get_data_error_result(
                retmsg="Duplicated knowledgebase name in creating dataset.")
        req["tenant_id"] = tenant_id
        req['created_by'] = tenant_id
        req['embd_id'] = t.embd_id
        if not KnowledgebaseService.save(**req):
            return get_data_error_result(retmsg="Create dataset error.(Database error)")
        return get_json_result(data=req)
    else:
        if "tenant_id" in req:
            if req["tenant_id"] != tenant_id:
                return get_data_error_result(
                    retmsg="Can't change tenant_id.")

        if "embd_id" in req:
            if req["embd_id"] != t.embd_id:
                return get_data_error_result(
                    retmsg="Can't change embedding_model.")

        if not KnowledgebaseService.query(
                created_by=tenant_id, id=req["id"]):
            return get_json_result(
                data=False, retmsg='You do not own the dataset.',
                retcode=RetCode.OPERATING_ERROR)

        e, kb = KnowledgebaseService.get_by_id(req["id"])

        if "chunk_num" in req:
            if req["chunk_num"] != kb.chunk_num:
                return get_data_error_result(
                    retmsg="Can't change chunk_count.")

        if "doc_num" in req:
            if req['doc_num'] != kb.doc_num:
                return get_data_error_result(
                    retmsg="Can't change document_count.")

        if "parser_id" in req:
            if kb.chunk_num > 0 and req['parser_id'] != kb.parser_id:
                return get_data_error_result(
                    retmsg="if chunk count is not 0, parse method is not changable.")
        if "name" in req:
            if req["name"].lower() != kb.name.lower() \
                    and len(KnowledgebaseService.query(name=req["name"], tenant_id=tenant_id,
                                                       status=StatusEnum.VALID.value)) > 0:
                return get_data_error_result(
                    retmsg="Duplicated knowledgebase name in updating dataset.")

        del req["id"]
        if not KnowledgebaseService.update_by_id(kb.id, req):
            return get_data_error_result(retmsg="Update dataset error.(Database error)")
        return get_json_result(data=True)


@manager.route('/delete', methods=['DELETE'])
@token_required
def delete(tenant_id):
    req = request.args
    kbs = KnowledgebaseService.query(
        created_by=tenant_id, id=req["id"])
    if not kbs:
        return get_json_result(
            data=False, retmsg='You do not own the dataset',
            retcode=RetCode.OPERATING_ERROR)

    for doc in DocumentService.query(kb_id=req["id"]):
        if not DocumentService.remove_document(doc, kbs[0].tenant_id):
            return get_data_error_result(
                retmsg="Remove document error.(Database error)")
        f2d = File2DocumentService.get_by_document_id(doc.id)
        FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == f2d[0].file_id])
        File2DocumentService.delete_by_document_id(doc.id)

    if not KnowledgebaseService.delete_by_id(req["id"]):
        return get_data_error_result(
            retmsg="Delete dataset error.(Database error)")
    return get_json_result(data=True)


@manager.route('/list', methods=['GET'])
@token_required
def list_datasets(tenant_id):
    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 1024))
    orderby = request.args.get("orderby", "create_time")
    desc = bool(request.args.get("desc", True))
    tenants = TenantService.get_joined_tenants_by_user_id(tenant_id)
    kbs = KnowledgebaseService.get_by_tenant_ids(
        [m["tenant_id"] for m in tenants], tenant_id, page_number, items_per_page, orderby, desc)
    return get_json_result(data=kbs)


@manager.route('/detail', methods=['GET'])
@token_required
def detail(tenant_id):
    req = request.args
    if "id" in req:
        id = req["id"]
        kb = KnowledgebaseService.query(created_by=tenant_id, id=req["id"])
        if not kb:
            return get_json_result(
                data=False, retmsg='You do not own the dataset',
                retcode=RetCode.OPERATING_ERROR)
        if "name" in req:
            name = req["name"]
            if kb[0].name != name:
                return get_json_result(
                    data=False, retmsg='You do not own the dataset',
                    retcode=RetCode.OPERATING_ERROR)
        e, k = KnowledgebaseService.get_by_id(id)
        return get_json_result(data=k.to_dict())
    else:
        if "name" in req:
            name = req["name"]
            e, k = KnowledgebaseService.get_by_name(kb_name=name, tenant_id=tenant_id)
            if not e:
                return get_json_result(
                    data=False, retmsg='You do not own the dataset',
                    retcode=RetCode.OPERATING_ERROR)
            return get_json_result(data=k.to_dict())
        else:
            return get_data_error_result(
                retmsg="At least one of `id` or `name` must be provided.")

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
from api.utils.api_utils import get_result, token_required,get_error_data_result

@manager.route('/dataset', methods=['POST'])
@token_required
def create(tenant_id):
    req = request.json
    e, t = TenantService.get_by_id(tenant_id)
    if "tenant_id" in req or "embedding_model" in req:
        return get_error_data_result(
            retmsg="Tenant_id or embedding_model must not be provided")
    chunk_count=req.get("chunk_count")
    document_count=req.get("document_count")
    if chunk_count or document_count:
        return get_error_data_result(retmsg="chunk_count or document_count must be 0 or not be provided")
    if "name" not in req:
        return get_error_data_result(
            retmsg="Name is not empty!")
    req['id'] = get_uuid()
    req["name"] = req["name"].strip()
    if req["name"] == "":
        return get_error_data_result(
            retmsg="Name is not empty string!")
    if KnowledgebaseService.query(name=req["name"], tenant_id=tenant_id, status=StatusEnum.VALID.value):
        return get_error_data_result(
            retmsg="Duplicated knowledgebase name in creating dataset.")
    req["tenant_id"] = req['created_by'] = tenant_id
    req['embedding_model'] = t.embd_id
    key_mapping = {
        "chunk_num": "chunk_count",
        "doc_num": "document_count",
        "parser_id": "parse_method",
        "embd_id": "embedding_model"
    }
    mapped_keys = {new_key: req[old_key] for new_key, old_key in key_mapping.items() if old_key in req}
    req.update(mapped_keys)
    if not KnowledgebaseService.save(**req):
        return get_error_data_result(retmsg="Create dataset error.(Database error)")
    renamed_data = {}
    e, k = KnowledgebaseService.get_by_id(req["id"])
    for key, value in k.to_dict().items():
        new_key = key_mapping.get(key, key)
        renamed_data[new_key] = value
    return get_result(data=renamed_data)

@manager.route('/dataset', methods=['DELETE'])
@token_required
def delete(tenant_id):
    req = request.json
    ids = req.get("ids")
    if not ids:
        return get_error_data_result(
            retmsg="ids are required")
    for id in ids:
        kbs = KnowledgebaseService.query(id=id, tenant_id=tenant_id)
        if not kbs:
            return get_error_data_result(retmsg=f"You don't own the dataset {id}")
    for doc in DocumentService.query(kb_id=id):
        if not DocumentService.remove_document(doc, tenant_id):
            return get_error_data_result(
                retmsg="Remove document error.(Database error)")
        f2d = File2DocumentService.get_by_document_id(doc.id)
        FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == f2d[0].file_id])
        File2DocumentService.delete_by_document_id(doc.id)
    if not KnowledgebaseService.delete_by_id(id):
        return get_error_data_result(
            retmsg="Delete dataset error.(Database serror)")
    return get_result(retcode=RetCode.SUCCESS)

@manager.route('/dataset/<dataset_id>', methods=['PUT'])
@token_required
def update(tenant_id,dataset_id):
    if not KnowledgebaseService.query(id=dataset_id,tenant_id=tenant_id):
        return get_error_data_result(retmsg="You don't own the dataset")
    req = request.json
    e, t = TenantService.get_by_id(tenant_id)
    invalid_keys = {"id", "embd_id", "chunk_num", "doc_num", "parser_id"}
    if any(key in req for key in invalid_keys):
        return get_error_data_result(retmsg="The input parameters are invalid.")
    if "tenant_id" in req:
        if req["tenant_id"] != tenant_id:
            return get_error_data_result(
                retmsg="Can't change tenant_id.")
    if "embedding_model" in req:
        if req["embedding_model"] != t.embd_id:
            return get_error_data_result(
                retmsg="Can't change embedding_model.")
        req.pop("embedding_model")
    e, kb = KnowledgebaseService.get_by_id(dataset_id)
    if "chunk_count" in req:
        if req["chunk_count"] != kb.chunk_num:
            return get_error_data_result(
                retmsg="Can't change chunk_count.")
        req.pop("chunk_count")
    if "document_count" in req:
        if req['document_count'] != kb.doc_num:
            return get_error_data_result(
                retmsg="Can't change document_count.")
        req.pop("document_count")
    if "parse_method" in req:
        if kb.chunk_num != 0 and req['parse_method'] != kb.parser_id:
            return get_error_data_result(
                retmsg="If chunk count is not 0, parse method is not changable.")
        req['parser_id'] = req.pop('parse_method')
    if "name" in req:
        req["name"] = req["name"].strip()
        if req["name"].lower() != kb.name.lower() \
                and len(KnowledgebaseService.query(name=req["name"], tenant_id=tenant_id,
                                                   status=StatusEnum.VALID.value)) > 0:
            return get_error_data_result(
                retmsg="Duplicated knowledgebase name in updating dataset.")
    if not KnowledgebaseService.update_by_id(kb.id, req):
        return get_error_data_result(retmsg="Update dataset error.(Database error)")
    return get_result(retcode=RetCode.SUCCESS)

@manager.route('/dataset', methods=['GET'])
@token_required
def list(tenant_id):
    id = request.args.get("id")
    name = request.args.get("name")
    kbs = KnowledgebaseService.query(id=id,name=name,status=1)
    if not kbs:
        return get_error_data_result(retmsg="The dataset doesn't exist")
    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 1024))
    orderby = request.args.get("orderby", "create_time")
    if request.args.get("desc") == "False":
        desc = False
    else:
        desc = True
    tenants = TenantService.get_joined_tenants_by_user_id(tenant_id)
    kbs = KnowledgebaseService.get_list(
        [m["tenant_id"] for m in tenants], tenant_id, page_number, items_per_page, orderby, desc, id, name)
    renamed_list = []
    for kb in kbs:
        key_mapping = {
            "chunk_num": "chunk_count",
            "doc_num": "document_count",
            "parser_id": "parse_method",
            "embd_id": "embedding_model"
        }
        renamed_data = {}
        for key, value in kb.items():
            new_key = key_mapping.get(key, key)
            renamed_data[new_key] = value
        renamed_list.append(renamed_data)
    return get_result(data=renamed_list)

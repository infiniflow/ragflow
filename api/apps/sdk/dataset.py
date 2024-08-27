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

from api.db import StatusEnum
from api.db.db_models import APIToken
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import TenantService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import get_data_error_result
from api.utils.api_utils import get_json_result


@manager.route('/save', methods=['POST'])
def save():
    req = request.json
    token = request.headers.get('Authorization').split()[1]
    objs = APIToken.query(token=token)
    if not objs:
        return get_json_result(
            data=False, retmsg='Token is not valid!"', retcode=RetCode.AUTHENTICATION_ERROR)
    tenant_id = objs[0].tenant_id
    e, t = TenantService.get_by_id(tenant_id)
    if not e:
        return get_data_error_result(retmsg="Tenant not found.")
    if "id" not in req:
        req['id'] = get_uuid()
        req["name"] = req["name"].strip()
        if req["name"] == "":
            return get_data_error_result(
                retmsg="Name is not empty")
        if KnowledgebaseService.query(name=req["name"]):
            return get_data_error_result(
                retmsg="Duplicated knowledgebase name")
        req["tenant_id"] = tenant_id
        req['created_by'] = tenant_id
        req['embd_id'] = t.embd_id
        if not KnowledgebaseService.save(**req):
            return get_data_error_result(retmsg="Data saving error")
        req.pop('created_by')
        keys_to_rename = {'embd_id': "embedding_model", 'parser_id': 'parser_method',
                          'chunk_num': 'chunk_count', 'doc_num': 'document_count'}
        for old_key,new_key in keys_to_rename.items():
            if old_key in req:
                req[new_key]=req.pop(old_key)
        return get_json_result(data=req)
    else:
        if req["tenant_id"] != tenant_id or req["embd_id"] != t.embd_id:
            return get_data_error_result(
                retmsg="Can't change tenant_id or embedding_model")

        e, kb = KnowledgebaseService.get_by_id(req["id"])
        if not e:
            return get_data_error_result(
                retmsg="Can't find this knowledgebase!")

        if not KnowledgebaseService.query(
                created_by=tenant_id, id=req["id"]):
            return get_json_result(
                data=False, retmsg=f'Only owner of knowledgebase authorized for this operation.',
                retcode=RetCode.OPERATING_ERROR)

        if req["chunk_num"] != kb.chunk_num or req['doc_num'] != kb.doc_num:
            return get_data_error_result(
                retmsg="Can't change document_count or chunk_count ")

        if kb.chunk_num > 0 and req['parser_id'] != kb.parser_id:
            return get_data_error_result(
                retmsg="if chunk count is not 0, parser method is not changable. ")


        if req["name"].lower() != kb.name.lower() \
                and len(KnowledgebaseService.query(name=req["name"], tenant_id=req['tenant_id'],
                                                   status=StatusEnum.VALID.value)) > 0:
            return get_data_error_result(
                retmsg="Duplicated knowledgebase name.")

        del req["id"]
        req['created_by'] = tenant_id
        if not KnowledgebaseService.update_by_id(kb.id, req):
            return get_data_error_result(retmsg="Data update error ")
        return get_json_result(data=True)

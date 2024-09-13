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

from apiflask import Schema, fields

from api.db import StatusEnum, FileSource
from api.db.db_models import File
from api.db.services import duplicate_name
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import TenantService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import get_json_result, get_data_error_result


class QueryDatasetReq(Schema):
    page = fields.Integer(load_default=1)
    page_size = fields.Integer(load_default=150)
    orderby = fields.String(load_default='create_time')
    desc = fields.Boolean(load_default=True)

class SearchDatasetReq(Schema):
    name = fields.String(required=True)


class CreateDatasetReq(Schema):
    name = fields.String(required=True)


class UpdateDatasetReq(Schema):
    kb_id = fields.String(required=True)
    name = fields.String()
    description = fields.String()
    permission = fields.String()
    parser_id = fields.String()


def get_all_datasets(user_id, offset, count, orderby, desc):
    tenants = TenantService.get_joined_tenants_by_user_id(user_id)
    datasets = KnowledgebaseService.get_by_tenant_ids_by_offset(
        [m["tenant_id"] for m in tenants], user_id, int(offset), int(count), orderby, desc)
    return get_json_result(data=datasets)


def get_tenant_dataset_by_id(tenant_id, kb_id):
    kbs = KnowledgebaseService.query(tenant_id=tenant_id, id=kb_id)
    if not kbs:
        return get_data_error_result(retmsg="Can't find this knowledgebase!")
    return get_json_result(data=kbs[0].to_dict())


def get_dataset_by_id(tenant_id, kb_id):
    kbs = KnowledgebaseService.query(created_by=tenant_id, id=kb_id)
    if not kbs:
        return get_data_error_result(retmsg="Can't find this knowledgebase!")
    return get_json_result(data=kbs[0].to_dict())


def get_dataset_by_name(tenant_id, kb_name):
    e, kb = KnowledgebaseService.get_by_name(kb_name=kb_name, tenant_id=tenant_id)
    if not e:
        return get_json_result(
            data=False, retmsg='You do not own the dataset.',
            retcode=RetCode.OPERATING_ERROR)
    return get_json_result(data=kb.to_dict())


def create_dataset(tenant_id, data):
    kb_name = data["name"].strip()
    kb_name = duplicate_name(
        KnowledgebaseService.query,
        name=kb_name,
        tenant_id=tenant_id,
        status=StatusEnum.VALID.value
    )
    e, t = TenantService.get_by_id(tenant_id)
    if not e:
        return get_data_error_result(retmsg="Tenant not found.")
    kb = {
        "id": get_uuid(),
        "name": kb_name,
        "tenant_id": tenant_id,
        "created_by": tenant_id,
        "embd_id": t.embd_id,
    }
    if not KnowledgebaseService.save(**kb):
        return get_data_error_result()
    return get_json_result(data={"kb_id": kb["id"]})


def update_dataset(tenant_id, data):
    kb_name = data["name"].strip()
    kb_id = data["kb_id"].strip()
    if not KnowledgebaseService.query(
            created_by=tenant_id, id=kb_id):
        return get_json_result(
            data=False, retmsg=f'Only owner of knowledgebase authorized for this operation.',
            retcode=RetCode.OPERATING_ERROR)

    e, kb = KnowledgebaseService.get_by_id(kb_id)
    if not e:
        return get_data_error_result(
            retmsg="Can't find this knowledgebase!")

    if kb_name.lower() != kb.name.lower() and len(
            KnowledgebaseService.query(name=kb_name, tenant_id=tenant_id, status=StatusEnum.VALID.value)) > 1:
        return get_data_error_result(
            retmsg="Duplicated knowledgebase name.")

    del data["kb_id"]
    if not KnowledgebaseService.update_by_id(kb.id, data):
        return get_data_error_result()

    e, kb = KnowledgebaseService.get_by_id(kb.id)
    if not e:
        return get_data_error_result(
            retmsg="Database error (Knowledgebase rename)!")

    return get_json_result(data=kb.to_json())


def delete_dataset(tenant_id, kb_id):
    kbs = KnowledgebaseService.query(created_by=tenant_id, id=kb_id)
    if not kbs:
        return get_json_result(
            data=False, retmsg=f'Only owner of knowledgebase authorized for this operation.',
            retcode=RetCode.OPERATING_ERROR)

    for doc in DocumentService.query(kb_id=kb_id):
        if not DocumentService.remove_document(doc, kbs[0].tenant_id):
            return get_data_error_result(
                retmsg="Database error (Document removal)!")
        f2d = File2DocumentService.get_by_document_id(doc.id)
        FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == f2d[0].file_id])
        File2DocumentService.delete_by_document_id(doc.id)

    if not KnowledgebaseService.delete_by_id(kb_id):
        return get_data_error_result(
            retmsg="Database error (Knowledgebase removal)!")
    return get_json_result(data=True)

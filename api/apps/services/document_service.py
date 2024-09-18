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
import re

from apiflask import Schema, fields, validators
from elasticsearch_dsl import Q

from api.db import FileType, TaskStatus, ParserType
from api.db.db_models import Task
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.task_service import TaskService, queue_tasks
from api.db.services.user_service import UserTenantService
from api.settings import RetCode
from api.utils.api_utils import get_data_error_result
from api.utils.api_utils import get_json_result
from rag.nlp import search
from rag.utils.es_conn import ELASTICSEARCH


class QueryDocumentsReq(Schema):
    kb_id = fields.String(required=True, error='Invalid kb_id parameter!')
    keywords = fields.String(load_default='')
    page = fields.Integer(load_default=1)
    page_size = fields.Integer(load_default=150)
    orderby = fields.String(load_default='create_time')
    desc = fields.Boolean(load_default=True)


class ChangeDocumentParserReq(Schema):
    doc_id = fields.String(required=True)
    parser_id = fields.String(
        required=True, validate=validators.OneOf([parser_type.value for parser_type in ParserType])
    )
    parser_config = fields.Dict()


class RunParsingReq(Schema):
    doc_ids = fields.List(fields.String(), required=True)
    run = fields.Integer(load_default=1)


class UploadDocumentsReq(Schema):
    kb_id = fields.String(required=True)
    file = fields.List(fields.File(), required=True)


def get_all_documents(query_data, tenant_id):
    kb_id = query_data["kb_id"]
    tenants = UserTenantService.query(user_id=tenant_id)
    for tenant in tenants:
        if KnowledgebaseService.query(
                tenant_id=tenant.tenant_id, id=kb_id):
            break
    else:
        return get_json_result(
            data=False, retmsg=f'Only owner of knowledgebase authorized for this operation.',
            retcode=RetCode.OPERATING_ERROR)
    keywords = query_data["keywords"]

    page_number = query_data["page"]
    items_per_page = query_data["page_size"]
    orderby = query_data["orderby"]
    desc = query_data["desc"]
    docs, tol = DocumentService.get_by_kb_id(
        kb_id, page_number, items_per_page, orderby, desc, keywords)
    return get_json_result(data={"total": tol, "docs": docs})


def upload_documents_2_dataset(form_and_files_data, tenant_id):
    file_objs = form_and_files_data['file']
    dataset_id = form_and_files_data['kb_id']
    for file_obj in file_objs:
        if file_obj.filename == '':
            return get_json_result(
                data=False, retmsg='No file selected!', retcode=RetCode.ARGUMENT_ERROR)
    e, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not e:
        raise LookupError(f"Can't find the knowledgebase with ID {dataset_id}!")
    err, _ = FileService.upload_document(kb, file_objs, tenant_id)
    if err:
        return get_json_result(
            data=False, retmsg="\n".join(err), retcode=RetCode.SERVER_ERROR)
    return get_json_result(data=True)


def change_document_parser(json_data):
    e, doc = DocumentService.get_by_id(json_data["doc_id"])
    if not e:
        return get_data_error_result(retmsg="Document not found!")
    if doc.parser_id.lower() == json_data["parser_id"].lower():
        if "parser_config" in json_data:
            if json_data["parser_config"] == doc.parser_config:
                return get_json_result(data=True)
        else:
            return get_json_result(data=True)

    if doc.type == FileType.VISUAL or re.search(
            r"\.(ppt|pptx|pages)$", doc.name):
        return get_data_error_result(retmsg="Not supported yet!")

    e = DocumentService.update_by_id(doc.id,
                                     {"parser_id": json_data["parser_id"], "progress": 0, "progress_msg": "",
                                      "run": TaskStatus.UNSTART.value})
    if not e:
        return get_data_error_result(retmsg="Document not found!")
    if "parser_config" in json_data:
        DocumentService.update_parser_config(doc.id, json_data["parser_config"])
    if doc.token_num > 0:
        e = DocumentService.increment_chunk_num(doc.id, doc.kb_id, doc.token_num * -1, doc.chunk_num * -1,
                                                doc.process_duation * -1)
        if not e:
            return get_data_error_result(retmsg="Document not found!")
        tenant_id = DocumentService.get_tenant_id(json_data["doc_id"])
        if not tenant_id:
            return get_data_error_result(retmsg="Tenant not found!")
        ELASTICSEARCH.deleteByQuery(
            Q("match", doc_id=doc.id), idxnm=search.index_name(tenant_id))

    return get_json_result(data=True)


def run_parsing(json_data):
    for id in json_data["doc_ids"]:
        run = str(json_data["run"])
        info = {"run": run, "progress": 0}
        if run == TaskStatus.RUNNING.value:
            info["progress_msg"] = ""
            info["chunk_num"] = 0
            info["token_num"] = 0
        DocumentService.update_by_id(id, info)
        tenant_id = DocumentService.get_tenant_id(id)
        if not tenant_id:
            return get_data_error_result(retmsg="Tenant not found!")
        ELASTICSEARCH.deleteByQuery(
            Q("match", doc_id=id), idxnm=search.index_name(tenant_id))

        if run == TaskStatus.RUNNING.value:
            TaskService.filter_delete([Task.doc_id == id])
            e, doc = DocumentService.get_by_id(id)
            doc = doc.to_dict()
            doc["tenant_id"] = tenant_id
            bucket, name = File2DocumentService.get_minio_address(doc_id=doc["id"])
            queue_tasks(doc, bucket, name)

    return get_json_result(data=True)

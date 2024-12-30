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
from flask_login import login_required, current_user

from api.db.services import duplicate_name
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.user_service import TenantService, UserTenantService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request, not_allowed_parameters
from api.utils import get_uuid
from api.db import StatusEnum, FileSource
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.db_models import File
from api.utils.api_utils import get_json_result
from api import settings
from rag.nlp import search
from api.constants import DATASET_NAME_LIMIT


@manager.route('/create', methods=['post'])  # noqa: F821
@login_required
@validate_request("name")
def create():
    req = request.json
    dataset_name = req["name"]
    if not isinstance(dataset_name, str):
        return get_data_error_result(message="Dataset name must be string.")
    if dataset_name == "":
        return get_data_error_result(message="Dataset name can't be empty.")
    if len(dataset_name) >= DATASET_NAME_LIMIT:
        return get_data_error_result(
            message=f"Dataset name length is {len(dataset_name)} which is large than {DATASET_NAME_LIMIT}")

    dataset_name = dataset_name.strip()
    dataset_name = duplicate_name(
        KnowledgebaseService.query,
        name=dataset_name,
        tenant_id=current_user.id,
        status=StatusEnum.VALID.value)
    try:
        req["id"] = get_uuid()
        req["tenant_id"] = current_user.id
        req["created_by"] = current_user.id
        e, t = TenantService.get_by_id(current_user.id)
        if not e:
            return get_data_error_result(message="Tenant not found.")
        req["embd_id"] = t.embd_id
        if not KnowledgebaseService.save(**req):
            return get_data_error_result()
        return get_json_result(data={"kb_id": req["id"]})
    except Exception as e:
        return server_error_response(e)


@manager.route('/update', methods=['post'])  # noqa: F821
@login_required
@validate_request("kb_id", "name", "description", "permission", "parser_id")
@not_allowed_parameters("id", "tenant_id", "created_by", "create_time", "update_time", "create_date", "update_date", "created_by")
def update():
    req = request.json
    req["name"] = req["name"].strip()
    if not KnowledgebaseService.accessible4deletion(req["kb_id"], current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    try:
        if not KnowledgebaseService.query(
                created_by=current_user.id, id=req["kb_id"]):
            return get_json_result(
                data=False, message='Only owner of knowledgebase authorized for this operation.',
                code=settings.RetCode.OPERATING_ERROR)

        e, kb = KnowledgebaseService.get_by_id(req["kb_id"])
        if not e:
            return get_data_error_result(
                message="Can't find this knowledgebase!")

        if req["name"].lower() != kb.name.lower() \
                and len(
            KnowledgebaseService.query(name=req["name"], tenant_id=current_user.id, status=StatusEnum.VALID.value)) > 1:
            return get_data_error_result(
                message="Duplicated knowledgebase name.")

        del req["kb_id"]
        if not KnowledgebaseService.update_by_id(kb.id, req):
            return get_data_error_result()

        if kb.pagerank != req.get("pagerank", 0):
            if req.get("pagerank", 0) > 0:
                settings.docStoreConn.update({"kb_id": kb.id}, {"pagerank_fea": req["pagerank"]},
                                         search.index_name(kb.tenant_id), kb.id)
            else:
                # Elasticsearch requires pagerank_fea be non-zero!
                settings.docStoreConn.update({"exist": "pagerank_fea"}, {"remove": "pagerank_fea"},
                                         search.index_name(kb.tenant_id), kb.id)

        e, kb = KnowledgebaseService.get_by_id(kb.id)
        if not e:
            return get_data_error_result(
                message="Database error (Knowledgebase rename)!")

        return get_json_result(data=kb.to_json())
    except Exception as e:
        return server_error_response(e)


@manager.route('/detail', methods=['GET'])  # noqa: F821
@login_required
def detail():
    kb_id = request.args["kb_id"]
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        for tenant in tenants:
            if KnowledgebaseService.query(
                    tenant_id=tenant.tenant_id, id=kb_id):
                break
        else:
            return get_json_result(
                data=False, message='Only owner of knowledgebase authorized for this operation.',
                code=settings.RetCode.OPERATING_ERROR)
        kb = KnowledgebaseService.get_detail(kb_id)
        if not kb:
            return get_data_error_result(
                message="Can't find this knowledgebase!")
        return get_json_result(data=kb)
    except Exception as e:
        return server_error_response(e)


@manager.route('/list', methods=['GET'])  # noqa: F821
@login_required
def list_kbs():
    keywords = request.args.get("keywords", "")
    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 150))
    orderby = request.args.get("orderby", "create_time")
    desc = request.args.get("desc", True)
    try:
        tenants = TenantService.get_joined_tenants_by_user_id(current_user.id)
        kbs, total = KnowledgebaseService.get_by_tenant_ids(
            [m["tenant_id"] for m in tenants], current_user.id, page_number, items_per_page, orderby, desc, keywords)
        return get_json_result(data={"kbs": kbs, "total": total})
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['post'])  # noqa: F821
@login_required
@validate_request("kb_id")
def rm():
    req = request.json
    if not KnowledgebaseService.accessible4deletion(req["kb_id"], current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    try:
        kbs = KnowledgebaseService.query(
            created_by=current_user.id, id=req["kb_id"])
        if not kbs:
            return get_json_result(
                data=False, message='Only owner of knowledgebase authorized for this operation.',
                code=settings.RetCode.OPERATING_ERROR)

        for doc in DocumentService.query(kb_id=req["kb_id"]):
            if not DocumentService.remove_document(doc, kbs[0].tenant_id):
                return get_data_error_result(
                    message="Database error (Document removal)!")
            f2d = File2DocumentService.get_by_document_id(doc.id)
            if f2d:
                FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == f2d[0].file_id])
            File2DocumentService.delete_by_document_id(doc.id)
        FileService.filter_delete(
            [File.source_type == FileSource.KNOWLEDGEBASE, File.type == "folder", File.name == kbs[0].name])
        if not KnowledgebaseService.delete_by_id(req["kb_id"]):
            return get_data_error_result(
                message="Database error (Knowledgebase removal)!")
        for kb in kbs:
            settings.docStoreConn.delete({"kb_id": kb.id}, search.index_name(kb.tenant_id), kb.id)
            settings.docStoreConn.deleteIdx(search.index_name(kb.tenant_id), kb.id)
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)

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


import json
import os
import re
from datetime import datetime, timedelta
from flask import request, Response
from flask_login import login_required, current_user

from api.db import FileType, ParserType, FileSource, StatusEnum
from api.db.db_models import APIToken, API4Conversation, Task, File
from api.db.services import duplicate_name
from api.db.services.api_service import APITokenService, API4ConversationService
from api.db.services.dialog_service import DialogService, chat
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.task_service import queue_tasks, TaskService
from api.db.services.user_service import UserTenantService, TenantService
from api.settings import RetCode, retrievaler
from api.utils import get_uuid, current_timestamp, datetime_format
# from api.utils.api_utils import server_error_response, get_data_error_result, get_json_result, validate_request
from itsdangerous import URLSafeTimedSerializer

from api.utils.file_utils import filename_type, thumbnail
from rag.utils.minio_conn import MINIO

# import library
from api.utils.api_utils import construct_json_result, construct_result, construct_error_response, validate_request
from api.contants import NAME_LENGTH_LIMIT

# ------------------------------ create a dataset ---------------------------------------

@manager.route('/', methods=['POST'])
@login_required  # use login
@validate_request("name")  # check name key
def create_dataset():
    # Check if Authorization header is present
    authorization_token = request.headers.get('Authorization')
    if not authorization_token:
        return construct_json_result(code=RetCode.AUTHENTICATION_ERROR, message="Authorization header is missing.")

    # TODO: Login or API key
    # objs = APIToken.query(token=authorization_token)
    #
    # # Authorization error
    # if not objs:
    #     return construct_json_result(code=RetCode.AUTHENTICATION_ERROR, message="Token is invalid.")
    #
    # tenant_id = objs[0].tenant_id

    tenant_id = current_user.id
    request_body = request.json

    # In case that there's no name
    if "name" not in request_body:
        return construct_json_result(code=RetCode.DATA_ERROR, message="Expected 'name' field in request body")

    dataset_name = request_body["name"]

    # empty dataset_name
    if not dataset_name:
        return construct_json_result(code=RetCode.DATA_ERROR, message="Empty dataset name")

    # In case that there's space in the head or the tail
    dataset_name = dataset_name.strip()

    # In case that the length of the name exceeds the limit
    dataset_name_length = len(dataset_name)
    if dataset_name_length > NAME_LENGTH_LIMIT:
        return construct_json_result(code=RetCode.DATA_ERROR,
                                     message=f"Dataset name: {dataset_name} with length {dataset_name_length} exceeds {NAME_LENGTH_LIMIT}!")

    # In case that there are other fields in the data-binary
    if len(request_body.keys()) > 1:
        name_list = []
        for key_name in request_body.keys():
            if key_name != 'name':
                name_list.append(key_name)
        return construct_json_result(code=RetCode.DATA_ERROR,
                                     message=f"fields: {name_list}, are not allowed in request body.")

    # If there is a duplicate name, it will modify it to make it unique
    request_body["name"] = duplicate_name(
        KnowledgebaseService.query,
        name=dataset_name,
        tenant_id=tenant_id,
        status=StatusEnum.VALID.value)
    try:
        request_body["id"] = get_uuid()
        request_body["tenant_id"] = tenant_id
        request_body["created_by"] = tenant_id
        e, t = TenantService.get_by_id(tenant_id)
        if not e:
            return construct_result(code=RetCode.AUTHENTICATION_ERROR, message="Tenant not found.")
        request_body["embd_id"] = t.embd_id
        if not KnowledgebaseService.save(**request_body):
            # failed to create new dataset
            return construct_result()
        return construct_json_result(data={"dataset_name": request_body["name"], "id": request_body["id"]})
    except Exception as e:
        return construct_error_response(e)

# -----------------------------list datasets-------------------------------------------------------

@manager.route('/', methods=['GET'])
@login_required
def list_datasets():
    offset = request.args.get("offset", 0)
    count = request.args.get("count", -1)
    orderby = request.args.get("orderby", "create_time")
    desc = request.args.get("desc", True)
    try:
        tenants = TenantService.get_joined_tenants_by_user_id(current_user.id)
        kbs = KnowledgebaseService.get_by_tenant_ids(
            [m["tenant_id"] for m in tenants], current_user.id, int(offset), int(count), orderby, desc)
        return construct_json_result(data=kbs, code=RetCode.DATA_ERROR, message=f"attempt to list datasets")
    except Exception as e:
        return construct_error_response(e)

# ---------------------------------delete a dataset ----------------------------

@manager.route('/<dataset_id>', methods=['DELETE'])
@login_required
def remove_dataset(dataset_id):
    req = request.json
    try:
        datasets = KnowledgebaseService.query(
            created_by=current_user.id, id=dataset_id)
        if not datasets:
            return construct_json_result(
                data=False, message=f'Only owner of knowledgebase authorized for this operation.',
                code=RetCode.OPERATING_ERROR)

        for doc in DocumentService.query(kb_id=dataset_id):
            if not DocumentService.remove_document(doc, datasets[0].tenant_id):
                return construct_json_result(
                    message="Database error (Document removal)!")
            f2d = File2DocumentService.get_by_document_id(doc.id)
            FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == f2d[0].file_id])
            File2DocumentService.delete_by_document_id(doc.id)

        if not KnowledgebaseService.delete_by_id(dataset_id):
            return construct_json_result(
                message="Database error (Knowledgebase removal)!")
        return construct_json_result(code=RetCode.SUCCESS, message=f"Remove dataset: {dataset_id} successfully")
    except Exception as e:
        return construct_error_response(e)

# ------------------------------ get details of a dataset ----------------------------------------

@manager.route('/<dataset_id>', methods=['GET'])
@login_required
def get_dataset(dataset_id):
    try:
        dataset = KnowledgebaseService.get_detail(dataset_id)
        if not dataset:
            return construct_json_result(message="Can't find this knowledgebase!")
        return construct_json_result(data=dataset, code=RetCode.DATA_ERROR,
                                     message=f"attempt to get detail of dataset: {dataset_id}")
    except Exception as e:
        return construct_json_result(e)

# ------------------------------ update a dataset --------------------------------------------

@manager.route('/<dataset_id>', methods=['PUT'])
@login_required
@validate_request("name", "dataset_id", "description", "permission", "parser_id", "language",
                  "embd_id")
def update_dataset(dataset_id):
    req = request.json
    req["name"] = req["name"].strip()
    try:
        # check whether the user is authorized to update
        if not KnowledgebaseService.query(created_by=current_user.id, id=dataset_id):
            return construct_json_result(
                data=False, message=f'Only owner of knowledgebase authorized for this operation.',
                code=RetCode.OPERATING_ERROR)

        e, dataset = KnowledgebaseService.get_by_id(dataset_id)
        # check whether there is this dataset
        if not e:
            return construct_json_result(message="Can't find this dataset!")

        if req["name"].lower() != dataset.name.lower() \
                and len(KnowledgebaseService.query(name=req["name"], tenant_id=current_user.id,
                                                   status=StatusEnum.VALID.value)) > 1:
            return construct_json_result(message="Duplicated dataset name.")

        del req["dataset_id"]
        if not KnowledgebaseService.update_by_id(dataset.id, req):
            return construct_json_result()

        e, dataset = KnowledgebaseService.get_by_id(dataset.id)
        if not e:
            return construct_json_result(message="Database error (Dataset rename)!")

        return construct_json_result(data=dataset.to_json(),
                                     message=f"Update dataset: {dataset_id} successfully!")
    except Exception as e:
        return construct_error_response(e)








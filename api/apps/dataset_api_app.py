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
from api.utils.api_utils import server_error_response, get_data_error_result, get_json_result, validate_request
from itsdangerous import URLSafeTimedSerializer

from api.utils.file_utils import filename_type, thumbnail
from rag.utils.minio_conn import MINIO

LIMIT_LENGTH_OF_NAME = 2 ** 10


# helper method to check whether it contains space
def contains_space(s):
    return any(char.isspace() for char in s)


# ------------------------------ create a dataset ---------------------------------------
@manager.route('/dataset', methods=['POST'])
@validate_request("name")  # check name key
def create():
    # Check if Authorization header is present
    if 'Authorization' not in request.headers:
        return get_json_result(data=False, message="Authorization header is missing!",
                               code=RetCode.AUTHENTICATION_ERROR)

    # Extract token from Authorization header
    try:
        token = request.headers.get('Authorization').split()[1]
    except IndexError:
        return get_json_result(data=False, message="Token is missing!", code=RetCode.AUTHENTICATION_ERROR)

    objs = APIToken.query(token=token)

    # Authorization error
    if not objs:
        return get_json_result(
            data=False, message='Token is invalid!"', code=RetCode.AUTHENTICATION_ERROR)

    tenant_id = objs[0].tenant_id
    request_body = request.json

    request_body_name = request_body["name"]

    # In case that there's no name
    if not request_body_name:
        return get_data_error_result(message="Name cannot be empty!")

    # In case that there's space in the end or the start
    request_body_name = request_body_name.strip()

    # In case that the length of the name exceeds the limit
    if len(request_body_name) > LIMIT_LENGTH_OF_NAME:
        return get_data_error_result(message=f"The name of the dataset exceeds {LIMIT_LENGTH_OF_NAME}!")

    # In case that there is space in the middle of the name
    if contains_space(request_body_name):
        return get_data_error_result(message="There is space in the middle of this dataset's name. Please remove it.")

    # In case that there are other fields in the data-binary
    if len(request_body.keys()) > 1:
        return get_data_error_result(
            message="There is other fields in addition to the 'name' field in the data-binary. Please remove it.")

    # If there is a duplicate name, it will modify it to make it unique
    request_body["name"] = duplicate_name(
        KnowledgebaseService.query,
        name=request_body_name,
        tenant_id=tenant_id,
        status=StatusEnum.VALID.value)
    try:
        request_body["id"] = get_uuid()
        request_body["tenant_id"] = tenant_id
        request_body["created_by"] = tenant_id
        e, t = TenantService.get_by_id(tenant_id)
        if not e:
            return get_data_error_result(message="Tenant not found.")
        request_body["embd_id"] = t.embd_id
        if not KnowledgebaseService.save(**request_body):
            return get_data_error_result()
        return get_json_result(data={"dataset_id": request_body["id"]})
    except Exception as e:
        return server_error_response(e)


# -------------------------- show details of the specific dataset  -----------------------------------
# @manager.route('/dataset/<kb_id>', methods=['GET'])
# def detail():
#     kb_id = request.args["kb_id"]
#     try:
#         kb = KnowledgebaseService.get_detail(kb_id)
#         if not kb:
#             return get_data_error_result(
#                 retmsg="Can't find this knowledgebase!")
#         return get_json_result(data=kb)
#     except Exception as e:
#         return server_error_response(e)

# -------------------------------------- update a dataset -------------------------------------------
# @manager.route('/dataset/<kb_id>', methods=['PUT'])
# @validate_request("name", "description", "permission", "parser_id")
# def update(kb_id):
#     req = request.json
#     req["name"] = req["name"].strip()
#     try:
#         if not KnowledgebaseService.query(
#                 created_by=current_user.id, id=req["kb_id"]):
#             return get_json_result(
#                 data=False, retmsg=f'Only owner of knowledgebase authorized for this operation.', retcode=RetCode.OPERATING_ERROR)
#
#         e, kb = KnowledgebaseService.get_by_id(req["kb_id"])
#         if not e:
#             return get_data_error_result(
#                 retmsg="Can't find this knowledgebase!")
#
#         if req["name"].lower() != kb.name.lower() \
#                 and len(KnowledgebaseService.query(name=req["name"], tenant_id=current_user.id, status=StatusEnum.VALID.value)) > 1:
#             return get_data_error_result(
#                 retmsg="Duplicated knowledgebase name.")
#
#         del req["kb_id"]
#         if not KnowledgebaseService.update_by_id(kb.id, req):
#             return get_data_error_result()
#
#         e, kb = KnowledgebaseService.get_by_id(kb.id)
#         if not e:
#             return get_data_error_result(
#                 retmsg="Database error (Knowledgebase rename)!")
#
#         return get_json_result(data=kb.to_json())
#     except Exception as e:
#         return server_error_response(e)
#
# # ------------------- list datasets ----------------------------
# @manager.route('/dataset', methods=['GET'])
# def list_kbs():
#     page_number = request.args.get("page", 1)
#     items_per_page = request.args.get("page_size", 150)
#     orderby = request.args.get("orderby", "create_time")
#     desc = request.args.get("desc", True)
#     try:
#         tenants = TenantService.get_joined_tenants_by_user_id(current_user.id)
#         kbs = KnowledgebaseService.get_by_tenant_ids(
#             [m["tenant_id"] for m in tenants], current_user.id, page_number, items_per_page, orderby, desc)
#         return get_json_result(data=kbs)
#     except Exception as e:
#         return server_error_response(e)
#
# # --------------------- remove a dataset ----------------------
# @manager.route('/dataset/<kb_id>', methods=['DELETE'])
# @validate_request("kb_id")
# def rm():
#     req = request.json
#     try:
#         kbs = KnowledgebaseService.query(
#                 created_by=current_user.id, id=req["kb_id"])
#         if not kbs:
#             return get_json_result(
#                 data=False, retmsg=f'Only owner of knowledgebase authorized for this operation.', retcode=RetCode.OPERATING_ERROR)
#
#         for doc in DocumentService.query(kb_id=req["kb_id"]):
#             if not DocumentService.remove_document(doc, kbs[0].tenant_id):
#                 return get_data_error_result(
#                     retmsg="Database error (Document removal)!")
#             f2d = File2DocumentService.get_by_document_id(doc.id)
#             FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == f2d[0].file_id])
#             File2DocumentService.delete_by_document_id(doc.id)
#
#         if not KnowledgebaseService.delete_by_id(req["kb_id"]):
#             return get_data_error_result(
#                 retmsg="Database error (Knowledgebase removal)!")
#         return get_json_result(data=True)
#     except Exception as e:
#         return server_error_response(e)
#

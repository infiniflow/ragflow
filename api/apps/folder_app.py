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
#  limitations under the License
#
import pathlib
import re

import flask
from elasticsearch_dsl import Q
from flask import request
from flask_login import login_required, current_user
from rag.nlp import search
from rag.utils import ELASTICSEARCH
from api.db.services import duplicate_name
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request
from api.utils import get_uuid
from api.db import FileType, TaskStatus, ParserType
from api.db.services.document_service import DocumentService
from api.db.services.folder_service import FolderService
from api.settings import RetCode
from api.utils.api_utils import get_json_result
from rag.utils.minio_conn import MINIO
from api.utils.file_utils import filename_type, thumbnail


@manager.route('/create', methods=['POST'])
@login_required
@validate_request("name", "pf_id")
def create():
    req = request.json
    pf_id = req["pf_id"]
    if not pf_id:
        return get_json_result(
            data=False, retmsg='Lack of "Folder ID"', retcode=RetCode.ARGUMENT_ERROR)

    try:
        e, doc = DocumentService.get_by_id(pf_id)
        if not e:
            return get_data_error_result(
                retmsg="Can't find this folder!")

        if FolderService.query(name=req["name"], pf_id=pf_id):
            return get_data_error_result(
                retmsg="Duplicated folder name in the same knowledgebase.")

        file = FolderService.insert({
            "id": get_uuid(),  # uuid
            "pf_id": pf_id,  # parent folder id
            "sf_id": "",  # sub folder id
            "created_by": current_user.id,
            "name": req["name"],
        })
        return get_json_result(data=file.to_json())
    except Exception as e:
        return server_error_response(e)


@manager.route('/list', methods=['GET'])
@login_required
def list():
    pf_id = request.args.get("pf_id")
    if not pf_id:
        return get_json_result(
            data=False, retmsg='Lack of "Parent Folder ID"', retcode=RetCode.ARGUMENT_ERROR)
    keywords = request.args.get("keywords", "")

    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 15))
    orderby = request.args.get("orderby", "create_time")
    desc = request.args.get("desc", True)
    try:
        files, tol = FolderService.get_by_pf_id(
            pf_id, page_number, items_per_page, orderby, desc, keywords)
        return get_json_result(data={"total": tol, "docs": files})
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['POST'])
@login_required
@validate_request("folder_id")
def rm():
    req = request.json
    try:
        e, folder = FolderService.get_by_id(req["folder_id"])
        if not e:
            return get_data_error_result(retmsg="Folder not found!")

        if not FolderService.delete(folder):
            return get_data_error_result(
                retmsg="Database error (Folder removal)!")

        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/rename', methods=['POST'])
@login_required
@validate_request("folder_id", "name")
def rename():
    req = request.json
    try:
        e, doc = FolderService.get_by_id(req["folder_id"])
        if not e:
            return get_data_error_result(retmsg="Folder not found!")
        if pathlib.Path(req["name"].lower()).suffix != pathlib.Path(
                doc.name.lower()).suffix:
            return get_json_result(
                data=False,
                retmsg="The extension of file can't be changed",
                retcode=RetCode.ARGUMENT_ERROR)
        if FolderService.query(name=req["name"], pf_id=doc.pf_id):
            return get_data_error_result(
                retmsg="Duplicated file name in the same folder.")

        if not FolderService.update_by_id(
                req["folder_id"], {"name": req["name"]}):
            return get_data_error_result(
                retmsg="Database error (Folder rename)!")

        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)

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

import os
import pathlib
import re

import flask
from elasticsearch_dsl import Q
from flask import request
from flask_login import login_required, current_user

from api.db.db_models import Task, File
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.task_service import TaskService, queue_tasks
from rag.nlp import search
from rag.utils.es_conn import ELASTICSEARCH
from api.db.services import duplicate_name
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request, construct_error_response, construct_json_result
from api.utils import get_uuid
from api.db import FileType, TaskStatus, ParserType, FileSource
from api.db.services.document_service import DocumentService
from api.settings import RetCode
from api.utils.api_utils import get_json_result
from rag.utils.minio_conn import MINIO
from api.utils.file_utils import filename_type, thumbnail
from api.utils.web_utils import html2pdf, is_valid_url
from api.utils.web_utils import html2pdf, is_valid_url

# ----------------------------upload a local file-----------------------------------------------------
@manager.route('/<dataset_id>', methods=['POST'])
@login_required
def upload(dataset_id):
    # the dataset where the user would like to upload files
    if not dataset_id:
        return construct_json_result(
            message='Lack of "dataset ID"', code=RetCode.ARGUMENT_ERROR)

    if 'file0' not in request.files:
        return construct_json_result(
            message='No file part!', code=RetCode.ARGUMENT_ERROR)

    file_objs = request.files.getlist('file')

    for file_obj in file_objs:
        if file_obj.filename == '':
            return construct_json_result(
                message='No file selected!', code=RetCode.ARGUMENT_ERROR)

    exist, dataset = KnowledgebaseService.get_by_id(dataset_id)
    if not exist:
        return construct_json_result(message="Can't find this dataset", code=RetCode.DATA_ERROR)

    root_folder = FileService.get_root_folder(current_user.id)
    pf_id = root_folder["id"]
    FileService.init_knowledgebase_docs(pf_id, current_user.id)
    kb_root_folder = FileService.get_kb_folder(current_user.id)
    kb_folder = FileService.new_a_file_from_kb(dataset.tenant_id, dataset.name, kb_root_folder["id"])

    err = []
    for file in file_objs:
        try:
            MAX_FILE_NUM_PER_USER = int(os.environ.get('MAX_FILE_NUM_PER_USER', 0))
            if MAX_FILE_NUM_PER_USER > 0 and DocumentService.get_doc_count(kb.tenant_id) >= MAX_FILE_NUM_PER_USER:
                raise RuntimeError("Exceed the maximum file number of a free user!")

            filename = duplicate_name(
                DocumentService.query,
                name=file.filename,
                kb_id=dataset.id)
            filetype = filename_type(filename)
            if filetype == FileType.OTHER.value:
                raise RuntimeError("This type of file has not been supported yet!")

            location = filename
            while MINIO.obj_exist(dataset_id, location):
                location += "_"
            blob = file.read()
            MINIO.put(dataset_id, location, blob)
            doc = {
                "id": get_uuid(),
                "kb_id": dataset.id,
                "parser_id": dataset.parser_id,
                "parser_config": dataset.parser_config,
                "created_by": current_user.id,
                "type": filetype,
                "name": filename,
                "location": location,
                "size": len(blob),
                "thumbnail": thumbnail(filename, blob)
            }
            if doc["type"] == FileType.VISUAL:
                doc["parser_id"] = ParserType.PICTURE.value
            if re.search(r"\.(ppt|pptx|pages)$", filename):
                doc["parser_id"] = ParserType.PRESENTATION.value
            DocumentService.insert(doc)

            FileService.add_file_from_kb(doc, kb_folder["id"], dataset.tenant_id)
        except Exception as e:
            err.append(file.filename + ": " + str(e))

    if err:
        return construct_json_result(
            message="\n".join(err), code=RetCode.SERVER_ERROR)
    return construct_json_result(data=True, code=RetCode.SUCCESS)

# ----------------------------upload a remote file------------------------------------------------

# ----------------------------download a file-----------------------------------------------------

# ----------------------------delete a file-----------------------------------------------------

# ----------------------------enable rename-----------------------------------------------------

# ----------------------------list files-----------------------------------------------------

# ----------------------------start parsing-----------------------------------------------------

# ----------------------------stop parsing-----------------------------------------------------

# ----------------------------show the status of the file-----------------------------------------------------

# ----------------------------list the chunks of the file-----------------------------------------------------

# ----------------------------delete the chunk-----------------------------------------------------

# ----------------------------edit the status of the chunk-----------------------------------------------------

# ----------------------------insert a new chunk-----------------------------------------------------

# ----------------------------upload a file-----------------------------------------------------

# ----------------------------get a specific chunk-----------------------------------------------------

# ----------------------------retrieval test-----------------------------------------------------

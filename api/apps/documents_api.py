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
import re
import warnings

from flask import request
from flask_login import login_required, current_user

from api.db import FileType, ParserType
from api.db.services import duplicate_name
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import construct_json_result
from api.utils.file_utils import filename_type, thumbnail
from rag.utils.minio_conn import MINIO
from api.db.db_models import Task, File
from api.db import FileType, TaskStatus, ParserType, FileSource


MAXIMUM_OF_UPLOADING_FILES = 256


# ----------------------------upload local files-----------------------------------------------------
@manager.route('/<dataset_id>', methods=['POST'])
@login_required
def upload(dataset_id):
    # no files
    if not request.files:
        return construct_json_result(
            message='There is no file!', code=RetCode.ARGUMENT_ERROR)

    # the number of uploading files exceeds the limit
    file_objs = request.files.getlist('file')
    num_file_objs = len(file_objs)

    if num_file_objs > MAXIMUM_OF_UPLOADING_FILES:
        return construct_json_result(code=RetCode.DATA_ERROR, message=f"You try to upload {num_file_objs} files, "
                                                                      f"which exceeds the maximum number of uploading files: {MAXIMUM_OF_UPLOADING_FILES}")

    for file_obj in file_objs:
        # the content of the file
        file_content = file_obj.read()
        file_name = file_obj.filename
        # no name
        if not file_name:
            return construct_json_result(
                message='There is a file without name!', code=RetCode.ARGUMENT_ERROR)

        # TODO: support the remote files
        if 'http' in file_name:
            return construct_json_result(code=RetCode.ARGUMENT_ERROR, message="Remote files have not unsupported.")

        # the content is empty, raising a warning
        if file_content == b'':
            warnings.warn(f"[WARNING]: The file {file_name} is empty.")

    # no dataset
    exist, dataset = KnowledgebaseService.get_by_id(dataset_id)
    if not exist:
        return construct_json_result(message="Can't find this dataset", code=RetCode.DATA_ERROR)

    # get the root_folder
    root_folder = FileService.get_root_folder(current_user.id)
    # get the id of the root_folder
    parent_file_id = root_folder["id"]  # document id
    # this is for the new user, create '.knowledgebase' file
    FileService.init_knowledgebase_docs(parent_file_id, current_user.id)
    # go inside this folder, get the kb_root_folder
    kb_root_folder = FileService.get_kb_folder(current_user.id)
    # link the file management to the kb_folder
    kb_folder = FileService.new_a_file_from_kb(dataset.tenant_id, dataset.name, kb_root_folder["id"])

    # grab all the errs
    err = []
    MAX_FILE_NUM_PER_USER = int(os.environ.get('MAX_FILE_NUM_PER_USER', 0))
    uploaded_docs_json = []
    for file in file_objs:
        try:
            # TODO: get this value from the database as some tenants have this limit while others don't
            if MAX_FILE_NUM_PER_USER > 0 and DocumentService.get_doc_count(dataset.tenant_id) >= MAX_FILE_NUM_PER_USER:
                return construct_json_result(code=RetCode.DATA_ERROR,
                                             message="Exceed the maximum file number of a free user!")
            # deal with the duplicate name
            filename = duplicate_name(
                DocumentService.query,
                name=file.filename,
                kb_id=dataset.id)

            # deal with the unsupported type
            filetype = filename_type(filename)
            if filetype == FileType.OTHER.value:
                return construct_json_result(code=RetCode.DATA_ERROR,
                                             message="This type of file has not been supported yet!")

            # upload to the minio
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
            uploaded_docs_json.append(doc)
        except Exception as e:
            err.append(file.filename + ": " + str(e))

    if err:
        # return all the errors
        return construct_json_result(message="\n".join(err), code=RetCode.SERVER_ERROR)
    # success
    return construct_json_result(data=uploaded_docs_json, code=RetCode.SUCCESS)

# ----------------------------delete a file-----------------------------------------------------
@manager.route('/<dataset_id>/<document_id>', methods=['DELETE'])
@login_required
def delete(document_id, dataset_id):  # string
    # get the root folder
    root_folder = FileService.get_root_folder(current_user.id)
    # parent file's id
    parent_file_id = root_folder["id"]
    # consider the new user
    FileService.init_knowledgebase_docs(parent_file_id, current_user.id)
    # store all the errors that may have
    errors = ""
    try:
        # whether there is this document
        exist, doc = DocumentService.get_by_id(document_id)
        if not exist:
            return construct_json_result(message=f"Document {document_id} not found!", code=RetCode.DATA_ERROR)
        # whether this doc is authorized by this tenant
        tenant_id = DocumentService.get_tenant_id(document_id)
        if not tenant_id:
            return construct_json_result(message=f"You cannot delete this document {document_id} due to the authorization"
                                                 f" reason!", code=RetCode.AUTHENTICATION_ERROR)

        # get the doc's id and location
        real_dataset_id, location = File2DocumentService.get_minio_address(doc_id=document_id)

        if real_dataset_id != dataset_id:
            return construct_json_result(message=f"The document {document_id} is not in the dataset: {dataset_id}, "
                                                 f"but in the dataset: {real_dataset_id}.", code=RetCode.ARGUMENT_ERROR)

        # there is an issue when removing
        if not DocumentService.remove_document(doc, tenant_id):
            return construct_json_result(
                message="There was an error during the document removal process. Please check the status of the "
                        "RAGFlow server and try the removal again.", code=RetCode.OPERATING_ERROR)

        # fetch the File2Document record associated with the provided document ID.
        file_to_doc = File2DocumentService.get_by_document_id(document_id)
        # delete the associated File record.
        FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == file_to_doc[0].file_id])
        # delete the File2Document record itself using the document ID. This removes the
        # association between the document and the file after the File record has been deleted.
        File2DocumentService.delete_by_document_id(document_id)

        # delete it from minio
        MINIO.rm(dataset_id, location)
    except Exception as e:
        errors += str(e)
    if errors:
        return construct_json_result(data=False, message=errors, code=RetCode.SERVER_ERROR)

    return construct_json_result(data=True, code=RetCode.SUCCESS)

# ----------------------------upload online files------------------------------------------------

# ----------------------------download a file-----------------------------------------------------

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

#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
import logging
import re

from quart import request, make_response
from api.apps import login_required
from api.db import FileType
from api.db.services.file2document_service import File2DocumentService
from api.utils.api_utils import (
    add_tenant_id_to_kwargs,
    get_error_argument_result,
    get_error_data_result,
    get_result,
)
from api.utils.validation_utils import (
    CreateFolderReq,
    DeleteFileReq,
    ListFileReq,
    MoveFileReq,
    RenameFileReq,
    validate_and_parse_json_request,
    validate_and_parse_request_args,
)
from api.utils.web_utils import CONTENT_TYPE_MAP, apply_safe_file_response_headers
from common import settings
from common.misc_utils import thread_pool_exec
from api.apps.services import file_api_service


@manager.route("/files", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def create_or_upload(tenant_id: str = None):
    """
    Upload files or create a folder.
    ---
    tags:
      - Files
    security:
      - ApiKeyAuth: []
    parameters:
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Successful operation.
    """
    content_type = request.content_type or ""
    try:
        if "multipart/form-data" in content_type:
            form = await request.form
            pf_id = form.get("parent_id")
            files = await request.files
            if 'file' not in files:
                return get_error_argument_result("No file part!")
            file_objs = files.getlist('file')
            for file_obj in file_objs:
                if file_obj.filename == '':
                    return get_error_argument_result("No file selected!")

            success, result = await file_api_service.upload_file(tenant_id, pf_id, file_objs)
            if success:
                return get_result(data=result)
            else:
                return get_error_data_result(message=result)
        else:
            req, err = await validate_and_parse_json_request(request, CreateFolderReq)
            if err is not None:
                return get_error_argument_result(err)

            success, result = await file_api_service.create_folder(
                tenant_id, req["name"], req.get("parent_id"), req.get("type")
            )
            if success:
                return get_result(data=result)
            else:
                return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/files", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def list_files(tenant_id: str = None):
    """
    List files under a folder.
    ---
    tags:
      - Files
    security:
      - ApiKeyAuth: []
    parameters:
      - in: query
        name: parent_id
        type: string
        description: Folder ID to list files from.
      - in: query
        name: keywords
        type: string
        description: Search keyword filter.
      - in: query
        name: page
        type: integer
        default: 1
      - in: query
        name: page_size
        type: integer
        default: 15
      - in: query
        name: orderby
        type: string
        default: "create_time"
      - in: query
        name: desc
        type: boolean
        default: true
    responses:
      200:
        description: Successful operation.
    """
    args, err = validate_and_parse_request_args(request, ListFileReq)
    if err is not None:
        return get_error_argument_result(err)

    try:
        success, result = file_api_service.list_files(tenant_id, args)
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/files", methods=["DELETE"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def delete(tenant_id: str = None):
    """
    Delete files.
    ---
    tags:
      - Files
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        required: true
        schema:
          type: object
          required:
            - ids
          properties:
            ids:
              type: array
              items:
                type: string
              description: List of file IDs to delete.
    responses:
      200:
        description: Successful operation.
    """
    req, err = await validate_and_parse_json_request(request, DeleteFileReq)
    if err is not None:
        return get_error_argument_result(err)

    try:
        success, result = await file_api_service.delete_files(tenant_id, req["ids"])
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")



@manager.route("/files/move", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def move(tenant_id: str = None):
    """
    Move files to a destination folder.
    ---
    tags:
      - Files
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        required: true
        schema:
          type: object
          required:
            - src_file_ids
            - dest_file_id
          properties:
            src_file_ids:
              type: array
              items:
                type: string
            dest_file_id:
              type: string
    responses:
      200:
        description: Successful operation.
    """
    req, err = await validate_and_parse_json_request(request, MoveFileReq)
    if err is not None:
        return get_error_argument_result(err)

    try:
        success, result = await file_api_service.move_files(tenant_id, req["src_file_ids"], req["dest_file_id"])
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/files/<file_id>", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def download(tenant_id: str = None, file_id: str = None):
    """
    Download a file.
    ---
    tags:
      - Files
    security:
      - ApiKeyAuth: []
    produces:
      - application/octet-stream
    parameters:
      - in: path
        name: file_id
        type: string
        required: true
        description: File ID to download.
    responses:
      200:
        description: File stream.
    """
    try:
        success, result = file_api_service.get_file_content(tenant_id, file_id)
        if not success:
            return get_error_data_result(message=result)

        file = result
        blob = await thread_pool_exec(settings.STORAGE_IMPL.get, file.parent_id, file.location)
        if not blob:
            b, n = File2DocumentService.get_storage_address(file_id=file_id)
            blob = await thread_pool_exec(settings.STORAGE_IMPL.get, b, n)

        response = await make_response(blob)
        ext = re.search(r"\.([^.]+)$", file.name.lower())
        ext = ext.group(1) if ext else None
        content_type = None
        if ext:
            fallback_prefix = "image" if file.type == FileType.VISUAL.value else "application"
            content_type = CONTENT_TYPE_MAP.get(ext, f"{fallback_prefix}/{ext}")
        apply_safe_file_response_headers(response, content_type, ext)
        return response
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/files/<file_id>/parent", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def parent_folder(tenant_id: str = None, file_id: str = None):
    """
    Get parent folder of a file.
    ---
    tags:
      - Files
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: file_id
        type: string
        required: true
    responses:
      200:
        description: Parent folder information.
    """
    try:
        success, result = file_api_service.get_parent_folder(file_id)
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/files/<file_id>/ancestors", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def ancestors(tenant_id: str = None, file_id: str = None):
    """
    Get all ancestor folders of a file.
    ---
    tags:
      - Files
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: file_id
        type: string
        required: true
    responses:
      200:
        description: List of ancestor folders.
    """
    try:
        success, result = file_api_service.get_all_parent_folders(file_id)
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/files/<file_id>", methods=["PUT"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def rename(tenant_id: str = None, file_id: str = None):
    """
    Rename a file.
    ---
    tags:
      - Files
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: file_id
        type: string
        required: true
      - in: body
        name: body
        required: true
        schema:
          type: object
          required:
            - name
          properties:
            name:
              type: string
              description: New file name.
    responses:
      200:
        description: Successful operation.
    """
    req, err = await validate_and_parse_json_request(request, RenameFileReq)
    if err is not None:
        return get_error_argument_result(err)

    try:
        success, result = await file_api_service.rename_file(tenant_id, file_id, req["name"])
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")

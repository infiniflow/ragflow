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
import logging
import os
import pathlib
import re
from typing import Annotated
from quart import request, make_response
from pydantic import BaseModel, ConfigDict, Field
from quart_schema import validate_request as qs_validate_request, validate_response, tag
from api.apps import login_required, current_user

from api.common.check_team_permission import check_file_team_permission
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request
from common.misc_utils import get_uuid, thread_pool_exec
from common.constants import RetCode, FileSource
from api.db import FileType
from api.db.services import duplicate_name
from api.db.services.file_service import FileService
from api.utils.api_utils import get_json_result, get_request_json
from api.utils.file_utils import filename_type
from api.utils.web_utils import CONTENT_TYPE_MAP
from common import settings


# Pydantic Schemas for OpenAPI Documentation

class BaseSchema(BaseModel):
    """Base schema with common configuration."""
    model_config = ConfigDict(extra="ignore", strict=False)


class UploadResponse(BaseModel):
    """Response schema for file upload."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[list[dict], Field(..., description="List of uploaded file information")]
    message: Annotated[str, Field("Success", description="Response message")]


class CreateFileRequest(BaseSchema):
    """Request schema for creating a file or folder."""
    name: Annotated[str, Field(..., description="File or folder name", min_length=1, max_length=255)]
    parent_id: Annotated[str | None, Field(None, description="Parent folder ID. If not provided, uses root folder")]
    type: Annotated[str | None, Field(None, description="File type (FOLDER or VIRTUAL)")]


class CreateFileResponse(BaseModel):
    """Response schema for creating a file or folder."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Created file/folder information")]
    message: Annotated[str, Field("Success", description="Response message")]


class ListFilesResponse(BaseModel):
    """Response schema for listing files."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(
        ...,
        description="Contains total (int), files (list), and parent_folder (dict) with file listing information"
    )]
    message: Annotated[str, Field("Success", description="Response message")]


class RootFolderResponse(BaseModel):
    """Response schema for getting root folder."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Root folder information")]
    message: Annotated[str, Field("Success", description="Response message")]


class ParentFolderResponse(BaseModel):
    """Response schema for getting parent folder."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Parent folder information")]
    message: Annotated[str, Field("Success", description="Response message")]


class AllParentFoldersResponse(BaseModel):
    """Response schema for getting all parent folders."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="List of all parent folders in hierarchy")]
    message: Annotated[str, Field("Success", description="Response message")]


class DeleteFilesRequest(BaseSchema):
    """Request schema for deleting files."""
    file_ids: Annotated[list[str], Field(..., description="List of file IDs to delete")]


class DeleteFilesResponse(BaseModel):
    """Response schema for deleting files."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="Deletion status, True if successful")]
    message: Annotated[str, Field("Success", description="Response message")]


class RenameFileRequest(BaseSchema):
    """Request schema for renaming a file."""
    file_id: Annotated[str, Field(..., description="File ID to rename")]
    name: Annotated[str, Field(..., description="New file name", min_length=1, max_length=255)]


class RenameFileResponse(BaseModel):
    """Response schema for renaming a file."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="Rename status, True if successful")]
    message: Annotated[str, Field("Success", description="Response message")]


class MoveFilesRequest(BaseSchema):
    """Request schema for moving files."""
    src_file_ids: Annotated[list[str], Field(..., description="List of source file IDs to move")]
    dest_file_id: Annotated[str, Field(..., description="Destination folder ID")]


class MoveFilesResponse(BaseModel):
    """Response schema for moving files."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="Move status, True if successful")]
    message: Annotated[str, Field("Success", description="Response message")]


# File Management API Tag
file_tag = tag(["file"])


@manager.route('/upload', methods=['POST'])  # noqa: F821
@login_required
@validate_response(200, UploadResponse)
@file_tag
async def upload():
    """
    Upload files to the file management system.

    Uploads one or more files to a specified parent folder. If parent_id is not provided,
    files are uploaded to the user's root folder. The system automatically creates folder
    structure based on file paths and handles duplicate names by appending underscores.
    Files are stored in the configured storage backend (e.g., MinIO).

    **Request:**
    - Content-Type: multipart/form-data
    - file: One or more files to upload
    - parent_id: (Optional) Parent folder ID where files should be uploaded

    **Response:**
    - List of uploaded file information including id, name, size, type, and location
    """
    form = await request.form
    pf_id = form.get("parent_id")

    if not pf_id:
        root_folder = FileService.get_root_folder(current_user.id)
        pf_id = root_folder["id"]

    files = await request.files
    if 'file' not in files:
        return get_json_result(
            data=False, message='No file part!', code=RetCode.ARGUMENT_ERROR)
    file_objs = files.getlist('file')

    for file_obj in file_objs:
        if file_obj.filename == '':
            return get_json_result(
                data=False, message='No file selected!', code=RetCode.ARGUMENT_ERROR)
    file_res = []
    try:
        e, pf_folder = FileService.get_by_id(pf_id)
        if not e:
            return get_data_error_result( message="Can't find this folder!")

        async def _handle_single_file(file_obj):
            MAX_FILE_NUM_PER_USER: int = int(os.environ.get('MAX_FILE_NUM_PER_USER', 0))
            if 0 < MAX_FILE_NUM_PER_USER <= await thread_pool_exec(DocumentService.get_doc_count, current_user.id):
                return get_data_error_result( message="Exceed the maximum file number of a free user!")

            # split file name path
            if not file_obj.filename:
                file_obj_names = [pf_folder.name, file_obj.filename]
            else:
                full_path = '/' + file_obj.filename
                file_obj_names = full_path.split('/')
            file_len = len(file_obj_names)

            # get folder
            file_id_list = await thread_pool_exec(FileService.get_id_list_by_id, pf_id, file_obj_names, 1, [pf_id])
            len_id_list = len(file_id_list)

            # create folder
            if file_len != len_id_list:
                e, file = await thread_pool_exec(FileService.get_by_id, file_id_list[len_id_list - 1])
                if not e:
                    return get_data_error_result(message="Folder not found!")
                last_folder = await thread_pool_exec(FileService.create_folder, file, file_id_list[len_id_list - 1], file_obj_names,
                                                        len_id_list)
            else:
                e, file = await thread_pool_exec(FileService.get_by_id, file_id_list[len_id_list - 2])
                if not e:
                    return get_data_error_result(message="Folder not found!")
                last_folder = await thread_pool_exec(FileService.create_folder, file, file_id_list[len_id_list - 2], file_obj_names,
                                                        len_id_list)

            # file type
            filetype = filename_type(file_obj_names[file_len - 1])
            location = file_obj_names[file_len - 1]
            while await thread_pool_exec(settings.STORAGE_IMPL.obj_exist, last_folder.id, location):
                location += "_"
            blob = await thread_pool_exec(file_obj.read)
            filename = await thread_pool_exec(
                duplicate_name,
                FileService.query,
                name=file_obj_names[file_len - 1],
                parent_id=last_folder.id)
            await thread_pool_exec(settings.STORAGE_IMPL.put, last_folder.id, location, blob)
            file_data = {
                "id": get_uuid(),
                "parent_id": last_folder.id,
                "tenant_id": current_user.id,
                "created_by": current_user.id,
                "type": filetype,
                "name": filename,
                "location": location,
                "size": len(blob),
            }
            inserted = await thread_pool_exec(FileService.insert, file_data)
            return inserted.to_json()

        for file_obj in file_objs:
            res = await _handle_single_file(file_obj)
            file_res.append(res)

        return get_json_result(data=file_res)
    except Exception as e:
        return server_error_response(e)


@manager.route('/create', methods=['POST'])  # noqa: F821
@login_required
@validate_request("name")
@qs_validate_request(CreateFileRequest)
@validate_response(200, CreateFileResponse)
@file_tag
async def create():
    """
    Create a new folder or virtual file.

    Creates a new folder or virtual file in the specified parent location.
    If parent_id is not provided, the item is created in the user's root folder.
    The type parameter determines whether to create a FOLDER or VIRTUAL file.

    **Request Body:**
    - name: (Required) Name for the file/folder
    - parent_id: (Optional) Parent folder ID
    - type: (Optional) File type - FOLDER or VIRTUAL (defaults to VIRTUAL)

    **Response:**
    - Created file/folder information including id, name, type, and timestamps
    """
    req = await get_request_json()
    pf_id = req.get("parent_id")
    input_file_type = req.get("type")
    if not pf_id:
        root_folder = FileService.get_root_folder(current_user.id)
        pf_id = root_folder["id"]

    try:
        if not FileService.is_parent_folder_exist(pf_id):
            return get_json_result(
                data=False, message="Parent Folder Doesn't Exist!", code=RetCode.OPERATING_ERROR)
        if FileService.query(name=req["name"], parent_id=pf_id):
            return get_data_error_result(
                message="Duplicated folder name in the same folder.")

        if input_file_type == FileType.FOLDER.value:
            file_type = FileType.FOLDER.value
        else:
            file_type = FileType.VIRTUAL.value

        file = FileService.insert({
            "id": get_uuid(),
            "parent_id": pf_id,
            "tenant_id": current_user.id,
            "created_by": current_user.id,
            "name": req["name"],
            "location": "",
            "size": 0,
            "type": file_type
        })

        return get_json_result(data=file.to_json())
    except Exception as e:
        return server_error_response(e)


@manager.route('/list', methods=['GET'])  # noqa: F821
@login_required
@validate_response(200, ListFilesResponse)
@file_tag
def list_files():
    """
    List files and folders in a parent directory.

    Retrieves a paginated list of files and folders within the specified parent folder.
    Supports filtering by keywords and sorting by various fields. If parent_id is not
    provided, lists contents of the user's root folder.

    **Query Parameters:**
    - parent_id: (Optional) Parent folder ID to list contents from
    - keywords: (Optional) Search/filter keywords for file names
    - page: (Optional) Page number, defaults to 1
    - page_size: (Optional) Items per page, defaults to 15
    - orderby: (Optional) Field to order by, defaults to "create_time"
    - desc: (Optional) Sort descending, defaults to True

    **Response:**
    - total: Total number of files/folders
    - files: List of file/folder objects with metadata
    - parent_folder: Parent folder information
    """
    pf_id = request.args.get("parent_id")

    keywords = request.args.get("keywords", "")

    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 15))
    orderby = request.args.get("orderby", "create_time")
    desc = request.args.get("desc", True)
    if not pf_id:
        root_folder = FileService.get_root_folder(current_user.id)
        pf_id = root_folder["id"]
        FileService.init_knowledgebase_docs(pf_id, current_user.id)
    try:
        e, file = FileService.get_by_id(pf_id)
        if not e:
            return get_data_error_result(message="Folder not found!")

        files, total = FileService.get_by_pf_id(
            current_user.id, pf_id, page_number, items_per_page, orderby, desc, keywords)

        parent_folder = FileService.get_parent_folder(pf_id)
        if not parent_folder:
            return get_json_result(message="File not found!")

        return get_json_result(data={"total": total, "files": files, "parent_folder": parent_folder.to_json()})
    except Exception as e:
        return server_error_response(e)


@manager.route('/root_folder', methods=['GET'])  # noqa: F821
@login_required
@validate_response(200, RootFolderResponse)
@file_tag
def get_root_folder():
    """
    Get the user's root folder.

    Retrieves the root folder information for the currently authenticated user.
    Each user has a unique root folder that serves as the top-level container
    for all their files and folders.

    **Response:**
    - root_folder: Root folder object with id, name, and other metadata
    """
    try:
        root_folder = FileService.get_root_folder(current_user.id)
        return get_json_result(data={"root_folder": root_folder})
    except Exception as e:
        return server_error_response(e)


@manager.route('/parent_folder', methods=['GET'])  # noqa: F821
@login_required
@validate_response(200, ParentFolderResponse)
@file_tag
def get_parent_folder():
    """
    Get the immediate parent folder of a file.

    Retrieves information about the parent folder of the specified file or folder.
    Useful for building breadcrumb navigation or folder traversal.

    **Query Parameters:**
    - file_id: (Required) File ID to get parent folder for

    **Response:**
    - parent_folder: Parent folder information with id, name, and metadata
    """
    file_id = request.args.get("file_id")
    try:
        e, file = FileService.get_by_id(file_id)
        if not e:
            return get_data_error_result(message="Folder not found!")

        parent_folder = FileService.get_parent_folder(file_id)
        return get_json_result(data={"parent_folder": parent_folder.to_json()})
    except Exception as e:
        return server_error_response(e)


@manager.route('/all_parent_folder', methods=['GET'])  # noqa: F821
@login_required
@validate_response(200, AllParentFoldersResponse)
@file_tag
def get_all_parent_folders():
    """
    Get all parent folders in the hierarchy.

    Retrieves the complete hierarchy of parent folders for the specified file,
    from the immediate parent up to the root folder. Useful for displaying
    the full breadcrumb path.

    **Query Parameters:**
    - file_id: (Required) File ID to get parent folder hierarchy for

    **Response:**
    - parent_folders: List of all parent folders in order from root to immediate parent
    """
    file_id = request.args.get("file_id")
    try:
        e, file = FileService.get_by_id(file_id)
        if not e:
            return get_data_error_result(message="Folder not found!")

        parent_folders = FileService.get_all_parent_folders(file_id)
        parent_folders_res = []
        for parent_folder in parent_folders:
            parent_folders_res.append(parent_folder.to_json())
        return get_json_result(data={"parent_folders": parent_folders_res})
    except Exception as e:
        return server_error_response(e)


@manager.route("/rm", methods=["POST"])  # noqa: F821
@login_required
@validate_request("file_ids")
@qs_validate_request(DeleteFilesRequest)
@validate_response(200, DeleteFilesResponse)
@file_tag
async def rm():
    """
    Delete files or folders.

    Deletes the specified files or folders. For folders, the entire contents
    are deleted recursively. Files are removed from storage and associated
    documents are cleaned up. This operation is irreversible.

    **Request Body:**
    - file_ids: (Required) List of file/folder IDs to delete

    **Response:**
    - Deletion status (True if successful)
    """
    req = await get_request_json()
    file_ids = req["file_ids"]

    try:
        def _delete_single_file(file):
            try:
                if file.location:
                    settings.STORAGE_IMPL.rm(file.parent_id, file.location)
            except Exception as e:
                logging.exception(f"Fail to remove object: {file.parent_id}/{file.location}, error: {e}")

            informs = File2DocumentService.get_by_file_id(file.id)
            for inform in informs:
                doc_id = inform.document_id
                e, doc = DocumentService.get_by_id(doc_id)
                if e and doc:
                    tenant_id = DocumentService.get_tenant_id(doc_id)
                    if tenant_id:
                        DocumentService.remove_document(doc, tenant_id)
                File2DocumentService.delete_by_file_id(file.id)

            FileService.delete(file)

        def _delete_folder_recursive(folder, tenant_id):
            sub_files = FileService.list_all_files_by_parent_id(folder.id)
            for sub_file in sub_files:
                if sub_file.type == FileType.FOLDER.value:
                    _delete_folder_recursive(sub_file, tenant_id)
                else:
                    _delete_single_file(sub_file)

            FileService.delete(folder)

        def _rm_sync():
            for file_id in file_ids:
                e, file = FileService.get_by_id(file_id)
                if not e or not file:
                    return get_data_error_result(message="File or Folder not found!")
                if not file.tenant_id:
                    return get_data_error_result(message="Tenant not found!")
                if not check_file_team_permission(file, current_user.id):
                    return get_json_result(data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)

                if file.source_type == FileSource.KNOWLEDGEBASE:
                    continue

                if file.type == FileType.FOLDER.value:
                    _delete_folder_recursive(file, current_user.id)
                    continue

                _delete_single_file(file)

            return get_json_result(data=True)

        return await thread_pool_exec(_rm_sync)

    except Exception as e:
        return server_error_response(e)


@manager.route('/rename', methods=['POST'])  # noqa: F821
@login_required
@validate_request("file_id", "name")
@qs_validate_request(RenameFileRequest)
@validate_response(200, RenameFileResponse)
@file_tag
async def rename():
    """
    Rename a file or folder.

    Renames the specified file or folder to a new name. The file extension
    cannot be changed for non-folder files. Duplicate names within the same
    parent folder are not allowed.

    **Request Body:**
    - file_id: (Required) File ID to rename
    - name: (Required) New name for the file/folder

    **Response:**
    - Rename status (True if successful)
    """
    req = await get_request_json()
    try:
        e, file = FileService.get_by_id(req["file_id"])
        if not e:
            return get_data_error_result(message="File not found!")
        if not check_file_team_permission(file, current_user.id):
            return get_json_result(data=False, message='No authorization.', code=RetCode.AUTHENTICATION_ERROR)
        if file.type != FileType.FOLDER.value \
            and pathlib.Path(req["name"].lower()).suffix != pathlib.Path(
                file.name.lower()).suffix:
            return get_json_result(
                data=False,
                message="The extension of file can't be changed",
                code=RetCode.ARGUMENT_ERROR)
        for file in FileService.query(name=req["name"], pf_id=file.parent_id):
            if file.name == req["name"]:
                return get_data_error_result(
                    message="Duplicated file name in the same folder.")

        if not FileService.update_by_id(
                req["file_id"], {"name": req["name"]}):
            return get_data_error_result(
                message="Database error (File rename)!")

        informs = File2DocumentService.get_by_file_id(req["file_id"])
        if informs:
            if not DocumentService.update_by_id(
                    informs[0].document_id, {"name": req["name"]}):
                return get_data_error_result(
                    message="Database error (Document rename)!")

        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/get/<file_id>', methods=['GET'])  # noqa: F821
@login_required
@file_tag
async def get(file_id):
    """
    Download or view a file.

    Retrieves and returns the file content with appropriate content-type headers.
    The file is streamed from the storage backend. Supports various file types
    including images, PDFs, and other documents.

    **Path Parameters:**
    - file_id: (Required) File ID to retrieve

    **Response:**
    - File content with appropriate Content-Type header
    """
    try:
        e, file = FileService.get_by_id(file_id)
        if not e:
            return get_data_error_result(message="Document not found!")
        if not check_file_team_permission(file, current_user.id):
            return get_json_result(data=False, message='No authorization.', code=RetCode.AUTHENTICATION_ERROR)

        blob = await thread_pool_exec(settings.STORAGE_IMPL.get, file.parent_id, file.location)
        if not blob:
            b, n = File2DocumentService.get_storage_address(file_id=file_id)
            blob = await thread_pool_exec(settings.STORAGE_IMPL.get, b, n)

        response = await make_response(blob)
        ext = re.search(r"\.([^.]+)$", file.name.lower())
        ext = ext.group(1) if ext else None
        if ext:
            if file.type == FileType.VISUAL.value:
                content_type = CONTENT_TYPE_MAP.get(ext, f"image/{ext}")
            else:
                content_type = CONTENT_TYPE_MAP.get(ext, f"application/{ext}")
            response.headers.set("Content-Type", content_type)
        return response
    except Exception as e:
        return server_error_response(e)


@manager.route("/mv", methods=["POST"])  # noqa: F821
@login_required
@validate_request("src_file_ids", "dest_file_id")
@qs_validate_request(MoveFilesRequest)
@validate_response(200, MoveFilesResponse)
@file_tag
async def move():
    """
    Move files or folders to a different location.

    Moves the specified files or folders to a destination folder. For folders,
    the entire contents including subfolders are moved. The operation updates
    both the database records and the storage layer. Handles name conflicts
    by appending underscores.

    **Request Body:**
    - src_file_ids: (Required) List of source file/folder IDs to move
    - dest_file_id: (Required) Destination folder ID

    **Response:**
    - Move status (True if successful)
    """
    req = await get_request_json()
    try:
        file_ids = req["src_file_ids"]
        dest_parent_id = req["dest_file_id"]

        ok, dest_folder = FileService.get_by_id(dest_parent_id)
        if not ok or not dest_folder:
            return get_data_error_result(message="Parent folder not found!")

        files = FileService.get_by_ids(file_ids)
        if not files:
            return get_data_error_result(message="Source files not found!")

        files_dict = {f.id: f for f in files}

        for file_id in file_ids:
            file = files_dict.get(file_id)
            if not file:
                return get_data_error_result(message="File or folder not found!")
            if not file.tenant_id:
                return get_data_error_result(message="Tenant not found!")
            if not check_file_team_permission(file, current_user.id):
                return get_json_result(
                    data=False,
                    message="No authorization.",
                    code=RetCode.AUTHENTICATION_ERROR,
                )

        def _move_entry_recursive(source_file_entry, dest_folder):
            if source_file_entry.type == FileType.FOLDER.value:
                existing_folder = FileService.query(name=source_file_entry.name, parent_id=dest_folder.id)
                if existing_folder:
                    new_folder = existing_folder[0]
                else:
                    new_folder = FileService.insert(
                        {
                            "id": get_uuid(),
                            "parent_id": dest_folder.id,
                            "tenant_id": source_file_entry.tenant_id,
                            "created_by": current_user.id,
                            "name": source_file_entry.name,
                            "location": "",
                            "size": 0,
                            "type": FileType.FOLDER.value,
                        }
                    )

                sub_files = FileService.list_all_files_by_parent_id(source_file_entry.id)
                for sub_file in sub_files:
                    _move_entry_recursive(sub_file, new_folder)

                FileService.delete_by_id(source_file_entry.id)
                return

            old_parent_id = source_file_entry.parent_id
            old_location = source_file_entry.location
            filename = source_file_entry.name

            new_location = filename
            while settings.STORAGE_IMPL.obj_exist(dest_folder.id, new_location):
                new_location += "_"

            try:
                settings.STORAGE_IMPL.move(old_parent_id, old_location, dest_folder.id, new_location)
            except Exception as storage_err:
                raise RuntimeError(f"Move file failed at storage layer: {str(storage_err)}")

            FileService.update_by_id(
                source_file_entry.id,
                {
                    "parent_id": dest_folder.id,
                    "location": new_location,
                },
            )

        def _move_sync():
            for file in files:
                _move_entry_recursive(file, dest_folder)
            return get_json_result(data=True)

        return await thread_pool_exec(_move_sync)

    except Exception as e:
        return server_error_response(e)

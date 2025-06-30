import pathlib
import re

import flask
from flask import request

from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.utils.api_utils import server_error_response, token_required
from api.utils import get_uuid
from api.db import FileType
from api.db.services import duplicate_name
from api.db.services.file_service import FileService
from api.utils.api_utils import get_json_result
from api.utils.file_utils import filename_type
from rag.utils.storage_factory import STORAGE_IMPL

@manager.route('/file/upload', methods=['POST']) # noqa: F821
@token_required
def upload(tenant_id):
    """
    Upload a file to the system.
    ---
    tags:
      - File Management
    security:
      - ApiKeyAuth: []
    parameters:
      - in: formData
        name: file
        type: file
        required: true
        description: The file to upload
      - in: formData
        name: parent_id
        type: string
        description: Parent folder ID where the file will be uploaded. Optional.
    responses:
      200:
        description: Successfully uploaded the file.
        schema:
          type: object
          properties:
            data:
            type: array
            items:
              type: object
              properties:
                id:
                  type: string
                  description: File ID
                name:
                  type: string
                  description: File name
                size:
                  type: integer
                  description: File size in bytes
                type:
                  type: string
                  description: File type (e.g., document, folder)
    """
    pf_id = request.form.get("parent_id")

    if not pf_id:
        root_folder = FileService.get_root_folder(tenant_id)
        pf_id = root_folder["id"]

    if 'file' not in request.files:
        return get_json_result(data=False, message='No file part!', code=400)
    file_objs = request.files.getlist('file')

    for file_obj in file_objs:
        if file_obj.filename == '':
            return get_json_result(data=False, message='No selected file!', code=400)

    file_res = []

    try:
        e, pf_folder = FileService.get_by_id(pf_id)
        if not e:
            return get_json_result(data=False, message="Can't find this folder!", code=404)

        for file_obj in file_objs:
            # 文件路径处理
            full_path = '/' + file_obj.filename
            file_obj_names = full_path.split('/')
            file_len = len(file_obj_names)

            # 获取文件夹路径ID
            file_id_list = FileService.get_id_list_by_id(pf_id, file_obj_names, 1, [pf_id])
            len_id_list = len(file_id_list)

            # 创建文件夹结构
            if file_len != len_id_list:
                e, file = FileService.get_by_id(file_id_list[len_id_list - 1])
                if not e:
                    return get_json_result(data=False, message="Folder not found!", code=404)
                last_folder = FileService.create_folder(file, file_id_list[len_id_list - 1], file_obj_names, len_id_list)
            else:
                e, file = FileService.get_by_id(file_id_list[len_id_list - 2])
                if not e:
                    return get_json_result(data=False, message="Folder not found!", code=404)
                last_folder = FileService.create_folder(file, file_id_list[len_id_list - 2], file_obj_names, len_id_list)

            filetype = filename_type(file_obj_names[file_len - 1])
            location = file_obj_names[file_len - 1]
            while STORAGE_IMPL.obj_exist(last_folder.id, location):
                location += "_"
            blob = file_obj.read()
            filename = duplicate_name(FileService.query, name=file_obj_names[file_len - 1], parent_id=last_folder.id)

            file = {
                "id": get_uuid(),
                "parent_id": last_folder.id,
                "tenant_id": tenant_id,
                "created_by": tenant_id,
                "type": filetype,
                "name": filename,
                "location": location,
                "size": len(blob),
            }
            file = FileService.insert(file)
            STORAGE_IMPL.put(last_folder.id, location, blob)
            file_res.append(file.to_json())
        return get_json_result(data=file_res)
    except Exception as e:
        return server_error_response(e)


@manager.route('/file/create', methods=['POST']) # noqa: F821
@token_required
def create(tenant_id):
    """
    Create a new file or folder.
    ---
    tags:
      - File Management
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        description: File creation parameters
        required: true
        schema:
          type: object
          properties:
            name:
              type: string
              description: Name of the file/folder
            parent_id:
              type: string
              description: Parent folder ID. Optional.
            type:
              type: string
              enum: ["FOLDER", "VIRTUAL"]
              description: Type of the file
    responses:
      200:
        description: File created successfully.
        schema:
          type: object
          properties:
            data:
              type: object
              properties:
                id:
                  type: string
                name:
                  type: string
                type:
                  type: string
    """
    req = request.json
    pf_id = request.json.get("parent_id")
    input_file_type = request.json.get("type")
    if not pf_id:
        root_folder = FileService.get_root_folder(tenant_id)
        pf_id = root_folder["id"]

    try:
        if not FileService.is_parent_folder_exist(pf_id):
            return get_json_result(data=False, message="Parent Folder Doesn't Exist!", code=400)
        if FileService.query(name=req["name"], parent_id=pf_id):
            return get_json_result(data=False, message="Duplicated folder name in the same folder.", code=409)

        if input_file_type == FileType.FOLDER.value:
            file_type = FileType.FOLDER.value
        else:
            file_type = FileType.VIRTUAL.value

        file = FileService.insert({
            "id": get_uuid(),
            "parent_id": pf_id,
            "tenant_id": tenant_id,
            "created_by": tenant_id,
            "name": req["name"],
            "location": "",
            "size": 0,
            "type": file_type
        })

        return get_json_result(data=file.to_json())
    except Exception as e:
        return server_error_response(e)


@manager.route('/file/list', methods=['GET']) # noqa: F821
@token_required
def list_files(tenant_id):
    """
    List files under a specific folder.
    ---
    tags:
      - File Management
    security:
      - ApiKeyAuth: []
    parameters:
      - in: query
        name: parent_id
        type: string
        description: Folder ID to list files from
      - in: query
        name: keywords
        type: string
        description: Search keyword filter
      - in: query
        name: page
        type: integer
        default: 1
        description: Page number
      - in: query
        name: page_size
        type: integer
        default: 15
        description: Number of results per page
      - in: query
        name: orderby
        type: string
        default: "create_time"
        description: Sort by field
      - in: query
        name: desc
        type: boolean
        default: true
        description: Descending order
    responses:
      200:
        description: Successfully retrieved file list.
        schema:
          type: object
          properties:
            total:
              type: integer
            files:
              type: array
              items:
                type: object
                properties:
                  id:
                    type: string
                  name:
                    type: string
                  type:
                    type: string
                  size:
                    type: integer
                  create_time:
                    type: string
                    format: date-time
    """
    pf_id = request.args.get("parent_id")
    keywords = request.args.get("keywords", "")
    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 15))
    orderby = request.args.get("orderby", "create_time")
    desc = request.args.get("desc", True)

    if not pf_id:
        root_folder = FileService.get_root_folder(tenant_id)
        pf_id = root_folder["id"]
        FileService.init_knowledgebase_docs(pf_id, tenant_id)

    try:
        e, file = FileService.get_by_id(pf_id)
        if not e:
            return get_json_result(message="Folder not found!", code=404)

        files, total = FileService.get_by_pf_id(tenant_id, pf_id, page_number, items_per_page, orderby, desc, keywords)

        parent_folder = FileService.get_parent_folder(pf_id)
        if not parent_folder:
            return get_json_result(message="File not found!", code=404)

        return get_json_result(data={"total": total, "files": files, "parent_folder": parent_folder.to_json()})
    except Exception as e:
        return server_error_response(e)


@manager.route('/file/root_folder', methods=['GET']) # noqa: F821
@token_required
def get_root_folder(tenant_id):
    """
    Get user's root folder.
    ---
    tags:
      - File Management
    security:
      - ApiKeyAuth: []
    responses:
      200:
        description: Root folder information
        schema:
          type: object
          properties:
            data:
              type: object
              properties:
                root_folder:
                  type: object
                  properties:
                    id:
                      type: string
                    name:
                      type: string
                    type:
                      type: string
    """
    try:
        root_folder = FileService.get_root_folder(tenant_id)
        return get_json_result(data={"root_folder": root_folder})
    except Exception as e:
        return server_error_response(e)


@manager.route('/file/parent_folder', methods=['GET']) # noqa: F821
@token_required
def get_parent_folder():
    """
    Get parent folder info of a file.
    ---
    tags:
      - File Management
    security:
      - ApiKeyAuth: []
    parameters:
      - in: query
        name: file_id
        type: string
        required: true
        description: Target file ID
    responses:
      200:
        description: Parent folder information
        schema:
          type: object
          properties:
            data:
              type: object
              properties:
                parent_folder:
                  type: object
                  properties:
                    id:
                      type: string
                    name:
                      type: string
    """
    file_id = request.args.get("file_id")
    try:
        e, file = FileService.get_by_id(file_id)
        if not e:
            return get_json_result(message="Folder not found!", code=404)

        parent_folder = FileService.get_parent_folder(file_id)
        return get_json_result(data={"parent_folder": parent_folder.to_json()})
    except Exception as e:
        return server_error_response(e)


@manager.route('/file/all_parent_folder', methods=['GET']) # noqa: F821
@token_required
def get_all_parent_folders(tenant_id):
    """
    Get all parent folders of a file.
    ---
    tags:
      - File Management
    security:
      - ApiKeyAuth: []
    parameters:
      - in: query
        name: file_id
        type: string
        required: true
        description: Target file ID
    responses:
      200:
        description: All parent folders of the file
        schema:
          type: object
          properties:
            data:
              type: object
              properties:
                parent_folders:
                  type: array
                  items:
                    type: object
                    properties:
                      id:
                        type: string
                      name:
                        type: string
    """
    file_id = request.args.get("file_id")
    try:
        e, file = FileService.get_by_id(file_id)
        if not e:
            return get_json_result(message="Folder not found!", code=404)

        parent_folders = FileService.get_all_parent_folders(file_id)
        parent_folders_res = [folder.to_json() for folder in parent_folders]
        return get_json_result(data={"parent_folders": parent_folders_res})
    except Exception as e:
        return server_error_response(e)


@manager.route('/file/rm', methods=['POST']) # noqa: F821
@token_required
def rm(tenant_id):
    """
    Delete one or multiple files/folders.
    ---
    tags:
      - File Management
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        description: Files to delete
        required: true
        schema:
          type: object
          properties:
            file_ids:
              type: array
              items:
                type: string
              description: List of file IDs to delete
    responses:
      200:
        description: Successfully deleted files
        schema:
          type: object
          properties:
            data:
              type: boolean
              example: true
    """
    req = request.json
    file_ids = req["file_ids"]
    try:
        for file_id in file_ids:
            e, file = FileService.get_by_id(file_id)
            if not e:
                return get_json_result(message="File or Folder not found!", code=404)
            if not file.tenant_id:
                return get_json_result(message="Tenant not found!", code=404)

            if file.type == FileType.FOLDER.value:
                file_id_list = FileService.get_all_innermost_file_ids(file_id, [])
                for inner_file_id in file_id_list:
                    e, file = FileService.get_by_id(inner_file_id)
                    if not e:
                        return get_json_result(message="File not found!", code=404)
                    STORAGE_IMPL.rm(file.parent_id, file.location)
                FileService.delete_folder_by_pf_id(tenant_id, file_id)
            else:
                STORAGE_IMPL.rm(file.parent_id, file.location)
                if not FileService.delete(file):
                    return get_json_result(message="Database error (File removal)!", code=500)

            informs = File2DocumentService.get_by_file_id(file_id)
            for inform in informs:
                doc_id = inform.document_id
                e, doc = DocumentService.get_by_id(doc_id)
                if not e:
                    return get_json_result(message="Document not found!", code=404)
                tenant_id = DocumentService.get_tenant_id(doc_id)
                if not tenant_id:
                    return get_json_result(message="Tenant not found!", code=404)
                if not DocumentService.remove_document(doc, tenant_id):
                    return get_json_result(message="Database error (Document removal)!", code=500)
            File2DocumentService.delete_by_file_id(file_id)

        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/file/rename', methods=['POST']) # noqa: F821
@token_required
def rename(tenant_id):
    """
    Rename a file.
    ---
    tags:
      - File Management
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        description: Rename file
        required: true
        schema:
          type: object
          properties:
            file_id:
              type: string
              description: Target file ID
            name:
              type: string
              description: New name for the file
    responses:
      200:
        description: File renamed successfully
        schema:
          type: object
          properties:
            data:
              type: boolean
              example: true
    """
    req = request.json
    try:
        e, file = FileService.get_by_id(req["file_id"])
        if not e:
            return get_json_result(message="File not found!", code=404)

        if file.type != FileType.FOLDER.value and pathlib.Path(req["name"].lower()).suffix != pathlib.Path(file.name.lower()).suffix:
            return get_json_result(data=False, message="The extension of file can't be changed", code=400)

        for existing_file in FileService.query(name=req["name"], pf_id=file.parent_id):
            if existing_file.name == req["name"]:
                return get_json_result(data=False, message="Duplicated file name in the same folder.", code=409)

        if not FileService.update_by_id(req["file_id"], {"name": req["name"]}):
            return get_json_result(message="Database error (File rename)!", code=500)

        informs = File2DocumentService.get_by_file_id(req["file_id"])
        if informs:
            if not DocumentService.update_by_id(informs[0].document_id, {"name": req["name"]}):
                return get_json_result(message="Database error (Document rename)!", code=500)

        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/file/get/<file_id>', methods=['GET']) # noqa: F821
@token_required
def get(tenant_id,file_id):
    """
    Download a file.
    ---
    tags:
      - File Management
    security:
      - ApiKeyAuth: []
    produces:
      - application/octet-stream
    parameters:
      - in: path
        name: file_id
        type: string
        required: true
        description: File ID to download
    responses:
      200:
        description: File stream
        schema:
          type: file
      404:
        description: File not found
    """
    try:
        e, file = FileService.get_by_id(file_id)
        if not e:
            return get_json_result(message="Document not found!", code=404)

        blob = STORAGE_IMPL.get(file.parent_id, file.location)
        if not blob:
            b, n = File2DocumentService.get_storage_address(file_id=file_id)
            blob = STORAGE_IMPL.get(b, n)

        response = flask.make_response(blob)
        ext = re.search(r"\.([^.]+)$", file.name)
        if ext:
            if file.type == FileType.VISUAL.value:
                response.headers.set('Content-Type', 'image/%s' % ext.group(1))
            else:
                response.headers.set('Content-Type', 'application/%s' % ext.group(1))
        return response
    except Exception as e:
        return server_error_response(e)


@manager.route('/file/mv', methods=['POST']) # noqa: F821
@token_required
def move(tenant_id):
    """
    Move one or multiple files to another folder.
    ---
    tags:
      - File Management
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        description: Move operation
        required: true
        schema:
          type: object
          properties:
            src_file_ids:
              type: array
              items:
                type: string
              description: Source file IDs
            dest_file_id:
              type: string
              description: Destination folder ID
    responses:
      200:
        description: Files moved successfully
        schema:
          type: object
          properties:
            data:
              type: boolean
              example: true
    """
    req = request.json
    try:
        file_ids = req["src_file_ids"]
        parent_id = req["dest_file_id"]
        files = FileService.get_by_ids(file_ids)
        files_dict = {f.id: f for f in files}

        for file_id in file_ids:
            file = files_dict[file_id]
            if not file:
                return get_json_result(message="File or Folder not found!", code=404)
            if not file.tenant_id:
                return get_json_result(message="Tenant not found!", code=404)

        fe, _ = FileService.get_by_id(parent_id)
        if not fe:
            return get_json_result(message="Parent Folder not found!", code=404)

        FileService.move_file(file_ids, parent_id)
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)

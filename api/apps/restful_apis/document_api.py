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

from quart import request
from peewee import OperationalError
from pydantic import ValidationError

from api.apps.services.document_api_service import map_doc_keys, validate_document_update_fields, \
    update_document_name_only, update_chunk_method_only, update_document_status_only
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from common.constants import RetCode
from api.apps import login_required
from api.utils.api_utils import get_error_data_result, get_result, add_tenant_id_to_kwargs, get_request_json, \
    server_error_response
from api.utils.validation_utils import (
    UpdateDocumentReq, format_validation_error_message,
)

@manager.route("/datasets/<dataset_id>/documents/<document_id>", methods=["PATCH"]) # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def update_document(tenant_id, dataset_id, document_id):
    """
    Update a document within a dataset.
    ---
    tags:
      - Documents
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: dataset_id
        type: string
        required: true
        description: ID of the dataset.
      - in: path
        name: document_id
        type: string
        required: true
        description: ID of the document to update.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Document update parameters.
        required: true
        schema:
          type: object
          properties:
            name:
              type: string
              description: New name of the document.
            parser_config:
              type: object
              description: Parser configuration.
            chunk_method:
              type: string
              description: Chunking method.
            enabled:
              type: boolean
              description: Document status.
    responses:
      200:
        description: Document updated successfully.
        schema:
          type: object
    """
    req = await get_request_json()

    # Verify ownership and existence of dataset and document
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(message="You don't own the dataset.")
    e, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not e:
        return get_error_data_result(message="Can't find this dataset!")

    # Prepare data for validation
    docs = DocumentService.query(kb_id=dataset_id, id=document_id)
    if not docs:
        return get_error_data_result(message="The dataset doesn't own the document.")

    # Validate document update request parameters
    try:
        update_doc_req = UpdateDocumentReq(**req)
    except ValidationError as e:
        return get_error_data_result(message=format_validation_error_message(e), code=RetCode.DATA_ERROR)

    doc = docs[0]

    # further check with inner status (from DB)
    error_msg, error_code = validate_document_update_fields(update_doc_req, doc, req)
    if error_msg:
        return get_error_data_result(message=error_msg, code=error_code)

    # All validations passed, now perform all updates
    # meta_fields provided, then update it
    if "meta_fields" in req:
        if not DocMetadataService.update_document_metadata(document_id, update_doc_req.meta_fields):
            return get_error_data_result(message="Failed to update metadata")
    # doc name provided from request and diff with existing value, update
    if "name" in req and req["name"] != doc.name:
        if error := update_document_name_only(document_id, req["name"]):
            return error

    # parser config provided (already validated in UpdateDocumentReq), update it
    if update_doc_req.parser_config:
        DocumentService.update_parser_config(doc.id, req["parser_config"])

    # chunk method provided - the update method will check if it's different with existing one
    if update_doc_req.chunk_method:
        if error := update_chunk_method_only(req, doc, dataset_id, tenant_id):
            return error

    if "enabled" in req: # already checked in UpdateDocumentReq - it's int if it's present
        # "enabled" flag provided, the update method will check if it's changed and then update if so
        if error := update_document_status_only(int(req["enabled"]), doc, kb):
            return error

    try:
        original_doc_id = doc.id
        ok, doc = DocumentService.get_by_id(doc.id)
        if not ok:
            return get_error_data_result(message=f"Can not get document by id:{original_doc_id}")
    except OperationalError as e:
        logging.exception(e)
        return get_error_data_result(message="Database operation failed")
    renamed_doc = map_doc_keys(doc)
    return get_result(data=renamed_doc)


@manager.route("/datasets/<dataset_id>/metadata/summary", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def metadata_summary(dataset_id, tenant_id):
    """
    Get metadata summary for a dataset.
    ---
    tags:
      - Documents
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: dataset_id
        type: string
        required: true
        description: ID of the dataset.
      - in: query
        name: doc_ids
        type: string
        required: false
        description: Comma-separated document IDs to filter metadata.
    responses:
      200:
        description: Metadata summary retrieved successfully.
    """
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}. ")
    # Get doc_ids from query parameters (comma-separated string)
    doc_ids_param = request.args.get("doc_ids", "")
    doc_ids = doc_ids_param.split(",") if doc_ids_param else None
    try:
        summary = DocMetadataService.get_metadata_summary(dataset_id, doc_ids)
        return get_result(data={"summary": summary})
    except Exception as e:
        return server_error_response(e)


@manager.route("/datasets/<dataset_id>/documents", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def upload_document(dataset_id, tenant_id):
    """
    Upload documents to a dataset.
    ---
    tags:
      - Documents
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: dataset_id
        type: string
        required: true
        description: ID of the dataset.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: formData
        name: file
        type: file
        required: true
        description: Document files to upload.
      - in: formData
        name: parent_path
        type: string
        description: Optional nested path under the parent folder. Uses '/' separators.
      - in: query
        name: return_raw_files
        type: boolean
        required: false
        default: false
        description: Whether to skip document key mapping and return raw document data
    responses:
      200:
        description: Successfully uploaded documents.
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
                    description: Document ID.
                  name:
                    type: string
                    description: Document name.
                  chunk_count:
                    type: integer
                    description: Number of chunks.
                  token_count:
                    type: integer
                    description: Number of tokens.
                  dataset_id:
                    type: string
                    description: ID of the dataset.
                  chunk_method:
                    type: string
                    description: Chunking method used.
                  run:
                    type: string
                    description: Processing status.
    """
    from api.constants import FILE_NAME_LEN_LIMIT
    from api.common.check_team_permission import check_kb_team_permission
    from api.db.services.file_service import FileService
    from common.misc_utils import thread_pool_exec
    
    form = await request.form
    files = await request.files
    
    # Validation
    if "file" not in files:
        return get_error_data_result(message="No file part!", code=RetCode.ARGUMENT_ERROR)
    
    file_objs = files.getlist("file")
    for file_obj in file_objs:
        if file_obj.filename == "":
            return get_error_data_result(message="No file selected!", code=RetCode.ARGUMENT_ERROR)
        if len(file_obj.filename.encode("utf-8")) > FILE_NAME_LEN_LIMIT:
            return get_error_data_result(
                message=f"File name must be {FILE_NAME_LEN_LIMIT} bytes or less.", 
                code=RetCode.ARGUMENT_ERROR
            )

    # KB Lookup
    e, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not e:
        return server_error_response(LookupError(f"Can't find the dataset with ID {dataset_id}!"))
    
    # Permission Check
    if not check_kb_team_permission(kb, tenant_id):
        return get_error_data_result(message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)

    # File Upload (async)
    err, files = await thread_pool_exec(
        FileService.upload_document, kb, file_objs, tenant_id,
        parent_path=form.get("parent_path")
    )
    if err:
        return get_error_data_result(message="\n".join(err), code=RetCode.SERVER_ERROR)

    if not files:
        return get_error_data_result(
            message="There seems to be an issue with your file format. please verify it is correct and not corrupted.", 
            code=RetCode.DATA_ERROR
        )
    
    files = [f[0] for f in files]  # remove the blob

    # Check if we should return raw files without document key mapping
    return_raw_files = request.args.get("return_raw_files", "false").lower() == "true"

    if return_raw_files:
        return get_result(data=files)

    renamed_doc_list = [map_doc_keys(doc, run_value="UNSTART") for doc in files]
    return get_result(data=renamed_doc_list)



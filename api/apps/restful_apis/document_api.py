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
import json

from quart import request
from peewee import OperationalError
from pydantic import ValidationError

from api.apps import login_required
from api.apps.services.document_api_service import validate_document_update_fields, map_doc_keys, \
    map_doc_keys_with_run_status, update_document_name_only, update_chunk_method_only, update_document_status_only
from api.constants import IMG_BASE64_PREFIX
from api.db import VALID_FILE_TYPES
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils.api_utils import get_data_error_result, get_error_data_result, get_result, get_json_result, \
    server_error_response, add_tenant_id_to_kwargs, get_request_json
from api.utils.validation_utils import (
    UpdateDocumentReq, format_validation_error_message,
)
from common.constants import RetCode
from common.metadata_utils import convert_conditions, meta_filter, turn2jsonschema

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
        logging.error("No file part!")
        return get_error_data_result(message="No file part!", code=RetCode.ARGUMENT_ERROR)
    
    file_objs = files.getlist("file")
    for file_obj in file_objs:
        if file_obj is None or file_obj.filename is None or file_obj.filename == "":
            logging.error("No file selected!")
            return get_error_data_result(message="No file selected!", code=RetCode.ARGUMENT_ERROR)
        if len(file_obj.filename.encode("utf-8")) > FILE_NAME_LEN_LIMIT:
            msg = f"File name must be {FILE_NAME_LEN_LIMIT} bytes or less."
            logging.error(msg)
            return get_error_data_result(message=msg, code=RetCode.ARGUMENT_ERROR)

    # KB Lookup
    e, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not e:
        logging.error(f"Can't find the dataset with ID {dataset_id}!")
        return get_error_data_result(message=f"Can't find the dataset with ID {dataset_id}!", code=RetCode.DATA_ERROR)
    
    # Permission Check
    if not check_kb_team_permission(kb, tenant_id):
        logging.error("No authorization.")
        return get_error_data_result(message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)

    # File Upload (async)
    err, files = await thread_pool_exec(
        FileService.upload_document, kb, file_objs, tenant_id,
        parent_path=form.get("parent_path")
    )
    if err:
        msg = "\n".join(err)
        logging.error(msg)
        return get_error_data_result(message=msg, code=RetCode.SERVER_ERROR)

    if not files:
        msg = "There seems to be an issue with your file format. please verify it is correct and not corrupted."
        logging.error(msg)
        return get_error_data_result(message=msg, code=RetCode.DATA_ERROR)
    
    files = [f[0] for f in files]  # remove the blob

    # Check if we should return raw files without document key mapping
    return_raw_files = request.args.get("return_raw_files", "false").lower() == "true"

    if return_raw_files:
        return get_result(data=files)

    renamed_doc_list = [map_doc_keys_with_run_status(doc, run_status="0") for doc in files]
    return get_result(data=renamed_doc_list)


@manager.route("/datasets/<dataset_id>/documents", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def list_docs(dataset_id, tenant_id):
    """
    List documents in a dataset.
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
        name: page
        type: integer
        required: false
        default: 1
        description: Page number.
      - in: query
        name: page_size
        type: integer
        required: false
        default: 30
        description: Number of items per page.
      - in: query
        name: orderby
        type: string
        required: false
        default: "create_time"
        description: Field to order by.
      - in: query
        name: desc
        type: boolean
        required: false
        default: true
        description: Order in descending.
      - in: query
        name: create_time_from
        type: integer
        required: false
        default: 0
        description: Unix timestamp for filtering documents created after this time. 0 means no filter.
      - in: query
        name: create_time_to
        type: integer
        required: false
        default: 0
        description: Unix timestamp for filtering documents created before this time. 0 means no filter.
      - in: query
        name: suffix
        type: array
        items:
          type: string
        required: false
        description: Filter by file suffix (e.g., ["pdf", "txt", "docx"]).
      - in: query
        name: run
        type: array
        items:
          type: string
        required: false
        description: Filter by document run status. Supports both numeric ("0", "1", "2", "3", "4") and text formats ("UNSTART", "RUNNING", "CANCEL", "DONE", "FAIL").
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: List of documents.
        schema:
          type: object
          properties:
            total:
              type: integer
              description: Total number of documents.
            docs:
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
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        logging.error(f"You don't own the dataset {dataset_id}. ")
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}. ")

    q = request.args

    page = int(q.get("page", 1))
    page_size = int(q.get("page_size", 30))

    print(f"page:{page}, page size:{page_size}")

    orderby = q.get("orderby", "create_time")
    desc = str(q.get("desc", "true")).strip().lower() != "false"
    keywords = q.get("keywords", "")

    # filters - align with OpenAPI parameter names
    suffix = q.getlist("suffix")

    types = q.getlist("types", [])
    if types:
        invalid_types = {t for t in types if t not in VALID_FILE_TYPES}
        if invalid_types:
            logging.error(msg=f"------------Invalid filter conditions: {', '.join(invalid_types)} type{'s' if len(invalid_types) > 1 else ''}")
            return get_data_error_result(message=f"Invalid filter conditions: {', '.join(invalid_types)} type{'s' if len(invalid_types) > 1 else ''}")

    # map run status (text or numeric) - align with API parameter
    run_status = q.getlist("run")
    run_status_text_to_numeric = {"UNSTART": "0", "RUNNING": "1", "CANCEL": "2", "DONE": "3", "FAIL": "4"}
    run_status_converted = [run_status_text_to_numeric.get(v, v) for v in run_status]
    if run_status_converted:
        invalid_status = {s for s in run_status_converted if s not in run_status_text_to_numeric.values()}
        if invalid_status:
            logging.error(msg=f"----------Invalid filter run status conditions: {', '.join(invalid_status)}")
            return get_data_error_result(message=f"Invalid filter run status conditions: {', '.join(invalid_status)}")

    doc_ids_filter, empty_result_or_error, return_empty_metadata = _parse_doc_id_filter_with_metadata(q, dataset_id)
    if empty_result_or_error is not None:
        logging.error(msg=f"----------empty_result_or_error: {empty_result_or_error}")
        return empty_result_or_error

    docs, total = DocumentService.get_by_kb_id(dataset_id, page, page_size, orderby, desc, keywords, run_status_converted, types, suffix, doc_ids_filter, return_empty_metadata=return_empty_metadata)

    # time range filter (0 means no bound)
    create_time_from = int(q.get("create_time_from", 0))
    create_time_to = int(q.get("create_time_to", 0))
    if create_time_from or create_time_to:
        docs = [d for d in docs if (create_time_from == 0 or d.get("create_time", 0) >= create_time_from) and (create_time_to == 0 or d.get("create_time", 0) <= create_time_to)]

    renamed_doc_list = [map_doc_keys(doc) for doc in docs]
    for doc_item in renamed_doc_list:
        if doc_item["thumbnail"] and not doc_item["thumbnail"].startswith(IMG_BASE64_PREFIX):
            doc_item["thumbnail"] = f"/v1/document/image/{dataset_id}-{doc_item['thumbnail']}"
        if doc_item.get("source_type"):
            doc_item["source_type"] = doc_item["source_type"].split("/")[0]
        if doc_item["parser_config"].get("metadata"):
            doc_item["parser_config"]["metadata"] = turn2jsonschema(doc_item["parser_config"]["metadata"])

    return get_result(data={"total": total, "docs": renamed_doc_list})

def _parse_doc_id_filter_with_metadata(req, kb_id):
    return_empty_metadata = req.get("return_empty_metadata", False)
    if isinstance(return_empty_metadata, str):
        return_empty_metadata = return_empty_metadata.lower() == "true"

    try:
        metadata_condition = json.loads(req.get("metadata_condition", "{}"))
    except json.JSONDecodeError:
        logging.error(msg=f'----------metadata_condition must be valid JSON: {req.get("metadata_condition")}.')
        return None, get_data_error_result(message="metadata_condition must be valid JSON."), return_empty_metadata
    try:
        metadata = json.loads(req.get("metadata", "{}"))
    except json.JSONDecodeError:
        logging.error(msg=f'------------metadata must be valid JSON: {req.get("metadata")}.')
        return None, get_data_error_result(message="metadata must be valid JSON."), return_empty_metadata

    if isinstance(metadata, dict) and metadata.get("empty_metadata"):
        return_empty_metadata = True
        metadata = {k: v for k, v in metadata.items() if k != "empty_metadata"}
    if return_empty_metadata:
        metadata_condition = {}
        metadata = {}
    else:
        if metadata_condition and not isinstance(metadata_condition, dict):
            return None, get_data_error_result(message="metadata_condition must be an object."), return_empty_metadata
        if metadata and not isinstance(metadata, dict):
            return None, get_data_error_result(message="metadata must be an object."), return_empty_metadata

    doc_ids_filter = None
    metas = None
    if metadata_condition or metadata:
        metas = DocMetadataService.get_flatted_meta_by_kbs([kb_id])

    if metadata_condition:
        doc_ids_filter = set(meta_filter(metas, convert_conditions(metadata_condition), metadata_condition.get("logic", "and")))
        if metadata_condition.get("conditions") and not doc_ids_filter:
            return None, get_json_result(data={"total": 0, "docs": []}), return_empty_metadata

    if metadata:
        metadata_doc_ids = None
        for key, values in metadata.items():
            if not values:
                continue
            if not isinstance(values, list):
                values = [values]
            values = [str(v) for v in values if v is not None and str(v).strip()]
            if not values:
                continue
            key_doc_ids = set()
            for value in values:
                key_doc_ids.update(metas.get(key, {}).get(value, []))
            if metadata_doc_ids is None:
                metadata_doc_ids = key_doc_ids
            else:
                metadata_doc_ids &= key_doc_ids
            if not metadata_doc_ids:
                return None, get_json_result(data={"total": 0, "docs": []}), return_empty_metadata
        if metadata_doc_ids is not None:
            if doc_ids_filter is None:
                doc_ids_filter = metadata_doc_ids
            else:
                doc_ids_filter &= metadata_doc_ids
            if not doc_ids_filter:
                return None, get_json_result(data={"total": 0, "docs": []}), return_empty_metadata

    return list(doc_ids_filter) if doc_ids_filter is not None else list(), None, return_empty_metadata

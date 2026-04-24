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
import os.path
import re

from quart import make_response, request
from peewee import OperationalError
from pydantic import ValidationError

from api.apps import login_required
from api.apps.services.document_api_service import validate_document_update_fields, map_doc_keys, \
    map_doc_keys_with_run_status, update_document_name_only, update_chunk_method_only, update_document_status_only
from api.constants import IMG_BASE64_PREFIX
from api.db import VALID_FILE_TYPES
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.document_service import DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.common.check_team_permission import check_kb_team_permission
from api.utils.api_utils import get_data_error_result, get_error_data_result, get_result, get_json_result, \
    server_error_response, add_tenant_id_to_kwargs, get_request_json, get_error_argument_result, check_duplicate_ids
from api.utils.validation_utils import (
    UpdateDocumentReq, format_validation_error_message, validate_and_parse_json_request, DeleteDocumentReq,
)
from common.constants import RetCode, SANDBOX_ARTIFACT_BUCKET
from common.metadata_utils import convert_conditions, meta_filter, turn2jsonschema
from common.misc_utils import thread_pool_exec
from api.utils.web_utils import apply_safe_file_response_headers

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
    from api.db.services.file_service import FileService

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

    err_code, err_msg, docs, total = _get_docs_with_request(request, dataset_id)
    if err_code != RetCode.SUCCESS:
        return get_data_error_result(code=err_code, message=err_msg)

    if request.args.get("type") == "filter":
        docs_filter = _aggregate_filters(docs)
        return get_json_result(data={"total": total, "filter": docs_filter})
    else:
        renamed_doc_list = [map_doc_keys(doc) for doc in docs]
        for doc_item in renamed_doc_list:
            if doc_item["thumbnail"] and not doc_item["thumbnail"].startswith(IMG_BASE64_PREFIX):
                doc_item["thumbnail"] = f"/v1/document/image/{dataset_id}-{doc_item['thumbnail']}"
            if doc_item.get("source_type"):
                doc_item["source_type"] = doc_item["source_type"].split("/")[0]
            if doc_item["parser_config"].get("metadata"):
                doc_item["parser_config"]["metadata"] = turn2jsonschema(doc_item["parser_config"]["metadata"])
        return get_json_result(data={"total": total, "docs": renamed_doc_list})


def _get_docs_with_request(req, dataset_id:str):
    """Get documents with request parameters from a dataset.

    This function extracts filtering parameters from the request and returns
    a list of documents matching the specified criteria.

    Args:
        req: The request object containing query parameters.
            - page (int): Page number for pagination (default: 1).
            - page_size (int): Number of documents per page (default: 30).
            - orderby (str): Field to order by (default: "create_time").
            - desc (bool): Whether to order in descending order (default: True).
            - keywords (str): Keywords to search in document names.
            - suffix (list): File suffix filters.
            - types (list): Document type filters.
            - run (list): Processing status filters.
            - create_time_from (int): Start timestamp for time range filter.
            - create_time_to (int): End timestamp for time range filter.
            - return_empty_metadata (bool|str): Whether to return documents with empty metadata.
            - metadata_condition (str): JSON string for complex metadata conditions.
            - metadata (str): JSON string for simple metadata key-value matching.
        dataset_id: The dataset ID to retrieve documents from.

    Returns:
        A tuple of (err_code, err_message, docs, total):
            - err_code (int): Success code (RetCode.SUCCESS) if successful, or error code if validation fails.
            - err_message (str): Empty string if successful, or error message if validation fails.
            - docs (list): List of document dictionaries matching the criteria, or empty list on error.
            - total (int): Total number of documents matching the criteria.

    Note:
        - The function supports filtering by document types, processing status, keywords, and time range.
        - Metadata filtering supports both simple key-value matching and complex conditions with operators.
    """
    q = req.args

    page = int(q.get("page", 1))
    page_size = int(q.get("page_size", 30))

    orderby = q.get("orderby", "create_time")
    desc = str(q.get("desc", "true")).strip().lower() != "false"
    keywords = q.get("keywords", "")

    # filters - align with OpenAPI parameter names
    suffix = q.getlist("suffix")

    types = q.getlist("types")
    if types:
        invalid_types = {t for t in types if t not in VALID_FILE_TYPES}
        if invalid_types:
            msg = f"Invalid filter conditions: {', '.join(invalid_types)} type{'s' if len(invalid_types) > 1 else ''}"
            return RetCode.DATA_ERROR, msg, [], 0

    # map run status (text or numeric) - align with API parameter
    run_status = q.getlist("run")
    run_status_text_to_numeric = {"UNSTART": "0", "RUNNING": "1", "CANCEL": "2", "DONE": "3", "FAIL": "4"}
    run_status_converted = [run_status_text_to_numeric.get(v, v) for v in run_status]
    if run_status_converted:
        invalid_status = {s for s in run_status_converted if s not in run_status_text_to_numeric.values()}
        if invalid_status:
            msg = f"Invalid filter run status conditions: {', '.join(invalid_status)}"
            return RetCode.DATA_ERROR, msg, [], 0

    err_code, err_message, doc_ids_filter, return_empty_metadata = _parse_doc_id_filter_with_metadata(q, dataset_id)
    if err_code != RetCode.SUCCESS:
        return err_code, err_message, [], 0

    doc_name = q.get("name")
    doc_id = q.get("id")
    if doc_id:
        if not DocumentService.query(id=doc_id, kb_id=dataset_id):
            return RetCode.DATA_ERROR, f"You don't own the document {doc_id}.", [], 0
        doc_ids_filter = [doc_id] # id provided, ignore other filters
    if doc_name and not DocumentService.query(name=doc_name, kb_id=dataset_id):
        return RetCode.DATA_ERROR, f"You don't own the document {doc_name}.", [], 0

    doc_ids = q.getlist("ids")
    if doc_id and len(doc_ids) > 0:
        return RetCode.DATA_ERROR, f"Should not provide both 'id':{doc_id} and 'ids'{doc_ids}"
    if len(doc_ids) > 0:
        doc_ids_filter = doc_ids

    docs, total = DocumentService.get_by_kb_id(dataset_id, page, page_size, orderby, desc, keywords, run_status_converted, types, suffix,
                                               name=doc_name, doc_ids=doc_ids_filter, return_empty_metadata=return_empty_metadata)

    # time range filter (0 means no bound)
    create_time_from = int(q.get("create_time_from", 0))
    create_time_to = int(q.get("create_time_to", 0))
    if create_time_from or create_time_to:
        docs = [d for d in docs if (create_time_from == 0 or d.get("create_time", 0) >= create_time_from) and (create_time_to == 0 or d.get("create_time", 0) <= create_time_to)]

    return RetCode.SUCCESS, "", docs, total

def _parse_doc_id_filter_with_metadata(req, kb_id):
    """Parse document ID filter based on metadata conditions from the request.

    This function extracts and processes metadata filtering parameters from the request
    and returns a list of document IDs that match the specified criteria. It supports
    two filtering modes: simple metadata key-value matching and complex metadata
    conditions with operators.

    Args:
        req: The request object containing filtering parameters.
            - return_empty_metadata (bool|str): If True, returns all documents regardless
              of their metadata. Can be a boolean or string "true"/"false".
            - metadata_condition (str): JSON string containing complex metadata conditions
              with optional "logic" (and/or) and "conditions" list. Each condition should
              have "name" (key), "comparison_operator", and "value" fields.
            - metadata (str): JSON string containing key-value pairs for exact metadata
              matching. Values can be a single value or list of values (OR logic within
              same key). Can include special key "empty_metadata" to indicate documents
              with empty metadata.
        kb_id: The knowledge base ID to filter documents from.

    Returns:
        A tuple of (err_code, err_message, docs, return_empty_metadata):
            - err_code (int): Success code (RetCode.SUCCESS) if successful, or error code if validation fails.
            - err_message (str): Empty string if successful, or error message if validation fails.
            - docs (list): List of document IDs matching the metadata criteria,
              or empty list if no filter should be applied or on error.
            - return_empty_metadata (bool): The processed flag indicating whether to
              return documents with empty metadata.

    Note:
        - When both metadata and metadata_condition are provided, they are combined with AND logic.
        - The metadata_condition uses operators like: =, !=, >, <, >=, <=, contains, not contains,
          in, not in, start with, end with, empty, not empty.
        - The metadata parameter performs exact matching where values are OR'd within the same key
          & AND'd across different keys.

    Examples:
        Simple metadata filter (exact match):
            req = {"metadata": '{"author": ["John", "Jane"]}'}
            # Returns documents where author is John OR Jane

        Simple metadata filter with multiple keys:
            req = {"metadata": '{"author": "John", "status": "published"}'}
            # Returns documents where author is John AND status is published

        Complex metadata conditions:
            req = {"metadata_condition": '{"logic": "and", "conditions": [{"name": "status", "comparison_operator": "eq", "value": "published"}]}'}
            # Returns documents where status equals "published"

        Complex conditions with multiple operators:
            req = {"metadata_condition": '{"logic": "or", "conditions": [{"name": "priority", "comparison_operator": "=", "value": "high"}, {"name": "status", "comparison_operator": "contains", "value": "urgent"}]}'}
            # Returns documents where priority is high OR status contains "urgent"

        Return empty metadata:
            req = {"return_empty_metadata": True}
            # Returns all documents regardless of metadata

        Combined metadata and metadata_condition:
            req = {"metadata": '{"author": "John"}', "metadata_condition": '{"logic": "and", "conditions": [{"name": "status", "comparison_operator": "=", "value": "published"}]}'}
            # Returns documents where author is John AND status equals published
    """
    return_empty_metadata = req.get("return_empty_metadata", False)
    if isinstance(return_empty_metadata, str):
        return_empty_metadata = return_empty_metadata.lower() == "true"

    try:
        metadata_condition = json.loads(req.get("metadata_condition", "{}"))
    except json.JSONDecodeError:
        msg = f'metadata_condition must be valid JSON: {req.get("metadata_condition")}.'
        return RetCode.DATA_ERROR, msg, [], return_empty_metadata
    try:
        metadata = json.loads(req.get("metadata", "{}"))
    except json.JSONDecodeError:
        logging.error(msg=f'metadata must be valid JSON: {req.get("metadata")}.')
        return RetCode.DATA_ERROR, "metadata must be valid JSON.", [], return_empty_metadata

    if isinstance(metadata, dict) and metadata.get("empty_metadata"):
        return_empty_metadata = True
        metadata = {k: v for k, v in metadata.items() if k != "empty_metadata"}
    if return_empty_metadata:
        metadata_condition = {}
        metadata = {}
    else:
        if metadata_condition and not isinstance(metadata_condition, dict):
            return RetCode.DATA_ERROR, "metadata_condition must be an object.", [], return_empty_metadata
        if metadata and not isinstance(metadata, dict):
            return RetCode.DATA_ERROR, "metadata must be an object.", [], return_empty_metadata

    metas = dict()
    if metadata_condition or metadata:
        metas = DocMetadataService.get_flatted_meta_by_kbs([kb_id])

    doc_ids_filter = None
    if metadata_condition:
        doc_ids_filter = set(meta_filter(metas, convert_conditions(metadata_condition), metadata_condition.get("logic", "and")))
        if metadata_condition.get("conditions") and not doc_ids_filter:
            return RetCode.SUCCESS, "", [], return_empty_metadata

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
                return RetCode.SUCCESS, "", [], return_empty_metadata

        if metadata_doc_ids is not None:
            if doc_ids_filter is None:
                doc_ids_filter = metadata_doc_ids
            else:
                doc_ids_filter &= metadata_doc_ids
            if not doc_ids_filter:
                return RetCode.SUCCESS, "", [], return_empty_metadata

    return RetCode.SUCCESS, "", list(doc_ids_filter) if doc_ids_filter is not None else [], return_empty_metadata


@manager.route("/datasets/<dataset_id>/documents", methods=["DELETE"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def delete_documents(tenant_id, dataset_id):
    """
    Delete documents from a dataset.
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
        description: ID of the dataset containing the documents.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Document deletion parameters.
        required: true
        schema:
          type: object
          properties:
            ids:
              type: array or null
              items:
                type: string
              description: |
                Specifies the documents to delete:
                - An array of IDs, only the specified documents will be deleted.
            delete_all:
              type: boolean
              default: false
              description: Whether to delete all documents in the dataset.
    responses:
      200:
        description: Successful operation.
        schema:
          type: object
    """
    req, err = await validate_and_parse_json_request(request, DeleteDocumentReq)
    if err is not None or req is None:
        return get_error_argument_result(err)

    try:
        # Validate dataset exists and user has permission
        if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
            return get_error_data_result(message=f"You don't own the dataset {dataset_id}. ")

        # Get documents to delete
        doc_ids = req.get("ids") or []
        delete_all = req.get("delete_all", False)
        if not delete_all and len(doc_ids) == 0:
            return get_error_data_result(message=f"should either provide doc ids or set delete_all(true), dataset: {dataset_id}. ")

        if len(doc_ids) > 0 and delete_all:
            return get_error_data_result(message=f"should not provide both doc ids and delete_all(true), dataset: {dataset_id}. ")
        if delete_all:
            doc_ids = [doc.id for doc in DocumentService.query(kb_id=dataset_id)]

        # make sure each id is unique
        unique_doc_ids, duplicate_messages = check_duplicate_ids(doc_ids, "document")
        if duplicate_messages:
            logging.warning(f"duplicate_messages:{duplicate_messages}")
        else:
            doc_ids = unique_doc_ids

        # Delete documents using existing FileService.delete_docs
        errors = await thread_pool_exec(FileService.delete_docs, doc_ids, tenant_id)

        if errors:
            return get_error_data_result(message=str(errors))

        return get_result(data={"deleted": len(doc_ids)})
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


def _aggregate_filters(docs):
    """Aggregate filter options from a list of documents.

    This function processes a list of document dictionaries and aggregates
    available filter values for building filter UI on the client side.

    Args:
        docs (list): List of document dictionaries, each containing:
            - id (str): Document ID
            - suffix (str): File extension (e.g., "pdf", "docx")
            - run (int): Parsing status code (0=UNSTART, 1=RUNNING, 2=CANCEL, 3=DONE, 4=FAIL)

    Returns:
        tuple: A tuple containing:
            - dict: Aggregated filter options with keys:
                - suffix: Dict mapping file extensions to document counts
                - run_status: Dict mapping status codes to document counts
                - metadata: Dict mapping metadata field names to value counts
            - int: Total number of documents processed
    """
    suffix_counter = {}
    run_status_counter = {}
    metadata_counter = {}
    empty_metadata_count = 0

    for doc in docs:
        suffix_counter[doc.get("suffix")] = suffix_counter.get(doc.get("suffix"), 0) + 1
        key_of_run = str(doc.get("run"))
        run_status_counter[key_of_run] = run_status_counter.get(key_of_run, 0) + 1
        meta_fields = doc.get("meta_fields", {})

        if not meta_fields:
            empty_metadata_count += 1
            continue
        has_valid_meta = False

        for key, value in meta_fields.items():
            values = value if isinstance(value, list) else [value]
            for vv in values:
                if vv is None:
                    continue
                if isinstance(vv, str) and not vv.strip():
                    continue
                sv = str(vv)
                if key not in metadata_counter:
                    metadata_counter[key] = {}
                metadata_counter[key][sv] = metadata_counter[key].get(sv, 0) + 1
                has_valid_meta = True
        if not has_valid_meta:
            empty_metadata_count += 1

    metadata_counter["empty_metadata"] = {"true": empty_metadata_count}
    return {
        "suffix": suffix_counter,
        "run_status": run_status_counter,
        "metadata": metadata_counter,
    }

@manager.route("/datasets/<dataset_id>/documents/<document_id>/metadata/config", methods=["PUT"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def update_metadata_config(tenant_id, dataset_id, document_id):
    """
    Update document metadata configuration.
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
        description: ID of the document.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Metadata configuration.
        required: true
        schema:
          type: object
          properties:
            metadata:
              type: object
              description: Metadata configuration JSON.
    responses:
      200:
        description: Document updated successfully.
    """
    # Verify ownership and existence of dataset
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(message="You don't own the dataset.")

    # Verify document exists in the dataset
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        msg = f"Document {document_id} not found in dataset {dataset_id}"
        return get_error_data_result(message=msg)
    doc = doc[0]

    # Get request body
    req = await get_request_json()
    if "metadata" not in req:
        return get_error_argument_result(message="metadata is required")

    # Update parser config with metadata
    try:
        DocumentService.update_parser_config(doc.id, {"metadata": req["metadata"]})
    except Exception as e:
        logging.error("error when update_parser_config", exc_info=e)
        return get_json_result(code=RetCode.EXCEPTION_ERROR, message=repr(e))

    # Get updated document
    try:
        e, doc = DocumentService.get_by_id(doc.id)
        if not e:
            return get_data_error_result(message="Document not found!")
    except Exception as e:
        return get_json_result(code=RetCode.EXCEPTION_ERROR, message=repr(e))

    return get_result(data=doc.to_dict())


@manager.route("/datasets/<dataset_id>/documents/metadatas", methods=["PATCH"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def update_metadata(tenant_id, dataset_id):
    """
    Update document metadata in batch.
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
      - in: body
        name: body
        description: Metadata update request.
        required: true
        schema:
          type: object
          properties:
            selector:
              type: object
              description: Document selector.
              properties:
                document_ids:
                  type: array
                  items:
                    type: string
                  description: List of document IDs to update.
                metadata_condition:
                  type: object
                  description: Filter documents by existing metadata.
            updates:
              type: array
              items:
                type: object
                properties:
                  key:
                    type: string
                  value:
                    type: any
              description: List of metadata key-value pairs to update.
            deletes:
              type: array
              items:
                type: object
                properties:
                  key:
                    type: string
              description: List of metadata keys to delete.
    responses:
      200:
        description: Metadata updated successfully.
    """
    # Verify ownership of dataset
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")

    # Get request body
    req = await get_request_json()
    selector = req.get("selector", {}) or {}
    updates = req.get("updates", []) or []
    deletes = req.get("deletes", []) or []

    # Validate selector
    if not isinstance(selector, dict):
        return get_error_data_result(message="selector must be an object.")
    if not isinstance(updates, list) or not isinstance(deletes, list):
        return get_error_data_result(message="updates and deletes must be lists.")

    # Validate metadata_condition
    metadata_condition = selector.get("metadata_condition", {}) or {}
    if metadata_condition and not isinstance(metadata_condition, dict):
        return get_error_data_result(message="metadata_condition must be an object.")

    # Validate document_ids
    document_ids = selector.get("document_ids", []) or []
    if document_ids and not isinstance(document_ids, list):
        return get_error_data_result(message="document_ids must be a list.")

    # Validate updates
    for upd in updates:
        if not isinstance(upd, dict) or not upd.get("key") or "value" not in upd:
            return get_error_data_result(message="Each update requires key and value.")

    # Validate deletes
    for d in deletes:
        if not isinstance(d, dict) or not d.get("key"):
            return get_error_data_result(message="Each delete requires key.")

    # Initialize target document IDs
    target_doc_ids = set()

    # If document_ids provided, validate they belong to the dataset
    if document_ids:
        kb_doc_ids = KnowledgebaseService.list_documents_by_ids([dataset_id])
        invalid_ids = set(document_ids) - set(kb_doc_ids)
        if invalid_ids:
            return get_error_data_result(
                message=f"These documents do not belong to dataset {dataset_id}: {', '.join(invalid_ids)}"
            )
        target_doc_ids = set(document_ids)

    # Apply metadata_condition filtering if provided
    if metadata_condition:
        metas = DocMetadataService.get_flatted_meta_by_kbs([dataset_id])
        filtered_ids = set(
            meta_filter(metas, convert_conditions(metadata_condition), metadata_condition.get("logic", "and"))
        )
        target_doc_ids = target_doc_ids & filtered_ids
        if metadata_condition.get("conditions") and not target_doc_ids:
            return get_result(data={"updated": 0, "matched_docs": 0})

    # Convert to list and perform update
    target_doc_ids = list(target_doc_ids)
    updated = DocMetadataService.batch_update_metadata(dataset_id, target_doc_ids, updates, deletes)
    return get_result(data={"updated": updated, "matched_docs": len(target_doc_ids)})


ARTIFACT_CONTENT_TYPES = {
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".svg": "image/svg+xml",
    ".pdf": "application/pdf",
    ".csv": "text/csv",
    ".json": "application/json",
    ".html": "text/html",
}


@manager.route("/documents/artifact/<filename>", methods=["GET"])  # noqa: F821
@login_required
async def get_artifact(filename):
    """
    Get an artifact file.
    ---
    tags:
      - Documents
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: filename
        type: string
        required: true
        description: Name of the artifact file.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Artifact file returned successfully.
    """
    from common import settings

    try:
        bucket = SANDBOX_ARTIFACT_BUCKET
        # Validate filename: must be uuid hex + allowed extension, nothing else
        basename = os.path.basename(filename)
        if basename != filename or "/" in filename or "\\" in filename:
            return get_data_error_result(message="Invalid filename.")
        ext = os.path.splitext(basename)[1].lower()
        if ext not in ARTIFACT_CONTENT_TYPES:
            return get_data_error_result(message="Invalid file type.")
        data = await thread_pool_exec(settings.STORAGE_IMPL.get, bucket, basename)
        if not data:
            return get_data_error_result(message="Artifact not found.")
        content_type = ARTIFACT_CONTENT_TYPES.get(ext, "application/octet-stream")
        response = await make_response(data)
        safe_filename = re.sub(r"[^\w.\-]", "_", basename)
        apply_safe_file_response_headers(response, content_type, ext)
        if not response.headers.get("Content-Disposition"):
            response.headers.set("Content-Disposition", f'inline; filename="{safe_filename}"')
        return response
    except Exception as e:
        return server_error_response(e)

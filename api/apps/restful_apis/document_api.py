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

from peewee import OperationalError
from pydantic import ValidationError

from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils import validation_utils
from common import settings
from common.constants import RetCode, TaskStatus
from api.apps import login_required
from api.utils.api_utils import get_error_data_result, get_result, add_tenant_id_to_kwargs, \
    get_request_json, server_error_response, get_parser_config
from api.utils.validation_utils import (
    UpdateDocumentReq, format_validation_error_message,
)
from rag.nlp import rag_tokenizer, search


@manager.route("/datasets/<dataset_id>/documents/<document_id>", methods=["PUT"]) # noqa: F821
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
    if update_doc_req.meta_fields:
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
        ok, doc = DocumentService.get_by_id(doc.id)
        if not ok:
            return get_error_data_result(message=f"Can not get document by id:{doc.id}")
    except OperationalError as e:
        logging.exception(e)
        return get_error_data_result(message="Database operation failed")
    renamed_doc = rename_doc(doc)
    return get_result(data=renamed_doc)


def update_document_name_only(document_id, req_doc_name):
    """Update document name only (without validation)."""
    if not DocumentService.update_by_id(document_id, {"name": req_doc_name}):
        return get_error_data_result(message="Database error (Document rename)!")

    informs = File2DocumentService.get_by_document_id(document_id)
    if informs:
        e, file = FileService.get_by_id(informs[0].file_id)
        FileService.update_by_id(file.id, {"name": req_doc_name})
    # Add logic to update index - refer to rename method in document_app.py
    tenant_id = DocumentService.get_tenant_id(document_id)
    title_tks = rag_tokenizer.tokenize(req_doc_name)
    es_body = {
        "docnm_kwd": req_doc_name,
        "title_tks": title_tks,
        "title_sm_tks": rag_tokenizer.fine_grained_tokenize(title_tks),
    }
    ok, doc = DocumentService.get_by_id(document_id)
    if not ok:
        return get_error_data_result(message=f"Not able to find document by id:{document_id}")
    if settings.docStoreConn.index_exist(search.index_name(tenant_id), doc.kb_id):
        settings.docStoreConn.update(
            {"doc_id": document_id},
            es_body,
            search.index_name(tenant_id),
            doc.kb_id,
        )
    return None

def update_chunk_method_only(req, doc, dataset_id, tenant_id):
    """Update chunk method only (without validation)."""
    if doc.parser_id.lower() != req["chunk_method"].lower():
        # if chunk method changed
        e = DocumentService.update_by_id(
            doc.id,
            {
                "parser_id": req["chunk_method"],
                "progress": 0,
                "progress_msg": "",
                "run": TaskStatus.UNSTART.value,
            },
        )
        if not e:
            return get_error_data_result(message="Document not found!")
    if not req.get("parser_config"):
        req["parser_config"] = get_parser_config(req["chunk_method"], req.get("parser_config"))
        DocumentService.update_parser_config(doc.id, req["parser_config"])
    if doc.token_num > 0:
        e = DocumentService.increment_chunk_num(
            doc.id,
            doc.kb_id,
            doc.token_num * -1,
            doc.chunk_num * -1,
            doc.process_duration * -1,
            )
        if not e:
            return get_error_data_result(message="Document not found!")
        settings.docStoreConn.delete({"doc_id": doc.id}, search.index_name(tenant_id), dataset_id)
    return None

def update_document_status_only(status:int, doc, kb):
    """Update document status only (without validation)."""
    if doc.status is None or (int(doc.status) != status):
        try:
            if not DocumentService.update_by_id(doc.id, {"status": str(status)}):
                return get_error_data_result(message="Database error (Document update)!")
            settings.docStoreConn.update({"doc_id": doc.id}, {"available_int": status}, search.index_name(kb.tenant_id), doc.kb_id)
        except Exception as e:
            return server_error_response(e)
    return None


def validate_document_update_fields(update_doc_req:UpdateDocumentReq, doc, req):
    """Validate document update fields in a single method."""
    # Validate immutable fields
    error_msg, error_code = validation_utils.validate_immutable_fields(update_doc_req, doc)
    if error_msg:
        return error_msg, error_code

    # Validate document name if present
    if "name" in req and req["name"] != doc.name:
        docs_from_name = DocumentService.query(name=req["name"], kb_id=doc.kb_id)
        error_msg, error_code = validation_utils.validate_document_name(req["name"], doc, docs_from_name)
        if error_msg:
            return error_msg, error_code

    # Validate chunk method if present
    if "chunk_method" in req:
        error_msg, error_code = validation_utils.validate_chunk_method(doc, req["chunk_method"])
        if error_msg:
            return error_msg, error_code

    return None, None

def rename_doc(doc):
    key_mapping = {
        "chunk_num": "chunk_count",
        "kb_id": "dataset_id",
        "token_num": "token_count",
        "parser_id": "chunk_method",
    }
    run_mapping = {
        "0": "UNSTART",
        "1": "RUNNING",
        "2": "CANCEL",
        "3": "DONE",
        "4": "FAIL",
    }
    renamed_doc = {}
    for key, value in doc.to_dict().items():
        new_key = key_mapping.get(key, key)
        renamed_doc[new_key] = value
        if key == "run":
            renamed_doc["run"] = run_mapping.get(str(value))
    return renamed_doc


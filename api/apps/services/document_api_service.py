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
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.utils import validation_utils
from common import settings
from common.constants import TaskStatus
from api.utils.api_utils import get_error_data_result, server_error_response, get_parser_config
from api.utils.validation_utils import UpdateDocumentReq
from rag.nlp import rag_tokenizer, search


def update_document_name_only(document_id, req_doc_name):
    """
    Update document name only (without validation).
    :param document_id: id (string) of the document
    :param req_doc_name: new name (string) from request for the document
    :return: None if all are good; otherwise returns the error message in the JSON format
    """
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
    """
    Update chunk method only (without validation).

    Updates the chunk method and parser configuration for a document,
    and resets the document's progress if the chunk method changes.
    Also clears existing chunks from the document store if the method changes.

    Args:
        req: The request dictionary containing chunk_method and parser_config.
        doc: The document model from the database.
        dataset_id: The ID of the dataset containing the document.
        tenant_id: The tenant ID for the document store.

    Returns:
        None if successful, or an error result dictionary if failed.
    """
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
    """
    Update document status only (without validation).

    Updates the enabled/disabled status of a document and updates
    the corresponding index in the document store.

    Args:
        status: The new status value (0 for disabled, 1 for enabled).
        doc: The document model from the database.
        kb: The knowledge base model.

    Returns:
        None if successful, or an error result dictionary if failed.
    """
    if doc.status is None or (int(doc.status) != status):
        try:
            if not DocumentService.update_by_id(doc.id, {"status": str(status)}):
                return get_error_data_result(message="Database error (Document update)!")
            settings.docStoreConn.update({"doc_id": doc.id}, {"available_int": status}, search.index_name(kb.tenant_id), doc.kb_id)
        except Exception as e:
            return server_error_response(e)
    return None


def validate_document_update_fields(update_doc_req:UpdateDocumentReq, doc, req):
    """
    Validate document update fields in a single method.

    Performs comprehensive validation of all document update fields,
    including immutable fields, document name, and chunk method.

    Args:
        update_doc_req: The validated update document request.
        doc: The document model from the database.
        req: The original request dictionary.

    Returns:
        A tuple of (error_message, error_code) if validation fails,
        or (None, None) if validation passes.
    """
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

def rename_doc_key(doc):
    """
    Rename document keys to match API response format.

    Converts internal document model field names to the external API
    response field names (e.g., 'chunk_num' -> 'chunk_count').

    Args:
        doc: The document model from the database.

    Returns:
        A dictionary with renamed keys for API response.
    """
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


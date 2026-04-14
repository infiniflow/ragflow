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
import datetime
import json
import re
from io import BytesIO

import xxhash
from pydantic import BaseModel, Field, validator
from quart import request, send_file

from api.constants import FILE_NAME_LEN_LIMIT
from api.db.db_models import APIToken, Document, File, Task
from api.db.joint_services.tenant_model_service import get_model_config_by_id, get_model_config_by_type_and_name, get_tenant_default_model_by_type
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api.db.services.task_service import TaskService, cancel_all_task_of, queue_tasks
from api.db.services.tenant_llm_service import TenantLLMService
from api.utils.api_utils import check_duplicate_ids, construct_json_result, get_error_data_result, get_request_json, get_result, server_error_response, token_required
from api.utils.image_utils import store_chunk_image
from common import settings
from common.constants import FileSource, LLMType, ParserType, RetCode, TaskStatus
from common.metadata_utils import convert_conditions, meta_filter
from common.misc_utils import thread_pool_exec
from common.string_utils import remove_redundant_spaces
from rag.app.qa import beAdoc, rmPrefix
from rag.app.tag import label_question
from rag.nlp import rag_tokenizer, search
from rag.prompts.generator import cross_languages, keyword_extraction

MAXIMUM_OF_UPLOADING_FILES = 256


class Chunk(BaseModel):
    id: str = ""
    content: str = ""
    document_id: str = ""
    docnm_kwd: str = ""
    important_keywords: list = Field(default_factory=list)
    tag_kwd: list = Field(default_factory=list)
    questions: list = Field(default_factory=list)
    question_tks: str = ""
    image_id: str = ""
    available: bool = True
    positions: list[list[int]] = Field(default_factory=list)

    @validator("positions")
    def validate_positions(cls, value):
        for sublist in value:
            if len(sublist) != 5:
                raise ValueError("Each sublist in positions must have a length of 5")
        return value


@manager.route("/datasets/<dataset_id>/documents/<document_id>", methods=["GET"])  # noqa: F821
@token_required
async def download(tenant_id, dataset_id, document_id):
    """
    Download a document from a dataset.
    ---
    tags:
      - Documents
    security:
      - ApiKeyAuth: []
    produces:
      - application/octet-stream
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
        description: ID of the document to download.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Document file stream.
        schema:
          type: file
      400:
        description: Error message.
        schema:
          type: object
    """
    if not document_id:
        return get_error_data_result(message="Specify document_id please.")
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(message=f"You do not own the dataset {dataset_id}.")
    doc = DocumentService.query(kb_id=dataset_id, id=document_id)
    if not doc:
        return get_error_data_result(message=f"The dataset not own the document {document_id}.")
    # The process of downloading
    doc_id, doc_location = File2DocumentService.get_storage_address(doc_id=document_id)  # minio address
    file_stream = settings.STORAGE_IMPL.get(doc_id, doc_location)
    if not file_stream:
        return construct_json_result(message="This file is empty.", code=RetCode.DATA_ERROR)
    file = BytesIO(file_stream)
    # Use send_file with a proper filename and MIME type
    return await send_file(
        file,
        as_attachment=True,
        attachment_filename=doc[0].name,
        mimetype="application/octet-stream",  # Set a default MIME type
    )


@manager.route("/documents/<document_id>", methods=["GET"])  # noqa: F821
async def download_doc(document_id):
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message="Authorization is not valid!")
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    if not document_id:
        return get_error_data_result(message="Specify document_id please.")
    doc = DocumentService.query(id=document_id)
    if not doc:
        return get_error_data_result(message=f"The dataset not own the document {document_id}.")
    # The process of downloading
    doc_id, doc_location = File2DocumentService.get_storage_address(doc_id=document_id)  # minio address
    file_stream = settings.STORAGE_IMPL.get(doc_id, doc_location)
    if not file_stream:
        return construct_json_result(message="This file is empty.", code=RetCode.DATA_ERROR)
    file = BytesIO(file_stream)
    # Use send_file with a proper filename and MIME type
    return await send_file(
        file,
        as_attachment=True,
        attachment_filename=doc[0].name,
        mimetype="application/octet-stream",  # Set a default MIME type
    )


@manager.route("/datasets/<dataset_id>/documents", methods=["GET"])  # noqa: F821
@token_required
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
        name: id
        type: string
        required: false
        description: Filter by document ID.
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
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}. ")

    q = request.args
    document_id = q.get("id")
    name = q.get("name")

    if document_id and not DocumentService.query(id=document_id, kb_id=dataset_id):
        return get_error_data_result(message=f"You don't own the document {document_id}.")
    if name and not DocumentService.query(name=name, kb_id=dataset_id):
        return get_error_data_result(message=f"You don't own the document {name}.")

    page = int(q.get("page", 1))
    page_size = int(q.get("page_size", 30))
    orderby = q.get("orderby", "create_time")
    desc = str(q.get("desc", "true")).strip().lower() != "false"
    keywords = q.get("keywords", "")

    # filters - align with OpenAPI parameter names
    suffix = q.getlist("suffix")
    run_status = q.getlist("run")
    create_time_from = int(q.get("create_time_from", 0))
    create_time_to = int(q.get("create_time_to", 0))
    metadata_condition_raw = q.get("metadata_condition")
    metadata_condition = {}
    if metadata_condition_raw:
        try:
            metadata_condition = json.loads(metadata_condition_raw)
        except Exception:
            return get_error_data_result(message="metadata_condition must be valid JSON.")
    if metadata_condition and not isinstance(metadata_condition, dict):
        return get_error_data_result(message="metadata_condition must be an object.")

    # map run status (text or numeric) - align with API parameter
    run_status_text_to_numeric = {"UNSTART": "0", "RUNNING": "1", "CANCEL": "2", "DONE": "3", "FAIL": "4"}
    run_status_converted = [run_status_text_to_numeric.get(v, v) for v in run_status]

    doc_ids_filter = None
    if metadata_condition:
        metas = DocMetadataService.get_flatted_meta_by_kbs([dataset_id])
        doc_ids_filter = meta_filter(metas, convert_conditions(metadata_condition), metadata_condition.get("logic", "and"))
        if metadata_condition.get("conditions") and not doc_ids_filter:
            return get_result(data={"total": 0, "docs": []})

    docs, total = DocumentService.get_list(dataset_id, page, page_size, orderby, desc, keywords, document_id, name, suffix, run_status_converted, doc_ids_filter)

    # time range filter (0 means no bound)
    if create_time_from or create_time_to:
        docs = [d for d in docs if (create_time_from == 0 or d.get("create_time", 0) >= create_time_from) and (create_time_to == 0 or d.get("create_time", 0) <= create_time_to)]

    # rename keys + map run status back to text for output
    key_mapping = {
        "chunk_num": "chunk_count",
        "kb_id": "dataset_id",
        "token_num": "token_count",
        "parser_id": "chunk_method",
    }
    run_status_numeric_to_text = {"0": "UNSTART", "1": "RUNNING", "2": "CANCEL", "3": "DONE", "4": "FAIL"}

    output_docs = []
    for d in docs:
        renamed_doc = {key_mapping.get(k, k): v for k, v in d.items()}
        if "run" in d:
            renamed_doc["run"] = run_status_numeric_to_text.get(str(d["run"]), d["run"])
        output_docs.append(renamed_doc)

    return get_result(data={"total": total, "docs": output_docs})


@manager.route("/datasets/<dataset_id>/metadata/update", methods=["POST"])  # noqa: F821
@token_required
async def metadata_batch_update(dataset_id, tenant_id):
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}. ")

    req = await get_request_json()
    selector = req.get("selector", {}) or {}
    updates = req.get("updates", []) or []
    deletes = req.get("deletes", []) or []

    if not isinstance(selector, dict):
        return get_error_data_result(message="selector must be an object.")
    if not isinstance(updates, list) or not isinstance(deletes, list):
        return get_error_data_result(message="updates and deletes must be lists.")

    metadata_condition = selector.get("metadata_condition", {}) or {}
    if metadata_condition and not isinstance(metadata_condition, dict):
        return get_error_data_result(message="metadata_condition must be an object.")

    document_ids = selector.get("document_ids", []) or []
    if document_ids and not isinstance(document_ids, list):
        return get_error_data_result(message="document_ids must be a list.")

    for upd in updates:
        if not isinstance(upd, dict) or not upd.get("key") or "value" not in upd:
            return get_error_data_result(message="Each update requires key and value.")
    for d in deletes:
        if not isinstance(d, dict) or not d.get("key"):
            return get_error_data_result(message="Each delete requires key.")

    if document_ids:
        kb_doc_ids = KnowledgebaseService.list_documents_by_ids([dataset_id])
        target_doc_ids = set(kb_doc_ids)
        invalid_ids = set(document_ids) - set(kb_doc_ids)
        if invalid_ids:
            return get_error_data_result(message=f"These documents do not belong to dataset {dataset_id}: {', '.join(invalid_ids)}")
        target_doc_ids = set(document_ids)

    if metadata_condition:
        metas = DocMetadataService.get_flatted_meta_by_kbs([dataset_id])
        filtered_ids = set(meta_filter(metas, convert_conditions(metadata_condition), metadata_condition.get("logic", "and")))
        target_doc_ids = target_doc_ids & filtered_ids
        if metadata_condition.get("conditions") and not target_doc_ids:
            return get_result(data={"updated": 0, "matched_docs": 0})

    target_doc_ids = list(target_doc_ids)
    updated = DocMetadataService.batch_update_metadata(dataset_id, target_doc_ids, updates, deletes)
    return get_result(data={"updated": updated, "matched_docs": len(target_doc_ids)})


@manager.route("/datasets/<dataset_id>/documents", methods=["DELETE"])  # noqa: F821
@token_required
async def delete(tenant_id, dataset_id):
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
        description: ID of the dataset.
      - in: body
        name: body
        description: Document deletion parameters.
        required: true
        schema:
          type: object
          properties:
            ids:
              type: array
              items:
                type: string
              description: |
                List of document IDs to delete.
                If omitted, `null`, or an empty array is provided, no documents will be deleted.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Documents deleted successfully.
        schema:
          type: object
    """
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}. ")
    req = await get_request_json()
    if not req:
        return get_result()

    doc_ids = req.get("ids")
    if not doc_ids:
        if req.get("delete_all") is True:
            doc_ids = [doc.id for doc in DocumentService.query(kb_id=dataset_id)]
            if not doc_ids:
                return get_result()
        else:
            return get_result()

    doc_list = doc_ids

    unique_doc_ids, duplicate_messages = check_duplicate_ids(doc_list, "document")
    doc_list = unique_doc_ids

    root_folder = FileService.get_root_folder(tenant_id)
    pf_id = root_folder["id"]
    FileService.init_knowledgebase_docs(pf_id, tenant_id)
    errors = ""
    not_found = []
    success_count = 0
    for doc_id in doc_list:
        try:
            e, doc = DocumentService.get_by_id(doc_id)
            if not e:
                not_found.append(doc_id)
                continue
            tenant_id = DocumentService.get_tenant_id(doc_id)
            if not tenant_id:
                return get_error_data_result(message="Tenant not found!")

            b, n = File2DocumentService.get_storage_address(doc_id=doc_id)

            if not DocumentService.remove_document(doc, tenant_id):
                return get_error_data_result(message="Database error (Document removal)!")

            f2d = File2DocumentService.get_by_document_id(doc_id)
            FileService.filter_delete(
                [
                    File.source_type == FileSource.KNOWLEDGEBASE,
                    File.id == f2d[0].file_id,
                ]
            )
            File2DocumentService.delete_by_document_id(doc_id)

            settings.STORAGE_IMPL.rm(b, n)
            success_count += 1
        except Exception as e:
            errors += str(e)

    if not_found:
        return get_result(message=f"Documents not found: {not_found}", code=RetCode.DATA_ERROR)

    if errors:
        return get_result(message=errors, code=RetCode.SERVER_ERROR)

    if duplicate_messages:
        if success_count > 0:
            return get_result(
                message=f"Partially deleted {success_count} datasets with {len(duplicate_messages)} errors",
                data={"success_count": success_count, "errors": duplicate_messages},
            )
        else:
            return get_error_data_result(message=";".join(duplicate_messages))

    return get_result()


DOC_STOP_PARSING_INVALID_STATE_MESSAGE = "Can't stop parsing document that has not started or already completed"
DOC_STOP_PARSING_INVALID_STATE_ERROR_CODE = "DOC_STOP_PARSING_INVALID_STATE"


@manager.route("/datasets/<dataset_id>/chunks", methods=["POST"])  # noqa: F821
@token_required
async def parse(tenant_id, dataset_id):
    """
    Start parsing documents into chunks.
    ---
    tags:
      - Chunks
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: dataset_id
        type: string
        required: true
        description: ID of the dataset.
      - in: body
        name: body
        description: Parsing parameters.
        required: true
        schema:
          type: object
          properties:
            document_ids:
              type: array
              items:
                type: string
              description: List of document IDs to parse.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Parsing started successfully.
        schema:
          type: object
    """
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    req = await get_request_json()
    if not req.get("document_ids"):
        return get_error_data_result("`document_ids` is required")
    doc_list = req.get("document_ids")
    unique_doc_ids, duplicate_messages = check_duplicate_ids(doc_list, "document")
    doc_list = unique_doc_ids

    not_found = []
    success_count = 0
    for id in doc_list:
        doc = DocumentService.query(id=id, kb_id=dataset_id)
        if not doc:
            not_found.append(id)
            continue
        if not doc:
            return get_error_data_result(message=f"You don't own the document {id}.")
        info = {"run": "1", "progress": 0, "progress_msg": "", "chunk_num": 0, "token_num": 0}
        if (
            DocumentService.filter_update(
                [
                    Document.id == id,
                    ((Document.run.is_null(True)) | (Document.run != TaskStatus.RUNNING.value)),
                ],
                info,
            )
            == 0
        ):
            return get_error_data_result("Can't parse document that is currently being processed")
        settings.docStoreConn.delete({"doc_id": id}, search.index_name(tenant_id), dataset_id)
        TaskService.filter_delete([Task.doc_id == id])
        e, doc = DocumentService.get_by_id(id)
        doc = doc.to_dict()
        doc["tenant_id"] = tenant_id
        bucket, name = File2DocumentService.get_storage_address(doc_id=doc["id"])
        queue_tasks(doc, bucket, name, 0)
        success_count += 1
    if not_found:
        return get_result(message=f"Documents not found: {not_found}", code=RetCode.DATA_ERROR)
    if duplicate_messages:
        if success_count > 0:
            return get_result(
                message=f"Partially parsed {success_count} documents with {len(duplicate_messages)} errors",
                data={"success_count": success_count, "errors": duplicate_messages},
            )
        else:
            return get_error_data_result(message=";".join(duplicate_messages))

    return get_result()


@manager.route("/datasets/<dataset_id>/chunks", methods=["DELETE"])  # noqa: F821
@token_required
async def stop_parsing(tenant_id, dataset_id):
    """
    Stop parsing documents into chunks.
    ---
    tags:
      - Chunks
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: dataset_id
        type: string
        required: true
        description: ID of the dataset.
      - in: body
        name: body
        description: Stop parsing parameters.
        required: true
        schema:
          type: object
          properties:
            document_ids:
              type: array
              items:
                type: string
              description: List of document IDs to stop parsing.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Parsing stopped successfully.
        schema:
          type: object
    """
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    req = await get_request_json()

    if not req.get("document_ids"):
        return get_error_data_result("`document_ids` is required")
    doc_list = req.get("document_ids")
    unique_doc_ids, duplicate_messages = check_duplicate_ids(doc_list, "document")
    doc_list = unique_doc_ids

    success_count = 0
    for id in doc_list:
        doc = DocumentService.query(id=id, kb_id=dataset_id)
        if not doc:
            return get_error_data_result(message=f"You don't own the document {id}.")
        if doc[0].run != TaskStatus.RUNNING.value:
            return construct_json_result(
                code=RetCode.DATA_ERROR,
                message=DOC_STOP_PARSING_INVALID_STATE_MESSAGE,
                data={"error_code": DOC_STOP_PARSING_INVALID_STATE_ERROR_CODE},
            )
        # Send cancellation signal via Redis to stop background task
        cancel_all_task_of(id)
        info = {"run": "2", "progress": 0, "chunk_num": 0}
        DocumentService.update_by_id(id, info)
        settings.docStoreConn.delete({"doc_id": doc[0].id}, search.index_name(tenant_id), dataset_id)
        success_count += 1
    if duplicate_messages:
        if success_count > 0:
            return get_result(
                message=f"Partially stopped {success_count} documents with {len(duplicate_messages)} errors",
                data={"success_count": success_count, "errors": duplicate_messages},
            )
        else:
            return get_error_data_result(message=";".join(duplicate_messages))
    return get_result()


@manager.route("/datasets/<dataset_id>/documents/<document_id>/chunks", methods=["GET"])  # noqa: F821
@token_required
async def list_chunks(tenant_id, dataset_id, document_id):
    """
    List chunks of a document.
    ---
    tags:
      - Chunks
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
        name: id
        type: string
        required: false
        default: ""
        description: Chunk id.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: List of chunks.
        schema:
          type: object
          properties:
            total:
              type: integer
              description: Total number of chunks.
            chunks:
              type: array
              items:
                type: object
                properties:
                  id:
                    type: string
                    description: Chunk ID.
                  content:
                    type: string
                    description: Chunk content.
                  document_id:
                    type: string
                    description: ID of the document.
                  important_keywords:
                    type: array
                    items:
                      type: string
                    description: Important keywords.
                  tag_kwd:
                    type: array
                    items:
                      type: string
                    description: Tag keywords.
                  image_id:
                    type: string
                    description: Image ID associated with the chunk.
            doc:
              type: object
              description: Document details.
    """
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(message=f"You don't own the document {document_id}.")
    doc = doc[0]
    req = request.args
    doc_id = document_id
    page = int(req.get("page", 1))
    size = int(req.get("page_size", 30))
    question = req.get("keywords", "")
    query = {
        "doc_ids": [doc_id],
        "page": page,
        "size": size,
        "question": question,
        "sort": True,
    }
    if "available" in req:
        query["available_int"] = 1 if req["available"] == "true" else 0
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
    doc = doc.to_dict()
    renamed_doc = {}
    for key, value in doc.items():
        new_key = key_mapping.get(key, key)
        renamed_doc[new_key] = value
        if key == "run":
            renamed_doc["run"] = run_mapping.get(str(value))

    res = {"total": 0, "chunks": [], "doc": renamed_doc}
    if req.get("id"):
        chunk = settings.docStoreConn.get(req.get("id"), search.index_name(tenant_id), [dataset_id])
        if not chunk:
            return get_result(message=f"Chunk not found: {dataset_id}/{req.get('id')}", code=RetCode.NOT_FOUND)
        k = []
        for n in chunk.keys():
            if re.search(r"(_vec$|_sm_|_tks|_ltks)", n):
                k.append(n)
        for n in k:
            del chunk[n]
        if not chunk:
            return get_error_data_result(f"Chunk `{req.get('id')}` not found.")
        res["total"] = 1
        final_chunk = {
            "id": chunk.get("id", chunk.get("chunk_id")),
            "content": chunk["content_with_weight"],
            "document_id": chunk.get("doc_id", chunk.get("document_id")),
            "docnm_kwd": chunk["docnm_kwd"],
            "important_keywords": chunk.get("important_kwd", []),
            "questions": chunk.get("question_kwd", []),
            "dataset_id": chunk.get("kb_id", chunk.get("dataset_id")),
            "image_id": chunk.get("img_id", ""),
            "available": bool(chunk.get("available_int", 1)),
            "positions": chunk.get("position_int", []),
            "tag_kwd": chunk.get("tag_kwd", []),
            "tag_feas": chunk.get("tag_feas", {}),
        }
        res["chunks"].append(final_chunk)
        _ = Chunk(**final_chunk)

    elif settings.docStoreConn.index_exist(search.index_name(tenant_id), dataset_id):
        sres = await settings.retriever.search(query, search.index_name(tenant_id), [dataset_id], emb_mdl=None, highlight=True)
        res["total"] = sres.total
        for id in sres.ids:
            d = {
                "id": id,
                "content": (remove_redundant_spaces(sres.highlight[id]) if question and id in sres.highlight else sres.field[id].get("content_with_weight", "")),
                "document_id": sres.field[id]["doc_id"],
                "docnm_kwd": sres.field[id]["docnm_kwd"],
                "important_keywords": sres.field[id].get("important_kwd", []),
                "tag_kwd": sres.field[id].get("tag_kwd", []),
                "questions": sres.field[id].get("question_kwd", []),
                "dataset_id": sres.field[id].get("kb_id", sres.field[id].get("dataset_id")),
                "image_id": sres.field[id].get("img_id", ""),
                "available": bool(int(sres.field[id].get("available_int", "1"))),
                "positions": sres.field[id].get("position_int", []),
            }
            res["chunks"].append(d)
            _ = Chunk(**d)  # validate the chunk
    return get_result(data=res)


@manager.route(  # noqa: F821
    "/datasets/<dataset_id>/documents/<document_id>/chunks", methods=["POST"]
)
@token_required
async def add_chunk(tenant_id, dataset_id, document_id):
    """
    Add a chunk to a document.
    ---
    tags:
      - Chunks
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
      - in: body
        name: body
        description: Chunk data.
        required: true
        schema:
          type: object
          properties:
            content:
              type: string
              required: true
              description: Content of the chunk.
            important_keywords:
              type: array
              items:
                type: string
              description: Important keywords.
            image_base64:
              type: string
              description: Base64-encoded image to associate with the chunk.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Chunk added successfully.
        schema:
          type: object
          properties:
            chunk:
              type: object
              properties:
                id:
                  type: string
                  description: Chunk ID.
                content:
                  type: string
                  description: Chunk content.
                document_id:
                  type: string
                  description: ID of the document.
                important_keywords:
                  type: array
                  items:
                    type: string
                  description: Important keywords.
    """
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(message=f"You don't own the document {document_id}.")
    doc = doc[0]
    req = await get_request_json()
    if not str(req.get("content", "")).strip():
        return get_error_data_result(message="`content` is required")
    if "important_keywords" in req:
        if not isinstance(req["important_keywords"], list):
            return get_error_data_result("`important_keywords` is required to be a list")
    if "questions" in req:
        if not isinstance(req["questions"], list):
            return get_error_data_result("`questions` is required to be a list")
    chunk_id = xxhash.xxh64((req["content"] + document_id).encode("utf-8")).hexdigest()
    d = {
        "id": chunk_id,
        "content_ltks": rag_tokenizer.tokenize(req["content"]),
        "content_with_weight": req["content"],
    }
    d["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(d["content_ltks"])
    d["important_kwd"] = req.get("important_keywords", [])
    d["important_tks"] = rag_tokenizer.tokenize(" ".join(req.get("important_keywords", [])))
    d["question_kwd"] = [str(q).strip() for q in req.get("questions", []) if str(q).strip()]
    d["question_tks"] = rag_tokenizer.tokenize("\n".join(req.get("questions", [])))
    d["create_time"] = str(datetime.datetime.now()).replace("T", " ")[:19]
    d["create_timestamp_flt"] = datetime.datetime.now().timestamp()
    d["kb_id"] = dataset_id
    d["docnm_kwd"] = doc.name
    d["doc_id"] = document_id
    if "tag_kwd" in req:
        if not isinstance(req["tag_kwd"], list):
            return get_error_data_result("`tag_kwd` is required to be a list")
        if not all(isinstance(t, str) for t in req["tag_kwd"]):
            return get_error_data_result("`tag_kwd` must be a list of strings")
        d["tag_kwd"] = req["tag_kwd"]
    if "tag_feas" in req:
        d["tag_feas"] = req["tag_feas"]
    import base64

    image_base64 = req.get("image_base64", None)
    if image_base64:
        d["img_id"] = "{}-{}".format(dataset_id, chunk_id)
        d["doc_type_kwd"] = "image"

    tenant_embd_id = DocumentService.get_tenant_embd_id(document_id)
    if tenant_embd_id:
        model_config = get_model_config_by_id(tenant_embd_id)
    else:
        embd_id = DocumentService.get_embd_id(document_id)
        model_config = get_model_config_by_type_and_name(tenant_id, LLMType.EMBEDDING.value, embd_id)
    embd_mdl = TenantLLMService.model_instance(model_config)
    v, c = embd_mdl.encode([doc.name, req["content"] if not d["question_kwd"] else "\n".join(d["question_kwd"])])
    v = 0.1 * v[0] + 0.9 * v[1]
    d["q_%d_vec" % len(v)] = v.tolist()
    settings.docStoreConn.insert([d], search.index_name(tenant_id), dataset_id)

    if image_base64:
        store_chunk_image(dataset_id, chunk_id, base64.b64decode(image_base64))

    DocumentService.increment_chunk_num(doc.id, doc.kb_id, c, 1, 0)
    # rename keys
    key_mapping = {
        "id": "id",
        "content_with_weight": "content",
        "doc_id": "document_id",
        "important_kwd": "important_keywords",
        "tag_kwd": "tag_kwd",
        "question_kwd": "questions",
        "kb_id": "dataset_id",
        "create_timestamp_flt": "create_timestamp",
        "create_time": "create_time",
        "document_keyword": "document",
        "img_id": "image_id",
    }
    renamed_chunk = {}
    for key, value in d.items():
        if key in key_mapping:
            new_key = key_mapping.get(key, key)
            renamed_chunk[new_key] = value
    _ = Chunk(**renamed_chunk)  # validate the chunk
    return get_result(data={"chunk": renamed_chunk})
    # return get_result(data={"chunk_id": chunk_id})


@manager.route(  # noqa: F821
    "datasets/<dataset_id>/documents/<document_id>/chunks", methods=["DELETE"]
)
@token_required
async def rm_chunk(tenant_id, dataset_id, document_id):
    """
    Remove chunks from a document.
    ---
    tags:
      - Chunks
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
      - in: body
        name: body
        description: Chunk removal parameters.
        required: true
        schema:
          type: object
          properties:
            chunk_ids:
              type: array
              items:
                type: string
              description: |
                List of chunk IDs to remove.
                If omitted, `null`, or an empty array is provided, no chunks will be deleted.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Chunks removed successfully.
        schema:
          type: object
    """
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    docs = DocumentService.get_by_ids([document_id])
    if not docs:
        raise LookupError(f"Can't find the document with ID {document_id}!")
    req = await get_request_json()
    if not req:
        return get_result()

    chunk_ids = req.get("chunk_ids")
    if not chunk_ids:
        if req.get("delete_all") is True:
            doc = docs[0]
            # Clean up storage assets while index rows still exist for discovery
            DocumentService.delete_chunk_images(doc, tenant_id)
            condition = {"doc_id": document_id}
            chunk_number = settings.docStoreConn.delete(condition, search.index_name(tenant_id), dataset_id)
            if chunk_number != 0:
                DocumentService.decrement_chunk_num(document_id, dataset_id, 1, chunk_number, 0)
            return get_result(message=f"deleted {chunk_number} chunks")
        else:
            return get_result()

    condition = {"doc_id": document_id}
    unique_chunk_ids, duplicate_messages = check_duplicate_ids(chunk_ids, "chunk")
    condition["id"] = unique_chunk_ids
    chunk_number = settings.docStoreConn.delete(condition, search.index_name(tenant_id), dataset_id)
    if chunk_number != 0:
        DocumentService.decrement_chunk_num(document_id, dataset_id, 1, chunk_number, 0)
    if chunk_number != len(unique_chunk_ids):
        if len(unique_chunk_ids) == 0:
            return get_result(message=f"deleted {chunk_number} chunks")
        return get_error_data_result(message=f"rm_chunk deleted chunks {chunk_number}, expect {len(unique_chunk_ids)}")
    if duplicate_messages:
        return get_result(
            message=f"Partially deleted {chunk_number} chunks with {len(duplicate_messages)} errors",
            data={"success_count": chunk_number, "errors": duplicate_messages},
        )
    return get_result(message=f"deleted {chunk_number} chunks")


@manager.route(  # noqa: F821
    "/datasets/<dataset_id>/documents/<document_id>/chunks/<chunk_id>", methods=["PUT"]
)
@token_required
async def update_chunk(tenant_id, dataset_id, document_id, chunk_id):
    """
    Update a chunk within a document.
    ---
    tags:
      - Chunks
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
      - in: path
        name: chunk_id
        type: string
        required: true
        description: ID of the chunk to update.
      - in: body
        name: body
        description: Chunk update parameters.
        required: true
        schema:
          type: object
          properties:
            content:
              type: string
              description: Updated content of the chunk.
            important_keywords:
              type: array
              items:
                type: string
              description: Updated important keywords.
            tag_kwd:
              type: array
              items:
                type: string
              description: Updated tag keywords.
            available:
              type: boolean
              description: Availability status of the chunk.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Chunk updated successfully.
        schema:
          type: object
    """
    chunk = settings.docStoreConn.get(chunk_id, search.index_name(tenant_id), [dataset_id])
    if chunk is None:
        return get_error_data_result(f"Can't find this chunk {chunk_id}")
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(message=f"You don't own the document {document_id}.")
    doc = doc[0]
    req = await get_request_json()
    if "content" in req and req["content"] is not None:
        content = req["content"]
    else:
        content = chunk.get("content_with_weight", "")
    d = {"id": chunk_id, "content_with_weight": content}
    d["content_ltks"] = rag_tokenizer.tokenize(d["content_with_weight"])
    d["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(d["content_ltks"])
    if "important_keywords" in req:
        if not isinstance(req["important_keywords"], list):
            return get_error_data_result("`important_keywords` should be a list")
        d["important_kwd"] = req.get("important_keywords", [])
        d["important_tks"] = rag_tokenizer.tokenize(" ".join(req["important_keywords"]))
    if "questions" in req:
        if not isinstance(req["questions"], list):
            return get_error_data_result("`questions` should be a list")
        d["question_kwd"] = [str(q).strip() for q in req.get("questions", []) if str(q).strip()]
        d["question_tks"] = rag_tokenizer.tokenize("\n".join(req["questions"]))
    if "available" in req:
        d["available_int"] = int(req["available"])
    if "positions" in req:
        if not isinstance(req["positions"], list):
            return get_error_data_result("`positions` should be a list")
        d["position_int"] = req["positions"]
    if "tag_kwd" in req:
        if not isinstance(req["tag_kwd"], list):
            return get_error_data_result("`tag_kwd` should be a list")
        if not all(isinstance(t, str) for t in req["tag_kwd"]):
            return get_error_data_result("`tag_kwd` must be a list of strings")
        d["tag_kwd"] = req["tag_kwd"]
    if "tag_feas" in req:
        d["tag_feas"] = req["tag_feas"]
    tenant_embd_id = DocumentService.get_tenant_embd_id(document_id)
    if tenant_embd_id:
        model_config = get_model_config_by_id(tenant_embd_id)
    else:
        embd_id = DocumentService.get_embd_id(document_id)
        model_config = get_model_config_by_type_and_name(tenant_id, LLMType.EMBEDDING.value, embd_id)
    embd_mdl = TenantLLMService.model_instance(model_config)
    if doc.parser_id == ParserType.QA:
        arr = [t for t in re.split(r"[\n\t]", d["content_with_weight"]) if len(t) > 1]
        if len(arr) != 2:
            return get_error_data_result(message="Q&A must be separated by TAB/ENTER key.")
        q, a = rmPrefix(arr[0]), rmPrefix(arr[1])
        d = beAdoc(d, arr[0], arr[1], not any([rag_tokenizer.is_chinese(t) for t in q + a]))

    v, c = embd_mdl.encode([doc.name, d["content_with_weight"] if not d.get("question_kwd") else "\n".join(d["question_kwd"])])
    v = 0.1 * v[0] + 0.9 * v[1] if doc.parser_id != ParserType.QA else v[1]
    d["q_%d_vec" % len(v)] = v.tolist()
    settings.docStoreConn.update({"id": chunk_id}, d, search.index_name(tenant_id), dataset_id)
    return get_result()


@manager.route(  # noqa: F821
    "/datasets/<dataset_id>/documents/<document_id>/chunks/switch", methods=["POST"]
)
@token_required
async def switch_chunks(tenant_id, dataset_id, document_id):
    """
    Switch availability of specified chunks (same as chunk_app switch).
    ---
    tags:
      - Chunks
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
      - in: body
        name: body
        required: true
        schema:
          type: object
          properties:
            chunk_ids:
              type: array
              items:
                type: string
              description: List of chunk IDs to switch.
            available_int:
              type: integer
              description: 1 for available, 0 for unavailable.
            available:
              type: boolean
              description: Availability status (alternative to available_int).
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Chunks availability switched successfully.
    """
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    req = await get_request_json()
    if not req.get("chunk_ids"):
        return get_error_data_result(message="`chunk_ids` is required.")
    if "available_int" not in req and "available" not in req:
        return get_error_data_result(message="`available_int` or `available` is required.")
    available_int = int(req["available_int"]) if "available_int" in req else (1 if req.get("available") else 0)
    try:

        def _switch_sync():
            e, doc = DocumentService.get_by_id(document_id)
            if not e:
                return get_error_data_result(message="Document not found!")
            if not doc or str(doc.kb_id) != str(dataset_id):
                return get_error_data_result(message="Document not found!")
            for cid in req["chunk_ids"]:
                if not settings.docStoreConn.update(
                    {"id": cid},
                    {"available_int": available_int},
                    search.index_name(tenant_id),
                    doc.kb_id,
                ):
                    return get_error_data_result(message="Index updating failure")
            return get_result(data=True)

        return await thread_pool_exec(_switch_sync)
    except Exception as e:
        return server_error_response(e)


@manager.route("/retrieval", methods=["POST"])  # noqa: F821
@token_required
async def retrieval_test(tenant_id):
    """
    Retrieve chunks based on a query.
    ---
    tags:
      - Retrieval
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        description: Retrieval parameters.
        required: true
        schema:
          type: object
          properties:
            dataset_ids:
              type: array
              items:
                type: string
              required: true
              description: List of dataset IDs to search in.
            question:
              type: string
              required: true
              description: Query string.
            document_ids:
              type: array
              items:
                type: string
              description: List of document IDs to filter.
            similarity_threshold:
              type: number
              format: float
              description: Similarity threshold.
            vector_similarity_weight:
              type: number
              format: float
              description: Vector similarity weight.
            top_k:
              type: integer
              description: Maximum number of chunks to return.
            highlight:
              type: boolean
              description: Whether to highlight matched content.
            metadata_condition:
              type: object
              description: metadata filter condition.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Retrieval results.
        schema:
          type: object
          properties:
            chunks:
              type: array
              items:
                type: object
                properties:
                  id:
                    type: string
                    description: Chunk ID.
                  content:
                    type: string
                    description: Chunk content.
                  document_id:
                    type: string
                    description: ID of the document.
                  dataset_id:
                    type: string
                    description: ID of the dataset.
                  similarity:
                    type: number
                    format: float
                    description: Similarity score.
    """
    req = await get_request_json()
    if not req.get("dataset_ids"):
        return get_error_data_result("`dataset_ids` is required.")
    kb_ids = req["dataset_ids"]
    if not isinstance(kb_ids, list):
        return get_error_data_result("`dataset_ids` should be a list")
    for id in kb_ids:
        if not KnowledgebaseService.accessible(kb_id=id, user_id=tenant_id):
            return get_error_data_result(f"You don't own the dataset {id}.")
    kbs = KnowledgebaseService.get_by_ids(kb_ids)
    embd_nms = list(set([TenantLLMService.split_model_name_and_factory(kb.embd_id)[0] for kb in kbs]))  # remove vendor suffix for comparison
    if len(embd_nms) != 1:
        return get_result(
            message='Datasets use different embedding models."',
            code=RetCode.DATA_ERROR,
        )
    if "question" not in req:
        return get_error_data_result("`question` is required.")
    page = int(req.get("page", 1))
    size = int(req.get("page_size", 30))
    question = req["question"]
    # Trim whitespace and validate question
    if isinstance(question, str):
        question = question.strip()
    # Return empty result if question is empty or whitespace-only
    if not question:
        return get_result(data={"total": 0, "chunks": [], "doc_aggs": {}})
    doc_ids = req.get("document_ids", [])
    use_kg = req.get("use_kg", False)
    toc_enhance = req.get("toc_enhance", False)
    langs = req.get("cross_languages", [])
    if not isinstance(doc_ids, list):
        return get_error_data_result("`documents` should be a list")
    if doc_ids:
        doc_ids_list = KnowledgebaseService.list_documents_by_ids(kb_ids)
        for doc_id in doc_ids:
            if doc_id not in doc_ids_list:
                return get_error_data_result(f"The datasets don't own the document {doc_id}")
    if not doc_ids:
        metadata_condition = req.get("metadata_condition")
        if metadata_condition:
            metas = DocMetadataService.get_flatted_meta_by_kbs(kb_ids)
            doc_ids = meta_filter(metas, convert_conditions(metadata_condition), metadata_condition.get("logic", "and"))
            # If metadata_condition has conditions but no docs match, return empty result
            if not doc_ids and metadata_condition.get("conditions"):
                return get_result(data={"total": 0, "chunks": [], "doc_aggs": {}})
            if metadata_condition and not doc_ids:
                doc_ids = ["-999"]
        else:
            # If doc_ids is None all documents of the datasets are used
            doc_ids = None
    similarity_threshold = float(req.get("similarity_threshold", 0.2))
    vector_similarity_weight = float(req.get("vector_similarity_weight", 0.3))
    top = int(req.get("top_k", 1024))
    highlight_val = req.get("highlight", None)
    if highlight_val is None:
        highlight = False
    elif isinstance(highlight_val, bool):
        highlight = highlight_val
    elif isinstance(highlight_val, str):
        if highlight_val.lower() in ["true", "false"]:
            highlight = highlight_val.lower() == "true"
        else:
            return get_error_data_result("`highlight` should be a boolean")
    else:
        return get_error_data_result("`highlight` should be a boolean")
    try:
        tenant_ids = list(set([kb.tenant_id for kb in kbs]))
        e, kb = KnowledgebaseService.get_by_id(kb_ids[0])
        if not e:
            return get_error_data_result(message="Dataset not found!")
        if kb.tenant_embd_id:
            embd_model_config = get_model_config_by_id(kb.tenant_embd_id)
        else:
            embd_model_config = get_model_config_by_type_and_name(kb.tenant_id, LLMType.EMBEDDING, kb.embd_id)
        embd_mdl = LLMBundle(kb.tenant_id, embd_model_config)

        rerank_mdl = None
        if req.get("tenant_rerank_id"):
            rerank_model_config = get_model_config_by_id(req["tenant_rerank_id"])
            rerank_mdl = LLMBundle(kb.tenant_id, rerank_model_config)
        elif req.get("rerank_id"):
            rerank_model_config = get_model_config_by_type_and_name(kb.tenant_id, LLMType.RERANK, req["rerank_id"])
            rerank_mdl = LLMBundle(kb.tenant_id, rerank_model_config)

        if langs:
            question = await cross_languages(kb.tenant_id, None, question, langs)

        if req.get("keyword", False):
            chat_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.CHAT)
            chat_mdl = LLMBundle(kb.tenant_id, chat_model_config)
            question += await keyword_extraction(chat_mdl, question)

        ranks = await settings.retriever.retrieval(
            question,
            embd_mdl,
            tenant_ids,
            kb_ids,
            page,
            size,
            similarity_threshold,
            vector_similarity_weight,
            top,
            doc_ids,
            rerank_mdl=rerank_mdl,
            highlight=highlight,
            rank_feature=label_question(question, kbs),
        )
        if toc_enhance:
            chat_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.CHAT)
            chat_mdl = LLMBundle(kb.tenant_id, chat_model_config)
            cks = await settings.retriever.retrieval_by_toc(question, ranks["chunks"], tenant_ids, chat_mdl, size)
            if cks:
                ranks["chunks"] = cks
        ranks["chunks"] = settings.retriever.retrieval_by_children(ranks["chunks"], tenant_ids)
        if use_kg:
            chat_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.CHAT)
            ck = await settings.kg_retriever.retrieval(question, [k.tenant_id for k in kbs], kb_ids, embd_mdl, LLMBundle(kb.tenant_id, chat_model_config))
            if ck["content_with_weight"]:
                ranks["chunks"].insert(0, ck)

        for c in ranks["chunks"]:
            c.pop("vector", None)

        ##rename keys
        renamed_chunks = []
        for chunk in ranks["chunks"]:
            key_mapping = {
                "chunk_id": "id",
                "content_with_weight": "content",
                "doc_id": "document_id",
                "important_kwd": "important_keywords",
                "question_kwd": "questions",
                "docnm_kwd": "document_keyword",
                "kb_id": "dataset_id",
            }
            rename_chunk = {}
            for key, value in chunk.items():
                new_key = key_mapping.get(key, key)
                rename_chunk[new_key] = value
            renamed_chunks.append(rename_chunk)
        ranks["chunks"] = renamed_chunks
        return get_result(data=ranks)
    except Exception as e:
        if str(e).find("not_found") > 0:
            return get_result(
                message="No chunk found! Check the chunk status please!",
                code=RetCode.DATA_ERROR,
            )
        return server_error_response(e)

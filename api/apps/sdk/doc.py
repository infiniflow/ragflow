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
import pathlib
import datetime

from api.db.services.dialog_service import keyword_extraction
from rag.app.qa import rmPrefix, beAdoc
from rag.nlp import rag_tokenizer
from api.db import LLMType, ParserType
from api.db.services.llm_service import TenantLLMService
from api import settings
import xxhash
import re
from api.utils.api_utils import token_required
from api.db.db_models import Task
from api.db.services.task_service import TaskService, queue_tasks
from api.utils.api_utils import server_error_response
from api.utils.api_utils import get_result, get_error_data_result
from io import BytesIO
from flask import request, send_file
from api.db import FileSource, TaskStatus, FileType
from api.db.db_models import File
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils.api_utils import construct_json_result, get_parser_config
from rag.nlp import search
from rag.utils import rmSpace
from rag.utils.storage_factory import STORAGE_IMPL

from pydantic import BaseModel, Field, validator

MAXIMUM_OF_UPLOADING_FILES = 256


class Chunk(BaseModel):
    id: str = ""
    content: str = ""
    document_id: str = ""
    docnm_kwd: str = ""
    important_keywords: list = Field(default_factory=list)
    questions: list = Field(default_factory=list)
    question_tks: str = ""
    image_id: str = ""
    available: bool = True
    positions: list[list[int]] = Field(default_factory=list)

    @validator('positions')
    def validate_positions(cls, value):
        for sublist in value:
            if len(sublist) != 5:
                raise ValueError("Each sublist in positions must have a length of 5")
        return value

@manager.route("/datasets/<dataset_id>/documents", methods=["POST"])  # noqa: F821
@token_required
def upload(dataset_id, tenant_id):
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
    if "file" not in request.files:
        return get_error_data_result(
            message="No file part!", code=settings.RetCode.ARGUMENT_ERROR
        )
    file_objs = request.files.getlist("file")
    for file_obj in file_objs:
        if file_obj.filename == "":
            return get_result(
                message="No file selected!", code=settings.RetCode.ARGUMENT_ERROR
            )
    '''
    # total size
    total_size = 0
    for file_obj in file_objs:
        file_obj.seek(0, os.SEEK_END)
        total_size += file_obj.tell()
        file_obj.seek(0)
    MAX_TOTAL_FILE_SIZE = 10 * 1024 * 1024
    if total_size > MAX_TOTAL_FILE_SIZE:
        return get_result(
            message=f"Total file size exceeds 10MB limit! ({total_size / (1024 * 1024):.2f} MB)",
            code=settings.RetCode.ARGUMENT_ERROR,
        )
    '''
    e, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not e:
        raise LookupError(f"Can't find the dataset with ID {dataset_id}!")
    err, files = FileService.upload_document(kb, file_objs, tenant_id)
    if err:
        return get_result(message="\n".join(err), code=settings.RetCode.SERVER_ERROR)
    # rename key's name
    renamed_doc_list = []
    for file in files:
        doc = file[0]
        key_mapping = {
            "chunk_num": "chunk_count",
            "kb_id": "dataset_id",
            "token_num": "token_count",
            "parser_id": "chunk_method",
        }
        renamed_doc = {}
        for key, value in doc.items():
            new_key = key_mapping.get(key, key)
            renamed_doc[new_key] = value
        renamed_doc["run"] = "UNSTART"
        renamed_doc_list.append(renamed_doc)
    return get_result(data=renamed_doc_list)


@manager.route("/datasets/<dataset_id>/documents/<document_id>", methods=["PUT"])  # noqa: F821
@token_required
def update_doc(tenant_id, dataset_id, document_id):
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
    responses:
      200:
        description: Document updated successfully.
        schema:
          type: object
    """
    req = request.json
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(message="You don't own the dataset.")
    doc = DocumentService.query(kb_id=dataset_id, id=document_id)
    if not doc:
        return get_error_data_result(message="The dataset doesn't own the document.")
    doc = doc[0]
    if "chunk_count" in req:
        if req["chunk_count"] != doc.chunk_num:
            return get_error_data_result(message="Can't change `chunk_count`.")
    if "token_count" in req:
        if req["token_count"] != doc.token_num:
            return get_error_data_result(message="Can't change `token_count`.")
    if "progress" in req:
        if req["progress"] != doc.progress:
            return get_error_data_result(message="Can't change `progress`.")

    if "name" in req and req["name"] != doc.name:
        if (
                pathlib.Path(req["name"].lower()).suffix
                != pathlib.Path(doc.name.lower()).suffix
        ):
            return get_result(
                message="The extension of file can't be changed",
                code=settings.RetCode.ARGUMENT_ERROR,
            )
        for d in DocumentService.query(name=req["name"], kb_id=doc.kb_id):
            if d.name == req["name"]:
                return get_error_data_result(
                    message="Duplicated document name in the same dataset."
                )
        if not DocumentService.update_by_id(document_id, {"name": req["name"]}):
            return get_error_data_result(message="Database error (Document rename)!")

        informs = File2DocumentService.get_by_document_id(document_id)
        if informs:
            e, file = FileService.get_by_id(informs[0].file_id)
            FileService.update_by_id(file.id, {"name": req["name"]})
    if "parser_config" in req:
        DocumentService.update_parser_config(doc.id, req["parser_config"])
    if "chunk_method" in req:
        valid_chunk_method = {
            "naive",
            "manual",
            "qa",
            "table",
            "paper",
            "book",
            "laws",
            "presentation",
            "picture",
            "one",
            "knowledge_graph",
            "email",
        }
        if req.get("chunk_method") not in valid_chunk_method:
            return get_error_data_result(
                f"`chunk_method` {req['chunk_method']} doesn't exist"
            )
        if doc.parser_id.lower() == req["chunk_method"].lower():
            return get_result()

        if doc.type == FileType.VISUAL or re.search(r"\.(ppt|pptx|pages)$", doc.name):
            return get_error_data_result(message="Not supported yet!")

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
        req["parser_config"] = get_parser_config(
            req["chunk_method"], req.get("parser_config")
        )
        DocumentService.update_parser_config(doc.id, req["parser_config"])
        if doc.token_num > 0:
            e = DocumentService.increment_chunk_num(
                doc.id,
                doc.kb_id,
                doc.token_num * -1,
                doc.chunk_num * -1,
                doc.process_duation * -1,
            )
            if not e:
                return get_error_data_result(message="Document not found!")
            settings.docStoreConn.delete({"doc_id": doc.id}, search.index_name(tenant_id), dataset_id)

    return get_result()


@manager.route("/datasets/<dataset_id>/documents/<document_id>", methods=["GET"])  # noqa: F821
@token_required
def download(tenant_id, dataset_id, document_id):
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
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(message=f"You do not own the dataset {dataset_id}.")
    doc = DocumentService.query(kb_id=dataset_id, id=document_id)
    if not doc:
        return get_error_data_result(
            message=f"The dataset not own the document {document_id}."
        )
    # The process of downloading
    doc_id, doc_location = File2DocumentService.get_storage_address(
        doc_id=document_id
    )  # minio address
    file_stream = STORAGE_IMPL.get(doc_id, doc_location)
    if not file_stream:
        return construct_json_result(
            message="This file is empty.", code=settings.RetCode.DATA_ERROR
        )
    file = BytesIO(file_stream)
    # Use send_file with a proper filename and MIME type
    return send_file(
        file,
        as_attachment=True,
        download_name=doc[0].name,
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
    id = request.args.get("id")
    name = request.args.get("name")
    if not DocumentService.query(id=id, kb_id=dataset_id):
        return get_error_data_result(message=f"You don't own the document {id}.")
    if not DocumentService.query(name=name, kb_id=dataset_id):
        return get_error_data_result(message=f"You don't own the document {name}.")
    page = int(request.args.get("page", 1))
    keywords = request.args.get("keywords", "")
    page_size = int(request.args.get("page_size", 30))
    orderby = request.args.get("orderby", "create_time")
    if request.args.get("desc") == "False":
        desc = False
    else:
        desc = True
    docs, tol = DocumentService.get_list(
        dataset_id, page, page_size, orderby, desc, keywords, id, name
    )

    # rename key's name
    renamed_doc_list = []
    for doc in docs:
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
        for key, value in doc.items():
            if key == "run":
                renamed_doc["run"] = run_mapping.get(str(value))
            new_key = key_mapping.get(key, key)
            renamed_doc[new_key] = value
            if key == "run":
                renamed_doc["run"] = run_mapping.get(value)
        renamed_doc_list.append(renamed_doc)
    return get_result(data={"total": tol, "docs": renamed_doc_list})


@manager.route("/datasets/<dataset_id>/documents", methods=["DELETE"])  # noqa: F821
@token_required
def delete(tenant_id, dataset_id):
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
              description: List of document IDs to delete.
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
    req = request.json
    if not req:
        doc_ids = None
    else:
        doc_ids = req.get("ids")
    if not doc_ids:
        doc_list = []
        docs = DocumentService.query(kb_id=dataset_id)
        for doc in docs:
            doc_list.append(doc.id)
    else:
        doc_list = doc_ids
    root_folder = FileService.get_root_folder(tenant_id)
    pf_id = root_folder["id"]
    FileService.init_knowledgebase_docs(pf_id, tenant_id)
    errors = ""
    for doc_id in doc_list:
        try:
            e, doc = DocumentService.get_by_id(doc_id)
            if not e:
                return get_error_data_result(message="Document not found!")
            tenant_id = DocumentService.get_tenant_id(doc_id)
            if not tenant_id:
                return get_error_data_result(message="Tenant not found!")

            b, n = File2DocumentService.get_storage_address(doc_id=doc_id)

            if not DocumentService.remove_document(doc, tenant_id):
                return get_error_data_result(
                    message="Database error (Document removal)!"
                )

            f2d = File2DocumentService.get_by_document_id(doc_id)
            FileService.filter_delete(
                [
                    File.source_type == FileSource.KNOWLEDGEBASE,
                    File.id == f2d[0].file_id,
                ]
            )
            File2DocumentService.delete_by_document_id(doc_id)

            STORAGE_IMPL.rm(b, n)
        except Exception as e:
            errors += str(e)

    if errors:
        return get_result(message=errors, code=settings.RetCode.SERVER_ERROR)

    return get_result()


@manager.route("/datasets/<dataset_id>/chunks", methods=["POST"])  # noqa: F821
@token_required
def parse(tenant_id, dataset_id):
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
    req = request.json
    if not req.get("document_ids"):
        return get_error_data_result("`document_ids` is required")
    for id in req["document_ids"]:
        doc = DocumentService.query(id=id, kb_id=dataset_id)
        if not doc:
            return get_error_data_result(message=f"You don't own the document {id}.")
        if doc[0].progress != 0.0:
            return get_error_data_result(
                "Can't stop parsing document with progress at 0 or 100"
            )
        info = {"run": "1", "progress": 0}
        info["progress_msg"] = ""
        info["chunk_num"] = 0
        info["token_num"] = 0
        DocumentService.update_by_id(id, info)
        settings.docStoreConn.delete({"doc_id": id}, search.index_name(tenant_id), dataset_id)
        TaskService.filter_delete([Task.doc_id == id])
        e, doc = DocumentService.get_by_id(id)
        doc = doc.to_dict()
        doc["tenant_id"] = tenant_id
        bucket, name = File2DocumentService.get_storage_address(doc_id=doc["id"])
        queue_tasks(doc, bucket, name)
    return get_result()


@manager.route("/datasets/<dataset_id>/chunks", methods=["DELETE"])  # noqa: F821
@token_required
def stop_parsing(tenant_id, dataset_id):
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
    req = request.json
    if not req.get("document_ids"):
        return get_error_data_result("`document_ids` is required")
    for id in req["document_ids"]:
        doc = DocumentService.query(id=id, kb_id=dataset_id)
        if not doc:
            return get_error_data_result(message=f"You don't own the document {id}.")
        if int(doc[0].progress) == 1 or int(doc[0].progress) == 0:
            return get_error_data_result(
                "Can't stop parsing document with progress at 0 or 1"
            )
        info = {"run": "2", "progress": 0, "chunk_num": 0}
        DocumentService.update_by_id(id, info)
        settings.docStoreConn.delete({"doc_id": doc.id}, search.index_name(tenant_id), dataset_id)
    return get_result()


@manager.route("/datasets/<dataset_id>/documents/<document_id>/chunks", methods=["GET"])  # noqa: F821
@token_required
def list_chunks(tenant_id, dataset_id, document_id):
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
        return get_error_data_result(
            message=f"You don't own the document {document_id}."
        )
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
        k = []
        for n in chunk.keys():
            if re.search(r"(_vec$|_sm_|_tks|_ltks)", n):
                k.append(n)
        for n in k:
            del chunk[n]
        if not chunk:
            return get_error_data_result(f"Chunk `{req.get('id')}` not found.")
        res['total'] = 1
        final_chunk = {
            "id":chunk.get("id",chunk.get("chunk_id")),
            "content":chunk["content_with_weight"],
            "document_id":chunk.get("doc_id",chunk.get("document_id")),
            "docnm_kwd":chunk["docnm_kwd"],
            "important_keywords":chunk.get("important_kwd",[]),
            "questions":chunk.get("question_kwd",[]),
            "dataset_id":chunk.get("kb_id",chunk.get("dataset_id")),
            "image_id":chunk["img_id"],
            "available":bool(chunk.get("available_int",1)),
            "positions":chunk.get("position_int",[]),
        }
        res["chunks"].append(final_chunk)
        _ = Chunk(**final_chunk)

    elif settings.docStoreConn.indexExist(search.index_name(tenant_id), dataset_id):
        sres = settings.retrievaler.search(query, search.index_name(tenant_id), [dataset_id], emb_mdl=None,
                                           highlight=True)
        res["total"] = sres.total
        for id in sres.ids:
            d = {
                "id": id,
                "content": (
                    rmSpace(sres.highlight[id])
                    if question and id in sres.highlight
                    else sres.field[id].get("content_with_weight", "")
                ),
                "document_id": sres.field[id]["doc_id"],
                "docnm_kwd": sres.field[id]["docnm_kwd"],
                "important_keywords": sres.field[id].get("important_kwd", []),
                "questions": sres.field[id].get("question_kwd", []),
                "dataset_id": sres.field[id].get("kb_id", sres.field[id].get("dataset_id")),
                "image_id": sres.field[id].get("img_id", ""),
                "available": bool(sres.field[id].get("available_int", 1)),
                "positions": sres.field[id].get("position_int",[]),
            }
            res["chunks"].append(d)
            _ = Chunk(**d) # validate the chunk
    return get_result(data=res)


@manager.route(  # noqa: F821
    "/datasets/<dataset_id>/documents/<document_id>/chunks", methods=["POST"]
)
@token_required
def add_chunk(tenant_id, dataset_id, document_id):
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
        return get_error_data_result(
            message=f"You don't own the document {document_id}."
        )
    doc = doc[0]
    req = request.json
    if not req.get("content"):
        return get_error_data_result(message="`content` is required")
    if "important_keywords" in req:
        if not isinstance(req["important_keywords"], list):
            return get_error_data_result(
                "`important_keywords` is required to be a list"
            )
    if "questions" in req:
        if not isinstance(req["questions"], list):
            return get_error_data_result(
                "`questions` is required to be a list"
            )
    chunk_id = xxhash.xxh64((req["content"] + document_id).encode("utf-8")).hexdigest()
    d = {
        "id": chunk_id,
        "content_ltks": rag_tokenizer.tokenize(req["content"]),
        "content_with_weight": req["content"],
    }
    d["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(d["content_ltks"])
    d["important_kwd"] = req.get("important_keywords", [])
    d["important_tks"] = rag_tokenizer.tokenize(
        " ".join(req.get("important_keywords", []))
    )
    d["question_kwd"] = req.get("questions", [])
    d["question_tks"] = rag_tokenizer.tokenize(
        "\n".join(req.get("questions", []))
    )
    d["create_time"] = str(datetime.datetime.now()).replace("T", " ")[:19]
    d["create_timestamp_flt"] = datetime.datetime.now().timestamp()
    d["kb_id"] = dataset_id
    d["docnm_kwd"] = doc.name
    d["doc_id"] = document_id
    embd_id = DocumentService.get_embd_id(document_id)
    embd_mdl = TenantLLMService.model_instance(
        tenant_id, LLMType.EMBEDDING.value, embd_id
    )
    v, c = embd_mdl.encode([doc.name, req["content"] if not d["question_kwd"] else "\n".join(d["question_kwd"])])
    v = 0.1 * v[0] + 0.9 * v[1]
    d["q_%d_vec" % len(v)] = v.tolist()
    settings.docStoreConn.insert([d], search.index_name(tenant_id), dataset_id)

    DocumentService.increment_chunk_num(doc.id, doc.kb_id, c, 1, 0)
    # rename keys
    key_mapping = {
        "id": "id",
        "content_with_weight": "content",
        "doc_id": "document_id",
        "important_kwd": "important_keywords",
        "question_kwd": "questions",
        "kb_id": "dataset_id",
        "create_timestamp_flt": "create_timestamp",
        "create_time": "create_time",
        "document_keyword": "document",
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
def rm_chunk(tenant_id, dataset_id, document_id):
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
              description: List of chunk IDs to remove.
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
    req = request.json
    condition = {"doc_id": document_id}
    if "chunk_ids" in req:
        condition["id"] = req["chunk_ids"]
    chunk_number = settings.docStoreConn.delete(condition, search.index_name(tenant_id), dataset_id)
    if chunk_number != 0:
        DocumentService.decrement_chunk_num(document_id, dataset_id, 1, chunk_number, 0)
    if "chunk_ids" in req and chunk_number != len(req["chunk_ids"]):
        return get_error_data_result(message=f"rm_chunk deleted chunks {chunk_number}, expect {len(req['chunk_ids'])}")
    return get_result(message=f"deleted {chunk_number} chunks")


@manager.route(  # noqa: F821
    "/datasets/<dataset_id>/documents/<document_id>/chunks/<chunk_id>", methods=["PUT"]
)
@token_required
def update_chunk(tenant_id, dataset_id, document_id, chunk_id):
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
        return get_error_data_result(
            message=f"You don't own the document {document_id}."
        )
    doc = doc[0]
    req = request.json
    if "content" in req:
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
        d["question_kwd"] = req.get("questions")
        d["question_tks"] = rag_tokenizer.tokenize("\n".join(req["questions"]))
    if "available" in req:
        d["available_int"] = int(req["available"])
    embd_id = DocumentService.get_embd_id(document_id)
    embd_mdl = TenantLLMService.model_instance(
        tenant_id, LLMType.EMBEDDING.value, embd_id
    )
    if doc.parser_id == ParserType.QA:
        arr = [t for t in re.split(r"[\n\t]", d["content_with_weight"]) if len(t) > 1]
        if len(arr) != 2:
            return get_error_data_result(
                message="Q&A must be separated by TAB/ENTER key."
            )
        q, a = rmPrefix(arr[0]), rmPrefix(arr[1])
        d = beAdoc(
            d, arr[0], arr[1], not any([rag_tokenizer.is_chinese(t) for t in q + a])
        )

    v, c = embd_mdl.encode([doc.name, d["content_with_weight"] if not d.get("question_kwd") else "\n".join(d["question_kwd"])])
    v = 0.1 * v[0] + 0.9 * v[1] if doc.parser_id != ParserType.QA else v[1]
    d["q_%d_vec" % len(v)] = v.tolist()
    settings.docStoreConn.update({"id": chunk_id}, d, search.index_name(tenant_id), dataset_id)
    return get_result()


@manager.route("/retrieval", methods=["POST"])  # noqa: F821
@token_required
def retrieval_test(tenant_id):
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
    req = request.json
    if not req.get("dataset_ids"):
        return get_error_data_result("`dataset_ids` is required.")
    kb_ids = req["dataset_ids"]
    if not isinstance(kb_ids, list):
        return get_error_data_result("`dataset_ids` should be a list")
    kbs = KnowledgebaseService.get_by_ids(kb_ids)
    for id in kb_ids:
        if not KnowledgebaseService.accessible(kb_id=id, user_id=tenant_id):
            return get_error_data_result(f"You don't own the dataset {id}.")
    embd_nms = list(set([kb.embd_id for kb in kbs]))
    if len(embd_nms) != 1:
        return get_result(
            message='Datasets use different embedding models."',
            code=settings.RetCode.AUTHENTICATION_ERROR,
        )
    if "question" not in req:
        return get_error_data_result("`question` is required.")
    page = int(req.get("page", 1))
    size = int(req.get("page_size", 30))
    question = req["question"]
    doc_ids = req.get("document_ids", [])
    if not isinstance(doc_ids, list):
        return get_error_data_result("`documents` should be a list")
    doc_ids_list = KnowledgebaseService.list_documents_by_ids(kb_ids)
    for doc_id in doc_ids:
        if doc_id not in doc_ids_list:
            return get_error_data_result(
                f"The datasets don't own the document {doc_id}"
            )
    similarity_threshold = float(req.get("similarity_threshold", 0.2))
    vector_similarity_weight = float(req.get("vector_similarity_weight", 0.3))
    top = int(req.get("top_k", 1024))
    if req.get("highlight") == "False" or req.get("highlight") == "false":
        highlight = False
    else:
        highlight = True
    try:
        e, kb = KnowledgebaseService.get_by_id(kb_ids[0])
        if not e:
            return get_error_data_result(message="Dataset not found!")
        embd_mdl = TenantLLMService.model_instance(
            kb.tenant_id, LLMType.EMBEDDING.value, llm_name=kb.embd_id
        )

        rerank_mdl = None
        if req.get("rerank_id"):
            rerank_mdl = TenantLLMService.model_instance(
                kb.tenant_id, LLMType.RERANK.value, llm_name=req["rerank_id"]
            )

        if req.get("keyword", False):
            chat_mdl = TenantLLMService.model_instance(kb.tenant_id, LLMType.CHAT)
            question += keyword_extraction(chat_mdl, question)

        retr = settings.retrievaler if kb.parser_id != ParserType.KG else settings.kg_retrievaler
        ranks = retr.retrieval(
            question,
            embd_mdl,
            kb.tenant_id,
            kb_ids,
            page,
            size,
            similarity_threshold,
            vector_similarity_weight,
            top,
            doc_ids,
            rerank_mdl=rerank_mdl,
            highlight=highlight,
        )
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
                "kb_id":"dataset_id"
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
                code=settings.RetCode.DATA_ERROR,
            )
        return server_error_response(e)

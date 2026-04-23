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
import base64
import datetime
import re

import xxhash
from pydantic import BaseModel, Field, validator
from quart import request

from api.apps import login_required
from api.db.joint_services.tenant_model_service import (
    get_model_config_by_id,
    get_model_config_by_type_and_name,
)
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.tenant_llm_service import TenantLLMService
from api.utils.api_utils import (
    add_tenant_id_to_kwargs,
    check_duplicate_ids,
    get_error_data_result,
    get_request_json,
    get_result,
    server_error_response,
)
from api.utils.image_utils import store_chunk_image
from common import settings
from common.constants import LLMType, ParserType, RetCode
from common.misc_utils import thread_pool_exec
from common.string_utils import is_content_empty, remove_redundant_spaces
from common.tag_feature_utils import validate_tag_features
from rag.app.qa import beAdoc, rmPrefix
from rag.nlp import rag_tokenizer, search


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


def _map_doc(doc):
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
        renamed_doc[key_mapping.get(key, key)] = value
        if key == "run":
            renamed_doc["run"] = run_mapping.get(str(value))
    return renamed_doc


def _strip_chunk_runtime_fields(chunk):
    for name in [name for name in chunk.keys() if re.search(r"(_vec$|_sm_|_tks|_ltks)", name)]:
        del chunk[name]
    return chunk


@manager.route("/datasets/<dataset_id>/documents/<document_id>/chunks", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def list_chunks(tenant_id, dataset_id, document_id):
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(message=f"You don't own the document {document_id}.")
    doc = doc[0]
    req = request.args
    page = int(req.get("page", 1))
    size = int(req.get("page_size", 30))
    question = req.get("keywords", "")
    query = {
        "doc_ids": [document_id],
        "page": page,
        "size": size,
        "question": question,
        "sort": True,
    }
    if "available" in req:
        query["available_int"] = 1 if req["available"] == "true" else 0

    res = {"total": 0, "chunks": [], "doc": _map_doc(doc)}
    if req.get("id"):
        chunk = settings.docStoreConn.get(req.get("id"), search.index_name(tenant_id), [dataset_id])
        if not chunk:
            return get_result(message=f"Chunk not found: {dataset_id}/{req.get('id')}", code=RetCode.DATA_ERROR)
        if str(chunk.get("doc_id", chunk.get("document_id"))) != str(document_id):
            return get_result(message=f"Chunk not found: {dataset_id}/{req.get('id')}", code=RetCode.DATA_ERROR)
        _strip_chunk_runtime_fields(chunk)
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
        sres = await settings.retriever.search(
            query,
            search.index_name(tenant_id),
            [dataset_id],
            emb_mdl=None,
            highlight=True,
        )
        res["total"] = sres.total
        for chunk_id in sres.ids:
            d = {
                "id": chunk_id,
                "content": (
                    remove_redundant_spaces(sres.highlight[chunk_id])
                    if question and chunk_id in sres.highlight
                    else sres.field[chunk_id].get("content_with_weight", "")
                ),
                "document_id": sres.field[chunk_id]["doc_id"],
                "docnm_kwd": sres.field[chunk_id]["docnm_kwd"],
                "important_keywords": sres.field[chunk_id].get("important_kwd", []),
                "tag_kwd": sres.field[chunk_id].get("tag_kwd", []),
                "questions": sres.field[chunk_id].get("question_kwd", []),
                "dataset_id": sres.field[chunk_id].get("kb_id", sres.field[chunk_id].get("dataset_id")),
                "image_id": sres.field[chunk_id].get("img_id", ""),
                "available": bool(int(sres.field[chunk_id].get("available_int", "1"))),
                "positions": sres.field[chunk_id].get("position_int", []),
            }
            res["chunks"].append(d)
            _ = Chunk(**d)
    return get_result(data=res)


@manager.route("/datasets/<dataset_id>/documents/<document_id>/chunks/<chunk_id>", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def get_chunk(tenant_id, dataset_id, document_id, chunk_id):
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(message=f"You don't own the document {document_id}.")
    try:
        chunk = settings.docStoreConn.get(chunk_id, search.index_name(tenant_id), [dataset_id])
        if chunk is None or str(chunk.get("doc_id", chunk.get("document_id"))) != str(document_id):
            return get_result(data=False, message="Chunk not found!", code=RetCode.DATA_ERROR)
        return get_result(data=_strip_chunk_runtime_fields(chunk))
    except Exception as e:
        if str(e).find("NotFoundError") >= 0:
            return get_result(data=False, message="Chunk not found!", code=RetCode.DATA_ERROR)
        return server_error_response(e)


@manager.route("/datasets/<dataset_id>/documents/<document_id>/chunks", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def add_chunk(tenant_id, dataset_id, document_id):
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(message=f"You don't own the document {document_id}.")
    doc = doc[0]
    req = await get_request_json()
    if is_content_empty(req.get("content")):
        return get_error_data_result(message="`content` is required")
    if "important_keywords" in req and not isinstance(req["important_keywords"], list):
        return get_error_data_result("`important_keywords` is required to be a list")
    if "questions" in req and not isinstance(req["questions"], list):
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
        try:
            d["tag_feas"] = validate_tag_features(req["tag_feas"])
        except ValueError as exc:
            return get_error_data_result(f"`tag_feas` {exc}")

    image_base64 = req.get("image_base64")
    if image_base64:
        d["img_id"] = f"{dataset_id}-{chunk_id}"
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
    d[f"q_{len(v)}_vec"] = v.tolist()
    settings.docStoreConn.insert([d], search.index_name(tenant_id), dataset_id)

    if image_base64:
        store_chunk_image(dataset_id, chunk_id, base64.b64decode(image_base64))

    DocumentService.increment_chunk_num(doc.id, doc.kb_id, c, 1, 0)
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
    renamed_chunk = {new_key: d[key] for key, new_key in key_mapping.items() if key in d}
    _ = Chunk(**renamed_chunk)
    return get_result(data={"chunk": renamed_chunk})


@manager.route("/datasets/<dataset_id>/documents/<document_id>/chunks", methods=["DELETE"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def rm_chunk(tenant_id, dataset_id, document_id):
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    docs = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not docs:
        return get_error_data_result(message=f"You don't own the document {document_id}.")
    req = await get_request_json()
    if not req:
        return get_result()

    chunk_ids = req.get("chunk_ids")
    if not chunk_ids:
        if req.get("delete_all") is True:
            doc = docs[0]
            DocumentService.delete_chunk_images(doc, tenant_id)
            chunk_number = settings.docStoreConn.delete({"doc_id": document_id}, search.index_name(tenant_id), dataset_id)
            if chunk_number != 0:
                DocumentService.decrement_chunk_num(document_id, dataset_id, 1, chunk_number, 0)
            return get_result(message=f"deleted {chunk_number} chunks")
        return get_result()

    unique_chunk_ids, duplicate_messages = check_duplicate_ids(chunk_ids, "chunk")
    chunk_number = settings.docStoreConn.delete(
        {"doc_id": document_id, "id": unique_chunk_ids},
        search.index_name(tenant_id),
        dataset_id,
    )
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


@manager.route("/datasets/<dataset_id>/documents/<document_id>/chunks/<chunk_id>", methods=["PATCH"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def update_chunk(tenant_id, dataset_id, document_id, chunk_id):
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(message=f"You don't own the document {document_id}.")
    doc = doc[0]
    chunk = settings.docStoreConn.get(chunk_id, search.index_name(tenant_id), [dataset_id])
    if chunk is None or str(chunk.get("doc_id", chunk.get("document_id"))) != str(document_id):
        return get_error_data_result(f"Can't find this chunk {chunk_id}")
    req = await get_request_json()
    content = req.get("content")
    if content is not None:
        if is_content_empty(content):
            return get_error_data_result(message="`content` is required")
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
        try:
            d["tag_feas"] = validate_tag_features(req["tag_feas"])
        except ValueError as exc:
            return get_error_data_result(f"`tag_feas` {exc}")
    image_base64 = req.get("image_base64")
    if image_base64:
        d["img_id"] = f"{dataset_id}-{chunk_id}"
        d["doc_type_kwd"] = "image"

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

    v, _ = embd_mdl.encode(
        [
            doc.name,
            d["content_with_weight"] if not d.get("question_kwd") else "\n".join(d["question_kwd"]),
        ]
    )
    v = 0.1 * v[0] + 0.9 * v[1] if doc.parser_id != ParserType.QA else v[1]
    d[f"q_{len(v)}_vec"] = v.tolist()
    settings.docStoreConn.update({"id": chunk_id}, d, search.index_name(tenant_id), dataset_id)
    if image_base64:
        store_chunk_image(dataset_id, chunk_id, base64.b64decode(image_base64))
    return get_result()


@manager.route("/datasets/<dataset_id>/documents/<document_id>/chunks", methods=["PATCH"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def switch_chunks(tenant_id, dataset_id, document_id):
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

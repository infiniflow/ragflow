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
import binascii
import datetime
import logging
import re

import xxhash
from pydantic import BaseModel, Field, validator
from quart import request

from api.apps import login_required
from api.db.joint_services.tenant_model_service import (
    get_model_config_from_provider_instance,
    get_tenant_default_model_by_type,
)
from api.db.db_models import Document, Task
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api.db.services.task_service import TaskService, cancel_all_task_of, queue_tasks
from api.db.services.tenant_llm_service import TenantLLMService
from api.utils.api_utils import (
    add_tenant_id_to_kwargs,
    check_duplicate_ids,
    construct_json_result,
    get_error_data_result,
    get_request_json,
    get_result,
    server_error_response,
    token_required,
)
from api.utils.pagination_utils import validate_rest_api_page_size
from api.utils.image_utils import store_chunk_image
from api.utils.reference_metadata_utils import (
    enrich_chunks_with_document_metadata,
    resolve_reference_metadata_preferences,
)
from common import settings
from common.constants import LLMType, ParserType, RetCode, TaskStatus
from common.metadata_utils import convert_conditions, meta_filter
from common.misc_utils import thread_pool_exec
from common.string_utils import is_content_empty, remove_redundant_spaces
from common.tag_feature_utils import validate_tag_features
from rag.app.tag import label_question
from rag.nlp import search
from rag.prompts.generator import cross_languages, keyword_extraction


DOC_STOP_PARSING_INVALID_STATE_MESSAGE = "Can't stop parsing document that has not started or already completed"
DOC_STOP_PARSING_INVALID_STATE_ERROR_CODE = "DOC_STOP_PARSING_INVALID_STATE"


def _decode_chunk_image_base64(image_base64):
    if not isinstance(image_base64, str) or not image_base64.strip():
        return None, "`image_base64` must be a non-empty string"
    try:
        image_binary = base64.b64decode(image_base64, validate=True)
    except (binascii.Error, ValueError):
        return None, "Invalid `image_base64`"
    if not image_binary:
        return None, "`image_base64` is empty"
    return image_binary, None


def _store_chunk_image_or_error(dataset_id, chunk_id, image_binary):
    try:
        store_chunk_image(dataset_id, chunk_id, image_binary)
    except Exception:
        logging.exception(
            "Failed to store chunk image. dataset_id=%s chunk_id=%s",
            dataset_id,
            chunk_id,
        )
        return "Failed to store chunk image"
    return None


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


def _get_dataset_tenant_id(dataset_id):
    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return None
    return kb.tenant_id


def _resolve_reference_metadata(req: dict, search_config: dict | None = None):
    return resolve_reference_metadata_preferences(req, search_config)


def _enrich_chunks_with_document_metadata(chunks: list[dict], metadata_fields=None) -> None:
    enrich_chunks_with_document_metadata(chunks, metadata_fields)


@manager.route("/datasets/<dataset_id>/chunks", methods=["POST"])  # noqa: F821
@token_required
async def parse(tenant_id, dataset_id):
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


@manager.route("/retrieval", methods=["POST"])  # noqa: F821
@token_required
async def retrieval_test(tenant_id):
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
    embd_nms = list(set([TenantLLMService.split_model_name_and_factory(kb.embd_id)[0] for kb in kbs]))
    if len(embd_nms) != 1:
        return get_result(message="Datasets use different embedding models.", code=RetCode.DATA_ERROR)
    if "question" not in req:
        return get_error_data_result("`question` is required.")
    page = int(req.get("page", 1))
    size = validate_rest_api_page_size(int(req.get("page_size", 30)))
    question = req["question"].strip() if isinstance(req["question"], str) else req["question"]
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
            if not doc_ids and metadata_condition.get("conditions"):
                return get_result(data={"total": 0, "chunks": [], "doc_aggs": {}})
            if metadata_condition and not doc_ids:
                doc_ids = ["-999"]
        else:
            doc_ids = None
    similarity_threshold = float(req.get("similarity_threshold", 0.2))
    vector_similarity_weight = float(req.get("vector_similarity_weight", 0.3))
    top = int(req.get("top_k", 1024))
    if top <= 0:
        return get_error_data_result("`top_k` must be greater than 0")
    highlight_val = req.get("highlight", None)
    if highlight_val is None:
        highlight = False
    elif isinstance(highlight_val, bool):
        highlight = highlight_val
    elif isinstance(highlight_val, str) and highlight_val.lower() in ["true", "false"]:
        highlight = highlight_val.lower() == "true"
    else:
        return get_error_data_result("`highlight` should be a boolean")
    include_metadata, metadata_fields = _resolve_reference_metadata(req)
    try:
        tenant_ids = list(set([kb.tenant_id for kb in kbs]))
        e, kb = KnowledgebaseService.get_by_id(kb_ids[0])
        if not e:
            return get_error_data_result(message="Dataset not found!")
        embd_model_config = get_model_config_from_provider_instance(kb.tenant_id, LLMType.EMBEDDING, kb.embd_id)
        embd_mdl = LLMBundle(kb.tenant_id, embd_model_config)

        rerank_mdl = None
        if req.get("rerank_id"):
            rerank_model_config = get_model_config_from_provider_instance(kb.tenant_id, LLMType.RERANK, req["rerank_id"])
            rerank_mdl = LLMBundle(kb.tenant_id, rerank_model_config)

        if langs:
            question = await cross_languages(kb.tenant_id, None, question, langs)
        if req.get("keyword", False):
            chat_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.CHAT)
            question += await keyword_extraction(LLMBundle(kb.tenant_id, chat_model_config), question)

        ranks = await settings.retriever.retrieval(
            question, embd_mdl, tenant_ids, kb_ids, page, size, similarity_threshold,
            vector_similarity_weight, top, doc_ids, rerank_mdl=rerank_mdl,
            highlight=highlight, rank_feature=label_question(question, kbs),
        )
        if toc_enhance:
            chat_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.CHAT)
            cks = await settings.retriever.retrieval_by_toc(question, ranks["chunks"], tenant_ids, LLMBundle(kb.tenant_id, chat_model_config), size)
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
        if include_metadata:
            logging.info("sdk.retrieval reference_metadata enabled dataset_ids=%s fields=%s chunks=%s", kb_ids, sorted(metadata_fields) if metadata_fields else None, len(ranks["chunks"]))
            enrich_chunks_with_document_metadata(ranks["chunks"], metadata_fields)

        key_mapping = {
            "chunk_id": "id",
            "content_with_weight": "content",
            "doc_id": "document_id",
            "important_kwd": "important_keywords",
            "question_kwd": "questions",
            "docnm_kwd": "document_keyword",
            "kb_id": "dataset_id",
        }
        ranks["chunks"] = [{key_mapping.get(key, key): value for key, value in chunk.items()} for chunk in ranks["chunks"]]
        return get_result(data=ranks)
    except Exception as e:
        if "not_found" in str(e):
            return get_result(message="No chunk found! Check the chunk status please!", code=RetCode.DATA_ERROR)
        return server_error_response(e)


@manager.route("/datasets/<dataset_id>/documents/<document_id>/chunks", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def list_chunks(tenant_id, dataset_id, document_id):
    from rag.nlp import search

    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    dataset_tenant_id = _get_dataset_tenant_id(dataset_id)
    if not dataset_tenant_id:
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(message=f"You don't own the document {document_id}.")
    doc = doc[0]
    req = request.args
    page = int(req.get("page", 1))
    size = validate_rest_api_page_size(int(req.get("page_size", 30)))
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
        chunk = settings.docStoreConn.get(req.get("id"), search.index_name(dataset_tenant_id), [dataset_id])
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
    elif settings.docStoreConn.index_exist(search.index_name(dataset_tenant_id), dataset_id):
        sres = await settings.retriever.search(
            query,
            search.index_name(dataset_tenant_id),
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
    from rag.nlp import search

    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    dataset_tenant_id = _get_dataset_tenant_id(dataset_id)
    if not dataset_tenant_id:
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(message=f"You don't own the document {document_id}.")
    try:
        chunk = settings.docStoreConn.get(chunk_id, search.index_name(dataset_tenant_id), [dataset_id])
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
    from rag.nlp import rag_tokenizer, search

    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    dataset_tenant_id = _get_dataset_tenant_id(dataset_id)
    if not dataset_tenant_id:
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

    if "image_base64" in req:
        image_binary, image_err = _decode_chunk_image_base64(req.get("image_base64"))
        if image_err:
            return get_error_data_result(message=image_err)
        store_err = _store_chunk_image_or_error(dataset_id, chunk_id, image_binary)
        if store_err:
            return get_error_data_result(message=store_err)
        d["img_id"] = f"{dataset_id}-{chunk_id}"
        d["doc_type_kwd"] = "image"

    embd_id = DocumentService.get_embd_id(document_id)
    model_config = get_model_config_from_provider_instance(dataset_tenant_id, LLMType.EMBEDDING.value, embd_id)
    embd_mdl = TenantLLMService.model_instance(model_config)
    v, c = embd_mdl.encode([doc.name, req["content"] if not d["question_kwd"] else "\n".join(d["question_kwd"])])
    v = 0.1 * v[0] + 0.9 * v[1]
    d[f"q_{len(v)}_vec"] = v.tolist()
    settings.docStoreConn.insert([d], search.index_name(dataset_tenant_id), dataset_id)

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
    from rag.nlp import search

    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    dataset_tenant_id = _get_dataset_tenant_id(dataset_id)
    if not dataset_tenant_id:
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
            DocumentService.delete_chunk_images(doc, dataset_tenant_id)
            chunk_number = settings.docStoreConn.delete({"doc_id": document_id}, search.index_name(dataset_tenant_id), dataset_id)
            if chunk_number != 0:
                DocumentService.decrement_chunk_num(document_id, dataset_id, 1, chunk_number, 0)
            return get_result(message=f"deleted {chunk_number} chunks")
        return get_result()

    unique_chunk_ids, duplicate_messages = check_duplicate_ids(chunk_ids, "chunk")
    chunk_number = settings.docStoreConn.delete(
        {"doc_id": document_id, "id": unique_chunk_ids},
        search.index_name(dataset_tenant_id),
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
    from rag.app.qa import beAdoc, rmPrefix
    from rag.nlp import rag_tokenizer, search

    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    dataset_tenant_id = _get_dataset_tenant_id(dataset_id)
    if not dataset_tenant_id:
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(message=f"You don't own the document {document_id}.")
    doc = doc[0]
    chunk = settings.docStoreConn.get(chunk_id, search.index_name(dataset_tenant_id), [dataset_id])
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
    if "image_base64" in req:
        image_binary, image_err = _decode_chunk_image_base64(req.get("image_base64"))
        if image_err:
            return get_error_data_result(message=image_err)
        store_err = _store_chunk_image_or_error(dataset_id, chunk_id, image_binary)
        if store_err:
            return get_error_data_result(message=store_err)
        d["img_id"] = f"{dataset_id}-{chunk_id}"
        d["doc_type_kwd"] = "image"

    embd_id = DocumentService.get_embd_id(document_id)
    model_config = get_model_config_from_provider_instance(dataset_tenant_id, LLMType.EMBEDDING.value, embd_id)
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
    settings.docStoreConn.update({"id": chunk_id}, d, search.index_name(dataset_tenant_id), dataset_id)
    return get_result()


@manager.route("/datasets/<dataset_id>/documents/<document_id>/chunks", methods=["PATCH"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def switch_chunks(tenant_id, dataset_id, document_id):
    from rag.nlp import search

    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    dataset_tenant_id = _get_dataset_tenant_id(dataset_id)
    if not dataset_tenant_id:
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
                    search.index_name(dataset_tenant_id),
                    doc.kb_id,
                ):
                    return get_error_data_result(message="Index updating failure")
            return get_result(data=True)

        return await thread_pool_exec(_switch_sync)
    except Exception as e:
        return server_error_response(e)

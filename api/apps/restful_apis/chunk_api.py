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
import json
import logging
import re

import xxhash
from pydantic import BaseModel, Field, validator
from quart import request

from api.apps import login_required
from api.db.joint_services.tenant_model_service import (
    split_model_name,
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
)
from api.utils.pagination_utils import validate_rest_api_page_size
from api.utils.image_utils import store_chunk_image
from api.utils.reference_metadata_utils import (
    enrich_chunks_with_document_metadata,
    resolve_reference_metadata_preferences,
)
from common import settings
from common.constants import LLMType, ParserType, RetCode, TaskStatus
from common.doc_store.doc_store_base import OrderByExpr
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


def _get_query_id_list(args, name: str) -> list[str]:
    values = args.getlist(name) if hasattr(args, "getlist") else [args.get(name)]
    ids: list[str] = []
    seen: set[str] = set()
    for value in values:
        for item in str(value or "").split(","):
            item = item.strip()
            if item and item not in seen:
                ids.append(item)
                seen.add(item)
    return ids


def _strip_chunk_runtime_fields(chunk):
    for name in [name for name in chunk.keys() if re.search(r"(_vec$|_sm_|_tks|_ltks)", name)]:
        del chunk[name]
    return chunk


def _get_dataset_tenant_id(dataset_id):
    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return None
    return kb.tenant_id


def _compilation_template_kind(kind) -> str:
    if not isinstance(kind, str):
        return ""
    normalized = kind.strip().lower().replace("-", "_")
    if normalized in {"pageindex", "page_index", "knowledge_graph"}:
        return "timeline"
    return normalized


def _resolve_reference_metadata(req: dict, search_config: dict | None = None):
    return resolve_reference_metadata_preferences(req, search_config)


def _enrich_chunks_with_document_metadata(chunks: list[dict], metadata_fields=None) -> None:
    enrich_chunks_with_document_metadata(chunks, metadata_fields)


@manager.route("/datasets/<dataset_id>/chunks", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def parse(tenant_id, dataset_id):
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    dataset_tenant_id = _get_dataset_tenant_id(dataset_id)
    if not dataset_tenant_id:
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    e, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not e:
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    if kb.pipeline_id:
        return get_error_data_result(
            message="Datasets configured with an ingestion pipeline cannot be parsed with `/datasets/{dataset_id}/chunks`. Use `/documents/ingest` instead.", code=RetCode.ARGUMENT_ERROR
        )
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
        index_name = search.index_name(dataset_tenant_id)
        if settings.docStoreConn.index_exist(index_name, doc[0].kb_id):
            settings.docStoreConn.delete({"doc_id": id}, index_name, doc[0].kb_id)
        else:
            logging.info(
                "Skipping chunk delete during parse for doc %s: index %s/%s does not exist",
                id,
                index_name,
                doc[0].kb_id,
            )
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
@login_required
@add_tenant_id_to_kwargs
async def stop_parsing(tenant_id, dataset_id):
    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    dataset_tenant_id = _get_dataset_tenant_id(dataset_id)
    if not dataset_tenant_id:
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
        index_name = search.index_name(dataset_tenant_id)
        if settings.docStoreConn.index_exist(index_name, doc[0].kb_id):
            settings.docStoreConn.delete({"doc_id": doc[0].id}, index_name, doc[0].kb_id)
        else:
            logging.info(
                "Skipping chunk delete during stop_parsing for doc %s: index %s/%s does not exist",
                doc[0].id,
                index_name,
                doc[0].kb_id,
            )
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
@login_required
@add_tenant_id_to_kwargs
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
    embd_nms = list(set([split_model_name(kb.embd_id)[0] for kb in kbs]))
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
    chunk_ids = _get_query_id_list(req, "chunk_ids")
    query = {
        "doc_ids": [document_id],
        "page": page,
        "size": size,
        "question": question,
        "sort": True,
        "must_not": {"exists": "compile_kwd"},
    }
    if chunk_ids:
        query["id"] = chunk_ids
    if "available" in req:
        query["available_int"] = 1 if req["available"] == "true" else 0

    res = {"total": 0, "chunks": [], "doc": _map_doc(doc)}
    if req.get("id"):
        chunk = settings.docStoreConn.get(req.get("id"), search.index_name(dataset_tenant_id), [dataset_id])
        if not chunk:
            return get_result(message=f"Chunk not found: {dataset_id}/{req.get('id')}", code=RetCode.DATA_ERROR)
        if str(chunk.get("doc_id", chunk.get("document_id"))) != str(document_id):
            return get_result(message=f"Chunk not found: {dataset_id}/{req.get('id')}", code=RetCode.DATA_ERROR)
        if chunk.get("compile_kwd"):
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
                "content": (remove_redundant_spaces(sres.highlight[chunk_id]) if question and chunk_id in sres.highlight else sres.field[chunk_id].get("content_with_weight", "")),
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
        if chunk.get("compile_kwd"):
            return get_result(data=False, message="Chunk not found!", code=RetCode.DATA_ERROR)
        return get_result(data=_strip_chunk_runtime_fields(chunk))
    except Exception as e:
        if str(e).find("NotFoundError") >= 0:
            return get_result(data=False, message="Chunk not found!", code=RetCode.DATA_ERROR)
        return server_error_response(e)


@manager.route("/datasets/<dataset_id>/documents/<document_id>/structure/graph", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def get_document_structure_graph(tenant_id, dataset_id, document_id):
    """Return per-template structure graphs for a document.

    Response shape::

        {
          "templates": [
            {
              "template_id": "<id> | 'legacy:<compile_kwd>'",
              "template_name": "<display name>",
              "kind": "list | set | hypergraph | timeline | page_index | …",
              "entities": [...],
              "relations": [...]
            },
            ...
          ]
        }

    Rows that pre-date the ``compilation_template_ids`` stamp are surfaced
    under a synthetic ``legacy:<compile_kwd>`` bucket so an in-flight
    migration doesn't drop their data on the floor. Empty templates
    (zero entities AND zero relations) are filtered out.
    """
    from rag.nlp import search
    from api.db.services.compilation_template_group_service import CompilationTemplateGroupService

    if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    dataset_tenant_id = _get_dataset_tenant_id(dataset_id)
    if not dataset_tenant_id:
        return get_error_data_result(message=f"You don't own the dataset {dataset_id}.")
    docs = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not docs:
        return get_error_data_result(message=f"You don't own the document {document_id}.")

    # Resolve the doc's configured template group → child template ids
    # so we can render tabs in the order the user picked them.
    # Artifacts-kind templates render on the dataset Artifact tab, not
    # here, so they're filtered out.
    parser_config = docs[0].parser_config or {}

    def _group_ids(raw) -> list[str]:
        if isinstance(raw, str):
            raw = [raw]
        if not isinstance(raw, list):
            return []
        ids: list[str] = []
        seen: set[str] = set()
        for gid in raw:
            if not isinstance(gid, str):
                continue
            gid = gid.strip()
            if gid and gid not in seen:
                seen.add(gid)
                ids.append(gid)
        return ids

    group_ids: list[str] = []
    if isinstance(parser_config, dict):
        if "compilation_template_group_id" in parser_config:
            group_ids = _group_ids(parser_config.get("compilation_template_group_id"))
        elif isinstance(parser_config.get("ext"), dict):
            group_ids = _group_ids(parser_config["ext"].get("compilation_template_group_id"))

    configured_ids: list[str] = []
    seen_configured_ids: set[str] = set()
    template_meta: dict[str, dict] = {}
    template_meta_by_kind: dict[str, list[dict]] = {}
    for group_id in group_ids:
        group = CompilationTemplateGroupService.get_saved(group_id, tenant_id)
        if not group:
            continue
        for template in group.get("templates") or []:
            if not isinstance(template, dict):
                continue
            template_id = str(template.get("id") or "").strip()
            if not template_id or template_id in seen_configured_ids:
                continue
            config = template.get("config") if isinstance(template.get("config"), dict) else {}
            raw_kind = (config.get("kind") if isinstance(config, dict) else "") or template.get("kind") or ""
            kind_norm = _compilation_template_kind(raw_kind)
            if kind_norm == "artifacts":
                continue
            seen_configured_ids.add(template_id)
            configured_ids.append(template_id)
            meta = {
                "template_id": template_id,
                "template_name": template.get("name") or template_id,
                "kind": raw_kind or kind_norm,
                "kind_norm": kind_norm,
            }
            template_meta[template_id] = meta
            template_meta_by_kind.setdefault(kind_norm, []).append(meta)

    # Load every graph row for this doc in one shot. Each row corresponds
    # to one (compile_kwd, template_id) tuple — written by
    # ``_struct_upsert_graph_json``.
    index_name = search.index_name(dataset_tenant_id)
    fields = [
        "content_with_weight",
        "compile_kwd",
        "compilation_template_ids",
        "compilation_template_kind_kwd",
    ]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            {"doc_id": [document_id], "knowledge_graph_kwd": ["graph"]},
            [],
            OrderByExpr(),
            0,
            1000,
            index_name,
            [dataset_id],
        )
        rows = settings.docStoreConn.get_fields(res, fields)

        # The RAPTOR graph row is identified by ``compile_kwd``
        # alone — it intentionally doesn't carry ``knowledge_graph_kwd``
        # (which belongs to the KG feature). Query it separately and
        # union into the same bucket map below.
        res_raptor = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            {"doc_id": [document_id], "compile_kwd": ["raptor_graph"]},
            [],
            OrderByExpr(),
            0,
            16,
            index_name,
            [dataset_id],
        )
        raptor_rows = settings.docStoreConn.get_fields(res_raptor, fields)
    except Exception as e:
        return server_error_response(e)

    # Merge the two field-maps so the grouping loop below treats them
    # identically. Raptor rows clobber by id, which is fine — both
    # sources produce stable per-row ids.
    if raptor_rows:
        rows = dict(rows or {})
        rows.update(raptor_rows)

    def _row_template_id(row: dict) -> str | None:
        raw = row.get("compilation_template_ids")
        if isinstance(raw, list):
            for v in raw:
                if isinstance(v, str) and v.strip():
                    return v.strip()
        if isinstance(raw, str) and raw.strip():
            return raw.strip()
        return None

    # Group: template_id → {entities, relations, kind}
    grouped: dict[str, dict] = {}
    for row in (rows or {}).values():
        graph = {}
        try:
            graph = json.loads(row.get("content_with_weight") or "{}")
        except Exception:
            continue
        if not isinstance(graph, dict):
            continue
        entities = graph.get("entities") or []
        relations = graph.get("relations") or []
        if not entities and not relations:
            continue

        tid = _row_template_id(row)
        compile_kwd_val = row.get("compile_kwd") or ""
        kind_val = row.get("compilation_template_kind_kwd") or compile_kwd_val

        # The RAPTOR graph row has no ``compilation_template_ids`` (it
        # isn't derived from a user-authored template). Treat it as its
        # own first-class bucket, not a legacy fallback.
        is_raptor = compile_kwd_val == "raptor_graph"

        if tid:
            bucket_id = tid
            row_kind_norm = _compilation_template_kind(kind_val)
            meta = template_meta.get(bucket_id)
            if not meta:
                kind_matches = template_meta_by_kind.get(row_kind_norm) or []
                if len(kind_matches) == 1:
                    meta = kind_matches[0]
            bucket_name = (meta or {}).get("template_name") or bucket_id
            bucket_kind = (meta or {}).get("kind") or kind_val
        elif is_raptor:
            bucket_id = "raptor"
            bucket_name = "RAPTOR Summary"
            bucket_kind = "raptor"
        else:
            # Legacy row: synthesize a stable id keyed by compile_kwd so
            # multiple legacy kinds (e.g. ``list`` + ``hypergraph``) on
            # the same doc surface as separate tabs.
            bucket_id = f"legacy:{compile_kwd_val}"
            bucket_name = f"Legacy ({compile_kwd_val})"
            bucket_kind = kind_val

        if bucket_id not in grouped:
            grouped[bucket_id] = {
                "template_id": bucket_id,
                "template_name": bucket_name,
                "kind": bucket_kind,
                "entities": [],
                "relations": [],
            }
        grouped[bucket_id]["entities"].extend(entities)
        grouped[bucket_id]["relations"].extend(relations)

    # Order: configured templates first (in the user's chosen order),
    # then any legacy buckets after.
    ordered_ids: list[str] = []
    for tid in configured_ids:
        if tid in grouped and tid not in ordered_ids:
            ordered_ids.append(tid)
    for bucket_id in grouped.keys():
        if bucket_id not in ordered_ids:
            ordered_ids.append(bucket_id)

    templates_out = [grouped[bid] for bid in ordered_ids if grouped[bid]["entities"] or grouped[bid]["relations"]]
    return get_result(data={"templates": templates_out})


@manager.route("/datasets/<dataset_id>/documents/<document_id>/structure/graph", methods=["DELETE"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def delete_document_structure_graph(tenant_id, dataset_id, document_id):
    """Delete one structure-graph tab for a document.

    Request body::

        {"template_id": "<template id> | legacy:<compile_kwd> | raptor"}

    Template-backed structure tabs remove both the compact graph row and
    the underlying entity/relation rows. RAPTOR only removes the graph
    projection row so summary chunks remain available for retrieval.
    """
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
    template_id = str(req.get("template_id") or "").strip()
    if not template_id:
        return get_error_data_result(message="`template_id` is required")

    index_name = search.index_name(dataset_tenant_id)

    def _delete(condition: dict) -> int:
        return settings.docStoreConn.delete(condition, index_name, dataset_id)

    try:
        deleted = 0
        if template_id == "raptor":
            deleted += _delete({"doc_id": [document_id], "compile_kwd": ["raptor_graph"]})
            return get_result(data={"deleted": deleted}, message=f"deleted {deleted} structure graph rows")

        if template_id.startswith("legacy:"):
            compile_kwd = template_id[len("legacy:") :].strip()
            if not compile_kwd:
                return get_error_data_result(message="`template_id` is invalid")
            base_condition = {"doc_id": [document_id], "compile_kwd": [compile_kwd]}
        else:
            base_condition = {"doc_id": [document_id], "compilation_template_ids": [template_id]}

        deleted += _delete({**base_condition, "knowledge_graph_kwd": ["graph"]})
        deleted += _delete({**base_condition, "knowledge_graph_kwd": ["entity", "relation"]})
        return get_result(data={"deleted": deleted}, message=f"deleted {deleted} structure graph rows")
    except Exception as e:
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
            chunk_number = settings.docStoreConn.delete(
                {"doc_id": document_id, "must_not": {"exists": "compile_kwd"}},
                search.index_name(dataset_tenant_id),
                dataset_id,
            )
            if chunk_number != 0:
                DocumentService.decrement_chunk_num(document_id, dataset_id, 1, chunk_number, 0)
            return get_result(message=f"deleted {chunk_number} chunks")
        return get_result()

    unique_chunk_ids, duplicate_messages = check_duplicate_ids(chunk_ids, "chunk")
    chunk_number = settings.docStoreConn.delete(
        {"doc_id": document_id, "id": unique_chunk_ids, "must_not": {"exists": "compile_kwd"}},
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
